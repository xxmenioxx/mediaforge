package handlers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func TestValidSubtitleSidecar(t *testing.T) {
	if !validSubtitleSidecar("srt", []byte("1\n00:00:01,000 --> 00:00:02,000\nHola\n")) {
		t.Fatal("valid SRT was rejected")
	}
	if !validSubtitleSidecar("ass", []byte("[Events]\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,Hola")) {
		t.Fatal("valid ASS was rejected")
	}
	if validSubtitleSidecar("srt", []byte("not a subtitle")) {
		t.Fatal("invalid SRT was accepted")
	}
}

func TestPublishSubtitleArtifactsUsesJellyfinPlexSidecarName(t *testing.T) {
	temp := t.TempDir()
	staged := filepath.Join(temp, "staging", "movie.spa.default.srt")
	if err := os.MkdirAll(filepath.Dir(staged), 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("1\n00:00:01,000 --> 00:00:02,000\nHola\n")
	if err := os.WriteFile(staged, content, 0o644); err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{SubtitleArtifacts: subtitleArtifactsJSON([]SubtitleArtifact{{
		StreamIndex: 2, SourceCodec: "subrip", Format: "srt", Language: "spa",
		Default: true, StagedPath: staged, SizeBytes: int64(len(content)),
	}})}
	destinationMedia := filepath.Join(temp, "library", "Movie.mkv")

	published, err := publishSubtitleArtifacts(&job, destinationMedia, false)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(temp, "library", "Movie.spa.default.srt")
	if len(published) != 1 || published[0] != expected {
		t.Fatalf("unexpected published paths: %#v", published)
	}
	if data, err := os.ReadFile(expected); err != nil || string(data) != string(content) {
		t.Fatalf("published subtitle mismatch: %q, %v", data, err)
	}
	artifacts := subtitleArtifactsFromJSON(job.SubtitleArtifacts)
	if len(artifacts) != 1 || artifacts[0].PublishedPath != expected {
		t.Fatalf("published path was not recorded: %#v", artifacts)
	}
}

func TestPublishSubtitleArtifactsRejectsDuplicateDestinationsBeforeCopy(t *testing.T) {
	temp := t.TempDir()
	first := filepath.Join(temp, "one.srt")
	second := filepath.Join(temp, "two.srt")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("1\n00:00:01,000 --> 00:00:02,000\nHola\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	job := models.QueueJob{SubtitleArtifacts: subtitleArtifactsJSON([]SubtitleArtifact{
		{StreamIndex: 2, Format: "srt", Language: "spa", StagedPath: first, SizeBytes: 40},
		{StreamIndex: 3, Format: "srt", Language: "spa", StagedPath: second, SizeBytes: 40},
	})}
	destinationMedia := filepath.Join(temp, "library", "Movie.mkv")

	_, err := publishSubtitleArtifacts(&job, destinationMedia, false)
	if err == nil || !strings.Contains(err.Error(), "same destination") {
		t.Fatalf("expected duplicate destination error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(temp, "library", "Movie.spa.srt")); !os.IsNotExist(statErr) {
		t.Fatalf("destination should not be created during failed preflight: %v", statErr)
	}
}

func TestPublishSubtitleArtifactsAcceptsAnIdenticalRetry(t *testing.T) {
	temp := t.TempDir()
	content := []byte("1\n00:00:01,000 --> 00:00:02,000\nHola\n")
	staged := filepath.Join(temp, "staged.srt")
	destination := filepath.Join(temp, "library", "Movie.spa.srt")
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, content, 0o644); err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{SubtitleArtifacts: subtitleArtifactsJSON([]SubtitleArtifact{{
		StreamIndex: 2, Format: "srt", Language: "spa", StagedPath: staged, SizeBytes: int64(len(content)),
	}})}

	published, err := publishSubtitleArtifacts(&job, filepath.Join(temp, "library", "Movie.mkv"), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(published) != 1 || published[0] != destination {
		t.Fatalf("unexpected retry result: %#v", published)
	}
}
