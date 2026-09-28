package profiletemplates

import (
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func TestAllContainsCanonicalBuiltInProfileTemplates(t *testing.T) {
	templates := All()
	if len(templates) != 11 {
		t.Fatalf("template count = %d, want 11", len(templates))
	}

	keys := make(map[string]bool, len(templates))
	legacyCount := 0
	for _, template := range templates {
		if template.Key == "" || template.Version < 1 || template.Name == "" || template.Category == "" || template.Description == "" {
			t.Fatalf("template metadata is incomplete: %#v", template)
		}
		if keys[template.Key] {
			t.Fatalf("duplicate template key %q", template.Key)
		}
		keys[template.Key] = true
		if template.Definition.Container == "" || template.Definition.VideoCodec == "" || template.Definition.AudioCodec == "" || template.Definition.QualityMode == "" || template.Definition.WorkerConfig == nil {
			t.Fatalf("template %q has an incomplete profile definition", template.Key)
		}
		if template.LegacySeedPreset != "" {
			legacyCount++
		}
	}
	if legacyCount != 7 {
		t.Fatalf("legacy seeded template count = %d, want 7", legacyCount)
	}
}

func TestTemplateDefinitionsAreReturnedAsIndependentCopies(t *testing.T) {
	template, ok := Get("hevc-small-size")
	if !ok {
		t.Fatal("hevc-small-size template was not found")
	}
	profile := template.Definition.NewProfile("Small copy", "asset", template.Description)
	profile.WorkerConfig["preset"] = "changed-copy"
	template.Definition.WorkerConfig["preset"] = "changed-template"

	again, ok := Get("hevc-small-size")
	if !ok {
		t.Fatal("hevc-small-size template was not found after local edits")
	}
	if again.Definition.WorkerConfig["preset"] != "hevc-small-size" {
		t.Fatalf("local edits changed registry definition: %#v", again.Definition.WorkerConfig)
	}
	if profile.TemplateKey != "" || profile.TemplateVersion != 0 {
		t.Fatal("a definition copy unexpectedly became a template record")
	}
}

func TestLegacySeedIdentityMapsToCorrespondingTemplate(t *testing.T) {
	for _, preset := range []string{
		"dvd-archive-main10", "anime-dvd-main10", "series-balanced-main10",
		"hevc-small-size", "hevc-balanced-fast", "hevc-archive-quality", "hevc-bulk-convert",
	} {
		profile := models.Profile{WorkerConfig: models.JSONMap{"source": "mvforge-v1", "preset": preset}}
		template, ok := LegacySeedTemplate(profile)
		if !ok || template.LegacySeedPreset != preset {
			t.Errorf("legacy preset %q mapped to %#v, found=%t", preset, template, ok)
		}
	}

	profile := models.Profile{WorkerConfig: models.JSONMap{"source": "user", "preset": "hevc-small-size"}}
	if _, ok := LegacySeedTemplate(profile); ok {
		t.Fatal("a profile without the legacy seed source marker was recognized as a built-in")
	}
}
