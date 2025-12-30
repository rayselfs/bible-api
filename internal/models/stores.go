package models

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hhc/bible-api/internal/utils"
	"math"
	"slices"

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

// StreamVectorsForVersion streams vector data for a specific version
// Format: Binary stream of [VerseID (uint32) + Vector (384 * float32)]
func (s *Store) StreamVectorsForVersion(c *gin.Context, ctx context.Context, versionID uint) (<-chan []byte, <-chan error) {
	contentChan := make(chan []byte, 50)
	errorChan := make(chan error, 1)

	go func() {
		defer close(contentChan)
		defer close(errorChan)

		// 0. Fetch Version to check permissions
		var version Versions
		if err := s.DB.First(&version, versionID).Error; err != nil {
			errorChan <- fmt.Errorf("version not found: %w", err)
			return
		}

		// Validate version access
		if err := validateVersionAccess(c, version.Code); err != nil {
			errorChan <- err
			return
		}

		// 1. Get IDs of books belonging to this version
		var bookIDs []uint
		if err := s.DB.Model(&Books{}).Where("version_id = ?", versionID).Pluck("id", &bookIDs).Error; err != nil {
			errorChan <- fmt.Errorf("failed to fetch books: %w", err)
			return
		}

		if len(bookIDs) == 0 {
			return
		}

		// 2. Query vectors
		rows, err := s.DB.Table("bible_vectors").
			Select("bible_vectors.verse_id, bible_vectors.embedding").
			Joins("JOIN verses ON bible_vectors.verse_id = verses.id").
			Joins("JOIN chapters ON verses.chapter_id = chapters.id").
			Where("chapters.book_id IN ?", bookIDs).
			Order("bible_vectors.verse_id ASC").
			Rows()

		if err != nil {
			errorChan <- fmt.Errorf("failed to query vectors: %w", err)
			return
		}
		defer rows.Close()

		// Buffer for batching 100 verses (~150KB)
		// 1 verse = 4 bytes (ID) + 384 * 4 bytes (Vector) = 1540 bytes
		const vectorDim = 384
		const bytesPerVerse = 4 + (vectorDim * 4)
		const batchSize = 100

		buffer := make([]byte, 0, batchSize*bytesPerVerse)
		count := 0

		for rows.Next() {
			var verseID uint32
			var vec pgvector.Vector

			if err := rows.Scan(&verseID, &vec); err != nil {
				errorChan <- fmt.Errorf("scan error: %w", err)
				return
			}

			if len(vec.Slice()) != vectorDim {
				// verify dimension to avoid corruption
				// Skip or error? Error is safer.
				errorChan <- fmt.Errorf("vector dimension mismatch: expected %d, got %d", vectorDim, len(vec.Slice()))
				return
			}

			// Append VerseID (uint32 LittleEndian)
			idBytes := make([]byte, 4)
			binary.LittleEndian.PutUint32(idBytes, verseID)
			buffer = append(buffer, idBytes...)

			// Append Vector (float32 LittleEndian)
			for _, v := range vec.Slice() {
				bits := math.Float32bits(v)
				floatBytes := make([]byte, 4)
				binary.LittleEndian.PutUint32(floatBytes, bits)
				buffer = append(buffer, floatBytes...)
			}

			count++
			if count >= batchSize {
				// Flush buffer
				out := make([]byte, len(buffer))
				copy(out, buffer)
				contentChan <- out
				buffer = buffer[:0]
				count = 0
			}

			// Check context cancellation
			select {
			case <-ctx.Done():
				return
			default:
			}
		}

		// Flush remaining buffer
		if len(buffer) > 0 {
			contentChan <- buffer
		}
	}()

	return contentChan, errorChan
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

// mergeResults merges the vector and keyword results using RRF (Reciprocal Rank Fusion)
// Strategy:
// 1. Keyword results have priority and are placed first
// 2. Remove duplicates from vector results (if already in keyword)
// 3. For remaining results, use combined scoring to rank them intelligently

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
