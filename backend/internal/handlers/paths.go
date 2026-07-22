package handlers

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type PathHandler struct {
	db *gorm.DB
}

type PathEntry struct {
	Path         string `json:"path"`
	RelativePath string `json:"relativePath"`
	Name         string `json:"name"`
}

type PathBrowseResponse struct {
	Root    string      `json:"root"`
	RootKey string      `json:"rootKey"`
	Paths   []PathEntry `json:"paths"`
}

func NewPathHandler(db *gorm.DB) PathHandler {
	return PathHandler{db: db}
}

func (h PathHandler) Browse(c *gin.Context) {
	rootKey := strings.TrimSpace(c.Query("root"))
	if rootKey == "" {
		rootKey = "raw"
	}

	root, ok, err := h.rootForKey(rootKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported root"})
		return
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	info, err := os.Stat(rootAbs)
	if err != nil || !info.IsDir() {
		c.JSON(http.StatusOK, PathBrowseResponse{Root: rootAbs, RootKey: rootKey, Paths: []PathEntry{}})
		return
	}

	paths := []PathEntry{}
	_ = filepath.WalkDir(rootAbs, func(path string, entry os.DirEntry, err error) error {
		if err != nil || !entry.IsDir() {
			return nil
		}

		pathAbs, err := filepath.Abs(path)
		if err != nil || !isInsideRoot(rootAbs, pathAbs) {
			return nil
		}

		relativePath, err := filepath.Rel(rootAbs, pathAbs)
		if err != nil {
			return nil
		}
		if relativePath == "." {
			relativePath = ""
		}

		paths = append(paths, PathEntry{
			Path:         pathAbs,
			RelativePath: relativePath,
			Name:         displayPathName(relativePath, rootAbs),
		})
		return nil
	})

	sort.SliceStable(paths, func(i, j int) bool {
		return paths[i].RelativePath < paths[j].RelativePath
	})

	c.JSON(http.StatusOK, PathBrowseResponse{Root: rootAbs, RootKey: rootKey, Paths: paths})
}

func (h PathHandler) rootForKey(rootKey string) (string, bool, error) {
	paths := models.JSONMap{
		"rawRoot":     "/media/raw",
		"libraryRoot": "/media/library",
		"stagingPath": "/media/staging",
	}

	var setting models.AppSetting
	if err := h.db.First(&setting, "key = ?", "paths").Error; err == nil && setting.Value != nil {
		paths = setting.Value
	} else if err != nil && err != gorm.ErrRecordNotFound {
		return "", false, err
	}

	switch rootKey {
	case "raw":
		return stringSetting(paths, "rawRoot", "/media/raw"), true, nil
	case "library":
		return stringSetting(paths, "libraryRoot", "/media/library"), true, nil
	case "staging":
		return stringSetting(paths, "stagingPath", "/media/staging"), true, nil
	default:
		return "", false, nil
	}
}

func stringSetting(values models.JSONMap, key string, fallback string) string {
	value, ok := values[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func isInsideRoot(rootAbs string, pathAbs string) bool {
	relative, err := filepath.Rel(rootAbs, pathAbs)
	if err != nil {
		return false
	}
	return relative == "." || (!strings.HasPrefix(relative, ".."+string(os.PathSeparator)) && relative != "..")
}

func displayPathName(relativePath string, rootAbs string) string {
	if relativePath == "" {
		return filepath.Base(rootAbs)
	}
	return relativePath
}
