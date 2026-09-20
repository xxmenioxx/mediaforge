package handlers

import (
	"strings"
	"testing"
)

func TestTranslationIdentityIsDeterministicAndExcludesTranslatedText(t *testing.T) {
	input := translationIdentityTestInput()
	first, err := newTranslationIdentity(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := newTranslationIdentity(input)
	if err != nil {
		t.Fatal(err)
	}
	if first.key() != second.key() {
		t.Fatalf("same canonical identity produced different keys: %q != %q", first.key(), second.key())
	}

	translatedText := "Hola, mundo"
	if translatedText == input.SourceArtifactIdentity {
		t.Fatal("test setup unexpectedly used translated text as source identity")
	}
	withoutTranslatedText, err := newTranslationIdentity(input)
	if err != nil || withoutTranslatedText.key() != first.key() {
		t.Fatalf("translated text changed identity: err=%v", err)
	}
}

func TestTranslationIdentityEveryCanonicalInputParticipates(t *testing.T) {
	base, err := newTranslationIdentity(translationIdentityTestInput())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		mutate func(*translationIdentityInput)
	}{
		{name: "source revision", mutate: func(input *translationIdentityInput) { input.SourceRevision = "revision-2" }},
		{name: "source artifact stream", mutate: func(input *translationIdentityInput) { input.SourceArtifactIdentity = "/media/title.mkv#stream=5" }},
		{name: "source representation", mutate: func(input *translationIdentityInput) { input.SourceRepresentation = "ass" }},
		{name: "source provenance", mutate: func(input *translationIdentityInput) { input.SourceProvenance = "native-ass-v2" }},
		{name: "requested language", mutate: func(input *translationIdentityInput) { input.RequestedSourceLanguage = "japanese" }},
		{name: "effective language", mutate: func(input *translationIdentityInput) { input.EffectiveSourceLanguage = "ja" }},
		{name: "target locale", mutate: func(input *translationIdentityInput) { input.TargetLocale = "es-ES" }},
		{name: "provider", mutate: func(input *translationIdentityInput) { input.Provider = "other-provider" }},
		{name: "provider contract", mutate: func(input *translationIdentityInput) { input.ProviderContractVersion = "provider-v2" }},
		{name: "provider model", mutate: func(input *translationIdentityInput) { input.ProviderModel = "different-model" }},
		{name: "context contract", mutate: func(input *translationIdentityInput) { input.ContextContractVersion = "context-v2" }},
		{name: "context mode", mutate: func(input *translationIdentityInput) { input.ContextMode = "neighbors" }},
		{name: "output representation", mutate: func(input *translationIdentityInput) { input.OutputRepresentation = "ass" }},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := translationIdentityTestInput()
			test.mutate(&input)
			changed, err := newTranslationIdentity(input)
			if err != nil {
				t.Fatal(err)
			}
			if changed.key() == base.key() {
				t.Fatalf("%s did not change identity key", test.name)
			}
		})
	}
}

func TestTranslationIdentityNormalizesSupportedLocalesAndRejectsUnsupported(t *testing.T) {
	es419, err := newTranslationIdentity(translationIdentityTestInputWithLocale(" ES_419 "))
	if err != nil {
		t.Fatal(err)
	}
	if es419.TargetLocale != "es-419" {
		t.Fatalf("locale=%q, want es-419", es419.TargetLocale)
	}
	esES, err := newTranslationIdentity(translationIdentityTestInputWithLocale("es-es"))
	if err != nil {
		t.Fatal(err)
	}
	if esES.TargetLocale != "es-ES" || es419.key() == esES.key() {
		t.Fatalf("locale identities were not distinct: %#v %#v", es419, esES)
	}
	if _, err := newTranslationIdentity(translationIdentityTestInputWithLocale("fr-FR")); err == nil {
		t.Fatal("unsupported locale accepted")
	}
}

func TestTranslatedSubtitleArtifactMetadataRequiresExactIdentity(t *testing.T) {
	identity, err := newTranslationIdentity(translationIdentityTestInput())
	if err != nil {
		t.Fatal(err)
	}
	metadata := identity.metadata()
	source := SubtitleArtifact{ArtifactID: "subtitle:4:converted:srt", StreamIndex: 4, Mode: "converted", Format: "srt", Language: "eng"}
	translated := source
	translated.ArtifactID = subtitleTranslationArtifactID(source, metadata)
	translated.Translation = &metadata
	if translated.ArtifactID == "" || !translationArtifactMatchesIdentity(translated, identity) {
		t.Fatal("exact translated artifact identity was not reusable")
	}

	nearMatch := metadata
	nearMatch.SourceRevision = "revision-2"
	translated.Translation = &nearMatch
	if translationArtifactMatchesIdentity(translated, identity) {
		t.Fatal("near-match source revision was reusable")
	}
	legacy := source
	if translationArtifactMatchesIdentity(legacy, identity) {
		t.Fatal("legacy non-translation artifact was treated as translated")
	}
}

func TestTranslatedSubtitleNamingIsLocaleQualifiedAndPreservesSourceNaming(t *testing.T) {
	source := SubtitleArtifact{ArtifactID: "subtitle:4:converted:srt", StreamIndex: 4, Mode: "converted", Format: "srt", Language: "eng", Forced: true}
	es419, err := newTranslationIdentity(translationIdentityTestInputWithLocale("es-419"))
	if err != nil {
		t.Fatal(err)
	}
	esES, err := newTranslationIdentity(translationIdentityTestInputWithLocale("es-ES"))
	if err != nil {
		t.Fatal(err)
	}
	used := map[string]struct{}{}
	sourcePath := resolvedSubtitleStagedPath("/staging/Movie", source, used)
	es419Path, err := resolvedTranslatedSubtitleStagedPath("/staging/Movie", source, es419.metadata(), used)
	if err != nil {
		t.Fatal(err)
	}
	esESPath, err := resolvedTranslatedSubtitleStagedPath("/staging/Movie", source, esES.metadata(), used)
	if err != nil {
		t.Fatal(err)
	}
	if sourcePath == es419Path || es419Path == esESPath {
		t.Fatalf("translation naming collided: source=%q es419=%q esES=%q", sourcePath, es419Path, esESPath)
	}
	if !strings.Contains(es419Path, ".es-419.eng.forced.srt") || !strings.Contains(esESPath, ".es-es.eng.forced.srt") {
		t.Fatalf("locale/disposition naming was not preserved: es419=%q esES=%q", es419Path, esESPath)
	}
}

func TestSubtitleTranslationMetadataRoundTripsAndLegacyArtifactsRemainReadable(t *testing.T) {
	identity, err := newTranslationIdentity(translationIdentityTestInput())
	if err != nil {
		t.Fatal(err)
	}
	metadata := identity.metadata()
	translated := SubtitleArtifact{ArtifactID: "translated", Type: "subtitle_sidecar", Format: "srt", Translation: &metadata}
	decoded := subtitleArtifactsFromJSON(subtitleArtifactsJSON([]SubtitleArtifact{translated}))
	if len(decoded) != 1 || decoded[0].Translation == nil || *decoded[0].Translation != metadata {
		t.Fatalf("translation metadata did not round-trip: %#v", decoded)
	}

	legacy := SubtitleArtifact{ArtifactID: "legacy", Type: "subtitle_sidecar", Format: "srt", Language: "eng"}
	legacyDecoded := subtitleArtifactsFromJSON(subtitleArtifactsJSON([]SubtitleArtifact{legacy}))
	if len(legacyDecoded) != 1 || legacyDecoded[0].Translation != nil || legacyDecoded[0].ArtifactID != legacy.ArtifactID {
		t.Fatalf("legacy artifact was not preserved: %#v", legacyDecoded)
	}
}

func translationIdentityTestInput() translationIdentityInput {
	return translationIdentityInput{
		SourceArtifactIdentity:  "/media/title.mkv#stream=4",
		SourceRevision:          "size=100;mtime=200;hash=abc",
		SourceRepresentation:    "pgs",
		SourceProvenance:        "canonical-bitmap-ocr-v1",
		RequestedSourceLanguage: "auto",
		EffectiveSourceLanguage: "en",
		TargetLocale:            "es-419",
		Provider:                "ollama",
		ProviderContractVersion: translationProviderContractVersion,
		ProviderModel:           "qwen2.5:7b",
		ContextContractVersion:  "translation-context-v1",
		ContextMode:             "none",
		OutputRepresentation:    "srt",
	}
}

func translationIdentityTestInputWithLocale(locale string) translationIdentityInput {
	input := translationIdentityTestInput()
	input.TargetLocale = locale
	return input
}
