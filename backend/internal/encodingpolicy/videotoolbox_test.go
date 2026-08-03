package encodingpolicy

import "testing"

func TestVideoToolboxBitrateUsesCroppedHeightAndSharedRateControls(t *testing.T) {
	target, maxrate, buffer, ok := VideoToolboxBitrate("recommended", 4_000_000, 1080, "crop=720:460:0:10")
	if !ok || target != 2795 || maxrate != 4193 || buffer != 6988 {
		t.Fatalf("unexpected VideoToolbox policy: %d/%d/%d ok=%v", target, maxrate, buffer, ok)
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
