package models

import (
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
