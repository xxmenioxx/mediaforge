package routes

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anuelvs/mvforge/backend/internal/applog"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/gin-gonic/gin"
)

func TestProductionMiddlewareAddsRequestIDAndRecoversPanics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(productionLogger(), productionRecovery())
	router.GET("/panic", func(c *gin.Context) { panic("test panic") })

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/panic", nil)
	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d want=%d", response.Code, http.StatusInternalServerError)
	}
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing X-Request-ID response header")
	}
}

func TestSafeDeleteRouteIsRegistered(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:routes-safe-delete?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	router := New(db)
	for _, route := range router.Routes() {
		if route.Method == http.MethodPost && route.Path == "/api/assets/delete-converted" {
			return
		}
	}
	t.Fatal("POST /api/assets/delete-converted is not registered")
}

func TestLibraryRenameRoutesAreRegistered(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:routes-library-rename-preview?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	router := New(db)
	found := map[string]bool{}
	for _, route := range router.Routes() {
		if route.Method == http.MethodPost {
			found[route.Path] = true
		}
	}
	for _, path := range []string{"/api/assets/library-rename/preview", "/api/assets/library-rename/apply"} {
		if !found[path] {
			t.Fatalf("POST %s is not registered", path)
		}
	}
}

func TestProductionMiddlewarePreservesIncomingRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(productionLogger(), productionRecovery())
	router.GET("/ok", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ok", nil)
	request.Header.Set("X-Request-ID", "known-request")
	router.ServeHTTP(response, request)

	if got := response.Header().Get("X-Request-ID"); got != "known-request" {
		t.Fatalf("request ID=%q", got)
	}
}

func TestProductionLoggingDoesNotIncludeSensitiveHeaders(t *testing.T) {
	logDir := t.TempDir()
	if err := applog.Initialize(logDir); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(productionLogger(), productionRecovery())
	router.GET("/ok", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.GET("/panic", func(c *gin.Context) { panic("test panic") })

	for _, path := range []string{"/ok", "/panic"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Cookie", "mvforge-cookie-secret")
		request.Header.Set("Authorization", "Bearer mvforge-authorization-secret")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
	}

	contents, err := os.ReadFile(filepath.Join(logDir, "backend.log"))
	if err != nil {
		t.Fatal(err)
	}
	logText := string(contents)
	for _, forbidden := range []string{"mvforge-cookie-secret", "mvforge-authorization-secret", `"Cookie"`, `"Authorization"`} {
		if strings.Contains(logText, forbidden) {
			t.Fatalf("production log contains sensitive request header data %q: %s", forbidden, logText)
		}
	}
}

func TestCORSRemainsNonCredentialed(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:routes-cors-non-credentialed?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	router := New(db)

	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("allow-origin=%q", got)
	}
	if got := response.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("allow-credentials=%q, want it absent", got)
	}
}
