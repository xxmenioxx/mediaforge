package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCodecMatchesTreatsHEVCEncoderLabelsAsSameFamily(t *testing.T) {
	for _, target := range []string{"hevc", "h265", "x265_10bit", "libx265", "hevc_qsv", "hevc_videotoolbox"} {
		if !codecMatches("hevc", target) {
			t.Fatalf("hevc should match %q", target)
		}
	}
	if codecMatches("hevc", "x264") {
		t.Fatal("hevc must not match h264")
	}
}

func TestAdvisorProvenanceUsesMediaForgeJobAndIgnoresRetiredPublication(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:advisor-provenance?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.QueueJob{}); err != nil {
		t.Fatal(err)
	}
	path := "/library/movie.mkv"
	job := models.QueueJob{MediaPath: "/raw/movie.mkv", LibraryID: 1, ProfileID: 1, Status: JobStatusCompleted, PublishedPath: path}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	handler := NewAdvisorHandler(db)
	if !handler.hasMediaForgeConversion(path) {
		t.Fatal("published MediaForge job was not recognized")
	}
	now := time.Now()
	job.PublicationRetiredAt = &now
	if err := db.Save(&job).Error; err != nil {
		t.Fatal(err)
	}
	if handler.hasMediaForgeConversion(path) {
		t.Fatal("retired publication must not count as current conversion provenance")
	}
}

func TestPreviouslyConvertedAssetIsRejectedOnlyWhenAnalysisShowsNoAdvantage(t *testing.T) {
	response := evaluateConversion(
		models.ScanResult{VideoCodec: "hevc", Bitrate: 20_000_000, Width: 3840, Height: 2160},
		models.Profile{VideoCodec: "x265_10bit", CodecFamily: "hevc", AudioCodec: "copy"},
		true,
	)
	if response.Score <= 0 || response.Recommendation != "not_recommended" {
		t.Fatalf("unexpected prior-conversion recommendation: score=%d recommendation=%s", response.Score, response.Recommendation)
	}
	if len(response.Reasons) == 0 || len(response.Warnings) == 0 {
		t.Fatal("library recommendation must explain generational quality risk")
	}
}

func TestUnverifiedLibraryAssetCanBeRecommendedFromTechnicalEvidence(t *testing.T) {
	response := evaluateConversion(
		models.ScanResult{VideoCodec: "h264", Bitrate: 12_000_000, Width: 1920, Height: 1080},
		models.Profile{VideoCodec: "x265_10bit", CodecFamily: "hevc", AudioCodec: "copy"},
		false,
	)
	if response.Recommendation != "worth_it" || response.Score < 70 {
		t.Fatalf("unverified Library asset should be judged on evidence: score=%d recommendation=%s", response.Score, response.Recommendation)
	}
	for _, reason := range response.Reasons {
		if strings.Contains(reason, "already lives in Library") {
			t.Fatalf("location-only assumption remains in reasons: %q", reason)
		}
	}
}

func TestProposedProfileUsesDetectedPreservationNeedsAndContinuousCRF(t *testing.T) {
	proposal := proposedProfileForScan(models.ScanResult{
		VideoCodec: "mpeg2video", Width: 720, Height: 480,
		SubtitleTracks: 2, Chapters: 12,
	})
	if proposal.QualityValue != 21 {
		t.Fatalf("expected DVD CRF 21, got %d", proposal.QualityValue)
	}
	if proposal.PreserveHDR || !proposal.PreserveSubtitles || !proposal.PreserveChapters {
		t.Fatalf("proposal preservation does not match detected source: %+v", proposal)
	}
}

func TestProfileFitRejectsMissingRequiredPreservation(t *testing.T) {
	scan := models.ScanResult{HDR: true, SubtitleTracks: 1, Chapters: 1}
	proposal := proposedProfileForScan(scan)
	profile := models.Profile{VideoCodec: "x265", CodecFamily: "hevc", Container: "mkv", QualityMode: "crf", QualityValue: proposal.QualityValue, BitDepth: 10}
	score, _ := profileFit(profile, proposal, scan)
	if score >= 80 {
		t.Fatalf("profile missing HDR/subtitle/chapter preservation must not be suggested, got %d", score)
	}
}
