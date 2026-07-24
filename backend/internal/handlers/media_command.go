package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
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
}

type MediaStream struct {
	Index      int
	Codec      string
	FieldOrder string
	Language   string
	Title      string
	Default    bool
	Forced     bool
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

	args = append(args, "-c", "copy")
	if plan.ProcessingMode == ProcessingModeFullEncode {
		effectiveProfile := profileWithAutomaticDeinterlace(plan.Profile, plan.Interlace)
		args = append(args, videoCodecArgs(effectiveProfile)...)
		args = append(args, videoWorkerArgs(effectiveProfile)...)
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
		if metadata.Language != "" {
			args = append(args, fmt.Sprintf("-metadata:s:a:%d", index), "language="+metadata.Language)
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
	} else if hasSelectedSubtitles && plan.Profile.PreserveSubtitles && profileWorkerBool(plan.Profile, "preferSrtSubtitles", false) {
		for index, stream := range selectedSubtitleStreams(plan) {
			if subtitleCanConvertToSRT(stream.Codec) {
				args = append(args, fmt.Sprintf("-c:s:%d", index), "srt")
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
	if filter == "" {
		return profile
	}
	profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
	existing := strings.TrimSpace(workerStringValue(profile.WorkerConfig["videoFilters"]))
	if existing != "" && !strings.Contains(existing, "bwdif=") && !strings.Contains(existing, "yadif=") {
		filter += "," + existing
	} else if existing != "" {
		filter = existing
	}
	profile.WorkerConfig["videoFilters"] = filter
	return profile
}

// FFmpeg can transcode text subtitles to SRT, but it cannot OCR bitmap
// subtitles such as DVD/VobSub or Blu-ray PGS. Since the command starts with
// "-c copy", leaving those streams untouched preserves them in MKV output.
func subtitleCanConvertToSRT(codec string) bool {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "ass", "ssa", "subrip", "srt", "text", "mov_text", "webvtt", "microdvd", "mpl2", "jacosub", "sami", "realtext", "subviewer", "subviewer1", "vplayer":
		return true
	default:
		return false
	}
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
	streams, err := probeMediaStreams(inputPath)
	if err != nil {
		return MediaJobPlan{}, err
	}

	fieldOrder := "unknown"
	if len(streams.Video) > 0 {
		fieldOrder = streams.Video[0].FieldOrder
	}
	processingMode := mediaProcessingMode(profile, audioProfile)
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
	plan.ProcessingMode = mediaProcessingMode(effectiveProfile, audioProfile)
	return plan, nil
}

func sanitizeConversionOverride(override AssetConversionOverrideState, streams MediaStreamInventory) (AssetConversionOverrideState, []string) {
	warnings := []string{}
	override.KeepVideoStreams = sanitizeSelectedStreams("video", override.KeepVideoStreams, streamIndexes(streams.Video), &warnings)
	override.KeepAudioStreams = sanitizeSelectedStreams("audio", override.KeepAudioStreams, audioStreamIndexes(streams.Audio), &warnings)
	override.KeepSubtitleStreams = sanitizeSelectedStreams("subtitle", override.KeepSubtitleStreams, streamIndexes(streams.Subtitle), &warnings)
	override.VideoMetadata = sanitizeStreamMetadata("video metadata", override.VideoMetadata, streamIndexes(streams.Video), &warnings)
	override.AudioMetadata = sanitizeStreamMetadata("audio metadata", override.AudioMetadata, audioStreamIndexes(streams.Audio), &warnings)
	override.SubtitleMetadata = sanitizeStreamMetadata("subtitle metadata", override.SubtitleMetadata, streamIndexes(streams.Subtitle), &warnings)
	if override.EnhancedAudioSourceStreamIndex != nil {
		if _, exists := intSet(audioStreamIndexes(streams.Audio))[*override.EnhancedAudioSourceStreamIndex]; !exists {
			warnings = append(warnings, fmt.Sprintf("Enhanced audio source stream %d is not present and was replaced with the available default audio stream.", *override.EnhancedAudioSourceStreamIndex))
			override.EnhancedAudioSourceStreamIndex = nil
		}
	}
	return override, warnings
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
	if value := strings.TrimSpace(override.ProcessingMode); value != "" {
		workerConfig["processingMode"] = value
	}
	if value := strings.TrimSpace(override.DeinterlaceMode); value != "" {
		workerConfig["deinterlaceMode"] = value
	}
	profile.WorkerConfig = workerConfig
	return profile
}

func planHasStreamSelection(override AssetConversionOverrideState) bool {
	return override.KeepVideoStreams != nil || override.KeepAudioStreams != nil || override.KeepSubtitleStreams != nil
}

func appendSelectedStreamMaps(args []string, plan MediaJobPlan) []string {
	for _, index := range selectedStreamIndexes(streamIndexes(plan.Streams.Video), plan.Override.KeepVideoStreams) {
		args = append(args, "-map", fmt.Sprintf("0:%d", index))
	}
	for _, index := range selectedStreamIndexes(audioStreamIndexes(plan.Streams.Audio), plan.Override.KeepAudioStreams) {
		args = append(args, "-map", fmt.Sprintf("0:%d", index))
	}
	if plan.Profile.PreserveSubtitles {
		for _, index := range selectedStreamIndexes(streamIndexes(plan.Streams.Subtitle), plan.Override.KeepSubtitleStreams) {
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
	streams := []MediaStream{}
	for _, stream := range plan.Streams.Subtitle {
		if _, ok := allowed[stream.Index]; ok {
			streams = append(streams, stream)
		}
	}
	return streams
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
			args = append(args, "-global_quality", fmt.Sprintf("%d", workerIntValue(profile.WorkerConfig["globalQuality"], profile.QualityValue)))
		case "hevc_nvenc":
			args = append(args, "-cq", fmt.Sprintf("%d", workerIntValue(profile.WorkerConfig["globalQuality"], profile.QualityValue)))
		case "hevc_videotoolbox":
			bitrate := workerIntValue(profile.WorkerConfig["videoToolboxBitrateMbps"], defaultVideoToolboxBitrateMbps(profile.QualityValue))
			maxrate := workerIntValue(profile.WorkerConfig["videoToolboxMaxrateMbps"], max(bitrate+1, bitrate*4/3))
			buffer := workerIntValue(profile.WorkerConfig["videoToolboxBufferMbps"], bitrate*2)
			args = append(args, "-b:v", fmt.Sprintf("%dM", bitrate), "-maxrate", fmt.Sprintf("%dM", maxrate), "-bufsize", fmt.Sprintf("%dM", buffer))
		case "hevc_amf":
			args = append(args, "-qp_i", fmt.Sprintf("%d", workerIntValue(profile.WorkerConfig["globalQuality"], profile.QualityValue)))
		}
	}
	if encoder == "hevc_videotoolbox" {
		if profileUsesTenBit(profile) {
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
			for _, encoder := range []string{"hevc_qsv", "hevc_videotoolbox", "hevc_nvenc", "hevc_amf"} {
				if ffmpegEncoderAvailable(encoder) {
					return encoder
				}
			}
		}
		return ffmpegCodecName(profile.VideoCodec)
	}
	encoder := ffmpegCodecName(selected)
	if isHardwareVideoEncoder(encoder) {
		if !profileWorkerBool(profile, "useHardwareIfAvailable", false) || !ffmpegEncoderAvailable(encoder) {
			return "libx265"
		}
	}
	return encoder
}

func videoWorkerArgs(profile models.Profile) []string {
	if profile.WorkerConfig == nil {
		return nil
	}
	codec := strings.ToLower(strings.TrimSpace(profile.VideoCodec))
	if codec == "" || codec == "copy" {
		return nil
	}

	args := []string{}
	encoder := resolvedVideoEncoder(profile)
	if filters := workerStringValue(profile.WorkerConfig["videoFilters"]); filters != "" {
		args = append(args, "-vf", filters)
	}
	if preset := workerStringValue(profile.WorkerConfig["videoPreset"]); preset != "" && encoder != "hevc_videotoolbox" {
		args = append(args, "-preset", preset)
	} else if preset := workerStringValue(profile.WorkerConfig["preset"]); preset != "" && preset != "profile-lab" {
		args = append(args, "-preset", preset)
	}
	if pixFmt := workerStringValue(profile.WorkerConfig["pixFmt"]); pixFmt != "" && pixFmt != "auto" && encoder != "hevc_videotoolbox" {
		args = append(args, "-pix_fmt", pixFmt)
	}
	if params := workerStringValue(profile.WorkerConfig["x265Params"]); params != "" && resolvedVideoEncoder(profile) == "libx265" {
		args = append(args, "-x265-params", params)
	}
	return args
}

func ffmpegEncoderAvailable(encoder string) bool {
	return capabilities.CheckEncoder(encoder).Usable
}

func profileUsesTenBit(profile models.Profile) bool {
	if profile.BitDepth >= 10 || isTenBitVideoCodec(profile.VideoCodec) {
		return true
	}
	pixelFormat := strings.ToLower(strings.TrimSpace(workerStringValue(profile.WorkerConfig["pixFmt"])))
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
	switch encoder {
	case "hevc_qsv", "hevc_nvenc", "hevc_videotoolbox", "hevc_amf":
		return true
	default:
		return false
	}
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

func mediaProcessingMode(profile models.Profile, audioProfile *audioEnhancementProfile) string {
	if profile.WorkerConfig != nil {
		mode := strings.ToLower(strings.TrimSpace(workerStringValue(profile.WorkerConfig["processingMode"])))
		switch mode {
		case ProcessingModeAudioOnly, "audio-only", "audioonly":
			return ProcessingModeAudioOnly
		case ProcessingModeFullEncode, "full-encode", "fullencode":
			return ProcessingModeFullEncode
		}
	}
	if audioProfile != nil {
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
		"-show_entries", "format=duration:stream=index,codec_type,codec_name,channels,channel_layout,field_order:stream_tags=language,title:stream_disposition=default,forced",
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
		} `json:"format"`
		Streams []struct {
			Index         int               `json:"index"`
			CodecType     string            `json:"codec_type"`
			CodecName     string            `json:"codec_name"`
			Channels      int               `json:"channels"`
			ChannelLayout string            `json:"channel_layout"`
			FieldOrder    string            `json:"field_order"`
			Tags          map[string]string `json:"tags"`
			Disposition   struct {
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

	inventory := MediaStreamInventory{Video: []MediaStream{}, Audio: []MediaAudioStream{}, Subtitle: []MediaStream{}, Duration: parseFloat(response.Format.Duration), ChapterCount: len(response.Chapters)}
	for _, stream := range response.Streams {
		common := MediaStream{
			Index:      stream.Index,
			Codec:      stream.CodecName,
			FieldOrder: stream.FieldOrder,
			Language:   stream.Tags["language"],
			Title:      stream.Tags["title"],
			Default:    stream.Disposition.Default == 1,
			Forced:     stream.Disposition.Forced == 1,
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
			})
		case "subtitle":
			inventory.Subtitle = append(inventory.Subtitle, common)
		}
	}
	return inventory, nil
}

func originalAudioTitle(stream MediaAudioStream) string {
	if stream.Title != "" && strings.Contains(strings.ToLower(stream.Title), "original") {
		return stream.Title
	}
	layout := strings.ToLower(strings.TrimSpace(stream.ChannelLayout))
	if stream.Channels == 1 || layout == "mono" {
		return "Original Mono"
	}
	if stream.Channels == 2 || layout == "stereo" {
		return "Original Stereo"
	}
	if stream.Channels > 2 {
		return "Original Surround"
	}
	return "Original Audio"
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
