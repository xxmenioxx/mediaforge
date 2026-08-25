package handlers

import (
	"encoding/json"
	"math"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

const (
	cadenceAnalysisVersion           = 1
	cadenceAnalysisSnapshotKey       = "cadenceAnalysisSnapshot"
	cadenceRecommendationSnapshotKey = "cadenceRecommendationSnapshot"
)

type CadenceAnalysis struct {
	Version               int       `json:"version"`
	Type                  string    `json:"type"`
	Pattern               string    `json:"pattern,omitempty"`
	DeclaredFrameRate     string    `json:"declaredFrameRate,omitempty"`
	DeclaredFPS           float64   `json:"declaredFps,omitempty"`
	EffectivePictureRate  string    `json:"effectivePictureRate,omitempty"`
	EffectiveFPS          float64   `json:"effectiveFps,omitempty"`
	FrameRateMismatch     bool      `json:"frameRateMismatch"`
	FrameRateRatio        float64   `json:"frameRateRatio,omitempty"`
	RepeatPictDetected    bool      `json:"repeatPictDetected"`
	Confidence            float64   `json:"confidence"`
	SampleCount           int       `json:"sampleCount"`
	RegionCount           int       `json:"regionCount"`
	AmbiguousSampleCount  int       `json:"ambiguousSampleCount"`
	ContradictorySamples  int       `json:"contradictorySampleCount"`
	SoftTelecineSamples   int       `json:"softTelecineSampleCount"`
	ConsistentSampleCount int       `json:"consistentSampleCount"`
	SampleEffectiveFPS    []float64 `json:"sampleEffectiveFps,omitempty"`
	DecisionReason        string    `json:"decisionReason"`
}

type CadenceRecommendation struct {
	Version         int     `json:"version"`
	Operation       string  `json:"operation"`
	OutputFrameRate string  `json:"outputFrameRate,omitempty"`
	Confidence      float64 `json:"confidence"`
	Reason          string  `json:"reason"`
}

func analyzeCadence(codec, declared string, interlace InterlaceAnalysis, frameStructure ...QSVFrameStructureAnalysis) CadenceAnalysis {
	result := CadenceAnalysis{Version: cadenceAnalysisVersion, Type: "unknown", DeclaredFrameRate: declared, DeclaredFPS: parseFrameRateValue(declared)}
	if result.DeclaredFPS <= 0 {
		result.DecisionReason = "declared frame rate is unavailable"
		return result
	}
	var totalIntervals int
	var totalSpan float64
	minimumSampleFPS, maximumSampleFPS := 0.0, 0.0
	repeatedSamples := 0
	progressiveSamples := 0
	type cadenceSample struct {
		signals     FrameSignalSummary
		progressive bool
	}
	samples := []cadenceSample{}
	if len(frameStructure) > 0 && len(frameStructure[0].Windows) > 0 {
		for _, window := range frameStructure[0].Windows {
			signals := window.FrameSignals
			samples = append(samples, cadenceSample{signals: signals, progressive: signals.DecodedFrames > 0 && signals.ProgressiveFrames*10 >= signals.DecodedFrames*9})
		}
	} else {
		for _, window := range interlace.Windows {
			samples = append(samples, cadenceSample{signals: window.FrameSignals, progressive: window.Status == "progressive"})
		}
	}
	result.RegionCount = len(samples)
	for _, sample := range samples {
		signals := sample.signals
		if signals.DecodedFrames == 0 || signals.ActualTimespan <= 0 || signals.EffectiveFPS <= 0 {
			result.AmbiguousSampleCount++
			continue
		}
		result.SampleCount++
		result.SampleEffectiveFPS = append(result.SampleEffectiveFPS, signals.EffectiveFPS)
		if minimumSampleFPS == 0 || signals.EffectiveFPS < minimumSampleFPS {
			minimumSampleFPS = signals.EffectiveFPS
		}
		if signals.EffectiveFPS > maximumSampleFPS {
			maximumSampleFPS = signals.EffectiveFPS
		}
		totalIntervals += max(0, signals.DecodedFrames-1)
		totalSpan += signals.ActualTimespan
		if signals.RepeatPictFrames > 0 && strings.HasPrefix(signals.Cadence, "stable_every_") {
			repeatedSamples++
		}
		if sample.progressive {
			progressiveSamples++
		}
		if nearFPS(signals.EffectiveFPS, 24000.0/1001.0, 0.20) {
			result.ConsistentSampleCount++
		}
	}
	if totalSpan > 0 {
		result.EffectiveFPS = float64(totalIntervals) / totalSpan
		result.EffectivePictureRate = canonicalFrameRate(result.EffectiveFPS)
		result.FrameRateRatio = result.DeclaredFPS / result.EffectiveFPS
		result.FrameRateMismatch = math.Abs(result.FrameRateRatio-1) > 0.02
	}
	result.RepeatPictDetected = repeatedSamples > 0
	allMeasuredConsistent := result.SampleCount >= 2 && result.ConsistentSampleCount == result.SampleCount
	allProgressive := result.SampleCount >= 2 && progressiveSamples == result.SampleCount && interlace.Status == "progressive"
	allRepeated := result.SampleCount >= 2 && repeatedSamples == result.SampleCount
	softCandidate := strings.EqualFold(codec, "mpeg2video") && nearFPS(result.DeclaredFPS, 30000.0/1001.0, 0.05)
	if softCandidate {
		result.SoftTelecineSamples = min(result.ConsistentSampleCount, min(progressiveSamples, repeatedSamples))
		result.ContradictorySamples = result.SampleCount - result.SoftTelecineSamples
	}
	if softCandidate && allMeasuredConsistent && allProgressive && allRepeated && nearFPS(result.FrameRateRatio, 1.25, 0.02) {
		result.Type = "soft_telecine"
		result.Pattern = "2:3"
		result.Confidence = 0.98
		if result.AmbiguousSampleCount > 0 {
			result.Confidence = 0.90
		}
		result.DecisionReason = "progressive MPEG-2 samples consistently decode near 23.976 fps while declared at 29.97 fps with repeat-picture cadence"
		return result
	}
	if result.SampleCount >= 2 && (maximumSampleFPS-minimumSampleFPS > 0.50 || result.ConsistentSampleCount > 0 && result.ConsistentSampleCount < result.SampleCount || progressiveSamples > 0 && progressiveSamples < result.SampleCount) {
		result.Type = "mixed"
		result.Confidence = 0.75
		result.DecisionReason = "sampled regions disagree on picture rate or progressive structure"
		return result
	}
	if interlace.Status == "telecine" || interlace.Status == "telecine_suspected" {
		result.Type = "hard_telecine"
		result.Confidence = interlace.Confidence
		result.DecisionReason = "field-based telecine evidence requires validated inverse telecine rather than frame-rate normalization"
		return result
	}
	if interlace.Status == "interlaced" {
		result.Type = "interlaced"
		result.Confidence = interlace.Confidence
		result.DecisionReason = "sampled pictures are interlaced"
		return result
	}
	if interlace.Status == "progressive" && result.EffectiveFPS > 0 && nearFPS(result.DeclaredFPS, result.EffectiveFPS, 0.10) {
		result.Type = "native_progressive"
		result.Confidence = interlace.Confidence
		result.DecisionReason = "declared and sampled progressive picture rates agree"
		return result
	}
	result.DecisionReason = "cadence evidence is incomplete or does not meet the conservative automatic threshold"
	return result
}

func recommendCadence(analysis CadenceAnalysis) CadenceRecommendation {
	result := CadenceRecommendation{Version: 1, Operation: "review", Confidence: analysis.Confidence, Reason: analysis.DecisionReason}
	switch {
	case analysis.Type == "soft_telecine" && analysis.Confidence >= 0.95:
		result.Operation = "remove_soft_telecine"
		result.OutputFrameRate = "24000/1001"
	case analysis.Type == "hard_telecine" && analysis.Confidence >= 0.85:
		result.Operation = "inverse_telecine"
		result.OutputFrameRate = "24000/1001"
	case analysis.Type == "native_progressive" || analysis.Type == "interlaced":
		result.Operation = "preserve"
	}
	return result
}

func cadenceRecommendationMap(value CadenceRecommendation) models.JSONMap {
	encoded, _ := json.Marshal(value)
	result := models.JSONMap{}
	_ = json.Unmarshal(encoded, &result)
	return result
}

func ensureCadenceRecommendation(scan *models.ScanResult) bool {
	if scan == nil || len(scan.CadenceRecommendation) > 0 {
		return false
	}
	analysis, ok := decodeCadenceAnalysis(scan.CadenceAnalysis)
	if !ok {
		return false
	}
	scan.CadenceRecommendation = cadenceRecommendationMap(recommendCadence(analysis))
	return true
}

func decodeCadenceRecommendation(value any) (CadenceRecommendation, bool) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return CadenceRecommendation{}, false
	}
	var result CadenceRecommendation
	if json.Unmarshal(encoded, &result) != nil || result.Version < 1 || strings.TrimSpace(result.Operation) == "" {
		return CadenceRecommendation{}, false
	}
	return result, true
}

func cadenceAnalysisMap(value CadenceAnalysis) models.JSONMap {
	encoded, _ := json.Marshal(value)
	result := models.JSONMap{}
	_ = json.Unmarshal(encoded, &result)
	return result
}

func decodeCadenceAnalysis(value any) (CadenceAnalysis, bool) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return CadenceAnalysis{}, false
	}
	var result CadenceAnalysis
	if json.Unmarshal(encoded, &result) != nil || result.Version < cadenceAnalysisVersion || strings.TrimSpace(result.Type) == "" {
		return CadenceAnalysis{}, false
	}
	return result, true
}

func nearFPS(actual, expected, tolerance float64) bool {
	return actual > 0 && math.Abs(actual-expected) <= tolerance
}

func canonicalFrameRate(fps float64) string {
	if nearFPS(fps, 24000.0/1001.0, 0.20) {
		return "24000/1001"
	}
	if nearFPS(fps, 30000.0/1001.0, 0.20) {
		return "30000/1001"
	}
	return ""
}

func firstReliableFrameRate(values ...string) string {
	for _, value := range values {
		if parseFrameRateValue(value) > 0 {
			return value
		}
	}
	return ""
}

func profileWithAutomaticCadence(profile models.Profile, analysis CadenceAnalysis, recommendations ...CadenceRecommendation) models.Profile {
	recommendation := recommendCadence(analysis)
	if len(recommendations) > 0 && recommendations[0].Operation != "" {
		recommendation = recommendations[0]
	}
	existing := strings.TrimSpace(workerStringValue(profile.WorkerConfig["videoFilters"]))
	if explicit, found, known := explicitFPSFilterRate(existing); found {
		profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
		delete(profile.WorkerConfig, "effectiveOutputFrameRate")
		profile.WorkerConfig["effectiveCadenceOperation"] = "explicit_fps_filter"
		if known {
			profile.WorkerConfig["effectiveOutputFrameRate"] = explicit
			delete(profile.WorkerConfig, "effectiveOutputFrameRateUnknown")
			delete(profile.WorkerConfig, "cadenceResolutionWarning")
		} else {
			profile.WorkerConfig["effectiveOutputFrameRateUnknown"] = true
			profile.WorkerConfig["cadenceResolutionWarning"] = "explicit fps filter won over automatic cadence, but its effective output rate could not be parsed safely"
		}
		return profile
	}
	mode := normalizedCadenceMode(workerStringValue(profile.WorkerConfig["cadenceMode"]))
	if mode == "" {
		mode = legacyCadenceMode(profile)
	}
	if mode == "preserve" {
		return profile
	}
	if normalizedFieldStructureMode(workerStringValue(profile.WorkerConfig["fieldStructureMode"])) == "deinterlace" {
		profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
		profile.WorkerConfig["cadenceResolutionWarning"] = "cadence normalization was skipped because force deinterlace is an incompatible explicit operation"
		return profile
	}
	if mode == "remove_soft_telecine" {
		recommendation = CadenceRecommendation{Version: 1, Operation: "remove_soft_telecine", OutputFrameRate: "24000/1001", Confidence: 1, Reason: "explicit user selection"}
	}
	if recommendation.Operation != "remove_soft_telecine" || recommendation.Confidence < 0.95 || (mode != "auto" && mode != "remove_soft_telecine") {
		return profile
	}
	profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
	target := recommendation.OutputFrameRate
	if target == "" {
		target = "24000/1001"
	}
	if existing != "" {
		existing = "," + existing
	}
	profile.WorkerConfig["videoFilters"] = "fps=" + target + existing
	profile.WorkerConfig["effectiveOutputFrameRate"] = target
	delete(profile.WorkerConfig, "effectiveOutputFrameRateUnknown")
	profile.WorkerConfig["effectiveCadenceOperation"] = recommendation.Operation
	return profile
}

func explicitFPSFilterRate(filters string) (string, bool, bool) {
	for _, filter := range strings.Split(filters, ",") {
		parts := strings.SplitN(strings.TrimSpace(filter), "=", 2)
		if len(parts) != 2 || !strings.EqualFold(strings.TrimSpace(parts[0]), "fps") {
			continue
		}
		value := strings.TrimSpace(parts[1])
		if parseFrameRateValue(value) <= 0 {
			return "", true, false
		}
		if !strings.Contains(value, "/") && !strings.Contains(value, ".") {
			value += "/1"
		}
		return value, true, true
	}
	return "", false, false
}

func simpleFPSFilterRate(filters string) (string, bool) {
	value, found, known := explicitFPSFilterRate(filters)
	return value, found && known
}

func profileWithCadenceOutputDecision(profile models.Profile, analysis CadenceAnalysis, recommendation CadenceRecommendation) models.Profile {
	profile = profileWithAutomaticCadence(profile, analysis, recommendation)
	mode := normalizedCadenceMode(workerStringValue(profile.WorkerConfig["cadenceMode"]))
	if mode == "inverse_telecine" || mode == "auto" && recommendation.Operation == "inverse_telecine" {
		profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
		target := recommendation.OutputFrameRate
		if target == "" {
			target = "24000/1001"
		}
		profile.WorkerConfig["effectiveCadenceOperation"] = "inverse_telecine"
		profile.WorkerConfig["effectiveOutputFrameRate"] = target
		delete(profile.WorkerConfig, "effectiveOutputFrameRateUnknown")
	}
	return profile
}

func normalizedCadenceMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "preserve", "auto", "remove_soft_telecine", "inverse_telecine":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizedFieldStructureMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "preserve", "auto", "deinterlace":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizedCadenceFieldOrder(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "auto", "tff", "bff":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func legacyCadenceMode(profile models.Profile) string {
	switch strings.ToLower(strings.TrimSpace(workerStringValue(profile.WorkerConfig["deinterlaceMode"]))) {
	case "ivtc_tff", "ivtc_bff":
		return "inverse_telecine"
	case "off", "force":
		return "preserve"
	default:
		return "auto"
	}
}

func profileWithResolvedFieldAndCadenceModes(profile models.Profile, analysis InterlaceAnalysis) models.Profile {
	profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
	fieldMode := normalizedFieldStructureMode(workerStringValue(profile.WorkerConfig["fieldStructureMode"]))
	legacy := strings.ToLower(strings.TrimSpace(workerStringValue(profile.WorkerConfig["deinterlaceMode"])))
	if fieldMode == "" {
		switch legacy {
		case "off", "ivtc_tff", "ivtc_bff":
			fieldMode = "preserve"
		case "force":
			fieldMode = "deinterlace"
		default:
			fieldMode = "auto"
		}
	}
	cadenceMode := normalizedCadenceMode(workerStringValue(profile.WorkerConfig["cadenceMode"]))
	if cadenceMode == "" {
		cadenceMode = legacyCadenceMode(profile)
	}
	profile.WorkerConfig["fieldStructureMode"] = fieldMode
	profile.WorkerConfig["cadenceMode"] = cadenceMode
	switch fieldMode {
	case "preserve":
		profile.WorkerConfig["deinterlaceMode"] = "off"
	case "deinterlace":
		profile.WorkerConfig["deinterlaceMode"] = "force"
	default:
		profile.WorkerConfig["deinterlaceMode"] = "auto"
	}
	if cadenceMode == "inverse_telecine" {
		order := strings.ToLower(strings.TrimSpace(workerStringValue(profile.WorkerConfig["cadenceFieldOrder"])))
		if order == "" && legacy == "ivtc_bff" {
			order = "bff"
		}
		if order == "" && legacy == "ivtc_tff" {
			order = "tff"
		}
		if order != "tff" && order != "bff" {
			order = analysis.DetectedFieldOrder
		}
		if order != "bff" {
			order = "tff"
		}
		profile.WorkerConfig["deinterlaceMode"] = "ivtc_" + order
	}
	return profile
}

func cadenceOutputArgs(profile models.Profile) []string {
	if strings.TrimSpace(workerStringValue(profile.WorkerConfig["effectiveOutputFrameRate"])) == "" {
		return nil
	}
	return []string{"-fps_mode", "cfr"}
}
