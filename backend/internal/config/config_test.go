package config

import "testing"

func TestLoadPrefersMVForgeEnvironment(t *testing.T) {
	t.Setenv("MVFORGE_API_HOST", "mvforge-host")
	t.Setenv("MEDIAFORGE_API_HOST", "legacy-host")
	t.Setenv("MVFORGE_API_PORT", "9090")
	t.Setenv("MEDIAFORGE_API_PORT", "8081")
	t.Setenv("MVFORGE_DB_PATH", "/data/mvforge.db")
	t.Setenv("MEDIAFORGE_DB_PATH", "/data/mediaforge.db")

	config := Load()
	if config.Host != "mvforge-host" || config.Port != "9090" || config.DatabasePath != "/data/mvforge.db" {
		t.Fatalf("unexpected config: %#v", config)
	}
}

func TestLoadAcceptsLegacyEnvironmentDuringRename(t *testing.T) {
	t.Setenv("MVFORGE_API_HOST", "")
	t.Setenv("MVFORGE_API_PORT", "")
	t.Setenv("MVFORGE_DB_PATH", "")
	t.Setenv("MEDIAFORGE_API_HOST", "legacy-host")
	t.Setenv("MEDIAFORGE_API_PORT", "8081")
	t.Setenv("MEDIAFORGE_DB_PATH", "/data/mediaforge.db")

	config := Load()
	if config.Host != "legacy-host" || config.Port != "8081" || config.DatabasePath != "/data/mediaforge.db" {
		t.Fatalf("unexpected legacy config: %#v", config)
	}
}
