package scheduler

import (
	"reflect"
	"testing"

	"github.com/anuelvs/mediaforge/backend/internal/models"
)

func TestResolveLegacySoftwareProfileLocksEncoder(t *testing.T) {
	profile := models.Profile{
		ID: 12, VideoCodec: "x265_10bit", QualityMode: "crf", QualityValue: 18,
		WorkerConfig: models.JSONMap{"videoEncoder": "libx265", "pixFmt": "yuv420p10le"},
	}

	got, err := ResolveExecutionConstraints(profile)
	if err != nil {
		t.Fatal(err)
	}
	if got.CodecFamily != "hevc" || got.EncoderPolicy != EncoderPolicyLocked || got.PreferredEncoder != "libx265" {
		t.Fatalf("unexpected constraints: %#v", got)
	}
	if !reflect.DeepEqual(got.AllowedEncoders, []string{"libx265"}) {
		t.Fatalf("unexpected allowed encoders: %#v", got.AllowedEncoders)
	}
	if got.BitDepth != 10 || got.PixelFormat != "yuv420p10le" {
		t.Fatalf("expected preserved 10-bit output, got %#v", got)
	}
}

func TestResolveLegacyHardwareProfileRestrictsFallback(t *testing.T) {
	profile := models.Profile{
		VideoCodec: "x265_10bit", QualityMode: "crf", QualityValue: 25,
		WorkerConfig: models.JSONMap{"videoEncoder": "hevc_qsv", "useHardwareIfAvailable": true},
	}

	got, err := ResolveExecutionConstraints(profile)
	if err != nil {
		t.Fatal(err)
	}
	if got.EncoderPolicy != EncoderPolicyRestricted || got.FallbackPolicy != FallbackPolicyAllowedOnly {
		t.Fatalf("unexpected fallback contract: %#v", got)
	}
	if !reflect.DeepEqual(got.AllowedEncoders, []string{"hevc_qsv", "libx265"}) {
		t.Fatalf("unexpected allowed encoders: %#v", got.AllowedEncoders)
	}
}

func TestAuthoritativeProfileRejectsCrossCodecEncoder(t *testing.T) {
	profile := models.Profile{
		CodecFamily: "hevc", EncoderPolicy: EncoderPolicyLocked,
		PreferredEncoder: "libx264", AllowedEncoders: models.StringList{"libx264"},
		FallbackPolicy: FallbackPolicyWait, QualityMode: "crf", QualityValue: 20,
	}

	_, err := ResolveExecutionConstraints(profile)
	if err == nil {
		t.Fatal("expected incompatible encoder to be rejected")
	}
}

func TestLockedProfileCannotDeclareFallbackEncoders(t *testing.T) {
	profile := models.Profile{
		CodecFamily: "hevc", EncoderPolicy: EncoderPolicyLocked,
		PreferredEncoder: "libx265", AllowedEncoders: models.StringList{"libx265", "hevc_qsv"},
		FallbackPolicy: FallbackPolicyAllowedOnly, QualityMode: "crf", QualityValue: 18,
	}

	_, err := ResolveExecutionConstraints(profile)
	if err == nil {
		t.Fatal("expected locked profile with multiple encoders to be rejected")
	}
}
