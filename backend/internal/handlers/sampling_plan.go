package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
)

const (
	samplingPlanSchemaVersion     = "sampling-plan/v1"
	samplingPlanPolicyVersion     = "quality-hard-scene/v1"
	samplingWindowIdentityVersion = "timeline-window/v1"
	samplingFallbackDistributed   = "distributed-baseline"
	samplingDefaultMaximumWindows = 6
	samplingDefaultSummaryLimit   = 12
)

const (
	SamplingRoleTypical            = "typical"
	SamplingRoleHighMotion         = "high_motion"
	SamplingRoleHighTextureOrGrain = "high_texture_or_grain"
	SamplingRoleDarkOrGradient     = "dark_or_gradient"
	SamplingRoleHighComplexity     = "high_complexity"
	SamplingRoleWorstPredicted     = "worst_predicted"
)

var samplingRolePriority = map[string]int{
	SamplingRoleWorstPredicted:     0,
	SamplingRoleHighComplexity:     1,
	SamplingRoleHighMotion:         2,
	SamplingRoleHighTextureOrGrain: 3,
	SamplingRoleDarkOrGradient:     4,
	SamplingRoleTypical:            5,
}

type SamplingEvidence struct {
	Identity     string             `json:"identity"`
	Source       string             `json:"source"`
	Version      string             `json:"version,omitempty"`
	Scope        string             `json:"scope"`
	Availability string             `json:"availability"`
	Confidence   float64            `json:"confidence,omitempty"`
	Signals      map[string]float64 `json:"signals,omitempty"`
	Weak         bool               `json:"weak,omitempty"`
}

type SamplingCandidate struct {
	Window       SamplingWindow     `json:"window"`
	Roles        []string           `json:"roles,omitempty"`
	Reasons      []string           `json:"reasons,omitempty"`
	EvidenceRefs []string           `json:"evidenceRefs,omitempty"`
	Signals      map[string]float64 `json:"signals,omitempty"`
	Confidence   float64            `json:"confidence,omitempty"`
}

type SamplingWindowSummary struct {
	Identity        string   `json:"identity"`
	StartSeconds    float64  `json:"startSeconds"`
	DurationSeconds float64  `json:"durationSeconds"`
	Roles           []string `json:"roles,omitempty"`
	Reasons         []string `json:"reasons,omitempty"`
}

type SamplingCandidateSummary struct {
	InputCount      int                     `json:"inputCount"`
	NormalizedCount int                     `json:"normalizedCount"`
	SelectedCount   int                     `json:"selectedCount"`
	Discarded       []SamplingWindowSummary `json:"discarded,omitempty"`
	Consolidated    []SamplingWindowSummary `json:"consolidated,omitempty"`
}

type SamplingFallback struct {
	Used                bool     `json:"used"`
	Kind                string   `json:"kind,omitempty"`
	Reason              string   `json:"reason,omitempty"`
	UnavailableRoles    []string `json:"unavailableRoles,omitempty"`
	UnavailableEvidence []string `json:"unavailableEvidence,omitempty"`
}

type SamplingLimits struct {
	MaximumWindows           int `json:"maximumWindows"`
	MaximumCandidates        int `json:"maximumCandidates"`
	MaximumCandidatesPerRole int `json:"maximumCandidatesPerRole"`
	MaximumEvidence          int `json:"maximumEvidence"`
	MaximumSummaries         int `json:"maximumSummaries"`
}

type SamplingPlan struct {
	AssetDurationSeconds     float64   `json:"assetDurationSeconds"`
	WindowSeconds            float64   `json:"windowSeconds"`
	Positions                []float64 `json:"positions"`
	Adaptive                 bool      `json:"adaptive"`
	InitialWindows           int       `json:"initialWindows"`
	EarlyConfidenceEnabled   bool      `json:"earlyConfidenceEnabled"`
	EarlyConfidenceThreshold float64   `json:"earlyConfidenceThreshold"`
	InterlaceValidation      string    `json:"interlaceValidation"`
	CropDepth                string    `json:"cropDepth"`

	SchemaVersion        string                   `json:"schemaVersion,omitempty"`
	PolicyVersion        string                   `json:"policyVersion,omitempty"`
	PlanIdentity         string                   `json:"planIdentity,omitempty"`
	SourceFingerprint    string                   `json:"sourceFingerprint,omitempty"`
	SourceRevision       string                   `json:"sourceRevision,omitempty"`
	SourceStreamIdentity string                   `json:"sourceStreamIdentity,omitempty"`
	NormalizationVersion string                   `json:"normalizationVersion,omitempty"`
	Evidence             []SamplingEvidence       `json:"evidence,omitempty"`
	SelectedWindows      []SamplingWindow         `json:"selectedWindows,omitempty"`
	Fallback             SamplingFallback         `json:"fallback,omitempty"`
	Limits               SamplingLimits           `json:"limits,omitempty"`
	CandidateSummary     SamplingCandidateSummary `json:"candidateSummary,omitempty"`
	LegacyBaseline       bool                     `json:"legacyBaseline,omitempty"`
}

type SamplingWindow struct {
	Position        float64            `json:"position"`
	StartSeconds    float64            `json:"startSeconds"`
	DurationSeconds float64            `json:"durationSeconds"`
	Identity        string             `json:"identity,omitempty"`
	Primary         bool               `json:"primary,omitempty"`
	Roles           []string           `json:"roles,omitempty"`
	Reasons         []string           `json:"reasons,omitempty"`
	EvidenceRefs    []string           `json:"evidenceRefs,omitempty"`
	Scores          map[string]float64 `json:"scores,omitempty"`
	Confidence      float64            `json:"confidence,omitempty"`
}

type SamplingPlanInput struct {
	BasePlan             SamplingPlan
	SourceFingerprint    string
	SourceRevision       string
	SourceStreamIdentity string
	PolicyVersion        string
	FrameDurationSeconds float64
	Primary              *SamplingWindow
	Evidence             []SamplingEvidence
	Candidates           []SamplingCandidate
}

type SamplingPlanRoleSelection struct {
	PlanIdentity     string           `json:"planIdentity"`
	PolicyVersion    string           `json:"policyVersion"`
	Windows          []SamplingWindow `json:"windows,omitempty"`
	RequestedRoles   []string         `json:"requestedRoles,omitempty"`
	UnavailableRoles []string         `json:"unavailableRoles,omitempty"`
}

func canonicalSamplingPlan(duration float64, policy FrameStructureSamplingPolicy) SamplingPlan {
	if policy.Windows <= 0 {
		policy = defaultFrameStructureSamplingPolicy()
	}
	windowSeconds := policy.WindowSeconds
	positions := policy.Positions
	if len(positions) < policy.Windows {
		positions = defaultFrameStructureSamplingPolicy().Positions
	}
	if policy.Windows > len(positions) {
		policy.Windows = len(positions)
	}
	if policy.InitialWindows <= 0 {
		policy.InitialWindows = min(3, policy.Windows)
	}
	if policy.EarlyConfidenceThreshold <= 0 {
		policy.EarlyConfidenceThreshold = 0.98
	}
	if policy.InterlaceValidation == "" {
		policy.InterlaceValidation = "automatic"
	}
	if policy.CropDepth == "" {
		policy.CropDepth = "normal"
	}
	if policy.Adaptive {
		switch {
		case duration > 0 && duration <= 60:
			windowSeconds = duration
			policy.Windows = 1
			positions = []float64{0.5}
		case duration < 120:
			windowSeconds = 10
		case duration < 600:
			windowSeconds = 15
		default:
			windowSeconds = 20
		}
	}
	if windowSeconds <= 0 {
		windowSeconds = 20
	}
	return SamplingPlan{
		AssetDurationSeconds:     duration,
		WindowSeconds:            windowSeconds,
		Positions:                append([]float64(nil), positions[:policy.Windows]...),
		Adaptive:                 policy.Adaptive,
		InitialWindows:           max(1, min(policy.InitialWindows, policy.Windows)),
		EarlyConfidenceEnabled:   policy.EarlyConfidenceEnabled,
		EarlyConfidenceThreshold: policy.EarlyConfidenceThreshold,
		InterlaceValidation:      policy.InterlaceValidation,
		CropDepth:                policy.CropDepth,
	}
}

func defaultSamplingLimits() SamplingLimits {
	return SamplingLimits{
		MaximumWindows:           samplingDefaultMaximumWindows,
		MaximumCandidates:        32,
		MaximumCandidatesPerRole: 2,
		MaximumEvidence:          24,
		MaximumSummaries:         samplingDefaultSummaryLimit,
	}
}

func resolveCanonicalSamplingPlan(input SamplingPlanInput) SamplingPlan {
	base := input.BasePlan
	if base.AssetDurationSeconds <= 0 {
		base.AssetDurationSeconds = 0
	}
	if len(base.Positions) == 0 {
		base = canonicalSamplingPlan(base.AssetDurationSeconds, defaultFrameStructureSamplingPolicy())
	}
	if base.SchemaVersion == "" {
		base.SchemaVersion = samplingPlanSchemaVersion
	}
	if input.PolicyVersion != "" {
		base.PolicyVersion = input.PolicyVersion
	}
	if base.PolicyVersion == "" {
		base.PolicyVersion = samplingPlanPolicyVersion
	}
	base.SourceFingerprint = input.SourceFingerprint
	base.SourceRevision = input.SourceRevision
	base.SourceStreamIdentity = input.SourceStreamIdentity
	if base.SourceStreamIdentity == "" {
		base.SourceStreamIdentity = "0:v:0"
	}
	base.NormalizationVersion = samplingWindowIdentityVersion
	base.Limits = normalizedSamplingLimits(base.Limits)
	base.Evidence = normalizeSamplingEvidence(input.Evidence, base.Limits.MaximumEvidence)

	intervalEvidence := make(map[string]SamplingEvidence, len(base.Evidence))
	for _, evidence := range base.Evidence {
		intervalEvidence[evidence.Identity] = evidence
	}

	type candidateRecord struct {
		candidate SamplingCandidate
		order     int
	}
	records := make([]candidateRecord, 0, len(base.Positions)+len(input.Candidates)+1)
	if input.Primary != nil {
		primary := *input.Primary
		primary.Primary = true
		records = append(records, candidateRecord{candidate: SamplingCandidate{
			Window:  primary,
			Roles:   []string{SamplingRoleTypical},
			Reasons: []string{"explicit_primary"},
		}, order: -1})
	}
	for index, window := range base.windows(base.WindowSeconds) {
		records = append(records, candidateRecord{candidate: SamplingCandidate{
			Window:  window,
			Roles:   []string{SamplingRoleTypical},
			Reasons: []string{"distributed_baseline"},
		}, order: index})
	}
	for index, candidate := range input.Candidates {
		records = append(records, candidateRecord{candidate: candidate, order: index + 1})
	}

	inputCount := len(records)
	if len(records) > base.Limits.MaximumCandidates {
		sort.SliceStable(records, func(i, j int) bool {
			return samplingCandidateSortKey(records[i].candidate) < samplingCandidateSortKey(records[j].candidate)
		})
		records = records[:base.Limits.MaximumCandidates]
	}

	consolidated := make([]SamplingWindowSummary, 0, base.Limits.MaximumSummaries)
	normalizedCount := 0
	merged := make([]candidateRecord, 0, len(records))
	for _, record := range records {
		window, ok := normalizeSamplingWindow(record.candidate.Window, base.AssetDurationSeconds, input.FrameDurationSeconds, base.WindowSeconds, base.SourceFingerprint, base.SourceStreamIdentity)
		if !ok {
			continue
		}
		normalizedCount++
		record.candidate.Window = window
		record.candidate.Roles = normalizeSamplingRoles(record.candidate.Roles)
		record.candidate.Reasons = normalizeSamplingStrings(record.candidate.Reasons)
		record.candidate.EvidenceRefs = normalizeSamplingStrings(record.candidate.EvidenceRefs)
		applyEvidenceToCandidate(&record.candidate, intervalEvidence)
		record.candidate.Roles = samplingRolesBackedByIntervalEvidence(record.candidate, intervalEvidence)

		match := -1
		for index := range merged {
			if samplingWindowsEquivalent(merged[index].candidate.Window, record.candidate.Window, input.FrameDurationSeconds) {
				match = index
				break
			}
		}
		if match < 0 {
			merged = append(merged, record)
			continue
		}
		merged[match].candidate = mergeSamplingCandidates(merged[match].candidate, record.candidate)
		if len(consolidated) < base.Limits.MaximumSummaries {
			consolidated = append(consolidated, samplingWindowSummary(record.candidate.Window))
		}
	}

	hardEvidenceAvailable := false
	for index := range merged {
		candidate := &merged[index].candidate
		if candidateHasIntervalEvidence(*candidate, intervalEvidence) {
			hardEvidenceAvailable = true
		}
		candidate.Window.Roles = normalizeSamplingRoles(candidate.Roles)
		candidate.Window.Reasons = normalizeSamplingStrings(candidate.Reasons)
		candidate.Window.EvidenceRefs = normalizeSamplingStrings(candidate.EvidenceRefs)
		candidate.Window.Scores = normalizedSamplingScores(candidate.Signals)
		candidate.Window.Confidence = quantizeSamplingScore(candidate.Confidence)
	}

	primaryIndex := -1
	for index := range merged {
		if merged[index].candidate.Window.Primary {
			primaryIndex = index
			break
		}
	}
	if primaryIndex < 0 && input.Primary != nil {
		// A malformed/zero-length explicit primary is not allowed to replace the baseline.
		base.Fallback.UnavailableRoles = append(base.Fallback.UnavailableRoles, "primary")
	}

	sort.SliceStable(merged, func(i, j int) bool {
		return samplingCandidateRankLess(merged[i].candidate, merged[j].candidate)
	})
	selected := make([]SamplingWindow, 0, base.Limits.MaximumWindows)
	if primaryIndex >= 0 {
		for index := range merged {
			if merged[index].candidate.Window.Primary {
				selected = append(selected, merged[index].candidate.Window)
				break
			}
		}
	}
	roleCounts := map[string]int{}
	for _, record := range merged {
		if len(selected) >= base.Limits.MaximumWindows {
			break
		}
		window := record.candidate.Window
		if window.Primary || !samplingWindowHasRole(window, SamplingRoleTypical, SamplingRoleHighMotion, SamplingRoleHighTextureOrGrain, SamplingRoleDarkOrGradient, SamplingRoleHighComplexity, SamplingRoleWorstPredicted) {
			continue
		}
		if samplingWindowAlreadySelected(selected, window, input.FrameDurationSeconds) {
			continue
		}
		eligibleRole := false
		for _, role := range window.Roles {
			if role == SamplingRoleTypical || roleCounts[role] < base.Limits.MaximumCandidatesPerRole {
				eligibleRole = true
				break
			}
		}
		if !eligibleRole {
			continue
		}
		selected = append(selected, window)
		for _, role := range window.Roles {
			if role != SamplingRoleTypical && roleCounts[role] < base.Limits.MaximumCandidatesPerRole {
				roleCounts[role]++
			}
		}
	}
	if len(selected) == 0 {
		for _, record := range merged {
			if len(selected) >= base.Limits.MaximumWindows {
				break
			}
			if !samplingWindowAlreadySelected(selected, record.candidate.Window, input.FrameDurationSeconds) {
				selected = append(selected, record.candidate.Window)
			}
		}
	}
	if !hardEvidenceAvailable {
		base.Fallback = SamplingFallback{
			Used:   true,
			Kind:   samplingFallbackDistributed,
			Reason: "insufficient_interval_evidence",
		}
		for _, role := range []string{SamplingRoleHighMotion, SamplingRoleHighTextureOrGrain, SamplingRoleDarkOrGradient, SamplingRoleHighComplexity, SamplingRoleWorstPredicted} {
			base.Fallback.UnavailableRoles = append(base.Fallback.UnavailableRoles, role)
		}
		for _, evidence := range base.Evidence {
			if evidence.Availability != "available" && len(base.Fallback.UnavailableEvidence) < base.Limits.MaximumSummaries {
				base.Fallback.UnavailableEvidence = append(base.Fallback.UnavailableEvidence, evidence.Identity)
			}
		}
	}
	discarded := make([]SamplingWindowSummary, 0, base.Limits.MaximumSummaries)
	for _, record := range merged {
		if samplingWindowAlreadySelected(selected, record.candidate.Window, input.FrameDurationSeconds) {
			continue
		}
		if len(discarded) >= base.Limits.MaximumSummaries {
			break
		}
		discarded = append(discarded, samplingWindowSummary(record.candidate.Window))
	}
	base.SelectedWindows = selected
	base.CandidateSummary = SamplingCandidateSummary{
		InputCount:      inputCount,
		NormalizedCount: normalizedCount,
		SelectedCount:   len(selected),
		Discarded:       discarded,
		Consolidated:    consolidated,
	}
	base.Evidence = normalizeSamplingEvidence(base.Evidence, base.Limits.MaximumEvidence)
	base.PlanIdentity = ""
	base.PlanIdentity = samplingPlanIdentity(base)
	return base
}

func normalizedSamplingLimits(limits SamplingLimits) SamplingLimits {
	defaults := defaultSamplingLimits()
	if limits.MaximumWindows <= 0 || limits.MaximumWindows > defaults.MaximumWindows {
		limits.MaximumWindows = defaults.MaximumWindows
	}
	if limits.MaximumCandidates <= 0 || limits.MaximumCandidates > defaults.MaximumCandidates {
		limits.MaximumCandidates = defaults.MaximumCandidates
	}
	if limits.MaximumCandidatesPerRole <= 0 || limits.MaximumCandidatesPerRole > defaults.MaximumCandidatesPerRole {
		limits.MaximumCandidatesPerRole = defaults.MaximumCandidatesPerRole
	}
	if limits.MaximumEvidence <= 0 || limits.MaximumEvidence > defaults.MaximumEvidence {
		limits.MaximumEvidence = defaults.MaximumEvidence
	}
	if limits.MaximumSummaries <= 0 || limits.MaximumSummaries > defaults.MaximumSummaries {
		limits.MaximumSummaries = defaults.MaximumSummaries
	}
	return limits
}

func normalizeSamplingEvidence(evidence []SamplingEvidence, maximum int) []SamplingEvidence {
	if maximum <= 0 {
		maximum = samplingDefaultSummaryLimit * 2
	}
	copyEvidence := append([]SamplingEvidence(nil), evidence...)
	for index := range copyEvidence {
		copyEvidence[index].Identity = strings.TrimSpace(copyEvidence[index].Identity)
		copyEvidence[index].Source = strings.TrimSpace(copyEvidence[index].Source)
		copyEvidence[index].Scope = strings.TrimSpace(copyEvidence[index].Scope)
		copyEvidence[index].Availability = strings.TrimSpace(copyEvidence[index].Availability)
		copyEvidence[index].Confidence = quantizeSamplingScore(copyEvidence[index].Confidence)
		copyEvidence[index].Signals = normalizedSamplingScores(copyEvidence[index].Signals)
		if copyEvidence[index].Identity == "" {
			copyEvidence[index].Identity = samplingEvidenceIdentity(copyEvidence[index])
		}
	}
	sort.Slice(copyEvidence, func(i, j int) bool {
		return copyEvidence[i].Identity < copyEvidence[j].Identity
	})
	if len(copyEvidence) > maximum {
		copyEvidence = copyEvidence[:maximum]
	}
	return copyEvidence
}

func samplingEvidenceIdentity(evidence SamplingEvidence) string {
	payload := strings.Join([]string{evidence.Source, evidence.Version, evidence.Scope, evidence.Availability}, "|")
	hash := sha256.Sum256([]byte(payload))
	return "evidence:" + hex.EncodeToString(hash[:8])
}

func normalizeSamplingWindow(window SamplingWindow, assetDuration, frameDuration, defaultDuration float64, sourceFingerprint, streamIdentity string) (SamplingWindow, bool) {
	if !isFiniteSamplingNumber(window.StartSeconds) || !isFiniteSamplingNumber(window.DurationSeconds) || window.DurationSeconds <= 0 {
		return SamplingWindow{}, false
	}
	if assetDuration > 0 {
		if window.StartSeconds > assetDuration {
			window.StartSeconds = assetDuration
		}
		window.StartSeconds = math.Max(0, window.StartSeconds)
		window.DurationSeconds = math.Min(window.DurationSeconds, assetDuration-window.StartSeconds)
	}
	if window.DurationSeconds <= 0 {
		return SamplingWindow{}, false
	}
	if defaultDuration > 0 && !window.Primary && window.DurationSeconds > defaultDuration {
		window.DurationSeconds = defaultDuration
		if assetDuration > 0 && window.StartSeconds+window.DurationSeconds > assetDuration {
			window.StartSeconds = math.Max(0, assetDuration-window.DurationSeconds)
		}
	}
	window.StartSeconds = samplingRoundTimeline(window.StartSeconds)
	window.DurationSeconds = samplingRoundTimeline(window.DurationSeconds)
	if assetDuration > 0 && window.StartSeconds+window.DurationSeconds > assetDuration {
		window.DurationSeconds = samplingRoundTimeline(assetDuration - window.StartSeconds)
	}
	if window.DurationSeconds <= 0 {
		return SamplingWindow{}, false
	}
	if !isFiniteSamplingNumber(window.Position) || window.Position < 0 || window.Position > 1 {
		center := window.StartSeconds + window.DurationSeconds/2
		if assetDuration > 0 {
			window.Position = center / assetDuration
		} else {
			window.Position = 0
		}
	}
	window.Position = math.Max(0, math.Min(1, samplingRoundTimeline(window.Position)))
	window.Identity = samplingWindowIdentity(sourceFingerprint, streamIdentity, window.StartSeconds, window.DurationSeconds)
	return window, true
}

func isFiniteSamplingNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func samplingRoundTimeline(value float64) float64 {
	return math.Round(value*1000) / 1000
}

func samplingWindowIdentity(sourceFingerprint, streamIdentity string, start, duration float64) string {
	payload := strings.Join([]string{
		samplingWindowIdentityVersion,
		sourceFingerprint,
		streamIdentity,
		strconv.FormatFloat(samplingRoundTimeline(start), 'f', 3, 64),
		strconv.FormatFloat(samplingRoundTimeline(duration), 'f', 3, 64),
	}, "|")
	hash := sha256.Sum256([]byte(payload))
	return "window:" + hex.EncodeToString(hash[:12])
}

func samplingWindowTolerance(frameDuration float64) float64 {
	if frameDuration <= 0 || !isFiniteSamplingNumber(frameDuration) {
		return 1.0 / 30.0
	}
	return math.Max(0.001, frameDuration)
}

func samplingWindowsEquivalent(left, right SamplingWindow, frameDuration float64) bool {
	tolerance := samplingWindowTolerance(frameDuration)
	leftEnd := left.StartSeconds + left.DurationSeconds
	rightEnd := right.StartSeconds + right.DurationSeconds
	return math.Abs(left.StartSeconds-right.StartSeconds) <= tolerance && math.Abs(leftEnd-rightEnd) <= tolerance
}

func mergeSamplingCandidates(left, right SamplingCandidate) SamplingCandidate {
	left.Roles = normalizeSamplingRoles(append(left.Roles, right.Roles...))
	left.Reasons = normalizeSamplingStrings(append(left.Reasons, right.Reasons...))
	left.EvidenceRefs = normalizeSamplingStrings(append(left.EvidenceRefs, right.EvidenceRefs...))
	left.Signals = mergeSamplingScores(left.Signals, right.Signals)
	left.Confidence = math.Max(left.Confidence, right.Confidence)
	if right.Window.Primary {
		left.Window.Primary = true
	}
	return left
}

func mergeSamplingScores(left, right map[string]float64) map[string]float64 {
	merged := map[string]float64{}
	for key, value := range left {
		merged[key] = value
	}
	for key, value := range right {
		if current, ok := merged[key]; !ok || value > current {
			merged[key] = value
		}
	}
	return merged
}

func normalizeSamplingStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func normalizeSamplingRoles(values []string) []string {
	result := normalizeSamplingStrings(values)
	filtered := result[:0]
	for _, role := range result {
		if _, ok := samplingRolePriority[role]; ok {
			filtered = append(filtered, role)
		}
	}
	return filtered
}

func normalizedSamplingScores(values map[string]float64) map[string]float64 {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]float64, len(values))
	for key, value := range values {
		if !isFiniteSamplingNumber(value) {
			continue
		}
		result[strings.TrimSpace(key)] = quantizeSamplingScore(math.Max(0, math.Min(1, value)))
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func quantizeSamplingScore(value float64) float64 {
	if !isFiniteSamplingNumber(value) {
		return 0
	}
	return math.Round(math.Max(0, math.Min(1, value))*1000) / 1000
}

func applyEvidenceToCandidate(candidate *SamplingCandidate, evidence map[string]SamplingEvidence) {
	for _, ref := range candidate.EvidenceRefs {
		item, ok := evidence[ref]
		if !ok || item.Scope != "interval" || item.Availability != "available" {
			continue
		}
		candidate.Confidence = math.Max(candidate.Confidence, item.Confidence)
		candidate.Signals = mergeSamplingScores(candidate.Signals, item.Signals)
		for role, key := range map[string]string{
			SamplingRoleHighMotion:         "motion",
			SamplingRoleHighTextureOrGrain: "texture_or_grain",
			SamplingRoleDarkOrGradient:     "dark_or_gradient",
			SamplingRoleHighComplexity:     "complexity",
			SamplingRoleWorstPredicted:     "worst_predicted",
		} {
			if score, ok := item.Signals[key]; ok && score >= 0.6 {
				candidate.Roles = append(candidate.Roles, role)
				candidate.Reasons = append(candidate.Reasons, "evidence:"+ref)
			}
		}
	}
	candidate.Roles = normalizeSamplingRoles(candidate.Roles)
	candidate.Reasons = normalizeSamplingStrings(candidate.Reasons)
	candidate.EvidenceRefs = normalizeSamplingStrings(candidate.EvidenceRefs)
}

func candidateHasIntervalEvidence(candidate SamplingCandidate, evidence map[string]SamplingEvidence) bool {
	for _, ref := range candidate.EvidenceRefs {
		item, ok := evidence[ref]
		if ok && item.Scope == "interval" && item.Availability == "available" {
			for role := range samplingRolePriority {
				if role != SamplingRoleTypical && containsSamplingString(candidate.Roles, role) {
					return true
				}
			}
		}
	}
	return false
}

func samplingRolesBackedByIntervalEvidence(candidate SamplingCandidate, evidence map[string]SamplingEvidence) []string {
	roles := make([]string, 0, len(candidate.Roles))
	for _, role := range candidate.Roles {
		if role == SamplingRoleTypical {
			roles = append(roles, role)
			continue
		}
		key := map[string]string{
			SamplingRoleHighMotion:         "motion",
			SamplingRoleHighTextureOrGrain: "texture_or_grain",
			SamplingRoleDarkOrGradient:     "dark_or_gradient",
			SamplingRoleHighComplexity:     "complexity",
			SamplingRoleWorstPredicted:     "worst_predicted",
		}[role]
		backed := false
		for _, ref := range candidate.EvidenceRefs {
			item, ok := evidence[ref]
			if ok && item.Scope == "interval" && item.Availability == "available" && item.Signals[key] >= 0.6 {
				backed = true
				break
			}
		}
		if backed {
			roles = append(roles, role)
		}
	}
	return normalizeSamplingRoles(roles)
}

func samplingCandidateSortKey(candidate SamplingCandidate) string {
	return strings.Join([]string{
		strconv.FormatFloat(candidate.Window.StartSeconds, 'f', 6, 64),
		strconv.FormatFloat(candidate.Window.DurationSeconds, 'f', 6, 64),
		candidate.Window.Identity,
	}, "|")
}

func samplingCandidateRankLess(left, right SamplingCandidate) bool {
	leftPrimary, rightPrimary := left.Window.Primary, right.Window.Primary
	if leftPrimary != rightPrimary {
		return leftPrimary
	}
	leftScore, rightScore := samplingCandidateScore(left), samplingCandidateScore(right)
	if leftScore != rightScore {
		return leftScore > rightScore
	}
	if left.Confidence != right.Confidence {
		return left.Confidence > right.Confidence
	}
	leftPriority, rightPriority := samplingCandidateRolePriority(left.Roles), samplingCandidateRolePriority(right.Roles)
	if leftPriority != rightPriority {
		return leftPriority < rightPriority
	}
	if left.Window.StartSeconds != right.Window.StartSeconds {
		return left.Window.StartSeconds < right.Window.StartSeconds
	}
	if left.Window.DurationSeconds != right.Window.DurationSeconds {
		return left.Window.DurationSeconds < right.Window.DurationSeconds
	}
	return left.Window.Identity < right.Window.Identity
}

func samplingCandidateScore(candidate SamplingCandidate) float64 {
	score := 0.0
	for _, role := range candidate.Roles {
		if candidate.Signals != nil {
			key := map[string]string{
				SamplingRoleHighMotion:         "motion",
				SamplingRoleHighTextureOrGrain: "texture_or_grain",
				SamplingRoleDarkOrGradient:     "dark_or_gradient",
				SamplingRoleHighComplexity:     "complexity",
				SamplingRoleWorstPredicted:     "worst_predicted",
			}[role]
			if candidate.Signals[key] > score {
				score = candidate.Signals[key]
			}
		}
	}
	return quantizeSamplingScore(score)
}

func samplingCandidateRolePriority(roles []string) int {
	priority := len(samplingRolePriority) + 1
	for _, role := range roles {
		if value, ok := samplingRolePriority[role]; ok && value < priority {
			priority = value
		}
	}
	return priority
}

func samplingWindowSummary(window SamplingWindow) SamplingWindowSummary {
	return SamplingWindowSummary{
		Identity:        window.Identity,
		StartSeconds:    window.StartSeconds,
		DurationSeconds: window.DurationSeconds,
		Roles:           append([]string(nil), window.Roles...),
		Reasons:         append([]string(nil), window.Reasons...),
	}
}

func samplingWindowAlreadySelected(selected []SamplingWindow, candidate SamplingWindow, frameDuration float64) bool {
	for _, current := range selected {
		if samplingWindowsEquivalent(current, candidate, frameDuration) {
			return true
		}
	}
	return false
}

func samplingWindowHasRole(window SamplingWindow, roles ...string) bool {
	for _, role := range roles {
		if containsSamplingString(window.Roles, role) {
			return true
		}
	}
	return false
}

func containsSamplingString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func samplingPlanIdentity(plan SamplingPlan) string {
	copyPlan := plan
	copyPlan.PlanIdentity = ""
	encoded, _ := json.Marshal(copyPlan)
	hash := sha256.Sum256(encoded)
	return "plan:" + hex.EncodeToString(hash[:16])
}

func samplingPlanMap(plan SamplingPlan) map[string]any {
	encoded, _ := json.Marshal(plan)
	var result map[string]any
	if err := json.Unmarshal(encoded, &result); err != nil {
		return map[string]any{}
	}
	return result
}

func samplingPlanFromMap(value map[string]any) (SamplingPlan, bool) {
	if len(value) == 0 {
		return SamplingPlan{}, false
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return SamplingPlan{}, false
	}
	var plan SamplingPlan
	if err := json.Unmarshal(encoded, &plan); err != nil {
		return SamplingPlan{}, false
	}
	if len(plan.SelectedWindows) == 0 && len(plan.Positions) > 0 {
		plan.LegacyBaseline = true
		plan.Fallback = SamplingFallback{Used: true, Kind: samplingFallbackDistributed, Reason: "legacy_position_only"}
	}
	return plan, true
}

// SamplingPlanWindowsForRoles is a read-only consumer contract. It filters the
// frozen selected windows and never performs selection or fallback reselection.
func SamplingPlanWindowsForRoles(plan SamplingPlan, roles ...string) SamplingPlanRoleSelection {
	requested := normalizeSamplingRoles(roles)
	selection := SamplingPlanRoleSelection{
		PlanIdentity:   plan.PlanIdentity,
		PolicyVersion:  plan.PolicyVersion,
		RequestedRoles: requested,
	}
	if len(requested) == 0 {
		selection.Windows = append([]SamplingWindow(nil), plan.SelectedWindows...)
		return selection
	}
	for _, role := range requested {
		found := false
		for _, window := range plan.SelectedWindows {
			if containsSamplingString(window.Roles, role) {
				found = true
				if !samplingWindowAlreadySelected(selection.Windows, window, 0) {
					selection.Windows = append(selection.Windows, window)
				}
			}
		}
		if !found {
			selection.UnavailableRoles = append(selection.UnavailableRoles, role)
		}
	}
	sort.Slice(selection.Windows, func(i, j int) bool {
		return samplingWindowOrderLess(selection.Windows[i], selection.Windows[j])
	})
	return selection
}

// SamplingPlanForPerceptualQuality is the #69 read-only consumer contract.
// The consumer receives the frozen windows and may not select replacements.
func SamplingPlanForPerceptualQuality(plan SamplingPlan) SamplingPlanRoleSelection {
	return SamplingPlanWindowsForRoles(plan)
}

// SamplingPlanForQSVPumping is the #9 read-only consumer contract. Missing
// roles remain explicitly unavailable instead of triggering a new selector.
func SamplingPlanForQSVPumping(plan SamplingPlan) SamplingPlanRoleSelection {
	return SamplingPlanWindowsForRoles(plan, SamplingRoleHighMotion, SamplingRoleHighTextureOrGrain, SamplingRoleWorstPredicted)
}

func samplingWindowOrderLess(left, right SamplingWindow) bool {
	if left.Primary != right.Primary {
		return left.Primary
	}
	if left.StartSeconds != right.StartSeconds {
		return left.StartSeconds < right.StartSeconds
	}
	if left.DurationSeconds != right.DurationSeconds {
		return left.DurationSeconds < right.DurationSeconds
	}
	return left.Identity < right.Identity
}

func samplingPlanIsStale(plan SamplingPlan, sourceFingerprint, sourceRevision, policyVersion string, evidence []SamplingEvidence) bool {
	if plan.SourceFingerprint != sourceFingerprint || plan.SourceRevision != sourceRevision || plan.PolicyVersion != policyVersion {
		return true
	}
	current := normalizeSamplingEvidence(evidence, plan.Limits.MaximumEvidence)
	return !samplingEvidenceIdentitySetEqual(plan.Evidence, current)
}

func samplingEvidenceIdentitySetEqual(left, right []SamplingEvidence) bool {
	leftIDs := make([]string, 0, len(left))
	rightIDs := make([]string, 0, len(right))
	for _, item := range left {
		leftIDs = append(leftIDs, strings.Join([]string{item.Identity, item.Source, item.Version, item.Scope, item.Availability}, "|"))
	}
	for _, item := range right {
		rightIDs = append(rightIDs, strings.Join([]string{item.Identity, item.Source, item.Version, item.Scope, item.Availability}, "|"))
	}
	sort.Strings(leftIDs)
	sort.Strings(rightIDs)
	return strings.Join(leftIDs, "\x00") == strings.Join(rightIDs, "\x00")
}

func (plan SamplingPlan) cropWindowMaximum() int {
	switch plan.CropDepth {
	case "reduced":
		return 2
	case "full":
		return len(plan.Positions)
	default:
		return 3
	}
}

func (plan SamplingPlan) windows(windowSeconds float64) []SamplingWindow {
	if len(plan.Positions) == 0 {
		return nil
	}
	if windowSeconds <= 0 {
		windowSeconds = plan.WindowSeconds
	}
	windows := make([]SamplingWindow, 0, len(plan.Positions))
	seen := map[int]bool{}
	for _, position := range plan.Positions {
		start := 0.0
		actualDuration := windowSeconds
		if plan.AssetDurationSeconds > windowSeconds*2 {
			start = math.Max(0, math.Min(plan.AssetDurationSeconds-windowSeconds, plan.AssetDurationSeconds*position-windowSeconds/2))
		} else if plan.AssetDurationSeconds > 0 {
			actualDuration = math.Min(windowSeconds, plan.AssetDurationSeconds)
		}
		key := int(math.Round(start * 1000))
		if seen[key] {
			continue
		}
		seen[key] = true
		windows = append(windows, SamplingWindow{Position: position, StartSeconds: start, DurationSeconds: actualDuration})
	}
	return windows
}

func representativeSamplingWindows(plan SamplingPlan, windowSeconds float64, maximum int) []SamplingWindow {
	windows := plan.windows(windowSeconds)
	if maximum <= 0 || len(windows) <= maximum {
		return windows
	}
	if maximum == 1 {
		return []SamplingWindow{windows[len(windows)/2]}
	}
	selected := make([]SamplingWindow, 0, maximum)
	for index := 0; index < maximum; index++ {
		windowIndex := int(math.Round(float64(index) * float64(len(windows)-1) / float64(maximum-1)))
		selected = append(selected, windows[windowIndex])
	}
	return selected
}
