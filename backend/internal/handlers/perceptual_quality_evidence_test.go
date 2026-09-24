package handlers

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func testPerceptualPlan() SamplingPlan {
	return SamplingPlan{
		PlanIdentity: "plan-1", SourceRevision: "source-1", PolicyVersion: "sampling-v1",
		SelectedWindows: []SamplingWindow{
			{Identity: "window-primary", Primary: true, StartSeconds: 10, DurationSeconds: 5, Roles: []string{"primary"}},
			{Identity: "window-hard", StartSeconds: 50, DurationSeconds: 5, Roles: []string{"hard_scene"}, Reasons: []string{"motion"}},
		},
	}
}

func TestPreviewMetricParserPreservesPSNRInfinityAndRejectsNaN(t *testing.T) {
	runner := func(_ context.Context, args []string) (string, error) {
		if strings.Contains(strings.Join(args, " "), "psnr") {
			return "[Parsed_psnr_0] PSNR y:inf average:inf min:inf max:inf", nil
		}
		return "[Parsed_ssim_0] SSIM All:0.987", nil
	}
	infinity, err := runPreviewFrameMetricValueWithRunner(context.Background(), "source", "0", "format=yuv420p", "candidate", "format=yuv420p", "psnr", runner)
	if err != nil || !infinity.IsPositiveInfinity() {
		t.Fatalf("PSNR infinity was not preserved: %#v %v", infinity, err)
	}
	finite, err := runPreviewFrameMetricValueWithRunner(context.Background(), "source", "0", "format=yuv420p", "candidate", "format=yuv420p", "ssim", runner)
	if err != nil || !finite.Valid() || finite.IsPositiveInfinity() {
		t.Fatalf("finite SSIM was not parsed: %#v %v", finite, err)
	}
	nanRunner := func(context.Context, []string) (string, error) { return "[Parsed_psnr_0] PSNR average:nan", nil }
	if _, err := runPreviewFrameMetricValueWithRunner(context.Background(), "source", "0", "", "candidate", "", "psnr", nanRunner); err == nil {
		t.Fatal("NaN must be failed measurement, not a successful value")
	}
}

func TestLogicalMetricProviderParserUsesExplicitProviderContract(t *testing.T) {
	runner := func(_ context.Context, args []string) (string, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "xpsnr"):
			return "XPSNR_avg:40.25", nil
		case strings.Contains(joined, "ms_ssim"):
			return "ms_ssim:0.95", nil
		default:
			return "VMAF score: 92.5", nil
		}
	}
	for _, test := range []struct {
		metric PerceptualMetric
		output string
	}{
		{PerceptualMetricVMAF, "VMAF score"},
		{PerceptualMetricXPSNR, "xpsnr"},
		{PerceptualMetricMSSSIM, "ms_ssim"},
	} {
		capability := MetricCapability{Metric: test.metric, Provider: "verified-provider", Availability: "available"}
		value, err := runPreviewLogicalMetricValueAtWithRunner(context.Background(), "source", "0", "", "candidate", "0", "", capability, runner)
		if err != nil || !value.Valid() {
			t.Fatalf("provider %s did not produce a tagged value: %#v %v", test.metric, value, err)
		}
	}
}

func TestCanonicalWindowMetricUsesBoundedComparisonAndValidBranches(t *testing.T) {
	var captured []string
	runner := func(_ context.Context, args []string) (string, error) {
		captured = append([]string(nil), args...)
		return "VMAF score: 92.5", nil
	}
	value, err := runCanonicalWindowLogicalMetricValueWithRunner(context.Background(), "source", "600", "setsar=1,format=yuv420p", "candidate", "0", "setsar=1,format=yuv420p", 20, MetricCapability{Metric: PerceptualMetricVMAF, Provider: "libvmaf", Availability: "available"}, runner)
	if err != nil || !value.Valid() {
		t.Fatalf("bounded VMAF failed: %#v %v", value, err)
	}
	joined := strings.Join(captured, " ")
	if !strings.Contains(joined, "-t 20.000") || strings.Contains(joined, "-frames:v 1") {
		t.Fatalf("canonical metric args were not bounded independently of legacy frame mode: %s", joined)
	}
	if !strings.Contains(joined, "[0:v:0]setsar=1,format=yuv420p[reference];[1:v:0]setsar=1,format=yuv420p[candidate]") {
		t.Fatalf("canonical comparison graph did not contain explicit reference/candidate branches: %s", joined)
	}
	for _, tc := range []struct {
		metric PerceptualMetric
		output string
	}{
		{PerceptualMetricXPSNR, "XPSNR_avg:40.25"},
		{PerceptualMetricMSSSIM, "ms_ssim:0.95"},
	} {
		var providerArgs []string
		providerValue, providerErr := runCanonicalWindowLogicalMetricValueWithRunner(context.Background(), "source", "600", "null", "candidate", "0", "null", 20, MetricCapability{Metric: tc.metric, Provider: "verified-provider", Availability: "available"}, func(_ context.Context, args []string) (string, error) {
			providerArgs = args
			return tc.output, nil
		})
		if providerErr != nil || !providerValue.Valid() {
			t.Fatalf("bounded %s provider failed: %#v %v", tc.metric, providerValue, providerErr)
		}
		providerCommand := strings.Join(providerArgs, " ")
		if !strings.Contains(providerCommand, "-t 20.000") || strings.Contains(providerCommand, "-frames:v 1") {
			t.Fatalf("bounded %s provider escaped the canonical window: %s", tc.metric, providerCommand)
		}
	}
}

func TestCanonicalComparisonDomainAddsFiltersForPreserveProfileWithoutUserFilters(t *testing.T) {
	source, candidate := previewVideoCharacteristics{Width: 1920, Height: 1080, SampleAspectRatio: "1:1"}, previewVideoCharacteristics{Width: 1920, Height: 1080, SampleAspectRatio: "1:1"}
	reference, output := canonicalComparisonDomainFilters(source, candidate, "", "")
	if reference == "null" || output == "null" || !strings.Contains(reference, "format=yuv420p") || !strings.Contains(output, "format=yuv420p") {
		t.Fatalf("preserve comparison domain did not create canonical no-op branches: %q / %q", reference, output)
	}
	args := canonicalWindowMetricArgs("source", "0", reference, "candidate", "0", output, 5, "ssim")
	if strings.Contains(strings.Join(args, " "), "[0:v:0]null[reference]") {
		t.Fatal("canonical graph emitted an invalid null-only reference branch")
	}
}

func TestCanonicalMetricResultCacheSeparatesProviderIdentity(t *testing.T) {
	resetLabFidelityEvidenceForTest()
	t.Cleanup(resetLabFidelityEvidenceForTest)
	pair := testPair(t, "generation-cache")
	descriptor := newMetricDescriptor(PerceptualMetricVMAF, "libvmaf", "1", "bounded-v1", "model-a", "runtime-a", "higher_is_better")
	identity, err := ComputeMetricResultIdentity(pair, descriptor)
	if err != nil {
		t.Fatal(err)
	}
	value, _ := NewFiniteMetricValue(92.5)
	result := MetricResult{Identity: identity, Status: PerceptualMetricAvailable, Value: &value}
	processes := 0
	for attempt := 0; attempt < 2; attempt++ {
		if _, ok := labPreviewEvidenceCacheGetMetricResult(identity.Identity); ok {
			continue
		}
		processes++
		labPreviewEvidenceCachePutMetricResult(result)
	}
	if processes != 1 {
		t.Fatalf("exact metric-result retry reran the provider %d times", processes)
	}
	changed := descriptor
	changed.Provider = "libvmaf-alt"
	changedIdentity, err := ComputeMetricResultIdentity(pair, changed)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := labPreviewEvidenceCacheGetMetricResult(changedIdentity.Identity); ok {
		t.Fatal("provider change reused a result from another provider")
	}
}

func testPair(t *testing.T, candidateRevision string) ComparisonPairIdentity {
	t.Helper()
	reference, err := ComputeReferenceIdentity(ReferenceIdentity{
		SamplingPlanIdentity: "plan-1", SamplingWindowIdentity: "window-primary", SourceRevision: "source-1",
		SourceStreamIdentity: "0:v:0", TransformIdentity: "transform-1", TransformVersion: "frame-fidelity-v1",
		AlignmentIdentity: "alignment-1", ComparisonDomainIdentity: "domain-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := ComputeCandidateIdentity(CandidateIdentity{
		Kind: "compatible_preview", ArtifactID: "artifact-a", GenerationToken: candidateRevision,
		GenerationVersion: 1, OutputRevision: candidateRevision, SourceRevision: "source-1",
		StreamIdentity: "0:v:0", SegmentStartSeconds: 10, SegmentDurationSeconds: 5,
		TimelineIdentity: "timeline-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := ComputeCandidateCoverage(CandidateCoverage{
		SourceStartSeconds: 10, SourceDurationSeconds: 5, CoveredWindowIdentities: []string{"window-primary"},
		SegmentMappingIdentity: "segment-10-5", SourceTimelineIdentity: "source-timeline-1", CandidateTimelineIdentity: "candidate-timeline-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	pair, err := ComputeComparisonPairIdentity(ComparisonPairIdentity{
		SamplingPlanIdentity: "plan-1", SamplingWindowIdentity: "window-primary", Reference: reference,
		Candidate: candidate, CoverageIdentity: coverage.Identity, SegmentMappingIdentity: coverage.SegmentMappingIdentity,
		AlignmentIdentity: reference.AlignmentIdentity, ComparisonDomainIdentity: reference.ComparisonDomainIdentity,
		SourceRevision: reference.SourceRevision, CandidateRevision: candidate.OutputRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	return pair
}

func TestMetricValueJSONRoundTripAndNaNRejection(t *testing.T) {
	finite, err := NewFiniteMetricValue(42.5)
	if err != nil {
		t.Fatal(err)
	}
	infinity := NewPositiveInfinityMetricValue()
	payload, err := json.Marshal([]MetricValue{finite, infinity})
	if err != nil {
		t.Fatal(err)
	}
	var decoded []MetricValue
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 2 || !decoded[0].Valid() || !decoded[1].IsPositiveInfinity() {
		t.Fatalf("unexpected metric values after round trip: %#v", decoded)
	}
	if _, err := NewFiniteMetricValue(math.NaN()); err == nil {
		t.Fatal("NaN must not be a successful metric value")
	}
	if _, err := json.Marshal(MetricValue{Kind: MetricValueFinite}); err == nil {
		t.Fatal("missing finite value must be rejected")
	}
}

func TestMaterialPairAndMetricResultIdentityLayers(t *testing.T) {
	pair := testPair(t, "generation-a")
	vmaf := newMetricDescriptor(PerceptualMetricVMAF, "libvmaf", "1", "filter-v1", "vmaf_v0.6.1", "runtime-a", "higher_is_better")
	ssim := newMetricDescriptor(PerceptualMetricSSIM, "ffmpeg.filter.ssim", "1", "filter-v1", "", "runtime-a", "higher_is_better")
	vmafResult, err := ComputeMetricResultIdentity(pair, vmaf)
	if err != nil {
		t.Fatal(err)
	}
	ssimResult, err := ComputeMetricResultIdentity(pair, ssim)
	if err != nil {
		t.Fatal(err)
	}
	if vmafResult.PairIdentity != pair.Identity || ssimResult.PairIdentity != pair.Identity || vmafResult.Identity == ssimResult.Identity {
		t.Fatalf("metric results did not retain one pair with distinct result identities: %#v %#v", vmafResult, ssimResult)
	}
	providerChange := vmaf
	providerChange.Provider = "libvmaf-alt"
	changed, err := ComputeMetricResultIdentity(pair, providerChange)
	if err != nil {
		t.Fatal(err)
	}
	if changed.PairIdentity != pair.Identity || changed.Identity == vmafResult.Identity {
		t.Fatal("provider change must change only the metric result identity")
	}
	if testPair(t, "generation-b").Identity == pair.Identity {
		t.Fatal("candidate generation change must change the material pair")
	}
}

func TestMetricMembershipChangesSetOnlyNotMaterialPair(t *testing.T) {
	plan := testPerceptualPlan()
	pair := testPair(t, "generation-a")
	psnr := newMetricDescriptor(PerceptualMetricPSNR, "ffmpeg.filter.psnr", "1", "filter-v1", "", "runtime-a", "higher_is_better")
	ssim := newMetricDescriptor(PerceptualMetricSSIM, "ffmpeg.filter.ssim", "1", "filter-v1", "", "runtime-a", "higher_is_better")
	resultIdentity, _ := ComputeMetricResultIdentity(pair, psnr)
	value, _ := NewFiniteMetricValue(35)
	window := PerceptualWindowEvidence{WindowIdentity: "window-primary", Primary: true, Status: "covered", PairIdentity: &pair, Metrics: []MetricResult{{Identity: resultIdentity, Status: PerceptualMetricAvailable, Value: &value}}}
	first, err := BuildPerceptualEvidenceSet(plan, "source-1", "runtime-a", []MetricDescriptor{psnr}, []PerceptualWindowEvidence{window}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildPerceptualEvidenceSet(plan, "source-1", "runtime-a", []MetricDescriptor{psnr, ssim}, []PerceptualWindowEvidence{window}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Identity == second.Identity || first.Windows[0].PairIdentity.Identity != second.Windows[0].PairIdentity.Identity {
		t.Fatal("requested metric membership must change the set, not the material pair")
	}
}

func TestFailedMetricDoesNotEraseSuccessfulMetrics(t *testing.T) {
	pair := testPair(t, "generation-a")
	vmaf := newMetricDescriptor(PerceptualMetricVMAF, "libvmaf", "1", "filter-v1", "model-a", "runtime-a", "higher_is_better")
	ssim := newMetricDescriptor(PerceptualMetricSSIM, "ffmpeg.filter.ssim", "1", "filter-v1", "", "runtime-a", "higher_is_better")
	vmafIdentity, _ := ComputeMetricResultIdentity(pair, vmaf)
	ssimIdentity, _ := ComputeMetricResultIdentity(pair, ssim)
	value, _ := NewFiniteMetricValue(.99)
	aggregates := AggregatePerceptualMetrics([]PerceptualWindowEvidence{{WindowIdentity: "window-primary", Status: "covered", PairIdentity: &pair, Metrics: []MetricResult{
		{Identity: vmafIdentity, Status: PerceptualMetricFailedMeasurement, Reason: "provider failed"},
		{Identity: ssimIdentity, Status: PerceptualMetricAvailable, Value: &value},
	}}})
	ssimMeasured := false
	for _, aggregate := range aggregates {
		if aggregate.LogicalMetric == PerceptualMetricSSIM && aggregate.MeasuredComparableWindows == 1 {
			ssimMeasured = true
		}
	}
	if len(aggregates) != 2 || !ssimMeasured {
		t.Fatalf("metric failure was not isolated: %#v", aggregates)
	}
}

func TestEvidenceSetUsesFrozenOrderAndNotCoveredLedger(t *testing.T) {
	plan := testPerceptualPlan()
	pair := testPair(t, "generation-a")
	metric := newMetricDescriptor(PerceptualMetricPSNR, "ffmpeg.filter.psnr", "1", "filter-v1", "", "runtime-a", "higher_is_better")
	resultIdentity, err := ComputeMetricResultIdentity(pair, metric)
	if err != nil {
		t.Fatal(err)
	}
	value, _ := NewFiniteMetricValue(40)
	windows := []PerceptualWindowEvidence{
		{WindowIdentity: "window-hard", StartSeconds: 50, DurationSeconds: 5, Status: "not_covered"},
		{WindowIdentity: "window-primary", StartSeconds: 10, DurationSeconds: 5, Primary: true, Status: "covered", PairIdentity: &pair,
			Metrics: []MetricResult{{Identity: resultIdentity, Status: PerceptualMetricAvailable, Value: &value}}},
	}
	set, err := BuildPerceptualEvidenceSet(plan, "source-1", "runtime-a", []MetricDescriptor{metric}, windows, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Windows) != 2 || set.Windows[0].WindowIdentity != "window-primary" || set.Windows[1].Status != "not_covered" {
		t.Fatalf("frozen order/not-covered ledger lost: %#v", set.Windows)
	}
	if set.Windows[1].PairIdentity != nil || set.Windows[1].Candidate != nil {
		t.Fatal("not-covered window must not receive a fake material pair")
	}
	setReordered, err := BuildPerceptualEvidenceSet(plan, "source-1", "runtime-a", []MetricDescriptor{metric}, []PerceptualWindowEvidence{windows[1], windows[0]}, nil, nil, nil)
	if err != nil || setReordered.Identity != set.Identity {
		t.Fatalf("map/order changes must not change set identity: %v %s %s", err, set.Identity, setReordered.Identity)
	}
}

func TestAggregatePSNRPositiveInfinityAndWorstIdentity(t *testing.T) {
	plan := testPerceptualPlan()
	pairA := testPair(t, "generation-a")
	pairB := testPair(t, "generation-b")
	metric := newMetricDescriptor(PerceptualMetricPSNR, "ffmpeg.filter.psnr", "1", "filter-v1", "", "runtime-a", "higher_is_better")
	resultA, _ := ComputeMetricResultIdentity(pairA, metric)
	resultB, _ := ComputeMetricResultIdentity(pairB, metric)
	finite, _ := NewFiniteMetricValue(30)
	infinity := NewPositiveInfinityMetricValue()
	windows := []PerceptualWindowEvidence{
		{WindowIdentity: "window-primary", Status: "covered", PairIdentity: &pairA, Metrics: []MetricResult{{Identity: resultA, Status: PerceptualMetricAvailable, Value: &finite}}},
		{WindowIdentity: "window-hard", Status: "covered", PairIdentity: &pairB, Metrics: []MetricResult{{Identity: resultB, Status: PerceptualMetricAvailable, Value: &infinity}}},
	}
	aggregates := AggregatePerceptualMetrics(windows)
	if len(aggregates) != 1 || aggregates[0].WorstWindowIdentity != "window-primary" || aggregates[0].WorstPairIdentity != pairA.Identity {
		t.Fatalf("worst identity was not traceable: %#v", aggregates)
	}
	if aggregates[0].Median == nil || !aggregates[0].Median.IsPositiveInfinity() || aggregates[0].LowerPercentileP10 == nil || aggregates[0].LowerPercentileP10.IsPositiveInfinity() {
		t.Fatalf("infinity aggregation was not deterministic: %#v", aggregates[0])
	}
	if aggregates[0].TotalFrozenWindows != len(plan.SelectedWindows) {
		t.Fatalf("aggregate must retain frozen window count: %#v", aggregates[0])
	}
}
