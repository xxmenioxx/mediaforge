package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	ocrCorrectionMemoryMaxEntries        = 8192
	ocrCorrectionMemoryMaxEvidence       = 16
	ocrCorrectionMemoryMaxPageSize       = 100
	ocrCorrectionMemoryMaxSuggestions    = 3
	ocrCorrectionMemoryMaxPromptExamples = 4
	ocrCorrectionMemoryMaxTextRunes      = 2048
	ocrCorrectionMemoryMaxQueryRunes     = 256
	ocrCorrectionMemoryMaxSearchEntries  = 256
)

var (
	errOCRCorrectionMemoryLimit     = errors.New("OCR correction memory is at its configured storage limit")
	errOCRCorrectionMemoryStale     = errors.New("OCR correction memory approval is stale")
	errOCRCorrectionMemoryScope     = errors.New("OCR correction memory scope is not supported by this source")
	errOCRCorrectionMemoryMissing   = errors.New("OCR correction memory entry was not found")
	errOCRCorrectionMemoryDuplicate = errors.New("OCR correction memory entry already exists in this scope")
)

type ocrCorrectionMemoryProvenance struct {
	SourceRevision        string    `json:"sourceRevision"`
	ArtifactRevision      string    `json:"artifactRevision"`
	SourceHash            string    `json:"sourceHash"`
	CueID                 int       `json:"cueId"`
	ApprovalAction        string    `json:"approvalAction"`
	DictionaryRuleID      string    `json:"dictionaryRuleId,omitempty"`
	DictionaryRuleVersion int       `json:"dictionaryRuleVersion,omitempty"`
	ModelIdentifier       string    `json:"modelIdentifier,omitempty"`
	ApprovedAt            time.Time `json:"approvedAt"`
}

type ocrCorrectionMemoryEntryResponse struct {
	models.OCRCorrectionMemoryEntry
	Provenance []ocrCorrectionMemoryProvenance `json:"provenance"`
}

type ocrCorrectionMemoryListResponse struct {
	Entries  []ocrCorrectionMemoryEntryResponse `json:"entries"`
	Total    int64                              `json:"total"`
	Page     int                                `json:"page"`
	PageSize int                                `json:"pageSize"`
}

type ocrCorrectionMemoryExportResponse struct {
	ExportedAt time.Time                          `json:"exportedAt"`
	Entries    []ocrCorrectionMemoryEntryResponse `json:"entries"`
}

type ocrCorrectionMemoryUpdateInput struct {
	OriginalText  string `json:"originalText"`
	CorrectedText string `json:"correctedText"`
	ScopeType     string `json:"scopeType"`
	ScopeID       string `json:"scopeId"`
}

type ocrCorrectionMemoryEnabledInput struct {
	Enabled bool `json:"enabled"`
}

type ocrCorrectionMemoryFeedbackInput struct {
	Decision string `json:"decision"`
}

type ocrCorrectionMemoryScope struct {
	Type string
	ID   string
	Rank int
}

type ocrCorrectionMemorySuggestion struct {
	EntryID             uint   `json:"entryId"`
	CorrectedText       string `json:"correctedText"`
	MatchType           string `json:"matchType"`
	ScopeType           string `json:"scopeType"`
	SourceLanguage      string `json:"sourceLanguage"`
	AcceptedSuggestions int    `json:"acceptedSuggestions"`
	RejectedSuggestions int    `json:"rejectedSuggestions"`
	ProvenanceCount     int    `json:"provenanceCount"`
	SourceRevision      string `json:"sourceRevision"`
	ArtifactRevision    string `json:"artifactRevision"`
}

type ocrCorrectionMemoryPromptSuggestion struct {
	CorrectedText string `json:"correctedText"`
	MatchType     string `json:"matchType"`
	ScopeType     string `json:"scopeType"`
}

func normalizeOCRCorrectionMemoryLanguage(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeOCRCorrectionMemoryText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func validateOCRCorrectionMemoryText(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') || utf8.RuneCountInString(value) > ocrCorrectionMemoryMaxTextRunes {
		return "", fmt.Errorf("cue text must be non-empty, valid UTF-8, and no longer than %d characters", ocrCorrectionMemoryMaxTextRunes)
	}
	return value, nil
}

func ocrCorrectionMemoryEntryFingerprint(language, original, corrected, contextFingerprint, scopeType, scopeID string) string {
	parts := []string{
		normalizeOCRCorrectionMemoryLanguage(language), normalizeOCRCorrectionMemoryText(original),
		normalizeOCRCorrectionMemoryText(corrected), contextFingerprint, scopeType, scopeID,
	}
	encoded, _ := json.Marshal(parts)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func ocrCorrectionMemoryContextFingerprint(cues []canonicalBitmapSubtitleCue, cueID int) string {
	ordered := append([]canonicalBitmapSubtitleCue(nil), cues...)
	sort.SliceStable(ordered, func(left, right int) bool { return ordered[left].SourceOrdinal < ordered[right].SourceOrdinal })
	index := -1
	for candidate := range ordered {
		if ordered[candidate].SourceOrdinal == cueID {
			index = candidate
			break
		}
	}
	if index < 0 {
		return ""
	}
	start, end := max(0, index-1), min(len(ordered), index+2)
	context := make([]string, 0, end-start)
	for current := start; current < end; current++ {
		text := normalizeOCRCorrectionMemoryText(ordered[current].Text)
		runes := []rune(text)
		if len(runes) > 256 {
			text = string(runes[:256])
		}
		context = append(context, strconv.Itoa(ordered[current].SourceOrdinal)+":"+text)
	}
	encoded, _ := json.Marshal(context)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func ocrCorrectionMemoryScopeDigest(value string) string {
	cleaned := filepath.Clean(strings.TrimSpace(value))
	digest := sha256.Sum256([]byte(cleaned))
	return hex.EncodeToString(digest[:])
}

func ocrCorrectionMemoryScopesForSource(db *gorm.DB, sourcePath, sourceRevision string) ([]ocrCorrectionMemoryScope, models.OCRCorrectionMemoryEvidence) {
	evidence := models.OCRCorrectionMemoryEvidence{SourceRevision: sourceRevision, SourceHash: sourceRevision}
	scopes := []ocrCorrectionMemoryScope{{Type: "global", ID: "global", Rank: 1}}
	var asset models.AssetRecord
	if db != nil && strings.TrimSpace(sourcePath) != "" {
		err := db.Where("path = ?", sourcePath).First(&asset).Error
		if err == nil {
			evidence.AssetID = asset.ID
			assetScopeID := "asset:" + strconv.FormatUint(uint64(asset.ID), 10)
			scopes = append(scopes, ocrCorrectionMemoryScope{Type: "asset", ID: assetScopeID, Rank: 5})
			if asset.LibraryID > 0 {
				evidence.LibraryScopeID = "library:" + strconv.FormatUint(uint64(asset.LibraryID), 10)
				scopes = append(scopes, ocrCorrectionMemoryScope{Type: "library", ID: evidence.LibraryScopeID, Rank: 3})
				var library models.Library
				if db.First(&library, asset.LibraryID).Error == nil && strings.TrimSpace(asset.LogicalGroupPath) != "" {
					groupID := "group:" + ocrCorrectionMemoryScopeDigest(asset.LogicalGroupPath)
					typeName := "title"
					libraryType := strings.ToLower(library.Type)
					if strings.Contains(libraryType, "series") || strings.Contains(libraryType, "show") || strings.Contains(libraryType, "tv") {
						typeName = "series"
						evidence.SeriesScopeID = groupID
					} else {
						evidence.TitleScopeID = groupID
					}
					scopes = append(scopes, ocrCorrectionMemoryScope{Type: typeName, ID: groupID, Rank: 4})
				}
			}
		} else {
			assetScopeID := "asset:" + sourceRevision
			scopes = append(scopes, ocrCorrectionMemoryScope{Type: "asset", ID: assetScopeID, Rank: 5})
		}
	} else {
		assetScopeID := "asset:" + sourceRevision
		scopes = append(scopes, ocrCorrectionMemoryScope{Type: "asset", ID: assetScopeID, Rank: 5})
	}
	return scopes, evidence
}

func defaultOCRCorrectionMemoryScope(scopes []ocrCorrectionMemoryScope) ocrCorrectionMemoryScope {
	for _, scope := range scopes {
		if scope.Type == "series" || scope.Type == "title" {
			return scope
		}
	}
	for _, scope := range scopes {
		if scope.Type == "library" {
			return scope
		}
	}
	for _, scope := range scopes {
		if scope.Type == "asset" {
			return scope
		}
	}
	return ocrCorrectionMemoryScope{Type: "global", ID: "global", Rank: 1}
}

func ocrCorrectionMemoryScopeSupported(scopeType, scopeID string) bool {
	scopeType = strings.ToLower(strings.TrimSpace(scopeType))
	scopeID = strings.TrimSpace(scopeID)
	if scopeID == "" || len(scopeID) > 256 {
		return false
	}
	switch scopeType {
	case "global", "library", "series", "title", "asset":
		return scopeType != "global" || scopeID == "global"
	default:
		return false
	}
}

func purgeExpiredOCRCorrectionMemory(db *gorm.DB, now time.Time) error {
	if db == nil {
		return nil
	}
	subquery := db.Model(&models.OCRCorrectionMemoryEntry{}).Select("id").Where("expires_at <= ?", now)
	if err := db.Where("entry_id IN (?)", subquery).Delete(&models.OCRCorrectionMemoryEvidence{}).Error; err != nil {
		return err
	}
	return db.Where("expires_at <= ?", now).Delete(&models.OCRCorrectionMemoryEntry{}).Error
}

func memoryProvenanceByEntry(db *gorm.DB, ids []uint) (map[uint][]ocrCorrectionMemoryProvenance, error) {
	result := make(map[uint][]ocrCorrectionMemoryProvenance, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	var evidence []models.OCRCorrectionMemoryEvidence
	if err := db.Where("entry_id IN ?", ids).Order("approved_at DESC").Order("id DESC").Find(&evidence).Error; err != nil {
		return nil, err
	}
	for _, item := range evidence {
		result[item.EntryID] = append(result[item.EntryID], ocrCorrectionMemoryProvenance{
			SourceRevision: item.SourceRevision, ArtifactRevision: item.ArtifactRevision, SourceHash: item.SourceHash,
			CueID: item.CueID, ApprovalAction: item.ApprovalAction, DictionaryRuleID: item.DictionaryRuleID,
			DictionaryRuleVersion: item.DictionaryRuleVersion, ModelIdentifier: item.ModelIdentifier, ApprovedAt: item.ApprovedAt,
		})
	}
	return result, nil
}

func ocrCorrectionMemoryResponses(db *gorm.DB, entries []models.OCRCorrectionMemoryEntry) ([]ocrCorrectionMemoryEntryResponse, error) {
	ids := make([]uint, len(entries))
	for index := range entries {
		ids[index] = entries[index].ID
	}
	provenance, err := memoryProvenanceByEntry(db, ids)
	if err != nil {
		return nil, err
	}
	responses := make([]ocrCorrectionMemoryEntryResponse, len(entries))
	for index := range entries {
		entryProvenance := provenance[entries[index].ID]
		if entryProvenance == nil {
			entryProvenance = []ocrCorrectionMemoryProvenance{}
		}
		responses[index] = ocrCorrectionMemoryEntryResponse{OCRCorrectionMemoryEntry: entries[index], Provenance: entryProvenance}
	}
	return responses, nil
}

func (h AssetHandler) ListOCRCorrectionMemory(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "OCR correction memory is unavailable"})
		return
	}
	now := time.Now().UTC()
	if err := purgeExpiredOCRCorrectionMemory(h.db.WithContext(c.Request.Context()), now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not maintain OCR correction memory"})
		return
	}
	query := h.db.WithContext(c.Request.Context()).Model(&models.OCRCorrectionMemoryEntry{})
	if language := normalizeOCRCorrectionMemoryLanguage(c.Query("language")); language != "" {
		query = query.Where("source_language = ?", language)
	}
	if scopeType := strings.ToLower(strings.TrimSpace(c.Query("scopeType"))); scopeType != "" {
		switch scopeType {
		case "global", "library", "series", "title", "asset":
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "scopeType must be global, library, series, title, or asset"})
			return
		}
		query = query.Where("scope_type = ?", scopeType)
	}
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		if utf8.RuneCountInString(search) > ocrCorrectionMemoryMaxQueryRunes {
			c.JSON(http.StatusBadRequest, gin.H{"error": "search exceeds the supported length"})
			return
		}
		pattern := "%" + strings.ReplaceAll(strings.ReplaceAll(search, "%", "\\%"), "_", "\\_") + "%"
		query = query.Where("original_text LIKE ? ESCAPE '\\' OR corrected_text LIKE ? ESCAPE '\\'", pattern, pattern)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not search OCR correction memory"})
		return
	}
	page := boundedPositiveQuery(c.Query("page"), 1, 1_000_000)
	pageSize := boundedPositiveQuery(c.Query("pageSize"), 50, ocrCorrectionMemoryMaxPageSize)
	var entries []models.OCRCorrectionMemoryEntry
	if err := query.Order("approved_at DESC").Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&entries).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not search OCR correction memory"})
		return
	}
	responses, err := ocrCorrectionMemoryResponses(h.db.WithContext(c.Request.Context()), entries)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load OCR correction provenance"})
		return
	}
	c.JSON(http.StatusOK, ocrCorrectionMemoryListResponse{Entries: responses, Total: total, Page: page, PageSize: pageSize})
}

func (h AssetHandler) ExportOCRCorrectionMemory(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "OCR correction memory is unavailable"})
		return
	}
	now := time.Now().UTC()
	db := h.db.WithContext(c.Request.Context())
	if err := purgeExpiredOCRCorrectionMemory(db, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not maintain OCR correction memory"})
		return
	}
	var entries []models.OCRCorrectionMemoryEntry
	if err := db.Order("approved_at DESC").Order("id DESC").Limit(ocrCorrectionMemoryMaxEntries).Find(&entries).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not export OCR correction memory"})
		return
	}
	responses, err := ocrCorrectionMemoryResponses(db, entries)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load OCR correction provenance"})
		return
	}
	c.JSON(http.StatusOK, ocrCorrectionMemoryExportResponse{ExportedAt: now, Entries: responses})
}

func (h AssetHandler) UpdateOCRCorrectionMemory(c *gin.Context) {
	var input ocrCorrectionMemoryUpdateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	original, err := validateOCRCorrectionMemoryText(input.OriginalText)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	corrected, err := validateOCRCorrectionMemoryText(input.CorrectedText)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	scopeType, scopeID := strings.ToLower(strings.TrimSpace(input.ScopeType)), strings.TrimSpace(input.ScopeID)
	if !ocrCorrectionMemoryScopeSupported(scopeType, scopeID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "a supported scope type and identity are required"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid OCR correction memory entry id"})
		return
	}
	var entry models.OCRCorrectionMemoryEntry
	db := h.db.WithContext(c.Request.Context())
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&entry, id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return errOCRCorrectionMemoryMissing
		} else if err != nil {
			return err
		}
		var evidenceCount int64
		if err := tx.Model(&models.OCRCorrectionMemoryEvidence{}).Where("entry_id = ?", entry.ID).Count(&evidenceCount).Error; err != nil {
			return err
		}
		if evidenceCount == 0 {
			return errOCRCorrectionMemoryScope
		}
		if !validateOCRCorrectionMemoryUpdateScope(tx, entry.ID, scopeType, scopeID) {
			return errOCRCorrectionMemoryScope
		}
		textChanged := entry.OriginalText != original || entry.CorrectedText != corrected
		scopeChanged := entry.ScopeType != scopeType || entry.ScopeID != scopeID
		if textChanged {
			now := time.Now().UTC()
			entry.OriginalText, entry.CorrectedText = original, corrected
			entry.ApprovalValid, entry.Enabled, entry.InvalidatedAt = false, false, &now
		}
		if scopeChanged && !textChanged {
			fingerprint := ocrCorrectionMemoryEntryFingerprint(entry.SourceLanguage, original, corrected, entry.ContextFingerprint, scopeType, scopeID)
			var duplicateCount int64
			if err := tx.Model(&models.OCRCorrectionMemoryEntry{}).Where("id <> ? AND fingerprint = ?", entry.ID, fingerprint).Count(&duplicateCount).Error; err != nil {
				return err
			}
			if duplicateCount != 0 {
				return errOCRCorrectionMemoryDuplicate
			}
			entry.Fingerprint = fingerprint
		}
		entry.ScopeType, entry.ScopeID = scopeType, scopeID
		return tx.Save(&entry).Error
	})
	if err != nil {
		memoryWriteError(c, err)
		return
	}
	response, err := ocrCorrectionMemoryResponses(db, []models.OCRCorrectionMemoryEntry{entry})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load OCR correction provenance"})
		return
	}
	c.JSON(http.StatusOK, response[0])
}

func (h AssetHandler) SetOCRCorrectionMemoryEnabled(c *gin.Context) {
	var input ocrCorrectionMemoryEnabledInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid OCR correction memory entry id"})
		return
	}
	var entry models.OCRCorrectionMemoryEntry
	db := h.db.WithContext(c.Request.Context())
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&entry, id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return errOCRCorrectionMemoryMissing
		} else if err != nil {
			return err
		}
		if input.Enabled && !entry.ApprovalValid {
			return errOCRCorrectionMemoryStale
		}
		if input.Enabled {
			var evidenceCount int64
			if err := tx.Model(&models.OCRCorrectionMemoryEvidence{}).Where("entry_id = ?", entry.ID).Count(&evidenceCount).Error; err != nil {
				return err
			}
			if evidenceCount == 0 {
				return errOCRCorrectionMemoryScope
			}
			if err := supersedeOCRCorrectionMemoryPeers(tx, &entry); err != nil {
				return err
			}
			entry.SupersededByID = 0
		}
		entry.Enabled = input.Enabled
		return tx.Save(&entry).Error
	})
	if err != nil {
		memoryWriteError(c, err)
		return
	}
	response, err := ocrCorrectionMemoryResponses(db, []models.OCRCorrectionMemoryEntry{entry})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load OCR correction provenance"})
		return
	}
	c.JSON(http.StatusOK, response[0])
}

func (h AssetHandler) RecordOCRCorrectionMemoryFeedback(c *gin.Context) {
	var input ocrCorrectionMemoryFeedbackInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	decision := strings.ToLower(strings.TrimSpace(input.Decision))
	if decision != "accepted" && decision != "rejected" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "decision must be accepted or rejected"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid OCR correction memory entry id"})
		return
	}
	query := h.db.WithContext(c.Request.Context()).Model(&models.OCRCorrectionMemoryEntry{}).Where("id = ? AND enabled = ? AND approval_valid = ?", id, true, true)
	field := "accepted_suggestions"
	if decision == "rejected" {
		field = "rejected_suggestions"
	}
	result := query.UpdateColumn(field, gorm.Expr(field+" + 1"))
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not record OCR correction memory feedback"})
		return
	}
	if result.RowsAffected == 0 {
		var entry models.OCRCorrectionMemoryEntry
		if err := h.db.WithContext(c.Request.Context()).First(&entry, id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": errOCRCorrectionMemoryMissing.Error()})
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load OCR correction memory entry"})
		} else {
			c.JSON(http.StatusConflict, gin.H{"error": errOCRCorrectionMemoryStale.Error()})
		}
		return
	}
	var entry models.OCRCorrectionMemoryEntry
	if err := h.db.First(&entry, id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": errOCRCorrectionMemoryMissing.Error()})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load OCR correction memory entry"})
		return
	}
	response, err := ocrCorrectionMemoryResponses(h.db.WithContext(c.Request.Context()), []models.OCRCorrectionMemoryEntry{entry})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load OCR correction provenance"})
		return
	}
	c.JSON(http.StatusOK, response[0])
}

func (h AssetHandler) DeleteOCRCorrectionMemory(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid OCR correction memory entry id"})
		return
	}
	err = h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var entry models.OCRCorrectionMemoryEntry
		if err := tx.First(&entry, id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return errOCRCorrectionMemoryMissing
		} else if err != nil {
			return err
		}
		if err := tx.Where("entry_id = ?", id).Delete(&models.OCRCorrectionMemoryEvidence{}).Error; err != nil {
			return err
		}
		return tx.Delete(&entry).Error
	})
	if err != nil {
		memoryWriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "OCR correction memory entry deleted"})
}

func memoryWriteError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errOCRCorrectionMemoryMissing):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, errOCRCorrectionMemoryStale), errors.Is(err, errOCRCorrectionMemoryScope):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, errOCRCorrectionMemoryLimit), errors.Is(err, errOCRCorrectionMemoryDuplicate):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.Is(err, gorm.ErrDuplicatedKey):
		c.JSON(http.StatusConflict, gin.H{"error": errOCRCorrectionMemoryDuplicate.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update OCR correction memory"})
	}
}

func supersedeOCRCorrectionMemoryPeers(tx *gorm.DB, selected *models.OCRCorrectionMemoryEntry) error {
	if selected == nil {
		return nil
	}
	var peers []models.OCRCorrectionMemoryEntry
	if err := tx.Where("id <> ? AND source_language = ? AND context_fingerprint = ? AND scope_type = ? AND scope_id = ? AND enabled = ? AND approval_valid = ?",
		selected.ID, selected.SourceLanguage, selected.ContextFingerprint, selected.ScopeType, selected.ScopeID, true, true).Find(&peers).Error; err != nil {
		return err
	}
	for _, peer := range peers {
		if normalizeOCRCorrectionMemoryText(peer.OriginalText) != normalizeOCRCorrectionMemoryText(selected.OriginalText) {
			continue
		}
		peer.Enabled, peer.SupersededByID = false, selected.ID
		if err := tx.Save(&peer).Error; err != nil {
			return err
		}
	}
	return nil
}

func addApprovedOCRCorrectionMemory(db *gorm.DB, identity canonicalOCRIdentity, artifactRevision string, cue canonicalBitmapSubtitleCue, beforeCues []canonicalBitmapSubtitleCue, correctedText string, action string, decision canonicalOCRCorrectionProvenance) error {
	if db == nil {
		return nil
	}
	settings, err := loadOCRCorrectionMemorySettings(db)
	if err != nil || !settings.Enabled {
		return err
	}
	language := normalizeOCRCorrectionMemoryLanguage(identity.RequestedLanguage)
	if language == "" || len(language) > 32 || len(artifactRevision) != 64 || cue.Text == correctedText {
		return nil
	}
	original, err := validateOCRCorrectionMemoryText(cue.Text)
	if err != nil {
		return nil
	}
	corrected, err := validateOCRCorrectionMemoryText(correctedText)
	if err != nil {
		return nil
	}
	contextFingerprint := ocrCorrectionMemoryContextFingerprint(beforeCues, cue.SourceOrdinal)
	if contextFingerprint == "" {
		return nil
	}
	sourceRevision := identity.key()
	if len(sourceRevision) != 64 {
		return nil
	}
	sourceHash := strings.TrimSpace(identity.PreparedRepresentationHash)
	if sourceHash == "" || len(sourceHash) != 64 {
		sourceHash = sourceRevision
	}
	scopes, evidence := ocrCorrectionMemoryScopesForSource(db, identity.SourcePath, sourceRevision)
	scope := defaultOCRCorrectionMemoryScope(scopes)
	evidence.SourceRevision, evidence.ArtifactRevision, evidence.SourceHash = sourceRevision, artifactRevision, sourceHash
	evidence.CueID, evidence.ApprovalAction, evidence.ApprovedAt = cue.SourceOrdinal, action, time.Now().UTC()
	if len(decision.DictionaryRuleIDs) > 0 {
		evidence.DictionaryRuleID = decision.DictionaryRuleIDs[0]
	}
	if len(decision.DictionaryRuleVersions) > 0 {
		evidence.DictionaryRuleVersion = decision.DictionaryRuleVersions[0]
	}
	entry := models.OCRCorrectionMemoryEntry{
		SourceLanguage: language, OriginalText: original, CorrectedText: corrected, ContextFingerprint: contextFingerprint,
		ScopeType: scope.Type, ScopeID: scope.ID, Enabled: true, ApprovalValid: true, OccurrenceCount: 1,
		ApprovedAt: evidence.ApprovedAt, ExpiresAt: evidence.ApprovedAt.Add(time.Duration(settings.RetentionDays) * 24 * time.Hour),
	}
	entry.Fingerprint = ocrCorrectionMemoryEntryFingerprint(entry.SourceLanguage, entry.OriginalText, entry.CorrectedText, entry.ContextFingerprint, entry.ScopeType, entry.ScopeID)
	return db.Transaction(func(tx *gorm.DB) error {
		if err := purgeExpiredOCRCorrectionMemory(tx, evidence.ApprovedAt); err != nil {
			return err
		}
		created := false
		if err := tx.Where("fingerprint = ?", entry.Fingerprint).First(&entry).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			var count int64
			if err := tx.Model(&models.OCRCorrectionMemoryEntry{}).Count(&count).Error; err != nil {
				return err
			}
			if count >= ocrCorrectionMemoryMaxEntries {
				return errOCRCorrectionMemoryLimit
			}
			if err := tx.Create(&entry).Error; err != nil {
				return err
			}
			created = true
		} else if err != nil {
			return err
		}
		evidence.EntryID = entry.ID
		var existing models.OCRCorrectionMemoryEvidence
		findErr := tx.Where("entry_id = ? AND source_revision = ? AND artifact_revision = ? AND cue_id = ?", entry.ID, evidence.SourceRevision, evidence.ArtifactRevision, evidence.CueID).First(&existing).Error
		if findErr == nil {
			return nil
		}
		if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		var evidenceCount int64
		if err := tx.Model(&models.OCRCorrectionMemoryEvidence{}).Where("entry_id = ?", entry.ID).Count(&evidenceCount).Error; err != nil {
			return err
		}
		if evidenceCount >= ocrCorrectionMemoryMaxEvidence {
			return errOCRCorrectionMemoryLimit
		}
		if err := tx.Create(&evidence).Error; err != nil {
			return err
		}
		updates := map[string]any{
			"enabled": true, "approval_valid": true,
			"invalidated_at": nil, "approved_at": evidence.ApprovedAt, "expires_at": evidence.ApprovedAt.Add(time.Duration(settings.RetentionDays) * 24 * time.Hour),
		}
		if !created {
			updates["occurrence_count"] = gorm.Expr("occurrence_count + 1")
		}
		if err := tx.Model(&entry).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.First(&entry, entry.ID).Error; err != nil {
			return err
		}
		return supersedeOCRCorrectionMemoryPeers(tx, &entry)
	})
}

func captureApprovedOCRCorrectionMemory(db *gorm.DB, snapshots []*publishedCanonicalOCRSnapshot, artifactRevision string, corrections []subtitleOCRCorrection, decisions [][]canonicalOCRCorrectionProvenance, beforeCues, afterCues []canonicalBitmapSubtitleCue) error {
	if db == nil || len(snapshots) == 0 || snapshots[0] == nil || len(decisions) == 0 {
		return nil
	}
	decisionByCue := make(map[int]canonicalOCRCorrectionProvenance)
	for _, decision := range decisions[0] {
		decisionByCue[decision.CueID] = decision
	}
	beforeByCue := make(map[int]canonicalBitmapSubtitleCue, len(beforeCues))
	afterByCue := make(map[int]canonicalBitmapSubtitleCue, len(afterCues))
	for _, cue := range beforeCues {
		beforeByCue[cue.SourceOrdinal] = cue
	}
	for _, cue := range afterCues {
		afterByCue[cue.SourceOrdinal] = cue
	}
	var firstErr error
	for _, correction := range corrections {
		action := strings.ToLower(strings.TrimSpace(correction.Action))
		decision, hasDecision := decisionByCue[correction.CueID]
		if !hasDecision || (action != "edit" && !(action == "apply" && decision.CandidateSource == "contextual-ocr-dictionary")) {
			continue
		}
		before, beforeOK := beforeByCue[correction.CueID]
		after, afterOK := afterByCue[correction.CueID]
		if !beforeOK || !afterOK || before.Text == after.Text {
			continue
		}
		if err := addApprovedOCRCorrectionMemory(db, snapshots[0].Evidence.Identity, artifactRevision, before, beforeCues, after.Text, action, decision); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func similarityOCRCorrectionMemory(left, right string) float64 {
	leftRunes, rightRunes := []rune(normalizeOCRCorrectionMemoryText(left)), []rune(normalizeOCRCorrectionMemoryText(right))
	if len(leftRunes) < 3 || len(rightRunes) < 3 {
		return 0
	}
	if len(leftRunes) > 256 {
		leftRunes = leftRunes[:256]
	}
	if len(rightRunes) > 256 {
		rightRunes = rightRunes[:256]
	}
	ngrams := func(value []rune) map[string]struct{} {
		result := make(map[string]struct{}, len(value)-1)
		for index := 0; index+1 < len(value); index++ {
			result[string(value[index:index+2])] = struct{}{}
		}
		return result
	}
	leftSet, rightSet := ngrams(leftRunes), ngrams(rightRunes)
	intersection := 0
	for item := range leftSet {
		if _, exists := rightSet[item]; exists {
			intersection++
		}
	}
	denominator := len(leftSet) + len(rightSet)
	if denominator == 0 {
		return 0
	}
	return float64(2*intersection) / float64(denominator)
}

type ocrCorrectionMemoryLookup struct {
	sourceRevision string
	assetID        uint
	language       string
	scopes         map[string]int
	entries        []models.OCRCorrectionMemoryEntry
	evidence       map[uint][]models.OCRCorrectionMemoryEvidence
}

func newOCRCorrectionMemoryLookup(db *gorm.DB, target *canonicalOCRReviewTarget) *ocrCorrectionMemoryLookup {
	if db == nil || target == nil {
		return nil
	}
	settings, err := loadOCRCorrectionMemorySettings(db)
	if err != nil || !settings.Enabled {
		return nil
	}
	language := normalizeOCRCorrectionMemoryLanguage(target.Identity.RequestedLanguage)
	if language == "" {
		return nil
	}
	now := time.Now().UTC()
	_ = purgeExpiredOCRCorrectionMemory(db, now)
	sourceRevision := target.Identity.key()
	resolvedScopes, sourceEvidence := ocrCorrectionMemoryScopesForSource(db, target.Identity.SourcePath, sourceRevision)
	if len(resolvedScopes) == 0 {
		return nil
	}
	clauses := make([]string, 0, len(resolvedScopes))
	args := make([]any, 0, len(resolvedScopes)*2+4)
	scopeRanks := make(map[string]int, len(resolvedScopes))
	for _, scope := range resolvedScopes {
		clauses = append(clauses, "(scope_type = ? AND scope_id = ?)")
		args = append(args, scope.Type, scope.ID)
		scopeRanks[scope.Type+"\x00"+scope.ID] = scope.Rank
	}
	args = append(args, language, true, true, now)
	query := "(" + strings.Join(clauses, " OR ") + ") AND source_language = ? AND enabled = ? AND approval_valid = ? AND expires_at > ?"
	var entries []models.OCRCorrectionMemoryEntry
	if err := db.Where(query, args...).Order("approved_at DESC").Limit(ocrCorrectionMemoryMaxSearchEntries).Find(&entries).Error; err != nil || len(entries) == 0 {
		return nil
	}
	ids := make([]uint, len(entries))
	for index := range entries {
		ids[index] = entries[index].ID
	}
	var evidence []models.OCRCorrectionMemoryEvidence
	if err := db.Where("entry_id IN ?", ids).Order("approved_at DESC").Find(&evidence).Error; err != nil {
		return nil
	}
	evidenceByEntry := make(map[uint][]models.OCRCorrectionMemoryEvidence, len(entries))
	for _, item := range evidence {
		evidenceByEntry[item.EntryID] = append(evidenceByEntry[item.EntryID], item)
	}
	return &ocrCorrectionMemoryLookup{sourceRevision: sourceRevision, assetID: sourceEvidence.AssetID, language: language, scopes: scopeRanks, entries: entries, evidence: evidenceByEntry}
}

func (lookup *ocrCorrectionMemoryLookup) suggestions(snapshot *publishedCanonicalOCRSnapshot, finding subtitleOCRQualityFinding) []ocrCorrectionMemorySuggestion {
	if lookup == nil || snapshot == nil || strings.TrimSpace(finding.OriginalText) == "" {
		return nil
	}
	type rankedSuggestion struct {
		item  ocrCorrectionMemorySuggestion
		rank  int
		match int
	}
	ranked := make([]rankedSuggestion, 0, len(lookup.entries))
	for _, entry := range lookup.entries {
		scopeRank, scopeOK := lookup.scopes[entry.ScopeType+"\x00"+entry.ScopeID]
		if !scopeOK {
			continue
		}
		entryEvidence := lookup.evidence[entry.ID]
		if len(entryEvidence) == 0 {
			continue
		}
		staleCurrentAsset, matchingCurrentAsset := false, false
		if lookup.assetID != 0 {
			for _, provenance := range entryEvidence {
				if provenance.AssetID != lookup.assetID {
					continue
				}
				if provenance.SourceRevision == lookup.sourceRevision {
					matchingCurrentAsset = true
				} else {
					staleCurrentAsset = true
				}
			}
		}
		if staleCurrentAsset && !matchingCurrentAsset {
			continue
		}
		if entry.ScopeType == "asset" {
			matchingSource := false
			for _, provenance := range entryEvidence {
				if provenance.SourceRevision == lookup.sourceRevision {
					matchingSource = true
					break
				}
			}
			if !matchingSource {
				continue
			}
		}
		matchType, matchRank := "", 0
		if normalizeOCRCorrectionMemoryText(entry.OriginalText) == normalizeOCRCorrectionMemoryText(finding.OriginalText) {
			if entry.ContextFingerprint == ocrCorrectionMemoryContextFingerprint(snapshot.Evidence.Cues, finding.CueID) {
				matchType, matchRank = "exact_context", 3
			} else {
				matchType, matchRank = "exact_text", 2
			}
		} else if similarityOCRCorrectionMemory(entry.OriginalText, finding.OriginalText) >= 0.6 {
			matchType, matchRank = "similar", 1
		}
		if matchRank == 0 {
			continue
		}
		latest := entryEvidence[0]
		ranked = append(ranked, rankedSuggestion{item: ocrCorrectionMemorySuggestion{
			EntryID: entry.ID, CorrectedText: entry.CorrectedText, MatchType: matchType, ScopeType: entry.ScopeType,
			SourceLanguage: entry.SourceLanguage, AcceptedSuggestions: entry.AcceptedSuggestions, RejectedSuggestions: entry.RejectedSuggestions,
			ProvenanceCount: len(entryEvidence), SourceRevision: latest.SourceRevision, ArtifactRevision: latest.ArtifactRevision,
		}, rank: scopeRank, match: matchRank})
	}
	sort.SliceStable(ranked, func(left, right int) bool {
		if ranked[left].match != ranked[right].match {
			return ranked[left].match > ranked[right].match
		}
		if ranked[left].rank != ranked[right].rank {
			return ranked[left].rank > ranked[right].rank
		}
		if ranked[left].item.AcceptedSuggestions != ranked[right].item.AcceptedSuggestions {
			return ranked[left].item.AcceptedSuggestions > ranked[right].item.AcceptedSuggestions
		}
		return ranked[left].item.EntryID < ranked[right].item.EntryID
	})
	if len(ranked) > ocrCorrectionMemoryMaxSuggestions {
		ranked = ranked[:ocrCorrectionMemoryMaxSuggestions]
	}
	result := make([]ocrCorrectionMemorySuggestion, len(ranked))
	for index := range ranked {
		result[index] = ranked[index].item
	}
	return result
}

func lookupOCRCorrectionMemorySuggestions(db *gorm.DB, target *canonicalOCRReviewTarget, snapshot *publishedCanonicalOCRSnapshot, finding subtitleOCRQualityFinding) []ocrCorrectionMemorySuggestion {
	return newOCRCorrectionMemoryLookup(db, target).suggestions(snapshot, finding)
}

func attachOCRCorrectionMemorySuggestions(db *gorm.DB, target *canonicalOCRReviewTarget, report *subtitleOCRQualityReport) {
	if report == nil || target == nil || db == nil {
		return
	}
	_, _, snapshots, err := canonicalOCRReviewTargetSnapshots(target)
	if err != nil || len(snapshots) == 0 || snapshots[0] == nil {
		return
	}
	lookup := newOCRCorrectionMemoryLookup(db, target)
	for index := range report.Findings {
		report.Findings[index].MemorySuggestions = lookup.suggestions(snapshots[0], report.Findings[index])
	}
}

func attachOCRCorrectionMemoryPromptSuggestions(db *gorm.DB, target *canonicalOCRReviewTarget, report subtitleOCRQualityReport, cues []subtitleProofreadingCue) {
	if db == nil || target == nil || len(cues) == 0 {
		return
	}
	settings, err := loadOCRCorrectionMemorySettings(db)
	if err != nil || !settings.IncludeInAIPrompts {
		return
	}
	suggestionsByCue := make(map[int][]ocrCorrectionMemoryPromptSuggestion, len(report.Findings))
	remaining := ocrCorrectionMemoryMaxPromptExamples
	for _, finding := range report.Findings {
		if remaining == 0 {
			break
		}
		if normalizeOCRCorrectionMemoryLanguage(target.Identity.RequestedLanguage) == "" || len(finding.MemorySuggestions) == 0 {
			continue
		}
		for _, suggestion := range finding.MemorySuggestions {
			if normalizeOCRCorrectionMemoryLanguage(suggestion.SourceLanguage) != normalizeOCRCorrectionMemoryLanguage(target.Identity.RequestedLanguage) || strings.TrimSpace(suggestion.CorrectedText) == "" {
				continue
			}
			suggestionsByCue[finding.CueID] = append(suggestionsByCue[finding.CueID], ocrCorrectionMemoryPromptSuggestion{
				CorrectedText: suggestion.CorrectedText, MatchType: suggestion.MatchType, ScopeType: suggestion.ScopeType,
			})
			remaining--
			if remaining == 0 || len(suggestionsByCue[finding.CueID]) >= ocrCorrectionMemoryMaxSuggestions {
				break
			}
		}
	}
	for index := range cues {
		cueID := 0
		if _, err := fmt.Sscanf(cues[index].ID, "ocr-cue-%d", &cueID); err == nil && cueID > 0 {
			cues[index].MemorySuggestions = suggestionsByCue[cueID]
		}
	}
}

func validateOCRCorrectionMemoryUpdateScope(db *gorm.DB, entryID uint, scopeType, scopeID string) bool {
	if scopeType == "global" {
		return scopeID == "global"
	}
	var evidence []models.OCRCorrectionMemoryEvidence
	if db.Where("entry_id = ?", entryID).Find(&evidence).Error != nil {
		return false
	}
	for _, item := range evidence {
		switch scopeType {
		case "asset":
			assetScopeID := "asset:" + item.SourceRevision
			if item.AssetID > 0 {
				assetScopeID = "asset:" + strconv.FormatUint(uint64(item.AssetID), 10)
			}
			if scopeID == assetScopeID {
				return true
			}
		case "library":
			if item.LibraryScopeID != "" && scopeID == item.LibraryScopeID {
				return true
			}
		case "series":
			if item.SeriesScopeID != "" && scopeID == item.SeriesScopeID {
				return true
			}
		case "title":
			if item.TitleScopeID != "" && scopeID == item.TitleScopeID {
				return true
			}
		}
	}
	return false
}
