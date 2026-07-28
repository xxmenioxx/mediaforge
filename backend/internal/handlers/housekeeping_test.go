package handlers

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestHousekeepingPreviewAndRunRespectWorkspaceBoundary(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:housekeeping?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	old := time.Now().Add(-10 * 24 * time.Hour)
	failed := models.QueueJob{MediaPath: "/raw/failed.mkv", Status: JobStatusFailed}
	running := models.QueueJob{MediaPath: "/raw/running.mkv", Status: JobStatusRunning}
	if err := db.Create(&failed).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&running).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&failed).UpdateColumn("updated_at", old).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint{failed.ID, running.ID, 999} {
		path := filepath.Join(root, "job-"+strconv.FormatUint(uint64(id), 10))
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "data.bin"), []byte("1234"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	outside := filepath.Join(t.TempDir(), "keep.bin")
	if err := os.WriteFile(outside, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	settings := []models.AppSetting{
		{Key: "storageRoles", Value: models.JSONMap{"work": models.JSONMap{"path": root}}},
		{Key: "housekeeping", Value: models.JSONMap{"autoEnabled": true, "intervalHours": 24, "failedRetentionDays": 7, "canceledRetentionDays": 3, "orphanRetentionDays": 7}},
	}
	if err := db.Create(&settings).Error; err != nil {
		t.Fatal(err)
	}
	preview, err := RunHousekeeping(db, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Candidates) != 2 || len(preview.RemovedPaths) != 0 {
		t.Fatalf("unexpected preview: %#v", preview)
	}
	if _, err := os.Stat(filepath.Join(root, "job-"+strconv.FormatUint(uint64(failed.ID), 10))); err != nil {
		t.Fatal("dry run removed workspace")
	}
	report, err := RunHousekeeping(db, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.RemovedPaths) != 2 || report.RecoveredBytes != 8 {
		t.Fatalf("unexpected cleanup: %#v", report)
	}
	if _, err := os.Stat(filepath.Join(root, "job-"+strconv.FormatUint(uint64(running.ID), 10))); err != nil {
		t.Fatal("running workspace was removed")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("file outside workspace was removed")
	}
}

func TestCanceledRetentionUsesTerminalJobTimeNotWorkspaceModification(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:housekeeping-canceled-time?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-4 * 24 * time.Hour)
	job := models.QueueJob{MediaPath: "/raw/canceled.mkv", Status: JobStatusCanceled, FinishedAt: &old}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&job).Updates(map[string]any{"updated_at": old, "finished_at": old}).Error; err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(t.TempDir(), "job-1")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	policy := HousekeepingPolicy{CanceledRetentionDays: 3}
	candidate, eligible, err := housekeepingCandidate(db, job.ID, workspace, time.Now(), policy, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !eligible || candidate.Reason != "canceled job retention expired" {
		t.Fatalf("recent workspace metadata incorrectly extended retention: eligible=%v candidate=%#v", eligible, candidate)
	}
}
