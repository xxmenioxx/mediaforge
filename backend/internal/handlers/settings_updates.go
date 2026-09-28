package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (h SettingsHandler) updateSettings(ctx context.Context, inputs []SettingInput) ([]models.AppSetting, error) {
	if len(inputs) == 0 {
		return nil, settingPolicyError{status: http.StatusBadRequest, message: "settings batch must not be empty"}
	}
	updated := make([]models.AppSetting, 0, len(inputs))
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		mutation := settingsMutationContext{ctx: ctx, db: tx, secrets: h.secretSource()}
		definitions := make([]settingDefinition, len(inputs))
		values := make([]models.JSONMap, len(inputs))
		seen := make(map[string]struct{}, len(inputs))
		for index, input := range inputs {
			if input.Key == "" {
				return settingPolicyError{status: http.StatusBadRequest, message: "setting key is required"}
			}
			if _, exists := seen[input.Key]; exists {
				return settingPolicyError{status: http.StatusBadRequest, message: fmt.Sprintf("duplicate setting key %q", input.Key)}
			}
			seen[input.Key] = struct{}{}
			definition := settingDefinitionFor(input.Key)
			value := input.Value
			if value == nil {
				return settingPolicyError{status: http.StatusBadRequest, message: fmt.Sprintf("setting %q value must be an object", input.Key)}
			}
			if definition.Validate != nil {
				if err := definition.Validate(mutation, value); err != nil {
					return asSettingPolicyError(err)
				}
			}
			if definition.Normalize != nil {
				normalized, err := definition.Normalize(mutation, value)
				if err != nil {
					return asSettingPolicyError(err)
				}
				value = normalized
			}
			definitions[index] = definition
			values[index] = value
		}

		now := time.Now()
		for index, input := range inputs {
			setting := models.AppSetting{Key: input.Key, Value: values[index], CreatedAt: now, UpdatedAt: now}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "key"}},
				DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
			}).Create(&setting).Error; err != nil {
				return fmt.Errorf("unable to save setting %q: %w", input.Key, err)
			}
		}
		for _, definition := range definitions {
			if definition.AfterSave != nil {
				if err := definition.AfterSave(mutation); err != nil {
					return fmt.Errorf("unable to apply update effect for setting %q: %w", definition.Key, err)
				}
			}
		}

		for index, input := range inputs {
			var setting models.AppSetting
			if err := tx.First(&setting, "key = ?", input.Key).Error; err != nil {
				return fmt.Errorf("unable to load setting %q: %w", input.Key, err)
			}
			if definition := definitions[index]; definition.Project != nil {
				projected, err := definition.Project(mutation, setting.Value)
				if err != nil {
					return fmt.Errorf("unable to project setting %q: %w", input.Key, err)
				}
				setting.Value = projected
			}
			updated = append(updated, setting)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func asSettingPolicyError(err error) error {
	if _, ok := err.(settingPolicyError); ok {
		return err
	}
	return settingPolicyError{status: http.StatusBadRequest, message: err.Error()}
}
