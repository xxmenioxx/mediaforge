package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	subtitleStylingVersion       = 1
	subtitleStylingPolicyKey     = "subtitleStyling"
	subtitleStylingPolicyVersion = "subtitleStylingPolicyVersion"
	maxSubtitleSavedColors       = 32
	maxSubtitleRecentColors      = 12
)

var subtitleStyleIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,63}$`)

const (
	SubtitleStyleDefault = "MVF_Default"
	SubtitleStyleTop     = "MVF_Top"
	SubtitleStyleLyrics  = "MVF_Lyrics"
	SubtitleStyleSign    = "MVF_Sign"
)

// SubtitleStylePreset is a reusable appearance definition. Its ID, rather
// than DisplayName, is the identity used by policies and future composition.
type SubtitleStylePreset struct {
	ID                string `json:"id"`
	DisplayName       string `json:"displayName"`
	PrimaryColor      string `json:"primaryColor"`
	OutlineColor      string `json:"outlineColor"`
	ShadowBackColor   string `json:"shadowBackColor"`
	Opacity           int    `json:"opacity"`
	FontFamily        string `json:"fontFamily"`
	FontSize          int    `json:"fontSize"`
	AutoFontSize      bool   `json:"autoFontSize"`
	Bold              bool   `json:"bold"`
	OutlineWidth      int    `json:"outlineWidth"`
	ShadowDepth       int    `json:"shadowDepth"`
	AlignmentBehavior string `json:"alignmentBehavior"`
	Provenance        string `json:"provenance"`
}

// SubtitleStylingSettings is the versioned, bounded persisted policy. It is
// intentionally independent from subtitle cues, semantic roles, and native
// ASS documents.
type SubtitleStylingSettings struct {
	Version                   int                            `json:"version"`
	Enabled                   bool                           `json:"enabled"`
	ApplyToGeneratedASS       bool                           `json:"applyToGeneratedASS"`
	PreserveSourceStyles      bool                           `json:"preserveSourceStyles"`
	OverrideExistingASSStyles bool                           `json:"overrideExistingASSStyles"`
	BuiltInOverrides          map[string]SubtitleStylePreset `json:"builtInOverrides"`
	CustomPresets             []SubtitleStylePreset          `json:"customPresets"`
	SavedColors               []string                       `json:"savedColors"`
	RecentColors              []string                       `json:"recentColors"`
}

// SubtitleStylePolicy is the immutable operation/Queue snapshot. Version is a
// digest of the normalized bounded settings, not a mutable database revision.
type SubtitleStylePolicy struct {
	Settings SubtitleStylingSettings `json:"settings"`
	Version  string                  `json:"version"`
	Role     SubtitleSemanticRole    `json:"-"`
}

func subtitleStylePolicyWithRole(policy SubtitleStylePolicy, role SubtitleSemanticRole) SubtitleStylePolicy {
	policy.Role = role
	return policy
}

type subtitleStyleResolutionInput struct {
	Role                SubtitleSemanticRole
	Geometry            *bitmapCueGeometry
	ExplicitStyleID     string
	ExplicitAlignment   int
	HasExplicitPosition bool
}

type subtitleStyleResolution struct {
	Role             SubtitleSemanticRole
	StyleID          string
	Style            SubtitleStylePreset
	Alignment        int
	HasExactPosition bool
}

var subtitlePalette = []struct {
	Name  string
	Color string
}{
	{Name: "White", Color: "#FFFFFF"},
	{Name: "Warm White", Color: "#FFF4D6"},
	{Name: "Yellow", Color: "#FFFF00"},
	{Name: "Gold", Color: "#FFD700"},
	{Name: "Cyan", Color: "#00FFFF"},
	{Name: "Sky Blue", Color: "#7FDBFF"},
	{Name: "Light Green", Color: "#90EE90"},
	{Name: "Pink", Color: "#FF9ED2"},
	{Name: "Orange", Color: "#FFA500"},
	{Name: "Lavender", Color: "#C4A7E7"},
}

func defaultSubtitleStylePresets() map[string]SubtitleStylePreset {
	return map[string]SubtitleStylePreset{
		SubtitleStyleDefault: {ID: SubtitleStyleDefault, DisplayName: "Default", PrimaryColor: "#FFFFFF", OutlineColor: "#000000", ShadowBackColor: "#000000", Opacity: 100, FontFamily: "Arial", FontSize: 20, AlignmentBehavior: "bottom", Provenance: "builtIn"},
		SubtitleStyleTop:     {ID: SubtitleStyleTop, DisplayName: "Top", PrimaryColor: "#FFFF00", OutlineColor: "#000000", ShadowBackColor: "#000000", Opacity: 100, FontFamily: "Arial", FontSize: 20, AlignmentBehavior: "top", Provenance: "builtIn"},
		SubtitleStyleLyrics:  {ID: SubtitleStyleLyrics, DisplayName: "Lyrics", PrimaryColor: "#00FFFF", OutlineColor: "#000000", ShadowBackColor: "#000000", Opacity: 100, FontFamily: "Arial", FontSize: 20, AlignmentBehavior: "bottom", Provenance: "builtIn"},
		SubtitleStyleSign:    {ID: SubtitleStyleSign, DisplayName: "Sign", PrimaryColor: "#90EE90", OutlineColor: "#000000", ShadowBackColor: "#000000", Opacity: 100, FontFamily: "Arial", FontSize: 20, AlignmentBehavior: "center", Provenance: "builtIn"},
	}
}

func defaultSubtitleStylingSettings() SubtitleStylingSettings {
	return SubtitleStylingSettings{
		Version:                   subtitleStylingVersion,
		Enabled:                   false,
		ApplyToGeneratedASS:       true,
		PreserveSourceStyles:      true,
		OverrideExistingASSStyles: false,
		BuiltInOverrides:          map[string]SubtitleStylePreset{},
		CustomPresets:             []SubtitleStylePreset{},
		SavedColors:               []string{},
		RecentColors:              []string{},
	}
}

func defaultSubtitleStylePolicy() SubtitleStylePolicy {
	settings := defaultSubtitleStylingSettings()
	return SubtitleStylePolicy{Settings: settings, Version: subtitleStylePolicyHash(settings)}
}

func subtitlePaletteColors() []string {
	result := make([]string, len(subtitlePalette))
	for index, entry := range subtitlePalette {
		result[index] = entry.Color
	}
	return result
}

func normalizeSubtitleHexColor(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if len(value) == 0 || value[0] != '#' {
		return "", errors.New("subtitle color must be a #RRGGBB value")
	}
	if len(value) == 4 {
		value = "#" + strings.Repeat(string(value[1]), 2) + strings.Repeat(string(value[2]), 2) + strings.Repeat(string(value[3]), 2)
	}
	if len(value) != 7 {
		return "", errors.New("subtitle color must contain three or six hexadecimal digits")
	}
	for _, char := range value[1:] {
		if !((char >= '0' && char <= '9') || (char >= 'A' && char <= 'F')) {
			return "", errors.New("subtitle color contains a non-hexadecimal digit")
		}
	}
	return value, nil
}

func subtitleASSColor(value string, opacity int) (string, error) {
	color, err := normalizeSubtitleHexColor(value)
	if err != nil {
		return "", err
	}
	if opacity < 0 || opacity > 100 {
		return "", errors.New("subtitle opacity must be between 0 and 100")
	}
	red, _ := strconvParseHexByte(color[1:3])
	green, _ := strconvParseHexByte(color[3:5])
	blue, _ := strconvParseHexByte(color[5:7])
	alpha := int(math.Round(float64(100-opacity) * 255 / 100))
	return fmt.Sprintf("&H%02X%02X%02X%02X&", alpha, blue, green, red), nil
}

func strconvParseHexByte(value string) (int, error) {
	parsed, err := strconv.ParseUint(value, 16, 8)
	return int(parsed), err
}

func subtitleASSColorRoundTrip(value string) (string, int, error) {
	value = strings.TrimSpace(value)
	if len(value) != 11 || !strings.HasPrefix(strings.ToUpper(value), "&H") || !strings.HasSuffix(value, "&") {
		return "", 0, errors.New("invalid ASS color")
	}
	alpha, err := strconv.ParseUint(value[2:4], 16, 8)
	if err != nil {
		return "", 0, err
	}
	blue, err := strconv.ParseUint(value[4:6], 16, 8)
	if err != nil {
		return "", 0, err
	}
	green, err := strconv.ParseUint(value[6:8], 16, 8)
	if err != nil {
		return "", 0, err
	}
	red, err := strconv.ParseUint(value[8:10], 16, 8)
	if err != nil {
		return "", 0, err
	}
	opacity := int(math.Round(100 - float64(alpha)*100/255))
	return fmt.Sprintf("#%02X%02X%02X", red, green, blue), opacity, nil
}

func normalizeSubtitleStylingValue(value models.JSONMap) (models.JSONMap, error) {
	settings, err := normalizeSubtitleStylingSettings(value[subtitleStylingPolicyKey])
	if err != nil {
		return nil, err
	}
	result := cloneSubtitleJSONMap(value)
	if result == nil {
		result = models.JSONMap{}
	}
	result[subtitleStylingPolicyKey] = subtitleStylingSettingsMap(settings)
	return result, nil
}

func normalizeSubtitleSettingsForUpdate(value models.JSONMap) (models.JSONMap, error) {
	result := cloneSubtitleJSONMap(value)
	if result == nil {
		result = models.JSONMap{}
	}
	settings, err := normalizeSubtitleStylingSettings(result[subtitleStylingPolicyKey])
	if err != nil {
		return nil, err
	}
	result[subtitleStylingPolicyKey] = subtitleStylingSettingsMap(settings)
	return result, nil
}

func normalizeSubtitleStylingSettings(raw any) (SubtitleStylingSettings, error) {
	settings := defaultSubtitleStylingSettings()
	if raw == nil {
		return settings, nil
	}
	value, ok := raw.(map[string]any)
	if !ok {
		if mapped, mappedOK := raw.(models.JSONMap); mappedOK {
			value, ok = map[string]any(mapped), true
		}
	}
	if !ok {
		return settings, errors.New("subtitleStyling must be an object")
	}
	if version, exists := value["version"]; exists {
		settings.Version = subtitleIntValueSetting(version, subtitleStylingVersion)
		if settings.Version > subtitleStylingVersion {
			return settings, fmt.Errorf("unsupported subtitleStyling version %d", settings.Version)
		}
	}
	settings.Version = subtitleStylingVersion
	settings.Enabled = boolValueSetting(value["enabled"], settings.Enabled)
	settings.ApplyToGeneratedASS = boolValueSetting(value["applyToGeneratedASS"], settings.ApplyToGeneratedASS)
	settings.PreserveSourceStyles = boolValueSetting(value["preserveSourceStyles"], settings.PreserveSourceStyles)
	settings.OverrideExistingASSStyles = boolValueSetting(value["overrideExistingASSStyles"], settings.OverrideExistingASSStyles)
	if settings.Enabled && !settings.PreserveSourceStyles && !settings.OverrideExistingASSStyles {
		return settings, errors.New("subtitleStyling must preserve source styles or explicitly enable native override")
	}

	defaults := defaultSubtitleStylePresets()
	if rawOverrides, exists := value["builtInOverrides"]; exists && rawOverrides != nil {
		overrides, ok := rawOverrides.(map[string]any)
		if !ok {
			return settings, errors.New("subtitleStyling builtInOverrides must be an object")
		}
		for id, rawPreset := range overrides {
			base, known := defaults[id]
			if !known {
				return settings, fmt.Errorf("unsupported built-in subtitle style %q", id)
			}
			preset, err := normalizeSubtitlePreset(rawPreset, base, id, "builtIn")
			if err != nil {
				return settings, fmt.Errorf("built-in style %s: %w", id, err)
			}
			settings.BuiltInOverrides[id] = preset
		}
	}
	if rawCustom, exists := value["customPresets"]; exists && rawCustom != nil {
		custom, ok := rawCustom.([]any)
		if !ok || len(custom) > 128 {
			return settings, errors.New("subtitleStyling customPresets must contain at most 128 presets")
		}
		seen := map[string]struct{}{}
		for _, rawPreset := range custom {
			preset, err := normalizeSubtitlePreset(rawPreset, SubtitleStylePreset{DisplayName: "Custom", PrimaryColor: "#FFFFFF", OutlineColor: "#000000", ShadowBackColor: "#000000", Opacity: 100, FontFamily: "Arial", FontSize: 20, AlignmentBehavior: "bottom"}, "", "custom")
			if err != nil {
				return settings, fmt.Errorf("custom style: %w", err)
			}
			if _, duplicate := seen[preset.ID]; duplicate {
				return settings, fmt.Errorf("duplicate custom subtitle style ID %q", preset.ID)
			}
			seen[preset.ID] = struct{}{}
			settings.CustomPresets = append(settings.CustomPresets, preset)
		}
	}
	var colorErr error
	settings.SavedColors, colorErr = normalizeSubtitleColorList(value["savedColors"], maxSubtitleSavedColors)
	if colorErr != nil {
		return settings, fmt.Errorf("savedColors: %w", colorErr)
	}
	settings.RecentColors, colorErr = normalizeSubtitleColorList(value["recentColors"], maxSubtitleRecentColors)
	if colorErr != nil {
		return settings, fmt.Errorf("recentColors: %w", colorErr)
	}
	return settings, nil
}

func normalizeSubtitlePreset(raw any, base SubtitleStylePreset, forcedID, provenance string) (SubtitleStylePreset, error) {
	encoded, err := json.Marshal(raw)
	if err != nil {
		return base, errors.New("must be an object")
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return base, errors.New("must be an object")
	}
	if fields == nil {
		return base, errors.New("must be an object")
	}
	if forcedID != "" {
		base.ID = forcedID
	} else if id, ok := fields["id"].(string); ok && strings.TrimSpace(id) != "" {
		base.ID = strings.TrimSpace(id)
	} else {
		base.ID = newSubtitleCustomPresetID()
	}
	if !subtitleStyleIDPattern.MatchString(base.ID) || strings.HasPrefix(base.ID, "MVF_") && provenance == "custom" {
		return base, fmt.Errorf("invalid stable style ID %q", base.ID)
	}
	if provenance != "builtIn" {
		if display, ok := fields["displayName"].(string); ok && strings.TrimSpace(display) != "" {
			base.DisplayName = strings.TrimSpace(display)
		}
	}
	if color, ok := fields["primaryColor"].(string); ok {
		base.PrimaryColor = color
	}
	if color, ok := fields["outlineColor"].(string); ok {
		base.OutlineColor = color
	}
	if color, ok := fields["shadowBackColor"].(string); ok {
		base.ShadowBackColor = color
	}
	base.PrimaryColor, err = normalizeSubtitleHexColor(base.PrimaryColor)
	if err != nil {
		return base, fmt.Errorf("primaryColor: %w", err)
	}
	base.OutlineColor, err = normalizeSubtitleHexColor(base.OutlineColor)
	if err != nil {
		return base, fmt.Errorf("outlineColor: %w", err)
	}
	base.ShadowBackColor, err = normalizeSubtitleHexColor(base.ShadowBackColor)
	if err != nil {
		return base, fmt.Errorf("shadowBackColor: %w", err)
	}
	base.Opacity = subtitleIntValueSetting(fields["opacity"], base.Opacity)
	base.FontSize = subtitleIntValueSetting(fields["fontSize"], base.FontSize)
	base.OutlineWidth = subtitleIntValueSetting(fields["outlineWidth"], base.OutlineWidth)
	base.ShadowDepth = subtitleIntValueSetting(fields["shadowDepth"], base.ShadowDepth)
	if base.Opacity < 0 || base.Opacity > 100 || base.FontSize < 0 || base.FontSize > 200 || base.OutlineWidth < 0 || base.OutlineWidth > 20 || base.ShadowDepth < 0 || base.ShadowDepth > 20 {
		return base, errors.New("numeric style field is outside its supported range")
	}
	if font, ok := fields["fontFamily"].(string); ok {
		base.FontFamily = strings.TrimSpace(font)
	}
	if len(base.FontFamily) > 128 {
		return base, errors.New("fontFamily is too long")
	}
	if auto, ok := fields["autoFontSize"].(bool); ok {
		base.AutoFontSize = auto
	}
	if bold, ok := fields["bold"].(bool); ok {
		base.Bold = bold
	}
	if alignment, ok := fields["alignmentBehavior"].(string); ok {
		base.AlignmentBehavior = strings.ToLower(strings.TrimSpace(alignment))
	}
	if base.AlignmentBehavior != "bottom" && base.AlignmentBehavior != "top" && base.AlignmentBehavior != "center" && base.AlignmentBehavior != "auto" {
		return base, errors.New("alignmentBehavior must be bottom, top, center, or auto")
	}
	base.Provenance = provenance
	return base, nil
}

func normalizeSubtitleColorList(raw any, limit int) ([]string, error) {
	if raw == nil {
		return []string{}, nil
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, errors.New("must be an array")
	}
	result := make([]string, 0, minInt(len(values), limit))
	seen := map[string]struct{}{}
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			return nil, errors.New("must contain only HEX strings")
		}
		color, err := normalizeSubtitleHexColor(text)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[color]; duplicate {
			continue
		}
		seen[color] = struct{}{}
		result = append(result, color)
		if len(result) == limit {
			break
		}
	}
	return result, nil
}

func subtitleStylingSettingsValue(value models.JSONMap) SubtitleStylingSettings {
	settings, err := normalizeSubtitleStylingSettings(value[subtitleStylingPolicyKey])
	if err != nil {
		return defaultSubtitleStylingSettings()
	}
	return settings
}

func subtitleStylingPolicyFromDB(db *gorm.DB) SubtitleStylePolicy {
	policy := defaultSubtitleStylePolicy()
	if db == nil {
		return policy
	}
	var setting models.AppSetting
	if err := db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)}).First(&setting, "key = ?", subtitleSettingsKey).Error; err != nil {
		return policy
	}
	settings := subtitleStylingSettingsValue(setting.Value)
	return SubtitleStylePolicy{Settings: settings, Version: subtitleStylePolicyHash(settings)}
}

func subtitleStylePolicyHash(settings SubtitleStylingSettings) string {
	encoded, _ := json.Marshal(settings)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:8])
}

func subtitleStylingSettingsMap(settings SubtitleStylingSettings) map[string]any {
	encoded, _ := json.Marshal(settings)
	var result map[string]any
	_ = json.Unmarshal(encoded, &result)
	return result
}

func subtitleStylePolicyMap(policy SubtitleStylePolicy) map[string]any {
	result := subtitleStylingSettingsMap(policy.Settings)
	result[subtitleStylingPolicyVersion] = policy.Version
	return result
}

func applyPersistedSubtitleStylingSettings(profile map[string]any, policy SubtitleStylePolicy) map[string]any {
	if profile == nil {
		profile = map[string]any{}
	}
	profile[subtitleStylingPolicyKey] = subtitleStylePolicyMap(policy)
	return profile
}

func subtitleStylePolicyForQueueJob(job models.QueueJob) SubtitleStylePolicy {
	if job.TrackProfileSnapshot == nil {
		return defaultSubtitleStylePolicy()
	}
	return subtitleStylePolicyFromMap(job.TrackProfileSnapshot[subtitleStylingPolicyKey])
}

func subtitleStylePolicyFromMap(raw any) SubtitleStylePolicy {
	value, ok := raw.(map[string]any)
	if !ok {
		return defaultSubtitleStylePolicy()
	}
	settings, err := normalizeSubtitleStylingSettings(value)
	if err != nil {
		return defaultSubtitleStylePolicy()
	}
	version, _ := value[subtitleStylingPolicyVersion].(string)
	if version == "" {
		version = subtitleStylePolicyHash(settings)
	}
	return SubtitleStylePolicy{Settings: settings, Version: version}
}

func subtitleStylePresetForID(policy SubtitleStylePolicy, id string) (SubtitleStylePreset, bool) {
	defaults := defaultSubtitleStylePresets()
	base, ok := defaults[id]
	if !ok {
		for _, preset := range policy.Settings.CustomPresets {
			if preset.ID == id {
				return preset, true
			}
		}
		return SubtitleStylePreset{}, false
	}
	if override, overridden := policy.Settings.BuiltInOverrides[id]; overridden {
		base = override
	}
	return base, true
}

func resolveSubtitleStyle(policy SubtitleStylePolicy, input subtitleStyleResolutionInput) subtitleStyleResolution {
	role := normalizeSubtitleSemanticRole(string(input.Role))
	styleID := strings.TrimSpace(input.ExplicitStyleID)
	if styleID == "" {
		switch role {
		case SubtitleRoleSigns:
			styleID = SubtitleStyleSign
		default:
			styleID = SubtitleStyleDefault
		}
	}
	style, ok := subtitleStylePresetForID(policy, styleID)
	if !ok {
		styleID = SubtitleStyleDefault
		style, _ = subtitleStylePresetForID(policy, styleID)
	}
	alignment := subtitleAlignmentForBehavior(style.AlignmentBehavior)
	if input.ExplicitAlignment >= 1 && input.ExplicitAlignment <= 9 {
		alignment = input.ExplicitAlignment
	} else if !input.HasExplicitPosition && input.Geometry != nil {
		alignment = bitmapASSAlignmentForGeometry(input.Geometry)
	}
	return subtitleStyleResolution{Role: role, StyleID: styleID, Style: style, Alignment: alignment, HasExactPosition: input.HasExplicitPosition}
}

func subtitleAlignmentForBehavior(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "top":
		return 8
	case "center":
		return 5
	default:
		return 2
	}
}

func bitmapASSAlignmentForGeometry(geometry *bitmapCueGeometry) int {
	if geometry == nil || geometry.CanvasWidth <= 0 || geometry.CanvasHeight <= 0 {
		return 2
	}
	centerX := float64(geometry.BitmapX+geometry.BitmapWidth/2) / float64(geometry.CanvasWidth)
	centerY := float64(geometry.BitmapY+geometry.BitmapHeight/2) / float64(geometry.CanvasHeight)
	column := 1
	if centerX < 1.0/3.0 {
		column = 0
	} else if centerX >= 2.0/3.0 {
		column = 2
	}
	row := 2
	if centerY < 1.0/3.0 {
		row = 0
	} else if centerY < 2.0/3.0 {
		row = 1
	}
	return [3][3]int{{7, 8, 9}, {4, 5, 6}, {1, 2, 3}}[row][column]
}

func generatedSubtitleStyleName(preset SubtitleStylePreset, existing map[string]struct{}, identity string) string {
	base := "MVF_" + preset.ID + "_Generated"
	if _, collision := existing[base]; !collision {
		return base
	}
	digest := sha256.Sum256([]byte(identity + "\x00" + preset.ID))
	name := fmt.Sprintf("%s_%s", base, hex.EncodeToString(digest[:4]))
	for index := 2; ; index++ {
		if _, collision := existing[name]; !collision {
			return name
		}
		name = fmt.Sprintf("%s_%d", name, index)
	}
}

func newSubtitleCustomPresetID() string {
	bytes := make([]byte, 12)
	if _, err := rand.Read(bytes); err != nil {
		digest := sha256.Sum256([]byte("mvforge-subtitle-style"))
		return "custom_" + hex.EncodeToString(digest[:8])
	}
	return "custom_" + hex.EncodeToString(bytes)
}

func cloneSubtitleJSONMap(value models.JSONMap) models.JSONMap {
	if value == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var result models.JSONMap
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil
	}
	return result
}

func boolValueSetting(value any, fallback bool) bool {
	parsed, ok := value.(bool)
	if !ok {
		return fallback
	}
	return parsed
}

func subtitleIntValueSetting(value any, fallback int) int {
	switch parsed := value.(type) {
	case int:
		return parsed
	case int64:
		return int(parsed)
	case float64:
		if parsed == float64(int(parsed)) {
			return int(parsed)
		}
	}
	return fallback
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func sortedSubtitleStyleIDs(policy SubtitleStylePolicy) []string {
	ids := []string{SubtitleStyleDefault, SubtitleStyleTop, SubtitleStyleLyrics, SubtitleStyleSign}
	for _, preset := range policy.Settings.CustomPresets {
		ids = append(ids, preset.ID)
	}
	sort.Strings(ids)
	return ids
}

func styledASSFromCanonicalSRT(content []byte, policy SubtitleStylePolicy) ([]byte, error) {
	return styledASSFromCanonicalSRTWithRole(content, policy, SubtitleRoleUnknown)
}

func styledASSFromCanonicalSRTWithRole(content []byte, policy SubtitleStylePolicy, role SubtitleSemanticRole) ([]byte, error) {
	cues := parseSRTBitmapSubtitleCues(string(content))
	if len(cues) == 0 {
		return nil, errors.New("canonical SRT contains no usable subtitle cues")
	}
	result := renderBitmapASSCuesWithPolicyAndRole(cues, policy, role)
	if !validSubtitleSidecar("ass", result) {
		return nil, errors.New("generated ASS did not contain valid timed subtitle text")
	}
	return result, nil
}

func renderTextSubtitleASSFromSource(ctx context.Context, sourcePath string, streamIndex int, stagePath string, policy SubtitleStylePolicy, roles ...SubtitleSemanticRole) error {
	canonicalFile, err := os.CreateTemp(filepath.Dir(stagePath), ".mvforge-generated-subtitle-*.srt")
	if err != nil {
		return err
	}
	canonicalPath := canonicalFile.Name()
	if err := canonicalFile.Close(); err != nil {
		_ = os.Remove(canonicalPath)
		return err
	}
	defer os.Remove(canonicalPath)
	plan := MediaJobPlan{InputPath: sourcePath}
	var stderr strings.Builder
	if err := runSubtitleCommand(ctx, "ffmpeg", textSubtitleExtractionArgs(plan, streamIndex, "srt", canonicalPath), nil, &stderr); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("subtitle extraction failed: %s", strings.TrimSpace(stderr.String()))
		}
		return err
	}
	content, err := os.ReadFile(canonicalPath)
	if err != nil {
		return err
	}
	role := SubtitleRoleUnknown
	if len(roles) > 0 {
		role = roles[0]
	}
	styled, err := styledASSFromCanonicalSRTWithRole(content, policy, role)
	if err != nil {
		return err
	}
	return os.WriteFile(stagePath, styled, 0o644)
}
