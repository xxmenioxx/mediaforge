package handlers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func TestValidateRequiredSubtitleArtifacts(t *testing.T) {
	tests := []struct {
		name       string
		write      bool
		content    []byte
		language   string
		wantStatus string
		wantWarn   bool
	}{
		{"ready", true, []byte("[Events]\nDialogue: 0,0:00:00.00,0:00:01.00,Default,,0,0,0,,Hola\n"), "spa", "passed", false},
		{"missing", false, nil, "spa", "failed", false},
		{"empty", true, []byte{}, "spa", "failed", false},
		{"metadata unavailable", true, []byte("[Events]\nDialogue: 0,0:00:00.00,0:00:01.00,Default,,0,0,0,,Hola\n"), "", "passed", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "Movie.spa.ass")
			if test.write {
				if err := os.WriteFile(path, test.content, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			job := sidecarValidationJob(t, path, test.language)
			checks, warnings, report := validateRequiredSubtitleArtifacts(job)
			if len(checks) != 1 || checks[0].Status != test.wantStatus {
				t.Fatalf("checks=%#v want status=%s", checks, test.wantStatus)
			}
			if (len(warnings) > 0) != test.wantWarn {
				t.Fatalf("warnings=%#v wantWarn=%t", warnings, test.wantWarn)
			}
			if intValueSetting(report["required"], 0) != 1 {
				t.Fatalf("validation report lost expected artifacts: %#v", report)
			}
		})
	}
}

func sidecarValidationJob(t *testing.T, path, artifactLanguage string) models.QueueJob {
	t.Helper()
	plan := ResolvedTrackPlan{
		SubtitleStreams: []ResolvedSubtitleTrack{{StreamIndex: 4, Codec: "ass", Language: "spa", Action: SubtitleDispositionKeepAndExtract}},
		SidecarOutputs:  []ResolvedTrackSidecar{{StreamIndex: 4, Codec: "ass", Language: "spa", Format: "ass"}},
	}
	planMap, err := resolvedTrackPlanMap(plan)
	if err != nil {
		t.Fatal(err)
	}
	return models.QueueJob{
		TrackProfileSnapshot: models.JSONMap{resolvedTrackPlanSnapshotKey: planMap},
		SubtitleArtifacts: subtitleArtifactsJSON([]SubtitleArtifact{{
			StreamIndex: 4, SourceCodec: "ass", Format: "ass", Language: artifactLanguage,
			StagedPath: path, Status: "ready",
		}}),
	}
}
