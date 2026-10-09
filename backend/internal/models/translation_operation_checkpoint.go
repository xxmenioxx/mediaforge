package models

import "time"

// TranslationOperationCheckpoint retains only bounded recovery metadata for
// the explicit Asset Info Translation operation. It never stores prompts,
// subtitle cues, generated text, credentials, provider responses, or endpoints.
type TranslationOperationCheckpoint struct {
	OperationID                   string    `json:"-" gorm:"primaryKey;size:128"`
	SourceIdentity                string    `json:"-" gorm:"not null;size:512"`
	SourceRevision                string    `json:"-" gorm:"not null;size:512"`
	ProviderConfigurationRevision string    `json:"-" gorm:"not null;size:128"`
	SelectedModel                 string    `json:"-" gorm:"not null;size:256"`
	AdmissionWorkflowID           string    `json:"-" gorm:"not null;size:128"`
	ExecutionPhase                string    `json:"-" gorm:"not null;size:64;index"`
	Deadline                      time.Time `json:"-" gorm:"not null;index"`
	CheckpointVersion             int       `json:"-" gorm:"not null;default:1"`
	RowVersion                    uint64    `json:"-" gorm:"not null;default:1"`
	Cancelled                     bool      `json:"-" gorm:"not null;default:false"`
	InferenceStarted              bool      `json:"-" gorm:"not null;default:false"`
	RequestMetadata               string    `json:"-" gorm:"not null;size:16384"`
	PublicationIntent             string    `json:"-" gorm:"not null;default:'';size:16384"`
	RecoveryReason                string    `json:"-" gorm:"not null;default:'';size:160"`
	CreatedAt                     time.Time `json:"-"`
	UpdatedAt                     time.Time `json:"-"`
}
