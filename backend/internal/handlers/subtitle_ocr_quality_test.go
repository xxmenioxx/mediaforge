package handlers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testPublishedOCREvidence(path string, text string) *canonicalOCREvidence {
	return &canonicalOCREvidence{
		Identity:          canonicalOCRIdentity{SourcePath: path},
		RequestedLanguage: "eng",
		EffectiveLanguage: "eng",
		OCRMode:           "accurate",
		Complete:          true,
		Integrity:         "canonical-ocr-evidence-v1",
		Cues: []canonicalBitmapSubtitleCue{{
			Start: 1, End: 2, Text: text, SourceOrdinal: 1,
		}},
	}
}

func writeTestSRT(t *testing.T, path, text string) {
	t.Helper()
	content := "1\n00:00:01,000 --> 00:00:02,000\n" + text + "\n\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSubtitleOCRAccurateScoreUsesDeterministicLanguageEvidence(t *testing.T) {
	primary := subtitleOCRScoreForLanguage("1\n00:00:01,000 --> 00:00:02,000\nThe h0use\n", "eng")
	secondary := subtitleOCRScoreForLanguage("1\n00:00:01,000 --> 00:00:02,000\nThe house\n", "eng")
	if secondary <= primary {
		t.Fatalf("expected dictionary/confusion evidence to prefer the valid candidate: primary=%v secondary=%v", primary, secondary)
	}
	if !strings.Contains(canonicalOCRPipelineProvenance, canonicalOCRScoringVersion) {
		t.Fatalf("scorer provenance version is not in the canonical pipeline provenance: %q", canonicalOCRPipelineProvenance)
	}
}

func TestAnalyzePublishedOCRQualityUsesDurableSnapshotAfterCacheEviction(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", root)
	sidecar := filepath.Join(root, "movie.eng.srt")
	writeTestSRT(t, sidecar, "The h0use")
	evidence := testPublishedOCREvidence(sidecar, "The h0use")
	if err := commitPublishedCanonicalOCRSnapshot(context.Background(), sidecar, "srt", "both-1", evidence, "test", ""); err != nil {
		t.Fatal(err)
	}
	// The execution cache is intentionally unrelated to the durable snapshot.
	if err := os.RemoveAll(filepath.Join(root, "execution-cache")); err != nil {
		t.Fatal(err)
	}
	report, err := analyzePublishedOCRQuality(sidecar, "srt", "both-1")
	if err != nil {
		t.Fatal(err)
	}
	if report.SuspiciousCount != 1 || report.BaselineRevision == "" {
		t.Fatalf("expected durable analysis baseline after cache eviction: %+v", report)
	}
	if content, err := os.ReadFile(sidecar); err != nil || !strings.Contains(string(content), "The h0use") {
		t.Fatalf("analysis changed the published sidecar: %q (%v)", content, err)
	}
}

func TestSubtitleOCRQualityAnalyzerSeparatesCleanSuspiciousAndReview(t *testing.T) {
	snapshot := &publishedCanonicalOCRSnapshot{
		Revision: publishedSubtitleRevision{ArtifactPath: "/tmp/quality.srt"},
		Format:   "srt",
		Evidence: canonicalOCREvidence{
			RequestedLanguage: "eng",
			Cues: []canonicalBitmapSubtitleCue{
				{SourceOrdinal: 1, Text: "The house"},
				{SourceOrdinal: 2, Text: "The h0use"},
				{SourceOrdinal: 3, Text: "The \ufffdouse"},
			},
		},
	}
	report := analyzeCanonicalOCRSnapshot(snapshot)
	if report.CleanCount != 1 || report.SuspiciousCount != 1 || report.ReviewCount != 1 {
		t.Fatalf("unexpected quality buckets: %+v", report)
	}
	if report.Findings[0].Status != subtitleOCRQualityStatusClean || report.Findings[1].Status != subtitleOCRQualityStatusSuspect || report.Findings[2].Status != subtitleOCRQualityStatusReview {
		t.Fatalf("unexpected finding statuses: %+v", report.Findings)
	}
	properNounScore := subtitleOCRScoreForLanguage("The Xylophonia", "eng")
	if properNounScore <= 0 {
		t.Fatalf("proper noun should not be rejected by unsupported dictionary evidence: %v", properNounScore)
	}
}

func TestLegacyOCRRepairPreservesRepresentationBoundary(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", root)
	srt := filepath.Join(root, "legacy.srt")
	ass := filepath.Join(root, "legacy.ass")
	writeTestSRT(t, srt, "The h0use")
	if err := os.WriteFile(ass, []byte("[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,The h0use\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	srtSnapshot, ok := reconstructLegacyOCRSnapshot(srt, "srt", "")
	if !ok || !srtSnapshot.CanCorrectLegacy("srt", 1) {
		t.Fatal("legacy SRT should provide text-only correction")
	}
	assSnapshot, ok := reconstructLegacyOCRSnapshot(ass, "ass", "")
	if !ok || assSnapshot.CanCorrectLegacy("ass", 1) {
		t.Fatal("legacy ASS without canonical geometry must not become a repair source")
	}
	if srtSnapshot.CanCorrectLegacy("both", 2) {
		t.Fatal("legacy SRT must not silently become a both-output source")
	}
}

func TestSubtitleOCRCorrectionAdvancesBaselineAndRejectsConcurrentStaleWriter(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", root)
	sidecar := filepath.Join(root, "movie.eng.srt")
	writeTestSRT(t, sidecar, "The h0use")
	evidence := testPublishedOCREvidence(sidecar, "The h0use")
	if err := commitPublishedCanonicalOCRSnapshot(context.Background(), sidecar, "srt", "one", evidence, "initial", ""); err != nil {
		t.Fatal(err)
	}
	oldRevision, _, err := publishedSubtitleRevisionFor(sidecar, "one")
	if err != nil {
		t.Fatal(err)
	}
	updated := cloneCanonicalOCREvidence(evidence)
	updated.Cues[0].Text = "The house"
	updated.Cues[0].Geometry = &bitmapCueGeometry{CanvasWidth: 1920, CanvasHeight: 1080, BitmapX: 640, BitmapY: 800, BitmapWidth: 640, BitmapHeight: 120}
	snapshot := &publishedCanonicalOCRSnapshot{Evidence: *updated, LogicalTransformKey: "one"}
	workspace := t.TempDir()
	staged, err := writeCorrectedSubtitleFiles(workspace, []string{sidecar}, []string{"srt"}, updated.Cues)
	if err != nil {
		t.Fatal(err)
	}
	if err := publishCorrectedSubtitleFiles(context.Background(), "correction-1", []string{sidecar}, []string{"srt"}, staged, []*publishedCanonicalOCRSnapshot{snapshot}, []publishedSubtitleRevision{oldRevision}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(sidecar)
	if err != nil || !strings.Contains(string(content), "The house") {
		t.Fatalf("corrected sidecar was not published: %q (%v)", content, err)
	}
	current, ok := loadPublishedCanonicalOCRSnapshot(sidecar, "srt", "one")
	if !ok || current.Evidence.Cues[0].Text != "The house" || current.PreviousRevision != oldRevision.key() {
		t.Fatalf("corrected baseline did not advance with lineage: %+v", current)
	}
	secondStage, err := writeCorrectedSubtitleFiles(t.TempDir(), []string{sidecar}, []string{"srt"}, []canonicalBitmapSubtitleCue{{Start: 1, End: 2, Text: "stale", SourceOrdinal: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err := publishCorrectedSubtitleFiles(context.Background(), "correction-2", []string{sidecar}, []string{"srt"}, secondStage, []*publishedCanonicalOCRSnapshot{snapshot}, []publishedSubtitleRevision{oldRevision}); err == nil {
		t.Fatal("expected concurrent correction based on the old revision to fail closed")
	}
	content, err = os.ReadFile(sidecar)
	if err != nil || !strings.Contains(string(content), "The house") || strings.Contains(string(content), "stale") {
		t.Fatalf("stale correction overwrote current sidecar: %q (%v)", content, err)
	}
}

func TestSubtitleOCRBothCorrectionUsesOneCanonicalCueSet(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", root)
	srt := filepath.Join(root, "movie.srt")
	ass := filepath.Join(root, "movie.ass")
	writeTestSRT(t, srt, "The h0use")
	if err := os.WriteFile(ass, renderBitmapASSCues([]canonicalBitmapSubtitleCue{{Start: 1, End: 2, Text: "The h0use", SourceOrdinal: 1}}), 0o644); err != nil {
		t.Fatal(err)
	}
	evidence := testPublishedOCREvidence(srt, "The h0use")
	if err := commitPublishedCanonicalOCRSnapshot(context.Background(), srt, "srt", "both-1", evidence, "initial", ""); err != nil {
		t.Fatal(err)
	}
	if err := commitPublishedCanonicalOCRSnapshot(context.Background(), ass, "ass", "both-1", evidence, "initial", ""); err != nil {
		t.Fatal(err)
	}
	oldSRT, _, _ := publishedSubtitleRevisionFor(srt, "both-1")
	oldASS, _, _ := publishedSubtitleRevisionFor(ass, "both-1")
	updated := cloneCanonicalOCREvidence(evidence)
	updated.Cues[0].Text = "The house"
	updated.Cues[0].Geometry = &bitmapCueGeometry{CanvasWidth: 1920, CanvasHeight: 1080, BitmapX: 640, BitmapY: 800, BitmapWidth: 640, BitmapHeight: 120}
	snapshots := []*publishedCanonicalOCRSnapshot{{Evidence: *cloneCanonicalOCREvidence(updated), LogicalTransformKey: "both-1"}, {Evidence: *cloneCanonicalOCREvidence(updated), LogicalTransformKey: "both-1"}}
	staged, err := writeCorrectedSubtitleFiles(t.TempDir(), []string{srt, ass}, []string{"srt", "ass"}, updated.Cues)
	if err != nil {
		t.Fatal(err)
	}
	if err := publishCorrectedSubtitleFiles(context.Background(), "both-correction", []string{srt, ass}, []string{"srt", "ass"}, staged, snapshots, []publishedSubtitleRevision{oldSRT, oldASS}); err != nil {
		t.Fatal(err)
	}
	srtContent, _ := os.ReadFile(srt)
	assContent, _ := os.ReadFile(ass)
	if !strings.Contains(string(srtContent), "The house") || !strings.Contains(string(assContent), "The house") {
		t.Fatalf("both outputs did not use the same corrected cue text: SRT=%q ASS=%q", srtContent, assContent)
	}
	if !strings.Contains(string(assContent), `\an`) {
		t.Fatalf("corrected ASS lost canonical geometry positioning: %q", assContent)
	}
}

func TestSubtitleOCRCorrectionCancellationBeforeBarrierDoesNotPublish(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", root)
	sidecar := filepath.Join(root, "movie.srt")
	writeTestSRT(t, sidecar, "original")
	oldRevision, _, err := publishedSubtitleRevisionFor(sidecar, "cancel")
	if err != nil {
		t.Fatal(err)
	}
	updated := []canonicalBitmapSubtitleCue{{Start: 1, End: 2, Text: "replacement", SourceOrdinal: 1}}
	staged, err := writeCorrectedSubtitleFiles(t.TempDir(), []string{sidecar}, []string{"srt"}, updated)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	snapshot := &publishedCanonicalOCRSnapshot{Evidence: *testPublishedOCREvidence(sidecar, "replacement"), LogicalTransformKey: "cancel"}
	if err := publishCorrectedSubtitleFiles(ctx, "cancelled", []string{sidecar}, []string{"srt"}, staged, []*publishedCanonicalOCRSnapshot{snapshot}, []publishedSubtitleRevision{oldRevision}); err == nil {
		t.Fatal("expected cancellation before the publication barrier")
	}
	content, err := os.ReadFile(sidecar)
	if err != nil || !strings.Contains(string(content), "original") || strings.Contains(string(content), "replacement") {
		t.Fatalf("cancelled correction changed the sidecar: %q (%v)", content, err)
	}
}

func TestPublishedOCRSnapshotCleanupRemovesOrphansWithoutTouchingSidecars(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", root)
	sidecar := filepath.Join(root, "movie.srt")
	writeTestSRT(t, sidecar, "The house")
	evidence := testPublishedOCREvidence(sidecar, "The house")
	if err := commitPublishedCanonicalOCRSnapshot(context.Background(), sidecar, "srt", "cleanup", evidence, "test", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(sidecar); err != nil {
		t.Fatal(err)
	}
	prunePublishedCanonicalSnapshots(publishedSubtitleSnapshotRoot())
	if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
		t.Fatalf("snapshot cleanup unexpectedly recreated or retained the removed sidecar: %v", err)
	}
	if entries, err := os.ReadDir(publishedSubtitleSnapshotRoot()); err != nil {
		t.Fatal(err)
	} else if len(entries) != 0 {
		t.Fatalf("orphan snapshot was not removed: %v", entries)
	}
}

func TestPublishedOCRSnapshotSourceOwnershipMismatchFailsClosed(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", root)
	sidecar := filepath.Join(root, "movie.srt")
	source := filepath.Join(root, "source.mkv")
	writeTestSRT(t, sidecar, "The house")
	if err := os.WriteFile(source, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	evidence := testPublishedOCREvidence(source, "The house")
	if err := commitPublishedCanonicalOCRSnapshot(context.Background(), sidecar, "srt", "owner", evidence, "test", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if _, err := analyzePublishedOCRQuality(sidecar, "srt", "owner"); err == nil {
		t.Fatal("expected source ownership mismatch to fail closed instead of reconstructing legacy evidence")
	}
}
