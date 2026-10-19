package server

import (
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// The shell fixture controls a real pinned Dapr container in its own network namespace.
func TestReadinessRealDapr(t *testing.T) {
	raw := os.Getenv("BIBLE_TEST_DAPR_HTTP_PORT")
	if raw == "" {
		t.Skip("isolated Dapr fixture not configured")
	}
	port, err := strconv.Atoi(raw)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := strconv.Atoi(os.Getenv("BIBLE_TEST_DAPR_EXPECTED_STATUS"))
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	NewAPI(nil).SetupRoutes(router)
	router.GET("/ready", NewReadinessHandler(port, ""))
	for path, want := range map[string]int{"/health": 200, "/ready": expected} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != want {
			t.Fatalf("%s = %d, want %d", path, response.Code, want)
		}
	}
}

func TestReadinessConflictNeverReady(t *testing.T) {
	stop := os.Getenv("BIBLE_DAPR_CONFLICT_MONITOR")
	if stop == "" {
		t.Skip("conflict monitor not configured")
	}
	port, err := strconv.Atoi(os.Getenv("BIBLE_TEST_DAPR_HTTP_PORT"))
	if err != nil {
		t.Fatal(err)
	}
	handler := NewReadinessHandler(port, "")
	if err := os.WriteFile(stop+".started", nil, 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(stop); err == nil {
			return
		}
		response := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(response)
		c.Request = httptest.NewRequest("GET", "/ready", nil)
		handler(c)
		if response.Code != 503 {
			t.Fatal("readiness succeeded while internal gRPC port was occupied")
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("conflict monitor did not receive bounded stop")
}
