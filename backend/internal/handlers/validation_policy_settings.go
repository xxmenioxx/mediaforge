package handlers

import (
	"encoding/json"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/validationpolicy"
	"gorm.io/gorm"
)

func normalizeValidationPolicySetting(ctx settingsMutationContext, value models.JSONMap) (models.JSONMap, error) {
	policy := validationpolicy.MigrateLegacy(map[string]any(value))
	result := validationPolicyJSONMap(policy)
	if outcomes, ok := value["autoAdvanceOutcomes"]; ok {
		result["autoAdvanceOutcomes"] = outcomes
	} else if ctx.db != nil {
		var current models.AppSetting
		if err := ctx.db.First(&current, "key = ?", "validation").Error; err == nil && current.Value != nil {
			if outcomes, ok := current.Value["autoAdvanceOutcomes"]; ok {
				result["autoAdvanceOutcomes"] = outcomes
			}
		}
	}
	return result, nil
}

func projectValidationPolicySetting(_ settingsMutationContext, value models.JSONMap) (models.JSONMap, error) {
	policy := validationpolicy.MigrateLegacy(map[string]any(value))
	projected := validationPolicyJSONMap(policy)
	if outcomes, ok := value["autoAdvanceOutcomes"]; ok {
		projected["autoAdvanceOutcomes"] = outcomes
	}
	differences := []string{}
	if policy.Preset == validationpolicy.Custom {
		differences = validationpolicy.Differences(policy, policy.BasedOn)
	}
	projected["differenceCount"] = len(differences)
	projected["differences"] = differences
	presetDefaults := models.JSONMap{}
	for _, preset := range []string{validationpolicy.Disabled, validationpolicy.Basic, validationpolicy.Default, validationpolicy.Strict} {
		presetDefaults[preset] = validationPolicyJSONMap(validationpolicy.Factory(preset))
	}
	projected["presetDefaults"] = presetDefaults
	return projected, nil
}

func resolveCurrentValidationPolicy(db *gorm.DB) validationpolicy.Policy {
	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", "validation").Error; err != nil || setting.Value == nil {
		return validationpolicy.DefaultPolicy()
	}
	return validationpolicy.MigrateLegacy(map[string]any(setting.Value))
}

func validationPolicyJSONMap(policy validationpolicy.Policy) models.JSONMap {
	encoded, _ := json.Marshal(policy)
	value := models.JSONMap{}
	_ = json.Unmarshal(encoded, &value)
	return value
}
