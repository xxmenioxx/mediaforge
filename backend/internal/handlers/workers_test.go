package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"net/http"
	"net/http/httptest"

	"github.com/gin-gonic/gin"
)

func TestNextClaimableJobUsesExactQueuePosition(t *testing.T) {
	db := queueJobTestDB(t)
	jobs := []models.QueueJob{
		{MediaPath: "/raw/episode.mkv", Status: JobStatusQueued, Priority: 1, QueuePosition: 20},
		{MediaPath: "/raw/movie.mkv", Status: JobStatusQueued, Priority: 10, QueuePosition: 2},
	}
	if err := db.Create(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	claimed, err := nextClaimableJob(db, workerLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if claimed.ID != jobs[1].ID {
		t.Fatalf("claimed job %d, want queue-position leader %d", claimed.ID, jobs[1].ID)
	}
}

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

func TestResolveAutomaticFrameStructureUsesAssetSnapshot(t *testing.T) {
	db := queueJobTestDB(t)
	mediaPath := "/media/raw/anime/episode.mkv"
	scan := models.ScanResult{
		Path:         mediaPath,
		VideoStreams: models.JSONList{map[string]any{"avgFrameRate": "24000/1001"}},
		FrameStructureAnalysis: models.JSONMap{
			"averageGopLength":      72.0,
			"maxConsecutiveBFrames": 2,
			"confidence":            "high",
		},
	}
	if err := db.Create(&scan).Error; err != nil {
		t.Fatal(err)
	}
	original := models.Profile{WorkerConfig: models.JSONMap{"frameStructureMode": "auto", "frameStructureGopMode": "auto"}}
	effective, err := resolveAutomaticFrameStructure(db, mediaPath, original)
	if err != nil {
		t.Fatalf("resolve automatic frame structure: %v", err)
	}
	if got := workerStringValue(effective.WorkerConfig["frameStructureGopMode"]); got != "recommended" {
		t.Fatalf("effective GOP mode = %q, want recommended", got)
	}
	if got := workerIntValue(effective.WorkerConfig["frameStructureGopFrames"], 0); got != 90 {
		t.Fatalf("effective GOP = %d, want 90", got)
	}
	if got := workerIntValue(effective.WorkerConfig["frameStructureMaxBFrames"], 0); got != 2 {
		t.Fatalf("effective B-frame maximum = %d, want 2", got)
	}
	if !profileWorkerBool(effective, "qsvAdaptiveI", false) || !profileWorkerBool(effective, "qsvAdaptiveB", false) {
		t.Fatal("Auto should request Adaptive I and Adaptive B")
	}
	if got := workerStringValue(original.WorkerConfig["frameStructureGopMode"]); got != "auto" {
		t.Fatalf("requested profile was mutated: GOP mode = %q", got)
	}
}

func TestResolveAutomaticFrameStructureUsesResolvedCadenceFPS(t *testing.T) {
	db := queueJobTestDB(t)
	mediaPath := "/media/raw/dvd.mkv"
	scan := models.ScanResult{Path: mediaPath, VideoStreams: models.JSONList{map[string]any{"avgFrameRate": "30000/1001"}}, FrameStructureAnalysis: models.JSONMap{"averageGopLength": 72.0, "maxConsecutiveBFrames": 2, "confidence": "high"}}
	if err := db.Create(&scan).Error; err != nil {
		t.Fatal(err)
	}
	profile := models.Profile{WorkerConfig: models.JSONMap{"frameStructureMode": "auto", "effectiveOutputFrameRate": "24000/1001"}}
	effective, err := resolveAutomaticFrameStructure(db, mediaPath, profile)
	if err != nil {
		t.Fatal(err)
	}
	if got := workerIntValue(effective.WorkerConfig["frameStructureGopFrames"], 0); got != 90 {
		t.Fatalf("GOP used declared FPS instead of resolved 23.976: %d", got)
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

func TestWorkerClaimsBatchInListedOrderWithoutWaitingForCompletion(t *testing.T) {
	db := queueJobTestDB(t)
	base := time.Now()
	jobs := []models.QueueJob{
		{BatchID: "ordered-concurrent-batch", BatchPosition: 1, MediaPath: "/raw/02.mkv", LibraryID: 1, Priority: 5, Status: JobStatusQueued, CreatedAt: base},
		{BatchID: "ordered-concurrent-batch", BatchPosition: 2, MediaPath: "/raw/01.mkv", LibraryID: 1, Priority: 5, Status: JobStatusQueued, CreatedAt: base.Add(time.Millisecond)},
	}
	if err := db.Create(&jobs).Error; err != nil {
		t.Fatal(err)
	}
	first, err := nextClaimableJob(db, workerLimits{})
	if err != nil || first.BatchPosition != 1 {
		t.Fatalf("first claim=(%d,%v), want batch position 1", first.BatchPosition, err)
	}
	if err := db.Model(&models.QueueJob{}).Where("id = ?", first.ID).Update("status", JobStatusRunning).Error; err != nil {
		t.Fatal(err)
	}
	second, err := nextClaimableJob(db, workerLimits{})
	if err != nil || second.BatchPosition != 2 {
		t.Fatalf("concurrent second claim=(%d,%v), want batch position 2", second.BatchPosition, err)
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

func TestAutomaticFrameStructureSnapshotUsesValidCachedSnapshot(t *testing.T) {
	db := queueJobTestDB(t)

	mediaPath := "/media/raw/anime/cached.mkv"

	expected := models.ScanResult{
		Path:       mediaPath,
		VideoCodec: "h264",
		Width:      1920,
		Height:     1080,
		VideoStreams: models.JSONList{
			map[string]any{
				"avgFrameRate": "24000/1001",
			},
		},
		FrameStructureAnalysis: models.JSONMap{
			"version":          2,
			"framesAnalyzed":   500,
			"averageGopLength": 72.0,
			"confidence":       "high",
		},
	}

	if err := db.Create(&expected).Error; err != nil {
		t.Fatal(err)
	}

	got, err := automaticFrameStructureSnapshot(db, mediaPath)
	if err != nil {
		t.Fatalf("valid cached snapshot returned error: %v", err)
	}

	if got.ID != expected.ID {
		t.Fatalf("snapshot ID = %d, want %d", got.ID, expected.ID)
	}
}

func TestAutomaticFrameStructureSnapshotAttemptsScanWhenMissing(t *testing.T) {
	db := queueJobTestDB(t)

	mediaPath := filepath.Join(t.TempDir(), "missing-snapshot.mkv")

	// Intentionally invalid media. If the helper correctly tries to build
	// a missing snapshot, FFprobe should be reached and return an error.
	if err := os.WriteFile(mediaPath, []byte("not real media"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := automaticFrameStructureSnapshot(db, mediaPath)
	if err == nil {
		t.Fatal("missing AUTO snapshot must trigger media analysis instead of silently using fallback values")
	}
}

func TestResolveAutomaticFrameStructureFailsWithoutUsableSnapshot(t *testing.T) {
	db := queueJobTestDB(t)

	mediaPath := filepath.Join(t.TempDir(), "invalid-auto.mkv")

	if err := os.WriteFile(mediaPath, []byte("not real media"), 0o644); err != nil {
		t.Fatal(err)
	}

	profile := models.Profile{
		WorkerConfig: models.JSONMap{
			"frameStructureMode": "auto",
		},
	}

	effective, err := resolveAutomaticFrameStructure(
		db,
		mediaPath,
		profile,
	)

	if err == nil {
		t.Fatalf(
			"expected AUTO frame structure to fail without a usable snapshot, got profile %#v",
			effective,
		)
	}

	if resolved, ok := effective.WorkerConfig["frameStructureAutoResolved"].(bool); ok && resolved {
		t.Fatal("AUTO frame structure must not report itself as resolved after analysis failure")
	}

	if workerIntValue(effective.WorkerConfig["frameStructureGopFrames"], 0) == 120 {
		t.Fatal("AUTO frame structure must not silently fall back to GOP 120")
	}

	if workerIntValue(effective.WorkerConfig["frameStructureMaxBFrames"], 0) == 3 {
		t.Fatal("AUTO frame structure must not silently fall back to 3 B-frames")
	}
}

func TestDryRunReturnsUnprocessableEntityWhenAutoFrameStructureCannotResolve(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := queueJobTestDB(t)

	if err := db.AutoMigrate(
		&models.Library{},
		&models.Profile{},
	); err != nil {
		t.Fatal(err)
	}

	mediaPath := filepath.Join(t.TempDir(), "invalid-auto.mkv")
	if err := os.WriteFile(mediaPath, []byte("not real media"), 0o644); err != nil {
		t.Fatal(err)
	}

	library := models.Library{
		Name:            "Test Library",
		SourcePath:      filepath.Dir(mediaPath),
		DestinationPath: filepath.Join(t.TempDir(), "library"),
	}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}

	profile := models.Profile{
		Name:      "AUTO Test",
		Container: "mkv",
		WorkerConfig: models.JSONMap{
			"frameStructureMode": "auto",
		},
	}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}

	job := models.QueueJob{
		MediaPath: mediaPath,
		LibraryID: library.ID,
		ProfileID: profile.ID,
		Status:    JobStatusRunning,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{
		{Key: "id", Value: strconv.FormatUint(uint64(job.ID), 10)},
	}

	NewWorkerHandler(db).DryRun(ctx)

	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf(
			"DryRun status = %d, want %d; body=%s",
			recorder.Code,
			http.StatusUnprocessableEntity,
			recorder.Body.String(),
		)
	}

	if !strings.Contains(
		recorder.Body.String(),
		"automatic frame structure",
	) {
		t.Fatalf(
			"DryRun error should explain AUTO frame structure failure; body=%s",
			recorder.Body.String(),
		)
	}

	var stored models.QueueJob
	if err := db.First(&stored, job.ID).Error; err != nil {
		t.Fatal(err)
	}

	if strings.Contains(stored.Notes, "Dry-run command:") {
		t.Fatalf(
			"DryRun must not generate an FFmpeg command after AUTO resolution failure: %s",
			stored.Notes,
		)
	}
}

func TestExecuteQueueJobFailsWhenAutoFrameStructureCannotResolve(t *testing.T) {
	db := queueJobTestDB(t)

	if err := db.AutoMigrate(
		&models.Library{},
		&models.Profile{},
	); err != nil {
		t.Fatal(err)
	}

	mediaPath := filepath.Join(t.TempDir(), "invalid-auto-execute.mkv")
	if err := os.WriteFile(mediaPath, []byte("not real media"), 0o644); err != nil {
		t.Fatal(err)
	}

	library := models.Library{
		Name:            "Test Library",
		SourcePath:      filepath.Dir(mediaPath),
		DestinationPath: filepath.Join(t.TempDir(), "library"),
	}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}

	profile := models.Profile{
		Name:      "AUTO Execute Test",
		Container: "mkv",
		WorkerConfig: models.JSONMap{
			"frameStructureMode": "auto",
		},
	}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}

	job := models.QueueJob{
		MediaPath: mediaPath,
		LibraryID: library.ID,
		ProfileID: profile.ID,
		Status:    JobStatusRunning,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}

	handler := NewWorkerHandler(db)

	result, status, err := handler.executeQueueJob(job, false)

	if err == nil {
		t.Fatalf(
			"expected AUTO frame structure failure, got status=%d result=%#v",
			status,
			result,
		)
	}

	if status != http.StatusUnprocessableEntity {
		t.Fatalf(
			"executeQueueJob status = %d, want %d",
			status,
			http.StatusUnprocessableEntity,
		)
	}

	if !strings.Contains(
		strings.ToLower(err.Error()),
		"automatic frame structure",
	) {
		t.Fatalf(
			"execution error should explain AUTO frame structure failure: %v",
			err,
		)
	}

	if strings.Contains(result.Notes, "Conversion command:") {
		t.Fatalf(
			"FFmpeg must not start after AUTO resolution failure: %s",
			result.Notes,
		)
	}
}

func TestForceFreshSnapshotBeforeExecutionSetting(t *testing.T) {
	db := queueJobTestDB(t)

	if forceFreshSnapshotBeforeExecution(db) {
		t.Fatal("force fresh snapshot must default to false when the setting is absent")
	}

	setting := models.AppSetting{
		Key: "pipelineAutomation",
		Value: models.JSONMap{
			"forceFreshSnapshotBeforeExecution": true,
		},
	}

	if err := db.Create(&setting).Error; err != nil {
		t.Fatal(err)
	}

	if !forceFreshSnapshotBeforeExecution(db) {
		t.Fatal("force fresh snapshot must be enabled from pipelineAutomation settings")
	}
}

func TestRefreshSnapshotBeforeExecutionBypassesValidCachedSnapshot(t *testing.T) {
	db := queueJobTestDB(t)

	mediaPath := filepath.Join(t.TempDir(), "recovered-asset.mkv")

	// Archivo deliberadamente inválido.
	//
	// Si el helper reutiliza el snapshot existente, no habrá error.
	// Si realmente fuerza el análisis, llegará a ffprobe y fallará.
	if err := os.WriteFile(mediaPath, []byte("not real media"), 0o644); err != nil {
		t.Fatal(err)
	}

	existing := models.ScanResult{
		Path:       mediaPath,
		FileName:   filepath.Base(mediaPath),
		VideoCodec: "h264",
		Width:      1920,
		Height:     1080,
		VideoStreams: models.JSONList{
			map[string]any{
				"codec":        "h264",
				"avgFrameRate": "24000/1001",
			},
		},
		FrameStructureAnalysis: models.JSONMap{
			"version":               2,
			"framesAnalyzed":        1000,
			"averageGopLength":      120.0,
			"maxConsecutiveBFrames": 3,
			"confidence":            "high",
		},
	}

	if err := db.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}

	_, err := refreshSnapshotBeforeExecution(db, mediaPath)
	if err == nil {
		t.Fatal(
			"forced execution snapshot must analyze the physical file instead of reusing a valid cached snapshot",
		)
	}

	var stored models.ScanResult
	if err := db.
		Where("path = ?", mediaPath).
		Order("updated_at desc, id desc").
		First(&stored).
		Error; err != nil {
		t.Fatal(err)
	}

	// Because the forced scan failed, the existing valid snapshot should
	// still remain. We must not destroy the last known-good snapshot
	// before a replacement has successfully been generated.
	if stored.ID != existing.ID {
		t.Fatalf(
			"failed forced scan replaced the previous valid snapshot: got ID=%d want ID=%d",
			stored.ID,
			existing.ID,
		)
	}
}

func TestExecuteQueueJobForcesFreshSnapshotWhenSettingEnabled(t *testing.T) {
	db := queueJobTestDB(t)
	if err := db.AutoMigrate(
		&models.Library{},
		&models.Profile{},
	); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(t.TempDir(), "recovered-source.mkv")

	// Existe físicamente, pero deliberadamente no es media válida.
	if err := os.WriteFile(
		mediaPath,
		[]byte("not real media"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	library := models.Library{
		Name:            "Test",
		SourcePath:      filepath.Dir(mediaPath),
		DestinationPath: filepath.Join(t.TempDir(), "library"),
	}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}

	profile := authoritativeTestProfile()

	// No queremos que este test dependa de AUTO.
	// La política de snapshot debe funcionar para cualquier ejecución.
	if profile.WorkerConfig == nil {
		profile.WorkerConfig = models.JSONMap{}
	}
	profile.WorkerConfig["frameStructureMode"] = "manual"

	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Create(&models.AppSetting{
		Key: "pipelineAutomation",
		Value: models.JSONMap{
			"forceFreshSnapshotBeforeExecution": true,
		},
	}).Error; err != nil {
		t.Fatal(err)
	}

	// Snapshot antiguo pero completamente usable.
	oldSnapshot := models.ScanResult{
		Path:       mediaPath,
		FileName:   filepath.Base(mediaPath),
		VideoCodec: "h264",
		Width:      1920,
		Height:     1080,
		VideoStreams: models.JSONList{
			map[string]any{
				"codec":        "h264",
				"avgFrameRate": "24000/1001",
			},
		},
		FrameStructureAnalysis: models.JSONMap{
			"version":               2,
			"framesAnalyzed":        1000,
			"averageGopLength":      120.0,
			"maxConsecutiveBFrames": 3,
			"confidence":            "high",
		},
	}
	if err := db.Create(&oldSnapshot).Error; err != nil {
		t.Fatal(err)
	}

	job := models.QueueJob{
		MediaPath: mediaPath,
		LibraryID: library.ID,
		ProfileID: profile.ID,
		Status:    JobStatusRunning,
	}

	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}

	handler := NewWorkerHandler(db)

	_, status, err := handler.executeQueueJob(job, false)

	if err == nil {
		t.Fatal(
			"execution with forced fresh snapshot must analyze the physical file",
		)
	}

	if status != http.StatusUnprocessableEntity {
		t.Fatalf(
			"status=%d want=%d err=%v",
			status,
			http.StatusUnprocessableEntity,
			err,
		)
	}

	if !strings.Contains(
		strings.ToLower(err.Error()),
		"forced fresh snapshot",
	) {
		t.Fatalf(
			"error does not identify forced snapshot failure: %v",
			err,
		)
	}

	// El análisis nuevo falló, por lo que el snapshot anterior debe seguir.
	var stored models.ScanResult
	if err := db.
		Where("path = ?", mediaPath).
		Order("updated_at desc, id desc").
		First(&stored).
		Error; err != nil {
		t.Fatal(err)
	}

	if stored.ID != oldSnapshot.ID {
		t.Fatalf(
			"failed forced scan replaced old snapshot: got=%d want=%d",
			stored.ID,
			oldSnapshot.ID,
		)
	}
}

func TestSeasonFolderNumericAssetsUseOrdinalEpisodePosition(t *testing.T) {
	db := queueJobTestDB(t)

	library := models.Library{
		ID:              1,
		Name:            "Anime",
		SourcePath:      "/media/raw",
		DestinationPath: "/media/library/anime",
		ValidationRules: models.JSONMap{
			"episodeNamingEnabled": true,
		},
	}

	profile := models.Profile{
		Container: "mkv",
	}

	paths := []string{
		"/media/raw/My Show/Season2/21.mkv",
		"/media/raw/My Show/Season2/22.mkv",
		"/media/raw/My Show/Season2/23.mkv",
		"/media/raw/My Show/Season2/24.mkv",
		"/media/raw/My Show/Season2/45.mkv",
	}

	for _, mediaPath := range paths {
		record := models.AssetRecord{
			Path:         mediaPath,
			RootPath:     "/media/raw",
			RelativePath: strings.TrimPrefix(mediaPath, "/media/raw/"),
			GroupPath:    "My Show/Season2",
			FileName:     filepath.Base(mediaPath),
			Status:       "unprocessed",
		}

		if err := db.Create(&record).Error; err != nil {
			t.Fatal(err)
		}
	}

	for index, mediaPath := range paths {
		job := models.QueueJob{
			MediaPath: mediaPath,
			BatchID:   "season2-numeric",
			BatchName: "My Show/Season2",
			LibraryID: library.ID,
			ProfileID: 1,
		}

		if err := db.Create(&job).Error; err != nil {
			t.Fatal(err)
		}

		got := plannedOutputPathForJob(
			db,
			job,
			library,
			profile,
		)

		want := fmt.Sprintf(
			"/media/library/anime/My Show/Season2/My Show - S02E%02d.mkv",
			index+1,
		)

		if got != want {
			t.Fatalf(
				"%s output=%q want=%q",
				filepath.Base(mediaPath),
				got,
				want,
			)
		}
	}
}

func TestSeasonFolderOrdinalPositionWinsOverSourceTrackMetadata(t *testing.T) {
	db := queueJobTestDB(t)

	library := models.Library{
		ID:              1,
		Name:            "Anime",
		SourcePath:      "/media/raw",
		DestinationPath: "/media/library/anime",
		ValidationRules: models.JSONMap{
			"episodeNamingEnabled": true,
		},
	}

	profile := models.Profile{Container: "mkv"}

	paths := []string{
		"/media/raw/My Show/Season2/21.mkv",
		"/media/raw/My Show/Season2/22.mkv",
		"/media/raw/My Show/Season2/23.mkv",
	}

	for _, mediaPath := range paths {
		if err := db.Create(&models.AssetRecord{
			Path:         mediaPath,
			RootPath:     "/media/raw",
			RelativePath: strings.TrimPrefix(mediaPath, "/media/raw/"),
			GroupPath:    "My Show/Season2",
			FileName:     filepath.Base(mediaPath),
			Status:       "unprocessed",
		}).Error; err != nil {
			t.Fatal(err)
		}
	}

	// Simulates source metadata carrying the original disc/title number.
	if err := db.Create(&models.ScanResult{
		Path: paths[0],
		RawProbe: models.JSONMap{
			"format": map[string]interface{}{
				"tags": map[string]interface{}{
					"track": "21",
				},
			},
		},
	}).Error; err != nil {
		t.Fatal(err)
	}

	job := models.QueueJob{
		MediaPath: paths[0],
		BatchName: "My Show/Season2",
		LibraryID: library.ID,
	}

	got := plannedOutputPathForJob(
		db,
		job,
		library,
		profile,
	)

	want := "/media/library/anime/My Show/Season2/My Show - S02E01.mkv"

	if got != want {
		t.Fatalf(
			"season-folder ordinal output=%q want=%q",
			got,
			want,
		)
	}
}

func TestSeasonFolderOrdinalRejectsHistoricalEpisodeNumber(t *testing.T) {
	db := queueJobTestDB(t)

	library := models.Library{
		ID:              1,
		Name:            "Anime",
		SourcePath:      "/media/raw",
		DestinationPath: "/media/anime",
		ValidationRules: models.JSONMap{
			"episodeNamingEnabled": true,
		},
	}

	profile := models.Profile{
		Container: "mkv",
	}

	paths := []string{
		"/media/raw/My Show/Season2/21.mkv",
		"/media/raw/My Show/Season2/22.mkv",
		"/media/raw/My Show/Season2/23.mkv",
		"/media/raw/My Show/Season2/24.mkv",
		"/media/raw/My Show/Season2/45.mkv",
	}

	for _, mediaPath := range paths {
		if err := db.Create(&models.AssetRecord{
			Path:         mediaPath,
			RootPath:     "/media/raw",
			RelativePath: strings.TrimPrefix(mediaPath, "/media/raw/"),
			GroupPath:    "My Show/Season2",
			FileName:     filepath.Base(mediaPath),
			Status:       "unprocessed",
		}).Error; err != nil {
			t.Fatal(err)
		}
	}

	retiredAt := time.Now()

	previous := models.QueueJob{
		MediaPath:            paths[0],
		LibraryID:            library.ID,
		ProfileID:            1,
		Status:               JobStatusCompleted,
		PublishedPath:        "/media/anime/My Show/Season2/My Show - S02E21.mkv",
		PublicationRetiredAt: &retiredAt,
	}

	if err := db.Create(&previous).Error; err != nil {
		t.Fatal(err)
	}

	job := models.QueueJob{
		MediaPath: paths[0],
		BatchID:   "season2-retry",
		BatchName: "My Show/Season2",
		LibraryID: library.ID,
		ProfileID: 1,
	}

	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}

	got := plannedOutputPathForJob(
		db,
		job,
		library,
		profile,
	)

	want := "/media/anime/My Show/Season2/My Show - S02E01.mkv"

	if got != want {
		t.Fatalf(
			"season-folder historical output=%q want=%q",
			got,
			want,
		)
	}
}

func TestSeasonFolderEpisodePositionDoesNotShiftAfterEarlierPublish(t *testing.T) {
	db := queueJobTestDB(t)

	paths := []string{
		"/media/raw/Arbegas/Season1/Arbegas1.mkv",
		"/media/raw/Arbegas/Season1/Arbegas2.mkv",
		"/media/raw/Arbegas/Season1/Arbegas3.mkv",
	}

	for _, mediaPath := range paths {
		if err := db.Create(&models.AssetRecord{
			Path:         mediaPath,
			RootPath:     "/media/raw",
			RelativePath: strings.TrimPrefix(mediaPath, "/media/raw/"),
			GroupPath:    "Arbegas/Season1",
			FileName:     filepath.Base(mediaPath),
			Missing:      false,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}

	// Episode 1 was already published and archived.
	if err := db.Model(&models.AssetRecord{}).
		Where("path = ?", paths[0]).
		Update("missing", true).Error; err != nil {
		t.Fatal(err)
	}

	job := models.QueueJob{
		MediaPath:     paths[2],
		BatchID:       "arbegas-season1",
		BatchName:     "Arbegas/Season1",
		BatchPosition: 3,
	}

	season, episode, ok := seasonFolderEpisodePosition(db, job)

	if !ok {
		t.Fatal("expected season-folder episode position")
	}

	if season != 1 || episode != 3 {
		t.Fatalf(
			"got S%02dE%02d; want S01E03",
			season,
			episode,
		)
	}
}
