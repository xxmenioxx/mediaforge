package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAssetHasOpenJobBlocksOnlyActiveLifecycle(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:queue-open-job-status?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}); err != nil {
		t.Fatal(err)
	}
	handler := NewQueueHandler(db)
	tests := []struct {
		status      string
		publishedAt *time.Time
		want        bool
	}{
		{status: JobStatusQueued, want: true},
		{status: JobStatusRunning, want: true},
		{status: JobStatusCompleted, want: true},
		{status: JobStatusFailed, want: false},
		{status: JobStatusCanceled, want: false},
	}
	for index, test := range tests {
		path := fmt.Sprintf("/media/asset-%d.mkv", index)
		if err := db.Create(&models.QueueJob{MediaPath: path, Status: test.status, PublishedAt: test.publishedAt}).Error; err != nil {
			t.Fatal(err)
		}
		got, err := handler.assetHasOpenJob(path, 0)
		if err != nil {
			t.Fatal(err)
		}
		if got != test.want {
			t.Errorf("status %s open=%v, want %v", test.status, got, test.want)
		}
	}
	publishedAt := time.Now()
	path := "/media/published.mkv"
	if err := db.Create(&models.QueueJob{MediaPath: path, Status: JobStatusCompleted, PublishedAt: &publishedAt}).Error; err != nil {
		t.Fatal(err)
	}
	if got, err := handler.assetHasOpenJob(path, 0); err != nil || got {
		t.Fatalf("published job open=%v err=%v", got, err)
	}
}

func TestQueueLibraryReplacementRejectsAliasOfActivePublication(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:queue-library-alias?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.Library{}, &models.Profile{}, &models.SchedulerReservation{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	animeRoot := filepath.Join(root, "anime")
	cartoonRoot := filepath.Join(root, "cartoon")
	if err := os.MkdirAll(filepath.Join(animeRoot, "Beck"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(cartoonRoot, "Beck"), 0o755); err != nil {
		t.Fatal(err)
	}
	animePath := filepath.Join(animeRoot, "Beck", "BECK - 01.mkv")
	aliasPath := filepath.Join(cartoonRoot, "Beck", "BECK - 01.mkv")
	if err := os.WriteFile(animePath, []byte("converted"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(animePath, aliasPath); err != nil {
		t.Fatal(err)
	}
	library := models.Library{ID: 7, Name: "Cartoons", SourcePath: filepath.Join(root, "raw"), DestinationPath: cartoonRoot, Type: "cartoon"}
	profile := models.Profile{Name: "Test", VideoCodec: "x265", AudioCodec: "copy"}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	publishedAt := time.Now()
	publication := models.QueueJob{MediaPath: animePath, LibraryID: 1, ProfileID: profile.ID, Status: JobStatusCompleted, PublishedPath: animePath, PublishedAt: &publishedAt}
	if err := db.Create(&publication).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/queue/jobs", NewQueueHandler(db).Create)
	body := `{"mediaPath":"` + aliasPath + `","publishMode":"replace_library_asset","libraryId":7,"profileId":` + strconv.FormatUint(uint64(profile.ID), 10) + `}`
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/queue/jobs", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "physical Library file") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestDismissQueuedPlaceholderDeletesRecordPlanAndReservation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:queue-dismiss?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.ExecutionPlan{}, &models.SchedulerReservation{}); err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: "/media/raw/remove.mkv", LibraryID: 1, ProfileID: 1, Status: JobStatusQueued, Stage: JobStageQueued}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := scheduler.LockQueuedAsset(db, job); err != nil {
		t.Fatal(err)
	}
	plan := models.ExecutionPlan{JobID: job.ID, Version: 1, Status: scheduler.ExecutionPlanPendingEvaluation}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.DELETE("/api/queue/jobs/:id", NewQueueHandler(db).Dismiss)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/queue/jobs/1", nil)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var stored models.QueueJob
	if err := db.First(&stored, job.ID).Error; err == nil {
		t.Fatalf("queued placeholder was preserved: %#v", stored)
	}
	var reservations int64
	if err := db.Model(&models.SchedulerReservation{}).Where("job_id = ?", job.ID).Count(&reservations).Error; err != nil {
		t.Fatal(err)
	}
	if reservations != 0 {
		t.Fatalf("reservations=%d", reservations)
	}
	var plans int64
	if err := db.Model(&models.ExecutionPlan{}).Where("job_id = ?", job.ID).Count(&plans).Error; err != nil {
		t.Fatal(err)
	}
	if plans != 0 {
		t.Fatalf("plans=%d", plans)
	}

	router.GET("/api/queue/jobs", NewQueueHandler(db).List)
	listResponse := httptest.NewRecorder()
	router.ServeHTTP(listResponse, httptest.NewRequest(http.MethodGet, "/api/queue/jobs", nil))
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listResponse.Code, listResponse.Body.String())
	}
	var visible []models.QueueJob
	if err := json.Unmarshal(listResponse.Body.Bytes(), &visible); err != nil {
		t.Fatal(err)
	}
	if len(visible) != 0 {
		t.Fatalf("dismissed jobs remain visible: %#v", visible)
	}
}

func TestReorderQueuedJobsPersistsExactPosition(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:queue-reorder?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}); err != nil {
		t.Fatal(err)
	}
	jobs := []models.QueueJob{
		{MediaPath: "/raw/episode-01.mkv", Status: JobStatusQueued, QueuePosition: 1},
		{MediaPath: "/raw/episode-02.mkv", Status: JobStatusRunning, QueuePosition: 2},
		{MediaPath: "/raw/episode-03.mkv", Status: JobStatusQueued, QueuePosition: 3},
		{MediaPath: "/raw/movie.mkv", Status: JobStatusQueued, QueuePosition: 4},
	}
	if err := db.Create(&jobs).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/queue/jobs/reorder", NewQueueHandler(db).Reorder)
	body := fmt.Sprintf(`{"jobId":%d,"targetJobId":%d,"placement":"before"}`, jobs[3].ID, jobs[2].ID)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/queue/jobs/reorder", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}

	var queued []models.QueueJob
	if err := db.Where("status = ?", JobStatusQueued).Order("queue_position asc").Find(&queued).Error; err != nil {
		t.Fatal(err)
	}
	if len(queued) != 3 || queued[0].ID != jobs[0].ID || queued[1].ID != jobs[3].ID || queued[2].ID != jobs[2].ID {
		t.Fatalf("unexpected queued order: %#v", queued)
	}
	var running models.QueueJob
	if err := db.First(&running, jobs[1].ID).Error; err != nil {
		t.Fatal(err)
	}
	if running.QueuePosition != 2 {
		t.Fatalf("running job position changed: %d", running.QueuePosition)
	}
}

func TestDismissQueueJobRejectsRunningAndCompleted(t *testing.T) {
	for _, status := range []string{JobStatusRunning, JobStatusCompleted} {
		t.Run(status, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open("file:queue-dismiss-"+status+"?mode=memory&cache=shared"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(&models.QueueJob{}, &models.SchedulerReservation{}); err != nil {
				t.Fatal(err)
			}
			job := models.QueueJob{MediaPath: "/media/raw/" + status + ".mkv", LibraryID: 1, ProfileID: 1, Status: status, Stage: status}
			if err := db.Create(&job).Error; err != nil {
				t.Fatal(err)
			}
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.DELETE("/api/queue/jobs/:id", NewQueueHandler(db).Dismiss)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/queue/jobs/1", nil))
			if response.Code != http.StatusConflict {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			var stored models.QueueJob
			if err := db.First(&stored, job.ID).Error; err != nil {
				t.Fatal(err)
			}
			if stored.DismissedAt != nil || stored.Status != status {
				t.Fatalf("protected job changed: %#v", stored)
			}
		})
	}
}

func TestDismissBatchRemovesPlaceholdersAndPreservesCompletedHistory(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:queue-dismiss-batch?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.QueueJob{}, &models.ExecutionPlan{}, &models.SchedulerReservation{}, &models.AppSetting{},
	); err != nil {
		t.Fatal(err)
	}
	started := time.Now().Add(-time.Minute)
	executionNumber := uint(9)
	jobs := []models.QueueJob{
		{BatchID: "digimon", MediaPath: "/raw/e01.mp4", Status: JobStatusQueued, Stage: JobStageQueued},
		{BatchID: "digimon", MediaPath: "/raw/e02.mp4", Status: JobStatusCanceled, Stage: JobStageCanceled, StartedAt: &started, ExecutionNumber: &executionNumber},
		{BatchID: "digimon", MediaPath: "/raw/e03.mp4", Status: JobStatusCompleted, Stage: JobStageCompleted, StartedAt: &started},
	}
	for index := range jobs {
		if err := db.Create(&jobs[index]).Error; err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			if err := scheduler.LockQueuedAsset(db, jobs[index]); err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&models.ExecutionPlan{JobID: jobs[index].ID, Version: 1, Status: scheduler.ExecutionPlanPendingEvaluation}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.DELETE("/api/queue/batches/:batchId", NewQueueHandler(db).DismissBatch)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/queue/batches/digimon", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var placeholderCount int64
	if err := db.Model(&models.QueueJob{}).Where("id = ?", jobs[0].ID).Count(&placeholderCount).Error; err != nil {
		t.Fatal(err)
	}
	if placeholderCount != 0 {
		t.Fatal("queued placeholder was preserved")
	}
	var canceled models.QueueJob
	if err := db.First(&canceled, jobs[1].ID).Error; err != nil {
		t.Fatal(err)
	}
	if canceled.DismissedAt == nil {
		t.Fatal("started canceled job was not dismissed")
	}
	var completed models.QueueJob
	if err := db.First(&completed, jobs[2].ID).Error; err != nil {
		t.Fatal(err)
	}
	if completed.DismissedAt != nil {
		t.Fatal("completed history was dismissed")
	}
}

func TestQueueProfileSnapshotDoesNotChangeWhenProfileChanges(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:queue-snapshot?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Profile{}, &models.QueueJob{}, &models.SchedulerReservation{}); err != nil {
		t.Fatal(err)
	}
	profile := authoritativeTestProfile()
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}

	handler := NewQueueHandler(db)
	job := models.QueueJob{MediaPath: "/media/raw/priority.mkv", LibraryID: 1, Priority: 1, Status: JobStatusQueued}
	if err := handler.captureProfile(&job, profile.ID, "queue_create"); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}

	profile.QualityValue = 24
	profile.ProfileVersion = 2
	if err := db.Save(&profile).Error; err != nil {
		t.Fatal(err)
	}
	var stored models.QueueJob
	if err := db.First(&stored, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.ProfileVersion != 1 || snapshotQualityValue(stored.ProfileSnapshot) != 18 {
		t.Fatalf("existing job snapshot changed after profile edit: %#v", stored.ProfileSnapshot)
	}

	if err := handler.captureProfile(&stored, profile.ID, "queue_profile_update"); err != nil {
		t.Fatal(err)
	}
	if stored.ProfileVersion != 2 || snapshotQualityValue(stored.ProfileSnapshot) != 24 {
		t.Fatalf("explicit refresh did not capture latest profile: %#v", stored.ProfileSnapshot)
	}
}

func TestQueueProfileSnapshotFreezesCurrentInterlaceAnalysis(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:queue-interlace-snapshot?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Profile{}, &models.QueueJob{}, &models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	profile := authoritativeTestProfile()
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	path := "/media/raw/hybrid.mkv"
	analysis := models.JSONMap{
		"version": float64(interlaceAnalysisVersion), "status": "hybrid",
		"recommendedAction": "review", "automaticFilter": "",
		"windows": models.JSONList{
			models.JSONMap{"start": 10.0, "status": "progressive"},
			models.JSONMap{"start": 50.0, "status": "interlaced"},
		},
	}
	cadence := models.JSONMap{"version": float64(cadenceAnalysisVersion), "type": "mixed", "recommendedAction": "review"}
	cadenceRecommendation := models.JSONMap{"version": 1.0, "operation": "review", "confidence": .75, "reason": "mixed samples"}
	if err := db.Create(&models.ScanResult{Path: path, FileName: "hybrid.mkv", InterlaceAnalysis: analysis, CadenceAnalysis: cadence, CadenceRecommendation: cadenceRecommendation}).Error; err != nil {
		t.Fatal(err)
	}

	job := models.QueueJob{MediaPath: path, Status: JobStatusQueued}
	if err := NewQueueHandler(db).captureProfile(&job, profile.ID, "queue_create"); err != nil {
		t.Fatal(err)
	}
	frozen, ok := decodeInterlaceAnalysis(job.ProfileSnapshot[interlaceAnalysisSnapshotKey])
	if !ok || frozen.Status != "hybrid" || len(frozen.Windows) != 2 {
		t.Fatalf("expected immutable v3 motion snapshot, got %#v", job.ProfileSnapshot[interlaceAnalysisSnapshotKey])
	}
	frozenCadence, ok := decodeCadenceAnalysis(job.ProfileSnapshot[cadenceAnalysisSnapshotKey])
	if !ok || frozenCadence.Type != "mixed" {
		t.Fatalf("expected immutable cadence snapshot, got %#v", job.ProfileSnapshot[cadenceAnalysisSnapshotKey])
	}
	if frozenRecommendation, ok := decodeCadenceRecommendation(job.ProfileSnapshot[cadenceRecommendationSnapshotKey]); !ok || frozenRecommendation.Operation != "review" {
		t.Fatalf("expected immutable cadence recommendation, got %#v", job.ProfileSnapshot[cadenceRecommendationSnapshotKey])
	}
}

func TestQueueProfileSnapshotIgnoresGlobalDraftPreferences(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:queue-ignores-draft-preferences?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Profile{}, &models.QueueJob{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	profile := authoritativeTestProfile()
	profile.OptimizationIntent = "maximum_quality"
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "mvforgePreferences", Value: models.JSONMap{
		"qualityGoal": "maximum_savings", "executionPreference": "hardware", "preferredVideoEncoder": "hevc_qsv", "preferredLanguages": models.JSONList{"jpn"},
	}}).Error; err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: "/media/raw/draft-preferences.mkv", LibraryID: 1, Priority: 1, Status: JobStatusQueued}
	if err := NewQueueHandler(db).captureProfile(&job, profile.ID, "queue_create"); err != nil {
		t.Fatal(err)
	}
	restored, err := scheduler.RestoreProfileSnapshot(job.ProfileSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if restored.OptimizationIntent != "maximum_quality" || restored.QualityValue != 18 || restored.PreferredEncoder != "libx265" {
		t.Fatalf("global draft preferences changed the queued profile: %#v", restored)
	}
}

func TestQueueProfileSnapshotUsesAssetProcessingPreference(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:queue-asset-encoder-override?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Profile{}, &models.QueueJob{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	profile := authoritativeTestProfile()
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	path := "/media/raw/hardware-override.mkv"
	enabled := true
	if err := saveAssetConversionOverrides(db, map[string]AssetConversionOverrideState{
		path: {
			PreferredEncoder:       "hardware",
			UseHardwareIfAvailable: &enabled,
			VideoEncoder:           "hevc_qsv",
		},
	}); err != nil {
		t.Fatal(err)
	}

	handler := NewQueueHandler(db)
	job := models.QueueJob{MediaPath: path, LibraryID: 1, Priority: 1, Status: JobStatusQueued}
	if err := handler.captureProfile(&job, profile.ID, "queue_create"); err != nil {
		t.Fatal(err)
	}
	restored, err := scheduler.RestoreProfileSnapshot(job.ProfileSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if restored.PreferredEncoder != "hevc_qsv" || restored.EncoderPolicy != scheduler.EncoderPolicyRestricted {
		t.Fatalf("asset hardware preference was not captured for scheduling: %#v", restored)
	}
}

func TestQueueProfileSnapshotFreezesFrameStructureOverride(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:queue-frame-structure-override?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Profile{}, &models.QueueJob{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	profile := authoritativeTestProfile()
	profile.WorkerConfig["frameStructureGopMode"] = "recommended"
	profile.WorkerConfig["frameStructureGopFrames"] = 120
	profile.WorkerConfig["frameStructureBFrameMode"] = "recommended"
	profile.WorkerConfig["frameStructureMaxBFrames"] = 3
	profile.WorkerConfig["frameStructureMode"] = "balanced"
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	path := "/media/raw/frame-override.mkv"
	if err := saveAssetConversionOverrides(db, map[string]AssetConversionOverrideState{
		path: {FrameStructureMode: "compatible", FrameStructureGOPMode: "custom", FrameStructureGOPFrames: 90, FrameStructureBFrameMode: "off"},
	}); err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: path, LibraryID: 1, Priority: 1, Status: JobStatusQueued}
	if err := NewQueueHandler(db).captureProfile(&job, profile.ID, "queue_create"); err != nil {
		t.Fatal(err)
	}
	restored, err := scheduler.RestoreProfileSnapshot(job.ProfileSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if workerStringValue(restored.WorkerConfig["frameStructureMode"]) != "compatible" || workerStringValue(restored.WorkerConfig["frameStructureGopMode"]) != "custom" || workerIntValue(restored.WorkerConfig["frameStructureGopFrames"], 0) != 90 || workerStringValue(restored.WorkerConfig["frameStructureBFrameMode"]) != "off" {
		t.Fatalf("frame override was not captured in effective profile: %#v", restored.WorkerConfig)
	}
	if err := saveAssetConversionOverrides(db, map[string]AssetConversionOverrideState{
		path: {FrameStructureMode: "maximum_compression", FrameStructureGOPMode: "custom", FrameStructureGOPFrames: 240, FrameStructureBFrameMode: "custom", FrameStructureMaxBFrames: 8},
	}); err != nil {
		t.Fatal(err)
	}
	frozen := conversionOverrideForJob(job, assetConversionOverrides(db))
	if frozen.FrameStructureMode != "compatible" || frozen.FrameStructureGOPFrames != 90 || frozen.FrameStructureBFrameMode != "off" {
		t.Fatalf("queued job used mutable asset override instead of frozen snapshot: %#v", frozen)
	}
}

func TestQueueProfileSnapshotUsesCodecOverrideForEncoderPlanning(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:queue-asset-codec-override?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Profile{}, &models.QueueJob{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	profile := authoritativeTestProfile()
	profile.EncoderPolicy = "automatic"
	profile.PreferredEncoder = "hevc_qsv"
	profile.AllowedEncoders = models.StringList{"hevc_qsv", "libx265"}
	profile.FallbackPolicy = "allowed_only"
	profile.WorkerConfig = models.JSONMap{"videoEncoder": "hevc_qsv", "useHardwareIfAvailable": true}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	path := "/media/raw/h264-override.mkv"
	if err := saveAssetConversionOverrides(db, map[string]AssetConversionOverrideState{
		path: {VideoCodec: "x264", PreferredEncoder: "software"},
	}); err != nil {
		t.Fatal(err)
	}

	handler := NewQueueHandler(db)
	job := models.QueueJob{MediaPath: path, LibraryID: 1, Priority: 1, Status: JobStatusQueued}
	if err := handler.captureProfile(&job, profile.ID, "queue_create"); err != nil {
		t.Fatal(err)
	}
	restored, err := scheduler.RestoreProfileSnapshot(job.ProfileSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if restored.CodecFamily != "h264" || restored.PreferredEncoder != "libx264" {
		t.Fatalf("codec override was not reflected in scheduler snapshot: %#v", restored)
	}
}

func TestQueueProfileSnapshotRejectsDisabledProfile(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:queue-disabled-profile?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Profile{}); err != nil {
		t.Fatal(err)
	}
	profile := authoritativeTestProfile()
	profile.Disabled = true
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}

	err = NewQueueHandler(db).captureProfile(&models.QueueJob{}, profile.ID, "queue_create")
	if err != errQueueProfileDisabled {
		t.Fatalf("expected disabled profile error, got %v", err)
	}
}

func TestQueueProcessingSelectionIsNormalizedAndOverridesAssetMode(t *testing.T) {
	job := models.QueueJob{MediaPath: "/media/raw/movie.mkv", ProcessingMode: "audio_only", TrackProfileKey: "japanese-only"}
	override := conversionOverrideForJob(job, map[string]AssetConversionOverrideState{
		job.MediaPath: {ProcessingMode: ProcessingModeFullEncode},
	})
	if override.ProcessingMode != ProcessingModeAudioOnly {
		t.Fatalf("processing mode=%q", override.ProcessingMode)
	}
	if normalized := normalizeQueueProcessingMode("full-encode"); normalized != ProcessingModeFullEncode {
		t.Fatalf("normalized mode=%q", normalized)
	}
}

func authoritativeTestProfile() models.Profile {
	return models.Profile{
		Name: "Priority Archive", Container: "mkv", VideoCodec: "x265_10bit", CodecFamily: "hevc",
		EncoderPolicy: "locked", PreferredEncoder: "libx265", AllowedEncoders: models.StringList{"libx265"},
		FallbackPolicy: "wait", BitDepth: 10, PixelFormat: "yuv420p10le", QualityStrategy: "high",
		ProfileVersion: 1, AudioCodec: "copy", QualityMode: "crf", QualityValue: 18,
		WorkerConfig: models.JSONMap{"videoEncoder": "libx265", "videoPreset": "slow"},
	}
}

func snapshotQualityValue(snapshot models.JSONMap) int {
	constraints, _ := snapshot["constraints"].(map[string]any)
	value, _ := constraints["qualityValue"].(float64)
	return int(value)
}

func TestQueueOverrideOnlyUsesAssetVideoOverrideWithoutPathProfileInheritance(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open("file:queue-override-only-video?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.AutoMigrate(
		&models.Profile{},
		&models.QueueJob{},
		&models.AppSetting{},
	); err != nil {
		t.Fatal(err)
	}

	// This represents the profile that would normally come from the path.
	// Its QSV quality must NOT leak into override-only mode.
	pathProfile := authoritativeTestProfile()
	pathProfile.Name = "Path QSV Profile"
	pathProfile.WorkerConfig["videoEncoder"] = "hevc_qsv"
	pathProfile.WorkerConfig["globalQuality"] = 26

	if err := db.Create(&pathProfile).Error; err != nil {
		t.Fatal(err)
	}

	mediaPath := "/media/raw/anime/example.mkv"

	if err := saveAssetConversionOverrides(
		db,
		map[string]AssetConversionOverrideState{
			mediaPath: {
				VideoEncoder:  "hevc_qsv",
				GlobalQuality: 20,
			},
		},
	); err != nil {
		t.Fatal(err)
	}

	handler := NewQueueHandler(db)

	job := models.QueueJob{
		MediaPath:      mediaPath,
		LibraryID:      1,
		Status:         JobStatusQueued,
		ProcessingMode: ProcessingModeFullEncode,
		ProfileID:      0,
		ProfileResolution: models.JSONMap{
			"video": models.JSONMap{
				"selection":    VideoAssignmentOverrideOnly,
				"overrideOnly": true,
			},
		},
	}

	if err := handler.captureOverrideOnlyProfile(
		&job,
		"queue_create_override_only",
	); err != nil {
		t.Fatal(err)
	}

	if job.ProcessingMode != ProcessingModeFullEncode {
		t.Fatalf(
			"processing mode=%q want=%q",
			job.ProcessingMode,
			ProcessingModeFullEncode,
		)
	}

	if job.ProfileID != 0 {
		t.Fatalf(
			"profile id=%d want=0",
			job.ProfileID,
		)
	}

	if !queueUsesOverrideOnlyVideo(job.ProfileResolution) {
		t.Fatal("expected override-only video resolution")
	}

	restored, err := scheduler.RestoreProfileSnapshot(job.ProfileSnapshot)
	if err != nil {
		t.Fatal(err)
	}

	if restored.Name != "Asset Override Only" {
		t.Fatalf(
			"profile name=%q want=%q",
			restored.Name,
			"Asset Override Only",
		)
	}

	if workerStringValue(restored.WorkerConfig["videoEncoder"]) != "hevc_qsv" {
		t.Fatalf(
			"video encoder=%q want=%q",
			workerStringValue(restored.WorkerConfig["videoEncoder"]),
			"hevc_qsv",
		)
	}

	if workerIntValue(restored.WorkerConfig["globalQuality"], 0) != 20 {
		t.Fatalf(
			"global quality=%d want=20",
			workerIntValue(restored.WorkerConfig["globalQuality"], 0),
		)
	}

	if workerIntValue(restored.WorkerConfig["globalQuality"], 0) == 26 {
		t.Fatal("override-only profile inherited globalQuality=26 from path profile")
	}

	rawOverride, ok := job.ProfileSnapshot[assetConversionOverrideSnapshotKey]
	if !ok {
		t.Fatal("asset conversion override was not frozen into profile snapshot")
	}

	if rawOverride == nil {
		t.Fatal("frozen asset conversion override is nil")
	}
}

func TestQueuedBatchJobsOrderByBatchPosition(t *testing.T) {
	db := queueJobTestDB(t)

	jobs := []models.QueueJob{
		{
			MediaPath:     "/media/raw/Anime/Season 01/title_02.mkv",
			BatchID:       "season-01",
			BatchName:     "Anime/Season 01",
			BatchPosition: 3,
			Status:        JobStatusQueued,
			QueuePosition: 1,
		},
		{
			MediaPath:     "/media/raw/Anime/Season 01/title.mkv",
			BatchID:       "season-01",
			BatchName:     "Anime/Season 01",
			BatchPosition: 1,
			Status:        JobStatusQueued,
			QueuePosition: 2,
		},
		{
			MediaPath:     "/media/raw/Anime/Season 01/title_01.mkv",
			BatchID:       "season-01",
			BatchName:     "Anime/Season 01",
			BatchPosition: 2,
			Status:        JobStatusQueued,
			QueuePosition: 3,
		},
	}

	for index := range jobs {
		if err := db.Create(&jobs[index]).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := normalizeQueuedBatchOrder(db, "season-01"); err != nil {
		t.Fatal(err)
	}

	var stored []models.QueueJob
	if err := db.
		Where("batch_id = ?", "season-01").
		Order("queue_position asc").
		Find(&stored).Error; err != nil {
		t.Fatal(err)
	}

	expected := []string{
		"title.mkv",
		"title_01.mkv",
		"title_02.mkv",
	}

	if len(stored) != len(expected) {
		t.Fatalf("got %d jobs, want %d", len(stored), len(expected))
	}

	for index := range stored {
		if filepath.Base(stored[index].MediaPath) != expected[index] {
			t.Fatalf(
				"queue position %d = %q, want %q",
				index+1,
				filepath.Base(stored[index].MediaPath),
				expected[index],
			)
		}

		if stored[index].BatchPosition != index+1 {
			t.Fatalf(
				"job %q batchPosition = %d, want %d",
				stored[index].MediaPath,
				stored[index].BatchPosition,
				index+1,
			)
		}
	}
}

func TestNormalizeQueuedBatchOrderPreservesGlobalBatchOrder(t *testing.T) {
	db := queueJobTestDB(t)

	// Batch A ya estaba correctamente en Queue.
	batchA := []models.QueueJob{
		{
			MediaPath:     "/media/raw/Anime/Season 01/A01.mkv",
			BatchID:       "batch-a",
			BatchName:     "Anime/Season 01",
			BatchPosition: 1,
			Status:        JobStatusQueued,
			QueuePosition: 1,
		},
		{
			MediaPath:     "/media/raw/Anime/Season 01/A02.mkv",
			BatchID:       "batch-a",
			BatchName:     "Anime/Season 01",
			BatchPosition: 2,
			Status:        JobStatusQueued,
			QueuePosition: 2,
		},
		{
			MediaPath:     "/media/raw/Anime/Season 01/A03.mkv",
			BatchID:       "batch-a",
			BatchName:     "Anime/Season 01",
			BatchPosition: 3,
			Status:        JobStatusQueued,
			QueuePosition: 3,
		},
	}

	for index := range batchA {
		if err := db.Create(&batchA[index]).Error; err != nil {
			t.Fatal(err)
		}
	}

	// Batch B llegó después, por lo que ocupa globalmente 4..6,
	// pero sus assets quedaron internamente desordenados.
	batchB := []models.QueueJob{
		{
			MediaPath:     "/media/raw/Anime/Season 02/title_02.mkv",
			BatchID:       "batch-b",
			BatchName:     "Anime/Season 02",
			BatchPosition: 3,
			Status:        JobStatusQueued,
			QueuePosition: 4,
		},
		{
			MediaPath:     "/media/raw/Anime/Season 02/title.mkv",
			BatchID:       "batch-b",
			BatchName:     "Anime/Season 02",
			BatchPosition: 1,
			Status:        JobStatusQueued,
			QueuePosition: 5,
		},
		{
			MediaPath:     "/media/raw/Anime/Season 02/title_01.mkv",
			BatchID:       "batch-b",
			BatchName:     "Anime/Season 02",
			BatchPosition: 2,
			Status:        JobStatusQueued,
			QueuePosition: 6,
		},
	}

	for index := range batchB {
		if err := db.Create(&batchB[index]).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := normalizeQueuedBatchOrder(db, "batch-b"); err != nil {
		t.Fatal(err)
	}

	var stored []models.QueueJob
	if err := db.
		Where("status = ?", JobStatusQueued).
		Order("queue_position asc").
		Find(&stored).Error; err != nil {
		t.Fatal(err)
	}

	expected := []struct {
		file          string
		batchID       string
		batchPosition int
		queuePosition int64
	}{
		{"A01.mkv", "batch-a", 1, 1},
		{"A02.mkv", "batch-a", 2, 2},
		{"A03.mkv", "batch-a", 3, 3},

		{"title.mkv", "batch-b", 1, 4},
		{"title_01.mkv", "batch-b", 2, 5},
		{"title_02.mkv", "batch-b", 3, 6},
	}

	if len(stored) != len(expected) {
		t.Fatalf("got %d queued jobs, want %d", len(stored), len(expected))
	}

	for index, want := range expected {
		got := stored[index]

		if filepath.Base(got.MediaPath) != want.file {
			t.Fatalf(
				"queue position %d file = %q, want %q",
				index+1,
				filepath.Base(got.MediaPath),
				want.file,
			)
		}

		if got.BatchID != want.batchID {
			t.Fatalf(
				"job %q batchID = %q, want %q",
				got.MediaPath,
				got.BatchID,
				want.batchID,
			)
		}

		if got.BatchPosition != want.batchPosition {
			t.Fatalf(
				"job %q batchPosition = %d, want %d",
				got.MediaPath,
				got.BatchPosition,
				want.batchPosition,
			)
		}

		if got.QueuePosition != want.queuePosition {
			t.Fatalf(
				"job %q queuePosition = %d, want %d",
				got.MediaPath,
				got.QueuePosition,
				want.queuePosition,
			)
		}
	}
}

func TestQueueCreatePreservesBatchOrderAndNormalizesAssetsWithinBatch(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open("file:queue-create-batch-order?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.AutoMigrate(
		&models.Profile{},
		&models.QueueJob{},
		&models.ExecutionPlan{},
		&models.SchedulerReservation{},
		&models.AppSetting{},
	); err != nil {
		t.Fatal(err)
	}

	profile := authoritativeTestProfile()
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.POST("/api/queue/jobs", NewQueueHandler(db).Create)

	createJob := func(
		mediaPath string,
		batchID string,
		batchName string,
		batchPosition int,
	) models.QueueJob {
		t.Helper()

		body := fmt.Sprintf(
			`{
				"mediaPath":%q,
				"libraryId":1,
				"profileId":%d,
				"batchId":%q,
				"batchName":%q,
				"batchPosition":%d
			}`,
			mediaPath,
			profile.ID,
			batchID,
			batchName,
			batchPosition,
		)

		response := httptest.NewRecorder()
		request := httptest.NewRequest(
			http.MethodPost,
			"/api/queue/jobs",
			strings.NewReader(body),
		)
		request.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(response, request)

		if response.Code != http.StatusCreated {
			t.Fatalf(
				"create %q status=%d body=%s",
				mediaPath,
				response.Code,
				response.Body.String(),
			)
		}

		var job models.QueueJob
		if err := json.Unmarshal(response.Body.Bytes(), &job); err != nil {
			t.Fatal(err)
		}

		return job
	}

	// Batch A arrives first and is already internally ordered.
	createJob(
		"/media/raw/Anime/Season 01/A01.mkv",
		"batch-a",
		"Anime/Season 01",
		1,
	)
	createJob(
		"/media/raw/Anime/Season 01/A02.mkv",
		"batch-a",
		"Anime/Season 01",
		2,
	)
	createJob(
		"/media/raw/Anime/Season 01/A03.mkv",
		"batch-a",
		"Anime/Season 01",
		3,
	)

	// Batch B arrives AFTER Batch A, but its HTTP requests/assets arrive
	// internally out of sequence: 3, 1, 2.
	createJob(
		"/media/raw/Anime/Season 02/title_02.mkv",
		"batch-b",
		"Anime/Season 02",
		3,
	)
	createJob(
		"/media/raw/Anime/Season 02/title.mkv",
		"batch-b",
		"Anime/Season 02",
		1,
	)
	createJob(
		"/media/raw/Anime/Season 02/title_01.mkv",
		"batch-b",
		"Anime/Season 02",
		2,
	)

	var queued []models.QueueJob
	if err := db.
		Where("status = ? AND dismissed_at IS NULL", JobStatusQueued).
		Order("queue_position asc").
		Find(&queued).Error; err != nil {
		t.Fatal(err)
	}

	expected := []struct {
		file          string
		batchID       string
		batchPosition int
		queuePosition int64
	}{
		{"A01.mkv", "batch-a", 1, 1},
		{"A02.mkv", "batch-a", 2, 2},
		{"A03.mkv", "batch-a", 3, 3},

		{"title.mkv", "batch-b", 1, 4},
		{"title_01.mkv", "batch-b", 2, 5},
		{"title_02.mkv", "batch-b", 3, 6},
	}

	if len(queued) != len(expected) {
		t.Fatalf(
			"queued jobs=%d want=%d: %#v",
			len(queued),
			len(expected),
			queued,
		)
	}

	for index, want := range expected {
		got := queued[index]

		if filepath.Base(got.MediaPath) != want.file {
			t.Fatalf(
				"queue position %d file=%q want=%q",
				index+1,
				filepath.Base(got.MediaPath),
				want.file,
			)
		}

		if got.BatchID != want.batchID {
			t.Fatalf(
				"%q batchID=%q want=%q",
				got.MediaPath,
				got.BatchID,
				want.batchID,
			)
		}

		if got.BatchPosition != want.batchPosition {
			t.Fatalf(
				"%q batchPosition=%d want=%d",
				got.MediaPath,
				got.BatchPosition,
				want.batchPosition,
			)
		}

		if got.QueuePosition != want.queuePosition {
			t.Fatalf(
				"%q queuePosition=%d want=%d",
				got.MediaPath,
				got.QueuePosition,
				want.queuePosition,
			)
		}
	}
}

func TestQueueCreateBatchCreatesWholeBatchInNaturalOrder(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open("file:queue-create-atomic-batch?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.AutoMigrate(
		&models.Profile{},
		&models.QueueJob{},
		&models.ExecutionPlan{},
		&models.SchedulerReservation{},
		&models.AppSetting{},
	); err != nil {
		t.Fatal(err)
	}

	profile := authoritativeTestProfile()
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}

	// Existing queue content must remain before the new batch.
	existing := models.QueueJob{
		MediaPath:     "/media/raw/Movies/existing.mkv",
		Status:        JobStatusQueued,
		Stage:         JobStageQueued,
		QueuePosition: 1,
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.POST("/api/queue/batches", NewQueueHandler(db).CreateBatch)

	body := fmt.Sprintf(
		`{
			"batchId":"batch-b",
			"batchName":"Anime/Season 02",
			"jobs":[
				{
					"mediaPath":"/media/raw/Anime/Season 02/title_02.mkv",
					"libraryId":1,
					"profileId":%d,
					"priority":5
				},
				{
					"mediaPath":"/media/raw/Anime/Season 02/title.mkv",
					"libraryId":1,
					"profileId":%d,
					"priority":5
				},
				{
					"mediaPath":"/media/raw/Anime/Season 02/title_01.mkv",
					"libraryId":1,
					"profileId":%d,
					"priority":5
				}
			]
		}`,
		profile.ID,
		profile.ID,
		profile.ID,
	)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/queue/batches",
		strings.NewReader(body),
	)
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf(
			"status=%d body=%s",
			response.Code,
			response.Body.String(),
		)
	}

	var queued []models.QueueJob
	if err := db.
		Where("dismissed_at IS NULL").
		Order("queue_position asc").
		Find(&queued).Error; err != nil {
		t.Fatal(err)
	}

	if len(queued) != 4 {
		t.Fatalf("queued jobs=%d want=4: %#v", len(queued), queued)
	}

	expected := []struct {
		file          string
		batchID       string
		batchPosition int
		queuePosition int64
	}{
		{"existing.mkv", "", 0, 1},
		{"title.mkv", "batch-b", 1, 2},
		{"title_01.mkv", "batch-b", 2, 3},
		{"title_02.mkv", "batch-b", 3, 4},
	}

	for index, want := range expected {
		got := queued[index]

		if filepath.Base(got.MediaPath) != want.file {
			t.Fatalf(
				"queue position %d file=%q want=%q",
				index+1,
				filepath.Base(got.MediaPath),
				want.file,
			)
		}

		if got.BatchID != want.batchID {
			t.Fatalf(
				"%q batchID=%q want=%q",
				got.MediaPath,
				got.BatchID,
				want.batchID,
			)
		}

		if got.BatchPosition != want.batchPosition {
			t.Fatalf(
				"%q batchPosition=%d want=%d",
				got.MediaPath,
				got.BatchPosition,
				want.batchPosition,
			)
		}

		if got.QueuePosition != want.queuePosition {
			t.Fatalf(
				"%q queuePosition=%d want=%d",
				got.MediaPath,
				got.QueuePosition,
				want.queuePosition,
			)
		}
	}
}

func TestQueueCreateBatchRollsBackEntireBatchWhenOneAssetReservationFails(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open("file:queue-create-atomic-rollback?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.AutoMigrate(
		&models.Profile{},
		&models.QueueJob{},
		&models.ExecutionPlan{},
		&models.SchedulerReservation{},
		&models.AppSetting{},
	); err != nil {
		t.Fatal(err)
	}

	profile := authoritativeTestProfile()
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}

	conflictingPath := filepath.Clean(
		"/media/raw/Anime/Season 02/title_01.mkv",
	)

	// Deliberately create only a scheduler reservation, not a QueueJob.
	//
	// This means prepareBatchQueueJob() will not reject the asset early.
	// The conflict will happen inside CreateBatch's transaction when
	// LockQueuedAsset() reaches this asset.
	existingReservation := models.SchedulerReservation{
		JobID:    999999,
		AssetKey: conflictingPath,
		State:    scheduler.ReservationStateLocked,
	}

	if err := db.Create(&existingReservation).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.POST("/api/queue/batches", NewQueueHandler(db).CreateBatch)

	body := fmt.Sprintf(
		`{
			"batchId":"rollback-batch",
			"batchName":"Anime/Season 02",
			"jobs":[
				{
					"mediaPath":"/media/raw/Anime/Season 02/title_02.mkv",
					"libraryId":1,
					"profileId":%d,
					"priority":5
				},
				{
					"mediaPath":"/media/raw/Anime/Season 02/title.mkv",
					"libraryId":1,
					"profileId":%d,
					"priority":5
				},
				{
					"mediaPath":"/media/raw/Anime/Season 02/title_01.mkv",
					"libraryId":1,
					"profileId":%d,
					"priority":5
				}
			]
		}`,
		profile.ID,
		profile.ID,
		profile.ID,
	)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/queue/batches",
		strings.NewReader(body),
	)
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf(
			"status=%d want=%d body=%s",
			response.Code,
			http.StatusConflict,
			response.Body.String(),
		)
	}

	// The first naturally ordered asset (title.mkv) is created before
	// title_01.mkv hits the reservation conflict. If the DB transaction
	// is truly atomic, that first QueueJob must also disappear.
	var batchJobCount int64
	if err := db.
		Model(&models.QueueJob{}).
		Where("batch_id = ?", "rollback-batch").
		Count(&batchJobCount).
		Error; err != nil {
		t.Fatal(err)
	}

	if batchJobCount != 0 {
		t.Fatalf(
			"atomic batch rollback left %d QueueJobs behind",
			batchJobCount,
		)
	}

	// Execution plans created before the conflicting asset must roll
	// back together with their QueueJobs.
	var planCount int64
	if err := db.
		Model(&models.ExecutionPlan{}).
		Count(&planCount).
		Error; err != nil {
		t.Fatal(err)
	}

	if planCount != 0 {
		t.Fatalf(
			"atomic batch rollback left %d execution plans behind",
			planCount,
		)
	}

	// The only reservation that should remain is the deliberately
	// pre-existing conflicting one. Any lock created for title.mkv
	// inside the failed transaction must have been rolled back.
	var reservations []models.SchedulerReservation
	if err := db.
		Order("id asc").
		Find(&reservations).
		Error; err != nil {
		t.Fatal(err)
	}

	if len(reservations) != 1 {
		t.Fatalf(
			"reservations=%d want=1: %#v",
			len(reservations),
			reservations,
		)
	}

	if reservations[0].ID != existingReservation.ID {
		t.Fatalf(
			"unexpected reservation survived rollback: %#v",
			reservations[0],
		)
	}
}
