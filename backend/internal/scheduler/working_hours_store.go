package scheduler

import (
	"encoding/json"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
)

func LoadWorkingHoursConfig(db *gorm.DB) (WorkingHoursConfig, error) {
	config := DefaultWorkingHoursConfig()
	var setting models.AppSetting
	result := db.Where("key = ?", "workingHours").Limit(1).Find(&setting)
	if result.Error != nil {
		return config, result.Error
	}
	if result.RowsAffected == 0 {
		return config, nil
	}
	encoded, err := json.Marshal(setting.Value)
	if err != nil {
		return config, err
	}
	if err := json.Unmarshal(encoded, &config); err != nil {
		return config, err
	}
	if config.Timezone == "" {
		config.Timezone = DefaultWorkingHoursConfig().Timezone
	}
	return config, nil
}

func EvaluatePlanWorkingHours(db *gorm.DB, now time.Time) (WorkingHoursDecision, JobClassification, error) {
	classification, err := ClassifyJob(JobTypeVideoConversion)
	if err != nil {
		return WorkingHoursDecision{}, JobClassification{}, err
	}
	config, err := LoadWorkingHoursConfig(db)
	if err != nil {
		return WorkingHoursDecision{}, classification, err
	}
	decision, err := EvaluateWorkingHours(config, now, classification.RequiresWorkingWindow)
	return decision, classification, err
}
