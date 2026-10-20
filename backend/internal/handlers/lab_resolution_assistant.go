package handlers

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
)

const labResolutionDiagnosticVersion = 1

type labResolutionField struct {
	Field          string `json:"field"`
	Source         any    `json:"source,omitempty"`
	Expected       any    `json:"expected,omitempty"`
	Value          any    `json:"value,omitempty"`
	Authority      string `json:"authority,omitempty"`
	AuthorityState string `json:"authorityState"`
	Reason         string `json:"reason"`
}

type labResolutionAction struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Eligible    bool   `json:"eligible"`
	Recommended bool   `json:"recommended,omitempty"`
	Reason      string `json:"reason"`
	Tradeoffs   string `json:"tradeoffs,omitempty"`
}

type labResolutionDiagnostic struct {
	Version        int                   `json:"version"`
	Status         string                `json:"status"`
	PlanningStatus string                `json:"planningStatus"`
	BlockingFields []labResolutionField  `json:"blockingFields"`
	Evidence       models.JSONMap        `json:"evidence"`
	Requested      models.JSONMap        `json:"requested"`
	Effective      models.JSONMap        `json:"effective"`
	Actions        []labResolutionAction `json:"actions"`
}

func (err *frameStructureResolutionRequiredError) Error() string {
	return "frame structure GOP resolution requires a known effective output frame rate after an explicit FPS transform"
}

type frameStructureResolutionRequiredError struct{}

func isFrameStructureResolutionRequired(err error) bool {
	_, ok := err.(*frameStructureResolutionRequiredError)
	return ok
}

func buildLabResolutionDiagnostic(
	db *gorm.DB,
	path string,
	profile models.Profile,
	streams MediaStreamInventory,
	interlace InterlaceAnalysis,
	cadence CadenceAnalysis,
	recommendation CadenceRecommendation,
	sourceRevision compatiblePreviewSourceRevision,
) *labResolutionDiagnostic {
	if len(streams.Video) == 0 {
		return nil
	}
	stream := streams.Video[0]
	effective := resolveEffectiveVideoMotionProfile(profile, interlace, cadence, recommendation)
	worker := map[string]interface{}(effective.WorkerConfig)
	source := canonicalFidelitySource{effective: models.JSONMap{
		"width": stream.Width, "height": stream.Height,
		"sampleAspectRatio": stream.SampleAspectRatio, "displayAspectRatio": stream.DisplayAspectRatio,
		"frameRate": stream.FrameRate, "fieldStructure": normalizedFieldStructure(stream.FieldOrder),
		"pixelFormat": stream.PixelFormat, "bitDepth": bitDepthFromPixelFormat(stream.PixelFormat),
	}}
	expectations, _ := fidelityExpectedOutput(effective, source)
	blocking := make([]labResolutionField, 0, 3)
	for _, name := range []string{"frameRate", "fieldStructure"} {
		expectation := expectations[name]
		if expectation.state != fidelityAuthorityUnresolved && !expectation.unreliable && fidelityValuePresent(expectation.value) {
			continue
		}
		blocking = append(blocking, labResolutionField{
			Field: name, Source: source.effective[name], Expected: expectation.value,
			Authority: expectation.authority, AuthorityState: string(expectation.state), Reason: expectation.reason,
		})
	}
	blocking = append(blocking, labResolutionField{
		Field: "effectiveOutputFrameRate", Value: worker["effectiveOutputFrameRate"],
		Authority: "resolved_cadence", AuthorityState: "unresolved",
		Reason: "automatic GOP planning requires a known effective output frame rate",
	})

	evidence := models.JSONMap{
		"path": filepath.Clean(path), "sourceSizeBytes": sourceRevision.Size,
		"sourceModifiedAtNs": sourceRevision.ModifiedAtNS, "sourceMode": sourceRevision.Mode,
		"cadenceAnalysisVersion": cadence.Version,
		"sourceFrameRate":        stream.FrameRate,
	}
	if db != nil && db.Migrator().HasTable(&models.ScanResult{}) {
		var scan models.ScanResult
		if db.Where("path = ?", filepath.Clean(path)).Order("updated_at desc").First(&scan).Error == nil {
			evidence["scanId"] = scan.ID
			evidence["scanUpdatedAt"] = scan.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
		}
	}
	evidence["requestedProfileSignature"] = configurationHash(profile)
	evidence["profileDecisionSignature"] = configurationHash(effectiveVideoDecision(effective))

	fieldOrderValidated := validatedIVTCFieldOrder(interlace) != ""
	preserveEligible := labPreserveCandidateResolves(db, path, profile, streams, interlace, cadence, recommendation)
	deepCadenceEligible := recommendation.Operation == "inverse_telecine" && interlace.IVTCValidation != nil && strings.TrimSpace(interlace.IVTCValidation.SelectedOrder) == "" && deepCadenceActionEligible(db, path, sourceRevision)
	actions := []labResolutionAction{
		{ID: "reanalyze_cadence", Label: "Re-analyze cadence", Eligible: deepCadenceEligible,
			Recommended: deepCadenceEligible, Reason: "Deep Cadence is available only for ambiguous automatic IVTC with current snapshot identity."},
		{ID: "preserve_source", Label: "Preserve source cadence and fields", Eligible: preserveEligible,
			Recommended: !deepCadenceEligible && preserveEligible, Reason: "Preserve is eligible only when source cadence, structure, geometry, and FPS authority are independently supported and no transform conflicts.",
			Tradeoffs: "Telecine, judder, or combing may remain; Safe still depends on canonical Fidelity evidence."},
		{ID: "compare_field_order", Label: "Compare TFF and BFF", Eligible: recommendation.Operation == "inverse_telecine" && !fieldOrderValidated,
			Reason: "Field order is ambiguous; comparison previews require an explicit operator request."},
	}
	return &labResolutionDiagnostic{
		Version: labResolutionDiagnosticVersion, Status: "resolution_required", PlanningStatus: "NEEDS_REVIEW",
		BlockingFields: blocking, Evidence: evidence,
		Requested: models.JSONMap{
			"cadence":        workerStringValue(profile.WorkerConfig["cadenceMode"]),
			"fieldStructure": workerStringValue(profile.WorkerConfig["fieldStructureMode"]),
			"gop":            workerStringValue(profile.WorkerConfig["frameStructureGopMode"]),
			"gopStrategy":    workerStringValue(profile.WorkerConfig["frameStructureGopStrategy"]),
		},
		Effective: models.JSONMap{
			"frameRate":           worker["effectiveOutputFrameRate"],
			"frameRateKnown":      !profileWorkerBool(effective, "effectiveOutputFrameRateUnknown", false) && parseFrameRateValue(workerStringValue(worker["effectiveOutputFrameRate"])) > 0,
			"progressive":         worker["effectiveOutputProgressive"],
			"progressiveReliable": !profileWorkerBool(effective, "effectiveOutputProgressiveUnreliable", true),
			"cadenceOperation":    worker["effectiveCadenceOperation"],
			"fieldOrder":          worker["effectiveCadenceFieldOrder"],
		},
		Actions: actions,
	}
}

func labPreserveCandidateResolves(db *gorm.DB, path string, requested models.Profile, streams MediaStreamInventory, interlace InterlaceAnalysis, cadence CadenceAnalysis, recommendation CadenceRecommendation) bool {
	if len(streams.Video) == 0 || recommendCadence(cadence).Operation != "preserve" || cadence.EffectiveFPS <= 0 || !nearFPS(parseFrameRateValue(streams.Video[0].FrameRate), cadence.EffectiveFPS, .02) {
		return false
	}
	candidate := requested
	candidate.WorkerConfig = cloneWorkerConfig(requested.WorkerConfig)
	candidate.WorkerConfig["cadenceMode"] = "preserve"
	candidate.WorkerConfig["fieldStructureMode"] = "preserve"
	resolved, err := resolvePreviewVideoProfile(db, path, candidate, streams, interlace, cadence, recommendation)
	if err != nil || profileWorkerBool(resolved, "effectiveOutputFrameRateUnknown", true) || workerStringValue(resolved.WorkerConfig["effectiveCadenceOperation"]) != "preserve" {
		return false
	}
	if streams.Video[0].Width <= 0 || streams.Video[0].Height <= 0 || normalizedFieldStructure(streams.Video[0].FieldOrder) == "" {
		return false
	}
	source := canonicalFidelitySource{effective: models.JSONMap{
		"width": streams.Video[0].Width, "height": streams.Video[0].Height,
		"sampleAspectRatio": streams.Video[0].SampleAspectRatio, "displayAspectRatio": streams.Video[0].DisplayAspectRatio,
		"frameRate": streams.Video[0].FrameRate, "fieldStructure": normalizedFieldStructure(streams.Video[0].FieldOrder),
		"pixelFormat": streams.Video[0].PixelFormat, "bitDepth": bitDepthFromPixelFormat(streams.Video[0].PixelFormat),
	}}
	expectations, _ := fidelityExpectedOutput(resolved, source)
	for _, field := range []string{"frameRate", "fieldStructure", "width", "height"} {
		expectation := expectations[field]
		if expectation.state == fidelityAuthorityUnresolved || expectation.unreliable || !fidelityValuePresent(expectation.value) {
			return false
		}
	}
	return true
}

func deepCadenceActionEligible(db *gorm.DB, path string, revision compatiblePreviewSourceRevision) bool {
	if db == nil || !db.Migrator().HasTable(&models.ScanResult{}) {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() != revision.Size || info.ModTime().UnixNano() != revision.ModifiedAtNS || uint32(info.Mode()) != revision.Mode {
		return false
	}
	var scan models.ScanResult
	if db.Where("path = ?", filepath.Clean(path)).Order("updated_at desc, id desc").First(&scan).Error != nil {
		return false
	}
	policy := analysisPolicy(db)
	fingerprintMatches, legacy, staleComponents := snapshotCacheStateWithEvidence(scan, path, info, policy, normalizedAnalysisSeconds(0))
	if legacy || !fingerprintMatches || slices.Contains(staleComponents, "interlace") || slices.Contains(staleComponents, "cadence") {
		return false
	}
	var interlace InterlaceAnalysis
	var cadence CadenceAnalysis
	if !decodeJSONValue(scan.InterlaceAnalysis, &interlace) || interlace.IVTCValidation == nil || !decodeJSONValue(scan.CadenceAnalysis, &cadence) || recommendCadence(cadence).Operation != "inverse_telecine" || strings.TrimSpace(interlace.IVTCValidation.SelectedOrder) != "" {
		return false
	}
	if interlace.IVTCValidation.DeepAnalysisVersion == 0 {
		return true
	}
	if interlace.IVTCValidation.DeepAnalysisVersion != ivtcDeepEvidenceVersion {
		return false
	}
	evidenceCurrent := deepCadenceIdentityCurrent(db, scan, path, info, interlace.IVTCValidation.SourceIdentity) && interlace.IVTCValidation.EvidenceSnapshotUpdatedAt == scan.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	return !evidenceCurrent && resetStaleDeepIVTCEvidence(&interlace)
}
