package models

import (
	"time"

	"gorm.io/gorm"
)

type Profile struct {
	ID                 uint           `json:"id" gorm:"primaryKey"`
	Name               string         `json:"name" gorm:"not null;uniqueIndex"`
	Scope              string         `json:"scope" gorm:"not null;default:'';index"`
	Description        string         `json:"description"`
	Container          string         `json:"container" gorm:"not null"`
	VideoCodec         string         `json:"videoCodec" gorm:"not null"`
	CodecFamily        string         `json:"codecFamily" gorm:"not null;default:''"`
	EncoderPolicy      string         `json:"encoderPolicy" gorm:"not null;default:''"`
	PreferredEncoder   string         `json:"preferredEncoder" gorm:"not null;default:''"`
	AllowedEncoders    StringList     `json:"allowedEncoders" gorm:"type:json"`
	FallbackPolicy     string         `json:"fallbackPolicy" gorm:"not null;default:''"`
	BitDepth           int            `json:"bitDepth" gorm:"not null;default:0"`
	PixelFormat        string         `json:"pixelFormat" gorm:"not null;default:''"`
	QualityStrategy    string         `json:"qualityStrategy" gorm:"not null;default:''"`
	OptimizationIntent string         `json:"optimizationIntent" gorm:"not null;default:'';index"`
	ProfileVersion     int            `json:"profileVersion" gorm:"not null;default:1"`
	AudioCodec         string         `json:"audioCodec" gorm:"not null"`
	QualityMode        string         `json:"qualityMode" gorm:"not null"`
	QualityValue       int            `json:"qualityValue" gorm:"not null"`
	PreserveHDR        bool           `json:"preserveHdr"`
	PreserveSubtitles  bool           `json:"preserveSubtitles"`
	PreserveChapters   bool           `json:"preserveChapters"`
	WorkerConfig       JSONMap        `json:"workerConfig" gorm:"type:json"`
	Disabled           bool           `json:"disabled"`
	CreatedAt          time.Time      `json:"createdAt"`
	UpdatedAt          time.Time      `json:"updatedAt"`
	DeletedAt          gorm.DeletedAt `json:"deletedAt" gorm:"index"`
}
