package handlers

import (
	"math"
	"net/http"
	"os"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/capabilities"
	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/quality"
	"github.com/anuelvs/mvforge/backend/internal/runtimeinfo"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
)

type qualityRecommendationInput struct {
	Path    string         `json:"path"`
	Profile models.Profile `json:"profile"`
}

type qualityRecommendationResponse struct {
	RequestedProfile     models.Profile                `json:"requestedProfile"`
	EffectiveProfile     models.Profile                `json:"effectiveProfile"`
	Recommendation       quality.EncoderRecommendation `json:"recommendation"`
	CapabilitySource     string                        `json:"capabilitySource"`
	FFmpegVideoArguments []string                      `json:"ffmpegVideoArguments"`
	EstimatedOutputMin   int64                         `json:"estimatedOutputMinBytes"`
	EstimatedOutputMax   int64                         `json:"estimatedOutputMaxBytes"`
	EstimatedSavingsMin  int64                         `json:"estimatedSavingsMinBytes"`
	EstimatedSavingsMax  int64                         `json:"estimatedSavingsMaxBytes"`
}

// QualityRecommendation returns the encoder-specific effective values for a
// quality intent. LAB, Profiles and Asset Overrides all consume this contract.
func (h AssetHandler) QualityRecommendation(c *gin.Context) {
	var input qualityRecommendationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "profile is required"})
		return
	}
	requested := input.Profile
	requested.WorkerConfig = cloneJSONMap(input.Profile.WorkerConfig)
	profile := input.Profile
	profile.WorkerConfig = cloneJSONMap(input.Profile.WorkerConfig)
	profile = normalizeHardwareQualityPreset(profile)
	streams := MediaStreamInventory{}
	resolvedPath := ""
	if path := strings.TrimSpace(input.Path); path != "" {
		var err error
		resolvedPath, err = h.resolveMediaPath(path)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "a readable media path is required"})
			return
		}
		allowed, allowedErr := h.pathBelongsToReadableMediaRoot(resolvedPath)
		if allowedErr != nil || !allowed {
			c.JSON(http.StatusForbidden, gin.H{"error": "media path is outside configured libraries"})
			return
		}
		streams, err = probeMediaStreams(resolvedPath)
		if err != nil || len(streams.Video) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "could not inspect the asset video stream"})
			return
		}
		if streams.Video[0].Bitrate <= 0 {
			streams.Video[0].Bitrate = estimatedVideoBitrate(streams)
		}
	}
	intent := qualityIntentForMedia(profile, resolvedPath, streams)
	encoder := strings.ToLower(strings.TrimSpace(workerStringValue(profile.WorkerConfig["videoEncoder"])))
	if encoder == "" || encoder == "auto" {
		encoder = strings.ToLower(resolvedVideoEncoder(profile))
	}
	capability, capabilitySource := h.activeEncoderCapability(encoder)
	var recommendation quality.EncoderRecommendation
	var err error
	switch encoder {
	case "hevc_qsv":
		if intent.MeasuredVideoBitrate == 0 {
			minimumRatio, maximumRatio, samples := scheduler.HistoricalQSVSampleRatios(h.db, workerStringValue(profile.WorkerConfig["hardwareQualityPreset"]), scheduler.ProfileEstimateFingerprint(profile))
			if samples == 1 {
				samples = 2
			} else if samples == 0 {
				minimumRatio, maximumRatio, samples = scheduler.HistoricalQSVSampleRatios(h.db, workerStringValue(profile.WorkerConfig["hardwareQualityPreset"]))
			}
			intent.HistoricalRatioMin, intent.HistoricalRatioMax, intent.HistoricalSamples = minimumRatio, maximumRatio, samples
		}
		if preset := strings.ToLower(strings.TrimSpace(workerStringValue(profile.WorkerConfig["hardwareQualityPreset"]))); preset == "" || preset == "custom" {
			recommendation = genericEncoderRecommendation(profile, intent, encoder)
			rateControl := strings.ToLower(strings.TrimSpace(workerStringValue(profile.WorkerConfig["qsvRateControl"])))
			if rateControl == "" {
				rateControl = "encoder default"
			}
			recommendation.RequestedRateControl = rateControl
			recommendation.EffectiveRateControl = rateControl
			qualityValue := workerIntValue(profile.WorkerConfig["globalQuality"], profile.QualityValue)
			recommendation.RequestedGlobalQuality = &qualityValue
			recommendation.GlobalQuality = &qualityValue
			recommendation.Warnings = append(recommendation.Warnings, "Custom QSV controls remain explicit")
		} else {
			recommendation, err = (quality.QSVTranslator{}).Translate(intent, qsvQualityCapabilities(capability, intent))
		}
		profile = applyQSVQualityRecommendation(profile, intent, capability)
	case "hevc_videotoolbox":
		recommendation, err = (quality.VideoToolboxTranslator{}).Translate(intent, videoToolboxQualityCapabilities(capability, profileUsesTenBit(profile)))
		preset := strings.TrimSpace(workerStringValue(profile.WorkerConfig["hardwareQualityPreset"]))
		if err == nil && (preset == "" || strings.EqualFold(preset, "custom")) {
			baseMbps := workerNumberValue(profile.WorkerConfig["videoToolboxBitrateMbps"], defaultVideoToolboxBitrateMbps(profile.QualityValue))
			targetMbps, maxrateMbps, bufferMbps := explicitVideoToolboxRates(profile, capability)
			base, target := int64(baseMbps*1_000_000), int64(targetMbps*1_000_000)
			maxrate, buffer := int64(maxrateMbps*1_000_000), int64(bufferMbps*1_000_000)
			recommendation.BaseTargetBitrate, recommendation.TargetBitrate = &base, &target
			recommendation.Maxrate, recommendation.Buffer = &maxrate, &buffer
			if intent.Duration > 0 {
				size := int64(float64(target) * intent.Duration.Seconds() / 8)
				recommendation.EstimatedOutputSize = &size
			}
		}
		profile = applyVideoToolboxQualityRecommendation(profile, intent, capability)
	default:
		recommendation = genericEncoderRecommendation(profile, intent, encoder)
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	args := videoCodecArgsForResolvedEncoder(profile, firstVideoStream(streams), encoder)
	if encoder == "hevc_qsv" {
		args = append(args, qsvWorkerArgsForCapability(profile, capability)...)
	}
	minimum, maximum := recommendationSizes(recommendation, intent)
	sourceSize := int64(0)
	if resolvedPath != "" {
		if info, statErr := os.Stat(resolvedPath); statErr == nil {
			sourceSize = info.Size()
		}
	}
	c.JSON(http.StatusOK, qualityRecommendationResponse{
		RequestedProfile: requested, EffectiveProfile: profile, Recommendation: recommendation,
		CapabilitySource: capabilitySource, FFmpegVideoArguments: args,
		EstimatedOutputMin: minimum, EstimatedOutputMax: maximum,
		EstimatedSavingsMin: max(int64(0), sourceSize-maximum), EstimatedSavingsMax: max(int64(0), sourceSize-minimum),
	})
}

func genericEncoderRecommendation(profile models.Profile, intent quality.QualityIntent, encoder string) quality.EncoderRecommendation {
	mode := strings.ToLower(strings.TrimSpace(profile.QualityMode))
	if mode == "" {
		mode = "encoder default"
	}
	policy, count := frameStructureBFrameIntent(profile)
	recommendation := quality.EncoderRecommendation{
		Encoder: encoder, EffectiveRateControl: mode, Profile: workerStringValue(profile.WorkerConfig["profile"]),
		PixelFormat: workerStringValue(profile.WorkerConfig["pixFmt"]), EstimateConfidence: "low",
		RequestedBFramePolicy: policy, EffectiveBFramePolicy: policy, RequestedBFrames: count,
		Warnings: []string{"Software and non-QSV bitrate estimates are planning ranges, not equivalents of hardware quality values."},
	}
	if recommendation.Profile == "" {
		recommendation.Profile = "encoder default"
	}
	if recommendation.PixelFormat == "" {
		recommendation.PixelFormat = profile.PixelFormat
	}
	if intent.SourceVideoBitrate <= 0 || intent.Duration <= 0 {
		return recommendation
	}
	ratio := 0.60
	if strings.EqualFold(profile.VideoCodec, "copy") {
		ratio = 1
	} else if mode == "crf" {
		ratio = math.Max(0.25, math.Min(0.85, 1.05-float64(profile.QualityValue)*0.0225))
	}
	if strings.Contains(strings.ToLower(workerStringValue(profile.WorkerConfig["videoFilters"])), "denoise") {
		ratio *= 0.95
	}
	videoBitrate := int64(float64(intent.SourceVideoBitrate) * ratio)
	minimumBitrate, maximumBitrate := int64(float64(videoBitrate)*0.80), int64(float64(videoBitrate)*1.20)
	minimum := int64(float64(minimumBitrate) * intent.Duration.Seconds() / 8)
	maximum := int64(float64(maximumBitrate) * intent.Duration.Seconds() / 8)
	recommendation.EstimatedVideoBitrate = &videoBitrate
	recommendation.EstimatedVideoBitrateMin, recommendation.EstimatedVideoBitrateMax = &minimumBitrate, &maximumBitrate
	recommendation.EstimatedOutputSizeMin, recommendation.EstimatedOutputSizeMax = &minimum, &maximum
	return recommendation
}

func (h AssetHandler) activeEncoderCapability(encoder string) (capabilities.EncoderCapability, string) {
	if snapshot, err := runtimeinfo.Latest(h.db); err == nil {
		if raw, ok := snapshot.Encoders[encoder]; ok {
			if capability, decoded := capabilities.DecodeEncoderCapability(raw); decoded {
				return capability, "active_runtime_snapshot"
			}
		}
	}
	return capabilities.CheckEncoder(encoder), "live_backend_probe"
}

func qsvQualityCapabilities(capability capabilities.EncoderCapability, intent quality.QualityIntent) quality.WorkerCapabilities {
	main10 := intent.RequestedBitDepth >= 10 || intent.RequestedPixelFormat == "p010le"
	result := quality.WorkerCapabilities{
		Encoder: "hevc_qsv", Main: capability.Usable, Main10: capability.Main10,
		ICQ: capability.QSVICQMain8, LAICQ: capability.QSVLAICQMain8, CQP: capability.QSVCQPMain8, VBR: capability.QSVVBRMain8, CBR: capability.QSVCBRMain8,
		LowPower: capability.LowPower, ExtendedBRC: capability.ExtendedBRC,
		VBRExtendedBRC: capability.QSVVBRExtBRCMain8, CBRExtendedBRC: capability.QSVCBRExtBRCMain8,
		VBRLookAhead: capability.QSVVBRLookAheadMain8, CBRLookAhead: capability.QSVCBRLookAheadMain8,
		AdaptiveI: capability.QSVAdaptiveIMain8, AdaptiveB: capability.QSVAdaptiveBMain8,
	}
	if main10 && capability.Main10 {
		result.ICQ, result.LAICQ = capability.QSVICQMain10, capability.QSVLAICQMain10
		result.CQP, result.VBR, result.CBR = capability.QSVCQPMain10, capability.QSVVBRMain10, capability.QSVCBRMain10
		result.AdaptiveI, result.AdaptiveB = capability.QSVAdaptiveIMain10, capability.QSVAdaptiveBMain10
		result.VBRExtendedBRC, result.CBRExtendedBRC = capability.QSVVBRExtBRCMain10, capability.QSVCBRExtBRCMain10
		result.VBRLookAhead, result.CBRLookAhead = capability.QSVVBRLookAheadMain10, capability.QSVCBRLookAheadMain10
	}
	return result
}

func firstVideoStream(streams MediaStreamInventory) *MediaStream {
	if len(streams.Video) == 0 {
		return nil
	}
	return &streams.Video[0]
}

func recommendationSizes(recommendation quality.EncoderRecommendation, intent quality.QualityIntent) (int64, int64) {
	minimum, maximum := int64(0), int64(0)
	if recommendation.EstimatedOutputSizeMin != nil {
		minimum = *recommendation.EstimatedOutputSizeMin
	}
	if recommendation.EstimatedOutputSizeMax != nil {
		maximum = *recommendation.EstimatedOutputSizeMax
	}
	if recommendation.EstimatedOutputSize != nil {
		if minimum == 0 {
			minimum = *recommendation.EstimatedOutputSize
		}
		if maximum == 0 {
			maximum = *recommendation.EstimatedOutputSize
		}
	}
	if intent.Duration > 0 {
		preserved := int64(float64(intent.AudioBitrate)*intent.Duration.Seconds()/8) + intent.SubtitleSize
		minimum += preserved
		maximum += preserved
	}
	return minimum, maximum
}
