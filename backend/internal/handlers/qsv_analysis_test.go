package handlers

import "testing"

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
