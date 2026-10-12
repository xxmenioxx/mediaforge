package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/database"
	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type publisherDeliveryFixture struct {
	db          *gorm.DB
	job         models.QueueJob
	root        string
	rawRoot     string
	libraryRoot string
	archiveRoot string
	destination string
	output      string
	original    string
	sidecar     string
}

func assertQueueStageSequence(t *testing.T, job models.QueueJob, expected ...string) {
	t.Helper()
	encoded, err := json.Marshal(job.StageHistory)
	if err != nil {
		t.Fatalf("marshal stage history: %v", err)
	}
	var history []struct {
		Stage string `json:"stage"`
		At    string `json:"at"`
	}
	if err := json.Unmarshal(encoded, &history); err != nil {
		t.Fatalf("decode stage history: %v", err)
	}
	if len(history) != len(expected) {
		t.Fatalf("stage history %q has %d events, want exactly %d: %q", encoded, len(history), len(expected), expected)
	}
	var previous time.Time
	for index, event := range history {
		if event.Stage != expected[index] {
			t.Fatalf("stage history %q has stage %q at position %d, want %q", encoded, event.Stage, index, expected[index])
		}
		at, err := time.Parse(time.RFC3339, event.At)
		if err != nil {
			t.Fatalf("stage %q has no valid persisted transition timestamp %q: %v", event.Stage, event.At, err)
		}
		if !previous.IsZero() && at.Before(previous) {
			t.Fatalf("stage transition timestamps are out of order: %v before %v", at, previous)
		}
		previous = at
	}
}

func newPublisherDeliveryFixture(t *testing.T, requiredSidecar bool) publisherDeliveryFixture {
	return newPublisherDeliveryFixtureMode(t, requiredSidecar, PublishModeStandard)
}

func newPublisherDeliveryFixtureMode(t *testing.T, requiredSidecar bool, mode string) publisherDeliveryFixture {
	t.Helper()
	db := testEncodeTestDB(t, "publisher-delivery-"+t.Name(),
		&models.QueueJob{}, &models.ExecutionPlan{}, &models.ValidationRun{}, &models.ValidationOverride{},
		&models.AssetRecord{}, &models.PublisherPublication{}, &models.PublisherPublicationAttempt{},
		&models.PublisherAttemptEvidence{}, &models.PublisherAuditEvent{}, &models.AssetPublishedOutbox{},
	)
	root := t.TempDir()
	rawRoot := filepath.Join(root, "raw")
	libraryRoot := filepath.Join(root, "library")
	archiveRoot := filepath.Join(root, "archive")
	output := filepath.Join(root, "staging", "job-1", "feature.mkv")
	original := filepath.Join(rawRoot, "series", "episode.mkv")
	if mode == PublishModeReplaceLibrary {
		original = filepath.Join(libraryRoot, "series", "episode.mkv")
	}
	destination := filepath.Join(libraryRoot, "series", "episode.mkv")
	sidecar := strings.TrimSuffix(output, filepath.Ext(output)) + ".eng.ass"
	for _, file := range []string{output, original} {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(output, []byte("canonical result media"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(original, []byte("raw original media"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{
		"rawRoot": rawRoot, "originalsArchivePath": archiveRoot,
		"resultsReportsPath": filepath.Join(root, "reports"), "stagingPath": filepath.Join(root, "staging"),
	}}).Error; err != nil {
		t.Fatal(err)
	}
	library := models.Library{Name: "Anime", SourcePath: rawRoot, DestinationPath: libraryRoot, Type: "series"}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	var sourceRecordID *uint
	info, err := os.Stat(original)
	if err != nil {
		t.Fatal(err)
	}
	recordRoot, recordStatus := rawRoot, "unprocessed"
	if mode == PublishModeReplaceLibrary {
		recordRoot, recordStatus = libraryRoot, "converted"
	}
	record := models.AssetRecord{Path: original, RootPath: recordRoot, RelativePath: filepath.ToSlash(filepath.Join("series", "episode.mkv")), FileName: filepath.Base(original), Extension: ".mkv", SizeBytes: info.Size(), ModifiedAt: info.ModTime(), Status: recordStatus, LibraryID: library.ID, LibraryName: library.Name, SyncedAt: time.Now().UTC()}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	sourceRecordID = &record.ID
	executionNumber := uint(1)
	job := models.QueueJob{
		ExecutionNumber: &executionNumber, MediaPath: original, OutputPath: output,
		Status: JobStatusCompleted, Stage: JobStageReadyToPublish, LibraryID: library.ID,
		ProfileID: 1, PublishMode: mode, PlannedLibraryRoot: libraryRoot, SourceAssetRecordID: sourceRecordID,
		PlannedPublishedPath: destination, ValidationStatus: ValidationStatusPassed,
	}
	if mode == PublishModeReplaceLibrary {
		job.ReplacementTargetPath = original
	}
	if requiredSidecar {
		trackPlan := ResolvedTrackPlan{AttachmentPolicy: AttachmentPolicyAuto, ChapterPolicy: ChapterPolicyKeep, SidecarOutputs: []ResolvedTrackSidecar{{StreamIndex: 3, Codec: "ass", Language: "eng", Format: "ass", Mode: "original"}}}
		planMap, err := resolvedTrackPlanMap(trackPlan)
		if err != nil {
			t.Fatal(err)
		}
		job.TrackProfileSnapshot = models.JSONMap{resolvedTrackPlanSnapshotKey: planMap}
		artifactContent := []byte("[Events]\nDialogue: 0,0:00:00.00,0:00:01.00,Default,,0,0,0,,delivery sidecar\n")
		if err := os.WriteFile(sidecar, artifactContent, 0o600); err != nil {
			t.Fatal(err)
		}
		job.SubtitleArtifacts = subtitleArtifactsJSON([]SubtitleArtifact{{StreamIndex: 3, SourceCodec: "ass", Format: "ass", Mode: "original", Language: "eng", StagedPath: sidecar, Status: "ready"}})
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	plan := models.ExecutionPlan{
		JobID: job.ID, Version: 1, Status: "frozen", LibraryID: library.ID,
		DestinationRoot: libraryRoot, DestinationPath: destination, PublishMode: mode,
		OutputPath: output,
	}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	job.ActiveExecutionPlanID = &plan.ID
	if err := db.Save(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := writeJobResultArtifact(db, job, map[string]any{"status": "completed"}); err != nil {
		t.Fatal(err)
	}
	artifact, _, err := readLatestJobArtifact(db, job, "result")
	if err != nil {
		t.Fatal(err)
	}
	identity, identityOK := resultIdentityFromArtifact(artifact)
	binding, bindingOK := resultBindingFromArtifact(artifact)
	if !identityOK || !bindingOK {
		t.Fatalf("result identity unavailable: %#v", artifact)
	}
	run := models.ValidationRun{
		RunID: "validation-run:publisher-delivery:" + t.Name(), JobID: job.ID,
		ExecutionNumber: executionNumber, ExecutionPlanID: binding.ExecutionPlanID,
		ExecutionPlanVersion: binding.ExecutionPlanVersion, ExecutionPlanFingerprint: binding.ExecutionPlanFingerprint,
		ResultArtifactID: identity.ArtifactID, ResultGenerationToken: identity.GenerationToken,
		ResultGenerationVersion: identity.GenerationVersion, ResultOutputRevision: identity.OutputRevision,
		SchemaVersion: validationSchemaVersion, ValidatorVersion: validationEngineVersion,
		RulesetID: validationRulesetID, RulesetVersion: validationRulesetVersion,
		ActorSource: publicationActorSystem, ActorID: publicationActorSystem, Outcome: "PASS",
		Findings: models.JSONList{}, Report: models.JSONMap{}, CreatedAt: time.Now().UTC(),
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	job.ValidationRunID = run.RunID
	job.ValidationOutcome = "PASS"
	job.ValidationStatus = ValidationStatusPassed
	if err := db.Save(&job).Error; err != nil {
		t.Fatal(err)
	}
	return publisherDeliveryFixture{db: db, job: job, root: root, rawRoot: rawRoot, libraryRoot: libraryRoot, archiveRoot: archiveRoot, destination: destination, output: output, original: original, sidecar: sidecar}
}

func TestPublisherDeliveryCommitsRequiredSidecarArchiveAndStableOutbox(t *testing.T) {
	fixture := newPublisherDeliveryFixture(t, true)
	handler := PublisherHandler{db: fixture.db}
	result, err := handler.publishQueueJobWithMode(fixture.job, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "published" || result.PublishedPath != fixture.destination {
		t.Fatalf("unexpected result: %#v", result)
	}
	for path, want := range map[string]string{fixture.destination: "canonical result media", strings.TrimSuffix(fixture.destination, filepath.Ext(fixture.destination)) + ".eng.ass": "[Events]\nDialogue: 0,0:00:00.00,0:00:01.00,Default,,0,0,0,,delivery sidecar\n"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("delivered file %s: content=%q err=%v", path, got, err)
		}
	}
	if _, err := os.Stat(fixture.original); !os.IsNotExist(err) {
		var archiveDebug models.PublisherPublication
		_ = fixture.db.Where("queue_job_id = ?", fixture.job.ID).First(&archiveDebug).Error
		t.Fatalf("original was not archived after publication: stat=%v status=%s source=%s destination=%s error=%s", err, archiveDebug.OriginalArchiveStatus, archiveDebug.OriginalArchiveSourcePath, archiveDebug.OriginalArchivedPath, archiveDebug.OriginalArchiveError)
	}
	var publication models.PublisherPublication
	if err := fixture.db.Where("queue_job_id = ?", fixture.job.ID).First(&publication).Error; err != nil {
		t.Fatal(err)
	}
	if publication.State != PublicationPublished || publication.OriginalArchiveStatus != "complete" || publication.FencingGeneration != 1 {
		t.Fatalf("unexpected publication state: %#v", publication)
	}
	var job models.QueueJob
	if err := fixture.db.First(&job, fixture.job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if job.PublicationState != PublicationPublished || job.OriginalArchiveStatus != "complete" || job.PublishedAt == nil {
		t.Fatalf("Queue projection was not committed: %#v", job)
	}
	if job.Status != JobStatusCompleted || job.Stage != JobStageCompleted || job.OriginalArchivedPath != publication.OriginalArchivedPath {
		t.Fatalf("terminal archive projection is incomplete: %#v", job)
	}
	assertQueueStageSequence(t, job, JobStagePublishing, JobStageArchivingOriginal, JobStageCompleted)
	var archivedSource models.AssetRecord
	if err := fixture.db.First(&archivedSource, *publication.SourceAssetRecordID).Error; err != nil || !archivedSource.Missing {
		t.Fatalf("#174 source inventory was not updated after successful archive: record=%#v err=%v", archivedSource, err)
	}
	var outbox []models.AssetPublishedOutbox
	if err := fixture.db.Where("publication_id = ?", publication.PublicationID).Find(&outbox).Error; err != nil {
		t.Fatal(err)
	}
	if len(outbox) != 1 || outbox[0].EventType != "AssetPublished" || outbox[0].EventID != "asset-published-"+publication.PublicationID {
		t.Fatalf("expected one stable AssetPublished outbox identity, got %#v", outbox)
	}
	var attempt models.PublisherPublicationAttempt
	if err := fixture.db.Where("publication_id = ?", publication.PublicationID).First(&attempt).Error; err != nil {
		t.Fatal(err)
	}
	if attempt.ActorID != publicationActorOperator || attempt.FencingGeneration != 1 {
		t.Fatalf("unexpected server-assigned attempt actor: %#v", attempt)
	}
	var audit models.PublisherAuditEvent
	if err := fixture.db.Where("publication_id = ?", publication.PublicationID).First(&audit).Error; err != nil {
		t.Fatal(err)
	}
	if audit.ActorID != publicationActorOperator {
		t.Fatalf("unexpected server-assigned audit actor: %#v", audit)
	}
	second, err := handler.publishQueueJobWithMode(job, false, false)
	if err != nil || second.Status != "already_published" {
		t.Fatalf("idempotent handoff result=%#v err=%v", second, err)
	}
	var eventCount int64
	if err := fixture.db.Model(&models.AssetPublishedOutbox{}).Where("publication_id = ?", publication.PublicationID).Count(&eventCount).Error; err != nil || eventCount != 1 {
		t.Fatalf("idempotent handoff changed outbox count=%d err=%v", eventCount, err)
	}

	deliveredIDs := []string{}
	firstDelivery := true
	unregister := RegisterAssetPublishedSubscriber(func(_ context.Context, eventID string, _ models.JSONMap) error {
		deliveredIDs = append(deliveredIDs, eventID)
		if firstDelivery {
			firstDelivery = false
			return errors.New("ambiguous acknowledgement")
		}
		return nil
	})
	defer unregister()
	auto := &AutoPublisher{db: fixture.db}
	auto.dispatchAssetPublishedOutbox()
	auto.dispatchAssetPublishedOutbox()
	if len(deliveredIDs) != 2 || deliveredIDs[0] != deliveredIDs[1] || deliveredIDs[0] != outbox[0].EventID {
		t.Fatalf("outbox did not retry the same stable identity: %v", deliveredIDs)
	}
	var delivered models.AssetPublishedOutbox
	if err := fixture.db.First(&delivered, outbox[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if delivered.Status != "delivered" || delivered.AttemptCount != 2 {
		t.Fatalf("subscriber retry state=%#v", delivered)
	}
	var persistedPublication models.PublisherPublication
	if err := fixture.db.First(&persistedPublication, publication.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persistedPublication.State != PublicationPublished {
		t.Fatalf("subscriber failure changed publication state: %s", persistedPublication.State)
	}
}

func TestPublisherStandardModeBlocksUnownedDestinationEvenWhenClientRequestsOverwrite(t *testing.T) {
	fixture := newPublisherDeliveryFixture(t, false)
	if err := os.MkdirAll(filepath.Dir(fixture.destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.destination, []byte("unowned Library file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (PublisherHandler{db: fixture.db}).publishQueueJobWithMode(fixture.job, true, false); err == nil {
		t.Fatal("standard publication overwrote an unowned destination")
	}
	if got, err := os.ReadFile(fixture.destination); err != nil || string(got) != "unowned Library file" {
		t.Fatalf("conflicting destination changed: content=%q err=%v", got, err)
	}
	if got, err := os.ReadFile(fixture.output); err != nil || string(got) != "canonical result media" {
		t.Fatalf("canonical Result was changed on conflict: content=%q err=%v", got, err)
	}
	if got, err := os.ReadFile(fixture.original); err != nil || string(got) != "raw original media" {
		t.Fatalf("original was archived despite blocked publication: content=%q err=%v", got, err)
	}
	var publication models.PublisherPublication
	if err := fixture.db.Where("queue_job_id = ?", fixture.job.ID).First(&publication).Error; err != nil {
		t.Fatal(err)
	}
	if publication.State != PublicationBlocked || publication.FailureCode != "destination_conflict" {
		t.Fatalf("conflict was not persisted as BLOCKED: %#v", publication)
	}
	var events int64
	if err := fixture.db.Model(&models.AssetPublishedOutbox{}).Where("publication_id = ?", publication.PublicationID).Count(&events).Error; err != nil || events != 0 {
		t.Fatalf("blocked publication created AssetPublished event count=%d err=%v", events, err)
	}
}

func TestPublisherUsesQueueFrozenDestinationAfterLibrarySettingChanges(t *testing.T) {
	fixture := newPublisherDeliveryFixture(t, false)
	changedRoot := filepath.Join(fixture.root, "changed-library")
	if err := fixture.db.Model(&models.Library{}).Where("id = ?", fixture.job.LibraryID).Update("destination_path", changedRoot).Error; err != nil {
		t.Fatal(err)
	}
	result, err := (PublisherHandler{db: fixture.db}).publishQueueJobWithMode(fixture.job, false, false)
	if err != nil || result.Status != "published" {
		t.Fatalf("frozen-destination publication result=%#v err=%v", result, err)
	}
	if result.PublishedPath != fixture.destination {
		t.Fatalf("Publisher recalculated destination from changed Library: got %s want %s", result.PublishedPath, fixture.destination)
	}
	if _, err := os.Stat(filepath.Join(changedRoot, "series", "episode.mkv")); !os.IsNotExist(err) {
		t.Fatalf("Publisher wrote to the current Library destination: %v", err)
	}
}

func TestPublisherBlocksSymlinkedDestinationStagingRoot(t *testing.T) {
	fixture := newPublisherDeliveryFixture(t, false)
	outside := filepath.Join(fixture.root, "outside")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(fixture.libraryRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(fixture.libraryRoot, ".mvforge-staging")); err != nil {
		t.Fatal(err)
	}
	if _, err := (PublisherHandler{db: fixture.db}).publishQueueJobWithMode(fixture.job, false, false); err == nil {
		t.Fatal("Publisher accepted a symlinked destination staging root")
	}
	var publication models.PublisherPublication
	if err := fixture.db.Where("queue_job_id = ?", fixture.job.ID).First(&publication).Error; err != nil {
		t.Fatal(err)
	}
	if publication.State != PublicationBlocked || publication.FailureCode != "staging_root" {
		t.Fatalf("symlinked staging root was not blocked: %#v", publication)
	}
	if _, err := os.Stat(fixture.destination); !os.IsNotExist(err) {
		t.Fatalf("symlinked staging root caused final materialization: %v", err)
	}
}

func TestPublisherReplaceModeArchivesOnlyTheFrozenCanonicalAsset(t *testing.T) {
	fixture := newPublisherDeliveryFixtureMode(t, false, PublishModeReplaceLibrary)
	result, err := (PublisherHandler{db: fixture.db}).publishQueueJobWithMode(fixture.job, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "published" || result.PublishedPath != fixture.original {
		t.Fatalf("unexpected replacement result: %#v", result)
	}
	if got, err := os.ReadFile(fixture.original); err != nil || string(got) != "canonical result media" {
		t.Fatalf("canonical replacement content=%q err=%v", got, err)
	}
	archived := filepath.Join(fixture.archiveRoot, "library-replacements", "library-1", "series", "episode.mkv")
	if got, err := os.ReadFile(archived); err != nil || string(got) != "raw original media" {
		t.Fatalf("original replacement archive content=%q err=%v", got, err)
	}
	var publication models.PublisherPublication
	if err := fixture.db.Where("queue_job_id = ?", fixture.job.ID).First(&publication).Error; err != nil {
		t.Fatal(err)
	}
	if publication.PublishMode != PublishModeReplaceLibrary || publication.OriginalArchiveStatus != "complete" {
		t.Fatalf("replacement did not preserve frozen mode and separate archive outcome: %#v", publication)
	}
}

func TestPublisherArchiveFailureLeavesPublishedStateAndOriginalInventoryIntact(t *testing.T) {
	fixture := newPublisherDeliveryFixture(t, false)
	archiveTarget := filepath.Join(fixture.root, "archive-target")
	if err := os.MkdirAll(archiveTarget, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(archiveTarget, fixture.archiveRoot); err != nil {
		t.Fatal(err)
	}
	result, err := (PublisherHandler{db: fixture.db}).publishQueueJobWithMode(fixture.job, false, false)
	if err != nil || result.Status != "published" {
		t.Fatalf("archive failure changed publication result: result=%#v err=%v", result, err)
	}
	var publication models.PublisherPublication
	if err := fixture.db.Where("queue_job_id = ?", fixture.job.ID).First(&publication).Error; err != nil {
		t.Fatal(err)
	}
	if publication.State != PublicationPublished || publication.OriginalArchiveStatus != "failed" {
		t.Fatalf("publication and archive outcomes were not kept separate: %#v", publication)
	}
	var original models.AssetRecord
	if err := fixture.db.First(&original, *publication.SourceAssetRecordID).Error; err != nil || original.Missing {
		t.Fatalf("failed archive changed #174 original inventory: record=%#v err=%v", original, err)
	}
	if _, err := os.Stat(fixture.original); err != nil {
		t.Fatalf("original was lost after archive failure: %v", err)
	}
	var outboxCount int64
	if err := fixture.db.Model(&models.AssetPublishedOutbox{}).Where("publication_id = ?", publication.PublicationID).Count(&outboxCount).Error; err != nil || outboxCount != 1 {
		t.Fatalf("archive outcome did not unlock one AssetPublished event: count=%d err=%v", outboxCount, err)
	}
}

func TestPublisherMissingRequiredSidecarBlocksBeforeAnyPromotion(t *testing.T) {
	fixture := newPublisherDeliveryFixture(t, true)
	if err := os.Remove(fixture.sidecar); err != nil {
		t.Fatal(err)
	}
	if _, err := (PublisherHandler{db: fixture.db}).publishQueueJobWithMode(fixture.job, false, false); err == nil {
		t.Fatal("publication accepted a missing required Result sidecar")
	}
	if _, err := os.Stat(fixture.destination); !os.IsNotExist(err) {
		t.Fatalf("primary media was promoted despite missing required sidecar: %v", err)
	}
	if _, err := os.Stat(fixture.original); err != nil {
		t.Fatalf("original was archived despite missing required sidecar: %v", err)
	}
	var publication models.PublisherPublication
	if err := fixture.db.Where("queue_job_id = ?", fixture.job.ID).First(&publication).Error; err != nil {
		t.Fatal(err)
	}
	if publication.State != PublicationBlocked || publication.FailureCode != "required_sidecar_missing" {
		t.Fatalf("missing sidecar was not persisted as BLOCKED: %#v", publication)
	}
}

func TestPublisherCapacityPreflightRejectsInsufficientSpace(t *testing.T) {
	if err := checkPublisherCapacity(1024, 1023); err == nil {
		t.Fatal("insufficient destination capacity was accepted")
	}
	if err := checkPublisherCapacity(1024, 1024); err != nil {
		t.Fatalf("exact available capacity was rejected: %v", err)
	}
}

func TestPublisherExpiredClaimRecoveryFencesStaleOwnerAfterFinalRename(t *testing.T) {
	fixture := newPublisherDeliveryFixture(t, false)
	handler := PublisherHandler{db: fixture.db}
	candidate, identity, err := publisherIdentityForJob(fixture.db, fixture.job, false)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := handler.persistPublicationIdentity(candidate, identity)
	if err != nil {
		t.Fatal(err)
	}
	claimed, staleOwner, err := handler.claimPublication(publication.PublicationID, publicationActorOperator, publicationActorOperator, false)
	if err != nil || !claimed {
		t.Fatalf("initial claim=%v err=%v", claimed, err)
	}
	secondClaimed, _, err := handler.claimPublication(publication.PublicationID, publicationActorSystem, publicationActorSystem, false)
	if err != nil || secondClaimed {
		t.Fatalf("second active claim=%v err=%v", secondClaimed, err)
	}
	entries, err := manifestEntries(publication.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	stagingRoot := filepath.Join(publication.DestinationRoot, ".mvforge-staging", publication.PublicationID, staleOwner.attemptID)
	if err := os.MkdirAll(stagingRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	entries[0].Temporary = filepath.Join(stagingRoot, "artifact-000.part")
	if err := handler.persistManifest(staleOwner, entries); err != nil {
		t.Fatal(err)
	}
	if err := handler.stagePublicationFile(staleOwner, entries[0].Source, entries[0].Temporary); err != nil {
		t.Fatal(err)
	}
	if err := recordFileOwnership(entries[0].Temporary, &entries[0]); err != nil {
		t.Fatal(err)
	}
	if err := handler.persistManifest(staleOwner, entries); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(entries[0].Final), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(entries[0].Temporary, entries[0].Final); err != nil {
		t.Fatal(err)
	}
	if err := syncDirectory(filepath.Dir(entries[0].Final)); err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&publication).Update("lease_expires_at", time.Now().UTC().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	(&AutoPublisher{db: fixture.db}).recoverPublisherPublications()
	if err := handler.assertPublicationLease(staleOwner, false); err == nil {
		t.Fatal("expired owner remained authorized after recovery reacquisition")
	}
	var recovered models.PublisherPublication
	if err := fixture.db.Where("publication_id = ?", publication.PublicationID).First(&recovered).Error; err != nil {
		t.Fatal(err)
	}
	if recovered.State != PublicationPublished || recovered.FencingGeneration != 2 || recovered.ActiveAttemptID != "" || recovered.AttemptCount != 1 {
		t.Fatalf("recovery did not commit through a new fence: %#v", recovered)
	}
	var recoveredJob models.QueueJob
	if err := fixture.db.First(&recoveredJob, fixture.job.ID).Error; err != nil {
		t.Fatal(err)
	}
	assertQueueStageSequence(t, recoveredJob, JobStagePublishing, JobStageArchivingOriginal, JobStageCompleted)
	var attempts []models.PublisherPublicationAttempt
	if err := fixture.db.Where("publication_id = ?", publication.PublicationID).Find(&attempts).Error; err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || attempts[0].AttemptID != staleOwner.attemptID {
		t.Fatalf("recovery replaced the logical attempt identity: %#v", attempts)
	}
	var reacquisition models.PublisherAttemptEvidence
	if err := fixture.db.Where("publication_id = ? AND outcome = ?", publication.PublicationID, "reacquired").First(&reacquisition).Error; err != nil {
		t.Fatal(err)
	}
	if reacquisition.AttemptID != staleOwner.attemptID || reacquisition.FencingGeneration != 2 {
		t.Fatalf("reacquisition evidence did not retain identity and advance fence: %#v", reacquisition)
	}
	got, err := os.ReadFile(fixture.destination)
	if err != nil || !bytes.Equal(got, []byte("canonical result media")) {
		t.Fatalf("recovered final media=%q err=%v", got, err)
	}
	var events int64
	if err := fixture.db.Model(&models.AssetPublishedOutbox{}).Where("publication_id = ?", publication.PublicationID).Count(&events).Error; err != nil || events != 1 {
		t.Fatalf("recovery created outbox event count=%d err=%v", events, err)
	}
}

func TestPublisherProvenanceTransactionFailureKeepsPromotedFilesRecoverable(t *testing.T) {
	fixture := newPublisherDeliveryFixture(t, true)
	handler := PublisherHandler{db: fixture.db}
	if err := fixture.db.Exec(`CREATE TRIGGER fail_publisher_publication_success BEFORE UPDATE OF state ON publisher_publications WHEN NEW.state = 'PUBLISHED' BEGIN SELECT RAISE(ABORT, 'forced publication provenance failure'); END`).Error; err != nil {
		t.Fatal("install publication transaction failure trigger:", err)
	}
	if _, err := handler.publishQueueJobWithMode(fixture.job, true, false); err == nil || !strings.Contains(err.Error(), "forced publication provenance failure") {
		t.Fatalf("Publisher did not surface the injected provenance transaction failure: %v", err)
	}
	var publication models.PublisherPublication
	if err := fixture.db.Where("queue_job_id = ?", fixture.job.ID).First(&publication).Error; err != nil {
		t.Fatal(err)
	}
	if publication.State != PublicationPublishing || publication.ActiveAttemptID == "" || publication.FencingGeneration != 1 || publication.AttemptCount != 1 {
		t.Fatalf("uncertain provenance transaction did not remain recoverable as PUBLISHING: %#v", publication)
	}
	entries, err := manifestEntries(publication.Manifest)
	if err != nil {
		t.Fatal("persisted manifest is unavailable after transaction failure:", err)
	}
	requiredReceipts := 0
	for _, entry := range entries {
		if !entry.Required || !entry.Promote {
			continue
		}
		requiredReceipts++
		if entry.Outcome != "committed" || !tokenMatches(entry.Final, entry) {
			t.Fatalf("required final is not present with its persisted receipt: %#v", entry)
		}
	}
	if requiredReceipts != 2 {
		t.Fatalf("persisted manifest has %d required receipts, want primary and sidecar", requiredReceipts)
	}
	if got, err := os.ReadFile(fixture.destination); err != nil || !bytes.Equal(got, []byte("canonical result media")) {
		t.Fatalf("promoted final media=%q err=%v", got, err)
	}
	if got, err := os.ReadFile(strings.TrimSuffix(fixture.destination, filepath.Ext(fixture.destination)) + ".eng.ass"); err != nil || !bytes.Contains(got, []byte("delivery sidecar")) {
		t.Fatalf("promoted required sidecar=%q err=%v", got, err)
	}
	if got, err := os.ReadFile(fixture.original); err != nil || !bytes.Equal(got, []byte("raw original media")) {
		t.Fatalf("original was archived before provenance commit: content=%q err=%v", got, err)
	}
	if publication.OriginalArchiveStatus != "pending" {
		t.Fatalf("archive status changed before PUBLISHED commit: %q", publication.OriginalArchiveStatus)
	}
	var jobBeforeRecovery models.QueueJob
	if err := fixture.db.First(&jobBeforeRecovery, fixture.job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if jobBeforeRecovery.PublishedAt != nil || jobBeforeRecovery.PublishedPath != "" || jobBeforeRecovery.PublicationState == PublicationPublished {
		t.Fatalf("Queue provenance was committed despite transaction failure: %#v", jobBeforeRecovery)
	}
	var eventCount int64
	if err := fixture.db.Model(&models.AssetPublishedOutbox{}).Count(&eventCount).Error; err != nil || eventCount != 0 {
		t.Fatalf("AssetPublished existed before provenance commit: count=%d err=%v", eventCount, err)
	}

	if err := fixture.db.Exec("DROP TRIGGER fail_publisher_publication_success").Error; err != nil {
		t.Fatal("remove publication transaction failure trigger:", err)
	}
	if err := fixture.db.Model(&models.PublisherPublication{}).Where("publication_id = ?", publication.PublicationID).Update("lease_expires_at", time.Now().UTC().Add(-time.Second)).Error; err != nil {
		t.Fatal("expire persisted publication lease:", err)
	}
	(&AutoPublisher{db: fixture.db}).recoverPublisherPublications()
	var recovered models.PublisherPublication
	if err := fixture.db.Where("publication_id = ?", publication.PublicationID).First(&recovered).Error; err != nil {
		t.Fatal(err)
	}
	if recovered.State != PublicationPublished || recovered.FencingGeneration != 2 || recovered.ActiveAttemptID != "" || recovered.AttemptCount != 1 {
		t.Fatalf("recovery did not commit through the next fence on the same logical attempt: %#v", recovered)
	}
	var attempts []models.PublisherPublicationAttempt
	if err := fixture.db.Where("publication_id = ?", publication.PublicationID).Find(&attempts).Error; err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || attempts[0].AttemptID != publication.ActiveAttemptID {
		t.Fatalf("recovery created a second logical attempt: %#v", attempts)
	}
	var reacquisition models.PublisherAttemptEvidence
	if err := fixture.db.Where("publication_id = ? AND phase = ? AND outcome = ?", publication.PublicationID, "claim", "reacquired").First(&reacquisition).Error; err != nil {
		t.Fatal("recovery did not persist reacquisition evidence:", err)
	}
	if reacquisition.AttemptID != publication.ActiveAttemptID || reacquisition.FencingGeneration != 2 {
		t.Fatalf("recovery changed logical attempt identity or failed to advance the fence: %#v", reacquisition)
	}
	if _, err := os.Stat(fixture.original); !os.IsNotExist(err) {
		t.Fatalf("original archive did not proceed after PUBLISHED: %v", err)
	}
	if recovered.OriginalArchiveStatus != "complete" {
		t.Fatalf("archive status after recovery=%q, want complete", recovered.OriginalArchiveStatus)
	}
	archivedOriginal, err := os.ReadFile(recovered.OriginalArchivedPath)
	if err != nil || !bytes.Equal(archivedOriginal, []byte("raw original media")) {
		t.Fatalf("archived original=%q err=%v", archivedOriginal, err)
	}
	var events []models.AssetPublishedOutbox
	if err := fixture.db.Where("publication_id = ?", publication.PublicationID).Find(&events).Error; err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventID == "" || events[0].PublicationID != publication.PublicationID {
		t.Fatalf("recovery did not create one stable outbox identity: %#v", events)
	}
	(&AutoPublisher{db: fixture.db}).recoverPublisherPublications()
	var finalEventCount int64
	if err := fixture.db.Model(&models.AssetPublishedOutbox{}).Where("publication_id = ?", publication.PublicationID).Count(&finalEventCount).Error; err != nil || finalEventCount != 1 {
		t.Fatalf("repeat recovery duplicated publication event: count=%d err=%v", finalEventCount, err)
	}
	var publicationCount int64
	if err := fixture.db.Model(&models.PublisherPublication{}).Where("queue_job_id = ?", fixture.job.ID).Count(&publicationCount).Error; err != nil || publicationCount != 1 {
		t.Fatalf("recovery created a duplicate publication: count=%d err=%v", publicationCount, err)
	}
}

func TestPublisherChangedResultBytesDoNotPersistPublication(t *testing.T) {
	fixture := newPublisherDeliveryFixture(t, false)
	if err := os.WriteFile(fixture.output, []byte("changed after Validation"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (PublisherHandler{db: fixture.db}).publishQueueJobWithMode(fixture.job, false, false); err == nil {
		t.Fatal("Publisher accepted Result bytes changed after Validation")
	}
	var count int64
	if err := fixture.db.Model(&models.PublisherPublication{}).Where("queue_job_id = ?", fixture.job.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("stale Result created %d PublisherPublication rows", count)
	}
	if _, err := os.Stat(fixture.destination); !os.IsNotExist(err) {
		t.Fatalf("stale Result reached the final destination: %v", err)
	}
	if got, err := os.ReadFile(fixture.output); err != nil || string(got) != "changed after Validation" {
		t.Fatalf("stale canonical Result was changed: content=%q err=%v", got, err)
	}
}

func TestPublisherAcceptsMetadataOnlyResultChangesUsingValidationRevision(t *testing.T) {
	fixture := newPublisherDeliveryFixture(t, false)
	artifact, _, err := readLatestJobArtifact(fixture.db, fixture.job, "result")
	if err != nil {
		t.Fatal(err)
	}
	identity, ok := resultIdentityFromArtifact(artifact)
	if !ok || !strings.HasPrefix(identity.OutputRevision, "result-output-revision-v2-") {
		t.Fatalf("fixture did not produce a v2 Result identity: %#v", artifact["resultIdentity"])
	}
	if err := os.Chmod(fixture.output, 0o640); err != nil {
		t.Fatal(err)
	}
	if allowed, reason := canonicalRunAllowsPublish(fixture.db, fixture.job, true); !allowed {
		t.Fatalf("Publisher rejected unchanged content after chmod: %s", reason)
	}
	currentEvidence, err := resultOutputRevisionForIdentity(fixture.output, identity.OutputRevision)
	if err != nil || currentEvidence.ContentRevision != identity.OutputRevision {
		t.Fatalf("Publisher did not resolve the Validation content revision: evidence=%#v err=%v", currentEvidence, err)
	}
	if _, err := (PublisherHandler{db: fixture.db}).publishQueueJobWithMode(fixture.job, true, false); err != nil {
		t.Fatalf("Publisher rejected the same chmod-only output accepted by Validation: %v", err)
	}
}

func TestPublisherMissingValidationRunDoesNotPersistPublication(t *testing.T) {
	fixture := newPublisherDeliveryFixture(t, false)
	fixture.job.ValidationRunID = ""
	if err := fixture.db.Save(&fixture.job).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := (PublisherHandler{db: fixture.db}).publishQueueJobWithMode(fixture.job, false, false); err == nil {
		t.Fatal("Publisher accepted a handoff without a canonical ValidationRun")
	}
	var count int64
	if err := fixture.db.Model(&models.PublisherPublication{}).Where("queue_job_id = ?", fixture.job.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("missing canonical ValidationRun created %d PublisherPublication rows", count)
	}
}

func TestAutoPublisherWaitsForCanonicalValidationBeforeFirstHandoff(t *testing.T) {
	fixture := newPublisherDeliveryFixture(t, false)
	var validation models.ValidationRun
	if err := fixture.db.Where("run_id = ?", fixture.job.ValidationRunID).First(&validation).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Delete(&validation).Error; err != nil {
		t.Fatal(err)
	}
	fixture.job.ValidationRunID = ""
	fixture.job.ValidationOutcome = ""
	fixture.job.ValidationStatus = ""
	fixture.job.Stage = JobStageValidating
	if err := fixture.db.Save(&fixture.job).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Create(&models.AppSetting{Key: "pipelineAutomation", Value: models.JSONMap{"autoPublisherEnabled": true}}).Error; err != nil {
		t.Fatal(err)
	}

	auto := &AutoPublisher{db: fixture.db}
	auto.tick()
	var publications int64
	if err := fixture.db.Model(&models.PublisherPublication{}).Where("queue_job_id = ?", fixture.job.ID).Count(&publications).Error; err != nil {
		t.Fatal(err)
	}
	if publications != 0 {
		t.Fatalf("completed Queue job without canonical Validation created %d publication rows", publications)
	}

	artifact, _, err := readLatestJobArtifact(fixture.db, fixture.job, "result")
	if err != nil {
		t.Fatal(err)
	}
	resultIdentity, identityOK := resultIdentityFromArtifact(artifact)
	binding, bindingOK := resultBindingFromArtifact(artifact)
	if !identityOK || !bindingOK {
		t.Fatalf("canonical Result identity unavailable: %#v", artifact)
	}
	validation.RunID = "validation-run:auto-publisher-race:" + t.Name()
	validation.JobID = fixture.job.ID
	validation.ExecutionNumber = *fixture.job.ExecutionNumber
	validation.ExecutionPlanID = binding.ExecutionPlanID
	validation.ExecutionPlanVersion = binding.ExecutionPlanVersion
	validation.ExecutionPlanFingerprint = binding.ExecutionPlanFingerprint
	validation.ResultArtifactID = resultIdentity.ArtifactID
	validation.ResultGenerationToken = resultIdentity.GenerationToken
	validation.ResultGenerationVersion = resultIdentity.GenerationVersion
	validation.ResultOutputRevision = resultIdentity.OutputRevision
	validation.Outcome = "PASS"
	validation.CreatedAt = time.Now().UTC()
	if err := fixture.db.Create(&validation).Error; err != nil {
		t.Fatal(err)
	}
	fixture.job.ValidationRunID = validation.RunID
	fixture.job.ValidationOutcome = "PASS"
	fixture.job.ValidationStatus = ValidationStatusPassed
	fixture.job.Stage = JobStageReadyToPublish
	if err := fixture.db.Save(&fixture.job).Error; err != nil {
		t.Fatal(err)
	}

	auto.tick()
	var publication models.PublisherPublication
	if err := fixture.db.Where("queue_job_id = ? AND execution_number = ?", fixture.job.ID, *fixture.job.ExecutionNumber).First(&publication).Error; err != nil {
		t.Fatal("ready Queue execution did not create its Publisher identity:", err)
	}
	if publication.ValidationRunID != fixture.job.ValidationRunID {
		t.Fatalf("publication ValidationRunID=%q, want exact Queue ValidationRunID %q", publication.ValidationRunID, fixture.job.ValidationRunID)
	}
}

func prepareBlockedValidationIdentityRace(t *testing.T, fixture publisherDeliveryFixture) (PublisherHandler, models.PublisherPublication) {
	t.Helper()
	handler := PublisherHandler{db: fixture.db}
	unbound := fixture.job
	unbound.ValidationRunID = ""
	candidate, resultIdentity, err := publisherIdentityForJob(fixture.db, unbound, false)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := handler.persistPublicationIdentity(candidate, resultIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&models.PublisherPublication{}).Where("publication_id = ?", publication.PublicationID).Updates(map[string]any{
		"state": PublicationBlocked, "failure_code": "identity_changed", "failure_summary": "Queue Validation identity changed before an attempt.",
		"attempt_count": 0, "fencing_generation": 0, "original_archive_status": "pending",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&models.QueueJob{}).Where("id = ?", fixture.job.ID).Updates(map[string]any{
		"publication_state": PublicationBlocked, "publication_summary": "Queue Validation identity changed before an attempt.",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Where("publication_id = ?", publication.PublicationID).First(&publication).Error; err != nil {
		t.Fatal(err)
	}
	return handler, publication
}

func TestAutoPublisherRecoversOnlyUnattemptedValidationIdentityRace(t *testing.T) {
	fixture := newPublisherDeliveryFixture(t, false)
	_, stored := prepareBlockedValidationIdentityRace(t, fixture)
	if stored.ValidationRunID != "" || stored.State != PublicationBlocked || stored.FailureCode != "identity_changed" || stored.AttemptCount != 0 || stored.FencingGeneration != 0 || stored.OriginalArchiveStatus != "pending" {
		t.Fatalf("fixture does not match the zero-attempt race state: %#v", stored)
	}
	if err := database.Migrate(fixture.db); err != nil {
		t.Fatalf("install production Publisher identity trigger: %v", err)
	}
	if err := fixture.db.Create(&models.AppSetting{Key: "pipelineAutomation", Value: models.JSONMap{"autoPublisherEnabled": true}}).Error; err != nil {
		t.Fatal(err)
	}
	(&AutoPublisher{db: fixture.db}).tick()

	var recovered models.PublisherPublication
	if err := fixture.db.Where("publication_id = ?", stored.PublicationID).First(&recovered).Error; err != nil {
		t.Fatal(err)
	}
	if recovered.State != PublicationPublished || recovered.ValidationRunID != fixture.job.ValidationRunID || recovered.AttemptCount != 1 || recovered.FencingGeneration != 1 {
		t.Fatalf("bounded Validation identity recovery did not proceed through a normal Publisher attempt: %#v", recovered)
	}
	var audit models.PublisherAuditEvent
	if err := fixture.db.Where("publication_id = ? AND action = ?", stored.PublicationID, "recover_validation_identity").First(&audit).Error; err != nil {
		t.Fatal("Validation identity recovery was not audited:", err)
	}
	if fmt.Sprint(audit.Parameters["recovery"]) != "zero_attempt_validation_handoff_race" || fmt.Sprint(audit.Parameters["validationRunId"]) != fixture.job.ValidationRunID {
		t.Fatalf("recovery audit does not identify the bounded repair: %#v", audit.Parameters)
	}
}

func TestPublisherValidationIdentityRecoveryRejectsChangedOrMutatedState(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*testing.T, publisherDeliveryFixture, models.PublisherPublication)
	}{
		{name: "execution", mutate: func(t *testing.T, fixture publisherDeliveryFixture, publication models.PublisherPublication) {
			if err := fixture.db.Model(&models.PublisherPublication{}).Where("publication_id = ?", publication.PublicationID).Update("execution_number", publication.ExecutionNumber+1).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "plan", mutate: func(t *testing.T, fixture publisherDeliveryFixture, publication models.PublisherPublication) {
			if err := fixture.db.Model(&models.PublisherPublication{}).Where("publication_id = ?", publication.PublicationID).Update("execution_plan_fingerprint", publication.ExecutionPlanFingerprint+"-changed").Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "result", mutate: func(t *testing.T, fixture publisherDeliveryFixture, publication models.PublisherPublication) {
			if err := fixture.db.Model(&models.PublisherPublication{}).Where("publication_id = ?", publication.PublicationID).Update("result_artifact_id", publication.ResultArtifactID+"-changed").Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "destination", mutate: func(t *testing.T, fixture publisherDeliveryFixture, publication models.PublisherPublication) {
			if err := fixture.db.Model(&models.PublisherPublication{}).Where("publication_id = ?", publication.PublicationID).Update("destination_path", filepath.Join(fixture.libraryRoot, "changed.mkv")).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "source identity", mutate: func(t *testing.T, fixture publisherDeliveryFixture, publication models.PublisherPublication) {
			changedID := uint(999)
			if err := fixture.db.Model(&models.PublisherPublication{}).Where("publication_id = ?", publication.PublicationID).Update("source_asset_record_id", changedID).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "non-empty different ValidationRunID", mutate: func(t *testing.T, fixture publisherDeliveryFixture, publication models.PublisherPublication) {
			if err := fixture.db.Model(&models.PublisherPublication{}).Where("publication_id = ?", publication.PublicationID).Update("validation_run_id", "validation-run:different").Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "attempt count", mutate: func(t *testing.T, fixture publisherDeliveryFixture, publication models.PublisherPublication) {
			if err := fixture.db.Model(&models.PublisherPublication{}).Where("publication_id = ?", publication.PublicationID).Update("attempt_count", 1).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "attempt row", mutate: func(t *testing.T, fixture publisherDeliveryFixture, publication models.PublisherPublication) {
			attempt := models.PublisherPublicationAttempt{AttemptID: "attempt:recovery-negative", PublicationID: publication.PublicationID, FencingGeneration: 1, ActorSource: publicationActorSystem, ActorID: publicationActorSystem, CreatedAt: time.Now().UTC()}
			if err := fixture.db.Create(&attempt).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "fencing generation", mutate: func(t *testing.T, fixture publisherDeliveryFixture, publication models.PublisherPublication) {
			if err := fixture.db.Model(&models.PublisherPublication{}).Where("publication_id = ?", publication.PublicationID).Update("fencing_generation", 1).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "attempt evidence", mutate: func(t *testing.T, fixture publisherDeliveryFixture, publication models.PublisherPublication) {
			evidence := models.PublisherAttemptEvidence{EvidenceID: "attempt-evidence:recovery-negative", PublicationID: publication.PublicationID, AttemptID: "attempt:recovery-negative", FencingGeneration: 0, Version: 1, Phase: "claim", Outcome: "acquired", CreatedAt: time.Now().UTC()}
			if err := fixture.db.Create(&evidence).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "archive started", mutate: func(t *testing.T, fixture publisherDeliveryFixture, publication models.PublisherPublication) {
			if err := fixture.db.Model(&models.PublisherPublication{}).Where("publication_id = ?", publication.PublicationID).Update("original_archive_status", "archiving").Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "filesystem mutation", mutate: func(t *testing.T, fixture publisherDeliveryFixture, publication models.PublisherPublication) {
			mutated := models.JSONList{models.JSONMap{"kind": "primary", "source": fixture.output, "final": fixture.destination, "required": true, "promote": true, "temporary": filepath.Join(fixture.root, "staged", "artifact.part")}}
			if err := fixture.db.Model(&models.PublisherPublication{}).Where("publication_id = ?", publication.PublicationID).Update("manifest", mutated).Error; err != nil {
				t.Fatal(err)
			}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newPublisherDeliveryFixture(t, false)
			handler, publication := prepareBlockedValidationIdentityRace(t, fixture)
			candidate, _, err := publisherIdentityForJob(fixture.db, fixture.job, false)
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(t, fixture, publication)
			if err := fixture.db.Where("publication_id = ?", publication.PublicationID).First(&publication).Error; err != nil {
				t.Fatal(err)
			}
			expectedValidationRunID := publication.ValidationRunID
			_, recovered, err := handler.recoverUnattemptedValidationIdentity(publication, candidate)
			if err != nil {
				t.Fatal(err)
			}
			if recovered {
				t.Fatal("recovery accepted a changed or mutated publication identity")
			}
			if err := fixture.db.Where("publication_id = ?", publication.PublicationID).First(&publication).Error; err != nil {
				t.Fatal(err)
			}
			if publication.ValidationRunID != expectedValidationRunID || publication.State != PublicationBlocked {
				t.Fatalf("rejected recovery changed the stored identity/state: %#v", publication)
			}
		})
	}
}

func TestPublisherIdentityBindsEligibleValidationOverrideEvent(t *testing.T) {
	fixture := newPublisherDeliveryFixture(t, false)
	artifact, _, err := readLatestJobArtifact(fixture.db, fixture.job, "result")
	if err != nil {
		t.Fatal(err)
	}
	identity, identityOK := resultIdentityFromArtifact(artifact)
	binding, bindingOK := resultBindingFromArtifact(artifact)
	if !identityOK || !bindingOK {
		t.Fatalf("canonical Result identity unavailable: %#v", artifact)
	}
	job := fixture.job
	job.ValidationRunID = "validation-run:publisher-override:" + t.Name()
	run := models.ValidationRun{
		RunID: job.ValidationRunID, JobID: job.ID, ExecutionNumber: *job.ExecutionNumber,
		ExecutionPlanID: binding.ExecutionPlanID, ExecutionPlanVersion: binding.ExecutionPlanVersion,
		ExecutionPlanFingerprint: binding.ExecutionPlanFingerprint, ResultArtifactID: identity.ArtifactID,
		ResultGenerationToken: identity.GenerationToken, ResultGenerationVersion: identity.GenerationVersion,
		ResultOutputRevision: identity.OutputRevision, SchemaVersion: validationSchemaVersion,
		ValidatorVersion: validationEngineVersion, RulesetID: validationRulesetID, RulesetVersion: validationRulesetVersion,
		PolicySchemaVersion: 1, PolicyVersion: "validation-policy-v1", PolicyPreset: "custom", PolicyFingerprint: "sha256:test-policy",
		PolicySnapshot: models.JSONMap{"schemaVersion": 1, "policyVersion": "validation-policy-v1", "preset": "custom"},
		ActorSource:    publicationActorSystem, ActorID: publicationActorSystem, Outcome: "FAIL",
		Findings: models.JSONList{
			models.JSONMap{"key": "duration.timeline", "disposition": "blocking", "state": "FAIL", "overrideable": true},
			models.JSONMap{"key": "video.properties", "disposition": "blocking", "state": "UNVERIFIED", "overrideable": true},
		},
		Report: models.JSONMap{}, CreatedAt: time.Now().UTC(),
	}
	if err := fixture.db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	override := models.ValidationOverride{
		EventID: "validation-override:publisher-54", ValidationRunID: run.RunID,
		Scope: "all_overrideable", ResultArtifactID: run.ResultArtifactID, ResultGenerationToken: run.ResultGenerationToken,
		ResultGenerationVersion: run.ResultGenerationVersion, ResultOutputRevision: run.ResultOutputRevision,
		PolicySchemaVersion: run.PolicySchemaVersion, PolicyVersion: run.PolicyVersion, PolicyFingerprint: run.PolicyFingerprint,
		ActorSource: publicationActorOperator, ActorID: publicationActorOperator,
		Reason: "Reviewed exception", RuleKeys: models.JSONList{"duration.timeline", "video.properties"},
		RequestedHandoff: "publisher", OverrideVersion: "v1", CreatedAt: time.Now().UTC(),
	}
	if err := fixture.db.Create(&override).Error; err != nil {
		t.Fatal(err)
	}
	job.ValidationOutcome, job.ValidationStatus = "FAIL", ValidationStatusFailed
	if err := fixture.db.Save(&job).Error; err != nil {
		t.Fatal(err)
	}
	result, err := (PublisherHandler{db: fixture.db}).handoffAndPublishWithReason(job, false, "Reviewed the eligible Validation overrides")
	if err != nil || result.Status != "published" {
		t.Fatalf("eligible #53 override did not authorize Publisher handoff: result=%#v err=%v", result, err)
	}
	var publication models.PublisherPublication
	if err := fixture.db.Where("queue_job_id = ?", job.ID).First(&publication).Error; err != nil {
		t.Fatal(err)
	}
	if len(publication.ValidationOverrideEventIDs) != 1 || fmt.Sprint(publication.ValidationOverrideEventIDs[0]) != override.EventID {
		t.Fatalf("publication identity did not bind the eligible override event: %#v", publication.ValidationOverrideEventIDs)
	}
	var event models.AssetPublishedOutbox
	if err := fixture.db.Where("publication_id = ?", publication.PublicationID).First(&event).Error; err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(event.Payload["validationOverrideEventIds"]) != "[validation-override:publisher-54]" {
		t.Fatalf("AssetPublished omitted canonical override provenance: %#v", event.Payload)
	}
}

func TestPublisherRejectsNonOverrideableAndStaleOverrides(t *testing.T) {
	tests := []struct {
		name          string
		findingKey    string
		findingState  string
		wrongRun      bool
		wrongResult   bool
		wrongHandoff  bool
		replaceResult bool
		legacyMinimum bool
		noOverride    bool
	}{
		{name: "crafted mandatory identity skip", findingKey: "artifact.integrity", findingState: "FAIL"},
		{name: "wrong ValidationRun", findingKey: "duration.timeline", findingState: "FAIL", wrongRun: true},
		{name: "wrong Result identity", findingKey: "duration.timeline", findingState: "FAIL", wrongResult: true},
		{name: "wrong handoff target", findingKey: "duration.timeline", findingState: "FAIL", wrongHandoff: true},
		{name: "new Result invalidates existing skip", findingKey: "duration.timeline", findingState: "FAIL", replaceResult: true},
		{name: "legacy minimumScore cannot authorize failure", findingKey: "duration.timeline", findingState: "FAIL", legacyMinimum: true, noOverride: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newPublisherDeliveryFixture(t, false)
			artifact, _, err := readLatestJobArtifact(fixture.db, fixture.job, "result")
			if err != nil {
				t.Fatal(err)
			}
			identity, identityOK := resultIdentityFromArtifact(artifact)
			binding, bindingOK := resultBindingFromArtifact(artifact)
			if !identityOK || !bindingOK {
				t.Fatalf("canonical Result identity unavailable: %#v", artifact)
			}
			run := models.ValidationRun{
				RunID: "validation-run:publisher-reject:" + strings.ReplaceAll(test.name, " ", "-"), JobID: fixture.job.ID,
				ExecutionNumber: *fixture.job.ExecutionNumber, ExecutionPlanID: binding.ExecutionPlanID,
				ExecutionPlanVersion: binding.ExecutionPlanVersion, ExecutionPlanFingerprint: binding.ExecutionPlanFingerprint,
				ResultArtifactID: identity.ArtifactID, ResultGenerationToken: identity.GenerationToken,
				ResultGenerationVersion: identity.GenerationVersion, ResultOutputRevision: identity.OutputRevision,
				SchemaVersion: validationSchemaVersion, ValidatorVersion: validationEngineVersion,
				RulesetID: validationRulesetID, RulesetVersion: validationRulesetVersion,
				PolicySchemaVersion: 1, PolicyVersion: "validation-policy-v1", PolicyPreset: "custom", PolicyFingerprint: "sha256:publisher-reject",
				PolicySnapshot: models.JSONMap{"schemaVersion": 1, "policyVersion": "validation-policy-v1", "preset": "custom"},
				ActorSource:    "deployment", ActorID: "local_operator", Outcome: "FAIL", Score: 100,
				Findings: models.JSONList{models.JSONMap{"key": test.findingKey, "disposition": "blocking", "state": test.findingState, "overrideable": true}},
				Report:   models.JSONMap{}, CreatedAt: time.Now().UTC(),
			}
			if err := fixture.db.Create(&run).Error; err != nil {
				t.Fatal(err)
			}
			if test.legacyMinimum {
				if err := fixture.db.Create(&models.AppSetting{Key: "validation", Value: models.JSONMap{"minimumScore": 0}}).Error; err != nil {
					t.Fatal(err)
				}
			}
			event := models.ValidationOverride{
				EventID:         "validation-override:publisher-reject:" + strings.ReplaceAll(test.name, " ", "-"),
				ValidationRunID: run.RunID, Scope: "rule", ResultArtifactID: run.ResultArtifactID,
				ResultGenerationToken: run.ResultGenerationToken, ResultGenerationVersion: run.ResultGenerationVersion,
				ResultOutputRevision: run.ResultOutputRevision, PolicySchemaVersion: run.PolicySchemaVersion,
				PolicyVersion: run.PolicyVersion, PolicyFingerprint: run.PolicyFingerprint,
				ActorSource: "deployment", ActorID: "local_operator", RuleKeys: models.JSONList{test.findingKey},
				RequestedHandoff: "publisher", OverrideVersion: validationOverrideV1, CreatedAt: time.Now().UTC(),
			}
			if test.wrongRun {
				event.ValidationRunID = "validation-run:wrong"
			}
			if test.wrongResult {
				event.ResultOutputRevision = "result:wrong-revision"
			}
			if test.wrongHandoff {
				event.RequestedHandoff = "automatic"
			}
			if !test.noOverride {
				if err := fixture.db.Create(&event).Error; err != nil {
					t.Fatal(err)
				}
			}
			job := fixture.job
			job.ValidationRunID = run.RunID
			if err := fixture.db.Save(&job).Error; err != nil {
				t.Fatal(err)
			}
			if test.replaceResult {
				if err := writeJobResultArtifact(fixture.db, job, map[string]any{"status": "completed", "replacement": true}); err != nil {
					t.Fatal(err)
				}
			}
			if allowed, reason := canonicalRunAllowsPublish(fixture.db, job, false); allowed || reason == "" {
				t.Fatalf("unsafe/stale override authorized Publisher: allowed=%t reason=%q", allowed, reason)
			}
		})
	}
}

func TestPublisherOriginalArchiveClaimFencesExpiredOwner(t *testing.T) {
	fixture := newPublisherDeliveryFixture(t, false)
	candidate, identity, err := publisherIdentityForJob(fixture.db, fixture.job, false)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := (PublisherHandler{db: fixture.db}).persistPublicationIdentity(candidate, identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&publication).Updates(map[string]any{"state": PublicationPublished, "original_archive_status": "pending"}).Error; err != nil {
		t.Fatal(err)
	}
	publishedAt := time.Now().UTC()
	plannedDestination := filepath.Join(fixture.archiveRoot, "planned-original.mkv")
	if err := fixture.db.Model(&models.QueueJob{}).Where("id = ?", fixture.job.ID).Updates(map[string]any{
		"published_at": publishedAt, "published_path": fixture.destination,
		"publication_id": publication.PublicationID, "publication_state": PublicationPublished,
		"original_archive_status": "pending", "original_archived_path": plannedDestination, "stage": JobStagePublishing,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.First(&publication, publication.ID).Error; err != nil {
		t.Fatal(err)
	}
	first, acquired, err := claimOriginalArchive(fixture.db, publication)
	if err != nil || !acquired {
		t.Fatalf("first archive claim acquired=%v err=%v", acquired, err)
	}
	var activeJob models.QueueJob
	if err := fixture.db.First(&activeJob, fixture.job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if activeJob.OriginalArchiveStatus != "archiving" || activeJob.Stage != JobStageArchivingOriginal || activeJob.OriginalArchivedPath != "" {
		t.Fatalf("archive claim was not projected as an active, uncommitted archive: %#v", activeJob)
	}
	assertQueueStageSequence(t, activeJob, JobStageArchivingOriginal)
	if open, err := NewQueueHandler(fixture.db).assetHasOpenJob(fixture.original, 0); err != nil || !open {
		t.Fatalf("slow archive was not protected as active work: open=%t err=%v", open, err)
	}
	if _, acquired, err := claimOriginalArchive(fixture.db, publication); err != nil || acquired {
		t.Fatalf("concurrent archive claim acquired=%v err=%v", acquired, err)
	}
	if err := fixture.db.Model(&models.PublisherPublication{}).Where("publication_id = ?", first.PublicationID).Update("original_archive_lease_expires_at", time.Now().UTC().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.First(&publication, publication.ID).Error; err != nil {
		t.Fatal(err)
	}
	second, acquired, err := claimOriginalArchive(fixture.db, publication)
	if err != nil || !acquired || second.OriginalArchiveFencingGeneration != first.OriginalArchiveFencingGeneration+1 {
		t.Fatalf("expired archive claim reacquired=%v row=%#v err=%v", acquired, second, err)
	}
	if err := assertOriginalArchiveLease(fixture.db, first, false); err == nil {
		t.Fatal("expired archive claim owner remained authorized after reacquisition")
	}
	if err := assertOriginalArchiveLease(fixture.db, second, false); err != nil {
		t.Fatalf("reacquired archive owner was not authorized: %v", err)
	}
	if err := fixture.db.Model(&models.PublisherPublication{}).Where("publication_id = ?", second.PublicationID).Update("original_archived_path", plannedDestination).Error; err != nil {
		t.Fatal(err)
	}
	(PublisherHandler{db: fixture.db}).recordArchiveOutcome(fixture.job, second, "failed", plannedDestination, fixture.original, "test archive failure")
	if err := fixture.db.First(&activeJob, fixture.job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if activeJob.OriginalArchiveStatus != "failed" || activeJob.Stage != JobStageCompleted || activeJob.OriginalArchivedPath != "" || !strings.Contains(activeJob.PublicationSummary, "test archive failure") {
		t.Fatalf("failed archive outcome was not terminal without exposing its planned path: %#v", activeJob)
	}
	assertQueueStageSequence(t, activeJob, JobStageArchivingOriginal, JobStageCompleted)
	if open, err := NewQueueHandler(fixture.db).assetHasOpenJob(fixture.original, 0); err != nil || open {
		t.Fatalf("terminal failed archive remained active: open=%t err=%v", open, err)
	}
}

func TestPublisherNotApplicableArchiveCompletesLifecycle(t *testing.T) {
	fixture := newPublisherDeliveryFixture(t, false)
	candidate, identity, err := publisherIdentityForJob(fixture.db, fixture.job, false)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := (PublisherHandler{db: fixture.db}).persistPublicationIdentity(candidate, identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&publication).Updates(map[string]any{"state": PublicationPublished, "original_archive_status": "pending"}).Error; err != nil {
		t.Fatal(err)
	}
	publishedAt := time.Now().UTC()
	if err := fixture.db.Model(&models.QueueJob{}).Where("id = ?", fixture.job.ID).Updates(map[string]any{
		"published_at": publishedAt, "published_path": fixture.destination,
		"publication_id": publication.PublicationID, "publication_state": PublicationPublished,
		"original_archive_status": "pending", "stage": JobStagePublishing,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.First(&publication, publication.ID).Error; err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := claimOriginalArchive(fixture.db, publication)
	if err != nil || !ok {
		t.Fatalf("archive claim acquired=%v err=%v", ok, err)
	}
	(PublisherHandler{db: fixture.db}).recordArchiveOutcome(fixture.job, claimed, "not_applicable", "", "", "No original source was associated.")
	var job models.QueueJob
	if err := fixture.db.First(&job, fixture.job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if job.Status != JobStatusCompleted || job.Stage != JobStageCompleted || job.OriginalArchiveStatus != "not_applicable" || job.OriginalArchivedPath != "" {
		t.Fatalf("not-applicable archive did not finish with an independent state: %#v", job)
	}
	assertQueueStageSequence(t, job, JobStageArchivingOriginal, JobStageCompleted)
	if open, err := NewQueueHandler(fixture.db).assetHasOpenJob(fixture.original, 0); err != nil || open {
		t.Fatalf("not-applicable archive remained active: open=%t err=%v", open, err)
	}
}

func TestPublisherActionIgnoresClientActorIdentity(t *testing.T) {
	fixture := newPublisherDeliveryFixture(t, false)
	handler := PublisherHandler{db: fixture.db}
	candidate, identity, err := publisherIdentityForJob(fixture.db, fixture.job, false)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := handler.persistPublicationIdentity(candidate, identity)
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/publisher/publications/:publicationId/actions", handler.PublisherAction)
	request := httptest.NewRequest(http.MethodPost, "/api/publisher/publications/"+publication.PublicationID+"/actions", bytes.NewBufferString(`{"action":"hold","reason":"operator review","actorId":"admin","actorSource":"authenticated"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("publisher action status=%d body=%s", response.Code, response.Body.String())
	}
	var audit models.PublisherAuditEvent
	if err := fixture.db.Where("publication_id = ?", publication.PublicationID).Order("version desc").First(&audit).Error; err != nil {
		t.Fatal(err)
	}
	if audit.ActorSource != publicationActorOperator || audit.ActorID != publicationActorOperator || audit.Action != "hold" {
		t.Fatalf("client actor identity was trusted: %#v", audit)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/publisher/publications/"+publication.PublicationID+"/actions", bytes.NewBufferString(`{"action":"release","reason":"review complete","actorId":"admin","actorSource":"authenticated"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("publisher release status=%d body=%s", response.Code, response.Body.String())
	}
	var actions []models.PublisherAuditEvent
	if err := fixture.db.Where("publication_id = ?", publication.PublicationID).Order("version asc").Find(&actions).Error; err != nil {
		t.Fatal(err)
	}
	if len(actions) != 2 || actions[0].Version != 1 || actions[0].Action != "hold" || actions[1].Version != 2 || actions[1].Action != "release" || actions[1].ActorSource != publicationActorOperator || actions[1].ActorID != publicationActorOperator {
		t.Fatalf("hold/release audit evidence was not append-only and server assigned: %#v", actions)
	}
}
