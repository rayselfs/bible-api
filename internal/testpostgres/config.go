// Package testpostgres restricts destructive PostgreSQL fixtures to loopback.
package testpostgres

import (
	"database/sql"
	"fmt"
	"net"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// LoopbackDSN freezes the validated driver config so later environment changes
// cannot redirect a fixture when database/sql opens a connection lazily.
func LoopbackDSN(t testing.TB, raw string) string {
	t.Helper()
	config, err := loopbackConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	dsn := stdlib.RegisterConnConfig(config)
	t.Cleanup(func() { stdlib.UnregisterConnConfig(dsn) })
	return dsn
}

// Open supplies the validated pgx pool directly to consumers such as GORM,
// whose DSN parser cannot read stdlib's registered configuration names.
func Open(t testing.TB, raw string) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", LoopbackDSN(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func loopbackConfig(raw string) (*pgx.ConnConfig, error) {
	config, err := pgx.ParseConfig(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid PostgreSQL fixture configuration")
	}
	loopback := func(host string, port uint16) bool {
		ip := net.ParseIP(host)
		return ip != nil && ip.IsLoopback() && port != 0
	}
	if !loopback(config.Host, config.Port) {
		return nil, fmt.Errorf("PostgreSQL fixture must use loopback")
	}
	for _, fallback := range config.Fallbacks {
		if !loopback(fallback.Host, fallback.Port) {
			return nil, fmt.Errorf("PostgreSQL fixture fallbacks must use loopback")
		}
	}
	return config, nil
}
