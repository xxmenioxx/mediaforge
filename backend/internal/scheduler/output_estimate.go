package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/encodingpolicy"
	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
)

// mediaEstimate deliberately contains only the information needed for a
// preflight size estimate. It keeps probing and estimation independent from
// the conversion command builder.
type mediaEstimate struct {
	DurationSeconds float64
	VideoBitrate    int64
	AudioBitrate    int64
	SubtitleBitrate int64
	VideoHeight     int
	AudioTracks     int64
}

type ffprobeEstimateResponse struct {
	Streams []struct {
		CodecType string            `json:"codec_type"`
		Bitrate   string            `json:"bit_rate"`
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
	output, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration,bit_rate:stream=codec_type,bit_rate,height:stream_tags", "-of", "json", path).Output()
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
	MinBytes      int64
	MaxBytes      int64
	Confidence    string
	Method        string
	VideoBytes    int64
	AudioBytes    int64
	SubtitleBytes int64
}

func estimatePlannedOutput(profile models.Profile, encoder string, source mediaEstimate, inputSize int64) (outputEstimate, bool) {
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
		targetKbps, ok := videoToolboxTargetKbps(profile, source.VideoBitrate, source.VideoHeight)
		if !ok {
			return outputEstimate{}, false
		}
		videoBytes := bitrateBytes(int64(targetKbps)*1000, duration)
		total := videoBytes + audioBytes + subtitleBytes
		total += overhead(total)
		return outputEstimate{MinBytes: int64(float64(total) * .95), MaxBytes: int64(float64(total) * 1.05), Confidence: "medium", Method: "videotoolbox_source_video_bitrate", VideoBytes: videoBytes, AudioBytes: audioBytes, SubtitleBytes: subtitleBytes}, true
	}
	if encoder == "hevc_qsv" {
		minRatio, maxRatio, ok := qsvOutputRatios(configString(profile.WorkerConfig, "hardwareQualityPreset"))
		if !ok {
			return outputEstimate{}, false
		}
		videoSourceBytes := bitrateBytes(source.VideoBitrate, duration)
		min := int64(float64(videoSourceBytes)*minRatio) + audioBytes + subtitleBytes
		max := int64(float64(videoSourceBytes)*maxRatio) + audioBytes + subtitleBytes
		return outputEstimate{MinBytes: min + overhead(min), MaxBytes: max + overhead(max), Confidence: "low", Method: "qsv_icq_source_video_range", VideoBytes: videoSourceBytes, AudioBytes: audioBytes, SubtitleBytes: subtitleBytes}, true
	}
	_ = inputSize
	return outputEstimate{}, false
}

func videoToolboxTargetKbps(profile models.Profile, sourceBitrate int64, sourceHeight int) (int, bool) {
	target, _, _, ok := encodingpolicy.VideoToolboxBitrate(configString(profile.WorkerConfig, "hardwareQualityPreset"), sourceBitrate, sourceHeight, configString(profile.WorkerConfig, "videoFilters"))
	return target, ok
}

func qsvOutputRatios(preset string) (float64, float64, bool) {
	values, ok := map[string][2]float64{
		"compact": {.25, .40}, "medium": {.32, .47}, "recommended": {.40, .55},
		"best_quality": {.50, .65}, "high_quality": {.60, .75}, "archive": {.70, .85}, "master": {.80, 1.00},
	}[preset]
	return values[0], values[1], ok
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
