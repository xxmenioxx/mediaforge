package routes

import "github.com/gin-gonic/gin"

// stripInboundCookies keeps MVForge independent from host-scoped cookies that
// belong to another application sharing the same hostname.
func stripInboundCookies() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request != nil {
			c.Request.Header.Del("Cookie")
		}
		c.Next()
	}
}
