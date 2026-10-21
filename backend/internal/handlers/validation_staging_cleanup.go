package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const validationStagingPreviewTTL = 5 * time.Minute

type validationStagingFile struct {
	Path      string `json:"path"`
	Relative  string `json:"relative"`
	Kind      string `json:"kind"`
	SizeBytes int64  `json:"sizeBytes"`
	Identity  string `json:"identity"`
}

type validationStagingPreviewResponse struct {
	JobID           uint                    `json:"jobId"`
	ExecutionNumber uint                    `json:"executionNumber"`
	ValidationRunID string                  `json:"validationRunId"`
	PreviewID       string                  `json:"previewId,omitempty"`
	ExpiresAt       *time.Time              `json:"expiresAt,omitempty"`
	State           string                  `json:"state"`
	WorkspacePath   string                  `json:"workspacePath"`
	Ownership       string                  `json:"ownership"`
	Reason          string                  `json:"reason"`
	Files           []validationStagingFile `json:"files"`
	TotalBytes      int64                   `json:"totalBytes"`
	Blockers        []string                `json:"blockers"`
}

type validationStagingCleanupResponse struct {
	JobID          uint     `json:"jobId"`
	PreviewID      string   `json:"previewId"`
	Outcome        string   `json:"outcome"`
	RemovedPaths   []string `json:"removedPaths"`
	RecoveredBytes int64    `json:"recoveredBytes"`
	Reason         string   `json:"reason,omitempty"`
	EvidenceID     string   `json:"evidenceId"`
}

type validationStagingSnapshot struct {
	Files        []validationStagingFile `json:"files"`
	TreeIdentity string                  `json:"treeIdentity"`
	OutputPath   string                  `json:"outputPath"`
	DismissedAt  *time.Time              `json:"dismissedAt,omitempty"`
	JobUpdatedAt time.Time               `json:"jobUpdatedAt"`
}

type explicitValidationStagingAuthorization struct {
	Preview  models.ValidationStagingCleanupPreview
	Snapshot validationStagingSnapshot
}

type validationStagingPartialRemovalError struct {
	paths []string
	bytes int64
	cause error
}

func (e *validationStagingPartialRemovalError) Error() string { return e.cause.Error() }
func (e *validationStagingPartialRemovalError) Unwrap() error { return e.cause }

type validationStagingJobLock struct {
	mu    sync.Mutex
	locks map[uint]*sync.Mutex
}

var validationStagingCleanupLocks = validationStagingJobLock{locks: map[uint]*sync.Mutex{}}

func lockValidationStagingJob(jobID uint) func() {
	validationStagingCleanupLocks.mu.Lock()
	lock := validationStagingCleanupLocks.locks[jobID]
	if lock == nil {
		lock = &sync.Mutex{}
		validationStagingCleanupLocks.locks[jobID] = lock
	}
	validationStagingCleanupLocks.mu.Unlock()
	lock.Lock()
	return lock.Unlock
}

func (h ValidationHandler) PreviewStagingCleanup(c *gin.Context) {
	jobID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || jobID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job id"})
		return
	}
	preview, err := createValidationStagingPreview(h.db, uint(jobID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, preview)
}

func (h ValidationHandler) ExecuteStagingCleanup(c *gin.Context) {
	jobID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || jobID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job id"})
		return
	}
	var input struct {
		PreviewID string `json:"previewId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.PreviewID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "previewId is required"})
		return
	}
	response, err := executeValidationStagingCleanup(h.db, uint(jobID), strings.TrimSpace(input.PreviewID), "local_operator")
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job or cleanup preview not found"})
			return
		}
		if errors.Is(err, errStaleHousekeepingPreview) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, response)
}

func createValidationStagingPreview(db *gorm.DB, jobID uint) (validationStagingPreviewResponse, error) {
	var job models.QueueJob
	if err := db.First(&job, jobID).Error; err != nil {
		return validationStagingPreviewResponse{}, err
	}
	response := validationStagingPreviewResponse{
		JobID: job.ID, ValidationRunID: job.ValidationRunID, State: "blocked",
		Ownership: "direct Work role child owned by QueueJob primary ID", Reason: "explicit Validation Dismiss staging cleanup",
		Files: []validationStagingFile{}, Blockers: []string{},
	}
	if job.ExecutionNumber != nil {
		response.ExecutionNumber = *job.ExecutionNumber
	}
	if job.Status == JobStatusCompleted && job.DismissedAt == nil {
		if err := validateCompletedValidationDismissal(db, job); err != nil {
			response.Blockers = append(response.Blockers, err.Error())
			return response, nil
		}
	} else if job.Status == JobStatusCompleted && job.DismissedAt != nil {
		if err := validateCompletedValidationDismissal(db, job); err != nil {
			response.Blockers = append(response.Blockers, err.Error())
			return response, nil
		}
	} else {
		response.Blockers = append(response.Blockers, "job is not a completed Validation exception")
		return response, nil
	}
	if job.ExecutionNumber == nil {
		response.Blockers = append(response.Blockers, "execution identity is missing")
		return response, nil
	}
	artifact, _, err := readLatestJobArtifact(db, job, "result")
	if err != nil {
		response.Blockers = append(response.Blockers, "current Result artifact is unavailable")
		return response, nil
	}
	identity, identityOK := resultIdentityFromArtifact(artifact)
	binding, bindingOK := resultBindingFromArtifact(artifact)
	if !identityOK || !bindingOK {
		response.Blockers = append(response.Blockers, "current Result identity is ambiguous")
		return response, nil
	}
	roles, err := scheduler.LoadStorageRoles(db)
	if err != nil {
		response.Blockers = append(response.Blockers, "storage roles could not be loaded")
		return response, nil
	}
	roots, err := housekeepingRoleRoots(roles)
	if err != nil {
		response.Blockers = append(response.Blockers, err.Error())
		return response, nil
	}
	workRoot := roots[string(scheduler.StorageRoleWork)]
	response.WorkspacePath = filepath.Join(workRoot, fmt.Sprintf("job-%d", job.ID))
	if workRoot == "" || unsafeOperationalRoleRoot(roots, "work") {
		response.Blockers = append(response.Blockers, "configured Work storage overlaps RAW, Library, or originals archive")
		return response, nil
	}
	workInfo, err := os.Lstat(workRoot)
	if err != nil || !workInfo.IsDir() || workInfo.Mode()&os.ModeSymlink != 0 {
		response.Blockers = append(response.Blockers, "configured Work storage root is missing or unsafe")
		return response, nil
	}
	lock := lockValidationStagingJob(job.ID)
	defer lock()
	workspaceInfo, err := os.Lstat(response.WorkspacePath)
	state := "ready"
	snapshot := validationStagingSnapshot{Files: []validationStagingFile{}, OutputPath: job.OutputPath, DismissedAt: job.DismissedAt, JobUpdatedAt: job.UpdatedAt}
	if errors.Is(err, os.ErrNotExist) {
		state = "noop"
		response.Reason = "QueueJob workspace is absent; no staged bytes can be recovered"
	} else if err != nil {
		response.Blockers = append(response.Blockers, "workspace could not be inspected")
		return response, nil
	} else {
		role, resolved, info, resolveErr := resolveOwnedFilePath(roots, response.WorkspacePath, []string{"work"})
		if resolveErr != nil || role != "work" || resolved != response.WorkspacePath || info == nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || workspaceInfo.Mode()&os.ModeSymlink != 0 {
			response.Blockers = append(response.Blockers, "workspace is not a clean canonical direct Work directory")
			return response, nil
		}
		files, tree, total, manifestErr := validationStagingManifest(response.WorkspacePath)
		if manifestErr != nil {
			response.Blockers = append(response.Blockers, manifestErr.Error())
			return response, nil
		}
		if strings.TrimSpace(job.OutputPath) != "" && !validationStagingContainsPath(response.WorkspacePath, job.OutputPath, files) {
			response.Blockers = append(response.Blockers, "persisted OutputPath is not contained in this QueueJob workspace")
			return response, nil
		}
		if explicitValidationStagingHasActiveReferences(db, job.ID, response.WorkspacePath) {
			response.Blockers = append(response.Blockers, "an active Worker, Queue, Publisher, maintenance, Test Encode, artifact, or other job reference may own this workspace")
			return response, nil
		}
		snapshot.Files, snapshot.TreeIdentity = files, tree
		response.Files, response.TotalBytes = validationStagingDisplayFiles(files), total
	}
	if len(response.Blockers) != 0 {
		return response, nil
	}
	encodedSnapshot, err := json.Marshal(snapshot)
	if err != nil {
		return validationStagingPreviewResponse{}, err
	}
	previewID, err := newValidationStagingID("vsc-preview-")
	if err != nil {
		return validationStagingPreviewResponse{}, err
	}
	expiresAt := time.Now().UTC().Add(validationStagingPreviewTTL)
	preview := models.ValidationStagingCleanupPreview{
		PreviewID: previewID, JobID: job.ID, ExecutionNumber: *job.ExecutionNumber,
		ExecutionPlanID: binding.ExecutionPlanID, ExecutionPlanVersion: binding.ExecutionPlanVersion,
		ExecutionPlanFingerprint: binding.ExecutionPlanFingerprint, ResultArtifactID: identity.ArtifactID,
		ResultGenerationToken: identity.GenerationToken, ResultGenerationVersion: identity.GenerationVersion,
		ResultOutputRevision: identity.OutputRevision, ValidationRunID: job.ValidationRunID,
		State: state, WorkRoot: workRoot, WorkspacePath: response.WorkspacePath,
		TreeIdentity: snapshot.TreeIdentity, ManifestJSON: string(encodedSnapshot), TotalBytes: response.TotalBytes,
		ExpiresAt: expiresAt, CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(&preview).Error; err != nil {
		return validationStagingPreviewResponse{}, err
	}
	response.PreviewID, response.ExpiresAt, response.State = previewID, &expiresAt, state
	return response, nil
}

func validationStagingManifest(root string) ([]validationStagingFile, string, int64, error) {
	files := make([]validationStagingFile, 0)
	var total int64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("workspace contains a symlink, special file, or unreadable entry: %s", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		kind := "file"
		if info.IsDir() {
			kind = "directory"
		} else {
			total += info.Size()
		}
		files = append(files, validationStagingFile{Path: path, Relative: relative, Kind: kind, SizeBytes: info.Size(), Identity: statIdentity(info, info.IsDir())})
		return nil
	})
	if err != nil {
		return nil, "", 0, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Relative < files[j].Relative })
	tree, err := directoryTreeIdentity(root)
	if err != nil {
		return nil, "", 0, err
	}
	return files, tree, total, nil
}

func validationStagingDisplayFiles(files []validationStagingFile) []validationStagingFile {
	displayed := make([]validationStagingFile, 0, len(files))
	for _, file := range files {
		if file.Kind == "file" {
			displayed = append(displayed, validationStagingFile{Path: file.Path, Kind: file.Kind, SizeBytes: file.SizeBytes})
		}
	}
	return displayed
}

func validationStagingContainsPath(workspace, outputPath string, files []validationStagingFile) bool {
	if !filepath.IsAbs(outputPath) || filepath.Clean(outputPath) != outputPath {
		return false
	}
	// The configured Work root is canonicalized before it is returned by
	// housekeepingRoleRoots. On platforms where the temp root is a symlink
	// (for example /var -> /private/var on macOS), QueueJob.OutputPath may
	// still use the equivalent lexical spelling. Resolve both spellings
	// before comparing ownership; a final symlink is never an owned output.
	outputInfo, err := os.Lstat(outputPath)
	if err != nil || outputInfo.Mode()&os.ModeSymlink != 0 || !outputInfo.Mode().IsRegular() {
		return false
	}
	canonicalWorkspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return false
	}
	canonicalOutput, err := filepath.EvalSymlinks(outputPath)
	if err != nil || !pathWithin(canonicalWorkspace, canonicalOutput) {
		return false
	}
	for _, file := range files {
		if file.Kind != "file" {
			continue
		}
		canonicalFile, err := filepath.EvalSymlinks(file.Path)
		if err == nil && filepath.Clean(canonicalFile) == filepath.Clean(canonicalOutput) {
			return true
		}
	}
	return false
}

func newValidationStagingID(prefix string) (string, error) {
	var value [24]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(value[:]), nil
}

func explicitValidationStagingHasActiveReferences(db *gorm.DB, jobID uint, workspace string) bool {
	if !db.Migrator().HasTable(&models.QueueJob{}) || !db.Migrator().HasTable(&models.ExecutionPlan{}) || !db.Migrator().HasTable(&models.AssetRecord{}) || !db.Migrator().HasTable(&models.Library{}) || !db.Migrator().HasTable(&models.SchedulerReservation{}) || !db.Migrator().HasTable(&models.AssetMaintenanceOperation{}) || !db.Migrator().HasTable(&models.PublisherPublication{}) || !db.Migrator().HasTable(&models.PublisherPublicationAttempt{}) || !db.Migrator().HasTable(&models.PublisherAttemptEvidence{}) || !db.Migrator().HasTable(&models.TestEncode{}) || !db.Migrator().HasTable(&models.TestEncodeArtifact{}) || !db.Migrator().HasTable(&models.CompatiblePreviewArtifactRecord{}) {
		return true
	}
	referencesWorkspace := func(path string) bool {
		return housekeepingPathWithin(workspace, path) || housekeepingPathWithin(path, workspace)
	}
	var job models.QueueJob
	if err := db.First(&job, jobID).Error; err != nil || job.StartedAt != nil && job.FinishedAt == nil {
		return true
	}
	for _, path := range []string{job.MediaPath, job.PublishedPath, job.PlannedPublishedPath, job.ReplacementTargetPath, job.OriginalArchivedPath} {
		if referencesWorkspace(path) {
			return true
		}
	}
	var assets []models.AssetRecord
	if err := db.Select("path, source_path").Find(&assets).Error; err != nil {
		return true
	}
	for _, asset := range assets {
		if referencesWorkspace(asset.Path) || referencesWorkspace(asset.SourcePath) {
			return true
		}
	}
	var libraries []models.Library
	if err := db.Select("source_path, destination_path").Find(&libraries).Error; err != nil {
		return true
	}
	for _, library := range libraries {
		if referencesWorkspace(library.SourcePath) || referencesWorkspace(library.DestinationPath) {
			return true
		}
	}
	var reservations int64
	if err := db.Model(&models.SchedulerReservation{}).Where("job_id = ? AND state IN ?", jobID, []string{scheduler.ReservationStateLocked, scheduler.ReservationStateActive}).Count(&reservations).Error; err != nil || reservations != 0 {
		return true
	}
	var operations []models.AssetMaintenanceOperation
	if err := db.Where("status IN ?", []string{maintenanceStatusQueued, maintenanceStatusRunning}).Find(&operations).Error; err != nil {
		return true
	}
	for _, operation := range operations {
		for _, path := range []string{operation.AssetPath, operation.TemporaryPath, operation.BackupPath} {
			if housekeepingPathWithin(workspace, path) || housekeepingPathWithin(path, workspace) {
				return true
			}
		}
	}
	var publications []models.PublisherPublication
	if err := db.Find(&publications).Error; err != nil {
		return true
	}
	for _, publication := range publications {
		for _, path := range []string{publication.SourcePath, publication.DestinationPath, publication.OriginalArchivedPath, publication.OriginalArchiveSourcePath, publication.OriginalArchiveTemporaryPath} {
			if referencesWorkspace(path) {
				return true
			}
		}
		referenced := false
		walkHousekeepingJSONStrings(publication.Manifest, func(path string) {
			if referencesWorkspace(path) {
				referenced = true
			}
		})
		if referenced {
			return true
		}
		if publication.QueueJobID == jobID {
			return true
		}
	}
	var publisherEvidence []models.PublisherAttemptEvidence
	if err := db.Find(&publisherEvidence).Error; err != nil {
		return true
	}
	for _, evidence := range publisherEvidence {
		referenced := false
		walkHousekeepingJSONStrings(evidence.Paths, func(path string) {
			if referencesWorkspace(path) {
				referenced = true
			}
		})
		if referenced {
			return true
		}
	}
	var otherPlans []models.ExecutionPlan
	if err := db.Where("job_id <> ?", jobID).Find(&otherPlans).Error; err != nil {
		return true
	}
	for _, plan := range otherPlans {
		if referencesWorkspace(plan.OutputPath) || referencesWorkspace(plan.DestinationRoot) || referencesWorkspace(plan.DestinationPath) {
			return true
		}
	}
	var activeTestEncodes []models.TestEncode
	if err := db.Where("status IN ? AND deleted_at IS NULL", []string{"queued", "running", "processing"}).Find(&activeTestEncodes).Error; err != nil {
		return true
	}
	for _, testEncode := range activeTestEncodes {
		for _, path := range []string{testEncode.OutputPath, testEncode.TemporaryPath} {
			if referencesWorkspace(path) {
				return true
			}
		}
		referenced := false
		walkHousekeepingJSONStrings(testEncode.SubtitleArtifacts, func(path string) {
			if referencesWorkspace(path) {
				referenced = true
			}
		})
		if referenced {
			return true
		}
	}
	var artifacts []models.TestEncodeArtifact
	if err := db.Find(&artifacts).Error; err != nil {
		return true
	}
	for _, artifact := range artifacts {
		if referencesWorkspace(artifact.Path) {
			return true
		}
	}
	var previews []models.CompatiblePreviewArtifactRecord
	if err := db.Where("state = ? OR (state = ? AND expires_at > ?)", compatiblePreviewArtifactGenerating, compatiblePreviewArtifactReady, time.Now().UTC()).Find(&previews).Error; err != nil {
		return true
	}
	for _, preview := range previews {
		if referencesWorkspace(preview.SourcePath) || referencesWorkspace(preview.CachePath) {
			return true
		}
	}
	var otherJobs []models.QueueJob
	if err := db.Where("id <> ?", jobID).Find(&otherJobs).Error; err != nil {
		return true
	}
	for _, other := range otherJobs {
		for _, path := range []string{other.MediaPath, other.OutputPath, other.PublishedPath, other.PlannedPublishedPath, other.ReplacementTargetPath, other.OriginalArchivedPath} {
			if referencesWorkspace(path) {
				return true
			}
		}
		referenced := false
		walkHousekeepingJSONStrings(other.SubtitleArtifacts, func(path string) {
			if referencesWorkspace(path) {
				referenced = true
			}
		})
		if referenced {
			return true
		}
	}
	return false
}

func executeValidationStagingCleanup(db *gorm.DB, jobID uint, previewID, actor string) (validationStagingCleanupResponse, error) {
	unlock := lockValidationStagingJob(jobID)
	defer unlock()
	var preview models.ValidationStagingCleanupPreview
	var started models.ValidationStagingCleanupEvidence
	var expiredResult *validationStagingCleanupResponse
	now := time.Now().UTC()
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("preview_id = ? AND job_id = ?", previewID, jobID).First(&preview).Error; err != nil {
			return err
		}
		if preview.ConsumedAt != nil {
			return errStaleHousekeepingPreview
		}
		var job models.QueueJob
		if err := tx.First(&job, jobID).Error; err != nil {
			return err
		}
		if job.DismissedAt == nil {
			return queueValidationDismissalConflict("staging cleanup requires a committed Dismiss")
		}
		if !preview.ExpiresAt.After(now) {
			update := tx.Model(&models.ValidationStagingCleanupPreview{}).Where("id = ? AND consumed_at IS NULL", preview.ID).Update("consumed_at", now)
			if update.Error != nil {
				return update.Error
			}
			if update.RowsAffected != 1 {
				return errStaleHousekeepingPreview
			}
			var captured validationStagingSnapshot
			_ = json.Unmarshal([]byte(preview.ManifestJSON), &captured)
			action := "dismiss_cleanup"
			if captured.DismissedAt != nil {
				action = "history_retry"
			}
			if actor == "" {
				actor = "local_operator"
			}
			evidenceID, idErr := newValidationStagingID("vsc-evidence-")
			if idErr != nil {
				return idErr
			}
			event := validationStagingEvidence(preview, evidenceID, "expired-"+preview.PreviewID, actor, action, "completed", "blocked", nil, 0, "cleanup preview expired before confirmation; review staged files with a fresh preview", now)
			if err := tx.Create(&event).Error; err != nil {
				return err
			}
			preview.ConsumedAt = &now
			expiredResult = &validationStagingCleanupResponse{JobID: jobID, PreviewID: previewID, Outcome: "blocked", RemovedPaths: []string{}, Reason: event.Reason, EvidenceID: event.EvidenceID}
			return nil
		}
		update := tx.Model(&models.ValidationStagingCleanupPreview{}).
			Where("id = ? AND consumed_at IS NULL AND expires_at > ?", preview.ID, now).
			Update("consumed_at", now)
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return errStaleHousekeepingPreview
		}
		preview.ConsumedAt = &now
		action := "dismiss_cleanup"
		var captured validationStagingSnapshot
		if json.Unmarshal([]byte(preview.ManifestJSON), &captured) == nil && captured.DismissedAt != nil {
			action = "history_retry"
		}
		if actor == "" {
			actor = "local_operator"
		}
		attemptID, err := newValidationStagingID("vsc-attempt-")
		if err != nil {
			return err
		}
		evidenceID, err := newValidationStagingID("vsc-evidence-")
		if err != nil {
			return err
		}
		started = validationStagingEvidence(preview, evidenceID, attemptID, actor, action, "started", "in_progress", nil, 0, "", now)
		return tx.Create(&started).Error
	})
	if errors.Is(err, errStaleHousekeepingPreview) {
		latest, latestErr := latestValidationStagingEvidence(db, jobID, previewID)
		if latestErr == nil && latest.ID != 0 {
			outcome, reason := latest.Outcome, latest.Reason
			if latest.Event == "started" {
				outcome, reason = "failed", "cleanup attempt was interrupted before a terminal outcome was recorded; review the remaining workspace with a fresh preview"
			}
			return validationStagingCleanupResponse{JobID: jobID, PreviewID: previewID, Outcome: outcome, RemovedPaths: decodeValidationStagingPaths(latest.RemovedPathsJSON), RecoveredBytes: latest.RecoveredBytes, Reason: reason, EvidenceID: latest.EvidenceID}, nil
		}
		return validationStagingCleanupResponse{}, err
	}
	if err != nil {
		return validationStagingCleanupResponse{}, err
	}
	if expiredResult != nil {
		return *expiredResult, nil
	}
	response := validationStagingCleanupResponse{JobID: jobID, PreviewID: previewID, Outcome: "blocked", RemovedPaths: []string{}, EvidenceID: started.EvidenceID}
	var snapshot validationStagingSnapshot
	if err := json.Unmarshal([]byte(preview.ManifestJSON), &snapshot); err != nil {
		response.Outcome, response.Reason = "failed", "stored preview manifest is unreadable"
		return finishValidationStagingAttempt(db, preview, started, response)
	}
	var job models.QueueJob
	if err := db.First(&job, jobID).Error; err != nil {
		response.Outcome, response.Reason = "blocked", "QueueJob disappeared after Dismiss"
		return finishValidationStagingAttempt(db, preview, started, response)
	}
	if err := validateExplicitDismissedStagingIdentity(db, jobID, explicitValidationStagingAuthorization{Preview: preview, Snapshot: snapshot}); err != nil {
		response.Outcome, response.Reason = "blocked", err.Error()
		return finishValidationStagingAttempt(db, preview, started, response)
	}
	roles, err := scheduler.LoadStorageRoles(db)
	if err != nil {
		response.Outcome, response.Reason = "blocked", "storage roles could not be reloaded"
		return finishValidationStagingAttempt(db, preview, started, response)
	}
	roots, err := housekeepingRoleRoots(roles)
	if err != nil || roots["work"] != preview.WorkRoot || filepath.Join(roots["work"], fmt.Sprintf("job-%d", job.ID)) != preview.WorkspacePath {
		response.Outcome, response.Reason = "blocked", "storage role or workspace path changed after preview"
		return finishValidationStagingAttempt(db, preview, started, response)
	}
	workspaceInfo, statErr := os.Lstat(preview.WorkspacePath)
	if preview.State == "noop" && errors.Is(statErr, os.ErrNotExist) {
		response.Outcome, response.Reason = "noop", "workspace remains absent; no staged bytes were recovered"
		return finishValidationStagingAttempt(db, preview, started, response)
	}
	if statErr != nil || !workspaceInfo.IsDir() || workspaceInfo.Mode()&os.ModeSymlink != 0 {
		response.Outcome, response.Reason = "blocked", "workspace is missing or no longer a real direct directory"
		return finishValidationStagingAttempt(db, preview, started, response)
	}
	files, tree, total, manifestErr := validationStagingManifest(preview.WorkspacePath)
	if manifestErr != nil || tree != preview.TreeIdentity || tree != snapshot.TreeIdentity || total != preview.TotalBytes || !reflect.DeepEqual(files, snapshot.Files) {
		response.Outcome, response.Reason = "blocked", "workspace paths, ownership, files, or byte total changed after preview"
		return finishValidationStagingAttempt(db, preview, started, response)
	}
	if explicitValidationStagingHasActiveReferences(db, jobID, preview.WorkspacePath) {
		response.Outcome, response.Reason = "blocked", "an active Worker, Queue, Publisher, maintenance, Test Encode, artifact, or other job reference appeared after preview"
		return finishValidationStagingAttempt(db, preview, started, response)
	}
	if preview.State == "noop" {
		response.Outcome, response.Reason = "blocked", "workspace appeared after the no-op preview; review a fresh file preview"
		return finishValidationStagingAttempt(db, preview, started, response)
	}
	candidate := HousekeepingCandidate{
		Kind: housekeepingKindWorkspace, Path: preview.WorkspacePath, JobID: jobID,
		Reason: "operator confirmed Validation Dismiss cleanup", Ownership: "direct work/job-<QueueJob.ID> workspace contract",
		SizeBytes: preview.TotalBytes, ModifiedAt: workspaceInfo.ModTime().UTC().Format(time.RFC3339Nano), SafeToDelete: true,
	}
	bound := housekeepingBoundCandidate{Candidate: candidate, Identity: preview.TreeIdentity}
	bound.Candidate.CandidateID = housekeepingCandidateID(candidate, bound.Identity)
	_, removed, executeErr := executeHousekeepingCandidateWithStaging(db, roots, HousekeepingPolicy{}, now, bound, &explicitValidationStagingAuthorization{Preview: preview, Snapshot: snapshot})
	if partial := new(validationStagingPartialRemovalError); errors.As(executeErr, &partial) {
		response.Outcome, response.Reason = "failed", partial.Error()
		response.RemovedPaths, response.RecoveredBytes = partial.paths, partial.bytes
	} else if executeErr != nil {
		response.Outcome, response.Reason = "failed", executeErr.Error()
	} else if !removed {
		response.Outcome, response.Reason = "blocked", "Housekeeping revalidation blocked workspace cleanup"
	} else {
		paths := make([]string, 0, len(snapshot.Files))
		for _, file := range snapshot.Files {
			paths = append(paths, filepath.Join(preview.WorkspacePath, file.Relative))
		}
		response.Outcome, response.RemovedPaths, response.RecoveredBytes = "cleaned", paths, preview.TotalBytes
	}
	return finishValidationStagingAttempt(db, preview, started, response)
}

func validateExplicitDismissedStagingIdentity(db *gorm.DB, jobID uint, authorization explicitValidationStagingAuthorization) error {
	preview := authorization.Preview
	var job models.QueueJob
	if err := db.First(&job, jobID).Error; err != nil {
		return errors.New("QueueJob identity is unavailable")
	}
	if job.ID != preview.JobID || job.Status != JobStatusCompleted || job.DismissedAt == nil || job.ExecutionNumber == nil || *job.ExecutionNumber != preview.ExecutionNumber || job.ValidationRunID != preview.ValidationRunID || job.OutputPath != authorization.Snapshot.OutputPath || job.PublishedAt != nil || job.PublishedPath != "" || job.PublicationRetiredAt != nil {
		return errors.New("QueueJob lifecycle, execution, Validation, Result path, or publication identity changed after preview")
	}
	if authorization.Snapshot.DismissedAt != nil && !job.DismissedAt.Equal(*authorization.Snapshot.DismissedAt) {
		return errors.New("Dismiss identity changed after preview")
	}
	if err := validateCompletedValidationDismissal(db, job); err != nil {
		return err
	}
	artifact, _, err := readLatestJobArtifact(db, job, "result")
	if err != nil {
		return errors.New("current Result artifact is unavailable")
	}
	identity, identityOK := resultIdentityFromArtifact(artifact)
	binding, bindingOK := resultBindingFromArtifact(artifact)
	if !identityOK || !bindingOK || binding.ExecutionNumber != preview.ExecutionNumber || binding.ExecutionPlanID != preview.ExecutionPlanID || binding.ExecutionPlanVersion != preview.ExecutionPlanVersion || binding.ExecutionPlanFingerprint != preview.ExecutionPlanFingerprint || identity.ArtifactID != preview.ResultArtifactID || identity.GenerationToken != preview.ResultGenerationToken || identity.GenerationVersion != preview.ResultGenerationVersion || identity.OutputRevision != preview.ResultOutputRevision || !frozenBindingMatches(db, job, binding) {
		return errors.New("frozen Result or ExecutionPlan identity changed after preview")
	}
	return nil
}

func removeValidationStagingManifest(workRoot, workspace string, manifest []validationStagingFile) ([]string, int64, error) {
	workRootInfo, err := os.Lstat(workRoot)
	if err != nil || !workRootInfo.IsDir() || workRootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, 0, errors.New("Work storage root changed or became unsafe")
	}
	relativeWorkspace, err := filepath.Rel(workRoot, workspace)
	if err != nil || filepath.IsAbs(relativeWorkspace) || relativeWorkspace != filepath.Base(relativeWorkspace) || !strings.HasPrefix(relativeWorkspace, "job-") {
		return nil, 0, errors.New("workspace is not a direct canonical child of Work storage")
	}
	work, err := os.OpenRoot(workRoot)
	if err != nil {
		return nil, 0, err
	}
	defer work.Close()
	rootInfo, err := work.Lstat(relativeWorkspace)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, 0, errors.New("workspace changed before rooted cleanup")
	}
	workspaceRoot, err := work.OpenRoot(relativeWorkspace)
	if err != nil {
		return nil, 0, err
	}
	defer workspaceRoot.Close()
	openedInfo, err := workspaceRoot.Stat(".")
	if err != nil || statIdentity(openedInfo, true) != statIdentity(rootInfo, true) {
		return nil, 0, errors.New("workspace directory identity changed while opening rooted cleanup")
	}
	directoryIdentity := map[string]string{".": statIdentity(openedInfo, true)}
	for _, entry := range manifest {
		if entry.Kind == "directory" {
			directoryIdentity[filepath.Clean(entry.Relative)] = entry.Identity
		}
	}
	ordered := append([]validationStagingFile(nil), manifest...)
	sort.Slice(ordered, func(i, j int) bool {
		leftDepth, rightDepth := pathDepth(ordered[i].Relative), pathDepth(ordered[j].Relative)
		if leftDepth != rightDepth {
			return leftDepth > rightDepth
		}
		if ordered[i].Kind != ordered[j].Kind {
			return ordered[i].Kind == "file"
		}
		return ordered[i].Relative > ordered[j].Relative
	})
	removed := make([]string, 0, len(ordered))
	var recovered int64
	for _, entry := range ordered {
		if entry.Relative == "." {
			continue
		}
		parentPath := filepath.Dir(entry.Relative)
		parentIdentity, parentKnown := directoryIdentity[parentPath]
		if !parentKnown {
			return removed, recovered, fmt.Errorf("workspace parent was not present in preview manifest: %s", entry.Relative)
		}
		parentRoot := workspaceRoot
		closeParentRoot := func() {}
		if parentPath != "." {
			parentRoot, err = workspaceRoot.OpenRoot(parentPath)
			if err != nil {
				return removed, recovered, fmt.Errorf("open previewed workspace parent %s: %w", parentPath, err)
			}
			closeParentRoot = func() { _ = parentRoot.Close() }
		}
		parentInfo, parentErr := parentRoot.Stat(".")
		if parentErr != nil || statIdentity(parentInfo, true) != parentIdentity {
			closeParentRoot()
			return removed, recovered, fmt.Errorf("workspace parent changed before removal: %s", parentPath)
		}
		baseName := filepath.Base(entry.Relative)
		info, err := parentRoot.Lstat(baseName)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || (entry.Kind == "directory") != info.IsDir() || (!info.IsDir() && !info.Mode().IsRegular()) || statIdentity(info, info.IsDir()) != entry.Identity {
			closeParentRoot()
			return removed, recovered, fmt.Errorf("workspace entry changed before removal: %s", entry.Relative)
		}
		if err := parentRoot.Remove(baseName); err != nil {
			closeParentRoot()
			return removed, recovered, fmt.Errorf("remove previewed workspace entry %s: %w", entry.Relative, err)
		}
		closeParentRoot()
		removed = append(removed, filepath.Join(workspace, entry.Relative))
		if entry.Kind == "file" {
			recovered += entry.SizeBytes
		}
	}
	rootIdentity, err := work.Lstat(relativeWorkspace)
	if err != nil || statIdentity(rootIdentity, true) != statIdentity(rootInfo, true) {
		return removed, recovered, errors.New("workspace directory changed before final removal")
	}
	if err := work.Remove(relativeWorkspace); err != nil {
		return removed, recovered, fmt.Errorf("remove empty previewed workspace: %w", err)
	}
	removed = append(removed, workspace)
	return removed, recovered, nil
}

func validationStagingEvidence(preview models.ValidationStagingCleanupPreview, evidenceID, attemptID, actor, action, event, outcome string, paths []string, recovered int64, reason string, createdAt time.Time) models.ValidationStagingCleanupEvidence {
	encodedPaths, _ := json.Marshal(paths)
	return models.ValidationStagingCleanupEvidence{
		EvidenceID: evidenceID, AttemptID: attemptID, JobID: preview.JobID,
		ExecutionNumber: preview.ExecutionNumber, ExecutionPlanID: preview.ExecutionPlanID,
		ExecutionPlanVersion: preview.ExecutionPlanVersion, ExecutionPlanFingerprint: preview.ExecutionPlanFingerprint,
		ResultArtifactID: preview.ResultArtifactID, ResultGenerationToken: preview.ResultGenerationToken,
		ResultGenerationVersion: preview.ResultGenerationVersion, ResultOutputRevision: preview.ResultOutputRevision,
		ValidationRunID: preview.ValidationRunID, PreviewID: preview.PreviewID, TreeIdentity: preview.TreeIdentity,
		Actor: actor, Action: action, Event: event, Outcome: outcome, WorkspacePath: preview.WorkspacePath,
		RemovedPathsJSON: string(encodedPaths), RecoveredBytes: recovered, Reason: reason, CreatedAt: createdAt,
	}
}

func finishValidationStagingAttempt(db *gorm.DB, preview models.ValidationStagingCleanupPreview, started models.ValidationStagingCleanupEvidence, response validationStagingCleanupResponse) (validationStagingCleanupResponse, error) {
	response.RemovedPaths = nonNilStrings(response.RemovedPaths)
	terminalID, err := newValidationStagingID("vsc-evidence-")
	if err != nil {
		return response, err
	}
	terminal := validationStagingEvidence(preview, terminalID, started.AttemptID, started.Actor, started.Action, "completed", response.Outcome, response.RemovedPaths, response.RecoveredBytes, response.Reason, time.Now().UTC())
	if err := db.Create(&terminal).Error; err != nil {
		return response, err
	}
	response.EvidenceID = terminal.EvidenceID
	return response, nil
}

func latestValidationStagingEvidence(db *gorm.DB, jobID uint, previewID string) (models.ValidationStagingCleanupEvidence, error) {
	var evidence models.ValidationStagingCleanupEvidence
	err := db.Where("job_id = ? AND preview_id = ?", jobID, previewID).Order("id desc").First(&evidence).Error
	return evidence, err
}

func decodeValidationStagingPaths(value string) []string {
	var paths []string
	if json.Unmarshal([]byte(value), &paths) != nil {
		return []string{}
	}
	return nonNilStrings(paths)
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
