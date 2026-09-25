package capabilities

import (
	"context"
	"strings"
	"testing"
)

func TestProbeMetricCapabilitiesResolvesLogicalProviders(t *testing.T) {
	calls := 0
	runner := func(_ context.Context, args []string) (string, error) {
		calls++
		switch {
		case len(args) == 1 && args[0] == "-version":
			return "ffmpeg version 7.0-test\n", nil
		case len(args) == 1 && args[0] == "-filters":
			return " ... psnr\n ... ssim\n ... xpsnr\n ... libvmaf\n", nil
		case strings.Contains(strings.Join(args, " "), "filter=libvmaf"):
			return "libvmaf options: ms_ssim=boolean model=string\n", nil
		default:
			return "", nil
		}
	}
	snapshot := ProbeMetricCapabilitiesWithRunner(context.Background(), runner, "")
	if snapshot.RuntimeCapabilityIdentity == "" || calls != 3 {
		t.Fatalf("expected one version/filter/help probe, got identity %q and %d calls", snapshot.RuntimeCapabilityIdentity, calls)
	}
	for _, capability := range snapshot.Metrics {
		if capability.Metric == LogicalMetricMSSSIM {
			if capability.Availability != "available" || !strings.Contains(capability.Provider, "libvmaf") {
				t.Fatalf("MS-SSIM provider was not resolved: %#v", capability)
			}
		}
	}
}

func TestProbeMetricCapabilitiesMissingOptionalProvidersIsSafe(t *testing.T) {
	runner := func(_ context.Context, args []string) (string, error) {
		if args[0] == "-version" {
			return "ffmpeg version 7.0-test\n", nil
		}
		return " ... ssim\n ... psnr\n", nil
	}
	snapshot := ProbeMetricCapabilitiesWithRunner(context.Background(), runner, "")
	for _, capability := range snapshot.Metrics {
		switch capability.Metric {
		case LogicalMetricSSIM, LogicalMetricPSNR:
			if capability.Availability != "available" {
				t.Fatalf("base metric unexpectedly unavailable: %#v", capability)
			}
		case LogicalMetricVMAF, LogicalMetricXPSNR, LogicalMetricMSSSIM:
			if capability.Availability == "available" {
				t.Fatalf("optional metric falsely available: %#v", capability)
			}
		}
	}
}
