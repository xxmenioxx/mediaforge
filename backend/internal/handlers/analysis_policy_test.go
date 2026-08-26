package handlers

import (
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAnalysisPolicyPresetsKeepIndependentOperationalControls(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:analysis-policy-preset?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	value := models.JSONMap{"mode": "fast", "reuseSnapshots": false, "incrementalRefresh": false, "concurrentAssets": 4}
	if err := db.Create(&models.AppSetting{Key: analysisPolicySettingKey, Value: value}).Error; err != nil {
		t.Fatal(err)
	}

	policy := analysisPolicy(db)
	if policy.Mode != "fast" || policy.InitialWindows != 3 || policy.MaximumWindows != 3 || policy.EarlyConfidenceEnabled || policy.CropDepth != "reduced" {
		t.Fatalf("unexpected fast preset: %#v", policy)
	}
	if policy.ReuseSnapshots || policy.IncrementalRefresh || policy.ConcurrentAssets != 4 {
		t.Fatalf("operational controls were not preserved: %#v", policy)
	}
}

func TestCustomAnalysisPolicyFlowsIntoSamplingPlan(t *testing.T) {
	policy := AnalysisPolicy{Mode: "custom", AdaptiveAnalysis: false, EarlyConfidenceEnabled: false, EarlyConfidenceThreshold: 0.91, InitialWindows: 2, MaximumWindows: 4, WindowSeconds: 30, Positions: []float64{0.1, 0.35, 0.65, 0.9}, InterlaceValidation: "always", CropDepth: "full", ReuseSnapshots: true, IncrementalRefresh: true, ConcurrentAssets: 1}
	plan := canonicalSamplingPlan(1800, frameStructurePolicyFromAnalysis(policy))
	if plan.Adaptive || plan.EarlyConfidenceEnabled || plan.InitialWindows != 2 || len(plan.Positions) != 4 || plan.WindowSeconds != 30 {
		t.Fatalf("unexpected sampling plan: %#v", plan)
	}
	if plan.InterlaceValidation != "always" || plan.cropWindowMaximum() != 4 {
		t.Fatalf("deep analysis policy was not preserved: %#v", plan)
	}
}

func TestCanonicalPixelDepthSurvivesFrameEarlyStop(t *testing.T) {
	policy := balancedAnalysisPolicy()
	policy.Mode = "custom"
	policy.InterlaceValidation = "always"
	policy.CropDepth = "full"
	canonical := canonicalSamplingPlan(1800, frameStructurePolicyFromAnalysis(policy))
	frame := QSVFrameStructureAnalysis{Positions: []float64{0.08, 0.5, 0.92}, EarlyStopped: true}
	interlace, crop := analyzerPixelPlans(canonical, frame)
	if len(interlace.Positions) != 5 || len(crop.Positions) != 5 {
		t.Fatalf("canonical depth was reduced: interlace=%v crop=%v", interlace.Positions, crop.Positions)
	}

	policy.InterlaceValidation = "automatic"
	policy.CropDepth = "normal"
	canonical = canonicalSamplingPlan(1800, frameStructurePolicyFromAnalysis(policy))
	interlace, crop = analyzerPixelPlans(canonical, frame)
	if len(interlace.Positions) != 3 || len(crop.Positions) != 3 {
		t.Fatalf("automatic plans should follow observed adaptive depth: interlace=%v crop=%v", interlace.Positions, crop.Positions)
	}
}

func TestValidateAnalysisPolicyRejectsInvalidCustomWindowContract(t *testing.T) {
	err := validateAnalysisPolicy(models.JSONMap{"mode": "custom", "initialWindows": 4, "maximumWindows": 2, "windowSeconds": 20, "earlyConfidenceThreshold": 0.98, "positions": []any{0.1, 0.5}, "interlaceValidation": "automatic", "cropDepth": "normal", "concurrentAssets": 1})
	if err == nil {
		t.Fatal("expected invalid custom window contract to be rejected")
	}
}

func TestValidateAnalysisPolicyRejectsUnsafeCustomEvidenceSettings(t *testing.T) {
	base := models.JSONMap{"mode": "custom", "adaptiveAnalysis": true, "earlyConfidenceEnabled": true, "initialWindows": 3, "maximumWindows": 5, "windowSeconds": 20, "earlyConfidenceThreshold": 0.98, "positions": []any{0.08, 0.27, 0.5, 0.73, 0.92}, "interlaceValidation": "automatic", "cropDepth": "normal", "concurrentAssets": 1}
	cases := []struct {
		name  string
		patch models.JSONMap
	}{
		{name: "confidence below safe floor", patch: models.JSONMap{"earlyConfidenceThreshold": 0.89}},
		{name: "one initial window", patch: models.JSONMap{"initialWindows": 1}},
		{name: "duplicate positions", patch: models.JSONMap{"positions": []any{0.08, 0.27, 0.5, 0.5, 0.92}}},
		{name: "unsorted positions", patch: models.JSONMap{"positions": []any{0.08, 0.5, 0.27, 0.73, 0.92}}},
		{name: "out of range position", patch: models.JSONMap{"positions": []any{0.0, 0.27, 0.5, 0.73, 0.92}}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			value := models.JSONMap{}
			for key, item := range base {
				value[key] = item
			}
			for key, item := range testCase.patch {
				value[key] = item
			}
			if err := validateAnalysisPolicy(value); err == nil {
				t.Fatalf("expected invalid policy to be rejected: %#v", value)
			}
		})
	}
}
