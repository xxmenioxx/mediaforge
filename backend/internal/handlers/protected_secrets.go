package handlers

import (
	"errors"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CredentialRef string

const jellyfinAPITokenCredentialRef CredentialRef = "jellyfin.apiToken"

// ProtectedSecretSource keeps credentials outside ordinary AppSetting values.
// Callers pass the active transaction so secret and configuration writes commit together.
type ProtectedSecretSource interface {
	Get(db *gorm.DB, ref CredentialRef) (string, bool, error)
	Put(db *gorm.DB, ref CredentialRef, value string) error
	Delete(db *gorm.DB, ref CredentialRef) error
}

type gormProtectedSecretSource struct{}

func (gormProtectedSecretSource) Get(db *gorm.DB, ref CredentialRef) (string, bool, error) {
	var credential models.ProtectedCredential
	err := db.First(&credential, "credential_ref = ?", string(ref)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return credential.SecretValue, true, nil
}

func (gormProtectedSecretSource) Put(db *gorm.DB, ref CredentialRef, value string) error {
	credential := models.ProtectedCredential{CredentialRef: string(ref), SecretValue: value}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "credential_ref"}},
		DoUpdates: clause.AssignmentColumns([]string{"secret_value", "updated_at"}),
	}).Create(&credential).Error
}

func (gormProtectedSecretSource) Delete(db *gorm.DB, ref CredentialRef) error {
	return db.Delete(&models.ProtectedCredential{}, "credential_ref = ?", string(ref)).Error
}

func migrateLegacyJellyfinCredential(db *gorm.DB, secrets ProtectedSecretSource) error {
	var stored models.AppSetting
	err := db.First(&stored, "key = ?", jellyfinSettingsKey).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	_, hasLegacyToken := stored.Value["apiToken"]
	_, hasCredentialRef := stored.Value["credentialRef"]
	if !hasLegacyToken && hasCredentialRef {
		return nil
	}
	settings, err := jellyfinSettingsFromValue(stored.Value, false)
	if err != nil {
		return err
	}
	secret, found, err := secrets.Get(db, jellyfinAPITokenCredentialRef)
	if err != nil {
		return err
	}
	if found {
		settings.APIToken = secret
	} else if settings.APIToken != "" {
		if err := secrets.Put(db, jellyfinAPITokenCredentialRef, settings.APIToken); err != nil {
			return err
		}
	}
	settings.CredentialRef = jellyfinAPITokenCredentialRef
	return db.Model(&models.AppSetting{}).Where("key = ?", jellyfinSettingsKey).
		UpdateColumn("value", jellyfinSettingsStoredValue(settings)).Error
}
