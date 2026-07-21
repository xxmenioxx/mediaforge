package handlers

import (
	"testing"

	"github.com/anuelvs/mediaforge/backend/internal/models"
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

func TestLibraryAssetAdvisorIsConservative(t *testing.T) {
	response := evaluateConversion(
		models.ScanResult{VideoCodec: "hevc", Bitrate: 20_000_000, Width: 3840, Height: 2160},
		models.Profile{VideoCodec: "x265_10bit", CodecFamily: "hevc", AudioCodec: "copy"},
		true,
	)
	if response.Score != 0 || response.Recommendation != "not_recommended" {
		t.Fatalf("unexpected library recommendation: score=%d recommendation=%s", response.Score, response.Recommendation)
	}
	if len(response.Reasons) == 0 || len(response.Warnings) == 0 {
		t.Fatal("library recommendation must explain generational quality risk")
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
