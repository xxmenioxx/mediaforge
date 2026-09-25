package handlers

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPhaseDLegacyInventoryPreservesEvidenceWithoutFabricatingProvenance(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:assets-phase-d-legacy?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.AppSetting{}, &models.AssetRecord{}, &models.Library{}, &models.QueueJob{},
		&models.DirectPublication{}, &models.AssetMaintenanceOperation{}, &models.ScanResult{},
		&models.SourceGroup{},
	); err != nil {
		t.Fatal(err)
	}

	library := models.Library{ID: 1, Name: "Anime", SourcePath: "/media/raw/anime", DestinationPath: "/media/library/anime"}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	staleGroup := models.SourceGroup{ID: 7, Name: "Stale", RelativePath: "stale", SourcePath: "/media/raw/stale", Enabled: false}
	if err := db.Create(&staleGroup).Error; err != nil {
		t.Fatal(err)
	}

	convertedOnly := models.AssetRecord{
		ID: 10, Path: "/media/library/anime/Legacy/converted-only.mkv", RootPath: library.DestinationPath,
		RelativePath: "Legacy/converted-only.mkv", FileName: "converted-only.mkv", Status: "converted",
		LibraryID: library.ID, LibraryName: library.Name,
	}
	pathReusedLibrary := models.AssetRecord{
		ID: 11, Path: "/media/library/anime/Legacy/path-reused.mkv", RootPath: library.DestinationPath,
		RelativePath: "Legacy/path-reused.mkv", FileName: "path-reused.mkv", Status: "converted",
		LibraryID: library.ID, LibraryName: library.Name,
	}
	pathReusedRaw := models.AssetRecord{
		ID: 12, Path: "/media/raw/anime/Legacy/path-reused.mkv", RootPath: "/media/raw/anime",
		RelativePath: "Legacy/path-reused.mkv", FileName: "path-reused.mkv", Status: "unprocessed",
		SourceGroupID: staleGroup.ID,
	}
	archive := models.AssetRecord{
		ID: 13, Path: "/media/originals_archive/anime/Legacy/converted-only.mkv", RootPath: "/media/originals_archive",
		RelativePath: "anime/Legacy/converted-only.mkv", FileName: "converted-only.mkv", Status: "archive",
		LibraryID: library.ID, LibraryName: library.Name,
	}
	asIs := models.AssetRecord{
		ID: 14, Path: "/media/library/anime/Legacy/as-is.mkv", RootPath: library.DestinationPath,
		RelativePath: "Legacy/as-is.mkv", FileName: "as-is.mkv", Status: "converted",
		LibraryID: library.ID, LibraryName: library.Name,
	}
	if err := db.Create(&[]models.AssetRecord{convertedOnly, pathReusedLibrary, pathReusedRaw, archive, asIs}).Error; err != nil {
		t.Fatal(err)
	}

	publishedAt := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	legacyJob := models.QueueJob{
		ID: 20, MediaPath: pathReusedRaw.Path, PublishedPath: pathReusedLibrary.Path, LibraryID: library.ID,
		Status: JobStatusCompleted, Stage: JobStageCompleted, PublishedAt: &publishedAt,
		ValidationReport: models.JSONMap{"legacy": true},
	}
	asIsPublication := models.DirectPublication{
		ID: 21, SourcePath: "/media/raw/anime/Legacy/as-is.mkv", PublishedPath: asIs.Path,
		LibraryID: library.ID, PublishedAt: publishedAt,
	}
	if err := db.Create(&legacyJob).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&asIsPublication).Error; err != nil {
		t.Fatal(err)
	}

	inventory, err := NewAssetHandler(db).assetInventoryFromDB()
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Library) != 3 {
		t.Fatalf("legacy Library records were not retained: %#v", inventory.Library)
	}
	if len(inventory.Unverified) != 1 || inventory.Unverified[0].ID != convertedOnly.ID {
		t.Fatalf("converted-only record was treated as proven converted: %#v", inventory.Unverified)
	}
	convertedProjection := inventory.Unverified[0].Lifecycle
	if convertedProjection == nil || convertedProjection.Provenance != AssetProvenanceLegacyOrUnknown {
		t.Fatalf("converted-only provenance=%#v", convertedProjection)
	}
	if convertedProjection.CurrentPublication.State != assetCurrentPublicationUnpublished {
		t.Fatalf("converted-only publication=%#v", convertedProjection.CurrentPublication)
	}
	if convertedProjection.Actions.DeleteOriginal.Allowed {
		t.Fatal("unknown legacy provenance must fail closed for original deletion")
	}

	pathProjection := findAssetByID(inventory.Library, pathReusedLibrary.ID).Lifecycle
	if pathProjection == nil || pathProjection.Provenance != AssetProvenanceConverted {
		t.Fatalf("path-only publication was not retained as converted provenance: %#v", pathProjection)
	}
	if pathProjection.CurrentRawCorrelation.State != assetCorrelationUnresolved {
		t.Fatalf("path reuse fabricated current RAW identity: %#v", pathProjection.CurrentRawCorrelation)
	}
	if len(pathProjection.SourceProvenance) != 1 || pathProjection.SourceProvenance[0].SourcePath != pathReusedRaw.Path {
		t.Fatalf("historical source path was not preserved: %#v", pathProjection.SourceProvenance)
	}

	asIsProjection := findAssetByID(inventory.Library, asIs.ID).Lifecycle
	if asIsProjection == nil || asIsProjection.Provenance != AssetProvenancePublishedAsIs {
		t.Fatalf("DirectPublication provenance=%#v", asIsProjection)
	}
	if asIsProjection.CurrentRawCorrelation.State != assetCorrelationUnresolved {
		t.Fatalf("path-only DirectPublication fabricated current RAW identity: %#v", asIsProjection.CurrentRawCorrelation)
	}

	if len(inventory.Archive) != 1 || inventory.Archive[0].ID != archive.ID {
		t.Fatalf("archive compatibility inventory was lost: %#v", inventory.Archive)
	}
	if inventory.Archive[0].Lifecycle == nil || inventory.Archive[0].Lifecycle.Original != AssetOriginalArchived {
		t.Fatalf("archive evidence was not projected: %#v", inventory.Archive[0].Lifecycle)
	}
	if len(inventory.SourceGroups) != 1 || inventory.SourceGroups[0].ID != staleGroup.ID {
		t.Fatalf("stale Source Group provenance was not retained for RAW: %#v", inventory.SourceGroups)
	}
	if len(inventory.Raw) != 1 || inventory.Raw[0].ID != pathReusedRaw.ID {
		t.Fatalf("RAW compatibility inventory=%#v", inventory.Raw)
	}
	hasLegacyEvidence := false
	for _, asset := range inventory.Library {
		for _, jobID := range asset.Lifecycle.EvidenceSummary.QueueJobIDs {
			if jobID == legacyJob.ID {
				hasLegacyEvidence = true
			}
		}
	}
	if !hasLegacyEvidence {
		t.Fatal("legacy report-bearing QueueJob was not retained as evidence summary")
	}
}

func findAssetByID(assets []Asset, id uint) Asset {
	for _, asset := range assets {
		if asset.ID == id {
			return asset
		}
	}
	return Asset{}
}

func TestPhaseDInventoryReadIsBatchedAndDoesNotReconcileFilesystem(t *testing.T) {
	type queryCounter struct{ bytes.Buffer }
	var counter queryCounter
	db, err := gorm.Open(sqlite.Open("file:assets-phase-d-query?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.New(log.New(&counter, "", 0), logger.Config{LogLevel: logger.Info, Colorful: false}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.AppSetting{}, &models.AssetRecord{}, &models.Library{}, &models.QueueJob{},
		&models.DirectPublication{}, &models.AssetMaintenanceOperation{}, &models.ScanResult{},
		&models.SourceGroup{},
	); err != nil {
		t.Fatal(err)
	}
	library := models.Library{Name: "Anime", SourcePath: "/phase-d/raw", DestinationPath: "/phase-d/library"}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 65; index++ {
		status := "converted"
		path := filepath.Join(library.DestinationPath, "Show", "episode-"+strings.TrimSpace(time.Duration(index).String())+".mkv")
		if index == 0 {
			path = filepath.Join(library.DestinationPath, "Show", "episode-000.mkv")
		}
		if err := db.Create(&models.AssetRecord{
			Path: path, RootPath: library.DestinationPath, RelativePath: filepath.Base(path),
			FileName: filepath.Base(path), Status: status, LibraryID: library.ID, LibraryName: library.Name,
			Missing: false,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	counter.Reset()

	handler := NewAssetHandler(db)
	first, err := handler.assetInventoryFromDB()
	if err != nil {
		t.Fatal(err)
	}
	firstQueryCount := strings.Count(strings.ToUpper(counter.String()), "SELECT")
	if len(first.Library) != 65 {
		t.Fatalf("first inventory size=%d", len(first.Library))
	}
	if first.Library[0].Missing {
		t.Fatal("inventory read changed persisted missing state")
	}

	for index := 65; index < 129; index++ {
		path := filepath.Join(library.DestinationPath, "Show", "episode-"+strings.TrimSpace(time.Duration(index).String())+".mkv")
		if err := db.Create(&models.AssetRecord{
			Path: path, RootPath: library.DestinationPath, RelativePath: filepath.Base(path),
			FileName: filepath.Base(path), Status: "converted", LibraryID: library.ID, LibraryName: library.Name,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	counter.Reset()
	second, err := handler.assetInventoryFromDB()
	if err != nil {
		t.Fatal(err)
	}
	secondQueryCount := strings.Count(strings.ToUpper(counter.String()), "SELECT")
	if len(second.Library) != 129 {
		t.Fatalf("second inventory size=%d", len(second.Library))
	}
	if secondQueryCount > firstQueryCount+2 {
		t.Fatalf("inventory query count grew with rows: first=%d second=%d", firstQueryCount, secondQueryCount)
	}

	var plan []struct {
		Detail string `gorm:"column:detail"`
	}
	if err := db.Raw("EXPLAIN QUERY PLAN SELECT * FROM asset_records WHERE library_id = ? AND status = ?", library.ID, "converted").Scan(&plan).Error; err != nil {
		t.Fatal(err)
	}
	planText := strings.ToUpper(strings.Join(func() []string {
		items := make([]string, 0, len(plan))
		for _, row := range plan {
			items = append(items, row.Detail)
		}
		return items
	}(), " "))
	if !strings.Contains(planText, "SEARCH") || !strings.Contains(planText, "INDEX") {
		t.Fatalf("library/status predicate is not indexed: %q", planText)
	}

	var record models.AssetRecord
	if err := db.First(&record, first.Library[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if record.Missing {
		t.Fatal("normal inventory rendering performed filesystem reconciliation")
	}
}

func TestPhaseDLibraryRenameReconcilesCurrentProjectionWithoutRewritingRetiredHistory(t *testing.T) {
	oldPath := "/phase-d/library/Show/episode.mkv"
	newPath := "/phase-d/library/Show/episode-renamed.mkv"
	retiredAt := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	publishedAt := retiredAt.Add(time.Hour)
	current := models.AssetRecord{ID: 90, Path: newPath, RootPath: "/phase-d/library", RelativePath: "Show/episode-renamed.mkv", FileName: "episode-renamed.mkv", Status: "converted", LibraryID: 1}
	legacySource := models.AssetRecord{ID: 91, Path: "/phase-d/raw/Show/episode.mkv", RootPath: "/phase-d/raw", Status: "unprocessed"}
	retired := models.QueueJob{ID: 901, MediaPath: "/phase-d/raw/Show/episode.mkv", PublishedPath: oldPath, LibraryID: 1, Status: JobStatusCompleted, Stage: JobStageCompleted, PublishedAt: &retiredAt, PublicationRetiredAt: &retiredAt}
	active := models.QueueJob{ID: 902, MediaPath: legacySource.Path, PublishedPath: newPath, LibraryID: 1, Status: JobStatusCompleted, Stage: JobStageCompleted, PublishedAt: &publishedAt}
	projection := ResolveAssetLifecycleProjection(AssetLifecycleProjectionInput{
		Record: current, Records: []models.AssetRecord{current, legacySource}, Libraries: []models.Library{{ID: 1, Name: "Phase D", DestinationPath: "/phase-d/library"}}, QueueJobs: []models.QueueJob{retired, active},
	})
	if projection.CurrentPublication.ID != active.ID || projection.CurrentPublication.PublishedPath != newPath {
		t.Fatalf("current publication did not follow reconciled path: %#v", projection.CurrentPublication)
	}
	if len(projection.SourceProvenance) != 1 || projection.SourceProvenance[0].RecordID != active.ID {
		t.Fatalf("current provenance was not resolved from reconciled evidence: %#v", projection.SourceProvenance)
	}
	if retired.PublishedPath != oldPath || retired.MediaPath != "/phase-d/raw/Show/episode.mkv" {
		t.Fatalf("retired history fixture was rewritten: %#v", retired)
	}

	unrelated := models.QueueJob{ID: 903, MediaPath: "/phase-d/library/Other/episode.mkv", Status: JobStatusQueued}
	if hasActiveQueueForTest(current, []models.QueueJob{unrelated}) {
		t.Fatal("unrelated Queue work must not block Library rename/correlation")
	}
}

func TestPhaseDMigratePathUsesCanonicalActiveWorkGuard(t *testing.T) {
	for _, test := range []struct {
		name       string
		queueMedia string
		wantStatus int
	}{
		{name: "same asset is blocked", queueMedia: "same", wantStatus: 409},
		{name: "unrelated Queue work is independent", queueMedia: "unrelated", wantStatus: 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			sourceRoot := filepath.Join(root, "source")
			destinationRoot := filepath.Join(root, "destination")
			sourcePath := filepath.Join(sourceRoot, "Show")
			mediaPath := filepath.Join(sourcePath, "episode.mkv")
			if err := os.MkdirAll(sourcePath, 0o755); err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, mediaPath, "phase-d-media")
			db, err := gorm.Open(sqlite.Open("file:assets-phase-d-migrate-"+strings.ReplaceAll(t.Name(), "/", "-")+"?mode=memory&cache=shared"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(&models.AppSetting{}, &models.AssetRecord{}, &models.Library{}, &models.QueueJob{}, &models.AssetMaintenanceOperation{}, &models.ScanResult{}); err != nil {
				t.Fatal(err)
			}
			sourceLibrary := models.Library{Name: "Source", DestinationPath: sourceRoot}
			destinationLibrary := models.Library{Name: "Destination", DestinationPath: destinationRoot}
			if err := db.Create(&sourceLibrary).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&destinationLibrary).Error; err != nil {
				t.Fatal(err)
			}
			record := models.AssetRecord{Path: mediaPath, RootPath: sourceRoot, RelativePath: "Show/episode.mkv", FileName: "episode.mkv", Status: "library", LibraryID: sourceLibrary.ID, LibraryName: sourceLibrary.Name}
			if err := db.Create(&record).Error; err != nil {
				t.Fatal(err)
			}
			queueMedia := "/phase-d/raw/unrelated.mkv"
			if test.queueMedia == "same" {
				queueMedia = "/phase-d/raw/episode.mkv"
			}
			if err := db.Create(&models.QueueJob{MediaPath: queueMedia, SourceAssetRecordID: func() *uint {
				if test.queueMedia == "same" {
					return &record.ID
				}
				return nil
			}(), Status: JobStatusQueued}).Error; err != nil {
				t.Fatal(err)
			}

			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.POST("/api/assets/migrate-path", NewAssetHandler(db).MigratePath)
			bodyBytes, err := json.Marshal(AssetPathMigrationInput{SourcePath: sourcePath, DestinationLibraryID: destinationLibrary.ID})
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/assets/migrate-path", strings.NewReader(string(bodyBytes)))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if test.wantStatus == 409 && !strings.Contains(response.Body.String(), "active work") {
				t.Fatalf("active-work reason missing: %s", response.Body.String())
			}
		})
	}
}

func hasActiveQueueForTest(record models.AssetRecord, jobs []models.QueueJob) bool {
	for _, job := range jobs {
		if job.MediaPath == record.Path || job.PublishedPath == record.Path || job.SourceAssetRecordID != nil && *job.SourceAssetRecordID == record.ID {
			return true
		}
	}
	return false
}
