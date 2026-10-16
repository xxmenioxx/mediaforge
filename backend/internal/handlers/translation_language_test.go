package handlers

import (
	"strings"
	"testing"
)

func TestResolveTranslationSourceLanguageExplicitSelectionIsAuthoritative(t *testing.T) {
	for _, test := range []struct {
		name string
		want string
	}{
		{name: "english", want: "en"},
		{name: "japanese", want: "ja"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := resolveTranslationSourceLanguage(translationLanguageResolutionInput{
				RequestedLanguage:      test.want,
				SourceLanguageMetadata: []string{"spa"},
				Cues:                   []translationSourceCue{{Text: "これは日本語"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.RequestedLanguage != test.want || result.EffectiveLanguage != test.want || result.EvidenceSource != translationLanguageEvidenceSourceExplicit {
				t.Fatalf("unexpected explicit result: %#v", result)
			}
		})
	}
	if _, err := resolveTranslationSourceLanguage(translationLanguageResolutionInput{RequestedLanguage: "es"}); err == nil {
		t.Fatal("unsupported explicit language accepted")
	}
}

func TestResolveTranslationSourceLanguageAutoUsesReliableMetadata(t *testing.T) {
	for _, metadata := range []string{"en", "eng", "english"} {
		result, err := resolveTranslationSourceLanguage(translationLanguageResolutionInput{RequestedLanguage: "auto", SourceLanguageMetadata: []string{metadata}})
		if err != nil || result.EffectiveLanguage != "en" || result.EvidenceSource != translationLanguageEvidenceSourceMetadata {
			t.Fatalf("metadata %q did not resolve English: result=%#v err=%v", metadata, result, err)
		}
	}
	for _, metadata := range []string{"ja", "jpn", "japanese", "jpn_vert"} {
		result, err := resolveTranslationSourceLanguage(translationLanguageResolutionInput{RequestedLanguage: "auto", SourceLanguageMetadata: []string{metadata}})
		if err != nil || result.EffectiveLanguage != "ja" || result.EvidenceSource != translationLanguageEvidenceSourceMetadata {
			t.Fatalf("metadata %q did not resolve Japanese: result=%#v err=%v", metadata, result, err)
		}
	}
	for _, metadata := range []string{"ko", "kor", "korean"} {
		result, err := resolveTranslationSourceLanguage(translationLanguageResolutionInput{RequestedLanguage: "auto", SourceLanguageMetadata: []string{metadata}})
		if err != nil || result.EffectiveLanguage != "ko" || result.EvidenceSource != translationLanguageEvidenceSourceMetadata {
			t.Fatalf("metadata %q did not resolve Korean: result=%#v err=%v", metadata, result, err)
		}
	}
	for _, metadata := range []string{"fr", "fra", "fre", "french"} {
		result, err := resolveTranslationSourceLanguage(translationLanguageResolutionInput{RequestedLanguage: "auto", SourceLanguageMetadata: []string{metadata}})
		if err != nil || result.EffectiveLanguage != "fr" || result.EvidenceSource != translationLanguageEvidenceSourceMetadata {
			t.Fatalf("metadata %q did not resolve French: result=%#v err=%v", metadata, result, err)
		}
	}
	for _, metadata := range [][]string{{"und"}, {""}, {"spa"}, {"eng", "jpn"}} {
		if result, err := resolveTranslationSourceLanguage(translationLanguageResolutionInput{RequestedLanguage: "auto", SourceLanguageMetadata: metadata, Cues: []translationSourceCue{{Text: "Name"}}}); err == nil {
			t.Fatalf("metadata %v unexpectedly resolved: %#v", metadata, result)
		}
	}
	for _, metadata := range [][]string{{"kor", "fre"}, {"fr", "ko"}} {
		if _, err := resolveTranslationSourceLanguage(translationLanguageResolutionInput{RequestedLanguage: "auto", SourceLanguageMetadata: metadata}); err == nil {
			t.Fatalf("mixed language metadata %v was not rejected", metadata)
		}
	}
}

func TestResolveTranslationSourceLanguageUsesConservativeBoundedCueEvidence(t *testing.T) {
	japanese, err := resolveTranslationSourceLanguage(translationLanguageResolutionInput{RequestedLanguage: "auto", Cues: []translationSourceCue{{Text: "こんにちは世界"}}})
	if err != nil || japanese.EffectiveLanguage != "ja" {
		t.Fatalf("strong kana evidence did not resolve Japanese: %#v err=%v", japanese, err)
	}
	english, err := resolveTranslationSourceLanguage(translationLanguageResolutionInput{RequestedLanguage: "auto", Cues: []translationSourceCue{{Text: "This is the subtitle"}, {Text: "You are here"}}})
	if err != nil || english.EffectiveLanguage != "en" {
		t.Fatalf("strong English evidence did not resolve English: %#v err=%v", english, err)
	}
	korean, err := resolveTranslationSourceLanguage(translationLanguageResolutionInput{RequestedLanguage: "auto", Cues: []translationSourceCue{{Text: "안녕하세요 세계"}}})
	if err != nil || korean.EffectiveLanguage != "ko" {
		t.Fatalf("strong Hangul script evidence did not resolve Korean: %#v err=%v", korean, err)
	}
	for _, text := range []string{"東京", "안", "John Smith", "ROLL CREDITS", "12345 !!!", "de la que", "Bonjour tout le monde", "https://example.com"} {
		if result, err := resolveTranslationSourceLanguage(translationLanguageResolutionInput{RequestedLanguage: "auto", Cues: []translationSourceCue{{Text: text}}}); err == nil {
			t.Fatalf("weak evidence %q unexpectedly resolved: %#v", text, result)
		}
	}
	if _, err := resolveTranslationSourceLanguage(translationLanguageResolutionInput{RequestedLanguage: "auto", Cues: []translationSourceCue{{Text: "This is the text"}, {Text: "こんにちは"}}}); err == nil {
		t.Fatal("mixed strong English/Japanese evidence resolved")
	}
	if _, err := resolveTranslationSourceLanguage(translationLanguageResolutionInput{RequestedLanguage: "auto", Cues: []translationSourceCue{{Text: "This is Korean 안녕하세요"}}}); err == nil {
		t.Fatal("mixed strong English/Korean evidence resolved")
	}
	if _, err := resolveTranslationSourceLanguage(translationLanguageResolutionInput{RequestedLanguage: "auto", Cues: []translationSourceCue{{Text: "안녕하세요"}, {Text: "こんにちは"}}}); err == nil {
		t.Fatal("mixed Korean/Japanese script evidence resolved")
	}
}

func TestTranslationLanguageEvidenceIsBoundedAndDoesNotExposeCueText(t *testing.T) {
	cues := make([]translationSourceCue, translationLanguageEvidenceMaxCues+1)
	for index := range cues {
		cues[index].Text = "Name"
	}
	cues[translationLanguageEvidenceMaxCues].Text = "This is the English evidence"
	if _, err := resolveTranslationSourceLanguage(translationLanguageResolutionInput{RequestedLanguage: "auto", Cues: cues}); err == nil {
		t.Fatal("cue beyond maximum sampled count affected language resolution")
	}
	long := strings.Repeat("Name ", translationLanguageEvidenceMaxRunes)
	long += " This is the English evidence"
	if _, err := resolveTranslationSourceLanguage(translationLanguageResolutionInput{RequestedLanguage: "auto", Cues: []translationSourceCue{{Text: long}}}); err == nil {
		t.Fatal("cue text beyond evidence rune bound affected language resolution")
	}
	if evidence := boundedTranslationLanguageEvidence(translationLanguageResolutionInput{Cues: []translationSourceCue{{Text: strings.Repeat("x", 10_000)}}}); len(strings.Join(evidence, "")) > translationLanguageEvidenceMaxBytes {
		t.Fatalf("bounded evidence exceeded byte limit: %d", len(strings.Join(evidence, "")))
	}
	result, err := resolveTranslationSourceLanguage(translationLanguageResolutionInput{RequestedLanguage: "auto", Cues: []translationSourceCue{{Text: "ambiguous provider-visible text"}}})
	if err == nil || result.EvidenceSummary != "" {
		t.Fatalf("ambiguous result exposed raw evidence: result=%#v err=%v", result, err)
	}
}

func TestTranslationLanguageEvidenceIgnoresASSProtectedStructure(t *testing.T) {
	dialogue, err := parseTranslationProtectedDialogue(`{\an8\pos(10,20)\k20}This is the visible text\N{\b1}日本語`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := resolveTranslationSourceLanguage(translationLanguageResolutionInput{RequestedLanguage: "auto", ProtectedDialogues: []translationProtectedDialogue{dialogue}}); err == nil {
		t.Fatal("protected ASS structure changed language resolution")
	}
	japaneseDialogue, err := parseTranslationProtectedDialogue(`{\an8}こんにちは\N世界`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := resolveTranslationSourceLanguage(translationLanguageResolutionInput{RequestedLanguage: "auto", ProtectedDialogues: []translationProtectedDialogue{japaneseDialogue}})
	if err != nil || result.EffectiveLanguage != "ja" {
		t.Fatalf("visible Japanese dialogue was not analyzed: %#v err=%v", result, err)
	}
	if dialogue.TranslatableText() == strings.Join(dialogue.OriginalTextSegments(), " ") {
		t.Fatal("test fixture did not contain protected provider anchors")
	}
}

func TestTranslationLanguageResolutionDoesNotMutateCuesOrUseMediaContext(t *testing.T) {
	cues := []translationSourceCue{{SourceOrdinal: 4, Text: "This is the text", Start: 1, End: 2}}
	before := cues[0]
	withoutContext, err := resolveTranslationSourceLanguage(translationLanguageResolutionInput{RequestedLanguage: "auto", Cues: cues})
	if err != nil {
		t.Fatal(err)
	}
	withContext, err := resolveTranslationSourceLanguage(translationLanguageResolutionInput{
		RequestedLanguage:    "auto",
		Cues:                 cues,
		AdvisoryMediaContext: &TranslationContext{Media: TranslationMediaContext{Title: "Título", Synopsis: "日本語の説明"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if withoutContext != withContext || cues[0] != before {
		t.Fatalf("language resolution changed with advisory context or mutated cues: before=%#v after=%#v", before, cues[0])
	}
	if withoutContext.RequestedLanguage != "auto" || withoutContext.EffectiveLanguage != "en" {
		t.Fatalf("unexpected resolved language: %#v", withoutContext)
	}
}
