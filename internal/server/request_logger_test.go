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
			router.Use(RequestLogger(), SafeRecovery())
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
			if got := strings.Contains(output.String(), `"event":"http_request"`); got != tc.wantLog {
				t.Fatalf("log = %q, wantLog = %v", output.String(), tc.wantLog)
			}
		})
	}
}

func TestRequestLoggerPreservesRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestLogger(), SafeRecovery())
	router.GET("/health", func(*gin.Context) { panic("probe failure") })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestLoggerPrivacyAndCorrelation(t *testing.T) {
	var output bytes.Buffer
	old := gin.DefaultWriter
	gin.DefaultWriter = &output
	defer func() { gin.DefaultWriter = old }()
	router := gin.New()
	router.Use(RequestLogger(), SafeRecovery())
	router.GET("/private/:secret", func(c *gin.Context) { panic("secret-provider-payload") })
	req := httptest.NewRequest("GET", "/private/member@example.test?token=secret-query", nil)
	req.Header.Set("traceparent", "00-11111111111111111111111111111111-2222222222222222-00")
	req.Header.Set("X-HHC-Request-ID", "33333333333333333333333333333333")
	req.Header.Set("X-Private-Token", "secret-header")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	got := output.String()
	if response.Code != 500 || !strings.Contains(got, `"status":500`) || !strings.Contains(got, "11111111111111111111111111111111") || response.Header().Get("X-HHC-Request-ID") != "33333333333333333333333333333333" {
		t.Fatalf("missing safe correlation: %s", got)
	}
	for _, value := range []string{"member@example.test", "secret-query", "secret-header", "secret-provider-payload"} {
		if strings.Contains(got, value) {
			t.Fatal("private request leaked")
		}
	}
}
