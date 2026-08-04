package quality

import "time"

type Preset string

const (
	PresetCompact     Preset = "compact"
	PresetMedium      Preset = "medium"
	PresetRecommended Preset = "recommended"
	PresetBest        Preset = "best_quality"
	PresetHighQuality Preset = "high_quality"
	PresetArchive     Preset = "archive"
	PresetMaster      Preset = "master"
	PresetCustom      Preset = "custom"
)

type ResolutionClass string

const (
	ResolutionSD      ResolutionClass = "sd"
	ResolutionHD      ResolutionClass = "hd"
	ResolutionFullHD  ResolutionClass = "full_hd"
	ResolutionUHD     ResolutionClass = "uhd"
	ResolutionUnknown ResolutionClass = "unknown"
)

type ContentType string

const (
	ContentUnknown     ContentType = "unknown"
	ContentAnime       ContentType = "anime"
	ContentMovie       ContentType = "movie"
	ContentSeries      ContentType = "series"
	ContentConcert     ContentType = "concert"
	ContentSports      ContentType = "sports"
	ContentDocumentary ContentType = "documentary"
)

type QualityIntent struct {
	Preset               Preset
	ResolutionClass      ResolutionClass
	SourceWidth          int
	SourceHeight         int
	OutputWidth          int
	OutputHeight         int
	SourceVideoBitrate   int64
	Duration             time.Duration
	HDR                  bool
	GrainScore           float64
	GrainScoreKnown      bool
	MotionScore          float64
	MotionScoreKnown     bool
	ComplexityScore      float64
	ComplexityScoreKnown bool
	ContentType          ContentType
	AudioBitrate         int64
	SubtitleSize         int64
	ColorPolicy          string
	RequestedRateControl string
	RequestedBitDepth    int
	RequestedPixelFormat string
	MeasuredVideoBitrate int64
	HistoricalRatioMin   float64
	HistoricalRatioMax   float64
	HistoricalSamples    int
}

type WorkerCapabilities struct {
	Encoder         string
	Main            bool
	Main10          bool
	ICQ             bool
	LAICQ           bool
	CQP             bool
	VBR             bool
	CBR             bool
	LowPower        bool
	ExtendedBRC     bool
	AdaptiveI       bool
	AdaptiveB       bool
	FullCombination bool
}

type EncoderRecommendation struct {
	Encoder                  string   `json:"encoder"`
	RequestedRateControl     string   `json:"requestedRateControl"`
	EffectiveRateControl     string   `json:"effectiveRateControl"`
	RateControlFallback      string   `json:"rateControlFallback,omitempty"`
	TargetBitrate            *int64   `json:"targetBitrate,omitempty"`
	RequestedGlobalQuality   *int     `json:"requestedGlobalQuality,omitempty"`
	GlobalQuality            *int     `json:"globalQuality,omitempty"`
	QualityAdjustment        int      `json:"qualityAdjustment"`
	QualityReasons           []string `json:"qualityReasons"`
	Maxrate                  *int64   `json:"maxrate,omitempty"`
	Buffer                   *int64   `json:"buffer,omitempty"`
	Profile                  string   `json:"profile"`
	PixelFormat              string   `json:"pixelFormat"`
	ColorPolicy              string   `json:"colorPolicy"`
	Realtime                 bool     `json:"realtime"`
	AllowFrameReordering     bool     `json:"allowFrameReordering"`
	PowerEfficiency          bool     `json:"powerEfficiency"`
	LookAhead                bool     `json:"lookAhead"`
	LookAheadDepth           int      `json:"lookAheadDepth"`
	LowPower                 bool     `json:"lowPower"`
	ExtendedBRC              bool     `json:"extendedBRC"`
	AdaptiveI                bool     `json:"adaptiveI"`
	AdaptiveB                bool     `json:"adaptiveB"`
	EstimatedVideoBitrate    *int64   `json:"estimatedVideoBitrate,omitempty"`
	EstimatedVideoBitrateMin *int64   `json:"estimatedVideoBitrateMin,omitempty"`
	EstimatedVideoBitrateMax *int64   `json:"estimatedVideoBitrateMax,omitempty"`
	EstimatedOutputSize      *int64   `json:"estimatedOutputSize,omitempty"`
	EstimatedOutputSizeMin   *int64   `json:"estimatedOutputSizeMin,omitempty"`
	EstimatedOutputSizeMax   *int64   `json:"estimatedOutputSizeMax,omitempty"`
	EstimateConfidence       string   `json:"estimateConfidence"`
	Warnings                 []string `json:"warnings"`
}

type Translator interface {
	Translate(QualityIntent, WorkerCapabilities) (EncoderRecommendation, error)
}
