package handlers

import (
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPlannedOutputPathForMultiEpisodeBatchNamesEpisodes(t *testing.T) {
	db := queueJobTestDB(t)
	library := models.Library{
		SourcePath:      "/media/raw",
		DestinationPath: "/media/anime",
		ValidationRules: models.JSONMap{"episodeNamingEnabled": true},
	}
	profile := models.Profile{Container: "mkv"}
	jobs := []models.QueueJob{
		{
			MediaPath: "/media/raw/Ranma 1-2 (1989)/Season 01/title01.mkv",
			BatchID:   "batch-ranma",
			BatchName: "Ranma 1-2 (1989)/Season 01",
			LibraryID: 1,
			ProfileID: 1,
		},
		{
			MediaPath: "/media/raw/Ranma 1-2 (1989)/Season 01/title02.mkv",
			BatchID:   "batch-ranma",
			BatchName: "Ranma 1-2 (1989)/Season 01",
			LibraryID: 1,
			ProfileID: 1,
		},
	}
	for index := range jobs {
		if err := db.Create(&jobs[index]).Error; err != nil {
			t.Fatalf("create job: %v", err)
		}
	}

	firstPath := plannedOutputPathForJob(db, jobs[0], library, profile)
	secondPath := plannedOutputPathForJob(db, jobs[1], library, profile)

	if firstPath != "/media/anime/Ranma 1-2 (1989)/Season 01/Ranma 1-2 (1989) - S01E01.mkv" {
		t.Fatalf("unexpected first output path: %s", firstPath)
	}
	if secondPath != "/media/anime/Ranma 1-2 (1989)/Season 01/Ranma 1-2 (1989) - S01E02.mkv" {
		t.Fatalf("unexpected second output path: %s", secondPath)
	}
}

func TestPlannedOutputPathUsesNestedSeasonAndExistingEpisodeIdentity(t *testing.T) {
	db := queueJobTestDB(t)
	library := models.Library{SourcePath: "/media/raw", DestinationPath: "/media/library/anime", ValidationRules: models.JSONMap{"episodeNamingEnabled": true}}
	profile := models.Profile{Container: "mkv"}
	jobs := []models.QueueJob{
		{MediaPath: "/media/raw/anime/Baccano/Season0/Baccano! S00E01 (OAV01) source.mkv", BatchID: "baccano", BatchName: "anime/Baccano"},
		{MediaPath: "/media/raw/anime/Baccano/Season0/Baccano! S00E02 (OAV02) source.mkv", BatchID: "baccano", BatchName: "anime/Baccano"},
		{MediaPath: "/media/raw/anime/Baccano/Season0/NCED Calling [B6126979].mkv", BatchID: "baccano", BatchName: "anime/Baccano"},
	}
	for index := range jobs {
		if err := db.Create(&jobs[index]).Error; err != nil {
			t.Fatal(err)
		}
	}
	if got := plannedOutputPathForJob(db, jobs[0], library, profile); got != "/media/library/anime/Baccano/Season0/Season0-S00E01.mkv" {
		t.Fatalf("unexpected OAV path: %s", got)
	}
	if got := plannedOutputPathForJob(db, jobs[2], library, profile); got != "/media/library/anime/Baccano/Season0/Season0-NCED Calling.mkv" {
		t.Fatalf("unexpected credit path: %s", got)
	}
}

func TestPlannedOutputPathForSingleJobKeepsSourceName(t *testing.T) {
	db := queueJobTestDB(t)
	library := models.Library{
		SourcePath:      "/media/raw",
		DestinationPath: "/media/anime",
	}
	profile := models.Profile{Container: "mkv"}
	job := models.QueueJob{
		MediaPath: "/media/raw/Ranma 1-2 (1989)/Season 01/title01.mkv",
		BatchID:   "batch-single",
		BatchName: "Ranma 1-2 (1989)/Season 01",
		LibraryID: 1,
		ProfileID: 1,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create job: %v", err)
	}

	outputPath := plannedOutputPathForJob(db, job, library, profile)

	if outputPath != "/media/anime/Ranma 1-2 (1989)/Season 01/title01.mkv" {
		t.Fatalf("unexpected output path: %s", outputPath)
	}
}

func TestPlannedOutputPathReusesRetiredPublishedNameAfterSafeRecovery(t *testing.T) {
	db := queueJobTestDB(t)
	library := models.Library{
		ID:              1,
		SourcePath:      "/media/raw",
		DestinationPath: "/media/library/anime",
		ValidationRules: models.JSONMap{"episodeNamingEnabled": true},
	}
	profile := models.Profile{Container: "mkv"}
	retiredAt := time.Now()
	previous := models.QueueJob{
		MediaPath:            "/media/raw/Baccano/Season 01/source-title.mkv",
		LibraryID:            library.ID,
		ProfileID:            1,
		Status:               JobStatusCompleted,
		PublishedPath:        "/media/library/anime/Baccano/Season 01/Baccano - S01E04.mkv",
		PublicationRetiredAt: &retiredAt,
	}
	if err := db.Create(&previous).Error; err != nil {
		t.Fatalf("create previous job: %v", err)
	}
	reconversion := models.QueueJob{
		MediaPath: "/media/raw/Baccano/Season 01/source-title.mkv",
		BatchID:   "single-recovered-asset",
		BatchName: "Baccano/Season 01",
		LibraryID: library.ID,
		ProfileID: 1,
	}
	if err := db.Create(&reconversion).Error; err != nil {
		t.Fatalf("create reconversion job: %v", err)
	}

	got := plannedOutputPathForJob(db, reconversion, library, profile)
	want := "/media/library/anime/Baccano/Season 01/Baccano - S01E04.mkv"
	if got != want {
		t.Fatalf("reconversion output=%q want=%q", got, want)
	}
}

func TestPlannedOutputPathDropsRawSourceBucketForSelectedLibrary(t *testing.T) {
	db := queueJobTestDB(t)
	library := models.Library{
		SourcePath:      "/media/raw",
		DestinationPath: "/media/library/anime",
	}
	profile := models.Profile{Container: "mkv"}
	job := models.QueueJob{
		MediaPath: "/media/raw/series/Rurouni Kenshin/Trust & Betrayal Act 4.mkv",
		BatchID:   "batch-kenshin",
		BatchName: "series/Rurouni Kenshin",
		LibraryID: 1,
		ProfileID: 1,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create job: %v", err)
	}

	outputPath := plannedOutputPathForJob(db, job, library, profile)

	if outputPath != "/media/library/anime/Rurouni Kenshin/Trust & Betrayal Act 4.mkv" {
		t.Fatalf("unexpected output path: %s", outputPath)
	}
}

func TestPlannedOutputPathDoesNotDuplicateDestinationCategory(t *testing.T) {
	db := queueJobTestDB(t)
	library := models.Library{
		Name:            "Anime Movies",
		SourcePath:      "/media/raw",
		DestinationPath: "/media/library/anime-movies",
	}
	profile := models.Profile{Container: "mkv"}
	job := models.QueueJob{
		MediaPath: "/media/raw/anime-movies/Akira (1988)/Akira (1988).mkv",
		LibraryID: 1,
		ProfileID: 1,
	}

	outputPath := plannedOutputPathForJob(db, job, library, profile)

	if outputPath != "/media/library/anime-movies/Akira (1988)/Akira (1988).mkv" {
		t.Fatalf("unexpected output path: %s", outputPath)
	}
}

func TestPlannedOutputPathRemovesRepeatedDestinationCategories(t *testing.T) {
	db := queueJobTestDB(t)
	library := models.Library{
		Name:            "Anime",
		SourcePath:      "/media/raw",
		DestinationPath: "/media/library/anime",
	}
	profile := models.Profile{Container: "mkv"}
	job := models.QueueJob{
		MediaPath: "/media/raw/anime/anime/Conan el niño del futuro/Conan, el niño del futuro - 04_(Ladyarmaroid).mkv",
		LibraryID: 1,
		ProfileID: 1,
	}

	outputPath := plannedOutputPathForJob(db, job, library, profile)

	if outputPath != "/media/library/anime/Conan el niño del futuro/Conan, el niño del futuro - 04_(Ladyarmaroid).mkv" {
		t.Fatalf("unexpected output path: %s", outputPath)
	}
}

func TestPlannedStagingOutputPathUsesFlatJobWorkspace(t *testing.T) {
	profile := models.Profile{Container: "mkv"}
	job := models.QueueJob{
		ID:        7,
		MediaPath: "/media/raw/movies/Movie (1989)/feature.mp4",
		LibraryID: 1,
		ProfileID: 1,
	}

	outputPath := plannedStagingOutputPath(job, profile, pathSettings{rawRoot: "/media/raw", stagingPath: "/media/staging"})

	if outputPath != "/media/staging/job-7/feature.mkv" {
		t.Fatalf("unexpected staging output path: %s", outputPath)
	}
}

func TestPlannedStagingOutputPathIsolatesMatchingNamesByJob(t *testing.T) {
	profile := models.Profile{Container: "mkv"}
	paths := pathSettings{stagingPath: "/media/staging"}
	first := plannedStagingOutputPath(models.QueueJob{ID: 7, MediaPath: "/raw/anime/Movie/feature.mp4"}, profile, paths)
	second := plannedStagingOutputPath(models.QueueJob{ID: 8, MediaPath: "/raw/movies/Other/feature.mp4"}, profile, paths)

	if first != "/media/staging/job-7/feature.mkv" || second != "/media/staging/job-8/feature.mkv" {
		t.Fatalf("matching names must remain isolated by job: first=%s second=%s", first, second)
	}
}

func TestPlannedOutputPathForMultiEpisodeBatchRequiresLibraryOption(t *testing.T) {
	db := queueJobTestDB(t)
	library := models.Library{
		SourcePath:      "/media/raw",
		DestinationPath: "/media/movies",
		ValidationRules: models.JSONMap{},
	}
	profile := models.Profile{Container: "mkv"}
	jobs := []models.QueueJob{
		{MediaPath: "/media/raw/Movie (1989)/feature.mkv", BatchID: "batch-movie", BatchName: "Movie (1989)", LibraryID: 1, ProfileID: 1},
		{MediaPath: "/media/raw/Movie (1989)/bonus.mkv", BatchID: "batch-movie", BatchName: "Movie (1989)", LibraryID: 1, ProfileID: 1},
	}
	for index := range jobs {
		if err := db.Create(&jobs[index]).Error; err != nil {
			t.Fatalf("create job: %v", err)
		}
	}

	outputPath := plannedOutputPathForJob(db, jobs[1], library, profile)

	if outputPath != "/media/movies/Movie (1989)/bonus.mkv" {
		t.Fatalf("unexpected output path: %s", outputPath)
	}
}

func TestPlannedOutputPathMovesTaggedExtrasWhenLibraryOptionEnabled(t *testing.T) {
	db := queueJobTestDB(t)
	library := models.Library{
		SourcePath:      "/media/raw",
		DestinationPath: "/media/movies",
		ValidationRules: models.JSONMap{"extrasPathEnabled": true},
	}
	profile := models.Profile{Container: "mkv"}
	job := models.QueueJob{
		MediaPath: "/media/raw/Movie (1989)/bonus.mkv",
		BatchID:   "batch-movie",
		BatchName: "Movie (1989)",
		LibraryID: 1,
		ProfileID: 1,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatalf("create job: %v", err)
	}
	if err := saveAssetMetadataOverrides(db, map[string]AssetMetadataState{
		job.MediaPath: {Categories: []string{"extras"}},
	}); err != nil {
		t.Fatalf("save metadata: %v", err)
	}

	outputPath := plannedOutputPathForJob(db, job, library, profile)

	if outputPath != "/media/movies/Movie (1989)/extras/bonus.mkv" {
		t.Fatalf("unexpected output path: %s", outputPath)
	}
}

func queueJobTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}, &models.ExecutionPlan{}, &models.AppSetting{}, &models.SchedulerReservation{}, &models.WorkerNode{}); err != nil {
		t.Fatalf("migrate test models: %v", err)
	}
	return db
}
