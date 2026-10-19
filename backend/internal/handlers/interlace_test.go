package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

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

func TestDeepIVTCEvidenceJSONRoundTripPreservesSourceRevision(t *testing.T) {
	const snapshotUpdatedAt = "2026-10-18T23:53:21.123456789Z"
	const sourceModifiedAt = "2026-10-18T23:50:00.987654321Z"
	want := InterlaceAnalysis{Version: interlaceAnalysisVersion, IVTCValidation: &IVTCValidation{
		DeepAnalysisVersion:       ivtcDeepEvidenceVersion,
		DeepAnalysisStatus:        "resolved",
		EvidenceSnapshotUpdatedAt: snapshotUpdatedAt,
		SourceFrameRate:           "30000/1001",
		ExpectedOutputFrameRate:   "24000/1001",
		AttemptedWindows:          5,
		ValidWindows:              3,
		InconclusiveWindows:       2,
		SourceIdentity: &IVTCSourceIdentity{
			Path:              "/synthetic/source.mkv",
			SnapshotID:        19,
			SnapshotUpdatedAt: snapshotUpdatedAt,
			SizeBytes:         12345,
			ModifiedAt:        sourceModifiedAt,
			Device:            "4",
			Inode:             "87",
			AssetRecordID:     23,
		},
		Windows: []IVTCWindowValidation{{
			TFFResidualInterlaced: 4, TFFRepeatedTop: 2, TFFRepeatedBottom: 1,
			BFFResidualInterlaced: 7, BFFRepeatedTop: 3, BFFRepeatedBottom: 2,
		}},
	}}
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got InterlaceAnalysis
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got.IVTCValidation == nil || got.IVTCValidation.DeepAnalysisVersion != ivtcDeepEvidenceVersion ||
		got.IVTCValidation.EvidenceSnapshotUpdatedAt != snapshotUpdatedAt || got.IVTCValidation.SourceIdentity == nil ||
		got.IVTCValidation.SourceIdentity.SnapshotUpdatedAt != snapshotUpdatedAt || got.IVTCValidation.SourceIdentity.ModifiedAt != sourceModifiedAt ||
		got.IVTCValidation.SourceIdentity.SnapshotID != 19 || got.IVTCValidation.SourceIdentity.AssetRecordID != 23 {
		t.Fatalf("deep cadence JSON round trip lost its version or exact identity revision: %#v", got.IVTCValidation)
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
	if analysis.RecommendedFilter != "fieldmatch=order=tff:combmatch=full,bwdif=mode=send_frame:parity=tff:deint=interlaced,decimate,setfield=prog" {
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

func TestFinalizeIVTCValidationWeightsByClassifiedFramesAndStillRequiresWindowAgreement(t *testing.T) {
	validation := &IVTCValidation{
		TFFProgressive: 1801, TFFClassified: 2100,
		BFFProgressive: 210, BFFClassified: 2100,
		Windows: []IVTCWindowValidation{
			{Start: 0, TFFProgressive: 1800, TFFClassified: 2000, BFFProgressive: 200, BFFClassified: 2000, SelectedOrder: "tff"},
			{Start: 40, TFFProgressive: 1, TFFClassified: 100, BFFProgressive: 10, BFFClassified: 100},
		},
	}
	analysis := InterlaceAnalysis{}
	finalizeIVTCValidation(&analysis, validation)

	if validation.TFFProgressiveRatio != 1801.0/2100 || validation.BFFProgressiveRatio != 210.0/2100 {
		t.Fatalf("aggregate ratios did not use classified-frame denominators: %#v", validation)
	}
	if selected, _ := validatedIVTCOrder(validation.TFFProgressiveRatio, validation.BFFProgressiveRatio); selected != "tff" {
		t.Fatalf("weighted aggregate fixture should cross the unchanged threshold: %q", selected)
	}
	if validation.SelectedOrder != "" || ivtcWindowsAgree(validation, "tff") {
		t.Fatalf("one favorable window must not override incomplete window agreement: %#v", validation)
	}
}

func TestIVTCWindowValidationCapturesResidualInterlaceBeforeFallback(t *testing.T) {
	var filters []string
	runner := func(_ context.Context, _ string, _ float64, _ int, filter string) (InterlaceAnalysis, bool) {
		filters = append(filters, filter)
		if strings.Contains(filter, "showinfo") {
			if strings.Contains(filter, "order=tff") {
				return syntheticCanonicalIVTCOutput(10), true
			}
			return InterlaceAnalysis{Progressive: 118, BFF: 2, RepeatedTop: 1, RepeatedBottom: 2, CanonicalOutputFrames: 120, CanonicalOutputProgressive: 120, CanonicalOutputFrameRate: 24000.0 / 1001.0}, true
		}
		if strings.Contains(filter, "order=tff") {
			return InterlaceAnalysis{Progressive: 90, TFF: 6, BFF: 4, RepeatedTop: 7, RepeatedBottom: 3}, true
		}
		return InterlaceAnalysis{Progressive: 40, TFF: 50, BFF: 10, RepeatedTop: 1, RepeatedBottom: 2}, true
	}
	window, err := compareIVTCWindow(context.Background(), "/synthetic/telecine.mkv", 12, 5, runner)
	if err != nil {
		t.Fatal(err)
	}
	if !equalStringSlices(filters, []string{
		"fieldmatch=order=tff,decimate,idet",
		"fieldmatch=order=bff,decimate,idet",
		canonicalIVTCDiagnosticFilter("tff"),
		canonicalIVTCDiagnosticFilter("bff"),
	}) {
		t.Fatalf("order scoring must preserve the existing authority and diagnostics must mirror the canonical pre/post-fallback chain: %v", filters)
	}
	if window.Outcome != ivtcWindowOutcomeValid || window.SelectedOrder != "tff" {
		t.Fatalf("conclusive TFF comparison was not retained: %#v", window)
	}
	if !window.DiagnosticsAvailable || window.TFFResidualInterlaced != 10 || window.TFFRepeatedTop != 1 || window.TFFRepeatedBottom != 2 ||
		window.BFFResidualInterlaced != 0 || window.BFFRepeatedTop != 1 || window.BFFRepeatedBottom != 2 ||
		window.TFFCanonicalOutputFrames != 120 || window.TFFCanonicalOutputFrameRate != 24000.0/1001.0 ||
		window.TFFCanonicalIDETInterlaced != 10 || window.TFFCanonicalIDETClassified != 120 ||
		window.BFFCanonicalIDETInterlaced != 2 || window.BFFCanonicalIDETClassified != 120 {
		t.Fatalf("pre-fallback combing and canonical timing evidence was not retained per order: %#v", window)
	}
}

func ambiguousDeepIVTCFixture() (InterlaceAnalysis, SamplingPlan) {
	analysis := InterlaceAnalysis{WindowSeconds: 5, ContainerFieldOrder: "tt"}
	analysis.IVTCValidation = &IVTCValidation{
		TFFProgressive: 90, TFFClassified: 100, TFFProgressiveRatio: .90,
		BFFProgressive: 45, BFFClassified: 100, BFFProgressiveRatio: .45,
		DurationSeconds: 100, WindowSeconds: 5,
		Windows: []IVTCWindowValidation{
			{Start: 0, SelectedOrder: "tff", Confidence: .95, Outcome: ivtcWindowOutcomeValid,
				TFFProgressive: 90, TFFClassified: 100, TFFProgressiveRatio: .90,
				BFFProgressive: 45, BFFClassified: 100, BFFProgressiveRatio: .45},
			{Start: 40, Confidence: .70, Outcome: ivtcWindowOutcomeInconclusive},
			{Start: 80, Confidence: .70, Outcome: ivtcWindowOutcomeInconclusive},
		},
	}
	plan := SamplingPlan{AssetDurationSeconds: 100, WindowSeconds: 5, SelectedWindows: []SamplingWindow{
		{StartSeconds: 20, DurationSeconds: 5, Roles: []string{SamplingRoleHighMotion}},
		{StartSeconds: 60, DurationSeconds: 5, Roles: []string{SamplingRoleHighComplexity}},
	}}
	return analysis, plan
}

func syntheticCanonicalIVTCOutput(residualInterlaced int) InterlaceAnalysis {
	return InterlaceAnalysis{
		ResidualInterlacedFrames:   residualInterlaced,
		TFF:                        10,
		Progressive:                110,
		RepeatedTop:                1,
		RepeatedBottom:             2,
		CanonicalOutputFrames:      120,
		CanonicalOutputProgressive: 120,
		CanonicalOutputFrameRate:   24000.0 / 1001.0,
	}
}

func TestDeepIVTCAnalysisResolvesFromTwoAdditionalDistributedWindows(t *testing.T) {
	analysis, plan := ambiguousDeepIVTCFixture()
	identity := &IVTCSourceIdentity{Path: "/synthetic/telecine.mkv", SnapshotID: 7}
	runner := func(_ context.Context, _ string, _ float64, _ int, filter string) (InterlaceAnalysis, bool) {
		if strings.Contains(filter, "showinfo") {
			return syntheticCanonicalIVTCOutput(0), true
		}
		if strings.Contains(filter, "order=tff") {
			return InterlaceAnalysis{Progressive: 95, TFF: 3, BFF: 2}, true
		}
		return InterlaceAnalysis{Progressive: 50, TFF: 25, BFF: 25}, true
	}

	if err := extendIVTCValidationContext(context.Background(), "/synthetic/telecine.mkv", &analysis, plan, identity, runner); err != nil {
		t.Fatal(err)
	}
	validation := analysis.IVTCValidation
	if validation.DeepAnalysisVersion != ivtcDeepEvidenceVersion || validation.DeepAnalysisStatus != "resolved" || validation.SelectedOrder != "tff" {
		t.Fatalf("deep analysis did not resolve the conservative TFF decision: %#v", validation)
	}
	if validation.AttemptedWindows != 5 || validation.ValidatedWindows != 3 || validation.ValidWindows != 3 || validation.InconclusiveWindows != 2 || validation.ContradictoryWindows != 0 {
		t.Fatalf("unexpected deep window accounting: %#v", validation)
	}
	if analysis.RecommendedMode != "ivtc_tff" || !strings.Contains(analysis.RecommendedFilter, "fieldmatch=order=tff") {
		t.Fatalf("deep resolution did not reuse canonical IVTC planning: %#v", analysis)
	}
	if validation.Windows[3].SelectionBasis != "existing_high_motion_sampling_evidence" {
		t.Fatalf("additional sample did not prioritize existing motion evidence: %#v", validation.Windows[3])
	}
}

func TestDeepIVTCSelectorSkipsReusedWindowsAndCapsAdditionalWork(t *testing.T) {
	analysis, _ := ambiguousDeepIVTCFixture()
	plan := SamplingPlan{AssetDurationSeconds: 100, WindowSeconds: 5, SelectedWindows: []SamplingWindow{
		{StartSeconds: 40, DurationSeconds: 5, Roles: []string{SamplingRoleHighMotion}},
		{StartSeconds: 10, DurationSeconds: 5},
		{StartSeconds: 55, DurationSeconds: 5},
		{StartSeconds: 90, DurationSeconds: 5},
	}}
	selected := selectDeepIVTCAdditionalWindows(analysis, plan, 10)
	if len(selected) != ivtcDeepMaximumExtraWindows {
		t.Fatalf("deep selector exceeded its bounded additional-window count: %#v", selected)
	}
	for _, candidate := range selected {
		for _, existing := range analysis.IVTCValidation.Windows {
			if candidate.Start == existing.Start {
				t.Fatalf("deep selector repeated an existing validation window: candidate=%#v existing=%#v", candidate, existing)
			}
		}
	}
	if noCandidates := selectDeepIVTCAdditionalWindows(analysis, SamplingPlan{}, ivtcDeepMaximumExtraWindows); len(noCandidates) != 0 {
		t.Fatalf("selector invented candidates without sampling evidence or a canonical duration: %#v", noCandidates)
	}
}

func TestDeepIVTCSelectorDeduplicatesCandidatesUsingHighestPriority(t *testing.T) {
	analysis := InterlaceAnalysis{
		IVTCValidation: &IVTCValidation{WindowSeconds: 5, Windows: []IVTCWindowValidation{{Start: 0}, {Start: 40}, {Start: 80}}},
		Windows: []InterlaceWindow{{
			Start: 10, Seconds: 5,
			FrameSignals: FrameSignalSummary{Cadence: "hard_telecine", RepeatPictFrames: 1, InterlacedFrames: 1},
		}},
	}
	plan := SamplingPlan{AssetDurationSeconds: 100, SelectedWindows: []SamplingWindow{
		{StartSeconds: 10, DurationSeconds: 5},
		{StartSeconds: 95, DurationSeconds: 5, Roles: []string{SamplingRoleHighComplexity}},
	}}
	selected := selectDeepIVTCAdditionalWindows(analysis, plan, 1)
	if len(selected) != 1 || selected[0].Start != 10 || selected[0].SelectionBasis != "existing_temporal_cadence_evidence" {
		t.Fatalf("deduplicated candidate did not retain its stronger cadence-evidence priority: %#v", selected)
	}
}

func TestJob2191HistoricalRatiosRemainReviewRequiredWithTwoInconclusiveWindows(t *testing.T) {
	scan := job2191AmbiguousIVTCScanResult(t, "/synthetic/Sin Limite_t00.mkv")
	analysis, ok := decodeInterlaceAnalysis(scan.InterlaceAnalysis)
	if !ok || analysis.IVTCValidation == nil {
		t.Fatal("could not decode the historical Job #2191 fixture")
	}
	validation := analysis.IVTCValidation
	validation.DeepAnalysisVersion = ivtcDeepEvidenceVersion
	validation.DeepAnalysisStatus = "review_required"
	validation.BaselineWindowCount = 3
	validation.AttemptedWindows = 5
	validation.Windows = append(validation.Windows,
		IVTCWindowValidation{Start: 1687.509, TFFProgressive: 49, TFFClassified: 123, TFFResidualInterlaced: 74,
			BFFProgressive: 5, BFFClassified: 123, BFFResidualInterlaced: 118,
			TFFProgressiveRatio: 49.0 / 123, BFFProgressiveRatio: 5.0 / 123, Outcome: ivtcWindowOutcomeInconclusive},
		IVTCWindowValidation{Start: 4579.563, TFFProgressive: 18, TFFClassified: 122, TFFResidualInterlaced: 104,
			BFFProgressive: 4, BFFClassified: 122, BFFResidualInterlaced: 118,
			TFFProgressiveRatio: 18.0 / 122, BFFProgressiveRatio: 4.0 / 122, Outcome: ivtcWindowOutcomeInconclusive},
	)
	analysis.Status = "telecine_suspected"
	applyIVTCValidation(&analysis, validation)

	if validation.SelectedOrder != "" || validation.ValidWindows != 1 || validation.InconclusiveWindows != 4 || validation.ContradictoryWindows != 0 {
		t.Fatalf("Job #2191's one valid of five windows must remain unresolved: %#v", validation)
	}
	if ivtcWindowsAgree(validation, "tff") || deepIVTCWindowsSufficient(validation, "tff") {
		t.Fatalf("one validated historical window plus two inconclusive probes passed distribution gates: %#v", validation)
	}
	if analysis.Status != "telecine_suspected" || analysis.RecommendedFilter != "" || !strings.Contains(analysis.DecisionReason, "insufficient") {
		t.Fatalf("historical ratios did not produce an actionable insufficient-evidence review: %#v", analysis)
	}
}

func TestDeepIVTCAnalysisRemainsReviewRequiredWhenEvidenceIsInsufficient(t *testing.T) {
	analysis, plan := ambiguousDeepIVTCFixture()
	runner := func(_ context.Context, _ string, _ float64, _ int, _ string) (InterlaceAnalysis, bool) {
		return InterlaceAnalysis{}, false
	}
	if err := extendIVTCValidationContext(context.Background(), "/synthetic/telecine.mkv", &analysis, plan, &IVTCSourceIdentity{Path: "/synthetic/telecine.mkv", SnapshotID: 7}, runner); err != nil {
		t.Fatal(err)
	}
	if analysis.IVTCValidation.DeepAnalysisStatus != "review_required" || analysis.IVTCValidation.SelectedOrder != "" || analysis.RecommendedFilter != "" {
		t.Fatalf("insufficient evidence should remain fail-closed: analysis=%#v validation=%#v", analysis, analysis.IVTCValidation)
	}
	if analysis.IVTCValidation.AttemptedWindows != 5 || analysis.IVTCValidation.InconclusiveWindows != 5 {
		t.Fatalf("inconclusive windows were not preserved: %#v", analysis.IVTCValidation)
	}
}

func TestDeepIVTCAnalysisRejectsContradictoryWindowOrders(t *testing.T) {
	analysis, plan := ambiguousDeepIVTCFixture()
	runner := func(_ context.Context, _ string, start float64, _ int, filter string) (InterlaceAnalysis, bool) {
		if strings.Contains(filter, "showinfo") {
			return syntheticCanonicalIVTCOutput(0), true
		}
		if start == 60 {
			if strings.Contains(filter, "order=tff") {
				return InterlaceAnalysis{Progressive: 50, TFF: 25, BFF: 25}, true
			}
			return InterlaceAnalysis{Progressive: 95, TFF: 3, BFF: 2}, true
		}
		if strings.Contains(filter, "order=tff") {
			return InterlaceAnalysis{Progressive: 95, TFF: 3, BFF: 2}, true
		}
		return InterlaceAnalysis{Progressive: 50, TFF: 25, BFF: 25}, true
	}
	if err := extendIVTCValidationContext(context.Background(), "/synthetic/telecine.mkv", &analysis, plan, &IVTCSourceIdentity{Path: "/synthetic/telecine.mkv", SnapshotID: 7}, runner); err != nil {
		t.Fatal(err)
	}
	if analysis.IVTCValidation.DeepAnalysisStatus != "review_required" || analysis.IVTCValidation.SelectedOrder != "" || analysis.IVTCValidation.ValidWindows != 0 || analysis.IVTCValidation.ContradictoryWindows == 0 {
		t.Fatalf("conflicting order evidence should remain review-required: %#v", analysis.IVTCValidation)
	}
	if !strings.Contains(analysis.DecisionReason, "mixed") {
		t.Fatalf("conflicting windows were not distinguished from insufficient evidence: %q", analysis.DecisionReason)
	}
}

func TestDeepIVTCAnalysisCancellationDoesNotPersistPartialEvidence(t *testing.T) {
	analysis, plan := ambiguousDeepIVTCFixture()
	original := *analysis.IVTCValidation
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	runner := func(_ context.Context, _ string, _ float64, _ int, _ string) (InterlaceAnalysis, bool) {
		calls++
		cancel()
		return InterlaceAnalysis{Progressive: 95, TFF: 3, BFF: 2}, true
	}
	err := extendIVTCValidationContext(ctx, "/synthetic/telecine.mkv", &analysis, plan, &IVTCSourceIdentity{Path: "/synthetic/telecine.mkv", SnapshotID: 7}, runner)
	if err == nil || calls != 1 {
		t.Fatalf("cancel should stop after the active FFmpeg comparison: err=%v calls=%d", err, calls)
	}
	if analysis.IVTCValidation.DeepAnalysisVersion != original.DeepAnalysisVersion || len(analysis.IVTCValidation.Windows) != len(original.Windows) {
		t.Fatalf("cancelled analysis exposed partial evidence: %#v", analysis.IVTCValidation)
	}
}

func TestStaleDeepIVTCEvidenceResetsToTheReusableBaseline(t *testing.T) {
	analysis, plan := ambiguousDeepIVTCFixture()
	runner := func(_ context.Context, _ string, _ float64, _ int, filter string) (InterlaceAnalysis, bool) {
		if strings.Contains(filter, "showinfo") {
			return syntheticCanonicalIVTCOutput(0), true
		}
		if strings.Contains(filter, "order=tff") {
			return InterlaceAnalysis{Progressive: 95, TFF: 3, BFF: 2}, true
		}
		return InterlaceAnalysis{Progressive: 50, TFF: 25, BFF: 25}, true
	}
	if err := extendIVTCValidationContext(context.Background(), "/synthetic/telecine.mkv", &analysis, plan, &IVTCSourceIdentity{Path: "/synthetic/telecine.mkv", SnapshotID: 7}, runner); err != nil {
		t.Fatal(err)
	}
	if !resetStaleDeepIVTCEvidence(&analysis) {
		t.Fatal("expected stale deep evidence to be safely reset")
	}
	if len(analysis.IVTCValidation.Windows) != 3 || analysis.IVTCValidation.DeepAnalysisVersion != 0 || analysis.IVTCValidation.BaselineWindowCount != 0 {
		t.Fatalf("stale refresh should retain exactly the reusable base windows: %#v", analysis.IVTCValidation)
	}
	if analysis.IVTCValidation.SelectedOrder != "" || analysis.RecommendedFilter != "" || analysis.AutomaticFilter != "" || analysis.IVTCValidation.TFFProgressive != 90 || analysis.IVTCValidation.TFFClassified != 100 {
		t.Fatalf("stale deep order or counters survived reset: analysis=%#v validation=%#v", analysis, analysis.IVTCValidation)
	}
	if err := extendIVTCValidationContext(context.Background(), "/synthetic/telecine.mkv", &analysis, plan, &IVTCSourceIdentity{Path: "/synthetic/telecine.mkv", SnapshotID: 8}, runner); err != nil {
		t.Fatalf("fresh deep analysis could not reuse the original three windows: %v", err)
	}
	if analysis.IVTCValidation.DeepAnalysisStatus != "resolved" || analysis.IVTCValidation.SelectedOrder != "tff" || analysis.IVTCValidation.SourceIdentity.SnapshotID != 8 {
		t.Fatalf("refreshed deep evidence was not bound to the new snapshot identity: %#v", analysis.IVTCValidation)
	}
}

func TestIVTCComparisonMeasuresResidualCombingWithIDETForBothOrders(t *testing.T) {
	var filters []string
	runner := func(_ context.Context, _ string, _ float64, _ int, filter string) (InterlaceAnalysis, bool) {
		filters = append(filters, filter)
		if strings.Contains(filter, "showinfo") {
			return syntheticCanonicalIVTCOutput(4), true
		}
		return InterlaceAnalysis{Progressive: 96, TFF: 2, BFF: 2}, true
	}
	window, err := compareIVTCWindow(context.Background(), "/synthetic/telecine.mkv", 20, 5, runner)
	if err != nil {
		t.Fatal(err)
	}
	if !equalStringSlices(filters, []string{
		"fieldmatch=order=tff,decimate,idet", "fieldmatch=order=bff,decimate,idet",
		canonicalIVTCDiagnosticFilter("tff"), canonicalIVTCDiagnosticFilter("bff"),
	}) {
		t.Fatalf("TFF/BFF comparison must retain legacy order scores and also measure the canonical chain: %#v", filters)
	}
	if window.TFFClassified != 100 || window.BFFClassified != 100 || window.TFFProgressiveRatio != .96 || window.BFFProgressiveRatio != .96 || window.TFFResidualInterlaced != 4 || window.BFFResidualInterlaced != 4 || !window.DiagnosticsAvailable {
		t.Fatalf("window evidence omitted canonical pre-fallback residual counts or output timing: %#v", window)
	}
}

func TestParseIDETOutputCapturesPreFallbackResidualsAndTimestampGaps(t *testing.T) {
	output := `[Parsed_fieldmatch_0 @ 0x1] Frame #2 at 0.0667333 is still interlaced
[Parsed_idet_1 @ 0x2] Repeated Fields: Neither: 7 Top: 2 Bottom: 1
[Parsed_idet_1 @ 0x2] Multi frame detection: TFF: 1 BFF: 0 Progressive: 7 Undetermined: 0
[Parsed_showinfo_4 @ 0x3] n: 0 pts: 0 pts_time:0 duration: 1 duration_time:0.0417083 fmt:yuv420p cl:unspecified sar:1/1 s:160x120 i:P
[Parsed_showinfo_4 @ 0x3] n: 1 pts: 1 pts_time:0.0417083 duration: 1 duration_time:0.0417083 fmt:yuv420p cl:unspecified sar:1/1 s:160x120 i:P
[Parsed_showinfo_4 @ 0x3] n: 2 pts: 3 pts_time:0.125125 duration: 1 duration_time:0.0417083 fmt:yuv420p cl:unspecified sar:1/1 s:160x120 i:P`
	analysis, ok := parseIDETOutput(output)
	if !ok {
		t.Fatal("expected the FFmpeg idet report to parse")
	}
	if analysis.ResidualInterlacedFrames != 1 || analysis.RepeatedTop != 2 || analysis.RepeatedBottom != 1 {
		t.Fatalf("pre-fallback combing/cadence metrics were lost: %#v", analysis)
	}
	if analysis.CanonicalOutputFrames != 3 || analysis.CanonicalOutputProgressive != 3 || analysis.CanonicalTimestampAnomalies != 1 || analysis.CanonicalOutputFrameRate <= 0 {
		t.Fatalf("canonical output timing metrics did not flag the irregular PTS gap: %#v", analysis)
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

func TestAdaptiveInterlaceSamplingPlanUsesQuickValidationOnlyForStrongProgressiveEvidence(t *testing.T) {
	plan := canonicalSamplingPlan(1200, defaultFrameStructureSamplingPolicy())
	stable := QSVFrameStructureAnalysis{ConfidenceScore: 0.995, WindowCount: 3, Windows: []FrameStructureWindow{
		{Position: 0.08, FrameSignals: FrameSignalSummary{DecodedFrames: 240, ProgressiveFrames: 240, EffectiveFPS: 24}},
		{Position: 0.50, FrameSignals: FrameSignalSummary{DecodedFrames: 240, ProgressiveFrames: 240, EffectiveFPS: 24}},
		{Position: 0.92, FrameSignals: FrameSignalSummary{DecodedFrames: 240, ProgressiveFrames: 240, EffectiveFPS: 24}},
	}}
	quick, depth, reason := adaptiveInterlaceSamplingPlan(plan, stable, "progressive")
	if depth != interlaceAnalysisDepthQuick || len(quick.Positions) != 2 || quick.Positions[0] != plan.Positions[0] || quick.Positions[1] != plan.Positions[len(plan.Positions)-1] || reason == "" {
		t.Fatalf("strong progressive evidence did not select representative quick IDET: depth=%s reason=%q plan=%#v", depth, reason, quick)
	}

	tests := []struct {
		name   string
		mutate func(*QSVFrameStructureAnalysis)
		field  string
	}{
		{name: "repeat pict", mutate: func(value *QSVFrameStructureAnalysis) { value.Windows[1].FrameSignals.RepeatPictFrames = 1 }},
		{name: "interlaced frame", mutate: func(value *QSVFrameStructureAnalysis) {
			value.Windows[1].FrameSignals.InterlacedFrames = 1
			value.Windows[1].FrameSignals.ProgressiveFrames = 239
		}},
		{name: "unstable fps", mutate: func(value *QSVFrameStructureAnalysis) { value.Windows[1].FrameSignals.EffectiveFPS = 29.97 }},
		{name: "low confidence", mutate: func(value *QSVFrameStructureAnalysis) { value.ConfidenceScore = 0.80 }},
		{name: "field order evidence", field: "tt", mutate: func(*QSVFrameStructureAnalysis) {}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := stable
			candidate.Windows = append([]FrameStructureWindow(nil), stable.Windows...)
			test.mutate(&candidate)
			deep, depth, reason := adaptiveInterlaceSamplingPlan(plan, candidate, test.field)
			if depth != interlaceAnalysisDepthDeep || len(deep.Positions) != len(plan.Positions) || reason == "" {
				t.Fatalf("suspicious evidence did not retain deep IDET: depth=%s reason=%q plan=%#v", depth, reason, deep)
			}
		})
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
