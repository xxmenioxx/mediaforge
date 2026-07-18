package models

import "time"

type WorkerNode struct {
	ID                uint      `json:"id" gorm:"primaryKey"`
	Name              string    `json:"name" gorm:"not null;uniqueIndex"`
	Status            string    `json:"status" gorm:"not null;index"`
	MaxConcurrentJobs int       `json:"maxConcurrentJobs" gorm:"not null;default:1"`
	Encoders          JSONList  `json:"encoders" gorm:"type:json"`
	RuntimeProfile    string    `json:"runtimeProfile"`
	LastSeenAt        time.Time `json:"lastSeenAt" gorm:"index"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}
