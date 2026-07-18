package handlers

import (
	"testing"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

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
