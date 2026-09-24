package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/capabilities"
	"github.com/anuelvs/mvforge/backend/internal/models"
)

const (
	qsvTemporalExperimentSchemaVersion     = "qsv-temporal-experiment-v1"
	qsvTemporalExperimentPolicyVersion     = "qsv-temporal-policy-v1"
	qsvTemporalInterpretationPolicyVersion = "qsv-temporal-interpretation-v1"
	qsvTemporalEvidenceSchemaVersion       = "qsv-temporal-evidence-v1"
	qsvTemporalControlSchemaVersion        = "qsv-temporal-control-v1"
	qsvTemporalSamplingPolicyVersion       = "qsv-temporal-sampling-v1"
	qsvTemporalSamplingRateHz              = 8.0
	qsvTemporalMaxFrames                   = 32
	qsvTemporalMaxEvents                   = 8
)

const (
	QSVTemporalClassificationReproducibleQVSSpecific = "reproducible_qsv_specific_pumping"
	QSVTemporalClassificationMeasurableNotSpecific   = "measurable_not_clearly_qsv_specific"
	QSVTemporalClassificationInconclusive            = "inconclusive"
	QSVTemporalClassificationNotObserved             = "no_pumping_in_tested_windows"

	QSVTemporalInterpretationLikelyPumping = "likely_pumping"
	QSVTemporalInterpretationInconclusive  = "inconclusive"
	QSVTemporalInterpretationNotObserved   = "not_observed"
	QSVTemporalMitigationImproved          = "improved"
	QSVTemporalMitigationWorsened          = "worsened"
	QSVTemporalMitigationUnchanged         = "unchanged"
	QSVTemporalMitigationInconclusive      = "inconclusive"

	QSVIsolationValid            = "valid"
	QSVIsolationInvalid          = "invalid/non-isolated"
	QSVIsolationInconclusive     = "inconclusive"
	QSVIsolationNotComparable    = "not-comparable"
	QSVControlComparable         = "comparable"
	QSVControlNotComparable      = "not-comparable"
	QSVTemporalChangedQuality    = "quality_rate_control"
	QSVTemporalChangedGOP        = "gop"
	QSVTemporalChangedBFrames    = "b_frame_strategy"
	QSVTemporalChangedQSVFeature = "qsv_feature"
)

// QSVTemporalExperimentPolicy bounds every request before any compatible
// preview request is created. The values are part of experiment identity.
type QSVTemporalExperimentPolicy struct {
	Version                 string `json:"version"`
	InterpretationVersion   string `json:"interpretationVersion"`
	MaxTargets              int    `json:"maxTargets"`
	MaxVariantsPerTarget    int    `json:"maxVariantsPerTarget"`
	MaxTotalQSVVariants     int    `json:"maxTotalQSVVariants"`
	MaxOptionalControls     int    `json:"maxOptionalControls"`
	MaxBaselineMaterializes int    `json:"maxBaselineMaterializations"`
	MaxTotalHeavyEncodes    int    `json:"maxTotalHeavyEncodes"`
}

type QSVTemporalCapabilityResult struct {
	Feature   string `json:"feature"`
	Supported bool   `json:"supported"`
	Reason    string `json:"reason,omitempty"`
}

type QSVTemporalSamplingPolicy struct {
	Version   string  `json:"version"`
	RateHz    float64 `json:"rateHz"`
	MaxFrames int     `json:"maxFrames"`
	Filter    string  `json:"filter"`
}

func DefaultQSVTemporalSamplingPolicyV1() QSVTemporalSamplingPolicy {
	return QSVTemporalSamplingPolicy{Version: qsvTemporalSamplingPolicyVersion, RateHz: qsvTemporalSamplingRateHz, MaxFrames: qsvTemporalFrameLimit, Filter: "fps=8,scale=64:36:flags=bilinear"}
}

func QSVTemporalFeatureCapability(capability capabilities.EncoderCapability, feature string) QSVTemporalCapabilityResult {
	feature = strings.ToLower(strings.TrimSpace(feature))
	result := QSVTemporalCapabilityResult{Feature: feature, Reason: "feature is not reported by the existing QSV capability probe"}
	switch feature {
	case "lookahead", "look_ahead":
		result.Supported, result.Reason = capability.LookAhead, "existing QSV look-ahead capability"
	case "extbrc", "extended_brc":
		result.Supported, result.Reason = capability.ExtendedBRC, "existing QSV ExtBRC capability"
	case "adaptive_i":
		result.Supported, result.Reason = capability.AdaptiveI, "existing QSV adaptive-I capability"
	case "adaptive_b":
		result.Supported, result.Reason = capability.AdaptiveB, "existing QSV adaptive-B capability"
	case "mbbrc":
		for mode, supported := range capability.TestedModes {
			if supported && strings.Contains(strings.ToLower(mode), "mbbrc") {
				result.Supported = true
				break
			}
		}
		result.Reason = "existing tested QSV MBBRC modes"
	case "rdo":
		for mode, supported := range capability.TestedModes {
			if supported && strings.Contains(strings.ToLower(mode), "rdo") {
				result.Supported = true
				break
			}
		}
		result.Reason = "existing tested QSV RDO modes"
	case "p_strategy", "pstrategy":
		for mode, supported := range capability.TestedModes {
			if supported && strings.Contains(strings.ToLower(mode), "pstrategy") {
				result.Supported = true
				break
			}
		}
		result.Reason = "existing tested QSV P-strategy modes"
	case "quality_rate_control":
		result.Supported, result.Reason = capability.ICQ || capability.QSVFullCombination, "existing QSV quality/rate-control capability"
	case "gop", "b_frame_strategy":
		result.Supported, result.Reason = capability.Usable, "GOP/B-frame strategy is resolved by the existing effective-plan capability"
	}
	return result
}

func DefaultQSVTemporalExperimentPolicyV1() QSVTemporalExperimentPolicy {
	return QSVTemporalExperimentPolicy{
		Version:                 qsvTemporalExperimentPolicyVersion,
		InterpretationVersion:   qsvTemporalInterpretationPolicyVersion,
		MaxTargets:              3,
		MaxVariantsPerTarget:    3,
		MaxTotalQSVVariants:     6,
		MaxOptionalControls:     3,
		MaxBaselineMaterializes: 3,
		MaxTotalHeavyEncodes:    12,
	}
}

type QSVTemporalExperiment struct {
	SchemaVersion         string                      `json:"schemaVersion"`
	ExperimentID          string                      `json:"experimentId"`
	SourceRevision        string                      `json:"sourceRevision"`
	SamplingPlanIdentity  string                      `json:"samplingPlanIdentity"`
	SamplingPolicyVersion string                      `json:"samplingPolicyVersion"`
	Targets               []QSVTemporalTarget         `json:"targets"`
	UnavailableRoles      []string                    `json:"unavailableRoles,omitempty"`
	Policy                QSVTemporalExperimentPolicy `json:"policy"`
	Warnings              []string                    `json:"warnings,omitempty"`
	Provenance            models.JSONMap              `json:"provenance,omitempty"`
}

type QSVTemporalTarget struct {
	WindowIdentity     string                              `json:"windowIdentity"`
	Interval           QSVTemporalInterval                 `json:"interval"`
	Roles              []string                            `json:"roles,omitempty"`
	Reasons            []string                            `json:"reasons,omitempty"`
	Primary            bool                                `json:"primary"`
	Baseline           QSVExperimentCandidate              `json:"baseline"`
	Variants           []QSVTemporalVariant                `json:"variants,omitempty"`
	TemporalEvidence   []QSVTemporalEvidence               `json:"temporalEvidence,omitempty"`
	PerceptualEvidence []QSVPerceptualEvidenceRef          `json:"perceptualEvidence,omitempty"`
	ControlCandidates  []EncoderControlCandidate           `json:"controlCandidates,omitempty"`
	Classification     string                              `json:"classification,omitempty"`
	Recommendation     *QSVPumpingMitigationRecommendation `json:"recommendation,omitempty"`
	Warnings           []string                            `json:"warnings,omitempty"`
}

type QSVTemporalInterval struct {
	StartSeconds    float64 `json:"startSeconds"`
	DurationSeconds float64 `json:"durationSeconds"`
}

type QSVTemporalVariant struct {
	VariantIdentity    string                     `json:"variantIdentity"`
	ChangedVariable    string                     `json:"changedVariable"`
	RequestedDelta     models.JSONMap             `json:"requestedDelta,omitempty"`
	EffectiveDelta     models.JSONMap             `json:"effectiveDelta,omitempty"`
	Isolation          QSVIsolationResult         `json:"isolation"`
	Candidate          QSVExperimentCandidate     `json:"candidate"`
	TemporalEvidence   []QSVTemporalEvidence      `json:"temporalEvidence,omitempty"`
	PerceptualEvidence []QSVPerceptualEvidenceRef `json:"perceptualEvidence,omitempty"`
}

type QSVExperimentCandidate struct {
	CandidateIdentity         string         `json:"candidateIdentity"`
	SourceRevision            string         `json:"sourceRevision"`
	SamplingWindowIdentity    string         `json:"samplingWindowIdentity"`
	RequestID                 string         `json:"requestId,omitempty"`
	ArtifactID                string         `json:"artifactId,omitempty"`
	GenerationToken           string         `json:"generationToken,omitempty"`
	GenerationVersion         int64          `json:"generationVersion,omitempty"`
	CanonicalEncodeSignature  string         `json:"canonicalEncodeSignature,omitempty"`
	RequestedQSVSettings      models.JSONMap `json:"requestedQSVSettings,omitempty"`
	EffectiveQSVSettings      models.JSONMap `json:"effectiveQSVSettings,omitempty"`
	FallbackReason            string         `json:"fallbackReason,omitempty"`
	RuntimeCapabilityIdentity string         `json:"runtimeCapabilityIdentity,omitempty"`
	EffectiveEncoder          string         `json:"effectiveEncoder,omitempty"`
	GOP                       models.JSONMap `json:"gop,omitempty"`
	BFrameBehavior            models.JSONMap `json:"bFrameBehavior,omitempty"`
	FilterPlanIdentity        string         `json:"filterPlanIdentity,omitempty"`
	NormalizationIdentity     string         `json:"normalizationIdentity,omitempty"`
	ReferenceDomain           string         `json:"referenceDomain,omitempty"`
	EffectiveVideoPlan        models.JSONMap `json:"effectiveVideoPlan,omitempty"`
	CanonicalFidelity         models.JSONMap `json:"canonicalFidelity,omitempty"`
	FrameBitstreamEvidence    models.JSONMap `json:"frameBitstreamEvidence,omitempty"`
	SizeBytes                 int64          `json:"sizeBytes,omitempty"`
	Bitrate                   float64        `json:"bitrate,omitempty"`
	Speed                     float64        `json:"speed,omitempty"`
}

type EncoderControlCandidate struct {
	SchemaVersion              string              `json:"schemaVersion"`
	WindowIdentity             string              `json:"windowIdentity"`
	Interval                   QSVTemporalInterval `json:"interval"`
	SourceRevision             string              `json:"sourceRevision"`
	EncoderFamily              string              `json:"encoderFamily"`
	RequestID                  string              `json:"requestId,omitempty"`
	ArtifactIdentity           string              `json:"artifactIdentity"`
	GenerationIdentity         string              `json:"generationIdentity"`
	EffectivePlan              models.JSONMap      `json:"effectivePlan,omitempty"`
	CanonicalFidelity          models.JSONMap      `json:"canonicalFidelity,omitempty"`
	FilterPlanIdentity         string              `json:"filterPlanIdentity"`
	NormalizationIdentity      string              `json:"normalizationIdentity"`
	ReferenceDomain            string              `json:"referenceDomain"`
	EvidenceIdentity           string              `json:"evidenceIdentity"`
	TemporalInterpretation     string              `json:"temporalInterpretation,omitempty"`
	ArtifactGeneration         string              `json:"artifactGeneration,omitempty"`
	ExpectedGenerationIdentity string              `json:"expectedGenerationIdentity,omitempty"`
	ExpectedEvidenceIdentity   string              `json:"expectedEvidenceIdentity,omitempty"`
	Comparable                 string              `json:"comparable"`
	Warnings                   []string            `json:"warnings,omitempty"`
}

type QSVIsolationResult struct {
	Result           string   `json:"result"`
	DeclaredVariable string   `json:"declaredVariable"`
	AllowedDerived   []string `json:"allowedDerived,omitempty"`
	ChangedFields    []string `json:"changedFields,omitempty"`
	UnexpectedFields []string `json:"unexpectedFields,omitempty"`
	Reason           string   `json:"reason,omitempty"`
}

type QSVTemporalFrameSample struct {
	FrameIndex            int     `json:"frameIndex"`
	TimestampSeconds      float64 `json:"timestampSeconds"`
	Comparable            bool    `json:"comparable"`
	RegionIdentity        string  `json:"regionIdentity,omitempty"`
	DetailEnergy          float64 `json:"detailEnergy"`
	GrainEnergy           float64 `json:"grainEnergy"`
	HighFrequencyEnergy   float64 `json:"highFrequencyEnergy"`
	LocalVariance         float64 `json:"localVariance"`
	SamplingPolicyVersion string  `json:"samplingPolicyVersion,omitempty"`
	SamplingRateHz        float64 `json:"samplingRateHz,omitempty"`
}

type QSVTemporalEvidenceRequest struct {
	TargetWindowIdentity  string                   `json:"targetWindowIdentity"`
	VariantIdentity       string                   `json:"variantIdentity"`
	SourceRevision        string                   `json:"sourceRevision"`
	BaselineArtifactID    string                   `json:"baselineArtifactId"`
	VariantArtifactID     string                   `json:"variantArtifactId"`
	NormalizationIdentity string                   `json:"normalizationIdentity"`
	RegionIdentity        string                   `json:"regionIdentity"`
	SampleStartSeconds    float64                  `json:"sampleStartSeconds"`
	SampleDurationSeconds float64                  `json:"sampleDurationSeconds"`
	SamplingPolicyVersion string                   `json:"samplingPolicyVersion"`
	SamplingRateHz        float64                  `json:"samplingRateHz"`
	ReferenceFrames       []QSVTemporalFrameSample `json:"referenceFrames,omitempty"`
	BaselineFrames        []QSVTemporalFrameSample `json:"baselineFrames"`
	VariantFrames         []QSVTemporalFrameSample `json:"variantFrames"`
}

type QSVTemporalEvidence struct {
	SchemaVersion               string                         `json:"schemaVersion"`
	TargetWindowIdentity        string                         `json:"targetWindowIdentity"`
	VariantIdentity             string                         `json:"variantIdentity"`
	SourceRevision              string                         `json:"sourceRevision"`
	BaselineArtifactID          string                         `json:"baselineArtifactId"`
	VariantArtifactID           string                         `json:"variantArtifactId"`
	SamplingPolicyVersion       string                         `json:"samplingPolicyVersion"`
	SamplingRateHz              float64                        `json:"samplingRateHz"`
	SampleStartSeconds          float64                        `json:"sampleStartSeconds"`
	SampleDurationSeconds       float64                        `json:"sampleDurationSeconds"`
	FrameCount                  int                            `json:"frameCount"`
	NormalizationIdentity       string                         `json:"normalizationIdentity"`
	RegionIdentity              string                         `json:"regionIdentity"`
	DetailEnergyByFrame         []float64                      `json:"detailEnergyByFrame,omitempty"`
	GrainEnergyByFrame          []float64                      `json:"grainEnergyByFrame,omitempty"`
	BlurProxyByFrame            []float64                      `json:"blurProxyByFrame,omitempty"`
	VariationByFrame            []float64                      `json:"variationByFrame,omitempty"`
	LossRecoveryEvents          []QSVTemporalLossRecoveryEvent `json:"lossRecoveryEvents,omitempty"`
	TemporalVariationSummary    QSVTemporalVariationSummary    `json:"temporalVariationSummary"`
	BaselineDetailEnergyByFrame []float64                      `json:"baselineDetailEnergyByFrame,omitempty"`
	BaselineGrainEnergyByFrame  []float64                      `json:"baselineGrainEnergyByFrame,omitempty"`
	BaselineBlurProxyByFrame    []float64                      `json:"baselineBlurProxyByFrame,omitempty"`
	BaselineVariationByFrame    []float64                      `json:"baselineVariationByFrame,omitempty"`
	BaselineLossRecoveryEvents  []QSVTemporalLossRecoveryEvent `json:"baselineLossRecoveryEvents,omitempty"`
	BaselineVariationSummary    QSVTemporalVariationSummary    `json:"baselineVariationSummary"`
	BaselineInterpretation      string                         `json:"baselineInterpretation"`
	VariantInterpretation       string                         `json:"variantInterpretation"`
	MitigationDirection         string                         `json:"mitigationDirection"`
	BaselineEvidenceIdentity    string                         `json:"baselineEvidenceIdentity,omitempty"`
	VariantEvidenceIdentity     string                         `json:"variantEvidenceIdentity,omitempty"`
	Availability                string                         `json:"availability"`
	EvidenceIdentity            string                         `json:"evidenceIdentity,omitempty"`
	Interpretation              string                         `json:"interpretation"`
	InterpretationPolicyVersion string                         `json:"interpretationPolicyVersion"`
	Warnings                    []string                       `json:"warnings,omitempty"`
}

type QSVTemporalLossRecoveryEvent struct {
	LossFrameIndex     int     `json:"lossFrameIndex"`
	RecoveryFrameIndex int     `json:"recoveryFrameIndex"`
	LossMagnitude      float64 `json:"lossMagnitude"`
	RecoveryMagnitude  float64 `json:"recoveryMagnitude"`
}

type QSVTemporalVariationSummary struct {
	ComparableFrames   int     `json:"comparableFrames"`
	MeanAbsVariation   float64 `json:"meanAbsVariation"`
	MaximumVariation   float64 `json:"maximumVariation"`
	LossEventCount     int     `json:"lossEventCount"`
	RecoveryEventCount int     `json:"recoveryEventCount"`
}

type QSVPerceptualEvidenceRef struct {
	EvidenceID         string         `json:"evidenceId"`
	Version            string         `json:"version"`
	SourceRevision     string         `json:"sourceRevision"`
	PlanIdentity       string         `json:"planIdentity"`
	WindowIdentity     string         `json:"windowIdentity"`
	ArtifactGeneration string         `json:"artifactGeneration"`
	Summary            models.JSONMap `json:"summary,omitempty"`
}

type QSVPerceptualEvidenceLookup struct {
	SourceRevision       string
	SamplingPlanIdentity string
	WindowIdentity       string
	ArtifactGeneration   string
	EvidenceVersion      string
}

type QSVPerceptualEvidenceProvider interface {
	Lookup(context.Context, QSVPerceptualEvidenceLookup) (*QSVPerceptualEvidenceRef, error)
}

var qsvPerceptualEvidenceSeam struct {
	sync.RWMutex
	provider QSVPerceptualEvidenceProvider
}

func qsvPerceptualEvidenceProvider() QSVPerceptualEvidenceProvider {
	qsvPerceptualEvidenceSeam.RLock()
	defer qsvPerceptualEvidenceSeam.RUnlock()
	return qsvPerceptualEvidenceSeam.provider
}

func setQSVPerceptualEvidenceProviderForTest(provider QSVPerceptualEvidenceProvider) func() {
	qsvPerceptualEvidenceSeam.Lock()
	previous := qsvPerceptualEvidenceSeam.provider
	qsvPerceptualEvidenceSeam.provider = provider
	qsvPerceptualEvidenceSeam.Unlock()
	return func() {
		qsvPerceptualEvidenceSeam.Lock()
		qsvPerceptualEvidenceSeam.provider = previous
		qsvPerceptualEvidenceSeam.Unlock()
	}
}

// AttachQSVPerceptualEvidence is deliberately an adapter only. #9 stores a
// bounded reference and never computes VMAF/SSIM itself.
func AttachQSVPerceptualEvidence(ctx context.Context, target *QSVTemporalTarget, variant *QSVTemporalVariant, provider QSVPerceptualEvidenceProvider, lookup QSVPerceptualEvidenceLookup) (string, error) {
	if provider == nil {
		return "unavailable", errors.New("canonical perceptual evidence provider is unavailable")
	}
	if target == nil || variant == nil {
		return "unavailable", errors.New("perceptual evidence target or variant is missing")
	}
	ref, err := provider.Lookup(ctx, lookup)
	if err != nil {
		return "unavailable", err
	}
	if ref == nil || ref.EvidenceID == "" || ref.SourceRevision != lookup.SourceRevision || ref.PlanIdentity != lookup.SamplingPlanIdentity || ref.WindowIdentity != lookup.WindowIdentity || ref.ArtifactGeneration != lookup.ArtifactGeneration {
		return "stale", errors.New("canonical perceptual evidence identity does not match the frozen candidate")
	}
	if variant.VariantIdentity != "" {
		variant.PerceptualEvidence = append(variant.PerceptualEvidence, *ref)
	} else {
		target.PerceptualEvidence = append(target.PerceptualEvidence, *ref)
	}
	return "attached", nil
}

type QSVPumpingMitigationRecommendation struct {
	Version            string         `json:"version"`
	ChangedVariable    string         `json:"changedVariable"`
	EffectiveSetting   models.JSONMap `json:"effectiveSetting"`
	EvidenceReferences []string       `json:"evidenceReferences"`
	Encoder            string         `json:"encoder"`
	Runtime            string         `json:"runtime"`
	MediaRoles         []string       `json:"mediaRoles,omitempty"`
	Confidence         string         `json:"confidence"`
	Tradeoffs          models.JSONMap `json:"tradeoffs,omitempty"`
	ControlAttribution string         `json:"controlAttribution"`
}

// QSVTemporalExperimentRequest is accepted only as an explicit LAB action.
// It describes already requested variants; it does not contain an encode path.
type QSVTemporalExperimentRequest struct {
	VariantsByWindow []QSVTemporalTargetRequest  `json:"variantsByWindow,omitempty"`
	Controls         []QSVTemporalControlRequest `json:"controls,omitempty"`
}

type QSVTemporalTargetRequest struct {
	WindowIdentity string                      `json:"windowIdentity"`
	Variants       []QSVTemporalVariantRequest `json:"variants,omitempty"`
}

type QSVTemporalVariantRequest struct {
	VariantIdentity string         `json:"variantIdentity,omitempty"`
	ChangedVariable string         `json:"changedVariable"`
	RequestedDelta  models.JSONMap `json:"requestedDelta"`
}

type QSVTemporalControlRequest struct {
	WindowIdentity     string `json:"windowIdentity"`
	RequestID          string `json:"requestId,omitempty"`
	GenerationIdentity string `json:"generationIdentity,omitempty"`
	EvidenceIdentity   string `json:"evidenceIdentity,omitempty"`
}

func QSVTemporalTargetsForFrozenPlan(primary SamplingWindow, plan SamplingPlan) (QSVTemporalExperiment, error) {
	policy := DefaultQSVTemporalExperimentPolicyV1()
	selection := SamplingPlanForQSVPumping(plan)

	primary = normalizeQSVTemporalWindow(primary, plan)
	targets := make([]QSVTemporalTarget, 0, policy.MaxTargets)
	targets = append(targets, qsvTemporalTargetFromWindow(primary))

	// SamplingPlanForQSVPumping is the only source of role windows. Keep the
	// frozen selection order and admit at most one canonical window per role;
	// this is a bound, not a replacement/worst-scene search.
	seenRoles := map[string]bool{}
	for _, role := range selection.RequestedRoles {
		for _, window := range selection.Windows {
			if len(targets) >= policy.MaxTargets || seenRoles[role] || !containsSamplingString(window.Roles, role) {
				continue
			}
			window = normalizeQSVTemporalWindow(window, plan)
			if qsvTemporalTargetExists(targets, window) {
				seenRoles[role] = true
				continue
			}
			targets = append(targets, qsvTemporalTargetFromWindow(window))
			seenRoles[role] = true
		}
	}

	experiment := QSVTemporalExperiment{
		SchemaVersion:         qsvTemporalExperimentSchemaVersion,
		SourceRevision:        plan.SourceRevision,
		SamplingPlanIdentity:  selection.PlanIdentity,
		SamplingPolicyVersion: selection.PolicyVersion,
		Targets:               targets,
		UnavailableRoles:      append([]string(nil), selection.UnavailableRoles...),
		Policy:                policy,
	}
	if err := ValidateQSVTemporalExperimentBounds(experiment); err != nil {
		return QSVTemporalExperiment{}, err
	}
	experiment.ExperimentID = ComputeQSVTemporalExperimentID(experiment)
	return experiment, nil
}

func (experiment *QSVTemporalExperiment) ApplyRequestedVariants(request QSVTemporalExperimentRequest) error {
	if experiment == nil {
		return errors.New("qsv temporal experiment is nil")
	}
	byWindow := make(map[string]int, len(experiment.Targets))
	for index := range experiment.Targets {
		byWindow[experiment.Targets[index].WindowIdentity] = index
	}
	for _, targetRequest := range request.VariantsByWindow {
		index, ok := byWindow[targetRequest.WindowIdentity]
		if !ok {
			return fmt.Errorf("qsv temporal variant target window %q is unavailable", targetRequest.WindowIdentity)
		}
		for _, variantRequest := range targetRequest.Variants {
			variable := strings.TrimSpace(variantRequest.ChangedVariable)
			if !qsvTemporalVariableSupported(variable) {
				return fmt.Errorf("unsupported qsv temporal changed variable %q", variable)
			}
			variantID := strings.TrimSpace(variantRequest.VariantIdentity)
			if variantID == "" {
				payload, _ := json.Marshal(struct {
					Window   string
					Variable string
					Delta    models.JSONMap
				}{experiment.Targets[index].WindowIdentity, variable, variantRequest.RequestedDelta})
				digest := sha256.Sum256(payload)
				variantID = "qsv-variant:" + hex.EncodeToString(digest[:12])
			}
			experiment.Targets[index].Variants = append(experiment.Targets[index].Variants, QSVTemporalVariant{VariantIdentity: variantID, ChangedVariable: variable, RequestedDelta: variantRequest.RequestedDelta, Isolation: QSVIsolationResult{Result: QSVIsolationInconclusive, DeclaredVariable: variable}})
		}
	}
	for _, controlRequest := range request.Controls {
		index, ok := byWindow[controlRequest.WindowIdentity]
		if !ok {
			return fmt.Errorf("qsv temporal control target window %q is unavailable", controlRequest.WindowIdentity)
		}
		controlID := strings.TrimSpace(controlRequest.RequestID)
		experiment.Targets[index].ControlCandidates = append(experiment.Targets[index].ControlCandidates, EncoderControlCandidate{
			SchemaVersion:              qsvTemporalControlSchemaVersion,
			WindowIdentity:             controlRequest.WindowIdentity,
			Interval:                   experiment.Targets[index].Interval,
			RequestID:                  controlID,
			ArtifactIdentity:           controlID,
			ExpectedGenerationIdentity: strings.TrimSpace(controlRequest.GenerationIdentity),
			ExpectedEvidenceIdentity:   strings.TrimSpace(controlRequest.EvidenceIdentity),
			Comparable:                 QSVControlNotComparable,
			Warnings:                   []string{"control comparability is pending artifact identity validation"},
		})
	}
	if err := ValidateQSVTemporalExperimentBounds(*experiment); err != nil {
		return err
	}
	experiment.ExperimentID = ComputeQSVTemporalExperimentID(*experiment)
	return nil
}

func qsvTemporalVariableSupported(variable string) bool {
	switch strings.ToLower(strings.TrimSpace(variable)) {
	case QSVTemporalChangedQuality, QSVTemporalChangedGOP, QSVTemporalChangedBFrames, QSVTemporalChangedQSVFeature:
		return true
	default:
		return false
	}
}

func QSVTemporalVariantCapability(descriptor *compatiblePreviewDescriptor, variant QSVTemporalVariant, capability capabilities.EncoderCapability) QSVTemporalCapabilityResult {
	variable := strings.ToLower(strings.TrimSpace(variant.ChangedVariable))
	if descriptor == nil || !strings.EqualFold(strings.TrimSpace(descriptor.EffectiveVideoEncoder), "hevc_qsv") {
		return QSVTemporalCapabilityResult{Feature: variable, Reason: "the frozen baseline does not resolve to hevc_qsv"}
	}
	if !capability.Usable {
		reason := capability.Reason
		if strings.TrimSpace(reason) == "" {
			reason = "existing QSV capability probe reports the encoder is not usable"
		}
		return QSVTemporalCapabilityResult{Feature: variable, Reason: reason}
	}
	if variable == QSVTemporalChangedQuality {
		return QSVTemporalFeatureCapability(capability, "quality_rate_control")
	}
	if variable == QSVTemporalChangedGOP {
		return QSVTemporalFeatureCapability(capability, "gop")
	}
	if variable == QSVTemporalChangedBFrames {
		return QSVTemporalFeatureCapability(capability, "b_frame_strategy")
	}
	if variable != QSVTemporalChangedQSVFeature {
		return QSVTemporalCapabilityResult{Feature: variable, Reason: "unsupported QSV temporal variable"}
	}
	if len(variant.RequestedDelta) == 0 {
		return QSVTemporalCapabilityResult{Feature: variable, Reason: "QSV feature variant does not declare a control"}
	}
	for field := range variant.RequestedDelta {
		feature := ""
		switch strings.ToLower(strings.TrimSpace(field)) {
		case "lookaheaddepth", "qsvlookaheaddepth":
			feature = "lookahead"
		case "extendedbrc", "qsvextendedbrc":
			feature = "extbrc"
		case "adaptivei", "qsvadaptivei":
			feature = "adaptive_i"
		case "adaptiveb", "qsvadaptiveb":
			feature = "adaptive_b"
		case "mbbrcmode", "qsvmbbrcmode":
			feature = "mbbrc"
		case "rdomode", "qsvrdomode":
			feature = "rdo"
		case "pstrategy", "qsvpstrategy":
			feature = "p_strategy"
		case "qsvratecontrol", "ratecontrol":
			feature = "quality_rate_control"
		default:
			return QSVTemporalCapabilityResult{Feature: field, Reason: "feature is not reported by the existing QSV capability probe"}
		}
		result := QSVTemporalFeatureCapability(capability, feature)
		if !result.Supported {
			return result
		}
	}
	return QSVTemporalCapabilityResult{Feature: variable, Supported: true, Reason: "existing QSV capability/effective-plan authority"}
}

func normalizeQSVTemporalWindow(window SamplingWindow, plan SamplingPlan) SamplingWindow {
	if strings.TrimSpace(window.Identity) == "" {
		window.Identity = samplingWindowIdentity(plan.SourceFingerprint, plan.SourceStreamIdentity, window.StartSeconds, window.DurationSeconds)
	}
	window.Roles = normalizeSamplingRoles(window.Roles)
	window.Reasons = normalizeSamplingStrings(window.Reasons)
	return window
}

func qsvTemporalTargetFromWindow(window SamplingWindow) QSVTemporalTarget {
	return QSVTemporalTarget{
		WindowIdentity: window.Identity,
		Interval:       QSVTemporalInterval{StartSeconds: window.StartSeconds, DurationSeconds: window.DurationSeconds},
		Roles:          append([]string(nil), window.Roles...), Reasons: append([]string(nil), window.Reasons...), Primary: window.Primary,
		Classification: QSVTemporalClassificationInconclusive,
	}
}

func qsvTemporalTargetExists(targets []QSVTemporalTarget, window SamplingWindow) bool {
	for _, target := range targets {
		if target.WindowIdentity == window.Identity || samplingWindowsEquivalent(SamplingWindow{StartSeconds: target.Interval.StartSeconds, DurationSeconds: target.Interval.DurationSeconds}, window, 1.0/30.0) {
			return true
		}
	}
	return false
}

func ValidateQSVTemporalExperimentBounds(experiment QSVTemporalExperiment) error {
	policy := experiment.Policy
	if policy.MaxTargets == 0 {
		policy = DefaultQSVTemporalExperimentPolicyV1()
	}
	if len(experiment.Targets) > policy.MaxTargets {
		return fmt.Errorf("qsv temporal experiment exceeds max targets: %d > %d", len(experiment.Targets), policy.MaxTargets)
	}
	variants, controls, baselineMaterializations := 0, 0, 0
	for index, target := range experiment.Targets {
		if target.WindowIdentity == "" || target.Baseline.SamplingWindowIdentity != "" && target.Baseline.SamplingWindowIdentity != target.WindowIdentity {
			return fmt.Errorf("target %d baseline does not belong to its window", index)
		}
		if len(target.Variants) > policy.MaxVariantsPerTarget {
			return fmt.Errorf("target %s exceeds max variants per target", target.WindowIdentity)
		}
		variants += len(target.Variants)
		controls += len(target.ControlCandidates)
		if !target.Primary && target.Baseline.ArtifactID == "" {
			baselineMaterializations++
		}
		for variantIndex, variant := range target.Variants {
			if variant.Candidate.SamplingWindowIdentity != "" && variant.Candidate.SamplingWindowIdentity != target.WindowIdentity {
				return fmt.Errorf("variant %d in target %s belongs to another window", variantIndex, target.WindowIdentity)
			}
		}
	}
	if variants > policy.MaxTotalQSVVariants {
		return fmt.Errorf("qsv temporal experiment exceeds total variant cap: %d > %d", variants, policy.MaxTotalQSVVariants)
	}
	if controls > policy.MaxOptionalControls {
		return fmt.Errorf("qsv temporal experiment exceeds optional control cap: %d > %d", controls, policy.MaxOptionalControls)
	}
	if baselineMaterializations > policy.MaxBaselineMaterializes {
		return fmt.Errorf("qsv temporal experiment exceeds baseline materialization cap: %d > %d", baselineMaterializations, policy.MaxBaselineMaterializes)
	}
	if baselineMaterializations+variants+controls > policy.MaxTotalHeavyEncodes {
		return fmt.Errorf("qsv temporal experiment exceeds total heavy encode cap")
	}
	return nil
}

func ComputeQSVTemporalExperimentID(experiment QSVTemporalExperiment) string {
	type stableVariant struct {
		Identity  string         `json:"identity"`
		Variable  string         `json:"variable"`
		Requested models.JSONMap `json:"requested,omitempty"`
		Effective models.JSONMap `json:"effective,omitempty"`
		Candidate string         `json:"candidate,omitempty"`
	}
	type stableTarget struct {
		Window             string              `json:"window"`
		Interval           QSVTemporalInterval `json:"interval"`
		Primary            bool                `json:"primary"`
		Baseline           string              `json:"baseline"`
		BaselineGeneration string              `json:"baselineGeneration,omitempty"`
		Variants           []stableVariant     `json:"variants,omitempty"`
		Controls           []string            `json:"controls,omitempty"`
	}
	targets := make([]stableTarget, 0, len(experiment.Targets))
	for _, target := range experiment.Targets {
		stable := stableTarget{Window: target.WindowIdentity, Interval: target.Interval, Primary: target.Primary, Baseline: target.Baseline.CandidateIdentity, BaselineGeneration: qsvCandidateGeneration(target.Baseline)}
		for _, variant := range target.Variants {
			stable.Variants = append(stable.Variants, stableVariant{Identity: variant.VariantIdentity, Variable: variant.ChangedVariable, Requested: variant.RequestedDelta, Effective: variant.EffectiveDelta, Candidate: variant.Candidate.CandidateIdentity})
		}
		for _, control := range target.ControlCandidates {
			stable.Controls = append(stable.Controls, control.ArtifactIdentity+"@"+control.GenerationIdentity)
		}
		targets = append(targets, stable)
	}
	identity := struct {
		Schema      string                      `json:"schema"`
		Source      string                      `json:"source"`
		Plan        string                      `json:"plan"`
		PlanPolicy  string                      `json:"planPolicy"`
		Policy      QSVTemporalExperimentPolicy `json:"policy"`
		Unavailable []string                    `json:"unavailable,omitempty"`
		Targets     []stableTarget              `json:"targets"`
	}{qsvTemporalExperimentSchemaVersion, experiment.SourceRevision, experiment.SamplingPlanIdentity, experiment.SamplingPolicyVersion, experiment.Policy, append([]string(nil), experiment.UnavailableRoles...), targets}
	payload, _ := json.Marshal(identity)
	digest := sha256.Sum256(payload)
	return "qsv-experiment:" + hex.EncodeToString(digest[:16])
}

func qsvCandidateGeneration(candidate QSVExperimentCandidate) string {
	return fmt.Sprintf("%s:%d", candidate.GenerationToken, candidate.GenerationVersion)
}

func qsvExperimentCandidateFromDescriptor(descriptor *compatiblePreviewDescriptor, requestID, windowIdentity, sourceRevision string) QSVExperimentCandidate {
	candidate := QSVExperimentCandidate{RequestID: requestID, SamplingWindowIdentity: windowIdentity, SourceRevision: sourceRevision}
	if descriptor == nil {
		return candidate
	}
	candidate.ArtifactID = descriptor.ArtifactID
	candidate.GenerationToken = descriptor.GenerationToken
	candidate.GenerationVersion = descriptor.GenerationVersion
	candidate.CanonicalEncodeSignature = descriptor.CanonicalEncodeSignature
	candidate.RuntimeCapabilityIdentity = qsvDescriptorWorkerValue(descriptor, "qsvCapabilityIdentity")
	if candidate.RuntimeCapabilityIdentity == "" {
		candidate.RuntimeCapabilityIdentity = "runtime:unavailable"
	}
	candidate.EffectiveEncoder = descriptor.EffectiveVideoEncoder
	candidate.FallbackReason = qsvDescriptorWorkerValue(descriptor, "qsvRateControlFallbackReason")
	candidate.RequestedQSVSettings = qsvDescriptorQSVSettings(descriptor, false)
	candidate.EffectiveQSVSettings = qsvDescriptorQSVSettings(descriptor, true)
	candidate.GOP = qsvDescriptorFrameSettings(descriptor, true)
	candidate.BFrameBehavior = qsvDescriptorBFrameSettings(descriptor, true)
	candidate.EffectiveVideoPlan = descriptor.ResolvedVideoDecision
	candidate.FilterPlanIdentity = qsvDescriptorFilterIdentity(descriptor)
	candidate.NormalizationIdentity = fmt.Sprintf("%v", descriptor.Normalization)
	candidate.ReferenceDomain = "same-source-window"
	identityPayload, _ := json.Marshal(struct {
		Window     string
		Source     string
		Request    string
		Artifact   string
		Generation string
		Signature  string
	}{windowIdentity, sourceRevision, requestID, descriptor.ArtifactID, descriptor.GenerationToken, descriptor.CanonicalEncodeSignature})
	digest := sha256.Sum256(identityPayload)
	candidate.CandidateIdentity = "qsv-candidate:" + hex.EncodeToString(digest[:12])
	return candidate
}

func populateQSVCandidateTradeoffs(candidate *QSVExperimentCandidate, descriptor *compatiblePreviewDescriptor) {
	if candidate == nil || descriptor == nil || strings.TrimSpace(descriptor.CachePath) == "" {
		return
	}
	if info, err := os.Stat(descriptor.CachePath); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
		candidate.SizeBytes = info.Size()
	}
	if streams, err := probeMediaStreams(descriptor.CachePath); err == nil && len(streams.Video) > 0 && streams.Video[0].Bitrate > 0 {
		candidate.Bitrate = float64(streams.Video[0].Bitrate)
	}
}

func qsvDescriptorWorkerValue(descriptor *compatiblePreviewDescriptor, key string) string {
	if descriptor == nil || descriptor.EffectivePreviewProfile == nil {
		return ""
	}
	return workerStringValue(descriptor.EffectivePreviewProfile.WorkerConfig[key])
}

func qsvDescriptorQSVSettings(descriptor *compatiblePreviewDescriptor, effective bool) models.JSONMap {
	settings := models.JSONMap{}
	if descriptor == nil {
		return settings
	}
	request := descriptor.Request
	settings["globalQuality"] = request.GlobalQuality
	settings["qsvRateControl"] = request.QSVRateControl
	settings["qsvLookAheadDepth"] = request.QSVLookAheadDepth
	settings["qsvExtendedBRC"] = request.QSVExtendedBRC
	settings["qsvAdaptiveI"] = request.QSVAdaptiveI
	settings["qsvAdaptiveB"] = request.QSVAdaptiveB
	settings["qsvMBBRCMode"] = request.QSVMBBRCMode
	settings["qsvRDOMode"] = request.QSVRDOMode
	settings["qsvPStrategy"] = request.QSVPStrategy
	if descriptor.EffectivePreviewProfile != nil {
		for _, key := range []string{"globalQuality", "qsvRequestedGlobalQuality", "qsvEffectiveGlobalQuality", "qsvRateControl", "qsvRequestedRateControl", "qsvEffectiveRateControl", "qsvLookAheadDepth", "qsvExtendedBRC", "qsvAdaptiveI", "qsvAdaptiveB", "qsvMBBRCMode", "qsvRDOMode", "qsvPStrategy", "qsvRateControlFallbackReason"} {
			if value, ok := descriptor.EffectivePreviewProfile.WorkerConfig[key]; ok {
				settings[key] = value
			}
		}
	}
	if effective {
		settings["effectiveEncoder"] = descriptor.EffectiveVideoEncoder
		settings["effectiveQSVRateControl"] = descriptor.EffectiveQSVRateControl
	} else {
		settings["requestedEncoder"] = descriptor.RequestedVideoEncoder
		settings["requestedQSVRateControl"] = descriptor.RequestedQSVRateControl
	}
	for key, value := range qsvDescriptorFrameSettings(descriptor, effective) {
		settings[key] = value
	}
	for key, value := range qsvDescriptorBFrameSettings(descriptor, effective) {
		settings[key] = value
	}
	settings["filterPlanIdentity"] = qsvDescriptorFilterIdentity(descriptor)
	settings["effectiveVideoPlan"] = descriptor.ResolvedVideoDecision
	settings["normalizationIdentity"] = fmt.Sprintf("%v", descriptor.Normalization)
	if descriptor.Request.SubtitleStreamIndex != nil {
		settings["subtitleStreamIndex"] = *descriptor.Request.SubtitleStreamIndex
	} else {
		settings["subtitleStreamIndex"] = -1
	}
	settings["audioSubtitlePlan"] = "audio_disabled_subtitles_frozen"
	return settings
}

func qsvDescriptorFrameSettings(descriptor *compatiblePreviewDescriptor, effective bool) models.JSONMap {
	settings := models.JSONMap{}
	if descriptor == nil || descriptor.EffectivePreviewProfile == nil {
		return settings
	}
	config := descriptor.EffectivePreviewProfile.WorkerConfig
	for _, key := range []string{"frameStructureGopMode", "frameStructureGopFrames", "frameStructureGopSeconds", "frameStructureGopFrozen", "frameStructureAutoResolved"} {
		if value, ok := config[key]; ok {
			settings[key] = value
		}
	}
	if !effective {
		settings["requested"] = settings["frameStructureGopFrames"]
	}
	return settings
}

func qsvDescriptorBFrameSettings(descriptor *compatiblePreviewDescriptor, effective bool) models.JSONMap {
	settings := models.JSONMap{}
	if descriptor == nil || descriptor.EffectivePreviewProfile == nil {
		return settings
	}
	config := descriptor.EffectivePreviewProfile.WorkerConfig
	for _, key := range []string{"frameStructureBFrameMode", "frameStructureMaxBFrames", "qsvAdaptiveB", "qsvAdaptiveI", "qsvPStrategy"} {
		if value, ok := config[key]; ok {
			settings[key] = value
		}
	}
	if !effective {
		settings["requested"] = settings["frameStructureMaxBFrames"]
	}
	return settings
}

func qsvDescriptorFilterIdentity(descriptor *compatiblePreviewDescriptor) string {
	if descriptor == nil {
		return ""
	}
	filter := ""
	if descriptor.EffectivePreviewProfile != nil {
		filter = existingVideoFilters(*descriptor.EffectivePreviewProfile)
	}
	payload, _ := json.Marshal(struct {
		Filter        string
		Normalization previewNormalizationDecision
	}{filter, descriptor.Normalization})
	digest := sha256.Sum256(payload)
	return "filter:" + hex.EncodeToString(digest[:12])
}

func qsvTemporalVariantPreviewRequest(base *compatiblePreviewDescriptor, target QSVTemporalTarget, variant QSVTemporalVariant) (compatiblePreviewRequest, error) {
	if base == nil {
		return compatiblePreviewRequest{}, errors.New("qsv temporal baseline descriptor is missing")
	}
	window := labFidelityWindowPlan{Start: target.Interval.StartSeconds, Duration: target.Interval.DurationSeconds, Identity: target.WindowIdentity, Primary: target.Primary, Roles: target.Roles, Reasons: target.Reasons}
	request := labFidelityAdditionalWindowRequest(base, window)
	if target.Primary {
		request = base.Request
		request.Start = base.Start
		request.Seconds = base.Seconds
	}
	request.ProfileID = 0
	request.Ephemeral = false
	if request.Profile != nil {
		request.Profile = cloneCompatiblePreviewProfile(request.Profile)
		request.Profile.WorkerConfig = cloneWorkerConfig(request.Profile.WorkerConfig)
	}
	delta := variant.RequestedDelta
	for key, value := range delta {
		switch strings.ToLower(strings.TrimSpace(variant.ChangedVariable)) {
		case QSVTemporalChangedQuality:
			switch key {
			case "globalQuality", "qsvGlobalQuality":
				request.GlobalQuality = int(qsvDeltaNumber(value, float64(request.GlobalQuality)))
				if request.Profile != nil {
					request.Profile.WorkerConfig["globalQuality"] = request.GlobalQuality
				}
			case "qsvRateControl", "rateControl":
				request.QSVRateControl = fmt.Sprint(value)
				if request.Profile != nil {
					request.Profile.WorkerConfig["qsvRateControl"] = request.QSVRateControl
				}
			default:
				return compatiblePreviewRequest{}, fmt.Errorf("quality variant field %q is not supported", key)
			}
		case QSVTemporalChangedGOP:
			if request.Profile == nil {
				return compatiblePreviewRequest{}, errors.New("GOP variants require an effective profile")
			}
			switch key {
			case "gopFrames", "frameStructureGopFrames":
				request.Profile.WorkerConfig["frameStructureGopMode"] = "custom"
				request.Profile.WorkerConfig["frameStructureGopFrames"] = int(qsvDeltaNumber(value, 0))
			case "gopMode", "frameStructureGopMode":
				request.Profile.WorkerConfig["frameStructureGopMode"] = fmt.Sprint(value)
			default:
				return compatiblePreviewRequest{}, fmt.Errorf("GOP variant field %q is not supported", key)
			}
		case QSVTemporalChangedBFrames:
			if request.Profile == nil {
				return compatiblePreviewRequest{}, errors.New("B-frame variants require an effective profile")
			}
			switch key {
			case "bf", "maxBFrames", "frameStructureMaxBFrames":
				request.Profile.WorkerConfig["frameStructureBFrameMode"] = "custom"
				request.Profile.WorkerConfig["frameStructureMaxBFrames"] = int(qsvDeltaNumber(value, 0))
			case "adaptiveB", "qsvAdaptiveB":
				request.Profile.WorkerConfig["qsvAdaptiveB"] = qsvDeltaBool(value)
			default:
				return compatiblePreviewRequest{}, fmt.Errorf("B-frame variant field %q is not supported", key)
			}
		case QSVTemporalChangedQSVFeature:
			switch key {
			case "lookAheadDepth", "qsvLookAheadDepth":
				request.QSVLookAheadDepth = int(qsvDeltaNumber(value, float64(request.QSVLookAheadDepth)))
				if request.Profile != nil {
					request.Profile.WorkerConfig["qsvLookAheadDepth"] = request.QSVLookAheadDepth
				}
			case "extendedBRC", "qsvExtendedBRC":
				request.QSVExtendedBRC = qsvDeltaBool(value)
				if request.Profile != nil {
					request.Profile.WorkerConfig["qsvExtendedBRC"] = request.QSVExtendedBRC
				}
			case "adaptiveI", "qsvAdaptiveI":
				request.QSVAdaptiveI = qsvDeltaBool(value)
				if request.Profile != nil {
					request.Profile.WorkerConfig["qsvAdaptiveI"] = request.QSVAdaptiveI
				}
			case "adaptiveB", "qsvAdaptiveB":
				request.QSVAdaptiveB = qsvDeltaBool(value)
				if request.Profile != nil {
					request.Profile.WorkerConfig["qsvAdaptiveB"] = request.QSVAdaptiveB
				}
			case "mbbrcMode", "qsvMBBRCMode":
				request.QSVMBBRCMode = fmt.Sprint(value)
				if request.Profile != nil {
					request.Profile.WorkerConfig["qsvMBBRCMode"] = request.QSVMBBRCMode
				}
			case "rdoMode", "qsvRDOMode":
				request.QSVRDOMode = fmt.Sprint(value)
				if request.Profile != nil {
					request.Profile.WorkerConfig["qsvRDOMode"] = request.QSVRDOMode
				}
			case "pStrategy", "qsvPStrategy":
				request.QSVPStrategy = int(qsvDeltaNumber(value, float64(request.QSVPStrategy)))
				if request.Profile != nil {
					request.Profile.WorkerConfig["qsvPStrategy"] = request.QSVPStrategy
				}
			default:
				return compatiblePreviewRequest{}, fmt.Errorf("QSV feature field %q is not supported", key)
			}
		default:
			return compatiblePreviewRequest{}, fmt.Errorf("unsupported qsv temporal variable %q", variant.ChangedVariable)
		}
	}
	return request, nil
}

func qsvDeltaNumber(value any, fallback float64) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, _ := typed.Float64()
		return parsed
	default:
		var parsed float64
		if _, err := fmt.Sscan(fmt.Sprint(value), &parsed); err == nil {
			return parsed
		}
	}
	return fallback
}

func qsvDeltaBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(typed, "true")
	}
	return qsvDeltaNumber(value, 0) != 0
}

// ValidateQSVEffectiveDelta compares only effective settings. Requested
// values remain evidence and cannot make an effective fallback look isolated.
func ValidateQSVEffectiveDelta(baseline, variant QSVExperimentCandidate, declaredVariable string, allowedDerived []string) QSVIsolationResult {
	base := normalizeQSVSettings(baseline.EffectiveQSVSettings)
	next := normalizeQSVSettings(variant.EffectiveQSVSettings)
	keys := make(map[string]bool, len(base)+len(next))
	for key := range base {
		keys[key] = true
	}
	for key := range next {
		keys[key] = true
	}
	allowed := map[string]bool{}
	for _, field := range qsvVariableFields(declaredVariable) {
		allowed[field] = true
	}
	for _, field := range allowedDerived {
		allowed[field] = true
	}
	changed := []string{}
	unexpected := []string{}
	for key := range keys {
		if !jsonValuesEqual(base[key], next[key]) {
			changed = append(changed, key)
			if !allowed[key] {
				unexpected = append(unexpected, key)
			}
		}
	}
	sort.Strings(changed)
	sort.Strings(unexpected)
	result := QSVIsolationValid
	reason := "only the declared effective variable changed"
	if len(unexpected) > 0 || len(changed) == 0 {
		result = QSVIsolationInvalid
		if len(changed) == 0 {
			reason = "the effective settings did not change"
		} else {
			reason = "effective settings changed outside the declared variable"
		}
	}
	return QSVIsolationResult{Result: result, DeclaredVariable: declaredVariable, AllowedDerived: append([]string(nil), allowedDerived...), ChangedFields: changed, UnexpectedFields: unexpected, Reason: reason}
}

func qsvVariableFields(variable string) []string {
	switch strings.ToLower(strings.TrimSpace(variable)) {
	case QSVTemporalChangedQuality:
		return []string{"globalQuality", "qsvGlobalQuality", "qsvRateControl", "effectiveQSVRateControl", "qsvEffectiveRateControl"}
	case QSVTemporalChangedGOP:
		return []string{"gopFrames", "gopSeconds", "frameStructureGopFrames", "frameStructureGopSeconds", "gopMode"}
	case QSVTemporalChangedBFrames:
		return []string{"bf", "maxBFrames", "adaptiveB", "qsvAdaptiveB"}
	case QSVTemporalChangedQSVFeature:
		return []string{"lookAheadDepth", "qsvLookAheadDepth", "extendedBRC", "qsvExtendedBRC", "mbbrcMode", "qsvMBBRCMode", "adaptiveI", "adaptiveB", "qsvAdaptiveI", "qsvAdaptiveB", "rdoMode", "qsvRDOMode", "pStrategy", "qsvPStrategy"}
	default:
		return []string{strings.TrimSpace(variable)}
	}
}

func normalizeQSVSettings(settings models.JSONMap) models.JSONMap {
	if settings == nil {
		return models.JSONMap{}
	}
	return settings
}

func jsonValuesEqual(left, right any) bool {
	leftBytes, _ := json.Marshal(left)
	rightBytes, _ := json.Marshal(right)
	return string(leftBytes) == string(rightBytes)
}

type QSVTemporalEvidenceProvider interface {
	Measure(context.Context, QSVTemporalEvidenceRequest) (QSVTemporalEvidence, error)
}

var qsvTemporalEvidenceSeam = struct {
	sync.RWMutex
	provider QSVTemporalEvidenceProvider
}{provider: DeterministicQSVTemporalEvidenceProvider{Policy: DefaultQSVTemporalInterpretationPolicyV1()}}

func qsvTemporalEvidenceProvider() QSVTemporalEvidenceProvider {
	qsvTemporalEvidenceSeam.RLock()
	defer qsvTemporalEvidenceSeam.RUnlock()
	return qsvTemporalEvidenceSeam.provider
}

func setQSVTemporalEvidenceProviderForTest(provider QSVTemporalEvidenceProvider) func() {
	qsvTemporalEvidenceSeam.Lock()
	previous := qsvTemporalEvidenceSeam.provider
	qsvTemporalEvidenceSeam.provider = provider
	qsvTemporalEvidenceSeam.Unlock()
	return func() {
		qsvTemporalEvidenceSeam.Lock()
		qsvTemporalEvidenceSeam.provider = previous
		qsvTemporalEvidenceSeam.Unlock()
	}
}

// DeterministicQSVTemporalEvidenceProvider consumes bounded observations from
// an existing artifact/evidence adapter. It never opens media or starts an
// encoder; production adapters own frame acquisition.
type DeterministicQSVTemporalEvidenceProvider struct {
	Policy QSVTemporalInterpretationPolicy
}

type QSVTemporalInterpretationPolicy struct {
	Version                   string  `json:"version"`
	MinimumComparableFrames   int     `json:"minimumComparableFrames"`
	LossThreshold             float64 `json:"lossThreshold"`
	RecoveryThreshold         float64 `json:"recoveryThreshold"`
	MinimumLossRecoveryEvents int     `json:"minimumLossRecoveryEvents"`
}

func DefaultQSVTemporalInterpretationPolicyV1() QSVTemporalInterpretationPolicy {
	return QSVTemporalInterpretationPolicy{Version: qsvTemporalInterpretationPolicyVersion, MinimumComparableFrames: 3, LossThreshold: 0.15, RecoveryThreshold: 0.10, MinimumLossRecoveryEvents: 1}
}

func (p DeterministicQSVTemporalEvidenceProvider) Measure(ctx context.Context, request QSVTemporalEvidenceRequest) (QSVTemporalEvidence, error) {
	if err := ctx.Err(); err != nil {
		return QSVTemporalEvidence{}, err
	}
	policy := p.Policy
	if policy.Version == "" {
		policy = DefaultQSVTemporalInterpretationPolicyV1()
	}
	sampling := DefaultQSVTemporalSamplingPolicyV1()
	if strings.TrimSpace(request.SamplingPolicyVersion) != "" {
		sampling.Version = request.SamplingPolicyVersion
	}
	if request.SamplingRateHz > 0 {
		sampling.RateHz = request.SamplingRateHz
	}
	baseline := boundedTemporalFrames(request.BaselineFrames)
	variant := boundedTemporalFrames(request.VariantFrames)
	reference := boundedTemporalFrames(request.ReferenceFrames)
	if len(reference) == 0 {
		// Injectable unit-test fixtures may omit a source sequence. In that
		// case the baseline remains the comparison reference for the variant,
		// while the baseline signal is conservatively treated as stable.
		reference = append([]QSVTemporalFrameSample(nil), baseline...)
	}
	evidence := QSVTemporalEvidence{
		SchemaVersion: qsvTemporalEvidenceSchemaVersion, TargetWindowIdentity: request.TargetWindowIdentity,
		VariantIdentity: request.VariantIdentity, SourceRevision: request.SourceRevision,
		BaselineArtifactID: request.BaselineArtifactID, VariantArtifactID: request.VariantArtifactID,
		SampleStartSeconds: request.SampleStartSeconds, SampleDurationSeconds: request.SampleDurationSeconds,
		NormalizationIdentity: request.NormalizationIdentity, RegionIdentity: request.RegionIdentity,
		SamplingPolicyVersion: sampling.Version, SamplingRateHz: sampling.RateHz,
		InterpretationPolicyVersion: policy.Version,
	}
	if !temporalSamplingContractMatches(append(append(append([]QSVTemporalFrameSample{}, baseline...), variant...), reference...), sampling) {
		evidence.Availability, evidence.Interpretation = "incompatible", QSVTemporalInterpretationInconclusive
		evidence.BaselineInterpretation, evidence.VariantInterpretation = QSVTemporalInterpretationInconclusive, QSVTemporalInterpretationInconclusive
		evidence.MitigationDirection = QSVTemporalMitigationInconclusive
		evidence.Warnings = []string{"baseline, variant, and reference frames do not share the frozen sampling contract"}
		return evidence, nil
	}
	baselineMeasurement, baselineSufficient := measureQSVTemporalSequence(baseline, reference, policy)
	variantMeasurement, variantSufficient := measureQSVTemporalSequence(variant, reference, policy)
	if !baselineSufficient || !variantSufficient {
		evidence.Availability, evidence.Interpretation = "insufficient", QSVTemporalInterpretationInconclusive
		evidence.BaselineInterpretation, evidence.VariantInterpretation = baselineMeasurement.Interpretation, variantMeasurement.Interpretation
		evidence.MitigationDirection = QSVTemporalMitigationInconclusive
		evidence.Warnings = []string{"baseline and variant require sufficient comparable temporal observations"}
		return evidence, nil
	}
	evidence.FrameCount = variantMeasurement.Summary.ComparableFrames
	evidence.DetailEnergyByFrame = variantMeasurement.DetailEnergy
	evidence.GrainEnergyByFrame = variantMeasurement.GrainEnergy
	evidence.BlurProxyByFrame = variantMeasurement.BlurProxy
	evidence.VariationByFrame = variantMeasurement.Variation
	evidence.LossRecoveryEvents = variantMeasurement.Events
	evidence.TemporalVariationSummary = variantMeasurement.Summary
	evidence.BaselineDetailEnergyByFrame = baselineMeasurement.DetailEnergy
	evidence.BaselineGrainEnergyByFrame = baselineMeasurement.GrainEnergy
	evidence.BaselineBlurProxyByFrame = baselineMeasurement.BlurProxy
	evidence.BaselineVariationByFrame = baselineMeasurement.Variation
	evidence.BaselineLossRecoveryEvents = baselineMeasurement.Events
	evidence.BaselineVariationSummary = baselineMeasurement.Summary
	evidence.BaselineInterpretation = baselineMeasurement.Interpretation
	evidence.VariantInterpretation = variantMeasurement.Interpretation
	evidence.Interpretation = variantMeasurement.Interpretation
	evidence.MitigationDirection = qsvTemporalMitigationDirection(baselineMeasurement.Interpretation, variantMeasurement.Interpretation)
	evidence.Availability = "measured"
	baselinePayload, _ := json.Marshal(struct {
		Frames         []QSVTemporalFrameSample
		Events         []QSVTemporalLossRecoveryEvent
		Interpretation string
	}{baseline, baselineMeasurement.Events, baselineMeasurement.Interpretation})
	baselineDigest := sha256.Sum256(baselinePayload)
	evidence.BaselineEvidenceIdentity = "temporal:baseline:" + hex.EncodeToString(baselineDigest[:12])
	variantPayload, _ := json.Marshal(struct {
		Frames         []QSVTemporalFrameSample
		Events         []QSVTemporalLossRecoveryEvent
		Interpretation string
	}{variant, variantMeasurement.Events, variantMeasurement.Interpretation})
	variantDigest := sha256.Sum256(variantPayload)
	evidence.VariantEvidenceIdentity = "temporal:variant:" + hex.EncodeToString(variantDigest[:12])
	payload, _ := json.Marshal(evidence)
	digest := sha256.Sum256(payload)
	evidence.EvidenceIdentity = "temporal:" + hex.EncodeToString(digest[:12])
	return evidence, nil
}

type qsvTemporalSequenceMeasurement struct {
	DetailEnergy   []float64
	GrainEnergy    []float64
	BlurProxy      []float64
	Variation      []float64
	Events         []QSVTemporalLossRecoveryEvent
	Summary        QSVTemporalVariationSummary
	Interpretation string
}

func measureQSVTemporalSequence(frames, reference []QSVTemporalFrameSample, policy QSVTemporalInterpretationPolicy) (qsvTemporalSequenceMeasurement, bool) {
	measurement := qsvTemporalSequenceMeasurement{Interpretation: QSVTemporalInterpretationInconclusive}
	if len(frames) != len(reference) || len(frames) < policy.MinimumComparableFrames {
		return measurement, false
	}
	losses := make([]float64, 0, len(frames))
	for index := range frames {
		if !frames[index].Comparable || !reference[index].Comparable {
			continue
		}
		referenceEnergy := temporalDetailEnergy(reference[index])
		outputEnergy := temporalDetailEnergy(frames[index])
		loss := 0.0
		if referenceEnergy > 0 {
			loss = (outputEnergy - referenceEnergy) / referenceEnergy
		}
		measurement.DetailEnergy = append(measurement.DetailEnergy, outputEnergy)
		measurement.GrainEnergy = append(measurement.GrainEnergy, frames[index].GrainEnergy)
		measurement.BlurProxy = append(measurement.BlurProxy, 1-math.Min(1, outputEnergy/math.Max(referenceEnergy, 1e-9)))
		measurement.Variation = append(measurement.Variation, math.Abs(loss))
		losses = append(losses, loss)
	}
	if len(losses) < policy.MinimumComparableFrames {
		return measurement, false
	}
	for index, loss := range losses {
		if loss > -policy.LossThreshold {
			continue
		}
		for recovery := index + 1; recovery < len(losses); recovery++ {
			if losses[recovery] >= policy.RecoveryThreshold {
				measurement.Events = append(measurement.Events, QSVTemporalLossRecoveryEvent{LossFrameIndex: index, RecoveryFrameIndex: recovery, LossMagnitude: -loss, RecoveryMagnitude: losses[recovery]})
				break
			}
		}
		if len(measurement.Events) >= qsvTemporalMaxEvents {
			break
		}
	}
	mean, maximum := 0.0, 0.0
	for _, value := range measurement.Variation {
		mean += value
		maximum = math.Max(maximum, value)
	}
	mean /= float64(len(measurement.Variation))
	measurement.Summary = QSVTemporalVariationSummary{ComparableFrames: len(losses), MeanAbsVariation: mean, MaximumVariation: maximum, LossEventCount: len(measurement.Events), RecoveryEventCount: len(measurement.Events)}
	if len(measurement.Events) >= policy.MinimumLossRecoveryEvents {
		measurement.Interpretation = QSVTemporalInterpretationLikelyPumping
	} else {
		measurement.Interpretation = QSVTemporalInterpretationNotObserved
	}
	return measurement, true
}

func qsvTemporalMitigationDirection(baseline, variant string) string {
	switch {
	case baseline == QSVTemporalInterpretationLikelyPumping && variant == QSVTemporalInterpretationNotObserved:
		return QSVTemporalMitigationImproved
	case baseline == QSVTemporalInterpretationNotObserved && variant == QSVTemporalInterpretationLikelyPumping:
		return QSVTemporalMitigationWorsened
	case baseline == QSVTemporalInterpretationLikelyPumping && variant == QSVTemporalInterpretationLikelyPumping:
		return QSVTemporalMitigationUnchanged
	case baseline == QSVTemporalInterpretationNotObserved && variant == QSVTemporalInterpretationNotObserved:
		return QSVTemporalMitigationUnchanged
	default:
		return QSVTemporalMitigationInconclusive
	}
}

func temporalSamplingContractMatches(frames []QSVTemporalFrameSample, policy QSVTemporalSamplingPolicy) bool {
	for _, frame := range frames {
		if frame.SamplingPolicyVersion != "" && frame.SamplingPolicyVersion != policy.Version {
			return false
		}
		if frame.SamplingRateHz > 0 && math.Abs(frame.SamplingRateHz-policy.RateHz) > 1e-9 {
			return false
		}
	}
	return true
}

func boundedTemporalFrames(frames []QSVTemporalFrameSample) []QSVTemporalFrameSample {
	if len(frames) <= qsvTemporalMaxFrames {
		return append([]QSVTemporalFrameSample(nil), frames...)
	}
	return append([]QSVTemporalFrameSample(nil), frames[:qsvTemporalMaxFrames]...)
}

func temporalDetailEnergy(sample QSVTemporalFrameSample) float64 {
	return math.Max(0, (sample.DetailEnergy+sample.GrainEnergy+sample.HighFrequencyEnergy+sample.LocalVariance)/4)
}

type QSVControlComparison struct {
	Result  string   `json:"result"`
	Reasons []string `json:"reasons,omitempty"`
}

func CompareQSVControl(target QSVTemporalTarget, control EncoderControlCandidate, candidate QSVExperimentCandidate) QSVControlComparison {
	reasons := []string{}
	encoderFamily := strings.ToLower(strings.TrimSpace(control.EncoderFamily))
	if encoderFamily == "" {
		reasons = append(reasons, "control encoder family is missing")
	} else if strings.Contains(encoderFamily, "qsv") {
		reasons = append(reasons, "control encoder is QSV and cannot provide a non-QSV control")
	}
	if control.WindowIdentity != target.WindowIdentity || !sameInterval(control.Interval, target.Interval) {
		reasons = append(reasons, "control window does not match target")
	}
	if control.SourceRevision == "" || control.SourceRevision != candidate.SourceRevision {
		reasons = append(reasons, "control source revision does not match")
	}
	if control.FilterPlanIdentity != candidate.FilterPlanIdentity {
		reasons = append(reasons, "control filter plan does not match")
	}
	if control.NormalizationIdentity == "" || control.NormalizationIdentity != candidate.NormalizationIdentity {
		reasons = append(reasons, "control normalization is missing or does not match")
	}
	if control.ReferenceDomain == "" || control.ReferenceDomain != candidate.ReferenceDomain {
		reasons = append(reasons, "control reference domain is missing or does not match")
	}
	if control.ArtifactIdentity == "" || control.GenerationIdentity == "" || control.EvidenceIdentity == "" || control.ArtifactGeneration == "" || control.ArtifactGeneration != control.GenerationIdentity {
		reasons = append(reasons, "control artifact, generation, or evidence identity is missing")
	}
	if control.ExpectedGenerationIdentity != "" && control.ExpectedGenerationIdentity != control.GenerationIdentity {
		reasons = append(reasons, "control generation identity is stale")
	}
	if control.ExpectedEvidenceIdentity != "" && control.ExpectedEvidenceIdentity != control.EvidenceIdentity {
		reasons = append(reasons, "control evidence identity is stale")
	}
	if len(control.EffectivePlan) == 0 {
		reasons = append(reasons, "control effective plan is missing")
	}
	if status := canonicalFidelityStatus(candidate.CanonicalFidelity); status != "passed" {
		reasons = append(reasons, fmt.Sprintf("QSV candidate Frame Fidelity is %s", status))
	}
	if status := canonicalFidelityStatus(control.CanonicalFidelity); status != "passed" {
		reasons = append(reasons, fmt.Sprintf("control Frame Fidelity is %s", status))
	}
	result := QSVControlComparable
	if len(reasons) > 0 {
		result = QSVControlNotComparable
	}
	return QSVControlComparison{Result: result, Reasons: reasons}
}

func canonicalFidelityStatus(fidelity models.JSONMap) string {
	if fidelity == nil {
		return "unverified"
	}
	status := strings.ToLower(strings.TrimSpace(stringFromUnknown(fidelity["status"])))
	if status == "" {
		return "unverified"
	}
	return status
}

func sameInterval(left, right QSVTemporalInterval) bool {
	return math.Abs(left.StartSeconds-right.StartSeconds) <= 1.0/30.0 && math.Abs((left.StartSeconds+left.DurationSeconds)-(right.StartSeconds+right.DurationSeconds)) <= 1.0/30.0
}

func ClassifyQSVTemporalTarget(target QSVTemporalTarget, controlResult QSVControlComparison) string {
	qsvEvidence, sufficient, valid := false, false, true
	for _, variant := range target.Variants {
		if variant.Isolation.Result != QSVIsolationValid {
			valid = false
			continue
		}
		if canonicalFidelityStatus(target.Baseline.CanonicalFidelity) != "passed" || canonicalFidelityStatus(variant.Candidate.CanonicalFidelity) != "passed" {
			valid = false
			continue
		}
		for _, evidence := range variant.TemporalEvidence {
			if evidence.Availability == "measured" && evidence.BaselineInterpretation != QSVTemporalInterpretationInconclusive && evidence.VariantInterpretation != QSVTemporalInterpretationInconclusive {
				sufficient = true
			}
			if evidence.BaselineInterpretation == QSVTemporalInterpretationLikelyPumping || evidence.VariantInterpretation == QSVTemporalInterpretationLikelyPumping || evidence.Interpretation == QSVTemporalInterpretationLikelyPumping {
				qsvEvidence = true
			}
			if evidence.Availability != "measured" {
				valid = false
			}
		}
	}
	if !valid || !sufficient {
		return QSVTemporalClassificationInconclusive
	}
	if !qsvEvidence {
		return QSVTemporalClassificationNotObserved
	}
	if controlResult.Result != QSVControlComparable {
		return QSVTemporalClassificationMeasurableNotSpecific
	}
	for _, control := range target.ControlCandidates {
		if control.Comparable == QSVControlComparable && (control.TemporalInterpretation == QSVTemporalInterpretationNotObserved || control.TemporalInterpretation == "materially_reduced") {
			return QSVTemporalClassificationReproducibleQVSSpecific
		}
	}
	return QSVTemporalClassificationMeasurableNotSpecific
}

func (e QSVTemporalExperiment) Validate() error {
	if e.SchemaVersion != "" && e.SchemaVersion != qsvTemporalExperimentSchemaVersion {
		return errors.New("unsupported qsv temporal experiment schema")
	}
	if err := ValidateQSVTemporalExperimentBounds(e); err != nil {
		return err
	}
	if e.ExperimentID != "" && e.ExperimentID != ComputeQSVTemporalExperimentID(e) {
		return errors.New("qsv temporal experiment identity is not deterministic")
	}
	return nil
}

func (h AssetHandler) executeQSVTemporalExperiment(ctx context.Context, input labFidelityOperationInput, id string, initialDescriptor *compatiblePreviewDescriptor, initialExperiment *QSVTemporalExperiment) {
	_ = input
	if initialExperiment == nil || initialDescriptor == nil {
		return
	}
	experiment := cloneQSVTemporalExperiment(*initialExperiment)
	operation, ok := labFidelityOperations.copy(id)
	if !ok {
		return
	}
	basePlan := operation.samplingPlan
	completed := make([]models.JSONMap, 0, len(experiment.Targets))
	for targetIndex := range experiment.Targets {
		if err := ctx.Err(); err != nil {
			return
		}
		target := &experiment.Targets[targetIndex]
		labFidelityOperations.update(id, func(operation *LabFidelityOperation) {
			operation.Phase = "qsv_temporal_target_baseline"
			operation.CurrentStart = target.Interval.StartSeconds
			operation.CurrentDuration = target.Interval.DurationSeconds
			operation.TotalWindows = len(experiment.Targets)
		})

		window := labFidelityWindowPlan{Position: float64(targetIndex), Start: target.Interval.StartSeconds, Duration: target.Interval.DurationSeconds, Primary: target.Primary, Identity: target.WindowIdentity, Roles: target.Roles, Reasons: target.Reasons}
		baselineDescriptor := initialDescriptor
		if !target.Primary {
			request := labFidelityAdditionalWindowRequest(initialDescriptor, window)
			requestID, candidateDescriptor, err := h.ensureLabCompatiblePreviewRequest(request)
			if err != nil {
				qsvTemporalOperationFailure(id, err, "qsv_temporal_baseline")
				return
			}
			_ = requestID
			baselineDescriptor = candidateDescriptor
			if baselineDescriptor.ArtifactID == initialDescriptor.ArtifactID || baselineDescriptor.CanonicalEncodeSignature == initialDescriptor.CanonicalEncodeSignature {
				qsvTemporalOperationFailure(id, errors.New("non-primary target resolved to primary Compatible Preview artifact"), "invalid_target_baseline")
				return
			}
		}
		baselineCandidate := qsvExperimentCandidateFromDescriptor(baselineDescriptor, baselineDescriptor.requestID, target.WindowIdentity, basePlan.SourceRevision)
		if target.Primary && baselineCandidate.ArtifactID != initialExperiment.Targets[0].Baseline.ArtifactID {
			qsvTemporalOperationFailure(id, errors.New("primary Compatible Preview generation changed before QSV experiment"), "stale_primary")
			return
		}
		target.Baseline = baselineCandidate
		baselineEvidence, err := h.collectLabFidelityWindow(ctx, baselineDescriptor, window)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			qsvTemporalOperationFailure(id, err, "qsv_temporal_baseline")
			return
		}
		completed = append(completed, baselineEvidence)
		target.Baseline.CanonicalFidelity = unknownRecord(baselineEvidence["canonicalFidelity"])
		populateQSVCandidateTradeoffs(&target.Baseline, baselineDescriptor)

		for variantIndex := range target.Variants {
			if err := ctx.Err(); err != nil {
				return
			}
			variant := &target.Variants[variantIndex]
			capability := QSVTemporalVariantCapability(baselineDescriptor, *variant, capabilities.CheckEncoder("hevc_qsv"))
			if !capability.Supported {
				variant.Isolation = QSVIsolationResult{Result: QSVIsolationNotComparable, DeclaredVariable: variant.ChangedVariable, Reason: capability.Reason}
				target.Warnings = append(target.Warnings, fmt.Sprintf("QSV variant %s unavailable before generation: %s", variant.ChangedVariable, capability.Reason))
				updateQSVTemporalOperation(id, &experiment, completed)
				continue
			}
			request, requestErr := qsvTemporalVariantPreviewRequest(baselineDescriptor, *target, *variant)
			if requestErr != nil {
				qsvTemporalOperationFailure(id, requestErr, "qsv_temporal_variant")
				return
			}
			requestID, variantDescriptor, requestErr := h.ensureLabCompatiblePreviewRequest(request)
			if requestErr != nil {
				qsvTemporalOperationFailure(id, requestErr, "qsv_temporal_variant")
				return
			}
			variantCandidate := qsvExperimentCandidateFromDescriptor(variantDescriptor, requestID, target.WindowIdentity, basePlan.SourceRevision)
			if variantCandidate.ArtifactID == baselineCandidate.ArtifactID || variantCandidate.CanonicalEncodeSignature == baselineCandidate.CanonicalEncodeSignature {
				variant.Isolation = QSVIsolationResult{Result: QSVIsolationInvalid, DeclaredVariable: variant.ChangedVariable, Reason: "variant did not produce a distinct Compatible Preview candidate"}
			} else {
				variant.Isolation = ValidateQSVEffectiveDelta(baselineCandidate, variantCandidate, variant.ChangedVariable, nil)
			}
			variant.EffectiveDelta = qsvEffectiveDeltaMap(baselineCandidate, variantCandidate, variant.Isolation.ChangedFields)
			variant.Candidate = variantCandidate
			variantEvidence, evidenceErr := h.collectLabFidelityWindow(ctx, variantDescriptor, window)
			if evidenceErr != nil {
				if ctx.Err() != nil {
					return
				}
				qsvTemporalOperationFailure(id, evidenceErr, "qsv_temporal_variant")
				return
			}
			completed = append(completed, variantEvidence)
			variant.Candidate.CanonicalFidelity = unknownRecord(variantEvidence["canonicalFidelity"])
			populateQSVCandidateTradeoffs(&variant.Candidate, variantDescriptor)
			evidence, temporalErr := qsvMeasureTemporalEvidence(ctx, target, *variant, baselineDescriptor, variantDescriptor)
			if temporalErr != nil {
				if ctx.Err() != nil {
					return
				}
				variant.TemporalEvidence = append(variant.TemporalEvidence, QSVTemporalEvidence{SchemaVersion: qsvTemporalEvidenceSchemaVersion, TargetWindowIdentity: target.WindowIdentity, VariantIdentity: variant.VariantIdentity, Availability: "unavailable", Interpretation: QSVTemporalInterpretationInconclusive, Warnings: []string{temporalErr.Error()}, InterpretationPolicyVersion: qsvTemporalInterpretationPolicyVersion})
			} else {
				variant.TemporalEvidence = append(variant.TemporalEvidence, evidence)
			}
			if perceptualProvider := qsvPerceptualEvidenceProvider(); perceptualProvider != nil {
				state, perceptualErr := AttachQSVPerceptualEvidence(ctx, target, variant, perceptualProvider, QSVPerceptualEvidenceLookup{SourceRevision: variant.Candidate.SourceRevision, SamplingPlanIdentity: experiment.SamplingPlanIdentity, WindowIdentity: target.WindowIdentity, ArtifactGeneration: qsvCandidateGeneration(variant.Candidate)})
				if perceptualErr != nil {
					target.Warnings = append(target.Warnings, fmt.Sprintf("#69 evidence %s: %v", state, perceptualErr))
				}
			} else {
				target.Warnings = append(target.Warnings, "#69 canonical perceptual evidence is unavailable; deterministic evidence remains authoritative")
			}
			if recommendation := qsvRecommendationForVariant(*target, *variant, variantCandidate); recommendation != nil {
				target.Recommendation = recommendation
			}
			updateQSVTemporalOperation(id, &experiment, completed)
		}
		for controlIndex := range target.ControlCandidates {
			control := &target.ControlCandidates[controlIndex]
			if control.ArtifactIdentity == "" {
				continue
			}
			entry, tombstone, known := compatiblePreviewRequestLookup(control.ArtifactIdentity, time.Now())
			if !known || tombstone != nil || entry == nil {
				control.Comparable = QSVControlNotComparable
				control.Warnings = append(control.Warnings, "control request is stale or unavailable")
				continue
			}
			controlDescriptor, controlErr := h.compatiblePreviewDescriptorForEntry(entry, control.ArtifactIdentity)
			if controlErr != nil || !compatiblePreviewPublishedArtifactValidForDescriptor(controlDescriptor) {
				control.Comparable = QSVControlNotComparable
				control.Warnings = append(control.Warnings, "control artifact is not ready")
				continue
			}
			if controlDescriptor.SourceRevision != baselineDescriptor.SourceRevision {
				control.Comparable = QSVControlNotComparable
				control.Warnings = append(control.Warnings, "control source revision is stale or does not match the frozen baseline")
				continue
			}
			control.EncoderFamily = controlDescriptor.EffectiveVideoEncoder
			control.SourceRevision = basePlan.SourceRevision
			control.Interval = QSVTemporalInterval{StartSeconds: float64(previewTimestampSeconds(controlDescriptor.Start)), DurationSeconds: float64(max(1, controlDescriptor.Seconds))}
			control.ArtifactIdentity = controlDescriptor.ArtifactID
			if control.RequestID == "" {
				control.RequestID = control.ArtifactIdentity
			}
			control.ArtifactGeneration = fmt.Sprintf("%s:%d", controlDescriptor.GenerationToken, controlDescriptor.GenerationVersion)
			control.GenerationIdentity = control.ArtifactGeneration
			control.EffectivePlan = controlDescriptor.ResolvedVideoDecision
			control.FilterPlanIdentity = qsvDescriptorFilterIdentity(controlDescriptor)
			control.NormalizationIdentity = fmt.Sprintf("%v", controlDescriptor.Normalization)
			control.ReferenceDomain = "same-source-window"
			control.EvidenceIdentity = controlDescriptor.CanonicalEncodeSignature
			controlEvidenceSummary, controlEvidenceErr := h.collectLabFidelityWindow(ctx, controlDescriptor, window)
			if controlEvidenceErr != nil {
				control.Comparable = QSVControlNotComparable
				control.Warnings = append(control.Warnings, fmt.Sprintf("control Frame Fidelity evidence unavailable: %v", controlEvidenceErr))
				continue
			}
			control.CanonicalFidelity = unknownRecord(controlEvidenceSummary["canonicalFidelity"])
			comparison := CompareQSVControl(*target, *control, baselineCandidate)
			control.Comparable = comparison.Result
			control.Warnings = append(control.Warnings, comparison.Reasons...)
			if control.Comparable == QSVControlComparable {
				controlVariant := QSVTemporalVariant{VariantIdentity: "control:" + control.ArtifactIdentity, Candidate: qsvExperimentCandidateFromDescriptor(controlDescriptor, control.ArtifactIdentity, target.WindowIdentity, basePlan.SourceRevision)}
				controlVariant.Candidate.CanonicalFidelity = control.CanonicalFidelity
				controlEvidence, controlEvidenceErr := qsvMeasureTemporalEvidence(ctx, target, controlVariant, baselineDescriptor, controlDescriptor)
				if controlEvidenceErr != nil {
					control.Warnings = append(control.Warnings, controlEvidenceErr.Error())
				} else {
					control.TemporalInterpretation = controlEvidence.VariantInterpretation
				}
			}
		}
		controlResult := QSVControlComparison{Result: QSVControlNotComparable}
		for _, control := range target.ControlCandidates {
			if control.Comparable == QSVControlComparable {
				controlResult = QSVControlComparison{Result: QSVControlComparable}
				break
			}
		}
		target.Classification = ClassifyQSVTemporalTarget(*target, controlResult)
		updateQSVTemporalOperation(id, &experiment, completed)
	}
	experiment.ExperimentID = ComputeQSVTemporalExperimentID(experiment)
	labFidelityOperations.update(id, func(operation *LabFidelityOperation) {
		operation.TemporalExperiment = &experiment
		operation.Windows = append([]models.JSONMap(nil), completed...)
		operation.CompletedWindows = len(experiment.Targets)
		operation.TotalWindows = len(experiment.Targets)
		operation.Status = labFidelityCompleted
		operation.Phase = "qsv_temporal_complete"
		operation.CurrentStart, operation.CurrentDuration = 0, 0
		operation.Result = qsvTemporalExperimentResult(experiment, completed)
	})
}

func qsvEffectiveDeltaMap(baseline, variant QSVExperimentCandidate, fields []string) models.JSONMap {
	result := models.JSONMap{}
	for _, field := range fields {
		result[field] = models.JSONMap{"baseline": baseline.EffectiveQSVSettings[field], "variant": variant.EffectiveQSVSettings[field]}
	}
	return result
}

func cloneQSVTemporalExperiment(experiment QSVTemporalExperiment) QSVTemporalExperiment {
	encoded, _ := json.Marshal(experiment)
	var clone QSVTemporalExperiment
	_ = json.Unmarshal(encoded, &clone)
	return clone
}

func qsvTemporalOperationFailure(id string, err error, phase string) {
	labFidelityOperations.update(id, func(operation *LabFidelityOperation) {
		operation.Status, operation.Phase, operation.Error, operation.Retryable = labFidelityFailed, phase, err.Error(), true
	})
}

func updateQSVTemporalOperation(id string, experiment *QSVTemporalExperiment, completed []models.JSONMap) {
	copy := cloneQSVTemporalExperiment(*experiment)
	copy.ExperimentID = ComputeQSVTemporalExperimentID(copy)
	labFidelityOperations.update(id, func(operation *LabFidelityOperation) {
		operation.TemporalExperiment = &copy
		operation.Windows = append([]models.JSONMap(nil), completed...)
		operation.CompletedWindows = len(copy.Targets)
		operation.TotalWindows = len(copy.Targets)
		operation.Result = qsvTemporalExperimentResult(copy, completed)
	})
}

func qsvTemporalExperimentResult(experiment QSVTemporalExperiment, completed []models.JSONMap) models.JSONMap {
	return models.JSONMap{"qsvTemporalExperiment": experiment, "experimentId": experiment.ExperimentID, "classifications": qsvTemporalClassifications(experiment), "unavailableRoles": experiment.UnavailableRoles, "warnings": experiment.Warnings, "fidelityWindows": append([]models.JSONMap(nil), completed...), "completedAt": 0}
}

func qsvTemporalClassifications(experiment QSVTemporalExperiment) []string {
	result := make([]string, 0, len(experiment.Targets))
	for _, target := range experiment.Targets {
		result = append(result, target.Classification)
	}
	return result
}

func qsvMeasureTemporalEvidence(ctx context.Context, target *QSVTemporalTarget, variant QSVTemporalVariant, baseline, candidate *compatiblePreviewDescriptor) (QSVTemporalEvidence, error) {
	provider := qsvTemporalEvidenceProvider()
	if provider == nil {
		return QSVTemporalEvidence{}, errors.New("temporal evidence provider is unavailable")
	}
	sampling := DefaultQSVTemporalSamplingPolicyV1()
	request := QSVTemporalEvidenceRequest{
		TargetWindowIdentity: target.WindowIdentity, VariantIdentity: variant.VariantIdentity,
		SourceRevision: variant.Candidate.SourceRevision, BaselineArtifactID: baseline.ArtifactID,
		VariantArtifactID: candidate.ArtifactID, SampleStartSeconds: target.Interval.StartSeconds,
		SampleDurationSeconds: target.Interval.DurationSeconds, NormalizationIdentity: fmt.Sprintf("%v", candidate.Normalization),
		RegionIdentity: "bounded-textured-region", SamplingPolicyVersion: sampling.Version, SamplingRateHz: sampling.RateHz,
	}
	_, deterministic := provider.(DeterministicQSVTemporalEvidenceProvider)
	if _, pointerDeterministic := provider.(*DeterministicQSVTemporalEvidenceProvider); pointerDeterministic {
		deterministic = true
	}
	if deterministic {
		var err error
		// Compatible Preview artifacts already represent the exact frozen window;
		// sample them from their local zero rather than applying the source offset a
		// second time. The source reference below is the only input that uses the
		// absolute SamplingWindow offset.
		request.BaselineFrames, err = qsvTemporalFrameSamplesForWindow(ctx, baseline.CachePath, 0, target.Interval.DurationSeconds)
		if err != nil {
			return QSVTemporalEvidence{}, err
		}
		request.VariantFrames, err = qsvTemporalFrameSamplesForWindow(ctx, candidate.CachePath, 0, target.Interval.DurationSeconds)
		if err != nil {
			return QSVTemporalEvidence{}, err
		}
		request.ReferenceFrames, err = qsvTemporalFrameSamplesForWindow(ctx, baseline.Path, target.Interval.StartSeconds, target.Interval.DurationSeconds)
		if err != nil {
			return QSVTemporalEvidence{}, err
		}
	}
	return provider.Measure(ctx, request)
}

const (
	qsvTemporalFrameWidth  = 64
	qsvTemporalFrameHeight = 36
	qsvTemporalFrameLimit  = 16
)

func qsvTemporalFrameSamples(ctx context.Context, path string) ([]QSVTemporalFrameSample, error) {
	return qsvTemporalFrameSamplesForWindow(ctx, path, 0, 0)
}

func qsvTemporalFrameSamplesForWindow(ctx context.Context, path string, startSeconds, durationSeconds float64) ([]QSVTemporalFrameSample, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("temporal artifact path is unavailable")
	}
	sampling := DefaultQSVTemporalSamplingPolicyV1()
	frameBytes := qsvTemporalFrameWidth * qsvTemporalFrameHeight
	args := []string{"-hide_banner", "-loglevel", "error"}
	if startSeconds > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.6f", startSeconds))
	}
	args = append(args, "-i", path)
	if durationSeconds > 0 {
		args = append(args, "-t", fmt.Sprintf("%.6f", durationSeconds))
	}
	args = append(args, "-vf", sampling.Filter, "-frames:v", fmt.Sprint(qsvTemporalFrameLimit), "-f", "rawvideo", "-pix_fmt", "gray", "pipe:1")
	command := exec.CommandContext(ctx, "ffmpeg", args...)
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("bounded temporal frame extraction: %w", err)
	}
	count := min(qsvTemporalFrameLimit, len(output)/frameBytes)
	if count < 3 {
		return nil, errors.New("bounded temporal frame extraction returned insufficient frames")
	}
	frames := make([]QSVTemporalFrameSample, 0, count)
	for index := 0; index < count; index++ {
		pixels := output[index*frameBytes : (index+1)*frameBytes]
		mean := 0.0
		for _, pixel := range pixels {
			mean += float64(pixel) / 255
		}
		mean /= float64(len(pixels))
		variance, detail := 0.0, 0.0
		for y := 0; y < qsvTemporalFrameHeight; y++ {
			for x := 0; x < qsvTemporalFrameWidth; x++ {
				value := float64(pixels[y*qsvTemporalFrameWidth+x]) / 255
				variance += (value - mean) * (value - mean)
				if x > 0 {
					detail += math.Abs(value - float64(pixels[y*qsvTemporalFrameWidth+x-1])/255)
				}
			}
		}
		pixelCount := float64(len(pixels))
		frames = append(frames, QSVTemporalFrameSample{FrameIndex: index, TimestampSeconds: float64(index) / sampling.RateHz, Comparable: true, RegionIdentity: "bounded-textured-region", DetailEnergy: detail / pixelCount, GrainEnergy: variance / pixelCount, HighFrequencyEnergy: detail / pixelCount, LocalVariance: variance / pixelCount, SamplingPolicyVersion: sampling.Version, SamplingRateHz: sampling.RateHz})
	}
	return frames, nil
}

func qsvRecommendationForVariant(target QSVTemporalTarget, variant QSVTemporalVariant, candidate QSVExperimentCandidate) *QSVPumpingMitigationRecommendation {
	if variant.Isolation.Result != QSVIsolationValid || canonicalFidelityStatus(target.Baseline.CanonicalFidelity) != "passed" || canonicalFidelityStatus(candidate.CanonicalFidelity) != "passed" {
		return nil
	}
	if len(variant.TemporalEvidence) == 0 {
		return nil
	}
	evidence := variant.TemporalEvidence[len(variant.TemporalEvidence)-1]
	if evidence.Availability != "measured" || evidence.BaselineInterpretation != QSVTemporalInterpretationLikelyPumping || evidence.VariantInterpretation != QSVTemporalInterpretationNotObserved || evidence.MitigationDirection != QSVTemporalMitigationImproved {
		return nil
	}
	return &QSVPumpingMitigationRecommendation{
		Version: qsvTemporalExperimentPolicyVersion, ChangedVariable: variant.ChangedVariable,
		EffectiveSetting: candidate.EffectiveQSVSettings, EvidenceReferences: temporalEvidenceReferences(variant.TemporalEvidence),
		Encoder: "hevc_qsv", Runtime: candidate.RuntimeCapabilityIdentity, MediaRoles: append([]string(nil), target.Roles...),
		Confidence: "bounded_temporal_evidence_baseline_to_variant", Tradeoffs: qsvRecommendationTradeoffs(target.Baseline, candidate),
		ControlAttribution: QSVTemporalClassificationMeasurableNotSpecific,
	}
}

func qsvRecommendationTradeoffs(baseline, variant QSVExperimentCandidate) models.JSONMap {
	tradeoffs := models.JSONMap{}
	if baseline.SizeBytes > 0 && variant.SizeBytes > 0 {
		tradeoffs["size"] = models.JSONMap{"baseline": baseline.SizeBytes, "variant": variant.SizeBytes, "delta": variant.SizeBytes - baseline.SizeBytes}
	}
	if baseline.Bitrate > 0 && variant.Bitrate > 0 {
		tradeoffs["bitrate"] = models.JSONMap{"baseline": baseline.Bitrate, "variant": variant.Bitrate, "delta": variant.Bitrate - baseline.Bitrate}
	}
	if baseline.Speed > 0 && variant.Speed > 0 {
		tradeoffs["speed"] = models.JSONMap{"baseline": baseline.Speed, "variant": variant.Speed, "delta": variant.Speed - baseline.Speed}
	}
	return tradeoffs
}

func temporalEvidenceReferences(evidence []QSVTemporalEvidence) []string {
	result := make([]string, 0, len(evidence))
	for _, item := range evidence {
		for _, identity := range []string{item.EvidenceIdentity, item.BaselineEvidenceIdentity, item.VariantEvidenceIdentity} {
			if identity != "" && !containsSamplingString(result, identity) {
				result = append(result, identity)
			}
		}
	}
	return result
}
