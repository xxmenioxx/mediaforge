package handlers

import (
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type testEncodeOrphanArtifact struct {
	ID           uint   `json:"id"`
	TestEncodeID uint   `json:"testEncodeId"`
	LibraryID    uint   `json:"libraryId"`
	Path         string `json:"path"`
	Type         string `json:"type"`
	SizeBytes    int64  `json:"sizeBytes"`
	Exists       bool   `json:"exists"`
	Safe         bool   `json:"safe"`
	Message      string `json:"message,omitempty"`
}

func (h AssetHandler) reconcileOrphanArtifactRows(limit int) ([]testEncodeOrphanArtifact, error) {
	if limit <= 0 || !h.db.Migrator().HasTable(&models.TestEncodeArtifact{}) {
		return []testEncodeOrphanArtifact{}, nil
	}
	var artifacts []models.TestEncodeArtifact
	if err := h.db.Model(&models.TestEncodeArtifact{}).
		Joins("LEFT JOIN test_encodes AS tests ON tests.id = test_encode_artifacts.test_encode_id").
		Where("test_encode_artifacts.state <> ? AND (tests.id IS NULL OR tests.deleted_at IS NOT NULL)", "removed").
		Order("test_encode_artifacts.id ASC").Limit(limit).Find(&artifacts).Error; err != nil {
		return nil, fmt.Errorf("load orphaned Test Encode artifact records: %w", err)
	}
	items := make([]testEncodeOrphanArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		if artifact.State != "orphaned" {
			if err := h.db.Model(&models.TestEncodeArtifact{}).Where("id = ?", artifact.ID).Update("state", "orphaned").Error; err != nil {
				return nil, fmt.Errorf("mark orphaned Test Encode artifact: %w", err)
			}
			artifact.State = "orphaned"
		}
		items = append(items, h.decorateTestEncodeOrphan(artifact))
	}
	return items, nil
}

func (h AssetHandler) ListOrphanedTestEncodeArtifacts(c *gin.Context) {
	policy := testEncodePolicyFromSettings(h.db)
	var artifacts []models.TestEncodeArtifact
	err := h.db.Model(&models.TestEncodeArtifact{}).
		Joins("LEFT JOIN test_encodes AS tests ON tests.id = test_encode_artifacts.test_encode_id").
		Where("test_encode_artifacts.state <> ? AND (tests.id IS NULL OR tests.deleted_at IS NOT NULL)", "removed").
		Order("test_encode_artifacts.id ASC").Limit(policy.MaxReconcileBatchSize).Find(&artifacts).Error
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	items := make([]testEncodeOrphanArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		item := h.decorateTestEncodeOrphan(artifact)
		items = append(items, item)
	}
	c.JSON(http.StatusOK, gin.H{"artifacts": items, "limit": policy.MaxReconcileBatchSize})
}

func (h AssetHandler) DeleteOrphanedTestEncodeArtifact(c *gin.Context) {
	artifactID, err := parsePositiveTestEncodeID(c.Param("artifactId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid Test Encode artifact ID"})
		return
	}
	var artifact models.TestEncodeArtifact
	if err := h.db.First(&artifact, artifactID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Test Encode artifact not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	var owner models.TestEncode
	ownerErr := h.db.Unscoped().First(&owner, artifact.TestEncodeID).Error
	if ownerErr != nil && ownerErr != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{"error": ownerErr.Error()})
		return
	}
	if ownerErr == nil && owner.DeletedAt == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "artifact still belongs to a retained Test Encode"})
		return
	}
	if artifact.State == "removed" {
		c.JSON(http.StatusConflict, gin.H{"error": "artifact was already cleaned up"})
		return
	}
	if ownerErr == nil && testEncodeOperationActive(owner.ID) {
		c.JSON(http.StatusConflict, gin.H{"error": "artifact is referenced by an active Test Encode operation"})
		return
	}
	var library models.Library
	if err := h.db.First(&library, artifact.LibraryID).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "configured Library root is unavailable"})
		return
	}
	if err := validateTestEncodeArtifactPath(artifact.Path, library.DestinationPath); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if ownerErr == nil && testEncodePathsReferToSameFile(artifact.Path, owner.SourcePath) {
		c.JSON(http.StatusConflict, gin.H{"error": "refusing to delete Test Encode source media"})
		return
	}
	if h.db.Migrator().HasTable(&models.AssetRecord{}) {
		var current models.AssetRecord
		if err := h.db.Where("path = ?", artifact.Path).First(&current).Error; err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "refusing to delete a current Library asset"})
			return
		} else if err != gorm.ErrRecordNotFound {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if ownerErr == nil {
		if referenced, err := h.testEncodeArtifactHasActiveReference(owner.ID, artifact.Path); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		} else if referenced {
			c.JSON(http.StatusConflict, gin.H{"error": "artifact is referenced by another active operation"})
			return
		}
	}
	var otherOwners int64
	if err := h.db.Model(&models.TestEncodeArtifact{}).Where("path = ? AND id <> ? AND state <> ?", artifact.Path, artifact.ID, "removed").Count(&otherOwners).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	} else if otherOwners > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "artifact path has additional persisted owners"})
		return
	}
	if info, err := os.Lstat(artifact.Path); err == nil && info.IsDir() {
		c.JSON(http.StatusConflict, gin.H{"error": "refusing to recursively delete a directory"})
		return
	} else if err != nil && !os.IsNotExist(err) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if err := os.Remove(artifact.Path); err != nil && !os.IsNotExist(err) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if err := h.setTestEncodeArtifactState(artifact, "removed", ""); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("record orphan cleanup: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": artifact.ID, "deleted": true})
}

func (h AssetHandler) decorateTestEncodeOrphan(artifact models.TestEncodeArtifact) testEncodeOrphanArtifact {
	item := testEncodeOrphanArtifact{ID: artifact.ID, TestEncodeID: artifact.TestEncodeID, LibraryID: artifact.LibraryID, Path: artifact.Path, Type: artifact.Type, SizeBytes: artifact.SizeBytes}
	var library models.Library
	if err := h.db.First(&library, artifact.LibraryID).Error; err != nil {
		item.Message = "configured Library root is unavailable"
		return item
	}
	if err := validateTestEncodeArtifactPath(artifact.Path, library.DestinationPath); err != nil {
		item.Message = "persisted artifact path is outside the configured Library root"
		return item
	}
	if owner, err := h.testEncodeSourceOwnsPath(artifact); err != nil {
		item.Message = "could not confirm source-media separation"
		return item
	} else if owner {
		item.Message = "artifact path resolves to source media"
		return item
	}
	if ownerErr := h.db.Migrator().HasTable(&models.AssetRecord{}); ownerErr {
		var current models.AssetRecord
		if err := h.db.Where("path = ?", artifact.Path).First(&current).Error; err == nil {
			item.Message = "artifact path is a current Library asset"
			return item
		} else if err != gorm.ErrRecordNotFound {
			item.Message = "could not confirm current Library asset identity"
			return item
		}
	}
	if testEncodeOperationActive(artifact.TestEncodeID) {
		item.Message = "an active Test Encode operation references this artifact"
		return item
	}
	if referenced, err := h.testEncodeArtifactHasActiveReference(artifact.TestEncodeID, artifact.Path); err != nil {
		item.Message = "could not confirm active operation references"
		return item
	} else if referenced {
		item.Message = "another active Test Encode operation references this artifact"
		return item
	}
	var otherOwners int64
	if err := h.db.Model(&models.TestEncodeArtifact{}).Where("path = ? AND id <> ? AND state <> ?", artifact.Path, artifact.ID, "removed").Count(&otherOwners).Error; err != nil {
		item.Message = "could not confirm exclusive artifact ownership"
		return item
	} else if otherOwners > 0 {
		item.Message = "artifact path has additional persisted owners"
		return item
	}
	if info, err := os.Stat(artifact.Path); err == nil && !info.IsDir() {
		item.Exists = true
		item.Safe = true
		if item.SizeBytes <= 0 {
			item.SizeBytes = info.Size()
		}
	} else if err != nil && !os.IsNotExist(err) {
		item.Message = "artifact could not be inspected"
	} else {
		item.Safe = true
	}
	return item
}

func (h AssetHandler) testEncodeSourceOwnsPath(artifact models.TestEncodeArtifact) (bool, error) {
	var owners []models.TestEncode
	if err := h.db.Unscoped().Where("id = ? OR source_path = ?", artifact.TestEncodeID, artifact.Path).Find(&owners).Error; err != nil {
		return false, err
	}
	for _, owner := range owners {
		if testEncodePathsReferToSameFile(artifact.Path, owner.SourcePath) {
			return true, nil
		}
	}
	return false, nil
}

func parsePositiveTestEncodeID(value string) (uint, error) {
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil || parsed == 0 {
		return 0, fmt.Errorf("invalid ID")
	}
	return uint(parsed), nil
}
