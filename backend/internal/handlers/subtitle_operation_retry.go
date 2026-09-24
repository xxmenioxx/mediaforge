package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

var errSubtitleRestyleRetryStaleSource = errors.New("stale_source: the subtitle source changed; start a fresh operation")

// RetrySubtitleOperation is the single retry dispatcher for subtitle
// operations. Translation keeps its existing request/provider validation; the
// Restyle branch uses only the bounded frozen request retained by the shared
// operation registry.
func (h AssetHandler) RetrySubtitleOperation(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "operation id is required"})
		return
	}

	subtitleExtractionOperations.RLock()
	operation, exists := subtitleExtractionOperations.items[id]
	kind := ""
	if exists {
		kind = operation.Kind
	}
	subtitleExtractionOperations.RUnlock()
	if !exists {
		c.JSON(http.StatusConflict, gin.H{"error": "subtitle operation not found"})
		return
	}

	switch kind {
	case subtitleTranslationOperationKind:
		h.retrySubtitleTranslation(c)
	case "restyle":
		h.retrySubtitleRestyle(c, id)
	case "compose":
		h.retrySubtitleComposition(c, id)
	default:
		c.JSON(http.StatusConflict, gin.H{"error": "subtitle operation is not retryable"})
	}
}

// RetrySubtitleTranslation remains as a compatibility entry point for
// repository callers; the route itself uses RetrySubtitleOperation.
func (h AssetHandler) RetrySubtitleTranslation(c *gin.Context) {
	h.RetrySubtitleOperation(c)
}

func subtitleRestyleRetryRequest(operationID string) (subtitleRestyleRequest, error) {
	subtitleExtractionOperations.RLock()
	operation, exists := subtitleExtractionOperations.items[operationID]
	if !exists {
		subtitleExtractionOperations.RUnlock()
		return subtitleRestyleRequest{}, errors.New("subtitle restyle operation not found")
	}
	if operation.Kind != "restyle" {
		subtitleExtractionOperations.RUnlock()
		return subtitleRestyleRequest{}, errors.New("operation is not a subtitle restyle")
	}
	if operation.Status == "running" {
		subtitleExtractionOperations.RUnlock()
		return subtitleRestyleRequest{}, errors.New("running subtitle restyle cannot be retried")
	}
	if operation.Status != "error" && operation.Status != "cancelled" {
		subtitleExtractionOperations.RUnlock()
		return subtitleRestyleRequest{}, errors.New("only failed or cancelled subtitle restyle can be retried")
	}
	if operation.restyleRequest == nil {
		subtitleExtractionOperations.RUnlock()
		return subtitleRestyleRequest{}, errors.New("subtitle restyle retry request is unavailable")
	}
	request := *operation.restyleRequest
	request.Policy = cloneSubtitleStylePolicy(request.Policy)
	subtitleExtractionOperations.RUnlock()

	fingerprint, err := mediaFileFingerprint(request.SourcePath)
	if err != nil || fingerprint != request.SourceRevision {
		return subtitleRestyleRequest{}, errSubtitleRestyleRetryStaleSource
	}
	request.TargetPath = subtitleRestyleSiblingPath(request.SourcePath, request.Representation, request.Mode, request.Replace)
	return request, nil
}

func (h AssetHandler) retrySubtitleRestyle(c *gin.Context, operationID string) {
	request, err := subtitleRestyleRetryRequest(operationID)
	if err != nil {
		if errors.Is(err, errSubtitleRestyleRetryStaleSource) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "code": "stale_source", "retryable": false})
			return
		}
		c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "retryable": false})
		return
	}
	newOperationID, err := startSubtitleRestyleOperation(request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	subtitleExtractionOperations.RLock()
	operation := cloneSubtitleExtractionOperationSnapshot(subtitleExtractionOperations.items[newOperationID])
	subtitleExtractionOperations.RUnlock()
	c.JSON(http.StatusAccepted, gin.H{"operation": operation})
}
