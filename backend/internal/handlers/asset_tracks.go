package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AssetTracksUpdateInput is deliberately limited to fields edited by the
// Asset Info Tracks section. RawMessage preserves omitted, null, and empty
// array values as distinct patch operations.
type AssetTracksUpdateInput struct {
	KeepVideoStreams               json.RawMessage `json:"keepVideoStreams"`
	KeepAudioStreams               json.RawMessage `json:"keepAudioStreams"`
	KeepSubtitleStreams            json.RawMessage `json:"keepSubtitleStreams"`
	VideoMetadata                  json.RawMessage `json:"videoMetadata"`
	AudioMetadata                  json.RawMessage `json:"audioMetadata"`
	SubtitleMetadata               json.RawMessage `json:"subtitleMetadata"`
	SubtitleRoleOverrides          json.RawMessage `json:"subtitleRoleOverrides"`
	SubtitleSidecarFormatsByStream json.RawMessage `json:"subtitleSidecarFormatsByStream"`
}

type assetTracksUpdateError struct {
	status  int
	message string
}

func (err *assetTracksUpdateError) Error() string {
	return err.message
}

func (input AssetTracksUpdateInput) empty() bool {
	return input.KeepVideoStreams == nil && input.KeepAudioStreams == nil && input.KeepSubtitleStreams == nil &&
		input.VideoMetadata == nil && input.AudioMetadata == nil && input.SubtitleMetadata == nil &&
		input.SubtitleRoleOverrides == nil && input.SubtitleSidecarFormatsByStream == nil
}

func (h AssetHandler) UpdateTracks(c *gin.Context) {
	path := strings.TrimSpace(c.Query("path"))
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
		return
	}
	resolvedPath, err := h.resolveMediaPath(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var input AssetTracksUpdateInput
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cleanPath := filepath.Clean(resolvedPath)
	// Queue creation and maintenance use assetMutationMu. Keep both guards and
	// the override read-modify-write in the same serialized boundary.
	assetMutationMu.Lock()
	defer assetMutationMu.Unlock()
	assetOverrideMutationMu.Lock()
	defer assetOverrideMutationMu.Unlock()
	allowed, err := h.pathBelongsToLibrary(cleanPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !allowed {
		c.JSON(http.StatusForbidden, gin.H{"error": "media path is outside configured libraries"})
		return
	}
	if _, err := os.Stat(cleanPath); err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "asset path changed; reload Asset Info before saving"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var persisted AssetConversionOverrideState
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if active, err := (QueueHandler{db: tx}).assetHasOpenJob(cleanPath, 0); err != nil {
			return &assetTracksUpdateError{status: http.StatusInternalServerError, message: "check active Queue work: " + err.Error()}
		} else if active {
			return &assetTracksUpdateError{status: http.StatusConflict, message: "asset has an active Queue job and Tracks cannot be modified"}
		}
		if active, err := activeAssetMaintenance(tx, cleanPath); err != nil {
			return &assetTracksUpdateError{status: http.StatusInternalServerError, message: "check active asset maintenance: " + err.Error()}
		} else if active {
			return &assetTracksUpdateError{status: http.StatusConflict, message: "asset has an active maintenance operation and Tracks cannot be modified"}
		}

		entries := assetConversionOverrides(tx)
		existing := entries[cleanPath]
		updated, err := applyAssetTracksPatch(tx, cleanPath, existing, input)
		if err != nil {
			return &assetTracksUpdateError{status: http.StatusBadRequest, message: err.Error()}
		}
		if !input.empty() {
			if assetConversionOverrideEmpty(updated) {
				delete(entries, cleanPath)
			} else {
				now := time.Now()
				updated.UpdatedAt = &now
				entries[cleanPath] = updated
			}
			if err := saveAssetConversionOverrides(tx, entries); err != nil {
				return err
			}
		}
		persisted = conversionOverrideForPath(cleanPath, entries)
		return nil
	})
	if err != nil {
		var updateErr *assetTracksUpdateError
		if errors.As(err, &updateErr) {
			if updateErr.status >= http.StatusInternalServerError {
				appendSystemLog(h.db, "asset_tracks_override_save_failed", map[string]string{"path": path, "resolvedPath": resolvedPath}, err)
			}
			c.JSON(updateErr.status, gin.H{"error": updateErr.Error()})
			return
		}
		appendSystemLog(h.db, "asset_tracks_override_save_failed", map[string]string{"path": path, "resolvedPath": resolvedPath}, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	appendSystemLog(h.db, "asset_tracks_override_saved", map[string]string{"path": path, "resolvedPath": resolvedPath}, nil)
	c.JSON(http.StatusOK, gin.H{"path": cleanPath, "conversion": persisted})
}

func applyAssetTracksPatch(db *gorm.DB, path string, existing AssetConversionOverrideState, input AssetTracksUpdateInput) (AssetConversionOverrideState, error) {
	updated := existing
	if value, supplied, err := decodeAssetTracksPatchValue[[]int](input.KeepVideoStreams); err != nil {
		return existing, fmt.Errorf("keepVideoStreams: %w", err)
	} else if supplied {
		updated.KeepVideoStreams = normalizedStreamIndexes(value)
	}
	if value, supplied, err := decodeAssetTracksPatchValue[[]int](input.KeepAudioStreams); err != nil {
		return existing, fmt.Errorf("keepAudioStreams: %w", err)
	} else if supplied {
		updated.KeepAudioStreams = normalizedStreamIndexes(value)
	}
	if value, supplied, err := decodeAssetTracksPatchValue[[]int](input.KeepSubtitleStreams); err != nil {
		return existing, fmt.Errorf("keepSubtitleStreams: %w", err)
	} else if supplied {
		updated.KeepSubtitleStreams = normalizedStreamIndexes(value)
	}
	if value, supplied, err := decodeAssetTracksPatchValue[map[int]StreamMetadataOverride](input.VideoMetadata); err != nil {
		return existing, fmt.Errorf("videoMetadata: %w", err)
	} else if supplied {
		updated.VideoMetadata = normalizedStreamMetadata(value)
	}
	if value, supplied, err := decodeAssetTracksPatchValue[map[int]StreamMetadataOverride](input.AudioMetadata); err != nil {
		return existing, fmt.Errorf("audioMetadata: %w", err)
	} else if supplied {
		updated.AudioMetadata = normalizedStreamMetadata(value)
	}
	if value, supplied, err := decodeAssetTracksPatchValue[map[int]StreamMetadataOverride](input.SubtitleMetadata); err != nil {
		return existing, fmt.Errorf("subtitleMetadata: %w", err)
	} else if supplied {
		updated.SubtitleMetadata = normalizedStreamMetadata(value)
	}
	if value, supplied, err := decodeAssetTracksPatchValue[map[int]SubtitleRoleOverride](input.SubtitleRoleOverrides); err != nil {
		return existing, fmt.Errorf("subtitleRoleOverrides: %w", err)
	} else if supplied {
		var scan models.ScanResult
		if err := db.Where("path = ?", path).Order("updated_at desc, id desc").First(&scan).Error; err != nil {
			scan = models.ScanResult{}
		}
		updated.SubtitleRoleOverrides = subtitleRoleOverridesForConversionUpdate(scan, existing.SubtitleRoleOverrides, value, true)
	}
	if value, supplied, err := decodeAssetTracksPatchValue[map[int][]string](input.SubtitleSidecarFormatsByStream); err != nil {
		return existing, fmt.Errorf("subtitleSidecarFormatsByStream: %w", err)
	} else if supplied {
		updated.SubtitleSidecarFormatsByStream = normalizedSubtitleSidecarFormatsByStream(value)
	}
	return updated, nil
}

func decodeAssetTracksPatchValue[T any](raw json.RawMessage) (T, bool, error) {
	var value T
	if raw == nil {
		return value, false, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return value, true, nil
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return value, true, err
	}
	return value, true, nil
}
