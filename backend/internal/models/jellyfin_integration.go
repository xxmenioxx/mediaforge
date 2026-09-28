package models

import "time"

// JellyfinIntegrationOutcome is the bounded delivery ledger for the optional
// Jellyfin side effect. Source and target identities stay backend-owned; API
// handlers return a deliberate projection instead of serializing this record.
type JellyfinIntegrationOutcome struct {
	ID               uint       `json:"-" gorm:"primaryKey"`
	SourceKey        string     `json:"-" gorm:"not null;uniqueIndex"`
	Reason           string     `json:"-" gorm:"not null;index"`
	PublicationID    string     `json:"-" gorm:"index"`
	TestEncodeID     uint       `json:"-" gorm:"index"`
	LibraryID        uint       `json:"-" gorm:"index"`
	MappedLibraryID  string     `json:"-"`
	EffectiveTarget  string     `json:"-" gorm:"not null;index:idx_jellyfin_target_due,priority:1"`
	ConfigRevision   int64      `json:"-" gorm:"not null;index"`
	Status           string     `json:"-" gorm:"not null;index:idx_jellyfin_target_due,priority:2"`
	EffectiveScope   string     `json:"-"`
	FailureClass     string     `json:"-"`
	AttemptCount     int        `json:"-" gorm:"not null;default:0"`
	DurationMillis   int64      `json:"-" gorm:"not null;default:0"`
	FirstTriggeredAt time.Time  `json:"-" gorm:"not null;index"`
	DueAt            time.Time  `json:"-" gorm:"not null;index:idx_jellyfin_target_due,priority:3"`
	LastAttemptAt    *time.Time `json:"-"`
	NextAttemptAt    *time.Time `json:"-" gorm:"index"`
	CreatedAt        time.Time  `json:"-"`
	UpdatedAt        time.Time  `json:"-"`
}
