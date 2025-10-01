package models

import (
	"gorm.io/gorm"
)

// Store 包含一個 *gorm.DB 的實例
type Store struct {
	DB *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{DB: db}
}

// GetAllVersions 獲取所有聖經版本
func (s *Store) GetAllVersions() ([]VersionListItem, error) {
	var versions []Versions
	err := s.DB.Find(&versions).Error
	if err != nil {
		return nil, err
	}

	// 轉換為 API 回應格式
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

// GetBibleContent 透過版本 ID 獲取全部經文內容
func (s *Store) GetBibleContent(versionID uint) (*BibleContentAPI, error) {
	var version Versions
	var books []Books

	// 先取得版本資訊
	if err := s.DB.Where(&Versions{ID: versionID}).First(&version).Error; err != nil {
		return nil, err
	}

	// 取得該版本的所有書卷、章節和經文
	err := s.DB.Preload("Chapters.Verses").Where(&Books{VersionID: version.ID}).Order("number ASC").Find(&books).Error
	if err != nil {
		return nil, err
	}

	// 轉換為 API 回應格式
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
