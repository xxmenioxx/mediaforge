package handlers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

type RestorationRecommendationState string

const (
	RestorationRecommendationRecommended   RestorationRecommendationState = "recommended"
	RestorationRecommendationManualReview  RestorationRecommendationState = "manual_review"
	RestorationRecommendationNotApplicable RestorationRecommendationState = "not_applicable"
	RestorationRecommendationNone          RestorationRecommendationState = "no_recommendation"
)

type RestorationRecommendationOutput struct {
	Width  int    `json:"width"`
	Height int    `json:"height"`
	SAR    string `json:"sar,omitempty"`
}

type RestorationRecommendation struct {
	ID                 string                           `json:"id"`
	Domain             string                           `json:"domain"`
	State              RestorationRecommendationState   `json:"state"`
	CurrentValue       string                           `json:"currentValue,omitempty"`
	RecommendedValue   string                           `json:"recommendedValue,omitempty"`
	Confidence         string                           `json:"confidence"`
	Reasons            []string                         `json:"reasons"`
	Warnings           []string                         `json:"warnings"`
	SupportingEvidence []string                         `json:"supportingEvidence"`
	Patch              models.JSONMap                   `json:"patch,omitempty"`
	ResolvedOutput     *RestorationRecommendationOutput `json:"resolvedOutput,omitempty"`
}

func (r RestorationRecommendation) Actionable() bool {
	return r.State == RestorationRecommendationRecommended && len(r.Patch) > 0
}

type RestorationRecommendationPlan struct {
	Version             int                         `json:"version"`
	ApplyLocked         bool                        `json:"applyLocked"`
	ApplyLockReason     string                      `json:"applyLockReason,omitempty"`
	Recommendations     []RestorationRecommendation `json:"recommendations"`
	RestorationEvidence RestorationAnalysis         `json:"restorationEvidence"`
}

func buildRestorationRecommendationPlan(scan models.ScanResult, proposal ProfileInput, applyLocked bool) RestorationRecommendationPlan {
	plan := RestorationRecommendationPlan{
		Version: 1, ApplyLocked: applyLocked,
		Recommendations:     make([]RestorationRecommendation, 0, 10),
		RestorationEvidence: decodeRestorationAnalysis(scan.RestorationAnalysis),
	}
	if applyLocked {
		plan.ApplyLockReason = "This asset has an active Queue job. Recommendations remain visible, but its frozen configuration cannot be changed."
	}

	profile := profileFromAdvisorProposal(proposal)
	profile.WorkerConfig = cloneWorkerConfig(profile.WorkerConfig)
	profile.WorkerConfig["fieldStructureMode"] = "auto"
	profile.WorkerConfig["cadenceMode"] = "auto"
	profile.WorkerConfig["deinterlaceMode"] = "auto"
	profile.WorkerConfig["cadenceFieldOrder"] = "auto"
	profile.WorkerConfig["deinterlaceFieldOrder"] = "auto"
	interlace, _ := decodeInterlaceAnalysis(scan.InterlaceAnalysis)
	cadence, _ := decodeCadenceAnalysis(scan.CadenceAnalysis)
	cadenceRecommendation, _ := decodeCadenceRecommendation(scan.CadenceRecommendation)
	resolvedMotion := resolveEffectiveVideoMotionProfile(profile, interlace, cadence, cadenceRecommendation)
	plan.Recommendations = append(plan.Recommendations, frameStructureRestorationRecommendation(interlace, resolvedMotion))

	upscaleRequest := cloneWorkerConfig(resolvedMotion.WorkerConfig)
	upscaleRequest["upscaleMode"] = string(UpscaleModeAuto)
	if strings.TrimSpace(workerStringValue(upscaleRequest["upscaleSharpen"])) == "" {
		upscaleRequest["upscaleSharpen"] = string(UpscaleSharpenOff)
	}
	delete(upscaleRequest, "upscaleCustomHeight")
	delete(upscaleRequest, "resolvedUpscaleDecision")
	resolvedMotion.WorkerConfig = upscaleRequest
	resolvedUpscale := resolveUpscaleProfile(resolvedMotion, mediaStreamInventoryFromScan(scan), upscaleAnalysisEvidence(scan))
	if decision, ok := resolvedUpscaleDecisionFromProfile(resolvedUpscale); ok {
		plan.Recommendations = append(plan.Recommendations, smartUpscaleRestorationRecommendation(*decision), smartUpscaleSharpenRecommendation(*decision))
	}

	evidence := plan.RestorationEvidence
	plan.Recommendations = append(plan.Recommendations,
		noAutomaticRestorationRecommendation("deflicker", "Deflicker", "No calibrated flicker analysis is available."),
		signalRestorationRecommendation("deblock", "Deblock", evidence.Blocking),
		combinedAmbiguousRecommendation("denoise", "Denoise", evidence.Noise, evidence.Grain),
		signalRestorationRecommendation("chroma_nr", "Chroma NR", evidence.ChromaNoise),
		signalRestorationRecommendation("deband", "Deband", evidence.Banding),
		signalRestorationRecommendation("ringing", "Ringing", evidence.Ringing),
		signalRestorationRecommendation("edge_detail", "Edge/detail confidence", evidence.EdgeDetail),
		noAutomaticRestorationRecommendation("exposure", "Exposure", "No calibrated brightness-correction advisor is available."),
		noAutomaticRestorationRecommendation("eq", "EQ / color adjustments", "No calibrated color-correction advisor is available."),
		noAutomaticRestorationRecommendation("color", "Color normalization", "No additional restoration color recommendation is available beyond the existing color-policy resolver."),
	)
	normalizeRestorationRecommendationPlan(&plan)
	return plan
}

func normalizeRestorationRecommendationPlan(plan *RestorationRecommendationPlan) {
	if plan == nil {
		return
	}
	if plan.Recommendations == nil {
		plan.Recommendations = []RestorationRecommendation{}
	}
	for index := range plan.Recommendations {
		recommendation := &plan.Recommendations[index]
		if recommendation.Reasons == nil {
			recommendation.Reasons = []string{}
		}
		if recommendation.Warnings == nil {
			recommendation.Warnings = []string{}
		}
		if recommendation.SupportingEvidence == nil {
			recommendation.SupportingEvidence = []string{}
		}
	}
	normalizeRestorationAnalysis(&plan.RestorationEvidence)
}

func annotateRestorationRecommendationCurrentProfile(plan *RestorationRecommendationPlan, profile models.Profile) {
	if plan == nil {
		return
	}
	worker := profile.WorkerConfig
	fieldMode := normalizedFieldStructureMode(workerStringValue(worker["fieldStructureMode"]))
	cadenceMode := normalizedCadenceMode(workerStringValue(worker["cadenceMode"]))
	legacy := strings.ToLower(strings.TrimSpace(workerStringValue(worker["deinterlaceMode"])))
	if fieldMode == "" {
		if legacy == "force" {
			fieldMode = "deinterlace"
		} else {
			fieldMode = "preserve"
		}
	}
	if cadenceMode == "" {
		if legacy == "ivtc_tff" || legacy == "ivtc_bff" {
			cadenceMode = "inverse_telecine"
		} else {
			cadenceMode = "preserve"
		}
	}
	upscaleMode := normalizedUpscaleMode(workerStringValue(worker["upscaleMode"]))
	if upscaleMode == "" {
		upscaleMode = string(UpscaleModeDisabled)
	}
	sharpenMode := normalizedUpscaleSharpen(workerStringValue(worker["upscaleSharpen"]))
	if sharpenMode == "" {
		sharpenMode = string(UpscaleSharpenOff)
	}
	for index := range plan.Recommendations {
		switch plan.Recommendations[index].ID {
		case "frame_structure":
			plan.Recommendations[index].CurrentValue = fieldMode + " / " + cadenceMode
		case "upscale":
			plan.Recommendations[index].CurrentValue = upscaleMode
		case "sharpen":
			plan.Recommendations[index].CurrentValue = sharpenMode
		}
	}
	annotateCurrentRestorationValues(plan.Recommendations, workerStringValue(worker["videoFilters"]))
}

func annotateCurrentRestorationValues(recommendations []RestorationRecommendation, filters string) {
	configured := map[string]bool{}
	for _, raw := range splitVideoFilterChain(filters) {
		name, _, _ := strings.Cut(strings.TrimSpace(raw), "=")
		configured[strings.ToLower(strings.TrimSpace(name))] = true
	}
	filterByID := map[string]string{"deflicker": "deflicker", "deblock": "deblock", "denoise": "hqdn3d", "chroma_nr": "chromanr", "deband": "deband", "exposure": "exposure", "eq": "eq"}
	for index := range recommendations {
		filter, ok := filterByID[recommendations[index].ID]
		if !ok {
			continue
		}
		if configured[filter] {
			recommendations[index].CurrentValue = "configured"
		} else {
			recommendations[index].CurrentValue = "off"
		}
	}
}

func profileFromAdvisorProposal(input ProfileInput) models.Profile {
	return models.Profile{
		Name: input.Name, Container: input.Container, VideoCodec: input.VideoCodec, CodecFamily: input.CodecFamily,
		EncoderPolicy: input.EncoderPolicy, PreferredEncoder: input.PreferredEncoder, AllowedEncoders: models.StringList(input.AllowedEncoders),
		FallbackPolicy: input.FallbackPolicy, BitDepth: input.BitDepth, PixelFormat: input.PixelFormat,
		QualityStrategy: input.QualityStrategy, OptimizationIntent: input.OptimizationIntent, AudioCodec: input.AudioCodec,
		QualityMode: input.QualityMode, QualityValue: input.QualityValue, PreserveHDR: input.PreserveHDR,
		WorkerConfig: cloneWorkerConfig(input.WorkerConfig),
	}
}

func frameStructureRestorationRecommendation(interlace InterlaceAnalysis, resolved models.Profile) RestorationRecommendation {
	confidence := confidenceLabel(interlace.Confidence)
	operation := strings.ToLower(workerStringValue(resolved.WorkerConfig["effectiveCadenceOperation"]))
	filters := strings.ToLower(workerStringValue(resolved.WorkerConfig["videoFilters"]))
	if operation == "inverse_telecine" || strings.Contains(filters, "fieldmatch") {
		patch := models.JSONMap{"fieldStructureMode": "preserve", "cadenceMode": "inverse_telecine", "deinterlaceMode": "off", "deinterlaceFieldOrder": "auto"}
		if order := normalizedRecommendationFieldOrder(interlace.DetectedFieldOrder, interlace.RecommendedMode); order != "" {
			patch["cadenceFieldOrder"] = order
			patch["deinterlaceMode"] = "ivtc_" + order
		} else {
			patch["cadenceFieldOrder"] = "auto"
		}
		return RestorationRecommendation{ID: "frame_structure", Domain: "Frame Structure", State: RestorationRecommendationRecommended, RecommendedValue: "ivtc", Confidence: confidence, Reasons: []string{"The authoritative cadence/frame-structure resolver selected inverse telecine."}, Patch: patch}
	}
	if strings.Contains(filters, "bwdif=") || strings.Contains(filters, "yadif=") {
		patch := models.JSONMap{"fieldStructureMode": "deinterlace", "cadenceMode": "preserve", "deinterlaceMode": "force", "cadenceFieldOrder": "auto"}
		if order := normalizedRecommendationFieldOrder(interlace.DetectedFieldOrder, interlace.RecommendedMode); order != "" {
			patch["deinterlaceFieldOrder"] = order
		} else {
			patch["deinterlaceFieldOrder"] = "auto"
		}
		return RestorationRecommendation{ID: "frame_structure", Domain: "Frame Structure", State: RestorationRecommendationRecommended, RecommendedValue: "deinterlace", Confidence: confidence, Reasons: []string{"The authoritative frame-structure resolver selected deinterlacing."}, Patch: patch}
	}
	if strings.EqualFold(interlace.Status, "progressive") {
		return RestorationRecommendation{ID: "frame_structure", Domain: "Frame Structure", State: RestorationRecommendationRecommended, RecommendedValue: "preserve_progressive", Confidence: confidence, Reasons: []string{"The authoritative frame-structure resolver classified the effective output as progressive."}, Patch: models.JSONMap{"fieldStructureMode": "preserve", "cadenceMode": "preserve", "deinterlaceMode": "off", "cadenceFieldOrder": "auto", "deinterlaceFieldOrder": "auto"}}
	}
	return RestorationRecommendation{ID: "frame_structure", Domain: "Frame Structure", State: RestorationRecommendationManualReview, CurrentValue: fallback(interlace.Status, "unknown"), Confidence: confidence, Reasons: []string{"Frame structure is not reliable enough for an automatic action."}}
}

func normalizedRecommendationFieldOrder(detected, mode string) string {
	value := strings.ToLower(strings.TrimSpace(detected))
	if value == "tff" || value == "bff" {
		return value
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if strings.HasSuffix(mode, "_bff") {
		return "bff"
	}
	if strings.HasSuffix(mode, "_tff") {
		return "tff"
	}
	return ""
}

func smartUpscaleRestorationRecommendation(decision ResolvedUpscaleDecision) RestorationRecommendation {
	value := string(decision.ResolvedMode)
	patch := models.JSONMap{"upscaleMode": string(decision.RequestedMode)}
	if decision.RequestedMode == UpscaleModeCustom && decision.TargetHeight > 0 {
		patch["upscaleCustomHeight"] = decision.TargetHeight
	}
	return RestorationRecommendation{
		ID: "upscale", Domain: "Smart Upscale", State: RestorationRecommendationRecommended,
		RecommendedValue: value, Confidence: string(decision.Confidence),
		Reasons: append([]string(nil), decision.Reasons...), Warnings: append([]string(nil), decision.Warnings...), Patch: patch,
		ResolvedOutput: &RestorationRecommendationOutput{Width: decision.TargetWidth, Height: decision.TargetHeight, SAR: decision.TargetSAR},
	}
}

func smartUpscaleSharpenRecommendation(decision ResolvedUpscaleDecision) RestorationRecommendation {
	if !decision.UpscaleApplied {
		return RestorationRecommendation{ID: "sharpen", Domain: "Final sharpen", State: RestorationRecommendationNotApplicable, Confidence: string(decision.Confidence), Reasons: []string{"Smart Upscale kept the source geometry, so no post-upscale sharpen is applicable."}}
	}
	if decision.SharpenMode == UpscaleSharpenOff {
		return RestorationRecommendation{ID: "sharpen", Domain: "Final sharpen", State: RestorationRecommendationNotApplicable, RecommendedValue: "off", Confidence: string(decision.Confidence), Reasons: []string{"The established Smart Upscale decision leaves post-scale sharpening off."}}
	}
	patch := models.JSONMap{"upscaleSharpen": string(decision.SharpenMode)}
	if decision.SharpenMode == UpscaleSharpenCustom {
		patch["upscaleSharpenCustomStrength"] = decision.SharpenStrength
	}
	return RestorationRecommendation{ID: "sharpen", Domain: "Final sharpen", State: RestorationRecommendationRecommended, RecommendedValue: string(decision.SharpenMode), Confidence: string(decision.Confidence), Reasons: []string{"This value comes from the established Smart Upscale resolver; restoration evidence did not increase it."}, Patch: patch}
}

func signalRestorationRecommendation(id, domain string, signal RestorationSignalEvidence) RestorationRecommendation {
	result := RestorationRecommendation{ID: id, Domain: domain, Confidence: fallback(signal.Confidence, "unavailable"), SupportingEvidence: append([]string(nil), signal.SupportingEvidence...)}
	switch strings.ToLower(strings.TrimSpace(signal.Availability)) {
	case "available":
		result.State = RestorationRecommendationManualReview
		result.Reasons = []string{fmt.Sprintf("%s evidence is available, but severity %q is not calibrated to restoration preset strengths.", domain, fallback(signal.Severity, "unclassified"))}
	case "ambiguous":
		result.State = RestorationRecommendationManualReview
		result.Reasons = []string{fmt.Sprintf("%s evidence is ambiguous and cannot safely select a restoration strength.", domain)}
	default:
		result.State = RestorationRecommendationNone
		result.Reasons = []string{fmt.Sprintf("%s analysis is unavailable; this is not evidence that the artifact is absent.", domain)}
	}
	return result
}

func combinedAmbiguousRecommendation(id, domain string, signals ...RestorationSignalEvidence) RestorationRecommendation {
	combined := RestorationSignalEvidence{Availability: "unavailable", Confidence: "unavailable"}
	for _, signal := range signals {
		combined.SupportingEvidence = append(combined.SupportingEvidence, signal.SupportingEvidence...)
		if signal.Availability == "ambiguous" {
			combined.Availability, combined.Confidence = "ambiguous", signal.Confidence
		} else if signal.Availability == "available" && combined.Availability == "unavailable" {
			combined.Availability, combined.Confidence, combined.Severity = signal.Availability, signal.Confidence, signal.Severity
		}
	}
	return signalRestorationRecommendation(id, domain, combined)
}

func noAutomaticRestorationRecommendation(id, domain, reason string) RestorationRecommendation {
	return RestorationRecommendation{ID: id, Domain: domain, State: RestorationRecommendationNone, Confidence: "unavailable", Reasons: []string{reason}}
}

func decodeRestorationAnalysis(value any) RestorationAnalysis {
	encoded, _ := json.Marshal(value)
	result := RestorationAnalysis{}
	_ = json.Unmarshal(encoded, &result)
	normalizeRestorationAnalysis(&result)
	return result
}

// applyRestorationRecommendations returns a new worker configuration and only
// applies explicitly selected actionable semantic patches. A nil selection
// means Apply All; manual-review and unavailable items are never applied.
func applyRestorationRecommendations(current models.JSONMap, plan RestorationRecommendationPlan, selected []string) models.JSONMap {
	result := cloneWorkerConfig(current)
	selectedSet := map[string]bool{}
	for _, id := range selected {
		selectedSet[id] = true
	}
	for _, recommendation := range plan.Recommendations {
		if !recommendation.Actionable() || (selected != nil && !selectedSet[recommendation.ID]) {
			continue
		}
		for key, value := range recommendation.Patch {
			result[key] = value
		}
	}
	return result
}
