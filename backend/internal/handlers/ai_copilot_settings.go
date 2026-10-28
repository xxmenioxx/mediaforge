package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const aiCopilotSettingsKey = "aiCopilotSettings"

const aiCopilotModelMaxLength = 256

func defaultAICopilotSettingsValue() models.JSONMap {
	return models.JSONMap{"enabled": false, "model": "", "preference": "balanced"}
}

func normalizeAICopilotSettingsValue(value models.JSONMap) (models.JSONMap, error) {
	normalized := make(models.JSONMap, len(value)+3)
	for key, item := range value {
		normalized[key] = item
	}

	enabled := false
	if raw, present := value["enabled"]; present {
		parsed, ok := raw.(bool)
		if !ok {
			return nil, fmt.Errorf("enabled must be a boolean")
		}
		enabled = parsed
	}

	model := ""
	if raw, present := value["model"]; present {
		parsed, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("model must be a string")
		}
		model = strings.TrimSpace(parsed)
	}
	if len(model) > aiCopilotModelMaxLength || strings.ContainsAny(model, "\x00\r\n") {
		return nil, fmt.Errorf("model must be a model identifier up to %d characters", aiCopilotModelMaxLength)
	}
	if enabled && model == "" {
		return nil, fmt.Errorf("model is required when AI Copilot is enabled")
	}

	preference := "balanced"
	if raw, present := value["preference"]; present {
		parsed, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("preference must be a string")
		}
		preference = strings.TrimSpace(parsed)
	}
	if preference != "quality" && preference != "balanced" && preference != "compression" {
		return nil, fmt.Errorf("preference must be quality, balanced, or compression")
	}

	normalized["enabled"] = enabled
	normalized["model"] = model
	normalized["preference"] = preference
	return normalized, nil
}

func migrateAICopilotSettings(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("settings database is unavailable")
	}
	now := time.Now()
	setting := models.AppSetting{
		Key:       aiCopilotSettingsKey,
		Value:     defaultAICopilotSettingsValue(),
		CreatedAt: now,
		UpdatedAt: now,
	}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoNothing: true,
	}).Create(&setting).Error
}
