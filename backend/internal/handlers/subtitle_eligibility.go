package handlers

import (
	"fmt"
	"net/http"
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
	Restyle                SubtitleModeEligibility `json:"restyle"`
	SRTToASS               SubtitleModeEligibility `json:"srtToAss"`
	Regenerate             SubtitleModeEligibility `json:"regenerate"`
	ReplaceEligible        bool                    `json:"replaceEligible"`
	NativeOverrideEligible bool                    `json:"nativeOverrideEligible"`
	SourceIdentity         string                  `json:"sourceIdentity"`
	SourceRevision         string                  `json:"sourceRevision"`
	Representation         string                  `json:"representation"`
	Role                   SubtitleSemanticRole    `json:"role"`
	RoleProvenance         string                  `json:"roleProvenance"`
	StreamIndex            *int                    `json:"streamIndex,omitempty"`
	PolicyVersion          string                  `json:"policyVersion"`
}

type SubtitleEligibilityResponse struct {
	External []ExternalSubtitle          `json:"external"`
	Embedded []SubtitleSourceEligibility `json:"embedded"`
}

func subtitleExternalEligibility(sidecar ExternalSubtitle, policy SubtitleStylePolicy) SubtitleSourceEligibility {
	representation := strings.ToLower(strings.TrimSpace(sidecar.Format))
	identity := "external:" + sidecar.Path
	eligibleRevision := strings.TrimSpace(sidecar.SourceRevision) != ""
	result := SubtitleSourceEligibility{
		SourceIdentity: identity, SourceRevision: sidecar.SourceRevision,
		Representation: representation, Role: SubtitleRoleUnknown,
		RoleProvenance: "external-sidecar", PolicyVersion: policy.Version,
	}
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
		Role: SubtitleRoleUnknown, RoleProvenance: "embedded-stream; semantic role resolved at execution", PolicyVersion: policy.Version,
	}
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
		return "ass"
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
		embedded = append(embedded, subtitleEmbeddedEligibility(mediaPath, stream, revision, policy))
	}
	c.JSON(http.StatusOK, SubtitleEligibilityResponse{External: external, Embedded: embedded})
}
