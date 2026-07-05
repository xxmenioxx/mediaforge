package models

import "time"

type AppSetting struct {
	Key       string    `json:"key" gorm:"primaryKey"`
	Value     JSONMap   `json:"value" gorm:"type:json"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
