package models

import "time"

type AssetRecord struct {
	ID               uint       `json:"id" gorm:"primaryKey"`
	Path             string     `json:"path" gorm:"not null;uniqueIndex"`
	RootPath         string     `json:"rootPath"`
	RelativePath     string     `json:"relativePath" gorm:"index"`
	GroupPath        string     `json:"groupPath" gorm:"index"`
	SourceGroupID    uint       `json:"sourceGroupId,omitempty" gorm:"index"`
	LogicalGroupPath string     `json:"logicalGroupPath" gorm:"index"`
	SourcePath       string     `json:"sourcePath" gorm:"index"`
	FileName         string     `json:"fileName" gorm:"not null"`
	Extension        string     `json:"extension"`
	SizeBytes        int64      `json:"sizeBytes"`
	ModifiedAt       time.Time  `json:"modifiedAt"`
	Status           string     `json:"status" gorm:"index"`
	LibraryID        uint       `json:"libraryId" gorm:"index"`
	LibraryName      string     `json:"libraryName"`
	Missing          bool       `json:"missing" gorm:"index"`
	ExpiresAt        *time.Time `json:"expiresAt"`
	SyncedAt         time.Time  `json:"syncedAt"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}
