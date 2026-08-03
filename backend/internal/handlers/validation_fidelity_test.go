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
