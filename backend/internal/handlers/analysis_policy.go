package handlers

import (
	"fmt"
	"slices"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
)

const analysisPolicySettingKey = "analysisPolicy"

type AnalysisPolicy struct {
	Mode                     string    `json:"mode"`
	AdaptiveAnalysis         bool      `json:"adaptiveAnalysis"`
	EarlyConfidenceEnabled   bool      `json:"earlyConfidenceEnabled"`
	EarlyConfidenceThreshold float64   `json:"earlyConfidenceThreshold"`
	InitialWindows           int       `json:"initialWindows"`
	MaximumWindows           int       `json:"maximumWindows"`
	WindowSeconds            float64   `json:"windowSeconds"`
	Positions                []float64 `json:"positions"`
	InterlaceValidation      string    `json:"interlaceValidation"`
	CropDepth                string    `json:"cropDepth"`
	ReuseSnapshots           bool      `json:"reuseSnapshots"`
	IncrementalRefresh       bool      `json:"incrementalRefresh"`
	ConcurrentAssets         int       `json:"concurrentAssets"`
}

func balancedAnalysisPolicy() AnalysisPolicy {
	return AnalysisPolicy{
		Mode: "balanced", AdaptiveAnalysis: true, EarlyConfidenceEnabled: true, EarlyConfidenceThreshold: 0.98,
		InitialWindows: 3, MaximumWindows: 5, WindowSeconds: 20, Positions: []float64{0.08, 0.27, 0.50, 0.73, 0.92},
		InterlaceValidation: "automatic", CropDepth: "normal", ReuseSnapshots: true, IncrementalRefresh: true, ConcurrentAssets: 1,
	}
}

func analysisPolicyPreset(mode string) AnalysisPolicy {
	policy := balancedAnalysisPolicy()
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "fast":
		policy.Mode = "fast"
		policy.AdaptiveAnalysis = true
		policy.EarlyConfidenceEnabled = false
		policy.InitialWindows = 3
		policy.MaximumWindows = 3
		policy.Positions = []float64{0.08, 0.50, 0.92}
		policy.CropDepth = "reduced"
	case "thorough":
		policy.Mode = "thorough"
		policy.AdaptiveAnalysis = false
		policy.EarlyConfidenceEnabled = false
		policy.InitialWindows = 5
		policy.MaximumWindows = 5
		policy.InterlaceValidation = "always"
		policy.CropDepth = "full"
	case "custom":
		policy.Mode = "custom"
	}
	return policy
}

func analysisPolicy(db *gorm.DB) AnalysisPolicy {
	policy := balancedAnalysisPolicy()
	if db == nil {
		return policy
	}
	var setting models.AppSetting
	if err := db.Where("key = ?", analysisPolicySettingKey).First(&setting).Error; err != nil {
		return policy
	}
	mode := strings.ToLower(strings.TrimSpace(stringFromUnknown(setting.Value["mode"])))
	if mode != "custom" {
		if slices.Contains([]string{"fast", "balanced", "thorough"}, mode) {
			policy = analysisPolicyPreset(mode)
			policy.ReuseSnapshots = boolFromUnknown(setting.Value["reuseSnapshots"], policy.ReuseSnapshots)
			policy.IncrementalRefresh = boolFromUnknown(setting.Value["incrementalRefresh"], policy.IncrementalRefresh)
			policy.ConcurrentAssets = workerIntValue(setting.Value["concurrentAssets"], policy.ConcurrentAssets)
			return normalizedAnalysisPolicy(policy)
		}
		return policy
	}
	policy.Mode = "custom"
	policy.AdaptiveAnalysis = boolFromUnknown(setting.Value["adaptiveAnalysis"], policy.AdaptiveAnalysis)
	policy.EarlyConfidenceEnabled = boolFromUnknown(setting.Value["earlyConfidenceEnabled"], policy.EarlyConfidenceEnabled)
	policy.EarlyConfidenceThreshold = workerNumberValue(setting.Value["earlyConfidenceThreshold"], policy.EarlyConfidenceThreshold)
	policy.InitialWindows = workerIntValue(setting.Value["initialWindows"], policy.InitialWindows)
	policy.MaximumWindows = workerIntValue(setting.Value["maximumWindows"], policy.MaximumWindows)
	policy.WindowSeconds = workerNumberValue(setting.Value["windowSeconds"], policy.WindowSeconds)
	policy.Positions = floatSliceFromUnknown(setting.Value["positions"], policy.Positions)
	policy.InterlaceValidation = strings.ToLower(strings.TrimSpace(stringFromUnknown(setting.Value["interlaceValidation"])))
	policy.CropDepth = strings.ToLower(strings.TrimSpace(stringFromUnknown(setting.Value["cropDepth"])))
	policy.ReuseSnapshots = boolFromUnknown(setting.Value["reuseSnapshots"], policy.ReuseSnapshots)
	policy.IncrementalRefresh = boolFromUnknown(setting.Value["incrementalRefresh"], policy.IncrementalRefresh)
	policy.ConcurrentAssets = workerIntValue(setting.Value["concurrentAssets"], policy.ConcurrentAssets)
	return normalizedAnalysisPolicy(policy)
}

func normalizedAnalysisPolicy(policy AnalysisPolicy) AnalysisPolicy {
	policy.InitialWindows = max(3, min(5, policy.InitialWindows))
	policy.MaximumWindows = max(policy.InitialWindows, min(5, policy.MaximumWindows))
	policy.WindowSeconds = max(5, min(60, policy.WindowSeconds))
	policy.EarlyConfidenceThreshold = max(0.90, min(1.0, policy.EarlyConfidenceThreshold))
	policy.ConcurrentAssets = max(1, min(4, policy.ConcurrentAssets))
	if len(policy.Positions) < policy.MaximumWindows {
		policy.Positions = append([]float64(nil), balancedAnalysisPolicy().Positions...)
	}
	policy.Positions = append([]float64(nil), policy.Positions[:policy.MaximumWindows]...)
	if !slices.Contains([]string{"automatic", "always"}, policy.InterlaceValidation) {
		policy.InterlaceValidation = "automatic"
	}
	if !slices.Contains([]string{"reduced", "normal", "full"}, policy.CropDepth) {
		policy.CropDepth = "normal"
	}
	return policy
}

func validateAnalysisPolicy(value models.JSONMap) error {
	mode := strings.ToLower(strings.TrimSpace(stringFromUnknown(value["mode"])))
	if !slices.Contains([]string{"fast", "balanced", "thorough", "custom"}, mode) {
		return fmt.Errorf("analysis mode must be fast, balanced, thorough, or custom")
	}
	concurrentAssets := workerIntValue(value["concurrentAssets"], 0)
	if concurrentAssets < 1 || concurrentAssets > 4 {
		return fmt.Errorf("concurrent analysis assets must be between 1 and 4")
	}
	if mode != "custom" {
		return nil
	}
	policy := analysisPolicyPreset("custom")
	policy.InitialWindows = workerIntValue(value["initialWindows"], 0)
	policy.MaximumWindows = workerIntValue(value["maximumWindows"], 0)
	policy.WindowSeconds = workerNumberValue(value["windowSeconds"], 0)
	policy.EarlyConfidenceThreshold = workerNumberValue(value["earlyConfidenceThreshold"], 0)
	if policy.InitialWindows < 3 || policy.InitialWindows > 5 || policy.MaximumWindows < policy.InitialWindows || policy.MaximumWindows > 5 {
		return fmt.Errorf("analysis windows must satisfy 3 <= initial <= maximum <= 5")
	}
	if policy.EarlyConfidenceThreshold < 0.90 || policy.EarlyConfidenceThreshold > 1 {
		return fmt.Errorf("early confidence threshold must be between 0.90 and 1.00")
	}
	if policy.WindowSeconds < 5 || policy.WindowSeconds > 60 {
		return fmt.Errorf("analysis window length must be between 5 and 60 seconds")
	}
	positions, err := strictAnalysisPositions(value["positions"])
	if err != nil || len(positions) < policy.MaximumWindows {
		if err != nil {
			return err
		}
		return fmt.Errorf("analysis positions must include at least maximumWindows entries")
	}
	interlaceValidation := strings.ToLower(strings.TrimSpace(stringFromUnknown(value["interlaceValidation"])))
	if !slices.Contains([]string{"automatic", "always"}, interlaceValidation) {
		return fmt.Errorf("interlace validation must be automatic or always")
	}
	cropDepth := strings.ToLower(strings.TrimSpace(stringFromUnknown(value["cropDepth"])))
	if !slices.Contains([]string{"reduced", "normal", "full"}, cropDepth) {
		return fmt.Errorf("crop depth must be reduced, normal, or full")
	}
	return nil
}

func normalizedAnalysisPolicyValue(value models.JSONMap) models.JSONMap {
	policy := analysisPolicyPreset(strings.ToLower(strings.TrimSpace(stringFromUnknown(value["mode"]))))
	policy.ReuseSnapshots = boolFromUnknown(value["reuseSnapshots"], policy.ReuseSnapshots)
	policy.IncrementalRefresh = boolFromUnknown(value["incrementalRefresh"], policy.IncrementalRefresh)
	policy.ConcurrentAssets = workerIntValue(value["concurrentAssets"], policy.ConcurrentAssets)
	if policy.Mode == "custom" {
		policy.AdaptiveAnalysis = boolFromUnknown(value["adaptiveAnalysis"], policy.AdaptiveAnalysis)
		policy.EarlyConfidenceEnabled = boolFromUnknown(value["earlyConfidenceEnabled"], policy.EarlyConfidenceEnabled)
		policy.EarlyConfidenceThreshold = workerNumberValue(value["earlyConfidenceThreshold"], policy.EarlyConfidenceThreshold)
		policy.InitialWindows = workerIntValue(value["initialWindows"], policy.InitialWindows)
		policy.MaximumWindows = workerIntValue(value["maximumWindows"], policy.MaximumWindows)
		policy.WindowSeconds = workerNumberValue(value["windowSeconds"], policy.WindowSeconds)
		policy.Positions, _ = strictAnalysisPositions(value["positions"])
		policy.InterlaceValidation = strings.ToLower(strings.TrimSpace(stringFromUnknown(value["interlaceValidation"])))
		policy.CropDepth = strings.ToLower(strings.TrimSpace(stringFromUnknown(value["cropDepth"])))
	}
	policy = normalizedAnalysisPolicy(policy)
	return models.JSONMap{"mode": policy.Mode, "adaptiveAnalysis": policy.AdaptiveAnalysis, "earlyConfidenceEnabled": policy.EarlyConfidenceEnabled, "earlyConfidenceThreshold": policy.EarlyConfidenceThreshold, "initialWindows": policy.InitialWindows, "maximumWindows": policy.MaximumWindows, "windowSeconds": policy.WindowSeconds, "positions": policy.Positions, "interlaceValidation": policy.InterlaceValidation, "cropDepth": policy.CropDepth, "reuseSnapshots": policy.ReuseSnapshots, "incrementalRefresh": policy.IncrementalRefresh, "concurrentAssets": policy.ConcurrentAssets}
}

func strictAnalysisPositions(value any) ([]float64, error) {
	var raw []any
	switch typed := value.(type) {
	case []any:
		raw = typed
	case []float64:
		raw = make([]any, len(typed))
		for index, item := range typed {
			raw[index] = item
		}
	default:
		return nil, fmt.Errorf("analysis positions must be a numeric list")
	}
	positions := make([]float64, 0, len(raw))
	for _, item := range raw {
		position := workerNumberValue(item, -1)
		if position <= 0 || position >= 1 {
			return nil, fmt.Errorf("analysis positions must be between 0 and 1")
		}
		if len(positions) > 0 && position <= positions[len(positions)-1] {
			return nil, fmt.Errorf("analysis positions must be unique and strictly increasing")
		}
		positions = append(positions, position)
	}
	return positions, nil
}

func boolFromUnknown(value any, fallback bool) bool {
	typed, ok := value.(bool)
	if !ok {
		return fallback
	}
	return typed
}

func floatSliceFromUnknown(value any, fallback []float64) []float64 {
	values, ok := value.([]any)
	if !ok {
		if typed, typedOK := value.([]float64); typedOK {
			return append([]float64(nil), typed...)
		}
		return append([]float64(nil), fallback...)
	}
	result := make([]float64, 0, len(values))
	for _, item := range values {
		position := workerNumberValue(item, 0)
		if position > 0 && position < 1 {
			result = append(result, position)
		}
	}
	return result
}
