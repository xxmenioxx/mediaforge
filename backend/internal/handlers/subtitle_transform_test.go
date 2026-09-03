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
		{StreamIndex: 4, SourceCodec: "ass", Format: "ass", Mode: "original", Language: "spa", StagedPath: "/staging/Movie.spa.ass", Status: "ready", SizeBytes: 20},
		{StreamIndex: 5, SourceCodec: "subrip", Format: "srt", Language: "eng", Forced: true, StagedPath: "/staging/Movie.eng.forced.srt", Status: "ready", SizeBytes: 30},
	}
	decoded := subtitleArtifactsFromJSON(subtitleArtifactsJSON(values))
	if len(decoded) != 2 || decoded[0].Status != "ready" || decoded[0].Mode != "original" || !decoded[1].Forced || decoded[1].StagedPath != values[1].StagedPath {
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
