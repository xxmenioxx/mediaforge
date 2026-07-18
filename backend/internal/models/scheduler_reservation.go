package models

import "time"

type SchedulerReservation struct {
	ID             uint       `json:"id" gorm:"primaryKey"`
	JobID          uint       `json:"jobId" gorm:"not null;uniqueIndex"`
	AssetKey       string     `json:"assetKey" gorm:"not null;uniqueIndex"`
	State          string     `json:"state" gorm:"not null;index"`
	WorkerName     string     `json:"workerName"`
	Encoder        string     `json:"encoder"`
	EncoderClass   string     `json:"encoderClass"`
	MemoryBytes    int64      `json:"memoryBytes"`
	WorkspaceBytes int64      `json:"workspaceBytes"`
	LibraryBytes   int64      `json:"libraryBytes"`
	AcquiredAt     *time.Time `json:"acquiredAt"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}
