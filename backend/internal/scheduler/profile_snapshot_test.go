package scheduler

import (
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func TestCaptureProfileSnapshotIsDeepAndVersioned(t *testing.T) {
	profile := models.Profile{
		ID: 7, Name: "Priority Archive", ProfileVersion: 3,
		Container: "mkv", VideoCodec: "x265_10bit", CodecFamily: "hevc",
		EncoderPolicy: EncoderPolicyLocked, PreferredEncoder: "libx265",
		AllowedEncoders: models.StringList{"libx265"}, FallbackPolicy: FallbackPolicyWait,
		BitDepth: 10, PixelFormat: "yuv420p10le", QualityStrategy: "high",
		QualityMode: "crf", QualityValue: 18, AudioCodec: "copy",
		WorkerConfig: models.JSONMap{"videoPreset": "slow"},
	}
	capturedAt := time.Date(2026, 7, 13, 23, 0, 0, 0, time.UTC)

	snapshot, err := CaptureProfileSnapshot(profile, capturedAt, "queue_create")
	if err != nil {
		t.Fatal(err)
	}
	profile.WorkerConfig["videoPreset"] = "veryfast"

	if snapshot["profileVersion"] != float64(3) || snapshot["captureSource"] != "queue_create" {
		t.Fatalf("unexpected snapshot metadata: %#v", snapshot)
	}
	workerConfig, ok := snapshot["workerConfig"].(map[string]any)
	if !ok || workerConfig["videoPreset"] != "slow" {
		t.Fatalf("snapshot changed with source profile: %#v", snapshot["workerConfig"])
	}
	constraints, ok := snapshot["constraints"].(map[string]any)
	if !ok || constraints["preferredEncoder"] != "libx265" || constraints["qualityValue"] != float64(18) {
		t.Fatalf("missing authoritative constraints: %#v", snapshot["constraints"])
	}
	restored, err := RestoreProfileSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if restored.ProfileVersion != 3 || restored.VideoCodec != "x265_10bit" || restored.QualityValue != 18 {
		t.Fatalf("snapshot did not restore captured profile: %#v", restored)
	}
}
