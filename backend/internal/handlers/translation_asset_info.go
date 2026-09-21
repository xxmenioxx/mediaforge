package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

const translationContextContractVersion = "translation-context-v1"

type subtitleTranslationCapabilityRequest struct {
	Path   string                            `json:"path" binding:"required"`
	Source subtitleTranslationSourceSelector `json:"source" binding:"required"`
}

type subtitleTranslationCapabilityProvider struct {
	Name            string `json:"name,omitempty"`
	Model           string `json:"model,omitempty"`
	ModelSelectable bool   `json:"modelSelectable"`
}

type subtitleTranslationCapabilityContext struct {
	Status string `json:"status"`
	Source string `json:"source,omitempty"`
}

type subtitleTranslationCapabilityResponse struct {
	Source                subtitleTranslationSourceResponse     `json:"source"`
	Eligible              bool                                  `json:"eligible"`
	Available             bool                                  `json:"available"`
	Reason                string                                `json:"reason,omitempty"`
	SourceLanguages       []string                              `json:"sourceLanguages"`
	TargetLocales         []string                              `json:"targetLocales"`
	Provider              subtitleTranslationCapabilityProvider `json:"provider"`
	OutputRepresentations []string                              `json:"outputRepresentations"`
	Context               subtitleTranslationCapabilityContext  `json:"context"`
}

type subtitleTranslationStartRequest struct {
	Path                 string                            `json:"path" binding:"required"`
	Source               subtitleTranslationSourceSelector `json:"source" binding:"required"`
	SourceLanguage       string                            `json:"sourceLanguage"`
	TargetLocale         string                            `json:"targetLocale"`
	OutputRepresentation string                            `json:"outputRepresentation"`
}

type subtitleTranslationExecutionResult struct {
	Outputs           map[string][]byte
	EffectiveLanguage string
	Metrics           TranslationProviderMetrics
}

func (h AssetHandler) resolveSubtitleTranslationAssetSource(
	ctx context.Context,
	path string,
	selector subtitleTranslationSourceSelector,
) (string, subtitleTranslationSourceResponse, error) {
	if err := validateSubtitleTranslationSourceSelector(selector); err != nil {
		return "", subtitleTranslationSourceResponse{}, err
	}
	mediaPath, _, err := h.resolveOwnedSubtitleAsset(path)
	if err != nil {
		return "", subtitleTranslationSourceResponse{}, err
	}

	switch strings.ToLower(strings.TrimSpace(selector.Type)) {
	case "embedded":
		response, err := h.resolveEmbeddedSubtitleTranslationSource(ctx, mediaPath, selector.Embedded.StreamIndex)
		return mediaPath, response, err
	case "external":
		response, err := resolveExternalSubtitleTranslationSource(mediaPath, selector.External.SubtitlePath)
		return mediaPath, response, err
	default:
		return "", subtitleTranslationSourceResponse{}, fmt.Errorf("unsupported subtitle translation source type %q", selector.Type)
	}
}

func translationCapabilityOutputs(representation string) []string {
	switch strings.ToLower(strings.TrimSpace(representation)) {
	case translationRepresentationSRT:
		return []string{translationRepresentationSRT}
	case translationRepresentationASS:
		return []string{translationRepresentationASS}
	case translationRepresentationSSA:
		return []string{translationRepresentationSSA}
	case translationRepresentationBitmap:
		return []string{translationRepresentationSRT, translationRepresentationASS, "both"}
	default:
		return nil
	}
}

func translationCapabilityAvailability(settings translationSettings) (bool, string) {
	if !translationSettingsConfigured(settings) {
		return false, "Translation provider is not configured."
	}
	if settings.Readiness == translationReadinessReady &&
		settings.ReadinessIdentity == translationProviderConfigurationIdentity(settings) {
		if settings.Enabled {
			return true, ""
		}
		return false, "Subtitle translation is disabled."
	}

	switch settings.Readiness {
	case translationReadinessNotConfigured:
		return false, "Translation provider is not configured."
	case translationReadinessUntested:
		return false, "Translation provider configuration must be tested."
	case translationReadinessUnavailable:
		return false, "Translation provider is unavailable."
	case translationReadinessModelNotFound:
		return false, "Configured translation model is unavailable."
	case translationReadinessInvalidConfiguration:
		return false, "Translation provider configuration is invalid."
	case translationReadinessDisabled:
		return false, "Subtitle translation is disabled."
	default:
		return false, "Translation provider configuration must be tested."
	}
}

func (h AssetHandler) SubtitleTranslationCapability(c *gin.Context) {
	var request subtitleTranslationCapabilityRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid subtitle translation capability request"})
		return
	}
	_, source, err := h.resolveSubtitleTranslationAssetSource(c.Request.Context(), request.Path, request.Source)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"source":    subtitleTranslationSourceResponse{},
			"eligible":  false,
			"available": false,
			"reason":    boundedTranslationAssetInfoReason(err.Error()),
		})
		return
	}

	settings := loadTranslationSettings(h.db)
	available, reason := translationCapabilityAvailability(settings)
	provider := subtitleTranslationCapabilityProvider{
		Name:            strings.ToLower(strings.TrimSpace(settings.Provider)),
		Model:           strings.TrimSpace(settings.OllamaModel),
		ModelSelectable: strings.EqualFold(strings.TrimSpace(settings.Provider), translationProviderOllama),
	}
	c.JSON(http.StatusOK, subtitleTranslationCapabilityResponse{
		Source:                source,
		Eligible:              true,
		Available:             available,
		Reason:                reason,
		SourceLanguages:       []string{"auto", "en", "ja"},
		TargetLocales:         []string{"es-419", "es-ES"},
		Provider:              provider,
		OutputRepresentations: translationCapabilityOutputs(source.Representation),
		Context: subtitleTranslationCapabilityContext{
			Status: "unavailable",
			Source: "none",
		},
	})
}

func boundedTranslationAssetInfoReason(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > translationReadinessDiagnosticMaxLength {
		return value[:translationReadinessDiagnosticMaxLength]
	}
	return value
}

func requestedTranslationOutput(sourceRepresentation, requested string) (string, error) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	sourceRepresentation = strings.ToLower(strings.TrimSpace(sourceRepresentation))
	if sourceRepresentation == translationRepresentationBitmap {
		switch requested {
		case "":
			return translationRepresentationSRT, nil
		case translationRepresentationSRT, translationRepresentationASS, "both":
			return requested, nil
		default:
			return "", fmt.Errorf("bitmap translation does not support output representation %s", requested)
		}
	}
	if requested == "" {
		return sourceRepresentation, nil
	}
	return resolveTranslationOutputRepresentation(sourceRepresentation, requested)
}

func (h AssetHandler) StartSubtitleTranslation(c *gin.Context) {
	var input subtitleTranslationStartRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid subtitle translation request"})
		return
	}
	mediaPath, source, err := h.resolveSubtitleTranslationAssetSource(c.Request.Context(), input.Path, input.Source)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": boundedTranslationAssetInfoReason(err.Error())})
		return
	}
	requestedLanguage, err := normalizeTranslationLanguage(input.SourceLanguage, true)
	if err != nil {
		if strings.TrimSpace(input.SourceLanguage) == "" {
			requestedLanguage = "auto"
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	targetLocale, err := normalizeTranslationTargetLocale(input.TargetLocale)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	requestedRepresentation, err := requestedTranslationOutput(source.Representation, input.OutputRepresentation)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	settings := loadTranslationSettings(h.db)
	available, reason := translationCapabilityAvailability(settings)
	if !available {
		c.JSON(http.StatusConflict, gin.H{"error": reason})
		return
	}
	provider, err := h.translationProviderForSettings(settings)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": boundedTranslationAssetInfoReason(err.Error())})
		return
	}

	streamIndex := -1
	if source.StreamIndex != nil {
		streamIndex = *source.StreamIndex
	}
	request := subtitleTranslationOperationRequest{
		AssetPath:               mediaPath,
		SourceType:              source.Type,
		StreamIndex:             streamIndex,
		Format:                  source.Representation,
		SourceIdentity:          source.SourceIdentity,
		SourceReference:         source.ExternalPath,
		RequestedSourceLanguage: requestedLanguage,
		TargetLocale:            targetLocale,
		RequestedRepresentation: requestedRepresentation,
		Provider:                settings.Provider,
		ProviderModel:           settings.OllamaModel,
		ContextMode:             "none",
		ContextSource:           "none",
		SourceRevision:          source.SourceRevision,
	}
	if source.Type == "embedded" && source.StreamIndex != nil {
		request.SourceReference = strconv.Itoa(*source.StreamIndex)
	}
	runner := func(ctx context.Context, operationID, workspace string, operationRequest subtitleTranslationOperationRequest) error {
		return h.runAssetInfoSubtitleTranslation(ctx, operationID, workspace, operationRequest, provider, settings)
	}
	operationID, err := startSubtitleTranslationOperation(request, runner)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": boundedTranslationAssetInfoReason(err.Error())})
		return
	}
	subtitleExtractionOperations.RLock()
	operation := cloneSubtitleExtractionOperationSnapshot(subtitleExtractionOperations.items[operationID])
	subtitleExtractionOperations.RUnlock()
	c.JSON(http.StatusAccepted, gin.H{"operation": operation})
}

func (h AssetHandler) RetrySubtitleTranslation(c *gin.Context) {
	operationID := strings.TrimSpace(c.Param("id"))
	if operationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "operation id is required"})
		return
	}
	oldRequest, err := subtitleTranslationRetryRequest(operationID)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": boundedTranslationAssetInfoReason(err.Error())})
		return
	}
	settings := loadTranslationSettings(h.db)
	available, reason := translationCapabilityAvailability(settings)
	if !available {
		c.JSON(http.StatusConflict, gin.H{"error": reason})
		return
	}
	provider, err := h.translationProviderForSettings(settings)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": boundedTranslationAssetInfoReason(err.Error())})
		return
	}

	mediaPath, source, stream, err := h.resolveTranslationExecutionSource(c.Request.Context(), oldRequest)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": boundedTranslationAssetInfoReason(err.Error())})
		return
	}
	requestedLanguage, err := normalizeTranslationLanguage(oldRequest.RequestedSourceLanguage, true)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	requestedRepresentation, err := requestedTranslationOutput(source.Representation, oldRequest.RequestedRepresentation)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sourceRevision, err := translationSourceRevisionForResolvedSource(source, stream)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "subtitle source revision is unavailable"})
		return
	}
	freshRequest := oldRequest
	freshRequest.AssetPath = mediaPath
	freshRequest.SourceType = source.Type
	freshRequest.Format = source.Representation
	freshRequest.SourceIdentity = source.SourceIdentity
	freshRequest.RequestedSourceLanguage = requestedLanguage
	freshRequest.RequestedRepresentation = requestedRepresentation
	freshRequest.Provider = settings.Provider
	freshRequest.ProviderModel = settings.OllamaModel
	freshRequest.SourceRevision = sourceRevision.key()
	if source.StreamIndex != nil {
		freshRequest.StreamIndex = *source.StreamIndex
	}
	if source.Type == "external" {
		freshRequest.SourceReference = source.ExternalPath
	} else if source.StreamIndex != nil {
		freshRequest.SourceReference = strconv.Itoa(*source.StreamIndex)
	}
	runner := func(ctx context.Context, retryID, workspace string, request subtitleTranslationOperationRequest) error {
		return h.runAssetInfoSubtitleTranslation(ctx, retryID, workspace, request, provider, settings)
	}
	newOperationID, err := retrySubtitleTranslationOperationWithRequest(operationID, freshRequest, runner)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": boundedTranslationAssetInfoReason(err.Error())})
		return
	}
	subtitleExtractionOperations.RLock()
	operation := cloneSubtitleExtractionOperationSnapshot(subtitleExtractionOperations.items[newOperationID])
	subtitleExtractionOperations.RUnlock()
	c.JSON(http.StatusAccepted, gin.H{"operation": operation})
}

func (h AssetHandler) translationProviderForSettings(settings translationSettings) (TranslationProvider, error) {
	if h.translationProviderFactory != nil {
		return h.translationProviderFactory(settings)
	}
	return translationProviderForSettings(settings)
}

func (h AssetHandler) runAssetInfoSubtitleTranslation(
	ctx context.Context,
	operationID, workspace string,
	request subtitleTranslationOperationRequest,
	provider TranslationProvider,
	settings translationSettings,
) error {
	updateSubtitleTranslationProgress(operationID, "preparing", 0, 0, "Preparing subtitle translation")
	mediaPath, source, stream, err := h.resolveTranslationExecutionSource(ctx, request)
	if err != nil {
		return err
	}
	if source.SourceRevision != request.SourceRevision {
		return staleTranslationSourceError("subtitle source changed before translation")
	}
	sourceRevision, err := translationSourceRevisionForResolvedSource(source, stream)
	if err != nil || sourceRevision.key() != request.SourceRevision {
		return staleTranslationSourceError("subtitle source revision changed before translation")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	var outputs map[string][]byte
	var effectiveLanguage string
	var metrics TranslationProviderMetrics
	if source.Representation == translationRepresentationBitmap {
		if stream == nil {
			return errors.New("embedded bitmap subtitle stream is unavailable")
		}
		updateSubtitleTranslationProgress(operationID, "ocr", 0, 0, "Preparing canonical OCR subtitles")
		ocrLanguage := request.RequestedSourceLanguage
		if strings.EqualFold(strings.TrimSpace(ocrLanguage), "auto") {
			ocrLanguage = ""
		}
		ocrLanguage = normalizedOCRLanguage(ocrLanguage, source.Language)
		result, err := executeBitmapTranslation(ctx, bitmapTranslationExecutionRequest{
			MediaPath:               mediaPath,
			Stream:                  *stream,
			RequestedLanguage:       request.RequestedSourceLanguage,
			OCRLanguage:             ocrLanguage,
			SourceLanguageMetadata:  []string{source.Language},
			OCRMode:                 "clean",
			SourceIdentity:          request.SourceIdentity,
			SourceRepresentation:    source.Representation,
			TargetLocale:            request.TargetLocale,
			RequestedRepresentation: request.RequestedRepresentation,
			Provider:                provider,
			Settings:                settings,
			OnProgress: func(processed, total int) {
				updateSubtitleTranslationProgress(operationID, "ocr", processed, total, "Preparing canonical OCR subtitles")
			},
			OnBatch: func(processed, total int) {
				updateSubtitleTranslationProgress(operationID, "translating", processed, total, "Translating subtitle batches")
			},
		})
		if err != nil {
			return err
		}
		outputs = result.Outputs
		effectiveLanguage = result.EffectiveLanguage
		metrics = result.Metrics
	} else {
		content, err := readAssetInfoTranslationSource(ctx, workspace, mediaPath, source, stream)
		if err != nil {
			return err
		}
		updateSubtitleTranslationProgress(operationID, "translating", 0, 0, "Translating subtitle cues")
		result, err := executeTextTranslation(ctx, source, content, request, provider, settings, nil, func(processed, total int) {
			updateSubtitleTranslationProgress(operationID, "translating", processed, total, "Translating subtitle batches")
		})
		if err != nil {
			return err
		}
		outputs = result.Outputs
		effectiveLanguage = result.EffectiveLanguage
		metrics = result.Metrics
	}
	if len(outputs) == 0 {
		return errors.New("translation produced no output")
	}

	updateSubtitleTranslationProgress(operationID, "validating", 1, 1, "Validating translated subtitles")
	targets, err := translationPublicationTargets(request, source, effectiveLanguage, metrics, operationID, outputs, mediaPath)
	if err != nil {
		return err
	}
	targets, err = stageSubtitleTranslationOutputs(workspace, outputs, targets)
	if err != nil {
		return err
	}
	updateSubtitleTranslationProgress(operationID, "publishing", -1, -1, "Publishing translated subtitles")
	publishedArtifacts, err := publishSubtitleTranslationOutputs(subtitleTranslationPublicationRequest{
		Ctx:            ctx,
		OperationID:    operationID,
		Workspace:      workspace,
		SourceRevision: sourceRevision,
		RecheckSource: func(expected translationSourceRevision) error {
			return h.recheckAssetInfoTranslationSource(ctx, request, expected)
		},
		Targets: targets,
	})
	if err == nil {
		setSubtitleTranslationResult(operationID, publishedArtifacts)
	}
	return err
}

func (h AssetHandler) resolveTranslationExecutionSource(ctx context.Context, request subtitleTranslationOperationRequest) (string, subtitleTranslationSourceResponse, *FFProbeStream, error) {
	if request.SourceType == "external" {
		mediaPath, source, err := h.resolveSubtitleTranslationAssetSource(ctx, request.AssetPath, subtitleTranslationSourceSelector{
			Type:     "external",
			External: &externalSubtitleTranslationSource{SubtitlePath: request.SourceReference},
		})
		return mediaPath, source, nil, err
	}
	if request.SourceType != "embedded" {
		return "", subtitleTranslationSourceResponse{}, nil, fmt.Errorf("unsupported subtitle translation source type %q", request.SourceType)
	}
	stream, err := strconv.Atoi(request.SourceReference)
	if err != nil {
		stream = request.StreamIndex
	}
	mediaPath, source, err := h.resolveSubtitleTranslationAssetSource(ctx, request.AssetPath, subtitleTranslationSourceSelector{
		Type:     "embedded",
		Embedded: &embeddedSubtitleTranslationSource{StreamIndex: stream},
	})
	if err != nil {
		return "", subtitleTranslationSourceResponse{}, nil, err
	}
	streams, err := probeTranslationSourceStreams(ctx, mediaPath)
	if err != nil {
		return "", subtitleTranslationSourceResponse{}, nil, errors.New("unable to inspect embedded subtitle streams")
	}
	for index := range streams {
		if streams[index].Index == stream {
			return mediaPath, source, &streams[index], nil
		}
	}
	return "", subtitleTranslationSourceResponse{}, nil, fmt.Errorf("subtitle stream index %d does not exist", stream)
}

func readAssetInfoTranslationSource(ctx context.Context, workspace, mediaPath string, source subtitleTranslationSourceResponse, stream *FFProbeStream) ([]byte, error) {
	if source.Type == "external" {
		return readBoundedTranslationSubtitle(source.ExternalPath)
	}
	if stream == nil || source.StreamIndex == nil {
		return nil, errors.New("embedded text subtitle stream is unavailable")
	}
	outputPath := filepath.Join(workspace, "embedded-source."+source.Representation)
	var stderr strings.Builder
	plan := MediaJobPlan{InputPath: mediaPath}
	args := textSubtitleExtractionArgs(plan, *source.StreamIndex, source.Representation, outputPath)
	if source.Representation == translationRepresentationASS || source.Representation == translationRepresentationSSA {
		format, muxer, supported := originalSubtitleExtractionFormat(stream.CodecName)
		if !supported || format != source.Representation {
			return nil, fmt.Errorf("embedded subtitle codec %q does not support representation %q", stream.CodecName, source.Representation)
		}
		args = originalSubtitleExtractionArgs(plan, *source.StreamIndex, muxer, outputPath)
	}
	if err := runSubtitleCommand(ctx, "ffmpeg", args, nil, &stderr); err != nil {
		return nil, fmt.Errorf("embedded subtitle extraction failed: %w", err)
	}
	return readBoundedTranslationOperationFile(outputPath, source.Representation)
}

func readBoundedTranslationOperationFile(path, representation string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > maxPublishedSubtitleBytes {
		return nil, errors.New("embedded subtitle output is unavailable for bounded translation")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxPublishedSubtitleBytes+1))
	if err != nil || int64(len(content)) > maxPublishedSubtitleBytes || !validTranslatedSubtitleSidecar(representation, content) {
		return nil, errors.New("embedded subtitle output is invalid")
	}
	return content, nil
}

func executeTextTranslation(
	ctx context.Context,
	source subtitleTranslationSourceResponse,
	content []byte,
	request subtitleTranslationOperationRequest,
	provider TranslationProvider,
	settings translationSettings,
	translationContext *TranslationContext,
	onBatch func(int, int),
) (subtitleTranslationExecutionResult, error) {
	if provider == nil {
		return subtitleTranslationExecutionResult{}, errors.New("text translation provider is required")
	}
	var cues []translationSourceCue
	var protected []translationProtectedDialogue
	var document translationASSDocument
	var srtCues []canonicalBitmapSubtitleCue
	if source.Representation == translationRepresentationSRT {
		srtCues = parseSRTBitmapSubtitleCues(string(content))
		for _, cue := range srtCues {
			cues = append(cues, translationSourceCue{SourceOrdinal: cue.SourceOrdinal, Text: cue.Text, Start: cue.Start, End: cue.End, TimingProvenance: cue.TimingProvenance})
		}
	} else {
		var err error
		document, err = parseTranslationASSDocument(source.Representation, string(content))
		if err != nil {
			return subtitleTranslationExecutionResult{}, err
		}
		cues = document.TranslationSourceCues(request.SourceIdentity, "")
		for _, event := range document.Events {
			if event.Dialogue.HasTranslatableText() {
				protected = append(protected, event.Dialogue)
			}
		}
	}
	if len(cues) == 0 {
		return subtitleTranslationExecutionResult{}, errors.New("subtitle source contains no usable translation cues")
	}
	language, err := resolveTranslationSourceLanguage(translationLanguageResolutionInput{
		RequestedLanguage:      request.RequestedSourceLanguage,
		SourceLanguageMetadata: []string{source.Language},
		Cues:                   cues,
		ProtectedDialogues:     protected,
		AdvisoryMediaContext:   translationContext,
	})
	if err != nil {
		return subtitleTranslationExecutionResult{}, err
	}
	envelopes := translationCueEnvelopesForSourceCues(request.SourceIdentity, source.Representation, "", cues)
	batches, err := translationBatchesForCueEnvelopes(language.EffectiveLanguage, request.TargetLocale, envelopes, translationContext, settings)
	if err != nil {
		return subtitleTranslationExecutionResult{}, err
	}
	if len(batches) == 0 {
		return subtitleTranslationExecutionResult{}, errors.New("subtitle source contains no translation batches")
	}
	translated := make([]TranslatedCue, 0, len(envelopes))
	metrics := TranslationProviderMetrics{}
	for index, batch := range batches {
		if err := ctx.Err(); err != nil {
			return subtitleTranslationExecutionResult{}, err
		}
		result, err := provider.Translate(ctx, batch)
		if err != nil {
			return subtitleTranslationExecutionResult{}, err
		}
		validated, err := validateTranslationBatchResult(batch, result)
		if err != nil {
			return subtitleTranslationExecutionResult{}, fmt.Errorf("validate translated text batch %d: %w", index, err)
		}
		translated = append(translated, validated...)
		metrics = aggregateTranslationProviderMetrics(metrics, result.Metrics)
		if onBatch != nil {
			onBatch(index+1, len(batches))
		}
	}
	byOrdinal := make(map[int]string, len(translated))
	byID := make(map[string]string, len(translated))
	for _, result := range translated {
		byID[result.ID] = result.Text
	}
	for _, envelope := range envelopes {
		text, ok := byID[envelope.StableID]
		if !ok {
			return subtitleTranslationExecutionResult{}, errors.New("translated cue result did not match source cue")
		}
		byOrdinal[envelope.SourceOrdinal] = text
	}
	output, err := requestedTranslationOutput(source.Representation, request.RequestedRepresentation)
	if err != nil {
		return subtitleTranslationExecutionResult{}, err
	}
	var outputs map[string][]byte
	if source.Representation == translationRepresentationSRT {
		content, err := serializeTranslatedTextCues(source.Representation, srtCues, byOrdinal, output)
		if err != nil {
			return subtitleTranslationExecutionResult{}, err
		}
		outputs = map[string][]byte{translationRepresentationSRT: content}
	} else {
		content, err := serializeTranslatedASSDocument(document, byOrdinal, output)
		if err != nil {
			return subtitleTranslationExecutionResult{}, err
		}
		outputs = map[string][]byte{output: content}
	}
	for format, outputContent := range outputs {
		if !validTranslatedSubtitleSidecar(format, outputContent) {
			return subtitleTranslationExecutionResult{}, fmt.Errorf("translated %s output failed validation", format)
		}
	}
	return subtitleTranslationExecutionResult{Outputs: outputs, EffectiveLanguage: language.EffectiveLanguage, Metrics: metrics}, nil
}

func translationSourceRevisionForResolvedSource(source subtitleTranslationSourceResponse, stream *FFProbeStream) (translationSourceRevision, error) {
	if source.Type == "external" {
		return captureTranslationExternalSourceRevision(source.ExternalPath, source.LogicalTransformKey)
	}
	if stream == nil {
		return translationSourceRevision{}, errors.New("embedded subtitle stream is unavailable")
	}
	info, err := os.Stat(source.SourceIdentityPath())
	if err != nil {
		return translationSourceRevision{}, err
	}
	representation, err := translationSourceRepresentationForCodec(stream.CodecName)
	if err != nil {
		return translationSourceRevision{}, err
	}
	return captureTranslationEmbeddedSourceRevision(translationEmbeddedSourceSnapshot{
		SourcePath:            source.SourceIdentityPath(),
		SourceSize:            info.Size(),
		SourceModTimeUnixNano: info.ModTime().UnixNano(),
		SourceSnapshotVersion: translationEmbeddedSourceSnapshotVersion,
		StreamIndex:           stream.Index,
		StreamID:              translationStreamID(*stream),
		StreamSignature:       subtitleStreamSignature(translationRoleStreamSummary(*stream, representation)),
		StreamCodec:           strings.ToLower(strings.TrimSpace(stream.CodecName)),
		SourceRepresentation:  representation,
	})
}

func (source subtitleTranslationSourceResponse) SourceIdentityPath() string {
	if source.Type == "external" {
		return source.ExternalPath
	}
	return source.SourcePath
}

func (h AssetHandler) recheckAssetInfoTranslationSource(ctx context.Context, request subtitleTranslationOperationRequest, expected translationSourceRevision) error {
	_, source, stream, err := h.resolveTranslationExecutionSource(ctx, request)
	if err != nil {
		return errTranslationSourceStale
	}
	actual, err := translationSourceRevisionForResolvedSource(source, stream)
	if err != nil {
		return errTranslationSourceStale
	}
	if request.SourceType == "external" {
		return recheckTranslationExternalSourceRevision(expected)
	}
	return recheckTranslationEmbeddedSourceRevision(expected, actual.EmbeddedSnapshot())
}

func translationPublicationTargets(
	request subtitleTranslationOperationRequest,
	source subtitleTranslationSourceResponse,
	effectiveLanguage string,
	metrics TranslationProviderMetrics,
	operationID string,
	outputs map[string][]byte,
	mediaPath string,
) ([]subtitleTranslationPublicationTarget, error) {
	formats := make([]string, 0, len(outputs))
	for format := range outputs {
		formats = append(formats, format)
	}
	if request.RequestedRepresentation == "both" {
		formats = []string{translationRepresentationSRT, translationRepresentationASS}
	} else if len(formats) == 0 {
		return nil, errors.New("translation output set is empty")
	}
	used := make(map[string]struct{}, len(formats))
	base := strings.TrimSuffix(mediaPath, filepath.Ext(mediaPath))
	targets := make([]subtitleTranslationPublicationTarget, 0, len(formats))
	for _, format := range formats {
		identity, err := newTranslationIdentity(translationIdentityInput{
			SourceArtifactIdentity:  source.SourceIdentity,
			SourceRevision:          request.SourceRevision,
			SourceRepresentation:    source.Representation,
			SourceProvenance:        source.Type + ":" + source.Codec,
			RequestedSourceLanguage: request.RequestedSourceLanguage,
			EffectiveSourceLanguage: effectiveLanguage,
			TargetLocale:            request.TargetLocale,
			Provider:                request.Provider,
			ProviderContractVersion: translationProviderContractVersion,
			ProviderModel:           request.ProviderModel,
			ContextContractVersion:  translationContextContractVersion,
			ContextMode:             request.ContextMode,
			OutputRepresentation:    format,
		})
		if err != nil {
			return nil, err
		}
		metadata := identity.metadata()
		provenance, err := buildSubtitleTranslationProvenance(metadata, operationID, request.ContextSource, "validated", metrics)
		if err != nil {
			return nil, err
		}
		metadata.Provenance = provenance
		artifact := SubtitleArtifact{
			ArtifactID:            subtitleTranslationArtifactID(SubtitleArtifact{ArtifactID: source.ArtifactID, StreamIndex: request.StreamIndex, Mode: "original", Format: source.Representation}, metadata),
			Type:                  "subtitle_sidecar",
			StreamIndex:           request.StreamIndex,
			SourceCodec:           source.Codec,
			SourceType:            source.Type,
			Format:                format,
			RequestedFormat:       request.RequestedRepresentation,
			Mode:                  "translation",
			OCRRequired:           source.Representation == translationRepresentationBitmap,
			Language:              metadata.TargetLocale,
			Default:               source.Default,
			Forced:                source.EffectiveForced,
			SourceForced:          source.SourceForced,
			Role:                  source.Role,
			RoleProvenance:        source.RoleProvenance,
			RoleConfidence:        source.RoleConfidence,
			RoleClassifierVersion: source.RoleClassifier,
			Title:                 source.Title,
			Status:                "ready",
			Translation:           &metadata,
		}
		finalPath, err := resolvedTranslatedSubtitleStagedPath(base, artifact, metadata, used)
		if err != nil {
			return nil, err
		}
		targets = append(targets, subtitleTranslationPublicationTarget{Format: format, FinalPath: finalPath, Artifact: artifact})
	}
	return targets, nil
}

func (revision translationSourceRevision) EmbeddedSnapshot() translationEmbeddedSourceSnapshot {
	if revision.Embedded == nil {
		return translationEmbeddedSourceSnapshot{}
	}
	return *revision.Embedded
}
