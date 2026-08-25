package handlers

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

type hevcLevelLimit struct {
	Level          string
	LevelIDC       int
	MaxLumaPicture int64
	MaxLumaRate    float64
	MaxBitrateKbps int64
}

// Main-tier limits from H.265 Annex A. Level recommendation intentionally uses
// Main tier because it is the more broadly compatible playback target. The
// produced bitstream is still probed and validated after encoding.
var hevcMainTierLevelLimits = []hevcLevelLimit{
	{Level: "1.0", LevelIDC: 30, MaxLumaPicture: 36_864, MaxLumaRate: 552_960, MaxBitrateKbps: 128},
	{Level: "2.0", LevelIDC: 60, MaxLumaPicture: 122_880, MaxLumaRate: 3_686_400, MaxBitrateKbps: 1_500},
	{Level: "2.1", LevelIDC: 63, MaxLumaPicture: 245_760, MaxLumaRate: 7_372_800, MaxBitrateKbps: 3_000},
	{Level: "3.0", LevelIDC: 90, MaxLumaPicture: 552_960, MaxLumaRate: 16_588_800, MaxBitrateKbps: 6_000},
	{Level: "3.1", LevelIDC: 93, MaxLumaPicture: 983_040, MaxLumaRate: 33_177_600, MaxBitrateKbps: 10_000},
	{Level: "4.0", LevelIDC: 120, MaxLumaPicture: 2_228_224, MaxLumaRate: 66_846_720, MaxBitrateKbps: 12_000},
	{Level: "4.1", LevelIDC: 123, MaxLumaPicture: 2_228_224, MaxLumaRate: 133_693_440, MaxBitrateKbps: 20_000},
	{Level: "5.0", LevelIDC: 150, MaxLumaPicture: 8_912_896, MaxLumaRate: 267_386_880, MaxBitrateKbps: 25_000},
	{Level: "5.1", LevelIDC: 153, MaxLumaPicture: 8_912_896, MaxLumaRate: 534_773_760, MaxBitrateKbps: 40_000},
	{Level: "5.2", LevelIDC: 156, MaxLumaPicture: 8_912_896, MaxLumaRate: 1_069_547_520, MaxBitrateKbps: 60_000},
	{Level: "6.0", LevelIDC: 180, MaxLumaPicture: 35_651_584, MaxLumaRate: 1_069_547_520, MaxBitrateKbps: 60_000},
	{Level: "6.1", LevelIDC: 183, MaxLumaPicture: 35_651_584, MaxLumaRate: 2_139_095_040, MaxBitrateKbps: 120_000},
	{Level: "6.2", LevelIDC: 186, MaxLumaPicture: 35_651_584, MaxLumaRate: 4_278_190_080, MaxBitrateKbps: 240_000},
}

type HEVCLevelRecommendation struct {
	Version          int      `json:"version"`
	RecommendedLevel string   `json:"recommendedLevel,omitempty"`
	LevelIDC         int      `json:"levelIdc,omitempty"`
	Tier             string   `json:"tier"`
	Width            int      `json:"width"`
	Height           int      `json:"height"`
	FPS              float64  `json:"fps"`
	Bitrate          int64    `json:"bitrate,omitempty"`
	LumaPictureSize  int64    `json:"lumaPictureSize"`
	LumaSampleRate   float64  `json:"lumaSampleRate"`
	SourceLevel      string   `json:"sourceLevel,omitempty"`
	SourceLevelIDC   int      `json:"sourceLevelIdc,omitempty"`
	Confidence       string   `json:"confidence"`
	LimitingFactor   string   `json:"limitingFactor,omitempty"`
	Reasons          []string `json:"reasons"`
	Warnings         []string `json:"warnings,omitempty"`
}

func recommendHEVCLevel(width, height int, fps float64, bitrate int64) HEVCLevelRecommendation {
	result := HEVCLevelRecommendation{
		Version: 1, Tier: "main", Width: width, Height: height, FPS: fps,
		Bitrate: bitrate, Confidence: "high", Reasons: []string{}, Warnings: []string{},
	}
	if width <= 0 || height <= 0 || fps <= 0 {
		result.Confidence = "low"
		result.Warnings = append(result.Warnings, "Resolution and a reliable frame rate are required before MVForge can calculate an HEVC Level recommendation.")
		return result
	}
	result.LumaPictureSize = int64(width) * int64(height)
	result.LumaSampleRate = float64(result.LumaPictureSize) * fps
	bitrateKbps := int64(0)
	if bitrate > 0 {
		bitrateKbps = int64(math.Ceil(float64(bitrate) / 1000))
	} else {
		result.Confidence = "medium"
		result.Warnings = append(result.Warnings, "Video bitrate was unavailable; the recommendation covers picture size and sample rate, and the output bitrate must be verified after encoding.")
	}

	for _, limit := range hevcMainTierLevelLimits {
		pictureFits := result.LumaPictureSize <= limit.MaxLumaPicture
		rateFits := result.LumaSampleRate <= limit.MaxLumaRate
		bitrateFits := bitrateKbps == 0 || bitrateKbps <= limit.MaxBitrateKbps
		if !pictureFits || !rateFits || !bitrateFits {
			continue
		}
		result.RecommendedLevel = limit.Level
		result.LevelIDC = limit.LevelIDC
		result.LimitingFactor = hevcLimitingFactor(result, bitrateKbps, limit)
		result.Reasons = append(result.Reasons,
			fmt.Sprintf("%dx%d at %.3f fps produces %.0f luma samples/s; HEVC Level %s Main Tier allows up to %.0f.", width, height, fps, result.LumaSampleRate, limit.Level, limit.MaxLumaRate),
		)
		if bitrateKbps > 0 {
			result.Reasons = append(result.Reasons, fmt.Sprintf("Observed video bitrate %.2f Mbps is within the %.2f Mbps Main Tier limit.", float64(bitrateKbps)/1000, float64(limit.MaxBitrateKbps)/1000))
		}
		return result
	}

	result.Confidence = "low"
	result.Warnings = append(result.Warnings, "The asset exceeds the supported HEVC Main Tier Level 6.2 limits; lower its resolution, frame rate, or bitrate, or review a High Tier workflow.")
	return result
}

func hevcLimitingFactor(rec HEVCLevelRecommendation, bitrateKbps int64, limit hevcLevelLimit) string {
	ratios := map[string]float64{
		"picture_size": float64(rec.LumaPictureSize) / float64(limit.MaxLumaPicture),
		"sample_rate":  rec.LumaSampleRate / limit.MaxLumaRate,
	}
	if bitrateKbps > 0 {
		ratios["bitrate"] = float64(bitrateKbps) / float64(limit.MaxBitrateKbps)
	}
	best := "picture_size"
	for name, ratio := range ratios {
		if ratio > ratios[best] {
			best = name
		}
	}
	return best
}

func buildHEVCLevelRecommendation(scan models.ScanResult) HEVCLevelRecommendation {
	// Source bitrate is not an output constraint for CRF/ICQ encodes. Using it
	// here can incorrectly promote high-bitrate 1080p sources to Level 5.x.
	result := recommendHEVCLevel(scan.Width, scan.Height, scanFrameRate(scan), 0)
	result.SourceLevelIDC = scanObservedHEVCLevelIDC(scan)
	result.SourceLevel = hevcLevelFromIDC(result.SourceLevelIDC)
	return result
}

func scanObservedHEVCLevelIDC(scan models.ScanResult) int {
	if len(scan.VideoStreams) > 0 {
		if stream, ok := scan.VideoStreams[0].(map[string]interface{}); ok {
			if level := workerIntValue(stream["level"], 0); level > 0 {
				return level
			}
		}
	}
	if streams, ok := scan.RawProbe["streams"].([]interface{}); ok {
		for _, raw := range streams {
			stream, ok := raw.(map[string]interface{})
			if ok && strings.EqualFold(stringFromUnknown(stream["codec_type"]), "video") {
				return workerIntValue(stream["level"], 0)
			}
		}
	}
	return 0
}

func scanVideoBitrate(scan models.ScanResult) int64 {
	if len(scan.VideoStreams) > 0 {
		if stream, ok := scan.VideoStreams[0].(map[string]interface{}); ok {
			if bitrate := int64(workerNumberValue(stream["bitrate"], 0)); bitrate > 0 {
				return bitrate
			}
		}
	}
	if scan.Bitrate <= 0 {
		return 0
	}
	audioBitrate := int64(0)
	for _, raw := range scan.AudioStreams {
		if stream, ok := raw.(map[string]interface{}); ok {
			audioBitrate += int64(workerNumberValue(stream["bitrate"], 0))
		}
	}
	estimate := scan.Bitrate - audioBitrate - scan.Bitrate/100
	if estimate > 0 {
		return estimate
	}
	return scan.Bitrate
}

func hevcLevelRecommendationMap(scan models.ScanResult) models.JSONMap {
	encoded, _ := json.Marshal(buildHEVCLevelRecommendation(scan))
	value := models.JSONMap{}
	_ = json.Unmarshal(encoded, &value)
	return value
}

func ensureHEVCLevelRecommendation(scan *models.ScanResult) bool {
	if scan == nil || workerIntValue(scan.HEVCLevelRecommendation["version"], 0) >= 1 {
		return false
	}
	scan.HEVCLevelRecommendation = hevcLevelRecommendationMap(*scan)
	return true
}

func normalizedHEVCLevelMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "auto", "recommended", "custom":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizedHEVCLevel(value string) string {
	value = strings.TrimSpace(value)
	for _, limit := range hevcMainTierLevelLimits {
		if value == limit.Level || (strings.HasSuffix(limit.Level, ".0") && value == strings.TrimSuffix(limit.Level, ".0")) {
			return limit.Level
		}
	}
	return ""
}

func hevcLevelFromIDC(levelIDC int) string {
	for _, limit := range hevcMainTierLevelLimits {
		if levelIDC == limit.LevelIDC {
			return limit.Level
		}
	}
	return ""
}

func hevcQSVLevelValue(level string) string {
	level = normalizedHEVCLevel(level)
	if level == "" {
		return ""
	}
	return strings.ReplaceAll(level, ".", "")
}

func hevcLevelLimitFor(level string) (hevcLevelLimit, bool) {
	level = normalizedHEVCLevel(level)
	for _, limit := range hevcMainTierLevelLimits {
		if limit.Level == level {
			return limit, true
		}
	}
	return hevcLevelLimit{}, false
}

func resolveHEVCLevel(profile models.Profile, streams MediaStreamInventory) models.Profile {
	if profile.VideoCodec == "copy" || videoCodecFamily(profile.VideoCodec) != "hevc" {
		return profile
	}
	encoder := resolvedVideoEncoder(profile)
	if encoder != "libx265" && encoder != "hevc_qsv" {
		return profile
	}
	mode := normalizedHEVCLevelMode(workerStringValue(profile.WorkerConfig["hevcLevelMode"]))
	if mode == "" {
		mode = "auto"
	}
	profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
	profile.WorkerConfig["hevcLevelMode"] = mode
	requested := normalizedHEVCLevel(workerStringValue(profile.WorkerConfig["hevcLevel"]))
	var source MediaStream
	if len(streams.Video) > 0 {
		source = streams.Video[0]
		width, height, geometryKnown := resolveEffectiveVideoGeometry(profile, source)
		if geometryKnown {
			profile.WorkerConfig["effectiveOutputWidth"] = width
			profile.WorkerConfig["effectiveOutputHeight"] = height
			delete(profile.WorkerConfig, "effectiveOutputGeometryUnknown")
		} else {
			delete(profile.WorkerConfig, "effectiveOutputWidth")
			delete(profile.WorkerConfig, "effectiveOutputHeight")
			profile.WorkerConfig["effectiveOutputGeometryUnknown"] = true
		}
	}
	if mode == "auto" {
		if len(streams.Video) == 0 {
			return profile
		}
		if profileWorkerBool(profile, "effectiveOutputFrameRateUnknown", false) || profileWorkerBool(profile, "effectiveOutputGeometryUnknown", false) {
			profile.WorkerConfig["hevcLevelResolutionWarning"] = "HEVC Level Auto is unresolved because effective output FPS or geometry is unknown"
			delete(profile.WorkerConfig, "hevcLevelEffective")
			return profile
		}
		// CRF and ICQ do not provide a target output bitrate up front. The source
		// bitrate must not be treated as an output requirement.
		fps := parseFrameRateValue(workerStringValue(profile.WorkerConfig["effectiveOutputFrameRate"]))
		if fps <= 0 {
			fps = parseFrameRateValue(source.FrameRate)
		}
		width := workerIntValue(profile.WorkerConfig["effectiveOutputWidth"], source.Width)
		height := workerIntValue(profile.WorkerConfig["effectiveOutputHeight"], source.Height)
		recommendation := recommendHEVCLevel(width, height, fps, 0)
		profile.WorkerConfig["hevcLevelRecommendation"] = recommendation
		requested = recommendation.RecommendedLevel
	} else if mode == "recommended" && requested == "" {
		if len(streams.Video) == 0 {
			return profile
		}
		if profileWorkerBool(profile, "effectiveOutputFrameRateUnknown", false) || profileWorkerBool(profile, "effectiveOutputGeometryUnknown", false) {
			profile.WorkerConfig["hevcLevelResolutionWarning"] = "Recommended HEVC Level is unresolved because effective output FPS or geometry is unknown"
			return profile
		}
		fps := parseFrameRateValue(workerStringValue(profile.WorkerConfig["effectiveOutputFrameRate"]))
		if fps <= 0 {
			fps = parseFrameRateValue(source.FrameRate)
		}
		width := workerIntValue(profile.WorkerConfig["effectiveOutputWidth"], source.Width)
		height := workerIntValue(profile.WorkerConfig["effectiveOutputHeight"], source.Height)
		recommendation := recommendHEVCLevel(width, height, fps, 0)
		profile.WorkerConfig["hevcLevelRecommendation"] = recommendation
		requested = recommendation.RecommendedLevel
		profile.WorkerConfig["hevcLevelResolutionWarning"] = "Stored Recommended Level was unavailable; MVForge recalculated it from the current asset."
	}
	if requested == "" {
		return profile
	}
	profile.WorkerConfig["hevcLevelEffective"] = requested
	profile.WorkerConfig["hevcLevelTier"] = "main"
	return profile
}

func resolveEffectiveVideoGeometry(profile models.Profile, source MediaStream) (int, int, bool) {
	width, height := source.Width, source.Height
	if width <= 0 || height <= 0 {
		return 0, 0, false
	}
	filters := workerStringValue(profile.WorkerConfig["videoFilters"])
	for _, raw := range strings.Split(filters, ",") {
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
				return 0, 0, false
			}
			cropWidth, widthErr := strconv.Atoi(strings.TrimSpace(values[0]))
			cropHeight, heightErr := strconv.Atoi(strings.TrimSpace(values[1]))
			if widthErr != nil || heightErr != nil || cropWidth <= 0 || cropHeight <= 0 {
				return 0, 0, false
			}
			width, height = cropWidth, cropHeight
		case "scale":
			values := strings.Split(value, ":")
			if len(values) < 2 {
				return 0, 0, false
			}
			scaleWidth, widthErr := strconv.Atoi(strings.Trim(strings.TrimSpace(values[0]), "'\""))
			scaleHeight, heightErr := strconv.Atoi(strings.Trim(strings.TrimSpace(values[1]), "'\""))
			if widthErr != nil || heightErr != nil || scaleWidth == 0 || scaleHeight == 0 {
				return 0, 0, false
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
				return 0, 0, false
			}
		case "fps", "bwdif", "yadif", "fieldmatch", "decimate", "eq", "hqdn3d", "unsharp", "format", "colorspace", "setfield", "setsar", "setdar", "null":
			// These filters preserve pixel dimensions in MVForge's current use.
		default:
			// An advanced/custom filter may change geometry. Unknown is safer
			// than deriving Auto Level from the source dimensions.
			return 0, 0, false
		}
	}
	return width, height, true
}

func scaledDimension(value, reference, target int, even bool) int {
	result := int(math.Round(float64(value) * float64(target) / float64(reference)))
	if even && result%2 != 0 {
		result--
	}
	return max(1, result)
}

func hevcLevelArgs(profile models.Profile, encoder string) []string {
	level := normalizedHEVCLevel(workerStringValue(profile.WorkerConfig["hevcLevelEffective"]))
	if level == "" {
		return nil
	}
	switch encoder {
	case "hevc_qsv":
		return []string{"-level:v", hevcQSVLevelValue(level), "-tier", "main"}
	}
	return nil
}

func x265ParamsWithHEVCLevel(profile models.Profile, params string) string {
	level := normalizedHEVCLevel(workerStringValue(profile.WorkerConfig["hevcLevelEffective"]))
	parts := strings.Split(params, ":")
	result := make([]string, 0, len(parts)+2)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(strings.SplitN(part, "=", 2)[0]))
		if level != "" && (name == "level-idc" || name == "high-tier") {
			continue
		}
		result = append(result, part)
	}
	if level != "" {
		result = append(result, "level-idc="+level, "high-tier=0")
	}
	return strings.Join(result, ":")
}
