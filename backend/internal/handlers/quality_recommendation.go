package handlers

import (
	"net/http"
	"os"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/capabilities"
	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/quality"
	"github.com/anuelvs/mvforge/backend/internal/runtimeinfo"
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
	encoder := strings.ToLower(workerStringValue(profile.WorkerConfig["videoEncoder"]))
	capability, capabilitySource := h.activeEncoderCapability(encoder)
	var recommendation quality.EncoderRecommendation
	var err error
	switch encoder {
	case "hevc_qsv":
		recommendation, err = (quality.QSVTranslator{}).Translate(intent, qsvQualityCapabilities(capability, intent))
		profile = applyQSVQualityRecommendation(profile, intent, capability)
	case "hevc_videotoolbox":
		recommendation, err = (quality.VideoToolboxTranslator{}).Translate(intent, quality.WorkerCapabilities{})
		profile = applyVideoToolboxQualityRecommendation(profile, intent)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "quality recommendations currently support hevc_qsv and hevc_videotoolbox"})
		return
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
		ICQ: capability.QSVICQMain8, CQP: capability.QSVCQPMain8, VBR: capability.QSVVBRMain8, CBR: capability.QSVCBRMain8,
		LowPower: capability.LowPower, ExtendedBRC: capability.ExtendedBRC,
		AdaptiveI: capability.AdaptiveI, AdaptiveB: capability.AdaptiveB, FullCombination: capability.QSVFullCombination,
	}
	if main10 && capability.Main10 {
		result.ICQ, result.LAICQ = capability.QSVICQMain10, capability.QSVLAICQMain10
		result.CQP, result.VBR, result.CBR = capability.QSVCQPMain10, capability.QSVVBRMain10, capability.QSVCBRMain10
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
