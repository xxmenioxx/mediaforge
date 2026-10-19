package handlers

import (
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
)

// HistoricalTrackReconciliationPreview is diagnostic only. It cannot grant a
// recovered expected-track contract, a ValidationRun or Publisher eligibility.
type HistoricalTrackReconciliationPreview struct {
	JobID             uint                                `json:"jobId"`
	Status            string                              `json:"status"`
	Reasons           []string                            `json:"reasons"`
	EvidenceRequired  []string                            `json:"evidenceRequired"`
	Identity          HistoricalReconciliationIdentity    `json:"identity"`
	IndependentIntent HistoricalIndependentIntentEvidence `json:"independentIntent"`
	Observed          HistoricalObservedTrackCounts       `json:"observed"`
	SnapshotCandidate HistoricalSnapshotCandidateCounts   `json:"snapshotCandidate"`
	Rules             []HistoricalRuleDiff                `json:"rules"`
	Manifest          HistoricalManifestDiff              `json:"manifest"`
}

type HistoricalReconciliationIdentity struct {
	QueueJobID                  uint   `json:"queueJobId"`
	SourceAssetRecordID         *uint  `json:"sourceAssetRecordId,omitempty"`
	QueueAssetBindingStatus     string `json:"queueAssetBindingStatus"`
	ExecutionNumber             uint   `json:"executionNumber"`
	FrozenQueueFingerprint      string `json:"frozenQueueFingerprint,omitempty"`
	ExecutionPlanID             uint   `json:"executionPlanId"`
	ExecutionPlanVersion        int    `json:"executionPlanVersion"`
	ExecutionPlanFingerprint    string `json:"executionPlanFingerprint,omitempty"`
	ResultArtifactID            string `json:"resultArtifactId,omitempty"`
	ResultGenerationToken       string `json:"resultGenerationToken,omitempty"`
	ResultGenerationVersion     int    `json:"resultGenerationVersion,omitempty"`
	ResultOutputRevision        string `json:"resultOutputRevision,omitempty"`
	OutputFileRevisionCheck     string `json:"outputFileRevisionCheck"`
	ValidationRunID             string `json:"validationRunId,omitempty"`
	ValidationRunOutputRevision string `json:"validationRunOutputRevision,omitempty"`
	ValidationRunBound          bool   `json:"validationRunBound"`
}

type HistoricalIndependentIntentEvidence struct {
	Status  string                           `json:"status"`
	Sources []HistoricalIntentEvidenceSource `json:"sources"`
}

type HistoricalIntentEvidenceSource struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type HistoricalObservedTrackCounts struct {
	Audio     int  `json:"audio"`
	Subtitles int  `json:"subtitles"`
	Available bool `json:"available"`
}

type HistoricalSnapshotCandidateCounts struct {
	Audio     int  `json:"audio"`
	Subtitles int  `json:"subtitles"`
	Sidecars  int  `json:"sidecars"`
	Available bool `json:"available"`
}

type HistoricalRuleDiff struct {
	Key                   string   `json:"key"`
	State                 string   `json:"state"`
	Expected              any      `json:"expected"`
	Actual                any      `json:"actual"`
	Explanation           string   `json:"explanation,omitempty"`
	EvidenceReferences    []string `json:"evidenceReferences,omitempty"`
	ExpectationAuthority  string   `json:"expectationAuthority"`
	CurrentResultRevision bool     `json:"currentResultRevision"`
}

type HistoricalManifestDiff struct {
	Available                    bool     `json:"available"`
	CandidateExpectedArtifactIDs []string `json:"candidateExpectedArtifactIds"`
	ActualArtifactIDs            []string `json:"actualArtifactIds"`
	MissingCandidateArtifactIDs  []string `json:"missingCandidateArtifactIds"`
	UnexpectedActualArtifactIDs  []string `json:"unexpectedActualArtifactIds"`
	ExpectationAuthority         string   `json:"expectationAuthority"`
	SidecarContentRevisionStatus string   `json:"sidecarContentRevisionStatus"`
}

// assessHistoricalTrackReconciliation never treats worker output or the
// crossed frozen track plan as independent evidence of the original operator
// intent. Queue/plan/result bindings and the active ValidationRun must agree.
func assessHistoricalTrackReconciliation(job models.QueueJob, artifact map[string]any, run *models.ValidationRun, bound bool) HistoricalTrackReconciliationPreview {
	preview := HistoricalTrackReconciliationPreview{
		JobID:  job.ID,
		Status: "BLOCKED",
		EvidenceRequired: []string{
			"An immutable per-asset audio/subtitle selection and action record captured before Queue snapshot construction, bound to the source asset identity and profile revision.",
			"For sidecars, the immutable intended artifact IDs, requiredness, format and output contract for this asset and execution.",
		},
		IndependentIntent: HistoricalIndependentIntentEvidence{
			Status: "UNAVAILABLE",
			Sources: []HistoricalIntentEvidenceSource{
				{Name: "QueueJob.TrackProfileSnapshot", Status: "AVAILABLE_NOT_INDEPENDENT", Reason: "This snapshot is the potentially crossed input whose original per-asset intent is under review."},
				{Name: "ExecutionPlan", Status: "NO_TRACK_CONTRACT", Reason: "The modeled plan binds video/execution decisions but has no independent per-asset track action contract."},
				{Name: "ResultArtifact", Status: "OBSERVED_OUTPUT_ONLY", Reason: "Result stream counts and manifest describe produced output; they do not prove what the operator intended."},
				{Name: "ValidationRun", Status: "DERIVED_FROM_QUEUE_SNAPSHOT", Reason: "Historical expected values were evaluated from the Queue snapshot and are retained for comparison, not as corrected authority."},
			},
		},
		Rules:    []HistoricalRuleDiff{},
		Manifest: HistoricalManifestDiff{ExpectationAuthority: "Queue snapshot candidate; not independent intent", SidecarContentRevisionStatus: "NOT_MODELED"},
	}
	if job.Status != JobStatusCompleted || job.PublishedAt != nil {
		preview.Reasons = append(preview.Reasons, "Only completed, unpublished jobs may be inspected for historical reconciliation")
		return preview
	}
	identity, identityOK := resultIdentityFromArtifact(artifact)
	binding, bindingOK := resultBindingFromArtifact(artifact)
	if identityOK {
		preview.Identity.ResultArtifactID = identity.ArtifactID
		preview.Identity.ResultGenerationToken = identity.GenerationToken
		preview.Identity.ResultGenerationVersion = identity.GenerationVersion
		preview.Identity.ResultOutputRevision = identity.OutputRevision
	}
	if bindingOK {
		preview.Identity.ExecutionNumber = binding.ExecutionNumber
		preview.Identity.FrozenQueueFingerprint = binding.FrozenQueueFingerprint
		preview.Identity.ExecutionPlanID = binding.ExecutionPlanID
		preview.Identity.ExecutionPlanVersion = binding.ExecutionPlanVersion
		preview.Identity.ExecutionPlanFingerprint = binding.ExecutionPlanFingerprint
	}
	preview.Identity.OutputFileRevisionCheck = "NOT_RECOMPUTED"
	preview.Identity.QueueJobID = job.ID
	preview.Identity.SourceAssetRecordID = job.SourceAssetRecordID
	preview.Identity.ValidationRunID = job.ValidationRunID
	if !identityOK || identity.GenerationVersion < 1 || !bindingOK || binding.ExecutionNumber == 0 || binding.ExecutionPlanVersion < 1 || !bound || artifactExecutionNumber(artifact) != binding.ExecutionNumber {
		preview.Reasons = append(preview.Reasons, "Canonical Result generation, output revision or frozen Queue/ExecutionPlan binding is unavailable or inconsistent")
		return preview
	}
	if run != nil {
		preview.Identity.ValidationRunOutputRevision = run.ResultOutputRevision
		preview.Identity.ValidationRunBound = historicalValidationRunMatches(job, identity, binding, *run)
		preview.Rules = historicalRuleDiffs(run, identity.OutputRevision)
	}
	if run == nil || job.ValidationRunID == "" {
		preview.Reasons = append(preview.Reasons, "The active immutable ValidationRun is unavailable; per-rule historical expectations cannot be bound to this Result")
		return preview
	}
	if !preview.Identity.ValidationRunBound {
		preview.Reasons = append(preview.Reasons, "The active ValidationRun belongs to a different Queue execution, ExecutionPlan or Result revision; preview is blocked as stale")
		return preview
	}

	outputProbe := unknownRecord(artifact["outputProbe"])
	if outputProbe != nil {
		preview.Observed.Available = true
		preview.Observed.Audio = countProbeStreams(outputProbe, "audio")
		preview.Observed.Subtitles = countProbeStreams(outputProbe, "subtitle")
	}
	resolved := strings.TrimSpace(workerStringValue(job.TrackProfileSnapshot["resolvedForAsset"]))
	switch {
	case resolved == "":
		preview.Identity.QueueAssetBindingStatus = "MISSING"
	case filepath.Clean(resolved) == filepath.Clean(job.MediaPath):
		preview.Identity.QueueAssetBindingStatus = "MATCH"
	default:
		preview.Identity.QueueAssetBindingStatus = "CROSSED"
	}
	if plan, ok := ResolvedTrackPlanFromSnapshot(job.TrackProfileSnapshot); ok {
		preview.SnapshotCandidate.Available = true
		preview.SnapshotCandidate.Audio = len(plan.AudioStreams)
		for _, stream := range plan.SubtitleStreams {
			if stream.Action == SubtitleDispositionKeep || stream.Action == SubtitleDispositionKeepAndExtract {
				preview.SnapshotCandidate.Subtitles++
			}
		}
		preview.SnapshotCandidate.Sidecars = len(plan.SidecarOutputs)
	}

	_, expectedManifest, actualManifest, manifestMatched := resultManifestEvidence(job, artifact)
	preview.Manifest.Available = unknownRecord(artifact["artifacts"]) != nil
	preview.Manifest.CandidateExpectedArtifactIDs = historicalStringList(expectedManifest["requiredArtifactIds"])
	preview.Manifest.ActualArtifactIDs = historicalStringList(actualManifest["artifactIds"])
	preview.Manifest.MissingCandidateArtifactIDs, preview.Manifest.UnexpectedActualArtifactIDs = historicalManifestDifference(
		preview.Manifest.CandidateExpectedArtifactIDs, preview.Manifest.ActualArtifactIDs,
	)
	if len(preview.Manifest.UnexpectedActualArtifactIDs) > 0 {
		preview.Reasons = append(preview.Reasons, "The Result manifest contains sidecar/attachment IDs absent from the frozen Queue candidate contract; their intended disposition is not independently proven")
	}
	if !preview.Manifest.Available || !manifestMatched {
		if len(preview.Manifest.MissingCandidateArtifactIDs) > 0 {
			preview.Reasons = append(preview.Reasons, "The Result manifest omits candidate sidecar/attachment IDs from the frozen Queue snapshot; this is an observed package mismatch, not independent proof that those artifacts were originally required")
		} else {
			preview.Reasons = append(preview.Reasons, "The Result manifest is missing or does not match the frozen package snapshot")
		}
	}

	crossedOrUnverifiable := preview.Identity.QueueAssetBindingStatus != "MATCH"
	trackFinding := historicalTrackFindingNeedsReview(preview.Rules)
	manifestDisagrees := !preview.Manifest.Available || !manifestMatched || len(preview.Manifest.UnexpectedActualArtifactIDs) > 0
	if crossedOrUnverifiable || trackFinding || manifestDisagrees {
		preview.Status = "NEEDS_EVIDENCE"
		preview.Reasons = append(preview.Reasons, "No modeled record independently proves original per-asset track and sidecar intent; output counts, manifests and current defaults cannot repair that gap")
		return preview
	}
	preview.Reasons = append(preview.Reasons, "No track/sidecar reconciliation candidate was found in the active ValidationRun")
	return preview
}

func historicalValidationRunMatches(job models.QueueJob, identity resultArtifactIdentity, binding resultExecutionBinding, run models.ValidationRun) bool {
	return run.RunID == job.ValidationRunID && run.JobID == job.ID &&
		run.ExecutionNumber == binding.ExecutionNumber &&
		run.ExecutionPlanID == binding.ExecutionPlanID && run.ExecutionPlanVersion == binding.ExecutionPlanVersion &&
		run.ExecutionPlanFingerprint == binding.ExecutionPlanFingerprint &&
		run.ResultArtifactID == identity.ArtifactID && run.ResultGenerationToken == identity.GenerationToken &&
		run.ResultGenerationVersion == identity.GenerationVersion && run.ResultOutputRevision == identity.OutputRevision
}

func historicalRuleDiffs(run *models.ValidationRun, currentOutputRevision string) []HistoricalRuleDiff {
	if run == nil {
		return []HistoricalRuleDiff{}
	}
	diffs := make([]HistoricalRuleDiff, 0, len(run.Findings))
	for _, raw := range run.Findings {
		finding := unknownRecord(raw)
		key := strings.TrimSpace(stringFromUnknown(finding["key"]))
		if key == "" {
			continue
		}
		authority := "Original ValidationRun finding; not independent original intent"
		if key == "audio.tracks_actions" || key == "subtitles.streams_actions" || key == "artifact.manifest" || key == "sidecars.package" || strings.HasPrefix(key, "attachments.") {
			authority = "Potentially contaminated Queue snapshot; not independent original intent"
		}
		references := historicalStringList(finding["evidenceReferences"])
		diffs = append(diffs, HistoricalRuleDiff{
			Key: key, State: stringFromUnknown(finding["state"]), Expected: finding["expected"], Actual: finding["actual"],
			Explanation: stringFromUnknown(finding["explanation"]), EvidenceReferences: references,
			ExpectationAuthority: authority, CurrentResultRevision: run.ResultOutputRevision == currentOutputRevision,
		})
		if len(diffs) == 64 {
			break
		}
	}
	sort.Slice(diffs, func(i, j int) bool { return diffs[i].Key < diffs[j].Key })
	return diffs
}

func historicalTrackFindingNeedsReview(diffs []HistoricalRuleDiff) bool {
	for _, diff := range diffs {
		if diff.Key != "audio.tracks_actions" && diff.Key != "subtitles.streams_actions" && diff.Key != "artifact.manifest" && diff.Key != "sidecars.package" && !strings.HasPrefix(diff.Key, "attachments.") {
			continue
		}
		if diff.State != "PASS" && diff.State != "NOT_APPLICABLE" {
			return true
		}
	}
	return false
}

func historicalStringList(value any) []string {
	var values []string
	switch list := value.(type) {
	case []string:
		values = append(values, list...)
	case []any:
		for _, item := range list {
			if text := strings.TrimSpace(stringFromUnknown(item)); text != "" {
				values = append(values, text)
			}
		}
	case models.JSONList:
		for _, item := range list {
			if text := strings.TrimSpace(stringFromUnknown(item)); text != "" {
				values = append(values, text)
			}
		}
	}
	sort.Strings(values)
	return values
}

func historicalManifestDifference(expected, actual []string) (missing, unexpected []string) {
	expectedSet := make(map[string]bool, len(expected))
	actualSet := make(map[string]bool, len(actual))
	for _, id := range expected {
		expectedSet[id] = true
	}
	for _, id := range actual {
		actualSet[id] = true
	}
	for _, id := range expected {
		if !actualSet[id] {
			missing = append(missing, id)
		}
	}
	for _, id := range actual {
		if !expectedSet[id] {
			unexpected = append(unexpected, id)
		}
	}
	return missing, unexpected
}

// PreviewHistoricalTrackReconciliation reads existing evidence only. It does
// not hash or probe output media, persist state or authorize a later
// commit/publish operation.
func (h ValidationHandler) PreviewHistoricalTrackReconciliation(c *gin.Context) {
	job, ok := h.job(c)
	if !ok {
		return
	}
	var run *models.ValidationRun
	if job.ValidationRunID != "" {
		var active models.ValidationRun
		if h.db.Where("run_id = ? AND job_id = ?", job.ValidationRunID, job.ID).First(&active).Error == nil {
			run = &active
		}
	}
	artifact, _, err := readLatestJobArtifact(h.db, job, "result")
	if err != nil {
		preview := assessHistoricalTrackReconciliation(job, nil, run, false)
		preview.Reasons = append(preview.Reasons, "Canonical Result artifact is unavailable; no historical repair can be inferred")
		c.JSON(http.StatusOK, preview)
		return
	}
	binding, bindingOK := resultBindingFromArtifact(artifact)
	bound := bindingOK && frozenBindingMatches(h.db, job, binding)
	c.JSON(http.StatusOK, assessHistoricalTrackReconciliation(job, artifact, run, bound))
}
