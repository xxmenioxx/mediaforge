package models

import "time"

type QueueJob struct {
	ID                    uint       `json:"id" gorm:"primaryKey"`
	BatchID               string     `json:"batchId" gorm:"index"`
	BatchName             string     `json:"batchName"`
	MediaPath             string     `json:"mediaPath" gorm:"not null"`
	PublishMode           string     `json:"publishMode" gorm:"not null;default:standard"`
	LibraryID             uint       `json:"libraryId" gorm:"not null"`
	ProfileID             uint       `json:"profileId" gorm:"not null"`
	ProfileVersion        int        `json:"profileVersion" gorm:"not null;default:1"`
	ProfileSnapshot       JSONMap    `json:"profileSnapshot" gorm:"type:json"`
	ProfileCapturedAt     *time.Time `json:"profileCapturedAt"`
	ActiveExecutionPlanID *uint      `json:"activeExecutionPlanId" gorm:"index"`
	AudioProfileKey       string     `json:"audioProfileKey"`
	Priority              int        `json:"priority" gorm:"not null;default:5"`
	Status                string     `json:"status" gorm:"not null;default:queued"`
	Stage                 string     `json:"stage" gorm:"not null;default:queued;index"`
	StageUpdatedAt        *time.Time `json:"stageUpdatedAt"`
	StageHistory          JSONList   `json:"stageHistory" gorm:"type:json"`
	Progress              int        `json:"progress" gorm:"not null;default:0"`
	WorkerName            string     `json:"workerName"`
	OutputPath            string     `json:"outputPath"`
	ErrorMessage          string     `json:"errorMessage"`
	Notes                 string     `json:"notes"`
	ValidationStatus      string     `json:"validationStatus"`
	ValidationScore       int        `json:"validationScore"`
	ValidationReport      JSONMap    `json:"validationReport" gorm:"type:json"`
	PublishedPath         string     `json:"publishedPath"`
	PublishedAt           *time.Time `json:"publishedAt"`
	PublicationRetiredAt  *time.Time `json:"publicationRetiredAt" gorm:"index"`
	ReplacementTargetPath string     `json:"replacementTargetPath"`
	OriginalArchivedPath  string     `json:"originalArchivedPath"`
	StartedAt             *time.Time `json:"startedAt"`
	FinishedAt            *time.Time `json:"finishedAt"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
}
