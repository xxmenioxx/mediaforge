package handlers

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestBuildSubtitleTranslationProvenanceUsesCanonicalIdentityAndMetrics(t *testing.T) {
	identity, err := newTranslationIdentity(translationIdentityTestInput())
	if err != nil {
		t.Fatal(err)
	}
	metadata := identity.metadata()
	metrics := TranslationProviderMetrics{
		Provider:      metadata.Provider,
		Model:         metadata.ProviderModel,
		RequestCount:  3,
		InputTokens:   120,
		OutputTokens:  80,
		LatencyMillis: 456,
	}
	provenance, err := buildSubtitleTranslationProvenance(metadata, "operation-123", "media-metadata", "validated", metrics)
	if err != nil {
		t.Fatal(err)
	}
	if provenance.IdentityKey != metadata.IdentityKey || provenance.Provider != metadata.Provider || provenance.ProviderModel != metadata.ProviderModel || provenance.RequestedSourceLanguage != metadata.RequestedSourceLanguage || provenance.EffectiveSourceLanguage != metadata.EffectiveSourceLanguage || provenance.TargetLocale != metadata.TargetLocale || provenance.SourceRevision != metadata.SourceRevision || provenance.OutputRepresentation != metadata.OutputRepresentation {
		t.Fatalf("provenance identity fields drifted: %#v", provenance)
	}
	if provenance.RequestCount != 3 || provenance.InputTokens != 120 || provenance.OutputTokens != 80 || provenance.LatencyMillis != 456 {
		t.Fatalf("metrics did not round-trip: %#v", provenance)
	}
}

func TestBuildSubtitleTranslationProvenanceRejectsIdentityDrift(t *testing.T) {
	identity, err := newTranslationIdentity(translationIdentityTestInput())
	if err != nil {
		t.Fatal(err)
	}
	metrics := TranslationProviderMetrics{Provider: identity.Provider, Model: identity.ProviderModel}
	cases := []struct {
		name   string
		mutate func(*SubtitleTranslationMetadata)
	}{
		{name: "provider", mutate: func(metadata *SubtitleTranslationMetadata) { metadata.Provider = "other" }},
		{name: "model", mutate: func(metadata *SubtitleTranslationMetadata) { metadata.ProviderModel = "other-model" }},
		{name: "requested language", mutate: func(metadata *SubtitleTranslationMetadata) { metadata.RequestedSourceLanguage = "ja" }},
		{name: "effective language", mutate: func(metadata *SubtitleTranslationMetadata) { metadata.EffectiveSourceLanguage = "ja" }},
		{name: "locale", mutate: func(metadata *SubtitleTranslationMetadata) { metadata.TargetLocale = "es-ES" }},
		{name: "source revision", mutate: func(metadata *SubtitleTranslationMetadata) { metadata.SourceRevision = "changed" }},
		{name: "output representation", mutate: func(metadata *SubtitleTranslationMetadata) { metadata.OutputRepresentation = "ass" }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			metadata := identity.metadata()
			test.mutate(&metadata)
			if _, err := buildSubtitleTranslationProvenance(metadata, "operation-123", "none", "validated", metrics); err == nil {
				t.Fatal("identity drift accepted")
			}
		})
	}
	if _, err := buildSubtitleTranslationProvenance(identity.metadata(), "operation-123", "none", "validated", TranslationProviderMetrics{Provider: "other", Model: identity.ProviderModel}); err == nil {
		t.Fatal("metric provider drift accepted")
	}
	if _, err := buildSubtitleTranslationProvenance(identity.metadata(), "operation-123", "none", "validated", TranslationProviderMetrics{Provider: identity.Provider, Model: "other-model"}); err == nil {
		t.Fatal("metric model drift accepted")
	}
}

func TestBuildSubtitleTranslationProvenanceClampsInvalidMetrics(t *testing.T) {
	identity, err := newTranslationIdentity(translationIdentityTestInput())
	if err != nil {
		t.Fatal(err)
	}
	provenance, err := buildSubtitleTranslationProvenance(identity.metadata(), "operation-123", "none", "validated", TranslationProviderMetrics{
		Provider:      identity.Provider,
		Model:         identity.ProviderModel,
		RequestCount:  -1,
		InputTokens:   math.MaxInt,
		OutputTokens:  -1,
		LatencyMillis: math.MaxInt64,
	})
	if err != nil {
		t.Fatal(err)
	}
	if provenance.RequestCount != 0 || provenance.InputTokens != maxTranslationProvenanceTokens || provenance.OutputTokens != 0 || provenance.LatencyMillis != maxTranslationProvenanceLatencyMs {
		t.Fatalf("invalid metrics were not safely bounded: %#v", provenance)
	}
}

func TestSubtitleTranslationProvenanceIsSecretSafeAndAPIProjectionIsBounded(t *testing.T) {
	identity, err := newTranslationIdentity(translationIdentityTestInput())
	if err != nil {
		t.Fatal(err)
	}
	provenance, err := buildSubtitleTranslationProvenance(identity.metadata(), "operation-123", "none", "validated", TranslationProviderMetrics{Provider: identity.Provider, Model: identity.ProviderModel})
	if err != nil {
		t.Fatal(err)
	}
	metadata := identity.metadata()
	metadata.Provenance = provenance
	artifactJSON, err := json.Marshal(subtitleArtifactsJSON([]SubtitleArtifact{{ArtifactID: "translated", Type: "subtitle_sidecar", Format: "srt", Translation: &metadata}}))
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(artifactJSON)
	for _, hostile := range []string{
		"Authorization: Bearer super-secret-token",
		"full provider response with translated cue payload",
		"https://ollama.example/?api_key=secret",
	} {
		if strings.Contains(serialized, hostile) {
			t.Fatalf("hostile provider data appeared in artifact JSON: %q", hostile)
		}
	}
	for _, forbiddenField := range []string{"authorization", "headers", "prompt", "response", "endpoint", "token"} {
		if strings.Contains(strings.ToLower(serialized), forbiddenField) {
			t.Fatalf("forbidden provenance field appeared in artifact JSON: %q", forbiddenField)
		}
	}

	metadata.Provenance = provenance
	view, err := translationProvenanceAPISafeView(metadata)
	if err != nil {
		t.Fatal(err)
	}
	viewJSON, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	viewString := string(viewJSON)
	if !strings.Contains(viewString, `"provider":"ollama"`) || !strings.Contains(viewString, `"targetLocale":"es-419"`) {
		t.Fatalf("API-safe fields missing: %s", viewString)
	}
	for _, forbiddenField := range []string{"identityKey", "sourceRevision", "endpoint", "prompt", "response", "headers"} {
		if strings.Contains(viewString, forbiddenField) {
			t.Fatalf("API-safe projection exposed forbidden field: %q", forbiddenField)
		}
	}
}

func TestPersistedSubtitleTranslationProvenanceMustMatchMetadata(t *testing.T) {
	identity, err := newTranslationIdentity(translationIdentityTestInput())
	if err != nil {
		t.Fatal(err)
	}
	baseMetadata := identity.metadata()
	validProvenance, err := buildSubtitleTranslationProvenance(baseMetadata, "operation-123", "media-metadata", "validated", TranslationProviderMetrics{Provider: identity.Provider, Model: identity.ProviderModel})
	if err != nil {
		t.Fatal(err)
	}

	provenanceMutationCases := []struct {
		name   string
		mutate func(*SubtitleTranslationProvenance)
	}{
		{name: "translation contract version", mutate: func(provenance *SubtitleTranslationProvenance) {
			provenance.TranslationContractVersion = "other-contract"
		}},
		{name: "provider", mutate: func(provenance *SubtitleTranslationProvenance) { provenance.Provider = "other" }},
		{name: "model", mutate: func(provenance *SubtitleTranslationProvenance) { provenance.ProviderModel = "other-model" }},
		{name: "provider contract version", mutate: func(provenance *SubtitleTranslationProvenance) {
			provenance.ProviderContractVersion = "other-provider-contract"
		}},
		{name: "requested language", mutate: func(provenance *SubtitleTranslationProvenance) { provenance.RequestedSourceLanguage = "ja" }},
		{name: "effective language", mutate: func(provenance *SubtitleTranslationProvenance) { provenance.EffectiveSourceLanguage = "ja" }},
		{name: "target locale", mutate: func(provenance *SubtitleTranslationProvenance) { provenance.TargetLocale = "es-ES" }},
		{name: "context contract version", mutate: func(provenance *SubtitleTranslationProvenance) {
			provenance.ContextContractVersion = "other-context-contract"
		}},
		{name: "context mode", mutate: func(provenance *SubtitleTranslationProvenance) { provenance.ContextMode = "other-context-mode" }},
		{name: "source revision", mutate: func(provenance *SubtitleTranslationProvenance) { provenance.SourceRevision = "revision-2" }},
		{name: "output representation", mutate: func(provenance *SubtitleTranslationProvenance) { provenance.OutputRepresentation = "ass" }},
		{name: "identity key", mutate: func(provenance *SubtitleTranslationProvenance) { provenance.IdentityKey = "stale-key" }},
		{name: "contract", mutate: func(provenance *SubtitleTranslationProvenance) {
			provenance.TranslationContractVersion = "stale-contract"
		}},
	}
	for _, test := range provenanceMutationCases {
		t.Run(test.name, func(t *testing.T) {
			provenance := *validProvenance
			test.mutate(&provenance)
			metadata := baseMetadata
			metadata.Provenance = &provenance
			artifact := persistedTranslationArtifact(t, metadata)
			if translationArtifactMatchesIdentity(artifact, identity) {
				t.Fatal("inconsistent persisted provenance was reusable")
			}
			if view, err := translationProvenanceAPISafeView(*artifact.Translation); err == nil || view != nil {
				t.Fatalf("inconsistent persisted provenance was projected: view=%#v err=%v", view, err)
			}
		})
	}
}

func TestPersistedSubtitleTranslationProvenanceBoundsAreValidated(t *testing.T) {
	identity, err := newTranslationIdentity(translationIdentityTestInput())
	if err != nil {
		t.Fatal(err)
	}
	validProvenance, err := buildSubtitleTranslationProvenance(identity.metadata(), "operation-123", "media-metadata", "validated", TranslationProviderMetrics{Provider: identity.Provider, Model: identity.ProviderModel})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		mutate func(*SubtitleTranslationProvenance)
	}{
		{name: "oversized operation", mutate: func(provenance *SubtitleTranslationProvenance) {
			provenance.OperationID = strings.Repeat("x", maxTranslationProvenanceOperationID+1)
		}},
		{name: "oversized context", mutate: func(provenance *SubtitleTranslationProvenance) {
			provenance.ContextSource = strings.Repeat("x", maxTranslationProvenanceContextSource+1)
		}},
		{name: "empty status", mutate: func(provenance *SubtitleTranslationProvenance) { provenance.ValidationStatus = "" }},
		{name: "oversized status", mutate: func(provenance *SubtitleTranslationProvenance) {
			provenance.ValidationStatus = strings.Repeat("x", maxTranslationProvenanceStatus+1)
		}},
		{name: "negative requests", mutate: func(provenance *SubtitleTranslationProvenance) { provenance.RequestCount = -1 }},
		{name: "negative tokens", mutate: func(provenance *SubtitleTranslationProvenance) { provenance.InputTokens = -1 }},
		{name: "negative output tokens", mutate: func(provenance *SubtitleTranslationProvenance) { provenance.OutputTokens = -1 }},
		{name: "negative latency", mutate: func(provenance *SubtitleTranslationProvenance) { provenance.LatencyMillis = -1 }},
		{name: "excessive requests", mutate: func(provenance *SubtitleTranslationProvenance) {
			provenance.RequestCount = maxTranslationProvenanceRequests + 1
		}},
		{name: "excessive tokens", mutate: func(provenance *SubtitleTranslationProvenance) {
			provenance.OutputTokens = maxTranslationProvenanceTokens + 1
		}},
		{name: "excessive latency", mutate: func(provenance *SubtitleTranslationProvenance) {
			provenance.LatencyMillis = maxTranslationProvenanceLatencyMs + 1
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			provenance := *validProvenance
			test.mutate(&provenance)
			metadata := identity.metadata()
			metadata.Provenance = &provenance
			artifact := persistedTranslationArtifact(t, metadata)
			if translationArtifactMatchesIdentity(artifact, identity) {
				t.Fatal("out-of-range persisted provenance was reusable")
			}
			if view, err := translationProvenanceAPISafeView(*artifact.Translation); err == nil || view != nil {
				t.Fatalf("out-of-range persisted provenance was projected: view=%#v err=%v", view, err)
			}
		})
	}
}

func persistedTranslationArtifact(t *testing.T, metadata SubtitleTranslationMetadata) SubtitleArtifact {
	t.Helper()
	decoded := subtitleArtifactsFromJSON(subtitleArtifactsJSON([]SubtitleArtifact{{ArtifactID: "translated", Type: "subtitle_sidecar", Format: "srt", Translation: &metadata}}))
	if len(decoded) != 1 || decoded[0].Translation == nil {
		t.Fatalf("translation metadata did not round-trip: %#v", decoded)
	}
	return decoded[0]
}

func TestSubtitleTranslationProvenanceAndLegacyMetadataRemainBackwardCompatible(t *testing.T) {
	identity, err := newTranslationIdentity(translationIdentityTestInput())
	if err != nil {
		t.Fatal(err)
	}
	metadata := identity.metadata()
	provenance, err := buildSubtitleTranslationProvenance(metadata, "operation-123", "none", "validated", TranslationProviderMetrics{Provider: identity.Provider, Model: identity.ProviderModel, RequestCount: 2})
	if err != nil {
		t.Fatal(err)
	}
	metadata.Provenance = provenance
	translated := SubtitleArtifact{ArtifactID: "translated", Type: "subtitle_sidecar", Format: "srt", Translation: &metadata}
	decoded := subtitleArtifactsFromJSON(subtitleArtifactsJSON([]SubtitleArtifact{translated}))
	if len(decoded) != 1 || decoded[0].Translation == nil || decoded[0].Translation.Provenance == nil || decoded[0].Translation.Provenance.RequestCount != 2 {
		t.Fatalf("provenance did not round-trip: %#v", decoded)
	}
	if !translationArtifactMatchesIdentity(decoded[0], identity) {
		t.Fatal("provenance caused exact identity reuse to fail")
	}

	metadataWithoutProvenance := identity.metadata()
	withoutProvenance := subtitleArtifactsFromJSON(subtitleArtifactsJSON([]SubtitleArtifact{{ArtifactID: "translated", Type: "subtitle_sidecar", Format: "srt", Translation: &metadataWithoutProvenance}}))
	if len(withoutProvenance) != 1 || withoutProvenance[0].Translation == nil || withoutProvenance[0].Translation.Provenance != nil || !translationArtifactMatchesIdentity(withoutProvenance[0], identity) {
		t.Fatalf("translated metadata without provenance was not readable: %#v", withoutProvenance)
	}

	legacy := subtitleArtifactsFromJSON(subtitleArtifactsJSON([]SubtitleArtifact{{ArtifactID: "legacy", Type: "subtitle_sidecar", Format: "srt"}}))
	if len(legacy) != 1 || legacy[0].Translation != nil {
		t.Fatalf("legacy artifact was not preserved: %#v", legacy)
	}
}
