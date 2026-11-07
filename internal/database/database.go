package database

import (
	"crypto/sha256"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"log"
	"os"

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
	connectionConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		appLogger.Fatal("Invalid database configuration")
	}
	if revision := os.Getenv("CONTAINER_APP_REVISION"); revision != "" {
		connectionConfig.RuntimeParams["application_name"] = revisionApplicationName(revision)
	}
	connection := stdlib.OpenDB(*connectionConfig)
	DB, err = gorm.Open(postgres.New(postgres.Config{Conn: connection}), &gorm.Config{Logger: customGormLogger, DisableAutomaticPing: true})
	if err != nil {
		appLogger.Fatal("Failed to connect to database")
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
		appLogger.Fatal("Failed to connect to database")
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
		migrations.ChangeVectorDimension,
		migrations.AddVectorUpdateLogs,
		migrations.AddLocaleToVersions,
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

// Preserve the complete revision normally; hash overlong names to retain identity.
func revisionApplicationName(revision string) string {
	sum := sha256.Sum256([]byte(revision))
	for _, c := range []byte(revision) {
		if c < 32 || c > 126 {
			return fmt.Sprintf("revision-%x", sum[:8])
		}
	}
	if len(revision) <= 63 {
		return revision
	}
	return fmt.Sprintf("%s-%x", revision[:46], sum[:8])
}
