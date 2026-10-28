package handlers

import (
	"fmt"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ocrCorrectionMemorySettingsKey          = "ocrCorrectionMemorySettings"
	ocrCorrectionMemoryDefaultRetentionDays = 180
	ocrCorrectionMemoryMinRetentionDays     = 30
	ocrCorrectionMemoryMaxRetentionDays     = 3650
)

type ocrCorrectionMemorySettings struct {
	Enabled            bool
	RetentionDays      int
	IncludeInAIPrompts bool
}

func defaultOCRCorrectionMemorySettingsValue() models.JSONMap {
	return models.JSONMap{"enabled": false, "retentionDays": ocrCorrectionMemoryDefaultRetentionDays, "includeInAIPrompts": false}
}

func normalizeOCRCorrectionMemorySettingsValue(value models.JSONMap) (models.JSONMap, error) {
	normalized := make(models.JSONMap, len(value)+3)
	for key, item := range value {
		normalized[key] = item
	}
	enabled, err := optionalBoolSetting(value, "enabled", false)
	if err != nil {
		return nil, err
	}
	includeInAIPrompts, err := optionalBoolSetting(value, "includeInAIPrompts", false)
	if err != nil {
		return nil, err
	}
	retentionDays := ocrCorrectionMemoryDefaultRetentionDays
	if raw, present := value["retentionDays"]; present {
		switch parsed := raw.(type) {
		case int:
			retentionDays = parsed
		case int64:
			retentionDays = int(parsed)
		case float64:
			if parsed != float64(int(parsed)) {
				return nil, fmt.Errorf("retentionDays must be a whole number")
			}
			retentionDays = int(parsed)
		default:
			return nil, fmt.Errorf("retentionDays must be a whole number")
		}
	}
	if retentionDays < ocrCorrectionMemoryMinRetentionDays || retentionDays > ocrCorrectionMemoryMaxRetentionDays {
		return nil, fmt.Errorf("retentionDays must be between %d and %d", ocrCorrectionMemoryMinRetentionDays, ocrCorrectionMemoryMaxRetentionDays)
	}
	normalized["enabled"] = enabled
	normalized["retentionDays"] = retentionDays
	normalized["includeInAIPrompts"] = includeInAIPrompts
	return normalized, nil
}

func optionalBoolSetting(value models.JSONMap, key string, fallback bool) (bool, error) {
	raw, present := value[key]
	if !present {
		return fallback, nil
	}
	parsed, ok := raw.(bool)
	if !ok {
		return false, fmt.Errorf("%s must be a boolean", key)
	}
	return parsed, nil
}

func loadOCRCorrectionMemorySettings(db *gorm.DB) (ocrCorrectionMemorySettings, error) {
	if db == nil {
		return ocrCorrectionMemorySettings{}, fmt.Errorf("OCR correction memory settings are unavailable")
	}
	var setting models.AppSetting
	err := db.First(&setting, "key = ?", ocrCorrectionMemorySettingsKey).Error
	if err == gorm.ErrRecordNotFound {
		return ocrCorrectionMemorySettings{RetentionDays: ocrCorrectionMemoryDefaultRetentionDays}, nil
	}
	if err != nil {
		return ocrCorrectionMemorySettings{}, err
	}
	value, err := normalizeOCRCorrectionMemorySettingsValue(setting.Value)
	if err != nil {
		return ocrCorrectionMemorySettings{}, err
	}
	return ocrCorrectionMemorySettings{
		Enabled: value["enabled"].(bool), RetentionDays: value["retentionDays"].(int),
		IncludeInAIPrompts: value["includeInAIPrompts"].(bool),
	}, nil
}

func migrateOCRCorrectionMemorySettings(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("settings database is unavailable")
	}
	now := time.Now()
	setting := models.AppSetting{Key: ocrCorrectionMemorySettingsKey, Value: defaultOCRCorrectionMemorySettingsValue(), CreatedAt: now, UpdatedAt: now}
	return db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoNothing: true}).Create(&setting).Error
}
