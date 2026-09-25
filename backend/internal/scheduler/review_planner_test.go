package scheduler

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestConditionalReviewEstimatesAndAutoApproves(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:review-plan?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Library{}, &models.QueueJob{}, &models.ExecutionPlan{}, &models.AppSetting{}, &models.RuntimeSnapshot{}, &models.RuntimeState{}, &models.SchedulerReservation{}, &models.WorkerNode{}); err != nil {
		t.Fatal(err)
	}
	seedReviewRuntime(t, db)
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

func TestRepeatedReviewEvaluationReusesPlanRuntimeSnapshot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:review-plan-idempotent?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Library{}, &models.QueueJob{}, &models.ExecutionPlan{}, &models.AppSetting{}, &models.RuntimeSnapshot{}, &models.RuntimeState{}, &models.SchedulerReservation{}, &models.WorkerNode{}); err != nil {
		t.Fatal(err)
	}
	seedReviewRuntime(t, db)
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
	if plan.RuntimeSnapshotID == nil {
		t.Fatal("first material evaluation did not bind runtime provenance")
	}
	boundID := *plan.RuntimeSnapshotID
	var countBefore int64
	if err := db.Model(&models.RuntimeSnapshot{}).Count(&countBefore).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.RuntimeSnapshot{DetectedAt: time.Now().Add(time.Minute), SelectedProfile: "laptop_safe", AvailableMemoryBytes: 16 << 30}).Error; err != nil {
		t.Fatal(err)
	}
	if err := EvaluateReviewPlan(db, &plan); err != nil {
		t.Fatal(err)
	}
	if plan.RuntimeSnapshotID == nil || *plan.RuntimeSnapshotID != boundID {
		t.Fatalf("repeated evaluation replaced plan provenance: got=%v want=%d", plan.RuntimeSnapshotID, boundID)
	}
	var countAfter int64
	if err := db.Model(&models.RuntimeSnapshot{}).Count(&countAfter).Error; err != nil {
		t.Fatal(err)
	}
	if countAfter != countBefore+1 {
		t.Fatalf("repeated evaluation unexpectedly captured runtime evidence: before=%d after=%d", countBefore, countAfter)
	}
}

func TestReviewPlanRuntimeFreezeFailsClosedAndRetriesIdempotently(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:review-plan-freeze-failure?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Library{}, &models.QueueJob{}, &models.ExecutionPlan{}, &models.AppSetting{}, &models.RuntimeSnapshot{}, &models.RuntimeState{}, &models.SchedulerReservation{}, &models.WorkerNode{}); err != nil {
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
	initialStatus := plan.Status
	if err := EvaluateReviewPlan(db, &plan); err == nil {
		t.Fatal("expected runtime freeze failure when RuntimeState is unavailable")
	}
	if plan.RuntimeSnapshotID != nil {
		t.Fatalf("failed runtime freeze bound a snapshot: %v", *plan.RuntimeSnapshotID)
	}
	if plan.Status != initialStatus || plan.RuntimeProfile == "desktop_safe" {
		t.Fatalf("failed runtime freeze advanced plan with fallback state: status=%q profile=%q", plan.Status, plan.RuntimeProfile)
	}
	var persisted models.ExecutionPlan
	if err := db.First(&persisted, plan.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.RuntimeSnapshotID != nil || persisted.Status != initialStatus {
		t.Fatalf("failed runtime freeze changed persisted plan: %#v", persisted)
	}

	seedReviewRuntime(t, db)
	if err := EvaluateReviewPlan(db, &plan); err != nil {
		t.Fatalf("retry after runtime state became available failed: %v", err)
	}
	if plan.RuntimeSnapshotID == nil {
		t.Fatal("successful retry did not bind runtime provenance")
	}
	boundID := *plan.RuntimeSnapshotID
	var countAfterRetry int64
	if err := db.Model(&models.RuntimeSnapshot{}).Count(&countAfterRetry).Error; err != nil {
		t.Fatal(err)
	}
	if err := EvaluateReviewPlan(db, &plan); err != nil {
		t.Fatalf("repeated retry evaluation failed: %v", err)
	}
	if plan.RuntimeSnapshotID == nil || *plan.RuntimeSnapshotID != boundID {
		t.Fatalf("repeated evaluation changed runtime provenance: got=%v want=%d", plan.RuntimeSnapshotID, boundID)
	}
	var countAfterRepeat int64
	if err := db.Model(&models.RuntimeSnapshot{}).Count(&countAfterRepeat).Error; err != nil {
		t.Fatal(err)
	}
	if countAfterRepeat != countAfterRetry {
		t.Fatalf("repeated evaluation captured another runtime snapshot: before=%d after=%d", countAfterRetry, countAfterRepeat)
	}
}

func TestManualReviewCanRejectAndApproveActivePlan(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:manual-review?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.ExecutionPlan{}, &models.AppSetting{}, &models.RuntimeSnapshot{}, &models.RuntimeState{}, &models.SchedulerReservation{}, &models.WorkerNode{}); err != nil {
		t.Fatal(err)
	}
	seedReviewRuntime(t, db)
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

func TestManualApprovalWaitsForScheduleWindow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:schedule-wait?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.ExecutionPlan{}, &models.AppSetting{}, &models.RuntimeSnapshot{}, &models.RuntimeState{}, &models.SchedulerReservation{}, &models.WorkerNode{}); err != nil {
		t.Fatal(err)
	}
	seedReviewRuntime(t, db)
	setting := models.AppSetting{Key: "workingHours", Value: models.JSONMap{
		"enabled": true, "timezone": "America/Mexico_City", "windows": []models.JSONMap{},
		"outsideWindowPolicy": models.JSONMap{"startNewHeavyJobs": false},
	}}
	if err := db.Create(&setting).Error; err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: "/media/raw/movie.mkv", LibraryID: 1, ProfileID: 1, Priority: 1, Status: "queued"}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	plan := models.ExecutionPlan{JobID: job.ID, Version: 1, Status: ExecutionPlanWaiting, ApprovalStatus: ApprovalPending, Evaluation: models.JSONMap{}}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	job.ActiveExecutionPlanID = &plan.ID
	if err := db.Save(&job).Error; err != nil {
		t.Fatal(err)
	}
	approved, err := SetPlanApproval(db, job.ID, plan.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != ExecutionPlanWaiting || approved.WaitingState != "WAITING_SCHEDULE_WINDOW" {
		t.Fatalf("expected schedule wait, got %#v", approved)
	}
}

func seedReviewRuntime(t *testing.T, db *gorm.DB) {
	t.Helper()
	disks := models.JSONMap{"workspace": models.JSONMap{"availableBytes": float64(500 << 30)}, "library": models.JSONMap{"availableBytes": float64(500 << 30)}}
	encoders := models.JSONMap{
		"libx265":           models.JSONMap{"listed": true, "usable": true, "reason": ""},
		"hevc_videotoolbox": models.JSONMap{"listed": true, "usable": true, "reason": ""},
		"hevc_qsv":          models.JSONMap{"listed": true, "usable": true, "reason": ""},
	}
	if err := db.Create(&models.RuntimeSnapshot{DetectedAt: time.Now(), SelectedProfile: "desktop_balanced", AvailableMemoryBytes: 16 << 30, Disks: disks, Encoders: encoders}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.RuntimeState{
		SingletonKey: models.RuntimeStateSingletonKey, DetectedAt: time.Now(), SelectedProfile: "desktop_balanced",
		AvailableMemoryBytes: 16 << 30, Disks: disks, Encoders: encoders,
		StableFingerprint: "review-runtime", FingerprintVersion: "v1",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.WorkerNode{Name: "test-worker", Status: "online", MaxConcurrentJobs: 2, Encoders: models.JSONList{"libx265", "hevc_videotoolbox", "hevc_qsv"}, LastSeenAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
}
