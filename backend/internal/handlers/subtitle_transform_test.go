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
	assertContains(t, segmented, "-ss 37.5 -i /raw/movie.mkv -ss 5 -t 20 -map 0:3")
	normal := shellJoin(textSubtitleExtractionArgs(MediaJobPlan{InputPath: "/raw/movie.mkv"}, 3, "srt", "/staging/full.srt"))
	assertNotContains(t, normal, "-ss")
	assertNotContains(t, normal, "-t 20")
	ass := shellJoin(textSubtitleExtractionArgs(MediaJobPlan{InputPath: "/raw/movie.mkv"}, 3, "ass", "/staging/full.ass"))
	assertContains(t, ass, "-map 0:3 -vn -an -c:s ass -f ass /staging/full.ass")
}

func TestGenerateResolvedConvertedTextSubtitleArtifactsProducesCues(t *testing.T) {
	bin := t.TempDir()
	ffmpeg := filepath.Join(bin, "ffmpeg")
	script := "#!/bin/sh\noutput=\"\"\nfor argument do output=\"$argument\"; done\ncase \"$output\" in\n  *.ass) printf '%s\\n' '[Events]' 'Format: Layer, Start, End, Style, Text' 'Dialogue: 0,0:00:01.00,0:00:02.00,Default,,Hola' > \"$output\" ;;\n  *) printf '%s\\n' '1' '00:00:01,000 --> 00:00:02,000' 'Hola' > \"$output\" ;;\nesac\n"
	if err := os.WriteFile(ffmpeg, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	for _, test := range []struct {
		name, requested, format string
	}{
		{name: "converted SRT", requested: "srt", format: "srt"},
		{name: "converted ASS", requested: "ass", format: "ass"},
		{name: "default converted SRT", requested: "", format: "srt"},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "Episode.mkv")
			plan := MediaJobPlan{
				InputPath: "/raw/Episode.mkv", OutputPath: output,
				Streams: MediaStreamInventory{Subtitle: []MediaStream{{Index: 3, Codec: "subrip", Language: "eng"}}},
				ResolvedTracks: &ResolvedTrackPlan{SidecarOutputs: []ResolvedTrackSidecar{{
					StreamIndex: 3, Codec: "subrip", SourceType: "text", Language: "eng", Format: test.format,
					RequestedFormat: test.requested, Mode: "converted", Extension: "." + test.format,
					Stages: []string{"extract", "normalize", "render"},
				}}},
			}
			artifacts, err := generateResolvedSubtitleArtifacts(context.Background(), plan, nil)
			if err != nil || len(artifacts) != 1 {
				t.Fatalf("artifacts=%#v err=%v", artifacts, err)
			}
			content, err := os.ReadFile(artifacts[0].StagedPath)
			if err != nil {
				t.Fatal(err)
			}
			if test.format == "srt" && !strings.Contains(string(content), "-->") {
				t.Fatalf("SRT artifact has no cue: %q", content)
			}
			if test.format == "ass" && (!strings.Contains(string(content), "[Events]") || !strings.Contains(string(content), "Dialogue:")) {
				t.Fatalf("ASS artifact has no normalized cue: %q", content)
			}
		})
	}
}

func TestGenerateResolvedBitmapOCRArtifactsForPGSAndVobSub(t *testing.T) {
	for _, test := range []struct {
		codec, format string
	}{
		{codec: "hdmv_pgs_subtitle", format: "srt"},
		{codec: "hdmv_pgs_subtitle", format: "ass"},
		{codec: "dvd_subtitle", format: "srt"},
		{codec: "dvd_subtitle", format: "ass"},
	} {
		t.Run(test.codec+"_"+test.format, func(t *testing.T) {
			bin := t.TempDir()
			scripts := map[string]string{
				"ffprobe":   "#!/bin/sh\nprintf '%s' '{\"streams\":[{\"index\":4,\"id\":\"0x7\",\"codec_type\":\"subtitle\",\"codec_name\":\"" + test.codec + "\",\"tags\":{\"language\":\"spa\"}}]}'\n",
				"ffmpeg":    "#!/bin/sh\noutput=\"\"\nfor argument do output=\"$argument\"; done\nprintf 'PG' > \"$output\"\n",
				"seconv":    "#!/bin/sh\noutput=\"\"\nfor argument do case \"$argument\" in --output-filename:*) output=\"${argument#--output-filename:}\" ;; esac; done\ncase \"$output\" in\n *.ass) printf '%s\\n' '[Events]' 'Format: Layer, Start, End, Style, Text' 'Dialogue: 0,0:00:01.00,0:00:02.00,Default,,Hola OCR' > \"$output\" ;;\n *) printf '%s\\n' '1' '00:00:01,000 --> 00:00:02,000' 'Hola OCR' > \"$output\" ;;\nesac\n",
				"tesseract": "#!/bin/sh\nif [ \"$1\" = \"--list-langs\" ]; then printf '%s\\n' eng spa jpn jpn_vert; exit 0; fi\nexit 0\n",
			}
			for name, script := range scripts {
				if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			output := filepath.Join(t.TempDir(), "Episode.mkv")
			plan := MediaJobPlan{
				InputPath: "/raw/Episode.mkv", OutputPath: output,
				Streams: MediaStreamInventory{Subtitle: []MediaStream{{Index: 4, Codec: test.codec, Language: "spa"}}},
				ResolvedTracks: &ResolvedTrackPlan{SidecarOutputs: []ResolvedTrackSidecar{{
					StreamIndex: 4, Codec: test.codec, SourceType: "bitmap", Language: "spa", Format: test.format,
					RequestedFormat: test.format, Mode: "converted", OCRRequired: true, OCRLanguage: "spa", OCRMode: "raw",
				}}},
			}
			artifacts, err := generateResolvedSubtitleArtifacts(context.Background(), plan, nil)
			if err != nil || len(artifacts) != 1 {
				t.Fatalf("artifacts=%#v err=%v", artifacts, err)
			}
			content, err := os.ReadFile(artifacts[0].StagedPath)
			if err != nil || !strings.Contains(string(content), "Hola OCR") {
				t.Fatalf("OCR artifact content=%q err=%v", content, err)
			}
		})
	}
}

func TestGenerateResolvedBitmapBothUsesOneOCRAndRendersBothArtifacts(t *testing.T) {
	bin := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "ocr.log")
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(t.TempDir(), "ocr-cache"))
	t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", filepath.Join(t.TempDir(), "ocr-persistent"))
	scripts := map[string]string{
		"ffprobe":   "#!/bin/sh\nprintf '%s' '{\"streams\":[{\"index\":4,\"codec_type\":\"subtitle\",\"codec_name\":\"dvd_subtitle\",\"tags\":{\"language\":\"spa\"}}]}'\n",
		"seconv":    "#!/bin/sh\nprintf 'seconv\\n' >> \"$MVFORGE_OCR_BOTH_LOG\"\noutput=\"\"\nfor argument do case \"$argument\" in --output-filename:*) output=\"${argument#--output-filename:}\" ;; esac; done\nprintf '%s\\n' '1' '00:00:01,000 --> 00:00:02,000' 'Hola OCR' > \"$output\"\n",
		"ffmpeg":    "#!/bin/sh\nprintf 'ffmpeg\\n' >> \"$MVFORGE_OCR_BOTH_LOG\"\noutput=\"\"\nfor argument do output=\"$argument\"; done\nprintf '%s\\n' '[Events]' 'Format: Layer, Start, End, Style, Text' 'Dialogue: 0,0:00:01.00,0:00:02.00,Default,,Hola OCR' > \"$output\"\n",
		"tesseract": "#!/bin/sh\nif [ \"$1\" = \"--list-langs\" ]; then printf '%s\\n' eng spa jpn jpn_vert; exit 0; fi\nexit 0\n",
	}
	for name, script := range scripts {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MVFORGE_OCR_BOTH_LOG", logPath)
	plan := MediaJobPlan{
		InputPath:       "/raw/Episode.mkv",
		OutputPath:      filepath.Join(t.TempDir(), "Episode.mkv"),
		SubtitleStyling: testEnabledSubtitleStylePolicy(),
		Streams:         MediaStreamInventory{Subtitle: []MediaStream{{Index: 4, Codec: "dvd_subtitle", Language: "spa"}}},
		ResolvedTracks: &ResolvedTrackPlan{SidecarOutputs: []ResolvedTrackSidecar{
			{StreamIndex: 4, Codec: "dvd_subtitle", SourceType: "bitmap", Language: "spa", Format: "srt", RequestedFormat: "both", Mode: "converted", OCRRequired: true, OCRLanguage: "spa", OCRMode: "raw", LogicalTransformKey: "subtitle:4:converted:both"},
			{StreamIndex: 4, Codec: "dvd_subtitle", SourceType: "bitmap", Language: "spa", Format: "ass", RequestedFormat: "both", Mode: "converted", OCRRequired: true, OCRLanguage: "spa", OCRMode: "raw", LogicalTransformKey: "subtitle:4:converted:both"},
		}},
	}
	artifacts, err := generateResolvedSubtitleArtifacts(context.Background(), plan, nil)
	if err != nil || len(artifacts) != 2 {
		t.Fatalf("bitmap both artifacts=%#v err=%v", artifacts, err)
	}
	if artifacts[0].Format != "srt" || artifacts[1].Format != "ass" || artifacts[0].LogicalTransformKey != artifacts[1].LogicalTransformKey {
		t.Fatalf("bitmap both artifact identity=%#v", artifacts)
	}
	for _, artifact := range artifacts {
		content, readErr := os.ReadFile(artifact.StagedPath)
		if readErr != nil || !strings.Contains(string(content), "Hola OCR") {
			t.Fatalf("bitmap both artifact=%#v content=%q err=%v", artifact, content, readErr)
		}
	}
	srtContent, _ := os.ReadFile(artifacts[0].StagedPath)
	assContent, _ := os.ReadFile(artifacts[1].StagedPath)
	if strings.Contains(string(srtContent), "\\an") || strings.Contains(string(srtContent), "MVF_") {
		t.Fatalf("bitmap both SRT was not text/timing-neutral: %q", srtContent)
	}
	if !strings.Contains(string(assContent), "MVF_") {
		t.Fatalf("bitmap both ASS did not consume configured style authority: %q", assContent)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Split(strings.TrimSpace(string(log)), "\n"); len(got) != 1 || got[0] != "seconv" {
		t.Fatalf("bitmap both did not invoke the canonical OCR lifecycle exactly once: %q", log)
	}
}

func TestGenerateResolvedBitmapOCRFailureDoesNotLeaveArtifact(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "ffprobe"), []byte("#!/bin/sh\nprintf '%s' '{\"streams\":[{\"index\":4,\"codec_type\":\"subtitle\",\"codec_name\":\"hdmv_pgs_subtitle\"}]}'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "ffmpeg"), []byte("#!/bin/sh\noutput=\"\"\nfor argument do output=\"$argument\"; done\nprintf 'PG' > \"$output\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "seconv"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "tesseract"), []byte("#!/bin/sh\nif [ \"$1\" = \"--list-langs\" ]; then printf '%s\\n' eng spa jpn jpn_vert; exit 0; fi\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	output := filepath.Join(t.TempDir(), "Episode.mkv")
	plan := MediaJobPlan{
		InputPath: "/raw/Episode.mkv", OutputPath: output,
		Streams: MediaStreamInventory{Subtitle: []MediaStream{{Index: 4, Codec: "hdmv_pgs_subtitle"}}},
		ResolvedTracks: &ResolvedTrackPlan{SidecarOutputs: []ResolvedTrackSidecar{{
			StreamIndex: 4, Codec: "hdmv_pgs_subtitle", SourceType: "bitmap", Format: "srt", Mode: "converted", OCRRequired: true,
		}}},
	}
	artifacts, err := generateResolvedSubtitleArtifacts(context.Background(), plan, nil)
	if err == nil || !strings.Contains(err.Error(), "OCR failed") {
		t.Fatalf("expected explicit OCR failure, artifacts=%#v err=%v", artifacts, err)
	}
	for _, artifact := range artifacts {
		if artifact.StagedPath != "" {
			if _, statErr := os.Stat(artifact.StagedPath); !os.IsNotExist(statErr) {
				t.Fatalf("failed OCR left misleading artifact %q: %v", artifact.StagedPath, statErr)
			}
		}
	}
}

func TestOptionalZeroCuePGSSidecarsLeaveQueueArtifactsTerminal(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		format string
	}{
		{name: "single format", format: "srt"},
		{name: "shared both formats", format: "both"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			mediaPath := filepath.Join(root, "Episode.mkv")
			if err := os.WriteFile(mediaPath, []byte("synthetic source"), 0o600); err != nil {
				t.Fatal(err)
			}
			marker := installZeroCuePGSTestTools(t)

			decisions := []ResolvedTrackSidecar{{
				StreamIndex: 4, Codec: "hdmv_pgs_subtitle", SourceType: "bitmap", Language: "eng",
				Format: "srt", RequestedFormat: testCase.format, Mode: "converted", OCRRequired: true,
				OCRLanguage: "eng", OCRMode: "accurate",
			}}
			if testCase.format == "both" {
				logicalKey := "subtitle:4:converted:both"
				decisions[0].LogicalTransformKey = logicalKey
				decisions = append(decisions, decisions[0])
				decisions[1].Format = "ass"
			}
			for index := range decisions {
				if decisions[index].LogicalTransformKey == "" {
					decisions[index].LogicalTransformKey = subtitleArtifactID(4, "converted", decisions[index].Format)
				}
			}
			plan := MediaJobPlan{
				InputPath: mediaPath, OutputPath: filepath.Join(root, "Episode.partial.mkv"),
				SegmentDurationSeconds: 20, AllowEmptySubtitleArtifacts: true,
				Streams:        MediaStreamInventory{Subtitle: []MediaStream{{Index: 4, Codec: "hdmv_pgs_subtitle", Language: "eng"}}},
				ResolvedTracks: &ResolvedTrackPlan{SidecarOutputs: decisions},
			}

			planned := make([]SubtitleArtifact, 0, len(decisions))
			for _, decision := range decisions {
				planned = append(planned, SubtitleArtifact{
					ArtifactID: subtitleArtifactID(4, "converted", decision.Format), Type: "subtitle_sidecar",
					StreamIndex: 4, SourceCodec: decision.Codec, Format: decision.Format, Mode: "converted",
					Status: "planned", StagedPath: filepath.Join(root, "planned."+decision.Format),
				})
			}
			job := models.QueueJob{Status: JobStatusRunning, SubtitleArtifacts: sidecarArtifactsJSON(planned, nil)}
			db := queueJobTestDB(t)
			if err := db.Create(&job).Error; err != nil {
				t.Fatal(err)
			}
			progress := func(update SubtitleArtifact) {
				current := subtitleArtifactsFromJSON(job.SubtitleArtifacts)
				job.SubtitleArtifacts = sidecarArtifactsJSON(
					mergeSubtitleArtifactProgress(current, update),
					fontAttachmentArtifactsFromJSON(job.SubtitleArtifacts),
				)
				if err := db.Model(&job).Update("subtitle_artifacts", job.SubtitleArtifacts).Error; err != nil {
					t.Errorf("persist QueueJob sidecar progress: %v", err)
				}
			}

			artifacts, err := generateSubtitleArtifactsWithProgress(context.Background(), plan, progress)
			if err != nil {
				job.Status = JobStatusFailed
				job.ErrorMessage = err.Error()
				_ = db.Save(&job).Error
				t.Fatalf("optional zero-cue sidecar failed QueueJob subtitle preparation: %v", err)
			}
			if len(artifacts) != 0 {
				t.Fatalf("optional zero-cue sidecars were returned as publishable artifacts: %#v", artifacts)
			}
			var finalJob models.QueueJob
			if err := db.First(&finalJob, job.ID).Error; err != nil {
				t.Fatal(err)
			}
			if finalJob.Status == JobStatusFailed || finalJob.ErrorMessage != "" {
				t.Fatalf("QueueJob failed during optional sidecar preparation: status=%q error=%q", finalJob.Status, finalJob.ErrorMessage)
			}
			finalArtifacts := subtitleArtifactsFromJSON(finalJob.SubtitleArtifacts)
			if len(finalArtifacts) != len(decisions) {
				t.Fatalf("QueueJob final sidecar artifact count=%d, want %d: %#v", len(finalArtifacts), len(decisions), finalArtifacts)
			}
			for _, artifact := range finalArtifacts {
				if artifact.Status != "skipped" || artifact.StagedPath != "" || artifact.Error == "" {
					t.Errorf("optional zero-cue sidecar did not reach a terminal skipped state: %#v", artifact)
				}
			}
			if log, readErr := os.ReadFile(marker); readErr == nil {
				t.Fatalf("zero-cue PGS invoked OCR tools: %q", log)
			} else if !os.IsNotExist(readErr) {
				t.Fatalf("read OCR invocation marker: %v", readErr)
			}
		})
	}
}

func TestRequiredZeroCuePGSSidecarStillFailsExplicitly(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Episode.mkv")
	if err := os.WriteFile(mediaPath, []byte("synthetic source"), 0o600); err != nil {
		t.Fatal(err)
	}
	marker := installZeroCuePGSTestTools(t)
	plan := MediaJobPlan{
		InputPath: mediaPath, OutputPath: filepath.Join(root, "Episode.partial.mkv"),
		SegmentDurationSeconds: 20,
		Streams:                MediaStreamInventory{Subtitle: []MediaStream{{Index: 4, Codec: "hdmv_pgs_subtitle", Language: "eng"}}},
		ResolvedTracks: &ResolvedTrackPlan{SidecarOutputs: []ResolvedTrackSidecar{{
			StreamIndex: 4, Codec: "hdmv_pgs_subtitle", SourceType: "bitmap", Language: "eng",
			Format: "srt", RequestedFormat: "srt", Mode: "converted", OCRRequired: true,
			OCRLanguage: "eng", OCRMode: "accurate",
		}}},
	}

	artifacts, err := generateResolvedSubtitleArtifacts(context.Background(), plan, nil)
	if err == nil || !strings.Contains(err.Error(), "no eligible PGS bitmap cues") || !strings.Contains(err.Error(), "OCR was not attempted") {
		t.Fatalf("required zero-cue sidecar did not fail explicitly: artifacts=%#v err=%v", artifacts, err)
	}
	if len(artifacts) != 1 || artifacts[0].Status != "failed" {
		t.Fatalf("required sidecar final state=%#v, want one failed artifact", artifacts)
	}
	if log, readErr := os.ReadFile(marker); readErr == nil {
		t.Fatalf("zero-cue PGS invoked OCR tools: %q", log)
	} else if !os.IsNotExist(readErr) {
		t.Fatalf("read OCR invocation marker: %v", readErr)
	}
}

func installZeroCuePGSTestTools(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	fixturePath := filepath.Join(t.TempDir(), "zero-cue.sup")
	if err := os.WriteFile(fixturePath, testPGSSUP(t, nil), 0o600); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "ocr-tools-started")
	t.Setenv("MVFORGE_TEST_PGS_SUP_FIXTURE", fixturePath)
	t.Setenv("MVFORGE_TEST_OCR_MARKER", marker)
	commands := map[string]string{
		"ffprobe": `#!/bin/sh
printf '%s\n' '{"streams":[{"index":4,"codec_type":"subtitle","codec_name":"hdmv_pgs_subtitle","tags":{"language":"eng"}}]}'
`,
		"ffmpeg": `#!/bin/sh
output=""
for argument do output="$argument"; done
case "$output" in *.sup) cp "$MVFORGE_TEST_PGS_SUP_FIXTURE" "$output" ;; esac
`,
		"tesseract": `#!/bin/sh
if [ "$1" = "--list-langs" ]; then printf '%s\n' eng; exit 0; fi
printf 'tesseract %s\n' "$*" >> "$MVFORGE_TEST_OCR_MARKER"
exit 0
`,
		"seconv": `#!/bin/sh
printf 'seconv %s\n' "$*" >> "$MVFORGE_TEST_OCR_MARKER"
exit 0
`,
	}
	for name, contents := range commands {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(contents), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return marker
}

func TestGenerateBitmapSubtitleBothReportsMissingOCRTools(t *testing.T) {
	for _, test := range []struct {
		name          string
		installSeconv bool
		wantError     string
	}{
		{name: "missing seconv", wantError: "OCR is not available in this backend image (seconv was not found)"},
		{name: "missing tesseract", installSeconv: true, wantError: "OCR is not available in this backend image (Tesseract was not found)"},
	} {
		t.Run(test.name, func(t *testing.T) {
			bin := t.TempDir()
			if test.installSeconv {
				if err := os.WriteFile(filepath.Join(bin, "seconv"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", bin)
			root := t.TempDir()
			destination := filepath.Join(root, "episode.mkv")
			_, err := generateBitmapSubtitleBoth(context.Background(), "/unused/source.mkv", destination, FFProbeStream{Index: 4, CodecName: "hdmv_pgs_subtitle"}, SubtitleExtractionInput{}, nil)
			if err == nil || err.Error() != test.wantError {
				t.Fatalf("missing-tool error=%v, want %q", err, test.wantError)
			}
			for _, extension := range []string{"srt", "ass"} {
				path := strings.TrimSuffix(destination, filepath.Ext(destination)) + ".und.4." + extension
				if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
					t.Fatalf("missing-tool path left output %q: %v", path, statErr)
				}
			}
		})
	}
}

func TestBitmapSubtitleExtractionUsesSameAccurateTestEncodeWindow(t *testing.T) {
	for _, test := range []struct {
		name       string
		start      float64
		want, omit string
	}{
		{name: "ordinary sample", start: 42.5, want: "-ss 37.5 -i /raw/movie.mkv -ss 5 -t 20 -map 0:3"},
		{name: "near beginning", start: 3, want: "-i /raw/movie.mkv -ss 3 -t 20 -map 0:3", omit: "-ss 0"},
		{name: "starts at zero", start: 0, want: "-i /raw/movie.mkv -t 20 -map 0:3", omit: "-ss"},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := shellJoin(bitmapSubtitleSegmentArgs("/raw/movie.mkv", 3, "/tmp/subtitle-segment.mkv", test.start, 20))
			assertContains(t, command, test.want)
			if test.omit != "" {
				assertNotContains(t, command, test.omit)
			}
		})
	}
}

func TestEmptySubtitleArtifactPolicyIsLimitedToSegmentedTestEncodes(t *testing.T) {
	segmentedTest := MediaJobPlan{SegmentDurationSeconds: 20, AllowEmptySubtitleArtifacts: true}
	if !emptySubtitleArtifactCanBeSkipped(segmentedTest) {
		t.Fatal("segmented Test Encode should skip a subtitle track with no cues in its window")
	}
	for _, plan := range []MediaJobPlan{
		{AllowEmptySubtitleArtifacts: true},
		{SegmentDurationSeconds: 20},
	} {
		if emptySubtitleArtifactCanBeSkipped(plan) {
			t.Fatal("full publication and ordinary segmented jobs must reject empty subtitle artifacts")
		}
	}
}

func TestCueFreeASSArtifactPolicyIsLimitedToSegmentedTestEncodes(t *testing.T) {
	segmentedTest := MediaJobPlan{SegmentDurationSeconds: 20, AllowEmptySubtitleArtifacts: true}
	cueFree := []byte("[Script Info]\nScriptType: v4.00+\n\n[Events]\nFormat: Layer, Start, End, Style, Text\n")
	withCue := append(append([]byte(nil), cueFree...), []byte("Dialogue: 0,0:00:01.00,0:00:02.00,Default,Hola\n")...)

	for _, format := range []string{"ass", "ssa"} {
		if !cueFreeSubtitleArtifactCanBeSkipped(segmentedTest, format, cueFree) {
			t.Fatalf("segmented Test Encode should skip cue-free %s", strings.ToUpper(format))
		}
	}
	for _, test := range []struct {
		name    string
		plan    MediaJobPlan
		format  string
		content []byte
	}{
		{name: "full processing", plan: MediaJobPlan{AllowEmptySubtitleArtifacts: true}, format: "ass", content: cueFree},
		{name: "ordinary segment", plan: MediaJobPlan{SegmentDurationSeconds: 20}, format: "ass", content: cueFree},
		{name: "subtitle with cue", plan: segmentedTest, format: "ass", content: withCue},
		{name: "missing script info", plan: segmentedTest, format: "ass", content: []byte("[Events]\nFormat: Layer, Start, End, Style, Text\n")},
		{name: "missing events format", plan: segmentedTest, format: "ass", content: []byte("[Script Info]\nScriptType: v4.00+\n[Events]\n")},
		{name: "non ASS", plan: segmentedTest, format: "srt", content: cueFree},
	} {
		t.Run(test.name, func(t *testing.T) {
			if cueFreeSubtitleArtifactCanBeSkipped(test.plan, test.format, test.content) {
				t.Fatal("artifact was incorrectly treated as an empty ASS/SSA window")
			}
		})
	}
}

func TestOriginalSubtitleExtractionFormats(t *testing.T) {
	tests := []struct {
		codec, format, muxer string
		supported            bool
	}{
		{"ass", "ass", "ass", true},
		{"ssa", "ssa", "ass", true},
		{"subrip", "srt", "srt", true},
		{"hdmv_pgs_subtitle", "sup", "sup", true},
		{"dvd_subtitle", "", "", false},
		{"dvb_subtitle", "", "", false},
	}
	for _, test := range tests {
		format, muxer, supported := originalSubtitleExtractionFormat(test.codec)
		if format != test.format || muxer != test.muxer || supported != test.supported {
			t.Fatalf("codec=%s got=(%s,%s,%t) want=(%s,%s,%t)", test.codec, format, muxer, supported, test.format, test.muxer, test.supported)
		}
	}
}

func TestOriginalSubtitleExtractionUsesStreamCopy(t *testing.T) {
	command := shellJoin(originalSubtitleExtractionArgs(MediaJobPlan{InputPath: "/raw/Movie.mkv"}, 4, "ass", "/staging/Movie.spa.ass"))
	assertContains(t, command, "-map 0:4")
	assertContains(t, command, "-c:s copy")
	assertContains(t, command, "-f ass")
	assertNotContains(t, command, "-c:s ass")
}

func TestExecuteOriginalPGSTargetUsesSourceCodecMuxerWithoutOCR(t *testing.T) {
	bin := t.TempDir()
	ffmpeg := filepath.Join(bin, "ffmpeg")
	logPath := filepath.Join(t.TempDir(), "original-muxer.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$MVFORGE_ORIGINAL_MUXER_LOG\"\noutput=\"\"\nfor argument do output=\"$argument\"; done\nprintf 'PG' > \"$output\"\n"
	if err := os.WriteFile(ffmpeg, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MVFORGE_ORIGINAL_MUXER_LOG", logPath)
	stagePath := filepath.Join(t.TempDir(), "Movie.sup")
	target := &subtitleExtractionTarget{
		plan: subtitleExtractionPlan{
			StreamIndex: 4, Mode: "original", Format: "sup", OutputPath: stagePath,
			Decision: ResolvedTrackSidecar{StreamIndex: 4, Codec: "hdmv_pgs_subtitle", Format: "sup", Mode: "original"},
		},
		stream:    FFProbeStream{Index: 4, CodecName: "hdmv_pgs_subtitle", CodecType: "subtitle"},
		finalPath: stagePath,
		stagePath: stagePath,
		artifact:  SubtitleArtifact{StreamIndex: 4, SourceCodec: "hdmv_pgs_subtitle", Format: "sup", Mode: "original", StagedPath: stagePath},
	}
	artifact, err := executeSingleSubtitleTarget(context.Background(), "", t.TempDir(), "/raw/Movie.mkv", target)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Status != "ready" || artifact.Format != "sup" || target.plan.Decision.OCRRequired {
		t.Fatalf("unexpected original PGS artifact=%#v target=%#v", artifact, target)
	}
	args, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "-f\nsup\n") {
		t.Fatalf("original PGS muxer was not sup: %q", args)
	}
}

func TestResolvedSubtitleStagedPathIsDeterministic(t *testing.T) {
	used := map[string]struct{}{}
	first := resolvedSubtitleStagedPath("/staging/Movie", SubtitleArtifact{StreamIndex: 4, Language: "spa", Format: "ass"}, used)
	second := resolvedSubtitleStagedPath("/staging/Movie", SubtitleArtifact{StreamIndex: 7, Language: "spa", Format: "ass"}, used)
	forced := resolvedSubtitleStagedPath("/staging/Movie", SubtitleArtifact{StreamIndex: 8, Language: "eng", Format: "srt", Forced: true}, used)
	if first != "/staging/Movie.spa.ass" || second != "/staging/Movie.spa.7.ass" || forced != "/staging/Movie.eng.forced.srt" {
		t.Fatalf("unexpected deterministic names: %q %q %q", first, second, forced)
	}
}

func TestResolvedSubtitleStagedPathSharesDuplicateIdentityAcrossFormats(t *testing.T) {
	used := map[string]struct{}{}
	want := []string{
		"/staging/Movie.spa.ass", "/staging/Movie.spa.srt",
		"/staging/Movie.spa.7.ass", "/staging/Movie.spa.7.srt",
	}
	artifacts := []SubtitleArtifact{
		{StreamIndex: 4, Language: "spa", Format: "ass"}, {StreamIndex: 4, Language: "spa", Format: "srt"},
		{StreamIndex: 7, Language: "spa", Format: "ass"}, {StreamIndex: 7, Language: "spa", Format: "srt"},
	}
	for index, artifact := range artifacts {
		if got := resolvedSubtitleStagedPath("/staging/Movie", artifact, used); got != want[index] {
			t.Fatalf("path[%d]=%q want=%q", index, got, want[index])
		}
	}
}

func TestSubtitleArtifactJSONPreservesResolvedExtractionState(t *testing.T) {
	values := []SubtitleArtifact{
		{StreamIndex: 4, SourceCodec: "ass", Format: "ass", Mode: "original", Language: "spa", Role: SubtitleRoleFull, RoleProvenance: "metadata", RoleConfidence: "high", OCRLanguage: "jpn", EffectiveOCRLanguage: "jpn_vert", StagedPath: "/staging/Movie.spa.ass", Status: "ready", SizeBytes: 20},
		{StreamIndex: 5, SourceCodec: "subrip", Format: "srt", Language: "eng", Forced: true, SourceForced: true, Role: SubtitleRoleSDH, StagedPath: "/staging/Movie.eng.forced.sdh.srt", Status: "ready", SizeBytes: 30},
	}
	decoded := subtitleArtifactsFromJSON(subtitleArtifactsJSON(values))
	if len(decoded) != 2 || decoded[0].Status != "ready" || decoded[0].Mode != "original" || decoded[0].Role != SubtitleRoleFull || decoded[0].RoleProvenance != "metadata" || decoded[0].OCRLanguage != "jpn" || decoded[0].EffectiveOCRLanguage != "jpn_vert" || !decoded[1].Forced || !decoded[1].SourceForced || decoded[1].Role != SubtitleRoleSDH || decoded[1].StagedPath != values[1].StagedPath {
		t.Fatalf("multiple sidecar artifacts did not round trip: %#v", decoded)
	}
	if got := subtitleArtifactsFromJSON(subtitleArtifactsJSON(nil)); len(got) != 0 {
		t.Fatalf("zero sidecar artifact result=%#v", got)
	}
}

func TestTypedSidecarArtifactJSONKeepsSubtitleAndFontDecodersIsolated(t *testing.T) {
	subtitle := SubtitleArtifact{ArtifactID: "subtitle:4:original:ass", Type: "subtitle_sidecar", StreamIndex: 4, Format: "ass", Status: "planned"}
	font := FontAttachmentArtifact{ArtifactID: "font-attachment:7", Type: "font_attachment", StreamIndex: 7, AttachmentOrdinal: 1, FontFormat: "TTF", SafeFilename: "Font.ttf", Status: "planned"}
	values := sidecarArtifactsJSON([]SubtitleArtifact{subtitle}, []FontAttachmentArtifact{font})
	if subtitles := subtitleArtifactsFromJSON(values); len(subtitles) != 1 || subtitles[0].StreamIndex != 4 {
		t.Fatalf("subtitle decoder=%#v", subtitles)
	}
	if fonts := fontAttachmentArtifactsFromJSON(values); len(fonts) != 1 || fonts[0].StreamIndex != 7 || fonts[0].AttachmentOrdinal != 1 {
		t.Fatalf("font decoder=%#v", fonts)
	}
	legacy := models.JSONList{map[string]interface{}{"streamIndex": 5, "format": "srt", "status": "ready"}}
	if subtitles := subtitleArtifactsFromJSON(legacy); len(subtitles) != 1 || subtitles[0].Type != "subtitle_sidecar" {
		t.Fatalf("legacy subtitle decoder=%#v", subtitles)
	}
}

func TestPlannedFontArtifactsUseFrozenTrackPlan(t *testing.T) {
	plan := ResolvedTrackPlan{FontAttachmentExportPolicy: FontAttachmentExportAll, FontAttachments: []ResolvedFontAttachment{
		{ArtifactID: "font-attachment:7", StreamIndex: 7, AttachmentOrdinal: 1, FontFormat: "TTF", SafeFilename: "Font.ttf"},
	}}
	planMap, err := resolvedTrackPlanMap(plan)
	if err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: "/raw/Movie.mkv", TrackProfileSnapshot: models.JSONMap{resolvedTrackPlanSnapshotKey: planMap}}
	artifacts := plannedFontAttachmentArtifacts(job)
	if len(artifacts) != 1 || artifacts[0].ArtifactID != "font-attachment:7" || artifacts[0].AttachmentOrdinal != 1 || artifacts[0].RelativePath != filepath.Join("Movie.fonts", "Font.ttf") {
		t.Fatalf("planned fonts=%#v", artifacts)
	}
	plan.FontAttachments = nil
	if len(artifacts) != 1 {
		t.Fatal("planned artifacts changed after mutable profile/plan edit")
	}
}

func TestFontAttachmentExtractionUsesFrozenAttachmentOrdinal(t *testing.T) {
	first := strings.Join(fontAttachmentExtractionArgs("/raw/Movie.mkv", 1, "/stage/Movie.fonts/Font.ttf"), " ")
	second := strings.Join(fontAttachmentExtractionArgs("/raw/Movie.mkv", 2, "/stage/Movie.fonts/Other.otf"), " ")
	if !strings.Contains(first, "-dump_attachment:t:1 /stage/Movie.fonts/Font.ttf") || strings.Contains(first, "t:7") {
		t.Fatalf("first selector=%s", first)
	}
	if !strings.Contains(second, "-dump_attachment:t:2 /stage/Movie.fonts/Other.otf") || strings.Contains(second, "t:9") {
		t.Fatalf("second selector=%s", second)
	}
}

func TestResolvedSubtitleExecutionRoutesTextBitmapAndOriginalSeparately(t *testing.T) {
	tests := []struct{ codec, mode, want string }{
		{codec: "ass", mode: "converted", want: "text_ffmpeg"},
		{codec: "hdmv_pgs_subtitle", mode: "converted", want: "bitmap_ocr"},
		{codec: "dvd_subtitle", mode: "converted", want: "bitmap_ocr"},
		{codec: "hdmv_pgs_subtitle", mode: "original", want: "original"},
		{codec: "unknown", mode: "converted", want: "unsupported"},
	}
	for _, test := range tests {
		if got := resolvedSubtitleExecutionKind(test.codec, test.mode); got != test.want {
			t.Fatalf("codec=%s mode=%s got=%s want=%s", test.codec, test.mode, got, test.want)
		}
	}
}

func TestConvertedSubtitleFormatSupportIsCanonicalAndFinite(t *testing.T) {
	for _, format := range []string{"srt", "ass", " SRT ", "ASS"} {
		if !convertedSubtitleFormatSupported(format) {
			t.Fatalf("supported converted subtitle format %q was rejected", format)
		}
	}
	for _, format := range []string{"", "ssa", "vtt", "sup"} {
		if convertedSubtitleFormatSupported(format) {
			t.Fatalf("unsupported converted subtitle format %q was accepted", format)
		}
	}
	if got := seconvFormat("ass"); got != "assa" {
		t.Fatalf("ASS OCR format selector=%q", got)
	}
	if got := seconvFormat("srt"); got != "subrip" {
		t.Fatalf("SRT OCR format selector=%q", got)
	}
}

func TestPublishFontAttachmentArtifactsCreatesOwnedDirectoryAndPreservesPreexistingDirectory(t *testing.T) {
	for _, preexisting := range []bool{false, true} {
		t.Run(map[bool]string{false: "created directory", true: "preexisting directory"}[preexisting], func(t *testing.T) {
			temp := t.TempDir()
			staged := filepath.Join(temp, "stage", "Font.ttf")
			if err := os.MkdirAll(filepath.Dir(staged), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(staged, []byte("font-data"), 0o644); err != nil {
				t.Fatal(err)
			}
			destinationMedia := filepath.Join(temp, "library", "Movie.mkv")
			destinationDir := filepath.Join(temp, "library", "Movie.fonts")
			if preexisting {
				if err := os.MkdirAll(destinationDir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			job := models.QueueJob{SubtitleArtifacts: fontAttachmentArtifactsJSON([]FontAttachmentArtifact{{
				ArtifactID: "font-attachment:7", Type: "font_attachment", StreamIndex: 7, AttachmentOrdinal: 0,
				FontFormat: "TTF", SafeFilename: "Font.ttf", StagedPath: staged, Status: "ready", SizeBytes: 9,
			}})}
			published, err := publishFontAttachmentArtifacts(&job, destinationMedia, false, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(destinationDir, "Font.ttf")); err != nil {
				t.Fatal(err)
			}
			if err := rollbackPublishedPaths(published); err != nil {
				t.Fatal(err)
			}
			_, err = os.Stat(destinationDir)
			if preexisting && err != nil {
				t.Fatalf("preexisting directory was removed: %v", err)
			}
			if !preexisting && !os.IsNotExist(err) {
				t.Fatalf("job-created empty directory survived rollback: %v", err)
			}
		})
	}
}

func TestPublishFontAttachmentArtifactsRejectsCollision(t *testing.T) {
	temp := t.TempDir()
	staged := filepath.Join(temp, "stage", "Font.ttf")
	destinationDir := filepath.Join(temp, "library", "Movie.fonts")
	if err := os.MkdirAll(filepath.Dir(staged), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destinationDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(destinationDir, "Font.ttf")
	if err := os.WriteFile(destination, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{SubtitleArtifacts: fontAttachmentArtifactsJSON([]FontAttachmentArtifact{{ArtifactID: "font-attachment:7", Type: "font_attachment", StreamIndex: 7, FontFormat: "TTF", SafeFilename: "Font.ttf", StagedPath: staged, Status: "ready"}})}
	if _, err := publishFontAttachmentArtifacts(&job, filepath.Join(temp, "library", "Movie.mkv"), false, nil); err == nil {
		t.Fatal("existing unrelated font was overwritten")
	}
	if content, _ := os.ReadFile(destination); string(content) != "existing" {
		t.Fatalf("existing font changed: %q", content)
	}
}

func TestOriginalASSAndSSASidecarsPreserveRepresentation(t *testing.T) {
	for _, test := range []struct{ codec, format, muxer string }{
		{"ass", "ass", "ass"}, {"ssa", "ssa", "ass"},
	} {
		format, muxer, ok := originalSubtitleExtractionFormat(test.codec)
		if !ok || format != test.format || muxer != test.muxer {
			t.Fatalf("codec=%s format=%s muxer=%s ok=%t", test.codec, format, muxer, ok)
		}
		args := originalSubtitleExtractionArgs(MediaJobPlan{InputPath: "/raw/Movie.mkv"}, 4, muxer, "/stage/Movie.es."+format)
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "-c:s copy") || strings.Contains(joined, " srt ") {
			t.Fatalf("ASS/SSA extraction was transcoded: %v", args)
		}
	}
}

func TestGenerateResolvedSubtitleArtifactsReportsUnsupportedCodec(t *testing.T) {
	plan := MediaJobPlan{
		OutputPath:     "/staging/Movie.mkv",
		Streams:        MediaStreamInventory{Subtitle: []MediaStream{{Index: 2, Codec: "dvd_subtitle", Language: "spa"}}},
		ResolvedTracks: &ResolvedTrackPlan{SidecarOutputs: []ResolvedTrackSidecar{{StreamIndex: 2, Codec: "dvd_subtitle", Language: "spa"}}},
	}
	artifacts, err := generateResolvedSubtitleArtifacts(context.Background(), plan, nil)
	if err == nil || len(artifacts) != 1 || artifacts[0].Status != "unsupported" || !strings.Contains(artifacts[0].Error, "stream 2") {
		t.Fatalf("unsupported extraction was not explicit: artifacts=%#v err=%v", artifacts, err)
	}
}

func TestPlannedSubtitleArtifactsFreezeStableDistinctIdentities(t *testing.T) {
	plan := ResolvedTrackPlan{SidecarOutputs: []ResolvedTrackSidecar{
		{StreamIndex: 4, Codec: "ass", Language: "spa", Format: "ass", Mode: "original"},
		{StreamIndex: 4, Codec: "ass", Language: "spa", Format: "srt", Mode: "converted"},
		{StreamIndex: 7, Codec: "ass", Language: "spa", Format: "ass", Mode: "original"},
		{StreamIndex: 7, Codec: "ass", Language: "spa", Format: "srt", Mode: "converted"},
	}}
	planMap, err := resolvedTrackPlanMap(plan)
	if err != nil {
		t.Fatal(err)
	}
	artifacts := plannedSubtitleArtifacts(models.QueueJob{MediaPath: "/raw/Movie.mkv", TrackProfileSnapshot: models.JSONMap{resolvedTrackPlanSnapshotKey: planMap}})
	identities := map[string]struct{}{}
	for _, artifact := range artifacts {
		identities[artifact.ArtifactID] = struct{}{}
		if artifact.Status != "planned" || artifact.DisplayName == "" {
			t.Fatalf("invalid planned artifact: %#v", artifact)
		}
	}
	if len(artifacts) != 4 || len(identities) != 4 {
		t.Fatalf("planned artifacts are not distinct: %#v", artifacts)
	}

	plan.SidecarOutputs = nil
	if len(artifacts) != 4 {
		t.Fatal("queued artifact snapshot changed after the source plan was edited")
	}
}

func TestMergeSubtitleArtifactProgressPreservesOtherPlannedArtifacts(t *testing.T) {
	current := []SubtitleArtifact{
		{ArtifactID: "subtitle:4:original:ass", StreamIndex: 4, Mode: "original", Format: "ass", Status: "planned"},
		{ArtifactID: "subtitle:4:converted:srt", StreamIndex: 4, Mode: "converted", Format: "srt", Status: "planned"},
	}
	current = mergeSubtitleArtifactProgress(current, SubtitleArtifact{ArtifactID: current[0].ArtifactID, StreamIndex: 4, Mode: "original", Format: "ass", Status: "generating"})
	if current[0].Status != "generating" || current[1].Status != "planned" {
		t.Fatalf("generating transition corrupted the frozen list: %#v", current)
	}
	current = mergeSubtitleArtifactProgress(current, SubtitleArtifact{ArtifactID: current[0].ArtifactID, StreamIndex: 4, Mode: "original", Format: "ass", Status: "failed", Error: "command failed"})
	if current[0].Status != "failed" || current[1].Status != "planned" {
		t.Fatalf("failure transition corrupted the frozen list: %#v", current)
	}
}

func TestGenerateResolvedSubtitleArtifactsReportsTruthfulProgress(t *testing.T) {
	temp := t.TempDir()
	ffmpeg := filepath.Join(temp, "ffmpeg")
	if err := os.WriteFile(ffmpeg, []byte("#!/bin/sh\nfor argument do output=\"$argument\"; done\nprintf '%s\\n' '[Events]' 'Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,Hola' > \"$output\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", temp+string(os.PathListSeparator)+os.Getenv("PATH"))
	statuses := []string{}
	plan := MediaJobPlan{
		InputPath: "/raw/Movie.mkv", OutputPath: filepath.Join(temp, "Movie.mkv"),
		Streams:        MediaStreamInventory{Subtitle: []MediaStream{{Index: 4, Codec: "ass", Language: "spa"}}},
		ResolvedTracks: &ResolvedTrackPlan{SidecarOutputs: []ResolvedTrackSidecar{{StreamIndex: 4, Codec: "ass", Language: "spa", Format: "ass", Mode: "original"}}},
	}
	artifacts, err := generateSubtitleArtifactsWithProgress(context.Background(), plan, func(artifact SubtitleArtifact) {
		statuses = append(statuses, artifact.Status)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(statuses, []string{"generating", "ready"}) || len(artifacts) != 1 || artifacts[0].Status != "ready" {
		t.Fatalf("unexpected artifact progress: statuses=%#v artifacts=%#v", statuses, artifacts)
	}
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

func TestPublishSubtitleArtifactsUsesForcedSuffix(t *testing.T) {
	temp := t.TempDir()
	staged := filepath.Join(temp, "staged.srt")
	if err := os.WriteFile(staged, []byte("1\n00:00:00,000 --> 00:00:01,000\nForced\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{SubtitleArtifacts: subtitleArtifactsJSON([]SubtitleArtifact{{
		StreamIndex: 3, SourceCodec: "subrip", Format: "srt", Language: "eng", Forced: true,
		StagedPath: staged, SizeBytes: 45, Status: "ready",
	}})}
	destinationMedia := filepath.Join(temp, "library", "Movie.mkv")
	if _, err := publishSubtitleArtifacts(&job, destinationMedia, false, nil); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(temp, "library", "Movie.eng.forced.srt")
	if artifacts := subtitleArtifactsFromJSON(job.SubtitleArtifacts); len(artifacts) != 1 || artifacts[0].PublishedPath != want {
		t.Fatalf("forced artifact destination=%#v want=%s", artifacts, want)
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

func TestMergeFontAttachmentArtifactProgressPreservesFrozenFields(t *testing.T) {
	current := []FontAttachmentArtifact{{
		ArtifactID:        "font-attachment:7",
		Type:              "font_attachment",
		StreamIndex:       7,
		AttachmentOrdinal: 1,
		SourceCodec:       "ttf",
		OriginalName:      "Original.ttf",
		MIMEType:          "font/ttf",
		FontFormat:        "TTF",
		SafeFilename:      "Original.ttf",
		RelativePath:      filepath.Join("Movie.fonts", "Original.ttf"),
		DisplayName:       "Original font",
		Status:            "planned",
	}}

	update := FontAttachmentArtifact{
		ArtifactID: "font-attachment:7",

		// Intentionally different planning fields.
		StreamIndex:       99,
		AttachmentOrdinal: 9,
		SourceCodec:       "otf",
		OriginalName:      "Wrong.otf",
		MIMEType:          "font/otf",
		FontFormat:        "OTF",
		SafeFilename:      "Wrong.otf",
		RelativePath:      filepath.Join("Wrong.fonts", "Wrong.otf"),
		DisplayName:       "Wrong font",

		// Runtime fields that should be updated.
		StagedPath:    "/tmp/Original.ttf",
		PublishedPath: "/media/Movies/Movie.fonts/Original.ttf",
		SizeBytes:     12345,
		Status:        "ready",
		Error:         "",
	}

	got := mergeFontAttachmentArtifactProgress(current, update)

	if len(got) != 1 {
		t.Fatalf("expected one artifact, got %d", len(got))
	}

	artifact := got[0]

	if artifact.ArtifactID != "font-attachment:7" {
		t.Fatalf("ArtifactID changed: %q", artifact.ArtifactID)
	}
	if artifact.StreamIndex != 7 {
		t.Fatalf("StreamIndex changed: %d", artifact.StreamIndex)
	}
	if artifact.AttachmentOrdinal != 1 {
		t.Fatalf("AttachmentOrdinal changed: %d", artifact.AttachmentOrdinal)
	}
	if artifact.SourceCodec != "ttf" {
		t.Fatalf("SourceCodec changed: %q", artifact.SourceCodec)
	}
	if artifact.OriginalName != "Original.ttf" {
		t.Fatalf("OriginalName changed: %q", artifact.OriginalName)
	}
	if artifact.MIMEType != "font/ttf" {
		t.Fatalf("MIMEType changed: %q", artifact.MIMEType)
	}
	if artifact.FontFormat != "TTF" {
		t.Fatalf("FontFormat changed: %q", artifact.FontFormat)
	}
	if artifact.SafeFilename != "Original.ttf" {
		t.Fatalf("SafeFilename changed: %q", artifact.SafeFilename)
	}
	if artifact.RelativePath != filepath.Join("Movie.fonts", "Original.ttf") {
		t.Fatalf("RelativePath changed: %q", artifact.RelativePath)
	}
	if artifact.DisplayName != "Original font" {
		t.Fatalf("DisplayName changed: %q", artifact.DisplayName)
	}

	if artifact.StagedPath != "/tmp/Original.ttf" {
		t.Fatalf("StagedPath not updated: %q", artifact.StagedPath)
	}
	if artifact.PublishedPath != "/media/Movies/Movie.fonts/Original.ttf" {
		t.Fatalf("PublishedPath not updated: %q", artifact.PublishedPath)
	}
	if artifact.SizeBytes != 12345 {
		t.Fatalf("SizeBytes not updated: %d", artifact.SizeBytes)
	}
	if artifact.Status != "ready" {
		t.Fatalf("Status not updated: %q", artifact.Status)
	}
}

func TestResolvedTrackPlanTakesPrecedenceOverLegacySubtitleTransforms(t *testing.T) {
	temp := t.TempDir()
	ffmpeg := filepath.Join(temp, "ffmpeg")
	if err := os.WriteFile(
		ffmpeg,
		[]byte("#!/bin/sh\nfor argument do output=\"$argument\"; done\nprintf '%s\\n' '[Events]' 'Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,Hola' > \"$output\"\n"),
		0o700,
	); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", temp+string(os.PathListSeparator)+os.Getenv("PATH"))

	override := AssetConversionOverrideState{
		SubtitleTransforms: []SubtitleTransform{{
			StreamIndex:    2,
			Format:         "ass",
			Language:       "eng",
			RemoveEmbedded: true,
		}},
	}

	plan := MediaJobPlan{
		InputPath:  "/raw/Movie.mkv",
		OutputPath: filepath.Join(temp, "Movie.mkv"),
		Streams: MediaStreamInventory{
			Subtitle: []MediaStream{
				{Index: 2, Codec: "ass", Language: "eng"},
				{Index: 4, Codec: "ass", Language: "spa"},
			},
		},
		Override: override,
		ResolvedTracks: &ResolvedTrackPlan{
			SidecarOutputs: []ResolvedTrackSidecar{{
				StreamIndex: 4,
				Codec:       "ass",
				Language:    "spa",
				Format:      "ass",
				Mode:        "original",
			}},
		},
	}

	artifacts, err := generateSubtitleArtifactsWithProgress(
		context.Background(),
		plan,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 ||
		artifacts[0].StreamIndex != 4 ||
		artifacts[0].Language != "spa" {
		t.Fatalf("canonical plan did not win over legacy transforms: %#v", artifacts)
	}

	legacyPlan := plan
	legacyPlan.OutputPath = filepath.Join(temp, "Legacy.mkv")
	legacyPlan.ResolvedTracks = nil

	legacyArtifacts, err := generateSubtitleArtifactsWithProgress(
		context.Background(),
		legacyPlan,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(legacyArtifacts) != 1 ||
		legacyArtifacts[0].StreamIndex != 2 ||
		legacyArtifacts[0].Language != "eng" {
		t.Fatalf("legacy-only transform stopped working: %#v", legacyArtifacts)
	}
}

func TestExcludedLegacySubtitleStreamsNeverInvokeExtractionOCROrConversion(t *testing.T) {
	for _, codec := range []string{"ass", "subrip", "hdmv_pgs_subtitle"} {
		t.Run(codec, func(t *testing.T) {
			bin := t.TempDir()
			for _, name := range []string{"ffmpeg", "ffprobe", "seconv", "tesseract"} {
				path := filepath.Join(bin, name)
				script := "#!/bin/sh\nprintf '%s' called > \"$0.called\"\nexit 1\n"
				if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", bin)
			plan := MediaJobPlan{
				InputPath: "/raw/Episode.mkv", OutputPath: filepath.Join(t.TempDir(), "Episode.mkv"),
				Streams: MediaStreamInventory{Subtitle: []MediaStream{
					{Index: 2, Codec: "subrip", Language: "eng"},
					{Index: 3, Codec: codec, Language: "eng"},
					{Index: 4, Codec: "subrip", Language: "eng"},
				}},
				Override: AssetConversionOverrideState{
					KeepSubtitleStreams: []int{2, 4},
					SubtitleTransforms:  []SubtitleTransform{{StreamIndex: 3, Format: "srt", RemoveEmbedded: true, Language: "eng"}},
				},
			}
			artifacts, err := generateSubtitleArtifactsWithProgress(context.Background(), plan, nil)
			if err != nil || len(artifacts) != 0 {
				t.Fatalf("excluded %s transform returned artifacts=%#v err=%v", codec, artifacts, err)
			}
			for _, name := range []string{"ffmpeg", "ffprobe", "seconv", "tesseract"} {
				if _, err := os.Stat(filepath.Join(bin, name+".called")); !os.IsNotExist(err) {
					t.Fatalf("excluded %s subtitle reached %s: stat err=%v", codec, name, err)
				}
			}
		})
	}
}

func TestEmptyAssetSubtitleSelectionNeverInvokesSubtitleOperations(t *testing.T) {
	bin := t.TempDir()
	for _, name := range []string{"ffmpeg", "ffprobe", "seconv", "tesseract"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nprintf called > \"$0.called\"\nexit 1\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	plan := MediaJobPlan{
		InputPath: "/raw/Episode.mkv", OutputPath: filepath.Join(t.TempDir(), "Episode.mkv"),
		Streams: MediaStreamInventory{Subtitle: []MediaStream{
			{Index: 2, Codec: "subrip", Language: "eng"},
			{Index: 3, Codec: "hdmv_pgs_subtitle", Language: "spa"},
			{Index: 4, Codec: "ass", Language: "jpn"},
		}},
		Override: AssetConversionOverrideState{
			KeepSubtitleStreams: []int{},
			SubtitleTransforms: []SubtitleTransform{
				{StreamIndex: 2, Format: "ass", RemoveEmbedded: true, Language: "eng"},
				{StreamIndex: 3, Format: "srt", RemoveEmbedded: true, Language: "spa"},
				{StreamIndex: 4, Format: "srt", RemoveEmbedded: true, Language: "jpn"},
			},
		},
	}
	artifacts, err := generateSubtitleArtifactsWithProgress(context.Background(), plan, nil)
	if err != nil || len(artifacts) != 0 {
		t.Fatalf("empty asset selection returned sidecars=%#v err=%v", artifacts, err)
	}
	for _, name := range []string{"ffmpeg", "ffprobe", "seconv", "tesseract"} {
		if _, err := os.Stat(filepath.Join(bin, name+".called")); !os.IsNotExist(err) {
			t.Fatalf("empty asset selection reached %s: stat err=%v", name, err)
		}
	}
}

func TestExcludedCanonicalSubtitleSidecarNeverInvokesSubtitleTools(t *testing.T) {
	bin := t.TempDir()
	for _, name := range []string{"ffmpeg", "ffprobe", "seconv", "tesseract"} {
		path := filepath.Join(bin, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nprintf called > \"$0.called\"\nexit 1\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	plan := MediaJobPlan{
		InputPath: "/raw/Episode.mkv", OutputPath: filepath.Join(t.TempDir(), "Episode.mkv"),
		Streams: MediaStreamInventory{Subtitle: []MediaStream{{Index: 3, Codec: "subrip", Language: "eng"}}},
		ResolvedTracks: &ResolvedTrackPlan{
			SubtitleStreams: []ResolvedSubtitleTrack{{StreamIndex: 3, Action: SubtitleDispositionRemove}},
			SidecarOutputs:  []ResolvedTrackSidecar{{StreamIndex: 3, Codec: "subrip", SourceType: "text", Format: "srt", RequestedFormat: "srt", Mode: "converted"}},
		},
		Override: AssetConversionOverrideState{SubtitleTransforms: []SubtitleTransform{{StreamIndex: 3, Format: "ass", RemoveEmbedded: true}}},
	}
	artifacts, err := generateSubtitleArtifactsWithProgress(context.Background(), plan, nil)
	if err != nil || len(artifacts) != 0 {
		t.Fatalf("excluded canonical sidecar returned artifacts=%#v err=%v", artifacts, err)
	}
	for _, name := range []string{"ffmpeg", "ffprobe", "seconv", "tesseract"} {
		if _, err := os.Stat(filepath.Join(bin, name+".called")); !os.IsNotExist(err) {
			t.Fatalf("excluded canonical sidecar reached %s: stat err=%v", name, err)
		}
	}
}

func TestExcludedSameLanguageSubtitleNeverReachesFFmpeg(t *testing.T) {
	bin := t.TempDir()
	ffmpeg := filepath.Join(bin, "ffmpeg")
	script := "#!/bin/sh\noutput=\"\"\nfor argument do output=\"$argument\"; done\nprintf '%s\\n' \"$*\" >> \"$0.log\"\nprintf '%s\\n' '1' '00:00:01,000 --> 00:00:02,000' 'Hola' > \"$output\"\n"
	if err := os.WriteFile(ffmpeg, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	plan := MediaJobPlan{
		InputPath: "/raw/Episode.mkv", OutputPath: filepath.Join(t.TempDir(), "Episode.mkv"),
		Streams: MediaStreamInventory{Subtitle: []MediaStream{
			{Index: 2, Codec: "subrip", Language: "eng"},
			{Index: 3, Codec: "subrip", Language: "eng"},
			{Index: 4, Codec: "subrip", Language: "eng"},
		}},
		Override: AssetConversionOverrideState{
			KeepSubtitleStreams: []int{2, 4},
			SubtitleTransforms: []SubtitleTransform{
				{StreamIndex: 2, Format: "srt", RemoveEmbedded: true, Language: "eng"},
				{StreamIndex: 3, Format: "srt", RemoveEmbedded: true, Language: "eng"},
				{StreamIndex: 4, Format: "srt", RemoveEmbedded: true, Language: "eng"},
			},
		},
	}
	var progress []int
	artifacts, err := generateSubtitleArtifactsWithProgress(context.Background(), plan, func(artifact SubtitleArtifact) {
		progress = append(progress, artifact.StreamIndex)
	})
	if err != nil || len(artifacts) != 2 || artifacts[0].StreamIndex != 2 || artifacts[1].StreamIndex != 4 {
		t.Fatalf("selected duplicate-language artifact=%#v err=%v", artifacts, err)
	}
	for _, index := range progress {
		if index == 3 {
			t.Fatalf("excluded stream contributed artifact progress: %#v", progress)
		}
	}
	log, err := os.ReadFile(ffmpeg + ".log")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), "0:2") || !strings.Contains(string(log), "0:4") || strings.Contains(string(log), "0:3") {
		t.Fatalf("FFmpeg operations did not use only the selected stream index: %s", log)
	}
}

func TestRemovedResolvedStreamHasNoPlannedSubtitleArtifactOrStageWork(t *testing.T) {
	plan := ResolvedTrackPlan{
		SubtitleStreams: []ResolvedSubtitleTrack{
			{StreamIndex: 3, Action: SubtitleDispositionRemove},
			{StreamIndex: 4, Action: SubtitleDispositionExtract},
		},
		SidecarOutputs: []ResolvedTrackSidecar{
			{StreamIndex: 3, Codec: "subrip", Format: "srt", Mode: "converted"},
			{StreamIndex: 4, Codec: "ass", Format: "ass", Mode: "original"},
		},
	}
	snapshot, err := resolvedTrackPlanMap(plan)
	if err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: "/raw/Episode.mkv", TrackProfileSnapshot: models.JSONMap{resolvedTrackPlanSnapshotKey: snapshot}}
	artifacts := plannedSubtitleArtifacts(job)
	if len(artifacts) != 1 || artifacts[0].StreamIndex != 4 {
		t.Fatalf("removed stream remained in planned SubtitleArtifacts: %#v", artifacts)
	}
	stagePlan := ResolvedTrackPlan{
		SubtitleStreams: []ResolvedSubtitleTrack{{StreamIndex: 3, Action: SubtitleDispositionRemove}},
		SidecarOutputs:  []ResolvedTrackSidecar{{StreamIndex: 3, Codec: "subrip", Format: "srt", Mode: "converted"}},
	}
	workPlan := MediaJobPlan{ResolvedTracks: &stagePlan}
	if hasSubtitleArtifactWork(workPlan) {
		t.Fatalf("removed-only sidecar scheduled subtitle stage work: %#v", stagePlan.SidecarOutputs)
	}
	stagePlan.FontAttachments = []ResolvedFontAttachment{{StreamIndex: 7}}
	workPlan.ResolvedTracks = &stagePlan
	if !hasSubtitleArtifactWork(workPlan) {
		t.Fatal("independent font attachment work did not retain subtitle preparation stage")
	}
}

func TestValidationAndPublisherIgnoreArtifactsOutsideTheFrozenTrackPlan(t *testing.T) {
	staged := filepath.Join(t.TempDir(), "excluded.srt")
	if err := os.WriteFile(staged, []byte("1\n00:00:01,000 --> 00:00:02,000\nexcluded\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolvedTrackPlanMap(ResolvedTrackPlan{
		SubtitleStreams: []ResolvedSubtitleTrack{{StreamIndex: 3, Codec: "subrip", Action: SubtitleDispositionRemove}},
		SidecarOutputs:  []ResolvedTrackSidecar{{StreamIndex: 3, Codec: "subrip", Format: "srt", Mode: "converted"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{
		MediaPath:            "/raw/Episode.mkv",
		TrackProfileSnapshot: models.JSONMap{resolvedTrackPlanSnapshotKey: resolved},
		SubtitleArtifacts: subtitleArtifactsJSON([]SubtitleArtifact{{
			Type: "subtitle_sidecar", StreamIndex: 3, Format: "srt", Mode: "converted", StagedPath: staged, Status: "ready",
		}}),
	}
	checks, _, report := validateRequiredSubtitleArtifacts(job)
	if len(checks) != 0 || intValueSetting(report["required"], -1) != 0 {
		t.Fatalf("Validation treated the removed stream as required: checks=%#v report=%#v", checks, report)
	}
	if artifacts, ok := report["artifacts"].([]SubtitleArtifact); !ok || len(artifacts) != 0 {
		t.Fatalf("Validation retained an artifact for the removed stream: %#v", report["artifacts"])
	}

	destination := filepath.Join(t.TempDir(), "Episode.mkv")
	var backups []publishBackup
	published, err := publishSubtitleArtifacts(&job, destination, false, &backups)
	if err != nil || len(published) != 0 {
		t.Fatalf("Publisher used an artifact outside the frozen track plan: published=%#v err=%v", published, err)
	}
	if len(subtitleArtifactsFromJSON(job.SubtitleArtifacts)) != 0 {
		t.Fatalf("Publisher retained the excluded artifact in the job: %#v", job.SubtitleArtifacts)
	}
	if _, err := os.Stat(staged); err != nil {
		t.Fatalf("filtering an excluded artifact unexpectedly removed its staged file: %v", err)
	}
}

func TestSubtitleArtifactSelectionPreservesNilEmptyAndLegacySnapshotSemantics(t *testing.T) {
	artifact := SubtitleArtifact{Type: "subtitle_sidecar", StreamIndex: 3, Format: "srt", Mode: "converted"}
	for _, test := range []struct {
		name     string
		snapshot models.JSONMap
		allowed  bool
	}{
		{name: "nil inherits", snapshot: models.JSONMap{assetSubtitleSelectionExplicitSnapshotKey: true}, allowed: true},
		{name: "empty excludes all", snapshot: models.JSONMap{
			assetSubtitleSelectionExplicitSnapshotKey: true,
			assetConversionOverrideSnapshotKey:        models.JSONMap{"keepSubtitleStreams": models.JSONList{}},
		}, allowed: false},
		{name: "legacy snapshot remains immutable", snapshot: models.JSONMap{
			assetConversionOverrideSnapshotKey: models.JSONMap{"keepSubtitleStreams": models.JSONList{2, 4}},
		}, allowed: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			job := models.QueueJob{ProfileSnapshot: test.snapshot}
			if got := subtitleArtifactAllowedForQueueJob(job, artifact); got != test.allowed {
				t.Fatalf("artifact allowed=%v want %v for snapshot %#v", got, test.allowed, test.snapshot)
			}
		})
	}
}
