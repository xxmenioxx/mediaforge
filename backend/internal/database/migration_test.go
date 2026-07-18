package database

import (
	"path/filepath"
	"testing"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMigrateUpgradesLegacyQueueWithoutLosingJob(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Profile{}); err != nil {
		t.Fatal(err)
	}
	profile := models.Profile{Name: "Legacy", Container: "mkv", VideoCodec: "x265_10bit", AudioCodec: "copy", QualityMode: "crf", QualityValue: 20, WorkerConfig: models.JSONMap{"videoEncoder": "libx265"}}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	legacySchema := `CREATE TABLE queue_jobs (id integer primary key autoincrement, media_path text not null, library_id integer not null, profile_id integer not null, priority integer default 5, status text default 'queued', progress integer default 0, created_at datetime, updated_at datetime)`
	if err := db.Exec(legacySchema).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO queue_jobs (media_path, library_id, profile_id, status, priority, created_at, updated_at) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)", "/raw/legacy.mkv", 1, profile.ID, "queued", 5).Error; err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	var job models.QueueJob
	if err := db.First(&job).Error; err != nil {
		t.Fatal(err)
	}
	if job.MediaPath != "/raw/legacy.mkv" || job.Stage != "queued" || len(job.ProfileSnapshot) == 0 || job.ActiveExecutionPlanID == nil {
		t.Fatalf("legacy job was not fully upgraded: %#v", job)
	}
	var reservation models.SchedulerReservation
	if err := db.Where("job_id = ?", job.ID).First(&reservation).Error; err != nil {
		t.Fatal(err)
	}
	if reservation.AssetKey != job.MediaPath {
		t.Fatalf("reservation was not backfilled: %#v", reservation)
	}
}
