package handlers

import (
	"reflect"
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

	x265 := resolveHEVCLevel(models.Profile{VideoCodec: "x265", WorkerConfig: models.JSONMap{"videoEncoder": "libx265", "hevcLevelMode": "custom", "hevcLevel": "4.1", "x265Params": "aq-mode=3:vbv-maxrate=20000:vbv-bufsize=40000:level-idc=5.0:high-tier=1"}}, streams)
	params := effectiveX265Params(x265)
	if params != "aq-mode=3:vbv-maxrate=20000:vbv-bufsize=40000:level-idc=4.1:high-tier=0" {
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

func TestHEVCLevelAutoAndRecommendedUseCurrentOutputRequirements(t *testing.T) {
	streams := MediaStreamInventory{Video: []MediaStream{{Width: 1920, Height: 1080, FrameRate: "60/1", Bitrate: 40_000_000}}}
	auto := resolveHEVCLevel(models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{"videoEncoder": "hevc_qsv", "hevcLevelMode": "auto", "hevcLevel": "4.0"}}, streams)
	recommended := resolveHEVCLevel(models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{"videoEncoder": "hevc_qsv", "hevcLevelMode": "recommended", "hevcLevel": "4.0"}}, streams)
	if auto.WorkerConfig["hevcLevelEffective"] != "4.1" {
		t.Fatalf("Auto did not recalculate for 1080p60: %#v", auto.WorkerConfig)
	}
	if recommended.WorkerConfig["hevcLevelEffective"] != "4.1" {
		t.Fatalf("Recommended did not recalculate without a matching fingerprint: %#v", recommended.WorkerConfig)
	}
}

func TestHEVCLevelAutoUsesResolvedCadenceFPS(t *testing.T) {
	profile := models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{"videoEncoder": "hevc_qsv", "hevcLevelMode": "auto", "effectiveOutputFrameRate": "24000/1001"}}
	resolved := resolveHEVCLevel(profile, MediaStreamInventory{Video: []MediaStream{{Width: 1280, Height: 720, FrameRate: "60/1"}}})
	recommendation, ok := resolved.WorkerConfig["hevcLevelRecommendation"].(HEVCLevelRecommendation)
	if !ok || !nearFPS(recommendation.FPS, 24000.0/1001.0, .001) {
		t.Fatalf("Level recommendation ignored resolved cadence FPS: %#v", resolved.WorkerConfig)
	}
}

func TestHEVCLevelUsesResolvedUpscaleGeometry(t *testing.T) {
	streams := MediaStreamInventory{Video: []MediaStream{{Width: 720, Height: 480, SampleAspectRatio: "32:27", DisplayAspectRatio: "16:9", FrameRate: "60/1"}}}
	profile := models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{
		"videoEncoder": "hevc_qsv", "hevcLevelMode": "auto", "upscaleMode": "1080p",
	}}
	profile = resolveUpscaleProfile(profile, streams, UpscaleAnalysisEvidence{})
	profile = resolveHEVCLevel(profile, streams)
	recommendation, ok := profile.WorkerConfig["hevcLevelRecommendation"].(HEVCLevelRecommendation)
	if !ok || recommendation.Width != 1920 || recommendation.Height != 1080 || recommendation.RecommendedLevel != "4.1" {
		t.Fatalf("HEVC Level used pre-upscale geometry: %#v", profile.WorkerConfig)
	}
}

func TestHEVCLevelUsesResolved1080pGeometryAndIVTCFrameRate(t *testing.T) {
	streams := MediaStreamInventory{Video: []MediaStream{{Width: 720, Height: 480, SampleAspectRatio: "32:27", DisplayAspectRatio: "16:9", FrameRate: "30000/1001"}}}
	profile := models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{
		"videoEncoder": "hevc_qsv", "hevcLevelMode": "auto", "upscaleMode": "1080p", "effectiveOutputFrameRate": "24000/1001",
	}}
	profile = resolveUpscaleProfile(profile, streams, UpscaleAnalysisEvidence{})
	profile = resolveHEVCLevel(profile, streams)
	recommendation, ok := profile.WorkerConfig["hevcLevelRecommendation"].(HEVCLevelRecommendation)
	if !ok || recommendation.Width != 1920 || recommendation.Height != 1080 || !nearFPS(recommendation.FPS, 24000.0/1001.0, .001) || recommendation.RecommendedLevel != "4.0" {
		t.Fatalf("HEVC Level ignored resolved 1080p/IVTC plan: %#v", profile.WorkerConfig)
	}
}

func TestHEVCLevelAutoUsesEffectiveOutputGeometry(t *testing.T) {
	tests := []struct {
		name                      string
		sourceWidth, sourceHeight int
		filters                   string
		wantWidth, wantHeight     int
		wantLevel                 string
	}{
		{name: "downscale", sourceWidth: 3840, sourceHeight: 2160, filters: "scale=1920:1080", wantWidth: 1920, wantHeight: 1080, wantLevel: "4.0"},
		{name: "upscale", sourceWidth: 1920, sourceHeight: 1080, filters: "scale=3840:2160", wantWidth: 3840, wantHeight: 2160, wantLevel: "5.0"},
		{name: "crop", sourceWidth: 3840, sourceHeight: 2160, filters: "crop=1920:1080:960:540", wantWidth: 1920, wantHeight: 1080, wantLevel: "4.0"},
		{name: "unchanged", sourceWidth: 1920, sourceHeight: 1080, wantWidth: 1920, wantHeight: 1080, wantLevel: "4.0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile := models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{
				"videoEncoder": "hevc_qsv", "hevcLevelMode": "auto", "videoFilters": test.filters, "effectiveOutputFrameRate": "24000/1001",
			}}
			resolved := resolveHEVCLevel(profile, MediaStreamInventory{Video: []MediaStream{{Width: test.sourceWidth, Height: test.sourceHeight, FrameRate: "24000/1001"}}})
			if workerIntValue(resolved.WorkerConfig["effectiveOutputWidth"], 0) != test.wantWidth || workerIntValue(resolved.WorkerConfig["effectiveOutputHeight"], 0) != test.wantHeight {
				t.Fatalf("effective geometry mismatch: %#v", resolved.WorkerConfig)
			}
			if got := workerStringValue(resolved.WorkerConfig["hevcLevelEffective"]); got != test.wantLevel {
				t.Fatalf("effective Level=%q want %s: %#v", got, test.wantLevel, resolved.WorkerConfig)
			}
		})
	}
}

func TestHEVCLevelUnknownCustomFPSDoesNotFallbackToSource(t *testing.T) {
	base := models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{
		"videoEncoder": "hevc_qsv", "hevcLevelMode": "auto", "effectiveOutputFrameRateUnknown": true,
	}}
	auto := resolveHEVCLevel(base, MediaStreamInventory{Video: []MediaStream{{Width: 1920, Height: 1080, FrameRate: "24/1"}}})
	if workerStringValue(auto.WorkerConfig["hevcLevelEffective"]) != "" || workerStringValue(auto.WorkerConfig["hevcLevelResolutionWarning"]) == "" {
		t.Fatalf("Auto Level silently used source FPS: %#v", auto.WorkerConfig)
	}
	base.WorkerConfig["hevcLevelMode"] = "custom"
	base.WorkerConfig["hevcLevel"] = "4.1"
	manual := resolveHEVCLevel(base, MediaStreamInventory{Video: []MediaStream{{Width: 1920, Height: 1080, FrameRate: "24/1"}}})
	if workerStringValue(manual.WorkerConfig["hevcLevelEffective"]) != "" || workerStringValue(manual.WorkerConfig["hevcLevelResolutionState"]) != hevcLevelResolutionUnresolved {
		t.Fatalf("manual Level was emitted without a compatibility preflight: %#v", manual.WorkerConfig)
	}
}

func TestHEVCLevelUnknownEffectiveFPSClearsStaleDecisionAndFingerprint(t *testing.T) {
	profile := models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{
		"videoEncoder": "hevc_qsv", "hevcLevelMode": "auto",
		"hevcLevelEffective": "4.0", "hevcLevelTier": "main",
		"hevcLevelResolutionState":  hevcLevelResolutionResolved,
		"hevcLevelInputFingerprint": "stale-fingerprint",
		"hevcLevelRecommendation":   HEVCLevelRecommendation{RecommendedLevel: "4.0", Fingerprint: "stale-fingerprint", FPS: 29.97},
		"effectiveOutputFrameRate":  "24000/1001", "effectiveOutputFrameRateUnknown": true,
		"cadenceResolutionWarning": "inverse telecine is unresolved because an explicit or validated field order is required; review required",
	}}
	resolved := resolveHEVCLevel(profile, MediaStreamInventory{Video: []MediaStream{{Width: 1920, Height: 1080, FrameRate: "30000/1001"}}})
	warning := workerStringValue(resolved.WorkerConfig["hevcLevelResolutionWarning"])
	if workerStringValue(resolved.WorkerConfig["hevcLevelEffective"]) != "" || workerStringValue(resolved.WorkerConfig["hevcLevelTier"]) != "" || workerStringValue(resolved.WorkerConfig["hevcLevelResolutionState"]) != hevcLevelResolutionUnresolved {
		t.Fatalf("unknown effective FPS retained a resolved HEVC decision: %#v", resolved.WorkerConfig)
	}
	if _, ok := resolved.WorkerConfig["hevcLevelInputFingerprint"]; ok {
		t.Fatalf("unknown effective FPS retained an HEVC input fingerprint: %#v", resolved.WorkerConfig)
	}
	if _, ok := resolved.WorkerConfig["hevcLevelRecommendation"]; ok {
		t.Fatalf("unknown effective FPS retained a stale Level recommendation: %#v", resolved.WorkerConfig)
	}
	if !strings.Contains(warning, "effective output FPS is unknown") || strings.Contains(warning, "geometry") || !strings.Contains(warning, "explicit or validated field order") {
		t.Fatalf("unknown FPS diagnostic did not identify its cause: %q", warning)
	}
	if err := hevcLevelPlanningError(resolved); err == nil || !strings.Contains(err.Error(), "effective output FPS is unknown") {
		t.Fatalf("unresolved HEVC decision did not block planning actionably: %v", err)
	}
}

func TestHEVCLevelGeometryClassificationIsConservative(t *testing.T) {
	streams := MediaStreamInventory{Video: []MediaStream{{Width: 1920, Height: 1080, FrameRate: "24000/1001"}}}
	for _, test := range []struct {
		name, filters         string
		wantWidth, wantHeight int
		wantKnown             bool
	}{
		{name: "structured scale", filters: "scale=1280:720", wantWidth: 1280, wantHeight: 720, wantKnown: true},
		{name: "structured crop", filters: "crop=1280:720:320:180", wantWidth: 1280, wantHeight: 720, wantKnown: true},
		{name: "preserving chain", filters: "fps=24000/1001,hqdn3d=1.5:1.5:6:6,format=p010le", wantWidth: 1920, wantHeight: 1080, wantKnown: true},
		{name: "advanced zscale", filters: "zscale=w=1280:h=720", wantKnown: false},
		{name: "hardware scale", filters: "scale_qsv=1280:720", wantKnown: false},
		{name: "pad", filters: "pad=1920:1200", wantKnown: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			profile := models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{
				"videoEncoder": "hevc_qsv", "hevcLevelMode": "auto", "videoFilters": test.filters, "effectiveOutputFrameRate": "24000/1001",
			}}
			resolved := resolveHEVCLevel(profile, streams)
			known := !profileWorkerBool(resolved, "effectiveOutputGeometryUnknown", false)
			if known != test.wantKnown {
				t.Fatalf("geometry known=%v want %v: %#v", known, test.wantKnown, resolved.WorkerConfig)
			}
			if !test.wantKnown {
				if workerStringValue(resolved.WorkerConfig["hevcLevelEffective"]) != "" || workerStringValue(resolved.WorkerConfig["hevcLevelResolutionWarning"]) == "" {
					t.Fatalf("Auto Level silently resolved unknown geometry: %#v", resolved.WorkerConfig)
				}
				return
			}
			if workerIntValue(resolved.WorkerConfig["effectiveOutputWidth"], 0) != test.wantWidth || workerIntValue(resolved.WorkerConfig["effectiveOutputHeight"], 0) != test.wantHeight {
				t.Fatalf("effective geometry mismatch: %#v", resolved.WorkerConfig)
			}
		})
	}
}

func TestHEVCLevelResolutionClearsDerivedStateBetweenPasses(t *testing.T) {
	streams := MediaStreamInventory{Video: []MediaStream{{Width: 1920, Height: 1080, FrameRate: "24000/1001"}}}
	base := models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{
		"videoEncoder": "hevc_qsv", "hevcLevelMode": "auto", "effectiveOutputFrameRate": "24000/1001",
	}}

	t.Run("unknown geometry to known geometry", func(t *testing.T) {
		profile := base
		profile.WorkerConfig = cloneWorkerConfig(base.WorkerConfig)
		profile.WorkerConfig["videoFilters"] = "zscale=w=1280:h=720"
		profile = resolveHEVCLevel(profile, streams)
		warning := workerStringValue(profile.WorkerConfig["hevcLevelResolutionWarning"])
		if !profileWorkerBool(profile, "effectiveOutputGeometryUnknown", false) || !strings.Contains(warning, "effective output geometry is unknown") || strings.Contains(warning, "effective output FPS") {
			t.Fatalf("first pass did not remain unresolved: %#v", profile.WorkerConfig)
		}
		profile.WorkerConfig["videoFilters"] = "scale=1280:720"
		profile = resolveHEVCLevel(profile, streams)
		if profileWorkerBool(profile, "effectiveOutputGeometryUnknown", false) || workerStringValue(profile.WorkerConfig["hevcLevelResolutionWarning"]) != "" || workerStringValue(profile.WorkerConfig["hevcLevelEffective"]) != "3.1" {
			t.Fatalf("known geometry retained stale unresolved state: %#v", profile.WorkerConfig)
		}
	})

	t.Run("known geometry to unknown geometry", func(t *testing.T) {
		profile := base
		profile.WorkerConfig = cloneWorkerConfig(base.WorkerConfig)
		profile.WorkerConfig["videoFilters"] = "scale=1280:720"
		profile = resolveHEVCLevel(profile, streams)
		profile.WorkerConfig["videoFilters"] = "scale_qsv=1280:720"
		profile = resolveHEVCLevel(profile, streams)
		if workerStringValue(profile.WorkerConfig["hevcLevelEffective"]) != "" || workerStringValue(profile.WorkerConfig["hevcLevelTier"]) != "" || workerStringValue(profile.WorkerConfig["hevcLevelResolutionState"]) != hevcLevelResolutionUnresolved || !profileWorkerBool(profile, "effectiveOutputGeometryUnknown", false) || workerStringValue(profile.WorkerConfig["hevcLevelResolutionWarning"]) == "" {
			t.Fatalf("unknown geometry retained stale successful state: %#v", profile.WorkerConfig)
		}
	})

	t.Run("auto and manual transitions", func(t *testing.T) {
		profile := resolveHEVCLevel(base, streams)
		profile.WorkerConfig["hevcLevelMode"] = "custom"
		profile.WorkerConfig["hevcLevel"] = "4.1"
		profile.WorkerConfig["x265Params"] = "vbv-maxrate=10000:vbv-bufsize=20000"
		profile = resolveHEVCLevel(profile, streams)
		if workerStringValue(profile.WorkerConfig["hevcLevelEffective"]) != "4.1" || workerStringValue(profile.WorkerConfig["hevcLevelResolutionWarning"]) != "" {
			t.Fatalf("manual pass retained Auto state: %#v", profile.WorkerConfig)
		}
		profile.WorkerConfig["hevcLevelMode"] = "auto"
		profile = resolveHEVCLevel(profile, streams)
		if workerStringValue(profile.WorkerConfig["hevcLevelEffective"]) != "4.0" || profile.WorkerConfig["hevcLevelRecommendation"] == nil {
			t.Fatalf("Auto did not recompute after manual mode: %#v", profile.WorkerConfig)
		}
	})

	t.Run("unknown and known FPS transitions", func(t *testing.T) {
		profile := base
		profile.WorkerConfig = cloneWorkerConfig(base.WorkerConfig)
		profile.WorkerConfig["effectiveOutputFrameRateUnknown"] = true
		profile = resolveHEVCLevel(profile, streams)
		delete(profile.WorkerConfig, "effectiveOutputFrameRateUnknown")
		profile = resolveHEVCLevel(profile, streams)
		if workerStringValue(profile.WorkerConfig["hevcLevelEffective"]) != "4.0" || workerStringValue(profile.WorkerConfig["hevcLevelResolutionWarning"]) != "" {
			t.Fatalf("known FPS retained stale unresolved state: %#v", profile.WorkerConfig)
		}
		profile.WorkerConfig["effectiveOutputFrameRateUnknown"] = true
		profile = resolveHEVCLevel(profile, streams)
		if workerStringValue(profile.WorkerConfig["hevcLevelEffective"]) != "" || workerStringValue(profile.WorkerConfig["hevcLevelResolutionState"]) != hevcLevelResolutionUnresolved || workerStringValue(profile.WorkerConfig["hevcLevelResolutionWarning"]) == "" {
			t.Fatalf("unknown FPS retained stale successful state: %#v", profile.WorkerConfig)
		}
	})
}

func TestHEVCLevelResolutionIsIdempotent(t *testing.T) {
	profile := models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{
		"videoEncoder": "hevc_qsv", "hevcLevelMode": "auto", "effectiveOutputFrameRate": "24000/1001", "videoFilters": "scale=1280:720",
	}}
	streams := MediaStreamInventory{Video: []MediaStream{{Width: 1920, Height: 1080, FrameRate: "30000/1001"}}}
	first := resolveHEVCLevel(profile, streams)
	second := resolveHEVCLevel(first, streams)
	for _, key := range []string{"hevcLevelEffective", "hevcLevelTier", "effectiveOutputWidth", "effectiveOutputHeight", "effectiveOutputGeometryUnknown", "hevcLevelResolutionWarning"} {
		if !reflect.DeepEqual(first.WorkerConfig[key], second.WorkerConfig[key]) {
			t.Fatalf("repeated resolution changed %s: first=%#v second=%#v", key, first.WorkerConfig, second.WorkerConfig)
		}
	}
	firstRecommendation, firstOK := first.WorkerConfig["hevcLevelRecommendation"].(HEVCLevelRecommendation)
	secondRecommendation, secondOK := second.WorkerConfig["hevcLevelRecommendation"].(HEVCLevelRecommendation)
	if !firstOK || !secondOK || !reflect.DeepEqual(firstRecommendation, secondRecommendation) {
		t.Fatalf("repeated resolution changed its recommendation: first=%#v second=%#v", first.WorkerConfig, second.WorkerConfig)
	}
}

func TestAssetHEVCLevelOverrideWinsOverProfileAuto(t *testing.T) {
	profile := applyAssetConversionOverrideToProfile(
		models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{"videoEncoder": "libx265", "hevcLevelMode": "auto", "x265Params": "vbv-maxrate=10000:vbv-bufsize=20000"}},
		AssetConversionOverrideState{HEVCLevelMode: "custom", HEVCLevel: "4.1"},
	)
	resolved := resolveHEVCLevel(profile, MediaStreamInventory{Video: []MediaStream{{Width: 1920, Height: 1080, FrameRate: "24/1"}}})
	if resolved.WorkerConfig["hevcLevelMode"] != "custom" || resolved.WorkerConfig["hevcLevelEffective"] != "4.1" {
		t.Fatalf("asset Custom Level did not win: %#v", resolved.WorkerConfig)
	}
}

func TestCustomHEVCLevelRequiresCompatibilityPreflight(t *testing.T) {
	profile := resolveHEVCLevel(models.Profile{
		VideoCodec: "hevc",
		WorkerConfig: models.JSONMap{
			"videoEncoder": "hevc_qsv", "preferredEncoder": "hardware", "useHardwareIfAvailable": true,
			"hevcLevelMode": "custom", "hevcLevel": "4.0",
		},
	}, MediaStreamInventory{})
	command := shellJoin(videoCodecArgsForResolvedEncoder(profile, nil, "hevc_qsv"))
	if strings.Contains(command, "-level:v 40 -tier main") || workerStringValue(profile.WorkerConfig["hevcLevelResolutionState"]) != hevcLevelResolutionUnresolved {
		t.Fatalf("LAB Custom Level was rendered without a compatibility preflight: %s %#v", command, profile.WorkerConfig)
	}
}

func TestHEVCLevelAutoEmitsMinimumLevelForAsset(t *testing.T) {
	profile := models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{"videoEncoder": "hevc_qsv", "hevcLevelMode": "auto"}}
	resolved := resolveHEVCLevel(profile, MediaStreamInventory{Video: []MediaStream{{Width: 1920, Height: 1080, FrameRate: "24/1"}}})
	if args := strings.Join(hevcLevelArgs(resolved, "hevc_qsv"), " "); args != "-level:v 40 -tier main" {
		t.Fatalf("auto mode did not emit the asset recommendation: args=%q config=%#v", args, resolved.WorkerConfig)
	}
}

func TestResolveHEVCLevelIgnoresSourceAverageBitrate(t *testing.T) {
	profile := models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{"videoEncoder": "hevc_qsv", "hevcLevelMode": "auto"}}
	resolved := resolveHEVCLevel(profile, MediaStreamInventory{Video: []MediaStream{{Width: 1920, Height: 1080, FrameRate: "24/1", Bitrate: 40_000_000}}})
	if got := workerStringValue(resolved.WorkerConfig["hevcLevelEffective"]); got != "4.0" {
		t.Fatalf("source average bitrate promoted output Level: got %q config=%#v", got, resolved.WorkerConfig)
	}
	if workerIntValue(resolved.WorkerConfig["hevcRateControlMaxrateKbps"], 0) != 0 {
		t.Fatalf("source average bitrate became a bounded output constraint: %#v", resolved.WorkerConfig)
	}
}

func TestResolveHEVCLevelUsesOnlyRenderedQSVPeakConstraint(t *testing.T) {
	base := models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{
		"videoEncoder": "hevc_qsv", "useHardwareIfAvailable": true, "hevcLevelMode": "auto", "qsvEffectiveRateControl": "vbr", "qsvTargetBitrate": int64(2_000_000),
	}}
	withTargetOnly := qsvHEVCRateConstraint(base)
	if !withTargetOnly.Bounded || withTargetOnly.MaxrateKbps != 2_000 || withTargetOnly.BufferKbps != 4_000 {
		t.Fatalf("rendered QSV target/maxrate default was not bounded: %#v", withTargetOnly)
	}
	base.WorkerConfig["qsvMaxrate"] = int64(20_000_000)
	withPeak := qsvHEVCRateConstraint(base)
	if !withPeak.Bounded || withPeak.MaxrateKbps != 20_000 {
		t.Fatalf("QSV maxrate did not participate in Level planning: %#v", withPeak)
	}
	base.WorkerConfig["qsvMaxrate"] = int64(40_000_000)
	base.WorkerConfig["qsvBuffer"] = int64(80_000_000)
	constraint := qsvHEVCRateConstraint(base)
	base.WorkerConfig["hevcRateControlFrozen"] = true
	base.WorkerConfig["hevcRateControlTargetBps"] = int64(2_000_000)
	base.WorkerConfig["hevcRateControlMaxrateBps"] = int64(40_000_000)
	base.WorkerConfig["hevcRateControlBufferBps"] = int64(80_000_000)
	args := strings.Join(qsvRateControlArgs(base), " ")
	if !strings.Contains(args, "-maxrate 40000000 -bufsize 80000000") {
		t.Fatalf("QSV rendered maxrate/buffer did not match frozen decision: %s constraint=%#v", args, constraint)
	}
}

func TestRecommendedHEVCLevelFingerprintIsDeterministic(t *testing.T) {
	profile := models.Profile{VideoCodec: "hevc", WorkerConfig: models.JSONMap{"videoEncoder": "hevc_qsv", "hevcLevelMode": "recommended"}}
	streams := MediaStreamInventory{Video: []MediaStream{{Width: 1920, Height: 1080, FrameRate: "24/1"}}}
	first := resolveHEVCLevel(profile, streams)
	reused := resolveHEVCLevel(first, streams)
	if workerStringValue(reused.WorkerConfig["hevcLevelEffective"]) != "4.0" || workerStringValue(reused.WorkerConfig["hevcLevelResolutionWarning"]) != "" {
		t.Fatalf("matching Recommended fingerprint was not reused: %#v", reused.WorkerConfig)
	}
	changed := resolveHEVCLevel(first, MediaStreamInventory{Video: []MediaStream{{Width: 1920, Height: 1080, FrameRate: "60/1"}}})
	if workerStringValue(changed.WorkerConfig["hevcLevelEffective"]) != "4.1" || workerStringValue(changed.WorkerConfig["hevcLevelResolutionWarning"]) == "" {
		t.Fatalf("changed Recommended fingerprint did not recompute: %#v", changed.WorkerConfig)
	}
	unknown := first
	unknown.WorkerConfig["effectiveOutputFrameRateUnknown"] = true
	unknown = resolveHEVCLevel(unknown, streams)
	if workerStringValue(unknown.WorkerConfig["hevcLevelEffective"]) != "" || workerStringValue(unknown.WorkerConfig["hevcLevelResolutionState"]) != hevcLevelResolutionUnresolved || len(hevcLevelArgs(unknown, "hevc_qsv")) != 0 {
		t.Fatalf("stale Recommended Level reached enforcement after inputs became insufficient: %#v", unknown.WorkerConfig)
	}
}

func TestX265HEVCLevelEnforcementIsDeterministic(t *testing.T) {
	streams := MediaStreamInventory{Video: []MediaStream{{Width: 1920, Height: 1080, FrameRate: "24/1"}}}
	advisory := resolveHEVCLevel(models.Profile{VideoCodec: "x265", WorkerConfig: models.JSONMap{
		"videoEncoder": "libx265", "hevcLevelMode": "auto", "x265Params": "level-idc=5.0:high-tier=1:aq-mode=3",
	}}, streams)
	if got := effectiveX265Params(advisory); got != "aq-mode=3" {
		t.Fatalf("unbounded Auto x265 decision was not advisory: %s", got)
	}
	hard := resolveHEVCLevel(models.Profile{VideoCodec: "x265", WorkerConfig: models.JSONMap{
		"videoEncoder": "libx265", "hevcLevelMode": "auto", "x265Params": "vbv-maxrate=20000:vbv-bufsize=40000",
	}}, streams)
	consequences := unknownRecord(hard.WorkerConfig["hevcLevelRateControlConsequences"])
	if workerIntValue(consequences["maxrateKbps"], 0) != 20_000 || workerIntValue(consequences["bufferKbps"], 0) != 40_000 || stringFromUnknown(consequences["vbvConsequence"]) != "existing_vbv_frozen" {
		t.Fatalf("x265 VBV consequences were not frozen before rendering: %#v", consequences)
	}
	if got := effectiveX265Params(hard); got != "vbv-maxrate=20000:vbv-bufsize=40000:level-idc=4.1:high-tier=0" {
		t.Fatalf("bounded Auto x265 decision was not hard-enforced: %s config=%#v", got, hard.WorkerConfig)
	}
	recommended := resolveHEVCLevel(models.Profile{VideoCodec: "x265", WorkerConfig: models.JSONMap{
		"videoEncoder": "libx265", "hevcLevelMode": "recommended", "x265Params": "vbv-maxrate=20000:vbv-bufsize=40000",
	}}, streams)
	if workerStringValue(recommended.WorkerConfig["hevcLevelEnforcement"]) != hevcLevelEnforcementHard || !boolSetting(recommended.WorkerConfig["hevcRateControlBounded"], false) || !strings.Contains(effectiveX265Params(recommended), "level-idc=4.1") {
		t.Fatalf("complete VBV Recommended decision was not hard-enforced: %#v", recommended.WorkerConfig)
	}
	maxrateOnly := resolveHEVCLevel(models.Profile{VideoCodec: "x265", WorkerConfig: models.JSONMap{
		"videoEncoder": "libx265", "hevcLevelMode": "auto", "x265Params": "vbv-maxrate=20000",
	}}, streams)
	if boolSetting(maxrateOnly.WorkerConfig["hevcRateControlBounded"], false) || workerStringValue(maxrateOnly.WorkerConfig["hevcLevelEnforcement"]) != hevcLevelEnforcementAdvisory || strings.Contains(effectiveX265Params(maxrateOnly), "level-idc=") || strings.Contains(effectiveX265Params(maxrateOnly), "high-tier=") {
		t.Fatalf("maxrate-only x265 constraint was treated as deterministic VBV: %#v", maxrateOnly.WorkerConfig)
	}
	bufsizeOnly := resolveHEVCLevel(models.Profile{VideoCodec: "x265", WorkerConfig: models.JSONMap{
		"videoEncoder": "libx265", "hevcLevelMode": "auto", "x265Params": "vbv-bufsize=40000",
	}}, streams)
	if boolSetting(bufsizeOnly.WorkerConfig["hevcRateControlBounded"], false) || workerStringValue(bufsizeOnly.WorkerConfig["hevcLevelEnforcement"]) != hevcLevelEnforcementAdvisory || strings.Contains(effectiveX265Params(bufsizeOnly), "level-idc=") || strings.Contains(effectiveX265Params(bufsizeOnly), "high-tier=") {
		t.Fatalf("bufsize-only x265 constraint was treated as deterministic VBV: %#v", bufsizeOnly.WorkerConfig)
	}
	genericNames := resolveHEVCLevel(models.Profile{VideoCodec: "x265", WorkerConfig: models.JSONMap{
		"videoEncoder": "libx265", "hevcLevelMode": "auto", "x265Params": "maxrate=20000:bufsize=40000",
	}}, streams)
	if boolSetting(genericNames.WorkerConfig["hevcRateControlBounded"], false) || workerStringValue(genericNames.WorkerConfig["hevcLevelEnforcement"]) != hevcLevelEnforcementAdvisory || strings.Contains(effectiveX265Params(genericNames), "level-idc=") || strings.Contains(effectiveX265Params(genericNames), "high-tier=") {
		t.Fatalf("generic x265 rate names were treated as canonical VBV: %#v", genericNames.WorkerConfig)
	}
	explicit := resolveHEVCLevel(models.Profile{VideoCodec: "x265", WorkerConfig: models.JSONMap{
		"videoEncoder": "libx265", "hevcLevelMode": "custom", "hevcLevel": "4.1", "x265Params": "vbv-maxrate=20000:vbv-bufsize=40000",
	}}, streams)
	if got := effectiveX265Params(explicit); got != "vbv-maxrate=20000:vbv-bufsize=40000:level-idc=4.1:high-tier=0" || hevcLevelPlanningError(explicit) != nil {
		t.Fatalf("explicit bounded x265 Level was not deterministically enforced: %s config=%#v", got, explicit.WorkerConfig)
	}
	blocked := resolveHEVCLevel(models.Profile{VideoCodec: "x265", WorkerConfig: models.JSONMap{
		"videoEncoder": "libx265", "hevcLevelMode": "custom", "hevcLevel": "4.1", "x265Params": "vbv-maxrate=20000",
	}}, streams)
	if workerStringValue(blocked.WorkerConfig["hevcLevelResolutionState"]) != hevcLevelResolutionIncompatible || hevcLevelPlanningError(blocked) == nil || !strings.Contains(workerStringValue(blocked.WorkerConfig["hevcLevelResolutionWarning"]), "vbv-maxrate") {
		t.Fatalf("explicit x265 Level without deterministic VBV was not blocked: %#v", blocked.WorkerConfig)
	}
	incompatible := resolveHEVCLevel(models.Profile{VideoCodec: "x265", WorkerConfig: models.JSONMap{
		"videoEncoder": "libx265", "hevcLevelMode": "custom", "hevcLevel": "4.0", "x265Params": "vbv-maxrate=20000:vbv-bufsize=40000",
	}}, MediaStreamInventory{Video: []MediaStream{{Width: 3840, Height: 2160, FrameRate: "24/1"}}})
	if workerStringValue(incompatible.WorkerConfig["hevcLevelResolutionState"]) != hevcLevelResolutionIncompatible || hevcLevelPlanningError(incompatible) == nil {
		t.Fatalf("incompatible explicit Level was not blocked: %#v", incompatible.WorkerConfig)
	}
}
