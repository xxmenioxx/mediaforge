package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPreviewTrackProfileResolutionUsesPathRulesWithoutPersistingDraftIndexes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:track-preview?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ScanResult{}); err != nil {
		t.Fatal(err)
	}
	path := "/media/series/Arbegas/Arbegas 21.mkv"
	scan := models.ScanResult{
		Path:         path,
		VideoStreams: models.JSONList{map[string]any{"index": 0, "codec": "mpeg2video"}},
		AudioStreams: models.JSONList{
			map[string]any{"index": 1, "language": "jpn", "default": true},
			map[string]any{"index": 2, "language": "eng", "title": "Director Commentary"},
			map[string]any{"index": 3, "language": "spa"},
		},
		SubtitleStreams: models.JSONList{
			map[string]any{"index": 4, "language": "spa"},
			map[string]any{"index": 5, "language": "fre"},
		},
	}
	if err := db.Create(&scan).Error; err != nil {
		t.Fatal(err)
	}
	body := `{"assetPath":"/media/series/Arbegas/Arbegas 21.mkv","profile":{"scope":"path","videoMode":"first","audioMode":"languages","audioLanguages":["jpn","spa","eng"],"dropCommentary":true,"subtitleMode":"languages","subtitleLanguages":["spa"],"audioRequired":true,"keepAudioStreams":[99]}}`
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/track-profiles/resolve-preview", NewQueueHandler(db).PreviewTrackProfileResolution)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/track-profiles/resolve-preview", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var preview TrackProfileResolutionPreview
	if err := json.Unmarshal(response.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	assertIndexesEqual(t, preview.KeepVideoStreams, []int{0})
	assertIndexesEqual(t, preview.KeepAudioStreams, []int{1, 3})
	assertIndexesEqual(t, preview.KeepSubtitleStreams, []int{4})
	if len(preview.Audio) != 3 || preview.Audio[1].Kept || preview.Audio[1].Reason != "commentary" {
		t.Fatalf("commentary decision was not explained: %#v", preview.Audio)
	}
	if len(preview.Subtitle) != 2 || preview.Subtitle[1].Kept || preview.Subtitle[1].Reason != "language_not_selected" {
		t.Fatalf("subtitle decision was not explained: %#v", preview.Subtitle)
	}
}

func assertIndexesEqual(t *testing.T, got, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("indexes=%v want=%v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("indexes=%v want=%v", got, want)
		}
	}
}
