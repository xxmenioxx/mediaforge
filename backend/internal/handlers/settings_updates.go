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
	responseCount := len(inputs)
	updated := make([]models.AppSetting, 0, responseCount)
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := migrateOllamaProviderSettings(tx); err != nil {
			return err
		}
		inputs = append([]SettingInput(nil), inputs...)
		seenInputs := make(map[string]struct{}, len(inputs))
		providerIndex, translationIndex := -1, -1
		for index, input := range inputs {
			if input.Key == "" {
				return settingPolicyError{status: http.StatusBadRequest, message: "setting key is required"}
			}
			if _, exists := seenInputs[input.Key]; exists {
				return settingPolicyError{status: http.StatusBadRequest, message: fmt.Sprintf("duplicate setting key %q", input.Key)}
			}
			seenInputs[input.Key] = struct{}{}
			if input.Key == ollamaProviderSettingsKey {
				providerIndex = index
			}
			if input.Key == translationSettingsKey {
				translationIndex = index
			}
		}
		configurationBefore, err := loadOllamaProviderConfiguration(tx)
		if err != nil {
			return err
		}
		providerEndpoint, providerEndpointPresent := "", false
		if providerIndex >= 0 {
			if inputs[providerIndex].Value == nil {
				return settingPolicyError{status: http.StatusBadRequest, message: fmt.Sprintf("setting %q value must be an object", ollamaProviderSettingsKey)}
			}
			providerEndpoint, providerEndpointPresent, err = settingEndpointInput(inputs[providerIndex].Value, "endpoint")
			if err != nil {
				return asSettingPolicyError(err)
			}
		}
		legacyEndpoint, legacyEndpointPresent := "", false
		if translationIndex >= 0 && inputs[translationIndex].Value != nil {
			legacyEndpoint, legacyEndpointPresent, err = settingEndpointInput(inputs[translationIndex].Value, "ollamaEndpoint")
			if err != nil {
				return asSettingPolicyError(err)
			}
		}
		if configurationBefore.State == ollamaProviderStateMigrationConflict && translationIndex >= 0 && !legacyEndpointPresent && providerIndex < 0 {
			return settingPolicyError{status: http.StatusConflict, message: "Ollama endpoint migration conflict must be resolved before updating Translation settings"}
		}
		if legacyEndpointPresent {
			if providerIndex >= 0 {
				if !providerEndpointPresent {
					return settingPolicyError{status: http.StatusBadRequest, message: "endpoint is required"}
				}
				if providerEndpoint != legacyEndpoint {
					return settingPolicyError{status: http.StatusConflict, message: "Translation and shared Ollama endpoints conflict in one update"}
				}
			} else {
				sharedValue := models.JSONMap{"endpoint": legacyEndpoint}
				var shared models.AppSetting
				if err := tx.First(&shared, "key = ?", ollamaProviderSettingsKey).Error; err == nil {
					sharedValue = cloneSettingJSONMap(shared.Value)
					sharedValue["endpoint"] = legacyEndpoint
				} else if err != nil && err != gorm.ErrRecordNotFound {
					return err
				}
				providerEndpoint, providerEndpointPresent = legacyEndpoint, true
				inputs = append(inputs, SettingInput{Key: ollamaProviderSettingsKey, Value: sharedValue})
				providerIndex = len(inputs) - 1
			}
		}
		if configurationBefore.State == ollamaProviderStateMigrationConflict && providerIndex >= 0 && !providerEndpointPresent {
			return settingPolicyError{status: http.StatusBadRequest, message: "endpoint is required"}
		}

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
		if providerEndpointPresent {
			if err := resolveLegacyTranslationEndpoint(tx, configurationBefore.SharedEndpoint, providerEndpoint); err != nil {
				return fmt.Errorf("resolve legacy Translation endpoint: %w", err)
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
			if index < responseCount {
				updated = append(updated, setting)
			}
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
