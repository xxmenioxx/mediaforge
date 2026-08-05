package quality

import (
	"errors"
	"math"
	"strings"
)

type QSVTranslator struct{}

var qsvBaseQualities = map[Preset]int{
	PresetCompact: 32, PresetMedium: 30, PresetRecommended: 28, PresetBest: 25,
	PresetHighQuality: 22, PresetArchive: 19, PresetMaster: 16,
}

var qsvEstimateRatios = map[Preset][2]float64{
	PresetCompact: {.25, .40}, PresetMedium: {.32, .47}, PresetRecommended: {.40, .55},
	PresetBest: {.50, .65}, PresetHighQuality: {.60, .75}, PresetArchive: {.70, .85}, PresetMaster: {.80, 1.00},
}

func QSVBaseQuality(preset Preset) (int, bool) {
	value, ok := qsvBaseQualities[preset]
	return value, ok
}

func QSVFormat(preset Preset) (profile, pixelFormat string, ok bool) {
	if _, known := QSVBaseQuality(preset); !known {
		return "", "", false
	}
	if preset == PresetHighQuality || preset == PresetArchive || preset == PresetMaster {
		return "main10", "p010le", true
	}
	return "main", "nv12", true
}

func (QSVTranslator) Translate(intent QualityIntent, capability WorkerCapabilities) (EncoderRecommendation, error) {
	if intent.Preset == PresetCustom || intent.Preset == "" {
		return EncoderRecommendation{}, ErrCustomQuality
	}
	base, known := QSVBaseQuality(intent.Preset)
	if !known {
		return EncoderRecommendation{}, errors.New("unknown QSV quality preset")
	}
	profile, pixelFormat, _ := QSVFormat(intent.Preset)
	if intent.RequestedBitDepth >= 10 || intent.RequestedPixelFormat == "p010le" {
		profile, pixelFormat = "main10", "p010le"
	} else if intent.RequestedBitDepth == 8 || intent.RequestedPixelFormat == "nv12" {
		profile, pixelFormat = "main", "nv12"
	}

	adjustment, reasons := qsvComplexityAdjustment(intent)
	effectiveQuality := clampInteger(base+adjustment, 16, 32)
	adjustment = effectiveQuality - base
	requestedRateControl := normalizeQSVRateControl(intent.RequestedRateControl)
	effectiveRateControl, fallback := selectQSVRateControl(requestedRateControl, capability)
	recommendation := EncoderRecommendation{
		Encoder: "hevc_qsv", RequestedRateControl: requestedRateControl,
		EffectiveRateControl: effectiveRateControl, RateControlFallback: fallback,
		RequestedGlobalQuality: integerPointer(base), GlobalQuality: integerPointer(effectiveQuality),
		QualityAdjustment: adjustment, QualityReasons: reasons,
		Profile: profile, PixelFormat: pixelFormat, ColorPolicy: intent.ColorPolicy,
		EstimateConfidence: "low",
	}
	if effectiveRateControl == "" {
		recommendation.Warnings = append(recommendation.Warnings, "No tested QSV rate-control mode is available")
		return recommendation, nil
	}
	if profile == "main10" && !capability.Main10 {
		recommendation.Profile, recommendation.PixelFormat = "main", "nv12"
		recommendation.Warnings = append(recommendation.Warnings, "QSV Main10 is unavailable; effective output was downgraded to Main 8-bit")
	}
	if effectiveRateControl == "la_icq" {
		recommendation.LookAhead, recommendation.LookAheadDepth = true, 40
	}
	if recommendation.Profile == "main10" && capability.FullCombination {
		recommendation.ExtendedBRC = capability.ExtendedBRC
		recommendation.AdaptiveI = capability.AdaptiveI
		recommendation.AdaptiveB = capability.AdaptiveB
	}
	applyQSVEstimate(&recommendation, intent)
	if effectiveRateControl == "vbr" || effectiveRateControl == "cbr" {
		target := intent.SourceVideoBitrate
		if recommendation.EstimatedVideoBitrate != nil {
			target = *recommendation.EstimatedVideoBitrate
		}
		if target <= 0 {
			recommendation.Warnings = append(recommendation.Warnings, "QSV bitrate fallback requires a readable source bitrate")
		} else {
			recommendation.TargetBitrate = &target
			maxrate := target
			if effectiveRateControl == "vbr" {
				maxrate = int64(math.Ceil(float64(target) * 1.5))
			}
			buffer := target * 2
			recommendation.Maxrate, recommendation.Buffer = &maxrate, &buffer
		}
	}
	return recommendation, nil
}

func qsvComplexityAdjustment(intent QualityIntent) (int, []string) {
	adjustment := 0
	reasons := []string{}
	if intent.GrainScoreKnown {
		switch {
		case intent.GrainScore >= .7:
			adjustment -= 2
			reasons = append(reasons, "heavy grain")
		case intent.GrainScore >= .35:
			adjustment--
			reasons = append(reasons, "moderate grain")
		}
	}
	switch intent.ContentType {
	case ContentAnime:
		adjustment--
		reasons = append(reasons, "anime line art")
	case ContentConcert:
		adjustment--
		reasons = append(reasons, "concert motion and lighting")
	case ContentSports:
		adjustment--
		reasons = append(reasons, "sports motion")
	}
	if intent.ResolutionClass == ResolutionSD {
		adjustment--
		reasons = append(reasons, "SD or DVD-class source")
	}
	if intent.MotionScoreKnown {
		if intent.MotionScore >= .7 {
			adjustment--
			reasons = append(reasons, "high measured motion")
		} else if intent.MotionScore <= .2 {
			adjustment++
			reasons = append(reasons, "low measured motion")
		}
	}
	if intent.ComplexityScoreKnown && intent.ComplexityScore <= .2 {
		adjustment++
		reasons = append(reasons, "simple visual complexity")
	}
	return clampInteger(adjustment, -3, 2), reasons
}

func normalizeQSVRateControl(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "la_icq", "icq", "cqp", "vbr", "cbr":
		return value
	default:
		return "la_icq"
	}
}

func selectQSVRateControl(requested string, capability WorkerCapabilities) (string, string) {
	available := map[string]bool{"la_icq": capability.LAICQ, "icq": capability.ICQ, "cqp": capability.CQP, "vbr": capability.VBR, "cbr": capability.CBR}
	order := []string{"la_icq", "icq", "cqp", "vbr", "cbr"}
	start := 0
	for index, candidate := range order {
		if candidate == requested {
			start = index
			break
		}
	}
	for _, candidate := range order[start:] {
		if !available[candidate] {
			continue
		}
		if candidate == requested {
			return candidate, ""
		}
		return candidate, requested + " is unavailable; using " + candidate
	}
	return "", requested + " and all supported fallback modes are unavailable"
}

func applyQSVEstimate(recommendation *EncoderRecommendation, intent QualityIntent) {
	if intent.MeasuredVideoBitrate > 0 {
		setQSVEstimate(recommendation, intent, intent.MeasuredVideoBitrate, intent.MeasuredVideoBitrate, "high")
		return
	}
	if intent.HistoricalSamples >= 2 && intent.HistoricalRatioMin > 0 && intent.HistoricalRatioMax >= intent.HistoricalRatioMin && intent.SourceVideoBitrate > 0 {
		setQSVEstimate(recommendation, intent, int64(float64(intent.SourceVideoBitrate)*intent.HistoricalRatioMin), int64(float64(intent.SourceVideoBitrate)*intent.HistoricalRatioMax), "medium")
		return
	}
	ratios := qsvEstimateRatios[intent.Preset]
	if intent.SourceVideoBitrate > 0 && ratios[0] > 0 {
		setQSVEstimate(recommendation, intent, int64(float64(intent.SourceVideoBitrate)*ratios[0]), int64(float64(intent.SourceVideoBitrate)*ratios[1]), "low")
	}
}

func setQSVEstimate(recommendation *EncoderRecommendation, intent QualityIntent, minRate, maxRate int64, confidence string) {
	minRate = roundBitrateToTwoDecimalMbps(minRate)
	maxRate = roundBitrateToTwoDecimalMbps(maxRate)
	midpoint := (minRate + maxRate) / 2
	recommendation.EstimatedVideoBitrate = &midpoint
	recommendation.EstimatedVideoBitrateMin = &minRate
	recommendation.EstimatedVideoBitrateMax = &maxRate
	recommendation.EstimateConfidence = confidence
	if intent.Duration > 0 {
		minSize := int64(float64(minRate) * intent.Duration.Seconds() / 8)
		maxSize := int64(float64(maxRate) * intent.Duration.Seconds() / 8)
		midSize := (minSize + maxSize) / 2
		recommendation.EstimatedOutputSize = &midSize
		recommendation.EstimatedOutputSizeMin = &minSize
		recommendation.EstimatedOutputSizeMax = &maxSize
	}
}

func integerPointer(value int) *int { return &value }

func clampInteger(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}
