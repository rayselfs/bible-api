package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// AddHybridSearch adds vector extension and columns for hybrid search
var AddHybridSearch = &gormigrate.Migration{
	ID: "202511261800_ADD_HYBRID_SEARCH",
	Migrate: func(tx *gorm.DB) error {
		// 1. Enable pgvector extension
		if err := tx.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error; err != nil {
			return err
		}

		// 2. Create bible_vectors table
		// Using 1536 dimensions for OpenAI text-embedding-3-small
		if err := tx.Exec(`
			CREATE TABLE IF NOT EXISTS bible_vectors (
				id SERIAL PRIMARY KEY,
				verse_id INTEGER NOT NULL REFERENCES verses(id) ON DELETE CASCADE,
				embedding vector(1536),
				updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
			)
		`).Error; err != nil {
			return err
		}

		// 3. Add HNSW index for vector search on bible_vectors
		if err := tx.Exec("CREATE INDEX IF NOT EXISTS bible_vectors_embedding_idx ON bible_vectors USING hnsw (embedding vector_cosine_ops)").Error; err != nil {
			return err
		}

		// 4. Add GIN index for full-text search (tsvector) on verses table
		if err := tx.Exec("CREATE INDEX IF NOT EXISTS verses_text_search_idx ON verses USING GIN (to_tsvector('simple', text))").Error; err != nil {
			return err
		}

		return nil
	},
	Rollback: func(tx *gorm.DB) error {
		if err := tx.Exec("DROP INDEX IF EXISTS verses_text_search_idx").Error; err != nil {
			return err
		}
		if err := tx.Exec("DROP TABLE IF EXISTS bible_vectors").Error; err != nil {
			return err
		}
		return nil
	},
}
