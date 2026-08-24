package handlers

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func testEncodeTestDB(t *testing.T, name string, values ...any) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	modelsToMigrate := []any{&models.TestEncode{}, &models.TaskReservation{}, &models.ScanResult{}, &models.Library{}, &models.AppSetting{}}
	modelsToMigrate = append(modelsToMigrate, values...)
	if err := db.AutoMigrate(modelsToMigrate...); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestResolveTestEncodeWindowModesAndBounds(t *testing.T) {
	db := testEncodeTestDB(t, "test-encode-windows")
	path := "/raw/series/episode.mkv"
	if err := db.Create(&models.ScanResult{
		Path: path, Duration: 100,
		InterlaceAnalysis: models.JSONMap{"sampledAt": models.JSONList{5.0, 45.0, 85.0}},
	}).Error; err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name         string
		mode         string
		custom       float64
		duration     int
		wantStart    float64
		wantDuration int
	}{
		{name: "representative uses sampled analysis", mode: "representative", duration: 20, wantStart: 45, wantDuration: 20},
		{name: "beginning", mode: "beginning", duration: 20, wantStart: 0, wantDuration: 20},
		{name: "middle", mode: "middle", duration: 20, wantStart: 40, wantDuration: 20},
		{name: "custom clamps to final complete window", mode: "custom", custom: 99, duration: 20, wantStart: 80, wantDuration: 20},
		{name: "duration has safe maximum", mode: "beginning", duration: 500, wantStart: 0, wantDuration: 120},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, duration := resolveTestEncodeWindow(db, path, tt.mode, tt.custom, tt.duration)
			if start != tt.wantStart || duration != tt.wantDuration {
				t.Fatalf("got start=%v duration=%d, want start=%v duration=%d", start, duration, tt.wantStart, tt.wantDuration)
			}
		})
	}
}

func TestConfigurationHashIsStableAndSensitive(t *testing.T) {
	a := configurationHash(models.JSONMap{"profile": models.JSONMap{"codec": "hevc", "quality": 20}, "tracks": models.JSONList{1.0, 2.0}})
	b := configurationHash(models.JSONMap{"tracks": models.JSONList{1.0, 2.0}, "profile": models.JSONMap{"quality": 20, "codec": "hevc"}})
	c := configurationHash(models.JSONMap{"profile": models.JSONMap{"codec": "hevc", "quality": 21}, "tracks": models.JSONList{1.0, 2.0}})
	if a != b {
		t.Fatalf("equivalent JSON configurations produced different hashes: %s != %s", a, b)
	}
	if a == c {
		t.Fatal("a material configuration change did not change the hash")
	}
}

func TestLabDraftTestEncodeDoesNotApplyPersistedAssetVideoOverrides(t *testing.T) {
	path := "/media/video/asset.mkv"
	job := models.QueueJob{MediaPath: path, ProcessingMode: ProcessingModeFullEncode}
	labTracks := AssetConversionOverrideState{KeepAudioStreams: []int{1}}
	persisted := map[string]AssetConversionOverrideState{
		path: {
			FrameStructureBFrameMode: "custom",
			FrameStructureMaxBFrames: 1,
			HEVCLevelMode:            "custom",
			HEVCLevel:                "5.0",
		},
	}

	override := testEncodeConversionOverride("lab_draft", job, labTracks, persisted)
	if override.FrameStructureBFrameMode != "" || override.FrameStructureMaxBFrames != 0 || override.HEVCLevelMode != "" || override.HEVCLevel != "" {
		t.Fatalf("persisted asset video override leaked into LAB Test Encode: %#v", override)
	}
	if len(override.KeepAudioStreams) != 1 || override.KeepAudioStreams[0] != 1 {
		t.Fatalf("LAB track override was not preserved: %#v", override)
	}

	labProfile := models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{
		"videoEncoder": "hevc_qsv", "frameStructureMode": "custom",
		"frameStructureBFrameMode": "custom", "frameStructureMaxBFrames": 3,
		"hevcLevelMode": "custom", "hevcLevel": "4.1",
	}}
	effective := applyAssetConversionOverrideToProfile(labProfile, override)
	effective = resolveHEVCLevel(effective, MediaStreamInventory{})
	command := shellJoin(videoCodecArgsForResolvedEncoder(effective, nil, "hevc_qsv"))
	assertContains(t, command, "-bf 3")
	assertContains(t, command, "-level:v 41 -tier main")
	assertNotContains(t, command, "-bf 1")
}

func TestLabDraftTrackProfileResolvesLanguageRulesForTestEncode(t *testing.T) {
	db := testEncodeTestDB(t, "test-encode-lab-track-language-rules")
	path := filepath.Clean("/media/raw/movie.mkv")
	if err := db.Create(&models.ScanResult{
		Path:         path,
		VideoStreams: models.JSONList{map[string]any{"index": 0}},
		AudioStreams: models.JSONList{
			map[string]any{"index": 1, "language": "spa", "default": true},
			map[string]any{"index": 2, "language": "eng"},
		},
	}).Error; err != nil {
		t.Fatal(err)
	}

	override := resolveLabTrackOverride(db, path, models.JSONMap{
		"scope": "path", "videoMode": "first", "audioMode": "languages",
		"audioLanguages": models.JSONList{"spa"}, "dropCommentary": true,
	}, AssetConversionOverrideState{})
	if len(override.KeepAudioStreams) != 1 || override.KeepAudioStreams[0] != 1 {
		t.Fatalf("LAB Test Encode kept audio streams %#v, want only Spanish stream 1", override.KeepAudioStreams)
	}

	plan := MediaJobPlan{
		Profile:  models.Profile{VideoCodec: "x265", AudioCodec: "copy"},
		Streams:  MediaStreamInventory{Video: []MediaStream{{Index: 0}}, Audio: []MediaAudioStream{{Index: 1}, {Index: 2}}},
		Override: override,
	}
	command := shellJoin(appendSelectedStreamMaps(nil, plan))
	assertContains(t, command, "-map 0:1")
	assertNotContains(t, command, "-map 0:2")
}

func TestEffectiveAssetTestEncodeKeepsPersistedVideoOverridePrecedence(t *testing.T) {
	path := "/media/video/asset.mkv"
	job := models.QueueJob{MediaPath: path, ProcessingMode: ProcessingModeFullEncode}
	override := testEncodeConversionOverride("effective_asset", job, AssetConversionOverrideState{}, map[string]AssetConversionOverrideState{
		path: {FrameStructureBFrameMode: "custom", FrameStructureMaxBFrames: 2, HEVCLevelMode: "custom", HEVCLevel: "4.0"},
	})
	if override.FrameStructureBFrameMode != "custom" || override.FrameStructureMaxBFrames != 2 || override.HEVCLevel != "4.0" {
		t.Fatalf("effective asset override was not preserved: %#v", override)
	}
}

func TestTestEncodeFFmpegCommandUsesMigratedGORMColumn(t *testing.T) {
	db := testEncodeTestDB(t, "test-encode-ffmpeg-command-column")
	test := models.TestEncode{SourcePath: "/raw/a.mkv", LibraryID: 1, ConfigurationSource: "effective_asset", Status: testEncodeWaiting, Phase: "waiting"}
	if err := db.Create(&test).Error; err != nil {
		t.Fatal(err)
	}
	command := "ffmpeg -i a.mkv output.mkv"
	if err := db.Model(&models.TestEncode{}).Where("id = ?", test.ID).Update(testEncodeFFmpegCommandColumn, command).Error; err != nil {
		t.Fatalf("write FFmpeg command through migrated column: %v", err)
	}
	if err := db.First(&test, test.ID).Error; err != nil {
		t.Fatal(err)
	}
	if test.FFmpegCommand != command {
		t.Fatalf("FFmpeg command was not persisted: %#v", test)
	}
}

func TestTestEncodeValidationReportsTrackAndDurationMismatch(t *testing.T) {
	plan := MediaJobPlan{
		Profile: models.Profile{VideoCodec: "x265", AudioCodec: "copy"},
		Streams: MediaStreamInventory{Video: []MediaStream{{Index: 0}}, Audio: []MediaAudioStream{{Index: 1}}},
	}
	passed := testEncodeValidationReport(plan, MediaStreamInventory{Video: []MediaStream{{Index: 0}}, Audio: []MediaAudioStream{{Index: 1}}, Duration: 20}, 20)
	if passed["passed"] != true {
		t.Fatalf("equivalent sample did not validate: %#v", passed)
	}
	failed := testEncodeValidationReport(plan, MediaStreamInventory{Video: []MediaStream{{Index: 0}}, Duration: 45}, 20)
	if failed["passed"] != false || len(workerStringSlice(failed["warnings"])) < 2 {
		t.Fatalf("mismatched sample was not reported: %#v", failed)
	}
}

func TestPlannedTestEncodeOutputKeepsLibraryAssetDirectory(t *testing.T) {
	db := queueJobTestDB(t)
	library := models.Library{
		ID:              1,
		Name:            "Anime",
		SourcePath:      "/media/raw",
		DestinationPath: "/media/library/anime",
	}
	profile := models.Profile{Container: "mkv"}
	job := models.QueueJob{
		MediaPath: "/media/library/anime/Gurren laggan/asset.mkv",
		LibraryID: library.ID,
		ProfileID: 1,
	}
	retiredAt := time.Now()
	if err := db.Create(&models.QueueJob{
		MediaPath:            job.MediaPath,
		LibraryID:            library.ID,
		ProfileID:            1,
		Status:               JobStatusCompleted,
		PublishedPath:        "/media/library/anime/Your Name/Your Name.mkv",
		PublicationRetiredAt: &retiredAt,
	}).Error; err != nil {
		t.Fatal(err)
	}

	got := plannedTestEncodeOutputPath(db, job, library, profile)
	want := "/media/library/anime/Gurren laggan/asset.mkv"
	if got != want {
		t.Fatalf("Test Encode planned output=%q want=%q", got, want)
	}
}

func TestRecoverInterruptedTestEncodesFailsOnlyActiveWorkAndReleasesReservations(t *testing.T) {
	db := testEncodeTestDB(t, "test-encode-recovery")
	root := t.TempDir()
	library := models.Library{Name: "TV", DestinationPath: root}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	partial := filepath.Join(root, ".a.partial.mkv")
	if err := os.WriteFile(partial, []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	waiting := models.TestEncode{SourcePath: "/raw/a.mkv", LibraryID: library.ID, ConfigurationSource: "effective_asset", Status: testEncodeWaiting, Phase: "waiting", TemporaryPath: partial}
	ready := models.TestEncode{SourcePath: "/raw/b.mkv", LibraryID: library.ID, ConfigurationSource: "effective_asset", Status: testEncodeReady, Phase: "ready"}
	if err := db.Create(&waiting).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&ready).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.TaskReservation{OwnerType: testEncodeOwnerType, OwnerID: waiting.ID, AssetKey: waiting.SourcePath, State: "active"}).Error; err != nil {
		t.Fatal(err)
	}
	recoverInterruptedTestEncodes(db)
	if err := db.First(&waiting, waiting.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&ready, ready.ID).Error; err != nil {
		t.Fatal(err)
	}
	if waiting.Status != testEncodeFailed || waiting.Phase != "interrupted" || ready.Status != testEncodeReady {
		t.Fatalf("unexpected recovered states: waiting=%#v ready=%#v", waiting, ready)
	}
	var count int64
	if err := db.Model(&models.TaskReservation{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("reservation was not released: %d", count)
	}
	if _, err := os.Stat(partial); !os.IsNotExist(err) {
		t.Fatalf("interrupted partial remains: %v", err)
	}
}

func TestRemoveTestEncodeFilesStaysInsideLibraryAndIncludesSidecars(t *testing.T) {
	db := testEncodeTestDB(t, "test-encode-delete")
	root := t.TempDir()
	library := models.Library{Name: "TV", DestinationPath: root}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "Show - MVForge Test T1.mkv")
	sidecar := filepath.Join(root, "Show - MVForge Test T1.spa.srt")
	for _, path := range []string{output, sidecar} {
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	test := models.TestEncode{LibraryID: library.ID, OutputPath: output, SubtitleArtifacts: models.JSONList{map[string]any{"stagedPath": sidecar}}}
	if err := (AssetHandler{db: db}).removeTestEncodeFiles(test); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{output, sidecar} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, err=%v", path, err)
		}
	}
	outside := filepath.Join(t.TempDir(), "outside.mkv")
	if err := os.WriteFile(outside, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (AssetHandler{db: db}).removeTestEncodeFiles(models.TestEncode{LibraryID: library.ID, OutputPath: outside}); err == nil {
		t.Fatal("expected an outside-library deletion to be rejected")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("outside file was modified")
	}
}

func TestFailTestEncodeRemovesOwnedPartialOutputAndSidecars(t *testing.T) {
	db := testEncodeTestDB(t, "test-encode-failure-cleanup")
	root := t.TempDir()
	library := models.Library{Name: "TV", DestinationPath: root}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "Episode - MVForge Test T1.mkv")
	partial := filepath.Join(root, ".Episode - MVForge Test T1.partial.mkv")
	sidecar := filepath.Join(root, "Episode - MVForge Test T1.eng.srt")
	for _, path := range []string{output, partial, sidecar} {
		if err := os.WriteFile(path, []byte("generated"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	test := models.TestEncode{
		SourcePath: "/raw/episode.mkv", LibraryID: library.ID, ConfigurationSource: "effective_asset",
		Status: testEncodeRunning, Phase: "generating", OutputPath: output, TemporaryPath: partial,
		SubtitleArtifacts: models.JSONList{map[string]any{"stagedPath": sidecar}},
	}
	if err := db.Create(&test).Error; err != nil {
		t.Fatal(err)
	}
	(AssetHandler{db: db}).failTestEncode(test.ID, os.ErrInvalid)
	if err := db.First(&test, test.ID).Error; err != nil {
		t.Fatal(err)
	}
	if test.Status != testEncodeFailed || test.ErrorMessage == "" {
		t.Fatalf("unexpected failed state: %#v", test)
	}
	for _, path := range []string{output, partial, sidecar} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("failed artifact remains at %s: %v", path, err)
		}
	}
}

func TestActiveTestEncodePathsIncludesPartialAndArtifacts(t *testing.T) {
	db := testEncodeTestDB(t, "test-encode-inventory")
	now := time.Now().Add(time.Hour)
	test := models.TestEncode{
		SourcePath: "/raw/a.mkv", LibraryID: 1, ConfigurationSource: "effective_asset", Status: testEncodeRunning, Phase: "generating",
		OutputPath: "/library/A - MVForge Test T1.mkv", TemporaryPath: "/library/.A - MVForge Test T1.partial.mkv", ExpiresAt: &now,
		SubtitleArtifacts: models.JSONList{map[string]any{"stagedPath": "/library/A - MVForge Test T1.eng.srt"}},
	}
	if err := db.Create(&test).Error; err != nil {
		t.Fatal(err)
	}
	paths := activeTestEncodePaths(db)
	for _, path := range []string{test.OutputPath, test.TemporaryPath, "/library/A - MVForge Test T1.eng.srt"} {
		if !paths[filepath.Clean(path)] {
			t.Fatalf("missing inventory exclusion for %s", path)
		}
	}
}

func TestDecorateTestEncodeMarksChangedSourceStale(t *testing.T) {
	db := testEncodeTestDB(t, "test-encode-stale-source")
	path := filepath.Join(t.TempDir(), "episode.mkv")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	modified := info.ModTime()
	test := models.TestEncode{SourcePath: path, SourceSizeBytes: info.Size(), SourceModifiedAt: &modified, ConfigurationSource: "lab_draft", Status: testEncodeReady}
	if err := os.WriteFile(path, []byte("after and larger"), 0o644); err != nil {
		t.Fatal(err)
	}
	(AssetHandler{db: db}).decorateTestEncode(&test)
	if !test.Stale || test.StaleReason == "" {
		t.Fatalf("source mutation was not marked stale: %#v", test)
	}
}

func TestGenerateTestExternalSubtitleArtifactsUsesTestBasename(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	source := filepath.Join(root, "Episode.mkv")
	sidecar := filepath.Join(root, "Episode.eng.forced.srt")
	output := filepath.Join(root, "Episode - MVForge Test T9.mkv")
	if err := os.WriteFile(source, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	content := []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n")
	if err := os.WriteFile(sidecar, content, 0o644); err != nil {
		t.Fatal(err)
	}
	artifacts, err := generateTestExternalSubtitleArtifacts(context.Background(), MediaJobPlan{
		SourceAssetPath: source, OutputPath: output, SegmentStartSeconds: 30, SegmentDurationSeconds: 20,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "Episode - MVForge Test T9.eng.forced.srt")
	if len(artifacts) != 1 || artifacts[0].StagedPath != want || artifacts[0].Language != "eng" {
		t.Fatalf("unexpected external subtitle artifacts: %#v", artifacts)
	}
	if got, err := os.ReadFile(want); err != nil || string(got) != string(content) {
		t.Fatalf("external sidecar was not preserved beside test output: %q err=%v", got, err)
	}
}
