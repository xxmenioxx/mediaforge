package handlers

import (
	"regexp"
	"strings"
)

const subtitleTranslationQualityPolicyVersion = "translation-quality-v1"
const subtitleTranslationQualityMaxFindings = 100
const subtitleTranslationQualityMaxCueRunes = 320

var subtitleTranslationQualityNumber = regexp.MustCompile(`[0-9]+`)

type subtitleTranslationQualityFinding struct {
	CueID          string   `json:"cueId"`
	SourceOrdinal  int      `json:"sourceOrdinal"`
	SourceText     string   `json:"sourceText"`
	TranslatedText string   `json:"translatedText"`
	Reasons        []string `json:"reasons"`
}

type subtitleTranslationQualityReport struct {
	PolicyVersion      string                                    `json:"policyVersion"`
	State              string                                    `json:"state"`
	CueCount           int                                       `json:"cueCount"`
	CleanCount         int                                       `json:"cleanCount"`
	WarningCount       int                                       `json:"warningCount"`
	ReviewCount        int                                       `json:"reviewCount"`
	Persisted          bool                                      `json:"persisted"`
	PersistenceWarning string                                    `json:"persistenceWarning,omitempty"`
	Findings           []subtitleTranslationQualityFinding       `json:"findings,omitempty"`
	Truncated          bool                                      `json:"truncated,omitempty"`
	Reviewer           *subtitleTranslationQualityReviewerResult `json:"reviewer,omitempty"`
}

// analyzeSubtitleTranslationQuality emits deterministic review signals for
// structurally matched cues. Findings are advisory and never edit cue content.
func analyzeSubtitleTranslationQuality(source []translationSourceCue, envelopes []translationCueEnvelope, translated []TranslatedCue) subtitleTranslationQualityReport {
	report := subtitleTranslationQualityReport{PolicyVersion: subtitleTranslationQualityPolicyVersion, State: "clean", CueCount: len(source)}
	if len(source) == 0 || len(source) != len(envelopes) || len(source) != len(translated) {
		report.State = "not_analyzed"
		return report
	}
	translationByID := make(map[string]string, len(translated))
	for _, cue := range translated {
		translationByID[cue.ID] = cue.Text
	}
	seenTranslation := make(map[string]string, len(translated))
	envelopeByOrdinal := make(map[int]translationCueEnvelope, len(envelopes))
	for _, envelope := range envelopes {
		envelopeByOrdinal[envelope.SourceOrdinal] = envelope
	}
	for _, sourceCue := range source {
		envelope, exists := envelopeByOrdinal[sourceCue.SourceOrdinal]
		if !exists {
			report.State = "not_analyzed"
			report.Findings = nil
			return report
		}
		translationText, exists := translationByID[envelope.StableID]
		if !exists {
			report.State = "not_analyzed"
			report.Findings = nil
			return report
		}
		var reasons []string
		sourceText := strings.TrimSpace(sourceCue.Text)
		targetText := strings.TrimSpace(translationText)
		switch {
		case sourceText != "" && targetText == "":
			reasons = append(reasons, "empty_translation")
		case sourceText == "" && targetText != "":
			reasons = append(reasons, "translation_without_source_text")
		}
		if len([]rune(sourceText)) >= 20 && len([]rune(targetText)) > 0 && float64(len([]rune(targetText)))/float64(len([]rune(sourceText))) < 0.2 {
			reasons = append(reasons, "unusually_short_translation")
		}
		if sourceNumbers := subtitleTranslationQualityNumber.FindAllString(sourceText, -1); len(sourceNumbers) > 0 {
			targetNumbers := subtitleTranslationQualityNumber.FindAllString(targetText, -1)
			for _, number := range sourceNumbers {
				if !containsSubtitleTranslationQualityString(targetNumbers, number) {
					reasons = append(reasons, "number_not_preserved")
					break
				}
			}
		}
		if targetText != "" {
			key := strings.ToLower(strings.Join(strings.Fields(targetText), " "))
			if previousSource, exists := seenTranslation[key]; exists && previousSource != strings.ToLower(strings.Join(strings.Fields(sourceText), " ")) {
				reasons = append(reasons, "duplicate_translation_for_distinct_source")
			} else {
				seenTranslation[key] = strings.ToLower(strings.Join(strings.Fields(sourceText), " "))
			}
		}
		if len(reasons) == 0 {
			report.CleanCount++
			continue
		}
		report.WarningCount++
		report.ReviewCount++
		if len(report.Findings) >= subtitleTranslationQualityMaxFindings {
			report.Truncated = true
			continue
		}
		report.Findings = append(report.Findings, subtitleTranslationQualityFinding{
			CueID: envelope.StableID, SourceText: truncateSubtitleTranslationQualityText(sourceCue.Text),
			SourceOrdinal: sourceCue.SourceOrdinal, TranslatedText: truncateSubtitleTranslationQualityText(translationText), Reasons: reasons,
		})
	}
	if report.ReviewCount > 0 {
		report.State = "review_required"
	} else if report.WarningCount > 0 {
		report.State = "warnings"
	}
	return report
}

func truncateSubtitleTranslationQualityText(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= subtitleTranslationQualityMaxCueRunes {
		return string(runes)
	}
	return string(runes[:subtitleTranslationQualityMaxCueRunes])
}

func containsSubtitleTranslationQualityString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
