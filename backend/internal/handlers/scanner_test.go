package handlers

import (
	"bytes"
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

func TestScanCacheInvalidatesWhenMediaWasReplaced(t *testing.T) {
	path := filepath.Join(t.TempDir(), "episode.mkv")
	if err := os.WriteFile(path, []byte("new converted asset"), 0o644); err != nil {
		t.Fatal(err)
	}
	modifiedAt := time.Now().Add(-time.Minute)
	if err := os.Chtimes(path, modifiedAt, modifiedAt); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	current := models.ScanResult{SizeBytes: info.Size(), CreatedAt: modifiedAt.Add(time.Second)}
	if !scanCacheMatchesFile(current, info) {
		t.Fatal("matching size and older media mtime should reuse the scan")
	}
	if scanCacheMatchesFile(models.ScanResult{SizeBytes: info.Size() + 1, CreatedAt: time.Now()}, info) {
		t.Fatal("changed media size must invalidate the scan")
	}
	if scanCacheMatchesFile(models.ScanResult{SizeBytes: info.Size(), CreatedAt: modifiedAt.Add(-time.Second)}, info) {
		t.Fatal("media modified after the scan must invalidate the scan")
	}
}
