package handlers

import (
	"fmt"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
)

const (
	JobStageQueued             = "queued"
	JobStageClaimed            = "claimed"
	JobStagePreparingWorkspace = "preparing_workspace"
	JobStageCopyingWorkspace   = "copying_to_workspace"
	JobStageAnalyzingAsIs      = "analyzing_as_is"
	JobStagePreparingSubtitles = "preparing_subtitles"
	JobStageConverting         = "converting"
	JobStageValidating         = "validating"
	JobStageValidationReview   = "validation_review"
	JobStageDirectPlayAnalysis = "directplay_analysis"
	JobStageReadyToPublish     = "ready_to_publish"
	JobStagePublishing         = "publishing"
	JobStageArchivingOriginal  = "archiving_original"
	JobStageAnalyzingFinal     = "analyzing_final"
	JobStageCleaningWorkspace  = "cleaning_workspace"
	JobStageCompleted          = "completed"
	JobStageFailed             = "failed"
	JobStageCanceled           = "canceled"
)

func transitionJobStage(db *gorm.DB, job *models.QueueJob, stage string) error {
	if job == nil {
		return fmt.Errorf("queue job is required for stage transition to %q", stage)
	}
	if job.ID == 0 {
		if job.Stage == stage && job.StageUpdatedAt != nil {
			return nil
		}
		now := time.Now()
		job.Stage = stage
		job.StageUpdatedAt = &now
		job.StageHistory = append(job.StageHistory, models.JSONMap{"stage": stage, "at": now.UTC().Format(time.RFC3339)})
		return nil
	}
	if db == nil {
		return fmt.Errorf("database is required for queue job %d stage transition to %q", job.ID, stage)
	}

	expectedStage, expectedUpdatedAt := job.Stage, job.StageUpdatedAt
	hasExpectedStage := expectedStage != "" || expectedUpdatedAt != nil || len(job.StageHistory) > 0
	var nextStage string
	var nextUpdatedAt *time.Time
	var nextHistory models.JSONList
	err := db.Transaction(func(tx *gorm.DB) error {
		var current models.QueueJob
		if err := tx.Select("id", "stage", "stage_updated_at", "stage_history").First(&current, job.ID).Error; err != nil {
			return err
		}
		if current.Stage == stage && current.StageUpdatedAt != nil {
			nextStage, nextUpdatedAt, nextHistory = current.Stage, current.StageUpdatedAt, current.StageHistory
			return nil
		}
		if hasExpectedStage && (expectedStage != current.Stage || !sameStageTimestamp(expectedUpdatedAt, current.StageUpdatedAt)) {
			return fmt.Errorf("queue job %d stage transition to %q conflicted with persisted stage %q", job.ID, stage, current.Stage)
		}

		now := time.Now()
		history := append(models.JSONList(nil), current.StageHistory...)
		history = append(history, models.JSONMap{"stage": stage, "at": now.UTC().Format(time.RFC3339)})
		query := tx.Model(&models.QueueJob{}).Where("id = ? AND stage = ?", job.ID, current.Stage)
		if stage == JobStagePublishing {
			query = query.Where("dismissed_at IS NULL")
		}
		if current.StageUpdatedAt == nil {
			query = query.Where("stage_updated_at IS NULL")
		} else {
			query = query.Where("stage_updated_at = ?", *current.StageUpdatedAt)
		}
		result := query.Updates(map[string]any{"stage": stage, "stage_updated_at": &now, "stage_history": history})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			var latest models.QueueJob
			if err := tx.Select("id", "stage", "stage_updated_at", "stage_history").First(&latest, job.ID).Error; err != nil {
				return err
			}
			if latest.Stage == stage && latest.StageUpdatedAt != nil {
				nextStage, nextUpdatedAt, nextHistory = latest.Stage, latest.StageUpdatedAt, latest.StageHistory
				return nil
			}
			return fmt.Errorf("queue job %d stage transition to %q conflicted with persisted stage %q", job.ID, stage, latest.Stage)
		}
		nextStage, nextUpdatedAt, nextHistory = stage, &now, history
		return nil
	})
	if err != nil {
		return err
	}
	job.Stage, job.StageUpdatedAt, job.StageHistory = nextStage, nextUpdatedAt, nextHistory
	return nil
}

func sameStageTimestamp(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func terminalStageForStatus(status string) string {
	switch status {
	case JobStatusFailed:
		return JobStageFailed
	case JobStatusCanceled:
		return JobStageCanceled
	case JobStatusCompleted:
		return JobStageValidationReview
	case JobStatusRunning:
		return JobStageConverting
	default:
		return JobStageQueued
	}
}

func stageForStatusUpdate(job models.QueueJob, previousStatus string) string {
	if job.Status != JobStatusCompleted {
		return terminalStageForStatus(job.Status)
	}
	if job.PublishedAt != nil {
		return JobStageCompleted
	}
	if previousStatus == JobStatusCompleted && job.Stage != "" {
		return job.Stage
	}
	return JobStageValidationReview
}
