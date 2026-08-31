package handlers

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
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

// resolveUpscaleProfile converts profile intent into an asset-specific
// geometry decision. It never renders filters. A decision already present in
// the effective profile is immutable so Queue workers consume the plan frozen
// at enqueue time instead of re-resolving it.
func resolveUpscaleProfile(profile models.Profile, streams MediaStreamInventory, evidence UpscaleAnalysisEvidence) models.Profile {
	profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
	request, err := parseUpscaleRequest(profile.WorkerConfig)
	if err != nil {
		return profile
	}
	var source MediaStream
	if len(streams.Video) > 0 {
		source = streams.Video[0]
	}
	geometry, geometryOK := effectiveUpscaleGeometry(profile, source)
	if strings.EqualFold(strings.TrimSpace(profile.VideoCodec), "copy") {
		decision := ResolvedUpscaleDecision{
			RequestedMode: request.Mode,
			ResolvedMode:  ResolvedUpscaleKeepSource,
			SourceWidth:   geometry.Width,
			SourceHeight:  geometry.Height,
			SourceSAR:     geometry.SAR,
			SourceDAR:     geometry.DAR,
			TargetWidth:   geometry.Width,
			TargetHeight:  geometry.Height,
			TargetSAR:     geometry.SAR,
			SharpenMode:   UpscaleSharpenOff,
			Confidence:    confidenceForGeometry(geometryOK),
			Reasons:       []string{"keep_source_video_copy"},
			Warnings:      []string{"Smart Upscale requires video re-encoding; Video Codec is configured as Copy."},
		}
		storeResolvedUpscaleDecision(profile.WorkerConfig, decision)
		return profile
	}
	if frozen, ok := resolvedUpscaleDecisionFromProfile(profile); ok {
		applyResolvedUpscaleGeometry(profile.WorkerConfig, *frozen)
		return profile
	}
	decision := ResolvedUpscaleDecision{
		RequestedMode: request.Mode,
		ResolvedMode:  ResolvedUpscaleKeepSource,
		SourceWidth:   geometry.Width,
		SourceHeight:  geometry.Height,
		SourceSAR:     geometry.SAR,
		SourceDAR:     geometry.DAR,
		TargetWidth:   geometry.Width,
		TargetHeight:  geometry.Height,
		TargetSAR:     geometry.SAR,
		SharpenMode:   UpscaleSharpenOff,
		Confidence:    UpscaleConfidenceUnavailable,
		Reasons:       []string{},
		Warnings:      []string{},
	}

	if request.Mode == UpscaleModeDisabled {
		if len(streams.Video) > 0 {
			if width, height, ok := resolveEffectiveVideoGeometry(profile, source); ok {
				decision.SourceWidth = source.Width
				decision.SourceHeight = source.Height
				decision.TargetWidth = width
				decision.TargetHeight = height
				geometryOK = true
			}
		}
		decision.Confidence = confidenceForGeometry(geometryOK)
		decision.Reasons = append(decision.Reasons, "upscale_disabled")
		storeResolvedUpscaleDecision(profile.WorkerConfig, decision)
		return profile
	}
	if !geometryOK {
		decision.Reasons = append(decision.Reasons, "keep_source_geometry_unresolved")
		decision.Warnings = append(decision.Warnings, "Upscale was not applied because effective post-crop SAR/DAR geometry could not be resolved safely.")
		storeResolvedUpscaleDecision(profile.WorkerConfig, decision)
		return profile
	}

	targetHeight := 0
	resolvedMode := ResolvedUpscaleKeepSource
	switch request.Mode {
	case UpscaleModeAuto:
		if geometry.Height > 576 {
			decision.Confidence = UpscaleConfidenceHigh
			decision.Reasons = append(decision.Reasons, "keep_source_above_sd")
			storeResolvedUpscaleDecision(profile.WorkerConfig, decision)
			return profile
		}
		if !autoUpscaleEvidenceReliable(profile, evidence) {
			decision.Confidence = UpscaleConfidenceLow
			decision.Reasons = append(decision.Reasons, "keep_source_evidence_insufficient")
			decision.Warnings = append(decision.Warnings, "Auto kept the source geometry because progressive output or geometry/frame evidence is insufficient.")
			storeResolvedUpscaleDecision(profile.WorkerConfig, decision)
			return profile
		}
		targetHeight, resolvedMode = 720, ResolvedUpscale720p
		decision.Confidence = UpscaleConfidenceHigh
		decision.Reasons = append(decision.Reasons, "reliable_sd_progressive_output", "conservative_720p_target")
	case UpscaleMode720p:
		targetHeight, resolvedMode = 720, ResolvedUpscale720p
		decision.Confidence = UpscaleConfidenceMedium
	case UpscaleMode1080p:
		targetHeight, resolvedMode = 1080, ResolvedUpscale1080p
		decision.Confidence = UpscaleConfidenceMedium
	case UpscaleModeCustom:
		targetHeight, resolvedMode = request.CustomHeight, ResolvedUpscaleCustom
		decision.Confidence = UpscaleConfidenceMedium
	}
	if targetHeight <= geometry.Height {
		decision.Reasons = append(decision.Reasons, "keep_source_target_is_not_an_upscale")
		decision.Warnings = append(decision.Warnings, fmt.Sprintf("Requested target height %d is not above the effective source height %d; Smart Upscale does not perform downscaling.", targetHeight, geometry.Height))
		storeResolvedUpscaleDecision(profile.WorkerConfig, decision)
		return profile
	}

	decision.ResolvedMode = resolvedMode
	decision.TargetHeight = targetHeight
	decision.TargetWidth = encoderSafeEvenDimension(float64(targetHeight) * geometry.DARRatio)
	decision.TargetSAR = "1:1"
	decision.UpscaleApplied = true
	decision.SharpenMode = request.Sharpen
	decision.Reasons = append(decision.Reasons, "target_width_derived_from_effective_dar", "square_pixel_output")
	storeResolvedUpscaleDecision(profile.WorkerConfig, decision)
	return profile
}

type upscaleGeometry struct {
	Width    int
	Height   int
	SAR      string
	DAR      string
	DARRatio float64
}

func effectiveUpscaleGeometry(profile models.Profile, source MediaStream) (upscaleGeometry, bool) {
	if source.Width <= 0 || source.Height <= 0 {
		return upscaleGeometry{}, false
	}
	sarNum, sarDen, sarOK := parsePositiveRatio(source.SampleAspectRatio)
	darNum, darDen, darOK := parsePositiveRatio(source.DisplayAspectRatio)
	if !sarOK && darOK {
		sarNum, sarDen = darNum*source.Height, darDen*source.Width
		sarNum, sarDen = reduceRatio(sarNum, sarDen)
		sarOK = true
	}
	if !sarOK {
		return upscaleGeometry{Width: source.Width, Height: source.Height}, false
	}

	width, height := source.Width, source.Height
	for _, raw := range strings.Split(workerStringValue(profile.WorkerConfig["videoFilters"]), ",") {
		filter := strings.TrimSpace(raw)
		if filter == "" {
			continue
		}
		name, value, found := strings.Cut(filter, "=")
		filterName := strings.ToLower(strings.TrimSpace(name))
		if !found {
			filterName = strings.ToLower(filter)
		}
		switch filterName {
		case "crop":
			values := strings.Split(value, ":")
			if len(values) < 2 {
				return upscaleGeometry{}, false
			}
			cropWidth, widthErr := strconv.Atoi(strings.TrimSpace(values[0]))
			cropHeight, heightErr := strconv.Atoi(strings.TrimSpace(values[1]))
			if widthErr != nil || heightErr != nil || cropWidth <= 0 || cropHeight <= 0 || cropWidth > width || cropHeight > height {
				return upscaleGeometry{}, false
			}
			width, height = cropWidth, cropHeight
		case "scale":
			values := strings.Split(value, ":")
			if len(values) < 2 {
				return upscaleGeometry{}, false
			}
			scaleWidth, widthErr := strconv.Atoi(strings.Trim(strings.TrimSpace(values[0]), "'\""))
			scaleHeight, heightErr := strconv.Atoi(strings.Trim(strings.TrimSpace(values[1]), "'\""))
			if widthErr != nil || heightErr != nil || scaleWidth == 0 || scaleHeight == 0 {
				return upscaleGeometry{}, false
			}
			switch {
			case scaleWidth > 0 && scaleHeight > 0:
				width, height = scaleWidth, scaleHeight
			case scaleWidth > 0 && (scaleHeight == -1 || scaleHeight == -2):
				height = scaledDimension(height, width, scaleWidth, scaleHeight == -2)
				width = scaleWidth
			case scaleHeight > 0 && (scaleWidth == -1 || scaleWidth == -2):
				width = scaledDimension(width, height, scaleHeight, scaleWidth == -2)
				height = scaleHeight
			default:
				return upscaleGeometry{}, false
			}
		case "setsar":
			if nextNum, nextDen, ok := parsePositiveRatio(value); ok {
				sarNum, sarDen = nextNum, nextDen
			} else {
				return upscaleGeometry{}, false
			}
		case "setdar":
			if nextNum, nextDen, ok := parsePositiveRatio(value); ok {
				sarNum, sarDen = nextNum*height, nextDen*width
				sarNum, sarDen = reduceRatio(sarNum, sarDen)
			} else {
				return upscaleGeometry{}, false
			}
		case "fps", "bwdif", "yadif", "fieldmatch", "decimate", "eq", "hqdn3d", "unsharp", "format", "colorspace", "setfield", "null":
		default:
			return upscaleGeometry{}, false
		}
	}
	darNum, darDen = reduceRatio(width*sarNum, height*sarDen)
	return upscaleGeometry{
		Width: width, Height: height,
		SAR: fmt.Sprintf("%d:%d", sarNum, sarDen), DAR: fmt.Sprintf("%d:%d", darNum, darDen),
		DARRatio: float64(darNum) / float64(darDen),
	}, true
}

func autoUpscaleEvidenceReliable(profile models.Profile, evidence UpscaleAnalysisEvidence) bool {
	progressive, progressiveKnown := profile.WorkerConfig["effectiveOutputProgressive"].(bool)
	if !progressiveKnown || !progressive {
		return false
	}
	if profileWorkerBool(profile, "effectiveOutputProgressiveUnreliable", false) {
		return false
	}
	if strings.Contains(strings.ToLower(workerStringValue(profile.WorkerConfig["videoFilters"])), "crop=") {
		if !evidence.CropAvailable || evidence.CropConfidence < .8 {
			return false
		}
	}
	if normalizedFrameStructureMode(workerStringValue(profile.WorkerConfig["frameStructureMode"])) == "auto" &&
		!profileWorkerBool(profile, "frameStructureAutoResolved", false) && !evidence.FrameStructureAvailable {
		return false
	}
	return true
}

func storeResolvedUpscaleDecision(config models.JSONMap, decision ResolvedUpscaleDecision) {
	config["resolvedUpscaleDecision"] = decision
	applyResolvedUpscaleGeometry(config, decision)
}

func applyResolvedUpscaleGeometry(config models.JSONMap, decision ResolvedUpscaleDecision) {
	if decision.TargetWidth > 0 && decision.TargetHeight > 0 {
		config["effectiveOutputWidth"] = decision.TargetWidth
		config["effectiveOutputHeight"] = decision.TargetHeight
		delete(config, "effectiveOutputGeometryUnknown")
	}
}

func confidenceForGeometry(ok bool) UpscaleConfidence {
	if ok {
		return UpscaleConfidenceHigh
	}
	return UpscaleConfidenceUnavailable
}

func parsePositiveRatio(value string) (int, int, bool) {
	value = strings.TrimSpace(strings.Trim(value, "'\""))
	value = strings.ReplaceAll(value, "/", ":")
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, 0, false
	}
	numerator, numeratorErr := strconv.Atoi(strings.TrimSpace(parts[0]))
	denominator, denominatorErr := strconv.Atoi(strings.TrimSpace(parts[1]))
	if numeratorErr != nil || denominatorErr != nil || numerator <= 0 || denominator <= 0 {
		return 0, 0, false
	}
	numerator, denominator = reduceRatio(numerator, denominator)
	return numerator, denominator, true
}

func reduceRatio(numerator, denominator int) (int, int) {
	common := numerator
	other := denominator
	for other != 0 {
		common, other = other, common%other
	}
	if common <= 0 {
		return numerator, denominator
	}
	return numerator / common, denominator / common
}

func encoderSafeEvenDimension(value float64) int {
	return max(2, int(math.Round(value/2))*2)
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

func normalizedUpscaleMode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch UpscaleMode(value) {
	case UpscaleModeDisabled, UpscaleModeAuto, UpscaleMode720p, UpscaleMode1080p, UpscaleModeCustom:
		return value
	default:
		return ""
	}
}

func normalizedUpscaleSharpen(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch UpscaleSharpen(value) {
	case UpscaleSharpenOff, UpscaleSharpenLight, UpscaleSharpenMedium:
		return value
	default:
		return ""
	}
}

func normalizedUpscaleCustomHeight(mode string, height int) int {
	if normalizedUpscaleMode(mode) == string(UpscaleModeCustom) && height >= 2 && height%2 == 0 {
		return height
	}
	return 0
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

func upscaleAnalysisEvidenceForPath(db *gorm.DB, path string) UpscaleAnalysisEvidence {
	if db == nil || strings.TrimSpace(path) == "" || !db.Migrator().HasTable(&models.ScanResult{}) {
		return UpscaleAnalysisEvidence{}
	}
	var scan models.ScanResult
	if err := db.Where("path = ?", path).Order("updated_at desc, id desc").First(&scan).Error; err != nil {
		return UpscaleAnalysisEvidence{}
	}
	return upscaleAnalysisEvidence(scan)
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
		if value := strings.TrimSpace(stringFromUnknown(values[key])); value != "" {
			return value
		}
	}
	return ""
}
