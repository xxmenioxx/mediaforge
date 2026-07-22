package scheduler

import (
	"fmt"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
)

type StorageRole string

const (
	StorageRoleRaw              StorageRole = "raw"
	StorageRoleLibrary          StorageRole = "library"
	StorageRoleOriginalsArchive StorageRole = "originals_archive"
	StorageRoleWork             StorageRole = "work"
	StorageRoleCache            StorageRole = "cache"
	StorageRoleReports          StorageRole = "reports"
	StorageRoleLogs             StorageRole = "logs"
)

type StorageRoleConfig struct {
	Path string `json:"path"`
}
type StorageRoles map[StorageRole]StorageRoleConfig

func LoadStorageRoles(db *gorm.DB) (StorageRoles, error) {
	roles := StorageRoles{}
	var setting models.AppSetting
	result := db.Where("key = ?", "storageRoles").Limit(1).Find(&setting)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected > 0 {
		for _, role := range allStorageRoles() {
			if value, ok := setting.Value[string(role)].(map[string]any); ok {
				if path, ok := value["path"].(string); ok && strings.TrimSpace(path) != "" {
					roles[role] = StorageRoleConfig{Path: strings.TrimSpace(path)}
				}
			}
		}
	}
	var paths models.AppSetting
	legacy := models.JSONMap{}
	if db.Where("key = ?", "paths").Limit(1).Find(&paths).RowsAffected > 0 {
		legacy = paths.Value
	}
	fallbacks := map[StorageRole]string{
		StorageRoleRaw: legacyString(legacy, "rawRoot", "/media/raw"), StorageRoleLibrary: legacyString(legacy, "libraryRoot", "/media/library"),
		StorageRoleOriginalsArchive: legacyString(legacy, "originalsArchivePath", "/media/originals_archive"), StorageRoleWork: legacyString(legacy, "stagingPath", "/media/staging"),
		StorageRoleCache: "/mvforge/cache", StorageRoleReports: legacyString(legacy, "resultsReportsPath", "/media/reports"), StorageRoleLogs: legacyString(legacy, "logsPath", "/media/reports/logs"),
	}
	for role, path := range fallbacks {
		if roles[role].Path == "" {
			roles[role] = StorageRoleConfig{Path: path}
		}
	}
	return roles, nil
}

func (roles StorageRoles) Path(role StorageRole) (string, error) {
	value := strings.TrimSpace(roles[role].Path)
	if value == "" {
		return "", fmt.Errorf("storage role %s has no path", role)
	}
	return value, nil
}

func allStorageRoles() []StorageRole {
	return []StorageRole{StorageRoleRaw, StorageRoleLibrary, StorageRoleOriginalsArchive, StorageRoleWork, StorageRoleCache, StorageRoleReports, StorageRoleLogs}
}
func legacyString(values models.JSONMap, key, fallback string) string {
	if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}
