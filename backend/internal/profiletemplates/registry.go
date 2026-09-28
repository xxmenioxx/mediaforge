package profiletemplates

import (
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

type Template struct {
	Key              string                   `json:"key"`
	Version          int                      `json:"version"`
	Name             string                   `json:"name"`
	Category         string                   `json:"category"`
	Description      string                   `json:"description"`
	Definition       models.ProfileDefinition `json:"definition"`
	LegacySeedPreset string                   `json:"-"`
}

var registry = []Template{
	{
		Key: "dvd-archive-main10", Version: 1, Name: "DVD Archive x265 Main10", Category: "Source",
		Description:      "Default DVD profile for clean SD sources. Preserves all audio tracks, subtitles, chapters, and targets a compact high-quality x265 Main10 output.",
		LegacySeedPreset: "dvd-archive-main10",
		Definition: models.ProfileDefinition{
			Container: "mkv", VideoCodec: "x265_10bit", AudioCodec: "copy", QualityMode: "crf", QualityValue: 20,
			PreserveHDR: false, PreserveSubtitles: true, PreserveChapters: true,
			WorkerConfig: models.JSONMap{"source": "mvforge-v1", "engine": "FFmpeg", "preset": "dvd-archive-main10", "videoPreset": "medium", "pixFmt": "yuv420p10le", "x265Params": "aq-mode=3:aq-strength=0.8:deblock=-1,-1"},
		},
	},
	{
		Key: "anime-dvd-main10", Version: 1, Name: "Anime DVD x265 Main10", Category: "Source",
		Description:      "Default anime DVD profile. Preserves all audio tracks, subtitles, chapters, and is tuned for line art with balanced size.",
		LegacySeedPreset: "anime-dvd-main10",
		Definition: models.ProfileDefinition{
			Container: "mkv", VideoCodec: "x265_10bit", AudioCodec: "copy", QualityMode: "crf", QualityValue: 20,
			PreserveHDR: false, PreserveSubtitles: true, PreserveChapters: true,
			WorkerConfig: models.JSONMap{"source": "mvforge-v1", "engine": "FFmpeg", "preset": "anime-dvd-main10", "videoPreset": "medium", "pixFmt": "yuv420p10le", "x265Params": "aq-mode=3:aq-strength=0.9:deblock=-1,-1"},
		},
	},
	{
		Key: "series-balanced-main10", Version: 1, Name: "Series Balanced x265 Main10", Category: "Source",
		Description:      "Default profile for episode batches and series. Preserves all audio tracks, subtitles, chapters, and is slightly smaller than DVD/Anime defaults.",
		LegacySeedPreset: "series-balanced-main10",
		Definition: models.ProfileDefinition{
			Container: "mkv", VideoCodec: "x265_10bit", AudioCodec: "copy", QualityMode: "crf", QualityValue: 21,
			PreserveHDR: false, PreserveSubtitles: true, PreserveChapters: true,
			WorkerConfig: models.JSONMap{"source": "mvforge-v1", "engine": "FFmpeg", "preset": "series-balanced-main10", "videoPreset": "medium", "pixFmt": "yuv420p10le", "x265Params": "aq-mode=3:aq-strength=0.75:deblock=-1,-1"},
		},
	},
	{
		Key: "hevc-small-size", Version: 1, Name: "HEVC Small Size", Category: "Size",
		Description:      "Software x265 profile for assets where saving space matters more than speed.",
		LegacySeedPreset: "hevc-small-size",
		Definition: models.ProfileDefinition{
			Container: "mkv", VideoCodec: "x265_10bit", AudioCodec: "copy", QualityMode: "crf", QualityValue: 21,
			PreserveHDR: true, PreserveSubtitles: true, PreserveChapters: true,
			WorkerConfig: models.JSONMap{"source": "mvforge-v1", "engine": "FFmpeg", "preset": "hevc-small-size", "preferredEncoder": "software", "videoEncoder": "libx265", "useHardwareIfAvailable": false, "videoPreset": "slow", "pixFmt": "yuv420p10le", "x265Params": "aq-mode=3:aq-strength=0.8:deblock=-1,-1", "addAacStereoTrack": true, "aacStereoDefault": false, "preserveOriginalAudio": true, "warnSubtitleFormats": true, "preferSrtSubtitles": false},
		},
	},
	{
		Key: "hevc-balanced-fast", Version: 1, Name: "HEVC Balanced Fast", Category: "Speed",
		Description:      "Hardware HEVC profile for faster queues and lower NAS pressure.",
		LegacySeedPreset: "hevc-balanced-fast",
		Definition: models.ProfileDefinition{
			Container: "mkv", VideoCodec: "x265_10bit", AudioCodec: "copy", QualityMode: "crf", QualityValue: 20,
			PreserveHDR: true, PreserveSubtitles: true, PreserveChapters: true,
			WorkerConfig: models.JSONMap{"source": "mvforge-v1", "engine": "FFmpeg", "preset": "hevc-balanced-fast", "preferredEncoder": "hardware", "videoEncoder": "hevc_qsv", "useHardwareIfAvailable": true, "globalQuality": 18, "qsvRateControl": "icq", "qsvLookAheadDepth": 40, "qsvExtendedBRC": false, "qsvAdaptiveI": true, "qsvAdaptiveB": true, "videoPreset": "medium", "pixFmt": "p010le", "addAacStereoTrack": true, "aacStereoDefault": false, "preserveOriginalAudio": true, "warnSubtitleFormats": true, "preferSrtSubtitles": false},
		},
	},
	{
		Key: "hevc-archive-quality", Version: 1, Name: "HEVC Archive Quality", Category: "Quality",
		Description:      "Software x265 profile for important movies, concerts, and difficult sources.",
		LegacySeedPreset: "hevc-archive-quality",
		Definition: models.ProfileDefinition{
			Container: "mkv", VideoCodec: "x265_10bit", AudioCodec: "copy", QualityMode: "crf", QualityValue: 19,
			PreserveHDR: true, PreserveSubtitles: true, PreserveChapters: true,
			WorkerConfig: models.JSONMap{"source": "mvforge-v1", "engine": "FFmpeg", "preset": "hevc-archive-quality", "preferredEncoder": "software", "videoEncoder": "libx265", "useHardwareIfAvailable": false, "videoPreset": "slow", "pixFmt": "yuv420p10le", "x265Params": "aq-mode=3:aq-strength=0.9:deblock=-1,-1", "addAacStereoTrack": true, "aacStereoDefault": false, "preserveOriginalAudio": true, "warnSubtitleFormats": true, "preferSrtSubtitles": false},
		},
	},
	{
		Key: "hevc-bulk-convert", Version: 1, Name: "HEVC Bulk Convert", Category: "Bulk",
		Description:      "Hardware HEVC profile for large libraries and unattended bulk conversion.",
		LegacySeedPreset: "hevc-bulk-convert",
		Definition: models.ProfileDefinition{
			Container: "mkv", VideoCodec: "x265_10bit", AudioCodec: "copy", QualityMode: "crf", QualityValue: 23,
			PreserveHDR: true, PreserveSubtitles: true, PreserveChapters: true,
			WorkerConfig: models.JSONMap{"source": "mvforge-v1", "engine": "FFmpeg", "preset": "hevc-bulk-convert", "preferredEncoder": "hardware", "videoEncoder": "hevc_qsv", "useHardwareIfAvailable": true, "globalQuality": 21, "qsvRateControl": "icq", "qsvLookAheadDepth": 40, "qsvExtendedBRC": false, "qsvAdaptiveI": true, "qsvAdaptiveB": false, "videoPreset": "medium", "pixFmt": "p010le", "addAacStereoTrack": true, "aacStereoDefault": false, "preserveOriginalAudio": true, "warnSubtitleFormats": true, "preferSrtSubtitles": false},
		},
	},
	{
		Key: "archive-quality", Version: 1, Name: "Archive Quality", Category: "Quality",
		Description: "Best for preserving high-quality sources. Keeps all audio tracks, subtitles, and chapters.",
		Definition: models.ProfileDefinition{
			Container: "mkv", VideoCodec: "x265", AudioCodec: "copy", QualityMode: "crf", QualityValue: 20,
			PreserveHDR: true, PreserveSubtitles: true, PreserveChapters: true,
			WorkerConfig: models.JSONMap{"videoPreset": "medium", "pixFmt": "yuv420p10le", "videoEncoder": "libx265", "preferredEncoder": "software", "useHardwareIfAvailable": false, "addAacStereoTrack": true, "aacStereoDefault": false, "preserveOriginalAudio": true, "preferSrtSubtitles": false, "warnSubtitleFormats": true},
		},
	},
	{
		Key: "streaming-compatible", Version: 1, Name: "Streaming Compatible", Category: "Compatibility",
		Description: "Broad playback-oriented video while still preserving all source audio tracks, subtitles, and chapters.",
		Definition: models.ProfileDefinition{
			Container: "mkv", VideoCodec: "x264", AudioCodec: "copy", QualityMode: "crf", QualityValue: 22,
			PreserveHDR: false, PreserveSubtitles: true, PreserveChapters: true,
			WorkerConfig: models.JSONMap{"videoPreset": "medium", "pixFmt": "yuv420p", "videoEncoder": "auto", "preferredEncoder": "hardware", "useHardwareIfAvailable": true, "addAacStereoTrack": true, "aacStereoDefault": false, "preserveOriginalAudio": true, "preferSrtSubtitles": true, "warnSubtitleFormats": true},
		},
	},
	{
		Key: "smaller-file", Version: 1, Name: "Smaller File", Category: "Size",
		Description: "Best for reducing video size while preserving all source audio tracks, subtitles, and chapters.",
		Definition: models.ProfileDefinition{
			Container: "mkv", VideoCodec: "x265", AudioCodec: "copy", QualityMode: "crf", QualityValue: 25,
			PreserveHDR: false, PreserveSubtitles: true, PreserveChapters: true,
			WorkerConfig: models.JSONMap{"videoPreset": "slow", "pixFmt": "yuv420p10le", "videoEncoder": "libx265", "preferredEncoder": "software", "useHardwareIfAvailable": false, "addAacStereoTrack": true, "aacStereoDefault": false, "preserveOriginalAudio": true, "preferSrtSubtitles": false, "warnSubtitleFormats": true},
		},
	},
	{
		Key: "preserve-original", Version: 1, Name: "Preserve Original", Category: "Preservation",
		Description: "Best for remuxing or keeping every stream untouched.",
		Definition: models.ProfileDefinition{
			Container: "mkv", VideoCodec: "copy", AudioCodec: "copy", QualityMode: "crf", QualityValue: 0,
			PreserveHDR: true, PreserveSubtitles: true, PreserveChapters: true,
			WorkerConfig: models.JSONMap{"videoPreset": "medium", "pixFmt": "yuv420p10le"},
		},
	},
}

func All() []Template {
	result := make([]Template, len(registry))
	for index := range registry {
		result[index] = cloneTemplate(registry[index])
	}
	return result
}

func Get(key string) (Template, bool) {
	key = strings.TrimSpace(key)
	for _, template := range registry {
		if template.Key == key {
			return cloneTemplate(template), true
		}
	}
	return Template{}, false
}

func GetVersion(key string, version int) (Template, bool) {
	template, ok := Get(key)
	return template, ok && template.Version == version
}

func LegacySeedTemplate(profile models.Profile) (Template, bool) {
	if strings.TrimSpace(stringFromConfig(profile.WorkerConfig["source"])) != "mvforge-v1" {
		return Template{}, false
	}
	preset := strings.TrimSpace(stringFromConfig(profile.WorkerConfig["preset"]))
	for _, template := range registry {
		if template.LegacySeedPreset == preset && preset != "" {
			return cloneTemplate(template), true
		}
	}
	return Template{}, false
}

func stringFromConfig(value any) string {
	text, _ := value.(string)
	return text
}

func cloneTemplate(template Template) Template {
	template.Definition = template.Definition.Clone()
	return template
}
