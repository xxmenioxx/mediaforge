package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
)

const subtitleOCRAIMaxCueTextBytes = 4 * 1024
const subtitleOCRAIContextRunes = 80

var errSubtitleOCRAIStaleTarget = errors.New("stale OCR review target; refresh the source revision")

type subtitleOCRAIReport struct {
	Enabled         bool   `json:"enabled"`
	State           string `json:"state"`
	Model           string `json:"model,omitempty"`
	Notice          string `json:"notice,omitempty"`
	SuggestedCount  int    `json:"suggestedCount"`
	UnassessedCount int    `json:"unassessedCount"`
}

type subtitleOCRAISuggestion struct {
	CueID            int    `json:"cueId"`
	OriginalHash     string `json:"originalHash"`
	ProposedText     string `json:"proposedText"`
	Reason           string `json:"reason"`
	Model            string `json:"model"`
	BaselineRevision string `json:"baselineRevision"`
}

type subtitleOCRAISelection struct {
	Enabled bool
	Model   string
}

func loadSubtitleOCRAISelection(db *gorm.DB) (subtitleOCRAISelection, error) {
	if db == nil {
		return subtitleOCRAISelection{}, errors.New("OCR AI settings are unavailable")
	}
	var setting models.AppSetting
	if err := db.First(&setting, "key = ?", ocrAISettingsKey).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return subtitleOCRAISelection{}, nil
		}
		return subtitleOCRAISelection{}, errors.New("OCR AI settings could not be loaded")
	}
	normalized, err := normalizeOCRAISettingsValue(setting.Value)
	if err != nil {
		return subtitleOCRAISelection{}, errors.New("OCR AI settings are invalid")
	}
	enabled, _ := normalized["enabled"].(bool)
	model, _ := normalized["model"].(string)
	return subtitleOCRAISelection{Enabled: enabled, Model: model}, nil
}

func enrichSubtitleOCRReportWithAI(
	ctx context.Context,
	operationID string,
	report subtitleOCRQualityReport,
	target *canonicalOCRReviewTarget,
	db *gorm.DB,
	lifecycle *AIModelLifecycle,
	client *http.Client,
) (subtitleOCRQualityReport, error) {
	suspicious := countSubtitleOCRAISuspicious(report.Findings)
	selection, err := loadSubtitleOCRAISelection(db)
	if err != nil {
		report.OCRAI = &subtitleOCRAIReport{
			State: "settings_unavailable", Notice: "OCR AI settings are unavailable; deterministic OCR review remains available.",
			UnassessedCount: suspicious,
		}
		return report, nil
	}
	report.OCRAI = &subtitleOCRAIReport{
		Enabled: selection.Enabled, Model: selection.Model, State: "disabled", UnassessedCount: suspicious,
	}
	if !selection.Enabled {
		report.OCRAI.Notice = "OCR AI assistance is disabled. Deterministic OCR review remains available."
		return report, nil
	}
	report.OCRAI.State = "checking"
	if selection.Model == "" {
		report.OCRAI.State = "invalid_settings"
		report.OCRAI.Notice = "OCR AI has no valid explicitly selected model; no model request was sent."
		return report, nil
	}
	if target == nil || target.State != "exact" || report.ReviewTarget == nil ||
		report.BaselineRevision == "" || report.BaselineRevision != target.BaselineRevision ||
		subtitleRevisionSetKey(report.RevisionSet) != target.BaselineRevision ||
		!reflect.DeepEqual(report.ReviewTarget, target) {
		report.OCRAI.State = "stale"
		report.OCRAI.Notice = "The exact OCR source or sidecar revision changed; AI suggestions were discarded."
		return report, errSubtitleOCRAIStaleTarget
	}
	if err := validateSubtitleOCRAIReviewTarget(target); err != nil {
		report.OCRAI.State = "stale"
		report.OCRAI.Notice = "The exact OCR source or sidecar revision changed; AI suggestions were discarded."
		return report, err
	}
	providerConfiguration, configErr := loadOllamaProviderConfiguration(db)
	if configErr != nil || providerConfiguration.State != ollamaProviderStateConfigured || providerConfiguration.SharedEndpoint == "" {
		report.OCRAI.State = "provider_unavailable"
		report.OCRAI.Notice = "The shared Ollama provider is not configured; deterministic OCR review remains available."
		return report, nil
	}
	if lifecycle == nil || lifecycle.manager == nil || !lifecycle.manager.enabled ||
		lifecycle.manager.admission == nil || strings.TrimSpace(lifecycle.manager.capacityDomain) == "" {
		report.OCRAI.State = "resource_unavailable"
		report.OCRAI.Notice = "AI resource admission is unavailable; no model request was sent."
		return report, nil
	}
	cues, unassessed := subtitleOCRAIReviewCues(report, target)
	attachOCRCorrectionMemoryPromptSuggestions(db, target, report, cues)
	if len(cues) == 0 {
		report.OCRAI.UnassessedCount = unassessed
		report.OCRAI.State = "not_needed"
		report.OCRAI.Notice = "No bounded suspicious cues are available for AI review."
		return report, nil
	}
	if client == nil {
		client = NewOllamaProviderHTTPClient()
	}
	language := subtitleProofreadingLanguage(target.Identity.RequestedLanguage, cues)
	if language == "unknown" {
		report.OCRAI.State = "language_unavailable"
		report.OCRAI.Notice = "The canonical OCR source language is unavailable; no model request was sent."
		return report, nil
	}
	provider, err := newOllamaTranslationProviderWithClient(translationSettings{
		Provider: translationProviderOllama, OllamaEndpoint: providerConfiguration.SharedEndpoint,
		OllamaModel: selection.Model, MaxCuesPerRequest: subtitleProofreadingMaxBatchCues,
		MaxRequestBytes: subtitleProofreadingMaxRequestBody,
	}, client)
	if err != nil {
		report.OCRAI.State = "provider_unavailable"
		report.OCRAI.Notice = "The shared Ollama provider configuration is invalid; deterministic OCR review remains available."
		return report, nil
	}

	workflowDigest := sha256.Sum256([]byte(target.Identity.key() + "\x00" + target.BaselineRevision))
	workflowID := "ocr-review-" + hex.EncodeToString(workflowDigest[:16])
	suggestions := make(map[int]*subtitleOCRAISuggestion)
	for start := 0; start < len(cues); start += subtitleProofreadingMaxBatchCues {
		if ctx == nil || ctx.Err() != nil {
			report.OCRAI.State = "cancelled"
			report.OCRAI.Notice = "AI review was cancelled; deterministic OCR review remains available."
			return report, nil
		}
		if err := validateSubtitleOCRAIReviewTarget(target); err != nil {
			report.OCRAI.State = "stale"
			report.OCRAI.Notice = "The exact OCR source or sidecar revision changed; AI suggestions were discarded."
			return report, err
		}
		currentSelection, selectionErr := loadSubtitleOCRAISelection(db)
		currentProvider, currentProviderErr := loadOllamaProviderConfiguration(db)
		if selectionErr != nil || currentSelection != selection || currentProviderErr != nil ||
			currentProvider.State != ollamaProviderStateConfigured || currentProvider.SharedEndpoint != providerConfiguration.SharedEndpoint {
			report.OCRAI.State = "configuration_changed"
			report.OCRAI.Notice = "OCR AI settings changed during review; suggestions were discarded."
			return report, nil
		}

		configurationRevision := providerConfigurationRevision(providerConfiguration.SharedEndpoint, selection.Model)
		if readyErr := lifecycle.manager.EnsureReady(
			ctx, operationID, providerConfiguration.SharedEndpoint, configurationRevision, selection.Model, nil, aiModelConsumerSubtitleOCR,
		); readyErr != nil {
			if ctx.Err() != nil {
				report.OCRAI.State = "cancelled"
				report.OCRAI.Notice = "AI review was cancelled while preparing the selected model; deterministic OCR review remains available."
			} else if errors.Is(readyErr, errAIModelWarmupFailed) {
				report.OCRAI.State = "warmup_failed"
				report.OCRAI.Notice = "The selected OCR model could not be loaded; no suggestion request was sent."
			} else if errors.Is(readyErr, errResourceAdmissionUnavailable) || strings.Contains(strings.ToLower(readyErr.Error()), "admission") {
				report.OCRAI.State = "resource_unavailable"
				report.OCRAI.Notice = "AI resource admission did not grant model readiness; no suggestion request was sent."
			} else {
				report.OCRAI.State = "model_unavailable"
				report.OCRAI.Notice = "The explicitly selected OCR model is unavailable; no suggestion request was sent."
			}
			return report, nil
		}
		if ctx.Err() != nil {
			report.OCRAI.State = "cancelled"
			report.OCRAI.Notice = "AI review was cancelled while preparing the selected model; deterministic OCR review remains available."
			return report, nil
		}
		if err := validateSubtitleOCRAIReviewTarget(target); err != nil {
			report.OCRAI.State = "stale"
			report.OCRAI.Notice = "The exact OCR source or sidecar revision changed while preparing the model; AI suggestions were discarded."
			return report, err
		}
		currentSelection, selectionErr = loadSubtitleOCRAISelection(db)
		currentProvider, currentProviderErr = loadOllamaProviderConfiguration(db)
		if selectionErr != nil || currentSelection != selection || currentProviderErr != nil ||
			currentProvider.State != ollamaProviderStateConfigured || currentProvider.SharedEndpoint != providerConfiguration.SharedEndpoint {
			report.OCRAI.State = "configuration_changed"
			report.OCRAI.Notice = "OCR AI settings changed while preparing the model; suggestions were discarded."
			return report, nil
		}

		end := min(start+subtitleProofreadingMaxBatchCues, len(cues))
		batch := cues[start:end]
		lease, admissionErr := lifecycle.manager.acquireInference(
			ctx, operationID, workflowID, providerConfiguration.SharedEndpoint, configurationRevision, selection.Model, nil, aiModelConsumerSubtitleOCR,
		)
		if admissionErr != nil {
			report.OCRAI.State = "resource_unavailable"
			report.OCRAI.Notice = "AI resource admission did not grant this review; no model request was sent."
			return report, nil
		}
		residents, residentErr := lifecycle.manager.providerAPI.loadedModels(ctx, providerConfiguration.SharedEndpoint)
		if residentErr != nil {
			if lifecycle.manager.releaseInference(ctx, operationID, configurationRevision, selection.Model, lease) != nil {
				report.OCRAI.State = "resource_unavailable"
				report.OCRAI.Notice = "AI resource admission could not be released safely; suggestions were discarded."
				return report, nil
			}
			report.OCRAI.State = "model_unavailable"
			report.OCRAI.Notice = "The explicitly selected OCR model could not be verified as resident; no suggestion request was sent."
			return report, nil
		}
		if resident, _ := modelIsResident(residents, selection.Model); !resident {
			if lifecycle.manager.releaseInference(ctx, operationID, configurationRevision, selection.Model, lease) != nil {
				report.OCRAI.State = "resource_unavailable"
				report.OCRAI.Notice = "AI resource admission could not be released safely; suggestions were discarded."
				return report, nil
			}
			report.OCRAI.State = "model_unavailable"
			report.OCRAI.Notice = "The explicitly selected OCR model is not resident; no suggestion request was sent."
			return report, nil
		}
		if err := validateSubtitleOCRAIReviewTarget(target); err != nil {
			_ = lifecycle.manager.releaseInference(ctx, operationID, configurationRevision, selection.Model, lease)
			report.OCRAI.State = "stale"
			report.OCRAI.Notice = "The exact OCR source or sidecar revision changed; AI suggestions were discarded."
			return report, err
		}
		currentSelection, selectionErr = loadSubtitleOCRAISelection(db)
		currentProvider, currentProviderErr = loadOllamaProviderConfiguration(db)
		if selectionErr != nil || currentSelection != selection || currentProviderErr != nil ||
			currentProvider.State != ollamaProviderStateConfigured || currentProvider.SharedEndpoint != providerConfiguration.SharedEndpoint {
			_ = lifecycle.manager.releaseInference(ctx, operationID, configurationRevision, selection.Model, lease)
			report.OCRAI.State = "configuration_changed"
			report.OCRAI.Notice = "OCR AI settings changed during review; suggestions were discarded."
			return report, nil
		}

		result, proofreadErr := provider.Proofread(ctx, language, batch)
		releaseErr := lifecycle.manager.releaseInference(ctx, operationID, configurationRevision, selection.Model, lease)
		if ctx.Err() != nil {
			report.OCRAI.State = "cancelled"
			report.OCRAI.Notice = "AI review was cancelled; deterministic OCR review remains available."
			return report, nil
		}
		if releaseErr != nil {
			report.OCRAI.State = "resource_unavailable"
			report.OCRAI.Notice = "AI resource admission could not be released safely; suggestions were discarded."
			return report, nil
		}
		if proofreadErr != nil {
			report.OCRAI.State = "provider_failed"
			report.OCRAI.Notice = "The OCR AI provider did not return a valid bounded review; deterministic OCR review remains available."
			return report, nil
		}
		if err := validateSubtitleOCRAIReviewTarget(target); err != nil {
			report.OCRAI.State = "stale"
			report.OCRAI.Notice = "The exact OCR source or sidecar revision changed; AI suggestions were discarded."
			return report, err
		}
		currentSelection, selectionErr = loadSubtitleOCRAISelection(db)
		currentProvider, currentProviderErr = loadOllamaProviderConfiguration(db)
		if selectionErr != nil || currentSelection != selection || currentProviderErr != nil ||
			currentProvider.State != ollamaProviderStateConfigured || currentProvider.SharedEndpoint != providerConfiguration.SharedEndpoint {
			report.OCRAI.State = "configuration_changed"
			report.OCRAI.Notice = "OCR AI settings changed during review; suggestions were discarded."
			return report, nil
		}
		for _, finding := range result.Findings {
			if finding.Status != "suggested" || strings.TrimSpace(finding.validatedText) == "" {
				continue
			}
			cueID, cue, ok := subtitleOCRAIResponseCue(finding.CueID, batch)
			if !ok || finding.SourceHash != cue.Hash {
				report.OCRAI.State = "provider_failed"
				report.OCRAI.Notice = "The OCR AI response did not match its requested cue identities; suggestions were discarded."
				return report, nil
			}
			proposed := finding.validatedText
			if cue.ASSDialogue != nil {
				proposed, err = cue.ASSDialogue.ReconstructProviderText(proposed)
				if err != nil {
					report.OCRAI.State = "provider_failed"
					report.OCRAI.Notice = "The OCR AI response did not preserve ASS structure; suggestions were discarded."
					return report, nil
				}
			}
			suggestions[cueID] = &subtitleOCRAISuggestion{
				CueID: cueID, OriginalHash: finding.SourceHash, ProposedText: proposed,
				Reason: finding.Reason, Model: selection.Model, BaselineRevision: target.BaselineRevision,
			}
		}
	}
	if err := validateSubtitleOCRAIReviewTarget(target); err != nil {
		report.OCRAI.State = "stale"
		report.OCRAI.Notice = "The exact OCR source or sidecar revision changed; AI suggestions were discarded."
		return report, err
	}
	currentSelection, selectionErr := loadSubtitleOCRAISelection(db)
	currentProvider, currentProviderErr := loadOllamaProviderConfiguration(db)
	if selectionErr != nil || currentSelection != selection || currentProviderErr != nil ||
		currentProvider.State != ollamaProviderStateConfigured || currentProvider.SharedEndpoint != providerConfiguration.SharedEndpoint {
		report.OCRAI.State = "configuration_changed"
		report.OCRAI.Notice = "OCR AI settings changed during review; suggestions were discarded."
		return report, nil
	}
	for index := range report.Findings {
		if suggestion := suggestions[report.Findings[index].CueID]; suggestion != nil {
			report.Findings[index].AISuggestion = suggestion
			report.OCRAI.SuggestedCount++
		}
	}
	report.OCRAI.UnassessedCount = unassessed
	report.OCRAI.State = "suggestions_ready"
	if report.OCRAI.SuggestedCount == 0 {
		report.OCRAI.State = "completed"
		report.OCRAI.Notice = "The selected model returned no validated correction suggestions."
	} else {
		report.OCRAI.Notice = "AI suggestions are proposals only. Review each cue and confirm any correction manually."
	}
	return report, nil
}

func validateSubtitleOCRAIReviewTarget(target *canonicalOCRReviewTarget) error {
	if target == nil || target.State != "exact" || !canonicalOCRSourceIdentityMatches(target.Identity) {
		return errSubtitleOCRAIStaleTarget
	}
	_, _, snapshots, err := canonicalOCRReviewTargetSnapshots(target)
	if err != nil {
		return errSubtitleOCRAIStaleTarget
	}
	for _, snapshot := range snapshots {
		if snapshot == nil || snapshot.Evidence.Identity.key() != target.Identity.key() {
			return errSubtitleOCRAIStaleTarget
		}
	}
	return nil
}

func countSubtitleOCRAISuspicious(findings []subtitleOCRQualityFinding) int {
	count := 0
	for _, finding := range findings {
		if finding.Status != subtitleOCRQualityStatusClean {
			count++
		}
	}
	return count
}

func subtitleOCRAIReviewCues(report subtitleOCRQualityReport, target *canonicalOCRReviewTarget) ([]subtitleProofreadingCue, int) {
	paths, formats, _, err := canonicalOCRReviewTargetSnapshots(target)
	if err != nil {
		return nil, countSubtitleOCRAISuspicious(report.Findings)
	}
	byCue := make(map[int][]subtitleProofreadingCue)
	complete := true
	for index, path := range paths {
		content, readErr := os.ReadFile(path)
		if readErr != nil || len(content) == 0 || len(content) > subtitleProofreadingMaxBytes {
			complete = false
			break
		}
		parsed, parseErr := parseSubtitleProofreadingCues(formats[index], content)
		if parseErr != nil {
			complete = false
			break
		}
		for cueIndex := range parsed {
			parsed[cueIndex].Neighbors = subtitleProofreadingNeighbors(parsed, cueIndex)
			for neighborIndex := range parsed[cueIndex].Neighbors {
				parsed[cueIndex].Neighbors[neighborIndex].Text = truncateSubtitleProofreadingText(parsed[cueIndex].Neighbors[neighborIndex].Text, subtitleOCRAIContextRunes)
			}
			byCue[parsed[cueIndex].Ordinal] = append(byCue[parsed[cueIndex].Ordinal], parsed[cueIndex])
		}
	}
	selected := make([]subtitleProofreadingCue, 0)
	unassessed := 0
	if !complete {
		return selected, countSubtitleOCRAISuspicious(report.Findings)
	}
	for _, finding := range report.Findings {
		if finding.Status == subtitleOCRQualityStatusClean {
			continue
		}
		if len(selected) >= subtitleProofreadingMaxModelCues {
			unassessed++
			continue
		}
		members := byCue[finding.CueID]
		if len(members) != len(formats) || len(finding.OriginalText) > subtitleOCRAIMaxCueTextBytes {
			unassessed++
			continue
		}
		matched := true
		original := ""
		for memberIndex, cue := range members {
			text := strings.TrimSpace(stripSubtitleProofreadingAnchors(cue.SourceText))
			if memberIndex == 0 {
				original = text
			} else if text != original {
				matched = false
			}
			if text != strings.TrimSpace(finding.OriginalText) {
				matched = false
			}
		}
		if !matched {
			unassessed++
			continue
		}
		chosen := members[0]
		for _, cue := range members {
			if cue.ASSDialogue != nil {
				chosen = cue
				break
			}
		}
		chosen.ID = fmt.Sprintf("ocr-cue-%d", finding.CueID)
		if len(chosen.SourceText) > subtitleOCRAIMaxCueTextBytes || chosen.Hash != subtitleProofreadingTextHash(chosen.SourceText) {
			unassessed++
			continue
		}
		selected = append(selected, chosen)
	}
	return selected, unassessed
}

func subtitleOCRAIResponseCue(id string, cues []subtitleProofreadingCue) (int, subtitleProofreadingCue, bool) {
	for _, cue := range cues {
		if cue.ID == id {
			ordinal := 0
			_, _ = fmt.Sscanf(id, "ocr-cue-%d", &ordinal)
			if ordinal > 0 {
				return ordinal, cue, true
			}
		}
	}
	return 0, subtitleProofreadingCue{}, false
}
