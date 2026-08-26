package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	stampSnapshotCacheMetadata(&selected, selectedPath, info)
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
	if !operation.CacheHit || operation.DurationMs < 0 || operation.StageTimingsMs == nil {
		t.Fatalf("cached operation observability missing: %#v", operation)
	}
	var untouched models.ScanResult
	if err := db.Where("path = ?", other.Path).First(&untouched).Error; err != nil || untouched.SizeBytes != 99 {
		t.Fatalf("unselected asset changed: %#v err=%v", untouched, err)
	}
}

func TestSnapshotOperationEnforcesConfiguredAssetConcurrency(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:snapshot-operation-concurrency?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ScanResult{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: analysisPolicySettingKey, Value: models.JSONMap{"mode": "balanced", "concurrentAssets": 1}}).Error; err != nil {
		t.Fatal(err)
	}
	selectedPath := filepath.Join(t.TempDir(), "selected.mkv")
	if err := os.WriteFile(selectedPath, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	snapshotOperations.Lock()
	snapshotOperations.items = map[string]*SnapshotOperation{"active": {ID: "active", AssetPath: "/fixture/other.mkv", Status: "running", CreatedAt: now, UpdatedAt: now, phaseStartedAt: now}}
	snapshotOperations.Unlock()
	t.Cleanup(func() {
		snapshotOperations.Lock()
		snapshotOperations.items = map[string]*SnapshotOperation{}
		snapshotOperations.Unlock()
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/scan/operations", NewScannerHandler(db).StartSnapshotOperation)
	body, _ := json.Marshal(ScanRequest{Path: selectedPath})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/scan/operations", bytes.NewReader(body)))
	if response.Code != http.StatusTooManyRequests || !strings.Contains(response.Body.String(), "concurrency limit reached") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestSnapshotCacheInvalidatesComponentsByAnalysisPolicySemantics(t *testing.T) {
	mediaPath := filepath.Join(t.TempDir(), "policy-cache.mkv")
	if err := os.WriteFile(mediaPath, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(mediaPath)
	if err != nil {
		t.Fatal(err)
	}
	balanced := balancedAnalysisPolicy()
	snapshot := models.ScanResult{Path: mediaPath, RawProbe: models.JSONMap{}, VideoCodec: "h264", Duration: 1200, FrameStructureAnalysis: models.JSONMap{"version": 2}, InterlaceAnalysis: models.JSONMap{"version": interlaceAnalysisCacheVersion}, CropAnalysis: models.JSONMap{"version": 3}, CadenceAnalysis: models.JSONMap{"version": cadenceAnalysisVersion}}
	stampSnapshotCacheMetadataWithPolicy(&snapshot, mediaPath, info, balanced)

	tests := []struct {
		name   string
		policy AnalysisPolicy
		stale  []string
	}{
		{name: "same effective policy", policy: balanced, stale: nil},
		{name: "thorough invalidates dependent evidence", policy: analysisPolicyPreset("thorough"), stale: []string{"cadence", "crop", "frameStructure", "interlace"}},
		{name: "crop depth only", policy: func() AnalysisPolicy {
			value := balanced
			value.Mode = "custom"
			value.CropDepth = "full"
			return value
		}(), stale: []string{"crop"}},
		{name: "interlace validation only", policy: func() AnalysisPolicy {
			value := balanced
			value.Mode = "custom"
			value.InterlaceValidation = "always"
			return value
		}(), stale: []string{"cadence", "interlace"}},
		{name: "execution controls only", policy: func() AnalysisPolicy {
			value := balanced
			value.ReuseSnapshots = false
			value.IncrementalRefresh = false
			value.ConcurrentAssets = 4
			return value
		}(), stale: nil},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			matches, legacy, stale := snapshotCacheStateWithPolicy(snapshot, mediaPath, info, testCase.policy)
			if !matches || legacy || !slices.Equal(stale, testCase.stale) {
				t.Fatalf("matches=%t legacy=%t stale=%v want=%v", matches, legacy, stale, testCase.stale)
			}
		})
	}
}

func TestSnapshotCacheInvalidatesInterlaceWhenMotionSampleChanges(t *testing.T) {
	mediaPath := filepath.Join(t.TempDir(), "motion-cache.mkv")
	if err := os.WriteFile(mediaPath, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(mediaPath)
	policy := balancedAnalysisPolicy()
	snapshot := models.ScanResult{Path: mediaPath, RawProbe: models.JSONMap{}, VideoCodec: "h264", Duration: 1200, FrameStructureAnalysis: models.JSONMap{"version": 2}, InterlaceAnalysis: models.JSONMap{"version": interlaceAnalysisCacheVersion}, CropAnalysis: models.JSONMap{"version": 3}, CadenceAnalysis: models.JSONMap{"version": cadenceAnalysisVersion}}
	stampSnapshotCacheMetadataWithRefreshReasonAndEvidence(&snapshot, mediaPath, info, "full", nil, snapshotComponentNames, "", policy, 10)

	if matches, legacy, stale := snapshotCacheStateWithEvidence(snapshot, mediaPath, info, policy, 10); !matches || legacy || len(stale) != 0 {
		t.Fatalf("same motion sample should remain valid: matches=%t legacy=%t stale=%v", matches, legacy, stale)
	}
	if matches, legacy, stale := snapshotCacheStateWithEvidence(snapshot, mediaPath, info, policy, 20); !matches || legacy || !slices.Equal(stale, []string{"cadence", "interlace"}) {
		t.Fatalf("changed motion sample invalidation mismatch: matches=%t legacy=%t stale=%v", matches, legacy, stale)
	}
}

func TestMotionSampleChangeRefreshesOnlyInterlaceAndCadence(t *testing.T) {
	binDir := t.TempDir()
	pixelScript := `#!/bin/sh
printf '%s\n' 'Repeated Fields: Neither: 100 Top: 0 Bottom: 0' 'Multi frame detection: TFF: 0 BFF: 0 Progressive: 100 Undetermined: 0' 'crop=700:400:10:40' >&2
`
	if err := os.WriteFile(filepath.Join(binDir, "ffmpeg"), []byte(pixelScript), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "ffprobe"), []byte("#!/bin/sh\nprintf 'unexpected frame probe' >&2\nexit 19\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	db, err := gorm.Open(sqlite.Open("file:motion-sample-incremental?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(t.TempDir(), "episode.mkv")
	if err := os.WriteFile(mediaPath, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(mediaPath)
	snapshot := completeAnalysisSnapshot(mediaPath, info)
	stampSnapshotCacheMetadataWithRefreshReasonAndEvidence(&snapshot, mediaPath, info, "full", nil, snapshotComponentNames, "", balancedAnalysisPolicy(), 10)
	if err := db.Create(&snapshot).Error; err != nil {
		t.Fatal(err)
	}
	var phases []string
	result, cached, err := NewScannerHandler(db).scanResolvedFile(mediaPath, info, ScanRequest{AnalysisSeconds: 20}, func(phase string, _ float64, _ string) { phases = append(phases, phase) })
	if err != nil || cached {
		t.Fatalf("motion refresh failed: cached=%t err=%v phases=%v", cached, err, phases)
	}
	reused, refreshed, _ := snapshotRefreshDetails(result, false)
	if slices.Contains(phases, "metadata") || slices.Contains(phases, "frame_structure") || slices.Contains(phases, "crop") || !slices.Contains(phases, "interlace") || !slices.Equal(refreshed, []string{"cadence", "interlace"}) || !slices.Equal(reused, []string{"metadata", "crop", "frameStructure"}) {
		t.Fatalf("unexpected motion refresh: phases=%v reused=%v refreshed=%v", phases, reused, refreshed)
	}
}

func TestBalancedSnapshotRefreshesEndToEndUnderThoroughPolicy(t *testing.T) {
	binDir := t.TempDir()
	probeScript := `#!/bin/sh
printf '%s' '{"frames":[
{"pict_type":"I","key_frame":1,"best_effort_timestamp_time":"80.00","pkt_duration_time":"0.04","interlaced_frame":0,"repeat_pict":0},
{"pict_type":"I","key_frame":1,"best_effort_timestamp_time":"270.00","pkt_duration_time":"0.04","interlaced_frame":0,"repeat_pict":0},
{"pict_type":"I","key_frame":1,"best_effort_timestamp_time":"500.00","pkt_duration_time":"0.04","interlaced_frame":0,"repeat_pict":0},
{"pict_type":"I","key_frame":1,"best_effort_timestamp_time":"730.00","pkt_duration_time":"0.04","interlaced_frame":0,"repeat_pict":0},
{"pict_type":"I","key_frame":1,"best_effort_timestamp_time":"920.00","pkt_duration_time":"0.04","interlaced_frame":0,"repeat_pict":0}
]}'
`
	pixelScript := `#!/bin/sh
printf '%s\n' 'Repeated Fields: Neither: 100 Top: 0 Bottom: 0' 'Multi frame detection: TFF: 0 BFF: 0 Progressive: 100 Undetermined: 0' 'crop=700:400:10:40' >&2
`
	if err := os.WriteFile(filepath.Join(binDir, "ffprobe"), []byte(probeScript), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "ffmpeg"), []byte(pixelScript), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	db, err := gorm.Open(sqlite.Open("file:balanced-to-thorough-e2e?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ScanResult{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(t.TempDir(), "episode.mkv")
	if err := os.WriteFile(mediaPath, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(mediaPath)
	snapshot := completeAnalysisSnapshot(mediaPath, info)
	stampSnapshotCacheMetadataWithPolicy(&snapshot, mediaPath, info, balancedAnalysisPolicy())
	if err := db.Create(&snapshot).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: analysisPolicySettingKey, Value: models.JSONMap{"mode": "thorough", "reuseSnapshots": true, "incrementalRefresh": true, "concurrentAssets": 1}}).Error; err != nil {
		t.Fatal(err)
	}
	var phases []string
	result, cached, err := NewScannerHandler(db).scanResolvedFile(mediaPath, info, ScanRequest{AnalysisSeconds: 20}, func(phase string, _ float64, _ string) { phases = append(phases, phase) })
	if err != nil || cached {
		t.Fatalf("Balanced to Thorough refresh failed: cached=%t err=%v phases=%v", cached, err, phases)
	}
	reused, refreshed, _ := snapshotRefreshDetails(result, false)
	if !slices.Equal(refreshed, []string{"cadence", "crop", "frameStructure", "interlace"}) || !slices.Equal(reused, []string{"metadata"}) || slices.Contains(phases, "metadata") || !slices.Contains(phases, "incremental_refresh") {
		t.Fatalf("unexpected Thorough refresh: phases=%v reused=%v refreshed=%v", phases, reused, refreshed)
	}
}

func completeAnalysisSnapshot(path string, info os.FileInfo) models.ScanResult {
	interlace := models.JSONMap{"version": interlaceAnalysisCacheVersion, "status": "progressive", "confidence": 0.99}
	crop := models.JSONMap{"version": 3, "status": "none", "reason": "cached-crop"}
	frame := models.JSONMap{"version": 2, "status": "valid", "source": "cached-frame", "framesAnalyzed": 100, "confidenceScore": 0.99}
	cadence := models.JSONMap{"version": cadenceAnalysisVersion, "status": "native", "recommendedAction": "preserve"}
	return models.ScanResult{Path: path, FileName: filepath.Base(path), SizeBytes: info.Size(), Duration: 1000, VideoCodec: "h264", Width: 720, Height: 480, VideoStreams: models.JSONList{map[string]any{"codec": "h264", "avgFrameRate": "25/1"}}, InterlaceAnalysis: interlace, CropAnalysis: crop, FrameStructureAnalysis: frame, CadenceAnalysis: cadence, RawProbe: models.JSONMap{"format": map[string]any{"duration": "1000"}, "streams": []any{map[string]any{"codec_type": "video", "codec_name": "h264", "avg_frame_rate": "25/1", "width": 720, "height": 480}}, "interlaceAnalysis": interlace, "cropAnalysis": crop, "frameStructureAnalysis": frame, "cadenceAnalysis": cadence}}
}

func TestSnapshotOperationRecordsStageTimings(t *testing.T) {
	startedAt := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	operation := &SnapshotOperation{
		Phase:          "preparing",
		CreatedAt:      startedAt,
		phaseStartedAt: startedAt,
	}

	transitionSnapshotOperationPhase(operation, "metadata", 10, "Reading metadata", startedAt.Add(125*time.Millisecond))
	transitionSnapshotOperationPhase(operation, "crop", 55, "Checking crop", startedAt.Add(375*time.Millisecond))
	finishSnapshotOperationTiming(operation, startedAt.Add(900*time.Millisecond))

	if operation.StageTimingsMs["preparing"] != 125 || operation.StageTimingsMs["metadata"] != 250 || operation.StageTimingsMs["crop"] != 525 {
		t.Fatalf("unexpected stage timings: %#v", operation.StageTimingsMs)
	}
	if operation.DurationMs != 900 {
		t.Fatalf("duration=%d want=900", operation.DurationMs)
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

	timedOut := markStaleSnapshotOperations(time.Now())
	snapshotOperations.RLock()
	operation := *snapshotOperations.items["dbz-stuck"]
	snapshotOperations.RUnlock()
	if operation.Status != "error" || operation.Phase != "timeout" || operation.Error == "" || len(timedOut) != 1 || timedOut[0] != "dbz-stuck" {
		t.Fatalf("stale operation was not released: %#v", operation)
	}
	if duplicate := markStaleSnapshotOperations(time.Now().Add(time.Second)); len(duplicate) != 0 {
		t.Fatalf("stale timeout produced a duplicate terminal event: %v", duplicate)
	}
	if finishRunningSnapshotOperation("dbz-stuck", func(item *SnapshotOperation) { item.Status = "error" }) {
		t.Fatal("worker timeout path overwrote stale-cleanup terminal outcome")
	}
}

func TestWorkerTimeoutTransitionPreventsStaleCleanupDuplicate(t *testing.T) {
	now := time.Now().Add(-snapshotOperationTimeout - time.Minute)
	snapshotOperations.Lock()
	snapshotOperations.items = map[string]*SnapshotOperation{
		"worker-timeout": {ID: "worker-timeout", Status: "running", Phase: "frame_structure", CreatedAt: now, UpdatedAt: now, phaseStartedAt: now},
	}
	snapshotOperations.cancels = map[string]context.CancelFunc{}
	snapshotOperations.Unlock()
	if !finishRunningSnapshotOperation("worker-timeout", func(operation *SnapshotOperation) {
		finishSnapshotOperationTiming(operation, time.Now())
		operation.Status, operation.Phase, operation.Error = "error", "timeout", "snapshot analysis exceeded 15 minutes"
	}) {
		t.Fatal("worker timeout transition was rejected")
	}
	if duplicate := markStaleSnapshotOperations(time.Now()); len(duplicate) != 0 {
		t.Fatalf("stale cleanup duplicated worker timeout event: %v", duplicate)
	}
	snapshotOperations.RLock()
	operation := *snapshotOperations.items["worker-timeout"]
	snapshotOperations.RUnlock()
	if operation.Status != "error" || operation.Phase != "timeout" || !operation.UpdatedAt.After(now) {
		t.Fatalf("worker timeout terminal state is invalid: %#v", operation)
	}
}

func TestCancelSnapshotOperationTerminatesRunningAnalysis(t *testing.T) {
	gin.SetMode(gin.TestMode)
	canceled := false
	snapshotOperations.Lock()
	snapshotOperations.items = map[string]*SnapshotOperation{
		"cancel-me": {ID: "cancel-me", AssetPath: "/media/raw/example.mkv", Status: "running", Phase: "crop", CreatedAt: time.Now(), phaseStartedAt: time.Now()},
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
	if operation.Status != "cancelled" || operation.Phase != "cancelled" || cancelRetained {
		t.Fatalf("operation was not cancelled cleanly: %#v cancelRetained=%v", operation, cancelRetained)
	}
}

func TestSnapshotOperationAcceptsOnlyOneTerminalTransition(t *testing.T) {
	now := time.Now().Add(-time.Hour)
	snapshotOperations.Lock()
	snapshotOperations.items = map[string]*SnapshotOperation{
		"race": {ID: "race", Status: "running", Phase: "frame_structure", CreatedAt: now, UpdatedAt: now, phaseStartedAt: now},
	}
	snapshotOperations.cancels = map[string]context.CancelFunc{}
	snapshotOperations.Unlock()
	if !finishRunningSnapshotOperation("race", func(operation *SnapshotOperation) {
		operation.Status, operation.Phase = "cancelled", "cancelled"
	}) {
		t.Fatal("first terminal transition was rejected")
	}
	snapshotOperations.RLock()
	terminalUpdatedAt := snapshotOperations.items["race"].UpdatedAt
	snapshotOperations.RUnlock()
	if !terminalUpdatedAt.After(now) {
		t.Fatalf("terminal transition did not advance UpdatedAt: before=%v after=%v", now, terminalUpdatedAt)
	}
	if finishRunningSnapshotOperation("race", func(operation *SnapshotOperation) {
		operation.Status, operation.Phase = "error", "error"
	}) {
		t.Fatal("worker result overwrote an existing terminal state")
	}
	snapshotOperations.RLock()
	status := snapshotOperations.items["race"].Status
	updatedAt := snapshotOperations.items["race"].UpdatedAt
	snapshotOperations.RUnlock()
	if status != "cancelled" || !updatedAt.Equal(terminalUpdatedAt) {
		t.Fatalf("terminal state was corrupted: status=%q firstUpdatedAt=%v updatedAt=%v", status, terminalUpdatedAt, updatedAt)
	}
}

func TestSnapshotOperationRetentionRemovesOnlyOldTerminalHistory(t *testing.T) {
	now := time.Now()
	snapshotOperations.Lock()
	snapshotOperations.items = map[string]*SnapshotOperation{
		"old":     {ID: "old", Status: "completed", UpdatedAt: now.Add(-snapshotOperationRetention - time.Minute)},
		"recent":  {ID: "recent", Status: "error", UpdatedAt: now.Add(-time.Minute)},
		"running": {ID: "running", Status: "running", CreatedAt: now, UpdatedAt: now},
	}
	snapshotOperations.cancels = map[string]context.CancelFunc{}
	cleanupSnapshotOperationsLocked(now)
	_, oldExists := snapshotOperations.items["old"]
	_, recentExists := snapshotOperations.items["recent"]
	_, runningExists := snapshotOperations.items["running"]
	snapshotOperations.Unlock()
	if oldExists || !recentExists || !runningExists {
		t.Fatalf("unexpected retention result: old=%t recent=%t running=%t", oldExists, recentExists, runningExists)
	}
}

func TestSnapshotOperationRetentionCapsNewestTerminalHistory(t *testing.T) {
	now := time.Now()
	snapshotOperations.Lock()
	snapshotOperations.items = map[string]*SnapshotOperation{
		"running": {ID: "running", Status: "running", CreatedAt: now, UpdatedAt: now},
	}
	for index := 0; index < snapshotOperationMaxHistory+2; index++ {
		id := fmt.Sprintf("terminal-%03d", index)
		snapshotOperations.items[id] = &SnapshotOperation{ID: id, Status: "completed", UpdatedAt: now.Add(time.Duration(index) * time.Millisecond)}
	}
	cleanupSnapshotOperationsLocked(now.Add(time.Second))
	_, oldestExists := snapshotOperations.items["terminal-000"]
	_, newestExists := snapshotOperations.items[fmt.Sprintf("terminal-%03d", snapshotOperationMaxHistory+1)]
	_, runningExists := snapshotOperations.items["running"]
	count := len(snapshotOperations.items)
	snapshotOperations.Unlock()
	if oldestExists || !newestExists || !runningExists || count != snapshotOperationMaxHistory+1 {
		t.Fatalf("unexpected capped history: oldest=%t newest=%t running=%t count=%d", oldestExists, newestExists, runningExists, count)
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
		Path: mediaPath, FileName: "episode.mkv", SizeBytes: int64(len("replacement with different size")), VideoCodec: "cached-codec", CreatedAt: time.Now().Add(-time.Hour),
		VideoStreams:           models.JSONList{map[string]any{"avgFrameRate": "25/1"}},
		FrameStructureAnalysis: models.JSONMap{"version": 2, "framesAnalyzed": 500, "averageGopLength": 75.0, "maxConsecutiveBFrames": 3, "confidence": "medium"},
		RawProbe:               models.JSONMap{},
	}
	info, _ := os.Stat(mediaPath)
	stampSnapshotCacheMetadata(&existing, mediaPath, info)
	if err := db.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}
	result, cached, err := NewScannerHandler(db).scanResolvedFile(mediaPath, info, ScanRequest{Force: false}, nil)
	if err != nil || !cached || result.VideoCodec != "cached-codec" {
		t.Fatalf("snapshot was regenerated without Re-scan: cached=%t result=%#v err=%v", cached, result, err)
	}
	if workerIntValue(result.FrameStructureRecommendation["version"], 0) != 1 {
		t.Fatalf("cached snapshot was not enriched from stored source facts: %#v", result.FrameStructureRecommendation)
	}
	if workerIntValue(result.HEVCLevelRecommendation["version"], 0) != 1 {
		t.Fatalf("cached snapshot was not enriched with an HEVC Level recommendation: %#v", result.HEVCLevelRecommendation)
	}
	var stored models.ScanResult
	if err := db.First(&stored, existing.ID).Error; err != nil || workerIntValue(stored.FrameStructureRecommendation["version"], 0) != 1 || workerIntValue(stored.HEVCLevelRecommendation["version"], 0) != 1 {
		t.Fatalf("enriched recommendation was not persisted: %#v err=%v", stored.FrameStructureRecommendation, err)
	}
	if matches, legacy := snapshotCacheMatches(stored, mediaPath, info); !matches || legacy {
		t.Fatalf("snapshot cache metadata was not retained: matches=%t legacy=%t raw=%#v", matches, legacy, stored.RawProbe)
	}
}

func TestSnapshotCacheFingerprintInvalidatesChangedFile(t *testing.T) {
	mediaPath := filepath.Join(t.TempDir(), "episode.mkv")
	if err := os.WriteFile(mediaPath, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	originalInfo, err := os.Stat(mediaPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := models.ScanResult{RawProbe: models.JSONMap{}}
	stampSnapshotCacheMetadata(&snapshot, mediaPath, originalInfo)
	if matches, legacy := snapshotCacheMatches(snapshot, mediaPath, originalInfo); !matches || legacy {
		t.Fatalf("fresh fingerprint did not match: matches=%t legacy=%t", matches, legacy)
	}

	if err := os.WriteFile(mediaPath, []byte("replacement with a different size"), 0o644); err != nil {
		t.Fatal(err)
	}
	changedInfo, err := os.Stat(mediaPath)
	if err != nil {
		t.Fatal(err)
	}
	if matches, _ := snapshotCacheMatches(snapshot, mediaPath, changedInfo); matches {
		t.Fatal("changed file size did not invalidate the snapshot fingerprint")
	}
}

func TestSnapshotCacheFingerprintInvalidatesChangedMtimeAndComponentVersion(t *testing.T) {
	mediaPath := filepath.Join(t.TempDir(), "episode.mkv")
	if err := os.WriteFile(mediaPath, []byte("same bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(mediaPath)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := models.ScanResult{RawProbe: models.JSONMap{}}
	stampSnapshotCacheMetadata(&snapshot, mediaPath, info)

	changedTime := info.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(mediaPath, changedTime, changedTime); err != nil {
		t.Fatal(err)
	}
	changedInfo, err := os.Stat(mediaPath)
	if err != nil {
		t.Fatal(err)
	}
	if matches, _ := snapshotCacheMatches(snapshot, mediaPath, changedInfo); matches {
		t.Fatal("changed mtime did not invalidate the snapshot fingerprint")
	}

	stampSnapshotCacheMetadata(&snapshot, mediaPath, changedInfo)
	cache, _ := snapshotCacheMap(snapshot.RawProbe["snapshotCache"])
	components, _ := snapshotCacheMap(cache["components"])
	components["crop"] = 0
	if matches, _ := snapshotCacheMatches(snapshot, mediaPath, changedInfo); matches {
		t.Fatal("stale component version did not invalidate the snapshot cache")
	}
}

func TestScanResolvedFileIncrementallyRefreshesCadenceWithoutReplacingOtherEvidence(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:incremental-cadence-refresh?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(t.TempDir(), "episode.mkv")
	if err := os.WriteFile(mediaPath, []byte("not decoded during cadence-only refresh"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(mediaPath)
	if err != nil {
		t.Fatal(err)
	}
	interlace := models.JSONMap{"version": interlaceAnalysisVersion, "status": "progressive", "confidence": 0.99}
	frame := models.JSONMap{"version": 2, "source": "fixture", "framesAnalyzed": 500, "averageGopLength": 50.0, "positions": []any{0.08, 0.5, 0.92}}
	crop := models.JSONMap{"version": 3, "status": "none", "reason": "preserve-me"}
	snapshot := models.ScanResult{
		Path: mediaPath, FileName: "episode.mkv", SizeBytes: info.Size(), Duration: 1200, VideoCodec: "h264", Width: 1920, Height: 1080,
		VideoStreams:      models.JSONList{map[string]any{"codec": "h264", "avgFrameRate": "25/1"}},
		InterlaceAnalysis: interlace, FrameStructureAnalysis: frame, CropAnalysis: crop,
		RawProbe: models.JSONMap{
			"format":            map[string]any{"duration": "1200"},
			"streams":           []any{map[string]any{"codec_type": "video", "codec_name": "h264", "avg_frame_rate": "25/1", "width": 1920, "height": 1080}},
			"interlaceAnalysis": interlace, "frameStructureAnalysis": frame, "cropAnalysis": crop,
		},
	}
	stampSnapshotCacheMetadata(&snapshot, mediaPath, info)
	cache, _ := snapshotCacheMap(snapshot.RawProbe["snapshotCache"])
	components, _ := snapshotCacheMap(cache["components"])
	components["cadence"] = 0
	if err := db.Create(&snapshot).Error; err != nil {
		t.Fatal(err)
	}

	result, cached, err := NewScannerHandler(db).scanResolvedFile(mediaPath, info, ScanRequest{}, nil)
	if err != nil {
		t.Fatalf("incremental cadence refresh failed: %v", err)
	}
	if cached {
		t.Fatal("incremental refresh was incorrectly reported as a cache hit")
	}
	if stringFromUnknown(result.CropAnalysis["reason"]) != "preserve-me" || stringFromUnknown(result.FrameStructureAnalysis["source"]) != "fixture" {
		t.Fatalf("incremental refresh replaced unrelated evidence: crop=%#v frame=%#v", result.CropAnalysis, result.FrameStructureAnalysis)
	}
	if workerIntValue(result.CadenceAnalysis["version"], 0) != cadenceAnalysisVersion {
		t.Fatalf("cadence was not refreshed: %#v", result.CadenceAnalysis)
	}
	if matches, legacy := snapshotCacheMatches(result, mediaPath, info); !matches || legacy {
		t.Fatalf("incrementally refreshed snapshot is not cacheable: matches=%t legacy=%t", matches, legacy)
	}
}

func TestIncrementalRefreshSupportsIndependentAnalysisComponents(t *testing.T) {
	if !incrementalRefreshSupported([]string{"cadence", "crop", "frameStructure", "interlace"}) {
		t.Fatal("analysis components should use the incremental refresh path")
	}
	if incrementalRefreshSupported([]string{"metadata", "frameStructure"}) {
		t.Fatal("stale metadata must require a complete rebuild")
	}
}

func TestScanResolvedFileIncrementallyRefreshesInterlaceAndCropWithSharedDecode(t *testing.T) {
	binDir := t.TempDir()
	counterPath := filepath.Join(binDir, "calls")
	ffmpegPath := filepath.Join(binDir, "ffmpeg")
	script := `#!/bin/sh
printf x >> "$PIXEL_COUNTER"
printf '%s\n' \
  'Repeated Fields: Neither: 100 Top: 0 Bottom: 0' \
  'Multi frame detection: TFF: 0 BFF: 0 Progressive: 100 Undetermined: 0' \
  'crop=700:400:10:40' >&2
`
	if err := os.WriteFile(ffmpegPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PIXEL_COUNTER", counterPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	db, err := gorm.Open(sqlite.Open("file:incremental-pixel-refresh?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(t.TempDir(), "episode.mkv")
	if err := os.WriteFile(mediaPath, []byte("pixel fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(mediaPath)
	frame := models.JSONMap{"version": 2, "source": "preserve-frame", "framesAnalyzed": 100, "positions": []any{0.5}}
	snapshot := models.ScanResult{
		Path: mediaPath, FileName: "episode.mkv", SizeBytes: info.Size(), Duration: 100, VideoCodec: "h264", Width: 720, Height: 480,
		VideoStreams:      models.JSONList{map[string]any{"codec": "h264", "avgFrameRate": "25/1"}},
		InterlaceAnalysis: models.JSONMap{"version": interlaceAnalysisVersion, "status": "unknown"},
		CropAnalysis:      models.JSONMap{"version": 3, "status": "unknown"}, FrameStructureAnalysis: frame,
		RawProbe: models.JSONMap{
			"format":            map[string]any{"duration": "100"},
			"streams":           []any{map[string]any{"codec_type": "video", "codec_name": "h264", "avg_frame_rate": "25/1", "width": 720, "height": 480}},
			"interlaceAnalysis": models.JSONMap{"version": interlaceAnalysisVersion, "status": "unknown"},
			"cropAnalysis":      models.JSONMap{"version": 3, "status": "unknown"}, "frameStructureAnalysis": frame,
		},
	}
	stampSnapshotCacheMetadata(&snapshot, mediaPath, info)
	cache, _ := snapshotCacheMap(snapshot.RawProbe["snapshotCache"])
	components, _ := snapshotCacheMap(cache["components"])
	components["interlace"] = 0
	components["crop"] = 0
	if err := db.Create(&snapshot).Error; err != nil {
		t.Fatal(err)
	}

	result, cached, err := NewScannerHandler(db).scanResolvedFile(mediaPath, info, ScanRequest{}, nil)
	if err != nil || cached {
		t.Fatalf("incremental pixel refresh failed: cached=%t err=%v", cached, err)
	}
	calls, err := os.ReadFile(counterPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(calls) != "x" {
		t.Fatalf("Interlace and Crop did not share one decode: calls=%q", calls)
	}
	if stringFromUnknown(result.FrameStructureAnalysis["source"]) != "preserve-frame" {
		t.Fatalf("Frame Structure was unexpectedly replaced: %#v", result.FrameStructureAnalysis)
	}
	if stringFromUnknown(result.InterlaceAnalysis["status"]) != "progressive" || stringFromUnknown(result.CropAnalysis["recommendedCrop"]) != "700:400:10:40" {
		t.Fatalf("pixel evidence was not refreshed: interlace=%#v crop=%#v", result.InterlaceAnalysis, result.CropAnalysis)
	}
	reused, refreshed, statuses := snapshotRefreshDetails(result, false)
	if !slices.Contains(reused, "metadata") || !slices.Contains(reused, "frameStructure") || !slices.Contains(refreshed, "interlace") || !slices.Contains(refreshed, "crop") || statuses["interlace"] != "valid" {
		t.Fatalf("unexpected refresh details: reused=%v refreshed=%v statuses=%v", reused, refreshed, statuses)
	}
}

func TestCropOnlyIncrementalRefreshKeepsIndependentThreeWindowDepth(t *testing.T) {
	binDir := t.TempDir()
	counterPath := filepath.Join(binDir, "calls")
	ffmpegPath := filepath.Join(binDir, "ffmpeg")
	script := `#!/bin/sh
printf x >> "$PIXEL_COUNTER"
printf '%s\n' 'crop=700:400:10:40' >&2
`
	if err := os.WriteFile(ffmpegPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PIXEL_COUNTER", counterPath)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	db, err := gorm.Open(sqlite.Open("file:incremental-crop-only-depth?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(t.TempDir(), "episode.mkv")
	if err := os.WriteFile(mediaPath, []byte("crop fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(mediaPath)
	interlace := models.JSONMap{"version": interlaceAnalysisVersion, "status": "progressive", "analysisDepth": "deep", "reason": "preserve-interlace"}
	frame := models.JSONMap{
		"version": 2, "source": "cached-frame", "framesAnalyzed": 720, "confidenceScore": 0.995, "windowCount": 3,
		"positions": []any{0.08, 0.27, 0.50, 0.73, 0.92},
		"windows": []any{
			map[string]any{"position": 0.08, "frameSignals": map[string]any{"decodedFrames": 240, "progressiveFrames": 240, "effectiveFps": 24.0}},
			map[string]any{"position": 0.50, "frameSignals": map[string]any{"decodedFrames": 240, "progressiveFrames": 240, "effectiveFps": 24.0}},
			map[string]any{"position": 0.92, "frameSignals": map[string]any{"decodedFrames": 240, "progressiveFrames": 240, "effectiveFps": 24.0}},
		},
	}
	snapshot := models.ScanResult{
		Path: mediaPath, FileName: "episode.mkv", SizeBytes: info.Size(), Duration: 1000, VideoCodec: "h264", Width: 720, Height: 480,
		VideoStreams:      models.JSONList{map[string]any{"codec": "h264", "avgFrameRate": "25/1"}},
		InterlaceAnalysis: interlace, FrameStructureAnalysis: frame, CropAnalysis: models.JSONMap{"version": 3, "status": "unknown"},
		RawProbe: models.JSONMap{
			"format":            map[string]any{"format_name": "matroska", "duration": "1000"},
			"streams":           []any{map[string]any{"codec_type": "video", "codec_name": "h264", "avg_frame_rate": "25/1", "width": 720, "height": 480}},
			"interlaceAnalysis": interlace, "frameStructureAnalysis": frame, "cropAnalysis": models.JSONMap{"version": 3, "status": "unknown"},
		},
	}
	stampSnapshotCacheMetadata(&snapshot, mediaPath, info)
	cache, _ := snapshotCacheMap(snapshot.RawProbe["snapshotCache"])
	components, _ := snapshotCacheMap(cache["components"])
	components["crop"] = 0
	if err := db.Create(&snapshot).Error; err != nil {
		t.Fatal(err)
	}

	result, cached, err := NewScannerHandler(db).scanResolvedFile(mediaPath, info, ScanRequest{}, nil)
	if err != nil || cached {
		t.Fatalf("crop-only incremental refresh failed: cached=%t err=%v", cached, err)
	}
	calls, err := os.ReadFile(counterPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(calls) != "xxx" || jsonMapInt(result.CropAnalysis, "windows") != 3 || stringFromUnknown(result.InterlaceAnalysis["reason"]) != "preserve-interlace" {
		t.Fatalf("Crop depth depended on Adaptive IDET: calls=%q crop=%#v interlace=%#v", calls, result.CropAnalysis, result.InterlaceAnalysis)
	}
}

func TestScanResolvedFileFingerprintAndForceBypassCacheEndToEnd(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(string, os.FileInfo) error
		force  bool
	}{
		{name: "size", mutate: func(path string, _ os.FileInfo) error { return os.WriteFile(path, []byte("different size"), 0o644) }},
		{name: "mtime", mutate: func(path string, info os.FileInfo) error {
			changed := info.ModTime().Add(2 * time.Second)
			return os.Chtimes(path, changed, changed)
		}},
		{name: "force", force: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open("file:"+test.name+"-cache-bypass?mode=memory&cache=shared"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(&models.ScanResult{}); err != nil {
				t.Fatal(err)
			}
			mediaPath := filepath.Join(t.TempDir(), "episode.mkv")
			if err := os.WriteFile(mediaPath, []byte("fixture"), 0o644); err != nil {
				t.Fatal(err)
			}
			info, _ := os.Stat(mediaPath)
			snapshot := models.ScanResult{
				Path: mediaPath, FileName: "episode.mkv", SizeBytes: info.Size(), VideoCodec: "h264",
				VideoStreams:           models.JSONList{map[string]any{"codec": "h264", "avgFrameRate": "25/1"}},
				FrameStructureAnalysis: models.JSONMap{"version": 2, "framesAnalyzed": 10}, RawProbe: models.JSONMap{},
			}
			stampSnapshotCacheMetadata(&snapshot, mediaPath, info)
			if err := db.Create(&snapshot).Error; err != nil {
				t.Fatal(err)
			}
			if test.mutate != nil {
				if err := test.mutate(mediaPath, info); err != nil {
					t.Fatal(err)
				}
				info, _ = os.Stat(mediaPath)
			}
			var phases []string
			_, cached, scanErr := NewScannerHandler(db).scanResolvedFile(mediaPath, info, ScanRequest{Force: test.force}, func(phase string, _ float64, _ string) {
				phases = append(phases, phase)
			})
			if cached || scanErr == nil || !slices.Contains(phases, "metadata") {
				t.Fatalf("cache was not bypassed: cached=%t err=%v phases=%v", cached, scanErr, phases)
			}
		})
	}
}

func TestFailedIncrementalFrameAnalyzerPersistsReusablePartialSnapshot(t *testing.T) {
	binDir := t.TempDir()
	ffprobePath := filepath.Join(binDir, "ffprobe")
	if err := os.WriteFile(ffprobePath, []byte("#!/bin/sh\nexit 2\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	db, err := gorm.Open(sqlite.Open("file:partial-frame-snapshot?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(t.TempDir(), "episode.mkv")
	if err := os.WriteFile(mediaPath, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(mediaPath)
	interlace := models.JSONMap{"version": interlaceAnalysisVersion, "status": "progressive"}
	frame := models.JSONMap{"version": 2, "source": "old", "framesAnalyzed": 10}
	snapshot := models.ScanResult{
		Path: mediaPath, FileName: "episode.mkv", SizeBytes: info.Size(), Duration: 100, VideoCodec: "h264", Width: 1920, Height: 1080,
		VideoStreams: models.JSONList{map[string]any{"codec": "h264", "avgFrameRate": "25/1"}}, InterlaceAnalysis: interlace, FrameStructureAnalysis: frame,
		RawProbe: models.JSONMap{
			"format":            map[string]any{"duration": "100"},
			"streams":           []any{map[string]any{"codec_type": "video", "codec_name": "h264", "avg_frame_rate": "25/1", "width": 1920, "height": 1080}},
			"interlaceAnalysis": interlace, "frameStructureAnalysis": frame,
		},
	}
	stampSnapshotCacheMetadata(&snapshot, mediaPath, info)
	cache, _ := snapshotCacheMap(snapshot.RawProbe["snapshotCache"])
	components, _ := snapshotCacheMap(cache["components"])
	components["frameStructure"] = 1
	if err := db.Create(&snapshot).Error; err != nil {
		t.Fatal(err)
	}

	partial, cached, err := NewScannerHandler(db).scanResolvedFile(mediaPath, info, ScanRequest{}, nil)
	if err != nil || cached || stringFromUnknown(partial.FrameStructureAnalysis["status"]) != "unverified" {
		t.Fatalf("partial snapshot was not persisted: cached=%t err=%v frame=%#v", cached, err, partial.FrameStructureAnalysis)
	}
	_, _, statuses := snapshotRefreshDetails(partial, false)
	if statuses["frameStructure"] != "unverified" || len(partial.VideoStreams) == 0 || partial.InterlaceAnalysis["status"] != "progressive" {
		t.Fatalf("partial evidence/status was lost: statuses=%v snapshot=%#v", statuses, partial)
	}
	reused, cacheHit, err := NewScannerHandler(db).scanResolvedFile(mediaPath, info, ScanRequest{}, nil)
	if err != nil || !cacheHit || stringFromUnknown(reused.FrameStructureAnalysis["status"]) != "unverified" {
		t.Fatalf("partial snapshot caused an automatic rebuild: cacheHit=%t err=%v frame=%#v", cacheHit, err, reused.FrameStructureAnalysis)
	}
}

func TestLegacySnapshotMigrationPreservesKnownVersionsAndMarksUnknownStale(t *testing.T) {
	mediaPath := filepath.Join(t.TempDir(), "episode.mkv")
	if err := os.WriteFile(mediaPath, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(mediaPath)
	snapshot := models.ScanResult{
		Path: mediaPath, Duration: 100,
		InterlaceAnalysis: models.JSONMap{"version": interlaceAnalysisVersion, "status": "progressive"},
		CropAnalysis:      models.JSONMap{"status": "none"},
		RawProbe: models.JSONMap{
			"format":  map[string]any{"format_name": "matroska", "duration": "100"},
			"streams": []any{map[string]any{"codec_type": "video", "codec_name": "h264"}},
		},
	}
	migrateLegacySnapshotCache(&snapshot, mediaPath, info)
	cache, _ := snapshotCacheMap(snapshot.RawProbe["snapshotCache"])
	components, _ := snapshotCacheMap(cache["components"])
	interlace, _ := snapshotCacheMap(components["interlace"])
	crop, _ := snapshotCacheMap(components["crop"])
	metadata, _ := snapshotCacheMap(components["metadata"])
	if workerIntValue(interlace["version"], 0) != interlaceAnalysisVersion || stringFromUnknown(interlace["provenance"]) != "legacy_inferred" {
		t.Fatalf("known legacy Interlace version was not preserved: %#v", interlace)
	}
	if fmt.Sprint(crop["version"]) != "0" || stringFromUnknown(crop["status"]) != "unknown" {
		t.Fatalf("unknown legacy Crop provenance was marked current: %#v", crop)
	}
	if workerIntValue(metadata["version"], 0) != workerIntValue(snapshotAnalysisVersions["metadata"], 0) {
		t.Fatalf("usable legacy metadata was not retained: %#v", metadata)
	}
}

func TestLegacySnapshotIdentityRequiresMatchingHistoricalSize(t *testing.T) {
	tests := []struct {
		name       string
		historical int64
		wantMatch  bool
	}{
		{name: "matching size", historical: 7, wantMatch: true},
		{name: "different size", historical: 10, wantMatch: false},
		{name: "missing historical identity", historical: 0, wantMatch: false},
	}
	mediaPath := filepath.Join(t.TempDir(), "episode.mkv")
	if err := os.WriteFile(mediaPath, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(mediaPath)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := legacySnapshotIdentityMatches(models.ScanResult{SizeBytes: test.historical}, info); got != test.wantMatch {
				t.Fatalf("identity match=%t want=%t historical=%d current=%d", got, test.wantMatch, test.historical, info.Size())
			}
		})
	}
}

func TestLegacySnapshotIdentityMismatchSelectsFullMetadataRefresh(t *testing.T) {
	tests := []struct {
		name       string
		historical int64
	}{
		{name: "size mismatch", historical: 999},
		{name: "missing identity", historical: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			binDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(binDir, "ffprobe"), []byte("#!/bin/sh\nprintf 'legacy full refresh attempted' >&2\nexit 19\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			db, err := gorm.Open(sqlite.Open("file:legacy-identity-"+strings.ReplaceAll(test.name, " ", "-")+"?mode=memory&cache=shared"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(&models.ScanResult{}); err != nil {
				t.Fatal(err)
			}
			mediaPath := filepath.Join(t.TempDir(), "episode.mkv")
			if err := os.WriteFile(mediaPath, []byte("current file"), 0o644); err != nil {
				t.Fatal(err)
			}
			legacy := models.ScanResult{
				Path: mediaPath, FileName: "episode.mkv", SizeBytes: test.historical, VideoCodec: "h264",
				VideoStreams:           models.JSONList{map[string]any{"codec": "h264", "avgFrameRate": "25/1"}},
				FrameStructureAnalysis: models.JSONMap{"version": 2, "framesAnalyzed": 100}, RawProbe: models.JSONMap{},
			}
			if err := db.Create(&legacy).Error; err != nil {
				t.Fatal(err)
			}
			info, _ := os.Stat(mediaPath)
			var phases []string
			_, cached, scanErr := NewScannerHandler(db).scanResolvedFile(mediaPath, info, ScanRequest{}, func(phase string, _ float64, _ string) { phases = append(phases, phase) })
			if cached || scanErr == nil || !slices.Contains(phases, "legacy_identity_mismatch") || !slices.Contains(phases, "metadata") || slices.Contains(phases, "incremental_refresh") {
				t.Fatalf("unsafe legacy migration was not rejected: cached=%t err=%v phases=%v", cached, scanErr, phases)
			}
			var stored models.ScanResult
			if err := db.First(&stored, legacy.ID).Error; err != nil {
				t.Fatal(err)
			}
			if _, migrated := stored.RawProbe["snapshotCache"]; migrated {
				t.Fatalf("unsafe legacy snapshot received a cache fingerprint: %#v", stored.RawProbe)
			}
		})
	}
}

func TestLegacySnapshotWithoutPolicyRefreshesAdvancedEvidenceWithoutMetadataProbe(t *testing.T) {
	binDir := t.TempDir()
	ffprobePath := filepath.Join(binDir, "ffprobe")
	probeScript := `#!/bin/sh
case "$*" in
  *-show_format*) exit 90 ;;
esac
printf '%s' '{"frames":[
{"pict_type":"I","key_frame":1,"best_effort_timestamp_time":"8.00","pkt_duration_time":"0.04","interlaced_frame":0,"repeat_pict":0},
{"pict_type":"I","key_frame":1,"best_effort_timestamp_time":"8.04","pkt_duration_time":"0.04","interlaced_frame":0,"repeat_pict":0},
{"pict_type":"I","key_frame":1,"best_effort_timestamp_time":"50.00","pkt_duration_time":"0.04","interlaced_frame":0,"repeat_pict":0},
{"pict_type":"I","key_frame":1,"best_effort_timestamp_time":"50.04","pkt_duration_time":"0.04","interlaced_frame":0,"repeat_pict":0},
{"pict_type":"I","key_frame":1,"best_effort_timestamp_time":"92.00","pkt_duration_time":"0.04","interlaced_frame":0,"repeat_pict":0},
{"pict_type":"I","key_frame":1,"best_effort_timestamp_time":"92.04","pkt_duration_time":"0.04","interlaced_frame":0,"repeat_pict":0}
]}'
`
	if err := os.WriteFile(ffprobePath, []byte(probeScript), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	db, err := gorm.Open(sqlite.Open("file:legacy-frame-incremental?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(t.TempDir(), "episode.mkv")
	if err := os.WriteFile(mediaPath, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(mediaPath)
	interlace := models.JSONMap{"version": interlaceAnalysisVersion, "status": "progressive"}
	crop := models.JSONMap{"version": 3, "status": "none", "reason": "legacy-crop"}
	legacy := models.ScanResult{
		Path: mediaPath, FileName: "episode.mkv", SizeBytes: info.Size(), Duration: 100, VideoCodec: "h264", Width: 1920, Height: 1080,
		VideoStreams: models.JSONList{map[string]any{"codec": "h264", "avgFrameRate": "25/1"}}, InterlaceAnalysis: interlace, CropAnalysis: crop,
		RawProbe: models.JSONMap{
			"format":            map[string]any{"format_name": "matroska", "duration": "100"},
			"streams":           []any{map[string]any{"codec_type": "video", "codec_name": "h264", "avg_frame_rate": "25/1", "width": 1920, "height": 1080}},
			"interlaceAnalysis": interlace, "cropAnalysis": crop,
		},
	}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	var phases []string
	result, cached, err := NewScannerHandler(db).scanResolvedFile(mediaPath, info, ScanRequest{}, func(phase string, _ float64, _ string) { phases = append(phases, phase) })
	if err != nil || cached {
		t.Fatalf("legacy incremental refresh failed: cached=%t err=%v phases=%v", cached, err, phases)
	}
	if slices.Contains(phases, "metadata") || !slices.Contains(phases, "frame_structure") || !slices.Contains(phases, "interlace") || !slices.Contains(phases, "crop") || stringFromUnknown(result.CropAnalysis["reason"]) == "legacy-crop" {
		t.Fatalf("legacy refresh did not rebuild policy-unknown evidence: phases=%v crop=%#v", phases, result.CropAnalysis)
	}
}

func TestInvalidIncrementalCacheFallsBackToSuccessfulFullRefresh(t *testing.T) {
	binDir := t.TempDir()
	ffprobePath := filepath.Join(binDir, "ffprobe")
	ffmpegPath := filepath.Join(binDir, "ffmpeg")
	probeScript := `#!/bin/sh
case "$*" in
  *-show_format*) printf '%s' '{"format":{"format_name":"matroska","duration":"100","bit_rate":"1000000"},"streams":[{"codec_type":"video","codec_name":"h264","avg_frame_rate":"25/1","width":720,"height":480}]}' ;;
  *) printf '%s' '{"frames":[
    {"pict_type":"I","key_frame":1,"best_effort_timestamp_time":"8.00","pkt_duration_time":"0.04","interlaced_frame":0,"repeat_pict":0},
    {"pict_type":"I","key_frame":1,"best_effort_timestamp_time":"8.04","pkt_duration_time":"0.04","interlaced_frame":0,"repeat_pict":0},
    {"pict_type":"I","key_frame":1,"best_effort_timestamp_time":"50.00","pkt_duration_time":"0.04","interlaced_frame":0,"repeat_pict":0},
    {"pict_type":"I","key_frame":1,"best_effort_timestamp_time":"50.04","pkt_duration_time":"0.04","interlaced_frame":0,"repeat_pict":0},
    {"pict_type":"I","key_frame":1,"best_effort_timestamp_time":"92.00","pkt_duration_time":"0.04","interlaced_frame":0,"repeat_pict":0},
    {"pict_type":"I","key_frame":1,"best_effort_timestamp_time":"92.04","pkt_duration_time":"0.04","interlaced_frame":0,"repeat_pict":0}
  ]}' ;;
esac
`
	pixelScript := `#!/bin/sh
printf '%s\n' \
  'Repeated Fields: Neither: 100 Top: 0 Bottom: 0' \
  'Multi frame detection: TFF: 0 BFF: 0 Progressive: 100 Undetermined: 0' \
  'crop=700:400:10:40' >&2
`
	if err := os.WriteFile(ffprobePath, []byte(probeScript), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ffmpegPath, []byte(pixelScript), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	db, err := gorm.Open(sqlite.Open("file:incremental-full-fallback?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(t.TempDir(), "episode.mkv")
	if err := os.WriteFile(mediaPath, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(mediaPath)
	snapshot := models.ScanResult{
		Path: mediaPath, FileName: "episode.mkv", SizeBytes: info.Size(), Duration: 100, VideoCodec: "h264", Width: 720, Height: 480,
		VideoStreams:           models.JSONList{map[string]any{"codec": "h264", "avgFrameRate": "25/1"}},
		FrameStructureAnalysis: models.JSONMap{"version": 2, "framesAnalyzed": 10}, RawProbe: models.JSONMap{},
	}
	// Stamp first, then corrupt the cached base metadata while retaining a valid manifest.
	snapshot.RawProbe = models.JSONMap{}
	stampSnapshotCacheMetadata(&snapshot, mediaPath, info)
	cache, _ := snapshotCacheMap(snapshot.RawProbe["snapshotCache"])
	components, _ := snapshotCacheMap(cache["components"])
	components["frameStructure"] = 1
	if err := db.Create(&snapshot).Error; err != nil {
		t.Fatal(err)
	}
	var phases []string
	result, cached, err := NewScannerHandler(db).scanResolvedFile(mediaPath, info, ScanRequest{}, func(phase string, _ float64, _ string) { phases = append(phases, phase) })
	if err != nil || cached {
		t.Fatalf("full fallback failed: cached=%t err=%v phases=%v", cached, err, phases)
	}
	if !slices.Contains(phases, "incremental_fallback") || !slices.Contains(phases, "metadata") || snapshotFallbackReason(result) != "cached_metadata_invalid" {
		t.Fatalf("fallback was not observable: phases=%v reason=%q", phases, snapshotFallbackReason(result))
	}
}

func TestInvalidIncrementalCacheReturnsFullRefreshFailure(t *testing.T) {
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "ffprobe"), []byte("#!/bin/sh\nprintf 'full probe failed' >&2\nexit 17\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	db, err := gorm.Open(sqlite.Open("file:incremental-fallback-fails?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(t.TempDir(), "episode.mkv")
	if err := os.WriteFile(mediaPath, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(mediaPath)
	snapshot := models.ScanResult{
		Path: mediaPath, FileName: "episode.mkv", SizeBytes: info.Size(), VideoCodec: "h264",
		VideoStreams:           models.JSONList{map[string]any{"codec": "h264", "avgFrameRate": "25/1"}},
		FrameStructureAnalysis: models.JSONMap{"version": 2, "framesAnalyzed": 10}, RawProbe: models.JSONMap{},
	}
	stampSnapshotCacheMetadata(&snapshot, mediaPath, info)
	cache, _ := snapshotCacheMap(snapshot.RawProbe["snapshotCache"])
	components, _ := snapshotCacheMap(cache["components"])
	components["frameStructure"] = 1
	if err := db.Create(&snapshot).Error; err != nil {
		t.Fatal(err)
	}
	var phases []string
	_, cached, scanErr := NewScannerHandler(db).scanResolvedFile(mediaPath, info, ScanRequest{}, func(phase string, _ float64, _ string) { phases = append(phases, phase) })
	if cached || scanErr == nil || !strings.Contains(scanErr.Error(), "full probe failed") || !slices.Contains(phases, "incremental_fallback") || !slices.Contains(phases, "metadata") {
		t.Fatalf("full refresh failure was not returned: cached=%t err=%v phases=%v", cached, scanErr, phases)
	}
}

func TestStaleMetadataComponentRequiresFullRefresh(t *testing.T) {
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "ffprobe"), []byte("#!/bin/sh\nprintf 'metadata refresh attempted' >&2\nexit 18\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	db, err := gorm.Open(sqlite.Open("file:metadata-component-stale?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(t.TempDir(), "episode.mkv")
	if err := os.WriteFile(mediaPath, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(mediaPath)
	snapshot := models.ScanResult{
		Path: mediaPath, FileName: "episode.mkv", SizeBytes: info.Size(), VideoCodec: "h264",
		VideoStreams:           models.JSONList{map[string]any{"codec": "h264", "avgFrameRate": "25/1"}},
		FrameStructureAnalysis: models.JSONMap{"version": 2, "framesAnalyzed": 10}, RawProbe: models.JSONMap{},
	}
	stampSnapshotCacheMetadata(&snapshot, mediaPath, info)
	cache, _ := snapshotCacheMap(snapshot.RawProbe["snapshotCache"])
	components, _ := snapshotCacheMap(cache["components"])
	components["metadata"] = 0
	if err := db.Create(&snapshot).Error; err != nil {
		t.Fatal(err)
	}
	var phases []string
	_, _, scanErr := NewScannerHandler(db).scanResolvedFile(mediaPath, info, ScanRequest{}, func(phase string, _ float64, _ string) { phases = append(phases, phase) })
	if scanErr == nil || !slices.Contains(phases, "metadata") || slices.Contains(phases, "incremental_refresh") {
		t.Fatalf("stale metadata did not require full refresh: err=%v phases=%v", scanErr, phases)
	}
}

func TestScanResolvedFileRefreshesOnlyFrameStructureAndDependentCadence(t *testing.T) {
	binDir := t.TempDir()
	ffprobePath := filepath.Join(binDir, "ffprobe")
	probeScript := `#!/bin/sh
printf '%s' '{"frames":[
{"pict_type":"I","key_frame":1,"best_effort_timestamp_time":"80.00","pkt_duration_time":"0.04","interlaced_frame":0,"repeat_pict":0},
{"pict_type":"P","key_frame":0,"best_effort_timestamp_time":"80.04","pkt_duration_time":"0.04","interlaced_frame":0,"repeat_pict":0},
{"pict_type":"I","key_frame":1,"best_effort_timestamp_time":"270.00","pkt_duration_time":"0.04","interlaced_frame":0,"repeat_pict":0},
{"pict_type":"I","key_frame":1,"best_effort_timestamp_time":"500.00","pkt_duration_time":"0.04","interlaced_frame":0,"repeat_pict":0},
{"pict_type":"P","key_frame":0,"best_effort_timestamp_time":"500.04","pkt_duration_time":"0.04","interlaced_frame":0,"repeat_pict":0},
{"pict_type":"I","key_frame":1,"best_effort_timestamp_time":"730.00","pkt_duration_time":"0.04","interlaced_frame":0,"repeat_pict":0},
{"pict_type":"I","key_frame":1,"best_effort_timestamp_time":"920.00","pkt_duration_time":"0.04","interlaced_frame":0,"repeat_pict":0},
{"pict_type":"P","key_frame":0,"best_effort_timestamp_time":"920.04","pkt_duration_time":"0.04","interlaced_frame":0,"repeat_pict":0}
]}'
`
	if err := os.WriteFile(ffprobePath, []byte(probeScript), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	db, err := gorm.Open(sqlite.Open("file:incremental-frame-refresh?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(t.TempDir(), "episode.mkv")
	if err := os.WriteFile(mediaPath, []byte("frame fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(mediaPath)
	interlace := models.JSONMap{"version": interlaceAnalysisVersion, "status": "progressive", "confidence": 0.99}
	crop := models.JSONMap{"version": 3, "status": "none", "reason": "preserve-me"}
	oldFrame := models.JSONMap{"version": 2, "source": "old-frame", "framesAnalyzed": 10}
	snapshot := models.ScanResult{
		Path: mediaPath, FileName: "episode.mkv", SizeBytes: info.Size(), Duration: 1000, VideoCodec: "h264", Width: 1920, Height: 1080,
		VideoStreams:      models.JSONList{map[string]any{"codec": "h264", "avgFrameRate": "25/1"}},
		InterlaceAnalysis: interlace, FrameStructureAnalysis: oldFrame, CropAnalysis: crop,
		RawProbe: models.JSONMap{
			"format":            map[string]any{"duration": "1000"},
			"streams":           []any{map[string]any{"codec_type": "video", "codec_name": "h264", "avg_frame_rate": "25/1", "width": 1920, "height": 1080}},
			"interlaceAnalysis": interlace, "frameStructureAnalysis": oldFrame, "cropAnalysis": crop,
		},
	}
	stampSnapshotCacheMetadata(&snapshot, mediaPath, info)
	cache, _ := snapshotCacheMap(snapshot.RawProbe["snapshotCache"])
	components, _ := snapshotCacheMap(cache["components"])
	components["frameStructure"] = 1
	if err := db.Create(&snapshot).Error; err != nil {
		t.Fatal(err)
	}

	var phases []string
	result, cached, err := NewScannerHandler(db).scanResolvedFile(mediaPath, info, ScanRequest{}, func(phase string, _ float64, _ string) {
		phases = append(phases, phase)
	})
	if err != nil || cached {
		t.Fatalf("incremental Frame Structure refresh failed: cached=%t err=%v", cached, err)
	}
	if stringFromUnknown(result.CropAnalysis["reason"]) != "preserve-me" || stringFromUnknown(result.InterlaceAnalysis["status"]) != "progressive" {
		t.Fatalf("unrelated evidence was replaced: crop=%#v interlace=%#v", result.CropAnalysis, result.InterlaceAnalysis)
	}
	if stringFromUnknown(result.FrameStructureAnalysis["source"]) == "old-frame" || jsonMapInt(result.FrameStructureAnalysis, "framesAnalyzed") == 0 {
		t.Fatalf("Frame Structure was not refreshed: %#v", result.FrameStructureAnalysis)
	}
	if !slices.Contains(phases, "incremental_refresh") || slices.Contains(phases, "metadata") || slices.Contains(phases, "crop") || slices.Contains(phases, "interlace") {
		t.Fatalf("unexpected incremental phases: %v", phases)
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
	if workerIntValue(result.HEVCLevelRecommendation["version"], 0) != 1 {
		t.Fatalf("archive did not inherit the HEVC Level recommendation: %#v", result.HEVCLevelRecommendation)
	}
}

func TestArchivedOriginalInheritanceValidatesCurrentAnalysisPolicy(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		thorough   bool
		wantCached bool
	}{
		{name: "same Balanced policy reuses inherited evidence", wantCached: true},
		{name: "Thorough policy refreshes inherited evidence", thorough: true, wantCached: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			binDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(binDir, "ffprobe"), []byte("#!/bin/sh\nprintf 'frame probe unavailable' >&2\nexit 20\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			pixelScript := "#!/bin/sh\nprintf '%s\\n' 'Repeated Fields: Neither: 100 Top: 0 Bottom: 0' 'Multi frame detection: TFF: 0 BFF: 0 Progressive: 100 Undetermined: 0' 'crop=700:400:10:40' >&2\n"
			if err := os.WriteFile(filepath.Join(binDir, "ffmpeg"), []byte(pixelScript), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			db, err := gorm.Open(sqlite.Open("file:archive-policy-"+strings.ReplaceAll(testCase.name, " ", "-")+"?mode=memory&cache=shared"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(&models.ScanResult{}, &models.QueueJob{}, &models.AppSetting{}); err != nil {
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
			info, _ := os.Stat(archivePath)
			source := completeAnalysisSnapshot(rawPath, info)
			stampSnapshotCacheMetadataWithPolicy(&source, rawPath, info, balancedAnalysisPolicy())
			if err := db.Create(&source).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&models.QueueJob{MediaPath: rawPath, OriginalArchivedPath: archivePath, Status: JobStatusCompleted}).Error; err != nil {
				t.Fatal(err)
			}
			if testCase.thorough {
				if err := db.Create(&models.AppSetting{Key: analysisPolicySettingKey, Value: models.JSONMap{"mode": "thorough", "reuseSnapshots": true, "incrementalRefresh": true, "concurrentAssets": 1}}).Error; err != nil {
					t.Fatal(err)
				}
			}
			var phases []string
			result, cached, err := NewScannerHandler(db).scanResolvedFile(archivePath, info, ScanRequest{AnalysisSeconds: 20}, func(phase string, _ float64, _ string) { phases = append(phases, phase) })
			if err != nil || cached != testCase.wantCached {
				t.Fatalf("inheritance policy validation failed: cached=%t err=%v phases=%v", cached, err, phases)
			}
			if testCase.thorough {
				_, refreshed, _ := snapshotRefreshDetails(result, false)
				if !slices.Contains(phases, "incremental_refresh") || slices.Contains(phases, "metadata") || !slices.Equal(refreshed, []string{"cadence", "crop", "frameStructure", "interlace"}) {
					t.Fatalf("inherited evidence was not selectively refreshed: phases=%v refreshed=%v", phases, refreshed)
				}
			}
		})
	}
}

func TestArchivedOriginalInheritancePreservesStaleComponentProvenance(t *testing.T) {
	for _, testCase := range []struct {
		component     string
		wantRefreshed []string
		wantReused    []string
	}{
		{component: "interlace", wantRefreshed: []string{"cadence", "interlace"}, wantReused: []string{"metadata", "crop", "frameStructure"}},
		{component: "crop", wantRefreshed: []string{"crop"}, wantReused: []string{"metadata", "interlace", "frameStructure", "cadence"}},
	} {
		t.Run(testCase.component, func(t *testing.T) {
			binDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(binDir, "ffprobe"), []byte("#!/bin/sh\nprintf 'unexpected frame probe' >&2\nexit 21\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			pixelScript := "#!/bin/sh\nprintf '%s\\n' 'Repeated Fields: Neither: 100 Top: 0 Bottom: 0' 'Multi frame detection: TFF: 0 BFF: 0 Progressive: 100 Undetermined: 0' 'crop=700:400:10:40' >&2\n"
			if err := os.WriteFile(filepath.Join(binDir, "ffmpeg"), []byte(pixelScript), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			db, err := gorm.Open(sqlite.Open("file:archive-stale-"+testCase.component+"?mode=memory&cache=shared"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(&models.ScanResult{}, &models.QueueJob{}, &models.AppSetting{}); err != nil {
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
			info, _ := os.Stat(archivePath)
			source := completeAnalysisSnapshot(rawPath, info)
			stampSnapshotCacheMetadataWithPolicy(&source, rawPath, info, balancedAnalysisPolicy())
			cache, _ := snapshotCacheMap(source.RawProbe["snapshotCache"])
			components, _ := snapshotCacheMap(cache["components"])
			entry, _ := snapshotCacheMap(components[testCase.component])
			oldVersion := workerIntValue(entry["version"], 0) - 1
			entry["version"] = oldVersion
			if err := db.Create(&source).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&models.QueueJob{MediaPath: rawPath, OriginalArchivedPath: archivePath, Status: JobStatusCompleted}).Error; err != nil {
				t.Fatal(err)
			}

			inherited, ok := inheritedOriginalSnapshot(db, archivePath, info)
			if !ok {
				t.Fatal("expected Raw snapshot inheritance")
			}
			inheritedCache, _ := snapshotCacheMap(inherited.RawProbe["snapshotCache"])
			inheritedComponents, _ := snapshotCacheMap(inheritedCache["components"])
			inheritedEntry, _ := snapshotCacheMap(inheritedComponents[testCase.component])
			if version := workerIntValue(inheritedEntry["version"], 0); version != oldVersion {
				t.Fatalf("inherited %s version=%d want=%d", testCase.component, version, oldVersion)
			}

			var phases []string
			result, cached, err := NewScannerHandler(db).scanResolvedFile(archivePath, info, ScanRequest{AnalysisSeconds: 20}, func(phase string, _ float64, _ string) { phases = append(phases, phase) })
			if err != nil || cached {
				t.Fatalf("stale inherited refresh failed: cached=%t err=%v phases=%v", cached, err, phases)
			}
			reused, refreshed, _ := snapshotRefreshDetails(result, false)
			if slices.Contains(phases, "metadata") || !slices.Contains(phases, "incremental_refresh") || !slices.Equal(refreshed, testCase.wantRefreshed) || !slices.Equal(reused, testCase.wantReused) {
				t.Fatalf("unexpected inherited refresh: phases=%v reused=%v refreshed=%v", phases, reused, refreshed)
			}
		})
	}
}

func TestArchivedOriginalLegacyInheritanceDoesNotUpgradeKnownVersions(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:archive-legacy-provenance?mode=memory&cache=shared"), &gorm.Config{})
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
	info, _ := os.Stat(archivePath)
	interlace := models.JSONMap{"version": interlaceAnalysisCacheVersion - 1, "status": "progressive"}
	source := completeAnalysisSnapshot(rawPath, info)
	source.InterlaceAnalysis = interlace
	source.RawProbe["interlaceAnalysis"] = interlace
	delete(source.RawProbe, "snapshotCache")
	if err := db.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.QueueJob{MediaPath: rawPath, OriginalArchivedPath: archivePath, Status: JobStatusCompleted}).Error; err != nil {
		t.Fatal(err)
	}
	inherited, ok := inheritedOriginalSnapshot(db, archivePath, info)
	if !ok {
		t.Fatal("expected legacy Raw snapshot inheritance")
	}
	cache, _ := snapshotCacheMap(inherited.RawProbe["snapshotCache"])
	components, _ := snapshotCacheMap(cache["components"])
	entry, _ := snapshotCacheMap(components["interlace"])
	if version := workerIntValue(entry["version"], 0); version != interlaceAnalysisCacheVersion-1 {
		t.Fatalf("legacy interlace provenance was upgraded: version=%d", version)
	}
	if _, hasPolicy := cache["analysisPolicy"]; hasPolicy {
		t.Fatalf("legacy inheritance invented analysis policy: %#v", cache["analysisPolicy"])
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
				"codec": "h264", "avgFrameRate": "25/1",
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
				"codec": "h264", "avgFrameRate": "25/1",
			},
		},
		FrameStructureAnalysis: models.JSONMap{
			"version": 2,
			"status":  "unverified",
			"error":   "frame structure analysis failed",
		},
	}

	if snapshotRequiresFrameStructureRefresh(snapshot) {
		t.Fatal("versioned unverified frame evidence must remain a useful partial snapshot")
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
