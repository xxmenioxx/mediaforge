package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/applog"
	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const maxLogReadBytes = 1024 * 1024

type LogHandler struct {
	db *gorm.DB
}

type LogFile struct {
	Name        string    `json:"name"`
	Category    string    `json:"category"`
	Description string    `json:"description"`
	SizeBytes   int64     `json:"sizeBytes"`
	ModifiedAt  time.Time `json:"modifiedAt"`
}

type LogFileContent struct {
	Name      string `json:"name"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

func NewLogHandler(db *gorm.DB) LogHandler {
	return LogHandler{db: db}
}

func ConfigureApplicationLogging(db *gorm.DB) error {
	logDir, err := configuredLogDir(db)
	if err != nil {
		return err
	}
	if strings.TrimSpace(logDir) == "" {
		logDir = os.Getenv("MVFORGE_LOG_DIR")
	}
	if strings.TrimSpace(logDir) == "" {
		logDir = "/media/reports/logs"
	}
	return applog.Initialize(logDir)
}

func (h LogHandler) ListFiles(c *gin.Context) {
	logDir, err := h.prepareLogFiles()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	entries, err := os.ReadDir(logDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	files := []LogFile{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		files = append(files, LogFile{
			Name:        entry.Name(),
			Category:    logFileCategory(entry.Name()),
			Description: logFileDescription(entry.Name()),
			SizeBytes:   info.Size(),
			ModifiedAt:  info.ModTime(),
		})
	}

	sort.SliceStable(files, func(i, j int) bool {
		return files[i].ModifiedAt.After(files[j].ModifiedAt)
	})

	c.JSON(http.StatusOK, files)
}

func (h LogHandler) ReadFile(c *gin.Context) {
	logDir, err := h.prepareLogFiles()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	name := c.Param("name")
	if !safeLogFileName(name) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid log file name"})
		return
	}

	content, truncated, err := readLogFile(filepath.Join(logDir, name))
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "log file not found"})
			return
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, LogFileContent{Name: name, Content: content, Truncated: truncated})
}

func (h LogHandler) prepareLogFiles() (string, error) {
	logDir, err := h.configuredLogDir()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(logDir) == "" {
		logDir = os.Getenv("MVFORGE_LOG_DIR")
	}
	if strings.TrimSpace(logDir) == "" {
		logDir = "/media/reports/logs"
	}

	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return "", err
	}

	var jobs []models.QueueJob
	if err := h.db.Order("created_at desc").Find(&jobs).Error; err != nil {
		return "", err
	}

	if err := os.WriteFile(filepath.Join(logDir, "jobs.log"), []byte(allJobsLog(h.db, jobs)), 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(logDir, "scheduler.log"), []byte(schedulerLog(h.db)), 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(logDir, "workers.log"), []byte(workersLog(h.db)), 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(logDir, "pipeline.log"), []byte(pipelineLog(jobs)), 0o644); err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(logDir, "system.log")); os.IsNotExist(err) {
		if err := os.WriteFile(filepath.Join(logDir, "system.log"), []byte(""), 0o644); err != nil {
			return "", err
		}
	}

	for _, job := range jobs {
		name := fmt.Sprintf("job-%d.log", job.ID)
		if err := os.WriteFile(filepath.Join(logDir, name), []byte(jobLog(h.db, job)), 0o644); err != nil {
			return "", err
		}
	}

	return logDir, nil
}

func logFileCategory(name string) string {
	switch name {
	case "system.log":
		return "system"
	case "backend.log":
		return "backend"
	case "scheduler.log":
		return "scheduler"
	case "workers.log":
		return "workers"
	case "pipeline.log":
		return "pipeline"
	default:
		return "jobs"
	}
}

func logFileDescription(name string) string {
	switch name {
	case "system.log":
		return "Application and subsystem events"
	case "backend.log":
		return "Persistent backend requests, errors and panics"
	case "scheduler.log":
		return "Plans, reservations and runtime decisions"
	case "workers.log":
		return "Worker heartbeats, capacity and active claims"
	case "pipeline.log":
		return "Lifecycle, validation and publishing activity"
	case "jobs.log":
		return "Summary of every queue job"
	default:
		return "Detailed job diagnostics"
	}
}

func schedulerLog(db *gorm.DB) string {
	var plans []models.ExecutionPlan
	var reservations []models.SchedulerReservation
	var snapshots []models.RuntimeSnapshot
	_ = db.Order("updated_at desc").Limit(200).Find(&plans).Error
	_ = db.Order("updated_at desc").Limit(200).Find(&reservations).Error
	_ = db.Order("detected_at desc").Limit(20).Find(&snapshots).Error
	var builder strings.Builder
	builder.WriteString("MVForge scheduler log\nGenerated: " + time.Now().Format(time.RFC3339) + "\n\nExecution plans\n")
	for _, plan := range plans {
		builder.WriteString(fmt.Sprintf("[%s] plan=%d job=%d v=%d status=%s waiting=%s encoder=%s runtime=%s approval=%s output=%s\n", plan.UpdatedAt.Format(time.RFC3339), plan.ID, plan.JobID, plan.Version, plan.Status, emptyLabel(plan.WaitingState), plan.SelectedEncoder, plan.RuntimeProfile, plan.ApprovalStatus, plan.OutputPath))
		if len(plan.DecisionReasons) > 0 || len(plan.Warnings) > 0 || len(plan.Evaluation) > 0 {
			builder.WriteString(fmt.Sprintf("  reasons=%v warnings=%v evaluation=%v\n", plan.DecisionReasons, plan.Warnings, plan.Evaluation))
		}
	}
	builder.WriteString("\nReservations\n")
	for _, item := range reservations {
		builder.WriteString(fmt.Sprintf("[%s] job=%d asset=%s state=%s worker=%s type=%s encoder=%s class=%s\n", item.UpdatedAt.Format(time.RFC3339), item.JobID, item.AssetKey, item.State, emptyLabel(item.WorkerName), emptyLabel(item.JobType), emptyLabel(item.Encoder), emptyLabel(item.EncoderClass)))
	}
	builder.WriteString("\nRuntime snapshots\n")
	for _, item := range snapshots {
		builder.WriteString(fmt.Sprintf("[%s] detected=%s effective=%s preferred=%s overrides=%v disks=%v warnings=%v\n", item.DetectedAt.Format(time.RFC3339), item.RecommendedProfile, item.SelectedProfile, emptyLabel(item.PreferredProfile), item.AppliedOverrides, item.Disks, item.Warnings))
	}
	return builder.String()
}

func workersLog(db *gorm.DB) string {
	var workers []models.WorkerNode
	var jobs []models.QueueJob
	_ = db.Order("name asc").Find(&workers).Error
	_ = db.Where("status IN ?", []string{JobStatusQueued, JobStatusRunning}).Order("priority asc, created_at asc").Find(&jobs).Error
	var builder strings.Builder
	builder.WriteString("MVForge workers log\nGenerated: " + time.Now().Format(time.RFC3339) + "\n\nWorkers\n")
	for _, worker := range workers {
		builder.WriteString(fmt.Sprintf("[%s] worker=%s status=%s slots=%d runtime=%s encoders=%v\n", worker.LastSeenAt.Format(time.RFC3339), worker.Name, worker.Status, worker.MaxConcurrentJobs, worker.RuntimeProfile, worker.Encoders))
	}
	builder.WriteString("\nQueued and active claims\n")
	for _, job := range jobs {
		builder.WriteString(fmt.Sprintf("[%s] job=%d status=%s stage=%s worker=%s priority=%d media=%s\n", job.UpdatedAt.Format(time.RFC3339), job.ID, job.Status, job.Stage, emptyLabel(job.WorkerName), job.Priority, job.MediaPath))
		if job.ActiveExecutionPlanID != nil {
			var plan models.ExecutionPlan
			if db.First(&plan, *job.ActiveExecutionPlanID).Error == nil {
				builder.WriteString(fmt.Sprintf("  plan=%d status=%s waiting=%s reasons=%v\n", plan.ID, plan.Status, emptyLabel(plan.WaitingState), plan.DecisionReasons))
			}
		}
	}
	return builder.String()
}

func pipelineLog(jobs []models.QueueJob) string {
	var builder strings.Builder
	builder.WriteString("MVForge pipeline log\nGenerated: " + time.Now().Format(time.RFC3339) + "\n\n")
	for _, job := range jobs {
		builder.WriteString(fmt.Sprintf("[%s] job=%d status=%s stage=%s validation=%s score=%d published=%s\n", job.UpdatedAt.Format(time.RFC3339), job.ID, job.Status, job.Stage, emptyLabel(job.ValidationStatus), job.ValidationScore, emptyLabel(job.PublishedPath)))
		if len(job.StageHistory) > 0 {
			payload, _ := json.Marshal(job.StageHistory)
			builder.WriteString("  lifecycle: " + string(payload) + "\n")
		}
		if job.ErrorMessage != "" {
			builder.WriteString("  error: " + job.ErrorMessage + "\n")
		}
	}
	return builder.String()
}

func (h LogHandler) configuredLogDir() (string, error) {
	return configuredLogDir(h.db)
}

func configuredLogDir(db *gorm.DB) (string, error) {
	setting := models.AppSetting{}
	if err := db.First(&setting, "key = ?", "paths").Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(stringFromUnknown(setting.Value["logsPath"])), nil
}

func appendSystemLog(db *gorm.DB, event string, fields map[string]string, err error) {
	structuredFields := make(map[string]any, len(fields))
	for key, value := range fields {
		structuredFields[key] = value
	}
	level := "info"
	if err != nil {
		level = "error"
	}
	applog.Event(level, "system", event, structuredFields, err)

	logDir, dirErr := configuredLogDir(db)
	if dirErr != nil {
		return
	}
	if strings.TrimSpace(logDir) == "" {
		logDir = os.Getenv("MVFORGE_LOG_DIR")
	}
	if strings.TrimSpace(logDir) == "" {
		logDir = "/media/reports/logs"
	}
	if mkErr := os.MkdirAll(logDir, 0o755); mkErr != nil {
		return
	}

	payload := map[string]string{
		"ts":    time.Now().Format(time.RFC3339),
		"event": event,
	}
	for key, value := range fields {
		payload[key] = value
	}
	if err != nil {
		payload["error"] = err.Error()
	}
	line, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return
	}
	_ = appendLine(filepath.Join(logDir, "system.log"), string(line))
}

func appendLine(path string, line string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(line + "\n")
	return err
}

func allJobsLog(db *gorm.DB, jobs []models.QueueJob) string {
	var builder strings.Builder
	builder.WriteString("MVForge job log summary\n")
	builder.WriteString("Generated: " + time.Now().Format(time.RFC3339) + "\n\n")

	for _, job := range jobs {
		builder.WriteString(fmt.Sprintf(
			"[%s] job=%d status=%s progress=%d media=%s output=%s validation=%s score=%d published=%s\n",
			job.UpdatedAt.Format(time.RFC3339),
			job.ID,
			job.Status,
			job.Progress,
			job.MediaPath,
			job.OutputPath,
			emptyLabel(job.ValidationStatus),
			job.ValidationScore,
			job.PublishedPath,
		))
		if job.ErrorMessage != "" {
			builder.WriteString("  error: " + job.ErrorMessage + "\n")
		}
		if job.Notes != "" {
			builder.WriteString("  notes: " + strings.ReplaceAll(job.Notes, "\n", "\n  ") + "\n")
		}
		if conversion := assetConversionReport(conversionOverrideForJob(job, assetConversionOverrides(db))); conversion.HasOverrides {
			builder.WriteString("  asset_conversion: " + strings.Join(conversion.HumanSummary, "; ") + "\n")
		}
	}

	return builder.String()
}

func jobLog(db *gorm.DB, job models.QueueJob) string {
	conversion := assetConversionReport(conversionOverrideForJob(job, assetConversionOverrides(db)))
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("MVForge job #%d\n", job.ID))
	builder.WriteString("Generated: " + time.Now().Format(time.RFC3339) + "\n\n")
	builder.WriteString(fmt.Sprintf("Batch ID: %s\n", emptyLabel(job.BatchID)))
	builder.WriteString(fmt.Sprintf("Batch Name: %s\n", emptyLabel(job.BatchName)))
	builder.WriteString(fmt.Sprintf("Media Path: %s\n", job.MediaPath))
	builder.WriteString(fmt.Sprintf("Library ID: %d\n", job.LibraryID))
	builder.WriteString(fmt.Sprintf("Profile ID: %d\n", job.ProfileID))
	builder.WriteString(fmt.Sprintf("Priority: %d\n", job.Priority))
	builder.WriteString(fmt.Sprintf("Status: %s\n", job.Status))
	builder.WriteString(fmt.Sprintf("Progress: %d\n", job.Progress))
	builder.WriteString(fmt.Sprintf("Worker: %s\n", emptyLabel(job.WorkerName)))
	builder.WriteString(fmt.Sprintf("Output Path: %s\n", emptyLabel(job.OutputPath)))
	builder.WriteString(fmt.Sprintf("Published Path: %s\n", emptyLabel(job.PublishedPath)))
	builder.WriteString(fmt.Sprintf("Validation: %s (%d/100)\n", emptyLabel(job.ValidationStatus), job.ValidationScore))
	builder.WriteString(fmt.Sprintf("Created At: %s\n", job.CreatedAt.Format(time.RFC3339)))
	builder.WriteString(fmt.Sprintf("Updated At: %s\n", job.UpdatedAt.Format(time.RFC3339)))
	if job.StartedAt != nil {
		builder.WriteString(fmt.Sprintf("Started At: %s\n", job.StartedAt.Format(time.RFC3339)))
	}
	if job.FinishedAt != nil {
		builder.WriteString(fmt.Sprintf("Finished At: %s\n", job.FinishedAt.Format(time.RFC3339)))
	}
	if job.PublishedAt != nil {
		builder.WriteString(fmt.Sprintf("Published At: %s\n", job.PublishedAt.Format(time.RFC3339)))
	}
	if job.ErrorMessage != "" {
		builder.WriteString("\nError\n")
		builder.WriteString(job.ErrorMessage + "\n")
	}
	if job.Notes != "" {
		builder.WriteString("\nNotes\n")
		builder.WriteString(job.Notes + "\n")
	}
	if conversion.HasOverrides {
		builder.WriteString("\nAsset Conversion Overrides\n")
		for _, item := range conversion.HumanSummary {
			builder.WriteString("- " + item + "\n")
		}
		payload, err := json.MarshalIndent(conversion, "", "  ")
		if err == nil {
			builder.WriteString("\nAsset Conversion JSON\n")
			builder.Write(payload)
			builder.WriteString("\n")
		}
	}
	if len(job.ValidationReport) > 0 {
		builder.WriteString("\nValidation Report\n")
		report, err := json.MarshalIndent(job.ValidationReport, "", "  ")
		if err == nil {
			builder.Write(report)
			builder.WriteString("\n")
		}
	}

	return builder.String()
}

func readLogFile(path string) (string, bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}

	if len(content) <= maxLogReadBytes {
		return string(content), false, nil
	}

	return string(content[len(content)-maxLogReadBytes:]), true, nil
}

func safeLogFileName(name string) bool {
	return name == filepath.Base(name) && strings.HasSuffix(name, ".log")
}

func emptyLabel(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(none)"
	}
	return value
}
