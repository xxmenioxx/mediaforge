package runtimeinfo

import (
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openRuntimeStateTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:runtime-state-test-" + strings.NewReplacer("/", "-", " ", "-").Replace(t.Name()) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.RuntimeSnapshot{}, &models.RuntimeState{}, &models.ExecutionPlan{}, &models.TestEncode{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func testRuntimeState() models.RuntimeState {
	return models.RuntimeState{
		SingletonKey: models.RuntimeStateSingletonKey,
		DetectedAt:   time.Unix(100, 0).UTC(),
		OS:           "linux", Architecture: "amd64", Container: true, CPUCores: 8,
		TotalMemoryBytes: 16 << 30, GPUDetected: true,
		Disks: models.JSONMap{
			"workspace": models.JSONMap{"path": "/media/work", "type": "ssd", "availableBytes": int64(10 << 30)},
		},
		Encoders: models.JSONMap{
			"hevc_qsv": models.JSONMap{
				"listed": true, "usable": true, "main10": true, "adaptiveB": true,
				"testedModes": models.JSONMap{"qsvIcqMain10": true},
				"reason":      "probe succeeded",
			},
		},
		RecommendedProfile: "desktop_balanced", SelectedProfile: "desktop_balanced",
		PreferredProfile: "auto", FallbackProfile: "desktop_safe",
		AppliedOverrides: models.JSONList{"maxVideoJobs", "minFreeRamGb"},
		EffectivePolicy: models.JSONMap{
			"mode": "automatic", "baseProfile": "desktop_balanced",
			"values":    models.JSONMap{"maxVideoJobs": 2},
			"overrides": models.JSONMap{"maxVideoJobs": 2},
		},
	}
}

func TestRuntimeStableFingerprintIsDeterministicAndExcludesVolatileInputs(t *testing.T) {
	first := testRuntimeState()
	second := testRuntimeState()
	second.Encoders = models.JSONMap{
		"hevc_qsv": models.JSONMap{
			"reason": "device temporarily busy", "adaptiveB": true, "main10": true,
			"testedModes": models.JSONMap{"qsvIcqMain10": true}, "usable": true, "listed": true,
		},
	}
	first.CPULoad1, first.AvailableMemoryBytes = 0.1, 12<<30
	first.GPUUsageAvailable, first.GPUUsagePercent, first.GPUMetricSource = true, 75, "drm_fdinfo"
	first.BatteryPercent, first.PowerSource, first.OnBattery = 90, "ac", false
	first.Disks["workspace"].(models.JSONMap)["availableBytes"] = int64(3 << 30)
	second.CPULoad1, second.AvailableMemoryBytes = 0.9, 2<<30
	second.GPUUsageAvailable, second.GPUUsagePercent, second.GPUMetricSource = false, 0, ""
	second.BatteryPercent, second.PowerSource, second.OnBattery = 20, "battery", true
	second.Disks["workspace"].(models.JSONMap)["availableBytes"] = int64(1 << 30)
	second.ID = 999
	second.DetectedAt = second.DetectedAt.Add(24 * time.Hour)
	second.CreatedAt = second.DetectedAt.Add(time.Hour)

	if got, want := RuntimeStableFingerprint(first), RuntimeStableFingerprint(second); got != want {
		t.Fatalf("volatile or map-order changes altered fingerprint: %s != %s", got, want)
	}

	second.Encoders["hevc_qsv"].(models.JSONMap)["usable"] = false
	if RuntimeStableFingerprint(first) == RuntimeStableFingerprint(second) {
		t.Fatal("execution-relevant encoder usability change did not alter fingerprint")
	}

	gpuChanged := first
	gpuChanged.GPUDetected = false
	if RuntimeStableFingerprint(first) == RuntimeStableFingerprint(gpuChanged) {
		t.Fatal("stable GPU capability change did not alter fingerprint")
	}

	diskChanged := first
	diskChanged.Disks = models.JSONMap{"workspace": models.JSONMap{"path": "/media/other", "type": "hdd", "availableBytes": int64(3 << 30)}}
	if RuntimeStableFingerprint(first) == RuntimeStableFingerprint(diskChanged) {
		t.Fatal("disk identity/type change did not alter fingerprint")
	}
}

func TestUpsertDetectedStateCreatesOneTransitionPerStableChange(t *testing.T) {
	db := openRuntimeStateTestDB(t)
	first := testRuntimeState()
	state, transition, err := UpsertDetectedState(db, first)
	if err != nil || transition != nil {
		t.Fatalf("first state write: state=%#v transition=%v err=%v", state, transition, err)
	}
	if state.ID == 0 {
		t.Fatal("first state write did not create a singleton row")
	}

	volatile := first
	volatile.CPULoad1, volatile.AvailableMemoryBytes = 2.5, 1
	updated, transition, err := UpsertDetectedState(db, volatile)
	if err != nil || transition != nil || updated.ID != state.ID {
		t.Fatalf("volatile update: state=%#v transition=%v err=%v", updated, transition, err)
	}

	changed := volatile
	changed.Encoders["hevc_qsv"].(models.JSONMap)["usable"] = false
	updated, transition, err = UpsertDetectedState(db, changed)
	if err != nil || transition == nil || updated.ID != state.ID {
		t.Fatalf("stable transition: state=%#v transition=%v err=%v", updated, transition, err)
	}
	var count int64
	if err := db.Model(&models.RuntimeSnapshot{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("transition snapshot count=%d want 1", count)
	}

	changed.DetectedAt = changed.DetectedAt.Add(time.Minute)
	if _, transition, err = UpsertDetectedState(db, changed); err != nil || transition != nil {
		t.Fatalf("equivalent transition duplicated: transition=%v err=%v", transition, err)
	}

	returned := first
	returned.DetectedAt = changed.DetectedAt.Add(time.Minute)
	returned.Encoders = models.JSONMap{"hevc_qsv": models.JSONMap{
		"listed": true, "usable": true, "main10": true, "adaptiveB": true,
		"testedModes": models.JSONMap{"qsvIcqMain10": true},
	}}
	if _, transition, err = UpsertDetectedState(db, returned); err != nil || transition == nil {
		t.Fatalf("A -> B -> A did not capture return transition: transition=%v err=%v", transition, err)
	}
	if err := db.Model(&models.RuntimeSnapshot{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("transition snapshot count=%d want 2", count)
	}
}

func TestInitializeLegacyStateUsesNewestSnapshotWithoutDeletingHistory(t *testing.T) {
	db := openRuntimeStateTestDB(t)
	older := models.RuntimeSnapshot{ID: 0, DetectedAt: time.Unix(100, 0), OS: "linux", Architecture: "amd64", CPUCores: 2}
	newer := models.RuntimeSnapshot{DetectedAt: time.Unix(200, 0), OS: "linux", Architecture: "arm64", CPUCores: 8, SelectedProfile: "nas_balanced"}
	if err := db.Create(&older).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&newer).Error; err != nil {
		t.Fatal(err)
	}
	if err := InitializeLegacyState(db); err != nil {
		t.Fatal(err)
	}
	state, err := Current(db)
	if err != nil {
		t.Fatal(err)
	}
	if state.Architecture != "arm64" || state.CPUCores != 8 || state.StableFingerprint == "" {
		t.Fatalf("legacy state initialized from wrong snapshot: %#v", state)
	}
	var count int64
	if err := db.Model(&models.RuntimeSnapshot{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("legacy history count=%d want 2", count)
	}
	if err := InitializeLegacyState(db); err != nil {
		t.Fatal(err)
	}
	var stateCount int64
	if err := db.Model(&models.RuntimeState{}).Count(&stateCount).Error; err != nil {
		t.Fatal(err)
	}
	if stateCount != 1 {
		t.Fatalf("runtime state count=%d want 1", stateCount)
	}
}

func TestRuntimeStateSingletonIdentityIsUnique(t *testing.T) {
	db := openRuntimeStateTestDB(t)
	first := testRuntimeState()
	first.StableFingerprint = RuntimeStableFingerprint(first)
	first.FingerprintVersion = RuntimeStableFingerprintVersion
	if err := db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	second := testRuntimeState()
	second.DetectedAt = second.DetectedAt.Add(time.Second)
	if err := db.Create(&second).Error; err == nil {
		t.Fatal("second singleton state unexpectedly succeeded")
	}
}

func TestStableAuthorityChangesCaptureTransitions(t *testing.T) {
	tests := []struct {
		name   string
		change func(*models.RuntimeState)
	}{
		{name: "gpu capability", change: func(state *models.RuntimeState) { state.GPUDetected = !state.GPUDetected }},
		{name: "runtime policy", change: func(state *models.RuntimeState) { state.EffectivePolicy["baseProfile"] = "nas_balanced" }},
		{name: "disk role type", change: func(state *models.RuntimeState) { state.Disks["workspace"].(models.JSONMap)["type"] = "hdd" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := openRuntimeStateTestDB(t)
			base := testRuntimeState()
			if _, _, err := UpsertDetectedState(db, base); err != nil {
				t.Fatal(err)
			}
			changed := testRuntimeState()
			test.change(&changed)
			if _, transition, err := UpsertDetectedState(db, changed); err != nil || transition == nil {
				t.Fatalf("stable change did not capture one transition: transition=%v err=%v", transition, err)
			}
			var count int64
			if err := db.Model(&models.RuntimeSnapshot{}).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("snapshot count=%d want 1", count)
			}
		})
	}
}
