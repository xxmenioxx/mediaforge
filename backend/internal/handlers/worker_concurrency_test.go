package handlers

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestConcurrentClaimsRespectSingleWorkerSlot(t *testing.T) {
	dsn := fmt.Sprintf("file:claim-concurrent-%d?mode=memory&cache=shared&_busy_timeout=5000", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
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
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for index := 0; index < 2; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, claimErr := NewWorkerHandler(db).claimNextJob("local")
			results <- claimErr
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	for claimErr := range results {
		if claimErr == nil {
			successes++
			continue
		}
		if !errors.Is(claimErr, errWorkerLimitReached) && !errors.Is(claimErr, gorm.ErrRecordNotFound) {
			t.Fatalf("unexpected concurrent claim error: %v", claimErr)
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one successful claim, got %d", successes)
	}
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

func TestClaimedJobExecutionFailureReleasesReservation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:claimed-execution-failure?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.SchedulerReservation{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	job := models.QueueJob{
		MediaPath: "/raw/missing.mkv", LibraryID: 1, ProfileID: 1,
		Status: JobStatusRunning, Stage: JobStageClaimed, Progress: 1, StartedAt: &now,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	reservation := models.SchedulerReservation{
		JobID: job.ID, AssetKey: job.MediaPath, State: scheduler.ReservationStateActive,
	}
	if err := db.Create(&reservation).Error; err != nil {
		t.Fatal(err)
	}

	NewWorkerHandler(db).failClaimedJobExecution(job.ID, errors.New("input media is not readable"))

	if err := db.First(&job, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if job.Status != JobStatusFailed || job.Stage != JobStageFailed || job.Progress != 0 || job.FinishedAt == nil {
		t.Fatalf("claimed job was not closed after execution failure: %#v", job)
	}
	if err := db.First(&reservation, reservation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if reservation.State == scheduler.ReservationStateActive {
		t.Fatalf("reservation remained active: %#v", reservation)
	}
}
