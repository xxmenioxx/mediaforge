package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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

	sidecarChecks, sidecarWarnings, sidecarReport := validateRequiredSubtitleArtifacts(job)
	for _, check := range sidecarChecks {
		checks = append(checks, check)
		if check.Status == "failed" {
			score -= 30
		}
	}
	warnings = append(warnings, sidecarWarnings...)
	fontChecks, fontWarnings, fontReport := validateRequiredFontAttachmentArtifacts(job)
	for _, check := range fontChecks {
		checks = append(checks, check)
		if check.Status == "failed" {
			score -= 30
		}
	}
	warnings = append(warnings, fontWarnings...)

	if job.Notes != "" && job.OutputPath != "" && !outputExists {
		warnings = append(warnings, "This looks like a dry-run or missing output; real validation requires a generated file.")
	}

	var directPlayReport scheduler.DirectPlayReport
	directPlayEvaluated := false
	var sourceDirectPlayReport scheduler.DirectPlayReport
	sourceDirectPlayAvailable := false
	colorReport := models.JSONMap{}
	frameFidelityReport := models.JSONMap{}
	var perceptualEvidence PerceptualEvidenceSet
	perceptualEvidenceAvailable := false
	perceptualEvidenceWarning := ""
	timingReport := models.JSONMap{"status": "unverified"}
	var sourceProbe map[string]any
	var outputProbe map[string]any
	var hevcEvidence models.JSONMap
	if strings.TrimSpace(job.MediaPath) != "" {
		sourceProbe = ffprobeJSON(job.MediaPath)
		if _, failed := sourceProbe["error"].(string); !failed {
			if report, directPlayErr := scheduler.EvaluateActualDirectPlay(db, models.JSONMap(sourceProbe)); directPlayErr == nil {
				sourceDirectPlayReport, sourceDirectPlayAvailable = report, true
			}
		}
	}
	if outputExists {
		outputProbe = ffprobeJSON(job.OutputPath)
		hevcEvidence = hevcBitstreamEvidenceForValidation(job.OutputPath, outputProbe)
		if probeError, failed := outputProbe["error"].(string); failed {
			warnings = append(warnings, "DirectPlay analysis could not inspect the final file: "+probeError)
		} else if report, directPlayErr := scheduler.EvaluateActualDirectPlay(db, models.JSONMap(outputProbe)); directPlayErr != nil {
			warnings = append(warnings, "DirectPlay analysis failed: "+directPlayErr.Error())
		} else {
			directPlayReport, directPlayEvaluated = report, report.Enabled
			if report.Enabled && report.Risk != "low" {
				warnings = append(warnings, "Final DirectPlay risk is "+report.Risk+" for at least one target client.")
			}
		}
		if sourceProbe != nil {
			sourceTiming, outputTiming := avTimingFromProbe(sourceProbe), avTimingFromProbe(outputProbe)
			if measured, timingErr := probeAVTiming(job.MediaPath, 0); timingErr == nil {
				sourceTiming = measured
			}
			if measured, timingErr := probeAVTiming(job.OutputPath, 0); timingErr == nil {
				outputTiming = measured
			}
			streamPlan, planAvailable := resolvedStreamPlanForJobValidation(db, job)
			sourceTiming, outputTiming = avTimingForQueueValidation(sourceTiming, outputTiming, streamPlan, planAvailable)
			timingReport = validateAVTiming(sourceTiming, outputTiming)
			if timingReport["status"] == "unverified" {
				warnings = append(warnings, "Final output A/V timestamp alignment could not be verified from available evidence.")
			} else if timingReport["status"] == "warning" || timingReport["status"] == "mismatch" {
				warnings = append(warnings, "Final output did not preserve source A/V timestamp alignment within one output frame.")
			}
		}
	}
	smartUpscaleReport := models.JSONMap{"status": "not_applicable"}
	restorationReport := models.JSONMap{"status": "not_applicable"}
	if profile, restoreErr := scheduler.RestoreProfileSnapshot(job.ProfileSnapshot); restoreErr == nil {
		smartUpscaleReport = validateSmartUpscaleOutput(profile, firstVideoFrameCharacteristics(sourceProbe), firstVideoFrameCharacteristics(outputProbe))
		switch smartUpscaleReport["status"] {
		case "mismatch":
			addCheck("smart_upscale_output", "Smart Upscale output", false, "Final output does not match the frozen Smart Upscale geometry/frame plan.", 40)
			warnings = append(warnings, "Final output does not match the frozen Smart Upscale decision.")
		case "unverified":
			warnings = append(warnings, "Smart Upscale output could not be fully verified from final ffprobe evidence.")
		}
		restorationReport = validateRestorationOutput(profile, firstVideoFrameCharacteristics(sourceProbe), firstVideoFrameCharacteristics(outputProbe), smartUpscaleReport)
		if restorationReport["status"] == "mismatch" && smartUpscaleReport["status"] != "mismatch" {
			addCheck("restoration_output", "Restoration output", false, "Final output does not match the frozen restoration geometry/frame plan.", 20)
			warnings = append(warnings, "Final output does not match the frozen restoration execution plan.")
		} else if restorationReport["status"] == "unverified" {
			warnings = append(warnings, "Restoration output could not be fully verified from final ffprobe evidence.")
		}
	}
	if outputExists {
		var colorWarning string
		colorReport, colorWarning = validateJobColorPolicy(db, job, sourceProbe, outputProbe)
		if colorWarning != "" {
			warnings = append(warnings, colorWarning)
		}
		frameFidelityReport = validateJobFrameFidelity(db, job, sourceProbe, outputProbe, hevcEvidence, colorReport, smartUpscaleReport, restorationReport)
		if frozenPlan, planOK := samplingPlanFromQueueJob(job); planOK {
			if sourceMatches, sourceErr := frozenSamplingPlanSourceMatches(job.MediaPath, frozenPlan); sourceErr != nil {
				perceptualEvidenceWarning = "perceptual evidence unavailable: current source revision could not be verified: " + sourceErr.Error()
			} else if !sourceMatches {
				staleEvidence, staleErr := unavailablePerceptualEvidenceSet(frozenPlan, frozenPlan.SourceRevision, "stale", "current source revision does not match the frozen Queue SamplingPlan")
				if staleErr == nil {
					perceptualEvidence = staleEvidence
					perceptualEvidenceAvailable = true
				}
				perceptualEvidenceWarning = "perceptual evidence stale: current source revision does not match the frozen Queue SamplingPlan"
			} else if sourceRevision, sourceErr := sourceRevisionForPath(job.MediaPath); sourceErr == nil {
				sourceRevisionID, revisionHashErr := canonicalEvidenceHash("source-revision", sourceRevision)
				candidateRevision, candidateErr := mediaFileFingerprint(job.OutputPath)
				candidateDuration := validationProbeDuration(outputProbe)
				if revisionHashErr == nil && candidateErr == nil {
					authority := resolveFidelityAuthority(db, job)
					perceptualEvidence, revisionHashErr = collectPerceptualEvidenceUnderHeavySlot(context.Background(), frozenPlan, authority.profile, perceptualArtifactInput{
						SourcePath: job.MediaPath, CandidatePath: job.OutputPath, SourceRevision: sourceRevisionID,
						CandidateRevision: candidateRevision, CandidateKind: "final_output", CandidateArtifactID: fmt.Sprintf("queue-job:%d", job.ID),
						GenerationToken: fmt.Sprintf("queue-job:%d", job.ID), GenerationVersion: 1,
						ConfigurationIdentity: fmt.Sprintf("queue-job:%d:profile:%d", job.ID, job.ProfileVersion),
						SegmentStartSeconds:   0, SegmentDuration: candidateDuration, CandidateDuration: candidateDuration, FullOutput: true,
					})
					if revisionHashErr == nil {
						perceptualEvidenceAvailable = true
					} else {
						perceptualEvidenceWarning = revisionHashErr.Error()
					}
				}
			} else {
				perceptualEvidenceWarning = sourceErr.Error()
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
	report["subtitleArtifacts"] = sidecarReport
	report["fontAttachmentArtifacts"] = fontReport
	if directPlayEvaluated {
		report["directPlay"] = directPlayReport
	}
	if sourceDirectPlayAvailable {
		report["directPlaySource"] = sourceDirectPlayReport
	}
	if len(colorReport) > 0 {
		report["colorPolicy"] = colorReport
	}
	if len(frameFidelityReport) > 0 {
		report["frameFidelity"] = frameFidelityReport
	}
	report["avTiming"] = timingReport
	report["smartUpscale"] = smartUpscaleReport
	report["restoration"] = restorationReport
	if len(hevcEvidence) > 0 {
		report["hevcBitstreamEvidence"] = hevcEvidence
	}
	if perceptualEvidenceAvailable {
		report["perceptualEvidence"] = perceptualEvidence
	} else if perceptualEvidenceWarning != "" {
		// Optional perceptual evidence is descriptive and must not alter the
		// existing Validation score/status when its reference is unavailable.
		report["perceptualEvidenceWarning"] = perceptualEvidenceWarning
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

func validateRestorationOutput(profile models.Profile, source, output, smartUpscale models.JSONMap) models.JSONMap {
	plan, ok := resolvedRestorationPlanFromProfile(profile)
	if !ok {
		return models.JSONMap{"status": "not_applicable"}
	}
	report := restorationPlanReport(profile, nil, nil, "")
	report["sourceStorage"] = models.JSONMap{"width": workerIntValue(source["width"], plan.SourceStorage.Width), "height": workerIntValue(source["height"], plan.SourceStorage.Height), "sar": stringFromUnknown(source["sampleAspectRatio"]), "dar": stringFromUnknown(source["displayAspectRatio"])}
	report["actualOutput"] = output
	fields := models.JSONMap{}
	actualWidth, actualHeight := workerIntValue(output["width"], 0), workerIntValue(output["height"], 0)
	geometryStatus := "passed"
	if plan.ResolvedOutput.Width <= 0 || plan.ResolvedOutput.Height <= 0 || actualWidth <= 0 || actualHeight <= 0 {
		geometryStatus = "unverified"
	} else if plan.ResolvedOutput.Width != actualWidth || plan.ResolvedOutput.Height != actualHeight {
		geometryStatus = "mismatch"
	}
	fields["geometry"] = models.JSONMap{"expectedWidth": plan.ResolvedOutput.Width, "expectedHeight": plan.ResolvedOutput.Height, "actualWidth": actualWidth, "actualHeight": actualHeight, "status": geometryStatus}
	if strings.TrimSpace(plan.ResolvedOutput.SAR) != "" {
		fields["sampleAspectRatio"] = ratioValidation(plan.ResolvedOutput.SAR, stringFromUnknown(output["sampleAspectRatio"]), 0.000001)
	}
	if strings.TrimSpace(plan.ResolvedOutput.DAR) != "" {
		actualDAR := stringFromUnknown(output["displayAspectRatio"])
		if ratioValue(actualDAR) <= 0 {
			actualDAR = aspectRatioString(actualWidth, actualHeight, stringFromUnknown(output["sampleAspectRatio"]))
		}
		fields["displayAspectRatio"] = ratioValidation(plan.ResolvedOutput.DAR, actualDAR, 0.01)
	}
	fields["frameRate"] = validateCadenceOutputFrameRate(map[string]interface{}(profile.WorkerConfig), output)
	if profileWorkerBool(profile, "effectiveOutputProgressive", false) {
		fieldOrder := strings.ToLower(strings.TrimSpace(stringFromUnknown(output["fieldOrder"])))
		status := "passed"
		if fieldOrder == "" || fieldOrder == "unknown" {
			status = "unverified"
		} else if fieldOrder != "progressive" {
			status = "mismatch"
		}
		fields["frameStructure"] = models.JSONMap{"expected": "progressive", "actual": fieldOrder, "status": status}
	}
	status := "passed"
	for _, raw := range fields {
		field := unknownRecord(raw)
		if field == nil {
			continue
		}
		if field["status"] == "mismatch" {
			status = "mismatch"
			break
		}
		if field["status"] == "unverified" {
			status = "unverified"
		}
	}
	if smartUpscale["status"] == "mismatch" {
		status = "mismatch"
	}
	report["fields"] = fields
	report["status"] = status
	report["validationResult"] = status
	return report
}

func validateSmartUpscaleOutput(profile models.Profile, source, output models.JSONMap) models.JSONMap {
	decision, ok := resolvedUpscaleDecisionFromProfile(profile)
	if !ok {
		return models.JSONMap{"status": "not_applicable"}
	}
	report := models.JSONMap{
		"requestedMode": decision.RequestedMode, "resolvedMode": decision.ResolvedMode,
		"upscaleApplied": decision.UpscaleApplied, "sharpenMode": decision.SharpenMode,
		"confidence": decision.Confidence, "reasons": decision.Reasons, "warnings": decision.Warnings,
		"sourceStorage":     models.JSONMap{"width": workerIntValue(source["width"], 0), "height": workerIntValue(source["height"], 0), "sar": stringFromUnknown(source["sampleAspectRatio"]), "dar": stringFromUnknown(source["displayAspectRatio"])},
		"effectiveGeometry": models.JSONMap{"width": decision.SourceWidth, "height": decision.SourceHeight, "sar": decision.SourceSAR, "dar": decision.SourceDAR},
		"resolvedOutput":    models.JSONMap{"width": decision.TargetWidth, "height": decision.TargetHeight, "sar": decision.TargetSAR, "dar": aspectRatioString(decision.TargetWidth, decision.TargetHeight, decision.TargetSAR)},
		"actualOutput":      models.JSONMap{"width": workerIntValue(output["width"], 0), "height": workerIntValue(output["height"], 0), "sar": stringFromUnknown(output["sampleAspectRatio"]), "dar": stringFromUnknown(output["displayAspectRatio"]), "frameRate": stringFromUnknown(output["frameRate"]), "realFrameRate": stringFromUnknown(output["realFrameRate"]), "fieldOrder": stringFromUnknown(output["fieldOrder"])},
	}
	if !decision.UpscaleApplied {
		report["status"] = "passed"
		report["validationResult"] = "keep_source"
		return report
	}

	fields := models.JSONMap{}
	actualWidth, actualHeight := workerIntValue(output["width"], 0), workerIntValue(output["height"], 0)
	geometryStatus := "passed"
	if actualWidth <= 0 || actualHeight <= 0 {
		geometryStatus = "unverified"
	} else if actualWidth != decision.TargetWidth || actualHeight != decision.TargetHeight {
		geometryStatus = "mismatch"
	}
	fields["geometry"] = models.JSONMap{"expectedWidth": decision.TargetWidth, "expectedHeight": decision.TargetHeight, "actualWidth": actualWidth, "actualHeight": actualHeight, "status": geometryStatus}
	fields["sampleAspectRatio"] = ratioValidation(decision.TargetSAR, stringFromUnknown(output["sampleAspectRatio"]), 0.000001)
	expectedDAR := aspectRatioValue(decision.TargetWidth, decision.TargetHeight, decision.TargetSAR)
	actualDAR := ratioValue(stringFromUnknown(output["displayAspectRatio"]))
	if actualDAR <= 0 {
		actualDAR = aspectRatioValue(actualWidth, actualHeight, stringFromUnknown(output["sampleAspectRatio"]))
	}
	darStatus := "passed"
	if expectedDAR <= 0 || actualDAR <= 0 {
		darStatus = "unverified"
	} else if !nearFPS(actualDAR, expectedDAR, 0.01) {
		darStatus = "mismatch"
	}
	fields["displayAspectRatio"] = models.JSONMap{"expected": aspectRatioString(decision.TargetWidth, decision.TargetHeight, decision.TargetSAR), "actual": stringFromUnknown(output["displayAspectRatio"]), "expectedValue": expectedDAR, "actualValue": actualDAR, "status": darStatus}
	fields["frameRate"] = validateCadenceOutputFrameRate(map[string]interface{}(profile.WorkerConfig), output)
	progressiveStatus := "not_applicable"
	if profileWorkerBool(profile, "effectiveOutputProgressive", false) {
		fieldOrder := strings.ToLower(strings.TrimSpace(stringFromUnknown(output["fieldOrder"])))
		progressiveStatus = "passed"
		if fieldOrder == "" || fieldOrder == "unknown" {
			progressiveStatus = "unverified"
		} else if fieldOrder != "progressive" {
			progressiveStatus = "mismatch"
		}
	}
	fields["frameStructure"] = models.JSONMap{"expected": "progressive", "actual": stringFromUnknown(output["fieldOrder"]), "status": progressiveStatus}

	status := "passed"
	for _, raw := range fields {
		field, _ := raw.(models.JSONMap)
		if field["status"] == "mismatch" {
			status = "mismatch"
			break
		}
		if field["status"] == "unverified" {
			status = "unverified"
		}
	}
	report["fields"] = fields
	report["status"] = status
	report["validationResult"] = status
	return report
}

func ratioValidation(expected, actual string, tolerance float64) models.JSONMap {
	expectedValue, actualValue := ratioValue(expected), ratioValue(actual)
	status := "passed"
	if expectedValue <= 0 || actualValue <= 0 {
		status = "unverified"
	} else if !nearFPS(expectedValue, actualValue, tolerance) {
		status = "mismatch"
	}
	return models.JSONMap{"expected": expected, "actual": actual, "expectedValue": expectedValue, "actualValue": actualValue, "status": status}
}

func ratioValue(value string) float64 {
	numerator, denominator, ok := parsePositiveRatio(value)
	if !ok {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func aspectRatioValue(width, height int, sar string) float64 {
	if width <= 0 || height <= 0 {
		return 0
	}
	sarValue := ratioValue(sar)
	if sarValue <= 0 {
		sarValue = 1
	}
	return float64(width) * sarValue / float64(height)
}

func aspectRatioString(width, height int, sar string) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	sarNum, sarDen, ok := parsePositiveRatio(sar)
	if !ok {
		sarNum, sarDen = 1, 1
	}
	numerator, denominator := reduceRatio(width*sarNum, height*sarDen)
	return fmt.Sprintf("%d:%d", numerator, denominator)
}

func validateRequiredSubtitleArtifacts(job models.QueueJob) ([]CheckResult, []string, models.JSONMap) {
	expected := []ResolvedTrackSidecar{}
	if plan, ok := ResolvedTrackPlanFromSnapshot(job.TrackProfileSnapshot); ok {
		expected = plan.SidecarOutputs
	}
	artifacts := subtitleArtifactsFromJSON(job.SubtitleArtifacts)
	byStream := map[int][]SubtitleArtifact{}
	for _, artifact := range artifacts {
		byStream[artifact.StreamIndex] = append(byStream[artifact.StreamIndex], artifact)
	}
	checks := []CheckResult{}
	warnings := []string{}
	reportItems := models.JSONList{}
	for _, decision := range expected {
		label := fmt.Sprintf("Subtitle %s sidecar for stream %d", strings.ToUpper(decision.Format), decision.StreamIndex)
		candidates := byStream[decision.StreamIndex]
		var artifact *SubtitleArtifact
		for index := range candidates {
			if decision.Format == "" || strings.EqualFold(candidates[index].Format, decision.Format) {
				candidate := candidates[index]
				artifact = &candidate
				break
			}
		}
		status, message := "passed", "Required subtitle sidecar is ready."
		item := models.JSONMap{"streamIndex": decision.StreamIndex, "language": decision.Language, "codec": decision.Codec, "requestedDisposition": subtitleDispositionForSidecar(job, decision.StreamIndex), "resolvedDisposition": subtitleDispositionForSidecar(job, decision.StreamIndex), "expectedFormat": decision.Format, "expectedMode": decision.Mode}
		if artifact == nil {
			status, message = "failed", "Requested subtitle sidecar is missing from the job artifact set."
		} else {
			item["artifact"] = artifact
			if artifact.Status != "" && artifact.Status != "ready" {
				status, message = "failed", "Requested subtitle sidecar is not ready: "+fallback(artifact.Error, artifact.Status)
			} else if !strings.EqualFold(filepath.Ext(artifact.StagedPath), "."+artifact.Format) || (decision.Format != "" && !strings.EqualFold(artifact.Format, decision.Format)) || (decision.Mode != "" && !strings.EqualFold(artifact.Mode, decision.Mode)) {
				status, message = "failed", "Subtitle sidecar extension or format does not match the resolved track plan."
			} else if info, err := os.Stat(artifact.StagedPath); err != nil || info.IsDir() {
				status, message = "failed", "Requested subtitle sidecar is not readable from staging."
			} else if info.Size() <= 0 {
				status, message = "failed", "Requested subtitle sidecar is empty."
			} else if content, err := os.ReadFile(artifact.StagedPath); err != nil || !validOriginalSubtitleSidecar(artifact.Format, content) {
				status, message = "failed", "Requested subtitle sidecar does not match its expected artifact type."
			}
			if decision.Language != "" && decision.Language != "und" && strings.TrimSpace(artifact.Language) == "" {
				warnings = append(warnings, fmt.Sprintf("Subtitle sidecar for stream %d is valid but language metadata is unavailable.", decision.StreamIndex))
			}
		}
		item["status"], item["message"] = status, message
		reportItems = append(reportItems, item)
		checks = append(checks, CheckResult{Key: fmt.Sprintf("subtitle_sidecar_%d_%s", decision.StreamIndex, strings.ToLower(decision.Format)), Label: label, Status: status, Message: message})
		if fallback(strings.ToLower(strings.TrimSpace(decision.Mode)), "original") == "original" &&
			(strings.EqualFold(decision.Format, "ass") || strings.EqualFold(decision.Format, "ssa")) &&
			shouldWarnMissingFontExport(job) {
			warnings = append(
				warnings,
				fmt.Sprintf(
					"Extracted %s sidecar for stream %d may reference custom fonts; source font attachments were not exported.",
					strings.ToUpper(decision.Format),
					decision.StreamIndex,
				),
			)
		}
	}
	return checks, warnings, models.JSONMap{"required": len(expected), "artifacts": artifacts, "checks": reportItems}
}

func shouldWarnMissingFontExport(job models.QueueJob) bool {
	plan, ok := ResolvedTrackPlanFromSnapshot(job.TrackProfileSnapshot)
	return ok && plan.FontAttachmentExportPolicy == FontAttachmentExportNone && hasSupportedFontAttachments(plan.AttachmentStreams)
}

func validateRequiredFontAttachmentArtifacts(job models.QueueJob) ([]CheckResult, []string, models.JSONMap) {
	expected := []ResolvedFontAttachment{}
	if plan, ok := ResolvedTrackPlanFromSnapshot(job.TrackProfileSnapshot); ok {
		expected = plan.FontAttachments
	}
	artifacts := fontAttachmentArtifactsFromJSON(job.SubtitleArtifacts)
	byID := map[string]FontAttachmentArtifact{}
	for _, artifact := range artifacts {
		byID[artifact.ArtifactID] = artifact
	}
	checks := []CheckResult{}
	reportItems := models.JSONList{}
	for _, decision := range expected {
		status, message := "passed", "Required font attachment is ready."
		artifact, exists := byID[decision.ArtifactID]
		expectedRelativePath := fontAttachmentRelativePath(job.MediaPath, decision.SafeFilename)
		item := models.JSONMap{"artifactId": decision.ArtifactID, "streamIndex": decision.StreamIndex, "attachmentOrdinal": decision.AttachmentOrdinal, "fontFormat": decision.FontFormat, "safeFilename": decision.SafeFilename}
		if !exists {
			status, message = "failed", "Required font attachment is missing from the job artifact set."
		} else {
			item["artifact"] = artifact
			if artifact.StreamIndex != decision.StreamIndex ||
				artifact.AttachmentOrdinal != decision.AttachmentOrdinal ||
				artifact.SafeFilename != decision.SafeFilename ||
				filepath.Clean(artifact.RelativePath) != filepath.Clean(expectedRelativePath) {
				status, message = "failed", "Font attachment identity or output path does not match the frozen track plan."
			} else if artifact.Status != "" && artifact.Status != "ready" {
				status, message = "failed", "Required font attachment is not ready: "+fallback(artifact.Error, artifact.Status)
			} else if filepath.Base(artifact.StagedPath) != decision.SafeFilename {
				status, message = "failed", "Font attachment staged filename does not match the frozen track plan."
			} else if info, err := os.Stat(artifact.StagedPath); err != nil || info.IsDir() {
				status, message = "failed", "Required font attachment is not readable from staging."
			} else if info.Size() <= 0 {
				status, message = "failed", "Required font attachment is empty."
			}
		}
		item["status"], item["message"] = status, message
		reportItems = append(reportItems, item)
		checks = append(checks, CheckResult{Key: fmt.Sprintf("font_attachment_%d", decision.StreamIndex), Label: fmt.Sprintf("Font attachment %s", decision.SafeFilename), Status: status, Message: message})
	}
	return checks, nil, models.JSONMap{"required": len(expected), "artifacts": artifacts, "checks": reportItems}
}

func subtitleDispositionForSidecar(job models.QueueJob, streamIndex int) SubtitleDisposition {
	if plan, ok := ResolvedTrackPlanFromSnapshot(job.TrackProfileSnapshot); ok {
		for _, subtitle := range plan.SubtitleStreams {
			if subtitle.StreamIndex == streamIndex {
				return subtitle.Action
			}
		}
	}
	return SubtitleDispositionExtract
}

func resolvedStreamPlanForJobValidation(db *gorm.DB, job models.QueueJob) (ResolvedStreamPlan, bool) {
	artifact, _, err := readLatestJobArtifact(db, job, "as-is")
	if err != nil {
		return ResolvedStreamPlan{}, false
	}
	raw, ok := artifact["streamPlan"]
	if !ok {
		return ResolvedStreamPlan{}, false
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return ResolvedStreamPlan{}, false
	}
	var plan ResolvedStreamPlan
	if json.Unmarshal(encoded, &plan) != nil || len(plan.Audio) == 0 {
		return ResolvedStreamPlan{}, false
	}
	return plan, true
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

func validateJobColorPolicy(db *gorm.DB, job models.QueueJob, probes ...map[string]any) (models.JSONMap, string) {
	policy := "preserve"
	worker := unknownRecord(job.ProfileSnapshot["workerConfig"])
	effectivePolicy := ""
	if worker != nil {
		if value := strings.ToLower(strings.TrimSpace(stringFromUnknown(worker["finalColorPolicy"]))); value != "" {
			policy = value
		}
		switch value := strings.ToLower(strings.TrimSpace(stringFromUnknown(worker["effectiveFinalColorPolicy"]))); value {
		case "preserve", "normalize_bt709":
			effectivePolicy = value
		}
	}
	overrideEntries := map[string]AssetConversionOverrideState(nil)
	if _, frozen := job.ProfileSnapshot[assetConversionOverrideSnapshotKey]; !frozen {
		// Legacy jobs may predate the Queue snapshot. Prepared jobs always use
		// the frozen override above; mutable Asset Overrides must not alter an
		// already-frozen validation expectation.
		overrideEntries = assetConversionOverrides(db)
	}
	override := conversionOverrideForJob(job, overrideEntries)
	if value := strings.ToLower(strings.TrimSpace(override.FinalColorPolicy)); value != "" {
		policy = value
	}
	var sourceProbe, outputProbe map[string]any
	if len(probes) > 0 {
		sourceProbe = probes[0]
	}
	if len(probes) > 1 {
		outputProbe = probes[1]
	}
	if sourceProbe == nil {
		sourceProbe = ffprobeJSON(job.MediaPath)
	}
	if outputProbe == nil {
		outputProbe = ffprobeJSON(job.OutputPath)
	}
	source := firstVideoColorCharacteristics(sourceProbe)
	output := firstVideoColorCharacteristics(outputProbe)
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
	if effectivePolicy != "" {
		effective = effectivePolicy
	} else {
		metadata := resolveColorMetadata(sourceStream)
		normalize := metadata.complete && (policy == "normalize_bt709" ||
			(policy == "automatic" && strings.Contains(strings.ToLower(job.Notes), "-c:v hevc_videotoolbox") && !sourceUsesBT709(sourceStream) && !sourceUsesHDRTransfer(sourceStream)))
		if normalize && !sourceUsesHDRTransfer(sourceStream) {
			effective = "normalize_bt709"
		}
	}
	if effective == "normalize_bt709" {
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
func hevcBitstreamEvidenceForValidation(path string, outputProbe map[string]any) models.JSONMap {
	output := firstVideoFrameCharacteristics(outputProbe)
	if !strings.EqualFold(stringFromUnknown(output["codec"]), "hevc") {
		return nil
	}
	structure, err := measureHEVCBitstreamStructure(context.Background(), path)
	evidence := hevcBitstreamEvidenceFromStructure(structure, err)
	evidence["stream"] = "0:v:0"
	evidence["probe"] = "ffmpeg_trace_headers"
	return evidence
}

func hevcBitstreamEvidenceFromStructure(structure HEVCBitstreamStructure, err error) models.JSONMap {
	evidence := models.JSONMap{
		"probeOutcome":           "unavailable",
		"levelEvidenceAvailable": structure.LevelKnown,
		"tierEvidenceAvailable":  structure.TierKnown,
		"generalLevelIdc":        structure.LevelIDC,
		"generalTierFlag":        structure.TierFlag,
		"generalLevel":           hevcLevelFromIDC(structure.LevelIDC),
		"generalTier":            map[int]string{0: "main", 1: "high"}[structure.TierFlag],
	}
	if err != nil {
		evidence["probeStatus"] = "unavailable"
		evidence["error"] = err.Error()
		return evidence
	}
	if structure.LevelKnown && structure.TierKnown {
		evidence["probeOutcome"] = "available"
	} else {
		evidence["probeStatus"] = "unavailable"
	}
	return evidence
}

func validateCadenceOutputFrameRate(worker map[string]interface{}, output models.JSONMap) models.JSONMap {
	if worker == nil {
		return models.JSONMap{"status": "not_applicable"}
	}
	expected := strings.TrimSpace(stringFromUnknown(worker["effectiveOutputFrameRate"]))
	if expected == "" {
		return models.JSONMap{"status": "not_applicable"}
	}
	observed := strings.TrimSpace(stringFromUnknown(output["frameRate"]))
	realObserved := strings.TrimSpace(stringFromUnknown(output["realFrameRate"]))
	expectedFPS := parseFrameRateValue(expected)
	avgFPS := parseFrameRateValue(observed)
	realFPS := parseFrameRateValue(realObserved)
	result := models.JSONMap{"expected": expected, "avgFrameRate": observed, "realFrameRate": realObserved}
	if expectedFPS <= 0 {
		result["status"] = "unverified"
		return result
	}
	if measuredFPS, frameCount, duration, ok := measuredFrameRate(output); ok {
		result["measuredFrameRate"] = measuredFPS
		result["measuredFrameCount"] = frameCount
		result["measuredDuration"] = duration
		if nearFPS(measuredFPS, expectedFPS, 0.01) {
			result["status"] = "validated"
		} else {
			result["status"] = "mismatch"
		}
		if avgFPS > 0 && !nearFPS(avgFPS, measuredFPS, 0.01) {
			result["streamRateDiagnostic"] = "avg_frame_rate differs from measured frame count/duration"
		}
		if realFPS > 0 && !nearFPS(realFPS, measuredFPS, 0.01) {
			result["baseRateDiagnostic"] = "r_frame_rate differs from measured frame count/duration"
		}
		return result
	}
	if avgFPS > 0 {
		if nearFPS(avgFPS, expectedFPS, 0.01) {
			result["status"] = "validated"
		} else {
			result["status"] = "mismatch"
		}
		if realFPS > 0 && !nearFPS(realFPS, avgFPS, 0.01) {
			result["baseRateDiagnostic"] = "r_frame_rate differs from avg_frame_rate; avg_frame_rate remains primary"
		}
		return result
	}
	result["status"] = "unverified"
	if realFPS > 0 {
		result["baseRateDiagnostic"] = "r_frame_rate is available only as corroborating metadata"
	}
	return result
}

func measuredFrameRate(output models.JSONMap) (rate, frameCount, duration float64, ok bool) {
	frameCount = jsonFloat64(output["frameCount"])
	duration = jsonFloat64(output["duration"])
	if frameCount <= 0 || duration <= 0 {
		return 0, frameCount, duration, false
	}
	return frameCount / duration, frameCount, duration, true
}

func validateHEVCLevelField(worker map[string]interface{}, source, output models.JSONMap, evidence ...models.JSONMap) models.JSONMap {
	if !strings.EqualFold(stringFromUnknown(output["codec"]), "hevc") {
		return models.JSONMap{"status": "not_applicable"}
	}
	mode := normalizedHEVCLevelMode(stringFromUnknown(worker["hevcLevelMode"]))
	if mode == "" {
		mode = "auto"
	}
	configuredEncoder := strings.ToLower(strings.TrimSpace(stringFromUnknown(worker["videoEncoder"])))
	if configuredEncoder != "" && configuredEncoder != "auto" && configuredEncoder != "libx265" && configuredEncoder != "hevc_qsv" {
		return models.JSONMap{"mode": mode, "status": "unavailable", "reason": "the selected runtime has no validated HEVC Level/Tier evidence mapping"}
	}
	expectedLevel := normalizedHEVCLevel(stringFromUnknown(worker["hevcLevelEffective"]))
	expectedTier := strings.ToLower(strings.TrimSpace(stringFromUnknown(worker["hevcLevelTier"])))
	if expectedTier == "" {
		expectedTier = "main"
	}
	bitstream := models.JSONMap{}
	if len(evidence) > 0 {
		bitstream = evidence[0]
	}
	actualLevel := normalizedHEVCLevel(stringFromUnknown(bitstream["generalLevel"]))
	actualTier := strings.ToLower(strings.TrimSpace(stringFromUnknown(bitstream["generalTier"])))
	if actualTier != "main" && actualTier != "high" {
		actualTier = ""
	}
	levelStatus := "unavailable"
	if boolSetting(bitstream["levelEvidenceAvailable"], false) && actualLevel != "" {
		levelStatus = "validated"
	}
	tierStatus := "unavailable"
	if boolSetting(bitstream["tierEvidenceAvailable"], false) && actualTier != "" {
		tierStatus = "validated"
	}
	status := "unavailable"
	if expectedLevel != "" && levelStatus == "validated" && actualLevel != expectedLevel {
		levelStatus = "mismatch"
	}
	if expectedTier != "" && tierStatus == "validated" && actualTier != expectedTier {
		tierStatus = "mismatch"
	}
	if levelStatus == "mismatch" || tierStatus == "mismatch" {
		status = "mismatch"
	} else if (expectedLevel == "" || expectedTier == "") && levelStatus == "validated" && tierStatus == "validated" {
		status = "unverified"
	} else if expectedLevel != "" && expectedTier != "" && levelStatus == "validated" && tierStatus == "validated" {
		status = "validated"
	} else if levelStatus == "unverified" || tierStatus == "unverified" {
		status = "unverified"
	}

	requirementsStatus := "unavailable"
	if expectedLimit, ok := hevcLevelLimitFor(expectedLevel); ok {
		width := workerIntValue(output["width"], 0)
		height := workerIntValue(output["height"], 0)
		fps := parseFrameRateValue(stringFromUnknown(output["frameRate"]))
		if width > 0 && height > 0 && fps > 0 {
			requirementsStatus = "validated"
			pictureSize := int64(width) * int64(height)
			if pictureSize > expectedLimit.MaxLumaPicture || float64(pictureSize)*fps > expectedLimit.MaxLumaRate {
				requirementsStatus = "mismatch"
				status = "mismatch"
			}
		}
	}
	observedBitrate := int64(workerIntValue(output["bitrate"], 0))
	bitrateStatus := "unavailable"
	if observedBitrate > 0 {
		// ffprobe average bit_rate is informative only. It cannot prove HEVC
		// MaxBR or CPB peak conformance.
		bitrateStatus = "unverified"
	}
	maxBitrate := int64(0)
	if limit, ok := hevcLevelLimitFor(actualLevel); ok {
		maxBitrate = limit.MaxBitrateKbps * 1000
	}
	return models.JSONMap{
		"mode": mode, "requested": expectedLevel, "effective": expectedLevel, "expectedLevel": expectedLevel, "actualLevel": actualLevel,
		"output": actualLevel, "status": status, "levelStatus": levelStatus, "expectedTier": expectedTier,
		"effectiveTier": expectedTier, "observedTier": actualTier, "tierStatus": tierStatus,
		"observedBitrate": observedBitrate, "maxMainTierBitrate": maxBitrate, "bitrateStatus": bitrateStatus,
		"requirementsStatus": requirementsStatus, "bitstreamEvidence": bitstream,
	}
}

// frameFidelityValue is retained only for legacy report/test compatibility.
// validateJobFrameFidelity no longer calls it: canonical field verdicts are
// produced from a frozen expectation and compare expected against actual.
func frameFidelityValue(source, output models.JSONMap, key string, allowedChange bool) models.JSONMap {
	from, to := stringFromUnknown(source[key]), stringFromUnknown(output[key])
	if from == "" || to == "" {
		return models.JSONMap{"source": from, "expected": to, "actual": to, "output": to, "authority": "legacy_compatibility", "status": "unverified"}
	}
	status := "preserved"
	if !strings.EqualFold(from, to) {
		if allowedChange {
			status = "changed_intentionally"
		} else {
			status = "changed_unexpectedly"
		}
	}
	return models.JSONMap{"source": from, "expected": to, "actual": to, "output": to, "authority": "legacy_compatibility", "status": status}
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
		rawColorRange := strings.TrimSpace(stringFromUnknown(stream["color_range"]))
		result := models.JSONMap{
			"codec":              strings.ToLower(strings.TrimSpace(stringFromUnknown(stream["codec_name"]))),
			"profile":            strings.TrimSpace(stringFromUnknown(stream["profile"])),
			"colorSpace":         strings.ToLower(strings.TrimSpace(stringFromUnknown(stream["color_space"]))),
			"colorTransfer":      strings.ToLower(strings.TrimSpace(stringFromUnknown(stream["color_transfer"]))),
			"colorPrimaries":     strings.ToLower(strings.TrimSpace(stringFromUnknown(stream["color_primaries"]))),
			"colorRange":         normalizedFFmpegRange(rawColorRange),
			"pixelFormat":        strings.ToLower(strings.TrimSpace(stringFromUnknown(stream["pix_fmt"]))),
			"bitDepth":           bitDepthFromPixelFormat(stringFromUnknown(stream["pix_fmt"])),
			"frameSize":          characteristicString(stream["width"]) + "x" + characteristicString(stream["height"]),
			"sampleAspectRatio":  strings.TrimSpace(stringFromUnknown(stream["sample_aspect_ratio"])),
			"displayAspectRatio": strings.TrimSpace(stringFromUnknown(stream["display_aspect_ratio"])),
			"frameRate":          strings.TrimSpace(stringFromUnknown(stream["avg_frame_rate"])),
			"realFrameRate":      strings.TrimSpace(stringFromUnknown(stream["r_frame_rate"])),
			"frameCount":         stream["nb_read_frames"],
			"duration":           stream["duration"],
			"width":              stream["width"],
			"height":             stream["height"],
			"bitrate":            stream["bit_rate"],
			"fieldOrder":         strings.ToLower(strings.TrimSpace(stringFromUnknown(stream["field_order"]))),
			"chromaLocation":     strings.ToLower(strings.TrimSpace(stringFromUnknown(stream["chroma_location"]))),
		}
		if _, ok := stream["nb_frames"]; ok {
			result["declaredFrameCount"] = stream["nb_frames"]
		}
		levelIDC := workerIntValue(stream["level"], 0)
		result["hevcLevelIdc"] = levelIDC
		result["hevcLevel"] = hevcLevelFromIDC(levelIDC)
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
