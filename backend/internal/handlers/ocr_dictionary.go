package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	ocrDictionaryMaxEntries     = 4096
	ocrDictionaryMaxFieldRunes  = 256
	ocrDictionaryMaxPageSize    = 100
	ocrDictionaryMaxSampleBytes = 4 * 1024
)

var ocrDictionaryRevisionMu sync.RWMutex

type ocrDictionaryRuleInput struct {
	Language       string `json:"language"`
	Pattern        string `json:"pattern"`
	Replacement    string `json:"replacement"`
	MatchMode      string `json:"matchMode"`
	BoundaryPolicy string `json:"boundaryPolicy"`
	LeftContext    string `json:"leftContext"`
	RightContext   string `json:"rightContext"`
	CasePolicy     string `json:"casePolicy"`
	Priority       int    `json:"priority"`
	Enabled        *bool  `json:"enabled,omitempty"`
}

type ocrDictionaryTestInput struct {
	Language string                 `json:"language"`
	Sample   string                 `json:"sample"`
	Rule     ocrDictionaryRuleInput `json:"rule"`
}

type ocrDictionaryRevisionRule struct {
	ID             string `json:"id"`
	Language       string `json:"language"`
	Pattern        string `json:"pattern"`
	Replacement    string `json:"replacement"`
	MatchMode      string `json:"matchMode"`
	BoundaryPolicy string `json:"boundaryPolicy"`
	LeftContext    string `json:"leftContext"`
	RightContext   string `json:"rightContext"`
	CasePolicy     string `json:"casePolicy"`
	Priority       int    `json:"priority"`
	Enabled        bool   `json:"enabled"`
	RuleVersion    int    `json:"ruleVersion"`
}

func strictOCRDictionaryLanguage(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "en", "eng", "english":
		return "eng", true
	case "es", "spa", "esp", "spanish":
		return "spa", true
	case "ja", "jp", "jpn", "japanese":
		return "jpn", true
	case "jpn_vert", "ja_vert", "japanese_vertical":
		return "jpn_vert", true
	default:
		return "", false
	}
}

func canonicalOCRDictionaryRule(input ocrDictionaryRuleInput) (models.OCRDictionaryEntry, error) {
	language, ok := strictOCRDictionaryLanguage(input.Language)
	if !ok {
		return models.OCRDictionaryEntry{}, errors.New("unsupported OCR dictionary language")
	}
	input.Pattern = strings.TrimSpace(input.Pattern)
	input.Replacement = strings.TrimSpace(input.Replacement)
	input.MatchMode = strings.ToLower(strings.TrimSpace(input.MatchMode))
	if input.MatchMode == "" {
		input.MatchMode = "literal"
	}
	input.BoundaryPolicy = strings.ToLower(strings.TrimSpace(input.BoundaryPolicy))
	if input.BoundaryPolicy == "" {
		input.BoundaryPolicy = "word"
	}
	input.CasePolicy = strings.ToLower(strings.TrimSpace(input.CasePolicy))
	if input.CasePolicy == "" {
		input.CasePolicy = "exact"
	}
	if input.MatchMode != "literal" || input.BoundaryPolicy != "word" || (input.CasePolicy != "exact" && input.CasePolicy != "insensitive") {
		return models.OCRDictionaryEntry{}, errors.New("rule must use literal matching, word boundaries, and an exact or insensitive case policy")
	}
	for name, value := range map[string]string{
		"pattern": input.Pattern, "replacement": input.Replacement,
		"left context": input.LeftContext, "right context": input.RightContext,
	} {
		if len([]rune(value)) > ocrDictionaryMaxFieldRunes || strings.ContainsAny(value, "\r\n\x00") {
			return models.OCRDictionaryEntry{}, fmt.Errorf("%s exceeds the supported rule bounds", name)
		}
	}
	if input.Pattern == "" || input.Replacement == "" || input.Pattern == input.Replacement {
		return models.OCRDictionaryEntry{}, errors.New("pattern and replacement must be non-empty and different")
	}
	if strings.Contains(input.Pattern, "|") && !containsOCRWordRune(input.Pattern+input.LeftContext+input.RightContext) {
		return models.OCRDictionaryEntry{}, errors.New("bar rules require a literal word context")
	}
	if input.Priority < -1000 || input.Priority > 1000 {
		return models.OCRDictionaryEntry{}, errors.New("priority must be between -1000 and 1000")
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	return models.OCRDictionaryEntry{
		Language: language, Pattern: input.Pattern, Replacement: input.Replacement,
		MatchMode: input.MatchMode, BoundaryPolicy: input.BoundaryPolicy,
		LeftContext: input.LeftContext, RightContext: input.RightContext,
		CasePolicy: input.CasePolicy, Priority: input.Priority, Enabled: enabled,
		Source: "user", RuleVersion: 1,
	}, nil
}

func containsOCRWordRune(value string) bool {
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func (h AssetHandler) ListOCRDictionary(c *gin.Context) {
	query := h.db.Model(&models.OCRDictionaryEntry{})
	if rawLanguage := strings.TrimSpace(c.Query("language")); rawLanguage != "" {
		language, ok := strictOCRDictionaryLanguage(rawLanguage)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported OCR dictionary language"})
			return
		}
		query = query.Where("language = ?", language)
	}
	if source := strings.ToLower(strings.TrimSpace(c.Query("source"))); source != "" {
		if source != "builtin" && source != "user" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "source must be builtin or user"})
			return
		}
		query = query.Where("source = ?", source)
	}
	if rawEnabled := strings.TrimSpace(c.Query("enabled")); rawEnabled != "" {
		enabled, err := strconv.ParseBool(rawEnabled)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "enabled must be true or false"})
			return
		}
		query = query.Where("enabled = ?", enabled)
	}
	if search := strings.TrimSpace(c.Query("search")); search != "" {
		if len([]rune(search)) > ocrDictionaryMaxFieldRunes {
			c.JSON(http.StatusBadRequest, gin.H{"error": "search exceeds the supported length"})
			return
		}
		pattern := "%" + strings.ReplaceAll(strings.ReplaceAll(search, "%", "\\%"), "_", "\\_") + "%"
		query = query.Where("pattern LIKE ? ESCAPE '\\' OR replacement LIKE ? ESCAPE '\\'", pattern, pattern)
	}
	page := boundedPositiveQuery(c.Query("page"), 1, 1_000_000)
	pageSize := boundedPositiveQuery(c.Query("pageSize"), 50, ocrDictionaryMaxPageSize)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not search OCR dictionary"})
		return
	}
	entries := make([]models.OCRDictionaryEntry, 0, pageSize)
	if err := query.Order("priority DESC").Order("id ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&entries).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not search OCR dictionary"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"entries": entries, "page": page, "pageSize": pageSize, "total": total})
}

func boundedPositiveQuery(value string, fallback, maximum int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 1 {
		return fallback
	}
	if parsed > maximum {
		return maximum
	}
	return parsed
}

func (h AssetHandler) CreateOCRDictionaryEntry(c *gin.Context) {
	var input ocrDictionaryRuleInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	entry, err := canonicalOCRDictionaryRule(input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, err := newOCRDictionaryID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create OCR dictionary rule"})
		return
	}
	entry.ID = id
	ocrDictionaryRevisionMu.Lock()
	defer ocrDictionaryRevisionMu.Unlock()
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&models.OCRDictionaryEntry{}).Count(&count).Error; err != nil {
			return err
		}
		if count >= ocrDictionaryMaxEntries {
			return errOCRDictionaryLimit
		}
		if err := ensureOCRDictionaryEntryIsUnique(tx, entry, ""); err != nil {
			return err
		}
		return tx.Create(&entry).Error
	}); err != nil {
		if errors.Is(err, errOCRDictionaryLimit) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		} else if errors.Is(err, errOCRDictionaryDuplicate) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create OCR dictionary rule"})
		}
		return
	}
	c.JSON(http.StatusCreated, entry)
}

var (
	errOCRDictionaryLimit     = errors.New("OCR dictionary entry limit reached")
	errOCRDictionaryDuplicate = errors.New("an identical OCR dictionary rule already exists")
)

func ensureOCRDictionaryEntryIsUnique(db *gorm.DB, entry models.OCRDictionaryEntry, excludedID string) error {
	query := db.Model(&models.OCRDictionaryEntry{}).
		Where("language = ? AND pattern = ? AND replacement = ? AND boundary_policy = ? AND case_policy = ? AND left_context = ? AND right_context = ?",
			entry.Language, entry.Pattern, entry.Replacement, entry.BoundaryPolicy, entry.CasePolicy, entry.LeftContext, entry.RightContext)
	if excludedID != "" {
		query = query.Where("id <> ?", excludedID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errOCRDictionaryDuplicate
	}
	return nil
}

func newOCRDictionaryID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "user:" + hex.EncodeToString(bytes), nil
}

func (h AssetHandler) UpdateOCRDictionaryEntry(c *gin.Context) {
	var input ocrDictionaryRuleInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	entry, err := canonicalOCRDictionaryRule(input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ocrDictionaryRevisionMu.Lock()
	defer ocrDictionaryRevisionMu.Unlock()
	var current models.OCRDictionaryEntry
	if err := h.db.First(&current, "id = ?", c.Param("id")).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "OCR dictionary entry not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load OCR dictionary entry"})
		return
	}
	if current.Source == "builtin" {
		c.JSON(http.StatusConflict, gin.H{"error": "built-in OCR dictionary entries cannot be edited"})
		return
	}
	entry.ID = current.ID
	entry.Source = current.Source
	entry.RuleVersion = current.RuleVersion + 1
	if err := ensureOCRDictionaryEntryIsUnique(h.db, entry, current.ID); err != nil {
		if errors.Is(err, errOCRDictionaryDuplicate) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not validate OCR dictionary entry"})
		}
		return
	}
	if err := h.db.Model(&current).Select("language", "pattern", "replacement", "match_mode", "boundary_policy", "left_context", "right_context", "case_policy", "priority", "enabled", "rule_version").Updates(&entry).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update OCR dictionary entry"})
		return
	}
	if err := h.db.First(&entry, "id = ?", current.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not reload OCR dictionary entry"})
		return
	}
	c.JSON(http.StatusOK, entry)
}

func (h AssetHandler) SetOCRDictionaryEntryEnabled(c *gin.Context) {
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || input.Enabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "enabled must be true or false"})
		return
	}
	ocrDictionaryRevisionMu.Lock()
	defer ocrDictionaryRevisionMu.Unlock()
	var entry models.OCRDictionaryEntry
	if err := h.db.First(&entry, "id = ?", c.Param("id")).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "OCR dictionary entry not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load OCR dictionary entry"})
		return
	}
	if err := h.db.Model(&entry).Update("enabled", *input.Enabled).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update OCR dictionary entry"})
		return
	}
	if err := h.db.First(&entry, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not reload OCR dictionary entry"})
		return
	}
	c.JSON(http.StatusOK, entry)
}

func (h AssetHandler) DeleteOCRDictionaryEntry(c *gin.Context) {
	ocrDictionaryRevisionMu.Lock()
	defer ocrDictionaryRevisionMu.Unlock()
	var entry models.OCRDictionaryEntry
	if err := h.db.First(&entry, "id = ?", c.Param("id")).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "OCR dictionary entry not found"})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load OCR dictionary entry"})
		return
	}
	if entry.Source == "builtin" {
		c.JSON(http.StatusConflict, gin.H{"error": "built-in OCR dictionary entries cannot be deleted"})
		return
	}
	if err := h.db.Delete(&entry).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not delete OCR dictionary entry"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h AssetHandler) TestOCRDictionaryRule(c *gin.Context) {
	var input ocrDictionaryTestInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(input.Sample) > ocrDictionaryMaxSampleBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sample exceeds the supported size"})
		return
	}
	if input.Rule.Language == "" {
		input.Rule.Language = input.Language
	}
	rule, err := canonicalOCRDictionaryRule(input.Rule)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if input.Language != "" {
		language, ok := strictOCRDictionaryLanguage(input.Language)
		if !ok || language != rule.Language {
			c.JSON(http.StatusBadRequest, gin.H{"error": "sample language must match the rule language"})
			return
		}
	}
	entry, err := newOCRDictionaryID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not preview OCR dictionary rule"})
		return
	}
	rule.ID = entry
	rules, revision, err := loadOCRDictionaryRules(h.db, rule.Language)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load OCR dictionary rules"})
		return
	}
	rules = append(rules, rule)
	preview := matchOCRDictionaryText(input.Sample, rule.Language, rules)
	c.JSON(http.StatusOK, gin.H{
		"language": rule.Language, "matched": len(preview.Matches) > 0,
		"matchCount": len(preview.Matches), "result": preview.Text,
		"ambiguous": preview.Ambiguous, "dictionaryRevision": revision,
	})
}

func loadOCRDictionaryRules(db *gorm.DB, language string) ([]models.OCRDictionaryEntry, string, error) {
	canonical, ok := strictOCRDictionaryLanguage(language)
	if !ok || db == nil {
		return nil, ocrDictionaryRevisionFor(nil), nil
	}
	entries := make([]models.OCRDictionaryEntry, 0)
	if err := db.Where("language = ?", canonical).Order("priority DESC").Order("id ASC").Find(&entries).Error; err != nil {
		return nil, "", err
	}
	revision := ocrDictionaryRevisionFor(entries)
	enabled := entries[:0]
	for _, entry := range entries {
		if entry.Enabled {
			enabled = append(enabled, entry)
		}
	}
	return enabled, revision, nil
}

func attachOCRDictionarySnapshot(snapshot *publishedCanonicalOCRSnapshot, db *gorm.DB, languageOverride string) error {
	if snapshot == nil {
		return errors.New("canonical OCR snapshot is required")
	}
	language := strings.TrimSpace(snapshot.Evidence.DictionaryLanguage)
	source := strings.TrimSpace(snapshot.Evidence.DictionaryLanguageSource)
	if languageOverride != "" {
		override, ok := strictOCRDictionaryLanguage(languageOverride)
		if !ok {
			return errors.New("unsupported OCR dictionary language selection")
		}
		if language != "" && language != override {
			return errors.New("selected dictionary language does not match canonical OCR language evidence")
		}
		if language == "" {
			language = override
			source = "reevaluation-selection"
		}
	}
	if language != "" {
		canonical, ok := strictOCRDictionaryLanguage(language)
		if !ok {
			return errors.New("canonical OCR dictionary language evidence is unsupported")
		}
		language = canonical
	}
	rules, revision, err := loadOCRDictionaryRules(db, language)
	if err != nil {
		return err
	}
	snapshot.DictionaryRules = rules
	snapshot.DictionaryRevision = revision
	snapshot.DictionaryLanguage = language
	snapshot.DictionaryLanguageSource = source
	return nil
}

func ocrDictionaryRevisionFor(entries []models.OCRDictionaryEntry) string {
	rules := make([]ocrDictionaryRevisionRule, 0, len(entries))
	for _, entry := range entries {
		rules = append(rules, ocrDictionaryRevisionRule{
			ID: entry.ID, Language: entry.Language, Pattern: entry.Pattern, Replacement: entry.Replacement,
			MatchMode: entry.MatchMode, BoundaryPolicy: entry.BoundaryPolicy, LeftContext: entry.LeftContext,
			RightContext: entry.RightContext, CasePolicy: entry.CasePolicy, Priority: entry.Priority,
			Enabled: entry.Enabled, RuleVersion: entry.RuleVersion,
		})
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })
	encoded, _ := json.Marshal(rules)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}
