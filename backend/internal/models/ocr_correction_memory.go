package models

import "time"

// OCRCorrectionMemoryEntry is a deduplicated, user-approved cue correction.
// Its evidence rows retain bounded source/artifact provenance separately.
type OCRCorrectionMemoryEntry struct {
	ID                  uint       `json:"id" gorm:"primaryKey"`
	Fingerprint         string     `json:"-" gorm:"size:64;not null;uniqueIndex"`
	SourceLanguage      string     `json:"sourceLanguage" gorm:"size:32;not null;index:idx_ocr_memory_language_active,priority:1"`
	OriginalText        string     `json:"originalText" gorm:"not null"`
	CorrectedText       string     `json:"correctedText" gorm:"not null"`
	ContextFingerprint  string     `json:"contextFingerprint" gorm:"size:64;not null;index"`
	ScopeType           string     `json:"scopeType" gorm:"size:16;not null;index:idx_ocr_memory_scope,priority:1"`
	ScopeID             string     `json:"scopeId" gorm:"size:256;not null;index:idx_ocr_memory_scope,priority:2"`
	Enabled             bool       `json:"enabled" gorm:"not null;default:true;index:idx_ocr_memory_language_active,priority:2"`
	ApprovalValid       bool       `json:"approvalValid" gorm:"not null;default:true;index:idx_ocr_memory_language_active,priority:3"`
	AcceptedSuggestions int        `json:"acceptedSuggestions" gorm:"not null;default:0"`
	RejectedSuggestions int        `json:"rejectedSuggestions" gorm:"not null;default:0"`
	OccurrenceCount     int        `json:"occurrenceCount" gorm:"not null;default:1"`
	SupersededByID      uint       `json:"supersededById,omitempty" gorm:"index"`
	InvalidatedAt       *time.Time `json:"invalidatedAt,omitempty"`
	ApprovedAt          time.Time  `json:"approvedAt" gorm:"not null;index"`
	ExpiresAt           time.Time  `json:"expiresAt" gorm:"not null;index"`
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
}

func (OCRCorrectionMemoryEntry) TableName() string { return "ocr_correction_memory_entries" }

// OCRCorrectionMemoryEvidence is immutable approval provenance. It contains
// hashed source/artifact revisions and managed scope identities, never paths
// or bitmap data.
type OCRCorrectionMemoryEvidence struct {
	ID                    uint      `json:"id" gorm:"primaryKey"`
	EntryID               uint      `json:"entryId" gorm:"not null;uniqueIndex:idx_ocr_memory_evidence_revision,priority:1;index"`
	SourceRevision        string    `json:"sourceRevision" gorm:"size:64;not null;uniqueIndex:idx_ocr_memory_evidence_revision,priority:2"`
	ArtifactRevision      string    `json:"artifactRevision" gorm:"size:64;not null;uniqueIndex:idx_ocr_memory_evidence_revision,priority:3"`
	CueID                 int       `json:"cueId" gorm:"not null;uniqueIndex:idx_ocr_memory_evidence_revision,priority:4"`
	SourceHash            string    `json:"sourceHash" gorm:"size:64;not null"`
	ApprovalAction        string    `json:"approvalAction" gorm:"size:24;not null"`
	DictionaryRuleID      string    `json:"dictionaryRuleId,omitempty" gorm:"size:96"`
	DictionaryRuleVersion int       `json:"dictionaryRuleVersion,omitempty"`
	ModelIdentifier       string    `json:"modelIdentifier,omitempty" gorm:"size:256"`
	AssetID               uint      `json:"assetId,omitempty" gorm:"index"`
	LibraryScopeID        string    `json:"libraryScopeId,omitempty" gorm:"size:256"`
	TitleScopeID          string    `json:"titleScopeId,omitempty" gorm:"size:256"`
	SeriesScopeID         string    `json:"seriesScopeId,omitempty" gorm:"size:256"`
	ApprovedAt            time.Time `json:"approvedAt" gorm:"not null;index"`
}

func (OCRCorrectionMemoryEvidence) TableName() string { return "ocr_correction_memory_evidence" }
