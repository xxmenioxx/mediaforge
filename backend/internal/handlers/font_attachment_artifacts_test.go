package handlers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func fontValidationJob(t *testing.T, stagedPath string, status string) models.QueueJob {
	t.Helper()
	plan := ResolvedTrackPlan{
		FontAttachmentExportPolicy: FontAttachmentExportAll,
		FontAttachmentsExported:    true,
		FontAttachments: []ResolvedFontAttachment{{
			ArtifactID: "font-attachment:7", StreamIndex: 7, AttachmentOrdinal: 1, FontFormat: "TTF", SafeFilename: "Font.ttf",
		}},
	}
	planMap, err := resolvedTrackPlanMap(plan)
	if err != nil {
		t.Fatal(err)
	}
	return models.QueueJob{
		TrackProfileSnapshot: models.JSONMap{resolvedTrackPlanSnapshotKey: planMap},
		SubtitleArtifacts: fontAttachmentArtifactsJSON([]FontAttachmentArtifact{{
			ArtifactID: "font-attachment:7", Type: "font_attachment", StreamIndex: 7, AttachmentOrdinal: 1,
			FontFormat: "TTF", SafeFilename: "Font.ttf", StagedPath: stagedPath, Status: status,
		}}),
	}
}

func TestValidateRequiredFontAttachmentArtifacts(t *testing.T) {
	t.Run("ready nonempty font", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "Font.ttf")
		if err := os.WriteFile(path, []byte("font"), 0o644); err != nil {
			t.Fatal(err)
		}
		checks, _, report := validateRequiredFontAttachmentArtifacts(fontValidationJob(t, path, "ready"))
		if len(checks) != 1 || checks[0].Status != "passed" || intValueSetting(report["required"], 0) != 1 {
			t.Fatalf("checks=%#v report=%#v", checks, report)
		}
	})

	for _, test := range []struct {
		name    string
		prepare func(string) error
	}{
		{name: "missing planned font"},
		{name: "zero-byte font", prepare: func(path string) error { return os.WriteFile(path, nil, 0o644) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "Font.ttf")
			if test.prepare != nil {
				if err := test.prepare(path); err != nil {
					t.Fatal(err)
				}
			}
			checks, _, _ := validateRequiredFontAttachmentArtifacts(fontValidationJob(t, path, "ready"))
			if len(checks) != 1 || checks[0].Status != "failed" {
				t.Fatalf("checks=%#v", checks)
			}
		})
	}
}
