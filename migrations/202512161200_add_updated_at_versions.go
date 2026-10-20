package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// AddUpdatedAtToVersions adds updated_at column to versions table
var AddUpdatedAtToVersions = &gormigrate.Migration{
	ID: "202512161200_ADD_UPDATED_AT_VERSIONS",
	Migrate: func(tx *gorm.DB) error {
		// InitialSchema uses current models and may have already created the column.
		if err := tx.Exec("ALTER TABLE versions ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP").Error; err != nil {
			return err
		}
		return tx.Exec("ALTER TABLE versions ALTER COLUMN updated_at SET DEFAULT CURRENT_TIMESTAMP").Error
	},
	Rollback: func(tx *gorm.DB) error {
		return tx.Exec("ALTER TABLE versions DROP COLUMN updated_at").Error
	},
}
