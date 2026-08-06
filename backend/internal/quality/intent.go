package quality

import (
	"strconv"
	"strings"
	"time"
)

type IntentInput struct {
	Preset               string
	SourcePath           string
	SourceWidth          int
	SourceHeight         int
	SourceVideoBitrate   int64
	DurationSeconds      float64
	HDR                  bool
	GrainScore           float64
	GrainScoreKnown      bool
	MotionScore          float64
	MotionScoreKnown     bool
	ComplexityScore      float64
	ComplexityScoreKnown bool
	ContentType          string
	AudioBitrate         int64
	SubtitleSize         int64
	VideoFilters         string
	ColorPolicy          string
	RequestedRateControl string
	RequestedBitDepth    int
	RequestedPixelFormat string
	MeasuredVideoBitrate int64
	HistoricalRatioMin   float64
	HistoricalRatioMax   float64
	HistoricalSamples    int
	VideoToolboxRealtime bool
	BFramePolicy         string
	BFrameCount          int
	AutoAdjustBitrate    bool
}

func NewIntent(input IntentInput) QualityIntent {
	width, height := outputDimensions(input.VideoFilters, input.SourceWidth, input.SourceHeight)
	contentType := normalizeContentType(input.ContentType)
	if contentType == ContentUnknown {
		contentType = contentTypeFromPath(input.SourcePath)
	}
	return QualityIntent{
		Preset:          Preset(strings.ToLower(strings.TrimSpace(input.Preset))),
		ResolutionClass: resolutionClass(height),
		SourceWidth:     input.SourceWidth, SourceHeight: input.SourceHeight,
		OutputWidth: width, OutputHeight: height,
		SourceVideoBitrate: input.SourceVideoBitrate,
		Duration:           time.Duration(input.DurationSeconds * float64(time.Second)),
		HDR:                input.HDR,
		GrainScore:         input.GrainScore, GrainScoreKnown: input.GrainScoreKnown,
		MotionScore: input.MotionScore, MotionScoreKnown: input.MotionScoreKnown,
		ComplexityScore: input.ComplexityScore, ComplexityScoreKnown: input.ComplexityScoreKnown,
		ContentType:          contentType,
		AudioBitrate:         input.AudioBitrate,
		SubtitleSize:         input.SubtitleSize,
		ColorPolicy:          strings.ToLower(strings.TrimSpace(input.ColorPolicy)),
		RequestedRateControl: strings.ToLower(strings.TrimSpace(input.RequestedRateControl)),
		RequestedBitDepth:    input.RequestedBitDepth,
		RequestedPixelFormat: strings.ToLower(strings.TrimSpace(input.RequestedPixelFormat)),
		MeasuredVideoBitrate: input.MeasuredVideoBitrate,
		HistoricalRatioMin:   input.HistoricalRatioMin,
		HistoricalRatioMax:   input.HistoricalRatioMax,
		HistoricalSamples:    input.HistoricalSamples,
		VideoToolboxRealtime: input.VideoToolboxRealtime,
		BFramePolicy:         normalizeBFramePolicy(input.BFramePolicy),
		BFrameCount:          input.BFrameCount,
		AutoAdjustBitrate:    input.AutoAdjustBitrate,
	}
}

func normalizeBFramePolicy(value string) BFramePolicy {
	switch BFramePolicy(strings.ToLower(strings.TrimSpace(value))) {
	case BFrameEnabled:
		return BFrameEnabled
	case BFrameDisabled:
		return BFrameDisabled
	default:
		return BFrameAuto
	}
}

func outputDimensions(filters string, sourceWidth, sourceHeight int) (int, int) {
	width, height := sourceWidth, sourceHeight
	for _, part := range strings.Split(filters, ",") {
		part = strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(part, "crop="):
			values := strings.Split(strings.TrimPrefix(part, "crop="), ":")
			if len(values) >= 2 {
				if value, err := strconv.Atoi(values[0]); err == nil && value > 0 {
					width = value
				}
				if value, err := strconv.Atoi(values[1]); err == nil && value > 0 {
					height = value
				}
			}
		case strings.HasPrefix(part, "scale="):
			values := strings.Split(strings.TrimPrefix(part, "scale="), ":")
			if len(values) >= 2 {
				if value, err := strconv.Atoi(values[0]); err == nil && value > 0 {
					width = value
				}
				if value, err := strconv.Atoi(values[1]); err == nil && value > 0 {
					height = value
				}
			}
		}
	}
	return width, height
}

func resolutionClass(height int) ResolutionClass {
	switch {
	case height <= 0:
		return ResolutionUnknown
	case height <= 576:
		return ResolutionSD
	case height <= 720:
		return ResolutionHD
	case height <= 1080:
		return ResolutionFullHD
	default:
		return ResolutionUHD
	}
}

func normalizeContentType(value string) ContentType {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "anime", "anime-movie", "anime-movies", "animation":
		return ContentAnime
	case "movie", "movies":
		return ContentMovie
	case "series", "episode", "season":
		return ContentSeries
	case "concert", "concerts", "music-video", "music-videos":
		return ContentConcert
	case "sport", "sports":
		return ContentSports
	case "documentary", "documentaries":
		return ContentDocumentary
	default:
		return ContentUnknown
	}
}

func contentTypeFromPath(path string) ContentType {
	path = "/" + strings.Trim(strings.ToLower(path), "/") + "/"
	for _, candidate := range []struct {
		parts []string
		value ContentType
	}{
		{[]string{"/anime/", "/anime-movies/"}, ContentAnime},
		{[]string{"/concert/", "/concerts/", "/music-videos/"}, ContentConcert},
		{[]string{"/sport/", "/sports/"}, ContentSports},
		{[]string{"/documentary/", "/documentaries/"}, ContentDocumentary},
		{[]string{"/series/", "/tv/"}, ContentSeries},
		{[]string{"/movie/", "/movies/"}, ContentMovie},
	} {
		for _, part := range candidate.parts {
			if strings.Contains(path, part) {
				return candidate.value
			}
		}
	}
	return ContentUnknown
}
