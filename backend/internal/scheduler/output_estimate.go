package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/quality"
	"gorm.io/gorm"
)

// mediaEstimate deliberately contains only the information needed for a
// preflight size estimate. It keeps probing and estimation independent from
// the conversion command builder.
type mediaEstimate struct {
	DurationSeconds      float64
	VideoBitrate         int64
	AudioBitrate         int64
	SubtitleBitrate      int64
	VideoHeight          int
	VideoWidth           int
	AudioTracks          int64
	MeasuredVideoBitrate int64
	HistoricalRatioMin   float64
	HistoricalRatioMax   float64
	HistoricalSamples    int
}

type ffprobeEstimateResponse struct {
	Streams []struct {
		CodecType string            `json:"codec_type"`
		Bitrate   string            `json:"bit_rate"`
		Width     int               `json:"width"`
		Height    int               `json:"height"`
		Tags      map[string]string `json:"tags"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
		Bitrate  string `json:"bit_rate"`
	} `json:"format"`
}

// probeMediaEstimate is intentionally best-effort. A plan remains usable on
// installations where the media mount or ffprobe is temporarily unavailable.
func probeMediaEstimate(path string) (mediaEstimate, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration,bit_rate:stream=codec_type,bit_rate,width,height:stream_tags", "-of", "json", path).Output()
	if err != nil {
		return mediaEstimate{}, false
	}
	var response ffprobeEstimateResponse
	if json.Unmarshal(output, &response) != nil {
		return mediaEstimate{}, false
	}
	estimate := mediaEstimate{DurationSeconds: estimateFloat(response.Format.Duration)}
	for _, stream := range response.Streams {
		bitrate := estimateInt(stream.Bitrate)
		if bitrate <= 0 {
			bitrate = bitrateFromStreamTags(stream.Tags, estimate.DurationSeconds)
		}
		switch strings.ToLower(stream.CodecType) {
		case "video":
			estimate.VideoBitrate += bitrate
			if estimate.VideoWidth == 0 {
				estimate.VideoWidth = stream.Width
			}
			if estimate.VideoHeight == 0 {
				estimate.VideoHeight = stream.Height
			}
		case "audio":
			estimate.AudioBitrate += bitrate
			estimate.AudioTracks++
		case "subtitle":
			estimate.SubtitleBitrate += bitrate
		}
	}
	if estimate.DurationSeconds <= 0 {
		return mediaEstimate{}, false
	}
	if estimate.VideoBitrate <= 0 {
		// A container bitrate is still preferable to a whole-file ratio. Reserve
		// one percent for muxing overhead when deriving the video stream.
		estimate.VideoBitrate = estimateInt(response.Format.Bitrate) - estimate.AudioBitrate - estimate.SubtitleBitrate - estimateInt(response.Format.Bitrate)/100
	}
	return estimate, estimate.VideoBitrate > 0
}

func bitrateFromStreamTags(tags map[string]string, duration float64) int64 {
	for key, value := range tags {
		if strings.HasPrefix(strings.ToUpper(key), "BPS") {
			if bitrate := estimateInt(value); bitrate > 0 {
				return bitrate
			}
		}
	}
	for key, value := range tags {
		if strings.HasPrefix(strings.ToUpper(key), "NUMBER_OF_BYTES") && duration > 0 {
			if bytes := estimateInt(value); bytes > 0 {
				return int64(float64(bytes) * 8 / duration)
			}
		}
	}
	return 0
}

type outputEstimate struct {
	MinBytes       int64
	MaxBytes       int64
	Confidence     string
	Method         string
	VideoBytes     int64
	AudioBytes     int64
	SubtitleBytes  int64
	Recommendation models.JSONMap
}

func estimatePlannedOutput(profile models.Profile, encoder string, source mediaEstimate, inputSize int64, workerCapabilities ...quality.WorkerCapabilities) (outputEstimate, bool) {
	if source.DurationSeconds <= 0 || source.VideoBitrate <= 0 {
		return outputEstimate{}, false
	}
	duration := source.DurationSeconds
	audioBitrate := source.AudioBitrate
	if strings.TrimSpace(profile.AudioCodec) != "" && !strings.EqualFold(profile.AudioCodec, "copy") {
		// The profile applies one audio codec to every mapped audio stream. 192k
		// per input track is a conservative default when a target bitrate was not
		// explicitly configured.
		tracks := maxInt64(1, source.AudioTracks)
		audioBitrate = tracks * 192000
	}
	if configBool(profile.WorkerConfig, "addAacStereoTrack") || configBool(profile.WorkerConfig, "addAacStereoDefault") {
		audioBitrate += 192000
	}
	audioBytes := bitrateBytes(audioBitrate, duration)
	subtitleBytes := int64(0)
	if profile.PreserveSubtitles {
		subtitleBytes = bitrateBytes(source.SubtitleBitrate, duration)
	}
	overhead := func(total int64) int64 { return maxInt64(16<<10, total/100) }

	if encoder == "hevc_videotoolbox" {
		targetKbps, ok := videoToolboxTargetKbps(profile, source)
		if !ok {
			return outputEstimate{}, false
		}
		videoBytes := bitrateBytes(int64(targetKbps)*1000, duration)
		total := videoBytes + audioBytes + subtitleBytes
		total += overhead(total)
		return outputEstimate{MinBytes: int64(float64(total) * .95), MaxBytes: int64(float64(total) * 1.05), Confidence: "medium", Method: "videotoolbox_source_video_bitrate", VideoBytes: videoBytes, AudioBytes: audioBytes, SubtitleBytes: subtitleBytes, Recommendation: models.JSONMap{"effectiveRateControl": "vbr", "targetBitrate": targetKbps * 1000, "estimateConfidence": "medium"}}, true
	}
	if encoder == "hevc_qsv" {
		intent := quality.NewIntent(quality.IntentInput{
			Preset:      configString(profile.WorkerConfig, "hardwareQualityPreset"),
			SourceWidth: source.VideoWidth, SourceHeight: source.VideoHeight,
			SourceVideoBitrate: source.VideoBitrate, DurationSeconds: source.DurationSeconds,
			AudioBitrate: source.AudioBitrate, SubtitleSize: bitrateBytes(source.SubtitleBitrate, source.DurationSeconds),
			VideoFilters:         configString(profile.WorkerConfig, "videoFilters"),
			RequestedRateControl: configString(profile.WorkerConfig, "qsvRateControl"),
			RequestedBitDepth:    profile.BitDepth, RequestedPixelFormat: configString(profile.WorkerConfig, "pixFmt"),
			MeasuredVideoBitrate: source.MeasuredVideoBitrate,
			HistoricalRatioMin:   source.HistoricalRatioMin, HistoricalRatioMax: source.HistoricalRatioMax, HistoricalSamples: source.HistoricalSamples,
		})
		capability := quality.WorkerCapabilities{Main: true, Main10: true, ICQ: true, LAICQ: true, CQP: true, VBR: true, CBR: true}
		if len(workerCapabilities) > 0 {
			capability = workerCapabilities[0]
		}
		recommendation, err := (quality.QSVTranslator{}).Translate(intent, capability)
		if err != nil || recommendation.EstimatedOutputSizeMin == nil || recommendation.EstimatedOutputSizeMax == nil {
			return outputEstimate{}, false
		}
		min := *recommendation.EstimatedOutputSizeMin + audioBytes + subtitleBytes
		max := *recommendation.EstimatedOutputSizeMax + audioBytes + subtitleBytes
		method := map[string]string{"high": "five_distributed_profile_samples", "medium": "qsv_historical_encoder_ratios", "low": "qsv_preset_calibration_range"}[recommendation.EstimateConfidence]
		videoBytes := (*recommendation.EstimatedOutputSizeMin + *recommendation.EstimatedOutputSizeMax) / 2
		return outputEstimate{MinBytes: min + overhead(min), MaxBytes: max + overhead(max), Confidence: recommendation.EstimateConfidence, Method: method, VideoBytes: videoBytes, AudioBytes: audioBytes, SubtitleBytes: subtitleBytes, Recommendation: models.JSONMap{"requestedRateControl": recommendation.RequestedRateControl, "effectiveRateControl": recommendation.EffectiveRateControl, "rateControlFallback": recommendation.RateControlFallback, "globalQuality": recommendation.GlobalQuality, "profile": recommendation.Profile, "pixelFormat": recommendation.PixelFormat, "estimateConfidence": recommendation.EstimateConfidence, "warnings": recommendation.Warnings}}, true
	}
	_ = inputSize
	return outputEstimate{}, false
}

func videoToolboxTargetKbps(profile models.Profile, source mediaEstimate) (int, bool) {
	intent := quality.NewIntent(quality.IntentInput{
		Preset:      configString(profile.WorkerConfig, "hardwareQualityPreset"),
		SourceWidth: source.VideoWidth, SourceHeight: source.VideoHeight,
		SourceVideoBitrate: source.VideoBitrate, DurationSeconds: source.DurationSeconds,
		AudioBitrate: source.AudioBitrate,
		SubtitleSize: bitrateBytes(source.SubtitleBitrate, source.DurationSeconds),
		VideoFilters: configString(profile.WorkerConfig, "videoFilters"),
		ColorPolicy:  configString(profile.WorkerConfig, "finalColorPolicy"),
	})
	recommendation, err := (quality.VideoToolboxTranslator{}).Translate(intent, quality.WorkerCapabilities{})
	if err != nil || recommendation.TargetBitrate == nil {
		return 0, false
	}
	return int(*recommendation.TargetBitrate / 1000), true
}

func bitrateBytes(bitrate int64, duration float64) int64 {
	return int64(float64(bitrate) * duration / 8)
}
func estimateInt(value string) int64 {
	parsed, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed
}
func estimateFloat(value string) float64 {
	parsed, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return parsed
}
func configString(config models.JSONMap, key string) string {
	value, _ := config[key].(string)
	return strings.ToLower(strings.TrimSpace(value))
}
func configBool(config models.JSONMap, key string) bool { value, _ := config[key].(bool); return value }
func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func persistedProfileSampleEstimate(db *gorm.DB, path string, profile models.Profile) (int64, string, bool) {
	var setting models.AppSetting
	if db.First(&setting, "key = ?", "profileSampleEstimates").Error != nil {
		return 0, "", false
	}
	records, _ := setting.Value["records"].([]interface{})
	for _, raw := range records {
		record, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		info, statErr := os.Stat(path)
		if statErr != nil || recordString(record["path"]) != path || uint(recordNumber(record["profileId"])) != profile.ID || int(recordNumber(record["profileVersion"])) != profile.ProfileVersion || recordString(record["profileFingerprint"]) != ProfileEstimateFingerprint(profile) || recordInt64(record["sourceSize"]) != info.Size() || recordInt64(record["sourceModifiedNs"]) != info.ModTime().UnixNano() {
			continue
		}
		estimate, ok := record["estimate"].(map[string]interface{})
		if !ok {
			continue
		}
		bytes, encoder := int64(recordNumber(estimate["estimatedVideoBytes"])), recordString(estimate["effectiveEncoder"])
		if bytes > 0 && encoder != "" {
			return bytes, encoder, true
		}
	}
	return 0, "", false
}

func historicalQSVSampleRatios(db *gorm.DB, preset string) (float64, float64, int) {
	ratios := []float64{}
	var sampleSetting models.AppSetting
	if db.First(&sampleSetting, "key = ?", "profileSampleEstimates").Error == nil {
		records, _ := sampleSetting.Value["records"].([]interface{})
		for _, raw := range records {
			record, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			estimate, ok := record["estimate"].(map[string]interface{})
			if !ok || recordString(estimate["effectiveEncoder"]) != "hevc_qsv" || recordString(estimate["hardwareQualityPreset"]) != preset {
				continue
			}
			duration := recordNumber(estimate["durationSeconds"])
			sourceBitrate := recordNumber(estimate["sourceVideoBitrate"])
			estimatedBytes := recordNumber(estimate["estimatedVideoBytes"])
			if duration > 0 && sourceBitrate > 0 && estimatedBytes > 0 {
				ratios = append(ratios, estimatedBytes/(sourceBitrate*duration/8))
			}
		}
	}
	var resultSetting models.AppSetting
	if db.First(&resultSetting, "key = ?", "encoderResultHistory").Error == nil {
		records, _ := resultSetting.Value["records"].([]interface{})
		for _, raw := range records {
			record, ok := raw.(map[string]interface{})
			if !ok || recordString(record["effectiveEncoder"]) != "hevc_qsv" || recordString(record["hardwareQualityPreset"]) != preset {
				continue
			}
			sourceBitrate := recordNumber(record["sourceVideoBitrate"])
			outputBitrate := recordNumber(record["outputVideoBitrate"])
			if sourceBitrate > 0 && outputBitrate > 0 {
				ratios = append(ratios, outputBitrate/sourceBitrate)
			}
		}
	}
	if len(ratios) < 2 {
		return 0, 0, len(ratios)
	}
	sort.Float64s(ratios)
	return ratios[0], ratios[len(ratios)-1], len(ratios)
}

// ProfileEstimateFingerprint identifies every profile value that can affect a
// measured encode. encoding/json sorts map keys, so WorkerConfig is stable.
func ProfileEstimateFingerprint(profile models.Profile) string {
	value := struct {
		Container         string         `json:"container"`
		VideoCodec        string         `json:"videoCodec"`
		AudioCodec        string         `json:"audioCodec"`
		BitDepth          int            `json:"bitDepth"`
		PixelFormat       string         `json:"pixelFormat"`
		QualityMode       string         `json:"qualityMode"`
		QualityValue      int            `json:"qualityValue"`
		PreserveHDR       bool           `json:"preserveHdr"`
		PreserveSubtitles bool           `json:"preserveSubtitles"`
		WorkerConfig      models.JSONMap `json:"workerConfig"`
	}{profile.Container, profile.VideoCodec, profile.AudioCodec, profile.BitDepth, profile.PixelFormat, profile.QualityMode, profile.QualityValue, profile.PreserveHDR, profile.PreserveSubtitles, profile.WorkerConfig}
	encoded, _ := json.Marshal(value)
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

func recordString(value interface{}) string { text, _ := value.(string); return text }
func recordNumber(value interface{}) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int64:
		return float64(typed)
	case int:
		return float64(typed)
	default:
		return 0
	}
}

func recordInt64(value interface{}) int64 {
	switch typed := value.(type) {
	case string:
		parsed, _ := strconv.ParseInt(typed, 10, 64)
		return parsed
	case json.Number:
		parsed, _ := typed.Int64()
		return parsed
	default:
		return int64(recordNumber(value))
	}
}
