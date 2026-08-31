package handlers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

type UpscaleMode string

const (
	UpscaleModeDisabled UpscaleMode = "disabled"
	UpscaleModeAuto     UpscaleMode = "auto"
	UpscaleMode720p     UpscaleMode = "720p"
	UpscaleMode1080p    UpscaleMode = "1080p"
	UpscaleModeCustom   UpscaleMode = "custom"
)

type UpscaleSharpen string

const (
	UpscaleSharpenOff    UpscaleSharpen = "off"
	UpscaleSharpenLight  UpscaleSharpen = "light"
	UpscaleSharpenMedium UpscaleSharpen = "medium"
)

type ResolvedUpscaleMode string

const (
	ResolvedUpscaleKeepSource ResolvedUpscaleMode = "keep_source"
	ResolvedUpscale720p       ResolvedUpscaleMode = "720p"
	ResolvedUpscale1080p      ResolvedUpscaleMode = "1080p"
	ResolvedUpscaleCustom     ResolvedUpscaleMode = "custom"
)

type UpscaleConfidence string

const (
	UpscaleConfidenceUnavailable UpscaleConfidence = "unavailable"
	UpscaleConfidenceLow         UpscaleConfidence = "low"
	UpscaleConfidenceMedium      UpscaleConfidence = "medium"
	UpscaleConfidenceHigh        UpscaleConfidence = "high"
)

// UpscaleRequest is profile intent only. Custom stores a target height so the
// future geometry resolver can derive width after crop and SAR/DAR handling.
type UpscaleRequest struct {
	Mode         UpscaleMode    `json:"upscaleMode"`
	Sharpen      UpscaleSharpen `json:"upscaleSharpen"`
	CustomHeight int            `json:"upscaleCustomHeight,omitempty"`
}

type ResolvedUpscaleDecision struct {
	RequestedMode  UpscaleMode         `json:"requestedMode"`
	ResolvedMode   ResolvedUpscaleMode `json:"resolvedMode"`
	SourceWidth    int                 `json:"sourceWidth"`
	SourceHeight   int                 `json:"sourceHeight"`
	SourceSAR      string              `json:"sourceSAR,omitempty"`
	SourceDAR      string              `json:"sourceDAR,omitempty"`
	TargetWidth    int                 `json:"targetWidth"`
	TargetHeight   int                 `json:"targetHeight"`
	TargetSAR      string              `json:"targetSAR,omitempty"`
	UpscaleApplied bool                `json:"upscaleApplied"`
	SharpenMode    UpscaleSharpen      `json:"sharpenMode"`
	Confidence     UpscaleConfidence   `json:"confidence"`
	Reasons        []string            `json:"reasons"`
	Warnings       []string            `json:"warnings"`
}

type UpscaleSignalAvailability struct {
	Noise       string `json:"noise"`
	Grain       string `json:"grain"`
	Compression string `json:"compressionArtifacts"`
	Ringing     string `json:"ringing"`
	EdgeDetail  string `json:"edgeDetail"`
}

type UpscaleAnalysisEvidence struct {
	SourceWidth             int                       `json:"sourceWidth"`
	SourceHeight            int                       `json:"sourceHeight"`
	SourceSAR               string                    `json:"sourceSAR,omitempty"`
	SourceDAR               string                    `json:"sourceDAR,omitempty"`
	Codec                   string                    `json:"codec,omitempty"`
	Bitrate                 int64                     `json:"bitrate,omitempty"`
	CropAvailable           bool                      `json:"cropAvailable"`
	CropStatus              string                    `json:"cropStatus,omitempty"`
	CropConfidence          float64                   `json:"cropConfidence,omitempty"`
	FrameStructureAvailable bool                      `json:"frameStructureAvailable"`
	InterlaceAvailable      bool                      `json:"interlaceAvailable"`
	InterlaceStatus         string                    `json:"interlaceStatus,omitempty"`
	InterlaceConfidence     float64                   `json:"interlaceConfidence,omitempty"`
	CadenceAvailable        bool                      `json:"cadenceAvailable"`
	CadenceType             string                    `json:"cadenceType,omitempty"`
	CadenceConfidence       float64                   `json:"cadenceConfidence,omitempty"`
	CadenceRecommendation   string                    `json:"cadenceRecommendation,omitempty"`
	EffectiveFrameRate      string                    `json:"effectiveFrameRate,omitempty"`
	QualitySignals          UpscaleSignalAvailability `json:"qualitySignals"`
}

func parseUpscaleRequest(config models.JSONMap) (UpscaleRequest, error) {
	request := UpscaleRequest{
		Mode:    UpscaleMode(strings.ToLower(strings.TrimSpace(stringFromUnknown(config["upscaleMode"])))),
		Sharpen: UpscaleSharpen(strings.ToLower(strings.TrimSpace(stringFromUnknown(config["upscaleSharpen"])))),
	}
	if request.Mode == "" {
		request.Mode = UpscaleModeDisabled
	}
	if request.Sharpen == "" {
		request.Sharpen = UpscaleSharpenOff
	}
	switch request.Mode {
	case UpscaleModeDisabled, UpscaleModeAuto, UpscaleMode720p, UpscaleMode1080p, UpscaleModeCustom:
	default:
		return UpscaleRequest{}, fmt.Errorf("upscaleMode must be disabled, auto, 720p, 1080p, or custom")
	}
	switch request.Sharpen {
	case UpscaleSharpenOff, UpscaleSharpenLight, UpscaleSharpenMedium:
	default:
		return UpscaleRequest{}, fmt.Errorf("upscaleSharpen must be off, light, or medium")
	}
	request.CustomHeight = intValueSetting(config["upscaleCustomHeight"], 0)
	if request.Mode == UpscaleModeCustom {
		if request.CustomHeight < 2 || request.CustomHeight%2 != 0 {
			return UpscaleRequest{}, fmt.Errorf("upscaleCustomHeight must be a positive even height for custom upscale")
		}
	}
	return request, nil
}

func resolvedUpscaleDecisionFromProfile(profile models.Profile) (*ResolvedUpscaleDecision, bool) {
	raw, ok := profile.WorkerConfig["resolvedUpscaleDecision"]
	if !ok || raw == nil {
		return nil, false
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	var decision ResolvedUpscaleDecision
	if err := json.Unmarshal(encoded, &decision); err != nil {
		return nil, false
	}
	return &decision, true
}

func upscaleAnalysisEvidence(scan models.ScanResult) UpscaleAnalysisEvidence {
	evidence := UpscaleAnalysisEvidence{
		SourceWidth: scan.Width, SourceHeight: scan.Height, Codec: scan.VideoCodec, Bitrate: scan.Bitrate,
		CropAvailable: len(scan.CropAnalysis) > 0, CropStatus: stringFromUnknown(scan.CropAnalysis["status"]),
		CropConfidence:          workerNumberValue(scan.CropAnalysis["confidence"], 0),
		FrameStructureAvailable: len(scan.FrameStructureAnalysis) > 0,
		InterlaceAvailable:      len(scan.InterlaceAnalysis) > 0, InterlaceStatus: stringFromUnknown(scan.InterlaceAnalysis["status"]),
		InterlaceConfidence: workerNumberValue(scan.InterlaceAnalysis["confidence"], 0),
		CadenceAvailable:    len(scan.CadenceAnalysis) > 0, CadenceType: stringFromUnknown(scan.CadenceAnalysis["type"]),
		CadenceConfidence:     workerNumberValue(scan.CadenceAnalysis["confidence"], 0),
		CadenceRecommendation: stringFromUnknown(scan.CadenceRecommendation["operation"]),
		EffectiveFrameRate:    stringFromUnknown(scan.CadenceRecommendation["outputFrameRate"]),
		QualitySignals: UpscaleSignalAvailability{
			Noise: "unavailable", Grain: "unavailable", Compression: "unavailable", Ringing: "unavailable", EdgeDetail: "unavailable",
		},
	}
	if len(scan.VideoStreams) > 0 {
		if stream, ok := upscaleStreamMap(scan.VideoStreams[0]); ok {
			evidence.SourceSAR = firstNonEmptyString(stream, "sampleAspectRatio", "sample_aspect_ratio")
			evidence.SourceDAR = firstNonEmptyString(stream, "displayAspectRatio", "display_aspect_ratio")
		}
	}
	if evidence.EffectiveFrameRate == "" {
		evidence.EffectiveFrameRate = stringFromUnknown(scan.CadenceAnalysis["effectivePictureRate"])
	}
	return evidence
}

func upscaleStreamMap(value interface{}) (map[string]interface{}, bool) {
	switch typed := value.(type) {
	case map[string]interface{}:
		return typed, true
	case models.JSONMap:
		return map[string]interface{}(typed), true
	default:
		return nil, false
	}
}

func firstNonEmptyString(values map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(stringFromUnknown(values[k