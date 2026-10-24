package server

import (
	"fmt"
	"hhc/bible-api/internal/models"
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

func TestPermissionDependentResponsesAreNotStored(t *testing.T) {
	raw := os.Getenv("BIBLE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("BIBLE_TEST_DATABASE_URL is not configured")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() != "127.0.0.1" || u.Path != "/bible_pool_test" {
		t.Fatal("requires the disposable loopback bible_pool_test fixture")
	}
	admin, err := gorm.Open(postgres.Open(raw), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	adminSQL, _ := admin.DB()
	defer adminSQL.Close()
	schema := fmt.Sprintf("bible_cache_%d", time.Now().UnixNano())
	if err := admin.Exec("CREATE EXTENSION IF NOT EXISTS vector; CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	defer admin.Exec("DROP SCHEMA " + schema + " CASCADE")
	query := u.Query()
	query.Set("search_path", schema+",public")
	u.RawQuery = query.Encode()
	db, err := gorm.Open(postgres.Open(u.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	defer pool.Close()
	fixture := `CREATE TABLE versions(id bigint PRIMARY KEY,code text,name text,locale text,updated_at timestamptz);
CREATE TABLE books(id bigint PRIMARY KEY,version_id bigint,number integer,name text,abbreviation text);
CREATE TABLE chapters(id bigint PRIMARY KEY,book_id bigint,number integer);
CREATE TABLE verses(id bigint PRIMARY KEY,chapter_id bigint,number integer,text text);
CREATE TABLE bible_vectors(verse_id bigint,embedding vector(384));
INSERT INTO versions VALUES(1,'KJV','Public fixture','en',now()),(2,'SYNTHETIC-PRIVATE','Private fixture','en',now());
INSERT INTO books VALUES(1,1,1,'Fixture','F'),(2,2,1,'Fixture','F');
INSERT INTO chapters VALUES(1,1,1),(2,2,1);
INSERT INTO verses VALUES(1,1,1,'Synthetic public'),(2,2,1,'Synthetic private');
INSERT INTO bible_vectors SELECT n,array_fill(0::real,ARRAY[384])::vector FROM generate_series(1,2) n;`
	for _, statement := range strings.Split(fixture, ";") {
		if strings.TrimSpace(statement) != "" {
			if err := db.Exec(statement).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewAPI(models.NewStore(db)).SetupRoutes(router)
	for _, permissions := range []string{"", "bible:read"} {
		for _, path := range []string{"/api/bible/v1/versions", "/api/bible/v2/versions", "/api/bible/v1/content/1", "/api/bible/v1/content/2", "/api/bible/v1/vectors/1", "/api/bible/v1/vectors/2"} {
			t.Run(path+"/"+permissions, func(t *testing.T) {
				response := httptest.NewRecorder()
				request := httptest.NewRequest("GET", path, nil)
				request.Header.Set("X-Permissions", permissions)
				router.ServeHTTP(response, request)
				if got := response.Header().Get("Cache-Control"); got != "private, no-store" {
					t.Errorf("Cache-Control = %q", got)
				}
				if strings.Contains(path, "/vectors/") {
					want := 4 + 384*4
					if strings.HasSuffix(path, "/2") && permissions == "" {
						want = 0
					}
					if response.Body.Len() != want {
						t.Errorf("vector bytes = %d, want %d", response.Body.Len(), want)
					}
				}
				if strings.HasSuffix(path, "/versions") && strings.Contains(response.Body.String(), "SYNTHETIC-PRIVATE") != (permissions != "") {
					t.Error("version filtering changed")
				}
			})
		}
	}
}
