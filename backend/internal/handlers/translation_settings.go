package handlers

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
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
	Provider          string
	OllamaEndpoint    string
	OllamaModel       string
	MaxCuesPerRequest int
	MaxRequestBytes   int
	MaxContextCues    int
	MaxContextChars   int
}

func defaultTranslationSettings() translationSettings {
	return translationSettings{
		MaxCuesPerRequest: translationDefaultMaxCuesPerRequest,
		MaxRequestBytes:   translationDefaultMaxRequestBytes,
		MaxContextCues:    translationDefaultMaxContextCues,
		MaxContextChars:   translationDefaultMaxContextChars,
	}
}

func normalizeTranslationSettingsValue(value models.JSONMap) (models.JSONMap, error) {
	settings, err := translationSettingsFromValue(value, true)
	if err != nil {
		return nil, err
	}
	return translationSettingsValue(settings), nil
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
	settings, err := translationSettingsFromValue(setting.Value, false)
	if err != nil {
		return defaults
	}
	return settings
}

func translationSettingsFromValue(value models.JSONMap, strict bool) (translationSettings, error) {
	settings := defaultTranslationSettings()
	provider := stringSettingValue(value, "provider")
	if provider != "" && provider != translationProviderOllama {
		if strict {
			return translationSettings{}, fmt.Errorf("provider must be %q or empty", translationProviderOllama)
		}
		return settings, nil
	}
	settings.Provider = provider
	settings.OllamaEndpoint = stringSettingValue(value, "ollamaEndpoint")
	settings.OllamaModel = stringSettingValue(value, "ollamaModel")
	if settings.OllamaEndpoint != "" {
		parsed, err := url.ParseRequestURI(settings.OllamaEndpoint)
		if err != nil || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			if strict {
				return translationSettings{}, fmt.Errorf("ollamaEndpoint must be an http or https URL without credentials")
			}
			return settings, nil
		}
	}
	if settings.OllamaModel != "" && len(settings.OllamaModel) > 256 {
		if strict {
			return translationSettings{}, fmt.Errorf("ollamaModel exceeds 256 characters")
		}
		return settings, nil
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
			return settings, nil
		}
		*field.dest = parsed
	}
	return settings, nil
}

func translationSettingsValue(settings translationSettings) models.JSONMap {
	return models.JSONMap{
		"provider":          settings.Provider,
		"ollamaEndpoint":    settings.OllamaEndpoint,
		"ollamaModel":       settings.OllamaModel,
		"maxCuesPerRequest": settings.MaxCuesPerRequest,
		"maxRequestBytes":   settings.MaxRequestBytes,
		"maxContextCues":    settings.MaxContextCues,
		"maxContextChars":   settings.MaxContextChars,
	}
}

func stringSettingValue(value models.JSONMap, key string) string {
	text, _ := value[key].(string)
	return strings.TrimSpace(text)
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
