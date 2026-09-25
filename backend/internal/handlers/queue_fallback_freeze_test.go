package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestQueueClaimResolvesCurrentSubtitleFallbackBeforeFreeze(t *testing.T) {
	tests := []struct {
		name          string
		queuedSetting bool
		claimSetting  bool
		wantFormats   []string
	}{
		{name: "setting changed on before claim", queuedSetting: false, claimSetting: true, wantFormats: []string{"srt", "ass"}},
		{name: "setting changed off before claim", queuedSetting: true, claimSetting: false, wantFormats: []string{"ass"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, path := fallbackFreezeTestDB(t, test.queuedSetting)
			job := models.QueueJob{
				MediaPath:       path,
				Status:          JobStatusQueued,
				Stage:           JobStageQueued,
				TrackProfileKey: "tracks",
			}
			if queueJobFreezeComplete(job) || job.ActiveExecutionPlanID != nil {
				t.Fatal("queued intent unexpectedly contains a completed execution freeze")
			}

			setFallbackFreezeTestSetting(t, db, test.claimSetting)
			job.Status = JobStatusRunning
			job.Stage = JobStageClaimed
			if err := NewQueueHandler(db).captureSupplementalProfilesAtClaim(&job); err != nil {
				t.Fatal(err)
			}
			plan, ok := ResolvedTrackPlanFromSnapshot(job.TrackProfileSnapshot)
			if !ok {
				t.Fatalf("claim preparation did not freeze a canonical plan: %#v", job.TrackProfileSnapshot)
			}
			if got := resolvedFallbackFormats(plan); !sameStringSlice(got, test.wantFormats) {
				t.Fatalf("claim formats=%v want=%v", got, test.wantFormats)
			}
		})
	}
}

func TestDirectQueueCreationDefersFallbackFreezeUntilClaim(t *testing.T) {
	tests := []struct {
		name          string
		queuedSetting bool
		claimSetting  bool
		wantFormats   []string
	}{
		{name: "setting changed on before claim", queuedSetting: false, claimSetting: true, wantFormats: []string{"srt", "ass"}},
		{name: "setting changed off before claim", queuedSetting: true, claimSetting: false, wantFormats: []string{"ass"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, path, library, profile := queueCreationFreezeFixture(t, test.queuedSetting)
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.POST("/api/queue/jobs", NewQueueHandler(db).Create)
			body := fmt.Sprintf(`{"mediaPath":%q,"libraryId":%d,"profileId":%d,"trackProfileKey":"tracks"}`, path, library.ID, profile.ID)
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/queue/jobs", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(response, request)
			if response.Code != http.StatusCreated {
				t.Fatalf("queue create status=%d body=%s", response.Code, response.Body.String())
			}

			var job models.QueueJob
			if err := db.First(&job).Error; err != nil {
				t.Fatal(err)
			}
			if queueJobFreezeComplete(job) || job.ActiveExecutionPlanID != nil {
				t.Fatalf("direct Queue create froze execution state: %#v", job)
			}

			setFallbackFreezeTestSetting(t, db, test.claimSetting)
			now := time.Now()
			if err := db.Model(&job).Updates(map[string]any{
				"status": JobStatusRunning, "stage": JobStageClaimed,
				"worker_name": "test-worker", "started_at": &now,
			}).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.First(&job, job.ID).Error; err != nil {
				t.Fatal(err)
			}
			prepared, err := NewWorkerHandler(db).prepareClaimedQueueJob(job)
			if err != nil {
				t.Fatal(err)
			}
			plan, ok := ResolvedTrackPlanFromSnapshot(prepared.TrackProfileSnapshot)
			if !ok || !sameStringSlice(resolvedFallbackFormats(plan), test.wantFormats) || prepared.ActiveExecutionPlanID == nil {
				t.Fatalf("claim did not freeze current fallback setting: plan=%#v job=%#v", plan, prepared)
			}
		})
	}
}

func TestExplicitQueueIntentPreservesProfileWhenClaimAssignmentDiffers(t *testing.T) {
	db, path, library, profile := queueCreationFreezeFixture(t, false)
	claimProfile := authoritativeTestProfile()
	claimProfile.Name = "Claim assignment profile"
	if err := db.Create(&claimProfile).Error; err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/queue/jobs", NewQueueHandler(db).Create)
	body := fmt.Sprintf(`{"mediaPath":%q,"libraryId":%d,"profileId":%d,"trackProfileKey":"tracks"}`, path, library.ID, profile.ID)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/queue/jobs", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("queue create status=%d body=%s", response.Code, response.Body.String())
	}
	var job models.QueueJob
	if err := db.First(&job).Error; err != nil {
		t.Fatal(err)
	}
	var record models.AssetRecord
	if err := db.Where("path = ?", path).First(&record).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.ProfileAssignment{TargetType: assetScopeLogicalGroup, TargetPath: record.LogicalGroupPath, MediaType: "video", Selection: "profile", VideoProfileID: claimProfile.ID}).Error; err != nil {
		t.Fatal(err)
	}
	setFallbackFreezeTestSetting(t, db, true)
	markQueueJobClaimed(t, db, &job)
	prepared, err := NewWorkerHandler(db).prepareClaimedQueueJob(job)
	if err != nil {
		t.Fatal(err)
	}
	plan, ok := ResolvedTrackPlanFromSnapshot(prepared.TrackProfileSnapshot)
	if !ok || prepared.ProfileID != profile.ID || prepared.TrackProfileKey != "tracks" || !sameStringSlice(resolvedFallbackFormats(plan), []string{"srt", "ass"}) {
		t.Fatalf("explicit Queue intent was replaced by claim assignment: profile=%d track=%q plan=%#v", prepared.ProfileID, prepared.TrackProfileKey, plan)
	}
}

func TestExplicitBatchQueueIntentFreezesOnlyAtClaim(t *testing.T) {
	db, path, library, profile := queueCreationFreezeFixture(t, false)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/queue/batches", NewQueueHandler(db).CreateBatch)
	body := fmt.Sprintf(`{"batchId":"explicit-batch","batchName":"Season 1","jobs":[{"mediaPath":%q,"libraryId":%d,"profileId":%d,"trackProfileKey":"tracks"}]}`, path, library.ID, profile.ID)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/queue/batches", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("batch create status=%d body=%s", response.Code, response.Body.String())
	}
	var job models.QueueJob
	if err := db.First(&job).Error; err != nil {
		t.Fatal(err)
	}
	if queueJobFreezeComplete(job) || job.ActiveExecutionPlanID != nil || job.ProfileID != profile.ID || job.TrackProfileKey != "tracks" {
		t.Fatalf("explicit batch intent was frozen or changed at enqueue: %#v", job)
	}
	setFallbackFreezeTestSetting(t, db, true)
	markQueueJobClaimed(t, db, &job)
	prepared, err := NewWorkerHandler(db).prepareClaimedQueueJob(job)
	if err != nil {
		t.Fatal(err)
	}
	plan, ok := ResolvedTrackPlanFromSnapshot(prepared.TrackProfileSnapshot)
	if !ok || prepared.ProfileID != profile.ID || prepared.TrackProfileKey != "tracks" || prepared.ActiveExecutionPlanID == nil || !sameStringSlice(resolvedFallbackFormats(plan), []string{"srt", "ass"}) {
		t.Fatalf("explicit batch intent was not preserved at claim: job=%#v plan=%#v", prepared, plan)
	}
}

func TestWorkerUsesFrozenFallbackPlanAfterSettingChanges(t *testing.T) {
	db, path := fallbackFreezeTestDB(t, true)
	job := models.QueueJob{MediaPath: path, TrackProfileKey: "tracks", Status: JobStatusRunning, Stage: JobStageClaimed}
	if err := NewQueueHandler(db).captureSupplementalProfilesAtClaim(&job); err != nil {
		t.Fatal(err)
	}
	frozen, ok := ResolvedTrackPlanFromSnapshot(job.TrackProfileSnapshot)
	if !ok || !sameStringSlice(resolvedFallbackFormats(frozen), []string{"srt", "ass"}) {
		t.Fatalf("unexpected frozen fallback plan: %#v", job.TrackProfileSnapshot)
	}
	setFallbackFreezeTestSetting(t, db, false)
	if err := db.Model(&models.AppSetting{}).Where("key = ?", subtitleSettingsKey).Delete(&models.AppSetting{}).Error; err != nil {
		t.Fatal(err)
	}
	job.ProfileSnapshot = models.JSONMap{"frozen": true}
	if !queueJobFreezeComplete(job) {
		t.Fatal("test job did not represent a completed execution freeze")
	}
	completedRetry, err := NewWorkerHandler(db).prepareClaimedQueueJob(job)
	if err != nil {
		t.Fatal(err)
	}
	completedPlan, ok := ResolvedTrackPlanFromSnapshot(completedRetry.TrackProfileSnapshot)
	if !ok || !sameStringSlice(resolvedFallbackFormats(completedPlan), []string{"srt", "ass"}) {
		t.Fatalf("completed-freeze retry re-resolved from current settings: %#v", completedRetry.TrackProfileSnapshot)
	}
	override := currentConversionOverrideForJob(job, nil)
	if override.ResolvedTrackPlan == nil || !sameStringSlice(resolvedFallbackFormats(override.ResolvedTrackPlan), []string{"srt", "ass"}) {
		t.Fatalf("worker did not consume the frozen fallback plan: %#v", override.ResolvedTrackPlan)
	}
}

func TestQueueRetryResolvesOnlyIncompleteFallbackFreeze(t *testing.T) {
	db, path, library, profile := queueCreationFreezeFixture(t, true)
	job := models.QueueJob{
		MediaPath: path, LibraryID: library.ID, ProfileID: profile.ID,
		TrackProfileKey: "tracks", Status: JobStatusRunning, Stage: JobStageClaimed,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	setFallbackFreezeTestSetting(t, db, false)
	prepared, err := NewWorkerHandler(db).prepareClaimedQueueJob(job)
	if err != nil {
		t.Fatal(err)
	}
	plan, ok := ResolvedTrackPlanFromSnapshot(prepared.TrackProfileSnapshot)
	if !ok || !sameStringSlice(resolvedFallbackFormats(plan), []string{"ass"}) || prepared.ActiveExecutionPlanID == nil {
		t.Fatalf("incomplete-freeze retry did not use current setting: plan=%#v job=%#v", plan, prepared)
	}

	setFallbackFreezeTestSetting(t, db, true)
	futureJob := models.QueueJob{MediaPath: path, TrackProfileKey: "tracks", Status: JobStatusRunning, Stage: JobStageClaimed}
	if err := NewQueueHandler(db).captureSupplementalProfilesAtClaim(&futureJob); err != nil {
		t.Fatal(err)
	}
	plan, ok = ResolvedTrackPlanFromSnapshot(futureJob.TrackProfileSnapshot)
	if !ok || !sameStringSlice(resolvedFallbackFormats(plan), []string{"srt", "ass"}) {
		t.Fatalf("new/future job did not use current setting: %#v", futureJob.TrackProfileSnapshot)
	}
}

func fallbackFreezeTestDB(t *testing.T, fallback bool) (*gorm.DB, string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:queue-fallback-freeze-"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}, &models.ScanResult{}, &models.RuntimeState{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.RuntimeState{SingletonKey: models.RuntimeStateSingletonKey, DetectedAt: time.Now(), SelectedProfile: "desktop_safe", Encoders: models.JSONMap{"libx265": models.JSONMap{"listed": true, "usable": true}}, StableFingerprint: "queue-fallback-test", FingerprintVersion: "v1"}).Error; err != nil {
		t.Fatal(err)
	}
	path := filepath.Clean("/media/raw/show/episode.mkv")
	if err := db.Create(&models.ScanResult{
		Path: path,
		SubtitleStreams: models.JSONList{map[string]any{
			"index": 4, "codec": "ass", "language": "eng", "title": "Full",
		}},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{
		Key: "trackProfiles",
		Value: models.JSONMap{"profiles": models.JSONList{models.JSONMap{
			"key": "tracks", "scope": "path", "trackDispositionVersion": 1,
			"subtitleDisposition": "extract", "subtitleSidecarFormats": models.JSONList{"ass"},
			"subtitleRules":    models.JSONList{models.JSONMap{"streamIndex": 4, "action": "extract", "sidecarFormats": models.JSONList{"ass"}}},
			"attachmentPolicy": "auto", "chapterPolicy": "keep",
		}}},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{
		Key:   subtitleSettingsKey,
		Value: models.JSONMap{generateSRTFallbackForASSKey: fallback},
	}).Error; err != nil {
		t.Fatal(err)
	}
	return db, path
}

func queueCreationFreezeFixture(t *testing.T, fallback bool) (*gorm.DB, string, models.Library, models.Profile) {
	t.Helper()
	db := queueSelectedAssetsTestDB(t)
	sourceDir := t.TempDir()
	path := filepath.Join(sourceDir, "episode.mkv")
	library := models.Library{Name: "Test Library", SourcePath: sourceDir, DestinationPath: t.TempDir(), Type: "anime"}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	profile := authoritativeTestProfile()
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AssetRecord{Path: path, SourcePath: path, LogicalGroupPath: sourceDir, FileName: filepath.Base(path), Status: "unprocessed"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.ScanResult{Path: path, VideoCodec: "h264", Width: 720, Height: 480,
		VideoStreams:    models.JSONList{map[string]any{"index": 0, "codec": "h264", "width": 720, "height": 480}},
		SubtitleStreams: models.JSONList{map[string]any{"index": 4, "codec": "ass", "language": "eng", "title": "Full"}},
	}).Error; err != nil {
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
	if err := db.Create(&models.AppSetting{Key: subtitleSettingsKey, Value: models.JSONMap{generateSRTFallbackForASSKey: fallback}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "pipelineAutomation", Value: models.JSONMap{"reviewMode": "automatic"}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.RuntimeSnapshot{DetectedAt: time.Now(), SelectedProfile: "test", Encoders: models.JSONMap{"libx265": models.JSONMap{"listed": true, "usable": true}}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.RuntimeState{SingletonKey: models.RuntimeStateSingletonKey, DetectedAt: time.Now(), SelectedProfile: "test", Encoders: models.JSONMap{"libx265": models.JSONMap{"listed": true, "usable": true}}, StableFingerprint: "queue-creation-test", FingerprintVersion: "v1"}).Error; err != nil {
		t.Fatal(err)
	}
	return db, path, library, profile
}

func setFallbackFreezeTestSetting(t *testing.T, db *gorm.DB, enabled bool) {
	t.Helper()
	setting := models.AppSetting{Key: subtitleSettingsKey, Value: models.JSONMap{generateSRTFallbackForASSKey: enabled}}
	if err := db.Where("key = ?", subtitleSettingsKey).Delete(&models.AppSetting{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&setting).Error; err != nil {
		t.Fatal(err)
	}
}

func markQueueJobClaimed(t *testing.T, db *gorm.DB, job *models.QueueJob) {
	t.Helper()
	now := time.Now()
	if err := db.Model(job).Updates(map[string]any{
		"status": JobStatusRunning, "stage": JobStageClaimed,
		"worker_name": "test-worker", "started_at": &now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(job, job.ID).Error; err != nil {
		t.Fatal(err)
	}
}

func resolvedFallbackFormats(plan *ResolvedTrackPlan) []string {
	if plan == nil {
		return nil
	}
	formats := make([]string, 0, len(plan.SidecarOutputs))
	for _, output := range plan.SidecarOutputs {
		formats = append(formats, output.Format)
	}
	return formats
}

func sameStringSlice(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
