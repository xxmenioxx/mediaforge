package scheduler

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"gorm.io/gorm"
)

const (
	ExecutionPlanPendingEvaluation = "pending_evaluation"
	ExecutionPlanReady             = "ready"
	ExecutionPlanWaiting           = "waiting"
	ExecutionPlanDispatched        = "dispatched"
	ExecutionPlanSuperseded        = "superseded"
)

func CreatePendingExecutionPlan(db *gorm.DB, job *models.QueueJob, reason string) (models.ExecutionPlan, error) {
	if job.ID == 0 {
		return models.ExecutionPlan{}, fmt.Errorf("job must be persisted before creating an execution plan")
	}
	profile, err := RestoreProfileSnapshot(job.ProfileSnapshot)
	if err != nil {
		return models.ExecutionPlan{}, err
	}
	constraints, err := ResolveExecutionConstraints(profile)
	if err != nil {
		return models.ExecutionPlan{}, err
	}
	constraintsJSON, err := jsonMap(constraints)
	if err != nil {
		return models.ExecutionPlan{}, err
	}

	var latest models.ExecutionPlan
	version := 1
	result := db.Where("job_id = ?", job.ID).Order("version desc").Limit(1).Find(&latest)
	if result.Error != nil {
		return models.ExecutionPlan{}, result.Error
	}
	if result.RowsAffected > 0 {
		version = latest.Version + 1
	}

	now := time.Now()
	if err := db.Model(&models.ExecutionPlan{}).
		Where("job_id = ? AND status <> ?", job.ID, ExecutionPlanSuperseded).
		Updates(map[string]any{"status": ExecutionPlanSuperseded, "superseded_at": &now}).Error; err != nil {
		return models.ExecutionPlan{}, err
	}

	plan := models.ExecutionPlan{
		JobID:           job.ID,
		Version:         version,
		Status:          ExecutionPlanPendingEvaluation,
		ProfileVersion:  job.ProfileVersion,
		Constraints:     constraintsJSON,
		CodecFamily:     constraints.CodecFamily,
		BitDepth:        constraints.BitDepth,
		PixelFormat:     constraints.PixelFormat,
		QualityMode:     constraints.QualityMode,
		QualityValue:    constraints.QualityValue,
		DecisionReasons: models.JSONList{reason},
		DecisionSources: models.JSONMap{
			"codecFamily":     "profile_snapshot",
			"bitDepth":        "profile_snapshot",
			"pixelFormat":     "profile_snapshot",
			"qualityMode":     "profile_snapshot",
			"qualityValue":    "profile_snapshot",
			"encoderPolicy":   "profile_snapshot",
			"selectedEncoder": "pending_scheduler_evaluation",
		},
		Warnings:    models.JSONList{},
		Reservation: models.JSONMap{},
		Evaluation:  models.JSONMap{},
	}
	if err := db.Create(&plan).Error; err != nil {
		return models.ExecutionPlan{}, err
	}
	job.ActiveExecutionPlanID = &plan.ID
	if err := db.Model(job).Update("active_execution_plan_id", plan.ID).Error; err != nil {
		return models.ExecutionPlan{}, err
	}
	return plan, nil
}

func jsonMap(value any) (models.JSONMap, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	result := models.JSONMap{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil, err
	}
	return result, nil
}
