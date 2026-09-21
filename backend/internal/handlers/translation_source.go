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

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
)

const translationEmbeddedSourceSnapshotVersion = "asset-info-ffprobe-v1"

type subtitleTranslationSourceRequest struct {
	Path   string                            `json:"path" binding:"required"`
	Source subtitleTranslationSourceSelector `json:"source" binding:"required"`
}

type subtitleTranslationSourceSelector struct {
	Type     string                             `json:"type" binding:"required"`
	Embedded *embeddedSubtitleTranslationSource `json:"embedded,omitempty"`
	External *externalSubtitleTranslationSource `json:"external,omitempty"`
}

type embeddedSubtitleTranslationSource struct {
	StreamIndex int `json:"streamIndex"`
}

type externalSubtitleTranslationSource struct {
	SubtitlePath string `json:"subtitlePath" binding:"required"`
}

// subtitleTranslationSourceResponse is deliberately metadata-only. It is the
// exact source contract for the later translation operation and never carries
// subtitle text, prompts, provider responses, or credentials.
type subtitleTranslationSourceResponse struct {
	Type            string               `json:"type"`
	SourceIdentity  string               `json:"sourceIdentity"`
	SourceRevision  string               `json:"sourceRevision"`
	Representation  string               `json:"representation"`
	StreamIndex     *int                 `json:"streamIndex,omitempty"`
	StreamID        string               `json:"streamId,omitempty"`
	ExternalPath    string               `json:"externalPath,omitempty"`
	ArtifactID      string               `json:"artifactId,omitempty"`
	Codec           string               `json:"codec"`
	Language        string               `json:"language,omitempty"`
	Title           string               `json:"title,omitempty"`
	Default         bool                 `json:"default"`
	SourceForced    bool                 `json:"sourceForced"`
	EffectiveForced bool                 `json:"effectiveForced"`
	Role            SubtitleSemanticRole `json:"role"`
	RoleProvenance  string               `json:"roleProvenance,omitempty"`
	RoleConfidence  string               `json:"roleConfidence,omitempty"`
	RoleClassifier  string               `json:"roleClassifierVersion,omitempty"`
}

type subtitleAssetResolutionError struct {
	status  int
	message string
}

func (e *subtitleAssetResolutionError) Error() string { return e.message }

func subtitleAssetResolutionFailure(status int, message string) error {
	return &subtitleAssetResolutionError{status: status, message: message}
}

func writeSubtitleAssetResolutionError(c *gin.Context, err error) {
	var resolutionErr *subtitleAssetResolutionError
	if errors.As(err, &resolutionErr) {
		c.JSON(resolutionErr.status, gin.H{"error": resolutionErr.message})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

// resolveOwnedSubtitleAsset centralizes the ownership checks used by Asset
// Info source operations. The external subtitle management endpoints continue
// to use their existing response behavior, while translation can reuse the
// same exact inventory, archive, readability, and media-root rules.
func (h AssetHandler) resolveOwnedSubtitleAsset(path string) (string, models.AssetRecord, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || path == "" {
		return "", models.AssetRecord{}, subtitleAssetResolutionFailure(http.StatusBadRequest, "path is required")
	}
	var record models.AssetRecord
	if err := h.db.First(&record, "path = ?", path).Error; err != nil {
		return "", models.AssetRecord{}, subtitleAssetResolutionFailure(http.StatusNotFound, "asset not found in inventory")
	}
	if record.Status == "archive" {
		return "", models.AssetRecord{}, subtitleAssetResolutionFailure(http.StatusConflict, "archived originals cannot manage external subtitles")
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", models.AssetRecord{}, subtitleAssetResolutionFailure(http.StatusNotFound, "asset is not readable from the backend container")
	}
	allowed, err := h.pathBelongsToLibrary(path)
	if err != nil {
		return "", models.AssetRecord{}, err
	}
	if !allowed {
		return "", models.AssetRecord{}, subtitleAssetResolutionFailure(http.StatusForbidden, "asset is outside configured media roots")
	}
	return path, record, nil
}

func (h AssetHandler) ResolveSubtitleTranslationSource(c *gin.Context) {
	var request subtitleTranslationSourceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid subtitle translation source request"})
		return
	}
	if err := validateSubtitleTranslationSourceSelector(request.Source); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	mediaPath, _, err := h.resolveOwnedSubtitleAsset(request.Path)
	if err != nil {
		writeSubtitleAssetResolutionError(c, err)
		return
	}

	var response subtitleTranslationSourceResponse
	switch strings.ToLower(strings.TrimSpace(request.Source.Type)) {
	case "embedded":
		response, err = h.resolveEmbeddedSubtitleTranslationSource(c.Request.Context(), mediaPath, request.Source.Embedded.StreamIndex)
	case "external":
		response, err = resolveExternalSubtitleTranslationSource(mediaPath, request.Source.External.SubtitlePath)
	}
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, response)
}

func validateSubtitleTranslationSourceSelector(selector subtitleTranslationSourceSelector) error {
	typeName := strings.ToLower(strings.TrimSpace(selector.Type))
	if typeName != "embedded" && typeName != "external" {
		return fmt.Errorf("unsupported subtitle translation source type %q", selector.Type)
	}
	if typeName == "embedded" && selector.Embedded == nil {
		return errors.New("embedded subtitle translation source is required")
	}
	if typeName == "external" && selector.External == nil {
		return errors.New("external subtitle translation source is required")
	}
	if selector.Embedded != nil && selector.External != nil {
		return errors.New("exactly one subtitle translation source variant is allowed")
	}
	if typeName == "embedded" && selector.External != nil {
		return errors.New("external source variant does not match embedded source type")
	}
	if typeName == "external" && selector.Embedded != nil {
		return errors.New("embedded source variant does not match external source type")
	}
	if typeName == "embedded" && selector.Embedded.StreamIndex < 0 {
		return errors.New("embedded subtitle stream index must not be negative")
	}
	if typeName == "external" && strings.TrimSpace(selector.External.SubtitlePath) == "" {
		return errors.New("subtitlePath is required")
	}
	return nil
}

func (h AssetHandler) resolveEmbeddedSubtitleTranslationSource(ctx context.Context, mediaPath string, streamIndex int) (subtitleTranslationSourceResponse, error) {
	streams, err := probeTranslationSourceStreams(ctx, mediaPath)
	if err != nil {
		return subtitleTranslationSourceResponse{}, errors.New("unable to inspect embedded subtitle streams")
	}
	var selected *FFProbeStream
	for index := range streams {
		if streams[index].Index == streamIndex {
			selected = &streams[index]
			break
		}
	}
	if selected == nil {
		return subtitleTranslationSourceResponse{}, fmt.Errorf("subtitle stream index %d does not exist", streamIndex)
	}
	if strings.ToLower(strings.TrimSpace(selected.CodecType)) != "subtitle" {
		return subtitleTranslationSourceResponse{}, fmt.Errorf("selected stream index %d is not a subtitle stream", streamIndex)
	}
	representation, err := translationSourceRepresentationForCodec(selected.CodecName)
	if err != nil {
		return subtitleTranslationSourceResponse{}, err
	}
	info, err := os.Stat(mediaPath)
	if err != nil || info.IsDir() {
		return subtitleTranslationSourceResponse{}, errors.New("embedded subtitle source is not readable")
	}
	streamSummary := translationRoleStreamSummary(*selected, representation)
	role := classifySubtitleRole(models.ScanResult{
		Path:      mediaPath,
		SizeBytes: info.Size(),
		UpdatedAt: info.ModTime(),
	}, streamSummary, nil)
	streamID := translationStreamID(*selected)
	snapshot := translationEmbeddedSourceSnapshot{
		SourcePath:            mediaPath,
		SourceSize:            info.Size(),
		SourceModTimeUnixNano: info.ModTime().UnixNano(),
		SourceSnapshotVersion: translationEmbeddedSourceSnapshotVersion,
		StreamIndex:           streamIndex,
		StreamID:              streamID,
		StreamSignature:       subtitleStreamSignature(streamSummary),
		StreamCodec:           strings.ToLower(strings.TrimSpace(selected.CodecName)),
		SourceRepresentation:  representation,
	}
	revision, err := captureTranslationEmbeddedSourceRevision(snapshot)
	if err != nil {
		return subtitleTranslationSourceResponse{}, fmt.Errorf("embedded subtitle source revision is unavailable")
	}
	revisionKey := revision.key()
	language := translationStreamLanguage(*selected)
	title := strings.TrimSpace(selected.Tags["title"])
	sourceIdentity := fmt.Sprintf("embedded:%s:%d:%s", filepath.Clean(mediaPath), streamIndex, revisionKey)
	effectiveForced := dispositionEnabled(selected.Disposition, "forced") || role.Role == SubtitleRoleForced
	return subtitleTranslationSourceResponse{
		Type:            "embedded",
		SourceIdentity:  sourceIdentity,
		SourceRevision:  revisionKey,
		Representation:  representation,
		StreamIndex:     translationIntPointer(streamIndex),
		StreamID:        streamID,
		ArtifactID:      subtitleArtifactID(streamIndex, "original", representation),
		Codec:           strings.ToLower(strings.TrimSpace(selected.CodecName)),
		Language:        language,
		Title:           title,
		Default:         dispositionEnabled(selected.Disposition, "default"),
		SourceForced:    dispositionEnabled(selected.Disposition, "forced"),
		EffectiveForced: effectiveForced,
		Role:            role.Role,
		RoleProvenance:  role.Provenance,
		RoleConfidence:  role.Confidence,
		RoleClassifier:  role.ClassifierVersion,
	}, nil
}

func resolveExternalSubtitleTranslationSource(mediaPath, requestedPath string) (subtitleTranslationSourceResponse, error) {
	subtitlePath, err := validatedExternalSubtitlePath(mediaPath, requestedPath)
	if err != nil {
		return subtitleTranslationSourceResponse{}, err
	}
	inventory, err := externalSubtitlesForMedia(mediaPath)
	if err != nil {
		return subtitleTranslationSourceResponse{}, errors.New("unable to inspect external subtitle inventory")
	}
	var selected *ExternalSubtitle
	for index := range inventory {
		if filepath.Clean(inventory[index].Path) == filepath.Clean(subtitlePath) {
			selected = &inventory[index]
			break
		}
	}
	if selected == nil {
		return subtitleTranslationSourceResponse{}, errors.New("external subtitle does not belong to this asset")
	}
	if _, err := readBoundedTranslationSubtitle(subtitlePath); err != nil {
		return subtitleTranslationSourceResponse{}, errors.New("external subtitle is not readable")
	}
	revision, err := captureTranslationExternalSourceRevision(subtitlePath, selected.LogicalTransformKey)
	if err != nil {
		return subtitleTranslationSourceResponse{}, errors.New("external subtitle source revision is unavailable")
	}
	revisionKey := revision.key()
	role := SubtitleRoleUnknown
	roleProvenance := "ambiguous/unknown"
	roleConfidence := "low"
	if selected.Forced {
		role = SubtitleRoleForced
		roleProvenance = "external-disposition"
		roleConfidence = "high"
	}
	return subtitleTranslationSourceResponse{
		Type:            "external",
		SourceIdentity:  fmt.Sprintf("external:%s:%s", filepath.Clean(subtitlePath), revisionKey),
		SourceRevision:  revisionKey,
		Representation:  strings.ToLower(strings.TrimSpace(selected.Format)),
		ExternalPath:    filepath.Clean(subtitlePath),
		ArtifactID:      fmt.Sprintf("external-subtitle:%s", revision.External.ArtifactIdentity),
		Codec:           strings.ToLower(strings.TrimSpace(selected.Format)),
		Language:        strings.ToLower(strings.TrimSpace(selected.Language)),
		Default:         selected.Default,
		SourceForced:    selected.Forced,
		EffectiveForced: selected.Forced,
		Role:            role,
		RoleProvenance:  roleProvenance,
		RoleConfidence:  roleConfidence,
	}, nil
}

func probeTranslationSourceStreams(ctx context.Context, path string) ([]FFProbeStream, error) {
	var stdout, stderr bytes.Buffer
	if err := runSubtitleCommand(ctx, "ffprobe", []string{
		"-v", "error",
		"-show_entries", "stream=index,id,codec_type,codec_name:stream_tags=language,title:stream_disposition=default,forced,comment,hearing_impaired",
		"-of", "json",
		path,
	}, &stdout, &stderr); err != nil {
		return nil, err
	}
	var probe struct {
		Streams []FFProbeStream `json:"streams"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &probe); err != nil {
		return nil, err
	}
	return probe.Streams, nil
}

func translationSourceRepresentationForCodec(codec string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(codec))
	switch normalized {
	case "hdmv_pgs_subtitle", "pgssub", "dvd_subtitle":
		return translationRepresentationBitmap, nil
	}
	if representation := subtitleSidecarFormat(normalized); representation != "" {
		return representation, nil
	}
	return "", fmt.Errorf("unsupported embedded subtitle codec %q", codec)
}

func translationRoleStreamSummary(stream FFProbeStream, representation string) map[string]any {
	return map[string]any{
		"index":           stream.Index,
		"order":           stream.Index,
		"codec":           strings.ToLower(strings.TrimSpace(stream.CodecName)),
		"language":        translationStreamLanguage(stream),
		"title":           strings.TrimSpace(stream.Tags["title"]),
		"default":         dispositionEnabled(stream.Disposition, "default"),
		"forced":          dispositionEnabled(stream.Disposition, "forced"),
		"comment":         dispositionEnabled(stream.Disposition, "comment"),
		"hearingImpaired": dispositionEnabled(stream.Disposition, "hearing_impaired"),
		"representation":  representation,
	}
}

func translationStreamLanguage(stream FFProbeStream) string {
	language := strings.ToLower(strings.TrimSpace(stream.Tags["language"]))
	if language == "" {
		return "und"
	}
	return language
}

func translationStreamID(stream FFProbeStream) string {
	if stream.ID != nil {
		if value := strings.TrimSpace(fmt.Sprint(stream.ID)); value != "" && value != "<nil>" {
			return value
		}
	}
	return fmt.Sprintf("index:%d", stream.Index)
}

func dispositionEnabled(disposition map[string]int, key string) bool {
	return disposition != nil && disposition[key] != 0
}

func translationIntPointer(value int) *int { return &value }

func readBoundedTranslationSubtitle(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > maxPublishedSubtitleBytes {
		return nil, errors.New("external subtitle is unavailable for bounded validation")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, maxPublishedSubtitleBytes+1))
	if err != nil || int64(len(content)) > maxPublishedSubtitleBytes {
		return nil, errors.New("external subtitle is unavailable for bounded validation")
	}
	format := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if !validSubtitleSidecar(format, content) {
		return nil, errors.New("external subtitle format is invalid")
	}
	return content, nil
}
