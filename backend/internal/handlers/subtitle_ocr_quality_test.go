package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func testPublishedOCREvidence(path string, text string) *canonicalOCREvidence {
	info, _ := os.Stat(path)
	size := int64(-1)
	modified := int64(-1)
	if info != nil {
		size = info.Size()
		modified = info.ModTime().UnixNano()
	}
	return &canonicalOCREvidence{
		Identity:          canonicalOCRIdentity{SourcePath: path, SourceSize: size, SourceModTimeUnixNano: modified, SourceSnapshotVersion: "stat-v1"},
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
	dictionaryRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(dictionaryRoot, "en.dic"), []byte("3\nthe\nhouse\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	previousFolder := ocrDictionaryFolderOverride
	ocrDictionaryFolderOverride = dictionaryRoot
	defer func() { ocrDictionaryFolderOverride = previousFolder }()
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
	executionRoot := t.TempDir()
	persistentRoot := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", executionRoot)
	t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", persistentRoot)
	sidecar := filepath.Join(root, "movie.eng.srt")
	source := filepath.Join(root, "source.mkv")
	writeTestSRT(t, sidecar, "The h0use")
	if err := os.WriteFile(source, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	evidence := testPublishedOCREvidence(sidecar, "The h0use")
	evidence.Identity.SourcePath = source
	info, _ := os.Stat(source)
	evidence.Identity.SourceSize = info.Size()
	evidence.Identity.SourceModTimeUnixNano = info.ModTime().UnixNano()
	if err := commitPublishedCanonicalOCRSnapshot(context.Background(), sidecar, "srt", "both-1", evidence, "test", ""); err != nil {
		t.Fatal(err)
	}
	// The execution cache is intentionally unrelated to the durable snapshot.
	if err := os.RemoveAll(executionRoot); err != nil {
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

func TestExternalSubtitleInventoryUsesIndexedOwnershipBeyondPreviousLimit(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "execution"))
	t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", filepath.Join(root, "persistent"))
	media := filepath.Join(root, "movie.mkv")
	managed := filepath.Join(root, "movie.eng.srt")
	generic := filepath.Join(root, "movie.spa.srt")
	if err := os.WriteFile(media, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTestSRT(t, managed, "The house")
	writeTestSRT(t, generic, "La casa")
	evidence := testPublishedOCREvidence(media, "The house")
	info, _ := os.Stat(media)
	evidence.Identity.SourcePath = media
	evidence.Identity.SourceSize = info.Size()
	evidence.Identity.SourceModTimeUnixNano = info.ModTime().UnixNano()
	if err := commitPublishedCanonicalOCRSnapshot(context.Background(), managed, "srt", "indexed-managed", evidence, "test", ""); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 4100; index++ {
		unrelatedPath := filepath.Join(root, "unrelated-"+strconv.Itoa(index)+".srt")
		owner := publishedSubtitleOwnerIdentity(unrelatedPath, "srt", "unrelated-"+strconv.Itoa(index))
		snapshot := publishedCanonicalOCRSnapshot{
			OwnerIdentity:       owner,
			Revision:            publishedSubtitleRevision{ArtifactPath: unrelatedPath},
			Format:              "srt",
			LogicalTransformKey: "unrelated-" + strconv.Itoa(index),
		}
		encoded, err := json.Marshal(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(publishedSubtitleSnapshotPath(owner), encoded, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	values, err := externalSubtitlesForMedia(media)
	if err != nil {
		t.Fatal(err)
	}
	var managedValue, genericValue ExternalSubtitle
	for _, value := range values {
		switch value.Path {
		case managed:
			managedValue = value
		case generic:
			genericValue = value
		}
	}
	if !managedValue.OCRManaged || !managedValue.OCRAnalysisAvailable || managedValue.LogicalTransformKey != "indexed-managed" {
		t.Fatalf("active ownership was hidden by unrelated snapshot count: %+v", managedValue)
	}
	if genericValue.OCRManaged || genericValue.OCRAnalysisAvailable || genericValue.LogicalTransformKey != "" {
		t.Fatalf("generic sidecar became managed: %+v", genericValue)
	}
}

func TestExternalSubtitleOwnershipAmbiguityFailsClosed(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "execution"))
	t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", filepath.Join(root, "persistent"))
	media := filepath.Join(root, "movie.mkv")
	managed := filepath.Join(root, "movie.eng.srt")
	if err := os.WriteFile(media, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTestSRT(t, managed, "The house")
	evidence := testPublishedOCREvidence(media, "The house")
	info, _ := os.Stat(media)
	evidence.Identity.SourcePath = media
	evidence.Identity.SourceSize = info.Size()
	evidence.Identity.SourceModTimeUnixNano = info.ModTime().UnixNano()
	for _, logicalTransform := range []string{"ambiguous-a", "ambiguous-b"} {
		if err := commitPublishedCanonicalOCRSnapshot(context.Background(), managed, "srt", logicalTransform, evidence, "test", ""); err != nil {
			t.Fatal(err)
		}
	}
	ownershipIndex := loadPublishedOCRExternalOwnershipIndex()
	managedValue, available, logicalTransform := publishedOCRExternalOwnershipFromIndex(ownershipIndex, managed, "srt")
	if !managedValue || available || logicalTransform != "" {
		t.Fatalf("ambiguous OCR ownership was not failed closed: managed=%v available=%v logical=%q", managedValue, available, logicalTransform)
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
	t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", root)
	srt := filepath.Join(root, "legacy.srt")
	ass := filepath.Join(root, "legacy.ass")
	writeTestSRT(t, srt, "The h0use")
	if err := os.WriteFile(ass, []byte("[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,The h0use\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := markMVForgeOwnedLegacyOCRArtifact(srt, "srt", "", srt); err != nil {
		t.Fatal(err)
	}
	if err := markMVForgeOwnedLegacyOCRArtifact(ass, "ass", "", ass); err != nil {
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

func TestLegacyOCRRepairRejectsUnownedAdjacentSubtitle(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", root)
	t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", root)
	sidecar := filepath.Join(root, "external.srt")
	writeTestSRT(t, sidecar, "The h0use")
	if snapshot, ok := reconstructLegacyOCRSnapshot(sidecar, "srt", ""); ok || snapshot != nil {
		t.Fatal("arbitrary adjacent subtitle must not be treated as an MVForge OCR baseline")
	}
}

func TestSubtitleOCRCorrectionAdvancesBaselineAndRejectsConcurrentStaleWriter(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", root)
	t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", root)
	sidecar := filepath.Join(root, "movie.eng.srt")
	source := filepath.Join(root, "source.mkv")
	writeTestSRT(t, sidecar, "The h0use")
	if err := os.WriteFile(source, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	evidence := testPublishedOCREvidence(sidecar, "The h0use")
	evidence.Identity.SourcePath = source
	info, _ := os.Stat(source)
	evidence.Identity.SourceSize = info.Size()
	evidence.Identity.SourceModTimeUnixNano = info.ModTime().UnixNano()
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
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".bak") || strings.Contains(entry.Name(), ".raw") || strings.HasPrefix(entry.Name(), ".subtitle-") {
			t.Fatalf("correction leaked a Library-visible staging artifact: %s", entry.Name())
		}
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
	t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", root)
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

func TestSubtitleOCRBothCorrectionRejectsStaleSibling(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", root)
	t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", root)
	srt := filepath.Join(root, "movie.srt")
	ass := filepath.Join(root, "movie.ass")
	writeTestSRT(t, srt, "The h0use")
	if err := os.WriteFile(ass, renderBitmapASSCues([]canonicalBitmapSubtitleCue{{Start: 1, End: 2, Text: "The h0use", SourceOrdinal: 1}}), 0o644); err != nil {
		t.Fatal(err)
	}
	evidence := testPublishedOCREvidence(srt, "The h0use")
	const logicalTransform = "both-stale"
	if err := commitPublishedCanonicalOCRSnapshot(context.Background(), srt, "srt", logicalTransform, evidence, "initial", ""); err != nil {
		t.Fatal(err)
	}
	if err := commitPublishedCanonicalOCRSnapshot(context.Background(), ass, "ass", logicalTransform, evidence, "initial", ""); err != nil {
		t.Fatal(err)
	}
	oldSRT, _, err := publishedSubtitleRevisionFor(srt, logicalTransform)
	if err != nil {
		t.Fatal(err)
	}
	oldASS, _, err := publishedSubtitleRevisionFor(ass, logicalTransform)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ass, []byte("external edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	updated := []canonicalBitmapSubtitleCue{{Start: 1, End: 2, Text: "The house", SourceOrdinal: 1}}
	staged, err := writeCorrectedSubtitleFiles(t.TempDir(), []string{srt, ass}, []string{"srt", "ass"}, updated)
	if err != nil {
		t.Fatal(err)
	}
	snapshots := []*publishedCanonicalOCRSnapshot{
		{Evidence: *cloneCanonicalOCREvidence(evidence), LogicalTransformKey: logicalTransform},
		{Evidence: *cloneCanonicalOCREvidence(evidence), LogicalTransformKey: logicalTransform},
	}
	if err := publishCorrectedSubtitleFiles(context.Background(), "both-stale", []string{srt, ass}, []string{"srt", "ass"}, staged, snapshots, []publishedSubtitleRevision{oldSRT, oldASS}); err == nil {
		t.Fatal("correction based on an edited sibling unexpectedly published")
	}
	srtContent, _ := os.ReadFile(srt)
	if !strings.Contains(string(srtContent), "The h0use") {
		t.Fatalf("stale sibling correction changed the other sidecar: %q", srtContent)
	}
}

func TestSubtitleOCRCorrectionCancellationBeforeBarrierDoesNotPublish(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", root)
	t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", root)
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
	t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", root)
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
	t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", root)
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

func TestPublishedOCRSnapshotsKeepMoreThanEightActiveBaselines(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", root)
	t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", root)
	paths := make([]string, 0, 10)
	for index := 0; index < 10; index++ {
		sidecar := filepath.Join(root, "movie-"+strconv.Itoa(index)+".srt")
		writeTestSRT(t, sidecar, "The house")
		evidence := testPublishedOCREvidence(sidecar, "The house")
		if err := commitPublishedCanonicalOCRSnapshot(context.Background(), sidecar, "srt", "active-"+strconv.Itoa(index), evidence, "test", ""); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, sidecar)
	}
	for index, sidecar := range paths {
		if _, ok := loadPublishedCanonicalOCRSnapshot(sidecar, "srt", "active-"+strconv.Itoa(index)); !ok {
			t.Fatalf("active OCR baseline %d was evicted", index)
		}
	}
}

func TestPublishedOCRSnapshotSamePathSourceReplacementFailsClosed(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", root)
	t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", root)
	sidecar := filepath.Join(root, "movie.srt")
	source := filepath.Join(root, "source.mkv")
	writeTestSRT(t, sidecar, "The house")
	if err := os.WriteFile(source, []byte("original-source"), 0o644); err != nil {
		t.Fatal(err)
	}
	evidence := testPublishedOCREvidence(source, "The house")
	if err := commitPublishedCanonicalOCRSnapshot(context.Background(), sidecar, "srt", "same-path", evidence, "test", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("replacement-source"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := loadPublishedCanonicalOCRSnapshot(sidecar, "srt", "same-path"); ok {
		t.Fatal("same-path source replacement reused the old OCR baseline")
	}
}

func TestSubtitleOCRCorrectionRequiresAnalyzeRevisionEvidence(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", root)
	t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", root)
	sidecar := filepath.Join(root, "movie.srt")
	writeTestSRT(t, sidecar, "The h0use")
	if _, err := validateSubtitleQualityCorrectionInput(subtitleOCRCorrectionInput{SidecarPath: sidecar, Format: "srt", Corrections: []subtitleOCRCorrection{{CueID: 1, Action: "edit", Text: "The house"}}}); err == nil {
		t.Fatal("correction without Analyze OCR revision evidence was accepted")
	}
}

func TestExternalSubtitleInventoryExposesExactOCROwnership(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "internal"))
	t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", filepath.Join(root, "internal"))
	media := filepath.Join(root, "movie.mkv")
	managed := filepath.Join(root, "movie.eng.srt")
	managedASS := filepath.Join(root, "movie.eng.ass")
	external := filepath.Join(root, "movie.spa.srt")
	if err := os.WriteFile(media, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTestSRT(t, managed, "The house")
	if err := os.WriteFile(managedASS, renderBitmapASSCues([]canonicalBitmapSubtitleCue{{Start: 1, End: 2, Text: "The house", SourceOrdinal: 1}}), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTestSRT(t, external, "La casa")
	evidence := testPublishedOCREvidence(managed, "The house")
	evidence.Identity.SourcePath = media
	info, _ := os.Stat(media)
	evidence.Identity.SourceSize = info.Size()
	evidence.Identity.SourceModTimeUnixNano = info.ModTime().UnixNano()
	if err := commitPublishedCanonicalOCRSnapshot(context.Background(), managed, "srt", "ocr-stream-4", evidence, "test", ""); err != nil {
		t.Fatal(err)
	}
	if err := commitPublishedCanonicalOCRSnapshot(context.Background(), managedASS, "ass", "ocr-stream-4", evidence, "test", ""); err != nil {
		t.Fatal(err)
	}
	values, err := externalSubtitlesForMedia(media)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 3 {
		t.Fatalf("expected both sidecars, got %#v", values)
	}
	var managedValue, managedASSValue, externalValue ExternalSubtitle
	for _, value := range values {
		if value.Path == managed {
			managedValue = value
		}
		if value.Path == managedASS {
			managedASSValue = value
		}
		if value.Path == external {
			externalValue = value
		}
	}
	if !managedValue.OCRManaged || !managedValue.OCRAnalysisAvailable || managedValue.LogicalTransformKey != "ocr-stream-4" {
		t.Fatalf("managed sidecar lost canonical ownership metadata: %+v", managedValue)
	}
	if !managedASSValue.OCRManaged || !managedASSValue.OCRAnalysisAvailable || managedASSValue.LogicalTransformKey != "ocr-stream-4" {
		t.Fatalf("logical both ASS sidecar lost canonical ownership metadata: %+v", managedASSValue)
	}
	if externalValue.OCRManaged || externalValue.OCRAnalysisAvailable || externalValue.LogicalTransformKey != "" {
		t.Fatalf("generic sidecar was labeled as OCR-managed: %+v", externalValue)
	}
}

func TestExternalSubtitleOwnershipIgnoresStaleOlderTransform(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "execution"))
	t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", filepath.Join(root, "persistent"))
	media := filepath.Join(root, "movie.mkv")
	sidecar := filepath.Join(root, "movie.eng.srt")
	if err := os.WriteFile(media, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTestSRT(t, sidecar, "The house")
	evidence := testPublishedOCREvidence(media, "The house")
	info, _ := os.Stat(media)
	evidence.Identity.SourcePath = media
	evidence.Identity.SourceSize = info.Size()
	evidence.Identity.SourceModTimeUnixNano = info.ModTime().UnixNano()
	if err := commitPublishedCanonicalOCRSnapshot(context.Background(), sidecar, "srt", "valid-new", evidence, "test", ""); err != nil {
		t.Fatal(err)
	}
	validSnapshot, ok := loadPublishedCanonicalOCRSnapshot(sidecar, "srt", "valid-new")
	if !ok {
		t.Fatal("valid OCR snapshot was not committed")
	}
	staleSnapshot := *validSnapshot
	staleSnapshot.LogicalTransformKey = "stale-old"
	staleSnapshot.OwnerIdentity = publishedSubtitleOwnerIdentity(sidecar, "srt", "stale-old")
	staleSnapshot.Evidence.Identity.SourceModTimeUnixNano = 1
	encoded, err := json.Marshal(staleSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(publishedSubtitleSnapshotPath(staleSnapshot.OwnerIdentity), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	ownershipIndex := loadPublishedOCRExternalOwnershipIndex()
	managed, available, logicalTransform := publishedOCRExternalOwnershipFromIndex(ownershipIndex, sidecar, "srt")
	if !managed || !available || logicalTransform != "valid-new" {
		t.Fatalf("stale ownership disabled valid transform: managed=%v available=%v logical=%q", managed, available, logicalTransform)
	}
}

func TestExternalSubtitleOwnershipWithOnlyStaleCandidateIsUnavailable(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "execution"))
	t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", filepath.Join(root, "persistent"))
	media := filepath.Join(root, "movie.mkv")
	sidecar := filepath.Join(root, "movie.eng.srt")
	if err := os.WriteFile(media, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTestSRT(t, sidecar, "The house")
	evidence := testPublishedOCREvidence(media, "The house")
	info, _ := os.Stat(media)
	evidence.Identity.SourcePath = media
	evidence.Identity.SourceSize = info.Size()
	evidence.Identity.SourceModTimeUnixNano = info.ModTime().UnixNano()
	if err := commitPublishedCanonicalOCRSnapshot(context.Background(), sidecar, "srt", "stale-only", evidence, "test", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(media, []byte("replacement"), 0o644); err != nil {
		t.Fatal(err)
	}
	ownershipIndex := loadPublishedOCRExternalOwnershipIndex()
	managed, available, logicalTransform := publishedOCRExternalOwnershipFromIndex(ownershipIndex, sidecar, "srt")
	if !managed || available || logicalTransform != "" {
		t.Fatalf("stale-only ownership did not fail closed: managed=%v available=%v logical=%q", managed, available, logicalTransform)
	}
}

func TestAssetInfoOCRAnalysisResolvesNonEmptyLogicalTransform(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "internal"))
	t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", filepath.Join(root, "internal"))
	media := filepath.Join(root, "movie.mkv")
	sidecar := filepath.Join(root, "movie.eng.srt")
	if err := os.WriteFile(media, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTestSRT(t, sidecar, "The house")
	evidence := testPublishedOCREvidence(sidecar, "The house")
	evidence.Identity.SourcePath = media
	info, _ := os.Stat(media)
	evidence.Identity.SourceSize = info.Size()
	evidence.Identity.SourceModTimeUnixNano = info.ModTime().UnixNano()
	logicalTransform := "asset-info-ocr-stream-4"
	if err := commitPublishedCanonicalOCRSnapshot(context.Background(), sidecar, "srt", logicalTransform, evidence, "test", ""); err != nil {
		t.Fatal(err)
	}
	operationID, err := startSubtitleOCRAnalysisOperation(subtitleOCRAnalyzeInput{MediaPath: media, SidecarPath: sidecar, Format: "srt", LogicalTransformKey: logicalTransform})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		subtitleExtractionOperations.Lock()
		delete(subtitleExtractionOperations.items, operationID)
		subtitleExtractionOperations.Unlock()
	}()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		subtitleExtractionOperations.RLock()
		operation := subtitleExtractionOperations.items[operationID]
		status := ""
		message := ""
		hasReport := false
		if operation != nil {
			status, message, hasReport = operation.Status, operation.Message, operation.QualityReport != nil
		}
		subtitleExtractionOperations.RUnlock()
		if status == "completed" {
			if !hasReport {
				t.Fatalf("Asset Info analysis completed without a quality report: %s", message)
			}
			return
		}
		if status == "error" {
			t.Fatalf("Asset Info analysis failed for exact logical transform: %s", message)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("Asset Info OCR analysis did not complete")
}

func TestAssetInfoBothOCRAnalysisUsesOneCanonicalBaseline(t *testing.T) {
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", filepath.Join(root, "internal"))
	t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", filepath.Join(root, "internal"))
	media := filepath.Join(root, "movie.mkv")
	srt := filepath.Join(root, "movie.srt")
	ass := filepath.Join(root, "movie.ass")
	if err := os.WriteFile(media, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTestSRT(t, srt, "The h0use")
	if err := os.WriteFile(ass, renderBitmapASSCues([]canonicalBitmapSubtitleCue{{Start: 1, End: 2, Text: "The h0use", SourceOrdinal: 1}}), 0o644); err != nil {
		t.Fatal(err)
	}
	evidence := testPublishedOCREvidence(media, "The h0use")
	info, _ := os.Stat(media)
	evidence.Identity.SourcePath = media
	evidence.Identity.SourceSize = info.Size()
	evidence.Identity.SourceModTimeUnixNano = info.ModTime().UnixNano()
	const logicalTransform = "both-analysis"
	if err := commitPublishedCanonicalOCRSnapshot(context.Background(), srt, "srt", logicalTransform, evidence, "test", ""); err != nil {
		t.Fatal(err)
	}
	if err := commitPublishedCanonicalOCRSnapshot(context.Background(), ass, "ass", logicalTransform, evidence, "test", ""); err != nil {
		t.Fatal(err)
	}
	operationID, err := startSubtitleOCRAnalysisOperation(subtitleOCRAnalyzeInput{
		MediaPath: media, SidecarPath: srt, SidecarPaths: []string{srt, ass}, Format: "both", LogicalTransformKey: logicalTransform,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		subtitleExtractionOperations.Lock()
		delete(subtitleExtractionOperations.items, operationID)
		subtitleExtractionOperations.Unlock()
	}()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		subtitleExtractionOperations.RLock()
		operation := subtitleExtractionOperations.items[operationID]
		if operation != nil && operation.Status == "completed" {
			report := operation.QualityReport
			subtitleExtractionOperations.RUnlock()
			if report == nil || report.Format != "both" || len(report.RevisionSet) != 2 || len(report.Findings) != 1 || report.SuspiciousCount != 1 || report.BaselineRevision != subtitleRevisionSetKey(report.RevisionSet) {
				t.Fatalf("logical both analysis was not one shared report: %+v", report)
			}
			return
		}
		subtitleExtractionOperations.RUnlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("logical both OCR analysis did not complete")
}

func TestSubtitleOCRCorrectionIntentSeparatesManualAndDeterministicApply(t *testing.T) {
	dictionaryRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(dictionaryRoot, "en.dic"), []byte("2\nhouse\nthe\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	previousFolder := ocrDictionaryFolderOverride
	ocrDictionaryFolderOverride = dictionaryRoot
	defer func() { ocrDictionaryFolderOverride = previousFolder }()
	snapshot := &publishedCanonicalOCRSnapshot{Format: "srt", Evidence: canonicalOCREvidence{RequestedLanguage: "eng", Cues: []canonicalBitmapSubtitleCue{{SourceOrdinal: 1, Text: "The h0use"}}}}
	if err := validateDeterministicOCRCorrectionIntents(snapshot, []subtitleOCRCorrection{{CueID: 1, Action: "edit", Text: "The house"}}); err != nil {
		t.Fatalf("individual manual edit was rejected: %v", err)
	}
	if err := validateDeterministicOCRCorrectionIntents(snapshot, []subtitleOCRCorrection{{CueID: 1, Action: "apply", Suggestion: "The house"}}); err != nil {
		t.Fatalf("deterministic suggestion apply was rejected: %v", err)
	}
	if err := validateDeterministicOCRCorrectionIntents(snapshot, []subtitleOCRCorrection{{CueID: 1, Action: "apply", Suggestion: "The hut"}}); err == nil {
		t.Fatal("arbitrary client suggestion was accepted as a deterministic apply")
	}
	if err := validateDeterministicOCRCorrectionIntents(snapshot, []subtitleOCRCorrection{{CueID: 1, Action: "apply", Text: "The hut"}}); err == nil {
		t.Fatal("manual text was accepted through the deterministic apply action")
	}
	provenance := correctionProvenanceFor(snapshot, []subtitleOCRCorrection{{CueID: 1, Action: "apply", Suggestion: "The house"}})
	if len(provenance) != 1 || provenance[0].CandidateSource != "deterministic-confusion-repair" || provenance[0].Validation != "deterministic-suggestion" {
		t.Fatalf("correction provenance did not retain candidate validation: %#v", provenance)
	}
}

type testSubtitleOCRContextualProvider struct {
	batches int
	seen    []subtitleOCRContextualCue
	err     error
}

func (provider *testSubtitleOCRContextualProvider) Suggest(ctx context.Context, cues []subtitleOCRContextualCue) ([]subtitleOCRContextualSuggestion, error) {
	provider.batches++
	provider.seen = append(provider.seen, cues...)
	if provider.err != nil {
		return nil, provider.err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []subtitleOCRContextualSuggestion{{CueID: cues[0].CueID, Text: "The house"}}, nil
}

func TestSubtitleOCRContextualProviderIsBoundedTextOnlyAndCancellable(t *testing.T) {
	dictionaryRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(dictionaryRoot, "en.dic"), []byte("2\nthe\nhouse\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	previousFolder := ocrDictionaryFolderOverride
	ocrDictionaryFolderOverride = dictionaryRoot
	defer func() { ocrDictionaryFolderOverride = previousFolder }()
	provider := &testSubtitleOCRContextualProvider{}
	findings := make([]subtitleOCRQualityFinding, 0, maxContextualOCRBatchCues+1)
	for index := 0; index < maxContextualOCRBatchCues+1; index++ {
		findings = append(findings, subtitleOCRQualityFinding{CueID: index + 1, OriginalText: "The h0use", Status: subtitleOCRQualityStatusSuspect, Reasons: []string{"review"}})
	}
	suggestions, err := boundedContextualOCRSuggestions(context.Background(), provider, findings, "eng")
	if err != nil || len(suggestions) != 1 || provider.batches != 1 || len(provider.seen) != maxContextualOCRBatchCues {
		t.Fatalf("provider request was not bounded to one candidate batch: suggestions=%#v provider=%#v err=%v", suggestions, provider, err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	provider.err = nil
	if _, err := boundedContextualOCRSuggestions(cancelled, provider, findings[:1], "eng"); !errors.Is(err, context.Canceled) {
		t.Fatalf("provider cancellation was not propagated: %v", err)
	}
}

func TestSelectiveOCRCapabilityIsBoundedAndPreparationAware(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(source, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(source)
	snapshot := &publishedCanonicalOCRSnapshot{Evidence: canonicalOCREvidence{Identity: canonicalOCRIdentity{
		SourcePath: source, SourceSize: info.Size(), SourceModTimeUnixNano: info.ModTime().UnixNano(), SourceSnapshotVersion: "stat-v1",
		StreamIndex: 4, StreamCodec: "dvd_subtitle", PreparationScope: "full-asset",
	}}}
	capability := selectiveOCRCueCapabilityFor(snapshot, canonicalBitmapSubtitleCue{SourceOrdinal: 1, Start: 12.5, End: 14.25})
	if !capability.Available || capability.SourcePath != source || capability.Start != 12.5 || capability.Duration != 2 {
		t.Fatalf("eligible bounded cue input was not recognized: %#v", capability)
	}
	snapshot.Evidence.Identity.PreparationScope = "segmented-test-encode"
	snapshot.Evidence.Identity.SegmentStart = "42.500000"
	segmented := selectiveOCRCueCapabilityFor(snapshot, canonicalBitmapSubtitleCue{SourceOrdinal: 1, Start: 2, End: 3})
	if !segmented.Available || segmented.Start != 44.5 {
		t.Fatalf("segmented cue input received incorrect source timing: %#v", segmented)
	}
	snapshot.Evidence.Identity.SegmentStart = "ambiguous"
	if unavailable := selectiveOCRCueCapabilityFor(snapshot, canonicalBitmapSubtitleCue{SourceOrdinal: 1, Start: 2, End: 3}); unavailable.Available || unavailable.Reason == "" {
		t.Fatalf("ambiguous segmented provenance did not fail closed: %#v", unavailable)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if unavailable := selectiveOCRCueCapabilityFor(snapshot, canonicalBitmapSubtitleCue{SourceOrdinal: 1, Start: 2, End: 3}); unavailable.Available || unavailable.Reason == "" {
		t.Fatalf("changed source identity did not disable selective OCR: %#v", unavailable)
	}
}

func TestSelectiveSecondaryOCRUsesOnlyBoundedCueInput(t *testing.T) {
	root := t.TempDir()
	bin := installCanonicalOCRTestTools(t, `#!/bin/sh
output=""
for argument do
  case "$argument" in
    --output-filename:*) output="${argument#--output-filename:}" ;;
    --track-number:5|--track-number:7) exit 91 ;;
  esac
done
printf '%s\n' '1' '00:00:01,000 --> 00:00:02,000' 'The house' > "$output"
`)
	writeExecutable(t, filepath.Join(bin, "ffmpeg"), `#!/bin/sh
output=""
for argument do output="$argument"; done
printf 'bounded subtitle input' > "$output"
`)
	writeExecutable(t, filepath.Join(bin, "ffprobe"), `#!/bin/sh
printf '%s' '{"streams":[{"index":0,"id":"0x1","codec_type":"subtitle","codec_name":"dvd_subtitle"}]}'
`)
	dictionaryRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(dictionaryRoot, "en.dic"), []byte("2\nthe\nhouse\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	previousFolder := ocrDictionaryFolderOverride
	ocrDictionaryFolderOverride = dictionaryRoot
	defer func() { ocrDictionaryFolderOverride = previousFolder }()
	source := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(source, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(source)
	snapshot := &publishedCanonicalOCRSnapshot{Evidence: canonicalOCREvidence{
		Identity:          canonicalOCRIdentity{SourcePath: source, SourceSize: info.Size(), SourceModTimeUnixNano: info.ModTime().UnixNano(), SourceSnapshotVersion: "stat-v1", StreamIndex: 4, StreamID: "0x7", StreamCodec: "dvd_subtitle"},
		RequestedLanguage: "eng", OCRMode: "raw",
	}}
	candidate, sourceName, err := runSelectiveSecondaryOCR(context.Background(), snapshot, canonicalBitmapSubtitleCue{SourceOrdinal: 1, Start: 1, End: 2, Text: "The h0use"}, t.TempDir(), nil)
	if err != nil || candidate != "The house" || sourceName != "selective-secondary-ocr" {
		t.Fatalf("bounded selective OCR did not produce the better candidate: candidate=%q source=%q err=%v", candidate, sourceName, err)
	}
}

func TestSelectiveOCRCueCapabilitySupportsDVDAndPGS(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(source, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(source)
	for _, codec := range []string{"dvd_subtitle", "hdmv_pgs_subtitle"} {
		snapshot := &publishedCanonicalOCRSnapshot{Evidence: canonicalOCREvidence{
			Identity: canonicalOCRIdentity{
				SourcePath: source, SourceSize: info.Size(), SourceModTimeUnixNano: info.ModTime().UnixNano(),
				SourceSnapshotVersion: "stat-v1", StreamIndex: 4, StreamCodec: codec,
			},
			RequestedLanguage: "eng", OCRMode: "raw",
		}}
		capability := selectiveOCRCueCapabilityFor(snapshot, canonicalBitmapSubtitleCue{SourceOrdinal: 1, Start: 1, End: 2})
		if !capability.Available || capability.Stream.Index != 4 || capability.Stream.CodecName != codec {
			t.Fatalf("selective OCR capability rejected %s: %#v", codec, capability)
		}
	}
}

func TestSubtitleOCRScoringUsesBoundedDictionaryEvidence(t *testing.T) {
	dictionaryRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(dictionaryRoot, "en.dic"), []byte("9\nhouse\nmorning\nlearn\nturn\nclear\ninclude\nproper\nname\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	previousFolder := ocrDictionaryFolderOverride
	ocrDictionaryFolderOverride = dictionaryRoot
	defer func() {
		ocrDictionaryFolderOverride = previousFolder
	}()
	valid := "morning learn turn clear include"
	if got := ocrConfusionCount(valid, "eng"); got != 0 {
		t.Fatalf("valid words were treated as OCR confusions: %d", got)
	}
	if suggestion, ok := deterministicOCRSuggestion("The h0use", "eng"); !ok || suggestion != "The house" {
		t.Fatalf("expected dictionary-backed OCR suggestion, got %q/%v", suggestion, ok)
	}
	if suggestion, ok := deterministicOCRSuggestion("Room 2024", "eng"); ok || suggestion != "" {
		t.Fatalf("real number received an OCR substitution: %q/%v", suggestion, ok)
	}
	if suggestion, ok := deterministicOCRSuggestion("Xylophonia", "eng"); ok || suggestion != "" {
		t.Fatalf("proper noun received an unsupported suggestion: %q/%v", suggestion, ok)
	}
	if suggestion, ok := deterministicOCRSuggestion("h0use", "zzz"); ok || suggestion != "" {
		t.Fatalf("unsupported language received a dictionary suggestion: %q/%v", suggestion, ok)
	}
}
