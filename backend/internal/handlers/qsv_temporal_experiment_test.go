package handlers

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/capabilities"
	"github.com/anuelvs/mvforge/backend/internal/models"
)

func qsvTemporalFixturePlan() (SamplingWindow, SamplingPlan) {
	primary := SamplingWindow{StartSeconds: 0, DurationSeconds: 20, Primary: true}
	plan := SamplingPlan{
		PlanIdentity:         "sampling-plan:fixture",
		PolicyVersion:        "sampling-policy-fixture",
		SourceRevision:       "source:fixture",
		SourceFingerprint:    "fingerprint:fixture",
		SourceStreamIdentity: "stream:0",
		SelectedWindows: []SamplingWindow{
			{Position: .2, StartSeconds: 30, DurationSeconds: 20, Identity: "window:motion", Roles: []string{SamplingRoleHighMotion}, Reasons: []string{"motion fixture"}},
			{Position: .5, StartSeconds: 60, DurationSeconds: 20, Identity: "window:texture", Roles: []string{SamplingRoleHighTextureOrGrain}, Reasons: []string{"texture fixture"}},
			{Position: .8, StartSeconds: 90, DurationSeconds: 20, Identity: "window:worst", Roles: []string{SamplingRoleWorstPredicted}, Reasons: []string{"worst fixture"}},
		},
	}
	return primary, plan
}

func TestQSVTemporalTargetsForFrozenPlanUsesOnlyCanonicalRoles(t *testing.T) {
	primary, plan := qsvTemporalFixturePlan()
	experiment, err := QSVTemporalTargetsForFrozenPlan(primary, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(experiment.Targets) != 3 {
		t.Fatalf("targets=%d, want 3 bounded targets", len(experiment.Targets))
	}
	if !experiment.Targets[0].Primary {
		t.Fatal("explicit LAB primary must remain first")
	}
	if experiment.Targets[0].WindowIdentity == experiment.Targets[1].WindowIdentity || experiment.Targets[1].WindowIdentity == experiment.Targets[2].WindowIdentity {
		t.Fatalf("target identities are not distinct: %#v", experiment.Targets)
	}
	if experiment.UnavailableRoles != nil {
		t.Fatalf("unexpected unavailable roles: %#v", experiment.UnavailableRoles)
	}

	plan.SelectedWindows = plan.SelectedWindows[:1]
	experiment, err = QSVTemporalTargetsForFrozenPlan(primary, plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(experiment.Targets) != 2 || len(experiment.UnavailableRoles) != 2 {
		t.Fatalf("missing roles were not retained explicitly: %#v", experiment)
	}
}

func TestQSVTemporalExperimentIdentityIncludesEachTargetGeneration(t *testing.T) {
	primary, plan := qsvTemporalFixturePlan()
	experiment, err := QSVTemporalTargetsForFrozenPlan(primary, plan)
	if err != nil {
		t.Fatal(err)
	}
	for index := range experiment.Targets {
		experiment.Targets[index].Baseline = QSVExperimentCandidate{CandidateIdentity: "baseline-" + string(rune('a'+index)), SamplingWindowIdentity: experiment.Targets[index].WindowIdentity, GenerationToken: "generation", GenerationVersion: int64(index + 1)}
	}
	experiment.ExperimentID = ComputeQSVTemporalExperimentID(experiment)
	if experiment.ExperimentID != ComputeQSVTemporalExperimentID(experiment) {
		t.Fatal("experiment identity is not deterministic")
	}
	changed := experiment
	changed.Targets = append([]QSVTemporalTarget(nil), experiment.Targets...)
	changed.Targets[1].Baseline.GenerationVersion++
	if experiment.ExperimentID == ComputeQSVTemporalExperimentID(changed) {
		t.Fatal("target baseline generation did not change experiment identity")
	}
}

func TestQSVTemporalExperimentBoundsAreCompleteAndDeterministic(t *testing.T) {
	primary, plan := qsvTemporalFixturePlan()
	experiment, err := QSVTemporalTargetsForFrozenPlan(primary, plan)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 7; index++ {
		experiment.Targets[0].Variants = append(experiment.Targets[0].Variants, QSVTemporalVariant{VariantIdentity: "v", ChangedVariable: QSVTemporalChangedQuality})
	}
	if err := ValidateQSVTemporalExperimentBounds(experiment); err == nil {
		t.Fatal("total variant cap accepted seven variants")
	}
	experiment.Targets[0].Variants = experiment.Targets[0].Variants[:0]
	request := QSVTemporalExperimentRequest{VariantsByWindow: []QSVTemporalTargetRequest{{WindowIdentity: experiment.Targets[0].WindowIdentity, Variants: []QSVTemporalVariantRequest{{ChangedVariable: QSVTemporalChangedQuality, RequestedDelta: models.JSONMap{"globalQuality": 24}}, {ChangedVariable: QSVTemporalChangedGOP, RequestedDelta: models.JSONMap{"gopFrames": 48}}}}}}
	if err := experiment.ApplyRequestedVariants(request); err != nil {
		t.Fatal(err)
	}
	if len(experiment.Targets[0].Variants) != 2 || len(experiment.Targets[1].Variants) != 0 {
		t.Fatalf("variants escaped their target: %#v", experiment.Targets)
	}
}

func TestQSVEffectiveDeltaAllowsOnlyDeclaredVariable(t *testing.T) {
	baseline := QSVExperimentCandidate{EffectiveQSVSettings: models.JSONMap{"qsvRateControl": "icq", "globalQuality": 25, "gopFrames": 60, "maxBFrames": 3, "filterPlan": "frozen"}}
	quality := QSVExperimentCandidate{EffectiveQSVSettings: models.JSONMap{"qsvRateControl": "icq", "globalQuality": 22, "gopFrames": 60, "maxBFrames": 3, "filterPlan": "frozen"}}
	result := ValidateQSVEffectiveDelta(baseline, quality, QSVTemporalChangedQuality, nil)
	if result.Result != QSVIsolationValid {
		t.Fatalf("quality-only delta=%#v", result)
	}
	gop := QSVExperimentCandidate{EffectiveQSVSettings: models.JSONMap{"qsvRateControl": "icq", "globalQuality": 25, "gopFrames": 48, "maxBFrames": 3, "filterPlan": "frozen"}}
	if got := ValidateQSVEffectiveDelta(baseline, gop, QSVTemporalChangedGOP, nil); got.Result != QSVIsolationValid {
		t.Fatalf("GOP-only delta=%#v", got)
	}
	bf := QSVExperimentCandidate{EffectiveQSVSettings: models.JSONMap{"qsvRateControl": "icq", "globalQuality": 25, "gopFrames": 60, "maxBFrames": 1, "filterPlan": "frozen"}}
	if got := ValidateQSVEffectiveDelta(baseline, bf, QSVTemporalChangedBFrames, nil); got.Result != QSVIsolationValid {
		t.Fatalf("BF-only delta=%#v", got)
	}
	unexpected := QSVExperimentCandidate{EffectiveQSVSettings: models.JSONMap{"qsvRateControl": "icq", "globalQuality": 22, "gopFrames": 60, "maxBFrames": 3, "filterPlan": "changed"}}
	if got := ValidateQSVEffectiveDelta(baseline, unexpected, QSVTemporalChangedQuality, nil); got.Result != QSVIsolationInvalid || len(got.UnexpectedFields) == 0 {
		t.Fatalf("second effective delta was not invalid: %#v", got)
	}
}

func TestQSVTemporalFeatureCapabilityDoesNotInventUnsupportedControls(t *testing.T) {
	supported := capabilities.EncoderCapability{Usable: true, LookAhead: true, ExtendedBRC: true, AdaptiveI: true, AdaptiveB: true, TestedModes: map[string]bool{"qsvMBBRCVBR": true}}
	if !QSVTemporalFeatureCapability(supported, "lookahead").Supported || !QSVTemporalFeatureCapability(supported, "mbbrc").Supported {
		t.Fatal("reported QSV capabilities were not admitted")
	}
	unsupported := QSVTemporalFeatureCapability(capabilities.EncoderCapability{Usable: true}, "lookahead")
	if unsupported.Supported || unsupported.Reason == "" {
		t.Fatalf("unsupported feature was invented: %#v", unsupported)
	}
}

func TestDeterministicQSVTemporalEvidenceLossRecoveryAndNoSignal(t *testing.T) {
	provider := DeterministicQSVTemporalEvidenceProvider{Policy: DefaultQSVTemporalInterpretationPolicyV1()}
	base := []QSVTemporalFrameSample{{Comparable: true, DetailEnergy: 1, GrainEnergy: 1, HighFrequencyEnergy: 1, LocalVariance: 1}, {Comparable: true, DetailEnergy: 1, GrainEnergy: 1, HighFrequencyEnergy: 1, LocalVariance: 1}, {Comparable: true, DetailEnergy: 1, GrainEnergy: 1, HighFrequencyEnergy: 1, LocalVariance: 1}}
	variant := []QSVTemporalFrameSample{{Comparable: true, DetailEnergy: 1, GrainEnergy: 1, HighFrequencyEnergy: 1, LocalVariance: 1}, {Comparable: true, DetailEnergy: .6, GrainEnergy: .6, HighFrequencyEnergy: .6, LocalVariance: .6}, {Comparable: true, DetailEnergy: 1.2, GrainEnergy: 1.2, HighFrequencyEnergy: 1.2, LocalVariance: 1.2}}
	evidence, err := provider.Measure(context.Background(), QSVTemporalEvidenceRequest{TargetWindowIdentity: "window:texture", VariantIdentity: "variant:quality", BaselineFrames: base, VariantFrames: variant})
	if err != nil || evidence.Interpretation != QSVTemporalInterpretationLikelyPumping || len(evidence.LossRecoveryEvents) != 1 {
		t.Fatalf("loss/recovery evidence=%#v err=%v", evidence, err)
	}
	noSignal, err := provider.Measure(context.Background(), QSVTemporalEvidenceRequest{BaselineFrames: base, VariantFrames: base})
	if err != nil || noSignal.Interpretation != QSVTemporalInterpretationNotObserved {
		t.Fatalf("no-signal evidence=%#v err=%v", noSignal, err)
	}
	short, err := provider.Measure(context.Background(), QSVTemporalEvidenceRequest{BaselineFrames: base[:2], VariantFrames: variant[:2]})
	if err != nil || short.Interpretation != QSVTemporalInterpretationInconclusive {
		t.Fatalf("insufficient evidence=%#v err=%v", short, err)
	}
}

type qsvTemporalFakePerceptualProvider struct {
	ref *QSVPerceptualEvidenceRef
	err error
}

func (p qsvTemporalFakePerceptualProvider) Lookup(context.Context, QSVPerceptualEvidenceLookup) (*QSVPerceptualEvidenceRef, error) {
	return p.ref, p.err
}

func TestQSVPerceptualEvidenceIsOptionalAndIdentityBound(t *testing.T) {
	target := QSVTemporalTarget{WindowIdentity: "window:texture"}
	variant := QSVTemporalVariant{VariantIdentity: "variant:quality"}
	lookup := QSVPerceptualEvidenceLookup{SourceRevision: "source", SamplingPlanIdentity: "plan", WindowIdentity: "window:texture", ArtifactGeneration: "artifact:1", EvidenceVersion: "69-v1"}
	ref := &QSVPerceptualEvidenceRef{EvidenceID: "69:1", Version: "69-v1", SourceRevision: "source", PlanIdentity: "plan", WindowIdentity: "window:texture", ArtifactGeneration: "artifact:1"}
	if state, err := AttachQSVPerceptualEvidence(context.Background(), &target, &variant, qsvTemporalFakePerceptualProvider{ref: ref}, lookup); err != nil || state != "attached" || len(variant.PerceptualEvidence) != 1 {
		t.Fatalf("matching #69 evidence state=%s err=%v variant=%#v", state, err, variant)
	}
	variant = QSVTemporalVariant{VariantIdentity: "variant:quality"}
	ref.WindowIdentity = "window:other"
	if state, err := AttachQSVPerceptualEvidence(context.Background(), &target, &variant, qsvTemporalFakePerceptualProvider{ref: ref}, lookup); err == nil || state != "stale" {
		t.Fatalf("stale #69 evidence state=%s err=%v", state, err)
	}
	if state, err := AttachQSVPerceptualEvidence(context.Background(), &target, &variant, nil, lookup); err == nil || state != "unavailable" {
		t.Fatalf("absent #69 evidence state=%s err=%v", state, err)
	}
}

func TestQSVControlComparisonSeparatesAttributionFromMitigation(t *testing.T) {
	target := QSVTemporalTarget{WindowIdentity: "window:texture", Interval: QSVTemporalInterval{StartSeconds: 60, DurationSeconds: 20}, Variants: []QSVTemporalVariant{{Isolation: QSVIsolationResult{Result: QSVIsolationValid}, TemporalEvidence: []QSVTemporalEvidence{{Availability: "measured", Interpretation: QSVTemporalInterpretationLikelyPumping}}}}}
	candidate := QSVExperimentCandidate{SourceRevision: "source", FilterPlanIdentity: "filter", NormalizationIdentity: "normalize", ReferenceDomain: "same", CandidateIdentity: "qsv"}
	control := EncoderControlCandidate{WindowIdentity: "window:texture", Interval: target.Interval, SourceRevision: "source", ArtifactIdentity: "x265", GenerationIdentity: "x265:1", FilterPlanIdentity: "filter", NormalizationIdentity: "normalize", ReferenceDomain: "same", EvidenceIdentity: "evidence", TemporalInterpretation: QSVTemporalInterpretationNotObserved, Comparable: QSVControlComparable}
	if got := CompareQSVControl(target, control, candidate); got.Result != QSVControlComparable {
		t.Fatalf("matching control=%#v", got)
	}
	target.ControlCandidates = []EncoderControlCandidate{control}
	if got := ClassifyQSVTemporalTarget(target, QSVControlComparison{Result: QSVControlComparable}); got != QSVTemporalClassificationReproducibleQVSSpecific {
		t.Fatalf("matching control classification=%s", got)
	}
	target.ControlCandidates = nil
	if got := ClassifyQSVTemporalTarget(target, QSVControlComparison{Result: QSVControlNotComparable}); got != QSVTemporalClassificationMeasurableNotSpecific {
		t.Fatalf("QSV-only classification=%s", got)
	}
	noPumping := target
	noPumping.Variants = []QSVTemporalVariant{{Isolation: QSVIsolationResult{Result: QSVIsolationValid}, TemporalEvidence: []QSVTemporalEvidence{{Availability: "measured", Interpretation: QSVTemporalInterpretationNotObserved}}}}
	if got := ClassifyQSVTemporalTarget(noPumping, QSVControlComparison{Result: QSVControlNotComparable}); got != QSVTemporalClassificationNotObserved {
		t.Fatalf("no-pumping classification=%s", got)
	}
	control.WindowIdentity = "window:other"
	if got := CompareQSVControl(target, control, candidate); got.Result != QSVControlNotComparable {
		t.Fatalf("different window control=%#v", got)
	}
}

func TestQSVTemporalTargetsDoNotRequireExternalMedia(t *testing.T) {
	primary, plan := qsvTemporalFixturePlan()
	experiment, err := QSVTemporalTargetsForFrozenPlan(primary, plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := experiment.Validate(); err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(experiment.Targets[0].Interval, QSVTemporalInterval{}) {
		t.Fatal("fixture did not produce a frozen interval")
	}
	if errors.Is(context.Canceled, context.DeadlineExceeded) {
		t.Fatal("unreachable test guard")
	}
}

func TestQSVTemporalCancellationPreservesCompletedTargetEvidence(t *testing.T) {
	primary, plan := qsvTemporalFixturePlan()
	experiment, err := QSVTemporalTargetsForFrozenPlan(primary, plan)
	if err != nil {
		t.Fatal(err)
	}
	experiment.Targets[0].TemporalEvidence = []QSVTemporalEvidence{{TargetWindowIdentity: experiment.Targets[0].WindowIdentity, Availability: "measured", Interpretation: QSVTemporalInterpretationNotObserved}}
	store := labFidelityOperationStore{items: map[string]*LabFidelityOperation{}, cancels: map[string]context.CancelFunc{}}
	ctx, cancel := context.WithCancel(context.Background())
	store.items["qsv-cancel"] = &LabFidelityOperation{ID: "qsv-cancel", Status: labFidelityRunning, TemporalExperiment: &experiment}
	store.cancels["qsv-cancel"] = cancel
	operation, ok := store.cancel("qsv-cancel")
	if !ok || operation.Status != labFidelityCanceled || operation.TemporalExperiment == nil || len(operation.TemporalExperiment.Targets[0].TemporalEvidence) != 1 {
		t.Fatalf("cancellation lost bounded target A evidence: %#v", operation)
	}
	if ctx.Err() == nil {
		t.Fatal("cancellation did not release the operation context")
	}
}

func TestQSVExperimentCandidateCapturesArtifactGenerationAndEffectivePlan(t *testing.T) {
	descriptor := &compatiblePreviewDescriptor{ArtifactID: "artifact:primary", GenerationToken: "generation", GenerationVersion: 4, CanonicalEncodeSignature: "encode:primary", EffectiveVideoEncoder: "hevc_qsv", Normalization: previewNormalizationDecision{Mode: "normalize_bt709"}, Request: compatiblePreviewRequest{GlobalQuality: 25, QSVRateControl: "icq"}, ResolvedVideoDecision: models.JSONMap{"cadence": "preserve"}}
	candidate := qsvExperimentCandidateFromDescriptor(descriptor, "request:primary", "window:primary", "source:1")
	if candidate.ArtifactID != "artifact:primary" || candidate.GenerationVersion != 4 || candidate.CanonicalEncodeSignature != "encode:primary" || candidate.SamplingWindowIdentity != "window:primary" || candidate.EffectiveVideoPlan["cadence"] != "preserve" {
		t.Fatalf("candidate identity capture incomplete: %#v", candidate)
	}
	if candidate.CandidateIdentity == "" {
		t.Fatal("candidate identity is missing")
	}
}
