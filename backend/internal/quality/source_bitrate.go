package quality

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	SourceBitrateEvidenceSchemaVersion = 1
	SourceBitrateSamplingPolicyVersion = 1
	MaxSourceBitrateSampleWindows      = 5
	SourceBitrateSampleWindowSeconds   = 0.5
	SourceBitrateSampleOutputLimit     = 32 * 1024
)

const (
	SourceBitrateMethodStreamMetadata     = "stream_metadata"
	SourceBitrateMethodMatroskaStatistics = "matroska_statistics"
	SourceBitrateMethodPacketSampling     = "packet_sampling"
	SourceBitrateMethodContainerBitrate   = "container_bitrate"
	SourceBitrateMethodFileSizeDuration   = "file_size_duration"
	SourceBitrateMethodUnavailable        = "unavailable"
)

type SourceStreamFacts struct {
	Index           int               `json:"index"`
	Bitrate         int64             `json:"bitrate,omitempty"`
	DurationSeconds float64           `json:"durationSeconds,omitempty"`
	Tags            map[string]string `json:"tags,omitempty"`
}

type SourceBitrateSampleWindow struct {
	StartSeconds    float64 `json:"startSeconds"`
	DurationSeconds float64 `json:"durationSeconds"`
	Bytes           int64   `json:"bytes,omitempty"`
	Status          string  `json:"status"`
	Reason          string  `json:"reason,omitempty"`
}

type SourceBitrateInput struct {
	Video            SourceStreamFacts
	Audio            []SourceStreamFacts
	Subtitles        []SourceStreamFacts
	ContainerBitrate int64
	FileSizeBytes    int64
	DurationSeconds  float64
	SampleWindows    []SourceBitrateSampleWindow
	RequestedWindows int
}

type SourceBitrateEvidence struct {
	SchemaVersion     int                         `json:"schemaVersion"`
	SamplingPolicy    int                         `json:"samplingPolicyVersion"`
	Bitrate           int64                       `json:"bitrate"`
	MinimumBitrate    *int64                      `json:"minimumBitrate,omitempty"`
	MaximumBitrate    *int64                      `json:"maximumBitrate,omitempty"`
	Method            string                      `json:"method"`
	Confidence        string                      `json:"confidence"`
	Detail            string                      `json:"detail,omitempty"`
	StreamIndex       *int                        `json:"streamIndex,omitempty"`
	DurationSeconds   float64                     `json:"durationSeconds,omitempty"`
	RequestedWindows  int                         `json:"requestedWindows"`
	SuccessfulWindows int                         `json:"successfulWindows"`
	SampledWindows    []SourceBitrateSampleWindow `json:"sampledWindows"`
}

type PacketSampleWindow struct {
	StartSeconds    float64
	DurationSeconds float64
}

type PacketSampleRunner func(context.Context, []string, int) ([]byte, error)

var errPacketOutputLimit = errors.New("packet sample output limit reached")

// ResolveSourceBitrate applies the one canonical source-video evidence order.
// It intentionally keeps video-only evidence ahead of all container estimates.
func ResolveSourceBitrate(input SourceBitrateInput) SourceBitrateEvidence {
	duration := input.DurationSeconds
	if input.Video.DurationSeconds > 0 {
		duration = input.Video.DurationSeconds
	}
	evidence := SourceBitrateEvidence{
		SchemaVersion:    SourceBitrateEvidenceSchemaVersion,
		SamplingPolicy:   SourceBitrateSamplingPolicyVersion,
		Method:           SourceBitrateMethodUnavailable,
		Confidence:       "unavailable",
		DurationSeconds:  duration,
		SampledWindows:   append([]SourceBitrateSampleWindow(nil), input.SampleWindows...),
		RequestedWindows: input.RequestedWindows,
		StreamIndex:      intPointer(input.Video.Index),
	}
	for _, window := range input.SampleWindows {
		if window.Status == "success" && window.Bytes > 0 && window.DurationSeconds > 0 {
			evidence.SuccessfulWindows++
		}
	}

	if input.Video.Bitrate > 0 {
		return sourceBitrateResult(evidence, input.Video.Bitrate, nil, nil, SourceBitrateMethodStreamMetadata, "high", "ffprobe video stream bit_rate")
	}
	if !staleMatroskaStatistics(input.Video.Tags) {
		if bitrate := tagInteger(input.Video.Tags, "BPS"); bitrate > 0 {
			return sourceBitrateResult(evidence, bitrate, nil, nil, SourceBitrateMethodMatroskaStatistics, "high", "Matroska per-stream BPS")
		}
		if bytes := tagInteger(input.Video.Tags, "NUMBER_OF_BYTES"); bytes > 0 && duration > 0 {
			if bitrate := bytesPerSecondBits(bytes, duration); bitrate > 0 {
				return sourceBitrateResult(evidence, bitrate, nil, nil, SourceBitrateMethodMatroskaStatistics, "high", "Matroska per-stream byte count divided by stream duration")
			}
		}
	}
	if bitrate, minimum, maximum, ok := packetSampleBitrate(input.SampleWindows); ok {
		confidence := "low"
		if evidence.SuccessfulWindows >= 3 {
			confidence = "medium"
		}
		return sourceBitrateResult(evidence, bitrate, &minimum, &maximum, SourceBitrateMethodPacketSampling, confidence, "distributed video-packet byte samples")
	}

	nonVideoBitrate := knownNonVideoBitrate(input.Audio) + knownNonVideoBitrate(input.Subtitles)
	if input.ContainerBitrate > nonVideoBitrate {
		return sourceBitrateResult(evidence, input.ContainerBitrate-nonVideoBitrate, nil, nil, SourceBitrateMethodContainerBitrate, "low", "container bitrate less known audio and subtitle bitrates")
	}
	if duration > 0 && input.FileSizeBytes > 0 {
		if bitrate := bytesPerSecondBits(input.FileSizeBytes, duration) - nonVideoBitrate; bitrate > 0 {
			return sourceBitrateResult(evidence, bitrate, nil, nil, SourceBitrateMethodFileSizeDuration, "low", "file size divided by duration less known audio and subtitle bitrates")
		}
	}
	return evidence
}

func NeedsSourcePacketSampling(input SourceBitrateInput) bool {
	if input.Video.Index < 0 || input.DurationSeconds <= 0 {
		return false
	}
	videoDuration := input.Video.DurationSeconds
	if videoDuration <= 0 {
		videoDuration = input.DurationSeconds
	}
	matroskaStatsAvailable := tagInteger(input.Video.Tags, "BPS") > 0 || tagInteger(input.Video.Tags, "NUMBER_OF_BYTES") > 0 && videoDuration > 0
	if input.Video.Bitrate > 0 || !staleMatroskaStatistics(input.Video.Tags) && matroskaStatsAvailable {
		return false
	}
	return true
}

func sourceBitrateResult(base SourceBitrateEvidence, bitrate int64, minimum, maximum *int64, method, confidence, detail string) SourceBitrateEvidence {
	base.Bitrate = bitrate
	base.MinimumBitrate = minimum
	base.MaximumBitrate = maximum
	base.Method = method
	base.Confidence = confidence
	base.Detail = detail
	return base
}

func packetSampleBitrate(windows []SourceBitrateSampleWindow) (int64, int64, int64, bool) {
	var totalBytes int64
	var totalSeconds float64
	rates := make([]int64, 0, len(windows))
	for _, window := range windows {
		if window.Status != "success" || window.Bytes <= 0 || window.DurationSeconds <= 0 {
			continue
		}
		rate := bytesPerSecondBits(window.Bytes, window.DurationSeconds)
		if rate <= 0 {
			continue
		}
		totalBytes += window.Bytes
		totalSeconds += window.DurationSeconds
		rates = append(rates, rate)
	}
	if totalSeconds <= 0 || len(rates) == 0 {
		return 0, 0, 0, false
	}
	bitrate := bytesPerSecondBits(totalBytes, totalSeconds)
	sort.Slice(rates, func(i, j int) bool { return rates[i] < rates[j] })
	return bitrate, rates[0], rates[len(rates)-1], bitrate > 0
}

func knownNonVideoBitrate(streams []SourceStreamFacts) int64 {
	var total int64
	for _, stream := range streams {
		if stream.Bitrate > 0 {
			total += stream.Bitrate
			continue
		}
		if staleMatroskaStatistics(stream.Tags) {
			continue
		}
		if bitrate := tagInteger(stream.Tags, "BPS"); bitrate > 0 {
			total += bitrate
			continue
		}
		duration := stream.DurationSeconds
		if bytes := tagInteger(stream.Tags, "NUMBER_OF_BYTES"); bytes > 0 && duration > 0 {
			total += bytesPerSecondBits(bytes, duration)
		}
	}
	return total
}

func staleMatroskaStatistics(tags map[string]string) bool {
	encoder := firstTagValue(tags, "ENCODER")
	writer := firstTagValue(tags, "_STATISTICS_WRITING_APP")
	return encoder != "" && writer != "" && !strings.Contains(strings.ToLower(writer), strings.ToLower(encoder))
}

func tagInteger(tags map[string]string, base string) int64 {
	for key, value := range tags {
		name := strings.ToUpper(strings.TrimSpace(key))
		if name == base || strings.HasPrefix(name, base+"-") {
			if parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && parsed > 0 {
				return parsed
			}
		}
	}
	return 0
}

func firstTagValue(tags map[string]string, base string) string {
	for key, value := range tags {
		name := strings.ToUpper(strings.TrimSpace(key))
		if name == base || strings.HasPrefix(name, base+"-") {
			if value = strings.TrimSpace(value); value != "" {
				return value
			}
		}
	}
	return ""
}

func bytesPerSecondBits(bytes int64, seconds float64) int64 {
	if bytes <= 0 || seconds <= 0 || float64(bytes) > float64(math.MaxInt64)/8 {
		return 0
	}
	return int64(math.Round(float64(bytes) * 8 / seconds))
}

func intPointer(value int) *int { return &value }

func CanSampleSourcePackets(formatName string) bool {
	for _, name := range strings.Split(strings.ToLower(formatName), ",") {
		switch strings.TrimSpace(name) {
		case "matroska", "webm", "mov", "mp4", "avi", "mpeg", "mpegts", "m4v":
			return true
		}
	}
	return false
}

// DistributedSourcePacketWindows returns deterministic interior sampling
// intervals. Short assets get one center window; long assets remain capped.
func DistributedSourcePacketWindows(durationSeconds float64) []PacketSampleWindow {
	if durationSeconds <= 0 || math.IsNaN(durationSeconds) || math.IsInf(durationSeconds, 0) {
		return nil
	}
	windowSeconds := math.Min(SourceBitrateSampleWindowSeconds, durationSeconds)
	count := int(math.Floor(durationSeconds / (windowSeconds * 1.5)))
	if count < 1 {
		count = 1
	}
	if count > MaxSourceBitrateSampleWindows {
		count = MaxSourceBitrateSampleWindows
	}
	windows := make([]PacketSampleWindow, 0, count)
	lastStart := -1.0
	for index := 0; index < count; index++ {
		center := durationSeconds * float64(index+1) / float64(count+1)
		start := math.Max(0, math.Min(durationSeconds-windowSeconds, center-windowSeconds/2))
		start = math.Round(start*1000) / 1000
		if start == lastStart {
			continue
		}
		windows = append(windows, PacketSampleWindow{StartSeconds: start, DurationSeconds: math.Round(windowSeconds*1000) / 1000})
		lastStart = start
	}
	return windows
}

// SampleSourceVideoPackets performs bounded ffprobe reads on the selected video
// stream. A failed interval is recorded and does not discard successful ones.
func SampleSourceVideoPackets(ctx context.Context, path, formatName string, durationSeconds float64, videoOrdinal int) []SourceBitrateSampleWindow {
	return SampleSourceVideoPacketsWithRunner(ctx, path, formatName, durationSeconds, videoOrdinal, runFFProbePacketWindow)
}

func SampleSourceVideoPacketsWithRunner(ctx context.Context, path, formatName string, durationSeconds float64, videoOrdinal int, runner PacketSampleRunner) []SourceBitrateSampleWindow {
	if !CanSampleSourcePackets(formatName) || strings.TrimSpace(path) == "" || videoOrdinal < 0 || runner == nil {
		return nil
	}
	windows := DistributedSourcePacketWindows(durationSeconds)
	results := make([]SourceBitrateSampleWindow, 0, len(windows))
	for _, window := range windows {
		if err := ctx.Err(); err != nil {
			results = append(results, SourceBitrateSampleWindow{StartSeconds: window.StartSeconds, DurationSeconds: window.DurationSeconds, Status: "cancelled", Reason: "analysis_cancelled"})
			continue
		}
		interval := fmt.Sprintf("%.3f%%+%.3f", window.StartSeconds, window.DurationSeconds)
		args := []string{
			"-v", "error", "-select_streams", fmt.Sprintf("v:%d", videoOrdinal),
			"-read_intervals", interval, "-show_packets", "-show_entries", "packet=size",
			"-of", "default=noprint_wrappers=1:nokey=1", path,
		}
		windowContext, cancel := context.WithTimeout(ctx, 2*time.Second)
		output, err := runner(windowContext, args, SourceBitrateSampleOutputLimit)
		cancel()
		sample := SourceBitrateSampleWindow{StartSeconds: window.StartSeconds, DurationSeconds: window.DurationSeconds}
		if err != nil {
			sample.Status, sample.Reason = packetSampleFailureStatus(err), packetSampleFailureReason(err)
			results = append(results, sample)
			continue
		}
		bytes, parseErr := parsePacketSizes(output)
		if parseErr != nil || bytes <= 0 {
			sample.Status, sample.Reason = "failed", "packet_sizes_unavailable"
			results = append(results, sample)
			continue
		}
		sample.Bytes, sample.Status = bytes, "success"
		results = append(results, sample)
	}
	return results
}

func packetSampleFailureStatus(err error) string {
	switch {
	case errors.Is(err, errPacketOutputLimit):
		return "failed"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	default:
		return "failed"
	}
}

func packetSampleFailureReason(err error) string {
	if errors.Is(err, errPacketOutputLimit) {
		return "packet_output_limit"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "window_timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "analysis_cancelled"
	}
	return "packet_probe_failed"
}

func parsePacketSizes(output []byte) (int64, error) {
	var total int64
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "N/A" {
			continue
		}
		value, err := strconv.ParseInt(line, 10, 64)
		if err != nil || value <= 0 || total > math.MaxInt64-value {
			return 0, fmt.Errorf("invalid video packet size")
		}
		total += value
	}
	if total <= 0 {
		return 0, fmt.Errorf("no video packet sizes")
	}
	return total, nil
}

func runFFProbePacketWindow(ctx context.Context, args []string, maxBytes int) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "ffprobe", args...)
	stdout := boundedPacketOutput{limit: maxBytes}
	stderr := boundedPacketOutput{limit: 4096}
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err != nil {
		if stdout.limited || stderr.limited {
			return nil, errPacketOutputLimit
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	if stdout.limited {
		return nil, errPacketOutputLimit
	}
	return stdout.bytes, nil
}

type boundedPacketOutput struct {
	limit   int
	bytes   []byte
	limited bool
}

func (buffer *boundedPacketOutput) Write(value []byte) (int, error) {
	remaining := buffer.limit - len(buffer.bytes)
	if remaining <= 0 || len(value) > remaining {
		if remaining > 0 {
			buffer.bytes = append(buffer.bytes, value[:remaining]...)
		}
		buffer.limited = true
		return max(remaining, 0), errPacketOutputLimit
	}
	buffer.bytes = append(buffer.bytes, value...)
	return len(value), nil
}
