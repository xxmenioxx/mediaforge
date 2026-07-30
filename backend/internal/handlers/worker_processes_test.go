package handlers

import "testing"

func TestProgressFromFFmpegSupportsCurrentProgressKeys(t *testing.T) {
	for _, line := range []string{
		"out_time_us=50000000",
		"out_time_ms=50000000",
		"out_time=00:00:50.000000",
	} {
		progress, ok := progressFromFFmpegLine(line, 100)
		if !ok || progress != 50 {
			t.Fatalf("progressFromFFmpegLine(%q) = %d, %v; want 50, true", line, progress, ok)
		}
	}
}

func TestProgressFromFFmpegRejectsMissingDuration(t *testing.T) {
	if progress, ok := progressFromFFmpegLine("out_time_us=50000000", 0); ok || progress != 0 {
		t.Fatalf("missing duration should not produce progress: %d, %v", progress, ok)
	}
}
