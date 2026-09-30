package quality

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestResolveSourceBitrateEvidencePrecedence(t *testing.T) {
	tests := []struct {
		name   string
		input  SourceBitrateInput
		method string
		want   int64
	}{
		{
			name:   "trusted video stream metadata",
			input:  SourceBitrateInput{Video: SourceStreamFacts{Index: 2, Bitrate: 5_000_000, Tags: map[string]string{"BPS-eng": "8"}}, ContainerBitrate: 12_000_000},
			method: SourceBitrateMethodStreamMetadata, want: 5_000_000,
		},
		{
			name:   "Matroska BPS statistics",
			input:  SourceBitrateInput{Video: SourceStreamFacts{Index: 1, Tags: map[string]string{"BPS-eng": "4000000"}}, ContainerBitrate: 12_000_000},
			method: SourceBitrateMethodMatroskaStatistics, want: 4_000_000,
		},
		{
			name:   "packet samples before container bitrate",
			input:  SourceBitrateInput{Video: SourceStreamFacts{Index: 0}, DurationSeconds: 30, ContainerBitrate: 12_000_000, SampleWindows: []SourceBitrateSampleWindow{{DurationSeconds: .5, Bytes: 250_000, Status: "success"}, {DurationSeconds: .5, Bytes: 300_000, Status: "success"}}},
			method: SourceBitrateMethodPacketSampling, want: 4_400_000,
		},
		{
			name:   "container after sampling is unavailable",
			input:  SourceBitrateInput{Video: SourceStreamFacts{Index: 0}, ContainerBitrate: 12_000_000, Audio: []SourceStreamFacts{{Bitrate: 1_000_000}}, Subtitles: []SourceStreamFacts{{Bitrate: 100_000}}},
			method: SourceBitrateMethodContainerBitrate, want: 10_900_000,
		},
		{
			name:   "file size and duration final fallback",
			input:  SourceBitrateInput{Video: SourceStreamFacts{Index: 0}, FileSizeBytes: 100_000_000, DurationSeconds: 100, Audio: []SourceStreamFacts{{Bitrate: 1_000_000}}},
			method: SourceBitrateMethodFileSizeDuration, want: 7_000_000,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ResolveSourceBitrate(test.input)
			if got.Method != test.method || got.Bitrate != test.want {
				t.Fatalf("evidence=%#v; want method=%s bitrate=%d", got, test.method, test.want)
			}
		})
	}
}

func TestAudioHeavyContainerDoesNotOverrideVideoOnlyEvidence(t *testing.T) {
	evidence := ResolveSourceBitrate(SourceBitrateInput{
		Video:            SourceStreamFacts{Index: 0, Bitrate: 2_000_000},
		Audio:            []SourceStreamFacts{{Bitrate: 8_000_000}},
		ContainerBitrate: 11_000_000,
	})
	if evidence.Bitrate != 2_000_000 || evidence.Method != SourceBitrateMethodStreamMetadata {
		t.Fatalf("audio-heavy container inflated video evidence: %#v", evidence)
	}
}

func TestResolveSourceBitrateUsesMatroskaBytesAndRejectsStaleStatistics(t *testing.T) {
	evidence := ResolveSourceBitrate(SourceBitrateInput{Video: SourceStreamFacts{
		Index: 3, DurationSeconds: 10, Tags: map[string]string{"NUMBER_OF_BYTES-eng": "5000000"},
	}})
	if evidence.Method != SourceBitrateMethodMatroskaStatistics || evidence.Bitrate != 4_000_000 {
		t.Fatalf("Matroska byte statistics not used: %#v", evidence)
	}
	stale := ResolveSourceBitrate(SourceBitrateInput{
		Video: SourceStreamFacts{Index: 3, DurationSeconds: 10, Tags: map[string]string{
			"NUMBER_OF_BYTES-eng": "5000000", "BPS-eng": "4000000", "ENCODER": "Lavc hevc_qsv", "_STATISTICS_WRITING_APP-eng": "MakeMKV"}},
		ContainerBitrate: 2_000_000,
	})
	if stale.Method != SourceBitrateMethodContainerBitrate || stale.Bitrate != 2_000_000 {
		t.Fatalf("stale Matroska statistics were trusted: %#v", stale)
	}
}

func TestDistributedSourcePacketWindowsAreDeterministicBoundedAndInterior(t *testing.T) {
	first := DistributedSourcePacketWindows(1800)
	second := DistributedSourcePacketWindows(1800)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("window plan changed: %#v versus %#v", first, second)
	}
	if len(first) != MaxSourceBitrateSampleWindows {
		t.Fatalf("got %d windows, max is %d", len(first), MaxSourceBitrateSampleWindows)
	}
	for _, window := range first {
		if window.StartSeconds <= 0 || window.StartSeconds+window.DurationSeconds >= 1800 || window.DurationSeconds > SourceBitrateSampleWindowSeconds {
			t.Fatalf("window is not a bounded interior sample: %#v", window)
		}
	}
}

func TestDistributedSourcePacketWindowsHandleShortAssets(t *testing.T) {
	for _, duration := range []float64{.08, .5, 1.25, 2} {
		windows := DistributedSourcePacketWindows(duration)
		if len(windows) == 0 || len(windows) > MaxSourceBitrateSampleWindows {
			t.Fatalf("duration %.3f produced invalid windows: %#v", duration, windows)
		}
		for _, window := range windows {
			if window.StartSeconds < 0 || window.DurationSeconds <= 0 || window.StartSeconds+window.DurationSeconds > duration+.001 {
				t.Fatalf("duration %.3f produced out-of-range window: %#v", duration, window)
			}
		}
	}
}

func TestSampleSourceVideoPacketsSelectsVideoBoundsAndKeepsPartialSuccess(t *testing.T) {
	var calls int
	runner := func(ctx context.Context, args []string, limit int) ([]byte, error) {
		calls++
		if limit != SourceBitrateSampleOutputLimit || !containsArgs(args, "-show_packets") || !containsArgs(args, "-select_streams") || !containsArgs(args, "v:0") {
			t.Fatalf("unexpected packet sample command args=%v limit=%d", args, limit)
		}
		if calls == 2 {
			return nil, errors.New("seek failed")
		}
		return []byte("100\n200\n"), nil
	}
	windows := SampleSourceVideoPacketsWithRunner(context.Background(), "/media/input.mkv", "matroska,webm", 30, 0, runner)
	if len(windows) != 5 || calls != 5 {
		t.Fatalf("unexpected bounded windows: calls=%d windows=%#v", calls, windows)
	}
	if windows[0].Status != "success" || windows[1].Status != "failed" || windows[2].Bytes != 300 {
		t.Fatalf("partial sample failures were not isolated: %#v", windows)
	}
}

func TestSampleSourceVideoPacketsSkipsUnsupportedFormatAndHonorsCancellation(t *testing.T) {
	calls := 0
	runner := func(context.Context, []string, int) ([]byte, error) { calls++; return []byte("1\n"), nil }
	if got := SampleSourceVideoPacketsWithRunner(context.Background(), "/media/input.xyz", "unknown", 20, 0, runner); len(got) != 0 || calls != 0 {
		t.Fatalf("unsupported format was sampled: %#v calls=%d", got, calls)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := SampleSourceVideoPacketsWithRunner(ctx, "/media/input.mkv", "matroska", 20, 0, runner)
	if calls != 0 || len(got) == 0 || got[0].Status != "cancelled" {
		t.Fatalf("cancelled sampling did not fail closed: %#v calls=%d", got, calls)
	}
}

func TestResolveSourceBitrateUsesSuccessfulWindowsAndRecordsFailureCount(t *testing.T) {
	evidence := ResolveSourceBitrate(SourceBitrateInput{
		Video: SourceStreamFacts{Index: 1}, DurationSeconds: 5, RequestedWindows: 3,
		SampleWindows: []SourceBitrateSampleWindow{
			{DurationSeconds: .5, Bytes: 100_000, Status: "success"},
			{DurationSeconds: .5, Status: "timeout"},
			{DurationSeconds: .5, Bytes: 200_000, Status: "success"},
		},
	})
	if evidence.Method != SourceBitrateMethodPacketSampling || evidence.Bitrate != 2_400_000 || evidence.SuccessfulWindows != 2 || evidence.Confidence != "low" {
		t.Fatalf("unexpected partial packet evidence: %#v", evidence)
	}
	if evidence.MinimumBitrate == nil || *evidence.MinimumBitrate != 1_600_000 || evidence.MaximumBitrate == nil || *evidence.MaximumBitrate != 3_200_000 {
		t.Fatalf("sample range was not retained: %#v", evidence)
	}
}

func containsArgs(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
