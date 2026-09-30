package handlers

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
)

type testEncodeDeleteRequest struct {
	IDs         []uint `json:"ids"`
	ConfirmKeep bool   `json:"confirmKeep"`
}

type testEncodeDeleteResult struct {
	ID      uint   `json:"id"`
	Deleted bool   `json:"deleted"`
	Error   string `json:"error,omitempty"`
}

type testEncodeDeletePreview struct {
	TestEncodeCount    int   `json:"testEncodeCount"`
	MediaOutputCount   int   `json:"mediaOutputCount"`
	OtherArtifactCount int   `json:"otherArtifactCount"`
	EstimatedBytes     int64 `json:"estimatedBytes"`
	KeepCount          int   `json:"keepCount"`
}

func (h AssetHandler) DeleteTestEncodePreview(c *gin.Context) {
	request, tests, ok := h.loadTestEncodeDeleteSelection(c)
	if !ok {
		return
	}
	preview := testEncodeDeletePreview{TestEncodeCount: len(tests)}
	for _, test := range tests {
		if testEncodeRecordIsActive(test) {
			c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("Test Encode %d is active and cannot be deleted", test.ID)})
			return
		}
		if test.Keep {
			preview.KeepCount++
		}
		_, artifacts, err := h.testEncodeArtifactCleanupPlan(test, 0)
		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("Test Encode %d: %v", test.ID, err)})
			return
		}
		var library models.Library
		if err := h.db.First(&library, test.LibraryID).Error; err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("Test Encode %d has no configured Library root", test.ID)})
			return
		}
		for _, artifact := range artifacts {
			if artifact.Type == models.TestEncodeArtifactMediaOutput {
				preview.MediaOutputCount++
			} else {
				preview.OtherArtifactCount++
			}
			size := artifact.SizeBytes
			if size <= 0 && validateTestEncodeArtifactPath(artifact.Path, library.DestinationPath) == nil {
				if info, err := os.Stat(artifact.Path); err == nil && !info.IsDir() {
					size = info.Size()
				}
			}
			preview.EstimatedBytes += max(int64(0), size)
		}
	}
	_ = request
	c.JSON(http.StatusOK, preview)
}

func (h AssetHandler) DeleteSelectedTestEncodes(c *gin.Context) {
	request, tests, ok := h.loadTestEncodeDeleteSelection(c)
	if !ok {
		return
	}
	for _, test := range tests {
		if testEncodeRecordIsActive(test) {
			c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("Test Encode %d is active and cannot be deleted", test.ID)})
			return
		}
		if test.Keep && !request.ConfirmKeep {
			c.JSON(http.StatusConflict, gin.H{"error": "explicit confirmation is required to delete Keep Test Encodes"})
			return
		}
		if _, _, err := h.testEncodeArtifactCleanupPlan(test, 0); err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("Test Encode %d: %v", test.ID, err)})
			return
		}
	}
	results := make([]testEncodeDeleteResult, 0, len(tests))
	for _, test := range tests {
		if err := h.deleteTestEncodeRecord(test, request.ConfirmKeep); err != nil {
			results = append(results, testEncodeDeleteResult{ID: test.ID, Error: err.Error()})
			continue
		}
		results = append(results, testEncodeDeleteResult{ID: test.ID, Deleted: true})
	}
	c.JSON(http.StatusOK, gin.H{"results": results})
}

func (h AssetHandler) deleteTestEncodeRecord(test models.TestEncode, confirmKeep bool) error {
	if test.Keep && !confirmKeep {
		return fmt.Errorf("explicit confirmation is required to delete a Keep Test Encode")
	}
	if testEncodeRecordIsActive(test) {
		return fmt.Errorf("cancel the generating test before deleting it")
	}
	mutated, cleanupErr := h.removeTestEncodeFilesWithEvidence(test)
	if cleanupErr != nil {
		now := time.Now().UTC()
		_ = h.db.Model(&models.TestEncode{}).Where("id = ?", test.ID).Updates(map[string]any{
			"reconciliation_status": "cleanup_failed", "reconciliation_message": cleanupErr.Error(),
			"last_reconciled_at": now, "updated_at": now,
		}).Error
		return cleanupErr
	}
	if test.Status == testEncodeReady && mutated {
		_ = recordJellyfinTrigger(h.db, jellyfinTrigger{
			Reason: jellyfinReasonTestRemoved, SourceKey: testEncodeJellyfinSourceKey(jellyfinReasonTestRemoved, test.ID),
			LibraryID: test.LibraryID, TestEncodeID: test.ID, Reconcile: true,
		})
	}
	now := time.Now().UTC()
	if err := h.db.Model(&test).Updates(map[string]any{
		"status": testEncodeDeleted, "phase": "deleted", "deleted_at": &now,
		"output_path": "", "temporary_path": "", "subtitle_artifacts": models.JSONList{}, "updated_at": now,
	}).Error; err != nil {
		return fmt.Errorf("mark Test Encode deleted after artifact cleanup: %w", err)
	}
	return nil
}

func testEncodeRecordIsActive(test models.TestEncode) bool {
	if testEncodeOperationActive(test.ID) {
		return true
	}
	return (test.Status == testEncodeWaiting || test.Status == testEncodeRunning) && test.ReconciliationStatus != "stale"
}

func (h AssetHandler) loadTestEncodeDeleteSelection(c *gin.Context) (testEncodeDeleteRequest, []models.TestEncode, bool) {
	policy := testEncodePolicyFromSettings(h.db)
	var request testEncodeDeleteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "a JSON request with Test Encode IDs is required"})
		return request, nil, false
	}
	if len(request.IDs) == 0 || len(request.IDs) > policy.MaxCleanupBatchSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("select between 1 and %d Test Encodes", policy.MaxCleanupBatchSize)})
		return request, nil, false
	}
	seen := make(map[uint]struct{}, len(request.IDs))
	for _, id := range request.IDs {
		if id == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Test Encode IDs must be positive"})
			return request, nil, false
		}
		if _, exists := seen[id]; exists {
			c.JSON(http.StatusBadRequest, gin.H{"error": "duplicate Test Encode ID"})
			return request, nil, false
		}
		seen[id] = struct{}{}
	}
	var tests []models.TestEncode
	if err := h.db.Where("deleted_at IS NULL AND id IN ?", request.IDs).Order("id ASC").Find(&tests).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return request, nil, false
	}
	if len(tests) != len(request.IDs) {
		c.JSON(http.StatusNotFound, gin.H{"error": "one or more selected Test Encodes do not exist"})
		return request, nil, false
	}
	return request, tests, true
}

func (h AssetHandler) DeleteTestEncode(c *gin.Context) {
	test, ok := h.testEncodeFromParam(c)
	if !ok {
		return
	}
	confirmKeep := c.Query("confirmKeep") == "true"
	if err := h.deleteTestEncodeRecord(test, confirmKeep); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": test.ID, "status": testEncodeDeleted})
}
