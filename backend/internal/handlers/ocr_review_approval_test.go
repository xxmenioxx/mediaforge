package handlers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

type canonicalOCRReviewFixture struct {
	root      string
	mediaPath string
	sidecar   string
	stream    FFProbeStream
	evidence  *canonicalOCREvidence
	request   canonicalOCRRequest
	target    *canonicalOCRReviewTarget
}

type cancelAfterErrContext struct {
	context.Context
	cancel   context.CancelFunc
	calls    int
	cancelAt int
}

func (ctx *cancelAfterErrContext) Err() error {
	ctx.calls++
	if ctx.calls >= ctx.cancelAt {
		ctx.cancel()
	}
	return ctx.Context.Err()
}

func newCanonicalOCRReviewFixture(t *testing.T, text string, cueEnd float64) canonicalOCRReviewFixture {
	t.Helper()
	root := t.TempDir()
	workspaceRoot := filepath.Join(root, "ocr-cache")
	if err := os.MkdirAll(workspaceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", workspaceRoot)
	t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", filepath.Join(root, "ocr-state"))
	mediaPath := filepath.Join(root, "movie.mkv")
	if err := os.WriteFile(mediaPath, []byte("protected PGS source"), 0o600); err != nil {
		t.Fatal(err)
	}
	stream := FFProbeStream{Index: 4, ID: "pgs-4", CodecName: "hdmv_pgs_subtitle", Tags: map[string]string{"language": "eng"}}
	ensureCanonicalOCRTestRuntime(t)
	request := canonicalOCRRequestFor(mediaPath, stream, "eng", "accurate", 0, 0)
	withRuntime, err := canonicalOCRRequestWithRuntime(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	evidence := &canonicalOCREvidence{
		Identity: withRuntime.Identity, RequestedLanguage: "eng", EffectiveLanguage: "eng", OCRMode: "accurate",
		Complete: true, Integrity: "canonical-cues-validated-v1",
		Cues: []canonicalBitmapSubtitleCue{{Start: 1, End: cueEnd, Text: text, SourceOrdinal: 1, TimingProvenance: originalSourceAbsolute}},
	}
	sidecar := filepath.Join(root, "movie.eng.srt")
	if err := os.WriteFile(sidecar, renderBitmapSRTCues(evidence.Cues), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := commitPublishedCanonicalOCRSnapshot(context.Background(), sidecar, "srt", "review-logical-transform", evidence, "review-fixture", ""); err != nil {
		t.Fatal(err)
	}
	selection, found, err := canonicalOCRTranslationSelectionForRequest(request)
	if err != nil || !found || selection.ReviewTarget == nil || selection.ReviewTarget.State != "exact" {
		t.Fatalf("review fixture did not resolve exactly: found=%v target=%#v err=%v", found, selection, err)
	}
	return canonicalOCRReviewFixture{root: root, mediaPath: mediaPath, sidecar: sidecar, stream: stream, evidence: evidence, request: request, target: selection.ReviewTarget}
}

func TestCanonicalOCRReviewJSONPreservesNanosecondRevisions(t *testing.T) {
	fixture := newCanonicalOCRReviewFixture(t, "The h0use", 2)
	if fixture.target.Identity.SourceModTimeUnixNano <= 1<<53 || fixture.target.Revisions[0].ModifiedAtUnixNano <= 1<<53 {
		t.Fatalf("fixture timestamps do not exercise JavaScript integer precision: source=%d artifact=%d", fixture.target.Identity.SourceModTimeUnixNano, fixture.target.Revisions[0].ModifiedAtUnixNano)
	}

	targetPayload, err := json.Marshal(fixture.target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(targetPayload), `"sourceModTimeUnixNano":"`) || !strings.Contains(string(targetPayload), `"modifiedAtUnixNano":"`) {
		t.Fatalf("review target did not serialize nanosecond fields as decimal strings: %s", targetPayload)
	}
	var decodedTarget canonicalOCRReviewTarget
	if err := json.Unmarshal(targetPayload, &decodedTarget); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*fixture.target, decodedTarget) {
		t.Fatalf("review target changed during JSON round trip: got %#v want %#v", decodedTarget, *fixture.target)
	}
	if _, _, _, err := canonicalOCRReviewTargetSnapshots(&decodedTarget); err != nil {
		t.Fatalf("round-tripped target no longer resolves to its exact canonical snapshots: %v", err)
	}

	report := subtitleOCRQualityReport{
		ArtifactPath: fixture.sidecar, Format: fixture.target.Format,
		LogicalTransformKey: fixture.target.LogicalTransformKey,
		Revision:            fixture.target.Revisions[0], RevisionSet: fixture.target.Revisions,
		BaselineRevision: fixture.target.BaselineRevision, AnalyzerVersion: subtitleOCRQualityVersion,
		Findings: []subtitleOCRQualityFinding{}, ReviewTarget: fixture.target,
	}
	reportPayload, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var decodedReport struct {
		Revision     publishedSubtitleRevision   `json:"revision"`
		RevisionSet  []publishedSubtitleRevision `json:"revisionSet"`
		ReviewTarget *canonicalOCRReviewTarget   `json:"reviewTarget"`
	}
	if err := json.Unmarshal(reportPayload, &decodedReport); err != nil {
		t.Fatal(err)
	}
	if decodedReport.Revision != fixture.target.Revisions[0] || !reflect.DeepEqual(decodedReport.RevisionSet, fixture.target.Revisions) || decodedReport.ReviewTarget == nil || !reflect.DeepEqual(*decodedReport.ReviewTarget, *fixture.target) {
		t.Fatalf("quality report revisions changed during JSON round trip: got %#v want %#v", decodedReport, report)
	}
	if !strings.Contains(string(reportPayload), `"modifiedAtUnixNano":"`+strconv.FormatInt(fixture.target.Revisions[0].ModifiedAtUnixNano, 10)+`"`) {
		t.Fatalf("quality report did not preserve the exact revision string: %s", reportPayload)
	}
}

func TestTranslationApprovalOnlyReviewPreservesSidecarAndRetriesWithoutLoop(t *testing.T) {
	fixture := newCanonicalOCRReviewFixture(t, "Hello|world", 40)
	fixture.evidence.CorrectionProvenance = []canonicalOCRCorrectionProvenance{{
		CueID: 1, Action: "apply", CandidateSource: "contextual-ocr-dictionary", TextHash: boundedTextHash("Hello|world"),
		BeforeTextHash: boundedTextHash("Hello|world"), AfterTextHash: boundedTextHash("Hello|world"),
		DictionaryLanguage: "eng", DictionaryRevision: "dictionary-revision-262", OperationID: "dictionary-operation",
		Validation: "dictionary-and-quality-gate", SelectedBy: "contextual-dictionary"}}
	if err := commitPublishedCanonicalOCRSnapshot(context.Background(), fixture.sidecar, "srt", fixture.target.LogicalTransformKey, fixture.evidence, "dictionary-correction", ""); err != nil {
		t.Fatal(err)
	}
	selection, found, err := canonicalOCRTranslationSelectionForRequest(fixture.request)
	if err != nil || !found {
		t.Fatalf("canonical OCR target could not be reselected: found=%v err=%v", found, err)
	}
	fixture.target = selection.ReviewTarget

	provider := &bitmapTranslationProviderSpy{}
	runner := func(ctx context.Context, operationID, _ string, _ subtitleTranslationOperationRequest) error {
		_, executionErr := executeBitmapTranslation(ctx, bitmapTranslationExecutionRequest{
			MediaPath: fixture.mediaPath, Stream: fixture.stream, RequestedLanguage: "en", OCRLanguage: "eng", OCRMode: "accurate",
			SourceRepresentation: "pgs", TargetLocale: "es-419", RequestedRepresentation: "srt",
			Provider: provider, Settings: bitmapTranslationExecutionTestSettings(), OperationID: operationID,
		})
		return executionErr
	}
	translationID, err := startSubtitleTranslationOperation(testSubtitleTranslationOperationRequest(fixture.mediaPath), runner)
	if err != nil {
		t.Fatal(err)
	}
	defer deleteSubtitleTranslationOperation(translationID)
	failed := waitForSubtitleTranslationOperation(t, translationID, "error")
	if failed.Translation == nil || failed.Translation.FailureClass != string(subtitleTranslationFailureOCRReviewRequired) ||
		failed.Translation.OCRReviewTarget == nil || failed.Translation.OCRReviewTarget.State != "exact" || provider.calls != 0 {
		t.Fatalf("blocked translation did not return an exact review target before provider work: %#v calls=%d", failed, provider.calls)
	}

	beforeBytes, err := os.ReadFile(fixture.sidecar)
	if err != nil {
		t.Fatal(err)
	}
	beforeRevision, _, err := publishedSubtitleRevisionFor(fixture.sidecar, fixture.target.LogicalTransformKey)
	if err != nil {
		t.Fatal(err)
	}
	input := subtitleOCRCorrectionInput{
		MediaPath: fixture.mediaPath, SidecarPath: fixture.sidecar, SidecarPaths: fixture.target.SidecarPaths,
		Format: "srt", LogicalTransformKey: fixture.target.LogicalTransformKey,
		BaselineRevision: fixture.target.BaselineRevision, Revisions: fixture.target.Revisions,
		Corrections: []subtitleOCRCorrection{{CueID: 1, Action: "approve"}}, ReviewTarget: failed.Translation.OCRReviewTarget,
	}
	correctionID, err := startSubtitleOCRCorrectionOperation(input)
	if err != nil {
		t.Fatal(err)
	}
	defer deleteSubtitleTranslationOperation(correctionID)
	if completed := waitForSubtitleOperationStatus(t, correctionID, "completed"); completed.Kind != "ocr_correction" {
		t.Fatalf("approval operation completed with unexpected kind: %#v", completed)
	}
	afterBytes, err := os.ReadFile(fixture.sidecar)
	if err != nil {
		t.Fatal(err)
	}
	afterRevision, _, err := publishedSubtitleRevisionFor(fixture.sidecar, fixture.target.LogicalTransformKey)
	if err != nil {
		t.Fatal(err)
	}
	if string(beforeBytes) != string(afterBytes) || beforeRevision != afterRevision {
		t.Fatalf("approval-only action rewrote the sidecar: before=%#v after=%#v", beforeRevision, afterRevision)
	}
	approvedSnapshot, ok := loadPublishedCanonicalOCRSnapshot(fixture.sidecar, "srt", fixture.target.LogicalTransformKey)
	if !ok {
		t.Fatal("approved canonical OCR snapshot could not be loaded")
	}
	if len(approvedSnapshot.Evidence.CorrectionProvenance) != 2 {
		t.Fatalf("approval replaced existing dictionary provenance: %#v", approvedSnapshot.Evidence.CorrectionProvenance)
	}
	approval := approvedSnapshot.Evidence.CorrectionProvenance[1]
	if !approval.HumanApproval || approval.Action != "approve" || approval.ApprovalSourceRevision != fixture.evidence.Identity.key() ||
		approval.ApprovalArtifactRevision != fixture.target.BaselineRevision || approval.ApprovalLogicalTransform != fixture.target.LogicalTransformKey ||
		approval.BeforeTextHash != boundedTextHash("Hello|world") || approval.AfterTextHash != boundedTextHash("Hello|world") ||
		approval.ApprovalAnalyzerVersion != subtitleOCRQualityVersion || approval.ApprovalRepairVersion != subtitleOCRRepairVersion ||
		approval.ApprovalScoringVersion != canonicalOCRScoringVersion {
		t.Fatalf("approval was not bound to the exact source, artifact, cue hash, and analyzer contract: %#v", approval)
	}

	retryID, err := retrySubtitleTranslationOperation(translationID, runner)
	if err != nil {
		t.Fatal(err)
	}
	defer deleteSubtitleTranslationOperation(retryID)
	completed := waitForSubtitleTranslationOperation(t, retryID, "completed")
	if completed.Phase != "completed" || provider.calls != 1 || len(provider.batches) != 1 || len(provider.batches[0].Cues) != 1 || provider.batches[0].Cues[0].Text != "Hello|world" {
		t.Fatalf("retry did not pass the approved unchanged cue exactly once: operation=%#v batches=%#v calls=%d", completed, provider.batches, provider.calls)
	}
	for _, testCase := range []struct {
		name   string
		mutate func(*canonicalOCREvidence)
	}{
		{name: "source revision", mutate: func(evidence *canonicalOCREvidence) {
			evidence.CorrectionProvenance[1].ApprovalSourceRevision = "stale-source-revision"
		}},
		{name: "artifact revision", mutate: func(evidence *canonicalOCREvidence) {
			evidence.CorrectionProvenance[1].ApprovalArtifactRevision = "stale-artifact-revision"
		}},
		{name: "cue text hash", mutate: func(evidence *canonicalOCREvidence) {
			evidence.CorrectionProvenance[1].AfterTextHash = boundedTextHash("different cue text")
		}},
	} {
		t.Run("stale approval "+testCase.name, func(t *testing.T) {
			staleEvidence := cloneCanonicalOCREvidence(&approvedSnapshot.Evidence)
			testCase.mutate(staleEvidence)
			if err := commitPublishedCanonicalOCRSnapshot(context.Background(), fixture.sidecar, "srt", fixture.target.LogicalTransformKey, staleEvidence, "stale-approval-test", ""); err != nil {
				t.Fatal(err)
			}
			staleProvider := &bitmapTranslationProviderSpy{}
			_, retryErr := executeBitmapTranslation(context.Background(), bitmapTranslationExecutionRequest{
				MediaPath: fixture.mediaPath, Stream: fixture.stream, RequestedLanguage: "en", OCRLanguage: "eng", OCRMode: "accurate",
				SourceRepresentation: "pgs", TargetLocale: "es-419", RequestedRepresentation: "srt",
				Provider: staleProvider, Settings: bitmapTranslationExecutionTestSettings(),
			})
			if retryErr == nil || subtitleTranslationFailureClassOf(retryErr) != subtitleTranslationFailureOCRReviewRequired || staleProvider.calls != 0 {
				t.Fatalf("stale approval reached the translation provider: calls=%d err=%v", staleProvider.calls, retryErr)
			}
		})
	}
}

func TestCanonicalOCRReviewTargetRejectsStaleAndConflictingSiblings(t *testing.T) {
	t.Run("stale artifact", func(t *testing.T) {
		fixture := newCanonicalOCRReviewFixture(t, "Hello|world", 40)
		if err := os.WriteFile(fixture.sidecar, []byte("external edit"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := canonicalOCRReviewTargetSnapshots(fixture.target); err == nil {
			t.Fatal("stale review target was accepted after an external sidecar edit")
		}
		selection, found, err := canonicalOCRTranslationSelectionForRequest(fixture.request)
		if err != nil || found || selection == nil || selection.ReviewTarget == nil || selection.ReviewTarget.State != "stale" {
			t.Fatalf("stale sidecar was not reported as a stale target: selection=%#v found=%v err=%v", selection, found, err)
		}
	})
	t.Run("stale source revision", func(t *testing.T) {
		fixture := newCanonicalOCRReviewFixture(t, "Hello|world", 40)
		if err := os.WriteFile(fixture.mediaPath, []byte("changed source revision"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := canonicalOCRReviewTargetSnapshots(fixture.target); err == nil {
			t.Fatal("review target remained valid after the source revision changed")
		}
		updatedRequest := canonicalOCRRequestFor(fixture.mediaPath, fixture.stream, "eng", "accurate", 0, 0)
		selection, found, err := canonicalOCRTranslationSelectionForRequest(updatedRequest)
		if err != nil || found || selection == nil || selection.ReviewTarget == nil || selection.ReviewTarget.State != "stale" {
			t.Fatalf("stale source was not reported as a stale target: selection=%#v found=%v err=%v", selection, found, err)
		}
	})
	t.Run("conflicting logical sibling", func(t *testing.T) {
		fixture := newCanonicalOCRReviewFixture(t, "Hello|world", 40)
		siblingRoot := filepath.Join(fixture.root, "sibling")
		if err := os.MkdirAll(siblingRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := publishCanonicalOCRReviewSibling(t, siblingRoot, fixture.mediaPath, fixture.stream, "A|C", "other-logical-transform"); err != nil {
			t.Fatal(err)
		}
		if _, found, err := canonicalOCRTranslationSelectionForRequest(fixture.request); err == nil || found {
			t.Fatalf("conflicting logical sibling was silently selected: found=%v err=%v", found, err)
		}
	})
}

func TestCanonicalOCRReviewSelectionDoesNotUseFilenameOrStreamIndexAlone(t *testing.T) {
	fixture := newCanonicalOCRReviewFixture(t, "Hello|world", 40)
	wrongStreamIdentity := fixture.stream
	wrongStreamIdentity.ID = "different-pgs-id"
	for name, stream := range map[string]FFProbeStream{
		"different stream id":      wrongStreamIdentity,
		"neighboring stream index": {Index: fixture.stream.Index + 1, ID: fixture.stream.ID, CodecName: fixture.stream.CodecName},
	} {
		t.Run(name, func(t *testing.T) {
			request := canonicalOCRRequestFor(fixture.mediaPath, stream, "eng", "accurate", 0, 0)
			selection, found, err := canonicalOCRTranslationSelectionForRequest(request)
			if err != nil || found || selection != nil {
				t.Fatalf("non-exact stream unexpectedly selected a sidecar: selection=%#v found=%v err=%v", selection, found, err)
			}
		})
	}
}

func TestCanonicalOCRReviewTargetGroupsOnlyEquivalentSRTASSMembers(t *testing.T) {
	t.Run("equivalent logical siblings", func(t *testing.T) {
		fixture := newCanonicalOCRReviewFixture(t, "Hello|world", 40)
		assPath := filepath.Join(fixture.root, "movie.eng.ass")
		if err := os.WriteFile(assPath, renderBitmapASSCues(fixture.evidence.Cues), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := commitPublishedCanonicalOCRSnapshot(context.Background(), assPath, "ass", fixture.target.LogicalTransformKey, fixture.evidence, "ass-sibling", ""); err != nil {
			t.Fatal(err)
		}
		selection, found, err := canonicalOCRTranslationSelectionForRequest(fixture.request)
		if err != nil || !found || selection.ReviewTarget.Format != "both" || len(selection.ReviewTarget.Artifacts) != 2 {
			t.Fatalf("equivalent SRT/ASS members did not resolve as one logical target: selection=%#v found=%v err=%v", selection, found, err)
		}
		paths, formats, snapshots, err := canonicalOCRReviewTargetSnapshots(selection.ReviewTarget)
		if err != nil || len(paths) != 2 || formats[0] != "srt" || formats[1] != "ass" || len(snapshots) != 2 {
			t.Fatalf("logical both target did not validate its complete revision set: paths=%v formats=%v err=%v", paths, formats, err)
		}
	})
	t.Run("conflicting canonical cues", func(t *testing.T) {
		fixture := newCanonicalOCRReviewFixture(t, "Hello|world", 40)
		assEvidence := cloneCanonicalOCREvidence(fixture.evidence)
		assEvidence.Cues[0].Text = "Hello|neighbor"
		assPath := filepath.Join(fixture.root, "movie.eng.ass")
		if err := os.WriteFile(assPath, renderBitmapASSCues(assEvidence.Cues), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := commitPublishedCanonicalOCRSnapshot(context.Background(), assPath, "ass", fixture.target.LogicalTransformKey, assEvidence, "conflicting-ass", ""); err != nil {
			t.Fatal(err)
		}
		if _, found, err := canonicalOCRTranslationSelectionForRequest(fixture.request); err == nil || found {
			t.Fatalf("SRT/ASS members with conflicting cues were silently grouped: found=%v err=%v", found, err)
		}
	})
}

func TestCanonicalOCRCorrectDecisionPublishesTextAndKeepsCueStructure(t *testing.T) {
	fixture := newCanonicalOCRReviewFixture(t, "The h0use", 2)
	beforeRevision, _, err := publishedSubtitleRevisionFor(fixture.sidecar, fixture.target.LogicalTransformKey)
	if err != nil {
		t.Fatal(err)
	}
	input := subtitleOCRCorrectionInput{
		MediaPath: fixture.mediaPath, SidecarPath: fixture.sidecar, SidecarPaths: fixture.target.SidecarPaths,
		Format: "srt", LogicalTransformKey: fixture.target.LogicalTransformKey,
		BaselineRevision: fixture.target.BaselineRevision, Revisions: fixture.target.Revisions,
		Corrections: []subtitleOCRCorrection{{CueID: 1, Action: "edit", Text: "The house"}}, ReviewTarget: fixture.target,
	}
	operationID, err := startSubtitleOCRCorrectionOperation(input)
	if err != nil {
		t.Fatal(err)
	}
	defer deleteSubtitleTranslationOperation(operationID)
	waitForSubtitleOperationStatus(t, operationID, "completed")
	content, err := os.ReadFile(fixture.sidecar)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "00:00:01,000 --> 00:00:02,000") || !strings.Contains(string(content), "The house") {
		t.Fatalf("correct action did not preserve cue timing and publish only the text edit: %q", content)
	}
	afterRevision, _, err := publishedSubtitleRevisionFor(fixture.sidecar, fixture.target.LogicalTransformKey)
	if err != nil || afterRevision == beforeRevision {
		t.Fatalf("correct action did not publish a new artifact revision: before=%#v after=%#v err=%v", beforeRevision, afterRevision, err)
	}
	snapshot, ok := loadPublishedCanonicalOCRSnapshot(fixture.sidecar, "srt", fixture.target.LogicalTransformKey)
	if !ok || len(snapshot.Evidence.Cues) != 1 || snapshot.Evidence.Cues[0].Start != 1 || snapshot.Evidence.Cues[0].End != 2 ||
		snapshot.Evidence.Cues[0].SourceOrdinal != 1 || snapshot.Evidence.Cues[0].Text != "The house" {
		t.Fatalf("canonical cue structure changed during Correct: %#v", snapshot)
	}
	if len(snapshot.Evidence.CorrectionProvenance) == 0 {
		t.Fatal("correct action did not persist human decision provenance")
	}
	decision := snapshot.Evidence.CorrectionProvenance[len(snapshot.Evidence.CorrectionProvenance)-1]
	if !decision.HumanApproval || decision.BeforeTextHash != boundedTextHash("The h0use") || decision.AfterTextHash != boundedTextHash("The house") ||
		decision.ApprovalArtifactRevision != afterRevision.key() {
		t.Fatalf("corrected cue did not receive approval bound to its committed revision: %#v", decision)
	}
}

func TestCanonicalOCRLeavePendingDoesNotPublishOrApprove(t *testing.T) {
	fixture := newCanonicalOCRReviewFixture(t, "Hello|world", 40)
	beforeBytes, err := os.ReadFile(fixture.sidecar)
	if err != nil {
		t.Fatal(err)
	}
	beforeSnapshot, ok := loadPublishedCanonicalOCRSnapshot(fixture.sidecar, "srt", fixture.target.LogicalTransformKey)
	if !ok {
		t.Fatal("baseline canonical OCR snapshot could not be loaded")
	}
	input := subtitleOCRCorrectionInput{
		MediaPath: fixture.mediaPath, SidecarPath: fixture.sidecar, SidecarPaths: fixture.target.SidecarPaths,
		Format: "srt", LogicalTransformKey: fixture.target.LogicalTransformKey,
		BaselineRevision: fixture.target.BaselineRevision, Revisions: fixture.target.Revisions,
		Corrections: []subtitleOCRCorrection{{CueID: 1, Action: "pending"}}, ReviewTarget: fixture.target,
	}
	operationID, err := startSubtitleOCRCorrectionOperation(input)
	if err != nil {
		t.Fatal(err)
	}
	defer deleteSubtitleTranslationOperation(operationID)
	waitForSubtitleOperationStatus(t, operationID, "completed")
	afterBytes, err := os.ReadFile(fixture.sidecar)
	if err != nil || string(beforeBytes) != string(afterBytes) {
		t.Fatalf("Leave pending changed the published sidecar: %q (%v)", afterBytes, err)
	}
	afterSnapshot, ok := loadPublishedCanonicalOCRSnapshot(fixture.sidecar, "srt", fixture.target.LogicalTransformKey)
	if !ok || len(afterSnapshot.Evidence.CorrectionProvenance) != len(beforeSnapshot.Evidence.CorrectionProvenance) {
		t.Fatalf("Leave pending stored an approval or changed the canonical snapshot: %#v", afterSnapshot)
	}
}

func publishCanonicalOCRReviewSibling(t *testing.T, root, mediaPath string, stream FFProbeStream, text, logicalTransform string) error {
	t.Helper()
	request := canonicalOCRRequestFor(mediaPath, stream, "eng", "accurate", 0, 0)
	withRuntime, err := canonicalOCRRequestWithRuntime(context.Background(), request)
	if err != nil {
		return err
	}
	evidence := &canonicalOCREvidence{
		Identity: withRuntime.Identity, RequestedLanguage: "eng", EffectiveLanguage: "eng", OCRMode: "accurate",
		Complete: true, Integrity: "canonical-cues-validated-v1",
		Cues: []canonicalBitmapSubtitleCue{{Start: 1, End: 40, Text: text, SourceOrdinal: 1}},
	}
	path := filepath.Join(root, "sibling.srt")
	if err := os.WriteFile(path, renderBitmapSRTCues(evidence.Cues), 0o600); err != nil {
		return err
	}
	return commitPublishedCanonicalOCRSnapshot(context.Background(), path, "srt", logicalTransform, evidence, "conflicting-sibling", "")
}

func TestCanonicalOCRApprovalCommitCancellationAndHashMismatchFailClosed(t *testing.T) {
	fixture := newCanonicalOCRReviewFixture(t, "Hello|world", 40)
	paths, formats, snapshots, err := canonicalOCRReviewTargetSnapshots(fixture.target)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(fixture.sidecar)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := commitCanonicalOCRApprovalSnapshots(ctx, "cancelled-approval", paths, formats, snapshots, fixture.target.Revisions); err == nil {
		t.Fatal("cancelled approval unexpectedly committed")
	}
	after, err := os.ReadFile(fixture.sidecar)
	if err != nil || string(before) != string(after) {
		t.Fatalf("cancelled approval changed the sidecar: %q (%v)", after, err)
	}
	snapshotPath := publishedSubtitleSnapshotPath(publishedSubtitleOwnerIdentity(fixture.sidecar, "srt", fixture.target.LogicalTransformKey))
	beforeSnapshot, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	base, cancelAfterWrite := context.WithCancel(context.Background())
	defer cancelAfterWrite()
	cancelDuringCommit := &cancelAfterErrContext{Context: base, cancel: cancelAfterWrite, cancelAt: 4}
	operationID := "cancel-after-approval-snapshot-write"
	decision := canonicalOCRCorrectionProvenance{
		CueID: 1, HumanApproval: true, AfterTextHash: boundedTextHash("Hello|world"),
		ApprovalSourceRevision: fixture.evidence.Identity.key(), ApprovalArtifactRevision: fixture.target.BaselineRevision,
		ApprovalLogicalTransform: fixture.target.LogicalTransformKey, ApprovalAnalyzerVersion: subtitleOCRQualityVersion,
		ApprovalRepairVersion: subtitleOCRRepairVersion, ApprovalScoringVersion: canonicalOCRScoringVersion,
		OperationID: operationID,
	}
	snapshots[0].Evidence.CorrectionProvenance = []canonicalOCRCorrectionProvenance{decision}
	if err := commitCanonicalOCRApprovalSnapshots(cancelDuringCommit, operationID, paths, formats, snapshots, fixture.target.Revisions); err == nil {
		t.Fatal("approval cancelled after snapshot write unexpectedly committed")
	}
	afterSnapshot, err := os.ReadFile(snapshotPath)
	if err != nil || string(beforeSnapshot) != string(afterSnapshot) {
		t.Fatalf("cancelled approval did not restore its prior canonical snapshot: %v", err)
	}
	after, err = os.ReadFile(fixture.sidecar)
	if err != nil || string(before) != string(after) {
		t.Fatalf("cancelled approval changed the sidecar: %q (%v)", after, err)
	}
	if approved := approvedCanonicalOCRFindings(snapshots[0], fixture.target.BaselineRevision, fixture.target.LogicalTransformKey); !approved[1] {
		t.Fatal("exact approval did not match its cue")
	}
	snapshots[0].Evidence.Cues[0].Text = "changed"
	if approved := approvedCanonicalOCRFindings(snapshots[0], fixture.target.BaselineRevision, fixture.target.LogicalTransformKey); approved[1] {
		t.Fatal("approval remained valid after its cue text hash changed")
	}
}
