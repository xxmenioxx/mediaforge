package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/applog"
	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
)

var canceledJobCleanupSlots = make(chan struct{}, 2)

type cancellationPolicy struct {
	DeleteGeneratedFiles           bool
	DeletePartialOutputFromStaging bool
	ControlledRoots                []string
}

func loadCancellationPolicy(db *gorm.DB) cancellationPolicy {
	policy := cancellationPolicy{
		ControlledRoots: []string{"/media/staging", "/mwp/work", "/mwp/work/temp", "/tmp/mvforge"},
	}
	var setting models.AppSetting
	if result := db.Where("key = ?", "cancellationPolicy").Limit(1).Find(&setting); result.Error == nil && result.RowsAffected > 0 {
		policy.DeleteGeneratedFiles = boolSetting(setting.Value["deleteGeneratedFiles"], false)
		policy.DeletePartialOutputFromStaging = boolSetting(setting.Value["deletePartialOutputFromStaging"], false)
		policy.ControlledRoots = stringSliceFromUnknown(setting.Value["controlledRoots"])
	}
	return policy
}

func cleanupCanceledJob(db *gorm.DB, job models.QueueJob) error {
	policy := loadCancellationPolicy(db)
	if !policy.DeleteGeneratedFiles && !policy.DeletePartialOutputFromStaging {
		return nil
	}

	outputPath := filepath.Clean(strings.TrimSpace(job.OutputPath))
	if outputPath == "." || outputPath == "" || !pathInsideAnyRoot(outputPath, policy.ControlledRoots) {
		return nil
	}
	if policy.DeleteGeneratedFiles {
		workspace := filepath.Dir(outputPath)
		if filepath.Base(workspace) != fmt.Sprintf("job-%d", job.ID) || !pathInsideAnyRoot(workspace, policy.ControlledRoots) {
			return nil
		}
		return os.RemoveAll(workspace)
	}
	if err := os.Remove(outputPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func scheduleCanceledJobCleanup(db *gorm.DB, job models.QueueJob) {
	if !canceledJobCleanupRequired(db, job) {
		_ = db.Model(&models.QueueJob{}).
			Where("id = ? AND status = ? AND cleanup_status = ?", job.ID, JobStatusCanceled, "").
			Update("cleanup_status", "not_required").Error
		return
	}
	scheduleCanceledJobCleanupWith(db, job, cleanupCanceledJob)
}

func canceledJobCleanupRequired(db *gorm.DB, job models.QueueJob) bool {
	policy := loadCancellationPolicy(db)
	if !policy.DeleteGeneratedFiles && !policy.DeletePartialOutputFromStaging {
		return false
	}
	outputPath := filepath.Clean(strings.TrimSpace(job.OutputPath))
	if outputPath == "." || outputPath == "" || !pathInsideAnyRoot(outputPath, policy.ControlledRoots) {
		return false
	}
	if policy.DeleteGeneratedFiles {
		workspace := filepath.Dir(outputPath)
		return filepath.Base(workspace) == fmt.Sprintf("job-%d", job.ID) && pathInsideAnyRoot(workspace, policy.ControlledRoots)
	}
	return true
}

func scheduleCanceledJobCleanupWith(db *gorm.DB, job models.QueueJob, cleanup func(*gorm.DB, models.QueueJob) error) {
	if db == nil || job.ID == 0 || cleanup == nil {
		return
	}
	result := db.Model(&models.QueueJob{}).
		Where("id = ? AND status = ? AND cleanup_status NOT IN ?", job.ID, JobStatusCanceled, []string{"pending", "running", "completed"}).
		Updates(map[string]any{"cleanup_status": "pending", "cleanup_error": ""})
	if result.Error != nil || result.RowsAffected != 1 {
		if result.Error != nil {
			applog.Event("error", "queue", "canceled_job_cleanup_schedule_failed", map[string]any{"jobId": job.ID}, result.Error)
		}
		return
	}

	go func() {
		canceledJobCleanupSlots <- struct{}{}
		defer func() { <-canceledJobCleanupSlots }()
		_ = db.Model(&models.QueueJob{}).
			Where("id = ? AND status = ? AND cleanup_status = ?", job.ID, JobStatusCanceled, "pending").
			Update("cleanup_status", "running").Error

		startedAt := time.Now()
		err := cleanup(db, job)
		updates := map[string]any{"cleanup_status": "completed", "cleanup_error": ""}
		if err != nil {
			updates["cleanup_status"] = "failed"
			updates["cleanup_error"] = boundedCanceledCleanupError(err)
			applog.Event("error", "queue", "canceled_job_cleanup_failed", map[string]any{"jobId": job.ID, "elapsedMs": time.Since(startedAt).Milliseconds()}, err)
		} else {
			applog.Event("info", "queue", "canceled_job_cleanup_completed", map[string]any{"jobId": job.ID, "elapsedMs": time.Since(startedAt).Milliseconds()}, nil)
		}
		if updateErr := db.Model(&models.QueueJob{}).
			Where("id = ? AND status = ?", job.ID, JobStatusCanceled).
			Updates(updates).Error; updateErr != nil {
			applog.Event("error", "queue", "canceled_job_cleanup_status_update_failed", map[string]any{"jobId": job.ID}, updateErr)
		}
	}()
}

func boundedCanceledCleanupError(err error) string {
	if err == nil {
		return ""
	}
	message := []rune(strings.TrimSpace(err.Error()))
	if len(message) > 1000 {
		message = message[:1000]
	}
	return string(message)
}

func pathInsideAnyRoot(path string, roots []string) bool {
	for _, root := range roots {
		if strings.TrimSpace(root) != "" && pathIsInside(path, filepath.Clean(root)) {
			return true
		}
	}
	return false
}
