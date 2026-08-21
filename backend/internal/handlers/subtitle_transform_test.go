package handlers

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func TestTextSubtitleExtractionUsesTestEncodeWindowOnlyWhenRequested(t *testing.T) {
	segmented := shellJoin(textSubtitleExtractionArgs(MediaJobPlan{
		InputPath: "/raw/movie.mkv", SegmentStartSeconds: 42.5, SegmentDurationSeconds: 20,
	}, 3, "srt", "/library/test.srt"))
	assertContains(t, segmented, "-ss 42.5 -i /raw/movie.mkv -t 20 -map 0:3")
	normal := shellJoin(textSubtitleExtractionArgs(MediaJobPlan{InputPath: "/raw/movie.mkv"}, 3, "srt", "/staging/full.srt"))
	assertNotContains(t, normal, "-ss")
	assertNotContains(t, normal, "-t 20")
}

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

func TestGenerateSubtitleArtifactsSkipsExistingIdentifiedSidecar(t *testing.T) {
	temp := t.TempDir()
	mediaPath := filepath.Join(temp, "Episode.mkv")
	if err := os.WriteFile(mediaPath, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(temp, "Episode.spa.default.srt"), []byte("1\n00:00:01,000 --> 00:00:02,000\nHola\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := MediaJobPlan{
		SourceAssetPath: mediaPath,
		OutputPath:      filepath.Join(temp, "staging", "Episode.mkv"),
		Streams:         MediaStreamInventory{Subtitle: []MediaStream{{Index: 2, Codec: "subrip", Language: "spa", Default: true}}},
		Override:        AssetConversionOverrideState{SubtitleTransforms: []SubtitleTransform{{StreamIndex: 2, Format: "srt", Language: "spa", MakeDefault: true, RemoveEmbedded: true}}},
	}
	artifacts, err := generateSubtitleArtifacts(context.Background(), plan)
	if err != nil || len(artifacts) != 0 {
		t.Fatalf("existing identified sidecar should skip conversion: artifacts=%#v err=%v", artifacts, err)
	}
}

func TestExistingSubtitleSidecarCanOnlySatisfyOneTransform(t *testing.T) {
	temp := t.TempDir()
	mediaPath := filepath.Join(temp, "Episode.mkv")
	if err := os.WriteFile(mediaPath, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(temp, "Episode.spa.srt"), []byte("1\n00:00:01,000 --> 00:00:02,000\nHola\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	used := map[string]struct{}{}
	transform := SubtitleTransform{Format: "srt", Language: "spa"}
	if !existingSubtitleTransformSidecar(mediaPath, transform, "spa", used) {
		t.Fatal("the first identified transform should reuse the existing sidecar")
	}
	if existingSubtitleTransformSidecar(mediaPath, transform, "spa", used) {
		t.Fatal("one sidecar must not satisfy two distinct subtitle transforms")
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

	published, err := publishSubtitleArtifacts(&job, destinationMedia, false, nil)
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

func TestPublishSubtitleArtifactsUsesStreamIndexForDuplicateDestinations(t *testing.T) {
	temp := t.TempDir()
	first := filepath.Join(temp, "one.srt")
	second := filepath.Join(temp, "two.srt")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("1\n00:00:01,000 --> 00:00:02,000\nHola\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	job := models.QueueJob{SubtitleArtifacts: subtitleArtifactsJSON([]SubtitleArtifact{
		{StreamIndex: 5, Format: "srt", Language: "eng", StagedPath: first, SizeBytes: 40},
		{StreamIndex: 6, Format: "srt", Language: "eng", StagedPath: second, SizeBytes: 40},
	})}
	destinationMedia := filepath.Join(temp, "library", "Movie.mkv")

	published, err := publishSubtitleArtifacts(&job, destinationMedia, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{
		filepath.Join(temp, "library", "Movie.eng.srt"),
		filepath.Join(temp, "library", "Movie.eng.6.srt"),
	}
	if !reflect.DeepEqual(published, expected) {
		t.Fatalf("published paths=%#v want %#v", published, expected)
	}
	for _, destination := range expected {
		if info, statErr := os.Stat(destination); statErr != nil || info.Size() == 0 {
			t.Fatalf("published subtitle %q is not readable: info=%#v err=%v", destination, info, statErr)
		}
	}

	published, err = publishSubtitleArtifacts(&job, destinationMedia, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(published) != 0 {
		t.Fatalf("identical retry must not create more sidecars: %#v", published)
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

	published, err := publishSubtitleArtifacts(&job, filepath.Join(temp, "library", "Movie.mkv"), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(published) != 0 {
		t.Fatalf("retry must not report an existing sidecar as newly published: %#v", published)
	}
}

func TestPublishSubtitleArtifactsPreservesDifferentLibrarySidecar(t *testing.T) {
	temp := t.TempDir()
	staged := filepath.Join(temp, "staged.srt")
	destination := filepath.Join(temp, "library", "Movie.spa.srt")
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("generated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("edited in library"), 0o644); err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{SubtitleArtifacts: subtitleArtifactsJSON([]SubtitleArtifact{{
		StreamIndex: 2, Format: "srt", Language: "spa", StagedPath: staged, SizeBytes: 9,
	}})}

	published, err := publishSubtitleArtifacts(&job, filepath.Join(temp, "library", "Movie.mkv"), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	renamed := filepath.Join(temp, "library", "Movie.mvf.spa.srt")
	if len(published) != 1 || published[0] != renamed {
		t.Fatalf("generated sidecar was not published under MVF name: %#v", published)
	}
	if content, err := os.ReadFile(destination); err != nil || string(content) != "edited in library" {
		t.Fatalf("Library sidecar was overwritten: content=%q err=%v", content, err)
	}
	if content, err := os.ReadFile(renamed); err != nil || string(content) != "generated" {
		t.Fatalf("renamed generated sidecar content=%q err=%v", content, err)
	}
	if !strings.Contains(job.Notes, "renamed") {
		t.Fatalf("rename was not recorded in job notes: %q", job.Notes)
	}
}
