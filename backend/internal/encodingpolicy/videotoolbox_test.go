package encodingpolicy

import "testing"

func TestVideoToolboxBitrateUsesCroppedHeightAndSharedRateControls(t *testing.T) {
	target, maxrate, buffer, ok := VideoToolboxBitrate("recommended", 4_000_000, 1080, "crop=720:460:0:10")
	if !ok || target != 1720 || maxrate != 2580 || buffer != 4300 {
		t.Fatalf("unexpected VideoToolbox policy: %d/%d/%d ok=%v", target, maxrate, buffer, ok)
	}
}

func TestVideoToolboxBitratePresetOrderForSD(t *testing.T) {
	presets := []string{"compact", "medium", "recommended", "best_quality", "high_quality", "archive", "master"}
	previous := 0
	for _, preset := range presets {
		target, _, _, ok := VideoToolboxBitrate(preset, 8_000_000, 480, "")
		if !ok || target <= previous {
			t.Fatalf("preset %s must exceed the previous SD target: %d <= %d", preset, target, previous)
		}
		previous = target
	}
}

func TestVideoToolboxBitrateRejectsCustomOrMissingSourceRate(t *testing.T) {
	if _, _, _, ok := VideoToolboxBitrate("custom", 4_000_000, 480, ""); ok {
		t.Fatal("custom controls must remain explicit")
	}
	if _, _, _, ok := VideoToolboxBitrate("recommended", 0, 480, ""); ok {
		t.Fatal("source-aware policy requires source video bitrate")
	}
}
