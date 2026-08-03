package handlers

import "testing"

func TestClassifyInterlace(t *testing.T) {
	analysis := InterlaceAnalysis{TFF: 420, BFF: 20, Progressive: 40, SampledFrames: 500}
	classifyInterlace(&analysis)
	if analysis.Status != "interlaced" || analysis.RecommendedFilter == "" {
		t.Fatalf("expected interlaced recommendation, got %#v", analysis)
	}
}

func TestClassifyInterlacePersistsVersionAndMeasuredBFF(t *testing.T) {
	analysis := InterlaceAnalysis{BFF: 501, SampledFrames: 501}
	classifyInterlace(&analysis)

	if analysis.Version != interlaceAnalysisVersion {
		t.Fatalf("expected analysis version %d, got %d", interlaceAnalysisVersion, analysis.Version)
	}
	if analysis.DetectedFieldOrder != "bff" {
		t.Fatalf("expected measured BFF order, got %q", analysis.DetectedFieldOrder)
	}
	if analysis.RecommendedFilter != "bwdif=mode=send_frame:parity=bff:deint=all" {
		t.Fatalf("expected explicit BFF recommendation, got %q", analysis.RecommendedFilter)
	}
}

func TestBWDIFRecoversParityFromLegacyFrameCounts(t *testing.T) {
	analysis := InterlaceAnalysis{Status: "interlaced", BFF: 501, SampledFrames: 501}
	if filter := bwdifFilter(analysis); filter != "bwdif=mode=send_frame:parity=bff:deint=all" {
		t.Fatalf("expected legacy counts to recover BFF parity, got %q", filter)
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

func TestEffectiveDeinterlaceFilterDoesNotPrefixIVTC(t *testing.T) {
	analysis := InterlaceAnalysis{Status: "telecine_suspected", FieldOrder: "tt"}
	if filter := effectiveDeinterlaceFilter("ivtc_tff", analysis); filter != "" {
		t.Fatalf("IVTC must be carried by the explicit fieldmatch chain, got %q", filter)
	}
}

func TestDominantFieldOrderOverridesIncorrectContainerMetadata(t *testing.T) {
	analysis := InterlaceAnalysis{
		ContainerFieldOrder: "bb",
		DetectedFieldOrder:  "tff",
		FieldOrder:          "tff",
		FieldOrderMismatch:  true,
	}
	if dominantFieldOrder(2722, 43) != "tff" {
		t.Fatal("expected distributed samples to select TFF")
	}
	if filter := bwdifFilter(analysis); filter != "bwdif=mode=send_frame:parity=tff:deint=all" {
		t.Fatalf("expected explicit measured parity, got %q", filter)
	}
}

func TestApplyIVTCValidationSelectsTFFForTerramarLikeEvidence(t *testing.T) {
	analysis := InterlaceAnalysis{ContainerFieldOrder: "bb"}
	validation := &IVTCValidation{
		TFFProgressive: 896, TFFClassified: 901, TFFProgressiveRatio: 896.0 / 901.0,
		BFFProgressive: 549, BFFClassified: 900, BFFProgressiveRatio: 549.0 / 900.0,
	}

	applyIVTCValidation(&analysis, validation)

	if analysis.Status != "telecine_suspected" || analysis.RecommendedMode != "ivtc_tff" {
		t.Fatalf("expected IVTC TFF recommendation, got %#v", analysis)
	}
	if analysis.RecommendedFilter != "fieldmatch=order=tff,decimate" {
		t.Fatalf("unexpected filter %q", analysis.RecommendedFilter)
	}
	if !analysis.FieldOrderMismatch {
		t.Fatal("expected BFF metadata versus TFF content mismatch")
	}
}

func TestDistributedInterlaceStartsSamplesLongAssets(t *testing.T) {
	starts := distributedInterlaceStarts(6600, 20)
	if len(starts) != 5 {
		t.Fatalf("expected five distributed samples, got %#v", starts)
	}
	if starts[0] <= 0 || starts[len(starts)-1] <= starts[0] {
		t.Fatalf("unexpected sample distribution: %#v", starts)
	}
}

func TestProgressiveFramesRecommendMetadataCorrectionForInterlacedContainer(t *testing.T) {
	analysis := InterlaceAnalysis{
		Status:              "progressive",
		ContainerFieldOrder: "tt",
	}

	finalizeFieldMetadataAnalysis(&analysis)

	if !analysis.FieldOrderMismatch || analysis.DetectedFieldOrder != "progressive" {
		t.Fatalf("expected progressive mismatch, got %#v", analysis)
	}
	if analysis.RecommendedFieldMetadataMode != "progressive" {
		t.Fatalf("expected progressive metadata recommendation, got %q", analysis.RecommendedFieldMetadataMode)
	}
}

func TestAutomaticFieldMetadataMarksProgressiveFramesWithoutDeinterlacing(t *testing.T) {
	analysis := InterlaceAnalysis{Status: "progressive", ContainerFieldOrder: "tt"}
	analysis.FieldOrderMismatch = true
	if filter := effectiveAutomaticFieldMetadataFilter("", "", analysis); filter != "setfield=prog" {
		t.Fatalf("expected progressive metadata filter, got %q", filter)
	}
}

func TestBWDIFAlwaysMarksOutputProgressive(t *testing.T) {
	analysis := InterlaceAnalysis{Status: "interlaced", ContainerFieldOrder: "tt"}
	if filter := effectiveAutomaticFieldMetadataFilter("bwdif=mode=send_frame:parity=tff:deint=all", "", analysis); filter != "setfield=prog" {
		t.Fatalf("expected bwdif output to be marked progressive, got %q", filter)
	}
}
