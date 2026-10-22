package models

import "time"

// SubtitleTranslationQualityReview stores bounded, secret-free review evidence
// for one completed canonical translation operation. Cue payload is limited to
// flagged review snippets; full source and translated media remain in sidecars.
type SubtitleTranslationQualityReview struct {
	OperationID     string    `json:"operationId" gorm:"primaryKey;size:128"`
	AssetPath       string    `json:"assetPath" gorm:"not null;size:2048;index"`
	SourceIdentity  string    `json:"sourceIdentity" gorm:"not null;size:512"`
	SourceRevision  string    `json:"sourceRevision" gorm:"not null;size:512"`
	RequestMetadata string    `json:"-" gorm:"not null;type:text"`
	TargetsMetadata string    `json:"-" gorm:"not null;type:text"`
	QualityReport   string    `json:"qualityReport" gorm:"not null;type:text"`
	Decisions       string    `json:"decisions" gorm:"not null;type:text"`
	Status          string    `json:"status" gorm:"not null;size:32;index"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}
