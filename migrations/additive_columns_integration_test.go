package migrations

import (
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"hhc/bible-api/internal/models"
)

func TestAdditiveColumnsAfterCurrentInitialModels(t *testing.T) {
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
	schema := fmt.Sprintf("bible_columns_%d", time.Now().UnixNano())
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	defer admin.Exec("DROP SCHEMA " + schema + " CASCADE")
	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()
	db, err := gorm.Open(postgres.Open(u.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	pool.SetMaxOpenConns(2)
	pool.SetMaxIdleConns(1)
	defer pool.Close()
	if err := db.AutoMigrate(&models.Versions{}); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"created by current initial models", "missing on an older schema"} {
		t.Run(state, func(t *testing.T) {
			for _, migration := range []struct {
				name  string
				apply func(*gorm.DB) error
			}{
				{"updated_at", AddUpdatedAtToVersions.Migrate}, {"locale", AddLocaleToVersions.Migrate},
			} {
				t.Run(migration.name, func(t *testing.T) {
					if err := migration.apply(db); err != nil {
						t.Fatal(err)
					}
				})
			}
			var updated time.Time
			if err := pool.QueryRowContext(t.Context(), "INSERT INTO versions(code,name) VALUES($1,'Column fixture') RETURNING updated_at", state[:3]).Scan(&updated); err != nil {
				t.Fatal(err)
			}
		})
		if t.Failed() {
			return
		}
		if err := db.Exec("ALTER TABLE versions DROP COLUMN updated_at, DROP COLUMN locale").Error; err != nil {
			t.Fatal(err)
		}
	}
}
