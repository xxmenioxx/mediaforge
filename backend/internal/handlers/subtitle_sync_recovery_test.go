package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type subtitleSyncRecoveryFixture struct {
	handler   AssetHandler
	db        *gorm.DB
	mediaPath string
	subtitle  string
	original  []byte
	source    subtitleTranslationSourceResponse
	proposal  SubtitleSyncProposal
	workspace string
	stagePath string
	output    []byte
}

func newSubtitleSyncRecoveryFixture(t *testing.T) subtitleSyncRecoveryFixture {
	t.Helper()
	root := t.TempDir()
	mediaPath := filepath.Join(root, "episode.mkv")
	subtitlePath := filepath.Join(root, "episode.es.srt")
	original := []byte("1\n00:00:01,000 --> 00:00:02,000\nOriginal\n")
	writeTestFile(t, mediaPath, "media fixture")
	writeTestFile(t, subtitlePath, string(original))
	installSubtitleSyncFFProbe(t, root)
	db := translationLibrarySourceTestDB(t, root, mediaPath)
	if err := db.AutoMigrate(&models.SubtitleSyncProvenance{}); err != nil {
		t.Fatal(err)
	}
	source, err := resolveExternalSubtitleTranslationSource(mediaPath, subtitlePath)
	if err != nil {
		t.Fatal(err)
	}
	audio, err := resolveSubtitleSyncAudio(context.Background(), mediaPath, 1)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := (fixedOffsetSubtitleSyncEngine{}).Analyze(context.Background(), SubtitleSyncAnalysisInput{
		SubtitlePath: subtitlePath, SourceIdentity: source.SourceIdentity, SourceRevision: source.SourceRevision,
		Representation: "srt", ReferenceAudio: audio, OffsetMilliseconds: 1000, Source: original,
	})
	if err != nil {
		t.Fatal(err)
	}
	output, err := (fixedOffsetSubtitleSyncEngine{}).Apply(context.Background(), original, proposal)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := os.MkdirTemp(root, ".mvforge-subtitle-sync-recovery-test-*")
	if err != nil {
		t.Fatal(err)
	}
	stagePath := filepath.Join(workspace, "output.srt")
	if err := os.WriteFile(stagePath, output, 0o600); err != nil {
		t.Fatal(err)
	}
	return subtitleSyncRecoveryFixture{
		handler: AssetHandler{db: db}, db: db, mediaPath: mediaPath, subtitle: subtitlePath,
		original: original, source: source, proposal: proposal, workspace: workspace, stagePath: stagePath, output: output,
	}
}

func TestSubtitleSyncFileCompensationFailurePersistsRecoveryAndBlocksApply(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := newSubtitleSyncRecoveryFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	err := fixture.handler.publishSubtitleSyncStageWithHooks(ctx, "sync-file-compensation-failure", fixture.workspace,
		fixture.mediaPath, fixture.source, fixture.proposal, fixture.stagePath, subtitleSyncPublicationHooks{
			afterPublish: cancel,
			restoreBackup: func(record subtitleSyncRecoveryRecord, target string) error {
				if target != fixture.subtitle {
					t.Fatalf("restore targeted unexpected file %q", target)
				}
				return errors.New("injected restore failure")
			},
		})
	cancel()
	if !errors.Is(err, errSubtitleSyncRecoveryRequired) {
		t.Fatalf("expected fail-closed recovery error, got %v", err)
	}
	if cleanupErr := cleanupSubtitleOperationWorkspace(fixture.workspace); cleanupErr != nil {
		t.Fatal(cleanupErr)
	}
	status := assertSubtitleSyncRecoveryState(t, fixture.subtitle, "recovery_required", fixture.proposal)
	if !status.BackupVerified || !status.TargetMatchesRecorded || status.Record.ProvenanceState != "not_persisted" {
		t.Fatalf("recovery evidence did not describe the failed file compensation: %#v", status)
	}
	if status.CurrentTargetSHA256 != status.Record.PublishedSHA256 {
		t.Fatalf("fixture did not retain the published target after injected restore failure: %#v", status)
	}
	assertSubtitleSyncApplyBlocked(t, fixture, status)
	assertSubtitleSyncRecoveryEndpoint(t, fixture, status)
}

func TestSubtitleSyncSQLiteCompensationFailurePersistsRecoveryAndBlocksApply(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fixture := newSubtitleSyncRecoveryFixture(t)
	if err := fixture.db.Exec(`CREATE TRIGGER reject_subtitle_sync_delete BEFORE DELETE ON subtitle_sync_provenances BEGIN SELECT RAISE(ABORT, 'injected provenance delete failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	err := fixture.handler.publishSubtitleSyncStageWithHooks(ctx, "sync-sqlite-compensation-failure", fixture.workspace,
		fixture.mediaPath, fixture.source, fixture.proposal, fixture.stagePath, subtitleSyncPublicationHooks{
			afterProvenanceCreate: cancel,
		})
	cancel()
	if !errors.Is(err, errSubtitleSyncRecoveryRequired) {
		t.Fatalf("expected fail-closed recovery error, got %v", err)
	}
	if cleanupErr := cleanupSubtitleOperationWorkspace(fixture.workspace); cleanupErr != nil {
		t.Fatal(cleanupErr)
	}
	status := assertSubtitleSyncRecoveryState(t, fixture.subtitle, "recovery_required", fixture.proposal)
	if !status.BackupVerified || !status.TargetMatchesRecorded || status.Record.ProvenanceState != "unknown" {
		t.Fatalf("recovery evidence did not describe the failed SQLite compensation: %#v", status)
	}
	if status.CurrentTargetSHA256 != status.Record.OriginalSHA256 {
		t.Fatalf("file compensation was not attempted after SQLite delete failed: %#v", status)
	}
	var provenanceCount int64
	if err := fixture.db.Model(&models.SubtitleSyncProvenance{}).Where("operation_id = ?", "sync-sqlite-compensation-failure").Count(&provenanceCount).Error; err != nil {
		t.Fatal(err)
	}
	if provenanceCount != 1 {
		t.Fatalf("expected trigger to retain one provenance row for reconciliation, got %d", provenanceCount)
	}
	assertSubtitleSyncApplyBlocked(t, fixture, status)
	assertSubtitleSyncRecoveryEndpoint(t, fixture, status)
}

func TestSubtitleSyncCancellationAfterCommitKeepsPublishedFileAndProvenance(t *testing.T) {
	fixture := newSubtitleSyncRecoveryFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	operationID := "sync-cancel-after-commit"
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[operationID] = &subtitleExtractionOperation{
		ID: operationID, Status: "running", cancel: cancel, publicationCommittedCh: make(chan struct{}),
	}
	subtitleExtractionOperations.Unlock()
	t.Cleanup(func() {
		cancel()
		subtitleExtractionOperations.Lock()
		delete(subtitleExtractionOperations.items, operationID)
		subtitleExtractionOperations.Unlock()
	})

	err := fixture.handler.publishSubtitleSyncStageWithHooks(ctx, operationID, fixture.workspace,
		fixture.mediaPath, fixture.source, fixture.proposal, fixture.stagePath, subtitleSyncPublicationHooks{
			afterCommit: cancel,
		})
	if err != nil {
		t.Fatalf("cancellation after the commit boundary changed the result: %v", err)
	}
	if !subtitleOperationPublicationCommitted(operationID) {
		t.Fatal("publication commit boundary was not recorded")
	}
	published, err := os.ReadFile(fixture.subtitle)
	if err != nil || string(published) != string(fixture.output) {
		t.Fatalf("post-commit cancellation rolled back the published subtitle: content=%q err=%v", published, err)
	}
	var provenance models.SubtitleSyncProvenance
	if err := fixture.db.First(&provenance, "operation_id = ?", operationID).Error; err != nil {
		t.Fatalf("post-commit cancellation removed persisted provenance: %v", err)
	}
	markerPath, err := subtitleSyncRecoveryMarkerPath(fixture.subtitle)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(markerPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("successful commit retained an unnecessary recovery marker: stat error=%v", err)
	}
}

func TestSubtitleSyncRecoveryIntentFailureAbortsPublication(t *testing.T) {
	fixture := newSubtitleSyncRecoveryFixture(t)
	err := fixture.handler.publishSubtitleSyncStageWithHooks(context.Background(), "sync-recovery-intent-failure", fixture.workspace,
		fixture.mediaPath, fixture.source, fixture.proposal, fixture.stagePath, subtitleSyncPublicationHooks{
			createRecoveryIntent: func(string, string, string, string, string, string) (*subtitleSyncRecoveryRecord, string, error) {
				return nil, "", errors.New("injected recovery intent persistence failure")
			},
		})
	if err == nil || !strings.Contains(err.Error(), "injected recovery intent persistence failure") {
		t.Fatalf("expected recovery intent failure before publication, got %v", err)
	}
	current, readErr := os.ReadFile(fixture.subtitle)
	if readErr != nil || string(current) != string(fixture.original) {
		t.Fatalf("subtitle changed after recovery intent persistence failed: content=%q err=%v", current, readErr)
	}
	var provenanceCount int64
	if err := fixture.db.Model(&models.SubtitleSyncProvenance{}).Where("operation_id = ?", "sync-recovery-intent-failure").Count(&provenanceCount).Error; err != nil {
		t.Fatal(err)
	}
	if provenanceCount != 0 {
		t.Fatalf("recovery intent failure persisted provenance: %d", provenanceCount)
	}
}

func assertSubtitleSyncRecoveryState(t *testing.T, target, expectedState string, proposal SubtitleSyncProposal) *subtitleSyncRecoveryStatus {
	t.Helper()
	status, err := inspectSubtitleSyncRecovery(target)
	if err != nil {
		t.Fatalf("recovery state did not verify: %v (%#v)", err, status)
	}
	if status == nil || status.State != expectedState || status.Record == nil {
		t.Fatalf("expected persisted recovery state %q, got %#v", expectedState, status)
	}
	if status.Record.SubtitlePath != target || status.Record.SourceRevision != proposal.SourceRevision ||
		status.Record.PublishedSHA256 == "" || status.Record.BackupPath == "" {
		t.Fatalf("recovery record omitted exact target, source, backup, or digest: %#v", status.Record)
	}
	backupDigest, err := subtitleSyncSHA256File(status.Record.BackupPath)
	if err != nil || backupDigest != status.Record.BackupSHA256 || backupDigest != status.Record.OriginalSHA256 {
		t.Fatalf("recovery backup is not independently verifiable: digest=%q err=%v record=%#v", backupDigest, err, status.Record)
	}
	if _, err := os.Stat(status.Record.BackupPath); err != nil {
		t.Fatalf("recovery backup did not survive workspace cleanup: %v", err)
	}
	return status
}

func assertSubtitleSyncApplyBlocked(t *testing.T, fixture subtitleSyncRecoveryFixture, status *subtitleSyncRecoveryStatus) {
	t.Helper()
	context, recorder := newSubtitleSyncTestContext(t, map[string]any{
		"path": fixture.mediaPath, "subtitlePath": fixture.subtitle, "referenceAudioStreamIndex": 1,
		"offsetMilliseconds": 1000, "sourceRevision": fixture.proposal.SourceRevision, "proposalId": fixture.proposal.ProposalID,
	})
	fixture.handler.StartSubtitleSync(context)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected later Apply to be blocked by persisted recovery, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Recovery *subtitleSyncRecoveryStatus `json:"recovery"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Recovery == nil || response.Recovery.Record == nil || response.Recovery.MarkerPath != status.MarkerPath {
		t.Fatalf("Apply did not surface persisted recovery evidence: %s", recorder.Body.String())
	}
}

func assertSubtitleSyncRecoveryEndpoint(t *testing.T, fixture subtitleSyncRecoveryFixture, status *subtitleSyncRecoveryStatus) {
	t.Helper()
	query := url.Values{"path": []string{fixture.mediaPath}, "subtitlePath": []string{fixture.subtitle}}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/assets/subtitle-sync/provenance?"+query.Encode(), nil)
	fixture.handler.SubtitleSyncProvenance(context)
	if recorder.Code != http.StatusOK {
		t.Fatalf("recovery provenance endpoint failed with %d: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Recovery *subtitleSyncRecoveryStatus `json:"recovery"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Recovery == nil || response.Recovery.Record == nil || response.Recovery.Record.BackupSHA256 != status.Record.BackupSHA256 {
		t.Fatalf("provenance endpoint did not surface durable recovery metadata: %s", recorder.Body.String())
	}
	if !strings.Contains(response.Recovery.Record.BackupPath, ".mvforge-subtitle-sync-recovery-backup-") {
		t.Fatalf("recovery endpoint did not expose the private backup path: %#v", response.Recovery.Record)
	}
}
