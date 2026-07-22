package scheduler

import (
	"errors"
	"sync"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAssetReservationPreventsDuplicateOpenJobs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:reservation-lock?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.SchedulerReservation{}); err != nil {
		t.Fatal(err)
	}
	if err := LockQueuedAsset(db, models.QueueJob{ID: 1, MediaPath: "/raw/show.mkv"}); err != nil {
		t.Fatal(err)
	}
	if err := LockQueuedAsset(db, models.QueueJob{ID: 2, MediaPath: "/raw/show.mkv"}); !errors.Is(err, ErrAssetAlreadyReserved) {
		t.Fatalf("expected duplicate reservation error, got %v", err)
	}
}

func TestConcurrentAssetReservationsAllowOnlyOneLock(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:reservation-concurrent?mode=memory&cache=shared&_busy_timeout=5000"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.SchedulerReservation{}); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for id := uint(1); id <= 2; id++ {
		wg.Add(1)
		go func(jobID uint) {
			defer wg.Done()
			<-start
			results <- LockQueuedAsset(db, models.QueueJob{ID: jobID, MediaPath: "/raw/same.mkv"})
		}(id)
	}
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for result := range results {
		if result == nil {
			success++
		} else if !errors.Is(result, ErrAssetAlreadyReserved) {
			t.Fatalf("unexpected lock error: %v", result)
		}
	}
	var count int64
	if err := db.Model(&models.SchedulerReservation{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if success != 1 || count != 1 {
		t.Fatalf("expected one lock, successes=%d rows=%d", success, count)
	}
}

func TestReservationActivatesAndReleasesResources(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:reservation-resources?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.SchedulerReservation{}); err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{ID: 8, MediaPath: "/raw/movie.mkv"}
	if err := LockQueuedAsset(db, job); err != nil {
		t.Fatal(err)
	}
	plan := models.ExecutionPlan{SelectedEncoder: "hevc_qsv", Reservation: models.JSONMap{"encoderClass": "hardware", "memoryBytes": int64(2 << 30), "workspaceBytes": int64(20 << 30)}}
	if err := ActivateReservation(db, job, plan, "worker-a"); err != nil {
		t.Fatal(err)
	}
	var reservation models.SchedulerReservation
	if err := db.Where("job_id = ?", job.ID).First(&reservation).Error; err != nil {
		t.Fatal(err)
	}
	if reservation.State != ReservationStateActive || reservation.Encoder != "hevc_qsv" || reservation.WorkerName != "worker-a" {
		t.Fatalf("unexpected active reservation: %#v", reservation)
	}
	if err := DeactivateReservationResources(db, job.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.Where("job_id = ?", job.ID).First(&reservation).Error; err != nil {
		t.Fatal(err)
	}
	if reservation.State != ReservationStateLocked || reservation.MemoryBytes != 0 {
		t.Fatalf("resources were not released: %#v", reservation)
	}
}
