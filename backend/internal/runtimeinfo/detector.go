package runtimeinfo

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/anuelvs/mediaforge/backend/internal/capabilities"
	"github.com/anuelvs/mediaforge/backend/internal/models"
	"gorm.io/gorm"
)

var encoderCandidates = []string{"libx264", "libx265", "libsvtav1", "hevc_videotoolbox", "hevc_qsv", "hevc_nvenc", "hevc_amf"}

type Detector struct {
	db   *gorm.DB
	mu   sync.Mutex
	stop chan struct{}
}

func StartDetector(db *gorm.DB) *Detector {
	d := &Detector{db: db, stop: make(chan struct{})}
	go d.run()
	return d
}

func (d *Detector) Stop() { close(d.stop) }

func (d *Detector) run() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_, _ = DetectAndSave(d.db)
		case <-d.stop:
			return
		}
	}
}

func DetectAndSave(db *gorm.DB) (models.RuntimeSnapshot, error) {
	snapshot := models.RuntimeSnapshot{
		DetectedAt: time.Now(), OS: runtime.GOOS, Architecture: runtime.GOARCH,
		Container: detectContainer(), CPUCores: runtime.NumCPU(),
		Disks: models.JSONMap{}, Encoders: models.JSONMap{}, Warnings: models.JSONList{}, SelectionReasons: models.JSONList{},
	}
	snapshot.TotalMemoryBytes, snapshot.AvailableMemoryBytes = detectMemory()
	snapshot.BatteryPresent = detectBattery()

	paths := loadPaths(db)
	for label, path := range paths {
		info, err := diskInfo(path)
		if err != nil {
			snapshot.Warnings = append(snapshot.Warnings, fmt.Sprintf("%s disk: %v", label, err))
			continue
		}
		snapshot.Disks[label] = info
	}
	for _, name := range encoderCandidates {
		capability := capabilities.CheckEncoder(name)
		snapshot.Encoders[name] = models.JSONMap{"listed": capability.Listed, "usable": capability.Usable, "reason": capability.Reason}
	}
	snapshot.RecommendedProfile, snapshot.SelectionReasons = SelectProfile(snapshot)
	snapshot.SelectedProfile = configuredProfile(db, snapshot.RecommendedProfile)
	if snapshot.SelectedProfile != snapshot.RecommendedProfile {
		snapshot.SelectionReasons = append(snapshot.SelectionReasons, "Runtime policy uses the explicitly selected machine profile")
	}
	if err := db.Create(&snapshot).Error; err != nil {
		return models.RuntimeSnapshot{}, err
	}
	return snapshot, nil
}

func Latest(db *gorm.DB) (models.RuntimeSnapshot, error) {
	var snapshot models.RuntimeSnapshot
	err := db.Order("detected_at desc").First(&snapshot).Error
	return snapshot, err
}

func SelectProfile(snapshot models.RuntimeSnapshot) (string, models.JSONList) {
	if snapshot.BatteryPresent {
		return "laptop_safe", models.JSONList{"Battery detected; conservative concurrency and sustained load are preferred"}
	}
	if snapshot.CPUCores >= 16 && snapshot.TotalMemoryBytes >= 32<<30 {
		return "workstation_balanced", models.JSONList{"At least 16 CPU cores and 32 GiB RAM detected"}
	}
	if snapshot.Container && snapshot.CPUCores <= 8 {
		return "nas_balanced", models.JSONList{"Container runtime with at most 8 visible CPU cores detected"}
	}
	if snapshot.CPUCores <= 4 || (snapshot.TotalMemoryBytes > 0 && snapshot.TotalMemoryBytes < 8<<30) {
		return "desktop_safe", models.JSONList{"Limited CPU or memory capacity detected"}
	}
	return "desktop_balanced", models.JSONList{"General-purpose desktop capacity detected"}
}

func configuredProfile(db *gorm.DB, recommended string) string {
	var setting models.AppSetting
	if db.First(&setting, "key = ?", "runtimePolicy").Error != nil {
		return recommended
	}
	mode, _ := setting.Value["mode"].(string)
	selected, _ := setting.Value["selectedProfile"].(string)
	if strings.EqualFold(strings.TrimSpace(mode), "manual") && validProfile(selected) {
		return selected
	}
	return recommended
}

func validProfile(value string) bool {
	switch value {
	case "nas_safe", "nas_balanced", "desktop_safe", "desktop_balanced", "laptop_safe", "workstation_balanced", "workstation_aggressive", "custom":
		return true
	default:
		return false
	}
}

func loadPaths(db *gorm.DB) map[string]string {
	result := map[string]string{}
	var setting models.AppSetting
	if db.First(&setting, "key = ?", "paths").Error != nil {
		return result
	}
	for label, key := range map[string]string{"workspace": "stagingPath", "library": "libraryRoot"} {
		if value, ok := setting.Value[key].(string); ok && strings.TrimSpace(value) != "" {
			result[label] = value
		}
	}
	return result
}

func diskInfo(path string) (models.JSONMap, error) {
	probe := filepath.Clean(path)
	for {
		var stat syscall.Statfs_t
		if err := syscall.Statfs(probe, &stat); err == nil {
			return models.JSONMap{"path": path, "totalBytes": int64(stat.Blocks) * int64(stat.Bsize), "availableBytes": int64(stat.Bavail) * int64(stat.Bsize)}, nil
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return nil, fmt.Errorf("path is not accessible: %s", path)
		}
		probe = parent
	}
}

func detectContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil || os.Getenv("container") != "" {
		return true
	}
	data, _ := os.ReadFile("/proc/1/cgroup")
	text := strings.ToLower(string(data))
	return strings.Contains(text, "docker") || strings.Contains(text, "containerd") || strings.Contains(text, "kubepods")
}

func detectBattery() bool {
	if entries, err := filepath.Glob("/sys/class/power_supply/*/type"); err == nil {
		for _, entry := range entries {
			data, _ := os.ReadFile(entry)
			if strings.EqualFold(strings.TrimSpace(string(data)), "battery") {
				return true
			}
		}
	}
	if runtime.GOOS == "darwin" {
		output, err := exec.Command("pmset", "-g", "batt").CombinedOutput()
		return err == nil && strings.Contains(strings.ToLower(string(output)), "internalbattery")
	}
	return false
}

func detectMemory() (int64, int64) {
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		return parseMemInfo(string(data))
	}
	if runtime.GOOS == "darwin" {
		output, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err == nil {
			total, _ := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64)
			vmOutput, vmErr := exec.Command("vm_stat").Output()
			if vmErr == nil {
				return total, parseVMStat(string(vmOutput))
			}
			return total, 0
		}
	}
	return 0, 0
}

func parseVMStat(value string) int64 {
	pageSize := int64(4096)
	var availablePages int64
	for index, line := range strings.Split(value, "\n") {
		if index == 0 {
			if fields := strings.Fields(line); len(fields) >= 8 {
				if parsed, err := strconv.ParseInt(fields[7], 10, 64); err == nil {
					pageSize = parsed
				}
			}
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		if key != "Pages free" && key != "Pages inactive" && key != "Pages speculative" && key != "Pages purgeable" {
			continue
		}
		amount := strings.TrimSpace(strings.TrimSuffix(parts[1], "."))
		pages, _ := strconv.ParseInt(amount, 10, 64)
		availablePages += pages
	}
	return availablePages * pageSize
}

func parseMemInfo(value string) (int64, int64) {
	var total, available int64
	scanner := bufio.NewScanner(strings.NewReader(value))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		amount, _ := strconv.ParseInt(fields[1], 10, 64)
		switch strings.TrimSuffix(fields[0], ":") {
		case "MemTotal":
			total = amount * 1024
		case "MemAvailable":
			available = amount * 1024
		}
	}
	return total, available
}
