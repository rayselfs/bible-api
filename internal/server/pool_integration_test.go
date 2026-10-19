package server

import (
	"context"
	"fmt"
	"hhc/bible-api/internal/models"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestUpdateVersePoolWaitCancellation(t *testing.T) {
	raw := os.Getenv("BIBLE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("BIBLE_TEST_DATABASE_URL is not configured")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() != "127.0.0.1" {
		t.Fatal("integration tests require a loopback PostgreSQL fixture")
	}
	admin, err := gorm.Open(postgres.Open(raw), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	adminSQL, _ := admin.DB()
	defer adminSQL.Close()
	schema := fmt.Sprintf("bible_edit_pool_%d", time.Now().UnixNano())
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	defer admin.Exec("DROP SCHEMA " + schema + " CASCADE")
	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()
	db, err := gorm.Open(postgres.Open(u.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	defer pool.Close()
	pool.SetMaxOpenConns(1)
	if err := db.Exec("CREATE TABLE verses(id bigint PRIMARY KEY, chapter_id bigint, number integer, text text)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO verses VALUES(1,1,1,'Original')").Error; err != nil {
		t.Fatal(err)
	}
	held, err := pool.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	c.Set("permissions", "bible:edit")
	c.Request = httptest.NewRequest(http.MethodPost, "/api/bible/v1/verse/1", strings.NewReader(`{"text":"Changed"}`)).WithContext(ctx)
	c.Request.Header.Set("Content-Type", "application/json")
	done := make(chan struct{})
	// Release the held connection before joining even when the regression fails.
	defer func() { cancel(); held.Close(); <-done }()
	go func() { NewAPI(models.NewStore(db)).HandleUpdateVerse(c); close(done) }()
	deadline := time.Now().Add(2 * time.Second)
	for pool.Stats().WaitCount == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if pool.Stats().WaitCount == 0 {
		t.Fatal("handler did not wait for the saturated pool")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("canceled edit kept waiting for a connection")
	}
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected canceled-query response: %d", response.Code)
	}
	held.Close()
	var text string
	if err := db.Raw("SELECT text FROM verses WHERE id=1").Scan(&text).Error; err != nil {
		t.Fatal(err)
	}
	if text != "Original" {
		t.Fatal("canceled edit wrote after acquiring a connection")
	}
	if pool.Stats().InUse != 0 {
		t.Fatal("canceled edit retained a connection")
	}
}
