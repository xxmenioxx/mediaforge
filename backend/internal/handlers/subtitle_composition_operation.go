package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type SubtitleCompositionSourceRequest struct {
	SubtitleCompositionSource
	SourcePath string `json:"sourcePath"`
}

type SubtitleCompositionRequest struct {
	Path              string                             `json:"path"`
	Primary           SubtitleCompositionSourceRequest   `json:"primary"`
	Supplementals     []SubtitleCompositionSourceRequest `json:"supplementals"`
	ObservedRevisions map[string]string                  `json:"observedRevisions"`
	Policy            *SubtitleCompositionPolicy         `json:"policy,omitempty"`
	PublicationMode   string                             `json:"publicationMode"`
}

type subtitleCompositionOperationRequest struct {
	AssetPath       string
	OutputPath      string
	Primary         SubtitleCompositionDocument
	Supplementals   []SubtitleCompositionDocument
	Policy          SubtitleCompositionPolicy
	OutputIdentity  string
	PublicationMode string
}

type subtitleCompositionOperationView struct {
	Sources        []SubtitleCompositionSource   `json:"sources"`
	Policy         SubtitleCompositionPolicy     `json:"policy"`
	OutputIdentity string                        `json:"outputIdentity"`
	OutputPath     string                        `json:"outputPath"`
	Provenance     SubtitleCompositionProvenance `json:"provenance"`
}

func currentSubtitleCompositionSourceRevision(source SubtitleCompositionSource) (string, error) {
	identity := strings.TrimSpace(source.Identity)
	path := strings.TrimPrefix(identity, "external:")
	if strings.HasPrefix(identity, "embedded:") {
		path = strings.TrimPrefix(identity, "embedded:")
		if marker := strings.LastIndex(path, "#stream="); marker >= 0 {
			path = path[:marker]
		}
	}
	if path == "" || (path == identity && !strings.HasPrefix(identity, "external:") && !strings.HasPrefix(identity, "embedded:")) {
		return "", fmt.Errorf("composition source identity is not revalidatable")
	}
	return mediaFileFingerprint(path)
}

func cloneSubtitleCompositionOperationRequest(request subtitleCompositionOperationRequest) subtitleCompositionOperationRequest {
	result := request
	result.Primary.Document.EventFormat = append([]string(nil), request.Primary.Document.EventFormat...)
	result.Primary.Document.Events = append([]assEvent(nil), request.Primary.Document.Events...)
	result.Primary.Document.Styles = append([]assStyleDefinition(nil), request.Primary.Document.Styles...)
	result.Supplementals = append([]SubtitleCompositionDocument(nil), request.Supplementals...)
	result.Policy.ExplicitStyleByClass = cloneCompositionStyleMap(request.Policy.ExplicitStyleByClass)
	return result
}

func cloneCompositionStyleMap(values map[CompositionCueClass]string) map[CompositionCueClass]string {
	if values == nil {
		return nil
	}
	result := make(map[CompositionCueClass]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func cloneCompositionCounts(values map[SubtitleCompositionDecisionKind]int) map[SubtitleCompositionDecisionKind]int {
	if values == nil {
		return nil
	}
	result := make(map[SubtitleCompositionDecisionKind]int, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func (h AssetHandler) ComposeSubtitles(c *gin.Context) {
	var input SubtitleCompositionRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	mediaPath, _, resolveErr := h.resolveOwnedSubtitleAsset(strings.TrimSpace(input.Path))
	if resolveErr != nil {
		writeSubtitleAssetResolutionError(c, resolveErr)
		return
	}
	if mediaPath == "" {
		return
	}
	request, err := h.prepareSubtitleCompositionRequest(mediaPath, input)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "stale") {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	operationID, err := startSubtitleCompositionOperation(request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"operationId": operationID, "status": "running", "kind": "compose", "phase": "preparing", "progress": 1, "outputIdentity": request.OutputIdentity})
}

func (h AssetHandler) prepareSubtitleCompositionRequest(mediaPath string, input SubtitleCompositionRequest) (subtitleCompositionOperationRequest, error) {
	if strings.TrimSpace(input.Primary.SourcePath) == "" || len(input.Supplementals) > compositionMaxSupplementals {
		return subtitleCompositionOperationRequest{}, fmt.Errorf("composition requires one primary source and at most %d supplementals", compositionMaxSupplementals)
	}
	style := subtitleStylingPolicyFromDB(h.db)
	policy := defaultSubtitleCompositionPolicy(style)
	if input.Policy != nil {
		policy = *input.Policy
		if policy.StylePolicy.Version == "" {
			policy.StylePolicy = style
		}
		if policy.StylePolicyVersion == "" {
			policy.StylePolicyVersion = policy.StylePolicy.Version
		}
	}
	if err := validateSubtitleCompositionPolicy(policy); err != nil {
		return subtitleCompositionOperationRequest{}, err
	}
	load := func(requested SubtitleCompositionSourceRequest, primary bool) (SubtitleCompositionDocument, error) {
		isEmbedded := strings.TrimSpace(requested.SourcePath) == "" && strings.HasPrefix(strings.TrimSpace(requested.Identity), "embedded:")
		path := ""
		if !isEmbedded {
			var err error
			path, err = validatedExternalSubtitlePath(mediaPath, filepath.Clean(strings.TrimSpace(requested.SourcePath)))
			if err != nil {
				return SubtitleCompositionDocument{}, err
			}
			info, statErr := os.Stat(path)
			if statErr != nil || info.IsDir() {
				return SubtitleCompositionDocument{}, fmt.Errorf("composition source is unavailable")
			}
		}
		identity := "external:" + path
		revisionPath := path
		if isEmbedded {
			identity = strings.TrimSpace(requested.Identity)
			revisionPath = mediaPath
		} else if requested.Identity != "" && strings.TrimSpace(requested.Identity) != identity {
			return SubtitleCompositionDocument{}, fmt.Errorf("composition source identity does not match the selected sidecar")
		}
		revision, err := mediaFileFingerprint(revisionPath)
		if err != nil {
			return SubtitleCompositionDocument{}, fmt.Errorf("composition source revision is unavailable")
		}
		if observed := strings.TrimSpace(input.ObservedRevisions[identity]); observed != "" && observed != revision {
			return SubtitleCompositionDocument{}, fmt.Errorf("composition source %q is stale", identity)
		}
		if requested.Revision != "" && requested.Revision != revision {
			return SubtitleCompositionDocument{}, fmt.Errorf("composition source %q is stale", identity)
		}
		representation := strings.ToLower(strings.TrimSpace(requested.Representation))
		if representation == "" {
			representation = strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
		}
		var document assDocument
		if isEmbedded {
			if requested.StreamIndex < 0 || representation == "bitmap" || representation == "pgs" || representation == "vobsub" {
				return SubtitleCompositionDocument{}, fmt.Errorf("bitmap composition requires the existing canonical OCR result")
			}
			temporary, tempErr := os.CreateTemp(filepath.Dir(mediaPath), ".mvforge-subtitle-compose-source-*.ass")
			if tempErr != nil {
				return SubtitleCompositionDocument{}, tempErr
			}
			temporaryPath := temporary.Name()
			_ = temporary.Close()
			defer os.Remove(temporaryPath)
			args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-i", mediaPath, "-map", fmt.Sprintf("0:%d", requested.StreamIndex), "-vn", "-an", "-c:s", "copy", "-f", "ass", temporaryPath}
			var stderr bytes.Buffer
			if commandErr := runSubtitleCommand(context.Background(), "ffmpeg", args, nil, &stderr); commandErr != nil {
				return SubtitleCompositionDocument{}, fmt.Errorf("embedded subtitle extraction failed: %s", fallback(strings.TrimSpace(stderr.String()), commandErr.Error()))
			}
			content, readErr := os.ReadFile(temporaryPath)
			if readErr != nil {
				return SubtitleCompositionDocument{}, readErr
			}
			document, err = parseASSDocument("ass", string(content))
		} else {
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return SubtitleCompositionDocument{}, readErr
			}
			switch representation {
			case "ass", "ssa":
				document, err = parseASSDocument(representation, string(content))
			case "srt":
				document, err = parseCompositionSRT(string(content))
			default:
				return SubtitleCompositionDocument{}, fmt.Errorf("unsupported composition representation %q", representation)
			}
		}
		if err != nil {
			return SubtitleCompositionDocument{}, err
		}
		source := requested.SubtitleCompositionSource
		source.Identity, source.Revision, source.Representation, source.Primary = identity, revision, representation, primary
		if err := sourceDescriptorValid(source); err != nil {
			return SubtitleCompositionDocument{}, err
		}
		return SubtitleCompositionDocument{Source: source, Document: document}, nil
	}
	primary, err := load(input.Primary, true)
	if err != nil {
		return subtitleCompositionOperationRequest{}, err
	}
	supplementals := make([]SubtitleCompositionDocument, 0, len(input.Supplementals))
	for _, requested := range input.Supplementals {
		document, loadErr := load(requested, false)
		if loadErr != nil {
			return subtitleCompositionOperationRequest{}, loadErr
		}
		if !cueLanguageCompatible(primary.Source, document.Source, policy) {
			return subtitleCompositionOperationRequest{}, fmt.Errorf("composition source %q is not language-compatible with primary", document.Source.Identity)
		}
		supplementals = append(supplementals, document)
	}
	sources := []SubtitleCompositionSource{primary.Source}
	for _, document := range supplementals {
		sources = append(sources, document.Source)
	}
	if strings.TrimSpace(input.PublicationMode) == "" {
		input.PublicationMode = "standard"
	}
	identity, err := SubtitleCompositionOutputIdentity(sources, policy, policy.StylePolicyVersion, "ass", input.PublicationMode)
	if err != nil {
		return subtitleCompositionOperationRequest{}, err
	}
	outputPath := filepath.Join(filepath.Dir(mediaPath), "."+strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath))+".subtitle-"+identity[len(identity)-12:]+".ass")
	return subtitleCompositionOperationRequest{AssetPath: mediaPath, OutputPath: outputPath, Primary: primary, Supplementals: supplementals, Policy: policy, OutputIdentity: identity, PublicationMode: input.PublicationMode}, nil
}

func startSubtitleCompositionOperation(request subtitleCompositionOperationRequest) (string, error) {
	workspace, err := os.MkdirTemp(filepath.Dir(request.OutputPath), ".mvforge-subtitle-compose-*")
	if err != nil {
		return "", err
	}
	operationID := fmt.Sprintf("subtitle-compose-%d", time.Now().UnixNano())
	workCtx, cancel := context.WithTimeout(context.Background(), subtitleOperationTimeout)
	sources := []SubtitleCompositionSource{request.Primary.Source}
	for _, supplemental := range request.Supplementals {
		sources = append(sources, supplemental.Source)
	}
	view := &subtitleCompositionOperationView{Sources: sources, Policy: request.Policy, OutputIdentity: request.OutputIdentity, OutputPath: request.OutputPath}
	now := time.Now()
	operation := &subtitleExtractionOperation{ID: operationID, Kind: "compose", Status: "running", Phase: "preparing", Progress: 1, AssetPath: request.AssetPath, Format: "ass", StylePolicyVersion: request.Policy.StylePolicyVersion, Composition: view, compositionRequest: &request, CreatedAt: now, UpdatedAt: now, cancel: cancel, publicationCommittedCh: make(chan struct{})}
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[operationID] = operation
	subtitleExtractionOperations.Unlock()
	go func() {
		defer cancel()
		runSubtitleCompositionOperation(workCtx, operationID, workspace, request)
	}()
	return operationID, nil
}

func runSubtitleCompositionOperation(ctx context.Context, operationID, workspace string, request subtitleCompositionOperationRequest) {
	cleanup := func() { _ = os.RemoveAll(workspace) }
	if err := ctx.Err(); err != nil {
		cleanup()
		finishSubtitleCompositionOperation(operationID, ctx, err, false)
		return
	}
	allSources := append([]SubtitleCompositionDocument{request.Primary}, request.Supplementals...)
	for _, source := range allSources {
		current, err := currentSubtitleCompositionSourceRevision(source.Source)
		if err != nil || current != source.Source.Revision {
			cleanup()
			finishSubtitleCompositionOperation(operationID, ctx, fmt.Errorf("composition source %q became stale before publication", source.Source.Identity), false)
			return
		}
	}
	updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
		operation.Phase = "reconciliation"
		operation.Progress = 35
		operation.Message = "Reconciling subtitle sources"
	})
	document, result, encoded, err := ComposeSubtitleDocuments(request.Policy, request.Primary, request.Supplementals...)
	_ = document
	if err != nil {
		cleanup()
		finishSubtitleCompositionOperation(operationID, ctx, err, false)
		return
	}
	result.Provenance.OutputIdentity = request.OutputIdentity
	result.Provenance.OutputPath = request.OutputPath
	sources := []SubtitleCompositionSource{request.Primary.Source}
	for _, supplemental := range request.Supplementals {
		sources = append(sources, supplemental.Source)
	}
	result.Provenance.Preferred, result.Provenance.Default = subtitleCompositionPromotion(request.Policy, sources)
	view := &subtitleCompositionOperationView{Sources: sources, Policy: request.Policy, OutputIdentity: request.OutputIdentity, OutputPath: request.OutputPath, Provenance: result.Provenance}
	updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
		operation.Composition = view
		operation.Phase = "validation"
		operation.Progress = 70
	})
	stagePath := filepath.Join(workspace, filepath.Base(request.OutputPath))
	if err := os.WriteFile(stagePath, encoded, 0o644); err != nil {
		cleanup()
		finishSubtitleCompositionOperation(operationID, ctx, err, false)
		return
	}
	if err := publishSubtitleComposition(operationID, ctx, stagePath, request.OutputPath, view); err != nil {
		cleanup()
		finishSubtitleCompositionOperation(operationID, ctx, err, false)
		return
	}
	cleanup()
	updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
		operation.Status = "completed"
		operation.Phase = "completed"
		operation.Progress = 100
		operation.Message = "Subtitle composition completed"
		operation.Result = &SubtitleExtractionResult{Created: []string{request.OutputPath}, Artifacts: []SubtitleArtifact{{ArtifactID: request.OutputIdentity, Type: "subtitle_composite", Format: "ass", Mode: "composition", StagedPath: stagePath, PublishedPath: request.OutputPath, Status: "ready"}}}
	})
}

func finishSubtitleCompositionOperation(operationID string, ctx context.Context, err error, committed bool) {
	if committed || subtitleOperationPublicationCommitted(operationID) {
		return
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
			operation.Status = "cancelled"
			operation.Phase = "cleanup"
			operation.Message = "Subtitle composition cancelled"
		})
		return
	}
	updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
		operation.Status = "error"
		operation.Phase = "error"
		operation.Message = "Subtitle composition failed"
		operation.Error = err.Error()
	})
}

func publishSubtitleComposition(operationID string, ctx context.Context, stagePath, outputPath string, view *subtitleCompositionOperationView) error {
	subtitlePublicationMu.Lock()
	defer subtitlePublicationMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	metadataPath := outputPath + ".composition.json"
	if metadata, err := os.ReadFile(metadataPath); err == nil {
		var existing subtitleCompositionOperationView
		if json.Unmarshal(metadata, &existing) == nil && existing.OutputIdentity == view.OutputIdentity {
			markSubtitlePublicationCommitted(operationID)
			return nil
		}
		return fmt.Errorf("composition output path already contains a different identity")
	}
	if _, err := os.Stat(outputPath); err == nil {
		return fmt.Errorf("composition output path already exists without matching identity metadata")
	} else if !os.IsNotExist(err) {
		return err
	}
	metadata, err := json.Marshal(view)
	if err != nil {
		return err
	}
	metadataStage := stagePath + ".composition.json"
	if err := os.WriteFile(metadataStage, metadata, 0o644); err != nil {
		return err
	}
	if err := os.Link(stagePath, outputPath); err != nil {
		_ = os.Remove(metadataStage)
		return err
	}
	if err := os.Link(metadataStage, metadataPath); err != nil {
		_ = os.Remove(outputPath)
		_ = os.Remove(metadataStage)
		return err
	}
	markSubtitlePublicationCommitted(operationID)
	return nil
}

func (h AssetHandler) retrySubtitleComposition(c *gin.Context, operationID string) {
	subtitleExtractionOperations.RLock()
	original, exists := subtitleExtractionOperations.items[operationID]
	if !exists || original.compositionRequest == nil {
		subtitleExtractionOperations.RUnlock()
		c.JSON(http.StatusConflict, gin.H{"error": "subtitle composition operation is not retryable"})
		return
	}
	request := cloneSubtitleCompositionOperationRequest(*original.compositionRequest)
	subtitleExtractionOperations.RUnlock()
	for _, document := range append([]SubtitleCompositionDocument{request.Primary}, request.Supplementals...) {
		current, err := currentSubtitleCompositionSourceRevision(document.Source)
		if err != nil || current != document.Source.Revision {
			c.JSON(http.StatusConflict, gin.H{"error": "one or more frozen composition sources are stale", "code": "stale_source", "retryable": false})
			return
		}
	}
	newOperationID, err := startSubtitleCompositionOperation(request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	subtitleExtractionOperations.RLock()
	operation := cloneSubtitleExtractionOperationSnapshot(subtitleExtractionOperations.items[newOperationID])
	subtitleExtractionOperations.RUnlock()
	c.JSON(http.StatusAccepted, gin.H{"operation": operation})
}
