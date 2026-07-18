package scheduler

import (
	"encoding/json"
	"fmt"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"github.com/anuelvs/mediaforge/backend/internal/runtimeinfo"
	"gorm.io/gorm"
)

const (
	WorkspaceModeCopyToWork = "copy_to_work_disk"
	WorkspaceModeDirect     = "direct_mode"
)

type WorkspaceConfig struct {
	PreferredMode         string `json:"preferredMode"`
	FallbackMode          string `json:"fallbackMode"`
	AllowDirectMode       bool   `json:"allowDirectMode"`
	EstimateRequiredSpace bool   `json:"estimateRequiredSpace"`
}

type WorkspaceDecision struct {
	Allowed        bool   `json:"allowed"`
	Mode           string `json:"mode"`
	Reason         string `json:"reason"`
	RequiredBytes  int64  `json:"requiredBytes"`
	AvailableBytes int64  `json:"availableBytes"`
}

func DefaultWorkspaceConfig() WorkspaceConfig {
	return WorkspaceConfig{PreferredMode: WorkspaceModeCopyToWork, FallbackMode: "wait", AllowDirectMode: false, EstimateRequiredSpace: true}
}

func LoadWorkspaceConfig(db *gorm.DB) (WorkspaceConfig, error) {
	config := DefaultWorkspaceConfig()
	var setting models.AppSetting
	result := db.Where("key = ?", "workspace").Limit(1).Find(&setting)
	if result.Error != nil {
		return config, result.Error
	}
	if result.RowsAffected == 0 {
		return config, nil
	}
	data, err := json.Marshal(setting.Value)
	if err != nil {
		return config, err
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return config, err
	}
	return config, nil
}

func EvaluateWorkspace(db *gorm.DB, plan models.ExecutionPlan) (WorkspaceDecision, error) {
	config, err := LoadWorkspaceConfig(db)
	if err != nil {
		return WorkspaceDecision{}, err
	}
	snapshot, err := runtimeinfo.Latest(db)
	if err != nil {
		return WorkspaceDecision{}, err
	}
	available := freeDisk(snapshot.Disks, "workspace")
	limits, err := LoadSchedulerLimits(db, snapshot.SelectedProfile)
	if err != nil {
		return WorkspaceDecision{}, err
	}
	if config.PreferredMode == WorkspaceModeDirect {
		if config.AllowDirectMode && limits.AllowDirectMode {
			return WorkspaceDecision{Allowed: true, Mode: WorkspaceModeDirect, Reason: "Direct workspace mode selected", RequiredBytes: plan.EstimatedOutputMaxBytes, AvailableBytes: available}, nil
		}
		return WorkspaceDecision{Allowed: false, Mode: WorkspaceModeDirect, Reason: "Direct workspace mode is disabled by workspace or machine policy", RequiredBytes: plan.EstimatedOutputMaxBytes, AvailableBytes: available}, nil
	}
	required := plan.EstimatedWorkspaceBytes
	if !config.EstimateRequiredSpace {
		required = plan.EstimatedOutputMaxBytes
	}
	if required <= limits.MaxWorkspaceBytes && available-required >= limits.MinFreeWorkBytes {
		return WorkspaceDecision{Allowed: true, Mode: WorkspaceModeCopyToWork, Reason: "Work disk has enough reserved space", RequiredBytes: required, AvailableBytes: available}, nil
	}
	if config.AllowDirectMode && limits.AllowDirectMode && config.FallbackMode == WorkspaceModeDirect {
		return WorkspaceDecision{Allowed: true, Mode: WorkspaceModeDirect, Reason: "Work disk is insufficient; direct mode fallback selected", RequiredBytes: plan.EstimatedOutputMaxBytes, AvailableBytes: available}, nil
	}
	return WorkspaceDecision{Allowed: false, Mode: WorkspaceModeCopyToWork, Reason: fmt.Sprintf("Workspace requires %d bytes but policy reserve cannot be satisfied", required), RequiredBytes: required, AvailableBytes: available}, nil
}
