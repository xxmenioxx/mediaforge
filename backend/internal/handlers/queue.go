package handlers

import (
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"github.com/anuelvs/mediaforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type QueueHandler struct {
	db *gorm.DB
}

type QueueJobInput struct {
	MediaPath       string `json:"mediaPath" binding:"required"`
	PublishMode     string `json:"publishMode"`
	BatchID         string `json:"batchId"`
	BatchName       string `json:"batchName"`
	LibraryID       uint   `json:"libraryId" binding:"required"`
	ProfileID       uint   `json:"profileId" binding:"required"`
	AudioProfileKey string `json:"audioProfileKey"`
	Priority        int    `json:"priority"`
	Notes           string `json:"notes"`
}

type QueueJobUpdateInput struct {
	MediaPath       string  `json:"mediaPath"`
	LibraryID       *uint   `json:"libraryId"`
	ProfileID       *uint   `json:"profileId"`
	AudioProfileKey *string `json:"audioProfileKey"`
	Priority        *int    `json:"priority"`
	Status          string  `json:"status"`
	Notes           string  `json:"notes"`
}

func NewQueueHandler(db *gorm.DB) QueueHandler {
	return QueueHandler{db: db}
}

func (h QueueHandler) List(c *gin.Context) {
	var jobs []models.QueueJob
	if err := h.db.Order("priority asc, created_at asc").Find(&jobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, jobs)
}

func (h QueueHandler) Create(c *gin.Context) {
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
	publishMode := input.PublishMode
	if publishMode == "" {
		publishMode = PublishModeStandard
	}
	if publishMode != PublishModeStandard && publishMode != PublishModeReplaceLibrary {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid publish mode"})
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
	}

	priority := input.Priority
	if priority == 0 {
		priority = 5
	}

	job := models.QueueJob{
		MediaPath:       input.MediaPath,
		PublishMode:     publishMode,
		BatchID:         input.BatchID,
		BatchName:       input.BatchName,
		LibraryID:       input.LibraryID,
		ProfileID:       input.ProfileID,
		AudioProfileKey: input.AudioProfileKey,
		Priority:        priority,
		Status:          "queued",
		Stage:           JobStageQueued,
		Notes:           input.Notes,
	}
	if err := h.captureProfile(&job, input.ProfileID, "queue_create"); err != nil {
		h.profileCaptureError(c, err)
		return
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		if err := scheduler.LockQueuedAsset(tx, job); err != nil {
			return err
		}
		if err := transitionJobStage(tx, &job, JobStageQueued); err != nil {
			return err
		}
		_, err := scheduler.CreatePendingExecutionPlan(tx, &job, "Execution plan created from queued profile snapshot")
		return err
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
	if input.ProfileID != nil {
		if err := h.captureProfile(&job, *input.ProfileID, "queue_profile_update"); err != nil {
			h.profileCaptureError(c, err)
			return
		}
	}
	if input.AudioProfileKey != nil {
		job.AudioProfileKey = *input.AudioProfileKey
	}

	if input.Notes != "" {
		job.Notes = input.Notes
	}

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
			cancelRunningJobProcess(job.ID)
			job.Status = JobStatusCanceled
			job.FinishedAt = &now
		default:
			job.Status = input.Status
		}
	}

	refreshExecutionPlan := input.ProfileID != nil || input.Status == JobStatusQueued
	if err := h.db.Transaction(func(tx *gorm.DB) error {
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

	c.JSON(http.StatusOK, job)
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
	now := time.Now()
	snapshot, err := scheduler.CaptureProfileSnapshot(profile, now, source)
	if err != nil {
		return err
	}
	job.ProfileID = profile.ID
	job.ProfileVersion = max(profile.ProfileVersion, 1)
	job.ProfileSnapshot = snapshot
	job.ProfileCapturedAt = &now
	return nil
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
	return input.MediaPath != "" || input.LibraryID != nil || input.ProfileID != nil || input.AudioProfileKey != nil
}

func (h QueueHandler) assetHasOpenJob(mediaPath string, excludeID uint) (bool, error) {
	cleanPath := filepath.Clean(mediaPath)
	var count int64
	query := h.db.Model(&models.QueueJob{}).
		Where("media_path = ?", cleanPath).
		Where("status <> ?", JobStatusCanceled).
		Where("published_at IS NULL")
	if excludeID != 0 {
		query = query.Where("id <> ?", excludeID)
	}
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
