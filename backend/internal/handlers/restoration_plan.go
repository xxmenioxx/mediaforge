package handlers

import (
	"encoding/json"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

const (
	resolvedRestorationPlanKey       = "resolvedRestorationPlan"
	restorationAnalysisSnapshotKey   = "restorationAnalysisSnapshot"
	restorationProvenanceSnapshotKey = "restorationRecommendationProvenance"
)

type RestorationGeometry struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	SAR    string `json:"sar,omitempty"`
	DAR    string `json:"dar,omitempty"`
}

type ResolvedRestorationStage struct {
	Stage      string         `json:"stage"`
	Filter     string         `json:"filter"`
	Parameters models.JSONMap `json:"parameters,omitempty"`
}

// ResolvedRestorationPlan is the immutable, asset-specific execution contract.
// It records the canonical filters which Preview, Test Encode and Queue/Worker
// render; the Worker never runs the Advisor or reconstructs this from a live
// profile once the plan has been frozen in ProfileSnapshot.
type ResolvedRestorationPlan struct {
	Version                  int                        `json:"version"`
	RequestedFilterChain     string                     `json:"requestedFilterChain,omitempty"`
	ResolvedFilterChain      string                     `json:"resolvedFilterChain,omitempty"`
	Stages                   []ResolvedRestorationStage `json:"stages"`
	RequiresVideoEncode      bool                       `json:"requiresVideoEncode"`
	Executable               bool                       `json:"executable"`
	SourceStorage            RestorationGeometry        `json:"sourceStorage"`
	EffectiveGeometry        RestorationGeometry        `json:"effectiveGeometry"`
	ResolvedOutput           RestorationGeometry        `json:"resolvedOutput"`
	RecommendationProvenance models.JSONMap             `json:"recommendationProvenance,omitempty"`
	Evidence                 *RestorationAnalysis       `json:"restorationEvidence,omitempty"`
	Warnings                 []string                   `json:"warnings"`
}

func resolveRestorationPlan(profile models.Profile, source *MediaStream) models.Profile {
	profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
	if frozen, ok := resolvedRestorationPlanFromProfile(profile); ok {
		profile.WorkerConfig[resolvedRestorationPlanKey] = *frozen
		return profile
	}

	requested := workerStringValue(profile.WorkerConfig["videoFilters"])
	resolved := applyCropAspectPolicy(requested, profile, source)
	resolved = canonicalizeRestorationFilterChain(resolved)
	resolved = renderResolvedUpscaleFilters(resolved, profile)
	resolved = canonicalizeRestorationFilterChain(resolved)

	plan := ResolvedRestorationPlan{
		Version: 1, RequestedFilterChain: requested, ResolvedFilterChain: resolved,
		Stages:              restorationStagesFromFilterChain(resolved),
		RequiresVideoEncode: strings.TrimSpace(resolved) != "",
		Executable:          !strings.EqualFold(strings.TrimSpace(profile.VideoCodec), "copy"),
		Warnings:            []string{},
	}
	if source != nil {
		plan.SourceStorage = RestorationGeometry{Width: source.Width, Height: source.Height, SAR: source.SampleAspectRatio, DAR: source.DisplayAspectRatio}
	}
	if upscale, ok := resolvedUpscaleDecisionFromProfile(profile); ok {
		plan.EffectiveGeometry = RestorationGeometry{Width: upscale.SourceWidth, Height: upscale.SourceHeight, SAR: upscale.SourceSAR, DAR: upscale.SourceDAR}
		plan.ResolvedOutput = RestorationGeometry{Width: upscale.TargetWidth, Height: upscale.TargetHeight, SAR: upscale.TargetSAR, DAR: aspectRatioString(upscale.TargetWidth, upscale.TargetHeight, upscale.TargetSAR)}
	} else if source != nil {
		plan.EffectiveGeometry = plan.SourceStorage
		plan.ResolvedOutput = plan.SourceStorage
	}
	if raw := unknownRecord(profile.WorkerConfig[restorationProvenanceSnapshotKey]); raw != nil {
		plan.RecommendationProvenance = models.JSONMap(raw)
	}
	if raw, ok := profile.WorkerConfig[restorationAnalysisSnapshotKey]; ok {
		evidence := restorationEvidenceFromRaw(raw)
		plan.Evidence = &evidence
	}
	if plan.RequiresVideoEncode && !plan.Executable {
		plan.Warnings = append(plan.Warnings, "Restoration filters require video re-encoding; Video Codec is configured as Copy.")
	}
	profile.WorkerConfig[resolvedRestorationPlanKey] = plan
	return profile
}

func resolvedRestorationPlanFromProfile(profile models.Profile) (*ResolvedRestorationPlan, bool) {
	raw, ok := profile.WorkerConfig[resolvedRestorationPlanKey]
	if !ok || raw == nil {
		return nil, false
	}
	var plan ResolvedRestorationPlan
	encoded, err := json.Marshal(raw)
	if err != nil || json.Unmarshal(encoded, &plan) != nil || plan.Version != 1 {
		return nil, false
	}
	if plan.Stages == nil {
		plan.Stages = []ResolvedRestorationStage{}
	}
	if plan.Warnings == nil {
		plan.Warnings = []string{}
	}
	return &plan, true
}

func restorationFilterChainForCommand(profile models.Profile, source *MediaStream) string {
	if strings.EqualFold(strings.TrimSpace(profile.VideoCodec), "copy") {
		return ""
	}
	if frozen, ok := resolvedRestorationPlanFromProfile(profile); ok {
		return frozen.ResolvedFilterChain
	}
	resolved := resolveRestorationPlan(profile, source)
	if plan, ok := resolvedRestorationPlanFromProfile(resolved); ok {
		return plan.ResolvedFilterChain
	}
	return ""
}

func restorationStagesFromFilterChain(filters string) []ResolvedRestorationStage {
	result := make([]ResolvedRestorationStage, 0)
	for _, raw := range splitVideoFilterChain(filters) {
		filter := strings.TrimSpace(raw)
		if filter == "" {
			continue
		}
		stage, known := restorationStageForFilter(filter)
		stageName := "advanced"
		if known {
			stageName = restorationStageName(stage)
		}
		name, value, _ := strings.Cut(filter, "=")
		parameters := models.JSONMap{"name": strings.TrimSpace(name)}
		if strings.TrimSpace(value) != "" {
			parameters["value"] = value
		}
		result = append(result, ResolvedRestorationStage{Stage: stageName, Filter: filter, Parameters: parameters})
	}
	return result
}

func restorationStageName(stage restorationFilterStage) string {
	return []string{"motion", "deflicker", "deblock", "crop", "chroma_cleanup", "denoise", "deband", "image_adjustments", "color_normalization", "smart_upscale", "sar_normalization", "final_sharpen", "field_metadata"}[int(stage)]
}
