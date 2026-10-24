package handlers

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFrenchKoreanBitmapOCRContainerE2E(t *testing.T) {
	mediaPath := strings.TrimSpace(os.Getenv("MVFORGE_OCR_E2E_MEDIA"))
	if mediaPath == "" {
		t.Skip("set MVFORGE_OCR_E2E_MEDIA through scripts/e2e/subtitle_ocr_runtime_e2e.py to run real SeConv/Tesseract in the supported container")
	}
	mediaPath, err := filepath.Abs(mediaPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(os.Getenv("MVFORGE_OCR_LIFECYCLE_ROOT")) == "" {
		t.Fatal("MVFORGE_OCR_LIFECYCLE_ROOT must point to the disposable smoke directory")
	}

	before, err := os.ReadFile(mediaPath)
	if err != nil {
		t.Fatalf("read synthetic MKV: %v", err)
	}
	beforeHash := sha256.Sum256(before)
	streams, err := probeSubtitleStreams(context.Background(), mediaPath)
	if err != nil {
		t.Fatalf("probe synthetic subtitle streams: %v", err)
	}

	type scenario struct {
		language string
		alias    string
		title    string
		index    int
	}
	scenarios := []scenario{
		{language: "fra", alias: "fr-FR", title: "synthetic-french", index: 1},
		{language: "kor", alias: "ko-KR", title: "synthetic-korean", index: 2},
	}
	selected := make(map[string]FFProbeStream, len(scenarios))
	for _, stream := range streams {
		if stream.CodecName != "hdmv_pgs_subtitle" {
			t.Fatalf("unexpected synthetic subtitle codec at stream %d: %q", stream.Index, stream.CodecName)
		}
		language := stream.Tags["language"]
		if language == "fra" || language == "kor" {
			if _, duplicate := selected[language]; duplicate {
				t.Fatalf("duplicate synthetic stream for %s", language)
			}
			selected[language] = stream
		}
	}
	for _, scenario := range scenarios {
		stream, ok := selected[scenario.language]
		if !ok {
			t.Fatalf("ffprobe did not find %s bitmap stream; streams=%+v", scenario.language, streams)
		}
		if stream.Index != scenario.index || stream.Tags["title"] != scenario.title {
			t.Fatalf("wrong source stream selected for %s: index=%d title=%q", scenario.language, stream.Index, stream.Tags["title"])
		}

		for _, format := range []string{"srt", "ass"} {
			outputPath := filepath.Join(filepath.Dir(mediaPath), "backend-output", scenario.language+"."+format)
			result, err := generateBitmapSubtitleAtPathWithResult(
				context.Background(), mediaPath, stream, format, scenario.alias, "raw", outputPath, 0, 0, nil,
			)
			if err != nil {
				t.Fatalf("real %s OCR for stream %d (%s): %v", format, stream.Index, scenario.language, err)
			}
			if result.EffectiveLanguage != scenario.language {
				t.Fatalf("%s alias %q resolved to effective OCR language %q, want %q", scenario.language, scenario.alias, result.EffectiveLanguage, scenario.language)
			}
			if result.Evidence == nil || !result.Evidence.Complete {
				t.Fatalf("%s stream %d did not produce complete canonical OCR evidence", scenario.language, stream.Index)
			}
			identity := result.Evidence.Identity
			if identity.SourcePath != mediaPath || identity.StreamIndex != stream.Index || identity.StreamCodec != stream.CodecName || identity.RequestedLanguage != scenario.language {
				t.Fatalf("canonical evidence lost source/stream/language identity: %+v", identity)
			}
			if len(result.Cues) != 2 {
				t.Fatalf("%s %s produced %d cues, want 2 timed source cues; OCR text is intentionally not required to be perfect: %+v", scenario.language, format, len(result.Cues), result.Cues)
			}
			if len(result.Geometry) != 2 {
				t.Fatalf("%s produced %d geometry records, want 2: %+v", scenario.language, len(result.Geometry), result.Geometry)
			}

			wantTimes := [][2]float64{{1.0, 2.5}, {3.0, 4.5}}
			for index, want := range wantTimes {
				cue := result.Cues[index]
				if ocrE2EAbsFloat(cue.Start-want[0]) > 0.02 || ocrE2EAbsFloat(cue.End-want[1]) > 0.02 {
					t.Fatalf("%s %s cue %d timing got %.3f..%.3f, want %.3f..%.3f", scenario.language, format, index, cue.Start, cue.End, want[0], want[1])
				}
				if strings.TrimSpace(cue.Text) == "" {
					t.Fatalf("%s %s cue %d has no OCR text", scenario.language, format, index)
				}
				geometry := result.Geometry[index]
				if ocrE2EAbsFloat(geometry.Start-want[0]) > 0.02 || ocrE2EAbsFloat(geometry.End-want[1]) > 0.02 {
					t.Fatalf("%s geometry cue %d timing got %.3f..%.3f, want %.3f..%.3f", scenario.language, index, geometry.Start, geometry.End, want[0], want[1])
				}
				if geometry.CanvasWidth != 1280 || geometry.CanvasHeight != 720 || geometry.BitmapX != 320 || geometry.BitmapY != 590 || geometry.BitmapWidth != 640 || geometry.BitmapHeight != 120 {
					t.Fatalf("%s cue %d geometry got %+v; want 1280x720 canvas and bitmap 640x120 at 320,590", scenario.language, index, geometry)
				}
				if cue.Geometry == nil || cue.Geometry.BitmapX != geometry.BitmapX || cue.Geometry.BitmapY != geometry.BitmapY {
					t.Fatalf("%s %s cue %d lost correlated geometry: cue=%+v raw=%+v", scenario.language, format, index, cue.Geometry, geometry)
				}
			}

			content, err := os.ReadFile(outputPath)
			if err != nil {
				t.Fatalf("read %s sidecar: %v", format, err)
			}
			if !validSubtitleSidecar(format, content) {
				t.Fatalf("invalid %s sidecar from actual OCR: %q", format, content)
			}
			parsed := parseCanonicalBitmapSubtitleCues(format, content)
			if len(parsed) != 2 {
				t.Fatalf("%s sidecar parsed %d cues: %q", format, len(parsed), content)
			}
			for index, want := range wantTimes {
				if ocrE2EAbsFloat(parsed[index].Start-want[0]) > 0.02 || ocrE2EAbsFloat(parsed[index].End-want[1]) > 0.02 || strings.TrimSpace(parsed[index].Text) == "" {
					t.Fatalf("%s sidecar cue %d lost timing/text: %+v", format, index, parsed[index])
				}
				canonicalText := normalizeOCRSidecarText(format, result.Cues[index].Text)
				sidecarText := normalizeOCRSidecarText(format, parsed[index].Text)
				if sidecarText != canonicalText {
					t.Fatalf("%s sidecar cue %d text does not match canonical OCR evidence: sidecar=%q canonical=%q", format, index, sidecarText, canonicalText)
				}
			}
			t.Logf("scenario=PASS language=%s alias=%s stream_index=%d format=%s cue_count=%d geometry=%+v observed_text=%q", scenario.language, scenario.alias, stream.Index, format, len(result.Cues), result.Geometry, []string{result.Cues[0].Text, result.Cues[1].Text})
		}
	}

	after, err := os.ReadFile(mediaPath)
	if err != nil {
		t.Fatalf("reread synthetic MKV: %v", err)
	}
	afterHash := sha256.Sum256(after)
	if beforeHash != afterHash {
		t.Fatal("OCR flow modified the synthetic source MKV")
	}
	afterStreams, err := probeSubtitleStreams(context.Background(), mediaPath)
	if err != nil {
		t.Fatalf("re-probe synthetic subtitle streams: %v", err)
	}
	if fmt.Sprint(streams) != fmt.Sprint(afterStreams) {
		t.Fatalf("source stream metadata changed during read-only OCR: before=%+v after=%+v", streams, afterStreams)
	}
}

func ocrE2EAbsFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func normalizeOCRSidecarText(format, text string) string {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	if strings.EqualFold(strings.TrimSpace(format), "ass") && len(text) >= 6 {
		tag := text[:6]
		if strings.HasPrefix(tag, "{\\an") && tag[5] == '}' && tag[4] >= '1' && tag[4] <= '9' {
			text = text[6:]
		}
	}
	return strings.TrimSpace(text)
}
