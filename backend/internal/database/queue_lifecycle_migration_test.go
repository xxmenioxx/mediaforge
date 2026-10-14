package database

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMigrateReconcilesQueueStageFromHistoryIdempotently(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "queue-lifecycle.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("initial migration: %v", err)
	}

	validatingAt := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	reviewAt := validatingAt.Add(time.Minute)
	readyAt := reviewAt.Add(time.Minute)
	backfillAt := readyAt.Add(time.Minute)
	publishedAt := readyAt.Add(2 * time.Minute)
	jobs := []models.QueueJob{
		{
			MediaPath: "/raw/warning.mkv", LibraryID: 1, ProfileID: 1, Status: "completed", Stage: "ready_to_publish",
			StageUpdatedAt: &backfillAt, StageHistory: lifecycleHistory("validating", validatingAt, "validation_review", reviewAt),
			ValidationRunID: "validation-run:warning", ValidationOutcome: "PASS_WITH_WARNINGS", ValidationActionable: true,
		},
		{
			MediaPath: "/raw/pass.mkv", LibraryID: 1, ProfileID: 1, Status: "completed", Stage: "ready_to_publish",
			StageUpdatedAt: &readyAt, StageHistory: lifecycleHistory("validating", validatingAt, "ready_to_publish", readyAt),
			ValidationRunID: "validation-run:pass", ValidationOutcome: "PASS", ValidationActionable: false,
		},
		{MediaPath: "/raw/legacy.mkv", LibraryID: 1, ProfileID: 1, Status: "completed", Stage: "queued"},
		{
			MediaPath: "/raw/invalid-stage.mkv", LibraryID: 1, ProfileID: 1, Status: "completed", Stage: "ready_to_publish",
			StageUpdatedAt: &backfillAt, StageHistory: models.JSONList{models.JSONMap{"stage": "not_a_stage", "at": reviewAt.Format(time.RFC3339)}},
		},
		{
			MediaPath: "/raw/invalid-time.mkv", LibraryID: 1, ProfileID: 1, Status: "completed", Stage: "ready_to_publish",
			StageUpdatedAt: &backfillAt, StageHistory: models.JSONList{models.JSONMap{"stage": "validation_review", "at": "not-a-time"}},
		},
		{
			MediaPath: "/raw/out-of-order.mkv", LibraryID: 1, ProfileID: 1, Status: "completed", Stage: "ready_to_publish",
			StageUpdatedAt: &backfillAt, StageHistory: lifecycleHistory("validation_review", reviewAt, "validating", validatingAt),
		},
		{
			MediaPath: "/raw/newer-stage.mkv", LibraryID: 1, ProfileID: 1, Status: "completed", Stage: "archiving_original",
			StageUpdatedAt: &backfillAt, StageHistory: lifecycleHistory("ready_to_publish", readyAt),
		},
		{
			MediaPath: "/raw/published.mkv", LibraryID: 1, ProfileID: 1, Status: "completed", Stage: "ready_to_publish",
			StageUpdatedAt: &backfillAt, StageHistory: lifecycleHistory("validation_review", reviewAt),
			PublishedAt: &publishedAt, PublishedPath: "/library/published.mkv", PublicationState: "PUBLISHED",
		},
		{
			MediaPath: "/raw/terminal.mkv", LibraryID: 1, ProfileID: 1, Status: "failed", Stage: "failed",
			StageUpdatedAt: &backfillAt, StageHistory: lifecycleHistory("converting", validatingAt),
		},
	}
	for i := range jobs {
		if err := db.Create(&jobs[i]).Error; err != nil {
			t.Fatalf("create job %s: %v", jobs[i].MediaPath, err)
		}
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("reconciliation migration: %v", err)
	}
	var warning models.QueueJob
	if err := db.First(&warning, "media_path = ?", "/raw/warning.mkv").Error; err != nil {
		t.Fatal(err)
	}
	if warning.Stage != "validation_review" || warning.StageUpdatedAt == nil || !warning.StageUpdatedAt.Equal(reviewAt) {
		t.Fatalf("warning job stage was not restored from persisted review event: stage=%q at=%v", warning.Stage, warning.StageUpdatedAt)
	}
	if warning.ValidationOutcome != "PASS_WITH_WARNINGS" || !warning.ValidationActionable || warning.ValidationRunID != "validation-run:warning" {
		t.Fatalf("Validation evidence changed during stage reconciliation: %#v", warning)
	}
	if len(warning.StageHistory) != 2 || warning.StageHistory[1].(map[string]any)["stage"] != "validation_review" {
		t.Fatalf("lifecycle history was rewritten: %#v", warning.StageHistory)
	}

	expected := map[string]string{
		"/raw/pass.mkv":          "ready_to_publish",
		"/raw/legacy.mkv":        "ready_to_publish",
		"/raw/invalid-stage.mkv": "ready_to_publish",
		"/raw/invalid-time.mkv":  "ready_to_publish",
		"/raw/out-of-order.mkv":  "ready_to_publish",
		"/raw/newer-stage.mkv":   "archiving_original",
		"/raw/published.mkv":     "ready_to_publish",
		"/raw/terminal.mkv":      "failed",
	}
	firstMigrationUpdatedAt := make(map[string]time.Time, len(expected)+1)
	for mediaPath, stage := range expected {
		var job models.QueueJob
		if err := db.First(&job, "media_path = ?", mediaPath).Error; err != nil {
			t.Fatal(err)
		}
		if job.Stage != stage {
			t.Errorf("%s: expected stage %q, got %q", mediaPath, stage, job.Stage)
		}
		if mediaPath == "/raw/pass.mkv" && (job.StageUpdatedAt == nil || !job.StageUpdatedAt.Equal(readyAt)) {
			t.Errorf("clean PASS timestamp changed: %v", job.StageUpdatedAt)
		}
		if mediaPath == "/raw/invalid-stage.mkv" || mediaPath == "/raw/invalid-time.mkv" || mediaPath == "/raw/out-of-order.mkv" || mediaPath == "/raw/newer-stage.mkv" || mediaPath == "/raw/published.mkv" || mediaPath == "/raw/terminal.mkv" {
			if job.StageUpdatedAt == nil || !job.StageUpdatedAt.Equal(backfillAt) {
				t.Errorf("protected or ambiguous timestamp changed for %s: %v", mediaPath, job.StageUpdatedAt)
			}
		}
		if mediaPath == "/raw/legacy.mkv" && len(job.StageHistory) != 0 {
			t.Errorf("legacy history was fabricated: %#v", job.StageHistory)
		}
		firstMigrationUpdatedAt[mediaPath] = job.UpdatedAt
	}
	firstMigrationUpdatedAt[warning.MediaPath] = warning.UpdatedAt

	if err := Migrate(db); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}
	for mediaPath, updatedAt := range firstMigrationUpdatedAt {
		var job models.QueueJob
		if err := db.First(&job, "media_path = ?", mediaPath).Error; err != nil {
			t.Fatal(err)
		}
		if !job.UpdatedAt.Equal(updatedAt) {
			t.Errorf("repeat migration rewrote %s: updated_at %s -> %s", mediaPath, updatedAt, job.UpdatedAt)
		}
	}
}

func TestQueueLifecycleStageReconciliationDoesNotOverwriteConcurrentTransition(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "queue-lifecycle-race.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(db); err != nil {
		t.Fatalf("initial migration: %v", err)
	}
	reviewAt := time.Date(2026, 10, 10, 12, 1, 0, 0, time.UTC)
	readAt := reviewAt.Add(time.Minute)
	backfillAt := readAt.Add(time.Minute)
	job := models.QueueJob{
		MediaPath: "/raw/racing.mkv", LibraryID: 1, ProfileID: 1, Status: "completed", Stage: "ready_to_publish",
		StageUpdatedAt: &backfillAt, StageHistory: lifecycleHistory("validation_review", reviewAt),
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	var stale models.QueueJob
	if err := db.First(&stale, job.ID).Error; err != nil {
		t.Fatal(err)
	}

	transitionAt := readAt.Add(time.Minute)
	transitionHistory := append(models.JSONList{}, stale.StageHistory...)
	transitionHistory = append(transitionHistory, models.JSONMap{"stage": "publishing", "at": transitionAt.Format(time.RFC3339)})
	if err := db.Model(&models.QueueJob{}).Where("id = ?", job.ID).Updates(map[string]any{
		"stage": "publishing", "stage_updated_at": &transitionAt, "stage_history": transitionHistory,
	}).Error; err != nil {
		t.Fatalf("commit concurrent transition: %v", err)
	}

	if err := updateQueueLifecycleStageIfUnchanged(db, &stale, "validation_review", &reviewAt); err != nil {
		t.Fatalf("attempt stale reconciliation: %v", err)
	}
	var current models.QueueJob
	if err := db.First(&current, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.Stage != "publishing" || len(current.StageHistory) != 2 || current.StageHistory[1].(map[string]any)["stage"] != "publishing" {
		t.Fatalf("concurrent transition was overwritten: %#v", current)
	}
}

func lifecycleHistory(events ...any) models.JSONList {
	history := make(models.JSONList, 0, len(events)/2)
	for i := 0; i < len(events); i += 2 {
		stage := events[i].(string)
		at := events[i+1].(time.Time)
		history = append(history, models.JSONMap{"stage": stage, "at": at.Format(time.RFC3339)})
	}
	return history
}
