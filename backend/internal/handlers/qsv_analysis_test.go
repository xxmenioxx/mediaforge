package handlers

import (
	"math"
	"testing"
)

func TestQSVFrameStructureAssessment(t *testing.T) {
	tests := []struct {
		name       string
		analysis   QSVFrameStructureAnalysis
		assessment string
	}{
		{
			name: "no frames",
			analysis: QSVFrameStructureAnalysis{
				FramesAnalyzed: 0,
			},
			assessment: "No frames were available for analysis.",
		},
		{
			name: "no b frames",
			analysis: QSVFrameStructureAnalysis{
				FramesAnalyzed: 100,
				IFrames:        2,
				PFrames:        98,
			},
			assessment: "No B-frames were detected.",
		},
		{
			name: "limited b frame structure",
			analysis: QSVFrameStructureAnalysis{
				FramesAnalyzed:        100,
				IFrames:               2,
				PFrames:               48,
				BFrames:               50,
				HasBFrames:            true,
				MaxConsecutiveBFrames: 1,
			},
			assessment: "The stream uses a limited B-frame structure.",
		},
		{
			name: "strong b frame structure",
			analysis: QSVFrameStructureAnalysis{
				FramesAnalyzed:        100,
				IFrames:               2,
				PFrames:               23,
				BFrames:               75,
				HasBFrames:            true,
				MaxConsecutiveBFrames: 3,
			},
			assessment: "The stream uses B-frames with a balanced I/P/B structure.",
		},
		{
			name: "extreme b frame structure",
			analysis: QSVFrameStructureAnalysis{
				FramesAnalyzed:        500,
				IFrames:               2,
				BFrames:               498,
				HasBFrames:            true,
				BFrameRatio:           0.996,
				MaxConsecutiveBFrames: 247,
			},
			assessment: "Unusual frame structure: the sample is B-frame-dominant, has no detected P-frames, and contains very long B-frame runs.",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			analysis := test.analysis
			applyQSVFrameStructureAssessment(&analysis)

			if analysis.Assessment != test.assessment {
				t.Fatalf(
					"expected assessment %q, got %q",
					test.assessment,
					analysis.Assessment,
				)
			}
		})
	}
}

func TestQSVFrameStructureWarnings(t *testing.T) {
	tests := []struct {
		name     string
		features QSVFeatureStatus
		source   QSVFrameStructureAnalysis
		output   QSVFrameStructureAnalysis
		expected []string
	}{
		{
			name:     "unsupported requested features",
			features: QSVFeatureStatus{AdaptiveIRequested: true, AdaptiveBRequested: true},
			output:   QSVFrameStructureAnalysis{FramesAnalyzed: 100, PFrames: 99, KeyFrames: 2},
			expected: []string{
				"Adaptive I was requested but the active worker did not validate it for this QSV combination, so MVForge left it out of the command.",
				"Adaptive B was requested but the active worker did not validate it for this QSV combination, so MVForge left it out of the command.",
			},
		},
		{
			name: "extreme b frame structure is independent from switches",
			source: QSVFrameStructureAnalysis{
				FramesAnalyzed:        500,
				PFrames:               203,
				BFrameRatio:           0.58,
				MaxConsecutiveBFrames: 3,
			},
			output: QSVFrameStructureAnalysis{
				FramesAnalyzed:        500,
				BFrames:               477,
				HasBFrames:            true,
				BFrameRatio:           0.954,
				MaxConsecutiveBFrames: 247,
				KeyFrames:             2,
				AverageGOPLength:      248,
			},
			expected: []string{
				"B-frames make up 95.4% of the sample. This is unusually high and may indicate an unstable or unsuitable GOP structure.",
				"No P-frames were detected in the output sample. Review the GOP and B-frame behavior before using this profile for production.",
				"The output contains a run of 247 consecutive B-frames. Long B-frame runs can reduce compatibility and make seeking less predictable.",
				"The output frame structure changed substantially from the source: B-frame share 58.0% to 95.4%, longest B-frame run 3 to 247, and detected P-frames 203 to 0. Review compatibility and visual quality before adopting this configuration as a standard profile.",
				"The average GOP is 248 frames, which is long for many playback and seeking workflows.",
			},
		},
		{
			name: "sample too short for gop",
			output: QSVFrameStructureAnalysis{
				FramesAnalyzed: 100,
				PFrames:        99,
				KeyFrames:      1,
			},
			expected: []string{
				"Only one keyframe was found, so the average GOP length is not representative for this sample.",
			},
		},
		{
			name:     "qsv gpb does not trigger conventional b frame warnings",
			features: QSVFeatureStatus{GPBKnown: true, GPBEffective: true, GopRefDist: 1, BRefType: "off", InterpretationMode: "qsv_gpb"},
			source:   QSVFrameStructureAnalysis{FramesAnalyzed: 2400, PFrames: 2343, KeyFrames: 33, AverageGOPLength: 75.4},
			output:   QSVFrameStructureAnalysis{FramesAnalyzed: 2399, BFrames: 2364, BFrameRatio: 0.985, MaxConsecutiveBFrames: 74, KeyFrames: 35, AverageGOPLength: 75},
			expected: []string{},
		},
		{
			name:     "qsv mixed b gpb does not trigger conventional b frame warnings",
			features: QSVFeatureStatus{GPBKnown: true, GPBEffective: true, GopRefDist: 4, BRefType: "pyramid", InterpretationMode: "qsv_mixed_b_gpb"},
			source:   QSVFrameStructureAnalysis{FramesAnalyzed: 2400, PFrames: 2343, KeyFrames: 33, AverageGOPLength: 75.4},
			output:   QSVFrameStructureAnalysis{FramesAnalyzed: 2399, BFrames: 2364, BFrameRatio: 0.985, MaxConsecutiveBFrames: 74, KeyFrames: 35, AverageGOPLength: 75},
			expected: []string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			warnings := qsvFrameStructureWarnings(
				test.features,
				test.source,
				test.output,
			)

			if len(warnings) != len(test.expected) {
				t.Fatalf(
					"expected %d warnings, got %d: %#v",
					len(test.expected),
					len(warnings),
					warnings,
				)
			}

			for index := range test.expected {
				if warnings[index] != test.expected[index] {
					t.Fatalf(
						"expected warning %q, got %q",
						test.expected[index],
						warnings[index],
					)
				}
			}
		})
	}
}

func TestParseQSVEffectiveFrameContext(t *testing.T) {
	log := "[hevc_qsv @ 0x123] GopPicSize: 75; GopRefDist: 4; BRefType: pyramid; PRefType: simple; GPB: ON; RateControlMethod: ICQ; TargetUsage: 4; AdaptiveI: ON\n"
	context := parseQSVEffectiveFrameContext(log)
	if !context.GPBKnown || !context.GPBEffective || context.GopPicSize != 75 || context.GopRefDist != 4 || context.BRefType != "pyramid" || context.PRefType != "simple" || context.RateControlMethod != "icq" || context.TargetUsage != 4 {
		t.Fatalf("unexpected effective QSV context: %#v", context)
	}
	unknown := parseQSVEffectiveFrameContext("ordinary ffmpeg output\n")
	if unknown.GPBKnown || unknown.GPBEffective || unknown.GopRefDist != 0 || unknown.BRefType != "" {
		t.Fatalf("must not invent QSV context: %#v", unknown)
	}
}

func TestFrameStructureRecommendationAndValidation(t *testing.T) {
	source := QSVFrameStructureAnalysis{AverageGOPLength: 82.7, MaxConsecutiveBFrames: 3, BFrameRatio: .58, Confidence: "high"}
	recommendation := recommendFrameStructure(source, 29.97, "anime", "balanced", true, true, false)
	if recommendation.TargetGOPFrames != 105 || math.Abs(recommendation.TargetGOPSeconds-3.51) > .02 || recommendation.MaxBFrames != 3 || !recommendation.AdaptiveI || recommendation.AdaptiveB {
		t.Fatalf("unexpected recommendation: %#v", recommendation)
	}
	safe := QSVFrameStructureAnalysis{AverageGOPLength: 110, MaxConsecutiveBFrames: 3, PFrames: 200, BFrameRatio: .6, WindowCount: 5, Confidence: "high"}
	if got := validateFrameStructureRecommendation(recommendation, source, safe); got.Verdict != "safe" {
		t.Fatalf("safe verdict=%#v", got)
	}
	extreme := QSVFrameStructureAnalysis{AverageGOPLength: 248, MaxConsecutiveBFrames: 247, PFrames: 0, BFrameRatio: .996, WindowCount: 5, Confidence: "high"}
	if got := validateFrameStructureRecommendation(recommendation, source, extreme); got.Verdict != "reject" || got.Confidence != "high" {
		t.Fatalf("reject verdict=%#v", got)
	}
}

func TestAssetDerivedGOPRecommendationUsesTimeAndFPS(t *testing.T) {
	rayearth := QSVFrameStructureAnalysis{AverageGOPLength: 71.7, Confidence: "high"}
	compatible := recommendFrameStructure(rayearth, 23.976, "anime", "compatible", false, false, false)
	balanced := recommendFrameStructure(rayearth, 23.976, "anime", "balanced", false, false, false)
	maximum := recommendFrameStructure(rayearth, 23.976, "anime", "maximum_compression", false, false, false)
	if compatible.TargetGOPFrames != 72 || balanced.TargetGOPFrames != 90 || maximum.TargetGOPFrames != 120 {
		t.Fatalf("unexpected Rayearth GOP sequence: compatible=%d balanced=%d maximum=%d", compatible.TargetGOPFrames, balanced.TargetGOPFrames, maximum.TargetGOPFrames)
	}
	baccano := QSVFrameStructureAnalysis{AverageGOPLength: 156.4166666667, Confidence: "medium"}
	baccanoCompatible := recommendFrameStructure(baccano, 24000.0/1001.0, "anime", "compatible", false, false, false)
	baccanoBalanced := recommendFrameStructure(baccano, 24000.0/1001.0, "anime", "balanced", false, false, false)
	baccanoMaximum := recommendFrameStructure(baccano, 24000.0/1001.0, "anime", "maximum_compression", false, false, false)
	if baccanoCompatible.TargetGOPFrames != 72 || baccanoBalanced.TargetGOPFrames != 96 || baccanoMaximum.TargetGOPFrames != 132 {
		t.Fatalf("long source GOP modes must remain distinct: compatible=%d balanced=%d maximum=%d", baccanoCompatible.TargetGOPFrames, baccanoBalanced.TargetGOPFrames, baccanoMaximum.TargetGOPFrames)
	}
	arbegas := recommendFrameStructure(QSVFrameStructureAnalysis{AverageGOPLength: 75.4, Confidence: "high"}, 29.97, "anime", "balanced", false, false, false)
	if arbegas.TargetGOPFrames != 98 {
		t.Fatalf("Arbegas must derive frames from its own FPS/time baseline, got %d", arbegas.TargetGOPFrames)
	}
	sixty := recommendFrameStructure(QSVFrameStructureAnalysis{AverageGOPLength: 179.82, Confidence: "high"}, 59.94, "sports", "compatible", false, false, false)
	if sixty.TargetGOPFrames != 180 || math.Abs(sixty.TargetGOPSeconds-3) > .01 {
		t.Fatalf("59.94 fps three-second GOP must be about 180 frames: %#v", sixty)
	}
	unknown := recommendFrameStructure(rayearth, 0, "anime", "balanced", false, false, false)
	if unknown.TargetGOPFrames != 0 || unknown.Confidence != "low" || len(unknown.Warnings) == 0 {
		t.Fatalf("missing FPS must not create a falsely precise recommendation: %#v", unknown)
	}
}

func TestFrameStructureWindowAggregationHelpers(t *testing.T) {
	if got := mergedIntervalSeconds([][2]float64{{0, 20}, {10, 30}, {50, 70}}); got != 50 {
		t.Fatalf("coverage=%v", got)
	}
	if got := frameStructureVariability([]float64{90, 92, 88, 240, 91}); got != "high" {
		t.Fatalf("variability=%q", got)
	}
}
