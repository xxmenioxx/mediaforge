package models

import "time"

// OCRDictionaryEntry is a bounded literal rule used to propose contextual OCR
// corrections. IDs are assigned by the service and remain stable for the life
// of an entry.
type OCRDictionaryEntry struct {
	ID             string    `json:"id" gorm:"primaryKey;size:96"`
	Language       string    `json:"language" gorm:"size:16;not null;index:idx_ocr_dictionary_language_enabled,priority:1"`
	Pattern        string    `json:"pattern" gorm:"size:1024;not null"`
	Replacement    string    `json:"replacement" gorm:"size:1024;not null"`
	MatchMode      string    `json:"matchMode" gorm:"size:16;not null;default:literal"`
	BoundaryPolicy string    `json:"boundaryPolicy" gorm:"size:16;not null;default:word"`
	LeftContext    string    `json:"leftContext,omitempty" gorm:"size:1024;not null;default:''"`
	RightContext   string    `json:"rightContext,omitempty" gorm:"size:1024;not null;default:''"`
	CasePolicy     string    `json:"casePolicy" gorm:"size:16;not null;default:exact"`
	Priority       int       `json:"priority" gorm:"not null;default:0;index"`
	Enabled        bool      `json:"enabled" gorm:"not null;default:true;index:idx_ocr_dictionary_language_enabled,priority:2"`
	Source         string    `json:"source" gorm:"size:16;not null;default:user;index"`
	RuleVersion    int       `json:"ruleVersion" gorm:"not null;default:1"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

func (OCRDictionaryEntry) TableName() string { return "ocr_dictionary_entries" }
