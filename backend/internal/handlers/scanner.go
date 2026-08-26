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
	"slices"
	"sort"
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
	ID                  string             `json:"id"`
	AssetPath           string             `json:"assetPath"`
	Status              string             `json:"status"`
	Phase               string             `json:"phase"`
	Progress            float64            `json:"progress"`
	Message             string             `json:"message"`
	StageTimingsMs      map[string]int64   `json:"stageTimingsMs,omitempty"`
	DurationMs          int64              `json:"durationMs"`
	CacheHit            bool               `json:"cacheHit"`
	IncrementalRefresh  bool               `json:"incrementalRefresh"`
	ReusedComponents    []string           `json:"reusedComponents,omitempty"`
	RefreshedComponents []string           `json:"refreshedComponents,omitempty"`
	ComponentStatuses   map[string]string  `json:"componentStatuses,omitempty"`
	FallbackReason      string             `json:"fallbackReason,omitempty"`
	Result              *models.ScanResult `json:"result,omitempty"`
	Error               string             `json:"error,omitempty"`
	CreatedAt           time.Time          `json:"createdAt"`
	UpdatedAt           time.Time          `json:"updatedAt"`
	phaseStartedAt      time.Time
}

var snapshotOperations = struct {
	sync.RWMutex
	items   map[string]*SnapshotOperation
	cancels map[string]context.CancelFunc
}{items: map[string]*SnapshotOperation{}, cancels: map[string]context.CancelFunc{}}

const snapshotOperationTimeout = 15 * time.Minute

const (
	snapshotOperationRetention  = 24 * time.Hour
	snapshotOperationMaxHistory = 500
)

func updateSnapshotOperation(id string, update func(*SnapshotOperation)) {
	snapshotOperations.Lock()
	defer snapshotOperations.Unlock()
	if operation := snapshotOperations.items[id]; operation != nil {
		update(operation)
		operation.UpdatedAt = time.Now()
	}
}

func finishRunningSnapshotOperation(id string, update func(*SnapshotOperation)) bool {
	snapshotOperations.Lock()
	defer snapshotOperations.Unlock()
	operation := snapshotOperations.items[id]
	if operation == nil || operation.Status != "running" {
		return false
	}
	update(operation)
	operation.UpdatedAt = time.Now()
	return true
}

func snapshotOperationCopy(operation *SnapshotOperation) SnapshotOperation {
	copy := *operation
	if operation.StageTimingsMs != nil {
		copy.StageTimingsMs = make(map[string]int64, len(operation.StageTimingsMs))
		for phase, duration := range operation.StageTimingsMs {
			copy.StageTimingsMs[phase] = duration
		}
	}
	copy.ReusedComponents = append([]string(nil), operation.ReusedComponents...)
	copy.RefreshedComponents = append([]string(nil), operation.RefreshedComponents...)
	if operation.ComponentStatuses != nil {
		copy.ComponentStatuses = make(map[string]string, len(operation.ComponentStatuses))
		for component, status := range operation.ComponentStatuses {
			copy.ComponentStatuses[component] = status
		}
	}
	return copy
}

func transitionSnapshotOperationPhase(operation *SnapshotOperation, phase string, progress float64, message string, now time.Time) {
	if operation == nil {
		return
	}
	if operation.StageTimingsMs == nil {
		operation.StageTimingsMs = map[string]int64{}
	}
	if operation.Phase != "" && operation.Phase != phase && !operation.phaseStartedAt.IsZero() {
		operation.StageTimingsMs[operation.Phase] += now.Sub(operation.phaseStartedAt).Milliseconds()
		operation.phaseStartedAt = now
	} else if operation.phaseStartedAt.IsZero() {
		operation.phaseStartedAt = now
	}
	operation.Phase, operation.Progress, operation.Message = phase, progress, message
	operation.DurationMs = now.Sub(operation.CreatedAt).Milliseconds()
}

func finishSnapshotOperationTiming(operation *SnapshotOperation, now time.Time) {
	if operation == nil {
		return
	}
	if operation.StageTimingsMs == nil {
		operation.StageTimingsMs = map[string]int64{}
	}
	if operation.Phase != "" && !operation.phaseStartedAt.IsZero() {
		operation.StageTimingsMs[operation.Phase] += now.Sub(operation.phaseStartedAt).Milliseconds()
		operation.phaseStartedAt = now
	}
	operation.DurationMs = now.Sub(operation.CreatedAt).Milliseconds()
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
	Level              int               `json:"level"`
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
	assetMutationMu.Lock()
	var request ScanRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		assetMutationMu.Unlock()
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	path := strings.TrimSpace(request.Path)
	if path == "" {
		assetMutationMu.Unlock()
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	path = resolveMediaPath(h.db, path)

	info, err := os.Stat(path)
	if err != nil {
		assetMutationMu.Unlock()
		c.JSON(http.StatusBadRequest, gin.H{"error": mediaPathReadError(err)})
		return
	}

	if info.IsDir() {
		assetMutationMu.Unlock()
		c.JSON(http.StatusBadRequest, gin.H{"error": "media path must point to a file"})
		return
	}
	if active, maintenanceErr := activeAssetMaintenance(h.db, path); maintenanceErr != nil {
		assetMutationMu.Unlock()
		c.JSON(http.StatusInternalServerError, gin.H{"error": maintenanceErr.Error()})
		return
	} else if active {
		assetMutationMu.Unlock()
		c.JSON(http.StatusConflict, gin.H{"error": "asset has an active maintenance operation and cannot be scanned"})
		return
	}
	markSynchronousScan(path, 1)
	assetMutationMu.Unlock()
	defer markSynchronousScan(path, -1)
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
	return h.scanResolvedFileContext(context.Background(), path, info, request, progress)
}

func snapshotRequiresFrameStructureRefresh(snapshot models.ScanResult) bool {
	hasVideo := len(snapshot.VideoStreams) > 0 ||
		strings.TrimSpace(snapshot.VideoCodec) != "" ||
		snapshot.Width > 0 ||
		snapshot.Height > 0

	if !hasVideo {
		return false
	}

	analysis := snapshot.FrameStructureAnalysis
	if len(analysis) == 0 {
		return true
	}

	if scanFrameRate(snapshot) <= 0 {
		return true
	}

	return false
}

const snapshotCacheSchemaVersion = 1

var snapshotAnalysisVersions = models.JSONMap{
	"metadata":       1,
	"interlace":      interlaceAnalysisVersion,
	"crop":           3,
	"frameStructure": 2,
	"cadence":        cadenceAnalysisVersion,
}

var snapshotComponentNames = []string{"metadata", "interlace", "crop", "frameStructure", "cadence"}

func snapshotFingerprint(path string, info os.FileInfo) models.JSONMap {
	fingerprint := models.JSONMap{"path": filepath.Clean(path)}
	if info != nil {
		fingerprint["sizeBytes"] = info.Size()
		fingerprint["mtimeNs"] = strconv.FormatInt(info.ModTime().UnixNano(), 10)
	}
	return fingerprint
}

func stampSnapshotCacheMetadata(result *models.ScanResult, path string, info os.FileInfo) {
	stampSnapshotCacheMetadataWithRefresh(result, path, info, "full", nil, snapshotComponentNames)
}

func stampSnapshotCacheMetadataWithRefresh(result *models.ScanResult, path string, info os.FileInfo, mode string, reused, refreshed []string) {
	stampSnapshotCacheMetadataWithRefreshReason(result, path, info, mode, reused, refreshed, "")
}

func stampSnapshotCacheMetadataWithRefreshReason(result *models.ScanResult, path string, info os.FileInfo, mode string, reused, refreshed []string, fallbackReason string) {
	if result == nil {
		return
	}
	if result.RawProbe == nil {
		result.RawProbe = models.JSONMap{}
	}
	components := models.JSONMap{}
	for component, version := range snapshotAnalysisVersions {
		components[component] = models.JSONMap{
			"version": version,
			"status":  snapshotComponentStatus(*result, component),
		}
	}
	lastRefresh := models.JSONMap{
		"mode": mode, "reused": append([]string(nil), reused...), "refreshed": append([]string(nil), refreshed...),
	}
	if fallbackReason != "" {
		lastRefresh["fallbackReason"] = fallbackReason
	}
	result.RawProbe["snapshotCache"] = models.JSONMap{
		"schemaVersion": snapshotCacheSchemaVersion,
		"fingerprint":   snapshotFingerprint(path, info),
		"components":    components,
		"lastRefresh":   lastRefresh,
	}
}

func migrateLegacySnapshotCache(result *models.ScanResult, path string, info os.FileInfo) {
	if result == nil {
		return
	}
	if result.RawProbe == nil {
		result.RawProbe = models.JSONMap{}
	}
	components := models.JSONMap{}
	versions := map[string]int{
		"metadata":       legacyMetadataVersion(*result),
		"interlace":      jsonMapInt(result.InterlaceAnalysis, "version"),
		"crop":           jsonMapInt(result.CropAnalysis, "version"),
		"frameStructure": jsonMapInt(result.FrameStructureAnalysis, "version"),
		"cadence":        jsonMapInt(result.CadenceAnalysis, "version"),
	}
	for _, component := range snapshotComponentNames {
		status := snapshotComponentStatus(*result, component)
		if versions[component] == 0 && status == "valid" {
			status = "unknown"
		}
		components[component] = models.JSONMap{"version": versions[component], "status": status, "provenance": "legacy_inferred"}
	}
	result.RawProbe["snapshotCache"] = models.JSONMap{
		"schemaVersion": snapshotCacheSchemaVersion,
		"fingerprint":   snapshotFingerprint(path, info),
		"components":    components,
		"lastRefresh":   models.JSONMap{"mode": "legacy_migration", "reused": []string{}, "refreshed": []string{}},
	}
}

func legacyMetadataVersion(result models.ScanResult) int {
	var probe FFProbeResult
	if len(result.RawProbe) == 0 || !decodeJSONValue(result.RawProbe, &probe) {
		return 0
	}
	if len(probe.Streams) == 0 && strings.TrimSpace(probe.Format.FormatName) == "" && strings.TrimSpace(probe.Format.Duration) == "" {
		return 0
	}
	return workerIntValue(snapshotAnalysisVersions["metadata"], 0)
}

func legacySnapshotIdentityMatches(result models.ScanResult, info os.FileInfo) bool {
	return info != nil && result.SizeBytes > 0 && result.SizeBytes == info.Size()
}

func snapshotComponentStatus(result models.ScanResult, component string) string {
	var analysis models.JSONMap
	switch component {
	case "metadata":
		if len(result.VideoStreams)+len(result.AudioStreams)+len(result.SubtitleStreams) > 0 || result.Duration > 0 || result.Container != "" {
			return "valid"
		}
		return "missing"
	case "interlace":
		analysis = result.InterlaceAnalysis
	case "crop":
		analysis = result.CropAnalysis
	case "frameStructure":
		analysis = result.FrameStructureAnalysis
	case "cadence":
		analysis = result.CadenceAnalysis
	}
	if len(analysis) == 0 {
		return "missing"
	}
	status := strings.ToLower(strings.TrimSpace(stringFromUnknown(analysis["status"])))
	if status == "unverified" || status == "timeout" || analysis["error"] != nil {
		return "unverified"
	}
	return "valid"
}

func snapshotCacheMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case models.JSONMap:
		return map[string]any(typed), true
	case map[string]any:
		return typed, true
	default:
		return nil, false
	}
}

func snapshotCacheMatches(snapshot models.ScanResult, path string, info os.FileInfo) (matches bool, legacy bool) {
	fingerprintMatches, legacy, staleComponents := snapshotCacheState(snapshot, path, info)
	return fingerprintMatches && len(staleComponents) == 0, legacy
}

func snapshotCacheState(snapshot models.ScanResult, path string, info os.FileInfo) (fingerprintMatches bool, legacy bool, staleComponents []string) {
	cache, ok := snapshotCacheMap(snapshot.RawProbe["snapshotCache"])
	if !ok {
		return true, true, nil
	}
	if workerIntValue(cache["schemaVersion"], 0) != snapshotCacheSchemaVersion {
		return false, false, nil
	}
	fingerprint, ok := snapshotCacheMap(cache["fingerprint"])
	if !ok || filepath.Clean(stringFromUnknown(fingerprint["path"])) != filepath.Clean(path) || info == nil {
		return false, false, nil
	}
	if jsonInt64(fingerprint["sizeBytes"]) != info.Size() ||
		jsonInt64(fingerprint["mtimeNs"]) != info.ModTime().UnixNano() {
		return false, false, nil
	}
	components, ok := snapshotCacheMap(cache["components"])
	if !ok {
		return true, false, []string{"component_manifest"}
	}
	for component, version := range snapshotAnalysisVersions {
		storedVersion := workerIntValue(components[component], 0)
		if entry, entryOK := snapshotCacheMap(components[component]); entryOK {
			storedVersion = workerIntValue(entry["version"], 0)
		}
		if storedVersion != workerIntValue(version, 0) {
			staleComponents = append(staleComponents, component)
		}
	}
	sort.Strings(staleComponents)
	return true, false, staleComponents
}

func snapshotRefreshDetails(snapshot models.ScanResult, cacheHit bool) (reused, refreshed []string, statuses map[string]string) {
	statuses = map[string]string{}
	cache, ok := snapshotCacheMap(snapshot.RawProbe["snapshotCache"])
	if !ok {
		return nil, nil, statuses
	}
	if components, componentsOK := snapshotCacheMap(cache["components"]); componentsOK {
		for _, component := range snapshotComponentNames {
			if entry, entryOK := snapshotCacheMap(components[component]); entryOK {
				statuses[component] = stringFromUnknown(entry["status"])
			}
		}
	}
	if cacheHit {
		return append([]string(nil), snapshotComponentNames...), nil, statuses
	}
	if lastRefresh, refreshOK := snapshotCacheMap(cache["lastRefresh"]); refreshOK {
		_ = decodeJSONValue(lastRefresh["reused"], &reused)
		_ = decodeJSONValue(lastRefresh["refreshed"], &refreshed)
	}
	return reused, refreshed, statuses
}

func snapshotFallbackReason(snapshot models.ScanResult) string {
	cache, ok := snapshotCacheMap(snapshot.RawProbe["snapshotCache"])
	if !ok {
		return ""
	}
	lastRefresh, ok := snapshotCacheMap(cache["lastRefresh"])
	if !ok {
		return ""
	}
	return stringFromUnknown(lastRefresh["fallbackReason"])
}

func incrementalRefreshSupported(staleComponents []string) bool {
	if len(staleComponents) == 0 {
		return false
	}
	for _, component := range staleComponents {
		if component == "metadata" || component == "component_manifest" {
			return false
		}
	}
	return true
}

func (h ScannerHandler) scanResolvedFileContext(ctx context.Context, path string, info os.FileInfo, request ScanRequest, progress func(string, float64, string)) (models.ScanResult, bool, error) {
	report := func(phase string, value float64, message string) {
		if progress != nil {
			progress(phase, value, message)
		}
	}

	fullRefreshMode := "full"
	fallbackReason := ""
	var existing models.ScanResult
	foundExisting := false
	if !request.Force {
		if err := h.db.Where("path = ?", path).Order("created_at desc").First(&existing).Error; err == nil {
			foundExisting = true
			fingerprintMatches, legacyCache, staleComponents := snapshotCacheState(existing, path, info)
			if legacyCache {
				if legacySnapshotIdentityMatches(existing, info) {
					migrateLegacySnapshotCache(&existing, path, info)
					_ = h.db.Model(&existing).Update("raw_probe", existing.RawProbe).Error
					fingerprintMatches, _, staleComponents = snapshotCacheState(existing, path, info)
				} else {
					fingerprintMatches = false
					report("legacy_identity_mismatch", 8, "Legacy snapshot identity cannot be verified; starting a full analysis")
				}
			}
			cacheMatches := fingerprintMatches && len(staleComponents) == 0
			if snapshotRequiresFrameStructureRefresh(existing) || !cacheMatches {
				if fingerprintMatches && incrementalRefreshSupported(staleComponents) {
					report("incremental_refresh", 10, "Refreshing stale analysis components while reusing valid evidence")
					refreshed, refreshErr := h.refreshSnapshotComponentsContext(ctx, existing, path, info, normalizedAnalysisSeconds(request.AnalysisSeconds), staleComponents, report)
					if refreshErr == nil {
						return refreshed, false, nil
					}
					if ctx.Err() != nil {
						return models.ScanResult{}, false, ctx.Err()
					}
					fullRefreshMode = "full_fallback"
					fallbackReason = incrementalFallbackReason(refreshErr)
					report("incremental_fallback", 12, "Cached evidence was unusable; starting a full analysis")
				}
				report("snapshot_refresh", 10, "Existing snapshot is incomplete or stale; rebuilding asset analysis")
			} else {
				if ensureFrameStructureRecommendation(&existing) {
					_ = h.db.Model(&existing).Update("frame_structure_recommendation", existing.FrameStructureRecommendation).Error
				}
				if ensureHEVCLevelRecommendation(&existing) {
					_ = h.db.Model(&existing).Update("hevc_level_recommendation", existing.HEVCLevelRecommendation).Error
				}
				if ensureCadenceRecommendation(&existing) {
					_ = h.db.Model(&existing).Update("cadence_recommendation", existing.CadenceRecommendation).Error
				}
				report("completed", 100, "Using the existing asset snapshot")
				return existing, true, nil
			}
		}
		if !foundExisting {
			if inherited, ok := inheritedOriginalSnapshot(h.db, path, info); ok {
				report("completed", 100, "Using the Raw snapshot for this archived original")
				return inherited, true, nil
			}
		}
	}

	probe, raw, err := runFFProbeWithProgressContext(ctx, path, normalizedAnalysisSeconds(request.AnalysisSeconds), report, frameStructureSamplingPolicy(h.db))
	if err != nil {
		return models.ScanResult{}, false, err
	}

	result := buildScanResult(path, info.Size(), probe, raw)
	stampSnapshotCacheMetadataWithRefreshReason(&result, path, info, fullRefreshMode, nil, snapshotComponentNames, fallbackReason)
	applySnapshotDirectPlay(h.db, &result)
	if err := persistFinalAssetSnapshot(h.db, &result); err != nil {
		return models.ScanResult{}, false, err
	}
	report("completed", 100, "Asset snapshot completed")
	return result, false, nil
}

func incrementalFallbackReason(err error) string {
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "metadata") && strings.Contains(message, "decode"):
		return "cached_metadata_invalid"
	case strings.Contains(message, "frame structure"):
		return "cached_frame_structure_invalid"
	case strings.Contains(message, "interlace"):
		return "cached_interlace_invalid"
	default:
		return "cached_dependency_invalid"
	}
}

func (h ScannerHandler) refreshSnapshotComponentsContext(ctx context.Context, existing models.ScanResult, path string, info os.FileInfo, analysisSeconds int, staleComponents []string, report func(string, float64, string)) (models.ScanResult, error) {
	var probe FFProbeResult
	encodedRaw, err := json.Marshal(existing.RawProbe)
	if err != nil || json.Unmarshal(encodedRaw, &probe) != nil {
		return models.ScanResult{}, fmt.Errorf("cached metadata cannot be decoded for incremental refresh")
	}
	if len(probe.Streams) == 0 || (strings.TrimSpace(probe.Format.FormatName) == "" && strings.TrimSpace(probe.Format.Duration) == "") {
		return models.ScanResult{}, fmt.Errorf("cached metadata cannot be decoded for incremental refresh")
	}
	video := firstStream(probe.Streams, "video")
	duration := existing.Duration
	if duration <= 0 {
		duration = parseFloat(probe.Format.Duration)
	}
	plan := canonicalSamplingPlan(duration, frameStructureSamplingPolicy(h.db))
	var frameAnalysis QSVFrameStructureAnalysis
	frameStale := slices.Contains(staleComponents, "frameStructure")
	if frameStale {
		report("frame_structure", 35, "Refreshing Frame Structure evidence")
		frameContext, cancel := context.WithTimeout(ctx, 3*time.Minute)
		frameAnalysis, err = analyzeVideoFrameStructureWithSamplingPlan(frameContext, path, plan)
		cancel()
		if ctx.Err() != nil {
			return models.ScanResult{}, ctx.Err()
		}
		if err != nil {
			existing.RawProbe["frameStructureAnalysis"] = models.JSONMap{"version": 2, "status": "unverified", "error": err.Error()}
		} else {
			existing.RawProbe["frameStructureAnalysis"] = structToJSONMap(frameAnalysis)
		}
	} else if !decodeJSONValue(existing.FrameStructureAnalysis, &frameAnalysis) {
		frameAnalysis = QSVFrameStructureAnalysis{Version: 2, Source: "cached_unverified"}
	}

	var interlace InterlaceAnalysis
	_ = decodeJSONValue(existing.InterlaceAnalysis, &interlace)
	interlaceStale := slices.Contains(staleComponents, "interlace")
	cropStale := slices.Contains(staleComponents, "crop")
	if interlaceStale || cropStale {
		pixelPlan := plan
		if len(frameAnalysis.Positions) > 0 {
			pixelPlan.Positions = append([]float64(nil), frameAnalysis.Positions...)
			pixelPlan.Adaptive = false
			pixelPlan.InitialWindows = len(pixelPlan.Positions)
		}
		pixelSession := newPixelSamplingSession(path, video.Width, video.Height, pixelPlan)
		if interlaceStale {
			report("interlace", 60, "Refreshing Interlace from shared frame and pixel evidence")
			interlace = detectInterlaceWithSharedEvidenceContext(ctx, path, video.FieldOrder, analysisSeconds, false, pixelPlan, frameAnalysis, pixelSession)
			interlace.Codec = video.CodecName
			interlace.AverageFrameRate = video.AverageFrameRate
			interlace.RealFrameRate = video.RealBaseFrameRate
			existing.RawProbe["interlaceAnalysis"] = interlace
			existing.InterlaceAnalysis = structToJSONMap(interlace)
		}
		if ctx.Err() != nil {
			return models.ScanResult{}, ctx.Err()
		}
		if cropStale {
			report("crop", 75, "Refreshing Crop from shared pixel evidence")
			crop := detectCropWithSharedPixelEvidenceContext(ctx, path, video.Width, video.Height, pixelPlan, pixelSession)
			existing.RawProbe["cropAnalysis"] = crop
			existing.CropAnalysis = structToJSONMap(crop)
		}
		if ctx.Err() != nil {
			return models.ScanResult{}, ctx.Err()
		}
	}

	refreshCadence := slices.Contains(staleComponents, "cadence") || frameStale || interlaceStale
	if refreshCadence {
		report("cadence", 85, "Recomputing Cadence from current shared evidence")
		declaredRate := firstReliableFrameRate(video.AverageFrameRate, video.RealBaseFrameRate)
		var cadence CadenceAnalysis
		if frameStale && err != nil {
			cadence = analyzeCadence(video.CodecName, declaredRate, interlace)
		} else {
			cadence = analyzeCadence(video.CodecName, declaredRate, interlace, frameAnalysis)
		}
		existing.RawProbe["cadenceAnalysis"] = cadence
		existing.RawProbe["cadenceRecommendation"] = recommendCadence(cadence)
		existing.CadenceAnalysis = structToJSONMap(cadence)
		existing.CadenceRecommendation = structToJSONMap(recommendCadence(cadence))
	}
	existing.FrameStructureAnalysis = analysisMapFromRaw(existing.RawProbe, "frameStructureAnalysis")
	existing.FrameStructureRecommendation = frameStructureRecommendationMap(existing)
	existing.HEVCLevelRecommendation = hevcLevelRecommendationMap(existing)
	refreshed := append([]string(nil), staleComponents...)
	if refreshCadence && !slices.Contains(refreshed, "cadence") {
		refreshed = append(refreshed, "cadence")
	}
	sort.Strings(refreshed)
	reused := componentDifference(snapshotComponentNames, refreshed)
	stampSnapshotCacheMetadataWithRefresh(&existing, path, info, "incremental", reused, refreshed)
	if err := persistFinalAssetSnapshot(h.db, &existing); err != nil {
		return models.ScanResult{}, err
	}
	report("completed", 100, "Incremental snapshot refresh completed")
	return existing, nil
}

func componentDifference(all, excluded []string) []string {
	result := make([]string, 0, len(all))
	for _, component := range all {
		if !slices.Contains(excluded, component) {
			result = append(result, component)
		}
	}
	return result
}

func decodeJSONValue(value any, target any) bool {
	encoded, err := json.Marshal(value)
	return err == nil && json.Unmarshal(encoded, target) == nil
}

func structToJSONMap(value any) models.JSONMap {
	result := models.JSONMap{}
	if !decodeJSONValue(value, &result) {
		return models.JSONMap{}
	}
	return result
}

func inheritedOriginalSnapshot(db *gorm.DB, archivePath string, info os.FileInfo) (models.ScanResult, bool) {
	if db == nil || !db.Migrator().HasTable(&models.QueueJob{}) || !db.Migrator().HasTable(&models.ScanResult{}) {
		return models.ScanResult{}, false
	}
	var job models.QueueJob
	if err := db.Where("original_archived_path = ?", filepath.Clean(archivePath)).Order("published_at desc, id desc").First(&job).Error; err != nil {
		return models.ScanResult{}, false
	}
	var source models.ScanResult
	if err := db.Where("path = ?", filepath.Clean(job.MediaPath)).Order("created_at desc, id desc").First(&source).Error; err != nil {
		return models.ScanResult{}, false
	}
	source.ID = 0
	source.Path = filepath.Clean(archivePath)
	source.FileName = filepath.Base(archivePath)
	if info != nil {
		source.SizeBytes = info.Size()
	}
	source.CreatedAt = time.Time{}
	source.UpdatedAt = time.Time{}
	if source.RawProbe == nil {
		source.RawProbe = models.JSONMap{}
	}
	stampSnapshotCacheMetadataWithRefresh(&source, archivePath, info, "inherited", snapshotComponentNames, nil)
	ensureFrameStructureRecommendation(&source)
	ensureHEVCLevelRecommendation(&source)
	source.RawProbe["snapshotProvenance"] = models.JSONMap{
		"source": "raw_asset", "sourcePath": filepath.Clean(job.MediaPath), "archivePath": filepath.Clean(archivePath), "jobId": job.ID,
	}
	if err := persistFinalAssetSnapshot(db, &source); err != nil {
		return models.ScanResult{}, false
	}
	return source, true
}

func (h ScannerHandler) StartSnapshotOperation(c *gin.Context) {
	assetMutationMu.Lock()
	defer assetMutationMu.Unlock()
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
	if active, maintenanceErr := activeAssetMaintenance(h.db, path); maintenanceErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": maintenanceErr.Error()})
		return
	} else if active {
		c.JSON(http.StatusConflict, gin.H{"error": "asset has an active maintenance operation and cannot be scanned"})
		return
	}

	h.markStaleSnapshotOperations(time.Now())

	snapshotOperations.RLock()
	for _, operation := range snapshotOperations.items {
		if operation.AssetPath == path && operation.Status == "running" {
			copy := snapshotOperationCopy(operation)
			snapshotOperations.RUnlock()
			c.JSON(http.StatusAccepted, copy)
			return
		}
	}
	snapshotOperations.RUnlock()

	now := time.Now()
	id := fmt.Sprintf("snapshot-%d", now.UnixNano())
	ctx, cancel := context.WithCancel(context.Background())
	operation := &SnapshotOperation{ID: id, AssetPath: path, Status: "running", Phase: "preparing", Progress: 1, Message: "Preparing this asset snapshot", StageTimingsMs: map[string]int64{}, CreatedAt: now, UpdatedAt: now, phaseStartedAt: now}
	snapshotOperations.Lock()
	snapshotOperations.items[id] = operation
	snapshotOperations.cancels[id] = cancel
	snapshotOperations.Unlock()
	response := snapshotOperationCopy(operation)

	go func(fileInfo os.FileInfo) {
		defer func() {
			snapshotOperations.Lock()
			delete(snapshotOperations.cancels, id)
			snapshotOperations.Unlock()
		}()
		type scanOutcome struct {
			result models.ScanResult
			cached bool
			err    error
		}
		outcomes := make(chan scanOutcome, 1)
		go func() {
			result, cached, scanErr := h.scanResolvedFileContext(ctx, path, fileInfo, request, func(phase string, progress float64, message string) {
				updateSnapshotOperation(id, func(item *SnapshotOperation) {
					if item.Status != "running" {
						return
					}
					if phase == "incremental_refresh" {
						item.IncrementalRefresh = true
					}
					transitionSnapshotOperationPhase(item, phase, progress, message, time.Now())
				})
			})
			outcomes <- scanOutcome{result: result, cached: cached, err: scanErr}
		}()

		var outcome scanOutcome
		select {
		case outcome = <-outcomes:
		case <-time.After(snapshotOperationTimeout):
			cancel()
			transitioned := finishRunningSnapshotOperation(id, func(item *SnapshotOperation) {
				finishSnapshotOperationTiming(item, time.Now())
				item.Status, item.Phase, item.Error, item.Message = "error", "timeout", "snapshot analysis exceeded 15 minutes", "Asset snapshot timed out; retry after checking FFmpeg and the NAS mount"
			})
			if transitioned {
				h.logSnapshotOperation(id, "snapshot_analysis_timeout", context.DeadlineExceeded)
			}
			return
		}
		if outcome.err != nil {
			transitioned := finishRunningSnapshotOperation(id, func(item *SnapshotOperation) {
				finishSnapshotOperationTiming(item, time.Now())
				item.Status, item.Phase, item.Error, item.Message = "error", "error", outcome.err.Error(), "Asset snapshot failed"
			})
			if transitioned {
				h.logSnapshotOperation(id, "snapshot_analysis_failed", outcome.err)
			}
			return
		}
		transitioned := finishRunningSnapshotOperation(id, func(item *SnapshotOperation) {
			finishSnapshotOperationTiming(item, time.Now())
			item.CacheHit = outcome.cached
			item.ReusedComponents, item.RefreshedComponents, item.ComponentStatuses = snapshotRefreshDetails(outcome.result, outcome.cached)
			item.FallbackReason = snapshotFallbackReason(outcome.result)
			item.Status, item.Phase, item.Progress, item.Message, item.Result = "completed", "completed", 100, "Asset snapshot completed", &outcome.result
		})
		if transitioned {
			h.logSnapshotOperation(id, "snapshot_analysis_completed", nil)
		}
	}(info)
	c.JSON(http.StatusAccepted, response)
}

func (h ScannerHandler) CancelSnapshotOperation(c *gin.Context) {
	id := c.Param("id")
	snapshotOperations.Lock()
	operation := snapshotOperations.items[id]
	if operation == nil {
		snapshotOperations.Unlock()
		c.JSON(http.StatusNotFound, gin.H{"error": "snapshot operation not found"})
		return
	}
	cancelled := false
	if operation.Status == "running" {
		if cancel := snapshotOperations.cancels[id]; cancel != nil {
			cancel()
		}
		delete(snapshotOperations.cancels, id)
		finishSnapshotOperationTiming(operation, time.Now())
		operation.Status, operation.Phase, operation.Message = "cancelled", "cancelled", "Analysis cancelled by user"
		operation.UpdatedAt = time.Now()
		cancelled = true
	}
	copy := snapshotOperationCopy(operation)
	snapshotOperations.Unlock()
	c.JSON(http.StatusOK, copy)
	if cancelled {
		h.logSnapshotOperation(id, "snapshot_analysis_cancelled", nil)
	}
}

func (h ScannerHandler) logSnapshotOperation(id, event string, operationErr error) {
	if h.db == nil {
		return
	}
	snapshotOperations.RLock()
	operation := snapshotOperations.items[id]
	if operation == nil {
		snapshotOperations.RUnlock()
		return
	}
	timings, _ := json.Marshal(operation.StageTimingsMs)
	reused, _ := json.Marshal(operation.ReusedComponents)
	refreshed, _ := json.Marshal(operation.RefreshedComponents)
	statuses, _ := json.Marshal(operation.ComponentStatuses)
	fields := map[string]string{
		"operationId":         id,
		"asset":               operation.AssetPath,
		"phase":               operation.Phase,
		"durationMs":          strconv.FormatInt(operation.DurationMs, 10),
		"cacheHit":            strconv.FormatBool(operation.CacheHit),
		"incrementalRefresh":  strconv.FormatBool(operation.IncrementalRefresh),
		"stageTimings":        string(timings),
		"reusedComponents":    string(reused),
		"refreshedComponents": string(refreshed),
		"componentStatuses":   string(statuses),
		"fallbackReason":      operation.FallbackReason,
	}
	snapshotOperations.RUnlock()
	appendSystemLog(h.db, event, fields, operationErr)
}

func (h ScannerHandler) GetSnapshotOperation(c *gin.Context) {
	h.markStaleSnapshotOperations(time.Now())
	snapshotOperations.RLock()
	operation := snapshotOperations.items[c.Param("id")]
	if operation == nil {
		snapshotOperations.RUnlock()
		c.JSON(http.StatusNotFound, gin.H{"error": "snapshot operation not found"})
		return
	}
	copy := snapshotOperationCopy(operation)
	snapshotOperations.RUnlock()
	c.JSON(http.StatusOK, copy)
}

func (h ScannerHandler) ListSnapshotOperations(c *gin.Context) {
	path := strings.TrimSpace(c.Query("path"))
	if path != "" {
		path = resolveMediaPath(h.db, path)
	}
	h.markStaleSnapshotOperations(time.Now())
	items := []SnapshotOperation{}
	snapshotOperations.RLock()
	for _, operation := range snapshotOperations.items {
		if path == "" || operation.AssetPath == path {
			items = append(items, snapshotOperationCopy(operation))
		}
	}
	snapshotOperations.RUnlock()
	c.JSON(http.StatusOK, gin.H{"operations": items})
}

func (h ScannerHandler) markStaleSnapshotOperations(now time.Time) {
	for _, id := range markStaleSnapshotOperations(now) {
		h.logSnapshotOperation(id, "snapshot_analysis_timeout", context.DeadlineExceeded)
	}
}

func markStaleSnapshotOperations(now time.Time) []string {
	snapshotOperations.Lock()
	defer snapshotOperations.Unlock()
	timedOut := []string{}
	for id, operation := range snapshotOperations.items {
		if operation.Status != "running" || now.Sub(operation.CreatedAt) <= snapshotOperationTimeout {
			continue
		}
		operation.Status = "error"
		finishSnapshotOperationTiming(operation, now)
		operation.Phase = "timeout"
		operation.Error = "snapshot operation stopped reporting before completion"
		operation.Message = "Asset snapshot timed out; retry after checking FFmpeg and the NAS mount"
		operation.UpdatedAt = now
		timedOut = append(timedOut, id)
	}
	cleanupSnapshotOperationsLocked(now)
	return timedOut
}

func cleanupSnapshotOperationsLocked(now time.Time) {
	type terminalOperation struct {
		id        string
		updatedAt time.Time
	}
	terminal := make([]terminalOperation, 0, len(snapshotOperations.items))
	for id, operation := range snapshotOperations.items {
		if operation == nil || operation.Status == "running" {
			continue
		}
		updatedAt := operation.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = operation.CreatedAt
		}
		if !updatedAt.IsZero() && now.Sub(updatedAt) > snapshotOperationRetention {
			delete(snapshotOperations.items, id)
			delete(snapshotOperations.cancels, id)
			continue
		}
		terminal = append(terminal, terminalOperation{id: id, updatedAt: updatedAt})
	}
	if len(terminal) <= snapshotOperationMaxHistory {
		return
	}
	sort.Slice(terminal, func(i, j int) bool { return terminal[i].updatedAt.After(terminal[j].updatedAt) })
	for _, operation := range terminal[snapshotOperationMaxHistory:] {
		delete(snapshotOperations.items, operation.id)
		delete(snapshotOperations.cancels, operation.id)
	}
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
	return runFFProbeWithProgressContext(context.Background(), path, analysisSeconds, progress, samplingPolicy)
}

func runFFProbeWithProgressContext(ctx context.Context, path string, analysisSeconds int, progress func(string, float64, string), samplingPolicy FrameStructureSamplingPolicy) (FFProbeResult, models.JSONMap, error) {
	report := func(phase string, value float64, message string) {
		if progress != nil {
			progress(phase, value, message)
		}
	}
	report("metadata", 10, "Reading media metadata")
	ctxTimeout := 60 * time.Second
	cmd := exec.CommandContext(ctx,
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
	duration := parseFloat(probe.Format.Duration)
	samplingPlan := canonicalSamplingPlan(duration, samplingPolicy)
	report("frame_structure", 30, "Collecting shared frame, cadence and GOP evidence")
	frameContext, cancelFrameAnalysis := context.WithTimeout(ctx, 3*time.Minute)
	frameAnalysis, frameErr := analyzeVideoFrameStructureWithSamplingPlan(frameContext, path, samplingPlan)
	cancelFrameAnalysis()
	if err := ctx.Err(); err != nil {
		return FFProbeResult{}, nil, err
	}
	report("interlace", 60, "Validating shared frame evidence with interlace samples")
	pixelPlan := samplingPlan
	if frameErr == nil && len(frameAnalysis.Positions) > 0 {
		pixelPlan.Positions = append([]float64(nil), frameAnalysis.Positions...)
		pixelPlan.Adaptive = false
		pixelPlan.InitialWindows = len(pixelPlan.Positions)
	}
	pixelSession := newPixelSamplingSession(path, video.Width, video.Height, pixelPlan)
	interlaceAnalysis := detectInterlaceWithSharedEvidenceContext(ctx, path, video.FieldOrder, analysisSeconds, false, pixelPlan, frameAnalysis, pixelSession)
	interlaceAnalysis.Codec = video.CodecName
	interlaceAnalysis.AverageFrameRate = video.AverageFrameRate
	interlaceAnalysis.RealFrameRate = video.RealBaseFrameRate
	raw["interlaceAnalysis"] = interlaceAnalysis
	if err := ctx.Err(); err != nil {
		return FFProbeResult{}, nil, err
	}
	report("crop", 80, "Checking shared sample regions for crop boundaries")
	raw["cropAnalysis"] = detectCropWithSharedPixelEvidenceContext(ctx, path, video.Width, video.Height, pixelPlan, pixelSession)
	if err := ctx.Err(); err != nil {
		return FFProbeResult{}, nil, err
	}
	if frameErr == nil {
		encoded, _ := json.Marshal(frameAnalysis)
		frameMap := models.JSONMap{}
		_ = json.Unmarshal(encoded, &frameMap)
		raw["frameStructureAnalysis"] = frameMap
		cadence := analyzeCadence(video.CodecName, firstReliableFrameRate(video.AverageFrameRate, video.RealBaseFrameRate), interlaceAnalysis, frameAnalysis)
		raw["cadenceAnalysis"] = cadence
		raw["cadenceRecommendation"] = recommendCadence(cadence)
	} else {
		raw["frameStructureAnalysis"] = models.JSONMap{"version": 1, "status": "unverified", "error": frameErr.Error()}
		cadence := analyzeCadence(video.CodecName, firstReliableFrameRate(video.AverageFrameRate, video.RealBaseFrameRate), interlaceAnalysis)
		raw["cadenceAnalysis"] = cadence
		raw["cadenceRecommendation"] = recommendCadence(cadence)
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
		CadenceAnalysis:        analysisMapFromRaw(raw, "cadenceAnalysis"),
		CadenceRecommendation:  analysisMapFromRaw(raw, "cadenceRecommendation"),
		CropAnalysis:           analysisMapFromRaw(raw, "cropAnalysis"),
		FrameStructureAnalysis: analysisMapFromRaw(raw, "frameStructureAnalysis"),
	}
	result.CompatibilityAnalysis = buildPlaybackCompatibilityAnalysis(result)
	result.FrameStructureRecommendation = frameStructureRecommendationMap(result)
	result.HEVCLevelRecommendation = hevcLevelRecommendationMap(result)
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
	stampSnapshotCacheMetadata(&result, path, info)
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
			summary["level"] = stream.Level
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
