package scheduler

import (
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestWorkerAvailabilityRequiresEncoderAndCapacity(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:worker-availability?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.WorkerNode{}, &models.SchedulerReservation{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	worker := models.WorkerNode{Name: "worker-a", Status: "online", MaxConcurrentJobs: 1, Encoders: models.JSONList{"hevc_qsv", "libx265"}, LastSeenAt: now}
	if err := db.Create(&worker).Error; err != nil {
		t.Fatal(err)
	}
	decision, err := EvaluateWorkerAvailability(db, models.ExecutionPlan{SelectedEncoder: "hevc_qsv"}, now)
	if err != nil || !decision.Available {
		t.Fatalf("expected available worker: %#v err=%v", decision, err)
	}
	if err := db.Create(&models.SchedulerReservation{JobID: 2, AssetKey: "/raw/a", State: ReservationStateActive, WorkerName: worker.Name}).Error; err != nil {
		t.Fatal(err)
	}
	decision, err = EvaluateWorkerAvailability(db, models.ExecutionPlan{SelectedEncoder: "hevc_qsv"}, now)
	if err != nil || decision.Available {
		t.Fatalf("expected capacity wait: %#v err=%v", decision, err)
	}
}

func TestWorkerAvailabilityHonorsTargetWorker(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:target-worker-availability?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.WorkerNode{}, &models.SchedulerReservation{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	workers := []models.WorkerNode{
		{Name: "intel", Status: "online", MaxConcurrentJobs: 1, Encoders: models.JSONList{"hevc_qsv", "libx265"}, LastSeenAt: now},
		{Name: "mac", Status: "online", MaxConcurrentJobs: 1, Encoders: models.JSONList{"hevc_videotoolbox", "libx265"}, LastSeenAt: now},
	}
	if err := db.Create(&workers).Error; err != nil {
		t.Fatal(err)
	}
	plan := models.ExecutionPlan{SelectedEncoder: "hevc_videotoolbox", Constraints: models.JSONMap{"targetWorkerName": "intel"}}
	decision, err := EvaluateWorkerAvailability(db, plan, now)
	if err != nil || decision.Available {
		t.Fatalf("intel worker must not accept VideoToolbox: %#v err=%v", decision, err)
	}
	plan.Constraints["targetWorkerName"] = "mac"
	decision, err = EvaluateWorkerAvailability(db, plan, now)
	if err != nil || !decision.Available || decision.Worker != "mac" {
		t.Fatalf("expected targeted mac worker: %#v err=%v", decision, err)
	}
}
