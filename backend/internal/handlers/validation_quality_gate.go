package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	validationSchemaVersion  = "validation-run-v1"
	validationEngineVersion  = "quality-gate-v1"
	validationRulesetID      = "mvforge-post-conversion"
	validationRulesetVersion = "1.0.0"
	validationOverrideV1     = "validation-override-v1"
)

type validationFinding struct {
	Key                string   `json:"key"`
	Expected           any      `json:"expected"`
	Actual             any      `json:"actual"`
	Severity           string   `json:"severity"`
	Disposition        string   `json:"disposition"`
	State              string   `json:"state"`
	Actionable         bool     `json:"actionable"`
	Overrideable       bool     `json:"overrideable"`
	Explanation        string   `json:"explanation"`
	EvidenceReferences []string `json:"evidenceReferences"`
}

type validationGateResult struct {
	Run      models.ValidationRun
	Findings []validationFinding
}

type validationSummaryPage struct {
	Items  []validationSummaryRow `json:"items"`
	Total  int64                  `json:"total"`
	Limit  int                    `json:"limit"`
	Offset int                    `json:"offset"`
}

type validationSummaryRow struct {
	ID                          uint                     `json:"id"`
	ExecutionNumber             *uint                    `json:"executionNumber,omitempty"`
	MediaPath                   string                   `json:"mediaPath"`
	OutputPath                  string                   `json:"outputPath,omitempty"`
	Status                      string                   `json:"status"`
	PublishedPath               string                   `json:"publishedPath,omitempty"`
	PublishedAt                 *time.Time               `json:"publishedAt,omitempty"`
	PublicationID               string                   `json:"publicationId,omitempty"`
	PublicationState            string                   `json:"publicationState,omitempty"`
	PublicationSummary          string                   `json:"publicationSummary,omitempty"`
	OriginalArchivedPath        string                   `json:"originalArchivedPath,omitempty"`
	OriginalArchiveStatus       string                   `json:"originalArchiveStatus,omitempty"`
	FinishedAt                  *time.Time               `json:"finishedAt,omitempty"`
	CreatedAt                   time.Time                `json:"createdAt"`
	UpdatedAt                   time.Time                `json:"updatedAt"`
	ValidationStatus            string                   `json:"validationStatus"`
	ValidationScore             int                      `json:"validationScore"`
	ValidationRunID             string                   `json:"validationRunId,omitempty"`
	ValidationOutcome           string                   `json:"validationOutcome,omitempty"`
	ValidationOverridePresent   bool                     `json:"validationOverridePresent"`
	ValidationHasActionableWarn bool                     `json:"validationHasActionableWarnings"`
	Publisher                   *historyPublisherSummary `json:"publisher,omitempty"`
}

type historyPublisherSummary struct {
	PublicationID              string                          `json:"publicationId"`
	State                      string                          `json:"state"`
	DestinationPath            string                          `json:"destinationPath"`
	PublishMode                string                          `json:"publishMode"`
	AttemptCount               int                             `json:"attemptCount"`
	LatestAttempt              *historyPublisherAttemptSummary `json:"latestAttempt,omitempty"`
	ValidationRunID            string                          `json:"validationRunId"`
	ValidationOverridePresent  bool                            `json:"validationOverridePresent"`
	ValidationOverrideEventIDs []string                        `json:"validationOverrideEventIds,omitempty"`
}

type historyPublisherAttemptSummary struct {
	Phase     string    `json:"phase"`
	Outcome   string    `json:"outcome"`
	Code      string    `json:"code,omitempty"`
	Summary   string    `json:"summary,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type validationOverrideRequest struct {
	Reason           string   `json:"reason"`
	RuleKeys         []string `json:"ruleKeys"`
	RequestedHandoff string   `json:"requestedHandoff"`
}

func (h ValidationHandler) validateAndPersist(job models.QueueJob, actorSourceInput string) (validationGateResult, error) {
	if job.Status != JobStatusCompleted {
		return validationGateResult{}, fmt.Errorf("job must have a completed output before Validation")
	}
	artifact, _, err := readLatestJobArtifact(h.db, job, "result")
	if err != nil {
		return validationGateResult{}, fmt.Errorf("canonical Result artifact is unavailable: %w", err)
	}
	identity, identityOK := resultIdentityFromArtifact(artifact)
	binding, bindingOK := resultBindingFromArtifact(artifact)
	if !identityOK || !bindingOK || artifactExecutionNumber(artifact) != binding.ExecutionNumber {
		return validationGateResult{}, fmt.Errorf("Result artifact is historical or unbound; queue a new execution to create a canonical Result")
	}
	findings, report := evaluateValidationRules(h.db, job, artifact, identity, binding)
	outcome, actionable := reduceValidationOutcome(findings)
	score := validationDisplayScore(findings)
	runToken := make([]byte, 16)
	if _, err := rand.Read(runToken); err != nil {
		return validationGateResult{}, err
	}
	runID := "validation-run:" + hex.EncodeToString(runToken)
	encodedFindings, err := json.Marshal(findings)
	if err != nil {
		return validationGateResult{}, err
	}
	planVersion := binding.ExecutionPlanVersion
	actorSource, actorID := "deployment", "local_operator"
	if actorSourceInput == "system" {
		actorSource, actorID = "system", "system"
	}
	createdAt := time.Now().UTC()
	executionNumber := artifactExecutionNumber(artifact)
	if executionNumber == 0 {
		executionNumber = binding.ExecutionNumber
	}
	run := models.ValidationRun{
		RunID: runID, JobID: job.ID, ExecutionNumber: executionNumber,
		ExecutionPlanID: binding.ExecutionPlanID, ExecutionPlanVersion: planVersion,
		ExecutionPlanFingerprint: binding.ExecutionPlanFingerprint,
		ResultArtifactID:         identity.ArtifactID, ResultGenerationToken: identity.GenerationToken,
		ResultGenerationVersion: identity.GenerationVersion, ResultOutputRevision: identity.OutputRevision,
		SchemaVersion: validationSchemaVersion, ValidatorVersion: validationEngineVersion,
		RulesetID: validationRulesetID, RulesetVersion: validationRulesetVersion,
		ActorSource: actorSource, ActorID: actorID,
		Outcome: outcome, Score: score, ActionableWarnings: actionable,
		Findings: models.JSONList{}, Report: report, CreatedAt: createdAt,
	}
	if err := json.Unmarshal(encodedFindings, &run.Findings); err != nil {
		return validationGateResult{}, err
	}
	run.Report["outcome"] = outcome
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		status := legacyValidationStatus(outcome)
		if err := tx.Model(&models.QueueJob{}).Where("id = ?", job.ID).Updates(map[string]any{
			"validation_run_id":     run.RunID,
			"validation_outcome":    outcome,
			"validation_status":     status,
			"validation_score":      score,
			"validation_report":     report,
			"validation_override":   false,
			"validation_actionable": actionable,
		}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return validationGateResult{}, err
	}
	return validationGateResult{Run: run, Findings: findings}, nil
}

func resultManifestEvidence(job models.QueueJob, artifact map[string]any) (models.JSONList, models.JSONMap, models.JSONMap, bool) {
	expectedIDs := make([]string, 0)
	for _, planned := range plannedSubtitleArtifacts(job) {
		if planned.ArtifactID != "" {
			expectedIDs = append(expectedIDs, planned.ArtifactID)
		}
	}
	for _, planned := range plannedFontAttachmentArtifacts(job) {
		if planned.ArtifactID != "" {
			expectedIDs = append(expectedIDs, planned.ArtifactID)
		}
	}
	sort.Strings(expectedIDs)

	expected := models.JSONMap{
		"primary":             models.JSONMap{"type": "primary_media", "path": filepath.Clean(job.OutputPath), "status": "ready"},
		"requiredArtifactIds": expectedIDs,
	}
	manifest := unknownRecord(artifact["artifacts"])
	if manifest == nil {
		return models.JSONList{}, expected, nil, false
	}
	primary := unknownRecord(manifest["primary"])
	rawSidecars, sidecarsPresent := manifest["sidecars"]
	var sidecars models.JSONList
	if sidecarsPresent && rawSidecars != nil {
		encoded, err := json.Marshal(rawSidecars)
		if err != nil || json.Unmarshal(encoded, &sidecars) != nil {
			return models.JSONList{}, expected, nil, false
		}
	}
	if len(expectedIDs) > 0 && (!sidecarsPresent || rawSidecars == nil) {
		return sidecars, expected, nil, false
	}

	actualIDs := make([]string, 0, len(sidecars))
	for _, raw := range sidecars {
		entry, _ := raw.(map[string]any)
		if id := strings.TrimSpace(stringFromUnknown(entry["artifactId"])); id != "" {
			actualIDs = append(actualIDs, id)
		}
	}
	sort.Strings(actualIDs)
	actual := models.JSONMap{"primary": primary, "artifactIds": actualIDs}
	primaryPath := filepath.Clean(stringFromUnknown(primary["path"]))
	matched := stringFromUnknown(primary["type"]) == "primary_media" &&
		primaryPath == filepath.Clean(job.OutputPath) && stringFromUnknown(primary["status"]) == "ready"
	actualSet := make(map[string]bool, len(actualIDs))
	for _, id := range actualIDs {
		actualSet[id] = true
	}
	for _, id := range expectedIDs {
		if !actualSet[id] {
			matched = false
		}
	}
	return sidecars, expected, actual, matched
}

func evaluateValidationRules(db *gorm.DB, job models.QueueJob, artifact map[string]any, identity resultArtifactIdentity, binding resultExecutionBinding) ([]validationFinding, models.JSONMap) {
	findings := make([]validationFinding, 0, 12)
	add := func(key string, expected, actual any, severity, disposition, state string, actionable, overrideable bool, explanation string, refs ...string) {
		findings = append(findings, validationFinding{Key: key, Expected: expected, Actual: actual, Severity: severity, Disposition: disposition, State: state, Actionable: actionable, Overrideable: overrideable, Explanation: explanation, EvidenceReferences: refs})
	}
	currentRevision, statErr := resultOutputRevisionForPath(job.OutputPath)
	identityMatches := statErr == nil && identity.OutputRevision != "" && currentRevision == identity.OutputRevision
	if identity.ArtifactID == "" || identity.GenerationToken == "" || identity.GenerationVersion < 1 || identity.OutputRevision == "" {
		add("artifact.identity", "canonical Result identity", nil, "critical", "blocking", "FAIL", true, false, "The Result artifact has no complete canonical generation and output identity.", "result.identity")
	} else if !identityMatches {
		actual := any(currentRevision)
		if statErr != nil {
			actual = statErr.Error()
		}
		add("artifact.identity", identity.OutputRevision, actual, "critical", "blocking", "FAIL", true, false, "The current output no longer matches the Result generation recorded at worker completion.", "result.identity", "filesystem.output")
	} else {
		add("artifact.identity", identity, identity, "info", "blocking", "PASS", false, false, "The canonical Result generation and bounded output revision match.", "result.identity")
	}
	resultSidecars, expectedManifest, actualManifest, manifestMatches := resultManifestEvidence(job, artifact)
	manifestState, manifestExplanation := "PASS", "The Result manifest identifies the produced output and all frozen required artifacts."
	if !manifestMatches {
		manifestState, manifestExplanation = "FAIL", "The Result manifest is missing, points at a different output, or omits a frozen required artifact."
	}
	add("artifact.manifest", expectedManifest, actualManifest, "critical", "blocking", manifestState, manifestState != "PASS", false, manifestExplanation, "result.artifactManifest", "queue.trackSnapshot")
	outputInfo, outputErr := os.Stat(job.OutputPath)
	if outputErr != nil || outputInfo.IsDir() || outputInfo.Size() <= 0 {
		actual := any("missing or unreadable")
		if outputInfo != nil {
			actual = outputInfo.Size()
		}
		add("artifact.integrity", "readable non-empty output", actual, "critical", "blocking", "FAIL", true, false, "The produced output is missing, unreadable, a directory, or empty.", "filesystem.output")
	} else {
		add("artifact.integrity", "readable non-empty output", outputInfo.Size(), "info", "blocking", "PASS", false, false, "The output is readable and non-empty.", "filesystem.output", "result.outputProbe")
	}
	if binding.ExecutionPlanID == 0 || binding.ExecutionPlanFingerprint == "" || artifactExecutionNumber(artifact) != binding.ExecutionNumber || !frozenBindingMatches(db, job, binding) {
		add("execution.binding", "active frozen Queue and ExecutionPlan", binding, "critical", "blocking", "REVIEW_REQUIRED", true, false, "The Result cannot be proven to belong to the active frozen Queue execution and ExecutionPlan.", "result.executionBinding", "queue.frozenSnapshots")
	} else {
		add("execution.binding", binding, binding, "info", "blocking", "PASS", false, false, "The Result is bound to the active frozen Queue snapshots and ExecutionPlan.", "result.executionBinding", "queue.frozenSnapshots")
	}

	outputProbe := unknownRecord(artifact["outputProbe"])
	if outputProbe == nil || outputProbe["error"] != nil {
		add("duration.timeline", "measured output duration and timeline", nil, "error", "blocking", "UNVERIFIED", true, true, "The worker Result artifact has no usable output probe evidence.", "result.outputProbe")
		add("video.properties", "frozen video properties", nil, "error", "blocking", "UNVERIFIED", true, true, "Output video properties are unavailable in the Result evidence.", "result.outputProbe")
		add("audio.tracks_actions", "frozen audio actions", nil, "error", "blocking", "UNVERIFIED", true, true, "Output audio streams cannot be compared with the frozen track plan.", "result.outputProbe", "queue.trackSnapshot")
		add("subtitles.streams_actions", "frozen subtitle actions", nil, "error", "blocking", "UNVERIFIED", true, true, "Output subtitle streams cannot be compared with the frozen track plan.", "result.outputProbe", "queue.trackSnapshot")
		add("chapters.actions", "frozen chapter action", nil, "warning", "advisory", "UNVERIFIED", true, true, "Chapter output evidence is unavailable.", "result.outputProbe", "queue.trackSnapshot")
		add("attachments.stream_actions", "frozen attachment actions", nil, "error", "blocking", "UNVERIFIED", true, true, "Attachment evidence is unavailable.", "result.outputProbe", "queue.trackSnapshot")
		add("attachments.font_artifacts", "required font artifacts", nil, "error", "blocking", "UNVERIFIED", true, true, "Font attachment evidence is unavailable.", "result.outputProbe", "queue.trackSnapshot")
	} else {
		actualDuration := validationProbeDuration(outputProbe)
		sourceProbe := sourceProbeForExecution(db, job, binding)
		expectedDuration := validationProbeDuration(sourceProbe)
		if expectedDuration <= 0 || actualDuration <= 0 {
			add("duration.timeline", expectedDuration, actualDuration, "error", "blocking", "UNVERIFIED", true, true, "Expected or actual timeline duration is unavailable from the frozen evidence.", "result.outputProbe", "queue.sourceEvidence")
		} else if absFloat(actualDuration-expectedDuration) > maxFloat(2, expectedDuration*0.02) {
			add("duration.timeline", expectedDuration, actualDuration, "error", "blocking", "FAIL", true, true, "Output duration differs from the source timeline beyond the bounded tolerance.", "result.outputProbe", "queue.sourceEvidence")
		} else {
			add("duration.timeline", expectedDuration, actualDuration, "info", "blocking", "PASS", false, false, "Output duration is within the bounded timeline tolerance.", "result.outputProbe", "queue.sourceEvidence")
		}
		streamPlan, planAvailable := resolvedStreamPlanForJobValidation(db, job)
		sourceTiming, outputTiming := avTimingFromProbe(sourceProbe), avTimingFromProbe(outputProbe)
		sourceTiming, outputTiming = avTimingForQueueValidation(sourceTiming, outputTiming, streamPlan, planAvailable)
		timingReport := validateAVTiming(sourceTiming, outputTiming)
		timingState, timingDisposition, timingSeverity, timingActionable := "PASS", "blocking", "info", false
		switch stringFromUnknown(timingReport["status"]) {
		case "unverified":
			timingState, timingSeverity, timingActionable = "UNVERIFIED", "error", true
		case "mismatch":
			timingState, timingSeverity, timingActionable = "FAIL", "error", true
		case "warning":
			timingState, timingDisposition, timingSeverity, timingActionable = "WARNING", "advisory", "warning", true
		}
		add("timeline.av_alignment", sourceTiming, outputTiming, timingSeverity, timingDisposition, timingState, timingActionable, timingState != "PASS", "Measured A/V timestamp and frame timing uses the worker Result probe and source AS-IS evidence.", "result.outputProbe", "queue.sourceEvidence", "queue.trackSnapshot")
		video := firstStreamByType(outputProbe, "video")
		if video == nil {
			add("video.properties", "at least one planned video stream", 0, "critical", "blocking", "FAIL", true, false, "The output contains no video stream for the frozen conversion plan.", "result.outputProbe", "executionPlan")
		} else {
			profile, profileErr := scheduler.RestoreProfileSnapshot(job.ProfileSnapshot)
			actual := models.JSONMap{"codec": video["codec_name"], "width": video["width"], "height": video["height"], "frameRate": video["avg_frame_rate"], "fieldOrder": video["field_order"], "pixelFormat": video["pix_fmt"], "bitDepth": video["bits_per_raw_sample"], "colorSpace": video["color_space"], "colorTransfer": video["color_transfer"], "colorPrimaries": video["color_primaries"]}
			if profileErr != nil {
				add("video.properties", "frozen ProfileSnapshot properties", actual, "error", "blocking", "UNVERIFIED", true, true, "Frozen video expectations could not be restored.", "result.outputProbe", "queue.profileSnapshot")
			} else {
				expected, actualVideo, state := evaluateFrozenVideoProperties(db, job, binding, profile, video, outputProbe)
				add("video.properties", expected, actualVideo, "error", "blocking", state, state != "PASS", true, "Output codec, frozen profile constraints, and required frame properties are compared with the active ExecutionPlan and Queue snapshots.", "result.outputProbe", "queue.profileSnapshot", "executionPlan")
			}
		}
		addTrackFindings(add, job, outputProbe)
	}

	manifestJob := job
	manifestJob.SubtitleArtifacts = resultSidecars
	subtitleChecks, _, subtitleReport := validateRequiredSubtitleArtifacts(manifestJob)
	addArtifactChecks(add, "sidecars.package", "required subtitle sidecars", subtitleChecks, "queue.subtitleArtifacts")
	fontChecks, _, fontReport := validateRequiredFontAttachmentArtifacts(manifestJob)
	if len(findings) > 0 && (outputProbe == nil || outputProbe["error"] != nil) {
		// Attachment evidence is already represented as unavailable above.
	} else {
		addArtifactChecks(add, "attachments.font_artifacts", "required font attachments", fontChecks, "queue.subtitleArtifacts", "queue.trackSnapshot")
	}

	colorReport := models.JSONMap{}
	frameReport := models.JSONMap{}
	if outputProbe != nil && outputProbe["error"] == nil {
		sourceProbe := sourceProbeForExecution(db, job, binding)
		if sourceProbe != nil {
			colorReport, _ = validateJobColorPolicyFrozen(job, sourceProbe, outputProbe)
			profile, _ := scheduler.RestoreProfileSnapshot(job.ProfileSnapshot)
			smart := validateSmartUpscaleOutput(profile, firstVideoFrameCharacteristics(sourceProbe), firstVideoFrameCharacteristics(outputProbe))
			restoration := validateRestorationOutput(profile, firstVideoFrameCharacteristics(sourceProbe), firstVideoFrameCharacteristics(outputProbe), smart)
			authority := fidelityAuthority{name: "queue_profile_snapshot", profile: profile, worker: map[string]interface{}(profile.WorkerConfig)}
			frameReport = evaluateCanonicalFidelity(authority, sourceProbe, outputProbe, nil, colorReport, smart, restoration)
		}
	}
	addChapterFinding(add, job, outputProbe)
	add("publication.readiness", "complete Result package with no blocking findings", "evaluated from findings", "info", "blocking", "PASS", false, false, "Publisher performs destination and filesystem checks; Validation only checks the produced package and rule findings.", "result.identity", "result.outputProbe", "queue.subtitleArtifacts")
	setPublicationReadiness(findings)

	report := models.JSONMap{"rulesetId": validationRulesetID, "rulesetVersion": validationRulesetVersion, "findings": findings}
	if len(colorReport) > 0 {
		report["colorPolicy"] = colorReport
	}
	if len(frameReport) > 0 {
		report["frameFidelity"] = frameReport
	}
	report["subtitleArtifacts"] = subtitleReport
	report["fontAttachmentArtifacts"] = fontReport
	if perceptual, ok := identityMatchedPerceptualEvidence(unknownRecord(artifact["validationReport"]), identity); ok {
		report["perceptualEvidence"] = perceptual
	}
	return findings, report
}

func addTrackFindings(add func(string, any, any, string, string, string, bool, bool, string, ...string), job models.QueueJob, probe models.JSONMap) {
	plan, ok := ResolvedTrackPlanFromSnapshot(job.TrackProfileSnapshot)
	if !ok {
		add("audio.tracks_actions", "frozen audio action plan", countProbeStreams(probe, "audio"), "error", "blocking", "UNVERIFIED", true, true, "The frozen track snapshot has no resolved audio plan.", "queue.trackSnapshot", "result.outputProbe")
		add("subtitles.streams_actions", "frozen subtitle action plan", countProbeStreams(probe, "subtitle"), "error", "blocking", "UNVERIFIED", true, true, "The frozen track snapshot has no resolved subtitle plan.", "queue.trackSnapshot", "result.outputProbe")
		add("attachments.stream_actions", "frozen attachment action plan", countProbeStreams(probe, "attachment"), "error", "blocking", "UNVERIFIED", true, true, "The frozen track snapshot has no resolved attachment plan.", "queue.trackSnapshot", "result.outputProbe")
		return
	}
	expectedAudio := len(plan.AudioStreams)
	actualAudio := countProbeStreams(probe, "audio")
	audioState := "PASS"
	if actualAudio != expectedAudio {
		audioState = "FAIL"
	}
	add("audio.tracks_actions", expectedAudio, actualAudio, "error", "blocking", audioState, audioState != "PASS", true, "Output audio membership is compared with the frozen selected and removed stream actions.", "queue.trackSnapshot", "result.outputProbe", "executionPlan")
	expectedSubtitles := 0
	for _, track := range plan.SubtitleStreams {
		if track.Action == SubtitleDispositionKeep || track.Action == SubtitleDispositionKeepAndExtract {
			expectedSubtitles++
		}
	}
	actualSubtitles := countProbeStreams(probe, "subtitle")
	subtitleState := "PASS"
	if actualSubtitles != expectedSubtitles {
		subtitleState = "FAIL"
	}
	add("subtitles.streams_actions", expectedSubtitles, actualSubtitles, "error", "blocking", subtitleState, subtitleState != "PASS", true, "Embedded subtitle membership is compared with the frozen keep/remove/extract actions.", "queue.trackSnapshot", "result.outputProbe")
	expectedAttachments := 0
	if plan.AttachmentsKept {
		expectedAttachments = len(plan.AttachmentStreams)
	}
	actualAttachments := countProbeStreams(probe, "attachment")
	attachmentState := "PASS"
	if actualAttachments != expectedAttachments {
		attachmentState = "FAIL"
	}
	add("attachments.stream_actions", expectedAttachments, actualAttachments, "error", "blocking", attachmentState, attachmentState != "PASS", true, "Output attachments are compared with the frozen attachment keep/remove action.", "queue.trackSnapshot", "result.outputProbe")
}

func evaluateFrozenVideoProperties(db *gorm.DB, job models.QueueJob, binding resultExecutionBinding, profile models.Profile, video models.JSONMap, outputProbe models.JSONMap) (models.JSONMap, models.JSONMap, string) {
	expectedCodec := normalizeVideoCodec(profile.VideoCodec)
	if plan, ok := activeExecutionPlanForValidation(db, job, binding); ok {
		if planned := normalizeVideoCodec(plan.SelectedEncoder); planned != "" {
			expectedCodec = planned
		}
	}
	sourceProbe := sourceProbeForExecution(db, job, binding)
	if expectedCodec == "copy" && sourceProbe != nil {
		if source := firstStreamByType(sourceProbe, "video"); source != nil {
			expectedCodec = normalizeVideoCodec(stringFromUnknown(source["codec_name"]))
		}
	} else if expectedCodec == "copy" {
		expectedCodec = ""
	}
	actualCodec := normalizeVideoCodec(stringFromUnknown(video["codec_name"]))
	expectedLevel := strings.TrimSpace(stringFromUnknown(profile.WorkerConfig["hevcLevel"]))
	if expectedLevel == "" {
		expectedLevel = strings.TrimSpace(stringFromUnknown(profile.WorkerConfig["level"]))
	}
	expectedTier := strings.TrimSpace(stringFromUnknown(profile.WorkerConfig["hevcTier"]))
	if expectedCodec != "hevc" {
		expectedLevel = ""
		expectedTier = ""
	}
	actualLevel := normalizedProbeLevel(video["level"])
	actualTier := strings.TrimSpace(stringFromUnknown(video["tier"]))
	expected := models.JSONMap{"codec": expectedCodec, "level": expectedLevel, "tier": expectedTier}
	actual := models.JSONMap{
		"codec": stringFromUnknown(video["codec_name"]), "normalizedCodec": actualCodec,
		"level": actualLevel, "tier": actualTier, "width": video["width"], "height": video["height"],
		"frameRate": video["avg_frame_rate"], "fieldOrder": video["field_order"],
		"pixelFormat": video["pix_fmt"], "bitDepth": video["bits_per_raw_sample"],
		"colorSpace": video["color_space"], "colorTransfer": video["color_transfer"], "colorPrimaries": video["color_primaries"],
	}
	state := "PASS"
	hasExpectation := expectedCodec != ""
	if expectedCodec == "" {
		state = "UNVERIFIED"
	}
	if expectedCodec != "" && actualCodec != expectedCodec {
		state = "FAIL"
	}
	if expectedLevel != "" {
		hasExpectation = true
		normalizedExpected := normalizeLevelString(expectedLevel)
		if actualLevel == "" {
			if state != "FAIL" {
				state = "UNVERIFIED"
			}
		} else if normalizeLevelString(actualLevel) != normalizedExpected {
			state = "FAIL"
		}
	}
	if expectedTier != "" {
		hasExpectation = true
		if actualTier == "" {
			if state != "FAIL" {
				state = "UNVERIFIED"
			}
		} else if !strings.EqualFold(actualTier, expectedTier) {
			state = "FAIL"
		}
	}
	frameReport := models.JSONMap{}
	if sourceProbe != nil {
		color, _ := validateJobColorPolicyFrozen(job, sourceProbe, outputProbe)
		authority := fidelityAuthority{name: "queue_profile_snapshot", profile: profile, worker: map[string]interface{}(profile.WorkerConfig)}
		frameReport = evaluateCanonicalFidelity(authority, sourceProbe, outputProbe, nil, color)
		if frameExpected := unknownRecord(frameReport["expected"]); frameExpected != nil {
			expected["frameProperties"] = frameExpected
		}
		if frameActual := unknownRecord(frameReport["actual"]); frameActual != nil {
			actual["frameProperties"] = frameActual
		}
		for _, raw := range unknownRecord(frameReport["fields"]) {
			field := unknownRecord(raw)
			if field["required"] != true {
				continue
			}
			hasExpectation = true
			switch stringFromUnknown(field["status"]) {
			case "mismatch":
				state = "FAIL"
			case "unverified":
				if state != "FAIL" {
					state = "UNVERIFIED"
				}
			}
		}
	}
	if !hasExpectation && state == "PASS" {
		state = "UNVERIFIED"
	}
	return expected, actual, state
}

func normalizeVideoCodec(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch {
	case value == "copy" || value == "passthrough":
		return "copy"
	case strings.Contains(value, "hevc") || strings.Contains(value, "265"):
		return "hevc"
	case strings.Contains(value, "h264") || strings.Contains(value, "264") || value == "avc":
		return "h264"
	case strings.Contains(value, "av1"):
		return "av1"
	case value == "mpeg2" || value == "mpeg2video":
		return "mpeg2video"
	default:
		return value
	}
}

func normalizedProbeLevel(value any) string {
	if value == nil {
		return ""
	}
	level := workerIntValue(value, 0)
	if level > 0 {
		return fmt.Sprintf("%d.%d", level/10, level%10)
	}
	return strings.TrimSpace(stringFromUnknown(value))
}

func normalizeLevelString(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed := workerIntValue(value, 0); parsed >= 10 {
		return fmt.Sprintf("%d.%d", parsed/10, parsed%10)
	}
	return value
}

func addArtifactChecks(add func(string, any, any, string, string, string, bool, bool, string, ...string), key, expected string, checks []CheckResult, refs ...string) {
	if len(checks) == 0 {
		add(key, expected, "no required artifacts", "info", "blocking", "NOT_APPLICABLE", false, false, "The frozen plan requires no artifacts in this category.", refs...)
		return
	}
	failed := false
	unverified := false
	for _, check := range checks {
		if check.Status == "failed" {
			failed = true
			break
		}
		if check.Status == "unverified" {
			unverified = true
		}
	}
	state := "PASS"
	if failed {
		state = "FAIL"
	} else if unverified {
		state = "UNVERIFIED"
	}
	add(key, expected, checks, "error", "blocking", state, failed || unverified, true, "Required artifact evidence is checked without modifying or regenerating the artifact.", refs...)
}

func addChapterFinding(add func(string, any, any, string, string, string, bool, bool, string, ...string), job models.QueueJob, probe models.JSONMap) {
	plan, ok := ResolvedTrackPlanFromSnapshot(job.TrackProfileSnapshot)
	if !ok || probe == nil {
		add("chapters.actions", "frozen chapter policy", nil, "warning", "advisory", "UNVERIFIED", true, true, "Chapter expectations or output probe evidence are unavailable.", "queue.profileSnapshot", "result.outputProbe")
		return
	}
	expected := plan.ChaptersKept
	actual := len(anySlice(probe["chapters"]))
	state := "PASS"
	if expected && actual == 0 {
		state = "FAIL"
	} else if !expected && actual > 0 {
		state = "FAIL"
	}
	add("chapters.actions", expected, actual, "warning", "advisory", state, state != "PASS", true, "Output chapter presence is checked against the frozen chapter policy.", "queue.profileSnapshot", "result.outputProbe")
}

func setPublicationReadiness(findings []validationFinding) {
	readiness := -1
	for i := range findings {
		if findings[i].Key == "publication.readiness" {
			readiness = i
			break
		}
	}
	if readiness < 0 {
		return
	}
	for _, finding := range findings {
		if finding.Key == "publication.readiness" || finding.Disposition != "blocking" {
			continue
		}
		if finding.State == "FAIL" {
			findings[readiness].State = "FAIL"
			findings[readiness].Actionable = true
			findings[readiness].Actual = "blocked by " + finding.Key
			return
		}
		if finding.State == "REVIEW_REQUIRED" || finding.State == "UNVERIFIED" {
			findings[readiness].State = "REVIEW_REQUIRED"
			findings[readiness].Actionable = true
			findings[readiness].Actual = "unresolved at " + finding.Key
			return
		}
	}
}

func reduceValidationOutcome(findings []validationFinding) (string, bool) {
	unresolved := false
	warning := false
	actionable := false
	for _, finding := range findings {
		if finding.Disposition == "blocking" && finding.State == "FAIL" {
			return "FAIL", hasActionableFinding(findings)
		}
		if finding.Disposition == "blocking" && (finding.State == "REVIEW_REQUIRED" || finding.State == "UNVERIFIED") {
			unresolved = true
		}
		if finding.State == "WARNING" || (finding.Disposition == "advisory" && (finding.State == "FAIL" || finding.State == "UNVERIFIED" || finding.State == "REVIEW_REQUIRED")) {
			warning = true
		}
		if finding.Actionable && (finding.State == "WARNING" || finding.State == "FAIL" || finding.State == "UNVERIFIED" || finding.State == "REVIEW_REQUIRED") {
			actionable = true
		}
	}
	if unresolved {
		return "REVIEW_REQUIRED", actionable
	}
	if warning {
		return "PASS_WITH_WARNINGS", actionable
	}
	return "PASS", false
}

func validationDisplayScore(findings []validationFinding) int {
	score := 100
	for _, finding := range findings {
		switch {
		case finding.State == "FAIL" && finding.Disposition == "blocking":
			score -= 25
		case finding.State == "WARNING" || finding.State == "UNVERIFIED":
			score -= 5
		}
	}
	if score < 0 {
		return 0
	}
	return score
}

func hasActionableFinding(findings []validationFinding) bool {
	for _, finding := range findings {
		if finding.Actionable {
			return true
		}
	}
	return false
}

func legacyValidationStatus(outcome string) string {
	switch outcome {
	case "PASS":
		return ValidationStatusPassed
	case "PASS_WITH_WARNINGS":
		return ValidationStatusWarning
	case "FAIL":
		return ValidationStatusFailed
	default:
		return ValidationStatusPending
	}
}

func resultIdentityFromArtifact(artifact map[string]any) (resultArtifactIdentity, bool) {
	raw := unknownRecord(artifact["resultIdentity"])
	if raw == nil {
		return resultArtifactIdentity{}, false
	}
	identity := resultArtifactIdentity{
		ArtifactID: stringFromUnknown(raw["artifactId"]), GenerationToken: stringFromUnknown(raw["generationToken"]),
		GenerationVersion: workerIntValue(raw["generationVersion"], 0), OutputRevision: stringFromUnknown(raw["outputRevision"]),
	}
	return identity, identity.ArtifactID != "" && identity.GenerationToken != "" && identity.OutputRevision != ""
}

func resultBindingFromArtifact(artifact map[string]any) (resultExecutionBinding, bool) {
	raw := unknownRecord(artifact["executionBinding"])
	if raw == nil {
		return resultExecutionBinding{}, false
	}
	binding := resultExecutionBinding{
		ExecutionNumber: uintFromUnknown(raw["executionNumber"]), ExecutionPlanID: uintFromUnknown(raw["executionPlanId"]),
		ExecutionPlanVersion: workerIntValue(raw["executionPlanVersion"], 0), ExecutionPlanFingerprint: stringFromUnknown(raw["executionPlanFingerprint"]),
		FrozenQueueFingerprint: stringFromUnknown(raw["frozenQueueFingerprint"]),
	}
	return binding, binding.ExecutionPlanID != 0 && binding.ExecutionPlanFingerprint != "" && binding.FrozenQueueFingerprint != ""
}

func artifactExecutionNumber(artifact map[string]any) uint {
	job := unknownRecord(artifact["job"])
	return uintFromUnknown(job["executionNumber"])
}

func activeExecutionPlanForValidation(db *gorm.DB, job models.QueueJob, binding resultExecutionBinding) (models.ExecutionPlan, bool) {
	var plan models.ExecutionPlan
	if job.ActiveExecutionPlanID == nil || *job.ActiveExecutionPlanID != binding.ExecutionPlanID || db.First(&plan, binding.ExecutionPlanID).Error != nil || plan.JobID != job.ID || plan.Version != binding.ExecutionPlanVersion {
		return plan, false
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		return plan, false
	}
	sum := sha256Bytes(encoded)
	return plan, sum == binding.ExecutionPlanFingerprint
}

func frozenBindingMatches(db *gorm.DB, job models.QueueJob, binding resultExecutionBinding) bool {
	if job.ExecutionNumber == nil || *job.ExecutionNumber != binding.ExecutionNumber || job.ActiveExecutionPlanID == nil || *job.ActiveExecutionPlanID != binding.ExecutionPlanID {
		return false
	}
	queueFingerprint, err := canonicalEvidenceHash("frozen-queue-snapshot-v1", models.JSONMap{
		"jobId": job.ID, "executionNumber": binding.ExecutionNumber, "profileVersion": job.ProfileVersion,
		"profileSnapshot": job.ProfileSnapshot, "audioProfileKey": job.AudioProfileKey,
		"audioProfileSnapshot": job.AudioProfileSnapshot, "trackProfileKey": job.TrackProfileKey,
		"trackProfileSnapshot": job.TrackProfileSnapshot, "profileResolution": job.ProfileResolution,
		"planId": binding.ExecutionPlanID, "planVersion": binding.ExecutionPlanVersion, "planFingerprint": binding.ExecutionPlanFingerprint,
	})
	if err != nil || queueFingerprint != binding.FrozenQueueFingerprint {
		return false
	}
	_, planMatches := activeExecutionPlanForValidation(db, job, binding)
	return planMatches
}

func sourceProbeForExecution(db *gorm.DB, job models.QueueJob, binding resultExecutionBinding) models.JSONMap {
	artifact, _, err := readLatestJobArtifact(db, job, "as-is")
	if err != nil {
		return nil
	}
	if artifactExecutionNumber(artifact) != binding.ExecutionNumber {
		return nil
	}
	return unknownRecord(artifact["sourceProbe"])
}

func videoCharacteristicsFromStream(stream models.JSONMap) models.JSONMap {
	return models.JSONMap{"width": stream["width"], "height": stream["height"], "sampleAspectRatio": stream["sample_aspect_ratio"], "displayAspectRatio": stream["display_aspect_ratio"], "frameRate": stream["avg_frame_rate"], "realFrameRate": stream["r_frame_rate"], "fieldOrder": stream["field_order"], "codec": stream["codec_name"], "pixelFormat": stream["pix_fmt"], "bitDepth": stream["bits_per_raw_sample"]}
}

func firstStreamByType(probe models.JSONMap, streamType string) models.JSONMap {
	for _, raw := range anySlice(probe["streams"]) {
		stream := unknownRecord(raw)
		if strings.EqualFold(stringFromUnknown(stream["codec_type"]), streamType) {
			return stream
		}
	}
	return nil
}

func countProbeStreams(probe models.JSONMap, streamType string) int {
	count := 0
	for _, raw := range anySlice(probe["streams"]) {
		if strings.EqualFold(stringFromUnknown(unknownRecord(raw)["codec_type"]), streamType) {
			count++
		}
	}
	return count
}

func anySlice(value any) []any {
	if values, ok := value.([]any); ok {
		return values
	}
	return nil
}

func identityMatchedPerceptualEvidence(report map[string]any, identity resultArtifactIdentity) (any, bool) {
	if report == nil {
		return nil, false
	}
	evidence := unknownRecord(report["perceptualEvidence"])
	for _, raw := range anySlice(evidence["candidateManifest"]) {
		candidate := unknownRecord(unknownRecord(raw)["candidate"])
		if stringFromUnknown(candidate["artifactId"]) == identity.ArtifactID && stringFromUnknown(candidate["generationToken"]) == identity.GenerationToken && workerIntValue(candidate["generationVersion"], 0) == identity.GenerationVersion && stringFromUnknown(candidate["outputRevision"]) == identity.OutputRevision {
			return evidence, true
		}
	}
	return nil, false
}

func bindPerceptualInputToResult(input perceptualArtifactInput, identity resultArtifactIdentity) perceptualArtifactInput {
	input.CandidateKind = "final_output"
	input.CandidateArtifactID = identity.ArtifactID
	input.GenerationToken = identity.GenerationToken
	input.GenerationVersion = int64(identity.GenerationVersion)
	input.CandidateRevision = identity.OutputRevision
	return input
}

func sha256Bytes(value []byte) string {
	hash := sha256.Sum256(value)
	return hex.EncodeToString(hash[:])
}

func (h ValidationHandler) RunDetail(c *gin.Context) {
	var run models.ValidationRun
	if err := h.db.Where("run_id = ? AND job_id = ?", c.Param("runId"), c.Param("id")).First(&run).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "validation run not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var overrides []models.ValidationOverride
	if err := h.db.Where("validation_run_id = ?", run.RunID).Order("created_at asc, id asc").Find(&overrides).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"run": run, "overrides": overrides})
}

func (h ValidationHandler) ReviewList(c *gin.Context) {
	view := strings.TrimSpace(c.DefaultQuery("view", "exceptions"))
	if view != "exceptions" && view != "pending" && view != "all" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "view must be exceptions, pending, or all"})
		return
	}
	limit := boundedValidationQueryInt(c.Query("limit"), 50, 1, 100)
	offset := boundedValidationQueryInt(c.Query("offset"), 0, 0, int(^uint(0)>>1))
	query := h.db.Model(&models.QueueJob{}).Where("status = ? AND published_at IS NULL", JobStatusCompleted)
	switch view {
	case "exceptions":
		query = query.Where("(validation_outcome IN ? OR validation_actionable = ? OR (validation_run_id = '' AND validation_status IN ?))", []string{"REVIEW_REQUIRED", "FAIL"}, true, []string{ValidationStatusPassed, ValidationStatusWarning, ValidationStatusFailed})
	case "pending":
		query = query.Where("(validation_run_id = '' OR validation_run_id IS NULL) AND (validation_status = '' OR validation_status = ?)", ValidationStatusPending)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var jobs []models.QueueJob
	selectFields := "id, execution_number, media_path, output_path, status, finished_at, created_at, updated_at, validation_status, validation_score, validation_run_id, validation_outcome, validation_override, validation_actionable, published_at"
	if err := query.Select(selectFields).Order("updated_at desc, id desc").Limit(limit).Offset(offset).Find(&jobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, validationSummaryPage{Items: validationSummaryRows(jobs), Total: total, Limit: limit, Offset: offset})
}

func (h ValidationHandler) HistoryList(c *gin.Context) {
	limit := boundedValidationQueryInt(c.Query("limit"), 50, 1, 100)
	offset := boundedValidationQueryInt(c.Query("offset"), 0, 0, int(^uint(0)>>1))
	query := h.db.Model(&models.QueueJob{}).Where("status IN ? OR published_at IS NOT NULL", []string{JobStatusCompleted, JobStatusFailed, JobStatusCanceled})
	if search := strings.TrimSpace(c.Query("q")); search != "" {
		like := "%" + search + "%"
		query = query.Where("(CAST(id AS TEXT) LIKE ? OR media_path LIKE ? OR published_path LIKE ? OR status LIKE ? OR validation_outcome LIKE ?)", like, like, like, like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	direction := "desc"
	if strings.EqualFold(c.Query("sort"), "asc") {
		direction = "asc"
	}
	var jobs []models.QueueJob
	selectFields := "id, execution_number, media_path, status, published_path, published_at, original_archived_path, original_archive_status, created_at, updated_at, validation_status, validation_score, validation_run_id, validation_outcome, validation_override, validation_actionable"
	if err := query.Select(selectFields).Order("id " + direction).Limit(limit).Offset(offset).Find(&jobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	rows, err := historySummaryRows(h.db, jobs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, validationSummaryPage{Items: rows, Total: total, Limit: limit, Offset: offset})
}

func validationSummaryRows(jobs []models.QueueJob) []validationSummaryRow {
	rows := make([]validationSummaryRow, 0, len(jobs))
	for _, job := range jobs {
		rows = append(rows, validationSummaryRow{
			ID: job.ID, ExecutionNumber: job.ExecutionNumber, MediaPath: job.MediaPath,
			OutputPath: job.OutputPath, Status: job.Status, PublishedPath: job.PublishedPath,
			PublishedAt: job.PublishedAt, PublicationID: job.PublicationID, PublicationState: job.PublicationState,
			PublicationSummary: job.PublicationSummary, OriginalArchivedPath: job.OriginalArchivedPath,
			OriginalArchiveStatus: job.OriginalArchiveStatus, FinishedAt: job.FinishedAt, CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt,
			ValidationStatus: job.ValidationStatus, ValidationScore: job.ValidationScore,
			ValidationRunID: job.ValidationRunID, ValidationOutcome: job.ValidationOutcome,
			ValidationOverridePresent: job.ValidationOverride, ValidationHasActionableWarn: job.ValidationActionable,
		})
	}
	return rows
}

func historySummaryRows(db *gorm.DB, jobs []models.QueueJob) ([]validationSummaryRow, error) {
	rows := validationSummaryRows(jobs)
	if len(jobs) == 0 {
		return rows, nil
	}
	jobExecutions := make([]string, 0, len(jobs))
	queryArgs := make([]any, 0, len(jobs)*2)
	executionByJob := make(map[uint]uint, len(jobs))
	for _, job := range jobs {
		if job.ExecutionNumber != nil {
			executionByJob[job.ID] = *job.ExecutionNumber
			jobExecutions = append(jobExecutions, "(queue_job_id = ? AND execution_number = ?)")
			queryArgs = append(queryArgs, job.ID, *job.ExecutionNumber)
		}
	}
	if len(jobExecutions) == 0 {
		return rows, nil
	}
	var publications []models.PublisherPublication
	if err := db.Select("id, publication_id, queue_job_id, execution_number, destination_path, publish_mode, validation_run_id, validation_override_event_ids, state, attempt_count, original_archive_status, original_archived_path").Where(strings.Join(jobExecutions, " OR "), queryArgs...).Find(&publications).Error; err != nil {
		return nil, err
	}
	if len(publications) == 0 {
		return rows, nil
	}
	publicationByJob := make(map[uint]models.PublisherPublication, len(publications))
	publicationIDs := make([]string, 0, len(publications))
	for _, publication := range publications {
		if execution, ok := executionByJob[publication.QueueJobID]; !ok || execution != publication.ExecutionNumber {
			continue
		}
		publicationByJob[publication.QueueJobID] = publication
		publicationIDs = append(publicationIDs, publication.PublicationID)
	}
	latestByPublication := make(map[string]models.PublisherAttemptEvidence, len(publicationIDs))
	if len(publicationIDs) > 0 {
		var evidence []models.PublisherAttemptEvidence
		if err := db.Raw("SELECT id, publication_id, phase, outcome, code, summary, created_at FROM publisher_attempt_evidences WHERE id IN (SELECT MAX(id) FROM publisher_attempt_evidences WHERE publication_id IN ? GROUP BY publication_id)", publicationIDs).Scan(&evidence).Error; err != nil {
			return nil, err
		}
		for _, item := range evidence {
			latestByPublication[item.PublicationID] = item
		}
	}
	for index, job := range jobs {
		publication, ok := publicationByJob[job.ID]
		if !ok {
			continue
		}
		overrideIDs := historyStringIDs(publication.ValidationOverrideEventIDs)
		projection := &historyPublisherSummary{
			PublicationID:              publication.PublicationID,
			State:                      publication.State,
			DestinationPath:            publication.DestinationPath,
			PublishMode:                publication.PublishMode,
			AttemptCount:               publication.AttemptCount,
			ValidationRunID:            publication.ValidationRunID,
			ValidationOverridePresent:  len(overrideIDs) > 0,
			ValidationOverrideEventIDs: overrideIDs,
		}
		if latest, found := latestByPublication[publication.PublicationID]; found {
			projection.LatestAttempt = &historyPublisherAttemptSummary{Phase: latest.Phase, Outcome: latest.Outcome, Code: latest.Code, Summary: latest.Summary, CreatedAt: latest.CreatedAt}
		}
		rows[index].Publisher = projection
		// Archive remains an independent #174 fact. For canonical publications,
		// read it from Publisher's persisted archive projection, not QueueJob.
		rows[index].OriginalArchiveStatus = publication.OriginalArchiveStatus
		rows[index].OriginalArchivedPath = publication.OriginalArchivedPath
	}
	return rows, nil
}

func historyStringIDs(values models.JSONList) []string {
	ids := make([]string, 0, len(values))
	for _, value := range values {
		if id, ok := value.(string); ok && strings.TrimSpace(id) != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func boundedValidationQueryInt(value string, fallback, minimum, maximum int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	if parsed < minimum {
		return minimum
	}
	if parsed > maximum {
		return maximum
	}
	return parsed
}

func (h ValidationHandler) CreateOverride(c *gin.Context) {
	var job models.QueueJob
	if err := h.db.First(&job, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	var request validationOverrideRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "override request must include a reason and rule scope"})
		return
	}
	request.Reason = strings.TrimSpace(request.Reason)
	if request.Reason == "" || len(request.Reason) > 2000 || len(request.RuleKeys) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "override requires a reason and at least one rule key"})
		return
	}
	requestedHandoff := strings.TrimSpace(request.RequestedHandoff)
	if requestedHandoff != "publisher" && requestedHandoff != "automatic" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "requestedHandoff must be publisher or automatic"})
		return
	}
	var run models.ValidationRun
	if err := h.db.Where("run_id = ? AND job_id = ?", job.ValidationRunID, job.ID).First(&run).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "job has no canonical ValidationRun"})
		return
	}
	var artifact map[string]any
	artifact, _, err := readLatestJobArtifact(h.db, job, "result")
	identity, identityOK := resultIdentityFromArtifact(artifact)
	currentRevision, revisionErr := resultOutputRevisionForPath(job.OutputPath)
	if err != nil || !runMatchesResult(run, artifact) || !identityOK || revisionErr != nil || currentRevision != identity.OutputRevision {
		c.JSON(http.StatusConflict, gin.H{"error": "Result identity changed; requeue execution before recording an override"})
		return
	}
	allowed := map[string]validationFinding{}
	var findings []validationFinding
	for _, raw := range run.Findings {
		encoded, _ := json.Marshal(raw)
		var finding validationFinding
		if json.Unmarshal(encoded, &finding) == nil {
			findings = append(findings, finding)
			allowed[finding.Key] = finding
		}
	}
	seen := map[string]bool{}
	keys := make([]string, 0, len(request.RuleKeys))
	for _, key := range request.RuleKeys {
		key = strings.TrimSpace(key)
		finding, ok := allowed[key]
		if !ok || !finding.Overrideable || finding.State == "PASS" || finding.State == "NOT_APPLICABLE" || nonOverrideableValidationRule(finding.Key) {
			c.JSON(http.StatusConflict, gin.H{"error": "override scope contains a non-overrideable or unknown rule"})
			return
		}
		if !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "override scope is empty"})
		return
	}
	sort.Strings(keys)
	var eventToken [16]byte
	if _, err := rand.Read(eventToken[:]); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	event := models.ValidationOverride{
		EventID: "validation-override:" + hex.EncodeToString(eventToken[:]), ValidationRunID: run.RunID,
		ActorSource: "deployment", ActorID: "local_operator", Reason: request.Reason,
		RuleKeys: models.JSONList{}, RequestedHandoff: requestedHandoff,
		OverrideVersion: validationOverrideV1, CreatedAt: time.Now().UTC(),
	}
	encodedKeys, _ := json.Marshal(keys)
	if err := json.Unmarshal(encodedKeys, &event.RuleKeys); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		return tx.Model(&models.QueueJob{}).Where("id = ?", job.ID).Update("validation_override", true).Error
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, event)
}

func nonOverrideableValidationRule(key string) bool {
	switch key {
	case "artifact.identity", "artifact.integrity", "artifact.manifest", "execution.binding":
		return true
	default:
		return false
	}
}

func runMatchesResult(run models.ValidationRun, artifact map[string]any) bool {
	identity, identityOK := resultIdentityFromArtifact(artifact)
	binding, bindingOK := resultBindingFromArtifact(artifact)
	return identityOK && bindingOK && run.ResultArtifactID == identity.ArtifactID && run.ResultGenerationToken == identity.GenerationToken && run.ResultGenerationVersion == identity.GenerationVersion && run.ResultOutputRevision == identity.OutputRevision && run.ExecutionNumber == binding.ExecutionNumber && run.ExecutionPlanID == binding.ExecutionPlanID && run.ExecutionPlanFingerprint == binding.ExecutionPlanFingerprint
}

func canonicalRunAllowsPublish(db *gorm.DB, job models.QueueJob, automatic bool) (bool, string) {
	var run models.ValidationRun
	if strings.TrimSpace(job.ValidationRunID) == "" || db.Where("run_id = ? AND job_id = ?", job.ValidationRunID, job.ID).First(&run).Error != nil {
		return false, "job has no canonical ValidationRun; historical outcomes are unbound"
	}
	artifact, _, err := readLatestJobArtifact(db, job, "result")
	if err != nil || !runMatchesResult(run, artifact) {
		return false, "current Result artifact does not match the ValidationRun identity"
	}
	identity, ok := resultIdentityFromArtifact(artifact)
	if !ok {
		return false, "current Result identity is missing"
	}
	revision, err := resultOutputRevisionForPath(job.OutputPath)
	if err != nil || revision != identity.OutputRevision {
		return false, "output has changed since Validation; queue a new execution"
	}
	if run.Outcome == "PASS" {
		if automatic && !automaticValidationOutcomeAllowed(db, run) {
			return false, "PASS is not enabled for automatic advancement"
		}
		return true, ""
	}
	if run.Outcome == "PASS_WITH_WARNINGS" && !run.ActionableWarnings {
		if !automatic || automaticValidationOutcomeAllowed(db, run) {
			return true, ""
		}
	}
	var overrides []models.ValidationOverride
	if db.Where("validation_run_id = ?", run.RunID).Find(&overrides).Error != nil {
		return false, "override audit events could not be read"
	}
	covered := map[string]bool{}
	coveredAutomatic := map[string]bool{}
	for _, event := range overrides {
		for _, raw := range event.RuleKeys {
			key := fmt.Sprint(raw)
			if event.RequestedHandoff == "publisher" || event.RequestedHandoff == "automatic" {
				covered[key] = true
			}
			if event.RequestedHandoff == "automatic" {
				coveredAutomatic[key] = true
			}
		}
	}
	var findings []validationFinding
	for _, raw := range run.Findings {
		encoded, _ := json.Marshal(raw)
		var finding validationFinding
		if json.Unmarshal(encoded, &finding) == nil {
			findings = append(findings, finding)
		}
	}
	for _, finding := range findings {
		if finding.Disposition == "blocking" && (finding.State == "FAIL" || finding.State == "REVIEW_REQUIRED" || finding.State == "UNVERIFIED") {
			if nonOverrideableValidationRule(finding.Key) || !finding.Overrideable || (automatic && !coveredAutomatic[finding.Key]) || (!automatic && !covered[finding.Key]) {
				return false, "blocking Validation finding has no eligible override: " + finding.Key
			}
		}
	}
	if automatic {
		policyAllows := automaticValidationOutcomeAllowed(db, run)
		for _, finding := range findings {
			if finding.Actionable && finding.State != "PASS" && !coveredAutomatic[finding.Key] {
				return false, "actionable Validation finding blocks automatic advancement: " + finding.Key
			}
		}
		if !policyAllows && !hasActionableFinding(findings) {
			return false, "Validation outcome is not enabled for automatic advancement"
		}
	}
	return true, ""
}

// canonicalEligibleOverrideEventIDs projects the exact #53 append-only events
// that cover findings for this handoff. Eligibility remains decided solely by
// canonicalRunAllowsPublish; this helper records the identities it relied on.
func canonicalEligibleOverrideEventIDs(db *gorm.DB, job models.QueueJob, automatic bool) ([]string, error) {
	allowed, _ := canonicalRunAllowsPublish(db, job, automatic)
	if !allowed {
		return nil, nil
	}
	var run models.ValidationRun
	if err := db.Where("run_id = ? AND job_id = ?", job.ValidationRunID, job.ID).First(&run).Error; err != nil {
		return nil, err
	}
	needed := map[string]bool{}
	for _, raw := range run.Findings {
		encoded, _ := json.Marshal(raw)
		var finding validationFinding
		if json.Unmarshal(encoded, &finding) != nil {
			continue
		}
		blockingOverride := finding.Disposition == "blocking" && (finding.State == "FAIL" || finding.State == "REVIEW_REQUIRED" || finding.State == "UNVERIFIED") && finding.Overrideable && !nonOverrideableValidationRule(finding.Key)
		automaticActionableOverride := automatic && finding.Actionable && finding.State != "PASS"
		if blockingOverride || automaticActionableOverride {
			needed[finding.Key] = true
		}
	}
	if len(needed) == 0 {
		return []string{}, nil
	}
	var overrides []models.ValidationOverride
	if err := db.Where("validation_run_id = ?", run.RunID).Order("created_at asc, id asc").Find(&overrides).Error; err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	ids := make([]string, 0, len(needed))
	for _, event := range overrides {
		if automatic && event.RequestedHandoff != "automatic" || !automatic && event.RequestedHandoff != "publisher" && event.RequestedHandoff != "automatic" {
			continue
		}
		for _, raw := range event.RuleKeys {
			if !needed[fmt.Sprint(raw)] || seen[event.EventID] {
				continue
			}
			seen[event.EventID] = true
			ids = append(ids, event.EventID)
			if len(ids) > 20 {
				return nil, fmt.Errorf("canonical Validation authorization requires more than 20 override evidence references")
			}
			break
		}
	}
	return ids, nil
}
