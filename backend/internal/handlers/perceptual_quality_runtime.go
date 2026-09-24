package handlers

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/capabilities"
	"github.com/anuelvs/mvforge/backend/internal/models"
)

type perceptualArtifactInput struct {
	SourcePath            string
	CandidatePath         string
	SourceRevision        string
	CandidateRevision     string
	ConfigurationIdentity string
	CandidateKind         string
	CandidateArtifactID   string
	GenerationToken       string
	GenerationVersion     int64
	SegmentStartSeconds   float64
	SegmentDuration       float64
	CandidateDuration     float64
	FullOutput            bool
}

// collectPerceptualEvidenceForArtifact is the shared Test Encode/Validation
// provider. It receives an already-ready candidate and never calls an encode
// path. The caller supplies the frozen effective authority and plan.
func collectPerceptualEvidenceForArtifact(ctx context.Context, plan SamplingPlan, profile models.Profile, input perceptualArtifactInput) (PerceptualEvidenceSet, error) {
	selection := SamplingPlanForPerceptualQuality(plan)
	runtimeSnapshot := capabilities.CachedMetricCapabilities(ctx)
	capabilityValues := make([]MetricCapability, 0, len(runtimeSnapshot.Metrics))
	requested := make([]MetricDescriptor, 0, len(runtimeSnapshot.Metrics))
	for _, rawCapability := range runtimeSnapshot.Metrics {
		capabilityValues = append(capabilityValues, MetricCapability{
			Metric: PerceptualMetric(rawCapability.Metric), Provider: rawCapability.Provider,
			ProviderVersion: rawCapability.ProviderVersion, InvocationContract: rawCapability.InvocationContract,
			ModelIdentity: rawCapability.ModelIdentity, Availability: rawCapability.Availability,
			Reason: rawCapability.Reason, RuntimeCapabilityIdentity: rawCapability.RuntimeCapabilityIdentity,
		})
		requested = append(requested, newMetricDescriptor(PerceptualMetric(rawCapability.Metric), rawCapability.Provider, rawCapability.ProviderVersion, rawCapability.InvocationContract, rawCapability.ModelIdentity, rawCapability.RuntimeCapabilityIdentity, "higher_is_better"))
	}
	if len(selection.Windows) == 0 {
		return BuildPerceptualEvidenceSet(plan, input.SourceRevision, runtimeSnapshot.RuntimeCapabilityIdentity, requested, nil, capabilityValues, []string{"SamplingPlan contains no frozen SelectedWindows"}, models.JSONMap{"owner": "shared-perceptual-provider"})
	}

	covered := make([]SamplingWindow, 0, len(selection.Windows))
	for _, window := range selection.Windows {
		if candidateCoversWindow(input, window) {
			covered = append(covered, window)
		}
	}
	coveredIDs := make([]string, 0, len(covered))
	for _, window := range covered {
		coveredIDs = append(coveredIDs, window.Identity)
	}

	transformIdentity, comparable, transformReason, err := canonicalPerceptualReference(profile)
	if err != nil {
		return PerceptualEvidenceSet{}, err
	}
	windowEvidence := make([]PerceptualWindowEvidence, 0, len(covered))
	warnings := []string{}
	if !comparable {
		warnings = append(warnings, transformReason)
	}
	for _, window := range covered {
		evidence, evidenceErr := collectPerceptualArtifactWindow(ctx, plan, profile, input, window, coveredIDs, transformIdentity, comparable, transformReason, capabilityValues)
		if evidenceErr != nil {
			return PerceptualEvidenceSet{}, evidenceErr
		}
		windowEvidence = append(windowEvidence, evidence)
	}
	return BuildPerceptualEvidenceSet(plan, input.SourceRevision, runtimeSnapshot.RuntimeCapabilityIdentity, requested, windowEvidence, capabilityValues, warnings, models.JSONMap{
		"owner": "shared-perceptual-provider", "candidateKind": input.CandidateKind,
		"metricWork": "sequential", "candidateCoverageWindowCount": len(covered),
	})
}

func collectPerceptualEvidenceUnderHeavySlot(ctx context.Context, plan SamplingPlan, profile models.Profile, input perceptualArtifactInput) (PerceptualEvidenceSet, error) {
	select {
	case labFidelityHeavySlot <- struct{}{}:
		defer func() { <-labFidelityHeavySlot }()
	case <-ctx.Done():
		return PerceptualEvidenceSet{}, ctx.Err()
	}
	return collectPerceptualEvidenceForArtifact(ctx, plan, profile, input)
}

func candidateCoversWindow(input perceptualArtifactInput, window SamplingWindow) bool {
	if input.SegmentDuration <= 0 || window.DurationSeconds <= 0 {
		return false
	}
	tolerance := 1.0 / 30.0
	segmentEnd := input.SegmentStartSeconds + input.SegmentDuration
	windowEnd := window.StartSeconds + window.DurationSeconds
	if window.StartSeconds < input.SegmentStartSeconds-tolerance || windowEnd > segmentEnd+tolerance {
		return false
	}
	if input.FullOutput {
		return input.CandidateDuration > 0 && windowEnd <= input.CandidateDuration+tolerance
	}
	return true
}

func collectPerceptualArtifactWindow(ctx context.Context, plan SamplingPlan, profile models.Profile, input perceptualArtifactInput, window SamplingWindow, coveredIDs []string, transformIdentity string, comparable bool, transformReason string, capabilityValues []MetricCapability) (PerceptualWindowEvidence, error) {
	sourceRevision := input.SourceRevision
	if sourceRevision == "" {
		sourceRevision = "unknown-source-revision"
	}
	comparisonDomain, err := canonicalEvidenceHash("comparison-domain", models.JSONMap{"pixelFormat": "yuv420p", "squarePixels": true, "transform": transformIdentity})
	if err != nil {
		return PerceptualWindowEvidence{}, err
	}
	candidateStart := 0.0
	if input.FullOutput {
		candidateStart = window.StartSeconds
	} else {
		candidateStart = math.Max(0, window.StartSeconds-input.SegmentStartSeconds)
	}
	alignment, err := canonicalEvidenceHash("alignment", models.JSONMap{
		"contract": perceptualAlignmentContractVersion, "sourceStartSeconds": window.StartSeconds,
		"sourceDurationSeconds": window.DurationSeconds, "candidateStartSeconds": candidateStart,
		"candidateDurationSeconds": input.SegmentDuration, "toleranceFrames": 1,
		"timestampDomain": "normalized-zero-based",
	})
	if err != nil {
		return PerceptualWindowEvidence{}, err
	}
	reference, err := ComputeReferenceIdentity(ReferenceIdentity{
		SamplingPlanIdentity: plan.PlanIdentity, SamplingWindowIdentity: window.Identity,
		SourceRevision: sourceRevision, SourceStreamIdentity: "0:v:0",
		TransformIdentity: transformIdentity, TransformVersion: perceptualEvidenceSchemaVersion,
		NormalizationProvenance: models.JSONMap{"mode": "preview-normalized", "filter": perceptualReferenceFilter(profile) + ",format=yuv420p,setsar=1"},
		AlignmentIdentity:       alignment, Alignment: perceptualAlignmentEvidence(window.StartSeconds, window.DurationSeconds, candidateStart, input.SegmentDuration), ComparisonDomainIdentity: comparisonDomain,
	})
	if err != nil {
		return PerceptualWindowEvidence{}, err
	}
	candidateTimeline, err := canonicalEvidenceHash("candidate-timeline", models.JSONMap{
		"kind": input.CandidateKind, "revision": input.CandidateRevision,
		"configurationIdentity": input.ConfigurationIdentity, "segmentStartSeconds": input.SegmentStartSeconds, "segmentDurationSeconds": input.SegmentDuration,
	})
	if err != nil {
		return PerceptualWindowEvidence{}, err
	}
	candidateBase, err := ComputeCandidateIdentity(CandidateIdentity{
		Kind: input.CandidateKind, ArtifactID: input.CandidateArtifactID,
		GenerationToken: input.GenerationToken, GenerationVersion: input.GenerationVersion,
		OutputRevision: input.CandidateRevision, SourceRevision: sourceRevision,
		ConfigurationIdentity: input.ConfigurationIdentity,
		StreamIdentity:        "0:v:0", SegmentStartSeconds: input.SegmentStartSeconds,
		SegmentDurationSeconds: input.SegmentDuration, TimelineIdentity: candidateTimeline,
	})
	if err != nil {
		return PerceptualWindowEvidence{}, err
	}
	coverage, err := ComputeCandidateCoverage(CandidateCoverage{
		SourceStartSeconds: input.SegmentStartSeconds, SourceDurationSeconds: input.SegmentDuration,
		CoveredWindowIdentities: coveredIDs, SegmentMappingIdentity: candidateTimeline,
		SourceTimelineIdentity: sourceRevision, CandidateTimelineIdentity: candidateTimeline,
	})
	if err != nil {
		return PerceptualWindowEvidence{}, err
	}
	pair, err := ComputeComparisonPairIdentity(ComparisonPairIdentity{
		SamplingPlanIdentity: plan.PlanIdentity, SamplingWindowIdentity: window.Identity,
		Reference: reference, Candidate: candidateBase, CoverageIdentity: coverage.Identity,
		SegmentMappingIdentity: coverage.SegmentMappingIdentity, AlignmentIdentity: alignment,
		ComparisonDomainIdentity: comparisonDomain, SourceRevision: sourceRevision,
		CandidateRevision: input.CandidateRevision,
	})
	if err != nil {
		return PerceptualWindowEvidence{}, err
	}

	metrics := previewFrameMetrics{}
	metricAttemptError := ""
	if comparable {
		outputStart := "0"
		if input.FullOutput {
			outputStart = strconv.FormatFloat(candidateStart, 'f', 3, 64)
		}
		metrics, err = generatePreviewFrameMetricsAt(ctx, input.SourcePath, strconv.FormatFloat(window.StartSeconds, 'f', 3, 64), perceptualReferenceFilter(profile), input.CandidatePath, outputStart)
		if err != nil {
			metrics.Reason = err.Error()
			metricAttemptError = err.Error()
		}
	}
	results := make([]MetricResult, 0, len(capabilityValues))
	for _, capability := range capabilityValues {
		status := PerceptualMetricUnavailable
		reason := capability.Reason
		var value *MetricValue
		if !comparable {
			status, reason = PerceptualMetricNonComparable, transformReason
			if strings.TrimSpace(reason) == "" {
				reason = metrics.Reason
			}
		} else if capability.Availability != "available" {
			if capability.Provider == "" {
				status = PerceptualMetricUnsupported
			}
		} else if capability.Metric == PerceptualMetricSSIM || capability.Metric == PerceptualMetricPSNR {
			metricAvailable := metrics.SSIMAvailable
			metricReason := metrics.SSIMReason
			candidateValue := metrics.SSIM
			metricValue := metrics.SSIMValue
			if capability.Metric == PerceptualMetricPSNR {
				metricAvailable, metricReason, candidateValue, metricValue = metrics.PSNRAvailable, metrics.PSNRReason, metrics.PSNR, metrics.PSNRValue
			}
			if !metrics.DomainComparable {
				status, reason = PerceptualMetricNonComparable, metrics.Reason
			} else if !metricAvailable {
				if metricAttemptError != "" {
					status, reason = PerceptualMetricFailedMeasurement, metricAttemptError
				} else {
					status, reason = PerceptualMetricFailedMeasurement, metricReason
				}
			} else {
				parsed, parseErr := NewFiniteMetricValue(candidateValue)
				if metricValue != nil {
					parsed, parseErr = *metricValue, nil
				}
				if parseErr != nil {
					status, reason = PerceptualMetricFailedMeasurement, parseErr.Error()
				} else {
					status, value, reason = PerceptualMetricAvailable, &parsed, ""
				}
			}
		} else {
			outputStart := "0"
			if input.FullOutput {
				outputStart = strconv.FormatFloat(candidateStart, 'f', 3, 64)
			}
			parsed, runErr := runPreviewLogicalMetricValueAtWithRunner(ctx, input.SourcePath, strconv.FormatFloat(window.StartSeconds, 'f', 3, 64), perceptualReferenceFilter(profile), input.CandidatePath, outputStart, "", capability, defaultPreviewMetricProcessRunner)
			if runErr != nil {
				status, reason = PerceptualMetricFailedMeasurement, runErr.Error()
			} else {
				status, value, reason = PerceptualMetricAvailable, &parsed, ""
			}
		}
		descriptor := newMetricDescriptor(capability.Metric, capability.Provider, capability.ProviderVersion, capability.InvocationContract, capability.ModelIdentity, capability.RuntimeCapabilityIdentity, "higher_is_better")
		resultIdentity, identityErr := ComputeMetricResultIdentity(pair, descriptor)
		if identityErr != nil {
			return PerceptualWindowEvidence{}, identityErr
		}
		results = append(results, MetricResult{Identity: resultIdentity, Status: status, Value: value, Reason: reason})
	}
	return PerceptualWindowEvidence{
		WindowIdentity: window.Identity, StartSeconds: window.StartSeconds, DurationSeconds: window.DurationSeconds,
		Primary: window.Primary, Roles: window.Roles, Reasons: window.Reasons, EvidenceRefs: window.EvidenceRefs,
		Status: "covered", Reference: &reference, Candidate: &candidateBase, Coverage: &coverage, PairIdentity: &pair,
		Metrics: results, ComparableReason: metrics.Reason,
	}, nil
}

func perceptualReferenceTransform(profile models.Profile) (string, bool, string, error) {
	executionPlan := frozenFidelityExecutionPlan(profile)
	worker := map[string]interface{}(profile.WorkerConfig)
	identity, err := canonicalEvidenceHash("fidelity-transform", models.JSONMap{
		"executionPlan": executionPlan, "frozenVideoFilterChain": workerStringValue(worker["videoFilters"]),
	})
	if err != nil {
		return "", false, "", err
	}
	// A raw-source comparison is valid only when the frozen Frame Fidelity
	// authority proves preservation. Transform states require a physical
	// canonical reference adapter and must never silently fall back to raw.
	cadenceOperation := stringFromUnknown(executionPlan["effectiveCadenceOperation"])
	fieldMode := strings.ToLower(strings.TrimSpace(stringFromUnknown(worker["fieldStructureMode"])))
	frozenFilterChain := strings.TrimSpace(workerStringValue(worker["videoFilters"]))
	if (fieldMode == "deinterlace" || fieldMode == "auto" && strings.Contains(strings.ToLower(stringFromUnknown(worker["effectiveAutomaticMotionFilters"])), "deinterlace")) && frozenFilterChain == "" {
		return identity, false, "canonical reference requires the frozen Frame Fidelity field-structure transform; raw source comparison was not used", nil
	}
	if (cadenceOperation != "" && !strings.EqualFold(cadenceOperation, "preserve") || executionPlan["upscale"] != nil || executionPlan["restoration"] != nil) && frozenFilterChain == "" {
		return identity, false, "canonical reference requires the frozen Frame Fidelity video transform; raw source comparison was not used", nil
	}
	for _, key := range []string{"effectiveCadenceOperation", "effectiveOutputFrameRate", "effectiveOutputProgressive", "upscale", "restoration"} {
		value := executionPlan[key]
		if key == "effectiveCadenceOperation" && (value == nil || strings.EqualFold(stringFromUnknown(value), "") || strings.EqualFold(stringFromUnknown(value), "preserve")) {
			continue
		}
		if key == "effectiveOutputFrameRate" && (cadenceOperation == "" || strings.EqualFold(cadenceOperation, "preserve")) {
			continue
		}
		if key == "effectiveOutputProgressive" && value == nil {
			continue
		}
		if key == "effectiveOutputProgressive" && fidelityFieldStructureExplicitPreserve(worker) {
			continue
		}
		if value == nil || stringFromUnknown(value) == "" || stringFromUnknown(value) == "0" {
			continue
		}
		return identity, false, fmt.Sprintf("canonical reference requires the frozen Frame Fidelity transform %s; raw source comparison was not used", key), nil
	}
	return identity, true, "", nil
}

// canonicalPerceptualReference is the narrow physical-reference adapter. It
// consumes the already frozen Frame Fidelity execution authority and exposes
// the exact resolved filter chain to the comparison graph; it never selects a
// window or derives policy from mutable/current FFmpeg text.
func canonicalPerceptualReference(profile models.Profile) (string, bool, string, error) {
	return perceptualReferenceTransform(profile)
}

func perceptualReferenceFilter(profile models.Profile) string {
	// This is the already resolved/frozen command filter chain. The provider
	// consumes it as a physical reference transform; it does not infer policy
	// by parsing current FFmpeg output or mutable UI settings.
	return strings.TrimSpace(workerStringValue(profile.WorkerConfig["videoFilters"]))
}

func validationProbeDuration(probe map[string]any) float64 {
	format := unknownRecord(probe["format"])
	if format == nil {
		return 0
	}
	value := numberFromUnknown(format["duration"])
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}

func perceptualAlignmentEvidence(sourceStart, duration, candidateStart, candidateDuration float64) AlignmentEvidence {
	return AlignmentEvidence{
		ContractVersion: perceptualAlignmentContractVersion,
		SourceStartPTS:  sourceStart, SourceEndPTS: sourceStart + duration,
		ReferenceStartPTS: 0, ReferenceEndPTS: duration,
		CandidateStartPTS: candidateStart, CandidateEndPTS: candidateStart + math.Min(duration, candidateDuration),
		ToleranceSeconds: 1.0 / 30.0,
	}
}
