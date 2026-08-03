package handlers

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
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
	colorReport := models.JSONMap{}
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
	if outputExists {
		var colorWarning string
		colorReport, colorWarning = validateJobColorPolicy(db, job)
		if colorWarning != "" {
			warnings = append(warnings, colorWarning)
		}
		if fidelity := validateJobFrameFidelity(job); len(fidelity) > 0 {
			colorReport["frameFidelity"] = fidelity
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
	if len(colorReport) > 0 {
		report["colorPolicy"] = colorReport
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

func validateJobColorPolicy(db *gorm.DB, job models.QueueJob) (models.JSONMap, string) {
	policy := "preserve"
	if worker := unknownRecord(job.ProfileSnapshot["workerConfig"]); worker != nil {
		if value := strings.ToLower(strings.TrimSpace(stringFromUnknown(worker["finalColorPolicy"]))); value != "" {
			policy = value
		}
	}
	override := conversionOverrideForJob(job, assetConversionOverrides(db))
	if value := strings.ToLower(strings.TrimSpace(override.FinalColorPolicy)); value != "" {
		policy = value
	}
	source := firstVideoColorCharacteristics(ffprobeJSON(job.MediaPath))
	output := firstVideoColorCharacteristics(ffprobeJSON(job.OutputPath))
	report := models.JSONMap{"requestedPolicy": policy, "source": source, "output": output}
	if len(source) == 0 || len(output) == 0 {
		report["status"] = "unverified"
		return report, "Final color policy could not be verified because source or output color metadata is incomplete."
	}
	expected := cloneJSONMap(source)
	effective := "preserve"
	sourceStream := MediaStream{
		ColorSpace: stringFromUnknown(source["colorSpace"]), ColorTransfer: stringFromUnknown(source["colorTransfer"]),
		ColorPrimaries: stringFromUnknown(source["colorPrimaries"]), ColorRange: stringFromUnknown(source["colorRange"]),
	}
	normalize := policy == "normalize_bt709" ||
		(policy == "automatic" && strings.Contains(strings.ToLower(job.Notes), "-c:v hevc_videotoolbox") && !sourceUsesBT709(sourceStream) && !sourceUsesHDRTransfer(sourceStream))
	if normalize && !sourceUsesHDRTransfer(sourceStream) {
		effective = "normalize_bt709"
		expected = models.JSONMap{"colorSpace": "bt709", "colorTransfer": "bt709", "colorPrimaries": "bt709", "colorRange": "tv"}
	}
	report["effectivePolicy"] = effective
	report["expected"] = expected
	matches := colorCharacteristicsMatch(expected, output)
	report["status"] = map[bool]string{true: "passed", false: "mismatch"}[matches]
	if !matches {
		return report, "Final output color characteristics do not match the effective color policy."
	}
	return report, ""
}

// validateJobFrameFidelity records the characteristics that are not purely a
// color policy: frame geometry, aspect, fields, chroma siting and output bit
// depth. It is informational by design; codec, depth and field order can be
// deliberate profile changes and must not make an otherwise valid job fail.
func validateJobFrameFidelity(job models.QueueJob) models.JSONMap {
	source := firstVideoFrameCharacteristics(ffprobeJSON(job.MediaPath))
	output := firstVideoFrameCharacteristics(ffprobeJSON(job.OutputPath))
	if len(source) == 0 || len(output) == 0 {
		return models.JSONMap{"status": "unverified", "source": source, "output": output}
	}
	worker := unknownRecord(job.ProfileSnapshot["workerConfig"])
	filters := ""
	if worker != nil {
		filters = strings.ToLower(stringFromUnknown(worker["videoFilters"]))
	}
	effectiveCommand := strings.ToLower(job.Notes)
	filters += "," + effectiveCommand
	fields := models.JSONMap{}
	fields["sampleAspectRatio"] = frameFidelityValue(source, output, "sampleAspectRatio", strings.Contains(filters, "crop=") || strings.Contains(filters, "setsar="))
	fields["displayAspectRatio"] = frameFidelityValue(source, output, "displayAspectRatio", strings.Contains(filters, "setdar="))
	fields["frameRate"] = frameFidelityValue(source, output, "frameRate", strings.Contains(filters, "decimate") || strings.Contains(filters, "fps="))
	fields["chromaLocation"] = frameFidelityValue(source, output, "chromaLocation", false)
	fields["fieldOrder"] = frameFidelityValue(source, output, "fieldOrder", strings.Contains(filters, "bwdif") || strings.Contains(filters, "yadif") || strings.Contains(filters, "deinterlace"))
	fields["pixelFormat"] = frameFidelityValue(source, output, "pixelFormat", true)
	fields["bitDepth"] = frameFidelityValue(source, output, "bitDepth", true)
	fields["frameSize"] = frameFidelityValue(source, output, "frameSize", strings.Contains(filters, "crop=") || strings.Contains(filters, "scale="))

	status := "passed"
	for _, raw := range fields {
		if value, ok := raw.(models.JSONMap); ok && value["status"] == "changed_unexpectedly" {
			status = "warning"
			break
		}
	}
	return models.JSONMap{"status": status, "source": source, "output": output, "fields": fields}
}

func frameFidelityValue(source, output models.JSONMap, key string, allowedChange bool) models.JSONMap {
	from, to := stringFromUnknown(source[key]), stringFromUnknown(output[key])
	if from == "" || to == "" {
		return models.JSONMap{"source": from, "output": to, "status": "unverified"}
	}
	status := "preserved"
	if !strings.EqualFold(from, to) {
		if allowedChange {
			status = "changed_intentionally"
		} else {
			status = "changed_unexpectedly"
		}
	}
	return models.JSONMap{"source": from, "output": to, "status": status}
}

func unknownRecord(value interface{}) map[string]interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return typed
	case models.JSONMap:
		return map[string]interface{}(typed)
	default:
		return nil
	}
}

func firstVideoColorCharacteristics(probe map[string]any) models.JSONMap {
	result := firstVideoFrameCharacteristics(probe)
	if len(result) == 0 || result["colorSpace"] == "" || result["colorTransfer"] == "" || result["colorPrimaries"] == "" {
		return nil
	}
	return result
}

func firstVideoFrameCharacteristics(probe map[string]any) models.JSONMap {
	rawStreams, ok := probe["streams"].([]interface{})
	if !ok {
		return nil
	}
	for _, raw := range rawStreams {
		stream, ok := raw.(map[string]interface{})
		if !ok || !strings.EqualFold(stringFromUnknown(stream["codec_type"]), "video") {
			continue
		}
		result := models.JSONMap{
			"colorSpace":         strings.ToLower(strings.TrimSpace(stringFromUnknown(stream["color_space"]))),
			"colorTransfer":      strings.ToLower(strings.TrimSpace(stringFromUnknown(stream["color_transfer"]))),
			"colorPrimaries":     strings.ToLower(strings.TrimSpace(stringFromUnknown(stream["color_primaries"]))),
			"colorRange":         normalizedFFmpegRange(stringFromUnknown(stream["color_range"])),
			"pixelFormat":        strings.ToLower(strings.TrimSpace(stringFromUnknown(stream["pix_fmt"]))),
			"bitDepth":           bitDepthFromPixelFormat(stringFromUnknown(stream["pix_fmt"])),
			"frameSize":          characteristicString(stream["width"]) + "x" + characteristicString(stream["height"]),
			"sampleAspectRatio":  strings.TrimSpace(stringFromUnknown(stream["sample_aspect_ratio"])),
			"displayAspectRatio": strings.TrimSpace(stringFromUnknown(stream["display_aspect_ratio"])),
			"frameRate":          strings.TrimSpace(stringFromUnknown(stream["avg_frame_rate"])),
			"fieldOrder":         strings.ToLower(strings.TrimSpace(stringFromUnknown(stream["field_order"]))),
			"chromaLocation":     strings.ToLower(strings.TrimSpace(stringFromUnknown(stream["chroma_location"]))),
		}
		return result
	}
	return nil
}

func bitDepthFromPixelFormat(pixelFormat string) string {
	pixelFormat = strings.ToLower(pixelFormat)
	if strings.Contains(pixelFormat, "p010") || strings.Contains(pixelFormat, "10") {
		return "10-bit"
	}
	if strings.Contains(pixelFormat, "12") {
		return "12-bit"
	}
	if pixelFormat != "" {
		return "8-bit"
	}
	return ""
}

func characteristicString(value interface{}) string {
	if text := stringFromUnknown(value); text != "" {
		return text
	}
	switch typed := value.(type) {
	case float64:
		return fmt.Sprintf("%.0f", typed)
	case int:
		return strconv.Itoa(typed)
	default:
		return ""
	}
}

func cloneJSONMap(value models.JSONMap) models.JSONMap {
	result := models.JSONMap{}
	for key, item := range value {
		if key != "pixelFormat" {
			result[key] = item
		}
	}
	return result
}

func colorCharacteristicsMatch(expected, actual models.JSONMap) bool {
	for _, key := range []string{"colorSpace", "colorTransfer", "colorPrimaries", "colorRange"} {
		if !strings.EqualFold(stringFromUnknown(expected[key]), stringFromUnknown(actual[key])) {
			return false
		}
	}
	return true
}
