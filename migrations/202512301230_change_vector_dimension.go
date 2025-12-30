package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// ChangeVectorDimension modifies the vector dimension from 1536 to 384
var ChangeVectorDimension = &gormigrate.Migration{
	ID: "202512301230_CHANGE_VECTOR_DIMENSION",
	Migrate: func(tx *gorm.DB) error {
		// 1. Drop the existing HNSW index (it depends on vector dimension)
		if err := tx.Exec("DROP INDEX IF EXISTS bible_vectors_embedding_idx").Error; err != nil {
			return err
		}

		// 2. Alter column type. We MUST use 'USING NULL' or similar because 1536->384 cannot be cast.
		// This CLEARS existing data. This is expected as we need to regenerate embeddings anyway.
		if err := tx.Exec("ALTER TABLE bible_vectors ALTER COLUMN embedding TYPE vector(384) USING NULL").Error; err != nil {
			return err
		}

		// 3. Recreate HNSW index
		if err := tx.Exec("CREATE INDEX IF NOT EXISTS bible_vectors_embedding_idx ON bible_vectors USING hnsw (embedding vector_cosine_ops)").Error; err != nil {
			return err
		}

		return nil
	},
	Rollback: func(tx *gorm.DB) error {
		// Rollback to 1536
		if err := tx.Exec("DROP INDEX IF EXISTS bible_vectors_embedding_idx").Error; err != nil {
			return err
		}

		if err := tx.Exec("ALTER TABLE bible_vectors ALTER COLUMN embedding TYPE vector(1536) USING NULL").Error; err != nil {
			return err
		}

		if err := tx.Exec("CREATE INDEX IF NOT EXISTS bible_vectors_embedding_idx ON bible_vectors USING hnsw (embedding vector_cosine_ops)").Error; err != nil {
			return err
		}
		return nil
	},
}
