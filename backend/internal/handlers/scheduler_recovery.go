package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"github.com/anuelvs/mediaforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SchedulerRecoveryReport struct {
	RanAt                   time.Time `json:"ranAt"`
	InterruptedJobs         int       `json:"interruptedJobs"`
	PartialOutputsPreserved int       `json:"partialOutputsPreserved"`
	ReservationsReleased    int       `json:"reservationsReleased"`
	WorkersMarkedOffline    int       `json:"workersMarkedOffline"`
	MissingCompletedOutputs int       `json:"missingCompletedOutputs"`
	MissingPublishedOutputs int       `json:"missingPublishedOutputs"`
	OrphanWorkspacePaths    []string  `json:"orphanWorkspacePaths"`
	Warnings                []string  `json:"warnings"`
}

func RecoverSchedulerState(db *gorm.DB) (SchedulerRecoveryReport, error) {
	return recoverSchedulerState(db, true)
}

func recoverLiveSchedulerState(db *gorm.DB) (SchedulerRecoveryReport, error) {
	return recoverSchedulerState(db, false)
}

func recoverSchedulerState(db *gorm.DB, startup bool) (SchedulerRecoveryReport, error) {
	report := SchedulerRecoveryReport{RanAt: time.Now(), OrphanWorkspacePaths: []string{}, Warnings: []string{}}
	cutoff := report.RanAt.Add(-scheduler.WorkerHeartbeatTimeout)
	result := db.Model(&models.WorkerNode{}).Where("status = ? AND last_seen_at < ?", "online", cutoff).Update("status", "offline")
	if result.Error != nil {
		return report, result.Error
	}
	report.WorkersMarkedOffline = int(result.RowsAffected)

	var replacements []models.QueueJob
	if err := db.Where("publish_mode = ? AND published_at IS NULL AND original_archived_path <> ''", PublishModeReplaceLibrary).Find(&replacements).Error; err != nil {
		return report, err
	}
	for i := range replacements {
		job := &replacements[i]
		target := strings.TrimSpace(job.ReplacementTargetPath)
		archive := strings.TrimSpace(job.OriginalArchivedPath)
		if target != "" && fileHasBytes(target) && fileHasBytes(archive) {
			if equal, compareErr := filesEqual(job.OutputPath, target); compareErr == nil && equal {
				now := report.RanAt
				job.PublishedPath, job.PublishedAt = target, &now
				job.Notes = appendNote(job.Notes, "Scheduler recovery finalized an interrupted Library replacement")
				if err := db.Save(job).Error; err != nil {
					return report, err
				}
				_ = (PublisherHandler{db: db}).cleanupStagedJob(*job)
				continue
			}
		}
		if fileHasBytes(archive) && !fileHasBytes(job.MediaPath) {
			if err := moveFile(archive, job.MediaPath); err != nil {
				report.Warnings = append(report.Warnings, "Library replacement rollback failed for job "+strconv.FormatUint(uint64(job.ID), 10)+": "+err.Error())
				continue
			}
		}
		if target != "" {
			temporary := filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+".mediaforge-job-"+strconv.FormatUint(uint64(job.ID), 10)+".tmp")
			_ = os.Remove(temporary)
		}
		job.Status = JobStatusFailed
		job.ErrorMessage = "Interrupted while replacing a Library asset; original was restored when necessary"
		job.Notes = appendNote(job.Notes, "Scheduler recovery stopped an incomplete Library replacement")
		job.ReplacementTargetPath, job.OriginalArchivedPath = "", ""
		if err := db.Save(job).Error; err != nil {
			return report, err
		}
		report.InterruptedJobs++
	}

	var running []models.QueueJob
	if err := db.Where("status = ?", JobStatusRunning).Find(&running).Error; err != nil {
		return report, err
	}
	for i := range running {
		job := &running[i]
		if !startup && (hasRunningJobProcess(job.ID) || (job.StartedAt != nil && report.RanAt.Sub(*job.StartedAt) < 30*time.Second)) {
			continue
		}
		if fileHasBytes(job.OutputPath) {
			report.PartialOutputsPreserved++
			job.Notes = appendNote(job.Notes, "Recovery preserved the partial output for diagnosis: "+job.OutputPath)
		}
		job.Status, job.Progress, job.ValidationStatus = JobStatusFailed, 0, ValidationStatusPending
		job.ErrorMessage = "MediaForge restarted while this job was running; automatic resume is not supported"
		job.FinishedAt = &report.RanAt
		job.Notes = appendNote(job.Notes, "Scheduler recovery marked the interrupted job as failed")
		if err := db.Save(job).Error; err != nil {
			return report, err
		}
		if err := transitionJobStage(db, job, JobStageFailed); err != nil {
			return report, err
		}
		var active int64
		if err := db.Model(&models.SchedulerReservation{}).Where("job_id = ? AND state = ?", job.ID, scheduler.ReservationStateActive).Count(&active).Error; err != nil {
			return report, err
		}
		if err := scheduler.DeactivateReservationResources(db, job.ID); err != nil {
			return report, err
		}
		report.ReservationsReleased += int(active)
		report.InterruptedJobs++
	}

	var activeReservations []models.SchedulerReservation
	if err := db.Where("state = ?", scheduler.ReservationStateActive).Find(&activeReservations).Error; err != nil {
		return report, err
	}
	for _, reservation := range activeReservations {
		var job models.QueueJob
		result := db.Where("id = ?", reservation.JobID).Limit(1).Find(&job)
		if result.Error != nil {
			return report, result.Error
		}
		if result.RowsAffected == 0 || job.Status != JobStatusRunning {
			if err := scheduler.DeactivateReservationResources(db, reservation.JobID); err != nil {
				return report, err
			}
			report.ReservationsReleased++
		}
	}

	var completed []models.QueueJob
	if err := db.Where("status = ? AND published_at IS NULL", JobStatusCompleted).Find(&completed).Error; err != nil {
		return report, err
	}
	for i := range completed {
		job := &completed[i]
		if strings.TrimSpace(job.OutputPath) == "" || fileHasBytes(job.OutputPath) {
			continue
		}
		job.Status, job.ValidationStatus, job.ValidationScore = JobStatusFailed, ValidationStatusFailed, 0
		job.ErrorMessage = "Recovery could not find the completed output file"
		job.Notes = appendNote(job.Notes, "Scheduler recovery detected a missing completed output")
		if err := db.Save(job).Error; err != nil {
			return report, err
		}
		if err := transitionJobStage(db, job, JobStageFailed); err != nil {
			return report, err
		}
		report.MissingCompletedOutputs++
	}

	var published []models.QueueJob
	if err := db.Where("published_at IS NOT NULL AND published_path <> '' AND publication_retired_at IS NULL").Find(&published).Error; err != nil {
		return report, err
	}
	for i := range published {
		if fileHasBytes(published[i].PublishedPath) {
			continue
		}
		published[i].Notes = appendNote(published[i].Notes, "Scheduler recovery warning: published output is missing from the library")
		if err := db.Save(&published[i]).Error; err != nil {
			return report, err
		}
		report.MissingPublishedOutputs++
	}

	if roles, err := scheduler.LoadStorageRoles(db); err == nil {
		if workRoot, pathErr := roles.Path(scheduler.StorageRoleWork); pathErr == nil {
			report.OrphanWorkspacePaths = orphanWorkspacePaths(db, workRoot)
		}
	} else {
		report.Warnings = append(report.Warnings, "Workspace reconciliation unavailable: "+err.Error())
	}

	if err := persistRecoveryReport(db, report); err != nil {
		return report, err
	}
	return report, nil
}

func fileHasBytes(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}

func orphanWorkspacePaths(db *gorm.DB, root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return []string{}
	}
	paths := []string{}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "job-") {
			continue
		}
		id, parseErr := strconv.ParseUint(strings.TrimPrefix(entry.Name(), "job-"), 10, 64)
		if parseErr != nil {
			continue
		}
		var job models.QueueJob
		result := db.Where("id = ?", uint(id)).Limit(1).Find(&job)
		if result.Error == nil && (result.RowsAffected == 0 || job.PublishedAt != nil || job.Status == JobStatusCanceled) {
			paths = append(paths, filepath.Join(root, entry.Name()))
		}
	}
	return paths
}

func persistRecoveryReport(db *gorm.DB, report SchedulerRecoveryReport) error {
	value := models.JSONMap{
		"ranAt": report.RanAt.UTC().Format(time.RFC3339), "interruptedJobs": report.InterruptedJobs, "partialOutputsPreserved": report.PartialOutputsPreserved,
		"reservationsReleased": report.ReservationsReleased, "workersMarkedOffline": report.WorkersMarkedOffline, "missingCompletedOutputs": report.MissingCompletedOutputs,
		"missingPublishedOutputs": report.MissingPublishedOutputs, "orphanWorkspacePaths": report.OrphanWorkspacePaths, "warnings": report.Warnings,
	}
	setting := models.AppSetting{Key: "schedulerRecovery", Value: value}
	return db.Where(models.AppSetting{Key: setting.Key}).Assign(models.AppSetting{Value: value}).FirstOrCreate(&setting).Error
}

type SchedulerRecoveryHandler struct{ db *gorm.DB }

func NewSchedulerRecoveryHandler(db *gorm.DB) SchedulerRecoveryHandler {
	return SchedulerRecoveryHandler{db: db}
}

func (h SchedulerRecoveryHandler) Latest(c *gin.Context) {
	var setting models.AppSetting
	result := h.db.Where("key = ?", "schedulerRecovery").Limit(1).Find(&setting)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "scheduler recovery has not run"})
		return
	}
	c.JSON(http.StatusOK, setting.Value)
}

func (h SchedulerRecoveryHandler) Run(c *gin.Context) {
	report, err := recoverLiveSchedulerState(h.db)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, report)
}
