package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSnapshotOperationUsesOnlySelectedCachedAsset(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:snapshot-operation-single-asset?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	selectedPath := filepath.Join(t.TempDir(), "selected.mkv")
	if err := os.WriteFile(selectedPath, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(selectedPath)
	selected := models.ScanResult{
		Path: selectedPath, FileName: filepath.Base(selectedPath), SizeBytes: info.Size(), CreatedAt: time.Now(),
		RawProbe: models.JSONMap{}, FrameStructureAnalysis: models.JSONMap{"version": 1, "framesAnalyzed": 500},
	}
	other := models.ScanResult{Path: filepath.Join(t.TempDir(), "other.mkv"), FileName: "other.mkv", SizeBytes: 99, CreatedAt: time.Now()}
	if err := db.Create(&selected).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}

	snapshotOperations.Lock()
	snapshotOperations.items = map[string]*SnapshotOperation{}
	snapshotOperations.Unlock()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/scan/operations", NewScannerHandler(db).StartSnapshotOperation)
	body, _ := json.Marshal(ScanRequest{Path: selectedPath})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/scan/operations", bytes.NewReader(body)))
	if response.Code != http.StatusAccepted {
		t.Fatalf("start status=%d body=%s", response.Code, response.Body.String())
	}
	var operation SnapshotOperation
	if err := json.Unmarshal(response.Body.Bytes(), &operation); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snapshotOperations.RLock()
		current := snapshotOperations.items[operation.ID]
		if current != nil {
			operation = *current
		}
		snapshotOperations.RUnlock()
		if operation.Status != "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if operation.Status != "completed" || operation.Result == nil || operation.Result.Path != selectedPath {
		t.Fatalf("unexpected operation: %#v", operation)
	}
	var untouched models.ScanResult
	if err := db.Where("path = ?", other.Path).First(&untouched).Error; err != nil || untouched.SizeBytes != 99 {
		t.Fatalf("unselected asset changed: %#v err=%v", untouched, err)
	}
}

func TestMarkStaleSnapshotOperationAllowsRetry(t *testing.T) {
	path := "/media/raw/anime/DBZ/episode14.mkv"
	createdAt := time.Now().Add(-snapshotOperationTimeout - time.Minute)
	snapshotOperations.Lock()
	snapshotOperations.items = map[string]*SnapshotOperation{
		"dbz-stuck": {ID: "dbz-stuck", AssetPath: path, Status: "running", Phase: "frame_structure", Progress: 80, CreatedAt: createdAt, UpdatedAt: createdAt},
	}
	snapshotOperations.Unlock()

	markStaleSnapshotOperations(time.Now())
	snapshotOperations.RLock()
	operation := *snapshotOperations.items["dbz-stuck"]
	snapshotOperations.RUnlock()
	if operation.Status != "error" || operation.Phase != "timeout" || operation.Error == "" {
		t.Fatalf("stale operation was not released: %#v", operation)
	}
}

func TestCancelSnapshotOperationPausesRunningAnalysis(t *testing.T) {
	gin.SetMode(gin.TestMode)
	canceled := false
	snapshotOperations.Lock()
	snapshotOperations.items = map[string]*SnapshotOperation{
		"cancel-me": {ID: "cancel-me", AssetPath: "/media/raw/example.mkv", Status: "running", Phase: "crop"},
	}
	snapshotOperations.cancels = map[string]context.CancelFunc{
		"cancel-me": func() { canceled = true },
	}
	snapshotOperations.Unlock()

	router := gin.New()
	router.POST("/api/scan/operations/:id/cancel", NewScannerHandler(nil).CancelSnapshotOperation)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/scan/operations/cancel-me/cancel", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !canceled {
		t.Fatal("operation context was not canceled")
	}
	snapshotOperations.RLock()
	operation := *snapshotOperations.items["cancel-me"]
	_, cancelRetained := snapshotOperations.cancels["cancel-me"]
	snapshotOperations.RUnlock()
	if operation.Status != "paused" || operation.Phase != "paused" || cancelRetained {
		t.Fatalf("operation was not paused cleanly: %#v cancelRetained=%v", operation, cancelRetained)
	}
}

func TestIsHDRDoesNotTreatTenBitSDRAsHDR(t *testing.T) {
	stream := FFProbeStream{
		Profile: "Main 10", PixFmt: "yuv420p10le",
		ColorTransfer: "smpte170m", ColorPrimaries: "smpte170m",
	}
	if isHDR(stream) {
		t.Fatal("10-bit SDR must not be classified as HDR")
	}
}

func TestPersistFinalAssetSnapshotReplacesStalePathAtomically(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:final-asset-snapshot?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	path := "/media/library/anime/Akira (1988)/Akira (1988).mkv"
	if err := db.Create(&models.ScanResult{Path: path, FileName: "Akira (1988).mkv", VideoCodec: "mpeg2video", SizeBytes: 10}).Error; err != nil {
		t.Fatal(err)
	}
	final := models.ScanResult{Path: path, FileName: "Akira (1988).mkv", VideoCodec: "hevc", SizeBytes: 20, Width: 720, Height: 460}
	if err := persistFinalAssetSnapshot(db, &final); err != nil {
		t.Fatal(err)
	}
	var scans []models.ScanResult
	if err := db.Where("path = ?", path).Find(&scans).Error; err != nil {
		t.Fatal(err)
	}
	if len(scans) != 1 || scans[0].VideoCodec != "hevc" || scans[0].SizeBytes != 20 || scans[0].Height != 460 {
		t.Fatalf("unexpected final snapshot: %#v", scans)
	}
}

func TestIsHDRDetectsPQAndHDRMetadata(t *testing.T) {
	if !isHDR(FFProbeStream{ColorTransfer: "smpte2084", ColorPrimaries: "bt2020"}) {
		t.Fatal("PQ transfer must be classified as HDR")
	}
	if !isHDR(FFProbeStream{SideDataList: []map[string]any{{"side_data_type": "Mastering display metadata"}}}) {
		t.Fatal("mastering display metadata must be classified as HDR")
	}
}

func TestStreamSizeBytesUsesMatroskaStatistics(t *testing.T) {
	stream := FFProbeStream{Tags: map[string]string{"NUMBER_OF_BYTES-eng": "46764973", "BPS-eng": "252532"}}
	if got := streamSizeBytes(stream); got != 46764973 {
		t.Fatalf("size=%d", got)
	}
	if got := streamBitrate(stream); got != 252532 {
		t.Fatalf("bitrate=%d", got)
	}
}

func TestStreamStatisticsIgnoreInheritedMakeMKVValuesAfterEncode(t *testing.T) {
	stream := FFProbeStream{Tags: map[string]string{
		"ENCODER":                          "Lavc62.28.101 hevc_videotoolbox",
		"BPS-eng":                          "5494883",
		"NUMBER_OF_BYTES-eng":              "4913281376",
		"_STATISTICS_WRITING_APP-eng":      "MakeMKV v1.18.3 darwin(x64-release)",
		"_STATISTICS_WRITING_DATE_UTC-eng": "2026-06-23 19:47:26",
	}}
	if got := streamSizeBytes(stream); got != 0 {
		t.Fatalf("stale size=%d want=0", got)
	}
	if got := streamBitrate(stream); got != 0 {
		t.Fatalf("stale bitrate=%d want=0", got)
	}
}

func TestScanResolvedFileReusesExistingSnapshotUntilForced(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:snapshot-reuse-until-forced?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(t.TempDir(), "episode.mkv")
	if err := os.WriteFile(mediaPath, []byte("replacement with different size"), 0o644); err != nil {
		t.Fatal(err)
	}
	existing := models.ScanResult{
		Path: mediaPath, FileName: "episode.mkv", SizeBytes: 3, VideoCodec: "cached-codec", CreatedAt: time.Now().Add(-time.Hour),
		VideoStreams:           models.JSONList{map[string]any{"avgFrameRate": "25/1"}},
		FrameStructureAnalysis: models.JSONMap{"version": 2, "framesAnalyzed": 500, "averageGopLength": 75.0, "maxConsecutiveBFrames": 3, "confidence": "medium"},
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(mediaPath)
	result, cached, err := NewScannerHandler(db).scanResolvedFile(mediaPath, info, ScanRequest{Force: false}, nil)
	if err != nil || !cached || result.VideoCodec != "cached-codec" {
		t.Fatalf("snapshot was regenerated without Re-scan: cached=%t result=%#v err=%v", cached, result, err)
	}
	if workerIntValue(result.FrameStructureRecommendation["version"], 0) != 1 {
		t.Fatalf("cached snapshot was not enriched from stored source facts: %#v", result.FrameStructureRecommendation)
	}
	var stored models.ScanResult
	if err := db.First(&stored, existing.ID).Error; err != nil || workerIntValue(stored.FrameStructureRecommendation["version"], 0) != 1 {
		t.Fatalf("enriched recommendation was not persisted: %#v err=%v", stored.FrameStructureRecommendation, err)
	}
}

func TestScanResolvedFileRefreshesLegacySnapshotWithoutFrameStructure(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open("file:legacy-snapshot-frame-refresh?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.AutoMigrate(&models.ScanResult{}); err != nil {
		t.Fatal(err)
	}

	mediaPath := filepath.Join(t.TempDir(), "legacy-episode.mkv")

	// This does not need to be valid media.
	//
	// If the legacy snapshot is incorrectly reused, scanResolvedFile()
	// will return it immediately.
	//
	// If the snapshot is correctly considered incomplete, the scanner
	// will continue into FFprobe and this fake file will fail analysis.
	if err := os.WriteFile(mediaPath, []byte("not real media"), 0o644); err != nil {
		t.Fatal(err)
	}

	existing := models.ScanResult{
		Path:       mediaPath,
		FileName:   "legacy-episode.mkv",
		SizeBytes:  14,
		VideoCodec: "h264",
		Width:      1920,
		Height:     1080,
		VideoStreams: models.JSONList{
			map[string]any{
				"codec":        "h264",
				"avgFrameRate": "25/1",
			},
		},

		// Simulates a snapshot generated before Frame Structure
		// analysis was persisted.
		FrameStructureAnalysis: models.JSONMap{},

		CreatedAt: time.Now().Add(-24 * time.Hour),
	}

	if err := db.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(mediaPath)
	if err != nil {
		t.Fatal(err)
	}

	var phases []string

	result, cached, scanErr := NewScannerHandler(db).scanResolvedFile(
		mediaPath,
		info,
		ScanRequest{Force: false},
		func(phase string, progress float64, message string) {
			phases = append(phases, phase)
		},
	)

	if cached {
		t.Fatalf(
			"legacy snapshot without frame structure was incorrectly reused: %#v",
			result,
		)
	}

	foundRefresh := false
	for _, phase := range phases {
		if phase == "snapshot_refresh" {
			foundRefresh = true
			break
		}
	}

	if !foundRefresh {
		t.Fatalf(
			"legacy snapshot did not trigger snapshot refresh; phases=%v",
			phases,
		)
	}

	// The fixture is intentionally not valid media.
	// Reaching FFprobe and failing proves that the stale cache was bypassed.
	if scanErr == nil {
		t.Fatal("expected fake media analysis to fail after bypassing stale snapshot")
	}
}

func TestArchivedOriginalInheritsRawSnapshot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:archive-inherits-raw-snapshot?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ScanResult{}, &models.QueueJob{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	rawPath := filepath.Join(root, "raw", "episode.mkv")
	archivePath := filepath.Join(root, "archive", "episode.mkv")
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archivePath, []byte("original bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := models.ScanResult{Path: rawPath, FileName: "episode.mkv", SizeBytes: 14, VideoCodec: "h264", FrameStructureAnalysis: models.JSONMap{"version": 2, "framesAnalyzed": 900}}
	if err := db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.QueueJob{MediaPath: rawPath, OriginalArchivedPath: archivePath, Status: JobStatusCompleted}).Error; err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(archivePath)
	result, ok := inheritedOriginalSnapshot(db, archivePath, info)
	if !ok || result.Path != archivePath || result.VideoCodec != "h264" || jsonMapInt(result.FrameStructureAnalysis, "framesAnalyzed") != 900 {
		t.Fatalf("archive did not inherit Raw snapshot: ok=%t result=%#v", ok, result)
	}
	if workerIntValue(result.FrameStructureRecommendation["version"], 0) != 1 {
		t.Fatalf("archive did not inherit the derived frame recommendation: %#v", result.FrameStructureRecommendation)
	}
}

func TestSnapshotRequiresFrameStructureRefreshForLegacyVideo(t *testing.T) {
	legacy := models.ScanResult{
		Path:       "/media/raw/anime/legacy.mkv",
		FileName:   "legacy.mkv",
		VideoCodec: "h264",
		Width:      1920,
		Height:     1080,
		VideoStreams: models.JSONList{
			map[string]any{
				"codec": "h264",
			},
		},
		FrameStructureAnalysis: models.JSONMap{},
	}

	if !snapshotRequiresFrameStructureRefresh(legacy) {
		t.Fatal("legacy video snapshot without frame structure analysis must be refreshed")
	}
}

func TestSnapshotDoesNotRequireFrameStructureRefreshWhenAnalysisExists(t *testing.T) {
	current := models.ScanResult{
		Path:       "/media/raw/anime/current.mkv",
		FileName:   "current.mkv",
		VideoCodec: "h264",
		Width:      1920,
		Height:     1080,
		VideoStreams: models.JSONList{
			map[string]any{
				"codec":        "h264",
				"avgFrameRate": "25/1",
			},
		},
		FrameStructureAnalysis: models.JSONMap{
			"version":        2,
			"framesAnalyzed": 500,
		},
	}

	if snapshotRequiresFrameStructureRefresh(current) {
		t.Fatal("snapshot with frame structure analysis must remain cacheable")
	}
}

func TestSnapshotWithoutVideoDoesNotRequireFrameStructureRefresh(t *testing.T) {
	audioOnly := models.ScanResult{
		Path:                   "/media/raw/music/track.flac",
		FileName:               "track.flac",
		AudioTracks:            1,
		VideoStreams:           models.JSONList{},
		FrameStructureAnalysis: models.JSONMap{},
	}

	if snapshotRequiresFrameStructureRefresh(audioOnly) {
		t.Fatal("audio-only snapshot must not require frame structure analysis")
	}
}

func TestSnapshotRequiresFrameStructureRefreshWhenAnalysisIsUnverified(t *testing.T) {
	snapshot := models.ScanResult{
		Path:       "/media/raw/anime/unverified.mkv",
		FileName:   "unverified.mkv",
		VideoCodec: "h264",
		Width:      1920,
		Height:     1080,
		VideoStreams: models.JSONList{
			map[string]any{
				"codec": "h264",
			},
		},
		FrameStructureAnalysis: models.JSONMap{
			"version": 1,
			"status":  "unverified",
			"error":   "frame structure analysis failed",
		},
	}

	if !snapshotRequiresFrameStructureRefresh(snapshot) {
		t.Fatal("video snapshot with unverified frame structure analysis must be refreshed")
	}
}

func TestSnapshotRequiresFrameStructureRefreshWhenFrameRateMissing(t *testing.T) {
	snapshot := models.ScanResult{
		Path:       "/media/raw/anime/missing-fps.mkv",
		FileName:   "missing-fps.mkv",
		VideoCodec: "h264",
		Width:      1920,
		Height:     1080,
		VideoStreams: models.JSONList{
			map[string]any{
				"codec": "h264",
			},
		},
		FrameStructureAnalysis: models.JSONMap{
			"version":               2,
			"framesAnalyzed":        500,
			"averageGopLength":      72.0,
			"maxConsecutiveBFrames": 3,
			"confidence":            "medium",
		},
	}

	if !snapshotRequiresFrameStructureRefresh(snapshot) {
		t.Fatal("video snapshot without a reliable frame rate must be refreshed")
	}
}

func TestSnapshotDoesNotRequireFrameStructureRefreshWithRealFrameRate(t *testing.T) {
	snapshot := models.ScanResult{
		Path:       "/media/raw/anime/real-fps.mkv",
		FileName:   "real-fps.mkv",
		VideoCodec: "h264",
		Width:      1920,
		Height:     1080,
		VideoStreams: models.JSONList{
			map[string]any{
				"codec":         "h264",
				"realFrameRate": "24000/1001",
			},
		},
		FrameStructureAnalysis: models.JSONMap{
			"version":               2,
			"framesAnalyzed":        500,
			"averageGopLength":      72.0,
			"maxConsecutiveBFrames": 3,
			"confidence":            "medium",
		},
	}

	if snapshotRequiresFrameStructureRefresh(snapshot) {
		t.Fatal("snapshot with a valid realFrameRate must remain cacheable")
	}
}
