package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ScannerHandler struct {
	db *gorm.DB
}

type ScanRequest struct {
	Path            string `json:"path" binding:"required"`
	Force           bool   `json:"force"`
	AnalysisSeconds int    `json:"analysisSeconds"`
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
	FieldOrder         string            `json:"field_order"`
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
	path = resolveMediaPath(h.db, path)

	info, err := os.Stat(path)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": mediaPathReadError(err)})
		return
	}

	if info.IsDir() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "media path must point to a file"})
		return
	}

	var existing models.ScanResult
	if !request.Force {
		if err := h.db.Where("path = ?", path).Order("created_at desc").First(&existing).Error; err == nil {
			if scanCacheMatchesFile(existing, info) {
				enrichCachedScan(&existing)
				_ = h.db.Save(&existing).Error
				c.JSON(http.StatusOK, existing)
				return
			}
			_ = h.db.Where("path = ?", path).Delete(&models.ScanResult{}).Error
		}
	}

	if request.Force {
		_ = h.db.Where("path = ?", path).Delete(&models.ScanResult{}).Error
	}

	probe, raw, err := runFFProbe(path, normalizedAnalysisSeconds(request.AnalysisSeconds))
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

func scanCacheMatchesFile(result models.ScanResult, info os.FileInfo) bool {
	if info == nil || result.CreatedAt.IsZero() || result.SizeBytes != info.Size() {
		return false
	}
	return !info.ModTime().After(result.CreatedAt)
}

func mediaPathReadError(err error) string {
	if os.IsNotExist(err) {
		return "media file no longer exists at the indexed path; synchronize Assets and verify the backend media mount"
	}
	if os.IsPermission(err) {
		return "backend does not have permission to read this media file or one of its parent folders"
	}
	return fmt.Sprintf("backend cannot read the media path: %v", err)
}

func resolveMediaPath(db *gorm.DB, path string) string {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "." || filepath.IsAbs(clean) {
		return clean
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return clean
	}

	var record models.AssetRecord
	if err := db.Where("relative_path = ? AND missing = ?", filepath.ToSlash(clean), false).
		Order("CASE status WHEN 'unprocessed' THEN 0 WHEN 'library' THEN 1 WHEN 'converted' THEN 2 WHEN 'archive' THEN 3 ELSE 4 END").
		First(&record).Error; err == nil {
		if info, statErr := os.Stat(record.Path); statErr == nil && !info.IsDir() {
			return filepath.Clean(record.Path)
		}
	}

	roots := [][2]string{
		{"rawRoot", "/media/raw"},
		{"libraryRoot", "/media/library"},
		{"originalsArchivePath", "/media/originals_archive"},
		{"stagingPath", "/media/staging"},
	}
	for _, configured := range roots {
		root, err := settingPath(db, configured[0], configured[1])
		if err != nil || strings.TrimSpace(root) == "" {
			continue
		}
		candidate := filepath.Join(root, clean)
		if pathIsInside(candidate, root) {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
		}
	}
	return clean
}

func runFFProbe(path string, analysisSeconds int) (FFProbeResult, models.JSONMap, error) {
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
	video := firstStream(probe.Streams, "video")
	raw["interlaceAnalysis"] = detectInterlace(path, video.FieldOrder, parseFloat(probe.Format.Duration), analysisSeconds)

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
		Path:              path,
		FileName:          filepath.Base(path),
		Container:         probe.Format.FormatName,
		SizeBytes:         size,
		Duration:          duration,
		Bitrate:           bitrate,
		VideoCodec:        video.CodecName,
		Width:             video.Width,
		Height:            video.Height,
		HDR:               isHDR(video),
		AudioTracks:       countStreams(probe.Streams, "audio"),
		SubtitleTracks:    countStreams(probe.Streams, "subtitle"),
		Chapters:          len(probe.Chapters),
		VideoStreams:      streamSummaries(probe.Streams, "video"),
		AudioStreams:      streamSummaries(probe.Streams, "audio"),
		SubtitleStreams:   streamSummaries(probe.Streams, "subtitle"),
		RawProbe:          raw,
		InterlaceAnalysis: interlaceAnalysisFromRaw(raw),
	}
}

func enrichCachedScan(result *models.ScanResult) {
	if result.RawProbe == nil {
		result.RawProbe = models.JSONMap{}
	}
	if len(result.InterlaceAnalysis) == 0 {
		result.InterlaceAnalysis = interlaceAnalysisFromRaw(result.RawProbe)
		if len(result.InterlaceAnalysis) == 0 {
			fieldOrder := fieldOrderFromRawProbe(result.RawProbe)
			analysis := detectInterlace(result.Path, fieldOrder, result.Duration, 20)
			encoded, _ := json.Marshal(analysis)
			_ = json.Unmarshal(encoded, &result.InterlaceAnalysis)
			result.RawProbe["interlaceAnalysis"] = analysis
		}
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
	result.HDR = isHDR(firstStream(streams, "video"))
	result.VideoStreams = streamSummaries(streams, "video")
	result.AudioStreams = streamSummaries(streams, "audio")
	result.SubtitleStreams = streamSummaries(streams, "subtitle")
}

func normalizedAnalysisSeconds(value int) int {
	if value == 10 {
		return 10
	}
	return 20
}

func fieldOrderFromRawProbe(raw models.JSONMap) string {
	rawStreams, ok := raw["streams"].([]any)
	if !ok {
		return "unknown"
	}
	for _, value := range rawStreams {
		stream, ok := value.(map[string]any)
		if !ok || stream["codec_type"] != "video" {
			continue
		}
		if fieldOrder, ok := stream["field_order"].(string); ok {
			return fieldOrder
		}
	}
	return "unknown"
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
			summary["fieldOrder"] = normalizeFieldOrder(stream.FieldOrder)
		}
		if sizeBytes := streamSizeBytes(stream); sizeBytes > 0 {
			summary["sizeBytes"] = sizeBytes
			summary["sizeEstimated"] = false
		} else if bitrate := streamBitrate(stream); bitrate > 0 && parseFloat(stream.Duration) > 0 {
			summary["sizeBytes"] = int64(float64(bitrate) * parseFloat(stream.Duration) / 8)
			summary["sizeEstimated"] = true
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

func streamSizeBytes(stream FFProbeStream) int64 {
	for _, key := range []string{"NUMBER_OF_BYTES", "NUMBER_OF_BYTES-eng"} {
		if value := parseInt(tagValue(stream.Tags, key, "")); value > 0 {
			return value
		}
	}
	return 0
}

func streamBitrate(stream FFProbeStream) int64 {
	if value := parseInt(stream.Bitrate); value > 0 {
		return value
	}
	for _, key := range []string{"BPS", "BPS-eng"} {
		if value := parseInt(tagValue(stream.Tags, key, "")); value > 0 {
			return value
		}
	}
	return 0
}

func interlaceAnalysisFromRaw(raw models.JSONMap) models.JSONMap {
	value, ok := raw["interlaceAnalysis"]
	if !ok {
		return models.JSONMap{}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return models.JSONMap{}
	}
	result := models.JSONMap{}
	if json.Unmarshal(encoded, &result) != nil {
		return models.JSONMap{}
	}
	return result
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
	transfer := strings.ToLower(stream.ColorTransfer)
	if strings.Contains(transfer, "smpte2084") || strings.Contains(transfer, "arib-std-b67") {
		return true
	}
	for _, sideData := range stream.SideDataList {
		kind := strings.ToLower(fmt.Sprint(sideData["side_data_type"]))
		if strings.Contains(kind, "mastering display") || strings.Contains(kind, "content light") || strings.Contains(kind, "dovi") || strings.Contains(kind, "dolby vision") {
			return true
		}
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
