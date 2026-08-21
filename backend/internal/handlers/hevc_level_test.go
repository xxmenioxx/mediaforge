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
		Width: 1920, Height: 1080, Bitrate: 6_418_000,
		VideoStreams: models.JSONList{map[string]any{"avgFrameRate": "24/1", "level": 150}},
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

func TestHEVCLevelAutoLeavesEncoderDefaults(t *testing.T) {
	profile := models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{"videoEncoder": "hevc_qsv", "hevcLevelMode": "auto"}}
	resolved := resolveHEVCLevel(profile, MediaStreamInventory{Video: []MediaStream{{Width: 1920, Height: 1080, FrameRate: "24/1"}}})
	if len(hevcLevelArgs(resolved, "hevc_qsv")) != 0 || effectiveX265Params(resolved) != "" {
		t.Fatalf("auto mode emitted an explicit level: %#v", resolved.WorkerConfig)
	}
}
