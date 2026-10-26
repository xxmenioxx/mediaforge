package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type ollamaTestRoundTripper func(*http.Request) (*http.Response, error)

func (roundTripper ollamaTestRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTripper(request)
}

func TestOllamaDurationMillis(t *testing.T) {
	oldFalseOverflowThreshold := math.MaxInt64 / int64(time.Millisecond)
	tests := []struct {
		name        string
		nanoseconds int64
		wantMillis  int64
	}{
		{name: "zero", nanoseconds: 0, wantMillis: 0},
		{name: "negative", nanoseconds: -1, wantMillis: 0},
		{name: "twenty-five milliseconds", nanoseconds: 25_000_000, wantMillis: 25},
		{
			name:        "above old false overflow threshold",
			nanoseconds: oldFalseOverflowThreshold + int64(time.Millisecond),
			wantMillis:  (oldFalseOverflowThreshold + int64(time.Millisecond)) / int64(time.Millisecond),
		},
		{name: "maximum int64", nanoseconds: math.MaxInt64, wantMillis: math.MaxInt64 / int64(time.Millisecond)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ollamaDurationMillis(test.nanoseconds); got != test.wantMillis {
				t.Fatalf("ollamaDurationMillis(%d)=%d, want %d", test.nanoseconds, got, test.wantMillis)
			}
		})
	}
}

func TestOllamaTranslationPromptMapsKoreanAndFrenchToSpanish(t *testing.T) {
	for _, testCase := range []struct {
		name, sourceLanguage, canonicalSource, targetLocale, wantLanguageLabel string
	}{
		{name: "Korean bibliographic alias", sourceLanguage: "kor", canonicalSource: "ko", targetLocale: "es-419", wantLanguageLabel: "Korean (ko) to Spanish (Latin America, es-419; es)"},
		{name: "French legacy alias", sourceLanguage: "fre", canonicalSource: "fr", targetLocale: "es-ES", wantLanguageLabel: "French (fr) to Spanish (Spain, es-ES; es)"},
	} {
		batch := TranslationBatch{
			ContractVersion: translationProviderContractVersion,
			SourceLanguage:  testCase.sourceLanguage,
			TargetLocale:    testCase.targetLocale,
			Cues:            []TranslationCue{{ID: "cue-1", Text: "source"}},
		}
		prompt, err := ollamaTranslationPrompt(batch)
		if err != nil {
			t.Fatalf("%s prompt failed: %v", testCase.name, err)
		}
		if !strings.Contains(prompt, testCase.wantLanguageLabel) || !strings.Contains(prompt, `"SourceLanguage":"`+testCase.canonicalSource+`"`) || !strings.Contains(prompt, `"TargetLocale":"`+testCase.targetLocale+`"`) {
			t.Fatalf("%s prompt did not preserve canonical language pair: %s", testCase.name, prompt)
		}
	}
}

func TestOllamaTranslationProviderRejectsUnverifiedKoreanModelBeforeRequest(t *testing.T) {
	requests := 0
	client := &http.Client{Transport: ollamaTestRoundTripper(func(request *http.Request) (*http.Response, error) {
		requests++
		return nil, errors.New("unexpected Ollama request")
	})}
	settings := translationSettings{Provider: translationProviderOllama, OllamaEndpoint: "http://ollama.invalid", OllamaModel: "llama3"}
	provider, err := newOllamaTranslationProviderWithClient(settings, client)
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Translate(context.Background(), TranslationBatch{
		ContractVersion: translationProviderContractVersion,
		SourceLanguage:  "ko",
		TargetLocale:    "es-419",
		Cues:            []TranslationCue{{ID: "cue-1", Text: "안녕하세요"}},
	})
	if err == nil || translationProviderErrorClassOf(err) != translationProviderErrorConfiguration || !strings.Contains(err.Error(), "TranslateGemma") {
		t.Fatalf("unverified model did not fail with a capability diagnostic: %v", err)
	}
	if requests != 0 {
		t.Fatalf("unverified language request reached Ollama: %d calls", requests)
	}
}

func TestOllamaTranslationProviderUsesConfiguredChatEndpointAndContract(t *testing.T) {
	for _, endpointSuffix := range []string{"", "/", "/ollama"} {
		t.Run(fmt.Sprintf("endpoint-%q", endpointSuffix), func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				wantPath := "/api/chat"
				if endpointSuffix == "/ollama" {
					wantPath = "/ollama/api/chat"
				}
				if r.URL.Path != wantPath {
					t.Errorf("path=%q, want %q", r.URL.Path, wantPath)
				}
				var request struct {
					Model    string `json:"model"`
					Messages []struct {
						Role    string `json:"role"`
						Content string `json:"content"`
					} `json:"messages"`
					Stream    bool            `json:"stream"`
					Format    json.RawMessage `json:"format"`
					KeepAlive string          `json:"keep_alive"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Errorf("decode request: %v", err)
					return
				}
				if request.Model != "qwen2.5:7b" || request.Stream || request.KeepAlive != "30m" || len(request.Messages) != 1 || request.Messages[0].Role != "user" {
					t.Errorf("unexpected Ollama request envelope: %#v", request)
				}
				var schema map[string]any
				if err := json.Unmarshal(request.Format, &schema); err != nil {
					t.Errorf("decode structured-output schema: %v", err)
				} else {
					properties, ok := schema["properties"].(map[string]any)
					version, versionOK := properties["contractVersion"].(map[string]any)
					cues, cuesOK := properties["cues"].(map[string]any)
					cueProperties, cuePropertiesOK := cues["items"].(map[string]any)["properties"].(map[string]any)
					idSchema, idOK := cueProperties["id"].(map[string]any)
					idEnum, enumOK := idSchema["enum"].([]any)
					if schema["type"] != "object" || schema["additionalProperties"] != false || !ok || !versionOK || version["const"] != translationProviderContractVersion || !cuesOK || cues["minItems"] != float64(1) || cues["maxItems"] != float64(1) || !cuePropertiesOK || !idOK || !enumOK || len(idEnum) != 1 || idEnum[0] != "cue-1" {
						t.Errorf("Ollama schema did not bind the response to the current contract and cue: %#v", schema)
					}
				}
				content := request.Messages[0].Content
				for _, expected := range []string{"translation-provider-v1", "en", "es-419", "cue-1", "Hello", "nearby", "Title"} {
					if !strings.Contains(content, expected) {
						t.Errorf("request content missing %q: %s", expected, content)
					}
				}
				for _, forbidden := range []string{"sourceRevision", "Timing", "Geometry", "media/path.mkv", "Authorization"} {
					if strings.Contains(content, forbidden) {
						t.Errorf("request content exposed %q: %s", forbidden, content)
					}
				}
				writeOllamaTestResponse(t, w, `{"ContractVersion":"translation-provider-v1","Cues":[{"ID":"cue-1","Text":"Hola"}]}`, 12, 7, 25_000_000)
			}))
			defer server.Close()
			settings := ollamaTestSettings(server.URL+endpointSuffix, "qwen2.5:7b")
			provider, err := newOllamaTranslationProvider(settings)
			if err != nil {
				t.Fatal(err)
			}
			batch := TranslationBatch{
				ContractVersion: translationProviderContractVersion,
				SourceLanguage:  "en",
				TargetLocale:    "es-419",
				Cues:            []TranslationCue{{ID: "cue-1", Text: "Hello"}},
				Context: &TranslationContext{
					NeighboringCues: []TranslationContextCue{{ID: "nearby", Text: "Earlier"}},
					Media:           TranslationMediaContext{Title: "Title"},
				},
			}
			result, err := provider.Translate(context.Background(), batch)
			if err != nil {
				t.Fatal(err)
			}
			if requests != 1 || len(result.Cues) != 1 || result.Cues[0].ID != "cue-1" || result.Cues[0].Text != "Hola" {
				t.Fatalf("unexpected request/result: requests=%d result=%#v", requests, result)
			}
			if result.Metrics.Provider != translationProviderOllama || result.Metrics.Model != "qwen2.5:7b" || result.Metrics.RequestCount != 1 || result.Metrics.InputTokens != 12 || result.Metrics.OutputTokens != 7 || result.Metrics.LatencyMillis != 25 {
				t.Fatalf("unexpected mapped metrics: %#v", result.Metrics)
			}
		})
	}
}

func TestOllamaTranslationProviderPreservesSyntheticBatchSizesAndOrder(t *testing.T) {
	for _, cueCount := range []int{4, 8, 16, 32} {
		t.Run(fmt.Sprintf("%d-cues", cueCount), func(t *testing.T) {
			batch := ollamaSyntheticBatch(cueCount)
			responseBody, err := ollamaSyntheticResponseForBatch(batch)
			if err != nil {
				t.Fatal(err)
			}
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				var request struct {
					Format map[string]any `json:"format"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Errorf("decode synthetic request: %v", err)
					return
				}
				properties, ok := request.Format["properties"].(map[string]any)
				cuesSchema, cuesOK := properties["cues"].(map[string]any)
				items, itemsOK := cuesSchema["items"].(map[string]any)
				itemProperties, itemPropertiesOK := items["properties"].(map[string]any)
				idSchema, idOK := itemProperties["id"].(map[string]any)
				ids, idsOK := idSchema["enum"].([]any)
				if !ok || !cuesOK || !itemsOK || !itemPropertiesOK || !idOK || !idsOK || cuesSchema["minItems"] != float64(cueCount) || cuesSchema["maxItems"] != float64(cueCount) || len(ids) != cueCount {
					t.Errorf("schema did not bind the %d-cue batch: %#v", cueCount, request.Format)
					return
				}
				for index, cue := range batch.Cues {
					if ids[index] != cue.ID {
						t.Errorf("schema cue ID at %d=%v, want current batch ID", index, ids[index])
						return
					}
				}
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write(responseBody); err != nil {
					t.Errorf("write synthetic response: %v", err)
				}
			}))
			defer server.Close()

			provider, err := newOllamaTranslationProvider(ollamaTestSettings(server.URL, "qwen2.5:7b"))
			if err != nil {
				t.Fatal(err)
			}
			result, err := provider.Translate(context.Background(), batch)
			if err != nil {
				t.Fatal(err)
			}
			validated, err := validateTranslationBatchResult(batch, result)
			if err != nil {
				t.Fatal(err)
			}
			if requests != 1 || len(validated) != cueCount {
				t.Fatalf("request count=%d result cue count=%d, want one request and %d cues", requests, len(validated), cueCount)
			}
			for index, cue := range batch.Cues {
				if validated[index].ID != cue.ID || validated[index].Text != fmt.Sprintf("Hola, subtitulo sintetico %d.", index+1) {
					t.Fatalf("cue %d changed identity/order or translation: got=%#v", index, validated[index])
				}
			}
		})
	}
}

func BenchmarkOllamaTranslationProviderSyntheticBatchSizes(b *testing.B) {
	for _, cueCount := range []int{4, 8, 16, 32} {
		b.Run(fmt.Sprintf("%d-cues", cueCount), func(b *testing.B) {
			batch := ollamaSyntheticBatch(cueCount)
			responseBody, err := ollamaSyntheticResponseForBatch(batch)
			if err != nil {
				b.Fatal(err)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if _, err := w.Write(responseBody); err != nil {
					b.Errorf("write synthetic response: %v", err)
				}
			}))
			defer server.Close()
			provider, err := newOllamaTranslationProvider(ollamaTestSettings(server.URL, "qwen2.5:7b"))
			if err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()
			b.ReportMetric(float64(cueCount), "cues/op")
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				result, err := provider.Translate(context.Background(), batch)
				if err != nil {
					b.Fatal(err)
				}
				if len(result.Cues) != cueCount {
					b.Fatalf("result cue count=%d, want %d", len(result.Cues), cueCount)
				}
			}
		})
	}
}

func TestOllamaTranslationRequestDeadlineUsesEarlierOperationDeadline(t *testing.T) {
	batch := ollamaTestBatch()
	responseBody, err := ollamaSyntheticResponseForBatch(batch)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name                   string
		operationDeadlineAfter time.Duration
	}{
		{name: "request cap"},
		{name: "earlier operation deadline", operationDeadlineAfter: 2 * time.Minute},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var wantOperationDeadline time.Time
			ctx := context.Background()
			if test.operationDeadlineAfter > 0 {
				wantOperationDeadline = time.Now().Add(test.operationDeadlineAfter)
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, wantOperationDeadline)
				defer cancel()
			}
			observedDeadline := make(chan time.Time, 1)
			client := &http.Client{
				Timeout: 10 * time.Minute,
				Transport: ollamaTestRoundTripper(func(request *http.Request) (*http.Response, error) {
					deadline, ok := request.Context().Deadline()
					if !ok {
						return nil, errors.New("translation request has no deadline")
					}
					observedDeadline <- deadline
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       io.NopCloser(bytes.NewReader(responseBody)),
						Request:    request,
					}, nil
				}),
			}
			provider, err := newOllamaTranslationProviderWithClient(ollamaTestSettings("http://ollama.test", "qwen2.5:7b"), client)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := provider.Translate(ctx, batch); err != nil {
				t.Fatal(err)
			}
			requestDeadline := <-observedDeadline
			if !wantOperationDeadline.IsZero() {
				if !requestDeadline.Equal(wantOperationDeadline) {
					t.Fatalf("request deadline=%s, want operation deadline %s", requestDeadline, wantOperationDeadline)
				}
				return
			}
			remaining := time.Until(requestDeadline)
			if remaining > ollamaTranslationRequestTimeout || remaining < ollamaTranslationRequestTimeout-time.Second {
				t.Fatalf("request deadline remaining=%s, want a cap of %s", remaining, ollamaTranslationRequestTimeout)
			}
		})
	}
}

func ollamaSyntheticBatch(cueCount int) TranslationBatch {
	batch := TranslationBatch{ContractVersion: translationProviderContractVersion, SourceLanguage: "en", TargetLocale: "es-419"}
	for index := 1; index <= cueCount; index++ {
		batch.Cues = append(batch.Cues, TranslationCue{ID: fmt.Sprintf("synthetic-cue-%d", index), Text: fmt.Sprintf("Hello, this is synthetic subtitle cue %d.", index)})
	}
	return batch
}

func ollamaSyntheticResponseForBatch(batch TranslationBatch) ([]byte, error) {
	result := TranslationBatchResult{ContractVersion: translationProviderContractVersion, Cues: make([]TranslatedCue, 0, len(batch.Cues))}
	for index, cue := range batch.Cues {
		result.Cues = append(result.Cues, TranslatedCue{ID: cue.ID, Text: fmt.Sprintf("Hola, subtitulo sintetico %d.", index+1)})
	}
	content, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"message": map[string]string{"role": "assistant", "content": string(content)}, "done": true})
}

func TestTranslateGemmaStructuredOutputSupportsSyntheticASSTranslation(t *testing.T) {
	sourceContent := []byte("[Script Info]\nScriptType: v4.00+\n[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,{\\an8\\pos(10,20)\\i1}Hello world{\\i0}\nDialogue: 0,0:00:03.00,0:00:04.00,Default,Good morning\n")
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var request struct {
			Messages []ollamaChatMessage `json:"messages"`
			Format   json.RawMessage     `json:"format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode Ollama request: %v", err)
			return
		}
		if len(request.Messages) != 1 {
			t.Errorf("messages=%d, want 1", len(request.Messages))
			return
		}
		parts := strings.SplitN(request.Messages[0].Content, "\n\n", 2)
		if len(parts) != 2 {
			t.Errorf("translation prompt did not contain one bounded batch payload")
			return
		}
		var batch TranslationBatch
		if err := json.Unmarshal([]byte(parts[1]), &batch); err != nil {
			t.Errorf("decode synthetic batch: %v", err)
			return
		}
		translated := make([]map[string]string, 0, len(batch.Cues))
		for _, cue := range batch.Cues {
			text := strings.NewReplacer("Hello", "Hola", "world", "mundo", "Good morning", "Buenos días").Replace(cue.Text)
			translated = append(translated, map[string]string{"id": cue.ID, "text": text})
		}
		inner, err := json.Marshal(map[string]any{"contractVersion": batch.ContractVersion, "cues": translated})
		if err != nil {
			t.Errorf("encode structured response: %v", err)
			return
		}
		writeOllamaTestResponse(t, w, string(inner), 20, 12, 31_000_000)
	}))
	defer server.Close()
	provider, err := newOllamaTranslationProvider(ollamaTestSettings(server.URL, "translategemma:12b"))
	if err != nil {
		t.Fatal(err)
	}
	source := subtitleTranslationSourceResponse{Representation: "ass", Language: "en"}
	request := subtitleTranslationOperationRequest{SourceIdentity: "synthetic-ass-source", RequestedSourceLanguage: "en", TargetLocale: "es-419", RequestedRepresentation: "ass", Provider: translationProviderOllama, ProviderModel: "translategemma:12b"}
	result, err := executeTextTranslation(context.Background(), source, sourceContent, request, provider, defaultTranslationSettings(), nil, nil)
	if err != nil {
		t.Fatalf("synthetic EN to es-419 ASS translation failed: %v", err)
	}
	if requests != 1 || !bytes.Equal(sourceContent, []byte("[Script Info]\nScriptType: v4.00+\n[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,{\\an8\\pos(10,20)\\i1}Hello world{\\i0}\nDialogue: 0,0:00:03.00,0:00:04.00,Default,Good morning\n")) {
		t.Fatalf("source ASS changed or requests=%d", requests)
	}
	output := string(result.Outputs["ass"])
	for _, expected := range []string{"0,0:00:01.00,0:00:02.00,Default,{\\an8\\pos(10,20)\\i1}Hola mundo{\\i0}", "0,0:00:03.00,0:00:04.00,Default,Buenos días"} {
		if !strings.Contains(output, expected) {
			t.Errorf("ASS output lost translated text, timing, or protected style: missing %q in %q", expected, output)
		}
	}
}

type syntheticTranslationProvider func(context.Context, TranslationBatch) (TranslationBatchResult, error)

func (provider syntheticTranslationProvider) Translate(ctx context.Context, batch TranslationBatch) (TranslationBatchResult, error) {
	return provider(ctx, batch)
}

func TestExecuteTextTranslationRejectsProtectedASSMutationBeforeOutput(t *testing.T) {
	sourceContent := []byte("[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,{\\an8\\pos(10,20)}Hello\n")
	provider := syntheticTranslationProvider(func(_ context.Context, batch TranslationBatch) (TranslationBatchResult, error) {
		return TranslationBatchResult{ContractVersion: batch.ContractVersion, Cues: []TranslatedCue{{ID: batch.Cues[0].ID, Text: "Hola"}}}, nil
	})
	source := subtitleTranslationSourceResponse{Representation: "ass", Language: "en"}
	request := subtitleTranslationOperationRequest{SourceIdentity: "synthetic-ass-protected", RequestedSourceLanguage: "en", TargetLocale: "es-419", RequestedRepresentation: "ass", Provider: translationProviderOllama, ProviderModel: "qwen2.5:7b"}
	result, err := executeTextTranslation(context.Background(), source, sourceContent, request, provider, defaultTranslationSettings(), nil, nil)
	if err == nil || len(result.Outputs) != 0 {
		t.Fatalf("protected ASS mutation produced output: result=%#v err=%v", result, err)
	}
	var diagnostic *translationResponseDiagnosticError
	if !errors.As(err, &diagnostic) || diagnostic.Category != translationResponseDiagnosticASSStructureInvalid || diagnostic.BatchIndex != 1 || diagnostic.BatchCount != 1 {
		t.Fatalf("ASS diagnostic=%#v err=%v", diagnostic, err)
	}
	if string(sourceContent) != "[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,{\\an8\\pos(10,20)}Hello\n" {
		t.Fatal("source ASS was modified")
	}
}

func TestExecuteTextTranslationRejectsLaterBatchWithoutPartialOutput(t *testing.T) {
	sourceContent := []byte("[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,Hello\nDialogue: 0,0:00:03.00,0:00:04.00,Default,Goodbye\n")
	providerCalls := 0
	provider := syntheticTranslationProvider(func(_ context.Context, batch TranslationBatch) (TranslationBatchResult, error) {
		providerCalls++
		if providerCalls == 1 {
			return TranslationBatchResult{ContractVersion: batch.ContractVersion, Cues: []TranslatedCue{{ID: batch.Cues[0].ID, Text: "Hola"}}}, nil
		}
		return TranslationBatchResult{ContractVersion: "translation-provider-v0", Cues: []TranslatedCue{{ID: batch.Cues[0].ID, Text: "Adiós"}}}, nil
	})
	source := subtitleTranslationSourceResponse{Representation: "ass", Language: "en"}
	request := subtitleTranslationOperationRequest{SourceIdentity: "synthetic-ass-batches", RequestedSourceLanguage: "en", TargetLocale: "es-419", RequestedRepresentation: "ass", Provider: translationProviderOllama, ProviderModel: "qwen2.5:7b"}
	settings := defaultTranslationSettings()
	settings.MaxCuesPerRequest = 1
	result, err := executeTextTranslation(context.Background(), source, sourceContent, request, provider, settings, nil, nil)
	if err == nil || len(result.Outputs) != 0 || providerCalls != 2 {
		t.Fatalf("later malformed batch was not rejected atomically: calls=%d result=%#v err=%v", providerCalls, result, err)
	}
	var diagnostic *translationResponseDiagnosticError
	if !errors.As(err, &diagnostic) || diagnostic.Category != translationResponseDiagnosticContractVersionMismatch || diagnostic.BatchIndex != 2 || diagnostic.BatchCount != 2 {
		t.Fatalf("later-batch diagnostic=%#v err=%v", diagnostic, err)
	}
	if string(sourceContent) != "[Events]\nFormat: Layer, Start, End, Style, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,Hello\nDialogue: 0,0:00:03.00,0:00:04.00,Default,Goodbye\n" {
		t.Fatal("source ASS was modified")
	}
}

func TestOllamaTranslationProviderDoesNotPerformHealthOrModelChecks(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/chat" {
			t.Errorf("unexpected preflight path: %q", r.URL.Path)
		}
		writeOllamaTestResponse(t, w, `{"ContractVersion":"translation-provider-v1","Cues":[{"ID":"cue-1","Text":"Hola"}]}`, 0, 0, 0)
	}))
	defer server.Close()
	provider, err := newOllamaTranslationProvider(ollamaTestSettings(server.URL, "model"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Translate(context.Background(), ollamaTestBatch()); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("request count=%d, want one chat request", requests)
	}
}

func TestOllamaTranslationProviderConfigurationFailsClosed(t *testing.T) {
	cases := []struct {
		name     string
		settings translationSettings
	}{
		{name: "provider", settings: translationSettings{Provider: "other", OllamaEndpoint: "http://127.0.0.1", OllamaModel: "model"}},
		{name: "endpoint", settings: translationSettings{Provider: translationProviderOllama, OllamaModel: "model"}},
		{name: "model", settings: translationSettings{Provider: translationProviderOllama, OllamaEndpoint: "http://127.0.0.1"}},
		{name: "invalid endpoint", settings: translationSettings{Provider: translationProviderOllama, OllamaEndpoint: "not-a-url", OllamaModel: "model"}},
		{name: "credential endpoint", settings: translationSettings{Provider: translationProviderOllama, OllamaEndpoint: "http://user:pass@example.com", OllamaModel: "model"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := newOllamaTranslationProvider(test.settings); err == nil {
				t.Fatal("invalid Ollama configuration accepted")
			}
		})
	}
}

func TestOllamaTranslationProviderCancellationBeforeWork(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
	}))
	defer server.Close()
	provider, err := newOllamaTranslationProvider(ollamaTestSettings(server.URL, "model"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := provider.Translate(ctx, ollamaTestBatch())
	if err == nil || translationProviderErrorClassOf(err) != translationProviderErrorCancelled || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation was not classified correctly: class=%q err=%v", translationProviderErrorClassOf(err), err)
	}
	assertEmptyOllamaResult(t, result)
	if requests != 0 {
		t.Fatalf("cancelled request reached server: %d", requests)
	}
}

func TestOllamaTranslationProviderCancellationDuringRequest(t *testing.T) {
	started := make(chan struct{})
	requestCancelled := make(chan struct{})
	client := &http.Client{Transport: ollamaRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		close(started)
		<-req.Context().Done()
		close(requestCancelled)
		return nil, req.Context().Err()
	})}
	provider, err := newOllamaTranslationProviderWithClient(ollamaTestSettings("http://ollama.test", "model"), client)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type translationCall struct {
		result TranslationBatchResult
		err    error
	}
	call := make(chan translationCall, 1)
	go func() {
		result, err := provider.Translate(ctx, ollamaTestBatch())
		call <- translationCall{result: result, err: err}
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("provider request did not reach server")
	}
	cancel()
	var outcome translationCall
	select {
	case outcome = <-call:
	case <-time.After(time.Second):
		t.Fatal("provider did not return after cancellation")
	}
	if outcome.err == nil || translationProviderErrorClassOf(outcome.err) != translationProviderErrorCancelled || !errors.Is(outcome.err, context.Canceled) {
		t.Fatalf("in-flight cancellation was not classified correctly: class=%q err=%v", translationProviderErrorClassOf(outcome.err), outcome.err)
	}
	assertEmptyOllamaResult(t, outcome.result)
	select {
	case <-requestCancelled:
	case <-time.After(time.Second):
		t.Fatal("HTTP transport did not observe request cancellation")
	}
}

func TestOllamaTranslationProviderCallerDeadlineIsTimeout(t *testing.T) {
	started := make(chan struct{})
	deadlineObserved := make(chan struct{})
	client := &http.Client{Transport: ollamaRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		close(started)
		<-req.Context().Done()
		close(deadlineObserved)
		return nil, req.Context().Err()
	})}
	provider, err := newOllamaTranslationProviderWithClient(ollamaTestSettings("http://ollama.test", "model"), client)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	type translationCall struct {
		result TranslationBatchResult
		err    error
	}
	call := make(chan translationCall, 1)
	go func() {
		result, err := provider.Translate(ctx, ollamaTestBatch())
		call <- translationCall{result: result, err: err}
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("provider request did not reach server")
	}
	select {
	case outcome := <-call:
		if outcome.err == nil || translationProviderErrorClassOf(outcome.err) != translationProviderErrorTimeout || !errors.Is(outcome.err, context.DeadlineExceeded) {
			t.Fatalf("deadline was not classified correctly: class=%q err=%v", translationProviderErrorClassOf(outcome.err), outcome.err)
		}
		assertEmptyOllamaResult(t, outcome.result)
	case <-time.After(time.Second):
		t.Fatal("provider did not return after deadline")
	}
	select {
	case <-deadlineObserved:
	case <-time.After(time.Second):
		t.Fatal("HTTP transport did not observe request deadline")
	}
}

func TestOllamaTranslationProviderHTTPClientTimeoutIsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()
	provider, err := newOllamaTranslationProviderWithClient(ollamaTestSettings(server.URL, "model"), &http.Client{Timeout: 25 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Translate(context.Background(), ollamaTestBatch())
	if err == nil || translationProviderErrorClassOf(err) != translationProviderErrorTimeout {
		t.Fatalf("HTTP client timeout was not classified correctly: class=%q err=%v", translationProviderErrorClassOf(err), err)
	}
	if translationProviderErrorClassOf(err) == translationProviderErrorCancelled {
		t.Fatalf("HTTP client timeout was classified as cancellation: %v", err)
	}
	assertEmptyOllamaResult(t, result)
}

func TestOllamaTranslationProviderResponseReadCancellation(t *testing.T) {
	started := make(chan struct{})
	client := &http.Client{Transport: ollamaRoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       &ollamaBlockingResponseBody{ctx: req.Context(), started: started},
			Request:    req,
		}, nil
	})}
	provider, err := newOllamaTranslationProviderWithClient(ollamaTestSettings("http://ollama.test", "model"), client)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type translationCall struct {
		result TranslationBatchResult
		err    error
	}
	call := make(chan translationCall, 1)
	go func() {
		result, err := provider.Translate(ctx, ollamaTestBatch())
		call <- translationCall{result: result, err: err}
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("response read did not start")
	}
	cancel()
	select {
	case outcome := <-call:
		if outcome.err == nil || translationProviderErrorClassOf(outcome.err) != translationProviderErrorCancelled || !errors.Is(outcome.err, context.Canceled) {
			t.Fatalf("response-read cancellation was not classified correctly: class=%q err=%v", translationProviderErrorClassOf(outcome.err), outcome.err)
		}
		assertEmptyOllamaResult(t, outcome.result)
	case <-time.After(time.Second):
		t.Fatal("provider did not return after response-read cancellation")
	}
}

func TestOllamaTranslationProviderHTTPStatusFailures(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusNotFound, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusServiceUnavailable} {
		t.Run(fmt.Sprintf("status-%d", status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "sensitive provider response body")
			}))
			defer server.Close()
			provider, err := newOllamaTranslationProvider(ollamaTestSettings(server.URL, "model"))
			if err != nil {
				t.Fatal(err)
			}
			result, err := provider.Translate(context.Background(), ollamaTestBatch())
			if err == nil || translationProviderErrorClassOf(err) != translationProviderErrorUnavailable {
				t.Fatalf("HTTP failure class=%q err=%v", translationProviderErrorClassOf(err), err)
			}
			var diagnostic *translationResponseDiagnosticError
			if !errors.As(err, &diagnostic) || diagnostic.Category != translationResponseDiagnosticHTTPStatus || diagnostic.HTTPStatus != status {
				t.Fatalf("HTTP diagnostic=%#v, want status %d", diagnostic, status)
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("%d", status)) || strings.Contains(err.Error(), "sensitive provider response body") {
				t.Fatalf("HTTP failure diagnostic was not bounded: %v", err)
			}
			assertEmptyOllamaResult(t, result)
		})
	}
}

func TestOllamaTranslationProviderMalformedResponses(t *testing.T) {
	cases := []struct {
		name           string
		payload        string
		wantDiagnostic string
	}{
		{name: "invalid outer JSON", payload: `not-json`, wantDiagnostic: translationResponseDiagnosticOuterEnvelopeInvalid},
		{name: "truncated outer JSON", payload: `{"message":`, wantDiagnostic: translationResponseDiagnosticOuterEnvelopeTruncated},
		{name: "missing message", payload: `{}`, wantDiagnostic: translationResponseDiagnosticAssistantContentEmpty},
		{name: "empty assistant content", payload: `{"message":{"content":""}}`, wantDiagnostic: translationResponseDiagnosticAssistantContentEmpty},
		{name: "invalid inner JSON", payload: `{"message":{"content":"not-json"}}`, wantDiagnostic: translationResponseDiagnosticInnerJSONInvalid},
		{name: "truncated inner JSON", payload: `{"message":{"content":"{"}}`, wantDiagnostic: translationResponseDiagnosticInnerJSONTruncated},
		{name: "unknown inner field", payload: `{"message":{"content":"{\"contractVersion\":\"translation-provider-v1\",\"cues\":[{\"id\":\"cue-1\",\"text\":\"Hola\",\"start\":1}]}"}}`, wantDiagnostic: translationResponseDiagnosticInnerJSONInvalid},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, test.payload)
			}))
			defer server.Close()
			provider, err := newOllamaTranslationProvider(ollamaTestSettings(server.URL, "model"))
			if err != nil {
				t.Fatal(err)
			}
			result, err := provider.Translate(context.Background(), ollamaTestBatch())
			if err == nil || translationProviderErrorClassOf(err) != translationProviderErrorMalformed {
				t.Fatalf("malformed response class=%q err=%v", translationProviderErrorClassOf(err), err)
			}
			var diagnostic *translationResponseDiagnosticError
			if !errors.As(err, &diagnostic) || diagnostic.Category != test.wantDiagnostic {
				t.Fatalf("diagnostic=%#v, want %q", diagnostic, test.wantDiagnostic)
			}
			if strings.Contains(err.Error(), "Hola") || strings.Contains(err.Error(), "not-json") {
				t.Fatalf("diagnostic exposed provider content: %v", err)
			}
			assertEmptyOllamaResult(t, result)
		})
	}
}

func TestOllamaTranslationProviderRejectsTruncatedCompletion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]string{"role": "assistant", "content": `{"contractVersion":"translation-provider-v1","cues":[{"id":"cue-1","text":"Hola"}]}`},
			"done":    true, "done_reason": "length",
		})
	}))
	defer server.Close()
	provider, err := newOllamaTranslationProvider(ollamaTestSettings(server.URL, "model"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Translate(context.Background(), ollamaTestBatch())
	if err == nil || translationProviderErrorClassOf(err) != translationProviderErrorMalformed {
		t.Fatalf("truncated completion class=%q err=%v", translationProviderErrorClassOf(err), err)
	}
	var diagnostic *translationResponseDiagnosticError
	if !errors.As(err, &diagnostic) || diagnostic.Category != translationResponseDiagnosticResponseTruncated {
		t.Fatalf("truncated completion diagnostic=%#v", diagnostic)
	}
	assertEmptyOllamaResult(t, result)
}

func TestOllamaTranslationProviderRejectsConfiguredRequestBodyOverflow(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++ }))
	defer server.Close()
	settings := ollamaTestSettings(server.URL, "model")
	settings.MaxRequestBytes = 1024
	provider, err := newOllamaTranslationProviderWithClient(settings, &http.Client{Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	batch := ollamaTestBatch()
	batch.Cues[0].Text = strings.Repeat("x", 900)
	if _, err := provider.Translate(context.Background(), batch); err == nil {
		t.Fatal("request body overflow accepted")
	}
	if requests != 0 {
		t.Fatalf("overflow made an HTTP request: %d", requests)
	}
}

func TestOllamaTranslationProviderRejectsMalformedOrOversizedResponsesWithoutRetainingBody(t *testing.T) {
	t.Run("malformed inner JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeOllamaTestResponse(t, w, "not-json", 0, 0, 0)
		}))
		defer server.Close()
		provider, err := newOllamaTranslationProvider(ollamaTestSettings(server.URL, "model"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := provider.Translate(context.Background(), ollamaTestBatch()); err == nil || strings.Contains(err.Error(), "not-json") {
			t.Fatalf("malformed response was not safely rejected: %v", err)
		}
	})
	t.Run("bounded outer response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(strings.Repeat("x", ollamaTranslationMaxResponseBody+1)))
		}))
		defer server.Close()
		provider, err := newOllamaTranslationProvider(ollamaTestSettings(server.URL, "model"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := provider.Translate(context.Background(), ollamaTestBatch()); err == nil || translationProviderErrorClassOf(err) != translationProviderErrorMalformed || strings.Contains(err.Error(), strings.Repeat("x", 100)) {
			t.Fatalf("oversized response was not safely bounded: %v", err)
		} else {
			var diagnostic *translationResponseDiagnosticError
			if !errors.As(err, &diagnostic) || diagnostic.Category != translationResponseDiagnosticResponseTooLarge {
				t.Fatalf("oversized response diagnostic=%#v", diagnostic)
			}
		}
	})
}

func TestOllamaTranslationProviderUnreachableEndpointIsActionable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	endpoint := server.URL
	server.Close()
	settings := ollamaTestSettings(endpoint, "model")
	provider, err := newOllamaTranslationProviderWithClient(settings, &http.Client{Timeout: 250 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Translate(context.Background(), ollamaTestBatch())
	if err == nil || translationProviderErrorClassOf(err) != translationProviderErrorUnavailable || !strings.Contains(strings.ToLower(err.Error()), "ollama") {
		t.Fatalf("unreachable endpoint error was not actionable: %v", err)
	}
	assertEmptyOllamaResult(t, result)
}

func assertEmptyOllamaResult(t *testing.T, result TranslationBatchResult) {
	t.Helper()
	if result.ContractVersion != "" || len(result.Cues) != 0 || result.Metrics != (TranslationProviderMetrics{}) {
		t.Fatalf("failed request returned a result: %#v", result)
	}
}

type ollamaRoundTripperFunc func(*http.Request) (*http.Response, error)

func (f ollamaRoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type ollamaBlockingResponseBody struct {
	ctx     context.Context
	started chan<- struct{}
}

func (b *ollamaBlockingResponseBody) Read([]byte) (int, error) {
	close(b.started)
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}

func (b *ollamaBlockingResponseBody) Close() error {
	return nil
}

func ollamaTestSettings(endpoint, model string) translationSettings {
	settings := defaultTranslationSettings()
	settings.Provider = translationProviderOllama
	settings.OllamaEndpoint = endpoint
	settings.OllamaModel = model
	return settings
}

func ollamaTestBatch() TranslationBatch {
	return TranslationBatch{
		ContractVersion: translationProviderContractVersion,
		SourceLanguage:  "en",
		TargetLocale:    "es-419",
		Cues:            []TranslationCue{{ID: "cue-1", Text: "Hello"}},
	}
}

func writeOllamaTestResponse(t *testing.T, w http.ResponseWriter, content string, promptTokens, outputTokens int, totalDuration int64) {
	t.Helper()
	payload := map[string]any{
		"message":           map[string]string{"role": "assistant", "content": content},
		"prompt_eval_count": promptTokens,
		"eval_count":        outputTokens,
		"total_duration":    totalDuration,
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Error(err)
	}
}
