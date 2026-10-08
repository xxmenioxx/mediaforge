package handlers

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const translationSettingsKey = "subtitleTranslationSettings"

const (
	translationProviderOllama = "ollama"

	translationDefaultMaxCuesPerRequest = 32
	translationDefaultMaxRequestBytes   = 64 * 1024
	translationDefaultMaxContextCues    = 8
	translationDefaultMaxContextChars   = 4096
)

type translationSettings struct {
	Provider            string
	OllamaEndpoint      string
	ConfigurationState  string
	OllamaModel         string
	MaxCuesPerRequest   int
	MaxRequestBytes     int
	MaxContextCues      int
	MaxContextChars     int
	Enabled             bool
	Readiness           translationProviderReadinessState
	ReadinessIdentity   string
	ReadinessCheckedAt  string
	ReadinessDiagnostic string
	Extra               models.JSONMap
}

func defaultTranslationSettings() translationSettings {
	return translationSettings{
		MaxCuesPerRequest: translationDefaultMaxCuesPerRequest,
		MaxRequestBytes:   translationDefaultMaxRequestBytes,
		MaxContextCues:    translationDefaultMaxContextCues,
		MaxContextChars:   translationDefaultMaxContextChars,
		Readiness:         translationReadinessNotConfigured,
	}
}

func normalizeTranslationSettingsValue(value models.JSONMap) (models.JSONMap, error) {
	if enabled, ok := value[translationSettingsEnabledKey].(bool); ok && enabled {
		return nil, fmt.Errorf("translation enablement must use the readiness-validated enable operation")
	}
	settings, err := translationSettingsFromValue(value, true)
	if err != nil {
		return nil, err
	}
	return translationSettingsValue(settings), nil
}

func translationSettingsForUpdate(db *gorm.DB, value models.JSONMap) (models.JSONMap, error) {
	previous := loadTranslationSettings(db)
	incomingValue := cloneSettingJSONMap(previous.Extra)
	for key, item := range value {
		incomingValue[key] = item
	}
	if _, hasEndpoint := incomingValue["ollamaEndpoint"]; !hasEndpoint {
		configuration, err := loadOllamaProviderConfiguration(db)
		if err != nil {
			return nil, err
		}
		incomingValue["ollamaEndpoint"] = configuration.Endpoint
	}
	normalized, err := normalizeTranslationSettingsValue(incomingValue)
	if err != nil {
		return nil, err
	}
	normalized["ollamaEndpoint"] = incomingValue["ollamaEndpoint"]
	incoming, err := translationSettingsFromValue(normalized, true)
	if err != nil {
		return nil, err
	}
	if translationProviderConfigurationIdentity(previous) == translationProviderConfigurationIdentity(incoming) &&
		translationSettingsConfigured(previous) == translationSettingsConfigured(incoming) {
		incoming.Enabled = previous.Enabled
		incoming.Readiness = previous.Readiness
		incoming.ReadinessIdentity = previous.ReadinessIdentity
		incoming.ReadinessCheckedAt = previous.ReadinessCheckedAt
		incoming.ReadinessDiagnostic = previous.ReadinessDiagnostic
		if enabled, present := value[translationSettingsEnabledKey]; present {
			if disabled, ok := enabled.(bool); ok && !disabled {
				incoming.Enabled = false
			}
		}
		return translationSettingsValue(incoming), nil
	}
	incoming.Enabled = false
	incoming.ReadinessIdentity = ""
	incoming.ReadinessCheckedAt = ""
	incoming.ReadinessDiagnostic = ""
	if translationSettingsConfigured(incoming) {
		incoming.Readiness = translationReadinessUntested
	} else {
		incoming.Readiness = translationReadinessNotConfigured
	}
	return translationSettingsValue(incoming), nil
}

func loadTranslationSettings(db *gorm.DB) translationSettings {
	defaults := defaultTranslationSettings()
	if db == nil {
		return defaults
	}
	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", translationSettingsKey).Error; err != nil {
		return defaults
	}
	configuration, err := loadOllamaProviderConfiguration(db)
	if err != nil {
		settings, parseErr := translationSettingsFromValue(setting.Value, false)
		if parseErr != nil {
			return defaults
		}
		settings.ConfigurationState = ollamaProviderStateInvalid
		settings.OllamaEndpoint = ""
		return settings
	}
	storedValue := cloneSettingJSONMap(setting.Value)
	if configuration.State == ollamaProviderStateConfigured {
		storedValue["ollamaEndpoint"] = configuration.Endpoint
	}
	settings, err := translationSettingsFromValue(storedValue, false)
	if err != nil {
		return defaults
	}
	settings.ConfigurationState = configuration.State
	settings.OllamaEndpoint = configuration.Endpoint
	if configuration.State != ollamaProviderStateConfigured || !translationSettingsUsable(settings) {
		return settings
	}
	return normalizeLoadedTranslationSettings(settings)
}

func translationSettingsFromValue(value models.JSONMap, strict bool) (translationSettings, error) {
	settings := defaultTranslationSettings()
	settings.Extra = translationSettingsExtraFields(value)
	if raw, present := value[translationSettingsEnabledKey]; present {
		enabled, ok := raw.(bool)
		if !ok {
			if strict {
				return translationSettings{}, fmt.Errorf("enabled must be a boolean")
			}
			return defaultTranslationSettings(), nil
		}
		settings.Enabled = enabled
	}
	provider, valid := optionalStringSettingValue(value, "provider")
	if !valid {
		if strict {
			return translationSettings{}, fmt.Errorf("provider must be a string")
		}
		return settings, nil
	}
	if provider != "" && provider != translationProviderOllama {
		if strict {
			return translationSettings{}, fmt.Errorf("provider must be %q or empty", translationProviderOllama)
		}
		return settings, nil
	}
	settings.Provider = provider
	settings.OllamaEndpoint, valid = optionalStringSettingValue(value, "ollamaEndpoint")
	if !valid {
		if strict {
			return translationSettings{}, fmt.Errorf("ollamaEndpoint must be a string")
		}
		return settings, nil
	}
	settings.OllamaModel, valid = optionalStringSettingValue(value, "ollamaModel")
	if !valid {
		if strict {
			return translationSettings{}, fmt.Errorf("ollamaModel must be a string")
		}
		return settings, nil
	}
	if raw, present := value[translationSettingsReadinessKey]; present {
		readiness, ok := raw.(string)
		readiness = strings.TrimSpace(readiness)
		if !ok || !validTranslationReadinessState(translationProviderReadinessState(readiness)) {
			if strict {
				return translationSettings{}, fmt.Errorf("readiness is invalid")
			}
			return defaultTranslationSettings(), nil
		}
		settings.Readiness = translationProviderReadinessState(readiness)
	} else if settings.Provider != "" && settings.OllamaEndpoint != "" && settings.OllamaModel != "" {
		settings.Readiness = translationReadinessUntested
	}
	for key, destination := range map[string]*string{
		translationSettingsReadinessIdentityKey:   &settings.ReadinessIdentity,
		translationSettingsReadinessCheckedAtKey:  &settings.ReadinessCheckedAt,
		translationSettingsReadinessDiagnosticKey: &settings.ReadinessDiagnostic,
	} {
		raw, present := value[key]
		if !present {
			continue
		}
		text, ok := raw.(string)
		if !ok || len(text) > maximumTranslationSettingsStringLength(key) {
			if strict {
				return translationSettings{}, fmt.Errorf("%s is invalid", key)
			}
			return defaultTranslationSettings(), nil
		}
		*destination = strings.TrimSpace(text)
	}
	if settings.OllamaEndpoint != "" {
		parsed, err := url.ParseRequestURI(settings.OllamaEndpoint)
		if err != nil || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			if strict {
				return translationSettings{}, fmt.Errorf("ollamaEndpoint must be an http or https URL without credentials")
			}
			return defaultTranslationSettings(), nil
		}
	}
	if settings.OllamaModel != "" && len(settings.OllamaModel) > 256 {
		if strict {
			return translationSettings{}, fmt.Errorf("ollamaModel exceeds 256 characters")
		}
		return defaultTranslationSettings(), nil
	}

	fields := []struct {
		key     string
		minimum int
		maximum int
		dest    *int
	}{
		{key: "maxCuesPerRequest", minimum: 1, maximum: 128, dest: &settings.MaxCuesPerRequest},
		{key: "maxRequestBytes", minimum: 1024, maximum: 1024 * 1024, dest: &settings.MaxRequestBytes},
		{key: "maxContextCues", minimum: 0, maximum: 32, dest: &settings.MaxContextCues},
		{key: "maxContextChars", minimum: 0, maximum: 16 * 1024, dest: &settings.MaxContextChars},
	}
	for _, field := range fields {
		raw, present := value[field.key]
		if !present {
			continue
		}
		parsed, valid := integerSettingValue(raw)
		if !valid || parsed < field.minimum || parsed > field.maximum {
			if strict {
				return translationSettings{}, fmt.Errorf("%s must be an integer between %d and %d", field.key, field.minimum, field.maximum)
			}
			return defaultTranslationSettings(), nil
		}
		*field.dest = parsed
	}
	if !translationSettingsConfigured(settings) {
		settings.Enabled = false
		settings.Readiness = translationReadinessNotConfigured
		settings.ReadinessIdentity = ""
		settings.ReadinessCheckedAt = ""
		settings.ReadinessDiagnostic = ""
	}
	return settings, nil
}

func translationSettingsValue(settings translationSettings) models.JSONMap {
	value := cloneSettingJSONMap(settings.Extra)
	knownValues := models.JSONMap{
		"provider":                                settings.Provider,
		"ollamaModel":                             settings.OllamaModel,
		"maxCuesPerRequest":                       settings.MaxCuesPerRequest,
		"maxRequestBytes":                         settings.MaxRequestBytes,
		"maxContextCues":                          settings.MaxContextCues,
		"maxContextChars":                         settings.MaxContextChars,
		translationSettingsEnabledKey:             settings.Enabled,
		translationSettingsReadinessKey:           string(settings.Readiness),
		translationSettingsReadinessIdentityKey:   settings.ReadinessIdentity,
		translationSettingsReadinessCheckedAtKey:  settings.ReadinessCheckedAt,
		translationSettingsReadinessDiagnosticKey: settings.ReadinessDiagnostic,
	}
	for key, item := range knownValues {
		value[key] = item
	}
	delete(value, "ollamaEndpoint")
	delete(value, "ollamaProviderState")
	return value
}

func translationSettingsExtraFields(value models.JSONMap) models.JSONMap {
	known := map[string]struct{}{
		"provider": {}, "ollamaEndpoint": {}, "ollamaModel": {},
		"maxCuesPerRequest": {}, "maxRequestBytes": {}, "maxContextCues": {}, "maxContextChars": {},
		translationSettingsEnabledKey: {}, translationSettingsReadinessKey: {},
		translationSettingsReadinessIdentityKey: {}, translationSettingsReadinessCheckedAtKey: {},
		translationSettingsReadinessDiagnosticKey: {}, "ollamaProviderState": {},
	}
	extra := make(models.JSONMap)
	for key, item := range value {
		if _, isKnown := known[key]; !isKnown {
			extra[key] = item
		}
	}
	return extra
}

func translationSettingsProjection(db *gorm.DB, value models.JSONMap) (models.JSONMap, error) {
	configuration, err := loadOllamaProviderConfiguration(db)
	if err != nil {
		return nil, err
	}
	projectedValue := cloneSettingJSONMap(value)
	if configuration.SharedEndpoint == "" || validateOllamaEndpoint(configuration.SharedEndpoint) == nil {
		projectedValue["ollamaEndpoint"] = configuration.SharedEndpoint
	}
	settings, err := translationSettingsFromValue(projectedValue, false)
	if err != nil {
		return nil, err
	}
	settings.ConfigurationState = configuration.State
	settings.OllamaEndpoint = configuration.Endpoint
	if configuration.State == ollamaProviderStateMigrationConflict || configuration.State == ollamaProviderStateInvalid || configuration.State == ollamaProviderStateMigrationRequired {
		settings.Enabled = false
		settings.Readiness = translationReadinessInvalidConfiguration
		settings.ReadinessIdentity = ""
		settings.ReadinessCheckedAt = ""
		settings.ReadinessDiagnostic = "Ollama provider configuration must be resolved before Translation can run"
	}
	projection := translationSettingsValue(settings)
	projection["ollamaEndpoint"] = configuration.SharedEndpoint
	projection["ollamaProviderState"] = configuration.State
	return projection, nil
}

func optionalStringSettingValue(value models.JSONMap, key string) (string, bool) {
	raw, present := value[key]
	if !present {
		return "", true
	}
	text, ok := raw.(string)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(text), true
}

func translationSettingsUsable(settings translationSettings) bool {
	return settings.Provider == translationProviderOllama && settings.OllamaEndpoint != "" && settings.OllamaModel != ""
}

func normalizeLoadedTranslationSettings(settings translationSettings) translationSettings {
	if !translationSettingsConfigured(settings) {
		return defaultTranslationSettings()
	}
	if settings.Readiness == translationReadinessReady && settings.ReadinessIdentity != translationProviderConfigurationIdentity(settings) {
		settings.Enabled = false
		settings.Readiness = translationReadinessUntested
		settings.ReadinessIdentity = ""
		settings.ReadinessCheckedAt = ""
		settings.ReadinessDiagnostic = ""
	}
	if settings.Readiness != translationReadinessReady {
		settings.Enabled = false
	}
	return settings
}

func maximumTranslationSettingsStringLength(key string) int {
	switch key {
	case translationSettingsReadinessIdentityKey:
		return 128
	case translationSettingsReadinessCheckedAtKey:
		return 64
	case translationSettingsReadinessDiagnosticKey:
		return 512
	default:
		return 0
	}
}

func saveTranslationSettings(db *gorm.DB, settings translationSettings) error {
	if db == nil {
		return fmt.Errorf("settings database is unavailable")
	}
	now := time.Now()
	setting := models.AppSetting{Key: translationSettingsKey, Value: translationSettingsValue(settings), CreatedAt: now, UpdatedAt: now}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&setting).Error
}

func integerSettingValue(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		if typed != float64(int(typed)) {
			return 0, false
		}
		return int(typed), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		return parsed, err == nil
	default:
		return 0, false
	}
}
