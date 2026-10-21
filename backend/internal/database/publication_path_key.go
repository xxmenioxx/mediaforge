package database

import (
	"fmt"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
)

func backfillPublicationPathKeys(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		for _, column := range [][3]string{
			{"queue_jobs", "media_path", "media_path_key"},
			{"queue_jobs", "published_path", "published_path_key"},
			{"direct_publications", "published_path", "published_path_key"},
		} {
			if err := backfillPublicationPathKeyColumn(tx, column[0], column[1], column[2]); err != nil {
				return err
			}
		}
		return nil
	})
}

func backfillPublicationPathKeyColumn(db *gorm.DB, table, pathColumn, keyColumn string) error {
	var lastID uint
	for {
		var rows []struct {
			ID            uint
			PublishedPath string
		}
		if err := db.Table(table).
			Select("id, "+pathColumn+" AS published_path").
			Where("id > ? AND "+pathColumn+" <> '' AND "+keyColumn+" = ''", lastID).
			Order("id ASC").
			Limit(256).
			Scan(&rows).Error; err != nil {
			return fmt.Errorf("read legacy publication paths from %s: %w", table, err)
		}
		if len(rows) == 0 {
			return nil
		}
		for _, row := range rows {
			key := models.CanonicalPublicationPathKey(row.PublishedPath)
			if key != "" {
				if err := db.Table(table).Where("id = ?", row.ID).UpdateColumn(keyColumn, key).Error; err != nil {
					return fmt.Errorf("backfill publication path identity in %s row %d: %w", table, row.ID, err)
				}
			}
			lastID = row.ID
		}
	}
}
