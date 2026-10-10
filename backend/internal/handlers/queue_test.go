package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
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

func TestQueueFreezesResolvedGOPAndWorkerPreservesIt(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:queue-gop-v2-freeze?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	path := "/media/raw/anime/short-gop.mkv"
	scan := models.ScanResult{
		Path: path,
		VideoStreams: models.JSONList{
			map[string]any{"avgFrameRate": "24000/1001", "width": 720, "height": 480},
		},
		FrameStructureAnalysis: models.JSONMap{
			"framesAnalyzed":   240,
			"averageGopLength": 14.9,
			"medianGopLength":  15.0,
			"p25GopLength":     15.0,
			"p75GopLength":     15.0,
			"completeGops":     8,
			"variability":      "low",
			"confidence":       "high",
		},
	}
	if err := db.Create(&scan).Error; err != nil {
		t.Fatal(err)
	}
	profile := models.Profile{Name: "Auto GOP", VideoCodec: "hevc", WorkerConfig: models.JSONMap{
		"frameStructureMode":       "auto",
		"frameStructureGopMode":    "auto",
		"effectiveOutputFrameRate": "24000/1001",
	}}
	snapshot, err := scheduler.CaptureProfileSnapshot(profile, time.Now(), "test")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewQueueHandler(db)
	if err := handler.captureInterlaceSnapshot(path, snapshot); err != nil {
		t.Fatal(err)
	}
	frozen, err := scheduler.RestoreProfileSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !profileWorkerBool(frozen, "frameStructureGopFrozen", false) {
		t.Fatalf("Queue snapshot did not mark GOP frozen: %#v", frozen.WorkerConfig)
	}
	if got := workerIntValue(frozen.WorkerConfig["frameStructureGopFrames"], 0); got != 19 {
		t.Fatalf("frozen GOP=%d want 19: %#v", got, frozen.WorkerConfig)
	}

	if err := db.Model(&scan).Update("frame_structure_analysis", models.JSONMap{
		"averageGopLength": 96.0,
		"medianGopLength":  96.0,
		"completeGops":     8,
		"variability":      "low",
		"confidence":       "high",
	}).Error; err != nil {
		t.Fatal(err)
	}
	frozen.WorkerConfig["frameStructureGopStrategy"] = "maximum_compression"
	resolved, err := resolveAutomaticFrameStructure(db, path, frozen)
	if err != nil {
		t.Fatal(err)
	}
	if got := workerIntValue(resolved.WorkerConfig["frameStructureGopFrames"], 0); got != 19 {
		t.Fatalf("Worker recalculated frozen GOP, got %d", got)
	}

	customProfile := models.Profile{Name: "Custom GOP", VideoCodec: "hevc", WorkerConfig: models.JSONMap{
		"frameStructureMode":        "custom",
		"frameStructureGopMode":     "custom",
		"frameStructureGopStrategy": "custom",
		"frameStructureGopFrames":   48,
	}}
	customSnapshot, err := scheduler.CaptureProfileSnapshot(customProfile, time.Now(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := handler.captureInterlaceSnapshot(path, customSnapshot); err != nil {
		t.Fatal(err)
	}
	customFrozen, err := scheduler.RestoreProfileSnapshot(customSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if got := workerIntValue(customFrozen.WorkerConfig["frameStructureGopFrames"], 0); got != 48 || !profileWorkerBool(customFrozen, "frameStructureGopFrozen", false) {
		t.Fatalf("custom Queue GOP was not frozen exactly: %#v", customFrozen.WorkerConfig)
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

func TestDismissBatchRemovesFullyCanceledJobsAndPreservesHistory(t *testing.T) {
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
		{BatchID: "digimon", MediaPath: "/raw/e01.mp4", Status: JobStatusCanceled, Stage: JobStageCanceled},
		{BatchID: "digimon", MediaPath: "/raw/e02.mp4", Status: JobStatusCanceled, Stage: JobStageCanceled, StartedAt: &started, ExecutionNumber: &executionNumber, Notes: "audit trail"},
	}
	for index := range jobs {
		if err := db.Create(&jobs[index]).Error; err != nil {
			t.Fatal(err)
		}
		if err := scheduler.LockQueuedAsset(db, jobs[index]); err != nil {
			t.Fatal(err)
		}
		if index == 0 {
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
	var responseBody struct {
		BatchID               string `json:"batchId"`
		DismissedJobs         int    `json:"dismissedJobs"`
		CleanedExecutionPlans int    `json:"cleanedExecutionPlans"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &responseBody); err != nil {
		t.Fatal(err)
	}
	if responseBody.BatchID != "digimon" || responseBody.DismissedJobs != 2 || responseBody.CleanedExecutionPlans != 1 {
		t.Fatalf("unexpected batch cleanup response: %#v", responseBody)
	}
	for index := range jobs {
		var stored models.QueueJob
		if err := db.First(&stored, jobs[index].ID).Error; err != nil {
			t.Fatal(err)
		}
		if stored.Status != JobStatusCanceled || stored.DismissedAt == nil {
			t.Fatalf("cancelled history was not preserved and dismissed: %#v", stored)
		}
	}
	var canceledHistory models.QueueJob
	if err := db.First(&canceledHistory, jobs[1].ID).Error; err != nil {
		t.Fatal(err)
	}
	if canceledHistory.ExecutionNumber == nil || canceledHistory.StartedAt == nil || canceledHistory.Notes != "audit trail" {
		t.Fatalf("cancelled audit fields were not preserved: %#v", canceledHistory)
	}

	var plans int64
	if err := db.Model(&models.ExecutionPlan{}).Where("job_id = ?", jobs[0].ID).Count(&plans).Error; err != nil {
		t.Fatal(err)
	}
	if plans != 0 {
		t.Fatalf("cancelled placeholder plan was preserved: %d", plans)
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
		t.Fatalf("dismissed jobs remain in active Queue: %#v", visible)
	}
}

func TestDismissBatchRejectsNonCanceledWithoutMutation(t *testing.T) {
	for _, test := range []struct {
		name     string
		statuses []string
	}{
		{name: "canceled-completed", statuses: []string{JobStatusCanceled, JobStatusCompleted}},
		{name: "canceled-failed", statuses: []string{JobStatusCanceled, JobStatusFailed}},
		{name: "failed-only", statuses: []string{JobStatusFailed}},
		{name: "canceled-queued", statuses: []string{JobStatusCanceled, JobStatusQueued}},
		{name: "canceled-running", statuses: []string{JobStatusCanceled, JobStatusRunning}},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open("file:queue-dismiss-batch-reject-"+test.name+"?mode=memory&cache=shared"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(&models.QueueJob{}, &models.SchedulerReservation{}); err != nil {
				t.Fatal(err)
			}
			jobs := make([]models.QueueJob, 0, len(test.statuses))
			for index, status := range test.statuses {
				job := models.QueueJob{BatchID: "protected", MediaPath: fmt.Sprintf("/raw/%s-%d.mkv", test.name, index), Status: status, Stage: status}
				if status == JobStatusRunning {
					startedAt := time.Now().Add(-time.Minute)
					execution := uint(12)
					job.StartedAt = &startedAt
					job.ExecutionNumber = &execution
				}
				jobs = append(jobs, job)
			}
			for index := range jobs {
				job := &jobs[index]
				if err := db.Create(job).Error; err != nil {
					t.Fatal(err)
				}
				if err := scheduler.LockQueuedAsset(db, *job); err != nil {
					t.Fatal(err)
				}
			}

			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.DELETE("/api/queue/batches/:batchId", NewQueueHandler(db).DismissBatch)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/queue/batches/protected", nil))
			if response.Code != http.StatusConflict {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}

			var stored []models.QueueJob
			if err := db.Where("batch_id = ?", "protected").Order("id asc").Find(&stored).Error; err != nil {
				t.Fatal(err)
			}
			if len(stored) != len(test.statuses) {
				t.Fatalf("rejected batch was partially mutated: %#v", stored)
			}
			for index := range stored {
				if stored[index].DismissedAt != nil || stored[index].Status != test.statuses[index] {
					t.Fatalf("rejected batch was partially mutated: %#v", stored)
				}
				if test.statuses[index] == JobStatusRunning && stored[index].FinishedAt != nil {
					t.Fatalf("running job was changed while rejecting batch: %#v", stored[index])
				}
			}
			jobIDs := make([]uint, 0, len(jobs))
			for index := range jobs {
				jobIDs = append(jobIDs, jobs[index].ID)
			}
			var reservations int64
			if err := db.Model(&models.SchedulerReservation{}).Where("job_id IN ?", jobIDs).Count(&reservations).Error; err != nil {
				t.Fatal(err)
			}
			if reservations != int64(len(jobs)) {
				t.Fatalf("rejected batch released reservations: %d", reservations)
			}
		})
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

func TestQueuePreparedProfileFreezesOnlyConsumedAnalysisIdentity(t *testing.T) {
	db := queueSelectedAssetsTestDB(t)
	path := filepath.Join(t.TempDir(), "frozen-evidence.mkv")
	if err := os.WriteFile(path, []byte("queue source"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	record := models.AssetRecord{Path: path, SourcePath: path, FileName: filepath.Base(path), Status: "unprocessed", SizeBytes: info.Size(), ModifiedAt: info.ModTime()}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	scan := completeAnalysisSnapshot(path, info)
	stampSnapshotCacheMetadata(&scan, path, info)
	if err := db.Create(&scan).Error; err != nil {
		t.Fatal(err)
	}
	profile := authoritativeTestProfile()
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	initialSnapshot, err := scheduler.CaptureProfileSnapshot(profile, time.Now(), "queue_test")
	if err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: path, SourceAssetRecordID: &record.ID, ProfileID: profile.ID, Status: JobStatusQueued, ProfileSnapshot: initialSnapshot}
	if err := NewQueueHandler(db).captureInterlaceSnapshot(path, job.ProfileSnapshot); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := freezeQueuePreparedProfileSnapshot(db, &job, profile); err != nil {
		t.Fatal(err)
	}
	var frozen models.QueueJob
	if err := db.First(&frozen, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	manifest, ok := snapshotCacheMap(frozen.ProfileSnapshot[sourceAnalysisEvidenceSnapshotKey])
	if !ok || manifest["status"] != assetAnalysisReady || uint(jsonInt64(manifest["sourceAssetRecordId"])) != record.ID {
		t.Fatalf("Queue did not freeze canonical evidence identity with the asset record: %#v", frozen.ProfileSnapshot[sourceAnalysisEvidenceSnapshotKey])
	}
	components, ok := manifest["components"].([]any)
	if !ok {
		if typed, typedOK := manifest["components"].([]string); typedOK {
			components = make([]any, len(typed))
			for index, component := range typed {
				components[index] = component
			}
		} else {
			t.Fatalf("frozen evidence components have unexpected representation: %#v", manifest["components"])
		}
	}
	for _, value := range components {
		if value == "hdr" || value == "audio" || value == "subtitles" {
			t.Fatalf("unused evidence was copied into the frozen manifest: %#v", components)
		}
	}
	frozenManifest, err := json.Marshal(frozen.ProfileSnapshot[sourceAnalysisEvidenceSnapshotKey])
	if err != nil {
		t.Fatal(err)
	}

	scan.CadenceAnalysis = models.JSONMap{"status": "changed_after_freeze"}
	if err := db.Save(&scan).Error; err != nil {
		t.Fatal(err)
	}
	profile.ProfileVersion++
	profile.WorkerConfig["crf"] = 99
	if err := db.Save(&profile).Error; err != nil {
		t.Fatal(err)
	}
	var after models.QueueJob
	if err := db.First(&after, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	afterManifest, err := json.Marshal(after.ProfileSnapshot[sourceAnalysisEvidenceSnapshotKey])
	if err != nil {
		t.Fatal(err)
	}
	if string(afterManifest) != string(frozenManifest) || after.ProfileVersion != frozen.ProfileVersion || !reflect.DeepEqual(after.ProfileSnapshot, frozen.ProfileSnapshot) {
		t.Fatalf("post-freeze evidence/settings mutation rewrote the Queue snapshot: before=%#v after=%#v", frozen.ProfileSnapshot, after.ProfileSnapshot)
	}
}

func TestQueuePreparedProfilePreservesUnavailableMissingAssetEvidence(t *testing.T) {
	db := queueSelectedAssetsTestDB(t)
	profile := authoritativeTestProfile()
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	record := models.AssetRecord{Path: "/media/raw/missing.mkv", FileName: "missing.mkv", Status: "unprocessed", Missing: true}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	initialSnapshot, err := scheduler.CaptureProfileSnapshot(profile, time.Now(), "queue_test")
	if err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: record.Path, SourceAssetRecordID: &record.ID, ProfileID: profile.ID, Status: JobStatusQueued, ProfileSnapshot: initialSnapshot}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := freezeQueuePreparedProfileSnapshot(db, &job, profile); err != nil {
		t.Fatal(err)
	}
	manifest, ok := snapshotCacheMap(job.ProfileSnapshot[sourceAnalysisEvidenceSnapshotKey])
	if !ok || manifest["status"] != assetAnalysisUnavailable || manifest["unavailableReason"] != "asset_missing" {
		t.Fatalf("missing asset evidence was incorrectly presented as absent-but-available: %#v", job.ProfileSnapshot[sourceAnalysisEvidenceSnapshotKey])
	}
}

func TestQueuePreparingFreezesHEVCDecisionBeforeProfileMutation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:queue-hevc-preparing-freeze?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Profile{}, &models.QueueJob{}, &models.SchedulerReservation{}); err != nil {
		t.Fatal(err)
	}
	profile := models.Profile{
		Name: "HEVC Queue Freeze", Container: "mkv", VideoCodec: "hevc", CodecFamily: "hevc",
		EncoderPolicy: "locked", PreferredEncoder: "libx265", AllowedEncoders: models.StringList{"libx265"},
		FallbackPolicy: "wait", BitDepth: 8, PixelFormat: "yuv420p", QualityStrategy: "high",
		ProfileVersion: 1, AudioCodec: "copy", QualityMode: "crf", QualityValue: 20,
		WorkerConfig: models.JSONMap{
			"videoEncoder": "libx265", "hevcLevelMode": "auto",
			"x265Params": "vbv-maxrate=10000:vbv-bufsize=20000",
		},
	}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	path := "/media/raw/queue-hevc-freeze.mkv"
	job := models.QueueJob{MediaPath: path, LibraryID: 1, ProfileID: profile.ID, ProcessingMode: ProcessingModeFullEncode, Status: JobStatusQueued}
	if err := NewQueueHandler(db).captureProfile(&job, profile.ID, "queue_create"); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}

	streams := MediaStreamInventory{Video: []MediaStream{{Index: 0, Width: 1920, Height: 1080, FrameRate: "24/1", PixelFormat: "yuv420p"}}}
	requested, err := scheduler.RestoreProfileSnapshot(job.ProfileSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	resolved := resolveMediaJobVideoProfile(requested, streams, path, InterlaceAnalysis{Status: "progressive"}, CadenceAnalysis{Type: "native"}, CadenceRecommendation{Operation: "preserve"})
	if workerStringValue(resolved.WorkerConfig["hevcLevelEffective"]) != "4.0" || workerStringValue(resolved.WorkerConfig["hevcLevelEnforcement"]) != hevcLevelEnforcementHard {
		t.Fatalf("Queue PREPARING did not resolve bounded HEVC decision: %#v", resolved.WorkerConfig)
	}
	decisionBeforeFreeze := configurationHash(effectiveVideoDecision(resolved))
	commandBeforeFreeze := shellJoin((FFmpegCommandBuilder{}).Build(MediaJobPlan{
		InputPath: path, OutputPath: "/tmp/queue-hevc-freeze.mkv", Profile: resolved,
		ProcessingMode: ProcessingModeFullEncode, Streams: streams, Overwrite: true,
	}))
	fingerprint := workerStringValue(resolved.WorkerConfig["hevcLevelInputFingerprint"])
	maxrate := workerIntValue(resolved.WorkerConfig["hevcRateControlMaxrateKbps"], 0)
	if fingerprint == "" || maxrate != 10_000 {
		t.Fatalf("prepared HEVC decision did not freeze its fingerprint/rate: %#v", resolved.WorkerConfig)
	}
	job.ProfileSnapshot[assetConversionOverrideSnapshotKey] = models.JSONMap{"videoCodec": "hevc"}
	if err := freezeQueuePreparedProfileSnapshot(db, &job, resolved); err != nil {
		t.Fatal(err)
	}

	profile.WorkerConfig["x265Params"] = "vbv-maxrate=40000:vbv-bufsize=80000"
	profile.ProfileVersion = 2
	if err := db.Save(&profile).Error; err != nil {
		t.Fatal(err)
	}
	mutated := resolveHEVCLevel(profile, streams)
	if workerStringValue(mutated.WorkerConfig["hevcLevelInputFingerprint"]) == fingerprint || workerIntValue(mutated.WorkerConfig["hevcRateControlMaxrateKbps"], 0) == maxrate {
		t.Fatalf("test mutation did not change the resolver inputs: %#v", mutated.WorkerConfig)
	}

	var restoredJob models.QueueJob
	if err := db.First(&restoredJob, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, ok := restoredJob.ProfileSnapshot[assetConversionOverrideSnapshotKey]; !ok {
		t.Fatalf("Queue preparation dropped the existing conversion override snapshot: %#v", restoredJob.ProfileSnapshot)
	}
	restored, err := scheduler.RestoreProfileSnapshot(restoredJob.ProfileSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if got := configurationHash(effectiveVideoDecision(restored)); got != decisionBeforeFreeze {
		t.Fatalf("restored Queue decision changed after profile mutation: got=%s want=%s config=%#v", got, decisionBeforeFreeze, restored.WorkerConfig)
	}
	if workerStringValue(restored.WorkerConfig["hevcLevelInputFingerprint"]) != fingerprint || workerIntValue(restored.WorkerConfig["hevcRateControlMaxrateKbps"], 0) != maxrate {
		t.Fatalf("restored Queue snapshot lost frozen HEVC state: %#v", restored.WorkerConfig)
	}
	commandAfterRestore := shellJoin((FFmpegCommandBuilder{}).Build(MediaJobPlan{
		InputPath: path, OutputPath: "/tmp/queue-hevc-freeze.mkv", Profile: restored,
		ProcessingMode: ProcessingModeFullEncode, Streams: streams, Overwrite: true,
	}))
	if commandAfterRestore != commandBeforeFreeze {
		t.Fatalf("restored Queue command changed after profile mutation:\nbefore=%s\nafter=%s", commandBeforeFreeze, commandAfterRestore)
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

func TestQueueProfileSnapshotFreezesResolvedCadencePlan(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:queue-cadence-resolution-snapshot?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Profile{}, &models.QueueJob{}, &models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	profile := authoritativeTestProfile()
	profile.WorkerConfig["fieldStructureMode"] = "preserve"
	profile.WorkerConfig["cadenceMode"] = "inverse_telecine"
	profile.WorkerConfig["cadenceFieldOrder"] = "auto"
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	path := "/media/raw/cadence-snapshot.mkv"
	if err := db.Create(&models.ScanResult{
		Path: path,
		InterlaceAnalysis: models.JSONMap{
			"version": float64(interlaceAnalysisVersion), "status": "telecine", "confidence": .99,
			"ivtcValidation": models.JSONMap{"selectedOrder": "bff", "confidence": .99},
		},
		CadenceAnalysis:       models.JSONMap{"version": float64(cadenceAnalysisVersion), "type": "hard_telecine", "confidence": .99},
		CadenceRecommendation: models.JSONMap{"version": 1.0, "operation": "inverse_telecine", "outputFrameRate": "24000/1001", "confidence": .99},
	}).Error; err != nil {
		t.Fatal(err)
	}

	job := models.QueueJob{MediaPath: path, Status: JobStatusQueued}
	if err := NewQueueHandler(db).captureProfile(&job, profile.ID, "queue_create"); err != nil {
		t.Fatal(err)
	}
	frozen := unknownRecord(job.ProfileSnapshot["workerConfig"])
	if frozen == nil {
		t.Fatal("resolved worker configuration was not frozen")
	}
	if got := stringFromUnknown(frozen["deinterlaceMode"]); got != "ivtc_bff" {
		t.Fatalf("frozen field order=%q want ivtc_bff: %#v", got, frozen)
	}
	if got := stringFromUnknown(frozen["effectiveOutputFrameRate"]); got != "24000/1001" {
		t.Fatalf("frozen effective FPS=%q want 24000/1001: %#v", got, frozen)
	}
	if filters := stringFromUnknown(frozen["videoFilters"]); !strings.Contains(filters, "fieldmatch=order=bff:combmatch=full,bwdif=mode=send_frame:parity=bff:deint=interlaced,decimate,setfield=prog") {
		t.Fatalf("frozen cadence chain=%q", filters)
	}

	profile.WorkerConfig["cadenceFieldOrder"] = "tff"
	if err := db.Save(&profile).Error; err != nil {
		t.Fatal(err)
	}
	frozen = unknownRecord(job.ProfileSnapshot["workerConfig"])
	if got := stringFromUnknown(frozen["deinterlaceMode"]); got != "ivtc_bff" {
		t.Fatalf("mutable profile changed frozen field order to %q: %#v", got, frozen)
	}
}

func TestQueueProfileSnapshotFreezesAutomaticCadencePreserveForFrameFidelity(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:queue-auto-cadence-preserve-snapshot?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Profile{}, &models.QueueJob{}, &models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	profile := authoritativeTestProfile()
	profile.WorkerConfig["cadenceMode"] = "auto"
	profile.WorkerConfig["fieldStructureMode"] = "auto"
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	path := "/media/raw/auto-preserve.mkv"
	analysis := CadenceAnalysis{
		Version: cadenceAnalysisVersion, Type: "native_progressive", DeclaredFrameRate: "24000/1001",
		DeclaredFPS: 24000.0 / 1001.0, EffectiveFPS: 24000.0 / 1001.0,
		Confidence: .99, SampleCount: 3, RegionCount: 3,
	}
	recommendation := CadenceRecommendation{Version: 1, Operation: "preserve", Confidence: .99}
	if err := db.Create(&models.ScanResult{
		Path: path, VideoCodec: "h264", Width: 1920, Height: 1080,
		VideoStreams: models.JSONList{map[string]any{"codec": "h264", "width": 1920, "height": 1080, "avgFrameRate": "24000/1001", "fieldOrder": "progressive"}},
		InterlaceAnalysis: models.JSONMap{
			"version": float64(interlaceAnalysisVersion), "status": "progressive", "fieldOrder": "progressive", "averageFrameRate": "24000/1001", "confidence": .99,
		},
		CadenceAnalysis: cadenceAnalysisMap(analysis), CadenceRecommendation: cadenceRecommendationMap(recommendation),
	}).Error; err != nil {
		t.Fatal(err)
	}

	job := models.QueueJob{MediaPath: path, Status: JobStatusQueued}
	if err := NewQueueHandler(db).captureProfile(&job, profile.ID, "queue_create"); err != nil {
		t.Fatal(err)
	}
	resolved, err := scheduler.RestoreProfileSnapshot(job.ProfileSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := freezeQueuePreparedProfileSnapshot(db, &job, resolved); err != nil {
		t.Fatal(err)
	}
	frozen := unknownRecord(job.ProfileSnapshot["workerConfig"])
	if got := stringFromUnknown(frozen["effectiveCadenceOperation"]); got != "preserve" {
		t.Fatalf("Queue snapshot cadence operation=%q want preserve: %#v", got, frozen)
	}
	if got := stringFromUnknown(frozen["effectiveOutputFrameRate"]); got != "24000/1001" {
		t.Fatalf("Queue snapshot effective FPS=%q want 24000/1001: %#v", got, frozen)
	}
	if unknown, ok := frozen["effectiveOutputFrameRateUnknown"].(bool); !ok || unknown {
		t.Fatalf("Queue snapshot did not freeze known FPS state: %#v", frozen["effectiveOutputFrameRateUnknown"])
	}

	restored, err := scheduler.RestoreProfileSnapshot(job.ProfileSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	decision := effectiveVideoDecision(restored)
	if workerStringValue(decision["cadenceOperation"]) != "preserve" || workerStringValue(decision["effectiveFrameRate"]) != "24000/1001" {
		t.Fatalf("restored Queue execution decision lost cadence authority: %#v", decision)
	}

	report := validateJobFrameFidelity(db, job,
		fidelityFrameProbe("h264", "yuv420p", "24000/1001", "progressive"),
		fidelityFrameProbe("h264", "yuv420p", "24000/1001", "progressive"), nil,
	)
	frameRate := report["fields"].(models.JSONMap)["frameRate"].(models.JSONMap)
	if frameRate["expected"] != "24000/1001" || frameRate["status"] != "passed" || frameRate["authorityState"] != "preserve" {
		t.Fatalf("Frame Fidelity did not use the frozen Queue cadence: %#v", frameRate)
	}
}

func TestQueueProfileSnapshotFreezesInterlacedMPEG2FPSForHEVCAutoLevel(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:queue-interlaced-mpeg2-hevc-auto-level?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Profile{}, &models.QueueJob{}, &models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	interlace := InterlaceAnalysis{Status: "interlaced", FieldOrder: "tff", Confidence: .99, Windows: []InterlaceWindow{
		interlacedCadenceWindow(30000.0 / 1001.0), interlacedCadenceWindow(29.97),
	}}
	analysis := analyzeCadence("mpeg2video", "30000/1001", interlace)
	recommendation := recommendCadence(analysis)
	profile := models.Profile{
		Name: "Interlaced MPEG-2 HEVC Auto Level", Container: "mkv", VideoCodec: "hevc", CodecFamily: "hevc",
		EncoderPolicy: "locked", PreferredEncoder: "libx265", AllowedEncoders: models.StringList{"libx265"},
		FallbackPolicy: "wait", BitDepth: 8, PixelFormat: "yuv420p", QualityStrategy: "high",
		ProfileVersion: 1, AudioCodec: "copy", QualityMode: "crf", QualityValue: 20,
		WorkerConfig: models.JSONMap{
			"videoEncoder": "libx265", "cadenceMode": "auto", "fieldStructureMode": "auto", "hevcLevelMode": "auto",
		},
	}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	path := "/media/raw/interlaced-mpeg2-auto-level.mkv"
	if err := db.Create(&models.ScanResult{
		Path: path, VideoCodec: "mpeg2video", Width: 1920, Height: 1080,
		VideoStreams: models.JSONList{map[string]any{
			"codec": "mpeg2video", "width": 1920, "height": 1080,
			"avgFrameRate": "30000/1001", "fieldOrder": "tt",
		}},
		InterlaceAnalysis: models.JSONMap{
			"version": float64(interlaceAnalysisVersion), "status": "interlaced", "fieldOrder": "tff", "confidence": .99,
		},
		CadenceAnalysis: cadenceAnalysisMap(analysis), CadenceRecommendation: cadenceRecommendationMap(recommendation),
	}).Error; err != nil {
		t.Fatal(err)
	}

	job := models.QueueJob{MediaPath: path, Status: JobStatusQueued}
	if err := NewQueueHandler(db).captureProfile(&job, profile.ID, "queue_create"); err != nil {
		t.Fatal(err)
	}
	requested, err := scheduler.RestoreProfileSnapshot(job.ProfileSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	queuedInterlace, ok := decodeInterlaceAnalysis(job.ProfileSnapshot[interlaceAnalysisSnapshotKey])
	if !ok {
		t.Fatal("Queue snapshot did not retain interlace analysis")
	}
	queuedCadence, ok := decodeCadenceAnalysis(job.ProfileSnapshot[cadenceAnalysisSnapshotKey])
	if !ok {
		t.Fatal("Queue snapshot did not retain cadence analysis")
	}
	queuedRecommendation, ok := decodeCadenceRecommendation(job.ProfileSnapshot[cadenceRecommendationSnapshotKey])
	if !ok {
		t.Fatal("Queue snapshot did not retain cadence recommendation")
	}
	streams := MediaStreamInventory{Video: []MediaStream{{
		Index: 0, Width: 1920, Height: 1080, FrameRate: "30000/1001", PixelFormat: "yuv420p",
	}}}
	prepared := resolveMediaJobVideoProfile(requested, streams, path, queuedInterlace, queuedCadence, queuedRecommendation)
	if got := workerStringValue(prepared.WorkerConfig["effectiveOutputFrameRate"]); got != "30000/1001" || profileWorkerBool(prepared, "effectiveOutputFrameRateUnknown", true) {
		t.Fatalf("Queue PREPARING did not resolve the interlaced source FPS: %#v", prepared.WorkerConfig)
	}
	if filters := workerStringValue(prepared.WorkerConfig["videoFilters"]); !strings.Contains(filters, "bwdif=mode=send_frame") {
		t.Fatalf("automatic interlace repair lost send_frame semantics: %q", filters)
	}
	recommendationValue, ok := prepared.WorkerConfig["hevcLevelRecommendation"].(HEVCLevelRecommendation)
	if !ok || !nearFPS(recommendationValue.FPS, 30000.0/1001.0, .01) {
		t.Fatalf("HEVC Auto Level did not use the preserved output FPS: %#v", prepared.WorkerConfig)
	}
	if workerStringValue(prepared.WorkerConfig["hevcLevelResolutionState"]) != hevcLevelResolutionResolved || workerStringValue(prepared.WorkerConfig["hevcLevelEffective"]) == "" {
		t.Fatalf("HEVC Auto Level remained unresolved after Queue cadence resolution: %#v", prepared.WorkerConfig)
	}
	if err := freezeQueuePreparedProfileSnapshot(db, &job, prepared); err != nil {
		t.Fatal(err)
	}
	frozen := unknownRecord(job.ProfileSnapshot["workerConfig"])
	if got := stringFromUnknown(frozen["effectiveOutputFrameRate"]); got != "30000/1001" {
		t.Fatalf("prepared Queue snapshot FPS=%q want 30000/1001: %#v", got, frozen)
	}
	if unknown, ok := frozen["effectiveOutputFrameRateUnknown"].(bool); !ok || unknown {
		t.Fatalf("prepared Queue snapshot did not freeze known FPS state: %#v", frozen["effectiveOutputFrameRateUnknown"])
	}
	restored, err := scheduler.RestoreProfileSnapshot(job.ProfileSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	decision := effectiveVideoDecision(restored)
	if workerStringValue(decision["effectiveFrameRate"]) != "30000/1001" || workerStringValue(restored.WorkerConfig["hevcLevelEffective"]) != workerStringValue(prepared.WorkerConfig["hevcLevelEffective"]) {
		t.Fatalf("frozen Queue profile lost cadence or HEVC Auto Level decisions: %#v", restored.WorkerConfig)
	}
}

func TestQueueProfileSnapshotFreezesResolvedUpscaleDecision(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:queue-upscale-snapshot?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Profile{}, &models.QueueJob{}, &models.ScanResult{}, &models.SchedulerReservation{}); err != nil {
		t.Fatal(err)
	}
	profile := authoritativeTestProfile()
	profile.WorkerConfig["upscaleMode"] = "auto"
	profile.WorkerConfig["upscaleSharpen"] = "custom"
	profile.WorkerConfig["upscaleSharpenCustomStrength"] = .16
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	path := "/media/raw/anamorphic-dvd.mkv"
	scan := models.ScanResult{
		Path: path, FileName: "anamorphic-dvd.mkv", Width: 720, Height: 480, VideoCodec: "mpeg2video",
		VideoStreams: models.JSONList{map[string]any{
			"width": 720, "height": 480, "sampleAspectRatio": "32:27", "displayAspectRatio": "16:9", "avgFrameRate": "30000/1001",
		}},
		InterlaceAnalysis:      models.JSONMap{"version": float64(interlaceAnalysisVersion), "status": "progressive", "confidence": .99, "recommendedAction": "preserve"},
		CadenceAnalysis:        models.JSONMap{"version": float64(cadenceAnalysisVersion), "type": "native", "confidence": .99},
		CadenceRecommendation:  models.JSONMap{"version": 1.0, "operation": "preserve", "confidence": .99},
		FrameStructureAnalysis: models.JSONMap{"version": 2.0, "framesAnalyzed": 900.0, "confidence": "high"},
	}
	if err := db.Create(&scan).Error; err != nil {
		t.Fatal(err)
	}

	job := models.QueueJob{MediaPath: path, Status: JobStatusQueued}
	if err := NewQueueHandler(db).captureProfile(&job, profile.ID, "queue_create"); err != nil {
		t.Fatal(err)
	}
	frozenProfile, err := scheduler.RestoreProfileSnapshot(job.ProfileSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	frozen, ok := resolvedUpscaleDecisionFromProfile(frozenProfile)
	if !ok || !frozen.UpscaleApplied || frozen.TargetWidth != 1280 || frozen.TargetHeight != 720 || frozen.SharpenMode != UpscaleSharpenCustom || frozen.SharpenStrength != .16 {
		t.Fatalf("queue did not freeze resolved upscale: %#v", frozen)
	}

	profile.WorkerConfig["upscaleMode"] = "1080p"
	profile.WorkerConfig["upscaleSharpenCustomStrength"] = .30
	if err := db.Save(&profile).Error; err != nil {
		t.Fatal(err)
	}
	scan.VideoStreams = models.JSONList{map[string]any{"width": 720, "height": 480, "sampleAspectRatio": "8:9", "displayAspectRatio": "4:3"}}
	if err := db.Save(&scan).Error; err != nil {
		t.Fatal(err)
	}
	restoredAgain, err := scheduler.RestoreProfileSnapshot(job.ProfileSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	stillFrozen, _ := resolvedUpscaleDecisionFromProfile(restoredAgain)
	if !reflect.DeepEqual(frozen, stillFrozen) {
		t.Fatalf("queued upscale changed after profile/snapshot edits: before=%#v after=%#v", frozen, stillFrozen)
	}
	workerResolved := resolveUpscaleProfile(restoredAgain, MediaStreamInventory{Video: []MediaStream{{Width: 720, Height: 480, SampleAspectRatio: "8:9", DisplayAspectRatio: "4:3"}}}, upscaleAnalysisEvidence(scan))
	consumed, _ := resolvedUpscaleDecisionFromProfile(workerResolved)
	if !reflect.DeepEqual(frozen, consumed) {
		t.Fatalf("worker re-resolved frozen upscale decision: frozen=%#v consumed=%#v", frozen, consumed)
	}
}

func TestQueueProfileSnapshotFreezesKeepSourceAfterProfileChangesTo1080p(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:queue-upscale-keep-source-snapshot?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Profile{}, &models.QueueJob{}, &models.ScanResult{}, &models.SchedulerReservation{}); err != nil {
		t.Fatal(err)
	}
	profile := authoritativeTestProfile()
	profile.WorkerConfig["upscaleMode"] = "auto"
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	path := "/media/raw/unsafe-sd.mkv"
	scan := models.ScanResult{Path: path, FileName: "unsafe-sd.mkv", Width: 720, Height: 480, VideoStreams: models.JSONList{map[string]any{"width": 720, "height": 480, "sampleAspectRatio": "32:27", "displayAspectRatio": "16:9"}}}
	if err := db.Create(&scan).Error; err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: path, Status: JobStatusQueued}
	if err := NewQueueHandler(db).captureProfile(&job, profile.ID, "queue_create"); err != nil {
		t.Fatal(err)
	}
	frozenProfile, err := scheduler.RestoreProfileSnapshot(job.ProfileSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	frozen, ok := resolvedUpscaleDecisionFromProfile(frozenProfile)
	if !ok || frozen.UpscaleApplied || frozen.ResolvedMode != ResolvedUpscaleKeepSource {
		t.Fatalf("Queue did not freeze Keep Source: %#v", frozen)
	}
	profile.WorkerConfig["upscaleMode"] = "1080p"
	if err := db.Save(&profile).Error; err != nil {
		t.Fatal(err)
	}
	consumedProfile := resolveUpscaleProfile(frozenProfile, MediaStreamInventory{Video: []MediaStream{{Width: 720, Height: 480, SampleAspectRatio: "32:27", DisplayAspectRatio: "16:9"}}}, UpscaleAnalysisEvidence{FrameStructureAvailable: true})
	consumed, _ := resolvedUpscaleDecisionFromProfile(consumedProfile)
	if !reflect.DeepEqual(frozen, consumed) {
		t.Fatalf("worker changed frozen Keep Source after profile edit: frozen=%#v consumed=%#v", frozen, consumed)
	}
}

func TestQueueProfileSnapshotFreezesDeinterlaceParityWithoutForcingIt(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:queue-deinterlace-parity-snapshot?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Profile{}, &models.QueueJob{}, &models.ScanResult{}, &models.SchedulerReservation{}); err != nil {
		t.Fatal(err)
	}
	profile := authoritativeTestProfile()
	profile.WorkerConfig["deinterlaceMode"] = "force"
	profile.WorkerConfig["deinterlaceFieldOrder"] = "bff"
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	path := "/media/raw/interlaced-dvd.mkv"
	scan := models.ScanResult{Path: path, InterlaceAnalysis: models.JSONMap{
		"version": float64(interlaceAnalysisVersion), "status": "interlaced", "confidence": .99, "detectedFieldOrder": "tff",
	}}
	if err := db.Create(&scan).Error; err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: path, Status: JobStatusQueued}
	if err := NewQueueHandler(db).captureProfile(&job, profile.ID, "queue_create"); err != nil {
		t.Fatal(err)
	}
	profile.WorkerConfig["deinterlaceFieldOrder"] = "tff"
	if err := db.Save(&profile).Error; err != nil {
		t.Fatal(err)
	}
	frozen, err := scheduler.RestoreProfileSnapshot(job.ProfileSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	analysis, ok := decodeInterlaceAnalysis(job.ProfileSnapshot[interlaceAnalysisSnapshotKey])
	if !ok {
		t.Fatalf("interlace evidence was not frozen: %#v", job.ProfileSnapshot)
	}
	resolved := profileWithAutomaticDeinterlace(frozen, analysis)
	filter := workerStringValue(resolved.WorkerConfig["videoFilters"])
	if !strings.Contains(filter, "bwdif=mode=send_frame:parity=bff:deint=all") {
		t.Fatalf("queued BFF parity changed after profile edit: %q", filter)
	}

	frozen.WorkerConfig["deinterlaceMode"] = "off"
	frozen.WorkerConfig["videoFilters"] = ""
	preserved := profileWithAutomaticDeinterlace(frozen, analysis)
	if filter := workerStringValue(preserved.WorkerConfig["videoFilters"]); strings.Contains(filter, "bwdif=") {
		t.Fatalf("frozen parity forced deinterlace after mode was disabled: %q", filter)
	}
}

func TestQueueUpscaleSnapshotUsesHighestIDWhenUpdatedAtTies(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:queue-upscale-snapshot-order?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Profile{}, &models.QueueJob{}, &models.ScanResult{}, &models.SchedulerReservation{}); err != nil {
		t.Fatal(err)
	}
	profile := authoritativeTestProfile()
	profile.WorkerConfig["upscaleMode"] = "720p"
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	path := "/media/raw/tied-snapshot.mkv"
	updatedAt := time.Date(2026, time.August, 31, 12, 0, 0, 0, time.UTC)
	for _, stream := range []models.JSONList{
		{map[string]any{"width": 720, "height": 480, "sampleAspectRatio": "8:9", "displayAspectRatio": "4:3"}},
		{map[string]any{"width": 720, "height": 480, "sampleAspectRatio": "32:27", "displayAspectRatio": "16:9"}},
	} {
		if err := db.Create(&models.ScanResult{Path: path, FileName: "tied-snapshot.mkv", Width: 720, Height: 480, VideoStreams: stream, UpdatedAt: updatedAt}).Error; err != nil {
			t.Fatal(err)
		}
	}
	job := models.QueueJob{MediaPath: path, Status: JobStatusQueued}
	if err := NewQueueHandler(db).captureProfile(&job, profile.ID, "queue_create"); err != nil {
		t.Fatal(err)
	}
	frozenProfile, err := scheduler.RestoreProfileSnapshot(job.ProfileSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	decision, ok := resolvedUpscaleDecisionFromProfile(frozenProfile)
	if !ok || decision.TargetWidth != 1280 || decision.TargetHeight != 720 {
		t.Fatalf("Queue did not select the highest-id tied snapshot: %#v", decision)
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
		&models.AssetRecord{},
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
		if want.batchID != "" && (queueJobFreezeComplete(got) || got.ActiveExecutionPlanID != nil) {
			t.Fatalf("ordinary batch job %q was frozen while queued: %#v", got.MediaPath, got)
		}

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

func TestBatchPreflightRetainsNaturalOrderWithoutCreatingQueueJobs(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open("file:queue-create-atomic-batch?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.AutoMigrate(
		&models.AssetRecord{},
		&models.Profile{},
		&models.QueueJob{},
		&models.ExecutionPlan{},
		&models.SchedulerReservation{},
		&models.AppSetting{},
		&models.QueueBatchPreflight{},
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
	router.POST("/api/queue/preflights", NewQueueHandler(db).CreateBatchPreflight)

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
		"/api/queue/preflights",
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

	var candidate QueueBatchPreflightResponse
	if err := json.Unmarshal(response.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	if len(candidate.Assets) != 3 {
		t.Fatalf("candidate assets=%d want=3", len(candidate.Assets))
	}
	for index, want := range []string{"title.mkv", "title_01.mkv", "title_02.mkv"} {
		if got := filepath.Base(candidate.Assets[index].MediaPath); got != want {
			t.Fatalf("candidate asset[%d]=%q want=%q", index, got, want)
		}
	}
	var queued []models.QueueJob
	if err := db.
		Where("dismissed_at IS NULL").
		Order("queue_position asc").
		Find(&queued).Error; err != nil {
		t.Fatal(err)
	}

	if len(queued) != 1 || queued[0].ID != existing.ID {
		t.Fatalf("candidate creation changed existing Queue state: %#v", queued)
	}
}

func TestLegacyBatchRouteCannotBypassPreflightOrMutateReservations(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open("file:queue-create-atomic-rollback?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.AutoMigrate(
		&models.AssetRecord{},
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

func TestQueueSelectedAssetsPlanIsAuthoritativeAndDoesNotPersist(t *testing.T) {
	db := queueSelectedAssetsTestDB(t)
	profile := authoritativeTestProfile()
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	logicalPath := "/media/raw/Anime/Akira"
	records := []models.AssetRecord{
		{Path: logicalPath + "/Season 1/Akira 02.mkv", LogicalGroupPath: logicalPath, SourcePath: logicalPath + "/Season 1/Akira 02.mkv", FileName: "Akira 02.mkv", Status: "unprocessed", SizeBytes: 20},
		{Path: logicalPath + "/Season 1/Akira 10.mkv", LogicalGroupPath: logicalPath, SourcePath: logicalPath + "/Season 1/Akira 10.mkv", FileName: "Akira 10.mkv", Status: "unprocessed", SizeBytes: 10},
		{Path: logicalPath + "/Season 1/Missing.mkv", LogicalGroupPath: logicalPath, SourcePath: logicalPath + "/Season 1/Missing.mkv", FileName: "Missing.mkv", Status: "unprocessed", Missing: true},
		{Path: "/media/raw/Anime/No Destination/Episode.mkv", LogicalGroupPath: "/media/raw/Anime/No Destination", SourcePath: "/media/raw/Anime/No Destination/Episode.mkv", FileName: "Episode.mkv", Status: "unprocessed"},
	}
	if err := db.Create(&records).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.ProfileAssignment{TargetType: assetScopeLogicalGroup, TargetPath: logicalPath, MediaType: "video", Selection: "profile", VideoProfileID: profile.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AssetScopeConfiguration{ScopeType: assetScopeLogicalGroup, ScopeKey: logicalPath, DestinationSelection: configSelectionValue, DestinationLibraryID: 7}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.ProfileAssignment{TargetType: assetScopeLogicalGroup, TargetPath: records[3].LogicalGroupPath, MediaType: "video", Selection: "profile", VideoProfileID: profile.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.QueueJob{MediaPath: "/media/raw/Other/Open.mkv", Status: JobStatusRunning}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AssetMaintenanceOperation{AssetPath: "/media/raw/Other/Maintenance.mkv", Status: maintenanceStatusRunning}).Error; err != nil {
		t.Fatal(err)
	}
	queryCounts := map[string]int{}
	callbackName := "test:count-selected-queue-effective-scans"
	countQuery := func(tx *gorm.DB) {
		table := tx.Statement.Table
		if table == "" && strings.Contains(tx.Statement.SQL.String(), "scan_results") {
			table = "scan_results"
		}
		queryCounts[table]++
	}
	if err := db.Callback().Query().After("gorm:query").Register(callbackName, countQuery); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Row().After("gorm:row").Register(callbackName, countQuery); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Query().Remove(callbackName)
	defer db.Callback().Row().Remove(callbackName)

	response := queueSelectedAssetsRequest(t, db, QueueSelectedAssetsInput{
		AssetIDs: []uint{records[1].ID, records[0].ID, records[2].ID, records[3].ID, 999999, records[0].ID},
		Commit:   false,
	})
	if queryCounts["queue_jobs"] != 1 || queryCounts["asset_maintenance_operations"] != 1 {
		t.Fatalf("Queue Selected repeated open-job or maintenance scans: %#v", queryCounts)
	}
	if response.Summary.Selected != 5 || response.Summary.Eligible != 2 || response.Summary.Skipped != 1 || response.Summary.Failed != 2 {
		t.Fatalf("unexpected plan summary: %#v", response.Summary)
	}
	if response.Summary.TitleCount != 2 || response.Summary.SizeBytes != 30 {
		t.Fatalf("unexpected title/size summary: %#v", response.Summary)
	}
	if len(response.Results) != 5 || response.Results[0].AssetID != records[0].ID || response.Results[1].AssetID != records[1].ID {
		t.Fatalf("results are not naturally ordered and deduplicated: %#v", response.Results)
	}
	reasons := map[uint]string{}
	for _, result := range response.Results {
		reasons[result.AssetID] = result.Reason
	}
	if reasons[records[2].ID] != "missing" || reasons[records[3].ID] != "invalid_configuration" || reasons[999999] != "not_found" {
		t.Fatalf("unexpected per-asset reasons: %#v", reasons)
	}
	for _, result := range response.Results[:2] {
		if result.Outcome != "eligible" || result.BatchID == "" || result.BatchName != "Season 1" {
			t.Fatalf("eligible result lacks backend batch plan: %#v", result)
		}
	}
	for model, expected := range map[any]int64{&models.QueueJob{}: 1, &models.SchedulerReservation{}: 0, &models.ExecutionPlan{}: 0} {
		var count int64
		if err := db.Model(model).Count(&count).Error; err != nil || count != expected {
			t.Fatalf("planning persisted %T count=%d want=%d err=%v", model, count, expected, err)
		}
	}
	for _, table := range []string{"profile_assignments", "asset_scope_configurations"} {
		if queryCounts[table] != 1 {
			t.Fatalf("Queue prepare scanned %s %d times, want once: %#v", table, queryCounts[table], queryCounts)
		}
	}
	if queryCounts["scan_results"] != 1 {
		t.Fatalf("Queue preview did not batch-read persisted analysis evidence exactly once: %#v", queryCounts)
	}
}

func TestQueueSelectedAssetsAnalysisEvidenceIsSharedAndBatchReadOnly(t *testing.T) {
	db := queueSelectedAssetsTestDB(t)
	path := filepath.Join(t.TempDir(), "selected-analysis.mkv")
	if err := os.WriteFile(path, []byte("source fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	ready := models.AssetRecord{Path: path, SourcePath: path, FileName: filepath.Base(path), Status: "unprocessed", SizeBytes: info.Size(), ModifiedAt: info.ModTime()}
	missing := models.AssetRecord{Path: path + ".missing", SourcePath: path + ".missing", FileName: "missing.mkv", Status: "unprocessed"}
	if err := db.Create(&[]models.AssetRecord{ready, missing}).Error; err != nil {
		t.Fatal(err)
	}
	var records []models.AssetRecord
	if err := db.Order("id").Find(&records).Error; err != nil {
		t.Fatal(err)
	}
	scan := completeAnalysisSnapshot(path, info)
	stampSnapshotCacheMetadata(&scan, path, info)
	if err := db.Create(&scan).Error; err != nil {
		t.Fatal(err)
	}
	queries := 0
	callbackName := "test:count-selected-analysis-scan-batch"
	countScanQuery := func(tx *gorm.DB) {
		if tx.Statement.Table == "scan_results" || strings.Contains(tx.Statement.SQL.String(), "scan_results") {
			queries++
		}
	}
	if err := db.Callback().Query().After("gorm:query").Register(callbackName, countScanQuery); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Row().After("gorm:row").Register(callbackName, countScanQuery); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Query().Remove(callbackName)
	defer db.Callback().Row().Remove(callbackName)

	response := queueSelectedAssetsRequest(t, db, QueueSelectedAssetsInput{AssetIDs: []uint{records[0].ID, records[1].ID}})
	if queries != 1 {
		t.Fatalf("selected Preflight issued %d ScanResult queries, want one batch read", queries)
	}
	want := resolveAssetAnalysisEvidence(records[0], []models.ScanResult{scan}, analysisPolicy(db), 20)
	if !reflect.DeepEqual(response.Results[0].AnalysisEvidence, want) {
		t.Fatalf("Preflight evidence differs from Asset Info projection: got %#v want %#v", response.Results[0].AnalysisEvidence, want)
	}
	if response.Results[0].AnalysisEvidence.Status != assetAnalysisReady || response.Results[0].AnalysisEvidence.SourceIdentity.SnapshotID != scan.ID {
		t.Fatalf("ready source identity was not exposed: %#v", response.Results[0].AnalysisEvidence)
	}
	if response.Results[1].AnalysisEvidence.Status != assetAnalysisMissing {
		t.Fatalf("missing persisted evidence was not reported: %#v", response.Results[1].AnalysisEvidence)
	}
	var scanCount, jobCount int64
	if err := db.Model(&models.ScanResult{}).Count(&scanCount).Error; err != nil || scanCount != 1 {
		t.Fatalf("read-only Preflight changed ScanResult count=%d err=%v", scanCount, err)
	}
	if err := db.Model(&models.QueueJob{}).Count(&jobCount).Error; err != nil || jobCount != 0 {
		t.Fatalf("read-only Preflight persisted Queue jobs count=%d err=%v", jobCount, err)
	}
}

func TestQueueSelectedAssetsCommitPreservesPartialSuccessIntentAndNaturalOrder(t *testing.T) {
	db := queueSelectedAssetsTestDB(t)
	rawRoot := t.TempDir()
	destinationRoot := t.TempDir()
	profile := authoritativeTestProfile()
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	alternateProfile := authoritativeTestProfile()
	alternateProfile.Name = "Alternate selected profile"
	alternateProfile.QualityValue = 21
	if err := db.Create(&alternateProfile).Error; err != nil {
		t.Fatal(err)
	}
	successLogical := filepath.Join(rawRoot, "Anime", "A Success")
	conflictLogical := filepath.Join(rawRoot, "Anime", "B Conflict")
	records := []models.AssetRecord{
		{Path: successLogical + "/Season 2/Episode 10.mkv", LogicalGroupPath: successLogical, SourcePath: successLogical + "/Season 2/Episode 10.mkv", FileName: "Episode 10.mkv", Status: "unprocessed", SizeBytes: 10},
		{Path: successLogical + "/Season 2/Episode 2.mkv", LogicalGroupPath: successLogical, SourcePath: successLogical + "/Season 2/Episode 2.mkv", FileName: "Episode 2.mkv", Status: "unprocessed", SizeBytes: 20},
		{Path: conflictLogical + "/Season 1/Episode 1.mkv", LogicalGroupPath: conflictLogical, SourcePath: conflictLogical + "/Season 1/Episode 1.mkv", FileName: "Episode 1.mkv", Status: "unprocessed", SizeBytes: 30},
	}
	if err := db.Create(&records).Error; err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if err := os.MkdirAll(filepath.Dir(record.Path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(record.Path, []byte("test media placeholder"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&models.Library{ID: 9, Name: "Selected assets", SourcePath: rawRoot, DestinationPath: destinationRoot, Type: "anime"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, logicalPath := range []string{successLogical, conflictLogical} {
		if err := db.Create(&models.ProfileAssignment{TargetType: assetScopeLogicalGroup, TargetPath: logicalPath, MediaType: "video", Selection: "profile", VideoProfileID: profile.ID}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&models.AssetScopeConfiguration{ScopeType: assetScopeLogicalGroup, ScopeKey: logicalPath, DestinationSelection: configSelectionValue, DestinationLibraryID: 9}).Error; err != nil {
			t.Fatal(err)
		}
	}
	settings := []models.AppSetting{
		{Key: "audioEnhancementProfiles", Value: models.JSONMap{"profiles": models.JSONList{models.JSONMap{"key": "selected-audio", "scope": "path", "filters": "volume=0.8"}}}},
		{Key: "trackProfiles", Value: models.JSONMap{"profiles": models.JSONList{models.JSONMap{"key": "selected-tracks", "scope": "path", "videoMode": "first", "audioMode": "default", "subtitleMode": "none"}}}},
	}
	for index := range settings {
		if err := db.Create(&settings[index]).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, mediaType := range []string{"audio", "tracks"} {
		profileKey := "selected-" + mediaType
		if err := db.Create(&models.ProfileAssignment{TargetType: assetScopeLogicalGroup, TargetPath: successLogical, MediaType: mediaType, Selection: "profile", ProfileKey: profileKey}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&models.ProfileAssignment{TargetType: assetScopeAsset, TargetPath: records[0].Path, MediaType: "video", Selection: "profile", VideoProfileID: alternateProfile.ID}).Error; err != nil {
		t.Fatal(err)
	}
	for _, record := range records[:2] {
		if err := db.Create(&models.ScanResult{Path: record.Path, VideoStreams: models.JSONList{map[string]any{"index": 0}}, AudioStreams: models.JSONList{map[string]any{"index": 1, "default": true}}}).Error; err != nil {
			t.Fatal(err)
		}
	}
	seedQueueSelectedPreflightRuntime(t, db)

	plan := queueSelectedAssetsRequest(t, db, QueueSelectedAssetsInput{AssetIDs: []uint{records[0].ID, records[2].ID, records[1].ID}, Commit: false})
	if plan.Summary.Eligible != 3 || plan.Summary.Failed != 0 {
		t.Fatalf("pre-commit plan was not fully eligible: %#v", plan)
	}
	if err := db.Create(&models.SchedulerReservation{JobID: 999999, AssetKey: filepath.Clean(records[2].Path), State: scheduler.ReservationStateLocked}).Error; err != nil {
		t.Fatal(err)
	}

	candidate := createQueueSelectedAssetsPreflight(t, db, []uint{records[0].ID, records[2].ID, records[1].ID})
	readyIDs := make([]string, 0, 2)
	blockedReservationFound := false
	for _, asset := range candidate.Assets {
		if asset.Status == queuePreflightReady {
			readyIDs = append(readyIDs, asset.ID)
		} else if asset.MediaPath == records[2].Path && strings.Contains(strings.Join(asset.Blockers, " "), "reservation") {
			blockedReservationFound = true
		}
	}
	if len(readyIDs) != 2 || !blockedReservationFound {
		t.Fatalf("Preflight did not preserve ready and reserved assets: %#v", candidate.Assets)
	}
	response := acceptQueueSelectedAssetsPreflight(t, db, candidate, readyIDs)
	if len(response.Jobs) != 2 || len(response.Assets) != 3 {
		t.Fatalf("partial acceptance did not preserve the candidate context: %#v", response)
	}
	queuedCount, unresolvedCount := 0, 0
	for _, asset := range response.Assets {
		if asset.Status == queuePreflightQueued {
			queuedCount++
		} else if asset.MediaPath == records[2].Path && asset.Status == queuePreflightBlocked {
			unresolvedCount++
		}
	}
	if queuedCount != 2 || unresolvedCount != 1 {
		t.Fatalf("partial acceptance lost Queue or unresolved asset evidence: %#v", response.Assets)
	}

	var jobs []models.QueueJob
	if err := db.Order("queue_position asc").Find(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 || filepath.Base(jobs[0].MediaPath) != "Episode 2.mkv" || filepath.Base(jobs[1].MediaPath) != "Episode 10.mkv" {
		t.Fatalf("queued jobs lost natural order: %#v", jobs)
	}
	if jobs[0].ProfileID != profile.ID || jobs[1].ProfileID != alternateProfile.ID {
		t.Fatalf("mixed effective profiles did not preserve queue intent: %#v", jobs)
	}
	for index, job := range jobs {
		if !queueJobFreezeComplete(job) || job.ProfileCapturedAt == nil || job.ActiveExecutionPlanID == nil {
			t.Fatalf("Queue acceptance did not freeze reviewed material decisions: %#v", job)
		}
		videoResolution, _ := job.ProfileResolution["video"].(map[string]any)
		expectedSource := assetScopeLogicalGroup
		if index == 1 {
			expectedSource = assetScopeAsset
		}
		if videoResolution["source"] != expectedSource {
			t.Fatalf("job did not preserve inherited provenance: %#v", job.ProfileResolution)
		}
	}
	var plans int64
	if err := db.Model(&models.ExecutionPlan{}).Count(&plans).Error; err != nil || plans != 2 {
		t.Fatalf("execution plans=%d err=%v want accepted jobs=2", plans, err)
	}
}

func TestQueueSelectedSeasonOnlyBatchUsesSeriesTitleAndCompleteGroupOrdinals(t *testing.T) {
	db := queueSelectedAssetsTestDB(t)
	root := t.TempDir()
	rawRoot := filepath.Join(root, "raw")
	destinationRoot := filepath.Join(root, "library")
	if err := os.MkdirAll(destinationRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	profile := authoritativeTestProfile()
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	library := models.Library{
		Name:            "Series",
		SourcePath:      rawRoot,
		DestinationPath: destinationRoot,
		Type:            "tv",
		ValidationRules: models.JSONMap{"episodeNamingEnabled": true},
	}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}

	logicalPath := filepath.Join(rawRoot, "series", "Doctor Who")
	groupPath := filepath.Join("series", "Doctor Who", "Season2")
	names := []string{
		"DR_WHO_COMPLETE_SERIES_2018_D03_Ttitle_t02.mkv",
		"DR_WHO_COMPLETE_SERIES_2018_D03_Ttitle_t03.mkv",
		"DR_WHO_COMPLETE_SERIES_2018_D03_Ttitle_t05.mkv",
		"DR_WHO_COMPLETE_SERIES_2018_D04_Ttitle_t00.mkv",
	}
	records := make([]models.AssetRecord, 0, len(names))
	for _, name := range names {
		mediaPath := filepath.Join(rawRoot, groupPath, name)
		records = append(records, models.AssetRecord{
			Path:             mediaPath,
			SourcePath:       mediaPath,
			RootPath:         rawRoot,
			GroupPath:        groupPath,
			LogicalGroupPath: logicalPath,
			RelativePath:     filepath.Join(groupPath, name),
			FileName:         name,
			Status:           "unprocessed",
		})
	}
	if err := db.Create(&records).Error; err != nil {
		t.Fatal(err)
	}
	for _, record := range records {
		if err := os.MkdirAll(filepath.Dir(record.Path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(record.Path, []byte("test media placeholder"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&models.ProfileAssignment{TargetType: assetScopeLogicalGroup, TargetPath: logicalPath, MediaType: "video", Selection: "profile", VideoProfileID: profile.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AssetScopeConfiguration{ScopeType: assetScopeLogicalGroup, ScopeKey: logicalPath, DestinationSelection: configSelectionValue, DestinationLibraryID: library.ID}).Error; err != nil {
		t.Fatal(err)
	}
	seedQueueSelectedPreflightRuntime(t, db)

	candidate := createQueueSelectedAssetsPreflight(t, db, []uint{records[3].ID, records[1].ID, records[0].ID})
	readyIDs := make([]string, 0, len(candidate.Assets))
	for _, asset := range candidate.Assets {
		if asset.Status != queuePreflightReady || asset.Plan == nil {
			t.Fatalf("season asset did not resolve in Preflight: %#v", asset)
		}
		readyIDs = append(readyIDs, asset.ID)
	}
	response := acceptQueueSelectedAssetsPreflight(t, db, candidate, readyIDs)
	jobs := response.Jobs
	if len(jobs) != 3 || jobs[0].BatchName != "Season2" {
		t.Fatalf("unexpected accepted Queue job shape: %#v", jobs)
	}
	wantEpisodes := []int{1, 2, 4}
	for index, job := range jobs {
		want := filepath.Join(destinationRoot, "Doctor Who", "Season2", fmt.Sprintf("Doctor Who - S02E%02d.mkv", wantEpisodes[index]))
		if got := workerStringValue(job.ResolvedDestinationSnapshot["outputPath"]); got != want {
			t.Fatalf("job %d output=%q want=%q", job.ID, got, want)
		}
	}
}

func TestAcceptedSelectedQueueJobFreezesReviewedConfigurationOnce(t *testing.T) {
	db := queueSelectedAssetsTestDB(t)
	sourceDir := t.TempDir()
	mediaPath := filepath.Join(sourceDir, "Episode 1.mkv")
	if err := os.WriteFile(mediaPath, []byte("test media placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}
	library := models.Library{Name: "Test Library", SourcePath: sourceDir, DestinationPath: t.TempDir(), Type: "anime"}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	first := authoritativeTestProfile()
	first.QualityValue = 18
	second := authoritativeTestProfile()
	second.Name = "Claim-time profile"
	second.QualityValue = 21
	if err := db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	record := models.AssetRecord{Path: mediaPath, SourcePath: mediaPath, LogicalGroupPath: sourceDir, FileName: filepath.Base(mediaPath), Status: "unprocessed"}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	assignment := models.ProfileAssignment{TargetType: assetScopeLogicalGroup, TargetPath: sourceDir, MediaType: "video", Selection: "profile", VideoProfileID: first.ID}
	if err := db.Create(&assignment).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.ProfileAssignment{TargetType: assetScopeLogicalGroup, TargetPath: sourceDir, MediaType: "tracks", Selection: "profile", ProfileKey: "tracks"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AssetScopeConfiguration{ScopeType: assetScopeLogicalGroup, ScopeKey: sourceDir, DestinationSelection: configSelectionValue, DestinationLibraryID: library.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.ScanResult{Path: mediaPath, VideoCodec: "h264", Width: 720, Height: 480, VideoStreams: models.JSONList{map[string]any{"index": 0, "codec": "h264", "width": 720, "height": 480}}, SubtitleStreams: models.JSONList{map[string]any{"index": 4, "codec": "ass", "language": "eng", "title": "Full"}}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "trackProfiles", Value: models.JSONMap{"profiles": models.JSONList{models.JSONMap{
		"key": "tracks", "scope": "path", "trackDispositionVersion": 1,
		"subtitleDisposition": "extract", "subtitleSidecarFormats": models.JSONList{"ass"},
		"subtitleRules":    models.JSONList{models.JSONMap{"streamIndex": 4, "action": "extract", "sidecarFormats": models.JSONList{"ass"}}},
		"attachmentPolicy": "auto", "chapterPolicy": "keep",
	}}}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: subtitleSettingsKey, Value: models.JSONMap{generateSRTFallbackForASSKey: false}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "pipelineAutomation", Value: models.JSONMap{"reviewMode": "automatic"}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.RuntimeSnapshot{DetectedAt: time.Now(), SelectedProfile: "test", Encoders: models.JSONMap{"libx265": models.JSONMap{"listed": true, "usable": true}}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.RuntimeState{SingletonKey: models.RuntimeStateSingletonKey, DetectedAt: time.Now(), SelectedProfile: "test", Encoders: models.JSONMap{"libx265": models.JSONMap{"listed": true, "usable": true}}, StableFingerprint: "queue-test", FingerprintVersion: "v1"}).Error; err != nil {
		t.Fatal(err)
	}
	acceptedDefaults := defaultAssetDefaultsValue()
	acceptedDefaults["audio"].(models.JSONMap)["custom"] = models.JSONMap{"preferredDefaultLanguage": "eng"}
	if err := db.Create(&models.AppSetting{Key: assetDefaultsSettingKey, Value: acceptedDefaults}).Error; err != nil {
		t.Fatal(err)
	}

	candidate := createQueueSelectedAssetsPreflight(t, db, []uint{record.ID})
	if len(candidate.Assets) != 1 || candidate.Assets[0].Status != queuePreflightReady || candidate.Assets[0].Plan == nil {
		t.Fatalf("selected asset did not resolve in Preflight: %#v", candidate.Assets)
	}
	var queuedBeforeAcceptance, plansBeforeAcceptance int64
	if err := db.Model(&models.QueueJob{}).Count(&queuedBeforeAcceptance).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.ExecutionPlan{}).Count(&plansBeforeAcceptance).Error; err != nil {
		t.Fatal(err)
	}
	if queuedBeforeAcceptance != 0 || plansBeforeAcceptance != 0 {
		t.Fatalf("Preflight created Queue state before acceptance: jobs=%d plans=%d", queuedBeforeAcceptance, plansBeforeAcceptance)
	}
	reviewedProfile := candidate.Assets[0].Plan.ProfileSnapshot
	reviewedTracks := candidate.Assets[0].Plan.TrackProfileSnapshot
	setFallbackFreezeTestSetting(t, db, true)
	if err := db.Model(&assignment).Update("video_profile_id", second.ID).Error; err != nil {
		t.Fatal(err)
	}
	first.QualityValue = 29
	if err := db.Save(&first).Error; err != nil {
		t.Fatal(err)
	}
	accepted := acceptQueueSelectedAssetsPreflight(t, db, candidate, []string{candidate.Assets[0].ID})
	if len(accepted.Jobs) != 1 {
		t.Fatalf("selected asset was not queued: %#v", accepted)
	}
	job := accepted.Jobs[0]
	if !queueJobFreezeComplete(job) || job.ActiveExecutionPlanID == nil || job.ProfileID != first.ID || snapshotQualityValue(job.ProfileSnapshot) != 18 || !reflect.DeepEqual(job.ProfileSnapshot, reviewedProfile) || !reflect.DeepEqual(job.TrackProfileSnapshot, reviewedTracks) {
		t.Fatalf("Accept did not freeze the exact reviewed Queue plan: %#v", job)
	}
	changedDefaults := defaultAssetDefaultsValue()
	changedDefaults["audio"].(models.JSONMap)["custom"] = models.JSONMap{"preferredDefaultLanguage": "jpn"}
	if err := db.Model(&models.AppSetting{}).Where("key = ?", assetDefaultsSettingKey).Update("value", changedDefaults).Error; err != nil {
		t.Fatal(err)
	}
	acceptedAudio := settingProfileObject(job.ProfileResolution["audio"])
	if workerStringValue(settingProfileObject(acceptedAudio["profileData"])["preferredDefaultLanguage"]) != "eng" {
		t.Fatalf("accepted Batch Preflight did not retain its reviewed Global Asset Defaults: %#v", job.ProfileResolution["audio"])
	}
	now := time.Now()
	if err := db.Model(&job).Updates(map[string]any{"status": JobStatusRunning, "stage": JobStageClaimed, "worker_name": "test-worker", "started_at": &now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&job, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	prepared, err := NewWorkerHandler(db).prepareClaimedQueueJob(job)
	if err != nil {
		t.Fatal(err)
	}
	frozenTrackPlan, frozenTrackPlanOK := ResolvedTrackPlanFromSnapshot(prepared.TrackProfileSnapshot)
	if prepared.ProfileID != first.ID || snapshotQualityValue(prepared.ProfileSnapshot) != 18 || !queueJobFreezeComplete(prepared) || prepared.ActiveExecutionPlanID == nil || !frozenTrackPlanOK || !sameStringSlice(resolvedFallbackFormats(frozenTrackPlan), []string{"ass"}) {
		t.Fatalf("claim changed the reviewed configuration after acceptance: %#v", prepared)
	}
	second.QualityValue = 30
	if err := db.Save(&second).Error; err != nil {
		t.Fatal(err)
	}
	setFallbackFreezeTestSetting(t, db, false)
	if err := db.Model(&assignment).Update("video_profile_id", second.ID).Error; err != nil {
		t.Fatal(err)
	}
	again, err := NewWorkerHandler(db).prepareClaimedQueueJob(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if snapshotQualityValue(again.ProfileSnapshot) != 18 {
		t.Fatalf("completed freeze changed after profile edit: %#v", again.ProfileSnapshot)
	}
	againPlan, againPlanOK := ResolvedTrackPlanFromSnapshot(again.TrackProfileSnapshot)
	if !againPlanOK || !sameStringSlice(resolvedFallbackFormats(againPlan), []string{"ass"}) {
		t.Fatalf("completed fallback freeze changed after setting edit: %#v", again.TrackProfileSnapshot)
	}
	var scans int64
	if err := db.Model(&models.ScanResult{}).Where("path = ?", mediaPath).Count(&scans).Error; err != nil || scans != 1 {
		t.Fatalf("valid ScanResult was not reused: count=%d err=%v", scans, err)
	}
}

func TestQueueSelectedAssetsReportsReviewOpenJobAndMaintenanceSeparately(t *testing.T) {
	db := queueSelectedAssetsTestDB(t)
	logicalPath := "/media/raw/Anime/Blocked"
	records := []models.AssetRecord{
		{Path: logicalPath + "/Review.mkv", LogicalGroupPath: logicalPath, SourcePath: logicalPath + "/Review.mkv", FileName: "Review.mkv", Status: "unprocessed"},
		{Path: logicalPath + "/Queued.mkv", LogicalGroupPath: logicalPath, SourcePath: logicalPath + "/Queued.mkv", FileName: "Queued.mkv", Status: "unprocessed"},
		{Path: logicalPath + "/Maintenance.mkv", LogicalGroupPath: logicalPath, SourcePath: logicalPath + "/Maintenance.mkv", FileName: "Maintenance.mkv", Status: "unprocessed"},
	}
	if err := db.Create(&records).Error; err != nil {
		t.Fatal(err)
	}
	if err := saveAssetReviewOverrides(db, map[string]AssetReviewState{records[0].Path: {RequiresReview: true, Reason: "manual", Source: "manual", UpdatedAt: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.QueueJob{MediaPath: records[1].Path, Status: JobStatusQueued, Stage: JobStageQueued}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AssetMaintenanceOperation{ID: "maintenance-test", AssetPath: records[2].Path, Status: maintenanceStatusRunning, Phase: "rewrite"}).Error; err != nil {
		t.Fatal(err)
	}

	response := queueSelectedAssetsRequest(t, db, QueueSelectedAssetsInput{AssetIDs: []uint{records[0].ID, records[1].ID, records[2].ID}})
	reasons := map[uint]string{}
	for _, result := range response.Results {
		reasons[result.AssetID] = result.Reason
	}
	if reasons[records[0].ID] != "needs_review" || reasons[records[1].ID] != "already_queued" || reasons[records[2].ID] != "active_maintenance" {
		t.Fatalf("unexpected blocked results: %#v", response.Results)
	}
	if response.Summary.Skipped != 3 || response.Summary.Eligible != 0 || response.Summary.Failed != 0 {
		t.Fatalf("unexpected blocked summary: %#v", response.Summary)
	}
}

func TestQueueSelectedAssetsPreflightBlocksNonUnprocessedRecords(t *testing.T) {
	db := queueSelectedAssetsTestDB(t)
	profile := authoritativeTestProfile()
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	logicalPath := "/media/raw/Anime/Converted"
	record := models.AssetRecord{Path: logicalPath + "/Episode 1.mkv", LogicalGroupPath: logicalPath, SourcePath: logicalPath + "/Episode 1.mkv", FileName: "Episode 1.mkv", Status: "converted"}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.ProfileAssignment{TargetType: assetScopePath, TargetPath: filepath.Dir(record.Path), MediaType: "video", Selection: "profile", VideoProfileID: profile.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AssetScopeConfiguration{ScopeType: assetScopePath, ScopeKey: filepath.Dir(record.Path), DestinationSelection: configSelectionValue, DestinationLibraryID: 3}).Error; err != nil {
		t.Fatal(err)
	}

	candidate := createQueueSelectedAssetsPreflight(t, db, []uint{record.ID})
	if len(candidate.Assets) != 1 || candidate.Assets[0].Status != queuePreflightBlocked || !strings.Contains(strings.Join(candidate.Assets[0].Blockers, " "), "Unprocessed") {
		t.Fatalf("non-Unprocessed record was not shown as a Preflight blocker: %#v", candidate.Assets)
	}
	var jobs int64
	if err := db.Model(&models.QueueJob{}).Count(&jobs).Error; err != nil || jobs != 0 {
		t.Fatalf("non-Unprocessed record created Queue jobs=%d err=%v", jobs, err)
	}
}

func TestQueueSelectedAssetsPreflightFreezesInheritedAudioOnlySnapshot(t *testing.T) {
	db := queueSelectedAssetsTestDB(t)
	sourceDir := t.TempDir()
	logicalPath := filepath.Join(sourceDir, "Audio Only")
	mediaPath := filepath.Join(logicalPath, "Episode 1.mkv")
	if err := os.MkdirAll(logicalPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mediaPath, []byte("test media placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}
	fallback := authoritativeTestProfile()
	if err := db.Create(&fallback).Error; err != nil {
		t.Fatal(err)
	}
	record := models.AssetRecord{Path: mediaPath, LogicalGroupPath: logicalPath, SourcePath: mediaPath, FileName: filepath.Base(mediaPath), Status: "unprocessed"}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	assignments := []models.ProfileAssignment{
		{TargetType: assetScopeLogicalGroup, TargetPath: logicalPath, MediaType: "video", Selection: VideoAssignmentAudioOnly},
		{TargetType: assetScopeLogicalGroup, TargetPath: logicalPath, MediaType: "audio", Selection: "profile", ProfileKey: "audio-only-profile"},
	}
	if err := db.Create(&assignments).Error; err != nil {
		t.Fatal(err)
	}
	library := models.Library{Name: "Audio only destination", SourcePath: sourceDir, DestinationPath: t.TempDir(), Type: "anime"}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AssetScopeConfiguration{ScopeType: assetScopeLogicalGroup, ScopeKey: logicalPath, DestinationSelection: configSelectionValue, DestinationLibraryID: library.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "audioEnhancementProfiles", Value: models.JSONMap{"profiles": models.JSONList{models.JSONMap{"key": "audio-only-profile", "scope": "path", "filters": "volume=0.9"}}}}).Error; err != nil {
		t.Fatal(err)
	}
	seedQueueSelectedPreflightRuntime(t, db)

	candidate := createQueueSelectedAssetsPreflight(t, db, []uint{record.ID})
	if len(candidate.Assets) != 1 || candidate.Assets[0].Status != queuePreflightReady || candidate.Assets[0].Plan == nil {
		t.Fatalf("audio-only asset did not resolve in Preflight: %#v", candidate.Assets)
	}
	accepted := acceptQueueSelectedAssetsPreflight(t, db, candidate, []string{candidate.Assets[0].ID})
	if len(accepted.Jobs) != 1 {
		t.Fatalf("audio-only selection did not queue: %#v", accepted)
	}
	job := accepted.Jobs[0]
	videoResolution, _ := job.ProfileResolution["video"].(map[string]any)
	if job.ProcessingMode != ProcessingModeAudioOnly || job.ProfileID != fallback.ID || len(job.AudioProfileSnapshot) == 0 || job.ProfileCapturedAt == nil || !queueJobFreezeComplete(job) || videoResolution["selection"] != VideoAssignmentAudioOnly {
		t.Fatalf("audio-only Queue plan was not frozen at acceptance: %#v", job)
	}
}

func queueSelectedAssetsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + strings.NewReplacer("/", "-", " ", "-").Replace(t.Name()) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.AssetRecord{}, &models.Profile{}, &models.ProfileAssignment{}, &models.AssetScopeConfiguration{},
		&models.TestEncode{}, &models.ProfileQualification{},
		&models.QueueJob{}, &models.ExecutionPlan{}, &models.SchedulerReservation{}, &models.AppSetting{}, &models.ScanResult{}, &models.AssetMaintenanceOperation{}, &models.Library{}, &models.RuntimeSnapshot{}, &models.RuntimeState{},
		&models.QueueBatchPreflight{},
	); err != nil {
		t.Fatal(err)
	}
	return db
}

func queueSelectedAssetsRequest(t *testing.T, db *gorm.DB, input QueueSelectedAssetsInput) QueueSelectedAssetsResponse {
	t.Helper()
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/queue/selected-assets", NewQueueHandler(db).QueueSelectedAssets)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/queue/selected-assets", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var result QueueSelectedAssetsResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func queueSelectedAssetsPreflightRequest(t *testing.T, db *gorm.DB, method, path string, input any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewQueueHandler(db)
	router.POST("/api/queue/preflights/selected-assets", handler.PreflightSelectedAssets)
	router.POST("/api/queue/preflights/:id/accept", handler.AcceptBatchPreflight)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	return response
}

func createQueueSelectedAssetsPreflight(t *testing.T, db *gorm.DB, assetIDs []uint) QueueBatchPreflightResponse {
	t.Helper()
	response := queueSelectedAssetsPreflightRequest(t, db, http.MethodPost, "/api/queue/preflights/selected-assets", map[string]any{"assetIds": assetIDs})
	if response.Code != http.StatusCreated {
		t.Fatalf("preflight status=%d body=%s", response.Code, response.Body.String())
	}
	var candidate QueueBatchPreflightResponse
	if err := json.Unmarshal(response.Body.Bytes(), &candidate); err != nil {
		t.Fatal(err)
	}
	return candidate
}

func acceptQueueSelectedAssetsPreflight(t *testing.T, db *gorm.DB, candidate QueueBatchPreflightResponse, assetIDs []string) QueueBatchPreflightResponse {
	t.Helper()
	response := queueSelectedAssetsPreflightRequest(t, db, http.MethodPost, "/api/queue/preflights/"+candidate.ID+"/accept", map[string]any{"revision": candidate.Revision, "assetIds": assetIDs})
	if response.Code != http.StatusCreated {
		t.Fatalf("accept status=%d body=%s", response.Code, response.Body.String())
	}
	var accepted QueueBatchPreflightResponse
	if err := json.Unmarshal(response.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	return accepted
}

func seedQueueSelectedPreflightRuntime(t *testing.T, db *gorm.DB) {
	t.Helper()
	now := time.Now()
	encoders := models.JSONMap{"libx265": models.JSONMap{"listed": true, "usable": true}}
	if err := db.Create(&models.RuntimeSnapshot{DetectedAt: now, SelectedProfile: "test", Encoders: encoders}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.RuntimeState{SingletonKey: models.RuntimeStateSingletonKey, DetectedAt: now, SelectedProfile: "test", Encoders: encoders, StableFingerprint: "queue-selected-test", FingerprintVersion: "v1"}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestQueueCaptureFreezesFontAttachmentArtifacts(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open("file:queue-font-artifact-freeze?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.AutoMigrate(
		&models.Profile{},
		&models.QueueJob{},
		&models.SchedulerReservation{},
		&models.ScanResult{},
	); err != nil {
		t.Fatal(err)
	}

	profile := authoritativeTestProfile()
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}

	mediaPath := filepath.Clean("/media/raw/Movie.mkv")

	scan := models.ScanResult{
		Path: mediaPath,
		SubtitleStreams: models.JSONList{
			map[string]any{
				"index":    4,
				"codec":    "ass",
				"language": "eng",
			},
		},
		AttachmentStreams: models.JSONList{
			map[string]any{
				"index":          7,
				"codec":          "ttf",
				"filename":       "Font.ttf",
				"mimeType":       "font/ttf",
				"attachmentKind": "FONT",
				"fontFormat":     "TTF",
			},
		},
	}

	if err := db.Create(&scan).Error; err != nil {
		t.Fatal(err)
	}

	trackProfile := map[string]any{
		"trackDispositionVersion":    1,
		"subtitleDisposition":        "extract",
		"subtitleSidecarFormats":     models.JSONList{"original"},
		"attachmentPolicy":           "auto",
		"fontAttachmentExportPolicy": "all",
		"chapterPolicy":              "keep",
	}

	resolved, err := resolveTrackPlan(scan, trackProfile)
	if err != nil {
		t.Fatal(err)
	}

	if len(resolved.FontAttachments) != 1 {
		t.Fatalf(
			"expected one resolved font attachment, got %#v",
			resolved.FontAttachments,
		)
	}

	planMap, err := resolvedTrackPlanMap(resolved)
	if err != nil {
		t.Fatal(err)
	}

	trackSnapshot := models.JSONMap{
		"trackDispositionVersion":    1,
		"subtitleDisposition":        "extract",
		"subtitleSidecarFormats":     models.JSONList{"original"},
		"attachmentPolicy":           "auto",
		"fontAttachmentExportPolicy": "all",
		"chapterPolicy":              "keep",
		resolvedTrackPlanSnapshotKey: planMap,
	}

	job := models.QueueJob{
		MediaPath:            mediaPath,
		Status:               JobStatusQueued,
		TrackProfileSnapshot: trackSnapshot,
	}

	if err := NewQueueHandler(db).captureProfile(
		&job,
		profile.ID,
		"queue_create",
	); err != nil {
		t.Fatal(err)
	}

	artifacts := fontAttachmentArtifactsFromJSON(
		job.SubtitleArtifacts,
	)

	if len(artifacts) != 1 {
		t.Fatalf(
			"expected one frozen font artifact, got %#v",
			artifacts,
		)
	}

	artifact := artifacts[0]

	if artifact.ArtifactID != "font-attachment:7" {
		t.Fatalf(
			"ArtifactID=%q want=%q",
			artifact.ArtifactID,
			"font-attachment:7",
		)
	}

	if artifact.StreamIndex != 7 {
		t.Fatalf(
			"StreamIndex=%d want=7",
			artifact.StreamIndex,
		)
	}

	if artifact.AttachmentOrdinal != 0 {
		t.Fatalf(
			"AttachmentOrdinal=%d want=0",
			artifact.AttachmentOrdinal,
		)
	}

	if artifact.SafeFilename != "Font.ttf" {
		t.Fatalf(
			"SafeFilename=%q want=%q",
			artifact.SafeFilename,
			"Font.ttf",
		)
	}

	expectedRelativePath := filepath.Join(
		"Movie.fonts",
		"Font.ttf",
	)

	if artifact.RelativePath != expectedRelativePath {
		t.Fatalf(
			"RelativePath=%q want=%q",
			artifact.RelativePath,
			expectedRelativePath,
		)
	}
}
