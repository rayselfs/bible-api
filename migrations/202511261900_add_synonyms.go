package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

var AddSynonyms = &gormigrate.Migration{
	ID: "202511261900_add_synonyms",
	Migrate: func(tx *gorm.DB) error {
		// 1. Create synonyms table
		if err := tx.Exec(`
			CREATE TABLE IF NOT EXISTS synonyms (
				id SERIAL PRIMARY KEY,
				term VARCHAR(100) UNIQUE NOT NULL,
				synonyms TEXT[] NOT NULL
			)
		`).Error; err != nil {
			return err
		}

		// 2. Insert initial data
		// "三位一體" -> ["父", "子", "聖靈"]
		// "登山寶訓" -> ["虛心的人有福了", "馬太福音"]
		if err := tx.Exec(`
			INSERT INTO synonyms (term, synonyms) VALUES 
			('三位一體', ARRAY['父', '子', '聖靈']),
			('登山寶訓', ARRAY['虛心的人有福了', '馬太福音'])
			ON CONFLICT (term) DO NOTHING
		`).Error; err != nil {
			return err
		}

		return nil
	},
	Rollback: func(tx *gorm.DB) error {
		return tx.Exec("DROP TABLE IF EXISTS synonyms").Error
	},
}
