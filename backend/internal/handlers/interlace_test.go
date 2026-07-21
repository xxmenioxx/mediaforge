package handlers

import "testing"

func TestClassifyInterlace(t *testing.T) {
	analysis := InterlaceAnalysis{TFF: 420, BFF: 20, Progressive: 40, SampledFrames: 500}
	classifyInterlace(&analysis)
	if analysis.Status != "interlaced" || analysis.RecommendedFilter == "" {
		t.Fatalf("expected interlaced recommendation, got %#v", analysis)
	}
}

func TestClassifyInterlaceKeepsMixedForReview(t *testing.T) {
	analysis := InterlaceAnalysis{TFF: 100, BFF: 0, Progressive: 300, SampledFrames: 500}
	classifyInterlace(&analysis)
	if analysis.Status != "mixed" || analysis.RecommendedFilter != "" {
		t.Fatalf("expected mixed review result, got %#v", analysis)
	}
}

func TestClassifyInterlaceFlagsRepeatedFieldsAsTelecine(t *testing.T) {
	analysis := InterlaceAnalysis{TFF: 300, Progressive: 100, RepeatedTop: 80, SampledFrames: 500}
	classifyInterlace(&analysis)
	if analysis.Status != "telecine_suspected" {
		t.Fatalf("expected telecine warning, got %#v", analysis)
	}
}
