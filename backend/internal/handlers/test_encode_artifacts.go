package handlers

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func testEncodeArtifactSet(test models.TestEncode, outputPath, temporaryPath string, sidecars []SubtitleArtifact, activated bool) []models.TestEncodeArtifact {
	createdAt := test.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	artifacts := []models.TestEncodeArtifact{}
	byPath := map[string]int{}
	add := func(path, kind string, expected bool, sizeBytes int64) {
		path = filepath.Clean(strings.TrimSpace(path))
		if path == "." || path == "" {
			return
		}
		if index, exists := byPath[path]; exists {
			artifacts[index].Expected = artifacts[index].Expected || expected
			if sizeBytes > 0 {
				artifacts[index].SizeBytes = sizeBytes
			}
			return
		}
		byPath[path] = len(artifacts)
		artifacts = append(artifacts, models.TestEncodeArtifact{
			TestEncodeID: test.ID, LibraryID: test.LibraryID, Path: path, Type: kind,
			Expected: expected, State: "owned", SizeBytes: max(int64(0), sizeBytes), CreatedAt: createdAt, UpdatedAt: time.Now().UTC(),
		})
	}
	add(outputPath, models.TestEncodeArtifactMediaOutput, true, 0)
	if !activated {
		add(temporaryPath, models.TestEncodeArtifactTemporaryMedia, false, 0)
	}
	for _, sidecar := range sidecars {
		path := strings.TrimSpace(sidecar.StagedPath)
		if path == "" {
			continue
		}
		if activated {
			add(path, models.TestEncodeArtifactSubtitle, true, 0)
			continue
		}
		add(path, models.TestEncodeArtifactSubtitle, false, 0)
		if finalPath, ok := plannedTestSubtitleArtifactPath(path, temporaryPath, outputPath); ok {
			add(finalPath, models.TestEncodeArtifactSubtitle, true, 0)
		}
	}
	return artifacts
}

func plannedTestSubtitleArtifactPath(stagedPath, temporaryMediaPath, outputMediaPath string) (string, bool) {
	temporaryBase := strings.TrimSuffix(temporaryMediaPath, filepath.Ext(temporaryMediaPath))
	if temporaryBase == "" || !strings.HasPrefix(stagedPath, temporaryBase+".") {
		return "", false
	}
	outputBase := strings.TrimSuffix(outputMediaPath, filepath.Ext(outputMediaPath))
	return outputBase + strings.TrimPrefix(stagedPath, temporaryBase), true
}

func persistTestEncodeArtifactSet(db *gorm.DB, testEncodeID uint, artifacts []models.TestEncodeArtifact) error {
	if db == nil {
		return fmt.Errorf("Test Encode database is unavailable")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		return writeTestEncodeArtifactSet(tx, testEncodeID, artifacts)
	})
}

func writeTestEncodeArtifactSet(tx *gorm.DB, testEncodeID uint, artifacts []models.TestEncodeArtifact) error {
	paths := make([]string, 0, len(artifacts))
	seen := map[string]struct{}{}
	for index := range artifacts {
		artifact := &artifacts[index]
		artifact.TestEncodeID = testEncodeID
		artifact.Path = filepath.Clean(strings.TrimSpace(artifact.Path))
		if artifact.Path == "." || artifact.Path == "" {
			continue
		}
		if artifact.CreatedAt.IsZero() {
			artifact.CreatedAt = time.Now().UTC()
		}
		artifact.UpdatedAt = time.Now().UTC()
		if _, exists := seen[artifact.Path]; exists {
			continue
		}
		seen[artifact.Path] = struct{}{}
		paths = append(paths, artifact.Path)
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "test_encode_id"}, {Name: "path"}},
			DoUpdates: clause.AssignmentColumns([]string{"library_id", "type", "expected", "state", "cleanup_error", "size_bytes", "updated_at"}),
		}).Create(artifact).Error; err != nil {
			return fmt.Errorf("persist Test Encode artifact %q: %w", artifact.Path, err)
		}
	}
	query := tx.Where("test_encode_id = ?", testEncodeID)
	if len(paths) > 0 {
		query = query.Where("path NOT IN ?", paths)
	}
	if err := query.Delete(&models.TestEncodeArtifact{}).Error; err != nil {
		return fmt.Errorf("remove stale Test Encode artifact ownership: %w", err)
	}
	return nil
}
