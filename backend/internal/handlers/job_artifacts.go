package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type jobArtifact struct {
	GeneratedAt     time.Time             `json:"generatedAt"`
	Kind            string                `json:"kind"`
	Job             models.QueueJob       `json:"job"`
	Command         string                `json:"command,omitempty"`
	ExecutionEngine string                `json:"executionEngine,omitempty"`
	Profile         models.Profile        `json:"profile"`
	AudioProfileKey string                `json:"audioProfileKey,omitempty"`
	ProcessingMode  string                `json:"processingMode,omitempty"`
	AssetConversion AssetConversionReport `json:"assetConversion,omitempty"`
	SourceProbe     map[string]any        `json:"sourceProbe,omitempty"`
	OutputProbe     map[string]any        `json:"outputProbe,omitempty"`
	Result          map[string]any        `json:"result,omitempty"`
	Notes           string                `json:"notes,omitempty"`
	ExecutionPlan   models.JSONMap        `json:"executionPlan,omitempty"`
	EncoderDecision models.JSONMap        `json:"encoderDecision,omitempty"`
	StreamPlan      ResolvedStreamPlan    `json:"streamPlan"`
}

type AssetConversionReport struct {
	HasOverrides       bool                           `json:"hasOverrides"`
	Override           AssetConversionOverrideState   `json:"override,omitempty"`
	SelectedVideo      []int                          `json:"selectedVideo,omitempty"`
	SelectedAudio      []int                          `json:"selectedAudio,omitempty"`
	SelectedSubtitles  []int                          `json:"selectedSubtitles,omitempty"`
	VideoMetadata      map[int]StreamMetadataOverride `json:"videoMetadata,omitempty"`
	AudioMetadata      map[int]StreamMetadataOverride `json:"audioMetadata,omitempty"`
	SubtitleMetadata   map[int]StreamMetadataOverride `json:"subtitleMetadata,omitempty"`
	SubtitleTransforms []SubtitleTransform            `json:"subtitleTransforms,omitempty"`
	SubtitleArtifacts  models.JSONList                `json:"subtitleArtifacts,omitempty"`
	ProfileOverrides   map[string]any                 `json:"profileOverrides,omitempty"`
	HumanSummary       []string                       `json:"humanSummary,omitempty"`
}

type jobArtifactsResponse struct {
	JobID      uint           `json:"jobId"`
	AsIs       map[string]any `json:"asIs,omitempty"`
	Result     map[string]any `json:"result,omitempty"`
	AsIsPath   string         `json:"asIsPath,omitempty"`
	ResultPath string         `json:"resultPath,omitempty"`
	Warnings   []string       `json:"warnings,omitempty"`
}

type analysisBackfillResponse struct {
	Imported  int `json:"imported"`
	Corrected int `json:"corrected"`
	Skipped   int `json:"skipped"`
	Total     int `json:"total"`
}

func JobArtifacts(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var job models.QueueJob
		if err := db.First(&job, c.Param("id")).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		response := jobArtifactsResponse{JobID: job.ID}
		warnings := []string{}
		if artifact, path, err := readLatestJobArtifact(db, job, "as-is"); err == nil {
			response.AsIs = artifact
			response.AsIsPath = path
		} else if err != os.ErrNotExist {
			warnings = append(warnings, "AS-IS artifact: "+err.Error())
		}
		if artifact, path, err := readLatestJobArtifact(db, job, "result"); err == nil {
			response.Result = artifact
			response.ResultPath = path
		} else if err != os.ErrNotExist {
			warnings = append(warnings, "Result artifact: "+err.Error())
		}
		response.Warnings = warnings
		c.JSON(http.StatusOK, response)
	}
}

func BackfillAnalysisFromAsIsReports(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := backfillAnalysisRecordsFromAsIsReports(db)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func writeJobAsIsArtifact(db *gorm.DB, job models.QueueJob, profile models.Profile, audioProfile *audioEnhancementProfile, command string, processingMode string, streamPlan ResolvedStreamPlan) error {
	sourceProbe := ffprobeJSON(job.MediaPath)
	override := conversionOverrideForPath(job.MediaPath, assetConversionOverrides(db))
	artifact := jobArtifact{
		GeneratedAt:     time.Now(),
		Kind:            "as-is",
		Job:             job,
		Command:         command,
		ExecutionEngine: "FFmpeg",
		Profile:         profile,
		ProcessingMode:  processingMode,
		AssetConversion: assetConversionReport(override),
		SourceProbe:     sourceProbe,
		Notes:           job.Notes,
		StreamPlan:      streamPlan,
	}
	artifact.EncoderDecision = encoderDecisionForProfile(profile)
	if job.ActiveExecutionPlanID != nil {
		var plan models.ExecutionPlan
		if db.First(&plan, *job.ActiveExecutionPlanID).Error == nil {
			artifact.ExecutionPlan = models.JSONMap{"id": plan.ID, "estimateConfidence": plan.EstimateConfidence, "evaluation": plan.Evaluation, "estimatedOutputMinBytes": plan.EstimatedOutputMinBytes, "estimatedOutputMaxBytes": plan.EstimatedOutputMaxBytes}
		}
	}
	if audioProfile != nil {
		artifact.AudioProfileKey = audioProfile.Key
	}
	appendAnalysisRecordForJob(db, job, sourceProbe, artifact.AssetConversion)
	return writeJobArtifact(db, job, "as-is", artifact)
}

func encoderDecisionForProfile(profile models.Profile) models.JSONMap {
	requestedEncoder := strings.ToLower(strings.TrimSpace(workerStringValue(profile.WorkerConfig["videoEncoder"])))
	if requestedEncoder == "" || requestedEncoder == "auto" {
		requestedEncoder = ffmpegCodecName(profile.VideoCodec)
	}
	effectiveEncoder := resolvedVideoEncoder(profile)
	decision := models.JSONMap{"requested": requestedEncoder, "effective": effectiveEncoder, "downgraded": requestedEncoder != effectiveEncoder}
	if requestedEncoder != effectiveEncoder {
		decision["reason"] = "requested encoder or bit-depth combination did not pass the active worker capability probe"
	}
	return decision
}

func appendAnalysisRecordForJob(db *gorm.DB, job models.QueueJob, sourceProbe map[string]any, conversion AssetConversionReport) {
	if len(sourceProbe) == 0 || sourceProbe["error"] != nil {
		return
	}

	var probe FFProbeResult
	content, err := json.Marshal(sourceProbe)
	if err != nil || json.Unmarshal(content, &probe) != nil {
		return
	}

	size := int64(0)
	if info, err := os.Stat(job.MediaPath); err == nil {
		size = info.Size()
	}
	scan := buildScanResult(job.MediaPath, size, probe, models.JSONMap(sourceProbe))
	record := models.JSONMap{
		"id":         strconv.FormatInt(time.Now().UnixMilli(), 10) + "-" + job.MediaPath,
		"assetPath":  job.MediaPath,
		"assetName":  scan.FileName,
		"decision":   "queued-conversion-as-is",
		"notes":      "Automatically captured before MVForge job #" + strconv.FormatUint(uint64(job.ID), 10),
		"scan":       scan,
		"conversion": conversion,
		"createdAt":  time.Now().UTC().Format(time.RFC3339),
	}

	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", "analysisRecords").Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			_ = db.Create(&models.AppSetting{Key: "analysisRecords", Value: models.JSONMap{"records": []any{record}}}).Error
		}
		return
	}

	records, _ := setting.Value["records"].([]any)
	next := append([]any{record}, records...)
	if len(next) > 200 {
		next = next[:200]
	}
	setting.Value = models.JSONMap{"records": next}
	_ = db.Save(&setting).Error
}

func backfillAnalysisRecordsFromAsIsReports(db *gorm.DB) (analysisBackfillResponse, error) {
	response := analysisBackfillResponse{}
	dir, err := jobReportPath(db, "as-is")
	if err != nil {
		return response, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return response, nil
		}
		return response, err
	}

	files := []os.DirEntry{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		files = append(files, entry)
	}
	sort.SliceStable(files, func(i, j int) bool {
		left, leftErr := files[i].Info()
		right, rightErr := files[j].Info()
		if leftErr != nil || rightErr != nil {
			return files[i].Name() > files[j].Name()
		}
		return left.ModTime().After(right.ModTime())
	})

	records, seenIDs, err := existingAnalysisRecords(db)
	if err != nil {
		return response, err
	}
	response.Corrected = normalizeAnalysisRecordHDR(records)

	imported := []any{}
	for _, file := range files {
		response.Total++
		record, ok := analysisRecordFromAsIsReport(filepath.Join(dir, file.Name()), file.Name())
		if !ok {
			response.Skipped++
			continue
		}
		id := stringFromUnknown(record["id"])
		if id == "" {
			response.Skipped++
			continue
		}
		if _, exists := seenIDs[id]; exists {
			response.Skipped++
			continue
		}
		seenIDs[id] = struct{}{}
		imported = append(imported, record)
		response.Imported++
	}

	if response.Imported == 0 && response.Corrected == 0 {
		return response, nil
	}

	next := append(imported, records...)
	if len(next) > 200 {
		next = next[:200]
	}
	value := models.JSONMap{"records": next}

	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", "analysisRecords").Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return response, db.Create(&models.AppSetting{Key: "analysisRecords", Value: value}).Error
		}
		return response, err
	}
	setting.Value = value
	return response, db.Save(&setting).Error
}

func normalizeAnalysisRecordHDR(records []any) int {
	corrected := 0
	for _, value := range records {
		record, ok := value.(map[string]any)
		if !ok {
			continue
		}
		scan, ok := record["scan"].(map[string]any)
		if !ok {
			continue
		}
		raw, ok := scan["rawProbe"].(map[string]any)
		if !ok {
			continue
		}
		content, err := json.Marshal(raw)
		if err != nil {
			continue
		}
		var probe FFProbeResult
		if json.Unmarshal(content, &probe) != nil {
			continue
		}
		hdr := isHDR(firstStream(probe.Streams, "video"))
		current, _ := scan["hdr"].(bool)
		if current == hdr {
			continue
		}
		scan["hdr"] = hdr
		scan["videoStreams"] = streamSummaries(probe.Streams, "video")
		corrected++
	}
	return corrected
}

func analysisRecordFromAsIsReport(path string, fileName string) (models.JSONMap, bool) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var artifact jobArtifact
	if err := json.Unmarshal(content, &artifact); err != nil {
		return nil, false
	}
	if artifact.Kind != "as-is" || len(artifact.SourceProbe) == 0 || artifact.SourceProbe["error"] != nil {
		return nil, false
	}

	var probe FFProbeResult
	probeContent, err := json.Marshal(artifact.SourceProbe)
	if err != nil || json.Unmarshal(probeContent, &probe) != nil {
		return nil, false
	}

	assetPath := strings.TrimSpace(artifact.Job.MediaPath)
	if assetPath == "" {
		assetPath = strings.TrimSpace(probe.Format.Filename)
	}
	if assetPath == "" {
		return nil, false
	}

	size := probeSizeBytes(artifact.SourceProbe)
	if info, err := os.Stat(assetPath); err == nil {
		size = info.Size()
	}

	generatedAt := artifact.GeneratedAt
	if generatedAt.IsZero() {
		if info, err := os.Stat(path); err == nil {
			generatedAt = info.ModTime()
		} else {
			generatedAt = time.Now()
		}
	}

	scan := buildScanResult(assetPath, size, probe, models.JSONMap(artifact.SourceProbe))
	jobID := strconv.FormatUint(uint64(artifact.Job.ID), 10)
	if artifact.Job.ID == 0 {
		jobID = "unknown"
	}
	return models.JSONMap{
		"id":         "as-is-report-" + strings.TrimSuffix(fileName, filepath.Ext(fileName)),
		"assetPath":  assetPath,
		"assetName":  scan.FileName,
		"decision":   "queued-conversion-as-is",
		"notes":      "Imported from AS-IS report for MVForge job #" + jobID + ".",
		"scan":       scan,
		"conversion": artifact.AssetConversion,
		"createdAt":  generatedAt.UTC().Format(time.RFC3339),
	}, true
}

func existingAnalysisRecords(db *gorm.DB) ([]any, map[string]struct{}, error) {
	seen := map[string]struct{}{}
	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", "analysisRecords").Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return []any{}, seen, nil
		}
		return nil, nil, err
	}

	records, _ := setting.Value["records"].([]any)
	for _, record := range records {
		if mapped, ok := record.(map[string]any); ok {
			if id := stringFromUnknown(mapped["id"]); id != "" {
				seen[id] = struct{}{}
			}
		}
	}
	return records, seen, nil
}

func probeSizeBytes(raw map[string]any) int64 {
	format, ok := raw["format"].(map[string]any)
	if !ok {
		return 0
	}
	size, err := strconv.ParseInt(stringFromUnknown(format["size"]), 10, 64)
	if err != nil {
		return 0
	}
	return size
}

func assetConversionReport(override AssetConversionOverrideState) AssetConversionReport {
	report := AssetConversionReport{
		HasOverrides: assetConversionReportHasOverrides(override),
	}
	if !report.HasOverrides {
		return report
	}

	report.Override = override
	report.SelectedVideo = override.KeepVideoStreams
	report.SelectedAudio = override.KeepAudioStreams
	report.SelectedSubtitles = override.KeepSubtitleStreams
	report.VideoMetadata = override.VideoMetadata
	report.AudioMetadata = override.AudioMetadata
	report.SubtitleMetadata = override.SubtitleMetadata
	report.SubtitleTransforms = override.SubtitleTransforms
	report.ProfileOverrides = assetProfileOverrideMap(override)
	report.HumanSummary = assetConversionHumanSummary(override)
	return report
}

func assetConversionReportHasOverrides(override AssetConversionOverrideState) bool {
	return !assetConversionOverrideEmpty(override) ||
		len(override.VideoMetadata) > 0 ||
		len(override.AudioMetadata) > 0 ||
		len(override.SubtitleMetadata) > 0
}

func assetProfileOverrideMap(override AssetConversionOverrideState) map[string]any {
	values := map[string]any{}
	if strings.TrimSpace(override.VideoCodec) != "" {
		values["videoCodec"] = override.VideoCodec
	}
	if strings.TrimSpace(override.AudioCodec) != "" {
		values["audioCodec"] = override.AudioCodec
	}
	if strings.TrimSpace(override.QualityMode) != "" {
		values["qualityMode"] = override.QualityMode
	}
	if override.QualityValue > 0 {
		values["qualityValue"] = override.QualityValue
	}
	if strings.TrimSpace(override.VideoPreset) != "" {
		values["videoPreset"] = override.VideoPreset
	}
	if strings.TrimSpace(override.PixFmt) != "" {
		values["pixFmt"] = override.PixFmt
	}
	if strings.TrimSpace(override.VideoFilters) != "" {
		values["videoFilters"] = override.VideoFilters
	}
	if strings.TrimSpace(override.DeinterlaceMode) != "" {
		values["deinterlaceMode"] = override.DeinterlaceMode
	}
	if strings.TrimSpace(override.X265Params) != "" {
		values["x265Params"] = override.X265Params
	}
	if strings.TrimSpace(override.ProcessingMode) != "" {
		values["processingMode"] = override.ProcessingMode
	}
	if override.PreserveHDR != nil {
		values["preserveHdr"] = *override.PreserveHDR
	}
	if override.PreserveSubtitles != nil {
		values["preserveSubtitles"] = *override.PreserveSubtitles
	}
	if override.PreserveChapters != nil {
		values["preserveChapters"] = *override.PreserveChapters
	}
	if strings.TrimSpace(override.ExternalSubtitleFormat) != "" {
		values["externalSubtitleFormat"] = override.ExternalSubtitleFormat
	}
	if strings.TrimSpace(override.FinalColorPolicy) != "" {
		values["finalColorPolicy"] = override.FinalColorPolicy
	}
	if override.AddAACStereoTrack != nil {
		values["addAacStereoTrack"] = *override.AddAACStereoTrack
	}
	if override.AACStereoDefault != nil {
		values["aacStereoDefault"] = *override.AACStereoDefault
	}
	if override.EnhancedAudioSourceStreamIndex != nil {
		values["enhancedAudioSourceStreamIndex"] = *override.EnhancedAudioSourceStreamIndex
	}
	if override.UseHardwareIfAvailable != nil {
		values["useHardwareIfAvailable"] = *override.UseHardwareIfAvailable
	}
	if strings.TrimSpace(override.VideoEncoder) != "" {
		values["videoEncoder"] = override.VideoEncoder
	}
	if strings.TrimSpace(override.PreferredEncoder) != "" {
		values["preferredEncoder"] = override.PreferredEncoder
	}
	if override.GlobalQuality > 0 {
		values["globalQuality"] = override.GlobalQuality
	}
	if strings.TrimSpace(override.QSVRateControl) != "" {
		values["qsvRateControl"] = override.QSVRateControl
	}
	if override.QSVLookAheadDepth > 0 {
		values["qsvLookAheadDepth"] = override.QSVLookAheadDepth
	}
	if override.QSVExtendedBRC != nil {
		values["qsvExtendedBrc"] = *override.QSVExtendedBRC
	}
	if override.QSVAdaptiveI != nil {
		values["qsvAdaptiveI"] = *override.QSVAdaptiveI
	}
	if override.QSVAdaptiveB != nil {
		values["qsvAdaptiveB"] = *override.QSVAdaptiveB
	}
	if override.VideoToolboxBitrateMbps > 0 {
		values["videoToolboxBitrateMbps"] = override.VideoToolboxBitrateMbps
	}
	if override.VideoToolboxMaxrateMbps > 0 {
		values["videoToolboxMaxrateMbps"] = override.VideoToolboxMaxrateMbps
	}
	if override.VideoToolboxBufferMbps > 0 {
		values["videoToolboxBufferMbps"] = override.VideoToolboxBufferMbps
	}
	if override.VideoToolboxProfile != "" {
		values["videoToolboxProfile"] = override.VideoToolboxProfile
	}
	if override.VideoToolboxGOP > 0 {
		values["videoToolboxGop"] = override.VideoToolboxGOP
	}
	if override.VideoToolboxRealtime != nil {
		values["videoToolboxRealtime"] = *override.VideoToolboxRealtime
	}
	if override.VideoToolboxAllowFrameReordering != nil {
		values["videoToolboxAllowFrameReordering"] = *override.VideoToolboxAllowFrameReordering
	}
	if override.VideoToolboxPowerEfficiency != nil {
		values["videoToolboxPowerEfficiency"] = *override.VideoToolboxPowerEfficiency
	}
	if override.HardwareQualityPreset != "" {
		values["hardwareQualityPreset"] = override.HardwareQualityPreset
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

func assetConversionHumanSummary(override AssetConversionOverrideState) []string {
	summary := []string{}
	if override.KeepVideoStreams != nil {
		summary = append(summary, "Selected video streams: "+intListLabel(override.KeepVideoStreams))
	}
	if override.KeepAudioStreams != nil {
		summary = append(summary, "Selected audio streams: "+intListLabel(override.KeepAudioStreams))
	}
	if override.KeepSubtitleStreams != nil {
		summary = append(summary, "Selected subtitle streams: "+intListLabel(override.KeepSubtitleStreams))
	}
	if override.EnhancedAudioSourceStreamIndex != nil {
		summary = append(summary, fmt.Sprintf("Enhanced audio source stream: %d", *override.EnhancedAudioSourceStreamIndex))
	}
	if len(override.AudioMetadata) > 0 {
		summary = append(summary, "Audio track metadata edited")
	}
	if len(override.VideoMetadata) > 0 {
		summary = append(summary, "Video track metadata edited")
	}
	if len(override.SubtitleMetadata) > 0 {
		summary = append(summary, "Subtitle track metadata edited")
	}
	for _, transform := range override.SubtitleTransforms {
		action := fmt.Sprintf("Subtitle stream %d exported as %s", transform.StreamIndex, strings.ToUpper(transform.Format))
		if transform.RemoveEmbedded {
			action += " and removed from the converted container"
		}
		if transform.MakeDefault {
			action += " as the default sidecar"
		}
		summary = append(summary, action)
	}
	for key, value := range assetProfileOverrideMap(override) {
		summary = append(summary, key+": "+fmt.Sprint(value))
	}
	sort.Strings(summary)
	return summary
}

func intListLabel(values []int) string {
	if values == nil {
		return "profile/default"
	}
	if len(values) == 0 {
		return "none"
	}
	labels := make([]string, 0, len(values))
	for _, value := range values {
		labels = append(labels, strconv.Itoa(value))
	}
	return strings.Join(labels, ", ")
}

func writeJobResultArtifact(db *gorm.DB, job models.QueueJob, result map[string]any) error {
	if db != nil && job.ID > 0 {
		var current models.QueueJob
		if db.First(&current, job.ID).Error == nil {
			job = current
		}
	}
	probePath := job.OutputPath
	for _, candidate := range []string{job.PublishedPath, job.PlannedPublishedPath, job.OutputPath} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			probePath = candidate
			break
		}
	}
	override := conversionOverrideForPath(job.MediaPath, assetConversionOverrides(db))
	artifact := jobArtifact{
		GeneratedAt:     time.Now(),
		Kind:            "result",
		Job:             job,
		AssetConversion: assetConversionReport(override),
		OutputProbe:     ffprobeJSON(probePath),
		Result:          result,
		Notes:           job.Notes,
	}
	if asIs, _, err := readLatestJobArtifact(db, job, "as-is"); err == nil {
		if raw, ok := asIs["streamPlan"]; ok {
			if encoded, marshalErr := json.Marshal(raw); marshalErr == nil {
				_ = json.Unmarshal(encoded, &artifact.StreamPlan)
			}
		}
	}
	if profile, err := scheduler.RestoreProfileSnapshot(job.ProfileSnapshot); err == nil {
		artifact.Profile = profile
		artifact.EncoderDecision = encoderDecisionForProfile(profile)
	}
	artifact.AssetConversion.SubtitleArtifacts = job.SubtitleArtifacts
	persistCompletedEncoderResult(db, artifact)
	return writeJobArtifact(db, job, "result", artifact)
}

func persistCompletedEncoderResult(db *gorm.DB, artifact jobArtifact) {
	if db == nil || artifact.Job.ID == 0 || artifact.Result["status"] != "completed" {
		return
	}
	effectiveEncoder := workerStringValue(artifact.EncoderDecision["effective"])
	var activePlan models.ExecutionPlan
	hasActivePlan := false
	if artifact.Job.ActiveExecutionPlanID != nil && db.First(&activePlan, *artifact.Job.ActiveExecutionPlanID).Error == nil {
		hasActivePlan = true
		if strings.TrimSpace(activePlan.SelectedEncoder) != "" {
			effectiveEncoder = activePlan.SelectedEncoder
		}
	}
	if effectiveEncoder != "hevc_qsv" && effectiveEncoder != "hevc_videotoolbox" {
		return
	}
	inputArtifact, _, err := readLatestJobArtifact(db, artifact.Job, "as-is")
	if err != nil {
		return
	}
	sourceProbe, _ := inputArtifact["sourceProbe"].(map[string]any)
	source := videoProbeFacts(sourceProbe)
	output := videoProbeFacts(artifact.OutputProbe)
	if source.Bitrate <= 0 || source.Duration <= 0 || output.Bitrate <= 0 {
		return
	}
	actualOutputBytes := probeFormatInt64(artifact.OutputProbe, "size")
	if actualOutputBytes <= 0 {
		actualOutputBytes = int64(float64(output.Bitrate) * output.Duration / 8)
	}
	record := models.JSONMap{
		"jobId": artifact.Job.ID, "recordedAt": time.Now().UTC().Format(time.RFC3339),
		"effectiveEncoder":      effectiveEncoder,
		"hardwareQualityPreset": workerStringValue(artifact.Profile.WorkerConfig["hardwareQualityPreset"]),
		"sourceVideoBitrate":    source.Bitrate, "outputVideoBitrate": output.Bitrate,
		"durationSeconds": source.Duration, "sourceWidth": source.Width, "sourceHeight": source.Height,
		"outputWidth": output.Width, "outputHeight": output.Height,
		"actualOutputBytes": actualOutputBytes,
	}
	if hasActivePlan {
		record["estimatedOutputMinBytes"] = activePlan.EstimatedOutputMinBytes
		record["estimatedOutputMaxBytes"] = activePlan.EstimatedOutputMaxBytes
		record["estimateConfidence"] = activePlan.EstimateConfidence
		record["encoderRecommendation"] = activePlan.Evaluation["encoderRecommendation"]
	}
	const key = "encoderResultHistory"
	_ = db.Transaction(func(tx *gorm.DB) error {
		var setting models.AppSetting
		err := tx.First(&setting, "key = ?", key).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		records := []any{}
		if err == nil {
			records, _ = setting.Value["records"].([]any)
		}
		records = append([]any{record}, records...)
		if len(records) > 500 {
			records = records[:500]
		}
		if err == gorm.ErrRecordNotFound {
			return tx.Create(&models.AppSetting{Key: key, Value: models.JSONMap{"records": records}}).Error
		}
		setting.Value = models.JSONMap{"records": records}
		return tx.Save(&setting).Error
	})
}

type encoderVideoProbeFacts struct {
	Bitrate  int64
	Duration float64
	Width    int
	Height   int
}

func videoProbeFacts(probe map[string]any) encoderVideoProbeFacts {
	facts := encoderVideoProbeFacts{Duration: probeFormatFloat64(probe, "duration")}
	streams, _ := probe["streams"].([]any)
	for _, raw := range streams {
		stream, _ := raw.(map[string]any)
		if workerStringValue(stream["codec_type"]) != "video" {
			continue
		}
		facts.Bitrate = jsonInt64(stream["bit_rate"])
		facts.Width = int(jsonInt64(stream["width"]))
		facts.Height = int(jsonInt64(stream["height"]))
		if facts.Duration <= 0 {
			facts.Duration = jsonFloat64(stream["duration"])
		}
		break
	}
	if facts.Bitrate <= 0 {
		facts.Bitrate = probeFormatInt64(probe, "bit_rate")
	}
	return facts
}

func probeFormatInt64(probe map[string]any, key string) int64 {
	format, _ := probe["format"].(map[string]any)
	return jsonInt64(format[key])
}

func probeFormatFloat64(probe map[string]any, key string) float64 {
	format, _ := probe["format"].(map[string]any)
	return jsonFloat64(format[key])
}

func jsonInt64(value any) int64 {
	parsed, _ := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(value)), 10, 64)
	return parsed
}

func jsonFloat64(value any) float64 {
	parsed, _ := strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(value)), 64)
	return parsed
}

func writeJobArtifact(db *gorm.DB, job models.QueueJob, kind string, artifact jobArtifact) error {
	dir, err := jobReportPath(db, kind)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	content, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, jobReportFileName(job, kind)), append(content, '\n'), 0o644)
}

func ffprobeJSON(path string) map[string]any {
	if path == "" {
		return nil
	}
	output, err := exec.Command(
		"ffprobe",
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	).Output()
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	var value map[string]any
	if err := json.Unmarshal(output, &value); err != nil {
		return map[string]any{"error": err.Error()}
	}
	return value
}

func jobReportPath(db *gorm.DB, kind string) (string, error) {
	setting := models.AppSetting{}
	values := models.JSONMap{}
	if err := db.First(&setting, "key = ?", "paths").Error; err == nil && setting.Value != nil {
		values = setting.Value
	} else if err != nil && err != gorm.ErrRecordNotFound {
		return "", err
	}

	key := "resultsReportsPath"
	fallback := "/media/reports/results"
	if kind == "as-is" {
		key = "asIsReportsPath"
		fallback = "/media/reports/as-is"
	}
	if value := strings.TrimSpace(stringFromUnknown(values[key])); value != "" {
		return value, nil
	}
	return fallback, nil
}

func readLatestJobArtifact(db *gorm.DB, job models.QueueJob, kind string) (map[string]any, string, error) {
	dir, err := jobReportPath(db, kind)
	if err != nil {
		return nil, "", err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", os.ErrNotExist
		}
		return nil, "", err
	}

	prefix := strconv.FormatUint(uint64(job.ID), 10) + "-job-" + strconv.FormatUint(uint64(job.ID), 10) + "-"
	matches := []os.DirEntry{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		matches = append(matches, entry)
	}
	if len(matches) == 0 {
		return nil, "", os.ErrNotExist
	}

	sort.SliceStable(matches, func(i, j int) bool {
		left, leftErr := matches[i].Info()
		right, rightErr := matches[j].Info()
		if leftErr != nil || rightErr != nil {
			return matches[i].Name() > matches[j].Name()
		}
		return left.ModTime().After(right.ModTime())
	})

	for _, match := range matches {
		path := filepath.Join(dir, match.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, "", err
		}
		var artifact map[string]any
		if err := json.Unmarshal(content, &artifact); err != nil {
			return nil, "", err
		}
		if jobArtifactMatchesJob(artifact, job) {
			return artifact, path, nil
		}
	}
	return nil, "", os.ErrNotExist
}

func jobArtifactMatchesJob(artifact map[string]any, job models.QueueJob) bool {
	rawJob, ok := artifact["job"].(map[string]any)
	if !ok {
		return false
	}
	if uintFromUnknown(rawJob["id"]) != job.ID {
		return false
	}
	return filepath.Clean(stringFromUnknown(rawJob["mediaPath"])) == filepath.Clean(job.MediaPath)
}

func uintFromUnknown(value any) uint {
	switch typed := value.(type) {
	case float64:
		if typed >= 0 && typed == float64(uint(typed)) {
			return uint(typed)
		}
	case int:
		if typed >= 0 {
			return uint(typed)
		}
	case uint:
		return typed
	case json.Number:
		parsed, err := strconv.ParseUint(string(typed), 10, 64)
		if err == nil {
			return uint(parsed)
		}
	case string:
		parsed, err := strconv.ParseUint(strings.TrimSpace(typed), 10, 64)
		if err == nil {
			return uint(parsed)
		}
	}
	return 0
}

func jobReportFileName(job models.QueueJob, kind string) string {
	assetName := strings.TrimSuffix(filepath.Base(job.MediaPath), filepath.Ext(job.MediaPath))
	if strings.TrimSpace(assetName) == "" {
		assetName = "asset"
	}
	date := time.Now().Format("20060102-150405")
	jobID := strconv.FormatUint(uint64(job.ID), 10)
	return jobID + "-job-" + jobID + "-" + safeReportName(assetName) + "-" + date + ".json"
}

var unsafeReportNamePattern = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func safeReportName(value string) string {
	name := unsafeReportNamePattern.ReplaceAllString(strings.TrimSpace(value), "-")
	name = strings.Trim(name, ".-_")
	if name == "" {
		return "asset"
	}
	return name
}
