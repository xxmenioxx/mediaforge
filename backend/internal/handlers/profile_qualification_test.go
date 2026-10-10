package handlers

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func profileQualificationFixture(t *testing.T, includeHardScenes bool, vmaf float64) (ProfileQualificationCandidate, PerceptualEvidenceSet) {
	profile := models.Profile{
		ID: 8, ProfileVersion: 3, TemplateKey: "hevc-balanced", TemplateVersion: 2,
		VideoCodec: "hevc", CodecFamily: "hevc", EncoderPolicy: "preferred",
		PreferredEncoder: "libx265", AllowedEncoders: models.StringList{"libx265"},
		QualityMode: "crf", QualityValue: 24, OptimizationIntent: "balanced",
		WorkerConfig: models.JSONMap{"gopSize": 48, "preserveGrain": true},
	}
	return profileQualificationFixtureForProfile(t, profile, "", "libx265", includeHardScenes, vmaf)
}

func profileQualificationFixtureForProfile(t *testing.T, profile models.Profile, definitionFingerprint, effectiveEncoder string, includeHardScenes bool, vmaf float64) (ProfileQualificationCandidate, PerceptualEvidenceSet) {
	t.Helper()
	if definitionFingerprint == "" {
		var err error
		definitionFingerprint, err = resolvedProfileFingerprint(profile)
		if err != nil {
			t.Fatalf("fingerprint profile: %v", err)
		}
	}
	effectiveFingerprint, err := effectiveProfileFingerprint(profile, effectiveEncoder)
	if err != nil {
		t.Fatalf("fingerprint effective profile: %v", err)
	}
	effectiveConfiguration := models.JSONMap{
		"profileIdentityContractVersion": profileIdentityContractVersion,
		"profileDefinitionFingerprint":   definitionFingerprint,
		"effectiveProfileFingerprint":    effectiveFingerprint,
		"effectiveEncoder":               effectiveEncoder,
		"override":                       models.JSONMap{},
		"streamPlan":                     models.JSONMap{},
		"subtitleStylePolicyVersion":     "subtitle-style-fixture-v1",
		"processingMode":                 ProcessingModeFullEncode,
	}
	configurationIdentity, err := testEncodeConfigurationFingerprint(effectiveConfiguration)
	if err != nil {
		t.Fatalf("fingerprint Test Encode configuration: %v", err)
	}
	plan := SamplingPlan{
		SchemaVersion: samplingPlanSchemaVersion, PolicyVersion: samplingPlanPolicyVersion,
		PlanIdentity: "plan:qualification-fixture", SourceFingerprint: "physical-source-v1", SourceRevision: "physical-source-v1",
	}
	plan.SelectedWindows = []SamplingWindow{{
		Identity: "window:typical", StartSeconds: 10, DurationSeconds: 5, Primary: true,
		Roles: []string{SamplingRoleTypical},
	}}
	if includeHardScenes {
		plan.SelectedWindows = append(plan.SelectedWindows,
			SamplingWindow{Identity: "window:motion", StartSeconds: 30, DurationSeconds: 5, Roles: []string{SamplingRoleHighMotion}},
			SamplingWindow{Identity: "window:dark", StartSeconds: 50, DurationSeconds: 5, Roles: []string{SamplingRoleDarkOrGradient}},
			SamplingWindow{Identity: "window:texture", StartSeconds: 70, DurationSeconds: 5, Roles: []string{SamplingRoleHighTextureOrGrain}},
		)
	}
	const sourceFingerprint = "source-fingerprint-v1"
	const testEncodeID = uint(17)
	candidateIdentity, err := ComputeCandidateIdentity(CandidateIdentity{
		Kind: "test_encode", ArtifactID: "test-encode:17", GenerationToken: "test-encode:17", GenerationVersion: 1,
		OutputRevision: "candidate-output-v1", ConfigurationIdentity: configurationIdentity,
		SourceRevision: sourceFingerprint, StreamIdentity: "0:v:0", SegmentStartSeconds: 0,
		SegmentDurationSeconds: 90, TimelineIdentity: "candidate-timeline-v1",
	})
	if err != nil {
		t.Fatalf("candidate identity: %v", err)
	}
	windowIDs := make([]string, 0, len(plan.SelectedWindows))
	for _, window := range plan.SelectedWindows {
		windowIDs = append(windowIDs, window.Identity)
	}
	coverage, err := ComputeCandidateCoverage(CandidateCoverage{
		SourceStartSeconds: 0, SourceDurationSeconds: 90, CoveredWindowIdentities: windowIDs,
		SegmentMappingIdentity: "segment-map-v1", SourceTimelineIdentity: "source-timeline-v1",
		CandidateTimelineIdentity: "candidate-timeline-v1",
	})
	if err != nil {
		t.Fatalf("candidate coverage: %v", err)
	}
	descriptor := newMetricDescriptor(PerceptualMetricVMAF, "ffmpeg", "7", "libvmaf-v1", "vmaf-v1", "runtime-v1", "higher_is_better")
	windows := make([]PerceptualWindowEvidence, 0, len(plan.SelectedWindows))
	for _, planWindow := range plan.SelectedWindows {
		reference, refErr := ComputeReferenceIdentity(ReferenceIdentity{
			SamplingPlanIdentity: plan.PlanIdentity, SamplingWindowIdentity: planWindow.Identity,
			SourceRevision: sourceFingerprint, SourceStreamIdentity: "0:v:0",
			TransformIdentity: "identity-transform-v1", TransformVersion: "v1",
			AlignmentIdentity: "alignment:" + planWindow.Identity, ComparisonDomainIdentity: "domain:" + planWindow.Identity,
			Alignment: AlignmentEvidence{ContractVersion: "alignment-v1", TimelineIdentity: "source-timeline-v1",
				SourceTimelineIdentity: "source-timeline-v1", ReferenceTimelineIdentity: "reference-timeline-v1",
				CandidateTimelineIdentity: "candidate-timeline-v1", TimestampOrigin: "source_start",
				MappingMode: "identity", FramePairingMode: "one_to_one", SourceFrameCount: 10,
				ReferenceFrameCount: 10, CandidateFrameCount: 10},
		})
		if refErr != nil {
			t.Fatalf("reference identity: %v", refErr)
		}
		pair, pairErr := ComputeComparisonPairIdentity(ComparisonPairIdentity{
			SamplingPlanIdentity: plan.PlanIdentity, SamplingWindowIdentity: planWindow.Identity,
			Reference: reference, Candidate: candidateIdentity, CoverageIdentity: coverage.Identity,
			SegmentMappingIdentity: coverage.SegmentMappingIdentity, AlignmentIdentity: reference.AlignmentIdentity,
			ComparisonDomainIdentity: reference.ComparisonDomainIdentity,
			SourceRevision:           sourceFingerprint, CandidateRevision: candidateIdentity.OutputRevision,
		})
		if pairErr != nil {
			t.Fatalf("comparison pair identity: %v", pairErr)
		}
		metricIdentity, metricErr := ComputeMetricResultIdentity(pair, descriptor)
		if metricErr != nil {
			t.Fatalf("metric identity: %v", metricErr)
		}
		value, valueErr := NewFiniteMetricValue(vmaf)
		if valueErr != nil {
			t.Fatalf("metric value: %v", valueErr)
		}
		windows = append(windows, PerceptualWindowEvidence{
			WindowIdentity: planWindow.Identity, StartSeconds: planWindow.StartSeconds,
			DurationSeconds: planWindow.DurationSeconds, Primary: planWindow.Primary,
			Roles: append([]string(nil), planWindow.Roles...), Status: "ready",
			Reference: &reference, Candidate: &candidateIdentity, Coverage: &coverage,
			PairIdentity: &pair, Metrics: []MetricResult{{Identity: metricIdentity, Status: PerceptualMetricAvailable, Value: &value}},
		})
	}
	evidence, err := BuildPerceptualEvidenceSet(plan, sourceFingerprint, "runtime-v1", []MetricDescriptor{descriptor}, windows, nil, nil, nil)
	if err != nil {
		t.Fatalf("build canonical perceptual evidence: %v", err)
	}
	candidate := ProfileQualificationCandidate{
		SourceAssetID: 32, SourcePath: "/raw/qualification-fixture.mkv",
		SourceRevision:    `{"Size":1000,"ModifiedAtNS":123,"Mode":420}`,
		SourceFingerprint: sourceFingerprint, Profile: profile,
		ResolvedProfileFingerprint: definitionFingerprint, EffectiveProfileFingerprint: effectiveFingerprint,
		EffectiveEncoder: effectiveEncoder, TestEncodeID: testEncodeID,
		ConfigurationIdentity: configurationIdentity, Plan: plan,
	}
	return candidate, evidence
}

func profileQualificationTestEncode(candidate ProfileQualificationCandidate, evidence PerceptualEvidenceSet) models.TestEncode {
	effective := models.JSONMap{
		"profileIdentityContractVersion": profileIdentityContractVersion,
		"profileDefinitionFingerprint":   candidate.ResolvedProfileFingerprint,
		"effectiveProfileFingerprint":    candidate.EffectiveProfileFingerprint,
		"effectiveEncoder":               candidate.EffectiveEncoder,
		"override":                       models.JSONMap{}, "streamPlan": models.JSONMap{},
		"subtitleStylePolicyVersion": "subtitle-style-fixture-v1", "processingMode": ProcessingModeFullEncode,
		"configurationHash": candidate.ConfigurationIdentity, "profile": candidate.Profile,
	}
	return models.TestEncode{
		ID: candidate.TestEncodeID, SourceAssetID: candidate.SourceAssetID, SourcePath: candidate.SourcePath,
		SourceFingerprint: candidate.SourceFingerprint, SourceRevision: candidate.SourceRevision,
		ResolvedProfileFingerprint:  candidate.ResolvedProfileFingerprint,
		EffectiveProfileFingerprint: candidate.EffectiveProfileFingerprint,
		EffectiveEncoder:            candidate.EffectiveEncoder, ConfigurationHash: candidate.ConfigurationIdentity,
		ProfileID: candidate.Profile.ID, ProfileVersion: candidate.Profile.ProfileVersion, Status: testEncodeReady,
		RequestedConfiguration: models.JSONMap{
			"samplingPlan": candidate.Plan, "samplingPlanIdentity": candidate.Plan.PlanIdentity,
			"profileIdentityContractVersion": profileIdentityContractVersion,
			"profileDefinitionFingerprint":   candidate.ResolvedProfileFingerprint,
		},
		EffectiveConfiguration: effective,
		ValidationReport:       models.JSONMap{"perceptualEvidence": evidence},
	}
}

func addQualificationArtifact(evidence PerceptualEvidenceSet, kind, referenceSeverity, candidateSeverity, status string) PerceptualEvidenceSet {
	if len(evidence.Windows) == 0 || evidence.Windows[0].PairIdentity == nil {
		return evidence
	}
	filtered := make([]ArtifactEvidence, 0, len(evidence.ArtifactEvidence)+1)
	for _, existing := range evidence.ArtifactEvidence {
		if existing.Kind != kind || existing.WindowIdentity != evidence.Windows[0].WindowIdentity {
			filtered = append(filtered, existing)
		}
	}
	evidence.ArtifactEvidence = filtered
	evidence.ArtifactEvidence = append(evidence.ArtifactEvidence, ArtifactEvidence{
		Kind: kind, Status: status, Scope: "pair_comparison", Version: "restoration-analysis-v1",
		WindowIdentity: evidence.Windows[0].WindowIdentity, PairIdentity: evidence.Windows[0].PairIdentity,
		ReferenceSeverity: referenceSeverity, CandidateSeverity: candidateSeverity,
		ReferenceConfidence: "high", CandidateConfidence: "high",
	})
	canonical, _ := ComputePerceptualEvidenceSetIdentity(evidence)
	return canonical
}

func addQualificationArtifactDefaults(evidence PerceptualEvidenceSet, severity string) PerceptualEvidenceSet {
	for _, kind := range []string{"blocking", "blur_detail_loss", "banding", "ringing_halo", "grain_texture_loss", "temporal_instability", "chroma_degradation"} {
		evidence = addQualificationArtifact(evidence, kind, severity, severity, "available")
	}
	return evidence
}

func qualificationRuleByKey(assessment ProfileQualificationAssessment, key string) (ProfileQualificationRuleResult, bool) {
	for _, rule := range assessment.Rules {
		if rule.Key == key {
			return rule, true
		}
	}
	return ProfileQualificationRuleResult{}, false
}

func TestResolvedProfileFingerprintIncludesConfigurationAndVersion(t *testing.T) {
	base := models.Profile{ID: 4, ProfileVersion: 2, QualityValue: 25, OptimizationIntent: "balanced", WorkerConfig: models.JSONMap{"gopSize": 66}}
	baseFingerprint, err := resolvedProfileFingerprint(base)
	if err != nil {
		t.Fatal(err)
	}
	changed := base
	changed.QualityValue++
	changedFingerprint, err := resolvedProfileFingerprint(changed)
	if err != nil {
		t.Fatal(err)
	}
	if baseFingerprint == changedFingerprint {
		t.Fatal("modified profile retained its resolved fingerprint")
	}
	changed = base
	changed.ProfileVersion++
	versionFingerprint, err := resolvedProfileFingerprint(changed)
	if err != nil {
		t.Fatal(err)
	}
	if baseFingerprint == versionFingerprint {
		t.Fatal("profile version change retained its resolved fingerprint")
	}
	changed = base
	changed.ID++
	idFingerprint, err := resolvedProfileFingerprint(changed)
	if err != nil {
		t.Fatal(err)
	}
	if baseFingerprint != idFingerprint {
		t.Fatal("database ID alone changed the content fingerprint")
	}
	for index, scope := range []string{"path", "source_group", "logical_group", "asset"} {
		assigned := base
		assigned.ID = uint(99 + index)
		assigned.Name = "Same definition through another assignment"
		assigned.Scope = scope
		assigned.WorkerConfig = cloneWorkerConfig(base.WorkerConfig)
		assigned.WorkerConfig["qsvAssetAnalysisPath"] = "/raw/another-asset.mkv"
		assignedFingerprint, err := resolvedProfileFingerprint(assigned)
		if err != nil {
			t.Fatal(err)
		}
		if baseFingerprint != assignedFingerprint {
			t.Fatalf("assignment scope %q or per-asset analysis path changed the reusable definition identity", scope)
		}
	}
}

func TestProfileDefinitionIdentitySurvivesQueueFreezeWithoutUpgradingLegacy(t *testing.T) {
	profile := models.Profile{ID: 17, Name: "Reusable", ProfileVersion: 3, VideoCodec: "hevc", QualityValue: 24}
	definitionFingerprint, err := resolvedProfileFingerprint(profile)
	if err != nil {
		t.Fatal(err)
	}
	job := models.QueueJob{ProfileSnapshot: models.JSONMap{
		profileIdentitySnapshotKey: profileIdentityMetadata{
			ContractVersion: profileIdentityContractVersion, DefinitionFingerprint: definitionFingerprint,
		},
	}}
	resolved := profile
	resolved.WorkerConfig = models.JSONMap{"qsvAssetAnalysisPath": "/raw/concierto.mkv", "hevcLevelEffective": "5.1"}
	if err := freezeQueuePreparedProfileSnapshot(nil, &job, resolved); err != nil {
		t.Fatal(err)
	}
	identity, ok := profileIdentityFromSnapshot(job.ProfileSnapshot)
	if !ok || identity.DefinitionFingerprint != definitionFingerprint {
		t.Fatalf("queue freeze lost the captured definition identity: %#v", job.ProfileSnapshot[profileIdentitySnapshotKey])
	}

	legacy := models.QueueJob{ProfileSnapshot: models.JSONMap{}}
	if err := freezeQueuePreparedProfileSnapshot(nil, &legacy, resolved); err != nil {
		t.Fatal(err)
	}
	if _, ok := profileIdentityFromSnapshot(legacy.ProfileSnapshot); ok {
		t.Fatalf("queue freeze synthesized a reusable identity for a legacy snapshot: %#v", legacy.ProfileSnapshot[profileIdentitySnapshotKey])
	}
}

func TestEvaluateProfileQualificationPassAndCurrentIdentity(t *testing.T) {
	candidate, evidence := profileQualificationFixture(t, true, 95)
	evidence = addQualificationArtifactDefaults(evidence, "medium")
	assessment := EvaluateProfileQualification(candidate, evidence)
	if assessment.Outcome != models.QualityOutcomePass {
		t.Fatalf("qualification outcome=%s rules=%#v", assessment.Outcome, assessment.Rules)
	}
	if assessment.WorstWindowIdentity != "" {
		t.Fatalf("passing evidence selected unexpected worst window %q", assessment.WorstWindowIdentity)
	}
	missingAsset := candidate
	missingAsset.SourceAssetID = 0
	if outcome := EvaluateProfileQualification(missingAsset, evidence).Outcome; outcome != models.QualityOutcomeReviewRequired {
		t.Fatalf("missing AssetRecord identity produced %s", outcome)
	}
	missingTestEncode := candidate
	missingTestEncode.TestEncodeID = 0
	if outcome := EvaluateProfileQualification(missingTestEncode, evidence).Outcome; outcome != models.QualityOutcomeReviewRequired {
		t.Fatalf("missing Test Encode generation identity produced %s", outcome)
	}
	record := models.ProfileQualification{
		ID: assessment.ID, TestEncodeID: candidate.TestEncodeID, SourceAssetID: candidate.SourceAssetID,
		SourcePath: candidate.SourcePath, SourceRevision: candidate.SourceRevision, SourceFingerprint: candidate.SourceFingerprint,
		ProfileID: candidate.Profile.ID, ProfileVersion: candidate.Profile.ProfileVersion,
		ResolvedProfileFingerprint:  candidate.ResolvedProfileFingerprint,
		EffectiveProfileFingerprint: candidate.EffectiveProfileFingerprint,
		ConfigurationHash:           candidate.ConfigurationIdentity,
		CandidateIdentity:           assessment.CandidateIdentity, CandidateRevision: assessment.CandidateRevision,
		SamplingPlanSchemaVersion: candidate.Plan.SchemaVersion, SamplingPlanPolicyVersion: candidate.Plan.PolicyVersion,
		SamplingPlanIdentity: candidate.Plan.PlanIdentity, PerceptualEvidenceIdentity: evidence.Identity,
		ReferenceIdentity: assessment.ReferenceIdentity, QualityPolicyVersion: profileQualificationPolicyVersion,
	}
	if current, reason := ProfileQualificationIsCurrent(record, candidate, evidence); !current {
		t.Fatalf("identical qualification was stale: %s", reason)
	}
	changedTestEncode := candidate
	changedTestEncode.TestEncodeID++
	if current, _ := ProfileQualificationIsCurrent(record, changedTestEncode, evidence); current {
		t.Fatal("another Test Encode generation retained qualification")
	}
	changedProfile := candidate
	changedProfile.Profile.QualityValue++
	if current, _ := ProfileQualificationIsCurrent(record, changedProfile, evidence); current {
		t.Fatal("profile modification with unchanged database ID retained qualification")
	}
	changedSource := candidate
	changedSource.SourceFingerprint = "different-source-fingerprint"
	if current, _ := ProfileQualificationIsCurrent(record, changedSource, evidence); current {
		t.Fatal("source fingerprint change retained qualification")
	}
	changedCandidate := evidence
	changedCandidate.CandidateManifest = append([]CandidateManifestEntry(nil), evidence.CandidateManifest...)
	changedCandidate.CandidateManifest[0].Candidate.OutputRevision = "replacement-candidate-revision"
	if current, _ := ProfileQualificationIsCurrent(record, candidate, changedCandidate); current {
		t.Fatal("candidate identity change retained qualification")
	}
	changedReference := evidence
	changedReference.ReferenceManifest = append([]ReferenceManifestEntry(nil), evidence.ReferenceManifest...)
	changedReference.ReferenceManifest[0].Reference.SourceRevision = "replacement-reference-source"
	if current, _ := ProfileQualificationIsCurrent(record, candidate, changedReference); current {
		t.Fatal("reference identity change retained qualification")
	}
	changedPlan := candidate
	changedPlan.Plan.PolicyVersion = "sampling-policy-v2"
	if current, _ := ProfileQualificationIsCurrent(record, changedPlan, evidence); current {
		t.Fatal("SamplingPlan policy change retained qualification")
	}
	changedPolicy := record
	changedPolicy.QualityPolicyVersion = "profile-qualification-policy-v0"
	if current, _ := ProfileQualificationIsCurrent(changedPolicy, candidate, evidence); current {
		t.Fatal("policy version change retained qualification")
	}
}

func TestProfileQualificationArtifactRegressionUsesReferenceDelta(t *testing.T) {
	candidate, evidence := profileQualificationFixture(t, true, 95)
	evidence = addQualificationArtifactDefaults(evidence, "high")
	equal := EvaluateProfileQualification(candidate, evidence)
	blocking, found := qualificationRuleByKey(equal, "artifact.blocking")
	if !found || blocking.Outcome != models.QualityOutcomePass || equal.Outcome != models.QualityOutcomePass {
		t.Fatalf("equal source/candidate blocking became regression: rule=%#v overall=%s", blocking, equal.Outcome)
	}
	regressed := addQualificationArtifact(evidence, "blocking", "medium", "high", "available")
	assessment := EvaluateProfileQualification(candidate, regressed)
	blocking, found = qualificationRuleByKey(assessment, "artifact.blocking")
	if !found || blocking.Outcome != models.QualityOutcomeFail || assessment.Outcome != models.QualityOutcomeFail {
		t.Fatalf("high-confidence blocking regression was not a hard failure: rule=%#v overall=%s", blocking, assessment.Outcome)
	}
	banding := addQualificationArtifact(evidence, "banding", "low", "high", "available")
	assessment = EvaluateProfileQualification(candidate, banding)
	bandingRule, found := qualificationRuleByKey(assessment, "artifact.banding")
	if !found || bandingRule.Outcome != models.QualityOutcomeReviewRequired || assessment.Outcome != models.QualityOutcomeReviewRequired {
		t.Fatalf("banding regression did not require review: rule=%#v overall=%s", bandingRule, assessment.Outcome)
	}
	blur := addQualificationArtifact(evidence, "blur_detail_loss", "low", "medium", "available")
	assessment = EvaluateProfileQualification(candidate, blur)
	blurRule, found := qualificationRuleByKey(assessment, "artifact.blur_detail_loss")
	if !found || blurRule.Outcome != models.QualityOutcomeReviewRequired {
		t.Fatalf("detail-loss regression did not require review: %#v", blurRule)
	}
	grain := addQualificationArtifact(evidence, "grain_texture_loss", "low", "medium", "available")
	assessment = EvaluateProfileQualification(candidate, grain)
	grainRule, found := qualificationRuleByKey(assessment, "artifact.grain_texture_loss")
	if !found || grainRule.Outcome != models.QualityOutcomeReviewRequired {
		t.Fatalf("supported grain/texture regression did not require review: %#v", grainRule)
	}
}

func TestProfileQualificationUnsupportedAndMissingEvidenceReduceConfidence(t *testing.T) {
	candidate, evidence := profileQualificationFixture(t, true, 95)
	evidence = addQualificationArtifactDefaults(evidence, "medium")
	for index := range evidence.ArtifactEvidence {
		if evidence.ArtifactEvidence[index].Kind == "temporal_instability" {
			evidence.ArtifactEvidence[index].Status = "unsupported"
			evidence.ArtifactEvidence[index].Scope = ""
			evidence.ArtifactEvidence[index].PairIdentity = nil
		}
	}
	evidence, _ = ComputePerceptualEvidenceSetIdentity(evidence)
	assessment := EvaluateProfileQualification(candidate, evidence)
	unsupported, found := qualificationRuleByKey(assessment, "artifact.temporal_instability")
	if !found || unsupported.Outcome != "UNAVAILABLE" || assessment.Outcome != models.QualityOutcomePassWithWarnings {
		t.Fatalf("unsupported temporal signal became a failure: rule=%#v overall=%s", unsupported, assessment.Outcome)
	}
	missingHardScenes, noHardEvidence := profileQualificationFixture(t, false, 95)
	noHardEvidence = addQualificationArtifactDefaults(noHardEvidence, "medium")
	assessment = EvaluateProfileQualification(missingHardScenes, noHardEvidence)
	if assessment.Outcome != models.QualityOutcomeReviewRequired {
		t.Fatalf("missing hard scenes produced %s", assessment.Outcome)
	}
	noCandidate := noHardEvidence
	noCandidate.CandidateManifest = nil
	noCandidate, _ = ComputePerceptualEvidenceSetIdentity(noCandidate)
	assessment = EvaluateProfileQualification(missingHardScenes, noCandidate)
	if assessment.Outcome != models.QualityOutcomeReviewRequired {
		t.Fatalf("missing candidate manifest produced %s", assessment.Outcome)
	}
	staleReference := evidence
	staleReference.ReferenceManifest = append([]ReferenceManifestEntry(nil), evidence.ReferenceManifest...)
	staleReference.ReferenceManifest[0].Reference.SourceRevision = "replacement-source"
	assessment = EvaluateProfileQualification(candidate, staleReference)
	if assessment.Outcome != models.QualityOutcomeReviewRequired {
		t.Fatalf("stale reference identity produced %s", assessment.Outcome)
	}
}

func TestProfileQualificationOutcomeAggregationAndOrderingAreDeterministic(t *testing.T) {
	for _, test := range []struct {
		name   string
		rules  []ProfileQualificationRuleResult
		result string
	}{
		{name: "pass", rules: []ProfileQualificationRuleResult{{Outcome: models.QualityOutcomePass, Disposition: "blocking"}}, result: models.QualityOutcomePass},
		{name: "warnings", rules: []ProfileQualificationRuleResult{{Outcome: models.QualityOutcomePass, Disposition: "blocking"}, {Outcome: "UNAVAILABLE", Disposition: "advisory"}}, result: models.QualityOutcomePassWithWarnings},
		{name: "review", rules: []ProfileQualificationRuleResult{{Outcome: models.QualityOutcomeReviewRequired, Disposition: "blocking"}}, result: models.QualityOutcomeReviewRequired},
		{name: "fail outranks review", rules: []ProfileQualificationRuleResult{{Outcome: models.QualityOutcomeReviewRequired, Disposition: "blocking"}, {Outcome: models.QualityOutcomeFail, Disposition: "blocking"}}, result: models.QualityOutcomeFail},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := aggregateProfileQualificationRules(test.rules); got != test.result {
				t.Fatalf("aggregate=%s want=%s", got, test.result)
			}
		})
	}
	candidate, evidence := profileQualificationFixture(t, true, 95)
	evidence = addQualificationArtifactDefaults(evidence, "medium")
	first := EvaluateProfileQualification(candidate, evidence)
	permuted := evidence
	permuted.ArtifactEvidence = append([]ArtifactEvidence(nil), evidence.ArtifactEvidence...)
	for left, right := 0, len(permuted.ArtifactEvidence)-1; left < right; left, right = left+1, right-1 {
		permuted.ArtifactEvidence[left], permuted.ArtifactEvidence[right] = permuted.ArtifactEvidence[right], permuted.ArtifactEvidence[left]
	}
	permuted, _ = ComputePerceptualEvidenceSetIdentity(permuted)
	second := EvaluateProfileQualification(candidate, permuted)
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	if !reflect.DeepEqual(firstJSON, secondJSON) {
		t.Fatalf("rule order/explanations changed with evidence enumeration order:\n%s\n%s", firstJSON, secondJSON)
	}
}

func TestProfileQualificationDoesNotTreatBPPPFAsMeasuredQuality(t *testing.T) {
	candidate, evidence := profileQualificationFixture(t, true, 95)
	evidence = addQualificationArtifactDefaults(evidence, "medium")
	assessment := EvaluateProfileQualification(candidate, evidence)
	for _, rule := range assessment.Rules {
		if rule.Key == "bpppf" || strings.Contains(rule.Explanation, "BPPPF") {
			t.Fatalf("BPPPF leaked into measured qualification rules: %#v", rule)
		}
	}
}

func TestProfileQualificationPersistenceIsIdempotentAndUsesFrozenEvidence(t *testing.T) {
	candidate, evidence := profileQualificationFixture(t, true, 95)
	evidence = addQualificationArtifactDefaults(evidence, "medium")
	db := testEncodeTestDB(t, "profile-qualification-persistence", &models.ProfileQualification{})
	testEncode := profileQualificationTestEncode(candidate, evidence)
	if err := db.Create(&testEncode).Error; err != nil {
		t.Fatal(err)
	}
	handler := AssetHandler{db: db}
	first, err := handler.evaluateAndPersistProfileQualification(testEncode)
	if err != nil {
		t.Fatalf("persist first evaluation: %v", err)
	}
	second, err := handler.evaluateAndPersistProfileQualification(testEncode)
	if err != nil {
		t.Fatalf("repeat evaluation: %v", err)
	}
	if first.ID != second.ID || first.Outcome != models.QualityOutcomePass || second.PerceptualEvidenceIdentity != evidence.Identity {
		t.Fatalf("repeated evaluation changed qualification identity or outcome: first=%#v second=%#v", first, second)
	}
	var count int64
	if err := db.Model(&models.ProfileQualification{}).Where("test_encode_id = ?", testEncode.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("qualification persistence is not idempotent: count=%d err=%v", count, err)
	}
	historical := testEncode
	historical.ID = 18
	historical.SourceRevision = ""
	historical.ResolvedProfileFingerprint = ""
	if err := db.Create(&historical).Error; err != nil {
		t.Fatal(err)
	}
	if err := handler.attachTestEncodeQualifications([]models.TestEncode{historical}); err != nil {
		t.Fatal(err)
	}
	if historical.ProfileQualification != nil {
		t.Fatalf("historical evidence was synthesized into a qualification: %#v", historical.ProfileQualification)
	}
}

func TestLegacyTestEncodeQualificationReportsMissingIdentityStage(t *testing.T) {
	candidate, evidence := profileQualificationFixture(t, true, 95)
	test := profileQualificationTestEncode(candidate, evidence)
	test.ResolvedProfileFingerprint = "legacy-unversioned-fingerprint"
	test.EffectiveProfileFingerprint = ""
	test.ConfigurationHash = "legacy-configuration-hash"
	stale, reason := profileQualificationStaleness(test, models.ProfileQualification{})
	if !stale || !strings.Contains(reason, "unsupported profile-definition fingerprint version") {
		t.Fatalf("legacy Test Encode did not report its stale identity stage: stale=%t reason=%q", stale, reason)
	}
}
