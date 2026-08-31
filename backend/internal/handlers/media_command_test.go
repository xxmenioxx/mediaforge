package handlers

import (
	"math"
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/capabilities"
	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/quality"
)

func TestFFmpegCommandBuilderAddsEnhancedAudioNonDestructively(t *testing.T) {
	plan := MediaJobPlan{
		InputPath:      "/media/raw/movie.mkv",
		OutputPath:     "/media/staging/job-1/movie.mkv",
		Overwrite:      true,
		ProcessingMode: ProcessingModeAudioOnly,
		Profile: models.Profile{
			VideoCodec:        "x265_10bit",
			AudioCodec:        "copy",
			QualityMode:       "crf",
			QualityValue:      20,
			PreserveSubtitles: true,
			PreserveChapters:  true,
		},
		AudioProfile: &audioEnhancementProfile{
			Key:            "dialogue",
			Filters:        "loudnorm=I=-18:TP=-2:LRA=11",
			OutputCodec:    "aac",
			ChannelMode:    "dual-mono",
			TargetLoudness: -18,
			TruePeak:       -2,
			EQBands:        map[string]float64{"12000": -1},
		},
		Streams: MediaStreamInventory{
			ChapterCount: 1,
			Audio: []MediaAudioStream{
				{Index: 1, Codec: "ac3", Channels: 1, ChannelLayout: "mono", Language: "jpn", Default: true},
			},
		},
	}

	args := FFmpegCommandBuilder{}.Build(plan)
	command := shellJoin(args)

	assertContains(t, command, "-map 0")
	assertContains(t, command, "-map 0:1")
	assertContains(t, command, "-c:a copy")
	assertNotContains(t, command, "-c:s copy")
	assertContains(t, command, "-c:v copy")
	assertContains(t, command, "-filter:a:1")
	assertContains(t, command, "-c:a:1 aac")
	assertContains(t, command, "-metadata:s:a:0 \"title=Original Japanese Mono\"")
	assertContains(t, command, "-metadata:s:a:0 language=jpn")
	assertContains(t, command, "-metadata:s:a:1 \"title=Stereo Enhanced (MVForge)\"")
	assertContains(t, command, "-disposition:a:0 0")
	assertContains(t, command, "-disposition:a:1 default")
	assertContains(t, command, "-map_chapters 0")
}

func TestFFmpegCommandBuilderFullEncodeUsesTenBitX265(t *testing.T) {
	plan := MediaJobPlan{
		InputPath:      "/media/raw/movie.mkv",
		OutputPath:     "/media/staging/job-1/movie.mkv",
		Overwrite:      true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{
			VideoCodec:        "x265_10bit",
			AudioCodec:        "copy",
			QualityMode:       "crf",
			QualityValue:      20,
			PreserveSubtitles: true,
			PreserveChapters:  true,
		},
		Streams: MediaStreamInventory{Audio: []MediaAudioStream{}},
	}

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))

	assertContains(t, command, "-c:v libx265")
	assertContains(t, command, "-pix_fmt yuv420p10le")
	assertContains(t, command, "-crf 20")
}

func TestFFmpegCommandBuilderAddsTestWindowAndSortedProvenanceMetadata(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/media/raw/movie.mkv", OutputPath: "/media/library/.movie.partial.mkv", Overwrite: true,
		ProcessingMode:      ProcessingModeFullEncode,
		Profile:             models.Profile{VideoCodec: "x265", AudioCodec: "copy", QualityMode: "crf", QualityValue: 20},
		Streams:             MediaStreamInventory{Video: []MediaStream{{Index: 0}}},
		SegmentStartSeconds: 125.5, SegmentDurationSeconds: 20,
		OutputMetadata: map[string]string{"MVFORGE_TEST_ID": "42", "MVFORGE_CONFIG_HASH": "abc"},
	}
	args := FFmpegCommandBuilder{}.Build(plan)
	command := shellJoin(args)
	assertContains(t, command, "-ss 120.5 -i /media/raw/movie.mkv -ss 5 -t 20")
	assertContains(t, command, "-metadata MVFORGE_CONFIG_HASH=abc -metadata MVFORGE_TEST_ID=42")
	if strings.Index(command, "-ss 120.5") > strings.Index(command, "-i /media/raw/movie.mkv") {
		t.Fatalf("input seek must precede the input: %s", command)
	}
}

func TestSegmentedSeekUsesAccurateTrimAfterBoundedPreroll(t *testing.T) {
	inputSeek, outputTrim := segmentedSeekWindow(3)
	if inputSeek != 0 || outputTrim != 3 {
		t.Fatalf("short seek=(%g,%g), want (0,3)", inputSeek, outputTrim)
	}
	inputSeek, outputTrim = segmentedSeekWindow(1380.3385)
	if inputSeek != 1375.3385 || outputTrim != 5 {
		t.Fatalf("long seek=(%g,%g), want bounded five-second preroll", inputSeek, outputTrim)
	}
}

func TestFFmpegCommandBuilderLeavesNormalJobsUnsegmented(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/media/raw/movie.mkv", OutputPath: "/media/staging/movie.mkv",
		ProcessingMode: ProcessingModeFullEncode,
		Profile:        models.Profile{VideoCodec: "x265", AudioCodec: "copy", QualityMode: "crf", QualityValue: 20},
		Streams:        MediaStreamInventory{Video: []MediaStream{{Index: 0}}},
	}
	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertNotContains(t, command, "-ss")
	assertNotContains(t, command, "-t 20")
	assertNotContains(t, command, "MVFORGE_TEST")
}

func TestFFmpegCommandBuilderEmitsEffectiveHEVCLevelForX265(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/media/raw/movie.mkv", OutputPath: "/media/staging/job-1/movie.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{
			VideoCodec: "x265_10bit", AudioCodec: "copy", QualityMode: "crf", QualityValue: 20,
			WorkerConfig: models.JSONMap{"hevcLevelMode": "recommended", "hevcLevelEffective": "4.0"},
		},
		Streams: MediaStreamInventory{Video: []MediaStream{{Index: 0}}},
	}
	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertContains(t, command, "-x265-params level-idc=4.0:high-tier=0")
}

func TestFFmpegCommandBuilderRendersResolvedSmartUpscaleTargets(t *testing.T) {
	tests := []struct {
		name       string
		decision   ResolvedUpscaleDecision
		filters    string
		wantFilter string
	}{
		{
			name: "disabled", decision: ResolvedUpscaleDecision{RequestedMode: UpscaleModeDisabled, ResolvedMode: ResolvedUpscaleKeepSource, TargetWidth: 720, TargetHeight: 480},
			wantFilter: "",
		},
		{
			name: "keep source", decision: ResolvedUpscaleDecision{RequestedMode: UpscaleModeAuto, ResolvedMode: ResolvedUpscaleKeepSource, TargetWidth: 720, TargetHeight: 480, SharpenMode: UpscaleSharpenMedium},
			wantFilter: "",
		},
		{
			name: "anamorphic 720p", decision: ResolvedUpscaleDecision{RequestedMode: UpscaleMode720p, ResolvedMode: ResolvedUpscale720p, TargetWidth: 1280, TargetHeight: 720, TargetSAR: "1:1", UpscaleApplied: true},
			wantFilter: "scale=1280:720:flags=lanczos,setsar=1",
		},
		{
			name: "four by three", decision: ResolvedUpscaleDecision{RequestedMode: UpscaleMode720p, ResolvedMode: ResolvedUpscale720p, TargetWidth: 960, TargetHeight: 720, TargetSAR: "1:1", UpscaleApplied: true},
			wantFilter: "scale=960:720:flags=lanczos,setsar=1",
		},
		{
			name: "explicit 1080p", decision: ResolvedUpscaleDecision{RequestedMode: UpscaleMode1080p, ResolvedMode: ResolvedUpscale1080p, TargetWidth: 1920, TargetHeight: 1080, TargetSAR: "1:1", UpscaleApplied: true},
			wantFilter: "scale=1920:1080:flags=lanczos,setsar=1",
		},
		{
			name: "custom", decision: ResolvedUpscaleDecision{RequestedMode: UpscaleModeCustom, ResolvedMode: ResolvedUpscaleCustom, TargetWidth: 1200, TargetHeight: 900, TargetSAR: "1:1", UpscaleApplied: true},
			wantFilter: "scale=1200:900:flags=lanczos,setsar=1",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile := models.Profile{VideoCodec: "x265_10bit", WorkerConfig: models.JSONMap{
				"videoEncoder": "libx265", "videoFilters": test.filters, "resolvedUpscaleDecision": test.decision,
			}}
			args := FFmpegCommandBuilder{}.Build(MediaJobPlan{
				InputPath: "/media/raw/dvd.mkv", OutputPath: "/media/staging/dvd.mkv", Overwrite: true,
				ProcessingMode: ProcessingModeFullEncode, Profile: profile,
				Streams: MediaStreamInventory{Video: []MediaStream{{Index: 0, Width: 720, Height: 480, SampleAspectRatio: "32:27"}}},
			})
			filter := argumentValue(args, "-vf")
			if filter != test.wantFilter {
				t.Fatalf("filter=%q want %q", filter, test.wantFilter)
			}
		})
	}
}

func TestFFmpegCommandBuilderOrdersSmartUpscaleAfterStructureAndCrop(t *testing.T) {
	tests := []struct {
		name    string
		filters string
		before  []string
	}{
		{name: "crop", filters: "crop=704:448:8:16,hqdn3d=2:2:6:6", before: []string{"crop="}},
		{name: "deinterlace", filters: "bwdif=mode=send_frame:parity=bff:deint=all,hqdn3d=2:2:6:6", before: []string{"bwdif="}},
		{name: "IVTC", filters: "fieldmatch=order=tff,decimate,setfield=prog,colorspace=space=bt709", before: []string{"fieldmatch=", "decimate", "setfield="}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile := models.Profile{VideoCodec: "x265_10bit", WorkerConfig: models.JSONMap{
				"videoEncoder": "libx265", "videoFilters": test.filters,
				"resolvedUpscaleDecision": ResolvedUpscaleDecision{RequestedMode: UpscaleModeAuto, ResolvedMode: ResolvedUpscale720p, TargetWidth: 1280, TargetHeight: 720, TargetSAR: "1:1", UpscaleApplied: true, SharpenMode: UpscaleSharpenLight},
			}}
			filter := argumentValue(videoWorkerArgsForSource(profile, &MediaStream{Width: 720, Height: 480, SampleAspectRatio: "32:27"}), "-vf")
			scaleIndex := strings.Index(filter, "scale=1280:720:flags=lanczos")
			casIndex := strings.Index(filter, "cas=strength=0.20")
			if scaleIndex < 0 || casIndex < scaleIndex {
				t.Fatalf("scale/CAS ordering=%q", filter)
			}
			for _, expected := range test.before {
				if index := strings.Index(filter, expected); index < 0 || index > scaleIndex {
					t.Fatalf("%s must precede scale: %q", expected, filter)
				}
			}
			if colorIndex := strings.Index(filter, "colorspace="); colorIndex >= 0 && colorIndex < casIndex {
				t.Fatalf("color conversion must remain after Smart Upscale/CAS: %q", filter)
			}
		})
	}
}

func TestFFmpegCommandBuilderMapsSmartUpscaleSharpenAndAvoidsDuplicateGeometry(t *testing.T) {
	tests := []struct {
		mode UpscaleSharpen
		cas  string
	}{
		{mode: UpscaleSharpenOff},
		{mode: UpscaleSharpenLight, cas: "cas=strength=0.20"},
		{mode: UpscaleSharpenMedium, cas: "cas=strength=0.35"},
	}
	for _, test := range tests {
		t.Run(string(test.mode), func(t *testing.T) {
			profile := models.Profile{VideoCodec: "x265_10bit", WorkerConfig: models.JSONMap{
				"videoEncoder": "libx265", "videoFilters": "scale=640:480,setsar=8/9,eq=contrast=1.05",
				"resolvedUpscaleDecision": ResolvedUpscaleDecision{RequestedMode: UpscaleMode720p, ResolvedMode: ResolvedUpscale720p, TargetWidth: 1280, TargetHeight: 720, TargetSAR: "1:1", UpscaleApplied: true, SharpenMode: test.mode},
			}}
			filter := argumentValue(videoWorkerArgsForSource(profile, &MediaStream{Width: 720, Height: 480, SampleAspectRatio: "32:27"}), "-vf")
			if strings.Count(filter, "scale=") != 1 || strings.Count(filter, "setsar=") != 1 || strings.Contains(filter, "scale=640:480") || strings.Contains(filter, "setsar=8/9") {
				t.Fatalf("duplicate/conflicting geometry=%q", filter)
			}
			if test.cas == "" {
				assertNotContains(t, filter, "cas=")
			} else {
				assertContains(t, filter, test.cas)
				if strings.Index(filter, test.cas) < strings.Index(filter, "setsar=1") {
					t.Fatalf("CAS must follow scale/SAR: %q", filter)
				}
			}
		})
	}

	zscaleProfile := models.Profile{VideoCodec: "x265_10bit", WorkerConfig: models.JSONMap{
		"videoEncoder": "libx265", "videoFilters": "zscale=w=640:h=480:matrix=bt709,setsar=8/9",
		"resolvedUpscaleDecision": ResolvedUpscaleDecision{RequestedMode: UpscaleMode720p, ResolvedMode: ResolvedUpscale720p, TargetWidth: 1280, TargetHeight: 720, TargetSAR: "1:1", UpscaleApplied: true},
	}}
	zscaleFilter := argumentValue(videoWorkerArgsForSource(zscaleProfile, &MediaStream{Width: 720, Height: 480, SampleAspectRatio: "32:27"}), "-vf")
	if strings.Count(zscaleFilter, "zscale=") != 1 || strings.Contains(zscaleFilter, ",scale=") || !strings.Contains(zscaleFilter, "zscale=w=1280:h=720:filter=lanczos:matrix=bt709") {
		t.Fatalf("zscale geometry was duplicated instead of composed: %q", zscaleFilter)
	}
}

func TestFFmpegCommandBuilderSmartUpscalePreservesQSVMain10PixelFormat(t *testing.T) {
	profile := models.Profile{VideoCodec: "x265_10bit", BitDepth: 10, WorkerConfig: models.JSONMap{
		"videoEncoder": "hevc_qsv", "pixFmt": "p010le",
		"resolvedUpscaleDecision": ResolvedUpscaleDecision{RequestedMode: UpscaleMode720p, ResolvedMode: ResolvedUpscale720p, TargetWidth: 1280, TargetHeight: 720, TargetSAR: "1:1", UpscaleApplied: true, SharpenMode: UpscaleSharpenLight},
	}}
	args := videoWorkerArgsForSource(profile, &MediaStream{Width: 720, Height: 480, SampleAspectRatio: "32:27"})
	if argumentValue(args, "-pix_fmt") != "p010le" || argumentValue(args, "-vf") != "scale=1280:720:flags=lanczos,setsar=1,cas=strength=0.20" {
		t.Fatalf("QSV Main10/upscale args=%#v", args)
	}
	assertNotContains(t, strings.Join(args, " "), "hwdownload")
}

func TestPreviewDisplayNormalizationKeepsSmartUpscaleSquarePixelOrder(t *testing.T) {
	profile := models.Profile{VideoCodec: "x265_10bit", WorkerConfig: models.JSONMap{
		"videoEncoder": "libx265", "videoFilters": "bwdif=mode=send_frame:parity=bff:deint=all", "effectiveFinalColorPolicy": "preserve",
		"resolvedUpscaleDecision": ResolvedUpscaleDecision{RequestedMode: UpscaleModeAuto, ResolvedMode: ResolvedUpscale720p, TargetWidth: 1280, TargetHeight: 720, TargetSAR: "1:1", UpscaleApplied: true, SharpenMode: UpscaleSharpenLight},
	}}
	profile = profileWithPreviewDisplayNormalization(profile, "colorspace=space=bt709:primaries=bt709:trc=bt709,setsar=32/27")
	filter := argumentValue(videoWorkerArgsForSource(profile, &MediaStream{Width: 720, Height: 480, SampleAspectRatio: "32:27"}), "-vf")
	want := "bwdif=mode=send_frame:parity=bff:deint=all,scale=1280:720:flags=lanczos,setsar=1,cas=strength=0.20,colorspace=space=bt709:primaries=bt709:trc=bt709"
	if filter != want || strings.Count(filter, "setsar=") != 1 {
		t.Fatalf("Preview Smart Upscale/display normalization=%q want %q", filter, want)
	}
}

func TestFFmpegCommandBuilderUsesSoftwareAndCodecDefaultWhenHardwareIsDisabled(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/media/raw/movie.mkv", OutputPath: "/media/staging/movie.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{
			VideoCodec: "x265", AudioCodec: "copy", QualityMode: "crf", QualityValue: 20,
			WorkerConfig: models.JSONMap{"videoEncoder": "hevc_qsv", "useHardwareIfAvailable": false, "pixFmt": "auto"},
		},
	}

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertContains(t, command, "-c:v libx265")
	assertNotContains(t, command, "hevc_qsv")
	assertNotContains(t, command, "-pix_fmt")
}

func TestAssetOverrideAppliesHardwareEncoderAndQuality(t *testing.T) {
	enabled := true
	disabled := false
	profile := applyAssetConversionOverrideToProfile(models.Profile{
		VideoCodec: "x265_10bit", AudioCodec: "copy", QualityMode: "crf", QualityValue: 20,
		WorkerConfig: models.JSONMap{"videoEncoder": "libx265", "useHardwareIfAvailable": false},
	}, AssetConversionOverrideState{
		OptimizationIntent:      "maximum_quality",
		UseHardwareIfAvailable:  &enabled,
		VideoEncoder:            "hevc_qsv",
		PreferredEncoder:        "hardware",
		GlobalQuality:           27,
		QSVRateControl:          "la_icq",
		QSVLookAheadDepth:       55,
		QSVExtendedBRC:          &enabled,
		QSVAdaptiveI:            &enabled,
		QSVAdaptiveB:            &disabled,
		HEVCLevelMode:           "custom",
		HEVCLevel:               "4.1",
		VideoToolboxBitrateMbps: 6,
		VideoToolboxMaxrateMbps: 8,
		VideoToolboxBufferMbps:  12,
		CropAspectPolicy:        "preserve_dar",
	})

	if profile.WorkerConfig["useHardwareIfAvailable"] != true ||
		profile.WorkerConfig["videoEncoder"] != "hevc_qsv" ||
		profile.WorkerConfig["preferredEncoder"] != "hardware" ||
		profile.WorkerConfig["globalQuality"] != 27 ||
		profile.WorkerConfig["qsvRateControl"] != "la_icq" ||
		profile.WorkerConfig["qsvLookAheadDepth"] != 55 ||
		profile.WorkerConfig["qsvExtendedBRC"] != true ||
		profile.WorkerConfig["qsvAdaptiveI"] != true ||
		profile.WorkerConfig["qsvAdaptiveB"] != false ||
		profile.WorkerConfig["hevcLevelMode"] != "custom" ||
		profile.WorkerConfig["hevcLevel"] != "4.1" ||
		profile.WorkerConfig["videoToolboxBitrateMbps"] != float64(6) ||
		profile.WorkerConfig["videoToolboxMaxrateMbps"] != float64(8) ||
		profile.WorkerConfig["videoToolboxBufferMbps"] != float64(12) {
		t.Fatalf("hardware overrides were not applied: %#v", profile.WorkerConfig)
	}
	if profile.WorkerConfig["cropAspectPolicy"] != "preserve_dar" {
		t.Fatalf("crop aspect policy override was not applied: %#v", profile.WorkerConfig)
	}
	if profile.OptimizationIntent != "maximum_quality" {
		t.Fatalf("optimization intent override was not applied: %#v", profile)
	}
}

func TestAssetProcessingPreferencePreventsContradictoryHardwareFlags(t *testing.T) {
	enabled := true
	software := applyAssetConversionOverrideToProfile(models.Profile{
		VideoCodec: "x265", WorkerConfig: models.JSONMap{},
	}, AssetConversionOverrideState{
		PreferredEncoder:       "software",
		UseHardwareIfAvailable: &enabled,
		VideoEncoder:           "hevc_qsv",
	})
	if software.WorkerConfig["useHardwareIfAvailable"] != false || software.WorkerConfig["videoEncoder"] != "auto" {
		t.Fatalf("software preference did not win: %#v", software.WorkerConfig)
	}

	auto := applyAssetConversionOverrideToProfile(models.Profile{
		VideoCodec: "x265", WorkerConfig: models.JSONMap{},
	}, AssetConversionOverrideState{
		PreferredEncoder: "auto",
		VideoEncoder:     "hevc_qsv",
	})
	if auto.WorkerConfig["useHardwareIfAvailable"] != true || auto.WorkerConfig["videoEncoder"] != "auto" {
		t.Fatalf("auto preference was not normalized: %#v", auto.WorkerConfig)
	}

	h264Auto := applyAssetConversionOverrideToProfile(models.Profile{
		VideoCodec: "x264", WorkerConfig: models.JSONMap{},
	}, AssetConversionOverrideState{PreferredEncoder: "auto"})
	if h264Auto.WorkerConfig["useHardwareIfAvailable"] != false || h264Auto.WorkerConfig["videoEncoder"] != "auto" {
		t.Fatalf("unsupported H.264 hardware path must remain software: %#v", h264Auto.WorkerConfig)
	}
}

func TestSoftwarePreferenceUsesEncoderMatchingSelectedVideoCodec(t *testing.T) {
	disabled := false
	for _, test := range []struct {
		codec   string
		encoder string
	}{
		{codec: "x264", encoder: "libx264"},
		{codec: "x265", encoder: "libx265"},
		{codec: "x265_10bit", encoder: "libx265"},
	} {
		profile := models.Profile{
			VideoCodec: test.codec,
			WorkerConfig: models.JSONMap{
				"preferredEncoder":       "software",
				"useHardwareIfAvailable": disabled,
				"videoEncoder":           "auto",
			},
		}
		if got := resolvedVideoEncoder(profile); got != test.encoder {
			t.Fatalf("codec %s resolved %s, want %s", test.codec, got, test.encoder)
		}
	}
}

func TestFFmpegCommandUsesH264SoftwareEncoderSelectedByProcessingPreference(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/media/raw/movie.mkv", OutputPath: "/media/staging/movie.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{
			VideoCodec: "x264", AudioCodec: "copy", QualityMode: "crf", QualityValue: 20,
			WorkerConfig: models.JSONMap{
				"preferredEncoder":       "software",
				"useHardwareIfAvailable": false,
				"videoEncoder":           "auto",
			},
		},
	}
	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertContains(t, command, "-c:v libx264")
	assertNotContains(t, command, "libx265")
	assertNotContains(t, command, "hevc_qsv")
}

func TestProcessingModeIsDerivedFromEffectiveVideoCodec(t *testing.T) {
	if got := mediaProcessingMode(models.Profile{VideoCodec: "x265"}); got != ProcessingModeFullEncode {
		t.Fatalf("x265 mode=%q, want %q", got, ProcessingModeFullEncode)
	}
	if got := mediaProcessingMode(models.Profile{VideoCodec: "copy"}); got != ProcessingModeAudioOnly {
		t.Fatalf("copy mode=%q, want %q", got, ProcessingModeAudioOnly)
	}
	if got := mediaProcessingMode(models.Profile{
		VideoCodec:   "x264",
		WorkerConfig: models.JSONMap{"processingMode": "audio_only"},
	}); got != ProcessingModeFullEncode {
		t.Fatalf("legacy Work mode overrode selected video codec: %q", got)
	}
}

func TestMismatchedHardwareEncoderFallsBackToSelectedCodecSoftware(t *testing.T) {
	profile := models.Profile{
		VideoCodec: "x264",
		WorkerConfig: models.JSONMap{
			"useHardwareIfAvailable": true,
			"videoEncoder":           "hevc_qsv",
		},
	}
	if got := resolvedVideoEncoder(profile); got != "libx264" {
		t.Fatalf("mismatched HEVC hardware encoder resolved %s, want libx264", got)
	}
}

func TestSoftwareAssetPreferenceUsesEncoderForSelectedVideoCodec(t *testing.T) {
	profile := applyAssetConversionOverrideToProfile(models.Profile{
		VideoCodec: "x265", AudioCodec: "copy", QualityMode: "crf", QualityValue: 20,
		WorkerConfig: models.JSONMap{"videoEncoder": "hevc_qsv", "useHardwareIfAvailable": true},
	}, AssetConversionOverrideState{
		VideoCodec:       "x264",
		PreferredEncoder: "software",
		VideoEncoder:     "hevc_qsv",
	})
	plan := MediaJobPlan{
		InputPath: "/media/raw/h264.mkv", OutputPath: "/media/staging/h264.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile:        profile,
	}
	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertContains(t, command, "-c:v libx264")
	assertNotContains(t, command, "libx265")
	assertNotContains(t, command, "hevc_qsv")
}

func TestDefaultQSVQualityUsesConservativeStartingPoint(t *testing.T) {
	tests := map[int]int{
		18: 16,
		20: 18,
		22: 21,
		24: 24,
		0:  18,
	}
	for crf, expected := range tests {
		if actual := defaultQSVQuality(crf); actual != expected {
			t.Fatalf("defaultQSVQuality(%d) = %d, expected %d", crf, actual, expected)
		}
	}
}

func TestQSVWorkerArgsApplyOnlyProbedFeatures(t *testing.T) {
	profile := models.Profile{VideoCodec: "x265", QualityMode: "crf", QualityValue: 20, BitDepth: 10, WorkerConfig: models.JSONMap{
		"pixFmt":            "p010le",
		"qsvRateControl":    "la_icq",
		"qsvExtendedBRC":    true,
		"qsvLookAheadDepth": 40,
		"qsvAdaptiveI":      true,
		"qsvAdaptiveB":      true,
	}}
	args := qsvWorkerArgsForCapability(profile, capabilities.EncoderCapability{
		ICQ: true, LookAhead: true, QSVLAICQMain10: true, ExtendedBRC: false,
		QSVAdaptiveIMain10: true, QSVAdaptiveBMain10: false,
	})
	command := strings.Join(args, " ")
	assertContains(t, command, "-look_ahead 1")
	assertContains(t, command, "-adaptive_i 1")
	assertNotContains(t, command, "-extbrc")
	assertNotContains(t, command, "-adaptive_b")
	assertContains(t, command, "-look_ahead_depth 40")
}

func TestQSVBFramesOffDisablesAdaptiveBAndSupportsPyramidPStrategy(t *testing.T) {
	profile := models.Profile{VideoCodec: "x265", BitDepth: 10, WorkerConfig: models.JSONMap{
		"videoEncoder": "hevc_qsv", "pixFmt": "p010le", "qsvRateControl": "icq",
		"qsvAdaptiveB": true, "qsvPStrategy": 2, "frameStructureBFrameMode": "off",
	}}
	capability := capabilities.EncoderCapability{QSVAdaptiveBMain10: true, TestedModes: map[string]bool{"qsvPStrategyPyramidMain10": true}}
	command := strings.Join(append(videoCodecArgsForResolvedEncoder(profile, nil, "hevc_qsv"), qsvWorkerArgsForCapability(profile, capability)...), " ")
	assertContains(t, command, "-bf 0")
	assertNotContains(t, command, "-adaptive_b")
	assertContains(t, command, "-p_strategy 2")
}

func TestQSVPyramidPStrategyRequiresBFramesOff(t *testing.T) {
	profile := models.Profile{BitDepth: 10, WorkerConfig: models.JSONMap{"pixFmt": "p010le", "qsvPStrategy": 2, "frameStructureBFrameMode": "custom", "frameStructureMaxBFrames": 3}}
	capability := capabilities.EncoderCapability{TestedModes: map[string]bool{"qsvPStrategyPyramidMain10": true}}
	assertNotContains(t, strings.Join(qsvWorkerArgsForCapability(profile, capability), " "), "-p_strategy")
}

func TestQSVCompatibleFrameStructureReachesEffectiveCommand(t *testing.T) {
	profile := models.Profile{VideoCodec: "x265", QualityMode: "crf", QualityValue: 20, BitDepth: 10, WorkerConfig: models.JSONMap{
		"pixFmt": "p010le", "frameStructureMode": "compatible", "frameStructureGopMode": "recommended", "frameStructureGopFrames": 75,
		"frameStructureBFrameMode": "off", "frameStructureMaxBFrames": 0, "qsvAdaptiveI": true, "qsvAdaptiveB": false, "qsvPStrategy": 1,
	}}
	capability := capabilities.EncoderCapability{QSVAdaptiveIMain10: true, TestedModes: map[string]bool{"qsvPStrategySimpleMain10": true}}
	command := strings.Join(append(videoCodecArgsForResolvedEncoder(profile, nil, "hevc_qsv"), qsvWorkerArgsForCapability(profile, capability)...), " ")
	assertContains(t, command, "-g 75")
	assertContains(t, command, "-bf 0")
	assertContains(t, command, "-adaptive_i 1")
	assertContains(t, command, "-p_strategy 1")
	assertNotContains(t, command, "-adaptive_b")
}

func TestQSVMain8EmitsValidatedAdaptiveFeatures(t *testing.T) {
	profile := models.Profile{BitDepth: 8, WorkerConfig: models.JSONMap{"pixFmt": "nv12", "qsvAdaptiveI": true, "qsvAdaptiveB": true}}
	capability := capabilities.EncoderCapability{QSVAdaptiveIMain8: true, QSVAdaptiveBMain8: true}
	command := strings.Join(qsvWorkerArgsForCapability(profile, capability), " ")
	assertContains(t, command, "-adaptive_i 1")
	assertContains(t, command, "-adaptive_b 1")
}

func TestQSVMain8UsesOnlyValidatedRateControlFeatures(t *testing.T) {
	profile := models.Profile{BitDepth: 8, WorkerConfig: models.JSONMap{"pixFmt": "nv12", "qsvRateControl": "vbr", "qsvExtendedBRC": true, "qsvLookAheadDepth": 40}}
	capability := capabilities.EncoderCapability{QSVVBRExtBRCMain8: true, QSVVBRLookAheadMain8: false}
	command := strings.Join(qsvWorkerArgsForCapability(profile, capability), " ")
	assertContains(t, command, "-extbrc 1")
	assertNotContains(t, command, "-look_ahead")
	assertNotContains(t, command, "-look_ahead_depth")
}

func TestQSVWorkerArgsUseContextualAdvancedCapabilities(t *testing.T) {
	base := models.Profile{BitDepth: 10, WorkerConfig: models.JSONMap{
		"pixFmt": "p010le", "qsvLookAheadDepth": 40,
		"qsvExtendedBRC": true, "qsvAdaptiveI": true, "qsvAdaptiveB": true,
	}}
	tests := []struct {
		name       string
		rate       string
		capability capabilities.EncoderCapability
		contains   []string
		absent     []string
	}{
		{
			name: "ICQ keeps independent adaptive features and rejects ExtBRC", rate: "icq",
			capability: capabilities.EncoderCapability{QSVAdaptiveIMain10: true, QSVAdaptiveBMain10: true, QSVFullCombination: false},
			contains:   []string{"-adaptive_i 1", "-adaptive_b 1"}, absent: []string{"-extbrc", "-look_ahead"},
		},
		{
			name: "VBR uses its validated ExtBRC combination", rate: "vbr",
			capability: capabilities.EncoderCapability{QSVVBRExtBRCMain10: true, QSVVBRLookAheadMain10: false},
			contains:   []string{"-extbrc 1"}, absent: []string{"-look_ahead", "-look_ahead_depth", "-adaptive_i", "-adaptive_b"},
		},
		{
			name: "CBR uses its validated ExtBRC combination", rate: "cbr",
			capability: capabilities.EncoderCapability{QSVCBRExtBRCMain10: true, QSVCBRLookAheadMain10: false},
			contains:   []string{"-extbrc 1"}, absent: []string{"-look_ahead", "-look_ahead_depth", "-adaptive_i", "-adaptive_b"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile := base
			profile.WorkerConfig = cloneJSONMap(base.WorkerConfig)
			profile.WorkerConfig["qsvRateControl"] = test.rate
			command := strings.Join(qsvWorkerArgsForCapability(profile, test.capability), " ")
			for _, expected := range test.contains {
				assertContains(t, command, expected)
			}
			for _, unexpected := range test.absent {
				assertNotContains(t, command, unexpected)
			}
		})
	}
}

func TestHardwareQualityPresetsNormalizeBeforeExecution(t *testing.T) {
	qsv := normalizeHardwareQualityPreset(models.Profile{WorkerConfig: models.JSONMap{
		"videoEncoder": "hevc_qsv", "hardwareQualityPreset": "best_quality", "hardwareQualityPresetScale": 2,
	}})
	if qsv.WorkerConfig["globalQuality"] != 25 || qsv.WorkerConfig["qsvRateControl"] != "icq" || qsv.WorkerConfig["pixFmt"] != "p010le" {
		t.Fatalf("unexpected QSV preset normalization: %#v", qsv.WorkerConfig)
	}
	if qsv.WorkerConfig["qsvAdaptiveI"] != false || qsv.WorkerConfig["qsvExtendedBRC"] != false {
		t.Fatalf("QSV preset must not opt into unverified advanced features: %#v", qsv.WorkerConfig)
	}
	videoToolbox := normalizeHardwareQualityPreset(models.Profile{WorkerConfig: models.JSONMap{
		"videoEncoder": "hevc_videotoolbox", "hardwareQualityPreset": "recommended", "hardwareQualityPresetScale": 2,
	}})
	if videoToolbox.WorkerConfig["videoToolboxProfile"] != "main10" || videoToolbox.WorkerConfig["pixFmt"] != "p010le" {
		t.Fatalf("unexpected VideoToolbox preset normalization: %#v", videoToolbox.WorkerConfig)
	}
	custom := normalizeHardwareQualityPreset(models.Profile{WorkerConfig: models.JSONMap{
		"videoEncoder": "hevc_qsv", "hardwareQualityPreset": "custom", "globalQuality": 19,
	}})
	if custom.WorkerConfig["globalQuality"] != 19 {
		t.Fatalf("custom preset must preserve manual controls: %#v", custom.WorkerConfig)
	}
}

func TestHardwareQualityPresetsTranslateLegacyNamesWithoutChangingIntent(t *testing.T) {
	profile := normalizeHardwareQualityPreset(models.Profile{WorkerConfig: models.JSONMap{
		"videoEncoder": "hevc_videotoolbox", "hardwareQualityPreset": "compact",
	}})
	if profile.WorkerConfig["hardwareQualityPreset"] != "recommended" || profile.WorkerConfig["hardwareQualityPresetScale"] != 2 {
		t.Fatalf("legacy Compact must become the recalibrated Recommended preset: %#v", profile.WorkerConfig)
	}
}

func TestHardwareQualityPresetsTranslateLegacyRecommendedToHighQuality(t *testing.T) {
	profile := normalizeHardwareQualityPreset(models.Profile{WorkerConfig: models.JSONMap{
		"videoEncoder": "hevc_qsv", "hardwareQualityPreset": "recommended",
	}})
	if profile.WorkerConfig["hardwareQualityPreset"] != "high_quality" || profile.WorkerConfig["globalQuality"] != 22 {
		t.Fatalf("legacy Recommended must map to the recalibrated High Quality preset: %#v", profile.WorkerConfig)
	}
}

func TestQSVHardwareQualityPresetScale(t *testing.T) {
	expected := map[string]int{
		"compact": 32, "medium": 30, "recommended": 28, "best_quality": 25,
		"high_quality": 22, "archive": 19, "master": 16,
	}
	for preset, quality := range expected {
		t.Run(preset, func(t *testing.T) {
			profile := normalizeHardwareQualityPreset(models.Profile{WorkerConfig: models.JSONMap{
				"videoEncoder": "hevc_qsv", "hardwareQualityPreset": preset, "hardwareQualityPresetScale": 2,
			}})
			if profile.WorkerConfig["globalQuality"] != quality {
				t.Fatalf("preset %s: expected global quality %d, got %#v", preset, quality, profile.WorkerConfig["globalQuality"])
			}
		})
	}
}

func TestQSVRecommendationStoresRequestedEffectiveAndFallback(t *testing.T) {
	profile := normalizeHardwareQualityPreset(models.Profile{WorkerConfig: models.JSONMap{
		"videoEncoder": "hevc_qsv", "hardwareQualityPreset": "recommended", "hardwareQualityPresetScale": 2,
	}})
	// Simulate an explicit LA-ICQ request after the named preset established its
	// safe ICQ baseline; the recommendation layer must still record a fallback.
	profile.WorkerConfig["qsvRateControl"] = "la_icq"
	intent := qualityIntentForMedia(profile, "/media/raw/anime/Rayearth/episode.mkv", MediaStreamInventory{
		Duration: 100, Video: []MediaStream{{Width: 1440, Height: 1080, Bitrate: 6_000_000}},
	})
	effective := applyQSVQualityRecommendation(profile, intent, capabilities.EncoderCapability{
		Usable: true, QSVICQMain8: true, QSVCQPMain8: true,
	})
	if effective.WorkerConfig["qsvRequestedRateControl"] != "la_icq" || effective.WorkerConfig["qsvEffectiveRateControl"] != "icq" || effective.WorkerConfig["qsvRateControlFallbackReason"] == "" {
		t.Fatalf("unexpected stored QSV recommendation: %#v", effective.WorkerConfig)
	}
	if effective.WorkerConfig["qsvRequestedGlobalQuality"] != 28 || effective.WorkerConfig["qsvEffectiveGlobalQuality"] != 27 {
		t.Fatalf("anime adjustment was not stored: %#v", effective.WorkerConfig)
	}
}

func TestQSVEffectiveRateControlGeneratesMatchingArguments(t *testing.T) {
	for _, test := range []struct {
		mode     string
		config   models.JSONMap
		contains []string
		excludes []string
	}{
		{"icq", models.JSONMap{"qsvEffectiveRateControl": "icq", "globalQuality": 25}, []string{"-global_quality 25"}, []string{"+qscale", "-b:v"}},
		{"la_icq", models.JSONMap{"qsvEffectiveRateControl": "la_icq", "globalQuality": 24}, []string{"-global_quality 24"}, []string{"+qscale", "-b:v"}},
		{"cqp", models.JSONMap{"qsvEffectiveRateControl": "cqp", "globalQuality": 23}, []string{"-global_quality 23", "-flags +qscale"}, []string{"-b:v"}},
		{"vbr", models.JSONMap{"qsvEffectiveRateControl": "vbr", "qsvTargetBitrate": int64(2_000_000), "qsvMaxrate": int64(3_000_000), "qsvBuffer": int64(4_000_000)}, []string{"-b:v 2000000", "-maxrate 3000000", "-bufsize 4000000"}, []string{"-global_quality"}},
		{"cbr", models.JSONMap{"qsvEffectiveRateControl": "cbr", "qsvTargetBitrate": int64(2_000_000), "qsvMaxrate": int64(2_000_000), "qsvBuffer": int64(4_000_000)}, []string{"-b:v 2000000", "-maxrate 2000000", "-bufsize 4000000"}, []string{"-global_quality"}},
	} {
		t.Run(test.mode, func(t *testing.T) {
			test.config["videoEncoder"] = "hevc_qsv"
			profile := models.Profile{VideoCodec: "x265", QualityMode: "crf", QualityValue: 20, WorkerConfig: test.config}
			command := strings.Join(qsvRateControlArgs(profile), " ")
			for _, expected := range test.contains {
				assertContains(t, command, expected)
			}
			for _, excluded := range test.excludes {
				assertNotContains(t, command, excluded)
			}
		})
	}
}

func TestQSVCustomQualityIsNotRewrittenByTranslator(t *testing.T) {
	profile := models.Profile{WorkerConfig: models.JSONMap{"videoEncoder": "hevc_qsv", "hardwareQualityPreset": "custom", "globalQuality": 23}}
	effective := applyQSVQualityRecommendation(profile, qualityIntentForMedia(profile, "/media/raw/anime/title.mkv", MediaStreamInventory{}), capabilities.EncoderCapability{QSVICQMain8: true})
	if effective.WorkerConfig["globalQuality"] != 23 {
		t.Fatalf("custom QSV quality must not be adjusted: %#v", effective.WorkerConfig)
	}
}

func TestVideoToolboxPresetUsesAdaptiveSourceBitrate(t *testing.T) {
	profile := models.Profile{WorkerConfig: models.JSONMap{"hardwareQualityPreset": "recommended"}}
	target, maxrate, buffer, ok := adaptiveVideoToolboxBitrate(profile, &MediaStream{Height: 480, Bitrate: 4_000_000})
	if !ok || target != 1330 || maxrate != 2000 || buffer != 3330 {
		t.Fatalf("unexpected adaptive VideoToolbox result: target=%d maxrate=%d buffer=%d ok=%v", target, maxrate, buffer, ok)
	}
	profile.WorkerConfig["hardwareQualityPreset"] = "custom"
	if _, _, _, ok := adaptiveVideoToolboxBitrate(profile, &MediaStream{Height: 480, Bitrate: 4_000_000}); ok {
		t.Fatal("custom VideoToolbox bitrate must not be replaced by source adaptation")
	}
	profile.WorkerConfig["hardwareQualityPreset"] = "recommended"
	target, _, _, ok = adaptiveVideoToolboxBitrate(profile, &MediaStream{Height: 480, Bitrate: 12_000_000})
	if !ok || target != 2060 {
		t.Fatalf("SD Recommended base must cap before the Auto efficiency adjustment, got %dk", target)
	}
}

func TestVideoToolboxRecommendationIsStoredOnEffectivePlan(t *testing.T) {
	profile := normalizeHardwareQualityPreset(models.Profile{VideoCodec: "x265", QualityMode: "crf", QualityValue: 20, WorkerConfig: models.JSONMap{
		"videoEncoder": "hevc_videotoolbox", "hardwareQualityPreset": "recommended", "hardwareQualityPresetScale": 2,
		"videoFilters": "crop=720:460:0:10",
	}})
	intent := qualityIntentForMedia(profile, "/media/raw/anime/Akira/movie.mkv", MediaStreamInventory{
		Duration: 100, Video: []MediaStream{{Width: 720, Height: 480, Bitrate: 4_000_000}},
		Audio: []MediaAudioStream{{Bitrate: 192_000}},
	})
	effective := applyVideoToolboxQualityRecommendation(profile, intent)
	if effective.WorkerConfig["videoToolboxBaseTargetKbps"] != int64(1_290) || effective.WorkerConfig["videoToolboxRecommendedTargetKbps"] != int64(1_330) || effective.WorkerConfig["videoToolboxBitrateMbps"] != 1.33 || effective.WorkerConfig["videoToolboxEffectiveProfile"] != "main10" || effective.WorkerConfig["videoToolboxEffectivePixelFormat"] != "p010le" || effective.WorkerConfig["videoToolboxEstimateConfidence"] != "medium" {
		t.Fatalf("unexpected stored VideoToolbox recommendation: %#v", effective.WorkerConfig)
	}
}

func TestCropPreservesSourcePixelAspectRatio(t *testing.T) {
	filters := applyCropAspectPolicy("crop=720:460:0:10", models.Profile{}, &MediaStream{Width: 720, Height: 480, SampleAspectRatio: "8:9"})
	assertContains(t, filters, "crop=720:460:0:10,setsar=8/9")
	assertNotContains(t, filters, "setdar=")
}

func TestExplicitNV12OverridesLegacyMain10ProfileMetadata(t *testing.T) {
	profile := models.Profile{
		VideoCodec: "x265_10bit",
		BitDepth:   10,
		WorkerConfig: models.JSONMap{
			"pixFmt": "nv12",
		},
	}
	if profileUsesTenBit(profile) {
		t.Fatal("explicit NV12 must select 8-bit HEVC Main even when legacy profile fields still say 10-bit")
	}
}

func TestFFmpegCommandBuilderAutomaticallyDeinterlacesDetectedVideo(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/media/raw/dvd.mkv", OutputPath: "/media/staging/dvd.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{
			VideoCodec: "x265_10bit", AudioCodec: "copy",
			WorkerConfig: models.JSONMap{"videoEncoder": "hevc_qsv", "deinterlaceMode": "auto"},
		},
		Interlace: InterlaceAnalysis{Status: "interlaced", FieldOrder: "tt"},
	}
	plan.Profile = resolveEffectiveVideoMotionProfile(plan.Profile, plan.Interlace, plan.Cadence, plan.CadenceRecommendation)

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertContains(t, command, "-vf bwdif=mode=send_frame:parity=auto:deint=all")
}

func TestFFmpegCommandBuilderCorrectsProgressiveFieldMetadataWithoutDeinterlacing(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/media/raw/dvd.mkv", OutputPath: "/media/staging/dvd.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{
			VideoCodec: "x265_10bit", AudioCodec: "copy",
			WorkerConfig: models.JSONMap{
				"deinterlaceMode": "off",
				"videoFilters":    "hqdn3d=1.5:1.5:6:6",
			},
		},
		Interlace: InterlaceAnalysis{Status: "progressive", ContainerFieldOrder: "tt", FieldOrderMismatch: true},
	}
	plan.Profile = resolveEffectiveVideoMotionProfile(plan.Profile, plan.Interlace, plan.Cadence, plan.CadenceRecommendation)

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertContains(t, command, "-vf hqdn3d=1.5:1.5:6:6,setfield=prog")
	assertNotContains(t, command, "bwdif")
}

func TestFFmpegCommandBuilderLeavesUnvalidatedTelecineForReview(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/media/raw/dvd.mkv", OutputPath: "/media/staging/dvd.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{
			VideoCodec: "x265_10bit", AudioCodec: "copy",
			WorkerConfig: models.JSONMap{"deinterlaceMode": "auto"},
		},
		Interlace: InterlaceAnalysis{Status: "telecine_suspected", DetectedFieldOrder: "bff"},
	}

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertNotContains(t, command, "bwdif=")
	assertNotContains(t, command, "fieldmatch=")
}

func TestFFmpegCommandBuilderAppliesOnlyValidatedAutomaticIVTC(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/media/raw/dvd.mkv", OutputPath: "/media/staging/dvd.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{
			VideoCodec: "x265_10bit", AudioCodec: "copy",
			WorkerConfig: models.JSONMap{"deinterlaceMode": "auto"},
		},
		Interlace: InterlaceAnalysis{
			Version: interlaceAnalysisVersion, Status: "telecine", RecommendedAction: "ivtc",
			RecommendedFilter: "fieldmatch=order=tff,decimate", AutomaticFilter: "fieldmatch=order=tff,decimate",
		},
	}
	plan.Profile = resolveEffectiveVideoMotionProfile(plan.Profile, plan.Interlace, plan.Cadence, plan.CadenceRecommendation)

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertContains(t, command, "-vf fieldmatch=order=tff,decimate,setfield=prog")
	assertNotContains(t, command, "bwdif=")
}

func TestFFmpegCommandBuilderOmitsAbsentPreservationOptions(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/media/raw/plain.mkv", OutputPath: "/media/staging/plain.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{
			VideoCodec: "x265_10bit", AudioCodec: "copy", PreserveSubtitles: true, PreserveChapters: true,
			WorkerConfig: models.JSONMap{"preferSrtSubtitles": true},
		},
		Streams: MediaStreamInventory{},
	}

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertNotContains(t, command, "-c:s copy")
	assertNotContains(t, command, "-sn")
	assertNotContains(t, command, "-map_chapters")
}

func TestFFmpegCommandBuilderDisablesOnlyExistingUnwantedFeatures(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/media/raw/dvd.mkv", OutputPath: "/media/staging/dvd.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile:        models.Profile{VideoCodec: "x265_10bit", AudioCodec: "copy"},
		Streams: MediaStreamInventory{
			Subtitle: []MediaStream{{Index: 2, Codec: "dvd_subtitle"}}, ChapterCount: 4,
		},
	}

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertContains(t, command, "-sn")
	assertContains(t, command, "-map_chapters -1")
}

func TestFFmpegCommandBuilderUsesSelectedStreamIndexes(t *testing.T) {
	plan := MediaJobPlan{
		InputPath:      "/media/raw/episode.mkv",
		OutputPath:     "/media/staging/job-2/episode.mkv",
		Overwrite:      true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{
			VideoCodec:        "x265_10bit",
			AudioCodec:        "copy",
			QualityMode:       "crf",
			QualityValue:      20,
			PreserveSubtitles: true,
			PreserveChapters:  true,
		},
		Streams: MediaStreamInventory{
			Video: []MediaStream{{Index: 0}, {Index: 3}},
			Audio: []MediaAudioStream{
				{Index: 1, Codec: "aac", Channels: 2, ChannelLayout: "stereo"},
				{Index: 2, Codec: "ac3", Channels: 6, ChannelLayout: "5.1"},
			},
			Subtitle: []MediaStream{{Index: 4}, {Index: 5}},
		},
		Override: AssetConversionOverrideState{
			KeepVideoStreams:    []int{0},
			KeepAudioStreams:    []int{2},
			KeepSubtitleStreams: []int{},
		},
	}

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))

	assertContains(t, command, "-map 0:0")
	assertContains(t, command, "-map 0:2")
	assertNotContains(t, command, "-map 0 ")
	assertNotContains(t, command, "-map 0:1")
	assertNotContains(t, command, "-map 0:4")
	assertNotContains(t, command, "-map 0:5")
}

func TestEnhancedAudioSourceUsesAssetOverride(t *testing.T) {
	streamIndex := 4
	streams := []MediaAudioStream{
		{Index: 1, Default: true, Language: "jpn"},
		{Index: 4, Language: "spa"},
	}

	if got := enhancedAudioSourceIndex(streams, AssetConversionOverrideState{EnhancedAudioSourceStreamIndex: &streamIndex}); got != 4 {
		t.Fatalf("expected enhanced source stream 4, got %d", got)
	}
}

func TestEnhancedAudioSourceFallsBackToDefaultTrack(t *testing.T) {
	streams := []MediaAudioStream{
		{Index: 1, Language: "jpn"},
		{Index: 4, Default: true, Language: "spa"},
	}

	if got := enhancedAudioSourceIndex(streams, AssetConversionOverrideState{}); got != 4 {
		t.Fatalf("expected default source stream 4, got %d", got)
	}
}

func TestAACStereoSourceUsesProfileSelection(t *testing.T) {
	plan := MediaJobPlan{
		Profile: models.Profile{WorkerConfig: models.JSONMap{"aacStereoSourceStreamIndex": 4}},
		Streams: MediaStreamInventory{Audio: []MediaAudioStream{
			{Index: 1, Default: true, Language: "jpn"},
			{Index: 4, Language: "spa"},
		}},
	}
	if got := aacStereoSourceIndex(plan); got != 4 {
		t.Fatalf("expected configured AAC source stream 4, got %d", got)
	}
}

func TestAACStereoSourceFallsBackWhenProfileSelectionIsMissing(t *testing.T) {
	plan := MediaJobPlan{
		Profile: models.Profile{WorkerConfig: models.JSONMap{"aacStereoSourceStreamIndex": 8}},
		Streams: MediaStreamInventory{Audio: []MediaAudioStream{{Index: 1, Default: true}}},
	}
	warnings := validateAACStereoSource(&plan)
	if got := aacStereoSourceIndex(plan); got != 1 {
		t.Fatalf("expected default AAC source stream 1, got %d", got)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "stream 8") {
		t.Fatalf("expected missing source warning, got %#v", warnings)
	}
}

func TestSanitizeConversionOverrideOmitsMissingStreamsAndMetadata(t *testing.T) {
	missingAudioSource := 8
	override := AssetConversionOverrideState{
		KeepVideoStreams:               []int{0},
		KeepAudioStreams:               []int{1, 8},
		KeepSubtitleStreams:            []int{3, 7},
		AudioMetadata:                  map[int]StreamMetadataOverride{1: {Title: "Japanese"}, 8: {Title: "Missing"}},
		SubtitleMetadata:               map[int]StreamMetadataOverride{3: {Language: "spa"}, 7: {Language: "eng"}},
		EnhancedAudioSourceStreamIndex: &missingAudioSource,
	}
	streams := MediaStreamInventory{
		Video:    []MediaStream{{Index: 0}},
		Audio:    []MediaAudioStream{{Index: 1, Default: true}},
		Subtitle: []MediaStream{{Index: 3}},
	}

	sanitized, warnings := sanitizeConversionOverride(override, streams)

	if len(sanitized.KeepAudioStreams) != 1 || sanitized.KeepAudioStreams[0] != 1 {
		t.Fatalf("unexpected sanitized audio streams: %#v", sanitized.KeepAudioStreams)
	}
	if len(sanitized.KeepSubtitleStreams) != 1 || sanitized.KeepSubtitleStreams[0] != 3 {
		t.Fatalf("unexpected sanitized subtitle streams: %#v", sanitized.KeepSubtitleStreams)
	}
	if _, exists := sanitized.AudioMetadata[8]; exists {
		t.Fatal("missing audio metadata was not removed")
	}
	if _, exists := sanitized.SubtitleMetadata[7]; exists {
		t.Fatal("missing subtitle metadata was not removed")
	}
	if sanitized.EnhancedAudioSourceStreamIndex != nil {
		t.Fatal("missing enhanced audio source was not reset")
	}
	if len(warnings) != 5 {
		t.Fatalf("expected five validation warnings, got %#v", warnings)
	}
}

func TestSanitizedMissingSubtitleIsNotMappedByFFmpeg(t *testing.T) {
	override, warnings := sanitizeConversionOverride(
		AssetConversionOverrideState{KeepVideoStreams: []int{0}, KeepSubtitleStreams: []int{4, 9}},
		MediaStreamInventory{Video: []MediaStream{{Index: 0}}, Subtitle: []MediaStream{{Index: 4}}},
	)
	plan := MediaJobPlan{
		InputPath: "/media/raw/movie.mkv", OutputPath: "/media/staging/movie.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile:        models.Profile{VideoCodec: "copy", AudioCodec: "copy", PreserveSubtitles: true},
		Streams:        MediaStreamInventory{Video: []MediaStream{{Index: 0}}, Subtitle: []MediaStream{{Index: 4}}},
		Override:       override,
	}

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertContains(t, command, "-map 0:4")
	assertNotContains(t, command, "-map 0:9")
	if len(warnings) != 1 || !strings.Contains(warnings[0], "subtitle stream 9") {
		t.Fatalf("unexpected warnings: %#v", warnings)
	}
}

func TestFFmpegCommandBuilderRemovesExportedSubtitleFromContainer(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/media/raw/movie.mkv", OutputPath: "/media/staging/movie.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile:        models.Profile{VideoCodec: "copy", AudioCodec: "copy", PreserveSubtitles: true},
		Streams: MediaStreamInventory{
			Video:    []MediaStream{{Index: 0}},
			Audio:    []MediaAudioStream{{Index: 1, Codec: "aac"}},
			Subtitle: []MediaStream{{Index: 2, Codec: "subrip"}, {Index: 3, Codec: "ass"}},
		},
		Override: AssetConversionOverrideState{
			SubtitleTransforms: []SubtitleTransform{{
				StreamIndex: 2, Format: "srt", RemoveEmbedded: true, MakeDefault: true, Language: "spa",
			}},
		},
	}

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertContains(t, command, "-map 0:0")
	assertContains(t, command, "-map 0:1")
	assertContains(t, command, "-map 0:3")
	assertNotContains(t, command, "-map 0:2")
	assertContains(t, command, "-c:a copy")
}

func TestSanitizeConversionOverrideSchedulesBitmapSubtitleOCR(t *testing.T) {
	override, warnings := sanitizeConversionOverride(
		AssetConversionOverrideState{SubtitleTransforms: []SubtitleTransform{{
			StreamIndex: 4, Format: "srt", RemoveEmbedded: true, Language: "spa",
		}}},
		MediaStreamInventory{Subtitle: []MediaStream{{Index: 4, Codec: "hdmv_pgs_subtitle"}}},
	)

	if len(override.SubtitleTransforms) != 1 {
		t.Fatalf("expected bitmap OCR transformation to remain scheduled: %#v", override.SubtitleTransforms)
	}
	if len(warnings) != 1 || !strings.Contains(strings.ToLower(warnings[0]), "ocr") {
		t.Fatalf("expected OCR warning, got %#v", warnings)
	}
}

func TestAssetOverrideExternalizesAllSubtitlesBeforeConversion(t *testing.T) {
	profile := applyAssetConversionOverrideToProfile(
		models.Profile{PreserveSubtitles: true, WorkerConfig: models.JSONMap{"subtitleOutputFormat": "source"}},
		AssetConversionOverrideState{ExternalSubtitleFormat: "ass"},
	)
	plan := MediaJobPlan{
		Profile: profile,
		Streams: MediaStreamInventory{Subtitle: []MediaStream{
			{Index: 2, Codec: "subrip", Language: "eng"},
			{Index: 3, Codec: "hdmv_pgs_subtitle", Language: "spa"},
		}},
	}

	applyProfileSubtitleExternalization(&plan)

	if profile.PreserveSubtitles {
		t.Fatal("externalizing subtitles must disable embedded subtitle preservation")
	}
	if len(plan.Override.SubtitleTransforms) != 2 {
		t.Fatalf("expected every subtitle to use the asset override, got %#v", plan.Override.SubtitleTransforms)
	}
	for _, transform := range plan.Override.SubtitleTransforms {
		if transform.Format != "ass" || !transform.RemoveEmbedded {
			t.Fatalf("unexpected subtitle externalization: %#v", transform)
		}
	}
}

func TestProfileExternalizesEverySubtitleWhenNoTrackProfileIsSelected(t *testing.T) {
	plan := MediaJobPlan{
		Profile: models.Profile{WorkerConfig: models.JSONMap{"externalSubtitleFormat": "srt", "subtitleOCRMode": "clean", "subtitleOCRLanguage": "spa"}},
		Streams: MediaStreamInventory{Subtitle: []MediaStream{
			{Index: 2, Codec: "subrip", Language: "eng", Default: true},
			{Index: 4, Codec: "dvd_subtitle", Language: "spa"},
		}},
	}

	applyProfileSubtitleExternalization(&plan)

	if len(plan.Override.SubtitleTransforms) != 2 {
		t.Fatalf("expected every subtitle to be externalized, got %#v", plan.Override.SubtitleTransforms)
	}
	for _, transform := range plan.Override.SubtitleTransforms {
		if transform.Format != "srt" || !transform.RemoveEmbedded {
			t.Fatalf("expected validated SRT externalization with embedded removal, got %#v", transform)
		}
		if transform.OCRMode != "clean" || transform.OCRLanguage != "spa" {
			t.Fatalf("expected profile OCR settings to reach every generated transform, got %#v", transform)
		}
	}
	if !plan.Override.SubtitleTransforms[0].MakeDefault {
		t.Fatalf("expected source default disposition to be retained in sidecar naming")
	}
}

func TestTrackProfileTakesPriorityOverVideoSubtitleExternalization(t *testing.T) {
	trackTransform := SubtitleTransform{StreamIndex: 4, Format: "ass", RemoveEmbedded: false, Language: "jpn"}
	plan := MediaJobPlan{
		Profile: models.Profile{WorkerConfig: models.JSONMap{"externalSubtitleFormat": "srt"}},
		Streams: MediaStreamInventory{Subtitle: []MediaStream{
			{Index: 2, Codec: "subrip", Language: "eng"},
			{Index: 4, Codec: "ass", Language: "jpn"},
		}},
		Override: AssetConversionOverrideState{
			TrackProfileKey:    "japanese-subs",
			SubtitleTransforms: []SubtitleTransform{trackTransform},
		},
	}

	applyProfileSubtitleExternalization(&plan)

	if len(plan.Override.SubtitleTransforms) != 1 || plan.Override.SubtitleTransforms[0] != trackTransform {
		t.Fatalf("video profile replaced the selected track profile: %#v", plan.Override.SubtitleTransforms)
	}
}

func TestEmptyTrackProfileStillDisablesGlobalSubtitleExternalization(t *testing.T) {
	plan := MediaJobPlan{
		Profile:  models.Profile{WorkerConfig: models.JSONMap{"externalSubtitleFormat": "ass"}},
		Streams:  MediaStreamInventory{Subtitle: []MediaStream{{Index: 2, Codec: "subrip"}}},
		Override: AssetConversionOverrideState{TrackProfileKey: "preserve-selected-tracks"},
	}

	applyProfileSubtitleExternalization(&plan)

	if len(plan.Override.SubtitleTransforms) != 0 {
		t.Fatalf("selected track profile must have complete priority: %#v", plan.Override.SubtitleTransforms)
	}
}

func TestTrackProfileKeepSelectionOverridesVideoRemovePolicy(t *testing.T) {
	plan := MediaJobPlan{
		Profile:  models.Profile{PreserveSubtitles: false, WorkerConfig: models.JSONMap{"externalSubtitleFormat": "remove"}},
		Streams:  MediaStreamInventory{Subtitle: []MediaStream{{Index: 2}, {Index: 4}}},
		Override: AssetConversionOverrideState{TrackProfileKey: "keep-spanish", KeepSubtitleStreams: []int{4}},
	}
	selected := selectedSubtitleStreams(plan)
	if len(selected) != 1 || selected[0].Index != 4 {
		t.Fatalf("tracks profile did not override video subtitle policy: %#v", selected)
	}
}

func TestDisabledVideoSubtitlePolicyPreservesSafelyWithoutTracksProfile(t *testing.T) {
	plan := MediaJobPlan{
		Profile: models.Profile{PreserveSubtitles: false, WorkerConfig: models.JSONMap{"externalSubtitleFormat": "disabled"}},
		Streams: MediaStreamInventory{Subtitle: []MediaStream{{Index: 2}}},
	}
	selected := selectedSubtitleStreams(plan)
	if len(selected) != 1 || selected[0].Index != 2 {
		t.Fatalf("disabled video policy should defer to safe preservation: %#v", selected)
	}
}

func TestFFmpegCommandBuilderUsesVideoToolboxBitrateAndMain10(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/media/raw/movie.mkv", OutputPath: "/media/staging/movie.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{
			VideoCodec: "x265_10bit", BitDepth: 10, AudioCodec: "copy", QualityMode: "crf", QualityValue: 20,
			WorkerConfig: models.JSONMap{
				"videoEncoder":            "hevc_videotoolbox",
				"useHardwareIfAvailable":  true,
				"videoToolboxBitrateMbps": 6,
				"videoToolboxMaxrateMbps": 8,
				"videoToolboxBufferMbps":  12,
				"videoPreset":             "medium",
				"pixFmt":                  "yuv420p10le",
			},
		},
	}

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	if !ffmpegEncoderAvailable("hevc_videotoolbox") {
		assertContains(t, command, "-c:v libx265")
		assertNotContains(t, command, "-c:v hevc_videotoolbox")
		return
	}
	assertContains(t, command, "-c:v hevc_videotoolbox")
	assertContains(t, command, "-b:v 6M")
	assertContains(t, command, "-maxrate 8M")
	assertContains(t, command, "-bufsize 12M")
	assertContains(t, command, "-profile:v main10")
	assertContains(t, command, "-pix_fmt p010le")
	assertNotContains(t, command, "-q:v")
	assertNotContains(t, command, "-global_quality")
	assertNotContains(t, command, "-pix_fmt yuv420p10le")
	assertNotContains(t, command, "-preset medium")
}

func TestVideoToolboxLegacyColorIsConvertedToBT709(t *testing.T) {
	profile := models.Profile{
		VideoCodec: "x265", AudioCodec: "copy",
		WorkerConfig: models.JSONMap{
			"finalColorPolicy": "automatic",
			"videoFilters":     "bwdif=mode=send_frame:parity=bff:deint=all,setfield=prog",
		},
	}
	source := MediaStream{
		ColorSpace: "bt470bg", ColorTransfer: "bt470m", ColorPrimaries: "bt470m", ColorRange: "tv",
	}
	effective := profileWithAutomaticVideoToolboxColorForEncoder(profile, source, "hevc_videotoolbox")
	filter := workerStringValue(effective.WorkerConfig["videoFilters"])
	assertContains(t, filter, "bwdif=mode=send_frame:parity=bff:deint=all,setfield=prog,colorspace=")
	assertContains(t, filter, "ispace=bt470bg")
	assertContains(t, filter, "iprimaries=bt470m")
	assertContains(t, filter, "itrc=bt470m")
	assertContains(t, filter, "space=bt709")
	colorArgs := shellJoin(videoColorMetadataArgs(effective))
	assertContains(t, colorArgs, "-colorspace bt709")
	assertContains(t, colorArgs, "-color_trc bt709")
	assertContains(t, colorArgs, "-color_primaries bt709")
	assertContains(t, colorArgs, "-color_range tv")
}

func TestVideoToolboxQualityPresetOptionsReachFFmpeg(t *testing.T) {
	profile := models.Profile{
		VideoCodec:   "x265",
		QualityMode:  "crf",
		QualityValue: 20,
		BitDepth:     10,
		WorkerConfig: models.JSONMap{
			"videoEncoder":                     "hevc_videotoolbox",
			"preferredEncoder":                 "hardware",
			"useHardwareIfAvailable":           true,
			"videoToolboxQualityProfile":       80,
			"videoToolboxBitrateMbps":          12,
			"videoToolboxMaxrateMbps":          16,
			"videoToolboxBufferMbps":           32,
			"videoToolboxProfile":              "main10",
			"pixFmt":                           "p010le",
			"videoToolboxGop":                  90,
			"videoToolboxRealtime":             false,
			"videoToolboxAllowFrameReordering": true,
			"videoToolboxPowerEfficiency":      false,
		},
	}
	if resolvedVideoEncoder(profile) != "hevc_videotoolbox" {
		t.Skip("VideoToolbox is listed but not usable in this test runtime")
	}
	command := shellJoin(videoCodecArgs(profile))
	for _, expected := range []string{"-b:v 12M", "-maxrate 16M", "-bufsize 32M", "-g 90", "-bf 3", "-realtime 0", "-power_efficient 0", "-profile:v main10", "-pix_fmt p010le"} {
		assertContains(t, command, expected)
	}
	assertNotContains(t, command, "-q:v")
	assertNotContains(t, command, "-allow_frame_reordering")
}

func TestVideoToolboxRealtimeAndBFramePoliciesAreDistinct(t *testing.T) {
	capability := capabilities.EncoderCapability{
		VideoToolboxBFrames: true, VideoToolboxBFramesDisabled: true,
		TestedModes: map[string]bool{"videoToolboxBFramesMain10": true, "videoToolboxBFramesDisabledMain10": true},
	}
	tests := []struct {
		name     string
		config   models.JSONMap
		contains []string
		absent   []string
	}{
		{name: "offline auto", config: models.JSONMap{}, contains: []string{"-realtime 0"}, absent: []string{"-bf"}},
		{name: "realtime auto disables", config: models.JSONMap{"videoToolboxRealtime": true}, contains: []string{"-realtime 1", "-bf 0"}},
		{name: "enabled count", config: models.JSONMap{"videoToolboxBFramePolicy": "enabled", "videoToolboxBFrames": 4}, contains: []string{"-realtime 0", "-bf 4"}},
		{name: "disabled", config: models.JSONMap{"videoToolboxBFramePolicy": "disabled"}, contains: []string{"-realtime 0", "-bf 0"}},
		{name: "legacy missing false is auto", config: models.JSONMap{"videoToolboxAllowFrameReordering": false}, contains: []string{"-realtime 0"}, absent: []string{"-bf"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile := models.Profile{WorkerConfig: test.config}
			command := shellJoin(videoToolboxRealtimeAndBFrameArgs(profile, capability, false))
			for _, expected := range test.contains {
				assertContains(t, command, expected)
			}
			for _, absent := range test.absent {
				assertNotContains(t, command, absent)
			}
		})
	}
}

func TestFrameStructureBFrameModesFeedVideoToolboxQualityIntent(t *testing.T) {
	tests := []struct {
		mode  string
		count int
		want  string
	}{
		{mode: "auto", want: "auto"},
		{mode: "recommended", count: 3, want: "enabled"},
		{mode: "custom", count: 2, want: "enabled"},
		{mode: "off", want: "disabled"},
	}
	for _, test := range tests {
		t.Run(test.mode, func(t *testing.T) {
			profile := models.Profile{WorkerConfig: models.JSONMap{"frameStructureBFrameMode": test.mode, "frameStructureMaxBFrames": test.count}}
			policy, count := frameStructureBFrameIntent(profile)
			if policy != test.want {
				t.Fatalf("policy = %q, want %q", policy, test.want)
			}
			if (test.mode == "recommended" || test.mode == "custom") && count != test.count {
				t.Fatalf("count = %d, want %d", count, test.count)
			}
		})
	}
}

func TestGenericEncoderRecommendationKeepsCRFAndBitrateDistinct(t *testing.T) {
	profile := models.Profile{VideoCodec: "x265", QualityMode: "crf", QualityValue: 20, PixelFormat: "yuv420p10le", WorkerConfig: models.JSONMap{"videoEncoder": "libx265", "frameStructureBFrameMode": "custom", "frameStructureMaxBFrames": 2}}
	intent := quality.NewIntent(quality.IntentInput{SourceVideoBitrate: 4_000_000, DurationSeconds: 120, AudioBitrate: 192_000})
	recommendation := genericEncoderRecommendation(profile, intent, "libx265")
	if recommendation.EffectiveRateControl != "crf" || recommendation.BaseTargetBitrate != nil || recommendation.TargetBitrate != nil {
		t.Fatalf("generic CRF recommendation was presented as bitrate: %+v", recommendation)
	}
	if recommendation.EffectiveBFramePolicy != "enabled" || recommendation.RequestedBFrames != 2 {
		t.Fatalf("frame structure was not preserved: %+v", recommendation)
	}
	if recommendation.EstimatedOutputSizeMin == nil || recommendation.EstimatedOutputSizeMax == nil || *recommendation.EstimatedOutputSizeMin >= *recommendation.EstimatedOutputSizeMax {
		t.Fatalf("expected a low-confidence output range: %+v", recommendation)
	}
}

func TestVideoToolboxEnabledFallsBackToAutoWithoutEffectiveProbe(t *testing.T) {
	profile := models.Profile{WorkerConfig: models.JSONMap{"videoToolboxBFramePolicy": "enabled", "videoToolboxBFrames": 3}}
	command := shellJoin(videoToolboxRealtimeAndBFrameArgs(profile, capabilities.EncoderCapability{VideoToolboxBFramesVerified: true}, false))
	assertContains(t, command, "-realtime 0")
	assertNotContains(t, command, "-bf")
}

func TestLegacyVideoToolboxBFrameBooleanMigratesWithoutDisabling(t *testing.T) {
	for _, test := range []struct {
		legacy bool
		want   string
	}{{legacy: false, want: "auto"}, {legacy: true, want: "enabled"}} {
		profile := normalizeHardwareQualityPreset(models.Profile{WorkerConfig: models.JSONMap{
			"videoEncoder": "hevc_videotoolbox", "hardwareQualityPreset": "custom", "videoToolboxAllowFrameReordering": test.legacy,
		}})
		if got := workerStringValue(profile.WorkerConfig["videoToolboxBFramePolicy"]); got != test.want {
			t.Fatalf("legacy %t migrated to %q, want %q", test.legacy, got, test.want)
		}
		if _, exists := profile.WorkerConfig["videoToolboxAllowFrameReordering"]; exists {
			t.Fatal("legacy ambiguous field was retained")
		}
	}
}

func TestVideoToolboxCustomManualRatesRemainExplicitUnlessAdjustmentEnabled(t *testing.T) {
	capability := capabilities.EncoderCapability{VideoToolboxBFramesDisabled: true}
	profile := models.Profile{WorkerConfig: models.JSONMap{
		"hardwareQualityPreset": "custom", "videoToolboxBitrateMbps": 1.65,
		"videoToolboxMaxrateMbps": 2.48, "videoToolboxBufferMbps": 4.13,
		"videoToolboxBFramePolicy": "disabled",
	}}
	target, maxrate, buffer := explicitVideoToolboxRates(profile, capability)
	if target != 1.65 || maxrate != 2.48 || buffer != 4.13 {
		t.Fatalf("manual custom rates changed without opt-in: %.2f %.2f %.2f", target, maxrate, buffer)
	}
	profile.WorkerConfig["videoToolboxAutoAdjustBitrate"] = true
	target, maxrate, buffer = explicitVideoToolboxRates(profile, capability)
	if math.Abs(target-1.782) > 0.0001 || math.Abs(maxrate-2.673) > 0.0001 || math.Abs(buffer-4.455) > 0.0001 {
		t.Fatalf("custom strategy adjustment not applied before limits: %.3f %.3f %.3f", target, maxrate, buffer)
	}
}

func TestVideoToolboxProfileFollowsOutputBitDepth(t *testing.T) {
	tenBit := models.Profile{
		BitDepth: 10,
		WorkerConfig: models.JSONMap{
			"pixFmt": "p010le", "videoToolboxProfile": "main",
		},
	}
	if got := normalizedVideoToolboxProfile(tenBit); got != "main10" {
		t.Fatalf("P010 output must use VideoToolbox Main10, got %q", got)
	}

	eightBit := models.Profile{
		BitDepth: 8,
		WorkerConfig: models.JSONMap{
			"pixFmt": "nv12", "videoToolboxProfile": "main10",
		},
	}
	if got := normalizedVideoToolboxProfile(eightBit); got != "main" {
		t.Fatalf("NV12 output must use VideoToolbox Main, got %q", got)
	}
}

func TestVideoToolboxFractionalMbpsIsPreservedForFFmpeg(t *testing.T) {
	if got := formatMbps(1.773); got != "1.77M" {
		t.Fatalf("fractional VideoToolbox bitrate was rounded: %q", got)
	}
	if got := formatMbps(3); got != "3M" {
		t.Fatalf("whole VideoToolbox bitrate should remain compact: %q", got)
	}
}

func TestVideoToolboxMain10OptionalFeaturesRequireMatchingProbe(t *testing.T) {
	capability := capabilities.EncoderCapability{
		VideoToolboxBFrames:        true,
		VideoToolboxPowerEfficient: true,
		TestedModes:                map[string]bool{},
	}
	if !videoToolboxOptionalFeatureSupported(capability, "bframes", false) {
		t.Fatal("Main B-frames should use the successful Main probe")
	}
	if videoToolboxOptionalFeatureSupported(capability, "bframes", true) || videoToolboxOptionalFeatureSupported(capability, "power_efficiency", true) {
		t.Fatal("Main10 optional features must not reuse Main capability evidence")
	}
	capability.TestedModes["videoToolboxBFramesMain10"] = true
	capability.TestedModes["videoToolboxPowerEfficientMain10"] = true
	if !videoToolboxOptionalFeatureSupported(capability, "bframes", true) || !videoToolboxOptionalFeatureSupported(capability, "power_efficiency", true) {
		t.Fatal("successful Main10 probes should enable matching optional features")
	}
}

func TestQSVMainDoesNotReuseMain10AdvancedCapabilities(t *testing.T) {
	capability := capabilities.EncoderCapability{
		LowPower:           true,
		LookAhead:          true,
		ExtendedBRC:        true,
		AdaptiveI:          true,
		AdaptiveB:          true,
		QSVLAICQMain10:     true,
		QSVLowPowerMain10:  true,
		QSVFullCombination: true,
		TestedModes:        map[string]bool{"qsvLowPowerMain8": true},
	}
	profile := models.Profile{VideoCodec: "x265", BitDepth: 8, WorkerConfig: models.JSONMap{"pixFmt": "nv12", "qsvRateControl": "la_icq", "qsvLowPower": true, "qsvExtendedBRC": true, "qsvAdaptiveI": true, "qsvAdaptiveB": true}}
	args := strings.Join(qsvWorkerArgsForCapability(profile, capability), " ")
	if !strings.Contains(args, "-low_power 1") {
		t.Fatalf("expected independently probed Main low-power option: %s", args)
	}
	for _, unsupported := range []string{"-look_ahead", "-extbrc", "-adaptive_i", "-adaptive_b"} {
		if strings.Contains(args, unsupported) {
			t.Fatalf("Main profile reused a Main10-only capability %s: %s", unsupported, args)
		}
	}
}

func TestVideoToolboxBT709IsTaggedWithoutRedundantConversion(t *testing.T) {
	profile := models.Profile{VideoCodec: "x265", WorkerConfig: models.JSONMap{}}
	source := MediaStream{
		ColorSpace: "bt709", ColorTransfer: "bt709", ColorPrimaries: "bt709", ColorRange: "tv",
	}
	effective := profileWithAutomaticVideoToolboxColorForEncoder(profile, source, "hevc_videotoolbox")
	if filter := workerStringValue(effective.WorkerConfig["videoFilters"]); strings.Contains(filter, "colorspace=") {
		t.Fatalf("BT.709 source received redundant conversion: %q", filter)
	}
	assertContains(t, shellJoin(videoColorMetadataArgs(effective)), "-color_primaries bt709")
}

func TestFinalColorPolicyPreservesSourceMetadataForSoftware(t *testing.T) {
	profile := models.Profile{VideoCodec: "x265", WorkerConfig: models.JSONMap{"finalColorPolicy": "preserve"}}
	source := MediaStream{ColorSpace: "bt470bg", ColorTransfer: "bt470m", ColorPrimaries: "bt470m", ColorRange: "tv"}
	effective := profileWithFinalColorPolicy(profile, source, "libx265")

	if filter := workerStringValue(effective.WorkerConfig["videoFilters"]); strings.Contains(filter, "colorspace=") {
		t.Fatalf("preserve policy must not transform pixels: %q", filter)
	}
	colorArgs := shellJoin(videoColorMetadataArgs(effective))
	assertContains(t, colorArgs, "-colorspace bt470bg")
	assertContains(t, colorArgs, "-color_trc bt470m")
	assertContains(t, colorArgs, "-color_primaries bt470m")
	assertContains(t, colorArgs, "-color_range tv")
}

func TestFinalColorPolicyNormalizesPixelsBeforeWritingBT709Metadata(t *testing.T) {
	profile := models.Profile{VideoCodec: "x265", WorkerConfig: models.JSONMap{"finalColorPolicy": "normalize_bt709"}}
	source := MediaStream{ColorSpace: "bt470bg", ColorTransfer: "bt470m", ColorPrimaries: "bt470m", ColorRange: "tv"}
	effective := profileWithFinalColorPolicy(profile, source, "libx265")

	filter := workerStringValue(effective.WorkerConfig["videoFilters"])
	assertContains(t, filter, "colorspace=ispace=bt470bg")
	assertContains(t, filter, "space=bt709")
	assertContains(t, shellJoin(videoColorMetadataArgs(effective)), "-color_primaries bt709")
}

func TestFinalColorPolicyDoesNotNormalizeHDRWithoutToneMapping(t *testing.T) {
	profile := models.Profile{VideoCodec: "x265", WorkerConfig: models.JSONMap{"finalColorPolicy": "normalize_bt709"}}
	source := MediaStream{ColorSpace: "bt2020nc", ColorTransfer: "smpte2084", ColorPrimaries: "bt2020", ColorRange: "tv"}
	effective := profileWithFinalColorPolicy(profile, source, "libx265")

	if filter := workerStringValue(effective.WorkerConfig["videoFilters"]); strings.Contains(filter, "colorspace=") {
		t.Fatalf("HDR must not be normalized without tone mapping: %q", filter)
	}
	assertContains(t, workerStringValue(effective.WorkerConfig["colorPolicyWarning"]), "tone mapping")
	assertContains(t, shellJoin(videoColorMetadataArgs(effective)), "-color_trc smpte2084")
}

func TestFFmpegCommandBuilderAddsProfileAudioAndSubtitleCompatibility(t *testing.T) {
	plan := MediaJobPlan{
		InputPath:      "/media/raw/episode.mkv",
		OutputPath:     "/media/staging/job-3/episode.mkv",
		Overwrite:      true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{
			VideoCodec:        "x265_10bit",
			AudioCodec:        "copy",
			QualityMode:       "crf",
			QualityValue:      21,
			PreserveSubtitles: true,
			PreserveChapters:  true,
			WorkerConfig: models.JSONMap{
				"videoEncoder":          "libx265",
				"addAacStereoTrack":     true,
				"aacStereoBitrateKbps":  224,
				"aacStereoDefault":      false,
				"preserveOriginalAudio": true,
				"preferSrtSubtitles":    true,
			},
		},
		Streams: MediaStreamInventory{
			Video:    []MediaStream{{Index: 0}},
			Audio:    []MediaAudioStream{{Index: 1, Codec: "ac3", Channels: 6, ChannelLayout: "5.1", Default: true}},
			Subtitle: []MediaStream{{Index: 2, Codec: "ass"}},
		},
	}

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))

	assertContains(t, command, "-map 0")
	assertContains(t, command, "-map 0:1")
	assertContains(t, command, "-c:v libx265")
	assertContains(t, command, "-crf 21")
	assertContains(t, command, "-c:a:1 aac")
	assertContains(t, command, "-b:a:1 224k")
	assertContains(t, command, "-ac:a:1 2")
	assertContains(t, command, "-disposition:a:0 0")
	assertContains(t, command, "-disposition:a:1 default")
	assertContains(t, command, "-c:s:0 srt")
}

func TestFFmpegCommandBuilderMakesAACDefaultWhenOriginalIsSecondary(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/media/raw/movie.mkv", OutputPath: "/media/staging/movie.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{
			VideoCodec: "copy", AudioCodec: "copy",
			WorkerConfig: models.JSONMap{
				"addAacStereoTrack":     true,
				"aacStereoDefault":      false,
				"preserveOriginalAudio": true,
			},
		},
		Streams: MediaStreamInventory{Audio: []MediaAudioStream{{Index: 1, Codec: "ac3", Channels: 6, Default: true}}},
	}

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertContains(t, command, "-disposition:a:0 0")
	assertContains(t, command, "-disposition:a:1 default")
}

func TestFFmpegCommandBuilderUsesDefaultAACCompatibilityBitrate(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/media/raw/movie.mkv", OutputPath: "/media/staging/movie.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile:        models.Profile{VideoCodec: "copy", AudioCodec: "copy", WorkerConfig: models.JSONMap{"addAacStereoTrack": true}},
		Streams:        MediaStreamInventory{Audio: []MediaAudioStream{{Index: 1, Codec: "ac3", Channels: 6}}},
	}

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertContains(t, command, "-b:a:1 192k")
}

func TestFFmpegCommandBuilderCopiesBitmapSubtitlesWhenSRTIsPreferred(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/media/raw/movie.mkv", OutputPath: "/media/staging/movie.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{
			VideoCodec: "x265_10bit", AudioCodec: "copy", PreserveSubtitles: true,
			WorkerConfig: models.JSONMap{"preferSrtSubtitles": true},
		},
		Streams: MediaStreamInventory{Subtitle: []MediaStream{
			{Index: 4, Codec: "dvd_subtitle"},
			{Index: 5, Codec: "hdmv_pgs_subtitle"},
		}},
	}

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertContains(t, command, "-c:s copy")
}

func TestFFmpegCommandBuilderConvertsOnlyTextSubtitlesToSRT(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/media/raw/movie.mkv", OutputPath: "/media/staging/movie.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{
			VideoCodec: "x265_10bit", AudioCodec: "copy", PreserveSubtitles: true,
			WorkerConfig: models.JSONMap{"preferSrtSubtitles": true},
		},
		Streams: MediaStreamInventory{Subtitle: []MediaStream{
			{Index: 4, Codec: "dvd_subtitle"},
			{Index: 5, Codec: "ass"},
		}},
	}

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertNotContains(t, command, "-c:s:0 srt")
	assertContains(t, command, "-c:s:1 srt")
}

func TestFFmpegCommandBuilderConvertsOnlyTextSubtitlesToASS(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/media/raw/movie.mkv", OutputPath: "/media/staging/movie.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{
			VideoCodec: "copy", AudioCodec: "copy", PreserveSubtitles: true,
			WorkerConfig: models.JSONMap{"subtitleOutputFormat": "ass"},
		},
		Streams: MediaStreamInventory{Subtitle: []MediaStream{
			{Index: 4, Codec: "hdmv_pgs_subtitle"},
			{Index: 5, Codec: "subrip"},
		}},
	}

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertNotContains(t, command, "-c:s:0 ass")
	assertContains(t, command, "-c:s:1 ass")
}

func TestFFmpegCommandBuilderPreservesSubtitleFormatWhenRequested(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/media/raw/movie.mkv", OutputPath: "/media/staging/movie.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{
			VideoCodec: "copy", AudioCodec: "copy", PreserveSubtitles: true,
			WorkerConfig: models.JSONMap{"subtitleOutputFormat": "source", "preferSrtSubtitles": true},
		},
		Streams: MediaStreamInventory{Subtitle: []MediaStream{{Index: 4, Codec: "ass"}}},
	}

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertContains(t, command, "-c:s copy")
}

func TestCommonFrameStructurePolicyMapsRequestedModes(t *testing.T) {
	tests := []struct {
		name       string
		encoder    string
		config     models.JSONMap
		contains   []string
		notContain []string
	}{
		{name: "x265 auto leaves encoder defaults", encoder: "libx265", config: models.JSONMap{"frameStructureGopMode": "auto", "frameStructureBFrameMode": "auto"}, notContain: []string{"-g", "-bf"}},
		{name: "x265 recommended emits common values", encoder: "libx265", config: models.JSONMap{"frameStructureGopMode": "recommended", "frameStructureGopFrames": 120, "frameStructureBFrameMode": "recommended", "frameStructureMaxBFrames": 3}, contains: []string{"-g 120", "-bf 3"}},
		{name: "qsv custom emits common values", encoder: "hevc_qsv", config: models.JSONMap{"frameStructureGopMode": "custom", "frameStructureGopFrames": 90, "frameStructureBFrameMode": "custom", "frameStructureMaxBFrames": 2}, contains: []string{"-g 90", "-bf 2"}},
		{name: "qsv off explicitly disables b frames", encoder: "hevc_qsv", config: models.JSONMap{"frameStructureBFrameMode": "off", "frameStructureMaxBFrames": 3}, contains: []string{"-bf 0"}},
		{name: "videotoolbox custom GOP reaches encoder command", encoder: "hevc_videotoolbox", config: models.JSONMap{"frameStructureGopMode": "custom", "frameStructureGopFrames": 96, "frameStructureBFrameMode": "auto"}, contains: []string{"-g 96"}, notContain: []string{"-bf"}},
		{name: "global off omits common frame structure", encoder: "libx265", config: models.JSONMap{"frameStructureMode": "off", "frameStructureGopMode": "custom", "frameStructureGopFrames": 96, "frameStructureBFrameMode": "off"}, notContain: []string{"-g", "-bf"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile := models.Profile{VideoCodec: "x265", QualityMode: "crf", QualityValue: 20, WorkerConfig: test.config}
			args := shellJoin(videoCodecArgsForResolvedEncoder(profile, nil, test.encoder))
			for _, expected := range test.contains {
				assertContains(t, args, expected)
			}
			for _, unexpected := range test.notContain {
				assertNotContains(t, args, unexpected)
			}
		})
	}
}

func TestCommonFrameStructurePolicyDominatesConflictingX265Params(t *testing.T) {
	profile := models.Profile{VideoCodec: "x265", QualityMode: "crf", QualityValue: 20, WorkerConfig: models.JSONMap{
		"videoEncoder": "libx265", "frameStructureGopMode": "recommended", "frameStructureGopFrames": 120,
		"frameStructureBFrameMode": "off", "x265Params": "keyint=300:min-keyint=30:scenecut=40:bframes=8:b-adapt=2:b-pyramid=1:aq-mode=3",
	}}
	command := shellJoin(FFmpegCommandBuilder{}.Build(MediaJobPlan{InputPath: "/media/raw/movie.mkv", OutputPath: "/media/out/movie.mkv", Overwrite: true, ProcessingMode: ProcessingModeFullEncode, Profile: profile, Streams: MediaStreamInventory{Video: []MediaStream{{Index: 0}}}}))
	assertContains(t, command, "-g 120")
	assertContains(t, command, "-bf 0")
	assertContains(t, command, "-x265-params aq-mode=3")
	for _, conflict := range []string{"keyint=300", "min-keyint=30", "scenecut=40", "bframes=8", "b-adapt=2", "b-pyramid=1"} {
		assertNotContains(t, command, conflict)
	}
}

func TestAssetFrameStructureOverrideTakesPriorityOverProfile(t *testing.T) {
	base := models.Profile{VideoCodec: "x265", QualityMode: "crf", QualityValue: 20, BitDepth: 10, WorkerConfig: models.JSONMap{
		"videoEncoder": "hevc_qsv", "pixFmt": "p010le", "frameStructureGopMode": "recommended", "frameStructureGopFrames": 120,
		"frameStructureBFrameMode": "recommended", "frameStructureMaxBFrames": 3, "qsvAdaptiveB": true,
	}}
	tests := []struct {
		name       string
		override   AssetConversionOverrideState
		contains   []string
		notContain []string
	}{
		{name: "explicit auto restores encoder defaults", override: AssetConversionOverrideState{FrameStructureGOPMode: "auto", FrameStructureBFrameMode: "auto"}, notContain: []string{"-g", "-bf"}},
		{name: "custom replaces profile values", override: AssetConversionOverrideState{FrameStructureGOPMode: "custom", FrameStructureGOPFrames: 90, FrameStructureBFrameMode: "custom", FrameStructureMaxBFrames: 2}, contains: []string{"-g 90", "-bf 2"}, notContain: []string{"-g 120", "-bf 3"}},
		{name: "qsv bf0 normalizes adaptive B off", override: AssetConversionOverrideState{FrameStructureBFrameMode: "off"}, contains: []string{"-bf 0"}, notContain: []string{"-adaptive_b"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile := applyAssetConversionOverrideToProfile(base, test.override)
			command := strings.Join(append(videoCodecArgsForResolvedEncoder(profile, nil, "hevc_qsv"), qsvWorkerArgsForCapability(profile, capabilities.EncoderCapability{QSVAdaptiveBMain10: true})...), " ")
			for _, value := range test.contains {
				assertContains(t, command, value)
			}
			for _, value := range test.notContain {
				assertNotContains(t, command, value)
			}
		})
	}
}

func TestAssetPStrategyDefaultExplicitlyOverridesProfile(t *testing.T) {
	defaultStrategy := 0
	base := models.Profile{WorkerConfig: models.JSONMap{"videoEncoder": "hevc_qsv", "hardwareQualityPreset": "custom", "qsvPStrategy": 2}}
	profile := applyAssetConversionOverrideToProfile(base, AssetConversionOverrideState{QSVPStrategy: &defaultStrategy})
	if actual, ok := profile.WorkerConfig["qsvPStrategy"].(int); !ok || actual != 0 {
		t.Fatalf("expected explicit asset p_strategy default 0, got %#v", profile.WorkerConfig["qsvPStrategy"])
	}
}

func TestFFmpegCommandBuilderAllowsAssetToMakeAACCompatibilityDefault(t *testing.T) {
	makeDefault := true
	plan := MediaJobPlan{
		InputPath: "/media/raw/movie.mkv", OutputPath: "/media/staging/movie.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{
			VideoCodec: "copy", AudioCodec: "copy",
			WorkerConfig: models.JSONMap{"addAacStereoTrack": true, "aacStereoDefault": false},
		},
		Override: AssetConversionOverrideState{AACStereoDefault: &makeDefault},
		Streams:  MediaStreamInventory{Audio: []MediaAudioStream{{Index: 1, Codec: "ac3", Channels: 6}}},
	}

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertContains(t, command, "-c:a:1 aac")
	assertContains(t, command, "-disposition:a:1 default")
}

func TestFFmpegCommandBuilderMapsConfiguredAACCompatibilitySource(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/media/raw/movie.mkv", OutputPath: "/media/staging/movie.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{
			VideoCodec: "copy", AudioCodec: "copy",
			WorkerConfig: models.JSONMap{"addAacStereoTrack": true, "aacStereoSourceStreamIndex": 4},
		},
		Streams: MediaStreamInventory{Audio: []MediaAudioStream{
			{Index: 1, Codec: "ac3", Channels: 6, Default: true},
			{Index: 4, Codec: "dts", Channels: 6},
		}},
	}

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertContains(t, command, "-map 0:4")
	assertContains(t, command, "-c:a:2 aac")
}

func TestFFmpegCommandBuilderAllowsAssetToDisableAACCompatibility(t *testing.T) {
	disabled := false
	plan := MediaJobPlan{
		InputPath: "/media/raw/movie.mkv", OutputPath: "/media/staging/movie.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{
			VideoCodec: "copy", AudioCodec: "copy",
			WorkerConfig: models.JSONMap{"addAacStereoTrack": true, "aacStereoDefault": false},
		},
		Override: AssetConversionOverrideState{AddAACStereoTrack: &disabled},
		Streams:  MediaStreamInventory{Audio: []MediaAudioStream{{Index: 1, Codec: "ac3", Channels: 6}}},
	}

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertNotContains(t, command, "AAC Stereo (MVForge)")
}

func TestFFmpegCommandBuilderDoesNotDuplicateSingleAACStereoTrack(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/media/raw/episode.mkv", OutputPath: "/media/staging/episode.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{
			VideoCodec: "x265_10bit", AudioCodec: "copy", QualityMode: "crf", QualityValue: 21,
			WorkerConfig: models.JSONMap{"videoEncoder": "libx265", "addAacStereoDefault": true, "preserveOriginalAudio": true},
		},
		Streams: MediaStreamInventory{Audio: []MediaAudioStream{{Index: 2, Codec: "aac", Channels: 2, ChannelLayout: "stereo", Language: "jpn"}}},
	}

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	if count := strings.Count(command, "-map 0:2"); count != 1 {
		t.Fatalf("existing AAC stream should be mapped exactly once, got %d: %s", count, command)
	}
	assertNotContains(t, command, "-c:a:1 aac")
	assertNotContains(t, command, "AAC Stereo (MVForge)")
}

func TestFFmpegCommandBuilderDoesNotDuplicateAACStereoAmongMultipleTracks(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/media/raw/movie.mkv", OutputPath: "/media/staging/movie.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{
			VideoCodec: "copy", AudioCodec: "copy",
			WorkerConfig: models.JSONMap{"addAacStereoTrack": true, "aacStereoDefault": false},
		},
		Streams: MediaStreamInventory{Audio: []MediaAudioStream{
			{Index: 1, Codec: "truehd", Channels: 8, Language: "eng"},
			{Index: 2, Codec: "aac", Channels: 2, Language: "spa"},
			{Index: 3, Codec: "aac", Channels: 1, Language: "jpn"},
		}},
	}

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertNotContains(t, command, "AAC Stereo (MVForge)")
	assertNotContains(t, command, "-map 0:1 -c copy")
}

func TestFFmpegCommandConsumesResolvedTrackPlan(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/raw/Movie.mkv", OutputPath: "/staging/Movie.mkv", Overwrite: true,
		Profile:        models.Profile{VideoCodec: "copy", AudioCodec: "copy", PreserveChapters: false, WorkerConfig: models.JSONMap{"videoEncoder": "copy"}},
		ProcessingMode: ProcessingModeAudioOnly,
		Streams: MediaStreamInventory{
			Video:      []MediaStream{{Index: 0}},
			Audio:      []MediaAudioStream{{Index: 1}, {Index: 2}},
			Subtitle:   []MediaStream{{Index: 3, Codec: "subrip"}, {Index: 4, Codec: "ass"}, {Index: 5, Codec: "ass"}},
			Attachment: []MediaStream{{Index: 6, Codec: "ttf"}}, ChapterCount: 2,
		},
		ResolvedTracks: &ResolvedTrackPlan{
			VideoStreams: []ResolvedTrackStream{{StreamIndex: 0}},
			AudioStreams: []ResolvedTrackStream{{StreamIndex: 1}, {StreamIndex: 2}},
			SubtitleStreams: []ResolvedSubtitleTrack{
				{StreamIndex: 3, Action: SubtitleDispositionKeep},
				{StreamIndex: 4, Action: SubtitleDispositionExtract},
				{StreamIndex: 5, Action: SubtitleDispositionKeepAndExtract},
			},
			AttachmentsKept: true, AttachmentStreams: []ResolvedTrackStream{{StreamIndex: 6}},
			ChaptersKept: true,
		},
	}
	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	for _, mapping := range []string{"-map 0:0", "-map 0:1", "-map 0:2", "-map 0:3", "-map 0:5", "-map 0:6"} {
		assertContains(t, command, mapping)
	}
	assertNotContains(t, command, "-map 0:4")
	assertContains(t, command, "-map_chapters 0")
}

func TestFFmpegCommandResolvedTrackPlanRemovesSubtitlesAttachmentsAndChapters(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/raw/Movie.mkv", OutputPath: "/staging/Movie.mkv",
		Profile:        models.Profile{VideoCodec: "copy", AudioCodec: "copy", PreserveSubtitles: true, PreserveChapters: true},
		ProcessingMode: ProcessingModeAudioOnly,
		Streams:        MediaStreamInventory{Video: []MediaStream{{Index: 0}}, Subtitle: []MediaStream{{Index: 2, Codec: "ass"}}, Attachment: []MediaStream{{Index: 3}}, ChapterCount: 1},
		ResolvedTracks: &ResolvedTrackPlan{
			VideoStreams:    []ResolvedTrackStream{{StreamIndex: 0}},
			SubtitleStreams: []ResolvedSubtitleTrack{{StreamIndex: 2, Action: SubtitleDispositionRemove}},
			AttachmentsKept: false, ChaptersKept: false,
		},
	}
	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertContains(t, command, "-map 0:0")
	assertNotContains(t, command, "-map 0:2")
	assertNotContains(t, command, "-map 0:3")
	assertContains(t, command, "-map_chapters -1")
}

func TestResolvedStreamPlanAssignsDerivedAudioAfterOriginalAudio(t *testing.T) {
	plan := MediaJobPlan{
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{VideoCodec: "x265", PreserveSubtitles: true, WorkerConfig: models.JSONMap{
			"videoEncoder": "hevc_videotoolbox", "aacStereoSourceStreamIndex": 1,
		}},
		Streams: MediaStreamInventory{
			Video:    []MediaStream{{Index: 0}},
			Audio:    []MediaAudioStream{{Index: 1, Language: "spa"}, {Index: 4, Language: "jpn"}},
			Subtitle: []MediaStream{{Index: 5}, {Index: 6}},
		},
	}
	resolved := resolveStreamPlan(plan, plan.Streams.Audio, true)
	if len(resolved.Audio) != 3 {
		t.Fatalf("expected two originals plus derived AAC: %#v", resolved.Audio)
	}
	derived := resolved.Audio[2]
	if derived.OutputTypeIndex != 2 || derived.InputIndex != 1 || derived.DuplicateOf == nil || *derived.DuplicateOf != 1 || derived.Codec != "aac" {
		t.Fatalf("incorrect derived AAC mapping: %#v", derived)
	}
	if resolved.Subtitles[0].OutputTypeIndex != 0 || resolved.Subtitles[1].OutputTypeIndex != 1 {
		t.Fatalf("subtitle indexes changed audio-relative indexing: %#v", resolved.Subtitles)
	}
}

func TestFFmpegCommandBuilderTargetsMetadataByResolvedOutputIndex(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/input.mkv", OutputPath: "/output.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{VideoCodec: "x265", AudioCodec: "copy", PreserveSubtitles: true, WorkerConfig: models.JSONMap{
			"videoEncoder": "libx265", "addAacStereoTrack": true, "aacStereoSourceStreamIndex": 4,
		}},
		Streams: MediaStreamInventory{
			Video:    []MediaStream{{Index: 0}},
			Audio:    []MediaAudioStream{{Index: 1, Language: "eng"}, {Index: 4, Language: "spa", Title: "Spanish Surround"}},
			Subtitle: []MediaStream{{Index: 5, Language: "spa"}},
		},
		Override: AssetConversionOverrideState{KeepAudioStreams: []int{4}, AudioMetadata: map[int]StreamMetadataOverride{
			4: {Title: "Original Spanish Surround", Language: "spa"},
		}},
	}

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertContains(t, command, "-map 0:4")
	assertContains(t, command, `-metadata:s:a:0 "title=Original Spanish Surround"`)
	assertContains(t, command, "-metadata:s:a:0 language=spa")
	assertContains(t, command, "-c:a:1 aac")
	assertNotContains(t, command, "-metadata:s:a:4")
}

func TestFFmpegCommandBuilderEmitsCodecsOnlyForMappedTypes(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/input.mkv", OutputPath: "/output.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile:        models.Profile{VideoCodec: "x265", QualityMode: "crf", QualityValue: 20, WorkerConfig: models.JSONMap{"videoEncoder": "libx265"}},
		Streams:        MediaStreamInventory{Video: []MediaStream{{Index: 0}}, Attachment: []MediaStream{{Index: 3}}},
	}
	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	if strings.Count(command, "-c:v") != 1 {
		t.Fatalf("video codec must be emitted exactly once: %s", command)
	}
	assertContains(t, command, "-map 0:3")
	assertContains(t, command, "-c:t copy")
	assertNotContains(t, command, "-c:a copy")
	assertNotContains(t, command, "-c:s copy")
	assertNotContains(t, command, "-c:d copy")
}

func TestFFmpegCommandBuilderClearsInheritedMatroskaStreamStatistics(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/input.mkv", OutputPath: "/output.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile:        models.Profile{VideoCodec: "x265", AudioCodec: "copy", WorkerConfig: models.JSONMap{"videoEncoder": "hevc_videotoolbox"}},
		Streams: MediaStreamInventory{
			Video: []MediaStream{{Index: 0}},
			Audio: []MediaAudioStream{{Index: 1}},
		},
	}
	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	for _, expected := range []string{
		"-metadata:s:v:0 BPS-eng=",
		"-metadata:s:v:0 NUMBER_OF_BYTES-eng=",
		"-metadata:s:v:0 _STATISTICS_WRITING_APP-eng=",
		"-metadata:s:a:0 BPS-eng=",
	} {
		assertContains(t, command, expected)
	}
	assertNotContains(t, command, "-metadata:s:s:0 BPS-eng=")
}

func TestCropAspectPolicyPreservesSourceSARByDefault(t *testing.T) {
	filters := applyCropAspectPolicy("crop=700:446:10:65", models.Profile{}, &MediaStream{Width: 720, Height: 576, SampleAspectRatio: "16:15"})
	assertContains(t, filters, "crop=700:446:10:65,setsar=16/15")
	assertNotContains(t, filters, "79/93")
	assertNotContains(t, filters, "setdar=")
}

func TestCropAspectPolicyCanExplicitlyPreserveDAR(t *testing.T) {
	filters := applyCropAspectPolicy("crop=672:438:24:20", models.Profile{WorkerConfig: models.JSONMap{"cropAspectPolicy": "preserve_dar"}}, &MediaStream{Width: 720, Height: 480, SampleAspectRatio: "8:9"})
	assertContains(t, filters, "setsar=73/84")
	assertNotContains(t, filters, "setdar=")
}

func TestApplySelectedEncoderUsesExecutionPlanWithoutMutatingSnapshot(t *testing.T) {
	original := models.Profile{WorkerConfig: models.JSONMap{"videoEncoder": "hevc_qsv", "videoPreset": "medium"}}
	effective := applySelectedEncoder(original, "libx265")

	if got := workerStringValue(effective.WorkerConfig["videoEncoder"]); got != "libx265" {
		t.Fatalf("effective encoder=%q", got)
	}
	if got := workerStringValue(original.WorkerConfig["videoEncoder"]); got != "hevc_qsv" {
		t.Fatalf("profile snapshot was mutated: %q", got)
	}
}

func assertContains(t *testing.T, value string, expected string) {
	t.Helper()
	if !strings.Contains(value, expected) {
		t.Fatalf("expected command to contain %q\ncommand: %s", expected, value)
	}
}

func assertNotContains(t *testing.T, value string, unexpected string) {
	t.Helper()
	if strings.Contains(value, unexpected) {
		t.Fatalf("expected command not to contain %q\ncommand: %s", unexpected, value)
	}
}

func TestFullEncodeOmitsSecondaryMJPEGAuxiliaryVideo(t *testing.T) {
	plan := MediaJobPlan{
		InputPath:      "/media/raw/movie.mkv",
		OutputPath:     "/media/staging/movie.mkv",
		Overwrite:      true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{
			VideoCodec: "x265",
			AudioCodec: "copy",
			WorkerConfig: models.JSONMap{
				"videoEncoder": "hevc_qsv",
			},
		},
		Streams: MediaStreamInventory{
			Video: []MediaStream{
				{
					Index:       0,
					Codec:       "h264",
					Width:       1920,
					Height:      1080,
					FrameRate:   "24000/1001",
					PixelFormat: "yuv420p",
				},
				{
					Index:       2,
					Codec:       "mjpeg",
					Width:       640,
					Height:      360,
					FrameRate:   "0/0",
					PixelFormat: "yuvj420p",
				},
			},
			Audio: []MediaAudioStream{
				{Index: 1, Codec: "aac"},
			},
		},
	}

	command := shellJoin(
		FFmpegCommandBuilder{}.Build(plan),
	)

	assertContains(t, command, "-map 0:0")
	assertContains(t, command, "-map 0:1")

	assertNotContains(t, command, "-map 0:2")
}

func TestFullEncodeKeepsPrimaryMJPEGVideo(t *testing.T) {
	plan := MediaJobPlan{
		InputPath:      "/media/raw/mjpeg-source.mkv",
		OutputPath:     "/media/staging/mjpeg-source.mkv",
		Overwrite:      true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{
			VideoCodec: "x265",
			AudioCodec: "copy",
			WorkerConfig: models.JSONMap{
				"videoEncoder": "libx265",
			},
		},
		Streams: MediaStreamInventory{
			Video: []MediaStream{
				{
					Index:       0,
					Codec:       "mjpeg",
					Width:       1280,
					Height:      720,
					FrameRate:   "30000/1001",
					PixelFormat: "yuvj420p",
				},
			},
		},
	}

	command := shellJoin(
		FFmpegCommandBuilder{}.Build(plan),
	)

	assertContains(t, command, "-map 0:0")
}
