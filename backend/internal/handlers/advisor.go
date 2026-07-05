package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AdvisorHandler struct {
	db *gorm.DB
}

type AdvisorRequest struct {
	MediaPath string `json:"mediaPath" binding:"required"`
	ProfileID uint   `json:"profileId" binding:"required"`
}

type AdvisorResponse struct {
	Recommendation string            `json:"recommendation"`
	Score          int               `json:"score"`
	Summary        string            `json:"summary"`
	Reasons        []string          `json:"reasons"`
	Warnings       []string          `json:"warnings"`
	Estimated      AdvisorEstimation `json:"estimated"`
	Scan           models.ScanResult `json:"scan"`
	Profile        models.Profile    `json:"profile"`
}

type AdvisorEstimation struct {
	CurrentSizeBytes int64  `json:"currentSizeBytes"`
	TargetCodec      string `json:"targetCodec"`
	TargetContainer  string `json:"targetContainer"`
}

func NewAdvisorHandler(db *gorm.DB) AdvisorHandler {
	return AdvisorHandler{db: db}
}

func (h AdvisorHandler) Evaluate(c *gin.Context) {
	var request AdvisorRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var profile models.Profile
	if err := h.db.First(&profile, request.ProfileID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
		return
	}

	scan, err := h.scanForPath(request.MediaPath)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	response := evaluateConversion(scan, profile)
	c.JSON(http.StatusOK, response)
}

func (h AdvisorHandler) scanForPath(path string) (models.ScanResult, error) {
	var existing models.ScanResult
	if err := h.db.Where("path = ?", path).Order("created_at desc").First(&existing).Error; err == nil {
		return existing, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return models.ScanResult{}, fmt.Errorf("media path is not readable from the backend container")
	}

	if info.IsDir() {
		return models.ScanResult{}, fmt.Errorf("media path must point to a file")
	}

	probe, raw, err := runFFProbe(path)
	if err != nil {
		return models.ScanResult{}, err
	}

	scan := buildScanResult(path, info.Size(), probe, raw)
	if err := h.db.Create(&scan).Error; err != nil {
		return models.ScanResult{}, err
	}

	return scan, nil
}

func evaluateConversion(scan models.ScanResult, profile models.Profile) AdvisorResponse {
	score := 50
	reasons := []string{}
	warnings := []string{}

	sourceCodec := strings.ToLower(scan.VideoCodec)
	targetCodec := strings.ToLower(profile.VideoCodec)

	if targetCodec == "copy" {
		score -= 15
		reasons = append(reasons, "The selected profile copies video, so it will not reduce video bitrate or change compatibility.")
	} else if codecMatches(sourceCodec, targetCodec) {
		score -= 20
		reasons = append(reasons, "The source video codec already matches the selected profile.")
	} else {
		score += 20
		reasons = append(reasons, fmt.Sprintf("The profile would convert video from %s to %s.", fallback(sourceCodec, "unknown"), profile.VideoCodec))
	}

	if scan.Bitrate > 8_000_000 && targetCodec != "copy" {
		score += 15
		reasons = append(reasons, "The source bitrate is high enough that conversion may reduce storage usage.")
	}

	if scan.Width >= 3840 || scan.Height >= 2160 {
		score += 5
		reasons = append(reasons, "The asset appears to be 4K, so codec and HDR preservation matter.")
	}

	if scan.HDR && profile.PreserveHDR {
		score += 10
		reasons = append(reasons, "HDR was detected and the selected profile preserves HDR.")
	}

	if scan.HDR && !profile.PreserveHDR {
		score -= 30
		warnings = append(warnings, "HDR was detected but the selected profile does not preserve HDR.")
	}

	if scan.SubtitleTracks > 0 && profile.PreserveSubtitles {
		reasons = append(reasons, "Subtitle tracks are present and the selected profile preserves subtitles.")
	}

	if scan.SubtitleTracks > 0 && !profile.PreserveSubtitles {
		score -= 10
		warnings = append(warnings, "Subtitle tracks are present but the selected profile does not preserve subtitles.")
	}

	if scan.AudioTracks > 1 && strings.ToLower(profile.AudioCodec) != "copy" {
		warnings = append(warnings, "Multiple audio tracks are present; verify the profile keeps the tracks you care about.")
	}

	score = clamp(score, 0, 100)
	recommendation := recommendationForScore(score)

	return AdvisorResponse{
		Recommendation: recommendation,
		Score:          score,
		Summary:        summaryForRecommendation(recommendation),
		Reasons:        reasons,
		Warnings:       warnings,
		Estimated: AdvisorEstimation{
			CurrentSizeBytes: scan.SizeBytes,
			TargetCodec:      profile.VideoCodec,
			TargetContainer:  profile.Container,
		},
		Scan:    scan,
		Profile: profile,
	}
}

func codecMatches(source string, target string) bool {
	switch target {
	case "x264":
		return source == "h264"
	case "x265":
		return source == "hevc" || source == "h265"
	default:
		return source == target
	}
}

func recommendationForScore(score int) string {
	if score >= 70 {
		return "worth_it"
	}
	if score >= 45 {
		return "maybe"
	}
	return "not_recommended"
}

func summaryForRecommendation(recommendation string) string {
	switch recommendation {
	case "worth_it":
		return "This conversion looks worth considering."
	case "maybe":
		return "This conversion may be useful, but review the tradeoffs first."
	default:
		return "This conversion is not recommended with the selected profile."
	}
}

func fallback(value string, fallbackValue string) string {
	if value == "" {
		return fallbackValue
	}
	return value
}

func clamp(value int, minValue int, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func advisorQueueNote(response AdvisorResponse) string {
	return fmt.Sprintf("Advisor: %s (%d/100) for %s", response.Recommendation, response.Score, filepath.Base(response.Scan.Path))
}
