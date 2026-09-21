package handlers

import (
	"strings"
	"testing"
)

func TestValidateTranslationBatchResultAcceptsExactOrderedCues(t *testing.T) {
	batch := translationValidationTestBatch()
	result := TranslationBatchResult{
		ContractVersion: translationProviderContractVersion,
		Cues: []TranslatedCue{
			{ID: "cue-1", Text: "Hola"},
			{ID: "cue-2", Text: "Mundo"},
		},
	}

	validated, err := validateTranslationBatchResult(batch, result)
	if err != nil {
		t.Fatalf("valid result rejected: %v", err)
	}
	if len(validated) != 2 || validated[0].ID != "cue-1" || validated[1].ID != "cue-2" {
		t.Fatalf("unexpected validated result: %#v", validated)
	}
}

func TestValidateTranslationBatchResultRejectsMalformedOwnership(t *testing.T) {
	cases := []struct {
		name   string
		result TranslationBatchResult
	}{
		{
			name: "wrong contract version",
			result: TranslationBatchResult{
				ContractVersion: "translation-provider-v0",
				Cues:            []TranslatedCue{{ID: "cue-1", Text: "Hola"}, {ID: "cue-2", Text: "Mundo"}},
			},
		},
		{
			name: "missing cue",
			result: TranslationBatchResult{
				ContractVersion: translationProviderContractVersion,
				Cues:            []TranslatedCue{{ID: "cue-1", Text: "Hola"}},
			},
		},
		{
			name: "duplicate cue ID",
			result: TranslationBatchResult{
				ContractVersion: translationProviderContractVersion,
				Cues:            []TranslatedCue{{ID: "cue-1", Text: "Hola"}, {ID: "cue-1", Text: "Mundo"}},
			},
		},
		{
			name: "unexpected cue ID",
			result: TranslationBatchResult{
				ContractVersion: translationProviderContractVersion,
				Cues:            []TranslatedCue{{ID: "cue-1", Text: "Hola"}, {ID: "cue-3", Text: "Mundo"}},
			},
		},
		{
			name: "reordered cue IDs",
			result: TranslationBatchResult{
				ContractVersion: translationProviderContractVersion,
				Cues:            []TranslatedCue{{ID: "cue-2", Text: "Mundo"}, {ID: "cue-1", Text: "Hola"}},
			},
		},
		{
			name: "split cue ownership",
			result: TranslationBatchResult{
				ContractVersion: translationProviderContractVersion,
				Cues:            []TranslatedCue{{ID: "cue-1", Text: "Hola"}, {ID: "cue-2", Text: "Mundo"}, {ID: "cue-3", Text: "Extra"}},
			},
		},
		{
			name: "empty cue ID",
			result: TranslationBatchResult{
				ContractVersion: translationProviderContractVersion,
				Cues:            []TranslatedCue{{ID: "", Text: "Hola"}, {ID: "cue-2", Text: "Mundo"}},
			},
		},
		{
			name: "empty translated text",
			result: TranslationBatchResult{
				ContractVersion: translationProviderContractVersion,
				Cues:            []TranslatedCue{{ID: "cue-1", Text: "  "}, {ID: "cue-2", Text: "Mundo"}},
			},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			validated, err := validateTranslationBatchResult(translationValidationTestBatch(), test.result)
			if err == nil {
				t.Fatal("malformed result accepted")
			}
			if validated != nil {
				t.Fatalf("partial validated result returned: %#v", validated)
			}
			if translationProviderErrorClassOf(err) != translationProviderErrorMalformed {
				t.Fatalf("error class=%q, want malformed", translationProviderErrorClassOf(err))
			}
		})
	}
}

func TestDecodeTranslationBatchResultStrictlyRejectsProviderFields(t *testing.T) {
	cases := []struct {
		name    string
		payload string
	}{
		{
			name:    "timing field",
			payload: `{"contractVersion":"translation-provider-v1","cues":[{"id":"cue-1","text":"Hola","start":1.0}],"metrics":{}}`,
		},
		{
			name:    "ordering field",
			payload: `{"contractVersion":"translation-provider-v1","cues":[{"id":"cue-1","text":"Hola","order":1}],"metrics":{}}`,
		},
		{
			name:    "structural field",
			payload: `{"contractVersion":"translation-provider-v1","cues":[{"id":"cue-1","text":"Hola","tags":"{\\i1}"}],"metrics":{}}`,
		},
		{
			name:    "extra result field",
			payload: `{"contractVersion":"translation-provider-v1","cues":[{"id":"cue-1","text":"Hola"}],"geometry":{},"metrics":{}}`,
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := decodeTranslationBatchResult([]byte(test.payload)); err == nil {
				t.Fatal("provider field accepted")
			} else if translationProviderErrorClassOf(err) != translationProviderErrorMalformed {
				t.Fatalf("error class=%q, want malformed", translationProviderErrorClassOf(err))
			}
		})
	}
}

func TestDecodeAndValidateTranslationBatchResultAcceptsSRTText(t *testing.T) {
	payload := []byte(`{"contractVersion":"translation-provider-v1","cues":[{"id":"cue-1","text":"Hola"},{"id":"cue-2","text":"Mundo"}],"metrics":{"provider":"ollama","requestCount":1}}`)
	result, err := decodeTranslationBatchResult(payload)
	if err != nil {
		t.Fatalf("valid provider response rejected: %v", err)
	}
	validated, err := validateTranslationBatchResult(translationValidationTestBatch(), result)
	if err != nil {
		t.Fatalf("valid SRT text rejected: %v", err)
	}
	if validated[0].Text != "Hola" || validated[1].Text != "Mundo" {
		t.Fatalf("unexpected validated text: %#v", validated)
	}
}

func TestValidateTranslationASSBatchResultReusesProtectedStructureValidation(t *testing.T) {
	document := translationValidationTestASSDocument(t)
	cues := document.TranslationSourceCues("asset.mkv#stream=4", "")
	envelopes := translationCueEnvelopesForSourceCues("asset.mkv#stream=4", "ass", "", cues)
	batch := translationBatchForCueEnvelopes("en", "es-419", envelopes, nil)
	providerText := batch.Cues[0].Text
	providerText = replaceFirstTranslationText(providerText, "Hello", "Hola")
	result := TranslationBatchResult{
		ContractVersion: translationProviderContractVersion,
		Cues:            []TranslatedCue{{ID: batch.Cues[0].ID, Text: providerText}},
	}

	validated, err := validateTranslationASSBatchResult(document, batch, result)
	if err != nil {
		t.Fatalf("valid protected response rejected: %v", err)
	}
	output, err := document.ReconstructDialogueProviderTexts(map[int]string{
		document.Events[0].SourceOrdinal: validated[0].Text,
	})
	if err != nil {
		t.Fatalf("validated response could not be reconstructed: %v", err)
	}
	if !containsTranslationStructure(output) || !containsTranslationText(output, "Hola") {
		t.Fatalf("protected ASS structure was not preserved: %q", output)
	}
}

func TestValidateTranslationASSBatchResultRejectsProtectedAnchorAndStructureMutation(t *testing.T) {
	document := translationValidationTestASSDocument(t)
	cues := document.TranslationSourceCues("asset.mkv#stream=4", "")
	envelopes := translationCueEnvelopesForSourceCues("asset.mkv#stream=4", "ass", "", cues)
	batch := translationBatchForCueEnvelopes("en", "es-419", envelopes, nil)
	providerText := batch.Cues[0].Text
	anchors := translationValidationProtectedAnchors(document.Events[0].Dialogue)
	if len(anchors) < 2 {
		t.Fatalf("fixture needs multiple protected anchors: %#v", anchors)
	}

	reordered := strings.Replace(providerText, anchors[0], "__first_anchor__", 1)
	reordered = strings.Replace(reordered, anchors[len(anchors)-1], anchors[0], 1)
	reordered = strings.Replace(reordered, "__first_anchor__", anchors[len(anchors)-1], 1)
	cases := []struct {
		name string
		text string
	}{
		{name: "missing anchor", text: strings.Replace(providerText, anchors[0], "", 1)},
		{name: "duplicate anchor", text: strings.Replace(providerText, anchors[0], anchors[0]+anchors[0], 1)},
		{name: "reordered anchor", text: reordered},
		{name: "provider tag", text: providerText + "{\\i1}"},
		{name: "provider positioning", text: providerText + "{\\pos(10,20)}"},
		{name: "provider line break", text: providerText + `\N`},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result := TranslationBatchResult{
				ContractVersion: translationProviderContractVersion,
				Cues:            []TranslatedCue{{ID: batch.Cues[0].ID, Text: test.text}},
			}
			validated, err := validateTranslationASSBatchResult(document, batch, result)
			if err == nil {
				t.Fatal("structurally mutated provider result accepted")
			}
			if validated != nil {
				t.Fatalf("partial validated result returned: %#v", validated)
			}
		})
	}
}

func translationValidationTestBatch() TranslationBatch {
	return TranslationBatch{
		ContractVersion: translationProviderContractVersion,
		SourceLanguage:  "en",
		TargetLocale:    "es-419",
		Cues: []TranslationCue{
			{ID: "cue-1", Text: "Hello"},
			{ID: "cue-2", Text: "World"},
		},
	}
}

func translationValidationTestASSDocument(t *testing.T) translationASSDocument {
	t.Helper()
	source := `[Script Info]
Title: unchanged

[V4+ Styles]
Format: Name, Fontname, Fontsize
Style: Default,Arial,20

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 2,0:00:01.00,0:00:03.00,Default,Actor,010,020,030,fx,{\an8\pos(100,200)\k20}Hello, comma\Nworld{\i1}!
`
	document, err := parseTranslationASSDocument("ass", source)
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func translationValidationProtectedAnchors(dialogue translationProtectedDialogue) []string {
	anchors := make([]string, 0)
	for _, part := range dialogue.Parts {
		if part.Protected {
			anchors = append(anchors, part.Anchor)
		}
	}
	return anchors
}

func replaceFirstTranslationText(text, from, to string) string {
	return strings.Replace(text, from, to, 1)
}

func containsTranslationStructure(text string) bool {
	return strings.Contains(text, `{\an8\pos(100,200)\k20}`) && strings.Contains(text, `\N`) && strings.Contains(text, `{\i1}`)
}

func containsTranslationText(text, expected string) bool {
	return strings.Contains(text, expected)
}
