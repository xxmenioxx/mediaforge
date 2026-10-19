package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const historicalReconciliationPreviewVersion = "historical-validation-reconciliation-preview-v1"

type historicalReconciliationPreview struct {
	SchemaVersion     string                               `json:"schemaVersion"`
	Status            string                               `json:"status"`
	Reasons           []string                             `json:"reasons"`
	Binding           historicalReconciliationBinding      `json:"binding,omitempty"`
	ExpectedContract  historicalExpectedContractAssessment `json:"expectedContract"`
	AffectedFindings  []historicalReconciliationFinding    `json:"affectedFindings"`
	UnchangedFindings []historicalReconciliationFinding    `json:"unchangedFindings"`
}

type historicalReconciliationBinding struct {
	QueueJobID               uint   `json:"queueJobId"`
	ExecutionNumber          uint   `json:"executionNumber"`
	ExecutionPlanID          uint   `json:"executionPlanId"`
	ExecutionPlanVersion     int    `json:"executionPlanVersion"`
	ExecutionPlanFingerprint string `json:"executionPlanFingerprint"`
	ResultArtifactID         string `json:"resultArtifactId"`
	ResultGenerationToken    string `json:"resultGenerationToken"`
	ResultGenerationVersion  int    `json:"resultGenerationVersion"`
	ResultOutputRevision     string `json:"resultOutputRevision"`
	ValidationRunID          string `json:"validationRunId"`
}

type historicalExpectedContractAssessment struct {
	Status          string   `json:"status"`
	OriginalSource  string   `json:"originalSource"`
	ProposedSource  string   `json:"proposedSource,omitempty"`
	MissingEvidence []string `json:"missingEvidence"`
	Reason          string   `json:"reason"`
}

type historicalReconciliationFinding struct {
	Key                string   `json:"key"`
	State              string   `json:"state"`
	OriginalExpected   any      `json:"originalExpected"`
	ObservedActual     any      `json:"observedActual"`
	ProposedExpected   any      `json:"proposedExpected"`
	EvidenceReferences []string `json:"evidenceReferences"`
	Explanation        string   `json:"explanation"`
}

var reconciliationTrackFindingKeys = map[string]bool{
	"audio.tracks_actions":       true,
	"subtitles.streams_actions":  true,
	"chapters.actions":           true,
	"attachments.stream_actions": true,
	"attachments.font_artifacts": true,
	"artifact.manifest":          true,
}

func (h ValidationHandler) ReconciliationPreview(c *gin.Context) {
	job, ok := h.job(c)
	if !ok {
		return
	}
	preview, err := buildHistoricalReconciliationPreview(h.db, job)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "historical reconciliation preview could not read its local evidence"})
		return
	}
	c.JSON(http.StatusOK, preview)
}

func buildHistoricalReconciliationPreview(db *gorm.DB, job models.QueueJob) (historicalReconciliationPreview, error) {
	preview := historicalReconciliationPreview{
		SchemaVersion:     historicalReconciliationPreviewVersion,
		Status:            "BLOCKED",
		Reasons:           []string{},
		AffectedFindings:  []historicalReconciliationFinding{},
		UnchangedFindings: []historicalReconciliationFinding{},
		ExpectedContract: historicalExpectedContractAssessment{
			Status:          "NEEDS_EVIDENCE",
			OriginalSource:  "canonical ValidationRun finding; derived from the persisted Queue track snapshot",
			MissingEvidence: []string{"immutable execution-time per-asset track intent with asset identity, revision, and fingerprint"},
			Reason:          "The existing Queue track snapshot, ProfileResolution, ExecutionPlan, Result stream plan, and current defaults do not independently prove corrected per-asset track expectations for legacy executions.",
		},
	}
	if job.Status != JobStatusCompleted || job.PublishedAt != nil || (job.ValidationOutcome != "FAIL" && job.ValidationOutcome != "REVIEW_REQUIRED") || strings.TrimSpace(job.ValidationRunID) == "" {
		preview.Reasons = append(preview.Reasons, "Only completed, unpublished jobs with an active canonical FAIL or REVIEW_REQUIRED ValidationRun can be previewed for reconciliation.")
		return preview, nil
	}
	if job.ExecutionNumber == nil || job.ActiveExecutionPlanID == nil {
		preview.Reasons = append(preview.Reasons, "The Queue execution number or active frozen ExecutionPlan identity is missing.")
		return preview, nil
	}
	var run models.ValidationRun
	if err := db.Where("run_id = ? AND job_id = ?", job.ValidationRunID, job.ID).First(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			preview.Reasons = append(preview.Reasons, "The active Queue ValidationRun is missing; a historical or unbound status projection cannot authorize preview.")
			return preview, nil
		}
		return preview, err
	}
	if run.Outcome != job.ValidationOutcome || run.ExecutionNumber != *job.ExecutionNumber {
		preview.Reasons = append(preview.Reasons, "The active ValidationRun outcome or execution number does not match the Queue projection.")
		return preview, nil
	}
	artifact, _, err := readLatestJobArtifact(db, job, "result")
	if err != nil {
		preview.Reasons = append(preview.Reasons, "The canonical kind=result artifact or its report directory is missing or unreadable; verify that the original Result evidence is available.")
		return preview, nil
	}
	if stringFromUnknown(artifact["kind"]) != "result" {
		preview.Reasons = append(preview.Reasons, "The selected artifact is not a canonical kind=result artifact.")
		return preview, nil
	}
	identity, identityOK := resultIdentityFromArtifact(artifact)
	binding, bindingOK := resultBindingFromArtifact(artifact)
	if !identityOK || identity.GenerationVersion < 1 || !bindingOK || !frozenBindingMatches(db, job, binding) {
		preview.Reasons = append(preview.Reasons, "The Result generation or frozen Queue/ExecutionPlan binding is incomplete, stale, or mismatched.")
		return preview, nil
	}
	if identity.ArtifactID != fmt.Sprintf("job-result:%d:execution:%d", job.ID, binding.ExecutionNumber) || binding.ExecutionNumber != *job.ExecutionNumber {
		preview.Reasons = append(preview.Reasons, "The canonical Result artifact ID or execution number does not match this Queue execution.")
		return preview, nil
	}
	var plan models.ExecutionPlan
	if err := db.First(&plan, binding.ExecutionPlanID).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return preview, err
		}
		preview.Reasons = append(preview.Reasons, "The frozen ExecutionPlan referenced by the Result is unavailable.")
		return preview, nil
	}
	currentOutput, revisionErr := resultOutputRevisionForIdentity(job.OutputPath, identity.OutputRevision)
	if revisionErr != nil || currentOutput.ContentRevision != identity.OutputRevision {
		preview.Reasons = append(preview.Reasons, "The current output bytes do not match the Result's recorded outputRevision.")
		return preview, nil
	}
	if run.ExecutionPlanID != binding.ExecutionPlanID || run.ExecutionPlanVersion != binding.ExecutionPlanVersion || run.ExecutionPlanFingerprint != binding.ExecutionPlanFingerprint || run.ResultArtifactID != identity.ArtifactID || run.ResultGenerationToken != identity.GenerationToken || run.ResultGenerationVersion != identity.GenerationVersion || run.ResultOutputRevision != identity.OutputRevision {
		preview.Reasons = append(preview.Reasons, "The active ValidationRun is not bound to the exact Result generation, output revision, and ExecutionPlan recorded by the Result.")
		return preview, nil
	}
	artifactJob := unknownRecord(artifact["job"])
	if artifactJob == nil ||
		filepath.Clean(stringFromUnknown(artifactJob["outputPath"])) != filepath.Clean(job.OutputPath) ||
		filepath.Clean(plan.OutputPath) != filepath.Clean(job.OutputPath) ||
		uintFromUnknown(artifactJob["executionNumber"]) != binding.ExecutionNumber {
		preview.Reasons = append(preview.Reasons, "The Result artifact, Queue output path, ExecutionPlan output path, or execution number disagree.")
		return preview, nil
	}
	if !historicalReconciliationJSONValuesEqual(artifactJob["subtitleArtifacts"], job.SubtitleArtifacts) {
		preview.Reasons = append(preview.Reasons, "Queue sidecar identity evidence differs from the Queue snapshot captured in the canonical Result artifact.")
		return preview, nil
	}
	preview.Binding = historicalReconciliationBinding{
		QueueJobID: job.ID, ExecutionNumber: binding.ExecutionNumber,
		ExecutionPlanID: binding.ExecutionPlanID, ExecutionPlanVersion: binding.ExecutionPlanVersion,
		ExecutionPlanFingerprint: binding.ExecutionPlanFingerprint,
		ResultArtifactID:         identity.ArtifactID, ResultGenerationToken: identity.GenerationToken,
		ResultGenerationVersion: identity.GenerationVersion, ResultOutputRevision: identity.OutputRevision,
		ValidationRunID: run.RunID,
	}
	preview.ExpectedContract = resolveHistoricalExpectedContract(job, plan, artifact)
	preview.AffectedFindings, preview.UnchangedFindings = reconciliationPreviewFindings(run.Findings)
	if len(preview.AffectedFindings) == 0 {
		preview.Reasons = append(preview.Reasons, "The bound ValidationRun contains no track, chapter, attachment, or artifact manifest findings to assess.")
		return preview, nil
	}
	preview.Status = "NEEDS_EVIDENCE"
	preview.Reasons = append(preview.Reasons, "The Result and frozen execution identities match, but the modeled records do not contain an independent per-asset expected track contract. Output counts, Result stream plans, and current defaults are not accepted as substitutes.")
	return preview, nil
}

// resolveHistoricalExpectedContract is intentionally independent of Result
// output counts, current defaults, and the Queue-derived track decision report.
// Those values can describe a contaminated historical plan but cannot correct
// it. Existing models do not persist a separately versioned per-asset intent.
func resolveHistoricalExpectedContract(job models.QueueJob, _ models.ExecutionPlan, _ map[string]any) historicalExpectedContractAssessment {
	reason := "No immutable execution-time per-asset track intent is modeled independently from the frozen Queue and Result track plans."
	trackSnapshot := unknownRecord(job.TrackProfileSnapshot[resolvedTrackPlanSnapshotKey])
	if resolvedForAsset := strings.TrimSpace(stringFromUnknown(trackSnapshot["resolvedForAsset"])); resolvedForAsset != "" && filepath.Clean(resolvedForAsset) != filepath.Clean(job.MediaPath) {
		reason = "The persisted track snapshot resolves to a different asset path; this confirms a mismatch but does not reveal the correct per-asset actions."
	}
	return historicalExpectedContractAssessment{
		Status:         "NEEDS_EVIDENCE",
		OriginalSource: "canonical ValidationRun finding; derived from the persisted Queue track snapshot",
		MissingEvidence: []string{
			"immutable per-asset track actions captured at execution time",
			"source asset identity and track-profile revision bound to that intent",
			"fingerprint proving the intent belongs to this Queue execution",
		},
		Reason: reason + " Output-derived counts and current defaults cannot substitute for that authority.",
	}
}

func reconciliationPreviewFindings(findings models.JSONList) ([]historicalReconciliationFinding, []historicalReconciliationFinding) {
	affected := []historicalReconciliationFinding{}
	unchanged := []historicalReconciliationFinding{}
	for _, value := range findings {
		finding := unknownRecord(value)
		if finding == nil {
			continue
		}
		key := stringFromUnknown(finding["key"])
		if !reconciliationTrackFindingKeys[key] && key != "video.properties" {
			continue
		}
		state := strings.ToUpper(stringFromUnknown(finding["state"]))
		if state == "PASS" || state == "NOT_APPLICABLE" {
			continue
		}
		references := []string{}
		if values, ok := finding["evidenceReferences"].([]any); ok {
			for _, value := range values {
				if reference := strings.TrimSpace(stringFromUnknown(value)); reference != "" {
					references = append(references, reference)
				}
			}
		}
		item := historicalReconciliationFinding{
			Key: key, State: state,
			OriginalExpected: finding["expected"], ObservedActual: finding["actual"],
			ProposedExpected: nil, EvidenceReferences: references,
			Explanation: stringFromUnknown(finding["explanation"]),
		}
		if reconciliationTrackFindingKeys[key] {
			affected = append(affected, item)
		} else {
			unchanged = append(unchanged, item)
		}
	}
	return affected, unchanged
}

func historicalReconciliationJSONValuesEqual(left, right any) bool {
	leftBytes, leftErr := json.Marshal(left)
	rightBytes, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftBytes) == string(rightBytes)
}
