package airesource

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

const SettingsKey = "aiResourceProtectionSettings"

const gibibyte int64 = 1 << 30

var supportedMetrics = map[string]struct{}{
	"cpuLoadPerCore":      {},
	"gpuUsagePercent":     {},
	"swapPressurePercent": {},
	"zramPressurePercent": {},
}

type ModelEstimate struct {
	ProviderConfigurationID string `json:"providerConfigurationId"`
	ModelID                 string `json:"modelId"`
	ResidentBytes           int64  `json:"residentBytes"`
	LoadPeakAdditionalBytes int64  `json:"loadPeakAdditionalBytes"`
	InferenceBaseBytes      int64  `json:"inferenceBaseBytes"`
	KVCacheBytesPerToken    int64  `json:"kvCacheBytesPerToken"`
	DefaultContextTokens    int64  `json:"defaultContextTokens"`
	MaximumContextTokens    int64  `json:"maximumContextTokens"`
	Source                  string `json:"source"`
	Revision                string `json:"revision"`
}

type Settings struct {
	Enabled                     bool               `json:"enabled"`
	MaxResidentModels           int                `json:"maxResidentModels"`
	MaxConcurrentInferences     int                `json:"maxConcurrentInferences"`
	MaxConcurrentModelLoads     int                `json:"maxConcurrentModelLoads"`
	MinimumAvailableMemoryBytes int64              `json:"minimumAvailableMemoryBytes"`
	MaximumAdmissionWaitSeconds int                `json:"maximumAdmissionWaitSeconds"`
	TelemetryMaximumAgeSeconds  int                `json:"telemetryMaximumAgeSeconds"`
	LeaseCleanupGraceSeconds    int                `json:"leaseCleanupGraceSeconds"`
	PressureThresholds          map[string]float64 `json:"pressureThresholds,omitempty"`
	RequiredMetrics             []string           `json:"requiredMetrics,omitempty"`
	ModelEstimates              []ModelEstimate    `json:"modelEstimates"`
}

func DefaultSettings() Settings {
	return Settings{
		Enabled:                     true,
		MaxResidentModels:           1,
		MaxConcurrentInferences:     1,
		MaxConcurrentModelLoads:     1,
		MinimumAvailableMemoryBytes: 6 * gibibyte,
		MaximumAdmissionWaitSeconds: 300,
		TelemetryMaximumAgeSeconds:  60,
		LeaseCleanupGraceSeconds:    30,
		PressureThresholds:          map[string]float64{},
		RequiredMetrics:             []string{},
		ModelEstimates:              []ModelEstimate{},
	}
}

func DefaultSettingsValue() models.JSONMap {
	encoded, _ := json.Marshal(DefaultSettings())
	value := models.JSONMap{}
	_ = json.Unmarshal(encoded, &value)
	return value
}

func DecodeSettings(value models.JSONMap) (Settings, error) {
	if value == nil {
		return Settings{}, fmt.Errorf("resource protection settings must be an object")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return Settings{}, fmt.Errorf("encode resource protection settings: %w", err)
	}
	settings := DefaultSettings()
	if err := json.Unmarshal(encoded, &settings); err != nil {
		return Settings{}, fmt.Errorf("decode resource protection settings: %w", err)
	}
	if err := ValidateSettings(settings); err != nil {
		return Settings{}, err
	}
	return settings, nil
}

func NormalizeSettingsValue(value models.JSONMap) (models.JSONMap, error) {
	settings, err := DecodeSettings(value)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("encode normalized resource protection settings: %w", err)
	}
	result := models.JSONMap{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil, fmt.Errorf("decode normalized resource protection settings: %w", err)
	}
	return result, nil
}

func ValidateSettings(settings Settings) error {
	if settings.MaxResidentModels < 1 || settings.MaxResidentModels > 64 {
		return fmt.Errorf("maxResidentModels must be between 1 and 64")
	}
	if settings.MaxConcurrentInferences < 1 || settings.MaxConcurrentInferences > 256 {
		return fmt.Errorf("maxConcurrentInferences must be between 1 and 256")
	}
	if settings.MaxConcurrentModelLoads < 1 || settings.MaxConcurrentModelLoads > 64 {
		return fmt.Errorf("maxConcurrentModelLoads must be between 1 and 64")
	}
	if settings.MinimumAvailableMemoryBytes < 0 || settings.MinimumAvailableMemoryBytes > 1<<40 {
		return fmt.Errorf("minimumAvailableMemoryBytes must be between 0 and 1 TiB")
	}
	if settings.MaximumAdmissionWaitSeconds < 1 || settings.MaximumAdmissionWaitSeconds > 1800 {
		return fmt.Errorf("maximumAdmissionWaitSeconds must be between 1 and 1800")
	}
	if settings.TelemetryMaximumAgeSeconds < 1 || settings.TelemetryMaximumAgeSeconds > 300 {
		return fmt.Errorf("telemetryMaximumAgeSeconds must be between 1 and 300")
	}
	if settings.LeaseCleanupGraceSeconds < 0 || settings.LeaseCleanupGraceSeconds > 300 {
		return fmt.Errorf("leaseCleanupGraceSeconds must be between 0 and 300")
	}

	for metric, threshold := range settings.PressureThresholds {
		if _, ok := supportedMetrics[metric]; !ok {
			return fmt.Errorf("unsupported pressure metric %q", metric)
		}
		if math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 || threshold > 100 {
			return fmt.Errorf("pressure threshold %q must be between 0 and 100", metric)
		}
	}
	for _, metric := range settings.RequiredMetrics {
		if _, ok := supportedMetrics[metric]; !ok {
			return fmt.Errorf("unsupported required metric %q", metric)
		}
	}

	profiles := make(map[string]struct{}, len(settings.ModelEstimates))
	if len(settings.ModelEstimates) > 512 {
		return fmt.Errorf("modelEstimates must contain at most 512 profiles")
	}
	for _, profile := range settings.ModelEstimates {
		if strings.TrimSpace(profile.ProviderConfigurationID) == "" || strings.TrimSpace(profile.ModelID) == "" {
			return fmt.Errorf("model estimate requires providerConfigurationId and modelId")
		}
		key := profile.ProviderConfigurationID + "\x00" + profile.ModelID
		if _, exists := profiles[key]; exists {
			return fmt.Errorf("duplicate model estimate for provider configuration and model")
		}
		profiles[key] = struct{}{}
		if profile.ResidentBytes <= 0 || profile.LoadPeakAdditionalBytes < 0 || profile.InferenceBaseBytes < 0 || profile.KVCacheBytesPerToken <= 0 {
			return fmt.Errorf("model estimate memory values are invalid for %q", profile.ModelID)
		}
		if profile.DefaultContextTokens < 1 || profile.MaximumContextTokens < profile.DefaultContextTokens || profile.MaximumContextTokens > 1_000_000 {
			return fmt.Errorf("model estimate context range is invalid for %q", profile.ModelID)
		}
		if profile.Source != "measured" && profile.Source != "benchmark" && profile.Source != "operator" {
			return fmt.Errorf("model estimate source must be measured, benchmark, or operator")
		}
		if strings.TrimSpace(profile.Revision) == "" {
			return fmt.Errorf("model estimate revision is required for %q", profile.ModelID)
		}
	}

	sort.Strings(settings.RequiredMetrics)
	return nil
}

func modelEstimate(settings Settings, providerConfigurationID, modelID string) (ModelEstimate, bool) {
	for _, profile := range settings.ModelEstimates {
		if profile.ProviderConfigurationID == providerConfigurationID && profile.ModelID == modelID {
			return profile, true
		}
	}
	return ModelEstimate{}, false
}
