package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h QueueHandler) requestCancellation(c *gin.Context, jobID uint) {
	var job models.QueueJob
	pending := false
	terminal := false
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&job, jobID).Error; err != nil {
			return err
		}
		if job.DismissedAt != nil {
			return queueValidationDismissalConflict("dismissed Queue jobs cannot be canceled")
		}
		if job.Status == JobStatusCanceled {
			terminal = true
			return nil
		}
		now := time.Now().UTC()
		switch job.Status {
		case JobStatusQueued:
			result := tx.Model(&models.QueueJob{}).
				Where("id = ? AND status = ? AND dismissed_at IS NULL AND updated_at = ?", job.ID, JobStatusQueued, job.UpdatedAt).
				Updates(map[string]any{"status": JobStatusCanceled, "finished_at": now})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return queueValidationDismissalConflict("Queue job changed while cancellation was being requested")
			}
			job.Status = JobStatusCanceled
			job.FinishedAt = &now
			if err := transitionJobStage(tx, &job, JobStageCanceled); err != nil {
				return err
			}
			if err := scheduler.ReleaseReservation(tx, job.ID); err != nil {
				return err
			}
			terminal = true
			return nil
		case JobStatusRunning:
			if job.CancellationRequestedAt == nil {
				job.CancellationRequestedAt = &now
				result := tx.Model(&models.QueueJob{}).
					Where("id = ? AND status = ? AND dismissed_at IS NULL AND cancellation_requested_at IS NULL AND updated_at = ?", job.ID, JobStatusRunning, job.UpdatedAt).
					Update("cancellation_requested_at", now)
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return queueValidationDismissalConflict("Queue job changed while cancellation was being requested")
				}
			}
			pending = true
			return tx.First(&job, job.ID).Error
		default:
			return queueValidationDismissalConflict("only queued or running jobs can be canceled")
		}
	})
	if err != nil {
		if errorsIsQueueCancellationConflict(err) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if pending {
		cancelRunningJobProcess(job.ID)
	}
	if terminal {
		scheduleCanceledJobCleanup(h.db, job)
	}
	c.JSON(http.StatusOK, job)
}

func errorsIsQueueCancellationConflict(err error) bool {
	return errors.Is(err, errQueueValidationDismissalConflict)
}

func finalizeQueueJobCancellation(db *gorm.DB, jobID uint) (models.QueueJob, bool, error) {
	var job models.QueueJob
	finalized := false
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&job, jobID).Error; err != nil {
			return err
		}
		if job.Status != JobStatusCanceled && job.CancellationRequestedAt == nil {
			return nil
		}
		wasCanceled := job.Status == JobStatusCanceled
		job.Status = JobStatusCanceled
		if job.FinishedAt == nil {
			now := time.Now().UTC()
			job.FinishedAt = &now
		}
		if err := tx.Save(&job).Error; err != nil {
			return err
		}
		if !wasCanceled {
			if err := transitionJobStage(tx, &job, JobStageCanceled); err != nil {
				return err
			}
		}
		if err := scheduler.ReleaseReservation(tx, job.ID); err != nil {
			return err
		}
		finalized = true
		return nil
	})
	if err != nil {
		return job, false, err
	}
	if finalized {
		scheduleCanceledJobCleanup(db, job)
	}
	return job, finalized, nil
}
