package models

import (
	"context"
	"encoding/json"
	"fmt"
	"hhc/bible-api/internal/utils"
	"slices"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/pgvector/pgvector-go"
	"gorm.io/gorm"
)

const (
	// PermissionBibleRead is the permission required to read all Bible versions
	PermissionBibleRead = "bible:read"
)

var (
	// publicVersionList is the list of public Bible version codes accessible without permission
	publicVersionList = []string{"CUNP-TC", "CUNP-SC", "KJV"}
)

// Store contains a *gorm.DB instance
type Store struct {
	DB *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{DB: db}
}

// checkPermission checks if the user has the specified permission from gin context
func checkPermission(c *gin.Context, permission string) bool {
	permissionsStr, exists := c.Get("permissions")
	if !exists {
		return false
	}

	permissions, ok := permissionsStr.(string)
	if !ok {
		return false
	}

	return utils.HasPermission(permissions, permission)
}

// canAccessVersion checks if the user can access the given version code
// Returns true if user has permission or version is in public list
func canAccessVersion(c *gin.Context, versionCode string) bool {
	hasPermission := checkPermission(c, PermissionBibleRead)
	if hasPermission {
		return true
	}
	return slices.Contains(publicVersionList, versionCode)
}

// validateVersionAccess validates if user can access a version, returns error if not
func validateVersionAccess(c *gin.Context, versionCode string) error {
	if !canAccessVersion(c, versionCode) {
		return fmt.Errorf("forbidden: access denied for version %s", versionCode)
	}
	return nil
}

// GetAllVersions returns all Bible versions, filtered by permission
func (s *Store) GetAllVersions(c *gin.Context) ([]VersionListItem, error) {
	hasPermission := checkPermission(c, PermissionBibleRead)

	query := s.DB
	if !hasPermission {
		// If no permission, only return public versions
		query = query.Where("code IN ?", publicVersionList)
	}

	var versions []Versions
	if err := query.Find(&versions).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch versions: %w", err)
	}

	// Convert to API response format
	versionList := make([]VersionListItem, len(versions))
	for i, version := range versions {
		versionList[i] = VersionListItem{
			ID:        version.ID,
			Code:      version.Code,
			Name:      version.Name,
			UpdatedAt: version.UpdatedAt.Unix(),
		}
	}

	return versionList, nil
}

// StreamBibleContent streams Bible content by version ID using channels
// This method returns a channel that yields Bible books one by one for streaming
func (s *Store) StreamBibleContent(c *gin.Context, ctx context.Context, versionID uint) (<-chan []byte, <-chan error) {
	contentChan := make(chan []byte, 10) // Buffer for better performance
	errorChan := make(chan error, 1)

	go func() {
		defer close(contentChan)
		defer close(errorChan)

		// Get version information first
		var version Versions
		if err := s.DB.WithContext(ctx).Where(&Versions{ID: versionID}).First(&version).Error; err != nil {
			errorChan <- fmt.Errorf("failed to fetch version: %w", err)
			return
		}

		// Validate version access
		if err := validateVersionAccess(c, version.Code); err != nil {
			errorChan <- err
			return
		}

		// Send version header
		versionHeader := map[string]interface{}{
			"version_id":   version.ID,
			"version_code": version.Code,
			"version_name": version.Name,
			"updated_at":   version.UpdatedAt.Unix(),
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
			errorChan <- fmt.Errorf("failed to fetch books: %w", err)
			return
		}

		for _, book := range books {
			select {
			case <-ctx.Done():
				errorChan <- ctx.Err()
				return
			default:
			}

			bookData := s.convertBookToAPIFormat(book)
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

// convertBookToAPIFormat converts a Books model to BibleContentBook API format
func (s *Store) convertBookToAPIFormat(book Books) BibleContentBook {
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

	return BibleContentBook{
		ID:           book.ID,
		Number:       book.Number,
		Name:         book.Name,
		Abbreviation: book.Abbreviation,
		Chapters:     chapters,
	}
}

// SearchVerses performs hybrid search using pgvector and tsvector
// Logic: Split into two queries (Vector + Keyword) and merge in backend using RRF
func (s *Store) SearchVerses(c *gin.Context, ctx context.Context, query string, embedding []float32, versionFilter string, limit int) ([]SearchResult, error) {
	// Validate version access
	if err := validateVersionAccess(c, versionFilter); err != nil {
		return nil, err
	}

	// Calculate keyword search limit (use smaller limit to avoid too many results)
	// For common single words, we want fewer but more relevant keyword results
	keywordLimit := limit
	if len(query) <= 3 {
		// For very short queries (likely common words), use smaller limit
		keywordLimit = max(limit/2, 5)
	}

	// 1. Vector Search
	vectorResults, err := s.performVectorSearch(ctx, embedding, versionFilter, limit)
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	// 2. Keyword Search with minimum score threshold
	keywordResults, err := s.performKeywordSearch(ctx, query, versionFilter, keywordLimit)
	if err != nil {
		return nil, fmt.Errorf("keyword search failed: %w", err)
	}

	// 3. Merge Results with intelligent scoring
	return s.mergeResults(vectorResults, keywordResults, limit)
}

// performVectorSearch executes vector similarity search
func (s *Store) performVectorSearch(ctx context.Context, embedding []float32, versionFilter string, limit int) ([]SearchResult, error) {
	var results []SearchResult
	vectorSQL := `
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

	if err := s.DB.WithContext(ctx).Raw(vectorSQL,
		pgvector.NewVector(embedding), versionFilter, pgvector.NewVector(embedding), limit,
	).Scan(&results).Error; err != nil {
		return nil, err
	}

	return results, nil
}

// performKeywordSearch executes keyword-based full-text search
func (s *Store) performKeywordSearch(ctx context.Context, query string, versionFilter string, limit int) ([]SearchResult, error) {
	var results []SearchResult
	keywordSQL := `
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

	if err := s.DB.WithContext(ctx).Raw(keywordSQL,
		query, versionFilter, query, query, limit,
	).Scan(&results).Error; err != nil {
		return nil, err
	}

	return results, nil
}

// mergeResults merges the vector and keyword results using RRF (Reciprocal Rank Fusion)
// Strategy:
// 1. Keyword results have priority and are placed first
// 2. Remove duplicates from vector results (if already in keyword)
// 3. For remaining results, use combined scoring to rank them intelligently
func (s *Store) mergeResults(vector, keyword []SearchResult, limit int) ([]SearchResult, error) {
	keywordVerseIDs := make(map[string]bool)
	finalResults := make([]SearchResult, 0, limit)

	// 1. Process keyword results first (they have priority)
	for _, res := range keyword {
		keywordVerseIDs[res.VerseID] = true
		// Boost keyword scores slightly to ensure they stay on top
		res.Score = res.Score * 1.2
		finalResults = append(finalResults, res)
	}

	// 2. Process vector results, skip duplicates
	vectorResults := make([]SearchResult, 0)
	for _, res := range vector {
		if !keywordVerseIDs[res.VerseID] {
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

// UpdateVerse updates a verse text and its embedding, and updates the parent version's UpdatedAt
func (s *Store) UpdateVerse(c *gin.Context, ctx context.Context, verseID uint, text string, embedding []float32) error {
	// Begin transaction
	tx := s.DB.WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 1. Update Verse Text
	if err := tx.Model(&Verses{}).Where("id = ?", verseID).Update("text", text).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to update verse text: %w", err)
	}

	// 2. Update Vector Embedding
	if err := tx.Model(&BibleVectors{}).Where("verse_id = ?", verseID).
		Update("embedding", pgvector.NewVector(embedding)).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to update vector embedding: %w", err)
	}

	// 3. Find Version ID from Verse -> Chapter -> Book -> Version
	var result struct {
		VersionID uint
	}
	query := `
		SELECT b.version_id 
		FROM verses v
		JOIN chapters c ON v.chapter_id = c.id
		JOIN books b ON c.book_id = b.id
		WHERE v.id = ?
	`
	if err := tx.Raw(query, verseID).Scan(&result).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to find version for verse %d: %w", verseID, err)
	}

	// 4. Update Version UpdatedAt
	if err := tx.Model(&Versions{}).Where("id = ?", result.VersionID).
		Update("updated_at", gorm.Expr("CURRENT_TIMESTAMP")).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to update version timestamp: %w", err)
	}

	// Commit
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
