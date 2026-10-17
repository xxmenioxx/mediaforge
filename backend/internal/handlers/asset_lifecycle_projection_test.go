package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func lifecycleTestTime(offset time.Duration) *time.Time {
	value := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC).Add(offset)
	return &value
}

func lifecycleTestLibrary() models.Library {
	return models.Library{ID: 1, Name: "Anime", DestinationPath: "/media/library/anime", SourcePath: "/media/raw/anime"}
}

func lifecycleTestLibraryRecord(id uint, path string) models.AssetRecord {
	return models.AssetRecord{
		ID: id, Path: path, RootPath: "/media/library/anime", RelativePath: "Show/S01/E01.mkv",
		FileName: "E01.mkv", Status: "converted", LibraryID: 1, LibraryName: "Anime",
	}
}

func lifecycleTestRawRecord(id uint, path string) models.AssetRecord {
	return models.AssetRecord{
		ID: id, Path: path, RootPath: "/media/raw/anime", RelativePath: "Show/S01/E01.mkv",
		FileName: "E01.mkv", Status: "unprocessed", SourceGroupID: 7,
	}
}

func lifecycleQueueJob(id uint, libraryPath string, libraryID uint, publishedAt *time.Time) models.QueueJob {
	return models.QueueJob{
		ID: id, MediaPath: "/media/raw/anime/Show/S01/E01.mkv", PublishedPath: libraryPath,
		LibraryID: libraryID, Status: JobStatusCompleted, Stage: JobStageCompleted,
		PublishedAt: publishedAt, ValidationStatus: ValidationStatusPassed,
	}
}

func lifecycleDirectPublication(id uint, libraryPath string, libraryID uint, publishedAt time.Time) models.DirectPublication {
	return models.DirectPublication{
		ID: id, SourcePath: "/media/raw/anime/Show/S01/E01.mkv", PublishedPath: libraryPath,
		LibraryID: libraryID, PublishedAt: publishedAt,
	}
}

func resolveLifecycleTestProjection(record models.AssetRecord, records []models.AssetRecord, jobs []models.QueueJob, publications []models.DirectPublication) AssetLifecycleProjection {
	return ResolveAssetLifecycleProjection(AssetLifecycleProjectionInput{
		Record: record, Records: records, Libraries: []models.Library{lifecycleTestLibrary()},
		QueueJobs: jobs, DirectPublications: publications,
	})
}

func hasLifecycleAttention(projection AssetLifecycleProjection, expected string) bool {
	for _, value := range projection.Attention {
		if value == expected {
			return true
		}
	}
	return false
}

func TestResolveAssetLifecycleProjectionPublicationAuthorities(t *testing.T) {
	record := lifecycleTestLibraryRecord(20, "/media/library/anime/Show/S01/E01.mkv")
	older := lifecycleTestTime(-time.Hour)
	newer := lifecycleTestTime(time.Hour)
	returnedAt := lifecycleTestTime(2 * time.Hour)

	tests := []struct {
		name           string
		jobs           []models.QueueJob
		publications   []models.DirectPublication
		wantAuthority  string
		wantProvenance string
		wantState      string
		wantConflict   bool
	}{
		{
			name:           "QueueJob only",
			jobs:           []models.QueueJob{lifecycleQueueJob(10, record.Path, record.LibraryID, newer)},
			wantAuthority:  assetPublicationAuthorityQueue,
			wantProvenance: AssetProvenanceConverted,
			wantState:      assetCurrentPublicationPublished,
		},
		{
			name: "DirectPublication only",
			publications: []models.DirectPublication{
				lifecycleDirectPublication(11, record.Path, record.LibraryID, *newer),
			},
			wantAuthority:  assetPublicationAuthorityDirect,
			wantProvenance: AssetProvenancePublishedAsIs,
			wantState:      assetCurrentPublicationPublished,
		},
		{
			name: "retired QueueJob plus active DirectPublication",
			jobs: []models.QueueJob{{ID: 12, MediaPath: "/media/raw/anime/Show/S01/E01.mkv", PublishedPath: record.Path, LibraryID: 1, Status: JobStatusCompleted, PublishedAt: newer, PublicationRetiredAt: older}},
			publications: []models.DirectPublication{
				lifecycleDirectPublication(13, record.Path, record.LibraryID, *newer),
			},
			wantAuthority:  assetPublicationAuthorityDirect,
			wantProvenance: AssetProvenancePublishedAsIs,
			wantState:      assetCurrentPublicationPublished,
		},
		{
			name: "active QueueJob plus returned DirectPublication",
			jobs: []models.QueueJob{lifecycleQueueJob(14, record.Path, record.LibraryID, newer)},
			publications: []models.DirectPublication{{
				ID: 15, SourcePath: "/media/raw/anime/Show/S01/E01.mkv", PublishedPath: record.Path,
				LibraryID: 1, PublishedAt: *older, ReturnedAt: returnedAt,
			}},
			wantAuthority:  assetPublicationAuthorityQueue,
			wantProvenance: AssetProvenanceConverted,
			wantState:      assetCurrentPublicationPublished,
		},
		{
			name: "active QueueJob plus active DirectPublication conflict",
			jobs: []models.QueueJob{lifecycleQueueJob(16, record.Path, record.LibraryID, newer)},
			publications: []models.DirectPublication{
				lifecycleDirectPublication(17, record.Path, record.LibraryID, *older),
			},
			wantProvenance: AssetProvenanceLegacyOrUnknown,
			wantState:      assetCurrentPublicationConflict,
			wantConflict:   true,
		},
		{
			name: "historical revisions plus one current",
			jobs: []models.QueueJob{
				{ID: 18, MediaPath: "/media/raw/anime/Show/S01/E01.mkv", PublishedPath: record.Path, LibraryID: 1, Status: JobStatusCompleted, PublishedAt: older, PublicationRetiredAt: older},
				lifecycleQueueJob(19, record.Path, record.LibraryID, newer),
			},
			wantAuthority:  assetPublicationAuthorityQueue,
			wantProvenance: AssetProvenanceConverted,
			wantState:      assetCurrentPublicationPublished,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projection := resolveLifecycleTestProjection(record, []models.AssetRecord{record}, test.jobs, test.publications)
			if projection.CurrentPublication.State != test.wantState {
				t.Fatalf("current state=%q want %q: %#v", projection.CurrentPublication.State, test.wantState, projection)
			}
			if projection.Provenance != test.wantProvenance {
				t.Fatalf("provenance=%q want %q", projection.Provenance, test.wantProvenance)
			}
			if test.wantAuthority != "" && projection.CurrentPublication.Authority != test.wantAuthority {
				t.Fatalf("authority=%q want %q", projection.CurrentPublication.Authority, test.wantAuthority)
			}
			if hasLifecycleAttention(projection, "publication_conflict") != test.wantConflict {
				t.Fatalf("conflict attention=%v want %v", hasLifecycleAttention(projection, "publication_conflict"), test.wantConflict)
			}
			if test.wantConflict && projection.Actions.DeleteOriginal.Allowed {
				t.Fatal("conflicting publication authorities must fail closed for original deletion")
			}
		})
	}
}

func TestResolveAssetAnalysisEvidenceStatesAndIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.mkv")
	if err := os.WriteFile(path, []byte("fixture source"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := completeAnalysisSnapshot(path, info)
	snapshot.ID = 17
	snapshot.UpdatedAt = time.Date(2026, 9, 25, 12, 0, 0, 123456789, time.UTC)
	stampSnapshotCacheMetadata(&snapshot, path, info)
	record := models.AssetRecord{Path: path, SizeBytes: info.Size(), ModifiedAt: info.ModTime()}
	legacy := snapshot
	legacy.RawProbe = models.JSONMap{}
	partial := completeAnalysisSnapshot(path, info)
	partial.ID = snapshot.ID + 1
	partial.UpdatedAt = snapshot.UpdatedAt.Add(time.Second)
	stampSnapshotCacheMetadata(&partial, path, info)
	cache := partial.RawProbe["snapshotCache"].(models.JSONMap)
	components := models.JSONMap{}
	for key, value := range cache["components"].(models.JSONMap) {
		components[key] = value
	}
	cache["components"] = components
	components["cadence"] = 0

	tests := []struct {
		name   string
		record models.AssetRecord
		scans  []models.ScanResult
		policy AnalysisPolicy
		want   string
	}{
		{name: "missing", record: record, want: assetAnalysisMissing},
		{name: "ready", record: record, scans: []models.ScanResult{snapshot}, want: assetAnalysisReady},
		{name: "stale source revision", record: models.AssetRecord{Path: path, SizeBytes: info.Size() + 1, ModifiedAt: info.ModTime()}, scans: []models.ScanResult{snapshot}, want: assetAnalysisStale},
		{name: "stale scanner policy", record: record, scans: []models.ScanResult{snapshot}, policy: analysisPolicyPreset("fast"), want: assetAnalysisStale},
		{name: "stale component evidence", record: record, scans: []models.ScanResult{partial}, want: assetAnalysisStale},
		{name: "legacy identity unavailable", record: record, scans: []models.ScanResult{legacy}, want: assetAnalysisUnavailable},
		{name: "missing asset unavailable", record: models.AssetRecord{Path: path, Missing: true}, scans: []models.ScanResult{snapshot}, want: assetAnalysisUnavailable},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			evidence := resolveAssetAnalysisEvidence(testCase.record, testCase.scans, testCase.policy, 20)
			if evidence.Status != testCase.want {
				t.Fatalf("status=%q want %q evidence=%#v", evidence.Status, testCase.want, evidence)
			}
			if testCase.name == "ready" && (evidence.SourceIdentity == nil || !evidence.SourceIdentity.MatchesCurrentSourceRevision || evidence.SourceIdentity.SnapshotID != snapshot.ID) {
				t.Fatalf("ready evidence lacks matching source identity: %#v", evidence)
			}
			if testCase.name == "stale source revision" && (evidence.SourceIdentity == nil || evidence.SourceIdentity.MatchesCurrentSourceRevision || !reflect.DeepEqual(evidence.StaleComponents, []string{"source"})) {
				t.Fatalf("stale source evidence was not fenced: %#v", evidence)
			}
			if testCase.name == "legacy identity unavailable" && evidence.UnavailableReason != "source_identity_unavailable" {
				t.Fatalf("legacy evidence was given an unsafe identity: %#v", evidence)
			}
			if testCase.name == "stale component evidence" && !reflect.DeepEqual(evidence.StaleComponents, []string{"cadence"}) {
				t.Fatalf("partial component provenance was not retained: %#v", evidence)
			}
		})
	}
}

func TestResolveAssetLifecycleProjectionSeparatesSourceAndCurrentCorrelation(t *testing.T) {
	raw := lifecycleTestRawRecord(10, "/media/raw/anime/Show/S01/E01.mkv")
	library := lifecycleTestLibraryRecord(20, "/media/library/anime/Show/S01/E01.mkv")
	publishedAt := lifecycleTestTime(time.Hour)
	job := lifecycleQueueJob(30, library.Path, library.LibraryID, publishedAt)
	job.SourceAssetRecordID = &raw.ID

	resolved := resolveLifecycleTestProjection(library, []models.AssetRecord{raw, library}, []models.QueueJob{job}, nil)
	if resolved.CurrentRawCorrelation.State != assetCorrelationResolved || resolved.CurrentRawCorrelation.RawAssetRecordID != raw.ID {
		t.Fatalf("normal correlation=%#v", resolved.CurrentRawCorrelation)
	}
	rawProjection := resolveLifecycleTestProjection(raw, []models.AssetRecord{raw, library}, []models.QueueJob{job}, nil)
	if rawProjection.CurrentPublication.State != assetCurrentPublicationPublished || rawProjection.CurrentPublication.LibraryAssetID != library.ID {
		t.Fatalf("raw current publication=%#v", rawProjection.CurrentPublication)
	}
	if len(resolved.SourceProvenance) != 1 || resolved.SourceProvenance[0].SourcePath != raw.Path {
		t.Fatalf("source provenance=%#v", resolved.SourceProvenance)
	}

	pathReused := lifecycleTestRawRecord(11, raw.Path)
	legacyJob := lifecycleQueueJob(31, library.Path, library.LibraryID, publishedAt)
	legacy := resolveLifecycleTestProjection(library, []models.AssetRecord{pathReused, library}, []models.QueueJob{legacyJob}, nil)
	if legacy.CurrentRawCorrelation.State != assetCorrelationUnresolved {
		t.Fatalf("path-only history fabricated correlation=%#v", legacy.CurrentRawCorrelation)
	}
	if len(legacy.SourceProvenance) != 1 || legacy.SourceProvenance[0].SourcePath != raw.Path {
		t.Fatalf("historical source provenance was lost=%#v", legacy.SourceProvenance)
	}

	raw.Path = "/media/raw/anime/Show/S01/E01-renamed.mkv"
	job.MediaPath = raw.Path
	renamed := resolveLifecycleTestProjection(library, []models.AssetRecord{raw, library}, []models.QueueJob{job}, nil)
	if renamed.CurrentRawCorrelation.State != assetCorrelationResolved || renamed.CurrentRawCorrelation.RawAssetRecordID != raw.ID {
		t.Fatalf("rename correlation=%#v", renamed.CurrentRawCorrelation)
	}

	library.Path = "/media/library/anime/Show/S01/E01-renamed.mkv"
	job.PublishedPath = library.Path
	renamedLibrary := resolveLifecycleTestProjection(library, []models.AssetRecord{raw, library}, []models.QueueJob{job}, nil)
	if renamedLibrary.CurrentPublication.State != assetCurrentPublicationPublished {
		t.Fatalf("library rename lost publication=%#v", renamedLibrary.CurrentPublication)
	}
}

func TestResolveAssetLifecycleProjectionKeepsDimensionsIndependent(t *testing.T) {
	raw := lifecycleTestRawRecord(40, "/media/raw/anime/Show/S01/E01.mkv")
	library := lifecycleTestLibraryRecord(41, "/media/library/anime/Show/S01/E01.mkv")
	archivedAt := lifecycleTestTime(time.Hour)
	job := lifecycleQueueJob(42, library.Path, library.LibraryID, archivedAt)
	job.SourceAssetRecordID = &raw.ID
	job.OriginalArchivedPath = "/media/originals_archive/anime/Show/S01/E01.mkv"

	projection := resolveLifecycleTestProjection(library, []models.AssetRecord{raw, library}, []models.QueueJob{job}, nil)
	if projection.Location != AssetLocationLibrary || projection.Lifecycle != AssetLifecyclePublished ||
		projection.Provenance != AssetProvenanceConverted || projection.Validation != AssetValidationPassed ||
		projection.Publication != AssetPublicationPublished || projection.Original != AssetOriginalArchived {
		t.Fatalf("independent dimensions collapsed: %#v", projection)
	}

	queued := lifecycleTestRawRecord(43, "/media/raw/anime/Show/S01/E02.mkv")
	queuedJob := models.QueueJob{ID: 44, MediaPath: queued.Path, Status: JobStatusQueued, Stage: JobStageQueued}
	queuedProjection := resolveLifecycleTestProjection(queued, []models.AssetRecord{queued}, []models.QueueJob{queuedJob}, nil)
	if queuedProjection.Location != AssetLocationRaw || queuedProjection.Lifecycle != AssetLifecycleQueued {
		t.Fatalf("workflow changed location: %#v", queuedProjection)
	}
}

func TestResolveAssetLifecycleProjectionActionCapabilitiesAreAdvisory(t *testing.T) {
	allowed := ResolveAssetActionCapabilities(AssetActionGuardContext{
		IdentityKnown: true, Location: AssetLocationLibrary, RootSafe: true,
		ProvenanceKnown: true, HasCurrentPublication: true, OriginalAvailable: true,
	})
	if !allowed.Rename.Allowed || !allowed.Reconvert.Allowed || !allowed.Archive.Allowed {
		t.Fatalf("safe supported actions were not exposed: %#v", allowed)
	}

	active := ResolveAssetActionCapabilities(AssetActionGuardContext{
		IdentityKnown: true, Location: AssetLocationLibrary, RootSafe: true,
		ProvenanceKnown: true, HasCurrentPublication: true, HasActiveQueue: true,
	})
	if active.Rename.Allowed || active.Archive.Allowed || !active.Cancel.Allowed || active.Rename.Reason != assetActionReasonActiveWork {
		t.Fatalf("active-work capability guard=%#v", active)
	}
	archiveActive := ResolveAssetActionCapabilities(AssetActionGuardContext{
		IdentityKnown: true, Location: AssetLocationLibrary, RootSafe: true,
		ProvenanceKnown: true, HasCurrentPublication: true, HasActiveQueue: true,
		OriginalArchiveActive: true,
	})
	if archiveActive.Rename.Allowed || archiveActive.Rename.Reason != assetActionReasonOriginalArchiveActive {
		t.Fatalf("active original archive did not expose the temporary Rename reason: %#v", archiveActive.Rename)
	}

	unknown := ResolveAssetActionCapabilities(AssetActionGuardContext{
		IdentityKnown: true, Location: AssetLocationLibrary, RootSafe: true,
		HasCurrentPublication: true,
	})
	if unknown.Archive.Allowed || unknown.Archive.Reason != assetActionReasonUnknownProvenance {
		t.Fatalf("unknown provenance did not fail closed=%#v", unknown)
	}

	unsafe := ResolveAssetActionCapabilities(AssetActionGuardContext{
		IdentityKnown: true, Location: AssetLocationLibrary, ProvenanceKnown: true,
		HasCurrentPublication: true, OriginalArchived: true,
		OriginalArchiveIdentityKnown: true, OriginalArchiveAvailable: true,
	})
	if unsafe.DeleteOriginal.Allowed || unsafe.DeleteOriginal.Reason != assetActionReasonUnsafeRoot {
		t.Fatalf("unsafe root did not fail closed=%#v", unsafe)
	}
}

func TestPublishedArchiveLifecycleIsActiveAssetWork(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:asset-published-archive-active?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}); err != nil {
		t.Fatal(err)
	}
	publishedAt := lifecycleTestTime(time.Minute)
	tests := []struct {
		status         string
		stage          string
		projectedStage string
		want           bool
	}{
		{status: "pending", stage: JobStageCompleted, projectedStage: JobStagePublishing, want: true},
		{status: "archiving", stage: JobStageCompleted, projectedStage: JobStageArchivingOriginal, want: true},
		{status: "complete", stage: JobStageCompleted, want: false},
		{status: "failed", stage: JobStageCompleted, want: false},
		{status: "not_applicable", stage: JobStageCompleted, want: false},
	}
	for _, test := range tests {
		t.Run(test.status, func(t *testing.T) {
			path := "/media/raw/published-" + test.status + ".mkv"
			record := models.AssetRecord{ID: 91, Path: path}
			job := models.QueueJob{
				MediaPath: path, Status: JobStatusCompleted, Stage: test.stage,
				PublishedAt: publishedAt, PublicationState: PublicationPublished,
				OriginalArchiveStatus: test.status,
			}
			if err := db.Create(&job).Error; err != nil {
				t.Fatal(err)
			}
			active, err := (AssetHandler{db: db}).assetHasActiveWork(record)
			if err != nil || active != test.want {
				t.Fatalf("asset active work=%t want=%t err=%v", active, test.want, err)
			}
			operation, hasQueue, hasMaintenance := resolveActiveOperation(record, "", []models.QueueJob{job}, nil)
			if hasQueue != test.want || hasMaintenance || (operation != nil) != test.want {
				t.Fatalf("active operation=%#v queue=%t maintenance=%t want=%t", operation, hasQueue, hasMaintenance, test.want)
			}
			if test.want && operation.Stage != test.projectedStage {
				t.Fatalf("active archive stage=%q want=%q", operation.Stage, test.projectedStage)
			}
		})
	}
}

func TestArchivedOriginalProjectionUsesCanonicalCreatedAtAndExistingRetention(t *testing.T) {
	raw := lifecycleTestRawRecord(50, "/media/raw/anime/Show/S01/E01.mkv")
	library := lifecycleTestLibraryRecord(51, "/media/library/anime/Show/S01/E01.mkv")
	archiveTime := time.Date(2025, 3, 4, 5, 6, 7, 0, time.UTC)
	expiry := time.Now().Add(48 * time.Hour)
	archive := models.AssetRecord{
		ID: 52, Path: "/media/originals_archive/anime/Show/S01/E01.mkv",
		RootPath: "/media/originals_archive", RelativePath: "anime/Show/S01/E01.mkv",
		FileName: "E01.mkv", SizeBytes: 987654, ModifiedAt: archiveTime.Add(3 * time.Hour),
		SyncedAt: archiveTime.Add(5 * time.Hour), CreatedAt: archiveTime,
		Status: "archive", ExpiresAt: &expiry,
	}
	publishedAt := archiveTime.Add(9 * time.Hour)
	job := lifecycleQueueJob(53, library.Path, library.LibraryID, &publishedAt)
	job.SourceAssetRecordID = &raw.ID
	job.OriginalArchivedPath = archive.Path
	project := func(currentJob models.QueueJob, archiveRecord models.AssetRecord) AssetLifecycleProjection {
		return ResolveAssetLifecycleProjection(AssetLifecycleProjectionInput{
			Record: library, Records: []models.AssetRecord{raw, library, archiveRecord},
			Libraries: []models.Library{lifecycleTestLibrary()}, QueueJobs: []models.QueueJob{currentJob},
			ArchiveRoot: "/media/originals_archive",
		})
	}

	projection := project(job, archive)
	if len(projection.ArchivedOriginals) != 1 {
		t.Fatalf("archived original inspection=%#v", projection.ArchivedOriginals)
	}
	original := projection.ArchivedOriginals[0]
	if original.Relationship != "current" || original.Availability != "available" || original.AssetRecordID != archive.ID || original.QueueJobID != job.ID {
		t.Fatalf("canonical archived-original identity=%#v", original)
	}
	if original.ArchivedAt == nil || !original.ArchivedAt.Equal(archiveTime) {
		t.Fatalf("archivedAt must use the archive AssetRecord CreatedAt: %#v", original.ArchivedAt)
	}
	if original.SizeBytes != archive.SizeBytes || original.RetentionExpiry == nil || !original.RetentionExpiry.Equal(expiry) || original.RetentionState != "retained" {
		t.Fatalf("archive metadata/retention was recalculated or lost: %#v", original)
	}
	if !original.Restore.Allowed || !original.DeleteOriginal.Allowed {
		t.Fatalf("current safe original actions=%#v", original)
	}

	archive.SyncedAt = archiveTime.Add(12 * time.Hour)
	archive.ModifiedAt = archiveTime.Add(24 * time.Hour)
	job.PublishedAt = lifecycleTestTime(72 * time.Hour)
	updated := project(job, archive).ArchivedOriginals[0]
	if updated.ArchivedAt == nil || !updated.ArchivedAt.Equal(archiveTime) {
		t.Fatalf("sync/file/publication timestamps changed archivedAt: %#v", updated.ArchivedAt)
	}
}

func TestArchivedOriginalRemainsCanonicalAfterConvertedAssetRename(t *testing.T) {
	raw := lifecycleTestRawRecord(60, "/media/raw/anime/Show/S01/E01.mkv")
	library := lifecycleTestLibraryRecord(61, "/media/library/anime/Show/S01/E01 renamed.mkv")
	library.FileName = "E01 renamed.mkv"
	library.RelativePath = "Show/S01/E01 renamed.mkv"
	archive := models.AssetRecord{
		ID: 62, Path: "/media/originals_archive/anime/Show/S01/E01.mkv",
		RootPath: "/media/originals_archive", RelativePath: "anime/Show/S01/E01.mkv",
		FileName: "E01.mkv", Status: "archive",
	}
	publishedAt := lifecycleTestTime(48 * time.Hour)
	job := lifecycleQueueJob(63, library.Path, library.LibraryID, publishedAt)
	job.SourceAssetRecordID = &raw.ID
	job.OriginalArchivedPath = archive.Path
	job.PublicationState = PublicationPublished
	job.OriginalArchiveStatus = "complete"
	projection := resolveLifecycleTestProjection(library, []models.AssetRecord{raw, library, archive}, []models.QueueJob{job}, nil)
	if !projection.Actions.Rename.Allowed {
		t.Fatalf("completed archive kept Rename blocked: %#v", projection.Actions.Rename)
	}
	if len(projection.ArchivedOriginals) != 1 {
		t.Fatalf("archived original association was lost after rename: %#v", projection.ArchivedOriginals)
	}
	original := projection.ArchivedOriginals[0]
	if original.Relationship != "current" || original.Path != archive.Path || original.AssetRecordID != archive.ID || original.QueueJobID != job.ID {
		t.Fatalf("renamed publication no longer resolves its canonical archived original: %#v", original)
	}

	job.OriginalArchiveStatus = "archiving"
	active := resolveLifecycleTestProjection(library, []models.AssetRecord{raw, library, archive}, []models.QueueJob{job}, nil)
	if active.Actions.Rename.Allowed || active.Actions.Rename.Reason != assetActionReasonOriginalArchiveActive {
		t.Fatalf("active archival did not temporarily block Rename: %#v", active.Actions.Rename)
	}
}

func TestArchivedOriginalProjectionLeavesLegacyAndHistoricalIdentityUnresolved(t *testing.T) {
	raw := lifecycleTestRawRecord(60, "/media/raw/anime/Show/S01/E01.mkv")
	library := lifecycleTestLibraryRecord(61, "/media/library/anime/Show/S01/E01.mkv")
	archive := models.AssetRecord{
		ID: 62, Path: "/media/originals_archive/anime/Show/S01/E01.mkv",
		RootPath: "/media/originals_archive", RelativePath: "anime/Show/S01/E01.mkv",
		FileName: "E01.mkv", Status: "archive", CreatedAt: time.Now(),
	}
	publishedAt := lifecycleTestTime(time.Hour)
	job := lifecycleQueueJob(63, library.Path, library.LibraryID, publishedAt)
	job.OriginalArchivedPath = archive.Path
	pathOnly := ResolveAssetLifecycleProjection(AssetLifecycleProjectionInput{
		Record: library, Records: []models.AssetRecord{library, archive},
		Libraries: []models.Library{lifecycleTestLibrary()}, QueueJobs: []models.QueueJob{job},
		ArchiveRoot: "/media/originals_archive",
	})
	if len(pathOnly.ArchivedOriginals) != 1 || pathOnly.ArchivedOriginals[0].Relationship != "unresolved" ||
		pathOnly.ArchivedOriginals[0].ArchivedAt != nil || pathOnly.ArchivedOriginals[0].Restore.Allowed || pathOnly.ArchivedOriginals[0].DeleteOriginal.Allowed {
		t.Fatalf("path-only legacy archive was trusted: %#v", pathOnly.ArchivedOriginals)
	}

	job.SourceAssetRecordID = &raw.ID
	retiredAt := time.Now()
	job.PublicationRetiredAt = &retiredAt
	historical := ResolveAssetLifecycleProjection(AssetLifecycleProjectionInput{
		Record: library, Records: []models.AssetRecord{raw, library, archive},
		Libraries: []models.Library{lifecycleTestLibrary()}, QueueJobs: []models.QueueJob{job},
		ArchiveRoot: "/media/originals_archive",
	})
	if len(historical.ArchivedOriginals) != 1 || historical.ArchivedOriginals[0].Relationship != "historical" ||
		historical.ArchivedOriginals[0].Restore.Allowed || historical.ArchivedOriginals[0].DeleteOriginal.Allowed {
		t.Fatalf("retired archive evidence was relabeled or made actionable: %#v", historical.ArchivedOriginals)
	}

	archive.Missing = true
	job.PublicationRetiredAt = nil
	missing := ResolveAssetLifecycleProjection(AssetLifecycleProjectionInput{
		Record: library, Records: []models.AssetRecord{raw, library, archive},
		Libraries: []models.Library{lifecycleTestLibrary()}, QueueJobs: []models.QueueJob{job},
		ArchiveRoot: "/media/originals_archive",
	})
	if len(missing.ArchivedOriginals) != 1 || missing.ArchivedOriginals[0].Availability != "missing" ||
		missing.ArchivedOriginals[0].Path != archive.Path || missing.ArchivedOriginals[0].Restore.Allowed || missing.ArchivedOriginals[0].DeleteOriginal.Allowed {
		t.Fatalf("missing archive evidence was lost or actionable: %#v", missing.ArchivedOriginals)
	}
}

func TestAdaptQueueSelectedAssetsResponsePreservesCanonicalOutcomes(t *testing.T) {
	adapted := AdaptQueueSelectedAssetsResponse(QueueSelectedAssetsResponse{
		Summary: QueueSelectedAssetsSummary{Selected: 4, Eligible: 1, Queued: 1, Skipped: 2, Failed: 1},
		Results: []QueueSelectedAssetResult{
			{AssetID: 1, Outcome: "queued", Reason: "", JobID: 101},
			{AssetID: 2, Outcome: "skipped", Reason: "already_queued"},
			{AssetID: 3, Outcome: "skipped", Reason: "needs_review"},
			{AssetID: 4, Outcome: "failed", Reason: "invalid_configuration"},
		},
	})
	if len(adapted.Results) != 4 || !adapted.Results[0].Selectable || !adapted.Results[0].Eligible {
		t.Fatalf("queued result mapping=%#v", adapted)
	}
	if adapted.Results[1].Eligible || adapted.Results[1].Reason != "already_queued" {
		t.Fatalf("skipped reason was changed=%#v", adapted.Results[1])
	}
	if adapted.Results[3].Eligible || adapted.Results[3].Outcome != "failed" || adapted.Results[3].Reason != "invalid_configuration" {
		t.Fatalf("failed result mapping=%#v", adapted.Results[3])
	}
}

func TestQueueAndDirectPublicationCaptureSourceAssetIdentity(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:asset-source-identity?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AssetRecord{}, &models.QueueJob{}, &models.DirectPublication{}); err != nil {
		t.Fatal(err)
	}
	record := lifecycleTestRawRecord(50, "/media/raw/anime/Show/S01/E01.mkv")
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	id, err := sourceAssetRecordIDForPath(db, record.Path)
	if err != nil || id == nil || *id != record.ID {
		t.Fatalf("source identity lookup=%v err=%v record=%#v", id, err, record)
	}
	if err := db.Create(&models.QueueJob{MediaPath: record.Path, SourceAssetRecordID: id, Status: JobStatusQueued}).Error; err != nil {
		t.Fatal(err)
	}
	publication := models.DirectPublication{SourcePath: record.Path, SourceAssetRecordID: id, PublishedPath: "/media/library/anime/Show/S01/E01.mkv", LibraryID: 1, PublishedAt: time.Now()}
	if err := db.Create(&publication).Error; err != nil {
		t.Fatal(err)
	}
	var storedJob models.QueueJob
	if err := db.First(&storedJob).Error; err != nil || storedJob.SourceAssetRecordID == nil || *storedJob.SourceAssetRecordID != record.ID {
		t.Fatalf("stored queue source identity=%#v err=%v", storedJob, err)
	}
	var storedPublication models.DirectPublication
	if err := db.First(&storedPublication).Error; err != nil || storedPublication.SourceAssetRecordID == nil || *storedPublication.SourceAssetRecordID != record.ID {
		t.Fatalf("stored publication source identity=%#v err=%v", storedPublication, err)
	}
	if _, hasMode := reflect.TypeOf(models.DirectPublication{}).FieldByName("PublicationMode"); hasMode {
		t.Fatal("DirectPublication must not persist a publication mode")
	}
}

func TestArchivedOriginalProjectionDoesNotRebindAfterLibraryPathReuse(t *testing.T) {
	previousSource := lifecycleTestRawRecord(70, "/media/raw/anime/Show/S01/E01.mkv")
	currentSource := lifecycleTestRawRecord(71, "/media/raw/anime/Replacement/S01/E01.mkv")
	library := lifecycleTestLibraryRecord(72, "/media/library/anime/Show/S01/E01.mkv")
	archive := models.AssetRecord{
		ID: 73, Path: "/media/originals_archive/anime/Show/S01/E01.mkv",
		RootPath: "/media/originals_archive", RelativePath: "anime/Show/S01/E01.mkv",
		FileName: "E01.mkv", Status: "archive", CreatedAt: time.Now(),
	}
	publishedAt := lifecycleTestTime(time.Hour)
	retiredAt := lifecycleTestTime(2 * time.Hour)
	previousJob := lifecycleQueueJob(74, library.Path, library.LibraryID, publishedAt)
	previousJob.SourceAssetRecordID = &previousSource.ID
	previousJob.OriginalArchivedPath = archive.Path
	previousJob.PublicationRetiredAt = retiredAt
	currentJob := lifecycleQueueJob(75, library.Path, library.LibraryID, publishedAt)
	currentJob.SourceAssetRecordID = &currentSource.ID
	currentJob.MediaPath = currentSource.Path

	projection := ResolveAssetLifecycleProjection(AssetLifecycleProjectionInput{
		Record: library, Records: []models.AssetRecord{previousSource, currentSource, library, archive},
		Libraries: []models.Library{lifecycleTestLibrary()}, QueueJobs: []models.QueueJob{previousJob, currentJob},
		ArchiveRoot: "/media/originals_archive",
	})
	if projection.CurrentRawCorrelation.State != assetCorrelationUnresolved {
		t.Fatalf("reused Library path arbitrarily selected one source identity: %#v", projection.CurrentRawCorrelation)
	}
	if len(projection.ArchivedOriginals) != 0 {
		t.Fatalf("historical archive was rebound to a reused Library path: %#v", projection.ArchivedOriginals)
	}

	conflictingJob := lifecycleQueueJob(76, library.Path, library.LibraryID, publishedAt)
	conflictingJob.SourceAssetRecordID = &currentSource.ID
	conflictingJob.MediaPath = currentSource.Path
	conflictingJob.OriginalArchivedPath = archive.Path
	conflictedProjection := ResolveAssetLifecycleProjection(AssetLifecycleProjectionInput{
		Record: library, Records: []models.AssetRecord{currentSource, library, archive},
		Libraries: []models.Library{lifecycleTestLibrary()}, QueueJobs: []models.QueueJob{conflictingJob},
		DirectPublications: []models.DirectPublication{lifecycleDirectPublication(77, library.Path, library.LibraryID, *publishedAt)},
		ArchiveRoot:        "/media/originals_archive",
	})
	if len(conflictedProjection.ArchivedOriginals) != 1 || conflictedProjection.ArchivedOriginals[0].Relationship != "unresolved" ||
		conflictedProjection.ArchivedOriginals[0].ArchivedAt != nil || conflictedProjection.ArchivedOriginals[0].Restore.Allowed ||
		conflictedProjection.ArchivedOriginals[0].DeleteOriginal.Allowed {
		t.Fatalf("conflicting publication authorities trusted or exposed the archived original: %#v", conflictedProjection.ArchivedOriginals)
	}
}

func TestDeleteArchiveRevalidatesActiveWork(t *testing.T) {
	db, rawRoot, libraryRoot, archiveRoot := safeDeleteTestDB(t, "archive-toctou")
	if err := db.AutoMigrate(&models.AssetMaintenanceOperation{}); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(archiveRoot, "Show", "E01.mkv")
	writeTestFile(t, archivePath, "archive bytes")
	archive, libraryAsset, _, _ := seedContextualArchivedOriginal(t, db, rawRoot, libraryRoot, archiveRoot, "Show/E01.mkv", archivePath)
	if err := db.Create(&models.AssetMaintenanceOperation{
		ID: "active-archive-delete-check", OperationType: "track", AssetRecordID: libraryAsset.ID,
		AssetPath: libraryAsset.Path, Status: maintenanceStatusRunning, Phase: maintenanceStatusRunning,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if active, err := (AssetHandler{db: db}).assetHasActiveWork(libraryAsset); err != nil || !active {
		t.Fatalf("focused active-work setup did not resolve: active=%t err=%v record=%#v", active, err, libraryAsset)
	}
	validatedArchive, validatedLibrary, validatedSource, status, err := (AssetHandler{db: db}).validateArchivedOriginalIdentity(archive.ID, libraryAsset.ID, archivePath, nil)
	if err != nil || status != http.StatusOK {
		t.Fatalf("canonical identity setup status=%d err=%v", status, err)
	}
	if active, err := (AssetHandler{db: db}).archivedOriginalHasActiveWork(validatedArchive, validatedLibrary, validatedSource); err != nil || !active {
		t.Fatalf("combined active-work setup did not resolve: active=%t err=%v", active, err)
	}
	body, _ := json.Marshal(map[string]any{
		"confirmed": true, "archiveAssetRecordId": archive.ID,
		"libraryAssetRecordId": libraryAsset.ID,
	})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.DELETE("/api/assets/archive", (AssetHandler{db: db}).DeleteArchive)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/assets/archive?path="+url.QueryEscape(archivePath), strings.NewReader(string(body))))
	if response.Code != http.StatusConflict {
		t.Fatalf("archive TOCTOU guard status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("active-work rejection modified the archive target: %v", err)
	}
	if gotStatus, err := (AssetHandler{db: db}).permanentlyDeleteArchiveAsset(archivePath, archive.ID, libraryAsset.ID, nil); err == nil || gotStatus != http.StatusConflict {
		t.Fatalf("archive TOCTOU guard status=%d err=%v", gotStatus, err)
	}
}

func TestDeleteArchiveRechecksIdentityAfterRelationshipChanges(t *testing.T) {
	db, rawRoot, libraryRoot, archiveRoot := safeDeleteTestDB(t, "archive-delete-identity-toctou")
	archivePath := filepath.Join(archiveRoot, "Show", "E02.mkv")
	writeTestFile(t, archivePath, "archive bytes")
	archive, libraryAsset, _, job := seedContextualArchivedOriginal(t, db, rawRoot, libraryRoot, archiveRoot, "Show/E02.mkv", archivePath)
	libraryReads := 0
	changed := false
	if err := db.Callback().Query().After("gorm:query").Register("test:mutate-archive-link-before-fence", func(tx *gorm.DB) {
		if tx.Statement.Table != "libraries" {
			return
		}
		libraryReads++
		if libraryReads != 2 {
			return
		}
		if err := db.Model(&models.QueueJob{}).Where("id = ?", job.ID).Update("original_archived_path", filepath.Join(archiveRoot, "Show", "changed.mkv")).Error; err != nil {
			t.Errorf("change archived-original relationship at fence: %v", err)
			return
		}
		changed = true
	}); err != nil {
		t.Fatal(err)
	}

	if status, err := (AssetHandler{db: db}).permanentlyDeleteArchiveAsset(archivePath, archive.ID, libraryAsset.ID, nil); err == nil || status != http.StatusConflict {
		t.Fatalf("changed archive relationship status=%d err=%v", status, err)
	}
	if !changed {
		t.Fatal("test did not change the relationship after initial identity validation")
	}
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("changed relationship was not fenced before unlink: %v", err)
	}
}
