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
			Audio: []MediaAudioStream{
				{Index: 1, Codec: "ac3", Channels: 1, ChannelLayout: "mono", Default: true},
			},
		},
	}

	args := FFmpegCommandBuilder{}.Build(plan)
	command := shellJoin(args)

	assertContains(t, command, "-map 0")
	assertContains(t, command, "-map 0:a:0")
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

func assertContains(t *testing.T, value string, expected string) {
	t.Helper()
	if !strings.Contains(value, expected) {
		t.Fatalf("expected command to contain %q\ncommand: %s", expected, value)
	}
}
