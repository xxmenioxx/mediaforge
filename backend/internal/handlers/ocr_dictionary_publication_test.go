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

func TestOCRDictionaryBothPublicationPreservesCueStructureAndASSFormatting(t *testing.T) {
	fixture := newOCRDictionaryBothPublicationFixture(t)
	updated := fixture.correctedEvidence(t)
	snapshots := []*publishedCanonicalOCRSnapshot{
		{Evidence: *cloneCanonicalOCREvidence(updated), LogicalTransformKey: fixture.logicalTransform},
		{Evidence: *cloneCanonicalOCREvidence(updated), LogicalTransformKey: fixture.logicalTransform},
	}
	staged, err := writeCorrectedSubtitleFiles(t.TempDir(), fixture.paths, fixture.formats, updated.Cues)
	if err != nil {
		t.Fatal(err)
	}
	if err := publishCorrectedSubtitleFiles(context.Background(), "dictionary-both-success", fixture.paths, fixture.formats, staged, snapshots, fixture.oldRevisions); err != nil {
		t.Fatal(err)
	}

	srtContent, err := os.ReadFile(fixture.paths[0])
	if err != nil {
		t.Fatal(err)
	}
	assContent, err := os.ReadFile(fixture.paths[1])
	if err != nil {
		t.Fatal(err)
	}
	for label, content := range map[string]string{"SRT": string(srtContent), "ASS": string(assContent)} {
		if !strings.Contains(content, "The I can't") {
			t.Errorf("%s did not publish the shared corrected cue text: %q", label, content)
		}
	}
	if !strings.Contains(string(srtContent), "00:01:05,000 --> 00:01:08,500") {
		t.Errorf("SRT timing changed: %q", srtContent)
	}
	if !strings.Contains(string(assContent), "Dialogue: 0,0:01:05.00,0:01:08.50,Default") || !strings.Contains(string(assContent), `{\an2}The I can't`) {
		t.Errorf("ASS timing, style, or geometry positioning changed: %q", assContent)
	}

	for index, path := range fixture.paths {
		snapshot, ok := loadPublishedCanonicalOCRSnapshot(path, fixture.formats[index], fixture.logicalTransform)
		if !ok {
			t.Fatalf("could not reload published snapshot for %s", path)
		}
		if !reflect.DeepEqual(snapshot.Evidence.Cues, updated.Cues) {
			t.Errorf("canonical cue structure changed for %s: got %#v want %#v", path, snapshot.Evidence.Cues, updated.Cues)
		}
		if snapshot.Evidence.Identity != fixture.evidence.Identity {
			t.Errorf("canonical source identity changed for %s: got %#v want %#v", path, snapshot.Evidence.Identity, fixture.evidence.Identity)
		}
		if len(snapshot.Evidence.CorrectionProvenance) != 1 || snapshot.Evidence.CorrectionProvenance[0].DictionaryRuleIDs[0] != "rule:eng:bar-cant" {
			t.Errorf("dictionary rule provenance missing from %s snapshot: %#v", path, snapshot.Evidence.CorrectionProvenance)
		}
	}
}

func TestOCRDictionaryBothPublicationRollsBackAllOutputs(t *testing.T) {
	fixture := newOCRDictionaryBothPublicationFixture(t)
	oldContents := make([][]byte, len(fixture.paths))
	for index, path := range fixture.paths {
		oldContents[index], _ = os.ReadFile(path)
	}
	updated := fixture.correctedEvidence(t)
	snapshots := []*publishedCanonicalOCRSnapshot{
		{Evidence: *cloneCanonicalOCREvidence(updated), LogicalTransformKey: fixture.logicalTransform},
		{Evidence: *cloneCanonicalOCREvidence(updated), LogicalTransformKey: fixture.logicalTransform},
	}
	staged, err := writeCorrectedSubtitleFiles(t.TempDir(), fixture.paths[:1], fixture.formats[:1], updated.Cues)
	if err != nil {
		t.Fatal(err)
	}
	staged = append(staged, filepath.Join(t.TempDir(), "missing-ass-stage.ass"))
	if err := publishCorrectedSubtitleFiles(context.Background(), "dictionary-both-rollback", fixture.paths, fixture.formats, staged, snapshots, fixture.oldRevisions); err == nil {
		t.Fatal("publication unexpectedly succeeded with a missing second staged output")
	}
	for index, path := range fixture.paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(content, oldContents[index]) {
			t.Errorf("failed logical-both publication left %s modified: got %q want %q", path, content, oldContents[index])
		}
		snapshot, ok := loadPublishedCanonicalOCRSnapshot(path, fixture.formats[index], fixture.logicalTransform)
		if !ok {
			t.Fatalf("could not reload published snapshot for %s", path)
		}
		if !reflect.DeepEqual(snapshot.Evidence.Cues, fixture.evidence.Cues) {
			t.Errorf("failed logical-both publication changed canonical snapshot for %s", path)
		}
		if snapshot.Revision != fixture.oldRevisions[index] {
			t.Errorf("failed logical-both publication changed the prior artifact revision for %s: got %#v want %#v", path, snapshot.Revision, fixture.oldRevisions[index])
		}
	}
}

type ocrDictionaryBothPublicationFixture struct {
	paths            []string
	formats          []string
	logicalTransform string
	evidence         *canonicalOCREvidence
	oldRevisions     []publishedSubtitleRevision
}

func newOCRDictionaryBothPublicationFixture(t *testing.T) ocrDictionaryBothPublicationFixture {
	t.Helper()
	root := t.TempDir()
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", root)
	t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", root)
	paths := []string{filepath.Join(root, "movie.srt"), filepath.Join(root, "movie.ass")}
	formats := []string{"srt", "ass"}
	text := "The | can't"
	writeTestSRT(t, paths[0], text)
	if err := os.WriteFile(paths[1], renderBitmapASSCues([]canonicalBitmapSubtitleCue{{Start: 65, End: 68.5, Text: text, SourceOrdinal: 9}}), 0o644); err != nil {
		t.Fatal(err)
	}
	evidence := testPublishedOCREvidence(paths[0], text)
	sourcePath := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(sourcePath, []byte("immutable source identity"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	evidence.Identity.SourcePath = sourcePath
	evidence.Identity.SourceSize = sourceInfo.Size()
	evidence.Identity.SourceModTimeUnixNano = sourceInfo.ModTime().UnixNano()
	evidence.DictionaryLanguage = "eng"
	evidence.DictionaryLanguageSource = "explicit_request"
	evidence.Cues = []canonicalBitmapSubtitleCue{{
		Start: 65, End: 68.5, Text: text, SourceOrdinal: 9,
		TimingProvenance: subtitleTimingProvenance("source-timestamps"),
		Geometry:         &bitmapCueGeometry{CanvasWidth: 1920, CanvasHeight: 1080, BitmapX: 640, BitmapY: 800, BitmapWidth: 640, BitmapHeight: 120},
		Role:             SubtitleSemanticRole("signs"),
	}}
	const logicalTransform = "dictionary-both-integrity"
	oldRevisions := make([]publishedSubtitleRevision, len(paths))
	for index, path := range paths {
		if err := commitPublishedCanonicalOCRSnapshot(context.Background(), path, formats[index], logicalTransform, evidence, "initial", ""); err != nil {
			t.Fatal(err)
		}
		oldRevision, _, err := publishedSubtitleRevisionFor(path, logicalTransform)
		if err != nil {
			t.Fatal(err)
		}
		oldRevisions[index] = oldRevision
	}
	return ocrDictionaryBothPublicationFixture{
		paths: paths, formats: formats, logicalTransform: logicalTransform,
		evidence: evidence, oldRevisions: oldRevisions,
	}
}

func (fixture ocrDictionaryBothPublicationFixture) correctedEvidence(t *testing.T) *canonicalOCREvidence {
	t.Helper()
	rule := models.OCRDictionaryEntry{
		ID: "rule:eng:bar-cant", Language: "eng", Pattern: "| can't", Replacement: "I can't",
		MatchMode: "literal", BoundaryPolicy: "word", CasePolicy: "exact", Priority: 100, Enabled: true, Source: "builtin", RuleVersion: 1,
	}
	matched := matchOCRDictionaryText(fixture.evidence.Cues[0].Text, "eng", []models.OCRDictionaryEntry{rule})
	if matched.Text != "The I can't" || len(matched.Matches) != 1 {
		t.Fatalf("dictionary matcher returned %#v", matched)
	}
	updated := cloneCanonicalOCREvidence(fixture.evidence)
	updated.Cues[0].Text = matched.Text
	updated.CorrectionProvenance = []canonicalOCRCorrectionProvenance{{
		CueID: 9, Action: "apply", CandidateSource: "contextual-ocr-dictionary",
		BeforeTextHash: "before-hash", AfterTextHash: "after-hash", DictionaryLanguage: "eng",
		DictionaryRevision: "dictionary-revision", DictionaryRuleIDs: []string{rule.ID},
		DictionaryRuleVersions: []int{rule.RuleVersion}, OperationID: "dictionary-both-success",
		Validation: "validated", SelectedBy: "user",
	}}
	return updated
}
