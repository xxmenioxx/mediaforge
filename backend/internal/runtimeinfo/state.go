package runtimeinfo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
)

const RuntimeStableFingerprintVersion = "v1"

// DetectAndUpsert is the only routine detection write boundary. It updates
// the singleton live state and returns a transition audit ID only when the
// stable execution authority changed.
func DetectAndUpsert(db *gorm.DB) (models.RuntimeState, *uint, error) {
	detected, err := Detect(db)
	if err != nil {
		return models.RuntimeState{}, nil, err
	}
	var state models.RuntimeState
	var transitionID *uint
	err = db.Transaction(func(tx *gorm.DB) error {
		var writeErr error
		state, transitionID, writeErr = UpsertDetectedState(tx, detected)
		return writeErr
	})
	if err != nil {
		return models.RuntimeState{}, nil, err
	}
	return state, transitionID, nil
}

// Current reads the singleton mutable state. It never creates audit history.
func Current(db *gorm.DB) (models.RuntimeState, error) {
	var state models.RuntimeState
	err := db.Where("singleton_key = ?", models.RuntimeStateSingletonKey).First(&state).Error
	return state, err
}

// LatestAuditSnapshot reads immutable history explicitly. It is not a live
// state fallback and callers must use it only for audit/provenance views.
func LatestAuditSnapshot(db *gorm.DB) (models.RuntimeSnapshot, error) {
	var snapshot models.RuntimeSnapshot
	err := db.Order("detected_at desc").Order("id desc").First(&snapshot).Error
	return snapshot, err
}

// CaptureAuditSnapshot creates one immutable evidence row from the supplied
// state. The caller owns the transaction when the row must be bound to another
// provenance record atomically.
func CaptureAuditSnapshot(tx *gorm.DB, state models.RuntimeState, reason string) (models.RuntimeSnapshot, error) {
	if strings.TrimSpace(reason) == "" {
		return models.RuntimeSnapshot{}, fmt.Errorf("runtime audit capture reason is required")
	}
	if state.StableFingerprint == "" {
		state.StableFingerprint = RuntimeStableFingerprint(state)
	}
	if state.FingerprintVersion == "" {
		state.FingerprintVersion = RuntimeStableFingerprintVersion
	}
	snapshot := runtimeSnapshotFromState(state)
	snapshot.ID = 0
	snapshot.CaptureReason = reason
	if snapshot.DetectedAt.IsZero() {
		snapshot.DetectedAt = time.Now()
	}
	if err := tx.Create(&snapshot).Error; err != nil {
		return models.RuntimeSnapshot{}, err
	}
	return snapshot, nil
}

// UpsertDetectedState applies one detection and handles the stable transition
// capture in the same transaction as the singleton update.
func UpsertDetectedState(tx *gorm.DB, detected models.RuntimeState) (models.RuntimeState, *uint, error) {
	detected.SingletonKey = models.RuntimeStateSingletonKey
	if detected.StableFingerprint == "" {
		detected.StableFingerprint = RuntimeStableFingerprint(detected)
	}
	if detected.FingerprintVersion == "" {
		detected.FingerprintVersion = RuntimeStableFingerprintVersion
	}
	if detected.DetectedAt.IsZero() {
		detected.DetectedAt = time.Now()
	}

	var current models.RuntimeState
	result := tx.Where("singleton_key = ?", models.RuntimeStateSingletonKey).First(&current)
	if result.Error == gorm.ErrRecordNotFound {
		detected.ID = 0
		detected.CreatedAt = time.Now()
		detected.UpdatedAt = detected.CreatedAt
		if err := tx.Create(&detected).Error; err != nil {
			return models.RuntimeState{}, nil, err
		}
		return detected, nil, nil
	}
	if result.Error != nil {
		return models.RuntimeState{}, nil, result.Error
	}

	var transitionID *uint
	if current.StableFingerprint != "" && current.StableFingerprint != detected.StableFingerprint {
		snapshot, err := CaptureAuditSnapshot(tx, detected, "stable_transition")
		if err != nil {
			return models.RuntimeState{}, nil, err
		}
		transitionID = &snapshot.ID
	}
	detected.ID = current.ID
	detected.SingletonKey = current.SingletonKey
	detected.CreatedAt = current.CreatedAt
	detected.UpdatedAt = time.Now()
	if err := tx.Save(&detected).Error; err != nil {
		return models.RuntimeState{}, nil, err
	}
	return detected, transitionID, nil
}

// InitializeLegacyState seeds the singleton from immutable history without
// manufacturing an audit event or deleting any legacy rows.
func InitializeLegacyState(db *gorm.DB) error {
	var current models.RuntimeState
	if err := db.Where("singleton_key = ?", models.RuntimeStateSingletonKey).First(&current).Error; err == nil {
		return nil
	} else if err != gorm.ErrRecordNotFound {
		return err
	}

	var legacy models.RuntimeSnapshot
	if err := db.Order("detected_at desc").Order("id desc").First(&legacy).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		return err
	}
	state := runtimeStateFromSnapshot(legacy)
	state.ID = 0
	state.SingletonKey = models.RuntimeStateSingletonKey
	state.StableFingerprint = RuntimeStableFingerprint(state)
	state.FingerprintVersion = RuntimeStableFingerprintVersion
	state.CreatedAt = legacy.CreatedAt
	if state.CreatedAt.IsZero() {
		state.CreatedAt = legacy.DetectedAt
	}
	state.UpdatedAt = time.Now()
	return db.Create(&state).Error
}

// RuntimeStableFingerprint returns the versioned identity of stable execution
// authority. It intentionally does not hash live telemetry or diagnostics.
func RuntimeStableFingerprint(state models.RuntimeState) string {
	payload := map[string]any{
		"schemaVersion":    RuntimeStableFingerprintVersion,
		"os":               strings.ToLower(strings.TrimSpace(state.OS)),
		"architecture":     strings.ToLower(strings.TrimSpace(state.Architecture)),
		"container":        state.Container,
		"cpuCores":         state.CPUCores,
		"totalMemory":      state.TotalMemoryBytes,
		"gpuDetected":      state.GPUDetected,
		"encoders":         stableCapabilityMap(state.Encoders),
		"profiles":         stableProfiles(state),
		"effectivePolicy":  stableValue(state.EffectivePolicy),
		"appliedOverrides": sortedStrings(state.AppliedOverrides),
		"disks":            stableDisks(state.Disks),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		// All inputs are JSON-compatible model values. Keep the API total if a
		// future detector adds an unsupported value.
		encoded = []byte(`{"schemaVersion":"` + RuntimeStableFingerprintVersion + `"}`)
	}
	digest := sha256.Sum256(encoded)
	return "runtime-stable-" + RuntimeStableFingerprintVersion + ":" + hex.EncodeToString(digest[:])
}

func stableProfiles(state models.RuntimeState) map[string]string {
	return map[string]string{
		"recommended": strings.TrimSpace(state.RecommendedProfile),
		"selected":    strings.TrimSpace(state.SelectedProfile),
		"preferred":   strings.TrimSpace(state.PreferredProfile),
		"fallback":    strings.TrimSpace(state.FallbackProfile),
	}
}

func stableDisks(disks models.JSONMap) map[string]any {
	result := map[string]any{}
	for role, raw := range disks {
		entry, ok := jsonObject(raw)
		if !ok {
			continue
		}
		path, _ := entry["path"].(string)
		storageType, _ := entry["type"].(string)
		cleanPath := ""
		if strings.TrimSpace(path) != "" {
			cleanPath = filepath.ToSlash(filepath.Clean(path))
		}
		result[role] = map[string]string{
			"path": filepath.ToSlash(cleanPath),
			"type": strings.ToLower(strings.TrimSpace(storageType)),
		}
	}
	return result
}

func stableCapabilityMap(values models.JSONMap) map[string]any {
	result := map[string]any{}
	for name, raw := range values {
		result[name] = stableCapabilityValue(raw)
	}
	return result
}

func stableCapabilityValue(value any) any {
	switch typed := value.(type) {
	case models.JSONMap:
		result := map[string]any{}
		for key, child := range typed {
			if transientCapabilityKey(key) {
				continue
			}
			result[key] = stableCapabilityValue(child)
		}
		return result
	case map[string]any:
		result := map[string]any{}
		for key, child := range typed {
			if transientCapabilityKey(key) {
				continue
			}
			result[key] = stableCapabilityValue(child)
		}
		return result
	case map[string]string:
		result := map[string]any{}
		for key, child := range typed {
			if transientCapabilityKey(key) {
				continue
			}
			result[key] = child
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, child := range typed {
			result[index] = stableCapabilityValue(child)
		}
		return result
	case models.JSONList:
		result := make([]any, len(typed))
		for index, child := range typed {
			result[index] = stableCapabilityValue(child)
		}
		return result
	default:
		return value
	}
}

func transientCapabilityKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	return normalized == "reason" || normalized == "modereasons" ||
		strings.Contains(normalized, "reason") || strings.Contains(normalized, "diagnostic") ||
		strings.Contains(normalized, "warning") || strings.Contains(normalized, "error") ||
		strings.Contains(normalized, "message")
}

func stableValue(value any) any {
	switch typed := value.(type) {
	case models.JSONMap:
		result := map[string]any{}
		for key, child := range typed {
			result[key] = stableValue(child)
		}
		return result
	case map[string]any:
		result := map[string]any{}
		for key, child := range typed {
			result[key] = stableValue(child)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, child := range typed {
			result[index] = stableValue(child)
		}
		return result
	case models.JSONList:
		result := make([]any, len(typed))
		for index, child := range typed {
			result[index] = stableValue(child)
		}
		return result
	default:
		return value
	}
}

func sortedStrings(values models.JSONList) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			result = append(result, text)
		}
	}
	sort.Strings(result)
	return result
}

func jsonObject(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case models.JSONMap:
		return map[string]any(typed), true
	case map[string]any:
		return typed, true
	default:
		return nil, false
	}
}

func runtimeStateFromSnapshot(snapshot models.RuntimeSnapshot) models.RuntimeState {
	return models.RuntimeState{
		DetectedAt: snapshot.DetectedAt, OS: snapshot.OS, Architecture: snapshot.Architecture,
		Container: snapshot.Container, CPUCores: snapshot.CPUCores, CPULoad1: snapshot.CPULoad1,
		GPUDetected: snapshot.GPUDetected, GPUUsageAvailable: snapshot.GPUUsageAvailable,
		GPUUsagePercent: snapshot.GPUUsagePercent, GPUMediaUsagePercent: snapshot.GPUMediaUsagePercent,
		GPURenderUsagePercent: snapshot.GPURenderUsagePercent, GPUMetricSource: snapshot.GPUMetricSource,
		TotalMemoryBytes: snapshot.TotalMemoryBytes, AvailableMemoryBytes: snapshot.AvailableMemoryBytes,
		BatteryPresent: snapshot.BatteryPresent, BatteryPercent: snapshot.BatteryPercent,
		PowerSource: snapshot.PowerSource, OnBattery: snapshot.OnBattery, Disks: snapshot.Disks,
		Encoders: snapshot.Encoders, RecommendedProfile: snapshot.RecommendedProfile,
		SelectedProfile: snapshot.SelectedProfile, PreferredProfile: snapshot.PreferredProfile,
		FallbackProfile: snapshot.FallbackProfile, AppliedOverrides: snapshot.AppliedOverrides,
		EffectivePolicy: snapshot.EffectivePolicy, SelectionReasons: snapshot.SelectionReasons,
		Warnings: snapshot.Warnings,
	}
}

func runtimeSnapshotFromState(state models.RuntimeState) models.RuntimeSnapshot {
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
