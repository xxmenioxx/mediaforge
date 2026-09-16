package handlers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAssetDefaultsValidationAndCompatibilityDefaults(t *testing.T) {
	value, err := validateAssetDefaultsValue(nil, models.JSONMap{})
	if err != nil {
		t.Fatal(err)
	}
	video := assetDefaultsDomain(value, "video")
	audio := assetDefaultsDomain(value, "audio")
	tracks := assetDefaultsDomain(value, "tracks")
	if workerStringValue(video["source"]) != "custom" || workerStringValue(settingProfileObject(video["custom"])["videoCodec"]) != "copy" {
		t.Fatalf("compatibility video defaults=%#v", video)
	}
	if assetDefaultsIntSetting(settingProfileObject(audio["custom"])["compatibleTrackChannels"], 0) != 2 {
		t.Fatalf("compatibility audio defaults=%#v", audio)
	}
	if workerStringValue(tracks["source"]) != "custom" || workerStringValue(settingProfileObject(tracks["custom"])["subtitleDisposition"]) != "keep" {
		t.Fatalf("compatibility track defaults=%#v", tracks)
	}
	if _, err := validateAssetDefaultsValue(nil, models.JSONMap{"video": models.JSONMap{"source": "invalid"}}); err == nil {
		t.Fatal("invalid source was accepted")
	}
	if _, err := validateAssetDefaultsValue(nil, models.JSONMap{"audio": models.JSONMap{"custom": models.JSONMap{"compatibleTrackChannels": 0}}}); err == nil {
		t.Fatal("invalid audio channel count was accepted")
	}
}

func TestAssetDefaultsPreserveDomainPrecedenceAndDisabledStates(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:asset-defaults-precedence?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}, &models.AssetRecord{}, &models.SourceGroup{}, &models.ProfileAssignment{}, &models.AssetScopeConfiguration{}); err != nil {
		t.Fatal(err)
	}
	defaults := defaultAssetDefaultsValue()
	defaults["video"].(models.JSONMap)["custom"] = models.JSONMap{"videoCodec": "x265"}
	if err := db.Create(&models.AppSetting{Key: assetDefaultsSettingKey, Value: defaults}).Error; err != nil {
		t.Fatal(err)
	}
	group := models.SourceGroup{Name: "Shows", SourcePath: "/raw/shows", RelativePath: "shows"}
	if err := db.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	record := models.AssetRecord{Path: "/raw/shows/Series/Episode.mkv", SourceGroupID: group.ID, LogicalGroupPath: "/raw/shows/Series", RootPath: "/raw", GroupPath: "shows/Series", FileName: "Episode.mkv", Status: "unprocessed"}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	assignments := []models.ProfileAssignment{
		{TargetType: assetScopePath, TargetPath: "/raw/shows/Series", MediaType: "video", Selection: configSelectionDisabled},
		{TargetType: assetScopeLogicalGroup, TargetPath: "/raw/shows/Series", MediaType: "audio", Selection: "profile", ProfileKey: "dialogue"},
		{TargetType: assetScopeSourceGroup, TargetPath: "/raw/shows", MediaType: "tracks", Selection: configSelectionDisabled},
	}
	if err := db.Create(&assignments).Error; err != nil {
		t.Fatal(err)
	}
	effective, err := effectiveAssetConfiguration(db, record.Path)
	if err != nil {
		t.Fatal(err)
	}
	if effective.Video.Selection != configSelectionDisabled || effective.Video.Source != assetScopePath {
		t.Fatalf("path video did not override default: %#v", effective.Video)
	}
	if effective.Audio.Selection != "profile" || effective.Audio.ProfileKey != "dialogue" || effective.Audio.Source != assetScopeLogicalGroup {
		t.Fatalf("logical-group audio did not override default: %#v", effective.Audio)
	}
	if effective.Tracks.Selection != configSelectionDisabled || effective.Tracks.Source != assetScopeSourceGroup {
		t.Fatalf("source-group tracks did not preserve disabled state: %#v", effective.Tracks)
	}
}

func TestAssetDefaultsInvalidStoredProfileFallsBackWithWarning(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:asset-defaults-invalid-profile?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}, &models.Profile{}, &models.AssetRecord{}); err != nil {
		t.Fatal(err)
	}
	value := defaultAssetDefaultsValue()
	value["video"].(models.JSONMap)["source"] = "profile"
	value["video"].(models.JSONMap)["profileId"] = 999
	if err := db.Create(&models.AppSetting{Key: assetDefaultsSettingKey, Value: value}).Error; err != nil {
		t.Fatal(err)
	}
	effective, err := effectiveAssetConfiguration(db, "/raw/episode.mkv")
	if err != nil {
		t.Fatal(err)
	}
	if len(effective.Warnings) == 0 || !strings.Contains(effective.Warnings[0], "unavailable") {
		t.Fatalf("invalid default warning=%#v", effective.Warnings)
	}
	if effective.Video.Source != assetDefaultsGlobalSource || effective.Video.ProfileData == nil {
		t.Fatalf("invalid default did not use safe fallback: %#v", effective.Video)
	}
}

func TestAssetDefaultsResolveCustomAudioTrackAndVideoPolicies(t *testing.T) {
	input := QueueJobInput{}
	effective := EffectiveAssetConfiguration{
		Video:  AssetConfigurationValue{Selection: configSelectionValue, Source: assetDefaultsGlobalSource, ProfileData: models.JSONMap{"videoCodec": "copy"}},
		Audio:  AssetConfigurationValue{Selection: configSelectionValue, Source: assetDefaultsGlobalSource, ProfileData: models.JSONMap{"addCompatibleTrack": true, "compatibleTrackQuality": 160, "compatibleTrackChannels": 2, "compatibleTrackSource": "default", "preferredDefaultLanguage": "jpn"}},
		Tracks: AssetConfigurationValue{Selection: configSelectionValue, Source: assetDefaultsGlobalSource, ProfileData: models.JSONMap{"trackDispositionVersion": 1, "subtitleDisposition": "extract", "subtitleSidecarFormats": []string{"both"}, "attachmentPolicy": "remove", "fontAttachmentExportPolicy": "all", "chapterPolicy": "keep", "ocrMode": "clean"}},
	}
	resolution := resolveProfileAssignmentsFromEffective(&input, effective)
	if input.ProcessingMode != ProcessingModeAudioOnly || input.AudioProfileKey != assetDefaultsCustomAudioKey || input.TrackProfileKey != assetDefaultsCustomTracksKey {
		t.Fatalf("custom defaults did not resolve: input=%#v resolution=%#v", input, resolution)
	}
	if got := workerStringValue(resolution["audio"].(models.JSONMap)["profileData"].(models.JSONMap)["preferredDefaultLanguage"]); got != "jpn" {
		t.Fatalf("audio default language=%q", got)
	}
	plan, err := resolveTrackPlan(models.ScanResult{SubtitleStreams: models.JSONList{map[string]any{"index": 3, "codec": "subrip", "language": "eng"}}}, effective.Tracks.ProfileData)
	if err != nil || len(plan.SidecarOutputs) != 2 {
		t.Fatalf("both track default plan=%#v err=%v", plan, err)
	}
	if plan.SidecarOutputs[0].LogicalTransformKey == "" || plan.SidecarOutputs[0].LogicalTransformKey != plan.SidecarOutputs[1].LogicalTransformKey {
		t.Fatalf("both outputs did not share logical identity: %#v", plan.SidecarOutputs)
	}
}

func TestAssetDefaultsQueueResolutionRemainsFrozenAfterSettingEdit(t *testing.T) {
	old := models.JSONMap{
		"video": models.JSONMap{"source": assetDefaultsGlobalSource, "selection": configSelectionValue, "profileData": models.JSONMap{"videoCodec": "copy"}},
		"audio": models.JSONMap{"source": assetDefaultsGlobalSource, "selection": configSelectionValue, "profileData": models.JSONMap{"preferredDefaultLanguage": "eng"}},
	}
	current := EffectiveAssetConfiguration{
		Video:  AssetConfigurationValue{Selection: configSelectionValue, Source: assetDefaultsGlobalSource, ProfileData: models.JSONMap{"videoCodec": "x265"}},
		Audio:  AssetConfigurationValue{Selection: configSelectionValue, Source: assetDefaultsGlobalSource, ProfileData: models.JSONMap{"preferredDefaultLanguage": "jpn"}},
		Tracks: AssetConfigurationValue{Selection: configSelectionValue, Source: assetDefaultsGlobalSource, ProfileData: models.JSONMap{"subtitleDisposition": "remove"}},
	}
	frozen := effectiveConfigurationWithFrozenAssetDefaults(current, old)
	if workerStringValue(frozen.Video.ProfileData["videoCodec"]) != "copy" || workerStringValue(frozen.Audio.ProfileData["preferredDefaultLanguage"]) != "eng" {
		t.Fatalf("queue resolution changed after defaults edit: %#v", frozen)
	}
}

func TestAssetDefaultsOptionalVideoTrackRemovalAndSharedSubtitleGeneration(t *testing.T) {
	streams := MediaStreamInventory{Video: []MediaStream{{Index: 0, Codec: "h264"}, {Index: 1, Codec: "mjpeg", StillImage: true}}, Subtitle: []MediaStream{{Index: 3, Codec: "subrip", Language: "eng"}}}
	plan := MediaJobPlan{InputPath: "/raw/Episode.mkv", OutputPath: filepath.Join(t.TempDir(), "Episode.mkv"), ProcessingMode: ProcessingModeFullEncode, Streams: streams, Profile: models.Profile{VideoCodec: "x265", WorkerConfig: models.JSONMap{"removeNonPrimaryVideoTracks": false}}, ResolvedTracks: &ResolvedTrackPlan{VideoStreams: []ResolvedTrackStream{{StreamIndex: 0}, {StreamIndex: 1}}, SidecarOutputs: []ResolvedTrackSidecar{{StreamIndex: 3, Codec: "subrip", SourceType: "text", Language: "eng", Format: "srt", RequestedFormat: "both", Mode: "converted", LogicalTransformKey: "subtitle:3:converted:both"}, {StreamIndex: 3, Codec: "subrip", SourceType: "text", Language: "eng", Format: "ass", RequestedFormat: "both", Mode: "converted", LogicalTransformKey: "subtitle:3:converted:both"}}}}
	if got := selectedVideoStreamsForPlan(plan); len(got) != 2 {
		t.Fatalf("extra video stream was removed while disabled: %#v", got)
	}
	plan.Profile.WorkerConfig["removeNonPrimaryVideoTracks"] = true
	if got := selectedVideoStreamsForPlan(plan); len(got) != 1 || got[0].Index != 0 {
		t.Fatalf("non-primary video removal=%#v", got)
	}

	bin := t.TempDir()
	ffmpeg := filepath.Join(bin, "ffmpeg")
	script := "#!/bin/sh\noutput=\"\"\nfor argument do output=\"$argument\"; done\nprintf '%s\\n' \"$output\" >> \"$MVFORGE_SUBTITLE_LOG\"\ncase \"$output\" in\n *.ass) printf '%s\\n' '[Events]' 'Dialogue: 0,0:00:01.00,0:00:02.00,Default,,Hola' > \"$output\" ;;\n *) printf '%s\\n' '1' '00:00:01,000 --> 00:00:02,000' 'Hola' > \"$output\" ;;\nesac\n"
	if err := os.WriteFile(ffmpeg, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "ffmpeg.log")
	t.Setenv("MVFORGE_SUBTITLE_LOG", logPath)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	artifacts, err := generateResolvedSubtitleArtifacts(context.Background(), plan, nil)
	if err != nil || len(artifacts) != 2 {
		t.Fatalf("shared subtitle artifacts=%#v err=%v", artifacts, err)
	}
	log, err := os.ReadFile(logPath)
	if err != nil || len(strings.Split(strings.TrimSpace(string(log)), "\n")) != 2 {
		t.Fatalf("both formats did not use one extraction plus one render: log=%q err=%v", log, err)
	}
}
