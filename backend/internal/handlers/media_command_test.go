package handlers

import (
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/capabilities"
	"github.com/anuelvs/mvforge/backend/internal/models"
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
				{Index: 1, Codec: "ac3", Channels: 1, ChannelLayout: "mono", Default: true},
			},
		},
	}

	args := FFmpegCommandBuilder{}.Build(plan)
	command := shellJoin(args)

	assertContains(t, command, "-map 0")
	assertContains(t, command, "-map 0:1")
	assertContains(t, command, "-c copy")
	assertContains(t, command, "-c:v copy")
	assertContains(t, command, "-filter:a:1")
	assertContains(t, command, "-c:a:1 aac")
	assertContains(t, command, "-metadata:s:a:0 \"title=Original Mono\"")
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
		UseHardwareIfAvailable:  &enabled,
		VideoEncoder:            "hevc_qsv",
		PreferredEncoder:        "hardware",
		GlobalQuality:           27,
		QSVRateControl:          "la_icq",
		QSVLookAheadDepth:       55,
		QSVExtendedBRC:          &enabled,
		QSVAdaptiveI:            &enabled,
		QSVAdaptiveB:            &disabled,
		VideoToolboxBitrateMbps: 6,
		VideoToolboxMaxrateMbps: 8,
		VideoToolboxBufferMbps:  12,
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
		profile.WorkerConfig["videoToolboxBitrateMbps"] != 6 ||
		profile.WorkerConfig["videoToolboxMaxrateMbps"] != 8 ||
		profile.WorkerConfig["videoToolboxBufferMbps"] != 12 {
		t.Fatalf("hardware overrides were not applied: %#v", profile.WorkerConfig)
	}
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
	profile := models.Profile{WorkerConfig: models.JSONMap{
		"qsvRateControl":    "la_icq",
		"qsvExtendedBRC":    true,
		"qsvLookAheadDepth": 40,
		"qsvAdaptiveI":      true,
		"qsvAdaptiveB":      true,
	}}
	args := qsvWorkerArgsForCapability(profile, capabilities.EncoderCapability{
		ICQ: true, LookAhead: true, ExtendedBRC: false, AdaptiveI: true, AdaptiveB: false,
	})
	command := strings.Join(args, " ")
	assertContains(t, command, "-look_ahead 1")
	assertContains(t, command, "-adaptive_i 1")
	assertNotContains(t, command, "-extbrc")
	assertNotContains(t, command, "-adaptive_b")
	assertNotContains(t, command, "-look_ahead_depth")
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

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertContains(t, command, "-vf bwdif=mode=send_frame:parity=auto:deint=all")
}

func TestFFmpegCommandBuilderDoesNotAutomaticallyFilterTelecine(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/media/raw/dvd.mkv", OutputPath: "/media/staging/dvd.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{
			VideoCodec: "x265_10bit", AudioCodec: "copy",
			WorkerConfig: models.JSONMap{"deinterlaceMode": "auto"},
		},
		Interlace: InterlaceAnalysis{Status: "telecine_suspected"},
	}

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertNotContains(t, command, "bwdif")
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
	assertNotContains(t, command, "-c:s")
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
	assertContains(t, command, "-c copy")
}

func TestSanitizeConversionOverrideKeepsBitmapSubtitleWhenOCRWouldBeRequired(t *testing.T) {
	override, warnings := sanitizeConversionOverride(
		AssetConversionOverrideState{SubtitleTransforms: []SubtitleTransform{{
			StreamIndex: 4, Format: "srt", RemoveEmbedded: true, Language: "spa",
		}}},
		MediaStreamInventory{Subtitle: []MediaStream{{Index: 4, Codec: "hdmv_pgs_subtitle"}}},
	)

	if len(override.SubtitleTransforms) != 0 {
		t.Fatalf("expected unsupported bitmap transformation to be omitted: %#v", override.SubtitleTransforms)
	}
	if len(warnings) != 1 || !strings.Contains(strings.ToLower(warnings[0]), "ocr") {
		t.Fatalf("expected OCR warning, got %#v", warnings)
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
	assertContains(t, command, "-disposition:a:0 default")
	assertContains(t, command, "-disposition:a:1 0")
	assertContains(t, command, "-c:s:0 srt")
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
	assertNotContains(t, command, "-c:s")
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
	assertNotContains(t, command, "-c:s")
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
	assertContains(t, command, "-map 0")
	assertNotContains(t, command, "-map 0:2")
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
