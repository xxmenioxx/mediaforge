package quality

import "testing"

func TestNewIntentUsesOutputCropAndContentPath(t *testing.T) {
	intent := NewIntent(IntentInput{
		Preset: "recommended", SourcePath: "/media/raw/anime/Akira/movie.mkv",
		SourceWidth: 720, SourceHeight: 480, SourceVideoBitrate: 4_000_000,
		DurationSeconds: 120, VideoFilters: "crop=720:460:0:10",
	})
	if intent.OutputWidth != 720 || intent.OutputHeight != 460 || intent.ResolutionClass != ResolutionSD || intent.ContentType != ContentAnime {
		t.Fatalf("unexpected quality intent: %#v", intent)
	}
}

func TestVideoToolboxTranslatorPreservesAdaptiveSDCalculation(t *testing.T) {
	intent := NewIntent(IntentInput{Preset: "recommended", SourceWidth: 720, SourceHeight: 1080, SourceVideoBitrate: 4_000_000, VideoFilters: "crop=720:460:0:10"})
	recommendation, err := (VideoToolboxTranslator{}).Translate(intent, WorkerCapabilities{})
	if err != nil || recommendation.BaseTargetBitrate == nil || *recommendation.BaseTargetBitrate != 1_290_000 || recommendation.TargetBitrate == nil || *recommendation.TargetBitrate != 1_330_000 || recommendation.Maxrate == nil || *recommendation.Maxrate != 2_000_000 || recommendation.Buffer == nil || *recommendation.Buffer != 3_330_000 {
		t.Fatalf("unexpected VideoToolbox recommendation: %#v err=%v", recommendation, err)
	}
	if recommendation.Profile != "main" || recommendation.PixelFormat != "yuv420p" {
		t.Fatalf("unexpected VideoToolbox format recommendation: %#v", recommendation)
	}
}

func TestVideoToolboxTranslatorPresetOrderAndSDCeiling(t *testing.T) {
	previous := int64(0)
	for _, preset := range []string{"compact", "medium", "recommended", "best_quality", "high_quality", "archive", "master"} {
		recommendation, err := (VideoToolboxTranslator{}).Translate(NewIntent(IntentInput{Preset: preset, SourceHeight: 480, SourceVideoBitrate: 12_000_000}), WorkerCapabilities{})
		if err != nil || recommendation.TargetBitrate == nil || *recommendation.TargetBitrate <= previous {
			t.Fatalf("preset %s did not increase target bitrate: %#v err=%v", preset, recommendation, err)
		}
		previous = *recommendation.TargetBitrate
	}
	if previous != 6_180_000 {
		t.Fatalf("master SD ceiling changed: %d", previous)
	}
}

func TestVideoToolboxBFrameStrategyAdjustsTargetBeforeRateLimits(t *testing.T) {
	base := IntentInput{Preset: "recommended", SourceHeight: 480, SourceVideoBitrate: 5_500_000, BFrameCount: 3}
	tests := []struct {
		name, policy, effective string
		capability              WorkerCapabilities
		multiplier              float64
	}{
		{name: "verified enabled", policy: "enabled", effective: "enabled", capability: WorkerCapabilities{BFramesVerified: true, BFramesEffective: true}, multiplier: 1},
		{name: "auto unverified", policy: "auto", effective: "auto", multiplier: 1.03},
		{name: "verified disabled", policy: "disabled", effective: "disabled", capability: WorkerCapabilities{BFramesVerified: true, BFramesDisabledVerified: true}, multiplier: 1.08},
		{name: "enabled ineffective", policy: "enabled", effective: "auto", capability: WorkerCapabilities{BFramesVerified: true}, multiplier: 1.03},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := base
			input.BFramePolicy = test.policy
			recommendation, err := (VideoToolboxTranslator{}).Translate(NewIntent(input), test.capability)
			if err != nil || recommendation.EffectiveBFramePolicy != test.effective || recommendation.BFrameEfficiencyMultiplier != test.multiplier || recommendation.BaseTargetBitrate == nil || recommendation.TargetBitrate == nil || recommendation.Maxrate == nil || recommendation.Buffer == nil {
				t.Fatalf("unexpected strategy: %#v err=%v", recommendation, err)
			}
			if *recommendation.TargetBitrate <= 0 || *recommendation.Maxrate != roundBitrateToTwoDecimalMbps(int64(float64(*recommendation.TargetBitrate)*1.5)) || *recommendation.Buffer != roundBitrateToTwoDecimalMbps(int64(float64(*recommendation.TargetBitrate)*2.5)) {
				t.Fatalf("rate limits were not calculated from effective target: %#v", recommendation)
			}
		})
	}
}

func TestVideoToolboxTranslatorLeavesMissingOrCustomBitrateExplicit(t *testing.T) {
	custom, err := (VideoToolboxTranslator{}).Translate(NewIntent(IntentInput{Preset: "custom", SourceVideoBitrate: 4_000_000}), WorkerCapabilities{})
	if err != nil || custom.TargetBitrate != nil || custom.EffectiveBFramePolicy != "auto" {
		t.Fatalf("custom preset must preserve explicit bitrate controls while resolving strategy: %#v err=%v", custom, err)
	}
	recommendation, err := (VideoToolboxTranslator{}).Translate(NewIntent(IntentInput{Preset: "recommended"}), WorkerCapabilities{})
	if err != nil || recommendation.TargetBitrate != nil || len(recommendation.Warnings) == 0 {
		t.Fatalf("missing bitrate must remain explicit: %#v err=%v", recommendation, err)
	}
}

func TestVideoToolboxTranslatorTreatsMissingPresetAsLegacyCustom(t *testing.T) {
	recommendation, err := (VideoToolboxTranslator{}).Translate(NewIntent(IntentInput{
		SourceVideoBitrate: 4_000_000,
		BFramePolicy:       "disabled",
	}), WorkerCapabilities{BFramesDisabledVerified: true})
	if err != nil {
		t.Fatalf("missing preset should be evaluated as custom: %v", err)
	}
	if recommendation.EffectiveRateControl != "vbr" || recommendation.EffectiveBFramePolicy != "disabled" || recommendation.TargetBitrate != nil {
		t.Fatalf("legacy custom controls were not preserved: %#v", recommendation)
	}
}
