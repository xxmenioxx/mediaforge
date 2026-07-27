package database

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/runtimeinfo"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func Open(path string) (*gorm.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	dsn := path
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	dsn += separator + "_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	// SQLite has a single writer. Serializing writes at the connection pool
	// prevents SQLITE_BUSY/SQLITE_LOCKED failures across background services.
	sqlDB.SetMaxOpenConns(1)
	return db, nil
}

func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&models.Library{},
		&models.Profile{},
		&models.QueueJob{},
		&models.ExecutionPlan{},
		&models.RuntimeSnapshot{},
		&models.SchedulerReservation{},
		&models.WorkerNode{},
		&models.ScanResult{},
		&models.AssetRecord{},
		&models.AppSetting{},
	); err != nil {
		return err
	}
	if err := migrateRuntimePolicy(db); err != nil {
		return err
	}

	if err := normalizeProfileContracts(db); err != nil {
		return err
	}
	if err := backfillQueueProfileSnapshots(db); err != nil {
		return err
	}
	if err := backfillQueueLifecycleStages(db); err != nil {
		return err
	}
	if err := scheduler.ReconcileReservations(db); err != nil {
		return err
	}
	return backfillPendingExecutionPlans(db)
}

func migrateRuntimePolicy(db *gorm.DB) error {
	var setting models.AppSetting
	result := db.Where("key = ?", "runtimePolicy").Limit(1).Find(&setting)
	if result.Error != nil || result.RowsAffected == 0 || intFromJSON(setting.Value["schemaVersion"]) >= 2 {
		return result.Error
	}
	mode, _ := setting.Value["mode"].(string)
	selected, _ := setting.Value["selectedProfile"].(string)
	if _, ok := runtimeinfo.RuntimeProfile(selected); !ok {
		selected = "desktop_balanced"
	}
	preferred := "auto"
	if mode == "manual" {
		preferred = selected
	}
	config := runtimeinfo.RuntimePolicyConfig{SchemaVersion: 2, Mode: mode, PreferredProfile: preferred, FallbackProfile: "desktop_safe", Overrides: map[string]runtimeinfo.RuntimeProfileOverride{}}
	if fallback, ok := setting.Value["fallbackProfile"].(string); ok && runtimeinfo.ValidRuntimeProfile(fallback) && fallback != "auto" {
		config.FallbackProfile = fallback
	}
	override := runtimeinfo.RuntimeProfileOverride{}
	if value, exists := setting.Value["pauseWhenOnBattery"]; exists {
		parsed := boolFromJSON(value)
		override.PauseWhenOnBattery = &parsed
	}
	if value, exists := setting.Value["preventSleepDuringJobs"]; exists {
		parsed := boolFromJSON(value)
		override.PreventSleepDuringJobs = &parsed
	}
	var limits models.AppSetting
	if db.Where("key = ?", "schedulerLimits").Limit(1).Find(&limits).RowsAffected > 0 && !boolFromJSON(limits.Value["useProfileDefaults"]) {
		override.MaxRunningJobs = intPointer(limits.Value["maxRunningJobs"])
		override.MaxVideoJobs = intPointer(limits.Value["maxVideoJobs"])
		override.MaxSoftwareX265Jobs = intPointer(limits.Value["maxSoftwareX265Jobs"])
		override.MaxHardwareEncodeJobs = intPointer(limits.Value["maxHardwareEncodeJobs"])
		override.MaxAudioJobs = intPointer(limits.Value["maxAudioJobs"])
		override.MaxLabJobs = intPointer(limits.Value["maxLabJobs"])
		override.MinFreeRAMGB = int64Pointer(limits.Value["minFreeRamGb"])
		override.MinFreeWorkGB = int64Pointer(limits.Value["minFreeWorkGb"])
		override.MinFreeLibraryGB = int64Pointer(limits.Value["minFreeLibraryGb"])
		override.MaxWorkspaceGB = int64Pointer(limits.Value["maxWorkspaceGb"])
		if value, exists := limits.Value["allowDirectMode"]; exists {
			parsed := boolFromJSON(value)
			override.AllowDirectMode = &parsed
		}
	}
	config.Overrides[selected] = override
	encoded, err := json.Marshal(config)
	if err != nil {
		return err
	}
	var value models.JSONMap
	if err := json.Unmarshal(encoded, &value); err != nil {
		return err
	}
	setting.Value = value
	return db.Save(&setting).Error
}

func intFromJSON(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case int64:
		return int(v)
	}
	return 0
}
func boolFromJSON(value any) bool { parsed, _ := value.(bool); return parsed }
func intPointer(value any) *int {
	parsed := intFromJSON(value)
	if parsed <= 0 {
		return nil
	}
	return &parsed
}
func int64Pointer(value any) *int64 {
	parsed := int64(intFromJSON(value))
	if parsed <= 0 {
		return nil
	}
	return &parsed
}

func backfillQueueLifecycleStages(db *gorm.DB) error {
	var jobs []models.QueueJob
	if err := db.Find(&jobs).Error; err != nil {
		return err
	}
	for i := range jobs {
		job := &jobs[i]
		stage := "queued"
		if job.PublishedAt != nil {
			stage = "completed"
		} else {
			switch job.Status {
			case "running":
				stage = "converting"
			case "completed":
				stage = "ready_to_publish"
			case "failed":
				stage = "failed"
			case "canceled":
				stage = "canceled"
			}
		}
		if job.Stage == stage {
			continue
		}
		now := time.Now()
		if err := db.Model(job).Updates(map[string]any{"stage": stage, "stage_updated_at": &now}).Error; err != nil {
			return err
		}
	}
	return nil
}

func backfillPendingExecutionPlans(db *gorm.DB) error {
	var jobs []models.QueueJob
	if err := db.Where("active_execution_plan_id IS NULL").Find(&jobs).Error; err != nil {
		return err
	}
	for i := range jobs {
		job := &jobs[i]
		if len(job.ProfileSnapshot) == 0 {
			continue
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			_, err := scheduler.CreatePendingExecutionPlan(tx, job, "Execution plan created from migration backfill")
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}

func backfillQueueProfileSnapshots(db *gorm.DB) error {
	var jobs []models.QueueJob
	if err := db.Where("profile_snapshot IS NULL OR profile_snapshot = ? OR profile_version <= 0", "{}").Find(&jobs).Error; err != nil {
		return err
	}
	for i := range jobs {
		job := &jobs[i]
		var profile models.Profile
		// Historical jobs may point to a soft-deleted profile. Recover that
		// contract when possible; Find avoids logging an expected not-found as
		// a database error when the profile was permanently removed.
		result := db.Unscoped().Where("id = ?", job.ProfileID).Limit(1).Find(&profile)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			continue
		}
		now := time.Now()
		snapshot, err := scheduler.CaptureProfileSnapshot(profile, now, "migration_backfill")
		if err != nil {
			return err
		}
		job.ProfileVersion = max(profile.ProfileVersion, 1)
		job.ProfileSnapshot = snapshot
		job.ProfileCapturedAt = &now
		if err := db.Save(job).Error; err != nil {
			return err
		}
	}
	return nil
}

func normalizeProfileContracts(db *gorm.DB) error {
	var profiles []models.Profile
	if err := db.Find(&profiles).Error; err != nil {
		return err
	}
	for i := range profiles {
		profile := &profiles[i]
		if profile.CodecFamily != "" && profile.EncoderPolicy != "" && len(profile.AllowedEncoders) > 0 {
			continue
		}
		if err := scheduler.ApplyAuthoritativeContract(profile); err != nil {
			return err
		}
		if err := db.Save(profile).Error; err != nil {
			return err
		}
	}
	return nil
}

func Seed(db *gorm.DB) error {
	var count int64
	if err := db.Model(&models.Profile{}).Count(&count).Error; err != nil {
		return err
	}

	if count > 0 {
		if err := seedRecommendedProfiles(db); err != nil {
			return err
		}
		if err := seedSettings(db); err != nil {
			return err
		}
		if err := seedStorageRolesFromPaths(db); err != nil {
			return err
		}
		return normalizeProfileContracts(db)
	}

	profiles := []models.Profile{
		{
			Name:              "DVD Archive x265 Main10",
			Description:       "Default DVD profile for clean SD sources. Preserves all audio tracks, subtitles, chapters, and targets a compact high-quality x265 Main10 output.",
			Container:         "mkv",
			VideoCodec:        "x265_10bit",
			AudioCodec:        "copy",
			QualityMode:       "crf",
			QualityValue:      20,
			PreserveHDR:       false,
			PreserveSubtitles: true,
			PreserveChapters:  true,
			WorkerConfig: models.JSONMap{
				"source":         "mvforge-v1",
				"engine":         "FFmpeg",
				"preset":         "dvd-archive-main10",
				"videoPreset":    "medium",
				"pixFmt":         "yuv420p10le",
				"x265Params":     "aq-mode=3:aq-strength=0.8:deblock=-1,-1",
				"processingMode": "full_encode",
			},
		},
		{
			Name:              "Anime DVD x265 Main10",
			Description:       "Default anime DVD profile. Preserves all audio tracks, subtitles, chapters, and is tuned for line art with balanced size.",
			Container:         "mkv",
			VideoCodec:        "x265_10bit",
			AudioCodec:        "copy",
			QualityMode:       "crf",
			QualityValue:      20,
			PreserveHDR:       false,
			PreserveSubtitles: true,
			PreserveChapters:  true,
			WorkerConfig: models.JSONMap{
				"source":         "mvforge-v1",
				"engine":         "FFmpeg",
				"preset":         "anime-dvd-main10",
				"videoPreset":    "medium",
				"pixFmt":         "yuv420p10le",
				"x265Params":     "aq-mode=3:aq-strength=0.9:deblock=-1,-1",
				"processingMode": "full_encode",
			},
		},
		{
			Name:              "Series Balanced x265 Main10",
			Description:       "Default profile for episode batches and series. Preserves all audio tracks, subtitles, chapters, and is slightly smaller than DVD/Anime defaults.",
			Container:         "mkv",
			VideoCodec:        "x265_10bit",
			AudioCodec:        "copy",
			QualityMode:       "crf",
			QualityValue:      21,
			PreserveHDR:       false,
			PreserveSubtitles: true,
			PreserveChapters:  true,
			WorkerConfig: models.JSONMap{
				"source":         "mvforge-v1",
				"engine":         "FFmpeg",
				"preset":         "series-balanced-main10",
				"videoPreset":    "medium",
				"pixFmt":         "yuv420p10le",
				"x265Params":     "aq-mode=3:aq-strength=0.75:deblock=-1,-1",
				"processingMode": "full_encode",
			},
		},
	}

	if err := db.Create(&profiles).Error; err != nil {
		return err
	}

	if err := seedRecommendedProfiles(db); err != nil {
		return err
	}

	if err := seedSettings(db); err != nil {
		return err
	}
	if err := seedStorageRolesFromPaths(db); err != nil {
		return err
	}
	return normalizeProfileContracts(db)
}

func seedRecommendedProfiles(db *gorm.DB) error {
	profiles := []models.Profile{
		{
			Name:              "HEVC Small Size",
			Description:       "Software x265 profile for assets where saving space matters more than speed.",
			Container:         "mkv",
			VideoCodec:        "x265_10bit",
			AudioCodec:        "copy",
			QualityMode:       "crf",
			QualityValue:      21,
			PreserveHDR:       true,
			PreserveSubtitles: true,
			PreserveChapters:  true,
			WorkerConfig: models.JSONMap{
				"source":                 "mvforge-v1",
				"engine":                 "FFmpeg",
				"preset":                 "hevc-small-size",
				"preferredEncoder":       "software",
				"videoEncoder":           "libx265",
				"useHardwareIfAvailable": false,
				"videoPreset":            "slow",
				"pixFmt":                 "yuv420p10le",
				"x265Params":             "aq-mode=3:aq-strength=0.8:deblock=-1,-1",
				"processingMode":         "full_encode",
				"addAacStereoTrack":      true,
				"aacStereoDefault":       false,
				"preserveOriginalAudio":  true,
				"warnSubtitleFormats":    true,
				"preferSrtSubtitles":     false,
			},
		},
		{
			Name:              "HEVC Balanced Fast",
			Description:       "Hardware HEVC profile for faster queues and lower NAS pressure.",
			Container:         "mkv",
			VideoCodec:        "x265_10bit",
			AudioCodec:        "copy",
			QualityMode:       "crf",
			QualityValue:      20,
			PreserveHDR:       true,
			PreserveSubtitles: true,
			PreserveChapters:  true,
			WorkerConfig: models.JSONMap{
				"source":                 "mvforge-v1",
				"engine":                 "FFmpeg",
				"preset":                 "hevc-balanced-fast",
				"preferredEncoder":       "hardware",
				"videoEncoder":           "hevc_qsv",
				"useHardwareIfAvailable": true,
				"globalQuality":          18,
				"qsvRateControl":         "icq",
				"qsvLookAheadDepth":      40,
				"qsvExtendedBRC":         false,
				"qsvAdaptiveI":           true,
				"qsvAdaptiveB":           true,
				"videoPreset":            "medium",
				"pixFmt":                 "p010le",
				"processingMode":         "full_encode",
				"addAacStereoTrack":      true,
				"aacStereoDefault":       false,
				"preserveOriginalAudio":  true,
				"warnSubtitleFormats":    true,
				"preferSrtSubtitles":     false,
			},
		},
		{
			Name:              "HEVC Archive Quality",
			Description:       "Software x265 profile for important movies, concerts, and difficult sources.",
			Container:         "mkv",
			VideoCodec:        "x265_10bit",
			AudioCodec:        "copy",
			QualityMode:       "crf",
			QualityValue:      19,
			PreserveHDR:       true,
			PreserveSubtitles: true,
			PreserveChapters:  true,
			WorkerConfig: models.JSONMap{
				"source":                 "mvforge-v1",
				"engine":                 "FFmpeg",
				"preset":                 "hevc-archive-quality",
				"preferredEncoder":       "software",
				"videoEncoder":           "libx265",
				"useHardwareIfAvailable": false,
				"videoPreset":            "slow",
				"pixFmt":                 "yuv420p10le",
				"x265Params":             "aq-mode=3:aq-strength=0.9:deblock=-1,-1",
				"processingMode":         "full_encode",
				"addAacStereoTrack":      true,
				"aacStereoDefault":       false,
				"preserveOriginalAudio":  true,
				"warnSubtitleFormats":    true,
				"preferSrtSubtitles":     false,
			},
		},
		{
			Name:              "HEVC Bulk Convert",
			Description:       "Hardware HEVC profile for large libraries and unattended bulk conversion.",
			Container:         "mkv",
			VideoCodec:        "x265_10bit",
			AudioCodec:        "copy",
			QualityMode:       "crf",
			QualityValue:      23,
			PreserveHDR:       true,
			PreserveSubtitles: true,
			PreserveChapters:  true,
			WorkerConfig: models.JSONMap{
				"source":                 "mvforge-v1",
				"engine":                 "FFmpeg",
				"preset":                 "hevc-bulk-convert",
				"preferredEncoder":       "hardware",
				"videoEncoder":           "hevc_qsv",
				"useHardwareIfAvailable": true,
				"globalQuality":          21,
				"qsvRateControl":         "icq",
				"qsvLookAheadDepth":      40,
				"qsvExtendedBRC":         false,
				"qsvAdaptiveI":           true,
				"qsvAdaptiveB":           false,
				"videoPreset":            "medium",
				"pixFmt":                 "p010le",
				"processingMode":         "full_encode",
				"addAacStereoTrack":      true,
				"aacStereoDefault":       false,
				"preserveOriginalAudio":  true,
				"warnSubtitleFormats":    true,
				"preferSrtSubtitles":     false,
			},
		},
	}

	for _, profile := range profiles {
		if err := db.Where(models.Profile{Name: profile.Name}).FirstOrCreate(&profile).Error; err != nil {
			return err
		}
	}
	return nil
}

func seedSettings(db *gorm.DB) error {
	defaults := []models.AppSetting{
		{
			Key: "paths",
			Value: models.JSONMap{
				"rawRoot":              "/media/raw",
				"libraryRoot":          "/media/library",
				"stagingPath":          "/media/staging",
				"originalsArchivePath": "/media/originals_archive",
				"asIsReportsPath":      "/media/reports/as-is",
				"resultsReportsPath":   "/media/reports/results",
				"logsPath":             "/media/reports/logs",
			},
		},
		{
			Key: "workers",
			Value: models.JSONMap{
				"defaultWorkerName":       "local-worker",
				"autoWorkerEnabled":       true,
				"maxConcurrentJobs":       1,
				"maxJobsPerBatch":         10,
				"delaySecondsBetweenJobs": 30,
				"batchCooldownSeconds":    600,
				"dryRunOnly":              true,
			},
		},
		{
			Key: "pipelineAutomation",
			Value: models.JSONMap{
				"autoAnalysisEnabled":               false,
				"reviewMode":                        "conditional",
				"autoExecutionEnabled":              true,
				"autoValidationEnabled":             false,
				"autoPublisherEnabled":              false,
				"publishedJobReconciliationEnabled": false,
			},
		},
		{
			Key: "runtimePolicy",
			Value: models.JSONMap{
				"schemaVersion":    2,
				"mode":             "automatic",
				"preferredProfile": "auto",
				"fallbackProfile":  "desktop_safe",
				"overrides":        models.JSONMap{},
			},
		},
		{
			Key: "workingHours",
			Value: models.JSONMap{"enabled": false, "timezone": "America/Mexico_City", "preset": "disabled", "windows": []models.JSONMap{}, "outsideWindowPolicy": models.JSONMap{
				"startNewHeavyJobs": true, "continueRunningJobs": true, "allowAnalysisJobs": true, "allowValidationJobs": true,
				"allowPublisherJobs": true, "allowCleanupJobs": true, "allowLabPreviews": true,
			}},
		},
		{
			Key:   "workspace",
			Value: models.JSONMap{"preferredMode": "copy_to_work_disk", "fallbackMode": "wait", "allowDirectMode": false, "estimateRequiredSpace": true},
		},
		{
			Key: "directPlay",
			Value: models.JSONMap{
				"enabled": true, "strategy": "balanced", "targetClients": []string{"jellyfin_web", "jellyfin_android_tv", "jellyfin_roku", "jellyfin_webos", "apple_tv"},
				"minimumScore": 70, "enforcement": "warn",
			},
		},
		{
			Key:   "housekeeping",
			Value: models.JSONMap{"autoEnabled": true, "intervalHours": 24, "failedRetentionDays": 7, "canceledRetentionDays": 3, "orphanRetentionDays": 7},
		},
		{
			Key: "validation",
			Value: models.JSONMap{
				"minimumScore":         90,
				"requireDurationMatch": true,
			},
		},
		{
			Key: "cancellationPolicy",
			Value: models.JSONMap{
				"keepLogsAndDiagnostics":         true,
				"deleteGeneratedFiles":           false,
				"deletePartialOutputFromStaging": false,
				"controlledRoots": []string{
					"/media/staging",
					"/mwp/work",
					"/mwp/work/temp",
					"/tmp/mvforge",
				},
			},
		},
		{
			Key: "originalRetentionPolicy",
			Value: models.JSONMap{
				"keepOriginalsDays":                   30,
				"enabledForSuccessfulConversionsOnly": true,
				"autoDeleteEnabled":                   false,
				"processedOriginalsPath":              "/media/originals_archive/processed-originals",
			},
		},
		{
			Key: "assetInventory",
			Value: models.JSONMap{
				"autoSyncEnabled":     true,
				"syncIntervalMinutes": 60,
				"expireArchiveFiles":  true,
				"reconciliationMode":  "exact",
			},
		},
		{
			Key: "audioEnhancementProfiles",
			Value: models.JSONMap{
				"profiles": []models.JSONMap{
					{
						"key":                   "gentle-normalize",
						"name":                  "Gentle Normalize",
						"description":           "Safely evens out quiet or inconsistent audio without changing the character too much.",
						"intent":                "Low-risk loudness normalization",
						"filters":               "loudnorm=I=-18:TP=-2:LRA=11",
						"rnnoiseModelPath":      "",
						"channelMode":           "preserve",
						"forceStereoMode":       "auto",
						"stereoDelayMs":         12,
						"stereoWidth":           20,
						"preserveOriginalTrack": true,
						"outputCodec":           "aac",
						"targetLoudness":        -18,
						"truePeak":              -2,
						"notes":                 "Good first pass for TV recordings, DVD rips, and uneven episode batches.",
					},
					{
						"key":                   "dialogue-clarity",
						"name":                  "Dialogue Clarity",
						"description":           "Adds gentle voice presence and compression for dialogue-heavy older sources.",
						"intent":                "Make voices easier to understand",
						"filters":               "highpass=f=80,equalizer=f=1800:t=q:w=1.1:g=2.5,acompressor=threshold=-20dB:ratio=2.2:attack=20:release=250,loudnorm=I=-18:TP=-2:LRA=9",
						"rnnoiseModelPath":      "",
						"channelMode":           "preserve",
						"forceStereoMode":       "auto",
						"stereoDelayMs":         12,
						"stereoWidth":           20,
						"preserveOriginalTrack": true,
						"outputCodec":           "aac",
						"targetLoudness":        -18,
						"truePeak":              -2,
						"notes":                 "Useful for old anime dubs, TV captures, and sources where voices sit behind music/effects.",
					},
					{
						"key":                   "old-source-cleanup",
						"name":                  "Old Source Cleanup",
						"description":           "Reduces light hiss/noise, trims rumble, and normalizes loudness for older TV/anime audio.",
						"intent":                "Cleanup for aged stereo or mono sources",
						"filters":               "highpass=f=70,lowpass=f=15000,afftdn=nf=-25,acompressor=threshold=-22dB:ratio=2:attack=25:release=300,loudnorm=I=-18:TP=-2:LRA=10",
						"rnnoiseModelPath":      "",
						"channelMode":           "preserve",
						"forceStereoMode":       "auto",
						"stereoDelayMs":         12,
						"stereoWidth":           20,
						"preserveOriginalTrack": true,
						"outputCodec":           "aac",
						"targetLoudness":        -18,
						"truePeak":              -2,
						"notes":                 "More aggressive. Preview before applying because denoise filters can create artifacts.",
					},
					{
						"key":                   "speech-neural-denoise",
						"name":                  "Speech Neural Denoise",
						"description":           "Uses FFmpeg arnndn with an external speech denoise model, then normalizes loudness.",
						"intent":                "Reduce speech noise with a neural model",
						"filters":               "loudnorm=I=-18:TP=-2:LRA=10",
						"rnnoiseModelPath":      "/mvforge/models/audio/rnnoise-model.rnnn",
						"channelMode":           "preserve",
						"forceStereoMode":       "auto",
						"stereoDelayMs":         12,
						"stereoWidth":           20,
						"preserveOriginalTrack": true,
						"outputCodec":           "aac",
						"targetLoudness":        -18,
						"truePeak":              -2,
						"notes":                 "Requires a valid arnndn/RNNoise model path mounted inside the backend container. Test before batch use.",
					},
					{
						"key":                   "hiss-reduction-light",
						"name":                  "Hiss Reduction Light",
						"description":           "Gently reduces high-frequency hiss while keeping ambience and music mostly intact.",
						"intent":                "Light hiss cleanup",
						"filters":               "highpass=f=55,lowpass=f=17000,afftdn=nf=-20,loudnorm=I=-18:TP=-2:LRA=11",
						"rnnoiseModelPath":      "",
						"channelMode":           "preserve",
						"forceStereoMode":       "auto",
						"stereoDelayMs":         12,
						"stereoWidth":           20,
						"preserveOriginalTrack": true,
						"outputCodec":           "aac",
						"targetLoudness":        -18,
						"truePeak":              -2,
						"notes":                 "Start here for music, concerts, and sources where noise reduction should be subtle.",
					},
					{
						"key":                   "hiss-reduction-medium",
						"name":                  "Hiss Reduction Medium",
						"description":           "Moderate hiss reduction for TV, tape, and older stereo or mono sources.",
						"intent":                "Balanced hiss cleanup",
						"filters":               "highpass=f=65,lowpass=f=15500,afftdn=nf=-24,acompressor=threshold=-22dB:ratio=1.8:attack=25:release=250,loudnorm=I=-18:TP=-2:LRA=10",
						"rnnoiseModelPath":      "",
						"channelMode":           "preserve",
						"forceStereoMode":       "auto",
						"stereoDelayMs":         12,
						"stereoWidth":           20,
						"preserveOriginalTrack": true,
						"outputCodec":           "aac",
						"targetLoudness":        -18,
						"truePeak":              -2,
						"notes":                 "Good default for old TV/anime audio. Preview for watery artifacts before batch use.",
					},
					{
						"key":                   "hiss-reduction-strong",
						"name":                  "Hiss Reduction Strong",
						"description":           "Aggressive hiss cleanup for noisy spoken material where artifacts are acceptable.",
						"intent":                "Strong hiss cleanup",
						"filters":               "highpass=f=80,lowpass=f=13500,afftdn=nf=-32,acompressor=threshold=-24dB:ratio=2.4:attack=20:release=300,loudnorm=I=-18:TP=-2:LRA=9",
						"rnnoiseModelPath":      "",
						"channelMode":           "preserve",
						"forceStereoMode":       "auto",
						"stereoDelayMs":         12,
						"stereoWidth":           20,
						"preserveOriginalTrack": true,
						"outputCodec":           "aac",
						"targetLoudness":        -18,
						"truePeak":              -2,
						"notes":                 "Use for dialogue-first sources. Strong denoise can damage music, ambience, and cymbals.",
					},
					{
						"key":                   "old-mono-cleanup",
						"name":                  "Old Mono Cleanup",
						"description":           "Centers old mono-style sources, reduces rumble/hiss, and normalizes loudness.",
						"intent":                "Mono restoration",
						"filters":               "highpass=f=80,lowpass=f=14500,afftdn=nf=-24,acompressor=threshold=-22dB:ratio=2:attack=25:release=280,loudnorm=I=-18:TP=-2:LRA=10",
						"rnnoiseModelPath":      "",
						"channelMode":           "dual-mono",
						"forceStereoMode":       "auto",
						"stereoDelayMs":         12,
						"stereoWidth":           20,
						"preserveOriginalTrack": true,
						"outputCodec":           "aac",
						"targetLoudness":        -18,
						"truePeak":              -2,
						"notes":                 "Best for old mono dubs, VHS/DVD extras, and sources that should play evenly on both speakers.",
					},
					{
						"key":                   "old-anime-dialogue-cleanup",
						"name":                  "Old Anime Dialogue Cleanup",
						"description":           "Raises dialogue presence while gently reducing hiss and low-end rumble.",
						"intent":                "Old anime and TV dialogue",
						"filters":               "highpass=f=75,lowpass=f=15000,afftdn=nf=-23,equalizer=f=1800:t=q:w=1.1:g=2,acompressor=threshold=-21dB:ratio=2.1:attack=18:release=260,loudnorm=I=-18:TP=-2:LRA=9",
						"rnnoiseModelPath":      "",
						"channelMode":           "preserve",
						"forceStereoMode":       "auto",
						"stereoDelayMs":         12,
						"stereoWidth":           20,
						"preserveOriginalTrack": true,
						"outputCodec":           "aac",
						"targetLoudness":        -18,
						"truePeak":              -2,
						"notes":                 "Designed for old anime/TV dubs where voices need help but music should not be crushed.",
					},
					{
						"key":                   "concert-pcm-preserve",
						"name":                  "Concert PCM Preserve",
						"description":           "Keeps concert dynamics mostly intact with only light cleanup and safe loudness.",
						"intent":                "Concert and PCM preservation",
						"filters":               "highpass=f=40,lowpass=f=19000,loudnorm=I=-19:TP=-2:LRA=14",
						"rnnoiseModelPath":      "",
						"channelMode":           "preserve",
						"forceStereoMode":       "auto",
						"stereoDelayMs":         12,
						"stereoWidth":           20,
						"preserveOriginalTrack": true,
						"outputCodec":           "flac",
						"targetLoudness":        -19,
						"truePeak":              -2,
						"notes":                 "Good for concerts and PCM tracks. Avoid heavy denoise unless a sample proves it is needed.",
					},
					{
						"key":                   "mono-dual-mono-safe",
						"name":                  "Mono to Dual Mono Safe",
						"description":           "Duplicates a mono source into left and right channels without fake stereo widening.",
						"intent":                "Safe mono compatibility",
						"filters":               "loudnorm=I=-18:TP=-2:LRA=11",
						"rnnoiseModelPath":      "",
						"channelMode":           "dual-mono",
						"forceStereoMode":       "auto",
						"stereoDelayMs":         12,
						"stereoWidth":           20,
						"preserveOriginalTrack": true,
						"outputCodec":           "aac",
						"targetLoudness":        -18,
						"truePeak":              -2,
						"notes":                 "Use for mono tracks that should play evenly on stereo devices. This does not create true stereo.",
					},
					{
						"key":                   "downmix-mono-safe",
						"name":                  "Downmix to Mono Safe",
						"description":           "Mixes stereo or multichannel audio into one centered mono track.",
						"intent":                "Mono compatibility",
						"filters":               "loudnorm=I=-18:TP=-2:LRA=11",
						"rnnoiseModelPath":      "",
						"channelMode":           "downmix-mono",
						"forceStereoMode":       "auto",
						"stereoDelayMs":         12,
						"stereoWidth":           20,
						"preserveOriginalTrack": true,
						"outputCodec":           "aac",
						"targetLoudness":        -18,
						"truePeak":              -2,
						"notes":                 "Useful when stereo image or phase problems make old sources clearer as a centered mono track.",
					},
					{
						"key":                   "mono-light-stereo-experimental",
						"name":                  "Mono to Light Stereo Experimental",
						"description":           "Creates a subtle pseudo-stereo image from mono using a small delay and widening.",
						"intent":                "Experimental mono widening",
						"filters":               "loudnorm=I=-18:TP=-2:LRA=11",
						"rnnoiseModelPath":      "",
						"channelMode":           "light-stereo",
						"forceStereoMode":       "auto",
						"stereoDelayMs":         12,
						"stereoWidth":           20,
						"preserveOriginalTrack": true,
						"outputCodec":           "aac",
						"targetLoudness":        -18,
						"truePeak":              -2,
						"notes":                 "Preview carefully. Pseudo-stereo can make dialogue or old dubs sound phasey if pushed too far.",
					},
				},
			},
		},
		{
			Key: "assetTypes",
			Value: models.JSONMap{
				"types": []models.JSONMap{
					{
						"key":        "movies",
						"label":      "Movies",
						"extensions": []string{".mkv", ".mp4", ".avi", ".mov"},
					},
					{
						"key":        "tv",
						"label":      "TV Shows",
						"extensions": []string{".mkv", ".mp4"},
					},
					{
						"key":        "anime",
						"label":      "Anime",
						"extensions": []string{".mkv", ".mp4"},
					},
					{
						"key":        "music-videos",
						"label":      "Music Videos",
						"extensions": []string{".mkv", ".mp4", ".mov"},
					},
					{
						"key":        "concerts",
						"label":      "Concerts",
						"extensions": []string{".mkv", ".mp4"},
					},
					{
						"key":        "home-videos",
						"label":      "Home Videos",
						"extensions": []string{".mp4", ".mov"},
					},
				},
			},
		},
		{
			Key: "assetCategories",
			Value: models.JSONMap{
				"categories": []string{"movie", "anime", "series", "episode", "season", "extras", "special", "concert", "music-video", "documentary"},
			},
		},
	}

	for _, setting := range defaults {
		if err := db.FirstOrCreate(&setting, models.AppSetting{Key: setting.Key}).Error; err != nil {
			return err
		}
	}

	return nil
}

func seedStorageRolesFromPaths(db *gorm.DB) error {
	var existing int64
	if err := db.Model(&models.AppSetting{}).Where("key = ?", "storageRoles").Count(&existing).Error; err != nil {
		return err
	}
	if existing > 0 {
		return nil
	}
	var paths models.AppSetting
	if err := db.Where("key = ?", "paths").First(&paths).Error; err != nil {
		return err
	}
	pathValue := func(key, fallback string) string {
		if value, ok := paths.Value[key].(string); ok && value != "" {
			return value
		}
		return fallback
	}
	reports := pathValue("resultsReportsPath", "/media/reports")
	roles := models.JSONMap{
		"raw": models.JSONMap{"path": pathValue("rawRoot", "/media/raw")}, "library": models.JSONMap{"path": pathValue("libraryRoot", "/media/library")},
		"originals_archive": models.JSONMap{"path": pathValue("originalsArchivePath", "/media/originals_archive")}, "work": models.JSONMap{"path": pathValue("stagingPath", "/media/staging")},
		"cache": models.JSONMap{"path": filepath.Join(filepath.Dir(reports), "cache")}, "reports": models.JSONMap{"path": reports}, "logs": models.JSONMap{"path": pathValue("logsPath", "/media/reports/logs")},
	}
	return db.Create(&models.AppSetting{Key: "storageRoles", Value: roles}).Error
}
