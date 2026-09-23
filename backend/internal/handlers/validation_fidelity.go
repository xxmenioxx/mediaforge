package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"gorm.io/gorm"
)

// fidelityExpectation is the frozen contract for one observable output field.
// value is deliberately separate from the raw working/command representation:
// hardware encoders often expose a different spelling after decode.
type fidelityAuthorityState string

const (
	fidelityAuthorityTransform     fidelityAuthorityState = "transform"
	fidelityAuthorityPreserve      fidelityAuthorityState = "preserve"
	fidelityAuthorityUnresolved    fidelityAuthorityState = "unresolved"
	fidelityAuthorityNotApplicable fidelityAuthorityState = "not_applicable"
)

type fidelityExpectation struct {
	value       any
	state       fidelityAuthorityState
	authority   string
	reason      string
	applicable  bool
	required    bool
	unreliable  bool
	exactFormat string
}

type fidelityAuthority struct {
	profile     models.Profile
	worker      map[string]interface{}
	name        string
	diagnostics models.JSONList
}

type canonicalFidelitySource struct {
	raw       models.JSONMap
	effective models.JSONMap
}

// validateJobFrameFidelity preserves the Queue call shape while resolving the
// immutable Queue authority. Preview and Test Encode can call
// evaluateCanonicalFidelity directly with their final effective profile.
func validateJobFrameFidelity(db *gorm.DB, job models.QueueJob, sourceProbe, outputProbe map[string]any, hevcEvidence models.JSONMap, specialized ...models.JSONMap) models.JSONMap {
	return evaluateCanonicalFidelity(resolveFidelityAuthority(db, job), sourceProbe, outputProbe, hevcEvidence, specialized...)
}

// evaluateCanonicalFidelity is the shared surface-neutral evaluator. Its
// profile argument is already immutable effective state supplied by Queue,
// Preview, or Test Encode; it never consults mutable Profile/Override state.
func evaluateCanonicalFidelity(authority fidelityAuthority, sourceProbe, outputProbe map[string]any, hevcEvidence models.JSONMap, specialized ...models.JSONMap) models.JSONMap {
	sourceRaw := firstVideoFrameCharacteristics(sourceProbe)
	outputRaw := firstVideoFrameCharacteristics(outputProbe)
	source := canonicalizeFidelitySource(sourceRaw)
	actual := canonicalizeFidelityActual(outputRaw)
	expectations, expectedOutput := fidelityExpectedOutput(authority.profile, source)
	specializedReports := models.JSONMap{}

	fields := models.JSONMap{}
	for _, name := range []string{"width", "height", "sampleAspectRatio", "displayAspectRatio", "frameRate", "fieldStructure", "pixelFormat", "bitDepth", "chromaLocation"} {
		expectation := expectations[name]
		fields[name] = canonicalFidelityField(name, source.effective[name], expectation, fidelityActualValue(name, actual))
	}
	fields["frameSize"] = canonicalFidelityCompositeField(
		"frameSize",
		source.effective["frameSize"],
		expectations["width"],
		expectations["height"],
		actual.effective["frameSize"],
		actual.effective["width"],
		actual.effective["height"],
	)
	fields["fieldOrder"] = fields["fieldStructure"]

	for _, name := range []string{"hevcLevel", "hevcTier"} {
		expectation := expectations[name]
		fields[name] = canonicalFidelityField(name, source.effective[name], expectation, fidelityActualValueWithHEVCEvidence(name, hevcEvidence))
	}

	result := models.JSONMap{
		"source":               source.raw,
		"output":               outputRaw,
		"canonicalSource":      models.JSONMap{"raw": source.raw, "effective": source.effective},
		"expected":             expectedOutput,
		"actual":               actual.effective,
		"authority":            authority.name,
		"authorityDiagnostics": authority.diagnostics,
		"fields":               fields,
		"executionPlan":        frozenFidelityExecutionPlan(authority.profile),
		"workingPixelFormat":   stringFromUnknown(authority.worker["pixFmt"]),
	}

	if len(specialized) > 0 {
		specializedReports["colorPolicy"] = specialized[0]
		if color := specialized[0]; len(color) > 0 {
			if colorField := canonicalColorPolicyField(color); colorField != nil {
				fields["colorPolicy"] = colorField
			}
		}
	}
	if len(specialized) > 1 {
		specializedReports["smartUpscale"] = specialized[1]
	}
	if len(specialized) > 2 {
		specializedReports["restoration"] = specialized[2]
	}

	result["cadenceOutputFrameRate"] = validateCadenceOutputFrameRate(authority.worker, outputRaw)
	specializedReports["cadence"] = result["cadenceOutputFrameRate"]
	specializedReports["hevcLevel"] = validateHEVCLevelField(authority.worker, source.raw, outputRaw, hevcEvidence)
	result["specialized"] = specializedReports

	result["status"] = aggregateCanonicalFidelityStatus(fields)
	if result["status"] == "passed" {
		result["legacyStatus"] = "passed"
	} else {
		result["legacyStatus"] = "warning"
	}
	return result
}

func resolveFidelityAuthority(db *gorm.DB, job models.QueueJob) fidelityAuthority {
	authority := fidelityAuthority{name: "queue_profile_snapshot", diagnostics: models.JSONList{}}
	profile, restoreErr := scheduler.RestoreProfileSnapshot(job.ProfileSnapshot)
	if restoreErr == nil {
		authority.profile = profile
	} else if worker := unknownRecord(job.ProfileSnapshot["workerConfig"]); worker != nil {
		authority.name = "legacy_frozen_snapshot"
		authority.profile = models.Profile{WorkerConfig: models.JSONMap(worker)}
		authority.diagnostics = append(authority.diagnostics, "ProfileSnapshot could not be restored as a complete profile; using its frozen worker configuration.")
	} else {
		authority.name = "unverified_no_frozen_plan"
	}
	authority.worker = map[string]interface{}(authority.profile.WorkerConfig)

	candidates, err := readJobArtifactCandidates(db, job, "as-is")
	if err != nil {
		return authority
	}
	for _, candidate := range candidates {
		if !jobArtifactMatchesJob(candidate.artifact, job) {
			continue
		}
		verified, identity := verifiedFidelityArtifactIdentity(candidate.artifact, job)
		if !verified {
			authority.diagnostics = append(authority.diagnostics, "AS-IS artifact ignored: execution identity is ambiguous (latest by job/path is insufficient).")
			continue
		}
		artifactProfile, ok := fidelityProfileFromArtifact(candidate.artifact)
		if !ok {
			authority.diagnostics = append(authority.diagnostics, "AS-IS artifact ignored: verified execution identity had no usable frozen profile.")
			continue
		}
		if len(job.ProfileSnapshot) > 0 && !sameJSONValue(artifactProfile.WorkerConfig, authority.profile.WorkerConfig) {
			authority.diagnostics = append(authority.diagnostics, "verified AS-IS artifact disagrees with Queue ProfileSnapshot; artifact is execution authority")
		}
		authority.profile = artifactProfile
		authority.worker = map[string]interface{}(artifactProfile.WorkerConfig)
		authority.name = "verified_execution_artifact"
		authority.diagnostics = append(authority.diagnostics, "AS-IS artifact verified by "+identity+".")
		return authority
	}
	return authority
}

func verifiedFidelityArtifactIdentity(artifact map[string]any, job models.QueueJob) (bool, string) {
	if !jobArtifactMatchesJob(artifact, job) {
		return false, ""
	}
	plan := unknownRecord(artifact["executionPlan"])
	artifactPlanID := uintFromUnknown(firstKnownValue(plan, "id", "ID"))
	if job.ActiveExecutionPlanID != nil && artifactPlanID != 0 && artifactPlanID == *job.ActiveExecutionPlanID {
		return true, "ActiveExecutionPlanID"
	}
	rawJob := unknownRecord(artifact["job"])
	artifactActivePlanID := uintFromUnknown(firstKnownValue(rawJob, "activeExecutionPlanId", "activeExecutionPlanID"))
	if job.ActiveExecutionPlanID != nil && artifactActivePlanID != 0 && artifactActivePlanID == *job.ActiveExecutionPlanID {
		return true, "embedded job ActiveExecutionPlanID"
	}
	artifactSnapshot := unknownRecord(firstKnownValue(rawJob, "profileSnapshot", "ProfileSnapshot"))
	if len(job.ProfileSnapshot) > 0 && len(artifactSnapshot) > 0 && sameJSONValue(artifactSnapshot, job.ProfileSnapshot) {
		return true, "embedded frozen ProfileSnapshot"
	}
	return false, ""
}

func fidelityProfileFromArtifact(artifact map[string]any) (models.Profile, bool) {
	if profileRecord := unknownRecord(artifact["profile"]); profileRecord != nil {
		encoded, err := json.Marshal(profileRecord)
		if err == nil {
			var profile models.Profile
			if json.Unmarshal(encoded, &profile) == nil && len(profile.WorkerConfig) > 0 {
				return profile, true
			}
		}
	}
	if rawJob := unknownRecord(artifact["job"]); rawJob != nil {
		if snapshot := unknownRecord(firstKnownValue(rawJob, "profileSnapshot", "ProfileSnapshot")); snapshot != nil {
			if profile, err := scheduler.RestoreProfileSnapshot(models.JSONMap(snapshot)); err == nil {
				return profile, true
			}
		}
	}
	return models.Profile{}, false
}

func firstKnownValue(record map[string]interface{}, keys ...string) any {
	for _, key := range keys {
		if value, ok := record[key]; ok {
			return value
		}
	}
	return nil
}

func sameJSONValue(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func canonicalizeFidelitySource(raw models.JSONMap) canonicalFidelitySource {
	effective := copyFidelityMap(raw)
	effective["fieldStructure"] = normalizedFieldStructure(stringFromUnknown(raw["fieldOrder"]))
	effective["pixelSemantic"] = fidelityPixelSemantic(raw)
	return canonicalFidelitySource{raw: raw, effective: effective}
}

func canonicalizeFidelityActual(raw models.JSONMap) canonicalFidelitySource {
	effective := copyFidelityMap(raw)
	effective["fieldStructure"] = normalizedFieldStructure(stringFromUnknown(raw["fieldOrder"]))
	effective["pixelSemantic"] = fidelityPixelSemantic(raw)
	if measured, frameCount, duration, ok := measuredFrameRate(raw); ok {
		effective["measuredFrameRate"] = measured
		effective["measuredFrameCount"] = frameCount
		effective["measuredDuration"] = duration
	}
	return canonicalFidelitySource{raw: raw, effective: effective}
}

func copyFidelityMap(value models.JSONMap) models.JSONMap {
	result := models.JSONMap{}
	for key, item := range value {
		result[key] = item
	}
	return result
}

func frozenFidelityEncoder(profile models.Profile) string {
	worker := map[string]interface{}(profile.WorkerConfig)
	for _, key := range []string{"effectiveEncoder", "videoEncoder", "encoder"} {
		if encoder := strings.ToLower(strings.TrimSpace(stringFromUnknown(worker[key]))); encoder != "" && encoder != "auto" && encoder != "ffmpeg" {
			return encoder
		}
	}
	if profile.VideoCodec != "" {
		return ffmpegCodecName(profile.VideoCodec)
	}
	return ""
}

func frozenFidelityExecutionPlan(profile models.Profile) models.JSONMap {
	worker := map[string]interface{}(profile.WorkerConfig)
	plan := models.JSONMap{
		"encoder":                          frozenFidelityEncoder(profile),
		"codec":                            profile.VideoCodec,
		"workingPixelFormat":               stringFromUnknown(worker["pixFmt"]),
		"effectiveCadenceOperation":        stringFromUnknown(worker["effectiveCadenceOperation"]),
		"effectiveCadenceFieldOrder":       stringFromUnknown(worker["effectiveCadenceFieldOrder"]),
		"effectiveResidualCombingFallback": stringFromUnknown(worker["effectiveResidualCombingFallback"]),
		"effectiveAutomaticMotionFilters":  stringFromUnknown(worker["effectiveAutomaticMotionFilters"]),
		"effectiveOutputFrameRate":         stringFromUnknown(worker["effectiveOutputFrameRate"]),
		"effectiveOutputWidth":             workerIntValue(worker["effectiveOutputWidth"], 0),
		"effectiveOutputHeight":            workerIntValue(worker["effectiveOutputHeight"], 0),
		"hevcLevel":                        stringFromUnknown(worker["hevcLevelEffective"]),
		"hevcTier":                         stringFromUnknown(worker["hevcLevelTier"]),
	}
	if progressive, ok := fidelityWorkerBool(worker, "effectiveOutputProgressive"); ok {
		plan["effectiveOutputProgressive"] = progressive
	} else if _, present := worker["effectiveOutputProgressive"]; present {
		plan["effectiveOutputProgressive"] = nil
	}
	if upscale, ok := resolvedUpscaleDecisionFromProfile(profile); ok {
		plan["upscale"] = upscale
	}
	if restoration, ok := resolvedRestorationPlanFromProfile(profile); ok {
		plan["restoration"] = restoration
	}
	return plan
}

func fidelityExpectedOutput(profile models.Profile, source canonicalFidelitySource) (map[string]fidelityExpectation, models.JSONMap) {
	worker := map[string]interface{}(profile.WorkerConfig)
	expectations := map[string]fidelityExpectation{}
	expectedOutput := models.JSONMap{}

	set := func(name string, value any, state fidelityAuthorityState, authority, reason string, required, unreliable bool) {
		expectations[name] = fidelityExpectation{value: value, state: state, authority: authority, reason: reason, applicable: state != fidelityAuthorityNotApplicable, required: required, unreliable: unreliable}
		if state != fidelityAuthorityNotApplicable {
			expectedOutput[name] = value
		}
	}

	width, height := 0, 0
	sar, dar := "", ""
	geometryState := fidelityAuthorityNotApplicable
	geometryAuthority, geometryReason := "resolved_geometry", "no frozen geometry authority"
	if restoration, ok := resolvedRestorationPlanFromProfile(profile); ok && restoration.ResolvedOutput.Width > 0 && restoration.ResolvedOutput.Height > 0 {
		width, height, sar, dar = restoration.ResolvedOutput.Width, restoration.ResolvedOutput.Height, restoration.ResolvedOutput.SAR, restoration.ResolvedOutput.DAR
		geometryState, geometryAuthority, geometryReason = fidelityAuthorityTransform, "resolved_restoration", "resolved restoration output geometry"
	} else if upscale, ok := resolvedUpscaleDecisionFromProfile(profile); ok && upscale.UpscaleApplied {
		width, height, sar, dar = upscale.TargetWidth, upscale.TargetHeight, upscale.TargetSAR, aspectRatioString(upscale.TargetWidth, upscale.TargetHeight, upscale.TargetSAR)
		geometryState, geometryAuthority, geometryReason = fidelityAuthorityTransform, "resolved_upscale", "resolved Smart Upscale output geometry"
	} else if !profileWorkerBool(profile, "effectiveOutputGeometryUnknown", false) {
		resolvedWidth := workerIntValue(worker["effectiveOutputWidth"], 0)
		resolvedHeight := workerIntValue(worker["effectiveOutputHeight"], 0)
		if resolvedWidth > 0 && resolvedHeight > 0 {
			width, height = resolvedWidth, resolvedHeight
			sar, dar = stringFromUnknown(worker["effectiveOutputSAR"]), stringFromUnknown(worker["effectiveOutputDAR"])
			if dar == "" && sar != "" {
				dar = aspectRatioString(width, height, sar)
			}
			geometryState, geometryAuthority, geometryReason = fidelityAuthorityTransform, "effective_video_decision", "frozen effective video geometry"
		}
	}
	if geometryState == fidelityAuthorityNotApplicable {
		if upscale, ok := resolvedUpscaleDecisionFromProfile(profile); ok && upscale.ResolvedMode == ResolvedUpscaleKeepSource {
			width, height = workerIntValue(source.effective["width"], 0), workerIntValue(source.effective["height"], 0)
			sar, dar = stringFromUnknown(source.effective["sampleAspectRatio"]), stringFromUnknown(source.effective["displayAspectRatio"])
			geometryState, geometryAuthority, geometryReason = fidelityAuthorityPreserve, "resolved_upscale", "resolved Smart Upscale explicitly preserves source geometry"
		} else if restoration, ok := resolvedRestorationPlanFromProfile(profile); ok && restoration.ResolvedOutput.Width > 0 && restoration.ResolvedOutput.Height > 0 && restoration.ResolvedOutput == restoration.EffectiveGeometry {
			width, height, sar, dar = restoration.ResolvedOutput.Width, restoration.ResolvedOutput.Height, restoration.ResolvedOutput.SAR, restoration.ResolvedOutput.DAR
			geometryState, geometryAuthority, geometryReason = fidelityAuthorityPreserve, "resolved_restoration", "resolved restoration explicitly preserves effective geometry"
		}
	}
	set("width", widthOrNil(width), geometryState, geometryAuthority, geometryReason, true, false)
	set("height", widthOrNil(height), geometryState, geometryAuthority, geometryReason, true, false)
	sarState, darState := geometryState, geometryState
	if strings.TrimSpace(sar) == "" {
		sarState = fidelityAuthorityNotApplicable
	}
	if strings.TrimSpace(dar) == "" {
		darState = fidelityAuthorityNotApplicable
	}
	set("sampleAspectRatio", stringOrNil(sar), sarState, geometryAuthority, geometryReason+" SAR", true, false)
	set("displayAspectRatio", stringOrNil(dar), darState, geometryAuthority, geometryReason+" DAR", true, false)

	expectedRate := strings.TrimSpace(stringFromUnknown(worker["effectiveOutputFrameRate"]))
	rateState := fidelityAuthorityUnresolved
	rateAuthority, rateReason := "resolved_cadence", "frozen cadence output rate is unresolved"
	rateUnreliable := false
	if expectedRate != "" && parseFrameRateValue(expectedRate) > 0 && !profileWorkerBool(profile, "effectiveOutputFrameRateUnknown", false) && strings.TrimSpace(stringFromUnknown(worker["cadenceResolutionWarning"])) == "" {
		rateState, rateReason = fidelityAuthorityTransform, cadenceReason(worker)
	} else if fidelityCadenceExplicitPreserve(worker) {
		expectedRate = stringFromUnknown(source.effective["frameRate"])
		rateState, rateAuthority, rateReason = fidelityAuthorityPreserve, "resolved_cadence", "frozen cadence explicitly preserves source rate"
	} else {
		rateUnreliable = true
	}
	set("frameRate", stringOrNil(expectedRate), rateState, rateAuthority, rateReason, true, rateUnreliable)

	progressive, progressiveKnown := fidelityWorkerBool(worker, "effectiveOutputProgressive")
	progressiveUnreliable := profileWorkerBool(profile, "effectiveOutputProgressiveUnreliable", false)
	structureState := fidelityAuthorityUnresolved
	structureAuthority, structureReason := "resolved_cadence", "frozen cadence field structure is unresolved"
	structureUnreliable := !progressiveKnown || progressiveUnreliable
	structureValue := ""
	if progressiveKnown && !progressiveUnreliable {
		structureState, structureReason = fidelityAuthorityTransform, cadenceReason(worker)
		if progressive {
			structureValue = "progressive"
		} else {
			structureValue = "interlaced"
		}
	} else if fidelityCadenceExplicitPreserve(worker) {
		structureValue = stringFromUnknown(source.effective["fieldStructure"])
		structureState, structureReason, structureUnreliable = fidelityAuthorityPreserve, "frozen cadence explicitly preserves source field structure", false
	}
	set("fieldStructure", stringOrNil(structureValue), structureState, structureAuthority, structureReason, true, structureUnreliable)

	pixelValue, bitDepthValue, pixelState, pixelAuthority, pixelReason, exactFormat := fidelityExpectedPixelSemantics(profile, source)
	set("pixelFormat", pixelValue, pixelState, pixelAuthority, pixelReason, true, false)
	if pixelState != fidelityAuthorityNotApplicable {
		pixelExpectation := expectations["pixelFormat"]
		pixelExpectation.exactFormat = exactFormat
		expectations["pixelFormat"] = pixelExpectation
	}
	set("bitDepth", stringOrNil(bitDepthValue), pixelState, pixelAuthority, pixelReason, true, false)
	if expectedChroma := strings.ToLower(strings.TrimSpace(stringFromUnknown(worker["effectiveChromaLocation"]))); expectedChroma != "" {
		set("chromaLocation", expectedChroma, fidelityAuthorityTransform, "resolved_video_decision", "frozen chroma-location decision", true, false)
	} else if fidelityPixelExplicitPreserve(profile) {
		set("chromaLocation", source.effective["chromaLocation"], fidelityAuthorityPreserve, "resolved_encoder", "frozen copy/preserve plan preserves source chroma location", false, false)
	} else {
		set("chromaLocation", nil, fidelityAuthorityNotApplicable, "resolved_video_decision", "no frozen chroma-location authority", false, false)
	}

	hevc := videoCodecFamily(profile.VideoCodec) == "hevc" || videoCodecFamily(stringFromUnknown(worker["codec"])) == "hevc" || videoCodecFamily(frozenFidelityEncoder(profile)) == "hevc"
	if hevc {
		set("hevcLevel", normalizedHEVCLevel(stringFromUnknown(worker["hevcLevelEffective"])), fidelityAuthorityTransform, "resolved_hevc", "frozen HEVC Level decision", true, false)
		tier := strings.ToLower(strings.TrimSpace(stringFromUnknown(worker["hevcLevelTier"])))
		if tier == "" {
			tier = "main"
		}
		set("hevcTier", tier, fidelityAuthorityTransform, "resolved_hevc", "frozen HEVC Tier decision", true, false)
	} else {
		expectations["hevcLevel"] = fidelityExpectation{state: fidelityAuthorityNotApplicable, authority: "resolved_hevc", reason: "HEVC Level is not applicable to this codec"}
		expectations["hevcTier"] = fidelityExpectation{state: fidelityAuthorityNotApplicable, authority: "resolved_hevc", reason: "HEVC Tier is not applicable to this codec"}
	}

	expectedOutput["workingPixelFormat"] = stringFromUnknown(worker["pixFmt"])
	expectedOutput["encoder"] = frozenFidelityEncoder(profile)
	if fieldStructure := expectations["fieldStructure"]; fieldStructure.state != fidelityAuthorityNotApplicable {
		expectedOutput["fieldStructure"] = fieldStructure.value
	}
	expectedOutput["effectiveCadenceOperation"] = stringFromUnknown(worker["effectiveCadenceOperation"])
	expectedOutput["effectiveCadenceFieldOrder"] = stringFromUnknown(worker["effectiveCadenceFieldOrder"])
	expectedOutput["effectiveResidualCombingFallback"] = stringFromUnknown(worker["effectiveResidualCombingFallback"])
	expectedOutput["effectiveAutomaticMotionFilters"] = stringFromUnknown(worker["effectiveAutomaticMotionFilters"])
	return expectations, expectedOutput
}

func fidelityExpectedPixelSemantics(profile models.Profile, source canonicalFidelitySource) (models.JSONMap, string, fidelityAuthorityState, string, string, string) {
	worker := map[string]interface{}(profile.WorkerConfig)
	working := strings.ToLower(strings.TrimSpace(stringFromUnknown(worker["pixFmt"])))
	encoder := frozenFidelityEncoder(profile)
	tenBit := profileUsesTenBit(profile)
	semantic := fidelityPixelSemantic(models.JSONMap{"pixelFormat": working, "bitDepth": bitDepthFromPixelFormat(working)})
	resolvedSemantic := semantic != nil
	if semantic == nil && tenBit && (videoCodecFamily(profile.VideoCodec) == "hevc" || videoCodecFamily(encoder) == "hevc" || strings.EqualFold(stringFromUnknown(worker["profile"]), "main10")) {
		semantic = models.JSONMap{"bitDepth": "10-bit", "chroma": "yuv420"}
		resolvedSemantic = true
	}
	if semantic == nil {
		if fidelityPixelExplicitPreserve(profile) {
			semantic = fidelityPixelSemantic(source.effective)
		}
	}
	if semantic == nil {
		return nil, "", fidelityAuthorityNotApplicable, "resolved_encoder", "no frozen pixel-format preserve or encoder authority", ""
	}
	if tenBit {
		semantic["bitDepth"] = "10-bit"
	}
	if (videoCodecFamily(profile.VideoCodec) == "hevc" || videoCodecFamily(encoder) == "hevc" || strings.EqualFold(stringFromUnknown(worker["profile"]), "main10")) && tenBit {
		semantic["profile"] = "main10"
	}
	isHardware := strings.Contains(strings.ToLower(encoder), "qsv") || strings.Contains(strings.ToLower(encoder), "videotoolbox")
	state, authority, reason := fidelityAuthorityPreserve, "resolved_encoder", "frozen copy/preserve plan preserves source pixel semantics"
	exact := ""
	if resolvedSemantic {
		state, authority, reason = fidelityAuthorityTransform, "resolved_encoder", "frozen encoder/profile pixel semantics"
		if working != "" && !isHardware {
			exact = working
		}
	}
	return semantic, stringFromUnknown(semantic["bitDepth"]), state, authority, reason, exact
}

func fidelityWorkerBool(worker map[string]interface{}, key string) (bool, bool) {
	value, ok := worker[key]
	if !ok {
		return false, false
	}
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes", "on", "enabled":
			return true, true
		case "false", "0", "no", "off", "disabled":
			return false, true
		}
	}
	return false, false
}

func fidelityCadenceExplicitPreserve(worker map[string]interface{}) bool {
	for _, key := range []string{"effectiveCadenceOperation", "cadenceMode", "fieldStructureMode"} {
		value := strings.ToLower(strings.TrimSpace(stringFromUnknown(worker[key])))
		if value == "preserve" || value == "copy" || value == "passthrough" || value == "pass_through" {
			return true
		}
	}
	return false
}

func fidelityPixelExplicitPreserve(profile models.Profile) bool {
	worker := map[string]interface{}(profile.WorkerConfig)
	if videoCodecFamily(profile.VideoCodec) == "copy" {
		return true
	}
	for _, key := range []string{"videoEncoder", "effectiveEncoder", "encoder", "codec"} {
		if videoCodecFamily(stringFromUnknown(worker[key])) == "copy" {
			return true
		}
	}
	return false
}

func widthOrNil(value int) any {
	if value <= 0 {
		return nil
	}
	return value
}

func stringOrNil(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func fidelityPixelSemantic(characteristics models.JSONMap) models.JSONMap {
	pixel := strings.ToLower(strings.TrimSpace(stringFromUnknown(characteristics["pixelFormat"])))
	if pixel == "" {
		return nil
	}
	chroma := ""
	switch {
	case pixel == "nv12" || strings.HasPrefix(pixel, "p010") || strings.HasPrefix(pixel, "yuv420"):
		chroma = "yuv420"
	case strings.HasPrefix(pixel, "yuv422"):
		chroma = "yuv422"
	case strings.HasPrefix(pixel, "yuv444"):
		chroma = "yuv444"
	}
	if chroma == "" {
		return nil
	}
	bitDepth := strings.TrimSpace(stringFromUnknown(characteristics["bitDepth"]))
	if bitDepth == "" {
		bitDepth = bitDepthFromPixelFormat(pixel)
	}
	if bitDepth == "" {
		return nil
	}
	result := models.JSONMap{"bitDepth": bitDepth, "chroma": chroma}
	if profile := normalizedPixelProfile(stringFromUnknown(characteristics["profile"])); profile != "" {
		result["profile"] = profile
	}
	return result
}

func normalizedPixelProfile(profile string) string {
	profile = strings.ToLower(strings.TrimSpace(profile))
	profile = strings.ReplaceAll(profile, " ", "")
	profile = strings.ReplaceAll(profile, "-", "")
	switch profile {
	case "main10", "main10intra":
		return "main10"
	case "main":
		return "main"
	default:
		return profile
	}
}

func fidelityActualValue(name string, actual canonicalFidelitySource) any {
	if name == "frameRate" {
		if measured := actual.effective["measuredFrameRate"]; fidelityValuePresent(measured) {
			return measured
		}
	}
	if name == "fieldStructure" {
		return actual.effective["fieldStructure"]
	}
	if name == "pixelFormat" {
		semantic := actual.effective["pixelSemantic"]
		semanticMap := unknownRecord(semantic)
		if semanticMap == nil {
			return models.JSONMap{"raw": stringFromUnknown(actual.raw["pixelFormat"])}
		}
		value := cloneJSONMap(semanticMap)
		value["raw"] = stringFromUnknown(actual.raw["pixelFormat"])
		return value
	}
	return actual.effective[name]
}

func fidelityActualValueWithHEVCEvidence(name string, evidence models.JSONMap) any {
	if name == "hevcLevel" {
		return normalizedHEVCLevel(stringFromUnknown(evidence["generalLevel"]))
	}
	return strings.ToLower(strings.TrimSpace(stringFromUnknown(evidence["generalTier"])))
}

func canonicalFidelityField(name string, source any, expectation fidelityExpectation, actual any) models.JSONMap {
	expected := any(nil)
	if fidelityValuePresent(expectation.value) {
		expected = expectation.value
	}
	result := models.JSONMap{"source": source, "expected": expected, "actual": actual, "output": actual, "status": "not_applicable", "authority": expectation.authority, "authorityState": string(expectation.state), "reason": expectation.reason}
	if !expectation.applicable || expectation.state == fidelityAuthorityNotApplicable {
		result["status"] = "not_applicable"
		return result
	}
	if expectation.unreliable || expectation.state == fidelityAuthorityUnresolved || !fidelityValuePresent(expectation.value) || !fidelityValuePresent(actual) {
		result["status"] = "unverified"
		result["required"] = expectation.required
		return result
	}
	if name == "pixelFormat" {
		expectedMap, actualMap := unknownRecord(expectation.value), unknownRecord(actual)
		if expectedMap != nil && fidelityValuePresent(expectedMap["profile"]) && (actualMap == nil || !fidelityValuePresent(actualMap["profile"])) {
			result["status"] = "unverified"
			result["required"] = expectation.required
			return result
		}
	}
	if name == "pixelFormat" && expectation.exactFormat != "" {
		actualMap := unknownRecord(actual)
		if actualMap == nil || !strings.EqualFold(expectation.exactFormat, stringFromUnknown(actualMap["raw"])) {
			result["status"] = "mismatch"
			result["required"] = expectation.required
			return result
		}
	} else if !canonicalFidelityValuesEqual(name, expectation.value, actual) {
		result["status"] = "mismatch"
		result["required"] = expectation.required
		return result
	}
	result["status"] = "passed"
	result["required"] = expectation.required
	if fidelityValuePresent(source) && !canonicalFidelityValuesEqual(name, source, expectation.value) {
		result["changed_intentionally"] = true
	}
	return result
}

func canonicalFidelityCompositeField(name string, source any, width, height fidelityExpectation, actual any, actualWidth, actualHeight any) models.JSONMap {
	result := models.JSONMap{"source": source, "expected": models.JSONMap{"width": width.value, "height": height.value}, "actual": models.JSONMap{"width": actualWidth, "height": actualHeight}, "output": actual, "status": "not_applicable", "authority": width.authority, "authorityState": string(width.state), "reason": width.reason}
	if (!width.applicable || width.state == fidelityAuthorityNotApplicable) && (!height.applicable || height.state == fidelityAuthorityNotApplicable) {
		result["status"] = "not_applicable"
		return result
	}
	if width.unreliable || height.unreliable || width.state == fidelityAuthorityUnresolved || height.state == fidelityAuthorityUnresolved || !fidelityValuePresent(width.value) || !fidelityValuePresent(height.value) {
		result["status"] = "unverified"
		result["required"] = width.required || height.required
		return result
	}
	if !fidelityValuePresent(actualWidth) || !fidelityValuePresent(actualHeight) {
		result["status"] = "unverified"
		result["required"] = width.required || height.required
		return result
	}
	if !canonicalFidelityValuesEqual("width", width.value, actualWidth) || !canonicalFidelityValuesEqual("height", height.value, actualHeight) {
		result["status"] = "mismatch"
	} else {
		result["status"] = "passed"
	}
	result["required"] = width.required || height.required
	return result
}

func canonicalFidelityValuesEqual(name string, expected, actual any) bool {
	switch name {
	case "width", "height":
		return workerIntValue(expected, 0) > 0 && workerIntValue(expected, 0) == workerIntValue(actual, 0)
	case "bitDepth":
		return strings.EqualFold(strings.TrimSpace(stringFromUnknown(expected)), strings.TrimSpace(stringFromUnknown(actual)))
	case "frameRate":
		expectedRate, actualRate := parseFrameRateValue(stringFromUnknown(expected)), parseFrameRateValue(stringFromUnknown(actual))
		if expectedRate <= 0 {
			expectedRate = jsonFloat64(expected)
		}
		if actualRate <= 0 {
			actualRate = jsonFloat64(actual)
		}
		return expectedRate > 0 && actualRate > 0 && nearFPS(expectedRate, actualRate, 0.01)
	case "sampleAspectRatio":
		return ratioValue(stringFromUnknown(expected)) > 0 && nearFPS(ratioValue(stringFromUnknown(expected)), ratioValue(stringFromUnknown(actual)), 0.000001)
	case "displayAspectRatio":
		return ratioValue(stringFromUnknown(expected)) > 0 && nearFPS(ratioValue(stringFromUnknown(expected)), ratioValue(stringFromUnknown(actual)), 0.01)
	case "fieldStructure":
		return strings.EqualFold(stringFromUnknown(expected), stringFromUnknown(actual))
	case "chromaLocation", "hevcLevel":
		if name == "hevcLevel" {
			return normalizedHEVCLevel(stringFromUnknown(expected)) == normalizedHEVCLevel(stringFromUnknown(actual))
		}
		return strings.EqualFold(strings.TrimSpace(stringFromUnknown(expected)), strings.TrimSpace(stringFromUnknown(actual)))
	case "hevcTier":
		return strings.EqualFold(strings.TrimSpace(stringFromUnknown(expected)), strings.TrimSpace(stringFromUnknown(actual)))
	case "pixelFormat":
		expectedMap, actualMap := unknownRecord(expected), unknownRecord(actual)
		if expectedMap == nil || actualMap == nil {
			return false
		}
		if !strings.EqualFold(stringFromUnknown(expectedMap["bitDepth"]), stringFromUnknown(actualMap["bitDepth"])) || strings.ToLower(stringFromUnknown(expectedMap["chroma"])) != strings.ToLower(stringFromUnknown(actualMap["chroma"])) {
			return false
		}
		expectedProfile, actualProfile := strings.ToLower(stringFromUnknown(expectedMap["profile"])), strings.ToLower(stringFromUnknown(actualMap["profile"]))
		return expectedProfile == "" || actualProfile == "" || expectedProfile == actualProfile
	default:
		return strings.EqualFold(strings.TrimSpace(stringFromUnknown(expected)), strings.TrimSpace(stringFromUnknown(actual)))
	}
}

func canonicalColorPolicyField(report models.JSONMap) models.JSONMap {
	status := strings.ToLower(strings.TrimSpace(stringFromUnknown(report["status"])))
	if status != "passed" && status != "mismatch" && status != "unverified" {
		return nil
	}
	return models.JSONMap{
		"source":    report["source"],
		"expected":  report["expected"],
		"actual":    report["output"],
		"output":    report["output"],
		"status":    status,
		"authority": "color_policy",
		"reason":    "existing color-policy validator result",
		"required":  true,
	}
}

func aggregateCanonicalFidelityStatus(fields models.JSONMap) string {
	applicable := 0
	unverified := false
	for _, raw := range fields {
		field := unknownRecord(raw)
		if field == nil {
			continue
		}
		switch stringFromUnknown(field["status"]) {
		case "mismatch":
			return "mismatch"
		case "passed":
			applicable++
		case "unverified":
			if boolSetting(field["required"], false) {
				applicable++
				unverified = true
			}
		}
	}
	if unverified {
		return "unverified"
	}
	if applicable == 0 {
		return "not_applicable"
	}
	return "passed"
}

func normalizedFieldStructure(fieldOrder string) string {
	switch strings.ToLower(strings.TrimSpace(fieldOrder)) {
	case "progressive", "prog", "unknown_progressive":
		return "progressive"
	case "tt", "bb", "tb", "bt", "interlaced":
		return "interlaced"
	default:
		return ""
	}
}

func fidelityValuePresent(value any) bool {
	if value == nil {
		return false
	}
	switch typed := value.(type) {
	case models.JSONMap:
		return typed != nil && len(typed) > 0
	case map[string]interface{}:
		return typed != nil && len(typed) > 0
	}
	if record := unknownRecord(value); record != nil {
		return len(record) > 0
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case int:
		return typed > 0
	case float64:
		return typed > 0
	}
	return fmt.Sprint(value) != ""
}

func cadenceReason(worker map[string]interface{}) string {
	operation := strings.TrimSpace(stringFromUnknown(worker["effectiveCadenceOperation"]))
	if operation == "" {
		return "frozen effective output cadence"
	}
	return operation + " resolved output cadence"
}
