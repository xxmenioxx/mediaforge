package handlers

import (
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func TestFrameFidelityTreatsUnknownAsUnverified(t *testing.T) {
	result := frameFidelityValue(models.JSONMap{"sar": ""}, models.JSONMap{"sar": ""}, "sar", false)
	if result["status"] != "unverified" {
		t.Fatalf("status=%v", result["status"])
	}
}

func TestFrameFidelityAllowsIntentionalAspectChange(t *testing.T) {
	result := frameFidelityValue(models.JSONMap{"sar": "8:9"}, models.JSONMap{"sar": "23:27"}, "sar", true)
	if result["status"] != "changed_intentionally" {
		t.Fatalf("status=%v", result["status"])
	}
}

func TestHEVCLevelValidationChecksRecommendedOutputLevel(t *testing.T) {
	worker := map[string]interface{}{"hevcLevelMode": "recommended"}
	source := models.JSONMap{"width": 1920, "height": 1080, "frameRate": "24/1", "bitrate": "6418000"}

	validated := validateHEVCLevelField(worker, source, models.JSONMap{"codec": "hevc", "hevcLevel": "4.0"})
	if validated["status"] != "validated" || validated["requested"] != "4.0" {
		t.Fatalf("validated result=%#v", validated)
	}

	mismatch := validateHEVCLevelField(worker, source, models.JSONMap{"codec": "hevc", "hevcLevel": "5.0"})
	if mismatch["status"] != "mismatch" || mismatch["output"] != "5.0" {
		t.Fatalf("mismatch result=%#v", mismatch)
	}
}
