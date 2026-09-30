package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (h AssetHandler) removeTestEncodeFilesWithEvidence(test models.TestEncode) (bool, error) {
	return h.removeTestEncodeFilesForOwner(test, 0)
}

func (h AssetHandler) removeTestEncodeFilesForOwner(test models.TestEncode, activeOwnerID uint) (bool, error) {
	library, artifacts, err := h.testEncodeArtifactCleanupPlan(test, activeOwnerID)
	if err != nil {
		return false, err
	}
	mutated := false
	for _, artifact := range artifacts {
		if err := validateTestEncodeArtifactPath(artifact.Path, library.DestinationPath); err != nil {
			_ = h.setTestEncodeArtifactState(artifact, "cleanup_failed", err.Error())
			return mutated, err
		}
		if info, err := os.Lstat(artifact.Path); err == nil && info.IsDir() {
			_ = h.setTestEncodeArtifactState(artifact, "cleanup_failed", "path is a directory")
			return mutated, fmt.Errorf("refusing to recursively delete a Test Encode directory")
		} else if err != nil && !os.IsNotExist(err) {
			_ = h.setTestEncodeArtifactState(artifact, "cleanup_failed", err.Error())
			return mutated, err
		}
		if err := os.Remove(artifact.Path); err != nil && !os.IsNotExist(err) {
			_ = h.setTestEncodeArtifactState(artifact, "cleanup_failed", err.Error())
			return mutated, err
		} else if err == nil {
			mutated = true
		}
		if err := h.setTestEncodeArtifactState(artifact, "removed", ""); err != nil {
			return mutated, fmt.Errorf("record Test Encode artifact cleanup: %w", err)
		}
	}
	return mutated, nil
}

func (h AssetHandler) testEncodeArtifactCleanupPlan(test models.TestEncode, activeOwnerID uint) (models.Library, []models.TestEncodeArtifact, error) {
	var library models.Library
	if err := h.db.First(&library, test.LibraryID).Error; err != nil {
		return library, nil, fmt.Errorf("load Test Encode Library root: %w", err)
	}
	if testEncodeOperationActive(test.ID) && activeOwnerID != test.ID {
		return library, nil, fmt.Errorf("refusing to delete artifacts referenced by an active Test Encode operation")
	}
	artifacts, err := h.testEncodeCleanupCandidates(test)
	if err != nil {
		return library, nil, err
	}
	for _, artifact := range artifacts {
		if artifact.TestEncodeID != test.ID || (artifact.LibraryID != 0 && artifact.LibraryID != test.LibraryID) {
			return library, nil, fmt.Errorf("refusing to delete Test Encode artifact with mismatched ownership")
		}
		if err := validateTestEncodeArtifactPath(artifact.Path, library.DestinationPath); err != nil {
			return library, nil, err
		}
		if info, statErr := os.Lstat(artifact.Path); statErr == nil && info.IsDir() {
			return library, nil, fmt.Errorf("refusing to recursively delete a Test Encode directory")
		} else if statErr != nil && !os.IsNotExist(statErr) {
			return library, nil, statErr
		}
		if testEncodePathsReferToSameFile(artifact.Path, test.SourcePath) {
			return library, nil, fmt.Errorf("refusing to delete Test Encode source media")
		}
		if h.db.Migrator().HasTable(&models.AssetRecord{}) {
			var asset models.AssetRecord
			assetErr := h.db.Where("path = ?", filepath.Clean(artifact.Path)).First(&asset).Error
			if assetErr == nil {
				return library, nil, fmt.Errorf("refusing to delete a current Library asset")
			} else if assetErr != gorm.ErrRecordNotFound {
				return library, nil, fmt.Errorf("check current Library asset identity: %w", assetErr)
			}
		}
		if referenced, referenceErr := h.testEncodeArtifactHasActiveReference(test.ID, artifact.Path); referenceErr != nil {
			return library, nil, referenceErr
		} else if referenced {
			return library, nil, fmt.Errorf("refusing to delete an artifact referenced by another active operation")
		}
	}
	return library, artifacts, nil
}

func (h AssetHandler) testEncodeOwnedArtifactDiskUsage(test models.TestEncode) (int64, error) {
	library, artifacts, err := h.testEncodeArtifactCleanupPlan(test, 0)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, artifact := range artifacts {
		if err := validateTestEncodeArtifactPath(artifact.Path, library.DestinationPath); err != nil {
			return total, err
		}
		info, err := os.Stat(artifact.Path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return total, err
		}
		if !info.IsDir() {
			total += info.Size()
		}
	}
	return total, nil
}

func (h AssetHandler) setTestEncodeArtifactState(artifact models.TestEncodeArtifact, state, cleanupError string) error {
	if !h.db.Migrator().HasTable(&models.TestEncodeArtifact{}) {
		return nil
	}
	now := time.Now().UTC()
	updates := map[string]any{"state": state, "cleanup_error": cleanupError, "updated_at": now}
	if artifact.ID != 0 {
		return h.db.Model(&models.TestEncodeArtifact{}).Where("id = ?", artifact.ID).Updates(updates).Error
	}
	artifact.State = state
	artifact.CleanupError = cleanupError
	artifact.UpdatedAt = now
	if artifact.CreatedAt.IsZero() {
		artifact.CreatedAt = now
	}
	return h.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "test_encode_id"}, {Name: "path"}},
		DoUpdates: clause.AssignmentColumns([]string{"state", "cleanup_error", "updated_at"}),
	}).Create(&artifact).Error
}

func (h AssetHandler) testEncodeCleanupCandidates(test models.TestEncode) ([]models.TestEncodeArtifact, error) {
	candidates := []models.TestEncodeArtifact{}
	if h.db.Migrator().HasTable(&models.TestEncodeArtifact{}) {
		if err := h.db.Where("test_encode_id = ?", test.ID).Order("id ASC").Find(&candidates).Error; err != nil {
			return nil, fmt.Errorf("load owned Test Encode artifacts: %w", err)
		}
	}
	seen := map[string]struct{}{}
	owned := make([]models.TestEncodeArtifact, 0, len(candidates))
	for _, artifact := range candidates {
		seen[artifact.Path] = struct{}{}
		if artifact.State != "removed" {
			owned = append(owned, artifact)
		}
	}
	candidates = owned
	addLegacy := func(path, kind string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if _, exists := seen[path]; exists {
			return
		}
		seen[path] = struct{}{}
		candidates = append(candidates, models.TestEncodeArtifact{
			TestEncodeID: test.ID, LibraryID: test.LibraryID, Path: path, Type: kind,
		})
	}
	addLegacy(test.OutputPath, models.TestEncodeArtifactMediaOutput)
	addLegacy(test.TemporaryPath, models.TestEncodeArtifactTemporaryMedia)
	for _, path := range subtitleArtifactPaths(test.SubtitleArtifacts) {
		addLegacy(path, models.TestEncodeArtifactSubtitle)
	}
	return candidates, nil
}

func (h AssetHandler) testEncodeArtifactHasActiveReference(ownerID uint, path string) (bool, error) {
	var active []models.TestEncode
	if err := h.db.Where("id <> ? AND status IN ?", ownerID, []string{testEncodeWaiting, testEncodeRunning}).Find(&active).Error; err != nil {
		return false, fmt.Errorf("check active Test Encode ownership: %w", err)
	}
	for _, test := range active {
		if testEncodePathsReferToSameFile(path, test.OutputPath) || testEncodePathsReferToSameFile(path, test.TemporaryPath) {
			return true, nil
		}
		for _, sidecarPath := range subtitleArtifactPaths(test.SubtitleArtifacts) {
			if testEncodePathsReferToSameFile(path, sidecarPath) {
				return true, nil
			}
		}
	}
	if h.db.Migrator().HasTable(&models.TestEncodeArtifact{}) {
		var artifact models.TestEncodeArtifact
		err := h.db.Joins("JOIN test_encodes ON test_encodes.id = test_encode_artifacts.test_encode_id").
			Where("test_encode_artifacts.test_encode_id <> ? AND test_encodes.status IN ? AND test_encode_artifacts.path = ?", ownerID, []string{testEncodeWaiting, testEncodeRunning}, filepath.Clean(path)).
			First(&artifact).Error
		if err == nil {
			return true, nil
		}
		if err != gorm.ErrRecordNotFound {
			return false, fmt.Errorf("check active Test Encode artifact ownership: %w", err)
		}
	}
	return false, nil
}

func testEncodeOperationActive(id uint) bool {
	testEncodeProcesses.Lock()
	defer testEncodeProcesses.Unlock()
	_, active := testEncodeProcesses.cancel[id]
	return active
}

func testEncodeActiveOperationIDs() []uint {
	testEncodeProcesses.Lock()
	defer testEncodeProcesses.Unlock()
	ids := make([]uint, 0, len(testEncodeProcesses.cancel))
	for id := range testEncodeProcesses.cancel {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func validateTestEncodeArtifactPath(candidate, root string) error {
	candidate = strings.TrimSpace(candidate)
	root = strings.TrimSpace(root)
	if candidate == "" || root == "" || !filepath.IsAbs(candidate) || !filepath.IsAbs(root) || filepath.Clean(candidate) != candidate {
		return fmt.Errorf("refusing to delete Test Encode artifact with an invalid or traversing path")
	}
	resolvedCandidate, err := resolvePathWithMissingSuffix(candidate)
	if err != nil {
		return fmt.Errorf("resolve Test Encode artifact path: %w", err)
	}
	resolvedRoot, err := resolvePathWithMissingSuffix(root)
	if err != nil {
		return fmt.Errorf("resolve configured Test Encode root: %w", err)
	}
	if !pathIsInside(resolvedCandidate, resolvedRoot) {
		return fmt.Errorf("refusing to delete Test Encode file outside its configured root")
	}
	if info, err := os.Lstat(candidate); err == nil {
		if info.IsDir() {
			return fmt.Errorf("refusing to delete a Test Encode directory")
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to delete a symlinked Test Encode artifact")
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect Test Encode artifact path: %w", err)
	}
	return nil
}

func resolvePathWithMissingSuffix(path string) (string, error) {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	current := abs
	suffix := []string{}
	for {
		resolved, resolveErr := filepath.EvalSymlinks(current)
		if resolveErr == nil {
			for index := len(suffix) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, suffix[index])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(resolveErr) {
			return "", resolveErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", resolveErr
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func testEncodePathsReferToSameFile(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return false
	}
	leftAbs, leftErr := filepath.Abs(filepath.Clean(left))
	rightAbs, rightErr := filepath.Abs(filepath.Clean(right))
	if leftErr == nil && rightErr == nil && leftAbs == rightAbs {
		return true
	}
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	return leftErr == nil && rightErr == nil && os.SameFile(leftInfo, rightInfo)
}
