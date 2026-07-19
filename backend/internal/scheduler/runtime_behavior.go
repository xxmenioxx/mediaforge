package scheduler

import (
	"github.com/anuelvs/mediaforge/backend/internal/runtimeinfo"
	"gorm.io/gorm"
)

type RuntimeBehaviorConfig struct {
	PauseWhenOnBattery     bool `json:"pauseWhenOnBattery"`
	PreventSleepDuringJobs bool `json:"preventSleepDuringJobs"`
}

func LoadRuntimeBehavior(db *gorm.DB, selectedProfile string) (RuntimeBehaviorConfig, error) {
	effective, err := runtimeinfo.ResolveEffectiveRuntimePolicy(db, selectedProfile)
	if err != nil {
		return RuntimeBehaviorConfig{}, err
	}
	return RuntimeBehaviorConfig{PauseWhenOnBattery: effective.Values.PauseWhenOnBattery, PreventSleepDuringJobs: effective.Values.PreventSleepDuringJobs}, nil
}
