package handlers

import "testing"

func TestTranslationCueEnvelopeHasDeterministicIdentityAndPreservesReferences(t *testing.T) {
	geometry := &bitmapCueGeometry{
		Start: 1, End: 2, CanvasWidth: 1920, CanvasHeight: 1080,
		BitmapX: 600, BitmapY: 80, BitmapWidth: 720, BitmapHeight: 120,
		SourceOrdinal: 4, TimingProvenance: preparedInputLocal,
	}
	cue := canonicalBitmapSubtitleCue{
		Start: 1, End: 2, Text: "Original", SourceOrdinal: 4,
		TimingProvenance: preparedInputLocal, Geometry: geometry,
	}
	first := translationCueEnvelopesForBitmapCues("asset/source.mkv#stream=4", "pgs", "transform-1", []canonicalBitmapSubtitleCue{cue})
	changedText := cue
	changedText.Text = "Translated or corrected later"
	second := translationCueEnvelopesForBitmapCues("asset/source.mkv#stream=4", "pgs", "transform-1", []canonicalBitmapSubtitleCue{changedText})

	if len(first) != 1 || len(second) != 1 || first[0].StableID != second[0].StableID {
		t.Fatalf("cue identity changed with text: first=%#v second=%#v", first, second)
	}
	if first[0].SourceOrdinal != 4 || first[0].Timing.Start != 1 || first[0].Timing.End != 2 || first[0].Timing.TimingProvenance != preparedInputLocal {
		t.Fatalf("canonical timing/reference data was not preserved: %#v", first[0])
	}
	if first[0].Geometry != geometry {
		t.Fatal("bitmap geometry was copied instead of referenced")
	}
	if first[0].Representation.SourceIdentity != "asset/source.mkv#stream=4" || first[0].Representation.SourceRepresentation != "pgs" || first[0].Representation.LogicalTransformKey != "transform-1" {
		t.Fatalf("representation reference was not preserved: %#v", first[0].Representation)
	}
}

func TestTranslationCueEnvelopeDoesNotMutateCanonicalCuesAndProviderPayloadIsTextOnly(t *testing.T) {
	geometry := &bitmapCueGeometry{SourceOrdinal: 2, BitmapX: 10, BitmapY: 20}
	cues := []canonicalBitmapSubtitleCue{{Start: 3, End: 4, Text: "Hello", SourceOrdinal: 2, Geometry: geometry}}
	before := cues[0]
	envelopes := translationCueEnvelopesForBitmapCues("asset.mkv#stream=2", "srt", "", cues)
	batch := translationBatchForCueEnvelopes("eng", "es-419", envelopes, nil)

	if cues[0].Text != before.Text || cues[0].Start != before.Start || cues[0].End != before.End || cues[0].Geometry != before.Geometry {
		t.Fatalf("building envelope mutated canonical cue: before=%#v after=%#v", before, cues[0])
	}
	if len(batch.Cues) != 1 || batch.Cues[0].ID != envelopes[0].StableID || batch.Cues[0].Text != "Hello" || batch.Context != nil {
		t.Fatalf("unexpected provider payload: %#v", batch)
	}
	if err := validateTranslationBatch(batch, defaultTranslationSettings()); err != nil {
		t.Fatalf("provider payload rejected: %v", err)
	}
}

func TestTranslationCueStableIDChangesWithSourceCueIdentityNotTranslatedText(t *testing.T) {
	base := translationSourceCue{Start: 1, End: 2, Text: "same", SourceOrdinal: 1}
	otherOrdinal := base
	otherOrdinal.SourceOrdinal = 2
	otherTiming := base
	otherTiming.Start = 1.5
	baseID := translationCueEnvelopesForSourceCues("source", "srt", "", []translationSourceCue{base})[0].StableID
	if translationCueEnvelopesForSourceCues("source", "srt", "", []translationSourceCue{otherOrdinal})[0].StableID == baseID {
		t.Fatal("source ordinal did not affect stable identity")
	}
	if translationCueEnvelopesForSourceCues("source", "srt", "", []translationSourceCue{otherTiming})[0].StableID == baseID {
		t.Fatal("timing did not affect stable identity")
	}
}

func TestTranslationCueEnvelopeSupportsCanonicalTextRepresentations(t *testing.T) {
	for _, representation := range []string{"srt", "ass", "ssa"} {
		t.Run(representation, func(t *testing.T) {
			cue := translationSourceCue{
				Start: 5, End: 6, Text: "Source text", SourceOrdinal: 7,
				TimingProvenance: preparedInputLocal,
			}
			first := translationCueEnvelopesForSourceCues("asset.mkv#stream=7", representation, "", []translationSourceCue{cue})
			changed := cue
			changed.Text = "Translated text"
			second := translationCueEnvelopesForSourceCues("asset.mkv#stream=7", representation, "", []translationSourceCue{changed})
			if len(first) != 1 || len(second) != 1 || first[0].StableID != second[0].StableID {
				t.Fatalf("text changed stable ID for %s: first=%#v second=%#v", representation, first, second)
			}
			if first[0].SourceOrdinal != cue.SourceOrdinal || first[0].Timing.Start != cue.Start || first[0].Timing.End != cue.End {
				t.Fatalf("canonical references were not preserved for %s: %#v", representation, first[0])
			}
			batch := translationBatchForCueEnvelopes("eng", "es-419", first, nil)
			if len(batch.Cues) != 1 || batch.Cues[0].ID != first[0].StableID || batch.Cues[0].Text != cue.Text {
				t.Fatalf("unexpected provider payload for %s: %#v", representation, batch)
			}
			if err := validateTranslationBatch(batch, defaultTranslationSettings()); err != nil {
				t.Fatalf("provider payload rejected for %s: %v", representation, err)
			}
		})
	}
}

func TestTranslationCueIdentityDoesNotDependOnOutputSerializerChoice(t *testing.T) {
	cue := translationSourceCue{Start: 1, End: 2, Text: "Source", SourceOrdinal: 3}
	// Serializer selection is intentionally absent from the identity inputs.
	envelopes := translationCueEnvelopesForSourceCues("asset.mkv#stream=3", "ass", "transform", []translationSourceCue{cue})
	srtBatch := translationBatchForCueEnvelopes("eng", "es-419", envelopes, nil)
	assBatch := translationBatchForCueEnvelopes("eng", "es-419", envelopes, nil)
	if len(srtBatch.Cues) != 1 || len(assBatch.Cues) != 1 || srtBatch.Cues[0].ID != assBatch.Cues[0].ID {
		t.Fatalf("serializer choice changed source cue identity: srt=%#v ass=%#v", srtBatch, assBatch)
	}
}
