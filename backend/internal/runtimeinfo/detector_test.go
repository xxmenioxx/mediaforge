package runtimeinfo

import (
	"testing"

	"github.com/anuelvs/mediaforge/backend/internal/models"
)

func TestParseMemInfo(t *testing.T) {
	total, available := parseMemInfo("MemTotal: 16384 kB\nMemAvailable: 4096 kB\n")
	if total != 16384*1024 || available != 4096*1024 {
		t.Fatalf("unexpected memory: %d %d", total, available)
	}
}

func TestParseVMStat(t *testing.T) {
	value := "Mach Virtual Memory Statistics: (page size of 16384 bytes)\nPages free: 10.\nPages active: 20.\nPages inactive: 30.\nPages speculative: 2.\nPages purgeable: 3.\n"
	if got := parseVMStat(value); got != 45*16384 {
		t.Fatalf("unexpected available memory: %d", got)
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
