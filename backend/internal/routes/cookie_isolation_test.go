package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestStripInboundCookiesPreservesUnrelatedHeadersAndRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(stripInboundCookies())
	router.GET("/preview", func(c *gin.Context) {
		if got := c.GetHeader("Cookie"); got != "" {
			t.Errorf("cookie header=%q, want it removed", got)
		}
		if len(c.Request.Cookies()) != 0 {
			t.Errorf("request cookies=%v, want none", c.Request.Cookies())
		}
		if got := c.GetHeader("X-Test-Header"); got != "available" {
			t.Errorf("unrelated header=%q", got)
		}
		if got := c.GetHeader("Range"); got != "bytes=0-99" {
			t.Errorf("range header=%q", got)
		}
		c.Status(http.StatusPartialContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/preview", nil)
	request.Header.Set("Cookie", "other-app=session-secret")
	request.Header.Set("X-Test-Header", "available")
	request.Header.Set("Range", "bytes=0-99")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusPartialContent {
		t.Fatalf("status=%d, want=%d", response.Code, http.StatusPartialContent)
	}
}
