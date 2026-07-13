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

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const maxLogReadBytes = 1024 * 1024

type LogHandler struct {
	db *gorm.DB
}

type LogFile struct {
	Name       string    `json:"name"`
	SizeBytes  int64     `json:"sizeBytes"`
	ModifiedAt time.Time `json:"modifiedAt"`
}

type LogFileContent struct {
	Name      string `json:"name"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

func NewLogHandler(db *gorm.DB) LogHandler {
	return LogHandler{db: db}
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
			Name:       entry.Name(),
			SizeBytes:  info.Size(),
			ModifiedAt: info.ModTime(),
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
		logDir = os.Getenv("MEDIAFORGE_LOG_DIR")
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

	for _, job := range jobs {
		name := fmt.Sprintf("job-%d.log", job.ID)
		if err := os.WriteFile(filepath.Join(logDir, name), []byte(jobLog(h.db, job)), 0o644); err != nil {
			return "", err
		}
	}

	return logDir, nil
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
	logDir, dirErr := configuredLogDir(db)
	if dirErr != nil {
		return
	}
	if strings.TrimSpace(logDir) == "" {
		logDir = os.Getenv("MEDIAFORGE_LOG_DIR")
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
	builder.WriteString("MediaForge job log summary\n")
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
		if conversion := assetConversionReport(conversionOverrideForPath(job.MediaPath, assetConversionOverrides(db))); conversion.HasOverrides {
			builder.WriteString("  asset_conversion: " + strings.Join(conversion.HumanSummary, "; ") + "\n")
		}
	}

	return builder.String()
}

func jobLog(db *gorm.DB, job models.QueueJob) string {
	conversion := assetConversionReport(conversionOverrideForPath(job.MediaPath, assetConversionOverrides(db)))
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("MediaForge job #%d\n", job.ID))
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
