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
