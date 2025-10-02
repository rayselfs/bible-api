package main

import (
	"log"

	"hhc/bible-api/configs"
	"hhc/bible-api/internal/models"
	"hhc/bible-api/internal/server"
	"hhc/bible-api/migrations"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

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
	cfg, err := configs.InitConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v\n", err)
	}

	dsn := cfg.MysqlUser + ":" + cfg.MysqlPass + "@tcp(" + cfg.MysqlHost + ":" + cfg.MysqlPort + ")/" + cfg.MysqlDB + "?charset=utf8mb4&parseTime=True&loc=Local"
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:               dsn,
		DefaultStringSize: 256,
	}), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v\n", err)
	}
	log.Println("Database connection successful.")

	m := gormigrate.New(db, gormigrate.DefaultOptions, []*gormigrate.Migration{
		migrations.InitialSchema,
	})
	if err = m.Migrate(); err != nil {
		log.Fatalf("Could not migrate: %v", err)
	}
	log.Println("Migration run successfully")

	store := models.NewStore(db)
	api := server.NewAPI(store, cfg.AISearchURL)
	router := api.RegisterRoutes()

	log.Printf("Starting server on :%s", cfg.ServerPort)
	log.Printf("Swagger UI is available at http://localhost:%s/swagger/index.html", cfg.ServerPort)
	if err := router.Run(":" + cfg.ServerPort); err != nil {
		log.Fatalf("Could not start server: %s\n", err)
	}
}
