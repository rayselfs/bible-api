package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"strings"
	"time"
)

func diagnosticID(value string, size int) bool {
	if len(value) != size || value != strings.ToLower(value) || value == strings.Repeat("0", size) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
func newDiagnosticID(size int) string {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		panic("diagnostic identity unavailable")
	}
	return hex.EncodeToString(b)
}

// RequestLogger excludes raw paths, query strings, headers, peer data and errors.
// The edge owns trace sampling. These records are independent of that decision.
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		requestID := c.GetHeader("X-HHC-Request-ID")
		if len(c.Request.Header.Values("X-HHC-Request-ID")) != 1 || !diagnosticID(requestID, 32) {
			requestID = newDiagnosticID(16)
		}
		traceID := newDiagnosticID(16)
		parent := c.GetHeader("traceparent")
		if len(c.Request.Header.Values("traceparent")) == 1 && len(parent) == 55 && parent[:3] == "00-" && parent[35] == '-' && parent[52] == '-' && (parent[53:] == "00" || parent[53:] == "01") && diagnosticID(parent[3:35], 32) && diagnosticID(parent[36:52], 16) {
			traceID = parent[3:35]
		}
		c.Header("X-HHC-Request-ID", requestID)
		c.Next()
		elapsed := time.Since(started)
		code := c.Writer.Status()
		probe := c.Request.Method == http.MethodGet && (c.Request.URL.Path == "/health" || c.Request.URL.Path == "/ready")
		if probe && code == 200 && len(c.Errors) == 0 && elapsed < time.Second {
			return
		}
		record := struct {
			Event     string `json:"event"`
			RequestID string `json:"request_id"`
			TraceID   string `json:"trace_id"`
			Status    int    `json:"status"`
			Duration  int64  `json:"duration_ms"`
		}{"http_request", requestID, traceID, code, elapsed.Milliseconds()}
		data, _ := json.Marshal(record)
		_, _ = fmt.Fprintln(gin.DefaultWriter, string(data))
	}
}

// Recovery must not print Gin's raw request dump or panic/provider error text.
func SafeRecovery() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, _ any) { c.AbortWithStatus(http.StatusInternalServerError) })
}
