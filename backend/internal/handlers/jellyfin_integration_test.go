package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func jellyfinIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "-") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AppSetting{}, &models.ProtectedCredential{}, &models.Library{}, &models.TestEncode{}, &models.PublisherPublication{}, &models.JellyfinIntegrationOutcome{}, &models.AssetPublishedOutbox{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if raw, err := db.DB(); err == nil {
			_ = raw.Close()
		}
	})
	return db
}

func saveJellyfinSettings(t *testing.T, db *gorm.DB, settings jellyfinSettings) {
	t.Helper()
	secrets := gormProtectedSecretSource{}
	if settings.APIToken != "" {
		if err := secrets.Put(db, jellyfinAPITokenCredentialRef, settings.APIToken); err != nil {
			t.Fatal(err)
		}
	} else if err := secrets.Delete(db, jellyfinAPITokenCredentialRef); err != nil {
		t.Fatal(err)
	}
	stored := models.AppSetting{Key: jellyfinSettingsKey, Value: jellyfinSettingsStoredValue(settings)}
	if err := db.Save(&stored).Error; err != nil {
		t.Fatal(err)
	}
}

func addJellyfinLibrary(t *testing.T, db *gorm.DB, id uint, destination string) {
	t.Helper()
	if err := db.Create(&models.Library{ID: id, Name: fmt.Sprintf("Library %d", id), SourcePath: filepath.Join(destination, "raw"), DestinationPath: destination, Type: "tv"}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestJellyfinSettingsRedactCredentialAndPreserveOmittedSecret(t *testing.T) {
	db := jellyfinIntegrationDB(t)
	addJellyfinLibrary(t, db, 7, t.TempDir())
	settings := defaultJellyfinSettings()
	settings.BaseURL, settings.APIToken, settings.Mappings = "http://127.0.0.1:8096", "secret-token-value", []jellyfinLibraryMapping{{MVForgeLibraryID: 7, JellyfinLibraryID: "Exact-Jellyfin-ID"}}
	saveJellyfinSettings(t, db, settings)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewSettingsHandler(db)
	router.GET("/api/settings", handler.List)
	router.POST("/api/settings/:key", handler.Update)
	read := httptest.NewRecorder()
	router.ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if strings.Contains(read.Body.String(), settings.APIToken) || !strings.Contains(read.Body.String(), `"credentialConfigured":true`) {
		t.Fatalf("generic Settings projection leaked or lost credential state: %s", read.Body.String())
	}
	var stored models.AppSetting
	if err := db.First(&stored, "key = ?", jellyfinSettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if _, persisted := stored.Value["apiToken"]; persisted {
		t.Fatalf("Jellyfin credential remained in AppSetting: %#v", stored.Value)
	}

	body := `{"value":{"enabled":true,"baseUrl":"http://127.0.0.1:8096","credentialConfigured":true,"refreshAfterPublish":true,"refreshTestEncode":true,"debounceSeconds":10,"mappings":[{"mvforgeLibraryId":7,"jellyfinLibraryId":"Exact-Jellyfin-ID"}]}}`
	updated := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/settings/jellyfin", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(updated, request)
	if updated.Code != http.StatusOK || strings.Contains(updated.Body.String(), settings.APIToken) {
		t.Fatalf("settings update status=%d response=%s", updated.Code, updated.Body.String())
	}
	loaded, _ := loadJellyfinSettings(db)
	if loaded.APIToken != settings.APIToken || loaded.Mappings[0].JellyfinLibraryID != "Exact-Jellyfin-ID" {
		t.Fatalf("omitted credential or exact mapping was not preserved: %#v", loaded)
	}

	clear := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/settings/jellyfin", strings.NewReader(`{"value":{"enabled":true,"baseUrl":"http://127.0.0.1:8096","clearCredential":true,"mappings":[]}}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(clear, request)
	loaded, _ = loadJellyfinSettings(db)
	if clear.Code != http.StatusOK || loaded.APIToken != "" {
		t.Fatalf("explicit credential clear status=%d token=%q", clear.Code, loaded.APIToken)
	}
}

func TestJellyfinLegacyCredentialMigratesOutOfAppSetting(t *testing.T) {
	db := jellyfinIntegrationDB(t)
	legacyValue := models.JSONMap{
		"enabled": true, "baseUrl": "http://127.0.0.1:8096", "apiToken": "legacy-secret-token",
		"refreshAfterPublish": true, "refreshTestEncode": true, "debounceSeconds": 10, "mappings": models.JSONList{},
	}
	if err := db.Create(&models.AppSetting{Key: jellyfinSettingsKey, Value: legacyValue}).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/settings", NewSettingsHandler(db).List)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "legacy-secret-token") || !strings.Contains(response.Body.String(), `"credentialConfigured":true`) {
		t.Fatalf("legacy Settings response status=%d body=%s", response.Code, response.Body.String())
	}
	var stored models.AppSetting
	if err := db.First(&stored, "key = ?", jellyfinSettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if _, persisted := stored.Value["apiToken"]; persisted {
		t.Fatalf("legacy API token was not removed from AppSetting: %#v", stored.Value)
	}
	secret, found, err := (gormProtectedSecretSource{}).Get(db, jellyfinAPITokenCredentialRef)
	if err != nil || !found || secret != "legacy-secret-token" {
		t.Fatalf("migrated credential found=%t secret=%q err=%v", found, secret, err)
	}
}

func TestJellyfinCredentialWriteUsesProtectedSource(t *testing.T) {
	db := jellyfinIntegrationDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewSettingsHandler(db)
	router.POST("/api/settings/:key", handler.Update)
	router.GET("/api/settings", handler.List)
	request := httptest.NewRequest(http.MethodPost, "/api/settings/jellyfin", strings.NewReader(`{"value":{"enabled":true,"baseUrl":"http://127.0.0.1:8096","apiToken":"new-secret-token","mappings":[]}}`))
	request.Header.Set("Content-Type", "application/json")
	updated := httptest.NewRecorder()
	router.ServeHTTP(updated, request)
	if updated.Code != http.StatusOK || strings.Contains(updated.Body.String(), "new-secret-token") {
		t.Fatalf("credential update status=%d body=%s", updated.Code, updated.Body.String())
	}
	var stored models.AppSetting
	if err := db.First(&stored, "key = ?", jellyfinSettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	if _, persisted := stored.Value["apiToken"]; persisted {
		t.Fatalf("API token remained in AppSetting: %#v", stored.Value)
	}
	secret, found, err := (gormProtectedSecretSource{}).Get(db, jellyfinAPITokenCredentialRef)
	if err != nil || !found || secret != "new-secret-token" {
		t.Fatalf("protected credential found=%t secret=%q err=%v", found, secret, err)
	}
	read := httptest.NewRecorder()
	router.ServeHTTP(read, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if read.Code != http.StatusOK || strings.Contains(read.Body.String(), "new-secret-token") {
		t.Fatalf("Settings list status=%d body=%s", read.Code, read.Body.String())
	}
}

func TestJellyfinSettingsRejectInvalidDebounceAndUnstableMappings(t *testing.T) {
	db := jellyfinIntegrationDB(t)
	addJellyfinLibrary(t, db, 7, t.TempDir())
	tests := []struct {
		name  string
		value string
	}{
		{name: "debounce limit", value: `{"value":{"debounceSeconds":61}}`},
		{name: "duplicate mapping identities", value: `{"value":{"mappings":[{"mvforgeLibraryId":7,"jellyfinLibraryId":"A"},{"mvforgeLibraryId":7,"jellyfinLibraryId":"B"}]}}`},
		{name: "unknown MVForge Library", value: `{"value":{"mappings":[{"mvforgeLibraryId":999,"jellyfinLibraryId":"A"}]}}`},
		{name: "whitespace jellyfin ID", value: `{"value":{"mappings":[{"mvforgeLibraryId":7,"jellyfinLibraryId":" ID "}]}}`},
	}
	gin.SetMode(gin.TestMode)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.POST("/api/settings/:key", NewSettingsHandler(db).Update)
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/settings/jellyfin", strings.NewReader(test.value))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestAssetPublishedSubscriberRequiresStableIdentityAndRetriesSettingsReadFailure(t *testing.T) {
	db := jellyfinIntegrationDB(t)
	coordinator := JellyfinCoordinator{db: db}
	payload := models.JSONMap{"publicationId": "publication-1", "libraryId": float64(1)}
	if err := coordinator.assetPublished(context.Background(), "", payload); err == nil {
		t.Fatal("AssetPublished without stable EventID was acknowledged")
	}
	if err := db.Migrator().DropTable(&models.AppSetting{}); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.assetPublished(context.Background(), "stable-event-1", payload); err == nil {
		t.Fatal("settings read failure was acknowledged instead of retrying outbox delivery")
	}
	var outcomes int64
	if err := db.Model(&models.JellyfinIntegrationOutcome{}).Count(&outcomes).Error; err != nil || outcomes != 0 {
		t.Fatalf("settings read failure created an outcome: count=%d err=%v", outcomes, err)
	}
}

func TestJellyfinDiscoveryUsesExactIDsAndRefreshUsesExplicitBroadScan(t *testing.T) {
	var scanRequests int
	var itemRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/System/Info/Public" {
			if r.Header.Get("Authorization") != "" || r.Header.Get("X-Emby-Token") != "" {
				t.Errorf("public server info request was authenticated")
			}
		} else if r.Header.Get("Authorization") != `MediaBrowser Token="test-secret"` || r.Header.Get("X-Emby-Token") != "" {
			t.Errorf("authenticated Jellyfin request used unexpected headers")
			if r.URL.Path == "/Library/MediaFolders" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		switch r.URL.Path {
		case "/System/Info/Public":
			_, _ = w.Write([]byte(`{"Version":"10.9.1"}`))
		case "/Library/MediaFolders":
			_, _ = w.Write([]byte(`{"Items":[{"Id":"Library-Exact/42","Name":"Anime"}]}`))
		case "/Library/Refresh":
			scanRequests++
			w.WriteHeader(http.StatusNoContent)
		case "/Items/Library-Exact/42/Refresh":
			itemRequests++
			w.WriteHeader(http.StatusNotFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	settings := defaultJellyfinSettings()
	settings.Enabled, settings.BaseURL, settings.APIToken = true, server.URL, "test-secret"
	settings.Mappings = []jellyfinLibraryMapping{{MVForgeLibraryID: 7, JellyfinLibraryID: "Library-Exact/42"}}
	result := sharedJellyfinClient.testConnection(context.Background(), settings)
	if !result.Reachable || !result.Authenticated || result.ServerVersion != "10.9.1" || len(result.Libraries) != 1 || result.Libraries[0].ID != "Library-Exact/42" {
		t.Fatalf("connection result=%#v", result)
	}
	coordinator := JellyfinCoordinator{}
	failure := coordinator.refreshBatch(context.Background(), settings, []models.JellyfinIntegrationOutcome{{LibraryID: 7, MappedLibraryID: "Library-Exact/42"}})
	if failure != nil || scanRequests != 1 || itemRequests != 0 {
		t.Fatalf("failure=%v server scans=%d item refreshes=%d", failure, scanRequests, itemRequests)
	}
}

func TestJellyfinAuthFailureRedactsSecretFromErrorsLogsAndOutcomes(t *testing.T) {
	var logs bytes.Buffer
	previousLogOutput := log.Writer()
	previousSlog := slog.Default()
	log.SetOutput(&logs)
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer log.SetOutput(previousLogOutput)
	defer slog.SetDefault(previousSlog)

	secret := "credential-never-log-this"
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			logStart := logs.Len()
			authorizationSeen := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/Library/MediaFolders" {
					http.NotFound(w, r)
					return
				}
				authorizationSeen = r.Header.Get("Authorization") == `MediaBrowser Token="`+secret+`"` && r.Header.Get("X-Emby-Token") == ""
				w.WriteHeader(status)
				_, _ = w.Write([]byte("authorization rejected: " + secret))
			}))
			defer server.Close()
			settings := defaultJellyfinSettings()
			settings.Enabled, settings.BaseURL, settings.APIToken = true, server.URL, secret

			_, failure := sharedJellyfinClient.discoverLibraries(context.Background(), settings)
			if !authorizationSeen || failure == nil || failure.class != "invalid_credentials" {
				t.Fatal("Jellyfin authentication failure was not classified correctly")
			}
			if strings.Contains(failure.Error(), secret) || strings.Contains(failure.Error(), "authorization rejected") {
				t.Fatal("Jellyfin error exposed credentials or the remote response body")
			}

			db := jellyfinIntegrationDB(t)
			addJellyfinLibrary(t, db, 15, t.TempDir())
			settings.DebounceSeconds = 0
			settings.Mappings = []jellyfinLibraryMapping{{MVForgeLibraryID: 15, JellyfinLibraryID: "library-15"}}
			saveJellyfinSettings(t, db, settings)
			sourceKey := testEncodeJellyfinSourceKey(jellyfinReasonTestReady, 88)
			if err := recordJellyfinTrigger(db, jellyfinTrigger{Reason: jellyfinReasonTestReady, SourceKey: sourceKey, LibraryID: 15, TestEncodeID: 88}); err != nil {
				t.Fatal(err)
			}
			coordinator := &JellyfinCoordinator{db: db}
			coordinator.processDue()
			if !authorizationSeen {
				t.Fatal("Jellyfin refresh did not use the modern authorization header")
			}
			var outcome models.JellyfinIntegrationOutcome
			if err := db.First(&outcome, "source_key = ?", sourceKey).Error; err != nil {
				t.Fatal(err)
			}
			if outcome.Status != jellyfinStatusPermanentError || outcome.FailureClass != "invalid_credentials" {
				t.Fatal("persisted authentication outcome was not classified correctly")
			}
			persisted, err := json.Marshal(outcome)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(persisted), secret) || strings.Contains(string(persisted), "authorization rejected") {
				t.Fatal("persisted Jellyfin outcome exposed credentials or the remote response body")
			}
			if strings.Contains(logs.String()[logStart:], secret) {
				t.Fatal("Jellyfin authentication failure logs exposed credentials")
			}
		})
	}
}

func TestJellyfinStaleMappingAndInvalidCredentialAreClassifiedWithoutGuessing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Library/MediaFolders" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("authorization rejected: wrong-secret-must-not-escape"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	settings := defaultJellyfinSettings()
	settings.Enabled, settings.BaseURL, settings.APIToken = true, server.URL, "wrong"
	if _, failure := sharedJellyfinClient.discoverLibraries(context.Background(), settings); failure == nil || failure.class != "invalid_credentials" || strings.Contains(failure.Error(), "wrong-secret") {
		t.Fatalf("invalid credential failure=%v", failure)
	}

	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Library/MediaFolders" {
			_, _ = w.Write([]byte(`{"Items":[{"Id":"different-id","Name":"Anime"}]}`))
			return
		}
		http.NotFound(w, r)
	})
	coordinator := JellyfinCoordinator{}
	failure := coordinator.refreshBatch(context.Background(), settings, []models.JellyfinIntegrationOutcome{{MappedLibraryID: "stale-id"}})
	if failure == nil || failure.class != "stale_mapping" || !failure.permanent {
		t.Fatalf("stale mapping failure=%v", failure)
	}
}

func TestJellyfinConnectionAllowsUnavailableVersionAndBoundsTimeout(t *testing.T) {
	versionUnavailable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/System/Info/Public" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"Items":[{"Id":"library","Name":"TV"}]}`))
	}))
	settings := defaultJellyfinSettings()
	settings.Enabled, settings.BaseURL, settings.APIToken = true, versionUnavailable.URL, "secret"
	result := sharedJellyfinClient.testConnection(context.Background(), settings)
	versionUnavailable.Close()
	if !result.Reachable || !result.Authenticated || result.VersionAvailable || result.ServerVersion != "" {
		t.Fatalf("unavailable version changed successful connection result: %#v", result)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	timeoutServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		_, _ = w.Write([]byte(`{"Items":[]}`))
	}))
	defer timeoutServer.Close()
	settings.BaseURL = timeoutServer.URL
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	finished := make(chan *jellyfinFailure, 1)
	go func() {
		_, failure := sharedJellyfinClient.discoverLibraries(ctx, settings)
		finished <- failure
	}()
	<-started
	failure := <-finished
	close(release)
	if failure == nil || failure.class != "timeout" {
		t.Fatalf("bounded request failure=%v", failure)
	}
}

func TestJellyfinTriggerDeduplicatesAssetPublishedAndCoalescesCrossTriggers(t *testing.T) {
	db := jellyfinIntegrationDB(t)
	addJellyfinLibrary(t, db, 9, t.TempDir())
	addJellyfinLibrary(t, db, 10, t.TempDir())
	settings := defaultJellyfinSettings()
	settings.Enabled, settings.BaseURL, settings.APIToken = true, "http://127.0.0.1:8096", "secret"
	settings.Mappings = []jellyfinLibraryMapping{{MVForgeLibraryID: 9, JellyfinLibraryID: "jelly-id"}, {MVForgeLibraryID: 10, JellyfinLibraryID: "other-jelly-id"}}
	saveJellyfinSettings(t, db, settings)
	coordinator := JellyfinCoordinator{db: db}
	payload := models.JSONMap{"publicationId": "publication-9", "libraryId": float64(9), "destinationPath": "/must/not/be/used"}
	if err := coordinator.assetPublished(context.Background(), "event-9", payload); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.assetPublished(context.Background(), "event-9", payload); err != nil {
		t.Fatal(err)
	}
	if err := recordJellyfinTrigger(db, jellyfinTrigger{Reason: jellyfinReasonTestReady, SourceKey: testEncodeJellyfinSourceKey(jellyfinReasonTestReady, 22), LibraryID: 9, TestEncodeID: 22}); err != nil {
		t.Fatal(err)
	}
	if err := recordJellyfinTrigger(db, jellyfinTrigger{Reason: jellyfinReasonTestReady, SourceKey: testEncodeJellyfinSourceKey(jellyfinReasonTestReady, 22), LibraryID: 9, TestEncodeID: 22}); err != nil {
		t.Fatal(err)
	}
	if err := recordJellyfinTrigger(db, jellyfinTrigger{Reason: jellyfinReasonTestRemoved, SourceKey: testEncodeJellyfinSourceKey(jellyfinReasonTestRemoved, 44), LibraryID: 10, TestEncodeID: 44}); err != nil {
		t.Fatal(err)
	}
	var rows []models.JellyfinIntegrationOutcome
	if err := db.Order("id asc").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0].Reason != jellyfinReasonPublished || rows[0].PublicationID != "publication-9" || rows[0].LibraryID != 9 || rows[0].MappedLibraryID != "jelly-id" || rows[1].Reason != jellyfinReasonTestReady || rows[2].MappedLibraryID != "other-jelly-id" {
		t.Fatalf("integration rows=%#v", rows)
	}
	if rows[0].EffectiveTarget != jellyfinTargetServerWide || rows[1].EffectiveTarget != rows[0].EffectiveTarget || rows[2].EffectiveTarget != rows[0].EffectiveTarget {
		t.Fatalf("fallback target identity did not coalesce: %#v", rows)
	}
	if !rows[0].DueAt.Equal(rows[1].DueAt) || !rows[0].DueAt.Equal(rows[2].DueAt) || !rows[0].FirstTriggeredAt.Equal(rows[1].FirstTriggeredAt) || !rows[0].FirstTriggeredAt.Equal(rows[2].FirstTriggeredAt) {
		t.Fatalf("cross-trigger requests did not share a persisted debounce window: %#v", rows)
	}
}

func TestJellyfinDebounceCapsWaitFromFirstPendingTrigger(t *testing.T) {
	db := jellyfinIntegrationDB(t)
	addJellyfinLibrary(t, db, 31, t.TempDir())
	settings := defaultJellyfinSettings()
	settings.Enabled, settings.DebounceSeconds = true, 60
	settings.Mappings = []jellyfinLibraryMapping{{MVForgeLibraryID: 31, JellyfinLibraryID: "jelly-31"}}
	saveJellyfinSettings(t, db, settings)
	if err := recordJellyfinTrigger(db, jellyfinTrigger{Reason: jellyfinReasonPublished, SourceKey: "asset-published:first", LibraryID: 31}); err != nil {
		t.Fatal(err)
	}
	var first models.JellyfinIntegrationOutcome
	if err := db.First(&first, "source_key = ?", "asset-published:first").Error; err != nil {
		t.Fatal(err)
	}
	first.FirstTriggeredAt = time.Now().UTC().Add(-55 * time.Second)
	if err := db.Model(&first).Update("first_triggered_at", first.FirstTriggeredAt).Error; err != nil {
		t.Fatal(err)
	}
	if err := recordJellyfinTrigger(db, jellyfinTrigger{Reason: jellyfinReasonTestReady, SourceKey: testEncodeJellyfinSourceKey(jellyfinReasonTestReady, 32), LibraryID: 31, TestEncodeID: 32}); err != nil {
		t.Fatal(err)
	}
	var rows []models.JellyfinIntegrationOutcome
	if err := db.Order("id asc").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || !rows[0].DueAt.Equal(rows[1].DueAt) || rows[0].DueAt.After(first.FirstTriggeredAt.Add(jellyfinMaximumDebounce)) {
		t.Fatalf("debounce exceeded max delay from first trigger: %#v", rows)
	}
}

func TestTestEncodeReadyTriggerRequiresPersistedValidatedTransitionAndUsesLibraryID(t *testing.T) {
	db := jellyfinIntegrationDB(t)
	addJellyfinLibrary(t, db, 23, t.TempDir())
	settings := defaultJellyfinSettings()
	settings.Enabled, settings.BaseURL, settings.APIToken = true, "http://127.0.0.1:8096", "secret"
	settings.Mappings = []jellyfinLibraryMapping{{MVForgeLibraryID: 23, JellyfinLibraryID: "Jellyfin-Library-Exact"}}
	saveJellyfinSettings(t, db, settings)
	if err := recordJellyfinTestEncodeReady(db, 67, 23, false, true); err != nil {
		t.Fatal(err)
	}
	if err := recordJellyfinTestEncodeReady(db, 67, 23, true, false); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&models.JellyfinIntegrationOutcome{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("incomplete Test Encode produced outcomes=%d err=%v", count, err)
	}
	if err := recordJellyfinTestEncodeReady(db, 67, 23, true, true); err != nil {
		t.Fatal(err)
	}
	if err := recordJellyfinTestEncodeReady(db, 67, 23, true, true); err != nil {
		t.Fatal(err)
	}
	var outcome models.JellyfinIntegrationOutcome
	if err := db.First(&outcome).Error; err != nil {
		t.Fatal(err)
	}
	if outcome.Reason != jellyfinReasonTestReady || outcome.TestEncodeID != 67 || outcome.LibraryID != 23 || outcome.MappedLibraryID != "Jellyfin-Library-Exact" {
		t.Fatalf("Test Encode integration target=%#v", outcome)
	}
	if err := db.Model(&models.AssetPublishedOutbox{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("Test Encode emitted AssetPublished rows=%d err=%v", count, err)
	}
}

func TestJellyfinCoordinatorResumesPendingWorkAndRetriesTransientFailure(t *testing.T) {
	db := jellyfinIntegrationDB(t)
	addJellyfinLibrary(t, db, 15, t.TempDir())
	var discoveryAttempts int
	var scans int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Library/MediaFolders":
			discoveryAttempts++
			if discoveryAttempts == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(`{"Items":[{"Id":"library-15","Name":"TV"}]}`))
		case "/Library/Refresh":
			scans++
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	settings := defaultJellyfinSettings()
	settings.Enabled, settings.BaseURL, settings.APIToken = true, server.URL, "secret"
	settings.DebounceSeconds = 0
	settings.Mappings = []jellyfinLibraryMapping{{MVForgeLibraryID: 15, JellyfinLibraryID: "library-15"}}
	saveJellyfinSettings(t, db, settings)
	if err := recordJellyfinTrigger(db, jellyfinTrigger{Reason: jellyfinReasonTestReady, SourceKey: testEncodeJellyfinSourceKey(jellyfinReasonTestReady, 88), LibraryID: 15, TestEncodeID: 88}); err != nil {
		t.Fatal(err)
	}

	// A newly constructed coordinator reloads the durable due row and records
	// attempt intent before the fake Jellyfin endpoint responds.
	coordinator := &JellyfinCoordinator{db: db}
	coordinator.processDue()
	var outcome models.JellyfinIntegrationOutcome
	if err := db.First(&outcome, "source_key = ?", testEncodeJellyfinSourceKey(jellyfinReasonTestReady, 88)).Error; err != nil {
		t.Fatal(err)
	}
	if outcome.Status != jellyfinStatusPending || outcome.AttemptCount != 1 || outcome.FailureClass != "server_error" || scans != 0 {
		t.Fatalf("first attempt outcome=%#v scans=%d", outcome, scans)
	}
	if err := db.Model(&outcome).Updates(map[string]any{"due_at": time.Now().UTC().Add(-time.Second), "next_attempt_at": nil}).Error; err != nil {
		t.Fatal(err)
	}
	coordinator.processDue()
	if err := db.First(&outcome, outcome.ID).Error; err != nil {
		t.Fatal(err)
	}
	if outcome.Status != jellyfinStatusSuccess || outcome.AttemptCount != 2 || outcome.EffectiveScope != "server_wide" || scans != 1 {
		t.Fatalf("recovered retry outcome=%#v scans=%d", outcome, scans)
	}
	_, revision := loadJellyfinSettings(db)
	unknown := models.JellyfinIntegrationOutcome{
		SourceKey: "test_encode_ready:89", Reason: jellyfinReasonTestReady, TestEncodeID: 89, LibraryID: 15,
		MappedLibraryID: "library-15", EffectiveTarget: jellyfinTargetServerWide, ConfigRevision: revision,
		Status: jellyfinStatusPending, AttemptCount: jellyfinMaximumAttempts, FirstTriggeredAt: time.Now().UTC(),
		DueAt: time.Now().UTC().Add(-time.Second),
	}
	if err := db.Create(&unknown).Error; err != nil {
		t.Fatal(err)
	}
	coordinator.processDue()
	if err := db.First(&unknown, unknown.ID).Error; err != nil {
		t.Fatal(err)
	}
	if unknown.Status != jellyfinStatusRetryableFailure || unknown.AttemptCount != jellyfinMaximumAttempts || scans != 1 {
		t.Fatalf("recovered fifth intent exceeded attempt bound: outcome=%#v scans=%d", unknown, scans)
	}
}

func TestJellyfinFailureDoesNotMutatePublishedOrReadyLifecycleState(t *testing.T) {
	db := jellyfinIntegrationDB(t)
	addJellyfinLibrary(t, db, 24, t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Library/MediaFolders" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("credential rejected"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	settings := defaultJellyfinSettings()
	settings.Enabled, settings.BaseURL, settings.APIToken, settings.DebounceSeconds = true, server.URL, "stored-secret", 0
	settings.Mappings = []jellyfinLibraryMapping{{MVForgeLibraryID: 24, JellyfinLibraryID: "jelly-24"}}
	saveJellyfinSettings(t, db, settings)

	publication := models.PublisherPublication{
		PublicationID: "publication-24", QueueJobID: 24, ExecutionNumber: 1, ExecutionPlanID: 1,
		ExecutionPlanVersion: 1, ExecutionPlanFingerprint: "plan", LibraryID: 24, DestinationRoot: "/library",
		DestinationPath: "/library/movie.mkv", PublishMode: "replace", ResultArtifactID: "artifact-24",
		ResultGenerationToken: "generation", ResultGenerationVersion: 1, ResultOutputRevision: "revision",
		ValidationRunID: "validation-24", SourcePath: "/raw/movie.mkv", State: PublicationPublished,
		OriginalArchiveStatus: "complete", Manifest: models.JSONList{},
	}
	if err := db.Create(&publication).Error; err != nil {
		t.Fatal(err)
	}
	readyTest := models.TestEncode{
		ID: 25, SourcePath: "/raw/test-source.mkv", LibraryID: 24, ConfigurationSource: "global",
		ConfigurationHash: "configuration", Status: testEncodeReady, Phase: "ready", Keep: true,
	}
	if err := db.Create(&readyTest).Error; err != nil {
		t.Fatal(err)
	}
	coordinator := JellyfinCoordinator{db: db}
	if err := coordinator.assetPublished(context.Background(), "asset-published-publication-24", models.JSONMap{"publicationId": publication.PublicationID, "libraryId": float64(24), "destinationPath": publication.DestinationPath}); err != nil {
		t.Fatal(err)
	}
	if err := recordJellyfinTestEncodeReady(db, readyTest.ID, readyTest.LibraryID, true, true); err != nil {
		t.Fatal(err)
	}
	coordinator.processDue()

	var persistedPublication models.PublisherPublication
	if err := db.First(&persistedPublication, publication.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persistedPublication.State != PublicationPublished || persistedPublication.OriginalArchiveStatus != "complete" {
		t.Fatalf("Jellyfin failure changed publication lifecycle: state=%q archive=%q", persistedPublication.State, persistedPublication.OriginalArchiveStatus)
	}
	var persistedTest models.TestEncode
	if err := db.First(&persistedTest, readyTest.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persistedTest.Status != testEncodeReady || !persistedTest.Keep || persistedTest.SourcePath != readyTest.SourcePath {
		t.Fatalf("Jellyfin failure changed Test Encode lifecycle: status=%q keep=%v source=%q", persistedTest.Status, persistedTest.Keep, persistedTest.SourcePath)
	}
	var count int64
	if err := db.Model(&models.JellyfinIntegrationOutcome{}).Where("status = ? AND failure_class = ?", jellyfinStatusPermanentError, "invalid_credentials").Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("bounded Jellyfin failures=%d err=%v", count, err)
	}
}

func TestJellyfinDisabledAndUnmappedTriggersAreBoundedNoOps(t *testing.T) {
	db := jellyfinIntegrationDB(t)
	settings := defaultJellyfinSettings()
	if err := recordJellyfinTrigger(db, jellyfinTrigger{Reason: jellyfinReasonPublished, SourceKey: "asset-published:disabled", LibraryID: 3}); err != nil {
		t.Fatal(err)
	}
	settings.Enabled = true
	saveJellyfinSettings(t, db, settings)
	if err := recordJellyfinTrigger(db, jellyfinTrigger{Reason: jellyfinReasonTestReady, SourceKey: testEncodeJellyfinSourceKey(jellyfinReasonTestReady, 33), LibraryID: 3, TestEncodeID: 33}); err != nil {
		t.Fatal(err)
	}
	var outcomes []models.JellyfinIntegrationOutcome
	if err := db.Order("id asc").Find(&outcomes).Error; err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 2 || outcomes[0].Status != jellyfinStatusSkippedDisabled || outcomes[1].Status != jellyfinStatusSkippedUnmapped {
		t.Fatalf("bounded no-op outcomes=%#v", outcomes)
	}
}

func TestTestEncodePartialCleanupRequestsReconciliationWithoutDeletingRecord(t *testing.T) {
	db := jellyfinIntegrationDB(t)
	root := t.TempDir()
	addJellyfinLibrary(t, db, 12, root)
	settings := defaultJellyfinSettings()
	settings.Enabled, settings.BaseURL, settings.APIToken = true, "http://127.0.0.1:8096", "secret"
	settings.Mappings = []jellyfinLibraryMapping{{MVForgeLibraryID: 12, JellyfinLibraryID: "jelly-12"}}
	saveJellyfinSettings(t, db, settings)
	output := filepath.Join(root, "ready.mkv")
	if err := os.WriteFile(output, []byte("temporary test media"), 0o600); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(root, "blocked-sidecar")
	if err := os.Mkdir(blocked, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blocked, "child"), []byte("prevents removal"), 0o600); err != nil {
		t.Fatal(err)
	}
	testEncode := models.TestEncode{
		ID: 44, SourcePath: filepath.Join(root, "source.mkv"), LibraryID: 12, ConfigurationSource: "global",
		ConfigurationHash: "hash", Status: testEncodeReady, Phase: "ready", OutputPath: output,
		SubtitleArtifacts: models.JSONList{models.JSONMap{"stagedPath": blocked}},
	}
	if err := db.Create(&testEncode).Error; err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.DELETE("/api/test-encodes/:id", (AssetHandler{db: db}).DeleteTestEncode)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/test-encodes/44", nil))
	if response.Code != http.StatusConflict {
		t.Fatalf("partial cleanup status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(output); err != nil {
		t.Fatalf("preflight failure removed or changed an owned artifact, stat err=%v", err)
	}
	var persisted models.TestEncode
	if err := db.First(&persisted, testEncode.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Status != testEncodeReady || persisted.DeletedAt != nil {
		t.Fatalf("partial cleanup changed Test Encode lifecycle: status=%q deletedAt=%v", persisted.Status, persisted.DeletedAt)
	}
	var outcomeCount int64
	if err := db.Model(&models.JellyfinIntegrationOutcome{}).Where("source_key = ?", testEncodeJellyfinSourceKey(jellyfinReasonTestRemoved, testEncode.ID)).Count(&outcomeCount).Error; err != nil {
		t.Fatal(err)
	}
	if outcomeCount != 0 {
		t.Fatalf("preflight rejection unexpectedly requested Jellyfin reconciliation")
	}
}

func TestReadyTestEncodeDeletionAndExpirationRefreshAfterRemoval(t *testing.T) {
	for _, mode := range []string{"delete", "expiration"} {
		t.Run(mode, func(t *testing.T) {
			db := jellyfinIntegrationDB(t)
			root := t.TempDir()
			addJellyfinLibrary(t, db, 19, root)
			settings := defaultJellyfinSettings()
			settings.Enabled, settings.BaseURL, settings.APIToken = true, "http://127.0.0.1:8096", "secret"
			settings.Mappings = []jellyfinLibraryMapping{{MVForgeLibraryID: 19, JellyfinLibraryID: "jelly-19"}}
			saveJellyfinSettings(t, db, settings)
			output := filepath.Join(root, "ready.mkv")
			if err := os.WriteFile(output, []byte("test"), 0o600); err != nil {
				t.Fatal(err)
			}
			expires := time.Now().Add(-time.Hour)
			testEncode := models.TestEncode{ID: 71, SourcePath: filepath.Join(root, "source.mkv"), LibraryID: 19, ConfigurationSource: "global", ConfigurationHash: "hash", Status: testEncodeReady, Phase: "ready", OutputPath: output, ExpiresAt: &expires}
			if err := db.Create(&testEncode).Error; err != nil {
				t.Fatal(err)
			}
			if mode == "delete" {
				gin.SetMode(gin.TestMode)
				router := gin.New()
				router.DELETE("/api/test-encodes/:id", (AssetHandler{db: db}).DeleteTestEncode)
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/api/test-encodes/71", nil))
				if response.Code != http.StatusOK {
					t.Fatalf("delete status=%d body=%s", response.Code, response.Body.String())
				}
			} else {
				retainedPath := filepath.Join(root, "kept.mkv")
				if err := os.WriteFile(retainedPath, []byte("kept"), 0o600); err != nil {
					t.Fatal(err)
				}
				retained := models.TestEncode{ID: 72, SourcePath: filepath.Join(root, "source-kept.mkv"), LibraryID: 19, ConfigurationSource: "global", ConfigurationHash: "hash", Status: testEncodeReady, Phase: "ready", OutputPath: retainedPath, ExpiresAt: &expires, Keep: true}
				if err := db.Create(&retained).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.Create(&models.AppSetting{Key: "storageRoles", Value: models.JSONMap{"work": models.JSONMap{"path": filepath.Join(root, "missing-work")}}}).Error; err != nil {
					t.Fatal(err)
				}
				report, err := RunHousekeeping(db, false)
				if err != nil || len(report.Errors) > 0 || len(report.RemovedPaths) != 1 {
					t.Fatalf("expiration report=%#v err=%v", report, err)
				}
				if _, err := os.Stat(retainedPath); err != nil {
					t.Fatalf("Keep=true Test Encode was removed: %v", err)
				}
				var retainedOutcomes int64
				if err := db.Model(&models.JellyfinIntegrationOutcome{}).Where("test_encode_id = ? AND reason = ?", retained.ID, jellyfinReasonTestRemoved).Count(&retainedOutcomes).Error; err != nil || retainedOutcomes != 0 {
					t.Fatalf("retained Test Encode generated removal outcomes=%d err=%v", retainedOutcomes, err)
				}
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatalf("removed Test Encode output remains: %v", err)
			}
			var outcome models.JellyfinIntegrationOutcome
			if err := db.First(&outcome, "source_key = ?", testEncodeJellyfinSourceKey(jellyfinReasonTestRemoved, testEncode.ID)).Error; err != nil {
				t.Fatalf("removal refresh was not recorded: %v", err)
			}
			if outcome.Status != jellyfinStatusPending || outcome.LibraryID != testEncode.LibraryID {
				t.Fatalf("removal outcome=%#v", outcome)
			}
		})
	}
}

func TestJellyfinRetryBackoffIsBoundedAndSettingsResponseOmitsToken(t *testing.T) {
	if jellyfinBackoff(1) != time.Second || jellyfinBackoff(4) != 2*time.Minute || jellyfinBackoff(5) != 2*time.Minute {
		t.Fatalf("unexpected bounded backoff: %v %v %v", jellyfinBackoff(1), jellyfinBackoff(4), jellyfinBackoff(5))
	}
	view, err := json.Marshal(jellyfinSettingsProjection(jellyfinSettings{BaseURL: "http://example", APIToken: "never-return-this"}))
	if err != nil || strings.Contains(string(view), "never-return-this") || !strings.Contains(string(view), `"credentialConfigured":true`) {
		t.Fatalf("settings projection=%s err=%v", view, err)
	}
}

func TestJellyfinPermanentConfigurationFailureBecomesRetryableAfterCorrection(t *testing.T) {
	settings := defaultJellyfinSettings()
	settings.Enabled, settings.BaseURL, settings.APIToken = true, "http://jellyfin:8096", "corrected-secret"
	settings.Mappings = []jellyfinLibraryMapping{{MVForgeLibraryID: 5, JellyfinLibraryID: "library-five"}}
	outcome := models.JellyfinIntegrationOutcome{LibraryID: 5, Reason: jellyfinReasonPublished, Status: jellyfinStatusPermanentError, FailureClass: "invalid_credentials", ConfigRevision: 10}
	if jellyfinOutcomeRetryEligible(outcome, settings, 10) {
		t.Fatal("unchanged invalid configuration became retryable")
	}
	if !jellyfinOutcomeRetryEligible(outcome, settings, 11) {
		t.Fatal("corrected configuration did not make the outcome eligible for retry")
	}
	outcome.FailureClass = "invalid_event"
	if jellyfinOutcomeRetryEligible(outcome, settings, 11) {
		t.Fatal("malformed canonical event became retryable")
	}
}

func TestJellyfinStaleMixedReasonBatchChecksConfigurationRevisionFirst(t *testing.T) {
	var jellyfinRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		jellyfinRequests.Add(1)
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	db := jellyfinIntegrationDB(t)
	addJellyfinLibrary(t, db, 9, t.TempDir())
	settings := defaultJellyfinSettings()
	settings.Enabled = true
	settings.BaseURL = server.URL
	settings.APIToken = "secret"
	settings.RefreshAfterPublish = true
	settings.RefreshTestEncode = true
	settings.DebounceSeconds = 60
	settings.Mappings = []jellyfinLibraryMapping{{MVForgeLibraryID: 9, JellyfinLibraryID: "Exact-Jellyfin-ID"}}
	saveJellyfinSettings(t, db, settings)
	_, initialRevision, err := loadJellyfinSettingsChecked(db)
	if err != nil {
		t.Fatal(err)
	}

	publishedAt := time.Now().UTC()
	publication := models.PublisherPublication{
		PublicationID: "publication-9", QueueJobID: 9, ExecutionNumber: 1, ExecutionPlanID: 9,
		ExecutionPlanVersion: 1, ExecutionPlanFingerprint: "plan-fingerprint", LibraryID: 9,
		DestinationRoot: "/library", DestinationPath: "/library/series", PublishMode: "move",
		ResultArtifactID: "artifact-9", ResultGenerationToken: "generation-9", ResultGenerationVersion: 1,
		ResultOutputRevision: "output-revision", ValidationRunID: "validation-9", SourcePath: "/source/series.mkv",
		State: "PUBLISHED", Manifest: models.JSONList{"series.mkv"}, PublishedAt: &publishedAt,
		OriginalArchiveStatus: "archived",
	}
	if err := db.Create(&publication).Error; err != nil {
		t.Fatal(err)
	}
	testEncode := models.TestEncode{
		ID: 22, SourcePath: "/source/test.mkv", LibraryID: 9, ConfigurationSource: "profile",
		ConfigurationHash: "configuration-hash", Status: "READY", Phase: "ready",
		OutputPath: "/library/test.mkv", Keep: true,
	}
	if err := db.Create(&testEncode).Error; err != nil {
		t.Fatal(err)
	}

	// Insert READY first so its now-disabled reason is the first row in the shared server-wide batch.
	if err := recordJellyfinTrigger(db, jellyfinTrigger{
		Reason: jellyfinReasonTestReady, SourceKey: testEncodeJellyfinSourceKey(jellyfinReasonTestReady, testEncode.ID),
		LibraryID: testEncode.LibraryID, TestEncodeID: testEncode.ID,
	}); err != nil {
		t.Fatal(err)
	}
	coordinator := JellyfinCoordinator{db: db}
	if err := coordinator.assetPublished(context.Background(), "event-9", models.JSONMap{
		"publicationId":   publication.PublicationID,
		"libraryId":       float64(publication.LibraryID),
		"destinationPath": publication.DestinationPath,
	}); err != nil {
		t.Fatal(err)
	}

	var beforeBatch []models.JellyfinIntegrationOutcome
	if err := db.Order("id asc").Find(&beforeBatch).Error; err != nil {
		t.Fatal(err)
	}
	if len(beforeBatch) != 2 || beforeBatch[0].Reason != jellyfinReasonTestReady || beforeBatch[1].Reason != jellyfinReasonPublished || beforeBatch[0].ConfigRevision != initialRevision || beforeBatch[1].ConfigRevision != initialRevision || beforeBatch[0].EffectiveTarget != jellyfinTargetServerWide || beforeBatch[1].EffectiveTarget != jellyfinTargetServerWide {
		t.Fatalf("expected one mixed pending server-wide batch, got %#v", beforeBatch)
	}

	settings.RefreshTestEncode = false
	var stored models.AppSetting
	if err := db.First(&stored, "key = ?", jellyfinSettingsKey).Error; err != nil {
		t.Fatal(err)
	}
	changedAt := stored.UpdatedAt.Add(time.Second)
	if err := db.Model(&stored).Updates(map[string]any{
		"value":      jellyfinSettingsStoredValue(settings),
		"updated_at": changedAt,
	}).Error; err != nil {
		t.Fatal(err)
	}
	_, currentRevision, err := loadJellyfinSettingsChecked(db)
	if err != nil {
		t.Fatal(err)
	}
	if currentRevision == initialRevision {
		t.Fatal("settings update did not advance the configuration revision")
	}
	if err := db.Model(&models.JellyfinIntegrationOutcome{}).Where("status = ?", jellyfinStatusPending).Update("due_at", time.Now().UTC().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}

	coordinator.processDue()
	if jellyfinRequests.Load() != 0 {
		t.Fatalf("stale batch sent %d request(s) to Jellyfin", jellyfinRequests.Load())
	}
	var outcomes []models.JellyfinIntegrationOutcome
	if err := db.Order("id asc").Find(&outcomes).Error; err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 2 {
		t.Fatalf("integration outcomes=%#v", outcomes)
	}
	for _, outcome := range outcomes {
		if outcome.Status != jellyfinStatusPermanentError || outcome.FailureClass != "configuration_changed" || outcome.ConfigRevision != initialRevision {
			t.Errorf("stale outcome was not retained as configuration_changed: %#v", outcome)
		}
	}
	if outcomes[0].Reason != jellyfinReasonTestReady || outcomes[1].Reason != jellyfinReasonPublished {
		t.Fatalf("bounded integration reasons changed: %#v", outcomes)
	}
	currentSettings, _, err := loadJellyfinSettingsChecked(db)
	if err != nil {
		t.Fatal(err)
	}
	if !jellyfinOutcomeRetryEligible(outcomes[1], currentSettings, currentRevision) {
		t.Error("asset_published was not eligible for retry under the current settings")
	}
	if jellyfinOutcomeRetryEligible(outcomes[0], currentSettings, currentRevision) {
		t.Error("test_encode_ready was eligible after its reason was disabled")
	}

	var afterPublication models.PublisherPublication
	if err := db.First(&afterPublication, "publication_id = ?", publication.PublicationID).Error; err != nil {
		t.Fatal(err)
	}
	var afterTestEncode models.TestEncode
	if err := db.First(&afterTestEncode, testEncode.ID).Error; err != nil {
		t.Fatal(err)
	}
	if afterPublication.State != publication.State || afterPublication.OriginalArchiveStatus != publication.OriginalArchiveStatus || afterPublication.PublishedAt == nil || !afterPublication.PublishedAt.Equal(*publication.PublishedAt) {
		t.Errorf("publication lifecycle evidence changed: before=%#v after=%#v", publication, afterPublication)
	}
	if afterTestEncode.Status != testEncode.Status || afterTestEncode.Keep != testEncode.Keep || afterTestEncode.DeletedAt != nil {
		t.Errorf("Test Encode lifecycle evidence changed: before=%#v after=%#v", testEncode, afterTestEncode)
	}
}

func jellyfinPersistenceRouter(db *gorm.DB) *gin.Engine {
	settings := NewSettingsHandler(db)
	jellyfin := NewJellyfinHandler(db)
	router := gin.New()
	router.GET("/api/settings/jellyfin/state", settings.JellyfinIntegrationState)
	router.POST("/api/settings/jellyfin/test-connection", settings.TestJellyfinConnection)
	router.GET("/api/settings", settings.List)
	router.POST("/api/settings/:key", settings.Update)
	router.POST("/api/jellyfin/manual-refresh", jellyfin.ManualRefresh)
	router.POST("/api/jellyfin/outcomes/:id/retry", jellyfin.Retry)
	return router
}

func jellyfinPersistenceRequest(t *testing.T, router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func jellyfinJSONMap(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, body)
	}
	return value
}

func TestJellyfinConnectionStatePersistsAndSurvivesRestartAndOutage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/System/Info/Public":
			_, _ = w.Write([]byte(`{"Id":"jellyfin-server-a","ServerName":"Home","Version":"10.10.0"}`))
		case "/Library/MediaFolders":
			_, _ = w.Write([]byte(`{"Items":[{"Id":"jellyfin-library-44","Name":"Anime","CollectionType":"tvshows"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	db := jellyfinIntegrationDB(t)
	addJellyfinLibrary(t, db, 7, t.TempDir())
	settings := defaultJellyfinSettings()
	settings.Enabled, settings.BaseURL, settings.APIToken = true, server.URL, "stored-secret-for-redaction"
	settings.Mappings = []jellyfinLibraryMapping{{MVForgeLibraryID: 7, JellyfinLibraryID: "jellyfin-library-44", JellyfinName: "Anime"}}
	saveJellyfinSettings(t, db, settings)

	response := jellyfinPersistenceRequest(t, jellyfinPersistenceRouter(db), http.MethodPost, "/api/settings/jellyfin/test-connection", `{}`)
	result := jellyfinJSONMap(t, response.Body.Bytes())
	if response.Code != http.StatusOK || result["authenticated"] != true || result["serverId"] != "jellyfin-server-a" {
		t.Fatalf("test connection status=%d result=%#v", response.Code, result)
	}
	if strings.Contains(response.Body.String(), "stored-secret-for-redaction") {
		t.Fatal("connection response exposed the stored API token")
	}

	// A newly constructed handler represents a browser reload or backend restart.
	restored, revision, err := loadJellyfinSettingsChecked(db)
	if err != nil || restored.LastSuccessfulConnectionAt == "" || restored.LastSuccessfulConfigRevision != restored.ConnectionConfigRevision || restored.ServerID != "jellyfin-server-a" || len(restored.DiscoveredLibraries) != 1 || restored.Mappings[0].JellyfinLibraryID != "jellyfin-library-44" || restored.Mappings[0].JellyfinServerID != "jellyfin-server-a" || revision == 0 {
		t.Fatalf("persisted connection state not restored: settings=%#v revision=%d err=%v", restored, revision, err)
	}

	server.Close()
	restartedRouter := jellyfinPersistenceRouter(db)
	stateResponse := jellyfinPersistenceRequest(t, restartedRouter, http.MethodGet, "/api/settings/jellyfin/state", "")
	state := jellyfinJSONMap(t, stateResponse.Body.Bytes())
	if stateResponse.Code != http.StatusOK || state["enabled"] != true || state["credentialConfigured"] != true || state["configurationValid"] != true || state["lastConnectionSuccessful"] != true || state["liveReachability"] != "unavailable" || state["librariesLoaded"] != true {
		t.Fatalf("outage lost persisted integration semantics: status=%d state=%#v", stateResponse.Code, state)
	}
	mappings, ok := state["mappings"].([]any)
	if !ok || len(mappings) != 1 || mappings[0].(map[string]any)["jellyfinLibraryId"] != "jellyfin-library-44" || mappings[0].(map[string]any)["needsRemap"] != false {
		t.Fatalf("mapping did not survive outage: %#v", state["mappings"])
	}
	listed := jellyfinPersistenceRequest(t, restartedRouter, http.MethodGet, "/api/settings", "")
	if strings.Contains(listed.Body.String(), "stored-secret-for-redaction") || !strings.Contains(listed.Body.String(), `"lastConnectionSuccessful":true`) || !strings.Contains(listed.Body.String(), `"credentialConfigured":true`) {
		t.Fatalf("settings hydration lost saved state or exposed the secret: %s", listed.Body.String())
	}
}

func TestJellyfinStateHydrationDoesNotInvalidatePendingRefresh(t *testing.T) {
	var scans int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/System/Info/Public":
			_, _ = w.Write([]byte(`{"Id":"hydration-server","ServerName":"Home","Version":"10.10.0"}`))
		case "/Library/MediaFolders":
			_, _ = w.Write([]byte(`{"Items":[{"Id":"hydration-library","Name":"Anime"}]}`))
		case "/Library/Refresh":
			scans++
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	db := jellyfinIntegrationDB(t)
	addJellyfinLibrary(t, db, 7, t.TempDir())
	settings := defaultJellyfinSettings()
	settings.Enabled, settings.BaseURL, settings.APIToken = true, server.URL, "stored-secret"
	settings.DebounceSeconds = 0
	settings.Mappings = []jellyfinLibraryMapping{{MVForgeLibraryID: 7, JellyfinLibraryID: "hydration-library"}}
	saveJellyfinSettings(t, db, settings)
	if err := recordJellyfinTrigger(db, jellyfinTrigger{Reason: jellyfinReasonTestReady, SourceKey: "hydration-pending-refresh", LibraryID: 7, TestEncodeID: 77}); err != nil {
		t.Fatal(err)
	}
	var pending models.JellyfinIntegrationOutcome
	if err := db.First(&pending, "source_key = ?", "hydration-pending-refresh").Error; err != nil {
		t.Fatal(err)
	}
	_, beforeRevision, err := loadJellyfinSettingsChecked(db)
	if err != nil || pending.ConfigRevision != beforeRevision {
		t.Fatalf("pending revision=%d current revision=%d err=%v", pending.ConfigRevision, beforeRevision, err)
	}

	state := jellyfinPersistenceRequest(t, jellyfinPersistenceRouter(db), http.MethodGet, "/api/settings/jellyfin/state", "")
	if state.Code != http.StatusOK {
		t.Fatalf("state hydration status=%d body=%s", state.Code, state.Body.String())
	}
	_, afterRevision, err := loadJellyfinSettingsChecked(db)
	if err != nil || afterRevision != beforeRevision {
		t.Fatalf("connection metadata changed the configuration revision: before=%d after=%d err=%v", beforeRevision, afterRevision, err)
	}
	if err := db.First(&pending, pending.ID).Error; err != nil || pending.ConfigRevision != afterRevision || pending.Status != jellyfinStatusPending {
		t.Fatalf("hydration invalidated pending work: outcome=%#v err=%v", pending, err)
	}

	(&JellyfinCoordinator{db: db}).processDue()
	if err := db.First(&pending, pending.ID).Error; err != nil {
		t.Fatal(err)
	}
	if pending.Status != jellyfinStatusSuccess || pending.FailureClass == "configuration_changed" || scans != 1 {
		t.Fatalf("hydrated pending work did not complete successfully: outcome=%#v scans=%d", pending, scans)
	}
}

func TestJellyfinSettingsLoadAutomaticallyRediscoversLibraries(t *testing.T) {
	var discoveryCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/System/Info/Public" {
			_, _ = w.Write([]byte(`{"Id":"jellyfin-server-auto","Version":"10.10.0"}`))
			return
		}
		if r.URL.Path == "/Library/MediaFolders" {
			discoveryCalls.Add(1)
			_, _ = w.Write([]byte(`{"Items":[{"Id":"library-auto-1","Name":"Movies","CollectionType":"movies"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	db := jellyfinIntegrationDB(t)
	addJellyfinLibrary(t, db, 8, t.TempDir())
	if err := (gormProtectedSecretSource{}).Put(db, jellyfinAPITokenCredentialRef, "auto-discovery-secret"); err != nil {
		t.Fatal(err)
	}
	legacySettings := models.AppSetting{
		Key: jellyfinSettingsKey,
		Value: models.JSONMap{
			"enabled": true, "baseUrl": server.URL, "credentialRef": string(jellyfinAPITokenCredentialRef),
			"refreshAfterPublish": true, "refreshTestEncode": true, "debounceSeconds": 10,
			"mappings": models.JSONList{models.JSONMap{"mvforgeLibraryId": 8, "jellyfinLibraryId": "library-auto-1", "jellyfinName": "Movies"}},
		},
	}
	if err := db.Create(&legacySettings).Error; err != nil {
		t.Fatal(err)
	}

	response := jellyfinPersistenceRequest(t, jellyfinPersistenceRouter(db), http.MethodGet, "/api/settings/jellyfin/state", "")
	state := jellyfinJSONMap(t, response.Body.Bytes())
	libraries, ok := state["libraries"].([]any)
	if response.Code != http.StatusOK || !ok || len(libraries) != 1 || libraries[0].(map[string]any)["id"] != "library-auto-1" || discoveryCalls.Load() != 1 {
		t.Fatalf("settings hydration did not rediscover Jellyfin Libraries: status=%d calls=%d state=%#v", response.Code, discoveryCalls.Load(), state)
	}
	if strings.Contains(response.Body.String(), "auto-discovery-secret") {
		t.Fatal("automatic discovery response exposed the API token")
	}
	loaded, _ := loadJellyfinSettings(db)
	if loaded.ConnectionConfigRevision != 1 || loaded.Mappings[0].JellyfinLibraryID != "library-auto-1" || loaded.Mappings[0].JellyfinServerID != "jellyfin-server-auto" {
		t.Fatalf("legacy settings or mapping did not hydrate with persisted discovery metadata: %#v", loaded)
	}
}

func TestJellyfinSettingsChangesRequireRevalidationAndPreserveMappings(t *testing.T) {
	db := jellyfinIntegrationDB(t)
	addJellyfinLibrary(t, db, 7, t.TempDir())
	settings := defaultJellyfinSettings()
	settings.Enabled, settings.BaseURL, settings.APIToken = true, "http://jellyfin-old:8096", "old-secret"
	settings.LastSuccessfulConnectionAt = time.Now().UTC().Format(time.RFC3339Nano)
	settings.LastSuccessfulConfigRevision = settings.ConnectionConfigRevision
	settings.ServerID, settings.ServerVersion = "server-old", "10.10.0"
	settings.DiscoveredLibraries = []jellyfinLibrary{{ID: "library-old", Name: "Anime"}}
	settings.LibrariesDiscoveredAt = settings.LastSuccessfulConnectionAt
	settings.LibrariesConfigRevision = settings.ConnectionConfigRevision
	settings.Mappings = []jellyfinLibraryMapping{{MVForgeLibraryID: 7, JellyfinLibraryID: "library-old", JellyfinServerID: "server-old"}}
	saveJellyfinSettings(t, db, settings)
	router := jellyfinPersistenceRouter(db)

	changeURL := `{"value":{"enabled":true,"baseUrl":"http://jellyfin-new:8096","apiToken":"old-secret","mappings":[{"mvforgeLibraryId":7,"jellyfinLibraryId":"library-old"}]}}`
	response := jellyfinPersistenceRequest(t, router, http.MethodPost, "/api/settings/jellyfin", changeURL)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "old-secret") {
		t.Fatalf("base URL update status=%d response=%s", response.Code, response.Body.String())
	}
	updated, _ := loadJellyfinSettings(db)
	if updated.ConnectionConfigRevision != settings.ConnectionConfigRevision+1 || updated.Mappings[0].JellyfinLibraryID != "library-old" || updated.ServerID != "server-old" {
		t.Fatalf("base URL change cleared or failed to invalidate mapping state: %#v", updated)
	}
	projection := jellyfinSettingsProjection(updated)
	if projection["configurationNeedsRevalidation"] != true || projection["librariesLoaded"] != false {
		t.Fatalf("base URL change was not exposed as requiring revalidation: %#v", projection)
	}

	changeCredential := `{"value":{"enabled":true,"baseUrl":"http://jellyfin-new:8096","apiToken":"new-secret","mappings":[{"mvforgeLibraryId":7,"jellyfinLibraryId":"library-old"}]}}`
	response = jellyfinPersistenceRequest(t, router, http.MethodPost, "/api/settings/jellyfin", changeCredential)
	updated, _ = loadJellyfinSettings(db)
	if response.Code != http.StatusOK || updated.APIToken != "new-secret" || updated.ConnectionConfigRevision != settings.ConnectionConfigRevision+2 || updated.Mappings[0].JellyfinLibraryID != "library-old" {
		t.Fatalf("credential change did not invalidate while preserving mapping: status=%d settings=%#v", response.Code, updated)
	}
	listed := jellyfinPersistenceRequest(t, router, http.MethodGet, "/api/settings", "")
	if strings.Contains(listed.Body.String(), "new-secret") {
		t.Fatal("updated credential was returned by settings hydration")
	}
}

func TestJellyfinRediscoveryMarksMissingAndForeignServerMappingsStale(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/System/Info/Public":
			_, _ = w.Write([]byte(`{"Id":"server-new","Version":"10.10.0"}`))
		case "/Library/MediaFolders":
			_, _ = w.Write([]byte(`{"Items":[{"Id":"shared-library","Name":"Anime"},{"Id":"different-library","Name":"Drama"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	db := jellyfinIntegrationDB(t)
	addJellyfinLibrary(t, db, 7, t.TempDir())
	addJellyfinLibrary(t, db, 8, t.TempDir())
	settings := defaultJellyfinSettings()
	settings.Enabled, settings.BaseURL, settings.APIToken = true, server.URL, "secret"
	settings.RefreshTestEncode = true
	settings.ServerID, settings.Mappings = "server-old", []jellyfinLibraryMapping{
		{MVForgeLibraryID: 7, JellyfinLibraryID: "shared-library", JellyfinName: "Anime", JellyfinServerID: "server-old"},
		{MVForgeLibraryID: 8, JellyfinLibraryID: "missing-library", JellyfinName: "Missing", JellyfinServerID: "server-new"},
	}
	saveJellyfinSettings(t, db, settings)

	router := jellyfinPersistenceRouter(db)
	response := jellyfinPersistenceRequest(t, router, http.MethodGet, "/api/settings/jellyfin/state", "")
	state := jellyfinJSONMap(t, response.Body.Bytes())
	mappings, ok := state["mappings"].([]any)
	if response.Code != http.StatusOK || !ok || len(mappings) != 2 {
		t.Fatalf("state response status=%d body=%s", response.Code, response.Body.String())
	}
	needsRemap := map[uint]bool{}
	for _, raw := range mappings {
		mapping := raw.(map[string]any)
		needsRemap[uint(mapping["mvforgeLibraryId"].(float64))] = mapping["needsRemap"] == true
	}
	if !needsRemap[7] || !needsRemap[8] || state["serverId"] != "server-new" {
		t.Fatalf("foreign same-ID or missing mapping was not retained and marked stale: state=%#v", state)
	}
	loaded, _ := loadJellyfinSettings(db)
	if len(loaded.Mappings) != 2 || loaded.Mappings[0].JellyfinLibraryID != "shared-library" {
		t.Fatalf("rediscovery silently deleted stale mapping: %#v", loaded.Mappings)
	}
	for _, libraryID := range []uint{7, 8} {
		body := fmt.Sprintf(`{"libraryId":%d}`, libraryID)
		manual := jellyfinPersistenceRequest(t, router, http.MethodPost, "/api/jellyfin/manual-refresh", body)
		if manual.Code != http.StatusConflict {
			t.Fatalf("manual refresh accepted mapping %d needing remap: status=%d body=%s", libraryID, manual.Code, manual.Body.String())
		}
	}

	_, revision := loadJellyfinSettings(db)
	staleOutcome := models.JellyfinIntegrationOutcome{
		SourceKey: "stale-mapping-retry", Reason: jellyfinReasonTestReady, TestEncodeID: 909, LibraryID: 7,
		MappedLibraryID: "shared-library", EffectiveTarget: jellyfinTargetServerWide, ConfigRevision: revision - 1,
		Status: jellyfinStatusPermanentError, FailureClass: "stale_mapping", FirstTriggeredAt: time.Now().UTC(), DueAt: time.Now().UTC(),
	}
	if err := db.Create(&staleOutcome).Error; err != nil {
		t.Fatal(err)
	}
	retry := jellyfinPersistenceRequest(t, router, http.MethodPost, fmt.Sprintf("/api/jellyfin/outcomes/%d/retry", staleOutcome.ID), "{}")
	if retry.Code != http.StatusConflict {
		t.Fatalf("retry accepted stale foreign-server mapping: status=%d body=%s", retry.Code, retry.Body.String())
	}
	if err := recordJellyfinTrigger(db, jellyfinTrigger{Reason: jellyfinReasonTestReady, SourceKey: "stale-auto-trigger", LibraryID: 8, TestEncodeID: 910}); err != nil {
		t.Fatal(err)
	}
	var automatic models.JellyfinIntegrationOutcome
	if err := db.First(&automatic, "source_key = ?", "stale-auto-trigger").Error; err != nil || automatic.Status != jellyfinStatusPermanentError || automatic.FailureClass != "stale_mapping" {
		t.Fatalf("automatic trigger did not retain stale-mapping failure: outcome=%#v err=%v", automatic, err)
	}

	remapBody := fmt.Sprintf(`{"value":{"enabled":true,"baseUrl":%q,"mappings":[{"mvforgeLibraryId":7,"jellyfinLibraryId":"different-library","remap":true},{"mvforgeLibraryId":8,"jellyfinLibraryId":"missing-library"}]}}`, server.URL)
	remapped := jellyfinPersistenceRequest(t, router, http.MethodPost, "/api/settings/jellyfin", remapBody)
	loaded, _ = loadJellyfinSettings(db)
	projectedMappings := jellyfinSettingsProjection(loaded)["mappings"].(models.JSONList)
	if remapped.Code != http.StatusOK || loaded.Mappings[0].JellyfinLibraryID != "different-library" || loaded.Mappings[0].JellyfinServerID != "server-new" || projectedMappings[0].(models.JSONMap)["needsRemap"] != false {
		t.Fatalf("explicit remap was not bound to rediscovered server identity: status=%d mapping=%#v", remapped.Code, loaded.Mappings[0])
	}
	retry = jellyfinPersistenceRequest(t, router, http.MethodPost, fmt.Sprintf("/api/jellyfin/outcomes/%d/retry", staleOutcome.ID), "{}")
	if retry.Code != http.StatusAccepted {
		t.Fatalf("explicit remap did not restore retry eligibility: status=%d body=%s", retry.Code, retry.Body.String())
	}
	manual := jellyfinPersistenceRequest(t, router, http.MethodPost, "/api/jellyfin/manual-refresh", `{"libraryId":7}`)
	if manual.Code != http.StatusAccepted {
		t.Fatalf("explicit remap did not restore manual refresh eligibility: status=%d body=%s", manual.Code, manual.Body.String())
	}
}
