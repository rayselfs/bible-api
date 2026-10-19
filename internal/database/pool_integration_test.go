package database

import (
	"context"
	"database/sql"
	"errors"
	"github.com/jackc/pgx/v5"
	"hhc/bible-api/configs"
	"hhc/bible-api/internal/testpostgres"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestPoolCapAndCancellation(t *testing.T) {
	for _, mode := range []string{"api", "import"} {
		t.Run(mode, func(t *testing.T) { testPoolCapAndCancellation(t, mode) })
	}
}

func testPoolCapAndCancellation(t *testing.T, mode string) {
	raw := os.Getenv("BIBLE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("BIBLE_TEST_DATABASE_URL is not configured")
	}
	testpostgres.LoopbackDSN(t, raw)
	parsed, err := pgx.ParseConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"DB_MAX_OPEN_CONNS", "DB_MAX_IDLE_CONNS", "DB_CONN_MAX_LIFETIME", "DB_CONN_MAX_IDLE_TIME"} {
		t.Setenv(key, "")
	}
	var cfg *configs.Env
	if mode == "import" {
		cfg, err = configs.InitImportConfig()
	} else {
		cfg, err = configs.InitConfig()
	}
	if err != nil {
		t.Fatal(err)
	}
	cfg.PostgresHost = parsed.Host
	cfg.PostgresPort = strconv.Itoa(int(parsed.Port))
	cfg.PostgresUser = parsed.User
	cfg.PostgresPass = parsed.Password
	cfg.PostgresDB = parsed.Database
	cfg.PostgresSSLMode = "disable"
	cfg.DBConnMaxLifetime = 80 * time.Millisecond
	// Validate the same assembled DSN Connect consumes, not just the URL authority.
	testpostgres.LoopbackDSN(t, buildDSN(cfg))
	cap := 8
	if mode == "import" {
		cap = 4
	}
	Connect(cfg)
	t.Cleanup(Close)
	db, err := DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	if db.Stats().MaxOpenConnections != cap {
		t.Fatal("runtime pool has no configured limit")
	}
	held := make([]*sql.Conn, 0, cap)
	defer func() {
		for _, conn := range held {
			conn.Close()
		}
	}()
	for range cap {
		conn, err := db.Conn(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, conn)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	if err := db.PingContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("pool waiter did not honor cancellation: %v", err)
	}
	if db.Stats().OpenConnections > cap || db.Stats().WaitCount == 0 {
		t.Fatal("pool limit was bypassed")
	}
	for _, conn := range held {
		conn.Close()
	}
	if err := db.PingContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := db.PingContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	if db.Stats().MaxLifetimeClosed == 0 {
		t.Fatal("configured connection lifetime was not applied")
	}
	// Verify the production idle-time setting through a second configured connection.
	cfg.DBConnMaxLifetime = 30 * time.Minute
	cfg.DBConnMaxIdleTime = 20 * time.Millisecond
	Close()
	Connect(cfg)
	db, _ = DB.DB()
	if err := db.PingContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	// database/sql reaps idle connections on a cleaner tick of at least one second.
	deadline := time.Now().Add(5 * time.Second)
	for db.Stats().MaxIdleTimeClosed == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if db.Stats().MaxIdleTimeClosed == 0 {
		t.Fatalf("configured idle timeout was not applied: %+v", db.Stats())
	}
	if err := db.PingContext(t.Context()); err != nil {
		t.Fatal(err)
	}
}
