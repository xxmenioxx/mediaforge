package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

// CompositionCueClass is intentionally separate from SubtitleSemanticRole.
// In particular, lyrics is a bounded cue/style classification and is not a
// new global subtitle role.
type CompositionCueClass string

const (
	CompositionCueDialogue   CompositionCueClass = "dialogue"
	CompositionCueSign       CompositionCueClass = "sign"
	CompositionCueLyrics     CompositionCueClass = "lyrics"
	CompositionCueCommentary CompositionCueClass = "commentary"
	CompositionCueUnknown    CompositionCueClass = "unknown"
)

const (
	compositionPolicyVersion        = "subtitle-composition-v1"
	compositionNormalizationVersion = "subtitle-composition-normalization-v1"
	compositionDefaultToleranceMS   = 250
	compositionMaxToleranceMS       = 2000
	compositionMaxSupplementals     = 8
	compositionMaxProvenanceItems   = 256
)

type compositionLayoutEvidence struct {
	StyleName            string `json:"styleName,omitempty"`
	ExplicitAlignment    int    `json:"explicitAlignment,omitempty"`
	HasExplicitAlignment bool   `json:"hasExplicitAlignment,omitempty"`
	HasExplicitPosition  bool   `json:"hasExplicitPosition,omitempty"`
	HasMove              bool   `json:"hasMove,omitempty"`
	HasBitmapGeometry    bool   `json:"hasBitmapGeometry,omitempty"`
	Meaningful           bool   `json:"meaningful"`
	Strength             int    `json:"strength,omitempty"`
}

// SubtitleCompositionSource is the frozen, auditable identity of one input
// track or sidecar. Identity and revision are deliberately distinct: the
// former identifies the source, while the latter detects stale work.
type SubtitleCompositionSource struct {
	Identity                     string               `json:"identity"`
	Revision                     string               `json:"revision"`
	Representation               string               `json:"representation"`
	Language                     string               `json:"language"`
	Role                         SubtitleSemanticRole `json:"role"`
	RoleProvenance               string               `json:"roleProvenance,omitempty"`
	CueClass                     CompositionCueClass  `json:"cueClass"`
	TrackLabel                   string               `json:"trackLabel,omitempty"`
	StreamIndex                  int                  `json:"streamIndex,omitempty"`
	Primary                      bool                 `json:"primary"`
	ExplicitLanguage             bool                 `json:"explicitLanguage,omitempty"`
	CanonicalOCRArtifactIdentity string               `json:"canonicalOcrArtifactIdentity,omitempty"`
	CanonicalOCRArtifactRevision string               `json:"canonicalOcrArtifactRevision,omitempty"`
	CanonicalOCRArtifactPath     string               `json:"canonicalOcrArtifactPath,omitempty"`
	CanonicalOCRLogicalTransform string               `json:"canonicalOcrLogicalTransform,omitempty"`
}

type SubtitleCompositionPolicy struct {
	Version                  string                         `json:"version"`
	NormalizationVersion     string                         `json:"normalizationVersion"`
	TimingToleranceMillis    int                            `json:"timingToleranceMillis"`
	MaxSupplementalSources   int                            `json:"maxSupplementalSources"`
	AllowCrossLanguage       bool                           `json:"allowCrossLanguage"`
	AllowReviewableConflicts bool                           `json:"allowReviewableConflicts"`
	CanonicalizeSourceOrder  bool                           `json:"canonicalizeSourceOrder"`
	PromotePreferred         bool                           `json:"promotePreferred"`
	PromoteDefault           bool                           `json:"promoteDefault"`
	NativeStyleOverride      bool                           `json:"nativeStyleOverride"`
	StylePolicyVersion       string                         `json:"stylePolicyVersion"`
	StylePolicy              SubtitleStylePolicy            `json:"stylePolicy"`
	ExplicitStyleByClass     map[CompositionCueClass]string `json:"explicitStyleByClass,omitempty"`
}

// ResolvedSubtitleComposition is the backend-owned result frozen beside the
// existing ResolvedTrackPlan. A nil field preserves legacy profile snapshots
// that predate composition.
type ResolvedSubtitleComposition struct {
	Enabled           bool                        `json:"enabled"`
	Policy            SubtitleCompositionPolicy   `json:"policy"`
	Primary           SubtitleCompositionSource   `json:"primary"`
	Supplementals     []SubtitleCompositionSource `json:"supplementals"`
	PreferredEligible bool                        `json:"preferredEligible"`
	DefaultEligible   bool                        `json:"defaultEligible"`
	EligibilityReason string                      `json:"eligibilityReason,omitempty"`
}

func defaultSubtitleCompositionPolicy(style SubtitleStylePolicy) SubtitleCompositionPolicy {
	return SubtitleCompositionPolicy{
		Version:                  compositionPolicyVersion,
		NormalizationVersion:     compositionNormalizationVersion,
		TimingToleranceMillis:    compositionDefaultToleranceMS,
		MaxSupplementalSources:   compositionMaxSupplementals,
		AllowReviewableConflicts: false,
		CanonicalizeSourceOrder:  false,
		StylePolicyVersion:       style.Version,
		StylePolicy:              style,
		ExplicitStyleByClass:     map[CompositionCueClass]string{CompositionCueLyrics: SubtitleStyleLyrics},
	}
}

func compositionPolicyFromProfile(profile map[string]any, style SubtitleStylePolicy) (*SubtitleCompositionPolicy, bool, error) {
	raw, exists := profile["subtitleComposition"]
	if !exists || raw == nil {
		return nil, false, nil
	}
	if enabled, ok := raw.(bool); ok {
		if !enabled {
			return nil, false, nil
		}
		raw = map[string]any{"enabled": true}
	}
	if object, ok := raw.(map[string]any); ok {
		if enabled, present := object["enabled"]; present {
			if value, valid := enabled.(bool); valid && !value {
				return nil, false, nil
			}
		}
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, false, fmt.Errorf("subtitle composition policy is invalid: %w", err)
	}
	policy := defaultSubtitleCompositionPolicy(style)
	if err := json.Unmarshal(encoded, &policy); err != nil {
		return nil, false, fmt.Errorf("subtitle composition policy is invalid: %w", err)
	}
	if policy.Version == "" {
		policy.Version = compositionPolicyVersion
	}
	if policy.NormalizationVersion == "" {
		policy.NormalizationVersion = compositionNormalizationVersion
	}
	if policy.TimingToleranceMillis == 0 {
		policy.TimingToleranceMillis = compositionDefaultToleranceMS
	}
	if policy.MaxSupplementalSources == 0 {
		policy.MaxSupplementalSources = compositionMaxSupplementals
	}
	if policy.StylePolicyVersion == "" {
		policy.StylePolicyVersion = style.Version
	}
	if policy.StylePolicy.Version == "" {
		policy.StylePolicy = style
	}
	if policy.ExplicitStyleByClass == nil {
		policy.ExplicitStyleByClass = map[CompositionCueClass]string{CompositionCueLyrics: SubtitleStyleLyrics}
	}
	if err := validateSubtitleCompositionPolicy(policy); err != nil {
		return nil, false, err
	}
	return &policy, true, nil
}

func compositionScanRevision(scanPath string, scanSize int64, scanUpdated string, stream map[string]any) string {
	payload := struct {
		Path    string         `json:"path"`
		Size    int64          `json:"size"`
		Updated string         `json:"updated"`
		Stream  map[string]any `json:"stream"`
	}{scanPath, scanSize, scanUpdated, stream}
	encoded, _ := json.Marshal(payload)
	digest := sha256.Sum256(encoded)
	return "scan-" + hex.EncodeToString(digest[:])
}

func compositionClassForTrack(raw any, role SubtitleSemanticRole) CompositionCueClass {
	for _, key := range []string{"compositionCueClass", "cueClass", "compositionClass"} {
		value := ""
		switch typed := raw.(type) {
		case map[string]any:
			value = workerStringValue(typed[key])
		case map[string]string:
			value = typed[key]
		}
		if class := CompositionCueClass(strings.ToLower(strings.TrimSpace(value))); validCompositionCueClass(class) {
			return class
		}
	}
	return normalizeCompositionCueClass(CompositionCueUnknown, role)
}

func compositionIndexList(raw any) []int {
	values := []int{}
	switch typed := raw.(type) {
	case []int:
		return append(values, typed...)
	case []any:
		for _, value := range typed {
			if index := streamIndexValue(value); index >= 0 {
				values = append(values, index)
			}
		}
	}
	return values
}

func resolveSubtitleCompositionPlan(scan models.ScanResult, profile map[string]any, subtitles []ResolvedSubtitleTrack) (*ResolvedSubtitleComposition, error) {
	style := defaultSubtitleStylePolicy()
	policy, enabled, err := compositionPolicyFromProfile(profile, style)
	if err != nil || !enabled {
		return nil, err
	}
	streamByIndex := map[int]map[string]any{}
	for _, raw := range scan.SubtitleStreams {
		stream := settingProfileObject(raw)
		if index := streamIndexValue(stream["index"]); index >= 0 {
			streamByIndex[index] = stream
		}
	}
	resolvedByIndex := map[int]ResolvedSubtitleTrack{}
	for _, resolved := range subtitles {
		resolvedByIndex[resolved.StreamIndex] = resolved
	}
	primaryIndex := -1
	if raw, exists := profile["subtitleCompositionPrimaryStream"]; exists {
		primaryIndex = streamIndexValue(raw)
	}
	if primaryIndex < 0 {
		for _, resolved := range subtitles {
			if resolved.Action != SubtitleDispositionRemove && resolved.Role == SubtitleRoleFull {
				primaryIndex = resolved.StreamIndex
				break
			}
		}
	}
	if primaryIndex < 0 {
		return nil, fmt.Errorf("subtitle composition requires a configured primary/full-dialogue source")
	}
	primaryResolved, ok := resolvedByIndex[primaryIndex]
	_, rawOK := streamByIndex[primaryIndex]
	if !ok || !rawOK || primaryResolved.Action == SubtitleDispositionRemove || primaryResolved.Role != SubtitleRoleFull {
		return nil, fmt.Errorf("subtitle composition primary stream %d is not an active full-dialogue source", primaryIndex)
	}
	makeSource := func(index int, primary bool) (SubtitleCompositionSource, error) {
		resolved, resolvedOK := resolvedByIndex[index]
		raw, rawOK := streamByIndex[index]
		if !resolvedOK || !rawOK || resolved.Action == SubtitleDispositionRemove {
			return SubtitleCompositionSource{}, fmt.Errorf("subtitle composition source stream %d is not an active resolved subtitle", index)
		}
		language := normalizedTrackLanguage(resolved.Language)
		if language == "" || language == "und" {
			return SubtitleCompositionSource{}, fmt.Errorf("subtitle composition source stream %d has unknown language", index)
		}
		return SubtitleCompositionSource{
			Identity:       fmt.Sprintf("embedded:%s#stream=%d", scan.Path, index),
			Revision:       compositionScanRevision(scan.Path, scan.SizeBytes, scan.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), raw),
			Representation: subtitleRepresentationForCodec(resolved.Codec), Language: language,
			Role: resolved.Role, RoleProvenance: resolved.RoleProvenance, CueClass: compositionClassForTrack(raw, resolved.Role),
			TrackLabel: workerStringValue(raw["title"]), StreamIndex: index, Primary: primary, ExplicitLanguage: policy.AllowCrossLanguage,
		}, nil
	}
	primarySource, err := makeSource(primaryIndex, true)
	if err != nil {
		return nil, err
	}
	indices := compositionIndexList(profile["subtitleCompositionSupplementalStreams"])
	if len(indices) == 0 {
		for _, resolved := range subtitles {
			if resolved.StreamIndex != primaryIndex && resolved.Action != SubtitleDispositionRemove && normalizedTrackLanguage(resolved.Language) == primarySource.Language {
				indices = append(indices, resolved.StreamIndex)
			}
		}
	}
	if len(indices) > policy.MaxSupplementalSources {
		return nil, fmt.Errorf("subtitle composition has too many supplemental sources")
	}
	supplementals := make([]SubtitleCompositionSource, 0, len(indices))
	seen := map[int]struct{}{primaryIndex: {}}
	for _, index := range indices {
		if _, exists := seen[index]; exists {
			continue
		}
		seen[index] = struct{}{}
		source, sourceErr := makeSource(index, false)
		if sourceErr != nil {
			return nil, sourceErr
		}
		if !cueLanguageCompatible(primarySource, source, *policy) {
			return nil, fmt.Errorf("subtitle composition source %q is not language-compatible with primary", source.Identity)
		}
		supplementals = append(supplementals, source)
	}
	result := &ResolvedSubtitleComposition{Enabled: true, Policy: *policy, Primary: primarySource, Supplementals: supplementals}
	sources := append([]SubtitleCompositionSource{primarySource}, supplementals...)
	result.PreferredEligible, result.DefaultEligible = subtitleCompositionPromotion(*policy, sources)
	if !result.PreferredEligible && !result.DefaultEligible && (policy.PromotePreferred || policy.PromoteDefault) {
		result.EligibilityReason = "promotion requires the configured primary/full-dialogue source"
	}
	return result, nil
}

func validateSubtitleCompositionPolicy(policy SubtitleCompositionPolicy) error {
	if strings.TrimSpace(policy.Version) == "" || strings.TrimSpace(policy.NormalizationVersion) == "" {
		return fmt.Errorf("composition policy and normalization versions are required")
	}
	if policy.MaxSupplementalSources < 0 || policy.MaxSupplementalSources > compositionMaxSupplementals {
		return fmt.Errorf("composition policy allows at most %d supplemental sources", compositionMaxSupplementals)
	}
	if policy.TimingToleranceMillis <= 0 || policy.TimingToleranceMillis > compositionMaxToleranceMS {
		return fmt.Errorf("composition timing tolerance must be between 1 and %d milliseconds", compositionMaxToleranceMS)
	}
	if strings.TrimSpace(policy.StylePolicyVersion) == "" {
		return fmt.Errorf("composition style policy version is required")
	}
	for class, styleID := range policy.ExplicitStyleByClass {
		if !validCompositionCueClass(class) || strings.TrimSpace(styleID) == "" {
			return fmt.Errorf("composition style mapping is invalid for %q", class)
		}
	}
	return nil
}

type SubtitleCompositionCue struct {
	Event          assEvent                  `json:"-"`
	Source         SubtitleCompositionSource `json:"source"`
	Class          CompositionCueClass       `json:"class"`
	NormalizedText string                    `json:"normalizedText,omitempty"`
	Layout         compositionLayoutEvidence `json:"layout"`
}

type SubtitleCompositionDecisionKind string

const (
	CompositionDecisionKeptPrimary       SubtitleCompositionDecisionKind = "kept-primary"
	CompositionDecisionDeduplicated      SubtitleCompositionDecisionKind = "deduplicated"
	CompositionDecisionEnriched          SubtitleCompositionDecisionKind = "enriched/repaired"
	CompositionDecisionAdded             SubtitleCompositionDecisionKind = "added"
	CompositionDecisionAmbiguousConflict SubtitleCompositionDecisionKind = "ambiguous/conflicting"
)

type SubtitleCompositionDecision struct {
	Kind           SubtitleCompositionDecisionKind `json:"kind"`
	SourceIdentity string                          `json:"sourceIdentity"`
	SourceOrdinal  int                             `json:"sourceOrdinal"`
	CanonicalIndex int                             `json:"canonicalIndex,omitempty"`
	Reason         string                          `json:"reason,omitempty"`
}

type SubtitleCompositionProvenance struct {
	SourceIdentities    []string                                `json:"sourceIdentities"`
	SourceRevisions     []string                                `json:"sourceRevisions"`
	Languages           []string                                `json:"languages"`
	ResolvedLanguage    string                                  `json:"resolvedLanguage"`
	Roles               []SubtitleSemanticRole                  `json:"roles"`
	Classes             []CompositionCueClass                   `json:"classes"`
	Counts              map[SubtitleCompositionDecisionKind]int `json:"counts"`
	Decisions           []SubtitleCompositionDecision           `json:"decisions,omitempty"`
	Conflicts           []string                                `json:"conflicts,omitempty"`
	AttachmentNotes     []string                                `json:"attachmentNotes,omitempty"`
	AttachmentDecisions []SubtitleCompositionAttachmentDecision `json:"attachmentDecisions,omitempty"`
	OutputIdentity      string                                  `json:"outputIdentity,omitempty"`
	OutputPath          string                                  `json:"outputPath,omitempty"`
	Preferred           bool                                    `json:"preferred"`
	Default             bool                                    `json:"default"`
}

type SubtitleCompositionResult struct {
	Events     []SubtitleCompositionCue      `json:"-"`
	Decisions  []SubtitleCompositionDecision `json:"decisions,omitempty"`
	Provenance SubtitleCompositionProvenance `json:"provenance"`
	Conflict   bool                          `json:"conflict"`
}

func validCompositionCueClass(class CompositionCueClass) bool {
	switch class {
	case CompositionCueDialogue, CompositionCueSign, CompositionCueLyrics, CompositionCueCommentary, CompositionCueUnknown:
		return true
	default:
		return false
	}
}

func normalizeCompositionCueClass(class CompositionCueClass, role SubtitleSemanticRole) CompositionCueClass {
	if validCompositionCueClass(class) && class != CompositionCueUnknown {
		return class
	}
	switch role {
	case SubtitleRoleSigns:
		return CompositionCueSign
	case SubtitleRoleCommentary:
		return CompositionCueCommentary
	default:
		return CompositionCueDialogue
	}
}

func normalizedCompositionLanguage(language string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(language), "_", "-"))
}

func normalizedCompositionText(event assEvent) string {
	var builder strings.Builder
	for _, part := range event.Dialogue.Parts {
		if part.Protected {
			continue
		}
		builder.WriteString(part.Text)
	}
	value := builder.String()
	value = strings.ReplaceAll(value, "\\N", " ")
	value = strings.ReplaceAll(value, "\\n", " ")
	value = strings.ReplaceAll(value, "\\h", " ")
	for {
		start := strings.IndexByte(value, '{')
		if start < 0 {
			break
		}
		end := strings.IndexByte(value[start:], '}')
		if end < 0 {
			break
		}
		value = value[:start] + value[start+end+1:]
	}
	var builder2 strings.Builder
	for _, r := range strings.ToLower(value) {
		if unicode.IsSpace(r) {
			if builder2.Len() > 0 && !strings.HasSuffix(builder2.String(), " ") {
				builder2.WriteByte(' ')
			}
			continue
		}
		builder2.WriteRune(r)
	}
	return strings.TrimSpace(builder2.String())
}

func compositionLayoutForEvent(event assEvent) compositionLayoutEvidence {
	layout := event.Layout
	if layout.StyleName == "" {
		layout.StyleName = strings.TrimSpace(event.Style)
	}
	text := ""
	for _, part := range event.Dialogue.Parts {
		text += part.Text
	}
	for _, tag := range []string{"\\pos(", "\\move("} {
		if strings.Contains(text, tag) {
			layout.HasExplicitPosition = true
			layout.HasMove = layout.HasMove || tag == "\\move("
		}
	}
	for i := 1; i <= 9; i++ {
		if strings.Contains(text, fmt.Sprintf("\\an%d", i)) {
			layout.ExplicitAlignment = i
			layout.HasExplicitAlignment = true
			break
		}
	}
	if layout.HasExplicitPosition {
		layout.Strength = 4
	} else if layout.HasExplicitAlignment {
		layout.Strength = 3
	} else if strings.TrimSpace(layout.StyleName) != "" && !strings.EqualFold(layout.StyleName, SubtitleStyleDefault) {
		layout.Strength = 2
	}
	layout.Meaningful = layout.Strength > 0 || layout.HasBitmapGeometry
	return layout
}

func newSubtitleCompositionCue(event assEvent, source SubtitleCompositionSource, class CompositionCueClass) SubtitleCompositionCue {
	event.SourceIdentity = source.Identity
	event.SourceRevision = source.Revision
	event.SourceRepresentation = source.Representation
	event.SemanticRole = source.Role
	event.CompositionClass = class
	layout := compositionLayoutForEvent(event)
	event.Layout = layout
	return SubtitleCompositionCue{Event: event, Source: source, Class: normalizeCompositionCueClass(class, source.Role), NormalizedText: normalizedCompositionText(event), Layout: layout}
}

func cueLanguageCompatible(a, b SubtitleCompositionSource, policy SubtitleCompositionPolicy) bool {
	left, right := normalizedCompositionLanguage(a.Language), normalizedCompositionLanguage(b.Language)
	if left == "" || right == "" || left == "und" || right == "und" {
		return false
	}
	if left == right {
		return true
	}
	return policy.AllowCrossLanguage && (a.ExplicitLanguage || b.ExplicitLanguage)
}

func cueRoleCompatible(a, b SubtitleCompositionCue) bool {
	left, right := a.Source.Role, b.Source.Role
	if left != "" && right != "" && left != SubtitleRoleUnknown && right != SubtitleRoleUnknown && left != right {
		return false
	}
	if a.Class != CompositionCueUnknown && b.Class != CompositionCueUnknown && a.Class != b.Class {
		return false
	}
	return true
}

func compositionEventsProveDistinct(a, b SubtitleCompositionCue) bool {
	if !cueRoleCompatible(a, b) {
		return true
	}
	if a.Layout.HasExplicitPosition && b.Layout.HasExplicitPosition {
		return a.Layout.HasMove != b.Layout.HasMove || a.Layout.ExplicitAlignment != b.Layout.ExplicitAlignment || a.Layout.StyleName != b.Layout.StyleName
	}
	if a.Layout.HasExplicitAlignment && b.Layout.HasExplicitAlignment && a.Layout.ExplicitAlignment != b.Layout.ExplicitAlignment {
		return true
	}
	if a.Class != CompositionCueUnknown && b.Class != CompositionCueUnknown && a.Class != b.Class {
		return true
	}
	return false
}

func compositionMeaningfulEvidence(cue SubtitleCompositionCue) bool {
	style := strings.TrimSpace(cue.Event.Style)
	defaultStyle := strings.EqualFold(style, SubtitleStyleDefault) || strings.EqualFold(style, "default")
	return cue.Layout.Meaningful && !defaultStyle || (style != "" && !defaultStyle)
}

func compositionEquivalent(a, b SubtitleCompositionCue, policy SubtitleCompositionPolicy) bool {
	if a.Source.Identity == b.Source.Identity && a.Event.SourceOrdinal == b.Event.SourceOrdinal {
		return false
	}
	if a.NormalizedText == "" || a.NormalizedText != b.NormalizedText {
		return false
	}
	tolerance := float64(policy.TimingToleranceMillis) / 1000
	if absFloat(a.Event.Start-b.Event.Start) > tolerance || absFloat(a.Event.End-b.Event.End) > tolerance {
		return false
	}
	if !cueLanguageCompatible(a.Source, b.Source, policy) || !cueRoleCompatible(a, b) {
		return false
	}
	return true
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func sourceDescriptorValid(source SubtitleCompositionSource) error {
	if strings.TrimSpace(source.Identity) == "" || strings.TrimSpace(source.Revision) == "" {
		return fmt.Errorf("composition source identity and revision are required")
	}
	if normalizedCompositionLanguage(source.Language) == "" || normalizedCompositionLanguage(source.Language) == "und" {
		return fmt.Errorf("composition source %q has unknown language", source.Identity)
	}
	if !validCompositionCueClass(source.CueClass) {
		return fmt.Errorf("composition source %q has invalid cue class", source.Identity)
	}
	if (source.Role == "" || source.Role == SubtitleRoleUnknown) && source.CueClass == CompositionCueUnknown {
		return fmt.Errorf("composition source %q has ambiguous role/classification", source.Identity)
	}
	if strings.TrimSpace(source.CanonicalOCRArtifactPath) != "" {
		if !strings.HasPrefix(strings.TrimSpace(source.Identity), "embedded:") ||
			strings.TrimSpace(source.CanonicalOCRArtifactIdentity) == "" ||
			strings.TrimSpace(source.CanonicalOCRArtifactRevision) == "" ||
			strings.TrimSpace(source.CanonicalOCRLogicalTransform) == "" {
			return fmt.Errorf("composition source %q has incomplete canonical OCR provenance", source.Identity)
		}
	}
	return nil
}

func mergeCompositionEvidence(existing, incoming SubtitleCompositionCue) SubtitleCompositionCue {
	if !compositionMeaningfulEvidence(existing) && compositionMeaningfulEvidence(incoming) {
		if strings.TrimSpace(existing.Event.Style) == "" || strings.EqualFold(strings.TrimSpace(existing.Event.Style), SubtitleStyleDefault) {
			existing.Event.Style = incoming.Event.Style
		}
		if existing.Layout.ExplicitAlignment == 0 {
			existing.Layout.ExplicitAlignment = incoming.Layout.ExplicitAlignment
			existing.Layout.HasExplicitAlignment = incoming.Layout.HasExplicitAlignment
		}
		existing.Layout.HasExplicitPosition = incoming.Layout.HasExplicitPosition
		existing.Layout.HasMove = incoming.Layout.HasMove
		existing.Layout.Strength = incoming.Layout.Strength
		existing.Layout.Meaningful = incoming.Layout.Meaningful
		existing.Event.Layout = existing.Layout
	}
	return existing
}

// ReconcileSubtitleComposition implements the evolving canonical event set.
// The caller passes supplementals in the already-frozen precedence order.
func ReconcileSubtitleComposition(policy SubtitleCompositionPolicy, primary []SubtitleCompositionCue, supplementals ...[]SubtitleCompositionCue) (SubtitleCompositionResult, error) {
	if err := validateSubtitleCompositionPolicy(policy); err != nil {
		return SubtitleCompositionResult{}, err
	}
	if len(supplementals) > policy.MaxSupplementalSources {
		return SubtitleCompositionResult{}, fmt.Errorf("composition request has %d supplemental sources; maximum is %d", len(supplementals), policy.MaxSupplementalSources)
	}
	if len(primary) == 0 {
		return SubtitleCompositionResult{}, fmt.Errorf("composition requires a configured primary source")
	}
	allSources := []SubtitleCompositionSource{}
	seenSources := map[string]struct{}{}
	collectSource := func(source SubtitleCompositionSource) error {
		if err := sourceDescriptorValid(source); err != nil {
			return err
		}
		if _, exists := seenSources[source.Identity]; exists {
			return nil
		}
		seenSources[source.Identity] = struct{}{}
		allSources = append(allSources, source)
		return nil
	}
	groupSeen := map[string]struct{}{}
	validateSourceGroup := func(cues []SubtitleCompositionCue, primary bool) error {
		if len(cues) == 0 {
			return fmt.Errorf("composition source has no events")
		}
		identity := cues[0].Source.Identity
		for _, cue := range cues {
			if cue.Source.Identity != identity || cue.Source.Primary != primary {
				return fmt.Errorf("composition source group is not internally consistent")
			}
		}
		if _, exists := groupSeen[identity]; exists {
			return fmt.Errorf("composition source identity %q is repeated as a source group", identity)
		}
		groupSeen[identity] = struct{}{}
		return nil
	}
	if err := validateSourceGroup(primary, true); err != nil {
		return SubtitleCompositionResult{}, err
	}
	for _, sourceCues := range supplementals {
		if err := validateSourceGroup(sourceCues, false); err != nil {
			return SubtitleCompositionResult{}, err
		}
	}
	for _, cue := range primary {
		if err := collectSource(cue.Source); err != nil {
			return SubtitleCompositionResult{}, err
		}
	}
	for _, sourceCues := range supplementals {
		if len(sourceCues) == 0 {
			continue
		}
		for _, cue := range sourceCues {
			if err := collectSource(cue.Source); err != nil {
				return SubtitleCompositionResult{}, err
			}
		}
	}
	for index, source := range allSources {
		if index == 0 && !source.Primary {
			return SubtitleCompositionResult{}, fmt.Errorf("first composition source must be primary")
		}
		if index > 0 && source.Primary {
			return SubtitleCompositionResult{}, fmt.Errorf("only the first source may be primary")
		}
		if index > 0 {
			primaryLanguage := normalizedCompositionLanguage(allSources[0].Language)
			language := normalizedCompositionLanguage(source.Language)
			if language != primaryLanguage && (!policy.AllowCrossLanguage || !source.ExplicitLanguage) {
				return SubtitleCompositionResult{}, fmt.Errorf("source %q is cross-language without explicit authorization", source.Identity)
			}
		}
	}

	result := SubtitleCompositionResult{Events: append([]SubtitleCompositionCue(nil), primary...)}
	result.Provenance.Counts = map[SubtitleCompositionDecisionKind]int{}
	for _, source := range allSources {
		result.Provenance.SourceIdentities = append(result.Provenance.SourceIdentities, source.Identity)
		result.Provenance.SourceRevisions = append(result.Provenance.SourceRevisions, source.Revision)
		result.Provenance.Languages = appendUniqueString(result.Provenance.Languages, normalizedCompositionLanguage(source.Language))
		result.Provenance.Roles = appendUniqueRole(result.Provenance.Roles, source.Role)
		result.Provenance.Classes = appendUniqueClass(result.Provenance.Classes, source.CueClass)
	}
	if len(result.Provenance.Languages) == 1 {
		result.Provenance.ResolvedLanguage = result.Provenance.Languages[0]
	} else if len(result.Provenance.Languages) > 1 {
		result.Provenance.ResolvedLanguage = "mixed"
	}
	for index := range result.Events {
		result.Events[index].Class = normalizeCompositionCueClass(result.Events[index].Class, result.Events[index].Source.Role)
		result.Events[index].NormalizedText = normalizedCompositionText(result.Events[index].Event)
		result.Events[index].Layout = compositionLayoutForEvent(result.Events[index].Event)
	}
	orderedSupplementals := append([][]SubtitleCompositionCue(nil), supplementals...)
	if policy.CanonicalizeSourceOrder {
		sort.SliceStable(orderedSupplementals, func(i, j int) bool {
			left, right := "", ""
			if len(orderedSupplementals[i]) > 0 {
				left = orderedSupplementals[i][0].Source.Identity
			}
			if len(orderedSupplementals[j]) > 0 {
				right = orderedSupplementals[j][0].Source.Identity
			}
			return left < right
		})
	}
	for supplementalIndex, sourceCues := range orderedSupplementals {
		for _, incoming := range sourceCues {
			incoming.Class = normalizeCompositionCueClass(incoming.Class, incoming.Source.Role)
			incoming.NormalizedText = normalizedCompositionText(incoming.Event)
			incoming.Layout = compositionLayoutForEvent(incoming.Event)
			candidates := make([]int, 0, 2)
			for canonicalIndex, accepted := range result.Events {
				if compositionEquivalent(incoming, accepted, policy) {
					candidates = append(candidates, canonicalIndex)
				}
			}
			kind := CompositionDecisionAdded
			decision := SubtitleCompositionDecision{Kind: kind, SourceIdentity: incoming.Source.Identity, SourceOrdinal: incoming.Event.SourceOrdinal}
			if len(candidates) == 0 {
				decision.CanonicalIndex = len(result.Events)
				result.Events = append(result.Events, incoming)
			} else if len(candidates) > 1 {
				decision.Kind = CompositionDecisionAmbiguousConflict
				decision.Reason = "multiple eligible canonical events"
				result.Conflict = true
			} else {
				canonicalIndex := candidates[0]
				accepted := result.Events[canonicalIndex]
				decision.CanonicalIndex = canonicalIndex
				if compositionEventsProveDistinct(incoming, accepted) {
					decision.Kind = CompositionDecisionAdded
					decision.CanonicalIndex = len(result.Events)
					result.Events = append(result.Events, incoming)
				} else if !accepted.Source.Primary && compositionMeaningfulEvidence(accepted) && compositionMeaningfulEvidence(incoming) && accepted.Event.Style != incoming.Event.Style {
					decision.Kind = CompositionDecisionAmbiguousConflict
					decision.Reason = "incompatible meaningful supplemental evidence"
					result.Conflict = true
				} else if accepted.Source.Primary && compositionMeaningfulEvidence(accepted) {
					decision.Kind = CompositionDecisionKeptPrimary
				} else if !compositionMeaningfulEvidence(accepted) && compositionMeaningfulEvidence(incoming) {
					decision.Kind = CompositionDecisionEnriched
					result.Events[canonicalIndex] = mergeCompositionEvidence(accepted, incoming)
				} else {
					decision.Kind = CompositionDecisionDeduplicated
				}
			}
			result.Provenance.Counts[decision.Kind]++
			if len(result.Decisions) < compositionMaxProvenanceItems {
				result.Decisions = append(result.Decisions, decision)
			}
			if decision.Kind == CompositionDecisionAmbiguousConflict && len(result.Provenance.Conflicts) < compositionMaxProvenanceItems {
				result.Provenance.Conflicts = append(result.Provenance.Conflicts, fmt.Sprintf("source %s ordinal %d: %s", incoming.Source.Identity, incoming.Event.SourceOrdinal, decision.Reason))
			}
		}
		_ = supplementalIndex // the slice order is the frozen precedence order
	}
	result.Provenance.Decisions = append([]SubtitleCompositionDecision(nil), result.Decisions...)
	return result, nil
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func appendUniqueRole(values []SubtitleSemanticRole, value SubtitleSemanticRole) []SubtitleSemanticRole {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func appendUniqueClass(values []CompositionCueClass, value CompositionCueClass) []CompositionCueClass {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func compositionStyleID(policy SubtitleCompositionPolicy, cue SubtitleCompositionCue) string {
	if cue.Class == CompositionCueLyrics {
		if styleID := strings.TrimSpace(policy.ExplicitStyleByClass[CompositionCueLyrics]); styleID != "" {
			return styleID
		}
		return SubtitleStyleLyrics
	}
	return strings.TrimSpace(policy.ExplicitStyleByClass[cue.Class])
}

// resolveCompositionCueStyle intentionally calls the existing #123 resolver.
// It is the only composition/style boundary; no composition-specific style
// resolver is introduced here.
func resolveCompositionCueStyle(policy SubtitleCompositionPolicy, cue SubtitleCompositionCue) subtitleStyleResolution {
	explicitStyle := ""
	if policy.NativeStyleOverride || !compositionMeaningfulEvidence(cue) {
		explicitStyle = compositionStyleID(policy, cue)
	}
	return resolveSubtitleStyle(policy.StylePolicy, subtitleStyleResolutionInput{
		Role:                cue.Source.Role,
		ExplicitStyleID:     explicitStyle,
		ExplicitAlignment:   cue.Layout.ExplicitAlignment,
		HasExplicitPosition: cue.Layout.HasExplicitPosition,
	})
}

func subtitleCompositionPromotionAllowed(policy SubtitleCompositionPolicy, sources []SubtitleCompositionSource) bool {
	if !policy.PromotePreferred && !policy.PromoteDefault {
		return false
	}
	for _, source := range sources {
		if source.Primary && source.Role == SubtitleRoleFull && source.CueClass == CompositionCueDialogue {
			return true
		}
	}
	return false
}

func subtitleCompositionPromotion(policy SubtitleCompositionPolicy, sources []SubtitleCompositionSource) (preferred, defaultTrack bool) {
	if !subtitleCompositionPromotionAllowed(policy, sources) {
		return false, false
	}
	return policy.PromotePreferred, policy.PromoteDefault
}

func SubtitleCompositionOutputIdentity(sources []SubtitleCompositionSource, policy SubtitleCompositionPolicy, stylePolicyVersion, representation, publicationMode string) (string, error) {
	if err := validateSubtitleCompositionPolicy(policy); err != nil {
		return "", err
	}
	if len(sources) == 0 || len(sources)-1 > policy.MaxSupplementalSources {
		return "", fmt.Errorf("composition source count is outside policy bounds")
	}
	ordered := append([]SubtitleCompositionSource(nil), sources...)
	if policy.CanonicalizeSourceOrder && len(ordered) > 1 {
		primary := ordered[0]
		sort.SliceStable(ordered[1:], func(i, j int) bool { return ordered[i+1].Identity < ordered[j+1].Identity })
		ordered[0] = primary
	}
	payload := struct {
		Version            string                      `json:"version"`
		Sources            []SubtitleCompositionSource `json:"sources"`
		StylePolicyVersion string                      `json:"stylePolicyVersion"`
		Representation     string                      `json:"representation"`
		PublicationMode    string                      `json:"publicationMode"`
	}{policy.Version, ordered, stylePolicyVersion, strings.ToLower(strings.TrimSpace(representation)), strings.ToLower(strings.TrimSpace(publicationMode))}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return "subtitle-composite-v1-" + hex.EncodeToString(digest[:]), nil
}

func formatCompositionASSTimestamp(seconds float64) string {
	if seconds < 0 {
		seconds = 0
	}
	hours := int(seconds / 3600)
	seconds -= float64(hours * 3600)
	minutes := int(seconds / 60)
	seconds -= float64(minutes * 60)
	centiseconds := int(seconds*100 + 0.5)
	if centiseconds >= 6000 {
		minutes++
		centiseconds = 0
	}
	return fmt.Sprintf("%d:%02d:%02d.%02d", hours, minutes, centiseconds/100, centiseconds%100)
}

func canonicalASSFieldValue(event assEvent, field string) string {
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "start":
		return formatCompositionASSTimestamp(event.Start)
	case "end":
		return formatCompositionASSTimestamp(event.End)
	case "style":
		return event.Style
	case "text":
		var builder strings.Builder
		for _, part := range event.Dialogue.Parts {
			builder.WriteString(part.Text)
		}
		return builder.String()
	default:
		return ""
	}
}

// serializeASSDocument is the shared canonical writer for Translation and
// Composition. Composition uses it after reconciliation; it never creates a
// second serializer.
func serializeASSDocument(document assDocument) ([]byte, error) {
	format := append([]string(nil), document.EventFormat...)
	if len(format) == 0 {
		format = []string{"layer", "start", "end", "style", "name", "marginl", "marginr", "marginv", "effect", "text"}
	}
	var builder strings.Builder
	builder.WriteString("[Script Info]\nScriptType: v4.00+\n\n")
	builder.WriteString("[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n")
	for _, style := range document.Styles {
		builder.WriteString("Style: ")
		builder.WriteString(strings.Join(style.Fields, ","))
		builder.WriteByte('\n')
	}
	builder.WriteString("\n[Events]\nFormat: ")
	prettyFormat := make([]string, len(format))
	for i, field := range format {
		prettyFormat[i] = strings.TrimSpace(field)
		if prettyFormat[i] != "" {
			prettyFormat[i] = strings.ToUpper(prettyFormat[i][:1]) + prettyFormat[i][1:]
		}
	}
	builder.WriteString(strings.Join(prettyFormat, ", "))
	builder.WriteByte('\n')
	for _, event := range document.Events {
		fields := append([]string(nil), event.Fields...)
		if len(fields) != len(format) {
			fields = make([]string, len(format))
		}
		for index, field := range format {
			if value := canonicalASSFieldValue(event, field); value != "" || strings.EqualFold(field, "text") || strings.EqualFold(field, "style") {
				fields[index] = value
			}
		}
		builder.WriteString("Dialogue: ")
		builder.WriteString(strings.Join(fields, ","))
		builder.WriteByte('\n')
	}
	return []byte(builder.String()), nil
}

type SubtitleCompositionDocument struct {
	Source      SubtitleCompositionSource
	Document    assDocument
	Attachments []SubtitleCompositionAttachment
}

func normalizedASSStyleDefinition(style assStyleDefinition) string {
	return strings.ToLower(strings.Join(style.Fields, "\x1f"))
}

func compositionStyleNamespace(base string, source SubtitleCompositionSource, definition string) string {
	payload := source.Identity + "\x1f" + definition
	digest := sha256.Sum256([]byte(payload))
	name := strings.TrimSpace(base)
	if name == "" {
		name = "Style"
	}
	return fmt.Sprintf("%s__src_%s", name, hex.EncodeToString(digest[:])[:8])
}

func generatedCompositionASSStyle(policy SubtitleCompositionPolicy, styleID string) assStyleDefinition {
	preset, ok := subtitleStylePresetForID(policy.StylePolicy, styleID)
	if !ok {
		preset, _ = subtitleStylePresetForID(policy.StylePolicy, SubtitleStyleDefault)
		styleID = SubtitleStyleDefault
	}
	fields := []string{
		styleID, preset.FontFamily, strconv.Itoa(preset.FontSize), preset.PrimaryColor, preset.PrimaryColor,
		preset.OutlineColor, preset.ShadowBackColor, strconv.FormatBool(preset.Bold), "0", "1",
		strconv.Itoa(preset.OutlineWidth), strconv.Itoa(preset.ShadowDepth), strconv.Itoa(subtitleAlignmentForBehavior(preset.AlignmentBehavior)), "10", "10", "20", "1",
	}
	return assStyleDefinition{Section: "V4+ Styles", Format: []string{"name", "fontname", "fontsize", "primarycolour", "secondarycolour", "outlinecolour", "backcolour", "bold", "italic", "borderstyle", "outline", "shadow", "alignment", "marginl", "marginr", "marginv", "encoding"}, Name: styleID, Fields: fields}
}

// ComposeSubtitleDocuments imports all source documents into the shared ASS
// model, reconciles their events, resolves styles through #123, and serializes
// one canonical ASS document. It is the composition execution seam used by
// operations and focused tests.
func ComposeSubtitleDocuments(policy SubtitleCompositionPolicy, primary SubtitleCompositionDocument, supplementals ...SubtitleCompositionDocument) (assDocument, SubtitleCompositionResult, []byte, error) {
	if !primary.Source.Primary {
		return assDocument{}, SubtitleCompositionResult{}, nil, fmt.Errorf("composition document primary source is not marked primary")
	}
	primaryCues := subtitleCompositionCuesFromDocument(primary.Document, primary.Source)
	supplementalCues := make([][]SubtitleCompositionCue, len(supplementals))
	for index, source := range supplementals {
		if source.Source.Primary {
			return assDocument{}, SubtitleCompositionResult{}, nil, fmt.Errorf("supplemental composition source is marked primary")
		}
		supplementalCues[index] = subtitleCompositionCuesFromDocument(source.Document, source.Source)
	}
	result, err := ReconcileSubtitleComposition(policy, primaryCues, supplementalCues...)
	if err != nil {
		return assDocument{}, result, nil, err
	}
	if result.Conflict && !policy.AllowReviewableConflicts {
		return assDocument{}, result, nil, fmt.Errorf("composition contains unresolved event conflicts")
	}
	attachments := make([]SubtitleCompositionAttachment, 0)
	for _, document := range append([]SubtitleCompositionDocument{primary}, supplementals...) {
		attachments = append(attachments, document.Attachments...)
	}
	if len(attachments) > 0 {
		decisions, attachmentErr := reconcileSubtitleCompositionAttachments(attachments)
		if attachmentErr != nil {
			return assDocument{}, result, nil, attachmentErr
		}
		if len(decisions) > compositionMaxProvenanceItems {
			decisions = decisions[:compositionMaxProvenanceItems]
		}
		result.Provenance.AttachmentDecisions = append([]SubtitleCompositionAttachmentDecision(nil), decisions...)
		for _, decision := range decisions {
			if len(result.Provenance.AttachmentNotes) >= compositionMaxProvenanceItems {
				break
			}
			result.Provenance.AttachmentNotes = append(result.Provenance.AttachmentNotes, decision.Action+":"+decision.OriginalName+"->"+decision.SafeName)
		}
	}

	documents := append([]SubtitleCompositionDocument{primary}, supplementals...)
	styleBySource := map[string]map[string]string{}
	styles := []assStyleDefinition{}
	definitionByName := map[string]string{}
	for _, document := range documents {
		mapping := map[string]string{}
		for _, style := range document.Document.Styles {
			definition := normalizedASSStyleDefinition(style)
			name := style.Name
			if strings.HasPrefix(strings.ToLower(name), "mvf_") {
				name = compositionStyleNamespace(name, document.Source, definition)
			} else if existing, ok := definitionByName[name]; ok && existing != definition {
				name = compositionStyleNamespace(name, document.Source, definition)
			}
			if _, exists := definitionByName[name]; !exists {
				copyStyle := style
				copyStyle.Name = name
				copyStyle.Fields = append([]string(nil), style.Fields...)
				if len(copyStyle.Fields) > 0 {
					copyStyle.Fields[0] = name
				}
				styles = append(styles, copyStyle)
				definitionByName[name] = definition
			}
			mapping[style.Name] = name
		}
		styleBySource[document.Source.Identity] = mapping
	}

	output := assDocument{Representation: "ass", EventFormat: []string{"layer", "start", "end", "style", "name", "marginl", "marginr", "marginv", "effect", "text"}, Styles: styles}
	generatedStyleNames := map[string]struct{}{}
	for _, cue := range result.Events {
		event := cue.Event
		mapping := styleBySource[cue.Source.Identity]
		nativeStyle := false
		if mapped, ok := mapping[event.Style]; ok {
			event.Style = mapped
			nativeStyle = true
		}
		defaultStyle := strings.EqualFold(strings.TrimSpace(cue.Event.Style), "") || strings.EqualFold(strings.TrimSpace(cue.Event.Style), "default") || strings.EqualFold(strings.TrimSpace(cue.Event.Style), SubtitleStyleDefault)
		if !nativeStyle && (defaultStyle || !compositionMeaningfulEvidence(cue) || (cue.Class == CompositionCueLyrics && policy.NativeStyleOverride)) {
			resolution := resolveCompositionCueStyle(policy, cue)
			if defaultStyle || !compositionMeaningfulEvidence(cue) || (cue.Class == CompositionCueLyrics && policy.NativeStyleOverride) {
				event.Style = resolution.StyleID
				if _, exists := generatedStyleNames[resolution.StyleID]; !exists {
					styles = append(styles, generatedCompositionASSStyle(policy, resolution.StyleID))
					generatedStyleNames[resolution.StyleID] = struct{}{}
				}
			}
		}
		originalFields := append([]string(nil), event.Fields...)
		originalFormat := append([]string(nil), event.Format...)
		event.Format = append([]string(nil), output.EventFormat...)
		event.Fields = make([]string, len(output.EventFormat))
		for index, field := range output.EventFormat {
			switch strings.ToLower(strings.TrimSpace(field)) {
			case "start", "end", "style", "text":
				event.Fields[index] = canonicalASSFieldValue(event, field)
			default:
				for originalIndex, originalField := range originalFormat {
					if strings.EqualFold(strings.TrimSpace(originalField), strings.TrimSpace(field)) && originalIndex < len(originalFields) {
						event.Fields[index] = originalFields[originalIndex]
						break
					}
				}
			}
		}
		output.Events = append(output.Events, event)
	}
	output.Styles = styles
	styleNames := map[string]struct{}{}
	for _, style := range output.Styles {
		styleNames[style.Name] = struct{}{}
	}
	for _, event := range output.Events {
		if strings.TrimSpace(event.Style) != "" {
			if _, exists := styleNames[event.Style]; !exists {
				return assDocument{}, result, nil, fmt.Errorf("canonical composition event references unresolved style %q", event.Style)
			}
		}
	}
	encoded, err := serializeASSDocument(output)
	if err != nil {
		return assDocument{}, result, nil, err
	}
	validated, err := parseASSDocument("ass", string(encoded))
	if err != nil {
		return assDocument{}, result, nil, fmt.Errorf("canonical composition ASS failed validation: %w", err)
	}
	return validated, result, encoded, nil
}

func parseCompositionSRTTimestamp(value string) (float64, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, ",", "."))
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid SRT timestamp %q", value)
	}
	hours, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, err
	}
	minutes, err := strconv.ParseFloat(parts[1], 64)
	if err != nil || minutes < 0 || minutes >= 60 {
		return 0, fmt.Errorf("invalid SRT minute field")
	}
	seconds, err := strconv.ParseFloat(parts[2], 64)
	if err != nil || seconds < 0 || seconds >= 60 {
		return 0, fmt.Errorf("invalid SRT second field")
	}
	return hours*3600 + minutes*60 + seconds, nil
}

func parseCompositionSRT(source string) (assDocument, error) {
	lines := strings.Split(strings.ReplaceAll(strings.TrimPrefix(source, "\ufeff"), "\r\n", "\n"), "\n")
	document := assDocument{Representation: "ass", EventFormat: []string{"layer", "start", "end", "style", "name", "marginl", "marginr", "marginv", "effect", "text"}}
	for index := 0; index < len(lines); {
		for index < len(lines) && strings.TrimSpace(lines[index]) == "" {
			index++
		}
		if index >= len(lines) {
			break
		}
		if _, err := strconv.Atoi(strings.TrimSpace(lines[index])); err == nil {
			index++
		}
		if index >= len(lines) {
			break
		}
		timing := strings.Split(strings.TrimSpace(lines[index]), "-->")
		if len(timing) != 2 {
			return assDocument{}, fmt.Errorf("invalid SRT timing line")
		}
		start, err := parseCompositionSRTTimestamp(timing[0])
		if err != nil {
			return assDocument{}, err
		}
		end, err := parseCompositionSRTTimestamp(timing[1])
		if err != nil || end < start {
			return assDocument{}, fmt.Errorf("invalid SRT end timestamp")
		}
		index++
		textLines := []string{}
		for index < len(lines) && strings.TrimSpace(lines[index]) != "" {
			textLines = append(textLines, lines[index])
			index++
		}
		text := strings.Join(textLines, `\N`)
		dialogue, err := parseTranslationProtectedDialogue(text)
		if err != nil {
			return assDocument{}, err
		}
		document.Events = append(document.Events, assEvent{SourceOrdinal: len(document.Events), Start: start, End: end, Style: SubtitleStyleDefault, Format: append([]string(nil), document.EventFormat...), Fields: []string{"0", formatCompositionASSTimestamp(start), formatCompositionASSTimestamp(end), SubtitleStyleDefault, "", "0", "0", "0", "", text}, Dialogue: dialogue})
	}
	if len(document.Events) == 0 {
		return assDocument{}, fmt.Errorf("SRT document has no cues")
	}
	return document, nil
}

func subtitleCompositionCuesFromDocument(document assDocument, source SubtitleCompositionSource) []SubtitleCompositionCue {
	result := make([]SubtitleCompositionCue, 0, len(document.Events))
	for _, event := range document.Events {
		result = append(result, newSubtitleCompositionCue(event, source, source.CueClass))
	}
	return result
}

func subtitleCompositionCuesFromBitmap(source []canonicalBitmapSubtitleCue, descriptor SubtitleCompositionSource) []SubtitleCompositionCue {
	result := make([]SubtitleCompositionCue, 0, len(source))
	for _, cue := range source {
		dialogue, _ := parseTranslationProtectedDialogue(cue.Text)
		event := assEvent{SourceOrdinal: cue.SourceOrdinal, Start: cue.Start, End: cue.End, Style: SubtitleStyleDefault, Format: []string{"layer", "start", "end", "style", "name", "marginl", "marginr", "marginv", "effect", "text"}, Fields: []string{"0", formatCompositionASSTimestamp(cue.Start), formatCompositionASSTimestamp(cue.End), SubtitleStyleDefault, "", "0", "0", "0", "", cue.Text}, Dialogue: dialogue, Layout: compositionLayoutEvidence{HasBitmapGeometry: cue.Geometry != nil, Meaningful: cue.Geometry != nil, Strength: 1}}
		result = append(result, newSubtitleCompositionCue(event, descriptor, descriptor.CueClass))
	}
	return result
}

type SubtitleCompositionAttachment struct {
	SourceIdentity string
	OriginalName   string
	MIMEType       string
	Content        []byte
	Required       bool
}

type SubtitleCompositionAttachmentDecision struct {
	SourceIdentity string `json:"sourceIdentity"`
	OriginalName   string `json:"originalName"`
	SafeName       string `json:"safeName"`
	Digest         string `json:"digest"`
	Action         string `json:"action"`
}

func reconcileSubtitleCompositionAttachments(attachments []SubtitleCompositionAttachment) ([]SubtitleCompositionAttachmentDecision, error) {
	decisions := []SubtitleCompositionAttachmentDecision{}
	byDigest := map[string]string{}
	used := map[string]struct{}{}
	for _, attachment := range attachments {
		name := strings.TrimSpace(attachment.OriginalName)
		if name == "" || strings.ContainsAny(name, `/\\`) {
			if attachment.Required {
				return nil, fmt.Errorf("required attachment has no safe filename")
			}
			continue
		}
		digestBytes := sha256.Sum256(attachment.Content)
		digest := hex.EncodeToString(digestBytes[:])
		if existing, ok := byDigest[digest]; ok {
			decisions = append(decisions, SubtitleCompositionAttachmentDecision{SourceIdentity: attachment.SourceIdentity, OriginalName: name, SafeName: existing, Digest: digest, Action: "deduplicated"})
			continue
		}
		safeName := name
		if _, exists := used[safeName]; exists {
			extension := ""
			base := safeName
			if dot := strings.LastIndexByte(base, '.'); dot > 0 {
				extension, base = base[dot:], base[:dot]
			}
			safeName = fmt.Sprintf("%s-%s%s", base, digest[:8], extension)
			if _, exists := used[safeName]; exists {
				if attachment.Required {
					return nil, fmt.Errorf("required attachment filename conflict cannot be represented")
				}
				continue
			}
		}
		used[safeName] = struct{}{}
		byDigest[digest] = safeName
		decisions = append(decisions, SubtitleCompositionAttachmentDecision{SourceIdentity: attachment.SourceIdentity, OriginalName: name, SafeName: safeName, Digest: digest, Action: "added"})
	}
	return decisions, nil
}
