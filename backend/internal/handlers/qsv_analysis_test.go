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
			assessment: "The stream uses a strong B-frame structure.",
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
		name               string
		adaptiveBRequested bool
		output             QSVFrameStructureAnalysis
		expected           []string
	}{
		{
			name:               "adaptive b without b frames",
			adaptiveBRequested: true,
			output: QSVFrameStructureAnalysis{
				FramesAnalyzed: 100,
				KeyFrames:      2,
			},
			expected: []string{
				"Adaptive B was requested, but no B-frames were detected in the encoded preview.",
			},
		},
		{
			name:               "adaptive b with limited structure",
			adaptiveBRequested: true,
			output: QSVFrameStructureAnalysis{
				FramesAnalyzed:        100,
				BFrames:               30,
				HasBFrames:            true,
				MaxConsecutiveBFrames: 1,
				KeyFrames:             2,
			},
			expected: []string{
				"Adaptive B produced only a limited B-frame structure in this preview sample.",
			},
		},
		{
			name:               "strong b frame structure",
			adaptiveBRequested: true,
			output: QSVFrameStructureAnalysis{
				FramesAnalyzed:        100,
				BFrames:               70,
				HasBFrames:            true,
				MaxConsecutiveBFrames: 3,
				KeyFrames:             2,
			},
			expected: []string{
				"The encoded preview produced a strong B-frame structure.",
			},
		},
		{
			name:               "sample too short for gop",
			adaptiveBRequested: false,
			output: QSVFrameStructureAnalysis{
				FramesAnalyzed: 100,
				KeyFrames:      1,
			},
			expected: []string{
				"The preview sample is too short to calculate a representative GOP average.",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			warnings := qsvFrameStructureWarnings(
				test.adaptiveBRequested,
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
