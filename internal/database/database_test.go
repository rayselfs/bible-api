package database

import (
	"database/sql"
	"testing"
)

func TestConfigurePoolBoundsConnections(t *testing.T) {
	db, err := sql.Open("pgx", "postgres://test:test@localhost/test?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	configurePool(db)
	if got := db.Stats().MaxOpenConnections; got != 4 {
		t.Fatalf("MaxOpenConnections = %d, want 4", got)
	}
}
