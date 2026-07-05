package migrations

import (
	"time"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"gorm.io/gorm"
)

type MWPSummary struct {
	ProfilesCreated     int      `json:"profilesCreated"`
	ProfilesUpdated     int      `json:"profilesUpdated"`
	LibraryTypesCreated int      `json:"libraryTypesCreated"`
	LibraryTypesUpdated int      `json:"libraryTypesUpdated"`
	SettingsUpdated     []string `json:"settingsUpdated"`
}

type mwpProfile struct {
	Key       string
	Name      string
	Use       string
	Quality   int
	Preset    string
	VideoArgs string
	Filters   string
	HDR       bool
}

type mwpLibraryType struct {
	Key         string
	Label       string
	Description string
	Extensions  []string
	PendingDir  string
	Destination string
}

const (
	mwpAudioArgs    = "--all-audio --aencoder copy --audio-copy-mask aac,ac3,eac3,truehd,dts,dtshd,mp3,flac --audio-fallback av_aac"
	mwpSubtitleArgs = "--all-subtitles"
)

func ImportMWP(db *gorm.DB) (MWPSummary, error) {
	summary := MWPSummary{}

	created, updated, err := upsertMWPProfiles(db)
	if err != nil {
		return summary, err
	}
	summary.ProfilesCreated = created
	summary.ProfilesUpdated = updated

	created, updated, err = upsertMWPLibraryTypes(db)
	if err != nil {
		return summary, err
	}
	summary.LibraryTypesCreated = created
	summary.LibraryTypesUpdated = updated

	if err := upsertMWPSettings(db); err != nil {
		return summary, err
	}
	summary.SettingsUpdated = []string{"mwp", "workers", "assetTypes"}

	return summary, nil
}

func upsertMWPProfiles(db *gorm.DB) (int, int, error) {
	created := 0
	updated := 0

	for _, source := range mwpProfiles() {
		profile := models.Profile{
			Name:              source.Name,
			Description:       source.Use,
			Container:         "mkv",
			VideoCodec:        "x265_10bit",
			AudioCodec:        "copy",
			QualityMode:       "rf",
			QualityValue:      source.Quality,
			PreserveHDR:       source.HDR,
			PreserveSubtitles: true,
			PreserveChapters:  true,
			WorkerConfig: models.JSONMap{
				"source":                "media-worker-pipeline",
				"engine":                "HandBrakeCLI",
				"mwpProfile":            source.Key,
				"handBrakeEncoder":      "x265_10bit",
				"handBrakePreset":       source.Preset,
				"handBrakeVideoArgs":    source.VideoArgs,
				"handBrakeFilterArgs":   source.Filters,
				"handBrakeAudioArgs":    mwpAudioArgs,
				"handBrakeSubtitleArgs": mwpSubtitleArgs,
				"notes":                 "Migrated from MWP scripts/config.sh.",
			},
		}

		var existing models.Profile
		err := db.First(&existing, "name = ?", profile.Name).Error
		if err != nil {
			if err != gorm.ErrRecordNotFound {
				return created, updated, err
			}
			if err := db.Create(&profile).Error; err != nil {
				return created, updated, err
			}
			created++
			continue
		}

		existing.Description = profile.Description
		existing.Container = profile.Container
		existing.VideoCodec = profile.VideoCodec
		existing.AudioCodec = profile.AudioCodec
		existing.QualityMode = profile.QualityMode
		existing.QualityValue = profile.QualityValue
		existing.PreserveHDR = profile.PreserveHDR
		existing.PreserveSubtitles = profile.PreserveSubtitles
		existing.PreserveChapters = profile.PreserveChapters
		existing.WorkerConfig = profile.WorkerConfig
		if err := db.Save(&existing).Error; err != nil {
			return created, updated, err
		}
		updated++
	}

	return created, updated, nil
}

func upsertMWPLibraryTypes(db *gorm.DB) (int, int, error) {
	setting, err := findSetting(db, "assetTypes")
	if err != nil {
		return 0, 0, err
	}

	existingTypes, _ := setting.Value["types"].([]any)
	byKey := map[string]models.JSONMap{}
	order := []string{}
	for _, item := range existingTypes {
		if typed, ok := item.(map[string]any); ok {
			entry := models.JSONMap(typed)
			key, _ := entry["key"].(string)
			if key != "" {
				byKey[key] = entry
				order = append(order, key)
			}
		}
	}

	created := 0
	updated := 0
	for _, source := range mwpLibraryTypes() {
		entry := models.JSONMap{
			"key":         source.Key,
			"label":       source.Label,
			"description": source.Description,
			"extensions":  source.Extensions,
			"source":      "media-worker-pipeline",
			"pendingDir":  source.PendingDir,
			"destination": source.Destination,
		}
		if _, exists := byKey[source.Key]; exists {
			updated++
		} else {
			created++
			order = append(order, source.Key)
		}
		byKey[source.Key] = entry
	}

	types := make([]models.JSONMap, 0, len(order))
	seen := map[string]bool{}
	for _, key := range order {
		if seen[key] {
			continue
		}
		if entry, ok := byKey[key]; ok {
			types = append(types, entry)
			seen[key] = true
		}
	}

	setting.Value = models.JSONMap{"types": types}
	if setting.CreatedAt.IsZero() {
		setting.CreatedAt = time.Now()
	}
	if err := db.Save(&setting).Error; err != nil {
		return 0, 0, err
	}

	return created, updated, nil
}

func upsertMWPSettings(db *gorm.DB) error {
	mwpSetting, err := findSetting(db, "mwp")
	if err != nil {
		return err
	}
	mwpSetting.Value = models.JSONMap{
		"version":             "1.0",
		"defaultProfile":      "dvd",
		"dvdWorkers":          1,
		"mkvWorkers":          1,
		"scanIntervalSeconds": 30,
		"maxRetries":          3,
		"minimumFreeGb":       50,
		"processingExtension": "_MWP",
		"root":                "/mwp",
		"stateRoot":           "/state",
		"pendingRoot":         "/state/pending",
		"processingRoot":      "/state/processing",
		"completedRoot":       "/state/completed",
		"failedRoot":          "/state/failed",
		"reviewRoot":          "/state/review",
		"workRoot":            "/mwp/work",
		"reportsRoot":         "/mwp/reports",
		"conversionReports":   "/mwp/reports/conversions",
	}
	if mwpSetting.CreatedAt.IsZero() {
		mwpSetting.CreatedAt = time.Now()
	}
	if err := db.Save(&mwpSetting).Error; err != nil {
		return err
	}

	workers, err := findSetting(db, "workers")
	if err != nil {
		return err
	}
	workers.Value["maxRetries"] = 3
	workers.Value["scanIntervalSeconds"] = 30
	workers.Value["mwpMkvWorkers"] = 1
	workers.Value["mwpDvdWorkers"] = 1
	if err := db.Save(&workers).Error; err != nil {
		return err
	}

	return nil
}

func findSetting(db *gorm.DB, key string) (models.AppSetting, error) {
	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", key).Error; err != nil {
		if err != gorm.ErrRecordNotFound {
			return setting, err
		}
		setting = models.AppSetting{
			Key:       key,
			Value:     models.JSONMap{},
			CreatedAt: time.Now(),
		}
	}
	if setting.Value == nil {
		setting.Value = models.JSONMap{}
	}
	return setting, nil
}

func mwpProfiles() []mwpProfile {
	return []mwpProfile{
		{"dvd-small", "MWP DVD Small", "DVD sources with stronger space savings.", 22, "medium", "--crop 0:0:0:0 --loose-anamorphic", "--comb-detect --decomb", false},
		{"movie-small", "MWP Movie Small", "Movie sources with stronger space savings.", 22, "medium", "--crop 0:0:0:0 --loose-anamorphic", "--comb-detect --decomb", false},
		{"dvd", "MWP DVD", "Default high quality profile for DVD sources.", 18, "medium", "--crop 0:0:0:0 --loose-anamorphic", "--comb-detect --decomb", false},
		{"movie", "MWP Movie", "Default high quality profile for movie sources.", 18, "medium", "--crop 0:0:0:0 --loose-anamorphic", "--comb-detect --decomb", false},
		{"dvd-hq", "MWP DVD HQ", "Important DVD sources with slower compression.", 18, "slow", "--crop 0:0:0:0 --loose-anamorphic", "--comb-detect --decomb", false},
		{"movie-hq", "MWP Movie HQ", "Important movies with slower compression.", 18, "slow", "--crop 0:0:0:0 --loose-anamorphic", "--comb-detect --decomb", false},
		{"anime-small", "MWP Anime Small", "Anime sources with stronger space savings.", 20, "medium", "--crop 0:0:0:0 --loose-anamorphic", "--comb-detect --decomb", false},
		{"anime", "MWP Anime", "Default high quality profile for anime.", 18, "medium", "--crop 0:0:0:0 --loose-anamorphic", "--comb-detect --decomb", false},
		{"anime-hq", "MWP Anime HQ", "Important anime sources with cleaner line art.", 17, "slow", "--crop 0:0:0:0 --loose-anamorphic", "--comb-detect --decomb", false},
		{"series-small", "MWP Series Small", "Episode libraries with stronger space savings.", 22, "medium", "--crop 0:0:0:0 --loose-anamorphic", "--comb-detect --decomb", false},
		{"series", "MWP Series", "Default profile for episode libraries.", 20, "medium", "--crop 0:0:0:0 --loose-anamorphic", "--comb-detect --decomb", false},
		{"series-hq", "MWP Series HQ", "Important episodes or short series.", 18, "medium", "--crop 0:0:0:0 --loose-anamorphic", "--comb-detect --decomb", false},
		{"blu-ray", "MWP Blu-ray", "Prepared for Blu-ray sources.", 21, "slow", "--crop 0:0:0:0", "", true},
		{"4k", "MWP 4K", "Prepared for UHD and 4K sources.", 22, "slow", "--crop 0:0:0:0", "", true},
	}
}

func mwpLibraryTypes() []mwpLibraryType {
	return []mwpLibraryType{
		{"movies", "Movies", "Feature films and movie collections.", []string{".mkv"}, "movies", "/media/movies"},
		{"anime", "Anime", "Anime series and episode libraries.", []string{".mkv"}, "anime", "/media/anime"},
		{"anime-movies", "Anime Movies", "Standalone anime films.", []string{".mkv"}, "anime-movies", "/media/anime-movies"},
		{"series", "Series", "Episode-based TV libraries.", []string{".mkv"}, "series", "/media/series"},
		{"concerts", "Concerts", "Live performances and concert films.", []string{".mkv"}, "concerts", "/media/concerts"},
		{"documentaries", "Documentaries", "Documentary films and docuseries.", []string{".mkv"}, "documentaries", "/media/documentaries"},
		{"videos", "Videos", "General purpose video libraries.", []string{".mkv", ".mp4", ".mov"}, "videos", "/media/videos"},
	}
}
