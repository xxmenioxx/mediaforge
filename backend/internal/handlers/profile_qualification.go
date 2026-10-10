package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

const (
	profileQualificationPolicyVersion   = "profile-qualification-policy-v1"
	profileQualificationDefaultsSource  = "mvforge_policy_defaults"
	profileQualificationScope           = "asset_profile_candidate_evidence"
	profileQualificationMaximumWarnings = 4
)

type ProfileQualificationCandidate struct {
	SourceAssetID               uint
	SourcePath                  string
	SourceRevision              string
	SourceFingerprint           string
	Profile                     models.Profile
	ResolvedProfileFingerprint  string
	EffectiveProfileFingerprint string
	IdentityError               string
	TestEncodeID                uint
	ConfigurationIdentity       string
	EffectiveEncoder            string
	Plan                        SamplingPlan
}

type ProfileQualificationRuleResult struct {
	Key                string         `json:"key"`
	Outcome            string         `json:"outcome"`
	Disposition        string         `json:"disposition"`
	Observed           any            `json:"observed,omitempty"`
	Reference          any            `json:"reference,omitempty"`
	AllowedTolerance   any            `json:"allowedTolerance,omitempty"`
	WindowIdentity     string         `json:"windowIdentity,omitempty"`
	SceneCategory      string         `json:"sceneCategory,omitempty"`
	Explanation        string         `json:"explanation"`
	EvidenceProvenance models.JSONMap `json:"evidenceProvenance,omitempty"`
}

type ProfileQualificationAssessment struct {
	ID                          string                           `json:"id"`
	Outcome                     string                           `json:"outcome"`
	PolicyVersion               string                           `json:"policyVersion"`
	PolicySource                string                           `json:"policySource"`
	OptimizationIntent          string                           `json:"optimizationIntent"`
	SourceAssetID               uint                             `json:"sourceAssetId"`
	SourcePath                  string                           `json:"sourcePath"`
	SourceRevision              string                           `json:"sourceRevision"`
	SourceFingerprint           string                           `json:"sourceFingerprint"`
	ProfileID                   uint                             `json:"profileId"`
	ProfileVersion              int                              `json:"profileVersion"`
	ResolvedProfileFingerprint  string                           `json:"resolvedProfileFingerprint"`
	EffectiveProfileFingerprint string                           `json:"effectiveProfileFingerprint"`
	ConfigurationHash           string                           `json:"configurationHash"`
	TestEncodeID                uint                             `json:"testEncodeId"`
	CandidateIdentity           string                           `json:"candidateIdentity"`
	CandidateRevision           string                           `json:"candidateRevision"`
	SamplingPlanSchemaVersion   string                           `json:"samplingPlanSchemaVersion"`
	SamplingPlanPolicyVersion   string                           `json:"samplingPlanPolicyVersion"`
	SamplingPlanIdentity        string                           `json:"samplingPlanIdentity"`
	PerceptualEvidenceIdentity  string                           `json:"perceptualEvidenceIdentity"`
	ReferenceIdentity           string                           `json:"referenceIdentity"`
	Rules                       []ProfileQualificationRuleResult `json:"rules"`
	Warnings                    []string                         `json:"warnings"`
	QualifiedScope              string                           `json:"qualifiedScope"`
	EvidenceProvenance          models.JSONMap                   `json:"evidenceProvenance"`
	WorstWindowIdentity         string                           `json:"worstWindowIdentity,omitempty"`
	WorstSceneCategory          string                           `json:"worstSceneCategory,omitempty"`
	WorstRuleKeys               []string                         `json:"worstRuleKeys,omitempty"`
}

type qualificationIntentDefaults struct {
	TypicalVMAFFloor float64
	WorstVMAFFloor   float64
}

var profileQualificationIntentDefaults = map[string]qualificationIntentDefaults{
	"maximum_savings": {TypicalVMAFFloor: 90, WorstVMAFFloor: 82},
	"balanced":        {TypicalVMAFFloor: 92, WorstVMAFFloor: 85},
	"conservative":    {TypicalVMAFFloor: 94, WorstVMAFFloor: 88},
	"maximum_quality": {TypicalVMAFFloor: 96, WorstVMAFFloor: 91},
	"archive":         {TypicalVMAFFloor: 97, WorstVMAFFloor: 93},
}

type qualificationArtifactPolicy struct {
	kind              string
	maximumIncrease   int
	regressionOutcome string
}

var profileQualificationArtifactPolicy = []qualificationArtifactPolicy{
	{kind: "blocking", maximumIncrease: 0, regressionOutcome: models.QualityOutcomeFail},
	{kind: "blur_detail_loss", maximumIncrease: 0, regressionOutcome: models.QualityOutcomeReviewRequired},
	{kind: "banding", maximumIncrease: 0, regressionOutcome: models.QualityOutcomeReviewRequired},
	{kind: "ringing_halo", maximumIncrease: 0, regressionOutcome: models.QualityOutcomeReviewRequired},
	{kind: "grain_texture_loss", maximumIncrease: 0, regressionOutcome: models.QualityOutcomeReviewRequired},
	{kind: "temporal_instability", maximumIncrease: 0, regressionOutcome: models.QualityOutcomeReviewRequired},
	{kind: "chroma_degradation", maximumIncrease: 0, regressionOutcome: models.QualityOutcomeReviewRequired},
}

// EvaluateProfileQualification is the pure per-candidate policy boundary.
// It consumes only a resolved profile, one frozen SamplingPlan, and persisted
// #69 evidence; it never performs media or database work.
func EvaluateProfileQualification(candidate ProfileQualificationCandidate, evidence PerceptualEvidenceSet) ProfileQualificationAssessment {
	assessment := ProfileQualificationAssessment{
		Outcome:            models.QualityOutcomeReviewRequired,
		PolicyVersion:      profileQualificationPolicyVersion,
		PolicySource:       profileQualificationDefaultsSource,
		OptimizationIntent: strings.ToLower(strings.TrimSpace(candidate.Profile.OptimizationIntent)),
		SourceAssetID:      candidate.SourceAssetID, SourcePath: candidate.SourcePath,
		SourceRevision: candidate.SourceRevision, SourceFingerprint: candidate.SourceFingerprint,
		ProfileID: candidate.Profile.ID, ProfileVersion: candidate.Profile.ProfileVersion,
		ResolvedProfileFingerprint:  candidate.ResolvedProfileFingerprint,
		EffectiveProfileFingerprint: candidate.EffectiveProfileFingerprint,
		ConfigurationHash:           candidate.ConfigurationIdentity,
		TestEncodeID:                candidate.TestEncodeID,
		SamplingPlanSchemaVersion:   candidate.Plan.SchemaVersion,
		SamplingPlanPolicyVersion:   candidate.Plan.PolicyVersion,
		SamplingPlanIdentity:        candidate.Plan.PlanIdentity,
		PerceptualEvidenceIdentity:  evidence.Identity,
		QualifiedScope:              profileQualificationScope,
		Rules:                       make([]ProfileQualificationRuleResult, 0, len(candidate.Plan.SelectedWindows)*3+len(profileQualificationArtifactPolicy)+4),
		Warnings:                    []string{},
	}

	canonicalEvidence, evidenceErr := ComputePerceptualEvidenceSetIdentity(evidence)
	if evidenceErr == nil && canonicalEvidence.Identity == evidence.Identity {
		assessment.PerceptualEvidenceIdentity = canonicalEvidence.Identity
	}
	assessment.EvidenceProvenance = profileQualificationProvenance(candidate, evidence)
	assessment.CandidateIdentity, assessment.CandidateRevision, assessment.ReferenceIdentity = qualificationEvidenceIdentities(evidence)
	valid, reason := profileQualificationEvidenceMatches(candidate, evidence, canonicalEvidence, evidenceErr)
	if !valid {
		assessment.Rules = append(assessment.Rules, qualificationRule(
			"identity.evidence_current", models.QualityOutcomeReviewRequired, "blocking", nil, nil, nil, "", "",
			"Qualification evidence is stale or incomplete: "+reason, assessment.EvidenceProvenance,
		))
		assessment.Outcome = aggregateProfileQualificationRules(assessment.Rules)
		assessment.ID = profileQualificationID(candidate, assessment)
		assessment.Warnings = qualificationWarnings(assessment.Rules)
		return assessment
	}

	assessment.Rules = append(assessment.Rules, qualificationRule(
		"identity.evidence_current", models.QualityOutcomePass, "blocking", evidence.Identity, nil, nil, "", "",
		"Source, reusable profile, frozen effective profile, candidate, SamplingPlan, reference, and #69 evidence identities match.", assessment.EvidenceProvenance,
	))

	defaults, intentOK := profileQualificationIntentDefaults[assessment.OptimizationIntent]
	if !intentOK {
		assessment.Rules = append(assessment.Rules, qualificationRule(
			"policy.optimization_intent", models.QualityOutcomeReviewRequired, "blocking", assessment.OptimizationIntent, nil, nil, "", "",
			"No MVForge policy defaults exist for this optimization intent.", assessment.EvidenceProvenance,
		))
	} else {
		assessment.Rules = append(assessment.Rules, qualificationRule(
			"policy.optimization_intent", models.QualityOutcomePass, "informational", assessment.OptimizationIntent, nil, nil, "", "",
			"Applied versioned MVForge policy defaults for the resolved optimization intent.", assessment.EvidenceProvenance,
		))
	}

	windowByIdentity := make(map[string]PerceptualWindowEvidence, len(evidence.Windows))
	for _, window := range evidence.Windows {
		windowByIdentity[window.WindowIdentity] = window
	}
	typicalWindows := make([]SamplingWindow, 0, 1)
	for _, window := range candidate.Plan.SelectedWindows {
		if window.Primary || containsString(window.Roles, SamplingRoleTypical) {
			typicalWindows = append(typicalWindows, window)
		}
	}
	if len(typicalWindows) == 0 {
		assessment.Rules = append(assessment.Rules, qualificationRule(
			"sampling.typical_window", models.QualityOutcomeReviewRequired, "blocking", nil, nil, nil, "", "",
			"The frozen SamplingPlan does not identify a typical or primary window.", assessment.EvidenceProvenance,
		))
	}
	for _, role := range []string{SamplingRoleHighMotion, SamplingRoleDarkOrGradient, SamplingRoleHighTextureOrGrain} {
		available := false
		for _, window := range candidate.Plan.SelectedWindows {
			available = available || containsSamplingRole(window, role)
		}
		if !available {
			assessment.Rules = append(assessment.Rules, qualificationRule(
				"sampling.hard_scene_role", models.QualityOutcomeReviewRequired, "blocking", role, nil, nil, "", role,
				"The frozen SamplingPlan has no selected window for this hard-scene role; qualification cannot infer or select a replacement.", assessment.EvidenceProvenance,
			))
		}
	}

	typicalVMAFMeasured := false
	for _, planWindow := range candidate.Plan.SelectedWindows {
		window, found := windowByIdentity[planWindow.Identity]
		category := qualificationSceneCategory(planWindow)
		if !found || window.Status == "not_covered" {
			state, disposition := "WARNING", "advisory"
			if planWindow.Primary || qualificationIsHardScene(planWindow) {
				state, disposition = models.QualityOutcomeReviewRequired, "blocking"
			}
			assessment.Rules = append(assessment.Rules, qualificationRule(
				"sampling.window_coverage", state, disposition, "not_covered", nil, nil, planWindow.Identity, category,
				"The existing Test Encode does not cover this frozen window; no replacement scene or encode was created.", qualificationWindowProvenance(assessment.EvidenceProvenance, planWindow.Identity),
			))
			continue
		}
		if window.PairIdentity == nil || window.Reference == nil || window.Candidate == nil || window.PairIdentity.SamplingWindowIdentity != planWindow.Identity {
			assessment.Rules = append(assessment.Rules, qualificationRule(
				"evidence.window_comparable", models.QualityOutcomeReviewRequired, "blocking", window.Status, nil, nil, planWindow.Identity, category,
				"This candidate window lacks a complete identity-matched source/reference-to-candidate comparison.", qualificationWindowProvenance(assessment.EvidenceProvenance, planWindow.Identity),
			))
			continue
		}
		vmaf, metricFound := qualificationMetric(window, PerceptualMetricVMAF)
		if !metricFound || vmaf.Status != PerceptualMetricAvailable || vmaf.Value == nil {
			state := models.QualityOutcomeReviewRequired
			disposition := "blocking"
			if metricFound && vmaf.Status == PerceptualMetricUnsupported {
				state, disposition = "UNAVAILABLE", "advisory"
			}
			assessment.Rules = append(assessment.Rules, qualificationRule(
				"perceptual.vmaf.available", state, disposition, qualificationMetricStatus(vmaf, metricFound), nil, nil, planWindow.Identity, category,
				"Comparable VMAF evidence is required for the MVForge typical/worst-window floor; the canonical metric state is preserved.", qualificationMetricProvenance(assessment.EvidenceProvenance, window, vmaf, metricFound),
			))
			continue
		}
		observed, finite := vmaf.Value.Float64()
		if !finite || math.IsNaN(observed) || math.IsInf(observed, 0) {
			assessment.Rules = append(assessment.Rules, qualificationRule(
				"perceptual.vmaf.available", models.QualityOutcomeReviewRequired, "blocking", vmaf.Value, nil, nil, planWindow.Identity, category,
				"The canonical VMAF result does not contain a finite value that this policy can compare.", qualificationMetricProvenance(assessment.EvidenceProvenance, window, vmaf, true),
			))
			continue
		}
		if containsSamplingRole(planWindow, SamplingRoleTypical) || planWindow.Primary {
			typicalVMAFMeasured = true
			if intentOK {
				state := models.QualityOutcomePass
				if observed < defaults.TypicalVMAFFloor {
					state = models.QualityOutcomeFail
				}
				assessment.Rules = append(assessment.Rules, qualificationRule(
					"perceptual.typical_vmaf_floor", state, "blocking", observed, nil, defaults.TypicalVMAFFloor, planWindow.Identity, category,
					fmt.Sprintf("Typical-window VMAF %.3g is evaluated against the MVForge default floor %.3g for %s.", observed, defaults.TypicalVMAFFloor, assessment.OptimizationIntent),
					qualificationMetricProvenance(assessment.EvidenceProvenance, window, vmaf, true),
				))
			}
		}
		if intentOK {
			state := models.QualityOutcomePass
			if observed < defaults.WorstVMAFFloor {
				state = models.QualityOutcomeFail
			}
			assessment.Rules = append(assessment.Rules, qualificationRule(
				"perceptual.worst_window_vmaf_floor", state, "blocking", observed, nil, defaults.WorstVMAFFloor, planWindow.Identity, category,
				fmt.Sprintf("Window VMAF %.3g is evaluated against the MVForge worst-window default floor %.3g for %s.", observed, defaults.WorstVMAFFloor, assessment.OptimizationIntent),
				qualificationMetricProvenance(assessment.EvidenceProvenance, window, vmaf, true),
			))
		}
	}
	if !typicalVMAFMeasured {
		assessment.Rules = append(assessment.Rules, qualificationRule(
			"perceptual.typical_vmaf_required", models.QualityOutcomeReviewRequired, "blocking", nil, nil, nil, "", "typical",
			"No comparable candidate VMAF result is available for the frozen typical window.", assessment.EvidenceProvenance,
		))
	}
	assessment.Rules = append(assessment.Rules, evaluateQualificationArtifacts(candidate, evidence, assessment.EvidenceProvenance)...)
	if warningCount(assessment.Rules) > profileQualificationMaximumWarnings {
		assessment.Rules = append(assessment.Rules, qualificationRule(
			"policy.warning_limit", models.QualityOutcomeReviewRequired, "blocking", warningCount(assessment.Rules), nil, profileQualificationMaximumWarnings, "", "",
			"The number of unavailable/advisory findings exceeds the MVForge policy review limit.", assessment.EvidenceProvenance,
		))
	}
	canonicalQualificationRuleOrder(assessment.Rules, candidate.Plan)
	assessment.Outcome = aggregateProfileQualificationRules(assessment.Rules)
	assessment.Warnings = qualificationWarnings(assessment.Rules)
	selectQualificationWorstWindow(&assessment, candidate.Plan)
	assessment.ID = profileQualificationID(candidate, assessment)
	return assessment
}

func profileQualificationEvidenceMatches(candidate ProfileQualificationCandidate, evidence, canonical PerceptualEvidenceSet, evidenceErr error) (bool, string) {
	if candidate.IdentityError != "" {
		return false, candidate.IdentityError
	}
	if evidenceErr != nil || evidence.Identity == "" || canonical.Identity != evidence.Identity {
		return false, "the perceptual evidence identity is missing or does not match its canonical contents"
	}
	if candidate.SourceAssetID == 0 || candidate.TestEncodeID == 0 {
		return false, "the exact AssetRecord or Test Encode generation identity is missing"
	}
	if candidate.SourcePath == "" || candidate.SourceRevision == "" || candidate.SourceFingerprint == "" || evidence.SourceRevision != candidate.SourceFingerprint {
		return false, "the source path, physical revision, or source fingerprint does not match"
	}
	if candidate.ResolvedProfileFingerprint == "" {
		return false, "the reusable profile-definition identity is missing from the requested profile snapshot"
	}
	if !strings.HasPrefix(candidate.ResolvedProfileFingerprint, profileQualificationFingerprintVersion+":") {
		return false, "the reusable profile-definition identity uses a legacy or unsupported fingerprint version"
	}
	if candidate.EffectiveProfileFingerprint == "" {
		return false, "the frozen effective-profile identity is missing from the post-plan Test Encode snapshot"
	}
	if !strings.HasPrefix(candidate.EffectiveProfileFingerprint, effectiveProfileFingerprintVersion+":") {
		return false, "the frozen effective-profile identity uses a legacy or unsupported fingerprint version"
	}
	effectiveFingerprint, err := effectiveProfileFingerprint(candidate.Profile, candidate.EffectiveEncoder)
	if err != nil {
		return false, fmt.Sprintf("the frozen effective-profile identity cannot be recomputed from effectiveConfiguration.profile: %v", err)
	}
	if effectiveFingerprint != candidate.EffectiveProfileFingerprint {
		return false, fmt.Sprintf("the frozen effective-profile identity does not match effectiveConfiguration.profile after planning (persisted %s, recomputed %s)", candidate.EffectiveProfileFingerprint, effectiveFingerprint)
	}
	if candidate.ConfigurationIdentity == "" || !strings.HasPrefix(candidate.ConfigurationIdentity, testEncodeConfigurationFingerprintVersion+":") {
		return false, "the candidate ConfigurationHash lacks the versioned frozen Test Encode configuration identity"
	}
	if candidate.Profile.ProfileVersion < 1 || candidate.Plan.PlanIdentity == "" || candidate.Plan.SchemaVersion == "" || candidate.Plan.PolicyVersion == "" || len(candidate.Plan.SelectedWindows) == 0 {
		return false, "the resolved profile version or frozen SamplingPlan identity/version/windows are incomplete"
	}
	if evidence.SamplingPlanIdentity != candidate.Plan.PlanIdentity {
		return false, "the evidence references a different SamplingPlan"
	}
	planWindows := make(map[string]bool, len(candidate.Plan.SelectedWindows))
	for _, window := range candidate.Plan.SelectedWindows {
		if window.Identity == "" || planWindows[window.Identity] {
			return false, "the frozen SamplingPlan contains an empty or duplicate window identity"
		}
		planWindows[window.Identity] = true
	}
	for _, window := range evidence.Windows {
		if !planWindows[window.WindowIdentity] {
			return false, "the evidence contains a window outside the frozen SamplingPlan"
		}
		if window.PairIdentity != nil {
			pair, pairErr := ComputeComparisonPairIdentity(*window.PairIdentity)
			referenceIdentity, referenceErr := ComputeReferenceIdentity(window.PairIdentity.Reference)
			candidateIdentity, candidateErr := ComputeCandidateIdentity(window.PairIdentity.Candidate)
			if pairErr != nil || pair.Identity != window.PairIdentity.Identity || pair.SamplingPlanIdentity != candidate.Plan.PlanIdentity || pair.SamplingWindowIdentity != window.WindowIdentity {
				return false, "a measured window has a stale comparison-pair identity"
			}
			if referenceErr != nil || referenceIdentity.Identity != window.PairIdentity.Reference.Identity || candidateErr != nil || candidateIdentity.Identity != window.PairIdentity.Candidate.Identity {
				return false, "a measured window reference/candidate identity is stale"
			}
			if window.Reference == nil || window.Candidate == nil || window.Reference.Identity != pair.Reference.Identity || window.Candidate.Identity != pair.Candidate.Identity {
				return false, "a measured window reference/candidate identity does not match its comparison pair"
			}
			if window.Candidate.SourceRevision != candidate.SourceFingerprint || window.Candidate.Identity == "" {
				return false, "a candidate window references a different source fingerprint"
			}
			if candidate.TestEncodeID > 0 {
				if window.Candidate.ArtifactID != fmt.Sprintf("test-encode:%d", candidate.TestEncodeID) ||
					window.Candidate.ConfigurationIdentity != candidate.ConfigurationIdentity ||
					window.Candidate.GenerationToken != fmt.Sprintf("test-encode:%d", candidate.TestEncodeID) {
					return false, "a candidate window does not identify this Test Encode configuration and generation"
				}
			}
		}
	}
	if len(evidence.ReferenceManifest) == 0 {
		return false, "the evidence has no reference manifest"
	}
	if len(evidence.CandidateManifest) == 0 {
		return false, "the evidence has no candidate manifest"
	}
	matchedCandidate := false
	for _, entry := range evidence.CandidateManifest {
		candidateIdentity, candidateErr := ComputeCandidateIdentity(entry.Candidate)
		coverageIdentity, coverageErr := ComputeCandidateCoverage(entry.Coverage)
		if candidateErr != nil || candidateIdentity.Identity != entry.Candidate.Identity || entry.Candidate.SourceRevision != candidate.SourceFingerprint ||
			coverageErr != nil || coverageIdentity.Identity != entry.Coverage.Identity || coverageIdentity.SourceDurationSeconds <= 0 {
			return false, "the candidate manifest identity is stale or references a different source"
		}
		for _, windowID := range entry.Coverage.CoveredWindowIdentities {
			if !planWindows[windowID] {
				return false, "candidate coverage references a window outside the frozen SamplingPlan"
			}
		}
		if candidate.TestEncodeID > 0 && entry.Candidate.ArtifactID == fmt.Sprintf("test-encode:%d", candidate.TestEncodeID) &&
			entry.Candidate.ConfigurationIdentity == candidate.ConfigurationIdentity && entry.Candidate.GenerationToken == fmt.Sprintf("test-encode:%d", candidate.TestEncodeID) {
			matchedCandidate = true
		}
	}
	if candidate.TestEncodeID > 0 && !matchedCandidate {
		return false, "the evidence candidate manifest does not identify this Test Encode"
	}
	for _, entry := range evidence.ReferenceManifest {
		referenceIdentity, referenceErr := ComputeReferenceIdentity(entry.Reference)
		if referenceErr != nil || referenceIdentity.Identity != entry.Reference.Identity || entry.Reference.SamplingPlanIdentity != candidate.Plan.PlanIdentity {
			return false, "a source/reference manifest identity is stale or references a different SamplingPlan"
		}
	}
	return true, ""
}

func qualificationEvidenceIdentities(evidence PerceptualEvidenceSet) (candidateID, candidateRevision, referenceID string) {
	candidates := append([]CandidateManifestEntry(nil), evidence.CandidateManifest...)
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Candidate.Identity < candidates[j].Candidate.Identity })
	if len(candidates) > 0 {
		candidateID = candidates[0].Candidate.Identity
		candidateRevision = candidates[0].Candidate.OutputRevision
	}
	references := make([]string, 0, len(evidence.ReferenceManifest))
	for _, reference := range evidence.ReferenceManifest {
		references = append(references, reference.Reference.Identity)
	}
	sort.Strings(references)
	encoded, _ := json.Marshal(references)
	digest := sha256.Sum256(encoded)
	return candidateID, candidateRevision, "qualification-references-v1:" + hex.EncodeToString(digest[:])
}

func ProfileQualificationIsCurrent(record models.ProfileQualification, candidate ProfileQualificationCandidate, evidence PerceptualEvidenceSet) (bool, string) {
	if record.QualityPolicyVersion != profileQualificationPolicyVersion {
		return false, "qualification policy version changed"
	}
	if record.TestEncodeID != candidate.TestEncodeID {
		return false, "Test Encode candidate identity changed"
	}
	if record.SourceAssetID != candidate.SourceAssetID || record.SourcePath != candidate.SourcePath || record.SourceRevision != candidate.SourceRevision || record.SourceFingerprint != candidate.SourceFingerprint {
		return false, "source identity changed"
	}
	if candidate.ResolvedProfileFingerprint != record.ResolvedProfileFingerprint {
		return false, "reusable profile-definition identity changed"
	}
	if candidate.IdentityError != "" {
		return false, candidate.IdentityError
	}
	effectiveFingerprint, err := effectiveProfileFingerprint(candidate.Profile, candidate.EffectiveEncoder)
	if err != nil || effectiveFingerprint != candidate.EffectiveProfileFingerprint || candidate.EffectiveProfileFingerprint != record.EffectiveProfileFingerprint {
		return false, "frozen effective-profile identity changed"
	}
	if candidate.ConfigurationIdentity != record.ConfigurationHash {
		return false, "Test Encode ConfigurationHash changed"
	}
	canonicalEvidence, evidenceErr := ComputePerceptualEvidenceSetIdentity(evidence)
	valid, reason := profileQualificationEvidenceMatches(candidate, evidence, canonicalEvidence, evidenceErr)
	if !valid {
		return false, reason
	}
	if record.ProfileID != candidate.Profile.ID || record.ProfileVersion != candidate.Profile.ProfileVersion {
		return false, "resolved profile version changed"
	}
	if record.SamplingPlanSchemaVersion != candidate.Plan.SchemaVersion || record.SamplingPlanPolicyVersion != candidate.Plan.PolicyVersion || record.SamplingPlanIdentity != candidate.Plan.PlanIdentity {
		return false, "SamplingPlan identity or version changed"
	}
	if record.PerceptualEvidenceIdentity != evidence.Identity {
		return false, "perceptual evidence identity changed"
	}
	assessment := EvaluateProfileQualification(candidate, evidence)
	if assessment.CandidateIdentity != record.CandidateIdentity || assessment.CandidateRevision != record.CandidateRevision || assessment.ReferenceIdentity != record.ReferenceIdentity {
		return false, "candidate or reference identity changed"
	}
	if record.ID != assessment.ID {
		return false, "qualification identity does not match the current evidence and policy"
	}
	return true, ""
}

func evaluateQualificationArtifacts(candidate ProfileQualificationCandidate, evidence PerceptualEvidenceSet, provenance models.JSONMap) []ProfileQualificationRuleResult {
	plan := candidate.Plan
	result := make([]ProfileQualificationRuleResult, 0, len(profileQualificationArtifactPolicy))
	byKind := make(map[string][]ArtifactEvidence, len(profileQualificationArtifactPolicy))
	for _, signal := range evidence.ArtifactEvidence {
		byKind[signal.Kind] = append(byKind[signal.Kind], signal)
	}
	for _, policy := range profileQualificationArtifactPolicy {
		signals := byKind[policy.kind]
		if len(signals) == 0 {
			result = append(result, qualificationRule(
				"artifact."+policy.kind, "UNAVAILABLE", "advisory", nil, nil, policy.maximumIncrease, "", "",
				"The current canonical evidence set has no comparable candidate/reference signal for this artifact domain.", provenance,
			))
			continue
		}
		for _, signal := range signals {
			key := "artifact." + policy.kind
			windowID := signal.WindowIdentity
			category := qualificationSceneCategoryByIdentity(plan, windowID)
			windowProvenance := qualificationWindowProvenance(provenance, windowID)
			if signal.Status == "unsupported" || signal.Status == "unavailable" {
				result = append(result, qualificationRule(key, "UNAVAILABLE", "advisory", signal.Status, nil, policy.maximumIncrease, windowID, category,
					firstNonEmpty(signal.Reason, "The runtime does not provide this canonical artifact signal."), windowProvenance))
				continue
			}
			if signal.Scope != "pair_comparison" || signal.PairIdentity == nil || signal.WindowIdentity == "" || signal.PairIdentity.SamplingWindowIdentity != signal.WindowIdentity || signal.ReferenceSeverity == "" || signal.CandidateSeverity == "" {
				result = append(result, qualificationRule(key, models.QualityOutcomeReviewRequired, "blocking", signal.Status, signal.ReferenceSeverity, policy.maximumIncrease, windowID, category,
					"The canonical signal is source context or lacks a same-window candidate/reference comparison; it cannot establish regression.", windowProvenance))
				continue
			}
			pair, pairErr := ComputeComparisonPairIdentity(*signal.PairIdentity)
			referenceIdentity, referenceErr := ComputeReferenceIdentity(signal.PairIdentity.Reference)
			candidateIdentity, candidateErr := ComputeCandidateIdentity(signal.PairIdentity.Candidate)
			candidateMatches := pairErr == nil && pair.Candidate.SourceRevision == candidate.SourceFingerprint && pair.Reference.SourceRevision == candidate.SourceFingerprint
			candidateMatches = candidateMatches && referenceErr == nil && referenceIdentity.Identity == signal.PairIdentity.Reference.Identity &&
				candidateErr == nil && candidateIdentity.Identity == signal.PairIdentity.Candidate.Identity
			if candidate.TestEncodeID > 0 {
				candidateMatches = candidateMatches && pair.Candidate.ArtifactID == fmt.Sprintf("test-encode:%d", candidate.TestEncodeID) &&
					pair.Candidate.ConfigurationIdentity == candidate.ConfigurationIdentity && pair.Candidate.GenerationToken == fmt.Sprintf("test-encode:%d", candidate.TestEncodeID)
			}
			if pairErr != nil || pair.Identity != signal.PairIdentity.Identity || pair.SamplingPlanIdentity != plan.PlanIdentity || pair.SamplingWindowIdentity != signal.WindowIdentity || !candidateMatches {
				result = append(result, qualificationRule(key, models.QualityOutcomeReviewRequired, "blocking", signal.Status, signal.ReferenceSeverity, policy.maximumIncrease, windowID, category,
					"The artifact comparison identity is stale or does not match the frozen plan.", windowProvenance))
				continue
			}
			referenceSeverity, referenceOK := artifactSeverityRank(signal.ReferenceSeverity)
			candidateSeverity, candidateOK := artifactSeverityRank(signal.CandidateSeverity)
			if !referenceOK || !candidateOK {
				result = append(result, qualificationRule(key, models.QualityOutcomeReviewRequired, "blocking", signal.CandidateSeverity, signal.ReferenceSeverity, policy.maximumIncrease, windowID, category,
					"The canonical artifact severity is ambiguous and cannot be compared with the versioned policy.", windowProvenance))
				continue
			}
			increase := candidateSeverity - referenceSeverity
			if increase <= policy.maximumIncrease {
				result = append(result, qualificationRule(key, models.QualityOutcomePass, "blocking", signal.CandidateSeverity, signal.ReferenceSeverity, policy.maximumIncrease, windowID, category,
					"Candidate artifact severity is within the recorded MVForge tolerance relative to the source/reference.", windowProvenance))
				continue
			}
			outcome := policy.regressionOutcome
			if !confidenceIsHigh(signal.ReferenceConfidence) || !confidenceIsHigh(signal.CandidateConfidence) {
				outcome = models.QualityOutcomeReviewRequired
			}
			result = append(result, qualificationRule(key, outcome, "blocking", signal.CandidateSeverity, signal.ReferenceSeverity, policy.maximumIncrease, windowID, category,
				fmt.Sprintf("Candidate severity increased by %d level(s) over source/reference severity; MVForge allows %d.", increase, policy.maximumIncrease), windowProvenance))
		}
	}
	return result
}

func qualificationRule(key, outcome, disposition string, observed, reference, tolerance any, windowID, category, explanation string, provenance models.JSONMap) ProfileQualificationRuleResult {
	return ProfileQualificationRuleResult{
		Key: key, Outcome: outcome, Disposition: disposition, Observed: observed,
		Reference: reference, AllowedTolerance: tolerance, WindowIdentity: windowID,
		SceneCategory: category, Explanation: explanation, EvidenceProvenance: provenance,
	}
}

func aggregateProfileQualificationRules(rules []ProfileQualificationRuleResult) string {
	warning, review := false, false
	for _, rule := range rules {
		if rule.Outcome == models.QualityOutcomeFail && rule.Disposition == "blocking" {
			return models.QualityOutcomeFail
		}
		if rule.Outcome == models.QualityOutcomeReviewRequired || rule.Outcome == "UNVERIFIED" {
			review = true
		}
		if rule.Outcome == models.QualityOutcomePassWithWarnings || rule.Outcome == "WARNING" || rule.Outcome == "UNAVAILABLE" ||
			(rule.Disposition == "advisory" && (rule.Outcome == models.QualityOutcomeFail || rule.Outcome == "UNVERIFIED" || rule.Outcome == models.QualityOutcomeReviewRequired)) {
			warning = true
		}
	}
	if review {
		return models.QualityOutcomeReviewRequired
	}
	if warning {
		return models.QualityOutcomePassWithWarnings
	}
	return models.QualityOutcomePass
}

func qualificationWarnings(rules []ProfileQualificationRuleResult) []string {
	warnings := make([]string, 0)
	for _, rule := range rules {
		if rule.Outcome == "WARNING" || rule.Outcome == "UNAVAILABLE" || rule.Outcome == models.QualityOutcomeReviewRequired || rule.Outcome == models.QualityOutcomeFail {
			warnings = append(warnings, rule.Key+": "+rule.Explanation)
		}
	}
	return warnings
}

func selectQualificationWorstWindow(assessment *ProfileQualificationAssessment, plan SamplingPlan) {
	if assessment == nil {
		return
	}
	order := make(map[string]int, len(plan.SelectedWindows))
	for index, window := range plan.SelectedWindows {
		order[window.Identity] = index
	}
	severity := func(outcome string) int {
		switch outcome {
		case models.QualityOutcomeFail:
			return 4
		case models.QualityOutcomeReviewRequired:
			return 3
		case "WARNING", "UNAVAILABLE", models.QualityOutcomePassWithWarnings:
			return 2
		case models.QualityOutcomePass:
			return 0
		default:
			return 0
		}
	}
	bestSeverity, bestOrder := 0, math.MaxInt
	for _, rule := range assessment.Rules {
		if rule.WindowIdentity == "" {
			continue
		}
		ruleSeverity := severity(rule.Outcome)
		if ruleSeverity == 0 {
			continue
		}
		ruleOrder, found := order[rule.WindowIdentity]
		if !found {
			continue
		}
		if ruleSeverity > bestSeverity || (ruleSeverity == bestSeverity && ruleOrder < bestOrder) {
			bestSeverity, bestOrder = ruleSeverity, ruleOrder
			assessment.WorstWindowIdentity = rule.WindowIdentity
			assessment.WorstSceneCategory = rule.SceneCategory
			assessment.WorstRuleKeys = []string{rule.Key}
		} else if ruleSeverity == bestSeverity && rule.WindowIdentity == assessment.WorstWindowIdentity {
			assessment.WorstRuleKeys = append(assessment.WorstRuleKeys, rule.Key)
		}
	}
}

func profileQualificationID(candidate ProfileQualificationCandidate, assessment ProfileQualificationAssessment) string {
	value := struct {
		SourceAssetID               uint   `json:"sourceAssetId"`
		SourcePath                  string `json:"sourcePath"`
		SourceRevision              string `json:"sourceRevision"`
		SourceFingerprint           string `json:"sourceFingerprint"`
		ProfileFingerprint          string `json:"profileFingerprint"`
		EffectiveProfileFingerprint string `json:"effectiveProfileFingerprint"`
		ConfigurationHash           string `json:"configurationHash"`
		CandidateIdentity           string `json:"candidateIdentity"`
		CandidateRevision           string `json:"candidateRevision"`
		PlanIdentity                string `json:"planIdentity"`
		EvidenceIdentity            string `json:"evidenceIdentity"`
		ReferenceIdentity           string `json:"referenceIdentity"`
		PolicyVersion               string `json:"policyVersion"`
	}{candidate.SourceAssetID, candidate.SourcePath, candidate.SourceRevision, candidate.SourceFingerprint,
		candidate.ResolvedProfileFingerprint, candidate.EffectiveProfileFingerprint, candidate.ConfigurationIdentity,
		assessment.CandidateIdentity, assessment.CandidateRevision,
		candidate.Plan.PlanIdentity, assessment.PerceptualEvidenceIdentity, assessment.ReferenceIdentity, profileQualificationPolicyVersion}
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return "profile-qualification-v1:" + hex.EncodeToString(digest[:])
}

func profileQualificationProvenance(candidate ProfileQualificationCandidate, evidence PerceptualEvidenceSet) models.JSONMap {
	return models.JSONMap{
		"policySource":                profileQualificationDefaultsSource,
		"qualityPolicyVersion":        profileQualificationPolicyVersion,
		"optimizationIntent":          strings.ToLower(strings.TrimSpace(candidate.Profile.OptimizationIntent)),
		"profileFingerprintVersion":   profileQualificationFingerprintVersion,
		"sourceRevision":              candidate.SourceRevision,
		"sourceFingerprint":           candidate.SourceFingerprint,
		"resolvedProfileFingerprint":  candidate.ResolvedProfileFingerprint,
		"effectiveProfileFingerprint": candidate.EffectiveProfileFingerprint,
		"configurationHash":           candidate.ConfigurationIdentity,
		"samplingPlanSchemaVersion":   candidate.Plan.SchemaVersion,
		"samplingPlanPolicyVersion":   candidate.Plan.PolicyVersion,
		"samplingPlanIdentity":        candidate.Plan.PlanIdentity,
		"perceptualEvidenceIdentity":  evidence.Identity,
		"referenceIdentities":         qualificationReferenceIDs(evidence),
		"candidateIdentities":         qualificationCandidateIDs(evidence),
	}
}

func qualificationWindowProvenance(provenance models.JSONMap, windowID string) models.JSONMap {
	copy := models.JSONMap{}
	for key, value := range provenance {
		copy[key] = value
	}
	copy["windowIdentity"] = windowID
	return copy
}

func qualificationMetricProvenance(provenance models.JSONMap, window PerceptualWindowEvidence, metric MetricResult, found bool) models.JSONMap {
	copy := qualificationWindowProvenance(provenance, window.WindowIdentity)
	if window.PairIdentity != nil {
		copy["pairIdentity"] = window.PairIdentity.Identity
	}
	if found {
		copy["metricIdentity"] = metric.Identity.Identity
		copy["metricStatus"] = metric.Status
	}
	return copy
}

func qualificationReferenceIDs(evidence PerceptualEvidenceSet) []string {
	result := make([]string, 0, len(evidence.ReferenceManifest))
	for _, entry := range evidence.ReferenceManifest {
		result = append(result, entry.Reference.Identity)
	}
	sort.Strings(result)
	return result
}

func qualificationCandidateIDs(evidence PerceptualEvidenceSet) []string {
	result := make([]string, 0, len(evidence.CandidateManifest))
	for _, entry := range evidence.CandidateManifest {
		result = append(result, entry.Candidate.Identity)
	}
	sort.Strings(result)
	return result
}

func qualificationMetric(window PerceptualWindowEvidence, wanted PerceptualMetric) (MetricResult, bool) {
	for _, metric := range window.Metrics {
		if metric.Identity.Metric.LogicalMetric == wanted {
			return metric, true
		}
	}
	return MetricResult{}, false
}

func qualificationMetricStatus(metric MetricResult, found bool) any {
	if !found {
		return "missing"
	}
	if metric.Status == PerceptualMetricAvailable && metric.Value != nil {
		return metric.Value
	}
	if metric.Reason != "" {
		return string(metric.Status) + ": " + metric.Reason
	}
	return metric.Status
}

func warningCount(rules []ProfileQualificationRuleResult) int {
	count := 0
	for _, rule := range rules {
		if rule.Outcome == "WARNING" || rule.Outcome == "UNAVAILABLE" || rule.Outcome == models.QualityOutcomePassWithWarnings {
			count++
		}
	}
	return count
}

func qualificationSceneCategory(window SamplingWindow) string {
	switch {
	case containsSamplingRole(window, SamplingRoleHighMotion):
		return "high_motion"
	case containsSamplingRole(window, SamplingRoleDarkOrGradient):
		return "dark_or_gradient"
	case containsSamplingRole(window, SamplingRoleHighTextureOrGrain):
		return "high_texture_or_grain"
	case containsSamplingRole(window, SamplingRoleTypical) || window.Primary:
		return "typical"
	case len(window.Roles) > 0:
		return window.Roles[0]
	default:
		return "sampled_window"
	}
}

func qualificationSceneCategoryByIdentity(plan SamplingPlan, windowID string) string {
	for _, window := range plan.SelectedWindows {
		if window.Identity == windowID {
			return qualificationSceneCategory(window)
		}
	}
	return ""
}

func qualificationIsHardScene(window SamplingWindow) bool {
	return containsSamplingRole(window, SamplingRoleHighMotion) || containsSamplingRole(window, SamplingRoleDarkOrGradient) || containsSamplingRole(window, SamplingRoleHighTextureOrGrain)
}

func containsSamplingRole(window SamplingWindow, role string) bool {
	return containsString(window.Roles, role)
}

func artifactSeverityRank(value string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "none", "absent", "normal":
		return 0, true
	case "low":
		return 1, true
	case "medium":
		return 2, true
	case "high":
		return 3, true
	default:
		return 0, false
	}
}

func confidenceIsHigh(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "high")
}

func qualificationRuleStateRank(rule ProfileQualificationRuleResult) int {
	if rule.Disposition == "advisory" && rule.Outcome == models.QualityOutcomeFail {
		return 1
	}
	switch rule.Outcome {
	case models.QualityOutcomeFail:
		return 5
	case models.QualityOutcomeReviewRequired, "UNVERIFIED":
		return 4
	case "WARNING", "UNAVAILABLE", models.QualityOutcomePassWithWarnings:
		return 3
	case models.QualityOutcomePass:
		return 2
	default:
		return 0
	}
}

func canonicalQualificationRuleOrder(rules []ProfileQualificationRuleResult, plan SamplingPlan) {
	order := map[string]int{
		"identity.evidence_current": 0, "policy.optimization_intent": 1, "sampling.typical_window": 2,
		"sampling.window_coverage": 3, "evidence.window_comparable": 4, "perceptual.vmaf.available": 5,
		"perceptual.typical_vmaf_floor": 6, "perceptual.worst_window_vmaf_floor": 7,
		"perceptual.typical_vmaf_required": 8, "artifact.blocking": 9, "artifact.blur_detail_loss": 10,
		"artifact.banding": 11, "artifact.ringing_halo": 12, "artifact.grain_texture_loss": 13,
		"artifact.temporal_instability": 14, "artifact.chroma_degradation": 15, "policy.warning_limit": 16,
	}
	windowOrder := make(map[string]int, len(plan.SelectedWindows))
	for index, window := range plan.SelectedWindows {
		windowOrder[window.Identity] = index
	}
	sort.SliceStable(rules, func(i, j int) bool {
		left, leftOK := order[rules[i].Key]
		right, rightOK := order[rules[j].Key]
		if !leftOK {
			left = len(order)
		}
		if !rightOK {
			right = len(order)
		}
		if left != right {
			return left < right
		}
		leftWindow, leftWindowOK := windowOrder[rules[i].WindowIdentity]
		rightWindow, rightWindowOK := windowOrder[rules[j].WindowIdentity]
		if !leftWindowOK {
			leftWindow = math.MaxInt
		}
		if !rightWindowOK {
			rightWindow = math.MaxInt
		}
		if leftWindow != rightWindow {
			return leftWindow < rightWindow
		}
		return rules[i].Explanation < rules[j].Explanation
	})
}

func profileQualificationDiagnostic(assessment ProfileQualificationAssessment) string {
	return fmt.Sprintf("%s (%s): %d rules, %d warnings", assessment.Outcome, assessment.PolicyVersion, len(assessment.Rules), len(assessment.Warnings))
}
