package handlers

import (
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
