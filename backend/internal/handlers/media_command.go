package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

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
}

type MediaStreamInventory struct {
	Audio []MediaAudioStream
}

type MediaAudioStream struct {
	Index         int
	Codec         string
	Channels      int
	ChannelLayout string
	Language      string
	Title         string
	Default       bool
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

	args = append(args, "-i", plan.InputPath, "-map", "0")

	enhancedAudioIndex := -1
	if plan.AudioProfile != nil && len(plan.Streams.Audio) > 0 {
		enhancedAudioIndex = len(plan.Streams.Audio)
		args = append(args, "-map", "0:a:0")
	}

	args = append(args, "-c", "copy")
	if plan.ProcessingMode == ProcessingModeFullEncode {
		args = append(args, splitArgs(codecArgs("-c:v", plan.Profile.VideoCodec, plan.Profile.QualityMode, plan.Profile.QualityValue))...)
		args = append(args, videoWorkerArgs(plan.Profile)...)
	} else {
		args = append(args, "-c:v", "copy")
	}

	if plan.AudioProfile == nil {
		args = appendAudioCodecArgs(args, plan.Profile)
	} else if enhancedAudioIndex >= 0 {
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
	}

	for index, stream := range plan.Streams.Audio {
		args = append(args,
			fmt.Sprintf("-metadata:s:a:%d", index), "title="+originalAudioTitle(stream),
			fmt.Sprintf("-disposition:a:%d", index), "0",
		)
	}

	if !plan.Profile.PreserveSubtitles {
		args = append(args, "-sn")
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

func appendAudioCodecArgs(args []string, profile models.Profile) []string {
	if profile.AudioCodec == "" || profile.AudioCodec == "copy" {
		return args
	}
	return append(args, splitArgs(codecArgs("-c:a", profile.AudioCodec, "", 0))...)
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
	if params := workerStringValue(profile.WorkerConfig["x265Params"]); params != "" && strings.Contains(ffmpegCodecName(codec), "265") {
		args = append(args, "-x265-params", params)
	}
	return args
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
		"-select_streams", "a",
		"-show_entries", "stream=index,codec_name,channels,channel_layout:stream_tags=language,title:stream_disposition=default",
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
			CodecName     string            `json:"codec_name"`
			Channels      int               `json:"channels"`
			ChannelLayout string            `json:"channel_layout"`
			Tags          map[string]string `json:"tags"`
			Disposition   struct {
				Default int `json:"default"`
			} `json:"disposition"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		return MediaStreamInventory{}, err
	}

	inventory := MediaStreamInventory{Audio: []MediaAudioStream{}}
	for _, stream := range response.Streams {
		inventory.Audio = append(inventory.Audio, MediaAudioStream{
			Index:         stream.Index,
			Codec:         stream.CodecName,
			Channels:      stream.Channels,
			ChannelLayout: stream.ChannelLayout,
			Language:      stream.Tags["language"],
			Title:         stream.Tags["title"],
			Default:       stream.Disposition.Default == 1,
		})
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
