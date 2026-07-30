package handlers

import (
	"fmt"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func buildPlaybackCompatibilityAnalysis(scan models.ScanResult) models.JSONMap {
	score := 100
	reasons := models.JSONList{}
	warnings := models.JSONList{}
	recommendations := models.JSONList{}
	videoStatus, audioStatus, subtitleStatus := "direct_play_possible", "direct_play_possible", "direct_play_possible"

	video := firstJSONStream(scan.VideoStreams)
	videoCodec := strings.ToLower(jsonString(video, "codec"))
	if videoCodec == "hevc" || videoCodec == "h265" {
		score -= 20
		videoStatus = "client_dependent"
		reasons = append(reasons, "HEVC playback is client-dependent; Jellyfin Web may transcode to H.264 when the browser cannot decode HEVC directly.")
	}
	sar := strings.TrimSpace(jsonString(video, "sampleAspectRatio"))
	if sar != "" && sar != "0:1" && sar != "1:1" {
		score -= 10
		if videoStatus == "direct_play_possible" {
			videoStatus = "client_dependent"
		}
		reasons = append(reasons, fmt.Sprintf("The video is anamorphic (SAR %s, DAR %s). Some clients may request normalization or scaling.", sar, fallback(jsonString(video, "displayAspectRatio"), "unknown")))
	}
	declaredFieldOrder := strings.ToLower(jsonString(video, "fieldOrder"))
	detectedScan := strings.ToLower(jsonMapString(scan.InterlaceAnalysis, "status"))
	if detectedScan == "interlaced" || detectedScan == "mixed" || detectedScan == "telecine_suspected" ||
		(declaredFieldOrder != "" && declaredFieldOrder != "unknown" && declaredFieldOrder != "progressive") {
		score -= 20
		videoStatus = "transcode_likely"
		reasons = append(reasons, "Interlaced or telecine video may make Jellyfin deinterlace and re-encode the video stream.")
	}

	hasAACStereo := false
	for _, raw := range scan.AudioStreams {
		stream := jsonListMap(raw)
		if strings.EqualFold(jsonString(stream, "codec"), "aac") && jsonInt(stream, "channels") > 0 && jsonInt(stream, "channels") <= 2 {
			hasAACStereo = true
			break
		}
	}
	if !hasAACStereo && len(scan.AudioStreams) > 0 {
		score -= 15
		audioStatus = "transcode_likely"
		reasons = append(reasons, "No AAC stereo compatibility track was found. Jellyfin may convert the selected surround track to AAC stereo for web playback.")
		recommendations = append(recommendations, "Preserve the original surround audio and add an AAC stereo compatibility track.")
	}

	imageSubtitleCodecs := []string{}
	for _, raw := range scan.SubtitleStreams {
		stream := jsonListMap(raw)
		codec := strings.ToLower(jsonString(stream, "codec"))
		if codec == "dvd_subtitle" || codec == "hdmv_pgs_subtitle" || codec == "pgssub" || codec == "dvb_subtitle" {
			imageSubtitleCodecs = append(imageSubtitleCodecs, codec)
		}
	}
	if len(imageSubtitleCodecs) > 0 {
		score -= 25
		subtitleStatus = "burn_in_likely"
		videoStatus = "transcode_likely"
		reasons = append(reasons, fmt.Sprintf("Selecting image subtitle tracks (%s) usually requires burn-in; burn-in forces a video transcode.", strings.Join(imageSubtitleCodecs, ", ")))
		recommendations = append(recommendations, "Generate an external SRT/ASS subtitle with OCR and select it instead of the bitmap subtitle when possible.")
	}

	colorSpace := strings.ToLower(jsonString(video, "colorSpace"))
	colorPrimaries := strings.ToLower(jsonString(video, "colorPrimaries"))
	colorTransfer := strings.ToLower(jsonString(video, "colorTransfer"))
	if scan.Width <= 720 && scan.Height <= 576 && (colorSpace == "smpte170m" || colorPrimaries == "smpte170m" || colorTransfer == "smpte170m") {
		warnings = append(warnings, "This SD source uses SMPTE 170M color metadata. Some Jellyfin transcode paths normalize it to BT.709; compare color metadata and intensity after transcoding.")
	}

	score = clamp(score, 0, 100)
	overall := "direct_play_likely"
	switch {
	case videoStatus == "transcode_likely" || subtitleStatus == "burn_in_likely":
		overall = "transcode_likely"
	case videoStatus == "client_dependent" || audioStatus == "transcode_likely":
		overall = "client_dependent"
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Test the intended Jellyfin client because DirectPlay support depends on the device and selected tracks.")
	}
	return models.JSONMap{
		"version": 1, "target": "jellyfin_web", "overall": overall, "score": score,
		"video": videoStatus, "audio": audioStatus, "subtitles": subtitleStatus,
		"reasons": reasons, "warnings": warnings, "recommendations": recommendations,
	}
}

func firstJSONStream(streams models.JSONList) map[string]any {
	if len(streams) == 0 {
		return map[string]any{}
	}
	return jsonListMap(streams[0])
}

func jsonListMap(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case models.JSONMap:
		return map[string]any(typed)
	default:
		return map[string]any{}
	}
}

func jsonString(value map[string]any, key string) string {
	text, _ := value[key].(string)
	return text
}

func jsonInt(value map[string]any, key string) int {
	switch number := value[key].(type) {
	case int:
		return number
	case int64:
		return int(number)
	case float64:
		return int(number)
	default:
		return 0
	}
}

func jsonMapString(value models.JSONMap, key string) string {
	text, _ := value[key].(string)
	return text
}
