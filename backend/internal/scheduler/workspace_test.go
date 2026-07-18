package scheduler

import (
	"testing"
	"time"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEvaluateWorkspaceSelectsCopyMode(t *testing.T) {
	db := workspaceTestDB(t, 500<<30)
	decision, err := EvaluateWorkspace(db, models.ExecutionPlan{EstimatedWorkspaceBytes: 20 << 30, EstimatedOutputMaxBytes: 10 << 30})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed || decision.Mode != WorkspaceModeCopyToWork {
		t.Fatalf("unexpected decision: %#v", decision)
	}
}

func TestEvaluateWorkspaceWaitsWhenWorkDiskIsInsufficient(t *testing.T) {
	db := workspaceTestDB(t, 45<<30)
	decision, err := EvaluateWorkspace(db, models.ExecutionPlan{EstimatedWorkspaceBytes: 20 << 30, EstimatedOutputMaxBytes: 10 << 30})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed {
		t.Fatalf("expected workspace wait: %#v", decision)
	}
}

func TestEvaluateWorkspaceFallsBackToDirectMode(t *testing.T) {
	db := workspaceTestDB(t, 45<<30)
	setting := models.AppSetting{Key: "workspace", Value: models.JSONMap{"preferredMode": WorkspaceModeCopyToWork, "fallbackMode": WorkspaceModeDirect, "allowDirectMode": true, "estimateRequiredSpace": true}}
	if err := db.Create(&setting).Error; err != nil {
		t.Fatal(err)
	}
	decision, err := EvaluateWorkspace(db, models.ExecutionPlan{EstimatedWorkspaceBytes: 20 << 30, EstimatedOutputMaxBytes: 10 << 30})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Allowed || decision.Mode != WorkspaceModeDirect {
		t.Fatalf("unexpected fallback: %#v", decision)
	}
}

func TestEvaluateWorkspaceRejectsDirectModeForNASProfile(t *testing.T) {
	db := workspaceTestDB(t, 45<<30)
	if err := db.Model(&models.RuntimeSnapshot{}).Where("1 = 1").Update("selected_profile", "nas_safe").Error; err != nil {
		t.Fatal(err)
	}
	setting := models.AppSetting{Key: "workspace", Value: models.JSONMap{"preferredMode": WorkspaceModeCopyToWork, "fallbackMode": WorkspaceModeDirect, "allowDirectMode": true, "estimateRequiredSpace": true}}
	if err := db.Create(&setting).Error; err != nil {
		t.Fatal(err)
	}
	decision, err := EvaluateWorkspace(db, models.ExecutionPlan{EstimatedWorkspaceBytes: 20 << 30, EstimatedOutputMaxBytes: 10 << 30})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed {
		t.Fatalf("expected NAS policy to reject direct mode: %#v", decision)
	}
}

func workspaceTestDB(t *testing.T, free int64) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}, &models.RuntimeSnapshot{}); err != nil {
		t.Fatal(err)
	}
	disks := models.JSONMap{"workspace": models.JSONMap{"availableBytes": float64(free)}, "library": models.JSONMap{"availableBytes": float64(500 << 30)}}
	if err := db.Create(&models.RuntimeSnapshot{DetectedAt: time.Now(), SelectedProfile: "desktop_balanced", AvailableMemoryBytes: 16 << 30, Disks: disks}).Error; err != nil {
		t.Fatal(err)
	}
	return db
}
