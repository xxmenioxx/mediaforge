package handlers

import "math"

type SamplingPlan struct {
	AssetDurationSeconds     float64
	WindowSeconds            float64
	Positions                []float64
	Adaptive                 bool
	InitialWindows           int
	EarlyConfidenceEnabled   bool
	EarlyConfidenceThreshold float64
	InterlaceValidation      string
	CropDepth                string
}

type SamplingWindow struct {
	Position        float64
	StartSeconds    float64
	DurationSeconds float64
}

func canonicalSamplingPlan(duration float64, policy FrameStructureSamplingPolicy) SamplingPlan {
	if policy.Windows <= 0 {
		policy = defaultFrameStructureSamplingPolicy()
	}
	windowSeconds := policy.WindowSeconds
	positions := policy.Positions
	if len(positions) < policy.Windows {
		positions = defaultFrameStructureSamplingPolicy().Positions
	}
	if policy.Windows > len(positions) {
		policy.Windows = len(positions)
	}
	if policy.InitialWindows <= 0 {
		policy.InitialWindows = min(3, policy.Windows)
	}
	if policy.EarlyConfidenceThreshold <= 0 {
		policy.EarlyConfidenceThreshold = 0.98
	}
	if policy.InterlaceValidation == "" {
		policy.InterlaceValidation = "automatic"
	}
	if policy.CropDepth == "" {
		policy.CropDepth = "normal"
	}
	if policy.Adaptive {
		switch {
		case duration > 0 && duration <= 60:
			windowSeconds = duration
			policy.Windows = 1
			positions = []float64{0.5}
		case duration < 120:
			windowSeconds = 10
		case duration < 600:
			windowSeconds = 15
		default:
			windowSeconds = 20
		}
	}
	if windowSeconds <= 0 {
		windowSeconds = 20
	}
	return SamplingPlan{
		AssetDurationSeconds:     duration,
		WindowSeconds:            windowSeconds,
		Positions:                append([]float64(nil), positions[:policy.Windows]...),
		Adaptive:                 policy.Adaptive,
		InitialWindows:           max(1, min(policy.InitialWindows, policy.Windows)),
		EarlyConfidenceEnabled:   policy.EarlyConfidenceEnabled,
		EarlyConfidenceThreshold: policy.EarlyConfidenceThreshold,
		InterlaceValidation:      policy.InterlaceValidation,
		CropDepth:                policy.CropDepth,
	}
}

func (plan SamplingPlan) cropWindowMaximum() int {
	switch plan.CropDepth {
	case "reduced":
		return 2
	case "full":
		return len(plan.Positions)
	default:
		return 3
	}
}

func (plan SamplingPlan) windows(windowSeconds float64) []SamplingWindow {
	if len(plan.Positions) == 0 {
		return nil
	}
	if windowSeconds <= 0 {
		windowSeconds = plan.WindowSeconds
	}
	windows := make([]SamplingWindow, 0, len(plan.Positions))
	seen := map[int]bool{}
	for _, position := range plan.Positions {
		start := 0.0
		actualDuration := windowSeconds
		if plan.AssetDurationSeconds > windowSeconds*2 {
			start = math.Max(0, math.Min(plan.AssetDurationSeconds-windowSeconds, plan.AssetDurationSeconds*position-windowSeconds/2))
		} else if plan.AssetDurationSeconds > 0 {
			actualDuration = math.Min(windowSeconds, plan.AssetDurationSeconds)
		}
		key := int(math.Round(start * 1000))
		if seen[key] {
			continue
		}
		seen[key] = true
		windows = append(windows, SamplingWindow{Position: position, StartSeconds: start, DurationSeconds: actualDuration})
	}
	return windows
}

func representativeSamplingWindows(plan SamplingPlan, windowSeconds float64, maximum int) []SamplingWindow {
	windows := plan.windows(windowSeconds)
	if maximum <= 0 || len(windows) <= maximum {
		return windows
	}
	if maximum == 1 {
		return []SamplingWindow{windows[len(windows)/2]}
	}
	selected := make([]SamplingWindow, 0, maximum)
	for index := 0; index < maximum; index++ {
		windowIndex := int(math.Round(float64(index) * float64(len(windows)-1) / float64(maximum-1)))
		selected = append(selected, windows[windowIndex])
	}
	return selected
}
