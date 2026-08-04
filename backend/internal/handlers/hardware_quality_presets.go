package handlers

import (
	"fmt"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/quality"
)

func workerStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

// normalizeHardwareQualityPreset is the execution authority for the UI presets.
// It deliberately runs when a job is planned as profiles can also arrive through
// saved records, imports, or per-asset overrides rather than the current UI.
func normalizeHardwareQualityPreset(profile models.Profile) models.Profile {
	preset := strings.ToLower(strings.TrimSpace(workerStringValue(profile.WorkerConfig["hardwareQualityPreset"])))
	if preset == "" || preset == "custom" {
		return profile
	}
	if profile.WorkerConfig == nil {
		profile.WorkerConfig = models.JSONMap{}
	}
	if workerIntValue(profile.WorkerConfig["hardwareQualityPresetScale"], 0) < 2 {
		preset = map[string]string{
			"compact": "recommended", "medium": "best_quality", "recommended": "high_quality",
			"best_quality": "archive", "high_quality": "master",
		}[preset]
		if preset == "" {
			preset = "custom"
		}
		profile.WorkerConfig["hardwareQualityPreset"] = preset
		profile.WorkerConfig["hardwareQualityPresetScale"] = 2
		if preset == "custom" {
			return profile
		}
	}
	encoder := strings.ToLower(strings.TrimSpace(workerStringValue(profile.WorkerConfig["videoEncoder"])))
	if encoder == "hevc_qsv" {
		baseQuality, ok := quality.QSVBaseQuality(quality.Preset(preset))
		qsvProfile, pixelFormat, formatOK := quality.QSVFormat(quality.Preset(preset))
		if !ok || !formatOK {
			preset = "recommended"
			baseQuality, _ = quality.QSVBaseQuality(quality.PresetRecommended)
			qsvProfile, pixelFormat, _ = quality.QSVFormat(quality.PresetRecommended)
		}
		profile.WorkerConfig["hardwareQualityPreset"] = preset
		profile.WorkerConfig["globalQuality"] = baseQuality
		profile.WorkerConfig["qsvRequestedGlobalQuality"] = baseQuality
		profile.WorkerConfig["qsvEffectiveGlobalQuality"] = baseQuality
		profile.WorkerConfig["qsvAssetQualityAdjustment"] = 0
		profile.WorkerConfig["qsvAssetQualityReasons"] = []string{}
		profile.WorkerConfig["qsvRateControl"] = "la_icq"
		profile.WorkerConfig["qsvLookAheadDepth"] = 40
		profile.WorkerConfig["qsvExtendedBRC"] = false
		profile.WorkerConfig["qsvAdaptiveI"] = false
		profile.WorkerConfig["qsvAdaptiveB"] = false
		profile.WorkerConfig["pixFmt"] = pixelFormat
		profile.PixelFormat = pixelFormat
		if qsvProfile == "main10" {
			profile.BitDepth = 10
		} else {
			profile.BitDepth = 8
		}
		return profile
	}
	if encoder == "hevc_videotoolbox" {
		intent := quality.NewIntent(quality.IntentInput{Preset: preset, ColorPolicy: workerStringValue(profile.WorkerConfig["finalColorPolicy"])})
		recommendation, err := (quality.VideoToolboxTranslator{}).Translate(intent, quality.WorkerCapabilities{})
		if err != nil {
			preset = "recommended"
			intent = quality.NewIntent(quality.IntentInput{Preset: preset, ColorPolicy: workerStringValue(profile.WorkerConfig["finalColorPolicy"])})
			recommendation, _ = (quality.VideoToolboxTranslator{}).Translate(intent, quality.WorkerCapabilities{})
		}
		profile.WorkerConfig["hardwareQualityPreset"] = preset
		profile.WorkerConfig["videoToolboxProfile"] = recommendation.Profile
		profile.WorkerConfig["videoToolboxGop"] = 120
		delete(profile.WorkerConfig, "videoToolboxRealtime")
		profile.WorkerConfig["videoToolboxAllowFrameReordering"] = false
		profile.WorkerConfig["videoToolboxPowerEfficiency"] = true
		profile.WorkerConfig["pixFmt"] = recommendation.PixelFormat
		delete(profile.WorkerConfig, "videoToolboxQualityProfile")
	}
	return profile
}
