package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// AddVectorUpdateLogs adds vector_update_logs table for async processing
var AddVectorUpdateLogs = &gormigrate.Migration{
	ID: "202512301330_ADD_VECTOR_UPDATE_LOGS",
	Migrate: func(tx *gorm.DB) error {
		return tx.Exec(`
			CREATE TABLE IF NOT EXISTS vector_update_logs (
				id SERIAL PRIMARY KEY,
				source VARCHAR(50) NOT NULL,
				reference_id VARCHAR(100),
				status VARCHAR(20) NOT NULL DEFAULT 'pending',
				created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
			);
			CREATE INDEX idx_vector_update_logs_status ON vector_update_logs(status);
		`).Error
	},
	Rollback: func(tx *gorm.DB) error {
		return tx.Exec("DROP TABLE IF EXISTS vector_update_logs").Error
	},
}
