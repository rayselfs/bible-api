package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"hhc/bible-api/internal/database"
	"hhc/bible-api/internal/testpostgres"
)

// Exercise the successful consumer in-process: process exit would hide a leaked pool.
func TestRunImportClosesEveryBackend(t *testing.T) {
	raw := os.Getenv("BIBLE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("BIBLE_TEST_DATABASE_URL is not configured")
	}
	dsn := testpostgres.LoopbackDSN(t, raw)
	cfg, err := pgx.ParseConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("bible_import_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(t.Context(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.ExecContext(t.Context(), "DROP SCHEMA "+schema+" CASCADE")
	if _, err := admin.ExecContext(t.Context(), "CREATE TABLE "+schema+`.versions(id bigserial PRIMARY KEY,code text UNIQUE,name text,locale text,updated_at timestamptz); CREATE TABLE `+schema+`.vector_update_logs(id bigserial PRIMARY KEY,source text,reference_id text,status text,created_at timestamptz,updated_at timestamptz)`); err != nil {
		t.Fatal(err)
	}
	// Connect builds a keyword DSN; these fields come only from this owned fixture.
	for key, value := range map[string]string{
		"POSTGRES_HOST": cfg.Host, "POSTGRES_PORT": strconv.Itoa(int(cfg.Port)), "POSTGRES_USER": cfg.User, "POSTGRES_PASS": cfg.Password, "POSTGRES_DB": cfg.Database, "POSTGRES_SSLMODE": "disable",
		"PGSERVICE": "", "PGSERVICEFILE": "", "PGOPTIONS": "-c search_path=" + schema, "PGAPPNAME": schema,
		"DB_MAX_OPEN_CONNS": "", "DB_MAX_IDLE_CONNS": "", "DB_CONN_MAX_LIFETIME": "", "DB_CONN_MAX_IDLE_TIME": "",
	} {
		t.Setenv(key, value)
	}
	// Validate the exact keyword DSN consumed by Connect, including env/service resolution.
	testpostgres.LoopbackDSN(t, fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", cfg.Host, strconv.Itoa(int(cfg.Port)), cfg.User, cfg.Password, cfg.Database))
	fixture := filepath.Join(t.TempDir(), "synthetic.json")
	if err := os.WriteFile(fixture, []byte(`{"version":{"code":"POOL-TEST","name":"Synthetic"},"books":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	args := os.Args
	os.Args = []string{"bible-api", "import", "-f", fixture}
	defer func() { os.Args = args; database.Close() }()
	runImport()
	pool, err := database.DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	if pool.Stats().OpenConnections != 0 {
		t.Fatalf("import did not close its pool: %+v", pool.Stats())
	}
	var imported, queued int
	if err := admin.QueryRowContext(t.Context(), "SELECT count(*) FROM "+schema+".versions WHERE code='POOL-TEST'").Scan(&imported); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRowContext(t.Context(), "SELECT count(*) FROM "+schema+".vector_update_logs WHERE reference_id='POOL-TEST' AND status='pending'").Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if imported != 1 || queued != 1 {
		t.Fatalf("import did not commit synthetic version and vector work: %d/%d", imported, queued)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		var backends int
		if err := admin.QueryRowContext(t.Context(), "SELECT count(*) FROM pg_stat_activity WHERE application_name=$1", schema).Scan(&backends); err != nil {
			t.Fatal(err)
		}
		if backends == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("successful import leaked %d PostgreSQL backend(s)", backends)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pool.Stats().MaxOpenConnections != 4 {
		t.Fatalf("actual import pool cap=%d, want 4", pool.Stats().MaxOpenConnections)
	}
}
