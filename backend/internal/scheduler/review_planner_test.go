package scheduler

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestConditionalReviewEstimatesAndAutoApproves(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:review-plan?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Library{}, &models.QueueJob{}, &models.ExecutionPlan{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	inputPath := filepath.Join(t.TempDir(), "movie.mkv")
	if err := os.WriteFile(inputPath, make([]byte, 10_000), 0o600); err != nil {
		t.Fatal(err)
	}
	library := models.Library{Name: "Movies", SourcePath: filepath.Dir(inputPath), DestinationPath: "/media/library"}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	profile := executionPlanTestProfile(1, 25)
	snapshot, err := CaptureProfileSnapshot(profile, time.Now(), "queue_create")
	if err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: inputPath, LibraryID: library.ID, ProfileID: profile.ID, ProfileVersion: 1, ProfileSnapshot: snapshot, Priority: 1, Status: "queued"}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	plan, err := CreatePendingExecutionPlan(db, &job, "review test")
	if err != nil {
		t.Fatal(err)
	}
	if err := EvaluateReviewPlan(db, &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Status != ExecutionPlanReady || plan.ApprovalStatus != ApprovalAutoApproved {
		t.Fatalf("expected conditional auto approval, got %#v", plan)
	}
	if plan.InputSizeBytes != 10_000 || plan.EstimatedOutputMinBytes == 0 || plan.EstimatedOutputMaxBytes >= plan.InputSizeBytes {
		t.Fatalf("unexpected size estimate: %#v", plan)
	}
	if plan.OutputPath != "/media/library/movie.mkv" || plan.SelectedEncoder != "libx265" {
		t.Fatalf("unexpected planned output: %#v", plan)
	}
}

func TestManualReviewCanRejectAndApproveActivePlan(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:manual-review?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.ExecutionPlan{}); err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: "/media/raw/movie.mkv", LibraryID: 1, ProfileID: 1, Priority: 1, Status: "queued"}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	plan := models.ExecutionPlan{JobID: job.ID, Version: 1, Status: ExecutionPlanWaiting, ApprovalStatus: ApprovalPending}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	job.ActiveExecutionPlanID = &plan.ID
	if err := db.Save(&job).Error; err != nil {
		t.Fatal(err)
	}
	rejected, err := SetPlanApproval(db, job.ID, plan.ID, false)
	if err != nil || rejected.ApprovalStatus != ApprovalRejected {
		t.Fatalf("reject failed: plan=%#v err=%v", rejected, err)
	}
	approved, err := SetPlanApproval(db, job.ID, plan.ID, true)
	if err != nil || approved.Status != ExecutionPlanReady || approved.ApprovalStatus != ApprovalManual {
		t.Fatalf("approve failed: plan=%#v err=%v", approved, err)
	}
}
