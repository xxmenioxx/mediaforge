package scheduler

import (
	"fmt"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

const (
	EncoderPolicyLocked     = "locked"
	EncoderPolicyRestricted = "restricted"
	EncoderPolicyAutomatic  = "automatic"

	FallbackPolicyWait        = "wait"
	FallbackPolicyAllowedOnly = "allowed_only"
)

type ExecutionConstraints struct {
	SourceProfileID      uint     `json:"sourceProfileId"`
	SourceProfileVersion int      `json:"sourceProfileVersion"`
	CodecFamily          string   `json:"codecFamily"`
	EncoderPolicy        string   `json:"encoderPolicy"`
	PreferredEncoder     string   `json:"preferredEncoder"`
	AllowedEncoders      []string `json:"allowedEncoders"`
	FallbackPolicy       string   `json:"fallbackPolicy"`
	BitDepth             int      `json:"bitDepth"`
	PixelFormat          string   `json:"pixelFormat"`
	QualityStrategy      string   `json:"qualityStrategy"`
	QualityMode          string   `json:"qualityMode"`
	QualityValue         int      `json:"qualityValue"`
}

// ResolveExecutionConstraints turns both legacy and authoritative profiles into
// the immutable set of output constraints that a future scheduler must obey.
func ResolveExecutionConstraints(profile models.Profile) (ExecutionConstraints, error) {
	codecFamily := normalizedCodecFamily(profile.CodecFamily, profile.VideoCodec)
	preferred := normalizedEncoder(profile.PreferredEncoder)
	policy := strings.ToLower(strings.TrimSpace(profile.EncoderPolicy))
	allowed := normalizedEncoderList(profile.AllowedEncoders)
	fallback := strings.ToLower(strings.TrimSpace(profile.FallbackPolicy))

	legacy := policy == "" && preferred == "" && len(allowed) == 0
	if legacy {
		policy, preferred, allowed, fallback = legacyEncoderContract(profile, codecFamily)
	} else {
		if policy == "" {
			policy = EncoderPolicyRestricted
		}
		if preferred == "" && len(allowed) > 0 {
			preferred = allowed[0]
		}
		if len(allowed) == 0 && preferred != "" {
			allowed = []string{preferred}
		}
		if fallback == "" {
			fallback = FallbackPolicyWait
		}
	}

	constraints := ExecutionConstraints{
		SourceProfileID:      profile.ID,
		SourceProfileVersion: max(profile.ProfileVersion, 1),
		CodecFamily:          codecFamily,
		EncoderPolicy:        policy,
		PreferredEncoder:     preferred,
		AllowedEncoders:      allowed,
		FallbackPolicy:       fallback,
		BitDepth:             resolvedBitDepth(profile),
		PixelFormat:          resolvedPixelFormat(profile),
		QualityStrategy:      strings.ToLower(strings.TrimSpace(profile.QualityStrategy)),
		QualityMode:          strings.ToLower(strings.TrimSpace(profile.QualityMode)),
		QualityValue:         profile.QualityValue,
	}
	if constraints.QualityStrategy == "" {
		constraints.QualityStrategy = legacyQualityStrategy(profile.QualityValue)
	}

	if err := ValidateExecutionConstraints(constraints); err != nil {
		return ExecutionConstraints{}, err
	}
	return constraints, nil
}

func ApplyAuthoritativeContract(profile *models.Profile) error {
	constraints, err := ResolveExecutionConstraints(*profile)
	if err != nil {
		return err
	}
	profile.CodecFamily = constraints.CodecFamily
	profile.EncoderPolicy = constraints.EncoderPolicy
	profile.PreferredEncoder = constraints.PreferredEncoder
	profile.AllowedEncoders = models.StringList(constraints.AllowedEncoders)
	profile.FallbackPolicy = constraints.FallbackPolicy
	profile.BitDepth = constraints.BitDepth
	profile.PixelFormat = constraints.PixelFormat
	profile.QualityStrategy = constraints.QualityStrategy
	profile.ProfileVersion = constraints.SourceProfileVersion
	return nil
}

func ValidateExecutionConstraints(value ExecutionConstraints) error {
	if value.CodecFamily == "" {
		return fmt.Errorf("codec family is required")
	}
	if value.CodecFamily == "copy" {
		return nil
	}
	if value.EncoderPolicy != EncoderPolicyLocked && value.EncoderPolicy != EncoderPolicyRestricted && value.EncoderPolicy != EncoderPolicyAutomatic {
		return fmt.Errorf("unsupported encoder policy %q", value.EncoderPolicy)
	}
	if value.PreferredEncoder == "" {
		return fmt.Errorf("preferred encoder is required")
	}
	if len(value.AllowedEncoders) == 0 {
		return fmt.Errorf("at least one allowed encoder is required")
	}
	if !contains(value.AllowedEncoders, value.PreferredEncoder) {
		return fmt.Errorf("preferred encoder %q must be allowed", value.PreferredEncoder)
	}
	if value.EncoderPolicy == EncoderPolicyLocked && len(value.AllowedEncoders) != 1 {
		return fmt.Errorf("locked encoder policy must allow exactly one encoder")
	}
	for _, encoder := range value.AllowedEncoders {
		if !encoderMatchesCodec(encoder, value.CodecFamily) {
			return fmt.Errorf("encoder %q does not produce codec family %q", encoder, value.CodecFamily)
		}
	}
	if value.FallbackPolicy != FallbackPolicyWait && value.FallbackPolicy != FallbackPolicyAllowedOnly {
		return fmt.Errorf("unsupported fallback policy %q", value.FallbackPolicy)
	}
	return nil
}

func legacyEncoderContract(profile models.Profile, codecFamily string) (string, string, []string, string) {
	selected := normalizedEncoder(stringValue(profile.WorkerConfig, "videoEncoder"))
	if selected == "" {
		selected = normalizedEncoder(stringValue(profile.WorkerConfig, "encoder"))
	}
	allowHardware := boolValue(profile.WorkerConfig, "useHardwareIfAvailable")
	software := softwareEncoder(codecFamily)

	if selected == "" || selected == "auto" || selected == "ffmpeg" {
		if !allowHardware {
			return EncoderPolicyLocked, software, []string{software}, FallbackPolicyWait
		}
		allowed := append(hardwareEncoders(codecFamily), software)
		return EncoderPolicyAutomatic, allowed[0], allowed, FallbackPolicyAllowedOnly
	}
	if allowHardware && isHardwareEncoder(selected) && software != "" {
		return EncoderPolicyRestricted, selected, []string{selected, software}, FallbackPolicyAllowedOnly
	}
	return EncoderPolicyLocked, selected, []string{selected}, FallbackPolicyWait
}

func normalizedCodecFamily(explicit, legacy string) string {
	if value := strings.ToLower(strings.TrimSpace(explicit)); value != "" {
		return value
	}
	value := strings.ToLower(strings.TrimSpace(legacy))
	switch {
	case value == "copy":
		return "copy"
	case strings.Contains(value, "265") || strings.Contains(value, "hevc"):
		return "hevc"
	case strings.Contains(value, "264") || value == "h264":
		return "h264"
	case strings.Contains(value, "av1"):
		return "av1"
	default:
		return value
	}
}

func normalizedEncoderList(values models.StringList) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = normalizedEncoder(value)
		if value != "" && !contains(result, value) {
			result = append(result, value)
		}
	}
	return result
}

func normalizedEncoder(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "x265", "x265_10bit", "hevc":
		return "libx265"
	case "x264", "h264":
		return "libx264"
	default:
		return value
	}
}

func softwareEncoder(codec string) string {
	switch codec {
	case "hevc":
		return "libx265"
	case "h264":
		return "libx264"
	case "av1":
		return "libsvtav1"
	case "copy":
		return "copy"
	default:
		return codec
	}
}

func hardwareEncoders(codec string) []string {
	switch codec {
	case "hevc":
		return []string{"hevc_qsv", "hevc_nvenc", "hevc_videotoolbox", "hevc_amf"}
	case "h264":
		return []string{"h264_qsv", "h264_nvenc", "h264_videotoolbox", "h264_amf"}
	case "av1":
		return []string{"av1_qsv", "av1_nvenc", "av1_amf"}
	default:
		return nil
	}
}

func encoderMatchesCodec(encoder, codec string) bool {
	if codec == "copy" {
		return encoder == "copy"
	}
	switch codec {
	case "hevc":
		return encoder == "libx265" || strings.HasPrefix(encoder, "hevc_")
	case "h264":
		return encoder == "libx264" || strings.HasPrefix(encoder, "h264_")
	case "av1":
		return encoder == "libaom-av1" || encoder == "libsvtav1" || strings.HasPrefix(encoder, "av1_")
	default:
		return true
	}
}

func resolvedBitDepth(profile models.Profile) int {
	if profile.BitDepth > 0 {
		return profile.BitDepth
	}
	pixelFormat := resolvedPixelFormat(profile)
	if strings.Contains(pixelFormat, "10") || strings.Contains(strings.ToLower(profile.VideoCodec), "10bit") {
		return 10
	}
	return 8
}

func resolvedPixelFormat(profile models.Profile) string {
	if value := strings.TrimSpace(profile.PixelFormat); value != "" {
		return value
	}
	return stringValue(profile.WorkerConfig, "pixFmt")
}

func legacyQualityStrategy(value int) string {
	switch {
	case value > 0 && value <= 18:
		return "high"
	case value >= 25:
		return "small_size"
	default:
		return "balanced"
	}
}

func stringValue(values models.JSONMap, key string) string {
	if values == nil {
		return ""
	}
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func boolValue(values models.JSONMap, key string) bool {
	if values == nil {
		return false
	}
	value, _ := values[key].(bool)
	return value
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func isHardwareEncoder(value string) bool {
	return strings.HasSuffix(value, "_qsv") || strings.HasSuffix(value, "_nvenc") || strings.HasSuffix(value, "_videotoolbox") || strings.HasSuffix(value, "_amf")
}
