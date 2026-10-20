package server

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestReadinessDoesNotWaitForSaturatedPool(t *testing.T) {
	raw := os.Getenv("BIBLE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("BIBLE_TEST_DATABASE_URL is not configured")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() != "127.0.0.1" {
		t.Fatal("requires loopback PG fixture")
	}
	dapr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer dapr.Close()
	ready := NewReadinessHandler(readinessPort(t, dapr.Listener.Addr().String()), "")
	for _, cap := range []int{1, 2, 8} {
		t.Run(strconv.Itoa(cap), func(t *testing.T) {
			db, err := gorm.Open(postgres.Open(raw), &gorm.Config{DisableAutomaticPing: true})
			if err != nil {
				t.Fatal(err)
			}
			pool, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			pool.SetMaxOpenConns(cap)
			pool.SetMaxIdleConns(2)
			var held []*sql.Conn
			defer func() {
				for _, connection := range held {
					connection.Close()
				}
			}()
			for i := 0; i < cap; i++ {
				connection, err := pool.Conn(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				held = append(held, connection)
			}
			before := pool.Stats()
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			c.Request = httptest.NewRequest("GET", "/ready", nil)
			ready(c)
			after := pool.Stats()
			if response.Code != 200 || after.InUse != cap || after.OpenConnections != cap || after.WaitCount != before.WaitCount {
				t.Fatalf("readiness acquired or waited for PG: before=%+v after=%+v status=%d", before, after, response.Code)
			}
		})
	}
}
