package scheduler

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"github.com/anuelvs/mediaforge/backend/internal/runtimeinfo"
	"gorm.io/gorm"
)

type MachineLimits struct {
	MaxRunningJobs        int   `json:"maxRunningJobs"`
	MaxVideoJobs          int   `json:"maxVideoJobs"`
	MaxSoftwareX265Jobs   int   `json:"maxSoftwareX265Jobs"`
	MaxHardwareEncodeJobs int   `json:"maxHardwareEncodeJobs"`
	MinFreeRAMBytes       int64 `json:"minFreeRamBytes"`
	MinFreeWorkBytes      int64 `json:"minFreeWorkBytes"`
	MinFreeLibraryBytes   int64 `json:"minFreeLibraryBytes"`
	MaxWorkspaceBytes     int64 `json:"maxWorkspaceBytes"`
	AllowDirectMode       bool  `json:"allowDirectMode"`
}

type SchedulerLimitsConfig struct {
	UseProfileDefaults    bool  `json:"useProfileDefaults"`
	MaxRunningJobs        int   `json:"maxRunningJobs"`
	MaxVideoJobs          int   `json:"maxVideoJobs"`
	MaxSoftwareX265Jobs   int   `json:"maxSoftwareX265Jobs"`
	MaxHardwareEncodeJobs int   `json:"maxHardwareEncodeJobs"`
	MinFreeRAMGB          int64 `json:"minFreeRamGb"`
	MinFreeWorkGB         int64 `json:"minFreeWorkGb"`
	MinFreeLibraryGB      int64 `json:"minFreeLibraryGb"`
	MaxWorkspaceGB        int64 `json:"maxWorkspaceGb"`
	AllowDirectMode       bool  `json:"allowDirectMode"`
}

type ResourceDecision struct {
	Allowed      bool                       `json:"allowed"`
	WaitingState string                     `json:"waitingState"`
	Reasons      []string                   `json:"reasons"`
	Limits       MachineLimits              `json:"limits"`
	Worker       WorkerAvailabilityDecision `json:"worker"`
}

func LimitsForProfile(name string) MachineLimits {
	gb := int64(1 << 30)
	switch name {
	case "nas_safe":
		return MachineLimits{1, 1, 1, 1, 4 * gb, 80 * gb, 300 * gb, 250 * gb, false}
	case "nas_balanced":
		return MachineLimits{2, 2, 1, 2, 4 * gb, 80 * gb, 500 * gb, 350 * gb, false}
	case "laptop_safe":
		return MachineLimits{1, 1, 1, 1, 6 * gb, 100 * gb, 100 * gb, 150 * gb, true}
	case "workstation_balanced":
		return MachineLimits{3, 3, 2, 3, 8 * gb, 100 * gb, 100 * gb, 500 * gb, true}
	case "workstation_aggressive":
		return MachineLimits{6, 6, 4, 6, 6 * gb, 80 * gb, 80 * gb, 800 * gb, true}
	case "desktop_safe":
		return MachineLimits{1, 1, 1, 1, 4 * gb, 30 * gb, 30 * gb, 200 * gb, true}
	default:
		return MachineLimits{2, 2, 1, 2, 4 * gb, 40 * gb, 50 * gb, 300 * gb, true}
	}
}

func LoadSchedulerLimits(db *gorm.DB, profile string) (MachineLimits, error) {
	limits := LimitsForProfile(profile)
	var setting models.AppSetting
	result := db.Where("key = ?", "schedulerLimits").Limit(1).Find(&setting)
	if result.Error != nil || result.RowsAffected == 0 {
		return limits, result.Error
	}
	config := SchedulerLimitsConfig{UseProfileDefaults: true}
	data, err := json.Marshal(setting.Value)
	if err != nil {
		return limits, err
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return limits, err
	}
	if config.UseProfileDefaults {
		return limits, nil
	}
	gb := int64(1 << 30)
	if config.MaxRunningJobs > 0 {
		limits.MaxRunningJobs = config.MaxRunningJobs
	}
	if config.MaxVideoJobs > 0 {
		limits.MaxVideoJobs = config.MaxVideoJobs
	}
	if config.MaxSoftwareX265Jobs > 0 {
		limits.MaxSoftwareX265Jobs = config.MaxSoftwareX265Jobs
	}
	if config.MaxHardwareEncodeJobs > 0 {
		limits.MaxHardwareEncodeJobs = config.MaxHardwareEncodeJobs
	}
	if config.MinFreeRAMGB > 0 {
		limits.MinFreeRAMBytes = config.MinFreeRAMGB * gb
	}
	if config.MinFreeWorkGB > 0 {
		limits.MinFreeWorkBytes = config.MinFreeWorkGB * gb
	}
	if config.MinFreeLibraryGB > 0 {
		limits.MinFreeLibraryBytes = config.MinFreeLibraryGB * gb
	}
	if config.MaxWorkspaceGB > 0 {
		limits.MaxWorkspaceBytes = config.MaxWorkspaceGB * gb
	}
	limits.AllowDirectMode = config.AllowDirectMode
	return limits, nil
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
	classification, _ := ClassifyJob(JobTypeVideoConversion)
	return models.JSONMap{
		"jobType": string(classification.Type), "weight": string(classification.Weight), "requiresWorkingWindow": classification.RequiresWorkingWindow, "encoder": plan.SelectedEncoder,
		"encoderClass": class, "memoryBytes": memory, "workspaceBytes": plan.EstimatedWorkspaceBytes,
		"libraryBytes": plan.EstimatedOutputMaxBytes,
	}
}

func CanDispatch(db *gorm.DB, plan *models.ExecutionPlan) (bool, []string, error) {
	decision, err := EvaluateResources(db, plan)
	return decision.Allowed, decision.Reasons, err
}

func EvaluateResources(db *gorm.DB, plan *models.ExecutionPlan) (ResourceDecision, error) {
	snapshot, err := runtimeinfo.Latest(db)
	if err != nil {
		return ResourceDecision{Reasons: []string{"Runtime snapshot is unavailable"}}, err
	}
	limits, err := LoadSchedulerLimits(db, snapshot.SelectedProfile)
	if err != nil {
		return ResourceDecision{}, err
	}
	plan.RuntimeProfile = snapshot.SelectedProfile
	plan.RuntimeSnapshotID = &snapshot.ID
	plan.Reservation = BuildReservation(*plan)
	reasons := []string{}
	var activeReservations []models.SchedulerReservation
	if err := db.Where("state = ?", ReservationStateActive).Find(&activeReservations).Error; err != nil {
		return ResourceDecision{}, err
	}
	running := int64(len(activeReservations))
	if int(running) >= limits.MaxRunningJobs {
		reasons = append(reasons, fmt.Sprintf("Running job limit reached (%d/%d)", running, limits.MaxRunningJobs))
	}

	video, software, hardware := 0, 0, 0
	for _, item := range activeReservations {
		if item.Encoder == "" || item.Encoder == "copy" {
			continue
		}
		video++
		if item.Encoder == "libx265" {
			software++
		}
		if item.EncoderClass == "hardware" {
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
	if plan.WorkspaceMode != WorkspaceModeDirect && plan.EstimatedWorkspaceBytes > limits.MaxWorkspaceBytes {
		reasons = append(reasons, fmt.Sprintf("Estimated workspace exceeds policy maximum (%d bytes)", limits.MaxWorkspaceBytes))
	}
	if plan.WorkspaceMode != WorkspaceModeDirect && freeDisk(snapshot.Disks, "workspace")-plan.EstimatedWorkspaceBytes < limits.MinFreeWorkBytes {
		reasons = append(reasons, "Work disk would fall below its free-space reserve")
	}
	if freeDisk(snapshot.Disks, "library")-plan.EstimatedOutputMaxBytes < limits.MinFreeLibraryBytes {
		reasons = append(reasons, "Library disk would fall below its free-space reserve")
	}
	workerDecision, workerErr := EvaluateWorkerAvailability(db, *plan, time.Now())
	if workerErr != nil {
		return ResourceDecision{}, workerErr
	}
	waitingState := ""
	if len(reasons) > 0 {
		waitingState = resourceWaitingState(reasons)
	} else if !workerDecision.Available {
		reasons = append(reasons, workerDecision.Reasons...)
		waitingState = "WAITING_WORKER"
	}
	return ResourceDecision{Allowed: len(reasons) == 0, WaitingState: waitingState, Reasons: reasons, Limits: limits, Worker: workerDecision}, nil
}

func resourceWaitingState(reasons []string) string {
	for _, reason := range reasons {
		if reason == "Work disk would fall below its free-space reserve" || strings.Contains(reason, "workspace exceeds") {
			return "WAITING_SSD_SPACE"
		}
		if reason == "Library disk would fall below its free-space reserve" {
			return "WAITING_HDD_SPACE"
		}
		if strings.Contains(reason, "RAM") {
			return "WAITING_RAM"
		}
	}
	return "WAITING_PROFILE_LIMIT"
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
