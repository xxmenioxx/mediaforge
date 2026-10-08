package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	translationSettingsEnabledKey             = "enabled"
	translationSettingsReadinessKey           = "readiness"
	translationSettingsReadinessIdentityKey   = "readinessConfigIdentity"
	translationSettingsReadinessCheckedAtKey  = "readinessCheckedAt"
	translationSettingsReadinessDiagnosticKey = "readinessDiagnostic"

	translationReadinessDiagnosticMaxLength = 512
)

type translationProviderReadinessState string

const (
	translationReadinessDisabled             translationProviderReadinessState = "disabled"
	translationReadinessNotConfigured        translationProviderReadinessState = "not_configured"
	translationReadinessUntested             translationProviderReadinessState = "untested"
	translationReadinessReady                translationProviderReadinessState = "ready"
	translationReadinessUnavailable          translationProviderReadinessState = "unavailable"
	translationReadinessModelNotFound        translationProviderReadinessState = "model_not_found"
	translationReadinessInvalidConfiguration translationProviderReadinessState = "invalid_configuration"
)

type TranslationProviderReadiness struct {
	State        translationProviderReadinessState
	Diagnostic   string
	FailureState string
}

type TranslationProviderReadinessChecker interface {
	TestConnection(context.Context) TranslationProviderReadiness
}

func validTranslationReadinessState(state translationProviderReadinessState) bool {
	switch state {
	case translationReadinessDisabled,
		translationReadinessNotConfigured,
		translationReadinessUntested,
		translationReadinessReady,
		translationReadinessUnavailable,
		translationReadinessModelNotFound,
		translationReadinessInvalidConfiguration:
		return true
	default:
		return false
	}
}

func translationSettingsConfigured(settings translationSettings) bool {
	return settings.Provider == translationProviderOllama &&
		strings.TrimSpace(settings.OllamaEndpoint) != "" &&
		strings.TrimSpace(settings.OllamaModel) != ""
}

func translationProviderConfigurationIdentity(settings translationSettings) string {
	identity := strings.Join([]string{
		strings.TrimSpace(settings.Provider),
		strings.TrimSpace(settings.OllamaEndpoint),
		strings.TrimSpace(settings.OllamaModel),
	}, "\x00")
	digest := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(digest[:])
}

func translationProviderReadinessCheckerForSettings(settings translationSettings) (TranslationProviderReadinessChecker, error) {
	switch provider := strings.TrimSpace(settings.Provider); provider {
	case "":
		return nil, translationProviderConfigurationError("translation provider is not configured")
	case translationProviderOllama:
		checker, err := newOllamaTranslationProvider(settings)
		if err != nil {
			return nil, translationProviderConfigurationError("invalid Ollama configuration: %v", err)
		}
		return checker, nil
	case translationProviderOpenAI, translationProviderDeepL, translationProviderGoogle, translationProviderAzure:
		return nil, translationProviderConfigurationError("translation provider %q is not implemented", provider)
	default:
		return nil, translationProviderConfigurationError("unknown translation provider %q", provider)
	}
}

func boundedTranslationReadinessDiagnostic(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > translationReadinessDiagnosticMaxLength {
		return value[:translationReadinessDiagnosticMaxLength]
	}
	return value
}

func translationReadinessForConfigurationError(err error) TranslationProviderReadiness {
	return TranslationProviderReadiness{
		State:      translationReadinessInvalidConfiguration,
		Diagnostic: boundedTranslationReadinessDiagnostic(err.Error()),
	}
}

func (h SettingsHandler) TestTranslationConnection(c *gin.Context) {
	if err := MigrateOllamaProviderSettings(h.db); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	settings := loadTranslationSettings(h.db)
	if settings.ConfigurationState == ollamaProviderStateMigrationConflict || settings.ConfigurationState == ollamaProviderStateInvalid || settings.ConfigurationState == ollamaProviderStateMigrationRequired {
		settings.Readiness = translationReadinessInvalidConfiguration
		settings.ReadinessDiagnostic = "Ollama provider endpoint migration must be resolved before testing Translation"
		c.JSON(http.StatusConflict, translationReadinessResponse(settings))
		return
	}
	if !translationSettingsConfigured(settings) {
		settings.Enabled = false
		settings.Readiness = translationReadinessNotConfigured
		settings.ReadinessIdentity = ""
		settings.ReadinessCheckedAt = ""
		settings.ReadinessDiagnostic = "translation provider endpoint and model are required"
		if err := saveTranslationSettings(h.db, settings); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(400, translationReadinessResponse(settings))
		return
	}
	checker, err := translationProviderReadinessCheckerForSettings(settings)
	if err != nil {
		result := translationReadinessForConfigurationError(err)
		applyTranslationReadiness(&settings, result, time.Now().UTC())
		if saveErr := saveTranslationSettings(h.db, settings); saveErr != nil {
			c.JSON(500, gin.H{"error": saveErr.Error()})
			return
		}
		c.JSON(400, translationReadinessResponse(settings))
		return
	}
	wasEnabled := settings.Enabled && settings.Readiness == translationReadinessReady &&
		settings.ReadinessIdentity == translationProviderConfigurationIdentity(settings)
	result := checker.TestConnection(c.Request.Context())
	applyTranslationReadiness(&settings, result, time.Now().UTC())
	settings.Enabled = wasEnabled && result.State == translationReadinessReady
	if err := saveTranslationSettings(h.db, settings); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, translationReadinessResponse(settings))
}

func (h SettingsHandler) EnableTranslation(c *gin.Context) {
	if err := MigrateOllamaProviderSettings(h.db); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	settings := loadTranslationSettings(h.db)
	if settings.ConfigurationState == ollamaProviderStateMigrationConflict || settings.ConfigurationState == ollamaProviderStateInvalid || settings.ConfigurationState == ollamaProviderStateMigrationRequired {
		settings.Readiness = translationReadinessInvalidConfiguration
		settings.ReadinessDiagnostic = "Ollama provider endpoint migration must be resolved before enabling Translation"
		c.JSON(http.StatusConflict, translationReadinessResponse(settings))
		return
	}
	if !translationSettingsConfigured(settings) {
		settings.Enabled = false
		settings.Readiness = translationReadinessNotConfigured
		settings.ReadinessIdentity = ""
		settings.ReadinessCheckedAt = ""
		settings.ReadinessDiagnostic = "translation provider endpoint and model are required"
		if err := saveTranslationSettings(h.db, settings); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(400, translationReadinessResponse(settings))
		return
	}
	checker, err := translationProviderReadinessCheckerForSettings(settings)
	if err != nil {
		result := translationReadinessForConfigurationError(err)
		applyTranslationReadiness(&settings, result, time.Now().UTC())
		settings.Enabled = false
		if saveErr := saveTranslationSettings(h.db, settings); saveErr != nil {
			c.JSON(500, gin.H{"error": saveErr.Error()})
			return
		}
		c.JSON(400, translationReadinessResponse(settings))
		return
	}
	result := checker.TestConnection(c.Request.Context())
	applyTranslationReadiness(&settings, result, time.Now().UTC())
	settings.Enabled = result.State == translationReadinessReady
	if err := saveTranslationSettings(h.db, settings); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if !settings.Enabled {
		c.JSON(409, translationReadinessResponse(settings))
		return
	}
	c.JSON(200, translationReadinessResponse(settings))
}

type translationReadinessResponseBody struct {
	Configured bool   `json:"configured"`
	Enabled    bool   `json:"enabled"`
	Readiness  string `json:"readiness"`
	CheckedAt  string `json:"lastCheckedAt,omitempty"`
	Diagnostic string `json:"diagnostic,omitempty"`
}

func translationReadinessResponse(settings translationSettings) translationReadinessResponseBody {
	return translationReadinessResponseBody{
		Configured: translationSettingsConfigured(settings),
		Enabled:    settings.Enabled,
		Readiness:  string(settings.Readiness),
		CheckedAt:  settings.ReadinessCheckedAt,
		Diagnostic: boundedTranslationReadinessDiagnostic(settings.ReadinessDiagnostic),
	}
}

func applyTranslationReadiness(settings *translationSettings, result TranslationProviderReadiness, checkedAt time.Time) {
	if settings == nil {
		return
	}
	if !validTranslationReadinessState(result.State) || result.State == translationReadinessDisabled {
		result.State = translationReadinessInvalidConfiguration
	}
	settings.Readiness = result.State
	settings.ReadinessIdentity = translationProviderConfigurationIdentity(*settings)
	settings.ReadinessCheckedAt = checkedAt.UTC().Format(time.RFC3339Nano)
	settings.ReadinessDiagnostic = boundedTranslationReadinessDiagnostic(result.Diagnostic)
}
