package runtimeinfo

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/capabilities"
	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
)

var encoderCandidates = []string{"libx264", "libx265", "libsvtav1", "hevc_videotoolbox", "hevc_qsv", "hevc_vaapi", "hevc_nvenc", "hevc_amf"}

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
			if _, err := DetectAndSave(d.db); err != nil {
				log.Printf("runtime detector: %v", err)
			}
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
	snapshot.CPULoad1 = detectCPULoad1()
	snapshot.BatteryPresent, snapshot.PowerSource, snapshot.OnBattery, snapshot.BatteryPercent = detectPower()

	paths := loadDiskProbePaths(db)
	for label, probePaths := range paths {
		info, err := diskInfoForPaths(probePaths)
		if err != nil {
			snapshot.Warnings = append(snapshot.Warnings, fmt.Sprintf("%s disk: %v", label, err))
			continue
		}
		snapshot.Disks[label] = info
	}
	for _, name := range encoderCandidates {
		capability := capabilities.CheckEncoder(name)
		snapshot.Encoders[name] = models.JSONMap{
			"listed": capability.Listed, "usable": capability.Usable, "main10": capability.Main10,
			"icq": capability.ICQ, "lowPower": capability.LowPower,
			"lookAhead": capability.LookAhead, "extendedBrc": capability.ExtendedBRC,
			"adaptiveI": capability.AdaptiveI, "adaptiveB": capability.AdaptiveB,
			"reason": capability.Reason,
		}
	}
	snapshot.RecommendedProfile, snapshot.SelectionReasons = SelectProfile(snapshot)
	effective, err := ResolveEffectiveRuntimePolicy(db, snapshot.RecommendedProfile)
	if err != nil {
		return models.RuntimeSnapshot{}, err
	}
	snapshot.SelectedProfile = effective.BaseProfile
	snapshot.PreferredProfile = effective.PreferredProfile
	snapshot.FallbackProfile = effective.FallbackProfile
	snapshot.AppliedOverrides = models.JSONList{}
	for _, field := range effective.OverriddenFields {
		snapshot.AppliedOverrides = append(snapshot.AppliedOverrides, field)
	}
	encoded, _ := json.Marshal(effective.Values)
	_ = json.Unmarshal(encoded, &snapshot.EffectivePolicy)
	snapshot.SelectionReasons = effective.SelectionReasons
	if err := db.Create(&snapshot).Error; err != nil {
		return models.RuntimeSnapshot{}, err
	}
	return snapshot, nil
}

// loadDiskProbePaths prefers registered library destinations because Docker
// deployments commonly bind-mount each library below an unmounted parent such
// as /media/library. Probing that parent reports the container filesystem, not
// the disk that will receive the converted file.
func loadDiskProbePaths(db *gorm.DB) map[string][]string {
	configured := loadPaths(db)
	result := map[string][]string{}
	for label, path := range configured {
		if strings.TrimSpace(path) != "" {
			result[label] = []string{path}
		}
	}

	var libraries []models.Library
	if err := db.Order("id asc").Find(&libraries).Error; err == nil {
		seen := map[string]bool{}
		var destinations []string
		for _, library := range libraries {
			path := strings.TrimSpace(library.DestinationPath)
			if path != "" && !seen[path] {
				seen[path] = true
				destinations = append(destinations, path)
			}
		}
		if len(destinations) > 0 {
			result["library"] = destinations
		}
	}
	return result
}

func diskInfoForPaths(paths []string) (models.JSONMap, error) {
	var selected models.JSONMap
	var selectedAvailable int64
	probed := models.JSONList{}
	warnings := models.JSONList{}
	for _, path := range paths {
		info, err := diskInfo(path)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		probed = append(probed, path)
		available, _ := info["availableBytes"].(int64)
		if selected == nil || available < selectedAvailable {
			selected, selectedAvailable = info, available
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("none of the configured paths are accessible: %v", paths)
	}
	selected["probePaths"] = probed
	if len(warnings) > 0 {
		selected["probeWarnings"] = warnings
	}
	return selected, nil
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

func loadPaths(db *gorm.DB) map[string]string {
	result := map[string]string{}
	var roles models.AppSetting
	if db.Where("key = ?", "storageRoles").Limit(1).Find(&roles).RowsAffected > 0 {
		for label, role := range map[string]string{"workspace": "work", "library": "library"} {
			if entry, ok := roles.Value[role].(map[string]any); ok {
				if value, ok := entry["path"].(string); ok && strings.TrimSpace(value) != "" {
					result[label] = value
				}
			}
		}
		if len(result) == 2 {
			return result
		}
	}
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
			return models.JSONMap{"path": path, "totalBytes": int64(stat.Blocks) * int64(stat.Bsize), "availableBytes": int64(stat.Bavail) * int64(stat.Bsize), "type": detectDiskType(probe)}, nil
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

func detectPower() (bool, string, bool, int) {
	batteryPresent, acOnline, percent := false, false, 0
	if entries, err := filepath.Glob("/sys/class/power_supply/*/type"); err == nil {
		for _, entry := range entries {
			data, _ := os.ReadFile(entry)
			typeName := strings.ToLower(strings.TrimSpace(string(data)))
			root := filepath.Dir(entry)
			if typeName == "battery" {
				batteryPresent = true
				if value, err := os.ReadFile(filepath.Join(root, "capacity")); err == nil {
					percent, _ = strconv.Atoi(strings.TrimSpace(string(value)))
				}
			} else if typeName == "mains" || typeName == "usb" || typeName == "usb_c" {
				if value, err := os.ReadFile(filepath.Join(root, "online")); err == nil && strings.TrimSpace(string(value)) == "1" {
					acOnline = true
				}
			}
		}
		if batteryPresent {
			if acOnline {
				return true, "ac", false, percent
			}
			return true, "battery", true, percent
		}
	}
	if runtime.GOOS == "darwin" {
		output, err := exec.Command("pmset", "-g", "batt").CombinedOutput()
		if err == nil {
			text := strings.ToLower(string(output))
			batteryPresent = strings.Contains(text, "internalbattery")
			onBattery := strings.Contains(text, "battery power")
			if index := strings.Index(text, "%"); index > 0 {
				start := index - 1
				for start > 0 && text[start-1] >= '0' && text[start-1] <= '9' {
					start--
				}
				percent, _ = strconv.Atoi(text[start:index])
			}
			if batteryPresent {
				if onBattery {
					return true, "battery", true, percent
				}
				return true, "ac", false, percent
			}
		}
	}
	return false, "unknown", false, 0
}

func detectCPULoad1() float64 {
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) > 0 {
			value, _ := strconv.ParseFloat(fields[0], 64)
			return value
		}
	}
	if runtime.GOOS == "darwin" {
		if output, err := exec.Command("sysctl", "-n", "vm.loadavg").Output(); err == nil {
			text := strings.Trim(string(output), "{} \n\t")
			fields := strings.Fields(text)
			if len(fields) > 0 {
				value, _ := strconv.ParseFloat(fields[0], 64)
				return value
			}
		}
	}
	return 0
}

func detectDiskType(path string) string {
	if runtime.GOOS == "darwin" {
		output, err := exec.Command("diskutil", "info", path).CombinedOutput()
		if err != nil {
			return "unknown"
		}
		for _, line := range strings.Split(strings.ToLower(string(output)), "\n") {
			if strings.Contains(line, "solid state:") {
				if strings.HasSuffix(strings.TrimSpace(line), "yes") {
					return "ssd"
				}
				if strings.HasSuffix(strings.TrimSpace(line), "no") {
					return "hdd"
				}
			}
		}
		return "unknown"
	}
	output, err := exec.Command("df", "-P", path).Output()
	if err != nil {
		return "unknown"
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return "unknown"
	}
	device := strings.Fields(lines[len(lines)-1])
	if len(device) == 0 || !strings.HasPrefix(device[0], "/dev/") {
		return "unknown"
	}
	name := strings.TrimPrefix(device[0], "/dev/")
	for len(name) > 0 && name[len(name)-1] >= '0' && name[len(name)-1] <= '9' {
		name = name[:len(name)-1]
	}
	name = strings.TrimSuffix(name, "p")
	if strings.HasPrefix(name, "nvme") {
		return "nvme"
	}
	rotational, err := os.ReadFile(filepath.Join("/sys/block", name, "queue/rotational"))
	if err != nil {
		return "unknown"
	}
	if strings.TrimSpace(string(rotational)) == "0" {
		return "ssd"
	}
	return "hdd"
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
