package handlers

import (
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
	JobStageConverting         = "converting"
	JobStageValidating         = "validating"
	JobStageDirectPlayAnalysis = "directplay_analysis"
	JobStageReadyToPublish     = "ready_to_publish"
	JobStagePublishing         = "publishing"
	JobStageArchivingOriginal  = "archiving_original"
	JobStageCleaningWorkspace  = "cleaning_workspace"
	JobStageCompleted          = "completed"
	JobStageFailed             = "failed"
	JobStageCanceled           = "canceled"
)

func transitionJobStage(db *gorm.DB, job *models.QueueJob, stage string) error {
	if job.Stage == stage && job.StageUpdatedAt != nil {
		return nil
	}
	now := time.Now()
	job.Stage = stage
	job.StageUpdatedAt = &now
	job.StageHistory = append(job.StageHistory, models.JSONMap{"stage": stage, "at": now.UTC().Format(time.RFC3339)})
	if job.ID == 0 {
		return nil
	}
	return db.Model(job).Updates(map[string]any{"stage": job.Stage, "stage_updated_at": job.StageUpdatedAt, "stage_history": job.StageHistory}).Error
}

func terminalStageForStatus(status string) string {
	switch status {
	case JobStatusFailed:
		return JobStageFailed
	case JobStatusCanceled:
		return JobStageCanceled
	case JobStatusCompleted:
		return JobStageReadyToPublish
	case JobStatusRunning:
		return JobStageConverting
	default:
		return JobStageQueued
	}
}
