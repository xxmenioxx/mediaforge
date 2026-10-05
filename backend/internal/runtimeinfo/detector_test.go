package runtimeinfo

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestParseMemInfo(t *testing.T) {
	total, available := parseMemInfo("MemTotal: 16384 kB\nMemAvailable: 4096 kB\n")
	if total != 16384*1024 || available != 4096*1024 {
		t.Fatalf("unexpected memory: %d %d", total, available)
	}
}

func TestDiskInfoDoesNotAttributeParentFilesystemToMissingDestination(t *testing.T) {
	parent := t.TempDir()
	missing := filepath.Join(parent, "not-created")
	if _, err := diskInfo(missing); err == nil {
		t.Fatalf("diskInfo(%q) unexpectedly reported parent filesystem capacity", missing)
	}
}

func TestDiskInfoForPathsDeduplicatesSharedVolumesAndKeepsDistinctVolumes(t *testing.T) {
	probes := map[string]models.JSONMap{
		"/workspace":      {"measuredPath": "/vol-a/workspace", "volumeId": "vol-a", "totalBytes": int64(1000), "availableBytes": int64(700), "type": "ssd"},
		"/library/anime":  {"measuredPath": "/vol-a/library/anime", "volumeId": "vol-a", "totalBytes": int64(1000), "availableBytes": int64(700), "type": "ssd"},
		"/library/movies": {"measuredPath": "/vol-b/library/movies", "volumeId": "vol-b", "totalBytes": int64(2000), "availableBytes": int64(500), "type": "hdd"},
	}
	got, err := diskInfoForPathsWithProbe([]string{"/workspace", "/library/anime", "/library/movies"}, func(path string) (models.JSONMap, error) {
		return probes[path], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	filesystems, ok := got["filesystems"].(models.JSONList)
	if !ok || len(filesystems) != 2 {
		t.Fatalf("filesystems=%#v want two unique volumes", got["filesystems"])
	}
	shared := filesystems[0].(models.JSONMap)
	if shared["volumeId"] != "vol-a" || len(shared["paths"].(models.JSONList)) != 2 {
		t.Fatalf("shared-volume destinations were not grouped: %#v", shared)
	}
	if got["availableBytes"] != int64(500) {
		t.Fatalf("scheduler summary availableBytes=%v want least-free filesystem 500", got["availableBytes"])
	}
}

func TestDiskInfoForPathsKeepsUnavailableDestinationWithoutParentCapacity(t *testing.T) {
	got, err := diskInfoForPathsWithProbe([]string{"/workspace", "/missing/library"}, func(path string) (models.JSONMap, error) {
		if path == "/missing/library" {
			return nil, errors.New("destination is not accessible")
		}
		return models.JSONMap{"measuredPath": "/vol-a/workspace", "volumeId": "vol-a", "totalBytes": int64(1000), "availableBytes": int64(700)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	destinations := got["destinations"].(models.JSONList)
	missing := destinations[1].(models.JSONMap)
	if missing["accessible"] != false || missing["path"] != "/missing/library" || missing["measuredPath"] != nil || missing["error"] == nil {
		t.Fatalf("unavailable destination was misreported: %#v", missing)
	}
	filesystems := got["filesystems"].(models.JSONList)
	if len(filesystems) != 1 || filesystems[0].(models.JSONMap)["volumeId"] != "vol-a" {
		t.Fatalf("unavailable destination contributed a filesystem: %#v", filesystems)
	}
}

func TestDiskInfoForPathsReturnsUnavailableDestinationsWhenNoneCanBeMeasured(t *testing.T) {
	got, err := diskInfoForPathsWithProbe([]string{"/missing/a", "/missing/b"}, func(path string) (models.JSONMap, error) {
		return nil, errors.New("unavailable")
	})
	if err == nil {
		t.Fatal("expected an unavailable-filesystems error")
	}
	destinations := got["destinations"].(models.JSONList)
	if len(destinations) != 2 || destinations[0].(models.JSONMap)["accessible"] != false {
		t.Fatalf("unavailable destinations were dropped: %#v", destinations)
	}
	if len(got["filesystems"].(models.JSONList)) != 0 || got["availableBytes"] != nil {
		t.Fatalf("unavailable paths received capacity data: %#v", got)
	}
}

func TestLoadDiskProbePathsUsesRegisteredLibraryDestinations(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:runtime-disk-paths?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}, &models.Library{}); err != nil {
		t.Fatal(err)
	}
	roles := models.AppSetting{Key: "storageRoles", Value: models.JSONMap{
		"work":    models.JSONMap{"path": "/media/staging"},
		"library": models.JSONMap{"path": "/media/library"},
	}}
	if err := db.Create(&roles).Error; err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/media/library/anime-movies", "/media/library/movies", "/media/library/anime-movies"} {
		library := models.Library{Name: path, SourcePath: "/media/raw", DestinationPath: path, Type: "movies"}
		if err := db.Create(&library).Error; err != nil {
			t.Fatal(err)
		}
	}

	got := loadDiskProbePaths(db)
	if want := []string{"/media/staging"}; !reflect.DeepEqual(got["workspace"], want) {
		t.Fatalf("workspace paths=%v want=%v", got["workspace"], want)
	}
	if want := []string{"/media/library/anime-movies", "/media/library/movies"}; !reflect.DeepEqual(got["library"], want) {
		t.Fatalf("library paths=%v want=%v", got["library"], want)
	}
}

func TestParseVMStat(t *testing.T) {
	value := "Mach Virtual Memory Statistics: (page size of 16384 bytes)\nPages free: 10.\nPages active: 20.\nPages inactive: 30.\nPages speculative: 2.\nPages purgeable: 3.\n"
	if got := parseVMStat(value); got != 45*16384 {
		t.Fatalf("unexpected available memory: %d", got)
	}
}

func TestParseDRMEngineCountersAndUsage(t *testing.T) {
	first := parseDRMEngineCounters("drm-driver: i915\ndrm-engine-render: 100000000 ns\ndrm-engine-video: 200000000 ns\n")
	second := parseDRMEngineCounters("drm-engine-render: 150000000 ns\ndrm-engine-video: 300000000 ns\n")
	usage := drmEngineUsage(first, second, 0.5)
	if usage["render"] != 10 || usage["video"] != 20 {
		t.Fatalf("unexpected DRM engine usage: %#v", usage)
	}
}

func TestSelectProfile(t *testing.T) {
	tests := []struct {
		name     string
		snapshot models.RuntimeSnapshot
		want     string
	}{
		{"laptop", models.RuntimeSnapshot{BatteryPresent: true, CPUCores: 16, TotalMemoryBytes: 64 << 30}, "laptop_safe"},
		{"workstation", models.RuntimeSnapshot{CPUCores: 16, TotalMemoryBytes: 32 << 30}, "workstation_balanced"},
		{"nas", models.RuntimeSnapshot{Container: true, CPUCores: 4, TotalMemoryBytes: 16 << 30}, "nas_balanced"},
		{"small", models.RuntimeSnapshot{CPUCores: 4, TotalMemoryBytes: 4 << 30}, "desktop_safe"},
		{"desktop", models.RuntimeSnapshot{CPUCores: 8, TotalMemoryBytes: 16 << 30}, "desktop_balanced"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := SelectProfile(tt.snapshot)
			if got != tt.want {
				t.Fatalf("got %s want %s", got, tt.want)
			}
		})
	}
}
