package migrations

import (
	"hhc/bible-api/internal/models"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// InitialSchema is a structure containing ID and migration functions
var InitialSchema = &gormigrate.Migration{
	// ID must be unique, usually using timestamp
	ID: "202508302145_INITIAL_SCHEMA",

	// Migrate is the upgrade function
	Migrate: func(tx *gorm.DB) error {
		// Use GORM AutoMigrate to create initial tables
		// Note order: create parent tables first, then child tables
		return tx.AutoMigrate(
			&models.Versions{},
			&models.Books{},
			&models.Chapters{},
			&models.Verses{},
		)
	},

	// Rollback is the downgrade function
	Rollback: func(tx *gorm.DB) error {
		// DropTable accepts interface{}, so we pass model pointers
		// Note Drop order is opposite to Migrate order to handle foreign key constraints
		return tx.Migrator().DropTable(
			&models.Verses{},
			&models.Chapters{},
			&models.Books{},
			&models.Versions{},
		)
	},
}
