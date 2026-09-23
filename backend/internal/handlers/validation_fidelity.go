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
type fidelityExpectation struct {
	value       any
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
		"effectiveOutputProgressive":       profileWorkerBool(profile, "effectiveOutputProgressive", false),
		"effectiveOutputWidth":             workerIntValue(worker["effectiveOutputWidth"], 0),
		"effectiveOutputHeight":            workerIntValue(worker["effectiveOutputHeight"], 0),
		"hevcLevel":                        stringFromUnknown(worker["hevcLevelEffective"]),
		"hevcTier":                         stringFromUnknown(worker["hevcLevelTier"]),
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

	set := func(name string, value any, authority, reason string, required bool) {
		applicable := fidelityValuePresent(value)
		expectations[name] = fidelityExpectation{value: value, authority: authority, reason: reason, applicable: applicable, required: required}
		expectedOutput[name] = value
	}

	width := workerIntValue(source.effective["width"], 0)
	height := workerIntValue(source.effective["height"], 0)
	sar := stringFromUnknown(source.effective["sampleAspectRatio"])
	dar := stringFromUnknown(source.effective["displayAspectRatio"])
	geometryAuthority, geometryReason := "preserved_source", "no frozen geometry transformation"
	if restoration, ok := resolvedRestorationPlanFromProfile(profile); ok && restoration.ResolvedOutput.Width > 0 && restoration.ResolvedOutput.Height > 0 {
		width, height, sar, dar = restoration.ResolvedOutput.Width, restoration.ResolvedOutput.Height, restoration.ResolvedOutput.SAR, restoration.ResolvedOutput.DAR
		geometryAuthority, geometryReason = "resolved_restoration", "resolved restoration output geometry"
	} else if upscale, ok := resolvedUpscaleDecisionFromProfile(profile); ok && upscale.UpscaleApplied {
		width, height, sar, dar = upscale.TargetWidth, upscale.TargetHeight, upscale.TargetSAR, aspectRatioString(upscale.TargetWidth, upscale.TargetHeight, upscale.TargetSAR)
		geometryAuthority, geometryReason = "resolved_upscale", "resolved Smart Upscale output geometry"
	} else if !profileWorkerBool(profile, "effectiveOutputGeometryUnknown", false) {
		if resolvedWidth := workerIntValue(worker["effectiveOutputWidth"], 0); resolvedWidth > 0 {
			width = resolvedWidth
			geometryAuthority, geometryReason = "effective_video_decision", "frozen effective video geometry"
		}
		if resolvedHeight := workerIntValue(worker["effectiveOutputHeight"], 0); resolvedHeight > 0 {
			height = resolvedHeight
			geometryAuthority, geometryReason = "effective_video_decision", "frozen effective video geometry"
		}
	}
	set("width", width, geometryAuthority, geometryReason, true)
	set("height", height, geometryAuthority, geometryReason, true)
	set("sampleAspectRatio", sar, geometryAuthority, geometryReason+" SAR", true)
	set("displayAspectRatio", dar, geometryAuthority, geometryReason+" DAR", true)

	if expectedRate := strings.TrimSpace(stringFromUnknown(worker["effectiveOutputFrameRate"])); expectedRate != "" && parseFrameRateValue(expectedRate) > 0 {
		set("frameRate", expectedRate, "resolved_cadence", cadenceReason(worker), true)
	} else {
		set("frameRate", source.effective["frameRate"], "preserved_source", "no frozen cadence transformation", true)
	}

	if cadenceOperation := strings.TrimSpace(stringFromUnknown(worker["effectiveCadenceOperation"])); cadenceOperation != "" || profileWorkerBool(profile, "effectiveOutputProgressive", false) {
		progressive := profileWorkerBool(profile, "effectiveOutputProgressive", false)
		value := "interlaced"
		if progressive {
			value = "progressive"
		}
		set("fieldStructure", value, "resolved_cadence", cadenceReason(worker), true)
	} else {
		set("fieldStructure", source.effective["fieldStructure"], "preserved_source", "no frozen field-structure transformation", true)
	}

	pixelValue, bitDepthValue, pixelAuthority, pixelReason, exactFormat := fidelityExpectedPixelSemantics(profile, source)
	if pixelValue != nil {
		set("pixelFormat", pixelValue, pixelAuthority, pixelReason, true)
		expectations["pixelFormat"] = fidelityExpectation{value: pixelValue, authority: pixelAuthority, reason: pixelReason, applicable: true, required: true, exactFormat: exactFormat}
	}
	set("bitDepth", bitDepthValue, pixelAuthority, pixelReason, true)
	if expectedChroma := stringFromUnknown(worker["effectiveChromaLocation"]); expectedChroma != "" {
		set("chromaLocation", strings.ToLower(expectedChroma), "resolved_video_decision", "frozen chroma-location decision", true)
	} else {
		// Chroma location is optional evidence. Preserve it when both sides expose
		// it, but do not let its absence poison an otherwise valid aggregate.
		set("chromaLocation", source.effective["chromaLocation"], "preserved_source", "no frozen chroma-location transformation", false)
	}

	if strings.EqualFold(profile.VideoCodec, "hevc") || strings.EqualFold(stringFromUnknown(worker["codec"]), "hevc") {
		set("hevcLevel", normalizedHEVCLevel(stringFromUnknown(worker["hevcLevelEffective"])), "resolved_hevc", "frozen HEVC Level decision", true)
		tier := strings.ToLower(strings.TrimSpace(stringFromUnknown(worker["hevcLevelTier"])))
		if tier == "" {
			tier = "main"
		}
		set("hevcTier", tier, "resolved_hevc", "frozen HEVC Tier decision", true)
	} else {
		expectations["hevcLevel"] = fidelityExpectation{authority: "resolved_hevc", reason: "HEVC Level is not applicable to this codec"}
		expectations["hevcTier"] = fidelityExpectation{authority: "resolved_hevc", reason: "HEVC Tier is not applicable to this codec"}
	}

	expectedOutput["workingPixelFormat"] = stringFromUnknown(worker["pixFmt"])
	expectedOutput["encoder"] = frozenFidelityEncoder(profile)
	expectedOutput["fieldStructure"] = expectations["fieldStructure"].value
	expectedOutput["effectiveCadenceOperation"] = stringFromUnknown(worker["effectiveCadenceOperation"])
	expectedOutput["effectiveCadenceFieldOrder"] = stringFromUnknown(worker["effectiveCadenceFieldOrder"])
	expectedOutput["effectiveResidualCombingFallback"] = stringFromUnknown(worker["effectiveResidualCombingFallback"])
	expectedOutput["effectiveAutomaticMotionFilters"] = stringFromUnknown(worker["effectiveAutomaticMotionFilters"])
	return expectations, expectedOutput
}

func fidelityExpectedPixelSemantics(profile models.Profile, source canonicalFidelitySource) (models.JSONMap, string, string, string, string) {
	worker := map[string]interface{}(profile.WorkerConfig)
	working := strings.ToLower(strings.TrimSpace(stringFromUnknown(worker["pixFmt"])))
	encoder := strings.ToLower(strings.TrimSpace(stringFromUnknown(worker["effectiveEncoder"])))
	if encoder == "" {
		encoder = strings.ToLower(strings.TrimSpace(stringFromUnknown(worker["videoEncoder"])))
	}
	if encoder == "" {
		encoder = strings.ToLower(strings.TrimSpace(stringFromUnknown(worker["encoder"])))
	}
	if encoder == "" {
		encoder = frozenFidelityEncoder(profile)
	}
	tenBit := profileUsesTenBit(profile)
	semantic := fidelityPixelSemantic(models.JSONMap{"pixelFormat": working, "bitDepth": bitDepthFromPixelFormat(working)})
	if semantic == nil && tenBit {
		semantic = models.JSONMap{"bitDepth": "10-bit", "chroma": "yuv420"}
	}
	if semantic == nil {
		semantic = fidelityPixelSemantic(source.effective)
	}
	if semantic == nil {
		return nil, "", "", "no authoritative pixel-format semantics", ""
	}
	if tenBit {
		semantic["bitDepth"] = "10-bit"
	}
	if (strings.EqualFold(profile.VideoCodec, "hevc") || strings.EqualFold(stringFromUnknown(worker["profile"]), "main10")) && tenBit {
		semantic["profile"] = "main10"
	} else if working == "" {
		delete(semantic, "profile")
	}
	isHardware := strings.Contains(encoder, "qsv") || strings.Contains(encoder, "videotoolbox")
	authority, reason := "preserved_source", "no frozen pixel-format transformation"
	exact := ""
	if working != "" {
		authority, reason = "resolved_encoder", "frozen encoder/profile pixel semantics"
		if !isHardware {
			exact = working
		}
	}
	return semantic, stringFromUnknown(semantic["bitDepth"]), authority, reason, exact
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
	result := models.JSONMap{"source": source, "expected": expectation.value, "actual": actual, "output": actual, "authority": expectation.authority, "reason": expectation.reason}
	if !expectation.applicable {
		result["status"] = "not_applicable"
		return result
	}
	if expectation.unreliable || !fidelityValuePresent(expectation.value) || !fidelityValuePresent(actual) {
		result["status"] = "unverified"
		result["required"] = expectation.required
		return result
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
	result := models.JSONMap{"source": source, "expected": models.JSONMap{"width": width.value, "height": height.value}, "actual": models.JSONMap{"width": actualWidth, "height": actualHeight}, "output": actual, "authority": width.authority, "reason": width.reason}
	if !width.applicable && !height.applicable {
		result["status"] = "not_applicable"
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
