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
	Source  string         `json:"source,omitempty"`
}

type AdvisorFinding struct {
	ID              string         `json:"id"`
	Category        string         `json:"category"`
	Title           string         `json:"title"`
	Detail          string         `json:"detail"`
	Severity        string         `json:"severity"`
	Confidence      string         `json:"confidence"`
	Actionable      bool           `json:"actionable"`
	DefaultSelected bool           `json:"defaultSelected"`
	Patch           models.JSONMap `json:"patch,omitempty"`
	Evidence        []string       `json:"evidence,omitempty"`
}

type ProfileSuggestionResponse struct {
	MatchType        string             `json:"matchType"`
	Summary          string             `json:"summary"`
	Scan             models.ScanResult  `json:"scan"`
	SuggestedProfile *models.Profile    `json:"suggestedProfile,omitempty"`
	Candidates       []ProfileCandidate `json:"candidates"`
	ProposedProfile  ProfileInput       `json:"proposedProfile"`
	Insights         AnalysisInsights   `json:"insights"`
	Findings         []AdvisorFinding   `json:"findings"`
}

type MVForgePreferences struct {
	QualityGoal           string   `json:"qualityGoal"`
	ExecutionPreference   string   `json:"executionPreference"`
	PreferredVideoEncoder string   `json:"preferredVideoEncoder"`
	PreferredLanguages    []string `json:"preferredLanguages"`
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

	preferences := mvforgePreferences(h.db)
	hardwareEncoder, hardwareWorker := preferredAvailableHardwareEncoder(h.db, preferences.PreferredVideoEncoder)
	proposal := proposedProfileForScanWithPreferences(scan, preferences, hardwareEncoder)
	var profiles []models.Profile
	if err := h.db.Where("disabled = ? OR disabled IS NULL", false).Order("name asc").Find(&profiles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	candidates := make([]ProfileCandidate, 0, len(profiles))
	for _, profile := range profiles {
		score, reasons := profileFitWithPreferences(profile, proposal, scan, preferences)
		candidates = append(candidates, ProfileCandidate{Profile: profile, Score: score, Reasons: reasons})
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Score > candidates[j].Score })
	assignedSource := ""
	if assignments, assignmentErr := profileAssignmentsForAsset(h.db, request.MediaPath); assignmentErr == nil {
		if assignment, ok := assignments["video"]; ok && assignment.Selection == "profile" && assignment.VideoProfileID > 0 {
			for index := range candidates {
				if candidates[index].Profile.ID == assignment.VideoProfileID {
					assignedSource = assignment.TargetType
					candidates[index].Source = "assigned_" + assignment.TargetType
					assigned := candidates[index]
					candidates = append([]ProfileCandidate{assigned}, append(candidates[:index], candidates[index+1:]...)...)
					break
				}
			}
		}
	}
	if len(candidates) > 3 {
		candidates = candidates[:3]
	}

	response := ProfileSuggestionResponse{MatchType: "create", Summary: "No existing profile covers the detected technical requirements closely enough.", Scan: scan, Candidates: candidates, ProposedProfile: proposal}
	if assignedSource != "" && len(candidates) > 0 {
		response.MatchType = "assigned_" + assignedSource
		response.SuggestedProfile = &candidates[0].Profile
		response.Summary = fmt.Sprintf("%s is already assigned at %s scope; MVForge evaluated it before considering alternatives.", candidates[0].Profile.Name, assignedSource)
	} else if len(candidates) > 0 && candidates[0].Score >= 80 {
		response.MatchType = "existing"
		response.SuggestedProfile = &candidates[0].Profile
		response.Summary = fmt.Sprintf("%s is the closest existing profile for this source.", candidates[0].Profile.Name)
	}
	targetCRF := proposal.QualityValue
	if response.SuggestedProfile != nil && response.SuggestedProfile.QualityValue > 0 {
		targetCRF = response.SuggestedProfile.QualityValue
	}
	response.Insights = analysisInsights(scan, targetCRF)
	response.Findings = advisorFindings(scan, proposal, preferences, hardwareEncoder, hardwareWorker)
	c.JSON(http.StatusOK, response)
}

func mvforgePreferences(db *gorm.DB) MVForgePreferences {
	result := MVForgePreferences{QualityGoal: "balanced", ExecutionPreference: "software", PreferredVideoEncoder: "auto", PreferredLanguages: []string{"jpn", "spa", "eng"}}
	if db == nil {
		return result
	}
	var setting models.AppSetting
	if err := db.Where("key = ?", "mvforgePreferences").First(&setting).Error; err != nil {
		return result
	}
	if value := normalizedOptimizationIntent(stringFromUnknown(setting.Value["qualityGoal"])); value != "" {
		result.QualityGoal = value
	}
	if value := strings.ToLower(strings.TrimSpace(stringFromUnknown(setting.Value["executionPreference"]))); value == "hardware" || value == "software" {
		result.ExecutionPreference = value
	}
	if value := strings.TrimSpace(stringFromUnknown(setting.Value["preferredVideoEncoder"])); value != "" {
		result.PreferredVideoEncoder = value
	}
	result.PreferredLanguages = normalizedPreferenceLanguages(setting.Value["preferredLanguages"])
	if len(result.PreferredLanguages) == 0 {
		result.PreferredLanguages = []string{"jpn", "spa", "eng"}
	}
	return result
}

func normalizedPreferenceLanguages(value any) []string {
	values := []string{}
	switch raw := value.(type) {
	case []any:
		for _, item := range raw {
			values = append(values, stringFromUnknown(item))
		}
	case models.JSONList:
		for _, item := range raw {
			values = append(values, stringFromUnknown(item))
		}
	case []string:
		values = append(values, raw...)
	}
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func preferredAvailableHardwareEncoder(db *gorm.DB, preferred string) (string, string) {
	if db == nil {
		return "", ""
	}
	var workers []models.WorkerNode
	if err := db.Where("status = ?", "online").Order("name asc").Find(&workers).Error; err != nil {
		return "", ""
	}
	preferred = strings.TrimSpace(preferred)
	order := []string{"hevc_qsv", "hevc_videotoolbox", "hevc_nvenc"}
	if preferred != "" && preferred != "auto" {
		order = append([]string{preferred}, order...)
	}
	for _, encoder := range order {
		for _, worker := range workers {
			for _, raw := range worker.Encoders {
				if strings.EqualFold(strings.TrimSpace(stringFromUnknown(raw)), encoder) {
					return encoder, worker.Name
				}
			}
		}
	}
	return "", ""
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
	proposal := proposedProfileForScanWithPreferences(scan, MVForgePreferences{QualityGoal: "conservative", ExecutionPreference: "software", PreferredVideoEncoder: "auto"}, "")
	crf := 20
	if scan.Width >= 3840 || scan.Height >= 2160 || scan.HDR {
		crf = 18
	}
	if scan.Width > 0 && scan.Width <= 720 && scan.Height <= 576 {
		crf = 21
	}
	proposal.QualityValue = crf
	proposal.OptimizationIntent = ""
	return proposal
}

func proposedProfileForScanWithPreferences(scan models.ScanResult, preferences MVForgePreferences, availableHardwareEncoder string) ProfileInput {
	intent := normalizedOptimizationIntent(preferences.QualityGoal)
	if intent == "" {
		intent = "balanced"
	}
	crfByIntent := map[string]int{"maximum_savings": 26, "balanced": 22, "conservative": 20, "maximum_quality": 18, "archive": 16}
	crf := crfByIntent[intent]
	if scan.Width >= 3840 || scan.Height >= 2160 || scan.HDR {
		crf = max(14, crf-2)
	}
	if scan.Width > 0 && scan.Width <= 720 && scan.Height <= 576 && intent != "archive" {
		crf = min(28, crf+1)
	}
	encoder := "libx265"
	preferredMode := "software"
	useHardware := preferences.ExecutionPreference == "hardware" && availableHardwareEncoder != ""
	if useHardware {
		encoder = availableHardwareEncoder
		preferredMode = "hardware"
	}
	name := fmt.Sprintf("Suggested %s CRF %d", strings.ToUpper(fallback(codecFamily(scan.VideoCodec), "source")), crf)
	qualityPreset := map[string]string{"maximum_savings": "compact", "balanced": "recommended", "conservative": "best_quality", "maximum_quality": "high_quality", "archive": "archive"}[intent]
	return ProfileInput{
		Name: name, Description: "Generated from Scan/Analysis technical metadata. Review before saving.",
		Container: "mkv", VideoCodec: "x265", CodecFamily: "hevc", EncoderPolicy: "locked",
		PreferredEncoder: encoder, AllowedEncoders: []string{encoder}, FallbackPolicy: "wait",
		BitDepth: 10, PixelFormat: "yuv420p10le", QualityStrategy: "crf", OptimizationIntent: intent, AudioCodec: "copy",
		QualityMode: "crf", QualityValue: crf, PreserveHDR: scan.HDR,
		PreserveSubtitles: scan.SubtitleTracks > 0, PreserveChapters: scan.Chapters > 0,
		WorkerConfig: models.JSONMap{"encoder": "ffmpeg", "videoEncoder": encoder, "preferredEncoder": preferredMode, "useHardwareIfAvailable": useHardware, "hardwareQualityPreset": qualityPreset, "hardwareQualityPresetScale": 2, "videoPreset": "medium", "pixFmt": "yuv420p10le", "deinterlaceMode": "auto", "preserveOriginalAudio": true, "addAacStereoTrack": false, "aacStereoBitrateKbps": 192},
	}
}

func profileFit(profile models.Profile, proposal ProfileInput, scan models.ScanResult) (int, []string) {
	return profileFitWithPreferences(profile, proposal, scan, MVForgePreferences{QualityGoal: proposal.OptimizationIntent, ExecutionPreference: "software"})
}

func profileFitWithPreferences(profile models.Profile, proposal ProfileInput, scan models.ScanResult, preferences MVForgePreferences) (int, []string) {
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
	profileIntent := normalizedOptimizationIntent(profile.OptimizationIntent)
	requestedIntent := normalizedOptimizationIntent(preferences.QualityGoal)
	if profileIntent != "" && requestedIntent != "" && profileIntent != requestedIntent {
		score -= 18
		reasons = append(reasons, fmt.Sprintf("Profile intent %s differs from the preferred %s intent.", profileIntent, requestedIntent))
	} else if profileIntent == "" && requestedIntent != "" {
		score -= 4
		reasons = append(reasons, "Profile has no optimization intent classification; quality matching has lower confidence.")
	}
	if profile.QualityMode == "crf" && proposal.QualityMode == "crf" {
		if delta := profile.QualityValue - proposal.QualityValue; delta < -3 || delta > 3 {
			score -= min(20, abs(delta)*3)
			reasons = append(reasons, fmt.Sprintf("CRF %d differs from the analyzed CRF %d target.", profile.QualityValue, proposal.QualityValue))
		}
	} else if profile.QualityMode != proposal.QualityMode && profileIntent == "" {
		score -= 8
		reasons = append(reasons, "Quality controls use a different encoder domain and the profile has no declared intent for comparison.")
	}
	profileExecution := strings.ToLower(strings.TrimSpace(stringFromUnknown(profile.WorkerConfig["preferredEncoder"])))
	if preferences.ExecutionPreference != "" && profileExecution != "" && profileExecution != preferences.ExecutionPreference {
		score -= 12
		reasons = append(reasons, fmt.Sprintf("Profile prefers %s encoding while Settings prefers %s for new drafts.", profileExecution, preferences.ExecutionPreference))
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

func advisorFindings(scan models.ScanResult, proposal ProfileInput, preferences MVForgePreferences, hardwareEncoder, hardwareWorker string) []AdvisorFinding {
	findings := []AdvisorFinding{{
		ID: "video-proposal", Category: "video", Title: "Use the proposed video quality intent",
		Detail:   fmt.Sprintf("Use %s with the %s quality/space intent as the starting point.", proposal.VideoCodec, proposal.OptimizationIntent),
		Severity: "recommended", Confidence: "medium", Actionable: true, DefaultSelected: true,
		Patch: models.JSONMap{"videoCodec": proposal.VideoCodec, "qualityMode": proposal.QualityMode, "qualityValue": proposal.QualityValue, "optimizationIntent": proposal.OptimizationIntent},
	}}
	interlaceStatus := strings.TrimSpace(stringFromUnknown(scan.InterlaceAnalysis["status"]))
	switch interlaceStatus {
	case "progressive":
		findings = append(findings, AdvisorFinding{ID: "motion-progressive", Category: "video", Title: "Keep deinterlacing disabled", Detail: "The distributed motion analysis classified the source as progressive.", Severity: "recommended", Confidence: confidenceLabel(scan.InterlaceAnalysis["confidence"]), Actionable: true, DefaultSelected: true, Patch: models.JSONMap{"deinterlaceMode": "off"}})
	case "interlaced":
		findings = append(findings, AdvisorFinding{ID: "motion-deinterlace", Category: "video", Title: "Enable deinterlacing", Detail: "Interlaced frames were detected; use the validated field-aware deinterlacing path and review motion in LAB.", Severity: "review", Confidence: confidenceLabel(scan.InterlaceAnalysis["confidence"]), Actionable: true, DefaultSelected: true, Patch: models.JSONMap{"deinterlaceMode": "force"}})
	case "telecine_suspected":
		mode := strings.TrimSpace(stringFromUnknown(scan.InterlaceAnalysis["recommendedMode"]))
		actionable := mode == "ivtc_tff" || mode == "ivtc_bff"
		finding := AdvisorFinding{ID: "motion-telecine", Category: "video", Title: "Review suspected telecine cadence", Detail: "Inverse telecine must be previewed before it becomes part of a profile.", Severity: "review", Confidence: confidenceLabel(scan.InterlaceAnalysis["confidence"]), Actionable: actionable, DefaultSelected: false}
		if actionable {
			finding.Patch = models.JSONMap{"deinterlaceMode": mode}
		}
		findings = append(findings, finding)
	}
	if status := strings.TrimSpace(stringFromUnknown(scan.CropAnalysis["status"])); status == "detected" {
		crop := strings.TrimSpace(stringFromUnknown(scan.CropAnalysis["recommendedCrop"]))
		if crop != "" {
			findings = append(findings, AdvisorFinding{ID: "crop", Category: "video", Title: "Preview the detected crop", Detail: "Stable borders were detected, but crop remains opt-in because framing and subtitles require visual review.", Severity: "review", Confidence: confidenceLabel(scan.CropAnalysis["confidence"]), Actionable: true, DefaultSelected: false, Patch: models.JSONMap{"videoFilters": "crop=" + crop}})
		}
	}
	frameRecommendation := storedFrameStructureRecommendation(scan, "balanced")
	if frameRecommendation.TargetGOPFrames > 0 {
		findings = append(findings, AdvisorFinding{ID: "frame-structure", Category: "frame_structure", Title: "Apply the source-derived frame structure", Detail: fmt.Sprintf("Balanced GOP %d (%.2fs) with maximum B depth %d.", frameRecommendation.TargetGOPFrames, frameRecommendation.TargetGOPSeconds, frameRecommendation.MaxBFrames), Severity: "recommended", Confidence: frameRecommendation.Confidence, Actionable: true, DefaultSelected: true, Patch: models.JSONMap{"frameStructureMode": "balanced", "frameStructureGopMode": "recommended", "frameStructureGopFrames": frameRecommendation.TargetGOPFrames, "frameStructureBFrameMode": "recommended", "frameStructureMaxBFrames": frameRecommendation.MaxBFrames}, Evidence: frameRecommendation.Reasons})
	}
	if scan.HDR {
		findings = append(findings, AdvisorFinding{ID: "preserve-hdr", Category: "color", Title: "Preserve HDR and 10-bit output", Detail: "HDR characteristics were detected. Preserve them unless a separate, explicit tone-mapping workflow is validated.", Severity: "recommended", Confidence: "high", Actionable: true, DefaultSelected: true, Patch: models.JSONMap{"preserveHdr": true, "pixFmt": "yuv420p10le"}})
	}
	findings = append(findings, advisorDiagnosticFindings(scan)...)
	findings = append(findings, colorMetadataFindings(scan)...)
	if preferences.ExecutionPreference == "hardware" {
		if hardwareEncoder != "" {
			findings = append(findings, AdvisorFinding{ID: "encoder-preference", Category: "encoder", Title: "Use the preferred hardware encoder", Detail: fmt.Sprintf("%s is reported by online worker %s and matches the Settings preference for new drafts.", hardwareEncoder, hardwareWorker), Severity: "recommended", Confidence: "high", Actionable: true, DefaultSelected: true, Patch: models.JSONMap{"preferredEncoder": "hardware", "useHardwareIfAvailable": true, "videoEncoder": hardwareEncoder, "hardwareQualityPreset": stringFromUnknown(proposal.WorkerConfig["hardwareQualityPreset"])}})
		} else {
			findings = append(findings, AdvisorFinding{ID: "encoder-unavailable", Category: "encoder", Title: "Preferred hardware encoder is unavailable", Detail: "No online worker currently reports the preferred hardware encoder. MVForge did not silently substitute one.", Severity: "review", Confidence: "high", Actionable: false})
		}
	} else {
		findings = append(findings, AdvisorFinding{ID: "encoder-preference", Category: "encoder", Title: "Use software encoding", Detail: "Software is the Settings preference for new drafts.", Severity: "recommended", Confidence: "high", Actionable: true, DefaultSelected: true, Patch: models.JSONMap{"preferredEncoder": "software", "useHardwareIfAvailable": false, "videoEncoder": "auto"}})
	}
	if scan.SubtitleTracks > 0 {
		findings = append(findings, AdvisorFinding{ID: "preserve-subtitles", Category: "tracks", Title: "Preserve subtitle tracks", Detail: fmt.Sprintf("The source contains %d subtitle track(s). Track language/default decisions remain owned by a Tracks Profile.", scan.SubtitleTracks), Severity: "recommended", Confidence: "high", Actionable: true, DefaultSelected: true, Patch: models.JSONMap{"preserveSubtitles": true}})
	}
	if scan.Chapters > 0 {
		findings = append(findings, AdvisorFinding{ID: "preserve-chapters", Category: "tracks", Title: "Preserve chapters", Detail: fmt.Sprintf("The source contains %d chapter(s).", scan.Chapters), Severity: "recommended", Confidence: "high", Actionable: true, DefaultSelected: true, Patch: models.JSONMap{"preserveChapters": true}})
	}
	return findings
}

func advisorDiagnosticFindings(scan models.ScanResult) []AdvisorFinding {
	findings := []AdvisorFinding{}
	overall := strings.TrimSpace(stringFromUnknown(scan.CompatibilityAnalysis["overall"]))
	severity := "information"
	if overall == "transcode_likely" || overall == "client_dependent" {
		severity = "review"
	}
	seen := map[string]bool{}
	index := 0
	for _, key := range []string{"reasons", "warnings", "recommendations"} {
		for _, detail := range advisorStringList(scan.CompatibilityAnalysis[key]) {
			if seen[detail] {
				continue
			}
			seen[detail] = true
			index++
			findings = append(findings, AdvisorFinding{
				ID: fmt.Sprintf("playback-%d", index), Category: "compatibility", Title: "Playback compatibility finding",
				Detail: detail, Severity: severity, Confidence: "medium", Actionable: false,
			})
		}
	}
	assessment := strings.TrimSpace(stringFromUnknown(scan.FrameStructureAnalysis["assessment"]))
	if assessment != "" && (strings.Contains(strings.ToLower(assessment), "unusual") || strings.EqualFold(stringFromUnknown(scan.FrameStructureAnalysis["variability"]), "high")) {
		findings = append(findings, AdvisorFinding{ID: "frame-structure-diagnostic", Category: "frame_structure", Title: "Review source frame structure", Detail: assessment, Severity: "review", Confidence: confidenceLabel(scan.FrameStructureAnalysis["confidence"]), Actionable: false})
	}
	return findings
}

func advisorStringList(value any) []string {
	items := []any{}
	switch typed := value.(type) {
	case []any:
		items = typed
	case models.JSONList:
		items = []any(typed)
	case []string:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if item = strings.TrimSpace(item); item != "" {
				result = append(result, item)
			}
		}
		return result
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if text := strings.TrimSpace(stringFromUnknown(item)); text != "" {
			result = append(result, text)
		}
	}
	return result
}

func colorMetadataFindings(scan models.ScanResult) []AdvisorFinding {
	if len(scan.VideoStreams) == 0 {
		return nil
	}
	stream, _ := scan.VideoStreams[0].(map[string]interface{})
	values := map[string]string{
		"matrix": strings.TrimSpace(stringFromUnknown(stream["colorSpace"])), "transfer": strings.TrimSpace(stringFromUnknown(stream["colorTransfer"])),
		"primaries": strings.TrimSpace(stringFromUnknown(stream["colorPrimaries"])), "range": strings.TrimSpace(stringFromUnknown(stream["colorRange"])),
	}
	missing := []string{}
	evidence := []string{}
	for _, key := range []string{"matrix", "transfer", "primaries", "range"} {
		if values[key] == "" || strings.EqualFold(values[key], "unknown") || strings.EqualFold(values[key], "unspecified") {
			missing = append(missing, key)
		} else {
			evidence = append(evidence, key+"="+values[key])
		}
	}
	if len(missing) > 0 {
		return []AdvisorFinding{{ID: "color-metadata", Category: "color", Title: "Color metadata requires review", Detail: fmt.Sprintf("Source color metadata is incomplete (%s). MVForge will preserve the source interpretation and will not normalize color automatically.", strings.Join(missing, ", ")), Severity: "review", Confidence: "high", Actionable: false, Evidence: evidence}}
	}
	return []AdvisorFinding{{ID: "color-metadata", Category: "color", Title: "Color metadata is complete", Detail: "Matrix, transfer, primaries and range are declared. Final output validation should confirm that the selected profile preserves the intended values.", Severity: "information", Confidence: "high", Actionable: false, Evidence: evidence}}
}

func confidenceLabel(value any) string {
	if label := strings.ToLower(strings.TrimSpace(stringFromUnknown(value))); label == "low" || label == "medium" || label == "high" {
		return label
	}
	confidence := workerNumberValue(value, 0)
	switch {
	case confidence >= 0.8:
		return "high"
	case confidence >= 0.5:
		return "medium"
	default:
		return "low"
	}
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
		if ensureFrameStructureRecommendation(&existing) {
			_ = h.db.Model(&existing).Update("frame_structure_recommendation", existing.FrameStructureRecommendation).Error
		}
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
