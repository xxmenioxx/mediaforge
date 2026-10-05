package handlers

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func thumbnailFixture(t *testing.T, name, status string) (*gorm.DB, models.AssetRecord, string) {
	t.Helper()
	root := t.TempDir()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "thumbnail.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.AssetRecord{}, &models.Library{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	if status == "unprocessed" {
		if err := db.Create(&models.AppSetting{Key: "paths", Value: models.JSONMap{"rawRoot": root}}).Error; err != nil {
			t.Fatal(err)
		}
	} else {
		if err := db.Create(&models.Library{Name: "Library", DestinationPath: root}).Error; err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte("disposable media fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	record := models.AssetRecord{Path: path, RootPath: root, FileName: "not-the-physical-name.mkv", Status: status, LibraryID: 1}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	// Any unexpected generation/probing must not find an external media tool.
	t.Setenv("PATH", t.TempDir())
	return db, record, filepath.Join(root, strings.TrimSuffix(name, filepath.Ext(name))+"-thumb.jpg")
}

func writeThumbnailJPEG(t *testing.T, path string) []byte {
	t.Helper()
	var data bytes.Buffer
	if err := jpeg.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func getThumbnail(db *gorm.DB, identity string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/assets/:assetId/thumbnail", NewAssetHandler(db).Thumbnail)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/assets/"+identity+"/thumbnail", nil))
	return response
}

func assertThumbnailUnavailable(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusNotFound || response.Body.String() != `{"error":"thumbnail unavailable"}` {
		t.Fatalf("unbounded/unexpected response: status=%d body=%q", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("unavailable thumbnails must not be cached")
	}
}

func TestAssetThumbnailCanonicalSibling(t *testing.T) {
	for _, status := range []string{"unprocessed", "library", "converted", "unverified", "accepted", "published_as_is"} {
		for _, name := range []string{"Movie.mkv", "Movie.2024.mkv", "Episode 01.mkv", "映画 Café 第１話.mkv"} {
			t.Run(status+"/"+name, func(t *testing.T) {
				db, record, thumbnail := thumbnailFixture(t, name, status)
				data := writeThumbnailJPEG(t, thumbnail)
				response := getThumbnail(db, fmt.Sprint(record.ID))
				if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), data) {
					t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
				}
				if response.Header().Get("Content-Type") != "image/jpeg" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
					t.Fatal("missing JPEG serving headers")
				}
				if response.Header().Get("Cache-Control") != "private, max-age=0, must-revalidate" {
					t.Fatal("thumbnail must be revalidated")
				}
			})
		}
	}
}

func TestAssetThumbnailConfiguredRootSymlink(t *testing.T) {
	db, record, thumbnail := thumbnailFixture(t, "Movie.2024.mkv", "unprocessed")
	data := writeThumbnailJPEG(t, thumbnail)
	configuredRoot := filepath.Join(t.TempDir(), "raw")
	if err := os.Symlink(record.RootPath, configuredRoot); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.AppSetting{}).Where("key = ?", "paths").Update("value", models.JSONMap{"rawRoot": configuredRoot}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&record).Update("path", filepath.Join(configuredRoot, "Movie.2024.mkv")).Error; err != nil {
		t.Fatal(err)
	}
	response := getThumbnail(db, fmt.Sprint(record.ID))
	if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), data) {
		t.Fatalf("configured root did not resolve its physical sibling: status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestAssetThumbnailUnavailable(t *testing.T) {
	for _, scenario := range []string{"missing", "directory", "fifo", "unreadable", "not-jpeg", "symlink", "missing-asset", "stale-record", "unknown-id", "archive", "outside-root", "asset-symlink", "parent-symlink", "changed-root", "unrelated-sibling", "parent-thumbnail"} {
		t.Run(scenario, func(t *testing.T) {
			db, record, thumbnail := thumbnailFixture(t, "Movie.mkv", "unprocessed")
			switch scenario {
			case "directory":
				if err := os.Mkdir(thumbnail, 0700); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := syscall.Mkfifo(thumbnail, 0600); err != nil {
					t.Fatal(err)
				}
			case "unreadable":
				writeThumbnailJPEG(t, thumbnail)
				if err := os.Chmod(thumbnail, 0000); err != nil {
					t.Fatal(err)
				}
				if file, err := os.Open(thumbnail); err == nil {
					file.Close()
					t.Skip("process can read mode-000 files")
				}
			case "not-jpeg":
				if err := os.WriteFile(thumbnail, []byte("unrelated secret data"), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				secret := filepath.Join(t.TempDir(), "secret.jpg")
				writeThumbnailJPEG(t, secret)
				if err := os.Symlink(secret, thumbnail); err != nil {
					t.Fatal(err)
				}
			case "missing-asset":
				writeThumbnailJPEG(t, thumbnail)
				if err := os.Remove(record.Path); err != nil {
					t.Fatal(err)
				}
			case "stale-record", "archive":
				writeThumbnailJPEG(t, thumbnail)
				column, value := "missing", any(true)
				if scenario == "archive" {
					column, value = "status", "archive"
				}
				if err := db.Model(&record).Update(column, value).Error; err != nil {
					t.Fatal(err)
				}
			case "unknown-id":
				record.ID += 1000
			case "outside-root", "asset-symlink", "parent-symlink":
				outside := filepath.Join(t.TempDir(), "Movie.mkv")
				if err := os.WriteFile(outside, []byte("private asset"), 0600); err != nil {
					t.Fatal(err)
				}
				writeThumbnailJPEG(t, strings.TrimSuffix(outside, ".mkv")+"-thumb.jpg")
				newPath := outside
				if scenario == "asset-symlink" {
					newPath = filepath.Join(record.RootPath, "linked.mkv")
					if err := os.Symlink(outside, newPath); err != nil {
						t.Fatal(err)
					}
				} else if scenario == "parent-symlink" {
					parent := filepath.Join(record.RootPath, "linked")
					if err := os.Symlink(filepath.Dir(outside), parent); err != nil {
						t.Fatal(err)
					}
					newPath = filepath.Join(parent, "Movie.mkv")
				}
				if err := db.Model(&record).Update("path", newPath).Error; err != nil {
					t.Fatal(err)
				}
			case "changed-root":
				writeThumbnailJPEG(t, thumbnail)
				if err := db.Model(&models.AppSetting{}).Where("key = ?", "paths").Update("value", models.JSONMap{"rawRoot": t.TempDir()}).Error; err != nil {
					t.Fatal(err)
				}
			case "unrelated-sibling":
				writeThumbnailJPEG(t, filepath.Join(record.RootPath, "Other-thumb.jpg"))
			case "parent-thumbnail":
				writeThumbnailJPEG(t, filepath.Join(record.RootPath, "Movie-thumb.jpg"))
				child := filepath.Join(record.RootPath, "child")
				if err := os.Mkdir(child, 0700); err != nil {
					t.Fatal(err)
				}
				newPath := filepath.Join(child, "Movie.mkv")
				if err := os.WriteFile(newPath, []byte("media"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := db.Model(&record).Update("path", newPath).Error; err != nil {
					t.Fatal(err)
				}
			}
			assertThumbnailUnavailable(t, getThumbnail(db, fmt.Sprint(record.ID)))
		})
	}
}

func TestAssetThumbnailRejectsClientPathSelectors(t *testing.T) {
	db, record, thumbnail := thumbnailFixture(t, "Movie.mkv", "unprocessed")
	writeThumbnailJPEG(t, thumbnail)
	other := record
	other.ID = 0
	other.Path = filepath.Join(record.RootPath, "Other.mkv")
	if err := os.WriteFile(other.Path, []byte("other media"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	writeThumbnailJPEG(t, filepath.Join(record.RootPath, "Other-thumb.jpg"))
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/assets/:assetId/thumbnail", NewAssetHandler(db).Thumbnail)
	for _, query := range []string{"path=../secret.jpg", "path=/etc/passwd", "filename=Other-thumb.jpg", "path=" + other.Path, "assetId=" + fmt.Sprint(other.ID)} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/assets/%d/thumbnail?%s", record.ID, query), nil))
		assertThumbnailUnavailable(t, response)
	}
	for _, id := range []string{"0", "-1", "Other-thumb.jpg", "..", "999999999999999999999999", "01"} {
		assertThumbnailUnavailable(t, getThumbnail(db, id))
	}
	if response := getThumbnail(db, fmt.Sprint(other.ID)); response.Code != http.StatusOK {
		t.Fatal("another authorized inventory identity should resolve its own sidecar")
	}
}
