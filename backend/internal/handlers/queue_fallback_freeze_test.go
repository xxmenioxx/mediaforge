package handlers

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
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
	db, path := fallbackFreezeTestDB(t, true)
	job := models.QueueJob{MediaPath: path, TrackProfileKey: "tracks", Status: JobStatusRunning, Stage: JobStageClaimed}
	setFallbackFreezeTestSetting(t, db, false)
	if err := NewQueueHandler(db).captureSupplementalProfilesAtClaim(&job); err != nil {
		t.Fatal(err)
	}
	plan, ok := ResolvedTrackPlanFromSnapshot(job.TrackProfileSnapshot)
	if !ok || !sameStringSlice(resolvedFallbackFormats(plan), []string{"ass"}) {
		t.Fatalf("incomplete-freeze retry did not use current setting: %#v", plan)
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
	if err := db.AutoMigrate(&models.AppSetting{}, &models.ScanResult{}); err != nil {
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
