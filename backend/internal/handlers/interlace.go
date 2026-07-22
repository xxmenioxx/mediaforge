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
	Status            string  `json:"status"`
	FieldOrder        string  `json:"fieldOrder"`
	Source            string  `json:"source"`
	Confidence        float64 `json:"confidence"`
	TFF               int     `json:"tff"`
	BFF               int     `json:"bff"`
	Progressive       int     `json:"progressive"`
	Undetermined      int     `json:"undetermined"`
	RepeatedTop       int     `json:"repeatedTop"`
	RepeatedBottom    int     `json:"repeatedBottom"`
	SampledFrames     int     `json:"sampledFrames"`
	WindowStart       float64 `json:"windowStart"`
	WindowSeconds     int     `json:"windowSeconds"`
	RecommendedFilter string  `json:"recommendedFilter,omitempty"`
}

var (
	idetMultiPattern    = regexp.MustCompile(`Multi frame detection:\s*TFF:\s*(\d+)\s*BFF:\s*(\d+)\s*Progressive:\s*(\d+)\s*Undetermined:\s*(\d+)`)
	idetRepeatedPattern = regexp.MustCompile(`Repeated Fields:\s*Neither:\s*(\d+)\s*Top:\s*(\d+)\s*Bottom:\s*(\d+)`)
)

func detectInterlace(path, fieldOrder string, duration float64, windowSeconds int) InterlaceAnalysis {
	windowSeconds = normalizedAnalysisSeconds(windowSeconds)
	analysis := InterlaceAnalysis{Status: interlaceStatusFromFieldOrder(fieldOrder), FieldOrder: normalizeFieldOrder(fieldOrder), Source: "ffprobe"}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(windowSeconds+30)*time.Second)
	defer cancel()

	start := 0.0
	if duration > float64(windowSeconds) {
		start = max(0, duration/2-float64(windowSeconds)/2)
	}
	analysis.WindowStart = start
	analysis.WindowSeconds = windowSeconds
	args := []string{"-hide_banner", "-ss", fmt.Sprintf("%.3f", start), "-i", path, "-map", "0:v:0", "-t", strconv.Itoa(windowSeconds), "-vf", "idet", "-an", "-f", "null", "-"}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil && ctx.Err() != nil {
		return analysis
	}

	multiMatches := idetMultiPattern.FindAllStringSubmatch(stderr.String(), -1)
	if len(multiMatches) == 0 {
		return analysis
	}
	multi := multiMatches[len(multiMatches)-1]
	if len(multi) != 5 {
		return analysis
	}
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
	analysis.Source = "idet"
	classifyInterlace(&analysis)
	return analysis
}

func classifyInterlace(analysis *InterlaceAnalysis) {
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
		analysis.RecommendedFilter = "bwdif=mode=send_frame:parity=auto:deint=all"
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
	case "ivtc", "ivtc_bff", "inverse_telecine":
		// IVTC is carried in videoFilters so it can preserve the selected
		// field order and must not be prefixed with ordinary deinterlacing.
		return ""
	case "force", "forced", "on":
		return "bwdif=mode=send_frame:parity=auto:deint=all"
	default:
		if analysis.Status == "interlaced" {
			return "bwdif=mode=send_frame:parity=auto:deint=all"
		}
		return ""
	}
}
