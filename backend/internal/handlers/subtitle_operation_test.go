package handlers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteSubtitleExtractionTargetsBitmapBothSharesCanonicalEvidence(t *testing.T) {
	root := t.TempDir()
	bin := t.TempDir()
	fixturePath := filepath.Join(root, "input.sup")
	logPath := filepath.Join(root, "commands.log")
	mediaPath := filepath.Join(root, "movie.mkv")
	workspace := filepath.Join(root, "workspace")
	if err := os.WriteFile(mediaPath, []byte("media"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixturePath, testPGSSUP(t, []struct{ id, x, y, width, height int }{
		{id: 1, x: 600, y: 80, width: 720, height: 120},
		{id: 2, x: 500, y: 850, width: 920, height: 130},
	}), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	t.Setenv("BITMAP_POSITIONING_FIXTURE", fixturePath)
	t.Setenv("BITMAP_POSITIONING_LOG", logPath)
	writeExecutable(t, filepath.Join(bin, "tesseract"), "#!/bin/sh\nprintf '%s\\n' spa\n")
	writeExecutable(t, filepath.Join(bin, "ffmpeg"), `#!/bin/sh
printf '%s\n' ffmpeg >> "$BITMAP_POSITIONING_LOG"
output=""
for argument do output="$argument"; done
case "$output" in *.sup) cp "$BITMAP_POSITIONING_FIXTURE" "$output" ;; esac
`)
	writeExecutable(t, filepath.Join(bin, "seconv"), `#!/bin/sh
printf '%s\n' seconv >> "$BITMAP_POSITIONING_LOG"
output=""
for argument do
  case "$argument" in
    --output-filename:*) output="${argument#--output-filename:}" ;;
  esac
done
printf '%s\n' '1' '00:00:01,000 --> 00:00:02,000' 'Corrected Top' '2' '00:00:01,000 --> 00:00:02,000' 'Corrected Bottom' > "$output"
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}

	stream := FFProbeStream{Index: 4, CodecName: "hdmv_pgs_subtitle", Tags: map[string]string{"language": "spa"}}
	logicalTransform := "subtitle:4:converted:both"
	targets := make([]subtitleExtractionTarget, 0, 2)
	for _, format := range []string{"srt", "ass"} {
		finalPath := filepath.Join(root, "movie.spa.4."+format)
		plan := subtitleExtractionPlan{
			StreamIndex: 4,
			Mode:        "converted",
			Format:      format,
			OutputPath:  finalPath,
			Decision: ResolvedTrackSidecar{
				StreamIndex: 4, Codec: "hdmv_pgs_subtitle", SourceType: "bitmap", Language: "spa",
				Format: format, RequestedFormat: "both", Mode: "converted", OCRRequired: true,
				OCRLanguage: "spa", OCRMode: "raw", LogicalTransformKey: logicalTransform,
			},
		}
		targets = append(targets, subtitleExtractionTarget{
			plan: plan, stream: stream, finalPath: finalPath,
			stagePath: filepath.Join(workspace, "movie.spa.4."+format),
			artifact:  subtitleExtractionArtifact(plan, finalPath),
		})
	}

	created, _, err := executeSubtitleExtractionTargets(context.Background(), "task4-bitmap-both", workspace, mediaPath, targets)
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 2 {
		t.Fatalf("created artifacts=%#v", created)
	}
	if created[0].LogicalTransformKey != logicalTransform || created[1].LogicalTransformKey != logicalTransform {
		t.Fatalf("fallback outputs lost logical transform identity: %#v", created)
	}
	if created[0].CanonicalEvidence == nil || created[1].CanonicalEvidence == nil {
		t.Fatalf("fallback outputs did not retain canonical evidence: %#v", created)
	}
	if created[0].CanonicalEvidence != created[1].CanonicalEvidence {
		t.Fatal("SRT and ASS did not share the operation's canonical evidence object")
	}
	if created[0].CanonicalEvidence.Identity.key() != created[1].CanonicalEvidence.Identity.key() {
		t.Fatal("SRT and ASS canonical evidence identities differ")
	}

	srt, err := os.ReadFile(targets[0].stagePath)
	if err != nil {
		t.Fatal(err)
	}
	ass, err := os.ReadFile(targets[1].stagePath)
	if err != nil {
		t.Fatal(err)
	}
	srtText, assText := string(srt), string(ass)
	for _, text := range []string{"Corrected Top", "Corrected Bottom"} {
		if !strings.Contains(srtText, text) || !strings.Contains(assText, text) {
			t.Fatalf("canonical cue %q was not shared by SRT=%q ASS=%q", text, srtText, assText)
		}
	}
	if strings.Contains(srtText, `\an`) {
		t.Fatalf("SRT serializer leaked bitmap geometry: %q", srtText)
	}
	if !strings.Contains(assText, `{\an8}Corrected Top`) || !strings.Contains(assText, `{\an2}Corrected Bottom`) {
		t.Fatalf("ASS serializer lost canonical bitmap geometry: %q", assText)
	}

	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(string(log)), "ffmpeg\nseconv"; got != want {
		t.Fatalf("bitmap fallback ran more than one OCR/evidence path or serializer render: got %q want %q", got, want)
	}
}

func TestBitmapBothFailureDoesNotPartiallyPublishOrReplaceExistingSidecars(t *testing.T) {
	root := t.TempDir()
	bin := t.TempDir()
	mediaPath := filepath.Join(root, "movie.mkv")
	workspace := filepath.Join(root, "workspace")
	srtPath := filepath.Join(root, "movie.spa.4.srt")
	assPath := filepath.Join(root, "movie.spa.4.ass")
	if err := os.WriteFile(mediaPath, []byte("media"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeTestSRT(t, srtPath, "Existing SRT")
	if err := os.WriteFile(assPath, []byte("[Script Info]\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,Existing ASS\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "ocr-cache"))
	writeExecutable(t, filepath.Join(bin, "tesseract"), "#!/bin/sh\nprintf '%s\\n' spa\n")
	writeExecutable(t, filepath.Join(bin, "seconv"), `#!/bin/sh
output=""
for argument do
  case "$argument" in
    --output-filename:*) output="${argument#--output-filename:}" ;;
  esac
done
printf '%s\n' '1' '00:00:01,000 --> 00:00:02,000' 'New OCR text' > "$output"
`)
	writeExecutable(t, filepath.Join(bin, "ffmpeg"), "#!/bin/sh\nprintf '%s\\n' 'ASS render failed' >&2\nexit 17\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}

	stream := FFProbeStream{Index: 4, CodecName: "dvd_subtitle", Tags: map[string]string{"language": "spa"}}
	logicalTransform := "subtitle:4:converted:both"
	targets := make([]subtitleExtractionTarget, 0, 2)
	for _, format := range []string{"srt", "ass"} {
		finalPath := filepath.Join(root, "movie.spa.4."+format)
		plan := subtitleExtractionPlan{
			StreamIndex: 4, Mode: "converted", Format: format, OutputPath: finalPath,
			Decision: ResolvedTrackSidecar{
				StreamIndex: 4, Codec: "dvd_subtitle", SourceType: "bitmap", Language: "spa",
				Format: format, RequestedFormat: "both", Mode: "converted", OCRRequired: true,
				OCRLanguage: "spa", OCRMode: "raw", LogicalTransformKey: logicalTransform,
			},
		}
		targets = append(targets, subtitleExtractionTarget{
			plan: plan, stream: stream, finalPath: finalPath,
			stagePath: filepath.Join(workspace, "movie.spa.4."+format),
			artifact:  subtitleExtractionArtifact(plan, finalPath),
		})
	}
	operationID := "task4-bitmap-both-failure"
	operation := &subtitleExtractionOperation{ID: operationID, Status: "running", cancel: func() {}, publicationCommittedCh: make(chan struct{})}
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[operationID] = operation
	subtitleExtractionOperations.Unlock()
	defer func() {
		subtitleExtractionOperations.Lock()
		delete(subtitleExtractionOperations.items, operationID)
		subtitleExtractionOperations.Unlock()
	}()

	runSubtitleExtractionOperation(context.Background(), operationID, workspace, mediaPath, SubtitleExtractionResult{}, targets)
	if status := snapshotSubtitleOperation(operationID); status.Status != "error" {
		t.Fatalf("ASS member failure did not fail the logical both operation: %#v", status)
	}
	srt, err := os.ReadFile(srtPath)
	if err != nil || !strings.Contains(string(srt), "Existing SRT") || strings.Contains(string(srt), "New OCR text") {
		t.Fatalf("failed both conversion replaced SRT: %q (%v)", srt, err)
	}
	ass, err := os.ReadFile(assPath)
	if err != nil || !strings.Contains(string(ass), "Existing ASS") || strings.Contains(string(ass), "New OCR text") {
		t.Fatalf("failed both conversion replaced ASS: %q (%v)", ass, err)
	}
}
