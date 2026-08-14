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
