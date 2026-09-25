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

func frozenSamplingPlanSourceMatches(path string, plan SamplingPlan) (bool, error) {
	if strings.TrimSpace(plan.SourceRevision) == "" || strings.TrimSpace(plan.SourceFingerprint) == "" {
		return false, nil
	}
	revision, err := sourceRevisionForPath(path)
	if err != nil {
		return false, err
	}
	current := samplingSourceIdentity(path, revision)
	return current == plan.SourceRevision && current == plan.SourceFingerprint, nil
}

// probePerceptualCharacteristics reuses the existing bounded LAB source-probe
// cache for stream-global candidate/reference characteristics. The revision
// is part of the key, so a same-path replacement cannot reuse probe evidence.
func probePerceptualCharacteristics(path, revision string) (previewVideoCharacteristics, error) {
	key := "perceptual-probe:" + path + ":" + revision
	if value, ok := labSourceProbeGet(key); ok {
		return value, nil
	}
	value, err := probePreviewCharacteristics(path)
	if err != nil {
		return previewVideoCharacteristics{}, err
	}
	labSourceProbePut(key, value)
	return value, nil
}

func unavailablePerceptualEvidenceSet(plan SamplingPlan, sourceRevision, status, reason string) (PerceptualEvidenceSet, error) {
	set, err := BuildPerceptualEvidenceSet(plan, sourceRevision, "", nil, nil, nil, []string{reason}, models.JSONMap{"status": status})
	if err != nil {
		return PerceptualEvidenceSet{}, err
	}
	set.Status = status
	set.UnavailableReason = reason
	return set, nil
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
	provenance := models.JSONMap{
		"owner": "shared-perceptual-provider", "candidateKind": input.CandidateKind,
		"metricWork": "sequential", "candidateCoverageWindowCount": len(covered),
	}
	if raw := profile.WorkerConfig[restorationAnalysisSnapshotKey]; raw != nil {
		provenance["restorationAnalysis"] = raw
	}
	set, setErr := BuildPerceptualEvidenceSet(plan, input.SourceRevision, runtimeSnapshot.RuntimeCapabilityIdentity, requested, windowEvidence, capabilityValues, warnings, provenance)
	if setErr == nil {
		labPreviewEvidenceCachePutEvidenceSet(set)
	}
	return set, setErr
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
	candidateStart := 0.0
	if input.FullOutput {
		candidateStart = window.StartSeconds
	} else {
		candidateStart = math.Max(0, window.StartSeconds-input.SegmentStartSeconds)
	}
	candidateTimeline, err := canonicalEvidenceHash("candidate-timeline", models.JSONMap{
		"kind": input.CandidateKind, "revision": input.CandidateRevision,
		"configurationIdentity": input.ConfigurationIdentity, "segmentStartSeconds": input.SegmentStartSeconds, "segmentDurationSeconds": input.SegmentDuration,
	})
	if err != nil {
		return PerceptualWindowEvidence{}, err
	}
	mappingMode := perceptualAlignmentMode(profile, input.FullOutput)
	alignmentValue := perceptualAlignmentEvidenceForWindow(window.StartSeconds, window.DurationSeconds, candidateStart, input.SegmentDuration,
		mappingMode, sourceRevision, transformIdentity, candidateTimeline)
	alignment, err := canonicalEvidenceHash("alignment", alignmentValue)
	if err != nil {
		return PerceptualWindowEvidence{}, err
	}
	sourceFilter, candidateFilter := "null", "null"
	if comparable {
		sourceCharacteristics, sourceErr := probePerceptualCharacteristics(input.SourcePath, sourceRevision)
		candidateCharacteristics, candidateErr := probePerceptualCharacteristics(input.CandidatePath, input.CandidateRevision)
		if sourceErr != nil || candidateErr != nil {
			comparable = false
			if sourceErr != nil {
				transformReason = "canonical comparison source probe failed: " + sourceErr.Error()
			} else {
				transformReason = "canonical comparison candidate probe failed: " + candidateErr.Error()
			}
		} else {
			sourceFilter, candidateFilter = canonicalComparisonDomainFiltersForProfile(sourceCharacteristics, candidateCharacteristics, profile, "")
		}
	}
	comparisonDomain, err := canonicalEvidenceHash("comparison-domain", models.JSONMap{
		"pixelFormat": "yuv420p", "squarePixels": true, "transform": transformIdentity,
		"referenceFilter": sourceFilter, "candidateFilter": candidateFilter,
	})
	if err != nil {
		return PerceptualWindowEvidence{}, err
	}
	reference, err := ComputeReferenceIdentity(ReferenceIdentity{
		SamplingPlanIdentity: plan.PlanIdentity, SamplingWindowIdentity: window.Identity,
		SourceRevision: sourceRevision, SourceStreamIdentity: "0:v:0",
		TransformIdentity: transformIdentity, TransformVersion: perceptualEvidenceSchemaVersion,
		NormalizationProvenance: models.JSONMap{"mode": "preview-normalized", "filter": sourceFilter},
		AlignmentIdentity:       alignment, Alignment: alignmentValue, ComparisonDomainIdentity: comparisonDomain,
	})
	if err != nil {
		return PerceptualWindowEvidence{}, err
	}
	if cached, cacheHit := labPreviewEvidenceCacheGetReference(reference.Identity); cacheHit {
		reference = cached
	} else {
		labPreviewEvidenceCachePutReference(reference)
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
	if cached, cacheHit := labPreviewEvidenceCacheGetPair(pair.Identity); cacheHit {
		pair = cached
	} else {
		labPreviewEvidenceCachePutPair(pair)
	}

	results := make([]MetricResult, 0, len(capabilityValues))
	cacheableResults := make([]MetricResult, 0, len(capabilityValues))
	for _, capability := range capabilityValues {
		status := PerceptualMetricUnavailable
		reason := capability.Reason
		var value *MetricValue
		if !comparable {
			status, reason = PerceptualMetricNonComparable, transformReason
		} else if capability.Availability != "available" {
			if capability.Provider == "" {
				status = PerceptualMetricUnsupported
			}
		} else {
			outputStart := "0"
			if input.FullOutput {
				outputStart = strconv.FormatFloat(candidateStart, 'f', 3, 64)
			}
			descriptor := newMetricDescriptor(capability.Metric, capability.Provider, capability.ProviderVersion, capability.InvocationContract, capability.ModelIdentity, capability.RuntimeCapabilityIdentity, "higher_is_better")
			resultIdentity, identityErr := ComputeMetricResultIdentity(pair, descriptor)
			if identityErr != nil {
				return PerceptualWindowEvidence{}, identityErr
			}
			if cached, cacheHit := labPreviewEvidenceCacheGetMetricResult(resultIdentity.Identity); cacheHit {
				results = append(results, cached)
				continue
			}
			var parsed MetricValue
			var runErr error
			if capability.Metric == PerceptualMetricSSIM || capability.Metric == PerceptualMetricPSNR {
				parsed, runErr = runCanonicalWindowMetricValueWithRunner(ctx, input.SourcePath, strconv.FormatFloat(window.StartSeconds, 'f', 3, 64), sourceFilter, input.CandidatePath, outputStart, candidateFilter, window.DurationSeconds, strings.ToLower(string(capability.Metric)), defaultPreviewMetricProcessRunner)
			} else {
				parsed, runErr = runCanonicalWindowLogicalMetricValueWithRunner(ctx, input.SourcePath, strconv.FormatFloat(window.StartSeconds, 'f', 3, 64), sourceFilter, input.CandidatePath, outputStart, candidateFilter, window.DurationSeconds, capability, defaultPreviewMetricProcessRunner)
			}
			if runErr != nil {
				status, reason = PerceptualMetricFailedMeasurement, runErr.Error()
			} else {
				status, value, reason = PerceptualMetricAvailable, &parsed, ""
			}
			result := MetricResult{Identity: resultIdentity, Status: status, Value: value, Reason: reason}
			if status == PerceptualMetricAvailable {
				cacheableResults = append(cacheableResults, result)
			}
			results = append(results, result)
			continue
		}
		descriptor := newMetricDescriptor(capability.Metric, capability.Provider, capability.ProviderVersion, capability.InvocationContract, capability.ModelIdentity, capability.RuntimeCapabilityIdentity, "higher_is_better")
		resultIdentity, identityErr := ComputeMetricResultIdentity(pair, descriptor)
		if identityErr != nil {
			return PerceptualWindowEvidence{}, identityErr
		}
		results = append(results, MetricResult{Identity: resultIdentity, Status: status, Value: value, Reason: reason})
	}
	if err := ctx.Err(); err != nil {
		return PerceptualWindowEvidence{}, err
	}
	for _, result := range cacheableResults {
		labPreviewEvidenceCachePutMetricResult(result)
	}
	return PerceptualWindowEvidence{
		WindowIdentity: window.Identity, StartSeconds: window.StartSeconds, DurationSeconds: window.DurationSeconds,
		Primary: window.Primary, Roles: window.Roles, Reasons: window.Reasons, EvidenceRefs: window.EvidenceRefs,
		Status: "covered", Reference: &reference, Candidate: &candidateBase, Coverage: &coverage, PairIdentity: &pair,
		Metrics: results, ComparableReason: transformReason,
	}, nil
}

func perceptualReferenceTransform(profile models.Profile) (string, bool, string, error) {
	executionPlan := frozenFidelityExecutionPlan(profile)
	worker := map[string]interface{}(profile.WorkerConfig)
	frozenFilterChain := perceptualReferenceFilter(profile)
	identity, err := canonicalEvidenceHash("fidelity-transform", models.JSONMap{
		"executionPlan": executionPlan, "frozenVideoFilterChain": frozenFilterChain,
	})
	if err != nil {
		return "", false, "", err
	}
	// A raw-source comparison is valid only when the frozen Frame Fidelity
	// authority proves preservation. Transform states require a physical
	// canonical reference adapter and must never silently fall back to raw.
	cadenceOperation := stringFromUnknown(executionPlan["effectiveCadenceOperation"])
	fieldMode := strings.ToLower(strings.TrimSpace(stringFromUnknown(worker["fieldStructureMode"])))
	if (fieldMode == "deinterlace" || fieldMode == "auto" && strings.Contains(strings.ToLower(stringFromUnknown(worker["effectiveAutomaticMotionFilters"])), "deinterlace")) && frozenFilterChain == "" {
		return identity, false, "canonical reference requires the frozen Frame Fidelity field-structure transform; raw source comparison was not used", nil
	}
	if (cadenceOperation != "" && !strings.EqualFold(cadenceOperation, "preserve") || executionPlan["upscale"] != nil || executionPlan["restoration"] != nil) && frozenFilterChain == "" {
		return identity, false, "canonical reference requires the frozen Frame Fidelity video transform; raw source comparison was not used", nil
	}
	if (workerIntValue(worker["effectiveOutputWidth"], 0) > 0 || workerIntValue(worker["effectiveOutputHeight"], 0) > 0) && frozenFilterChain == "" {
		return identity, false, "canonical reference requires the frozen Frame Fidelity output geometry transform; raw source comparison was not used", nil
	}
	// A non-empty chain is the already resolved physical representation owned
	// by Frame Fidelity. It may contain cadence, field, crop, scale, restoration
	// and color stages; do not reinterpret those stages here.
	if frozenFilterChain != "" {
		return identity, true, "", nil
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
	// by parsing current FFmpeg output or mutable UI settings. Some frozen
	// Frame Fidelity snapshots persist automatic motion filters separately, so
	// they are the fallback only when the complete videoFilters chain is absent.
	worker := map[string]interface{}(profile.WorkerConfig)
	if chain := strings.TrimSpace(workerStringValue(worker["videoFilters"])); chain != "" {
		return chain
	}
	return strings.TrimSpace(workerStringValue(worker["effectiveAutomaticMotionFilters"]))
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
	mappingMode := "preserved_or_preview_segment"
	if candidateStart != 0 {
		mappingMode = "trimmed_candidate_segment"
	}
	return perceptualAlignmentEvidenceForWindow(sourceStart, duration, candidateStart, candidateDuration, mappingMode, "source-timeline", "reference-timeline", "candidate-timeline")
}

func perceptualAlignmentEvidenceForWindow(sourceStart, duration, candidateStart, candidateDuration float64, mappingMode, sourceTimeline, referenceTimeline, candidateTimeline string) AlignmentEvidence {
	if strings.TrimSpace(mappingMode) == "" {
		mappingMode = "pts_timeline_mapping"
	}
	if strings.TrimSpace(sourceTimeline) == "" {
		sourceTimeline = "source-timeline"
	}
	if strings.TrimSpace(referenceTimeline) == "" {
		referenceTimeline = "reference-timeline"
	}
	if strings.TrimSpace(candidateTimeline) == "" {
		candidateTimeline = "candidate-timeline"
	}
	timelineIdentity, _ := canonicalEvidenceHash("timeline", models.JSONMap{
		"contract": perceptualAlignmentContractVersion, "source": sourceTimeline, "reference": referenceTimeline,
		"candidate": candidateTimeline, "sourceStart": sourceStart, "duration": duration,
		"candidateStart": candidateStart, "candidateDuration": candidateDuration, "mappingMode": mappingMode,
	})
	return AlignmentEvidence{
		ContractVersion: perceptualAlignmentContractVersion, TimelineIdentity: timelineIdentity,
		SourceTimelineIdentity: sourceTimeline, ReferenceTimelineIdentity: referenceTimeline, CandidateTimelineIdentity: candidateTimeline,
		TimestampOrigin: "normalized-zero-based", MappingMode: mappingMode, FramePairingMode: "pts_with_tolerance",
		SourceStartPTS: sourceStart, SourceEndPTS: sourceStart + duration,
		ReferenceStartPTS: 0, ReferenceEndPTS: duration,
		CandidateStartPTS: candidateStart, CandidateEndPTS: candidateStart + math.Min(duration, candidateDuration),
		ToleranceSeconds: 1.0 / 30.0,
	}
}

func perceptualAlignmentMode(profile models.Profile, fullOutput bool) string {
	base := "trimmed_candidate_segment"
	if fullOutput {
		base = "full_output_source_timeline"
	}
	executionPlan := frozenFidelityExecutionPlan(profile)
	cadence := strings.ToLower(strings.TrimSpace(stringFromUnknown(executionPlan["effectiveCadenceOperation"])))
	fieldMode := strings.ToLower(strings.TrimSpace(stringFromUnknown(profile.WorkerConfig["fieldStructureMode"])))
	filters := strings.ToLower(perceptualReferenceFilter(profile))
	suffix := ""
	switch {
	case strings.Contains(cadence, "inverse_telecine") || strings.Contains(cadence, "ivtc") || strings.Contains(filters, "fieldmatch") && strings.Contains(filters, "decimate"):
		suffix = "ivtc"
	case fieldMode == "deinterlace" || strings.Contains(filters, "bwdif") || strings.Contains(filters, "yadif"):
		suffix = "deinterlace"
	case strings.TrimSpace(stringFromUnknown(executionPlan["effectiveOutputFrameRate"])) != "" && cadence != "" && cadence != "preserve":
		suffix = "fps_conversion"
	case cadence != "" && cadence != "preserve":
		suffix = "cadence_transform"
	}
	if suffix == "" {
		return base
	}
	return base + "+" + suffix
}
