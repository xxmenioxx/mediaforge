package handlers

import (
	"fmt"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

var supportedSubtitleLanguages = map[string]struct{}{
	"spa": {}, "eng": {}, "jpn": {}, "fra": {}, "deu": {},
	"ita": {}, "por": {}, "kor": {}, "zho": {},
}

var subtitleLanguageAliases = map[string]string{
	"es":  "spa",
	"esp": "spa",
	"en":  "eng",
	"ja":  "jpn",
	"fr":  "fra",
	"fre": "fra",
	"de":  "deu",
	"ger": "deu",
	"it":  "ita",
	"pt":  "por",
	"ko":  "kor",
	"zh":  "zho",
	"chi": "zho",
}

func canonicalSubtitleLanguage(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if canonical, ok := subtitleLanguageAliases[value]; ok {
		value = canonical
	}
	_, ok := supportedSubtitleLanguages[value]
	return value, ok
}

func normalizePreferredSubtitleRules(value any) (models.JSONList, error) {
	if value == nil {
		return models.JSONList{}, nil
	}
	rawRules, ok := value.(models.JSONList)
	if !ok {
		if values, ok := value.([]any); ok {
			rawRules = models.JSONList(values)
		} else {
			return nil, fmt.Errorf("preferredSubtitleRules must be an array")
		}
	}
	normalized := models.JSONList{}
	positions := map[string]int{}
	for _, raw := range rawRules {
		rule := settingProfileObject(raw)
		if rule == nil {
			return nil, fmt.Errorf("preferredSubtitleRules entries must be objects")
		}
		language, ok := canonicalSubtitleLanguage(workerStringValue(rule["language"]))
		if !ok {
			return nil, fmt.Errorf("preferredSubtitleRules language %q is unsupported", workerStringValue(rule["language"]))
		}
		action, err := ParseSubtitleDisposition(workerStringValue(rule["action"]))
		if err != nil {
			return nil, fmt.Errorf("preferredSubtitleRules.%s: %w", language, err)
		}
		if action == SubtitleDispositionRemove {
			return nil, fmt.Errorf("preferredSubtitleRules.%s cannot use remove", language)
		}
		normalizedRule := models.JSONMap{"language": language, "action": string(action)}
		if position, exists := positions[language]; exists {
			normalized[position] = normalizedRule
			continue
		}
		positions[language] = len(normalized)
		normalized = append(normalized, normalizedRule)
	}
	return normalized, nil
}

func normalizedTrackLanguageValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "und"
	}
	if canonical, ok := canonicalSubtitleLanguage(value); ok {
		return canonical
	}
	if canonical, ok := subtitleLanguageAliases[value]; ok {
		return canonical
	}
	return value
}
