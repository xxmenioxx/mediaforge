package models

import "time"

type ExecutionPlan struct {
	ID                      uint       `json:"id" gorm:"primaryKey"`
	JobID                   uint       `json:"jobId" gorm:"not null;index;uniqueIndex:idx_execution_plan_job_version"`
	Version                 int        `json:"version" gorm:"not null;uniqueIndex:idx_execution_plan_job_version"`
	Status                  string     `json:"status" gorm:"not null;index"`
	ProfileVersion          int        `json:"profileVersion" gorm:"not null"`
	Constraints             JSONMap    `json:"constraints" gorm:"type:json"`
	CodecFamily             string     `json:"codecFamily"`
	SelectedEncoder         string     `json:"selectedEncoder"`
	BitDepth                int        `json:"bitDepth"`
	PixelFormat             string     `json:"pixelFormat"`
	QualityMode             string     `json:"qualityMode"`
	QualityValue            int        `json:"qualityValue"`
	RuntimeProfile          string     `json:"runtimeProfile"`
	RuntimeSnapshotID       *uint      `json:"runtimeSnapshotId" gorm:"index"`
	WorkspaceMode           string     `json:"workspaceMode"`
	WaitingState            string     `json:"waitingState"`
	DecisionReasons         JSONList   `json:"decisionReasons" gorm:"type:json"`
	DecisionSources         JSONMap    `json:"decisionSources" gorm:"type:json"`
	Warnings                JSONList   `json:"warnings" gorm:"type:json"`
	Reservation             JSONMap    `json:"reservation" gorm:"type:json"`
	Evaluation              JSONMap    `json:"evaluation" gorm:"type:json"`
	OutputPath              string     `json:"outputPath"`
	InputSizeBytes          int64      `json:"inputSizeBytes"`
	EstimatedOutputMinBytes int64      `json:"estimatedOutputMinBytes"`
	EstimatedOutputMaxBytes int64      `json:"estimatedOutputMaxBytes"`
	EstimatedWorkspaceBytes int64      `json:"estimatedWorkspaceBytes"`
	EstimateConfidence      string     `json:"estimateConfidence"`
	ApprovalStatus          string     `json:"approvalStatus"`
	ApprovalMode            string     `json:"approvalMode"`
	ApprovedAt              *time.Time `json:"approvedAt"`
	RejectedAt              *time.Time `json:"rejectedAt"`
	SupersededAt            *time.Time `json:"supersededAt"`
	CreatedAt               time.Time  `json:"createdAt"`
	UpdatedAt               time.Time  `json:"updatedAt"`
}
