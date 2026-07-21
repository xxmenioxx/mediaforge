package handlers

import (
	"strings"
	"testing"

	"github.com/anuelvs/mediaforge/backend/internal/models"
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
	assertContains(t, command, "-metadata:s:a:1 \"title=Stereo Enhanced (MediaForge)\"")
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

func TestFFmpegCommandBuilderUsesVideoToolboxBitrateAndMain10(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/media/raw/movie.mkv", OutputPath: "/media/staging/movie.mkv", Overwrite: true,
		ProcessingMode: ProcessingModeFullEncode,
		Profile: models.Profile{
			VideoCodec: "x265_10bit", BitDepth: 10, AudioCodec: "copy", QualityMode: "crf", QualityValue: 20,
			WorkerConfig: models.JSONMap{
				"videoEncoder":            "hevc_videotoolbox",
				"useHardwareIfAvailable":  false,
				"videoToolboxBitrateMbps": 6,
				"videoToolboxMaxrateMbps": 8,
				"videoToolboxBufferMbps":  12,
				"videoPreset":             "medium",
				"pixFmt":                  "yuv420p10le",
			},
		},
	}

	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
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
	assertNotContains(t, command, "AAC Stereo (MediaForge)")
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
	assertNotContains(t, command, "AAC Stereo (MediaForge)")
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
	assertNotContains(t, command, "AAC Stereo (MediaForge)")
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
