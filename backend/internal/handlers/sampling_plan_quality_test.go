package handlers

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func testQualitySamplingBasePlan() SamplingPlan {
	return canonicalSamplingPlan(600, FrameStructureSamplingPolicy{
		Windows:        4,
		InitialWindows: 2,
		WindowSeconds:  20,
		Positions:      []float64{0.1, 0.4, 0.7, 0.9},
	})
}

func testQualitySamplingInput() SamplingPlanInput {
	return SamplingPlanInput{
		BasePlan:             testQualitySamplingBasePlan(),
		SourceFingerprint:    "asset:size=1000:mtime=42",
		SourceRevision:       "revision-1",
		SourceStreamIdentity: "0:v:0",
		PolicyVersion:        "quality-hard-scene/v1",
		FrameDurationSeconds: 1.0 / 24.0,
		Primary: &SamplingWindow{
			StartSeconds:    302,
			DurationSeconds: 20,
			Position:        0.5,
		},
	}
}

func TestResolveCanonicalSamplingPlanIsByteStable(t *testing.T) {
	input := testQualitySamplingInput()
	input.Evidence = []SamplingEvidence{{
		Identity:     "interval-b",
		Source:       "frame-structure",
		Version:      "2",
		Scope:        "interval",
		Availability: "available",
		Confidence:   0.9,
		Signals:      map[string]float64{"complexity": 0.8, "worst_predicted": 0.7},
	}}
	input.Candidates = []SamplingCandidate{{
		Window:       SamplingWindow{StartSeconds: 120, DurationSeconds: 20},
		EvidenceRefs: []string{"interval-b"},
	}}

	first := resolveCanonicalSamplingPlan(input)
	second := resolveCanonicalSamplingPlan(input)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same input produced different plans:\n%#v\n%#v", first, second)
	}
	if first.PlanIdentity == "" || first.SelectedWindows[0].Identity == "" {
		t.Fatalf("resolved plan is missing stable identity: %#v", first)
	}
}

func TestResolveCanonicalSamplingPlanConsolidatesEquivalentWindowsAndSharesRoles(t *testing.T) {
	input := testQualitySamplingInput()
	input.Candidates = []SamplingCandidate{
		{
			Window:       SamplingWindow{StartSeconds: 100, DurationSeconds: 20},
			EvidenceRefs: []string{"interval-hard"},
		},
		{
			Window:       SamplingWindow{StartSeconds: 100.02, DurationSeconds: 20.01},
			EvidenceRefs: []string{"interval-hard"},
		},
	}
	input.Evidence = []SamplingEvidence{{
		Identity:     "interval-hard",
		Source:       "frame-structure",
		Version:      "2",
		Scope:        "interval",
		Availability: "available",
		Confidence:   0.8,
		Signals: map[string]float64{
			"motion":           0.9,
			"texture_or_grain": 0.9,
			"worst_predicted":  0.9,
		},
	}}

	plan := resolveCanonicalSamplingPlan(input)
	hard := 0
	for _, window := range plan.SelectedWindows {
		if window.StartSeconds == 100 {
			hard++
			if len(window.Roles) < 3 {
				t.Fatalf("equivalent candidates did not union roles: %#v", window)
			}
		}
	}
	if hard != 1 || len(plan.CandidateSummary.Consolidated) != 1 {
		t.Fatalf("expected one bounded consolidation, plan=%#v", plan)
	}
}

func TestResolveCanonicalSamplingPlanUsesDeterministicTieBreak(t *testing.T) {
	input := testQualitySamplingInput()
	input.Candidates = []SamplingCandidate{
		{Window: SamplingWindow{StartSeconds: 40, DurationSeconds: 20}, EvidenceRefs: []string{"e"}},
		{Window: SamplingWindow{StartSeconds: 20, DurationSeconds: 20}, EvidenceRefs: []string{"e"}},
	}
	input.Evidence = []SamplingEvidence{{
		Identity:     "e",
		Source:       "frame-structure",
		Scope:        "interval",
		Availability: "available",
		Confidence:   0.7,
		Signals:      map[string]float64{"complexity": 0.8},
	}}

	plan := resolveCanonicalSamplingPlan(input)
	if len(plan.SelectedWindows) < 2 || plan.SelectedWindows[1].StartSeconds != 20 {
		t.Fatalf("equal scores were not ordered by start: %#v", plan.SelectedWindows)
	}
}

func TestResolveCanonicalSamplingPlanFallsBackWithoutIntervalEvidence(t *testing.T) {
	input := testQualitySamplingInput()
	input.Evidence = []SamplingEvidence{{
		Identity:     "global-bitrate",
		Source:       "scan-metadata",
		Scope:        "global",
		Availability: "available",
		Confidence:   1,
		Signals:      map[string]float64{"complexity": 1},
		Weak:         true,
	}}
	input.Candidates = []SamplingCandidate{{
		Window:       SamplingWindow{StartSeconds: 100, DurationSeconds: 20},
		EvidenceRefs: []string{"global-bitrate"},
	}}

	plan := resolveCanonicalSamplingPlan(input)
	if !plan.Fallback.Used || plan.Fallback.Kind != samplingFallbackDistributed {
		t.Fatalf("expected distributed fallback: %#v", plan.Fallback)
	}
	for _, window := range plan.SelectedWindows {
		for _, role := range window.Roles {
			if role == SamplingRoleHighComplexity || role == SamplingRoleWorstPredicted {
				t.Fatalf("global evidence invented hard-scene role: %#v", window)
			}
		}
	}
}

func TestSamplingPlanRoleConsumerDoesNotReselect(t *testing.T) {
	input := testQualitySamplingInput()
	input.Candidates = []SamplingCandidate{{
		Window:       SamplingWindow{StartSeconds: 100, DurationSeconds: 20},
		EvidenceRefs: []string{"e"},
	}}
	input.Evidence = []SamplingEvidence{{
		Identity:     "e",
		Source:       "frame-structure",
		Scope:        "interval",
		Availability: "available",
		Confidence:   0.8,
		Signals:      map[string]float64{"motion": 0.9},
	}}
	plan := resolveCanonicalSamplingPlan(input)
	selection := SamplingPlanWindowsForRoles(plan, SamplingRoleHighTextureOrGrain)
	if len(selection.Windows) != 0 || len(selection.UnavailableRoles) != 1 {
		t.Fatalf("consumer unexpectedly selected a replacement window: %#v", selection)
	}
	if selection.PlanIdentity != plan.PlanIdentity {
		t.Fatalf("consumer lost frozen plan identity: %#v", selection)
	}
}

func TestSamplingPlanSerializationIsBoundedAndLegacyReadable(t *testing.T) {
	input := testQualitySamplingInput()
	input.Evidence = []SamplingEvidence{{
		Identity:     "e",
		Source:       "frame-structure",
		Scope:        "interval",
		Availability: "available",
		Signals:      map[string]float64{"complexity": 0.8},
	}}
	plan := resolveCanonicalSamplingPlan(input)
	persisted := samplingPlanMap(plan)
	encoded, err := json.Marshal(persisted)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) > 12000 {
		t.Fatalf("sampling provenance is not bounded: %d bytes", len(encoded))
	}
	if _, ok := persisted["frameArrays"]; ok {
		t.Fatal("persisted plan contains frame-level evidence")
	}
	legacy, ok := samplingPlanFromMap(map[string]any{
		"assetDurationSeconds": 600.0,
		"windowSeconds":        20.0,
		"positions":            []any{0.1, 0.5, 0.9},
	})
	if !ok || !legacy.LegacyBaseline || !legacy.Fallback.Used {
		t.Fatalf("position-only legacy plan was not readable: %#v", legacy)
	}
}

func TestSamplingPlanStalenessTracksRevisionPolicyAndEvidence(t *testing.T) {
	input := testQualitySamplingInput()
	input.Evidence = []SamplingEvidence{{Identity: "e", Source: "frame-structure", Scope: "interval", Availability: "available"}}
	plan := resolveCanonicalSamplingPlan(input)
	if samplingPlanIsStale(plan, input.SourceFingerprint, input.SourceRevision, input.PolicyVersion, input.Evidence) {
		t.Fatal("matching frozen inputs were marked stale")
	}
	if !samplingPlanIsStale(plan, input.SourceFingerprint, "new-revision", input.PolicyVersion, input.Evidence) {
		t.Fatal("changed source revision was not marked stale")
	}
	if !samplingPlanIsStale(plan, input.SourceFingerprint, input.SourceRevision, "quality-hard-scene/v2", input.Evidence) {
		t.Fatal("changed policy was not marked stale")
	}
	changedEvidence := []SamplingEvidence{{Identity: "e", Source: "frame-structure", Version: "next", Scope: "interval", Availability: "available"}}
	if !samplingPlanIsStale(plan, input.SourceFingerprint, input.SourceRevision, input.PolicyVersion, changedEvidence) {
		t.Fatal("changed evidence version was not marked stale")
	}
}

func TestSamplingEvidenceAdaptersReusePersistedIntervalAndGlobalComponents(t *testing.T) {
	scan := models.ScanResult{
		Path:         "/media/example.mkv",
		SizeBytes:    1000,
		Duration:     600,
		Bitrate:      1000000,
		VideoCodec:   "h264",
		VideoStreams: models.JSONList{models.JSONMap{"index": 0}},
		FrameStructureAnalysis: models.JSONMap{
			"version":         2,
			"confidenceScore": 0.8,
			"windows": []any{models.JSONMap{
				"startSeconds":    120.0,
				"durationSeconds": 20.0,
				"analysis": models.JSONMap{
					"framesAnalyzed":   100,
					"bFrameRatio":      0.8,
					"averageGopLength": 30.0,
				},
			}},
		},
		RestorationAnalysis: models.JSONMap{"version": "restoration-v1", "status": "available"},
	}
	evidence := samplingEvidenceFromScan(scan)
	if len(evidence) < 4 {
		t.Fatalf("expected persisted evidence manifest, got %#v", evidence)
	}
	candidates := samplingCandidatesFromScan(scan)
	if len(candidates) != 1 || len(candidates[0].EvidenceRefs) != 1 {
		t.Fatalf("expected one interval candidate from frame structure evidence, got %#v", candidates)
	}
	plan := canonicalSamplingPlanFromScan(scan, testQualitySamplingBasePlanPolicy(), nil, samplingPlanPolicyVersion)
	if plan.Fallback.Used {
		t.Fatalf("interval frame/GOP evidence unexpectedly fell back: %#v", plan.Fallback)
	}
	if plan.PlanIdentity == "" || plan.SourceFingerprint == "" {
		t.Fatalf("persisted evidence did not freeze plan identity: %#v", plan)
	}
}

func testQualitySamplingBasePlanPolicy() FrameStructureSamplingPolicy {
	return FrameStructureSamplingPolicy{Windows: 4, InitialWindows: 2, WindowSeconds: 20, Positions: []float64{0.1, 0.4, 0.7, 0.9}}
}

func TestSamplingPlanFutureConsumersOnlyReadFrozenWindows(t *testing.T) {
	plan := resolveCanonicalSamplingPlan(SamplingPlanInput{
		BasePlan:             testQualitySamplingBasePlan(),
		SourceFingerprint:    "source",
		SourceRevision:       "revision",
		SourceStreamIdentity: "0:v:0",
		PolicyVersion:        samplingPlanPolicyVersion,
		Candidates: []SamplingCandidate{{
			Window:       SamplingWindow{StartSeconds: 100, DurationSeconds: 20},
			Roles:        []string{SamplingRoleHighMotion},
			EvidenceRefs: []string{"motion"},
		}},
		Evidence: []SamplingEvidence{{
			Identity:     "motion",
			Source:       "frame-signals",
			Scope:        "interval",
			Availability: "available",
			Signals:      map[string]float64{"motion": 0.9},
		}},
	})
	quality := SamplingPlanForPerceptualQuality(plan)
	qsv := SamplingPlanForQSVPumping(plan)
	if quality.PlanIdentity != plan.PlanIdentity || qsv.PlanIdentity != plan.PlanIdentity {
		t.Fatal("future consumer lost frozen plan identity")
	}
	if len(quality.Windows) == 0 || len(qsv.Windows) == 0 || qsv.Windows[0].Identity != quality.Windows[0].Identity {
		t.Fatalf("consumers did not preserve selected identities: quality=%#v qsv=%#v", quality, qsv)
	}
}

func TestLABWindowPlansPreserveExplicitPrimaryAndBoundSequentialAdditionalWindows(t *testing.T) {
	input := testQualitySamplingInput()
	input.Candidates = []SamplingCandidate{{
		Window:       SamplingWindow{StartSeconds: 100, DurationSeconds: 20},
		EvidenceRefs: []string{"hard"},
	}}
	input.Evidence = []SamplingEvidence{{
		Identity:     "hard",
		Source:       "frame-structure",
		Scope:        "interval",
		Availability: "available",
		Confidence:   0.9,
		Signals:      map[string]float64{"complexity": 0.95},
	}}
	plan := resolveCanonicalSamplingPlan(input)
	windows := labFidelityWindowPlans(302, 20, 600, plan)
	if len(windows) == 0 || !windows[0].Primary || windows[0].Start != 302 {
		t.Fatalf("explicit LAB primary was not retained first: %#v", windows)
	}
	if len(windows) > plan.Limits.MaximumWindows {
		t.Fatalf("LAB additional windows exceeded canonical bound: %d > %d", len(windows), plan.Limits.MaximumWindows)
	}
	if len(windows) == 1 || windows[1].Identity == "" {
		t.Fatalf("hard-scene additional window was not routed through the plan: %#v", windows)
	}
	for _, window := range windows[1:] {
		if window.Primary {
			t.Fatal("additional LAB window was marked primary")
		}
	}
}
