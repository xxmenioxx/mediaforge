package scheduler

import (
	"github.com/anuelvs/mediaforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"testing"
)

func TestStorageRolesFallbackToLegacyPaths(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:storage-roles?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{"rawRoot": "/host/raw", "stagingPath": "/ssd/work"}}).Error; err != nil {
		t.Fatal(err)
	}
	roles, err := LoadStorageRoles(db)
	if err != nil {
		t.Fatal(err)
	}
	if roles[StorageRoleRaw].Path != "/host/raw" || roles[StorageRoleWork].Path != "/ssd/work" {
		t.Fatalf("unexpected roles: %#v", roles)
	}
}
