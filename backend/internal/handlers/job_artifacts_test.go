package handlers

import (
	"testing"

	"github.com/anuelvs/mediaforge/backend/internal/models"
)

func TestJobArtifactMatchesJobRequiresSameIDAndMediaPath(t *testing.T) {
	job := models.QueueJob{ID: 1, MediaPath: "/media/raw/series/current.mkv"}

	matching := map[string]any{
		"job": map[string]any{
			"id":        float64(1),
			"mediaPath": "/media/raw/series/current.mkv",
		},
	}
	if !jobArtifactMatchesJob(matching, job) {
		t.Fatalf("expected artifact to match current job")
	}

	wrongAsset := map[string]any{
		"job": map[string]any{
			"id":        float64(1),
			"mediaPath": "/media/raw/series/old-asset.mkv",
		},
	}
	if jobArtifactMatchesJob(wrongAsset, job) {
		t.Fatalf("expected artifact with old media path to be ignored")
	}

	wrongJob := map[string]any{
		"job": map[string]any{
			"id":        float64(2),
			"mediaPath": "/media/raw/series/current.mkv",
		},
	}
	if jobArtifactMatchesJob(wrongJob, job) {
		t.Fatalf("expected artifact with different job id to be ignored")
	}
}
