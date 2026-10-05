package handlers

import (
	"image/jpeg"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
)

// Thumbnail serves only an existing, derived sidecar for a current inventory
// identity. It does not accept paths, alternate filenames or generation options.
func (h AssetHandler) Thumbnail(c *gin.Context) {
	unavailable := func() {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusNotFound, gin.H{"error": "thumbnail unavailable"})
	}
	idText := c.Param("assetId")
	id, err := strconv.ParseUint(idText, 10, 64)
	if err != nil || id == 0 || strconv.FormatUint(id, 10) != idText || c.Request.URL.RawQuery != "" {
		unavailable()
		return
	}
	var record models.AssetRecord
	if err := h.db.First(&record, "id = ?", id).Error; err != nil || record.Missing || record.Status == "archive" {
		unavailable()
		return
	}
	// Both RAW and Library resolve through the persisted inventory path and the
	// existing current configured-root authority. RootPath is not client authority.
	root, err := h.mediaRootForPath(record.Path)
	if err != nil || root == "" || !isMediaExtension(strings.ToLower(filepath.Ext(record.Path))) {
		unavailable()
		return
	}
	if err := validateRegularFileWithinRoot(record.Path, root); err != nil {
		unavailable()
		return
	}
	assetPath, err := filepath.EvalSymlinks(record.Path)
	if err != nil {
		unavailable()
		return
	}
	assetPath, err = filepath.Abs(assetPath)
	if err != nil {
		unavailable()
		return
	}
	directory := filepath.Dir(assetPath)
	name := filepath.Base(assetPath)
	thumbnail := filepath.Clean(filepath.Join(directory, strings.TrimSuffix(name, filepath.Ext(name))+"-thumb.jpg"))
	if filepath.Dir(thumbnail) != directory || validateRegularFileWithinRoot(thumbnail, directory) != nil {
		unavailable()
		return
	}
	// Anchor the open to the configured root and then the selected directory.
	// Root handles also prevent a concurrent symlink swap from escaping it.
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		unavailable()
		return
	}
	rootReal, err = filepath.Abs(rootReal)
	if err != nil {
		unavailable()
		return
	}
	relativeDirectory, err := filepath.Rel(rootReal, directory)
	if err != nil {
		unavailable()
		return
	}
	ownedRoot, err := os.OpenRoot(root)
	if err != nil {
		unavailable()
		return
	}
	defer ownedRoot.Close()
	selectedDirectory, err := ownedRoot.OpenRoot(relativeDirectory)
	if err != nil {
		unavailable()
		return
	}
	defer selectedDirectory.Close()
	assetInfo, err := selectedDirectory.Lstat(name)
	if err != nil || !assetInfo.Mode().IsRegular() {
		unavailable()
		return
	}
	thumbnailName := filepath.Base(thumbnail)
	before, err := selectedDirectory.Lstat(thumbnailName)
	if err != nil || !before.Mode().IsRegular() {
		unavailable()
		return
	}
	file, err := selectedDirectory.Open(thumbnailName)
	if err != nil {
		unavailable()
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(before, info) {
		unavailable()
		return
	}
	// Bound header parsing and reject files merely named .jpg. The browser still
	// handles a later decode/read failure with the normal placeholder fallback.
	if _, err := jpeg.DecodeConfig(io.LimitReader(file, 1<<20)); err != nil {
		unavailable()
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		unavailable()
		return
	}
	c.Header("Content-Type", "image/jpeg")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, max-age=0, must-revalidate")
	http.ServeContent(c.Writer, c.Request, "thumbnail.jpg", info.ModTime(), file)
}
