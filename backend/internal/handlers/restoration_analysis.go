package handlers

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const restorationAnalysisVersion = 1

type RestorationSignalEvidence struct {
	Availability       string   `json:"availability"`
	Severity           string   `json:"severity"`
	Value              *float64 `json:"value,omitempty"`
	Confidence         string   `json:"confidence"`
	SupportingEvidence []string `json:"supportingEvidence"`
}

type RestorationAnalysis struct {
	Version       int                       `json:"version"`
	Status        string                    `json:"status"`
	Source        string                    `json:"source"`
	Windows       int                       `json:"windows"`
	SampledFrames int                       `json:"sampledFrames"`
	Blocking      RestorationSignalEvidence `json:"blocking"`
	Noise         RestorationSignalEvidence `json:"noise"`
	Grain         RestorationSignalEvidence `json:"grain"`
	ChromaNoise   RestorationSignalEvidence `json:"chromaNoise"`
	Banding       RestorationSignalEvidence `json:"banding"`
	Ringing       RestorationSignalEvidence `json:"ringing"`
	EdgeDetail    RestorationSignalEvidence `json:"edgeDetailConfidence"`
}

func normalizeRestorationAnalysis(analysis *RestorationAnalysis) {
	if analysis == nil {
		return
	}
	for _, signal := range []*RestorationSignalEvidence{
		&analysis.Blocking,
		&analysis.Noise,
		&analysis.Grain,
		&analysis.ChromaNoise,
		&analysis.Banding,
		&analysis.Ringing,
		&analysis.EdgeDetail,
	} {
		if signal.SupportingEvidence == nil {
			signal.SupportingEvidence = []string{}
		}
	}
}

type restorationWindowEvidence struct {
	blocking []float64
	luma     []float64
	chromaU  []float64
	chromaV  []float64
}

var restorationMetricPattern = regexp.MustCompile(`lavfi\.(block|bitplanenoise\.[012]\.1)=([-+0-9.eE]+)`)

func parseRestorationWindowEvidence(output string) restorationWindowEvidence {
	result := restorationWindowEvidence{}
	for _, match := range restorationMetricPattern.FindAllStringSubmatch(output, -1) {
		value, err := strconv.ParseFloat(match[2], 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		switch match[1] {
		case "block":
			result.blocking = append(result.blocking, value)
		case "bitplanenoise.0.1":
			result.luma = append(result.luma, value)
		case "bitplanenoise.1.1":
			result.chromaU = append(result.chromaU, value)
		case "bitplanenoise.2.1":
			result.chromaV = append(result.chromaV, value)
		}
	}
	return result
}

func (e restorationWindowEvidence) available() bool {
	return len(e.blocking)+len(e.luma)+len(e.chromaU)+len(e.chromaV) > 0
}

func classifyRestorationEvidence(windows []restorationWindowEvidence) RestorationAnalysis {
	analysis := RestorationAnalysis{Version: restorationAnalysisVersion, Status: "unavailable", Source: "sampled_ffmpeg_metrics"}
	blocking, luma, chroma := []float64{}, []float64{}, []float64{}
	for _, window := range windows {
		if !window.available() {
			continue
		}
		analysis.Windows++
		blocking = append(blocking, window.blocking...)
		luma = append(luma, window.luma...)
		chroma = append(chroma, window.chromaU...)
		chroma = append(chroma, window.chromaV...)
		analysis.SampledFrames += max(max(len(window.blocking), len(window.luma)), max(len(window.chromaU), len(window.chromaV)))
	}
	analysis.Blocking = unavailableRestorationSignal("FFmpeg blockdetect did not produce sampled source evidence.")
	if len(blocking) > 0 {
		analysis.Status = "available"
		analysis.Blocking = measuredRestorationSignal(blocking, analysis.Windows, "lavfi.block", "unclassified", "The raw blockdetect score is preserved without inventing severity thresholds.")
	}
	analysis.Noise = unavailableRestorationSignal("No reliable sampled luma-noise evidence was produced.")
	analysis.Grain = unavailableRestorationSignal("No reliable sampled luma texture evidence was produced.")
	if len(luma) > 0 {
		analysis.Status = "available"
		analysis.Noise = ambiguousRestorationSignal(luma, analysis.Windows, "lavfi.bitplanenoise.0.1", "Bit-plane activity cannot reliably distinguish random noise from intentional grain or fine detail.")
		analysis.Grain = ambiguousRestorationSignal(luma, analysis.Windows, "lavfi.bitplanenoise.0.1", "Grain remains ambiguous because the sampled bit-plane metric also responds to noise and fine detail.")
	}
	analysis.ChromaNoise = unavailableRestorationSignal("No reliable sampled chroma-noise evidence was produced.")
	if len(chroma) > 0 {
		analysis.Status = "available"
		analysis.ChromaNoise = ambiguousRestorationSignal(chroma, analysis.Windows, "lavfi.bitplanenoise.1/2.1", "Chroma bit-plane activity is real evidence but does not provide a validated severity threshold.")
	}
	analysis.Banding = unavailableRestorationSignal("No canonical banding metric exists; black bars and flat artwork must not be treated as banding evidence.")
	analysis.Ringing = unavailableRestorationSignal("No canonical ringing metric is currently reliable enough for source evidence.")
	analysis.EdgeDetail = unavailableRestorationSignal("Blur or edge-filter output is not equivalent to reliable edge/detail confidence.")
	return analysis
}

func measuredRestorationSignal(values []float64, windows int, metric, severity, note string) RestorationSignalEvidence {
	mean, minimum, maximum := metricSummary(values)
	return RestorationSignalEvidence{
		Availability: "available", Severity: severity, Value: &mean, Confidence: sampledMetricConfidence(windows, len(values)),
		SupportingEvidence: []string{fmt.Sprintf("%s mean=%.6f min=%.6f max=%.6f across %d samples", metric, mean, minimum, maximum, len(values)), note},
	}
}

func ambiguousRestorationSignal(values []float64, windows int, metric, note string) RestorationSignalEvidence {
	evidence := measuredRestorationSignal(values, windows, metric, "unknown", note)
	evidence.Availability = "ambiguous"
	evidence.Confidence = "low"
	return evidence
}

func unavailableRestorationSignal(reason string) RestorationSignalEvidence {
	return RestorationSignalEvidence{Availability: "unavailable", Severity: "unknown", Confidence: "unavailable", SupportingEvidence: []string{reason}}
}

func metricSummary(values []float64) (mean, minimum, maximum float64) {
	if len(values) == 0 {
		return 0, 0, 0
	}
	minimum, maximum = values[0], values[0]
	for _, value := range values {
		mean += value
		minimum = math.Min(minimum, value)
		maximum = math.Max(maximum, value)
	}
	return mean / float64(len(values)), minimum, maximum
}

func sampledMetricConfidence(windows, samples int) string {
	if windows >= 3 && samples >= 6 {
		return "high"
	}
	if windows >= 2 && samples >= 2 {
		return "medium"
	}
	return "low"
}

func analyzeRestorationSignalsWithSamplingPlanContext(ctx context.Context, path string, plan SamplingPlan) RestorationAnalysis {
	windows := representativeSamplingWindows(plan, 3, min(3, len(plan.Positions)))
	evidence := make([]restorationWindowEvidence, 0, len(windows))
	for _, window := range windows {
		evidence = append(evidence, runRestorationMetricProbeContext(ctx, path, window.StartSeconds, min(3, int(math.Ceil(window.DurationSeconds)))))
		if ctx.Err() != nil {
			break
		}
	}
	return classifyRestorationEvidence(evidence)
}

func runRestorationMetricProbeContext(parent context.Context, path string, start float64, seconds int) restorationWindowEvidence {
	ctx, cancel := context.WithTimeout(parent, time.Duration(max(1, seconds)+20)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-ss", fmt.Sprintf("%.3f", start), "-i", path,
		"-map", "0:v:0", "-t", strconv.Itoa(max(1, seconds)), "-vf", restorationMetricFilterChain(),
		"-an", "-sn", "-f", "null", "-",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	_ = cmd.Run()
	return parseRestorationWindowEvidence(stderr.String())
}

func restorationMetricFilterChain() string {
	return "fps=1,blockdetect,bitplanenoise=bitplane=1,metadata=mode=print"
}

func restorationMetricFilterUnavailable(output string) bool {
	normalized := strings.ToLower(output)
	return strings.Contains(normalized, "no such filter") || strings.Contains(normalized, "error initializing complex filters") || strings.Contains(normalized, "option not found")
}

func restorationEvidenceUnavailable() RestorationAnalysis {
	return classifyRestorationEvidence(nil)
}

func restorationEvidenceFromRaw(value any) RestorationAnalysis {
	var analysis RestorationAnalysis
	if !decodeJSONValue(value, &analysis) || analysis.Version != restorationAnalysisVersion {
		return restorationEvidenceUnavailable()
	}
	normalizeRestorationAnalysis(&analysis)
	return analysis
}

func restorationEvidenceStatus(analysis RestorationAnalysis) string {
	return strings.ToLower(strings.TrimSpace(analysis.Status))
}
