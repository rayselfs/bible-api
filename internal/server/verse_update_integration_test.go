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
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func verseEditDB(t *testing.T) *gorm.DB {
	t.Helper()
	raw := os.Getenv("BIBLE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("BIBLE_TEST_DATABASE_URL not configured")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() != "127.0.0.1" || u.Path != "/bible_pool_test" {
		t.Fatal("requires disposable loopback bible_pool_test")
	}
	admin, err := gorm.Open(postgres.Open(raw), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := admin.DB()
	t.Cleanup(func() { pool.Close() })
	schema := fmt.Sprintf("verse_edit_%d", time.Now().UnixNano())
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec("DROP SCHEMA " + schema + " CASCADE") })
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := gorm.Open(postgres.Open(u.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := db.DB()
	t.Cleanup(func() { p.Close() })
	if err := db.Exec(`CREATE TABLE versions(id bigint PRIMARY KEY,updated_at timestamptz NOT NULL);
CREATE TABLE books(id bigint PRIMARY KEY,version_id bigint);
CREATE TABLE chapters(id bigint PRIMARY KEY,book_id bigint);
CREATE TABLE verses(id bigint PRIMARY KEY,chapter_id bigint,text text NOT NULL);
CREATE TABLE vector_update_logs(id bigserial PRIMARY KEY,source varchar(50),reference_id varchar(100),status varchar(20),created_at timestamptz DEFAULT now(),updated_at timestamptz DEFAULT now());
INSERT INTO versions VALUES(1,'2000-01-01Z'); INSERT INTO books VALUES(1,1);
INSERT INTO chapters VALUES(1,1); INSERT INTO verses VALUES(1,1,'Original');
CREATE FUNCTION reject_edit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'sensitive synthetic database error'; END $$;`).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func editVerse(db *gorm.DB, ctx context.Context, id, permission string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: id}}
	c.Set("permissions", permission)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/bible/v1/verse/"+id, strings.NewReader(`{"text":"Changed"}`)).WithContext(ctx)
	c.Request.Header.Set("Content-Type", "application/json")
	NewAPI(models.NewStore(db)).HandleUpdateVerse(c)
	return w
}

func TestVerseEditAtomicFailure(t *testing.T) {
	for _, tc := range []struct {
		name, fault, id, permission string
		status                      int
	}{
		{"missing", "", "999", "bible:edit", 404},
		{"invalid ID", "", "-1", "bible:edit", 400},
		{"permission", "", "1", "bible:read", 403},
		{"text", "CREATE TRIGGER fail BEFORE UPDATE ON verses FOR EACH ROW EXECUTE FUNCTION reject_edit()", "1", "bible:edit", 500},
		{"parent lookup", "DROP TABLE books", "1", "bible:edit", 500},
		{"parent absent", "DELETE FROM books", "1", "bible:edit", 500},
		{"version absent", "DELETE FROM versions", "1", "bible:edit", 500},
		{"version", "CREATE TRIGGER fail BEFORE UPDATE ON versions FOR EACH ROW EXECUTE FUNCTION reject_edit()", "1", "bible:edit", 500},
		{"work", "CREATE TRIGGER fail BEFORE INSERT ON vector_update_logs FOR EACH ROW EXECUTE FUNCTION reject_edit()", "1", "bible:edit", 500},
		{"cancelled", "", "1", "bible:edit", 500},
		{"cancel during work", "CREATE FUNCTION wait_edit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(2); RETURN NEW; END $$; CREATE TRIGGER wait BEFORE INSERT ON vector_update_logs FOR EACH ROW EXECUTE FUNCTION wait_edit()", "1", "bible:edit", 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := verseEditDB(t)
			if tc.fault != "" {
				if err := db.Exec(tc.fault).Error; err != nil {
					t.Fatal(err)
				}
			}
			ctx := t.Context()
			if tc.name == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if tc.name == "cancel during work" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
			}
			w := editVerse(db, ctx, tc.id, tc.permission)
			if w.Code != tc.status || strings.Contains(w.Body.String(), "sensitive") {
				t.Errorf("status=%d want=%d body=%s", w.Code, tc.status, w.Body)
			}
			var text string
			var stamp time.Time
			var count int64
			if err := db.Raw("SELECT text FROM verses WHERE id=1").Scan(&text).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Raw("SELECT updated_at FROM versions WHERE id=1").Scan(&stamp).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Table("vector_update_logs").Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if text != "Original" || (tc.name != "version absent" && stamp.Year() != 2000) || count != 0 {
				t.Fatalf("partial edit text=%q stamp=%s work=%d", text, stamp, count)
			}
		})
	}
}

func TestVerseEditConcurrentSuccess(t *testing.T) {
	db := verseEditDB(t)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := editVerse(db, t.Context(), "1", "bible:edit")
			if w.Code != 200 || w.Body.String() != `{"id":1,"message":"Verse updated successfully"}` {
				t.Errorf("response=%d %s", w.Code, w.Body)
			}
		}()
	}
	wg.Wait()
	var text string
	var stamp time.Time
	var count int64
	db.Raw("SELECT text FROM verses WHERE id=1").Scan(&text)
	db.Raw("SELECT updated_at FROM versions WHERE id=1").Scan(&stamp)
	db.Table("vector_update_logs").Where("source='api_update_verse' AND reference_id='1' AND status='pending'").Count(&count)
	if text != "Changed" || stamp.Year() == 2000 || count != 2 {
		t.Fatalf("text=%q stamp=%s work=%d", text, stamp, count)
	}
}
