package models

import (
	"time"

	"gorm.io/gorm"
)

const QueueSourceAnalysisEvidenceKey = "sourceAnalysisEvidence"
const QueuePreflightSelectedEncoderKey = "queuePreflightSelectedEncoder"

type QueueJob struct {
	ID                          uint       `json:"id" gorm:"primaryKey"`
	ExecutionNumber             *uint      `json:"executionNumber,omitempty" gorm:"uniqueIndex"`
	BatchID                     string     `json:"batchId" gorm:"index"`
	BatchName                   string     `json:"batchName"`
	BatchPosition               int        `json:"batchPosition" gorm:"not null;default:0;index"`
	MediaPath                   string     `json:"mediaPath" gorm:"not null;index"`
	MediaPathKey                string     `json:"-" gorm:"not null;default:'';index"`
	SourceAssetRecordID         *uint      `json:"sourceAssetRecordId,omitempty" gorm:"index"`
	PublishMode                 string     `json:"publishMode" gorm:"not null;default:standard"`
	LibraryID                   uint       `json:"libraryId" gorm:"not null;index;index:idx_queue_jobs_library_published_path_key,priority:1"`
	ProfileID                   uint       `json:"profileId" gorm:"not null"`
	ProfileVersion              int        `json:"profileVersion" gorm:"not null;default:1"`
	ProfileSnapshot             JSONMap    `json:"profileSnapshot" gorm:"type:json"`
	ProfileCapturedAt           *time.Time `json:"profileCapturedAt"`
	ResolvedDestinationSnapshot JSONMap    `json:"resolvedDestinationSnapshot,omitempty" gorm:"type:json"`
	ActiveExecutionPlanID       *uint      `json:"activeExecutionPlanId" gorm:"index"`
	AudioProfileKey             string     `json:"audioProfileKey"`
	TrackProfileKey             string     `json:"trackProfileKey"`
	AudioProfileSnapshot        JSONMap    `json:"audioProfileSnapshot" gorm:"type:json"`
	TrackProfileSnapshot        JSONMap    `json:"trackProfileSnapshot" gorm:"type:json"`
	ProfileResolution           JSONMap    `json:"profileResolution" gorm:"type:json"`
	ProcessingMode              string     `json:"processingMode"`
	Priority                    int        `json:"priority" gorm:"not null;default:5"`
	QueuePosition               int64      `json:"queuePosition" gorm:"not null;default:0;index"`
	Status                      string     `json:"status" gorm:"not null;default:queued"`
	Stage                       string     `json:"stage" gorm:"not null;default:queued;index"`
	StageUpdatedAt              *time.Time `json:"stageUpdatedAt"`
	StageHistory                JSONList   `json:"stageHistory" gorm:"type:json"`
	Progress                    int        `json:"progress" gorm:"not null;default:0"`
	WorkerName                  string     `json:"workerName"`
	OutputPath                  string     `json:"outputPath"`
	SubtitleArtifacts           JSONList   `json:"subtitleArtifacts" gorm:"type:json"`
	PlannedLibraryRoot          string     `json:"plannedLibraryRoot"`
	PlannedPublishedPath        string     `json:"plannedPublishedPath"`
	ErrorMessage                string     `json:"errorMessage"`
	Notes                       string     `json:"notes"`
	ValidationStatus            string     `json:"validationStatus"`
	ValidationScore             int        `json:"validationScore"`
	ValidationReport            JSONMap    `json:"validationReport" gorm:"type:json"`
	ValidationRunID             string     `json:"validationRunId,omitempty" gorm:"index"`
	ValidationOutcome           string     `json:"validationOutcome,omitempty" gorm:"index"`
	ValidationOverride          bool       `json:"validationOverridePresent" gorm:"not null;default:false"`
	ValidationActionable        bool       `json:"validationHasActionableWarnings" gorm:"not null;default:false"`
	PublishedPath               string     `json:"publishedPath" gorm:"index"`
	PublishedPathKey            string     `json:"-" gorm:"not null;default:'';index:idx_queue_jobs_library_published_path_key,priority:2"`
	PublishedFingerprint        string     `json:"publishedFingerprint" gorm:"index"`
	PublishedSizeBytes          int64      `json:"publishedSizeBytes"`
	PublishedAt                 *time.Time `json:"publishedAt"`
	PublicationRetiredAt        *time.Time `json:"publicationRetiredAt" gorm:"index"`
	ReplacementTargetPath       string     `json:"replacementTargetPath"`
	OriginalArchivedPath        string     `json:"originalArchivedPath" gorm:"index"`
	OriginalArchiveStatus       string     `json:"originalArchiveStatus,omitempty" gorm:"index"`
	PublicationID               string     `json:"publicationId,omitempty" gorm:"index"`
	PublicationState            string     `json:"publicationState,omitempty" gorm:"index"`
	PublicationSummary          string     `json:"publicationSummary,omitempty"`
	StartedAt                   *time.Time `json:"startedAt"`
	FinishedAt                  *time.Time `json:"finishedAt"`
	DismissedAt                 *time.Time `json:"dismissedAt" gorm:"index"`
	CreatedAt                   time.Time  `json:"createdAt"`
	UpdatedAt                   time.Time  `json:"updatedAt"`
}

func (job *QueueJob) BeforeSave(_ *gorm.DB) error {
	job.MediaPathKey = CanonicalPublicationPathKey(job.MediaPath)
	job.PublishedPathKey = CanonicalPublicationPathKey(job.PublishedPath)
	return nil
}
