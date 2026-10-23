package models

import "time"

// PublisherPublication is the single Publisher-owned delivery identity for a
// completed Queue execution. QueueJob and AssetRecord remain their respective
// execution and original-retention authorities.
type PublisherPublication struct {
	ID                               uint       `json:"id" gorm:"primaryKey"`
	PublicationID                    string     `json:"publicationId" gorm:"not null;uniqueIndex"`
	QueueJobID                       uint       `json:"queueJobId" gorm:"not null;uniqueIndex:idx_publisher_publications_queue_job_execution"`
	ExecutionNumber                  uint       `json:"executionNumber" gorm:"not null;uniqueIndex:idx_publisher_publications_queue_job_execution"`
	ExecutionPlanID                  uint       `json:"executionPlanId" gorm:"not null;index"`
	ExecutionPlanVersion             int        `json:"executionPlanVersion" gorm:"not null"`
	ExecutionPlanFingerprint         string     `json:"executionPlanFingerprint" gorm:"not null"`
	LibraryID                        uint       `json:"libraryId" gorm:"not null;index"`
	DestinationRoot                  string     `json:"destinationRoot" gorm:"not null"`
	DestinationPath                  string     `json:"destinationPath" gorm:"not null"`
	PublishMode                      string     `json:"publishMode" gorm:"not null"`
	ResultArtifactID                 string     `json:"resultArtifactId" gorm:"not null;index"`
	ResultGenerationToken            string     `json:"resultGenerationToken" gorm:"not null"`
	ResultGenerationVersion          int        `json:"resultGenerationVersion" gorm:"not null"`
	ResultOutputRevision             string     `json:"resultOutputRevision" gorm:"not null;index"`
	ValidationRunID                  string     `json:"validationRunId" gorm:"not null;index"`
	ValidationOverrideEventIDs       JSONList   `json:"validationOverrideEventIds,omitempty" gorm:"type:json"`
	SourceAssetRecordID              *uint      `json:"sourceAssetRecordId,omitempty" gorm:"index"`
	SourceAssetRecordCreatedAt       time.Time  `json:"sourceAssetRecordCreatedAt,omitempty"`
	SourceAssetRecordModifiedAt      time.Time  `json:"sourceAssetRecordModifiedAt,omitempty"`
	SourceAssetRecordSizeBytes       int64      `json:"sourceAssetRecordSizeBytes" gorm:"not null;default:0"`
	SourcePath                       string     `json:"sourcePath" gorm:"not null"`
	State                            string     `json:"state" gorm:"not null;index"`
	ManualRequestID                  string     `json:"manualRequestId,omitempty" gorm:"index"`
	ManualRequestStatus              string     `json:"manualRequestStatus,omitempty" gorm:"index"`
	ManualRequestReason              string     `json:"-"`
	ManualRequestAttemptCount        int        `json:"-" gorm:"not null;default:0"`
	ManualRequestedAt                *time.Time `json:"manualRequestedAt,omitempty"`
	ManualRequestCompletedAt         *time.Time `json:"manualRequestCompletedAt,omitempty"`
	HoldReason                       string     `json:"holdReason,omitempty"`
	FailureCode                      string     `json:"failureCode,omitempty"`
	FailureSummary                   string     `json:"failureSummary,omitempty"`
	ActiveAttemptID                  string     `json:"activeAttemptId,omitempty" gorm:"index"`
	LeaseExpiresAt                   *time.Time `json:"leaseExpiresAt,omitempty" gorm:"index"`
	FencingGeneration                uint64     `json:"fencingGeneration" gorm:"not null;default:0"`
	AttemptCount                     int        `json:"attemptCount" gorm:"not null;default:0"`
	Manifest                         JSONList   `json:"manifest" gorm:"type:json;not null"`
	PublishedAt                      *time.Time `json:"publishedAt,omitempty" gorm:"index"`
	OriginalArchiveStatus            string     `json:"originalArchiveStatus" gorm:"not null;default:pending"`
	OriginalArchivedPath             string     `json:"originalArchivedPath,omitempty"`
	OriginalArchiveSourcePath        string     `json:"originalArchiveSourcePath,omitempty"`
	OriginalArchiveTemporaryPath     string     `json:"originalArchiveTemporaryPath,omitempty"`
	OriginalArchiveSizeBytes         int64      `json:"originalArchiveSizeBytes" gorm:"not null;default:0"`
	OriginalArchiveDevice            uint64     `json:"originalArchiveDevice" gorm:"not null;default:0"`
	OriginalArchiveInode             uint64     `json:"originalArchiveInode" gorm:"not null;default:0"`
	OriginalArchiveTempDevice        uint64     `json:"originalArchiveTempDevice" gorm:"not null;default:0"`
	OriginalArchiveTempInode         uint64     `json:"originalArchiveTempInode" gorm:"not null;default:0"`
	OriginalArchiveClaimID           string     `json:"-" gorm:"index"`
	OriginalArchiveLeaseExpiresAt    *time.Time `json:"-" gorm:"index"`
	OriginalArchiveFencingGeneration uint64     `json:"originalArchiveFencingGeneration" gorm:"not null;default:0"`
	OriginalArchiveError             string     `json:"originalArchiveError,omitempty"`
	CreatedAt                        time.Time  `json:"createdAt" gorm:"not null;index"`
	UpdatedAt                        time.Time  `json:"updatedAt" gorm:"not null;index"`
}

// PublisherPublicationAttempt is immutable claim evidence. Mutable phase and
// lease progress lives on PublisherPublication; later outcomes are appended as
// PublisherAttemptEvidence rows.
type PublisherPublicationAttempt struct {
	ID                uint      `json:"id" gorm:"primaryKey"`
	AttemptID         string    `json:"attemptId" gorm:"not null;uniqueIndex"`
	PublicationID     string    `json:"publicationId" gorm:"not null;index"`
	FencingGeneration uint64    `json:"fencingGeneration" gorm:"not null"`
	ActorSource       string    `json:"actorSource" gorm:"not null"`
	ActorID           string    `json:"actorId" gorm:"not null"`
	CreatedAt         time.Time `json:"createdAt" gorm:"not null;index"`
}

// PublisherAttemptEvidence is append-only phase and outcome evidence.
type PublisherAttemptEvidence struct {
	ID                uint      `json:"id" gorm:"primaryKey"`
	EvidenceID        string    `json:"evidenceId" gorm:"not null;uniqueIndex"`
	PublicationID     string    `json:"publicationId" gorm:"not null;index"`
	AttemptID         string    `json:"attemptId" gorm:"not null;index"`
	FencingGeneration uint64    `json:"fencingGeneration" gorm:"not null;default:0"`
	Version           int       `json:"version" gorm:"not null"`
	Phase             string    `json:"phase" gorm:"not null;index"`
	Outcome           string    `json:"outcome" gorm:"not null"`
	Code              string    `json:"code,omitempty"`
	Summary           string    `json:"summary,omitempty"`
	Paths             JSONList  `json:"paths,omitempty" gorm:"type:json"`
	CreatedAt         time.Time `json:"createdAt" gorm:"not null;index"`
}

// PublisherAuditEvent is immutable, versioned evidence for operator decisions.
type PublisherAuditEvent struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	EventID        string    `json:"eventId" gorm:"not null;uniqueIndex"`
	PublicationID  string    `json:"publicationId" gorm:"not null;index"`
	IdempotencyKey string    `json:"-" gorm:"uniqueIndex:idx_publisher_audit_idempotency_key,where:idempotency_key <> ''"`
	Version        int       `json:"version" gorm:"not null"`
	Action         string    `json:"action" gorm:"not null;index"`
	ActorSource    string    `json:"actorSource" gorm:"not null"`
	ActorID        string    `json:"actorId" gorm:"not null"`
	Reason         string    `json:"reason,omitempty"`
	Parameters     JSONMap   `json:"parameters,omitempty" gorm:"type:json"`
	CreatedAt      time.Time `json:"createdAt" gorm:"not null;index"`
}

// AssetPublishedOutbox is the stable, retryable event identity emitted only
// after publication and its archive outcome are durably recorded.
type AssetPublishedOutbox struct {
	ID             uint       `json:"id" gorm:"primaryKey"`
	EventID        string     `json:"eventId" gorm:"not null;uniqueIndex"`
	PublicationID  string     `json:"publicationId" gorm:"not null;uniqueIndex"`
	EventType      string     `json:"eventType" gorm:"not null"`
	Payload        JSONMap    `json:"payload" gorm:"type:json;not null"`
	Status         string     `json:"status" gorm:"not null;default:pending;index"`
	AttemptCount   int        `json:"attemptCount" gorm:"not null;default:0"`
	LeaseExpiresAt *time.Time `json:"leaseExpiresAt,omitempty" gorm:"index"`
	LastError      string     `json:"lastError,omitempty"`
	DeliveredAt    *time.Time `json:"deliveredAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt" gorm:"not null;index"`
	UpdatedAt      time.Time  `json:"updatedAt" gorm:"not null;index"`
}
