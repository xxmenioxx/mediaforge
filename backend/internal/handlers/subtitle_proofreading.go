package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	subtitleProofreadingContractVersion = "proofread-v1"
	subtitleProofreadingOperationLimit  = 1
	subtitleProofreadingTimeout         = 5 * time.Minute
	subtitleProofreadingMaxBytes        = 8 * 1024 * 1024
	subtitleProofreadingMaxCues         = 5000
	subtitleProofreadingMaxModelCues    = 240
	subtitleProofreadingMaxBatchCues    = 12
	subtitleProofreadingNeighborRunes   = 240
)

const subtitleProofreadingProtectedPlaceholder = "⟦formatting⟧"

var (
	subtitleProofreadingAdmission = make(chan struct{}, subtitleProofreadingOperationLimit)
	subtitleProofreadingMojibake  = regexp.MustCompile(`(?:Ã[\x80-\xff]|Â[\x80-\xff]|â(?:€|™|œ|“|”|†|‡|€¦)|ï»¿|ðŸ)`)
	subtitleProofreadingConfusion = regexp.MustCompile(`(?i)(?:[a-z][0-9|][a-z]|[a-z][0-9|]{2,}|[0-9|][a-z]{2,})`)
	subtitleProofreadingSpacing   = regexp.MustCompile(`(?:\s+[,.!?;:]|[,;:!?](?:[A-Za-zÀ-ÿ]))`)
)

type subtitleProofreadingStartInput struct {
	Path             string `json:"path" binding:"required"`
	SubtitlePath     string `json:"subtitlePath" binding:"required"`
	Mode             string `json:"mode"`
	ExpectedRevision string `json:"expectedRevision,omitempty"`
}

type subtitleProofreadingCue struct {
	ID                string
	Ordinal           int
	Hash              string
	SourceText        string
	DisplayText       string
	Start             float64
	End               float64
	TextSegments      []subtitleProofreadingTextSegment
	ASSDialogue       *assProtectedDialogue
	Neighbors         []subtitleProofreadingNeighbor
	MemorySuggestions []ocrCorrectionMemoryPromptSuggestion
}

type subtitleProofreadingTextSegment struct {
	Start int
	End   int
}

type subtitleProofreadingNeighbor struct {
	CueID string `json:"cueId"`
	Text  string `json:"text"`
}

type subtitleProofreadingFinding struct {
	CueID                 string                         `json:"cueId"`
	Ordinal               int                            `json:"ordinal"`
	SourceHash            string                         `json:"sourceTextHash"`
	OriginalText          string                         `json:"originalText"`
	Suggested             string                         `json:"suggestedText,omitempty"`
	SuggestedProviderText string                         `json:"-"`
	Status                string                         `json:"status"`
	Category              string                         `json:"category"`
	ReasonCode            string                         `json:"reasonCode"`
	Reason                string                         `json:"reason"`
	Confidence            float64                        `json:"confidence,omitempty"`
	Neighbors             []subtitleProofreadingNeighbor `json:"neighbors,omitempty"`
}

type subtitleProofreadingReport struct {
	SidecarPath     string                        `json:"sidecarPath"`
	Format          string                        `json:"format"`
	Mode            string                        `json:"mode"`
	SourceType      string                        `json:"sourceType"`
	SelfReview      bool                          `json:"selfReview"`
	Language        string                        `json:"language,omitempty"`
	Revision        string                        `json:"revision"`
	Provider        string                        `json:"provider"`
	Model           string                        `json:"model,omitempty"`
	ProviderState   string                        `json:"providerState"`
	ProviderNotice  string                        `json:"providerNotice,omitempty"`
	TotalCount      int                           `json:"totalCount"`
	CleanCount      int                           `json:"cleanCount"`
	SuggestedCount  int                           `json:"suggestedCount"`
	ReviewCount     int                           `json:"reviewCount"`
	UnassessedCount int                           `json:"unassessedCount"`
	Findings        []subtitleProofreadingFinding `json:"findings"`
}

type subtitleProofreadingOperationRequest struct {
	MediaPath           string
	SidecarPath         string
	Format              string
	LogicalTransformKey string
	Revision            publishedSubtitleRevision
	RevisionKey         string
	Mode                string
	Language            string
	SourceType          string
	Cues                []subtitleProofreadingCue
	TotalCueCount       int
	Deterministic       map[string]subtitleProofreadingFinding
	SkippedCueCount     int
}

type subtitleProofreadingApplyInput struct {
	Path         string                              `json:"path" binding:"required"`
	SubtitlePath string                              `json:"subtitlePath" binding:"required"`
	OperationID  string                              `json:"operationId" binding:"required"`
	Revision     string                              `json:"revision" binding:"required"`
	Confirmed    bool                                `json:"confirmed"`
	Decisions    []subtitleProofreadingDecisionInput `json:"decisions" binding:"required"`
}

type subtitleProofreadingDecisionInput struct {
	CueID      string `json:"cueId" binding:"required"`
	SourceHash string `json:"sourceTextHash" binding:"required"`
	Action     string `json:"action" binding:"required"`
	Text       string `json:"text,omitempty"`
}

type subtitleProofreadingRestoreInput struct {
	Path         string `json:"path" binding:"required"`
	SubtitlePath string `json:"subtitlePath" binding:"required"`
	RevisionID   string `json:"revisionId" binding:"required"`
	Expected     string `json:"expectedRevision" binding:"required"`
}

func (h AssetHandler) StartSubtitleProofreading(c *gin.Context) {
	var input subtitleProofreadingStartInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": "invalid subtitle proofreading request"})
		return
	}
	mode := strings.ToLower(strings.TrimSpace(input.Mode))
	if mode == "" {
		mode = "quick"
	}
	if mode != "quick" && mode != "extended" {
		c.JSON(400, gin.H{"error": "proofreading mode must be quick or extended"})
		return
	}
	select {
	case subtitleProofreadingAdmission <- struct{}{}:
	default:
		c.JSON(409, gin.H{"error": "another subtitle proofreading operation is using the local provider"})
		return
	}
	request, err := h.prepareSubtitleProofreading(input, mode)
	if err != nil {
		<-subtitleProofreadingAdmission
		c.JSON(422, gin.H{"error": err.Error()})
		return
	}
	if input.ExpectedRevision != "" && input.ExpectedRevision != request.RevisionKey {
		<-subtitleProofreadingAdmission
		c.JSON(409, gin.H{"error": "subtitle revision changed before proofreading started"})
		return
	}
	operationID := fmt.Sprintf("proofread-%d", time.Now().UnixNano())
	workCtx, cancel := context.WithTimeout(context.Background(), subtitleProofreadingTimeout)
	operation := &subtitleExtractionOperation{
		ID:                  operationID,
		Kind:                "proofreading",
		Status:              "running",
		Phase:               "analyzing",
		Progress:            1,
		AssetPath:           request.MediaPath,
		Format:              request.Format,
		Message:             "Scanning subtitle text",
		CreatedAt:           time.Now(),
		UpdatedAt:           time.Now(),
		cancel:              cancel,
		proofreadingRequest: &request,
		Proofreading:        initialSubtitleProofreadingReport(request),
	}
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[operationID] = operation
	subtitleExtractionOperations.Unlock()
	go func() {
		defer cancel()
		defer func() { <-subtitleProofreadingAdmission }()
		runSubtitleProofreadingOperation(workCtx, operationID, request, h.db)
	}()
	c.JSON(202, gin.H{"operationId": operationID, "status": "running", "phase": "analyzing", "progress": 1, "sourceRevision": request.RevisionKey})
}

func (h AssetHandler) prepareSubtitleProofreading(input subtitleProofreadingStartInput, mode string) (subtitleProofreadingOperationRequest, error) {
	mediaPath, _, err := h.resolveOwnedSubtitleAsset(input.Path)
	if err != nil {
		return subtitleProofreadingOperationRequest{}, err
	}
	sidecarPath, err := validatedExternalSubtitlePath(mediaPath, input.SubtitlePath)
	if err != nil {
		return subtitleProofreadingOperationRequest{}, err
	}
	if err := rejectProofreadingSymlink(sidecarPath); err != nil {
		return subtitleProofreadingOperationRequest{}, err
	}
	info, err := os.Stat(sidecarPath)
	if err != nil || info.IsDir() || info.Size() <= 0 || info.Size() > subtitleProofreadingMaxBytes {
		return subtitleProofreadingOperationRequest{}, errors.New("subtitle sidecar exceeds the bounded proofreading size or is unavailable")
	}
	source, err := resolveExternalSubtitleTranslationSource(mediaPath, sidecarPath)
	if err != nil {
		return subtitleProofreadingOperationRequest{}, err
	}
	format := strings.ToLower(strings.TrimSpace(source.Representation))
	if format != "srt" && format != "ass" && format != "ssa" {
		return subtitleProofreadingOperationRequest{}, errors.New("only owned SRT, ASS, and SSA text sidecars can be proofread")
	}
	revision, content, err := publishedSubtitleRevisionFor(source.ExternalPath, source.LogicalTransformKey)
	if err != nil || len(content) == 0 || len(content) > subtitleProofreadingMaxBytes {
		return subtitleProofreadingOperationRequest{}, errors.New("subtitle sidecar is unavailable for bounded proofreading")
	}
	if !utf8.Valid(content) || !validTranslatedSubtitleSidecar(format, content) {
		return subtitleProofreadingOperationRequest{}, errors.New("subtitle encoding or format is unsupported for safe proofreading")
	}
	if revisionKey := source.SourceRevision; revisionKey == "" {
		return subtitleProofreadingOperationRequest{}, errors.New("subtitle source revision is unavailable")
	}
	currentSourceRevision, err := captureTranslationExternalSourceRevision(source.ExternalPath, source.LogicalTransformKey)
	if err != nil || currentSourceRevision.key() != source.SourceRevision || currentSourceRevision.External == nil || *currentSourceRevision.External != revision {
		return subtitleProofreadingOperationRequest{}, errors.New("subtitle source changed while its revision was being captured")
	}
	parsed, err := parseSubtitleProofreadingCues(format, content)
	if err != nil || len(parsed) == 0 {
		return subtitleProofreadingOperationRequest{}, errors.New("subtitle sidecar contains no supported text cues")
	}
	totalCueCount := len(parsed)
	skippedCueCount := 0
	if len(parsed) > subtitleProofreadingMaxCues {
		skippedCueCount = len(parsed) - subtitleProofreadingMaxCues
		parsed = parsed[:subtitleProofreadingMaxCues]
	}
	language := subtitleProofreadingLanguage(source.Language, parsed)
	sourceType := subtitleProofreadingSourceType(h.db, mediaPath, source.ExternalPath, format, source.LogicalTransformKey)
	deterministic := make(map[string]subtitleProofreadingFinding)
	for index, cue := range parsed {
		cue.Neighbors = subtitleProofreadingNeighbors(parsed, index)
		parsed[index] = cue
		if finding, ok := deterministicSubtitleProofreadingFinding(cue, language); ok {
			finding.Neighbors = cue.Neighbors
			deterministic[cue.ID] = finding
		}
	}
	if sourceType == "ocr-generated" {
		mergeCanonicalOCRProofreadingFindings(source.ExternalPath, format, source.LogicalTransformKey, parsed, deterministic)
	}
	return subtitleProofreadingOperationRequest{
		MediaPath: mediaPath, SidecarPath: source.ExternalPath, Format: format,
		LogicalTransformKey: source.LogicalTransformKey, Revision: revision,
		RevisionKey: source.SourceRevision, Mode: mode, Language: language,
		SourceType: sourceType, Cues: parsed, TotalCueCount: totalCueCount, Deterministic: deterministic, SkippedCueCount: skippedCueCount,
	}, nil
}

func mergeCanonicalOCRProofreadingFindings(path, format, logicalTransform string, cues []subtitleProofreadingCue, findings map[string]subtitleProofreadingFinding) {
	snapshot, ok := loadOrReconstructPublishedOCRSnapshot(path, format, logicalTransform)
	if !ok || snapshot == nil {
		return
	}
	quality := analyzeCanonicalOCRSnapshot(snapshot)
	for _, ocrFinding := range quality.Findings {
		if ocrFinding.Status == subtitleOCRQualityStatusClean {
			continue
		}
		for _, cue := range cues {
			if cue.Ordinal != ocrFinding.CueID || strings.TrimSpace(stripSubtitleProofreadingAnchors(cue.SourceText)) != strings.TrimSpace(ocrFinding.OriginalText) {
				continue
			}
			category := "OCR-like confusion"
			reasonCode := "canonical-ocr-quality"
			if containsString(ocrFinding.Reasons, "replacement character") {
				category = "replacement glyphs"
			}
			reason := strings.Join(ocrFinding.Reasons, "; ")
			if reason == "" {
				reason = "Canonical OCR quality evidence flags this cue for review."
			}
			findings[cue.ID] = subtitleProofreadingFinding{
				CueID: cue.ID, Ordinal: cue.Ordinal, SourceHash: cue.Hash,
				OriginalText: cue.DisplayText,
				Status:       "needs_review", Category: category, ReasonCode: reasonCode,
				Reason: reason + " Use the OCR-specific correction review for correction candidates.", Neighbors: cue.Neighbors,
			}
			break
		}
	}
}

func parseSubtitleProofreadingCues(format string, content []byte) ([]subtitleProofreadingCue, error) {
	if !utf8.Valid(content) {
		return nil, errors.New("subtitle must be valid UTF-8")
	}
	source := string(content)
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "srt":
		return parseSubtitleProofreadingSRT(source), nil
	case "ass", "ssa":
		document, err := parseASSDocument(format, source)
		if err != nil {
			return nil, err
		}
		result := make([]subtitleProofreadingCue, 0, len(document.Events))
		for _, event := range document.Events {
			if !event.Dialogue.HasTranslatableText() {
				continue
			}
			text := event.Dialogue.TranslatableText()
			display := displaySubtitleProofreadingASSDialogue(event.Dialogue)
			result = append(result, makeSubtitleProofreadingCue(event.SourceOrdinal, text, display, event.Start, event.End, nil, &event.Dialogue))
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported subtitle format %q", format)
	}
}

type subtitleProofreadingLine struct {
	start int
	end   int
	full  int
	text  string
}

func parseSubtitleProofreadingSRT(source string) []subtitleProofreadingCue {
	lines := make([]subtitleProofreadingLine, 0)
	for start := 0; start < len(source); {
		end := start
		for end < len(source) && source[end] != '\n' && source[end] != '\r' {
			end++
		}
		full := end
		if full < len(source) && source[full] == '\r' {
			full++
		}
		if full < len(source) && source[full] == '\n' {
			full++
		}
		lines = append(lines, subtitleProofreadingLine{start: start, end: end, full: full, text: source[start:end]})
		start = full
	}
	result := make([]subtitleProofreadingCue, 0)
	for index := 0; index < len(lines); index++ {
		line := lines[index]
		if !strings.Contains(line.text, "-->") {
			continue
		}
		parts := strings.SplitN(line.text, "-->", 2)
		if len(parts) != 2 {
			continue
		}
		startSeconds, startOK := parseSubtitleTimestamp(strings.TrimSpace(parts[0]))
		endParts := strings.Fields(parts[1])
		if !startOK || len(endParts) == 0 {
			continue
		}
		endSeconds, endOK := parseSubtitleTimestamp(endParts[0])
		if !endOK || endSeconds < startSeconds {
			continue
		}
		bodyStart := index + 1
		bodyEnd := bodyStart
		for bodyEnd < len(lines) && strings.TrimSpace(lines[bodyEnd].text) != "" && !strings.Contains(lines[bodyEnd].text, "-->") {
			bodyEnd++
		}
		if bodyStart == bodyEnd {
			continue
		}
		bodyLines := lines[bodyStart:bodyEnd]
		segments := make([]subtitleProofreadingTextSegment, 0, len(bodyLines))
		textParts := make([]string, 0, len(bodyLines))
		for _, body := range bodyLines {
			segments = append(segments, subtitleProofreadingTextSegment{Start: body.start, End: body.end})
			textParts = append(textParts, body.text)
		}
		text := strings.Join(textParts, "\n")
		if strings.TrimSpace(text) != "" {
			ordinal := len(result)
			result = append(result, makeSubtitleProofreadingCue(ordinal, text, text, startSeconds, endSeconds, segments, nil))
		}
		index = bodyEnd - 1
	}
	return result
}

func makeSubtitleProofreadingCue(ordinal int, sourceText, display string, start, end float64, segments []subtitleProofreadingTextSegment, dialogue *assProtectedDialogue) subtitleProofreadingCue {
	hash := subtitleProofreadingTextHash(sourceText)
	return subtitleProofreadingCue{
		ID: fmt.Sprintf("%d:%s", ordinal+1, hash[:16]), Ordinal: ordinal + 1,
		Hash: hash, SourceText: sourceText, DisplayText: display, Start: start, End: end,
		TextSegments: segments, ASSDialogue: dialogue,
	}
}

func displaySubtitleProofreadingASSDialogue(dialogue assProtectedDialogue) string {
	var builder strings.Builder
	for _, part := range dialogue.Parts {
		if part.Protected {
			builder.WriteString(subtitleProofreadingProtectedPlaceholder)
		} else {
			builder.WriteString(part.Text)
		}
	}
	return builder.String()
}

func subtitleProofreadingProviderText(cue subtitleProofreadingCue) (string, error) {
	if cue.ASSDialogue == nil {
		return cue.SourceText, nil
	}
	return displaySubtitleProofreadingASSDialogue(*cue.ASSDialogue), nil
}

func normalizeSubtitleProofreadingProtectedText(cue subtitleProofreadingCue, candidate string) (string, error) {
	if cue.ASSDialogue == nil {
		return candidate, nil
	}
	protectedCount := 0
	for _, part := range cue.ASSDialogue.Parts {
		if part.Protected {
			protectedCount++
		}
	}
	if strings.Count(candidate, subtitleProofreadingProtectedPlaceholder) != protectedCount || strings.Contains(candidate, translationProtectedAnchorPrefix) {
		return "", errors.New("ASS/SSA text must preserve each formatting placeholder exactly once")
	}
	canonical := candidate
	for _, part := range cue.ASSDialogue.Parts {
		if part.Protected {
			canonical = strings.Replace(canonical, subtitleProofreadingProtectedPlaceholder, part.Anchor, 1)
		}
	}
	if _, err := cue.ASSDialogue.ReconstructProviderText(canonical); err != nil {
		return "", fmt.Errorf("ASS/SSA formatting placeholders changed: %w", err)
	}
	return canonical, nil
}

func subtitleProofreadingTextHash(text string) string {
	digest := sha256.Sum256([]byte(text))
	return hex.EncodeToString(digest[:])
}

func deterministicSubtitleProofreadingFinding(cue subtitleProofreadingCue, language string) (subtitleProofreadingFinding, bool) {
	text := cue.SourceText
	finding := subtitleProofreadingFinding{CueID: cue.ID, Ordinal: cue.Ordinal, SourceHash: cue.Hash, OriginalText: cue.DisplayText, Status: "needs_review"}
	switch {
	case strings.ContainsRune(text, '\uFFFD'):
		finding.Category, finding.ReasonCode, finding.Reason = "replacement glyphs", "replacement-glyph", "Contains the Unicode replacement character; verify the original encoding."
	case subtitleProofreadingMojibake.MatchString(text):
		finding.Category, finding.ReasonCode, finding.Reason = "encoding/mojibake", "mojibake", "Contains a common mojibake pattern; confirm the intended characters."
	case subtitleProofreadingConfusion.MatchString(stripSubtitleProofreadingAnchors(text)):
		finding.Category, finding.ReasonCode, finding.Reason = "OCR-like confusion", "confusable-character", "Contains a digit or vertical bar inside a word; check it in context."
	case subtitleProofreadingSpacing.MatchString(stripSubtitleProofreadingAnchors(text)):
		finding.Category, finding.ReasonCode, finding.Reason = "punctuation/spacing", "punctuation-spacing", "Punctuation spacing looks unusual for the detected language."
	default:
		return subtitleProofreadingFinding{}, false
	}
	if language == "unknown" {
		finding.Reason += " Source language is uncertain, so no spelling rule was applied."
	}
	return finding, true
}

func stripSubtitleProofreadingAnchors(text string) string {
	for index := 1; index <= maxTranslationProtectedAnchors; index++ {
		text = strings.ReplaceAll(text, translationProtectedAnchor(index), " ")
	}
	return text
}

func subtitleProofreadingNeighbors(cues []subtitleProofreadingCue, index int) []subtitleProofreadingNeighbor {
	neighbors := make([]subtitleProofreadingNeighbor, 0, 2)
	for _, nearby := range []int{index - 1, index + 1} {
		if nearby < 0 || nearby >= len(cues) {
			continue
		}
		neighbors = append(neighbors, subtitleProofreadingNeighbor{CueID: cues[nearby].ID, Text: truncateSubtitleProofreadingText(cues[nearby].DisplayText, subtitleProofreadingNeighborRunes)})
	}
	return neighbors
}

func reflectStringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func truncateSubtitleProofreadingText(value string, maximum int) string {
	runes := []rune(value)
	if len(runes) <= maximum {
		return value
	}
	return string(runes[:maximum]) + "…"
}

func subtitleProofreadingLanguage(metadata string, cues []subtitleProofreadingCue) string {
	value := strings.ToLower(strings.TrimSpace(metadata))
	switch value {
	case "en", "eng", "english":
		return "en"
	case "es", "spa", "spanish", "castilian":
		return "es"
	case "ja", "jpn", "japanese":
		return "ja"
	case "ko", "kor", "korean":
		return "ko"
	case "fr", "fra", "fre", "french":
		return "fr"
	}
	texts := make([]string, 0, min(8, len(cues)))
	for _, cue := range cues {
		texts = append(texts, cue.DisplayText)
		if len(texts) == 8 {
			break
		}
	}
	return classifySubtitleProofreadingLanguage(strings.Join(texts, " "))
}

func classifySubtitleProofreadingLanguage(text string) string {
	kana, hangul := 0, 0
	spanishMarkers, englishMarkers, frenchMarkers := 0, 0, 0
	for _, r := range text {
		if isJapaneseKana(r) {
			kana++
		}
		if unicode.Is(unicode.Hangul, r) {
			hangul++
		}
	}
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) }) {
		switch word {
		case "el", "que", "qué", "de", "y", "una", "un", "pero", "hola", "porque", "para", "por", "esta", "está", "es", "la", "pasó", "respuesta", "bien":
			spanishMarkers++
		case "the", "and", "you", "this", "that", "what", "know", "with", "have", "happened", "answer", "story", "hello":
			englishMarkers++
		case "le", "les", "des", "est", "et", "mais", "bonjour":
			frenchMarkers++
		}
	}
	switch {
	case kana >= 2:
		return "ja"
	case hangul >= 2:
		return "ko"
	case spanishMarkers >= 1 && spanishMarkers > englishMarkers:
		return "es"
	case frenchMarkers >= 2 && frenchMarkers > englishMarkers:
		return "fr"
	case englishMarkers >= 1 && englishMarkers > spanishMarkers:
		return "en"
	default:
		return "unknown"
	}
}

func subtitleProofreadingCandidateLanguageMatches(source, candidate, expected string) bool {
	if expected == "" || expected == "unknown" {
		return false
	}
	sourceScript := subtitleProofreadingScript(source)
	candidateScript := subtitleProofreadingScript(candidate)
	if expected == "ja" && sourceScript == "japanese" {
		return candidateScript == "japanese"
	}
	if expected == "ko" && sourceScript == "korean" {
		return candidateScript == "korean"
	}
	if expected != "en" && expected != "es" && expected != "fr" {
		return sourceScript == candidateScript
	}
	if sourceScript == "japanese" || sourceScript == "korean" {
		return candidateScript == sourceScript
	}
	other := classifySubtitleProofreadingLanguage(candidate)
	if expected == "en" && (other == "es" || other == "fr") {
		return false
	}
	if expected == "es" && (other == "en" || other == "fr") {
		return false
	}
	if expected == "fr" && (other == "en" || other == "es") {
		return false
	}
	return candidateScript == "latin" || candidateScript == "other"
}

func subtitleProofreadingScript(text string) string {
	kana, hangul, latin := 0, 0, 0
	for _, r := range text {
		switch {
		case isJapaneseKana(r):
			kana++
		case unicode.Is(unicode.Hangul, r):
			hangul++
		case unicode.IsLetter(r) && unicode.In(r, unicode.Latin):
			latin++
		}
	}
	if kana > 0 {
		return "japanese"
	}
	if hangul > 0 {
		return "korean"
	}
	if latin > 0 {
		return "latin"
	}
	return "other"
}

func subtitleProofreadingSourceType(db *gorm.DB, mediaPath, path, format, logicalTransform string) string {
	_, hasOCR := loadOrReconstructPublishedOCRSnapshot(path, format, logicalTransform)
	cleanPath := filepath.Clean(path)
	translationEvidence, translated := false, false
	if db != nil {
		var jobs []models.QueueJob
		if err := db.Where("published_path = ? OR media_path = ?", mediaPath, mediaPath).Find(&jobs).Error; err == nil {
			for _, job := range jobs {
				for _, artifact := range subtitleArtifactsFromJSON(job.SubtitleArtifacts) {
					if filepath.Clean(artifact.PublishedPath) != cleanPath || artifact.Mode != "translation" {
						continue
					}
					translationEvidence = true
					if artifact.Translation != nil {
						if identity, identityErr := translationIdentityFromMetadata(*artifact.Translation); identityErr == nil && identity.OutputRepresentation == format {
							translated = true
						}
					}
				}
			}
		}
	}
	subtitleExtractionOperations.RLock()
	defer subtitleExtractionOperations.RUnlock()
	for _, operation := range subtitleExtractionOperations.items {
		if operation.Status != "completed" || operation.Result == nil {
			continue
		}
		for _, artifact := range operation.Result.Artifacts {
			publishedPath := artifact.PublishedPath
			if publishedPath == "" {
				publishedPath = artifact.StagedPath
			}
			if filepath.Clean(publishedPath) == cleanPath && artifact.Mode == "translation" {
				translationEvidence = true
				if artifact.Translation != nil {
					if identity, identityErr := translationIdentityFromMetadata(*artifact.Translation); identityErr == nil && identity.OutputRepresentation == format {
						translated = true
					}
				}
			}
		}
	}
	if hasOCR && translationEvidence {
		return "unknown"
	}
	if hasOCR {
		return "ocr-generated"
	}
	if translated {
		return "translated"
	}
	if translationEvidence {
		return "unknown"
	}
	return "imported/external"
}

func initialSubtitleProofreadingReport(request subtitleProofreadingOperationRequest) *subtitleProofreadingReport {
	report := &subtitleProofreadingReport{
		SidecarPath: request.SidecarPath, Format: request.Format, Mode: request.Mode,
		SourceType: request.SourceType, SelfReview: request.SourceType == "translated", Language: request.Language,
		Revision: request.RevisionKey, Provider: translationProviderOllama,
		ProviderState: "pending", TotalCount: request.TotalCueCount, Findings: []subtitleProofreadingFinding{},
	}
	for _, finding := range request.Deterministic {
		report.Findings = append(report.Findings, finding)
	}
	report.UnassessedCount = request.SkippedCueCount
	sortSubtitleProofreadingFindings(report.Findings)
	if request.Mode == "extended" {
		report.ProviderNotice = "Extended review sends bounded cue chunks to the configured Translation model and may take longer on this local system."
	}
	return report
}

func sortSubtitleProofreadingFindings(findings []subtitleProofreadingFinding) {
	sort.Slice(findings, func(i, j int) bool { return findings[i].Ordinal < findings[j].Ordinal })
}

func (h AssetHandler) ApplySubtitleProofreading(c *gin.Context) {
	var input subtitleProofreadingApplyInput
	if err := c.ShouldBindJSON(&input); err != nil || len(input.Decisions) == 0 {
		c.JSON(400, gin.H{"error": "proofreading decisions are required"})
		return
	}
	result, err := h.applySubtitleProofreading(input)
	if err != nil {
		c.JSON(409, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, result)
}

func (h AssetHandler) RestoreSubtitleProofreadingRevision(c *gin.Context) {
	var input subtitleProofreadingRestoreInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": "proofreading revision restore request is invalid"})
		return
	}
	result, err := h.restoreSubtitleProofreadingRevision(input)
	if err != nil {
		c.JSON(409, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, result)
}

func proofreadCueByID(cues []subtitleProofreadingCue, cueID string) (subtitleProofreadingCue, bool) {
	for _, cue := range cues {
		if cue.ID == cueID {
			return cue, true
		}
	}
	return subtitleProofreadingCue{}, false
}

func validateSubtitleProofreadingCandidate(cue subtitleProofreadingCue, candidate, format, language string) error {
	if !utf8.ValidString(candidate) || strings.TrimSpace(candidate) == "" || strings.ContainsRune(candidate, '\r') || len(candidate) > 4*1024 {
		return errors.New("suggested text is empty, oversized, or invalid UTF-8")
	}
	if len([]rune(candidate)) > max(64, len([]rune(cue.SourceText))*3) {
		return errors.New("suggested text changes the cue beyond the supported length")
	}
	if language == "unknown" || !subtitleProofreadingCandidateLanguageMatches(cue.SourceText, candidate, language) {
		return errors.New("suggested text does not preserve the detected language")
	}
	if format == "srt" {
		if strings.Count(candidate, "\n") != strings.Count(cue.SourceText, "\n") {
			return errors.New("suggested text changes SRT line breaks")
		}
		if !reflectStringSlicesEqual(subtitleProofreadingInlineTags(cue.SourceText), subtitleProofreadingInlineTags(candidate)) {
			return errors.New("suggested text changes SRT formatting tags")
		}
		return nil
	}
	if cue.ASSDialogue == nil {
		return errors.New("ASS/SSA cue structure is unavailable")
	}
	if _, err := cue.ASSDialogue.ReconstructProviderText(candidate); err != nil {
		return fmt.Errorf("suggested text changes protected ASS/SSA structure: %w", err)
	}
	return nil
}

func subtitleProofreadingInlineTags(text string) []string {
	tags := make([]string, 0)
	for index := 0; index < len(text); {
		if text[index] != '<' {
			index++
			continue
		}
		end := strings.IndexByte(text[index:], '>')
		if end < 0 {
			break
		}
		end += index
		candidate := text[index : end+1]
		lower := strings.ToLower(candidate)
		if lower == "<i>" || lower == "</i>" || lower == "<b>" || lower == "</b>" || lower == "<u>" || lower == "</u>" || lower == "<s>" || lower == "</s>" || strings.HasPrefix(lower, "<font ") || strings.HasPrefix(lower, "</font") {
			tags = append(tags, candidate)
		}
		index = end + 1
	}
	return tags
}

func applySubtitleProofreadingReplacements(format string, content []byte, cues []subtitleProofreadingCue, replacements map[string]string) ([]byte, error) {
	if len(replacements) == 0 {
		return append([]byte(nil), content...), nil
	}
	source := string(content)
	if format == "srt" {
		splices := make([]subtitleProofreadingSplice, 0)
		for cueID, replacement := range replacements {
			cue, ok := proofreadCueByID(cues, cueID)
			if !ok {
				return nil, fmt.Errorf("unknown proofreading cue %s", cueID)
			}
			parts := strings.Split(replacement, "\n")
			if len(parts) != len(cue.TextSegments) {
				return nil, errors.New("SRT replacement changes cue line count")
			}
			for index, span := range cue.TextSegments {
				splices = append(splices, subtitleProofreadingSplice{start: span.Start, end: span.End, text: parts[index]})
			}
		}
		sort.Slice(splices, func(i, j int) bool { return splices[i].start > splices[j].start })
		for _, splice := range splices {
			source = source[:splice.start] + splice.text + source[splice.end:]
		}
		return []byte(source), nil
	}
	document, err := parseASSDocument(format, source)
	if err != nil {
		return nil, err
	}
	ordinalReplacements := make(map[int]string, len(replacements))
	for cueID, replacement := range replacements {
		cue, ok := proofreadCueByID(cues, cueID)
		if !ok {
			return nil, fmt.Errorf("unknown proofreading cue %s", cueID)
		}
		ordinalReplacements[cue.Ordinal-1] = replacement
	}
	serialized, err := document.ReconstructDialogueProviderTexts(ordinalReplacements)
	if err != nil {
		return nil, err
	}
	return []byte(serialized), nil
}

type subtitleProofreadingSplice struct {
	start int
	end   int
	text  string
}

func subtitleProofreadingModelTargets(request subtitleProofreadingOperationRequest, report *subtitleProofreadingReport) []subtitleProofreadingCue {
	selected := make([]subtitleProofreadingCue, 0)
	if request.Mode == "extended" {
		for _, cue := range request.Cues {
			selected = append(selected, cue)
		}
	} else {
		for _, cue := range request.Cues {
			if _, flagged := request.Deterministic[cue.ID]; flagged {
				selected = append(selected, cue)
			}
		}
	}
	if len(selected) > subtitleProofreadingMaxModelCues {
		missing := len(selected) - subtitleProofreadingMaxModelCues
		report.UnassessedCount += missing
		selected = selected[:subtitleProofreadingMaxModelCues]
	}
	return selected
}

func runSubtitleProofreadingOperation(ctx context.Context, operationID string, request subtitleProofreadingOperationRequest, db *gorm.DB) {
	report := initialSubtitleProofreadingReport(request)
	finish := func(status, phase, message string) {
		refreshSubtitleProofreadingCounts(report)
		updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
			operation.Status = status
			operation.Phase = phase
			operation.Progress = 100
			operation.Message = message
			operation.Proofreading = report
		})
	}
	if !publishedSubtitleRevisionMatches(request.Revision) {
		report.ProviderState = "stale"
		report.ProviderNotice = "The source sidecar changed during review. Refresh Asset Info and retry."
		finish("completed", "stale", "Proofreading findings are bound to a stale source revision")
		return
	}
	selected := subtitleProofreadingModelTargets(request, report)
	if ctx.Err() != nil {
		report.ProviderState = "cancelled"
		report.ProviderNotice = "Proofreading was cancelled; deterministic findings remain available."
		addProofreadingUnassessedCount(report, selected, request.Deterministic)
		finish("cancelled", "cancelled", "Proofreading cancelled; deterministic findings remain available")
		return
	}
	if request.Mode == "quick" && len(selected) == 0 {
		report.ProviderState = "not_needed"
		finish("completed", "completed", "Quick review found no deterministic cues requiring AI review")
		return
	}
	if request.Language == "unknown" {
		report.ProviderState = "language_uncertain"
		report.ProviderNotice = "Language could not be identified confidently; AI suggestions were skipped."
		addProofreadingUnassessedCount(report, selected, request.Deterministic)
		finish("completed", "completed", "Deterministic findings are available; AI review needs a known language")
		return
	}
	settings := loadTranslationSettings(db)
	if !settings.Enabled {
		report.ProviderState = "disabled"
		report.ProviderNotice = "Enable the existing Translation provider before using AI proofreading."
		addProofreadingUnassessedCount(report, selected, request.Deterministic)
		finish("completed", "completed", "Deterministic findings are available; AI proofreading is disabled in Translation Settings")
		return
	}
	provider, err := newOllamaTranslationProvider(settings)
	if err != nil {
		report.ProviderState = "not_configured"
		report.ProviderNotice = subtitleProofreadingProviderReason(err)
		addProofreadingUnassessedCount(report, selected, request.Deterministic)
		finish("completed", "completed", "Deterministic findings are available; configure the existing Translation model and retry")
		return
	}
	report.Model = provider.model
	readiness := provider.TestConnection(ctx)
	if readiness.State != translationReadinessReady {
		report.ProviderState = string(readiness.State)
		report.ProviderNotice = boundedTranslationReadinessDiagnostic(readiness.Diagnostic)
		addProofreadingUnassessedCount(report, selected, request.Deterministic)
		finish("completed", "completed", "Deterministic findings are available; the configured model is not ready")
		return
	}
	report.ProviderState = "ready"
	if len(selected) == 0 {
		finish("completed", "completed", "No eligible cues required an AI proofreading request")
		return
	}
	processed := 0
	for start := 0; start < len(selected); start += subtitleProofreadingMaxBatchCues {
		if err := ctx.Err(); err != nil {
			addProofreadingUnassessedCount(report, selected[processed:], request.Deterministic)
			finish("cancelled", "cancelled", "Proofreading cancelled; completed deterministic findings remain available")
			return
		}
		end := min(start+subtitleProofreadingMaxBatchCues, len(selected))
		batch := selected[start:end]
		result, err := provider.Proofread(ctx, request.Language, batch)
		if err != nil {
			report.ProviderState = "unavailable"
			report.ProviderNotice = subtitleProofreadingProviderReason(err)
			addProofreadingUnassessedCount(report, selected[processed:], request.Deterministic)
			finish("completed", "completed", "Deterministic findings remain available; AI review can be retried")
			return
		}
		for _, finding := range result.Findings {
			cue, ok := proofreadCueByID(batch, finding.CueID)
			if !ok || cue.Hash != finding.SourceHash {
				report.ProviderState = "invalid_response"
				report.ProviderNotice = "The model returned a cue that did not match the captured sidecar revision."
				addProofreadingUnassessedCount(report, selected[processed:], request.Deterministic)
				finish("completed", "completed", "Deterministic findings remain available; invalid model output was discarded")
				return
			}
			existing, deterministic := request.Deterministic[cue.ID]
			if finding.Status == "clean" {
				if deterministic {
					// Deterministic encoding and glyph evidence stays reviewable even
					// when the model does not identify an edit.
					existing.Suggested = ""
					existing.Status = "needs_review"
					upsertSubtitleProofreadingFinding(report, existing)
				} else {
					removeSubtitleProofreadingFinding(report, cue.ID)
				}
				continue
			}
			category := finding.Category
			if category == "" {
				category = "spelling"
			}
			reasonCode, reason := finding.ReasonCode, strings.TrimSpace(finding.Reason)
			if reasonCode == "" {
				reasonCode = "model-review"
			}
			if reason == "" {
				reason = "The model recommends an operator review for this cue."
			}
			findingStatus := "needs_review"
			if finding.Status == "suggested" && strings.TrimSpace(finding.SuggestedText) != "" {
				findingStatus = "suggested"
			}
			upsertSubtitleProofreadingFinding(report, subtitleProofreadingFinding{
				CueID: cue.ID, Ordinal: cue.Ordinal, SourceHash: cue.Hash,
				OriginalText: cue.DisplayText, Suggested: finding.SuggestedText, SuggestedProviderText: finding.validatedText,
				Status: findingStatus, Category: category, ReasonCode: reasonCode,
				Reason: reason, Confidence: finding.Confidence, Neighbors: cue.Neighbors,
			})
		}
		processed += len(batch)
		updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
			operation.Phase = "reviewing"
			operation.Processed = processed
			operation.Total = len(selected)
			operation.Progress = min(95, 5+float64(processed)/float64(max(1, len(selected)))*90)
			operation.Message = fmt.Sprintf("Reviewed %d of %d bounded cue targets", processed, len(selected))
		})
	}
	if !publishedSubtitleRevisionMatches(request.Revision) {
		report.ProviderState = "stale"
		report.ProviderNotice = "The source sidecar changed while the model was reviewing it. Refresh Asset Info and retry."
	}
	sortSubtitleProofreadingFindings(report.Findings)
	finish("completed", "completed", "Proofreading review completed; suggestions remain pending operator approval")
}

func addProofreadingUnassessedCount(report *subtitleProofreadingReport, cues []subtitleProofreadingCue, deterministic map[string]subtitleProofreadingFinding) {
	for _, cue := range cues {
		if _, flagged := deterministic[cue.ID]; !flagged {
			report.UnassessedCount++
		}
	}
}

func refreshSubtitleProofreadingCounts(report *subtitleProofreadingReport) {
	report.SuggestedCount = 0
	report.ReviewCount = 0
	for _, finding := range report.Findings {
		switch finding.Status {
		case "suggested":
			report.SuggestedCount++
		case "needs_review":
			report.ReviewCount++
		}
	}
	report.CleanCount = max(0, report.TotalCount-report.SuggestedCount-report.ReviewCount-report.UnassessedCount)
}

func upsertSubtitleProofreadingFinding(report *subtitleProofreadingReport, finding subtitleProofreadingFinding) {
	for index := range report.Findings {
		if report.Findings[index].CueID == finding.CueID {
			report.Findings[index] = finding
			return
		}
	}
	report.Findings = append(report.Findings, finding)
}

func removeSubtitleProofreadingFinding(report *subtitleProofreadingReport, cueID string) {
	for index, finding := range report.Findings {
		if finding.CueID == cueID {
			report.Findings = append(report.Findings[:index], report.Findings[index+1:]...)
			return
		}
	}
}

func currentSubtitleProofreadingOperation(id string) (subtitleExtractionOperation, bool) {
	subtitleExtractionOperations.RLock()
	defer subtitleExtractionOperations.RUnlock()
	operation, ok := subtitleExtractionOperations.items[id]
	if !ok {
		return subtitleExtractionOperation{}, false
	}
	return cloneSubtitleExtractionOperationSnapshot(operation), true
}

func normalizeProofreadingExtension(path string) string {
	return strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
}

func subtitleProofreadingProviderReason(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > translationReadinessDiagnosticMaxLength {
		message = message[:translationReadinessDiagnosticMaxLength]
	}
	return message
}

func subtitleProofreadingCueCount(cues []subtitleProofreadingCue) int {
	return len(cues)
}

func validSubtitleProofreadingRevisionKey(value string) bool {
	decoded, err := hex.DecodeString(strings.TrimSpace(value))
	return err == nil && len(decoded) == 32
}

func subtitleProofreadingTempFile(path string) (string, *os.File, error) {
	file, err := os.CreateTemp(filepath.Dir(path), ".mvforge-proofreading-*")
	if err != nil {
		return "", nil, err
	}
	return file.Name(), file, nil
}
