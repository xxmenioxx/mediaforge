package handlers

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

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
	value["audio"].(models.JSONMap)["custom"] = models.JSONMap{"preferredDefaultLanguage": "jpn"}
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
	if workerStringValue(effective.Audio.ProfileData["preferredDefaultLanguage"]) != "jpn" {
		t.Fatalf("invalid video profile changed valid audio default: %#v", effective.Audio)
	}
}

func TestAssetDefaultsCustomVideoUsesNeutralBaseAndAppliesContractAfterCustom(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:asset-defaults-neutral-video?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Profile{}); err != nil {
		t.Fatal(err)
	}
	profile := models.Profile{
		Name: "User HEVC Profile", VideoCodec: "x265_10bit", CodecFamily: "hevc", AudioCodec: "copy",
		QualityMode: "crf", QualityValue: 12, PreferredEncoder: "libx265", AllowedEncoders: models.StringList{"libx265"},
		WorkerConfig: models.JSONMap{"videoEncoder": "libx265", "videoPreset": "slow"},
	}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	value := defaultAssetDefaultsValue()
	value["video"].(models.JSONMap)["custom"] = models.JSONMap{"videoCodec": "x264", "preferredEncoder": "libx264"}
	resolved, err := (QueueHandler{db: db}).customVideoProfile(value)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.VideoCodec != "x264" || resolved.CodecFamily != "h264" || resolved.PreferredEncoder != "libx264" || len(resolved.AllowedEncoders) != 1 || resolved.AllowedEncoders[0] != "libx264" {
		t.Fatalf("custom video did not apply neutral contract: %#v", resolved)
	}
	if resolved.QualityValue == profile.QualityValue || resolved.WorkerConfig["videoPreset"] != nil {
		t.Fatalf("custom video inherited user profile fields: %#v", resolved)
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

func TestAssetDefaultsPreviewTestEncodeQueueParityForUnassignedAsset(t *testing.T) {
	db := queueSelectedAssetsTestDB(t)
	sourceDir := t.TempDir()
	mediaPath := filepath.Join(sourceDir, "Episode.mkv")
	if err := os.WriteFile(mediaPath, []byte("test media placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}
	library := models.Library{Name: "Asset Defaults Parity Library", SourcePath: sourceDir, DestinationPath: t.TempDir(), Type: "anime"}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AssetRecord{Path: mediaPath, SourcePath: mediaPath, FileName: filepath.Base(mediaPath), Status: "unprocessed"}).Error; err != nil {
		t.Fatal(err)
	}
	scan := models.ScanResult{
		Path:       mediaPath,
		VideoCodec: "h264", Width: 1280, Height: 720,
		VideoStreams:           models.JSONList{map[string]any{"index": 0, "codec": "h264", "width": 1280, "height": 720, "avgFrameRate": "24000/1001"}},
		AudioStreams:           models.JSONList{map[string]any{"index": 1, "codec": "aac", "language": "jpn", "default": true}},
		SubtitleStreams:        models.JSONList{map[string]any{"index": 3, "codec": "subrip", "language": "eng", "title": "Signs"}},
		FrameStructureAnalysis: models.JSONMap{"averageGopLength": 48.0, "medianGopLength": 48.0, "completeGops": 8, "variability": "low", "confidence": "high"},
	}
	if err := db.Create(&scan).Error; err != nil {
		t.Fatal(err)
	}

	defaults := defaultAssetDefaultsValue()
	defaults["video"].(models.JSONMap)["custom"] = models.JSONMap{"videoCodec": "x265", "preferredEncoder": "libx265"}
	defaults["audio"].(models.JSONMap)["custom"] = models.JSONMap{
		"addCompatibleTrack": true, "compatibleTrackQuality": 160, "compatibleTrackChannels": 2,
		"compatibleTrackSource": "default", "preferredDefaultLanguage": "jpn",
	}
	defaults["tracks"].(models.JSONMap)["custom"] = models.JSONMap{
		"trackDispositionVersion": canonicalTrackDispositionVersion, "audioMode": "all", "subtitleMode": "all",
		"subtitleDisposition": "extract", "subtitleSidecarFormats": models.JSONList{"both"},
		"attachmentPolicy": "auto", "fontAttachmentExportPolicy": "none", "chapterPolicy": "keep", "ocrMode": "clean",
	}
	if err := db.Create(&models.AppSetting{Key: assetDefaultsSettingKey, Value: defaults}).Error; err != nil {
		t.Fatal(err)
	}

	queue := NewQueueHandler(db)
	queueInput := QueueJobInput{MediaPath: mediaPath, LibraryID: library.ID, ResolveProfileAssignments: true}
	queueResolution, err := queue.resolveProfileAssignments(&queueInput)
	if err != nil {
		t.Fatal(err)
	}
	previewEffective, err := effectiveAssetConfiguration(db, mediaPath)
	if err != nil {
		t.Fatal(err)
	}
	previewInput := QueueJobInput{MediaPath: mediaPath, LibraryID: library.ID}
	previewResolution := resolveProfileAssignmentsFromEffective(&previewInput, previewEffective)
	if configurationHash(previewResolution) != configurationHash(queueResolution) {
		t.Fatalf("Preview effective Asset Defaults diverged from Queue: preview=%#v queue=%#v", previewResolution, queueResolution)
	}

	queueJob := models.QueueJob{
		MediaPath: mediaPath, LibraryID: library.ID, ProfileID: queueInput.ProfileID,
		AudioProfileKey: queueInput.AudioProfileKey, TrackProfileKey: queueInput.TrackProfileKey,
		ProcessingMode: queueInput.ProcessingMode, ProfileResolution: queueResolution,
	}
	if err := queue.captureSupplementalProfiles(&queueJob); err != nil {
		t.Fatal(err)
	}
	if err := queue.captureProfile(&queueJob, queueJob.ProfileID, "asset_defaults_parity_queue"); err != nil {
		t.Fatal(err)
	}

	testEncode, err := (AssetHandler{db: db}).resolveTestEncodeRequest(testEncodeRequest{
		SourcePath: mediaPath, LibraryID: library.ID, ConfigurationSource: "effective_asset", ResolveAssignments: true,
	})
	if err != nil {
		t.Fatalf("Test Encode resolution failed: %v (queue input=%#v resolution=%#v)", err, queueInput, queueResolution)
	}
	if configurationHash(queueResolution) != configurationHash(testEncode.job.ProfileResolution) {
		t.Fatalf("Queue and Test Encode resolved different Asset Defaults: queue=%#v test=%#v", queueResolution, testEncode.job.ProfileResolution)
	}

	queueProfile, err := (WorkerHandler{db: db}).profileForJob(queueJob)
	if err != nil {
		t.Fatal(err)
	}
	streams := MediaStreamInventory{
		Video: []MediaStream{{Index: 0, Codec: "h264", Width: 1280, Height: 720, FrameRate: "24000/1001"}},
	}
	previewJob := models.QueueJob{
		MediaPath: mediaPath, ProfileResolution: previewResolution, ProfileID: previewInput.ProfileID,
		AudioProfileKey: previewInput.AudioProfileKey, TrackProfileKey: previewInput.TrackProfileKey,
		ProcessingMode: previewInput.ProcessingMode,
	}
	previewProfile, custom, err := queue.customVideoProfileForJob(previewJob)
	if err != nil || !custom {
		t.Fatalf("Preview did not receive the effective custom video defaults: profile=%#v custom=%v err=%v", previewProfile, custom, err)
	}
	applyAssetDefaultsAudioToVideoProfile(&previewProfile, previewJob)
	previewProfile, err = resolvePreviewVideoProfile(db, mediaPath, previewProfile, streams, InterlaceAnalysis{}, CadenceAnalysis{}, CadenceRecommendation{})
	if err != nil {
		t.Fatal(err)
	}
	queueProfile, err = resolveQueueVideoProfile(db, mediaPath, queueJob, queueProfile, AssetConversionOverrideState{}, "")
	if err != nil {
		t.Fatal(err)
	}
	testProfile := resolveMediaJobVideoProfile(testEncode.profile, streams, mediaPath, InterlaceAnalysis{}, CadenceAnalysis{}, CadenceRecommendation{})
	queueProfile = resolveMediaJobVideoProfile(queueProfile, streams, mediaPath, InterlaceAnalysis{}, CadenceAnalysis{}, CadenceRecommendation{})

	if configurationHash(assetDefaultsVideoParityDecision(previewProfile)) != configurationHash(assetDefaultsVideoParityDecision(testProfile)) ||
		configurationHash(assetDefaultsVideoParityDecision(queueProfile)) != configurationHash(assetDefaultsVideoParityDecision(testProfile)) {
		t.Fatalf("Preview, Test Encode, and Queue video decisions diverged: preview=%#v test=%#v queue=%#v", assetDefaultsVideoParityDecision(previewProfile), assetDefaultsVideoParityDecision(testProfile), assetDefaultsVideoParityDecision(queueProfile))
	}

	queueAudio := audioProfileFromMap(queueJob.AudioProfileKey, queueJob.AudioProfileSnapshot)
	if !reflect.DeepEqual(queueAudio, testEncode.audio) {
		t.Fatalf("Preview/Test Encode/Queue audio defaults diverged: queue=%#v test=%#v", queueAudio, testEncode.audio)
	}
	previewAudio := audioProfileFromMap(previewJob.AudioProfileKey, profileDataForQueueJob(previewJob, "audio"))
	if !reflect.DeepEqual(previewAudio, testEncode.audio) {
		t.Fatalf("Preview audio defaults diverged: preview=%#v test=%#v", previewAudio, testEncode.audio)
	}
	previewTracks := applyAssetDefaultsAudioToTrackProfile(previewJob, profileDataForQueueJob(previewJob, "tracks"))
	previewTracks = resolveTrackProfileForAsset(db, mediaPath, previewTracks)
	previewTrackPlan, err := resolveTrackPlan(scan, previewTracks)
	if err != nil {
		t.Fatal(err)
	}
	previewTrackPlanMap, err := resolvedTrackPlanMap(previewTrackPlan)
	if err != nil {
		t.Fatal(err)
	}
	queueTrackPlan := settingProfileObject(queueJob.TrackProfileSnapshot[resolvedTrackPlanSnapshotKey])
	if configurationHash(previewTrackPlanMap) != configurationHash(queueTrackPlan) {
		t.Fatalf("Preview canonical track/subtitle plan diverged: preview=%#v queue=%#v", previewTrackPlanMap, queueTrackPlan)
	}
	for name, job := range map[string]models.QueueJob{"queue": queueJob, "test_encode": testEncode.job} {
		if configurationHash(profileDataForQueueJob(job, "tracks")) != configurationHash(profileDataForQueueJob(testEncode.job, "tracks")) {
			t.Fatalf("%s track defaults diverged: %#v", name, profileDataForQueueJob(job, "tracks"))
		}
		if configurationHash(plannedSidecarArtifactsJSON(job)) != configurationHash(plannedSidecarArtifactsJSON(testEncode.job)) {
			t.Fatalf("%s canonical subtitle metadata diverged: %#v", name, plannedSidecarArtifactsJSON(job))
		}
	}
	if len(plannedSidecarArtifactsJSON(queueJob)) != 2 {
		t.Fatalf("canonical both subtitle plan did not produce SRT and ASS metadata: %#v", plannedSidecarArtifactsJSON(queueJob))
	}
}

func assetDefaultsVideoParityDecision(profile models.Profile) models.JSONMap {
	decision := effectiveVideoDecision(profile)
	for _, key := range []string{"restoration", "upscale"} {
		delete(decision, key)
	}
	return decision
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

func TestAssetDefaultsQueueFreezeAndRetryIgnoreChangedSettings(t *testing.T) {
	db := queueSelectedAssetsTestDB(t)
	sourceDir := t.TempDir()
	mediaPath := filepath.Join(sourceDir, "Episode.mkv")
	if err := os.WriteFile(mediaPath, []byte("test media placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}
	library := models.Library{Name: "Asset Defaults Library", SourcePath: sourceDir, DestinationPath: t.TempDir(), Type: "anime"}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	profile := authoritativeTestProfile()
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	record := models.AssetRecord{Path: mediaPath, SourcePath: mediaPath, LogicalGroupPath: sourceDir, FileName: filepath.Base(mediaPath), Status: "unprocessed"}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AssetScopeConfiguration{ScopeType: assetScopeLogicalGroup, ScopeKey: sourceDir, DestinationSelection: configSelectionValue, DestinationLibraryID: library.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.ScanResult{Path: mediaPath, VideoCodec: "h264", Width: 720, Height: 480, VideoStreams: models.JSONList{map[string]any{"index": 0, "codec": "h264", "width": 720, "height": 480}}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "pipelineAutomation", Value: models.JSONMap{"reviewMode": "automatic"}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.RuntimeSnapshot{DetectedAt: time.Now(), SelectedProfile: "test", Encoders: models.JSONMap{"libx265": models.JSONMap{"listed": true, "usable": true}}}).Error; err != nil {
		t.Fatal(err)
	}
	oldDefaults := defaultAssetDefaultsValue()
	oldDefaults["audio"].(models.JSONMap)["custom"] = models.JSONMap{"preferredDefaultLanguage": "eng"}
	if err := db.Create(&models.AppSetting{Key: assetDefaultsSettingKey, Value: oldDefaults}).Error; err != nil {
		t.Fatal(err)
	}
	newDefaults := defaultAssetDefaultsValue()
	newDefaults["video"].(models.JSONMap)["custom"] = models.JSONMap{"videoCodec": "x265"}
	newDefaults["audio"].(models.JSONMap)["custom"] = models.JSONMap{"preferredDefaultLanguage": "jpn"}
	if err := db.Model(&models.AppSetting{}).Where("key = ?", assetDefaultsSettingKey).Update("value", newDefaults).Error; err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	job := models.QueueJob{
		MediaPath: mediaPath, LibraryID: library.ID, Status: JobStatusRunning, Stage: JobStageClaimed,
		WorkerName: "test-worker", StartedAt: &now, ProcessingMode: ProcessingModeAudioOnly,
		ProfileResolution: models.JSONMap{
			"video":  models.JSONMap{"source": assetDefaultsGlobalSource, "selection": configSelectionValue, "profileData": models.JSONMap{"videoCodec": "copy"}},
			"audio":  models.JSONMap{"source": assetDefaultsGlobalSource, "selection": configSelectionValue, "profileData": models.JSONMap{"preferredDefaultLanguage": "eng"}},
			"tracks": models.JSONMap{"source": assetDefaultsGlobalSource, "selection": configSelectionValue, "profileData": models.JSONMap{"subtitleDisposition": "keep"}},
		},
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	prepared, err := NewWorkerHandler(db).prepareClaimedQueueJob(job)
	if err != nil {
		t.Fatal(err)
	}
	preparedVideo := settingProfileObject(prepared.ProfileResolution["video"])
	preparedAudio := settingProfileObject(prepared.ProfileResolution["audio"])
	if workerStringValue(settingProfileObject(preparedVideo["profileData"])["videoCodec"]) != "copy" || workerStringValue(settingProfileObject(preparedAudio["profileData"])["preferredDefaultLanguage"]) != "eng" {
		t.Fatalf("claim preparation re-resolved changed defaults: %#v", prepared.ProfileResolution)
	}
	again, err := NewWorkerHandler(db).prepareClaimedQueueJob(prepared)
	if err != nil {
		t.Fatal(err)
	}
	againVideo := settingProfileObject(again.ProfileResolution["video"])
	againAudio := settingProfileObject(again.ProfileResolution["audio"])
	if workerStringValue(settingProfileObject(againVideo["profileData"])["videoCodec"]) != "copy" || workerStringValue(settingProfileObject(againAudio["profileData"])["preferredDefaultLanguage"]) != "eng" {
		t.Fatalf("retry changed frozen Asset Defaults resolution: %#v", again.ProfileResolution)
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
	script := "#!/bin/sh\noutput=\"\"\nfor argument do output=\"$argument\"; done\nprintf '%s\\n' \"$output\" >> \"$MVFORGE_SUBTITLE_LOG\"\nif [ \"$output\" = \"-\" ]; then exit 0; fi\ncase \"$output\" in\n *.ass) printf '%s\\n' '[Events]' 'Dialogue: 0,0:00:01.00,0:00:02.00,Default,,Hola' > \"$output\" ;;\n *) printf '%s\\n' '1' '00:00:01,000 --> 00:00:02,000' 'Hola' > \"$output\" ;;\nesac\n"
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
