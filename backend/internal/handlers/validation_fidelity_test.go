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
	worker := map[string]interface{}{"hevcLevelMode": "recommended", "hevcLevel": "4.0"}
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

func TestHEVCLevelValidationChecksAutoOutputAgainstAssetRecommendation(t *testing.T) {
	worker := map[string]interface{}{"hevcLevelMode": "auto", "videoEncoder": "hevc_qsv"}
	source := models.JSONMap{"width": 1920, "height": 1080, "frameRate": "24/1", "bitrate": "6418000"}
	validated := validateHEVCLevelField(worker, source, models.JSONMap{"codec": "hevc", "hevcLevel": "4.0"})
	if validated["status"] != "validated" || validated["requested"] != "4.0" {
		t.Fatalf("auto Level validation=%#v", validated)
	}
}

func TestHEVCLevelValidationChecksObservedMainTierBitrate(t *testing.T) {
	worker := map[string]interface{}{"hevcLevelMode": "custom", "hevcLevel": "4.0", "hevcLevelTier": "main", "videoEncoder": "hevc_qsv"}
	source := models.JSONMap{"width": 1920, "height": 1080, "frameRate": "24/1"}
	output := models.JSONMap{"codec": "hevc", "hevcLevel": "4.0", "bitrate": 13_000_000}
	validated := validateHEVCLevelField(worker, source, output)
	if validated["status"] != "mismatch" || validated["bitrateStatus"] != "exceeds_main_tier" || validated["effectiveTier"] != "main" {
		t.Fatalf("Main Tier bitrate validation=%#v", validated)
	}
}

func TestHEVCLevelValidationRejectsCustomLevelBelowOutputGeometry(t *testing.T) {
	worker := map[string]interface{}{"hevcLevelMode": "custom", "hevcLevel": "4.0", "videoEncoder": "hevc_qsv"}
	output := models.JSONMap{"codec": "hevc", "hevcLevel": "4.0", "width": 3840, "height": 2160, "frameRate": "24/1"}
	validated := validateHEVCLevelField(worker, models.JSONMap{}, output)
	if validated["status"] != "mismatch" || validated["requirementsStatus"] != "exceeds_level_limits" {
		t.Fatalf("Custom Level geometry validation=%#v", validated)
	}
}
