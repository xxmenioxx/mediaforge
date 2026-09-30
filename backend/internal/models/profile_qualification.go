package models

import "time"

const (
	QualityOutcomePass             = "PASS"
	QualityOutcomePassWithWarnings = "PASS_WITH_WARNINGS"
	QualityOutcomeReviewRequired   = "REVIEW_REQUIRED"
	QualityOutcomeFail             = "FAIL"
)

// ProfileQualification is an immutable pre-batch decision referencing the
// canonical Test Encode and #69 evidence identities. It does not duplicate
// decoded media or metric evidence.
type ProfileQualification struct {
	ID                         string     `json:"id" gorm:"primaryKey;size:96"`
	TestEncodeID               uint       `json:"testEncodeId" gorm:"not null;uniqueIndex:idx_profile_qualification_evaluation,priority:1;index"`
	SourceAssetID              uint       `json:"sourceAssetId" gorm:"not null;index:idx_profile_qualification_scope,priority:1"`
	SourcePath                 string     `json:"sourcePath" gorm:"not null;index"`
	SourceRevision             string     `json:"sourceRevision" gorm:"not null;index:idx_profile_qualification_scope,priority:2"`
	SourceFingerprint          string     `json:"sourceFingerprint" gorm:"not null;index"`
	ProfileID                  uint       `json:"profileId" gorm:"not null;index"`
	ProfileVersion             int        `json:"profileVersion" gorm:"not null"`
	ResolvedProfileFingerprint string     `json:"resolvedProfileFingerprint" gorm:"not null;index:idx_profile_qualification_scope,priority:3"`
	CandidateIdentity          string     `json:"candidateIdentity" gorm:"not null;index"`
	CandidateRevision          string     `json:"candidateRevision" gorm:"not null"`
	SamplingPlanSchemaVersion  string     `json:"samplingPlanSchemaVersion" gorm:"not null"`
	SamplingPlanPolicyVersion  string     `json:"samplingPlanPolicyVersion" gorm:"not null"`
	SamplingPlanIdentity       string     `json:"samplingPlanIdentity" gorm:"not null;index"`
	PerceptualEvidenceIdentity string     `json:"perceptualEvidenceIdentity" gorm:"not null;uniqueIndex:idx_profile_qualification_evaluation,priority:2;index"`
	ReferenceIdentity          string     `json:"referenceIdentity" gorm:"not null"`
	QualityPolicyVersion       string     `json:"qualityPolicyVersion" gorm:"not null;uniqueIndex:idx_profile_qualification_evaluation,priority:3;index:idx_profile_qualification_scope,priority:4"`
	Outcome                    string     `json:"outcome" gorm:"not null;index"`
	Rules                      JSONList   `json:"rules" gorm:"type:json;not null"`
	Warnings                   JSONList   `json:"warnings" gorm:"type:json;not null"`
	QualifiedScope             string     `json:"qualifiedScope" gorm:"not null"`
	EvidenceProvenance         JSONMap    `json:"evidenceProvenance" gorm:"type:json;not null"`
	WorstWindowIdentity        string     `json:"worstWindowIdentity" gorm:"index"`
	WorstSceneCategory         string     `json:"worstSceneCategory"`
	WorstRuleKeys              StringList `json:"worstRuleKeys" gorm:"type:json"`
	CreatedAt                  time.Time  `json:"createdAt" gorm:"not null;index"`

	Stale       bool   `json:"stale,omitempty" gorm:"-"`
	StaleReason string `json:"staleReason,omitempty" gorm:"-"`
}
