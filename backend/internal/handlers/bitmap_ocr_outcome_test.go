package handlers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPGSWithNoEligibleCuesFailsBeforeOCR(t *testing.T) {
	root := t.TempDir()
	bin := t.TempDir()
	mediaPath := filepath.Join(root, "Episode.mkv")
	outputPath := filepath.Join(root, "Episode.srt")
	marker := filepath.Join(root, "ocr-tools-started")
	writeTestFile(t, mediaPath, "synthetic source")
	writeExecutable(t, filepath.Join(bin, "ffmpeg"), `#!/bin/sh
output=""
for argument do output="$argument"; done
: > "$output"
`)
	writeExecutable(t, filepath.Join(bin, "tesseract"), `#!/bin/sh
printf 'tesseract\n' >> "$OCR_OUTCOME_MARKER"
if [ "$1" = "--list-langs" ]; then printf '%s\n' eng; exit 0; fi
exit 0
`)
	writeExecutable(t, filepath.Join(bin, "seconv"), `#!/bin/sh
printf 'seconv\n' >> "$OCR_OUTCOME_MARKER"
exit 0
`)
	t.Setenv("OCR_OUTCOME_MARKER", marker)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, err := generateBitmapSubtitleAtPathWithResult(
		context.Background(), mediaPath,
		FFProbeStream{Index: 13, CodecName: "hdmv_pgs_subtitle", Tags: map[string]string{"language": "eng"}},
		"srt", "eng", "accurate", outputPath, 0, 0, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "subtitle stream 13 (hdmv_pgs_subtitle)") || !strings.Contains(err.Error(), "no eligible PGS bitmap cues") || !strings.Contains(err.Error(), "OCR was not attempted") {
		t.Fatalf("zero-cue PGS outcome was not explicit: %v", err)
	}
	if log, readErr := os.ReadFile(marker); readErr != nil || strings.TrimSpace(string(log)) != "tesseract" {
		t.Fatalf("expected only the cached runtime capability probe and no SeConv recognition, log=%q err=%v", log, readErr)
	}
	if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
		t.Fatalf("zero-cue PGS stream produced a misleading artifact: %v", statErr)
	}
}

func TestPGSZeroRecognitionRetainsAttemptEvidenceWithoutCallingStreamEmpty(t *testing.T) {
	root := t.TempDir()
	bin := t.TempDir()
	mediaPath := filepath.Join(root, "Episode.mkv")
	fixturePath := filepath.Join(root, "source.sup")
	outputPath := filepath.Join(root, "Episode.srt")
	logPath := filepath.Join(root, "commands.log")
	writeTestFile(t, mediaPath, "synthetic source")
	if err := os.WriteFile(fixturePath, testPGSSUP(t, []struct{ id, x, y, width, height int }{{id: 1, x: 100, y: 100, width: 320, height: 80}}), 0o600); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(bin, "ffmpeg"), `#!/bin/sh
output=""
for argument do output="$argument"; done
cp "$PGS_SUP_SOURCE" "$output"
`)
	writeExecutable(t, filepath.Join(bin, "tesseract"), `#!/bin/sh
if [ "$1" = "--list-langs" ]; then printf '%s\n' eng; exit 0; fi
exit 0
`)
	writeExecutable(t, filepath.Join(bin, "seconv"), `#!/bin/sh
printf 'seconv\n' >> "$OCR_OUTCOME_LOG"
printf 'Running tesseract OCR on 3 PGS bitmap image(s)\n' >&2
printf '3 image(s) produced no OCR text\n' >&2
printf 'No subtitles recognised\n' >&2
exit 17
`)
	t.Setenv("PGS_SUP_SOURCE", fixturePath)
	t.Setenv("OCR_OUTCOME_LOG", logPath)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, err := generateBitmapSubtitleAtPathWithResult(
		context.Background(), mediaPath,
		FFProbeStream{Index: 13, CodecName: "hdmv_pgs_subtitle", Tags: map[string]string{"language": "eng"}},
		"srt", "eng", "raw", outputPath, 0, 0, nil,
	)
	if err == nil {
		t.Fatal("zero-recognition PGS stream produced a successful sidecar")
	}
	for _, expected := range []string{
		"OCR failed for subtitle stream 13 (hdmv_pgs_subtitle)",
		"OCR recognized no subtitle text using eng",
		"OCR zero-recognition evidence: stream=13",
		"attempted languages=[eng]",
		"mode=raw",
		"presentation composition events=1",
		"bitmap object references=1",
		"SeConv bitmap image counts=[3]",
		"SeConv no-text image counts=[3]",
		"candidate glyph presence=unknown",
		"no empty artifact was created",
	} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf("zero-recognition error omitted %q: %v", expected, err)
		}
	}
	if strings.Contains(strings.ToLower(err.Error()), "stream is empty") {
		t.Fatalf("zero recognition was incorrectly classified as an empty stream: %v", err)
	}
	log, readErr := os.ReadFile(logPath)
	if readErr != nil || strings.TrimSpace(string(log)) != "seconv" {
		t.Fatalf("expected exactly one bounded OCR attempt, log=%q err=%v", log, readErr)
	}
	if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
		t.Fatalf("zero-recognition failure left an empty artifact: %v", statErr)
	}
}
