package handlers

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const ollamaProviderSettingsKey = "ollamaProviderSettings"

const (
	ollamaProviderStateUnconfigured      = "unconfigured"
	ollamaProviderStateConfigured        = "configured"
	ollamaProviderStateMigrationRequired = "migration_required"
	ollamaProviderStateMigrationConflict = "migration_conflict"
	ollamaProviderStateInvalid           = "invalid_configuration"
)

type ollamaProviderConfiguration struct {
	Endpoint              string
	SharedEndpoint        string
	LegacyEndpoint        string
	LegacyEndpointPresent bool
	State                 string
}

func normalizeOllamaProviderSettingsValue(value models.JSONMap) (models.JSONMap, error) {
	if value == nil {
		return nil, fmt.Errorf("Ollama provider settings must be an object")
	}
	endpoint, present := value["endpoint"]
	if !present {
		return nil, fmt.Errorf("endpoint is required")
	}
	endpointText, ok := endpoint.(string)
	if !ok {
		return nil, fmt.Errorf("endpoint must be a string")
	}
	endpointText = strings.TrimSpace(endpointText)
	if endpointText != "" {
		if err := validateOllamaEndpoint(endpointText); err != nil {
			return nil, err
		}
	}

	normalized := cloneSettingJSONMap(value)
	delete(normalized, "state")
	delete(normalized, "migrationState")
	delete(normalized, "legacyEndpoint")
	normalized["endpoint"] = endpointText
	return normalized, nil
}

func ollamaProviderSettingsForUpdate(db *gorm.DB, value models.JSONMap) (models.JSONMap, error) {
	merged := make(models.JSONMap)
	if db != nil {
		var current models.AppSetting
		err := db.First(&current, "key = ?", ollamaProviderSettingsKey).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		if err == nil {
			merged = cloneSettingJSONMap(current.Value)
		}
	}
	for key, item := range value {
		merged[key] = item
	}
	return normalizeOllamaProviderSettingsValue(merged)
}

func validateOllamaEndpoint(endpoint string) error {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(endpoint))
	if err != nil || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("endpoint must be an http or https URL without credentials")
	}
	return nil
}

func settingEndpointInput(value models.JSONMap, key string) (string, bool, error) {
	raw, present := value[key]
	if !present {
		return "", false, nil
	}
	endpoint, ok := raw.(string)
	if !ok {
		return "", true, fmt.Errorf("%s must be a string", key)
	}
	endpoint = strings.TrimSpace(endpoint)
	if endpoint != "" {
		if err := validateOllamaEndpoint(endpoint); err != nil {
			return "", true, fmt.Errorf("%s %w", key, err)
		}
	}
	return endpoint, true, nil
}

func cloneSettingJSONMap(value models.JSONMap) models.JSONMap {
	cloned := make(models.JSONMap, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

// MigrateOllamaProviderSettings moves the legacy Translation endpoint into the
// shared provider setting in one transaction. Conflicts and invalid legacy
// values are retained for explicit resolution and are reported by the resolver.
func MigrateOllamaProviderSettings(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}
	return db.Transaction(migrateOllamaProviderSettings)
}

func migrateOllamaProviderSettings(tx *gorm.DB) error {
	var legacy models.AppSetting
	legacyErr := tx.First(&legacy, "key = ?", translationSettingsKey).Error
	if legacyErr != nil && !errors.Is(legacyErr, gorm.ErrRecordNotFound) {
		return legacyErr
	}
	if errors.Is(legacyErr, gorm.ErrRecordNotFound) {
		return nil
	}
	legacyEndpoint, legacyPresent := legacy.Value["ollamaEndpoint"]
	if !legacyPresent {
		return nil
	}
	legacyText, ok := legacyEndpoint.(string)
	if !ok {
		return nil
	}
	legacyText = strings.TrimSpace(legacyText)
	if legacyText != "" && validateOllamaEndpoint(legacyText) != nil {
		return nil
	}

	var shared models.AppSetting
	sharedErr := tx.First(&shared, "key = ?", ollamaProviderSettingsKey).Error
	if sharedErr != nil && !errors.Is(sharedErr, gorm.ErrRecordNotFound) {
		return sharedErr
	}
	sharedExists := !errors.Is(sharedErr, gorm.ErrRecordNotFound)
	if sharedExists {
		sharedRaw, present := shared.Value["endpoint"]
		sharedText, valid := sharedRaw.(string)
		if !present || !valid {
			return nil
		}
		sharedText = strings.TrimSpace(sharedText)
		if sharedText != "" && validateOllamaEndpoint(sharedText) != nil {
			return nil
		}
		if legacyText != "" && sharedText != legacyText {
			return nil
		}
		if legacyText == "" && sharedText != "" {
			// An empty legacy projection carries no competing endpoint.
		}
	} else if legacyText != "" {
		shared = models.AppSetting{
			Key:       ollamaProviderSettingsKey,
			Value:     models.JSONMap{"endpoint": legacyText},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&shared).Error; err != nil {
			return fmt.Errorf("migrate shared Ollama endpoint: %w", err)
		}
	}

	updatedLegacy := cloneSettingJSONMap(legacy.Value)
	delete(updatedLegacy, "ollamaEndpoint")
	if err := tx.Model(&models.AppSetting{}).
		Where("key = ?", translationSettingsKey).
		Updates(map[string]any{"value": updatedLegacy, "updated_at": time.Now()}).Error; err != nil {
		return fmt.Errorf("remove migrated Translation endpoint: %w", err)
	}
	return nil
}

func loadOllamaProviderConfiguration(db *gorm.DB) (ollamaProviderConfiguration, error) {
	configuration := ollamaProviderConfiguration{State: ollamaProviderStateUnconfigured}
	if db == nil {
		configuration.State = ollamaProviderStateInvalid
		return configuration, fmt.Errorf("database is required")
	}

	var shared models.AppSetting
	sharedErr := db.First(&shared, "key = ?", ollamaProviderSettingsKey).Error
	if sharedErr != nil && !errors.Is(sharedErr, gorm.ErrRecordNotFound) {
		return configuration, sharedErr
	}
	sharedExists := !errors.Is(sharedErr, gorm.ErrRecordNotFound)
	if sharedExists {
		sharedRaw, present := shared.Value["endpoint"]
		sharedText, valid := sharedRaw.(string)
		if !present || !valid {
			configuration.State = ollamaProviderStateInvalid
			return configuration, nil
		}
		configuration.SharedEndpoint = strings.TrimSpace(sharedText)
		if configuration.SharedEndpoint != "" && validateOllamaEndpoint(configuration.SharedEndpoint) != nil {
			configuration.State = ollamaProviderStateInvalid
			return configuration, nil
		}
	}

	var legacy models.AppSetting
	legacyErr := db.First(&legacy, "key = ?", translationSettingsKey).Error
	if legacyErr != nil && !errors.Is(legacyErr, gorm.ErrRecordNotFound) {
		return configuration, legacyErr
	}
	if !errors.Is(legacyErr, gorm.ErrRecordNotFound) {
		legacyRaw, present := legacy.Value["ollamaEndpoint"]
		configuration.LegacyEndpointPresent = present
		if present {
			legacyText, ok := legacyRaw.(string)
			if !ok {
				configuration.State = ollamaProviderStateInvalid
				return configuration, nil
			}
			configuration.LegacyEndpoint = strings.TrimSpace(legacyText)
			if configuration.LegacyEndpoint != "" && validateOllamaEndpoint(configuration.LegacyEndpoint) != nil {
				configuration.State = ollamaProviderStateInvalid
				return configuration, nil
			}
		}
	}

	if !sharedExists {
		if configuration.LegacyEndpoint != "" {
			configuration.State = ollamaProviderStateMigrationRequired
		}
		return configuration, nil
	}
	if configuration.LegacyEndpoint != "" && configuration.SharedEndpoint != configuration.LegacyEndpoint {
		configuration.State = ollamaProviderStateMigrationConflict
		return configuration, nil
	}
	if configuration.SharedEndpoint == "" {
		if configuration.LegacyEndpoint != "" {
			configuration.State = ollamaProviderStateMigrationConflict
		}
		return configuration, nil
	}
	configuration.Endpoint = configuration.SharedEndpoint
	configuration.State = ollamaProviderStateConfigured
	return configuration, nil
}

func ollamaProviderSettingsProjection(db *gorm.DB, value models.JSONMap) (models.JSONMap, error) {
	projection := cloneSettingJSONMap(value)
	configuration, err := loadOllamaProviderConfiguration(db)
	if err != nil {
		return nil, err
	}
	projection["endpoint"] = configuration.SharedEndpoint
	projection["state"] = configuration.State
	return projection, nil
}

func resolveLegacyTranslationEndpoint(tx *gorm.DB, previousEndpoint, endpoint string) error {
	var translation models.AppSetting
	if err := tx.First(&translation, "key = ?", translationSettingsKey).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	updated := cloneSettingJSONMap(translation.Value)
	_, hasLegacyEndpoint := updated["ollamaEndpoint"]
	if previousEndpoint == endpoint && !hasLegacyEndpoint {
		return nil
	}
	candidate := cloneSettingJSONMap(updated)
	candidate["ollamaEndpoint"] = endpoint
	settings, _ := translationSettingsFromValue(candidate, false)
	storedIdentity, _ := optionalStringSettingValue(updated, translationSettingsReadinessIdentityKey)
	targetIdentity := translationProviderConfigurationIdentity(settings)
	delete(updated, "ollamaEndpoint")
	if storedIdentity != targetIdentity {
		provider, _ := optionalStringSettingValue(updated, "provider")
		model, _ := optionalStringSettingValue(updated, "ollamaModel")
		if strings.TrimSpace(endpoint) != "" && strings.TrimSpace(provider) == translationProviderOllama && strings.TrimSpace(model) != "" {
			updated[translationSettingsReadinessKey] = string(translationReadinessUntested)
		} else {
			updated[translationSettingsReadinessKey] = string(translationReadinessNotConfigured)
		}
		updated[translationSettingsReadinessIdentityKey] = ""
		updated[translationSettingsReadinessCheckedAtKey] = ""
		updated[translationSettingsReadinessDiagnosticKey] = ""
	}
	return tx.Model(&models.AppSetting{}).
		Where("key = ?", translationSettingsKey).
		Updates(map[string]any{"value": updated, "updated_at": time.Now()}).Error
}
