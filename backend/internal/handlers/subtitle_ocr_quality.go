package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/config"
	"github.com/gin-gonic/gin"
)

const (
	subtitleOCRQualityVersion       = "quality-v1"
	subtitleOCRRepairVersion        = "repair-v1"
	maxPublishedSubtitleBytes       = 64 * 1024 * 1024
	subtitleOCRQualityStatusClean   = "clean"
	subtitleOCRQualityStatusSuspect = "suspicious"
	subtitleOCRQualityStatusReview  = "review"
)

var subtitleOCRSnapshotMu sync.Mutex

type publishedSubtitleRevision struct {
	ArtifactPath       string `json:"artifactPath"`
	ArtifactIdentity   string `json:"artifactIdentity"`
	SizeBytes          int64  `json:"sizeBytes"`
	ModifiedAtUnixNano int64  `json:"modifiedAtUnixNano"`
	ContentHash        string `json:"contentHash"`
	LogicalTransform   string `json:"logicalTransform,omitempty"`
}

func (revision publishedSubtitleRevision) key() string {
	encoded, _ := json.Marshal(revision)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

type publishedCanonicalOCRSnapshot struct {
	Version             int                       `json:"version"`
	OwnerIdentity       string                    `json:"ownerIdentity"`
	SourcePath          string                    `json:"sourcePath"`
	Format              string                    `json:"format"`
	LogicalTransformKey string                    `json:"logicalTransformKey,omitempty"`
	Revision            publishedSubtitleRevision `json:"revision"`
	Evidence            canonicalOCREvidence      `json:"evidence"`
	CreatedAt           time.Time                 `json:"createdAt"`
	UpdatedAt           time.Time                 `json:"updatedAt"`
	AnalyzerVersion     string                    `json:"analyzerVersion"`
	RepairVersion       string                    `json:"repairVersion"`
	ScoringVersion      string                    `json:"scoringVersion"`
	PreviousRevision    string                    `json:"previousRevision,omitempty"`
	CorrectionOperation string                    `json:"correctionOperation,omitempty"`
	Integrity           string                    `json:"integrity"`
}

type subtitleOCRQualityFinding struct {
	CueID           int      `json:"cueId"`
	OriginalText    string   `json:"originalText"`
	Suggestion      string   `json:"suggestion,omitempty"`
	Status          string   `json:"status"`
	Reasons         []string `json:"reasons,omitempty"`
	Evidence        []string `json:"evidence,omitempty"`
	CandidateSource string   `json:"candidateSource,omitempty"`
	Baseline        string   `json:"baselineRevision"`
}

type subtitleOCRQualityReport struct {
	ArtifactPath        string                      `json:"artifactPath"`
	Format              string                      `json:"format"`
	LogicalTransformKey string                      `json:"logicalTransformKey,omitempty"`
	Revision            publishedSubtitleRevision   `json:"revision"`
	RevisionSet         []publishedSubtitleRevision `json:"revisionSet,omitempty"`
	BaselineRevision    string                      `json:"baselineRevision"`
	AnalyzerVersion     string                      `json:"analyzerVersion"`
	RepairVersion       string                      `json:"repairVersion"`
	CleanCount          int                         `json:"cleanCount"`
	SuspiciousCount     int                         `json:"suspiciousCount"`
	ReviewCount         int                         `json:"reviewCount"`
	LegacyTextOnly      bool                        `json:"legacyTextOnly,omitempty"`
	CanCorrect          bool                        `json:"canCorrect"`
	Findings            []subtitleOCRQualityFinding `json:"findings"`
}

type subtitleOCRCorrection struct {
	CueID      int    `json:"cueId"`
	Action     string `json:"action"`
	Text       string `json:"text,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
}

const maxSelectiveOCRCueDurationSeconds = 30

type selectiveOCRCueCapability struct {
	Available  bool
	Reason     string
	SourcePath string
	Stream     FFProbeStream
	Start      float64
	Duration   int
}

const maxContextualOCRBatchCues = 16

type subtitleOCRContextualCue struct {
	CueID        int
	Text         string
	ReviewReason string
}

type subtitleOCRContextualSuggestion struct {
	CueID int
	Text  string
}

// subtitleOCRContextualProvider is only the bounded text-suggestion boundary.
// The repository has no provider implementation; callers may leave it nil.
// It cannot return timing, geometry, styles, or any other subtitle structure.
type subtitleOCRContextualProvider interface {
	Suggest(context.Context, []subtitleOCRContextualCue) ([]subtitleOCRContextualSuggestion, error)
}

func boundedContextualOCRSuggestions(ctx context.Context, provider subtitleOCRContextualProvider, findings []subtitleOCRQualityFinding, language string) ([]subtitleOCRContextualSuggestion, error) {
	if provider == nil {
		return nil, nil
	}
	cues := make([]subtitleOCRContextualCue, 0, maxContextualOCRBatchCues)
	for _, finding := range findings {
		if finding.Status == subtitleOCRQualityStatusClean || len(cues) >= maxContextualOCRBatchCues || strings.TrimSpace(finding.OriginalText) == "" || len(finding.OriginalText) > 4*1024 {
			continue
		}
		cues = append(cues, subtitleOCRContextualCue{CueID: finding.CueID, Text: finding.OriginalText, ReviewReason: strings.Join(finding.Reasons, "; ")})
	}
	if len(cues) == 0 {
		return nil, nil
	}
	suggestions, err := provider.Suggest(ctx, cues)
	if err != nil {
		return nil, err
	}
	valid := make([]subtitleOCRContextualSuggestion, 0, len(suggestions))
	seen := make(map[int]struct{}, len(suggestions))
	for _, suggestion := range suggestions {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, duplicate := seen[suggestion.CueID]; duplicate || strings.TrimSpace(suggestion.Text) == "" || len(suggestion.Text) > 4*1024 || strings.ContainsAny(suggestion.Text, "\r\n") {
			continue
		}
		for _, cue := range cues {
			if cue.CueID == suggestion.CueID && subtitleOCRCandidateIsMateriallyBetter(cue.Text, suggestion.Text, language) {
				valid = append(valid, subtitleOCRContextualSuggestion{CueID: suggestion.CueID, Text: strings.TrimSpace(suggestion.Text)})
				seen[suggestion.CueID] = struct{}{}
				break
			}
		}
	}
	return valid, nil
}

func subtitleOCRCandidateIsMateriallyBetter(original, candidate, language string) bool {
	if strings.TrimSpace(original) == "" || strings.TrimSpace(candidate) == "" || strings.ContainsAny(candidate, "\r\n") {
		return false
	}
	originalScore := subtitleOCRScoreForLanguage(original, language)
	candidateScore := subtitleOCRScoreForLanguage(candidate, language)
	return candidateScore > originalScore+math.Max(5, math.Abs(originalScore)*0.05)
}

type subtitleOCRAnalyzeInput struct {
	MediaPath           string   `json:"mediaPath,omitempty"`
	SidecarPath         string   `json:"sidecarPath"`
	SidecarPaths        []string `json:"sidecarPaths,omitempty"`
	Format              string   `json:"format"`
	LogicalTransformKey string   `json:"logicalTransformKey,omitempty"`
}

type subtitleOCRCorrectionInput struct {
	MediaPath           string                      `json:"mediaPath,omitempty"`
	SidecarPath         string                      `json:"sidecarPath"`
	SidecarPaths        []string                    `json:"sidecarPaths,omitempty"`
	Format              string                      `json:"format"`
	LogicalTransformKey string                      `json:"logicalTransformKey,omitempty"`
	BaselineRevision    string                      `json:"baselineRevision"`
	Revisions           []publishedSubtitleRevision `json:"revisions,omitempty"`
	Corrections         []subtitleOCRCorrection     `json:"corrections"`
}

func subtitleOCRPaths(inputSidecarPath string, inputPaths []string) []string {
	paths := append([]string(nil), inputPaths...)
	if strings.TrimSpace(inputSidecarPath) != "" && len(paths) == 0 {
		paths = []string{inputSidecarPath}
	}
	for index := range paths {
		paths[index] = filepath.Clean(strings.TrimSpace(paths[index]))
	}
	return paths
}

func subtitleRevisionSetKey(revisions []publishedSubtitleRevision) string {
	if len(revisions) == 1 {
		return revisions[0].key()
	}
	encoded, _ := json.Marshal(revisions)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func validateSubtitleOCRSidecarPaths(mediaPath string, paths []string) ([]string, error) {
	mediaPath = filepath.Clean(strings.TrimSpace(mediaPath))
	if mediaPath == "." || mediaPath == "" {
		return paths, nil
	}
	validated := make([]string, len(paths))
	for index, path := range paths {
		value, err := validatedExternalSubtitlePath(mediaPath, path)
		if err != nil {
			return nil, err
		}
		validated[index] = value
	}
	return validated, nil
}

func loadOrReconstructPublishedOCRSnapshot(path, format, logicalTransform string) (*publishedCanonicalOCRSnapshot, bool) {
	if snapshot, ok := loadPublishedCanonicalOCRSnapshot(path, format, logicalTransform); ok {
		return snapshot, true
	}
	owner := publishedSubtitleOwnerIdentity(path, format, logicalTransform)
	if _, err := os.Stat(publishedSubtitleSnapshotPath(owner)); err == nil {
		// An owned snapshot that no longer matches its artifact/source is stale;
		// do not silently downgrade it to a legacy reconstruction.
		return nil, false
	}
	return reconstructLegacyOCRSnapshot(path, format, logicalTransform)
}

func analyzePublishedOCRQuality(path, format, logicalTransform string) (subtitleOCRQualityReport, error) {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	}
	if format != "srt" && format != "ass" {
		return subtitleOCRQualityReport{}, fmt.Errorf("OCR quality analysis supports SRT or ASS artifacts")
	}
	snapshot, ok := loadOrReconstructPublishedOCRSnapshot(path, format, logicalTransform)
	if !ok {
		return subtitleOCRQualityReport{}, fmt.Errorf("published OCR canonical baseline is unavailable; regenerate through the canonical subtitle pipeline")
	}
	return analyzeCanonicalOCRSnapshot(snapshot), nil
}

func applySubtitleOCRCorrections(cues []canonicalBitmapSubtitleCue, corrections []subtitleOCRCorrection) ([]canonicalBitmapSubtitleCue, error) {
	result := cloneCanonicalCues(cues)
	byID := make(map[int]int, len(result))
	for index, cue := range result {
		byID[cue.SourceOrdinal] = index
	}
	for _, correction := range corrections {
		index, ok := byID[correction.CueID]
		if !ok {
			return nil, fmt.Errorf("OCR correction references unknown cue %d", correction.CueID)
		}
		action := strings.ToLower(strings.TrimSpace(correction.Action))
		if action == "keep" || action == "reject" {
			continue
		}
		text := strings.TrimSpace(correction.Text)
		if action == "apply" {
			text = strings.TrimSpace(correction.Suggestion)
		}
		if text == "" {
			return nil, fmt.Errorf("OCR correction for cue %d has no text", correction.CueID)
		}
		result[index].Text = text
	}
	return result, nil
}

func validateDeterministicOCRCorrectionIntents(snapshot *publishedCanonicalOCRSnapshot, corrections []subtitleOCRCorrection) error {
	if snapshot == nil {
		return errors.New("canonical OCR baseline is required")
	}
	report := analyzeCanonicalOCRSnapshot(snapshot)
	findings := make(map[int]subtitleOCRQualityFinding, len(report.Findings))
	for _, finding := range report.Findings {
		findings[finding.CueID] = finding
	}
	for _, correction := range corrections {
		finding, ok := findings[correction.CueID]
		if !ok {
			return fmt.Errorf("OCR correction references unknown cue %d", correction.CueID)
		}
		switch strings.ToLower(strings.TrimSpace(correction.Action)) {
		case "keep", "reject":
			continue
		case "edit":
			if strings.TrimSpace(correction.Text) == "" {
				return fmt.Errorf("manual OCR correction for cue %d has no text", correction.CueID)
			}
		case "apply":
			expected := strings.TrimSpace(finding.Suggestion)
			candidate := strings.TrimSpace(correction.Suggestion)
			if finding.CandidateSource != "deterministic-confusion-repair" || expected == "" || candidate != expected || strings.TrimSpace(correction.Text) != "" {
				return fmt.Errorf("OCR correction for cue %d is not the current deterministic suggestion", correction.CueID)
			}
		default:
			return fmt.Errorf("unsupported OCR correction action %q", correction.Action)
		}
	}
	return nil
}

func correctionProvenanceFor(snapshot *publishedCanonicalOCRSnapshot, corrections []subtitleOCRCorrection) []canonicalOCRCorrectionProvenance {
	if snapshot == nil || len(corrections) == 0 {
		return nil
	}
	report := analyzeCanonicalOCRSnapshot(snapshot)
	sources := make(map[int]string, len(report.Findings))
	for _, finding := range report.Findings {
		sources[finding.CueID] = finding.CandidateSource
	}
	provenance := make([]canonicalOCRCorrectionProvenance, 0, len(corrections))
	for _, correction := range corrections {
		if len(provenance) >= 128 {
			break
		}
		text := strings.TrimSpace(correction.Text)
		if strings.EqualFold(strings.TrimSpace(correction.Action), "apply") {
			text = strings.TrimSpace(correction.Suggestion)
		}
		textHash := ""
		if text != "" {
			sum := sha256.Sum256([]byte(text))
			textHash = hex.EncodeToString(sum[:])
		}
		selectedBy := "user"
		validation := "manual-text"
		if strings.EqualFold(strings.TrimSpace(correction.Action), "apply") {
			selectedBy = "deterministic-analyzer"
			validation = "deterministic-suggestion"
		}
		provenance = append(provenance, canonicalOCRCorrectionProvenance{
			CueID: correction.CueID, Action: strings.ToLower(strings.TrimSpace(correction.Action)), CandidateSource: sources[correction.CueID], TextHash: textHash, Validation: validation, SelectedBy: selectedBy,
		})
	}
	return provenance
}

func selectiveOCRCueCapabilityFor(snapshot *publishedCanonicalOCRSnapshot, cue canonicalBitmapSubtitleCue) selectiveOCRCueCapability {
	if snapshot == nil {
		return selectiveOCRCueCapability{Reason: "canonical OCR baseline is unavailable"}
	}
	identity := snapshot.Evidence.Identity
	if !canonicalOCRSourceIdentityMatches(identity) {
		return selectiveOCRCueCapability{Reason: "source identity is unavailable or changed"}
	}
	codec := strings.ToLower(strings.TrimSpace(identity.StreamCodec))
	if codec != "dvd_subtitle" && codec != "hdmv_pgs_subtitle" && codec != "pgssub" {
		return selectiveOCRCueCapability{Reason: "bounded selective OCR input is unsupported for this codec"}
	}
	if cue.End <= cue.Start || cue.Start < 0 || cue.End-cue.Start > maxSelectiveOCRCueDurationSeconds {
		return selectiveOCRCueCapability{Reason: "cue timing is outside the bounded selective OCR window"}
	}
	start := cue.Start
	if identity.PreparationScope == "segmented-test-encode" {
		segmentStart, err := strconv.ParseFloat(identity.SegmentStart, 64)
		if err != nil || segmentStart < 0 {
			return selectiveOCRCueCapability{Reason: "segmented OCR timing provenance is unavailable"}
		}
		start += segmentStart
	}
	duration := int(math.Ceil(cue.End - cue.Start))
	if duration < 1 {
		duration = 1
	}
	if duration > maxSelectiveOCRCueDurationSeconds {
		duration = maxSelectiveOCRCueDurationSeconds
	}
	return selectiveOCRCueCapability{
		Available:  true,
		SourcePath: identity.SourcePath,
		Stream:     FFProbeStream{Index: identity.StreamIndex, CodecName: identity.StreamCodec, ID: identity.StreamID},
		Start:      start,
		Duration:   duration,
	}
}

func runSelectiveSecondaryOCR(ctx context.Context, snapshot *publishedCanonicalOCRSnapshot, cue canonicalBitmapSubtitleCue, workspace string, onProgress OCRProgressFunc) (string, string, error) {
	capability := selectiveOCRCueCapabilityFor(snapshot, cue)
	if !capability.Available {
		return "", capability.Reason, nil
	}
	segmentPath := filepath.Join(workspace, fmt.Sprintf("selective-cue-%d.mkv", cue.SourceOrdinal))
	args := bitmapSubtitleSegmentArgs(capability.SourcePath, capability.Stream.Index, segmentPath, capability.Start, capability.Duration)
	var output strings.Builder
	if err := runSubtitleCommand(ctx, "ffmpeg", args, &output, &output); err != nil {
		return "", "bounded cue extraction failed", err
	}
	preparedStreams, err := probeSubtitleStreams(ctx, segmentPath)
	if err != nil || len(preparedStreams) != 1 {
		return "", "prepared subtitle stream identity unavailable or ambiguous", nil
	}
	preparedStream := preparedStreams[0]
	if preparedStream.CodecType != "subtitle" || !isBitmapSubtitleCodecName(preparedStream.CodecName) {
		return "", "prepared subtitle stream is not a supported bitmap representation", nil
	}
	result, err := runBitmapOCRWithTiming(
		ctx,
		segmentPath,
		preparedStream,
		"srt",
		snapshot.Evidence.RequestedLanguage,
		snapshot.Evidence.OCRMode,
		filepath.Join(workspace, fmt.Sprintf("selective-cue-%d.srt", cue.SourceOrdinal)),
		filepath.Join(workspace, fmt.Sprintf("selective-cue-%d.raw.srt", cue.SourceOrdinal)),
		onProgress,
		true,
		capability.Start,
	)
	if err != nil {
		return "", "selective OCR failed", err
	}
	for _, candidate := range result.Cues {
		text := strings.TrimSpace(candidate.Text)
		if text != "" && subtitleOCRScoreForLanguage(text, snapshot.Evidence.RequestedLanguage) > subtitleOCRScoreForLanguage(cue.Text, snapshot.Evidence.RequestedLanguage) {
			return text, "selective-secondary-ocr", nil
		}
	}
	return "", "selective OCR produced no materially better candidate", nil
}

func loadCanonicalOCRBothSnapshots(paths []string, logicalTransform string) ([]*publishedCanonicalOCRSnapshot, []string, error) {
	if len(paths) != 2 || strings.TrimSpace(logicalTransform) == "" {
		return nil, nil, errors.New("canonical both OCR requires SRT and ASS sidecars with a logical transform")
	}
	formats := make([]string, len(paths))
	seen := make(map[string]bool, len(paths))
	for index, path := range paths {
		format := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
		if format != "srt" && format != "ass" || seen[format] {
			return nil, nil, errors.New("canonical both OCR requires one SRT and one ASS sidecar")
		}
		seen[format] = true
		formats[index] = format
	}
	if !seen["srt"] || !seen["ass"] {
		return nil, nil, errors.New("canonical both OCR requires one SRT and one ASS sidecar")
	}
	snapshots := make([]*publishedCanonicalOCRSnapshot, len(paths))
	for index, path := range paths {
		snapshot, ok := loadOrReconstructPublishedOCRSnapshot(path, formats[index], logicalTransform)
		if !ok || snapshot == nil || snapshot.LogicalTransformKey != logicalTransform {
			return nil, nil, errors.New("canonical both OCR evidence is unavailable or stale")
		}
		snapshots[index] = snapshot
	}
	if !canonicalOCRIdentityMatches(snapshots[0].Evidence.Identity, snapshots[1].Evidence.Identity) ||
		!reflect.DeepEqual(snapshots[0].Evidence.Cues, snapshots[1].Evidence.Cues) {
		return nil, nil, errors.New("canonical both OCR evidence is inconsistent")
	}
	return snapshots, formats, nil
}

func analyzeCanonicalOCRBoth(paths []string, logicalTransform string) (subtitleOCRQualityReport, []*publishedCanonicalOCRSnapshot, error) {
	snapshots, formats, err := loadCanonicalOCRBothSnapshots(paths, logicalTransform)
	if err != nil {
		return subtitleOCRQualityReport{}, nil, err
	}
	for index, snapshot := range snapshots {
		if !snapshot.CanCorrectLegacy(formats[index], len(snapshots)) {
			return subtitleOCRQualityReport{}, nil, errors.New("canonical both OCR structural evidence is unavailable")
		}
	}
	report := analyzeCanonicalOCRSnapshot(snapshots[0])
	report.Format = "both"
	report.RevisionSet = make([]publishedSubtitleRevision, len(snapshots))
	for index, snapshot := range snapshots {
		report.RevisionSet[index] = snapshot.Revision
	}
	report.BaselineRevision = subtitleRevisionSetKey(report.RevisionSet)
	return report, snapshots, nil
}

func correctedSubtitleEvidence(snapshot *publishedCanonicalOCRSnapshot, cues []canonicalBitmapSubtitleCue) *canonicalOCREvidence {
	evidence := cloneCanonicalOCREvidence(&snapshot.Evidence)
	evidence.Cues = cues
	return evidence
}

// correctedCanonicalOCREvidenceForTranslation applies only the same bounded
// deterministic suggestions that the OCR repair workflow permits to be safely
// applied. Review-only and provider/selective candidates remain reviewable and
// are never silently applied while preparing a translation source.
func correctedCanonicalOCREvidenceForTranslation(evidence *canonicalOCREvidence) (*canonicalOCREvidence, error) {
	if evidence == nil {
		return nil, errors.New("canonical OCR evidence is required")
	}
	working := cloneCanonicalOCREvidence(evidence)
	snapshot := &publishedCanonicalOCRSnapshot{Evidence: *working}
	report := analyzeCanonicalOCRSnapshot(snapshot)
	corrections := make([]subtitleOCRCorrection, 0)
	for _, finding := range report.Findings {
		if finding.CandidateSource == "deterministic-confusion-repair" && strings.TrimSpace(finding.Suggestion) != "" {
			corrections = append(corrections, subtitleOCRCorrection{
				CueID:      finding.CueID,
				Action:     "apply",
				Suggestion: finding.Suggestion,
			})
		}
	}
	if len(corrections) == 0 {
		return working, nil
	}
	if err := validateDeterministicOCRCorrectionIntents(snapshot, corrections); err != nil {
		return nil, err
	}
	cues, err := applySubtitleOCRCorrections(working.Cues, corrections)
	if err != nil {
		return nil, err
	}
	working.Cues = cues
	working.CorrectionProvenance = correctionProvenanceFor(snapshot, corrections)
	return working, nil
}

// prepareCanonicalOCREvidenceForTranslation completes the same bounded OCR
// quality path used by #39 before translation sees any cues. Deterministic
// repairs have already been applied by correctedCanonicalOCREvidenceForTranslation;
// remaining findings may use only the existing bounded selective OCR seam.
func prepareCanonicalOCREvidenceForTranslation(ctx context.Context, evidence *canonicalOCREvidence, onProgress OCRProgressFunc) (*canonicalOCREvidence, error) {
	if ctx == nil {
		return nil, subtitleOCRFailedError(errors.New("canonical OCR translation preparation requires a context"))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if evidence == nil || !evidence.Complete || len(evidence.Cues) == 0 {
		return nil, subtitleOCRFailedError(errors.New("canonical OCR evidence is incomplete for translation"))
	}

	working := cloneCanonicalOCREvidence(evidence)
	snapshot := &publishedCanonicalOCRSnapshot{Evidence: *working}
	report := analyzeCanonicalOCRSnapshot(snapshot)
	needsSelective := false
	for _, finding := range report.Findings {
		if finding.Status != subtitleOCRQualityStatusClean {
			needsSelective = true
			break
		}
	}
	if !needsSelective {
		return working, nil
	}

	root := canonicalOCRWorkspaceRoot()
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, subtitleOCRFailedError(fmt.Errorf("cannot create selective OCR workspace: %w", err))
	}
	workspace, err := os.MkdirTemp(root, ".mvforge-translation-selective-*")
	if err != nil {
		return nil, subtitleOCRFailedError(fmt.Errorf("cannot create selective OCR workspace: %w", err))
	}
	defer cleanupSubtitleOperationWorkspace(workspace)

	byID := make(map[int]canonicalBitmapSubtitleCue, len(working.Cues))
	for _, cue := range working.Cues {
		byID[cue.SourceOrdinal] = cue
	}
	for _, finding := range report.Findings {
		if finding.Status == subtitleOCRQualityStatusClean {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cue, ok := byID[finding.CueID]
		if !ok {
			return nil, subtitleOCRFailedError(fmt.Errorf("canonical OCR cue %d cannot be safely translated", finding.CueID))
		}
		capability := selectiveOCRCueCapabilityFor(snapshot, cue)
		if !capability.Available {
			return nil, subtitleOCRReviewRequiredError(fmt.Errorf("canonical OCR cue %d requires review before translation: %s", finding.CueID, capability.Reason))
		}
		candidate, source, err := runSelectiveSecondaryOCR(ctx, snapshot, cue, workspace, onProgress)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			return nil, subtitleOCRFailedError(fmt.Errorf("canonical OCR cue %d selective correction failed: %w", finding.CueID, err))
		}
		candidate = strings.TrimSpace(candidate)
		if source != "selective-secondary-ocr" || len(candidate) > 4*1024 || strings.TrimSpace(candidate) == "" || strings.ContainsAny(candidate, "\r\n") || !subtitleOCRCandidateIsMateriallyBetter(cue.Text, candidate, snapshot.Evidence.RequestedLanguage) {
			return nil, subtitleOCRReviewRequiredError(fmt.Errorf("canonical OCR cue %d requires review before translation: selective candidate was not safely validated", finding.CueID))
		}
		corrected, err := applySubtitleOCRCorrections(working.Cues, []subtitleOCRCorrection{{CueID: finding.CueID, Action: "edit", Text: candidate}})
		if err != nil {
			return nil, subtitleOCRFailedError(fmt.Errorf("canonical OCR cue %d selective correction failed: %w", finding.CueID, err))
		}
		working.Cues = corrected
		working.CorrectionProvenance = append(working.CorrectionProvenance, selectiveOCRCorrectionProvenance(finding.CueID, candidate))
		byID[finding.CueID] = correctedCueByOrdinal(corrected, finding.CueID)
		snapshot.Evidence = *working
	}

	finalReport := analyzeCanonicalOCRSnapshot(snapshot)
	for _, finding := range finalReport.Findings {
		if finding.Status != subtitleOCRQualityStatusClean {
			return nil, subtitleOCRReviewRequiredError(fmt.Errorf("canonical OCR cue %d requires review before translation after selective correction", finding.CueID))
		}
	}
	return working, nil
}

func correctedCueByOrdinal(cues []canonicalBitmapSubtitleCue, ordinal int) canonicalBitmapSubtitleCue {
	for _, cue := range cues {
		if cue.SourceOrdinal == ordinal {
			return cue
		}
	}
	return canonicalBitmapSubtitleCue{SourceOrdinal: ordinal}
}

func selectiveOCRCorrectionProvenance(cueID int, text string) canonicalOCRCorrectionProvenance {
	sum := sha256.Sum256([]byte(text))
	return canonicalOCRCorrectionProvenance{
		CueID:           cueID,
		Action:          "apply",
		CandidateSource: "selective-secondary-ocr",
		TextHash:        hex.EncodeToString(sum[:]),
		Validation:      "deterministic-score",
		SelectedBy:      "canonical-ocr-quality",
	}
}

func validateSubtitleQualityCorrectionInput(input subtitleOCRCorrectionInput) ([]publishedSubtitleRevision, error) {
	paths := subtitleOCRPaths(input.SidecarPath, input.SidecarPaths)
	if len(paths) == 0 {
		return nil, errors.New("OCR correction requires a published sidecar path")
	}
	if strings.TrimSpace(input.BaselineRevision) == "" {
		return nil, errors.New("OCR correction requires the revision returned by Analyze OCR")
	}
	revisions := append([]publishedSubtitleRevision(nil), input.Revisions...)
	if len(revisions) != len(paths) {
		return nil, errors.New("OCR correction revision set does not match the requested sidecars")
	}
	if subtitleRevisionSetKey(revisions) != input.BaselineRevision {
		return nil, errors.New("stale OCR analysis revision")
	}
	for index, path := range paths {
		if revisions[index].ArtifactPath != path {
			return nil, errors.New("OCR correction revision identity does not match its sidecar path")
		}
	}
	return revisions, nil
}

func writeCorrectedSubtitleFiles(workspace string, paths []string, formats []string, cues []canonicalBitmapSubtitleCue) ([]string, error) {
	staged := make([]string, 0, len(paths))
	for index, path := range paths {
		format := strings.ToLower(strings.TrimSpace(formats[index]))
		if format == "ass" && len(cues) == 0 {
			return nil, errors.New("corrected ASS canonical cues are unavailable")
		}
		var content []byte
		if format == "srt" {
			content = renderBitmapSRTCues(cues)
		} else if format == "ass" {
			content = renderBitmapASSCues(cues)
		} else {
			return nil, fmt.Errorf("unsupported OCR correction format %q", format)
		}
		if !validSubtitleSidecar(format, content) {
			return nil, fmt.Errorf("corrected %s sidecar failed validation", format)
		}
		stage := filepath.Join(workspace, fmt.Sprintf("output-%d.%s", index, format))
		if err := os.WriteFile(stage, content, 0o644); err != nil {
			return nil, err
		}
		staged = append(staged, stage)
		_ = path
	}
	return staged, nil
}

func publishCorrectedSubtitleFiles(
	ctx context.Context,
	operationID string,
	paths, formats, staged []string,
	snapshots []*publishedCanonicalOCRSnapshot,
	oldRevisions []publishedSubtitleRevision,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(paths) == 0 || len(paths) != len(staged) || len(paths) != len(snapshots) {
		return errors.New("invalid OCR correction publication set")
	}
	subtitlePublicationMu.Lock()
	defer subtitlePublicationMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	for index, revision := range oldRevisions {
		current, _, err := publishedSubtitleRevisionFor(paths[index], revision.LogicalTransform)
		if err != nil || current != revision {
			return errors.New("stale OCR analysis revision")
		}
	}
	oldContent := make([][]byte, len(paths))
	oldSnapshot := make([][]byte, len(paths))
	for index, path := range paths {
		var err error
		oldContent[index], err = os.ReadFile(path)
		if err != nil {
			return err
		}
		owner := publishedSubtitleOwnerIdentity(path, formats[index], snapshots[index].LogicalTransformKey)
		oldSnapshot[index], _ = os.ReadFile(publishedSubtitleSnapshotPath(owner))
	}
	rollback := func() {
		rollbackRoot := filepath.Join(canonicalOCRWorkspaceRoot(), "rollback")
		_ = os.MkdirAll(rollbackRoot, 0o700)
		for index, path := range paths {
			private := filepath.Join(rollbackRoot, fmt.Sprintf("%s-%d", filepath.Base(path), index))
			if err := os.WriteFile(private, oldContent[index], 0o600); err == nil {
				_ = copyPublishedFile(private, path, true)
			}
			_ = os.Remove(private)
			owner := publishedSubtitleOwnerIdentity(path, formats[index], snapshots[index].LogicalTransformKey)
			if len(oldSnapshot[index]) == 0 {
				_ = os.Remove(publishedSubtitleSnapshotPath(owner))
			} else {
				_ = os.WriteFile(publishedSubtitleSnapshotPath(owner), oldSnapshot[index], 0o600)
			}
		}
	}
	for index, path := range paths {
		if err := ctx.Err(); err != nil {
			rollback()
			return err
		}
		if err := publishStagedSubtitleFile(staged[index], path); err != nil {
			rollback()
			return err
		}
	}
	for index, snapshot := range snapshots {
		previous := oldRevisions[index].key()
		// The complete output set has crossed the publication barrier. A later
		// cancellation must not roll back a committed publication.
		if err := commitPublishedCanonicalOCRSnapshot(context.Background(), paths[index], formats[index], snapshot.LogicalTransformKey, &snapshot.Evidence, operationID, previous); err != nil {
			rollback()
			return fmt.Errorf("cannot commit corrected OCR baseline: %w", err)
		}
	}
	return nil
}

func publishStagedSubtitleFile(staged, destination string) error {
	if err := os.Rename(staged, destination); err == nil {
		return nil
	}
	return copyPublishedFile(staged, destination, true)
}

func runSubtitleOCRCorrectionOperation(
	ctx context.Context,
	operationID, workspace string,
	input subtitleOCRCorrectionInput,
) {
	defer cleanupSubtitleOperationWorkspace(workspace)
	paths := subtitleOCRPaths(input.SidecarPath, input.SidecarPaths)
	oldRevisions, err := validateSubtitleQualityCorrectionInput(input)
	if err != nil {
		updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
			operation.Status = "error"
			operation.Phase = "error"
			operation.Message = "Stale OCR analysis"
			operation.Error = err.Error()
		})
		return
	}
	formats := make([]string, len(paths))
	for index, path := range paths {
		formats[index] = strings.ToLower(strings.TrimSpace(input.Format))
		if formats[index] == "both" || formats[index] == "" {
			formats[index] = strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
		}
		if formats[index] != "srt" && formats[index] != "ass" {
			updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
				operation.Status = "error"
				operation.Phase = "error"
				operation.Message = "OCR correction failed"
				operation.Error = "unsupported correction format"
			})
			return
		}
	}
	snapshots := make([]*publishedCanonicalOCRSnapshot, len(paths))
	if strings.EqualFold(strings.TrimSpace(input.Format), "both") {
		var loadErr error
		snapshots, formats, loadErr = loadCanonicalOCRBothSnapshots(paths, input.LogicalTransformKey)
		if loadErr != nil {
			updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
				operation.Status = "error"
				operation.Phase = "error"
				operation.Message = "OCR correction failed"
				operation.Error = loadErr.Error()
			})
			return
		}
	}
	if len(snapshots) == 0 || len(snapshots) != len(paths) {
		updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
			operation.Status = "error"
			operation.Phase = "error"
			operation.Message = "OCR correction failed"
			operation.Error = "canonical structural evidence is unavailable for this representation"
		})
		return
	}
	for index, path := range paths {
		if snapshots[index] == nil {
			snapshot, ok := loadOrReconstructPublishedOCRSnapshot(path, formats[index], input.LogicalTransformKey)
			if !ok {
				updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
					operation.Status = "error"
					operation.Phase = "error"
					operation.Message = "OCR correction failed"
					operation.Error = "canonical structural evidence is unavailable for this representation"
				})
				return
			}
			snapshots[index] = snapshot
		}
		if !snapshots[index].CanCorrectLegacy(formats[index], len(paths)) {
			updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
				operation.Status = "error"
				operation.Phase = "error"
				operation.Message = "OCR correction failed"
				operation.Error = "canonical structural evidence is unavailable for this representation"
			})
			return
		}
	}
	for _, snapshot := range snapshots {
		if err := validateDeterministicOCRCorrectionIntents(snapshot, input.Corrections); err != nil {
			updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
				operation.Status = "error"
				operation.Phase = "error"
				operation.Message = "OCR correction failed"
				operation.Error = err.Error()
			})
			return
		}
	}
	for _, snapshot := range snapshots {
		snapshot.Evidence.CorrectionProvenance = correctionProvenanceFor(snapshot, input.Corrections)
	}
	cues, err := applySubtitleOCRCorrections(snapshots[0].Evidence.Cues, input.Corrections)
	if err != nil {
		updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
			operation.Status = "error"
			operation.Phase = "error"
			operation.Message = "OCR correction failed"
			operation.Error = err.Error()
		})
		return
	}
	for index := range snapshots {
		snapshots[index].Evidence.Cues = cloneCanonicalCues(cues)
	}
	staged, err := writeCorrectedSubtitleFiles(workspace, paths, formats, cues)
	if err == nil {
		err = publishCorrectedSubtitleFiles(ctx, operationID, paths, formats, staged, snapshots, oldRevisions)
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
				operation.Status = "cancelled"
				operation.Phase = "cleanup"
				operation.Message = "OCR correction cancelled"
				operation.Error = ""
			})
		} else {
			updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
				operation.Status = "error"
				operation.Phase = "error"
				operation.Message = "OCR correction failed"
				operation.Error = err.Error()
			})
		}
		return
	}
	markSubtitlePublicationCommitted(operationID)
	updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
		operation.Status = "completed"
		operation.Phase = "completed"
		operation.Progress = 100
		operation.Message = "OCR correction completed"
	})
}

func runSubtitleOCRAnalysisOperation(ctx context.Context, operationID, workspace string, input subtitleOCRAnalyzeInput) {
	defer cleanupSubtitleOperationWorkspace(workspace)
	paths := subtitleOCRPaths(input.SidecarPath, input.SidecarPaths)
	if len(paths) == 0 {
		updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
			operation.Status = "error"
			operation.Phase = "error"
			operation.Message = "OCR analysis failed"
			operation.Error = "OCR analysis requires a published sidecar path"
		})
		return
	}
	if strings.EqualFold(strings.TrimSpace(input.Format), "both") {
		if err := ctx.Err(); err != nil {
			updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
				operation.Status = "cancelled"
				operation.Phase = "cancelled"
				operation.Message = "OCR analysis cancelled"
			})
			return
		}
		report, snapshots, err := analyzeCanonicalOCRBoth(paths, input.LogicalTransformKey)
		if err == nil && len(snapshots) > 0 {
			report = enrichSubtitleOCRReportWithSelective(ctx, workspace, snapshots[0], report)
		}
		if err != nil {
			updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
				operation.Status = "error"
				operation.Phase = "error"
				operation.Message = "OCR analysis failed"
				operation.Error = err.Error()
			})
			return
		}
		updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
			operation.Phase = "analyzing"
			operation.Progress = 100
			operation.Processed = len(paths)
			operation.Total = len(paths)
			operation.Message = "Analyzing OCR quality"
		})
		report.BaselineRevision = subtitleRevisionSetKey(report.RevisionSet)
		updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
			operation.Status = "completed"
			operation.Phase = "completed"
			operation.Progress = 100
			operation.QualityReport = &report
			operation.Message = "OCR analysis completed"
		})
		return
	}
	var combined subtitleOCRQualityReport
	for index, path := range paths {
		if err := ctx.Err(); err != nil {
			updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
				operation.Status = "cancelled"
				operation.Phase = "cancelled"
				operation.Message = "OCR analysis cancelled"
			})
			return
		}
		report, err := analyzePublishedOCRQuality(path, input.Format, input.LogicalTransformKey)
		if err != nil {
			updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
				operation.Status = "error"
				operation.Phase = "error"
				operation.Message = "OCR analysis failed"
				operation.Error = err.Error()
			})
			return
		}
		if snapshot, ok := loadOrReconstructPublishedOCRSnapshot(path, report.Format, input.LogicalTransformKey); ok {
			report = enrichSubtitleOCRReportWithSelective(ctx, workspace, snapshot, report)
		}
		if index == 0 {
			combined = report
		} else {
			combined.RevisionSet = append(combined.RevisionSet, report.RevisionSet...)
			combined.CleanCount += report.CleanCount
			combined.SuspiciousCount += report.SuspiciousCount
			combined.ReviewCount += report.ReviewCount
			combined.Findings = append(combined.Findings, report.Findings...)
			combined.LegacyTextOnly = combined.LegacyTextOnly || report.LegacyTextOnly
			combined.CanCorrect = combined.CanCorrect && report.CanCorrect
		}
		updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
			operation.Phase = "analyzing"
			operation.Progress = float64(index+1) / float64(len(paths)) * 100
			operation.Processed = index + 1
			operation.Total = len(paths)
			operation.Message = "Analyzing OCR quality"
		})
	}
	combined.BaselineRevision = subtitleRevisionSetKey(combined.RevisionSet)
	updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
		operation.Status = "completed"
		operation.Phase = "completed"
		operation.Progress = 100
		operation.QualityReport = &combined
		operation.Message = "OCR analysis completed"
	})
}

func startSubtitleOCRAnalysisOperation(input subtitleOCRAnalyzeInput) (string, error) {
	paths := subtitleOCRPaths(input.SidecarPath, input.SidecarPaths)
	validated, err := validateSubtitleOCRSidecarPaths(input.MediaPath, paths)
	if err != nil {
		return "", err
	}
	paths = validated
	if len(paths) == 0 {
		return "", errors.New("OCR analysis requires a published sidecar path")
	}
	workspace, err := os.MkdirTemp(canonicalOCRWorkspaceRoot(), ".mvforge-subtitle-analysis-*")
	if err != nil {
		return "", err
	}
	operationID := fmt.Sprintf("subtitle-ocr-analysis-%d", time.Now().UnixNano())
	workCtx, cancel := context.WithTimeout(context.Background(), subtitleOperationTimeout)
	now := time.Now()
	operation := &subtitleExtractionOperation{
		ID: operationID, Kind: "ocr_analysis", Status: "running", Phase: "preparing", Progress: 1,
		Format: input.Format, AssetPath: paths[0], Total: len(paths), Message: "Preparing OCR analysis",
		CreatedAt: now, UpdatedAt: now, cancel: cancel, publicationCommittedCh: make(chan struct{}),
	}
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[operationID] = operation
	subtitleExtractionOperations.Unlock()
	go func() {
		defer cancel()
		runSubtitleOCRAnalysisOperation(workCtx, operationID, workspace, input)
	}()
	return operationID, nil
}

func (snapshot *publishedCanonicalOCRSnapshot) CanCorrectLegacy(format string, outputCount int) bool {
	if snapshot == nil {
		return false
	}
	if snapshot.Evidence.Integrity != "legacy-text-cues-v1" {
		return true
	}
	return strings.EqualFold(format, "srt") && outputCount == 1
}

func startSubtitleOCRCorrectionOperation(input subtitleOCRCorrectionInput) (string, error) {
	paths := subtitleOCRPaths(input.SidecarPath, input.SidecarPaths)
	validated, err := validateSubtitleOCRSidecarPaths(input.MediaPath, paths)
	if err != nil {
		return "", err
	}
	paths = validated
	if len(paths) == 0 {
		return "", errors.New("OCR correction requires a published sidecar path")
	}
	workspace, err := os.MkdirTemp(canonicalOCRWorkspaceRoot(), ".mvforge-subtitle-correction-*")
	if err != nil {
		return "", err
	}
	operationID := fmt.Sprintf("subtitle-ocr-correction-%d", time.Now().UnixNano())
	workCtx, cancel := context.WithTimeout(context.Background(), subtitleOperationTimeout)
	now := time.Now()
	operation := &subtitleExtractionOperation{ID: operationID, Kind: "ocr_correction", Status: "running", Phase: "preparing", Progress: 1, Format: input.Format, AssetPath: paths[0], Message: "Preparing OCR correction", CreatedAt: now, UpdatedAt: now, cancel: cancel, publicationCommittedCh: make(chan struct{})}
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[operationID] = operation
	subtitleExtractionOperations.Unlock()
	go func() {
		defer cancel()
		runSubtitleOCRCorrectionOperation(workCtx, operationID, workspace, input)
	}()
	return operationID, nil
}

func (h AssetHandler) AnalyzeSubtitleOCR(c *gin.Context) {
	var input subtitleOCRAnalyzeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	paths, err := validateSubtitleOCRSidecarPaths(input.MediaPath, subtitleOCRPaths(input.SidecarPath, input.SidecarPaths))
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if len(paths) > 0 {
		input.SidecarPath = paths[0]
		input.SidecarPaths = paths
	}
	operationID, err := startSubtitleOCRAnalysisOperation(input)
	if err != nil {
		c.JSON(409, gin.H{"error": err.Error()})
		return
	}
	c.JSON(202, gin.H{"operationId": operationID, "status": "running", "phase": "preparing", "progress": 1})
}

func (h AssetHandler) CorrectSubtitleOCR(c *gin.Context) {
	var input subtitleOCRCorrectionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	paths, err := validateSubtitleOCRSidecarPaths(input.MediaPath, subtitleOCRPaths(input.SidecarPath, input.SidecarPaths))
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if len(paths) > 0 {
		input.SidecarPath = paths[0]
		input.SidecarPaths = paths
	}
	if _, err := validateSubtitleQualityCorrectionInput(input); err != nil {
		c.JSON(409, gin.H{"error": err.Error()})
		return
	}
	operationID, err := startSubtitleOCRCorrectionOperation(input)
	if err != nil {
		c.JSON(409, gin.H{"error": err.Error()})
		return
	}
	c.JSON(202, gin.H{"operationId": operationID, "status": "running", "phase": "preparing", "progress": 1})
}

func publishedSubtitleRevisionFor(path, logicalTransform string) (publishedSubtitleRevision, []byte, error) {
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	if cleanPath == "." || cleanPath == "" {
		return publishedSubtitleRevision{}, nil, errors.New("published subtitle path is required")
	}
	info, err := os.Stat(cleanPath)
	if err != nil {
		return publishedSubtitleRevision{}, nil, err
	}
	if info.IsDir() || info.Size() > maxPublishedSubtitleBytes {
		return publishedSubtitleRevision{}, nil, fmt.Errorf("published subtitle is unavailable for bounded revision analysis")
	}
	content, err := os.ReadFile(cleanPath)
	if err != nil {
		return publishedSubtitleRevision{}, nil, err
	}
	sum := sha256.Sum256(content)
	revision := publishedSubtitleRevision{
		ArtifactPath:       cleanPath,
		ArtifactIdentity:   fmt.Sprintf("path:%s", cleanPath),
		SizeBytes:          info.Size(),
		ModifiedAtUnixNano: info.ModTime().UnixNano(),
		ContentHash:        hex.EncodeToString(sum[:]),
		LogicalTransform:   strings.TrimSpace(logicalTransform),
	}
	return revision, content, nil
}

func publishedSubtitleRevisionMatches(expected publishedSubtitleRevision) bool {
	actual, _, err := publishedSubtitleRevisionFor(expected.ArtifactPath, expected.LogicalTransform)
	return err == nil && actual == expected
}

func publishedSubtitleSnapshotRoot() string {
	root := filepath.Join(publishedOCRStateRoot(), "published-snapshots")
	_ = os.MkdirAll(root, 0o700)
	return root
}

func publishedOCRStateRoot() string {
	if root := strings.TrimSpace(os.Getenv("MVFORGE_OCR_PERSISTENT_ROOT")); root != "" {
		_ = os.MkdirAll(root, 0o700)
		return root
	}
	databasePath := strings.TrimSpace(config.Load().DatabasePath)
	if databasePath == "" {
		databasePath = filepath.Join("data", "mvforge.db")
	}
	root := filepath.Join(filepath.Dir(databasePath), "mvforge-ocr-state")
	_ = os.MkdirAll(root, 0o700)
	return root
}

func publishedSubtitleSnapshotPath(ownerIdentity string) string {
	sum := sha256.Sum256([]byte(ownerIdentity))
	return filepath.Join(publishedSubtitleSnapshotRoot(), hex.EncodeToString(sum[:])+".json")
}

func publishedSubtitleOwnerIdentity(path, format, logicalTransform string) string {
	return strings.Join([]string{filepath.Clean(path), strings.ToLower(strings.TrimSpace(format)), strings.TrimSpace(logicalTransform)}, "\x00")
}

type publishedOCRExternalOwnershipCandidate struct {
	LogicalTransform string
	Snapshot         bool
}

type publishedOCRExternalOwnershipIndex map[string][]publishedOCRExternalOwnershipCandidate

func publishedOCROwnershipLookupKey(path, format string) string {
	return publishedSubtitleOwnerIdentity(filepath.Clean(path), strings.ToLower(strings.TrimSpace(format)), "")
}

func appendPublishedOCROwnershipCandidate(ownershipIndex publishedOCRExternalOwnershipIndex, path, format, logicalTransform string, snapshot bool) {
	lookupKey := publishedOCROwnershipLookupKey(path, format)
	candidates := ownershipIndex[lookupKey]
	for candidateIndex, candidate := range candidates {
		if candidate.LogicalTransform == logicalTransform {
			if snapshot {
				candidates[candidateIndex].Snapshot = true
			}
			ownershipIndex[lookupKey] = candidates
			return
		}
	}
	ownershipIndex[lookupKey] = append(candidates, publishedOCRExternalOwnershipCandidate{LogicalTransform: logicalTransform, Snapshot: snapshot})
}

func loadPublishedOCRExternalOwnershipIndex() publishedOCRExternalOwnershipIndex {
	index := make(publishedOCRExternalOwnershipIndex)
	root := publishedSubtitleSnapshotRoot()
	if entries, err := os.ReadDir(root); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			content, readErr := os.ReadFile(filepath.Join(root, entry.Name()))
			if readErr != nil {
				continue
			}
			var snapshot publishedCanonicalOCRSnapshot
			if json.Unmarshal(content, &snapshot) != nil || snapshot.OwnerIdentity == "" || snapshot.Revision.ArtifactPath == "" || snapshot.Format == "" {
				continue
			}
			if snapshot.OwnerIdentity != publishedSubtitleOwnerIdentity(snapshot.Revision.ArtifactPath, snapshot.Format, snapshot.LogicalTransformKey) {
				continue
			}
			appendPublishedOCROwnershipCandidate(index, snapshot.Revision.ArtifactPath, snapshot.Format, snapshot.LogicalTransformKey, true)
		}
	}

	legacyRoot := filepath.Join(publishedOCRStateRoot(), "legacy-ownership")
	if entries, err := os.ReadDir(legacyRoot); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			content, readErr := os.ReadFile(filepath.Join(legacyRoot, entry.Name()))
			if readErr != nil {
				continue
			}
			var ownership legacyPublishedOCROwnership
			if json.Unmarshal(content, &ownership) != nil || ownership.ArtifactPath == "" || ownership.Format == "" {
				continue
			}
			appendPublishedOCROwnershipCandidate(index, ownership.ArtifactPath, ownership.Format, ownership.LogicalTransformKey, false)
		}
	}
	return index
}

func publishedOCRExternalOwnershipFromIndex(index publishedOCRExternalOwnershipIndex, path, format string) (managed, analysisAvailable bool, logicalTransform string) {
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	format = strings.ToLower(strings.TrimSpace(format))
	if cleanPath == "." || cleanPath == "" || format == "" {
		return false, false, ""
	}
	candidates := index[publishedOCROwnershipLookupKey(cleanPath, format)]
	if len(candidates) == 0 {
		return false, false, ""
	}
	validCandidates := make([]publishedOCRExternalOwnershipCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		valid := false
		if candidate.Snapshot {
			_, valid = loadPublishedCanonicalOCRSnapshot(cleanPath, format, candidate.LogicalTransform)
		} else {
			_, valid = reconstructLegacyOCRSnapshot(cleanPath, format, candidate.LogicalTransform)
		}
		if valid {
			validCandidates = append(validCandidates, candidate)
		}
	}
	if len(validCandidates) == 0 {
		return true, false, ""
	}
	if len(validCandidates) > 1 {
		return true, false, ""
	}
	return true, true, validCandidates[0].LogicalTransform
}

func publishedOCRExternalOwnership(path, format string) (managed, analysisAvailable bool, logicalTransform string) {
	return publishedOCRExternalOwnershipFromIndex(loadPublishedOCRExternalOwnershipIndex(), path, format)
}

func canonicalOCRSourceIdentityMatches(identity canonicalOCRIdentity) bool {
	sourcePath := filepath.Clean(strings.TrimSpace(identity.SourcePath))
	if sourcePath == "." || sourcePath == "" || identity.SourceSnapshotVersion != "stat-v1" || identity.SourceSize < 0 || identity.SourceModTimeUnixNano < 0 {
		return false
	}
	info, err := os.Stat(sourcePath)
	return err == nil && !info.IsDir() && info.Size() == identity.SourceSize && info.ModTime().UnixNano() == identity.SourceModTimeUnixNano
}

func loadPublishedCanonicalOCRSnapshot(path, format, logicalTransform string) (*publishedCanonicalOCRSnapshot, bool) {
	owner := publishedSubtitleOwnerIdentity(path, format, logicalTransform)
	content, err := os.ReadFile(publishedSubtitleSnapshotPath(owner))
	if err != nil {
		return nil, false
	}
	var snapshot publishedCanonicalOCRSnapshot
	if json.Unmarshal(content, &snapshot) != nil || snapshot.OwnerIdentity != owner || snapshot.Version != 1 || snapshot.Integrity != "published-canonical-cues-v1" || snapshot.AnalyzerVersion != subtitleOCRQualityVersion || snapshot.RepairVersion != subtitleOCRRepairVersion || snapshot.ScoringVersion != canonicalOCRScoringVersion {
		return nil, false
	}
	if snapshot.Revision.ArtifactPath != filepath.Clean(path) || !publishedSubtitleRevisionMatches(snapshot.Revision) {
		return nil, false
	}
	if !canonicalOCRSourceIdentityMatches(snapshot.Evidence.Identity) {
		return nil, false
	}
	return &snapshot, true
}

type canonicalOCRArtifactDescriptor struct {
	Identity         string
	Revision         string
	Path             string
	Format           string
	LogicalTransform string
	Snapshot         *publishedCanonicalOCRSnapshot
}

// findPublishedCanonicalOCRArtifactForEmbeddedSource only consults the
// canonical OCR publication snapshots. It does not discover sidecars by
// filename and never starts OCR work.
func findPublishedCanonicalOCRArtifactForEmbeddedSource(mediaPath string, stream FFProbeStream) (canonicalOCRArtifactDescriptor, bool) {
	candidates := findPublishedCanonicalOCRArtifactsForEmbeddedSource(mediaPath, stream)
	if len(candidates) == 0 {
		return canonicalOCRArtifactDescriptor{}, false
	}
	return candidates[0], true
}

func findPublishedCanonicalOCRArtifactsForEmbeddedSource(mediaPath string, stream FFProbeStream) []canonicalOCRArtifactDescriptor {
	cleanMediaPath := filepath.Clean(strings.TrimSpace(mediaPath))
	if cleanMediaPath == "" || cleanMediaPath == "." {
		return nil
	}
	root := publishedSubtitleSnapshotRoot()
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	candidates := []canonicalOCRArtifactDescriptor{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		content, readErr := os.ReadFile(filepath.Join(root, entry.Name()))
		if readErr != nil {
			continue
		}
		var raw publishedCanonicalOCRSnapshot
		if json.Unmarshal(content, &raw) != nil || raw.Revision.ArtifactPath == "" {
			continue
		}
		snapshot, valid := loadPublishedCanonicalOCRSnapshot(raw.Revision.ArtifactPath, raw.Format, raw.LogicalTransformKey)
		if !valid || snapshot == nil || snapshot.Evidence.Identity.SourcePath != cleanMediaPath || snapshot.Evidence.Identity.StreamIndex != stream.Index || !snapshot.Evidence.Complete || len(snapshot.Evidence.Cues) == 0 {
			continue
		}
		artifactRevision, revisionErr := mediaFileFingerprint(snapshot.Revision.ArtifactPath)
		if revisionErr != nil {
			continue
		}
		candidates = append(candidates, canonicalOCRArtifactDescriptor{
			Identity: snapshot.OwnerIdentity, Revision: artifactRevision, Path: snapshot.Revision.ArtifactPath,
			Format: strings.ToLower(strings.TrimSpace(snapshot.Format)), LogicalTransform: snapshot.LogicalTransformKey,
			Snapshot: snapshot,
		})
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Format != candidates[j].Format {
			return candidates[i].Format == "ass"
		}
		if candidates[i].Path != candidates[j].Path {
			return candidates[i].Path < candidates[j].Path
		}
		return candidates[i].LogicalTransform < candidates[j].LogicalTransform
	})
	return candidates
}

func commitPublishedCanonicalOCRSnapshot(
	ctx context.Context,
	path, format, logicalTransform string,
	evidence *canonicalOCREvidence,
	operationID, previousRevision string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if evidence == nil || !evidence.Complete || len(evidence.Cues) == 0 {
		return errors.New("canonical OCR evidence is incomplete")
	}
	revision, _, err := publishedSubtitleRevisionFor(path, logicalTransform)
	if err != nil {
		return err
	}
	owner := publishedSubtitleOwnerIdentity(path, format, logicalTransform)
	snapshot := publishedCanonicalOCRSnapshot{
		Version:             1,
		OwnerIdentity:       owner,
		SourcePath:          evidence.Identity.SourcePath,
		Format:              strings.ToLower(strings.TrimSpace(format)),
		LogicalTransformKey: strings.TrimSpace(logicalTransform),
		Revision:            revision,
		Evidence:            *cloneCanonicalOCREvidence(evidence),
		CreatedAt:           time.Now().UTC(),
		UpdatedAt:           time.Now().UTC(),
		AnalyzerVersion:     subtitleOCRQualityVersion,
		RepairVersion:       subtitleOCRRepairVersion,
		ScoringVersion:      canonicalOCRScoringVersion,
		PreviousRevision:    strings.TrimSpace(previousRevision),
		CorrectionOperation: strings.TrimSpace(operationID),
		Integrity:           "published-canonical-cues-v1",
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	subtitleOCRSnapshotMu.Lock()
	defer subtitleOCRSnapshotMu.Unlock()
	root := publishedSubtitleSnapshotRoot()
	temporary, err := os.CreateTemp(root, ".published-canonical-cues-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err = temporary.Write(encoded); err == nil {
		err = temporary.Chmod(0o600)
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, publishedSubtitleSnapshotPath(owner)); err != nil {
		return err
	}
	prunePublishedCanonicalSnapshots(root)
	return nil
}

func prunePublishedCanonicalSnapshots(root string) {
	entries, _ := os.ReadDir(root)
	cleanupOrphanedPublishedCanonicalSnapshots(root, entries)
}

func cleanupOrphanedPublishedCanonicalSnapshots(root string, entries []os.DirEntry) {
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var snapshot publishedCanonicalOCRSnapshot
		if json.Unmarshal(content, &snapshot) != nil || snapshot.OwnerIdentity == "" {
			_ = os.Remove(path)
			continue
		}
		if _, err := os.Stat(snapshot.Revision.ArtifactPath); err != nil {
			_ = os.Remove(path)
		}
	}
}

type legacyPublishedOCROwnership struct {
	Version             int    `json:"version"`
	ArtifactPath        string `json:"artifactPath"`
	Format              string `json:"format"`
	LogicalTransformKey string `json:"logicalTransformKey,omitempty"`
	SourcePath          string `json:"sourcePath"`
	Provenance          string `json:"provenance"`
}

func legacyPublishedOCROwnershipPath(path, format, logicalTransform string) string {
	owner := publishedSubtitleOwnerIdentity(path, format, logicalTransform)
	sum := sha256.Sum256([]byte(owner))
	root := filepath.Join(publishedOCRStateRoot(), "legacy-ownership")
	_ = os.MkdirAll(root, 0o700)
	return filepath.Join(root, hex.EncodeToString(sum[:])+".json")
}

func markMVForgeOwnedLegacyOCRArtifact(path, format, logicalTransform, sourcePath string) error {
	if filepath.Clean(strings.TrimSpace(path)) == "." || filepath.Clean(strings.TrimSpace(sourcePath)) == "." {
		return errors.New("legacy OCR ownership requires artifact and source paths")
	}
	ownership := legacyPublishedOCROwnership{
		Version:             1,
		ArtifactPath:        filepath.Clean(path),
		Format:              strings.ToLower(strings.TrimSpace(format)),
		LogicalTransformKey: strings.TrimSpace(logicalTransform),
		SourcePath:          filepath.Clean(sourcePath),
		Provenance:          "mvforge-ocr-publication-v1",
	}
	encoded, err := json.Marshal(ownership)
	if err != nil {
		return err
	}
	return os.WriteFile(legacyPublishedOCROwnershipPath(path, format, logicalTransform), encoded, 0o600)
}

func loadMVForgeOwnedLegacyOCRArtifact(path, format, logicalTransform string) (legacyPublishedOCROwnership, bool) {
	content, err := os.ReadFile(legacyPublishedOCROwnershipPath(path, format, logicalTransform))
	if err != nil {
		return legacyPublishedOCROwnership{}, false
	}
	var ownership legacyPublishedOCROwnership
	if json.Unmarshal(content, &ownership) != nil || ownership.Version != 1 || ownership.Provenance != "mvforge-ocr-publication-v1" || ownership.ArtifactPath != filepath.Clean(path) || ownership.Format != strings.ToLower(strings.TrimSpace(format)) || ownership.LogicalTransformKey != strings.TrimSpace(logicalTransform) || ownership.SourcePath == "" {
		return legacyPublishedOCROwnership{}, false
	}
	return ownership, true
}

func reconstructLegacyOCRSnapshot(path, format, logicalTransform string) (*publishedCanonicalOCRSnapshot, bool) {
	ownership, owned := loadMVForgeOwnedLegacyOCRArtifact(path, format, logicalTransform)
	if !owned {
		return nil, false
	}
	revision, content, err := publishedSubtitleRevisionFor(path, logicalTransform)
	if err != nil || !validSubtitleSidecar(format, content) {
		return nil, false
	}
	cues := parseCanonicalBitmapSubtitleCues(format, content)
	if len(cues) == 0 {
		return nil, false
	}
	owner := publishedSubtitleOwnerIdentity(path, format, logicalTransform)
	return &publishedCanonicalOCRSnapshot{
		Version: 1, OwnerIdentity: owner, SourcePath: ownership.SourcePath, Format: format,
		LogicalTransformKey: logicalTransform, Revision: revision,
		Evidence:  canonicalOCREvidence{RequestedLanguage: "", OCRMode: "legacy", Complete: true, Integrity: "legacy-text-cues-v1", Cues: cues},
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), Integrity: "published-canonical-cues-v1",
	}, true
}

func analyzeCanonicalOCRSnapshot(snapshot *publishedCanonicalOCRSnapshot) subtitleOCRQualityReport {
	report := subtitleOCRQualityReport{
		ArtifactPath:        snapshot.Revision.ArtifactPath,
		Format:              snapshot.Format,
		LogicalTransformKey: snapshot.LogicalTransformKey,
		Revision:            snapshot.Revision,
		RevisionSet:         []publishedSubtitleRevision{snapshot.Revision},
		BaselineRevision:    snapshot.Revision.key(),
		AnalyzerVersion:     subtitleOCRQualityVersion,
		RepairVersion:       subtitleOCRRepairVersion,
		LegacyTextOnly:      snapshot.Evidence.Integrity == "legacy-text-cues-v1",
		CanCorrect:          snapshot.Evidence.Integrity != "legacy-text-cues-v1" || strings.EqualFold(snapshot.Format, "srt"),
		Findings:            make([]subtitleOCRQualityFinding, 0, len(snapshot.Evidence.Cues)),
	}
	for _, cue := range snapshot.Evidence.Cues {
		finding := subtitleOCRQualityFinding{CueID: cue.SourceOrdinal, OriginalText: cue.Text, Status: subtitleOCRQualityStatusClean, Baseline: report.BaselineRevision}
		lower := strings.ToLower(cue.Text)
		if strings.ContainsRune(cue.Text, '\ufffd') {
			finding.Status = subtitleOCRQualityStatusReview
			finding.Reasons = append(finding.Reasons, "replacement character")
		}
		if confusion := ocrConfusionCount(cue.Text, snapshot.Evidence.RequestedLanguage); confusion > 0 {
			finding.Status = subtitleOCRQualityStatusSuspect
			finding.Reasons = append(finding.Reasons, "known OCR confusion pattern")
			finding.Evidence = append(finding.Evidence, fmt.Sprintf("%d bounded confusion signal(s)", confusion))
			if suggestion, ok := deterministicOCRSuggestion(cue.Text, snapshot.Evidence.RequestedLanguage); ok {
				finding.Suggestion = suggestion
				finding.CandidateSource = "deterministic-confusion-repair"
			}
		}
		if strings.ContainsAny(lower, "|{}[]") {
			finding.Status = subtitleOCRQualityStatusReview
			finding.Reasons = append(finding.Reasons, "suspicious OCR character")
		}
		if finding.Status == subtitleOCRQualityStatusClean {
			finding.Evidence = append(finding.Evidence, "no bounded deterministic suspicion detected")
		}
		report.Findings = append(report.Findings, finding)
		switch finding.Status {
		case subtitleOCRQualityStatusClean:
			report.CleanCount++
		case subtitleOCRQualityStatusSuspect:
			report.SuspiciousCount++
		default:
			report.ReviewCount++
		}
	}
	return report
}

func enrichSubtitleOCRReportWithSelective(ctx context.Context, workspace string, snapshot *publishedCanonicalOCRSnapshot, report subtitleOCRQualityReport) subtitleOCRQualityReport {
	if snapshot == nil {
		return report
	}
	byID := make(map[int]canonicalBitmapSubtitleCue, len(snapshot.Evidence.Cues))
	for _, cue := range snapshot.Evidence.Cues {
		byID[cue.SourceOrdinal] = cue
	}
	attempts := 0
	for index := range report.Findings {
		finding := &report.Findings[index]
		if finding.Status == subtitleOCRQualityStatusClean || attempts >= 16 {
			continue
		}
		cue, ok := byID[finding.CueID]
		if !ok {
			continue
		}
		attempts++
		candidate, source, err := runSelectiveSecondaryOCR(ctx, snapshot, cue, workspace, nil)
		if err != nil {
			if ctx.Err() != nil {
				return report
			}
			finding.Reasons = append(finding.Reasons, "selective OCR unavailable: bounded attempt failed")
			continue
		}
		if candidate != "" {
			finding.Suggestion = candidate
			finding.CandidateSource = source
			finding.Evidence = append(finding.Evidence, "bounded selective OCR candidate")
		} else if source != "" {
			finding.Reasons = append(finding.Reasons, source)
		}
	}
	return report
}

func deterministicOCRSuggestion(text, language string) (string, bool) {
	if strings.Contains(text, "{\\") || strings.TrimSpace(text) == "" {
		return "", false
	}
	changed := false
	candidate := ocrConfusionTokenPattern.ReplaceAllStringFunc(text, func(token string) string {
		replacement, ok := ocrDictionaryCandidate(token, language)
		if !ok {
			return token
		}
		changed = true
		return replacement
	})
	if !changed || candidate == text || subtitleOCRScoreForLanguage(candidate, language) <= subtitleOCRScoreForLanguage(text, language) {
		return "", false
	}
	return candidate, true
}
