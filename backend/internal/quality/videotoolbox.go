package quality

import (
	"errors"
	"math"
)

var ErrCustomQuality = errors.New("custom quality requires explicit encoder settings")

type VideoToolboxTranslator struct{}

func (VideoToolboxTranslator) Translate(intent QualityIntent, _ WorkerCapabilities) (EncoderRecommendation, error) {
	if intent.Preset == PresetCustom || intent.Preset == "" {
		return EncoderRecommendation{}, ErrCustomQuality
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
	}
	if intent.SourceVideoBitrate <= 0 {
		recommendation.Warnings = append(recommendation.Warnings, "Source video bitrate is unavailable; explicit VideoToolbox bitrate controls are required")
		return recommendation, nil
	}
	target := int64(math.Ceil(float64(intent.SourceVideoBitrate)*settings.multiplier*videoToolboxResolutionAdjustment(intent.OutputHeight)/1000)) * 1000
	if floor := videoToolboxBitrateFloor(intent.Preset, intent.OutputHeight); target < floor {
		target = floor
	}
	if ceiling := videoToolboxBitrateCeiling(intent.Preset, intent.OutputHeight); ceiling > 0 && target > ceiling {
		target = ceiling
	}
	maxrate := int64(math.Ceil(float64(target) * 1.5))
	buffer := int64(math.Ceil(float64(target) * 2.5))
	recommendation.TargetBitrate = &target
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

type videoToolboxSettings struct {
	multiplier  float64
	profile     string
	pixelFormat string
}

func videoToolboxPresetSettings(preset Preset) (videoToolboxSettings, bool) {
	value, ok := map[Preset]videoToolboxSettings{
		PresetCompact: {0.25, "main", "yuv420p"}, PresetMedium: {0.33, "main", "yuv420p"},
		PresetRecommended: {0.40, "main", "yuv420p"}, PresetBest: {0.52, "main", "yuv420p"},
		PresetHighQuality: {0.65, "main10", "p010le"}, PresetArchive: {0.80, "main10", "p010le"},
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
		PresetCompact: {900_000, 1_400_000, 2_000_000, 4_000_000}, PresetMedium: {1_200_000, 1_800_000, 2_500_000, 5_000_000},
		PresetRecommended: {1_500_000, 2_200_000, 3_000_000, 6_000_000}, PresetBest: {2_000_000, 3_000_000, 4_000_000, 8_000_000},
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
		PresetCompact: 1_700_000, PresetMedium: 2_200_000, PresetRecommended: 2_500_000,
		PresetBest: 3_200_000, PresetHighQuality: 4_000_000, PresetArchive: 5_000_000, PresetMaster: 6_000_000,
	}[preset]
}
