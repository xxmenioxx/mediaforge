package models

import "time"

type Library struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	Name             string    `json:"name" gorm:"not null"`
	SourcePath       string    `json:"sourcePath" gorm:"not null"`
	DestinationPath  string    `json:"destinationPath" gorm:"not null"`
	Type             string    `json:"type" gorm:"not null"`
	ValidationRules  JSONMap   `json:"validationRules" gorm:"type:json"`
	DefaultProfileID *uint     `json:"defaultProfileId"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}
