package handlers

import (
	"fmt"
	"reflect"
	"strings"
)

const (
	translationRepresentationSRT    = "srt"
	translationRepresentationASS    = "ass"
	translationRepresentationSSA    = "ssa"
	translationRepresentationBitmap = "bitmap"
)

func resolveTranslationOutputRepresentation(sourceRepresentation, requestedRepresentation string) (string, error) {
	source := strings.ToLower(strings.TrimSpace(sourceRepresentation))
	requested := strings.ToLower(strings.TrimSpace(requestedRepresentation))
	switch source {
	case translationRepresentationSRT, translationRepresentationASS, translationRepresentationSSA:
		if requested == "" {
			return source, nil
		}
		if requested != source {
			return "", fmt.Errorf("translation output %s cannot change source representation %s", requested, source)
		}
		return source, nil
	case translationRepresentationBitmap, "pgs", "vobsub":
		switch requested {
		case "", translationRepresentationSRT:
			return translationRepresentationSRT, nil
		case translationRepresentationASS:
			return translationRepresentationASS, nil
		default:
			return "", fmt.Errorf("bitmap translation does not support output representation %s", requested)
		}
	default:
		return "", fmt.Errorf("unsupported translation source representation %s", source)
	}
}

func translatedBitmapCueView(source []canonicalBitmapSubtitleCue, translated map[int]string) []canonicalBitmapSubtitleCue {
	result := append([]canonicalBitmapSubtitleCue(nil), source...)
	for index := range result {
		if text, ok := translated[result[index].SourceOrdinal]; ok {
			result[index].Text = text
		}
	}
	return result
}

// mergeTranslatedBitmapCues is the stable-ID boundary between provider output
// and canonical bitmap cues. The provider result is already contract-validated
// by Task 3.1; this helper additionally verifies that the envelopes still
// describe the exact immutable source cues before changing text in a deep copy.
func mergeTranslatedBitmapCues(source []canonicalBitmapSubtitleCue, envelopes []translationCueEnvelope, translated []TranslatedCue) ([]canonicalBitmapSubtitleCue, error) {
	if len(source) == 0 || len(source) != len(envelopes) || len(source) != len(translated) {
		return nil, fmt.Errorf("translated bitmap cue count does not match canonical envelopes")
	}
	expected := make(map[string]int, len(envelopes))
	for index, envelope := range envelopes {
		if strings.TrimSpace(envelope.StableID) == "" {
			return nil, fmt.Errorf("translation envelope has an empty stable ID")
		}
		if _, exists := expected[envelope.StableID]; exists {
			return nil, fmt.Errorf("translation envelopes contain duplicate stable ID %q", envelope.StableID)
		}
		if envelope.SourceOrdinal != source[index].SourceOrdinal || envelope.Text != source[index].Text || envelope.Timing.Start != source[index].Start || envelope.Timing.End != source[index].End || envelope.Timing.TimingProvenance != source[index].TimingProvenance || !reflect.DeepEqual(envelope.Geometry, source[index].Geometry) {
			return nil, fmt.Errorf("translation envelope does not match canonical cue %d", index)
		}
		expected[envelope.StableID] = index
	}
	result := cloneCanonicalCues(source)
	seen := make(map[string]struct{}, len(translated))
	for index, cue := range translated {
		if strings.TrimSpace(cue.ID) == "" {
			return nil, fmt.Errorf("translated bitmap cue has an empty stable ID")
		}
		if _, exists := seen[cue.ID]; exists {
			return nil, fmt.Errorf("translated bitmap cues contain duplicate stable ID %q", cue.ID)
		}
		seen[cue.ID] = struct{}{}
		expectedIndex, exists := expected[cue.ID]
		if !exists {
			return nil, fmt.Errorf("translated bitmap cue contains unexpected stable ID %q", cue.ID)
		}
		if expectedIndex != index {
			return nil, fmt.Errorf("translated bitmap cues reordered stable ID %q", cue.ID)
		}
		if strings.TrimSpace(cue.Text) == "" {
			return nil, fmt.Errorf("translated bitmap cue %q has empty text", cue.ID)
		}
		result[expectedIndex].Text = cue.Text
	}
	return result, nil
}

func serializeTranslatedBitmapCuesFromResult(source []canonicalBitmapSubtitleCue, envelopes []translationCueEnvelope, result []TranslatedCue, requestedRepresentation string) (map[string][]byte, error) {
	cues, err := mergeTranslatedBitmapCues(source, envelopes, result)
	if err != nil {
		return nil, err
	}
	return serializeTranslatedBitmapCueFanout(cues, requestedRepresentation)
}

func serializeTranslatedBitmapCueFanout(cues []canonicalBitmapSubtitleCue, requestedRepresentation string) (map[string][]byte, error) {
	requested := strings.ToLower(strings.TrimSpace(requestedRepresentation))
	if requested == "" {
		requested = translationRepresentationSRT
	}
	if requested != translationRepresentationSRT && requested != translationRepresentationASS && requested != "both" {
		return nil, fmt.Errorf("bitmap translation does not support output representation %s", requested)
	}
	formats := []string{requested}
	if requested == "both" {
		formats = []string{translationRepresentationSRT, translationRepresentationASS}
	}
	outputs := make(map[string][]byte, len(formats))
	for _, format := range formats {
		var content []byte
		if format == translationRepresentationSRT {
			content = renderBitmapSRTCues(cues)
		} else {
			content = renderBitmapASSCues(cues)
		}
		if !validSubtitleSidecar(format, content) {
			return nil, fmt.Errorf("translated bitmap %s output failed validation", format)
		}
		outputs[format] = content
	}
	return outputs, nil
}

func serializeTranslatedBitmapCues(source []canonicalBitmapSubtitleCue, translated map[int]string, requestedRepresentation string) ([]byte, error) {
	output, err := resolveTranslationOutputRepresentation(translationRepresentationBitmap, requestedRepresentation)
	if err != nil {
		return nil, err
	}
	outputs, err := serializeTranslatedBitmapCueFanout(translatedBitmapCueView(source, translated), output)
	if err != nil {
		return nil, err
	}
	return outputs[output], nil
}

func serializeTranslatedTextCues(sourceRepresentation string, source []canonicalBitmapSubtitleCue, translated map[int]string, requestedRepresentation string) ([]byte, error) {
	output, err := resolveTranslationOutputRepresentation(sourceRepresentation, requestedRepresentation)
	if err != nil {
		return nil, err
	}
	if output != translationRepresentationSRT {
		return nil, fmt.Errorf("canonical text cue serializer only supports SRT, got %s", output)
	}
	return renderBitmapSRTCues(translatedBitmapCueView(source, translated)), nil
}

func serializeTranslatedASSDocument(document translationASSDocument, translated map[int]string, requestedRepresentation string) ([]byte, error) {
	output, err := resolveTranslationOutputRepresentation(document.Representation, requestedRepresentation)
	if err != nil {
		return nil, err
	}
	if output != document.Representation {
		return nil, fmt.Errorf("protected text output changed representation from %s to %s", document.Representation, output)
	}
	result, err := document.ReconstructDialogueProviderTexts(translated)
	if err != nil {
		return nil, err
	}
	return []byte(result), nil
}
