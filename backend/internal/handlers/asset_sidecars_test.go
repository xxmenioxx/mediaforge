package handlers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMetadataSidecarsForMediaOnlyReturnsExactStemNFO(t *testing.T) {
	directory := t.TempDir()
	mediaPath := filepath.Join(directory, "Movie.mkv")
	for _, name := range []string{"Movie.nfo", "Movie.extra.nfo", "Movie.mkv.nfo", "notes.nfo"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(name), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	sidecars, err := metadataSidecarsForMedia(mediaPath)
	if err != nil {
		t.Fatalf("metadataSidecarsForMedia returned an error: %v", err)
	}
	if len(sidecars) != 1 {
		t.Fatalf("sidecars=%#v, want only the exact-stem NFO", sidecars)
	}
	if sidecars[0].Path != filepath.Join(directory, "Movie.nfo") || sidecars[0].FileName != "Movie.nfo" || sidecars[0].Format != "nfo" {
		t.Fatalf("sidecar=%#v, want Movie.nfo metadata", sidecars[0])
	}
}

func TestMetadataSidecarsForMediaDoesNotReturnNFODirectories(t *testing.T) {
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "Movie.nfo"), 0o700); err != nil {
		t.Fatalf("create NFO directory: %v", err)
	}

	sidecars, err := metadataSidecarsForMedia(filepath.Join(directory, "Movie.mkv"))
	if err != nil {
		t.Fatalf("metadataSidecarsForMedia returned an error: %v", err)
	}
	if len(sidecars) != 0 {
		t.Fatalf("sidecars=%#v, want no sidecar for a directory", sidecars)
	}
}
