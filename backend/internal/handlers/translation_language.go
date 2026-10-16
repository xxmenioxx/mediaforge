package handlers

import (
	"fmt"
	"strings"
	"unicode"
)

const (
	translationLanguageEvidenceSourceMetadata = "source-metadata"
	translationLanguageEvidenceSourceCues     = "bounded-cue-evidence"
	translationLanguageEvidenceSourceExplicit = "explicit"
	translationLanguageEvidenceMaxCues        = 8
	translationLanguageEvidenceMaxBytes       = 4096
	translationLanguageEvidenceMaxRunes       = 2048
	translationLanguageEvidenceMaxSummary     = 160
)

var translationEnglishEvidenceWords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "are": {}, "for": {}, "in": {}, "is": {}, "it": {}, "of": {}, "on": {}, "that": {}, "the": {}, "this": {}, "to": {}, "we": {}, "you": {},
}

// translationLanguageResolutionInput is operation-local evidence. Protected
// ASS/SSA dialogues are supplied in their existing parsed representation so
// structural tokens never become language evidence.
type translationLanguageResolutionInput struct {
	RequestedLanguage      string
	SourceLanguageMetadata []string
	Cues                   []translationSourceCue
	ProtectedDialogues     []translationProtectedDialogue
	// AdvisoryMediaContext is intentionally ignored. Media context belongs to
	// translation assistance and cannot participate in source-language choice.
	AdvisoryMediaContext *TranslationContext
}

type translationLanguageResolution struct {
	RequestedLanguage string
	EffectiveLanguage string
	EvidenceSource    string
	EvidenceSummary   string
}

func resolveTranslationSourceLanguage(input translationLanguageResolutionInput) (translationLanguageResolution, error) {
	requested, err := normalizeTranslationLanguage(input.RequestedLanguage, true)
	if err != nil {
		return translationLanguageResolution{}, fmt.Errorf("unsupported translation source language %q", input.RequestedLanguage)
	}
	if requested != "auto" {
		return translationLanguageResolution{
			RequestedLanguage: requested,
			EffectiveLanguage: requested,
			EvidenceSource:    translationLanguageEvidenceSourceExplicit,
			EvidenceSummary:   "explicit source language selection",
		}, nil
	}

	metadataLanguage, metadataFound, err := translationLanguageFromMetadata(input.SourceLanguageMetadata)
	if err != nil {
		return translationLanguageResolution{}, err
	}
	if metadataFound {
		return translationLanguageResolution{
			RequestedLanguage: requested,
			EffectiveLanguage: metadataLanguage,
			EvidenceSource:    translationLanguageEvidenceSourceMetadata,
			EvidenceSummary:   boundedTranslationLanguageSummary("reliable source metadata: " + metadataLanguage),
		}, nil
	}

	evidenceText := boundedTranslationLanguageEvidence(input)
	lang, summary, err := classifyBoundedTranslationLanguageEvidence(evidenceText)
	if err != nil {
		return translationLanguageResolution{}, err
	}
	return translationLanguageResolution{
		RequestedLanguage: requested,
		EffectiveLanguage: lang,
		EvidenceSource:    translationLanguageEvidenceSourceCues,
		EvidenceSummary:   boundedTranslationLanguageSummary(summary),
	}, nil
}

func translationLanguageFromMetadata(values []string) (string, bool, error) {
	resolved := ""
	for _, raw := range values {
		value := strings.ToLower(strings.TrimSpace(raw))
		if value == "" || value == "und" || value == "unknown" || value == "unidentified" {
			continue
		}
		if value == "jpn_vert" {
			value = "ja"
		}
		language, err := normalizeTranslationLanguage(value, false)
		if err != nil {
			return "", false, ambiguousTranslationLanguageError("unsupported source language metadata")
		}
		if resolved != "" && resolved != language {
			return "", false, ambiguousTranslationLanguageError("conflicting source language metadata")
		}
		resolved = language
	}
	return resolved, resolved != "", nil
}

func boundedTranslationLanguageEvidence(input translationLanguageResolutionInput) []string {
	evidence := make([]string, 0, translationLanguageEvidenceMaxCues)
	runesUsed := 0
	bytesUsed := 0
	if len(input.ProtectedDialogues) > 0 {
		for _, dialogue := range input.ProtectedDialogues {
			if !appendBoundedTranslationLanguageEvidence(&evidence, strings.Join(dialogue.OriginalTextSegments(), " "), &bytesUsed, &runesUsed) {
				break
			}
		}
		return evidence
	}
	for _, cue := range input.Cues {
		if !appendBoundedTranslationLanguageEvidence(&evidence, cue.Text, &bytesUsed, &runesUsed) {
			break
		}
	}
	return evidence
}

func appendBoundedTranslationLanguageEvidence(evidence *[]string, text string, bytesUsed, runesUsed *int) bool {
	if len(*evidence) >= translationLanguageEvidenceMaxCues || *bytesUsed >= translationLanguageEvidenceMaxBytes || *runesUsed >= translationLanguageEvidenceMaxRunes {
		return false
	}
	var builder strings.Builder
	for _, r := range text {
		runeBytes := len(string(r))
		if *bytesUsed+runeBytes > translationLanguageEvidenceMaxBytes || *runesUsed >= translationLanguageEvidenceMaxRunes {
			*evidence = append(*evidence, builder.String())
			return false
		}
		builder.WriteRune(r)
		*bytesUsed += runeBytes
		*runesUsed++
	}
	*evidence = append(*evidence, builder.String())
	return true
}

func classifyBoundedTranslationLanguageEvidence(texts []string) (string, string, error) {
	kanaCount := 0
	hangulCount := 0
	hanCount := 0
	usefulCueCount := 0
	englishMarkers := make(map[string]struct{})
	markerCueCount := make(map[int]struct{})
	for cueIndex, text := range texts {
		markerFoundInCue := false
		lettersOrDigits := false
		for _, r := range text {
			switch {
			case isJapaneseKana(r):
				kanaCount++
				lettersOrDigits = true
			case unicode.Is(unicode.Hangul, r):
				hangulCount++
				lettersOrDigits = true
			case isHanRune(r):
				hanCount++
				lettersOrDigits = true
			case unicode.IsLetter(r) || unicode.IsDigit(r):
				lettersOrDigits = true
			}
		}
		if lettersOrDigits {
			usefulCueCount++
		}
		for _, cue := range strings.Fields(text) {
			for _, token := range strings.FieldsFunc(cue, func(r rune) bool { return !unicode.IsLetter(r) }) {
				if _, ok := translationEnglishEvidenceWords[strings.ToLower(token)]; ok {
					englishMarkers[strings.ToLower(token)] = struct{}{}
					markerFoundInCue = true
				}
			}
		}
		if markerFoundInCue {
			markerCueCount[cueIndex] = struct{}{}
		}
	}

	if hangulCount >= 2 && (kanaCount >= 2 || len(englishMarkers) >= 2) {
		return "", "", ambiguousTranslationLanguageError("mixed Korean and other language evidence")
	}
	if hangulCount >= 2 {
		return "ko", "strong Korean script evidence", nil
	}
	if kanaCount >= 2 && len(englishMarkers) >= 2 {
		return "", "", ambiguousTranslationLanguageError("mixed English and Japanese evidence")
	}
	if kanaCount >= 2 {
		return "ja", "strong Japanese script evidence", nil
	}
	if hanCount > 0 {
		return "", "", ambiguousTranslationLanguageError("Han-only or mixed script evidence")
	}
	if len(englishMarkers) >= 2 && (usefulCueCount == 1 || len(markerCueCount) >= 2) {
		return "en", "bounded English lexical evidence", nil
	}
	return "", "", ambiguousTranslationLanguageError("insufficient bounded language evidence")
}

func isJapaneseKana(r rune) bool {
	return (r >= '\u3040' && r <= '\u309f') || (r >= '\u30a0' && r <= '\u30ff')
}

func isHanRune(r rune) bool {
	return unicode.Is(unicode.Han, r)
}

func ambiguousTranslationLanguageError(reason string) error {
	return fmt.Errorf("source language is ambiguous; select a supported source language explicitly (%s)", reason)
}

func boundedTranslationLanguageSummary(value string) string {
	if len(value) <= translationLanguageEvidenceMaxSummary {
		return value
	}
	return value[:translationLanguageEvidenceMaxSummary]
}
