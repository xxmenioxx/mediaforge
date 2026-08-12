package handlers

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAssignExecutionNumberIgnoresQueuedPlaceholderIDs(t *testing.T) {
	db := queueJobTestDB(t)
	executedNumber := uint(4)
	executed := models.QueueJob{
		MediaPath:       "/media/raw/executed.mkv",
		Status:          JobStatusCompleted,
		ExecutionNumber: &executedNumber,
	}
	placeholder := models.QueueJob{MediaPath: "/media/raw/pending.mkv", Status: JobStatusQueued}
	for _, job := range []*models.QueueJob{&executed, &placeholder} {
		if err := db.Create(job).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := assignExecutionNumber(db, &placeholder); err != nil {
		t.Fatal(err)
	}
	if placeholder.ExecutionNumber == nil || *placeholder.ExecutionNumber != 5 {
		t.Fatalf("execution number = %#v, want 5", placeholder.ExecutionNumber)
	}
}

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

func TestPlannedOutputPathForSingleJobUsesFullAssetGroupEpisodePosition(t *testing.T) {
	db := queueJobTestDB(t)
	library := models.Library{SourcePath: "/media/raw", DestinationPath: "/media/anime", ValidationRules: models.JSONMap{"episodeNamingEnabled": true}}
	profile := models.Profile{Container: "mkv"}
	paths := []string{
		"/media/raw/Ranma 1-2 (1989)/Season 01/title01.mkv",
		"/media/raw/Ranma 1-2 (1989)/Season 01/title02.mkv",
		"/media/raw/Ranma 1-2 (1989)/Season 01/title03.mkv",
	}
	for _, mediaPath := range paths {
		record := models.AssetRecord{Path: mediaPath, RootPath: "/media/raw", RelativePath: strings.TrimPrefix(mediaPath, "/media/raw/"), GroupPath: "Ranma 1-2 (1989)/Season 01", FileName: filepath.Base(mediaPath), Status: "unprocessed"}
		if err := db.Create(&record).Error; err != nil {
			t.Fatal(err)
		}
	}
	job := models.QueueJob{MediaPath: paths[1], BatchID: "single-title02", BatchName: "Ranma 1-2 (1989)/Season 01", LibraryID: 1, ProfileID: 1}
	retiredAt := time.Now()
	previous := models.QueueJob{MediaPath: paths[1], LibraryID: 1, ProfileID: 1, Status: JobStatusCompleted, PublishedPath: "/media/anime/Ranma 1-2 (1989)/Season 01/title02.mkv", PublicationRetiredAt: &retiredAt}
	if err := db.Create(&previous).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	got := plannedOutputPathForJob(db, job, library, profile)
	want := "/media/anime/Ranma 1-2 (1989)/Season 01/Ranma 1-2 (1989) - S01E02.mkv"
	if got != want {
		t.Fatalf("single-job output=%q want=%q", got, want)
	}
}

func TestPlannedOutputPathUsesNaturalInventoryOrderForPartialBatch(t *testing.T) {
	db := queueJobTestDB(t)
	library := models.Library{SourcePath: "/media/raw", DestinationPath: "/media/anime", ValidationRules: models.JSONMap{"episodeNamingEnabled": true}}
	profile := models.Profile{Container: "mkv"}
	paths := []string{
		"/media/raw/My Show/Season 01/episode1.mkv",
		"/media/raw/My Show/Season 01/episode2.mkv",
		"/media/raw/My Show/Season 01/episode3.mkv",
		"/media/raw/My Show/Season 01/episode10.mkv",
	}
	for _, mediaPath := range paths {
		record := models.AssetRecord{Path: mediaPath, RootPath: "/media/raw", RelativePath: strings.TrimPrefix(mediaPath, "/media/raw/"), GroupPath: "My Show/Season 01", FileName: filepath.Base(mediaPath), Status: "unprocessed"}
		if err := db.Create(&record).Error; err != nil {
			t.Fatal(err)
		}
	}
	jobs := []models.QueueJob{
		{MediaPath: paths[0], BatchID: "partial", BatchName: "My Show/Season 01", LibraryID: 1, ProfileID: 1},
		{MediaPath: paths[2], BatchID: "partial", BatchName: "My Show/Season 01", LibraryID: 1, ProfileID: 1},
	}
	if err := db.Create(&jobs).Error; err != nil {
		t.Fatal(err)
	}

	if got := plannedOutputPathForJob(db, jobs[1], library, profile); got != "/media/anime/My Show/Season 01/My Show - S01E03.mkv" {
		t.Fatalf("partial-batch episode output=%q", got)
	}
	if episode := episodeNumberFromAssetGroup(db, paths[3]); episode != 4 {
		t.Fatalf("natural episode position=%d want=4", episode)
	}
}

func TestExplicitEpisodeIdentifierWinsOverInventoryPosition(t *testing.T) {
	db := queueJobTestDB(t)
	mediaPath := "/media/raw/Series/Season 02/Series S02E07.mkv"
	record := models.AssetRecord{Path: mediaPath, RootPath: "/media/raw", RelativePath: "Series/Season 02/Series S02E07.mkv", GroupPath: "Series/Season 02", FileName: filepath.Base(mediaPath), Status: "unprocessed"}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{MediaPath: mediaPath, BatchName: "Series/Season 02"}
	spec, ok := multiEpisodeNameSpecForJob(db, job)
	if !ok || spec.Season != 2 || spec.Episode != 7 {
		t.Fatalf("explicit episode spec=%#v ok=%t", spec, ok)
	}
}

func TestEmbeddedEpisodeMetadataWinsOverBatchPosition(t *testing.T) {
	db := queueJobTestDB(t)
	mediaPath := "/media/raw/My Show/source.mkv"
	scan := models.ScanResult{Path: mediaPath, RawProbe: models.JSONMap{"format": map[string]interface{}{"tags": map[string]interface{}{"season_number": "3", "episode_id": "12/24"}}}}
	if err := db.Create(&scan).Error; err != nil {
		t.Fatal(err)
	}
	spec, ok := multiEpisodeNameSpecForJob(db, models.QueueJob{MediaPath: mediaPath, BatchName: "My Show"})
	if !ok || spec.Season != 3 || spec.Episode != 12 {
		t.Fatalf("metadata episode spec=%#v ok=%t", spec, ok)
	}
}

func TestEpisodeVideoTrackTitleUsesOriginalAssetName(t *testing.T) {
	db := queueJobTestDB(t)
	mediaPath := "/media/raw/Series/Season 01/original_episode_03.mkv"
	record := models.AssetRecord{Path: mediaPath, RootPath: "/media/raw", RelativePath: "Series/Season 01/original_episode_03.mkv", GroupPath: "Series/Season 01", FileName: filepath.Base(mediaPath), Status: "unprocessed"}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	// A sibling makes episode naming applicable even when only this asset is queued.
	sibling := models.AssetRecord{Path: "/media/raw/Series/Season 01/original_episode_01.mkv", RootPath: "/media/raw", RelativePath: "Series/Season 01/original_episode_01.mkv", GroupPath: "Series/Season 01", FileName: "original_episode_01.mkv", Status: "unprocessed"}
	if err := db.Create(&sibling).Error; err != nil {
		t.Fatal(err)
	}
	plan := MediaJobPlan{
		InputPath: "/tmp/input.mkv", OutputPath: "/tmp/output.mkv",
		Profile: models.Profile{VideoCodec: "copy", AudioCodec: "copy", Container: "mkv"},
		Streams: MediaStreamInventory{Video: []MediaStream{{Index: 0}}},
	}
	job := models.QueueJob{MediaPath: mediaPath, BatchName: "Series/Season 01"}
	applyEpisodeVideoTrackTitle(db, &plan, job, models.Library{ValidationRules: models.JSONMap{"episodeNamingEnabled": true}})
	if got := plan.Override.VideoMetadata[0].Title; got != "original_episode_03" {
		t.Fatalf("video title=%q", got)
	}
	command := strings.Join(FFmpegCommandBuilder{}.Build(plan), " ")
	if !strings.Contains(command, "-metadata:s:v:0 title=original_episode_03") {
		t.Fatalf("episode video title missing from command: %s", command)
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

func TestPlannedOutputPathPrefersOlderNormalizedNameOverLatestSourceName(t *testing.T) {
	db := queueJobTestDB(t)
	library := models.Library{
		ID:              1,
		SourcePath:      "/media/raw",
		DestinationPath: "/media/library/anime",
		ValidationRules: models.JSONMap{"episodeNamingEnabled": true},
	}
	profile := models.Profile{Container: "mkv"}
	retiredAt := time.Now()
	mediaPath := "/media/raw/anime/Baccano/Baccano! S01E01 MULTi 1080p 10bits BluRay x265 AAC -Punisher694.mkv"
	history := []models.QueueJob{
		{
			MediaPath:            mediaPath,
			LibraryID:            library.ID,
			ProfileID:            1,
			Status:               JobStatusCompleted,
			PublishedPath:        "/media/library/anime/Baccano/Baccano - S01E01.mkv",
			PublicationRetiredAt: &retiredAt,
		},
		{
			MediaPath:            mediaPath,
			LibraryID:            library.ID,
			ProfileID:            1,
			Status:               JobStatusCompleted,
			PublishedPath:        "/media/library/anime/Baccano/Baccano! S01E01 MULTi 1080p 10bits BluRay x265 AAC -Punisher694.mkv",
			PublicationRetiredAt: &retiredAt,
		},
	}
	if err := db.Create(&history).Error; err != nil {
		t.Fatalf("create job history: %v", err)
	}
	reconversion := models.QueueJob{
		MediaPath: mediaPath,
		BatchID:   "single-recovered-asset",
		BatchName: "anime/Baccano",
		LibraryID: library.ID,
		ProfileID: 1,
	}
	if err := db.Create(&reconversion).Error; err != nil {
		t.Fatalf("create reconversion job: %v", err)
	}

	got := plannedOutputPathForJob(db, reconversion, library, profile)
	want := "/media/library/anime/Baccano/Baccano - S01E01.mkv"
	if got != want {
		t.Fatalf("reconversion output=%q want=%q", got, want)
	}
}

func TestPlannedOutputPathNormalizesSourceBucketFromRecoveredHistory(t *testing.T) {
	db := queueJobTestDB(t)
	library := models.Library{ID: 1, Name: "Anime", SourcePath: "/media/raw", DestinationPath: "/media/library/anime"}
	profile := models.Profile{Container: "mkv"}
	retiredAt := time.Now()
	mediaPath := "/media/raw/anime/Conan el niño del futuro/Conan, el niño del futuro - 01_(Ladyarmaroid).mp4"
	previous := models.QueueJob{
		MediaPath: mediaPath, LibraryID: library.ID, ProfileID: 1, Status: JobStatusCompleted,
		PublishedPath:        "/media/library/anime/anime/Conan el niño del futuro/Conan, el niño del futuro - 01_(Ladyarmaroid).mkv",
		PublicationRetiredAt: &retiredAt,
	}
	if err := db.Create(&previous).Error; err != nil {
		t.Fatal(err)
	}
	recovered := models.QueueJob{MediaPath: mediaPath, LibraryID: library.ID, ProfileID: 1}
	if err := db.Create(&recovered).Error; err != nil {
		t.Fatal(err)
	}

	got := plannedOutputPathForJob(db, recovered, library, profile)
	want := "/media/library/anime/Conan el niño del futuro/Conan, el niño del futuro - 01_(Ladyarmaroid).mkv"
	if got != want {
		t.Fatalf("recovered output=%q want=%q", got, want)
	}
}

func TestPartialRecoveredBatchUsesLeadingEpisodeNumberInsteadOfRemainingRawOrder(t *testing.T) {
	db := queueJobTestDB(t)
	library := models.Library{
		ID: 1, Name: "Anime", SourcePath: "/media/raw", DestinationPath: "/media/library/anime",
		ValidationRules: models.JSONMap{"episodeNamingEnabled": true},
	}
	profile := models.Profile{Container: "mkv"}
	job := models.QueueJob{
		MediaPath: "/media/raw/anime/Rayearth/02-La llegada de Presea.mkv",
		BatchID:   "partial-recovered-rayearth", BatchName: "anime/Rayearth", LibraryID: library.ID,
	}
	if err := db.Create(&models.AssetRecord{
		Path: job.MediaPath, RootPath: "/media/raw", RelativePath: "anime/Rayearth/02-La llegada de Presea.mkv",
		GroupPath: "anime/Rayearth", FileName: "02-La llegada de Presea.mkv", Status: "unprocessed",
	}).Error; err != nil {
		t.Fatal(err)
	}

	got := plannedOutputPathForJob(db, job, library, profile)
	want := "/media/library/anime/Rayearth/Rayearth - S01E02.mkv"
	if got != want {
		t.Fatalf("partial recovered batch output=%q want=%q", got, want)
	}
}

func TestRecoveredAssetRejectsHistoricalEpisodeNameThatContradictsSourceNumber(t *testing.T) {
	db := queueJobTestDB(t)
	library := models.Library{
		ID: 1, Name: "Anime", SourcePath: "/media/raw", DestinationPath: "/media/library/anime",
		ValidationRules: models.JSONMap{"episodeNamingEnabled": true},
	}
	profile := models.Profile{Container: "mkv"}
	mediaPath := "/media/raw/anime/Rayearth/02-La llegada de Presea.mkv"
	retiredAt := time.Now()
	previous := models.QueueJob{
		MediaPath: mediaPath, LibraryID: library.ID, Status: JobStatusCompleted,
		PublishedPath: "/media/library/anime/Rayearth/Rayearth - S01E01.mkv", PublicationRetiredAt: &retiredAt,
	}
	if err := db.Create(&previous).Error; err != nil {
		t.Fatal(err)
	}
	recovered := models.QueueJob{MediaPath: mediaPath, BatchID: "retry-rayearth", BatchName: "anime/Rayearth", LibraryID: library.ID}
	if err := db.Create(&recovered).Error; err != nil {
		t.Fatal(err)
	}

	got := plannedOutputPathForJob(db, recovered, library, profile)
	want := "/media/library/anime/Rayearth/Rayearth - S01E02.mkv"
	if got != want {
		t.Fatalf("recovered retry output=%q want=%q", got, want)
	}
}

func TestLeadingEpisodeNumberFromNameRequiresAnExplicitSeparator(t *testing.T) {
	tests := []struct {
		name string
		want int
		ok   bool
	}{
		{name: "02-La llegada de Presea.mkv", want: 2, ok: true},
		{name: "003 Episode.mkv", want: 3, ok: true},
		{name: "12_Episode.mkv", want: 12, ok: true},
		{name: "1984.mkv", ok: false},
		{name: "1080p encode.mkv", ok: false},
		{name: "Episode 02.mkv", ok: false},
	}
	for _, test := range tests {
		got, ok := leadingEpisodeNumberFromName(test.name)
		if got != test.want || ok != test.ok {
			t.Errorf("leadingEpisodeNumberFromName(%q)=(%d,%t), want (%d,%t)", test.name, got, ok, test.want, test.ok)
		}
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
	if err := db.AutoMigrate(&models.QueueJob{}, &models.ExecutionPlan{}, &models.AppSetting{}, &models.SchedulerReservation{}, &models.WorkerNode{}, &models.AssetRecord{}, &models.ScanResult{}); err != nil {
		t.Fatalf("migrate test models: %v", err)
	}
	return db
}
