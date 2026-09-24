package capabilities

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
)

type LogicalMetric string

const (
	LogicalMetricVMAF   LogicalMetric = "VMAF"
	LogicalMetricXPSNR  LogicalMetric = "XPSNR"
	LogicalMetricSSIM   LogicalMetric = "SSIM"
	LogicalMetricMSSSIM LogicalMetric = "MS-SSIM"
	LogicalMetricPSNR   LogicalMetric = "PSNR"
)

type MetricCapability struct {
	Metric                    LogicalMetric `json:"metric"`
	Provider                  string        `json:"provider,omitempty"`
	ProviderVersion           string        `json:"providerVersion,omitempty"`
	InvocationContract        string        `json:"invocationContract,omitempty"`
	ModelIdentity             string        `json:"modelIdentity,omitempty"`
	Availability              string        `json:"availability"`
	Reason                    string        `json:"reason,omitempty"`
	RuntimeCapabilityIdentity string        `json:"runtimeCapabilityIdentity"`
}

type MetricCapabilitySnapshot struct {
	SchemaVersion             string             `json:"schemaVersion"`
	FFmpegVersion             string             `json:"ffmpegVersion"`
	RuntimeCapabilityIdentity string             `json:"runtimeCapabilityIdentity"`
	Metrics                   []MetricCapability `json:"metrics"`
}

type MetricProcessRunner func(context.Context, []string) (string, error)

func defaultMetricProcessRunner(ctx context.Context, args []string) (string, error) {
	command := exec.CommandContext(ctx, "ffmpeg", args...)
	output, err := command.CombinedOutput()
	return string(output), err
}

var metricCapabilityCache = struct {
	sync.Mutex
	items map[string]MetricCapabilitySnapshot
}{items: map[string]MetricCapabilitySnapshot{}}

func ProbeMetricCapabilities(ctx context.Context) MetricCapabilitySnapshot {
	modelPath := strings.TrimSpace(os.Getenv("MVFORGE_VMAF_MODEL"))
	if modelPath == "" {
		modelPath = strings.TrimSpace(os.Getenv("MVFORGE_VMAF_MODEL_PATH"))
	}
	return ProbeMetricCapabilitiesWithRunner(ctx, defaultMetricProcessRunner, modelPath)
}

func ProbeMetricCapabilitiesWithRunner(ctx context.Context, runner MetricProcessRunner, modelPath string) MetricCapabilitySnapshot {
	versionOutput, versionErr := runner(ctx, []string{"-version"})
	filtersOutput, filtersErr := runner(ctx, []string{"-filters"})
	helpOutput := ""
	if strings.Contains(filtersOutput, "libvmaf") {
		helpOutput, _ = runner(ctx, []string{"-hide_banner", "-h", "filter=libvmaf"})
	}
	version := firstFFmpegVersionLine(versionOutput)
	modelIdentity := usableModelIdentity(modelPath)
	filterText := strings.ToLower(filtersOutput)
	capabilities := []MetricCapability{
		metricCapability(LogicalMetricSSIM, "ffmpeg.filter.ssim", filterPresent(filterText, "ssim"), version, "ffmpeg-filter-ssim-v1", "", "SSIM decoded-frame similarity"),
		metricCapability(LogicalMetricPSNR, "ffmpeg.filter.psnr", filterPresent(filterText, "psnr"), version, "ffmpeg-filter-psnr-v1", "", "PSNR decoded-frame error; +Inf is best"),
		metricCapability(LogicalMetricXPSNR, "ffmpeg.filter.xpsnr", filterPresent(filterText, "xpsnr"), version, "ffmpeg-filter-xpsnr-v1", "", "XPSNR decoded-frame quality"),
	}
	libvmafPresent := filterPresent(filterText, "libvmaf")
	capabilities = append(capabilities, metricCapability(LogicalMetricVMAF, "ffmpeg.filter.libvmaf", libvmafPresent && modelIdentity != "", version, "ffmpeg-filter-libvmaf-v1", modelIdentity, "VMAF model-based quality"))
	msSSIMProvider := libvmafPresent && (strings.Contains(strings.ToLower(helpOutput), "ms_ssim") || strings.Contains(strings.ToLower(helpOutput), "msssim"))
	capabilities = append(capabilities, metricCapability(LogicalMetricMSSSIM, "ffmpeg.filter.libvmaf:ms_ssim", msSSIMProvider, version, "ffmpeg-filter-libvmaf-ms-ssim-v1", modelIdentity, "MS-SSIM through a verified libvmaf provider"))
	for index := range capabilities {
		if versionErr != nil || filtersErr != nil {
			if capabilities[index].Availability == "available" {
				capabilities[index].Availability = "unavailable"
				capabilities[index].Reason = "FFmpeg runtime probe failed"
			}
		}
	}
	runtimeIdentity := metricCapabilityIdentity(version, capabilities)
	for index := range capabilities {
		capabilities[index].RuntimeCapabilityIdentity = runtimeIdentity
	}
	return MetricCapabilitySnapshot{SchemaVersion: "metric-capability-v1", FFmpegVersion: version, RuntimeCapabilityIdentity: runtimeIdentity, Metrics: capabilities}
}

func CachedMetricCapabilities(ctx context.Context) MetricCapabilitySnapshot {
	modelPath := strings.TrimSpace(os.Getenv("MVFORGE_VMAF_MODEL"))
	if modelPath == "" {
		modelPath = strings.TrimSpace(os.Getenv("MVFORGE_VMAF_MODEL_PATH"))
	}
	key := modelPath
	metricCapabilityCache.Lock()
	if snapshot, ok := metricCapabilityCache.items[key]; ok {
		metricCapabilityCache.Unlock()
		return snapshot
	}
	metricCapabilityCache.Unlock()
	snapshot := ProbeMetricCapabilities(ctx)
	metricCapabilityCache.Lock()
	metricCapabilityCache.items[key] = snapshot
	metricCapabilityCache.Unlock()
	return snapshot
}

func ResetMetricCapabilityCache() {
	metricCapabilityCache.Lock()
	defer metricCapabilityCache.Unlock()
	metricCapabilityCache.items = map[string]MetricCapabilitySnapshot{}
}

func metricCapability(metric LogicalMetric, provider string, available bool, providerVersion, invocation, model, semantics string) MetricCapability {
	capability := MetricCapability{Metric: metric, Provider: provider, ProviderVersion: providerVersion, InvocationContract: invocation, ModelIdentity: model, Availability: "unavailable", Reason: "verified provider is absent"}
	if available {
		capability.Availability = "available"
		capability.Reason = ""
	}
	if metric == LogicalMetricMSSSIM && !available {
		capability.Reason = "runtime has no verified MS-SSIM provider/invocation"
	}
	if metric == LogicalMetricVMAF && model == "" {
		capability.Reason = "usable libvmaf provider or model identity is absent"
	}
	_ = semantics
	return capability
}

func filterPresent(filters, name string) bool {
	for _, line := range strings.Split(filters, "\n") {
		fields := strings.Fields(line)
		for _, field := range fields {
			if strings.TrimSpace(field) == name {
				return true
			}
		}
	}
	return false
}

func firstFFmpegVersionLine(value string) string {
	for _, line := range strings.Split(value, "\n") {
		if strings.Contains(strings.ToLower(line), "ffmpeg version") {
			return strings.TrimSpace(line)
		}
	}
	return "unknown"
}

func usableModelIdentity(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		return ""
	}
	value := fmt.Sprintf("%s|%d|%d", path, info.Size(), info.ModTime().UnixNano())
	digest := sha256.Sum256([]byte(value))
	return "file:" + hex.EncodeToString(digest[:])
}

func metricCapabilityIdentity(version string, values []MetricCapability) string {
	canonical := append([]MetricCapability(nil), values...)
	sort.Slice(canonical, func(i, j int) bool { return canonical[i].Metric < canonical[j].Metric })
	payload, _ := json.Marshal(struct {
		Version string             `json:"version"`
		Metrics []MetricCapability `json:"metrics"`
	}{version, canonical})
	digest := sha256.Sum256(payload)
	return "metric-runtime:" + hex.EncodeToString(digest[:])
}
