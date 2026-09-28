package models

import "time"

type AppSetting struct {
	Key       string    `json:"key" gorm:"primaryKey"`
	Value     JSONMap   `json:"value" gorm:"type:json"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ProtectedCredential stores secret material separately from ordinary settings.
// Its fields are deliberately omitted from JSON serialization.
type ProtectedCredential struct {
	CredentialRef string    `json:"-" gorm:"primaryKey"`
	SecretValue   string    `json:"-" gorm:"not null"`
	UpdatedAt     time.Time `json:"-"`
}
