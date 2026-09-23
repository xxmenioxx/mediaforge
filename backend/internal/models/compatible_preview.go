package models

import "time"

// CompatiblePreviewRequestRecord stores the bounded, restart-recovery portion
// of a persistent Compatible Preview request. Runtime ownership remains in the
// handler registries; this row is not a replacement request engine.
type CompatiblePreviewRequestRecord struct {
	RequestID               string     `gorm:"primaryKey;size:64"`
	State                   string     `gorm:"not null;index;size:32"`
	NormalizedRequestJSON   string     `gorm:"type:text;not null"`
	PreparedSnapshotVersion int        `gorm:"not null;default:0;index"`
	PreparedSnapshotJSON    string     `gorm:"type:text"`
	ArtifactID              string     `gorm:"index;size:64"`
	CreatedAt               time.Time  `gorm:"index"`
	UpdatedAt               time.Time  `gorm:"index"`
	AccessedAt              time.Time  `gorm:"index"`
	ExpiresAt               time.Time  `gorm:"index"`
	TombstoneExpiresAt      *time.Time `gorm:"index"`
	RecoveryReason          string     `gorm:"type:text"`
}

// CompatiblePreviewArtifactRecord stores durable artifact metadata and the
// ownership token for the current generation attempt. It deliberately omits
// contexts, waiters, timers, and other process-local runtime state.
type CompatiblePreviewArtifactRecord struct {
	ArtifactID               string    `gorm:"primaryKey;size:64"`
	CanonicalEncodeSignature string    `gorm:"index;size:64"`
	CachePath                string    `gorm:"type:text;not null"`
	SourcePath               string    `gorm:"type:text;not null"`
	SourceSize               int64     `gorm:"not null;default:0"`
	SourceModifiedAtNS       int64     `gorm:"not null;default:0"`
	SourceMode               uint32    `gorm:"not null;default:0"`
	RetentionPolicy          string    `gorm:"index;size:32"`
	GenerationToken          string    `gorm:"index;size:64"`
	GenerationVersion        int64     `gorm:"not null;default:0"`
	State                    string    `gorm:"not null;index;size:32"`
	GenerationStartedAt      time.Time `gorm:"index"`
	GenerationFinishedAt     time.Time `gorm:"index"`
	CreatedAt                time.Time `gorm:"index"`
	UpdatedAt                time.Time `gorm:"index"`
	LastUsedAt               time.Time `gorm:"index"`
	ExpiresAt                time.Time `gorm:"index"`
	RecoveryReason           string    `gorm:"type:text"`
}
