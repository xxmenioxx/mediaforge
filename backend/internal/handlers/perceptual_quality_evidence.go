package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

// These versions are persisted with the evidence. They are deliberately
// separate from the LAB operation version: an operation retry must not change
// material evidence identity.
const (
	perceptualEvidenceSchemaVersion    = "perceptual-evidence-v1"
	perceptualEvidencePolicyVersion    = "perceptual-quality-policy-v1"
	perceptualMetricContractVersion    = "perceptual-metric-contract-v1"
	perceptualAlignmentContractVersion = "perceptual-alignment-v1"
)

type PerceptualMetric string

const (
	PerceptualMetricVMAF   PerceptualMetric = "VMAF"
	PerceptualMetricXPSNR  PerceptualMetric = "XPSNR"
	PerceptualMetricSSIM   PerceptualMetric = "SSIM"
	PerceptualMetricMSSSIM PerceptualMetric = "MS-SSIM"
	PerceptualMetricPSNR   PerceptualMetric = "PSNR"
)

type PerceptualMetricStatus string

const (
	PerceptualMetricAvailable         PerceptualMetricStatus = "available"
	PerceptualMetricUnsupported       PerceptualMetricStatus = "unsupported"
	PerceptualMetricUnavailable       PerceptualMetricStatus = "unavailable"
	PerceptualMetricFailedMeasurement PerceptualMetricStatus = "failed_measurement"
	PerceptualMetricNonComparable     PerceptualMetricStatus = "non_comparable"
)

type MetricValueKind string

const (
	MetricValueFinite           MetricValueKind = "finite"
	MetricValuePositiveInfinity MetricValueKind = "positive_infinity"
)

// MetricValue is the only representation allowed across JSON, persistence,
// and identity boundaries. encoding/json cannot encode IEEE Inf or NaN, so
// positive infinity is explicitly tagged and NaN is rejected.
type MetricValue struct {
	Kind  MetricValueKind `json:"kind"`
	Value *float64        `json:"value,omitempty"`
}

func NewFiniteMetricValue(value float64) (MetricValue, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return MetricValue{}, errors.New("metric value must be finite")
	}
	return MetricValue{Kind: MetricValueFinite, Value: &value}, nil
}

func NewPositiveInfinityMetricValue() MetricValue {
	return MetricValue{Kind: MetricValuePositiveInfinity}
}

func (value MetricValue) Valid() bool {
	switch value.Kind {
	case MetricValueFinite:
		return value.Value != nil && !math.IsNaN(*value.Value) && !math.IsInf(*value.Value, 0)
	case MetricValuePositiveInfinity:
		return value.Value == nil
	default:
		return false
	}
}

func (value MetricValue) IsPositiveInfinity() bool {
	return value.Kind == MetricValuePositiveInfinity && value.Valid()
}

func (value MetricValue) Float64() (float64, bool) {
	if !value.Valid() {
		return 0, false
	}
	if value.IsPositiveInfinity() {
		return math.Inf(1), true
	}
	return *value.Value, true
}

func (value MetricValue) MarshalJSON() ([]byte, error) {
	if !value.Valid() {
		return nil, errors.New("invalid metric value")
	}
	type wire MetricValue
	return json.Marshal(wire(value))
}

func (value *MetricValue) UnmarshalJSON(data []byte) error {
	type wire MetricValue
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	parsed := MetricValue(decoded)
	if !parsed.Valid() {
		return errors.New("invalid metric value")
	}
	*value = parsed
	return nil
}

type ReferenceIdentity struct {
	Identity                 string            `json:"identity"`
	SamplingPlanIdentity     string            `json:"samplingPlanIdentity"`
	SamplingWindowIdentity   string            `json:"samplingWindowIdentity"`
	SourceRevision           string            `json:"sourceRevision"`
	SourceStreamIdentity     string            `json:"sourceStreamIdentity"`
	TransformIdentity        string            `json:"transformIdentity"`
	TransformVersion         string            `json:"transformVersion"`
	NormalizationProvenance  models.JSONMap    `json:"normalizationProvenance,omitempty"`
	AlignmentIdentity        string            `json:"alignmentIdentity"`
	Alignment                AlignmentEvidence `json:"alignment"`
	ComparisonDomainIdentity string            `json:"comparisonDomainIdentity"`
	MaterializationRevision  string            `json:"materializationRevision,omitempty"`
}

type CandidateIdentity struct {
	Identity                  string  `json:"identity"`
	Kind                      string  `json:"kind"`
	ArtifactID                string  `json:"artifactId,omitempty"`
	GenerationToken           string  `json:"generationToken,omitempty"`
	GenerationVersion         int64   `json:"generationVersion,omitempty"`
	OutputRevision            string  `json:"outputRevision,omitempty"`
	ConfigurationIdentity     string  `json:"configurationIdentity,omitempty"`
	SourceRevision            string  `json:"sourceRevision"`
	StreamIdentity            string  `json:"streamIdentity"`
	SegmentStartSeconds       float64 `json:"segmentStartSeconds"`
	SegmentDurationSeconds    float64 `json:"segmentDurationSeconds"`
	TimelineIdentity          string  `json:"timelineIdentity"`
	RuntimeCapabilityIdentity string  `json:"runtimeCapabilityIdentity,omitempty"`
}

type CandidateCoverage struct {
	Identity                  string   `json:"identity"`
	SourceStartSeconds        float64  `json:"sourceStartSeconds"`
	SourceDurationSeconds     float64  `json:"sourceDurationSeconds"`
	CoveredWindowIdentities   []string `json:"coveredWindowIdentities"`
	SegmentMappingIdentity    string   `json:"segmentMappingIdentity"`
	SourceTimelineIdentity    string   `json:"sourceTimelineIdentity"`
	CandidateTimelineIdentity string   `json:"candidateTimelineIdentity"`
}

type CandidateManifestEntry struct {
	Candidate CandidateIdentity `json:"candidate"`
	Coverage  CandidateCoverage `json:"coverage"`
}

// AlignmentEvidence is bounded bookkeeping for the source -> reference ->
// candidate time map. It deliberately excludes decoded frames and raw stderr.
type AlignmentEvidence struct {
	ContractVersion      string  `json:"contractVersion"`
	SourceStartPTS       float64 `json:"sourceStartPTS"`
	SourceEndPTS         float64 `json:"sourceEndPTS"`
	ReferenceStartPTS    float64 `json:"referenceStartPTS"`
	ReferenceEndPTS      float64 `json:"referenceEndPTS"`
	CandidateStartPTS    float64 `json:"candidateStartPTS"`
	CandidateEndPTS      float64 `json:"candidateEndPTS"`
	ToleranceSeconds     float64 `json:"toleranceSeconds"`
	SourceFrameCount     int     `json:"sourceFrameCount"`
	ReferenceFrameCount  int     `json:"referenceFrameCount"`
	CandidateFrameCount  int     `json:"candidateFrameCount"`
	DroppedFrameCount    int     `json:"droppedFrameCount"`
	DuplicatedFrameCount int     `json:"duplicatedFrameCount"`
	UnmatchedFrameCount  int     `json:"unmatchedFrameCount"`
}

type ReferenceManifestEntry struct {
	Reference ReferenceIdentity `json:"reference"`
}

// ComparisonPairIdentity is the identity of the comparable media material.
// It intentionally contains no metric/provider/model fields.
type ComparisonPairIdentity struct {
	Identity                 string            `json:"identity"`
	SamplingPlanIdentity     string            `json:"samplingPlanIdentity"`
	SamplingWindowIdentity   string            `json:"samplingWindowIdentity"`
	Reference                ReferenceIdentity `json:"reference"`
	Candidate                CandidateIdentity `json:"candidate"`
	CoverageIdentity         string            `json:"coverageIdentity"`
	SegmentMappingIdentity   string            `json:"segmentMappingIdentity"`
	AlignmentIdentity        string            `json:"alignmentIdentity"`
	ComparisonDomainIdentity string            `json:"comparisonDomainIdentity"`
	SourceRevision           string            `json:"sourceRevision"`
	CandidateRevision        string            `json:"candidateRevision"`
}

type MetricDescriptor struct {
	Identity                  string           `json:"identity"`
	LogicalMetric             PerceptualMetric `json:"logicalMetric"`
	Provider                  string           `json:"provider"`
	ProviderVersion           string           `json:"providerVersion"`
	InvocationContract        string           `json:"invocationContract"`
	ModelIdentity             string           `json:"modelIdentity,omitempty"`
	RuntimeCapabilityIdentity string           `json:"runtimeCapabilityIdentity"`
	Direction                 string           `json:"direction"`
	Semantics                 string           `json:"semantics"`
}

type MetricResultIdentity struct {
	Identity     string           `json:"identity"`
	PairIdentity string           `json:"pairIdentity"`
	Metric       MetricDescriptor `json:"metric"`
}

type MetricResult struct {
	Identity MetricResultIdentity   `json:"identity"`
	Status   PerceptualMetricStatus `json:"status"`
	Value    *MetricValue           `json:"value,omitempty"`
	Reason   string                 `json:"reason,omitempty"`
}

type PerceptualWindowEvidence struct {
	WindowIdentity   string                  `json:"windowIdentity"`
	StartSeconds     float64                 `json:"startSeconds"`
	DurationSeconds  float64                 `json:"durationSeconds"`
	Primary          bool                    `json:"primary"`
	Roles            []string                `json:"roles,omitempty"`
	Reasons          []string                `json:"reasons,omitempty"`
	EvidenceRefs     []string                `json:"evidenceRefs,omitempty"`
	Status           string                  `json:"status"`
	Reference        *ReferenceIdentity      `json:"reference,omitempty"`
	Candidate        *CandidateIdentity      `json:"candidate,omitempty"`
	Coverage         *CandidateCoverage      `json:"coverage,omitempty"`
	PairIdentity     *ComparisonPairIdentity `json:"pairIdentity,omitempty"`
	Metrics          []MetricResult          `json:"metrics,omitempty"`
	ComparableReason string                  `json:"comparableReason,omitempty"`
}

type MetricAggregate struct {
	LogicalMetric             PerceptualMetric `json:"logicalMetric"`
	Direction                 string           `json:"direction"`
	TotalFrozenWindows        int              `json:"totalFrozenWindows"`
	CandidateCoveredWindows   int              `json:"candidateCoveredWindows"`
	MeasuredComparableWindows int              `json:"measuredComparableWindows"`
	NotCoveredWindows         int              `json:"notCoveredWindows"`
	UnsupportedWindows        int              `json:"unsupportedWindows"`
	UnavailableWindows        int              `json:"unavailableWindows"`
	FailedMeasurementWindows  int              `json:"failedMeasurementWindows"`
	NonComparableWindows      int              `json:"nonComparableWindows"`
	Median                    *MetricValue     `json:"median,omitempty"`
	LowerPercentileP10        *MetricValue     `json:"lowerPercentileP10,omitempty"`
	PercentileDegraded        bool             `json:"percentileDegraded"`
	PercentileProvenance      string           `json:"percentileProvenance,omitempty"`
	WorstValue                *MetricValue     `json:"worstValue,omitempty"`
	WorstWindowIdentity       string           `json:"worstWindowIdentity,omitempty"`
	WorstPairIdentity         string           `json:"worstPairIdentity,omitempty"`
}

type MetricCapability struct {
	Metric                    PerceptualMetric `json:"metric"`
	Provider                  string           `json:"provider,omitempty"`
	ProviderVersion           string           `json:"providerVersion,omitempty"`
	InvocationContract        string           `json:"invocationContract,omitempty"`
	ModelIdentity             string           `json:"modelIdentity,omitempty"`
	Availability              string           `json:"availability"`
	Reason                    string           `json:"reason,omitempty"`
	RuntimeCapabilityIdentity string           `json:"runtimeCapabilityIdentity"`
}

type ArtifactEvidence struct {
	Kind     string         `json:"kind"`
	Status   string         `json:"status"`
	Reason   string         `json:"reason,omitempty"`
	Evidence models.JSONMap `json:"evidence,omitempty"`
}

type PerceptualEvidenceSet struct {
	Status                    string                     `json:"status,omitempty"`
	UnavailableReason         string                     `json:"unavailableReason,omitempty"`
	SchemaVersion             string                     `json:"schemaVersion"`
	PolicyVersion             string                     `json:"policyVersion"`
	Identity                  string                     `json:"identity"`
	SamplingPlanIdentity      string                     `json:"samplingPlanIdentity"`
	SourceRevision            string                     `json:"sourceRevision"`
	RuntimeCapabilityIdentity string                     `json:"runtimeCapabilityIdentity"`
	RequestedMetrics          []MetricDescriptor         `json:"requestedMetrics"`
	CandidateManifest         []CandidateManifestEntry   `json:"candidateManifest,omitempty"`
	ReferenceManifest         []ReferenceManifestEntry   `json:"referenceManifest,omitempty"`
	Windows                   []PerceptualWindowEvidence `json:"windows"`
	Aggregates                []MetricAggregate          `json:"aggregates,omitempty"`
	Capabilities              []MetricCapability         `json:"capabilities,omitempty"`
	Warnings                  []string                   `json:"warnings,omitempty"`
	UnavailableCapabilities   []string                   `json:"unavailableCapabilities,omitempty"`
	ArtifactEvidence          []ArtifactEvidence         `json:"artifactEvidence,omitempty"`
	Provenance                models.JSONMap             `json:"provenance,omitempty"`
}

func canonicalEvidenceHash(prefix string, value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return prefix + "-" + hex.EncodeToString(digest[:]), nil
}

func ComputeReferenceIdentity(value ReferenceIdentity) (ReferenceIdentity, error) {
	value.Identity = ""
	identity, err := canonicalEvidenceHash("reference", value)
	if err != nil {
		return ReferenceIdentity{}, err
	}
	value.Identity = identity
	return value, nil
}

func ComputeCandidateIdentity(value CandidateIdentity) (CandidateIdentity, error) {
	value.Identity = ""
	identity, err := canonicalEvidenceHash("candidate", value)
	if err != nil {
		return CandidateIdentity{}, err
	}
	value.Identity = identity
	return value, nil
}

func ComputeCandidateCoverage(value CandidateCoverage) (CandidateCoverage, error) {
	value.Identity = ""
	value.CoveredWindowIdentities = append([]string(nil), value.CoveredWindowIdentities...)
	sort.Strings(value.CoveredWindowIdentities)
	identity, err := canonicalEvidenceHash("coverage", value)
	if err != nil {
		return CandidateCoverage{}, err
	}
	value.Identity = identity
	return value, nil
}

func ComputeComparisonPairIdentity(value ComparisonPairIdentity) (ComparisonPairIdentity, error) {
	if value.Reference.Identity == "" {
		reference, err := ComputeReferenceIdentity(value.Reference)
		if err != nil {
			return ComparisonPairIdentity{}, err
		}
		value.Reference = reference
	}
	if value.Candidate.Identity == "" {
		candidate, err := ComputeCandidateIdentity(value.Candidate)
		if err != nil {
			return ComparisonPairIdentity{}, err
		}
		value.Candidate = candidate
	}
	value.Identity = ""
	identity, err := canonicalEvidenceHash("comparison-pair", value)
	if err != nil {
		return ComparisonPairIdentity{}, err
	}
	value.Identity = identity
	return value, nil
}

func ComputeMetricDescriptorIdentity(value MetricDescriptor) (MetricDescriptor, error) {
	value.Identity = ""
	identity, err := canonicalEvidenceHash("metric-descriptor", value)
	if err != nil {
		return MetricDescriptor{}, err
	}
	value.Identity = identity
	return value, nil
}

func ComputeMetricResultIdentity(pair ComparisonPairIdentity, descriptor MetricDescriptor) (MetricResultIdentity, error) {
	if pair.Identity == "" {
		var err error
		pair, err = ComputeComparisonPairIdentity(pair)
		if err != nil {
			return MetricResultIdentity{}, err
		}
	}
	descriptor, err := ComputeMetricDescriptorIdentity(descriptor)
	if err != nil {
		return MetricResultIdentity{}, err
	}
	value := MetricResultIdentity{PairIdentity: pair.Identity, Metric: descriptor}
	identity, err := canonicalEvidenceHash("metric-result", value)
	if err != nil {
		return MetricResultIdentity{}, err
	}
	value.Identity = identity
	return value, nil
}

func orderedPerceptualWindows(plan SamplingPlan, windows []PerceptualWindowEvidence) []PerceptualWindowEvidence {
	selection := SamplingPlanForPerceptualQuality(plan)
	byIdentity := make(map[string]PerceptualWindowEvidence, len(windows))
	for _, window := range windows {
		byIdentity[window.WindowIdentity] = window
	}
	ordered := make([]PerceptualWindowEvidence, 0, len(selection.Windows))
	for _, frozen := range selection.Windows {
		if window, ok := byIdentity[frozen.Identity]; ok {
			ordered = append(ordered, window)
			delete(byIdentity, frozen.Identity)
			continue
		}
		ordered = append(ordered, PerceptualWindowEvidence{
			WindowIdentity: frozen.Identity, StartSeconds: frozen.StartSeconds,
			DurationSeconds: frozen.DurationSeconds, Primary: frozen.Primary,
			Roles: append([]string(nil), frozen.Roles...), Reasons: append([]string(nil), frozen.Reasons...),
			EvidenceRefs: append([]string(nil), frozen.EvidenceRefs...), Status: "not_covered",
			ComparableReason: "candidate does not represent this frozen SamplingWindow",
		})
	}
	// A caller cannot make map/goroutine order affect the set. Unknown entries
	// are retained only for diagnostics and are put after the frozen ledger.
	extra := make([]PerceptualWindowEvidence, 0, len(byIdentity))
	for _, window := range byIdentity {
		extra = append(extra, window)
	}
	sort.Slice(extra, func(i, j int) bool { return extra[i].WindowIdentity < extra[j].WindowIdentity })
	return append(ordered, extra...)
}

func ComputePerceptualEvidenceSetIdentity(value PerceptualEvidenceSet) (PerceptualEvidenceSet, error) {
	value.Identity = ""
	value.RequestedMetrics = canonicalMetricDescriptors(value.RequestedMetrics)
	identity, err := canonicalEvidenceHash("perceptual-evidence", struct {
		SchemaVersion             string             `json:"schemaVersion"`
		PolicyVersion             string             `json:"policyVersion"`
		SamplingPlanIdentity      string             `json:"samplingPlanIdentity"`
		SourceRevision            string             `json:"sourceRevision"`
		RuntimeCapabilityIdentity string             `json:"runtimeCapabilityIdentity"`
		RequestedMetrics          []MetricDescriptor `json:"requestedMetrics"`
		WindowLedger              []struct {
			WindowIdentity string `json:"windowIdentity"`
			PairIdentity   string `json:"pairIdentity,omitempty"`
			Status         string `json:"status"`
		} `json:"windowLedger"`
	}{
		SchemaVersion: value.SchemaVersion, PolicyVersion: value.PolicyVersion,
		SamplingPlanIdentity: value.SamplingPlanIdentity, SourceRevision: value.SourceRevision,
		RuntimeCapabilityIdentity: value.RuntimeCapabilityIdentity,
		RequestedMetrics:          canonicalMetricDescriptors(value.RequestedMetrics),
		WindowLedger:              perceptualWindowLedger(value.Windows),
	})
	if err != nil {
		return PerceptualEvidenceSet{}, err
	}
	value.Identity = identity
	return value, nil
}

func canonicalMetricDescriptors(values []MetricDescriptor) []MetricDescriptor {
	result := append([]MetricDescriptor(nil), values...)
	for index := range result {
		if result[index].Identity == "" {
			result[index], _ = ComputeMetricDescriptorIdentity(result[index])
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].LogicalMetric != result[j].LogicalMetric {
			return result[i].LogicalMetric < result[j].LogicalMetric
		}
		return result[i].Identity < result[j].Identity
	})
	return result
}

func perceptualWindowLedger(windows []PerceptualWindowEvidence) []struct {
	WindowIdentity string `json:"windowIdentity"`
	PairIdentity   string `json:"pairIdentity,omitempty"`
	Status         string `json:"status"`
} {
	result := make([]struct {
		WindowIdentity string `json:"windowIdentity"`
		PairIdentity   string `json:"pairIdentity,omitempty"`
		Status         string `json:"status"`
	}, 0, len(windows))
	for _, window := range windows {
		pair := ""
		if window.PairIdentity != nil {
			pair = window.PairIdentity.Identity
		}
		result = append(result, struct {
			WindowIdentity string `json:"windowIdentity"`
			PairIdentity   string `json:"pairIdentity,omitempty"`
			Status         string `json:"status"`
		}{window.WindowIdentity, pair, window.Status})
	}
	return result
}

func BuildPerceptualEvidenceSet(plan SamplingPlan, sourceRevision, runtimeCapabilityIdentity string, requested []MetricDescriptor, windows []PerceptualWindowEvidence, capabilities []MetricCapability, warnings []string, provenance models.JSONMap) (PerceptualEvidenceSet, error) {
	ordered := orderedPerceptualWindows(plan, windows)
	set := PerceptualEvidenceSet{
		Status:        "ready",
		SchemaVersion: perceptualEvidenceSchemaVersion, PolicyVersion: perceptualEvidencePolicyVersion,
		SamplingPlanIdentity: plan.PlanIdentity, SourceRevision: sourceRevision,
		RuntimeCapabilityIdentity: runtimeCapabilityIdentity, RequestedMetrics: canonicalMetricDescriptors(requested),
		Windows: ordered, Capabilities: append([]MetricCapability(nil), capabilities...), Warnings: append([]string(nil), warnings...),
		Provenance: provenance,
	}
	for _, window := range ordered {
		if window.Candidate != nil && window.Coverage != nil {
			set.CandidateManifest = appendUniqueCandidateManifest(set.CandidateManifest, CandidateManifestEntry{Candidate: *window.Candidate, Coverage: *window.Coverage})
		}
		if window.Reference != nil {
			set.ReferenceManifest = appendUniqueReferenceManifest(set.ReferenceManifest, ReferenceManifestEntry{Reference: *window.Reference})
		}
	}
	set.Aggregates = AggregatePerceptualMetricsForRequested(ordered, set.RequestedMetrics)
	return ComputePerceptualEvidenceSetIdentity(set)
}

func appendUniqueCandidateManifest(values []CandidateManifestEntry, entry CandidateManifestEntry) []CandidateManifestEntry {
	for _, existing := range values {
		if existing.Candidate.Identity == entry.Candidate.Identity && existing.Coverage.Identity == entry.Coverage.Identity {
			return values
		}
	}
	return append(values, entry)
}

func appendUniqueReferenceManifest(values []ReferenceManifestEntry, entry ReferenceManifestEntry) []ReferenceManifestEntry {
	for _, existing := range values {
		if existing.Reference.Identity == entry.Reference.Identity {
			return values
		}
	}
	return append(values, entry)
}

func metricDirection(metric PerceptualMetric) string {
	if metric == PerceptualMetricPSNR || metric == PerceptualMetricSSIM || metric == PerceptualMetricMSSSIM || metric == PerceptualMetricVMAF || metric == PerceptualMetricXPSNR {
		return "higher_is_better"
	}
	return "unknown"
}

func AggregatePerceptualMetrics(windows []PerceptualWindowEvidence) []MetricAggregate {
	return AggregatePerceptualMetricsForRequested(windows, nil)
}

func AggregatePerceptualMetricsForRequested(windows []PerceptualWindowEvidence, requested []MetricDescriptor) []MetricAggregate {
	metrics := map[PerceptualMetric][]struct {
		value  MetricValue
		window string
		pair   string
	}{}
	counts := map[PerceptualMetric]*MetricAggregate{}
	covered := map[PerceptualMetric]map[string]bool{}
	total := len(windows)
	for _, window := range windows {
		seen := map[PerceptualMetric]bool{}
		for _, result := range window.Metrics {
			metric := result.Identity.Metric.LogicalMetric
			seen[metric] = true
			aggregate := counts[metric]
			if aggregate == nil {
				aggregate = &MetricAggregate{LogicalMetric: metric, Direction: metricDirection(metric), TotalFrozenWindows: total}
				counts[metric] = aggregate
				covered[metric] = map[string]bool{}
			}
			switch result.Status {
			case PerceptualMetricAvailable:
				if window.Status != "not_covered" {
					covered[metric][window.WindowIdentity] = true
				}
				if result.Value == nil || !result.Value.Valid() {
					continue
				}
				metrics[metric] = append(metrics[metric], struct {
					value  MetricValue
					window string
					pair   string
				}{*result.Value, window.WindowIdentity, pairIdentityOf(window)})
				aggregate.MeasuredComparableWindows++
			case PerceptualMetricUnsupported:
				aggregate.UnsupportedWindows++
			case PerceptualMetricUnavailable:
				aggregate.UnavailableWindows++
			case PerceptualMetricFailedMeasurement:
				aggregate.FailedMeasurementWindows++
			case PerceptualMetricNonComparable:
				aggregate.NonComparableWindows++
			}
			if window.Status != "not_covered" {
				covered[metric][window.WindowIdentity] = true
			}
		}
		if window.Status == "not_covered" {
			for _, descriptor := range requested {
				metric := descriptor.LogicalMetric
				if seen[metric] {
					continue
				}
				aggregate := counts[metric]
				if aggregate == nil {
					aggregate = &MetricAggregate{LogicalMetric: metric, Direction: metricDirection(metric), TotalFrozenWindows: total}
					counts[metric] = aggregate
					covered[metric] = map[string]bool{}
				}
				aggregate.NotCoveredWindows++
			}
		}
	}
	keys := make([]PerceptualMetric, 0, len(counts))
	for metric := range counts {
		keys = append(keys, metric)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	result := make([]MetricAggregate, 0, len(keys))
	for _, metric := range keys {
		aggregate := *counts[metric]
		aggregate.CandidateCoveredWindows = len(covered[metric])
		values := append([]struct {
			value  MetricValue
			window string
			pair   string
		}{}, metrics[metric]...)
		sort.Slice(values, func(i, j int) bool { return compareMetricValues(values[i].value, values[j].value) < 0 })
		if len(values) > 0 {
			aggregate.Median = metricValuePointer(medianMetricValue(values))
			index := (len(values) - 1) / 10
			if len(values) < 10 {
				aggregate.PercentileDegraded = true
				aggregate.PercentileProvenance = fmt.Sprintf("nearest_rank_p10_degraded_insufficient_samples:%d", len(values))
			} else {
				aggregate.PercentileProvenance = "nearest_rank_p10"
			}
			aggregate.LowerPercentileP10 = metricValuePointer(values[index].value)
			worst := values[0]
			for _, candidate := range values[1:] {
				if compareMetricValues(candidate.value, worst.value) < 0 || (compareMetricValues(candidate.value, worst.value) == 0 && candidate.window < worst.window) {
					worst = candidate
				}
			}
			aggregate.WorstValue = metricValuePointer(worst.value)
			aggregate.WorstWindowIdentity, aggregate.WorstPairIdentity = worst.window, worst.pair
		}
		result = append(result, aggregate)
	}
	return result
}

func pairIdentityOf(window PerceptualWindowEvidence) string {
	if window.PairIdentity == nil {
		return ""
	}
	return window.PairIdentity.Identity
}

func metricValuePointer(value MetricValue) *MetricValue { return &value }

func compareMetricValues(left, right MetricValue) int {
	leftValue, leftOK := left.Float64()
	rightValue, rightOK := right.Float64()
	if !leftOK || !rightOK {
		return 0
	}
	if leftValue < rightValue {
		return -1
	}
	if leftValue > rightValue {
		return 1
	}
	return 0
}

func medianMetricValue(values []struct {
	value  MetricValue
	window string
	pair   string
}) MetricValue {
	if len(values)%2 == 1 {
		return values[len(values)/2].value
	}
	left, right := values[len(values)/2-1].value, values[len(values)/2].value
	if left.IsPositiveInfinity() || right.IsPositiveInfinity() {
		return NewPositiveInfinityMetricValue()
	}
	leftValue, _ := left.Float64()
	rightValue, _ := right.Float64()
	value, err := NewFiniteMetricValue((leftValue + rightValue) / 2)
	if err != nil {
		return NewPositiveInfinityMetricValue()
	}
	return value
}

func newMetricDescriptor(metric PerceptualMetric, provider, providerVersion, invocation, model, capability, semantics string) MetricDescriptor {
	descriptor, _ := ComputeMetricDescriptorIdentity(MetricDescriptor{
		LogicalMetric: metric, Provider: provider, ProviderVersion: providerVersion,
		InvocationContract: invocation, ModelIdentity: model, RuntimeCapabilityIdentity: capability,
		Direction: metricDirection(metric), Semantics: semantics,
	})
	return descriptor
}

func validateMetricResult(result MetricResult) error {
	if result.Status == PerceptualMetricAvailable {
		if result.Value == nil || !result.Value.Valid() {
			return errors.New("available metric result requires a valid value")
		}
		return nil
	}
	if result.Value != nil {
		return errors.New("non-available metric result must not contain a value")
	}
	return nil
}

func metricUnavailableResult(pair ComparisonPairIdentity, capability MetricCapability, status PerceptualMetricStatus, reason string) MetricResult {
	descriptor := newMetricDescriptor(capability.Metric, capability.Provider, capability.ProviderVersion, capability.InvocationContract, capability.ModelIdentity, capability.RuntimeCapabilityIdentity, "higher_is_better")
	identity, _ := ComputeMetricResultIdentity(pair, descriptor)
	return MetricResult{Identity: identity, Status: status, Reason: strings.TrimSpace(reason)}
}
