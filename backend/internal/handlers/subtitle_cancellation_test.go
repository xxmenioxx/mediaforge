package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSubtitleConversionCancellationIsAtomicAndRetryGetsNewIdentity(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:subtitle-cancellation?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AssetRecord{}, &models.Library{}, &models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Movie.mkv")
	writeTestFile(t, mediaPath, "source")
	if err := db.Create(&models.Library{Name: "Movies", DestinationPath: root}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AssetRecord{Path: mediaPath, RootPath: root, RelativePath: filepath.Base(mediaPath), FileName: filepath.Base(mediaPath), Status: "library"}).Error; err != nil {
		t.Fatal(err)
	}

	bin := t.TempDir()
	marker := filepath.Join(root, "ass-started")
	t.Setenv("SUBTITLE_ASS_MARKER", marker)
	t.Setenv("SUBTITLE_ASS_RETRY", "")
	writeExecutable(t, filepath.Join(bin, "ffprobe"), `#!/bin/sh
printf '%s' '{"streams":[{"index":2,"codec_type":"subtitle","codec_name":"subrip","tags":{"language":"eng"}}]}'
`)
	writeExecutable(t, filepath.Join(bin, "ffmpeg"), `#!/bin/sh
for argument do output="$argument"; done
if [ "$output" = "-" ]; then exit 0; fi
case "$output" in
  *.ass)
    if [ "${SUBTITLE_ASS_RETRY:-}" = "1" ]; then
      printf '%s\n' '[Script Info]' '[Events]' 'Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text' 'Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,Retry subtitle' > "$output"
      exit 0
    fi
    printf '%s\n' 'partial ASS' > "$output"
    touch "$SUBTITLE_ASS_MARKER"
    while :; do sleep 1; done
    ;;
  *)
    printf '%s\n' '1' '00:00:01,000 --> 00:00:02,000' 'Shared subtitle' > "$output"
    ;;
esac
`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	gin.SetMode(gin.TestMode)
	handler := NewAssetHandler(db)
	router := gin.New()
	router.POST("/api/assets/extract-subtitles", handler.ExtractSubtitles)
	router.GET("/api/assets/extract-subtitles/:id", handler.GetSubtitleExtractionOperation)
	router.POST("/api/assets/extract-subtitles/:id/cancel", handler.CancelSubtitleExtractionOperation)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/assets/extract-subtitles?path="+mediaPath, strings.NewReader(`{"streamIndex":2,"format":"both"}`))
	router.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("start status=%d body=%s", response.Code, response.Body.String())
	}
	var started subtitleExtractionStartedResponse
	if err := json.Unmarshal(response.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started.OperationID == "" {
		t.Fatal("missing operation identity")
	}
	waitForFile(t, marker)

	response = httptest.NewRecorder()
	cancelRequest := httptest.NewRequest(http.MethodPost, "/api/assets/extract-subtitles/"+started.OperationID+"/cancel", nil)
	router.ServeHTTP(response, cancelRequest)
	if response.Code != http.StatusAccepted && response.Code != http.StatusOK {
		t.Fatalf("cancel status=%d body=%s", response.Code, response.Body.String())
	}
	cancelled := waitForSubtitleOperationStatus(t, started.OperationID, "cancelled")
	if cancelled.Error != "" {
		t.Fatalf("cancellation was misclassified as failure: %#v", cancelled)
	}
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/assets/extract-subtitles/"+started.OperationID+"/cancel", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"cancelled"`) {
		t.Fatalf("repeated cancellation changed terminal state: status=%d body=%s", response.Code, response.Body.String())
	}
	for _, path := range []string{
		filepath.Join(root, "Movie.eng.2.srt"),
		filepath.Join(root, "Movie.eng.2.ass"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("cancelled both operation published %q: %v", path, err)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".mvforge-subtitle-operation-") {
			t.Fatalf("operation workspace survived cancellation: %q", entry.Name())
		}
	}

	t.Setenv("SUBTITLE_ASS_RETRY", "1")
	response = httptest.NewRecorder()
	retryRequest := httptest.NewRequest(http.MethodPost, "/api/assets/extract-subtitles?path="+mediaPath, strings.NewReader(`{"streamIndex":2,"format":"both"}`))
	router.ServeHTTP(response, retryRequest)
	if response.Code != http.StatusAccepted {
		t.Fatalf("retry status=%d body=%s", response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	if started.OperationID == "" || started.OperationID == cancelled.ID {
		t.Fatalf("retry did not receive a new operation identity: new=%q old=%q", started.OperationID, cancelled.ID)
	}
	retried := waitForSubtitleOperationStatus(t, started.OperationID, "completed")
	if retried.Result == nil || len(retried.Result.Created) != 2 {
		t.Fatalf("retry should publish both outputs together: %#v", retried.Result)
	}
	old := snapshotSubtitleOperation(cancelled.ID)
	if old.Status != "cancelled" {
		t.Fatalf("cancelled operation changed after retry: %#v", old)
	}
}

func TestSubtitlePublicationBarrierWinsRacingCancellation(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	operationID := "subtitle-publication-race"
	operation := &subtitleExtractionOperation{
		ID:                     operationID,
		Status:                 "running",
		Phase:                  "publishing",
		cancel:                 cancel,
		publicationCommittedCh: make(chan struct{}),
	}
	subtitleExtractionOperations.Lock()
	subtitleExtractionOperations.items[operationID] = operation
	subtitleExtractionOperations.Unlock()
	defer func() {
		subtitleExtractionOperations.Lock()
		delete(subtitleExtractionOperations.items, operationID)
		subtitleExtractionOperations.Unlock()
	}()

	targets := []subtitleExtractionTarget{}
	created := []SubtitleArtifact{}
	createdPaths := []string{}
	for _, format := range []string{"srt", "ass"} {
		stagePath := filepath.Join(root, "stage."+format)
		finalPath := filepath.Join(root, "Movie.eng.2."+format)
		content := "1\n00:00:01,000 --> 00:00:02,000\nShared subtitle\n"
		if format == "ass" {
			content = "[Script Info]\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,Shared subtitle\n"
		}
		writeTestFile(t, stagePath, content)
		targets = append(targets, subtitleExtractionTarget{stagePath: stagePath, finalPath: finalPath})
		created = append(created, SubtitleArtifact{StagedPath: finalPath, Format: format})
		createdPaths = append(createdPaths, finalPath)
	}

	publishDone := make(chan error, 1)
	go func() {
		_, _, err := publishSubtitleExtractionTargets(operationID, ctx, targets, created, createdPaths)
		publishDone <- err
	}()
	select {
	case <-operation.publicationCommittedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("publication barrier did not commit")
	}

	cancelDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		gin.SetMode(gin.TestMode)
		router := gin.New()
		router.POST("/api/assets/extract-subtitles/:id/cancel", (&AssetHandler{}).CancelSubtitleExtractionOperation)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/assets/extract-subtitles/"+operationID+"/cancel", nil))
		cancelDone <- response
	}()

	if err := <-publishDone; err != nil {
		t.Fatalf("publication failed: %v", err)
	}
	response := <-cancelDone
	if response.Code != http.StatusAccepted && response.Code != http.StatusOK {
		t.Fatalf("cancel status=%d body=%s", response.Code, response.Body.String())
	}
	if err := ctx.Err(); err != nil {
		t.Fatalf("racing cancellation cancelled committed operation: %v", err)
	}
	if !snapshotSubtitleOperation(operationID).publicationCommitted {
		t.Fatal("operation lost publication commit")
	}
	for _, format := range []string{"srt", "ass"} {
		if _, err := os.Stat(filepath.Join(root, "Movie.eng.2."+format)); err != nil {
			t.Fatalf("committed %s publication missing after racing cancellation: %v", format, err)
		}
	}
}

func TestRunSubtitleCommandTerminatesProcessTreeOnCancellation(t *testing.T) {
	bin := t.TempDir()
	marker := filepath.Join(bin, "child.pid")
	script := filepath.Join(bin, "tree.sh")
	writeExecutable(t, script, `#!/bin/sh
sleep 60 &
child=$!
printf '%s' "$child" > "$TREE_MARKER"
wait "$child"
`)
	t.Setenv("TREE_MARKER", marker)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runSubtitleCommand(ctx, script, nil, nil, nil) }()
	waitForFile(t, marker)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("process tree did not terminate after cancellation")
	}
	contents, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(contents)))
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(pid, 0); err == nil {
		t.Fatalf("child process %d survived cancellation", pid)
	}
}

func TestBitmapSubtitleConversionCancellation(t *testing.T) {
	for _, testCase := range []struct {
		name            string
		codec           string
		format          string
		expectedFormats []string
	}{
		{name: "PGS-SRT", codec: "hdmv_pgs_subtitle", format: "srt", expectedFormats: []string{"srt"}},
		{name: "PGS-ASS", codec: "hdmv_pgs_subtitle", format: "ass", expectedFormats: []string{"ass"}},
		{name: "PGS-both", codec: "hdmv_pgs_subtitle", format: "both", expectedFormats: []string{"srt", "ass"}},
		{name: "VobSub-SRT", codec: "dvd_subtitle", format: "srt", expectedFormats: []string{"srt"}},
		{name: "VobSub-ASS", codec: "dvd_subtitle", format: "ass", expectedFormats: []string{"ass"}},
		{name: "VobSub-both", codec: "dvd_subtitle", format: "both", expectedFormats: []string{"srt", "ass"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:subtitle-bitmap-cancellation-%s?mode=memory&cache=shared", strings.ToLower(testCase.name))), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(&models.AssetRecord{}, &models.Library{}, &models.AppSetting{}); err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			mediaPath := filepath.Join(root, "Bitmap.mkv")
			writeTestFile(t, mediaPath, "source")
			t.Setenv("MVFORGE_OCR_PERSISTENT_ROOT", filepath.Join(root, "persistent"))
			if err := db.Create(&models.Library{Name: "Movies", DestinationPath: root}).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&models.AssetRecord{Path: mediaPath, RootPath: root, RelativePath: filepath.Base(mediaPath), FileName: filepath.Base(mediaPath), Status: "library"}).Error; err != nil {
				t.Fatal(err)
			}
			bin := t.TempDir()
			marker := filepath.Join(root, "ocr-started")
			t.Setenv("BITMAP_OCR_MARKER", marker)
			writeExecutable(t, filepath.Join(bin, "ffprobe"), fmt.Sprintf(`#!/bin/sh
printf '%%s' '{"streams":[{"index":4,"id":"0x7","codec_type":"subtitle","codec_name":"%s","tags":{"language":"eng"}}]}'
`, testCase.codec))
			writeExecutable(t, filepath.Join(bin, "ffmpeg"), `#!/bin/sh
for argument do output="$argument"; done
case "$output" in
  *.sup) printf '%s' 'PG' > "$output" ;;
  *.ass) printf '%s\n' '[Script Info]' '[Events]' 'Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text' 'Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,Retry OCR subtitle' > "$output" ;;
esac
`)
			writeExecutable(t, filepath.Join(bin, "seconv"), `#!/bin/sh
if [ "${BITMAP_OCR_RETRY:-0}" = "1" ]; then
  for argument do
    case "$argument" in
      --output-filename:*) output="${argument#--output-filename:}" ;;
    esac
  done
  case "$output" in
    *.srt) printf '%s\n' '1' '00:00:01,000 --> 00:00:02,000' 'Retry OCR subtitle' > "$output" ;;
    *.ass) printf '%s\n' '[Script Info]' '[Events]' 'Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text' 'Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,Retry OCR subtitle' > "$output" ;;
  esac
  exit 0
fi
printf '%s\n' 'Running tesseract OCR on 5 VobSub image(s)...' >&2
touch "$BITMAP_OCR_MARKER"
while :; do sleep 1; done
`)
			writeExecutable(t, filepath.Join(bin, "tesseract"), "#!/bin/sh\nif [ \"$1\" = \"--list-langs\" ]; then printf '%s\\n' eng spa jpn jpn_vert; exit 0; fi\nexit 0\n")
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

			gin.SetMode(gin.TestMode)
			handler := NewAssetHandler(db)
			router := gin.New()
			router.POST("/api/assets/extract-subtitles", handler.ExtractSubtitles)
			router.POST("/api/assets/extract-subtitles/:id/cancel", handler.CancelSubtitleExtractionOperation)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/assets/extract-subtitles?path="+mediaPath, strings.NewReader(fmt.Sprintf(`{"streamIndex":4,"format":"%s"}`, testCase.format))))
			if response.Code != http.StatusAccepted {
				t.Fatalf("start status=%d body=%s", response.Code, response.Body.String())
			}
			var started subtitleExtractionStartedResponse
			if err := json.Unmarshal(response.Body.Bytes(), &started); err != nil {
				t.Fatal(err)
			}
			waitForFile(t, marker)
			progress := waitForSubtitleOperationProgress(t, started.OperationID, 5)
			if progress.Phase != "ocr" || progress.Processed != 0 || progress.Total != 5 || progress.Progress < 20 {
				t.Fatalf("bitmap OCR progress was not preserved: %#v", progress)
			}
			response = httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/assets/extract-subtitles/"+started.OperationID+"/cancel", nil))
			if response.Code != http.StatusAccepted {
				t.Fatalf("cancel status=%d body=%s", response.Code, response.Body.String())
			}
			cancelled := waitForSubtitleOperationStatus(t, started.OperationID, "cancelled")
			if cancelled.Error != "" {
				t.Fatalf("bitmap cancellation reported failure: %#v", cancelled)
			}
			for _, format := range testCase.expectedFormats {
				outputPath := filepath.Join(root, "Bitmap.eng.4."+format)
				if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
					t.Fatalf("bitmap cancellation published a %s sidecar: %v", format, err)
				}
			}

			t.Setenv("BITMAP_OCR_RETRY", "1")
			response = httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/assets/extract-subtitles?path="+mediaPath, strings.NewReader(fmt.Sprintf(`{"streamIndex":4,"format":"%s"}`, testCase.format))))
			if response.Code != http.StatusAccepted {
				t.Fatalf("retry status=%d body=%s", response.Code, response.Body.String())
			}
			var retried subtitleExtractionStartedResponse
			if err := json.Unmarshal(response.Body.Bytes(), &retried); err != nil {
				t.Fatal(err)
			}
			if retried.OperationID == started.OperationID {
				t.Fatalf("retry reused cancelled operation identity %q", retried.OperationID)
			}
			completed := waitForSubtitleOperationStatus(t, retried.OperationID, "completed")
			if completed.Error != "" {
				t.Fatalf("bitmap retry reported failure: %#v", completed)
			}
			if completed.Result == nil || len(completed.Result.Created) != len(testCase.expectedFormats) {
				t.Fatalf("bitmap retry did not publish the complete %s set: %#v", testCase.format, completed.Result)
			}
			for _, format := range testCase.expectedFormats {
				outputPath := filepath.Join(root, "Bitmap.eng.4."+format)
				if info, err := os.Stat(outputPath); err != nil || info.Size() == 0 {
					t.Fatalf("bitmap retry did not publish %s: %v", format, err)
				}
			}
			if testCase.format == "both" {
				const logicalTransform = "subtitle:4:converted:both"
				var evidenceIdentity string
				for _, format := range testCase.expectedFormats {
					snapshot, ok := loadPublishedCanonicalOCRSnapshot(filepath.Join(root, "Bitmap.eng.4."+format), format, logicalTransform)
					if !ok || snapshot == nil || len(snapshot.Evidence.Cues) == 0 {
						t.Fatalf("successful bitmap fallback did not commit %s canonical baseline: %#v", format, snapshot)
					}
					identity := snapshot.Evidence.Identity.key()
					if evidenceIdentity == "" {
						evidenceIdentity = identity
					} else if identity != evidenceIdentity {
						t.Fatalf("bitmap fallback committed different evidence identities for %s: %q vs %q", format, identity, evidenceIdentity)
					}
				}
			}
		})
	}
}

func snapshotSubtitleOperation(id string) subtitleExtractionOperation {
	subtitleExtractionOperations.RLock()
	defer subtitleExtractionOperations.RUnlock()
	if operation := subtitleExtractionOperations.items[id]; operation != nil {
		return cloneSubtitleExtractionOperationSnapshot(operation)
	}
	return subtitleExtractionOperation{}
}

func waitForSubtitleOperationStatus(t *testing.T, id, status string) subtitleExtractionOperation {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := snapshotSubtitleOperation(id)
		if snapshot.Status == status {
			return snapshot
		}
		if snapshot.Status == "error" {
			t.Fatalf("operation failed while waiting for %q: %#v", status, snapshot)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for operation %q to become %q", id, status)
	return subtitleExtractionOperation{}
}

func waitForSubtitleOperationProgress(t *testing.T, id string, total int) subtitleExtractionOperation {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snapshot := snapshotSubtitleOperation(id)
		if snapshot.Phase == "ocr" && snapshot.Total == total {
			return snapshot
		}
		if snapshot.Status == "error" || snapshot.Status == "cancelled" {
			t.Fatalf("operation stopped before OCR progress was observed: %#v", snapshot)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for OCR progress on operation %q", id)
	return subtitleExtractionOperation{}
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q", path)
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
}
