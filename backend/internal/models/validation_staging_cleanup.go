package models

import "time"

// ValidationStagingCleanupPreview fences one explicit, job-scoped cleanup
// confirmation. It is independent from the singleton Housekeeping preview.
type ValidationStagingCleanupPreview struct {
	ID                       uint       `json:"id" gorm:"primaryKey"`
	PreviewID                string     `json:"previewId" gorm:"not null;uniqueIndex"`
	JobID                    uint       `json:"jobId" gorm:"not null;index"`
	ExecutionNumber          uint       `json:"executionNumber" gorm:"not null"`
	ExecutionPlanID          uint       `json:"executionPlanId" gorm:"not null"`
	ExecutionPlanVersion     int        `json:"executionPlanVersion" gorm:"not null"`
	ExecutionPlanFingerprint string     `json:"executionPlanFingerprint" gorm:"not null"`
	ResultArtifactID         string     `json:"resultArtifactId" gorm:"not null"`
	ResultGenerationToken    string     `json:"resultGenerationToken" gorm:"not null"`
	ResultGenerationVersion  int        `json:"resultGenerationVersion" gorm:"not null"`
	ResultOutputRevision     string     `json:"resultOutputRevision" gorm:"not null"`
	ValidationRunID          string     `json:"validationRunId" gorm:"not null"`
	State                    string     `json:"state" gorm:"not null;index"`
	WorkRoot                 string     `json:"workRoot" gorm:"not null"`
	WorkspacePath            string     `json:"workspacePath" gorm:"not null"`
	TreeIdentity             string     `json:"treeIdentity" gorm:"not null"`
	ManifestJSON             string     `json:"manifestJson" gorm:"type:text;not null"`
	TotalBytes               int64      `json:"totalBytes" gorm:"not null"`
	ExpiresAt                time.Time  `json:"expiresAt" gorm:"not null;index"`
	ConsumedAt               *time.Time `json:"consumedAt"`
	CreatedAt                time.Time  `json:"createdAt" gorm:"not null;index"`
}

// ValidationStagingCleanupEvidence is append-only. Each lifecycle event,
// including attempt start and completion, is a distinct row.
type ValidationStagingCleanupEvidence struct {
	ID                       uint      `json:"id" gorm:"primaryKey"`
	EvidenceID               string    `json:"evidenceId" gorm:"not null;uniqueIndex"`
	AttemptID                string    `json:"attemptId" gorm:"not null;index"`
	JobID                    uint      `json:"jobId" gorm:"not null;index"`
	ExecutionNumber          uint      `json:"executionNumber" gorm:"not null"`
	ExecutionPlanID          uint      `json:"executionPlanId" gorm:"not null"`
	ExecutionPlanVersion     int       `json:"executionPlanVersion" gorm:"not null"`
	ExecutionPlanFingerprint string    `json:"executionPlanFingerprint" gorm:"not null"`
	ResultArtifactID         string    `json:"resultArtifactId" gorm:"not null"`
	ResultGenerationToken    string    `json:"resultGenerationToken" gorm:"not null"`
	ResultGenerationVersion  int       `json:"resultGenerationVersion" gorm:"not null"`
	ResultOutputRevision     string    `json:"resultOutputRevision" gorm:"not null"`
	ValidationRunID          string    `json:"validationRunId" gorm:"not null"`
	PreviewID                string    `json:"previewId" gorm:"not null;index"`
	TreeIdentity             string    `json:"treeIdentity" gorm:"not null"`
	Actor                    string    `json:"actor" gorm:"not null"`
	Action                   string    `json:"action" gorm:"not null"`
	Event                    string    `json:"event" gorm:"not null;index"`
	Outcome                  string    `json:"outcome" gorm:"not null;index"`
	WorkspacePath            string    `json:"workspacePath" gorm:"not null"`
	RemovedPathsJSON         string    `json:"removedPathsJson" gorm:"type:text;not null"`
	RecoveredBytes           int64     `json:"recoveredBytes" gorm:"not null"`
	Reason                   string    `json:"reason" gorm:"type:text"`
	CreatedAt                time.Time `json:"createdAt" gorm:"not null;index"`
}
