package quality

import (
	"math"
	"strconv"
	"strings"
)

const (
	BPPPFPolicyVersion        = 1
	BPPPFEncoderModelVersion  = 1
	BPPPFStatusNormal         = "normal"
	BPPPFStatusWarning        = "warning"
	BPPPFStatusHighRisk       = "high-risk"
	BPPPFStatusUnavailable    = "unavailable"
	BPPPFEncoderClassSoftware = "software"
	BPPPFEncoderClassHardware = "hardware"
)

type BPPPFInput struct {
	TargetVideoBitrate int64
	BitrateBasis       string
	OutputWidth        int
	OutputHeight       int
	EffectiveFPS       float64
	CodecFamily        string
	Encoder            string
	HDR                bool
	OptimizationIntent string
}

type BPPPFAssessment struct {
	PolicyVersion       int     `json:"policyVersion"`
	EncoderModelVersion int     `json:"encoderModelVersion"`
	Status              string  `json:"status"`
	BPPPF               float64 `json:"bpppf,omitempty"`
	ReferenceBPPPF      float64 `json:"referenceBpppf,omitempty"`
	ReferenceBitrate    int64   `json:"referenceBitrate,omitempty"`
	TargetBitrate       int64   `json:"targetBitrate,omitempty"`
	BitrateBasis        string  `json:"bitrateBasis,omitempty"`
	OutputWidth         int     `json:"outputWidth,omitempty"`
	OutputHeight        int     `json:"outputHeight,omitempty"`
	EffectiveFPS        float64 `json:"effectiveFps,omitempty"`
	CodecFamily         string  `json:"codecFamily,omitempty"`
	Encoder             string  `json:"encoder,omitempty"`
	EncoderClass        string  `json:"encoderClass,omitempty"`
	DynamicRange        string  `json:"dynamicRange,omitempty"`
	OptimizationIntent  string  `json:"optimizationIntent,omitempty"`
	Explanation         string  `json:"explanation"`
	HeuristicOnly       bool    `json:"heuristicOnly"`
}

// EvaluateBPPPF applies versioned planning references. These values are
// review rails only and do not describe universal codec efficiency ratios.
func EvaluateBPPPF(input BPPPFInput) BPPPFAssessment {
	assessment := BPPPFAssessment{
		PolicyVersion: BPPPFPolicyVersion, EncoderModelVersion: BPPPFEncoderModelVersion,
		Status: BPPPFStatusUnavailable, TargetBitrate: input.TargetVideoBitrate,
		BitrateBasis: strings.TrimSpace(input.BitrateBasis),
		OutputWidth: input.OutputWidth, OutputHeight: input.OutputHeight,
		EffectiveFPS: input.EffectiveFPS, Encoder: strings.ToLower(strings.TrimSpace(input.Encoder)),
		CodecFamily: NormalizeCodecFamily(input.CodecFamily), HeuristicOnly: true,
	}
	if input.HDR {
		assessment.DynamicRange = "hdr"
	} else {
		assessment.DynamicRange = "sdr"
	}
	assessment.OptimizationIntent = normalizeOptimizationIntent(input.OptimizationIntent)
	assessment.EncoderClass = EncoderEfficiencyClass(input.Encoder)
	if input.TargetVideoBitrate <= 0 || input.OutputWidth <= 0 || input.OutputHeight <= 0 ||
		input.EffectiveFPS <= 0 || math.IsNaN(input.EffectiveFPS) || math.IsInf(input.EffectiveFPS, 0) ||
		assessment.CodecFamily == "" || assessment.EncoderClass == "" {
		assessment.Explanation = "BPPPF planning rail unavailable because target bitrate, effective output geometry/FPS, or a supported codec/encoder class is unknown."
		return assessment
	}
	if input.OutputWidth > math.MaxInt/input.OutputHeight {
		assessment.Explanation = "BPPPF planning rail unavailable because output geometry is outside supported bounds."
		return assessment
	}
	pixelsPerFrame := float64(input.OutputWidth) * float64(input.OutputHeight)
	assessment.BPPPF = float64(input.TargetVideoBitrate) / (pixelsPerFrame * input.EffectiveFPS)
	baseReference, ok := codecBPPPFReferences[assessment.CodecFamily]
	if !ok {
		assessment.Explanation = "BPPPF planning rail unavailable because the target codec has no versioned reference."
		return assessment
	}
	reference := baseReference
	reference *= encoderClassBPPPFFactor[assessment.EncoderClass]
	if input.HDR {
		reference *= 1.15
	}
	reference *= optimizationBPPPFFactor[assessment.OptimizationIntent]
	assessment.ReferenceBPPPF = reference
	assessment.ReferenceBitrate = int64(math.Ceil(reference * pixelsPerFrame * input.EffectiveFPS))
	switch {
	case assessment.BPPPF >= reference:
		assessment.Status = BPPPFStatusNormal
	case assessment.BPPPF >= reference*0.75:
		assessment.Status = BPPPFStatusWarning
	default:
		assessment.Status = BPPPFStatusHighRisk
	}
	assessment.Explanation = bpppfExplanation(assessment)
	return assessment
}

var codecBPPPFReferences = map[string]float64{
	"h264": 0.065,
	"hevc": 0.045,
	"av1":  0.035,
}

var encoderClassBPPPFFactor = map[string]float64{
	BPPPFEncoderClassSoftware: 1.0,
	BPPPFEncoderClassHardware: 1.15,
}

var optimizationBPPPFFactor = map[string]float64{
	"maximum_savings": 0.85,
	"balanced":        1.0,
	"conservative":    1.08,
	"maximum_quality": 1.18,
	"archive":         1.25,
}

func NormalizeCodecFamily(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(normalized, "av1"):
		return "av1"
	case strings.Contains(normalized, "hevc"), strings.Contains(normalized, "h265"), strings.Contains(normalized, "x265"):
		return "hevc"
	case strings.Contains(normalized, "h264"), strings.Contains(normalized, "avc"), strings.Contains(normalized, "x264"):
		return "h264"
	default:
		return ""
	}
}

func EncoderEfficiencyClass(value string) string {
	encoder := strings.ToLower(strings.TrimSpace(value))
	if encoder == "" {
		return ""
	}
	for _, suffix := range []string{"_qsv", "_nvenc", "_videotoolbox", "_vaapi", "_amf"} {
		if strings.HasSuffix(encoder, suffix) {
			return BPPPFEncoderClassHardware
		}
	}
	if strings.HasPrefix(encoder, "libx264") || strings.HasPrefix(encoder, "libx265") ||
		strings.HasPrefix(encoder, "libsvtav1") || strings.HasPrefix(encoder, "libaom-av1") ||
		strings.HasPrefix(encoder, "librav1e") || strings.HasPrefix(encoder, "libvpx") {
		return BPPPFEncoderClassSoftware
	}
	return ""
}

func ParseFrameRate(value string) float64 {
	value = strings.TrimSpace(value)
	if value == "" || value == "0/0" || value == "N/A" {
		return 0
	}
	parts := strings.Split(value, "/")
	if len(parts) == 2 {
		numerator, numeratorErr := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		denominator, denominatorErr := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if numeratorErr != nil || denominatorErr != nil || denominator <= 0 {
			return 0
		}
		value = strconv.FormatFloat(numerator/denominator, 'f', -1, 64)
	}
	rate, err := strconv.ParseFloat(value, 64)
	if err != nil || rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 0
	}
	return rate
}

func normalizeOptimizationIntent(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "maximum_savings", "conservative", "maximum_quality", "archive":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "balanced"
	}
}

func bpppfExplanation(assessment BPPPFAssessment) string {
	basis := "Estimated target "
	if assessment.BitrateBasis == "configured_target" {
		basis = "Configured target "
	}
	return basis + formatMbps(assessment.TargetBitrate) + " is " + assessment.Status +
		" against the versioned " + assessment.CodecFamily + "/" + assessment.EncoderClass + " " +
		assessment.DynamicRange + " BPPPF reference for " + strconv.Itoa(assessment.OutputWidth) + "×" +
		strconv.Itoa(assessment.OutputHeight) + " at " + strconv.FormatFloat(assessment.EffectiveFPS, 'f', 3, 64) +
		" fps and the " + assessment.OptimizationIntent + " goal. This is heuristic planning context; measured LAB evidence remains authoritative."
}

func formatMbps(bitrate int64) string {
	return strconv.FormatFloat(float64(bitrate)/1_000_000, 'f', 2, 64) + " Mbps"
}
