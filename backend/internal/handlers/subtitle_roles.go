package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

type SubtitleSemanticRole string

const (
	SubtitleRoleFull       SubtitleSemanticRole = "full"
	SubtitleRoleForced     SubtitleSemanticRole = "forced"
	SubtitleRoleSigns      SubtitleSemanticRole = "signs"
	SubtitleRoleCommentary SubtitleSemanticRole = "commentary"
	SubtitleRoleSDH        SubtitleSemanticRole = "sdh"
	SubtitleRoleUnknown    SubtitleSemanticRole = "unknown"
)

type SubtitleRoleOverride struct {
	Role                      SubtitleSemanticRole `json:"role"`
	SourceSnapshotFingerprint string               `json:"sourceSnapshotFingerprint,omitempty"`
	StreamSignature           string               `json:"streamSignature,omitempty"`
}

type subtitleRoleDecision struct {
	Role                      SubtitleSemanticRole
	Provenance                string
	Confidence                string
	ClassifierVersion         string
	SourceSnapshotFingerprint string
	StreamSignature           string
	OverrideState             string
}

const subtitleSemanticRoleClassifierVersion = "subtitle-semantic-roles/v1"

var subtitleSemanticRoles = map[SubtitleSemanticRole]struct{}{
	SubtitleRoleFull: {}, SubtitleRoleForced: {}, SubtitleRoleSigns: {},
	SubtitleRoleCommentary: {}, SubtitleRoleSDH: {}, SubtitleRoleUnknown: {},
}

func normalizeSubtitleSemanticRole(value string) SubtitleSemanticRole {
	role := SubtitleSemanticRole(strings.ToLower(strings.TrimSpace(value)))
	if _, ok := subtitleSemanticRoles[role]; ok {
		return role
	}
	return ""
}

func subtitleSnapshotFingerprint(scan models.ScanResult) string {
	payload := struct {
		ID       uint
		Path     string
		Size     int64
		Duration float64
		Updated  int64
	}{scan.ID, scan.Path, scan.SizeBytes, scan.Duration, scan.UpdatedAt.UTC().UnixNano()}
	encoded, _ := json.Marshal(payload)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func subtitleStreamSignature(stream map[string]any) string {
	keys := []string{"index", "codec", "language", "title", "default", "forced", "comment", "commentary", "hearingImpaired", "order"}
	payload := make(map[string]any, len(keys))
	for _, key := range keys {
		if value, ok := stream[key]; ok {
			payload[key] = value
		}
	}
	encoded, _ := json.Marshal(payload)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func subtitleRoleOverrideFor(profile map[string]any, streamIndex int) (SubtitleRoleOverride, bool) {
	raw := profile["subtitleRoleOverrides"]
	if raw == nil {
		return SubtitleRoleOverride{}, false
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return SubtitleRoleOverride{}, false
	}
	var values map[string]SubtitleRoleOverride
	if err := json.Unmarshal(encoded, &values); err != nil {
		return SubtitleRoleOverride{}, false
	}
	override, ok := values[fmt.Sprintf("%d", streamIndex)]
	if !ok {
		return SubtitleRoleOverride{}, false
	}
	override.Role = normalizeSubtitleSemanticRole(string(override.Role))
	if override.Role == "" {
		return SubtitleRoleOverride{}, false
	}
	return override, true
}

func normalizedSubtitleRoleOverrides(raw map[int]SubtitleRoleOverride) map[int]SubtitleRoleOverride {
	if len(raw) == 0 {
		return nil
	}
	result := make(map[int]SubtitleRoleOverride, len(raw))
	for index, override := range raw {
		role := normalizeSubtitleSemanticRole(string(override.Role))
		if index < 0 || role == "" {
			continue
		}
		override.Role = role
		result[index] = override
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func bindSubtitleRoleOverrides(scan models.ScanResult, overrides map[int]SubtitleRoleOverride) map[int]SubtitleRoleOverride {
	if len(overrides) == 0 {
		return nil
	}
	snapshotFingerprint := subtitleSnapshotFingerprint(scan)
	streams := map[int]map[string]any{}
	for _, raw := range scan.SubtitleStreams {
		stream := settingProfileObject(raw)
		index := streamIndexValue(stream["index"])
		if index >= 0 {
			streams[index] = stream
		}
	}
	result := map[int]SubtitleRoleOverride{}
	for index, override := range normalizedSubtitleRoleOverrides(overrides) {
		stream, ok := streams[index]
		if !ok {
			continue
		}
		override.SourceSnapshotFingerprint = snapshotFingerprint
		override.StreamSignature = subtitleStreamSignature(stream)
		result[index] = override
	}
	return result
}

// resolveSubtitleRoleOverridesForUpdate treats a present request map as the
// complete current set of user overrides. An omitted map is handled by the
// caller as "leave the stored set unchanged". New/unbound entries are bound to
// the current scan; already-bound entries retain their original binding so an
// unrelated save cannot make a stale override current again.
func resolveSubtitleRoleOverridesForUpdate(scan models.ScanResult, existing, incoming map[int]SubtitleRoleOverride) map[int]SubtitleRoleOverride {
	if len(incoming) == 0 {
		return nil
	}
	incoming = normalizedSubtitleRoleOverrides(incoming)
	result := make(map[int]SubtitleRoleOverride, len(incoming))
	bound := bindSubtitleRoleOverrides(scan, incoming)
	for index, override := range incoming {
		if override.SourceSnapshotFingerprint != "" && override.StreamSignature != "" {
			result[index] = override
			continue
		}
		if prior, ok := existing[index]; ok && prior.SourceSnapshotFingerprint != "" &&
			prior.StreamSignature != "" && prior.Role == override.Role {
			result[index] = prior
			continue
		}
		if replacement, ok := bound[index]; ok {
			result[index] = replacement
		}
	}
	return result
}

func subtitleRoleOverridesForConversionUpdate(scan models.ScanResult, existing, incoming map[int]SubtitleRoleOverride, supplied bool) map[int]SubtitleRoleOverride {
	if !supplied {
		return existing
	}
	return resolveSubtitleRoleOverridesForUpdate(scan, existing, incoming)
}

func classifySubtitleRole(scan models.ScanResult, stream map[string]any, profile map[string]any) subtitleRoleDecision {
	snapshotFingerprint := subtitleSnapshotFingerprint(scan)
	streamSignature := subtitleStreamSignature(stream)
	decision := subtitleRoleDecision{
		Role:                      SubtitleRoleUnknown,
		Provenance:                "ambiguous/unknown",
		Confidence:                "low",
		ClassifierVersion:         subtitleSemanticRoleClassifierVersion,
		SourceSnapshotFingerprint: snapshotFingerprint,
		StreamSignature:           streamSignature,
		OverrideState:             "automatic",
	}
	if override, ok := subtitleRoleOverrideFor(profile, streamIndexValue(stream["index"])); ok {
		if override.SourceSnapshotFingerprint == snapshotFingerprint && override.StreamSignature == streamSignature {
			decision.Role = override.Role
			decision.Provenance = "manual"
			decision.Confidence = "high"
			decision.OverrideState = "manual"
			return decision
		}
		decision.OverrideState = "stale/ignored"
	}

	generatedRole := (workerStringValue(stream["roleSourceSnapshotFingerprint"]) != "" ||
		workerStringValue(stream["roleStreamSignature"]) != "") &&
		(workerStringValue(stream["roleProvenance"]) == "heuristic" ||
			workerStringValue(stream["roleProvenance"]) == "disposition" ||
			workerStringValue(stream["roleProvenance"]) == "ambiguous/unknown")
	if !generatedRole {
		for _, key := range []string{"role", "semanticRole", "subtitleRole"} {
			if role := normalizeSubtitleSemanticRole(workerStringValue(stream[key])); role != "" {
				decision.Role = role
				decision.Provenance = "metadata"
				decision.Confidence = "high"
				return decision
			}
		}
	}

	flags := []SubtitleSemanticRole{}
	// The source forced disposition is publication metadata, not a semantic
	// role. Keep it independent so a signs or SDH track can remain that role
	// while still receiving effective forced disposition at publication time.
	if boolValue(stream["comment"], false) || boolValue(stream["commentary"], false) {
		flags = append(flags, SubtitleRoleCommentary)
	}
	if boolValue(stream["hearingImpaired"], false) || boolValue(stream["hearing_impaired"], false) {
		flags = append(flags, SubtitleRoleSDH)
	}
	if boolValue(stream["forced"], false) {
		flags = append(flags, SubtitleRoleForced)
	}
	if len(flags) == 1 {
		decision.Role = flags[0]
		decision.Provenance = "disposition"
		decision.Confidence = "high"
		return decision
	}
	if len(flags) > 1 {
		return decision
	}

	title := strings.ToLower(strings.TrimSpace(workerStringValue(stream["title"])))
	heuristics := []SubtitleSemanticRole{}
	for _, candidate := range []struct {
		role  SubtitleSemanticRole
		terms []string
	}{
		{SubtitleRoleSigns, []string{"signs", "songs", "songs & signs", "songs and signs"}},
		{SubtitleRoleCommentary, []string{"commentary", "comment"}},
		{SubtitleRoleSDH, []string{"sdh", "hearing impaired", "hearing-impaired", "closed caption"}},
		{SubtitleRoleForced, []string{"forced"}},
		{SubtitleRoleFull, []string{"full", "dialogue"}},
	} {
		for _, term := range candidate.terms {
			if strings.Contains(title, term) {
				heuristics = append(heuristics, candidate.role)
				break
			}
		}
	}
	if len(heuristics) == 1 {
		decision.Role = heuristics[0]
		decision.Provenance = "heuristic"
		decision.Confidence = "medium"
		return decision
	}
	return decision
}

// canonicalEmbeddedSubtitleRoleSummary is the bounded stream evidence shared
// by Track resolution and subtitle operations. Composition must feed this
// same evidence into classifySubtitleRole rather than interpreting embedded
// metadata with a second role authority.
func canonicalEmbeddedSubtitleRoleSummary(stream FFProbeStream) map[string]any {
	if strings.TrimSpace(stream.CodecType) == "" {
		stream.CodecType = "subtitle"
	}
	summaries := streamSummaries([]FFProbeStream{stream}, "subtitle")
	if len(summaries) == 0 {
		return map[string]any{"index": stream.Index, "codec": stream.CodecName}
	}
	summary := settingProfileObject(summaries[0])
	if summary == nil {
		summary = map[string]any{}
	}
	for _, key := range []string{"role", "semanticRole", "subtitleRole", "roleProvenance", "roleConfidence", "roleClassifierVersion", "roleSourceSnapshotFingerprint", "roleStreamSignature", "compositionCueClass", "cueClass", "compositionClass"} {
		if value, ok := stream.Tags[key]; ok {
			summary[key] = value
		}
	}
	if _, ok := summary["commentary"]; !ok {
		summary["commentary"] = summary["comment"]
	}
	return summary
}

func classifyEmbeddedSubtitleRole(path string, stream FFProbeStream) subtitleRoleDecision {
	summary := canonicalEmbeddedSubtitleRoleSummary(stream)
	scan := models.ScanResult{Path: path, SubtitleStreams: models.JSONList{summary}}
	return classifySubtitleRole(scan, summary, nil)
}

func annotateSubtitleRoleSummaries(scan *models.ScanResult) {
	if scan == nil {
		return
	}
	for _, raw := range scan.SubtitleStreams {
		stream := settingProfileObject(raw)
		if stream == nil {
			continue
		}
		decision := classifySubtitleRole(*scan, stream, nil)
		stream["role"] = decision.Role
		stream["roleProvenance"] = decision.Provenance
		stream["roleConfidence"] = decision.Confidence
		stream["roleClassifierVersion"] = decision.ClassifierVersion
		stream["roleSourceSnapshotFingerprint"] = decision.SourceSnapshotFingerprint
		stream["roleStreamSignature"] = decision.StreamSignature
	}
}

func normalizedSubtitleRoleList(raw any) ([]SubtitleSemanticRole, bool) {
	if raw == nil {
		return nil, false
	}
	values := []SubtitleSemanticRole{}
	encoded, err := json.Marshal(raw)
	if err != nil || json.Unmarshal(encoded, &values) != nil {
		return nil, true
	}
	result := make([]SubtitleSemanticRole, 0, len(values))
	for _, value := range values {
		if role := normalizeSubtitleSemanticRole(string(value)); role != "" {
			result = append(result, role)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, true
}

func subtitleRuleMatchesRole(rule map[string]any, role SubtitleSemanticRole) bool {
	roles, configured := normalizedSubtitleRoleList(rule["roles"])
	if !configured {
		return true
	}
	for _, candidate := range roles {
		if candidate == role {
			return true
		}
	}
	return false
}

func subtitleRuleMatches(rule map[string]any, streamIndex int, language string, role SubtitleSemanticRole) (bool, bool) {
	rawIndex, hasIndex := rule["streamIndex"]
	index := streamIndexValue(rawIndex)
	streamSpecific := hasIndex && index >= 0
	if streamSpecific && index != streamIndex {
		return false, false
	}
	rawLanguage, hasLanguage := rule["language"]
	if hasLanguage && strings.TrimSpace(workerStringValue(rawLanguage)) != "" {
		if normalizedTrackLanguage(workerStringValue(rawLanguage)) != language {
			return false, false
		}
	} else if !streamSpecific {
		return false, false
	}
	if !subtitleRuleMatchesRole(rule, role) {
		return false, false
	}
	return true, streamSpecific
}
