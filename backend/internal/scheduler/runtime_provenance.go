package scheduler

import (
	"fmt"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/anuelvs/mvforge/backend/internal/runtimeinfo"
	"gorm.io/gorm"
)

// runtimeSnapshotForPlan returns the immutable evidence already bound to a
// material plan version. A plan that has not reached its freeze boundary falls
// back to the current runtime authority; callers that persist the plan must
// bind that result exactly once at the material freeze boundary.
func runtimeSnapshotForPlan(db *gorm.DB, plan *models.ExecutionPlan) (models.RuntimeSnapshot, error) {
	if plan.RuntimeSnapshotID != nil {
		var snapshot models.RuntimeSnapshot
		if err := db.First(&snapshot, *plan.RuntimeSnapshotID).Error; err != nil {
			return models.RuntimeSnapshot{}, fmt.Errorf("load frozen runtime snapshot %d: %w", *plan.RuntimeSnapshotID, err)
		}
		return snapshot, nil
	}

	state, err := runtimeinfo.Current(db)
	if err != nil {
		return models.RuntimeSnapshot{}, fmt.Errorf("load current runtime state: %w", err)
	}
	return runtimeSnapshotFromCurrentState(state), nil
}

// freezeExecutionPlanRuntime binds the current immutable runtime evidence to
// a persisted material plan exactly once. The conditional update makes a
// concurrent retry reuse the winner's reference instead of replacing it.
// CaptureAuditSnapshot supplies the immutable row at this boundary and the
// conditional update binds it to the plan in the same transaction.
func freezeExecutionPlanRuntime(db *gorm.DB, plan *models.ExecutionPlan) (models.RuntimeSnapshot, error) {
	if plan.RuntimeSnapshotID != nil {
		return runtimeSnapshotForPlan(db, plan)
	}
	if plan.ID == 0 {
		return models.RuntimeSnapshot{}, fmt.Errorf("execution plan must be persisted before runtime freeze")
	}

	var snapshot models.RuntimeSnapshot
	err := db.Transaction(func(tx *gorm.DB) error {
		var current models.ExecutionPlan
		if err := tx.First(&current, plan.ID).Error; err != nil {
			return fmt.Errorf("load execution plan %d for runtime freeze: %w", plan.ID, err)
		}
		if current.RuntimeSnapshotID != nil {
			if err := tx.First(&snapshot, *current.RuntimeSnapshotID).Error; err != nil {
				return fmt.Errorf("load frozen execution-plan runtime snapshot %d: %w", *current.RuntimeSnapshotID, err)
			}
			plan.RuntimeSnapshotID = current.RuntimeSnapshotID
			plan.RuntimeProfile = current.RuntimeProfile
			return nil
		}

		state, err := runtimeinfo.Current(tx)
		if err != nil {
			return fmt.Errorf("capture execution-plan runtime evidence: %w", err)
		}
		currentSnapshot, err := runtimeinfo.CaptureAuditSnapshot(tx, state, "execution_plan_freeze")
		if err != nil {
			return fmt.Errorf("capture execution-plan runtime evidence: %w", err)
		}
		result := tx.Model(&models.ExecutionPlan{}).
			Where("id = ? AND runtime_snapshot_id IS NULL", current.ID).
			Updates(map[string]any{"runtime_snapshot_id": currentSnapshot.ID, "runtime_profile": currentSnapshot.SelectedProfile})
		if result.Error != nil {
			return fmt.Errorf("bind execution-plan runtime snapshot: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			if err := tx.First(&current, plan.ID).Error; err != nil {
				return fmt.Errorf("reload execution plan after runtime freeze race: %w", err)
			}
			if current.RuntimeSnapshotID == nil {
				return fmt.Errorf("execution plan %d runtime freeze lost its binding", plan.ID)
			}
			if err := tx.First(&snapshot, *current.RuntimeSnapshotID).Error; err != nil {
				return fmt.Errorf("load raced execution-plan runtime snapshot %d: %w", *current.RuntimeSnapshotID, err)
			}
		} else {
			snapshot = currentSnapshot
		}
		plan.RuntimeSnapshotID = &snapshot.ID
		plan.RuntimeProfile = snapshot.SelectedProfile
		return nil
	})
	if err != nil {
		return models.RuntimeSnapshot{}, err
	}
	return snapshot, nil
}

func runtimeSnapshotFromCurrentState(state models.RuntimeState) models.RuntimeSnapshot {
	return models.RuntimeSnapshot{
		DetectedAt: state.DetectedAt, OS: state.OS, Architecture: state.Architecture,
		Container: state.Container, CPUCores: state.CPUCores, CPULoad1: state.CPULoad1,
		GPUDetected: state.GPUDetected, GPUUsageAvailable: state.GPUUsageAvailable,
		GPUUsagePercent: state.GPUUsagePercent, GPUMediaUsagePercent: state.GPUMediaUsagePercent,
		GPURenderUsagePercent: state.GPURenderUsagePercent, GPUMetricSource: state.GPUMetricSource,
		TotalMemoryBytes: state.TotalMemoryBytes, AvailableMemoryBytes: state.AvailableMemoryBytes,
		BatteryPresent: state.BatteryPresent, BatteryPercent: state.BatteryPercent,
		PowerSource: state.PowerSource, OnBattery: state.OnBattery, Disks: state.Disks,
		Encoders: state.Encoders, RecommendedProfile: state.RecommendedProfile,
		SelectedProfile: state.SelectedProfile, PreferredProfile: state.PreferredProfile,
		FallbackProfile: state.FallbackProfile, AppliedOverrides: state.AppliedOverrides,
		EffectivePolicy: state.EffectivePolicy, SelectionReasons: state.SelectionReasons,
		Warnings: state.Warnings, StableFingerprint: state.StableFingerprint,
		FingerprintVersion: state.FingerprintVersion,
	}
}
