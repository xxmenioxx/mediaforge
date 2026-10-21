package models

import (
	"time"

	"gorm.io/gorm"
)

// DirectPublication records an original that was approved and moved to a
// Library without an FFmpeg conversion.
type DirectPublication struct {
	ID                   uint       `json:"id" gorm:"primaryKey"`
	SourcePath           string     `json:"sourcePath" gorm:"not null;index"`
	SourceAssetRecordID  *uint      `json:"sourceAssetRecordId,omitempty" gorm:"index"`
	PublishedPath        string     `json:"publishedPath" gorm:"not null;uniqueIndex"`
	LibraryID            uint       `json:"libraryId" gorm:"not null;index;index:idx_direct_publications_library_published_path_key,priority:1"`
	PublishedPathKey     string     `json:"-" gorm:"not null;default:'';index:idx_direct_publications_library_published_path_key,priority:2"`
	PublishedFingerprint string     `json:"publishedFingerprint"`
	PublishedSizeBytes   int64      `json:"publishedSizeBytes"`
	PublishedAt          time.Time  `json:"publishedAt"`
	ReturnedAt           *time.Time `json:"returnedAt" gorm:"index"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
}

func (publication *DirectPublication) BeforeSave(_ *gorm.DB) error {
	publication.PublishedPathKey = CanonicalPublicationPathKey(publication.PublishedPath)
	return nil
}
