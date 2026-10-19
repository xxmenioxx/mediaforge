package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func TestHistoricalTrackReconciliationPreviewKeepsThreePatternsNeedsEvidence(t *testing.T) {
	tests := []struct {
		name       string
		findings   []any
		manifest   []any
		wantRule   string
		wantMissed string
	}{
		{
			name:     "audio action mismatch",
			findings: []any{historicalFinding("audio.tracks_actions", "FAIL", 2, 1)},
			manifest: []any{}, wantRule: "audio.tracks_actions",
		},
		{
			name:     "subtitle action mismatch",
			findings: []any{historicalFinding("subtitles.streams_actions", "FAIL", 1, 0)},
			manifest: []any{}, wantRule: "subtitles.streams_actions",
		},
		{
			name: "missing sidecar and manifest mismatch",
			findings: []any{
				historicalFinding("artifact.manifest", "FAIL", []string{"subtitle:2:extract:srt"}, []string{"subtitle:3:extract:srt"}),
				historicalFinding("sidecars.package", "FAIL", []string{"subtitle:2:extract:srt"}, []string{"subtitle:3:extract:srt"}),
				historicalFinding("video.properties", "UNVERIFIED", "frozen video properties", nil),
			},
			manifest: []any{map[string]any{"artifactId": "subtitle:3:extract:srt"}},
			wantRule: "artifact.manifest", wantMissed: "subtitle:2:extract:srt",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			job, artifact, run := historicalPreviewFixture(test.findings, test.manifest)
			got := assessHistoricalTrackReconciliation(job, artifact, &run, true)
			if got.Status != "NEEDS_EVIDENCE" {
				t.Fatalf("status=%q reasons=%v", got.Status, got.Reasons)
			}
			if got.IndependentIntent.Status != "UNAVAILABLE" {
				t.Fatalf("independent intent was manufactured: %#v", got.IndependentIntent)
			}
			if got.Identity.QueueJobID != job.ID || got.Identity.ExecutionNumber != 1 || got.Identity.ExecutionPlanID != 11 || got.Identity.ResultArtifactID != "result:1" || got.Identity.ResultOutputRevision != "revision:1" || !got.Identity.ValidationRunBound {
				t.Fatalf("Queue/ExecutionPlan/Result/Validation identity was not fully bound: %#v", got.Identity)
			}
			if got.Identity.QueueAssetBindingStatus != "CROSSED" {
				t.Fatalf("crossed per-asset snapshot was not reported: %#v", got.Identity)
			}
			diff, ok := historicalRuleByKey(got.Rules, test.wantRule)
			if !ok || diff.ExpectationAuthority != "Potentially contaminated Queue snapshot; not independent original intent" || !diff.CurrentResultRevision {
				t.Fatalf("expected per-rule diff did not preserve its source/revision: %#v", diff)
			}
			if test.wantMissed != "" {
				if len(got.Manifest.MissingCandidateArtifactIDs) != 1 || got.Manifest.MissingCandidateArtifactIDs[0] != test.wantMissed {
					t.Fatalf("missing candidate sidecar was not reported: %#v", got.Manifest)
				}
				if got.Manifest.SidecarContentRevisionStatus != "NOT_MODELED" {
					t.Fatalf("preview invented sidecar content revision evidence: %#v", got.Manifest)
				}
				if len(got.Manifest.UnexpectedActualArtifactIDs) != 1 || got.Manifest.UnexpectedActualArtifactIDs[0] != "subtitle:3:extract:srt" {
					t.Fatalf("replacement sidecar identity was not reported: %#v", got.Manifest)
				}
				video, ok := historicalRuleByKey(got.Rules, "video.properties")
				if !ok || video.State != "UNVERIFIED" {
					t.Fatalf("unrelated video finding was hidden or changed: %#v", video)
				}
			}
			if len(got.EvidenceRequired) == 0 {
				t.Fatal("preview did not specify the independent evidence still needed")
			}
		})
	}
}

func TestHistoricalTrackReconciliationPreviewBlocksStaleRevisions(t *testing.T) {
	job, artifact, run := historicalPreviewFixture([]any{historicalFinding("audio.tracks_actions", "FAIL", 2, 1)}, []any{})
	identity := artifact["resultIdentity"].(map[string]any)
	identity["outputRevision"] = "revision:replaced"
	got := assessHistoricalTrackReconciliation(job, artifact, &run, true)
	if got.Status != "BLOCKED" || got.Identity.ValidationRunBound {
		t.Fatalf("stale Result revision was not blocked: %#v", got)
	}
	if got.Identity.ResultOutputRevision != "revision:replaced" || got.Identity.ValidationRunOutputRevision != "revision:1" {
		t.Fatalf("stale revision pair was not surfaced: %#v", got.Identity)
	}
	if got.Identity.OutputFileRevisionCheck != "NOT_RECOMPUTED" {
		t.Fatalf("preview claimed an unperformed live output revision check: %#v", got.Identity)
	}
	if diff, ok := historicalRuleByKey(got.Rules, "audio.tracks_actions"); !ok || diff.CurrentResultRevision {
		t.Fatalf("stale ValidationRun finding was presented as current: %#v", diff)
	}
	if !strings.Contains(strings.Join(got.Reasons, " "), "stale") {
		t.Fatalf("stale revision reason is not actionable: %v", got.Reasons)
	}
}

func TestHistoricalTrackReconciliationPreviewRequiresExactPlanAndQueueIdentity(t *testing.T) {
	job, artifact, run := historicalPreviewFixture([]any{historicalFinding("subtitles.streams_actions", "FAIL", 1, 0)}, []any{})
	run.ExecutionPlanFingerprint = "different-plan"
	if got := assessHistoricalTrackReconciliation(job, artifact, &run, false); got.Status != "BLOCKED" {
		t.Fatalf("mismatched frozen plan/queue binding was not blocked: %#v", got)
	}
}

func TestHistoricalTrackReconciliationPreviewBlocksUnaffectedAndPublishedJobs(t *testing.T) {
	passFindings := []any{
		historicalFinding("audio.tracks_actions", "PASS", 1, 1),
		historicalFinding("subtitles.streams_actions", "NOT_APPLICABLE", 0, 0),
		historicalFinding("artifact.manifest", "PASS", []string{}, []string{}),
	}
	job, artifact, run := historicalPreviewFixture(passFindings, []any{})
	job.TrackProfileSnapshot["resolvedForAsset"] = job.MediaPath
	job.TrackProfileSnapshot["resolvedTrackPlan"] = map[string]any{"fontAttachmentExportPolicy": "none"}
	if got := assessHistoricalTrackReconciliation(job, artifact, &run, true); got.Status != "BLOCKED" {
		t.Fatalf("unaffected job incorrectly became a reconciliation candidate: %#v", got)
	}
	if got := assessHistoricalTrackReconciliation(job, artifact, &run, true); got.Identity.QueueAssetBindingStatus != "MATCH" {
		t.Fatalf("matching Queue asset identity was not reported: %#v", got.Identity)
	}
	now := time.Now()
	job.PublishedAt = &now
	if got := assessHistoricalTrackReconciliation(job, artifact, &run, true); got.Status != "BLOCKED" {
		t.Fatalf("published job incorrectly became a reconciliation candidate: %#v", got)
	}
}

func historicalPreviewFixture(findings []any, actualSidecars []any) (models.QueueJob, map[string]any, models.ValidationRun) {
	job := models.QueueJob{
		ID: 1, Status: JobStatusCompleted, MediaPath: "/fixtures/one.mkv", OutputPath: "/stage/output.mkv",
		ExecutionNumber: historicalUintPointer(1), ActiveExecutionPlanID: historicalUintPointer(11), ValidationRunID: "run:1",
		TrackProfileSnapshot: models.JSONMap{
			"resolvedForAsset": "/fixtures/two.mkv",
			"resolvedTrackPlan": map[string]any{
				"fontAttachmentExportPolicy": "none",
				"subtitleStreams":            []any{map[string]any{"streamIndex": 2, "action": "extract"}},
				"sidecarOutputs":             []any{map[string]any{"streamIndex": 2, "format": "srt", "mode": "extract"}},
			},
		},
	}
	artifact := map[string]any{
		"job": map[string]any{"executionNumber": float64(1)},
		"resultIdentity": map[string]any{
			"artifactId": "result:1", "generationToken": "token:1", "generationVersion": float64(1), "outputRevision": "revision:1",
		},
		"executionBinding": map[string]any{
			"executionNumber": float64(1), "executionPlanId": float64(11), "executionPlanVersion": float64(1),
			"executionPlanFingerprint": "plan:1", "frozenQueueFingerprint": "queue:1",
		},
		"outputProbe": map[string]any{"streams": []any{
			map[string]any{"codec_type": "audio"}, map[string]any{"codec_type": "subtitle"},
		}},
		"artifacts": map[string]any{
			"primary":  map[string]any{"type": "primary_media", "path": "/stage/output.mkv", "status": "ready"},
			"sidecars": actualSidecars,
		},
	}
	run := models.ValidationRun{
		RunID: "run:1", JobID: 1, ExecutionNumber: 1, ExecutionPlanID: 11, ExecutionPlanVersion: 1,
		ExecutionPlanFingerprint: "plan:1", ResultArtifactID: "result:1", ResultGenerationToken: "token:1",
		ResultGenerationVersion: 1, ResultOutputRevision: "revision:1", Outcome: "FAIL", Findings: models.JSONList(findings),
	}
	return job, artifact, run
}

func historicalFinding(key, state string, expected, actual any) map[string]any {
	return map[string]any{
		"key": key, "state": state, "expected": expected, "actual": actual,
		"explanation": "Synthetic historical finding", "evidenceReferences": []any{"queue.trackSnapshot", "result.outputProbe"},
	}
}

func historicalRuleByKey(diffs []HistoricalRuleDiff, key string) (HistoricalRuleDiff, bool) {
	for _, diff := range diffs {
		if diff.Key == key {
			return diff, true
		}
	}
	return HistoricalRuleDiff{}, false
}

func historicalUintPointer(value uint) *uint {
	return &value
}
