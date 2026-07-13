package handlers

import (
	"testing"

	"github.com/anuelvs/mediaforge/backend/internal/models"
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
			MediaPath:  "/media/raw/Ranma 1-2 (1989)/Season 01/title01.mkv",
			BatchID:    "batch-ranma",
			BatchName:  "Ranma 1-2 (1989)/Season 01",
			LibraryID:  1,
			ProfileID: 1,
		},
		{
			MediaPath:  "/media/raw/Ranma 1-2 (1989)/Season 01/title02.mkv",
			BatchID:    "batch-ranma",
			BatchName:  "Ranma 1-2 (1989)/Season 01",
			LibraryID:  1,
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

func TestPlannedOutputPathForSingleJobKeepsSourceName(t *testing.T) {
	db := queueJobTestDB(t)
	library := models.Library{
		SourcePath:      "/media/raw",
		DestinationPath: "/media/anime",
	}
	profile := models.Profile{Container: "mkv"}
	job := models.QueueJob{
		MediaPath:  "/media/raw/Ranma 1-2 (1989)/Season 01/title01.mkv",
		BatchID:    "batch-single",
		BatchName:  "Ranma 1-2 (1989)/Season 01",
		LibraryID:  1,
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

func TestPlannedOutputPathDropsRawSourceBucketForSelectedLibrary(t *testing.T) {
	db := queueJobTestDB(t)
	library := models.Library{
		SourcePath:      "/media/raw",
		DestinationPath: "/media/library/anime",
	}
	profile := models.Profile{Container: "mkv"}
	job := models.QueueJob{
		MediaPath:  "/media/raw/series/Rurouni Kenshin/Trust & Betrayal Act 4.mkv",
		BatchID:    "batch-kenshin",
		BatchName:  "series/Rurouni Kenshin",
		LibraryID:  1,
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

func TestPlannedStagingOutputPathDropsRawSourceBucket(t *testing.T) {
	db := queueJobTestDB(t)
	library := models.Library{
		SourcePath:      "/media/raw",
		DestinationPath: "/media/library/movies",
	}
	profile := models.Profile{Container: "mkv"}
	job := models.QueueJob{
		ID:        7,
		MediaPath: "/media/raw/movies/Movie (1989)/feature.mp4",
		LibraryID: 1,
		ProfileID: 1,
	}

	outputPath := plannedStagingOutputPath(db, job, library, profile, pathSettings{rawRoot: "/media/raw", stagingPath: "/media/staging"})

	if outputPath != "/media/staging/job-7/Movie (1989)/feature.mkv" {
		t.Fatalf("unexpected staging output path: %s", outputPath)
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
		MediaPath:  "/media/raw/Movie (1989)/bonus.mkv",
		BatchID:    "batch-movie",
		BatchName:  "Movie (1989)",
		LibraryID:  1,
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
	if err := db.AutoMigrate(&models.QueueJob{}, &models.AppSetting{}); err != nil {
		t.Fatalf("migrate test models: %v", err)
	}
	return db
}
