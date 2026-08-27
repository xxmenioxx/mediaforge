package models

import "time"

// SourceGroup is a configurable first-level directory below the Raw source
// root. It organizes input media and is deliberately independent of Library,
// which remains a publication destination.
type SourceGroup struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	Name         string    `json:"name" gorm:"not null"`
	RelativePath string    `json:"relativePath" gorm:"not null;uniqueIndex"`
	SourcePath   string    `json:"sourcePath" gorm:"not null;uniqueIndex"`
	Enabled      bool      `json:"enabled" gorm:"not null;default:true"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// AssetScopeConfiguration stores non-profile inheritance decisions. Missing
// rows mean inherit. Explicit selection values remain distinct from inherit.
type AssetScopeConfiguration struct {
	ID                   uint      `json:"id" gorm:"primaryKey"`
	ScopeType            string    `json:"scopeType" gorm:"not null;uniqueIndex:idx_asset_scope_configuration"`
	ScopeKey             string    `json:"scopeKey" gorm:"not null;uniqueIndex:idx_asset_scope_configuration"`
	CategorySelection    string    `json:"categorySelection" gorm:"not null;default:inherit"`
	Category             string    `json:"category"`
	DestinationSelection string    `json:"destinationSelection" gorm:"not null;default:inherit"`
	DestinationLibraryID uint      `json:"destinationLibraryId,omitempty" gorm:"index"`
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
}
