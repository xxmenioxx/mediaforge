package handlers

import (
	"strings"
	"testing"
)

func TestEffectiveAudioFiltersDoesNotAddLoudnessNormalization(t *testing.T) {
	profile := audioEnhancementProfile{
		Filters:        "anull",
		TargetLoudness: -18,
		TruePeak:       -2,
	}

	result := effectiveAudioFilters(profile)

	if result != "anull" {
		t.Fatalf("expected a neutral filter chain, got %q", result)
	}
}

func TestEffectiveAudioFiltersUpdatesExplicitLoudnessNormalization(t *testing.T) {
	profile := audioEnhancementProfile{
		Filters:        "highpass=f=70,loudnorm=I=-20:TP=-3:LRA=9",
		TargetLoudness: -18,
		TruePeak:       -2,
	}

	result := effectiveAudioFilters(profile)

	if !strings.Contains(result, "loudnorm=I=-18:TP=-2:LRA=9") {
		t.Fatalf("expected explicit loudnorm values to be updated, got %q", result)
	}
}

func TestEffectiveAudioFiltersUsesModernOutputChannelLayoutOption(t *testing.T) {
	profile := audioEnhancementProfile{
		Filters:         "anull",
		ChannelMode:     "force-stereo",
		ForceStereoMode: "auto",
	}

	result := effectiveAudioFilters(profile)

	if !strings.Contains(result, "aresample=ochl=stereo") {
		t.Fatalf("expected modern aresample output layout option, got %q", result)
	}
	if strings.Contains(result, "aresample=ocl=") {
		t.Fatalf("deprecated ocl option must not be generated: %q", result)
	}
}

func TestEffectiveAudioFiltersMigratesSavedDeprecatedChannelLayoutOption(t *testing.T) {
	profile := audioEnhancementProfile{Filters: "aresample=ocl=mono"}
	result := effectiveAudioFilters(profile)
	if result != "aresample=ochl=mono" {
		t.Fatalf("expected saved filter migration, got %q", result)
	}
}
