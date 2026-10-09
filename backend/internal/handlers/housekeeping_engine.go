package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"gorm.io/gorm"
)

func PreviewHousekeeping(db *gorm.DB) (HousekeepingReport, error) {
	policy, err := loadHousekeepingPolicy(db)
	if err != nil {
		return HousekeepingReport{}, err
	}
	roles, err := scheduler.LoadStorageRoles(db)
	if err != nil {
		return HousekeepingReport{}, err
	}
	roots, err := housekeepingRoleRoots(roles)
	if err != nil {
		return HousekeepingReport{}, err
	}
	now := time.Now().UTC()
	state, err := buildHousekeepingPreviewState(db, policy, roots, now)
	if err != nil {
		return HousekeepingReport{}, err
	}
	if err := saveHousekeepingPreview(db, state); err != nil {
		return HousekeepingReport{}, err
	}
	report := housekeepingReportForState(state, true)
	if err := persistHousekeepingReport(db, report); err != nil {
		return report, err
	}
	return report, nil
}

func ExecuteHousekeepingPreview(db *gorm.DB, previewID string) (HousekeepingReport, error) {
	previewID = strings.TrimSpace(previewID)
	if previewID == "" {
		return HousekeepingReport{}, errStaleHousekeepingPreview
	}
	policy, err := loadHousekeepingPolicy(db)
	if err != nil {
		return HousekeepingReport{}, err
	}
	roles, err := scheduler.LoadStorageRoles(db)
	if err != nil {
		return HousekeepingReport{}, err
	}
	roots, err := housekeepingRoleRoots(roles)
	if err != nil {
		return HousekeepingReport{}, err
	}
	state, err := consumeHousekeepingPreview(db, previewID, policy, roots, time.Now().UTC())
	if err != nil {
		return HousekeepingReport{}, err
	}
	report := housekeepingReportForState(state, false)
	report.RanAt = time.Now().UTC()
	applyHousekeepingPlan(db, roots, policy, &state, &report)
	if err := persistHousekeepingReport(db, report); err != nil {
		return report, err
	}
	return report, nil
}

func runAutomaticHousekeeping(db *gorm.DB) (HousekeepingReport, error) {
	report := HousekeepingReport{RanAt: time.Now().UTC(), Candidates: []HousekeepingCandidate{}, Summaries: []HousekeepingCategorySummary{}, RemovedPaths: []string{}, Errors: []string{}, RuntimeSnapshots: RuntimeSnapshotRetentionReport{Errors: []string{}}}
	policy, err := loadHousekeepingPolicy(db)
	if err != nil {
		return report, err
	}
	roles, err := scheduler.LoadStorageRoles(db)
	if err != nil {
		return report, err
	}
	roots, err := housekeepingRoleRoots(roles)
	if err != nil {
		return report, err
	}
	state, err := buildHousekeepingPreviewState(db, policy, roots, report.RanAt)
	if err != nil {
		return report, err
	}
	report = housekeepingReportForState(state, false)
	applyHousekeepingPlan(db, roots, policy, &state, &report)
	if err := persistHousekeepingReport(db, report); err != nil {
		return report, err
	}
	return report, nil
}

func buildHousekeepingPreviewState(db *gorm.DB, policy HousekeepingPolicy, roots map[string]string, now time.Time) (housekeepingPreviewState, error) {
	previewID, err := newHousekeepingPreviewID()
	if err != nil {
		return housekeepingPreviewState{}, err
	}
	state := housekeepingPreviewState{
		PreviewID: previewID, CreatedAt: now, ExpiresAt: now.Add(housekeepingPreviewTTL), Policy: policy,
		RoleRoots: roots, Candidates: []housekeepingBoundCandidate{},
		RuntimeSnapshots: RuntimeSnapshotRetentionReport{Errors: []string{}}, Errors: []string{},
	}
	if err := addWorkspaceCandidates(db, roots, policy, now, &state); err != nil {
		state.Errors = append(state.Errors, err.Error())
	}
	if err := addJobLogCandidates(db, roots, policy, now, &state); err != nil {
		state.Errors = append(state.Errors, err.Error())
	}
	if policy.OrphanStageArtifactsEnabled {
		if err := addStageArtifactCandidates(db, roots, policy, now, &state); err != nil {
			state.Errors = append(state.Errors, err.Error())
		}
	} else {
		state.Errors = append(state.Errors, "orphan stage artifact cleanup is disabled by policy")
	}
	if err := addTestEncodeCandidates(db, now, &state); err != nil {
		state.Errors = append(state.Errors, err.Error())
	}
	if err := addRuntimeSnapshotCandidates(db, now, &state); err != nil {
		state.Errors = append(state.Errors, err.Error())
	}
	unsafeRaw := unsafeRawRoleRoot(roots)
	if unsafeRaw {
		state.Errors = append(state.Errors, "configured RAW storage role overlaps another storage role; empty RAW directory cleanup is blocked")
	}
	if err := addEmptyDirectoryCandidates(db, roots, unsafeRaw, policy.EmptyRawDirectoriesEnabled, &state); err != nil {
		state.Errors = append(state.Errors, err.Error())
	}
	sort.SliceStable(state.Candidates, func(i, j int) bool {
		left, right := state.Candidates[i].Candidate, state.Candidates[j].Candidate
		if housekeepingCandidateOrder(left.Kind) != housekeepingCandidateOrder(right.Kind) {
			return housekeepingCandidateOrder(left.Kind) < housekeepingCandidateOrder(right.Kind)
		}
		if left.Kind == housekeepingKindEmptyDirectory && right.Kind == housekeepingKindEmptyDirectory {
			leftDepth, rightDepth := pathDepth(left.Path), pathDepth(right.Path)
			if leftDepth != rightDepth {
				return leftDepth > rightDepth
			}
		}
		return left.Path < right.Path
	})
	return state, nil
}

func addWorkspaceCandidates(db *gorm.DB, roots map[string]string, policy HousekeepingPolicy, now time.Time, state *housekeepingPreviewState) error {
	root := roots[string(scheduler.StorageRoleWork)]
	entries, err := readRoleRoot(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read work storage role: %w", err)
	}
	unsafeRoleRoot := unsafeOperationalRoleRoot(roots, "work")
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "job-") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		info, statErr := os.Lstat(path)
		candidate := HousekeepingCandidate{Kind: housekeepingKindWorkspace, Path: path, Ownership: "direct work/job-N workspace contract", Reason: "job workspace retention", ModifiedAt: now.Format(time.RFC3339Nano)}
		if unsafeRoleRoot {
			candidate.BlockedReason = "configured work storage role overlaps RAW, Library, or originals archive"
		}
		if statErr != nil {
			candidate.BlockedReason = "workspace could not be inspected"
			candidate.Reason = "workspace ownership could not be verified"
			addBoundCandidate(state, candidate, "unavailable:"+statErr.Error())
			continue
		}
		candidate.ModifiedAt = info.ModTime().UTC().Format(time.RFC3339Nano)
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			candidate.Reason = "job workspace path is not a direct regular directory"
			candidate.BlockedReason = "symlink or non-directory workspace entries are never followed or removed"
			addBoundCandidate(state, candidate, statIdentity(info, false))
			continue
		}
		suffix := strings.TrimPrefix(entry.Name(), "job-")
		id, parseErr := strconv.ParseUint(suffix, 10, 64)
		if parseErr != nil || id == 0 || strconv.FormatUint(id, 10) != suffix {
			candidate.Reason = "ambiguous work directory"
			candidate.BlockedReason = "workspace name does not identify a canonical QueueJob ID"
			addBoundCandidate(state, candidate, statIdentity(info, true))
			continue
		}
		candidate.JobID = uint(id)
		classified, _, classifyErr := housekeepingCandidate(db, uint(id), path, info.ModTime(), policy, now)
		if classifyErr != nil {
			candidate.Reason = "workspace lifecycle could not be read"
			candidate.BlockedReason = classifyErr.Error()
		} else {
			candidate = classified
		}
		identity, identityErr := directoryTreeIdentity(path)
		if identityErr != nil {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "workspace contains a symlink, special file, or unreadable entry"
			identity = statIdentity(info, true)
		} else {
			candidate.SizeBytes, _ = directorySizeNoSymlinks(path)
			if unsafeRoleRoot {
				candidate.SafeToDelete = false
				candidate.BlockedReason = "configured work storage role overlaps RAW, Library, or originals archive"
			}
		}
		addBoundCandidate(state, candidate, identity)
	}
	return nil
}

func addJobLogCandidates(db *gorm.DB, roots map[string]string, policy HousekeepingPolicy, now time.Time, state *housekeepingPreviewState) error {
	root := roots[string(scheduler.StorageRoleLogs)]
	entries, err := readRoleRoot(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read logs storage role: %w", err)
	}
	unsafeRoleRoot := unsafeOperationalRoleRoot(roots, "logs")
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "job-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		suffix := strings.TrimSuffix(strings.TrimPrefix(name, "job-"), ".log")
		id, parseErr := strconv.ParseUint(suffix, 10, 64)
		if parseErr != nil || id == 0 || strconv.FormatUint(id, 10) != suffix {
			continue
		}
		path := filepath.Join(root, name)
		info, statErr := os.Lstat(path)
		candidate := HousekeepingCandidate{Kind: housekeepingKindJobLog, Path: path, JobID: uint(id), Ownership: "canonical logs/job-<queueJobId>.log name", Reason: "job log retention", ModifiedAt: now.Format(time.RFC3339Nano)}
		if statErr != nil {
			candidate.BlockedReason = "job log could not be inspected"
			candidate.Reason = "job log ownership could not be verified"
			addBoundCandidate(state, candidate, "unavailable:"+statErr.Error())
			continue
		}
		candidate.ModifiedAt = info.ModTime().UTC().Format(time.RFC3339Nano)
		identity := statIdentity(info, false)
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			candidate.BlockedReason = "job logs must be regular files without symlink components"
			candidate.Reason = "job log path is unsafe"
			addBoundCandidate(state, candidate, identity)
			continue
		}
		candidate.SizeBytes = info.Size()
		if unsafeRoleRoot {
			candidate.BlockedReason = "configured logs storage role overlaps RAW, Library, or originals archive"
		}
		var job models.QueueJob
		result := db.Where("id = ?", uint(id)).Limit(1).Find(&job)
		if result.Error != nil {
			candidate.BlockedReason = result.Error.Error()
		} else if result.RowsAffected == 0 {
			candidate.JobID = 0
			candidate.Ownership = "canonical job log filename; missing QueueJob row"
			candidate.Reason = "orphan job log retention expired"
			candidate.SafeToDelete = now.After(info.ModTime().Add(time.Duration(max(policy.OrphanJobLogRetentionDays, 0)) * 24 * time.Hour))
			if !candidate.SafeToDelete {
				candidate.BlockedReason = "orphan log retention period has not elapsed"
			}
		} else {
			candidate = classifyJobLog(job, path, info, policy, now)
		}
		if unsafeRoleRoot {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "configured logs storage role overlaps RAW, Library, or originals archive"
		}
		if candidate.SafeToDelete && jobHasActiveHousekeepingReferences(db, uint(id), "") {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "active worker, reservation, or operation still owns this job"
		}
		addBoundCandidate(state, candidate, identity)
	}
	return nil
}

func classifyJobLog(job models.QueueJob, path string, info os.FileInfo, policy HousekeepingPolicy, now time.Time) HousekeepingCandidate {
	candidate := HousekeepingCandidate{Kind: housekeepingKindJobLog, Path: path, JobID: job.ID, Ownership: "queue_job lifecycle plus canonical job log name", ModifiedAt: info.ModTime().UTC().Format(time.RFC3339Nano), SizeBytes: info.Size()}
	if isProtectedHousekeepingJob(&job) {
		candidate.Reason = "job log retained for active, unpublished, or unresolved review lifecycle"
		candidate.BlockedReason = "job is queued, active, completed but unpublished, or has unresolved review/block state"
		return candidate
	}
	var retentionDays int
	var reference time.Time
	switch {
	case job.Status == JobStatusFailed:
		candidate.Reason, retentionDays = "failed job log retention expired", policy.FailedJobLogRetentionDays
	case job.Status == JobStatusCanceled:
		candidate.Reason, retentionDays = "canceled job log retention expired", policy.CanceledJobLogRetentionDays
	case job.Status == JobStatusCompleted && job.PublishedAt != nil:
		candidate.Reason, retentionDays = "published job log retention expired", policy.PublishedJobLogRetentionDays
		if job.PublishedAt != nil {
			reference = *job.PublishedAt
		}
	default:
		candidate.Reason = "job log lifecycle is not terminal"
		candidate.BlockedReason = "only failed, canceled, or published jobs have expiring logs"
		return candidate
	}
	if reference.IsZero() {
		reference = job.UpdatedAt
		if job.FinishedAt != nil {
			reference = *job.FinishedAt
		}
	}
	if now.Before(reference.Add(time.Duration(max(retentionDays, 0)) * 24 * time.Hour)) {
		candidate.BlockedReason = "job log retention period has not elapsed"
		return candidate
	}
	if jobHasActiveHousekeepingReferencesFromJob(&job) {
		candidate.BlockedReason = "worker or operation still owns this job"
		return candidate
	}
	candidate.SafeToDelete = true
	return candidate
}

func addStageArtifactCandidates(db *gorm.DB, roots map[string]string, policy HousekeepingPolicy, now time.Time, state *housekeepingPreviewState) error {
	if db.Migrator().HasTable(&models.AssetMaintenanceOperation{}) {
		var operations []models.AssetMaintenanceOperation
		if err := db.Where("TRIM(temporary_path) <> ''").Order("updated_at ASC").Order("id ASC").Limit(housekeepingMaxCandidates).Find(&operations).Error; err != nil {
			return fmt.Errorf("load owned maintenance artifacts: %w", err)
		}
		for _, operation := range operations {
			candidate, identity := maintenanceArtifactCandidate(db, roots, operation, policy, now)
			if coveredBySafeWorkspace(state.Candidates, candidate.Path) {
				continue
			}
			addBoundCandidate(state, candidate, identity)
		}
	}
	if db.Migrator().HasTable(&models.PublisherPublication{}) {
		var publications []models.PublisherPublication
		if err := db.Where("TRIM(original_archive_temporary_path) <> ''").Order("updated_at ASC").Order("publication_id ASC").Limit(housekeepingMaxCandidates).Find(&publications).Error; err != nil {
			return fmt.Errorf("load owned publisher staging artifacts: %w", err)
		}
		for _, publication := range publications {
			candidate, identity := publisherStageArtifactCandidate(db, roots, publication, policy, now)
			if coveredBySafeWorkspace(state.Candidates, candidate.Path) {
				continue
			}
			addBoundCandidate(state, candidate, identity)
		}
	}
	return nil
}

func maintenanceArtifactCandidate(db *gorm.DB, roots map[string]string, operation models.AssetMaintenanceOperation, policy HousekeepingPolicy, now time.Time) (HousekeepingCandidate, string) {
	candidate := HousekeepingCandidate{Kind: housekeepingKindStageArtifact, OperationID: operation.ID, Path: operation.TemporaryPath, Ownership: "asset_maintenance_operation.temporary_path", Reason: "abandoned maintenance temporary artifact", ModifiedAt: operation.UpdatedAt.UTC().Format(time.RFC3339Nano)}
	if operation.FinishedAt != nil {
		candidate.ModifiedAt = operation.FinishedAt.UTC().Format(time.RFC3339Nano)
	}
	identity := "unresolved"
	if operation.TemporaryPath == "" {
		candidate.BlockedReason = "operation has no persisted temporary path"
		return candidate, identity
	}
	if operationIsActive(operation.Status) {
		candidate.BlockedReason = "owning operation is still active"
		return candidate, identity
	}
	if !operationIsAbandoned(operation.Status, operation.Phase) {
		candidate.BlockedReason = "operation status does not prove an abandoned temporary artifact"
		return candidate, identity
	}
	retentionReference := operation.UpdatedAt
	if operation.FinishedAt != nil {
		retentionReference = *operation.FinishedAt
	}
	if now.Before(retentionReference.Add(time.Duration(max(policy.OrphanRetentionDays, 0)) * 24 * time.Hour)) {
		candidate.BlockedReason = "abandoned artifact retention period has not elapsed"
	}
	role, path, info, pathErr := resolveOwnedFilePath(roots, operation.TemporaryPath, []string{"work", "cache", "reports", "logs"})
	if pathErr != nil {
		candidate.BlockedReason = pathErr.Error()
		return candidate, identity
	}
	candidate.Path, candidate.Ownership = path, "asset_maintenance_operation.temporary_path under "+role+" storage role"
	if info == nil {
		candidate.SafeToDelete = candidate.BlockedReason == ""
		return candidate, "missing"
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		candidate.BlockedReason = "owned stage artifacts must be regular non-symlink files"
		return candidate, statIdentity(info, false)
	}
	candidate.SizeBytes = info.Size()
	identity = statIdentity(info, false)
	if reason := operationArtifactReferenceBlock(db, operation.ID, path); reason != "" {
		candidate.BlockedReason = reason
		return candidate, identity
	}
	if candidate.BlockedReason == "" {
		candidate.SafeToDelete = true
	}
	return candidate, identity
}

func publisherStageArtifactCandidate(db *gorm.DB, roots map[string]string, publication models.PublisherPublication, policy HousekeepingPolicy, now time.Time) (HousekeepingCandidate, string) {
	candidate := HousekeepingCandidate{Kind: housekeepingKindStageArtifact, OperationID: publication.PublicationID, JobID: publication.QueueJobID, Path: publication.OriginalArchiveTemporaryPath, Ownership: "publisher_publication.original_archive_temporary_path", Reason: "abandoned Publisher staging artifact", ModifiedAt: publication.UpdatedAt.UTC().Format(time.RFC3339Nano)}
	identity := "unresolved"
	if publication.OriginalArchiveTemporaryPath == "" {
		candidate.BlockedReason = "publication has no persisted staging path"
		return candidate, identity
	}
	if !publisherArchiveStageIsAbandoned(publication) {
		candidate.BlockedReason = "Publisher archive lifecycle may still require recovery"
		return candidate, identity
	}
	if publication.OriginalArchiveLeaseExpiresAt != nil && publication.OriginalArchiveLeaseExpiresAt.After(now) {
		candidate.BlockedReason = "Publisher archive lease is active"
		return candidate, identity
	}
	if now.Before(publication.UpdatedAt.Add(time.Duration(max(policy.OrphanRetentionDays, 0)) * 24 * time.Hour)) {
		candidate.BlockedReason = "abandoned artifact retention period has not elapsed"
	}
	role, path, info, pathErr := resolveOwnedFilePath(roots, publication.OriginalArchiveTemporaryPath, []string{"originals_archive"})
	if pathErr != nil {
		candidate.BlockedReason = pathErr.Error()
		return candidate, identity
	}
	candidate.Path, candidate.Ownership = path, "publisher_publication temporary path under "+role+" storage role"
	for _, protected := range []string{publication.OriginalArchivedPath, publication.OriginalArchiveSourcePath, publication.SourcePath, publication.DestinationPath} {
		if protected != "" && path == filepath.Clean(protected) {
			candidate.BlockedReason = "staging path aliases a canonical publication or archived original"
			return candidate, identity
		}
	}
	if info == nil {
		candidate.SafeToDelete = candidate.BlockedReason == ""
		return candidate, "missing"
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		candidate.BlockedReason = "publisher staging artifacts must be regular non-symlink files"
		return candidate, statIdentity(info, false)
	}
	candidate.SizeBytes = info.Size()
	identity = statIdentity(info, false)
	if reason := publisherArtifactReferenceBlock(db, publication.PublicationID, path); reason != "" {
		candidate.BlockedReason = reason
		return candidate, identity
	}
	if candidate.BlockedReason == "" {
		candidate.SafeToDelete = true
	}
	return candidate, identity
}

func addTestEncodeCandidates(db *gorm.DB, now time.Time, state *housekeepingPreviewState) error {
	if !db.Migrator().HasTable(&models.TestEncode{}) {
		return nil
	}
	var tests []models.TestEncode
	if err := db.Where("deleted_at IS NULL AND keep = ? AND expires_at IS NOT NULL AND expires_at <= ? AND status NOT IN ?", false, now, []string{testEncodeDeleted, "expired"}).Order("expires_at ASC").Order("id ASC").Limit(100).Find(&tests).Error; err != nil {
		return err
	}
	for _, test := range tests {
		candidate := HousekeepingCandidate{Kind: housekeepingKindTestEncode, Path: test.OutputPath, TestEncodeID: test.ID, Ownership: "test_encode_artifact persisted owner", Reason: "Test Encode retention expired", ModifiedAt: test.UpdatedAt.UTC().Format(time.RFC3339Nano)}
		diskUsage, usageErr := (AssetHandler{db: db}).testEncodeOwnedArtifactDiskUsage(test)
		identity, identityErr := testEncodeCandidateIdentity(db, test)
		if usageErr == nil && identityErr != nil {
			usageErr = identityErr
		}
		if usageErr != nil {
			candidate.BlockedReason = usageErr.Error()
		} else {
			candidate.SizeBytes = diskUsage
			candidate.SafeToDelete = test.Status != testEncodeWaiting && test.Status != testEncodeRunning && !testEncodeOperationActive(test.ID)
			if !candidate.SafeToDelete {
				candidate.BlockedReason = "Test Encode is active or has an in-process owner"
			}
		}
		addBoundCandidate(state, candidate, identity)
	}
	return nil
}

func addRuntimeSnapshotCandidates(db *gorm.DB, now time.Time, state *housekeepingPreviewState) error {
	if !db.Migrator().HasTable(&models.RuntimeSnapshot{}) {
		return nil
	}
	plan, err := resolveRuntimeSnapshotRetention(db, now, NewRuntimeSnapshotReferenceRegistry())
	if err != nil {
		return err
	}
	state.RuntimeSnapshots = plan.RuntimeSnapshotRetentionReport
	for _, snapshot := range plan.candidates {
		path := fmt.Sprintf("runtime-snapshot/%d", snapshot.ID)
		candidate := HousekeepingCandidate{Kind: housekeepingKindRuntimeSnapshot, Path: path, OperationID: strconv.FormatUint(uint64(snapshot.ID), 10), Ownership: "runtime_snapshot_retention registry", Reason: "old unreferenced runtime snapshot", ModifiedAt: snapshot.DetectedAt.UTC().Format(time.RFC3339Nano), SafeToDelete: true}
		addBoundCandidate(state, candidate, fmt.Sprintf("runtime:%d:%d", snapshot.ID, snapshot.DetectedAt.UnixNano()))
	}
	return nil
}

func addEmptyDirectoryCandidates(db *gorm.DB, roots map[string]string, unsafeRaw, rawEnabled bool, state *housekeepingPreviewState) error {
	removedFiles := map[string]struct{}{}
	removedWorkspaces := []string{}
	protectedWorkspaces := []string{}
	for _, bound := range state.Candidates {
		candidate := bound.Candidate
		switch candidate.Kind {
		case housekeepingKindWorkspace:
			if candidate.SafeToDelete {
				removedWorkspaces = append(removedWorkspaces, candidate.Path)
			} else {
				protectedWorkspaces = append(protectedWorkspaces, candidate.Path)
			}
		case housekeepingKindJobLog, housekeepingKindStageArtifact:
			if candidate.SafeToDelete {
				removedFiles[candidate.Path] = struct{}{}
			}
		}
	}
	sharedPlannedDirectories := map[string]struct{}{}
	seenDirectories := map[string]struct{}{}
	if rawEnabled {
		protectedRaw, err := protectedRawDirectories(db, roots[string(scheduler.StorageRoleRaw)])
		if err != nil {
			return err
		}
		rawRoot := roots[string(scheduler.StorageRoleRaw)]
		if err := addEmptyDirectoryRoleCandidates(rawRoot, "raw", roots, true, unsafeRaw, nil, nil, protectedRaw, sharedPlannedDirectories, seenDirectories, state); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	for _, role := range []string{"logs", "reports", "cache", "work"} {
		root := roots[role]
		if root == "" {
			continue
		}
		protected, err := protectedOperationalDirectories(db, root, role == "work")
		if err != nil {
			return err
		}
		if role == "work" {
			for _, path := range protectedWorkspaces {
				protectHousekeepingPathAndParents(protected, root, path)
			}
		}
		for _, roleRoot := range roots {
			if pathWithin(root, roleRoot) {
				protectHousekeepingPathAndParents(protected, root, roleRoot)
			}
		}
		if err := addEmptyDirectoryRoleCandidates(root, role, roots, false, unsafeOperationalRoleRoot(roots, role), removedFiles, removedWorkspaces, protected, sharedPlannedDirectories, seenDirectories, state); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func addEmptyDirectoryRoleCandidates(root, role string, allRoots map[string]string, raw, unsafeRoot bool, removedFiles map[string]struct{}, removedWorkspaces []string, protected, plannedDirectories, seenDirectories map[string]struct{}, state *housekeepingPreviewState) error {
	if root == "" {
		return nil
	}
	directories := []string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() && filepath.Clean(path) != filepath.Clean(root) {
			directories = append(directories, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(directories, func(i, j int) bool {
		leftDepth, rightDepth := pathDepth(directories[i]), pathDepth(directories[j])
		if leftDepth != rightDepth {
			return leftDepth > rightDepth
		}
		return directories[i] < directories[j]
	})
	for _, path := range directories {
		if _, exists := seenDirectories[filepath.Clean(path)]; exists {
			continue
		}
		if isConfiguredRoleRoot(allRoots, path) {
			seenDirectories[filepath.Clean(path)] = struct{}{}
			continue
		}
		insideWorkspace := false
		for _, workspace := range removedWorkspaces {
			if pathWithin(workspace, path) {
				insideWorkspace = true
				break
			}
		}
		if insideWorkspace {
			continue
		}
		info, statErr := os.Lstat(path)
		if statErr != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			continue
		}
		emptyAfterPlan, currentEmpty, emptyErr := directoryWillBeEmpty(path, removedFiles, plannedDirectories)
		if emptyErr != nil || !emptyAfterPlan {
			continue
		}
		candidate := HousekeepingCandidate{
			Kind: housekeepingKindEmptyDirectory, Path: path, OperationID: role,
			Ownership: "empty_directory under " + role + " storage role",
			Reason:    "directory is literally empty", ModifiedAt: info.ModTime().UTC().Format(time.RFC3339Nano),
		}
		if !currentEmpty {
			candidate.Reason = "directory will be empty after previewed owned-file cleanup"
		}
		if _, isProtected := protected[filepath.Clean(path)]; isProtected {
			candidate.BlockedReason = "directory is referenced by a current asset, Queue source/restore target, or active operation"
		} else {
			candidate.SafeToDelete = true
			plannedDirectories[filepath.Clean(path)] = struct{}{}
		}
		if raw && !currentEmpty && !emptyAfterPlan {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "RAW directories are removable only when literally empty at preview and execution time"
		} else if raw && !currentEmpty && candidate.SafeToDelete {
			candidate.Reason = "directory will be empty after previewed empty child directories are removed"
		}
		if unsafeRoot {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "configured RAW storage role overlaps another storage role"
		}
		addBoundCandidate(state, candidate, statIdentity(info, true))
		seenDirectories[filepath.Clean(path)] = struct{}{}
	}
	return nil
}

func directoryWillBeEmpty(path string, removedFiles, removedDirectories map[string]struct{}) (emptyAfterPlan, currentEmpty bool, err error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, false, err
	}
	if len(entries) == 0 {
		return true, true, nil
	}
	for _, entry := range entries {
		child := filepath.Join(path, entry.Name())
		if _, ok := removedFiles[filepath.Clean(child)]; ok {
			continue
		}
		if _, ok := removedDirectories[filepath.Clean(child)]; ok {
			continue
		}
		return false, false, nil
	}
	return true, false, nil
}

func protectedRawDirectories(db *gorm.DB, root string) (map[string]struct{}, error) {
	protected := map[string]struct{}{}
	if root == "" {
		return protected, nil
	}
	root = canonicalHousekeepingPath(root)
	add := func(path string, directory bool) {
		if strings.TrimSpace(path) == "" {
			return
		}
		path = canonicalHousekeepingPath(path)
		if path == "" {
			return
		}
		path = filepath.Clean(path)
		if !filepath.IsAbs(path) {
			return
		}
		if !directory {
			path = filepath.Dir(path)
		}
		for pathWithin(root, path) {
			protected[filepath.Clean(path)] = struct{}{}
			if filepath.Clean(path) == filepath.Clean(root) {
				break
			}
			path = filepath.Dir(path)
		}
	}
	if db.Migrator().HasTable(&models.AssetRecord{}) {
		var assets []models.AssetRecord
		if err := db.Where("missing = ?", false).Select("path, source_path, root_path, group_path").Find(&assets).Error; err != nil {
			return nil, err
		}
		for _, asset := range assets {
			add(asset.Path, false)
			add(asset.SourcePath, false)
			add(asset.RootPath, true)
			add(asset.GroupPath, true)
		}
	}
	if db.Migrator().HasTable(&models.QueueJob{}) {
		var jobs []models.QueueJob
		if err := db.Select("id, media_path, output_path, published_path, planned_published_path, replacement_target_path, original_archived_path, subtitle_artifacts, status, stage, validation_status, validation_outcome, validation_actionable, validation_override, published_at, started_at, finished_at").Find(&jobs).Error; err != nil {
			return nil, err
		}
		activeReservations := map[uint]struct{}{}
		if db.Migrator().HasTable(&models.SchedulerReservation{}) {
			var reservations []struct {
				JobID uint
			}
			if err := db.Model(&models.SchedulerReservation{}).Select("job_id").Where("state IN ?", []string{scheduler.ReservationStateLocked, scheduler.ReservationStateActive}).Find(&reservations).Error; err != nil {
				return nil, err
			}
			for _, reservation := range reservations {
				activeReservations[reservation.JobID] = struct{}{}
			}
		}
		activePublisherJobs := map[uint]struct{}{}
		if db.Migrator().HasTable(&models.PublisherPublication{}) {
			var publications []struct {
				QueueJobID uint
			}
			now := time.Now().UTC()
			if err := db.Model(&models.PublisherPublication{}).Select("queue_job_id").Where("TRIM(active_attempt_id) <> '' OR lease_expires_at > ? OR original_archive_lease_expires_at > ?", now, now).Find(&publications).Error; err != nil {
				return nil, err
			}
			for _, publication := range publications {
				activePublisherJobs[publication.QueueJobID] = struct{}{}
			}
		}
		for _, job := range jobs {
			_, hasActiveReservation := activeReservations[job.ID]
			_, hasActivePublisher := activePublisherJobs[job.ID]
			if !jobHasActiveHousekeepingReferencesFromJob(&job) && !hasActiveReservation && !hasActivePublisher {
				continue
			}
			for _, path := range []string{job.MediaPath, job.OutputPath, job.PublishedPath, job.PlannedPublishedPath, job.ReplacementTargetPath, job.OriginalArchivedPath} {
				add(path, false)
			}
			walkHousekeepingJSONStrings(job.SubtitleArtifacts, func(path string) { add(path, false) })
		}
	}
	if db.Migrator().HasTable(&models.AssetMaintenanceOperation{}) {
		var operations []models.AssetMaintenanceOperation
		if err := db.Where("status IN ?", []string{"queued", "running"}).Select("asset_path, temporary_path, backup_path").Find(&operations).Error; err != nil {
			return nil, err
		}
		for _, operation := range operations {
			add(operation.AssetPath, false)
			add(operation.TemporaryPath, false)
			add(operation.BackupPath, false)
		}
	}
	return protected, nil
}

func protectedOperationalDirectories(db *gorm.DB, root string, includeQueueWorkspaces bool) (map[string]struct{}, error) {
	protected := map[string]struct{}{}
	if root == "" {
		return protected, nil
	}
	root = canonicalHousekeepingPath(root)
	if includeQueueWorkspaces && db.Migrator().HasTable(&models.QueueJob{}) {
		var jobs []models.QueueJob
		if err := db.Select("id, media_path, worker_name, status, stage, published_at").Find(&jobs).Error; err != nil {
			return nil, err
		}
		for _, job := range jobs {
			if !isProtectedHousekeepingJob(&job) && !jobHasActiveHousekeepingReferences(db, job.ID, filepath.Join(root, fmt.Sprintf("job-%d", job.ID))) {
				continue
			}
			protectHousekeepingPathAndParents(protected, root, filepath.Join(root, fmt.Sprintf("job-%d", job.ID)))
		}
	}
	if db.Migrator().HasTable(&models.AssetMaintenanceOperation{}) {
		var operations []models.AssetMaintenanceOperation
		if err := db.Where("status IN ?", []string{"queued", "running"}).Select("asset_path, temporary_path, backup_path").Find(&operations).Error; err != nil {
			return nil, err
		}
		for _, operation := range operations {
			for _, path := range []string{operation.AssetPath, operation.TemporaryPath, operation.BackupPath} {
				if strings.TrimSpace(path) == "" || !housekeepingPathWithin(root, path) {
					continue
				}
				protectHousekeepingPathAndParents(protected, root, path)
			}
		}
	}
	return protected, nil
}

func protectHousekeepingPathAndParents(protected map[string]struct{}, root, path string) {
	root = canonicalHousekeepingPath(root)
	path = canonicalHousekeepingPath(path)
	if !pathWithin(root, path) {
		return
	}
	for current := filepath.Clean(path); pathWithin(root, current); current = filepath.Dir(current) {
		protected[filepath.Clean(current)] = struct{}{}
		if filepath.Clean(current) == filepath.Clean(root) {
			break
		}
	}
}

func addBoundCandidate(state *housekeepingPreviewState, candidate HousekeepingCandidate, identity string) {
	if len(state.Candidates) >= housekeepingMaxCandidates {
		if !containsHousekeepingString(state.Errors, "candidate set limit reached; remaining entries were not previewed") {
			state.Errors = append(state.Errors, "candidate set limit reached; remaining entries were not previewed")
		}
		return
	}
	candidate.CandidateID = housekeepingCandidateID(candidate, identity)
	state.Candidates = append(state.Candidates, housekeepingBoundCandidate{Candidate: candidate, Identity: identity})
}

func housekeepingCandidateID(candidate HousekeepingCandidate, identity string) string {
	copy := candidate
	copy.CandidateID = ""
	material, _ := json.Marshal(struct {
		Candidate HousekeepingCandidate
		Identity  string
	}{copy, identity})
	sum := sha256.Sum256(material)
	return hex.EncodeToString(sum[:16])
}

func housekeepingReportForState(state housekeepingPreviewState, dryRun bool) HousekeepingReport {
	report := HousekeepingReport{
		RanAt: state.CreatedAt, DryRun: dryRun, PreviewID: state.PreviewID,
		PreviewExpiresAt: &state.ExpiresAt, Candidates: make([]HousekeepingCandidate, 0, len(state.Candidates)),
		Summaries: []HousekeepingCategorySummary{}, RemovedPaths: []string{}, Errors: append([]string{}, state.Errors...), RuntimeSnapshots: state.RuntimeSnapshots,
	}
	for _, bound := range state.Candidates {
		report.Candidates = append(report.Candidates, bound.Candidate)
	}
	if !dryRun {
		report.PreviewID = ""
		report.PreviewExpiresAt = nil
	}
	report.Summaries = housekeepingSummaries(report.Candidates)
	return report
}

func housekeepingSummaries(candidates []HousekeepingCandidate) []HousekeepingCategorySummary {
	byKind := map[string]HousekeepingCategorySummary{}
	for _, candidate := range candidates {
		summary := byKind[candidate.Kind]
		summary.Kind = candidate.Kind
		summary.CandidateCount++
		if candidate.SafeToDelete {
			summary.RecoverableBytes += candidate.SizeBytes
		}
		byKind[candidate.Kind] = summary
	}
	summaries := make([]HousekeepingCategorySummary, 0, len(byKind))
	for _, summary := range byKind {
		summaries = append(summaries, summary)
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Kind < summaries[j].Kind })
	return summaries
}

func applyHousekeepingPlan(db *gorm.DB, roots map[string]string, policy HousekeepingPolicy, state *housekeepingPreviewState, report *HousekeepingReport) {
	report.RuntimeSnapshots = state.RuntimeSnapshots
	report.Candidates = make([]HousekeepingCandidate, len(state.Candidates))
	for index, bound := range state.Candidates {
		candidate := bound.Candidate
		if candidate.SafeToDelete {
			updated, removed, err := executeHousekeepingCandidate(db, roots, policy, report.RanAt, bound)
			candidate = updated
			if err != nil {
				candidate.SafeToDelete = false
				candidate.BlockedReason = err.Error()
				report.Errors = append(report.Errors, fmt.Sprintf("%s: %v", candidate.Path, err))
			}
			if removed {
				report.RemovedPaths = append(report.RemovedPaths, candidate.Path)
				report.RecoveredBytes += candidate.SizeBytes
				if candidate.Kind == housekeepingKindRuntimeSnapshot {
					report.RuntimeSnapshots.Deleted++
				}
			}
		}
		report.Candidates[index] = candidate
	}
	report.Summaries = housekeepingSummaries(report.Candidates)
}

func saveHousekeepingPreview(db *gorm.DB, state housekeepingPreviewState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	value := models.JSONMap{}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	return db.Save(&models.AppSetting{Key: housekeepingPreviewKey, Value: value}).Error
}

func consumeHousekeepingPreview(db *gorm.DB, previewID string, policy HousekeepingPolicy, roots map[string]string, now time.Time) (housekeepingPreviewState, error) {
	var state housekeepingPreviewState
	err := db.Transaction(func(tx *gorm.DB) error {
		var setting models.AppSetting
		result := tx.Where("key = ?", housekeepingPreviewKey).Limit(1).Find(&setting)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errStaleHousekeepingPreview
		}
		data, err := json.Marshal(setting.Value)
		if err != nil {
			return errStaleHousekeepingPreview
		}
		if err := json.Unmarshal(data, &state); err != nil {
			return errStaleHousekeepingPreview
		}
		if state.PreviewID != previewID || !now.Before(state.ExpiresAt) || !reflect.DeepEqual(state.Policy, policy) || !reflect.DeepEqual(state.RoleRoots, roots) {
			return errStaleHousekeepingPreview
		}
		deleted := tx.Where("key = ? AND updated_at = ?", housekeepingPreviewKey, setting.UpdatedAt).Delete(&models.AppSetting{})
		if deleted.Error != nil {
			return deleted.Error
		}
		if deleted.RowsAffected != 1 {
			return errStaleHousekeepingPreview
		}
		return nil
	})
	if err != nil {
		return housekeepingPreviewState{}, err
	}
	return state, nil
}

func newHousekeepingPreviewID() (string, error) {
	var value [24]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func housekeepingRoleRoots(roles scheduler.StorageRoles) (map[string]string, error) {
	roots := make(map[string]string, len(roles))
	for role, config := range roles {
		path := strings.TrimSpace(config.Path)
		if path == "" {
			continue
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		absolute = filepath.Clean(absolute)
		if resolved, resolveErr := filepath.EvalSymlinks(absolute); resolveErr == nil {
			absolute = filepath.Clean(resolved)
		} else if !errors.Is(resolveErr, os.ErrNotExist) {
			return nil, resolveErr
		}
		roots[string(role)] = absolute
	}
	return roots, nil
}

func unsafeRawRoleRoot(roots map[string]string) bool {
	raw := roots["raw"]
	if raw == "" {
		return false
	}
	for _, role := range []string{"library", "originals_archive", "work", "cache", "reports", "logs"} {
		other := roots[role]
		if other != "" && (pathWithin(raw, other) || pathWithin(other, raw)) {
			return true
		}
	}
	return false
}

func unsafeOperationalRoleRoot(roots map[string]string, role string) bool {
	root := roots[role]
	if root == "" {
		return false
	}
	if overlapsProtectedStorageRoot(roots, root) {
		return true
	}
	for _, otherRole := range []string{"work", "cache", "reports", "logs"} {
		if otherRole == role {
			continue
		}
		other := roots[otherRole]
		if other == "" || (!pathWithin(root, other) && !pathWithin(other, root)) {
			continue
		}
		if (role == "reports" && otherRole == "logs") || (role == "logs" && otherRole == "reports") {
			continue
		}
		return true
	}
	return false
}

func publisherArchiveStageIsAbandoned(publication models.PublisherPublication) bool {
	status := strings.ToLower(strings.TrimSpace(publication.OriginalArchiveStatus))
	if status == "complete" || status == "not_applicable" {
		return true
	}
	return publication.State == PublicationPublished && (status == "failed" || status == "interrupted" || status == "canceled" || status == "cancelled")
}

func coveredBySafeWorkspace(candidates []housekeepingBoundCandidate, path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	for _, bound := range candidates {
		candidate := bound.Candidate
		if candidate.Kind == housekeepingKindWorkspace && candidate.SafeToDelete && pathWithin(candidate.Path, path) {
			return true
		}
	}
	return false
}

func readRoleRoot(root string) ([]os.DirEntry, error) {
	if root == "" {
		return nil, os.ErrNotExist
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("configured storage role root is not a real directory: %s", root)
	}
	return os.ReadDir(root)
}

func resolveOwnedFilePath(roots map[string]string, path string, allowedRoles []string) (string, string, os.FileInfo, error) {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", "", nil, errors.New("owned artifact path is not an absolute clean path")
	}
	resolved, err := resolveWithoutFollowingLeaf(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", "", nil, errors.New("artifact path contains an unsafe or escaping symlink")
	}
	if resolved == "" {
		resolved = filepath.Clean(path)
	}
	role := ""
	longest := -1
	for _, allowed := range allowedRoles {
		root := roots[allowed]
		if root == "" || !pathWithin(root, resolved) || filepath.Clean(root) == filepath.Clean(resolved) {
			continue
		}
		if len(root) > longest {
			role, longest = allowed, len(root)
		} else if len(root) == longest {
			return "", "", nil, errors.New("artifact storage role is ambiguous")
		}
	}
	if role == "" {
		return "", "", nil, errors.New("artifact is outside configured temporary storage roles")
	}
	if role != "originals_archive" && role != "raw" && role != "library" && overlapsProtectedStorageRoot(roots, resolved) {
		return "", "", nil, errors.New("artifact path overlaps RAW, Library, or originals archive storage")
	}
	if (role == "work" || role == "cache" || role == "reports" || role == "logs") && unsafeOperationalRoleRoot(roots, role) {
		return "", "", nil, errors.New("configured operational storage role overlaps another protected role")
	}
	if err != nil {
		return role, resolved, nil, nil
	}
	info, statErr := os.Lstat(resolved)
	if errors.Is(statErr, os.ErrNotExist) {
		return role, resolved, nil, nil
	}
	if statErr != nil {
		return "", "", nil, statErr
	}
	return role, resolved, info, nil
}

func resolveWithoutFollowingLeaf(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil || filepath.Clean(absolute) != path {
		return "", errors.New("path is not clean")
	}
	parent := filepath.Dir(path)
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if errors.Is(err, os.ErrNotExist) {
		return "", os.ErrNotExist
	}
	if err != nil {
		return "", err
	}
	return filepath.Join(resolvedParent, filepath.Base(path)), nil
}

func overlapsProtectedStorageRoot(roots map[string]string, path string) bool {
	for _, role := range []string{"raw", "library", "originals_archive"} {
		root := roots[role]
		if root == "" {
			continue
		}
		if pathWithin(root, path) || pathWithin(path, root) {
			return true
		}
	}
	return false
}

func pathWithin(root, path string) bool {
	root, path = filepath.Clean(root), filepath.Clean(path)
	if root == "" || path == "" {
		return false
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func isConfiguredRoleRoot(roots map[string]string, path string) bool {
	for _, root := range roots {
		if root != "" && filepath.Clean(root) == filepath.Clean(path) {
			return true
		}
	}
	return false
}

func pathDepth(path string) int {
	clean := filepath.Clean(path)
	depth := 0
	for clean != filepath.Dir(clean) {
		depth++
		clean = filepath.Dir(clean)
	}
	return depth
}

func statIdentity(info os.FileInfo, directory bool) string {
	dev, ino, hasStableID := filesystemDeviceInode(info)
	if directory {
		if hasStableID {
			return fmt.Sprintf("dir:%d:%d:%o", dev, ino, info.Mode().Perm())
		}
		return fmt.Sprintf("dir:%s:%d", info.Mode(), info.ModTime().UnixNano())
	}
	if hasStableID {
		return fmt.Sprintf("file:%d:%d:%o:%d:%d", dev, ino, info.Mode().Perm(), info.Size(), info.ModTime().UnixNano())
	}
	return fmt.Sprintf("file:%s:%d:%d:%d", info.Mode(), info.Size(), info.ModTime().UnixNano(), info.ModTime().Unix())
}

func filesystemDeviceInode(info os.FileInfo) (uint64, uint64, bool) {
	value := reflect.ValueOf(info.Sys())
	if !value.IsValid() {
		return 0, 0, false
	}
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if !value.IsValid() || value.Kind() != reflect.Struct {
		return 0, 0, false
	}
	read := func(name string) (uint64, bool) {
		field := value.FieldByName(name)
		if !field.IsValid() {
			return 0, false
		}
		switch field.Kind() {
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return field.Uint(), true
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return uint64(field.Int()), true
		default:
			return 0, false
		}
	}
	dev, devOK := read("Dev")
	ino, inoOK := read("Ino")
	return dev, ino, devOK && inoOK
}

func directoryTreeIdentity(root string) (string, error) {
	hash := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("unsafe workspace entry %s", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(hash, "%s\x00%s\x00", relative, statIdentity(info, info.IsDir()))
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func directorySizeNoSymlinks(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("symlink found under workspace")
		}
		if entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		} else if !entry.IsDir() {
			return errors.New("special file found under workspace")
		}
		return nil
	})
	return total, err
}

func containsHousekeepingString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func housekeepingCandidateOrder(kind string) int {
	switch kind {
	case housekeepingKindStageArtifact, housekeepingKindJobLog, housekeepingKindTestEncode:
		return 0
	case housekeepingKindWorkspace:
		return 1
	case housekeepingKindRuntimeSnapshot:
		return 2
	case housekeepingKindEmptyDirectory:
		return 3
	default:
		return 4
	}
}

func executeHousekeepingCandidate(db *gorm.DB, roots map[string]string, policy HousekeepingPolicy, now time.Time, bound housekeepingBoundCandidate) (HousekeepingCandidate, bool, error) {
	candidate := bound.Candidate
	if candidate.CandidateID == "" || candidate.CandidateID != housekeepingCandidateID(candidate, bound.Identity) {
		candidate.SafeToDelete = false
		candidate.BlockedReason = "preview candidate identity is invalid"
		return candidate, false, nil
	}
	if !candidate.SafeToDelete {
		return candidate, false, nil
	}
	if candidate.Kind == housekeepingKindRuntimeSnapshot {
		removed, reason, err := executeRuntimeSnapshotCandidate(db, candidate, bound.Identity, now)
		if reason != "" {
			candidate.SafeToDelete = false
			candidate.BlockedReason = reason
		}
		return candidate, removed, err
	}
	if candidate.Kind == housekeepingKindTestEncode {
		return executeTestEncodeCandidate(db, candidate, bound.Identity, now)
	}
	allowedRoles := []string{}
	switch candidate.Kind {
	case housekeepingKindWorkspace:
		allowedRoles = []string{"work"}
	case housekeepingKindJobLog:
		allowedRoles = []string{"logs"}
	case housekeepingKindEmptyDirectory:
		allowedRoles = []string{candidate.OperationID}
	case housekeepingKindStageArtifact:
		if strings.HasPrefix(candidate.Ownership, "publisher_publication") {
			allowedRoles = []string{"originals_archive"}
		} else {
			allowedRoles = []string{"work", "cache", "reports", "logs"}
		}
	default:
		candidate.SafeToDelete = false
		candidate.BlockedReason = "unknown candidate kind"
		return candidate, false, nil
	}
	role, path, info, pathErr := resolveOwnedFilePath(roots, candidate.Path, allowedRoles)
	if pathErr != nil {
		candidate.SafeToDelete = false
		candidate.BlockedReason = pathErr.Error()
		return candidate, false, nil
	}
	if path != candidate.Path {
		candidate.SafeToDelete = false
		candidate.BlockedReason = "candidate path resolved differently from the previewed path"
		return candidate, false, nil
	}
	if info == nil {
		candidate.SizeBytes = 0
		return candidate, true, nil
	}
	identity := statIdentity(info, info.IsDir())
	if candidate.Kind == housekeepingKindWorkspace {
		identity, pathErr = directoryTreeIdentity(path)
		if pathErr != nil {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "workspace contains an unsafe or unreadable entry"
			return candidate, false, nil
		}
	}
	if identity != bound.Identity {
		candidate.SafeToDelete = false
		candidate.BlockedReason = "path identity changed after preview"
		return candidate, false, nil
	}
	switch candidate.Kind {
	case housekeepingKindWorkspace:
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "workspace is no longer a real directory"
			return candidate, false, nil
		}
		id := candidate.JobID
		if id == 0 {
			parsed, parseErr := strconv.ParseUint(strings.TrimPrefix(filepath.Base(path), "job-"), 10, 64)
			if parseErr != nil || parsed == 0 {
				candidate.SafeToDelete = false
				candidate.BlockedReason = "orphan workspace name is no longer canonical"
				return candidate, false, nil
			}
			id = uint(parsed)
		}
		current, eligible, err := housekeepingCandidate(db, id, path, info.ModTime(), policy, now)
		if err != nil {
			return candidate, false, err
		}
		if !eligible || !current.SafeToDelete || current.Reason != candidate.Reason {
			candidate.SafeToDelete = false
			candidate.BlockedReason = current.BlockedReason
			if candidate.BlockedReason == "" {
				candidate.BlockedReason = "workspace lifecycle or retention changed after preview"
			}
			return candidate, false, nil
		}
		currentSize, err := directorySizeNoSymlinks(path)
		if err != nil {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "workspace became unsafe after preview"
			return candidate, false, nil
		}
		if currentSize != candidate.SizeBytes {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "workspace size changed after preview"
			return candidate, false, nil
		}
		finalIdentity, identityErr := directoryTreeIdentity(path)
		if identityErr != nil || finalIdentity != bound.Identity {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "workspace contents changed immediately before cleanup"
			return candidate, false, nil
		}
		if err := os.RemoveAll(path); err != nil {
			return candidate, false, err
		}
		return candidate, true, nil
	case housekeepingKindJobLog:
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "job log is no longer a regular non-symlink file"
			return candidate, false, nil
		}
		jobID := candidate.JobID
		if jobID == 0 {
			parsed, parseErr := strconv.ParseUint(strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "job-"), ".log"), 10, 64)
			if parseErr != nil || parsed == 0 {
				candidate.SafeToDelete = false
				candidate.BlockedReason = "orphan log name is no longer canonical"
				return candidate, false, nil
			}
			jobID = uint(parsed)
		}
		var job models.QueueJob
		result := db.Where("id = ?", jobID).Limit(1).Find(&job)
		if result.Error != nil {
			return candidate, false, result.Error
		}
		current := HousekeepingCandidate{Kind: housekeepingKindJobLog, Path: path, Reason: "orphan job log retention expired", ModifiedAt: info.ModTime().UTC().Format(time.RFC3339Nano), SizeBytes: info.Size(), SafeToDelete: true}
		if result.RowsAffected > 0 {
			current = classifyJobLog(job, path, info, policy, now)
		}
		if current.SafeToDelete && jobHasActiveHousekeepingReferences(db, jobID, "") {
			current.SafeToDelete = false
			current.BlockedReason = "active worker, reservation, or operation still owns this job"
		}
		if !current.SafeToDelete || current.Reason != candidate.Reason {
			candidate.SafeToDelete = false
			candidate.BlockedReason = current.BlockedReason
			if candidate.BlockedReason == "" {
				candidate.BlockedReason = "job lifecycle or log retention changed after preview"
			}
			return candidate, false, nil
		}
		removed, changed, err := removeHousekeepingFileIfUnchanged(path, bound.Identity)
		if changed {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "job log changed immediately before cleanup"
		}
		return candidate, removed, err
	case housekeepingKindStageArtifact:
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "stage artifact is no longer a regular non-symlink file"
			return candidate, false, nil
		}
		if strings.HasPrefix(candidate.Ownership, "publisher_publication") {
			current, err := publisherArtifactStillOwned(db, candidate, now)
			if err != nil {
				return candidate, false, err
			}
			if !current {
				candidate.SafeToDelete = false
				candidate.BlockedReason = "Publisher staging ownership or archive state changed after preview"
				return candidate, false, nil
			}
		} else {
			current, err := maintenanceArtifactStillOwned(db, candidate)
			if err != nil {
				return candidate, false, err
			}
			if !current {
				candidate.SafeToDelete = false
				candidate.BlockedReason = "operation ownership or terminal state changed after preview"
				return candidate, false, nil
			}
		}
		removed, changed, err := removeHousekeepingFileIfUnchanged(path, bound.Identity)
		if changed {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "stage artifact changed immediately before cleanup"
		}
		return candidate, removed, err
	case housekeepingKindEmptyDirectory:
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "directory was replaced or became a symlink"
			return candidate, false, nil
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return candidate, false, err
		}
		if len(entries) != 0 {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "directory contains an entry at execution time"
			return candidate, false, nil
		}
		if role == "raw" {
			protected, err := protectedRawDirectories(db, roots["raw"])
			if err != nil {
				return candidate, false, err
			}
			if _, exists := protected[filepath.Clean(path)]; exists {
				candidate.SafeToDelete = false
				candidate.BlockedReason = "RAW directory is referenced by an asset, Queue source/restore target, or active operation"
				return candidate, false, nil
			}
			if unsafeRawRoleRoot(roots) {
				candidate.SafeToDelete = false
				candidate.BlockedReason = "configured RAW storage role overlaps Library or originals archive"
				return candidate, false, nil
			}
			if filepath.Clean(path) == filepath.Clean(roots["raw"]) {
				candidate.SafeToDelete = false
				candidate.BlockedReason = "configured RAW root is never removable"
				return candidate, false, nil
			}
		} else if role != "raw" {
			if unsafeOperationalRoleRoot(roots, role) {
				candidate.SafeToDelete = false
				candidate.BlockedReason = "configured operational storage role overlaps another protected role"
				return candidate, false, nil
			}
			protected, err := protectedOperationalDirectories(db, roots[role], role == "work")
			if err != nil {
				return candidate, false, err
			}
			if _, exists := protected[filepath.Clean(path)]; exists {
				candidate.SafeToDelete = false
				candidate.BlockedReason = "work directory is referenced by an active job or operation"
				return candidate, false, nil
			}
		}
		if filepath.Clean(path) == filepath.Clean(roots[role]) || isConfiguredRoleRoot(roots, path) {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "configured storage role root is never removable"
			return candidate, false, nil
		}
		removed, changed, err := removeEmptyHousekeepingDirectoryIfUnchanged(path, bound.Identity)
		if changed {
			candidate.SafeToDelete = false
			candidate.BlockedReason = "directory became nonempty or changed immediately before cleanup"
		}
		return candidate, removed, err
	}
	return candidate, false, nil
}

func removeHousekeepingFileIfUnchanged(path, identity string) (removed, changed bool, err error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return true, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || statIdentity(info, false) != identity {
		return false, true, nil
	}
	if err := os.Remove(path); errors.Is(err, os.ErrNotExist) {
		return true, false, nil
	} else if err != nil {
		return false, false, err
	}
	return true, false, nil
}

func removeEmptyHousekeepingDirectoryIfUnchanged(path, identity string) (removed, changed bool, err error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return true, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || statIdentity(info, true) != identity {
		return false, true, nil
	}
	entries, err := os.ReadDir(path)
	if errors.Is(err, os.ErrNotExist) {
		return true, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if len(entries) != 0 {
		return false, true, nil
	}
	if err := os.Remove(path); errors.Is(err, os.ErrNotExist) {
		return true, false, nil
	} else if err != nil {
		return false, false, err
	}
	return true, false, nil
}

func executeTestEncodeCandidate(db *gorm.DB, candidate HousekeepingCandidate, identity string, now time.Time) (HousekeepingCandidate, bool, error) {
	var test models.TestEncode
	result := db.Where("id = ?", candidate.TestEncodeID).Limit(1).Find(&test)
	if result.Error != nil {
		return candidate, false, result.Error
	}
	if result.RowsAffected == 0 || test.Keep || test.DeletedAt != nil || test.ExpiresAt == nil || test.ExpiresAt.After(now) || test.Status == testEncodeWaiting || test.Status == testEncodeRunning || test.Status == testEncodeDeleted || test.Status == "expired" || testEncodeOperationActive(test.ID) {
		candidate.SafeToDelete = false
		candidate.BlockedReason = "Test Encode is kept, active, missing, or no longer expired"
		return candidate, false, nil
	}
	if filepath.Clean(test.OutputPath) != filepath.Clean(candidate.Path) {
		candidate.SafeToDelete = false
		candidate.BlockedReason = "Test Encode output ownership changed after preview"
		return candidate, false, nil
	}
	currentIdentity, err := testEncodeCandidateIdentity(db, test)
	if err != nil {
		return candidate, false, err
	}
	if currentIdentity != identity {
		candidate.SafeToDelete = false
		candidate.BlockedReason = "Test Encode artifact identities changed after preview"
		return candidate, false, nil
	}
	diskUsage, err := (AssetHandler{db: db}).testEncodeOwnedArtifactDiskUsage(test)
	if err != nil {
		candidate.SafeToDelete = false
		candidate.BlockedReason = err.Error()
		return candidate, false, nil
	}
	if diskUsage != candidate.SizeBytes {
		candidate.SafeToDelete = false
		candidate.BlockedReason = "Test Encode artifact size changed after preview"
		return candidate, false, nil
	}
	mutated, cleanupErr := (AssetHandler{db: db}).removeTestEncodeFilesWithEvidence(test)
	if test.Status == testEncodeReady && mutated {
		_ = recordJellyfinTrigger(db, jellyfinTrigger{Reason: jellyfinReasonTestRemoved, SourceKey: testEncodeJellyfinSourceKey(jellyfinReasonTestRemoved, test.ID), LibraryID: test.LibraryID, TestEncodeID: test.ID, Reconcile: true})
	}
	if cleanupErr != nil {
		_ = db.Model(&test).Updates(map[string]any{"reconciliation_status": "cleanup_failed", "reconciliation_message": cleanupErr.Error(), "last_reconciled_at": now}).Error
		return candidate, false, cleanupErr
	}
	_ = db.Model(&test).Updates(map[string]any{"status": "expired", "phase": "expired", "deleted_at": &now, "output_path": "", "temporary_path": "", "subtitle_artifacts": models.JSONList{}, "updated_at": now}).Error
	return candidate, true, nil
}

func testEncodeCandidateIdentity(db *gorm.DB, test models.TestEncode) (string, error) {
	paths := []string{test.OutputPath, test.TemporaryPath}
	for _, raw := range test.SubtitleArtifacts {
		if path, ok := raw.(string); ok {
			paths = append(paths, path)
		}
	}
	if db.Migrator().HasTable(&models.TestEncodeArtifact{}) {
		var artifacts []models.TestEncodeArtifact
		if err := db.Where("test_encode_id = ?", test.ID).Order("path ASC").Find(&artifacts).Error; err != nil {
			return "", err
		}
		for _, artifact := range artifacts {
			paths = append(paths, fmt.Sprintf("%d:%s:%s:%t", artifact.ID, artifact.Path, artifact.Type, artifact.Expected))
		}
	}
	sort.Strings(paths)
	material, err := json.Marshal(paths)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(material)
	diskUsage, err := (AssetHandler{db: db}).testEncodeOwnedArtifactDiskUsage(test)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("test-encode:%d:%s:%d", test.ID, hex.EncodeToString(sum[:]), diskUsage), nil
}

func executeRuntimeSnapshotCandidate(db *gorm.DB, candidate HousekeepingCandidate, identity string, now time.Time) (bool, string, error) {
	id, err := strconv.ParseUint(candidate.OperationID, 10, 64)
	if err != nil || id == 0 {
		return false, "runtime snapshot identity is invalid", nil
	}
	var snapshot models.RuntimeSnapshot
	result := db.Where("id = ?", uint(id)).Limit(1).Find(&snapshot)
	if result.Error != nil {
		return false, "", result.Error
	}
	if result.RowsAffected == 0 {
		return true, "", nil
	}
	if identity != fmt.Sprintf("runtime:%d:%d", snapshot.ID, snapshot.DetectedAt.UnixNano()) {
		return false, "runtime snapshot identity changed after preview", nil
	}
	latestID, err := newestRuntimeSnapshotID(db)
	if err != nil {
		return false, "", err
	}
	if latestID == snapshot.ID || now.Before(snapshot.DetectedAt.Add(runtimeSnapshotRetentionAge)) {
		return false, "runtime snapshot is now latest or inside the retention window", nil
	}
	referenced, err := NewRuntimeSnapshotReferenceRegistry().isReferenced(db, snapshot.ID)
	if err != nil {
		return false, "", err
	}
	if referenced {
		return false, "runtime snapshot gained a historical execution reference", nil
	}
	deleted := db.Where("id = ? AND detected_at = ?", snapshot.ID, snapshot.DetectedAt).Delete(&models.RuntimeSnapshot{})
	return deleted.RowsAffected == 1, "", deleted.Error
}

func jobHasActiveHousekeepingReferences(db *gorm.DB, jobID uint, workspace string) bool {
	var job models.QueueJob
	jobResult := db.Where("id = ?", jobID).Limit(1).Find(&job)
	if jobResult.Error != nil {
		return true
	}
	if jobResult.RowsAffected > 0 && jobHasActiveHousekeepingReferencesFromJob(&job) {
		return true
	}
	if db.Migrator().HasTable(&models.SchedulerReservation{}) {
		var count int64
		if err := db.Model(&models.SchedulerReservation{}).Where("job_id = ? AND state IN ?", jobID, []string{scheduler.ReservationStateLocked, scheduler.ReservationStateActive}).Count(&count).Error; err != nil {
			return true
		}
		if count > 0 {
			return true
		}
	}
	if db.Migrator().HasTable(&models.AssetMaintenanceOperation{}) {
		var operations []models.AssetMaintenanceOperation
		if err := db.Where("status IN ?", []string{"queued", "running"}).Find(&operations).Error; err != nil {
			return true
		}
		for _, operation := range operations {
			for _, path := range []string{operation.AssetPath, operation.TemporaryPath, operation.BackupPath} {
				if housekeepingPathWithin(workspace, path) {
					return true
				}
			}
		}
	}
	if db.Migrator().HasTable(&models.PublisherPublication{}) {
		var publications []models.PublisherPublication
		if err := db.Where("queue_job_id = ?", jobID).Find(&publications).Error; err != nil {
			return true
		}
		now := time.Now().UTC()
		for _, publication := range publications {
			if publication.ActiveAttemptID != "" || (publication.LeaseExpiresAt != nil && publication.LeaseExpiresAt.After(now)) || (publication.OriginalArchiveLeaseExpiresAt != nil && publication.OriginalArchiveLeaseExpiresAt.After(now)) {
				return true
			}
		}
	}
	return false
}

func jobHasActiveHousekeepingReferencesFromJob(job *models.QueueJob) bool {
	if job == nil {
		return false
	}
	if isProtectedHousekeepingJob(job) || (job.StartedAt != nil && job.FinishedAt == nil) {
		return true
	}
	return false
}

func isProtectedHousekeepingJob(job *models.QueueJob) bool {
	if job == nil {
		return true
	}
	status := strings.ToLower(strings.TrimSpace(job.Status))
	stage := strings.ToLower(strings.TrimSpace(job.Stage))
	if status == JobStatusQueued || status == JobStatusRunning || status == "validating" || status == "validation_review" || status == "ready" || status == "publishing" || status == "review" || status == "blocked" {
		return true
	}
	if strings.Contains(status, "review") || strings.Contains(status, "validat") || strings.Contains(status, "publish") || strings.Contains(status, "block") || strings.Contains(status, "ready") {
		return true
	}
	if strings.Contains(stage, "review") || strings.Contains(stage, "validat") || strings.Contains(stage, "publish") || strings.Contains(stage, "ready") || strings.Contains(stage, "block") {
		return true
	}
	for _, value := range []string{job.ValidationStatus, job.ValidationOutcome} {
		value = strings.ToLower(strings.TrimSpace(value))
		if strings.Contains(value, "review") || strings.Contains(value, "block") || strings.Contains(value, "pending") || strings.Contains(value, "required") {
			return true
		}
	}
	if job.ValidationActionable && !job.ValidationOverride {
		return true
	}
	return job.Status == JobStatusCompleted && job.PublishedAt == nil
}

func operationIsActive(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued", "running", "active", "in_progress", "in-progress", "processing", "publishing", "archiving":
		return true
	default:
		return false
	}
}

func operationIsAbandoned(status, phase string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	phase = strings.ToLower(strings.TrimSpace(phase))
	if operationIsActive(status) {
		return false
	}
	return status == "failed" || status == "canceled" || status == "cancelled" || status == "interrupted" || status == "abandoned" || strings.Contains(phase, "interrupted") || strings.Contains(phase, "canceled") || strings.Contains(phase, "cancelled") || strings.Contains(phase, "failed")
}

func operationArtifactReferenceBlock(db *gorm.DB, operationID, path string) string {
	if db.Migrator().HasTable(&models.AssetMaintenanceOperation{}) {
		var operations []models.AssetMaintenanceOperation
		if err := db.Where("id <> ? AND status IN ?", operationID, []string{"queued", "running"}).Find(&operations).Error; err != nil {
			return "active maintenance operation references could not be verified"
		}
		for _, operation := range operations {
			if sameHousekeepingPath(operation.TemporaryPath, path) || sameHousekeepingPath(operation.BackupPath, path) || sameHousekeepingPath(operation.AssetPath, path) {
				return "active maintenance operation references this artifact"
			}
		}
	}
	return protectedFileReferenceBlock(db, path, operationID, "")
}

func publisherArtifactReferenceBlock(db *gorm.DB, publicationID, path string) string {
	if db.Migrator().HasTable(&models.PublisherPublication{}) {
		var publications []models.PublisherPublication
		if err := db.Where("publication_id <> ?", publicationID).Find(&publications).Error; err != nil {
			return "Publisher staging references could not be verified"
		}
		for _, publication := range publications {
			if sameHousekeepingPath(publication.OriginalArchiveTemporaryPath, path) {
				return "another Publisher operation references this staging artifact"
			}
		}
		var owner models.PublisherPublication
		result := db.Select("manifest").Where("publication_id = ?", publicationID).Limit(1).Find(&owner)
		if result.Error != nil || result.RowsAffected == 0 {
			return "Publisher manifest references could not be verified"
		}
		if housekeepingJSONContainsPath(owner.Manifest, path) {
			return "Publisher manifest references this staging artifact"
		}
	}
	return protectedFileReferenceBlock(db, path, "", publicationID)
}

func protectedFileReferenceBlock(db *gorm.DB, path, exceptOperationID, exceptPublicationID string) string {
	if db.Migrator().HasTable(&models.AssetRecord{}) {
		var assets []models.AssetRecord
		if err := db.Select("path, source_path").Find(&assets).Error; err != nil {
			return "AssetRecord references could not be verified"
		}
		for _, asset := range assets {
			if sameHousekeepingPath(asset.Path, path) || sameHousekeepingPath(asset.SourcePath, path) {
				return "current or historical AssetRecord references this artifact"
			}
		}
	}
	if db.Migrator().HasTable(&models.QueueJob{}) {
		var jobs []models.QueueJob
		if err := db.Select("media_path, output_path, published_path, planned_published_path, replacement_target_path, original_archived_path, subtitle_artifacts").Find(&jobs).Error; err != nil {
			return "QueueJob references could not be verified"
		}
		for _, job := range jobs {
			for _, reference := range []string{job.MediaPath, job.OutputPath, job.PublishedPath, job.PlannedPublishedPath, job.ReplacementTargetPath, job.OriginalArchivedPath} {
				if sameHousekeepingPath(reference, path) {
					return "QueueJob provenance references this artifact"
				}
			}
			if housekeepingJSONContainsPath(job.SubtitleArtifacts, path) {
				return "QueueJob required sidecar ownership references this artifact"
			}
		}
	}
	if db.Migrator().HasTable(&models.TestEncodeArtifact{}) {
		var count int64
		if err := db.Model(&models.TestEncodeArtifact{}).Where("path = ?", path).Count(&count).Error; err != nil {
			return "Test Encode artifact ownership could not be verified"
		}
		if count > 0 {
			return "Test Encode artifact ownership references this path"
		}
	}
	if db.Migrator().HasTable(&models.TestEncode{}) {
		var tests []models.TestEncode
		if err := db.Select("output_path, temporary_path, subtitle_artifacts").Find(&tests).Error; err != nil {
			return "Test Encode lifecycle references could not be verified"
		}
		for _, test := range tests {
			if sameHousekeepingPath(test.OutputPath, path) || sameHousekeepingPath(test.TemporaryPath, path) {
				return "Test Encode lifecycle references this artifact"
			}
			for _, raw := range test.SubtitleArtifacts {
				if artifactPath, ok := raw.(string); ok && sameHousekeepingPath(artifactPath, path) {
					return "Test Encode subtitle ownership references this artifact"
				}
			}
		}
	}
	if db.Migrator().HasTable(&models.PublisherPublication{}) {
		var publications []models.PublisherPublication
		query := db.Select("publication_id, destination_path, source_path, original_archived_path, original_archive_source_path, original_archive_temporary_path")
		if exceptPublicationID != "" {
			query = query.Where("publication_id <> ?", exceptPublicationID)
		}
		if err := query.Find(&publications).Error; err != nil {
			return "Publisher lifecycle references could not be verified"
		}
		for _, publication := range publications {
			for _, reference := range []string{publication.DestinationPath, publication.SourcePath, publication.OriginalArchivedPath, publication.OriginalArchiveSourcePath, publication.OriginalArchiveTemporaryPath} {
				if sameHousekeepingPath(reference, path) {
					return "Publisher lifecycle or publication history references this artifact"
				}
			}
		}
	}
	if db.Migrator().HasTable(&models.AssetMaintenanceOperation{}) {
		var operations []models.AssetMaintenanceOperation
		query := db.Select("id, temporary_path, backup_path, asset_path")
		if exceptOperationID != "" {
			query = query.Where("id <> ?", exceptOperationID)
		}
		if err := query.Find(&operations).Error; err != nil {
			return "persisted operation references could not be verified"
		}
		for _, operation := range operations {
			for _, reference := range []string{operation.TemporaryPath, operation.BackupPath, operation.AssetPath} {
				if sameHousekeepingPath(reference, path) {
					return "another persisted operation references this artifact"
				}
			}
		}
	}
	_ = exceptOperationID
	return ""
}

func sameHousekeepingPath(left, right string) bool {
	left = canonicalHousekeepingPath(left)
	right = canonicalHousekeepingPath(right)
	return left != "" && right != "" && filepath.Clean(left) == filepath.Clean(right)
}

func housekeepingPathWithin(root, path string) bool {
	root = canonicalHousekeepingPath(root)
	path = canonicalHousekeepingPath(path)
	return root != "" && path != "" && pathWithin(root, path)
}

func housekeepingJSONContainsPath(value any, path string) bool {
	found := false
	walkHousekeepingJSONStrings(value, func(candidate string) {
		if sameHousekeepingPath(candidate, path) {
			found = true
		}
	})
	return found
}

func walkHousekeepingJSONStrings(value any, visit func(string)) {
	if value == nil || visit == nil {
		return
	}
	item := reflect.ValueOf(value)
	if !item.IsValid() {
		return
	}
	for item.Kind() == reflect.Interface || item.Kind() == reflect.Pointer {
		if item.IsNil() {
			return
		}
		item = item.Elem()
	}
	switch item.Kind() {
	case reflect.String:
		visit(item.String())
	case reflect.Slice, reflect.Array:
		for index := 0; index < item.Len(); index++ {
			walkHousekeepingJSONStrings(item.Index(index).Interface(), visit)
		}
	case reflect.Map:
		iterator := item.MapRange()
		for iterator.Next() {
			walkHousekeepingJSONStrings(iterator.Value().Interface(), visit)
		}
	}
}

func canonicalHousekeepingPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	absolute = filepath.Clean(absolute)
	current := absolute
	missing := []string{}
	for {
		resolved, resolveErr := filepath.EvalSymlinks(current)
		if resolveErr == nil {
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved)
		}
		if !errors.Is(resolveErr, os.ErrNotExist) {
			return absolute
		}
		parent := filepath.Dir(current)
		if parent == current {
			return absolute
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func maintenanceArtifactStillOwned(db *gorm.DB, candidate HousekeepingCandidate) (bool, error) {
	var operation models.AssetMaintenanceOperation
	result := db.Where("id = ?", candidate.OperationID).Limit(1).Find(&operation)
	if result.Error != nil || result.RowsAffected == 0 {
		return false, result.Error
	}
	if !sameHousekeepingPath(operation.TemporaryPath, candidate.Path) || operationIsActive(operation.Status) || !operationIsAbandoned(operation.Status, operation.Phase) {
		return false, nil
	}
	return operationArtifactReferenceBlock(db, operation.ID, candidate.Path) == "", nil
}

func publisherArtifactStillOwned(db *gorm.DB, candidate HousekeepingCandidate, now time.Time) (bool, error) {
	var publication models.PublisherPublication
	result := db.Where("publication_id = ?", candidate.OperationID).Limit(1).Find(&publication)
	if result.Error != nil || result.RowsAffected == 0 {
		return false, result.Error
	}
	if !sameHousekeepingPath(publication.OriginalArchiveTemporaryPath, candidate.Path) || !publisherArchiveStageIsAbandoned(publication) || (publication.OriginalArchiveLeaseExpiresAt != nil && publication.OriginalArchiveLeaseExpiresAt.After(now)) {
		return false, nil
	}
	return publisherArtifactReferenceBlock(db, publication.PublicationID, candidate.Path) == "", nil
}
