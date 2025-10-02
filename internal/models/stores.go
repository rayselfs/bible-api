package models

import (
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

// GetBibleContent Get complete Bible content by version ID
func (s *Store) GetBibleContent(versionID uint) (*BibleContentAPI, error) {
	var version Versions
	var books []Books

	// First get version information
	if err := s.DB.Where(&Versions{ID: versionID}).First(&version).Error; err != nil {
		return nil, err
	}

	// Get all books, chapters and verses for this version
	err := s.DB.Preload("Chapters.Verses").Where(&Books{VersionID: version.ID}).Order("number ASC").Find(&books).Error
	if err != nil {
		return nil, err
	}

	// Convert to API response format
	bibleBooks := make([]BibleContentBook, len(books))
	for i, book := range books {
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

		bibleBooks[i] = BibleContentBook{
			ID:           book.ID,
			Number:       book.Number,
			Name:         book.Name,
			Abbreviation: book.Abbreviation,
			Chapters:     chapters,
		}
	}

	return &BibleContentAPI{
		VersionID:   version.ID,
		VersionCode: version.Code,
		VersionName: version.Name,
		Books:       bibleBooks,
	}, nil
}

// SearchDatabase Search for exact matches in database
func (s *Store) SearchDatabase(query string, versionFilter string) ([]SearchResult, error) {
	// Use LIKE query for substring matching
	var verses []Verses
	var err error

	if versionFilter != "" {
		// If version filter is specified, first find version ID
		var version Versions
		if err := s.DB.Where("code = ?", versionFilter).First(&version).Error; err != nil {
			return nil, fmt.Errorf("version not found: %s", versionFilter)
		}

		// Search in specified version
		err = s.DB.Joins("JOIN chapters ON verses.chapter_id = chapters.id").
			Joins("JOIN books ON chapters.book_id = books.id").
			Joins("JOIN versions ON books.version_id = versions.id").
			Where("verses.text LIKE ? AND versions.id = ?", "%"+query+"%", version.ID).
			Preload("Chapter.Book.Version").
			Find(&verses).Error
	} else {
		// Search in all versions
		err = s.DB.Joins("JOIN chapters ON verses.chapter_id = chapters.id").
			Joins("JOIN books ON chapters.book_id = books.id").
			Joins("JOIN versions ON books.version_id = versions.id").
			Where("verses.text LIKE ?", "%"+query+"%").
			Preload("Chapter.Book.Version").
			Find(&verses).Error
	}

	if err != nil {
		return nil, err
	}

	// Convert to SearchResult format
	var results []SearchResult
	for _, verse := range verses {
		results = append(results, SearchResult{
			Text:    verse.Text,
			Version: verse.Chapter.Book.Version.Code,
			Book:    int(verse.Chapter.Book.Number),
			Chapter: int(verse.Chapter.Number),
			Verse:   int(verse.Number),
		})
	}

	return results, nil
}
