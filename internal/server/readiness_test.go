package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func readinessPort(t *testing.T, address string) int {
	t.Helper()
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestReadinessDaprHealth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, status := range []int{204, 200, 302, 401, 403, 500, 503} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			var calls atomic.Int32
			dapr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/v1.0/healthz/outbound" || r.Header.Get("dapr-api-token") != "fixture-token" {
					t.Error("wrong Dapr health request")
				}
				w.Header().Set("Location", "/unexpected-redirect")
				w.WriteHeader(status)
			}))
			defer dapr.Close()
			handler := NewReadinessHandler(readinessPort(t, dapr.Listener.Addr().String()), "fixture-token")
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest("GET", "/ready", nil)
			handler(c)
			wantCode, wantBody := 503, `{"status":"DOWN"}`
			if status == 204 {
				wantCode, wantBody = 200, `{"status":"UP"}`
			}
			if recorder.Code != wantCode || recorder.Body.String() != wantBody || recorder.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("readiness = %d %s, want %d %s", recorder.Code, recorder.Body.String(), wantCode, wantBody)
			}
			if calls.Load() != 1 {
				t.Fatal("health request retried or followed redirect")
			}
		})
	}
}

func TestReadinessTimeoutCancellationAndRecovery(t *testing.T) {
	var healthy atomic.Bool
	dapr := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if healthy.Load() {
			w.WriteHeader(204)
			return
		}
		<-r.Context().Done()
	}))
	port := readinessPort(t, dapr.Listener.Addr().String())
	handler := NewReadinessHandler(port, "")
	probe := func(ctx context.Context, want int, max time.Duration) {
		t.Helper()
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest("GET", "/ready", nil).WithContext(ctx)
		started := time.Now()
		handler(c)
		if recorder.Code != want || time.Since(started) > max {
			t.Fatalf("probe = %d in %s", recorder.Code, time.Since(started))
		}
	}
	// The app can serve before the sidecar starts and recover using the same handler.
	dapr.Listener.Close()
	probe(t.Context(), 503, time.Second)
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	dapr.Listener = listener
	dapr.Start()
	defer dapr.Close()
	probe(t.Context(), 503, time.Second)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	probe(ctx, 503, 200*time.Millisecond)
	healthy.Store(true)
	probe(t.Context(), 200, time.Second)
	dapr.Close()
	probe(t.Context(), 503, time.Second)
}
