package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/scheduler"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const profileQualificationPreflightMaxCandidates = 10000

func profileQualificationInputs(test models.TestEncode) (ProfileQualificationCandidate, PerceptualEvidenceSet, error) {
	var candidate ProfileQualificationCandidate
	candidate.SourceAssetID = test.SourceAssetID
	candidate.SourcePath = test.SourcePath
	candidate.SourceRevision = test.SourceRevision
	candidate.SourceFingerprint = test.SourceFingerprint
	candidate.TestEncodeID = test.ID
	candidate.ConfigurationIdentity = test.ConfigurationHash
	candidate.ResolvedProfileFingerprint = test.ResolvedProfileFingerprint
	if test.RequestedConfiguration != nil {
		if err := decodeProfileQualificationValue(test.RequestedConfiguration["samplingPlan"], &candidate.Plan); err != nil {
			return candidate, PerceptualEvidenceSet{}, fmt.Errorf("decode frozen SamplingPlan: %w", err)
		}
	}
	profileSnapshot := test.EffectiveConfiguration["profile"]
	if profileSnapshot != nil {
		if err := decodeProfileQualificationValue(profileSnapshot, &candidate.Profile); err != nil {
			return candidate, PerceptualEvidenceSet{}, fmt.Errorf("decode resolved profile snapshot: %w", err)
		}
	} else if test.RequestedConfiguration != nil {
		if raw := test.RequestedConfiguration["labProfile"]; raw != nil {
			if err := decodeProfileQualificationValue(raw, &candidate.Profile); err != nil {
				return candidate, PerceptualEvidenceSet{}, fmt.Errorf("decode requested LAB profile snapshot: %w", err)
			}
		}
	}
	if candidate.Profile.ID == 0 {
		candidate.Profile.ID = test.ProfileID
	}
	if candidate.Profile.ProfileVersion < 1 {
		candidate.Profile.ProfileVersion = test.ProfileVersion
	}
	if candidate.Profile.ID == 0 {
		candidate.Profile.ID = test.ProfileID
	}
	if candidate.Profile.ProfileVersion < 1 {
		candidate.Profile.ProfileVersion = test.ProfileVersion
	}
	var evidence PerceptualEvidenceSet
	if raw := test.ValidationReport["perceptualEvidence"]; raw != nil {
		if err := decodeProfileQualificationValue(raw, &evidence); err != nil {
			return candidate, evidence, fmt.Errorf("decode persisted perceptual evidence: %w", err)
		}
	}
	return candidate, evidence, nil
}

func decodeProfileQualificationValue(raw any, target any) error {
	if raw == nil {
		return errors.New("value is missing")
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, target)
}

func profileQualificationModel(candidate ProfileQualificationCandidate, evidence PerceptualEvidenceSet, assessment ProfileQualificationAssessment, now time.Time) models.ProfileQualification {
	rules := make(models.JSONList, 0, len(assessment.Rules))
	for _, rule := range assessment.Rules {
		rules = append(rules, rule)
	}
	warnings := make(models.JSONList, 0, len(assessment.Warnings))
	for _, warning := range assessment.Warnings {
		warnings = append(warnings, warning)
	}
	return models.ProfileQualification{
		ID: assessment.ID, TestEncodeID: candidate.TestEncodeID,
		SourceAssetID: candidate.SourceAssetID, SourcePath: candidate.SourcePath,
		SourceRevision: candidate.SourceRevision, SourceFingerprint: candidate.SourceFingerprint,
		ProfileID: candidate.Profile.ID, ProfileVersion: candidate.Profile.ProfileVersion,
		ResolvedProfileFingerprint: candidate.ResolvedProfileFingerprint,
		CandidateIdentity:          assessment.CandidateIdentity, CandidateRevision: assessment.CandidateRevision,
		SamplingPlanSchemaVersion:  candidate.Plan.SchemaVersion,
		SamplingPlanPolicyVersion:  candidate.Plan.PolicyVersion,
		SamplingPlanIdentity:       candidate.Plan.PlanIdentity,
		PerceptualEvidenceIdentity: assessment.PerceptualEvidenceIdentity,
		ReferenceIdentity:          assessment.ReferenceIdentity,
		QualityPolicyVersion:       assessment.PolicyVersion, Outcome: assessment.Outcome,
		Rules: rules, Warnings: warnings, QualifiedScope: assessment.QualifiedScope,
		EvidenceProvenance:  assessment.EvidenceProvenance,
		WorstWindowIdentity: assessment.WorstWindowIdentity,
		WorstSceneCategory:  assessment.WorstSceneCategory,
		WorstRuleKeys:       append(models.StringList(nil), assessment.WorstRuleKeys...),
		CreatedAt:           now,
	}
}

func (h AssetHandler) evaluateAndPersistProfileQualification(test models.TestEncode) (models.ProfileQualification, error) {
	candidate, evidence, err := profileQualificationInputs(test)
	if err != nil {
		return models.ProfileQualification{}, err
	}
	assessment := EvaluateProfileQualification(candidate, evidence)
	record := profileQualificationModel(candidate, evidence, assessment, time.Now().UTC())
	var existing models.ProfileQualification
	if err := h.db.First(&existing, "id = ?", record.ID).Error; err == nil {
		return existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return models.ProfileQualification{}, err
	}
	if err := h.db.Create(&record).Error; err != nil {
		var raced models.ProfileQualification
		if readErr := h.db.First(&raced, "id = ?", record.ID).Error; readErr == nil {
			return raced, nil
		}
		return models.ProfileQualification{}, err
	}
	return record, nil
}

func (h AssetHandler) EvaluateTestEncodeProfileQualification(c *gin.Context) {
	test, ok := h.testEncodeFromParam(c)
	if !ok {
		return
	}
	if test.Status != testEncodeReady || strings.TrimSpace(test.OutputPath) == "" {
		c.JSON(http.StatusConflict, gin.H{"error": "Profile Qualification requires a ready Test Encode candidate"})
		return
	}
	h.decorateTestEncode(&test)
	currentRevision, revisionErr := queuePreflightSourceRevision(test.SourcePath)
	if test.Stale || revisionErr != nil || currentRevision != test.SourceRevision {
		c.JSON(http.StatusConflict, gin.H{"error": "Source or resolved profile changed; create a new Test Encode before qualification"})
		return
	}
	if info, err := os.Stat(test.OutputPath); err != nil || info.IsDir() {
		c.JSON(http.StatusConflict, gin.H{"error": "Test Encode candidate output is unavailable"})
		return
	}
	qualification, err := h.evaluateAndPersistProfileQualification(test)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, qualification)
}

func (h AssetHandler) attachTestEncodeQualifications(tests []models.TestEncode) error {
	if len(tests) == 0 || !h.db.Migrator().HasTable(&models.ProfileQualification{}) {
		return nil
	}
	ids := make([]uint, 0, len(tests))
	for _, test := range tests {
		ids = append(ids, test.ID)
	}
	var records []models.ProfileQualification
	if err := h.db.Where("test_encode_id IN ?", ids).Order("created_at DESC").Order("id DESC").Find(&records).Error; err != nil {
		return fmt.Errorf("load Test Encode Profile Qualifications: %w", err)
	}
	latest := make(map[uint]models.ProfileQualification, len(ids))
	for _, record := range records {
		if _, exists := latest[record.TestEncodeID]; !exists {
			latest[record.TestEncodeID] = record
		}
	}
	for index := range tests {
		record, ok := latest[tests[index].ID]
		if !ok {
			continue
		}
		test := &tests[index]
		record.Stale, record.StaleReason = profileQualificationStaleness(*test, record)
		test.ProfileQualification = &record
	}
	return nil
}

func profileQualificationStaleness(test models.TestEncode, record models.ProfileQualification) (bool, string) {
	if test.Stale {
		return true, "Test Encode source or resolved configuration is stale"
	}
	if record.QualityPolicyVersion != profileQualificationPolicyVersion {
		return true, "Qualification policy version changed"
	}
	if record.SourceRevision == "" || record.SourceRevision != test.SourceRevision || record.SourceFingerprint == "" || record.SourceFingerprint != test.SourceFingerprint {
		return true, "Source identity changed or was not captured"
	}
	if record.ResolvedProfileFingerprint == "" || record.ResolvedProfileFingerprint != test.ResolvedProfileFingerprint || record.ProfileVersion != test.ProfileVersion {
		return true, "Resolved profile fingerprint changed or was not captured"
	}
	if record.SamplingPlanIdentity == "" || record.SamplingPlanIdentity != workerStringValue(test.RequestedConfiguration["samplingPlanIdentity"]) {
		return true, "Frozen SamplingPlan identity changed or was not captured"
	}
	if record.PerceptualEvidenceIdentity != "" {
		candidate, evidence, err := profileQualificationInputs(test)
		if err != nil {
			return true, "Perceptual evidence or its frozen identities are unavailable"
		}
		canonical, canonicalErr := ComputePerceptualEvidenceSetIdentity(evidence)
		valid, reason := profileQualificationEvidenceMatches(candidate, evidence, canonical, canonicalErr)
		if !valid || canonical.Identity != record.PerceptualEvidenceIdentity {
			if reason == "" {
				reason = "Perceptual evidence identity changed"
			}
			return true, reason
		}
		candidateID, candidateRevision, referenceID := qualificationEvidenceIdentities(evidence)
		if candidateID != record.CandidateIdentity || candidateRevision != record.CandidateRevision || referenceID != record.ReferenceIdentity {
			return true, "Candidate or reference identity changed"
		}
	}
	return false, ""
}

type profileQualificationPreflightView struct {
	Status                     string `json:"status"`
	Message                    string `json:"message,omitempty"`
	QualificationID            string `json:"qualificationId,omitempty"`
	Outcome                    string `json:"outcome,omitempty"`
	PolicyVersion              string `json:"policyVersion,omitempty"`
	TestEncodeID               uint   `json:"testEncodeId,omitempty"`
	SourceAssetID              uint   `json:"sourceAssetId,omitempty"`
	SourceRevision             string `json:"sourceRevision,omitempty"`
	SourceFingerprint          string `json:"sourceFingerprint,omitempty"`
	ResolvedProfileFingerprint string `json:"resolvedProfileFingerprint,omitempty"`
	PerceptualEvidenceIdentity string `json:"perceptualEvidenceIdentity,omitempty"`
	ReferenceIdentity          string `json:"referenceIdentity,omitempty"`
	CandidateIdentity          string `json:"candidateIdentity,omitempty"`
	WorstWindowIdentity        string `json:"worstWindowIdentity,omitempty"`
	WorstSceneCategory         string `json:"worstSceneCategory,omitempty"`
	Rules                      []any  `json:"rules,omitempty"`
	Warnings                   []any  `json:"warnings,omitempty"`
}

func attachPreflightProfileQualifications(db *gorm.DB, assets []queueBatchPreflightAsset) error {
	if db == nil || len(assets) == 0 {
		return nil
	}
	profiles := make(map[string]string, len(assets))
	paths := make([]string, 0, len(assets))
	for index := range assets {
		asset := &assets[index]
		if asset.ResolvedJob == nil || len(asset.Constraints) == 0 || asset.SourceRevision == "" {
			continue
		}
		profile, err := scheduler.RestoreProfileSnapshot(asset.ResolvedJob.ProfileSnapshot)
		if err != nil {
			continue
		}
		fingerprint, err := resolvedProfileFingerprint(profile)
		if err != nil {
			continue
		}
		profiles[filepath.Clean(asset.MediaPath)] = fingerprint
		paths = append(paths, filepath.Clean(asset.MediaPath))
	}
	if len(paths) == 0 {
		return nil
	}
	var tests []models.TestEncode
	if err := db.Where("source_path IN ? AND status = ? AND deleted_at IS NULL", paths, testEncodeReady).
		Order("created_at DESC").Order("id DESC").Limit(profileQualificationPreflightMaxCandidates).Find(&tests).Error; err != nil {
		return fmt.Errorf("load Preflight Profile Qualification candidates: %w", err)
	}
	testIDs := make([]uint, 0, len(tests))
	for _, test := range tests {
		testIDs = append(testIDs, test.ID)
	}
	qualifications := map[uint]models.ProfileQualification{}
	if len(testIDs) > 0 && db.Migrator().HasTable(&models.ProfileQualification{}) {
		var records []models.ProfileQualification
		if err := db.Where("test_encode_id IN ? AND quality_policy_version = ?", testIDs, profileQualificationPolicyVersion).
			Order("created_at DESC").Order("id DESC").Limit(profileQualificationPreflightMaxCandidates).Find(&records).Error; err != nil {
			return fmt.Errorf("load Preflight Profile Qualifications: %w", err)
		}
		for _, record := range records {
			if _, exists := qualifications[record.TestEncodeID]; !exists {
				qualifications[record.TestEncodeID] = record
			}
		}
	}
	testsByPath := map[string][]models.TestEncode{}
	for _, test := range tests {
		path := filepath.Clean(test.SourcePath)
		testsByPath[path] = append(testsByPath[path], test)
	}
	for index := range assets {
		asset := &assets[index]
		path := filepath.Clean(asset.MediaPath)
		profileFingerprint, hasProfile := profiles[path]
		if !hasProfile {
			continue
		}
		view := profileQualificationPreflightView{Status: "NOT_QUALIFIED", Message: "Profile not yet qualified for this asset/profile evidence."}
		var matchingTest *models.TestEncode
		for _, test := range testsByPath[path] {
			if test.SourceRevision == asset.SourceRevision && test.ResolvedProfileFingerprint == profileFingerprint &&
				(asset.AssetRecordID == nil || test.SourceAssetID == *asset.AssetRecordID) {
				matchingTest = &test
				break
			}
		}
		if matchingTest != nil {
			test := *matchingTest
			if record, exists := qualifications[test.ID]; exists {
				stale, staleReason := profileQualificationStaleness(test, record)
				if !stale {
					view = profileQualificationPreflightView{
						Status: record.Outcome, QualificationID: record.ID, Outcome: record.Outcome,
						PolicyVersion: record.QualityPolicyVersion, TestEncodeID: test.ID,
						SourceAssetID: record.SourceAssetID, SourceRevision: record.SourceRevision,
						SourceFingerprint:          record.SourceFingerprint,
						ResolvedProfileFingerprint: record.ResolvedProfileFingerprint,
						PerceptualEvidenceIdentity: record.PerceptualEvidenceIdentity,
						ReferenceIdentity:          record.ReferenceIdentity, CandidateIdentity: record.CandidateIdentity,
						WorstWindowIdentity: record.WorstWindowIdentity, WorstSceneCategory: record.WorstSceneCategory,
						Rules:    append([]any(nil), record.Rules...),
						Warnings: append([]any(nil), record.Warnings...),
					}
				} else {
					view = profileQualificationPreflightView{Status: "STALE", Message: staleReason, QualificationID: record.ID}
				}
			}
		}
		if asset.QualityEvidence == nil {
			asset.QualityEvidence = models.JSONMap{}
		}
		asset.QualityEvidence["profileQualification"] = view
		switch view.Status {
		case models.QualityOutcomePass:
		case models.QualityOutcomePassWithWarnings:
			asset.Warnings = append(asset.Warnings, "Profile Qualification passed with warnings")
		case models.QualityOutcomeReviewRequired, models.QualityOutcomeFail:
			asset.Warnings = append(asset.Warnings, "Profile Qualification requires attention: "+view.Status)
		default:
			asset.Warnings = append(asset.Warnings, view.Message)
		}
	}
	return nil
}
