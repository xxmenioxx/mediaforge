package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

type subtitleProofreadingDecisionRecord struct {
	CueID      string    `json:"cueId"`
	SourceHash string    `json:"sourceTextHash"`
	Action     string    `json:"action"`
	TextHash   string    `json:"selectedTextHash,omitempty"`
	SelectedBy string    `json:"selectedBy"`
	DecidedAt  time.Time `json:"decidedAt"`
}

type subtitleProofreadingRevisionManifest struct {
	Version             int                                  `json:"version"`
	RevisionID          string                               `json:"revisionId"`
	MediaPath           string                               `json:"mediaPath"`
	SidecarPath         string                               `json:"sidecarPath"`
	Format              string                               `json:"format"`
	LogicalTransformKey string                               `json:"logicalTransformKey,omitempty"`
	BeforeRevision      string                               `json:"beforeRevision"`
	AfterContentHash    string                               `json:"afterContentHash"`
	OperationID         string                               `json:"operationId,omitempty"`
	Provider            string                               `json:"provider,omitempty"`
	Model               string                               `json:"model,omitempty"`
	ContractVersion     string                               `json:"contractVersion"`
	Kind                string                               `json:"kind"`
	CreatedAt           time.Time                            `json:"createdAt"`
	Decisions           []subtitleProofreadingDecisionRecord `json:"decisions,omitempty"`
}

type subtitleProofreadingApplyResult struct {
	Path             string `json:"path"`
	RevisionID       string `json:"revisionId"`
	AffectedCueCount int    `json:"affectedCueCount"`
	ChangedCueCount  int    `json:"changedCueCount"`
	SourceRevision   string `json:"sourceRevision"`
	Message          string `json:"message"`
}

type subtitleProofreadingRestoreResult struct {
	Path           string `json:"path"`
	RevisionID     string `json:"revisionId"`
	SourceRevision string `json:"sourceRevision"`
	Restored       bool   `json:"restored"`
	Message        string `json:"message"`
}

func (h AssetHandler) applySubtitleProofreading(input subtitleProofreadingApplyInput) (subtitleProofreadingApplyResult, error) {
	if !input.Confirmed {
		return subtitleProofreadingApplyResult{}, errors.New("explicit proofreading batch confirmation is required")
	}
	if len(input.Decisions) > subtitleProofreadingMaxModelCues {
		return subtitleProofreadingApplyResult{}, errors.New("proofreading decision batch exceeds the supported limit")
	}
	operation, ok := currentSubtitleProofreadingOperation(strings.TrimSpace(input.OperationID))
	if !ok || operation.Kind != "proofreading" || operation.Status != "completed" || operation.proofreadingRequest == nil || operation.Proofreading == nil {
		return subtitleProofreadingApplyResult{}, errors.New("completed proofreading operation is unavailable")
	}
	request := *operation.proofreadingRequest
	if filepath.Clean(input.Path) != request.MediaPath || filepath.Clean(input.SubtitlePath) != request.SidecarPath || input.Revision != request.RevisionKey || operation.Proofreading.ProviderState == "stale" {
		return subtitleProofreadingApplyResult{}, errors.New("proofreading decisions do not match the captured sidecar revision")
	}
	mediaPath, _, err := h.resolveOwnedSubtitleAsset(input.Path)
	if err != nil {
		return subtitleProofreadingApplyResult{}, err
	}
	source, err := resolveExternalSubtitleTranslationSource(mediaPath, input.SubtitlePath)
	if err != nil || source.SourceRevision != input.Revision || source.Representation != request.Format {
		return subtitleProofreadingApplyResult{}, errors.New("subtitle source revision changed; refresh Asset Info and review again")
	}
	if err := rejectProofreadingSymlink(source.ExternalPath); err != nil {
		return subtitleProofreadingApplyResult{}, err
	}
	current, content, err := publishedSubtitleRevisionFor(source.ExternalPath, source.LogicalTransformKey)
	if err != nil || current != request.Revision {
		return subtitleProofreadingApplyResult{}, errors.New("subtitle source revision changed; refresh Asset Info and review again")
	}
	if !utf8.Valid(content) || len(content) > subtitleProofreadingMaxBytes || !validTranslatedSubtitleSidecar(request.Format, content) {
		return subtitleProofreadingApplyResult{}, errors.New("current subtitle cannot be safely proofread")
	}
	selected := make(map[string]subtitleProofreadingDecisionInput, len(input.Decisions))
	replacements := make(map[string]string, len(input.Decisions))
	records := make([]subtitleProofreadingDecisionRecord, 0, len(input.Decisions))
	changed := 0
	for _, decision := range input.Decisions {
		if _, duplicate := selected[decision.CueID]; duplicate {
			return subtitleProofreadingApplyResult{}, errors.New("proofreading decision batch contains a duplicate cue")
		}
		cue, exists := proofreadCueByID(request.Cues, decision.CueID)
		finding, findingExists := subtitleProofreadingReportFinding(*operation.Proofreading, decision.CueID)
		if !exists || !findingExists || cue.Hash != decision.SourceHash || finding.SourceHash != decision.SourceHash {
			return subtitleProofreadingApplyResult{}, errors.New("selected cue identity or source hash does not match the review")
		}
		selected[decision.CueID] = decision
		selectedText := ""
		switch decision.Action {
		case "accept":
			if strings.TrimSpace(finding.Suggested) == "" || strings.TrimSpace(finding.SuggestedProviderText) == "" || decision.Text != "" {
				return subtitleProofreadingApplyResult{}, errors.New("accept requires an available AI suggestion")
			}
			selectedText = finding.SuggestedProviderText
		case "edit":
			if strings.TrimSpace(decision.Text) == "" {
				return subtitleProofreadingApplyResult{}, errors.New("edited proofreading text is required")
			}
			selectedText, err = normalizeSubtitleProofreadingProtectedText(cue, decision.Text)
			if err != nil {
				return subtitleProofreadingApplyResult{}, fmt.Errorf("cue %s: %w", cue.ID, err)
			}
		case "reject", "keep_original":
			if decision.Text != "" {
				return subtitleProofreadingApplyResult{}, errors.New("reject and keep-original decisions cannot include replacement text")
			}
		default:
			return subtitleProofreadingApplyResult{}, fmt.Errorf("unsupported proofreading decision %q", decision.Action)
		}
		if selectedText != "" {
			if err := validateSubtitleProofreadingCandidate(cue, selectedText, request.Format, request.Language); err != nil {
				return subtitleProofreadingApplyResult{}, fmt.Errorf("cue %s: %w", cue.ID, err)
			}
			replacements[cue.ID] = selectedText
			if selectedText != cue.SourceText {
				changed++
			}
		}
		textHash := ""
		if selectedText != "" {
			textHash = subtitleProofreadingTextHash(selectedText)
		}
		records = append(records, subtitleProofreadingDecisionRecord{
			CueID: cue.ID, SourceHash: cue.Hash, Action: decision.Action,
			TextHash: textHash, SelectedBy: "operator", DecidedAt: time.Now().UTC(),
		})
	}
	updated, err := applySubtitleProofreadingReplacements(request.Format, content, request.Cues, replacements)
	if err != nil || len(updated) == 0 || len(updated) > subtitleProofreadingMaxBytes || !utf8.Valid(updated) || !validTranslatedSubtitleSidecar(request.Format, updated) {
		return subtitleProofreadingApplyResult{}, errors.New("proofreading replacement failed full subtitle validation")
	}
	if err := rejectProofreadingSymlink(source.ExternalPath); err != nil {
		return subtitleProofreadingApplyResult{}, err
	}
	manifest := subtitleProofreadingRevisionManifest{
		MediaPath: mediaPath, SidecarPath: source.ExternalPath, Format: request.Format,
		LogicalTransformKey: source.LogicalTransformKey, BeforeRevision: input.Revision,
		AfterContentHash: subtitleProofreadingTextHash(string(updated)), OperationID: input.OperationID,
		Provider: operation.Proofreading.Provider, Model: operation.Proofreading.Model,
		ContractVersion: subtitleProofreadingContractVersion, Kind: "apply", CreatedAt: time.Now().UTC(),
		Decisions: records,
	}
	subtitlePublicationMu.Lock()
	defer subtitlePublicationMu.Unlock()
	lockedRevision, _, err := publishedSubtitleRevisionFor(source.ExternalPath, source.LogicalTransformKey)
	if err != nil || lockedRevision != request.Revision || sourceRevisionKeyForExternal(source.ExternalPath, source.LogicalTransformKey) != input.Revision {
		return subtitleProofreadingApplyResult{}, errors.New("subtitle source became stale before approved publication")
	}
	revisionID, err := persistSubtitleProofreadingPriorRevision(manifest, content)
	if err != nil {
		return subtitleProofreadingApplyResult{}, fmt.Errorf("preserve subtitle revision before publication: %w", err)
	}
	if err := publishSubtitleProofreadingContentLocked(source.ExternalPath, lockedRevision, source.LogicalTransformKey, updated); err != nil {
		return subtitleProofreadingApplyResult{}, err
	}
	newRevision, _, err := publishedSubtitleRevisionFor(source.ExternalPath, source.LogicalTransformKey)
	newSourceRevision := sourceRevisionKeyForExternal(source.ExternalPath, source.LogicalTransformKey)
	if err == nil {
		manifest.AfterContentHash = newRevision.ContentHash
		_ = updateSubtitleProofreadingManifest(revisionID, manifest)
	}
	return subtitleProofreadingApplyResult{
		Path: source.ExternalPath, RevisionID: revisionID, AffectedCueCount: len(records),
		ChangedCueCount: changed, SourceRevision: newSourceRevision,
		Message: "Confirmed subtitle decisions were recorded; approved text changes were published atomically.",
	}, nil
}

func (h AssetHandler) restoreSubtitleProofreadingRevision(input subtitleProofreadingRestoreInput) (subtitleProofreadingRestoreResult, error) {
	mediaPath, _, err := h.resolveOwnedSubtitleAsset(input.Path)
	if err != nil {
		return subtitleProofreadingRestoreResult{}, err
	}
	source, err := resolveExternalSubtitleTranslationSource(mediaPath, input.SubtitlePath)
	if err != nil || filepath.Clean(source.ExternalPath) != filepath.Clean(input.SubtitlePath) {
		return subtitleProofreadingRestoreResult{}, errors.New("proofreading revision target is not an owned sidecar")
	}
	if err := rejectProofreadingSymlink(source.ExternalPath); err != nil {
		return subtitleProofreadingRestoreResult{}, err
	}
	manifest, prior, err := loadSubtitleProofreadingRevision(input.RevisionID)
	if err != nil {
		return subtitleProofreadingRestoreResult{}, err
	}
	if manifest.MediaPath != mediaPath || manifest.SidecarPath != source.ExternalPath || manifest.Format != source.Representation || source.SourceRevision != input.Expected {
		return subtitleProofreadingRestoreResult{}, errors.New("proofreading revision does not match the current sidecar identity")
	}
	current, currentBytes, err := publishedSubtitleRevisionFor(source.ExternalPath, source.LogicalTransformKey)
	if err != nil || current.ContentHash != manifest.AfterContentHash || !utf8.Valid(prior) || !validTranslatedSubtitleSidecar(source.Representation, prior) {
		return subtitleProofreadingRestoreResult{}, errors.New("sidecar changed after the stored proofreading revision; restore is stale")
	}
	if len(prior) > subtitleProofreadingMaxBytes {
		return subtitleProofreadingRestoreResult{}, errors.New("stored subtitle revision exceeds the restore size bound")
	}
	rollbackManifest := subtitleProofreadingRevisionManifest{
		MediaPath: mediaPath, SidecarPath: source.ExternalPath, Format: source.Representation,
		LogicalTransformKey: source.LogicalTransformKey, BeforeRevision: input.Expected,
		AfterContentHash: subtitleProofreadingTextHash(string(prior)), Provider: manifest.Provider,
		Model: manifest.Model, ContractVersion: subtitleProofreadingContractVersion,
		Kind: "restore", CreatedAt: time.Now().UTC(),
	}
	subtitlePublicationMu.Lock()
	defer subtitlePublicationMu.Unlock()
	lockedRevision, _, err := publishedSubtitleRevisionFor(source.ExternalPath, source.LogicalTransformKey)
	if err != nil || sourceRevisionKeyForExternal(source.ExternalPath, source.LogicalTransformKey) != input.Expected {
		return subtitleProofreadingRestoreResult{}, errors.New("sidecar changed before restore publication")
	}
	rollbackID, err := persistSubtitleProofreadingPriorRevision(rollbackManifest, currentBytes)
	if err != nil {
		return subtitleProofreadingRestoreResult{}, fmt.Errorf("preserve current revision before restore: %w", err)
	}
	if err := publishSubtitleProofreadingContentLocked(source.ExternalPath, lockedRevision, source.LogicalTransformKey, prior); err != nil {
		return subtitleProofreadingRestoreResult{}, err
	}
	newRevision, _, err := publishedSubtitleRevisionFor(source.ExternalPath, source.LogicalTransformKey)
	if err == nil {
		rollbackManifest.AfterContentHash = newRevision.ContentHash
		_ = updateSubtitleProofreadingManifest(rollbackID, rollbackManifest)
	}
	return subtitleProofreadingRestoreResult{
		Path: source.ExternalPath, RevisionID: rollbackID, SourceRevision: sourceRevisionKeyForExternal(source.ExternalPath, source.LogicalTransformKey),
		Restored: true, Message: "The stored subtitle revision was restored atomically.",
	}, nil
}

func subtitleProofreadingReportFinding(report subtitleProofreadingReport, cueID string) (subtitleProofreadingFinding, bool) {
	for _, finding := range report.Findings {
		if finding.CueID == cueID {
			return finding, true
		}
	}
	return subtitleProofreadingFinding{}, false
}

func sourceRevisionKeyForExternal(path, logicalTransform string) string {
	revision, err := captureTranslationExternalSourceRevision(path, logicalTransform)
	if err != nil {
		return ""
	}
	return revision.key()
}

func rejectProofreadingSymlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("subtitle sidecar is not a regular owned file")
	}
	return nil
}

func publishSubtitleProofreadingContentLocked(path string, expected publishedSubtitleRevision, logicalTransform string, content []byte) error {
	if int64(len(content)) <= 0 || len(content) > subtitleProofreadingMaxBytes {
		return errors.New("approved subtitle output is empty or oversized")
	}
	if !utf8.Valid(content) || !validTranslatedSubtitleSidecar(normalizeProofreadingExtension(path), content) {
		return errors.New("approved subtitle output failed encoding or format validation")
	}
	if err := rejectProofreadingSymlink(path); err != nil {
		return err
	}
	current, _, err := publishedSubtitleRevisionFor(path, logicalTransform)
	if err != nil || current != expected {
		return errors.New("subtitle source changed before atomic publication")
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return errors.New("subtitle target is unavailable for publication")
	}
	tempPath, temp, err := subtitleProofreadingTempFile(path)
	if err != nil {
		return fmt.Errorf("stage approved subtitle revision: %w", err)
	}
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(info.Mode()); err != nil {
		_ = temp.Close()
		return fmt.Errorf("set staged subtitle permissions: %w", err)
	}
	if _, err := temp.Write(content); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write staged subtitle: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync staged subtitle: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close staged subtitle: %w", err)
	}
	current, _, err = publishedSubtitleRevisionFor(path, logicalTransform)
	if err != nil || current != expected {
		return errors.New("subtitle source changed while staging approved text")
	}
	if err := publishStagedSubtitleFile(tempPath, path); err != nil {
		return fmt.Errorf("atomically publish approved subtitle: %w", err)
	}
	removeTemp = false
	if directory, err := os.Open(filepath.Dir(path)); err == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return nil
}

func subtitleProofreadingRevisionRoot() (string, error) {
	root := filepath.Join(publishedOCRStateRoot(), "subtitle-proofreading-revisions")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return "", err
	}
	return root, nil
}

func persistSubtitleProofreadingPriorRevision(manifest subtitleProofreadingRevisionManifest, before []byte) (string, error) {
	root, err := subtitleProofreadingRevisionRoot()
	if err != nil {
		return "", err
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	id := hex.EncodeToString(random[:])
	manifest.Version = 1
	manifest.RevisionID = id
	if manifest.CreatedAt.IsZero() {
		manifest.CreatedAt = time.Now().UTC()
	}
	temp, err := os.MkdirTemp(root, ".proofreading-revision-*")
	if err != nil {
		return "", err
	}
	cleanup := func() {
		_ = os.Remove(filepath.Join(temp, "prior-content"))
		_ = os.Remove(filepath.Join(temp, "manifest.json"))
		_ = os.Remove(temp)
	}
	priorPath := filepath.Join(temp, "prior-content")
	prior, err := os.OpenFile(priorPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		cleanup()
		return "", err
	}
	if _, err := prior.Write(before); err == nil {
		err = prior.Sync()
	}
	closeErr := prior.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		cleanup()
		return "", err
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		cleanup()
		return "", err
	}
	manifestPath := filepath.Join(temp, "manifest.json")
	if err := os.WriteFile(manifestPath, encoded, 0o600); err != nil {
		cleanup()
		return "", err
	}
	manifestFile, err := os.OpenFile(manifestPath, os.O_RDWR, 0)
	if err != nil {
		cleanup()
		return "", err
	}
	if err := manifestFile.Sync(); err != nil {
		_ = manifestFile.Close()
		cleanup()
		return "", err
	}
	_ = manifestFile.Close()
	if err := os.Rename(temp, filepath.Join(root, id)); err != nil {
		cleanup()
		return "", err
	}
	return id, nil
}

func updateSubtitleProofreadingManifest(id string, manifest subtitleProofreadingRevisionManifest) error {
	root, err := subtitleProofreadingRevisionRoot()
	if err != nil || !validSubtitleProofreadingRevisionKey(id) {
		return errors.New("invalid proofreading revision identity")
	}
	manifest.Version = 1
	manifest.RevisionID = id
	manifestPath := filepath.Join(root, id, "manifest.json")
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(manifestPath), ".manifest-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		return err
	}
	if _, err := temp.Write(encoded); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, manifestPath); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

func loadSubtitleProofreadingRevision(id string) (subtitleProofreadingRevisionManifest, []byte, error) {
	root, err := subtitleProofreadingRevisionRoot()
	if err != nil || !validSubtitleProofreadingRevisionKey(id) {
		return subtitleProofreadingRevisionManifest{}, nil, errors.New("proofreading revision identity is invalid")
	}
	base := filepath.Join(root, id)
	manifestBytes, err := os.ReadFile(filepath.Join(base, "manifest.json"))
	if err != nil {
		return subtitleProofreadingRevisionManifest{}, nil, errors.New("stored proofreading revision is unavailable")
	}
	var manifest subtitleProofreadingRevisionManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil || manifest.Version != 1 || manifest.RevisionID != id || manifest.ContractVersion != subtitleProofreadingContractVersion {
		return subtitleProofreadingRevisionManifest{}, nil, errors.New("stored proofreading revision is invalid")
	}
	prior, err := os.ReadFile(filepath.Join(base, "prior-content"))
	if err != nil || len(prior) == 0 || len(prior) > subtitleProofreadingMaxBytes {
		return subtitleProofreadingRevisionManifest{}, nil, errors.New("stored subtitle bytes are unavailable or oversized")
	}
	return manifest, prior, nil
}

func readProofreadingBoundedContent(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > subtitleProofreadingMaxBytes {
		return nil, errors.New("subtitle source is unavailable for bounded review")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, subtitleProofreadingMaxBytes+1))
	if err != nil || len(content) > subtitleProofreadingMaxBytes {
		return nil, errors.New("subtitle source exceeds the proofreading byte bound")
	}
	return content, nil
}
