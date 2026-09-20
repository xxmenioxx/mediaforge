package handlers

import (
	"fmt"
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

func serializeTranslatedBitmapCues(source []canonicalBitmapSubtitleCue, translated map[int]string, requestedRepresentation string) ([]byte, error) {
	output, err := resolveTranslationOutputRepresentation(translationRepresentationBitmap, requestedRepresentation)
	if err != nil {
		return nil, err
	}
	cues := translatedBitmapCueView(source, translated)
	switch output {
	case translationRepresentationSRT:
		return renderBitmapSRTCues(cues), nil
	case translationRepresentationASS:
		return renderBitmapASSCues(cues), nil
	default:
		return nil, fmt.Errorf("unsupported bitmap translation output %s", output)
	}
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
