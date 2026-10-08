package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ocrDictionaryReevaluationInput struct {
	MediaPath           string   `json:"mediaPath,omitempty"`
	SidecarPath         string   `json:"sidecarPath,omitempty"`
	SidecarPaths        []string `json:"sidecarPaths,omitempty"`
	Format              string   `json:"format"`
	LogicalTransformKey string   `json:"logicalTransformKey,omitempty"`
	DictionaryLanguage  string   `json:"dictionaryLanguage,omitempty"`
}

type ocrDictionaryReevaluationChange struct {
	CueID          int      `json:"cueId"`
	Before         string   `json:"before"`
	After          string   `json:"after"`
	RuleIDs        []string `json:"ruleIds"`
	RuleVersions   []int    `json:"ruleVersions"`
	BeforeTextHash string   `json:"beforeTextHash"`
	AfterTextHash  string   `json:"afterTextHash"`
}

type ocrDictionaryReevaluationPreview struct {
	PreviewID                string                            `json:"previewId"`
	ArtifactRevision         string                            `json:"artifactRevision"`
	Revisions                []publishedSubtitleRevision       `json:"revisions"`
	DictionaryLanguage       string                            `json:"dictionaryLanguage,omitempty"`
	DictionaryLanguageSource string                            `json:"dictionaryLanguageSource,omitempty"`
	DictionaryRevision       string                            `json:"dictionaryRevision"`
	CleanCount               int                               `json:"cleanCount"`
	SuspiciousCount          int                               `json:"suspiciousCount"`
	ReviewCount              int                               `json:"reviewCount"`
	RequiresLanguageChoice   bool                              `json:"requiresLanguageChoice"`
	Changes                  []ocrDictionaryReevaluationChange `json:"changes"`
}

type ocrDictionaryReevaluationApplyInput struct {
	MediaPath           string   `json:"mediaPath,omitempty"`
	SidecarPath         string   `json:"sidecarPath,omitempty"`
	SidecarPaths        []string `json:"sidecarPaths,omitempty"`
	Format              string   `json:"format"`
	LogicalTransformKey string   `json:"logicalTransformKey,omitempty"`
	DictionaryLanguage  string   `json:"dictionaryLanguage,omitempty"`
	PreviewID           string   `json:"previewId"`
	ArtifactRevision    string   `json:"artifactRevision"`
	DictionaryRevision  string   `json:"dictionaryRevision"`
	AcceptedCueIDs      []int    `json:"acceptedCueIds"`
}

func (input ocrDictionaryReevaluationApplyInput) previewInput() ocrDictionaryReevaluationInput {
	return ocrDictionaryReevaluationInput{
		MediaPath: input.MediaPath, SidecarPath: input.SidecarPath, SidecarPaths: input.SidecarPaths,
		Format: input.Format, LogicalTransformKey: input.LogicalTransformKey,
		DictionaryLanguage: input.DictionaryLanguage,
	}
}

type ocrDictionaryReevaluationState struct {
	preview  ocrDictionaryReevaluationPreview
	report   subtitleOCRQualityReport
	snapshot *publishedCanonicalOCRSnapshot
}

func (h AssetHandler) PreviewOCRDictionaryReevaluation(c *gin.Context) {
	var input ocrDictionaryReevaluationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	state, err := buildOCRDictionaryReevaluationPreview(h.db, input)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, state.preview)
}

func (h AssetHandler) ApplyOCRDictionaryReevaluation(c *gin.Context) {
	var input ocrDictionaryReevaluationApplyInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(input.PreviewID) == "" || strings.TrimSpace(input.ArtifactRevision) == "" || strings.TrimSpace(input.DictionaryRevision) == "" || len(input.AcceptedCueIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "preview, artifact, dictionary, and accepted cue bindings are required"})
		return
	}
	state, err := buildOCRDictionaryReevaluationPreview(h.db, input.previewInput())
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if state.preview.PreviewID != input.PreviewID || state.preview.ArtifactRevision != input.ArtifactRevision || state.preview.DictionaryRevision != input.DictionaryRevision {
		c.JSON(http.StatusConflict, gin.H{"error": "OCR dictionary preview is stale; evaluate the artifact again"})
		return
	}
	accepted := make(map[int]struct{}, len(input.AcceptedCueIDs))
	for _, cueID := range input.AcceptedCueIDs {
		accepted[cueID] = struct{}{}
	}
	corrections := make([]subtitleOCRCorrection, 0, len(accepted))
	for _, change := range state.preview.Changes {
		if _, ok := accepted[change.CueID]; ok {
			corrections = append(corrections, subtitleOCRCorrection{CueID: change.CueID, Action: "apply", Suggestion: change.After})
			delete(accepted, change.CueID)
		}
	}
	if len(accepted) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "accepted cue selection contains a cue without a validated dictionary candidate"})
		return
	}
	if len(corrections) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no validated dictionary candidates were accepted"})
		return
	}
	paths := subtitleOCRPaths(input.SidecarPath, input.SidecarPaths)
	correction := subtitleOCRCorrectionInput{
		MediaPath: input.MediaPath, SidecarPath: input.SidecarPath, SidecarPaths: paths,
		Format: input.Format, LogicalTransformKey: input.LogicalTransformKey,
		BaselineRevision: state.report.BaselineRevision, Revisions: state.report.RevisionSet,
		DictionaryLanguage: state.preview.DictionaryLanguage, DictionaryRevision: state.preview.DictionaryRevision,
		Corrections: corrections,
	}
	if len(correction.Revisions) == 0 {
		correction.Revisions = []publishedSubtitleRevision{state.report.Revision}
	}
	if _, err := validateSubtitleQualityCorrectionInput(correction); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	operationID, err := startSubtitleOCRCorrectionOperation(correction, h.db)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"operationId": operationID, "status": "running", "phase": "preparing", "progress": 1})
}

func buildOCRDictionaryReevaluationPreview(db *gorm.DB, input ocrDictionaryReevaluationInput) (ocrDictionaryReevaluationState, error) {
	ocrDictionaryRevisionMu.RLock()
	defer ocrDictionaryRevisionMu.RUnlock()
	paths, err := validateSubtitleOCRSidecarPaths(input.MediaPath, subtitleOCRPaths(input.SidecarPath, input.SidecarPaths))
	if err != nil {
		return ocrDictionaryReevaluationState{}, err
	}
	if len(paths) == 0 {
		return ocrDictionaryReevaluationState{}, errors.New("OCR dictionary re-evaluation requires a published subtitle sidecar")
	}
	format := strings.ToLower(strings.TrimSpace(input.Format))
	if format == "" || format == "both" {
		if format == "" {
			format = strings.TrimPrefix(strings.ToLower(filepath.Ext(paths[0])), ".")
		}
	}
	var snapshots []*publishedCanonicalOCRSnapshot
	var report subtitleOCRQualityReport
	if format == "both" {
		var formats []string
		snapshots, formats, err = loadCanonicalOCRBothSnapshots(paths, input.LogicalTransformKey)
		if err == nil {
			for index, snapshot := range snapshots {
				if !snapshot.CanCorrectLegacy(formats[index], len(snapshots)) {
					err = errors.New("canonical structural evidence is unavailable for one representation")
					break
				}
			}
		}
		if err == nil {
			err = attachOCRDictionarySnapshot(snapshots[0], db, input.DictionaryLanguage)
		}
		if err == nil {
			report = analyzeCanonicalOCRSnapshot(snapshots[0])
			report.Format = "both"
			report.RevisionSet = make([]publishedSubtitleRevision, len(snapshots))
			for index, snapshot := range snapshots {
				report.RevisionSet[index] = snapshot.Revision
			}
			report.BaselineRevision = subtitleRevisionSetKey(report.RevisionSet)
		}
	} else if format == "srt" || format == "ass" {
		if len(paths) != 1 {
			return ocrDictionaryReevaluationState{}, errors.New("single-format re-evaluation requires exactly one sidecar")
		}
		var snapshot *publishedCanonicalOCRSnapshot
		snapshot, ok := loadOrReconstructPublishedOCRSnapshot(paths[0], format, input.LogicalTransformKey)
		if !ok {
			return ocrDictionaryReevaluationState{}, errors.New("published canonical OCR baseline is unavailable; re-evaluation cannot run bitmap OCR")
		}
		if err := attachOCRDictionarySnapshot(snapshot, db, input.DictionaryLanguage); err != nil {
			return ocrDictionaryReevaluationState{}, err
		}
		report = analyzeCanonicalOCRSnapshot(snapshot)
		snapshots = []*publishedCanonicalOCRSnapshot{snapshot}
	} else {
		return ocrDictionaryReevaluationState{}, errors.New("re-evaluation supports SRT, ASS, or logical both artifacts")
	}
	if err != nil {
		return ocrDictionaryReevaluationState{}, err
	}
	if len(snapshots) == 0 || snapshots[0] == nil {
		return ocrDictionaryReevaluationState{}, errors.New("published canonical OCR baseline is unavailable")
	}
	if report.BaselineRevision == "" {
		report.BaselineRevision = subtitleRevisionSetKey(report.RevisionSet)
	}
	changes := make([]ocrDictionaryReevaluationChange, 0)
	corrections := make([]subtitleOCRCorrection, 0)
	for _, finding := range report.Findings {
		if finding.CandidateSource != "contextual-ocr-dictionary" || finding.Suggestion == "" {
			continue
		}
		cue, ok := canonicalCueByOrdinal(snapshots[0].Evidence.Cues, finding.CueID)
		if !ok {
			continue
		}
		changes = append(changes, ocrDictionaryReevaluationChange{
			CueID: finding.CueID, Before: cue.Text, After: finding.Suggestion,
			RuleIDs: append([]string(nil), finding.DictionaryRuleIDs...), RuleVersions: append([]int(nil), finding.DictionaryRuleVersions...),
			BeforeTextHash: boundedTextHash(cue.Text), AfterTextHash: boundedTextHash(finding.Suggestion),
		})
		corrections = append(corrections, subtitleOCRCorrection{CueID: finding.CueID, Action: "apply", Suggestion: finding.Suggestion})
	}
	if len(corrections) > 0 {
		previewSnapshot := *snapshots[0]
		previewSnapshot.Evidence = *cloneCanonicalOCREvidence(&snapshots[0].Evidence)
		corrected, applyErr := applySubtitleOCRCorrections(previewSnapshot.Evidence.Cues, corrections)
		if applyErr != nil {
			return ocrDictionaryReevaluationState{}, applyErr
		}
		previewSnapshot.Evidence.Cues = corrected
		updated := analyzeCanonicalOCRSnapshot(&previewSnapshot)
		report.CleanCount = updated.CleanCount
		report.SuspiciousCount = updated.SuspiciousCount
		report.ReviewCount = updated.ReviewCount
	}
	preview := ocrDictionaryReevaluationPreview{
		ArtifactRevision:         report.BaselineRevision,
		Revisions:                append([]publishedSubtitleRevision(nil), report.RevisionSet...),
		DictionaryLanguage:       report.DictionaryLanguage,
		DictionaryLanguageSource: snapshots[0].DictionaryLanguageSource,
		DictionaryRevision:       report.DictionaryRevision,
		CleanCount:               report.CleanCount, SuspiciousCount: report.SuspiciousCount, ReviewCount: report.ReviewCount,
		RequiresLanguageChoice: snapshots[0].Evidence.DictionaryLanguage == "" && report.DictionaryLanguage == "",
		Changes:                changes,
	}
	preview.PreviewID = ocrDictionaryPreviewID(preview, input)
	return ocrDictionaryReevaluationState{preview: preview, report: report, snapshot: snapshots[0]}, nil
}

func ocrDictionaryPreviewID(preview ocrDictionaryReevaluationPreview, input ocrDictionaryReevaluationInput) string {
	cueIDs := make([]int, 0, len(preview.Changes))
	for _, change := range preview.Changes {
		cueIDs = append(cueIDs, change.CueID)
	}
	sort.Ints(cueIDs)
	value := struct {
		ArtifactRevision   string                            `json:"artifactRevision"`
		DictionaryRevision string                            `json:"dictionaryRevision"`
		Language           string                            `json:"language"`
		Format             string                            `json:"format"`
		LogicalTransform   string                            `json:"logicalTransform"`
		CueIDs             []int                             `json:"cueIds"`
		Changes            []ocrDictionaryReevaluationChange `json:"changes"`
	}{preview.ArtifactRevision, preview.DictionaryRevision, preview.DictionaryLanguage, strings.ToLower(strings.TrimSpace(input.Format)), strings.TrimSpace(input.LogicalTransformKey), cueIDs, preview.Changes}
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}
