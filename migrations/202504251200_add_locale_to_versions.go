package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// AddLocaleToVersions adds locale column to versions table
var AddLocaleToVersions = &gormigrate.Migration{
	ID: "202504251200_ADD_LOCALE_TO_VERSIONS",
	Migrate: func(tx *gorm.DB) error {
		return tx.Exec("ALTER TABLE versions ADD COLUMN locale VARCHAR(20)").Error
	},
	Rollback: func(tx *gorm.DB) error {
		return tx.Exec("ALTER TABLE versions DROP COLUMN locale").Error
	},
}
