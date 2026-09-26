package handlers

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// resolvedSubtitleTrackProfileForAsset composes the existing asset scope,
// Track Profile, Asset Defaults, and Subtitle Settings authorities into the
// profile consumed by resolveTrackPlan. It does not make subtitle decisions.
func resolvedSubtitleTrackProfileForAsset(db *gorm.DB, assetPath string) (map[string]any, error) {
	defaults, warnings, err := loadAssetDefaultsWithWarnings(db)
	if err != nil {
		return nil, err
	}
	defaultsSource := "global_settings"
	if len(defaults) == 0 || assetDefaultsWarningsReplaceWholeValue(warnings) {
		if len(defaults) == 0 {
			defaults = defaultAssetDefaultsValue()
		}
		defaultsSource = "built_in"
	}
	globalProfile, err := globalTrackProfileData(db, defaults)
	if err != nil {
		return nil, fmt.Errorf("global Tracks defaults: %w", err)
	}
	tracksSource := defaultsSource
	if assetDefaultsWarningsReplaceDomain(warnings, "tracks") {
		tracksSource = "built_in"
	}
	markGlobalSubtitleTrackSources(db, defaults, globalProfile, tracksSource)

	effective := EffectiveAssetConfiguration{AssetPath: filepath.Clean(assetPath)}
	effective.Tracks.Selection = configSelectionInherit
	if db != nil && db.Migrator().HasTable(&models.AssetRecord{}) {
		effective, err = effectiveAssetConfiguration(db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}), assetPath)
		if err != nil {
			return nil, err
		}
	}
	assetProfile, err := effectiveTrackProfileData(db, effective.Tracks)
	if err != nil {
		return nil, fmt.Errorf("asset Tracks profile: %w", err)
	}
	switch {
	case effective.Tracks.Source == assetDefaultsGlobalSource || effective.Tracks.Selection == configSelectionInherit:
		assetProfile = nil
	case effective.Tracks.Selection == configSelectionDisabled:
		// Explicit none shadows the global Track Profile while retaining the
		// canonical built-in behavior for operation-only extraction.
		globalProfile = nil
		assetProfile = nil
	default:
		markSubtitleTrackSources(assetProfile, "asset_profile")
	}

	profile := mergeSubtitleTrackProfiles(globalProfile, assetProfile)
	if profile == nil {
		profile = map[string]any{}
	}
	profile[subtitleTrackSourceKey] = "built_in"
	if assetProfile != nil {
		profile[subtitleTrackSourceKey] = "asset_profile"
	}
	if db != nil && db.Migrator().HasTable(&models.ScanResult{}) {
		profile = resolveTrackProfileForAsset(db, assetPath, profile)
	}
	profile = applyPersistedSubtitleSettings(profile, generateSRTFallbackForASSSetting(db), persistedSubtitleFieldSource(db, generateSRTFallbackForASSKey))
	profile = applyPersistedSubtitleStylingSettings(profile, subtitleStylingPolicyFromDB(db), persistedSubtitleFieldSource(db, subtitleStylingPolicyKey))
	return profile, nil
}

func globalTrackProfileData(db *gorm.DB, defaults models.JSONMap) (map[string]any, error) {
	block := assetDefaultsDomain(defaults, "tracks")
	if block == nil {
		return nil, nil
	}
	if strings.EqualFold(workerStringValue(block["source"]), "profile") {
		key := strings.TrimSpace(workerStringValue(block["profileKey"]))
		if key == "" {
			return nil, fmt.Errorf("selected Tracks Profile has no profile key")
		}
		return settingProfileByKey(db, "trackProfiles", key)
	}
	return customProfileFromDefaults(defaults, "tracks"), nil
}

func effectiveTrackProfileData(db *gorm.DB, configuration AssetConfigurationValue) (map[string]any, error) {
	profile := map[string]any{}
	switch configuration.Selection {
	case "profile":
		if strings.TrimSpace(configuration.ProfileKey) == "" {
			return nil, fmt.Errorf("selected Tracks Profile has no profile key")
		}
		return settingProfileByKey(db, "trackProfiles", configuration.ProfileKey)
	case configSelectionValue:
		for key, value := range configuration.ProfileData {
			profile[key] = value
		}
	case "", configSelectionInherit, configSelectionDisabled:
		return profile, nil
	default:
		return nil, fmt.Errorf("unsupported Tracks configuration selection %q", configuration.Selection)
	}
	return profile, nil
}

func markGlobalSubtitleTrackSources(db *gorm.DB, defaults models.JSONMap, profile map[string]any, fallbackSource string) {
	if profile == nil {
		return
	}
	if fallbackSource == "built_in" {
		markSubtitleTrackSources(profile, "built_in")
		return
	}
	block := assetDefaultsDomain(defaults, "tracks")
	if strings.EqualFold(workerStringValue(block["source"]), "profile") {
		markSubtitleTrackSources(profile, "global_settings")
		return
	}
	var setting models.AppSetting
	if db == nil || !db.Migrator().HasTable(&models.AppSetting{}) || db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).First(&setting, "key = ?", assetDefaultsSettingKey).Error != nil {
		return
	}
	tracks := settingProfileObject(settingProfileObject(setting.Value["tracks"])["custom"])
	markSubtitleTrackSourcesPresent(profile, tracks, "global_settings")
}

func markSubtitleTrackSourcesPresent(profile, values map[string]any, source string) {
	if profile == nil || values == nil {
		return
	}
	for _, key := range []string{
		"subtitleSidecarFormats", "subtitleSidecarFormatsByStream", "subtitleRules", "ocrLanguage", "ocrMode",
		"subtitleDisposition", "defaultSubtitleLanguage", "preferredSubtitleRules",
	} {
		if _, present := values[key]; present {
			setSubtitleSettingSource(profile, key, source)
		}
	}
	if rules := workerSliceValue(values["subtitleRules"]); len(rules) > 0 {
		sources := settingProfileObject(profile[subtitleSettingSourcesKey])
		copy := map[string]any{}
		for key, value := range sources {
			copy[key] = value
		}
		copy[subtitleRuleSourcesKey] = repeatedSubtitleRuleSources(len(rules), source)
		profile[subtitleSettingSourcesKey] = copy
	}
}

func persistedSubtitleFieldSource(db *gorm.DB, field string) string {
	if db == nil || !db.Migrator().HasTable(&models.AppSetting{}) {
		return "built_in"
	}
	var setting models.AppSetting
	if err := db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).First(&setting, "key = ?", subtitleSettingsKey).Error; err != nil {
		return "built_in"
	}
	switch field {
	case generateSRTFallbackForASSKey:
		if _, present := setting.Value[generateSRTFallbackForASSKey]; present {
			return "global_settings"
		}
	case subtitleStylingPolicyKey:
		if _, present := setting.Value[subtitleStylingPolicyKey]; present {
			return "global_settings"
		}
	}
	return "built_in"
}
