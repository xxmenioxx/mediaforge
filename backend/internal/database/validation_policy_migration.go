package database

import (
	"encoding/json"
	"errors"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/validationpolicy"
	"gorm.io/gorm"
)

func migrateValidationPolicySetting(db *gorm.DB) error {
	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", "validation").Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	policy := validationpolicy.MigrateLegacy(map[string]any(setting.Value))
	value := validationPolicySettingValue(policy)
	if outcomes, ok := setting.Value["autoAdvanceOutcomes"]; ok {
		value["autoAdvanceOutcomes"] = outcomes
	}
	current, _ := json.Marshal(setting.Value)
	migrated, _ := json.Marshal(value)
	if string(current) == string(migrated) {
		return nil
	}
	return db.Model(&models.AppSetting{}).Where("key = ?", "validation").Update("value", value).Error
}

func validationPolicySettingValue(policy validationpolicy.Policy) models.JSONMap {
	encoded, _ := json.Marshal(policy)
	value := models.JSONMap{}
	_ = json.Unmarshal(encoded, &value)
	return value
}
