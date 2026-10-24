package handlers

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

const ocrAISettingsKey = "ocrAISettings"

const ocrAIModelMaxLength = 256

func defaultOCRAISettingsValue() models.JSONMap {
	return models.JSONMap{"enabled": false, "model": ""}
}

func normalizeOCRAISettingsValue(value models.JSONMap) (models.JSONMap, error) {
	normalized := make(models.JSONMap, len(value)+2)
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
	if len(model) > ocrAIModelMaxLength || strings.ContainsAny(model, "\x00\r\n") {
		return nil, fmt.Errorf("model must be a model identifier up to %d characters", ocrAIModelMaxLength)
	}
	if enabled && model == "" {
		return nil, fmt.Errorf("model is required when OCR AI is enabled")
	}

	normalized["enabled"] = enabled
	normalized["model"] = model
	return normalized, nil
}

// migrateOCRAISettings adds only the new independent feature setting. Existing
// Translation configuration is intentionally not consulted or copied.
func migrateOCRAISettings(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("settings database is unavailable")
	}
	now := time.Now()
	setting := models.AppSetting{
		Key:       ocrAISettingsKey,
		Value:     defaultOCRAISettingsValue(),
		CreatedAt: now,
		UpdatedAt: now,
	}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoNothing: true,
	}).Create(&setting).Error
}
