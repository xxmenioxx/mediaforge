package handlers

import (
	"net/http"
	"os"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	ValidationStatusPending = "pending"
	ValidationStatusPassed  = "passed"
	ValidationStatusWarning = "warning"
	ValidationStatusFailed  = "failed"
)

type ValidationHandler struct {
	db *gorm.DB
}

type ValidationResult struct {
	JobID    uint           `json:"jobId"`
	Status   string         `json:"status"`
	Score    int            `json:"score"`
	Checks   []CheckResult  `json:"checks"`
	Warnings []string       `json:"warnings"`
	Report   models.JSONMap `json:"report"`
}

type CheckResult struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func NewValidationHandler(db *gorm.DB) ValidationHandler {
	return ValidationHandler{db: db}
}

func (h ValidationHandler) ValidateJob(c *gin.Context) {
	job, ok := h.job(c)
	if !ok {
		return
	}
	if job.PublishedAt != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "published jobs are immutable and cannot be validated again"})
		return
	}

	if err := transitionJobStage(h.db, &job, JobStageValidating); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	result := validateQueueJob(h.db, job)
	if err := transitionJobStage(h.db, &job, JobStageDirectPlayAnalysis); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	job.ValidationStatus = result.Status
	job.ValidationScore = result.Score
	job.ValidationReport = result.Report

	if err := h.db.Save(&job).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	finalStage := JobStageReadyToPublish
	if result.Status == ValidationStatusFailed {
		finalStage = JobStageFailed
	}
	if err := transitionJobStage(h.db, &job, finalStage); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h ValidationHandler) job(c *gin.Context) (models.QueueJob, bool) {
	var job models.QueueJob
	if err := h.db.First(&job, c.Param("id")).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return job, false
		}

		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return job, false
	}

	return job, true
}

func validateQueueJob(db *gorm.DB, job models.QueueJob) ValidationResult {
	checks := []CheckResult{}
	warnings := []string{}
	score := 100

	addCheck := func(key string, label string, ok bool, message string, penalty int) {
		status := "passed"
		if !ok {
			status = "failed"
			score -= penalty
		}
		checks = append(checks, CheckResult{Key: key, Label: label, Status: status, Message: message})
	}

	addCheck(
		"job_completed",
		"Job completed",
		job.Status == JobStatusCompleted,
		"Job must be completed before validation.",
		35,
	)
	addCheck(
		"output_path",
		"Output path recorded",
		job.OutputPath != "",
		"Worker must record an output path.",
		25,
	)

	var info os.FileInfo
	var err error
	if job.OutputPath != "" {
		info, err = os.Stat(job.OutputPath)
	}
	outputExists := err == nil && info != nil && !info.IsDir()
	addCheck(
		"output_exists",
		"Output file exists",
		outputExists,
		"Output file must be readable from the backend container.",
		30,
	)

	outputHasBytes := outputExists && info.Size() > 0
	addCheck(
		"output_nonempty",
		"Output is not empty",
		outputHasBytes,
		"Output file must be larger than 0 bytes.",
		10,
	)

	if job.Notes != "" && job.OutputPath != "" && !outputExists {
		warnings = append(warnings, "This looks like a dry-run or missing output; real validation requires a generated file.")
	}

	var directPlayReport scheduler.DirectPlayReport
	directPlayEvaluated := false
	if outputExists {
		probe := ffprobeJSON(job.OutputPath)
		if probeError, failed := probe["error"].(string); failed {
			warnings = append(warnings, "DirectPlay analysis could not inspect the final file: "+probeError)
		} else if report, directPlayErr := scheduler.EvaluateActualDirectPlay(db, models.JSONMap(probe)); directPlayErr != nil {
			warnings = append(warnings, "DirectPlay analysis failed: "+directPlayErr.Error())
		} else {
			directPlayReport, directPlayEvaluated = report, report.Enabled
			if report.Enabled && report.Risk != "low" {
				warnings = append(warnings, "Final DirectPlay risk is "+report.Risk+" for at least one target client.")
			}
		}
	}

	if score < 0 {
		score = 0
	}

	status := ValidationStatusPassed
	if directPlayEvaluated && directPlayReport.Blocked {
		status = ValidationStatusFailed
	} else if score < 80 {
		status = ValidationStatusFailed
	} else if len(warnings) > 0 || score < 100 {
		status = ValidationStatusWarning
	}

	report := models.JSONMap{
		"checks":      checksToJSON(checks),
		"warnings":    warnings,
		"validatedAt": time.Now().Format(time.RFC3339),
	}
	if directPlayEvaluated {
		report["directPlay"] = directPlayReport
	}

	return ValidationResult{
		JobID:    job.ID,
		Status:   status,
		Score:    score,
		Checks:   checks,
		Warnings: warnings,
		Report:   report,
	}
}

func checksToJSON(checks []CheckResult) []models.JSONMap {
	values := make([]models.JSONMap, 0, len(checks))
	for _, check := range checks {
		values = append(values, models.JSONMap{
			"key":     check.Key,
			"label":   check.Label,
			"status":  check.Status,
			"message": check.Message,
		})
	}
	return values
}
