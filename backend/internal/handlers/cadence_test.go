package handlers

import (
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func cadenceSignals(fps float64, repeats int, pattern string, progressive bool) FrameSignalSummary {
	progressiveFrames := 0
	if progressive {
		progressiveFrames = 240
	}
	return FrameSignalSummary{DecodedFrames: 240, ProgressiveFrames: progressiveFrames, InterlacedFrames: 240 - progressiveFrames,
		RepeatPictFrames: repeats, Cadence: pattern, ActualTimespan: 239 / fps, EffectiveFPS: fps}
}

func TestSummarizeTimestampDeltasKeepsCompactCadenceEvidence(t *testing.T) {
	summary := summarizeTimestampDeltas([]float64{.0334, .0500, .0333, .0501, .0334, .0500})
	if summary.Count != 6 || len(summary.Dominant) != 2 || summary.Dominant[0] != .033 || summary.Dominant[1] != .05 {
		t.Fatalf("unexpected timestamp summary %#v", summary)
	}
}

func progressiveCadenceWindow(fps float64, repeated bool) InterlaceWindow {
	repeats := 0
	if repeated {
		repeats = 48
	}
	return InterlaceWindow{Status: "progressive", FrameSignals: cadenceSignals(fps, repeats, "stable_every_5_frames", true)}
}

func progressiveFrameStructureWindow(fps float64, repeated bool) FrameStructureWindow {
	window := progressiveCadenceWindow(fps, repeated)
	return FrameStructureWindow{FrameSignals: window.FrameSignals}
}

func TestAnalyzeCadenceDetectsHighConfidenceSoftTelecine(t *testing.T) {
	frameStructure := QSVFrameStructureAnalysis{Windows: []FrameStructureWindow{
		progressiveFrameStructureWindow(24000.0/1001.0, true), progressiveFrameStructureWindow(23.98, true), progressiveFrameStructureWindow(23.97, true),
		progressiveFrameStructureWindow(23.99, true), progressiveFrameStructureWindow(23.96, true),
	}}
	analysis := analyzeCadence("mpeg2video", "30000/1001", InterlaceAnalysis{Status: "progressive", Confidence: .99}, frameStructure)
	recommendation := recommendCadence(analysis)
	if analysis.Type != "soft_telecine" || recommendation.OutputFrameRate != "24000/1001" || recommendation.Operation != "remove_soft_telecine" {
		t.Fatalf("expected automatic soft-telecine normalization, got %#v", analysis)
	}
}

func TestAnalyzeCadenceRejectsWeakSoftTelecineSignals(t *testing.T) {
	tests := []struct {
		name    string
		codec   string
		windows []FrameStructureWindow
	}{
		{name: "wrong codec", codec: "h264", windows: []FrameStructureWindow{progressiveFrameStructureWindow(23.976, true), progressiveFrameStructureWindow(23.976, true)}},
		{name: "no repeat pict", codec: "mpeg2video", windows: []FrameStructureWindow{progressiveFrameStructureWindow(23.976, false), progressiveFrameStructureWindow(23.976, false)}},
		{name: "single region", codec: "mpeg2video", windows: []FrameStructureWindow{progressiveFrameStructureWindow(23.976, true)}},
		{name: "irregular repeats", codec: "mpeg2video", windows: []FrameStructureWindow{
			{FrameSignals: cadenceSignals(23.976, 48, "irregular", true)}, {FrameSignals: cadenceSignals(23.976, 48, "irregular", true)},
		}},
		{name: "interlaced pictures", codec: "mpeg2video", windows: []FrameStructureWindow{
			{FrameSignals: cadenceSignals(23.976, 48, "stable_every_5_frames", false)}, {FrameSignals: cadenceSignals(23.976, 48, "stable_every_5_frames", false)},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			analysis := analyzeCadence(test.codec, "30000/1001", InterlaceAnalysis{Status: "progressive", Confidence: .99}, QSVFrameStructureAnalysis{Windows: test.windows})
			if analysis.Type == "soft_telecine" || recommendCadence(analysis).Operation == "remove_soft_telecine" {
				t.Fatalf("weak evidence must not normalize automatically: %#v", analysis)
			}
		})
	}
}

func TestAnalyzeCadenceRetainsOneAmbiguousRegionWithoutAutoCorrection(t *testing.T) {
	windows := []FrameStructureWindow{
		progressiveFrameStructureWindow(23.976, true), progressiveFrameStructureWindow(23.976, true), progressiveFrameStructureWindow(23.976, true), progressiveFrameStructureWindow(23.976, true), {},
	}
	analysis := analyzeCadence("mpeg2video", "30000/1001", InterlaceAnalysis{Status: "progressive", Confidence: .99}, QSVFrameStructureAnalysis{Windows: windows})
	if analysis.Type != "soft_telecine" || analysis.RegionCount != 5 || analysis.SampleCount != 4 || analysis.AmbiguousSampleCount != 1 || analysis.SoftTelecineSamples != 4 {
		t.Fatalf("ambiguous region evidence was lost: %#v", analysis)
	}
	if recommendation := recommendCadence(analysis); recommendation.Operation != "review" {
		t.Fatalf("medium-confidence evidence must not auto-correct: %#v", recommendation)
	}
}

func TestAnalyzeCadencePreservesNativeProgressive(t *testing.T) {
	analysis := analyzeCadence("mpeg2video", "30000/1001", InterlaceAnalysis{Status: "progressive", Confidence: .99, Windows: []InterlaceWindow{
		progressiveCadenceWindow(30000.0/1001.0, false), progressiveCadenceWindow(29.97, false),
	}})
	if analysis.Type != "native_progressive" || recommendCadence(analysis).Operation != "preserve" {
		t.Fatalf("expected native progressive cadence, got %#v", analysis)
	}
}

func TestAnalyzeCadenceKeepsMixedSamplesReviewOnly(t *testing.T) {
	analysis := analyzeCadence("mpeg2video", "30000/1001", InterlaceAnalysis{Status: "progressive", Windows: []InterlaceWindow{
		progressiveCadenceWindow(24000.0/1001.0, true), progressiveCadenceWindow(30000.0/1001.0, false),
	}})
	if analysis.Type != "mixed" || recommendCadence(analysis).Operation != "review" {
		t.Fatalf("expected mixed cadence review, got %#v", analysis)
	}
}

func TestAnalyzeCadenceMarksNonFilmRateDisagreementMixed(t *testing.T) {
	analysis := analyzeCadence("h264", "30000/1001", InterlaceAnalysis{Status: "progressive"}, QSVFrameStructureAnalysis{Windows: []FrameStructureWindow{
		progressiveFrameStructureWindow(25, false), progressiveFrameStructureWindow(29.97, false),
	}})
	if analysis.Type != "mixed" || recommendCadence(analysis).Operation != "review" {
		t.Fatalf("expected rate disagreement to remain review-only, got %#v", analysis)
	}
}

func TestAnalyzeCadenceKeepsFieldBasedTelecineSeparate(t *testing.T) {
	analysis := analyzeCadence("mpeg2video", "30000/1001", InterlaceAnalysis{Status: "telecine", Confidence: .91})
	recommendation := recommendCadence(analysis)
	if analysis.Type != "hard_telecine" || recommendation.Operation != "inverse_telecine" || recommendation.OutputFrameRate != "24000/1001" {
		t.Fatalf("hard telecine must not use soft-telecine normalization: %#v", analysis)
	}
}

func TestFFmpegCommandBuilderNormalizesConfirmedSoftTelecine(t *testing.T) {
	plan := MediaJobPlan{
		InputPath: "/media/raw/dvd.mkv", OutputPath: "/media/staging/dvd.mkv", ProcessingMode: ProcessingModeFullEncode,
		Profile:               models.Profile{VideoCodec: "x265", QualityMode: "crf", QualityValue: 20},
		Streams:               MediaStreamInventory{Video: []MediaStream{{Index: 0, Codec: "mpeg2video", FrameRate: "30000/1001"}}},
		Cadence:               CadenceAnalysis{Version: cadenceAnalysisVersion, Type: "soft_telecine", Confidence: .98},
		CadenceRecommendation: CadenceRecommendation{Version: 1, Operation: "remove_soft_telecine", OutputFrameRate: "24000/1001", Confidence: .98},
	}
	command := shellJoin(FFmpegCommandBuilder{}.Build(plan))
	assertContains(t, command, "-vf fps=24000/1001")
	assertContains(t, command, "-fps_mode cfr")
	assertNotContains(t, command, "-r 24000/1001")
	assertNotContains(t, command, "fieldmatch")
	assertNotContains(t, command, "decimate")
	assertNotContains(t, command, "bwdif")
}

func TestLegacyMotionModesMapWithoutChangingBehavior(t *testing.T) {
	tests := []struct{ legacy, field, cadence, order string }{
		{"off", "preserve", "preserve", ""}, {"auto", "auto", "auto", ""}, {"force", "deinterlace", "preserve", ""},
		{"ivtc_tff", "preserve", "inverse_telecine", "tff"}, {"ivtc_bff", "preserve", "inverse_telecine", "bff"},
	}
	for _, test := range tests {
		profile := profileWithResolvedFieldAndCadenceModes(models.Profile{WorkerConfig: models.JSONMap{"deinterlaceMode": test.legacy}}, InterlaceAnalysis{})
		if got := workerStringValue(profile.WorkerConfig["fieldStructureMode"]); got != test.field {
			t.Errorf("%s field=%s", test.legacy, got)
		}
		if got := workerStringValue(profile.WorkerConfig["cadenceMode"]); got != test.cadence {
			t.Errorf("%s cadence=%s", test.legacy, got)
		}
		if test.order != "" && workerStringValue(profile.WorkerConfig["deinterlaceMode"]) != "ivtc_"+test.order {
			t.Errorf("%s lost field order: %#v", test.legacy, profile.WorkerConfig)
		}
	}
}

func TestExplicitSoftTelecineRemovalOverridesLowConfidenceAnalysis(t *testing.T) {
	profile := models.Profile{WorkerConfig: models.JSONMap{"fieldStructureMode": "preserve", "cadenceMode": "remove_soft_telecine"}}
	resolved := profileWithCadenceOutputDecision(profile, CadenceAnalysis{Type: "unknown"}, CadenceRecommendation{Version: 1, Operation: "review"})
	if workerStringValue(resolved.WorkerConfig["videoFilters"]) != "fps=24000/1001" || workerStringValue(resolved.WorkerConfig["effectiveOutputFrameRate"]) != "24000/1001" {
		t.Fatalf("explicit semantic override was not resolved: %#v", resolved.WorkerConfig)
	}
}

func TestForceDeinterlacePreventsContradictorySoftCadenceNormalization(t *testing.T) {
	profile := models.Profile{WorkerConfig: models.JSONMap{"fieldStructureMode": "deinterlace", "cadenceMode": "remove_soft_telecine"}}
	resolved := profileWithCadenceOutputDecision(profile, CadenceAnalysis{Type: "soft_telecine", Confidence: .99}, CadenceRecommendation{Version: 1, Operation: "remove_soft_telecine", OutputFrameRate: "24000/1001", Confidence: .99})
	if workerStringValue(resolved.WorkerConfig["effectiveOutputFrameRate"]) != "" || workerStringValue(resolved.WorkerConfig["cadenceResolutionWarning"]) == "" {
		t.Fatalf("contradictory operations were not rejected: %#v", resolved.WorkerConfig)
	}
}

func TestCadenceValidationRequiresAverageAndRealFrameRates(t *testing.T) {
	worker := map[string]interface{}{"effectiveOutputFrameRate": "24000/1001"}
	for _, test := range []struct {
		name, avg, real, want string
	}{
		{name: "both match", avg: "24000/1001", real: "24000/1001", want: "validated"},
		{name: "average only", avg: "24000/1001", want: "unverified"},
		{name: "real only", real: "24000/1001", want: "unverified"},
		{name: "real mismatch", avg: "24000/1001", real: "30000/1001", want: "mismatch"},
		{name: "average mismatch", avg: "30000/1001", real: "24000/1001", want: "mismatch"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := validateCadenceOutputFrameRate(worker, models.JSONMap{"frameRate": test.avg, "realFrameRate": test.real})
			if result["status"] != test.want {
				t.Fatalf("status=%v want %s: %#v", result["status"], test.want, result)
			}
		})
	}
}

func TestExplicitFPSFilterWinsOverAutomaticCadence(t *testing.T) {
	profile := models.Profile{WorkerConfig: models.JSONMap{"videoFilters": "fps=25"}}
	resolved := profileWithAutomaticCadence(profile, CadenceAnalysis{Type: "soft_telecine", Confidence: .99})
	if got := workerStringValue(resolved.WorkerConfig["videoFilters"]); got != "fps=25" {
		t.Fatalf("expected explicit filter to win, got %q", got)
	}
	if got := workerStringValue(resolved.WorkerConfig["effectiveOutputFrameRate"]); got != "25/1" {
		t.Fatalf("explicit FPS was not recorded as effective output rate: %q", got)
	}
	if got := workerStringValue(resolved.WorkerConfig["effectiveCadenceOperation"]); got != "explicit_fps_filter" {
		t.Fatalf("explicit FPS operation was not recorded: %q", got)
	}
}

func TestUnparseableExplicitFPSFilterDoesNotKeepStaleEffectiveRate(t *testing.T) {
	profile := models.Profile{WorkerConfig: models.JSONMap{"videoFilters": "fps=fps*2", "effectiveOutputFrameRate": "30000/1001"}}
	resolved := profileWithAutomaticCadence(profile, CadenceAnalysis{Type: "soft_telecine", Confidence: .99})
	if got := workerStringValue(resolved.WorkerConfig["effectiveOutputFrameRate"]); got != "" {
		t.Fatalf("unparseable explicit FPS retained stale rate %q", got)
	}
	if workerStringValue(resolved.WorkerConfig["cadenceResolutionWarning"]) == "" {
		t.Fatalf("unparseable explicit FPS did not record a warning: %#v", resolved.WorkerConfig)
	}
	if !profileWorkerBool(resolved, "effectiveOutputFrameRateUnknown", false) {
		t.Fatalf("unparseable explicit FPS was not marked unknown: %#v", resolved.WorkerConfig)
	}
}

func TestExplicitFPSIsEffectiveWithoutAutomaticCadenceDecision(t *testing.T) {
	profile := models.Profile{WorkerConfig: models.JSONMap{"cadenceMode": "preserve", "videoFilters": "scale=1280:720,fps=25"}}
	resolved := profileWithAutomaticCadence(profile, CadenceAnalysis{Type: "unknown"})
	if got := workerStringValue(resolved.WorkerConfig["effectiveOutputFrameRate"]); got != "25/1" {
		t.Fatalf("explicit FPS depended on automatic Cadence analysis: %#v", resolved.WorkerConfig)
	}
}

func TestCadencePreserveModeDisablesAutomaticNormalization(t *testing.T) {
	profile := models.Profile{WorkerConfig: models.JSONMap{"cadenceMode": "preserve"}}
	resolved := profileWithAutomaticCadence(profile, CadenceAnalysis{Type: "soft_telecine", Confidence: .99})
	if got := workerStringValue(resolved.WorkerConfig["videoFilters"]); got != "" {
		t.Fatalf("preserve mode added filter %q", got)
	}
	if args := cadenceOutputArgs(resolved); len(args) != 0 {
		t.Fatalf("preserve mode emitted output args %#v", args)
	}
}

func TestCadenceResolutionClearsDerivedStateBetweenPasses(t *testing.T) {
	analysis := CadenceAnalysis{Type: "soft_telecine", Confidence: .99}
	recommendation := CadenceRecommendation{Version: 1, Operation: "remove_soft_telecine", OutputFrameRate: "24000/1001", Confidence: .99}
	resolved := profileWithCadenceOutputDecision(models.Profile{WorkerConfig: models.JSONMap{"cadenceMode": "auto"}}, analysis, recommendation)
	if workerStringValue(resolved.WorkerConfig["effectiveOutputFrameRate"]) != "24000/1001" {
		t.Fatalf("first pass did not resolve cadence: %#v", resolved.WorkerConfig)
	}
	resolved.WorkerConfig["cadenceMode"] = "preserve"
	resolved = profileWithCadenceOutputDecision(resolved, CadenceAnalysis{Type: "unknown"}, CadenceRecommendation{Version: 1, Operation: "review"})
	if workerStringValue(resolved.WorkerConfig["effectiveOutputFrameRate"]) != "" || workerStringValue(resolved.WorkerConfig["effectiveCadenceOperation"]) != "" || workerStringValue(resolved.WorkerConfig["videoFilters"]) != "" {
		t.Fatalf("Preserve retained stale automatic cadence state: %#v", resolved.WorkerConfig)
	}

	resolved.WorkerConfig["videoFilters"] = "fps=fps*2"
	resolved = profileWithCadenceOutputDecision(resolved, analysis, recommendation)
	if !profileWorkerBool(resolved, "effectiveOutputFrameRateUnknown", false) {
		t.Fatalf("unknown custom FPS was not recorded: %#v", resolved.WorkerConfig)
	}
	resolved.WorkerConfig["videoFilters"] = ""
	resolved = profileWithCadenceOutputDecision(resolved, CadenceAnalysis{Type: "unknown"}, CadenceRecommendation{Version: 1, Operation: "review"})
	if profileWorkerBool(resolved, "effectiveOutputFrameRateUnknown", false) || workerStringValue(resolved.WorkerConfig["cadenceResolutionWarning"]) != "" {
		t.Fatalf("stale unknown FPS state survived a later pass: %#v", resolved.WorkerConfig)
	}
}

func TestCadenceResolutionIsIdempotent(t *testing.T) {
	profile := models.Profile{WorkerConfig: models.JSONMap{"cadenceMode": "auto", "videoFilters": "hqdn3d=1.5:1.5:6:6"}}
	analysis := CadenceAnalysis{Type: "soft_telecine", Confidence: .99}
	recommendation := CadenceRecommendation{Version: 1, Operation: "remove_soft_telecine", OutputFrameRate: "24000/1001", Confidence: .99}
	first := profileWithCadenceOutputDecision(profile, analysis, recommendation)
	second := profileWithCadenceOutputDecision(first, analysis, recommendation)
	if got := workerStringValue(second.WorkerConfig["videoFilters"]); got != "fps=24000/1001,hqdn3d=1.5:1.5:6:6" {
		t.Fatalf("repeated resolution accumulated or lost filters: %q", got)
	}
	if workerStringValue(second.WorkerConfig["effectiveCadenceOperation"]) != "remove_soft_telecine" {
		t.Fatalf("repeated resolution changed the effective operation: %#v", second.WorkerConfig)
	}
}
