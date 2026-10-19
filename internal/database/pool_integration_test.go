package database

import (
	"context"
	"errors"
	"hhc/bible-api/configs"
	"net/url"
	"os"
	"testing"
	"time"
)

func TestPoolCapAndCancellation(t *testing.T) {
	raw := os.Getenv("BIBLE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("BIBLE_TEST_DATABASE_URL is not configured")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() != "127.0.0.1" {
		t.Fatal("integration tests require a loopback PostgreSQL fixture")
	}
	password, _ := u.User.Password()
	cfg := &configs.Env{PostgresHost: u.Hostname(), PostgresPort: u.Port(), PostgresUser: u.User.Username(), PostgresPass: password, PostgresDB: u.Path[1:], PostgresSSLMode: "disable", DBMaxOpenConns: 2, DBMaxIdleConns: 1, DBConnMaxLifetime: 80 * time.Millisecond, DBConnMaxIdleTime: 5 * time.Minute}
	Connect(cfg)
	t.Cleanup(Close)
	db, err := DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	if db.Stats().MaxOpenConnections != 2 {
		t.Fatal("runtime pool has no configured limit")
	}
	first, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	if err := db.PingContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("pool waiter did not honor cancellation: %v", err)
	}
	if db.Stats().OpenConnections > 2 || db.Stats().WaitCount == 0 {
		t.Fatal("pool limit was bypassed")
	}
	first.Close()
	second.Close()
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
	db.SetConnMaxLifetime(30 * time.Minute)
	// Verify the production idle-time setting through a second configured connection.
	cfg.DBConnMaxLifetime = 30 * time.Minute
	cfg.DBConnMaxIdleTime = 20 * time.Millisecond
	Close()
	Connect(cfg)
	db, _ = DB.DB()
	deadline := time.Now().Add(2 * time.Second)
	for db.Stats().MaxIdleTimeClosed == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if db.Stats().MaxIdleTimeClosed == 0 {
		t.Fatal("configured idle timeout was not applied")
	}
	if err := db.PingContext(t.Context()); err != nil {
		t.Fatal(err)
	}
}
