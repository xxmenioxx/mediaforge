package handlers

import (
	"context"
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTranslationPreparationReusesDictionaryMatcherAndProvenance(t *testing.T) {
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", t.TempDir())
	db, err := gorm.Open(sqlite.Open("file:ocr-dictionary-translation?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.OCRDictionaryEntry{}); err != nil {
		t.Fatal(err)
	}
	rule := testOCRDictionaryEntry("builtin:eng:im", "|'m", "I'm", 100)
	rule.Source = "builtin"
	if err := db.Create(&rule).Error; err != nil {
		t.Fatal(err)
	}
	evidence := &canonicalOCREvidence{
		Identity:          canonicalOCRIdentity{SourcePath: "/media/source.mkv", StreamIndex: 3, RequestedLanguage: "eng"},
		RequestedLanguage: "eng", EffectiveLanguage: "eng", DictionaryLanguage: "eng", DictionaryLanguageSource: "request",
		Complete: true, Integrity: "canonical-cues-validated-v1",
		Cues:     []canonicalBitmapSubtitleCue{{Start: 1.25, End: 2.5, Text: "Well, |'m here.", SourceOrdinal: 17}},
		Geometry: []bitmapCueGeometry{{SourceOrdinal: 17, BitmapX: 10, BitmapY: 20, BitmapWidth: 100, BitmapHeight: 40}},
	}
	prepared, err := prepareCanonicalOCREvidenceForTranslation(context.Background(), evidence, nil, canonicalOCRDictionaryPreparation{DB: db, OperationID: "translation-operation"})
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Cues) != 1 || prepared.Cues[0].Text != "Well, I'm here." {
		t.Fatalf("translation did not use the canonical contextual correction: %#v", prepared.Cues)
	}
	if prepared.Cues[0].SourceOrdinal != 17 || prepared.Cues[0].Start != 1.25 || prepared.Cues[0].End != 2.5 || len(prepared.Geometry) != 1 || prepared.Geometry[0].BitmapX != 10 {
		t.Fatalf("dictionary correction changed source identity, timing, or geometry: %#v / %#v", prepared.Cues[0], prepared.Geometry)
	}
	if len(prepared.CorrectionProvenance) != 1 {
		t.Fatalf("expected one applied-rule provenance record, got %#v", prepared.CorrectionProvenance)
	}
	provenance := prepared.CorrectionProvenance[0]
	if provenance.CueID != 17 || provenance.OperationID != "translation-operation" || provenance.DictionaryLanguage != "eng" || provenance.DictionaryRevision == "" || len(provenance.DictionaryRuleIDs) != 1 || provenance.DictionaryRuleIDs[0] != rule.ID || provenance.BeforeTextHash == "" || provenance.AfterTextHash == "" || provenance.BeforeTextHash == provenance.AfterTextHash {
		t.Fatalf("translation provenance is incomplete: %#v", provenance)
	}
	if len(evidence.Cues) != 1 || evidence.Cues[0].Text != "Well, |'m here." {
		t.Fatalf("translation mutated the source canonical evidence: %#v", evidence.Cues)
	}
}

func TestTranslationPreparationDoesNotAutoApplyUserDictionaryRule(t *testing.T) {
	t.Setenv("MVFORGE_OCR_LIFECYCLE_ROOT", t.TempDir())
	db, err := gorm.Open(sqlite.Open("file:ocr-dictionary-translation-user-rule?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.OCRDictionaryEntry{}); err != nil {
		t.Fatal(err)
	}
	rule := testOCRDictionaryEntry("user:eng:im", "|'m", "I'm", 100)
	if err := db.Create(&rule).Error; err != nil {
		t.Fatal(err)
	}
	if candidate, ok, _, _, _ := deterministicOCRSuggestionWithDictionary("Well, |'m here.", "eng", "eng", []models.OCRDictionaryEntry{rule}); !ok || candidate != "Well, I'm here." {
		t.Fatalf("custom rule should remain a validated OCR Review candidate, got %q, %v", candidate, ok)
	}
	evidence := &canonicalOCREvidence{
		Identity:          canonicalOCRIdentity{SourcePath: "/media/source.mkv", StreamIndex: 3, StreamCodec: "subrip", RequestedLanguage: "eng"},
		RequestedLanguage: "eng", EffectiveLanguage: "eng", DictionaryLanguage: "eng", DictionaryLanguageSource: "request",
		Complete: true, Integrity: "canonical-cues-validated-v1",
		Cues: []canonicalBitmapSubtitleCue{{Start: 1.25, End: 2.5, Text: "Well, |'m here.", SourceOrdinal: 17}},
	}
	prepared, err := prepareCanonicalOCREvidenceForTranslation(context.Background(), evidence, nil, canonicalOCRDictionaryPreparation{DB: db, OperationID: "translation-custom-rule"})
	if err == nil || prepared != nil || !strings.Contains(err.Error(), "requires review before translation") {
		t.Fatalf("Translation must keep user-rule suggestions behind the canonical review gate, prepared=%#v err=%v", prepared, err)
	}
}
