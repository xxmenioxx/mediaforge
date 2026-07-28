package models

import "time"

// DirectPublication records an original that was approved and moved to a
// Library without an FFmpeg conversion.
type DirectPublication struct {
	ID                   uint       `json:"id" gorm:"primaryKey"`
	SourcePath           string     `json:"sourcePath" gorm:"not null;index"`
	PublishedPath        string     `json:"publishedPath" gorm:"not null;uniqueIndex"`
	LibraryID            uint       `json:"libraryId" gorm:"not null;index"`
	PublishedFingerprint string     `json:"publishedFingerprint"`
	PublishedSizeBytes   int64      `json:"publishedSizeBytes"`
	PublishedAt          time.Time  `json:"publishedAt"`
	ReturnedAt           *time.Time `json:"returnedAt" gorm:"index"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
}
