package models

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

func vectorTestStore(t *testing.T) (*Store, *gin.Context) {
	t.Helper()
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
	t.Cleanup(func() { adminSQL.Close() })
	schema := fmt.Sprintf("bible_pool_%d", time.Now().UnixNano())
	if err := admin.Exec(`CREATE EXTENSION IF NOT EXISTS vector; CREATE SCHEMA ` + schema).Error; err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()
	db, err := gorm.Open(postgres.Open(u.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(2)
	sqlDB.SetMaxIdleConns(2)
	t.Cleanup(func() { sqlDB.Close(); admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`) })
	fixture := `CREATE TABLE versions(id bigint PRIMARY KEY,code text,name text,locale text,updated_at timestamptz);
 CREATE TABLE books(id bigint PRIMARY KEY,version_id bigint,number integer,name text,abbreviation text);
 CREATE TABLE chapters(id bigint PRIMARY KEY,book_id bigint,number integer);
 CREATE TABLE verses(id bigint PRIMARY KEY,chapter_id bigint,number integer,text text);
 CREATE TABLE bible_vectors(verse_id bigint,embedding vector(384));
 INSERT INTO versions VALUES(1,'KJV','Fixture','en',now());
 INSERT INTO books VALUES(1,1,1,'Fixture','F'); INSERT INTO chapters VALUES(1,1,1);
 INSERT INTO verses SELECT n,1,n,'Fixture' FROM generate_series(1,5201) n;
 INSERT INTO bible_vectors SELECT n, array_fill(0::real,ARRAY[384])::vector FROM generate_series(1,5201) n;`
	setupCtx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	for _, statement := range strings.Split(fixture, ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if err := db.WithContext(setupCtx).Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	return NewStore(db), c
}

func waitVectorCondition(t *testing.T, condition func() bool, failure string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal(failure)
}

func TestVectorStreamCancellationReleasesPool(t *testing.T) {
	store, c := vectorTestStore(t)
	db, _ := store.DB.DB()
	for attempt := 0; attempt < 3; attempt++ {
		ctx, cancel := context.WithCancel(t.Context())
		content, errors := store.StreamVectorsForVersion(c, ctx, 1)
		// Cleanup drains only after the assertion, so the old blocking send cannot pass by being rescued.
		t.Cleanup(func() {
			cancel()
			for range content {
			}
			for range errors {
			}
		})
		waitVectorCondition(t, func() bool { return len(content) == cap(content) && db.Stats().InUse == 1 && vectorProducerWaiting() }, "stream never filled its buffer while holding a connection")
		cancel()
		select {
		case err, ok := <-errors:
			if ok {
				t.Fatalf("stream producer returned an unexpected error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("canceled producer is still blocked on channel send")
		}
		waitVectorCondition(t, func() bool { return db.Stats().InUse == 0 }, "canceled stream retained its PostgreSQL connection")
		for range content {
		}
		for err := range errors {
			if err != nil && !strings.Contains(err.Error(), "context canceled") {
				t.Fatal(err)
			}
		}
	}
}

func TestVectorStreamWireFormatAndVersionPermission(t *testing.T) {
	store, c := vectorTestStore(t)
	content, errors := store.StreamVectorsForVersion(c, t.Context(), 1)
	bytes, verses := 0, 0
	for batch := range content {
		if len(batch)%1540 != 0 {
			t.Fatal("invalid vector frame size")
		}
		for offset := 0; offset < len(batch); offset += 1540 {
			verses++
			if got := binary.LittleEndian.Uint32(batch[offset:]); got != uint32(verses) {
				t.Fatal("vector ordering changed")
			}
			for _, b := range batch[offset+4 : offset+1540] {
				if b != 0 {
					t.Fatal("vector encoding changed")
				}
			}
		}
		bytes += len(batch)
	}
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if verses != 5201 || bytes != 5201*1540 {
		t.Fatal("incomplete vector stream")
	}
	if err := store.DB.Exec("UPDATE versions SET code='PRIVATE' WHERE id=1").Error; err != nil {
		t.Fatal(err)
	}
	content, errors = store.StreamVectorsForVersion(c, t.Context(), 1)
	if _, ok := <-content; ok {
		t.Fatal("private version bytes exposed")
	}
	if err := <-errors; err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatal("version permission was bypassed")
	}
}

func TestVectorStreamCancellationWhileWaitingForPool(t *testing.T) {
	store, c := vectorTestStore(t)
	db, _ := store.DB.DB()
	first, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	second, err := db.Conn(t.Context())
	if err != nil {
		first.Close()
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	content, errors := store.StreamVectorsForVersion(c, ctx, 1)
	t.Cleanup(func() {
		cancel()
		first.Close()
		second.Close()
		for range content {
		}
		for range errors {
		}
	})
	waitVectorCondition(t, func() bool { return db.Stats().WaitCount > 0 }, "stream did not wait for the saturated pool")
	cancel()
	select {
	case err := <-errors:
		if err == nil || !strings.Contains(err.Error(), "context canceled") {
			t.Fatal("missing cancellation error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream query ignored context while waiting for pool")
	}
}
func TestVectorStreamTailCancellation(t *testing.T) {
	store, c := vectorTestStore(t)
	db, _ := store.DB.DB()
	if err := store.DB.Exec("DELETE FROM bible_vectors WHERE verse_id>5001").Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	content, errors := store.StreamVectorsForVersion(c, ctx, 1)
	t.Cleanup(func() {
		cancel()
		for range content {
		}
		for range errors {
		}
	})
	// EOF returns the DB connection before the remaining one-verse batch is sent.
	waitVectorCondition(t, func() bool { return len(content) == cap(content) && db.Stats().InUse == 0 }, "tail batch never reached its full channel")
	cancel()
	select {
	case err, ok := <-errors:
		if ok {
			t.Fatalf("unexpected producer error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("tail send ignored cancellation")
	}
}

func TestVersionListPoolWaitCancellation(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		t.Run(version, func(t *testing.T) {
			store, c := vectorTestStore(t)
			db, _ := store.DB.DB()
			before := db.Stats().WaitCount
			first, err := db.Conn(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			second, err := db.Conn(t.Context())
			if err != nil {
				first.Close()
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			c.Request = c.Request.WithContext(ctx)
			done := make(chan error, 1)
			go func() {
				if version == "v1" {
					_, err := store.GetAllVersions(c)
					done <- err
				} else {
					_, err := store.GetAllVersionsV2(c)
					done <- err
				}
			}()
			t.Cleanup(func() { cancel(); first.Close(); second.Close() })
			waitVectorCondition(t, func() bool { return db.Stats().WaitCount > before }, "versions request did not wait for pool")
			cancel()
			select {
			case err := <-done:
				if err == nil || !strings.Contains(err.Error(), "context canceled") {
					t.Fatal("version query lost cancellation")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("version query ignored canceled request")
			}
		})
	}
}
func TestBibleContentStreamCancellationAndFormat(t *testing.T) {
	store, c := vectorTestStore(t)
	db, _ := store.DB.DB()
	content, errors := store.StreamBibleContent(c, t.Context(), 1)
	header := <-content
	var decoded map[string]any
	if err := json.Unmarshal(header, &decoded); err != nil || decoded["version_code"] != "KJV" {
		t.Fatal("SSE header changed")
	}
	books := 0
	for batch := range content {
		var book BibleContentBook
		if err := json.Unmarshal(batch, &book); err != nil || len(book.Chapters) != 1 || len(book.Chapters[0].Verses) != 5201 {
			t.Fatal("SSE book payload changed")
		}
		books++
	}
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if books != 1 {
		t.Fatal("incomplete SSE stream")
	}
	if err := store.DB.Exec("INSERT INTO books SELECT n,1,n,'Fixture','F' FROM generate_series(2,20) n").Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	content, errors = store.StreamBibleContent(c, ctx, 1)
	t.Cleanup(func() {
		cancel()
		for range content {
		}
		for range errors {
		}
	})
	waitVectorCondition(t, func() bool { return len(content) == cap(content) && db.Stats().InUse == 0 }, "SSE buffer did not fill")
	cancel()
	select {
	case err, ok := <-errors:
		if ok && err != context.Canceled {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SSE producer ignored cancellation")
	}
	// Producers must finish before a slow or disconnected consumer drains the channel.
	select {
	case _, ok := <-errors:
		if ok {
			t.Fatal("multiple SSE errors")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SSE producer did not exit")
	}
}

// Observe the producer's blocked state so cancel cannot win at the preceding row check.
// Both the original channel send and the fixed context-aware select are valid preconditions.
func vectorProducerWaiting() bool {
	stack := make([]byte, 1<<20)
	n := runtime.Stack(stack, true)
	for _, goroutine := range strings.Split(string(stack[:n]), "\n\n") {
		lines := strings.Split(goroutine, "\n")
		if len(lines) > 1 && (strings.Contains(lines[0], "[chan send]") || strings.Contains(lines[0], "[select]")) && strings.Contains(lines[1], "(*Store).StreamVectorsForVersion.func1()") {
			return true
		}
	}
	return false
}
