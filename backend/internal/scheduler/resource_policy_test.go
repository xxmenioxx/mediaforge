package scheduler

import (
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestBuildReservationClassifiesEncoders(t *testing.T) {
	hardware := BuildReservation(models.ExecutionPlan{SelectedEncoder: "hevc_videotoolbox", EstimatedWorkspaceBytes: 10})
	software := BuildReservation(models.ExecutionPlan{SelectedEncoder: "libx265", EstimatedWorkspaceBytes: 20})
	if hardware["encoderClass"] != "hardware" || software["encoderClass"] != "software" {
		t.Fatalf("unexpected reservations: %#v %#v", hardware, software)
	}
}

func TestWorkerAvailabilityIgnoresLegacyTaskReservations(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:legacy-task-reservation?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.SchedulerReservation{}, &models.TaskReservation{}, &models.WorkerNode{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	worker := models.WorkerNode{Name: "local", Status: "online", MaxConcurrentJobs: 1, Encoders: models.JSONList{"libx265"}, LastSeenAt: now}
	if err := db.Create(&worker).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.TaskReservation{OwnerType: "test_encode", OwnerID: 7, AssetKey: "/raw/a.mkv", State: ReservationStateActive, WorkerName: worker.Name}).Error; err != nil {
		t.Fatal(err)
	}
	decision, err := EvaluateWorkerAvailability(db, models.ExecutionPlan{SelectedEncoder: "libx265"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Available {
		t.Fatalf("legacy Test Encode reservation affected Queue worker capacity: %#v", decision)
	}
}

func TestCanDispatchBlocksAtMachineRunningLimit(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:resource-limit?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.ExecutionPlan{}, &models.RuntimeSnapshot{}, &models.AppSetting{}, &models.SchedulerReservation{}, &models.WorkerNode{}); err != nil {
		t.Fatal(err)
	}
	disks := models.JSONMap{"workspace": models.JSONMap{"availableBytes": float64(500 << 30)}, "library": models.JSONMap{"availableBytes": float64(500 << 30)}}
	if err := db.Create(&models.RuntimeSnapshot{DetectedAt: time.Now(), SelectedProfile: "desktop_safe", AvailableMemoryBytes: 16 << 30, Disks: disks}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.WorkerNode{Name: "test-worker", Status: "online", MaxConcurrentJobs: 2, Encoders: models.JSONList{"libx265"}, LastSeenAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	runningPlan := models.ExecutionPlan{JobID: 1, Version: 1, Status: ExecutionPlanDispatched, SelectedEncoder: "libx265"}
	if err := db.Create(&runningPlan).Error; err != nil {
		t.Fatal(err)
	}
	runningJob := models.QueueJob{MediaPath: "/raw/a.mkv", LibraryID: 1, ProfileID: 1, Status: "running", ActiveExecutionPlanID: &runningPlan.ID}
	if err := db.Create(&runningJob).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.SchedulerReservation{JobID: runningJob.ID, AssetKey: runningJob.MediaPath, State: ReservationStateActive, Encoder: "libx265", EncoderClass: "software"}).Error; err != nil {
		t.Fatal(err)
	}
	candidate := models.ExecutionPlan{SelectedEncoder: "libx265", EstimatedWorkspaceBytes: 10 << 30, EstimatedOutputMaxBytes: 5 << 30, Reservation: models.JSONMap{}}
	allowed, reasons, err := CanDispatch(db, &candidate)
	if err != nil {
		t.Fatal(err)
	}
	if allowed || len(reasons) == 0 {
		t.Fatalf("expected resource wait, allowed=%v reasons=%v", allowed, reasons)
	}
	if candidate.SelectedEncoder != "libx265" {
		t.Fatalf("resource saturation must not replace a locked encoder: %#v", candidate)
	}
}

func TestMachineProfilesHaveSafeLimits(t *testing.T) {
	for _, name := range []string{"nas_safe", "nas_balanced", "desktop_safe", "desktop_balanced", "laptop_safe", "workstation_balanced"} {
		limits := LimitsForProfile(name)
		if limits.MaxRunningJobs < 1 || limits.MinFreeRAMBytes <= 0 || limits.MaxWorkspaceBytes <= 0 {
			t.Fatalf("invalid limits for %s: %#v", name, limits)
		}
	}
}

func TestLoadSchedulerLimitsAppliesCustomOverrides(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:custom-limits?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	setting := models.AppSetting{Key: "runtimePolicy", Value: models.JSONMap{"schemaVersion": 2, "mode": "automatic", "preferredProfile": "desktop_balanced", "fallbackProfile": "desktop_safe", "overrides": models.JSONMap{"desktop_balanced": models.JSONMap{"maxRunningJobs": 4, "minFreeRamGb": 12, "allowDirectMode": false}}}}
	if err := db.Create(&setting).Error; err != nil {
		t.Fatal(err)
	}
	limits, err := LoadSchedulerLimits(db, "desktop_balanced")
	if err != nil {
		t.Fatal(err)
	}
	if limits.MaxRunningJobs != 4 || limits.MinFreeRAMBytes != 12<<30 || limits.AllowDirectMode {
		t.Fatalf("unexpected custom limits: %#v", limits)
	}
}

func TestResourceWaitingStateUsesSpecificCause(t *testing.T) {
	if state := resourceWaitingState([]string{"Free RAM is below policy minimum (1 bytes required)"}); state != "WAITING_RAM" {
		t.Fatalf("unexpected RAM state: %s", state)
	}
	if state := resourceWaitingState([]string{"Library disk would fall below its free-space reserve"}); state != "WAITING_HDD_SPACE" {
		t.Fatalf("unexpected library state: %s", state)
	}
}

func TestEvaluateResourcesWaitsForACPower(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:battery-policy?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.ExecutionPlan{}, &models.RuntimeSnapshot{}, &models.AppSetting{}, &models.SchedulerReservation{}, &models.WorkerNode{}); err != nil {
		t.Fatal(err)
	}
	disks := models.JSONMap{"workspace": models.JSONMap{"availableBytes": float64(500 << 30)}, "library": models.JSONMap{"availableBytes": float64(500 << 30)}}
	if err := db.Create(&models.RuntimeSnapshot{DetectedAt: time.Now(), SelectedProfile: "laptop_safe", AvailableMemoryBytes: 16 << 30, OnBattery: true, BatteryPresent: true, Disks: disks}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "runtimePolicy", Value: models.JSONMap{"pauseWhenOnBattery": true}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.WorkerNode{Name: "local", Status: "online", MaxConcurrentJobs: 1, Encoders: models.JSONList{"libx265"}, LastSeenAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	decision, err := EvaluateResources(db, &models.ExecutionPlan{SelectedEncoder: "libx265", EstimatedWorkspaceBytes: 1 << 30, EstimatedOutputMaxBytes: 1 << 30})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.WaitingState != "WAITING_POWER" {
		t.Fatalf("unexpected power decision: %#v", decision)
	}
}
