package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
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

type SnapshotOperation struct {
	ID        string             `json:"id"`
	AssetPath string             `json:"assetPath"`
	Status    string             `json:"status"`
	Phase     string             `json:"phase"`
	Progress  float64            `json:"progress"`
	Message   string             `json:"message"`
	Result    *models.ScanResult `json:"result,omitempty"`
	Error     string             `json:"error,omitempty"`
	CreatedAt time.Time          `json:"createdAt"`
	UpdatedAt time.Time          `json:"updatedAt"`
}

var snapshotOperations = struct {
	sync.RWMutex
	items map[string]*SnapshotOperation
}{items: map[string]*SnapshotOperation{}}

func updateSnapshotOperation(id string, update func(*SnapshotOperation)) {
	snapshotOperations.Lock()
	defer snapshotOperations.Unlock()
	if operation := snapshotOperations.items[id]; operation != nil {
		update(operation)
		operation.UpdatedAt = time.Now()
	}
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
	ID                 any               `json:"id"`
	CodecType          string            `json:"codec_type"`
	CodecName          string            `json:"codec_name"`
	Width              int               `json:"width"`
	Height             int               `json:"height"`
	ColorTransfer      string            `json:"color_transfer"`
	ColorPrimaries     string            `json:"color_primaries"`
	ColorSpace         string            `json:"color_space"`
	ColorRange         string            `json:"color_range"`
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
	result, cached, scanErr := h.scanResolvedFile(path, info, request, nil)
	if scanErr != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": scanErr.Error()})
		return
	}
	if cached {
		c.JSON(http.StatusOK, result)
		return
	}
	c.JSON(http.StatusAccepted, result)
}

func (h ScannerHandler) scanResolvedFile(path string, info os.FileInfo, request ScanRequest, progress func(string, float64, string)) (models.ScanResult, bool, error) {
	report := func(phase string, value float64, message string) {
		if progress != nil {
			progress(phase, value, message)
		}
	}

	var existing models.ScanResult
	if !request.Force {
		if err := h.db.Where("path = ?", path).Order("created_at desc").First(&existing).Error; err == nil {
			if scanCacheMatchesFile(existing, info) {
				if len(existing.FrameStructureAnalysis) == 0 || jsonMapInt(existing.FrameStructureAnalysis, "version") < 2 {
					report("frame_structure", 80, "Adding the missing I, P, B and GOP analysis")
				}
				enrichCachedScan(h.db, &existing)
				applySnapshotDirectPlay(h.db, &existing)
				_ = h.db.Save(&existing).Error
				report("completed", 100, "Using the current asset snapshot")
				return existing, true, nil
			}
		}
	}

	probe, raw, err := runFFProbeWithProgress(path, normalizedAnalysisSeconds(request.AnalysisSeconds), report, frameStructureSamplingPolicy(h.db))
	if err != nil {
		return models.ScanResult{}, false, err
	}

	result := buildScanResult(path, info.Size(), probe, raw)
	applySnapshotDirectPlay(h.db, &result)
	if err := persistFinalAssetSnapshot(h.db, &result); err != nil {
		return models.ScanResult{}, false, err
	}
	report("completed", 100, "Asset snapshot completed")
	return result, false, nil
}

func (h ScannerHandler) StartSnapshotOperation(c *gin.Context) {
	var request ScanRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	path := resolveMediaPath(h.db, strings.TrimSpace(request.Path))
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": mediaPathReadError(err)})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "media path must point to a file"})
		}
		return
	}

	snapshotOperations.RLock()
	for _, operation := range snapshotOperations.items {
		if operation.AssetPath == path && operation.Status == "running" {
			copy := *operation
			snapshotOperations.RUnlock()
			c.JSON(http.StatusAccepted, copy)
			return
		}
	}
	snapshotOperations.RUnlock()

	now := time.Now()
	id := fmt.Sprintf("snapshot-%d", now.UnixNano())
	operation := &SnapshotOperation{ID: id, AssetPath: path, Status: "running", Phase: "preparing", Progress: 1, Message: "Preparing this asset snapshot", CreatedAt: now, UpdatedAt: now}
	snapshotOperations.Lock()
	snapshotOperations.items[id] = operation
	snapshotOperations.Unlock()
	response := *operation

	go func(fileInfo os.FileInfo) {
		result, _, scanErr := h.scanResolvedFile(path, fileInfo, request, func(phase string, progress float64, message string) {
			updateSnapshotOperation(id, func(item *SnapshotOperation) {
				item.Phase, item.Progress, item.Message = phase, progress, message
			})
		})
		if scanErr != nil {
			updateSnapshotOperation(id, func(item *SnapshotOperation) {
				item.Status, item.Phase, item.Error, item.Message = "error", "error", scanErr.Error(), "Asset snapshot failed"
			})
			return
		}
		updateSnapshotOperation(id, func(item *SnapshotOperation) {
			item.Status, item.Phase, item.Progress, item.Message, item.Result = "completed", "completed", 100, "Asset snapshot completed", &result
		})
	}(info)
	c.JSON(http.StatusAccepted, response)
}

func (h ScannerHandler) GetSnapshotOperation(c *gin.Context) {
	snapshotOperations.RLock()
	operation := snapshotOperations.items[c.Param("id")]
	if operation == nil {
		snapshotOperations.RUnlock()
		c.JSON(http.StatusNotFound, gin.H{"error": "snapshot operation not found"})
		return
	}
	copy := *operation
	snapshotOperations.RUnlock()
	c.JSON(http.StatusOK, copy)
}

func (h ScannerHandler) ListSnapshotOperations(c *gin.Context) {
	path := strings.TrimSpace(c.Query("path"))
	if path != "" {
		path = resolveMediaPath(h.db, path)
	}
	items := []SnapshotOperation{}
	snapshotOperations.RLock()
	for _, operation := range snapshotOperations.items {
		if path == "" || operation.AssetPath == path {
			items = append(items, *operation)
		}
	}
	snapshotOperations.RUnlock()
	c.JSON(http.StatusOK, gin.H{"operations": items})
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
	return runFFProbeWithProgress(path, analysisSeconds, nil, defaultFrameStructureSamplingPolicy())
}

func runFFProbeWithProgress(path string, analysisSeconds int, progress func(string, float64, string), samplingPolicy FrameStructureSamplingPolicy) (FFProbeResult, models.JSONMap, error) {
	report := func(phase string, value float64, message string) {
		if progress != nil {
			progress(phase, value, message)
		}
	}
	report("metadata", 10, "Reading media metadata")
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
	report("interlace", 30, "Analyzing motion and interlace samples")
	raw["interlaceAnalysis"] = detectInterlace(path, video.FieldOrder, parseFloat(probe.Format.Duration), analysisSeconds)
	report("crop", 55, "Checking sampled scenes for crop boundaries")
	raw["cropAnalysis"] = detectCrop(path, video.Width, video.Height, parseFloat(probe.Format.Duration))
	report("frame_structure", 80, "Inspecting I, P, B and GOP frame structure")
	frameContext, cancelFrameAnalysis := context.WithTimeout(context.Background(), 3*time.Minute)
	frameAnalysis, frameErr := analyzeVideoFrameStructureDistributed(frameContext, path, parseFloat(probe.Format.Duration), samplingPolicy)
	cancelFrameAnalysis()
	if frameErr == nil {
		encoded, _ := json.Marshal(frameAnalysis)
		frameMap := models.JSONMap{}
		_ = json.Unmarshal(encoded, &frameMap)
		raw["frameStructureAnalysis"] = frameMap
	} else {
		raw["frameStructureAnalysis"] = models.JSONMap{"version": 1, "status": "unverified", "error": frameErr.Error()}
	}
	report("persisting", 95, "Saving the asset snapshot")

	return probe, raw, nil
}

func buildScanResult(path string, size int64, probe FFProbeResult, raw models.JSONMap) models.ScanResult {
	video := firstStream(probe.Streams, "video")
	duration := parseFloat(probe.Format.Duration)
	bitrate := parseInt(probe.Format.Bitrate)
	if bitrate == 0 {
		bitrate = parseInt(video.Bitrate)
	}

	result := models.ScanResult{
		Path:                   path,
		FileName:               filepath.Base(path),
		Container:              probe.Format.FormatName,
		SizeBytes:              size,
		Duration:               duration,
		Bitrate:                bitrate,
		VideoCodec:             video.CodecName,
		Width:                  video.Width,
		Height:                 video.Height,
		HDR:                    isHDR(video),
		AudioTracks:            countStreams(probe.Streams, "audio"),
		SubtitleTracks:         countStreams(probe.Streams, "subtitle"),
		Chapters:               len(probe.Chapters),
		VideoStreams:           streamSummaries(probe.Streams, "video"),
		AudioStreams:           streamSummaries(probe.Streams, "audio"),
		SubtitleStreams:        streamSummaries(probe.Streams, "subtitle"),
		RawProbe:               raw,
		InterlaceAnalysis:      interlaceAnalysisFromRaw(raw),
		CropAnalysis:           analysisMapFromRaw(raw, "cropAnalysis"),
		FrameStructureAnalysis: analysisMapFromRaw(raw, "frameStructureAnalysis"),
	}
	result.CompatibilityAnalysis = buildPlaybackCompatibilityAnalysis(result)
	return result
}

// captureFinalAssetSnapshot persists the technical state of the published
// file while it is known to be readable. Converted inventory and the Snapshot
// dialog can then use this record without probing the media on first view.
func captureFinalAssetSnapshot(db *gorm.DB, path string) (models.ScanResult, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	info, err := os.Stat(path)
	if err != nil {
		return models.ScanResult{}, err
	}
	if info.IsDir() {
		return models.ScanResult{}, fmt.Errorf("final asset path must point to a file")
	}
	probe, raw, err := runFFProbe(path, 20)
	if err != nil {
		return models.ScanResult{}, err
	}
	result := buildScanResult(path, info.Size(), probe, raw)
	applySnapshotDirectPlay(db, &result)
	if err := persistFinalAssetSnapshot(db, &result); err != nil {
		return models.ScanResult{}, err
	}
	return result, nil
}

func applySnapshotDirectPlay(db *gorm.DB, result *models.ScanResult) {
	if db == nil || result == nil || len(result.RawProbe) == 0 {
		return
	}
	report, err := scheduler.EvaluateActualDirectPlay(db, result.RawProbe)
	if err != nil {
		result.DirectPlayAnalysis = models.JSONMap{"status": "unverified", "error": err.Error()}
		return
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		return
	}
	_ = json.Unmarshal(encoded, &result.DirectPlayAnalysis)
}

func persistFinalAssetSnapshot(db *gorm.DB, result *models.ScanResult) error {
	if db == nil || result == nil || strings.TrimSpace(result.Path) == "" {
		return fmt.Errorf("final asset snapshot requires a database and path")
	}
	result.Path = filepath.Clean(result.Path)
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("path = ?", result.Path).Delete(&models.ScanResult{}).Error; err != nil {
			return err
		}
		return tx.Create(result).Error
	})
}

func enrichCachedScan(db *gorm.DB, result *models.ScanResult) {
	if result.RawProbe == nil {
		result.RawProbe = models.JSONMap{}
	}
	if len(result.InterlaceAnalysis) == 0 || jsonMapInt(result.InterlaceAnalysis, "version") < interlaceAnalysisVersion {
		result.InterlaceAnalysis = interlaceAnalysisFromRaw(result.RawProbe)
		if len(result.InterlaceAnalysis) == 0 || jsonMapInt(result.InterlaceAnalysis, "version") < interlaceAnalysisVersion {
			fieldOrder := fieldOrderFromRawProbe(result.RawProbe)
			analysis := detectInterlace(result.Path, fieldOrder, result.Duration, 20)
			encoded, _ := json.Marshal(analysis)
			_ = json.Unmarshal(encoded, &result.InterlaceAnalysis)
			result.RawProbe["interlaceAnalysis"] = analysis
		}
	}
	if len(result.CropAnalysis) == 0 || jsonMapInt(result.CropAnalysis, "version") < 3 {
		result.CropAnalysis = analysisMapFromRaw(result.RawProbe, "cropAnalysis")
		if len(result.CropAnalysis) == 0 || jsonMapInt(result.CropAnalysis, "version") < 3 {
			analysis := detectCrop(result.Path, result.Width, result.Height, result.Duration)
			encoded, _ := json.Marshal(analysis)
			_ = json.Unmarshal(encoded, &result.CropAnalysis)
			result.RawProbe["cropAnalysis"] = analysis
		}
	}
	if len(result.FrameStructureAnalysis) == 0 || jsonMapInt(result.FrameStructureAnalysis, "version") < 2 {
		result.FrameStructureAnalysis = analysisMapFromRaw(result.RawProbe, "frameStructureAnalysis")
		if len(result.FrameStructureAnalysis) == 0 || jsonMapInt(result.FrameStructureAnalysis, "version") < 2 {
			frameContext, cancelFrameAnalysis := context.WithTimeout(context.Background(), 3*time.Minute)
			analysis, err := analyzeVideoFrameStructureDistributed(frameContext, result.Path, result.Duration, frameStructureSamplingPolicy(db))
			cancelFrameAnalysis()
			if err == nil {
				encoded, _ := json.Marshal(analysis)
				_ = json.Unmarshal(encoded, &result.FrameStructureAnalysis)
				result.RawProbe["frameStructureAnalysis"] = result.FrameStructureAnalysis
			}
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
	result.CompatibilityAnalysis = buildPlaybackCompatibilityAnalysis(*result)
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
			summary["colorSpace"] = stream.ColorSpace
			summary["colorRange"] = stream.ColorRange
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
	if inheritedMatroskaStatisticsAreStale(stream) {
		return 0
	}
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
	if inheritedMatroskaStatisticsAreStale(stream) {
		return 0
	}
	for _, key := range []string{"BPS", "BPS-eng"} {
		if value := parseInt(tagValue(stream.Tags, key, "")); value > 0 {
			return value
		}
	}
	return 0
}

func inheritedMatroskaStatisticsAreStale(stream FFProbeStream) bool {
	encoder := strings.TrimSpace(tagValue(stream.Tags, "ENCODER", ""))
	if encoder == "" {
		encoder = strings.TrimSpace(tagValue(stream.Tags, "encoder", ""))
	}
	statisticsWriter := strings.TrimSpace(tagValue(stream.Tags, "_STATISTICS_WRITING_APP", ""))
	if statisticsWriter == "" {
		statisticsWriter = strings.TrimSpace(tagValue(stream.Tags, "_STATISTICS_WRITING_APP-eng", ""))
	}
	// FFmpeg writes ENCODER on a newly encoded stream. MakeMKV statistics still
	// attached to that stream describe its pre-conversion source and must not be
	// used as output facts.
	return encoder != "" && statisticsWriter != "" && !strings.Contains(strings.ToLower(statisticsWriter), strings.ToLower(encoder))
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

func analysisMapFromRaw(raw models.JSONMap, key string) models.JSONMap {
	value, ok := raw[key]
	if !ok {
		return models.JSONMap{}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return models.JSONMap{}
	}
	result := models.JSONMap{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		return models.JSONMap{}
	}
	return result
}

func jsonMapInt(value models.JSONMap, key string) int {
	switch candidate := value[key].(type) {
	case int:
		return candidate
	case float64:
		return int(candidate)
	case json.Number:
		parsed, _ := candidate.Int64()
		return int(parsed)
	default:
		return 0
	}
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
