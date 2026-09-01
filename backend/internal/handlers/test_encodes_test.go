package handlers

import (
	"context"
	"fmt"
	"math"
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

func TestExternalSubtitleSampleUsesSamePrerollWindowAsMedia(t *testing.T) {
	args := externalSubtitleSampleArgs(MediaJobPlan{
		SegmentStartSeconds: 42.5, SegmentDurationSeconds: 20,
	}, ExternalSubtitle{Path: "/raw/movie.en.srt", Format: "srt"}, "/library/test.en.srt")
	command := shellJoin(args)
	assertContains(t, command, "-ss 37.5 -i /raw/movie.en.srt -ss 5 -t 20")
	assertContains(t, command, "-c:s srt -f srt /library/test.en.srt")

	early := shellJoin(externalSubtitleSampleArgs(MediaJobPlan{
		SegmentStartSeconds: 3, SegmentDurationSeconds: 20,
	}, ExternalSubtitle{Path: "/raw/movie.en.srt", Format: "srt"}, "/library/test.en.srt"))
	assertContains(t, early, "-i /raw/movie.en.srt -ss 3 -t 20")
	assertNotContains(t, early, "-ss 0")
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

	override, err := resolveLabTrackOverride(db, path, models.JSONMap{
		"scope": "path", "videoMode": "first", "audioMode": "languages",
		"audioLanguages": models.JSONList{"spa"}, "dropCommentary": true,
	}, AssetConversionOverrideState{})
	if err != nil {
		t.Fatal(err)
	}
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

func TestTestEncodeResolvesAutomaticGOPAfterFrozenCadence(t *testing.T) {
	db := testEncodeTestDB(t, "test-encode-cadence-before-gop")
	path := filepath.Clean("/media/raw/soft-telecine.mkv")
	if err := db.Create(&models.ScanResult{
		Path:         path,
		VideoStreams: models.JSONList{map[string]any{"avgFrameRate": "30000/1001"}},
		FrameStructureAnalysis: models.JSONMap{
			"averageGopLength": 72.0, "maxConsecutiveBFrames": 2, "confidence": "high",
		},
	}).Error; err != nil {
		t.Fatal(err)
	}
	analysis := CadenceAnalysis{
		Version: cadenceAnalysisVersion, Type: "soft_telecine", DeclaredFrameRate: "30000/1001",
		DeclaredFPS: 30000.0 / 1001.0, EffectivePictureRate: "24000/1001", EffectiveFPS: 24000.0 / 1001.0,
		Confidence: .99,
	}
	recommendation := CadenceRecommendation{
		Version: 1, Operation: "remove_soft_telecine", OutputFrameRate: "24000/1001", Confidence: .99,
	}
	job := models.QueueJob{ProfileSnapshot: models.JSONMap{
		cadenceAnalysisSnapshotKey:       cadenceAnalysisMap(analysis),
		cadenceRecommendationSnapshotKey: cadenceRecommendationMap(recommendation),
	}}
	requested := models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{
		"videoEncoder": "hevc_qsv", "cadenceMode": "auto", "frameStructureMode": "auto",
	}}
	effective, err := resolveTestEncodeVideoProfile(db, path, job, requested, AssetConversionOverrideState{})
	if err != nil {
		t.Fatal(err)
	}
	if got := workerStringValue(effective.WorkerConfig["effectiveOutputFrameRate"]); got != "24000/1001" {
		t.Fatalf("effective FPS=%q want 24000/1001", got)
	}
	recommendationEvidence := unknownRecord(effective.WorkerConfig["frameStructureRecommendation"])
	if recommendationEvidence == nil || !nearFPS(workerNumberValue(recommendationEvidence["fps"], 0), 24000.0/1001.0, .001) {
		t.Fatalf("Test Encode GOP used declared source FPS: %#v", effective.WorkerConfig)
	}
	gopFrames := workerIntValue(effective.WorkerConfig["frameStructureGopFrames"], 0)
	gopSeconds := workerNumberValue(recommendationEvidence["targetGopSeconds"], 0)
	if gopFrames <= 0 || math.Abs(gopSeconds-float64(gopFrames)/(24000.0/1001.0)) > .001 {
		t.Fatalf("GOP evidence is inconsistent: frames=%d recommendation=%#v", gopFrames, recommendationEvidence)
	}
	streams := MediaStreamInventory{Video: []MediaStream{{
		Width: 720, Height: 480, FrameRate: "30000/1001",
	}}}
	effective = resolveEffectiveVideoEncodingProfile(effective, streams, path)
	if got := workerStringValue(effective.WorkerConfig["hevcLevelEffective"]); got != "3.0" {
		t.Fatalf("HEVC Level=%q want 3.0 from effective 720x480@24000/1001", got)
	}
	if workerIntValue(effective.WorkerConfig["effectiveOutputWidth"], 0) != 720 || workerIntValue(effective.WorkerConfig["effectiveOutputHeight"], 0) != 480 {
		t.Fatalf("effective geometry was not frozen: %#v", effective.WorkerConfig)
	}
	command := shellJoin(append(videoCodecArgsForResolvedEncoder(effective, nil, "hevc_qsv"), cadenceOutputArgs(effective)...))
	assertContains(t, command, fmt.Sprintf("-g %d", gopFrames))
	assertContains(t, command, "-level:v 30")
	assertContains(t, command, "-tier main")
	assertContains(t, command, "-fps_mode cfr")
	if cadence := validateCadenceOutputFrameRate(map[string]interface{}(effective.WorkerConfig), models.JSONMap{
		"frameRate": "24000/1001", "realFrameRate": "24000/1001",
	}); cadence["status"] != "validated" {
		t.Fatalf("cadence validation=%#v want validated", cadence)
	}
	if level := validateHEVCLevelField(map[string]interface{}(effective.WorkerConfig), models.JSONMap{
		"width": 720, "height": 480, "frameRate": "30000/1001",
	}, models.JSONMap{
		"codec": "hevc", "hevcLevel": "3.0", "width": 720, "height": 480, "frameRate": "24000/1001",
	}); level["status"] != "validated" {
		t.Fatalf("HEVC Level validation=%#v want validated", level)
	}
}

func TestEffectiveDecisionSnapshotCommandAndValidationRemainConsistent(t *testing.T) {
	db := testEncodeTestDB(t, "effective-decision-consistency")
	path := filepath.Clean("/media/raw/soft-telecine-hd.mkv")
	if err := db.Create(&models.ScanResult{
		Path: path, VideoStreams: models.JSONList{map[string]any{"avgFrameRate": "30000/1001"}},
		FrameStructureAnalysis: models.JSONMap{"averageGopLength": 72.0, "maxConsecutiveBFrames": 2, "confidence": "high"},
	}).Error; err != nil {
		t.Fatal(err)
	}
	analysis := CadenceAnalysis{Version: cadenceAnalysisVersion, Type: "soft_telecine", DeclaredFrameRate: "30000/1001", DeclaredFPS: 30000.0 / 1001.0, EffectivePictureRate: "24000/1001", EffectiveFPS: 24000.0 / 1001.0, Confidence: .99}
	recommendation := CadenceRecommendation{Version: 1, Operation: "remove_soft_telecine", OutputFrameRate: "24000/1001", Confidence: .99}
	interlace := InterlaceAnalysis{Version: interlaceAnalysisVersion, Status: "progressive", RecommendedAction: "preserve"}
	job := models.QueueJob{MediaPath: path, ProfileSnapshot: models.JSONMap{
		interlaceAnalysisSnapshotKey: interlace, cadenceAnalysisSnapshotKey: cadenceAnalysisMap(analysis), cadenceRecommendationSnapshotKey: cadenceRecommendationMap(recommendation),
	}}
	requested := models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{
		"videoEncoder": "hevc_qsv", "cadenceMode": "auto", "frameStructureMode": "auto", "hevcLevelMode": "auto", "videoFilters": "scale=1280:720", "pixFmt": "p010le", "finalColorPolicy": "preserve",
	}}
	streams := MediaStreamInventory{Video: []MediaStream{{
		Index: 0, Width: 720, Height: 480, FrameRate: "30000/1001", PixelFormat: "yuv420p",
		ColorSpace: "bt709", ColorTransfer: "bt709", ColorPrimaries: "bt709", ColorRange: "tv",
	}}}
	previewProfile, err := resolvePreviewVideoProfile(db, path, requested, streams, interlace, analysis, recommendation)
	if err != nil {
		t.Fatal(err)
	}
	testProfile, err := resolveTestEncodeVideoProfile(db, path, job, requested, AssetConversionOverrideState{})
	if err != nil {
		t.Fatal(err)
	}
	testProfile = resolveMediaJobVideoProfile(testProfile, streams, path, interlace, analysis, recommendation)
	queueProfile, err := resolveQueueVideoProfile(db, path, job, requested, AssetConversionOverrideState{}, "")
	if err != nil {
		t.Fatal(err)
	}
	queueProfile = resolveMediaJobVideoProfile(queueProfile, streams, path, interlace, analysis, recommendation)

	decisions := map[string]models.JSONMap{
		"preview": effectiveVideoDecision(previewProfile), "test_encode": effectiveVideoDecision(testProfile), "queue": effectiveVideoDecision(queueProfile),
	}
	canonicalHash := configurationHash(decisions["queue"])
	for target, decision := range decisions {
		if configurationHash(decision) != canonicalHash {
			t.Fatalf("%s production entrypoint diverged: queue=%#v target=%#v", target, decisions["queue"], decision)
		}
		if workerIntValue(decision["gopFrames"], 0) <= 0 || workerIntValue(decision["maxBFrames"], 0) <= 0 {
			t.Fatalf("%s reached rendering without frozen automatic frame structure: %#v", target, decision)
		}
	}
	effective := queueProfile
	if workerStringValue(effective.WorkerConfig["effectiveOutputFrameRate"]) != "24000/1001" || workerIntValue(effective.WorkerConfig["effectiveOutputWidth"], 0) != 1280 || workerIntValue(effective.WorkerConfig["effectiveOutputHeight"], 0) != 720 || workerStringValue(effective.WorkerConfig["hevcLevelEffective"]) != "3.1" {
		t.Fatalf("effective snapshot is inconsistent: %#v", effective.WorkerConfig)
	}
	plan := MediaJobPlan{InputPath: path, OutputPath: "/tmp/test.mkv", Profile: effective, ProcessingMode: ProcessingModeFullEncode, Streams: streams, Interlace: interlace, Cadence: analysis, CadenceRecommendation: recommendation, Overwrite: true}
	decisionBeforeRender := configurationHash(effectiveVideoDecision(plan.Profile))
	command := shellJoin((FFmpegCommandBuilder{}).Build(plan))
	if decisionAfterRender := configurationHash(effectiveVideoDecision(plan.Profile)); decisionAfterRender != decisionBeforeRender {
		t.Fatalf("target rendering mutated the effective video decision: before=%s after=%s", decisionBeforeRender, decisionAfterRender)
	}
	assertContains(t, command, "-vf fps=24000/1001,scale=1280:720")
	if !strings.Contains(command, "-level:v 31") && !strings.Contains(command, "level-idc=3.1") {
		t.Fatalf("command did not emit HEVC Level 3.1: %s", command)
	}
	assertContains(t, command, fmt.Sprintf("-g %d", workerIntValue(effective.WorkerConfig["frameStructureGopFrames"], 0)))
	previewRendered := profileWithPreviewDisplayNormalization(previewProfile, "colorspace=space=bt709:primaries=bt709:trc=bt709")
	if configurationHash(effectiveVideoDecision(previewRendered)) != configurationHash(effectiveVideoDecision(previewProfile)) {
		t.Fatalf("Preview display normalization changed the shared effective decision: core=%#v rendered=%#v", effectiveVideoDecision(previewProfile), effectiveVideoDecision(previewRendered))
	}
	assertContains(t, workerStringValue(previewRendered.WorkerConfig["videoFilters"]), "colorspace=")
	assertNotContains(t, workerStringValue(testProfile.WorkerConfig["videoFilters"]), "colorspace=")
	assertNotContains(t, workerStringValue(queueProfile.WorkerConfig["videoFilters"]), "colorspace=")
	if level := validateHEVCLevelField(map[string]interface{}(effective.WorkerConfig), models.JSONMap{
		"width": 720, "height": 480, "frameRate": "30000/1001",
	}, models.JSONMap{
		"codec": "hevc", "hevcLevel": "3.1", "width": 1280, "height": 720, "frameRate": "24000/1001",
	}); level["status"] != "validated" {
		t.Fatalf("validation disagrees with effective snapshot and command: %#v", level)
	}
}

func TestSmartUpscaleDecisionMatchesPreviewTestEncodeAndQueue(t *testing.T) {
	db := testEncodeTestDB(t, "smart-upscale-entrypoint-parity")
	path := filepath.Clean("/media/raw/anamorphic-sd.mkv")
	interlace := InterlaceAnalysis{Version: interlaceAnalysisVersion, Status: "progressive", Confidence: .99, RecommendedAction: "preserve"}
	cadence := CadenceAnalysis{Version: cadenceAnalysisVersion, Type: "native", Confidence: .99}
	recommendation := CadenceRecommendation{Version: 1, Operation: "preserve", Confidence: .99}
	job := models.QueueJob{MediaPath: path, ProfileSnapshot: models.JSONMap{
		interlaceAnalysisSnapshotKey: interlace, cadenceAnalysisSnapshotKey: cadenceAnalysisMap(cadence), cadenceRecommendationSnapshotKey: cadenceRecommendationMap(recommendation),
	}}
	requested := models.Profile{VideoCodec: "x265", WorkerConfig: models.JSONMap{
		"videoEncoder": "libx265", "upscaleMode": "auto", "upscaleSharpen": "light", "frameStructureMode": "balanced", "hevcLevelMode": "auto", "finalColorPolicy": "preserve",
		"videoFilters": "setfield=prog,hqdn3d=2:2:7:7,deblock=filter=strong:block=8",
	}}
	streams := MediaStreamInventory{Video: []MediaStream{{
		Width: 720, Height: 480, SampleAspectRatio: "32:27", DisplayAspectRatio: "16:9", FrameRate: "30000/1001",
		ColorSpace: "smpte170m", ColorTransfer: "smpte170m", ColorPrimaries: "smpte170m", ColorRange: "tv",
	}}}

	preview, err := resolvePreviewVideoProfile(db, path, requested, streams, interlace, cadence, recommendation)
	if err != nil {
		t.Fatal(err)
	}
	testEncode, err := resolveTestEncodeVideoProfile(db, path, job, requested, AssetConversionOverrideState{})
	if err != nil {
		t.Fatal(err)
	}
	testEncode = resolveMediaJobVideoProfile(testEncode, streams, path, interlace, cadence, recommendation)
	queue, err := resolveQueueVideoProfile(db, path, job, requested, AssetConversionOverrideState{}, "")
	if err != nil {
		t.Fatal(err)
	}
	queue = resolveMediaJobVideoProfile(queue, streams, path, interlace, cadence, recommendation)

	want, ok := resolvedUpscaleDecisionFromProfile(queue)
	if !ok || !want.UpscaleApplied || want.TargetWidth != 1280 || want.TargetHeight != 720 {
		t.Fatalf("queue decision=%#v", want)
	}
	for name, profile := range map[string]models.Profile{"preview": preview, "test_encode": testEncode} {
		got, ok := resolvedUpscaleDecisionFromProfile(profile)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s upscale diverged: got=%#v queue=%#v", name, got, want)
		}
	}
	wantFilter := argumentValue(videoWorkerArgsForSource(queue, &streams.Video[0]), "-vf")
	for name, profile := range map[string]models.Profile{"preview": preview, "test_encode": testEncode} {
		if got := argumentValue(videoWorkerArgsForSource(profile, &streams.Video[0]), "-vf"); got != wantFilter {
			t.Fatalf("%s rendered Smart Upscale filter diverged: got=%q queue=%q", name, got, wantFilter)
		}
	}
	if wantFilter != "deblock=filter=strong:block=8,hqdn3d=2:2:7:7,scale=1280:720:flags=lanczos,setsar=1,cas=strength=0.10,setfield=prog" {
		t.Fatalf("unexpected shared Smart Upscale filter=%q", wantFilter)
	}
}

func TestResolvedRestorationPlanMatchesPreviewTestEncodeAndQueue(t *testing.T) {
	db := testEncodeTestDB(t, "restoration-entrypoint-parity")
	path := filepath.Clean("/media/raw/restoration-parity.mkv")
	interlace := InterlaceAnalysis{Version: interlaceAnalysisVersion, Status: "interlaced", Confidence: .99, DetectedFieldOrder: "bff", RecommendedAction: "deinterlace", RecommendedMode: "bwdif_bff"}
	cadence := CadenceAnalysis{Version: cadenceAnalysisVersion, Type: "native", Confidence: .99}
	recommendation := CadenceRecommendation{Version: 1, Operation: "preserve", Confidence: .99}
	job := models.QueueJob{MediaPath: path, ProfileSnapshot: models.JSONMap{
		interlaceAnalysisSnapshotKey: interlace, cadenceAnalysisSnapshotKey: cadenceAnalysisMap(cadence), cadenceRecommendationSnapshotKey: cadenceRecommendationMap(recommendation),
	}}
	requested := exactRestorationProfile()
	requested.WorkerConfig["fieldStructureMode"] = "deinterlace"
	requested.WorkerConfig["deinterlaceMode"] = "force"
	requested.WorkerConfig["deinterlaceFieldOrder"] = "bff"
	requested.WorkerConfig["finalColorPolicy"] = "preserve"
	streams := MediaStreamInventory{Video: []MediaStream{{
		Width: 720, Height: 480, SampleAspectRatio: "8:9", DisplayAspectRatio: "4:3", FrameRate: "30000/1001",
		ColorSpace: "smpte170m", ColorTransfer: "smpte170m", ColorPrimaries: "smpte170m", ColorRange: "tv", FieldOrder: "bb",
	}}}

	preview, err := resolvePreviewVideoProfile(db, path, requested, streams, interlace, cadence, recommendation)
	if err != nil {
		t.Fatal(err)
	}
	testEncode, err := resolveTestEncodeVideoProfile(db, path, job, requested, AssetConversionOverrideState{})
	if err != nil {
		t.Fatal(err)
	}
	testEncode = resolveMediaJobVideoProfile(testEncode, streams, path, interlace, cadence, recommendation)
	queue, err := resolveQueueVideoProfile(db, path, job, requested, AssetConversionOverrideState{}, "")
	if err != nil {
		t.Fatal(err)
	}
	queue = resolveMediaJobVideoProfile(queue, streams, path, interlace, cadence, recommendation)

	want, ok := resolvedRestorationPlanFromProfile(queue)
	if !ok {
		t.Fatal("Queue restoration plan missing")
	}
	for name, profile := range map[string]models.Profile{"preview": preview, "test_encode": testEncode} {
		got, ok := resolvedRestorationPlanFromProfile(profile)
		if !ok || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s restoration plan diverged: got=%#v queue=%#v", name, got, want)
		}
		if filter := argumentValue(videoWorkerArgsForSource(profile, &streams.Video[0]), "-vf"); filter != want.ResolvedFilterChain {
			t.Fatalf("%s rendered chain=%q queue plan=%q", name, filter, want.ResolvedFilterChain)
		}
	}
	for _, fragment := range []string{"bwdif=mode=send_frame:parity=bff", "deblock=filter=strong:block=8", "crop=704:448:8:16", "chromanr=thres=25:sizew=3:sizeh=3", "hqdn3d=4:3:6:4.5", "deband=1thr=0.024", "exposure=exposure=0.12", "saturation=0.96:gamma=0.94", "zscale=w=960:h=720", "setsar=1", "cas=strength=0.16", "setfield=prog"} {
		if !strings.Contains(want.ResolvedFilterChain, fragment) {
			t.Fatalf("shared restoration chain missing %q: %s", fragment, want.ResolvedFilterChain)
		}
	}
}

func TestMediaJobPlanWithOverridePreservesResolvedVideoDecisionThroughRender(t *testing.T) {
	db := testEncodeTestDB(t, "media-job-plan-override-parity")
	path := filepath.Clean("/media/raw/soft-telecine.mkv")
	if err := db.Create(&models.ScanResult{
		Path: path, VideoStreams: models.JSONList{map[string]any{"avgFrameRate": "30000/1001"}},
		FrameStructureAnalysis: models.JSONMap{"averageGopLength": 72.0, "maxConsecutiveBFrames": 2, "confidence": "high"},
	}).Error; err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	ffprobePath := filepath.Join(binDir, "ffprobe")
	probeJSON := `{"format":{"duration":"120.0","bit_rate":"5000000"},"streams":[{"index":0,"codec_type":"video","codec_name":"h264","field_order":"progressive","pix_fmt":"yuv420p","width":720,"height":480,"avg_frame_rate":"30000/1001","r_frame_rate":"30000/1001","sample_aspect_ratio":"1:1","display_aspect_ratio":"3:2"}]}`
	probeScript := "#!/bin/sh\nprintf '%s\\n' '" + probeJSON + "'\n"
	if err := os.WriteFile(ffprobePath, []byte(probeScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	analysis := CadenceAnalysis{Version: cadenceAnalysisVersion, Type: "soft_telecine", DeclaredFrameRate: "30000/1001", DeclaredFPS: 30000.0 / 1001.0, EffectivePictureRate: "24000/1001", EffectiveFPS: 24000.0 / 1001.0, Confidence: .99}
	recommendation := CadenceRecommendation{Version: 1, Operation: "remove_soft_telecine", OutputFrameRate: "24000/1001", Confidence: .99}
	interlace := InterlaceAnalysis{Version: interlaceAnalysisVersion, Status: "progressive", RecommendedAction: "preserve"}
	requested := models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{
		"videoEncoder": "hevc_qsv", "cadenceMode": "auto", "frameStructureMode": "auto", "hevcLevelMode": "auto", "pixFmt": "p010le", "finalColorPolicy": "preserve",
		interlaceAnalysisSnapshotKey: interlace, cadenceAnalysisSnapshotKey: cadenceAnalysisMap(analysis), cadenceRecommendationSnapshotKey: cadenceRecommendationMap(recommendation),
	}}
	override := AssetConversionOverrideState{VideoFilters: "scale=1280:720"}
	streams := MediaStreamInventory{Video: []MediaStream{{Index: 0, Codec: "h264", FieldOrder: "progressive", PixelFormat: "yuv420p", Width: 720, Height: 480, FrameRate: "30000/1001"}}, Duration: 120, TotalBitrate: 5_000_000}
	job := models.QueueJob{MediaPath: path, ProfileSnapshot: models.JSONMap{
		interlaceAnalysisSnapshotKey: interlace, cadenceAnalysisSnapshotKey: cadenceAnalysisMap(analysis), cadenceRecommendationSnapshotKey: cadenceRecommendationMap(recommendation),
	}}

	effective, err := resolveQueueVideoProfile(db, path, job, requested, override, "")
	if err != nil {
		t.Fatal(err)
	}
	effective = resolveMediaJobVideoProfile(effective, streams, path, interlace, analysis, recommendation)
	before := effectiveVideoDecision(effective)
	if workerStringValue(before["effectiveFrameRate"]) != "24000/1001" || workerIntValue(before["effectiveWidth"], 0) != 1280 || workerIntValue(before["effectiveHeight"], 0) != 720 || workerStringValue(before["hevcLevel"]) != "3.1" {
		t.Fatalf("pre-plan effective decision is incomplete: %#v", before)
	}
	if workerIntValue(before["gopFrames"], 0) <= 0 {
		t.Fatalf("pre-plan effective decision has no automatic GOP: %#v", before)
	}

	plan, err := buildMediaJobPlanWithOverride(path, "/tmp/test.mkv", effective, nil, true, override)
	if err != nil {
		t.Fatal(err)
	}
	after := effectiveVideoDecision(plan.Profile)
	if configurationHash(after) != configurationHash(before) {
		t.Fatalf("final MediaJobPlan changed the resolved video decision: before=%#v after=%#v", before, after)
	}
	if got := workerStringValue(plan.Profile.WorkerConfig["videoFilters"]); !strings.Contains(got, "scale=1280:720") {
		t.Fatalf("asset override was not retained in final plan: videoFilters=%q", got)
	}

	decisionBeforeRender := configurationHash(after)
	command := shellJoin((FFmpegCommandBuilder{}).Build(plan))
	if decisionAfterRender := configurationHash(effectiveVideoDecision(plan.Profile)); decisionAfterRender != decisionBeforeRender {
		t.Fatalf("rendering mutated final MediaJobPlan: before=%s after=%s", decisionBeforeRender, decisionAfterRender)
	}
	assertContains(t, command, "-vf fps=24000/1001,scale=1280:720")
	assertContains(t, command, fmt.Sprintf("-g %d", workerIntValue(after["gopFrames"], 0)))
	if !strings.Contains(command, "-level:v 31") && !strings.Contains(command, "level-idc=3.1") {
		t.Fatalf("command did not render the effective HEVC Level 3.1: %s", command)
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
		Streams: MediaStreamInventory{Video: []MediaStream{{Index: 0, FrameRate: "24/1", StartTimeValid: true}}, Audio: []MediaAudioStream{{Index: 1, StartTimeValid: true}}},
	}
	passed := testEncodeValidationReport(plan, MediaStreamInventory{Video: []MediaStream{{Index: 0, FrameRate: "24/1", StartTimeValid: true}}, Audio: []MediaAudioStream{{Index: 1, StartTimeValid: true}}, Duration: 20}, 20)
	if passed["passed"] != true {
		t.Fatalf("equivalent sample did not validate: %#v", passed)
	}
	failed := testEncodeValidationReport(plan, MediaStreamInventory{Video: []MediaStream{{Index: 0}}, Duration: 45}, 20)
	if failed["passed"] != false || len(workerStringSlice(failed["warnings"])) < 2 {
		t.Fatalf("mismatched sample was not reported: %#v", failed)
	}
}

func TestTestEncodeValidationReportsCadenceAndDecodedFrames(t *testing.T) {
	plan := MediaJobPlan{
		Profile: models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{"effectiveOutputFrameRate": "24000/1001"}},
		Streams: MediaStreamInventory{Video: []MediaStream{{Index: 0}}},
	}
	passed := testEncodeValidationReport(plan, MediaStreamInventory{
		Video: []MediaStream{{Index: 0, FrameRate: "24000/1001", RealFrameRate: "24000/1001", DecodedFrames: 480}}, Duration: 20,
	}, 20)
	if passed["passed"] != true {
		t.Fatalf("matching cadence did not validate: %#v", passed)
	}
	if cadence, ok := passed["cadence"].(models.JSONMap); !ok || cadence["status"] != "validated" {
		t.Fatalf("cadence evidence was not persisted: %#v", passed)
	}
	if frames, ok := passed["frameCount"].(models.JSONMap); !ok || frames["status"] != "validated" || frames["observed"] != int64(480) {
		t.Fatalf("frame-count evidence was not persisted: %#v", passed)
	}

	failed := testEncodeValidationReport(plan, MediaStreamInventory{
		Video: []MediaStream{{Index: 0, FrameRate: "24000/1001", RealFrameRate: "30000/1001", DecodedFrames: 600}}, Duration: 20,
	}, 20)
	if failed["passed"] != false {
		t.Fatalf("incorrect cadence and frame count passed: %#v", failed)
	}
}

func TestTestEncodeValidationRejectsLargeIntroducedAVOffset(t *testing.T) {
	plan := MediaJobPlan{
		Profile: models.Profile{VideoCodec: "hevc"},
		Streams: MediaStreamInventory{
			Video: []MediaStream{{Index: 0, FrameRate: "24000/1001", StartTime: 0, StartTimeValid: true}},
			Audio: []MediaAudioStream{{Index: 1, StartTime: 0, StartTimeValid: true}},
		},
	}
	report := testEncodeValidationReport(plan, MediaStreamInventory{
		Video: []MediaStream{{Index: 0, FrameRate: "24000/1001", StartTime: .563, StartTimeValid: true}},
		Audio: []MediaAudioStream{{Index: 1, StartTime: 0, StartTimeValid: true}}, Duration: 20,
	}, 20)
	if report["passed"] != false {
		t.Fatalf("large introduced A/V offset passed validation: %#v", report)
	}
	timing, ok := report["avTiming"].(models.JSONMap)
	if !ok || timing["status"] != "mismatch" {
		t.Fatalf("large A/V offset evidence missing: %#v", report)
	}
}

func TestTestEncodeValidationWarnsWhenAVTimingIsUnverified(t *testing.T) {
	plan := MediaJobPlan{
		Profile: models.Profile{VideoCodec: "hevc"},
		Streams: MediaStreamInventory{
			Video: []MediaStream{{Index: 0, FrameRate: "24000/1001"}},
			Audio: []MediaAudioStream{{Index: 1}},
		},
	}
	report := testEncodeValidationReport(plan, MediaStreamInventory{
		Video: []MediaStream{{Index: 0, FrameRate: "24000/1001"}},
		Audio: []MediaAudioStream{{Index: 1}}, Duration: 20,
	}, 20)
	if report["passed"] != false {
		t.Fatalf("missing expected A/V evidence silently passed: %#v", report)
	}
	timing := report["avTiming"].(models.JSONMap)
	if timing["status"] != "unverified" || len(workerStringSlice(report["warnings"])) == 0 {
		t.Fatalf("unverified A/V policy was not visible: %#v", report)
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
	if err := db.Create(&models.TaskReservation{OwnerType: legacyTestEncodeOwnerType, OwnerID: waiting.ID, AssetKey: waiting.SourcePath, State: "active"}).Error; err != nil {
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

func TestActivateTestSubtitleArtifactsMovesTemporarySidecarsAtomically(t *testing.T) {
	temp := t.TempDir()
	temporaryMedia := filepath.Join(temp, ".Movie - MVForge Test T1.partial.mkv")
	outputMedia := filepath.Join(temp, "Movie - MVForge Test T1.mkv")
	first := strings.TrimSuffix(temporaryMedia, filepath.Ext(temporaryMedia)) + ".spa.ass"
	second := strings.TrimSuffix(temporaryMedia, filepath.Ext(temporaryMedia)) + ".eng.forced.srt"
	if err := os.WriteFile(first, []byte("ass"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("srt"), 0o644); err != nil {
		t.Fatal(err)
	}
	artifacts := []SubtitleArtifact{{StagedPath: first}, {StagedPath: second}}
	if err := activateTestSubtitleArtifacts(artifacts, temporaryMedia, outputMedia); err != nil {
		t.Fatal(err)
	}
	for _, artifact := range artifacts {
		if strings.Contains(artifact.StagedPath, ".partial") {
			t.Fatalf("artifact retained temporary basename: %#v", artifacts)
		}
		if info, err := os.Stat(artifact.StagedPath); err != nil || info.IsDir() {
			t.Fatalf("activated artifact is missing: %s err=%v", artifact.StagedPath, err)
		}
	}
	for _, path := range []string{first, second} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("temporary sidecar still exists: %s err=%v", path, err)
		}
	}
}
