package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

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
