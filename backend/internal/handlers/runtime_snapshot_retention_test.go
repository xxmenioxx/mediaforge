package handlers

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openRuntimeSnapshotRetentionDB(t *testing.T, modelsToMigrate ...any) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "retention.db")+"?_busy_timeout=5000&_foreign_keys=on"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(modelsToMigrate...); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestReferencedRuntimeSnapshotIDsCoversPlanAndTestEncode(t *testing.T) {
	db := openRuntimeSnapshotRetentionDB(t, &models.RuntimeSnapshot{}, &models.ExecutionPlan{}, &models.TestEncode{})
	first := models.RuntimeSnapshot{DetectedAt: time.Now().Add(-9 * 24 * time.Hour)}
	second := models.RuntimeSnapshot{DetectedAt: time.Now().Add(-8 * 24 * time.Hour)}
	third := models.RuntimeSnapshot{DetectedAt: time.Now().Add(-7 * 24 * time.Hour)}
	for _, snapshot := range []*models.RuntimeSnapshot{&first, &second, &third} {
		if err := db.Create(snapshot).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&models.ExecutionPlan{JobID: 1, Version: 1, Status: "waiting", RuntimeSnapshotID: &first.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.TestEncode{SourcePath: "/source.mkv", LibraryID: 1, ConfigurationSource: "test", ConfigurationHash: "hash", Status: "completed", Phase: "completed", RuntimeSnapshotID: &second.ID}).Error; err != nil {
		t.Fatal(err)
	}
	ids, err := ReferencedRuntimeSnapshotIDs(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected two distinct references, got %#v", ids)
	}
	for _, id := range []uint{first.ID, second.ID} {
		if _, ok := ids[id]; !ok {
			t.Fatalf("missing referenced snapshot %d: %#v", id, ids)
		}
	}
	if _, ok := ids[third.ID]; ok {
		t.Fatalf("unreferenced snapshot was reported as protected: %#v", ids)
	}
}

func TestRuntimeSnapshotRetentionUsesSevenDayAgeAndBoundedBatches(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)

	t.Run("old rows are eligible below former count threshold", func(t *testing.T) {
		db := openRuntimeSnapshotRetentionDB(t, &models.RuntimeSnapshot{}, &models.ExecutionPlan{}, &models.TestEncode{})
		for i := 0; i < 50; i++ {
			snapshot := models.RuntimeSnapshot{DetectedAt: now.Add(-runtimeSnapshotRetentionAge - time.Duration(i+1)*time.Minute)}
			if err := db.Create(&snapshot).Error; err != nil {
				t.Fatal(err)
			}
		}
		if err := db.Create(&models.RuntimeSnapshot{DetectedAt: now}).Error; err != nil {
			t.Fatal(err)
		}
		preview, err := pruneRuntimeSnapshots(db, now, true)
		if err != nil {
			t.Fatal(err)
		}
		if preview.PruneCandidates != 50 || preview.Deleted != 0 {
			t.Fatalf("unexpected preview: %#v", preview)
		}
		var count int64
		if err := db.Model(&models.RuntimeSnapshot{}).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 51 {
			t.Fatalf("dry-run changed row count: %d", count)
		}
	})

	t.Run("the seven day boundary is eligible", func(t *testing.T) {
		db := openRuntimeSnapshotRetentionDB(t, &models.RuntimeSnapshot{})
		if err := db.Create(&models.RuntimeSnapshot{DetectedAt: now.Add(-runtimeSnapshotRetentionAge)}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&models.RuntimeSnapshot{DetectedAt: now}).Error; err != nil {
			t.Fatal(err)
		}
		report, err := pruneRuntimeSnapshots(db, now, true)
		if err != nil {
			t.Fatal(err)
		}
		if report.PruneCandidates != 1 {
			t.Fatalf("exactly seven-day-old snapshot was not eligible: %#v", report)
		}
	})

	t.Run("recent rows are protected by age", func(t *testing.T) {
		db := openRuntimeSnapshotRetentionDB(t, &models.RuntimeSnapshot{})
		for i := 0; i < 500; i++ {
			snapshot := models.RuntimeSnapshot{DetectedAt: now.Add(-time.Duration(i+1) * time.Minute)}
			if err := db.Create(&snapshot).Error; err != nil {
				t.Fatal(err)
			}
		}
		report, err := pruneRuntimeSnapshots(db, now, false)
		if err != nil {
			t.Fatal(err)
		}
		if report.PruneCandidates != 0 || report.Deleted != 0 || report.RecentUnreferencedKept != 500 {
			t.Fatalf("recent rows were not fully protected: %#v", report)
		}
	})

	t.Run("old rows delete oldest first in batches of one hundred", func(t *testing.T) {
		db := openRuntimeSnapshotRetentionDB(t, &models.RuntimeSnapshot{})
		oldest := make([]uint, 0, 500)
		for i := 0; i < 500; i++ {
			snapshot := models.RuntimeSnapshot{DetectedAt: now.Add(-runtimeSnapshotRetentionAge - time.Duration(500-i)*time.Minute)}
			if err := db.Create(&snapshot).Error; err != nil {
				t.Fatal(err)
			}
			oldest = append(oldest, snapshot.ID)
		}
		latest := models.RuntimeSnapshot{DetectedAt: now}
		if err := db.Create(&latest).Error; err != nil {
			t.Fatal(err)
		}
		report, err := pruneRuntimeSnapshots(db, now, false)
		if err != nil {
			t.Fatal(err)
		}
		if report.PruneCandidates != 100 || report.Deleted != 100 {
			t.Fatalf("unexpected bounded deletion report: %#v", report)
		}
		var remaining int64
		if err := db.Model(&models.RuntimeSnapshot{}).Count(&remaining).Error; err != nil {
			t.Fatal(err)
		}
		if remaining != 401 {
			t.Fatalf("expected 400 old rows plus newest row, got %d", remaining)
		}
		for _, id := range oldest[:100] {
			if err := db.First(&models.RuntimeSnapshot{}, id).Error; err == nil {
				t.Fatalf("oldest snapshot %d was not deleted", id)
			}
		}
		if err := db.First(&models.RuntimeSnapshot{}, oldest[100]).Error; err != nil {
			t.Fatalf("snapshot after first batch was deleted: %v", err)
		}
		if err := db.First(&models.RuntimeSnapshot{}, latest.ID).Error; err != nil {
			t.Fatalf("newest snapshot was deleted: %v", err)
		}
	})

	t.Run("large reference history stays database-scoped and bounded", func(t *testing.T) {
		db := openRuntimeSnapshotRetentionDB(t, &models.RuntimeSnapshot{}, &models.ExecutionPlan{}, &models.TestEncode{})
		snapshots := make([]models.RuntimeSnapshot, 600)
		for i := range snapshots {
			snapshots[i] = models.RuntimeSnapshot{DetectedAt: now.Add(-runtimeSnapshotRetentionAge - time.Duration(600-i)*time.Minute)}
			if err := db.Create(&snapshots[i]).Error; err != nil {
				t.Fatal(err)
			}
		}
		for i := 0; i < 500; i++ {
			if err := db.Create(&models.ExecutionPlan{JobID: uint(i + 1), Version: 1, Status: "waiting", RuntimeSnapshotID: &snapshots[i].ID}).Error; err != nil {
				t.Fatal(err)
			}
		}
		latest := models.RuntimeSnapshot{DetectedAt: now}
		if err := db.Create(&latest).Error; err != nil {
			t.Fatal(err)
		}

		report, err := pruneRuntimeSnapshots(db, now, false)
		if err != nil {
			t.Fatal(err)
		}
		if report.PruneCandidates != runtimeSnapshotRetentionBatchSize || report.Deleted != runtimeSnapshotRetentionBatchSize {
			t.Fatalf("large history exceeded bounded batch: %#v", report)
		}
		for i := 0; i < 500; i++ {
			if err := db.First(&models.RuntimeSnapshot{}, snapshots[i].ID).Error; err != nil {
				t.Fatalf("referenced snapshot %d was deleted: %v", snapshots[i].ID, err)
			}
		}
	})
}

func TestRuntimeSnapshotRetentionRechecksReferencesAfterPreview(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	db := openRuntimeSnapshotRetentionDB(t, &models.RuntimeSnapshot{}, &models.ExecutionPlan{}, &models.TestEncode{})
	candidate := models.RuntimeSnapshot{DetectedAt: now.Add(-runtimeSnapshotRetentionAge - time.Hour)}
	latest := models.RuntimeSnapshot{DetectedAt: now}
	if err := db.Create(&candidate).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&latest).Error; err != nil {
		t.Fatal(err)
	}
	preview, err := pruneRuntimeSnapshots(db, now, true)
	if err != nil {
		t.Fatal(err)
	}
	if preview.PruneCandidates != 1 {
		t.Fatalf("expected one preview candidate, got %#v", preview)
	}
	if err := db.Create(&models.ExecutionPlan{JobID: 99, Version: 1, Status: "waiting", RuntimeSnapshotID: &candidate.ID}).Error; err != nil {
		t.Fatal(err)
	}
	run, err := pruneRuntimeSnapshots(db, now, false)
	if err != nil {
		t.Fatal(err)
	}
	if run.Deleted != 0 || run.ReferencedKept != 1 {
		t.Fatalf("stale preview deleted a newly referenced row: %#v", run)
	}
	if err := db.First(&models.RuntimeSnapshot{}, candidate.ID).Error; err != nil {
		t.Fatalf("referenced snapshot was deleted: %v", err)
	}
}

func TestRunHousekeepingReportsRuntimeRowsWithoutFilesystemBytes(t *testing.T) {
	now := time.Now().UTC()
	db := openRuntimeSnapshotRetentionDB(t, &models.QueueJob{}, &models.AppSetting{}, &models.RuntimeSnapshot{})
	workRoot := t.TempDir()
	if err := db.Create(&models.AppSetting{Key: "storageRoles", Value: models.JSONMap{"work": models.JSONMap{"path": workRoot}}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.RuntimeSnapshot{DetectedAt: now.Add(-8 * 24 * time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.RuntimeSnapshot{DetectedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	preview, err := RunHousekeeping(db, true)
	if err != nil {
		t.Fatal(err)
	}
	if preview.RuntimeSnapshots.RowsConsidered != 2 || preview.RuntimeSnapshots.PruneCandidates != 1 || preview.RuntimeSnapshots.Deleted != 0 || preview.RecoveredBytes != 0 {
		t.Fatalf("unexpected runtime preview report: %#v", preview)
	}
	var count int64
	if err := db.Model(&models.RuntimeSnapshot{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("dry-run removed runtime rows: %d", count)
	}
	report, err := RunHousekeeping(db, false)
	if err != nil {
		t.Fatal(err)
	}
	if report.RuntimeSnapshots.Deleted != 1 || report.RecoveredBytes != 0 {
		t.Fatalf("unexpected runtime run report: %#v", report)
	}
}
