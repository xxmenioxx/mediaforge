package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type AssetMetadataSidecar struct {
	Path       string    `json:"path"`
	FileName   string    `json:"fileName"`
	Format     string    `json:"format"`
	SizeBytes  int64     `json:"sizeBytes"`
	ModifiedAt time.Time `json:"modifiedAt"`
}

func (h AssetHandler) MetadataSidecars(c *gin.Context) {
	mediaPath, _, ok := h.externalSubtitleAsset(c)
	if !ok {
		return
	}

	sidecars, err := metadataSidecarsForMedia(mediaPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, sidecars)
}

func metadataSidecarsForMedia(mediaPath string) ([]AssetMetadataSidecar, error) {
	directory := filepath.Dir(mediaPath)
	stem := strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath))
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}

	expectedName := stem + ".nfo"
	matches := make([]os.DirEntry, 0, 1)
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), expectedName) {
			matches = append(matches, entry)
		}
	}
	if len(matches) != 1 || matches[0].IsDir() {
		return []AssetMetadataSidecar{}, nil
	}

	info, err := matches[0].Info()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return []AssetMetadataSidecar{}, nil
	}

	path := filepath.Join(directory, matches[0].Name())
	return []AssetMetadataSidecar{{
		Path:       path,
		FileName:   matches[0].Name(),
		Format:     "nfo",
		SizeBytes:  info.Size(),
		ModifiedAt: info.ModTime(),
	}}, nil
}
