package scheduler

import (
	"fmt"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"github.com/anuelvs/mediaforge/backend/internal/runtimeinfo"
	"gorm.io/gorm"
)

type MachineLimits struct {
	MaxRunningJobs, MaxVideoJobs, MaxSoftwareX265Jobs, MaxHardwareEncodeJobs  int
	MinFreeRAMBytes, MinFreeWorkBytes, MinFreeLibraryBytes, MaxWorkspaceBytes int64
}

func LimitsForProfile(name string) MachineLimits {
	gb := int64(1 << 30)
	switch name {
	case "nas_safe":
		return MachineLimits{1, 1, 1, 1, 4 * gb, 80 * gb, 300 * gb, 250 * gb}
	case "nas_balanced":
		return MachineLimits{2, 2, 1, 2, 4 * gb, 80 * gb, 500 * gb, 350 * gb}
	case "laptop_safe":
		return MachineLimits{1, 1, 1, 1, 6 * gb, 100 * gb, 100 * gb, 150 * gb}
	case "workstation_balanced":
		return MachineLimits{3, 3, 2, 3, 8 * gb, 100 * gb, 100 * gb, 500 * gb}
	case "workstation_aggressive":
		return MachineLimits{6, 6, 4, 6, 6 * gb, 80 * gb, 80 * gb, 800 * gb}
	case "desktop_safe":
		return MachineLimits{1, 1, 1, 1, 4 * gb, 30 * gb, 30 * gb, 200 * gb}
	default:
		return MachineLimits{2, 2, 1, 2, 4 * gb, 40 * gb, 50 * gb, 300 * gb}
	}
}

func BuildReservation(plan models.ExecutionPlan) models.JSONMap {
	class := "software"
	if isHardwareEncoder(plan.SelectedEncoder) {
		class = "hardware"
	}
	memory := int64(4 << 30)
	if class == "hardware" {
		memory = 2 << 30
	}
	return models.JSONMap{
		"jobType": "video_conversion", "weight": "heavy", "encoder": plan.SelectedEncoder,
		"encoderClass": class, "memoryBytes": memory, "workspaceBytes": plan.EstimatedWorkspaceBytes,
		"libraryBytes": plan.EstimatedOutputMaxBytes,
	}
}

func CanDispatch(db *gorm.DB, plan *models.ExecutionPlan) (bool, []string, error) {
	snapshot, err := runtimeinfo.Latest(db)
	if err != nil {
		return false, []string{"Runtime snapshot is unavailable"}, err
	}
	limits := LimitsForProfile(snapshot.SelectedProfile)
	plan.RuntimeProfile = snapshot.SelectedProfile
	plan.RuntimeSnapshotID = &snapshot.ID
	plan.Reservation = BuildReservation(*plan)
	reasons := []string{}
	var running int64
	if err := db.Model(&models.QueueJob{}).Where("status = ?", "running").Count(&running).Error; err != nil {
		return false, nil, err
	}
	if int(running) >= limits.MaxRunningJobs {
		reasons = append(reasons, fmt.Sprintf("Running job limit reached (%d/%d)", running, limits.MaxRunningJobs))
	}

	var runningPlans []models.ExecutionPlan
	if err := db.Model(&models.ExecutionPlan{}).Joins("JOIN queue_jobs ON queue_jobs.active_execution_plan_id = execution_plans.id").Where("queue_jobs.status = ?", "running").Find(&runningPlans).Error; err != nil {
		return false, nil, err
	}
	video, software, hardware := 0, 0, 0
	for _, item := range runningPlans {
		if item.SelectedEncoder == "" || item.SelectedEncoder == "copy" {
			continue
		}
		video++
		if item.SelectedEncoder == "libx265" {
			software++
		}
		if isHardwareEncoder(item.SelectedEncoder) {
			hardware++
		}
	}
	if video >= limits.MaxVideoJobs {
		reasons = append(reasons, fmt.Sprintf("Video job limit reached (%d/%d)", video, limits.MaxVideoJobs))
	}
	if plan.SelectedEncoder == "libx265" && software >= limits.MaxSoftwareX265Jobs {
		reasons = append(reasons, fmt.Sprintf("Software x265 limit reached (%d/%d)", software, limits.MaxSoftwareX265Jobs))
	}
	if isHardwareEncoder(plan.SelectedEncoder) && hardware >= limits.MaxHardwareEncodeJobs {
		reasons = append(reasons, fmt.Sprintf("Hardware encoder limit reached (%d/%d)", hardware, limits.MaxHardwareEncodeJobs))
	}
	if snapshot.AvailableMemoryBytes > 0 && snapshot.AvailableMemoryBytes < limits.MinFreeRAMBytes {
		reasons = append(reasons, fmt.Sprintf("Free RAM is below policy minimum (%d bytes required)", limits.MinFreeRAMBytes))
	}
	if plan.EstimatedWorkspaceBytes > limits.MaxWorkspaceBytes {
		reasons = append(reasons, fmt.Sprintf("Estimated workspace exceeds policy maximum (%d bytes)", limits.MaxWorkspaceBytes))
	}
	if freeDisk(snapshot.Disks, "workspace")-plan.EstimatedWorkspaceBytes < limits.MinFreeWorkBytes {
		reasons = append(reasons, "Work disk would fall below its free-space reserve")
	}
	if freeDisk(snapshot.Disks, "library")-plan.EstimatedOutputMaxBytes < limits.MinFreeLibraryBytes {
		reasons = append(reasons, "Library disk would fall below its free-space reserve")
	}
	return len(reasons) == 0, reasons, nil
}

func freeDisk(disks models.JSONMap, role string) int64 {
	entry, ok := disks[role].(map[string]any)
	if !ok {
		if typed, yes := disks[role].(models.JSONMap); yes {
			entry = typed
		}
	}
	if value, ok := entry["availableBytes"].(float64); ok {
		return int64(value)
	}
	if value, ok := entry["availableBytes"].(int64); ok {
		return value
	}
	return 1<<63 - 1
}
