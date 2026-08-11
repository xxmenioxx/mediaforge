package handlers

import (
	"math"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/capabilities"
	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/quality"
)

func qualityIntentForMedia(profile models.Profile, sourcePath string, streams MediaStreamInventory) quality.QualityIntent {
	video := MediaStream{}
	if len(streams.Video) > 0 {
		video = streams.Video[0]
	}
	audioBitrate := int64(0)
	for _, stream := range streams.Audio {
		audioBitrate += stream.Bitrate
	}
	subtitleBitrate := int64(0)
	for _, stream := range streams.Subtitle {
		subtitleBitrate += stream.Bitrate
	}
	subtitleSize := int64(0)
	if streams.Duration > 0 && subtitleBitrate > 0 {
		subtitleSize = int64(float64(subtitleBitrate) * streams.Duration / 8)
	}
	grainScore, grainKnown := qualityScore(profile.WorkerConfig, "grainScore")
	motionScore, motionKnown := qualityScore(profile.WorkerConfig, "motionScore")
	complexityScore, complexityKnown := qualityScore(profile.WorkerConfig, "complexityScore")
	bFramePolicy, bFrameCount := frameStructureBFrameIntent(profile)
	return quality.NewIntent(quality.IntentInput{
		Preset:     workerStringValue(profile.WorkerConfig["hardwareQualityPreset"]),
		SourcePath: sourcePath, SourceWidth: video.Width, SourceHeight: video.Height,
		SourceVideoBitrate: video.Bitrate, DurationSeconds: streams.Duration,
		HDR:        isHDRMediaStream(video),
		GrainScore: grainScore, GrainScoreKnown: grainKnown,
		MotionScore: motionScore, MotionScoreKnown: motionKnown,
		ComplexityScore: complexityScore, ComplexityScoreKnown: complexityKnown,
		ContentType:  workerStringValue(profile.WorkerConfig["contentType"]),
		AudioBitrate: audioBitrate, SubtitleSize: subtitleSize,
		VideoFilters:         workerStringValue(profile.WorkerConfig["videoFilters"]),
		ColorPolicy:          workerStringValue(profile.WorkerConfig["finalColorPolicy"]),
		RequestedRateControl: workerStringValue(profile.WorkerConfig["qsvRateControl"]),
		RequestedBitDepth:    profile.BitDepth,
		RequestedPixelFormat: workerStringValue(profile.WorkerConfig["pixFmt"]),
		VideoToolboxRealtime: profileWorkerBool(profile, "videoToolboxRealtime", false),
		BFramePolicy:         bFramePolicy,
		BFrameCount:          bFrameCount,
		AutoAdjustBitrate:    profileWorkerBool(profile, "videoToolboxAutoAdjustBitrate", false),
	})
}

func frameStructureBFrameIntent(profile models.Profile) (string, int) {
	switch normalizedFrameStructureBFrameMode(workerStringValue(profile.WorkerConfig["frameStructureBFrameMode"])) {
	case "recommended", "custom":
		return "enabled", min(16, max(1, workerIntValue(profile.WorkerConfig["frameStructureMaxBFrames"], 3)))
	case "off":
		return "disabled", 0
	case "auto":
		return "auto", 0
	default:
		return workerStringValue(profile.WorkerConfig["videoToolboxBFramePolicy"]), workerIntValue(profile.WorkerConfig["videoToolboxBFrames"], 3)
	}
}

func applyQSVQualityRecommendation(profile models.Profile, intent quality.QualityIntent, capability capabilities.EncoderCapability) models.Profile {
	if profile.WorkerConfig == nil || !strings.EqualFold(workerStringValue(profile.WorkerConfig["videoEncoder"]), "hevc_qsv") || strings.EqualFold(workerStringValue(profile.WorkerConfig["hardwareQualityPreset"]), "custom") {
		return profile
	}
	recommendation, err := (quality.QSVTranslator{}).Translate(intent, qsvQualityCapabilities(capability, intent))
	if err != nil {
		return profile
	}
	profile.WorkerConfig["qsvRequestedRateControl"] = recommendation.RequestedRateControl
	profile.WorkerConfig["qsvEffectiveRateControl"] = recommendation.EffectiveRateControl
	profile.WorkerConfig["qsvRateControlFallbackReason"] = recommendation.RateControlFallback
	profile.WorkerConfig["qsvRequestedGlobalQuality"] = pointerIntValue(recommendation.RequestedGlobalQuality)
	profile.WorkerConfig["qsvEffectiveGlobalQuality"] = pointerIntValue(recommendation.GlobalQuality)
	profile.WorkerConfig["qsvAssetQualityAdjustment"] = recommendation.QualityAdjustment
	profile.WorkerConfig["qsvAssetQualityReasons"] = recommendation.QualityReasons
	profile.WorkerConfig["globalQuality"] = pointerIntValue(recommendation.GlobalQuality)
	profile.WorkerConfig["qsvEstimateConfidence"] = recommendation.EstimateConfidence
	profile.WorkerConfig["qsvRecommendationWarnings"] = recommendation.Warnings
	profile.WorkerConfig["qsvLookAheadDepth"] = recommendation.LookAheadDepth
	profile.WorkerConfig["qsvExtendedBRC"] = recommendation.ExtendedBRC
	profile.WorkerConfig["qsvAdaptiveI"] = recommendation.AdaptiveI
	profile.WorkerConfig["qsvAdaptiveB"] = recommendation.AdaptiveB
	profile.WorkerConfig["pixFmt"] = recommendation.PixelFormat
	profile.PixelFormat = recommendation.PixelFormat
	if recommendation.Profile == "main10" {
		profile.BitDepth = 10
	} else {
		profile.BitDepth = 8
	}
	storeInt64Recommendation(profile.WorkerConfig, "qsvTargetBitrate", recommendation.TargetBitrate)
	storeInt64Recommendation(profile.WorkerConfig, "qsvMaxrate", recommendation.Maxrate)
	storeInt64Recommendation(profile.WorkerConfig, "qsvBuffer", recommendation.Buffer)
	storeInt64Recommendation(profile.WorkerConfig, "qsvEstimatedVideoBitrateMin", recommendation.EstimatedVideoBitrateMin)
	storeInt64Recommendation(profile.WorkerConfig, "qsvEstimatedVideoBitrateMax", recommendation.EstimatedVideoBitrateMax)
	return profile
}

func pointerIntValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func storeInt64Recommendation(config models.JSONMap, key string, value *int64) {
	if value != nil {
		config[key] = *value
	}
}

func applyVideoToolboxQualityRecommendation(profile models.Profile, intent quality.QualityIntent, supplied ...capabilities.EncoderCapability) models.Profile {
	if profile.WorkerConfig == nil || !strings.EqualFold(workerStringValue(profile.WorkerConfig["videoEncoder"]), "hevc_videotoolbox") {
		return profile
	}
	capability := capabilities.CheckEncoder("hevc_videotoolbox")
	if len(supplied) > 0 {
		capability = supplied[0]
	}
	recommendation, err := (quality.VideoToolboxTranslator{}).Translate(intent, videoToolboxQualityCapabilities(capability, profileUsesTenBit(profile)))
	if err != nil {
		return profile
	}
	profile.WorkerConfig["videoToolboxEffectiveProfile"] = recommendation.Profile
	profile.WorkerConfig["videoToolboxEffectivePixelFormat"] = recommendation.PixelFormat
	profile.WorkerConfig["videoToolboxEffectiveRateControl"] = recommendation.EffectiveRateControl
	profile.WorkerConfig["videoToolboxEstimateConfidence"] = recommendation.EstimateConfidence
	profile.WorkerConfig["videoToolboxRecommendationWarnings"] = recommendation.Warnings
	profile.WorkerConfig["videoToolboxRequestedRealtime"] = recommendation.RequestedRealtime
	profile.WorkerConfig["videoToolboxEffectiveRealtime"] = recommendation.EffectiveRealtime
	profile.WorkerConfig["videoToolboxRequestedBFramePolicy"] = recommendation.RequestedBFramePolicy
	profile.WorkerConfig["videoToolboxEffectiveBFramePolicy"] = recommendation.EffectiveBFramePolicy
	profile.WorkerConfig["videoToolboxRequestedBFrames"] = recommendation.RequestedBFrames
	profile.WorkerConfig["videoToolboxObservedBFrameCount"] = recommendation.ObservedBFrameCount
	profile.WorkerConfig["videoToolboxBFrameEfficiencyMultiplier"] = recommendation.BFrameEfficiencyMultiplier
	profile.WorkerConfig["videoToolboxBFrameDowngradeReason"] = recommendation.BFrameDowngradeReason
	if recommendation.BaseTargetBitrate != nil {
		profile.WorkerConfig["videoToolboxBaseTargetKbps"] = *recommendation.BaseTargetBitrate / 1000
	}
	preset := strings.TrimSpace(workerStringValue(profile.WorkerConfig["hardwareQualityPreset"]))
	if preset == "" || strings.EqualFold(preset, "custom") {
		baseMbps := workerNumberValue(profile.WorkerConfig["videoToolboxBitrateMbps"], defaultVideoToolboxBitrateMbps(profile.QualityValue))
		targetMbps, maxrateMbps, bufferMbps := explicitVideoToolboxRates(profile, capability)
		profile.WorkerConfig["videoToolboxBaseTargetKbps"] = int64(baseMbps * 1000)
		profile.WorkerConfig["videoToolboxRecommendedTargetKbps"] = int64(targetMbps * 1000)
		profile.WorkerConfig["videoToolboxRecommendedMaxrateKbps"] = int64(maxrateMbps * 1000)
		profile.WorkerConfig["videoToolboxRecommendedBufferKbps"] = int64(bufferMbps * 1000)
	}
	if recommendation.TargetBitrate != nil {
		profile.WorkerConfig["videoToolboxRecommendedTargetKbps"] = *recommendation.TargetBitrate / 1000
		profile.WorkerConfig["videoToolboxBitrateMbps"] = fractionalMbps(*recommendation.TargetBitrate)
	}
	if recommendation.Maxrate != nil {
		profile.WorkerConfig["videoToolboxRecommendedMaxrateKbps"] = *recommendation.Maxrate / 1000
		profile.WorkerConfig["videoToolboxMaxrateMbps"] = fractionalMbps(*recommendation.Maxrate)
	}
	if recommendation.Buffer != nil {
		profile.WorkerConfig["videoToolboxRecommendedBufferKbps"] = *recommendation.Buffer / 1000
		profile.WorkerConfig["videoToolboxBufferMbps"] = fractionalMbps(*recommendation.Buffer)
	}
	if recommendation.EstimatedOutputSize != nil {
		profile.WorkerConfig["videoToolboxEstimatedVideoBytes"] = *recommendation.EstimatedOutputSize
	}
	profile.WorkerConfig["videoToolboxProfile"] = recommendation.Profile
	profile.WorkerConfig["pixFmt"] = recommendation.PixelFormat
	profile.PixelFormat = recommendation.PixelFormat
	if recommendation.Profile == "main10" {
		profile.BitDepth = 10
	} else {
		profile.BitDepth = 8
	}
	return profile
}

func videoToolboxQualityCapabilities(capability capabilities.EncoderCapability, main10 bool) quality.WorkerCapabilities {
	effective := capability.VideoToolboxBFrames
	disabled := capability.VideoToolboxBFramesDisabled
	autoEffective := capability.TestedModes["videoToolboxAutoBFramesMain"]
	if main10 {
		effective = capability.TestedModes["videoToolboxBFramesMain10"]
		disabled = capability.TestedModes["videoToolboxBFramesDisabledMain10"]
		autoEffective = capability.TestedModes["videoToolboxAutoBFramesMain10"]
	}
	return quality.WorkerCapabilities{
		Encoder: "hevc_videotoolbox", Main: capability.VideoToolboxMain, Main10: capability.VideoToolboxMain10,
		BFramesVerified:         capability.VideoToolboxBFramesVerified,
		BFramesEffective:        effective,
		BFramesDisabledVerified: disabled,
		ObservedBFrameCount:     capability.VideoToolboxObservedBFrames,
		AutoBFramesEffective:    autoEffective,
	}
}

func fractionalMbps(bitsPerSecond int64) float64 {
	return math.Round(float64(bitsPerSecond)/10_000) / 100
}

func qualityScore(config models.JSONMap, key string) (float64, bool) {
	value, exists := config[key]
	if !exists {
		return 0, false
	}
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	default:
		return 0, false
	}
}

func isHDRMediaStream(stream MediaStream) bool {
	transfer := strings.ToLower(strings.TrimSpace(stream.ColorTransfer))
	return transfer == "smpte2084" || transfer == "arib-std-b67"
}
