package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
)

type cancellationPolicy struct {
	DeleteGeneratedFiles           bool
	DeletePartialOutputFromStaging bool
	ControlledRoots                []string
}

func loadCancellationPolicy(db *gorm.DB) cancellationPolicy {
	policy := cancellationPolicy{
		ControlledRoots: []string{"/media/staging", "/mwp/work", "/mwp/work/temp", "/tmp/mvforge"},
	}
	var setting models.AppSetting
	if result := db.Where("key = ?", "cancellationPolicy").Limit(1).Find(&setting); result.Error == nil && result.RowsAffected > 0 {
		policy.DeleteGeneratedFiles = boolSetting(setting.Value["deleteGeneratedFiles"], false)
		policy.DeletePartialOutputFromStaging = boolSetting(setting.Value["deletePartialOutputFromStaging"], false)
		policy.ControlledRoots = stringSliceFromUnknown(setting.Value["controlledRoots"])
	}
	return policy
}

func cleanupCanceledJob(db *gorm.DB, job models.QueueJob) error {
	policy := loadCancellationPolicy(db)
	if !policy.DeleteGeneratedFiles && !policy.DeletePartialOutputFromStaging {
		return nil
	}

	outputPath := filepath.Clean(strings.TrimSpace(job.OutputPath))
	if outputPath == "." || outputPath == "" || !pathInsideAnyRoot(outputPath, policy.ControlledRoots) {
		return nil
	}
	if policy.DeleteGeneratedFiles {
		workspace := filepath.Dir(outputPath)
		if filepath.Base(workspace) != fmt.Sprintf("job-%d", job.ID) || !pathInsideAnyRoot(workspace, policy.ControlledRoots) {
			return nil
		}
		return os.RemoveAll(workspace)
	}
	if err := os.Remove(outputPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func pathInsideAnyRoot(path string, roots []string) bool {
	for _, root := range roots {
		if strings.TrimSpace(root) != "" && pathIsInside(path, filepath.Clean(root)) {
			return true
		}
	}
	return false
}
