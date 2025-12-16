package database

import (
	"fmt"
	"log"

	"hhc/bible-api/configs"
	"hhc/bible-api/internal/logger"
	"hhc/bible-api/migrations"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"
)

var DB *gorm.DB

// Connect establishes database connection
func Connect(cfg *configs.Env) {
	dsn := buildDSN(cfg)
	appLogger := logger.GetAppLogger()
	customGormLogger := logger.NewGormLogger(appLogger, gormLogger.Warn)

	var err error
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: customGormLogger})
	if err != nil {
		appLogger.Fatalf("Failed to connect to database: %v", err)
	}

	appLogger.Info("Database connection successful")
}

// buildDSN constructs PostgreSQL connection string from config
func buildDSN(cfg *configs.Env) string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.PostgresHost, cfg.PostgresPort, cfg.PostgresUser, cfg.PostgresPass, cfg.PostgresDB, cfg.PostgresSSLMode)
}

// Migrate runs database migrations
func Migrate() {
	appLogger := logger.GetAppLogger()

	m := gormigrate.New(DB, gormigrate.DefaultOptions, []*gormigrate.Migration{
		migrations.InitialSchema,
		migrations.AddHybridSearch,
		migrations.AddUpdatedAtToVersions,
	})

	if err := m.Migrate(); err != nil {
		appLogger.Fatalf("Database migration failed: %v", err)
	}

	appLogger.Info("Database migration completed successfully")
}

// Close closes the database connection
func Close() {
	if DB != nil {
		sqlDB, err := DB.DB()
		if err == nil {
			if err := sqlDB.Close(); err != nil {
				log.Printf("Error closing database connection: %v", err)
			}
		}
	}
}
