package models

import (
	"context"
	"encoding/json"
	"fmt"

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
