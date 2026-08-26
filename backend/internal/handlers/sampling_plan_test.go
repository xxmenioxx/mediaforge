package handlers

import "testing"

func TestCanonicalSamplingPlanSharesPositionsAcrossAnalyzers(t *testing.T) {
	plan := canonicalSamplingPlan(6600, defaultFrameStructureSamplingPolicy())
	frameWindows := plan.windows(plan.WindowSeconds)
	interlaceWindows := plan.windows(10)
	cropWindows := representativeSamplingWindows(plan, 3, 3)

	if len(frameWindows) != 5 || len(interlaceWindows) != 5 || len(cropWindows) != 3 {
		t.Fatalf("unexpected window counts: frame=%d interlace=%d crop=%d", len(frameWindows), len(interlaceWindows), len(cropWindows))
	}
	for index := range frameWindows {
		if frameWindows[index].Position != interlaceWindows[index].Position {
			t.Fatalf("position %d differs: frame=%v interlace=%v", index, frameWindows[index].Position, interlaceWindows[index].Position)
		}
	}
	if cropWindows[0].Position != frameWindows[0].Position || cropWindows[1].Position != frameWindows[2].Position || cropWindows[2].Position != frameWindows[4].Position {
		t.Fatalf("crop did not select representative canonical windows: %#v", cropWindows)
	}
}

func TestCanonicalSamplingPlanAdaptsShortAssetOnce(t *testing.T) {
	plan := canonicalSamplingPlan(45, defaultFrameStructureSamplingPolicy())
	if len(plan.Positions) != 1 || plan.Positions[0] != 0.5 || plan.WindowSeconds != 45 {
		t.Fatalf("unexpected short-asset plan: %#v", plan)
	}
	window := plan.windows(plan.WindowSeconds)
	if len(window) != 1 || window[0].StartSeconds != 0 || window[0].DurationSeconds != 45 {
		t.Fatalf("unexpected short-asset window: %#v", window)
	}
}

func TestCanonicalSamplingPlanClampsUnsupportedWindowCount(t *testing.T) {
	plan := canonicalSamplingPlan(600, FrameStructureSamplingPolicy{Windows: 9, WindowSeconds: 20, Positions: []float64{0.25}, Adaptive: false})
	if len(plan.Positions) != len(defaultFrameStructureSamplingPolicy().Positions) {
		t.Fatalf("window count was not clamped to available positions: %#v", plan)
	}
}
