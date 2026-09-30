package models

import "time"

const (
	TestEncodeArtifactMediaOutput    = "media_output"
	TestEncodeArtifactTemporaryMedia = "temporary_media"
	TestEncodeArtifactSubtitle       = "subtitle_sidecar"
	TestEncodeArtifactAuxiliary      = "auxiliary"
)

// TestEncodeArtifact extends the canonical TestEncode record with durable,
// exact-path ownership evidence for each generated file.
type TestEncodeArtifact struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	TestEncodeID uint      `json:"testEncodeId" gorm:"not null;uniqueIndex:idx_test_encode_artifact_owner_path;index"`
	LibraryID    uint      `json:"libraryId" gorm:"not null;index"`
	Path         string    `json:"path" gorm:"type:text;not null;uniqueIndex:idx_test_encode_artifact_owner_path"`
	Type         string    `json:"type" gorm:"not null;index"`
	Expected     bool      `json:"expected" gorm:"not null"`
	State        string    `json:"state" gorm:"not null;default:owned;index"`
	SizeBytes    int64     `json:"sizeBytes" gorm:"not null;default:0"`
	CleanupError string    `json:"cleanupError,omitempty" gorm:"type:text"`
	CreatedAt    time.Time `json:"createdAt" gorm:"not null"`
	UpdatedAt    time.Time `json:"updatedAt" gorm:"not null"`
}
