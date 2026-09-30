package models

// TestEncodePolicy contains only Test Encode sampling and lifecycle defaults.
// Profile and encoder settings remain owned by the existing resolvers.
type TestEncodePolicy struct {
	DefaultDurationSeconds  int    `json:"defaultDurationSeconds"`
	DefaultStartMode        string `json:"defaultStartMode"`
	DefaultRetention        string `json:"defaultRetention"`
	TemporaryRetentionHours int    `json:"temporaryRetentionHours"`
	FailedRetentionHours    int    `json:"failedRetentionHours"`
	MaxReconcileBatchSize   int    `json:"maxReconcileBatchSize"`
	MaxCleanupBatchSize     int    `json:"maxCleanupBatchSize"`
}

func DefaultTestEncodePolicy() TestEncodePolicy {
	return TestEncodePolicy{
		DefaultDurationSeconds:  20,
		DefaultStartMode:        "representative",
		DefaultRetention:        "temporary",
		TemporaryRetentionHours: 24,
		FailedRetentionHours:    6,
		MaxReconcileBatchSize:   100,
		MaxCleanupBatchSize:     25,
	}
}

func (policy TestEncodePolicy) JSONMap() JSONMap {
	return JSONMap{
		"defaultDurationSeconds":  policy.DefaultDurationSeconds,
		"defaultStartMode":        policy.DefaultStartMode,
		"defaultRetention":        policy.DefaultRetention,
		"temporaryRetentionHours": policy.TemporaryRetentionHours,
		"failedRetentionHours":    policy.FailedRetentionHours,
		"maxReconcileBatchSize":   policy.MaxReconcileBatchSize,
		"maxCleanupBatchSize":     policy.MaxCleanupBatchSize,
	}
}
