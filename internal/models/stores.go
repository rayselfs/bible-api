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

// getBookCode returns the standard 3-letter code for a book number
func getBookCode(bookNumber uint) string {
	bookCodeMap := map[uint]string{
		1: "GEN", 2: "EXO", 3: "LEV", 4: "NUM", 5: "DEU",
		6: "JOS", 7: "JDG", 8: "RUT", 9: "1SA", 10: "2SA",
		11: "1KI", 12: "2KI", 13: "1CH", 14: "2CH", 15: "EZR",
		16: "NEH", 17: "EST", 18: "JOB", 19: "PSA", 20: "PRO",
		21: "ECC", 22: "SNG", 23: "ISA", 24: "JER", 25: "LAM",
		26: "EZK", 27: "DAN", 28: "HOS", 29: "JOL", 30: "AMO",
		31: "OBA", 32: "JON", 33: "MIC", 34: "NAM", 35: "HAB",
		36: "ZEP", 37: "HAG", 38: "ZEC", 39: "MAL", 40: "MAT",
		41: "MRK", 42: "LUK", 43: "JHN", 44: "ACT", 45: "ROM",
		46: "1CO", 47: "2CO", 48: "GAL", 49: "EPH", 50: "PHP",
		51: "COL", 52: "1TH", 53: "2TH", 54: "1TI", 55: "2TI",
		56: "TIT", 57: "PHM", 58: "HEB", 59: "JAS", 60: "1PE",
		61: "2PE", 62: "1JN", 63: "2JN", 64: "3JN", 65: "JUD",
		66: "REV",
	}
	if code, ok := bookCodeMap[bookNumber]; ok {
		return code
	}
	return "" // Return empty string if book number not found
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
			"books":        []any{},
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
				Code:         getBookCode(book.Number),
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
	// Strategy to handle common words like "上帝":
	// 1. Limit keyword search results to avoid too many matches
	// 2. Add minimum score threshold for keyword search
	// 3. Use combined scoring in merge for better ranking

	// Calculate keyword search limit (use smaller limit to avoid too many results)
	// For common single words, we want fewer but more relevant keyword results
	keywordLimit := limit
	if len(query) <= 3 {
		// For very short queries (likely common words), use smaller limit
		keywordLimit = max(limit/2, 5)
	}

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

	// 2. Keyword Search with minimum score threshold
	// Use ts_rank_cd for better ranking and add minimum threshold
	var keywordResults []SearchResult
	keywordSql := `
		SELECT 
			v.id::text as verse_id, 
			v.text, 
			v.number as verse_number,
			c.number as chapter_number,
			b.number as book_number,
			ver.code as version_code,
			ts_rank_cd(to_tsvector('simple', v.text), websearch_to_tsquery('simple', ?)) as score
		FROM verses v
		JOIN chapters c ON v.chapter_id = c.id
		JOIN books b ON c.book_id = b.id
		JOIN versions ver ON b.version_id = ver.id
		WHERE ver.code = ? 
			AND to_tsvector('simple', v.text) @@ websearch_to_tsquery('simple', ?)
			AND ts_rank_cd(to_tsvector('simple', v.text), websearch_to_tsquery('simple', ?)) > 0.05
		ORDER BY score DESC LIMIT ?
	`
	if err := s.DB.WithContext(ctx).Raw(keywordSql,
		query, versionFilter, query, query, keywordLimit,
	).Scan(&keywordResults).Error; err != nil {
		return nil, fmt.Errorf("keyword search failed: %w", err)
	}

	// 3. Merge Results with intelligent scoring
	return s.mergeResults(vectorResults, keywordResults, limit)
}

// mergeResults merges the vector and keyword results
// Strategy:
// 1. Keyword results have priority and are placed first
// 2. Remove duplicates from vector results (if already in keyword)
// 3. For remaining results, use combined scoring to rank them intelligently
func (s *Store) mergeResults(vector, keyword []SearchResult, limit int) ([]SearchResult, error) {
	keywordVerseIDs := make(map[string]bool)
	finalResults := make([]SearchResult, 0)

	// 1. Process keyword results first (they have priority)
	for _, res := range keyword {
		keywordVerseIDs[res.VerseID] = true
		// Boost keyword scores slightly to ensure they stay on top
		res.Score = res.Score * 1.2 // Boost keyword relevance
		finalResults = append(finalResults, res)
	}

	// 2. Process vector results, skip duplicates
	// For non-duplicates, combine scores intelligently
	vectorResults := make([]SearchResult, 0)
	for _, res := range vector {
		if !keywordVerseIDs[res.VerseID] {
			// Normalize vector score (it's already between 0-1 from cosine distance)
			// Keep original score for ranking
			vectorResults = append(vectorResults, res)
		}
	}

	// 3. Sort vector results by score (they're already sorted, but ensure it)
	sort.Slice(vectorResults, func(i, j int) bool {
		return vectorResults[i].Score > vectorResults[j].Score
	})

	// 4. Add vector results after keyword results
	finalResults = append(finalResults, vectorResults...)

	// 5. Apply limit
	if len(finalResults) > limit {
		finalResults = finalResults[:limit]
	}

	return finalResults, nil
}
