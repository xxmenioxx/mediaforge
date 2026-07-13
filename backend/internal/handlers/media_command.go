package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/anuelvs/mediaforge/backend/internal/models"
)

const (
	ProcessingModeAudioOnly  = "audio_only"
	ProcessingModeFullEncode = "full_encode"
)

type MediaJobPlan struct {
	InputPath      string
	OutputPath     string
	Profile        models.Profile
	AudioProfile   *audioEnhancementProfile
	Overwrite      bool
	ProcessingMode string
	Streams        MediaStreamInventory
	Override       AssetConversionOverrideState
}

type MediaStreamInventory struct {
	Video    []MediaStream
	Audio    []MediaAudioStream
	Subtitle []MediaStream
}

type MediaStream struct {
	Index    int
	Codec    string
	Language string
	Title    string
	Default  bool
	Forced   bool
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

var encoderAvailability = struct {
	sync.Mutex
	checked bool
	values  map[string]bool
}{values: map[string]bool{}}

func (FFmpegCommandBuilder) Build(plan MediaJobPlan) []string {
	args := []string{"-hide_banner"}
	if plan.Overwrite {
		args = append(args, "-y")
	} else {
		args = append(args, "-n")
	}

	args = append(args, "-i", plan.InputPath)
	selectedAudioStreams := selectedAudioStreams(plan.Streams.Audio, plan.Override)
	addAACStereoDefault := profileWorkerBool(plan.Profile, "addAacStereoDefault", false)
	preserveOriginalAudio := profileWorkerBool(plan.Profile, "preserveOriginalAudio", true)
	mappedAudioStreams := selectedAudioStreams
	if planHasStreamSelection(plan.Override) {
		args = appendSelectedStreamMaps(args, plan)
	} else if addAACStereoDefault && !preserveOriginalAudio {
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
	} else if addAACStereoDefault && len(plan.Streams.Audio) > 0 {
		aacStereoIndex = len(mappedAudioStreams)
		args = append(args, "-map", fmt.Sprintf("0:%d", enhancedAudioSourceIndex(plan.Streams.Audio, plan.Override)))
	}

	args = append(args, "-c", "copy")
	if plan.ProcessingMode == ProcessingModeFullEncode {
		args = append(args, videoCodecArgs(plan.Profile)...)
		args = append(args, videoWorkerArgs(plan.Profile)...)
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
		args = append(args,
			fmt.Sprintf("-c:a:%d", aacStereoIndex), "aac",
			fmt.Sprintf("-ac:a:%d", aacStereoIndex), "2",
			fmt.Sprintf("-metadata:s:a:%d", aacStereoIndex), "title=AAC Stereo (MediaForge)",
			fmt.Sprintf("-disposition:a:%d", aacStereoIndex), "default",
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
		args = append(args, fmt.Sprintf("-disposition:a:%d", index), streamDisposition(metadata, false, false))
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

	if !planHasStreamSelection(plan.Override) && !plan.Profile.PreserveSubtitles {
		args = append(args, "-sn")
	} else if plan.Profile.PreserveSubtitles && profileWorkerBool(plan.Profile, "preferSrtSubtitles", false) {
		args = append(args, "-c:s", "srt")
	}
	if plan.Profile.PreserveChapters {
		args = append(args, "-map_chapters", "0")
	} else {
		args = append(args, "-map_chapters", "-1")
	}

	args = append(args, plan.OutputPath)
	return args
}

func buildMediaJobPlan(inputPath string, outputPath string, profile models.Profile, audioProfile *audioEnhancementProfile, overwrite bool) (MediaJobPlan, error) {
	streams, err := probeMediaStreams(inputPath)
	if err != nil {
		return MediaJobPlan{}, err
	}

	return MediaJobPlan{
		InputPath:      inputPath,
		OutputPath:     outputPath,
		Profile:        profile,
		AudioProfile:   audioProfile,
		Overwrite:      overwrite,
		ProcessingMode: mediaProcessingMode(profile, audioProfile),
		Streams:        streams,
	}, nil
}

func buildMediaJobPlanWithOverride(inputPath string, outputPath string, profile models.Profile, audioProfile *audioEnhancementProfile, overwrite bool, override AssetConversionOverrideState) (MediaJobPlan, error) {
	effectiveProfile := applyAssetConversionOverrideToProfile(profile, override)
	plan, err := buildMediaJobPlan(inputPath, outputPath, effectiveProfile, audioProfile, overwrite)
	if err != nil {
		return MediaJobPlan{}, err
	}
	plan.Override = override
	plan.ProcessingMode = mediaProcessingMode(effectiveProfile, audioProfile)
	return plan, nil
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
	selected := selectedAudioStreams(streams, override)
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
			args = append(args, "-q:v", fmt.Sprintf("%d", workerIntValue(profile.WorkerConfig["globalQuality"], profile.QualityValue)))
		case "hevc_amf":
			args = append(args, "-qp_i", fmt.Sprintf("%d", workerIntValue(profile.WorkerConfig["globalQuality"], profile.QualityValue)))
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
	if isHardwareVideoEncoder(encoder) && profileWorkerBool(profile, "useHardwareIfAvailable", true) && !ffmpegEncoderAvailable(encoder) {
		return "libx265"
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
	if filters := workerStringValue(profile.WorkerConfig["videoFilters"]); filters != "" {
		args = append(args, "-vf", filters)
	}
	if preset := workerStringValue(profile.WorkerConfig["videoPreset"]); preset != "" {
		args = append(args, "-preset", preset)
	} else if preset := workerStringValue(profile.WorkerConfig["preset"]); preset != "" && preset != "profile-lab" {
		args = append(args, "-preset", preset)
	}
	if pixFmt := workerStringValue(profile.WorkerConfig["pixFmt"]); pixFmt != "" {
		args = append(args, "-pix_fmt", pixFmt)
	}
	if params := workerStringValue(profile.WorkerConfig["x265Params"]); params != "" && resolvedVideoEncoder(profile) == "libx265" {
		args = append(args, "-x265-params", params)
	}
	return args
}

func ffmpegEncoderAvailable(encoder string) bool {
	encoder = strings.TrimSpace(encoder)
	if encoder == "" {
		return false
	}
	encoderAvailability.Lock()
	defer encoderAvailability.Unlock()
	if !encoderAvailability.checked {
		encoderAvailability.checked = true
		output, err := exec.Command("ffmpeg", "-hide_banner", "-encoders").CombinedOutput()
		if err == nil {
			text := string(output)
			for _, candidate := range []string{"hevc_qsv", "hevc_nvenc", "hevc_videotoolbox", "hevc_amf"} {
				encoderAvailability.values[candidate] = strings.Contains(text, candidate)
			}
		}
	}
	return encoderAvailability.values[encoder]
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
		"-show_entries", "stream=index,codec_type,codec_name,channels,channel_layout:stream_tags=language,title:stream_disposition=default,forced",
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
		Streams []struct {
			Index         int               `json:"index"`
			CodecType     string            `json:"codec_type"`
			CodecName     string            `json:"codec_name"`
			Channels      int               `json:"channels"`
			ChannelLayout string            `json:"channel_layout"`
			Tags          map[string]string `json:"tags"`
			Disposition   struct {
				Default int `json:"default"`
				Forced  int `json:"forced"`
			} `json:"disposition"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		return MediaStreamInventory{}, err
	}

	inventory := MediaStreamInventory{Video: []MediaStream{}, Audio: []MediaAudioStream{}, Subtitle: []MediaStream{}}
	for _, stream := range response.Streams {
		common := MediaStream{
			Index:    stream.Index,
			Codec:    stream.CodecName,
			Language: stream.Tags["language"],
			Title:    stream.Tags["title"],
			Default:  stream.Disposition.Default == 1,
			Forced:   stream.Disposition.Forced == 1,
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
		return "Stereo Enhanced (MediaForge)"
	case "downmix-mono":
		return "Mono Enhanced (MediaForge)"
	default:
		return "Enhanced Audio (MediaForge)"
	}
}

func dryRunCommandFromArgs(args []string) string {
	return "ffmpeg " + shellJoin(args)
}
