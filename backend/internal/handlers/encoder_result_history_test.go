package handlers

import "testing"

func TestVideoProbeFactsPreferVideoStreamBitrate(t *testing.T) {
	probe := map[string]any{
		"format": map[string]any{"duration": "100.5", "bit_rate": "9000000", "size": "12345"},
		"streams": []any{
			map[string]any{"codec_type": "audio", "bit_rate": "640000"},
			map[string]any{"codec_type": "video", "bit_rate": "4000000", "width": 720.0, "height": 480.0},
		},
	}
	facts := videoProbeFacts(probe)
	if facts.Bitrate != 4_000_000 || facts.Duration != 100.5 || facts.Width != 720 || facts.Height != 480 {
		t.Fatalf("unexpected video facts: %#v", facts)
	}
	if size := probeFormatInt64(probe, "size"); size != 12345 {
		t.Fatalf("format size=%d, want 12345", size)
	}
}
