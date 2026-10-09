package handlers

import (
	"context"
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
	if len(preview.Candidates) != 3 || len(preview.RemovedPaths) != 0 || preview.Summaries[0].RecoverableBytes != 8 {
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

func TestHousekeepingExpiresTestEncodeEvenWhenWorkRootDoesNotExist(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:housekeeping-test-encode?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.AppSetting{}, &models.Library{}, &models.TestEncode{}); err != nil {
		t.Fatal(err)
	}
	libraryRoot := t.TempDir()
	missingWorkRoot := filepath.Join(t.TempDir(), "missing-work")
	library := models.Library{Name: "TV", DestinationPath: libraryRoot}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	expired := time.Now().Add(-time.Hour)
	tests := []models.TestEncode{
		{SourcePath: "/raw/episode.mkv", Status: testEncodeReady, Phase: "ready", OutputPath: filepath.Join(libraryRoot, "ready-test.mkv")},
		{SourcePath: "/raw/failed.mkv", Status: testEncodeFailed, Phase: "failed", OutputPath: filepath.Join(libraryRoot, "failed-test.mkv")},
		{SourcePath: "/raw/canceled.mkv", Status: testEncodeCanceled, Phase: "canceled", OutputPath: filepath.Join(libraryRoot, "canceled-test.mkv")},
		{SourcePath: "/raw/kept.mkv", Status: testEncodeReady, Phase: "ready", OutputPath: filepath.Join(libraryRoot, "kept-test.mkv"), Keep: true},
	}
	for index := range tests {
		tests[index].LibraryID = library.ID
		tests[index].ConfigurationSource = "effective_asset"
		tests[index].OutputSizeBytes = 6
		tests[index].ExpiresAt = &expired
		if err := os.WriteFile(tests[index].OutputPath, []byte("sample"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&tests[index]).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&models.AppSetting{Key: "storageRoles", Value: models.JSONMap{"work": models.JSONMap{"path": missingWorkRoot}}}).Error; err != nil {
		t.Fatal(err)
	}
	report, err := RunHousekeeping(db, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.RemovedPaths) != 3 || report.RecoveredBytes != 18 {
		t.Fatalf("unexpected Test Encode cleanup report: %#v", report)
	}
	for index, test := range tests {
		_, err := os.Stat(test.OutputPath)
		if index == len(tests)-1 {
			if err != nil {
				t.Fatalf("Keep output was removed automatically: %v", err)
			}
			continue
		}
		if !os.IsNotExist(err) {
			t.Fatalf("expired output remains: %v", err)
		}
	}
	var test models.TestEncode
	if err := db.Unscoped().First(&test, tests[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if test.Status != "expired" || test.DeletedAt == nil || test.OutputPath != "" {
		t.Fatalf("unexpected Test Encode tombstone: %#v", test)
	}
}

func TestHousekeepingDoesNotDeleteActiveTestEncodeArtifacts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:housekeeping-active-test-encode?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.AppSetting{}, &models.Library{}, &models.TestEncode{}, &models.TestEncodeArtifact{}, &models.AssetRecord{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	workRoot := filepath.Join(t.TempDir(), "missing-work")
	library := models.Library{Name: "TV", DestinationPath: root}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "active-test.mkv")
	if err := os.WriteFile(output, []byte("active"), 0o600); err != nil {
		t.Fatal(err)
	}
	expired := time.Now().Add(-time.Hour)
	testEncode := models.TestEncode{SourcePath: "/raw/source.mkv", LibraryID: library.ID, ConfigurationSource: "global", ConfigurationHash: "active", Status: testEncodeReady, Phase: "ready", OutputPath: output, ExpiresAt: &expired}
	if err := db.Create(&testEncode).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.TestEncodeArtifact{TestEncodeID: testEncode.ID, LibraryID: library.ID, Path: output, Type: models.TestEncodeArtifactMediaOutput, Expected: true, State: "owned"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "storageRoles", Value: models.JSONMap{"work": models.JSONMap{"path": workRoot}}}).Error; err != nil {
		t.Fatal(err)
	}
	testEncodeProcesses.Lock()
	testEncodeProcesses.cancel[testEncode.ID] = context.CancelFunc(func() {})
	testEncodeProcesses.Unlock()
	t.Cleanup(func() {
		testEncodeProcesses.Lock()
		delete(testEncodeProcesses.cancel, testEncode.ID)
		testEncodeProcesses.Unlock()
	})
	report, err := RunHousekeeping(db, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.RemovedPaths) != 0 || len(report.Candidates) != 1 || report.Candidates[0].SafeToDelete || report.Candidates[0].BlockedReason == "" {
		t.Fatalf("active Test Encode appeared in automatic cleanup: %#v", report)
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("active Test Encode artifact was removed: %v", err)
	}
}
