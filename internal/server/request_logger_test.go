package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRequestLogger(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, method, path string
		status             int
		slow               bool
		wantLog            bool
	}{
		{"healthy", "GET", "/health", 200, false, false},
		{"ready", "GET", "/ready", 200, false, false},
		{"failed", "GET", "/health", 503, false, true},
		{"slow", "GET", "/health", 200, true, true},
		{"api", "GET", "/api/bible/v1/versions", 200, false, true},
		{"non-probe method", "POST", "/health", 200, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			original := gin.DefaultWriter
			gin.DefaultWriter = &output
			defer func() { gin.DefaultWriter = original }()
			router := gin.New()
			router.Use(RequestLogger(), gin.Recovery())
			router.Handle(tc.method, tc.path, func(c *gin.Context) {
				if tc.slow {
					time.Sleep(1050 * time.Millisecond)
				}
				c.Status(tc.status)
			})
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(tc.method, tc.path, nil))
			if recorder.Code != tc.status {
				t.Fatalf("status = %d", recorder.Code)
			}
			if got := strings.Contains(output.String(), tc.path); got != tc.wantLog {
				t.Fatalf("log = %q, wantLog = %v", output.String(), tc.wantLog)
			}
		})
	}
}

func TestRequestLoggerPreservesRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestLogger(), gin.Recovery())
	router.GET("/health", func(*gin.Context) { panic("probe failure") })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", recorder.Code)
	}
}
