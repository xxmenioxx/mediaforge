package scheduler

import (
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCreatePendingExecutionPlanVersionsAndSupersedes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:execution-plan-versions?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.ExecutionPlan{}); err != nil {
		t.Fatal(err)
	}
	profile := executionPlanTestProfile(1, 18)
	snapshot, err := CaptureProfileSnapshot(profile, time.Now(), "queue_create")
	if err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{
		MediaPath: "/media/raw/priority.mkv", LibraryID: 1, ProfileID: profile.ID,
		ProfileVersion: 1, ProfileSnapshot: snapshot, Priority: 1, Status: "queued",
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}

	first, err := CreatePendingExecutionPlan(db, &job, "initial profile snapshot")
	if err != nil {
		t.Fatal(err)
	}
	if first.Version != 1 || first.Status != ExecutionPlanPendingEvaluation || job.ActiveExecutionPlanID == nil || *job.ActiveExecutionPlanID != first.ID {
		t.Fatalf("unexpected first plan: %#v job=%#v", first, job)
	}

	profile.ProfileVersion = 2
	profile.QualityValue = 22
	job.ProfileVersion = 2
	job.ProfileSnapshot, err = CaptureProfileSnapshot(profile, time.Now(), "queue_profile_update")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Save(&job).Error; err != nil {
		t.Fatal(err)
	}
	second, err := CreatePendingExecutionPlan(db, &job, "explicit profile update")
	if err != nil {
		t.Fatal(err)
	}
	if second.Version != 2 || second.QualityValue != 22 || *job.ActiveExecutionPlanID != second.ID {
		t.Fatalf("unexpected second plan: %#v job=%#v", second, job)
	}

	var storedFirst models.ExecutionPlan
	if err := db.First(&storedFirst, first.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedFirst.Status != ExecutionPlanSuperseded || storedFirst.SupersededAt == nil {
		t.Fatalf("first plan was not superseded: %#v", storedFirst)
	}
	var activeCount int64
	if err := db.Model(&models.ExecutionPlan{}).Where("job_id = ? AND status <> ?", job.ID, ExecutionPlanSuperseded).Count(&activeCount).Error; err != nil {
		t.Fatal(err)
	}
	if activeCount != 1 {
		t.Fatalf("expected one active plan, got %d", activeCount)
	}
}

func executionPlanTestProfile(version, quality int) models.Profile {
	return models.Profile{
		ID: 9, Name: "Priority Archive", ProfileVersion: version,
		Container: "mkv", VideoCodec: "x265_10bit", CodecFamily: "hevc",
		EncoderPolicy: EncoderPolicyLocked, PreferredEncoder: "libx265",
		AllowedEncoders: models.StringList{"libx265"}, FallbackPolicy: FallbackPolicyWait,
		BitDepth: 10, PixelFormat: "yuv420p10le", QualityStrategy: "high",
		QualityMode: "crf", QualityValue: quality, AudioCodec: "copy",
		WorkerConfig: models.JSONMap{"videoEncoder": "libx265"},
	}
}
