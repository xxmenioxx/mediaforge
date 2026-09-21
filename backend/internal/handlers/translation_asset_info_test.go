package handlers

import (
	"context"
	"strings"
	"testing"
)

type assetInfoTranslationProviderStub struct {
	calls int
	fn    func(TranslationBatch) TranslationBatchResult
}

func (provider *assetInfoTranslationProviderStub) Translate(_ context.Context, batch TranslationBatch) (TranslationBatchResult, error) {
	provider.calls++
	return provider.fn(batch), nil
}

func (provider *assetInfoTranslationProviderStub) resultFor(batch TranslationBatch) TranslationBatchResult {
	result := TranslationBatchResult{ContractVersion: batch.ContractVersion}
	for _, cue := range batch.Cues {
		result.Cues = append(result.Cues, TranslatedCue{ID: cue.ID, Text: strings.Replace(cue.Text, "Hello", "Hola", 1)})
	}
	return result
}

func TestTranslationCapabilityAvailabilityIsBackendAuthoritative(t *testing.T) {
	base := defaultTranslationSettings()
	base.Provider = translationProviderOllama
	base.OllamaEndpoint = "http://nas:11434"
	base.OllamaModel = "llama3"
	cases := []struct {
		name      string
		mutate    func(*translationSettings)
		available bool
		reason    string
	}{
		{name: "not configured", mutate: func(settings *translationSettings) { settings.Provider = "" }, reason: "Translation provider is not configured."},
		{name: "untested", mutate: func(settings *translationSettings) { settings.Readiness = translationReadinessUntested }, reason: "Translation provider configuration must be tested."},
		{name: "disabled", mutate: func(settings *translationSettings) { settings.Readiness = translationReadinessDisabled }, reason: "Subtitle translation is disabled."},
		{name: "ready and enabled", mutate: func(settings *translationSettings) {
			settings.Readiness = translationReadinessReady
			settings.Enabled = true
			settings.ReadinessIdentity = translationProviderConfigurationIdentity(*settings)
		}, available: true},
		{name: "stale readiness", mutate: func(settings *translationSettings) {
			settings.Readiness = translationReadinessReady
			settings.Enabled = true
			settings.ReadinessIdentity = "stale"
		}, reason: "Translation provider configuration must be tested."},
		{name: "model unavailable", mutate: func(settings *translationSettings) { settings.Readiness = translationReadinessModelNotFound }, reason: "Configured translation model is unavailable."},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			settings := base
			testCase.mutate(&settings)
			available, reason := translationCapabilityAvailability(settings)
			if available != testCase.available || reason != testCase.reason {
				t.Fatalf("availability = %v, %q; want %v, %q", available, reason, testCase.available, testCase.reason)
			}
		})
	}
}

func TestRequestedTranslationOutputPreservesTextAndBitmapRules(t *testing.T) {
	for _, testCase := range []struct {
		source, requested, want string
	}{
		{source: "srt", requested: "", want: "srt"},
		{source: "ass", requested: "ass", want: "ass"},
		{source: "ssa", requested: "ssa", want: "ssa"},
		{source: "bitmap", requested: "", want: "srt"},
		{source: "bitmap", requested: "both", want: "both"},
	} {
		got, err := requestedTranslationOutput(testCase.source, testCase.requested)
		if err != nil || got != testCase.want {
			t.Fatalf("requestedTranslationOutput(%q, %q) = %q, %v; want %q", testCase.source, testCase.requested, got, err, testCase.want)
		}
	}
	if _, err := requestedTranslationOutput("srt", "ass"); err == nil {
		t.Fatal("expected text representation change to be rejected")
	}
	if _, err := requestedTranslationOutput("bitmap", "ssa"); err == nil {
		t.Fatal("expected bitmap SSA output to be rejected")
	}
}

func TestExecuteTextTranslationSRTUsesExactCueContract(t *testing.T) {
	provider := &assetInfoTranslationProviderStub{}
	provider.fn = provider.resultFor
	settings := defaultTranslationSettings()
	source := subtitleTranslationSourceResponse{
		Type:           "external",
		SourceIdentity: "external:/media/movie.eng.srt:revision",
		SourceRevision: "revision",
		Representation: "srt",
		ExternalPath:   "/media/movie.eng.srt",
		Codec:          "srt",
		Language:       "eng",
	}
	result, err := executeTextTranslation(context.Background(), source, []byte("1\n00:00:01,000 --> 00:00:02,000\nHello\n\n"), subtitleTranslationOperationRequest{
		RequestedSourceLanguage: "auto",
		TargetLocale:            "es-419",
		RequestedRepresentation: "srt",
		SourceIdentity:          source.SourceIdentity,
	}, provider, settings, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 || !strings.Contains(string(result.Outputs["srt"]), "Hola") || strings.Contains(string(result.Outputs["srt"]), "Hello") {
		t.Fatalf("unexpected SRT result: calls=%d output=%q", provider.calls, result.Outputs["srt"])
	}
	if !strings.Contains(string(result.Outputs["srt"]), "00:00:01,000 --> 00:00:02,000") {
		t.Fatalf("translated SRT lost canonical timing: %q", result.Outputs["srt"])
	}
}

func TestExecuteTextTranslationASSAndSSAKeepProtectedStructure(t *testing.T) {
	fixtures := []struct {
		name, representation, format string
	}{
		{name: "ASS", representation: "ass", format: "Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text"},
		{name: "SSA", representation: "ssa", format: "Marked, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text"},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			provider := &assetInfoTranslationProviderStub{}
			provider.fn = provider.resultFor
			settings := defaultTranslationSettings()
			source := subtitleTranslationSourceResponse{
				Type: "external", SourceIdentity: "external:/media/movie." + fixture.representation,
				SourceRevision: "revision", Representation: fixture.representation,
				ExternalPath: "/media/movie." + fixture.representation, Codec: fixture.representation,
				Language: "eng",
			}
			content := "[Script Info]\nTitle: Fixture\n\n[Events]\nFormat: " + fixture.format + "\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,{\\an8}Hello{\\pos(10,20)}\n"
			result, err := executeTextTranslation(context.Background(), source, []byte(content), subtitleTranslationOperationRequest{
				RequestedSourceLanguage: "en", TargetLocale: "es-ES", RequestedRepresentation: fixture.representation, SourceIdentity: source.SourceIdentity,
			}, provider, settings, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			output := string(result.Outputs[fixture.representation])
			if !strings.Contains(output, "{\\an8}Hola{\\pos(10,20)}") || !strings.Contains(output, "Format: "+fixture.format) {
				t.Fatalf("protected %s structure was not preserved: %q", fixture.name, output)
			}
		})
	}
}

func TestTranslationCapabilityOutputsAreBackendSelected(t *testing.T) {
	cases := map[string][]string{
		"srt":    {"srt"},
		"ass":    {"ass"},
		"ssa":    {"ssa"},
		"bitmap": {"srt", "ass", "both"},
	}
	for source, want := range cases {
		got := translationCapabilityOutputs(source)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("capability outputs for %s = %v; want %v", source, got, want)
		}
	}
}
