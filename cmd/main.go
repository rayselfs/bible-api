package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"hhc/bible-api/configs"
	importer "hhc/bible-api/internal/import"
	"hhc/bible-api/internal/logger"
	"hhc/bible-api/internal/models"
	oaiOpenAI "hhc/bible-api/internal/pkg/openai"
	"hhc/bible-api/internal/server"
	"hhc/bible-api/migrations"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"

	_ "hhc/bible-api/docs"
)

// @title        Bible System API
// @version      1.0
// @description  This is a sample server for a Bible API system, powered by Gin and GORM.
// @termsOfService http://swagger.io/terms/

// @contact.name   API Support
// @contact.url    https://github.com/your-repo
// @contact.email  rayselfs@alive.org.tw

// @license.name  MIT
// @license.url   https://opensource.org/licenses/MIT

// @host         www.alive.org.tw
// @schemes      https
// @BasePath     /
func main() {
	if len(os.Args) > 1 && os.Args[1] == "import" {
		if len(os.Args) < 3 {
			importer.PrintUsage()
			os.Exit(1)
		}
		runImport(os.Args[2])
		return
	}
	runServer()
}

// buildDSN constructs PostgreSQL connection string from config
func buildDSN(cfg *configs.Env) string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.PostgresHost, cfg.PostgresPort, cfg.PostgresUser, cfg.PostgresPass, cfg.PostgresDB, cfg.PostgresSSLMode)
}

// connectDB establishes database connection with optional GORM config
func connectDB(dsn string, gormConfig *gorm.Config) (*gorm.DB, error) {
	if gormConfig == nil {
		gormConfig = &gorm.Config{}
	}
	return gorm.Open(postgres.Open(dsn), gormConfig)
}

// runImport executes Bible data import
func runImport(filePath string) {
	cfg, err := configs.InitConfig()
	if err != nil {
		log.Fatalf("❌ Failed to load config: %v", err)
	}

	fmt.Println("✅ Connecting to PostgreSQL database")
	db, err := connectDB(buildDSN(cfg), nil)
	if err != nil {
		log.Fatalf("❌ Failed to connect to database: %v", err)
	}
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	}()

	fmt.Println("✅ Database connection successful")

	// Initialize OpenAI client for embeddings
	fmt.Println("✅ Initializing OpenAI client for embeddings")
	oaiClient := openai.NewClient(
		option.WithBaseURL(cfg.AzureOpenAIBaseURL),
		option.WithAPIKey(cfg.AzureOpenAIKey),
	)
	openAIService := oaiOpenAI.NewOpenAIService(&oaiClient, cfg.AzureOpenAIModelName)

	if err := importer.Run(db, openAIService, filePath); err != nil {
		log.Fatalf("❌ %v", err)
	}
}

// runServer starts the API service
func runServer() {
	logger.Init()
	appLogger := logger.GetAppLogger()

	appLogger.Info("Starting Bible API Service...")

	cfg, err := configs.InitConfig()
	if err != nil {
		appLogger.Fatalf("Failed to load config: %v", err)
	}
	appLogger.Info("Configuration loaded successfully")

	appLogger.Info("Connecting to PostgreSQL database")
	customGormLogger := logger.NewGormLogger(appLogger, gormLogger.Warn)
	db, err := connectDB(buildDSN(cfg), &gorm.Config{Logger: customGormLogger})
	if err != nil {
		appLogger.Fatalf("Failed to connect to database: %v", err)
	}
	appLogger.Info("Database connection successful")

	m := gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{
		migrations.InitialSchema,
		migrations.AddHybridSearch,
	})
	if err = m.Migrate(); err != nil {
		appLogger.Fatalf("Database migration failed: %v", err)
	}
	appLogger.Info("Database migration completed successfully")

	store := models.NewStore(db)
	httpClient := &http.Client{Timeout: 30 * time.Second}
	oaiClient := openai.NewClient(
		option.WithBaseURL(cfg.AzureOpenAIBaseURL),
		option.WithAPIKey(cfg.AzureOpenAIKey),
	)

	api := server.NewAPI(store, &oaiClient, httpClient, cfg.AzureOpenAIModelName)
	router := api.RegisterRoutes()

	appLogger.Infof("Starting server on port %s", cfg.ServerPort)
	appLogger.Infof("Swagger UI available at http://localhost:%s/swagger/index.html", cfg.ServerPort)
	appLogger.Info("Bible API Service started successfully")

	if err := router.Run(":" + cfg.ServerPort); err != nil {
		appLogger.Fatalf("Server startup failed: %s", err)
	}
}
