package models

import "time"

// QueueBatchPreflight stores editable batch intent and its latest resolved
// review revision without creating claimable QueueJobs.
type QueueBatchPreflight struct {
	ID        string    `json:"id" gorm:"primaryKey;size:64"`
	Revision  uint      `json:"revision" gorm:"not null;default:1"`
	Resolved  bool      `json:"resolved" gorm:"not null;default:false;index"`
	State     JSONMap   `json:"state" gorm:"type:json"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
