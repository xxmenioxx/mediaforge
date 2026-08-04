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
	if err != nil || recommendation.TargetBitrate == nil || *recommendation.TargetBitrate != 1_720_000 || recommendation.Maxrate == nil || *recommendation.Maxrate != 2_580_000 || recommendation.Buffer == nil || *recommendation.Buffer != 4_300_000 {
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
	if previous != 6_000_000 {
		t.Fatalf("master SD ceiling changed: %d", previous)
	}
}

func TestVideoToolboxTranslatorLeavesMissingOrCustomBitrateExplicit(t *testing.T) {
	if _, err := (VideoToolboxTranslator{}).Translate(NewIntent(IntentInput{Preset: "custom", SourceVideoBitrate: 4_000_000}), WorkerCapabilities{}); err == nil {
		t.Fatal("custom preset must require explicit encoder settings")
	}
	recommendation, err := (VideoToolboxTranslator{}).Translate(NewIntent(IntentInput{Preset: "recommended"}), WorkerCapabilities{})
	if err != nil || recommendation.TargetBitrate != nil || len(recommendation.Warnings) == 0 {
		t.Fatalf("missing bitrate must remain explicit: %#v err=%v", recommendation, err)
	}
}
