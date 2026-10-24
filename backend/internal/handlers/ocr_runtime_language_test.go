package handlers

import (
	"strings"
	"testing"
)

func TestFrenchKoreanOCRRuntimeLanguagesFailClosedWhenTraineddataIsMissing(t *testing.T) {
	tests := []struct {
		name      string
		requested string
		detected  string
		canonical string
	}{
		{name: "French ISO 639-1", requested: "fr", canonical: "fra"},
		{name: "French ISO 639-2/T", requested: "fra", canonical: "fra"},
		{name: "French ISO 639-2/B", requested: "fre", canonical: "fra"},
		{name: "French BCP-47 region", requested: "fr-FR", canonical: "fra"},
		{name: "French detected BCP-47 region", detected: "fr-FR", canonical: "fra"},
		{name: "Korean ISO 639-1", requested: "ko", canonical: "kor"},
		{name: "Korean ISO 639-2", requested: "kor", canonical: "kor"},
		{name: "Korean BCP-47 region", requested: "ko-KR", canonical: "kor"},
		{name: "Korean detected BCP-47 region", detected: "ko-KR", canonical: "kor"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			language := normalizedOCRLanguage(tt.requested, tt.detected)
			if language != tt.canonical {
				t.Fatalf("normalizedOCRLanguage(%q, %q) = %q, want %q", tt.requested, tt.detected, language, tt.canonical)
			}

			if err := requireOCRLanguageAvailable(map[string]struct{}{language: {}}, language); err != nil {
				t.Fatalf("installed canonical language %q was rejected: %v", language, err)
			}

			err := requireOCRLanguageAvailable(map[string]struct{}{"eng": {}}, language)
			if err == nil || !strings.Contains(err.Error(), `OCR language "`+language+`" is unavailable`) {
				t.Fatalf("missing %q traineddata did not fail closed without English fallback: %v", language, err)
			}
		})
	}
}

func TestOCRLanguageNormalizationPreservesEnglishDefaultForUnsupportedValues(t *testing.T) {
	for _, input := range []string{"", "und", "unknown", "pt-BR"} {
		t.Run(input, func(t *testing.T) {
			if got := normalizedOCRLanguage(input, ""); got != "eng" {
				t.Fatalf("normalizedOCRLanguage(%q) = %q, want existing default eng", input, got)
			}
		})
	}
}
