package handlers

import (
	"reflect"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
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
	})
	if unsafe.DeleteOriginal.Allowed || unsafe.DeleteOriginal.Reason != assetActionReasonUnsafeRoot {
		t.Fatalf("unsafe root did not fail closed=%#v", unsafe)
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

func TestDeleteArchiveRevalidatesActiveWork(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:archive-toctou?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AssetRecord{}, &models.QueueJob{}, &models.AssetMaintenanceOperation{}); err != nil {
		t.Fatal(err)
	}
	record := models.AssetRecord{ID: 60, Path: "/media/originals_archive/Show/E01.mkv", FileName: "E01.mkv", Status: "archive"}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.QueueJob{MediaPath: "/media/raw/Show/E01.mkv", SourceAssetRecordID: &record.ID, Status: JobStatusQueued}).Error; err != nil {
		t.Fatal(err)
	}
	gotStatus, err := (AssetHandler{db: db}).permanentlyDeleteArchiveAsset(record.Path)
	if err == nil || gotStatus != 409 {
		t.Fatalf("archive TOCTOU guard status=%d err=%v", gotStatus, err)
	}
}
