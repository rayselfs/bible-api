package database

import (
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestMigrateFreshSchemaAndPreserveAppliedHistory(t *testing.T) {
	raw := os.Getenv("BIBLE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("BIBLE_TEST_DATABASE_URL is not configured")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() != "127.0.0.1" || u.Path != "/bible_pool_test" {
		t.Fatal("requires disposable loopback bible_pool_test fixture")
	}
	admin, err := gorm.Open(postgres.Open(raw), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	adminPool, _ := admin.DB()
	defer adminPool.Close()
	if err := admin.Exec("CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public").Error; err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("bible_bootstrap_%d", time.Now().UnixNano())
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	defer admin.Exec("DROP SCHEMA " + schema + " CASCADE")
	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()
	isolated, err := gorm.Open(postgres.Open(u.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := isolated.DB()
	pool.SetMaxOpenConns(2)
	pool.SetMaxIdleConns(1)
	defer pool.Close()
	var tables int
	if err := pool.QueryRowContext(t.Context(), "SELECT count(*) FROM information_schema.tables WHERE table_schema=$1", schema).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Fatal("bootstrap schema is not empty")
	}
	previous := DB
	DB = isolated
	defer func() { DB = previous }()
	Migrate()
	var ids int
	if err := pool.QueryRowContext(t.Context(), "SELECT count(*) FROM migrations").Scan(&ids); err != nil {
		t.Fatal(err)
	}
	if ids != 6 {
		t.Fatalf("migration IDs=%d, want six actual migrations", ids)
	}
	var localeType, vectorType string
	if err := pool.QueryRowContext(t.Context(), "SELECT data_type FROM information_schema.columns WHERE table_schema=$1 AND table_name='versions' AND column_name='locale'", schema).Scan(&localeType); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRowContext(t.Context(), "SELECT format_type(atttypid,atttypmod) FROM pg_attribute WHERE attrelid='bible_vectors'::regclass AND attname='embedding'").Scan(&vectorType); err != nil {
		t.Fatal(err)
	}
	if localeType != "character varying" || vectorType != "vector(384)" {
		t.Fatal("fresh schema contract changed")
	}
	var id int
	var updated time.Time
	if err := pool.QueryRowContext(t.Context(), "INSERT INTO versions(code,name) VALUES('KJV','Bootstrap fixture') RETURNING id,updated_at").Scan(&id, &updated); err != nil {
		t.Fatal(err)
	}
	Migrate()
	var after time.Time
	if err := pool.QueryRowContext(t.Context(), "SELECT updated_at FROM versions WHERE id=$1", id).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if !updated.Equal(after) {
		t.Fatal("replaying applied history changed existing version data")
	}
	if err := pool.QueryRowContext(t.Context(), "SELECT count(*) FROM migrations").Scan(&ids); err != nil {
		t.Fatal(err)
	}
	if ids != 6 {
		t.Fatal("replaying history added migration records")
	}
}
