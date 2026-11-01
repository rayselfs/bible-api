package server

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GatewayAuth authenticates the caller before trusting its permission headers.
func GatewayAuth(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/health" || c.Request.URL.Path == "/ready" {
			c.Next()
			return
		}
		values := c.Request.Header.Values("X-Bible-Gateway-Token")
		if token == "" || len(values) != 1 || subtle.ConstantTimeCompare([]byte(values[0]), []byte(token)) != 1 {
			c.Header("Cache-Control", "no-store")
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
}

// ProcessReadiness is registered only after database startup and migrations complete.
// @Summary Process readiness
// @Description Ready after startup and migrations complete, without polling dependencies.
// @Tags Operations
// @Produce json
// @Success 200 {object} map[string]string
// @Router /ready [get]
func ProcessReadiness(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"status": "UP"})
}
