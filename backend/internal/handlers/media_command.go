package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/capabilities"
	"github.com/anuelvs/mvforge/backend/internal/models"
)

const (
	ProcessingModeAudioOnly  = "audio_only"
	ProcessingModeFullEncode = "full_encode"
)

type MediaJobPlan struct {
	InputPath                string
	OutputPath               string
	Profile                  models.Profile
	AudioProfile             *audioEnhancementProfile
	Overwrite                bool
	ProcessingMode           string
	Streams                  MediaStreamInventory
	Override                 AssetConversionOverrideState
	StreamValidationWarnings []string
	Interlace                InterlaceAnalysis
}

type MediaStreamInventory struct {
	Video        []MediaStream
	Audio        []MediaAudioStream
	Subtitle     []MediaStream
	Duration     float64
	ChapterCount int
	TotalBitrate int64
}

type MediaStream struct {
	Index              int
	Codec              string
	FieldOrder         string
	PixelFormat        string
	ColorRange         string
	ColorSpace         string
	ColorTransfer      string
	ColorPrimaries     string
	Language           string
	Title              string
	Default            bool
	Forced             bool
	Width              int
	Height             int
	Bitrate            int64
	FrameRate          string
	SampleAspectRatio  string
	DisplayAspectRatio string
}

type MediaAudioStream struct {
	Index         int
	Codec         string
	Channels      int
	ChannelLayout string
	Language      string
	Title         string
	Default       bool
	Forced        bool
	Bitrate       int64
}

type AudioProcessor interface {
	Name() string
	Build() string
	Validate() error
}

type AudioFilterProcessor struct {
	name   string
	filter string
}

func (p AudioFilterProcessor) Name() string {
	return p.name
}

func (p AudioFilterProcessor) Build() string {
	return strings.TrimSpace(p.filter)
}

func (p AudioFilterProcessor) Validate() error {
	if strings.TrimSpace(p.filter) == "" {
		return fmt.Errorf("%s audio processor has an empty filter", p.name)
	}
	return nil
}

type FFmpegCommandBuilder struct{}

func (FFmpegCommandBuilder) Build(plan MediaJobPlan) []string {
	args := []string{"-hide_banner"}
	if plan.Overwrite {
		args = append(args, "-y")
	} else {
		args = append(args, "-n")
	}

	args = append(args, "-i", plan.InputPath)
	selectedAudioStreams := selectedAudioStreams(plan.Streams.Audio, plan.Override)
	addAACStereoTrack := effectiveAACOption(plan, plan.Override.AddAACStereoTrack, "addAacStereoTrack", "addAacStereoDefault", false)
	aacStereoDefault := effectiveAACOption(plan, plan.Override.AACStereoDefault, "aacStereoDefault", "", false)
	needsAACCompatibility := addAACStereoTrack && !hasAACStereoStream(selectedAudioStreams)
	preserveOriginalAudio := profileWorkerBool(plan.Profile, "preserveOriginalAudio", true)
	mappedAudioStreams := selectedAudioStreams
	if planHasStreamSelection(plan.Override) {
		args = appendSelectedStreamMaps(args, plan)
	} else if needsAACCompatibility && !preserveOriginalAudio {
		args = appendNonAudioStreamMaps(args, plan)
		mappedAudioStreams = []MediaAudioStream{}
	} else {
		args = append(args, "-map", "0")
	}

	enhancedAudioIndex := -1
	aacStereoIndex := -1
	if plan.AudioProfile != nil && len(plan.Streams.Audio) > 0 {
		enhancedAudioIndex = len(mappedAudioStreams)
		enhancedSourceIndex := enhancedAudioSourceIndex(plan.Streams.Audio, plan.Override)
		args = append(args, "-map", fmt.Sprintf("0:%d", enhancedSourceIndex))
	} else if needsAACCompatibility && len(selectedAudioStreams) > 0 {
		aacStereoIndex = len(mappedAudioStreams)
		args = append(args, "-map", fmt.Sprintf("0:%d", enhancedAudioSourceIndex(plan.Streams.Audio, plan.Override)))
	}

	// Set copy behavior per stream type before applying explicit overrides.
	// This is clearer than a global -c copy and prevents accidental defaults.
	args = append(args, "-c:v", "copy", "-c:a", "copy", "-c:s", "copy", "-c:d", "copy", "-c:t", "copy")
	if plan.ProcessingMode == ProcessingModeFullEncode {
		effectiveProfile := profileWithAutomaticDeinterlace(plan.Profile, plan.Interlace)
		if len(plan.Streams.Video) > 0 {
			effectiveProfile = profileWithFinalColorPolicy(effectiveProfile, plan.Streams.Video[0], resolvedVideoEncoder(effectiveProfile))
		}
		var source *MediaStream
		if len(plan.Streams.Video) > 0 {
			source = &plan.Streams.Video[0]
		}
		args = append(args, videoCodecArgsForSource(effectiveProfile, source)...)
		args = append(args, videoWorkerArgsForSource(effectiveProfile, source)...)
		args = append(args, videoColorMetadataArgs(effectiveProfile)...)
	} else {
		args = append(args, "-c:v", "copy")
	}

	if enhancedAudioIndex >= 0 {
		filterChain := audioProcessorChain(audioProcessorsForProfile(*plan.AudioProfile))
		codec := plan.AudioProfile.OutputCodec
		if codec == "" || codec == "copy" {
			codec = "aac"
		}
		args = append(args,
			fmt.Sprintf("-filter:a:%d", enhancedAudioIndex), filterChain,
			fmt.Sprintf("-c:a:%d", enhancedAudioIndex), ffmpegCodecName(codec),
			fmt.Sprintf("-metadata:s:a:%d", enhancedAudioIndex), "title="+enhancedAudioTitle(*plan.AudioProfile),
			fmt.Sprintf("-disposition:a:%d", enhancedAudioIndex), "default",
		)
	} else if aacStereoIndex >= 0 {
		disposition := "0"
		if aacStereoDefault {
			disposition = "default"
		}
		args = append(args,
			fmt.Sprintf("-c:a:%d", aacStereoIndex), "aac",
			fmt.Sprintf("-b:a:%d", aacStereoIndex), fmt.Sprintf("%dk", aacStereoBitrateKbps(plan.Profile)),
			fmt.Sprintf("-ac:a:%d", aacStereoIndex), "2",
			fmt.Sprintf("-metadata:s:a:%d", aacStereoIndex), "title=AAC Stereo (MVForge)",
			fmt.Sprintf("-disposition:a:%d", aacStereoIndex), disposition,
		)
	} else {
		args = appendAudioCodecArgs(args, plan.Profile)
	}

	for index, stream := range selectedVideoStreams(plan.Streams.Video, plan.Override) {
		metadata := metadataOverrideFor(plan.Override.VideoMetadata, stream.Index)
		if metadata.Title != "" {
			args = append(args, fmt.Sprintf("-metadata:s:v:%d", index), "title="+metadata.Title)
		}
		if metadata.Language != "" {
			args = append(args, fmt.Sprintf("-metadata:s:v:%d", index), "language="+metadata.Language)
		}
		if metadata.Default != nil || metadata.Forced != nil {
			args = append(args, fmt.Sprintf("-disposition:v:%d", index), streamDisposition(metadata, stream.Default, stream.Forced))
		}
	}

	for index, stream := range mappedAudioStreams {
		metadata := metadataOverrideFor(plan.Override.AudioMetadata, stream.Index)
		title := metadata.Title
		if title == "" {
			title = originalAudioTitle(stream)
		}
		args = append(args, fmt.Sprintf("-metadata:s:a:%d", index), "title="+title)
		language := metadata.Language
		if language == "" {
			language = stream.Language
		}
		if language != "" {
			args = append(args, fmt.Sprintf("-metadata:s:a:%d", index), "language="+language)
		}
		originalDefault := stream.Default
		if enhancedAudioIndex >= 0 || (aacStereoIndex >= 0 && aacStereoDefault) {
			originalDefault = false
		}
		args = append(args, fmt.Sprintf("-disposition:a:%d", index), streamDisposition(metadata, originalDefault, stream.Forced))
	}

	for index, stream := range selectedSubtitleStreams(plan) {
		metadata := metadataOverrideFor(plan.Override.SubtitleMetadata, stream.Index)
		if metadata.Title != "" {
			args = append(args, fmt.Sprintf("-metadata:s:s:%d", index), "title="+metadata.Title)
		}
		if metadata.Language != "" {
			args = append(args, fmt.Sprintf("-metadata:s:s:%d", index), "language="+metadata.Language)
		}
		if metadata.Default != nil || metadata.Forced != nil {
			args = append(args, fmt.Sprintf("-disposition:s:%d", index), streamDisposition(metadata, stream.Default, stream.Forced))
		}
	}

	sourceHasSubtitles := len(plan.Streams.Subtitle) > 0
	hasSelectedSubtitles := len(selectedSubtitleStreams(plan)) > 0
	if sourceHasSubtitles && !planHasStreamSelection(plan.Override) && !plan.Profile.PreserveSubtitles {
		args = append(args, "-sn")
	} else if hasSelectedSubtitles && plan.Profile.PreserveSubtitles {
		subtitleCodec := effectiveSubtitleOutputFormat(plan.Profile)
		for index, stream := range selectedSubtitleStreams(plan) {
			if subtitleCodec != "source" && subtitleCanConvertText(stream.Codec) {
				args = append(args, fmt.Sprintf("-c:s:%d", index), subtitleCodec)
			}
		}
	}
	if plan.Streams.ChapterCount > 0 && plan.Profile.PreserveChapters {
		args = append(args, "-map_chapters", "0")
	} else if plan.Streams.ChapterCount > 0 {
		args = append(args, "-map_chapters", "-1")
	}

	args = append(args, plan.OutputPath)
	return args
}

func aacStereoBitrateKbps(profile models.Profile) int {
	bitrate := workerIntValue(profile.WorkerConfig["aacStereoBitrateKbps"], 192)
	if bitrate < 64 {
		return 64
	}
	if bitrate > 512 {
		return 512
	}
	return bitrate
}

func profileWithAutomaticDeinterlace(profile models.Profile, analysis InterlaceAnalysis) models.Profile {
	mode := workerStringValue(profile.WorkerConfig["deinterlaceMode"])
	if mode == "" {
		mode = workerStringValue(profile.WorkerConfig["deinterlace"])
	}
	filter := effectiveDeinterlaceFilter(mode, analysis)
	fieldMetadataFilter := effectiveAutomaticFieldMetadataFilter(filter, existingVideoFilters(profile), analysis)
	if filter == "" && fieldMetadataFilter == "" {
		return profile
	}
	profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
	existing := strings.TrimSpace(workerStringValue(profile.WorkerConfig["videoFilters"]))
	if existing != "" && !strings.Contains(existing, "bwdif=") && !strings.Contains(existing, "yadif=") {
		if filter != "" {
			filter += ","
		}
		filter += existing
	} else if existing != "" {
		filter = existing
	}
	if fieldMetadataFilter != "" && !strings.Contains(filter, "setfield=") {
		if filter != "" {
			filter += ","
		}
		filter += fieldMetadataFilter
	}
	profile.WorkerConfig["videoFilters"] = filter
	return profile
}

func existingVideoFilters(profile models.Profile) string {
	return strings.TrimSpace(workerStringValue(profile.WorkerConfig["videoFilters"]))
}

func profileWithAutomaticVideoToolboxColor(profile models.Profile, source MediaStream) models.Profile {
	return profileWithFinalColorPolicy(profile, source, resolvedVideoEncoder(profile))
}

func profileWithAutomaticVideoToolboxColorForEncoder(profile models.Profile, source MediaStream, encoder string) models.Profile {
	return profileWithFinalColorPolicy(profile, source, encoder)
}

func profileWithFinalColorPolicy(profile models.Profile, source MediaStream, encoder string) models.Profile {
	profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
	policy := strings.ToLower(strings.TrimSpace(workerStringValue(profile.WorkerConfig["finalColorPolicy"])))
	if policy != "automatic" && policy != "normalize_bt709" {
		policy = "preserve"
	}
	profile.WorkerConfig["effectiveFinalColorPolicy"] = policy
	for _, key := range []string{"outputColorSpace", "outputColorTransfer", "outputColorPrimaries", "outputColorRange", "automaticColorConversion"} {
		delete(profile.WorkerConfig, key)
	}
	matrix, matrixOK := colorspaceColorAlias(source.ColorSpace, "matrix")
	primaries, primariesOK := colorspaceColorAlias(source.ColorPrimaries, "primaries")
	transfer, transferOK := colorspaceColorAlias(source.ColorTransfer, "transfer")
	inputRange, rangeOK := colorspaceColorAlias(source.ColorRange, "range")
	if !matrixOK || !primariesOK || !transferOK || !rangeOK {
		profile.WorkerConfig["colorPolicyWarning"] = "source color characteristics are incomplete or unsupported"
		return profile
	}
	delete(profile.WorkerConfig, "colorPolicyWarning")
	normalize := policy == "normalize_bt709" || (policy == "automatic" && encoder == "hevc_videotoolbox" && !sourceUsesBT709(source) && !sourceUsesHDRTransfer(source))
	if !normalize {
		setProfileOutputColorMetadata(profile.WorkerConfig, matrix, transfer, primaries, inputRange)
		profile.WorkerConfig["effectiveFinalColorPolicy"] = "preserve"
		profile.WorkerConfig["colorPolicyDecision"] = "source characteristics explicitly preserved"
		return profile
	}
	if sourceUsesHDRTransfer(source) {
		setProfileOutputColorMetadata(profile.WorkerConfig, matrix, transfer, primaries, inputRange)
		profile.WorkerConfig["effectiveFinalColorPolicy"] = "preserve"
		profile.WorkerConfig["colorPolicyWarning"] = "BT.709 normalization was skipped for HDR because tone mapping was not requested"
		return profile
	}
	existing := strings.TrimSpace(workerStringValue(profile.WorkerConfig["videoFilters"]))
	if !strings.Contains(strings.ToLower(existing), "colorspace=") {
		colorFilter := fmt.Sprintf(
			"colorspace=ispace=%s:iprimaries=%s:itrc=%s:irange=%s:space=bt709:primaries=bt709:trc=bt709:range=tv",
			matrix, primaries, transfer, inputRange,
		)
		if existing != "" {
			existing += ","
		}
		profile.WorkerConfig["videoFilters"] = existing + colorFilter
	}
	setProfileOutputColorMetadata(profile.WorkerConfig, "bt709", "bt709", "bt709", "tv")
	profile.WorkerConfig["automaticColorConversion"] = "source_to_bt709"
	profile.WorkerConfig["effectiveFinalColorPolicy"] = "normalize_bt709"
	profile.WorkerConfig["colorPolicyDecision"] = "pixel values converted mathematically before BT.709 metadata was written"
	return profile
}

func sourceUsesBT709(source MediaStream) bool {
	return strings.EqualFold(strings.TrimSpace(source.ColorSpace), "bt709") &&
		strings.EqualFold(strings.TrimSpace(source.ColorTransfer), "bt709") &&
		strings.EqualFold(strings.TrimSpace(source.ColorPrimaries), "bt709")
}

func sourceUsesHDRTransfer(source MediaStream) bool {
	switch strings.ToLower(strings.TrimSpace(source.ColorTransfer)) {
	case "smpte2084", "arib-std-b67", "hlg":
		return true
	default:
		return false
	}
}

func normalizedFFmpegRange(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "pc", "full":
		return "pc"
	default:
		return "tv"
	}
}

func setProfileOutputColorMetadata(config models.JSONMap, matrix, transfer, primaries, colorRange string) {
	config["outputColorSpace"] = matrix
	config["outputColorTransfer"] = transfer
	config["outputColorPrimaries"] = primaries
	config["outputColorRange"] = colorRange
}

func videoColorMetadataArgs(profile models.Profile) []string {
	if profile.WorkerConfig == nil {
		return nil
	}
	args := []string{}
	for _, option := range []struct {
		key  string
		flag string
	}{
		{key: "outputColorSpace", flag: "-colorspace"},
		{key: "outputColorTransfer", flag: "-color_trc"},
		{key: "outputColorPrimaries", flag: "-color_primaries"},
		{key: "outputColorRange", flag: "-color_range"},
	} {
		if value := strings.TrimSpace(workerStringValue(profile.WorkerConfig[option.key])); value != "" {
			args = append(args, option.flag, value)
		}
	}
	return args
}

func effectiveAutomaticFieldMetadataFilter(motionFilter, existingFilters string, analysis InterlaceAnalysis) string {
	combined := strings.ToLower(strings.Join([]string{motionFilter, existingFilters}, ","))
	if strings.Contains(combined, "setfield=") {
		return ""
	}
	correctionProducesProgressiveFrames := strings.Contains(combined, "bwdif=") ||
		strings.Contains(combined, "yadif=") ||
		(strings.Contains(combined, "fieldmatch") && strings.Contains(combined, "decimate"))
	if correctionProducesProgressiveFrames {
		return "setfield=prog"
	}
	if analysis.Status == "progressive" && analysis.FieldOrderMismatch {
		return "setfield=prog"
	}
	return ""
}

// FFmpeg can transcode text subtitles to SRT or ASS, but it cannot OCR bitmap
// subtitles such as DVD/VobSub or Blu-ray PGS. Since the command starts with
// "-c copy", leaving those streams untouched preserves them in MKV output.
func subtitleCanConvertText(codec string) bool {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "ass", "ssa", "subrip", "srt", "text", "mov_text", "webvtt", "microdvd", "mpl2", "jacosub", "sami", "realtext", "subviewer", "subviewer1", "vplayer":
		return true
	default:
		return false
	}
}

func effectiveSubtitleOutputFormat(profile models.Profile) string {
	switch strings.ToLower(strings.TrimSpace(workerStringValue(profile.WorkerConfig["subtitleOutputFormat"]))) {
	case "srt":
		return "srt"
	case "ass", "ssa":
		return "ass"
	case "source", "copy":
		return "source"
	}
	if profileWorkerBool(profile, "preferSrtSubtitles", false) {
		return "srt"
	}
	return "source"
}

func effectiveAACOption(plan MediaJobPlan, override *bool, key, legacyKey string, fallback bool) bool {
	if override != nil {
		return *override
	}
	if plan.Profile.WorkerConfig != nil {
		if _, exists := plan.Profile.WorkerConfig[key]; exists {
			return profileWorkerBool(plan.Profile, key, fallback)
		}
		if legacyKey != "" {
			if _, exists := plan.Profile.WorkerConfig[legacyKey]; exists {
				return profileWorkerBool(plan.Profile, legacyKey, fallback)
			}
		}
	}
	return fallback
}

func hasAACStereoStream(streams []MediaAudioStream) bool {
	for _, stream := range streams {
		if strings.EqualFold(strings.TrimSpace(stream.Codec), "aac") && stream.Channels == 2 {
			return true
		}
	}
	return false
}

func buildMediaJobPlan(inputPath string, outputPath string, profile models.Profile, audioProfile *audioEnhancementProfile, overwrite bool) (MediaJobPlan, error) {
	profile = normalizeHardwareQualityPreset(profile)
	streams, err := probeMediaStreams(inputPath)
	if err != nil {
		return MediaJobPlan{}, err
	}
	if len(streams.Video) > 0 && streams.Video[0].Bitrate <= 0 {
		streams.Video[0].Bitrate = estimatedVideoBitrate(streams)
	}

	fieldOrder := "unknown"
	if len(streams.Video) > 0 {
		fieldOrder = streams.Video[0].FieldOrder
	}
	processingMode := mediaProcessingMode(profile)
	analysis := InterlaceAnalysis{Status: interlaceStatusFromFieldOrder(fieldOrder), FieldOrder: normalizeFieldOrder(fieldOrder), Source: "ffprobe"}
	if processingMode == ProcessingModeFullEncode {
		analysis = detectInterlace(inputPath, fieldOrder, streams.Duration, 20)
	}
	return MediaJobPlan{
		InputPath:      inputPath,
		OutputPath:     outputPath,
		Profile:        profile,
		AudioProfile:   audioProfile,
		Overwrite:      overwrite,
		ProcessingMode: processingMode,
		Streams:        streams,
		Interlace:      analysis,
	}, nil
}

func buildMediaJobPlanWithOverride(inputPath string, outputPath string, profile models.Profile, audioProfile *audioEnhancementProfile, overwrite bool, override AssetConversionOverrideState) (MediaJobPlan, error) {
	effectiveProfile := applyAssetConversionOverrideToProfile(profile, override)
	plan, err := buildMediaJobPlan(inputPath, outputPath, effectiveProfile, audioProfile, overwrite)
	if err != nil {
		return MediaJobPlan{}, err
	}
	plan.Override, plan.StreamValidationWarnings = sanitizeConversionOverride(override, plan.Streams)
	applyProfileSubtitleExternalization(&plan)
	plan.ProcessingMode = mediaProcessingMode(effectiveProfile)
	// audio_only represents "no video profile selected" while the queue still
	// carries a technical fallback profile. full_encode is derived from the
	// effective video codec so a copy profile cannot accidentally encode.
	if normalizeQueueProcessingMode(override.ProcessingMode) == ProcessingModeAudioOnly {
		plan.ProcessingMode = ProcessingModeAudioOnly
	}
	return plan, nil
}

func applyProfileSubtitleExternalization(plan *MediaJobPlan) {
	if plan == nil || strings.TrimSpace(plan.Override.TrackProfileKey) != "" || len(plan.Override.SubtitleTransforms) > 0 {
		return
	}
	format := effectiveExternalSubtitleFormat(plan.Profile)
	if format == "" {
		return
	}
	transforms := make([]SubtitleTransform, 0, len(plan.Streams.Subtitle))
	ocrMode := normalizedOCRMode(workerStringValue(plan.Profile.WorkerConfig["subtitleOCRMode"]))
	forcedOCRLanguage := strings.ToLower(strings.TrimSpace(workerStringValue(plan.Profile.WorkerConfig["subtitleOCRLanguage"])))
	for _, stream := range plan.Streams.Subtitle {
		ocrLanguage := stream.Language
		if forcedOCRLanguage != "" && forcedOCRLanguage != "auto" {
			ocrLanguage = forcedOCRLanguage
		}
		transforms = append(transforms, SubtitleTransform{
			StreamIndex:    stream.Index,
			Format:         format,
			RemoveEmbedded: true,
			MakeDefault:    stream.Default,
			Language:       stream.Language,
			OCRLanguage:    ocrLanguage,
			OCRMode:        ocrMode,
			Title:          stream.Title,
		})
	}
	plan.Override.SubtitleTransforms = normalizedSubtitleTransforms(transforms)
	if len(plan.Override.SubtitleTransforms) > 0 {
		plan.Override.KeepSubtitleStreams = nil
		plan.StreamValidationWarnings = append(plan.StreamValidationWarnings,
			fmt.Sprintf("Video profile will externalize all subtitle tracks as %s; embedded subtitle tracks are removed only after every sidecar passes validation.", strings.ToUpper(format)))
	}
}

func effectiveExternalSubtitleFormat(profile models.Profile) string {
	configured := strings.ToLower(strings.TrimSpace(workerStringValue(profile.WorkerConfig["externalSubtitleFormat"])))
	if configured == "" {
		// Profiles saved before externalSubtitleFormat used subtitleOutputFormat.
		configured = strings.ToLower(strings.TrimSpace(workerStringValue(profile.WorkerConfig["subtitleOutputFormat"])))
	}
	switch configured {
	case "srt":
		return "srt"
	case "ass", "ssa":
		return "ass"
	default:
		return ""
	}
}

func sanitizeConversionOverride(override AssetConversionOverrideState, streams MediaStreamInventory) (AssetConversionOverrideState, []string) {
	warnings := []string{}
	override.KeepVideoStreams = sanitizeSelectedStreams("video", override.KeepVideoStreams, streamIndexes(streams.Video), &warnings)
	override.KeepAudioStreams = sanitizeSelectedStreams("audio", override.KeepAudioStreams, audioStreamIndexes(streams.Audio), &warnings)
	override.KeepSubtitleStreams = sanitizeSelectedStreams("subtitle", override.KeepSubtitleStreams, streamIndexes(streams.Subtitle), &warnings)
	override.VideoMetadata = sanitizeStreamMetadata("video metadata", override.VideoMetadata, streamIndexes(streams.Video), &warnings)
	override.AudioMetadata = sanitizeStreamMetadata("audio metadata", override.AudioMetadata, audioStreamIndexes(streams.Audio), &warnings)
	override.SubtitleMetadata = sanitizeStreamMetadata("subtitle metadata", override.SubtitleMetadata, streamIndexes(streams.Subtitle), &warnings)
	override.SubtitleTransforms = sanitizeSubtitleTransforms(override.SubtitleTransforms, streams.Subtitle, &warnings)
	if override.EnhancedAudioSourceStreamIndex != nil {
		if _, exists := intSet(audioStreamIndexes(streams.Audio))[*override.EnhancedAudioSourceStreamIndex]; !exists {
			warnings = append(warnings, fmt.Sprintf("Enhanced audio source stream %d is not present and was replaced with the available default audio stream.", *override.EnhancedAudioSourceStreamIndex))
			override.EnhancedAudioSourceStreamIndex = nil
		}
	}
	return override, warnings
}

func sanitizeSubtitleTransforms(values []SubtitleTransform, streams []MediaStream, warnings *[]string) []SubtitleTransform {
	available := map[int]MediaStream{}
	for _, stream := range streams {
		available[stream.Index] = stream
	}
	result := []SubtitleTransform{}
	for _, value := range normalizedSubtitleTransforms(values) {
		stream, exists := available[value.StreamIndex]
		if !exists {
			*warnings = append(*warnings, fmt.Sprintf("Subtitle transformation for missing stream %d was omitted.", value.StreamIndex))
			continue
		}
		if !subtitleCanConvertText(stream.Codec) {
			if !isBitmapSubtitleCodecName(stream.Codec) {
				*warnings = append(*warnings, fmt.Sprintf("Subtitle stream %d (%s) cannot be transformed and was preserved.", stream.Index, stream.Codec))
				continue
			}
			*warnings = append(*warnings, fmt.Sprintf("Subtitle stream %d (%s) will run OCR before media conversion.", stream.Index, stream.Codec))
		}
		result = append(result, value)
	}
	return result
}

func sanitizeSelectedStreams(label string, selected []int, available []int, warnings *[]string) []int {
	if selected == nil {
		return nil
	}
	availableSet := intSet(available)
	valid := make([]int, 0, len(selected))
	for _, index := range selected {
		if _, exists := availableSet[index]; exists {
			valid = append(valid, index)
			continue
		}
		*warnings = append(*warnings, fmt.Sprintf("Selected %s stream %d is not present and was omitted.", label, index))
	}
	return valid
}

func sanitizeStreamMetadata(label string, metadata map[int]StreamMetadataOverride, available []int, warnings *[]string) map[int]StreamMetadataOverride {
	if metadata == nil {
		return nil
	}
	availableSet := intSet(available)
	valid := make(map[int]StreamMetadataOverride, len(metadata))
	for index, value := range metadata {
		if _, exists := availableSet[index]; exists {
			valid[index] = value
			continue
		}
		*warnings = append(*warnings, fmt.Sprintf("%s for stream %d was omitted because the stream is not present.", label, index))
	}
	return valid
}

func applyAssetConversionOverrideToProfile(profile models.Profile, override AssetConversionOverrideState) models.Profile {
	if value := strings.TrimSpace(override.VideoCodec); value != "" {
		profile.VideoCodec = value
	}
	if value := strings.TrimSpace(override.AudioCodec); value != "" {
		profile.AudioCodec = value
	}
	if value := strings.TrimSpace(override.QualityMode); value != "" {
		profile.QualityMode = value
	}
	if override.QualityValue > 0 {
		profile.QualityValue = override.QualityValue
	}
	if override.PreserveHDR != nil {
		profile.PreserveHDR = *override.PreserveHDR
	}
	if override.PreserveSubtitles != nil {
		profile.PreserveSubtitles = *override.PreserveSubtitles
	}
	if override.PreserveChapters != nil {
		profile.PreserveChapters = *override.PreserveChapters
	}

	if profile.WorkerConfig == nil {
		profile.WorkerConfig = models.JSONMap{}
	}
	workerConfig := models.JSONMap{}
	for key, value := range profile.WorkerConfig {
		workerConfig[key] = value
	}
	if value := strings.ToLower(strings.TrimSpace(override.ExternalSubtitleFormat)); value == "source" || value == "srt" || value == "ass" {
		workerConfig["externalSubtitleFormat"] = value
		workerConfig["subtitleOutputFormat"] = "source"
		profile.PreserveSubtitles = value == "source"
	}
	if value := strings.ToLower(strings.TrimSpace(override.FinalColorPolicy)); value == "automatic" || value == "preserve" || value == "normalize_bt709" {
		workerConfig["finalColorPolicy"] = value
	}
	if value := strings.TrimSpace(override.VideoPreset); value != "" {
		workerConfig["videoPreset"] = value
	}
	if value := strings.TrimSpace(override.PixFmt); value != "" {
		workerConfig["pixFmt"] = value
	}
	if value := strings.TrimSpace(override.VideoFilters); value != "" {
		workerConfig["videoFilters"] = value
	}
	if value := strings.TrimSpace(override.X265Params); value != "" {
		workerConfig["x265Params"] = value
	}
	if value := strings.TrimSpace(override.DeinterlaceMode); value != "" {
		workerConfig["deinterlaceMode"] = value
	}
	if override.UseHardwareIfAvailable != nil {
		workerConfig["useHardwareIfAvailable"] = *override.UseHardwareIfAvailable
	}
	if value := strings.TrimSpace(override.VideoEncoder); value != "" {
		workerConfig["videoEncoder"] = value
	}
	if value := strings.TrimSpace(override.PreferredEncoder); value != "" {
		workerConfig["preferredEncoder"] = value
	}
	switch strings.ToLower(strings.TrimSpace(override.PreferredEncoder)) {
	case "software":
		workerConfig["useHardwareIfAvailable"] = false
		// "auto" with hardware disabled resolves to the software encoder for
		// the selected VideoCodec (libx264, libx265, libsvtav1, etc.).
		workerConfig["videoEncoder"] = "auto"
	case "auto":
		workerConfig["useHardwareIfAvailable"] = len(hardwareEncoderCandidatesForVideoCodec(profile.VideoCodec)) > 0
		workerConfig["videoEncoder"] = "auto"
	case "hardware":
		hardwareSupported := len(hardwareEncoderCandidatesForVideoCodec(profile.VideoCodec)) > 0
		workerConfig["useHardwareIfAvailable"] = hardwareSupported
		encoder := strings.ToLower(strings.TrimSpace(workerStringValue(workerConfig["videoEncoder"])))
		if !hardwareSupported || encoder == "" || encoder == "libx265" || encoder == "libx264" || encoder == "libsvtav1" {
			workerConfig["videoEncoder"] = "auto"
		}
	}
	if override.GlobalQuality > 0 {
		workerConfig["globalQuality"] = override.GlobalQuality
	}
	if value := strings.TrimSpace(override.QSVRateControl); value != "" {
		workerConfig["qsvRateControl"] = value
	}
	if override.QSVLookAheadDepth > 0 {
		workerConfig["qsvLookAheadDepth"] = override.QSVLookAheadDepth
	}
	if override.QSVExtendedBRC != nil {
		workerConfig["qsvExtendedBRC"] = *override.QSVExtendedBRC
	}
	if override.QSVAdaptiveI != nil {
		workerConfig["qsvAdaptiveI"] = *override.QSVAdaptiveI
	}
	if override.QSVAdaptiveB != nil {
		workerConfig["qsvAdaptiveB"] = *override.QSVAdaptiveB
	}
	if override.VideoToolboxBitrateMbps > 0 {
		workerConfig["videoToolboxBitrateMbps"] = override.VideoToolboxBitrateMbps
	}
	if override.VideoToolboxMaxrateMbps > 0 {
		workerConfig["videoToolboxMaxrateMbps"] = override.VideoToolboxMaxrateMbps
	}
	if override.VideoToolboxBufferMbps > 0 {
		workerConfig["videoToolboxBufferMbps"] = override.VideoToolboxBufferMbps
	}
	delete(workerConfig, "videoToolboxQualityProfile")
	if strings.TrimSpace(override.VideoToolboxProfile) != "" {
		workerConfig["videoToolboxProfile"] = override.VideoToolboxProfile
	}
	if override.VideoToolboxGOP > 0 {
		workerConfig["videoToolboxGop"] = override.VideoToolboxGOP
	}
	if override.VideoToolboxRealtime != nil {
		workerConfig["videoToolboxRealtime"] = *override.VideoToolboxRealtime
	}
	if override.VideoToolboxAllowFrameReordering != nil {
		workerConfig["videoToolboxAllowFrameReordering"] = *override.VideoToolboxAllowFrameReordering
	}
	if override.VideoToolboxPowerEfficiency != nil {
		workerConfig["videoToolboxPowerEfficiency"] = *override.VideoToolboxPowerEfficiency
	}
	if strings.TrimSpace(override.HardwareQualityPreset) != "" {
		workerConfig["hardwareQualityPreset"] = override.HardwareQualityPreset
	}
	profile.WorkerConfig = workerConfig
	return normalizeHardwareQualityPreset(profile)
}

func planHasStreamSelection(override AssetConversionOverrideState) bool {
	return override.KeepVideoStreams != nil || override.KeepAudioStreams != nil || override.KeepSubtitleStreams != nil || len(override.SubtitleTransforms) > 0
}

func appendSelectedStreamMaps(args []string, plan MediaJobPlan) []string {
	for _, index := range selectedStreamIndexes(streamIndexes(plan.Streams.Video), plan.Override.KeepVideoStreams) {
		args = append(args, "-map", fmt.Sprintf("0:%d", index))
	}
	for _, index := range selectedStreamIndexes(audioStreamIndexes(plan.Streams.Audio), plan.Override.KeepAudioStreams) {
		args = append(args, "-map", fmt.Sprintf("0:%d", index))
	}
	if plan.Profile.PreserveSubtitles {
		removed := removedEmbeddedSubtitleSet(plan.Override.SubtitleTransforms)
		for _, index := range selectedStreamIndexes(streamIndexes(plan.Streams.Subtitle), plan.Override.KeepSubtitleStreams) {
			if _, remove := removed[index]; remove {
				continue
			}
			args = append(args, "-map", fmt.Sprintf("0:%d", index))
		}
	}
	return args
}

func appendNonAudioStreamMaps(args []string, plan MediaJobPlan) []string {
	for _, index := range streamIndexes(plan.Streams.Video) {
		args = append(args, "-map", fmt.Sprintf("0:%d", index))
	}
	if plan.Profile.PreserveSubtitles {
		for _, index := range streamIndexes(plan.Streams.Subtitle) {
			args = append(args, "-map", fmt.Sprintf("0:%d", index))
		}
	}
	return args
}

func selectedVideoStreams(streams []MediaStream, override AssetConversionOverrideState) []MediaStream {
	if override.KeepVideoStreams == nil {
		return streams
	}
	allowed := intSet(override.KeepVideoStreams)
	selected := []MediaStream{}
	for _, stream := range streams {
		if _, ok := allowed[stream.Index]; ok {
			selected = append(selected, stream)
		}
	}
	return selected
}

func selectedAudioStreams(streams []MediaAudioStream, override AssetConversionOverrideState) []MediaAudioStream {
	if override.KeepAudioStreams == nil {
		return streams
	}
	allowed := intSet(override.KeepAudioStreams)
	selected := []MediaAudioStream{}
	for _, stream := range streams {
		if _, ok := allowed[stream.Index]; ok {
			selected = append(selected, stream)
		}
	}
	return selected
}

func selectedSubtitleStreams(plan MediaJobPlan) []MediaStream {
	if !plan.Profile.PreserveSubtitles {
		return []MediaStream{}
	}
	selectedIndexes := selectedStreamIndexes(streamIndexes(plan.Streams.Subtitle), plan.Override.KeepSubtitleStreams)
	allowed := intSet(selectedIndexes)
	removed := removedEmbeddedSubtitleSet(plan.Override.SubtitleTransforms)
	streams := []MediaStream{}
	for _, stream := range plan.Streams.Subtitle {
		if _, remove := removed[stream.Index]; remove {
			continue
		}
		if _, ok := allowed[stream.Index]; ok {
			streams = append(streams, stream)
		}
	}
	return streams
}

func removedEmbeddedSubtitleSet(values []SubtitleTransform) map[int]struct{} {
	result := map[int]struct{}{}
	for _, value := range values {
		if value.RemoveEmbedded {
			result[value.StreamIndex] = struct{}{}
		}
	}
	return result
}

func metadataOverrideFor(metadata map[int]StreamMetadataOverride, index int) StreamMetadataOverride {
	if metadata == nil {
		return StreamMetadataOverride{}
	}
	return metadata[index]
}

func streamDisposition(metadata StreamMetadataOverride, defaultValue bool, forcedValue bool) string {
	flags := []string{}
	isDefault := defaultValue
	isForced := forcedValue
	if metadata.Default != nil {
		isDefault = *metadata.Default
	}
	if metadata.Forced != nil {
		isForced = *metadata.Forced
	}
	if isDefault {
		flags = append(flags, "default")
	}
	if isForced {
		flags = append(flags, "forced")
	}
	if len(flags) == 0 {
		return "0"
	}
	return strings.Join(flags, "+")
}

func enhancedAudioSourceIndex(streams []MediaAudioStream, override AssetConversionOverrideState) int {
	if override.EnhancedAudioSourceStreamIndex != nil {
		for _, stream := range streams {
			if stream.Index == *override.EnhancedAudioSourceStreamIndex {
				return stream.Index
			}
		}
	}
	selected := selectedAudioStreams(streams, override)
	for _, stream := range selected {
		if stream.Default {
			return stream.Index
		}
	}
	if len(selected) > 0 {
		return selected[0].Index
	}
	if len(streams) > 0 {
		return streams[0].Index
	}
	return 0
}

func selectedStreamIndexes(all []int, selected []int) []int {
	if selected == nil {
		return all
	}
	allowed := intSet(selected)
	indexes := []int{}
	for _, index := range all {
		if _, ok := allowed[index]; ok {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

func streamIndexes(streams []MediaStream) []int {
	indexes := []int{}
	for _, stream := range streams {
		indexes = append(indexes, stream.Index)
	}
	return indexes
}

func audioStreamIndexes(streams []MediaAudioStream) []int {
	indexes := []int{}
	for _, stream := range streams {
		indexes = append(indexes, stream.Index)
	}
	return indexes
}

func intSet(values []int) map[int]struct{} {
	set := map[int]struct{}{}
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func appendAudioCodecArgs(args []string, profile models.Profile) []string {
	if profile.AudioCodec == "" || profile.AudioCodec == "copy" {
		return args
	}
	return append(args, splitArgs(codecArgs("-c:a", profile.AudioCodec, "", 0))...)
}

func videoCodecArgs(profile models.Profile) []string {
	return videoCodecArgsForSource(profile, nil)
}

// adaptiveVideoToolboxBitrate uses source-video bitrate for named presets.
// Custom intentionally keeps explicit user bitrate controls unchanged.
func adaptiveVideoToolboxBitrate(profile models.Profile, source *MediaStream) (int, int, int, bool) {
	if source == nil || source.Bitrate <= 0 || strings.ToLower(workerStringValue(profile.WorkerConfig["hardwareQualityPreset"])) == "custom" {
		return 0, 0, 0, false
	}
	preset := strings.ToLower(workerStringValue(profile.WorkerConfig["hardwareQualityPreset"]))
	multiplier, known := map[string]float64{"compact": 0.40, "medium": 0.52, "recommended": 0.65, "best_quality": 0.80, "high_quality": 0.95}[preset]
	if !known {
		return 0, 0, 0, false
	}
	adjustment := 1.0
	outputHeight := effectiveVideoToolboxOutputHeight(profile, source.Height)
	if outputHeight > 0 && outputHeight <= 576 { // DVD / SD detail retention.
		adjustment += 0.075
	}
	target := int(math.Ceil(float64(source.Bitrate) * multiplier * adjustment / 1000.0))
	if floor := videoToolboxBitrateFloorKbps(preset, outputHeight); target < floor {
		target = floor
	}
	if ceiling := videoToolboxBitrateCeilingKbps(preset, outputHeight); ceiling > 0 && target > ceiling {
		target = ceiling
	}
	return target, int(math.Ceil(float64(target) * 1.5)), int(math.Ceil(float64(target) * 2.5)), true
}

func videoToolboxBitrateCeilingKbps(preset string, height int) int {
	if height <= 0 || height > 576 {
		return 0
	}
	return map[string]int{"compact": 2500, "medium": 3200, "recommended": 4000, "best_quality": 5000, "high_quality": 6000}[preset]
}

func effectiveVideoToolboxOutputHeight(profile models.Profile, sourceHeight int) int {
	filters := workerStringValue(profile.WorkerConfig["videoFilters"])
	for _, part := range strings.Split(filters, ",") {
		part = strings.TrimSpace(part)
		if !strings.HasPrefix(part, "crop=") {
			continue
		}
		values := strings.Split(strings.TrimPrefix(part, "crop="), ":")
		if len(values) < 2 {
			continue
		}
		if height := int(parseInt(values[1])); height > 0 {
			return height
		}
	}
	return sourceHeight
}

func videoToolboxBitrateFloorKbps(preset string, height int) int {
	values := map[string][]int{
		"compact": {1500, 2200, 3000, 6000}, "medium": {2000, 3000, 4000, 8000},
		"recommended": {2500, 4000, 5000, 10000}, "best_quality": {3200, 5000, 6000, 12000}, "high_quality": {4000, 6500, 7000, 14000},
	}
	index := 2
	if height > 0 && height <= 576 {
		index = 0
	} else if height <= 720 {
		index = 1
	} else if height > 1080 {
		index = 3
	}
	return values[preset][index]
}

func videoCodecArgsForSource(profile models.Profile, source *MediaStream) []string {
	if profile.VideoCodec == "" || profile.VideoCodec == "copy" {
		return []string{"-c:v", "copy"}
	}
	encoder := resolvedVideoEncoder(profile)
	args := []string{"-c:v", encoder}
	if profile.QualityMode == "crf" && profile.QualityValue > 0 {
		switch encoder {
		case "libx264", "libx265":
			args = append(args, "-crf", fmt.Sprintf("%d", profile.QualityValue))
		case "hevc_qsv":
			if capabilities.CheckEncoder("hevc_qsv").ICQ {
				args = append(args, "-global_quality", fmt.Sprintf("%d", workerIntValue(profile.WorkerConfig["globalQuality"], defaultQSVQuality(profile.QualityValue))))
			} else {
				args = append(args, "-b:v", fmt.Sprintf("%dM", defaultHardwareBitrateMbps(profile.QualityValue)))
			}
		case "hevc_vaapi":
			args = append(args, "-qp", fmt.Sprintf("%d", workerIntValue(profile.WorkerConfig["globalQuality"], defaultQSVQuality(profile.QualityValue))))
		case "hevc_nvenc":
			args = append(args, "-cq", fmt.Sprintf("%d", workerIntValue(profile.WorkerConfig["globalQuality"], profile.QualityValue)))
		case "hevc_videotoolbox":
			if targetKbps, maxrateKbps, bufferKbps, ok := adaptiveVideoToolboxBitrate(profile, source); ok {
				args = append(args, "-b:v", fmt.Sprintf("%dk", targetKbps), "-maxrate", fmt.Sprintf("%dk", maxrateKbps), "-bufsize", fmt.Sprintf("%dk", bufferKbps))
			} else {
				bitrate := workerIntValue(profile.WorkerConfig["videoToolboxBitrateMbps"], defaultVideoToolboxBitrateMbps(profile.QualityValue))
				maxrate := workerIntValue(profile.WorkerConfig["videoToolboxMaxrateMbps"], max(bitrate+1, bitrate*3/2))
				buffer := workerIntValue(profile.WorkerConfig["videoToolboxBufferMbps"], max(bitrate+1, bitrate*5/2))
				args = append(args, "-b:v", fmt.Sprintf("%dM", bitrate), "-maxrate", fmt.Sprintf("%dM", maxrate), "-bufsize", fmt.Sprintf("%dM", buffer))
			}
		case "hevc_amf":
			args = append(args, "-qp_i", fmt.Sprintf("%d", workerIntValue(profile.WorkerConfig["globalQuality"], profile.QualityValue)))
		}
	}
	if encoder == "hevc_qsv" || encoder == "hevc_vaapi" {
		if profileUsesTenBit(profile) {
			args = append(args, "-profile:v", "main10")
		} else {
			args = append(args, "-profile:v", "main")
		}
	}
	if encoder == "hevc_videotoolbox" {
		videoToolboxProfile := strings.ToLower(strings.TrimSpace(workerStringValue(profile.WorkerConfig["videoToolboxProfile"])))
		if videoToolboxProfile != "main" && videoToolboxProfile != "main10" {
			if profileUsesTenBit(profile) {
				videoToolboxProfile = "main10"
			} else {
				videoToolboxProfile = "main"
			}
		}
		if gop := workerIntValue(profile.WorkerConfig["videoToolboxGop"], 0); gop > 0 {
			args = append(args, "-g", fmt.Sprintf("%d", gop))
		}
		if value, ok := profile.WorkerConfig["videoToolboxAllowFrameReordering"].(bool); ok {
			args = append(args, "-bf", map[bool]string{true: "3", false: "0"}[value])
		}
		if value, ok := profile.WorkerConfig["videoToolboxPowerEfficiency"].(bool); ok {
			args = append(args, "-power_efficient", map[bool]string{true: "1", false: "0"}[value])
		}
		if videoToolboxProfile == "main10" {
			args = append(args, "-profile:v", "main10", "-pix_fmt", "p010le")
		} else {
			args = append(args, "-profile:v", "main", "-pix_fmt", "yuv420p")
		}
	}
	if isTenBitVideoCodec(profile.VideoCodec) && encoder == "libx265" {
		args = append(args, "-pix_fmt", "yuv420p10le")
	}
	return args
}

func resolvedVideoEncoder(profile models.Profile) string {
	config := profile.WorkerConfig
	selected := strings.ToLower(strings.TrimSpace(workerStringValue(config["videoEncoder"])))
	if selected == "" {
		selected = strings.ToLower(strings.TrimSpace(workerStringValue(config["encoder"])))
	}
	if selected == "" || selected == "ffmpeg" || selected == "auto" {
		if profileWorkerBool(profile, "useHardwareIfAvailable", false) {
			for _, encoder := range hardwareEncoderCandidatesForVideoCodec(profile.VideoCodec) {
				if ffmpegEncoderAvailableForProfile(encoder, profile) {
					return encoder
				}
			}
		}
		return ffmpegCodecName(profile.VideoCodec)
	}
	encoder := ffmpegCodecName(selected)
	if isHardwareVideoEncoder(encoder) {
		if !stringSliceContains(hardwareEncoderCandidatesForVideoCodec(profile.VideoCodec), encoder) ||
			!encoderMatchesVideoCodec(encoder, profile.VideoCodec) ||
			!profileWorkerBool(profile, "useHardwareIfAvailable", false) ||
			!ffmpegEncoderAvailableForProfile(encoder, profile) {
			return ffmpegCodecName(profile.VideoCodec)
		}
	}
	return encoder
}

func hardwareEncoderCandidatesForVideoCodec(codec string) []string {
	switch videoCodecFamily(codec) {
	case "hevc":
		return []string{"hevc_qsv", "hevc_vaapi", "hevc_videotoolbox", "hevc_nvenc", "hevc_amf"}
	default:
		// MVForge currently smoke-tests HEVC hardware encoders only. Other
		// codec families remain on their matching software encoder.
		return nil
	}
}

func videoCodecFamily(codec string) string {
	codec = strings.ToLower(strings.TrimSpace(codec))
	switch {
	case codec == "copy":
		return "copy"
	case strings.Contains(codec, "265") || strings.Contains(codec, "hevc"):
		return "hevc"
	case strings.Contains(codec, "264"):
		return "h264"
	case strings.Contains(codec, "av1"):
		return "av1"
	default:
		return codec
	}
}

func encoderMatchesVideoCodec(encoder, codec string) bool {
	encoder = strings.ToLower(strings.TrimSpace(encoder))
	family := videoCodecFamily(codec)
	switch family {
	case "hevc":
		return strings.HasPrefix(encoder, "hevc_")
	case "h264":
		return strings.HasPrefix(encoder, "h264_")
	case "av1":
		return strings.HasPrefix(encoder, "av1_")
	default:
		return false
	}
}

func videoWorkerArgs(profile models.Profile) []string {
	return videoWorkerArgsForSource(profile, nil)
}

func videoWorkerArgsForSource(profile models.Profile, source *MediaStream) []string {
	if profile.WorkerConfig == nil {
		return nil
	}
	codec := strings.ToLower(strings.TrimSpace(profile.VideoCodec))
	if codec == "" || codec == "copy" {
		return nil
	}

	args := []string{}
	encoder := resolvedVideoEncoder(profile)
	filters := workerStringValue(profile.WorkerConfig["videoFilters"])
	filters = applyCropAspectPolicy(filters, profile, source)
	if encoder == "hevc_vaapi" {
		format := "nv12"
		if profileUsesTenBit(profile) {
			format = "p010le"
		}
		if filters != "" {
			filters += ","
		}
		filters += "format=" + format + ",hwupload"
		args = append(args, "-vaapi_device", "/dev/dri/renderD128")
	} else if encoder == "hevc_qsv" {
		args = append(args, "-init_hw_device", "qsv=hw,child_device=/dev/dri/renderD128")
	}
	if filters != "" {
		args = append(args, "-vf", filters)
	}
	if encoder != "hevc_videotoolbox" && encoder != "hevc_vaapi" {
		if preset := workerStringValue(profile.WorkerConfig["videoPreset"]); preset != "" {
			args = append(args, "-preset", preset)
		} else if preset := workerStringValue(profile.WorkerConfig["preset"]); preset != "" && preset != "profile-lab" {
			args = append(args, "-preset", preset)
		}
	}
	if pixFmt := workerStringValue(profile.WorkerConfig["pixFmt"]); pixFmt != "" && pixFmt != "auto" && encoder != "hevc_videotoolbox" && encoder != "hevc_vaapi" {
		if encoder == "hevc_qsv" {
			switch pixFmt {
			case "yuv420p10le":
				pixFmt = "p010le"
			case "yuv420p":
				pixFmt = "nv12"
			}
		}
		args = append(args, "-pix_fmt", pixFmt)
	}
	if params := workerStringValue(profile.WorkerConfig["x265Params"]); params != "" && resolvedVideoEncoder(profile) == "libx265" {
		args = append(args, "-x265-params", params)
	}
	if encoder == "hevc_qsv" {
		args = append(args, qsvWorkerArgs(profile)...)
	}
	return args
}

func applyCropAspectPolicy(filters string, profile models.Profile, source *MediaStream) string {
	if source == nil || strings.ToLower(workerStringValue(profile.WorkerConfig["cropAspectPolicy"])) == "source_sar" {
		return filters
	}
	policy := strings.ToLower(workerStringValue(profile.WorkerConfig["cropAspectPolicy"]))
	if policy != "" && policy != "preserve_dar" {
		return filters
	}
	var cropWidth, cropHeight int
	for _, part := range strings.Split(filters, ",") {
		if !strings.HasPrefix(strings.TrimSpace(part), "crop=") {
			continue
		}
		values := strings.Split(strings.TrimPrefix(strings.TrimSpace(part), "crop="), ":")
		if len(values) < 2 {
			continue
		}
		cropWidth, cropHeight = int(parseInt(values[0])), int(parseInt(values[1]))
		break
	}
	if cropWidth <= 0 || cropHeight <= 0 || source.Width <= 0 || source.Height <= 0 {
		return filters
	}
	sarNum, sarDen := aspectRatioParts(source.SampleAspectRatio)
	if sarNum <= 0 || sarDen <= 0 {
		return filters
	}
	// Preserve the displayed DAR: DARsrc=(W/H)*(SARsrc), SARout=DARsrc/(cropW/cropH).
	numerator := source.Width * sarNum * cropHeight
	denominator := source.Height * sarDen * cropWidth
	if numerator <= 0 || denominator <= 0 {
		return filters
	}
	return filters + fmt.Sprintf(",setsar=%d/%d,setdar=%d/%d", numerator, denominator, source.Width*sarNum, source.Height*sarDen)
}

func aspectRatioParts(value string) (int, int) {
	parts := strings.FieldsFunc(strings.TrimSpace(value), func(r rune) bool { return r == ':' || r == '/' })
	if len(parts) != 2 {
		return 0, 0
	}
	return int(parseInt(parts[0])), int(parseInt(parts[1]))
}

func qsvWorkerArgs(profile models.Profile) []string {
	return qsvWorkerArgsForCapability(profile, capabilities.CheckEncoder("hevc_qsv"))
}

func qsvWorkerArgsForCapability(profile models.Profile, capability capabilities.EncoderCapability) []string {
	args := []string{}
	if profileWorkerBool(profile, "qsvLowPower", false) && capability.LowPower {
		args = append(args, "-low_power", "1")
	}
	rateControl := strings.ToLower(strings.TrimSpace(workerStringValue(profile.WorkerConfig["qsvRateControl"])))
	lookAheadEnabled := rateControl == "la_icq" && capability.LookAhead
	if lookAheadEnabled {
		args = append(args, "-look_ahead", "1")
	}
	lookAheadDepth := workerIntValue(profile.WorkerConfig["qsvLookAheadDepth"], 0)
	if lookAheadEnabled && lookAheadDepth > 0 {
		args = append(args, "-look_ahead_depth", fmt.Sprintf("%d", min(100, max(10, lookAheadDepth))))
	}
	advancedCombination := capability.QSVFullCombination
	if profileWorkerBool(profile, "qsvExtendedBRC", false) && capability.ExtendedBRC && advancedCombination {
		args = append(args, "-extbrc", "1")
		if !lookAheadEnabled && lookAheadDepth > 0 {
			args = append(args, "-look_ahead_depth", fmt.Sprintf("%d", min(100, max(10, lookAheadDepth))))
		}
	}
	if profileWorkerBool(profile, "qsvAdaptiveI", false) && capability.AdaptiveI && advancedCombination {
		args = append(args, "-adaptive_i", "1")
	}
	if profileWorkerBool(profile, "qsvAdaptiveB", false) && capability.AdaptiveB && advancedCombination {
		args = append(args, "-adaptive_b", "1")
	}
	return args
}

func defaultHardwareBitrateMbps(softwareCRF int) int {
	switch {
	case softwareCRF <= 18:
		return 8
	case softwareCRF <= 20:
		return 6
	case softwareCRF <= 23:
		return 4
	default:
		return 3
	}
}

func ffmpegEncoderAvailable(encoder string) bool {
	return capabilities.CheckEncoder(encoder).Usable
}

func ffmpegEncoderAvailableForProfile(encoder string, profile models.Profile) bool {
	capability := capabilities.CheckEncoder(encoder)
	if !capability.Usable {
		return false
	}
	return (encoder != "hevc_qsv" && encoder != "hevc_vaapi") || !profileUsesTenBit(profile) || capability.Main10
}

func defaultQSVQuality(softwareCRF int) int {
	if softwareCRF <= 0 {
		return 18
	}
	if softwareCRF <= 20 {
		return max(15, softwareCRF-2)
	}
	return min(30, 18+(3*(softwareCRF-20)+1)/2)
}

func profileUsesTenBit(profile models.Profile) bool {
	pixelFormat := strings.ToLower(strings.TrimSpace(workerStringValue(profile.WorkerConfig["pixFmt"])))
	switch pixelFormat {
	case "nv12", "yuv420p":
		return false
	case "p010", "p010le", "yuv420p10", "yuv420p10le":
		return true
	}
	if profile.BitDepth >= 10 || isTenBitVideoCodec(profile.VideoCodec) {
		return true
	}
	return strings.Contains(pixelFormat, "10") || strings.Contains(pixelFormat, "p010")
}

func defaultVideoToolboxBitrateMbps(quality int) int {
	switch {
	case quality > 0 && quality <= 18:
		return 8
	case quality >= 25:
		return 4
	default:
		return 6
	}
}

func isHardwareVideoEncoder(encoder string) bool {
	encoder = strings.ToLower(strings.TrimSpace(encoder))
	for _, suffix := range []string{"_qsv", "_vaapi", "_nvenc", "_videotoolbox", "_amf"} {
		if strings.HasSuffix(encoder, suffix) {
			return true
		}
	}
	return false
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func profileWorkerBool(profile models.Profile, key string, fallback bool) bool {
	if profile.WorkerConfig == nil {
		return fallback
	}
	switch typed := profile.WorkerConfig[key].(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes", "on", "enabled":
			return true
		case "false", "0", "no", "off", "disabled":
			return false
		}
	}
	return fallback
}

func cloneWorkerConfig(config models.JSONMap) models.JSONMap {
	cloned := models.JSONMap{}
	for key, value := range config {
		cloned[key] = value
	}
	return cloned
}

func applySelectedEncoder(profile models.Profile, selectedEncoder string) models.Profile {
	selectedEncoder = strings.TrimSpace(selectedEncoder)
	if selectedEncoder == "" {
		return profile
	}
	profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
	profile.WorkerConfig["videoEncoder"] = selectedEncoder
	return profile
}

func workerIntValue(value interface{}, fallback int) int {
	number := workerNumberValue(value, float64(fallback))
	if number <= 0 {
		return fallback
	}
	return int(number)
}

func mediaProcessingMode(profile models.Profile) string {
	videoCodec := strings.ToLower(strings.TrimSpace(profile.VideoCodec))
	if videoCodec == "" || videoCodec == "copy" || videoCodec == "none" {
		return ProcessingModeAudioOnly
	}
	return ProcessingModeFullEncode
}

func audioProcessorsForProfile(profile audioEnhancementProfile) []AudioProcessor {
	processors := []AudioProcessor{}
	if rnnoise := rnnoiseFilter(profile.RNNoiseModelPath); rnnoise != "" {
		processors = append(processors, AudioFilterProcessor{name: "arnndn", filter: rnnoise})
	}
	if channel := channelFilter(profile); channel != "" {
		processors = append(processors, AudioFilterProcessor{name: "channel", filter: channel})
	}
	if baseFilters := normalizedBaseAudioFilters(profile); baseFilters != "" {
		processors = append(processors, AudioFilterProcessor{name: "base", filter: baseFilters})
	}
	if eq := eqFilterChain(profile.EQBands); eq != "" {
		processors = append(processors, AudioFilterProcessor{name: "equalizer", filter: eq})
	}
	return processors
}

func audioProcessorChain(processors []AudioProcessor) string {
	filters := []string{}
	for _, processor := range processors {
		if err := processor.Validate(); err != nil {
			continue
		}
		if filter := processor.Build(); filter != "" {
			filters = append(filters, filter)
		}
	}
	if len(filters) == 0 {
		return "anull"
	}
	return sanitizeAudioFilterChain(strings.Join(filters, ","))
}

func probeMediaStreams(inputPath string) (MediaStreamInventory, error) {
	cmd := exec.Command(
		"ffprobe",
		"-v", "error",
		"-show_entries", "format=duration,bit_rate:stream=index,codec_type,codec_name,channels,channel_layout,field_order,pix_fmt,color_range,color_space,color_transfer,color_primaries,width,height,bit_rate,avg_frame_rate,sample_aspect_ratio,display_aspect_ratio:stream_tags=language,title:stream_disposition=default,forced",
		"-show_chapters",
		"-of", "json",
		inputPath,
	)
	var output bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return MediaStreamInventory{}, fmt.Errorf("ffprobe audio stream scan failed: %s", message)
	}

	var response struct {
		Format struct {
			Duration string `json:"duration"`
			Bitrate  string `json:"bit_rate"`
		} `json:"format"`
		Streams []struct {
			Index              int               `json:"index"`
			CodecType          string            `json:"codec_type"`
			CodecName          string            `json:"codec_name"`
			Channels           int               `json:"channels"`
			ChannelLayout      string            `json:"channel_layout"`
			FieldOrder         string            `json:"field_order"`
			PixelFormat        string            `json:"pix_fmt"`
			ColorRange         string            `json:"color_range"`
			ColorSpace         string            `json:"color_space"`
			ColorTransfer      string            `json:"color_transfer"`
			ColorPrimaries     string            `json:"color_primaries"`
			Width              int               `json:"width"`
			Height             int               `json:"height"`
			Bitrate            string            `json:"bit_rate"`
			FrameRate          string            `json:"avg_frame_rate"`
			SampleAspectRatio  string            `json:"sample_aspect_ratio"`
			DisplayAspectRatio string            `json:"display_aspect_ratio"`
			Tags               map[string]string `json:"tags"`
			Disposition        struct {
				Default int `json:"default"`
				Forced  int `json:"forced"`
			} `json:"disposition"`
		} `json:"streams"`
		Chapters []struct {
			ID int `json:"id"`
		} `json:"chapters"`
	}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		return MediaStreamInventory{}, err
	}

	inventory := MediaStreamInventory{Video: []MediaStream{}, Audio: []MediaAudioStream{}, Subtitle: []MediaStream{}, Duration: parseFloat(response.Format.Duration), ChapterCount: len(response.Chapters), TotalBitrate: parseInt(response.Format.Bitrate)}
	for _, stream := range response.Streams {
		common := MediaStream{
			Index:              stream.Index,
			Codec:              stream.CodecName,
			FieldOrder:         stream.FieldOrder,
			PixelFormat:        stream.PixelFormat,
			ColorRange:         stream.ColorRange,
			ColorSpace:         stream.ColorSpace,
			ColorTransfer:      stream.ColorTransfer,
			ColorPrimaries:     stream.ColorPrimaries,
			Language:           stream.Tags["language"],
			Title:              stream.Tags["title"],
			Default:            stream.Disposition.Default == 1,
			Forced:             stream.Disposition.Forced == 1,
			Width:              stream.Width,
			Height:             stream.Height,
			Bitrate:            parseInt(stream.Bitrate),
			FrameRate:          stream.FrameRate,
			SampleAspectRatio:  stream.SampleAspectRatio,
			DisplayAspectRatio: stream.DisplayAspectRatio,
		}
		switch stream.CodecType {
		case "video":
			inventory.Video = append(inventory.Video, common)
		case "audio":
			inventory.Audio = append(inventory.Audio, MediaAudioStream{
				Index:         stream.Index,
				Codec:         stream.CodecName,
				Channels:      stream.Channels,
				ChannelLayout: stream.ChannelLayout,
				Language:      stream.Tags["language"],
				Title:         stream.Tags["title"],
				Default:       stream.Disposition.Default == 1,
				Forced:        stream.Disposition.Forced == 1,
				Bitrate:       parseInt(stream.Bitrate),
			})
		case "subtitle":
			inventory.Subtitle = append(inventory.Subtitle, common)
		}
	}
	return inventory, nil
}

func estimatedVideoBitrate(streams MediaStreamInventory) int64 {
	if streams.TotalBitrate <= 0 {
		return 0
	}
	audioBitrate := int64(0)
	for _, stream := range streams.Audio {
		audioBitrate += stream.Bitrate
	}
	// Leave a small allowance for container/subtitle overhead when the video
	// stream itself does not report bit_rate.
	estimate := streams.TotalBitrate - audioBitrate - streams.TotalBitrate/100
	if estimate < 0 {
		return 0
	}
	return estimate
}

func originalAudioTitle(stream MediaAudioStream) string {
	if stream.Title != "" && strings.Contains(strings.ToLower(stream.Title), "original") {
		if stream.Language == "" || strings.Contains(strings.ToLower(stream.Title), strings.ToLower(audioLanguageLabel(stream.Language))) {
			return stream.Title
		}
		return stream.Title + " · " + audioLanguageLabel(stream.Language)
	}
	prefix := "Original"
	if stream.Language != "" {
		prefix += " " + audioLanguageLabel(stream.Language)
	}
	layout := strings.ToLower(strings.TrimSpace(stream.ChannelLayout))
	if stream.Channels == 1 || layout == "mono" {
		return prefix + " Mono"
	}
	if stream.Channels == 2 || layout == "stereo" {
		return prefix + " Stereo"
	}
	if stream.Channels > 2 {
		return prefix + " Surround"
	}
	return prefix + " Audio"
}

func audioLanguageLabel(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "jpn", "ja":
		return "Japanese"
	case "eng", "en":
		return "English"
	case "spa", "es", "esl", "mex":
		return "Spanish"
	default:
		return strings.ToUpper(strings.TrimSpace(language))
	}
}

func enhancedAudioTitle(profile audioEnhancementProfile) string {
	switch profile.ChannelMode {
	case "dual-mono", "force-stereo", "light-stereo":
		return "Stereo Enhanced (MVForge)"
	case "downmix-mono":
		return "Mono Enhanced (MVForge)"
	default:
		return "Enhanced Audio (MVForge)"
	}
}

func dryRunCommandFromArgs(args []string) string {
	return "ffmpeg " + shellJoin(args)
}
