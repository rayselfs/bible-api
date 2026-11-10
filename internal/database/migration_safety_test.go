package database

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"hhc/bible-api/migrations"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func migrationFixture(t *testing.T) (*gorm.DB, *gorm.DB) {
	t.Helper()
	raw := os.Getenv("BIBLE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("BIBLE_TEST_DATABASE_URL not configured")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() != "127.0.0.1" || u.Path != "/bible_pool_test" {
		t.Fatal("requires disposable loopback bible_pool_test fixture")
	}
	open := func(dsn string) *gorm.DB {
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			t.Fatal(err)
		}
		pool, _ := db.DB()
		pool.SetMaxOpenConns(1)
		pool.SetMaxIdleConns(1)
		t.Cleanup(func() { pool.Close() })
		return db
	}
	admin := open(raw)
	schema := fmt.Sprintf("bible_migration_safety_%d", time.Now().UnixNano())
	if err := admin.Exec("CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public; CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec("DROP SCHEMA " + schema + " CASCADE") })
	query := u.Query()
	query.Set("search_path", schema+",public")
	u.RawQuery = query.Encode()
	return open(u.String()), admin
}

func TestMigrationFailureRollsBackDDLAndHistory(t *testing.T) {
	db, _ := migrationFixture(t)
	original := migrations.AddLocaleToVersions.Migrate
	defer func() { migrations.AddLocaleToVersions.Migrate = original }()
	migrations.AddLocaleToVersions.Migrate = func(tx *gorm.DB) error {
		if err := tx.Exec("CREATE TABLE migration_partial_effect(id integer)").Error; err != nil {
			return err
		}
		return errors.New("synthetic failure after DDL")
	}
	if err := migrate(t.Context(), db); err == nil || !strings.Contains(err.Error(), "synthetic failure") {
		t.Fatalf("expected injected DDL failure, got %v", err)
	}
	var tables int
	if err := db.Raw("SELECT count(*) FROM information_schema.tables WHERE table_schema=current_schema()").Scan(&tables).Error; err != nil || tables != 0 {
		t.Fatalf("failed migration retained tables=%d: %v", tables, err)
	}
	migrations.AddLocaleToVersions.Migrate = original
	if err := migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	if err := migrate(t.Context(), db); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.Raw("SELECT count(*) FROM migrations").Scan(&count).Error; err != nil || count != 6 {
		t.Fatalf("retry/replay history count=%d: %v", count, err)
	}
}

func TestConcurrentMigrationsWaitBeforeReadingHistory(t *testing.T) {
	db, admin := migrationFixture(t)
	other, err := gorm.Open(postgres.Open(db.Dialector.(*postgres.Dialector).Config.DSN), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := other.DB()
	defer pool.Close()
	pool.SetMaxOpenConns(1)
	original := migrations.InitialSchema.Migrate
	defer func() { migrations.InitialSchema.Migrate = original }()
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	migrations.InitialSchema.Migrate = func(tx *gorm.DB) error {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		return original(tx)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	results := make(chan error, 2)
	go func() { results <- migrate(ctx, db) }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("first migration never entered")
	}
	go func() { results <- migrate(ctx, other) }()
	deadline := time.Now().Add(2 * time.Second)
	var waiting bool
	for time.Now().Before(deadline) {
		if err := admin.Raw("SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event='advisory' AND query LIKE '%pg_advisory_xact_lock%')").Scan(&waiting).Error; err != nil {
			t.Error(err)
			break
		}
		if waiting || calls.Load() > 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(release)
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Errorf("concurrent migration: %v", err)
		}
	}
	if !waiting || calls.Load() != 1 {
		t.Fatalf("second runner did not wait before reading history: waiting=%v applied=%d", waiting, calls.Load())
	}
}

func TestMigrationLockWaitCancelsWithoutDDL(t *testing.T) {
	db, admin := migrationFixture(t)
	holder := admin.Begin()
	if holder.Error != nil {
		t.Fatal(holder.Error)
	}
	defer holder.Rollback()
	if err := holder.Exec("SELECT pg_advisory_xact_lock(721732019261110)").Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if err := migrate(ctx, db); err == nil || ctx.Err() == nil {
		t.Fatalf("lock wait did not cancel: %v", err)
	}
	var tables int
	if err := db.Raw("SELECT count(*) FROM information_schema.tables WHERE table_schema=current_schema()").Scan(&tables).Error; err != nil || tables != 0 {
		t.Fatalf("canceled lock wait wrote tables=%d: %v", tables, err)
	}
}
