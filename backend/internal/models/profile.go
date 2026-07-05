package models

import (
	"time"

	"gorm.io/gorm"
)

type Profile struct {
	ID                uint           `json:"id" gorm:"primaryKey"`
	Name              string         `json:"name" gorm:"not null;uniqueIndex"`
	Description       string         `json:"description"`
	Container         string         `json:"container" gorm:"not null"`
	VideoCodec        string         `json:"videoCodec" gorm:"not null"`
	AudioCodec        string         `json:"audioCodec" gorm:"not null"`
	QualityMode       string         `json:"qualityMode" gorm:"not null"`
	QualityValue      int            `json:"qualityValue" gorm:"not null"`
	PreserveHDR       bool           `json:"preserveHdr"`
	PreserveSubtitles bool           `json:"preserveSubtitles"`
	PreserveChapters  bool           `json:"preserveChapters"`
	WorkerConfig      JSONMap        `json:"workerConfig" gorm:"type:json"`
	Disabled          bool           `json:"disabled"`
	CreatedAt         time.Time      `json:"createdAt"`
	UpdatedAt         time.Time      `json:"updatedAt"`
	DeletedAt         gorm.DeletedAt `json:"deletedAt" gorm:"index"`
}
