package handlers

import (
	"context"
	"math"
	"strings"
	"testing"
)

func TestSubtitleSyncProposalShapeAcceptsBoundedEngineNeutralModels(t *testing.T) {
	endOfFirst := int64(10_000)
	startOfSecond, endOfSecond := int64(10_000), int64(20_000)
	startOfThird := int64(20_000)
	manualFixed := validSubtitleSyncModelFixture(subtitleSyncModelFixedOffset)
	automaticFixed := validSubtitleSyncModelFixture(subtitleSyncModelFixedOffset)
	automaticFixed.Engine = subtitleSyncAutoEngine
	automaticFixed.EngineVersion = subtitleSyncAutoEngineVersion
	automaticFixed.QualityState = "automatic_review"
	affine := validSubtitleSyncModelFixture(subtitleSyncModelAffine)
	affine.Scale = 1.05
	segmented := validSubtitleSyncModelFixture(subtitleSyncModelSegmented)
	segmented.Segments = []SubtitleSyncSegment{
		{EndMilliseconds: &endOfFirst, OffsetMilliseconds: 100, Scale: 1},
		{StartMilliseconds: &startOfSecond, EndMilliseconds: &endOfSecond, OffsetMilliseconds: 200, Scale: 1.01},
		{StartMilliseconds: &startOfThird, OffsetMilliseconds: 250, Scale: 1},
	}

	for name, proposal := range map[string]SubtitleSyncProposal{
		"manual fixed offset":                          manualFixed,
		"automatic fixed offset shares model identity": automaticFixed,
		"affine":    affine,
		"segmented": segmented,
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateSubtitleSyncProposalShape(proposal); err != nil {
				t.Fatalf("expected bounded %s model shape to pass: %v", name, err)
			}
		})
	}
	if manualFixed.Model != automaticFixed.Model || manualFixed.Engine == automaticFixed.Engine || manualFixed.EngineVersion == automaticFixed.EngineVersion {
		t.Fatalf("model identity must remain engine-neutral while engine identity remains separate: manual=%#v automatic=%#v", manualFixed, automaticFixed)
	}
}

func TestSubtitleSyncProposalShapeRejectsInvalidOrUnboundedModels(t *testing.T) {
	validSegmented := validSubtitleSyncModelFixture(subtitleSyncModelSegmented)
	start, end := int64(1_000), int64(2_000)
	validSegmented.Segments = []SubtitleSyncSegment{{StartMilliseconds: &start, EndMilliseconds: &end, Scale: 1}}
	overlapping := validSegmented
	secondStart, secondEnd := int64(1_500), int64(2_500)
	overlapping.Segments = append(append([]SubtitleSyncSegment(nil), validSegmented.Segments...), SubtitleSyncSegment{StartMilliseconds: &secondStart, EndMilliseconds: &secondEnd, Scale: 1})
	tooManySegments := validSegmented
	tooManySegments.Segments = make([]SubtitleSyncSegment, maxSubtitleSyncSegments+1)
	for index := range tooManySegments.Segments {
		segmentStart := int64(index * 2)
		segmentEnd := segmentStart + 1
		tooManySegments.Segments[index] = SubtitleSyncSegment{StartMilliseconds: &segmentStart, EndMilliseconds: &segmentEnd, Scale: 1}
	}
	fixedWithSegments := validSubtitleSyncModelFixture(subtitleSyncModelFixedOffset)
	fixedWithSegments.Segments = validSegmented.Segments
	affineWithSegments := validSubtitleSyncModelFixture(subtitleSyncModelAffine)
	affineWithSegments.Segments = validSegmented.Segments
	unknownModel := validSubtitleSyncModelFixture("future-model")
	nanScale := validSubtitleSyncModelFixture(subtitleSyncModelAffine)
	nanScale.Scale = math.NaN()
	invalidConfidence := validSubtitleSyncModelFixture(subtitleSyncModelFixedOffset)
	invalidConfidence.Confidence = math.Inf(1)
	tooManyDiagnostics := validSubtitleSyncModelFixture(subtitleSyncModelFixedOffset)
	tooManyDiagnostics.Diagnostics = make([]string, maxSubtitleSyncDiagnosticCount+1)
	tooLongDiagnostic := validSubtitleSyncModelFixture(subtitleSyncModelFixedOffset)
	tooLongDiagnostic.Diagnostics = []string{strings.Repeat("x", maxSubtitleSyncDiagnosticBytes+1)}
	tooManyPreviewCues := validSubtitleSyncModelFixture(subtitleSyncModelFixedOffset)
	tooManyPreviewCues.Preview = make([]SubtitleSyncCuePreview, maxSubtitleSyncPreview+1)

	invalid := map[string]SubtitleSyncProposal{
		"fixed model with segments":  fixedWithSegments,
		"affine model with segments": affineWithSegments,
		"unknown model":              unknownModel,
		"non-finite scale":           nanScale,
		"non-finite confidence":      invalidConfidence,
		"overlapping segments":       overlapping,
		"too many segments":          tooManySegments,
		"too many diagnostics":       tooManyDiagnostics,
		"oversized diagnostic":       tooLongDiagnostic,
		"too many preview cues":      tooManyPreviewCues,
	}
	for name, proposal := range invalid {
		t.Run(name, func(t *testing.T) {
			if err := validateSubtitleSyncProposalShape(proposal); err == nil {
				t.Fatalf("expected malformed or unbounded %s proposal to fail", name)
			}
		})
	}
}

func TestFixedOffsetEnginesDoNotExecuteAffineOrSegmentedShapes(t *testing.T) {
	start, end := int64(1_000), int64(5_000)
	proposals := []SubtitleSyncProposal{
		validSubtitleSyncModelFixture(subtitleSyncModelAffine),
		{
			Engine: "future-segmented-engine", EngineVersion: "1", Model: subtitleSyncModelSegmented,
			Scale: 1, QualityState: "review", Segments: []SubtitleSyncSegment{{StartMilliseconds: &start, EndMilliseconds: &end, OffsetMilliseconds: 250, Scale: 1}},
		},
	}
	for _, proposal := range proposals {
		if err := validateSubtitleSyncProposalShape(proposal); err != nil {
			t.Fatalf("expected valid non-executable shape to pass structural validation: %v", err)
		}
		proposal.ProposalID = subtitleSyncProposalID(proposal)
		for _, engine := range []SubtitleSyncEngine{fixedOffsetSubtitleSyncEngine{}, automaticSubtitleSyncEngine{}} {
			if _, err := engine.Apply(context.Background(), []byte("1\n00:00:01,000 --> 00:00:02,000\ncue\n"), proposal); err == nil {
				t.Fatalf("%s engine unexpectedly executed %q model", proposal.Engine, proposal.Model)
			}
		}
	}
}

func validSubtitleSyncModelFixture(model string) SubtitleSyncProposal {
	return SubtitleSyncProposal{
		SourceIdentity: "external:/media/episode.srt", SourceRevision: "revision-1", Representation: "srt",
		ReferenceAudio: SubtitleSyncAudioReference{StreamIndex: 2, StreamID: "0x2", Codec: "aac"},
		Engine:         "test-engine", EngineVersion: "test-v1", Model: model,
		OffsetMilliseconds: 100, Scale: 1, Segments: []SubtitleSyncSegment{}, QualityState: "review",
		Diagnostics: []string{}, Preview: []SubtitleSyncCuePreview{},
	}
}
