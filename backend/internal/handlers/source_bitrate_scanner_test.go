package handlers

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/quality"
)

func TestResolveSourceBitrateEvidenceUsesBoundedVideoPacketSamples(t *testing.T) {
	var calls int
	sampler := func(_ context.Context, path, format string, duration float64, ordinal int) []quality.SourceBitrateSampleWindow {
		calls++
		if path != "/media/movie.mkv" || format != "matroska,webm" || duration != 90 || ordinal != 0 {
			t.Fatalf("unexpected sampler input path=%s format=%s duration=%v ordinal=%d", path, format, duration, ordinal)
		}
		return []quality.SourceBitrateSampleWindow{
			{StartSeconds: 15, DurationSeconds: .5, Bytes: 250_000, Status: "success"},
			{StartSeconds: 30, DurationSeconds: .5, Bytes: 300_000, Status: "success"},
			{StartSeconds: 45, DurationSeconds: .5, Bytes: 275_000, Status: "success"},
			{StartSeconds: 60, DurationSeconds: .5, Status: "timeout"},
			{StartSeconds: 75, DurationSeconds: .5, Bytes: 280_000, Status: "success"},
		}
	}
	probe := FFProbeResult{}
	probe.Format.FormatName = "matroska,webm"
	probe.Format.Duration = "90"
	probe.Format.Bitrate = "18000000"
	probe.Streams = []FFProbeStream{{Index: 0, CodecType: "video", CodecName: "hevc"}, {Index: 1, CodecType: "audio", Bitrate: "10000000"}}
	evidence := resolveSourceBitrateEvidence(context.Background(), "/media/movie.mkv", 100_000_000, probe, sampler)
	if calls != 1 || evidence.Method != quality.SourceBitrateMethodPacketSampling || evidence.Bitrate != 4_420_000 || evidence.RequestedWindows != 5 || evidence.SuccessfulWindows != 4 || evidence.Confidence != "medium" {
		t.Fatalf("unexpected packet bitrate evidence: calls=%d evidence=%#v", calls, evidence)
	}
	if evidence.StreamIndex == nil || *evidence.StreamIndex != 0 || len(evidence.SampledWindows) != 5 {
		t.Fatalf("sample provenance missing: %#v", evidence)
	}
}

func TestResolveSourceBitrateEvidencePrefersVideoStreamWithoutSampling(t *testing.T) {
	calls := 0
	probe := FFProbeResult{}
	probe.Format.FormatName = "matroska,webm"
	probe.Format.Duration = "120"
	probe.Format.Bitrate = "40000000"
	probe.Streams = []FFProbeStream{{Index: 2, CodecType: "video", Bitrate: "3500000"}, {Index: 3, CodecType: "audio", Bitrate: "12000000"}}
	evidence := resolveSourceBitrateEvidence(context.Background(), "/media/audio-heavy.mkv", 500_000_000, probe, func(context.Context, string, string, float64, int) []quality.SourceBitrateSampleWindow {
		calls++
		return nil
	})
	if calls != 0 || evidence.Method != quality.SourceBitrateMethodStreamMetadata || evidence.Bitrate != 3_500_000 || *evidence.StreamIndex != 2 {
		t.Fatalf("strong video stream evidence was not selected: calls=%d evidence=%#v", calls, evidence)
	}
}

func TestBuildScanResultPersistsSourceBitrateEvidence(t *testing.T) {
	evidence := quality.ResolveSourceBitrate(quality.SourceBitrateInput{Video: quality.SourceStreamFacts{Index: 1, Bitrate: 2_400_000}, DurationSeconds: 60})
	probe := FFProbeResult{}
	probe.Format.Duration = "60"
	probe.Format.Bitrate = "10000000"
	probe.Streams = []FFProbeStream{{Index: 1, CodecType: "video", CodecName: "hevc", Bitrate: "2400000"}}
	result := buildScanResult("/media/movie.mkv", 100_000_000, probe, models.JSONMap{"sourceBitrateEvidence": sourceBitrateEvidenceMap(evidence)})
	if result.SourceBitrateEvidence["method"] != quality.SourceBitrateMethodStreamMetadata || workerIntValue(result.SourceBitrateEvidence["bitrate"], 0) != 2_400_000 {
		t.Fatalf("source bitrate evidence was not persisted on ScanResult: %#v", result.SourceBitrateEvidence)
	}
}

func TestSourceBitrateVersionRefreshInvalidatesOnlyItsAnalysisComponent(t *testing.T) {
	mediaPath := t.TempDir() + "/movie.mkv"
	if err := os.WriteFile(mediaPath, []byte("media"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(mediaPath)
	if err != nil {
		t.Fatal(err)
	}
	scan := models.ScanResult{SourceBitrateEvidence: models.JSONMap{"schemaVersion": quality.SourceBitrateEvidenceSchemaVersion, "method": quality.SourceBitrateMethodUnavailable}}
	stampSnapshotCacheMetadataWithPolicy(&scan, mediaPath, info, balancedAnalysisPolicy())
	cache, _ := snapshotCacheMap(scan.RawProbe["snapshotCache"])
	components, _ := snapshotCacheMap(cache["components"])
	components["sourceBitrate"] = models.JSONMap{"version": 0, "status": "missing"}
	scan.RawProbe["snapshotCache"] = models.JSONMap(cache)

	fingerprintMatches, legacy, stale := snapshotCacheStateWithEvidence(scan, mediaPath, info, balancedAnalysisPolicy(), 20)
	if !fingerprintMatches || legacy || !reflect.DeepEqual(stale, []string{"sourceBitrate"}) || !incrementalRefreshSupported(stale) {
		t.Fatalf("source bitrate version invalidated unrelated analysis: fingerprint=%v legacy=%v stale=%v", fingerprintMatches, legacy, stale)
	}
	if snapshotComponentStatus(scan, "sourceBitrate") != "valid" {
		t.Fatalf("current evidence schema was not recognized: %#v", scan.SourceBitrateEvidence)
	}
}
