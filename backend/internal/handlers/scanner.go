package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ScannerHandler struct {
	db *gorm.DB
}

type ScanRequest struct {
	Path  string `json:"path" binding:"required"`
	Force bool   `json:"force"`
}

type FFProbeResult struct {
	Format struct {
		Filename   string            `json:"filename"`
		FormatName string            `json:"format_name"`
		Duration   string            `json:"duration"`
		Bitrate    string            `json:"bit_rate"`
		Size       string            `json:"size"`
		Tags       map[string]string `json:"tags"`
	} `json:"format"`
	Streams  []FFProbeStream `json:"streams"`
	Chapters []struct {
		ID int `json:"id"`
	} `json:"chapters"`
}

type FFProbeStream struct {
	Index              int               `json:"index"`
	CodecType          string            `json:"codec_type"`
	CodecName          string            `json:"codec_name"`
	Width              int               `json:"width"`
	Height             int               `json:"height"`
	ColorTransfer      string            `json:"color_transfer"`
	ColorPrimaries     string            `json:"color_primaries"`
	PixFmt             string            `json:"pix_fmt"`
	Tags               map[string]string `json:"tags"`
	Disposition        map[string]int    `json:"disposition"`
	SideDataList       []map[string]any  `json:"side_data_list"`
	BitsPerRawSample   string            `json:"bits_per_raw_sample"`
	Profile            string            `json:"profile"`
	ChannelLayout      string            `json:"channel_layout"`
	Channels           int               `json:"channels"`
	Duration           string            `json:"duration"`
	Bitrate            string            `json:"bit_rate"`
	AverageFrameRate   string            `json:"avg_frame_rate"`
	RealBaseFrameRate  string            `json:"r_frame_rate"`
	SampleAspectRatio  string            `json:"sample_aspect_ratio"`
	DisplayAspectRatio string            `json:"display_aspect_ratio"`
	CodecLongName      string            `json:"codec_long_name"`
	SampleRate         string            `json:"sample_rate"`
	BitDepth           int               `json:"bits_per_sample"`
}

func NewScannerHandler(db *gorm.DB) ScannerHandler {
	return ScannerHandler{db: db}
}

func (h ScannerHandler) Scan(c *gin.Context) {
	var request ScanRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	path := strings.TrimSpace(request.Path)
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}

	var existing models.ScanResult
	if !request.Force {
		if err := h.db.Where("path = ?", path).Order("created_at desc").First(&existing).Error; err == nil {
			enrichCachedScan(&existing)
			_ = h.db.Save(&existing).Error
			c.JSON(http.StatusOK, existing)
			return
		}
	}

	if request.Force {
		_ = h.db.Where("path = ?", path).Delete(&models.ScanResult{}).Error
	}

	if !request.Force && existing.ID != 0 {
		c.JSON(http.StatusOK, existing)
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "media path is not readable from the backend container"})
		return
	}

	if info.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "media path must point to a file"})
		return
	}

	probe, raw, err := runFFProbe(path)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	result := buildScanResult(path, info.Size(), probe, raw)
	if err := h.db.Create(&result).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, result)
}

func runFFProbe(path string) (FFProbeResult, models.JSONMap, error) {
	ctxTimeout := 60 * time.Second
	cmd := exec.Command(
		"ffprobe",
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		"-show_chapters",
		path,
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	timer := time.AfterFunc(ctxTimeout, func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	defer timer.Stop()

	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return FFProbeResult{}, nil, &probeError{message: message}
	}

	var probe FFProbeResult
	if err := json.Unmarshal(stdout.Bytes(), &probe); err != nil {
		return FFProbeResult{}, nil, err
	}

	var raw models.JSONMap
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		raw = models.JSONMap{}
	}

	return probe, raw, nil
}

func buildScanResult(path string, size int64, probe FFProbeResult, raw models.JSONMap) models.ScanResult {
	video := firstStream(probe.Streams, "video")
	duration := parseFloat(probe.Format.Duration)
	bitrate := parseInt(probe.Format.Bitrate)
	if bitrate == 0 {
		bitrate = parseInt(video.Bitrate)
	}

	return models.ScanResult{
		Path:            path,
		FileName:        filepath.Base(path),
		Container:       probe.Format.FormatName,
		SizeBytes:       size,
		Duration:        duration,
		Bitrate:         bitrate,
		VideoCodec:      video.CodecName,
		Width:           video.Width,
		Height:          video.Height,
		HDR:             isHDR(video),
		AudioTracks:     countStreams(probe.Streams, "audio"),
		SubtitleTracks:  countStreams(probe.Streams, "subtitle"),
		Chapters:        len(probe.Chapters),
		VideoStreams:    streamSummaries(probe.Streams, "video"),
		AudioStreams:    streamSummaries(probe.Streams, "audio"),
		SubtitleStreams: streamSummaries(probe.Streams, "subtitle"),
		RawProbe:        raw,
	}
}

func enrichCachedScan(result *models.ScanResult) {
	if len(result.VideoStreams) > 0 || len(result.AudioStreams) > 0 || len(result.SubtitleStreams) > 0 {
		return
	}

	rawStreams, ok := result.RawProbe["streams"].([]any)
	if !ok {
		return
	}

	var streams []FFProbeStream
	bytes, err := json.Marshal(rawStreams)
	if err != nil {
		return
	}
	if err := json.Unmarshal(bytes, &streams); err != nil {
		return
	}

	result.VideoStreams = streamSummaries(streams, "video")
	result.AudioStreams = streamSummaries(streams, "audio")
	result.SubtitleStreams = streamSummaries(streams, "subtitle")
}

func firstStream(streams []FFProbeStream, codecType string) FFProbeStream {
	for _, stream := range streams {
		if stream.CodecType == codecType {
			return stream
		}
	}
	return FFProbeStream{}
}

func countStreams(streams []FFProbeStream, codecType string) int {
	count := 0
	for _, stream := range streams {
		if stream.CodecType == codecType {
			count++
		}
	}
	return count
}

func streamSummaries(streams []FFProbeStream, codecType string) models.JSONList {
	summaries := models.JSONList{}
	for _, stream := range streams {
		if stream.CodecType != codecType {
			continue
		}

		summary := models.JSONMap{
			"index":           stream.Index,
			"type":            stream.CodecType,
			"codec":           stream.CodecName,
			"codecLong":       stream.CodecLongName,
			"profile":         stream.Profile,
			"language":        tagValue(stream.Tags, "language", "und"),
			"title":           tagValue(stream.Tags, "title", ""),
			"duration":        parseFloat(stream.Duration),
			"bitrate":         parseInt(stream.Bitrate),
			"default":         dispositionValue(stream.Disposition, "default") == 1,
			"forced":          dispositionValue(stream.Disposition, "forced") == 1,
			"comment":         dispositionValue(stream.Disposition, "comment") == 1,
			"hearingImpaired": dispositionValue(stream.Disposition, "hearing_impaired") == 1,
		}

		if codecType == "video" {
			summary["width"] = stream.Width
			summary["height"] = stream.Height
			summary["pixFmt"] = stream.PixFmt
			summary["colorTransfer"] = stream.ColorTransfer
			summary["colorPrimaries"] = stream.ColorPrimaries
			summary["bitsPerRawSample"] = stream.BitsPerRawSample
			summary["avgFrameRate"] = stream.AverageFrameRate
			summary["realFrameRate"] = stream.RealBaseFrameRate
			summary["sampleAspectRatio"] = stream.SampleAspectRatio
			summary["displayAspectRatio"] = stream.DisplayAspectRatio
			summary["hdr"] = isHDR(stream)
		}

		if codecType == "audio" {
			summary["channels"] = stream.Channels
			summary["channelLayout"] = stream.ChannelLayout
			summary["sampleRate"] = parseInt(stream.SampleRate)
			summary["bitDepth"] = stream.BitDepth
		}

		summaries = append(summaries, summary)
	}

	return summaries
}

func tagValue(tags map[string]string, key string, fallback string) string {
	if tags == nil {
		return fallback
	}

	if value := strings.TrimSpace(tags[key]); value != "" {
		return value
	}

	return fallback
}

func dispositionValue(disposition map[string]int, key string) int {
	if disposition == nil {
		return 0
	}

	return disposition[key]
}

func isHDR(stream FFProbeStream) bool {
	if strings.Contains(stream.ColorTransfer, "smpte2084") || strings.Contains(stream.ColorTransfer, "arib-std-b67") {
		return true
	}
	if strings.Contains(stream.ColorPrimaries, "bt2020") {
		return true
	}
	if strings.Contains(stream.PixFmt, "10") && strings.Contains(strings.ToLower(stream.Profile), "main 10") {
		return true
	}
	return false
}

func parseFloat(value string) float64 {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func parseInt(value string) int64 {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

type probeError struct {
	message string
}

func (e *probeError) Error() string {
	return e.message
}
