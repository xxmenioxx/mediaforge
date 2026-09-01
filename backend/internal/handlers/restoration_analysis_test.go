package handlers

import (
	"math"
	"strings"
	"testing"
)

func TestParseAndClassifyRestorationEvidenceKeepsAmbiguityExplicit(t *testing.T) {
	first := parseRestorationWindowEvidence(strings.Join([]string{
		"lavfi.block=10.000000",
		"lavfi.bitplanenoise.0.1=0.030000",
		"lavfi.bitplanenoise.1.1=0.060000",
		"lavfi.bitplanenoise.2.1=0.080000",
	}, "\n"))
	second := parseRestorationWindowEvidence(strings.Join([]string{
		"lavfi.block=12.000000",
		"lavfi.bitplanenoise.0.1=0.050000",
		"lavfi.bitplanenoise.1.1=0.040000",
		"lavfi.bitplanenoise.2.1=0.060000",
	}, "\n"))
	analysis := classifyRestorationEvidence([]restorationWindowEvidence{first, second})
	if analysis.Version != restorationAnalysisVersion || analysis.Status != "available" || analysis.Windows != 2 || analysis.SampledFrames != 2 {
		t.Fatalf("unexpected analysis envelope: %#v", analysis)
	}
	if analysis.Blocking.Availability != "available" || analysis.Blocking.Value == nil || *analysis.Blocking.Value != 11 || analysis.Blocking.Severity != "unclassified" || analysis.Blocking.Confidence != "medium" {
		t.Fatalf("blocking evidence was not preserved honestly: %#v", analysis.Blocking)
	}
	if analysis.Noise.Availability != "ambiguous" || analysis.Grain.Availability != "ambiguous" || analysis.Noise.Severity != "unknown" || analysis.Grain.Severity != "unknown" {
		t.Fatalf("noise/grain ambiguity was collapsed: noise=%#v grain=%#v", analysis.Noise, analysis.Grain)
	}
	if analysis.ChromaNoise.Availability != "ambiguous" || analysis.ChromaNoise.Value == nil || math.Abs(*analysis.ChromaNoise.Value-.06) > 0.000001 {
		t.Fatalf("chroma evidence mismatch: %#v", analysis.ChromaNoise)
	}
	for name, signal := range map[string]RestorationSignalEvidence{"banding": analysis.Banding, "ringing": analysis.Ringing, "edgeDetail": analysis.EdgeDetail} {
		if signal.Availability != "unavailable" || signal.Confidence != "unavailable" || signal.Severity != "unknown" {
			t.Fatalf("%s fabricated evidence: %#v", name, signal)
		}
	}
}

func TestRestorationEvidenceUnavailableIsNotLow(t *testing.T) {
	analysis := restorationEvidenceUnavailable()
	if analysis.Status != "unavailable" || analysis.Blocking.Availability != "unavailable" || analysis.Blocking.Confidence == "low" || analysis.Blocking.Value != nil {
		t.Fatalf("unavailable evidence was represented as low: %#v", analysis)
	}
}

func TestRestorationMetricFilterDoesNotUsePreviewComparisonMetrics(t *testing.T) {
	filter := restorationMetricFilterChain()
	if !strings.Contains(filter, "blockdetect") || !strings.Contains(filter, "bitplanenoise") {
		t.Fatalf("source metric filters missing: %q", filter)
	}
	if strings.Contains(strings.ToLower(filter), "psnr") || strings.Contains(strings.ToLower(filter), "ssim") {
		t.Fatalf("Preview comparison metric leaked into source analysis: %q", filter)
	}
}
