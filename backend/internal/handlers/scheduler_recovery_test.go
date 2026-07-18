package handlers

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"github.com/anuelvs/mediaforge/backend/internal/scheduler"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRecoverSchedulerStateMarksInterruptedJobsAndPreservesFiles(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:recovery?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.SchedulerReservation{}, &models.WorkerNode{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	partial := filepath.Join(root, "job-1", "partial.mkv")
	if err := os.MkdirAll(filepath.Dir(partial), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(partial, []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "job-999"), 0o755); err != nil {
		t.Fatal(err)
	}
	roles := models.AppSetting{Key: "storageRoles", Value: models.JSONMap{"work": models.JSONMap{"path": root}}}
	if err := db.Create(&roles).Error; err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: "/raw/a.mkv", Status: JobStatusRunning, Stage: JobStageConverting, OutputPath: partial}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.SchedulerReservation{JobID: job.ID, AssetKey: job.MediaPath, State: scheduler.ReservationStateActive, WorkerName: "old-worker"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.WorkerNode{Name: "old-worker", Status: "online", LastSeenAt: time.Now().Add(-time.Hour), MaxConcurrentJobs: 1}).Error; err != nil {
		t.Fatal(err)
	}
	report, err := RecoverSchedulerState(db)
	if err != nil {
		t.Fatal(err)
	}
	if report.InterruptedJobs != 1 || report.PartialOutputsPreserved != 1 || report.ReservationsReleased != 1 || report.WorkersMarkedOffline != 1 || len(report.OrphanWorkspacePaths) != 1 {
		t.Fatalf("unexpected report: %#v", report)
	}
	if err := db.First(&job, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if job.Status != JobStatusFailed || job.Stage != JobStageFailed {
		t.Fatalf("job was not recovered: %#v", job)
	}
	if !fileHasBytes(partial) {
		t.Fatal("partial output was removed")
	}
}
