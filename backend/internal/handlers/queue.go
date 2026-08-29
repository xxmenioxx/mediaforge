package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type QueueHandler struct {
	db *gorm.DB
}

type QueueJobInput struct {
	MediaPath                 string `json:"mediaPath" binding:"required"`
	PublishMode               string `json:"publishMode"`
	BatchID                   string `json:"batchId"`
	BatchName                 string `json:"batchName"`
	BatchPosition             int    `json:"batchPosition"`
	LibraryID                 uint   `json:"libraryId" binding:"required"`
	ProfileID                 uint   `json:"profileId" binding:"required"`
	AudioProfileKey           string `json:"audioProfileKey"`
	TrackProfileKey           string `json:"trackProfileKey"`
	ProcessingMode            string `json:"processingMode"`
	Priority                  int    `json:"priority"`
	Notes                     string `json:"notes"`
	ResolveProfileAssignments bool   `json:"resolveProfileAssignments"`
	resolvedProfileResolution models.JSONMap
}

type QueueBatchInput struct {
	BatchID   string          `json:"batchId" binding:"required"`
	BatchName string          `json:"batchName"`
	Jobs      []QueueJobInput `json:"jobs" binding:"required,min=1,dive"`
}

type QueueSelectedAssetsInput struct {
	AssetIDs []uint `json:"assetIds" binding:"required,min=1,max=1000"`
	Commit   bool   `json:"commit"`
}

type QueueSelectedAssetsSummary struct {
	Selected   int   `json:"selected"`
	Eligible   int   `json:"eligible"`
	Queued     int   `json:"queued"`
	Skipped    int   `json:"skipped"`
	Failed     int   `json:"failed"`
	TitleCount int   `json:"titleCount"`
	SizeBytes  int64 `json:"sizeBytes"`
}

type QueueSelectedAssetResult struct {
	AssetID   uint   `json:"assetId"`
	Outcome   string `json:"outcome"`
	Reason    string `json:"reason,omitempty"`
	Message   string `json:"message,omitempty"`
	BatchID   string `json:"batchId,omitempty"`
	BatchName string `json:"batchName,omitempty"`
	JobID     uint   `json:"jobId,omitempty"`
}

type QueueSelectedAssetsBatchResult struct {
	BatchID   string `json:"batchId"`
	BatchName string `json:"batchName"`
	JobCount  int    `json:"jobCount"`
}

type QueueSelectedAssetsResponse struct {
	Summary QueueSelectedAssetsSummary       `json:"summary"`
	Results []QueueSelectedAssetResult       `json:"results"`
	Batches []QueueSelectedAssetsBatchResult `json:"batches"`
}

const (
	VideoAssignmentOverrideOnly = "override_only"
	VideoAssignmentAudioOnly    = "audio_only"
)

type QueueJobUpdateInput struct {
	MediaPath       string  `json:"mediaPath"`
	LibraryID       *uint   `json:"libraryId"`
	ProfileID       *uint   `json:"profileId"`
	AudioProfileKey *string `json:"audioProfileKey"`
	TrackProfileKey *string `json:"trackProfileKey"`
	ProcessingMode  *string `json:"processingMode"`
	Priority        *int    `json:"priority"`
	Status          string  `json:"status"`
	Notes           string  `json:"notes"`
}

type QueueReorderInput struct {
	JobID       uint   `json:"jobId" binding:"required"`
	TargetJobID uint   `json:"targetJobId" binding:"required"`
	Placement   string `json:"placement" binding:"required"`
}

func NewQueueHandler(db *gorm.DB) QueueHandler {
	return QueueHandler{db: db}
}

func (h QueueHandler) List(c *gin.Context) {
	var jobs []models.QueueJob
	if err := h.db.Where("dismissed_at IS NULL").Order("queue_position asc, created_at asc").Find(&jobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, jobs)
}

func (h QueueHandler) Reorder(c *gin.Context) {
	var input QueueReorderInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if input.JobID == input.TargetJobID || (input.Placement != "before" && input.Placement != "after") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "reorder requires different queued jobs and placement before or after"})
		return
	}

	var ordered []models.QueueJob
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("dismissed_at IS NULL AND status = ?", JobStatusQueued).
			Order("queue_position asc, priority asc, created_at asc, id asc").Find(&ordered).Error; err != nil {
			return err
		}
		draggedIndex, targetIndex := -1, -1
		for index := range ordered {
			if ordered[index].ID == input.JobID {
				draggedIndex = index
			}
			if ordered[index].ID == input.TargetJobID {
				targetIndex = index
			}
		}
		if draggedIndex < 0 || targetIndex < 0 {
			return gorm.ErrRecordNotFound
		}
		dragged := ordered[draggedIndex]
		ordered = append(ordered[:draggedIndex], ordered[draggedIndex+1:]...)
		targetIndex = -1
		for index := range ordered {
			if ordered[index].ID == input.TargetJobID {
				targetIndex = index
				break
			}
		}
		if input.Placement == "after" {
			targetIndex++
		}
		ordered = append(ordered, models.QueueJob{})
		copy(ordered[targetIndex+1:], ordered[targetIndex:])
		ordered[targetIndex] = dragged
		for index := range ordered {
			if err := tx.Model(&models.QueueJob{}).Where("id = ? AND status = ?", ordered[index].ID, JobStatusQueued).
				Update("queue_position", int64(index+1)).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusConflict, gin.H{"error": "only currently queued jobs can be reordered"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"jobs": ordered})
}

func (h QueueHandler) Dismiss(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job id"})
		return
	}
	var job models.QueueJob
	if err := h.db.First(&job, uint(id)).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if job.DismissedAt != nil {
		c.JSON(http.StatusOK, job)
		return
	}
	if job.Status == JobStatusRunning || job.Status == JobStatusCompleted {
		c.JSON(http.StatusConflict, gin.H{"error": "running or completed jobs cannot be removed from queue"})
		return
	}
	if job.ExecutionNumber == nil && job.StartedAt == nil {
		err = h.db.Transaction(func(tx *gorm.DB) error {
			if err := scheduler.ReleaseReservation(tx, job.ID); err != nil {
				return err
			}
			if err := tx.Where("job_id = ?", job.ID).Delete(&models.ExecutionPlan{}).Error; err != nil {
				return err
			}
			return tx.Delete(&job).Error
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"removed": true, "placeholderId": job.ID})
		return
	}

	now := time.Now()
	err = h.db.Transaction(func(tx *gorm.DB) error {
		job.DismissedAt = &now
		if err := tx.Save(&job).Error; err != nil {
			return err
		}
		return scheduler.ReleaseReservation(tx, job.ID)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, job)
}

func (h QueueHandler) DismissBatch(c *gin.Context) {
	batchID := strings.TrimSpace(c.Param("batchId"))
	if batchID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "batch id is required"})
		return
	}
	var jobs []models.QueueJob
	if err := h.db.Where("batch_id = ? AND dismissed_at IS NULL", batchID).Order("id asc").Find(&jobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(jobs) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "batch not found"})
		return
	}

	now := time.Now()
	removedPlaceholders := 0
	dismissedJobs := 0
	preservedCompleted := 0
	canceledProcesses := map[uint]bool{}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		for index := range jobs {
			job := &jobs[index]
			if job.Status == JobStatusCompleted {
				preservedCompleted++
				continue
			}
			if job.ExecutionNumber == nil && job.StartedAt == nil {
				if err := scheduler.ReleaseReservation(tx, job.ID); err != nil {
					return err
				}
				if err := tx.Where("job_id = ?", job.ID).Delete(&models.ExecutionPlan{}).Error; err != nil {
					return err
				}
				if err := tx.Delete(job).Error; err != nil {
					return err
				}
				removedPlaceholders++
				continue
			}
			if job.Status == JobStatusRunning {
				canceledProcesses[job.ID] = cancelRunningJobProcess(job.ID)
				job.Status = JobStatusCanceled
				job.FinishedAt = &now
				if err := transitionJobStage(tx, job, JobStageCanceled); err != nil {
					return err
				}
			}
			job.DismissedAt = &now
			if err := tx.Save(job).Error; err != nil {
				return err
			}
			if err := scheduler.ReleaseReservation(tx, job.ID); err != nil {
				return err
			}
			dismissedJobs++
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for _, job := range jobs {
		if job.Status == JobStatusCanceled && !canceledProcesses[job.ID] {
			_ = cleanupCanceledJob(h.db, job)
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"batchId":             batchID,
		"removedPlaceholders": removedPlaceholders,
		"dismissedJobs":       dismissedJobs,
		"preservedCompleted":  preservedCompleted,
	})
}

func queueUsesOverrideOnlyVideo(resolution models.JSONMap) bool {
	raw, ok := resolution["video"]
	if !ok {
		return false
	}

	video, ok := raw.(models.JSONMap)
	if !ok {
		if generic, genericOK := raw.(map[string]any); genericOK {
			return boolValue(generic["overrideOnly"], false)
		}
		return false
	}

	return boolValue(video["overrideOnly"], false)
}

func normalizeQueuedBatchOrder(db *gorm.DB, batchID string) error {
	batchID = strings.TrimSpace(batchID)
	if batchID == "" {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		return normalizeQueuedBatchOrderTx(tx, batchID)
	})
}

func normalizeQueuedBatchOrderTx(tx *gorm.DB, batchID string) error {
	var jobs []models.QueueJob

	if err := tx.
		Where(
			"batch_id = ? AND status = ? AND dismissed_at IS NULL",
			batchID,
			JobStatusQueued,
		).
		Order("queue_position asc, created_at asc, id asc").
		Find(&jobs).Error; err != nil {
		return err
	}

	if len(jobs) < 2 {
		return nil
	}

	// Preserve exactly the global queue slots already occupied by this batch.
	//
	// Example:
	// Batch A occupies 1,2,3
	// Batch B occupies 4,5,6
	//
	// Normalizing Batch B may change WHICH B job owns 4/5/6,
	// but must never move Batch B into 1/2/3.
	slots := make([]int64, len(jobs))
	for index := range jobs {
		slots[index] = jobs[index].QueuePosition
	}

	ordered := append([]models.QueueJob(nil), jobs...)

	sort.SliceStable(ordered, func(i, j int) bool {
		left := ordered[i]
		right := ordered[j]

		leftPosition := left.BatchPosition
		rightPosition := right.BatchPosition

		// Explicit batch positions are authoritative.
		if leftPosition > 0 || rightPosition > 0 {
			if leftPosition == 0 {
				return false
			}
			if rightPosition == 0 {
				return true
			}
			if leftPosition != rightPosition {
				return leftPosition < rightPosition
			}
		}

		// Defensive fallback for legacy/duplicate/missing batchPosition.
		if left.MediaPath != right.MediaPath {
			return assetSequenceLess(left.MediaPath, right.MediaPath)
		}

		return left.ID < right.ID
	})

	for index := range ordered {
		targetPosition := slots[index]

		if ordered[index].QueuePosition == targetPosition {
			continue
		}

		if err := tx.
			Model(&models.QueueJob{}).
			Where(
				"id = ? AND status = ? AND dismissed_at IS NULL",
				ordered[index].ID,
				JobStatusQueued,
			).
			Update("queue_position", targetPosition).
			Error; err != nil {
			return err
		}
	}

	return nil
}

func (h QueueHandler) prepareBatchQueueJob(
	input QueueJobInput,
) (models.QueueJob, int, error) {
	input.MediaPath = filepath.Clean(input.MediaPath)

	if review := reviewForPath(
		input.MediaPath,
		assetReviewOverrides(h.db),
	); review.RequiresReview {
		return models.QueueJob{},
			http.StatusConflict,
			fmt.Errorf("asset requires review before queueing")
	}

	if active, err := h.assetHasOpenJob(input.MediaPath, 0); err != nil {
		return models.QueueJob{}, http.StatusInternalServerError, err
	} else if active {
		return models.QueueJob{},
			http.StatusConflict,
			fmt.Errorf("asset already has an open queue job")
	}
	if active, err := activeAssetMaintenance(h.db, input.MediaPath); err != nil {
		return models.QueueJob{}, http.StatusInternalServerError, err
	} else if active {
		return models.QueueJob{}, http.StatusConflict, fmt.Errorf("asset has an active maintenance operation")
	}

	profileResolution := input.resolvedProfileResolution
	if profileResolution == nil {
		profileResolution = models.JSONMap{}
	}

	if input.ResolveProfileAssignments {
		var err error
		profileResolution, err = h.resolveProfileAssignments(&input)
		if err != nil {
			return models.QueueJob{},
				http.StatusInternalServerError,
				fmt.Errorf("resolve profile assignments: %w", err)
		}
	}
	if normalizeQueueProcessingMode(input.ProcessingMode) == ProcessingModeAudioOnly && input.ProfileID == 0 {
		var fallback models.Profile
		if err := h.db.Where("disabled = ?", false).Order("id asc").First(&fallback).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return models.QueueJob{}, http.StatusBadRequest, fmt.Errorf("no usable video profile is available for audio/track-only jobs")
			}
			return models.QueueJob{}, http.StatusInternalServerError, fmt.Errorf("resolve audio-only fallback profile: %w", err)
		}
		input.ProfileID = fallback.ID
	}

	publishMode := strings.TrimSpace(input.PublishMode)
	if publishMode == "" {
		publishMode = PublishModeStandard
	}

	if publishMode != PublishModeStandard &&
		publishMode != PublishModeReplaceLibrary {
		return models.QueueJob{},
			http.StatusBadRequest,
			fmt.Errorf("invalid publish mode")
	}

	processingMode := normalizeQueueProcessingMode(input.ProcessingMode)

	if strings.TrimSpace(input.ProcessingMode) != "" &&
		processingMode == "" {
		return models.QueueJob{},
			http.StatusBadRequest,
			fmt.Errorf("invalid processing mode")
	}

	if publishMode == PublishModeReplaceLibrary {
		var library models.Library

		if err := h.db.First(&library, input.LibraryID).Error; err != nil {
			return models.QueueJob{},
				http.StatusBadRequest,
				fmt.Errorf("library not found")
		}

		if !pathIsInside(input.MediaPath, library.DestinationPath) {
			return models.QueueJob{},
				http.StatusBadRequest,
				fmt.Errorf(
					"library replacement source must be inside the selected library destination",
				)
		}

		existing, err := h.activePublicationForSamePhysicalFile(
			input.MediaPath,
		)
		if err != nil {
			return models.QueueJob{},
				http.StatusInternalServerError,
				err
		}

		if existing != nil {
			return models.QueueJob{},
				http.StatusConflict,
				fmt.Errorf(
					"this physical Library file already has an active MVForge publication through another library path",
				)
		}
	}

	priority := input.Priority
	if priority == 0 {
		priority = 5
	}

	job := models.QueueJob{
		MediaPath:         input.MediaPath,
		PublishMode:       publishMode,
		LibraryID:         input.LibraryID,
		ProfileID:         input.ProfileID,
		AudioProfileKey:   input.AudioProfileKey,
		TrackProfileKey:   input.TrackProfileKey,
		ProfileResolution: profileResolution,
		ProcessingMode:    processingMode,
		Priority:          priority,
		Status:            JobStatusQueued,
		Stage:             JobStageQueued,
		Notes:             input.Notes,
	}

	if err := h.captureSupplementalProfiles(&job); err != nil {
		return models.QueueJob{}, http.StatusBadRequest, err
	}

	if queueUsesOverrideOnlyVideo(job.ProfileResolution) {
		if err := h.captureOverrideOnlyProfile(
			&job,
			"queue_batch_create_override_only",
		); err != nil {
			return models.QueueJob{}, http.StatusBadRequest, err
		}
	} else {
		if err := h.captureProfile(
			&job,
			input.ProfileID,
			"queue_batch_create",
		); err != nil {
			switch {
			case errors.Is(err, errQueueProfileDisabled):
				return models.QueueJob{}, http.StatusConflict, err

			case errors.Is(err, gorm.ErrRecordNotFound):
				return models.QueueJob{},
					http.StatusBadRequest,
					fmt.Errorf("profile not found")

			default:
				return models.QueueJob{},
					http.StatusBadRequest,
					err
			}
		}
	}

	return job, http.StatusOK, nil
}

func (h QueueHandler) persistPreparedQueueBatch(batchID, batchName string, prepared []models.QueueJob) error {
	return h.db.Transaction(func(tx *gorm.DB) error {
		var highest int64
		if err := tx.Model(&models.QueueJob{}).Select("COALESCE(MAX(queue_position), 0)").Scan(&highest).Error; err != nil {
			return err
		}

		for index := range prepared {
			job := &prepared[index]
			job.BatchID = batchID
			job.BatchName = batchName
			job.BatchPosition = index + 1
			job.QueuePosition = highest + int64(index) + 1

			if err := tx.Create(job).Error; err != nil {
				return err
			}
			if err := scheduler.LockQueuedAsset(tx, *job); err != nil {
				return err
			}
			if err := transitionJobStage(tx, job, JobStageQueued); err != nil {
				return err
			}
			if _, err := scheduler.CreatePendingExecutionPlan(tx, job, "Execution plan created from atomic queue batch"); err != nil {
				return err
			}
		}
		return nil
	})
}

func (h QueueHandler) CreateBatch(c *gin.Context) {
	assetMutationMu.Lock()
	defer assetMutationMu.Unlock()
	var input QueueBatchInput

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	input.BatchID = strings.TrimSpace(input.BatchID)
	input.BatchName = strings.TrimSpace(input.BatchName)

	if input.BatchID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "batch id is required",
		})
		return
	}

	if len(input.Jobs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "batch must contain at least one job",
		})
		return
	}

	//
	// A batch represents exactly one physical containing path.
	//
	groupPath := ""

	for index := range input.Jobs {
		current := filepath.Dir(
			filepath.Clean(input.Jobs[index].MediaPath),
		)

		if index == 0 {
			groupPath = current
			continue
		}

		if current != groupPath {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "all jobs in a batch must belong to the same path",
			})
			return
		}
	}

	//
	// Backend is authoritative for asset order.
	//
	jobsInput := append([]QueueJobInput(nil), input.Jobs...)

	sort.SliceStable(jobsInput, func(i, j int) bool {
		return assetSequenceLess(
			jobsInput[i].MediaPath,
			jobsInput[j].MediaPath,
		)
	})

	//
	// Detect exact duplicates before doing expensive profile preparation.
	//
	seenPaths := map[string]bool{}

	for _, item := range jobsInput {
		cleanPath := filepath.Clean(item.MediaPath)

		if seenPaths[cleanPath] {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf(
					"duplicate asset in batch: %s",
					cleanPath,
				),
			})
			return
		}

		seenPaths[cleanPath] = true
	}

	//
	// Prepare every job before opening the DB transaction.
	//
	// No queued job exists yet, so workers cannot see a partial batch.
	//
	prepared := make([]models.QueueJob, 0, len(jobsInput))

	for _, item := range jobsInput {
		job, status, err := h.prepareBatchQueueJob(item)
		if err != nil {
			c.JSON(status, gin.H{
				"error":     err.Error(),
				"mediaPath": item.MediaPath,
			})
			return
		}

		prepared = append(prepared, job)
	}

	//
	// Everything below is atomic.
	//
	err := h.persistPreparedQueueBatch(input.BatchID, input.BatchName, prepared)

	if err != nil {
		if errors.Is(err, scheduler.ErrAssetAlreadyReserved) {
			c.JSON(http.StatusConflict, gin.H{
				"error": "one or more assets already have an open queue job",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"batchId":   input.BatchID,
		"batchName": input.BatchName,
		"jobs":      prepared,
	})
}

type selectedQueueCandidate struct {
	record      models.AssetRecord
	job         models.QueueJob
	resultIndex int
	groupPath   string
	batchID     string
	batchName   string
}

func (h QueueHandler) QueueSelectedAssets(c *gin.Context) {
	var input QueueSelectedAssetsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(input.AssetIDs) == 0 || len(input.AssetIDs) > 1000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "assetIds must contain between 1 and 1000 assets"})
		return
	}

	assetIDs := make([]uint, 0, len(input.AssetIDs))
	seen := make(map[uint]bool, len(input.AssetIDs))
	for _, assetID := range input.AssetIDs {
		if assetID == 0 || seen[assetID] {
			continue
		}
		seen[assetID] = true
		assetIDs = append(assetIDs, assetID)
	}
	if len(assetIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "assetIds must contain a valid asset id"})
		return
	}

	if input.Commit {
		assetMutationMu.Lock()
		defer assetMutationMu.Unlock()
	}

	response, err := h.resolveSelectedAssetsQueue(assetIDs, input.Commit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h QueueHandler) resolveSelectedAssetsQueue(assetIDs []uint, commit bool) (QueueSelectedAssetsResponse, error) {
	response := QueueSelectedAssetsResponse{
		Results: make([]QueueSelectedAssetResult, 0, len(assetIDs)),
		Batches: []QueueSelectedAssetsBatchResult{},
	}
	response.Summary.Selected = len(assetIDs)

	var records []models.AssetRecord
	if err := h.db.Where("id IN ?", assetIDs).Find(&records).Error; err != nil {
		return response, fmt.Errorf("load selected assets: %w", err)
	}
	sort.SliceStable(records, func(i, j int) bool {
		leftLogical := filepath.Clean(records[i].LogicalGroupPath)
		rightLogical := filepath.Clean(records[j].LogicalGroupPath)
		if comparison := naturalAssetCompare(leftLogical, rightLogical); comparison != 0 {
			return comparison < 0
		}
		leftGroup := selectedAssetPhysicalGroup(records[i])
		rightGroup := selectedAssetPhysicalGroup(records[j])
		if comparison := naturalAssetCompare(leftGroup, rightGroup); comparison != 0 {
			return comparison < 0
		}
		return assetSequenceLess(records[i].Path, records[j].Path)
	})

	found := make(map[uint]bool, len(records))
	titles := map[string]bool{}
	candidates := make([]selectedQueueCandidate, 0, len(records))
	configurationResolver, err := loadAssetConfigurationResolver(h.db)
	if err != nil {
		return response, fmt.Errorf("load effective configuration batch: %w", err)
	}
	reviews := assetReviewOverrides(h.db)
	for _, record := range records {
		found[record.ID] = true
		titlePath := filepath.Clean(record.LogicalGroupPath)
		if titlePath == "." || strings.TrimSpace(record.LogicalGroupPath) == "" {
			titlePath = selectedAssetPhysicalGroup(record)
		}
		titles[titlePath] = true

		result := QueueSelectedAssetResult{AssetID: record.ID}
		if record.Status != "unprocessed" {
			result.Outcome, result.Reason, result.Message = "skipped", "not_unprocessed", "Asset is not currently Unprocessed"
			response.Results = append(response.Results, result)
			continue
		}
		if record.Missing {
			result.Outcome, result.Reason, result.Message = "skipped", "missing", "Asset file is missing"
			response.Results = append(response.Results, result)
			continue
		}
		if reviewForPath(filepath.Clean(record.Path), reviews).RequiresReview {
			result.Outcome, result.Reason, result.Message = "skipped", "needs_review", "Asset requires review before queueing"
			response.Results = append(response.Results, result)
			continue
		}
		if active, err := h.assetHasOpenJob(record.Path, 0); err != nil {
			return response, fmt.Errorf("check open queue job for asset %d: %w", record.ID, err)
		} else if active {
			result.Outcome, result.Reason, result.Message = "skipped", "already_queued", "Asset already has an open Queue job"
			response.Results = append(response.Results, result)
			continue
		}
		if active, err := activeAssetMaintenance(h.db, record.Path); err != nil {
			return response, fmt.Errorf("check maintenance for asset %d: %w", record.ID, err)
		} else if active {
			result.Outcome, result.Reason, result.Message = "skipped", "active_maintenance", "Asset has an active maintenance operation"
			response.Results = append(response.Results, result)
			continue
		}

		effective := configurationResolver.resolve(record)
		hasOperation := effective.Video.VideoProfileID > 0 ||
			effective.Video.Selection == VideoAssignmentOverrideOnly ||
			effective.Video.Selection == VideoAssignmentAudioOnly ||
			strings.TrimSpace(effective.Audio.ProfileKey) != "" ||
			strings.TrimSpace(effective.Tracks.ProfileKey) != ""
		if effective.Destination.Selection != configSelectionValue || effective.Destination.DestinationLibraryID == 0 || !hasOperation {
			result.Outcome, result.Reason = "failed", "invalid_configuration"
			if effective.Destination.Selection != configSelectionValue || effective.Destination.DestinationLibraryID == 0 {
				result.Message = "No effective destination is configured"
			} else {
				result.Message = "No effective processing operation is configured"
			}
			response.Results = append(response.Results, result)
			continue
		}

		processingMode := ProcessingModeAudioOnly
		if effective.Video.VideoProfileID > 0 || effective.Video.Selection == VideoAssignmentOverrideOnly {
			processingMode = ProcessingModeFullEncode
		}
		queueInput := QueueJobInput{
			MediaPath:                 record.Path,
			PublishMode:               PublishModeStandard,
			LibraryID:                 effective.Destination.DestinationLibraryID,
			ProfileID:                 effective.Video.VideoProfileID,
			AudioProfileKey:           effective.Audio.ProfileKey,
			TrackProfileKey:           effective.Tracks.ProfileKey,
			ProcessingMode:            processingMode,
			Priority:                  5,
			Notes:                     "Queued from selected titles",
			ResolveProfileAssignments: false,
		}
		queueInput.resolvedProfileResolution = resolveProfileAssignmentsFromEffective(&queueInput, effective)
		job, status, err := h.prepareBatchQueueJob(queueInput)
		if err != nil {
			result.Outcome, result.Reason, result.Message = "failed", "invalid_configuration", err.Error()
			if status >= http.StatusInternalServerError {
				result.Reason = "queue_creation_failed"
			}
			response.Results = append(response.Results, result)
			continue
		}

		result.Outcome = "eligible"
		response.Results = append(response.Results, result)
		candidates = append(candidates, selectedQueueCandidate{
			record:      record,
			job:         job,
			resultIndex: len(response.Results) - 1,
			groupPath:   selectedAssetPhysicalGroup(record),
		})
	}

	for _, assetID := range assetIDs {
		if found[assetID] {
			continue
		}
		response.Results = append(response.Results, QueueSelectedAssetResult{
			AssetID: assetID,
			Outcome: "failed",
			Reason:  "not_found",
			Message: "Asset record was not found",
		})
	}
	response.Summary.TitleCount = len(titles)
	response.Summary.Eligible = len(candidates)
	for _, candidate := range candidates {
		response.Summary.SizeBytes += candidate.record.SizeBytes
	}

	batchStamp := time.Now().UTC().UnixNano()
	groups := make([][]int, 0)
	for index := range candidates {
		if len(groups) == 0 || candidates[groups[len(groups)-1][0]].groupPath != candidates[index].groupPath {
			groups = append(groups, []int{index})
		} else {
			groups[len(groups)-1] = append(groups[len(groups)-1], index)
		}
	}
	for groupIndex, indexes := range groups {
		batchID := fmt.Sprintf("selected-%d-%03d", batchStamp, groupIndex+1)
		batchName := filepath.Base(candidates[indexes[0]].groupPath)
		if batchName == "." || batchName == string(filepath.Separator) || strings.TrimSpace(batchName) == "" {
			batchName = "Selected assets"
		}
		prepared := make([]models.QueueJob, 0, len(indexes))
		for _, candidateIndex := range indexes {
			candidate := &candidates[candidateIndex]
			candidate.batchID, candidate.batchName = batchID, batchName
			result := &response.Results[candidate.resultIndex]
			result.BatchID, result.BatchName = batchID, batchName
			prepared = append(prepared, candidate.job)
		}
		if !commit {
			continue
		}
		if err := h.persistPreparedQueueBatch(batchID, batchName, prepared); err != nil {
			reason := "queue_creation_failed"
			message := err.Error()
			if errors.Is(err, scheduler.ErrAssetAlreadyReserved) {
				reason = "reservation_conflict"
				message = "Asset became reserved before Queue creation completed"
			}
			for _, candidateIndex := range indexes {
				result := &response.Results[candidates[candidateIndex].resultIndex]
				result.Outcome, result.Reason, result.Message = "failed", reason, message
			}
			continue
		}
		response.Batches = append(response.Batches, QueueSelectedAssetsBatchResult{BatchID: batchID, BatchName: batchName, JobCount: len(prepared)})
		for preparedIndex, candidateIndex := range indexes {
			result := &response.Results[candidates[candidateIndex].resultIndex]
			result.Outcome, result.JobID = "queued", prepared[preparedIndex].ID
		}
	}

	for _, result := range response.Results {
		switch result.Outcome {
		case "queued":
			response.Summary.Queued++
		case "skipped":
			response.Summary.Skipped++
		case "failed":
			response.Summary.Failed++
		}
	}
	return response, nil
}

func selectedAssetPhysicalGroup(record models.AssetRecord) string {
	if strings.TrimSpace(record.SourcePath) != "" {
		return filepath.Dir(filepath.Clean(record.SourcePath))
	}
	return filepath.Dir(filepath.Clean(record.Path))
}

func (h QueueHandler) Create(c *gin.Context) {
	assetMutationMu.Lock()
	defer assetMutationMu.Unlock()
	var input QueueJobInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if review := reviewForPath(filepath.Clean(input.MediaPath), assetReviewOverrides(h.db)); review.RequiresReview {
		c.JSON(http.StatusConflict, gin.H{
			"error":  "asset requires review before queueing",
			"review": review,
		})
		return
	}
	if active, err := h.assetHasOpenJob(input.MediaPath, 0); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	} else if active {
		c.JSON(http.StatusConflict, gin.H{"error": "asset already has an open queue job"})
		return
	}
	if active, err := activeAssetMaintenance(h.db, input.MediaPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	} else if active {
		c.JSON(http.StatusConflict, gin.H{"error": "asset has an active maintenance operation"})
		return
	}
	profileResolution := models.JSONMap{}
	if input.ResolveProfileAssignments {
		var err error
		profileResolution, err = h.resolveProfileAssignments(&input)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "resolve profile assignments: " + err.Error()})
			return
		}
	}
	publishMode := input.PublishMode
	if publishMode == "" {
		publishMode = PublishModeStandard
	}
	if publishMode != PublishModeStandard && publishMode != PublishModeReplaceLibrary {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid publish mode"})
		return
	}
	processingMode := normalizeQueueProcessingMode(input.ProcessingMode)
	if strings.TrimSpace(input.ProcessingMode) != "" && processingMode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid processing mode"})
		return
	}
	if publishMode == PublishModeReplaceLibrary {
		var library models.Library
		if err := h.db.First(&library, input.LibraryID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "library not found"})
			return
		}
		if !pathIsInside(input.MediaPath, library.DestinationPath) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "library replacement source must be inside the selected library destination"})
			return
		}
		if existing, err := h.activePublicationForSamePhysicalFile(input.MediaPath); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		} else if existing != nil {
			c.JSON(http.StatusConflict, gin.H{
				"error":         "this physical Library file already has an active MVForge publication through another library path",
				"publishedPath": existing.PublishedPath,
				"jobId":         existing.ID,
			})
			return
		}
	}

	priority := input.Priority
	if priority == 0 {
		priority = 5
	}

	job := models.QueueJob{
		MediaPath:         input.MediaPath,
		PublishMode:       publishMode,
		BatchID:           input.BatchID,
		BatchName:         input.BatchName,
		BatchPosition:     max(0, input.BatchPosition),
		LibraryID:         input.LibraryID,
		ProfileID:         input.ProfileID,
		AudioProfileKey:   input.AudioProfileKey,
		TrackProfileKey:   input.TrackProfileKey,
		ProfileResolution: profileResolution,
		ProcessingMode:    processingMode,
		Priority:          priority,
		Status:            "queued",
		Stage:             JobStageQueued,
		Notes:             input.Notes,
	}
	if err := h.captureSupplementalProfiles(&job); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if queueUsesOverrideOnlyVideo(job.ProfileResolution) {
		if err := h.captureOverrideOnlyProfile(
			&job,
			"queue_create_override_only",
		); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": err.Error(),
			})
			return
		}
	} else {
		if err := h.captureProfile(
			&job,
			input.ProfileID,
			"queue_create",
		); err != nil {
			h.profileCaptureError(c, err)
			return
		}
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		var highest int64
		if err := tx.Model(&models.QueueJob{}).Select("COALESCE(MAX(queue_position), 0)").Scan(&highest).Error; err != nil {
			return err
		}
		job.QueuePosition = highest + 1
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		if err := scheduler.LockQueuedAsset(tx, job); err != nil {
			return err
		}
		if err := transitionJobStage(tx, &job, JobStageQueued); err != nil {
			return err
		}

		if _, err := scheduler.CreatePendingExecutionPlan(
			tx,
			&job,
			"Execution plan created from queued profile snapshot",
		); err != nil {
			return err
		}

		batchID := strings.TrimSpace(job.BatchID)
		if batchID != "" {
			if err := normalizeQueuedBatchOrderTx(tx, batchID); err != nil {
				return err
			}

			// Normalization may have changed this job's global queue slot.
			// Reload it so the API response contains the effective queuePosition.
			if err := tx.First(&job, job.ID).Error; err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		if errors.Is(err, scheduler.ErrAssetAlreadyReserved) {
			c.JSON(http.StatusConflict, gin.H{"error": "asset already has an open queue job"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, job)
}

func (h QueueHandler) resolveProfileAssignments(input *QueueJobInput) (models.JSONMap, error) {
	effective, err := effectiveAssetConfiguration(h.db, input.MediaPath)
	if err != nil {
		return nil, err
	}
	return resolveProfileAssignmentsFromEffective(input, effective), nil
}

func resolveProfileAssignmentsFromEffective(input *QueueJobInput, effective EffectiveAssetConfiguration) models.JSONMap {
	resolution := models.JSONMap{}
	for mediaType, value := range map[string]AssetConfigurationValue{"video": effective.Video, "audio": effective.Audio, "tracks": effective.Tracks} {
		if value.Source == "" || value.Selection == configSelectionInherit {
			continue
		}
		resolution[mediaType] = models.JSONMap{
			"source":         value.Source,
			"targetPath":     value.SourceKey,
			"selection":      value.Selection,
			"videoProfileId": value.VideoProfileID,
			"profileKey":     value.ProfileKey,
			"overrideOnly":   value.Selection == VideoAssignmentOverrideOnly,
		}
		switch mediaType {

		case "video":
			switch value.Selection {
			case "profile":
				if value.VideoProfileID > 0 {
					input.ProfileID = value.VideoProfileID
					input.ProcessingMode = ProcessingModeFullEncode
				}

			case VideoAssignmentOverrideOnly:
				input.ProfileID = 0
				input.ProcessingMode = ProcessingModeFullEncode

			case VideoAssignmentAudioOnly, "disabled":
				// Keep "disabled" for backward compatibility with existing
				// stored assignments, but new UI should use "audio_only".
				input.ProfileID = 0
				input.ProcessingMode = ProcessingModeAudioOnly
			}

		case "audio":
			if value.Selection == "profile" {
				input.AudioProfileKey = value.ProfileKey
			} else if value.Selection == "disabled" {
				input.AudioProfileKey = ""
			}
		case "tracks":
			if value.Selection == "profile" {
				input.TrackProfileKey = value.ProfileKey
			} else if value.Selection == "disabled" {
				input.TrackProfileKey = ""
			}
		}
	}
	for name, value := range map[string]AssetConfigurationValue{"category": effective.Category, "destination": effective.Destination} {
		resolution[name] = models.JSONMap{
			"source": value.Source, "targetPath": value.SourceKey, "selection": value.Selection,
			"category": value.Category, "libraryId": value.DestinationLibraryID,
		}
	}
	if effective.Destination.Selection == configSelectionValue && effective.Destination.DestinationLibraryID > 0 {
		input.LibraryID = effective.Destination.DestinationLibraryID
	}
	return resolution
}

func (h QueueHandler) captureSupplementalProfiles(job *models.QueueJob) error {
	if strings.TrimSpace(job.AudioProfileKey) != "" {
		profile, err := settingProfileByKey(h.db, "audioEnhancementProfiles", job.AudioProfileKey)
		if err != nil {
			return fmt.Errorf("audio profile: %w", err)
		}
		job.AudioProfileSnapshot = models.JSONMap(profile)
	}
	if strings.TrimSpace(job.TrackProfileKey) != "" {
		profile, err := settingProfileByKey(h.db, "trackProfiles", job.TrackProfileKey)
		if err != nil {
			return fmt.Errorf("track profile: %w", err)
		}
		resolved := resolveTrackProfileForAsset(h.db, job.MediaPath, profile)
		if storedSettingProfileScope(profile) == "path" && strings.TrimSpace(workerStringValue(resolved["resolvedForAsset"])) == "" {
			return fmt.Errorf("track profile: a cached asset snapshot is required to resolve a path profile safely")
		}
		var scan models.ScanResult
		if err := h.db.Where("path = ?", filepath.Clean(job.MediaPath)).Order("updated_at desc, id desc").First(&scan).Error; err != nil {
			return fmt.Errorf("track profile: asset snapshot: %w", err)
		}
		// Profiles persisted before the canonical disposition model keep using
		// the legacy Video Profile compatibility path. The three base policies
		// form an atomic migration marker; newly saved Track Profiles always
		// include all three and become authoritative for these decisions.
		if !trackProfileHasCanonicalDisposition(resolved) {
			job.TrackProfileSnapshot = models.JSONMap(resolved)
			return nil
		}
		trackPlan, err := resolveTrackPlan(scan, resolved)
		if err != nil {
			return fmt.Errorf("track profile: resolve track plan: %w", err)
		}
		trackPlanMap, err := resolvedTrackPlanMap(trackPlan)
		if err != nil {
			return fmt.Errorf("track profile: freeze track plan: %w", err)
		}
		resolved[resolvedTrackPlanSnapshotKey] = trackPlanMap
		job.TrackProfileSnapshot = models.JSONMap(resolved)
	}
	return nil
}

func trackProfileHasCanonicalDisposition(profile map[string]any) bool {
	for _, key := range []string{"subtitleDisposition", "attachmentPolicy", "chapterPolicy"} {
		if strings.TrimSpace(workerStringValue(profile[key])) == "" {
			return false
		}
	}
	return true
}

func resolveTrackProfileForAsset(db *gorm.DB, mediaPath string, profile map[string]any) map[string]any {
	var scan models.ScanResult
	if err := db.Where("path = ?", filepath.Clean(mediaPath)).Order("updated_at desc, id desc").First(&scan).Error; err != nil {
		return profile
	}
	scope := storedSettingProfileScope(profile)
	if scope == "path" {
		delete(profile, "keepVideoStreams")
		delete(profile, "keepAudioStreams")
		delete(profile, "keepSubtitleStreams")
		delete(profile, "videoMetadata")
		delete(profile, "audioMetadata")
		delete(profile, "subtitleMetadata")
		delete(profile, "subtitleTransforms")
	}
	if _, exists := profile["keepVideoStreams"]; !exists {
		indexes := snapshotStreamIndexes(scan.VideoStreams, nil)
		if workerStringValue(profile["videoMode"]) != "all" && len(indexes) > 1 {
			indexes = indexes[:1]
		}
		profile["keepVideoStreams"] = indexes
	}
	if _, exists := profile["keepAudioStreams"]; !exists {
		mode := workerStringValue(profile["audioMode"])
		languages := stringSet(profile["audioLanguages"])
		indexes := snapshotStreamIndexes(scan.AudioStreams, func(stream map[string]any) bool {
			if mode == "none" {
				return false
			}
			if boolValue(profile["dropCommentary"], true) && (boolValue(stream["comment"], false) || strings.Contains(strings.ToLower(workerStringValue(stream["title"])), "commentary")) {
				return false
			}
			if mode == "default" {
				return boolValue(stream["default"], false)
			}
			if mode == "languages" {
				return languages[strings.ToLower(workerStringValue(stream["language"]))]
			}
			return true
		})
		if mode == "default" && len(indexes) == 0 {
			all := snapshotStreamIndexes(scan.AudioStreams, nil)
			if len(all) > 0 {
				indexes = all[:1]
			}
		}
		profile["keepAudioStreams"] = indexes
	}
	if _, exists := profile["keepSubtitleStreams"]; !exists {
		mode := workerStringValue(profile["subtitleMode"])
		languages := stringSet(profile["subtitleLanguages"])
		profile["keepSubtitleStreams"] = snapshotStreamIndexes(scan.SubtitleStreams, func(stream map[string]any) bool {
			forced := boolValue(stream["forced"], false)
			languageMatch := languages[strings.ToLower(workerStringValue(stream["language"]))]
			switch mode {
			case "none":
				return false
			case "forced":
				return forced
			case "languages":
				return languageMatch
			case "forced-or-languages":
				return forced || languageMatch
			default:
				return true
			}
		})
	}
	if scope == "path" {
		if metadata := defaultLanguageMetadata(scan.AudioStreams, profile["keepAudioStreams"], workerStringValue(profile["defaultAudioLanguage"])); len(metadata) > 0 {
			profile["audioMetadata"] = metadata
		}
		if metadata := defaultLanguageMetadata(scan.SubtitleStreams, profile["keepSubtitleStreams"], workerStringValue(profile["defaultSubtitleLanguage"])); len(metadata) > 0 {
			profile["subtitleMetadata"] = metadata
		}
	}
	profile["resolvedForAsset"] = filepath.Clean(mediaPath)
	return profile
}

func defaultLanguageMetadata(streams models.JSONList, selected any, language string) map[string]any {
	language = strings.ToLower(strings.TrimSpace(language))
	if language == "" {
		return nil
	}
	selectedIndexes := map[int]bool{}
	for _, value := range workerSliceValue(selected) {
		if index := streamIndexValue(value); index >= 0 {
			selectedIndexes[index] = true
		}
	}
	defaultIndex := -1
	for _, raw := range streams {
		stream := settingProfileObject(raw)
		index := streamIndexValue(stream["index"])
		if selectedIndexes[index] && strings.EqualFold(strings.TrimSpace(workerStringValue(stream["language"])), language) {
			defaultIndex = index
			break
		}
	}
	if defaultIndex < 0 {
		return nil
	}
	metadata := map[string]any{}
	for index := range selectedIndexes {
		metadata[strconv.Itoa(index)] = map[string]any{"default": index == defaultIndex}
	}
	return metadata
}

func workerSliceValue(value any) []any {
	switch values := value.(type) {
	case []any:
		return values
	case []int:
		result := make([]any, len(values))
		for index := range values {
			result[index] = values[index]
		}
		return result
	case models.JSONList:
		return []any(values)
	default:
		return nil
	}
}

func snapshotStreamIndexes(streams models.JSONList, keep func(map[string]any) bool) []int {
	indexes := []int{}
	for _, raw := range streams {
		var stream map[string]any
		switch value := raw.(type) {
		case map[string]any:
			stream = value
		case models.JSONMap:
			stream = map[string]any(value)
		default:
			continue
		}
		if keep != nil && !keep(stream) {
			continue
		}
		index := streamIndexValue(stream["index"])
		if index >= 0 {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

func streamIndexValue(value any) int {
	number := workerNumberValue(value, -1)
	if number < 0 {
		return -1
	}
	return int(number)
}

func stringSet(value any) map[string]bool {
	result := map[string]bool{}
	for _, item := range workerStringSlice(value) {
		result[strings.ToLower(strings.TrimSpace(item))] = true
	}
	return result
}

func boolValue(value any, fallback bool) bool {
	parsed, ok := value.(bool)
	if !ok {
		return fallback
	}
	return parsed
}

func settingProfileByKey(db *gorm.DB, settingKey, profileKey string) (map[string]any, error) {
	var setting models.AppSetting
	if err := db.Where("key = ?", settingKey).First(&setting).Error; err != nil {
		return nil, gorm.ErrRecordNotFound
	}
	profiles := settingProfileValues(setting.Value["profiles"])
	for _, raw := range profiles {
		profile := settingProfileObject(raw)
		if profile != nil && strings.TrimSpace(stringFromUnknown(profile["key"])) == strings.TrimSpace(profileKey) {
			copy := map[string]any{}
			for key, value := range profile {
				copy[key] = value
			}
			return copy, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (h QueueHandler) activePublicationForSamePhysicalFile(mediaPath string) (*models.QueueJob, error) {
	requested, err := os.Stat(filepath.Clean(mediaPath))
	if err != nil {
		return nil, err
	}
	var publications []models.QueueJob
	if err := h.db.Where("publication_retired_at IS NULL AND published_path <> ''").Order("id desc").Find(&publications).Error; err != nil {
		return nil, err
	}
	for index := range publications {
		candidatePath := filepath.Clean(strings.TrimSpace(publications[index].PublishedPath))
		if candidatePath == "." {
			continue
		}
		candidate, statErr := os.Stat(candidatePath)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return nil, statErr
		}
		if os.SameFile(requested, candidate) {
			return &publications[index], nil
		}
	}
	return nil, nil
}

func (h QueueHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job id"})
		return
	}

	var input QueueJobUpdateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var job models.QueueJob
	if err := h.db.First(&job, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if input.Priority != nil {
		job.Priority = clamp(*input.Priority, 1, 10)
	}
	if queueJobConfigChanged(input) && job.Status != JobStatusQueued && job.Status != JobStatusFailed && job.Status != JobStatusCanceled {
		c.JSON(http.StatusConflict, gin.H{"error": "only queued, failed, or canceled jobs can be edited"})
		return
	}
	mediaPathChanged := input.MediaPath != "" && input.MediaPath != job.MediaPath
	if mediaPathChanged {
		if active, err := h.assetHasOpenJob(input.MediaPath, job.ID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		} else if active {
			c.JSON(http.StatusConflict, gin.H{"error": "asset already has an open queue job"})
			return
		}
		if review := reviewForPath(filepath.Clean(input.MediaPath), assetReviewOverrides(h.db)); review.RequiresReview {
			c.JSON(http.StatusConflict, gin.H{
				"error":  "asset requires review before queueing",
				"review": review,
			})
			return
		}
		job.MediaPath = input.MediaPath
	}
	if input.LibraryID != nil {
		job.LibraryID = *input.LibraryID
	}
	if job.PublishMode == PublishModeReplaceLibrary && (mediaPathChanged || input.LibraryID != nil) {
		var library models.Library
		if err := h.db.First(&library, job.LibraryID).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "library not found"})
			return
		}
		if !pathIsInside(job.MediaPath, library.DestinationPath) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "library replacement source must remain inside the selected library destination"})
			return
		}
	}
	if input.AudioProfileKey != nil {
		job.AudioProfileKey = *input.AudioProfileKey
		job.AudioProfileSnapshot = nil
	}
	if input.TrackProfileKey != nil {
		job.TrackProfileKey = *input.TrackProfileKey
		job.TrackProfileSnapshot = nil
	}
	if input.AudioProfileKey != nil || input.TrackProfileKey != nil {
		if err := h.captureSupplementalProfiles(&job); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	if input.ProfileID != nil || input.TrackProfileKey != nil {
		profileID := job.ProfileID
		if input.ProfileID != nil {
			profileID = *input.ProfileID
		}
		if err := h.captureProfile(&job, profileID, "queue_profile_update"); err != nil {
			h.profileCaptureError(c, err)
			return
		}
	}
	if input.ProcessingMode != nil {
		mode := normalizeQueueProcessingMode(*input.ProcessingMode)
		if strings.TrimSpace(*input.ProcessingMode) != "" && mode == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid processing mode"})
			return
		}
		job.ProcessingMode = mode
	}

	if input.Notes != "" {
		job.Notes = input.Notes
	}

	canceledRunningProcess := false
	previousStatus := job.Status
	if input.Status != "" {
		if !validJobStatus(input.Status) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job status"})
			return
		}

		now := time.Now()
		switch input.Status {
		case JobStatusQueued:
			job.Status = JobStatusQueued
			job.Progress = 0
			job.WorkerName = ""
			job.ErrorMessage = ""
			job.StartedAt = nil
			job.FinishedAt = nil
		case JobStatusCanceled:
			canceledRunningProcess = cancelRunningJobProcess(job.ID)
			job.Status = JobStatusCanceled
			job.FinishedAt = &now
		default:
			job.Status = input.Status
		}
	}

	refreshExecutionPlan := input.ProfileID != nil || input.Status == JobStatusQueued
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if input.Status == JobStatusQueued && previousStatus != JobStatusQueued {
			var highest int64
			if err := tx.Model(&models.QueueJob{}).Select("COALESCE(MAX(queue_position), 0)").Scan(&highest).Error; err != nil {
				return err
			}
			job.QueuePosition = highest + 1
		}
		if err := tx.Save(&job).Error; err != nil {
			return err
		}
		if input.Status != "" {
			if err := transitionJobStage(tx, &job, terminalStageForStatus(job.Status)); err != nil {
				return err
			}
		}
		if mediaPathChanged || input.Status == JobStatusQueued {
			if err := scheduler.UpdateAssetLock(tx, job); err != nil {
				return err
			}
		}
		if job.Status == JobStatusCanceled {
			if err := scheduler.ReleaseReservation(tx, job.ID); err != nil {
				return err
			}
		}
		if refreshExecutionPlan {
			_, err := scheduler.CreatePendingExecutionPlan(tx, &job, "Execution plan replaced after explicit profile update")
			return err
		}
		return nil
	}); err != nil {
		if errors.Is(err, scheduler.ErrAssetAlreadyReserved) {
			c.JSON(http.StatusConflict, gin.H{"error": "asset already has an open queue job"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if job.Status == JobStatusCanceled && !canceledRunningProcess {
		_ = cleanupCanceledJob(h.db, job)
	}

	c.JSON(http.StatusOK, job)
}

func (h QueueHandler) captureOverrideOnlyProfile(
	job *models.QueueJob,
	source string,
) error {
	override := currentConversionOverrideForJob(
		*job,
		assetConversionOverrides(h.db),
	)
	if err := h.freezeEffectiveTrackPlan(job, &override); err != nil {
		return err
	}

	if assetConversionOverrideEmpty(override) {
		return fmt.Errorf(
			"override-only video selection requires an asset conversion override",
		)
	}

	// Neutral baseline.
	//
	// This profile is intentionally not loaded from any persisted profile.
	// The asset override becomes the authoritative video configuration.
	profile := models.Profile{
		ID:             0,
		Name:           "Asset Override Only",
		ProfileVersion: 1,

		// MKV is the safest neutral container for MVForge because it can
		// preserve arbitrary audio/subtitle streams without inheriting a
		// container choice from the path profile.
		Container: "mkv",

		// Start from copy semantics. Asset video overrides are then applied
		// explicitly below.
		VideoCodec: "copy",
		AudioCodec: "copy",

		QualityMode:  "crf",
		QualityValue: 24,

		PreserveHDR:       true,
		PreserveSubtitles: true,
		PreserveChapters:  true,

		WorkerConfig: models.JSONMap{},
	}

	profile = applyAssetConversionOverrideToProfile(profile, override)

	// A full_encode override-only job must end up with an actual video
	// configuration. Do not silently inherit any persisted profile.
	videoEncoder := strings.TrimSpace(
		workerStringValue(profile.WorkerConfig["videoEncoder"]),
	)
	videoCodec := strings.TrimSpace(profile.VideoCodec)

	if videoEncoder == "" && (videoCodec == "" || videoCodec == "copy") {
		return fmt.Errorf(
			"override-only video selection requires videoCodec or videoEncoder in the asset override",
		)
	}

	// Asset-level encoder choice must remain authoritative for scheduler
	// planning, just like captureProfile().
	if strings.TrimSpace(override.PreferredEncoder) != "" ||
		strings.TrimSpace(override.VideoEncoder) != "" ||
		strings.TrimSpace(override.VideoCodec) != "" ||
		override.UseHardwareIfAvailable != nil {

		profile.EncoderPolicy = ""
		profile.PreferredEncoder = ""
		profile.AllowedEncoders = nil
		profile.FallbackPolicy = ""
		profile.CodecFamily = ""
		profile.BitDepth = 0
		profile.PixelFormat = ""
		profile.QualityStrategy = ""
	}

	now := time.Now()

	snapshot, err := scheduler.CaptureProfileSnapshot(
		profile,
		now,
		source,
	)
	if err != nil {
		return err
	}

	// Freeze the asset override into the same snapshot mechanism used by
	// normal profiles. The worker will therefore execute the exact override
	// that existed when the job was queued.
	encoded, err := json.Marshal(override)
	if err != nil {
		return err
	}

	var frozen map[string]any
	if err := json.Unmarshal(encoded, &frozen); err != nil {
		return err
	}

	snapshot[assetConversionOverrideSnapshotKey] = frozen
	snapshot["overrideOnly"] = true
	h.captureInterlaceSnapshot(job.MediaPath, snapshot)

	job.ProfileID = 0
	job.ProfileVersion = 1
	job.ProfileSnapshot = snapshot
	job.ProfileCapturedAt = &now

	return nil
}

var errQueueProfileDisabled = errors.New("profile is disabled")

func (h QueueHandler) captureProfile(job *models.QueueJob, profileID uint, source string) error {
	var profile models.Profile
	if err := h.db.First(&profile, profileID).Error; err != nil {
		return err
	}
	if profile.Disabled {
		return errQueueProfileDisabled
	}
	override := currentConversionOverrideForJob(*job, assetConversionOverrides(h.db))
	if err := h.freezeEffectiveTrackPlan(job, &override); err != nil {
		return err
	}
	if !assetConversionOverrideEmpty(override) {
		profile = applyAssetConversionOverrideToProfile(profile, override)
	}
	if strings.TrimSpace(override.PreferredEncoder) != "" ||
		strings.TrimSpace(override.VideoEncoder) != "" ||
		strings.TrimSpace(override.VideoCodec) != "" ||
		override.UseHardwareIfAvailable != nil {
		// Asset processing preference must participate in immutable scheduler
		// planning. Clear the profile's authoritative encoder contract so the
		// effective worker configuration is resolved into the job snapshot.
		profile.EncoderPolicy = ""
		profile.PreferredEncoder = ""
		profile.AllowedEncoders = nil
		profile.FallbackPolicy = ""
		profile.CodecFamily = ""
		profile.BitDepth = 0
		profile.PixelFormat = ""
		profile.QualityStrategy = ""
	}
	now := time.Now()
	snapshot, err := scheduler.CaptureProfileSnapshot(profile, now, source)
	if err != nil {
		return err
	}
	if !assetConversionOverrideEmpty(override) {
		encoded, encodeErr := json.Marshal(override)
		if encodeErr != nil {
			return encodeErr
		}
		var frozen map[string]any
		if decodeErr := json.Unmarshal(encoded, &frozen); decodeErr != nil {
			return decodeErr
		}
		snapshot[assetConversionOverrideSnapshotKey] = frozen
	}
	h.captureInterlaceSnapshot(job.MediaPath, snapshot)
	job.ProfileID = profile.ID
	job.ProfileVersion = max(profile.ProfileVersion, 1)
	job.ProfileSnapshot = snapshot
	job.ProfileCapturedAt = &now
	return nil
}

func (h QueueHandler) freezeEffectiveTrackPlan(job *models.QueueJob, override *AssetConversionOverrideState) error {
	if job == nil || override == nil || len(job.TrackProfileSnapshot) == 0 {
		return nil
	}
	if _, ok := ResolvedTrackPlanFromSnapshot(job.TrackProfileSnapshot); !ok {
		return nil
	}
	profile := map[string]any{}
	for key, value := range job.TrackProfileSnapshot {
		if key != resolvedTrackPlanSnapshotKey {
			profile[key] = value
		}
	}
	profile["keepVideoStreams"] = override.KeepVideoStreams
	profile["keepAudioStreams"] = override.KeepAudioStreams
	profile["keepSubtitleStreams"] = override.KeepSubtitleStreams
	profile["videoMetadata"] = override.VideoMetadata
	profile["audioMetadata"] = override.AudioMetadata
	profile["subtitleMetadata"] = override.SubtitleMetadata
	profile["subtitleTransforms"] = override.SubtitleTransforms
	var scan models.ScanResult
	if err := h.db.Where("path = ?", filepath.Clean(job.MediaPath)).Order("updated_at desc, id desc").First(&scan).Error; err != nil {
		return fmt.Errorf("track profile: refresh effective track plan from asset snapshot: %w", err)
	}
	plan, err := resolveTrackPlan(scan, profile)
	if err != nil {
		return fmt.Errorf("track profile: resolve effective track plan: %w", err)
	}
	planMap, err := resolvedTrackPlanMap(plan)
	if err != nil {
		return fmt.Errorf("track profile: freeze effective track plan: %w", err)
	}
	job.TrackProfileSnapshot[resolvedTrackPlanSnapshotKey] = planMap
	override.ResolvedTrackPlan = &plan
	return nil
}

func (h QueueHandler) captureInterlaceSnapshot(path string, snapshot models.JSONMap) {
	if h.db == nil || snapshot == nil || strings.TrimSpace(path) == "" || !h.db.Migrator().HasTable(&models.ScanResult{}) {
		return
	}
	var scan models.ScanResult
	if err := h.db.Where("path = ?", path).Order("updated_at desc").First(&scan).Error; err != nil {
		return
	}
	if _, ok := decodeInterlaceAnalysis(scan.InterlaceAnalysis); ok {
		snapshot[interlaceAnalysisSnapshotKey] = scan.InterlaceAnalysis
	}
	if analysis, ok := decodeCadenceAnalysis(scan.CadenceAnalysis); ok {
		snapshot[cadenceAnalysisSnapshotKey] = scan.CadenceAnalysis
		if _, recommendationOK := decodeCadenceRecommendation(scan.CadenceRecommendation); !recommendationOK {
			snapshot[cadenceRecommendationSnapshotKey] = cadenceRecommendationMap(recommendCadence(analysis))
		}
	}
	if _, ok := decodeCadenceRecommendation(scan.CadenceRecommendation); ok {
		snapshot[cadenceRecommendationSnapshotKey] = scan.CadenceRecommendation
	}
}

func (h QueueHandler) profileCaptureError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusBadRequest, gin.H{"error": "profile not found"})
	case errors.Is(err, errQueueProfileDisabled):
		c.JSON(http.StatusConflict, gin.H{"error": "profile is disabled"})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
}

func queueJobConfigChanged(input QueueJobUpdateInput) bool {
	return input.MediaPath != "" || input.LibraryID != nil || input.ProfileID != nil || input.AudioProfileKey != nil || input.TrackProfileKey != nil || input.ProcessingMode != nil
}

func normalizeQueueProcessingMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "full_encode", "full-encode", "fullencode":
		return ProcessingModeFullEncode
	case "audio_only", "audio-only", "audioonly":
		return ProcessingModeAudioOnly
	default:
		return ""
	}
}

func (h QueueHandler) assetHasOpenJob(mediaPath string, excludeID uint) (bool, error) {
	cleanPath := filepath.Clean(mediaPath)
	var jobs []models.QueueJob
	query := h.db.
		Where("dismissed_at IS NULL").
		Where("status IN ?", []string{JobStatusQueued, JobStatusRunning, JobStatusCompleted}).
		Where("published_at IS NULL")
	if excludeID != 0 {
		query = query.Where("id <> ?", excludeID)
	}
	if err := query.Find(&jobs).Error; err != nil {
		return false, err
	}
	requested, requestedErr := os.Stat(cleanPath)
	for _, job := range jobs {
		candidatePath := filepath.Clean(job.MediaPath)
		if candidatePath == cleanPath {
			return true, nil
		}
		if requestedErr != nil {
			continue
		}
		candidate, err := os.Stat(candidatePath)
		if err == nil && os.SameFile(requested, candidate) {
			return true, nil
		}
	}
	return false, nil
}
