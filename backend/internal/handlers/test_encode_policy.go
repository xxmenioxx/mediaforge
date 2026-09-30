package handlers

import (
	"fmt"
	"math"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
)

const testEncodePolicySettingKey = "testEncodePolicy"

func validateTestEncodePolicyValue(value models.JSONMap) error {
	if raw, exists := value["defaultDurationSeconds"]; exists {
		seconds, ok := policyInteger(raw)
		if !ok || seconds < 5 || seconds > 120 {
			return fmt.Errorf("defaultDurationSeconds must be an integer from 5 to 120")
		}
	}
	if raw, exists := value["defaultStartMode"]; exists {
		mode, ok := raw.(string)
		if !ok || !validTestEncodeStartMode(mode) {
			return fmt.Errorf("defaultStartMode must be beginning, middle, or representative")
		}
	}
	if raw, exists := value["defaultRetention"]; exists {
		retention, ok := raw.(string)
		if !ok || (strings.ToLower(strings.TrimSpace(retention)) != "temporary" && strings.ToLower(strings.TrimSpace(retention)) != "keep") {
			return fmt.Errorf("defaultRetention must be temporary or keep")
		}
	}
	for _, field := range []struct {
		key string
		max int
	}{
		{key: "temporaryRetentionHours", max: 24 * 365},
		{key: "failedRetentionHours", max: 24 * 365},
		{key: "maxReconcileBatchSize", max: 250},
		{key: "maxCleanupBatchSize", max: 100},
	} {
		if raw, exists := value[field.key]; exists {
			count, ok := policyInteger(raw)
			if !ok || count < 1 || count > field.max {
				return fmt.Errorf("%s must be an integer from 1 to %d", field.key, field.max)
			}
		}
	}
	return nil
}

func normalizeTestEncodePolicyValue(value models.JSONMap) (models.JSONMap, error) {
	if value == nil {
		value = models.JSONMap{}
	}
	if err := validateTestEncodePolicyValue(value); err != nil {
		return nil, err
	}
	policy := models.DefaultTestEncodePolicy()
	if raw, exists := value["defaultDurationSeconds"]; exists {
		policy.DefaultDurationSeconds, _ = policyInteger(raw)
	}
	if raw, exists := value["defaultStartMode"]; exists {
		policy.DefaultStartMode = strings.ToLower(strings.TrimSpace(raw.(string)))
	}
	if raw, exists := value["defaultRetention"]; exists {
		policy.DefaultRetention = strings.ToLower(strings.TrimSpace(raw.(string)))
	}
	if raw, exists := value["temporaryRetentionHours"]; exists {
		policy.TemporaryRetentionHours, _ = policyInteger(raw)
	} else if raw, exists := value["retentionHours"]; exists {
		policy.TemporaryRetentionHours, _ = policyInteger(raw)
	}
	if raw, exists := value["failedRetentionHours"]; exists {
		policy.FailedRetentionHours, _ = policyInteger(raw)
	}
	if raw, exists := value["maxReconcileBatchSize"]; exists {
		policy.MaxReconcileBatchSize, _ = policyInteger(raw)
	}
	if raw, exists := value["maxCleanupBatchSize"]; exists {
		policy.MaxCleanupBatchSize, _ = policyInteger(raw)
	}
	return policy.JSONMap(), nil
}

func testEncodePolicyFromSettings(db *gorm.DB) models.TestEncodePolicy {
	policy := models.DefaultTestEncodePolicy()
	if db == nil {
		return policy
	}
	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", testEncodePolicySettingKey).Error; err == nil {
		value := setting.Value
		if _, exists := value["temporaryRetentionHours"]; !exists {
			if legacy, found := value["retentionHours"]; found {
				value["temporaryRetentionHours"] = legacy
			}
		}
		if normalized, err := normalizeTestEncodePolicyValue(value); err == nil {
			return testEncodePolicyFromMap(normalized)
		}
		return policy
	}
	var housekeeping models.AppSetting
	if err := db.First(&housekeeping, "key = ?", "housekeeping").Error; err == nil {
		legacy := workerNumberValue(housekeeping.Value["testEncodeRetentionHours"], float64(policy.TemporaryRetentionHours))
		policy.TemporaryRetentionHours = min(24*365, max(1, int(legacy)))
	}
	return policy
}

func testEncodePolicyFromMap(value models.JSONMap) models.TestEncodePolicy {
	policy := models.DefaultTestEncodePolicy()
	if seconds, ok := policyInteger(value["defaultDurationSeconds"]); ok {
		policy.DefaultDurationSeconds = seconds
	}
	if mode, ok := value["defaultStartMode"].(string); ok {
		policy.DefaultStartMode = mode
	}
	if retention, ok := value["defaultRetention"].(string); ok {
		policy.DefaultRetention = retention
	}
	if hours, ok := policyInteger(value["temporaryRetentionHours"]); ok {
		policy.TemporaryRetentionHours = hours
	}
	if hours, ok := policyInteger(value["failedRetentionHours"]); ok {
		policy.FailedRetentionHours = hours
	}
	if count, ok := policyInteger(value["maxReconcileBatchSize"]); ok {
		policy.MaxReconcileBatchSize = count
	}
	if count, ok := policyInteger(value["maxCleanupBatchSize"]); ok {
		policy.MaxCleanupBatchSize = count
	}
	return policy
}

func policyInteger(value any) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case int64:
		return int(number), true
	case float64:
		if math.IsNaN(number) || math.IsInf(number, 0) || math.Trunc(number) != number {
			return 0, false
		}
		return int(number), true
	case float32:
		converted := float64(number)
		if math.IsNaN(converted) || math.IsInf(converted, 0) || math.Trunc(converted) != converted {
			return 0, false
		}
		return int(converted), true
	default:
		return 0, false
	}
}

func validTestEncodeStartMode(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "beginning", "middle", "representative":
		return true
	default:
		return false
	}
}
