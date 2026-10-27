package handlers

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

// TestAssetRecordPathIdentityDoesNotResolveFilesystemAliases is synthetic
// evidence about today's path-to-record behavior. It demonstrates why an
// indexed ID cannot be inferred from a cleaned path for aliases or unindexed
// files. There is no production lock-identity resolver or exclusive fallback
// to exercise yet; this test intentionally does not claim either exists.
func TestAssetRecordPathIdentityDoesNotResolveFilesystemAliases(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "asset-identity.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.AutoMigrate(&models.AssetRecord{}); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	canonicalPath := filepath.Join(root, "canonical", "movie.mkv")
	aliasPath := filepath.Join(root, "alias", "movie.mkv")
	if err := os.MkdirAll(filepath.Dir(canonicalPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(aliasPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(canonicalPath, []byte("synthetic media identity fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(canonicalPath, aliasPath); err != nil {
		t.Fatal(err)
	}
	canonicalInfo, err := os.Stat(canonicalPath)
	if err != nil {
		t.Fatal(err)
	}
	aliasInfo, err := os.Stat(aliasPath)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(canonicalInfo, aliasInfo) {
		t.Fatal("fixture paths must refer to the same filesystem object")
	}

	handler := AssetHandler{db: db}
	resolvedCanonical, err := handler.resolveMediaPath(canonicalPath)
	if err != nil {
		t.Fatal(err)
	}
	resolvedAlias, err := handler.resolveMediaPath(aliasPath)
	if err != nil {
		t.Fatal(err)
	}
	if resolvedCanonical == resolvedAlias {
		t.Fatalf("current lexical path resolver unexpectedly collapsed symlink alias %q to %q", aliasPath, resolvedCanonical)
	}

	canonicalRecord := models.AssetRecord{Path: resolvedCanonical, FileName: filepath.Base(resolvedCanonical), Status: "library"}
	aliasRecord := models.AssetRecord{Path: resolvedAlias, FileName: filepath.Base(resolvedAlias), Status: "library"}
	if err := db.Create(&canonicalRecord).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&aliasRecord).Error; err != nil {
		t.Fatalf("path-only unique index should allow distinct aliases of one file: %v", err)
	}
	if canonicalRecord.ID == 0 || aliasRecord.ID == 0 || canonicalRecord.ID == aliasRecord.ID {
		t.Fatalf("distinct path rows should have distinct IDs, got canonical=%d alias=%d", canonicalRecord.ID, aliasRecord.ID)
	}

	unindexedPath := filepath.Join(root, "unindexed.mkv")
	if err := os.WriteFile(unindexedPath, []byte("synthetic unindexed file"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolvedUnindexed, err := handler.resolveMediaPath(unindexedPath)
	if err != nil {
		t.Fatal(err)
	}
	var unindexed models.AssetRecord
	if err := db.Where("path = ?", resolvedUnindexed).First(&unindexed).Error; err == nil {
		t.Fatalf("expected no AssetRecord identity for unindexed path %q", resolvedUnindexed)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("lookup unindexed AssetRecord: %v", err)
	}
}
