package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTranslationProviderForSettingsSelectsExistingOllamaAdapter(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
	}))
	defer server.Close()
	settings := ollamaTestSettings(server.URL, "model")
	provider, err := translationProviderForSettings(settings)
	if err != nil {
		t.Fatal(err)
	}
	ollama, ok := provider.(*ollamaTranslationProvider)
	if !ok {
		t.Fatalf("provider type=%T, want *ollamaTranslationProvider", provider)
	}
	if ollama.model != settings.OllamaModel || ollama.chatURL.String() != server.URL+"/api/chat" {
		t.Fatalf("factory did not delegate existing Ollama configuration: %#v", ollama)
	}
	if requests != 0 {
		t.Fatalf("provider selection made an HTTP request: %d", requests)
	}
}

func TestTranslationProviderForSettingsFailsClosedWithoutFallback(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		wantText string
	}{
		{name: "missing", wantText: "translation provider is not configured"},
		{name: "unknown", provider: "foo", wantText: `unknown translation provider "foo"`},
		{name: "OpenAI future seam", provider: translationProviderOpenAI, wantText: `translation provider "openai" is not implemented`},
		{name: "DeepL future seam", provider: translationProviderDeepL, wantText: `translation provider "deepl" is not implemented`},
		{name: "Google future seam", provider: translationProviderGoogle, wantText: `translation provider "google" is not implemented`},
		{name: "Azure future seam", provider: translationProviderAzure, wantText: `translation provider "azure" is not implemented`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			provider, err := translationProviderForSettings(translationSettings{Provider: test.provider, OllamaEndpoint: "http://should-not-be-used", OllamaModel: "model"})
			if err == nil || provider != nil {
				t.Fatalf("provider selection unexpectedly succeeded: provider=%T err=%v", provider, err)
			}
			if translationProviderErrorClassOf(err) != translationProviderErrorConfiguration {
				t.Fatalf("error class=%q, want configuration", translationProviderErrorClassOf(err))
			}
			if !strings.Contains(err.Error(), test.wantText) {
				t.Fatalf("error=%q, want substring %q", err, test.wantText)
			}
		})
	}
}

func TestTranslationProviderForSettingsDoesNotInferOllamaDefaults(t *testing.T) {
	cases := []translationSettings{
		{Provider: translationProviderOllama, OllamaModel: "model"},
		{Provider: translationProviderOllama, OllamaEndpoint: "http://ollama.example"},
		{Provider: translationProviderOllama},
	}
	for _, settings := range cases {
		provider, err := translationProviderForSettings(settings)
		if err == nil || provider != nil {
			t.Fatalf("incomplete Ollama settings unexpectedly succeeded: provider=%T err=%v", provider, err)
		}
		if translationProviderErrorClassOf(err) != translationProviderErrorConfiguration {
			t.Fatalf("error class=%q, want configuration", translationProviderErrorClassOf(err))
		}
	}
}
