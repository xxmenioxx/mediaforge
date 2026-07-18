package models

import "time"

// RuntimeSnapshot records the host capabilities used by the scheduler when it
// evaluates execution plans. Snapshots are immutable so an old plan remains
// explainable after the machine or its configuration changes.
type RuntimeSnapshot struct {
	ID                   uint      `json:"id" gorm:"primaryKey"`
	DetectedAt           time.Time `json:"detectedAt" gorm:"not null;index"`
	OS                   string    `json:"os"`
	Architecture         string    `json:"architecture"`
	Container            bool      `json:"container"`
	CPUCores             int       `json:"cpuCores"`
	CPULoad1             float64   `json:"cpuLoad1"`
	TotalMemoryBytes     int64     `json:"totalMemoryBytes"`
	AvailableMemoryBytes int64     `json:"availableMemoryBytes"`
	BatteryPresent       bool      `json:"batteryPresent"`
	BatteryPercent       int       `json:"batteryPercent"`
	PowerSource          string    `json:"powerSource"`
	OnBattery            bool      `json:"onBattery"`
	Disks                JSONMap   `json:"disks" gorm:"type:json"`
	Encoders             JSONMap   `json:"encoders" gorm:"type:json"`
	RecommendedProfile   string    `json:"recommendedProfile"`
	SelectedProfile      string    `json:"selectedProfile"`
	SelectionReasons     JSONList  `json:"selectionReasons" gorm:"type:json"`
	Warnings             JSONList  `json:"warnings" gorm:"type:json"`
	CreatedAt            time.Time `json:"createdAt"`
}
