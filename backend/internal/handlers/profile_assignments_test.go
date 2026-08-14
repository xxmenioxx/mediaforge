package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestProfileAssignmentsPreferAssetOverPathPerMediaType(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:profile-assignment-priority?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AssetRecord{}, &models.ProfileAssignment{}); err != nil {
		t.Fatal(err)
	}
	assetPath := "/media/raw/anime/Show/episode-02.mkv"
	if err := db.Create(&models.AssetRecord{Path: assetPath, RootPath: "/media/raw/anime", GroupPath: "Show", FileName: "episode-02.mkv"}).Error; err != nil {
		t.Fatal(err)
	}
	assignments := []models.ProfileAssignment{
		{TargetType: "path", TargetPath: "/media/raw/anime/Show", MediaType: "video", Selection: "profile", VideoProfileID: 10},
		{TargetType: "asset", TargetPath: assetPath, MediaType: "video", Selection: "profile", VideoProfileID: 20},
		{TargetType: "path", TargetPath: "/media/raw/anime/Show", MediaType: "audio", Selection: "profile", ProfileKey: "path-audio"},
	}
	for index := range assignments {
		if err := db.Create(&assignments[index]).Error; err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := profileAssignmentsForAsset(db, assetPath)
	if err != nil {
		t.Fatal(err)
	}
	if resolved["video"].VideoProfileID != 20 || resolved["video"].TargetType != "asset" {
		t.Fatalf("video resolution = %#v, want asset profile 20", resolved["video"])
	}
	if resolved["audio"].ProfileKey != "path-audio" || resolved["audio"].TargetType != "path" {
		t.Fatalf("audio resolution = %#v, want inherited path audio", resolved["audio"])
	}
}

func TestQueueFreezesResolvedPathAndAssetProfiles(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:profile-assignment-queue?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.Profile{}, &models.ProfileAssignment{}, &models.AppSetting{}, &models.ScanResult{},
		&models.AssetRecord{}, &models.QueueJob{}, &models.ExecutionPlan{}, &models.SchedulerReservation{},
	); err != nil {
		t.Fatal(err)
	}
	assetPath := filepath.Clean("/media/raw/anime/Show/episode-02.mkv")
	pathProfile := models.Profile{Name: "Show video", Scope: "path", Container: "mkv", VideoCodec: "x265", AudioCodec: "copy", QualityMode: "crf", QualityValue: 20, WorkerConfig: models.JSONMap{"videoEncoder": "libx265"}}
	fallbackProfile := models.Profile{Name: "Fallback asset", Scope: "asset", Container: "mkv", VideoCodec: "x265", AudioCodec: "copy", QualityMode: "crf", QualityValue: 24, WorkerConfig: models.JSONMap{"videoEncoder": "libx265"}}
	if err := db.Create(&pathProfile).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&fallbackProfile).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AssetRecord{Path: assetPath, RootPath: "/media/raw/anime", GroupPath: "Show", FileName: "episode-02.mkv"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.ScanResult{Path: assetPath,
		VideoStreams: models.JSONList{map[string]any{"index": 0}},
		AudioStreams: models.JSONList{map[string]any{"index": 1, "language": "jpn", "default": true}},
	}).Error; err != nil {
		t.Fatal(err)
	}
	settings := []models.AppSetting{
		{Key: "audioEnhancementProfiles", Value: models.JSONMap{"profiles": models.JSONList{
			models.JSONMap{"key": "path-audio", "scope": "path", "filters": "volume=0.5"},
			models.JSONMap{"key": "asset-audio", "scope": "asset", "filters": "volume=0.8"},
		}}},
		{Key: "trackProfiles", Value: models.JSONMap{"profiles": models.JSONList{
			models.JSONMap{"key": "path-tracks", "scope": "path", "videoMode": "first", "audioMode": "default", "subtitleMode": "none"},
		}}},
	}
	for index := range settings {
		if err := db.Create(&settings[index]).Error; err != nil {
			t.Fatal(err)
		}
	}
	assignments := []models.ProfileAssignment{
		{TargetType: "path", TargetPath: "/media/raw/anime/Show", MediaType: "video", Selection: "profile", VideoProfileID: pathProfile.ID},
		{TargetType: "path", TargetPath: "/media/raw/anime/Show", MediaType: "audio", Selection: "profile", ProfileKey: "path-audio"},
		{TargetType: "asset", TargetPath: assetPath, MediaType: "audio", Selection: "profile", ProfileKey: "asset-audio"},
		{TargetType: "path", TargetPath: "/media/raw/anime/Show", MediaType: "tracks", Selection: "profile", ProfileKey: "path-tracks"},
	}
	for index := range assignments {
		if err := db.Create(&assignments[index]).Error; err != nil {
			t.Fatal(err)
		}
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/queue/jobs", NewQueueHandler(db).Create)
	body := `{"mediaPath":"` + assetPath + `","libraryId":1,"profileId":` + strconv.FormatUint(uint64(fallbackProfile.ID), 10) + `,"resolveProfileAssignments":true}`
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/queue/jobs", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var job models.QueueJob
	if err := json.Unmarshal(response.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	if job.ProfileID != pathProfile.ID || job.AudioProfileKey != "asset-audio" || job.TrackProfileKey != "path-tracks" {
		t.Fatalf("assignment precedence was not frozen: %#v", job)
	}
	if job.AudioProfileSnapshot["filters"] != "volume=0.8" || job.TrackProfileSnapshot["resolvedForAsset"] != assetPath {
		t.Fatalf("supplemental snapshots were not frozen: audio=%#v tracks=%#v", job.AudioProfileSnapshot, job.TrackProfileSnapshot)
	}
	if indexes, ok := job.TrackProfileSnapshot["keepAudioStreams"].([]any); !ok || len(indexes) != 1 || indexes[0] != float64(1) {
		t.Fatalf("track snapshot was not resolved for the queued asset: %#v", job.TrackProfileSnapshot)
	}
}

func TestPathTrackProfileResolvesSemanticRulesPerAsset(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:path-track-resolution?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	assetPath := filepath.Clean("/media/raw/anime/Show/episode.mkv")
	scan := models.ScanResult{
		Path:         assetPath,
		VideoStreams: models.JSONList{map[string]any{"index": 0}},
		AudioStreams: models.JSONList{
			map[string]any{"index": 1, "language": "jpn", "default": true},
			map[string]any{"index": 2, "language": "eng", "title": "Director commentary"},
		},
		SubtitleStreams: models.JSONList{
			map[string]any{"index": 3, "language": "spa", "forced": false},
			map[string]any{"index": 4, "language": "eng", "forced": true},
		},
	}
	if err := db.Create(&scan).Error; err != nil {
		t.Fatal(err)
	}
	profile := map[string]any{
		"scope": "path", "videoMode": "first", "audioMode": "languages", "audioLanguages": []any{"jpn"},
		"dropCommentary": true, "subtitleMode": "forced-or-languages", "subtitleLanguages": []any{"spa"},
		"defaultAudioLanguage": "jpn", "defaultSubtitleLanguage": "spa", "keepAudioStreams": []any{99},
	}
	resolved := resolveTrackProfileForAsset(db, assetPath, profile)
	assertIntSlice := func(key string, want []int) {
		t.Helper()
		got, ok := resolved[key].([]int)
		if !ok || len(got) != len(want) {
			t.Fatalf("%s = %#v, want %#v", key, resolved[key], want)
		}
		for index := range want {
			if got[index] != want[index] {
				t.Fatalf("%s = %#v, want %#v", key, got, want)
			}
		}
	}
	assertIntSlice("keepVideoStreams", []int{0})
	assertIntSlice("keepAudioStreams", []int{1})
	assertIntSlice("keepSubtitleStreams", []int{3, 4})
	audioMetadata, _ := resolved["audioMetadata"].(map[string]any)
	defaultAudio, _ := audioMetadata["1"].(map[string]any)
	if defaultAudio["default"] != true {
		t.Fatalf("default audio language was not resolved: %#v", resolved["audioMetadata"])
	}
	subtitleMetadata, _ := resolved["subtitleMetadata"].(map[string]any)
	defaultSpanish, _ := subtitleMetadata["3"].(map[string]any)
	forcedEnglish, _ := subtitleMetadata["4"].(map[string]any)
	if defaultSpanish["default"] != true || forcedEnglish["default"] != false {
		t.Fatalf("default subtitle language was not resolved: %#v", resolved["subtitleMetadata"])
	}
}

func TestTrackProfileMergesBelowManualAssetOverride(t *testing.T) {
	profile := AssetConversionOverrideState{KeepAudioStreams: []int{1}, KeepSubtitleStreams: []int{3}}
	asset := AssetConversionOverrideState{KeepAudioStreams: []int{2}}
	merged := mergeTrackProfileBelowAssetOverride(profile, asset)
	if len(merged.KeepAudioStreams) != 1 || merged.KeepAudioStreams[0] != 2 {
		t.Fatalf("manual audio override lost: %#v", merged.KeepAudioStreams)
	}
	if len(merged.KeepSubtitleStreams) != 1 || merged.KeepSubtitleStreams[0] != 3 {
		t.Fatalf("profile subtitle selection was not inherited: %#v", merged.KeepSubtitleStreams)
	}
}

func TestAssetRenameMigratesAssignmentEvenWithMetadataOverride(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:profile-assignment-rename?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}, &models.ProfileAssignment{}); err != nil {
		t.Fatal(err)
	}
	oldPath := "/media/raw/anime/Show/original.mkv"
	newPath := "/media/raw/anime/Show/Show S01E01.mkv"
	if err := db.Create(&models.AppSetting{Key: "assetMetadataOverrides", Value: models.JSONMap{"entries": models.JSONMap{
		oldPath: models.JSONMap{"categories": models.JSONList{"anime"}},
	}}}).Error; err != nil {
		t.Fatal(err)
	}
	assignment := models.ProfileAssignment{TargetType: "asset", TargetPath: oldPath, MediaType: "video", Selection: "profile", VideoProfileID: 9}
	if err := db.Create(&assignment).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateSingleAssetPathOverrides(db, oldPath, newPath); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&assignment, assignment.ID).Error; err != nil {
		t.Fatal(err)
	}
	if assignment.TargetPath != newPath {
		t.Fatalf("assignment stayed on old path: %#v", assignment)
	}
}
