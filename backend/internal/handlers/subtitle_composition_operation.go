package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

type subtitleCompositionAttachmentRef struct {
	Identity          string `json:"identity"`
	Revision          string `json:"revision"`
	SourceIdentity    string `json:"sourceIdentity"`
	OriginalName      string `json:"originalName"`
	MIMEType          string `json:"mimeType,omitempty"`
	FontFormat        string `json:"fontFormat"`
	StreamIndex       int    `json:"streamIndex"`
	AttachmentOrdinal int    `json:"attachmentOrdinal"`
	Required          bool   `json:"required"`
}

type subtitleCompositionOperationRequest struct {
	AssetPath       string
	OutputPath      string
	Primary         SubtitleCompositionDocument
	Supplementals   []SubtitleCompositionDocument
	Policy          SubtitleCompositionPolicy
	OutputIdentity  string
	PublicationMode string
	AttachmentRefs  []subtitleCompositionAttachmentRef
}

type subtitleCompositionOperationView struct {
	Sources        []SubtitleCompositionSource        `json:"sources"`
	Policy         SubtitleCompositionPolicy          `json:"policy"`
	OutputIdentity string                             `json:"outputIdentity"`
	OutputPath     string                             `json:"outputPath"`
	Provenance     SubtitleCompositionProvenance      `json:"provenance"`
	AttachmentRefs []subtitleCompositionAttachmentRef `json:"attachmentRefs,omitempty"`
}

type subtitleCompositionStagedAttachment struct {
	Path     string
	SafeName string
	Decision SubtitleCompositionAttachmentDecision
}

var subtitleCompositionBeforePublicationHook func()

var subtitleCompositionRunCommand = func(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error {
	return runSubtitleCommand(ctx, name, args, stdout, stderr)
}

func extractEmbeddedSubtitleCompositionDocument(ctx context.Context, mediaPath string, stream FFProbeStream) (assDocument, error) {
	format, muxer, supported := originalSubtitleExtractionFormat(stream.CodecName)
	if !supported || isBitmapSubtitleCodecName(stream.CodecName) || (format != "ass" && format != "ssa" && format != "srt") {
		return assDocument{}, fmt.Errorf("embedded subtitle codec %q is not supported by the canonical text extraction boundary", stream.CodecName)
	}
	temporary, err := os.CreateTemp(filepath.Dir(mediaPath), ".mvforge-subtitle-compose-source-*")
	if err != nil {
		return assDocument{}, err
	}
	temporaryPath := temporary.Name()
	if closeErr := temporary.Close(); closeErr != nil {
		_ = os.Remove(temporaryPath)
		return assDocument{}, closeErr
	}
	defer os.Remove(temporaryPath)
	args := originalSubtitleExtractionArgs(MediaJobPlan{InputPath: mediaPath}, stream.Index, muxer, temporaryPath)
	var stderr bytes.Buffer
	if err := subtitleCompositionRunCommand(ctx, "ffmpeg", args, nil, &stderr); err != nil {
		return assDocument{}, fmt.Errorf("embedded subtitle extraction failed: %s", fallback(strings.TrimSpace(stderr.String()), err.Error()))
	}
	content, err := os.ReadFile(temporaryPath)
	if err != nil {
		return assDocument{}, err
	}
	if format == "srt" {
		return parseCompositionSRT(string(content))
	}
	return parseASSDocument(format, string(content))
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
	revision, err := mediaFileFingerprint(path)
	if err != nil {
		return "", err
	}
	if artifactPath := strings.TrimSpace(source.CanonicalOCRArtifactPath); artifactPath != "" {
		artifact, valid := loadPublishedCanonicalOCRSnapshot(artifactPath, source.Representation, source.CanonicalOCRLogicalTransform)
		if !valid || artifact == nil || artifact.OwnerIdentity != strings.TrimSpace(source.CanonicalOCRArtifactIdentity) || artifact.Evidence.Identity.SourcePath != path || artifact.Evidence.Identity.StreamIndex != source.StreamIndex || !artifact.Evidence.Complete || len(artifact.Evidence.Cues) == 0 {
			return "", fmt.Errorf("canonical OCR artifact is stale or unavailable")
		}
		artifactRevision, artifactErr := mediaFileFingerprint(artifactPath)
		if artifactErr != nil || artifactRevision != source.CanonicalOCRArtifactRevision {
			return "", fmt.Errorf("canonical OCR artifact is stale")
		}
	}
	return revision, nil
}

func validateFrozenSubtitleCompositionSources(request subtitleCompositionOperationRequest) error {
	for _, source := range append([]SubtitleCompositionDocument{request.Primary}, request.Supplementals...) {
		if strings.TrimSpace(source.Source.Identity) == "" {
			continue
		}
		current, err := currentSubtitleCompositionSourceRevision(source.Source)
		if err != nil || current != source.Source.Revision {
			if err != nil {
				return fmt.Errorf("composition source %q became stale before publication: %w", source.Source.Identity, err)
			}
			return fmt.Errorf("composition source %q became stale before publication", source.Source.Identity)
		}
	}
	for _, attachment := range request.AttachmentRefs {
		if strings.TrimSpace(attachment.Revision) == "" || strings.TrimSpace(attachment.SourceIdentity) == "" {
			return fmt.Errorf("composition attachment %q has incomplete frozen provenance", attachment.Identity)
		}
		current, err := currentSubtitleCompositionAttachmentRevision(attachment.SourceIdentity)
		if err != nil || current != attachment.Revision {
			if err != nil {
				return fmt.Errorf("composition attachment %q became stale before publication: %w", attachment.Identity, err)
			}
			return fmt.Errorf("composition attachment %q became stale before publication", attachment.Identity)
		}
	}
	return nil
}

func currentSubtitleCompositionAttachmentRevision(identity string) (string, error) {
	path := strings.TrimPrefix(strings.TrimSpace(identity), "embedded:")
	if marker := strings.LastIndex(path, "#attachment="); marker >= 0 {
		path = path[:marker]
	}
	if path == "" || path == identity {
		return "", fmt.Errorf("composition attachment identity is not revalidatable")
	}
	return mediaFileFingerprint(path)
}

func cloneSubtitleCompositionOperationRequest(request subtitleCompositionOperationRequest) subtitleCompositionOperationRequest {
	result := request
	result.Primary.Document.EventFormat = append([]string(nil), request.Primary.Document.EventFormat...)
	result.Primary.Document.Events = append([]assEvent(nil), request.Primary.Document.Events...)
	result.Primary.Document.Styles = append([]assStyleDefinition(nil), request.Primary.Document.Styles...)
	result.Primary.Attachments = cloneSubtitleCompositionAttachments(request.Primary.Attachments)
	result.Supplementals = append([]SubtitleCompositionDocument(nil), request.Supplementals...)
	for index := range result.Supplementals {
		result.Supplementals[index].Document.EventFormat = append([]string(nil), request.Supplementals[index].Document.EventFormat...)
		result.Supplementals[index].Document.Events = append([]assEvent(nil), request.Supplementals[index].Document.Events...)
		result.Supplementals[index].Document.Styles = append([]assStyleDefinition(nil), request.Supplementals[index].Document.Styles...)
		result.Supplementals[index].Attachments = cloneSubtitleCompositionAttachments(request.Supplementals[index].Attachments)
	}
	result.AttachmentRefs = append([]subtitleCompositionAttachmentRef(nil), request.AttachmentRefs...)
	result.Policy.ExplicitStyleByClass = cloneCompositionStyleMap(request.Policy.ExplicitStyleByClass)
	return result
}

func cloneSubtitleCompositionAttachments(values []SubtitleCompositionAttachment) []SubtitleCompositionAttachment {
	result := make([]SubtitleCompositionAttachment, len(values))
	for index, value := range values {
		result[index] = value
		result[index].Content = append([]byte(nil), value.Content...)
	}
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
	primaryEmbedded := strings.TrimSpace(input.Primary.SourcePath) == "" && strings.HasPrefix(strings.TrimSpace(input.Primary.Identity), "embedded:")
	if (!primaryEmbedded && strings.TrimSpace(input.Primary.SourcePath) == "") || len(input.Supplementals) == 0 || len(input.Supplementals) > compositionMaxSupplementals {
		return subtitleCompositionOperationRequest{}, fmt.Errorf("composition requires one primary source and one to %d supplementals", compositionMaxSupplementals)
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
		isCanonicalOCR := strings.TrimSpace(requested.CanonicalOCRArtifactPath) != ""
		path := ""
		if isCanonicalOCR {
			if !isEmbedded {
				return SubtitleCompositionDocument{}, fmt.Errorf("canonical OCR composition requires an embedded bitmap source identity")
			}
			path = filepath.Clean(strings.TrimSpace(requested.CanonicalOCRArtifactPath))
			info, statErr := os.Stat(path)
			if statErr != nil || info.IsDir() {
				return SubtitleCompositionDocument{}, fmt.Errorf("canonical OCR artifact is unavailable")
			}
		} else if !isEmbedded {
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
		var embeddedStream FFProbeStream
		hasEmbeddedStream := false
		if isEmbedded && !isCanonicalOCR {
			if representation == "bitmap" || representation == "pgs" || representation == "vobsub" {
				return SubtitleCompositionDocument{}, fmt.Errorf("bitmap composition requires the existing canonical OCR result")
			}
			probe, _, probeErr := runFFProbe(mediaPath, 20)
			if probeErr != nil {
				return SubtitleCompositionDocument{}, fmt.Errorf("cannot validate embedded subtitle representation: %w", probeErr)
			}
			for _, stream := range probe.Streams {
				if stream.Index == requested.StreamIndex && isBitmapSubtitleCodecName(stream.CodecName) {
					return SubtitleCompositionDocument{}, fmt.Errorf("bitmap composition requires the existing canonical OCR result")
				}
				if stream.Index == requested.StreamIndex && strings.EqualFold(strings.TrimSpace(stream.CodecType), "subtitle") {
					embeddedStream, hasEmbeddedStream = stream, true
				}
			}
			if !hasEmbeddedStream {
				return SubtitleCompositionDocument{}, fmt.Errorf("embedded subtitle stream %d is unavailable", requested.StreamIndex)
			}
			resolvedRepresentation := subtitleRepresentationForCodec(embeddedStream.CodecName)
			if resolvedRepresentation == "" {
				return SubtitleCompositionDocument{}, fmt.Errorf("embedded subtitle codec %q is unsupported", embeddedStream.CodecName)
			}
			if representation != "" && representation != resolvedRepresentation {
				return SubtitleCompositionDocument{}, fmt.Errorf("embedded subtitle representation %q does not match codec %q", representation, embeddedStream.CodecName)
			}
			representation = resolvedRepresentation
			role := classifyEmbeddedSubtitleRole(mediaPath, embeddedStream)
			if role.Role == SubtitleRoleUnknown {
				return SubtitleCompositionDocument{}, fmt.Errorf("embedded subtitle stream %d has ambiguous semantic role", requested.StreamIndex)
			}
			if requested.Role != "" && requested.Role != role.Role {
				return SubtitleCompositionDocument{}, fmt.Errorf("embedded subtitle stream %d role does not match canonical classifier", requested.StreamIndex)
			}
			resolvedLanguage := normalizedTrackLanguage(embeddedStream.Tags["language"])
			if requested.Language != "" && normalizedTrackLanguage(requested.Language) != resolvedLanguage {
				return SubtitleCompositionDocument{}, fmt.Errorf("embedded subtitle stream %d language does not match canonical source", requested.StreamIndex)
			}
			requested.Language = resolvedLanguage
			requested.Role = role.Role
			requested.RoleProvenance = role.Provenance
			requested.RoleConfidence = role.Confidence
			requested.RoleClassifierVersion = role.ClassifierVersion
			requested.RoleOverrideState = role.OverrideState
		}
		if isCanonicalOCR {
			snapshot, valid := loadPublishedCanonicalOCRSnapshot(path, representation, requested.CanonicalOCRLogicalTransform)
			if !valid || snapshot == nil || !isBitmapSubtitleCodecName(snapshot.Evidence.Identity.StreamCodec) || snapshot.OwnerIdentity != strings.TrimSpace(requested.CanonicalOCRArtifactIdentity) || snapshot.Evidence.Identity.SourcePath != filepath.Clean(mediaPath) || snapshot.Evidence.Identity.StreamIndex != requested.StreamIndex || !snapshot.Evidence.Complete || len(snapshot.Evidence.Cues) == 0 {
				return SubtitleCompositionDocument{}, fmt.Errorf("canonical OCR artifact is stale or unavailable")
			}
			artifactRevision, artifactErr := mediaFileFingerprint(path)
			if artifactErr != nil || artifactRevision != strings.TrimSpace(requested.CanonicalOCRArtifactRevision) {
				return SubtitleCompositionDocument{}, fmt.Errorf("canonical OCR artifact is stale")
			}
		}
		var document assDocument
		if isCanonicalOCR {
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
				return SubtitleCompositionDocument{}, fmt.Errorf("unsupported canonical OCR representation %q", representation)
			}
		} else if isEmbedded {
			if requested.StreamIndex < 0 {
				return SubtitleCompositionDocument{}, fmt.Errorf("bitmap composition requires the existing canonical OCR result")
			}
			if !hasEmbeddedStream {
				return SubtitleCompositionDocument{}, fmt.Errorf("embedded subtitle stream %d is unavailable", requested.StreamIndex)
			}
			document, err = extractEmbeddedSubtitleCompositionDocument(context.Background(), mediaPath, embeddedStream)
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
	attachmentRefs, attachmentErr := frozenCompositionAttachmentRefs(mediaPath, sources)
	if attachmentErr != nil {
		return subtitleCompositionOperationRequest{}, attachmentErr
	}
	if strings.TrimSpace(input.PublicationMode) == "" {
		input.PublicationMode = "standard"
	}
	identity, err := SubtitleCompositionOutputIdentity(sources, policy, policy.StylePolicyVersion, "ass", input.PublicationMode)
	if err != nil {
		return subtitleCompositionOperationRequest{}, err
	}
	outputPath := filepath.Join(filepath.Dir(mediaPath), "."+strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath))+".subtitle-"+identity[len(identity)-12:]+".ass")
	return subtitleCompositionOperationRequest{AssetPath: mediaPath, OutputPath: outputPath, Primary: primary, Supplementals: supplementals, Policy: policy, OutputIdentity: identity, PublicationMode: input.PublicationMode, AttachmentRefs: attachmentRefs}, nil
}

func frozenCompositionAttachmentRefs(mediaPath string, sources []SubtitleCompositionSource) ([]subtitleCompositionAttachmentRef, error) {
	fontSource := ""
	for _, source := range sources {
		if strings.HasPrefix(strings.TrimSpace(source.Identity), "embedded:") && (strings.EqualFold(source.Representation, "ass") || strings.EqualFold(source.Representation, "ssa")) {
			fontSource = source.Identity
			break
		}
	}
	if fontSource == "" {
		return nil, nil
	}
	probe, _, err := runFFProbe(mediaPath, 20)
	if err != nil {
		return nil, fmt.Errorf("required font attachment inventory is unavailable: %w", err)
	}
	mediaRevision, err := mediaFileFingerprint(mediaPath)
	if err != nil {
		return nil, fmt.Errorf("required font attachment revision is unavailable: %w", err)
	}
	refs := []subtitleCompositionAttachmentRef{}
	used := map[string]struct{}{}
	attachmentOrdinal := 0
	for _, stream := range probe.Streams {
		if !strings.EqualFold(strings.TrimSpace(stream.CodecType), "attachment") {
			continue
		}
		filename := attachmentTagValue(stream.Tags, "filename")
		mimeType := attachmentTagValue(stream.Tags, "mimetype", "mime_type", "mime-type")
		kind, fontFormat := classifyAttachment(stream.CodecName, filename, mimeType)
		if strings.EqualFold(kind, "FONT") && supportedFontAttachmentFormat(fontFormat) {
			refs = append(refs, subtitleCompositionAttachmentRef{
				Identity: fmt.Sprintf("embedded:%s#attachment=%d", mediaPath, stream.Index), Revision: mediaRevision,
				SourceIdentity: fontSource, OriginalName: filename, MIMEType: mimeType, FontFormat: fontFormat,
				StreamIndex: stream.Index, AttachmentOrdinal: attachmentOrdinal, Required: true,
			})
			_ = uniqueFontAttachmentFilename(safeFontAttachmentFilename(filename, stream.Index, fontFormat), stream.Index, used)
		}
		attachmentOrdinal++
	}
	return refs, nil
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
	view := &subtitleCompositionOperationView{Sources: sources, Policy: request.Policy, OutputIdentity: request.OutputIdentity, OutputPath: request.OutputPath, AttachmentRefs: append([]subtitleCompositionAttachmentRef(nil), request.AttachmentRefs...)}
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
	if err := validateFrozenSubtitleCompositionSources(request); err != nil {
		cleanup()
		finishSubtitleCompositionOperation(operationID, ctx, fmt.Errorf("stale_source: %w", err), false)
		return
	}
	updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
		operation.Phase = "reconciliation"
		operation.Progress = 35
		operation.Message = "Reconciling subtitle sources"
	})
	primary, supplementals, stagedAttachments, err := stageSubtitleCompositionAttachments(ctx, workspace, request)
	if err != nil {
		cleanup()
		finishSubtitleCompositionOperation(operationID, ctx, err, false)
		return
	}
	document, result, encoded, err := ComposeSubtitleDocuments(request.Policy, primary, supplementals...)
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
	view := &subtitleCompositionOperationView{Sources: sources, Policy: request.Policy, OutputIdentity: request.OutputIdentity, OutputPath: request.OutputPath, Provenance: result.Provenance, AttachmentRefs: append([]subtitleCompositionAttachmentRef(nil), request.AttachmentRefs...)}
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
	if subtitleCompositionBeforePublicationHook != nil {
		subtitleCompositionBeforePublicationHook()
	}
	if err := publishSubtitleCompositionUnit(operationID, ctx, request, stagePath, request.OutputPath, view, stagedAttachments); err != nil {
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
		artifacts := []SubtitleArtifact{{ArtifactID: request.OutputIdentity, Type: "subtitle_composite", Format: "ass", Mode: "composition", StagedPath: stagePath, PublishedPath: request.OutputPath, Status: "ready"}}
		for _, attachment := range stagedAttachments {
			artifacts = append(artifacts, SubtitleArtifact{ArtifactID: attachment.Decision.Digest, Type: "font_attachment", Format: "ass", Mode: "composition", StagedPath: attachment.Path, PublishedPath: filepath.Join(strings.TrimSuffix(request.OutputPath, filepath.Ext(request.OutputPath))+".fonts", attachment.SafeName), Status: "ready"})
		}
		created := []string{request.OutputPath}
		for _, attachment := range stagedAttachments {
			created = append(created, filepath.Join(strings.TrimSuffix(request.OutputPath, filepath.Ext(request.OutputPath))+".fonts", attachment.SafeName))
		}
		operation.Result = &SubtitleExtractionResult{Created: created, Artifacts: artifacts}
	})
}

func stageSubtitleCompositionAttachments(ctx context.Context, workspace string, request subtitleCompositionOperationRequest) (SubtitleCompositionDocument, []SubtitleCompositionDocument, []subtitleCompositionStagedAttachment, error) {
	primary := request.Primary
	supplementals := append([]SubtitleCompositionDocument(nil), request.Supplementals...)
	if len(request.AttachmentRefs) == 0 {
		return primary, supplementals, nil, nil
	}
	frozen := make([]FontAttachmentArtifact, 0, len(request.AttachmentRefs))
	used := map[string]struct{}{}
	for _, ref := range request.AttachmentRefs {
		frozen = append(frozen, FontAttachmentArtifact{
			ArtifactID: fontAttachmentArtifactID(ref.StreamIndex), StreamIndex: ref.StreamIndex, AttachmentOrdinal: ref.AttachmentOrdinal,
			OriginalName: ref.OriginalName, MIMEType: ref.MIMEType, FontFormat: ref.FontFormat,
			SafeFilename: uniqueFontAttachmentFilename(safeFontAttachmentFilename(ref.OriginalName, ref.StreamIndex, ref.FontFormat), ref.StreamIndex, used),
			Status:       "planned",
		})
	}
	plan := MediaJobPlan{InputPath: request.AssetPath, OutputPath: filepath.Join(workspace, "composition.ass")}
	artifacts, err := generateFontAttachmentArtifactsWithProgress(ctx, plan, frozen, nil)
	if err != nil {
		return primary, supplementals, nil, err
	}
	inputs := make([]SubtitleCompositionAttachment, 0, len(artifacts))
	for index, artifact := range artifacts {
		content, readErr := os.ReadFile(artifact.StagedPath)
		if readErr != nil {
			return primary, supplementals, nil, readErr
		}
		ref := request.AttachmentRefs[index]
		inputs = append(inputs, SubtitleCompositionAttachment{SourceIdentity: ref.SourceIdentity, OriginalName: ref.OriginalName, MIMEType: ref.MIMEType, Content: content, Required: ref.Required})
	}
	decisions, err := reconcileSubtitleCompositionAttachments(inputs)
	if err != nil {
		return primary, supplementals, nil, err
	}
	if len(decisions) != len(artifacts) {
		return primary, supplementals, nil, fmt.Errorf("attachment reconciliation returned an incomplete decision set")
	}
	byDigest := map[string]string{}
	staged := make([]subtitleCompositionStagedAttachment, 0, len(decisions))
	for index, decision := range decisions {
		artifact := artifacts[index]
		if _, ok := byDigest[decision.Digest]; ok {
			_ = os.Remove(artifact.StagedPath)
		} else {
			target := filepath.Join(filepath.Dir(artifact.StagedPath), decision.SafeName)
			if target != artifact.StagedPath {
				if err := os.Rename(artifact.StagedPath, target); err != nil {
					return primary, supplementals, nil, fmt.Errorf("stage attachment %q: %w", decision.OriginalName, err)
				}
			}
			byDigest[decision.Digest] = target
			staged = append(staged, subtitleCompositionStagedAttachment{Path: target, SafeName: decision.SafeName, Decision: decision})
		}
		attachment := inputs[index]
		if attachment.SourceIdentity == primary.Source.Identity {
			primary.Attachments = append(primary.Attachments, attachment)
		} else {
			for supplementalIndex := range supplementals {
				if supplementals[supplementalIndex].Source.Identity == attachment.SourceIdentity {
					supplementals[supplementalIndex].Attachments = append(supplementals[supplementalIndex].Attachments, attachment)
					break
				}
			}
		}
	}
	return primary, supplementals, staged, nil
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
			operation.Retryable = true
		})
		return
	}
	updateSubtitleExtractionOperation(operationID, func(operation *subtitleExtractionOperation) {
		operation.Status = "error"
		operation.Phase = "error"
		operation.Message = "Subtitle composition failed"
		operation.Error = err.Error()
		operation.Retryable = !strings.Contains(strings.ToLower(err.Error()), "stale") && !strings.Contains(strings.ToLower(err.Error()), "already exists")
	})
}

func publishSubtitleComposition(operationID string, ctx context.Context, stagePath, outputPath string, view *subtitleCompositionOperationView) error {
	return publishSubtitleCompositionUnit(operationID, ctx, subtitleCompositionOperationRequest{OutputIdentity: view.OutputIdentity, OutputPath: outputPath}, stagePath, outputPath, view, nil)
}

func publishSubtitleCompositionUnit(operationID string, ctx context.Context, request subtitleCompositionOperationRequest, stagePath, outputPath string, view *subtitleCompositionOperationView, stagedAttachments []subtitleCompositionStagedAttachment) error {
	subtitlePublicationMu.Lock()
	defer subtitlePublicationMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateFrozenSubtitleCompositionSources(request); err != nil {
		return fmt.Errorf("stale_source: %w", err)
	}
	metadataPath := outputPath + ".composition.json"
	fontDir := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".fonts"
	if metadata, err := os.ReadFile(metadataPath); err == nil {
		var existing subtitleCompositionOperationView
		if json.Unmarshal(metadata, &existing) == nil && existing.OutputIdentity == view.OutputIdentity {
			if _, outputErr := os.Stat(outputPath); outputErr != nil {
				return fmt.Errorf("composition identity metadata exists without the canonical ASS output")
			}
			for _, attachment := range stagedAttachments {
				if _, attachmentErr := os.Stat(filepath.Join(fontDir, attachment.SafeName)); attachmentErr != nil {
					return fmt.Errorf("composition identity metadata exists without the complete attachment set")
				}
			}
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
	published := []string{}
	directoryCreated := false
	rollback := func() {
		for _, path := range published {
			_ = os.Remove(path)
		}
		_ = os.Remove(metadataStage)
		if directoryCreated {
			_ = os.Remove(fontDir)
		}
	}
	if err := os.Link(stagePath, outputPath); err != nil {
		rollback()
		return err
	}
	published = append(published, outputPath)
	if len(stagedAttachments) > 0 {
		if info, statErr := os.Stat(fontDir); os.IsNotExist(statErr) {
			if err := os.MkdirAll(fontDir, 0o755); err != nil {
				rollback()
				return err
			}
			directoryCreated = true
		} else if statErr != nil || !info.IsDir() {
			rollback()
			return fmt.Errorf("composition attachment destination is not a directory")
		}
	}
	for _, attachment := range stagedAttachments {
		if err := ctx.Err(); err != nil {
			rollback()
			return err
		}
		destination := filepath.Join(fontDir, attachment.SafeName)
		if _, err := os.Stat(destination); err == nil {
			rollback()
			return fmt.Errorf("composition attachment destination already exists: %s", destination)
		} else if !os.IsNotExist(err) {
			rollback()
			return err
		}
		if err := os.Link(attachment.Path, destination); err != nil {
			rollback()
			return fmt.Errorf("cannot publish composition attachment: %w", err)
		}
		published = append(published, destination)
	}
	if err := os.Link(metadataStage, metadataPath); err != nil {
		rollback()
		return err
	}
	published = append(published, metadataPath)
	markSubtitlePublicationCommitted(operationID)
	return nil
}

func (h AssetHandler) retrySubtitleComposition(c *gin.Context, operationID string) {
	request, err := subtitleCompositionRetryRequest(operationID)
	if errors.Is(err, errSubtitleCompositionRetryStaleSource) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "code": "stale_source", "retryable": false})
		return
	}
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "retryable": false})
		return
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

var errSubtitleCompositionRetryStaleSource = errors.New("one or more frozen composition sources are stale")

func subtitleCompositionRetryRequest(operationID string) (subtitleCompositionOperationRequest, error) {
	subtitleExtractionOperations.RLock()
	original, exists := subtitleExtractionOperations.items[operationID]
	if !exists {
		subtitleExtractionOperations.RUnlock()
		return subtitleCompositionOperationRequest{}, errors.New("subtitle composition operation is not found")
	}
	if original.Kind != "compose" || original.compositionRequest == nil {
		subtitleExtractionOperations.RUnlock()
		return subtitleCompositionOperationRequest{}, errors.New("subtitle composition operation is not retryable")
	}
	if original.Status == "running" {
		subtitleExtractionOperations.RUnlock()
		return subtitleCompositionOperationRequest{}, errors.New("running subtitle composition cannot be retried")
	}
	if original.Status != "error" && original.Status != "cancelled" {
		subtitleExtractionOperations.RUnlock()
		return subtitleCompositionOperationRequest{}, errors.New("only failed or cancelled subtitle composition can be retried")
	}
	if !original.Retryable {
		subtitleExtractionOperations.RUnlock()
		return subtitleCompositionOperationRequest{}, errors.New("subtitle composition operation is not retryable")
	}
	request := cloneSubtitleCompositionOperationRequest(*original.compositionRequest)
	subtitleExtractionOperations.RUnlock()
	if err := validateFrozenSubtitleCompositionSources(request); err != nil {
		return subtitleCompositionOperationRequest{}, fmt.Errorf("%w: %v", errSubtitleCompositionRetryStaleSource, err)
	}
	return request, nil
}
