package handlers

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// SubtitleModeEligibility is backend-owned capability state. The UI may
// render the reason, but must not reconstruct source/provenance rules.
type SubtitleModeEligibility struct {
	Eligible bool   `json:"eligible"`
	Reason   string `json:"reason,omitempty"`
}

type SubtitleSourceEligibility struct {
	Restyle                      SubtitleModeEligibility `json:"restyle"`
	SRTToASS                     SubtitleModeEligibility `json:"srtToAss"`
	Regenerate                   SubtitleModeEligibility `json:"regenerate"`
	ReplaceEligible              bool                    `json:"replaceEligible"`
	NativeOverrideEligible       bool                    `json:"nativeOverrideEligible"`
	SourceIdentity               string                  `json:"sourceIdentity"`
	SourceRevision               string                  `json:"sourceRevision"`
	Representation               string                  `json:"representation"`
	Language                     string                  `json:"language"`
	Role                         SubtitleSemanticRole    `json:"role"`
	RoleProvenance               string                  `json:"roleProvenance"`
	StreamIndex                  *int                    `json:"streamIndex,omitempty"`
	PolicyVersion                string                  `json:"policyVersion"`
	CompositionClass             CompositionCueClass     `json:"compositionClass,omitempty"`
	CanonicalOCRArtifactIdentity string                  `json:"canonicalOcrArtifactIdentity,omitempty"`
	CanonicalOCRArtifactRevision string                  `json:"canonicalOcrArtifactRevision,omitempty"`
	CanonicalOCRArtifactPath     string                  `json:"canonicalOcrArtifactPath,omitempty"`
	CanonicalOCRLogicalTransform string                  `json:"canonicalOcrLogicalTransform,omitempty"`
}

type SubtitleCompositionCandidate struct {
	SourceIdentity               string               `json:"sourceIdentity"`
	SourceRevision               string               `json:"sourceRevision"`
	Representation               string               `json:"representation"`
	Language                     string               `json:"language"`
	Role                         SubtitleSemanticRole `json:"role"`
	CompositionClass             CompositionCueClass  `json:"compositionClass"`
	StreamIndex                  *int                 `json:"streamIndex,omitempty"`
	SourceType                   string               `json:"sourceType"`
	SourcePath                   string               `json:"sourcePath,omitempty"`
	CanonicalOCRArtifactIdentity string               `json:"canonicalOcrArtifactIdentity,omitempty"`
	CanonicalOCRArtifactRevision string               `json:"canonicalOcrArtifactRevision,omitempty"`
	CanonicalOCRArtifactPath     string               `json:"canonicalOcrArtifactPath,omitempty"`
	CanonicalOCRLogicalTransform string               `json:"canonicalOcrLogicalTransform,omitempty"`
	Eligible                     bool                 `json:"eligible"`
	Reason                       string               `json:"reason,omitempty"`
}

type SubtitleCompositionEligibility struct {
	Eligible               bool                           `json:"eligible"`
	Reason                 string                         `json:"reason,omitempty"`
	PolicyVersion          string                         `json:"policyVersion"`
	MaxSupplementals       int                            `json:"maxSupplementals"`
	PrimaryCandidates      []SubtitleCompositionCandidate `json:"primaryCandidates"`
	SupplementalCandidates []SubtitleCompositionCandidate `json:"supplementalCandidates"`
	CrossLanguageExplicit  bool                           `json:"crossLanguageExplicit"`
}

type SubtitleEligibilityResponse struct {
	External    []ExternalSubtitle              `json:"external"`
	Embedded    []SubtitleSourceEligibility     `json:"embedded"`
	Composition *SubtitleCompositionEligibility `json:"composition,omitempty"`
}

func subtitleExternalEligibility(sidecar ExternalSubtitle, policy SubtitleStylePolicy) SubtitleSourceEligibility {
	representation := strings.ToLower(strings.TrimSpace(sidecar.Format))
	identity := "external:" + sidecar.Path
	eligibleRevision := strings.TrimSpace(sidecar.SourceRevision) != ""
	role, roleProvenance := externalSubtitleCompositionRole(sidecar)
	result := SubtitleSourceEligibility{
		SourceIdentity: identity, SourceRevision: sidecar.SourceRevision,
		Representation: representation, Language: normalizedTrackLanguage(sidecar.Language), Role: role,
		RoleProvenance: roleProvenance, PolicyVersion: policy.Version,
	}
	result.CompositionClass = compositionClassForTrack(nil, result.Role)
	switch representation {
	case "ass", "ssa":
		result.Restyle = SubtitleModeEligibility{Eligible: eligibleRevision, Reason: eligibilityReason(eligibleRevision, "exact source revision is unavailable")}
		result.ReplaceEligible = eligibleRevision
		result.NativeOverrideEligible = eligibleRevision && policy.Settings.Enabled && policy.Settings.OverrideExistingASSStyles
		result.SRTToASS = SubtitleModeEligibility{Reason: "source is already ASS/SSA"}
		result.Regenerate = SubtitleModeEligibility{Reason: "external sidecars do not have embedded extraction provenance"}
	case "srt":
		result.SRTToASS = SubtitleModeEligibility{Eligible: eligibleRevision, Reason: eligibilityReason(eligibleRevision, "exact source revision is unavailable")}
		result.Restyle = SubtitleModeEligibility{Reason: "SRT is text/timing-only; generate an ASS sibling"}
		result.Regenerate = SubtitleModeEligibility{Reason: "external sidecars do not have embedded extraction provenance"}
	default:
		result.Restyle = SubtitleModeEligibility{Reason: "representation is unsupported"}
		result.SRTToASS = SubtitleModeEligibility{Reason: "representation is unsupported"}
		result.Regenerate = SubtitleModeEligibility{Reason: "representation is unsupported"}
	}
	return result
}

func externalSubtitleCompositionRole(sidecar ExternalSubtitle) (SubtitleSemanticRole, string) {
	base := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(sidecar.FileName)), filepath.Ext(strings.TrimSpace(sidecar.FileName)))
	for _, token := range strings.Split(base, ".") {
		if role := normalizeSubtitleSemanticRole(token); role != SubtitleRoleUnknown {
			return role, "external-sidecar-filename"
		}
	}
	if sidecar.Forced {
		return SubtitleRoleForced, "external-sidecar-disposition"
	}
	return SubtitleRoleFull, "external-sidecar-default"
}

func eligibilityReason(eligible bool, reason string) string {
	if eligible {
		return ""
	}
	return reason
}

func subtitleEmbeddedEligibility(path string, stream FFProbeStream, revision string, policy SubtitleStylePolicy) SubtitleSourceEligibility {
	representation := subtitleRepresentationForCodec(stream.CodecName)
	result := SubtitleSourceEligibility{
		SourceIdentity: fmt.Sprintf("embedded:%s#stream=%d", path, stream.Index),
		SourceRevision: revision, Representation: representation,
		Language: normalizedTrackLanguage(stream.Tags["language"]),
		Role:     SubtitleRoleUnknown, RoleProvenance: "embedded-stream; semantic role resolved at execution", PolicyVersion: policy.Version,
	}
	result.CompositionClass = compositionClassForTrack(stream.Tags, result.Role)
	streamIndex := stream.Index
	result.StreamIndex = &streamIndex
	for _, key := range []string{"role", "semanticRole", "subtitleRole"} {
		if role := normalizeSubtitleSemanticRole(stream.Tags[key]); role != "" {
			result.Role = role
			result.RoleProvenance = "metadata"
			break
		}
	}
	if representation == "" {
		result.Regenerate = SubtitleModeEligibility{Reason: "embedded subtitle codec is unsupported"}
		return result
	}
	if representation == "bitmap" {
		// Generic embedded regeneration remains eligible here for the existing
		// subtitle workflow. Composition applies the stricter canonical-OCR
		// boundary in SubtitleEligibility below.
		result.Regenerate = SubtitleModeEligibility{Eligible: true}
		result.Restyle = SubtitleModeEligibility{Reason: "embedded subtitles use canonical regeneration"}
		result.SRTToASS = SubtitleModeEligibility{Reason: "embedded subtitles use canonical regeneration"}
		return result
	}
	if strings.TrimSpace(revision) == "" {
		result.Regenerate = SubtitleModeEligibility{Reason: "exact media source revision is unavailable"}
		return result
	}
	result.Regenerate = SubtitleModeEligibility{Eligible: true}
	result.Restyle = SubtitleModeEligibility{Reason: "embedded subtitles use canonical regeneration"}
	result.SRTToASS = SubtitleModeEligibility{Reason: "embedded subtitles use canonical regeneration"}
	result.NativeOverrideEligible = (representation == "ass" || representation == "ssa") && policy.Settings.Enabled && policy.Settings.OverrideExistingASSStyles
	return result
}

func subtitleRepresentationForCodec(codec string) string {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "ass", "ssa":
		return strings.ToLower(strings.TrimSpace(codec))
	case "subrip", "srt":
		return "srt"
	case "hdmv_pgs_subtitle", "pgs", "dvd_subtitle", "vobsub":
		return "bitmap"
	default:
		return ""
	}
}

func (h AssetHandler) SubtitleEligibility(c *gin.Context) {
	mediaPath, _, ok := h.externalSubtitleAsset(c)
	if !ok {
		return
	}
	policy := subtitleStylingPolicyFromDB(h.db)
	external, err := externalSubtitlesForMedia(mediaPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for index := range external {
		eligibility := subtitleExternalEligibility(external[index], policy)
		external[index].Eligibility = &eligibility
	}

	revision, revisionErr := mediaFileFingerprint(mediaPath)
	if revisionErr != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "the exact media source revision is unavailable"})
		return
	}
	probe, _, probeErr := runFFProbe(mediaPath, 20)
	if probeErr != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": probeErr.Error()})
		return
	}
	embedded := make([]SubtitleSourceEligibility, 0)
	for _, stream := range probe.Streams {
		if strings.ToLower(strings.TrimSpace(stream.CodecType)) != "subtitle" {
			continue
		}
		eligibility := subtitleEmbeddedEligibility(mediaPath, stream, revision, policy)
		if eligibility.Representation == "bitmap" {
			if artifact, found := findPublishedCanonicalOCRArtifactForEmbeddedSource(mediaPath, stream); found {
				eligibility.Representation = artifact.Format
				eligibility.Regenerate = SubtitleModeEligibility{Eligible: true}
				eligibility.CanonicalOCRArtifactIdentity = artifact.Identity
				eligibility.CanonicalOCRArtifactRevision = artifact.Revision
				eligibility.CanonicalOCRArtifactPath = artifact.Path
				eligibility.CanonicalOCRLogicalTransform = artifact.LogicalTransform
			} else {
				eligibility.Regenerate = SubtitleModeEligibility{Reason: "current canonical OCR result for the exact source revision is required"}
			}
		}
		embedded = append(embedded, eligibility)
	}
	c.JSON(http.StatusOK, SubtitleEligibilityResponse{External: external, Embedded: embedded, Composition: subtitleCompositionEligibility(external, embedded, policy)})
}

func subtitleCompositionEligibility(external []ExternalSubtitle, embedded []SubtitleSourceEligibility, style SubtitleStylePolicy) *SubtitleCompositionEligibility {
	result := &SubtitleCompositionEligibility{PolicyVersion: compositionPolicyVersion, MaxSupplementals: compositionMaxSupplementals, PrimaryCandidates: []SubtitleCompositionCandidate{}, SupplementalCandidates: []SubtitleCompositionCandidate{}}
	add := func(candidate SubtitleCompositionCandidate) {
		if candidate.Language == "" || candidate.Language == "und" {
			return
		}
		if candidate.Role == SubtitleRoleFull {
			result.PrimaryCandidates = appendBoundedCompositionCandidate(result.PrimaryCandidates, candidate)
		}
		if candidate.SourceIdentity != "" {
			result.SupplementalCandidates = appendBoundedCompositionCandidate(result.SupplementalCandidates, candidate)
		}
	}
	for _, sidecar := range external {
		eligibility := sidecar.Eligibility
		if eligibility == nil {
			continue
		}
		candidate := SubtitleCompositionCandidate{SourceIdentity: eligibility.SourceIdentity, SourceRevision: eligibility.SourceRevision, Representation: eligibility.Representation, Language: normalizedTrackLanguage(sidecar.Language), Role: eligibility.Role, CompositionClass: eligibility.CompositionClass, SourceType: "external", SourcePath: sidecar.Path}
		candidate.Eligible = candidate.SourceRevision != "" && candidate.Language != "" && candidate.Language != "und" && candidate.Role != SubtitleRoleUnknown && (candidate.Representation == "ass" || candidate.Representation == "ssa" || candidate.Representation == "srt")
		if !candidate.Eligible {
			candidate.Reason = "exact language, revision, and supported representation are required"
		}
		add(candidate)
	}
	for _, eligibility := range embedded {
		candidate := SubtitleCompositionCandidate{SourceIdentity: eligibility.SourceIdentity, SourceRevision: eligibility.SourceRevision, Representation: eligibility.Representation, Language: normalizedTrackLanguage(eligibility.Language), Role: eligibility.Role, CompositionClass: eligibility.CompositionClass, StreamIndex: eligibility.StreamIndex, SourceType: "embedded", CanonicalOCRArtifactIdentity: eligibility.CanonicalOCRArtifactIdentity, CanonicalOCRArtifactRevision: eligibility.CanonicalOCRArtifactRevision, CanonicalOCRArtifactPath: eligibility.CanonicalOCRArtifactPath, CanonicalOCRLogicalTransform: eligibility.CanonicalOCRLogicalTransform}
		candidate.Eligible = eligibility.Regenerate.Eligible && candidate.Representation != "bitmap" && candidate.Language != "" && candidate.Language != "und" && candidate.Role != SubtitleRoleUnknown
		if !candidate.Eligible {
			candidate.Reason = eligibility.Regenerate.Reason
			if candidate.Representation == "bitmap" && candidate.Reason == "" {
				candidate.Reason = "current canonical OCR result for the exact source revision is required"
			}
			if candidate.Reason == "" {
				candidate.Reason = "backend role/language resolution or canonical source revision is unavailable"
			}
		}
		add(candidate)
	}
	hasEligiblePrimary := false
	for _, candidate := range result.PrimaryCandidates {
		if candidate.Eligible {
			hasEligiblePrimary = true
			break
		}
	}
	if !hasEligiblePrimary {
		result.Reason = "a resolved full-dialogue primary source is required"
		return result
	}
	hasEligibleSupplemental := false
	for _, candidate := range result.SupplementalCandidates {
		if candidate.Eligible {
			hasEligibleSupplemental = true
			break
		}
	}
	result.Eligible = hasEligibleSupplemental
	if !result.Eligible {
		result.Reason = "no compatible supplemental source is currently eligible"
	}
	_ = style // style version is carried by the existing eligibility response.
	return result
}

func appendBoundedCompositionCandidate(values []SubtitleCompositionCandidate, candidate SubtitleCompositionCandidate) []SubtitleCompositionCandidate {
	if len(values) >= compositionMaxSupplementals+1 {
		return values
	}
	return append(values, candidate)
}
