package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"hhc/bible-api/configs"
	"hhc/bible-api/internal/database"
	importer "hhc/bible-api/internal/import"
	"hhc/bible-api/internal/logger"
	"hhc/bible-api/internal/models"
	"hhc/bible-api/internal/server"

	"github.com/gin-gonic/gin"

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
		runImport()
		return
	}
	runServer()
}

// runImport executes Bible data import
func runImport() {
	// Parse flags
	importFlags := flag.NewFlagSet("import", flag.ExitOnError)
	dirFlag := importFlags.String("d", "", "Directory path containing JSON files to import")
	fileFlag := importFlags.String("f", "", "JSON file path to import")
	bookFlag := importFlags.Uint("b", 0, "Book number (required with -c)")
	chapterFlag := importFlags.Uint("c", 0, "Chapter number (required with -b)")

	// Parse flags from os.Args[2:] (skip "import" command)
	if err := importFlags.Parse(os.Args[2:]); err != nil {
		importer.PrintUsage()
		os.Exit(1)
	}

	// Validate flags
	hasDir := *dirFlag != ""
	hasFile := *fileFlag != ""
	hasBook := *bookFlag > 0
	hasChapter := *chapterFlag > 0

	if !hasDir && !hasFile {
		log.Fatalf("error: either -d (directory) or -f (file) must be specified")
		importer.PrintUsage()
		os.Exit(1)
	}

	if hasDir && hasFile {
		log.Fatalf("error: cannot use both -d and -f at the same time")
		importer.PrintUsage()
		os.Exit(1)
	}

	if (hasBook && !hasChapter) || (!hasBook && hasChapter) {
		log.Fatalf("error: both -b (book) and -c (chapter) must be specified together")
		importer.PrintUsage()
		os.Exit(1)
	}

	cfg, err := configs.InitConfig()
	if err != nil {
		log.Fatalf("error: failed to load config: %v", err)
	}

	database.Connect(cfg)
	defer database.Close()

	database.Connect(cfg)
	defer database.Close()

	// Execute import based on flags
	if hasDir {
		// Mode 1: Import all JSON files from directory
		if err := importer.ImportAllFromDataDir(database.DB, *dirFlag); err != nil {
			log.Fatalf("error: %v", err)
		}
	} else if hasFile {
		if hasBook && hasChapter {
			// Mode 3: Import single chapter
			if err := importer.Run(database.DB, *fileFlag, *bookFlag, *chapterFlag); err != nil {
				log.Fatalf("error: %v", err)
			}
		} else {
			// Mode 2: Import single file
			if err := importer.Run(database.DB, *fileFlag, 0, 0); err != nil {
				log.Fatalf("error: %v", err)
			}
		}
	}
}

// runServer starts the API service
func runServer() {
	// Initialize Logger
	logger.Init()
	appLogger := logger.GetAppLogger()

	appLogger.Info("Starting Bible API Service...")

	// Load Config
	cfg, err := configs.InitConfig()
	if err != nil {
		appLogger.Fatalf("Failed to load config: %v", err)
	}
	appLogger.Info("Configuration loaded successfully")

	// Connect to Database
	database.Connect(cfg)
	database.Migrate()

	// Initialize Services
	store := models.NewStore(database.DB)

	// Initialize Handlers
	api := server.NewAPI(store)

	// Setup Router
	r := gin.Default()
	api.SetupRoutes(r)

	// Setup Server with timeouts
	srv := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start Server in Goroutine
	go func() {
		appLogger.Infof("Server starting on port %s", cfg.ServerPort)
		appLogger.Infof("Swagger UI available at http://localhost:%s/swagger/index.html", cfg.ServerPort)
		appLogger.Info("Bible API Service started successfully")

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			appLogger.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	appLogger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		appLogger.Fatalf("Server forced to shutdown: %v", err)
	}

	// Clean up resources
	appLogger.Info("Closing database connection...")
	database.Close()

	appLogger.Info("Server exiting")
}
