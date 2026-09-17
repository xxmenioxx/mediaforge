package handlers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJapanesePGSOCRPreflightAndFallbackReusePreparedInput(t *testing.T) {
	root := t.TempDir()
	bin := t.TempDir()
	mediaPath := filepath.Join(root, "Episode.mkv")
	outputPath := filepath.Join(root, "Episode.srt")
	rawPath := filepath.Join(root, "Episode.raw.srt")
	logPath := filepath.Join(root, "commands.log")
	writeTestFile(t, mediaPath, "source")
	t.Setenv("JPN_OCR_LOG", logPath)

	writeExecutable(t, filepath.Join(bin, "tesseract"), `#!/bin/sh
if [ "$1" = "--list-langs" ]; then
  printf '%s\n' eng jpn jpn_vert spa
  exit 0
fi
exit 0
`)
	writeExecutable(t, filepath.Join(bin, "ffmpeg"), `#!/bin/sh
output=""
for argument do output="$argument"; done
case "$output" in
  *.sup)
    printf 'extract\n' >> "$JPN_OCR_LOG"
    printf 'PGS' > "$output"
    ;;
esac
`)
	writeExecutable(t, filepath.Join(bin, "seconv"), `#!/bin/sh
language=""
output=""
for argument do
  case "$argument" in
	    --ocr-language:*) language="${argument#--ocr-language:}" ;;
	    --output-filename:*) output="${argument#--output-filename:}" ;;
  esac
done
printf '%s\n' "$language" >> "$JPN_OCR_LOG"
if [ "$language" = "jpn" ]; then
  printf 'No subtitles recognised\n' >&2
  exit 17
fi
printf '%s\n' '1' '00:00:01,000 --> 00:00:02,000' 'Japanese OCR' > "$output"
exit 0
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	stream := FFProbeStream{Index: 4, CodecName: "hdmv_pgs_subtitle", Tags: map[string]string{"language": "jpn"}}
	message, err := runBitmapOCR(context.Background(), mediaPath, stream, "srt", "jpn", "raw", outputPath, rawPath, nil)
	if err != nil {
		t.Fatalf("Japanese OCR fallback failed: message=%q err=%v", message, err)
	}
	content, err := os.ReadFile(outputPath)
	if err != nil || !strings.Contains(string(content), "Japanese OCR") {
		t.Fatalf("fallback output=%q err=%v", content, err)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(string(log)), "extract\njpn\njpn_vert"; got != want {
		t.Fatalf("Japanese fallback should extract once and attempt each orientation once: got %q want %q", got, want)
	}
}

func TestJapanesePGSOCRAccurateFallbackPreservesModeAndEffectiveLanguage(t *testing.T) {
	root := t.TempDir()
	bin := t.TempDir()
	logPath := filepath.Join(root, "commands.log")
	writeExecutable(t, filepath.Join(bin, "tesseract"), "#!/bin/sh\nif [ \"$1\" = \"--list-langs\" ]; then printf '%s\\n' jpn jpn_vert; exit 0; fi\nexit 0\n")
	writeExecutable(t, filepath.Join(bin, "seconv"), `#!/bin/sh
language=""
output=""
for argument do
  case "$argument" in
    --ocr-language:*) language="${argument#--ocr-language:}" ;;
    --output-filename:*) output="${argument#--output-filename:}" ;;
  esac
done
if [ -n "$language" ]; then printf '%s\n' "$language" >> "$JPN_OCR_LOG"; fi
if [ "$language" = "jpn" ]; then printf '%s\n' 'No subtitles recognised' >&2; exit 17; fi
if [ -n "$output" ]; then printf '%s\n' '1' '00:00:01,000 --> 00:00:02,000' 'Japanese OCR' > "$output"; fi
`)
	t.Setenv("JPN_OCR_LOG", logPath)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	result, err := runBitmapOCRWithResult(
		context.Background(),
		filepath.Join(root, "Episode.mkv"),
		FFProbeStream{Index: 4, CodecName: "dvd_subtitle", Tags: map[string]string{"language": "jpn"}},
		"srt",
		"jpn",
		"accurate",
		filepath.Join(root, "out.srt"),
		filepath.Join(root, "raw.srt"),
		nil,
	)
	if err != nil {
		t.Fatalf("accurate Japanese OCR fallback failed: result=%#v err=%v", result, err)
	}
	if result.EffectiveLanguage != "jpn_vert" {
		t.Fatalf("accurate fallback effective language=%q", result.EffectiveLanguage)
	}
	content, err := os.ReadFile(filepath.Join(root, "out.srt"))
	if err != nil || !strings.Contains(string(content), "Japanese OCR") {
		t.Fatalf("accurate fallback output=%q err=%v", content, err)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(string(log)), "jpn\njpn_vert\njpn_vert"; got != want {
		t.Fatalf("accurate mode should run both passes only for the successful fallback orientation: got %q want %q", got, want)
	}
}

func TestJapanesePGSOCRRejectsMissingRuntimeLanguageBeforeSeConv(t *testing.T) {
	root := t.TempDir()
	bin := t.TempDir()
	mediaPath := filepath.Join(root, "Episode.mkv")
	marker := filepath.Join(root, "seconv-started")
	writeTestFile(t, mediaPath, "source")
	writeExecutable(t, filepath.Join(bin, "tesseract"), "#!/bin/sh\nif [ \"$1\" = \"--list-langs\" ]; then printf '%s\\n' eng spa; exit 0; fi\nexit 0\n")
	writeExecutable(t, filepath.Join(bin, "seconv"), "#!/bin/sh\ntouch \"$JPN_OCR_MARKER\"\nexit 0\n")
	t.Setenv("JPN_OCR_MARKER", marker)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	stream := FFProbeStream{Index: 4, CodecName: "dvd_subtitle", Tags: map[string]string{"language": "jpn"}}
	message, err := runBitmapOCR(context.Background(), mediaPath, stream, "srt", "jpn", "raw", filepath.Join(root, "out.srt"), filepath.Join(root, "raw.srt"), nil)
	if err == nil || !strings.Contains(err.Error(), `OCR language "jpn" is unavailable`) {
		t.Fatalf("missing Japanese language was not rejected: message=%q err=%v", message, err)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("SeConv started despite failed language preflight: %v", statErr)
	}
}

func TestJapanesePGSOCRDoesNotAttemptUnavailableOppositeOrientation(t *testing.T) {
	root := t.TempDir()
	bin := t.TempDir()
	logPath := filepath.Join(root, "commands.log")
	writeExecutable(t, filepath.Join(bin, "tesseract"), "#!/bin/sh\nif [ \"$1\" = \"--list-langs\" ]; then printf '%s\\n' eng jpn; exit 0; fi\nexit 0\n")
	writeExecutable(t, filepath.Join(bin, "seconv"), `#!/bin/sh
language=""
for argument do
  case "$argument" in
    --ocr-language:*) language="${argument#--ocr-language:}" ;;
  esac
done
printf '%s\n' "$language" >> "$JPN_OCR_LOG"
printf 'No subtitles recognised\n' >&2
exit 17
`)
	t.Setenv("JPN_OCR_LOG", logPath)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	stream := FFProbeStream{Index: 4, CodecName: "dvd_subtitle", Tags: map[string]string{"language": "jpn"}}
	_, err := runBitmapOCR(context.Background(), filepath.Join(root, "Episode.mkv"), stream, "srt", "jpn", "raw", filepath.Join(root, "out.srt"), filepath.Join(root, "raw.srt"), nil)
	if err == nil || !strings.Contains(err.Error(), "alternate Japanese orientation jpn_vert is unavailable") {
		t.Fatalf("unavailable opposite orientation was not reported: %v", err)
	}
	log, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if got := strings.TrimSpace(string(log)); got != "jpn" {
		t.Fatalf("unavailable opposite orientation was attempted: %q", got)
	}
}

func TestJapanesePGSOCRDoesNotFallbackForUnrelatedNonZeroFailure(t *testing.T) {
	root := t.TempDir()
	bin := t.TempDir()
	logPath := filepath.Join(root, "commands.log")
	writeExecutable(t, filepath.Join(bin, "tesseract"), "#!/bin/sh\nif [ \"$1\" = \"--list-langs\" ]; then printf '%s\\n' eng jpn jpn_vert; exit 0; fi\nexit 0\n")
	writeExecutable(t, filepath.Join(bin, "seconv"), `#!/bin/sh
language=""
for argument do
  case "$argument" in
	    --ocr-language:*) language="${argument#--ocr-language:}" ;;
  esac
done
printf '%s\n' "$language" >> "$JPN_OCR_LOG"
printf 'fatal malformed input\n' >&2
exit 17
`)
	t.Setenv("JPN_OCR_LOG", logPath)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	stream := FFProbeStream{Index: 4, CodecName: "dvd_subtitle", Tags: map[string]string{"language": "jpn"}}
	message, err := runBitmapOCR(context.Background(), filepath.Join(root, "Episode.mkv"), stream, "srt", "jpn", "raw", filepath.Join(root, "out.srt"), filepath.Join(root, "raw.srt"), nil)
	if err == nil || !strings.Contains(err.Error(), "exit status") {
		t.Fatalf("unrelated non-zero SeConv result was not preserved: message=%q err=%v", message, err)
	}
	log, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if got := strings.TrimSpace(string(log)); got != "jpn" {
		t.Fatalf("unrelated failure incorrectly triggered Japanese fallback: %q", got)
	}
}

func TestJapanesePGSOCRBothOrientationsZeroHasActionableFailure(t *testing.T) {
	root := t.TempDir()
	bin := t.TempDir()
	logPath := filepath.Join(root, "commands.log")
	writeExecutable(t, filepath.Join(bin, "tesseract"), "#!/bin/sh\nif [ \"$1\" = \"--list-langs\" ]; then printf '%s\\n' eng jpn jpn_vert; exit 0; fi\nexit 0\n")
	writeExecutable(t, filepath.Join(bin, "seconv"), `#!/bin/sh
language=""
for argument do
  case "$argument" in
	    --ocr-language:*) language="${argument#--ocr-language:}" ;;
  esac
done
printf '%s\n' "$language" >> "$JPN_OCR_LOG"
printf 'No subtitles recognised\n' >&2
exit 17
`)
	t.Setenv("JPN_OCR_LOG", logPath)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	stream := FFProbeStream{Index: 4, CodecName: "dvd_subtitle", Tags: map[string]string{"language": "jpn"}}
	_, err := runBitmapOCR(context.Background(), filepath.Join(root, "Episode.mkv"), stream, "srt", "jpn", "raw", filepath.Join(root, "out.srt"), filepath.Join(root, "raw.srt"), nil)
	if err == nil || !strings.Contains(err.Error(), "Japanese orientations jpn and jpn_vert") || !strings.Contains(err.Error(), "check subtitle orientation") {
		t.Fatalf("neither-orientation failure was not concise/actionable: %v", err)
	}
	log, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if got, want := strings.TrimSpace(string(log)), "jpn\njpn_vert"; got != want {
		t.Fatalf("unexpected orientation attempts: got %q want %q", got, want)
	}
}

func TestJapanesePGSBothUsesOneCanonicalOCRResult(t *testing.T) {
	root := t.TempDir()
	bin := t.TempDir()
	logPath := filepath.Join(root, "commands.log")
	writeExecutable(t, filepath.Join(bin, "ffprobe"), "#!/bin/sh\nprintf '%s' '{\"streams\":[{\"index\":4,\"codec_type\":\"subtitle\",\"codec_name\":\"hdmv_pgs_subtitle\",\"tags\":{\"language\":\"jpn\"}}]}'\n")
	writeExecutable(t, filepath.Join(bin, "tesseract"), "#!/bin/sh\nif [ \"$1\" = \"--list-langs\" ]; then printf '%s\\n' eng jpn jpn_vert; exit 0; fi\nexit 0\n")
	writeExecutable(t, filepath.Join(bin, "ffmpeg"), `#!/bin/sh
output=""
for argument do output="$argument"; done
case "$output" in
  *.sup) printf '%s\n' extract >> "$JPN_OCR_LOG"; printf 'PGS' > "$output" ;;
  *.ass) printf '%s\n' render >> "$JPN_OCR_LOG"; printf '%s\n' '[Script Info]' '[Events]' 'Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text' 'Dialogue: 0,0:00:01.00,0:00:02.00,Default,,Japanese OCR' > "$output" ;;
esac
`)
	writeExecutable(t, filepath.Join(bin, "seconv"), `#!/bin/sh
output=""
language=""
for argument do
  case "$argument" in
	    --ocr-language:*) language="${argument#--ocr-language:}" ;;
	    --output-filename:*) output="${argument#--output-filename:}" ;;
  esac
done
if [ -n "$language" ]; then printf '%s\n' "ocr-$language" >> "$JPN_OCR_LOG"; fi
if [ "$language" = "jpn" ]; then printf '%s\n' 'No subtitles recognised' >&2; exit 17; fi
if [ -n "$output" ]; then printf '%s\n' '1' '00:00:01,000 --> 00:00:02,000' 'Japanese OCR' > "$output"; fi
`)
	t.Setenv("JPN_OCR_LOG", logPath)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	plan := MediaJobPlan{
		InputPath: filepath.Join(root, "Episode.mkv"), OutputPath: filepath.Join(root, "Episode.mkv"),
		Streams: MediaStreamInventory{Subtitle: []MediaStream{{Index: 4, Codec: "hdmv_pgs_subtitle", Language: "jpn"}}},
		ResolvedTracks: &ResolvedTrackPlan{SidecarOutputs: []ResolvedTrackSidecar{
			{StreamIndex: 4, Codec: "hdmv_pgs_subtitle", SourceType: "bitmap", Language: "jpn", Format: "srt", RequestedFormat: "both", Mode: "converted", OCRRequired: true, OCRLanguage: "jpn", OCRMode: "raw", LogicalTransformKey: "subtitle:4:converted:both"},
			{StreamIndex: 4, Codec: "hdmv_pgs_subtitle", SourceType: "bitmap", Language: "jpn", Format: "ass", RequestedFormat: "both", Mode: "converted", OCRRequired: true, OCRLanguage: "jpn", OCRMode: "raw", LogicalTransformKey: "subtitle:4:converted:both"},
		}},
	}
	artifacts, err := generateResolvedSubtitleArtifacts(context.Background(), plan, nil)
	if err != nil || len(artifacts) != 2 {
		t.Fatalf("Japanese both artifacts=%#v err=%v", artifacts, err)
	}
	for _, artifact := range artifacts {
		if artifact.OCRLanguage != "jpn" || artifact.EffectiveOCRLanguage != "jpn_vert" {
			t.Fatalf("Japanese fallback language metadata=%#v", artifact)
		}
		content, readErr := os.ReadFile(artifact.StagedPath)
		if readErr != nil || !strings.Contains(string(content), "Japanese OCR") {
			t.Fatalf("Japanese both artifact=%#v content=%q err=%v", artifact, content, readErr)
		}
	}
	log, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if got, want := strings.TrimSpace(string(log)), "extract\nocr-jpn\nocr-jpn_vert\nrender"; got != want {
		t.Fatalf("Japanese both duplicated OCR or extraction: got %q want %q", got, want)
	}
}
