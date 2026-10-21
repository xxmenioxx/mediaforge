package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestResolveSubtitleTranslationSourceEmbeddedExactStream(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "episode.mkv")
	writeTestFile(t, mediaPath, "media fixture")
	installTranslationSourceFFProbe(t, `{"streams":[
{"index":0,"id":"video-0","codec_type":"video","codec_name":"h264"},
{"index":3,"id":"eng-full","codec_type":"subtitle","codec_name":"ass","tags":{"language":"eng","title":"Full"},"disposition":{"default":1}},
{"index":4,"id":"eng-forced","codec_type":"subtitle","codec_name":"ass","tags":{"language":"eng","title":"Forced"},"disposition":{"forced":1}},
{"index":5,"id":"jpn-pgs","codec_type":"subtitle","codec_name":"hdmv_pgs_subtitle","tags":{"language":"jpn"},"disposition":{}},
{"index":6,"id":"dvb","codec_type":"subtitle","codec_name":"dvb_subtitle","tags":{"language":"eng"},"disposition":{}}
]}`)
	db := translationSourceTestDB(t, root, mediaPath)
	handler := AssetHandler{db: db}

	status, body := postTranslationSource(t, handler, fmt.Sprintf(`{"path":%q,"source":{"type":"embedded","embedded":{"streamIndex":4}}}`, mediaPath))
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	var response subtitleTranslationSourceResponse
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatal(err)
	}
	if response.Type != "embedded" || response.StreamIndex == nil || *response.StreamIndex != 4 || response.StreamID != "eng-forced" {
		t.Fatalf("unexpected exact embedded source: %#v", response)
	}
	if response.Codec != "ass" || response.Language != "eng" || response.Title != "Forced" || !response.SourceForced || !response.EffectiveForced {
		t.Fatalf("embedded stream metadata was not preserved: %#v", response)
	}
	if response.Role != SubtitleRoleForced || response.RoleProvenance != "disposition" || response.SourceRevision == "" || response.SourceIdentity == "" {
		t.Fatalf("embedded role/identity metadata was not resolved: %#v", response)
	}
	if strings.Contains(body, "subtitle body") || strings.Contains(body, "provider") {
		t.Fatalf("source response leaked non-metadata content: %s", body)
	}
}

func TestResolveSubtitleTranslationSourceRejectsWrongOrUnsupportedEmbeddedStreams(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "episode.mkv")
	writeTestFile(t, mediaPath, "media fixture")
	installTranslationSourceFFProbe(t, `{"streams":[
{"index":0,"id":"video-0","codec_type":"video","codec_name":"h264"},
{"index":4,"id":"dvb","codec_type":"subtitle","codec_name":"dvb_subtitle","tags":{"language":"eng"},"disposition":{}}
]}`)
	db := translationSourceTestDB(t, root, mediaPath)
	handler := AssetHandler{db: db}

	cases := []struct {
		name    string
		index   int
		message string
	}{
		{name: "wrong stream type", index: 0, message: "not a subtitle stream"},
		{name: "missing stream", index: 99, message: "does not exist"},
		{name: "unsupported codec", index: 4, message: "unsupported embedded subtitle codec"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			status, body := postTranslationSource(t, handler, fmt.Sprintf(`{"path":%q,"source":{"type":"embedded","embedded":{"streamIndex":%d}}}`, mediaPath, test.index))
			if status < http.StatusBadRequest || status >= http.StatusInternalServerError || !strings.Contains(body, test.message) {
				t.Fatalf("status=%d body=%s", status, body)
			}
		})
	}
}

func TestResolveSubtitleTranslationSourceUsesExactExternalArtifact(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Episode.mkv")
	writeTestFile(t, mediaPath, "media fixture")
	basePath := filepath.Join(root, "Episode.eng.ass")
	forcedPath := filepath.Join(root, "Episode.eng.forced.ass")
	japanesePath := filepath.Join(root, "Episode.jpn.srt")
	writeTestFile(t, basePath, "[Events]\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,BASE\n")
	writeTestFile(t, forcedPath, "[Events]\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,FORCED\n")
	writeTestFile(t, japanesePath, "1\n00:00:01,000 --> 00:00:02,000\n日本語\n")
	db := translationSourceTestDB(t, root, mediaPath)
	handler := AssetHandler{db: db}

	status, body := postTranslationSource(t, handler, fmt.Sprintf(`{"path":%q,"source":{"type":"external","external":{"subtitlePath":%q}}}`, mediaPath, forcedPath))
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	var response subtitleTranslationSourceResponse
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatal(err)
	}
	if response.ExternalPath != forcedPath || response.ExternalPath == basePath || response.Language != "eng" || response.Representation != "ass" || !response.SourceForced || response.Role != SubtitleRoleForced {
		t.Fatalf("exact external artifact was not selected: %#v", response)
	}
	if response.SourceRevision == "" || !strings.Contains(response.SourceIdentity, forcedPath) || strings.Contains(body, "FORCED") {
		t.Fatalf("external identity/revision or text boundary invalid: %s", body)
	}
	oldRevision := response.SourceRevision
	writeTestFile(t, forcedPath, "[Events]\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,CHANGED\n")
	status, body = postTranslationSource(t, handler, fmt.Sprintf(`{"path":%q,"source":{"type":"external","external":{"subtitlePath":%q}}}`, mediaPath, forcedPath))
	if status != http.StatusOK {
		t.Fatalf("changed source status=%d body=%s", status, body)
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatal(err)
	}
	if response.SourceRevision == oldRevision {
		t.Fatal("external source revision did not change after content changed")
	}
}

func TestResolveSubtitleTranslationSourceRejectsExternalGuessingAndEscapes(t *testing.T) {
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Episode.mkv")
	writeTestFile(t, mediaPath, "media fixture")
	writeTestFile(t, filepath.Join(root, "Episode.eng.ass"), "[Events]\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,BASE\n")
	writeTestFile(t, filepath.Join(root, "Episode.eng.forced.ass"), "[Events]\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,FORCED\n")
	writeTestFile(t, filepath.Join(root, "OtherMovie.eng.ass"), "[Events]\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,OTHER\n")
	db := translationSourceTestDB(t, root, mediaPath)
	handler := AssetHandler{db: db}

	for _, path := range []string{filepath.Join(root, "../OtherMovie.eng.ass"), filepath.Join(root, "Episode.eng.ssa")} {
		status, body := postTranslationSource(t, handler, fmt.Sprintf(`{"path":%q,"source":{"type":"external","external":{"subtitlePath":%q}}}`, mediaPath, path))
		if status < http.StatusBadRequest || status >= http.StatusInternalServerError {
			t.Fatalf("path=%q status=%d body=%s", path, status, body)
		}
		if strings.Contains(body, "BASE") || strings.Contains(body, "FORCED") || strings.Contains(body, "OTHER") {
			t.Fatalf("external rejection leaked subtitle content: %s", body)
		}
	}
}

func TestValidateSubtitleTranslationSourceSelectorRequiresOneExactVariant(t *testing.T) {
	cases := []subtitleTranslationSourceSelector{
		{Type: ""},
		{Type: "embedded"},
		{Type: "external"},
		{Type: "unknown", Embedded: &embeddedSubtitleTranslationSource{}},
		{Type: "embedded", Embedded: &embeddedSubtitleTranslationSource{}, External: &externalSubtitleTranslationSource{SubtitlePath: "x.ass"}},
	}
	for _, selector := range cases {
		if err := validateSubtitleTranslationSourceSelector(selector); err == nil {
			t.Fatalf("selector %#v was accepted", selector)
		}
	}
}

func translationSourceTestDB(t *testing.T, root, mediaPath string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:translation-source-"+strings.ReplaceAll(t.Name(), "/", "-")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Library{}, &models.AssetRecord{}, &models.AppSetting{}, &models.OCRDictionaryEntry{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Library{Name: "Test", SourcePath: root, DestinationPath: filepath.Join(root, "library"), Type: "movies"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AssetRecord{Path: mediaPath, RootPath: root, RelativePath: filepath.Base(mediaPath), FileName: filepath.Base(mediaPath), Status: "unprocessed"}).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func translationLibrarySourceTestDB(t *testing.T, root, mediaPath string) *gorm.DB {
	t.Helper()
	db := translationSourceTestDB(t, root, mediaPath)
	if err := db.Model(&models.Library{}).Where("id = ?", 1).Update("destination_path", root).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.AssetRecord{}).Where("path = ?", mediaPath).Updates(map[string]any{
		"library_id":   1,
		"library_name": "Test",
		"status":       "accepted",
	}).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func postTranslationSource(t *testing.T, handler AssetHandler, body string) (int, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/assets/subtitle-translation/source", handler.ResolveSubtitleTranslationSource)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/assets/subtitle-translation/source", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder.Code, recorder.Body.String()
}

func installTranslationSourceFFProbe(t *testing.T, fixture string) {
	t.Helper()
	bin := t.TempDir()
	fixturePath := filepath.Join(bin, "probe.json")
	writeTestFile(t, fixturePath, fixture)
	writeExecutable(t, filepath.Join(bin, "ffprobe"), "#!/bin/sh\ncat \"$MVFORGE_TRANSLATION_SOURCE_PROBE\"\n")
	t.Setenv("MVFORGE_TRANSLATION_SOURCE_PROBE", fixturePath)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}
