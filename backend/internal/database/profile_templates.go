package database

import (
	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/profiletemplates"

	"gorm.io/gorm"
)

func migrateSeededProfileProvenance(db *gorm.DB) error {
	var profiles []models.Profile
	if err := db.Unscoped().Find(&profiles).Error; err != nil {
		return err
	}
	for _, profile := range profiles {
		if profile.TemplateKey != "" || profile.TemplateVersion != 0 {
			continue
		}
		template, ok := profiletemplates.LegacySeedTemplate(profile)
		if !ok {
			continue
		}
		if err := db.Unscoped().Model(&models.Profile{}).Where("id = ?", profile.ID).UpdateColumns(map[string]any{
			"template_key":     template.Key,
			"template_version": template.Version,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}
