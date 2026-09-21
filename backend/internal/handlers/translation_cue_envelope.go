package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

const translationCueEnvelopeVersion = "translation-cue-envelope-v1"

// translationCueEnvelope is an operation-local view of a canonical subtitle
// cue. MVForge-owned timing, representation, provenance, and geometry stay
// here; only StableID and Text cross the TranslationProvider boundary.
type translationCueEnvelope struct {
	StableID       string
	SourceOrdinal  int
	Text           string
	Timing         translationCueTimingReference
	Representation translationCueRepresentationReference
	Geometry       *bitmapCueGeometry
}

// translationSourceCue is the smallest MVForge-owned source view needed to
// build a translation envelope. Text parsers and the bitmap adapter can feed
// this without sharing a persisted translation model or exposing structure to
// the provider.
type translationSourceCue struct {
	SourceOrdinal    int
	Text             string
	Start            float64
	End              float64
	TimingProvenance subtitleTimingProvenance
	Geometry         *bitmapCueGeometry
}

type translationCueTimingReference struct {
	Start            float64
	End              float64
	TimingProvenance subtitleTimingProvenance
}

type translationCueRepresentationReference struct {
	SourceIdentity       string
	SourceRepresentation string
	LogicalTransformKey  string
}

func translationCueEnvelopesForSourceCues(sourceIdentity, sourceRepresentation, logicalTransformKey string, cues []translationSourceCue) []translationCueEnvelope {
	result := make([]translationCueEnvelope, 0, len(cues))
	for _, cue := range cues {
		result = append(result, translationCueEnvelope{
			StableID:      translationCueStableID(sourceIdentity, sourceRepresentation, logicalTransformKey, cue),
			SourceOrdinal: cue.SourceOrdinal,
			Text:          cue.Text,
			Timing: translationCueTimingReference{
				Start:            cue.Start,
				End:              cue.End,
				TimingProvenance: cue.TimingProvenance,
			},
			Representation: translationCueRepresentationReference{
				SourceIdentity:       sourceIdentity,
				SourceRepresentation: sourceRepresentation,
				LogicalTransformKey:  logicalTransformKey,
			},
			Geometry: cue.Geometry,
		})
	}
	return result
}

func translationCueEnvelopesForBitmapCues(sourceIdentity, sourceRepresentation, logicalTransformKey string, cues []canonicalBitmapSubtitleCue) []translationCueEnvelope {
	sourceCues := make([]translationSourceCue, 0, len(cues))
	for _, cue := range cues {
		sourceCues = append(sourceCues, translationSourceCue{
			SourceOrdinal:    cue.SourceOrdinal,
			Text:             cue.Text,
			Start:            cue.Start,
			End:              cue.End,
			TimingProvenance: cue.TimingProvenance,
			Geometry:         cue.Geometry,
		})
	}
	return translationCueEnvelopesForSourceCues(sourceIdentity, sourceRepresentation, logicalTransformKey, sourceCues)
}

func translationBatchForCueEnvelopes(sourceLanguage, targetLocale string, envelopes []translationCueEnvelope, context *TranslationContext) TranslationBatch {
	cues := make([]TranslationCue, 0, len(envelopes))
	for _, envelope := range envelopes {
		cues = append(cues, TranslationCue{ID: envelope.StableID, Text: envelope.Text})
	}
	return TranslationBatch{
		ContractVersion: translationProviderContractVersion,
		SourceLanguage:  sourceLanguage,
		TargetLocale:    targetLocale,
		Cues:            cues,
		Context:         context,
	}
}

func translationCueStableID(sourceIdentity, sourceRepresentation, logicalTransformKey string, cue translationSourceCue) string {
	parts := []string{
		translationCueEnvelopeVersion,
		sourceIdentity,
		sourceRepresentation,
		logicalTransformKey,
		strconv.Itoa(cue.SourceOrdinal),
		strconv.FormatFloat(cue.Start, 'g', -1, 64),
		strconv.FormatFloat(cue.End, 'g', -1, 64),
		string(cue.TimingProvenance),
	}
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "cue-" + hex.EncodeToString(hash[:])
}
