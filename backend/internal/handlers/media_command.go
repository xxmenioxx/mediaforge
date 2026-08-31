package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/capabilities"
	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/quality"
)

const (
	ProcessingModeAudioOnly  = "audio_only"
	ProcessingModeFullEncode = "full_encode"
)

type MediaJobPlan struct {
	InputPath                    string
	SourceAssetPath              string
	OutputPath                   string
	Profile                      models.Profile
	AudioProfile                 *audioEnhancementProfile
	Overwrite                    bool
	ProcessingMode               string
	Streams                      MediaStreamInventory
	Override                     AssetConversionOverrideState
	ResolvedTracks               *ResolvedTrackPlan
	StreamValidationWarnings     []string
	Interlace                    InterlaceAnalysis
	Cadence                      CadenceAnalysis
	CadenceRecommendation        CadenceRecommendation
	SegmentStartSeconds          float64
	SegmentDurationSeconds       int
	OutputMetadata               map[string]string
	IndependentSubtitleArtifacts bool
	AllowEmptySubtitleArtifacts  bool
}

type MediaStreamInventory struct {
	Video        []MediaStream
	Audio        []MediaAudioStream
	Subtitle     []MediaStream
	Attachment   []MediaStream
	Data         []MediaStream
	Duration     float64
	ChapterCount int
	TotalBitrate int64
}

type PlannedStream struct {
	InputIndex      int    `json:"inputIndex"`
	InputType       string `json:"inputType"`
	InputTypeIndex  int    `json:"inputTypeIndex"`
	OutputIndex     int    `json:"outputIndex"`
	OutputTypeIndex int    `json:"outputTypeIndex"`
	Action          string `json:"action"`
	Codec           string `json:"codec"`
	Language        string `json:"language,omitempty"`
	Title           string `json:"title,omitempty"`
	Default         bool   `json:"default"`
	Forced          bool   `json:"forced"`
	DuplicateOf     *int   `json:"duplicateOfInput,omitempty"`
	Optional        bool   `json:"optional"`
}

type ResolvedStreamPlan struct {
	Video       []PlannedStream `json:"video"`
	Audio       []PlannedStream `json:"audio"`
	Subtitles   []PlannedStream `json:"subtitles"`
	Attachments []PlannedStream `json:"attachments"`
	Data        []PlannedStream `json:"data"`
}

type MediaStream struct {
	Index              int
	Codec              string
	FieldOrder         string
	PixelFormat        string
	AttachedPic        bool
	StillImage         bool
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
	RealFrameRate      string
	DecodedFrames      int64
	StartTime          float64
	StartTimeValid     bool
	SampleAspectRatio  string
	DisplayAspectRatio string
}

type MediaAudioStream struct {
	Index          int
	Codec          string
	Channels       int
	ChannelLayout  string
	Language       string
	Title          string
	Default        bool
	Forced         bool
	Bitrate        int64
	StartTime      float64
	StartTimeValid bool
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

const testEncodeSeekPrerollSeconds = 5.0

func (FFmpegCommandBuilder) Build(plan MediaJobPlan) []string {
	args := []string{"-hide_banner"}
	if plan.Overwrite {
		args = append(args, "-y")
	} else {
		args = append(args, "-n")
	}

	inputSeek, outputTrim := segmentedSeekWindow(plan.SegmentStartSeconds)
	if inputSeek > 0 {
		args = append(args, "-ss", strconv.FormatFloat(inputSeek, 'f', -1, 64))
	}
	args = append(args, "-i", plan.InputPath)
	if outputTrim > 0 {
		args = append(args, "-ss", strconv.FormatFloat(outputTrim, 'f', -1, 64))
	}
	if plan.SegmentDurationSeconds > 0 {
		args = append(args, "-t", strconv.Itoa(plan.SegmentDurationSeconds))
	}
	selectedAudioStreams := selectedAudioStreamsForPlan(plan)
	addAACStereoTrack := effectiveAACOption(plan, plan.Override.AddAACStereoTrack, "addAacStereoTrack", "addAacStereoDefault", false)
	aacStereoDefault := effectiveAACOption(plan, plan.Override.AACStereoDefault, "aacStereoDefault", "", false)
	needsAACCompatibility := addAACStereoTrack && !hasAACStereoStream(selectedAudioStreams)
	preserveOriginalAudio := profileWorkerBool(plan.Profile, "preserveOriginalAudio", true)
	mappedAudioStreams := selectedAudioStreams
	// Keeping the source audio as secondary means the generated compatibility
	// track is the primary/default track. Older profiles could persist the
	// contradictory combination preserveOriginalAudio=true and
	// aacStereoDefault=false, so resolve the effective disposition here as well
	// as in the UI.
	if needsAACCompatibility && preserveOriginalAudio {
		aacStereoDefault = true
	}
	if needsAACCompatibility && !preserveOriginalAudio {
		mappedAudioStreams = []MediaAudioStream{}
	}
	streamPlan := resolveEffectiveStreamPlan(plan)
	args = appendResolvedStreamMaps(args, streamPlan)

	enhancedAudioIndex := -1
	aacStereoIndex := -1
	if plan.AudioProfile != nil && len(plan.Streams.Audio) > 0 {
		enhancedAudioIndex = len(mappedAudioStreams)
	} else if needsAACCompatibility && len(selectedAudioStreams) > 0 {
		aacStereoIndex = len(mappedAudioStreams)
	}

	if plan.ProcessingMode == ProcessingModeFullEncode {
		effectiveProfile := plan.Profile
		var source *MediaStream
		if len(plan.Streams.Video) > 0 {
			source = &plan.Streams.Video[0]
		}
		args = append(args, videoCodecArgsForSource(effectiveProfile, source)...)
		args = append(args, videoWorkerArgsForSource(effectiveProfile, source)...)
		args = append(args, cadenceOutputArgs(effectiveProfile)...)
		args = append(args, videoColorMetadataArgs(effectiveProfile)...)
	} else {
		args = append(args, "-c:v", "copy")
	}
	if len(streamPlan.Audio) > 0 {
		args = append(args, "-c:a", "copy")
	}
	if len(streamPlan.Subtitles) > 0 {
		args = append(args, "-c:s", "copy")
	}
	if len(streamPlan.Attachments) > 0 {
		args = append(args, "-c:t", "copy")
	}
	if len(streamPlan.Data) > 0 {
		args = append(args, "-c:d", "copy")
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
	args = appendClearInheritedStreamStatistics(args, streamPlan)
	if len(plan.OutputMetadata) > 0 {
		keys := make([]string, 0, len(plan.OutputMetadata))
		for key := range plan.OutputMetadata {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			args = append(args, "-metadata", key+"="+plan.OutputMetadata[key])
		}
	}

	for index, stream := range selectedVideoStreamsForPlan(plan) {
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
	if sourceHasSubtitles && !planHasStreamSelection(plan.Override) && len(selectedSubtitleStreams(plan)) == 0 {
		args = append(args, "-sn")
	} else if hasSelectedSubtitles {
		subtitleCodec := effectiveSubtitleOutputFormat(plan.Profile)
		for index, stream := range selectedSubtitleStreams(plan) {
			if subtitleCodec != "source" && subtitleCanConvertText(stream.Codec) {
				args = append(args, fmt.Sprintf("-c:s:%d", index), subtitleCodec)
			}
		}
	}
	keepChapters := plan.Profile.PreserveChapters
	if plan.ResolvedTracks != nil {
		keepChapters = plan.ResolvedTracks.ChaptersKept
	}
	if plan.Streams.ChapterCount > 0 && keepChapters {
		args = append(args, "-map_chapters", "0")
	} else if plan.Streams.ChapterCount > 0 {
		args = append(args, "-map_chapters", "-1")
	}

	args = append(args, plan.OutputPath)
	return args
}

func segmentedSeekWindow(start float64) (float64, float64) {
	if start <= 0 {
		return 0, 0
	}
	inputSeek := math.Max(0, start-testEncodeSeekPrerollSeconds)
	return inputSeek, start - inputSeek
}

func resolveEffectiveStreamPlan(plan MediaJobPlan) ResolvedStreamPlan {
	selectedAudio := selectedAudioStreamsForPlan(plan)
	needsAAC := effectiveAACOption(plan, plan.Override.AddAACStereoTrack, "addAacStereoTrack", "addAacStereoDefault", false) && !hasAACStereoStream(selectedAudio)
	mappedAudio := selectedAudio
	if needsAAC && !profileWorkerBool(plan.Profile, "preserveOriginalAudio", true) {
		mappedAudio = []MediaAudioStream{}
	}
	return resolveStreamPlan(plan, mappedAudio, needsAAC)
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
	hasMotionFilter := strings.Contains(existing, "bwdif=") || strings.Contains(existing, "yadif=") || strings.Contains(existing, "fieldmatch") || strings.Contains(existing, "decimate")
	if existing != "" && !hasMotionFilter {
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
	cadence := CadenceAnalysis{Version: cadenceAnalysisVersion, Type: "unknown"}
	cadenceRecommendation := CadenceRecommendation{Version: 1, Operation: "review"}
	if processingMode == ProcessingModeFullEncode {
		if frozen, ok := decodeInterlaceAnalysis(profile.WorkerConfig[interlaceAnalysisSnapshotKey]); ok {
			analysis = frozen
			analysis.Source = "queue_scan_snapshot"
		} else {
			analysis = detectInterlace(inputPath, fieldOrder, streams.Duration, 20)
			if len(streams.Video) > 0 {
				analysis.Codec = streams.Video[0].Codec
				analysis.AverageFrameRate = streams.Video[0].FrameRate
			}
		}
		if frozen, ok := decodeCadenceAnalysis(profile.WorkerConfig[cadenceAnalysisSnapshotKey]); ok {
			cadence = frozen
		} else if len(streams.Video) > 0 {
			cadence = analyzeCadence(streams.Video[0].Codec, streams.Video[0].FrameRate, analysis)
		}
		if frozen, ok := decodeCadenceRecommendation(profile.WorkerConfig[cadenceRecommendationSnapshotKey]); ok {
			cadenceRecommendation = frozen
		} else {
			cadenceRecommendation = recommendCadence(cadence)
		}
	}
	qualityAnalysisPath := strings.TrimSpace(workerStringValue(profile.WorkerConfig["qsvAssetAnalysisPath"]))
	if qualityAnalysisPath == "" {
		qualityAnalysisPath = strings.TrimSpace(workerStringValue(profile.WorkerConfig["derivedFromAsset"]))
	}
	if qualityAnalysisPath == "" {
		qualityAnalysisPath = inputPath
	}
	if processingMode == ProcessingModeFullEncode {
		profile = resolveMediaJobVideoProfile(profile, streams, qualityAnalysisPath, analysis, cadence, cadenceRecommendation)
	} else {
		profile = resolveEffectiveVideoEncodingProfile(profile, streams, qualityAnalysisPath)
	}
	return MediaJobPlan{
		InputPath:             inputPath,
		SourceAssetPath:       inputPath,
		OutputPath:            outputPath,
		Profile:               profile,
		AudioProfile:          audioProfile,
		Overwrite:             overwrite,
		ProcessingMode:        processingMode,
		Streams:               streams,
		Interlace:             analysis,
		Cadence:               cadence,
		CadenceRecommendation: cadenceRecommendation,
	}, nil
}

func resolveMediaJobVideoProfile(profile models.Profile, streams MediaStreamInventory, analysisPath string, interlace InterlaceAnalysis, cadence CadenceAnalysis, recommendation CadenceRecommendation) models.Profile {
	profile = resolveEffectiveVideoMotionProfile(profile, interlace, cadence, recommendation)
	return resolveEffectiveVideoEncodingProfile(profile, streams, analysisPath)
}

func resolveEffectiveVideoMotionProfile(profile models.Profile, interlace InterlaceAnalysis, cadence CadenceAnalysis, recommendation CadenceRecommendation) models.Profile {
	profile = profileWithResolvedFieldAndCadenceModes(profile, interlace)
	profile = profileWithAutomaticDeinterlace(profile, interlace)
	profile = profileWithCadenceOutputDecision(profile, cadence, recommendation)
	return profileWithEffectiveProgressiveOutput(profile, interlace, cadence, recommendation)
}

func resolveEffectiveVideoEncodingProfile(profile models.Profile, streams MediaStreamInventory, analysisPath string, upscaleEvidence ...UpscaleAnalysisEvidence) models.Profile {
	intent := qualityIntentForMedia(profile, analysisPath, streams)
	profile = applyVideoToolboxQualityRecommendation(profile, intent)
	profile = applyQSVQualityRecommendation(profile, intent, capabilities.CheckEncoder("hevc_qsv"))
	evidence := UpscaleAnalysisEvidence{}
	if len(upscaleEvidence) > 0 {
		evidence = upscaleEvidence[0]
	}
	profile = resolveUpscaleProfile(profile, streams, evidence)
	if len(streams.Video) > 0 {
		profile = profileWithFinalColorPolicy(profile, streams.Video[0], resolvedVideoEncoder(profile))
	}
	return resolveHEVCLevel(profile, streams)
}

func profileWithEffectiveProgressiveOutput(profile models.Profile, interlace InterlaceAnalysis, cadence CadenceAnalysis, recommendation CadenceRecommendation) models.Profile {
	profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
	filters := strings.ToLower(workerStringValue(profile.WorkerConfig["videoFilters"]))
	operation := strings.ToLower(workerStringValue(profile.WorkerConfig["effectiveCadenceOperation"]))
	progressive := strings.Contains(filters, "bwdif=") || strings.Contains(filters, "yadif=") ||
		strings.Contains(filters, "fieldmatch") || operation == "inverse_telecine" || operation == "remove_soft_telecine"
	reliable := progressive
	if !progressive && strings.EqualFold(strings.TrimSpace(interlace.Status), "progressive") {
		progressive = true
		reliable = interlace.Confidence >= .8
	}
	if (operation == "inverse_telecine" || operation == "remove_soft_telecine") && recommendation.Confidence > 0 {
		reliable = recommendation.Confidence >= .8 && cadence.Confidence >= .8
	}
	profile.WorkerConfig["effectiveOutputProgressive"] = progressive
	if reliable {
		delete(profile.WorkerConfig, "effectiveOutputProgressiveUnreliable")
	} else {
		profile.WorkerConfig["effectiveOutputProgressiveUnreliable"] = true
	}
	return profile
}

func effectiveVideoDecision(profile models.Profile) models.JSONMap {
	fps := workerStringValue(profile.WorkerConfig["effectiveOutputFrameRate"])
	gopFrames := workerIntValue(profile.WorkerConfig["frameStructureGopFrames"], 0)
	gopSeconds := 0.0
	if parsedFPS := parseFrameRateValue(fps); parsedFPS > 0 && gopFrames > 0 {
		gopSeconds = float64(gopFrames) / parsedFPS
	}
	decision := models.JSONMap{
		"encoder":                   resolvedVideoEncoder(profile),
		"codec":                     profile.VideoCodec,
		"profile":                   map[bool]string{true: "main10", false: "main"}[profileUsesTenBit(profile)],
		"pixelFormat":               workerStringValue(profile.WorkerConfig["pixFmt"]),
		"frameStructureMode":        workerStringValue(profile.WorkerConfig["frameStructureMode"]),
		"cadenceOperation":          workerStringValue(profile.WorkerConfig["effectiveCadenceOperation"]),
		"effectiveFrameRate":        fps,
		"effectiveWidth":            workerIntValue(profile.WorkerConfig["effectiveOutputWidth"], 0),
		"effectiveHeight":           workerIntValue(profile.WorkerConfig["effectiveOutputHeight"], 0),
		"geometryUnknown":           profileWorkerBool(profile, "effectiveOutputGeometryUnknown", false),
		"gopFrames":                 gopFrames,
		"gopSeconds":                gopSeconds,
		"maxBFrames":                workerIntValue(profile.WorkerConfig["frameStructureMaxBFrames"], 0),
		"hevcLevel":                 workerStringValue(profile.WorkerConfig["hevcLevelEffective"]),
		"hevcTier":                  workerStringValue(profile.WorkerConfig["hevcLevelTier"]),
		"hevcLevelWarning":          workerStringValue(profile.WorkerConfig["hevcLevelResolutionWarning"]),
		"effectiveFinalColorPolicy": workerStringValue(profile.WorkerConfig["effectiveFinalColorPolicy"]),
	}
	if upscale, ok := resolvedUpscaleDecisionFromProfile(profile); ok {
		decision["upscale"] = upscale
	}
	return decision
}

func buildMediaJobPlanWithOverride(inputPath string, outputPath string, profile models.Profile, audioProfile *audioEnhancementProfile, overwrite bool, override AssetConversionOverrideState) (MediaJobPlan, error) {
	effectiveProfile := applyAssetConversionOverrideToProfile(profile, override)
	plan, err := buildMediaJobPlan(inputPath, outputPath, effectiveProfile, audioProfile, overwrite)
	if err != nil {
		return MediaJobPlan{}, err
	}
	plan.Override, plan.StreamValidationWarnings = sanitizeConversionOverride(override, plan.Streams)
	plan.ResolvedTracks = plan.Override.ResolvedTrackPlan
	plan.StreamValidationWarnings = append(plan.StreamValidationWarnings, validateAACStereoSource(&plan)...)
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

func effectiveSubtitlePolicy(profile models.Profile) string {
	configured := strings.ToLower(strings.TrimSpace(workerStringValue(profile.WorkerConfig["externalSubtitleFormat"])))
	switch configured {
	case "disabled", "defer":
		return "disabled"
	case "source":
		return "source"
	case "srt", "ass", "ssa":
		return "externalize"
	case "remove":
		return "remove"
	default:
		if profile.PreserveSubtitles {
			return "source"
		}
		return "remove"
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
	if value := normalizedOptimizationIntent(override.OptimizationIntent); value != "" {
		profile.OptimizationIntent = value
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
	if value := strings.ToLower(strings.TrimSpace(override.ExternalSubtitleFormat)); value == "disabled" || value == "source" || value == "srt" || value == "ass" || value == "remove" {
		workerConfig["externalSubtitleFormat"] = value
		workerConfig["subtitleOutputFormat"] = "source"
		profile.PreserveSubtitles = value == "disabled" || value == "source"
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
	if value := normalizedUpscaleMode(override.UpscaleMode); value != "" {
		workerConfig["upscaleMode"] = value
		if value == "custom" && override.UpscaleCustomHeight >= 2 && override.UpscaleCustomHeight%2 == 0 {
			workerConfig["upscaleCustomHeight"] = override.UpscaleCustomHeight
		} else {
			delete(workerConfig, "upscaleCustomHeight")
		}
	}
	if value := normalizedUpscaleSharpen(override.UpscaleSharpen); value != "" {
		workerConfig["upscaleSharpen"] = value
	}
	if value := strings.ToLower(strings.TrimSpace(override.CropAspectPolicy)); value == "source_sar" || value == "preserve_dar" {
		workerConfig["cropAspectPolicy"] = value
	}
	if value := strings.TrimSpace(override.X265Params); value != "" {
		workerConfig["x265Params"] = value
	}
	if value := normalizedFrameStructureMode(override.FrameStructureMode); value != "" {
		workerConfig["frameStructureMode"] = value
	}
	if value := normalizedFrameStructureGOPMode(override.FrameStructureGOPMode); value != "" {
		workerConfig["frameStructureGopMode"] = value
		if value == "recommended" || value == "custom" {
			frames := override.FrameStructureGOPFrames
			if frames <= 0 {
				frames = 120
			}
			workerConfig["frameStructureGopFrames"] = min(1000, frames)
		}
	}
	if value := normalizedFrameStructureBFrameMode(override.FrameStructureBFrameMode); value != "" {
		workerConfig["frameStructureBFrameMode"] = value
		switch value {
		case "recommended", "custom":
			frames := override.FrameStructureMaxBFrames
			if frames <= 0 {
				frames = 3
			}
			workerConfig["frameStructureMaxBFrames"] = min(16, frames)
		case "off":
			workerConfig["frameStructureMaxBFrames"] = 0
		}
	}
	if value := normalizedHEVCLevelMode(override.HEVCLevelMode); value != "" {
		workerConfig["hevcLevelMode"] = value
	}
	if value := normalizedHEVCLevel(override.HEVCLevel); value != "" {
		workerConfig["hevcLevel"] = value
	}
	if value := strings.TrimSpace(override.DeinterlaceMode); value != "" {
		workerConfig["deinterlaceMode"] = value
	}
	if value := normalizedFieldStructureMode(override.FieldStructureMode); value != "" {
		workerConfig["fieldStructureMode"] = value
	}
	if value := normalizedCadenceMode(override.CadenceMode); value != "" {
		workerConfig["cadenceMode"] = value
	}
	if value := normalizedCadenceFieldOrder(override.CadenceFieldOrder); value != "" {
		workerConfig["cadenceFieldOrder"] = value
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
	if value := strings.TrimSpace(override.TargetWorkerName); value != "" {
		workerConfig["targetWorkerName"] = value
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
	if override.QSVPStrategy != nil {
		workerConfig["qsvPStrategy"] = min(2, max(0, *override.QSVPStrategy))
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
	if policy := strings.ToLower(strings.TrimSpace(override.VideoToolboxBFramePolicy)); policy == "auto" || policy == "enabled" || policy == "disabled" {
		workerConfig["videoToolboxBFramePolicy"] = policy
	}
	if override.VideoToolboxBFrames > 0 {
		workerConfig["videoToolboxBFrames"] = clampVideoToolboxBFrames(override.VideoToolboxBFrames)
	}
	if override.VideoToolboxAutoAdjustBitrate != nil {
		workerConfig["videoToolboxAutoAdjustBitrate"] = *override.VideoToolboxAutoAdjustBitrate
	}
	if override.VideoToolboxAllowFrameReordering != nil {
		if *override.VideoToolboxAllowFrameReordering {
			workerConfig["videoToolboxBFramePolicy"] = "enabled"
		}
		delete(workerConfig, "videoToolboxAllowFrameReordering")
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

func resolveStreamPlan(plan MediaJobPlan, mappedAudio []MediaAudioStream, needsAACCompatibility bool) ResolvedStreamPlan {
	resolved := ResolvedStreamPlan{}
	outputIndex := 0
	aacStereoDefault := effectiveAACOption(plan, plan.Override.AACStereoDefault, "aacStereoDefault", "", false)
	preserveOriginalAudio := profileWorkerBool(plan.Profile, "preserveOriginalAudio", true)
	if needsAACCompatibility && preserveOriginalAudio {
		aacStereoDefault = true
	}
	derivedAudioDefault := plan.AudioProfile != nil || (needsAACCompatibility && aacStereoDefault)
	for typeIndex, stream := range selectedVideoStreamsForPlan(plan) {
		action, codec := "copy", "copy"
		if plan.ProcessingMode == ProcessingModeFullEncode {
			action, codec = "encode", resolvedVideoEncoder(plan.Profile)
		}
		metadata := metadataOverrideFor(plan.Override.VideoMetadata, stream.Index)
		resolved.Video = append(resolved.Video, plannedMediaStream(stream, "video", typeIndex, outputIndex, action, codec, metadata))
		outputIndex++
	}
	for typeIndex, stream := range mappedAudio {
		metadata := metadataOverrideFor(plan.Override.AudioMetadata, stream.Index)
		title := metadata.Title
		if title == "" {
			title = originalAudioTitle(stream)
		}
		language := metadata.Language
		if language == "" {
			language = stream.Language
		}
		isDefault, isForced := stream.Default && !derivedAudioDefault, stream.Forced
		if metadata.Default != nil {
			isDefault = *metadata.Default
		}
		if metadata.Forced != nil {
			isForced = *metadata.Forced
		}
		resolved.Audio = append(resolved.Audio, PlannedStream{
			InputIndex: stream.Index, InputType: "audio", InputTypeIndex: audioTypeIndex(plan.Streams.Audio, stream.Index),
			OutputIndex: outputIndex, OutputTypeIndex: typeIndex, Action: "copy", Codec: "copy",
			Language: language, Title: title, Default: isDefault, Forced: isForced,
		})
		outputIndex++
	}
	for typeIndex, stream := range selectedSubtitleStreams(plan) {
		metadata := metadataOverrideFor(plan.Override.SubtitleMetadata, stream.Index)
		codec := "copy"
		if output := effectiveSubtitleOutputFormat(plan.Profile); output != "source" && subtitleCanConvertText(stream.Codec) {
			codec = output
		}
		resolved.Subtitles = append(resolved.Subtitles, plannedMediaStream(stream, "subtitle", typeIndex, outputIndex, map[bool]string{true: "copy", false: "encode"}[codec == "copy"], codec, metadata))
		outputIndex++
	}
	for typeIndex, stream := range selectedAttachmentStreamsForPlan(plan) {
		resolved.Attachments = append(resolved.Attachments, plannedMediaStream(stream, "attachment", typeIndex, outputIndex, "copy", "copy", StreamMetadataOverride{}))
		outputIndex++
	}
	for typeIndex, stream := range plan.Streams.Data {
		resolved.Data = append(resolved.Data, plannedMediaStream(stream, "data", typeIndex, outputIndex, "copy", "copy", StreamMetadataOverride{}))
		outputIndex++
	}
	if (plan.AudioProfile != nil || needsAACCompatibility) && len(plan.Streams.Audio) > 0 {
		sourceIndex := enhancedAudioSourceIndex(plan.Streams.Audio, plan.Override)
		codec := "aac"
		title := "AAC Stereo (MVForge)"
		if plan.AudioProfile != nil {
			codec = ffmpegCodecName(plan.AudioProfile.OutputCodec)
			if codec == "" || codec == "copy" {
				codec = "aac"
			}
			title = enhancedAudioTitle(*plan.AudioProfile)
		} else {
			sourceIndex = aacStereoSourceIndex(plan)
		}
		duplicate := sourceIndex
		resolved.Audio = append(resolved.Audio, PlannedStream{
			InputIndex: sourceIndex, InputType: "audio", InputTypeIndex: audioTypeIndex(plan.Streams.Audio, sourceIndex),
			OutputIndex: outputIndex, OutputTypeIndex: len(mappedAudio), Action: "derive", Codec: codec,
			Title: title, Default: derivedAudioDefault, DuplicateOf: &duplicate,
		})
	}
	return resolved
}

func plannedMediaStream(stream MediaStream, streamType string, typeIndex, outputIndex int, action, codec string, metadata StreamMetadataOverride) PlannedStream {
	language, title := stream.Language, stream.Title
	isDefault, isForced := stream.Default, stream.Forced
	if metadata.Language != "" {
		language = metadata.Language
	}
	if metadata.Title != "" {
		title = metadata.Title
	}
	if metadata.Default != nil {
		isDefault = *metadata.Default
	}
	if metadata.Forced != nil {
		isForced = *metadata.Forced
	}
	return PlannedStream{InputIndex: stream.Index, InputType: streamType, InputTypeIndex: typeIndex, OutputIndex: outputIndex, OutputTypeIndex: typeIndex, Action: action, Codec: codec, Language: language, Title: title, Default: isDefault, Forced: isForced}
}

func audioTypeIndex(streams []MediaAudioStream, inputIndex int) int {
	for index, stream := range streams {
		if stream.Index == inputIndex {
			return index
		}
	}
	return -1
}

func appendResolvedStreamMaps(args []string, plan ResolvedStreamPlan) []string {
	streams := append([]PlannedStream{}, plan.Video...)
	streams = append(streams, plan.Audio...)
	streams = append(streams, plan.Subtitles...)
	streams = append(streams, plan.Attachments...)
	streams = append(streams, plan.Data...)
	sort.SliceStable(streams, func(left, right int) bool { return streams[left].OutputIndex < streams[right].OutputIndex })
	for _, stream := range streams {
		args = append(args, "-map", fmt.Sprintf("0:%d", stream.InputIndex))
	}
	return args
}

var inheritedStreamStatisticTags = []string{
	"BPS", "BPS-eng",
	"DURATION", "DURATION-eng",
	"NUMBER_OF_FRAMES", "NUMBER_OF_FRAMES-eng",
	"NUMBER_OF_BYTES", "NUMBER_OF_BYTES-eng",
	"_STATISTICS_WRITING_APP", "_STATISTICS_WRITING_APP-eng",
	"_STATISTICS_WRITING_DATE_UTC", "_STATISTICS_WRITING_DATE_UTC-eng",
	"_STATISTICS_TAGS", "_STATISTICS_TAGS-eng",
}

// Matroska stream statistics describe the source bitstream. Copying them onto
// a re-encoded or remuxed output makes snapshots report source bytes/bitrate as
// if they belonged to the output. Clear them on every mapped output stream;
// ffprobe must describe the newly written container instead.
func appendClearInheritedStreamStatistics(args []string, plan ResolvedStreamPlan) []string {
	groups := []struct {
		typeCode string
		streams  []PlannedStream
	}{
		{typeCode: "v", streams: plan.Video},
		{typeCode: "a", streams: plan.Audio},
		{typeCode: "s", streams: plan.Subtitles},
		{typeCode: "t", streams: plan.Attachments},
		{typeCode: "d", streams: plan.Data},
	}
	for _, group := range groups {
		for _, stream := range group.streams {
			for _, tag := range inheritedStreamStatisticTags {
				args = append(args, fmt.Sprintf("-metadata:s:%s:%d", group.typeCode, stream.OutputTypeIndex), tag+"=")
			}
		}
	}
	return args
}

func appendSelectedStreamMaps(args []string, plan MediaJobPlan) []string {
	for _, index := range selectedStreamIndexes(streamIndexes(plan.Streams.Video), plan.Override.KeepVideoStreams) {
		args = append(args, "-map", fmt.Sprintf("0:%d", index))
	}
	for _, index := range selectedStreamIndexes(audioStreamIndexes(plan.Streams.Audio), plan.Override.KeepAudioStreams) {
		args = append(args, "-map", fmt.Sprintf("0:%d", index))
	}
	for _, stream := range selectedSubtitleStreams(plan) {
		args = append(args, "-map", fmt.Sprintf("0:%d", stream.Index))
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

func selectedAudioStreamsForPlan(plan MediaJobPlan) []MediaAudioStream {
	if plan.ResolvedTracks == nil {
		return selectedAudioStreams(plan.Streams.Audio, plan.Override)
	}
	allowed := map[int]struct{}{}
	for _, stream := range plan.ResolvedTracks.AudioStreams {
		allowed[stream.StreamIndex] = struct{}{}
	}
	selected := []MediaAudioStream{}
	for _, stream := range plan.Streams.Audio {
		if _, ok := allowed[stream.Index]; ok {
			selected = append(selected, stream)
		}
	}
	return selected
}

func selectedSubtitleStreams(plan MediaJobPlan) []MediaStream {
	if plan.ResolvedTracks != nil {
		allowed := map[int]struct{}{}
		for _, stream := range plan.ResolvedTracks.SubtitleStreams {
			if stream.Action.KeepsEmbedded() {
				allowed[stream.StreamIndex] = struct{}{}
			}
		}
		selected := []MediaStream{}
		for _, stream := range plan.Streams.Subtitle {
			if _, ok := allowed[stream.Index]; ok {
				selected = append(selected, stream)
			}
		}
		return selected
	}
	// A Tracks Profile owns the complete subtitle decision. Video-profile
	// preservation flags must not erase tracks that the Tracks Profile kept.
	trackProfileSelected := strings.TrimSpace(plan.Override.TrackProfileKey) != ""
	if !trackProfileSelected && effectiveSubtitlePolicy(plan.Profile) == "remove" {
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

func selectedAttachmentStreamsForPlan(plan MediaJobPlan) []MediaStream {
	if plan.ResolvedTracks == nil {
		return plan.Streams.Attachment
	}
	if !plan.ResolvedTracks.AttachmentsKept {
		return []MediaStream{}
	}
	if len(plan.ResolvedTracks.AttachmentStreams) == 0 {
		return plan.Streams.Attachment
	}
	allowed := map[int]struct{}{}
	for _, stream := range plan.ResolvedTracks.AttachmentStreams {
		allowed[stream.StreamIndex] = struct{}{}
	}
	selected := []MediaStream{}
	for _, stream := range plan.Streams.Attachment {
		if _, ok := allowed[stream.Index]; ok {
			selected = append(selected, stream)
		}
	}
	return selected
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

func aacStereoSourceIndex(plan MediaJobPlan) int {
	if plan.Override.EnhancedAudioSourceStreamIndex != nil {
		return enhancedAudioSourceIndex(plan.Streams.Audio, plan.Override)
	}
	configured := workerIntValue(plan.Profile.WorkerConfig["aacStereoSourceStreamIndex"], -1)
	if configured >= 0 {
		for _, stream := range selectedAudioStreamsForPlan(plan) {
			if stream.Index == configured {
				return configured
			}
		}
	}
	return enhancedAudioSourceIndex(plan.Streams.Audio, plan.Override)
}

func validateAACStereoSource(plan *MediaJobPlan) []string {
	if plan == nil || plan.Override.EnhancedAudioSourceStreamIndex != nil {
		return nil
	}
	configured := workerIntValue(plan.Profile.WorkerConfig["aacStereoSourceStreamIndex"], -1)
	if configured < 0 {
		return nil
	}
	for _, stream := range selectedAudioStreamsForPlan(*plan) {
		if stream.Index == configured {
			return nil
		}
	}
	return []string{fmt.Sprintf("AAC compatibility source stream %d is not available in this asset; the selected default audio stream will be used.", configured)}
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
func adaptiveVideoToolboxBitrate(profile models.Profile, source *MediaStream, encoders ...string) (int, int, int, bool) {
	if source == nil || source.Bitrate <= 0 || strings.ToLower(workerStringValue(profile.WorkerConfig["hardwareQualityPreset"])) == "custom" {
		return 0, 0, 0, false
	}
	intent := qualityIntentForMedia(profile, workerStringValue(profile.WorkerConfig["qsvAssetAnalysisPath"]), MediaStreamInventory{Video: []MediaStream{*source}})
	encoder := "hevc_videotoolbox"
	if len(encoders) > 0 && strings.HasSuffix(encoders[0], "_videotoolbox") {
		encoder = encoders[0]
	}
	capability := capabilities.CheckEncoder(encoder)
	recommendation, err := (quality.VideoToolboxTranslator{}).Translate(intent, videoToolboxQualityCapabilities(capability, profileUsesTenBit(profile)))
	if err != nil || recommendation.TargetBitrate == nil || recommendation.Maxrate == nil || recommendation.Buffer == nil {
		return 0, 0, 0, false
	}
	return int(*recommendation.TargetBitrate / 1000), int(*recommendation.Maxrate / 1000), int(*recommendation.Buffer / 1000), true
}

func videoCodecArgsForSource(profile models.Profile, source *MediaStream) []string {
	return videoCodecArgsForResolvedEncoder(profile, source, resolvedVideoEncoder(profile))
}

func videoCodecArgsForResolvedEncoder(profile models.Profile, source *MediaStream, encoder string) []string {
	if profile.VideoCodec == "" || profile.VideoCodec == "copy" {
		return []string{"-c:v", "copy"}
	}
	args := []string{"-c:v", encoder}
	if profile.QualityMode == "crf" && profile.QualityValue > 0 {
		switch encoder {
		case "libx264", "libx265":
			args = append(args, "-crf", fmt.Sprintf("%d", profile.QualityValue))
		case "hevc_qsv":
			args = append(args, qsvRateControlArgs(profile)...)
		case "hevc_vaapi":
			args = append(args, "-qp", fmt.Sprintf("%d", workerIntValue(profile.WorkerConfig["globalQuality"], defaultQSVQuality(profile.QualityValue))))
		case "hevc_nvenc":
			args = append(args, "-cq", fmt.Sprintf("%d", workerIntValue(profile.WorkerConfig["globalQuality"], profile.QualityValue)))
		case "hevc_videotoolbox", "h264_videotoolbox":
			if targetKbps, maxrateKbps, bufferKbps, ok := adaptiveVideoToolboxBitrate(profile, source, encoder); ok {
				args = append(args, "-b:v", fmt.Sprintf("%dk", targetKbps), "-maxrate", fmt.Sprintf("%dk", maxrateKbps), "-bufsize", fmt.Sprintf("%dk", bufferKbps))
			} else {
				bitrate, maxrate, buffer := explicitVideoToolboxRates(profile, capabilities.CheckEncoder(encoder))
				args = append(args, "-b:v", formatMbps(bitrate), "-maxrate", formatMbps(maxrate), "-bufsize", formatMbps(buffer))
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
	if encoder == "hevc_videotoolbox" || encoder == "h264_videotoolbox" {
		capability := capabilities.CheckEncoder(encoder)
		videoToolboxProfile := normalizedVideoToolboxProfile(profile)
		if encoder == "h264_videotoolbox" {
			videoToolboxProfile = "high"
		}
		if normalizedFrameStructureGOPMode(workerStringValue(profile.WorkerConfig["frameStructureGopMode"])) == "" {
			if gop := workerIntValue(profile.WorkerConfig["videoToolboxGop"], 0); gop > 0 {
				args = append(args, "-g", fmt.Sprintf("%d", gop))
			}
		}
		main10 := encoder == "hevc_videotoolbox" && videoToolboxProfile == "main10"
		powerSupported := videoToolboxOptionalFeatureSupported(capability, "power_efficiency", main10)
		if normalizedFrameStructureMode(workerStringValue(profile.WorkerConfig["frameStructureMode"])) == "off" {
			args = append(args, "-realtime", map[bool]string{true: "1", false: "0"}[profileWorkerBool(profile, "videoToolboxRealtime", false)])
		} else if normalizedFrameStructureBFrameMode(workerStringValue(profile.WorkerConfig["frameStructureBFrameMode"])) == "" {
			args = append(args, videoToolboxRealtimeAndBFrameArgs(profile, capability, main10)...)
		} else {
			args = append(args, "-realtime", map[bool]string{true: "1", false: "0"}[profileWorkerBool(profile, "videoToolboxRealtime", false)])
		}
		if value, ok := profile.WorkerConfig["videoToolboxPowerEfficiency"].(bool); ok && powerSupported {
			args = append(args, "-power_efficient", map[bool]string{true: "1", false: "0"}[value])
		}
		if main10 && capability.VideoToolboxMain10 {
			args = append(args, "-profile:v", "main10", "-pix_fmt", "p010le")
		} else if encoder == "h264_videotoolbox" {
			args = append(args, "-profile:v", "high", "-pix_fmt", "yuv420p")
		} else {
			args = append(args, "-profile:v", "main", "-pix_fmt", "yuv420p")
		}
	}
	if isTenBitVideoCodec(profile.VideoCodec) && encoder == "libx265" {
		args = append(args, "-pix_fmt", "yuv420p10le")
	}
	args = append(args, hevcLevelArgs(profile, encoder)...)
	args = append(args, commonFrameStructureArgs(profile, encoder)...)
	return args
}

func normalizedFrameStructureGOPMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "auto", "recommended", "custom":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizedFrameStructureMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "auto", "off", "compatible", "balanced", "maximum_compression", "custom":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizedFrameStructureBFrameMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "auto", "recommended", "custom", "off":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func effectiveX265Params(profile models.Profile) string {
	raw := strings.TrimSpace(workerStringValue(profile.WorkerConfig["x265Params"]))
	if raw == "" {
		return x265ParamsWithHEVCLevel(profile, "")
	}
	gopMode := normalizedFrameStructureGOPMode(workerStringValue(profile.WorkerConfig["frameStructureGopMode"]))
	bFrameMode := normalizedFrameStructureBFrameMode(workerStringValue(profile.WorkerConfig["frameStructureBFrameMode"]))
	parts := strings.Split(raw, ":")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(strings.SplitN(part, "=", 2)[0]))
		if normalizedFrameStructureMode(workerStringValue(profile.WorkerConfig["frameStructureMode"])) == "off" && (name == "keyint" || name == "min-keyint" || name == "scenecut" || name == "bframes" || name == "b-adapt" || name == "b-pyramid") {
			continue
		}
		if (gopMode == "recommended" || gopMode == "custom") && (name == "keyint" || name == "min-keyint") {
			continue
		}
		if gopMode != "custom" && name == "scenecut" {
			continue
		}
		if bFrameMode != "" && bFrameMode != "auto" && name == "bframes" {
			continue
		}
		if bFrameMode != "custom" && (name == "b-adapt" || name == "b-pyramid") {
			continue
		}
		result = append(result, part)
	}
	return x265ParamsWithHEVCLevel(profile, strings.Join(result, ":"))
}

func commonFrameStructureArgs(profile models.Profile, encoder string) []string {
	args := []string{}
	if normalizedFrameStructureMode(workerStringValue(profile.WorkerConfig["frameStructureMode"])) == "off" {
		return args
	}
	if mode := normalizedFrameStructureGOPMode(workerStringValue(profile.WorkerConfig["frameStructureGopMode"])); mode == "recommended" || mode == "custom" {
		value := workerIntValue(profile.WorkerConfig["frameStructureGopFrames"], 120)
		args = append(args, "-g", strconv.Itoa(min(1000, max(1, value))))
	}
	mode := normalizedFrameStructureBFrameMode(workerStringValue(profile.WorkerConfig["frameStructureBFrameMode"]))
	if mode == "auto" || mode == "" {
		return args
	}
	value := 0
	if mode == "recommended" || mode == "custom" {
		value = min(16, max(1, workerIntValue(profile.WorkerConfig["frameStructureMaxBFrames"], 3)))
	}
	if encoder == "hevc_videotoolbox" || encoder == "h264_videotoolbox" {
		capability := capabilities.CheckEncoder(encoder)
		main10 := encoder == "hevc_videotoolbox" && normalizedVideoToolboxProfile(profile) == "main10"
		if value == 0 && !videoToolboxBFramesDisabledSupported(capability, main10) {
			return args
		}
		if value > 0 && !videoToolboxOptionalFeatureSupported(capability, "bframes", main10) {
			return args
		}
		value = min(4, value)
	}
	return append(args, "-bf", strconv.Itoa(value))
}

func explicitVideoToolboxRates(profile models.Profile, capability capabilities.EncoderCapability) (float64, float64, float64) {
	bitrate := workerNumberValue(profile.WorkerConfig["videoToolboxBitrateMbps"], defaultVideoToolboxBitrateMbps(profile.QualityValue))
	maxrate := workerNumberValue(profile.WorkerConfig["videoToolboxMaxrateMbps"], bitrate*1.5)
	buffer := workerNumberValue(profile.WorkerConfig["videoToolboxBufferMbps"], bitrate*2.5)
	if strings.EqualFold(workerStringValue(profile.WorkerConfig["hardwareQualityPreset"]), "custom") && profileWorkerBool(profile, "videoToolboxAutoAdjustBitrate", false) {
		bitrate *= videoToolboxStrategyMultiplier(profile, capability)
		maxrate = bitrate * 1.5
		buffer = bitrate * 2.5
	}
	return bitrate, maxrate, buffer
}

func requestedVideoToolboxBFramePolicy(profile models.Profile) string {
	if mode := normalizedFrameStructureBFrameMode(workerStringValue(profile.WorkerConfig["frameStructureBFrameMode"])); mode != "" {
		switch mode {
		case "recommended", "custom":
			return "enabled"
		case "off":
			return "disabled"
		default:
			return "auto"
		}
	}
	switch strings.ToLower(strings.TrimSpace(workerStringValue(profile.WorkerConfig["videoToolboxBFramePolicy"]))) {
	case "enabled":
		return "enabled"
	case "disabled":
		return "disabled"
	case "auto":
		return "auto"
	}
	// Legacy true meant an explicit positive -bf value. Legacy false was also
	// the zero value used for missing settings, so it must migrate to Auto.
	if legacy, ok := profile.WorkerConfig["videoToolboxAllowFrameReordering"].(bool); ok && legacy {
		return "enabled"
	}
	return "auto"
}

func videoToolboxRealtimeAndBFrameArgs(profile models.Profile, capability capabilities.EncoderCapability, main10 bool) []string {
	realtime := profileWorkerBool(profile, "videoToolboxRealtime", false)
	args := []string{"-realtime", map[bool]string{true: "1", false: "0"}[realtime]}
	policy := requestedVideoToolboxBFramePolicy(profile)
	if policy == "auto" && realtime {
		policy = "disabled"
	}
	switch policy {
	case "enabled":
		if videoToolboxOptionalFeatureSupported(capability, "bframes", main10) {
			args = append(args, "-bf", strconv.Itoa(clampVideoToolboxBFrames(workerIntValue(profile.WorkerConfig["videoToolboxBFrames"], 3))))
		}
	case "disabled":
		if videoToolboxBFramesDisabledSupported(capability, main10) {
			args = append(args, "-bf", "0")
		}
	}
	return args
}

func clampVideoToolboxBFrames(value int) int {
	if value < 1 {
		return 1
	}
	if value > 4 {
		return 4
	}
	return value
}

func videoToolboxBFramesDisabledSupported(capability capabilities.EncoderCapability, main10 bool) bool {
	if main10 {
		return capability.TestedModes["videoToolboxBFramesDisabledMain10"]
	}
	return capability.VideoToolboxBFramesDisabled
}

func videoToolboxStrategyMultiplier(profile models.Profile, capability capabilities.EncoderCapability) float64 {
	main10 := profileUsesTenBit(profile)
	policy := requestedVideoToolboxBFramePolicy(profile)
	if policy == "auto" && profileWorkerBool(profile, "videoToolboxRealtime", false) {
		policy = "disabled"
	}
	switch policy {
	case "enabled":
		if videoToolboxOptionalFeatureSupported(capability, "bframes", main10) {
			return 1
		}
		return 1.03
	case "disabled":
		if videoToolboxBFramesDisabledSupported(capability, main10) {
			return 1.08
		}
		return 1.03
	default:
		key := "videoToolboxAutoBFramesMain"
		if main10 {
			key = "videoToolboxAutoBFramesMain10"
		}
		if capability.TestedModes[key] {
			return 1
		}
		return 1.03
	}
}

func qsvRateControlArgs(profile models.Profile) []string {
	effectiveRateControl := strings.ToLower(strings.TrimSpace(workerStringValue(profile.WorkerConfig["qsvEffectiveRateControl"])))
	switch effectiveRateControl {
	case "cqp":
		return []string{"-global_quality", fmt.Sprintf("%d", workerIntValue(profile.WorkerConfig["globalQuality"], defaultQSVQuality(profile.QualityValue))), "-flags", "+qscale"}
	case "vbr", "cbr":
		target := workerIntValue(profile.WorkerConfig["qsvTargetBitrate"], 0)
		maxrate := workerIntValue(profile.WorkerConfig["qsvMaxrate"], target)
		buffer := workerIntValue(profile.WorkerConfig["qsvBuffer"], target*2)
		if target > 0 {
			return []string{"-b:v", fmt.Sprintf("%d", target), "-maxrate", fmt.Sprintf("%d", maxrate), "-bufsize", fmt.Sprintf("%d", buffer)}
		}
	case "icq", "la_icq":
		return []string{"-global_quality", fmt.Sprintf("%d", workerIntValue(profile.WorkerConfig["globalQuality"], defaultQSVQuality(profile.QualityValue)))}
	}
	if capabilities.CheckEncoder("hevc_qsv").ICQ {
		return []string{"-global_quality", fmt.Sprintf("%d", workerIntValue(profile.WorkerConfig["globalQuality"], defaultQSVQuality(profile.QualityValue)))}
	}
	return []string{"-b:v", fmt.Sprintf("%dM", defaultHardwareBitrateMbps(profile.QualityValue))}
}

func videoToolboxOptionalFeatureSupported(capability capabilities.EncoderCapability, feature string, main10 bool) bool {
	switch feature {
	case "bframes":
		return capability.VideoToolboxBFrames && (!main10 || capability.TestedModes["videoToolboxBFramesMain10"])
	case "power_efficiency":
		return capability.VideoToolboxPowerEfficient && (!main10 || capability.TestedModes["videoToolboxPowerEfficientMain10"])
	default:
		return false
	}
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
	filters = canonicalizeRestorationFilterChain(filters)
	filters = renderResolvedUpscaleFilters(filters, profile)
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
	if params := effectiveX265Params(profile); params != "" && resolvedVideoEncoder(profile) == "libx265" {
		args = append(args, "-x265-params", params)
	}
	if encoder == "hevc_qsv" {
		args = append(args, qsvWorkerArgs(profile)...)
	}
	return args
}

const (
	smartUpscaleCASLightStrength  = "0.10"
	smartUpscaleCASMediumStrength = "0.18"
)

// renderResolvedUpscaleFilters translates an already-resolved decision into
// FFmpeg filters. Policy stays in the resolver; this renderer consumes the
// frozen target verbatim and does not inspect source quality or dimensions.
func renderResolvedUpscaleFilters(filters string, profile models.Profile) string {
	if strings.EqualFold(strings.TrimSpace(profile.VideoCodec), "copy") {
		return filters
	}
	decision, ok := resolvedUpscaleDecisionFromProfile(profile)
	if !ok || !decision.UpscaleApplied || decision.TargetWidth <= 0 || decision.TargetHeight <= 0 {
		return filters
	}

	parts := []string{}
	fieldMetadata := []string{}
	preferZScale := false
	zscaleOptions := []string{}
	for _, raw := range splitVideoFilterChain(filters) {
		filter := strings.TrimSpace(raw)
		if filter == "" {
			continue
		}
		name, value, _ := strings.Cut(filter, "=")
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "scale", "scale_qsv", "vpp_qsv":
			// Smart Upscale owns final geometry. Replace any prior geometry
			// scaler rather than producing two resize operations.
			continue
		case "zscale":
			preferZScale = true
			zscaleOptions = zscaleOptionsWithoutGeometry(value)
			continue
		case "setsar", "setdar":
			// The resolved target is square-pixel. Remove conflicting legacy
			// aspect metadata and emit one canonical setsar stage below.
			continue
		case "cas", "unsharp":
			if _, apply := smartUpscaleCASStrength(decision.SharpenMode); apply {
				// A resolved Smart Upscale sharpen is the sole final sharpen
				// authority. The resolver records the legacy conflict.
				continue
			}
		case "setfield":
			fieldMetadata = append(fieldMetadata, filter)
			continue
		}
		parts = append(parts, filter)
	}

	scaler := fmt.Sprintf("scale=%d:%d:flags=lanczos", decision.TargetWidth, decision.TargetHeight)
	if preferZScale {
		scaler = fmt.Sprintf("zscale=w=%d:h=%d:filter=lanczos", decision.TargetWidth, decision.TargetHeight)
		if len(zscaleOptions) > 0 {
			scaler += ":" + strings.Join(zscaleOptions, ":")
		}
	}
	stages := []string{scaler, "setsar=1"}
	if strength, apply := smartUpscaleCASStrength(decision.SharpenMode); apply {
		stages = append(stages, "cas=strength="+strength)
	}
	rendered := make([]string, 0, len(parts)+len(stages)+len(fieldMetadata))
	rendered = append(rendered, parts...)
	rendered = append(rendered, stages...)
	rendered = append(rendered, fieldMetadata...)
	return strings.Join(rendered, ",")
}

type restorationFilterStage int

const (
	restorationStageMotion restorationFilterStage = iota
	restorationStageDeflicker
	restorationStageDeblock
	restorationStageCrop
	restorationStageChromaCleanup
	restorationStageDenoise
	restorationStageDeband
	restorationStageImageAdjustments
	restorationStageColorNormalization
	restorationStageSmartUpscale
	restorationStageSARNormalization
	restorationStageFinalSharpen
	restorationStageFieldMetadata
)

type classifiedRestorationFilter struct {
	filter string
	stage  restorationFilterStage
}

// canonicalizeRestorationFilterChain orders only filters whose semantics are
// known to MVForge. Unknown advanced filters remain stable barriers so the raw
// escape hatch is never guessed at or destructively reordered.
func canonicalizeRestorationFilterChain(filters string) string {
	parts := splitVideoFilterChain(filters)
	if len(parts) == 0 {
		return ""
	}
	parts = deduplicateRestorationAuthorities(parts)
	result := make([]string, 0, len(parts))
	run := make([]classifiedRestorationFilter, 0, len(parts))
	flush := func() {
		sort.SliceStable(run, func(left, right int) bool { return run[left].stage < run[right].stage })
		for _, item := range run {
			result = append(result, item.filter)
		}
		run = run[:0]
	}
	for _, raw := range parts {
		filter := strings.TrimSpace(raw)
		if filter == "" {
			continue
		}
		if stage, ok := restorationStageForFilter(filter); ok {
			run = append(run, classifiedRestorationFilter{filter: filter, stage: stage})
			continue
		}
		flush()
		result = append(result, filter)
	}
	flush()
	return strings.Join(result, ",")
}

func restorationStageForFilter(filter string) (restorationFilterStage, bool) {
	name, value, _ := strings.Cut(strings.TrimSpace(filter), "=")
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "fps", "bwdif", "yadif", "fieldmatch", "decimate":
		return restorationStageMotion, true
	case "deflicker":
		return restorationStageDeflicker, true
	case "deblock":
		return restorationStageDeblock, true
	case "crop":
		return restorationStageCrop, true
	case "chromanr", "chromashift":
		return restorationStageChromaCleanup, true
	case "hqdn3d", "nlmeans", "atadenoise", "dctdnoiz", "fftdnoiz", "owdenoise", "vaguedenoiser":
		return restorationStageDenoise, true
	case "deband", "gradfun":
		return restorationStageDeband, true
	case "exposure", "eq", "colorlevels", "vibrance", "colorbalance", "curves", "normalize", "limiter":
		return restorationStageImageAdjustments, true
	case "colorspace", "colormatrix":
		return restorationStageColorNormalization, true
	case "zscale":
		if zscaleHasGeometry(value) {
			return restorationStageSmartUpscale, true
		}
		return restorationStageColorNormalization, true
	case "scale", "scale_qsv", "vpp_qsv":
		return restorationStageSmartUpscale, true
	case "setsar", "setdar":
		return restorationStageSARNormalization, true
	case "cas", "unsharp":
		return restorationStageFinalSharpen, true
	case "setfield":
		return restorationStageFieldMetadata, true
	default:
		return 0, false
	}
}

func deduplicateRestorationAuthorities(parts []string) []string {
	lastGeometry, lastAspect, lastSharpen := -1, -1, -1
	for index, raw := range parts {
		filter := strings.TrimSpace(raw)
		name, value, _ := strings.Cut(filter, "=")
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "scale", "scale_qsv", "vpp_qsv":
			lastGeometry = index
		case "zscale":
			if zscaleHasGeometry(value) {
				lastGeometry = index
			}
		case "setsar", "setdar":
			lastAspect = index
		case "cas", "unsharp":
			lastSharpen = index
		}
	}
	result := make([]string, 0, len(parts))
	for index, raw := range parts {
		filter := strings.TrimSpace(raw)
		if filter == "" {
			continue
		}
		name, value, _ := strings.Cut(filter, "=")
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "scale", "scale_qsv", "vpp_qsv":
			if index != lastGeometry {
				continue
			}
		case "zscale":
			if zscaleHasGeometry(value) && index != lastGeometry {
				options := zscaleOptionsWithoutGeometry(value)
				if len(options) == 0 {
					continue
				}
				filter = "zscale=" + strings.Join(options, ":")
			}
		case "setsar", "setdar":
			if index != lastAspect {
				continue
			}
		case "cas", "unsharp":
			if index != lastSharpen {
				continue
			}
		}
		result = append(result, filter)
	}
	return result
}

func zscaleHasGeometry(value string) bool {
	for _, option := range strings.Split(value, ":") {
		name, _, found := strings.Cut(strings.TrimSpace(option), "=")
		if !found {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "w", "width", "h", "height":
			return true
		}
	}
	return false
}

func splitVideoFilterChain(filters string) []string {
	parts := []string{}
	start := 0
	quote := rune(0)
	escaped := false
	for index, value := range filters {
		if escaped {
			escaped = false
			continue
		}
		if value == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if value == quote {
				quote = 0
			}
			continue
		}
		if value == '\'' || value == '"' {
			quote = value
			continue
		}
		if value == ',' {
			parts = append(parts, filters[start:index])
			start = index + 1
		}
	}
	parts = append(parts, filters[start:])
	return parts
}

func zscaleOptionsWithoutGeometry(value string) []string {
	result := []string{}
	for _, option := range strings.Split(value, ":") {
		option = strings.TrimSpace(option)
		if option == "" {
			continue
		}
		name, _, found := strings.Cut(option, "=")
		if !found {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "w", "width", "h", "height", "filter":
			continue
		}
		result = append(result, option)
	}
	return result
}

func smartUpscaleCASStrength(mode UpscaleSharpen) (string, bool) {
	switch mode {
	case UpscaleSharpenLight:
		return smartUpscaleCASLightStrength, true
	case UpscaleSharpenMedium:
		return smartUpscaleCASMediumStrength, true
	default:
		return "", false
	}
}

func filterChainWithoutAspectMetadata(filters string) string {
	result := []string{}
	for _, raw := range strings.Split(filters, ",") {
		filter := strings.TrimSpace(raw)
		if filter == "" {
			continue
		}
		name, _, _ := strings.Cut(filter, "=")
		if strings.EqualFold(strings.TrimSpace(name), "setsar") || strings.EqualFold(strings.TrimSpace(name), "setdar") {
			continue
		}
		result = append(result, filter)
	}
	return strings.Join(result, ",")
}

func applyCropAspectPolicy(filters string, profile models.Profile, source *MediaStream) string {
	if source == nil || !strings.Contains(filters, "crop=") {
		return filters
	}
	policy := strings.ToLower(workerStringValue(profile.WorkerConfig["cropAspectPolicy"]))
	if policy == "" || policy == "source_sar" {
		if validAspectRatio(source.SampleAspectRatio) && !strings.Contains(filters, "setsar=") && !strings.Contains(filters, "setdar=") {
			return filters + ",setsar=" + strings.ReplaceAll(source.SampleAspectRatio, ":", "/")
		}
		return filters
	}
	if policy != "preserve_dar" {
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
	divisor := greatestCommonDivisor(numerator, denominator)
	return filters + fmt.Sprintf(",setsar=%d/%d", numerator/divisor, denominator/divisor)
}

func greatestCommonDivisor(left, right int) int {
	if left < 0 {
		left = -left
	}
	if right < 0 {
		right = -right
	}
	for right != 0 {
		left, right = right, left%right
	}
	if left == 0 {
		return 1
	}
	return left
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

type qsvEffectiveFeatures struct {
	LookAhead   bool
	ExtendedBRC bool
	AdaptiveI   bool
	AdaptiveB   bool
}

func resolveQSVEffectiveFeatures(profile models.Profile, capability capabilities.EncoderCapability) qsvEffectiveFeatures {
	main10 := profileUsesTenBit(profile)
	rateControl := strings.ToLower(strings.TrimSpace(workerStringValue(profile.WorkerConfig["qsvEffectiveRateControl"])))
	if rateControl == "" {
		rateControl = strings.ToLower(strings.TrimSpace(workerStringValue(profile.WorkerConfig["qsvRateControl"])))
	}
	features := qsvEffectiveFeatures{
		AdaptiveI: map[bool]bool{false: capability.QSVAdaptiveIMain8, true: capability.QSVAdaptiveIMain10}[main10],
		AdaptiveB: map[bool]bool{false: capability.QSVAdaptiveBMain8, true: capability.QSVAdaptiveBMain10}[main10],
	}
	switch rateControl {
	case "la_icq":
		features.LookAhead = map[bool]bool{false: capability.QSVLAICQMain8, true: capability.QSVLAICQMain10}[main10]
	case "vbr":
		features.LookAhead = map[bool]bool{false: capability.QSVVBRLookAheadMain8, true: capability.QSVVBRLookAheadMain10}[main10]
		features.ExtendedBRC = map[bool]bool{false: capability.QSVVBRExtBRCMain8, true: capability.QSVVBRExtBRCMain10}[main10]
	case "cbr":
		features.LookAhead = map[bool]bool{false: capability.QSVCBRLookAheadMain8, true: capability.QSVCBRLookAheadMain10}[main10]
		features.ExtendedBRC = map[bool]bool{false: capability.QSVCBRExtBRCMain8, true: capability.QSVCBRExtBRCMain10}[main10]
	}
	return features
}

func qsvWorkerArgsForCapability(profile models.Profile, capability capabilities.EncoderCapability) []string {
	args := []string{}
	main10 := profileUsesTenBit(profile)
	features := resolveQSVEffectiveFeatures(profile, capability)
	lowPowerSupported := capability.TestedModes["qsvLowPowerMain8"]
	if main10 {
		lowPowerSupported = capability.QSVLowPowerMain10
	}
	if profileWorkerBool(profile, "qsvLowPower", false) && lowPowerSupported {
		args = append(args, "-low_power", "1")
	}
	rateControl := strings.ToLower(strings.TrimSpace(workerStringValue(profile.WorkerConfig["qsvEffectiveRateControl"])))
	if rateControl == "" {
		rateControl = strings.ToLower(strings.TrimSpace(workerStringValue(profile.WorkerConfig["qsvRateControl"])))
	}
	lookAheadEnabled := features.LookAhead
	if lookAheadEnabled {
		if rateControl == "la_icq" {
			args = append(args, "-look_ahead", "1")
		}
	}
	lookAheadDepth := workerIntValue(profile.WorkerConfig["qsvLookAheadDepth"], 0)
	if lookAheadEnabled && lookAheadDepth > 0 {
		args = append(args, "-look_ahead_depth", fmt.Sprintf("%d", min(100, max(10, lookAheadDepth))))
	}
	if profileWorkerBool(profile, "qsvExtendedBRC", false) && features.ExtendedBRC {
		args = append(args, "-extbrc", "1")
	}
	frameStructureEnabled := normalizedFrameStructureMode(workerStringValue(profile.WorkerConfig["frameStructureMode"])) != "off"
	if frameStructureEnabled && profileWorkerBool(profile, "qsvAdaptiveI", false) && features.AdaptiveI {
		args = append(args, "-adaptive_i", "1")
	}
	// GopRefDist=1 (-bf 0) has no regular B-frame placement for Adaptive B to
	// manage. Keep the requested profile intact for provenance, but normalize
	// the effective QSV command by omitting Adaptive B.
	bFramesDisabled := normalizedFrameStructureBFrameMode(workerStringValue(profile.WorkerConfig["frameStructureBFrameMode"])) == "off"
	if frameStructureEnabled && !bFramesDisabled && profileWorkerBool(profile, "qsvAdaptiveB", false) && features.AdaptiveB {
		args = append(args, "-adaptive_b", "1")
	}
	pStrategy := min(2, max(0, workerIntValue(profile.WorkerConfig["qsvPStrategy"], 0)))
	if frameStructureEnabled && pStrategy > 0 {
		format := "Main8"
		if main10 {
			format = "Main10"
		}
		mode := map[int]string{1: "Simple", 2: "Pyramid"}[pStrategy]
		pStrategyAllowed := normalizedFrameStructureBFrameMode(workerStringValue(profile.WorkerConfig["frameStructureBFrameMode"])) == "off"
		if pStrategyAllowed && capability.TestedModes["qsvPStrategy"+mode+format] {
			args = append(args, "-p_strategy", strconv.Itoa(pStrategy))
		}
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
	return (encoder != "hevc_qsv" && encoder != "hevc_vaapi" && encoder != "hevc_videotoolbox") || !profileUsesTenBit(profile) || capability.Main10
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

func normalizedVideoToolboxProfile(profile models.Profile) string {
	if profileUsesTenBit(profile) {
		return "main10"
	}
	return "main"
}

func defaultVideoToolboxBitrateMbps(quality int) float64 {
	switch {
	case quality > 0 && quality <= 18:
		return 4
	case quality >= 25:
		return 1.5
	default:
		return 2
	}
}

func formatMbps(value float64) string {
	value = math.Round(value*100) / 100
	return strconv.FormatFloat(value, 'f', -1, 64) + "M"
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
	return probeMediaStreamsWithFrameCount(inputPath, false)
}

func probeMediaStreamsWithFrameCount(inputPath string, countFrames bool) (MediaStreamInventory, error) {
	args := []string{
		"-v", "error",
	}
	if countFrames {
		args = append(args, "-count_frames")
	}
	args = append(args,
		"-show_entries", "format=duration,bit_rate:stream=index,codec_type,codec_name,channels,channel_layout,field_order,pix_fmt,color_range,color_space,color_transfer,color_primaries,width,height,bit_rate,avg_frame_rate,r_frame_rate,nb_read_frames,start_time,sample_aspect_ratio,display_aspect_ratio:stream_tags=language,title:stream_disposition=default,forced,attached_pic,still_image",
		"-show_chapters",
		"-of", "json",
		inputPath,
	)
	cmd := exec.Command("ffprobe", args...)
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
			RealFrameRate      string            `json:"r_frame_rate"`
			DecodedFrames      string            `json:"nb_read_frames"`
			StartTime          string            `json:"start_time"`
			SampleAspectRatio  string            `json:"sample_aspect_ratio"`
			DisplayAspectRatio string            `json:"display_aspect_ratio"`
			Tags               map[string]string `json:"tags"`
			Disposition        struct {
				Default     int `json:"default"`
				Forced      int `json:"forced"`
				AttachedPic int `json:"attached_pic"`
				StillImage  int `json:"still_image"`
			} `json:"disposition"`
		} `json:"streams"`
		Chapters []struct {
			ID int `json:"id"`
		} `json:"chapters"`
	}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		return MediaStreamInventory{}, err
	}

	inventory := MediaStreamInventory{Video: []MediaStream{}, Audio: []MediaAudioStream{}, Subtitle: []MediaStream{}, Attachment: []MediaStream{}, Data: []MediaStream{}, Duration: parseFloat(response.Format.Duration), ChapterCount: len(response.Chapters), TotalBitrate: parseInt(response.Format.Bitrate)}
	for _, stream := range response.Streams {
		common := MediaStream{
			Index:              stream.Index,
			Codec:              stream.CodecName,
			FieldOrder:         stream.FieldOrder,
			PixelFormat:        stream.PixelFormat,
			AttachedPic:        stream.Disposition.AttachedPic == 1,
			StillImage:         stream.Disposition.StillImage == 1,
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
			RealFrameRate:      stream.RealFrameRate,
			DecodedFrames:      parseInt(stream.DecodedFrames),
			StartTime:          parseFloat(stream.StartTime),
			StartTimeValid:     optionalSeconds(stream.StartTime) != nil,
			SampleAspectRatio:  stream.SampleAspectRatio,
			DisplayAspectRatio: stream.DisplayAspectRatio,
		}
		switch stream.CodecType {
		case "video":
			inventory.Video = append(inventory.Video, common)
		case "audio":
			inventory.Audio = append(inventory.Audio, MediaAudioStream{
				Index:          stream.Index,
				Codec:          stream.CodecName,
				Channels:       stream.Channels,
				ChannelLayout:  stream.ChannelLayout,
				Language:       stream.Tags["language"],
				Title:          stream.Tags["title"],
				Default:        stream.Disposition.Default == 1,
				Forced:         stream.Disposition.Forced == 1,
				Bitrate:        parseInt(stream.Bitrate),
				StartTime:      parseFloat(stream.StartTime),
				StartTimeValid: optionalSeconds(stream.StartTime) != nil,
			})
		case "subtitle":
			inventory.Subtitle = append(inventory.Subtitle, common)
		case "attachment":
			inventory.Attachment = append(inventory.Attachment, common)
		case "data":
			inventory.Data = append(inventory.Data, common)
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

func isAuxiliaryVideoStream(
	stream MediaStream,
	videoIndex int,
	totalVideos int,
) bool {
	if totalVideos <= 1 {
		return false
	}

	// El primer video sigue siendo tratado como principal.
	if videoIndex == 0 {
		return false
	}

	if stream.AttachedPic || stream.StillImage {
		return true
	}

	switch strings.ToLower(strings.TrimSpace(stream.Codec)) {
	case "mjpeg":
		return true
	default:
		return false
	}
}

func selectedVideoStreamsForPlan(
	plan MediaJobPlan,
) []MediaStream {
	selected := selectedVideoStreams(plan.Streams.Video, plan.Override)
	if plan.ResolvedTracks != nil {
		allowed := map[int]struct{}{}
		for _, stream := range plan.ResolvedTracks.VideoStreams {
			allowed[stream.StreamIndex] = struct{}{}
		}
		selected = selected[:0]
		for _, stream := range plan.Streams.Video {
			if _, ok := allowed[stream.Index]; ok {
				selected = append(selected, stream)
			}
		}
	}

	if plan.ProcessingMode != ProcessingModeFullEncode {
		return selected
	}

	result := make([]MediaStream, 0, len(selected))

	for index, stream := range selected {
		if isAuxiliaryVideoStream(
			stream,
			index,
			len(selected),
		) {
			continue
		}

		result = append(result, stream)
	}

	return result
}
