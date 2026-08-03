package handlers

import "testing"

func TestClassifyCropCandidatesRecommendsStableSideBars(t *testing.T) {
	analysis := CropAnalysis{Status: "unknown", OriginalWidth: 1920, OriginalHeight: 1080, Windows: 3}
	candidate := cropCandidate{width: 1440, height: 1080, x: 240, y: 0}

	result := classifyCropCandidates(analysis, []cropCandidate{candidate, candidate, candidate})

	if result.Status != "detected" {
		t.Fatalf("expected detected, got %q", result.Status)
	}
	if result.RecommendedCrop != "1440:1080:240:0" {
		t.Fatalf("unexpected crop: %q", result.RecommendedCrop)
	}
	if result.Confidence != 1 {
		t.Fatalf("expected full confidence, got %v", result.Confidence)
	}
}

func TestClassifyCropCandidatesRejectsVariableBorders(t *testing.T) {
	analysis := CropAnalysis{Status: "unknown", OriginalWidth: 1920, OriginalHeight: 1080, Windows: 3}
	result := classifyCropCandidates(analysis, []cropCandidate{
		{width: 1440, height: 1080, x: 240},
		{width: 1920, height: 800, y: 140},
		{width: 1800, height: 1080, x: 60},
	})

	if result.Status != "variable" {
		t.Fatalf("expected variable, got %q", result.Status)
	}
	if result.RecommendedCrop != "" {
		t.Fatalf("variable borders must not recommend a crop: %q", result.RecommendedCrop)
	}
}

func TestClassifyCropCandidatesOffersConservativeDisabledCandidateForSimilarBorders(t *testing.T) {
	analysis := CropAnalysis{Status: "unknown", OriginalWidth: 720, OriginalHeight: 480, Windows: 3}
	result := classifyCropCandidates(analysis, []cropCandidate{
		{width: 674, height: 442, x: 24, y: 18},
		{width: 680, height: 444, x: 22, y: 18},
		{width: 676, height: 442, x: 22, y: 18},
	})

	if result.Status != "variable" {
		t.Fatalf("nearby candidates must remain manual, got %q", result.Status)
	}
	if result.RecommendedCrop != "680:444:22:18" {
		t.Fatalf("unexpected conservative crop: %q", result.RecommendedCrop)
	}
	if result.MatchingWindows != 3 || result.Confidence != 1 {
		t.Fatalf("unexpected clustered confidence: %d %.2f", result.MatchingWindows, result.Confidence)
	}
}

func TestClassifyCropCandidatesIgnoresSmallOverscan(t *testing.T) {
	analysis := CropAnalysis{Status: "unknown", OriginalWidth: 1920, OriginalHeight: 1080, Windows: 3}
	candidate := cropCandidate{width: 1912, height: 1076, x: 4, y: 2}

	result := classifyCropCandidates(analysis, []cropCandidate{candidate, candidate, candidate})

	if result.Status != "none" {
		t.Fatalf("expected none for small borders, got %q", result.Status)
	}
}

func TestClassifyCropCandidatesAcceptsSmallDVDMatteVariation(t *testing.T) {
	analysis := CropAnalysis{Status: "unknown", OriginalWidth: 720, OriginalHeight: 480, Windows: 3}
	result := classifyCropCandidates(analysis, []cropCandidate{
		{width: 720, height: 286, x: 0, y: 98},
		{width: 720, height: 286, x: 0, y: 98},
		{width: 720, height: 284, x: 0, y: 100},
	})

	if result.Status != "detected" {
		t.Fatalf("expected stable DVD matte to be detected, got %q", result.Status)
	}
	if result.RecommendedCrop != "720:286:0:98" {
		t.Fatalf("unexpected DVD crop: %q", result.RecommendedCrop)
	}
}
