package models

import "time"

// AssetMaintenanceOperation records an in-place, non-conversion mutation of a
// Library asset. It is deliberately separate from QueueJob so publication and
// conversion provenance remain distinct.
type AssetMaintenanceOperation struct {
	ID                  string     `json:"id" gorm:"primaryKey"`
	OperationType       string     `json:"operationType" gorm:"not null;index"`
	AssetRecordID       uint       `json:"assetRecordId" gorm:"index"`
	AssetPath           string     `json:"assetPath" gorm:"not null;index"`
	AssetStatus         string     `json:"assetStatus"`
	RequestedIndexes    JSONList   `json:"requestedIndexes" gorm:"type:json"`
	OriginalFingerprint string     `json:"originalFingerprint"`
	ResultFingerprint   string     `json:"resultFingerprint"`
	Status              string     `json:"status" gorm:"not null;index"`
	Phase               string     `json:"phase" gorm:"not null;index"`
	Progress            int        `json:"progress"`
	TemporaryPath       string     `json:"temporaryPath"`
	BackupPath          string     `json:"backupPath"`
	ErrorMessage        string     `json:"errorMessage"`
	Warning             string     `json:"warning"`
	StartedAt           *time.Time `json:"startedAt"`
	FinishedAt          *time.Time `json:"finishedAt"`
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
}
