package handlers

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type InterlaceAnalysis struct {
	Version                      int             `json:"version"`
	Status                       string          `json:"status"`
	FieldOrder                   string          `json:"fieldOrder"`
	ContainerFieldOrder          string          `json:"containerFieldOrder"`
	DetectedFieldOrder           string          `json:"detectedFieldOrder,omitempty"`
	FieldOrderMismatch           bool            `json:"fieldOrderMismatch"`
	Source                       string          `json:"source"`
	Confidence                   float64         `json:"confidence"`
	TFF                          int             `json:"tff"`
	BFF                          int             `json:"bff"`
	Progressive                  int             `json:"progressive"`
	Undetermined                 int             `json:"undetermined"`
	RepeatedTop                  int             `json:"repeatedTop"`
	RepeatedBottom               int             `json:"repeatedBottom"`
	SampledFrames                int             `json:"sampledFrames"`
	WindowStart                  float64         `json:"windowStart"`
	WindowSeconds                int             `json:"windowSeconds"`
	SampleCount                  int             `json:"sampleCount"`
	SampledAt                    []float64       `json:"sampledAt,omitempty"`
	RecommendedMode              string          `json:"recommendedMode,omitempty"`
	RecommendedFieldMetadataMode string          `json:"recommendedFieldMetadataMode,omitempty"`
	RecommendedFilter            string          `json:"recommendedFilter,omitempty"`
	IVTCValidation               *IVTCValidation `json:"ivtcValidation,omitempty"`
}

const interlaceAnalysisVersion = 2

type IVTCValidation struct {
	TFFProgressive      int     `json:"tffProgressive"`
	TFFClassified       int     `json:"tffClassified"`
	TFFProgressiveRatio float64 `json:"tffProgressiveRatio"`
	BFFProgressive      int     `json:"bffProgressive"`
	BFFClassified       int     `json:"bffClassified"`
	BFFProgressiveRatio float64 `json:"bffProgressiveRatio"`
	SelectedOrder       string  `json:"selectedOrder,omitempty"`
	Confidence          float64 `json:"confidence"`
}

var (
	idetMultiPattern    = regexp.MustCompile(`Multi frame detection:\s*TFF:\s*(\d+)\s*BFF:\s*(\d+)\s*Progressive:\s*(\d+)\s*Undetermined:\s*(\d+)`)
	idetRepeatedPattern = regexp.MustCompile(`Repeated Fields:\s*Neither:\s*(\d+)\s*Top:\s*(\d+)\s*Bottom:\s*(\d+)`)
)

func detectInterlace(path, fieldOrder string, duration float64, windowSeconds int) InterlaceAnalysis {
	return detectInterlaceContext(context.Background(), path, fieldOrder, duration, windowSeconds)
}

func detectInterlaceContext(ctx context.Context, path, fieldOrder string, duration float64, windowSeconds int) InterlaceAnalysis {
	windowSeconds = normalizedAnalysisSeconds(windowSeconds)
	containerOrder := normalizeFieldOrder(fieldOrder)
	analysis := InterlaceAnalysis{
		Version: interlaceAnalysisVersion,
		Status:  interlaceStatusFromFieldOrder(fieldOrder), FieldOrder: containerOrder,
		ContainerFieldOrder: containerOrder, Source: "ffprobe", WindowSeconds: windowSeconds,
	}
	for _, start := range distributedInterlaceStarts(duration, windowSeconds) {
		sample, ok := runIDETContext(ctx, path, start, windowSeconds, "idet")
		if !ok {
			continue
		}
		analysis.TFF += sample.TFF
		analysis.BFF += sample.BFF
		analysis.Progressive += sample.Progressive
		analysis.Undetermined += sample.Undetermined
		analysis.RepeatedTop += sample.RepeatedTop
		analysis.RepeatedBottom += sample.RepeatedBottom
		analysis.SampledFrames += sample.SampledFrames
		analysis.SampledAt = append(analysis.SampledAt, start)
	}
	analysis.SampleCount = len(analysis.SampledAt)
	if analysis.SampleCount == 0 {
		return analysis
	}
	analysis.WindowStart = analysis.SampledAt[0]
	analysis.Source = "idet_multi_sample"
	analysis.DetectedFieldOrder = dominantFieldOrder(analysis.TFF, analysis.BFF)
	if analysis.DetectedFieldOrder != "" {
		analysis.FieldOrder = analysis.DetectedFieldOrder
		analysis.FieldOrderMismatch = fieldOrderFamily(containerOrder) != "" &&
			fieldOrderFamily(containerOrder) != analysis.DetectedFieldOrder
	}
	classifyInterlace(&analysis)
	finalizeFieldMetadataAnalysis(&analysis)
	if shouldValidateIVTC(analysis) {
		validateIVTCContext(ctx, path, duration, windowSeconds, &analysis)
	}
	return analysis
}

func finalizeFieldMetadataAnalysis(analysis *InterlaceAnalysis) {
	containerOrder := normalizeFieldOrder(analysis.ContainerFieldOrder)
	if analysis.Status == "progressive" {
		analysis.DetectedFieldOrder = "progressive"
		analysis.FieldOrder = "progressive"
		analysis.FieldOrderMismatch = fieldOrderFamily(containerOrder) != ""
		if analysis.FieldOrderMismatch {
			analysis.RecommendedFieldMetadataMode = "progressive"
		}
	}
	if analysis.Status == "interlaced" && analysis.RecommendedMode == "force" {
		// The recommended bwdif pass emits progressive frames.
		analysis.RecommendedFieldMetadataMode = "progressive"
	}
}

func distributedInterlaceStarts(duration float64, windowSeconds int) []float64 {
	if duration <= float64(windowSeconds) || duration <= 0 {
		return []float64{0}
	}
	maxStart := max(0, duration-float64(windowSeconds))
	starts := []float64{}
	seen := map[int]bool{}
	for _, position := range []float64{0.05, 0.25, 0.50, 0.75, 0.90} {
		start := min(maxStart, max(0, duration*position-float64(windowSeconds)/2))
		key := int(start * 1000)
		if !seen[key] {
			seen[key] = true
			starts = append(starts, start)
		}
	}
	return starts
}

func runIDET(path string, start float64, seconds int, filter string) (InterlaceAnalysis, bool) {
	return runIDETContext(context.Background(), path, start, seconds, filter)
}

func runIDETContext(parent context.Context, path string, start float64, seconds int, filter string) (InterlaceAnalysis, bool) {
	ctx, cancel := context.WithTimeout(parent, time.Duration(seconds+30)*time.Second)
	defer cancel()
	args := []string{"-hide_banner", "-ss", fmt.Sprintf("%.3f", start), "-i", path, "-map", "0:v:0", "-t", strconv.Itoa(seconds), "-vf", filter, "-an", "-sn", "-f", "null", "-"}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil && ctx.Err() != nil {
		return InterlaceAnalysis{}, false
	}

	multiMatches := idetMultiPattern.FindAllStringSubmatch(stderr.String(), -1)
	if len(multiMatches) == 0 {
		return InterlaceAnalysis{}, false
	}
	multi := multiMatches[len(multiMatches)-1]
	if len(multi) != 5 {
		return InterlaceAnalysis{}, false
	}
	analysis := InterlaceAnalysis{}
	analysis.TFF = atoi(multi[1])
	analysis.BFF = atoi(multi[2])
	analysis.Progressive = atoi(multi[3])
	analysis.Undetermined = atoi(multi[4])
	analysis.SampledFrames = analysis.TFF + analysis.BFF + analysis.Progressive + analysis.Undetermined
	repeatedMatches := idetRepeatedPattern.FindAllStringSubmatch(stderr.String(), -1)
	if len(repeatedMatches) > 0 {
		repeated := repeatedMatches[len(repeatedMatches)-1]
		analysis.RepeatedTop = atoi(repeated[2])
		analysis.RepeatedBottom = atoi(repeated[3])
	}
	return analysis, true
}

func dominantFieldOrder(tff, bff int) string {
	total := tff + bff
	if total == 0 {
		return ""
	}
	if float64(tff)/float64(total) >= 0.65 {
		return "tff"
	}
	if float64(bff)/float64(total) >= 0.65 {
		return "bff"
	}
	return ""
}

func fieldOrderFamily(value string) string {
	switch normalizeFieldOrder(value) {
	case "tt", "tb", "tff":
		return "tff"
	case "bb", "bt", "bff":
		return "bff"
	default:
		return ""
	}
}

func shouldValidateIVTC(analysis InterlaceAnalysis) bool {
	classified := analysis.TFF + analysis.BFF + analysis.Progressive
	return classified > 0 && (analysis.Status == "telecine_suspected" ||
		float64(analysis.TFF+analysis.BFF)/float64(classified) >= 0.50)
}

func validateIVTC(path string, duration float64, seconds int, analysis *InterlaceAnalysis) {
	validateIVTCContext(context.Background(), path, duration, seconds, analysis)
}

func validateIVTCContext(ctx context.Context, path string, duration float64, seconds int, analysis *InterlaceAnalysis) {
	start := max(0, duration/2-float64(seconds)/2)
	tff, tffOK := runIDETContext(ctx, path, start, seconds, "fieldmatch=order=tff,idet")
	bff, bffOK := runIDETContext(ctx, path, start, seconds, "fieldmatch=order=bff,idet")
	if !tffOK || !bffOK {
		return
	}
	tffClassified := tff.TFF + tff.BFF + tff.Progressive
	bffClassified := bff.TFF + bff.BFF + bff.Progressive
	validation := &IVTCValidation{
		TFFProgressive: tff.Progressive, TFFClassified: tffClassified,
		BFFProgressive: bff.Progressive, BFFClassified: bffClassified,
	}
	if tffClassified > 0 {
		validation.TFFProgressiveRatio = float64(tff.Progressive) / float64(tffClassified)
	}
	if bffClassified > 0 {
		validation.BFFProgressiveRatio = float64(bff.Progressive) / float64(bffClassified)
	}
	applyIVTCValidation(analysis, validation)
	analysis.IVTCValidation = validation
}

func applyIVTCValidation(analysis *InterlaceAnalysis, validation *IVTCValidation) {
	selected, best, other := "tff", validation.TFFProgressiveRatio, validation.BFFProgressiveRatio
	if validation.BFFProgressiveRatio > validation.TFFProgressiveRatio {
		selected, best, other = "bff", validation.BFFProgressiveRatio, validation.TFFProgressiveRatio
	}
	validation.Confidence = best
	if best >= 0.85 && best-other >= 0.15 {
		validation.SelectedOrder = selected
		analysis.Status = "telecine_suspected"
		analysis.Confidence = best
		analysis.DetectedFieldOrder = selected
		analysis.FieldOrder = selected
		analysis.FieldOrderMismatch = fieldOrderFamily(analysis.ContainerFieldOrder) != "" &&
			fieldOrderFamily(analysis.ContainerFieldOrder) != selected
		analysis.RecommendedMode = "ivtc_" + selected
		analysis.RecommendedFieldMetadataMode = "progressive"
		analysis.RecommendedFilter = "fieldmatch=order=" + selected + ",decimate"
	}
}

func classifyInterlace(analysis *InterlaceAnalysis) {
	analysis.Version = interlaceAnalysisVersion
	if analysis.DetectedFieldOrder == "" {
		analysis.DetectedFieldOrder = dominantFieldOrder(analysis.TFF, analysis.BFF)
	}
	classified := analysis.TFF + analysis.BFF + analysis.Progressive
	if classified == 0 {
		return
	}
	interlaced := analysis.TFF + analysis.BFF
	interlacedRatio := float64(interlaced) / float64(classified)
	repeatedRatio := float64(analysis.RepeatedTop+analysis.RepeatedBottom) / float64(max(analysis.SampledFrames, 1))
	switch {
	case repeatedRatio >= 0.15 && interlacedRatio >= 0.20:
		analysis.Status = "telecine_suspected"
		analysis.Confidence = repeatedRatio
	case interlacedRatio >= 0.70:
		analysis.Status = "interlaced"
		analysis.Confidence = interlacedRatio
		analysis.RecommendedMode = "force"
		analysis.RecommendedFilter = bwdifFilter(*analysis)
	case interlacedRatio >= 0.10:
		analysis.Status = "mixed"
		analysis.Confidence = max(interlacedRatio, 1-interlacedRatio)
	case interlacedRatio < 0.10:
		analysis.Status = "progressive"
		analysis.Confidence = 1 - interlacedRatio
	}
}

func interlaceStatusFromFieldOrder(value string) string {
	switch normalizeFieldOrder(value) {
	case "tt", "bb", "tb", "bt":
		return "interlaced"
	case "progressive":
		return "progressive"
	default:
		return "unknown"
	}
}

func normalizeFieldOrder(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "unknown"
	}
	return value
}

func atoi(value string) int {
	parsed, _ := strconv.Atoi(value)
	return parsed
}

func effectiveDeinterlaceFilter(profileMode string, analysis InterlaceAnalysis) string {
	switch strings.ToLower(strings.TrimSpace(profileMode)) {
	case "off", "disabled", "none":
		return ""
	case "ivtc", "ivtc_tff", "ivtc_bff", "inverse_telecine":
		// IVTC is carried in videoFilters so it can preserve the selected
		// field order and must not be prefixed with ordinary deinterlacing.
		return ""
	case "force", "forced", "on":
		return bwdifFilter(analysis)
	default:
		if analysis.Status == "telecine_suspected" && strings.TrimSpace(analysis.RecommendedFilter) != "" {
			return strings.TrimSpace(analysis.RecommendedFilter)
		}
		if analysis.Status == "interlaced" || analysis.Status == "telecine_suspected" {
			return bwdifFilter(analysis)
		}
		return ""
	}
}

func bwdifFilter(analysis InterlaceAnalysis) string {
	parity := analysis.DetectedFieldOrder
	if parity != "tff" && parity != "bff" {
		// Older snapshots did not persist detectedFieldOrder even when IDET's
		// frame counts provided decisive evidence. Never discard that evidence
		// and fall back to possibly incorrect container metadata.
		parity = dominantFieldOrder(analysis.TFF, analysis.BFF)
	}
	if parity != "tff" && parity != "bff" {
		parity = "auto"
	}
	return "bwdif=mode=send_frame:parity=" + parity + ":deint=all"
}
