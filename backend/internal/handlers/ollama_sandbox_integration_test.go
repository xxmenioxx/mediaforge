//go:build ollama_sandbox

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/airesource"
)

const (
	ollamaSandboxOCRModel         = "ocr-sandbox:latest"
	ollamaSandboxTranslationModel = "translate-sandbox:latest"
)

func TestOllamaSandboxBackendHealthAndExternalEgressBlocked(t *testing.T) {
	endpoint := requireOllamaSandboxEndpoint(t)
	apiURL := strings.TrimRight(os.Getenv("MVFORGE_API_URL"), "/")
	if apiURL == "" {
		t.Fatal("MVFORGE_API_URL must point to the isolated backend service")
	}
	response, err := http.Get(apiURL + "/health")
	if err != nil {
		t.Fatalf("isolated backend health endpoint is unreachable: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("isolated backend health status=%d, want %d", response.StatusCode, http.StatusOK)
	}
	parsed, _ := url.Parse(endpoint)
	if parsed.Host != "ollama-mock:11434" {
		t.Fatalf("test runner endpoint escaped the private mock service: %q", parsed.Host)
	}
	connection, err := net.DialTimeout("tcp", "1.1.1.1:443", 400*time.Millisecond)
	if err == nil {
		connection.Close()
		t.Fatal("sandbox private network unexpectedly reached an external endpoint")
	}
}

func TestOllamaSandboxTranslationSRTAndASSOverMockNetwork(t *testing.T) {
	endpoint := requireOllamaSandboxEndpoint(t)
	resetOllamaSandboxMock(t, endpoint)
	provider, err := newOllamaTranslationProvider(ollamaTestSettings(endpoint, ollamaSandboxTranslationModel))
	if err != nil {
		t.Fatal(err)
	}

	srtSource := []byte("1\n00:00:01,000 --> 00:00:02,000\nHello world\n\n2\n00:00:03,000 --> 00:00:04,000\nGood morning\n")
	srtRequest := subtitleTranslationOperationRequest{
		SourceIdentity: "sandbox-synthetic-srt", RequestedSourceLanguage: "en", TargetLocale: "es-419",
		RequestedRepresentation: "srt", Provider: translationProviderOllama, ProviderModel: ollamaSandboxTranslationModel,
	}
	srtResult, err := executeTextTranslation(context.Background(), subtitleTranslationSourceResponse{
		Representation: translationRepresentationSRT, Language: "en",
	}, srtSource, srtRequest, provider, defaultTranslationSettings(), nil, nil)
	if err != nil {
		t.Fatalf("synthetic SRT translation failed: %v", err)
	}
	if !bytes.Equal(srtSource, []byte("1\n00:00:01,000 --> 00:00:02,000\nHello world\n\n2\n00:00:03,000 --> 00:00:04,000\nGood morning\n")) {
		t.Fatal("SRT source changed during translation")
	}
	srtOutput := string(srtResult.Outputs["srt"])
	for _, expected := range []string{"00:00:01,000 --> 00:00:02,000", "Hola mundo", "00:00:03,000 --> 00:00:04,000", "Buenos días"} {
		if !strings.Contains(srtOutput, expected) {
			t.Fatalf("SRT output omitted %q: %q", expected, srtOutput)
		}
	}

	assSource := []byte("[Script Info]\nScriptType: v4.00+\n[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,{\\an8\\pos(10,20)\\i1}Hello world{\\i0}\nDialogue: 0,0:00:03.00,0:00:04.00,Default,Good morning\n")
	assRequest := srtRequest
	assRequest.SourceIdentity = "sandbox-synthetic-ass"
	assRequest.RequestedRepresentation = "ass"
	assResult, err := executeTextTranslation(context.Background(), subtitleTranslationSourceResponse{
		Representation: "ass", Language: "en",
	}, assSource, assRequest, provider, defaultTranslationSettings(), nil, nil)
	if err != nil {
		t.Fatalf("synthetic ASS translation failed: %v", err)
	}
	if !bytes.Equal(assSource, []byte("[Script Info]\nScriptType: v4.00+\n[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,{\\an8\\pos(10,20)\\i1}Hello world{\\i0}\nDialogue: 0,0:00:03.00,0:00:04.00,Default,Good morning\n")) {
		t.Fatal("ASS source changed during translation")
	}
	assOutput := string(assResult.Outputs["ass"])
	for _, expected := range []string{"0:00:01.00,0:00:02.00,Default,{\\an8\\pos(10,20)\\i1}Hola mundo{\\i0}", "0:00:03.00,0:00:04.00,Default,Buenos días"} {
		if !strings.Contains(assOutput, expected) {
			t.Fatalf("ASS output lost timing, translated text, or protected tags %q: %q", expected, assOutput)
		}
	}
	stats := readOllamaSandboxStats(t, endpoint)
	if stats.ChatRequests != 2 || stats.SchemaRequests != 2 || stats.MaxActiveChats != 1 {
		t.Fatalf("mock did not observe two schema-constrained, sequential requests: %#v", stats)
	}
	for _, model := range stats.ChatModels {
		if model != ollamaSandboxTranslationModel {
			t.Fatalf("Translation used unexpected mock model %q", model)
		}
	}
}

func TestOllamaSandboxTranslationRejectsPartialOutputAndRetriesAfterMalformedBatch(t *testing.T) {
	endpoint := requireOllamaSandboxEndpoint(t)
	resetOllamaSandboxMock(t, endpoint)
	setOllamaSandboxMockMode(t, endpoint, "malformed-at", 2)
	provider, err := newOllamaTranslationProvider(ollamaTestSettings(endpoint, ollamaSandboxTranslationModel))
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("1\n00:00:01,000 --> 00:00:02,000\nHello world\n\n2\n00:00:03,000 --> 00:00:04,000\nGood morning\n")
	request := subtitleTranslationOperationRequest{
		SourceIdentity: "sandbox-retry-srt", RequestedSourceLanguage: "en", TargetLocale: "es-419",
		RequestedRepresentation: "srt", Provider: translationProviderOllama, ProviderModel: ollamaSandboxTranslationModel,
	}
	settings := defaultTranslationSettings()
	settings.MaxCuesPerRequest = 1
	provider.maxCuesPerRequest = 1
	result, err := executeTextTranslation(context.Background(), subtitleTranslationSourceResponse{Representation: "srt", Language: "en"}, content, request, provider, settings, nil, nil)
	if err == nil || len(result.Outputs) != 0 {
		t.Fatalf("later malformed batch returned partial translated output: result=%#v err=%v", result, err)
	}
	if !bytes.Equal(content, []byte("1\n00:00:01,000 --> 00:00:02,000\nHello world\n\n2\n00:00:03,000 --> 00:00:04,000\nGood morning\n")) {
		t.Fatal("source subtitle changed after a rejected batch")
	}
	resetOllamaSandboxMock(t, endpoint)
	provider, err = newOllamaTranslationProvider(ollamaTestSettings(endpoint, ollamaSandboxTranslationModel))
	if err != nil {
		t.Fatal(err)
	}
	result, err = executeTextTranslation(context.Background(), subtitleTranslationSourceResponse{Representation: "srt", Language: "en"}, content, request, provider, settings, nil, nil)
	if err != nil || len(result.Outputs["srt"]) == 0 {
		t.Fatalf("safe retry after the rejected attempt failed: outputs=%v err=%v", len(result.Outputs), err)
	}
}

func TestOllamaSandboxTranslationCancellationReturnsNoOutput(t *testing.T) {
	endpoint := requireOllamaSandboxEndpoint(t)
	resetOllamaSandboxMock(t, endpoint)
	setOllamaSandboxMockMode(t, endpoint, "delay", 0)
	provider, err := newOllamaTranslationProvider(ollamaTestSettings(endpoint, ollamaSandboxTranslationModel))
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("1\n00:00:01,000 --> 00:00:02,000\nHello world\n")
	request := subtitleTranslationOperationRequest{
		SourceIdentity: "sandbox-cancel-srt", RequestedSourceLanguage: "en", TargetLocale: "es-419",
		RequestedRepresentation: "srt", Provider: translationProviderOllama, ProviderModel: ollamaSandboxTranslationModel,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	result, err := executeTextTranslation(ctx, subtitleTranslationSourceResponse{Representation: "srt", Language: "en"}, content, request, provider, defaultTranslationSettings(), nil, nil)
	if err == nil || len(result.Outputs) != 0 {
		t.Fatalf("canceled Translation returned output: outputs=%v err=%v", len(result.Outputs), err)
	}
}

func TestOllamaSandboxOCRColdStartUsesExplicitModelAndLeavesCanonicalSidecarUnchanged(t *testing.T) {
	endpoint := requireOllamaSandboxEndpoint(t)
	resetOllamaSandboxMock(t, endpoint)
	fixture := newCanonicalOCRReviewFixture(t, "Hello|world", 40)
	fixture.evidence.CorrectionProvenance = []canonicalOCRCorrectionProvenance{{
		CueID: 1, Action: "apply", CandidateSource: "contextual-ocr-dictionary", TextHash: boundedTextHash("Hello|world"),
		BeforeTextHash: boundedTextHash("Hello|world"), AfterTextHash: boundedTextHash("Hello|world"),
		DictionaryLanguage: "eng", DictionaryRevision: "sandbox-dictionary", OperationID: "sandbox-dictionary-operation",
	}}
	if err := commitPublishedCanonicalOCRSnapshot(context.Background(), fixture.sidecar, "srt", fixture.target.LogicalTransformKey, fixture.evidence, "sandbox-dictionary", ""); err != nil {
		t.Fatal(err)
	}
	selection, found, err := canonicalOCRTranslationSelectionForRequest(fixture.request)
	if err != nil || !found || selection.ReviewTarget == nil {
		t.Fatalf("canonical synthetic OCR target could not be reselected: found=%v err=%v", found, err)
	}
	fixture.target = selection.ReviewTarget
	db := newSubtitleOCRAITestDB(t, endpoint, true, ollamaSandboxOCRModel)
	admission := &subtitleOCRAITestAdmission{}
	manager := newAIModelLifecycle(db, admission, NewOllamaProviderHTTPClient(), true, "ollama-sandbox-integration")
	lifecycle := &AIModelLifecycle{manager: manager}
	before, err := os.ReadFile(fixture.sidecar)
	if err != nil {
		t.Fatal(err)
	}
	report := newSubtitleOCRAIFixtureReport(t, fixture)
	report, err = enrichSubtitleOCRReportWithAI(context.Background(), "sandbox-ocr-review", report, fixture.target, db, lifecycle, NewOllamaProviderHTTPClient())
	if err != nil {
		t.Fatalf("mock-backed OCR review failed: %v", err)
	}
	if report.OCRAI == nil || report.OCRAI.State != "suggestions_ready" || report.OCRAI.Model != ollamaSandboxOCRModel || report.OCRAI.SuggestedCount != 1 {
		cues, _ := subtitleOCRAIReviewCues(report, fixture.target)
		provider, providerErr := newOllamaTranslationProviderWithClient(translationSettings{
			Provider: translationProviderOllama, OllamaEndpoint: endpoint, OllamaModel: ollamaSandboxOCRModel,
			MaxCuesPerRequest: subtitleProofreadingMaxBatchCues, MaxRequestBytes: subtitleProofreadingMaxRequestBody,
		}, NewOllamaProviderHTTPClient())
		var proofreadErr error
		if providerErr == nil {
			_, proofreadErr = provider.Proofread(context.Background(), subtitleProofreadingLanguage(fixture.target.Identity.RequestedLanguage, cues), cues)
		}
		t.Fatalf("OCR review did not produce a revision-bound suggestion: report=%#v providerError=%v proofreadError=%v mock=%#v", report.OCRAI, providerErr, proofreadErr, readOllamaSandboxStats(t, endpoint))
	}
	suggestion := report.Findings[0].AISuggestion
	if suggestion == nil || suggestion.Model != ollamaSandboxOCRModel || suggestion.CueID != 1 || suggestion.OriginalHash != subtitleProofreadingTextHash("Hello|world") {
		t.Fatalf("OCR suggestion lost cue, revision, or model identity: %#v", suggestion)
	}
	if suggestion.ProposedText != "Hello world" {
		t.Fatalf("mock did not return a conservative dictionary-compatible suggestion: %q", suggestion.ProposedText)
	}
	after, err := os.ReadFile(fixture.sidecar)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("OCR AI suggestion automatically changed the canonical sidecar")
	}
	admission.mu.Lock()
	requests := append([]airesource.Request(nil), admission.requests...)
	releases := admission.releases
	admission.mu.Unlock()
	if len(requests) != 2 || releases != 2 {
		t.Fatalf("cold start/inference admission leases were not balanced: requests=%#v releases=%d", requests, releases)
	}
	if requests[0].Purpose != airesource.PurposeModelLoad || requests[1].Purpose != airesource.PurposeInference {
		t.Fatalf("expected warmup admission followed by inference admission: %#v", requests)
	}
	for _, request := range requests {
		if request.ModelID != ollamaSandboxOCRModel || request.ConsumerID != aiModelConsumerSubtitleOCR {
			t.Fatalf("OCR resource admission used an implicit model or wrong consumer: %#v", request)
		}
	}
	stats := readOllamaSandboxStats(t, endpoint)
	if stats.WarmupRequests != 1 || stats.ChatRequests != 1 || !stats.Resident || stats.MaxActiveChats != 1 {
		t.Fatalf("mock did not observe cold-start then a single inference: %#v", stats)
	}
	if len(stats.ChatModels) != 1 || stats.ChatModels[0] != ollamaSandboxOCRModel {
		t.Fatalf("OCR AI did not use only its selected model: %#v", stats.ChatModels)
	}
}

func requireOllamaSandboxEndpoint(t *testing.T) string {
	t.Helper()
	endpoint := strings.TrimSpace(os.Getenv("OLLAMA_SANDBOX_ENDPOINT"))
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "http" || parsed.Host != "ollama-mock:11434" || parsed.User != nil || parsed.RawQuery != "" {
		t.Fatalf("OLLAMA_SANDBOX_ENDPOINT must be exactly the private mock service, got %q", endpoint)
	}
	return endpoint
}

func resetOllamaSandboxMock(t *testing.T, endpoint string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, endpoint+"/__admin/reset", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("reset synthetic provider state: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("reset mock status=%d", response.StatusCode)
	}
}

func setOllamaSandboxMockMode(t *testing.T, endpoint, mode string, failAt int) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"mode": mode, "failAt": failAt})
	request, err := http.NewRequest(http.MethodPost, endpoint+"/__admin/mode", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("configure synthetic provider mode: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("set mock mode status=%d", response.StatusCode)
	}
}

type ollamaSandboxStats struct {
	Resident       bool     `json:"resident"`
	ChatRequests   int      `json:"chatRequests"`
	WarmupRequests int      `json:"warmupRequests"`
	PSRequests     int      `json:"psRequests"`
	TagsRequests   int      `json:"tagsRequests"`
	ShowRequests   int      `json:"showRequests"`
	SchemaRequests int      `json:"schemaRequests"`
	MaxActiveChats int      `json:"maxActiveChats"`
	ChatModels     []string `json:"chatModels"`
}

func readOllamaSandboxStats(t *testing.T, endpoint string) ollamaSandboxStats {
	t.Helper()
	response, err := http.Get(endpoint + "/__admin/stats")
	if err != nil {
		t.Fatalf("read synthetic provider counters: %v", err)
	}
	defer response.Body.Close()
	var stats ollamaSandboxStats
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&stats) != nil {
		t.Fatalf("synthetic provider counters were unavailable (HTTP %d)", response.StatusCode)
	}
	return stats
}

func TestOllamaSandboxFailureMessageDoesNotLeakSyntheticPrompt(t *testing.T) {
	endpoint := requireOllamaSandboxEndpoint(t)
	resetOllamaSandboxMock(t, endpoint)
	setOllamaSandboxMockMode(t, endpoint, "http-error", 0)
	provider, err := newOllamaTranslationProvider(ollamaTestSettings(endpoint, ollamaSandboxTranslationModel))
	if err != nil {
		t.Fatal(err)
	}
	secretMarker := "SYNTHETIC-PRIVATE-MARKER"
	result, err := provider.Translate(context.Background(), TranslationBatch{
		ContractVersion: "translation-batch-v1", SourceLanguage: "en", TargetLocale: "es-419",
		Cues: []TranslationCue{{ID: "cue-1", Text: secretMarker}},
	})
	if err == nil || len(result.Cues) != 0 || strings.Contains(err.Error(), secretMarker) {
		t.Fatalf("provider failure leaked prompt data or returned cues: result=%#v err=%v", result, err)
	}
}
