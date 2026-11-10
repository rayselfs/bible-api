package database

import (
	"context"
	"fmt"
	"log"
	"time"

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
	// Configure both timers before the first connection starts the Go 1.24 cleaner.
	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: customGormLogger, DisableAutomaticPing: true})
	if err != nil {
		appLogger.Fatalf("Failed to connect to database: %v", err)
	}

	sqlDB, err := DB.DB()
	if err != nil {
		appLogger.Fatalf("Failed to configure database pool: %v", err)
	}
	sqlDB.SetMaxOpenConns(cfg.DBMaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.DBMaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.DBConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.DBConnMaxIdleTime)
	if err := sqlDB.Ping(); err != nil {
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := migrate(ctx, DB); err != nil {
		appLogger.Fatalf("Database migration failed: %v", err)
	}
	appLogger.Info("Database migration completed successfully")
}

func migrate(ctx context.Context, db *gorm.DB) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock and DDL share one transaction so failure cannot leave partial history or a pooled session lock.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(721732019261110)").Error; err != nil {
			return err
		}
		return gormigrate.New(tx, gormigrate.DefaultOptions, []*gormigrate.Migration{
			migrations.InitialSchema,
			migrations.AddHybridSearch,
			migrations.AddUpdatedAtToVersions,
			migrations.ChangeVectorDimension,
			migrations.AddVectorUpdateLogs,
			migrations.AddLocaleToVersions,
		}).Migrate()
	})
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
