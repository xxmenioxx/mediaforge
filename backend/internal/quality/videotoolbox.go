package quality

import (
	"errors"
	"math"
)

var ErrCustomQuality = errors.New("custom quality requires explicit encoder settings")

type VideoToolboxTranslator struct{}

const (
	videoToolboxBFramesEffectiveMultiplier = 1.00
	videoToolboxBFramesAutoMultiplier      = 1.03
	videoToolboxBFramesDisabledMultiplier  = 1.08
)

func (VideoToolboxTranslator) Translate(intent QualityIntent, capabilities WorkerCapabilities) (EncoderRecommendation, error) {
	if intent.Preset == "" {
		// Profiles created before hardwareQualityPreset existed already carry
		// explicit VideoToolbox bitrate/profile controls. Treat an omitted preset
		// as Custom so the evaluator can describe those effective settings instead
		// of rejecting an otherwise executable profile.
		intent.Preset = PresetCustom
	}
	if intent.Preset == PresetCustom {
		profile, pixelFormat := "main", "yuv420p"
		if intent.RequestedBitDepth >= 10 || intent.RequestedPixelFormat == "p010le" {
			profile, pixelFormat = "main10", "p010le"
		}
		recommendation := EncoderRecommendation{
			Encoder: "hevc_videotoolbox", RequestedRateControl: "vbr", EffectiveRateControl: "vbr",
			Profile: profile, PixelFormat: pixelFormat, ColorPolicy: intent.ColorPolicy,
			RequestedRealtime: intent.VideoToolboxRealtime, EffectiveRealtime: intent.VideoToolboxRealtime,
			PowerEfficiency: true, EstimateConfidence: "low",
		}
		resolveVideoToolboxBFrames(intent, capabilities, &recommendation)
		recommendation.Warnings = append(recommendation.Warnings, "Custom VideoToolbox bitrate controls remain manual")
		return recommendation, nil
	}
	settings, known := videoToolboxPresetSettings(intent.Preset)
	if !known {
		return EncoderRecommendation{}, errors.New("unknown VideoToolbox quality preset")
	}
	recommendation := EncoderRecommendation{
		Encoder: "hevc_videotoolbox", RequestedRateControl: "vbr", EffectiveRateControl: "vbr",
		Profile: settings.profile, PixelFormat: settings.pixelFormat,
		ColorPolicy: intent.ColorPolicy, Realtime: false,
		AllowFrameReordering: false, PowerEfficiency: true,
		EstimateConfidence: "low",
		RequestedRealtime:  intent.VideoToolboxRealtime, EffectiveRealtime: intent.VideoToolboxRealtime,
	}
	resolveVideoToolboxBFrames(intent, capabilities, &recommendation)
	if intent.SourceVideoBitrate <= 0 {
		recommendation.Warnings = append(recommendation.Warnings, "Source video bitrate is unavailable; explicit VideoToolbox bitrate controls are required")
		return recommendation, nil
	}
	baseTarget := roundBitrateToTwoDecimalMbps(int64(math.Round(float64(intent.SourceVideoBitrate) * settings.multiplier * videoToolboxResolutionAdjustment(intent.OutputHeight))))
	if floor := videoToolboxBitrateFloor(intent.Preset, intent.OutputHeight); baseTarget < floor {
		baseTarget = floor
	}
	if ceiling := videoToolboxBitrateCeiling(intent.Preset, intent.OutputHeight); ceiling > 0 && baseTarget > ceiling {
		baseTarget = ceiling
	}
	target := roundBitrateToTwoDecimalMbps(int64(math.Round(float64(baseTarget) * recommendation.BFrameEfficiencyMultiplier)))
	maxrate := roundBitrateToTwoDecimalMbps(int64(math.Round(float64(target) * 1.5)))
	buffer := roundBitrateToTwoDecimalMbps(int64(math.Round(float64(target) * 2.5)))
	recommendation.TargetBitrate = &target
	recommendation.BaseTargetBitrate = &baseTarget
	recommendation.Maxrate = &maxrate
	recommendation.Buffer = &buffer
	recommendation.EstimatedVideoBitrate = &target
	recommendation.EstimateConfidence = "medium"
	if intent.Duration > 0 {
		size := int64(float64(target) * intent.Duration.Seconds() / 8)
		recommendation.EstimatedOutputSize = &size
	}
	return recommendation, nil
}

func resolveVideoToolboxBFrames(intent QualityIntent, capabilities WorkerCapabilities, recommendation *EncoderRecommendation) {
	requested := intent.BFramePolicy
	if requested == "" {
		requested = BFrameAuto
	}
	count := intent.BFrameCount
	if count < 1 || count > 4 {
		count = 3
	}
	recommendation.RequestedBFramePolicy = string(requested)
	recommendation.RequestedBFrames = count
	recommendation.ObservedBFrameCount = capabilities.ObservedBFrameCount
	switch requested {
	case BFrameEnabled:
		if capabilities.BFramesVerified && capabilities.BFramesEffective {
			recommendation.EffectiveBFramePolicy = string(BFrameEnabled)
			recommendation.BFrameEfficiencyMultiplier = videoToolboxBFramesEffectiveMultiplier
			return
		}
		recommendation.EffectiveBFramePolicy = string(BFrameAuto)
		recommendation.BFrameEfficiencyMultiplier = videoToolboxBFramesAutoMultiplier
		recommendation.BFrameDowngradeReason = "VideoToolbox B-frames were requested but the worker did not verify effective B-frame output"
		recommendation.Warnings = append(recommendation.Warnings, recommendation.BFrameDowngradeReason)
	case BFrameDisabled:
		if !capabilities.BFramesDisabledVerified {
			recommendation.EffectiveBFramePolicy = string(BFrameAuto)
			recommendation.BFrameEfficiencyMultiplier = videoToolboxBFramesAutoMultiplier
			recommendation.BFrameDowngradeReason = "VideoToolbox did not verify that -bf 0 disables B-frames"
			recommendation.Warnings = append(recommendation.Warnings, recommendation.BFrameDowngradeReason)
			return
		}
		recommendation.EffectiveBFramePolicy = string(BFrameDisabled)
		recommendation.BFrameEfficiencyMultiplier = videoToolboxBFramesDisabledMultiplier
	default:
		if intent.VideoToolboxRealtime {
			if capabilities.BFramesDisabledVerified {
				recommendation.EffectiveBFramePolicy = string(BFrameDisabled)
				recommendation.BFrameEfficiencyMultiplier = videoToolboxBFramesDisabledMultiplier
			} else {
				recommendation.EffectiveBFramePolicy = string(BFrameAuto)
				recommendation.BFrameEfficiencyMultiplier = videoToolboxBFramesAutoMultiplier
				recommendation.BFrameDowngradeReason = "Realtime requested Auto B-frames, but the worker did not verify that -bf 0 is respected"
				recommendation.Warnings = append(recommendation.Warnings, recommendation.BFrameDowngradeReason)
			}
		} else if capabilities.AutoBFramesEffective {
			recommendation.EffectiveBFramePolicy = string(BFrameAuto)
			recommendation.BFrameEfficiencyMultiplier = videoToolboxBFramesEffectiveMultiplier
		} else {
			recommendation.EffectiveBFramePolicy = string(BFrameAuto)
			recommendation.BFrameEfficiencyMultiplier = videoToolboxBFramesAutoMultiplier
		}
	}
}

func roundBitrateToTwoDecimalMbps(bitsPerSecond int64) int64 {
	return int64(math.Round(float64(bitsPerSecond)/10_000)) * 10_000
}

type videoToolboxSettings struct {
	multiplier  float64
	profile     string
	pixelFormat string
}

func videoToolboxPresetSettings(preset Preset) (videoToolboxSettings, bool) {
	value, ok := map[Preset]videoToolboxSettings{
		PresetCompact: {0.18, "main", "yuv420p"}, PresetMedium: {0.24, "main", "yuv420p"},
		PresetRecommended: {0.30, "main", "yuv420p"}, PresetBest: {0.38, "main", "yuv420p"},
		PresetHighQuality: {0.48, "main10", "p010le"}, PresetArchive: {0.65, "main10", "p010le"},
		PresetMaster: {0.95, "main10", "p010le"},
	}[preset]
	return value, ok
}

func videoToolboxResolutionAdjustment(height int) float64 {
	if height > 0 && height <= 576 {
		return 1.075
	}
	return 1
}

func videoToolboxBitrateFloor(preset Preset, height int) int64 {
	values := map[Preset][]int64{
		PresetCompact: {700_000, 1_100_000, 1_600_000, 3_200_000}, PresetMedium: {950_000, 1_450_000, 2_000_000, 4_000_000},
		PresetRecommended: {1_250_000, 1_800_000, 2_500_000, 5_000_000}, PresetBest: {1_600_000, 2_400_000, 3_200_000, 6_400_000},
		PresetHighQuality: {2_500_000, 4_000_000, 5_000_000, 10_000_000}, PresetArchive: {3_200_000, 5_000_000, 6_000_000, 12_000_000},
		PresetMaster: {4_000_000, 6_500_000, 7_000_000, 14_000_000},
	}
	index := 2
	if height > 0 && height <= 576 {
		index = 0
	} else if height > 0 && height <= 720 {
		index = 1
	} else if height > 1080 {
		index = 3
	}
	return values[preset][index]
}

func videoToolboxBitrateCeiling(preset Preset, height int) int64 {
	if height <= 0 || height > 576 {
		return 0
	}
	return map[Preset]int64{
		PresetCompact: 1_300_000, PresetMedium: 1_600_000, PresetRecommended: 2_000_000,
		PresetBest: 2_600_000, PresetHighQuality: 3_400_000, PresetArchive: 4_500_000, PresetMaster: 6_000_000,
	}[preset]
}
