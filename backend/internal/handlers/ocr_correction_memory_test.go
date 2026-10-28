package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newOCRCorrectionMemoryTestDB(t *testing.T, extra ...any) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:ocr-correction-memory-%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "-"))), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	modelsToMigrate := []any{&models.AppSetting{}, &models.OCRCorrectionMemoryEntry{}, &models.OCRCorrectionMemoryEvidence{}, &models.AssetRecord{}, &models.Library{}}
	modelsToMigrate = append(modelsToMigrate, extra...)
	if err := db.AutoMigrate(modelsToMigrate...); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestOCRCorrectionMemoryDefaultsToOptOut(t *testing.T) {
	db := newOCRCorrectionMemoryTestDB(t)
	settings, err := loadOCRCorrectionMemorySettings(db)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Enabled || settings.IncludeInAIPrompts || settings.RetentionDays != ocrCorrectionMemoryDefaultRetentionDays {
		t.Fatalf("default settings = %#v, want disabled with %d-day retention", settings, ocrCorrectionMemoryDefaultRetentionDays)
	}

	if err := addApprovedOCRCorrectionMemory(db, canonicalOCRIdentity{RequestedLanguage: "eng"}, strings.Repeat("a", 64), canonicalBitmapSubtitleCue{SourceOrdinal: 1, Text: "h0use"}, []canonicalBitmapSubtitleCue{{SourceOrdinal: 1, Text: "h0use"}}, "house", "edit", canonicalOCRCorrectionProvenance{}); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&models.OCRCorrectionMemoryEntry{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("captured %d entries while learning was disabled", count)
	}
}

func TestMigrateOCRCorrectionMemorySettingsAddsDisabledDefaultIdempotently(t *testing.T) {
	db := newOCRCorrectionMemoryTestDB(t)
	if err := migrateOCRCorrectionMemorySettings(db); err != nil {
		t.Fatal(err)
	}
	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", ocrCorrectionMemorySettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if setting.Value["enabled"] != false || setting.Value["includeInAIPrompts"] != false || setting.Value["retentionDays"] != float64(ocrCorrectionMemoryDefaultRetentionDays) && setting.Value["retentionDays"] != ocrCorrectionMemoryDefaultRetentionDays {
		t.Fatalf("default memory settings = %#v", setting.Value)
	}
	if err := db.Model(&models.AppSetting{}).Where("key = ?", ocrCorrectionMemorySettingsKey).Update("value", models.JSONMap{"enabled": true, "retentionDays": 90, "includeInAIPrompts": true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateOCRCorrectionMemorySettings(db); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&setting, "key = ?", ocrCorrectionMemorySettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if setting.Value["enabled"] != true || setting.Value["includeInAIPrompts"] != true {
		t.Fatalf("migration reset explicit memory settings: %#v", setting.Value)
	}
}

func TestOCRCorrectionMemoryCapturesAndDeduplicatesApprovedEdits(t *testing.T) {
	db := newOCRCorrectionMemoryTestDB(t)
	setting := models.AppSetting{Key: ocrCorrectionMemorySettingsKey, Value: models.JSONMap{"enabled": true, "retentionDays": 30, "includeInAIPrompts": false}}
	if err := db.Create(&setting).Error; err != nil {
		t.Fatal(err)
	}
	library := models.Library{Name: "Shows", Type: "series", SourcePath: "/media", DestinationPath: "/library"}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	assetPath := "/media/show/episode.mkv"
	asset := models.AssetRecord{Path: assetPath, FileName: "episode.mkv", LibraryID: library.ID, LogicalGroupPath: "/media/show"}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatal(err)
	}
	identity := canonicalOCRIdentity{SourcePath: assetPath, RequestedLanguage: "eng", PreparedRepresentationHash: strings.Repeat("b", 64)}
	cues := []canonicalBitmapSubtitleCue{
		{SourceOrdinal: 1, Text: "The h0use is here."},
		{SourceOrdinal: 2, Text: "Come inside."},
	}
	decision := canonicalOCRCorrectionProvenance{CandidateSource: "manual-review"}
	if err := addApprovedOCRCorrectionMemory(db, identity, strings.Repeat("c", 64), cues[0], cues, "The house is here.", "edit", decision); err != nil {
		t.Fatal(err)
	}
	var first models.OCRCorrectionMemoryEntry
	if err := db.First(&first).Error; err != nil {
		t.Fatal(err)
	}
	if first.OccurrenceCount != 1 || !first.Enabled || !first.ApprovalValid || first.ScopeType != "series" {
		t.Fatalf("first capture = %#v", first)
	}
	if err := addApprovedOCRCorrectionMemory(db, identity, strings.Repeat("d", 64), cues[0], cues, "The house is here.", "edit", decision); err != nil {
		t.Fatal(err)
	}
	var entries []models.OCRCorrectionMemoryEntry
	if err := db.Find(&entries).Error; err != nil {
		t.Fatal(err)
	}
	var evidence []models.OCRCorrectionMemoryEvidence
	if err := db.Where("entry_id = ?", first.ID).Find(&evidence).Error; err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].OccurrenceCount != 2 || len(evidence) != 2 {
		t.Fatalf("dedupe entries=%d occurrence=%d provenance=%d", len(entries), entries[0].OccurrenceCount, len(evidence))
	}
	for _, item := range evidence {
		if strings.Contains(item.SourceHash, assetPath) || strings.Contains(item.SourceRevision, assetPath) || item.DictionaryRuleID != "" {
			t.Fatalf("provenance retained a raw path or invented a dictionary match: %#v", item)
		}
	}
	var currentSetting models.AppSetting
	if err := db.First(&currentSetting, "key = ?", ocrCorrectionMemorySettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	currentSetting.Value["enabled"] = false
	if err := db.Save(&currentSetting).Error; err != nil {
		t.Fatal(err)
	}
	if err := addApprovedOCRCorrectionMemory(db, identity, strings.Repeat("6", 64), cues[0], cues, "The home is here.", "edit", decision); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&models.OCRCorrectionMemoryEntry{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("disabling learning changed existing entries or captured a new one: count=%d err=%v", count, err)
	}
}

func TestOCRCorrectionMemoryCapturesOnlyManualAndAppliedDictionaryEdits(t *testing.T) {
	db := newOCRCorrectionMemoryTestDB(t)
	if err := db.Create(&models.AppSetting{Key: ocrCorrectionMemorySettingsKey, Value: models.JSONMap{"enabled": true}}).Error; err != nil {
		t.Fatal(err)
	}
	identity := canonicalOCRIdentity{SourcePath: "/unmanaged/show.mkv", RequestedLanguage: "eng", PreparedRepresentationHash: strings.Repeat("2", 64)}
	before := []canonicalBitmapSubtitleCue{
		{SourceOrdinal: 1, Text: "manual h0use"},
		{SourceOrdinal: 2, Text: "dictionary cant"},
		{SourceOrdinal: 3, Text: "unapproved output"},
		{SourceOrdinal: 4, Text: "next cue"},
	}
	after := []canonicalBitmapSubtitleCue{
		{SourceOrdinal: 1, Text: "manual house"},
		{SourceOrdinal: 2, Text: "dictionary can't"},
		{SourceOrdinal: 3, Text: "unapproved result"},
		{SourceOrdinal: 4, Text: "next cue"},
	}
	snapshot := &publishedCanonicalOCRSnapshot{Evidence: canonicalOCREvidence{Identity: identity}}
	corrections := []subtitleOCRCorrection{
		{CueID: 1, Action: "edit", Text: "manual house"},
		{CueID: 2, Action: "apply", Text: "dictionary can't"},
		{CueID: 3, Action: "apply", Text: "unapproved result"},
	}
	decisions := [][]canonicalOCRCorrectionProvenance{{
		{CueID: 1, CandidateSource: "manual-review", Action: "edit"},
		{CueID: 2, CandidateSource: "contextual-ocr-dictionary", Action: "apply", DictionaryRuleIDs: []string{"rule:eng:cant"}, DictionaryRuleVersions: []int{7}},
		{CueID: 3, CandidateSource: "selective-secondary-ocr", Action: "apply"},
	}}
	if err := captureApprovedOCRCorrectionMemory(db, []*publishedCanonicalOCRSnapshot{snapshot}, strings.Repeat("3", 64), corrections, decisions, before, after); err != nil {
		t.Fatal(err)
	}
	var entries []models.OCRCorrectionMemoryEntry
	if err := db.Find(&entries).Error; err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("captured %d entries, want only manual and applied dictionary corrections", len(entries))
	}
	for _, entry := range entries {
		if entry.ScopeType == "global" {
			t.Fatalf("approved examples were automatically promoted to global scope: %#v", entry)
		}
	}
	var dictionaryEvidence models.OCRCorrectionMemoryEvidence
	if err := db.Where("dictionary_rule_id = ?", "rule:eng:cant").First(&dictionaryEvidence).Error; err != nil {
		t.Fatal(err)
	}
	if dictionaryEvidence.DictionaryRuleVersion != 7 || dictionaryEvidence.ApprovalAction != "apply" {
		t.Fatalf("dictionary provenance = %#v", dictionaryEvidence)
	}
}

func TestOCRCorrectionMemorySuggestionsStayScopedToLanguageAndSeries(t *testing.T) {
	db := newOCRCorrectionMemoryTestDB(t)
	if err := db.Create(&models.AppSetting{Key: ocrCorrectionMemorySettingsKey, Value: models.JSONMap{"enabled": true}}).Error; err != nil {
		t.Fatal(err)
	}
	library := models.Library{Name: "Shows", Type: "series", SourcePath: "/media", DestinationPath: "/library"}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/media/show/episode-1.mkv", "/media/show/episode-2.mkv"} {
		if err := db.Create(&models.AssetRecord{Path: path, FileName: "episode.mkv", LibraryID: library.ID, LogicalGroupPath: "/media/show"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	identity := canonicalOCRIdentity{SourcePath: "/media/show/episode-1.mkv", RequestedLanguage: "eng", PreparedRepresentationHash: strings.Repeat("e", 64)}
	cues := []canonicalBitmapSubtitleCue{{SourceOrdinal: 1, Text: "The h0use is here."}, {SourceOrdinal: 2, Text: "Come inside."}}
	if err := addApprovedOCRCorrectionMemory(db, identity, strings.Repeat("f", 64), cues[0], cues, "The house is here.", "edit", canonicalOCRCorrectionProvenance{}); err != nil {
		t.Fatal(err)
	}
	changedSource := identity
	changedSource.PreparedRepresentationHash = strings.Repeat("8", 64)
	target := &canonicalOCRReviewTarget{State: "exact", Identity: changedSource}
	snapshot := &publishedCanonicalOCRSnapshot{Evidence: canonicalOCREvidence{Cues: cues}}
	finding := subtitleOCRQualityFinding{CueID: 1, OriginalText: "The h0use is here."}
	if suggestions := lookupOCRCorrectionMemorySuggestions(db, target, snapshot, finding); len(suggestions) != 0 {
		t.Fatalf("same-asset suggestions survived a changed source revision: %#v", suggestions)
	}
	otherEpisode := canonicalOCRIdentity{SourcePath: "/media/show/episode-2.mkv", RequestedLanguage: "eng", PreparedRepresentationHash: strings.Repeat("1", 64)}
	target = &canonicalOCRReviewTarget{State: "exact", Identity: otherEpisode}
	suggestions := lookupOCRCorrectionMemorySuggestions(db, target, snapshot, finding)
	if len(suggestions) != 1 || suggestions[0].CorrectedText != "The house is here." || suggestions[0].ScopeType != "series" {
		t.Fatalf("same-series suggestions = %#v", suggestions)
	}
	target.Identity.RequestedLanguage = "spa"
	if suggestions := lookupOCRCorrectionMemorySuggestions(db, target, snapshot, finding); len(suggestions) != 0 {
		t.Fatalf("cross-language suggestions = %#v", suggestions)
	}
}

func TestOCRCorrectionMemorySimilarityOnlyFindsCloseCueText(t *testing.T) {
	if score := similarityOCRCorrectionMemory("The h0use is here", "The house is here"); score < 0.6 {
		t.Fatalf("single-character OCR correction similarity = %.3f, want at least 0.6", score)
	}
	if score := similarityOCRCorrectionMemory("The house is here", "We should leave now"); score >= 0.6 {
		t.Fatalf("unrelated cue similarity = %.3f, want less than 0.6", score)
	}
}

func TestOCRCorrectionMemoryPromptEnrichmentNeedsSeparateOptIn(t *testing.T) {
	db := newOCRCorrectionMemoryTestDB(t)
	setting := models.AppSetting{Key: ocrCorrectionMemorySettingsKey, Value: models.JSONMap{"enabled": true, "includeInAIPrompts": false}}
	if err := db.Create(&setting).Error; err != nil {
		t.Fatal(err)
	}
	target := &canonicalOCRReviewTarget{Identity: canonicalOCRIdentity{RequestedLanguage: "eng"}}
	report := subtitleOCRQualityReport{Findings: []subtitleOCRQualityFinding{{
		CueID: 1, MemorySuggestions: []ocrCorrectionMemorySuggestion{{SourceLanguage: "eng", CorrectedText: "The house.", MatchType: "exact_context", ScopeType: "series"}},
	}}}
	cues := []subtitleProofreadingCue{{ID: "ocr-cue-1"}}
	attachOCRCorrectionMemoryPromptSuggestions(db, target, report, cues)
	if len(cues[0].MemorySuggestions) != 0 {
		t.Fatalf("suggestions were attached to prompt while separate opt-in was off: %#v", cues[0])
	}
	setting.Value["includeInAIPrompts"] = true
	if err := db.Save(&setting).Error; err != nil {
		t.Fatal(err)
	}
	attachOCRCorrectionMemoryPromptSuggestions(db, target, report, cues)
	if len(cues[0].MemorySuggestions) != 1 || cues[0].MemorySuggestions[0].CorrectedText != "The house." {
		t.Fatalf("opted-in prompt examples = %#v", cues[0].MemorySuggestions)
	}
	many := subtitleOCRQualityReport{}
	manyCues := make([]subtitleProofreadingCue, 6)
	for cueID := range manyCues {
		cueID++
		many.Findings = append(many.Findings, subtitleOCRQualityFinding{
			CueID: cueID, MemorySuggestions: []ocrCorrectionMemorySuggestion{{SourceLanguage: "eng", CorrectedText: fmt.Sprintf("approved %d", cueID), MatchType: "exact_context", ScopeType: "series"}},
		})
		manyCues[cueID-1].ID = fmt.Sprintf("ocr-cue-%d", cueID)
	}
	attachOCRCorrectionMemoryPromptSuggestions(db, target, many, manyCues)
	attached := 0
	for _, cue := range manyCues {
		attached += len(cue.MemorySuggestions)
	}
	if attached != ocrCorrectionMemoryMaxPromptExamples {
		t.Fatalf("prompt enrichment attached %d examples, want its bound %d", attached, ocrCorrectionMemoryMaxPromptExamples)
	}
}

func TestOCRCorrectionMemoryAPIListsEditsRevokesExportsAndDeletes(t *testing.T) {
	db := newOCRCorrectionMemoryTestDB(t)
	if err := db.Create(&models.AppSetting{Key: ocrCorrectionMemorySettingsKey, Value: models.JSONMap{"enabled": true}}).Error; err != nil {
		t.Fatal(err)
	}
	identity := canonicalOCRIdentity{SourcePath: "/media/movie.mkv", RequestedLanguage: "eng", PreparedRepresentationHash: strings.Repeat("4", 64)}
	cue := canonicalBitmapSubtitleCue{SourceOrdinal: 1, Text: "The h0use."}
	if err := addApprovedOCRCorrectionMemory(db, identity, strings.Repeat("5", 64), cue, []canonicalBitmapSubtitleCue{cue}, "The house.", "edit", canonicalOCRCorrectionProvenance{}); err != nil {
		t.Fatal(err)
	}
	var entry models.OCRCorrectionMemoryEntry
	if err := db.First(&entry).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	handler := AssetHandler{db: db}
	router := gin.New()
	router.GET("/memory", handler.ListOCRCorrectionMemory)
	router.GET("/memory/export", handler.ExportOCRCorrectionMemory)
	router.POST("/memory/:id", handler.UpdateOCRCorrectionMemory)
	router.POST("/memory/:id/enabled", handler.SetOCRCorrectionMemoryEnabled)
	router.POST("/memory/:id/feedback", handler.RecordOCRCorrectionMemoryFeedback)
	router.DELETE("/memory/:id", handler.DeleteOCRCorrectionMemory)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		router.ServeHTTP(recorder, req)
		return recorder
	}
	list := request(http.MethodGet, "/memory?search=house&language=eng", "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"sourceRevision"`) {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	disabled := request(http.MethodPost, "/memory/"+fmt.Sprint(entry.ID)+"/enabled", `{"enabled":false}`)
	if disabled.Code != http.StatusOK {
		t.Fatalf("disable status=%d body=%s", disabled.Code, disabled.Body.String())
	}
	enabled := request(http.MethodPost, "/memory/"+fmt.Sprint(entry.ID)+"/enabled", `{"enabled":true}`)
	if enabled.Code != http.StatusOK {
		t.Fatalf("re-enable status=%d body=%s", enabled.Code, enabled.Body.String())
	}
	accepted := request(http.MethodPost, "/memory/"+fmt.Sprint(entry.ID)+"/feedback", `{"decision":"accepted"}`)
	if accepted.Code != http.StatusOK {
		t.Fatalf("accepted feedback status=%d body=%s", accepted.Code, accepted.Body.String())
	}
	updateBody := `{"originalText":"The h0use.","correctedText":"The home.","scopeType":"asset","scopeId":"asset:` + identity.key() + `"}`
	collision := models.OCRCorrectionMemoryEntry{
		SourceLanguage: entry.SourceLanguage, OriginalText: "The h0use.", CorrectedText: "The home.",
		ContextFingerprint: entry.ContextFingerprint, ScopeType: "asset", ScopeID: "asset:" + identity.key(),
		Enabled: true, ApprovalValid: true, OccurrenceCount: 1, ApprovedAt: entry.ApprovedAt, ExpiresAt: entry.ExpiresAt,
	}
	collision.Fingerprint = ocrCorrectionMemoryEntryFingerprint(collision.SourceLanguage, collision.OriginalText, collision.CorrectedText, collision.ContextFingerprint, collision.ScopeType, collision.ScopeID)
	if err := db.Create(&collision).Error; err != nil {
		t.Fatal(err)
	}
	conflict := request(http.MethodPost, "/memory/"+fmt.Sprint(entry.ID), updateBody)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("duplicate update status=%d body=%s", conflict.Code, conflict.Body.String())
	}
	var unchanged models.OCRCorrectionMemoryEntry
	if err := db.First(&unchanged, entry.ID).Error; err != nil {
		t.Fatal(err)
	}
	if unchanged.Fingerprint != entry.Fingerprint || unchanged.CorrectedText != entry.CorrectedText || !unchanged.ApprovalValid || !unchanged.Enabled {
		t.Fatalf("rejected duplicate edit changed the approved entry: %#v", unchanged)
	}
	if err := db.Delete(&collision).Error; err != nil {
		t.Fatal(err)
	}
	update := request(http.MethodPost, "/memory/"+fmt.Sprint(entry.ID), updateBody)
	if update.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", update.Code, update.Body.String())
	}
	var saved models.OCRCorrectionMemoryEntry
	if err := db.First(&saved, entry.ID).Error; err != nil {
		t.Fatal(err)
	}
	expectedFingerprint := ocrCorrectionMemoryEntryFingerprint(saved.SourceLanguage, saved.OriginalText, saved.CorrectedText, saved.ContextFingerprint, saved.ScopeType, saved.ScopeID)
	if saved.Fingerprint != expectedFingerprint {
		t.Fatalf("edited entry fingerprint is stale: got %q want %q", saved.Fingerprint, expectedFingerprint)
	}
	if saved.ApprovalValid || saved.Enabled || saved.CorrectedText != "The home." || saved.AcceptedSuggestions != 1 {
		t.Fatalf("edited entry did not invalidate approval: %#v", saved)
	}
	feedback := request(http.MethodPost, "/memory/"+fmt.Sprint(entry.ID)+"/feedback", `{"decision":"rejected"}`)
	if feedback.Code != http.StatusConflict {
		t.Fatalf("feedback for stale entry status=%d body=%s", feedback.Code, feedback.Body.String())
	}
	export := request(http.MethodGet, "/memory/export", "")
	if export.Code != http.StatusOK || !strings.Contains(export.Body.String(), "The home.") {
		t.Fatalf("export status=%d body=%s", export.Code, export.Body.String())
	}
	deleted := request(http.MethodDelete, "/memory/"+fmt.Sprint(entry.ID), "")
	if deleted.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}
	var remaining int64
	if err := db.Model(&models.OCRCorrectionMemoryEntry{}).Count(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("delete left %d memory entries", remaining)
	}
}
