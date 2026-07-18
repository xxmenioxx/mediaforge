package scheduler

import (
	"encoding/json"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"gorm.io/gorm"
)

type RuntimeBehaviorConfig struct {
	PauseWhenOnBattery     bool `json:"pauseWhenOnBattery"`
	PreventSleepDuringJobs bool `json:"preventSleepDuringJobs"`
}

func LoadRuntimeBehavior(db *gorm.DB, selectedProfile string) (RuntimeBehaviorConfig, error) {
	config := RuntimeBehaviorConfig{PauseWhenOnBattery: selectedProfile == "laptop_safe", PreventSleepDuringJobs: selectedProfile == "laptop_safe"}
	var setting models.AppSetting
	result := db.Where("key = ?", "runtimePolicy").Limit(1).Find(&setting)
	if result.Error != nil || result.RowsAffected == 0 {
		return config, result.Error
	}
	data, err := json.Marshal(setting.Value)
	if err != nil {
		return config, err
	}
	var values struct {
		PauseWhenOnBattery     *bool `json:"pauseWhenOnBattery"`
		PreventSleepDuringJobs *bool `json:"preventSleepDuringJobs"`
	}
	if err := json.Unmarshal(data, &values); err != nil {
		return config, err
	}
	if values.PauseWhenOnBattery != nil {
		config.PauseWhenOnBattery = *values.PauseWhenOnBattery
	}
	if values.PreventSleepDuringJobs != nil {
		config.PreventSleepDuringJobs = *values.PreventSleepDuringJobs
	}
	return config, nil
}
