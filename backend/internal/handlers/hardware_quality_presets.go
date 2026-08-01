package handlers

import (
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

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
	encoder := strings.ToLower(strings.TrimSpace(workerStringValue(profile.WorkerConfig["videoEncoder"])))
	if encoder == "hevc_qsv" {
		quality := map[string]int{"compact": 30, "medium": 27, "recommended": 25, "best_quality": 22, "high_quality": 20}[preset]
		if quality == 0 {
			preset, quality = "recommended", 25
		}
		profile.WorkerConfig["hardwareQualityPreset"] = preset
		profile.WorkerConfig["globalQuality"] = quality
		profile.WorkerConfig["qsvRateControl"] = map[string]string{"best_quality": "la_icq", "high_quality": "la_icq"}[preset]
		if profile.WorkerConfig["qsvRateControl"] == "" {
			profile.WorkerConfig["qsvRateControl"] = "icq"
		}
		profile.WorkerConfig["qsvLookAheadDepth"] = 40
		profile.WorkerConfig["qsvExtendedBRC"] = false
		profile.WorkerConfig["qsvAdaptiveI"] = false
		profile.WorkerConfig["qsvAdaptiveB"] = false
		if preset == "compact" || preset == "medium" {
			profile.WorkerConfig["pixFmt"] = "nv12"
		} else {
			profile.WorkerConfig["pixFmt"] = "p010le"
		}
		return profile
	}
	if encoder == "hevc_videotoolbox" {
		type settings struct{ profile, pixFmt string }
		values := map[string]settings{
			"compact": {"main", "yuv420p"}, "medium": {"main", "yuv420p"},
			"recommended": {"main10", "p010le"}, "best_quality": {"main10", "p010le"}, "high_quality": {"main10", "p010le"},
		}
		value, ok := values[preset]
		if !ok {
			preset, value = "recommended", values["recommended"]
		}
		profile.WorkerConfig["hardwareQualityPreset"] = preset
		profile.WorkerConfig["videoToolboxProfile"] = value.profile
		profile.WorkerConfig["videoToolboxGop"] = 120
		delete(profile.WorkerConfig, "videoToolboxRealtime")
		profile.WorkerConfig["videoToolboxAllowFrameReordering"] = false
		profile.WorkerConfig["videoToolboxPowerEfficiency"] = true
		profile.WorkerConfig["pixFmt"] = value.pixFmt
		delete(profile.WorkerConfig, "videoToolboxQualityProfile")
	}
	return profile
}
