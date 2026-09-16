package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"gorm.io/gorm"
)

const (
	assetDefaultsSettingKey      = "assetDefaults"
	assetDefaultsSchemaVersion   = 1
	assetDefaultsCustomVideoKey  = "__asset_defaults_custom_video__"
	assetDefaultsCustomAudioKey  = "__asset_defaults_custom_audio__"
	assetDefaultsCustomTracksKey = "__asset_defaults_custom_tracks__"
	assetDefaultsGlobalSource    = "global"
)

type AssetDefaultsSource string

const (
	AssetDefaultsSourceCustom  AssetDefaultsSource = "custom"
	AssetDefaultsSourceProfile AssetDefaultsSource = "profile"
)

type AssetDefaultsDomain struct {
	Source     AssetDefaultsSource `json:"source"`
	ProfileID  uint                `json:"profileId,omitempty"`
	ProfileKey string              `json:"profileKey,omitempty"`
	Custom     models.JSONMap      `json:"custom,omitempty"`
}

type AssetDefaultsValue struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Video         AssetDefaultsDomain `json:"video"`
	Audio         AssetDefaultsDomain `json:"audio"`
	Tracks        AssetDefaultsDomain `json:"tracks"`
}

func defaultAssetDefaultsValue() models.JSONMap {
	return models.JSONMap{
		"schemaVersion": assetDefaultsSchemaVersion,
		"video": models.JSONMap{
			"source":                      "custom",
			"custom":                      models.JSONMap{"videoCodec": "copy"},
			"removeNonPrimaryVideoTracks": false,
		},
		"audio": models.JSONMap{
			"source": "custom",
			"custom": models.JSONMap{
				"addCompatibleTrack":       false,
				"compatibleTrackQuality":   192,
				"compatibleTrackChannels":  2,
				"compatibleTrackSource":    "resolved",
				"preferredDefaultLanguage": "",
			},
		},
		"tracks": models.JSONMap{
			"source": "custom",
			"custom": models.JSONMap{
				"trackDispositionVersion":    canonicalTrackDispositionVersion,
				"audioMode":                  "all",
				"subtitleMode":               "all",
				"subtitleDisposition":        "keep",
				"subtitleSidecarFormats":     models.JSONList{"srt"},
				"attachmentPolicy":           "auto",
				"fontAttachmentExportPolicy": "none",
				"chapterPolicy":              "keep",
				"ocrMode":                    "accurate",
				"defaultAudioLanguage":       "",
				"defaultSubtitleLanguage":    "",
			},
		},
	}
}

func validateAssetDefaultsValue(db *gorm.DB, value models.JSONMap) (models.JSONMap, error) {
	result := mergeJSONMaps(defaultAssetDefaultsValue(), value)
	if assetDefaultsIntSetting(result["schemaVersion"], assetDefaultsSchemaVersion) != assetDefaultsSchemaVersion {
		return nil, fmt.Errorf("assetDefaults schemaVersion must be %d", assetDefaultsSchemaVersion)
	}
	for _, domain := range []string{"video", "audio", "tracks"} {
		block := settingProfileObject(result[domain])
		if block == nil {
			return nil, fmt.Errorf("assetDefaults.%s must be an object", domain)
		}
		source := strings.ToLower(strings.TrimSpace(workerStringValue(block["source"])))
		if source != "custom" && source != "profile" {
			return nil, fmt.Errorf("assetDefaults.%s.source must be custom or profile", domain)
		}
		block["source"] = source
		if source == "custom" {
			if custom := settingProfileObject(block["custom"]); custom != nil {
				block["custom"] = custom
			} else {
				block["custom"] = models.JSONMap{}
			}
			delete(block, "profileId")
			delete(block, "profileKey")
			continue
		}
		switch domain {
		case "video":
			block["custom"] = models.JSONMap{}
			profileID := uint(workerNumberValue(block["profileId"], 0))
			if profileID == 0 {
				return nil, fmt.Errorf("assetDefaults.video.profileId is required for a profile source")
			}
			if db != nil && db.Migrator().HasTable(&models.Profile{}) {
				var profile models.Profile
				if err := db.First(&profile, profileID).Error; err != nil || profile.Disabled {
					return nil, fmt.Errorf("assetDefaults.video.profileId references an unavailable profile")
				}
			}
			block["profileId"] = profileID
		case "audio", "tracks":
			block["custom"] = models.JSONMap{}
			key := strings.TrimSpace(workerStringValue(block["profileKey"]))
			if key == "" {
				return nil, fmt.Errorf("assetDefaults.%s.profileKey is required for a profile source", domain)
			}
			if db != nil && db.Migrator().HasTable(&models.AppSetting{}) {
				settingKey := "audioEnhancementProfiles"
				if domain == "tracks" {
					settingKey = "trackProfiles"
				}
				profile, err := settingProfileByKey(db, settingKey, key)
				if err != nil || boolValue(profile["disabled"], false) || strings.TrimSpace(workerStringValue(profile["deletedAt"])) != "" {
					return nil, fmt.Errorf("assetDefaults.%s.profileKey references an unavailable profile", domain)
				}
			}
			block["profileKey"] = key
		}
	}
	if audio := assetDefaultsDomain(result, "audio"); audio != nil {
		custom := settingProfileObject(audio["custom"])
		channels := assetDefaultsIntSetting(custom["compatibleTrackChannels"], 2)
		if channels < 1 || channels > 8 {
			return nil, fmt.Errorf("assetDefaults.audio.custom.compatibleTrackChannels must be between 1 and 8")
		}
		quality := assetDefaultsIntSetting(custom["compatibleTrackQuality"], 192)
		if quality < 1 {
			return nil, fmt.Errorf("assetDefaults.audio.custom.compatibleTrackQuality must be positive")
		}
		source := strings.ToLower(strings.TrimSpace(workerStringValue(custom["compatibleTrackSource"])))
		if source != "resolved" && source != "default" {
			return nil, fmt.Errorf("assetDefaults.audio.custom.compatibleTrackSource must be resolved or default")
		}
	}
	if tracks := assetDefaultsDomain(result, "tracks"); tracks != nil {
		custom := settingProfileObject(tracks["custom"])
		if _, err := ParseSubtitleDisposition(workerStringValue(custom["subtitleDisposition"])); err != nil {
			return nil, fmt.Errorf("assetDefaults.tracks.custom: %w", err)
		}
		if _, err := ParseAttachmentPolicy(workerStringValue(custom["attachmentPolicy"])); err != nil {
			return nil, fmt.Errorf("assetDefaults.tracks.custom: %w", err)
		}
		if _, err := ParseFontAttachmentExportPolicy(workerStringValue(custom["fontAttachmentExportPolicy"])); err != nil {
			return nil, fmt.Errorf("assetDefaults.tracks.custom: %w", err)
		}
		if _, err := ParseChapterPolicy(workerStringValue(custom["chapterPolicy"])); err != nil {
			return nil, fmt.Errorf("assetDefaults.tracks.custom: %w", err)
		}
		if len(normalizedSubtitleSidecarFormats(custom["subtitleSidecarFormats"])) == 0 {
			return nil, fmt.Errorf("assetDefaults.tracks.custom.subtitleSidecarFormats must contain original, srt, ass, or both")
		}
		ocrMode := strings.ToLower(strings.TrimSpace(workerStringValue(custom["ocrMode"])))
		if ocrMode != "" && ocrMode != "raw" && ocrMode != "clean" && ocrMode != "accurate" {
			return nil, fmt.Errorf("assetDefaults.tracks.custom.ocrMode must be raw, clean, or accurate")
		}
	}
	return result, nil
}

func loadAssetDefaults(db *gorm.DB) (models.JSONMap, error) {
	defaults, _, err := loadAssetDefaultsWithWarnings(db)
	return defaults, err
}

func loadAssetDefaultsWithWarnings(db *gorm.DB) (models.JSONMap, []string, error) {
	if db == nil || !db.Migrator().HasTable(&models.AppSetting{}) {
		return models.JSONMap{}, nil, nil
	}
	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", assetDefaultsSettingKey).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return models.JSONMap{}, nil, nil
		}
		return nil, nil, err
	}
	normalized, err := validateAssetDefaultsValue(db, setting.Value)
	if err == nil {
		return normalized, nil, nil
	}
	return defaultAssetDefaultsValue(), []string{err.Error()}, nil
}

func mergeJSONMaps(base, overlay models.JSONMap) models.JSONMap {
	result := models.JSONMap{}
	for key, value := range base {
		result[key] = value
	}
	for key, value := range overlay {
		baseMap := settingProfileObject(result[key])
		overlayMap := settingProfileObject(value)
		if baseMap != nil && overlayMap != nil {
			result[key] = mergeJSONMaps(models.JSONMap(baseMap), models.JSONMap(overlayMap))
			continue
		}
		result[key] = value
	}
	return result
}

func assetDefaultsIntSetting(value any, fallback int) int {
	return int(workerNumberValue(value, float64(fallback)))
}

func assetDefaultsDomain(value models.JSONMap, domain string) map[string]any {
	return settingProfileObject(value[domain])
}

func assetDefaultsProfileData(value models.JSONMap, domain string) (map[string]any, bool) {
	block := assetDefaultsDomain(value, domain)
	if block == nil || strings.EqualFold(workerStringValue(block["source"]), "profile") {
		return nil, false
	}
	custom := settingProfileObject(block["custom"])
	if len(custom) == 0 {
		return nil, false
	}
	return custom, true
}

func profileMap(profile models.Profile) map[string]any {
	encoded, err := json.Marshal(profile)
	if err != nil {
		return map[string]any{}
	}
	result := map[string]any{}
	if json.Unmarshal(encoded, &result) != nil {
		return map[string]any{}
	}
	return result
}

func profileFromMap(value map[string]any) (models.Profile, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return models.Profile{}, err
	}
	var profile models.Profile
	if err := json.Unmarshal(encoded, &profile); err != nil {
		return models.Profile{}, err
	}
	if profile.WorkerConfig == nil {
		profile.WorkerConfig = models.JSONMap{}
	}
	profile.ID = 0
	profile.Name = "Asset Defaults"
	profile.Scope = "asset"
	profile.ProfileVersion = max(profile.ProfileVersion, 1)
	return profile, nil
}

func (h QueueHandler) customVideoProfile(value models.JSONMap) (models.Profile, error) {
	var fallbackProfile models.Profile
	if err := h.db.Where("disabled = ?", false).Order("id asc").First(&fallbackProfile).Error; err != nil && err != gorm.ErrRecordNotFound {
		return models.Profile{}, err
	}
	custom, _ := assetDefaultsProfileData(value, "video")
	merged := profileMap(fallbackProfile)
	if custom != nil {
		merged = mergeJSONMaps(models.JSONMap(merged), models.JSONMap(custom))
	}
	profile, err := profileFromMap(map[string]any(merged))
	if err != nil {
		return models.Profile{}, err
	}
	if err := scheduler.ApplyAuthoritativeContract(&profile); err != nil {
		return models.Profile{}, fmt.Errorf("asset defaults video profile: %w", err)
	}
	applyAssetDefaultsVideoCustom(&profile, custom)
	block := assetDefaultsDomain(value, "video")
	if block != nil && boolValue(block["removeNonPrimaryVideoTracks"], false) {
		profile.WorkerConfig["removeNonPrimaryVideoTracks"] = true
	}
	return profile, nil
}

func customProfileFromDefaults(value models.JSONMap, domain string) map[string]any {
	custom, _ := assetDefaultsProfileData(value, domain)
	result := map[string]any{}
	for key, item := range custom {
		result[key] = item
	}
	return result
}

func profileDataForQueueJob(job models.QueueJob, domain string) map[string]any {
	if job.ProfileResolution == nil {
		return nil
	}
	raw, ok := job.ProfileResolution[domain]
	if !ok {
		return nil
	}
	block := settingProfileObject(raw)
	if block == nil {
		return nil
	}
	return settingProfileObject(block["profileData"])
}

func (h QueueHandler) customVideoProfileForJob(job models.QueueJob) (models.Profile, bool, error) {
	custom := profileDataForQueueJob(job, "video")
	if len(custom) == 0 {
		return models.Profile{}, false, nil
	}
	var fallbackProfile models.Profile
	if err := h.db.Where("disabled = ?", false).Order("id asc").First(&fallbackProfile).Error; err != nil {
		return models.Profile{}, false, err
	}
	merged := mergeJSONMaps(models.JSONMap(profileMap(fallbackProfile)), models.JSONMap(custom))
	profile, err := profileFromMap(map[string]any(merged))
	if err != nil {
		return models.Profile{}, false, err
	}
	if err := scheduler.ApplyAuthoritativeContract(&profile); err != nil {
		return models.Profile{}, false, fmt.Errorf("asset defaults video profile: %w", err)
	}
	applyAssetDefaultsVideoCustom(&profile, custom)
	return profile, true, nil
}

func applyAssetDefaultsVideoCustom(profile *models.Profile, custom map[string]any) {
	if profile == nil {
		return
	}
	if profile.WorkerConfig == nil {
		profile.WorkerConfig = models.JSONMap{}
	}
	if preferred := strings.TrimSpace(workerStringValue(custom["preferredEncoder"])); preferred != "" {
		profile.WorkerConfig["videoEncoder"] = preferred
		profile.WorkerConfig["useHardwareIfAvailable"] = preferred == "auto" || isHardwareVideoEncoder(preferred)
	}
	if encoder := strings.TrimSpace(workerStringValue(custom["videoEncoder"])); encoder != "" {
		profile.WorkerConfig["videoEncoder"] = encoder
		profile.WorkerConfig["useHardwareIfAvailable"] = encoder == "auto" || isHardwareVideoEncoder(encoder)
	}
}

func applyAssetDefaultsAudioToVideoProfile(profile *models.Profile, job models.QueueJob) {
	if profile == nil {
		return
	}
	audio := profileDataForQueueJob(job, "audio")
	if len(audio) == 0 {
		return
	}
	if profile.WorkerConfig == nil {
		profile.WorkerConfig = models.JSONMap{}
	}
	if value, ok := audio["addCompatibleTrack"]; ok {
		profile.WorkerConfig["addAacStereoTrack"] = boolValue(value, false)
	}
	if value, ok := audio["compatibleTrackQuality"]; ok {
		profile.WorkerConfig["aacStereoBitrateKbps"] = workerNumberValue(value, 192)
	}
	if value, ok := audio["compatibleTrackChannels"]; ok {
		profile.WorkerConfig["aacStereoChannels"] = workerNumberValue(value, 2)
	}
	if value, ok := audio["compatibleTrackSource"]; ok && strings.EqualFold(workerStringValue(value), "default") {
		profile.WorkerConfig["aacStereoSourceStreamIndex"] = -1
	}
}

func applyAssetDefaultsAudioToTrackProfile(job models.QueueJob, profile map[string]any) map[string]any {
	if len(profile) == 0 {
		return profile
	}
	audio := profileDataForQueueJob(job, "audio")
	if len(audio) == 0 {
		return profile
	}
	if strings.TrimSpace(workerStringValue(profile["defaultAudioLanguage"])) == "" {
		if language := strings.TrimSpace(workerStringValue(audio["preferredDefaultLanguage"])); language != "" {
			profile["defaultAudioLanguage"] = strings.ToLower(language)
		}
	}
	return profile
}

func captureCustomProfileSnapshot(profile models.Profile, source string) (models.JSONMap, *time.Time, error) {
	now := time.Now()
	snapshot, err := scheduler.CaptureProfileSnapshot(profile, now, source)
	if err != nil {
		return nil, nil, err
	}
	return snapshot, &now, nil
}
