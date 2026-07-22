package scheduler

import (
	"fmt"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/runtimeinfo"
	"gorm.io/gorm"
)

type MachineLimits struct {
	MaxRunningJobs        int   `json:"maxRunningJobs"`
	MaxVideoJobs          int   `json:"maxVideoJobs"`
	MaxSoftwareX265Jobs   int   `json:"maxSoftwareX265Jobs"`
	MaxHardwareEncodeJobs int   `json:"maxHardwareEncodeJobs"`
	MaxAudioJobs          int   `json:"maxAudioJobs"`
	MaxLabJobs            int   `json:"maxLabJobs"`
	MinFreeRAMBytes       int64 `json:"minFreeRamBytes"`
	MinFreeWorkBytes      int64 `json:"minFreeWorkBytes"`
	MinFreeLibraryBytes   int64 `json:"minFreeLibraryBytes"`
	MaxWorkspaceBytes     int64 `json:"maxWorkspaceBytes"`
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
	profile, ok := runtimeinfo.RuntimeProfile(name)
	if !ok {
		profile, _ = runtimeinfo.RuntimeProfile("desktop_safe")
	}
	value := profile.Values
	return MachineLimits{value.MaxRunningJobs, value.MaxVideoJobs, value.MaxSoftwareX265Jobs, value.MaxHardwareEncodeJobs, value.MaxAudioJobs, value.MaxLabJobs, value.MinFreeRAMGB * gb, value.MinFreeWorkGB * gb, value.MinFreeLibraryGB * gb, value.MaxWorkspaceGB * gb, value.AllowDirectMode}
}

func LoadSchedulerLimits(db *gorm.DB, profile string) (MachineLimits, error) {
	effective, err := runtimeinfo.ResolveEffectiveRuntimePolicy(db, profile)
	if err != nil {
		return MachineLimits{}, err
	}
	config := effective.Values
	gb := int64(1 << 30)
	return MachineLimits{config.MaxRunningJobs, config.MaxVideoJobs, config.MaxSoftwareX265Jobs, config.MaxHardwareEncodeJobs, config.MaxAudioJobs, config.MaxLabJobs, config.MinFreeRAMGB * gb, config.MinFreeWorkGB * gb, config.MinFreeLibraryGB * gb, config.MaxWorkspaceGB * gb, config.AllowDirectMode}, nil
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
	runtimeBehavior, err := LoadRuntimeBehavior(db, snapshot.SelectedProfile)
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

	video, software, hardware, audio, lab := 0, 0, 0, 0, 0
	for _, item := range activeReservations {
		if item.JobType == string(JobTypeAudioRestoration) {
			audio++
		}
		if item.JobType == string(JobTypeLabPreview) {
			lab++
		}
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
	jobType := jsonString(plan.Reservation, "jobType")
	if jobType == string(JobTypeAudioRestoration) && audio >= limits.MaxAudioJobs {
		reasons = append(reasons, fmt.Sprintf("Audio job limit reached (%d/%d)", audio, limits.MaxAudioJobs))
	}
	if jobType == string(JobTypeLabPreview) && lab >= limits.MaxLabJobs {
		reasons = append(reasons, fmt.Sprintf("Lab job limit reached (%d/%d)", lab, limits.MaxLabJobs))
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
	if snapshot.OnBattery && runtimeBehavior.PauseWhenOnBattery {
		reasons = append([]string{"Machine is running on battery and runtime policy pauses new jobs"}, reasons...)
		waitingState = "WAITING_POWER"
	} else if len(reasons) > 0 {
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
