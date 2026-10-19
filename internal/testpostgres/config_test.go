package testpostgres

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestLoopbackConfigRejectsRemoteHostOverride(t *testing.T) {
	if _, err := loopbackConfig("postgres://127.0.0.1/fixture?host=remote.example&sslmode=disable"); err == nil {
		t.Fatal("accepted a remote pgx host override behind a loopback URL authority")
	}
}

func TestLoopbackConfigTargets(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want bool
	}{
		{"ipv4", "postgres://127.0.0.1:5432/fixture?sslmode=disable", true},
		{"ipv6", "postgres://[::1]:5432/fixture?sslmode=disable", true},
		{"tls fallback", "postgres://127.0.0.1/fixture?sslmode=prefer", true},
		{"loopback override", "postgres://remote.example/fixture?host=127.0.0.1&sslmode=disable", true},
		{"loopback hosts and ports", "postgres://127.0.0.1:5432,127.0.0.2:5433/fixture?sslmode=disable", true},
		{"remote primary", "postgres://remote.example/fixture?sslmode=disable", false},
		{"remote query fallback", "postgres://127.0.0.1/fixture?host=127.0.0.1,remote.example&sslmode=disable", false},
		{"remote authority fallback", "postgres://127.0.0.1:5432,remote.example:5433/fixture?sslmode=disable", false},
		{"remote before loopback", "postgres://127.0.0.1/fixture?host=remote.example,127.0.0.1&sslmode=prefer", false},
		{"socket override", "postgres://127.0.0.1/fixture?host=%2Ftmp&sslmode=disable", false},
		{"dns hostname", "postgres://localhost/fixture?sslmode=disable", false},
		{"zero port", "postgres://127.0.0.1:0/fixture?sslmode=disable", false},
		{"invalid fallback port", "postgres://127.0.0.1:5432,127.0.0.2:99999/fixture?sslmode=disable", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearPGEnvironment(t)
			_, err := loopbackConfig(tc.raw)
			if (err == nil) != tc.want {
				t.Fatalf("accepted=%t, want %t", err == nil, tc.want)
			}
		})
	}
}

func clearPGEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{"PGHOST", "PGPORT", "PGSERVICE", "PGSERVICEFILE", "PGSSLMODE", "PGSSLROOTCERT", "PGSSLCERT", "PGSSLKEY"} {
		t.Setenv(name, "")
	}
}

func TestLoopbackConfigEnvironmentTargets(t *testing.T) {
	for _, tc := range []struct {
		name string
		host string
		port string
		raw  string
		want bool
	}{
		{"remote PGHOST", "remote.example", "5432", "postgres:///fixture?sslmode=disable", false},
		{"remote PGHOST fallback", "127.0.0.1,remote.example", "5432,5433", "postgres:///fixture?sslmode=disable", false},
		{"socket PGHOST", "/tmp", "5432", "postgres:///fixture?sslmode=disable", false},
		{"invalid PGPORT", "127.0.0.1", "99999", "postgres:///fixture?sslmode=disable", false},
		{"explicit loopback wins", "remote.example", "5433", "postgres://127.0.0.1:5432/fixture?sslmode=disable", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearPGEnvironment(t)
			t.Setenv("PGHOST", tc.host)
			t.Setenv("PGPORT", tc.port)
			_, err := loopbackConfig(tc.raw)
			if (err == nil) != tc.want {
				t.Fatalf("accepted=%t, want %t", err == nil, tc.want)
			}
		})
	}
}

func TestLoopbackConfigRejectsRemoteServiceTargets(t *testing.T) {
	for _, host := range []string{"remote.example", "127.0.0.1,remote.example"} {
		t.Run(host, func(t *testing.T) {
			clearPGEnvironment(t)
			path := filepath.Join(t.TempDir(), "pg_service.conf")
			if err := os.WriteFile(path, []byte("[fixture]\nhost="+host+"\nport=5432\nsslmode=disable\n"), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PGSERVICE", "fixture")
			t.Setenv("PGSERVICEFILE", path)
			if _, err := loopbackConfig("postgres:///fixture"); err == nil {
				t.Fatal("accepted a remote service target")
			}
		})
	}
}

func TestLoopbackDSNFreezesEnvironmentBeforeLazyConnect(t *testing.T) {
	raw := os.Getenv("BIBLE_TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("BIBLE_TEST_DATABASE_URL is not configured")
	}
	config, err := loopbackConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	clearPGEnvironment(t)
	t.Setenv("PGHOST", config.Host)
	t.Setenv("PGPORT", strconv.Itoa(int(config.Port)))
	t.Setenv("PGDATABASE", config.Database)
	t.Setenv("PGUSER", config.User)
	t.Setenv("PGPASSWORD", config.Password)
	dsn := LoopbackDSN(t, "postgres:///?sslmode=disable")
	t.Setenv("PGHOST", "remote.example")
	t.Setenv("PGPORT", "99999")
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal("validated fixture configuration was not preserved at connection time")
	}
}
