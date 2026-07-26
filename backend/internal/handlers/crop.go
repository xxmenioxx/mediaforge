package handlers

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"time"
)

type CropAnalysis struct {
	Status          string    `json:"status"`
	Source          string    `json:"source"`
	Confidence      float64   `json:"confidence"`
	RecommendedCrop string    `json:"recommendedCrop,omitempty"`
	OriginalWidth   int       `json:"originalWidth"`
	OriginalHeight  int       `json:"originalHeight"`
	OutputWidth     int       `json:"outputWidth,omitempty"`
	OutputHeight    int       `json:"outputHeight,omitempty"`
	X               int       `json:"x,omitempty"`
	Y               int       `json:"y,omitempty"`
	Windows         int       `json:"windows"`
	MatchingWindows int       `json:"matchingWindows"`
	SampledAt       []float64 `json:"sampledAt"`
	Reason          string    `json:"reason"`
}

type cropCandidate struct {
	width  int
	height int
	x      int
	y      int
}

var cropDetectPattern = regexp.MustCompile(`crop=(\d+):(\d+):(\d+):(\d+)`)

func detectCrop(path string, width, height int, duration float64) CropAnalysis {
	analysis := CropAnalysis{
		Status: "unknown", Source: "cropdetect", OriginalWidth: width, OriginalHeight: height,
		Reason: "Crop detection did not return enough stable samples.",
	}
	if width <= 0 || height <= 0 {
		return analysis
	}
	starts := cropSampleStarts(duration, 3)
	analysis.Windows = len(starts)
	analysis.SampledAt = starts
	candidates := make([]cropCandidate, 0, len(starts))
	for _, start := range starts {
		if candidate, ok := cropCandidateAt(path, start); ok {
			candidates = append(candidates, candidate)
		}
	}
	return classifyCropCandidates(analysis, candidates)
}

func cropSampleStarts(duration float64, windowSeconds float64) []float64 {
	if duration <= windowSeconds*2 {
		return []float64{0}
	}
	maxStart := max(0, duration-windowSeconds)
	return []float64{
		min(maxStart, max(0, duration*0.10)),
		min(maxStart, max(0, duration*0.50-windowSeconds/2)),
		min(maxStart, max(0, duration*0.90-windowSeconds)),
	}
}

func cropCandidateAt(path string, start float64) (cropCandidate, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner", "-ss", fmt.Sprintf("%.3f", start), "-i", path,
		"-map", "0:v:0", "-t", "3", "-vf", "fps=3,cropdetect=limit=24:round=2:reset=0",
		"-an", "-f", "null", "-",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	_ = cmd.Run()
	matches := cropDetectPattern.FindAllStringSubmatch(stderr.String(), -1)
	if len(matches) == 0 {
		return cropCandidate{}, false
	}
	counts := map[cropCandidate]int{}
	for _, match := range matches {
		candidate := cropCandidate{width: atoi(match[1]), height: atoi(match[2]), x: atoi(match[3]), y: atoi(match[4])}
		counts[candidate]++
	}
	type ranked struct {
		candidate cropCandidate
		count     int
	}
	ranking := make([]ranked, 0, len(counts))
	for candidate, count := range counts {
		ranking = append(ranking, ranked{candidate: candidate, count: count})
	}
	sort.SliceStable(ranking, func(i, j int) bool { return ranking[i].count > ranking[j].count })
	return ranking[0].candidate, true
}

func classifyCropCandidates(analysis CropAnalysis, candidates []cropCandidate) CropAnalysis {
	if len(candidates) == 0 {
		return analysis
	}
	counts := map[cropCandidate]int{}
	for _, candidate := range candidates {
		counts[candidate]++
	}
	var best cropCandidate
	bestCount := 0
	for candidate, count := range counts {
		if count > bestCount {
			best, bestCount = candidate, count
		}
	}
	analysis.MatchingWindows = bestCount
	analysis.Confidence = float64(bestCount) / float64(max(analysis.Windows, 1))
	removedX := max(0, analysis.OriginalWidth-best.width)
	removedY := max(0, analysis.OriginalHeight-best.height)
	minHorizontal := max(8, analysis.OriginalWidth/100)
	minVertical := max(8, analysis.OriginalHeight/100)
	if removedX < minHorizontal && removedY < minVertical {
		analysis.Status = "none"
		analysis.Confidence = float64(len(candidates)) / float64(max(analysis.Windows, 1))
		analysis.Reason = "No stable black bars large enough to justify cropping were detected."
		return analysis
	}
	requiredMatches := 1
	if analysis.Windows > 1 {
		requiredMatches = 2
	}
	if bestCount < requiredMatches || analysis.Confidence < 0.66 {
		analysis.Status = "variable"
		analysis.Reason = "Black borders vary between sampled scenes; review manually before cropping."
		return analysis
	}
	analysis.Status = "detected"
	analysis.OutputWidth, analysis.OutputHeight, analysis.X, analysis.Y = best.width, best.height, best.x, best.y
	analysis.RecommendedCrop = strconv.Itoa(best.width) + ":" + strconv.Itoa(best.height) + ":" + strconv.Itoa(best.x) + ":" + strconv.Itoa(best.y)
	switch {
	case removedX >= minHorizontal && removedY >= minVertical:
		analysis.Reason = "Stable black bars were detected on the sides and top/bottom."
	case removedX >= minHorizontal:
		analysis.Reason = "Stable black bars were detected on the left and right sides."
	default:
		analysis.Reason = "Stable black bars were detected on the top and bottom."
	}
	return analysis
}
