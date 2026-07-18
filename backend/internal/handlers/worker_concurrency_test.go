package handlers

import (
	"sync"
	"testing"
	"time"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"github.com/anuelvs/mediaforge/backend/internal/scheduler"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestConcurrentClaimsRespectSingleWorkerSlot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:claim-concurrent?mode=memory&cache=shared&_busy_timeout=5000"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(4)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.ExecutionPlan{}, &models.RuntimeSnapshot{}, &models.AppSetting{}, &models.SchedulerReservation{}, &models.WorkerNode{}); err != nil {
		t.Fatal(err)
	}
	disks := models.JSONMap{"workspace": models.JSONMap{"availableBytes": float64(500 << 30)}, "library": models.JSONMap{"availableBytes": float64(500 << 30)}}
	encoders := models.JSONMap{"libx265": models.JSONMap{"usable": true}}
	if err := db.Create(&models.RuntimeSnapshot{DetectedAt: time.Now(), SelectedProfile: "desktop_balanced", AvailableMemoryBytes: 16 << 30, Disks: disks, Encoders: encoders}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "workers", Value: models.JSONMap{"defaultWorkerName": "local", "maxConcurrentJobs": 1, "maxJobsPerBatch": 0, "delaySecondsBetweenJobs": 0, "batchCooldownSeconds": 0}}).Error; err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2; index++ {
		job := models.QueueJob{MediaPath: "/raw/job-" + string(rune('a'+index)) + ".mkv", LibraryID: 1, ProfileID: 1, Status: JobStatusQueued, Priority: 1}
		if err := db.Create(&job).Error; err != nil {
			t.Fatal(err)
		}
		plan := models.ExecutionPlan{JobID: job.ID, Version: 1, Status: scheduler.ExecutionPlanReady, SelectedEncoder: "libx265", WorkspaceMode: scheduler.WorkspaceModeDirect, ApprovalStatus: scheduler.ApprovalAutoApproved, Reservation: models.JSONMap{}}
		if err := db.Create(&plan).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Model(&job).Update("active_execution_plan_id", plan.ID).Error; err != nil {
			t.Fatal(err)
		}
		if err := scheduler.LockQueuedAsset(db, job); err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for index := 0; index < 2; index++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; _, _ = NewWorkerHandler(db).claimNextJob("local") }()
	}
	close(start)
	wg.Wait()
	var running int64
	if err := db.Model(&models.QueueJob{}).Where("status = ?", JobStatusRunning).Count(&running).Error; err != nil {
		t.Fatal(err)
	}
	var active int64
	if err := db.Model(&models.SchedulerReservation{}).Where("state = ?", scheduler.ReservationStateActive).Count(&active).Error; err != nil {
		t.Fatal(err)
	}
	if running != 1 || active != 1 {
		t.Fatalf("single slot violated: running=%d active=%d", running, active)
	}
}
