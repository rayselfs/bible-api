package models

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/pgvector/pgvector-go"
	"gorm.io/gorm"
)

// Store contains a *gorm.DB instance
type Store struct {
	DB *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{DB: db}
}

// GetAllVersions Get all Bible versions
func (s *Store) GetAllVersions() ([]VersionListItem, error) {
	var versions []Versions
	err := s.DB.Find(&versions).Error
	if err != nil {
		return nil, err
	}

	// Convert to API response format
	versionList := make([]VersionListItem, len(versions))
	for i, version := range versions {
		versionList[i] = VersionListItem{
			ID:   version.ID,
			Code: version.Code,
			Name: version.Name,
		}
	}

	return versionList, nil
}

// StreamBibleContent streams Bible content by version ID using channels
// This method returns a channel that yields Bible books one by one for streaming
func (s *Store) StreamBibleContent(ctx context.Context, versionID uint) (<-chan []byte, <-chan error) {
	contentChan := make(chan []byte, 10) // Buffer for better performance
	errorChan := make(chan error, 1)

	go func() {
		defer close(contentChan)
		defer close(errorChan)

		// Get version information first
		var version Versions
		if err := s.DB.WithContext(ctx).Where(&Versions{ID: versionID}).First(&version).Error; err != nil {
			errorChan <- err
			return
		}

		// Send version header
		versionHeader := map[string]interface{}{
			"version_id":   version.ID,
			"version_code": version.Code,
			"version_name": version.Name,
			"books":        []interface{}{},
		}

		headerBytes, err := json.Marshal(versionHeader)
		if err != nil {
			errorChan <- fmt.Errorf("failed to marshal version header: %w", err)
			return
		}
		contentChan <- headerBytes

		// Get books one by one and stream them
		var books []Books
		if err := s.DB.WithContext(ctx).Preload("Chapters.Verses").
			Where(&Books{VersionID: version.ID}).
			Order("number ASC").Find(&books).Error; err != nil {
			errorChan <- err
			return
		}

		for _, book := range books {
			select {
			case <-ctx.Done():
				errorChan <- ctx.Err()
				return
			default:
			}

			// Convert book to API format
			chapters := make([]BibleContentChapter, len(book.Chapters))
			for j, chapter := range book.Chapters {
				verses := make([]BibleContentVerse, len(chapter.Verses))
				for k, verse := range chapter.Verses {
					verses[k] = BibleContentVerse{
						ID:     verse.ID,
						Number: verse.Number,
						Text:   verse.Text,
					}
				}

				chapters[j] = BibleContentChapter{
					ID:     chapter.ID,
					Number: chapter.Number,
					Verses: verses,
				}
			}

			bookData := BibleContentBook{
				ID:           book.ID,
				Number:       book.Number,
				Name:         book.Name,
				Abbreviation: book.Abbreviation,
				Chapters:     chapters,
			}

			bookBytes, err := json.Marshal(bookData)
			if err != nil {
				errorChan <- fmt.Errorf("failed to marshal book %d: %w", book.ID, err)
				return
			}

			contentChan <- bookBytes
		}
	}()

	return contentChan, errorChan
}

// SearchVerses performs hybrid search using pgvector and tsvector
// Logic: Split into two queries (Vector + Keyword) and merge in backend using RRF
// SearchVerses performs hybrid search using pgvector and tsvector
// Logic: Split into two queries (Vector + Keyword) and merge in backend using RRF
func (s *Store) SearchVerses(ctx context.Context, query string, embedding []float32, versionFilter string, limit int) ([]SearchResult, error) {
	// 1. Vector Search
	var vectorResults []SearchResult
	vectorSql := `
		SELECT 
			v.id::text as verse_id, 
			v.text, 
			v.number as verse_number,
			c.number as chapter_number,
			b.number as book_number,
			ver.code as version_code,
			1 - (bv.embedding <=> ?) as score
		FROM verses v
		JOIN bible_vectors bv ON v.id = bv.verse_id
		JOIN chapters c ON v.chapter_id = c.id
		JOIN books b ON c.book_id = b.id
		JOIN versions ver ON b.version_id = ver.id
		WHERE ver.code = ?
		ORDER BY bv.embedding <=> ? LIMIT ?
	`
	if err := s.DB.WithContext(ctx).Raw(vectorSql,
		pgvector.NewVector(embedding), versionFilter, pgvector.NewVector(embedding), limit,
	).Scan(&vectorResults).Error; err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	// 2. Keyword Search
	var keywordResults []SearchResult
	keywordSql := `
		SELECT 
			v.id::text as verse_id, 
			v.text, 
			v.number as verse_number,
			c.number as chapter_number,
			b.number as book_number,
			ver.code as version_code,
			ts_rank(to_tsvector('simple', v.text), websearch_to_tsquery('simple', ?)) as score
		FROM verses v
		JOIN chapters c ON v.chapter_id = c.id
		JOIN books b ON c.book_id = b.id
		JOIN versions ver ON b.version_id = ver.id
		WHERE ver.code = ? AND to_tsvector('simple', v.text) @@ websearch_to_tsquery('simple', ?)
		ORDER BY score DESC LIMIT ?
	`
	if err := s.DB.WithContext(ctx).Raw(keywordSql,
		query, versionFilter, query, limit,
	).Scan(&keywordResults).Error; err != nil {
		return nil, fmt.Errorf("keyword search failed: %w", err)
	}

	// 3. Merge Results using RRF
	return s.mergeResults(vectorResults, keywordResults, limit)
}

func (s *Store) mergeResults(vector, keyword []SearchResult, limit int) ([]SearchResult, error) {
	const k = 60.0
	scores := make(map[string]float64)
	data := make(map[string]SearchResult)

	// Helper to process results
	process := func(results []SearchResult) {
		for i, res := range results {
			if _, exists := data[res.VerseID]; !exists {
				data[res.VerseID] = res
			}
			// RRF: 1 / (k + rank)
			// rank is 1-based index (i + 1)
			scores[res.VerseID] += 1.0 / (k + float64(i+1))
		}
	}

	process(vector)
	process(keyword)

	// Convert to slice
	finalResults := make([]SearchResult, 0)
	for id, score := range scores {
		res := data[id]
		res.Score = score
		finalResults = append(finalResults, res)
	}

	// Sort by Score DESC
	sort.Slice(finalResults, func(i, j int) bool {
		return finalResults[i].Score > finalResults[j].Score
	})

	// Apply limit
	if len(finalResults) > limit {
		finalResults = finalResults[:limit]
	}

	return finalResults, nil
}
