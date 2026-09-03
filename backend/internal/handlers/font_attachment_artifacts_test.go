package handlers

import (
	"os"
	"path/filepath"
	"strings"
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

	mediaPath := "/media/Movies/Movie.mkv"

	return models.QueueJob{
		MediaPath:            mediaPath,
		TrackProfileSnapshot: models.JSONMap{resolvedTrackPlanSnapshotKey: planMap},
		SubtitleArtifacts: fontAttachmentArtifactsJSON([]FontAttachmentArtifact{{
			ArtifactID:        "font-attachment:7",
			Type:              "font_attachment",
			StreamIndex:       7,
			AttachmentOrdinal: 1,
			FontFormat:        "TTF",
			SafeFilename:      "Font.ttf",
			RelativePath:      fontAttachmentRelativePath(mediaPath, "Font.ttf"),
			StagedPath:        stagedPath,
			Status:            status,
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

	for _, test := range []struct {
		name         string
		relativePath func(models.QueueJob) string
	}{
		{
			name: "empty relative path",
			relativePath: func(job models.QueueJob) string {
				return ""
			},
		},
		{
			name: "wrong fonts directory",
			relativePath: func(job models.QueueJob) string {
				return filepath.Join("Other.fonts", "Font.ttf")
			},
		},
		{
			name: "wrong relative filename",
			relativePath: func(job models.QueueJob) string {
				return fontAttachmentRelativePath(job.MediaPath, "Other.ttf")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "Font.ttf")
			if err := os.WriteFile(path, []byte("font"), 0o644); err != nil {
				t.Fatal(err)
			}

			job := fontValidationJob(t, path, "ready")

			artifacts := fontAttachmentArtifactsFromJSON(job.SubtitleArtifacts)
			if len(artifacts) != 1 {
				t.Fatalf("expected one font attachment artifact, got %d", len(artifacts))
			}

			artifacts[0].RelativePath = test.relativePath(job)
			job.SubtitleArtifacts = fontAttachmentArtifactsJSON(artifacts)

			checks, _, _ := validateRequiredFontAttachmentArtifacts(job)
			if len(checks) != 1 || checks[0].Status != "failed" {
				t.Fatalf("checks=%#v", checks)
			}
		})
	}
}

func subtitleFontWarningJob(
	t *testing.T,
	codec string,
	format string,
	mode string,
	disposition SubtitleDisposition,
	fontPolicy FontAttachmentExportPolicy,
	withSupportedFont bool,
) models.QueueJob {
	t.Helper()

	stagedPath := filepath.Join(t.TempDir(), "Movie."+format)

	content := []byte("[Events]\nDialogue: 0,0:00:00.00,0:00:01.00,Default,,0,0,0,,Hello")
	if format == "srt" {
		content = []byte("1\n00:00:00,000 --> 00:00:01,000\nHello\n")
	}

	if err := os.WriteFile(stagedPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	plan := ResolvedTrackPlan{
		SubtitleStreams: []ResolvedSubtitleTrack{{
			StreamIndex: 4,
			Codec:       codec,
			Language:    "eng",
			Action:      disposition,
		}},
		FontAttachmentExportPolicy: fontPolicy,
		SidecarOutputs: []ResolvedTrackSidecar{{
			StreamIndex: 4,
			Codec:       codec,
			Language:    "eng",
			Format:      format,
			Mode:        mode,
		}},
	}

	if withSupportedFont {
		plan.AttachmentStreams = []ResolvedAttachmentStream{{
			StreamIndex:    7,
			Filename:       "Font.ttf",
			AttachmentKind: "FONT",
			FontFormat:     "TTF",
		}}
	}

	planMap, err := resolvedTrackPlanMap(plan)
	if err != nil {
		t.Fatal(err)
	}

	return models.QueueJob{
		MediaPath:            "/media/Movies/Movie.mkv",
		TrackProfileSnapshot: models.JSONMap{resolvedTrackPlanSnapshotKey: planMap},
		SubtitleArtifacts: subtitleArtifactsJSON([]SubtitleArtifact{{
			ArtifactID:  subtitleArtifactID(4, mode, format),
			Type:        "subtitle_sidecar",
			StreamIndex: 4,
			SourceCodec: codec,
			Format:      format,
			Mode:        mode,
			Language:    "eng",
			StagedPath:  stagedPath,
			Status:      "ready",
		}}),
	}
}

func TestValidateRequiredSubtitleArtifactsFontWarnings(t *testing.T) {
	tests := []struct {
		name              string
		codec             string
		format            string
		mode              string
		disposition       SubtitleDisposition
		fontPolicy        FontAttachmentExportPolicy
		withSupportedFont bool
		wantWarning       bool
	}{
		{
			name:              "original ASS extract without font export warns",
			codec:             "ass",
			format:            "ass",
			mode:              "original",
			disposition:       SubtitleDispositionExtract,
			fontPolicy:        FontAttachmentExportNone,
			withSupportedFont: true,
			wantWarning:       true,
		},
		{
			name:              "original SSA extract without font export warns",
			codec:             "ssa",
			format:            "ssa",
			mode:              "original",
			disposition:       SubtitleDispositionExtract,
			fontPolicy:        FontAttachmentExportNone,
			withSupportedFont: true,
			wantWarning:       true,
		},
		{
			name:              "original ASS keep and extract without font export warns",
			codec:             "ass",
			format:            "ass",
			mode:              "original",
			disposition:       SubtitleDispositionKeepAndExtract,
			fontPolicy:        FontAttachmentExportNone,
			withSupportedFont: true,
			wantWarning:       true,
		},
		{
			name:              "original SSA keep and extract without font export warns",
			codec:             "ssa",
			format:            "ssa",
			mode:              "original",
			disposition:       SubtitleDispositionKeepAndExtract,
			fontPolicy:        FontAttachmentExportNone,
			withSupportedFont: true,
			wantWarning:       true,
		},
		{
			name:              "converted ASS to SRT does not warn",
			codec:             "ass",
			format:            "srt",
			mode:              "converted",
			disposition:       SubtitleDispositionExtract,
			fontPolicy:        FontAttachmentExportNone,
			withSupportedFont: true,
			wantWarning:       false,
		},
		{
			name:              "original ASS with font export enabled does not warn",
			codec:             "ass",
			format:            "ass",
			mode:              "original",
			disposition:       SubtitleDispositionExtract,
			fontPolicy:        FontAttachmentExportAll,
			withSupportedFont: true,
			wantWarning:       false,
		},
		{
			name:              "original ASS without supported fonts does not warn",
			codec:             "ass",
			format:            "ass",
			mode:              "original",
			disposition:       SubtitleDispositionExtract,
			fontPolicy:        FontAttachmentExportNone,
			withSupportedFont: false,
			wantWarning:       false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			job := subtitleFontWarningJob(
				t,
				test.codec,
				test.format,
				test.mode,
				test.disposition,
				test.fontPolicy,
				test.withSupportedFont,
			)

			checks, warnings, _ := validateRequiredSubtitleArtifacts(job)

			if len(checks) != 1 || checks[0].Status != "passed" {
				t.Fatalf("checks=%#v", checks)
			}

			hasFontWarning := false
			for _, warning := range warnings {
				if strings.Contains(warning, "may reference custom fonts") {
					hasFontWarning = true
					break
				}
			}

			if hasFontWarning != test.wantWarning {
				t.Fatalf(
					"warnings=%#v wantWarning=%v",
					warnings,
					test.wantWarning,
				)
			}
		})
	}
}
