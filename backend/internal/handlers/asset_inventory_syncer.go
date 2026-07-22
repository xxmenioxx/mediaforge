package handlers

import (
	"log"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
)

func StartAssetInventorySyncer(db *gorm.DB) {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if !assetInventoryAutoSyncEnabled(db) {
				continue
			}
			lastSync, interval := assetInventorySyncSchedule(db)
			if !lastSync.IsZero() && time.Since(lastSync) < interval {
				continue
			}
			if _, err := (AssetHandler{db: db}).syncAssetInventory(); err != nil {
				log.Printf("asset inventory sync error: %v", err)
			}
		}
	}()
}

func assetInventoryAutoSyncEnabled(db *gorm.DB) bool {
	setting := models.AppSetting{}
	if err := db.First(&setting, "key = ?", "assetInventory").Error; err != nil || setting.Value == nil {
		return true
	}
	return boolSetting(setting.Value["autoSyncEnabled"], true)
}

func assetInventorySyncSchedule(db *gorm.DB) (time.Time, time.Duration) {
	setting := models.AppSetting{}
	minutes := 60
	if err := db.First(&setting, "key = ?", "assetInventory").Error; err == nil && setting.Value != nil {
		minutes = intValueSetting(setting.Value["syncIntervalMinutes"], 60)
	}
	if minutes < 5 {
		minutes = 5
	}

	var last models.AssetRecord
	if err := db.Order("synced_at desc").First(&last).Error; err == nil {
		return last.SyncedAt, time.Duration(minutes) * time.Minute
	}
	return time.Time{}, time.Duration(minutes) * time.Minute
}
