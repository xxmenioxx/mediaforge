package handlers

import (
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func TestRecommendHEVCLevelUsesPictureRateAndMainTierBitrate(t *testing.T) {
	tests := []struct {
		name    string
		width   int
		height  int
		fps     float64
		bitrate int64
		level   string
	}{
		{name: "your name 1080p24", width: 1920, height: 1080, fps: 24, bitrate: 6_418_000, level: "4.0"},
		{name: "1080p60", width: 1920, height: 1080, fps: 60, bitrate: 12_000_000, level: "4.1"},
		{name: "uhd24", width: 3840, height: 2160, fps: 24, bitrate: 20_000_000, level: "5.0"},
		{name: "uhd60", width: 3840, height: 2160, fps: 60, bitrate: 35_000_000, level: "5.1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := recommendHEVCLevel(test.width, test.height, test.fps, test.bitrate)
			if got.RecommendedLevel != test.level || got.Tier != "main" {
				t.Fatalf("recommendation=%#v want level=%s main tier", got, test.level)
			}
		})
	}
}

func TestRecommendHEVCLevelKeepsGeometryRecommendationWhenBitrateUnknown(t *testing.T) {
	got := recommendHEVCLevel(1920, 1080, 24, 0)
	if got.RecommendedLevel != "4.0" || got.Confidence != "medium" || len(got.Warnings) == 0 {
		t.Fatalf("unexpected recommendation without bitrate: %#v", got)
	}
}

func TestSnapshotHEVCLevelRecommendationRecordsObservedLevel(t *testing.T) {
	scan := models.ScanResult{
		Width: 1920, Height: 1080, Bitrate: 35_000_000,
		VideoStreams: models.JSONList{map[string]any{"avgFrameRate": "24/1", "bitrate": 35_000_000, "level": 150}},
	}
	got := buildHEVCLevelRecommendation(scan)
	if got.RecommendedLevel != "4.0" || got.SourceLevel != "5.0" || got.SourceLevelIDC != 150 {
		t.Fatalf("snapshot recommendation=%#v", got)
	}
}

func TestResolveHEVCLevelAndEncoderArguments(t *testing.T) {
	streams := MediaStreamInventory{Video: []MediaStream{{Width: 1920, Height: 1080, FrameRate: "24/1", Bitrate: 6_418_000}}}

	qsv := resolveHEVCLevel(models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{"videoEncoder": "hevc_qsv", "hevcLevelMode": "recommended"}}, streams)
	qsvArgs := strings.Join(hevcLevelArgs(qsv, "hevc_qsv"), " ")
	if qsv.WorkerConfig["hevcLevelEffective"] != "4.0" || qsvArgs != "-level:v 40 -tier main" {
		t.Fatalf("QSV effective=%#v args=%s", qsv.WorkerConfig, qsvArgs)
	}

	x265 := resolveHEVCLevel(models.Profile{VideoCodec: "x265", WorkerConfig: models.JSONMap{"videoEncoder": "libx265", "hevcLevelMode": "custom", "hevcLevel": "4.1", "x265Params": "aq-mode=3:level-idc=5.0:high-tier=1"}}, streams)
	params := effectiveX265Params(x265)
	if params != "aq-mode=3:level-idc=4.1:high-tier=0" {
		t.Fatalf("x265 params=%s", params)
	}
}

func TestHEVCLevelSemanticAdaptersKeepRepresentationsSeparate(t *testing.T) {
	tests := []struct {
		semantic string
		qsv      string
		ffprobe  int
	}{
		{semantic: "4.0", qsv: "40", ffprobe: 120},
		{semantic: "4.1", qsv: "41", ffprobe: 123},
		{semantic: "5.0", qsv: "50", ffprobe: 150},
		{semantic: "5.1", qsv: "51", ffprobe: 153},
	}
	for _, test := range tests {
		if got := hevcQSVLevelValue(test.semantic); got != test.qsv {
			t.Errorf("semantic %s mapped to QSV %s, want %s", test.semantic, got, test.qsv)
		}
		if got := hevcLevelFromIDC(test.ffprobe); got != test.semantic {
			t.Errorf("FFprobe %d normalized to %s, want %s", test.ffprobe, got, test.semantic)
		}
	}
}

func TestHEVCLevelAutoRecalculatesWhileRecommendedKeepsStoredValue(t *testing.T) {
	streams := MediaStreamInventory{Video: []MediaStream{{Width: 1920, Height: 1080, FrameRate: "60/1", Bitrate: 40_000_000}}}
	auto := resolveHEVCLevel(models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{"videoEncoder": "hevc_qsv", "hevcLevelMode": "auto", "hevcLevel": "4.0"}}, streams)
	recommended := resolveHEVCLevel(models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{"videoEncoder": "hevc_qsv", "hevcLevelMode": "recommended", "hevcLevel": "4.0"}}, streams)
	if auto.WorkerConfig["hevcLevelEffective"] != "4.1" {
		t.Fatalf("Auto did not recalculate for 1080p60: %#v", auto.WorkerConfig)
	}
	if recommended.WorkerConfig["hevcLevelEffective"] != "4.0" {
		t.Fatalf("Recommended did not keep the stored snapshot value: %#v", recommended.WorkerConfig)
	}
}

func TestAssetHEVCLevelOverrideWinsOverProfileAuto(t *testing.T) {
	profile := applyAssetConversionOverrideToProfile(
		models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{"videoEncoder": "hevc_qsv", "hevcLevelMode": "auto"}},
		AssetConversionOverrideState{HEVCLevelMode: "custom", HEVCLevel: "4.1"},
	)
	resolved := resolveHEVCLevel(profile, MediaStreamInventory{Video: []MediaStream{{Width: 1920, Height: 1080, FrameRate: "24/1"}}})
	if resolved.WorkerConfig["hevcLevelMode"] != "custom" || resolved.WorkerConfig["hevcLevelEffective"] != "4.1" {
		t.Fatalf("asset Custom Level did not win: %#v", resolved.WorkerConfig)
	}
}

func TestCustomHEVCLevelResolvesWithoutProbeForLabCommandDisplay(t *testing.T) {
	profile := resolveHEVCLevel(models.Profile{
		VideoCodec: "hevc",
		WorkerConfig: models.JSONMap{
			"videoEncoder": "hevc_qsv", "preferredEncoder": "hardware", "useHardwareIfAvailable": true,
			"hevcLevelMode": "custom", "hevcLevel": "4.0",
		},
	}, MediaStreamInventory{})
	command := shellJoin(videoCodecArgsForResolvedEncoder(profile, nil, "hevc_qsv"))
	if !strings.Contains(command, "-level:v 40 -tier main") {
		t.Fatalf("LAB Custom Level missing from displayed command: %s", command)
	}
}

func TestHEVCLevelAutoEmitsMinimumLevelForAsset(t *testing.T) {
	profile := models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{"videoEncoder": "hevc_qsv", "hevcLevelMode": "auto"}}
	resolved := resolveHEVCLevel(profile, MediaStreamInventory{Video: []MediaStream{{Width: 1920, Height: 1080, FrameRate: "24/1"}}})
	if args := strings.Join(hevcLevelArgs(resolved, "hevc_qsv"), " "); args != "-level:v 40 -tier main" {
		t.Fatalf("auto mode did not emit the asset recommendation: args=%q config=%#v", args, resolved.WorkerConfig)
	}
}
