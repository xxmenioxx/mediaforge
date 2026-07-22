package runtimeinfo

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
)

type RuntimeLimits struct {
	MaxRunningJobs        int   `json:"maxRunningJobs"`
	MaxVideoJobs          int   `json:"maxVideoJobs"`
	MaxSoftwareX265Jobs   int   `json:"maxSoftwareX265Jobs"`
	MaxHardwareEncodeJobs int   `json:"maxHardwareEncodeJobs"`
	MaxAudioJobs          int   `json:"maxAudioJobs"`
	MaxLabJobs            int   `json:"maxLabJobs"`
	MinFreeRAMGB          int64 `json:"minFreeRamGb"`
	MinFreeWorkGB         int64 `json:"minFreeWorkGb"`
	MinFreeLibraryGB      int64 `json:"minFreeLibraryGb"`
	MaxWorkspaceGB        int64 `json:"maxWorkspaceGb"`
	AllowDirectMode       bool  `json:"allowDirectMode"`
}

type RuntimeBehavior struct {
	PauseWhenOnBattery     bool `json:"pauseWhenOnBattery"`
	PreventSleepDuringJobs bool `json:"preventSleepDuringJobs"`
}

type RuntimeProfileValues struct {
	RuntimeLimits
	RuntimeBehavior
}

type RuntimeProfileDefinition struct {
	Key         string               `json:"key"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Official    bool                 `json:"official"`
	Values      RuntimeProfileValues `json:"values"`
}

type RuntimeProfileOverride struct {
	MaxRunningJobs         *int   `json:"maxRunningJobs,omitempty"`
	MaxVideoJobs           *int   `json:"maxVideoJobs,omitempty"`
	MaxSoftwareX265Jobs    *int   `json:"maxSoftwareX265Jobs,omitempty"`
	MaxHardwareEncodeJobs  *int   `json:"maxHardwareEncodeJobs,omitempty"`
	MaxAudioJobs           *int   `json:"maxAudioJobs,omitempty"`
	MaxLabJobs             *int   `json:"maxLabJobs,omitempty"`
	MinFreeRAMGB           *int64 `json:"minFreeRamGb,omitempty"`
	MinFreeWorkGB          *int64 `json:"minFreeWorkGb,omitempty"`
	MinFreeLibraryGB       *int64 `json:"minFreeLibraryGb,omitempty"`
	MaxWorkspaceGB         *int64 `json:"maxWorkspaceGb,omitempty"`
	AllowDirectMode        *bool  `json:"allowDirectMode,omitempty"`
	PauseWhenOnBattery     *bool  `json:"pauseWhenOnBattery,omitempty"`
	PreventSleepDuringJobs *bool  `json:"preventSleepDuringJobs,omitempty"`
}

type RuntimePolicyConfig struct {
	SchemaVersion    int                               `json:"schemaVersion"`
	Mode             string                            `json:"mode"`
	PreferredProfile string                            `json:"preferredProfile"`
	SelectedProfile  string                            `json:"selectedProfile,omitempty"`
	FallbackProfile  string                            `json:"fallbackProfile"`
	Overrides        map[string]RuntimeProfileOverride `json:"overrides"`
}

type EffectiveRuntimePolicy struct {
	Mode             string                 `json:"mode"`
	DetectedProfile  string                 `json:"detectedProfile"`
	PreferredProfile string                 `json:"preferredProfile"`
	FallbackProfile  string                 `json:"fallbackProfile"`
	BaseProfile      string                 `json:"baseProfile"`
	Values           RuntimeProfileValues   `json:"values"`
	Overrides        RuntimeProfileOverride `json:"overrides"`
	OverriddenFields []string               `json:"overriddenFields"`
	SelectionReasons models.JSONList        `json:"selectionReasons"`
}

func OfficialRuntimeProfiles() []RuntimeProfileDefinition {
	gb := func(running, video, software, hardware, audio, lab int, ram, work, library, workspace int64, direct, battery, sleep bool) RuntimeProfileValues {
		return RuntimeProfileValues{RuntimeLimits: RuntimeLimits{running, video, software, hardware, audio, lab, ram, work, library, workspace, direct}, RuntimeBehavior: RuntimeBehavior{battery, sleep}}
	}
	return []RuntimeProfileDefinition{
		{"nas_safe", "NAS Safe", "Single heavy conversion with conservative storage reserves.", true, gb(1, 1, 1, 1, 2, 1, 4, 80, 300, 250, false, false, false)},
		{"nas_balanced", "NAS Balanced", "Balanced concurrency for a containerized NAS.", true, gb(2, 2, 1, 2, 3, 1, 4, 80, 500, 350, false, false, false)},
		{"desktop_safe", "Desktop Safe", "Conservative general-purpose desktop policy.", true, gb(1, 1, 1, 1, 2, 1, 4, 30, 30, 200, true, false, false)},
		{"desktop_balanced", "Desktop Balanced", "Default policy for a general-purpose desktop.", true, gb(2, 2, 1, 2, 3, 1, 4, 40, 50, 300, true, false, false)},
		{"laptop_safe", "Laptop Safe", "Low concurrency with battery and sleep protection.", true, gb(1, 1, 1, 1, 2, 1, 6, 100, 100, 150, true, true, true)},
		{"workstation_balanced", "Workstation Balanced", "Moderate parallelism for high-core workstations.", true, gb(3, 3, 2, 3, 4, 1, 8, 100, 100, 500, true, false, false)},
		{"workstation_aggressive", "Workstation Aggressive", "High throughput when thermals and storage allow it.", true, gb(6, 6, 4, 6, 6, 2, 6, 80, 80, 800, true, false, false)},
	}
}

func RuntimeProfile(key string) (RuntimeProfileDefinition, bool) {
	for _, profile := range OfficialRuntimeProfiles() {
		if profile.Key == key {
			return profile, true
		}
	}
	return RuntimeProfileDefinition{}, false
}

func LoadRuntimePolicy(db *gorm.DB) (RuntimePolicyConfig, error) {
	config := RuntimePolicyConfig{SchemaVersion: 2, Mode: "automatic", PreferredProfile: "auto", FallbackProfile: "desktop_safe", Overrides: map[string]RuntimeProfileOverride{}}
	var setting models.AppSetting
	result := db.Where("key = ?", "runtimePolicy").Limit(1).Find(&setting)
	if result.Error != nil || result.RowsAffected == 0 {
		return config, result.Error
	}
	data, err := json.Marshal(setting.Value)
	if err != nil {
		return config, err
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return config, err
	}
	if config.Overrides == nil {
		config.Overrides = map[string]RuntimeProfileOverride{}
	}
	if config.PreferredProfile == "" {
		if config.Mode == "manual" && config.SelectedProfile != "" {
			config.PreferredProfile = config.SelectedProfile
		} else {
			config.PreferredProfile = "auto"
		}
	}
	if _, ok := RuntimeProfile(config.FallbackProfile); !ok {
		config.FallbackProfile = "desktop_safe"
	}
	return config, nil
}

func ValidateRuntimePolicyValue(value models.JSONMap) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var config RuntimePolicyConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}
	if config.Mode != "automatic" && config.Mode != "manual" {
		return fmt.Errorf("runtime mode must be automatic or manual")
	}
	if config.Mode == "manual" && config.PreferredProfile == "auto" {
		return fmt.Errorf("manual runtime mode requires a specific preferred profile")
	}
	if !ValidRuntimeProfile(config.PreferredProfile) {
		return fmt.Errorf("unknown preferred runtime profile %q", config.PreferredProfile)
	}
	if config.FallbackProfile == "auto" || !ValidRuntimeProfile(config.FallbackProfile) {
		return fmt.Errorf("unknown fallback runtime profile %q", config.FallbackProfile)
	}
	for key := range config.Overrides {
		if _, ok := RuntimeProfile(key); !ok {
			return fmt.Errorf("runtime overrides reference unknown official profile %q", key)
		}
	}
	return nil
}

func ResolveEffectiveRuntimePolicy(db *gorm.DB, detected string) (EffectiveRuntimePolicy, error) {
	config, err := LoadRuntimePolicy(db)
	if err != nil {
		return EffectiveRuntimePolicy{}, err
	}
	base := detected
	if config.Mode == "manual" || (config.PreferredProfile != "" && config.PreferredProfile != "auto") {
		base = config.PreferredProfile
	}
	profile, ok := RuntimeProfile(base)
	if !ok {
		base = config.FallbackProfile
		profile, ok = RuntimeProfile(base)
	}
	if !ok {
		return EffectiveRuntimePolicy{}, fmt.Errorf("runtime fallback profile %q is unavailable", config.FallbackProfile)
	}
	override := config.Overrides[base]
	values, fields := applyRuntimeOverride(profile.Values, override)
	reasons := models.JSONList{fmt.Sprintf("Machine detection recommended %s", detected)}
	if base != detected {
		reasons = append(reasons, fmt.Sprintf("Runtime policy selected preferred profile %s", base))
	}
	if len(fields) > 0 {
		reasons = append(reasons, fmt.Sprintf("Applied %d user runtime overrides", len(fields)))
	}
	return EffectiveRuntimePolicy{Mode: config.Mode, DetectedProfile: detected, PreferredProfile: config.PreferredProfile, FallbackProfile: config.FallbackProfile, BaseProfile: base, Values: values, Overrides: override, OverriddenFields: fields, SelectionReasons: reasons}, nil
}

func applyRuntimeOverride(values RuntimeProfileValues, override RuntimeProfileOverride) (RuntimeProfileValues, []string) {
	fields := []string{}
	setInt := func(name string, source *int, target *int) {
		if source != nil {
			*target = max(*source, 1)
			fields = append(fields, name)
		}
	}
	setInt64 := func(name string, source *int64, target *int64) {
		if source != nil {
			*target = max(*source, 1)
			fields = append(fields, name)
		}
	}
	setBool := func(name string, source *bool, target *bool) {
		if source != nil {
			*target = *source
			fields = append(fields, name)
		}
	}
	setInt("maxRunningJobs", override.MaxRunningJobs, &values.MaxRunningJobs)
	setInt("maxVideoJobs", override.MaxVideoJobs, &values.MaxVideoJobs)
	setInt("maxSoftwareX265Jobs", override.MaxSoftwareX265Jobs, &values.MaxSoftwareX265Jobs)
	setInt("maxHardwareEncodeJobs", override.MaxHardwareEncodeJobs, &values.MaxHardwareEncodeJobs)
	setInt("maxAudioJobs", override.MaxAudioJobs, &values.MaxAudioJobs)
	setInt("maxLabJobs", override.MaxLabJobs, &values.MaxLabJobs)
	setInt64("minFreeRamGb", override.MinFreeRAMGB, &values.MinFreeRAMGB)
	setInt64("minFreeWorkGb", override.MinFreeWorkGB, &values.MinFreeWorkGB)
	setInt64("minFreeLibraryGb", override.MinFreeLibraryGB, &values.MinFreeLibraryGB)
	setInt64("maxWorkspaceGb", override.MaxWorkspaceGB, &values.MaxWorkspaceGB)
	setBool("allowDirectMode", override.AllowDirectMode, &values.AllowDirectMode)
	setBool("pauseWhenOnBattery", override.PauseWhenOnBattery, &values.PauseWhenOnBattery)
	setBool("preventSleepDuringJobs", override.PreventSleepDuringJobs, &values.PreventSleepDuringJobs)
	return values, fields
}

func ValidRuntimeProfile(value string) bool {
	if strings.TrimSpace(value) == "auto" {
		return true
	}
	_, ok := RuntimeProfile(value)
	return ok
}
