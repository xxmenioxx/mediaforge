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

func TestClassifyInterlaceKeepsHybridForReview(t *testing.T) {
	analysis := InterlaceAnalysis{TFF: 100, BFF: 0, Progressive: 300, SampledFrames: 500}
	classifyInterlace(&analysis)
	if analysis.Status != "hybrid" || analysis.RecommendedAction != "review" || analysis.RecommendedFilter != "" {
		t.Fatalf("expected hybrid review result, got %#v", analysis)
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

func TestAutomaticDeinterlaceDoesNotFilterUnvalidatedTelecine(t *testing.T) {
	analysis := InterlaceAnalysis{Status: "telecine_suspected", DetectedFieldOrder: "bff"}
	if filter := effectiveDeinterlaceFilter("auto", analysis); filter != "" {
		t.Fatalf("expected unvalidated telecine to remain review-only, got %q", filter)
	}
	analysis.Status = "telecine"
	analysis.RecommendedFilter = "fieldmatch=order=bff,decimate"
	if filter := effectiveDeinterlaceFilter("auto", analysis); filter != analysis.RecommendedFilter {
		t.Fatalf("expected validated IVTC recommendation, got %q", filter)
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

	if analysis.Status != "telecine" || analysis.RecommendedMode != "ivtc_tff" {
		t.Fatalf("expected IVTC TFF recommendation, got %#v", analysis)
	}
	if analysis.RecommendedFilter != "fieldmatch=order=tff,decimate" {
		t.Fatalf("unexpected filter %q", analysis.RecommendedFilter)
	}
	if !analysis.FieldOrderMismatch {
		t.Fatal("expected BFF metadata versus TFF content mismatch")
	}
}

func TestDistributedClassificationDetectsHybridWindows(t *testing.T) {
	analysis := InterlaceAnalysis{Windows: []InterlaceWindow{
		{TFF: 5, Progressive: 95, SampledFrames: 100},
		{TFF: 95, Progressive: 5, SampledFrames: 100},
	}}

	classifyInterlace(&analysis)

	if analysis.Status != "hybrid" || analysis.RecommendedAction != "review" || analysis.AutomaticFilter != "" {
		t.Fatalf("expected divergent windows to remain review-only, got %#v", analysis)
	}
}

func TestDistributedClassificationDeinterlacesOnlyConsistentInterlace(t *testing.T) {
	analysis := InterlaceAnalysis{DetectedFieldOrder: "tff", Windows: []InterlaceWindow{
		{TFF: 95, Progressive: 5, SampledFrames: 100},
		{TFF: 90, Progressive: 10, SampledFrames: 100},
		{TFF: 98, Progressive: 2, SampledFrames: 100},
	}}

	classifyInterlace(&analysis)

	if analysis.Status != "interlaced" || analysis.RecommendedAction != "deinterlace" {
		t.Fatalf("expected consistent interlacing, got %#v", analysis)
	}
	if analysis.AutomaticFilter != "bwdif=mode=send_frame:parity=tff:deint=all" {
		t.Fatalf("unexpected automatic filter %q", analysis.AutomaticFilter)
	}
}

func TestWindowClassificationUsesDecodedFrameSignalsWhenIDETIsUnavailable(t *testing.T) {
	window := InterlaceWindow{FrameSignals: FrameSignalSummary{
		DecodedFrames: 100, InterlacedFrames: 90, ProgressiveFrames: 10,
	}}

	classifyInterlaceWindow(&window)

	if window.Status != "interlaced" || window.Confidence != .9 {
		t.Fatalf("expected decoded frame evidence to classify the window, got %#v", window)
	}
}

func TestWindowClassificationMarksContradictoryDetectorsUnknown(t *testing.T) {
	window := InterlaceWindow{
		TFF: 95, Progressive: 5, SampledFrames: 100,
		FrameSignals: FrameSignalSummary{DecodedFrames: 100, InterlacedFrames: 2, ProgressiveFrames: 98},
	}

	classifyInterlaceWindow(&window)

	if window.Status != "unknown" {
		t.Fatalf("expected contradictory IDET and decoded-frame evidence to remain unknown, got %#v", window)
	}
}

func TestDistributedClassificationIsIndependentOfWindowOrder(t *testing.T) {
	windows := []InterlaceWindow{
		{TFF: 95, Progressive: 5, SampledFrames: 100},
		{TFF: 5, Progressive: 95, SampledFrames: 100},
		{TFF: 90, Progressive: 10, SampledFrames: 100},
	}
	forward := InterlaceAnalysis{Windows: append([]InterlaceWindow(nil), windows...)}
	reverse := InterlaceAnalysis{Windows: []InterlaceWindow{windows[2], windows[1], windows[0]}}

	classifyInterlace(&forward)
	classifyInterlace(&reverse)

	if forward.Status != "hybrid" || reverse.Status != forward.Status || reverse.RecommendedAction != forward.RecommendedAction {
		t.Fatalf("window position/order changed the decision: forward=%#v reverse=%#v", forward, reverse)
	}
}

func TestIVTCRequiresDistributedWindowAgreement(t *testing.T) {
	analysis := InterlaceAnalysis{ContainerFieldOrder: "tt"}
	validation := &IVTCValidation{
		TFFProgressiveRatio: .95, BFFProgressiveRatio: .50,
		Windows: []IVTCWindowValidation{
			{SelectedOrder: "tff", Confidence: .96},
			{SelectedOrder: "", Confidence: .70},
			{SelectedOrder: "bff", Confidence: .93},
		},
	}

	applyIVTCValidation(&analysis, validation)

	if analysis.Status == "telecine" || analysis.RecommendedFilter != "" {
		t.Fatalf("expected conflicting IVTC windows to remain unvalidated, got %#v", analysis)
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

func TestSharedFrameSignalsMatchCanonicalPosition(t *testing.T) {
	want := FrameSignalSummary{DecodedFrames: 240, ProgressiveFrames: 240, EffectiveFPS: 23.976}
	analysis := QSVFrameStructureAnalysis{Windows: []FrameStructureWindow{
		{Position: 0.08, FrameSignals: FrameSignalSummary{DecodedFrames: 100}},
		{Position: 0.50, FrameSignals: want},
	}}

	got, ok := sharedFrameSignalsForPosition(analysis, 0.50)
	if !ok || got.DecodedFrames != want.DecodedFrames || got.EffectiveFPS != want.EffectiveFPS {
		t.Fatalf("shared signals=%#v ok=%t want=%#v", got, ok, want)
	}
	if _, ok := sharedFrameSignalsForPosition(analysis, 0.73); ok {
		t.Fatal("unexpected shared evidence for an unsampled position")
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
