package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestFilesEqualUsesContentNotOnlySize(t *testing.T) {
	dir := t.TempDir()
	left := filepath.Join(dir, "left.mkv")
	right := filepath.Join(dir, "right.mkv")
	if err := os.WriteFile(left, []byte("same-content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(right, []byte("same-content"), 0o600); err != nil {
		t.Fatal(err)
	}
	equal, err := filesEqual(left, right)
	if err != nil || !equal {
		t.Fatalf("expected identical files: equal=%v err=%v", equal, err)
	}
	if err := os.WriteFile(right, []byte("other-value!"), 0o600); err != nil {
		t.Fatal(err)
	}
	equal, err = filesEqual(left, right)
	if err != nil || equal {
		t.Fatalf("expected different files: equal=%v err=%v", equal, err)
	}
}

func TestAutomatedPipelineDoesNotRequireAutoAnalysis(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:pipeline-independent-stages?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	setting := models.AppSetting{Key: "pipelineAutomation", Value: models.JSONMap{
		"autoAnalysisEnabled": false, "autoValidationEnabled": false, "autoPublisherEnabled": true,
	}}
	if err := db.Create(&setting).Error; err != nil {
		t.Fatal(err)
	}
	result := NewWorkerHandler(db).runAutomatedPipeline(models.QueueJob{})
	if result["analysisStatus"] != "skipped" || result["stoppedAt"] != "validation" {
		t.Fatalf("unexpected independent pipeline result: %#v", result)
	}
}

func TestOriginalsArchivePathMapsLegacyContainerPathToConfiguredHostRoot(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:archive-path-mapping?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	hostRoot := filepath.Join(t.TempDir(), "originals")
	settings := []models.AppSetting{
		{Key: "paths", Value: models.JSONMap{"originalsArchivePath": hostRoot}},
		{Key: "originalRetentionPolicy", Value: models.JSONMap{"processedOriginalsPath": "/media/originals_archive/processed-originals"}},
	}
	for _, setting := range settings {
		if err := db.Create(&setting).Error; err != nil {
			t.Fatal(err)
		}
	}
	got, err := originalsArchivePath(db)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(hostRoot, "processed-originals")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestOriginalsArchivePathDoesNotOverrideConfiguredRootWithLegacyTrashPath(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:archive-path-not-trash?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	archiveRoot := filepath.Join(t.TempDir(), "originals")
	if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{
		"originalsArchivePath": archiveRoot,
		"trashPath":            "/media/trash",
	}}).Error; err != nil {
		t.Fatal(err)
	}
	got, err := originalsArchivePath(db)
	if err != nil {
		t.Fatal(err)
	}
	if got != archiveRoot {
		t.Fatalf("got %q want configured archive root %q", got, archiveRoot)
	}
}

func TestConcurrentOriginalArchivalCreatesOnlyOneArchive(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:concurrent-original-archive?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	rawRoot := filepath.Join(root, "raw")
	archiveRoot := filepath.Join(root, "archive")
	original := filepath.Join(rawRoot, "anime", "Baccano", "episode.mkv")
	if err := os.MkdirAll(filepath.Dir(original), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(original, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{"rawRoot": rawRoot, "originalsArchivePath": archiveRoot}}).Error; err != nil {
		t.Fatal(err)
	}

	handler := PublisherHandler{db: db}
	job := models.QueueJob{MediaPath: original}
	results := make(chan string, 2)
	errors := make(chan error, 2)
	var workers sync.WaitGroup
	workers.Add(2)
	for range 2 {
		go func() {
			defer workers.Done()
			archived, archiveErr := handler.archivePublishedOriginal(job)
			results <- archived
			errors <- archiveErr
		}()
	}
	workers.Wait()
	close(results)
	close(errors)
	for archiveErr := range errors {
		if archiveErr != nil {
			t.Fatalf("concurrent archive failed: %v", archiveErr)
		}
	}
	nonEmptyResults := 0
	for archived := range results {
		if archived != "" {
			nonEmptyResults++
		}
	}
	if nonEmptyResults != 1 {
		t.Fatalf("archive operations returning a path=%d want=1", nonEmptyResults)
	}
	entries, err := os.ReadDir(filepath.Join(archiveRoot, "anime", "Baccano"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("archive entries=%d want=1", len(entries))
	}
}

func TestCleanupStagedJobRemovesWholeJobDirectory(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:staged-job-cleanup?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{"stagingPath": root}}).Error; err != nil {
		t.Fatal(err)
	}
	jobRoot := filepath.Join(root, "job-7")
	output := filepath.Join(jobRoot, "title", "output.mkv")
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, []byte("output"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobRoot, ".DS_Store"), []byte("metadata"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (PublisherHandler{db: db}).cleanupStagedJob(models.QueueJob{ID: 7, OutputPath: output}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(jobRoot); !os.IsNotExist(err) {
		t.Fatalf("job directory still exists: %v", err)
	}
}

func TestPublishLibraryReplacementArchivesOriginalAndKeepsPath(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:library-replacement?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Library{}, &models.QueueJob{}, &models.AppSetting{}, &models.SchedulerReservation{}, &models.AssetRecord{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	libraryRoot := filepath.Join(root, "library")
	archiveRoot := filepath.Join(root, "archive")
	stagingRoot := filepath.Join(root, "staging")
	original := filepath.Join(libraryRoot, "Anime", "Movie", "movie.mkv")
	output := filepath.Join(stagingRoot, "job-1", "movie.mkv")
	if err := os.MkdirAll(filepath.Dir(original), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(original, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	originalSidecar := strings.TrimSuffix(original, filepath.Ext(original)) + ".spa.srt"
	if err := os.WriteFile(originalSidecar, []byte("library subtitle"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, []byte("converted"), 0o600); err != nil {
		t.Fatal(err)
	}
	library := models.Library{Name: "Anime", SourcePath: libraryRoot, DestinationPath: libraryRoot, Type: "movies"}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{"stagingPath": stagingRoot, "originalsArchivePath": archiveRoot}}).Error; err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: original, PublishMode: PublishModeReplaceLibrary, LibraryID: library.ID, ProfileID: 1, Status: JobStatusCompleted, Stage: JobStageValidating, OutputPath: output, ValidationStatus: ValidationStatusPassed}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}

	result, err := (PublisherHandler{db: db}).publishLibraryReplacement(job, library, models.Profile{Container: "mkv"})
	if err != nil {
		t.Fatal(err)
	}
	if result.PublishedPath != original {
		t.Fatalf("published path=%q want=%q", result.PublishedPath, original)
	}
	got, err := os.ReadFile(original)
	if err != nil || string(got) != "converted" {
		t.Fatalf("replacement content=%q err=%v", got, err)
	}
	archived := filepath.Join(archiveRoot, "library-replacements", fmt.Sprintf("library-%d", library.ID), "Anime", "Movie", "movie.mkv")
	got, err = os.ReadFile(archived)
	if err != nil || string(got) != "original" {
		t.Fatalf("archive content=%q err=%v", got, err)
	}
	if _, err := os.Stat(filepath.Dir(output)); !os.IsNotExist(err) {
		t.Fatalf("workspace was not cleaned: %v", err)
	}
	if got, err := os.ReadFile(originalSidecar); err != nil || string(got) != "library subtitle" {
		t.Fatalf("library sidecar was moved or removed: content=%q err=%v", got, err)
	}
	var convertedRecord models.AssetRecord
	if err := db.First(&convertedRecord, "path = ?", original).Error; err != nil {
		t.Fatalf("converted asset was not incrementally indexed: %v", err)
	}
	if convertedRecord.Status != "converted" || convertedRecord.LibraryID != library.ID || convertedRecord.SizeBytes != int64(len("converted")) {
		t.Fatalf("unexpected converted inventory record: %#v", convertedRecord)
	}
	var archiveRecord models.AssetRecord
	if err := db.First(&archiveRecord, "path = ?", archived).Error; err != nil {
		t.Fatalf("archived original was not incrementally indexed: %v", err)
	}
	if archiveRecord.Status != "archive" || archiveRecord.SizeBytes != int64(len("original")) {
		t.Fatalf("unexpected archive inventory record: %#v", archiveRecord)
	}
	second, err := (PublisherHandler{db: db}).publishQueueJob(job, false)
	if err != nil {
		t.Fatalf("idempotent second publication failed: %v", err)
	}
	if second.Status != "already_published" {
		t.Fatalf("second publication status=%q", second.Status)
	}
	archivedFiles, err := filepath.Glob(filepath.Join(filepath.Dir(archived), "*.mkv"))
	if err != nil || len(archivedFiles) != 1 {
		t.Fatalf("second publication archived the converted output: files=%v err=%v", archivedFiles, err)
	}
}

func TestCopyExternalSubtitleSidecarsToPublishedAsset(t *testing.T) {
	root := t.TempDir()
	rawMedia := filepath.Join(root, "raw", "Movies", "Original Name.mkv")
	libraryMedia := filepath.Join(root, "library", "Movies", "Published Name.mkv")
	for _, mediaPath := range []string{rawMedia, libraryMedia} {
		if err := os.MkdirAll(filepath.Dir(mediaPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(mediaPath, []byte("video"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	rawBase := strings.TrimSuffix(rawMedia, filepath.Ext(rawMedia))
	srt := rawBase + ".spa.default.srt"
	ass := rawBase + ".eng.2.ass"
	if err := os.WriteFile(srt, []byte("spanish"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ass, []byte("english"), 0o600); err != nil {
		t.Fatal(err)
	}

	copied, err := copyExternalSubtitleSidecars(rawMedia, libraryMedia, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(copied) != 2 {
		t.Fatalf("copied sidecars=%#v", copied)
	}
	libraryBase := strings.TrimSuffix(libraryMedia, filepath.Ext(libraryMedia))
	for path, want := range map[string]string{
		libraryBase + ".spa.default.srt": "spanish",
		libraryBase + ".eng.2.ass":       "english",
	} {
		got, readErr := os.ReadFile(path)
		if readErr != nil || string(got) != want {
			t.Fatalf("published sidecar %q content=%q err=%v", path, got, readErr)
		}
	}
	for path, want := range map[string]string{srt: "spanish", ass: "english"} {
		got, readErr := os.ReadFile(path)
		if readErr != nil || string(got) != want {
			t.Fatalf("raw sidecar %q was moved or removed: content=%q err=%v", path, got, readErr)
		}
	}

	if err := os.WriteFile(libraryBase+".spa.default.srt", []byte("edited in library"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := copyExternalSubtitleSidecars(rawMedia, libraryMedia, false, nil); err != nil {
		t.Fatalf("different Library sidecar must not block publication: %v", err)
	}
	if got, err := os.ReadFile(libraryBase + ".spa.default.srt"); err != nil || string(got) != "edited in library" {
		t.Fatalf("existing Library sidecar was overwritten: content=%q err=%v", got, err)
	}
	if got, err := os.ReadFile(libraryBase + ".mvf.spa.default.srt"); err != nil || string(got) != "spanish" {
		t.Fatalf("incoming sidecar was not preserved with MVF name: content=%q err=%v", got, err)
	}

	if err := os.WriteFile(libraryBase+".mvf.spa.default.srt", []byte("another version"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := copyExternalSubtitleSidecars(rawMedia, libraryMedia, false, nil); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(libraryBase + ".mvf-2.spa.default.srt"); err != nil || string(got) != "spanish" {
		t.Fatalf("second collision was not preserved: content=%q err=%v", got, err)
	}
}

func TestPathIsInsideRejectsSiblingPrefix(t *testing.T) {
	root := filepath.Join(t.TempDir(), "library")
	if !pathIsInside(filepath.Join(root, "movie.mkv"), root) {
		t.Fatal("expected library child to be accepted")
	}
	if pathIsInside(filepath.Join(root+"-other", "movie.mkv"), root) {
		t.Fatal("sibling prefix must not be accepted")
	}
}

func TestRestoreOriginalIfNeededSkipsWhenRawOriginalExists(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open("file:restore-original-already-present?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	rawRoot := filepath.Join(root, "raw")
	archiveRoot := filepath.Join(root, "archive")

	rawOriginal := filepath.Join(rawRoot, "Movies", "movie.mkv")
	archivedOriginal := filepath.Join(archiveRoot, "Movies", "movie.mkv")

	if err := os.MkdirAll(filepath.Dir(rawOriginal), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(archivedOriginal), 0o755); err != nil {
		t.Fatal(err)
	}

	// Deliberately use different content.
	// Raw must always win.
	if err := os.WriteFile(rawOriginal, []byte("raw-original"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(archivedOriginal, []byte("archived-original"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := db.Create(&models.AppSetting{
		Key: "paths",
		Value: models.JSONMap{
			"rawRoot":              rawRoot,
			"originalsArchivePath": archiveRoot,
		},
	}).Error; err != nil {
		t.Fatal(err)
	}

	job := models.QueueJob{
		MediaPath:            rawOriginal,
		OriginalArchivedPath: archivedOriginal,
	}

	result, err := (PublisherHandler{db: db}).restoreOriginalIfNeeded(job)
	if err != nil {
		t.Fatal(err)
	}

	if result != "already_present" {
		t.Fatalf("restore result=%q want=%q", result, "already_present")
	}

	rawContent, err := os.ReadFile(rawOriginal)
	if err != nil {
		t.Fatal(err)
	}

	if string(rawContent) != "raw-original" {
		t.Fatalf(
			"raw original was modified: content=%q",
			string(rawContent),
		)
	}

	archiveContent, err := os.ReadFile(archivedOriginal)
	if err != nil {
		t.Fatal(err)
	}

	if string(archiveContent) != "archived-original" {
		t.Fatalf(
			"archived original was unexpectedly modified: content=%q",
			string(archiveContent),
		)
	}
}

func TestRestoreOriginalIfNeededRestoresFromArchiveWhenRawMissing(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open("file:restore-original-from-archive?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	rawRoot := filepath.Join(root, "raw")
	archiveRoot := filepath.Join(root, "archive")

	rawOriginal := filepath.Join(rawRoot, "Movies", "movie.mkv")
	archivedOriginal := filepath.Join(archiveRoot, "Movies", "movie.mkv")

	if err := os.MkdirAll(filepath.Dir(archivedOriginal), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		archivedOriginal,
		[]byte("archived-original"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	if err := db.Create(&models.AppSetting{
		Key: "paths",
		Value: models.JSONMap{
			"rawRoot":              rawRoot,
			"originalsArchivePath": archiveRoot,
		},
	}).Error; err != nil {
		t.Fatal(err)
	}

	job := models.QueueJob{
		MediaPath:            rawOriginal,
		OriginalArchivedPath: archivedOriginal,
	}

	result, err := (PublisherHandler{db: db}).restoreOriginalIfNeeded(job)
	if err != nil {
		t.Fatal(err)
	}

	if result != "restored" {
		t.Fatalf("restore result=%q want=%q", result, "restored")
	}

	rawContent, err := os.ReadFile(rawOriginal)
	if err != nil {
		t.Fatal(err)
	}

	if string(rawContent) != "archived-original" {
		t.Fatalf(
			"restored raw content=%q want=%q",
			string(rawContent),
			"archived-original",
		)
	}

	if _, err := os.Stat(archivedOriginal); !os.IsNotExist(err) {
		t.Fatalf(
			"archived original should no longer exist after restore, err=%v",
			err,
		)
	}
}

func TestDiscardJobKeepsRawAndRemovesStaging(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open("file:discard-publisher-keeps-raw?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.AutoMigrate(
		&models.QueueJob{},
		&models.AppSetting{},
		&models.SchedulerReservation{},
	); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()

	rawRoot := filepath.Join(root, "raw")
	stagingRoot := filepath.Join(root, "staging")
	archiveRoot := filepath.Join(root, "archive")

	rawOriginal := filepath.Join(
		rawRoot,
		"Movies",
		"movie.mkv",
	)

	if err := os.MkdirAll(filepath.Dir(rawOriginal), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		rawOriginal,
		[]byte("original"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	if err := db.Create(&models.AppSetting{
		Key: "paths",
		Value: models.JSONMap{
			"rawRoot":              rawRoot,
			"stagingPath":          stagingRoot,
			"originalsArchivePath": archiveRoot,
		},
	}).Error; err != nil {
		t.Fatal(err)
	}

	job := models.QueueJob{
		MediaPath: rawOriginal,
		Status:    JobStatusCompleted,
		Stage:     JobStageReadyToPublish,
	}

	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}

	jobRoot := filepath.Join(
		stagingRoot,
		fmt.Sprintf("job-%d", job.ID),
	)

	outputPath := filepath.Join(
		jobRoot,
		"output.mkv",
	)

	if err := os.MkdirAll(jobRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		outputPath,
		[]byte("converted"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	job.OutputPath = outputPath

	if err := db.Save(&job).Error; err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()

	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(
		http.MethodDelete,
		fmt.Sprintf("/api/publisher/jobs/%d", job.ID),
		nil,
	)

	ctx.Params = gin.Params{
		{
			Key:   "id",
			Value: strconv.FormatUint(uint64(job.ID), 10),
		},
	}

	handler := PublisherHandler{
		db: db,
	}

	handler.DiscardJob(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"status=%d body=%s",
			rec.Code,
			rec.Body.String(),
		)
	}

	// RAW original must remain untouched.
	rawContent, err := os.ReadFile(rawOriginal)
	if err != nil {
		t.Fatal(err)
	}

	if string(rawContent) != "original" {
		t.Fatalf(
			"raw original changed: content=%q",
			string(rawContent),
		)
	}

	// Entire staging workspace must be removed.
	if _, err := os.Stat(jobRoot); !os.IsNotExist(err) {
		t.Fatalf(
			"staging job directory still exists: %s err=%v",
			jobRoot,
			err,
		)
	}

	var updated models.QueueJob

	if err := db.First(&updated, job.ID).Error; err != nil {
		t.Fatal(err)
	}

	if updated.Status != JobStatusCanceled {
		t.Fatalf(
			"status=%q want=%q",
			updated.Status,
			JobStatusCanceled,
		)
	}

	if updated.Stage != JobStageCanceled {
		t.Fatalf(
			"stage=%q want=%q",
			updated.Stage,
			JobStageCanceled,
		)
	}

	if updated.DismissedAt == nil {
		t.Fatal("expected dismissed_at to be set")
	}
}

func TestDiscardJobAllowsStuckPublishingJob(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open("file:discard-stuck-publishing-job?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.AutoMigrate(
		&models.QueueJob{},
		&models.AppSetting{},
		&models.SchedulerReservation{},
	); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()

	rawRoot := filepath.Join(root, "raw")
	stagingRoot := filepath.Join(root, "staging")
	archiveRoot := filepath.Join(root, "archive")

	rawOriginal := filepath.Join(
		rawRoot,
		"Movies",
		"movie.mkv",
	)

	if err := os.MkdirAll(filepath.Dir(rawOriginal), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		rawOriginal,
		[]byte("original"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	if err := db.Create(&models.AppSetting{
		Key: "paths",
		Value: models.JSONMap{
			"rawRoot":              rawRoot,
			"stagingPath":          stagingRoot,
			"originalsArchivePath": archiveRoot,
		},
	}).Error; err != nil {
		t.Fatal(err)
	}

	job := models.QueueJob{
		MediaPath: rawOriginal,
		Status:    JobStatusCompleted,
		Stage:     JobStagePublishing,
	}

	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}

	jobRoot := filepath.Join(
		stagingRoot,
		fmt.Sprintf("job-%d", job.ID),
	)

	outputPath := filepath.Join(
		jobRoot,
		"output.mkv",
	)

	if err := os.MkdirAll(jobRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		outputPath,
		[]byte("converted"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	job.OutputPath = outputPath

	if err := db.Save(&job).Error; err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()

	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(
		http.MethodDelete,
		fmt.Sprintf("/api/publisher/jobs/%d", job.ID),
		nil,
	)

	ctx.Params = gin.Params{
		{
			Key:   "id",
			Value: strconv.FormatUint(uint64(job.ID), 10),
		},
	}

	handler := PublisherHandler{
		db: db,
	}

	handler.DiscardJob(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"status=%d body=%s",
			rec.Code,
			rec.Body.String(),
		)
	}

	// RAW original must remain untouched.
	rawContent, err := os.ReadFile(rawOriginal)
	if err != nil {
		t.Fatal(err)
	}

	if string(rawContent) != "original" {
		t.Fatalf(
			"raw original changed: content=%q",
			string(rawContent),
		)
	}

	// Entire staging workspace must be removed.
	if _, err := os.Stat(jobRoot); !os.IsNotExist(err) {
		t.Fatalf(
			"staging job directory still exists: %s err=%v",
			jobRoot,
			err,
		)
	}

	var updated models.QueueJob

	if err := db.First(&updated, job.ID).Error; err != nil {
		t.Fatal(err)
	}

	if updated.Status != JobStatusCanceled {
		t.Fatalf(
			"status=%q want=%q",
			updated.Status,
			JobStatusCanceled,
		)
	}

	if updated.Stage != JobStageCanceled {
		t.Fatalf(
			"stage=%q want=%q",
			updated.Stage,
			JobStageCanceled,
		)
	}

	if updated.DismissedAt == nil {
		t.Fatal("expected dismissed_at to be set")
	}
}

func TestDiscardJobRestoresRawFromArchiveAndRemovesStaging(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open("file:discard-publisher-restores-raw?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.AutoMigrate(
		&models.QueueJob{},
		&models.AppSetting{},
		&models.SchedulerReservation{},
	); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()

	rawRoot := filepath.Join(root, "raw")
	stagingRoot := filepath.Join(root, "staging")
	archiveRoot := filepath.Join(root, "archive")

	rawOriginal := filepath.Join(
		rawRoot,
		"Movies",
		"movie.mkv",
	)

	archivedOriginal := filepath.Join(
		archiveRoot,
		"Movies",
		"movie.mkv",
	)

	if err := os.MkdirAll(filepath.Dir(archivedOriginal), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		archivedOriginal,
		[]byte("original-from-archive"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	if err := db.Create(&models.AppSetting{
		Key: "paths",
		Value: models.JSONMap{
			"rawRoot":              rawRoot,
			"stagingPath":          stagingRoot,
			"originalsArchivePath": archiveRoot,
		},
	}).Error; err != nil {
		t.Fatal(err)
	}

	job := models.QueueJob{
		MediaPath:            rawOriginal,
		OriginalArchivedPath: archivedOriginal,
		Status:               JobStatusCompleted,
		Stage:                JobStageReadyToPublish,
	}

	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}

	jobRoot := filepath.Join(
		stagingRoot,
		fmt.Sprintf("job-%d", job.ID),
	)

	outputPath := filepath.Join(
		jobRoot,
		"output.mkv",
	)

	if err := os.MkdirAll(jobRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		outputPath,
		[]byte("converted"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	job.OutputPath = outputPath

	if err := db.Save(&job).Error; err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()

	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(
		http.MethodDelete,
		fmt.Sprintf("/api/publisher/jobs/%d", job.ID),
		nil,
	)

	ctx.Params = gin.Params{
		{
			Key:   "id",
			Value: strconv.FormatUint(uint64(job.ID), 10),
		},
	}

	handler := PublisherHandler{
		db: db,
	}

	handler.DiscardJob(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf(
			"status=%d body=%s",
			rec.Code,
			rec.Body.String(),
		)
	}

	rawContent, err := os.ReadFile(rawOriginal)
	if err != nil {
		t.Fatal(err)
	}

	if string(rawContent) != "original-from-archive" {
		t.Fatalf(
			"restored raw content=%q want=%q",
			string(rawContent),
			"original-from-archive",
		)
	}

	if _, err := os.Stat(archivedOriginal); !os.IsNotExist(err) {
		t.Fatalf(
			"archived original still exists: %s err=%v",
			archivedOriginal,
			err,
		)
	}

	if _, err := os.Stat(jobRoot); !os.IsNotExist(err) {
		t.Fatalf(
			"staging job directory still exists: %s err=%v",
			jobRoot,
			err,
		)
	}

	var updated models.QueueJob

	if err := db.First(&updated, job.ID).Error; err != nil {
		t.Fatal(err)
	}

	if updated.Status != JobStatusCanceled {
		t.Fatalf(
			"status=%q want=%q",
			updated.Status,
			JobStatusCanceled,
		)
	}

	if updated.Stage != JobStageCanceled {
		t.Fatalf(
			"stage=%q want=%q",
			updated.Stage,
			JobStageCanceled,
		)
	}

	if updated.DismissedAt == nil {
		t.Fatal("expected dismissed_at to be set")
	}
}

func TestPublishFailureBeforeArchiveReturnsJobToReadyToPublish(t *testing.T) {
	db, err := gorm.Open(
		sqlite.Open("file:publish-failure-rolls-back-stage?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := db.AutoMigrate(
		&models.Library{},
		&models.Profile{},
		&models.QueueJob{},
		&models.AppSetting{},
		&models.SchedulerReservation{},
	); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()

	rawRoot := filepath.Join(root, "raw")
	stagingRoot := filepath.Join(root, "staging")
	libraryRoot := filepath.Join(root, "library")
	archiveRoot := filepath.Join(root, "archive")

	rawOriginal := filepath.Join(
		rawRoot,
		"Movies",
		"movie.mkv",
	)

	outputPath := filepath.Join(
		stagingRoot,
		"job-output",
		"movie.mkv",
	)

	destinationPath := filepath.Join(
		libraryRoot,
		"Movies",
		"movie.mkv",
	)

	for _, path := range []string{
		rawOriginal,
		outputPath,
		destinationPath,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.WriteFile(
		rawOriginal,
		[]byte("original"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(
		outputPath,
		[]byte("converted-output"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	// Existing Library destination deliberately differs from staging.
	if err := os.WriteFile(
		destinationPath,
		[]byte("existing-different-content"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	if err := db.Create(&models.AppSetting{
		Key: "paths",
		Value: models.JSONMap{
			"rawRoot":              rawRoot,
			"stagingPath":          stagingRoot,
			"originalsArchivePath": archiveRoot,
		},
	}).Error; err != nil {
		t.Fatal(err)
	}

	library := models.Library{
		Name:            "Movies",
		SourcePath:      libraryRoot,
		DestinationPath: libraryRoot,
		Type:            "movies",
	}

	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}

	profile := models.Profile{
		Name:      "Test Profile",
		Container: "mkv",
	}

	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}

	job := models.QueueJob{
		MediaPath:            rawOriginal,
		OutputPath:           outputPath,
		PlannedPublishedPath: destinationPath,
		LibraryID:            library.ID,
		ProfileID:            profile.ID,
		Status:               JobStatusCompleted,
		Stage:                JobStageReadyToPublish,
		ValidationStatus:     ValidationStatusPassed,
	}

	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}

	handler := PublisherHandler{db: db}

	_, publishErr := handler.publishQueueJob(job, false)

	if publishErr == nil {
		t.Fatal("expected publication conflict")
	}

	var typedErr publishError
	if !errors.As(publishErr, &typedErr) {
		t.Fatalf(
			"publish error type=%T want publishError: %v",
			publishErr,
			publishErr,
		)
	}

	if typedErr.Status != http.StatusConflict {
		t.Fatalf(
			"publish status=%d want=%d",
			typedErr.Status,
			http.StatusConflict,
		)
	}

	var updated models.QueueJob
	if err := db.First(&updated, job.ID).Error; err != nil {
		t.Fatal(err)
	}

	if updated.Stage != JobStageReadyToPublish {
		t.Fatalf(
			"stage=%q want=%q",
			updated.Stage,
			JobStageReadyToPublish,
		)
	}

	if updated.Status != JobStatusCompleted {
		t.Fatalf(
			"status=%q want=%q",
			updated.Status,
			JobStatusCompleted,
		)
	}

	if updated.PublishedAt != nil {
		t.Fatal("published_at must remain nil after failed publication")
	}

	if strings.TrimSpace(updated.PublishedPath) != "" {
		t.Fatalf(
			"published_path=%q want empty",
			updated.PublishedPath,
		)
	}

	// Staging output must remain available for retry/discard.
	gotOutput, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}

	if string(gotOutput) != "converted-output" {
		t.Fatalf(
			"staging output changed: content=%q",
			string(gotOutput),
		)
	}

	// Existing Library file must not be overwritten.
	gotDestination, err := os.ReadFile(destinationPath)
	if err != nil {
		t.Fatal(err)
	}

	if string(gotDestination) != "existing-different-content" {
		t.Fatalf(
			"existing destination changed: content=%q",
			string(gotDestination),
		)
	}

	// Original must still be untouched because archival was never reached.
	gotOriginal, err := os.ReadFile(rawOriginal)
	if err != nil {
		t.Fatal(err)
	}

	if string(gotOriginal) != "original" {
		t.Fatalf(
			"raw original changed: content=%q",
			string(gotOriginal),
		)
	}
}

func TestExternalSubtitleSidecarOverwriteCreatesBackup(t *testing.T) {
	root := t.TempDir()

	sourceMedia := filepath.Join(root, "raw", "Movie.mkv")
	sourceSubtitle := filepath.Join(root, "raw", "Movie.en.srt")

	destinationMedia := filepath.Join(root, "library", "Movie.mkv")
	destinationSubtitle := filepath.Join(root, "library", "Movie.en.srt")

	if err := os.MkdirAll(filepath.Dir(sourceMedia), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Dir(destinationMedia), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(sourceMedia, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(sourceSubtitle, []byte("new subtitle"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(destinationSubtitle, []byte("old subtitle"), 0o644); err != nil {
		t.Fatal(err)
	}

	backups := []publishBackup{}

	_, err := copyExternalSubtitleSidecars(
		sourceMedia,
		destinationMedia,
		true,
		&backups,
	)

	if err != nil {
		t.Fatal(err)
	}

	if len(backups) != 1 {
		t.Fatalf("backups=%d want=1", len(backups))
	}

	backupData, err := os.ReadFile(backups[0].BackupPath)
	if err != nil {
		t.Fatal(err)
	}

	if string(backupData) != "old subtitle" {
		t.Fatalf(
			"backup content=%q want=%q",
			string(backupData),
			"old subtitle",
		)
	}
}
