package server

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGatewayAuth(t *testing.T) {
	token := strings.Repeat("a", 64)
	for _, tc := range []struct {
		name, token, path string
		headers           []string
		want              int
	}{
		{"missing", token, "/api/bible/v2/versions", nil, 401},
		{"forged", token, "/api/bible/v2/versions", []string{strings.Repeat("b", 64)}, 401},
		{"duplicate", token, "/api/bible/v2/versions", []string{token, token}, 401},
		{"unconfigured", "", "/api/bible/v2/versions", []string{token}, 401},
		{"trusted", token, "/api/bible/v2/versions", []string{token}, 200},
		{"probe", token, "/ready", nil, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.Use(GatewayAuth(tc.token))
			r.GET(tc.path, ProcessReadiness)
			request := httptest.NewRequest("GET", tc.path, nil)
			request.Header.Set("X-Permissions", "bible:read")
			request.Header.Set("Dapr-Caller-App-Id", "api-gateway")
			for _, value := range tc.headers {
				request.Header.Add("X-Bible-Gateway-Token", value)
			}
			response := httptest.NewRecorder()
			r.ServeHTTP(response, request)
			if response.Code != tc.want || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("status=%d, want=%d", response.Code, tc.want)
			}
		})
	}
}

func TestDaprDiscoveryRemainsOptionalDuringCutover(t *testing.T) {
	r := gin.New()
	r.Use(GatewayAuth(strings.Repeat("a", 64)))
	for _, path := range []string{"/dapr/config", "/dapr/subscribe"} {
		response := httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != 404 {
			t.Fatalf("optional discovery %s returned %d, want 404", path, response.Code)
		}
	}
}
