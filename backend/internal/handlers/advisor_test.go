package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
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

func TestAdvisorProvenanceUsesMVForgeJobAndIgnoresRetiredPublication(t *testing.T) {
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
	if !handler.hasMVForgeConversion(path) {
		t.Fatal("published MVForge job was not recognized")
	}
	now := time.Now()
	job.PublicationRetiredAt = &now
	if err := db.Save(&job).Error; err != nil {
		t.Fatal(err)
	}
	if handler.hasMVForgeConversion(path) {
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

func TestAdvisorTreatsExternalSubtitleConversionAsPreservation(t *testing.T) {
	for _, format := range []string{"srt", "ass"} {
		profile := models.Profile{
			VideoCodec: "x265", AudioCodec: "copy", PreserveSubtitles: false,
			WorkerConfig: models.JSONMap{"externalSubtitleFormat": format},
		}
		response := evaluateConversion(models.ScanResult{VideoCodec: "h264", SubtitleTracks: 2}, profile, false)
		for _, warning := range response.Warnings {
			if strings.Contains(warning, "does not preserve subtitles") {
				t.Fatalf("%s externalization produced a false preservation warning: %#v", format, response.Warnings)
			}
		}
		found := false
		for _, reason := range response.Reasons {
			if strings.Contains(reason, strings.ToUpper(format)) && strings.Contains(reason, "sidecars") {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s externalization was not explained: %#v", format, response.Reasons)
		}
	}
}

func TestAdvisorWarnsOnlyWhenSubtitlePolicyReallyRemovesSubtitles(t *testing.T) {
	profile := models.Profile{
		VideoCodec: "x265", AudioCodec: "copy", PreserveSubtitles: true,
		WorkerConfig: models.JSONMap{"externalSubtitleFormat": "remove"},
	}
	response := evaluateConversion(models.ScanResult{VideoCodec: "h264", SubtitleTracks: 1}, profile, false)
	if len(response.Warnings) == 0 || !strings.Contains(response.Warnings[0], "embedded or external form") {
		t.Fatalf("explicit subtitle removal must be reported: %#v", response.Warnings)
	}
}

func TestProfileFitAcceptsExternalSubtitlePreservation(t *testing.T) {
	scan := models.ScanResult{SubtitleTracks: 1}
	proposal := proposedProfileForScan(scan)
	profile := models.Profile{
		VideoCodec: "x265", CodecFamily: "hevc", Container: "mkv", QualityMode: "crf",
		QualityValue: proposal.QualityValue, BitDepth: 10,
		WorkerConfig: models.JSONMap{"externalSubtitleFormat": "srt"},
	}
	score, reasons := profileFit(profile, proposal, scan)
	if score != 100 {
		t.Fatalf("external subtitle preservation must not reduce profile fit: score=%d reasons=%#v", score, reasons)
	}
}
