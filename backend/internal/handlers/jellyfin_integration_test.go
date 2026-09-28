package handlers

import (
	"context"
	"encoding/json"
	"fmt"
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
		if got := r.Header.Get("X-Emby-Token"); r.URL.Path != "/System/Info/Public" && got != "test-secret" {
			t.Errorf("authorization header=%q", got)
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
	result := sharedJellyfinClient.testConnection(context.Background(), settings)
	if !result.Reachable || !result.Authenticated || result.ServerVersion != "10.9.1" || len(result.Libraries) != 1 || result.Libraries[0].ID != "Library-Exact/42" {
		t.Fatalf("connection result=%#v", result)
	}
	coordinator := JellyfinCoordinator{}
	failure := coordinator.refreshBatch(context.Background(), settings, []models.JellyfinIntegrationOutcome{{MappedLibraryID: "Library-Exact/42"}})
	if failure != nil || scanRequests != 1 || itemRequests != 0 {
		t.Fatalf("failure=%v server scans=%d item refreshes=%d", failure, scanRequests, itemRequests)
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
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("first Library artifact was not removed, stat err=%v", err)
	}
	var persisted models.TestEncode
	if err := db.First(&persisted, testEncode.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Status != testEncodeReady || persisted.DeletedAt != nil {
		t.Fatalf("partial cleanup changed Test Encode lifecycle: status=%q deletedAt=%v", persisted.Status, persisted.DeletedAt)
	}
	var outcome models.JellyfinIntegrationOutcome
	if err := db.First(&outcome, "source_key = ?", testEncodeJellyfinSourceKey(jellyfinReasonTestRemoved, testEncode.ID)).Error; err != nil {
		t.Fatalf("partial cleanup did not record separate Jellyfin reconciliation: %v", err)
	}
	if outcome.Reason != jellyfinReasonTestRemoved || outcome.Status != jellyfinStatusPending || outcome.LibraryID != testEncode.LibraryID {
		t.Fatalf("partial cleanup outcome=%#v", outcome)
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
