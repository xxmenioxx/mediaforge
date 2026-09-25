package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func TestTestEncodeCandidateCoverageOnlyUsesFrozenRepresentedWindows(t *testing.T) {
	selection := SamplingPlan{SelectedWindows: []SamplingWindow{
		{Identity: "primary", Primary: true, StartSeconds: 100, DurationSeconds: 20},
		{Identity: "hard-a", StartSeconds: 300, DurationSeconds: 20},
		{Identity: "hard-b", StartSeconds: 500, DurationSeconds: 20},
		{Identity: "hard-c", StartSeconds: 700, DurationSeconds: 20},
		{Identity: "hard-d", StartSeconds: 900, DurationSeconds: 20},
		{Identity: "hard-e", StartSeconds: 1100, DurationSeconds: 20},
	}}
	input := perceptualArtifactInput{SegmentStartSeconds: 100, SegmentDuration: 20}
	covered := 0
	for _, window := range SamplingPlanForPerceptualQuality(selection).Windows {
		if candidateCoversWindow(input, window) {
			covered++
			if !window.Primary {
				t.Fatalf("Test Encode segment falsely covered a non-primary window: %#v", window)
			}
		}
	}
	if covered != 1 {
		t.Fatalf("expected primary-only coverage, got %d", covered)
	}
}

func TestTestEncodeNonZeroSegmentKeepsFullSourcePlanAndNoFakePairs(t *testing.T) {
	plan := SamplingPlan{PlanIdentity: "full-plan-1800", SourceRevision: "source-1800", AssetDurationSeconds: 1800, SelectedWindows: []SamplingWindow{
		{Identity: "primary", Primary: true, StartSeconds: 600, DurationSeconds: 20, Roles: []string{"primary"}},
		{Identity: "hard-a", StartSeconds: 120, DurationSeconds: 20, Roles: []string{"hard_scene"}},
		{Identity: "hard-b", StartSeconds: 900, DurationSeconds: 20, Roles: []string{"hard_scene"}},
		{Identity: "hard-c", StartSeconds: 1500, DurationSeconds: 20, Roles: []string{"hard_scene"}},
		{Identity: "hard-d", StartSeconds: 300, DurationSeconds: 20, Roles: []string{"hard_scene"}},
		{Identity: "hard-e", StartSeconds: 30, DurationSeconds: 20, Roles: []string{"hard_scene"}},
	}}
	input := perceptualArtifactInput{SegmentStartSeconds: 600, SegmentDuration: 20, CandidateDuration: 20}
	selection := SamplingPlanForPerceptualQuality(plan)
	coveredIDs := []string{}
	var primary SamplingWindow
	for _, window := range selection.Windows {
		if candidateCoversWindow(input, window) {
			coveredIDs = append(coveredIDs, window.Identity)
			primary = window
		}
	}
	if plan.AssetDurationSeconds != 1800 || len(coveredIDs) != 1 || primary.Identity != "primary" {
		t.Fatalf("Test Encode changed the source-relative plan or coverage: duration=%v covered=%v primary=%#v", plan.AssetDurationSeconds, coveredIDs, primary)
	}
	if primary.StartSeconds != 600 || primary.StartSeconds+primary.DurationSeconds != 620 {
		t.Fatalf("segment coverage was not source-relative to 600..620: %#v", primary)
	}
	for _, window := range selection.Windows {
		if window.Identity != primary.Identity && candidateCoversWindow(input, window) {
			t.Fatalf("unrelated frozen window was treated as represented: %#v", window)
		}
	}

	pair := testPair(t, "test-encode-generation-a")
	set, err := BuildPerceptualEvidenceSet(plan, "source-1800", "runtime-a", nil, []PerceptualWindowEvidence{{
		WindowIdentity: primary.Identity, StartSeconds: primary.StartSeconds, DurationSeconds: primary.DurationSeconds,
		Primary: true, Status: "covered", PairIdentity: &pair,
	}}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, window := range set.Windows {
		if window.WindowIdentity != primary.Identity && (window.Status != "not_covered" || window.PairIdentity != nil || window.Candidate != nil) {
			t.Fatalf("uncovered Test Encode window received fake evidence: %#v", window)
		}
	}
}

func TestRawSourceIsRejectedWhenFrozenAuthorityTransformsTimeline(t *testing.T) {
	identity, comparable, reason, err := perceptualReferenceTransform(models.Profile{WorkerConfig: models.JSONMap{
		"effectiveCadenceOperation": "inverse_telecine",
	}})
	if err != nil || identity == "" || comparable || reason == "" {
		t.Fatalf("cadence transform was not rejected as raw comparison: %q %t %q %v", identity, comparable, reason, err)
	}
	_, comparable, _, err = perceptualReferenceTransform(models.Profile{WorkerConfig: models.JSONMap{
		"effectiveCadenceOperation": "preserve", "effectiveOutputProgressive": true, "fieldStructureMode": "preserve",
	}})
	if err != nil || !comparable {
		t.Fatalf("unchanged cadence/field authority should remain comparable: %t %v", comparable, err)
	}
}

func TestCanonicalReferenceAdapterUsesFrozenPhysicalTransforms(t *testing.T) {
	cases := []struct {
		name   string
		worker models.JSONMap
		want   string
	}{
		{name: "ivtc", worker: models.JSONMap{"effectiveCadenceOperation": "inverse_telecine", "videoFilters": "fieldmatch=order=tff,decimate"}, want: "fieldmatch"},
		{name: "deinterlace", worker: models.JSONMap{"fieldStructureMode": "deinterlace", "videoFilters": "bwdif=mode=send_frame"}, want: "bwdif"},
		{name: "scale-crop", worker: models.JSONMap{"effectiveOutputWidth": 1280, "effectiveOutputHeight": 720, "videoFilters": "crop=1920:1080:0:0,scale=1280:720"}, want: "scale=1280:720"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			identity, comparable, reason, err := perceptualReferenceTransform(models.Profile{WorkerConfig: tc.worker})
			if err != nil || identity == "" || !comparable || reason != "" {
				t.Fatalf("frozen transform was not adapted: identity=%q comparable=%t reason=%q err=%v", identity, comparable, reason, err)
			}
			if filter := perceptualReferenceFilter(models.Profile{WorkerConfig: tc.worker}); !strings.Contains(filter, tc.want) {
				t.Fatalf("reference adapter did not retain frozen filter %q: %q", tc.want, filter)
			}
		})
	}
}

func TestFrozenSamplingPlanRejectsSamePathSourceReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.mkv")
	if err := os.WriteFile(path, []byte("old source"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldRevision, err := sourceRevisionForPath(path)
	if err != nil {
		t.Fatal(err)
	}
	oldIdentity := samplingSourceIdentity(path, oldRevision)
	plan := SamplingPlan{SourceFingerprint: oldIdentity, SourceRevision: oldIdentity}
	if matches, matchErr := frozenSamplingPlanSourceMatches(path, plan); matchErr != nil || !matches {
		t.Fatalf("unchanged source was not accepted: matches=%t err=%v", matches, matchErr)
	}
	if err := os.WriteFile(path, []byte("replacement source with different revision"), 0o600); err != nil {
		t.Fatal(err)
	}
	matches, matchErr := frozenSamplingPlanSourceMatches(path, plan)
	if matchErr != nil || matches {
		t.Fatalf("same-path source replacement reused frozen authority: matches=%t err=%v", matches, matchErr)
	}
}

func TestPerceptualAlignmentUsesExplicitPTSContractAcrossTimelineModes(t *testing.T) {
	cases := []struct {
		name, mode                  string
		sourceStart, candidateStart float64
	}{
		{name: "preserve cadence", mode: "preserved_cadence", sourceStart: 10, candidateStart: 0},
		{name: "ivtc", mode: "frozen_ivtc", sourceStart: 10, candidateStart: 0},
		{name: "fps conversion", mode: "frozen_fps_conversion", sourceStart: 10, candidateStart: 0},
		{name: "deinterlace", mode: "frozen_deinterlace", sourceStart: 10, candidateStart: 0},
		{name: "test encode trim", mode: "trimmed_candidate_segment", sourceStart: 600, candidateStart: 0},
		{name: "final output", mode: "full_output_source_timeline", sourceStart: 600, candidateStart: 600},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			alignment := perceptualAlignmentEvidenceForWindow(tc.sourceStart, 20, tc.candidateStart, 20, tc.mode, "source-revision", "transform-1", "candidate-timeline")
			if alignment.FramePairingMode != "pts_with_tolerance" || alignment.FramePairingMode == "frame_index" {
				t.Fatalf("timeline-changing comparison used frame-index pairing: %#v", alignment)
			}
			if alignment.TimestampOrigin != "normalized-zero-based" || alignment.ReferenceStartPTS != 0 || alignment.ReferenceEndPTS != 20 {
				t.Fatalf("comparison timeline was not normalized locally: %#v", alignment)
			}
			if alignment.TimelineIdentity == "" || alignment.MappingMode != tc.mode || alignment.ToleranceSeconds <= 0 {
				t.Fatalf("alignment identity/mapping contract missing: %#v", alignment)
			}
			if tc.mode == "trimmed_candidate_segment" && alignment.CandidateStartPTS != 0 {
				t.Fatalf("trimmed candidate was not mapped to local zero: %#v", alignment)
			}
		})
	}
}

func TestPerceptualAlignmentModeUsesFrozenAuthority(t *testing.T) {
	cases := []struct {
		name, want string
		profile    models.Profile
	}{
		{name: "preserve", want: "trimmed_candidate_segment", profile: models.Profile{WorkerConfig: models.JSONMap{"effectiveCadenceOperation": "preserve", "fieldStructureMode": "preserve"}}},
		{name: "ivtc", want: "trimmed_candidate_segment+ivtc", profile: models.Profile{WorkerConfig: models.JSONMap{"effectiveCadenceOperation": "inverse_telecine"}}},
		{name: "fps", want: "trimmed_candidate_segment+fps_conversion", profile: models.Profile{WorkerConfig: models.JSONMap{"effectiveCadenceOperation": "convert", "effectiveOutputFrameRate": "24000/1001"}}},
		{name: "deinterlace", want: "trimmed_candidate_segment+deinterlace", profile: models.Profile{WorkerConfig: models.JSONMap{"fieldStructureMode": "deinterlace", "videoFilters": "bwdif=mode=send_frame"}}},
		{name: "final", want: "full_output_source_timeline", profile: models.Profile{WorkerConfig: models.JSONMap{"effectiveCadenceOperation": "preserve"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := perceptualAlignmentMode(tc.profile, tc.name == "final"); got != tc.want {
				t.Fatalf("alignment mode=%q want %q", got, tc.want)
			}
		})
	}
}

func TestCanonicalComparisonGeometryUsesPostTransformAuthority(t *testing.T) {
	source := previewVideoCharacteristics{Width: 720, Height: 480, SampleAspectRatio: "8:9", DisplayAspectRatio: "4:3"}
	candidate := previewVideoCharacteristics{Width: 1280, Height: 720, SampleAspectRatio: "1:1", DisplayAspectRatio: "16:9"}
	profile := models.Profile{WorkerConfig: models.JSONMap{
		"videoFilters":         "crop=704:480:8:0,scale=1280:720",
		"effectiveOutputWidth": 1280, "effectiveOutputHeight": 720,
	}}
	reference, output := canonicalComparisonDomainFiltersForProfile(source, candidate, profile, "")
	if strings.Contains(reference, "scale=640:480") || strings.Contains(reference, "scale=720:480") {
		t.Fatalf("reference reused pre-transform anamorphic geometry: %q", reference)
	}
	if !strings.Contains(reference, "crop=704:480:8:0,scale=1280:720") || !strings.Contains(reference, "setpts=PTS-STARTPTS") {
		t.Fatalf("frozen post-transform reference graph was not preserved: %q", reference)
	}
	if !strings.Contains(output, "format=yuv420p") || !strings.Contains(output, "setsar=1") || !strings.Contains(output, "setpts=PTS-STARTPTS") {
		t.Fatalf("candidate comparison geometry was not canonical: %q", output)
	}

	cropScale, _ := canonicalComparisonDomainFiltersWithExpectedGeometry(source, previewVideoCharacteristics{Width: 720, Height: 480, SampleAspectRatio: "1:1"}, "crop=704:480,scale=1280:720", "", 1280, 720)
	if strings.Contains(cropScale, "scale=640:480") || !strings.Contains(cropScale, "scale=1280:720") {
		t.Fatalf("crop+scale reference geometry was not post-transform: %q", cropScale)
	}
	preserveReference, preserveCandidate := canonicalComparisonDomainFilters(source, candidate, "", "")
	if !strings.Contains(preserveReference, "scale=640:480") || !strings.Contains(preserveCandidate, "setsar=1") {
		t.Fatalf("preserve path lost source square-pixel normalization: %q / %q", preserveReference, preserveCandidate)
	}
}

func TestArtifactEvidenceOnlyProjectsExistingDeterministicSignals(t *testing.T) {
	available := 1.25
	raw := models.JSONMap{
		"version":              restorationAnalysisVersion,
		"blocking":             models.JSONMap{"availability": "available", "severity": "unclassified", "value": available, "confidence": "medium", "supportingEvidence": []string{"lavfi.block"}},
		"banding":              models.JSONMap{"availability": "unavailable"},
		"ringing":              models.JSONMap{"availability": "unavailable"},
		"edgeDetailConfidence": models.JSONMap{"availability": "unavailable"},
	}
	evidence := artifactEvidenceFromProvenance(models.JSONMap{"restorationAnalysis": raw})
	if len(evidence) != 1 || evidence[0].Kind != "blocking" || evidence[0].Status != "available" {
		t.Fatalf("deterministic blocking evidence was not projected: %#v", evidence)
	}
	for _, item := range evidence {
		if item.Kind == "banding" || item.Kind == "ringing_halo" || item.Kind == "temporal_instability" {
			t.Fatalf("unavailable analyzer was fabricated: %#v", item)
		}
	}
	plan := SamplingPlan{PlanIdentity: "artifact-plan", SelectedWindows: []SamplingWindow{{Identity: "primary", Primary: true, StartSeconds: 0, DurationSeconds: 5}}}
	set, err := BuildPerceptualEvidenceSet(plan, "source", "runtime", nil, nil, nil, nil, models.JSONMap{"restorationAnalysis": raw})
	if err != nil || len(set.ArtifactEvidence) != 1 {
		t.Fatalf("structured artifact evidence was not attached independently of metric aggregates: %#v %v", set.ArtifactEvidence, err)
	}
	withoutAnalyzer, err := BuildPerceptualEvidenceSet(plan, "source", "runtime", nil, nil, nil, nil, nil)
	if err != nil || len(withoutAnalyzer.ArtifactEvidence) != 0 {
		t.Fatalf("missing analyzer should remain absent without blocking evidence: %#v %v", withoutAnalyzer.ArtifactEvidence, err)
	}
}
