package handlers

import (
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
)

func TestTestEncodeArtifactSetTracksStagedAndActivatedSidecars(t *testing.T) {
	db := testEncodeTestDB(t, "test-encode-artifact-set", &models.TestEncodeArtifact{})
	test := models.TestEncode{LibraryID: 3, SourcePath: "/raw/a.mkv", Status: testEncodeRunning, Phase: "generating", CreatedAt: time.Now()}
	if err := db.Create(&test).Error; err != nil {
		t.Fatal(err)
	}
	output := "/library/MVForge Test T1.mkv"
	temporary := "/library/.MVForge Test T1.partial.mkv"
	staged := "/library/.MVForge Test T1.partial.eng.srt"
	sidecars := []SubtitleArtifact{{StagedPath: staged}}

	if err := persistTestEncodeArtifactSet(db, test.ID, testEncodeArtifactSet(test, output, temporary, sidecars, false)); err != nil {
		t.Fatal(err)
	}
	var artifacts []models.TestEncodeArtifact
	if err := db.Where("test_encode_id = ?", test.ID).Find(&artifacts).Error; err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 4 {
		t.Fatalf("staged ownership count = %d, want output, temporary, staged subtitle, and final subtitle", len(artifacts))
	}
	expected := map[string]bool{
		output: true, temporary: false, staged: false,
		"/library/MVForge Test T1.eng.srt": true,
	}
	for _, artifact := range artifacts {
		wantExpected, exists := expected[artifact.Path]
		if !exists || artifact.Expected != wantExpected || artifact.LibraryID != test.LibraryID {
			t.Fatalf("unexpected staged ownership: %#v", artifact)
		}
	}

	sidecars[0].StagedPath = "/library/MVForge Test T1.eng.srt"
	if err := persistTestEncodeArtifactSet(db, test.ID, testEncodeArtifactSet(test, output, "", sidecars, true)); err != nil {
		t.Fatal(err)
	}
	artifacts = nil
	if err := db.Where("test_encode_id = ?", test.ID).Find(&artifacts).Error; err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("activated ownership count = %d, want output and final subtitle", len(artifacts))
	}
	for _, artifact := range artifacts {
		if !artifact.Expected {
			t.Fatalf("activated artifact is not expected: %#v", artifact)
		}
	}
}
