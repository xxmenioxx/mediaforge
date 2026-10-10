package validationpolicy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
)

const (
	SchemaVersion = 1
	PolicyVersion = "validation-policy-v1"
	Disabled      = "disabled"
	Basic         = "basic"
	Default       = "default"
	Strict        = "strict"
	Custom        = "custom"
)

// Policy is the normalized effective Validation configuration. Fingerprints
// are computed from this complete value, never from settings storage bytes.
type Policy struct {
	SchemaVersion int             `json:"schemaVersion"`
	PolicyVersion string          `json:"policyVersion"`
	Preset        string          `json:"preset"`
	BasedOn       string          `json:"basedOn,omitempty"`
	Rules         map[string]Rule `json:"rules"`
}

type Rule struct {
	Enabled        bool               `json:"enabled"`
	Disposition    string             `json:"disposition"`
	Overrideable   bool               `json:"overrideable"`
	Thresholds     map[string]float64 `json:"thresholds,omitempty"`
	Fields         map[string]bool    `json:"fields,omitempty"`
	WarningPenalty int                `json:"warningPenalty"`
	FailurePenalty int                `json:"failurePenalty"`
}

var mandatoryRules = map[string]bool{
	"artifact.identity":  true,
	"artifact.manifest":  true,
	"artifact.integrity": true,
	"execution.binding":  true,
}

var fidelityFields = []string{
	"codec", "width", "height", "sampleAspectRatio", "displayAspectRatio",
	"frameRate", "fieldStructure", "pixelSemantic", "bitDepth", "chromaLocation",
	"hevcLevel", "hevcTier", "colorPolicy",
}

func KnownRuleKeys() []string {
	keys := []string{
		"artifact.identity", "artifact.integrity", "artifact.manifest", "execution.binding",
		"duration.timeline", "timeline.av_alignment", "video.properties",
		"audio.tracks_actions", "subtitles.streams_actions", "chapters.actions",
		"attachments.stream_actions", "attachments.font_artifacts", "sidecars.package",
	}
	sort.Strings(keys)
	return keys
}

func IsMandatory(key string) bool { return mandatoryRules[key] }

func Factory(preset string) Policy {
	preset = normalizePreset(preset)
	rules := defaultRules()
	switch preset {
	case Disabled:
		for key, rule := range rules {
			if !mandatoryRules[key] {
				rule.Enabled = false
				rules[key] = rule
			}
		}
	case Basic:
		for _, key := range []string{"timeline.av_alignment", "video.properties"} {
			rule := rules[key]
			rule.Disposition = "advisory"
			rules[key] = rule
		}
	case Strict:
		for key, rule := range rules {
			if !mandatoryRules[key] {
				rule.Disposition = "blocking"
				rules[key] = rule
			}
		}
	}
	return Policy{SchemaVersion: SchemaVersion, PolicyVersion: PolicyVersion, Preset: preset, Rules: rules}
}

func DefaultPolicy() Policy { return Factory(Default) }

// Normalize accepts JSON-shaped settings and fills all omitted or invalid
// values from a safe factory preset. Unknown rule keys and fields are dropped.
func Normalize(raw map[string]any) Policy {
	requestedPreset := normalizePreset(stringValue(raw["preset"]))
	basePreset := requestedPreset
	if requestedPreset == Custom {
		basePreset = normalizeNamedPreset(stringValue(raw["basedOn"]))
	}
	out := Factory(basePreset)
	out.Preset = requestedPreset
	if requestedPreset == Custom {
		out.BasedOn = basePreset
	}

	rawRules, _ := raw["rules"].(map[string]any)
	for key, value := range rawRules {
		current, exists := out.Rules[key]
		if !exists {
			continue
		}
		fields, _ := value.(map[string]any)
		if fields == nil {
			continue
		}
		if v, ok := fields["enabled"].(bool); ok {
			current.Enabled = v
		}
		if v, ok := fields["disposition"].(string); ok && (v == "blocking" || v == "advisory") {
			current.Disposition = v
		}
		if v, ok := fields["overrideable"].(bool); ok {
			current.Overrideable = v
		}
		if v, ok := intValue(fields["warningPenalty"]); ok && v >= 0 && v <= 100 {
			current.WarningPenalty = v
		}
		if v, ok := intValue(fields["failurePenalty"]); ok && v >= 0 && v <= 100 {
			current.FailurePenalty = v
		}
		if rawThresholds, ok := fields["thresholds"].(map[string]any); ok {
			for threshold, rawValue := range rawThresholds {
				if !knownThreshold(key, threshold) {
					continue
				}
				if value, ok := floatValue(rawValue); ok && value >= 0 && value <= maxThreshold(key, threshold) {
					current.Thresholds[threshold] = value
				}
			}
		}
		if key == "timeline.av_alignment" && current.Thresholds["warningFrames"] < current.Thresholds["passFrames"] {
			defaults := Factory(basePreset).Rules[key].Thresholds
			current.Thresholds["passFrames"] = defaults["passFrames"]
			current.Thresholds["warningFrames"] = defaults["warningFrames"]
		}
		if rawFields, ok := fields["fields"].(map[string]any); ok && key == "video.properties" {
			for name, value := range rawFields {
				if !isFidelityField(name) {
					continue
				}
				if enabled, ok := value.(bool); ok {
					current.Fields[name] = enabled
				}
			}
		}
		out.Rules[key] = current
	}

	for key := range mandatoryRules {
		out.Rules[key] = mandatoryRule()
	}
	if requestedPreset != Custom && !sameRules(out.Rules, Factory(basePreset).Rules) {
		out.Preset = Custom
		out.BasedOn = basePreset
	}
	return out
}

// MigrateLegacy converts the former minimumScore/requireDurationMatch shape.
// minimumScore is discarded because it cannot authorize Validation or publish.
func MigrateLegacy(raw map[string]any) Policy {
	if _, hasRules := raw["rules"]; hasRules {
		return Normalize(raw)
	}
	policy := DefaultPolicy()
	if enabled, ok := raw["requireDurationMatch"].(bool); ok && !enabled {
		rule := policy.Rules["duration.timeline"]
		rule.Enabled = false
		policy.Rules["duration.timeline"] = rule
		policy.Preset = Custom
		policy.BasedOn = Default
	}
	return policy
}

func Fingerprint(policy Policy) string {
	encoded, _ := json.Marshal(policy)
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Differences returns deterministic field paths whose effective values differ
// from the selected named preset.
func Differences(policy Policy, basePreset string) []string {
	base := Factory(normalizeNamedPreset(basePreset))
	paths := make([]string, 0)
	for _, key := range KnownRuleKeys() {
		actual, expected := policy.Rules[key], base.Rules[key]
		prefix := "rules." + key + "."
		if actual.Enabled != expected.Enabled {
			paths = append(paths, prefix+"enabled")
		}
		if actual.Disposition != expected.Disposition {
			paths = append(paths, prefix+"disposition")
		}
		if actual.Overrideable != expected.Overrideable {
			paths = append(paths, prefix+"overrideable")
		}
		if actual.WarningPenalty != expected.WarningPenalty {
			paths = append(paths, prefix+"warningPenalty")
		}
		if actual.FailurePenalty != expected.FailurePenalty {
			paths = append(paths, prefix+"failurePenalty")
		}
		paths = append(paths, mapDifferences(prefix+"thresholds.", actual.Thresholds, expected.Thresholds)...)
		for _, field := range fidelityFields {
			if actual.Fields[field] != expected.Fields[field] {
				paths = append(paths, prefix+"fields."+field)
			}
		}
	}
	return paths
}

func defaultRules() map[string]Rule {
	fields := make(map[string]bool, len(fidelityFields))
	for _, key := range fidelityFields {
		fields[key] = true
	}
	return map[string]Rule{
		"artifact.identity":          mandatoryRule(),
		"artifact.manifest":          mandatoryRule(),
		"artifact.integrity":         mandatoryRule(),
		"execution.binding":          mandatoryRule(),
		"duration.timeline":          qualityRule(true, "blocking", true, 5, 25, map[string]float64{"maxAbsoluteSeconds": 2, "maxPercent": 2}, nil),
		"timeline.av_alignment":      qualityRule(true, "blocking", true, 5, 25, map[string]float64{"passFrames": 1, "warningFrames": 2}, nil),
		"video.properties":           qualityRule(true, "blocking", true, 5, 25, nil, fields),
		"audio.tracks_actions":       qualityRule(true, "blocking", true, 5, 25, nil, nil),
		"subtitles.streams_actions":  qualityRule(true, "blocking", true, 5, 25, nil, nil),
		"chapters.actions":           qualityRule(true, "advisory", true, 4, 15, nil, nil),
		"attachments.stream_actions": qualityRule(true, "blocking", true, 5, 25, nil, nil),
		"attachments.font_artifacts": qualityRule(true, "blocking", true, 5, 25, nil, nil),
		"sidecars.package":           qualityRule(true, "blocking", true, 5, 25, nil, nil),
	}
}

func mandatoryRule() Rule {
	return Rule{Enabled: true, Disposition: "blocking", Overrideable: false}
}

func qualityRule(enabled bool, disposition string, overrideable bool, warning, failure int, thresholds map[string]float64, fields map[string]bool) Rule {
	return Rule{Enabled: enabled, Disposition: disposition, Overrideable: overrideable, WarningPenalty: warning, FailurePenalty: failure, Thresholds: thresholds, Fields: fields}
}

func mapDifferences(prefix string, left, right map[string]float64) []string {
	keys := map[string]bool{}
	for key := range left {
		keys[key] = true
	}
	for key := range right {
		keys[key] = true
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	paths := []string{}
	for _, key := range ordered {
		if left[key] != right[key] {
			paths = append(paths, prefix+key)
		}
	}
	return paths
}

func sameRules(left, right map[string]Rule) bool {
	leftBytes, _ := json.Marshal(left)
	rightBytes, _ := json.Marshal(right)
	return string(leftBytes) == string(rightBytes)
}

func normalizePreset(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case Disabled, Basic, Default, Strict, Custom:
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return Default
	}
}

func normalizeNamedPreset(value string) string {
	preset := normalizePreset(value)
	if preset == Custom {
		return Default
	}
	return preset
}

func knownThreshold(rule, name string) bool {
	switch rule {
	case "duration.timeline":
		return name == "maxAbsoluteSeconds" || name == "maxPercent"
	case "timeline.av_alignment":
		return name == "passFrames" || name == "warningFrames"
	default:
		return false
	}
}

func maxThreshold(rule, name string) float64 {
	if rule == "duration.timeline" && name == "maxAbsoluteSeconds" {
		return 3600
	}
	if rule == "duration.timeline" && name == "maxPercent" {
		return 100
	}
	return 100
}

func isFidelityField(value string) bool {
	for _, field := range fidelityFields {
		if value == field {
			return true
		}
	}
	return false
}

func stringValue(value any) string {
	parsed, _ := value.(string)
	return parsed
}

func intValue(value any) (int, bool) {
	parsed, ok := floatValue(value)
	if !ok || parsed != math.Trunc(parsed) {
		return 0, false
	}
	return int(parsed), true
}

func floatValue(value any) (float64, bool) {
	switch parsed := value.(type) {
	case float64:
		return parsed, !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
	case float32:
		return float64(parsed), !math.IsNaN(float64(parsed)) && !math.IsInf(float64(parsed), 0)
	case int:
		return float64(parsed), true
	case int64:
		return float64(parsed), true
	case json.Number:
		value, err := strconv.ParseFloat(parsed.String(), 64)
		return value, err == nil && !math.IsNaN(value) && !math.IsInf(value, 0)
	default:
		return 0, false
	}
}
