package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func writeColorPolicyValidationProbe(t *testing.T, sourceStream, outputStream string) (string, string) {
	t.Helper()
	tempDir := t.TempDir()
	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(tempDir, "source.mkv")
	outputPath := filepath.Join(tempDir, "output.mkv")
	for _, path := range []string{sourcePath, outputPath} {
		if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	probeScript := fmt.Sprintf(`#!/bin/sh
case "$*" in
  *output.mkv*) printf '%%s' '{"streams":[%s]}' ;;
  *) printf '%%s' '{"streams":[%s]}' ;;
esac
`, outputStream, sourceStream)
	if err := os.WriteFile(filepath.Join(binDir, "ffprobe"), []byte(probeScript), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return sourcePath, outputPath
}

func TestValidateJobColorPolicyUsesFrozenPreserveForIncompleteSource(t *testing.T) {
	sourcePath, outputPath := writeColorPolicyValidationProbe(t,
		`{"codec_type":"video","codec_name":"hevc","color_space":"unknown-matrix","color_transfer":"bt709","color_primaries":"bt709","color_range":"tv"}`,
		`{"codec_type":"video","codec_name":"hevc","color_space":"unknown-matrix","color_transfer":"bt709","color_primaries":"bt709","color_range":"tv"}`,
	)
	db := testEncodeTestDB(t, "validation-color-policy-frozen-preserve")
	report, warning := validateJobColorPolicy(db, models.QueueJob{
		MediaPath:  sourcePath,
		OutputPath: outputPath,
		ProfileSnapshot: models.JSONMap{"workerConfig": models.JSONMap{
			"finalColorPolicy":          "normalize_bt709",
			"effectiveFinalColorPolicy": "preserve",
		}},
	})

	if warning != "" {
		t.Fatalf("frozen preserve should validate without a warning: %s", warning)
	}
	if got := workerStringValue(report["effectivePolicy"]); got != "preserve" {
		t.Fatalf("effective policy=%q, want preserve: %#v", got, report)
	}
	expected := report["expected"].(models.JSONMap)
	if got := workerStringValue(expected["colorSpace"]); got == "bt709" {
		t.Fatalf("incomplete source must not create a BT.709 expectation: %#v", expected)
	}
	if got := workerStringValue(report["status"]); got != "passed" {
		t.Fatalf("preserved incomplete metadata did not validate: %#v", report)
	}
}

func TestValidateJobColorPolicyMissingRangeDoesNotExpectNormalization(t *testing.T) {
	sourcePath, outputPath := writeColorPolicyValidationProbe(t,
		`{"codec_type":"video","codec_name":"hevc","color_space":"bt470bg","color_transfer":"bt470m","color_primaries":"bt470m"}`,
		`{"codec_type":"video","codec_name":"hevc","color_space":"bt470bg","color_transfer":"bt470m","color_primaries":"bt470m"}`,
	)
	db := testEncodeTestDB(t, "validation-color-policy-missing-range")
	report, warning := validateJobColorPolicy(db, models.QueueJob{
		MediaPath:  sourcePath,
		OutputPath: outputPath,
		ProfileSnapshot: models.JSONMap{"workerConfig": models.JSONMap{
			"finalColorPolicy": "normalize_bt709",
		}},
	})

	if warning != "" {
		t.Fatalf("missing range should validate preserved metadata without a warning: %s", warning)
	}
	if got := workerStringValue(report["effectivePolicy"]); got != "preserve" {
		t.Fatalf("effective policy=%q, want preserve: %#v", got, report)
	}
	expected := report["expected"].(models.JSONMap)
	if got := workerStringValue(expected["colorSpace"]); got == "bt709" {
		t.Fatalf("missing range must not create a BT.709 expectation: %#v", expected)
	}
	if got := workerStringValue(expected["colorRange"]); got != "" {
		t.Fatalf("missing range should remain unknown in expected metadata, got %q", got)
	}
	if got := workerStringValue(report["status"]); got != "passed" {
		t.Fatalf("preserved metadata with missing range did not validate: %#v", report)
	}
}
