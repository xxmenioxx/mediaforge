package handlers

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
)

type testEncodeReconcileRequest struct {
	IDs []uint `json:"ids"`
}

type testEncodeReconcileResult struct {
	ID                   uint   `json:"id"`
	PreviousStatus       string `json:"previousStatus"`
	ReconciliationStatus string `json:"reconciliationStatus"`
	Message              string `json:"message,omitempty"`
	OutputExists         bool   `json:"outputExists"`
	ArtifactCount        int    `json:"artifactCount"`
	SkippedActive        bool   `json:"skippedActive,omitempty"`
}

func (h AssetHandler) ReconcileTestEncodes(c *gin.Context) {
	policy := testEncodePolicyFromSettings(h.db)
	var request testEncodeReconcileRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid reconciliation request"})
			return
		}
	}
	if len(request.IDs) > policy.MaxReconcileBatchSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("at most %d Test Encodes can be reconciled at once", policy.MaxReconcileBatchSize)})
		return
	}
	query := h.db.Where("deleted_at IS NULL AND status <> ?", testEncodeDeleted)
	if len(request.IDs) > 0 {
		seen := make(map[uint]struct{}, len(request.IDs))
		for _, id := range request.IDs {
			if id == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Test Encode IDs must be positive"})
				return
			}
			if _, duplicate := seen[id]; duplicate {
				c.JSON(http.StatusBadRequest, gin.H{"error": "duplicate Test Encode ID"})
				return
			}
			seen[id] = struct{}{}
		}
		query = query.Where("id IN ?", request.IDs)
	} else {
		query = query.Where("last_reconciled_at IS NULL OR last_reconciled_at <= ?", time.Now().Add(-15*time.Minute)).Limit(policy.MaxReconcileBatchSize)
	}
	var tests []models.TestEncode
	if err := query.Order("created_at ASC").Order("id ASC").Find(&tests).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	results := make([]testEncodeReconcileResult, 0, len(tests))
	for _, test := range tests {
		result, err := h.reconcileTestEncode(test)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		results = append(results, result)
	}
	orphanLimit := policy.MaxReconcileBatchSize - len(results)
	if orphanLimit < 0 {
		orphanLimit = 0
	}
	orphans, err := h.reconcileOrphanArtifactRows(orphanLimit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"results": results, "processed": len(results), "orphanedArtifacts": orphans, "limit": policy.MaxReconcileBatchSize})
}

func (h AssetHandler) reconcileTestEncode(test models.TestEncode) (testEncodeReconcileResult, error) {
	result := testEncodeReconcileResult{ID: test.ID, PreviousStatus: test.Status, ReconciliationStatus: "healthy"}
	if testEncodeOperationActive(test.ID) {
		result.ReconciliationStatus = test.ReconciliationStatus
		result.Message = "active Test Encode operation was not reconciled"
		result.SkippedActive = true
		return result, nil
	}
	var library models.Library
	if err := h.db.First(&library, test.LibraryID).Error; err != nil {
		result.ReconciliationStatus = "partial"
		result.Message = "configured Library root is unavailable"
		return h.persistTestEncodeReconciliation(test, result)
	}
	artifacts, err := h.testEncodeCleanupCandidates(test)
	if err != nil {
		return result, err
	}
	for _, artifact := range artifacts {
		if artifact.State == "removed" {
			continue
		}
		result.ArtifactCount++
		if err := validateTestEncodeArtifactPath(artifact.Path, library.DestinationPath); err != nil {
			result.ReconciliationStatus = "partial"
			result.Message = "a persisted artifact path is outside its configured Library root"
		}
	}
	outputPath := strings.TrimSpace(test.OutputPath)
	if outputPath == "" {
		for _, artifact := range artifacts {
			if artifact.Type == models.TestEncodeArtifactMediaOutput {
				outputPath = artifact.Path
				break
			}
		}
	}
	if outputPath != "" {
		if err := validateTestEncodeArtifactPath(outputPath, library.DestinationPath); err != nil {
			result.ReconciliationStatus = "partial"
			result.Message = "the persisted output path is outside its configured Library root"
		} else if info, statErr := os.Stat(outputPath); statErr == nil && !info.IsDir() {
			result.OutputExists = true
			if test.OutputSizeBytes <= 0 {
				_ = h.db.Model(&models.TestEncode{}).Where("id = ?", test.ID).Update("output_size_bytes", info.Size()).Error
			}
		} else if statErr != nil && !os.IsNotExist(statErr) {
			result.ReconciliationStatus = "partial"
			result.Message = "the output could not be inspected"
		}
	}
	missingExpected := false
	remainingSidecars := false
	for _, artifact := range artifacts {
		if artifact.State == "removed" || artifact.Type == models.TestEncodeArtifactMediaOutput || artifact.Type == models.TestEncodeArtifactTemporaryMedia {
			continue
		}
		if err := validateTestEncodeArtifactPath(artifact.Path, library.DestinationPath); err != nil {
			continue
		}
		info, statErr := os.Stat(artifact.Path)
		exists := statErr == nil && !info.IsDir()
		if exists {
			remainingSidecars = true
			if artifact.ID != 0 && artifact.SizeBytes != info.Size() {
				if err := h.db.Model(&models.TestEncodeArtifact{}).Where("id = ?", artifact.ID).Update("size_bytes", info.Size()).Error; err != nil {
					return result, fmt.Errorf("persist Test Encode artifact size: %w", err)
				}
			}
		}
		if artifact.Expected && !exists {
			missingExpected = true
		}
	}
	if test.Status == testEncodeWaiting || test.Status == testEncodeRunning {
		if time.Since(test.CreatedAt) >= 5*time.Minute {
			result.ReconciliationStatus = "stale"
			result.Message = "no active operation owns this waiting or running Test Encode"
		} else {
			result.ReconciliationStatus = "unknown"
			result.Message = "waiting for the active operation to appear"
		}
	} else if test.Status == testEncodeReady && !result.OutputExists {
		if remainingSidecars {
			result.ReconciliationStatus = "partial"
			result.Message = "the media output is missing while owned sidecars remain"
		} else {
			result.ReconciliationStatus = "missing_output"
			result.Message = "the persisted Test Encode output is missing"
		}
	} else if test.Status == testEncodeReady && missingExpected {
		result.ReconciliationStatus = "partial"
		result.Message = "one or more expected owned sidecars are missing"
	} else if (test.Status == testEncodeFailed || test.Status == testEncodeCanceled) && result.ArtifactCount > 0 {
		result.ReconciliationStatus = "partial"
		result.Message = "failed or cancelled Test Encode still has owned artifacts"
	}
	return h.persistTestEncodeReconciliation(test, result)
}

func (h AssetHandler) persistTestEncodeReconciliation(test models.TestEncode, result testEncodeReconcileResult) (testEncodeReconcileResult, error) {
	now := time.Now().UTC()
	if err := h.db.Model(&models.TestEncode{}).Where("id = ?", test.ID).Updates(map[string]any{
		"reconciliation_status":  result.ReconciliationStatus,
		"reconciliation_message": result.Message,
		"last_reconciled_at":     now,
		"updated_at":             now,
	}).Error; err != nil {
		return result, fmt.Errorf("persist Test Encode reconciliation: %w", err)
	}
	return result, nil
}
