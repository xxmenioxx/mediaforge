package handlers

import (
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
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

type ProfileSuggestionRequest struct {
	MediaPath string `json:"mediaPath" binding:"required"`
}

type ProfileCandidate struct {
	Profile models.Profile `json:"profile"`
	Score   int            `json:"score"`
	Reasons []string       `json:"reasons"`
}

type ProfileSuggestionResponse struct {
	MatchType        string             `json:"matchType"`
	Summary          string             `json:"summary"`
	Scan             models.ScanResult  `json:"scan"`
	SuggestedProfile *models.Profile    `json:"suggestedProfile,omitempty"`
	Candidates       []ProfileCandidate `json:"candidates"`
	ProposedProfile  ProfileInput       `json:"proposedProfile"`
	Insights         AnalysisInsights   `json:"insights"`
}

type AnalysisInsights struct {
	RecommendedCRF       int      `json:"recommendedCrf"`
	EstimatedMinBytes    int64    `json:"estimatedMinBytes"`
	EstimatedMaxBytes    int64    `json:"estimatedMaxBytes"`
	EstimatedSavingsLow  int      `json:"estimatedSavingsLow"`
	EstimatedSavingsHigh int      `json:"estimatedSavingsHigh"`
	Recommendations      []string `json:"recommendations"`
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

	response := evaluateConversion(scan, profile, h.hasMVForgeConversion(request.MediaPath))
	c.JSON(http.StatusOK, response)
}

func (h AdvisorHandler) Suggest(c *gin.Context) {
	var request ProfileSuggestionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	scan, err := h.scanForPath(request.MediaPath)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}

	proposal := proposedProfileForScan(scan)
	var profiles []models.Profile
	if err := h.db.Where("disabled = ? OR disabled IS NULL", false).Order("name asc").Find(&profiles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	candidates := make([]ProfileCandidate, 0, len(profiles))
	for _, profile := range profiles {
		score, reasons := profileFit(profile, proposal, scan)
		candidates = append(candidates, ProfileCandidate{Profile: profile, Score: score, Reasons: reasons})
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Score > candidates[j].Score })
	if len(candidates) > 3 {
		candidates = candidates[:3]
	}

	response := ProfileSuggestionResponse{MatchType: "create", Summary: "No existing profile covers the detected technical requirements closely enough.", Scan: scan, Candidates: candidates, ProposedProfile: proposal}
	if len(candidates) > 0 && candidates[0].Score >= 80 {
		response.MatchType = "existing"
		response.SuggestedProfile = &candidates[0].Profile
		response.Summary = fmt.Sprintf("%s is the closest existing profile for this source.", candidates[0].Profile.Name)
	}
	targetCRF := proposal.QualityValue
	if response.SuggestedProfile != nil && response.SuggestedProfile.QualityValue > 0 {
		targetCRF = response.SuggestedProfile.QualityValue
	}
	response.Insights = analysisInsights(scan, targetCRF)
	c.JSON(http.StatusOK, response)
}

func analysisInsights(scan models.ScanResult, crf int) AnalysisInsights {
	videoKbps := 3500.0
	switch {
	case scan.Width >= 3840 || scan.Height >= 2160:
		videoKbps = 12000
	case scan.Width <= 720 && scan.Height <= 576:
		videoKbps = 1200
	case scan.Width <= 1280 && scan.Height <= 720:
		videoKbps = 2200
	}
	videoKbps *= math.Pow(2, float64(21-crf)/6.0)
	totalKbps := videoKbps + 512
	center := int64(scan.Duration * totalKbps * 1000 / 8)
	minBytes, maxBytes := int64(float64(center)*0.75), int64(float64(center)*1.35)
	low, high := 0, 0
	if scan.SizeBytes > 0 {
		low = int(math.Round((1 - float64(maxBytes)/float64(scan.SizeBytes)) * 100))
		high = int(math.Round((1 - float64(minBytes)/float64(scan.SizeBytes)) * 100))
	}
	recommendations := []string{fmt.Sprintf("CRF %d is the technical starting point for this resolution; validate a representative motion-heavy scene before processing the full asset.", crf)}
	if codecFamily(scan.VideoCodec) == "hevc" {
		recommendations = append(recommendations, "The source is already HEVC. Re-encoding may lose detail and may not reduce size; prefer video copy unless there is a verified correction goal.")
	}
	if status, _ := scan.InterlaceAnalysis["status"].(string); status == "progressive" {
		recommendations = append(recommendations, "The analyzed window is progressive; no deinterlacing filter is recommended.")
	} else if status == "interlaced" {
		recommendations = append(recommendations, "Interlacing was detected. Apply bwdif before encoding and inspect motion in a preview.")
	} else if status == "mixed" {
		recommendations = append(recommendations, "Mixed progressive/interlaced frames were detected. Validate a motion-heavy preview before deciding whether to force deinterlacing.")
	} else if status == "telecine_suspected" {
		recommendations = append(recommendations, "Telecine cadence is suspected. Validate fieldmatch/decimate in LAB instead of applying ordinary deinterlacing blindly.")
	} else {
		recommendations = append(recommendations, "Motion structure could not be classified reliably; inspect a preview before conversion.")
	}
	if scan.HDR {
		recommendations = append(recommendations, "Preserve HDR metadata and 10-bit output; otherwise highlights and color may change.")
	}
	if status, _ := scan.CropAnalysis["status"].(string); status == "detected" {
		crop, _ := scan.CropAnalysis["recommendedCrop"].(string)
		confidence, _ := scan.CropAnalysis["confidence"].(float64)
		recommendations = append(recommendations, fmt.Sprintf("Stable black bars were detected. LAB can preview crop=%s (%.0f%% confidence); verify subtitles and framing before saving.", crop, confidence*100))
	} else if status == "variable" {
		crop, _ := scan.CropAnalysis["recommendedCrop"].(string)
		if crop != "" {
			recommendations = append(recommendations, fmt.Sprintf("Black borders vary slightly across sampled scenes. LAB can preview the conservative candidate crop=%s, but it remains disabled by default and requires multi-scene review.", crop))
		} else {
			recommendations = append(recommendations, "Black borders vary across sampled scenes. Do not crop automatically; inspect multiple LAB previews.")
		}
	}
	if frames := scan.FrameStructureAnalysis; len(frames) > 0 {
		bRatio, _ := frames["bFrameRatio"].(float64)
		bRun := jsonMapInt(frames, "maxConsecutiveBFrames")
		pFrames := jsonMapInt(frames, "pFrames")
		if bRatio >= 0.9 || bRun >= 12 || pFrames == 0 {
			recommendations = append(recommendations, fmt.Sprintf("The source has an unusual frame structure (%.1f%% B-frames, longest B run %d, P-frames %d). Validate GOP behavior in LAB and avoid assuming Adaptive B caused it.", bRatio*100, bRun, pFrames))
		} else if bRatio >= 0.5 {
			recommendations = append(recommendations, fmt.Sprintf("The source uses B-frames heavily (%.1f%% of the sampled frames). Compare the encoded preview before accepting the profile.", bRatio*100))
		}
	}
	if high <= 0 {
		recommendations = append(recommendations, "The estimated output is not smaller than the source, so conversion is not recommended for storage savings alone.")
	}
	return AnalysisInsights{RecommendedCRF: crf, EstimatedMinBytes: minBytes, EstimatedMaxBytes: maxBytes, EstimatedSavingsLow: low, EstimatedSavingsHigh: high, Recommendations: recommendations}
}

func proposedProfileForScan(scan models.ScanResult) ProfileInput {
	crf := 20
	if scan.Width >= 3840 || scan.Height >= 2160 || scan.HDR {
		crf = 18
	}
	if scan.Width > 0 && scan.Width <= 720 && scan.Height <= 576 {
		crf = 21
	}
	name := fmt.Sprintf("Suggested %s CRF %d", strings.ToUpper(fallback(codecFamily(scan.VideoCodec), "source")), crf)
	return ProfileInput{
		Name: name, Description: "Generated from Scan/Analysis technical metadata. Review before saving.",
		Container: "mkv", VideoCodec: "x265", CodecFamily: "hevc", EncoderPolicy: "locked",
		PreferredEncoder: "libx265", AllowedEncoders: []string{"libx265"}, FallbackPolicy: "wait",
		BitDepth: 10, PixelFormat: "yuv420p10le", QualityStrategy: "crf", AudioCodec: "copy",
		QualityMode: "crf", QualityValue: crf, PreserveHDR: scan.HDR,
		PreserveSubtitles: scan.SubtitleTracks > 0, PreserveChapters: scan.Chapters > 0,
		WorkerConfig: models.JSONMap{"encoder": "ffmpeg", "videoEncoder": "libx265", "videoPreset": "medium", "pixFmt": "yuv420p10le", "deinterlaceMode": "auto", "preserveOriginalAudio": true, "addAacStereoTrack": false, "aacStereoBitrateKbps": 192},
	}
}

func profileFit(profile models.Profile, proposal ProfileInput, scan models.ScanResult) (int, []string) {
	score := 100
	reasons := []string{}
	if codecFamily(profile.CodecFamily) != codecFamily(proposal.CodecFamily) && codecFamily(profile.VideoCodec) != codecFamily(proposal.CodecFamily) {
		score -= 35
		reasons = append(reasons, "Video codec family differs from the suggested HEVC target.")
	}
	if profile.Container != proposal.Container {
		score -= 8
		reasons = append(reasons, "Container differs from the suggested MKV container.")
	}
	if profile.QualityMode != "crf" {
		score -= 15
		reasons = append(reasons, "Profile does not use a CRF quality target.")
	} else if delta := profile.QualityValue - proposal.QualityValue; delta < -3 || delta > 3 {
		score -= min(20, abs(delta)*3)
		reasons = append(reasons, fmt.Sprintf("CRF %d differs from the analyzed CRF %d target.", profile.QualityValue, proposal.QualityValue))
	}
	if scan.HDR && !profile.PreserveHDR {
		score -= 35
		reasons = append(reasons, "HDR would not be preserved.")
	}
	if preserved, _ := advisorSubtitlePreservation(profile); scan.SubtitleTracks > 0 && !preserved {
		score -= 20
		reasons = append(reasons, "Subtitle tracks would not be preserved.")
	}
	if scan.Chapters > 0 && !profile.PreserveChapters {
		score -= 10
		reasons = append(reasons, "Chapters would not be preserved.")
	}
	if profile.BitDepth > 0 && profile.BitDepth < proposal.BitDepth {
		score -= 10
		reasons = append(reasons, "Bit depth is below the proposed 10-bit output.")
	}
	interlaceStatus, _ := scan.InterlaceAnalysis["status"].(string)
	deinterlaceMode, _ := profile.WorkerConfig["deinterlaceMode"].(string)
	if (interlaceStatus == "interlaced" || interlaceStatus == "mixed" || interlaceStatus == "telecine_suspected") && deinterlaceMode == "off" {
		score -= 25
		reasons = append(reasons, "Interlacing was detected but this profile explicitly disables correction.")
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "Codec, CRF and required preservation options match the scan.")
	}
	return clamp(score, 0, 100), reasons
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func (h AdvisorHandler) hasMVForgeConversion(mediaPath string) bool {
	clean := filepath.Clean(strings.TrimSpace(mediaPath))
	var count int64
	err := h.db.Model(&models.QueueJob{}).
		Where("publication_retired_at IS NULL AND ((published_path = ? AND published_path <> '') OR (published_path = '' AND status = ? AND output_path = ?))", clean, JobStatusCompleted, clean).
		Count(&count).Error
	return err == nil && count > 0
}

func (h AdvisorHandler) scanForPath(path string) (models.ScanResult, error) {
	path = resolveMediaPath(h.db, path)
	var existing models.ScanResult
	if err := h.db.Where("path = ?", path).Order("created_at desc").First(&existing).Error; err == nil {
		enrichCachedScan(h.db, &existing)
		_ = h.db.Save(&existing).Error
		return existing, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return models.ScanResult{}, fmt.Errorf("%s", mediaPathReadError(err))
	}

	if info.IsDir() {
		return models.ScanResult{}, fmt.Errorf("media path must point to a file")
	}

	probe, raw, err := runFFProbe(path, 20)
	if err != nil {
		return models.ScanResult{}, err
	}

	scan := buildScanResult(path, info.Size(), probe, raw)
	if err := h.db.Create(&scan).Error; err != nil {
		return models.ScanResult{}, err
	}

	return scan, nil
}

func evaluateConversion(scan models.ScanResult, profile models.Profile, previouslyConverted bool) AdvisorResponse {
	score := 50
	reasons := []string{}
	warnings := []string{}

	sourceCodec := strings.ToLower(scan.VideoCodec)
	targetCodec := strings.ToLower(profile.VideoCodec)
	targetFamily := profile.CodecFamily
	if targetFamily == "" {
		targetFamily = targetCodec
	}

	if targetCodec == "copy" {
		score -= 15
		reasons = append(reasons, "The selected profile copies video, so it will not reduce video bitrate or change compatibility.")
	} else if codecMatches(sourceCodec, targetFamily) {
		score -= 20
		reasons = append(reasons, "The source video is already in the same codec family as the selected profile. Re-encoding it would introduce another lossy generation.")
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

	if scan.SubtitleTracks > 0 {
		preserved, reason := advisorSubtitlePreservation(profile)
		if preserved {
			reasons = append(reasons, reason)
		} else {
			score -= 10
			warnings = append(warnings, "Subtitle tracks are present but the selected profile does not preserve subtitles in embedded or external form.")
		}
	}

	if scan.AudioTracks > 1 && strings.ToLower(profile.AudioCodec) != "copy" {
		warnings = append(warnings, "Multiple audio tracks are present; verify the profile keeps the tracks you care about.")
	}
	if previouslyConverted {
		reasons = append([]string{"MVForge publication history confirms that this asset was converted previously. The recommendation still evaluates its current codec, bitrate, preservation needs, and selected target profile."}, reasons...)
		if targetCodec != "copy" {
			score -= 15
			warnings = append(warnings, "This would add another lossy video generation. Conversion should proceed only when the analysis shows a measurable compatibility, size, or correction advantage.")
		}
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

func advisorSubtitlePreservation(profile models.Profile) (bool, string) {
	if format := effectiveExternalSubtitleFormat(profile); format != "" {
		return true, fmt.Sprintf("Subtitle tracks are present and the selected profile preserves them as external %s sidecars.", strings.ToUpper(format))
	}
	switch effectiveSubtitlePolicy(profile) {
	case "source":
		return true, "Subtitle tracks are present and the selected profile preserves the embedded subtitle tracks."
	case "disabled":
		return true, "Subtitle tracks are present and the selected profile leaves subtitle handling unchanged, so the tracks remain preserved."
	default:
		return false, ""
	}
}

func codecMatches(source string, target string) bool {
	return codecFamily(source) == codecFamily(target)
}

func codecFamily(codec string) string {
	normalized := strings.ToLower(strings.TrimSpace(codec))
	switch {
	case normalized == "hevc", normalized == "h265", normalized == "h.265", strings.Contains(normalized, "x265"), strings.HasPrefix(normalized, "hevc_"):
		return "hevc"
	case normalized == "h264", normalized == "h.264", normalized == "avc", strings.Contains(normalized, "x264"), strings.HasPrefix(normalized, "h264_"):
		return "h264"
	default:
		return normalized
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
