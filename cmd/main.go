package main

import (
	"crypto/tls"
	"crypto/x509"
	"os"

	"hhc/bible-api/configs"
	"hhc/bible-api/internal/logger"
	"hhc/bible-api/internal/models"
	"hhc/bible-api/internal/server"
	"hhc/bible-api/migrations"

	"github.com/go-gormigrate/gormigrate/v2"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
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

// @host         localhost:8080
// @BasePath     /
func main() {
	// Initialize structured logger
	logger.Init()
	appLogger := logger.GetAppLogger()

	appLogger.Info("Starting Bible API Service...")

	cfg, err := configs.InitConfig()
	if err != nil {
		appLogger.Fatalf("Failed to load config: %v", err)
	}
	appLogger.Info("Configuration loaded successfully")

	// Check if CA certificate exists and register TLS config for Azure MySQL
	var tlsParam string
	certPath := "DigiCertGlobalRootG2.crt.pem"
	if _, err := os.Stat(certPath); err == nil {
		// Certificate exists, use TLS
		rootCertPool := x509.NewCertPool()
		pem, err := os.ReadFile(certPath)
		if err != nil {
			appLogger.Fatalf("Failed to read CA certificate: %v", err)
		}
		if ok := rootCertPool.AppendCertsFromPEM(pem); !ok {
			appLogger.Fatal("Failed to append CA certificate to pool")
		}
		mysqlDriver.RegisterTLSConfig("azure", &tls.Config{
			RootCAs: rootCertPool,
		})
		tlsParam = "&tls=azure"
		appLogger.Info("TLS configuration registered for Azure MySQL")
	} else {
		// Certificate not found, use plain connection (for local development)
		tlsParam = ""
		appLogger.Info("CA certificate not found, using non-TLS connection for local development")
	}

	dsn := cfg.MysqlUser + ":" + cfg.MysqlPass + "@tcp(" + cfg.MysqlHost + ":" + cfg.MysqlPort + ")/" + cfg.MysqlDB + "?charset=utf8mb4&parseTime=True&loc=Local" + tlsParam

	// Create custom GORM logger with JSON format
	customGormLogger := logger.NewGormLogger(appLogger, gormLogger.Warn)

	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:               dsn,
		DefaultStringSize: 256,
	}), &gorm.Config{
		Logger: customGormLogger,
	})
	if err != nil {
		appLogger.Fatalf("Failed to connect to database: %v", err)
	}
	appLogger.Info("Database connection successful")

	m := gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{
		migrations.InitialSchema,
	})
	if err = m.Migrate(); err != nil {
		appLogger.Fatalf("Database migration failed: %v", err)
	}
	appLogger.Info("Database migration completed successfully")

	store := models.NewStore(db)
	api := server.NewAPI(store, cfg.AISearchURL)
	router := api.RegisterRoutes()

	appLogger.Infof("Starting server on port %s", cfg.ServerPort)
	appLogger.Infof("Swagger UI available at http://localhost:%s/swagger/index.html", cfg.ServerPort)
	appLogger.Info("Bible API Service started successfully")

	if err := router.Run(":" + cfg.ServerPort); err != nil {
		appLogger.Fatalf("Server startup failed: %s", err)
	}
}
