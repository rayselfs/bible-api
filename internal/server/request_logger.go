package server

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestLogger keeps failed and slow probes observable while omitting routine successes.
func RequestLogger() gin.HandlerFunc {
	const startedKey = "bible.request-log-started"
	logger := gin.LoggerWithConfig(gin.LoggerConfig{Skip: func(c *gin.Context) bool {
		started, _ := c.Get(startedKey)
		return c.Request.Method == http.MethodGet && (c.Request.URL.Path == "/health" || c.Request.URL.Path == "/ready") &&
			c.Writer.Status() == http.StatusOK && len(c.Errors) == 0 && time.Since(started.(time.Time)) < time.Second
	}})
	return func(c *gin.Context) {
		c.Set(startedKey, time.Now())
		logger(c)
	}
}
