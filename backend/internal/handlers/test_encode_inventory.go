package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
)

type testEncodeStorageSummary struct {
	TotalRuns            int64 `json:"totalRuns"`
	KeepRuns             int64 `json:"keepRuns"`
	TotalKnownBytes      int64 `json:"totalKnownBytes"`
	KeepKnownBytes       int64 `json:"keepKnownBytes"`
	CleanupEligibleRuns  int64 `json:"cleanupEligibleRuns"`
	CleanupEligibleBytes int64 `json:"cleanupEligibleBytes"`
	UnknownSizeArtifacts int64 `json:"unknownSizeArtifacts"`
}

func (h AssetHandler) listTestEncodeInventory(c *gin.Context) {
	policy := testEncodePolicyFromSettings(h.db)
	limit := 100
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > policy.MaxReconcileBatchSize {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("limit must be between 1 and %d", policy.MaxReconcileBatchSize)})
			return
		}
		limit = parsed
	} else if policy.MaxReconcileBatchSize < limit {
		limit = policy.MaxReconcileBatchSize
	}
	offset := 0
	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 || parsed > 100000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "offset must be between 0 and 100000"})
			return
		}
		offset = parsed
	}

	query := h.db.Model(&models.TestEncode{}).Where("deleted_at IS NULL")
	if raw := strings.TrimSpace(c.Query("sourcePath")); raw != "" {
		resolved, err := h.resolveMediaPath(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		query = query.Where("source_path = ?", filepath.Clean(resolved))
	}
	if raw := strings.TrimSpace(c.Query("sourceAssetId")); raw != "" {
		id, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || id == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "sourceAssetId must be a positive integer"})
			return
		}
		query = query.Where("source_asset_id = ?", id)
	}
	if raw := strings.TrimSpace(c.Query("libraryId")); raw != "" {
		id, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || id == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "libraryId must be a positive integer"})
			return
		}
		query = query.Where("library_id = ?", id)
	}
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	switch retention := strings.ToLower(strings.TrimSpace(c.Query("retention"))); retention {
	case "":
	case "keep":
		query = query.Where("keep = ?", true)
	case "temporary":
		query = query.Where("keep = ?", false)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "retention must be keep or temporary"})
		return
	}
	if reconciliation := strings.TrimSpace(c.Query("reconciliation")); reconciliation != "" {
		query = query.Where("reconciliation_status = ?", reconciliation)
	}
	if raw := strings.TrimSpace(c.Query("ageDays")); raw != "" {
		days, err := strconv.Atoi(raw)
		if err != nil || days < 1 || days > 3650 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ageDays must be between 1 and 3650"})
			return
		}
		query = query.Where("created_at <= ?", time.Now().Add(-time.Duration(days)*24*time.Hour))
	}

	var tests []models.TestEncode
	if err := query.Order("created_at DESC").Order("id DESC").Limit(limit).Offset(offset).Find(&tests).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.decorateTestEncodeInventory(tests); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(c.Query("sourcePath")) != "" {
		for index := range tests {
			h.decorateTestEncode(&tests[index])
		}
	}
	c.JSON(http.StatusOK, tests)
}

func (h AssetHandler) decorateTestEncodeInventory(tests []models.TestEncode) error {
	if len(tests) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(tests))
	libraryIDs := make(map[uint]struct{}, len(tests))
	for _, test := range tests {
		ids = append(ids, test.ID)
		libraryIDs[test.LibraryID] = struct{}{}
	}
	rootIDs := make([]uint, 0, len(libraryIDs))
	for id := range libraryIDs {
		rootIDs = append(rootIDs, id)
	}
	var libraries []models.Library
	if err := h.db.Where("id IN ?", rootIDs).Find(&libraries).Error; err != nil {
		return fmt.Errorf("load Test Encode inventory Library roots: %w", err)
	}
	roots := make(map[uint]string, len(libraries))
	for _, library := range libraries {
		roots[library.ID] = library.DestinationPath
	}
	artifactsByTest := map[uint][]models.TestEncodeArtifact{}
	if h.db.Migrator().HasTable(&models.TestEncodeArtifact{}) {
		var artifacts []models.TestEncodeArtifact
		if err := h.db.Where("test_encode_id IN ?", ids).Order("id ASC").Find(&artifacts).Error; err != nil {
			return fmt.Errorf("load Test Encode inventory artifacts: %w", err)
		}
		for _, artifact := range artifacts {
			artifactsByTest[artifact.TestEncodeID] = append(artifactsByTest[artifact.TestEncodeID], artifact)
		}
	}
	for index := range tests {
		test := &tests[index]
		artifacts := artifactsByTest[test.ID]
		root := roots[test.LibraryID]
		outputPath := test.OutputPath
		for _, artifact := range artifacts {
			if artifact.Type == models.TestEncodeArtifactMediaOutput && outputPath == "" {
				outputPath = artifact.Path
			}
			if artifact.State == "removed" {
				continue
			}
			test.ArtifactCount++
			if artifact.Type == models.TestEncodeArtifactSubtitle {
				test.SidecarArtifactCount++
			}
			if root == "" || validateTestEncodeArtifactPath(artifact.Path, root) != nil {
				test.UnknownSizeArtifactCount++
				continue
			}
			if info, err := os.Stat(artifact.Path); err == nil && !info.IsDir() {
				size := artifact.SizeBytes
				if size <= 0 {
					size = info.Size()
				}
				test.KnownDiskUsageBytes += max(int64(0), size)
			} else if artifact.SizeBytes <= 0 {
				test.UnknownSizeArtifactCount++
			}
		}
		if outputPath != "" && root != "" && validateTestEncodeArtifactPath(outputPath, root) == nil {
			if info, err := os.Stat(outputPath); err == nil && !info.IsDir() {
				test.OutputExists = true
				if test.OutputSizeBytes <= 0 {
					test.OutputSizeBytes = info.Size()
				}
			} else {
				test.OutputExists = false
			}
		}
	}
	return nil
}

func (h AssetHandler) TestEncodeInventorySummary(c *gin.Context) {
	var summary testEncodeStorageSummary
	if err := h.db.Model(&models.TestEncode{}).Where("deleted_at IS NULL").Count(&summary.TotalRuns).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := h.db.Model(&models.TestEncode{}).Where("deleted_at IS NULL AND keep = ?", true).Count(&summary.KeepRuns).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.db.Migrator().HasTable(&models.TestEncodeArtifact{}) {
		var totals struct {
			TotalKnownBytes      int64
			KeepKnownBytes       int64
			UnknownSizeArtifacts int64
		}
		err := h.db.Table("test_encode_artifacts AS artifacts").
			Select(`
				COALESCE(SUM(artifacts.size_bytes), 0) AS total_known_bytes,
				COALESCE(SUM(CASE WHEN tests.keep = ? THEN artifacts.size_bytes ELSE 0 END), 0) AS keep_known_bytes,
				COALESCE(SUM(CASE WHEN artifacts.size_bytes <= 0 THEN 1 ELSE 0 END), 0) AS unknown_size_artifacts
			`, true).
			Joins("JOIN test_encodes AS tests ON tests.id = artifacts.test_encode_id").
			Where("tests.deleted_at IS NULL AND artifacts.state <> ?", "removed").Scan(&totals).Error
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		summary.TotalKnownBytes = totals.TotalKnownBytes
		summary.KeepKnownBytes = totals.KeepKnownBytes
		summary.UnknownSizeArtifacts = totals.UnknownSizeArtifacts
	}
	policy := testEncodePolicyFromSettings(h.db)
	var eligible []models.TestEncode
	query := h.db.Where("deleted_at IS NULL AND keep = ? AND expires_at IS NOT NULL AND expires_at <= ? AND status NOT IN ?",
		false, time.Now(), []string{testEncodeWaiting, testEncodeRunning, testEncodeDeleted})
	if activeIDs := testEncodeActiveOperationIDs(); len(activeIDs) > 0 {
		query = query.Where("id NOT IN ?", activeIDs)
	}
	if err := query.Order("expires_at ASC").Order("id ASC").Limit(policy.MaxCleanupBatchSize).Find(&eligible).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	filtered := eligible[:0]
	for _, test := range eligible {
		if !testEncodeOperationActive(test.ID) {
			filtered = append(filtered, test)
		}
	}
	eligible = filtered
	if err := h.decorateTestEncodeInventory(eligible); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for _, test := range eligible {
		summary.CleanupEligibleRuns++
		size := test.KnownDiskUsageBytes
		if size <= 0 {
			size = test.OutputSizeBytes
		}
		summary.CleanupEligibleBytes += max(int64(0), size)
	}
	c.JSON(http.StatusOK, summary)
}
