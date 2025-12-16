package models

import (
	"time"

	"github.com/pgvector/pgvector-go"
)

// Versions corresponds to versions table, stores Bible version information
type Versions struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Code      string    `gorm:"uniqueIndex;not null;size:20" json:"code"`
	Name      string    `gorm:"not null;size:100" json:"name"`
	UpdatedAt time.Time `json:"updated_at"`
	Books     []Books   `gorm:"foreignKey:VersionID;constraint:OnDelete:CASCADE"`
}

// Books corresponds to books table, stores Bible book information
type Books struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	VersionID    uint       `gorm:"not null;index" json:"version_id"`
	Number       uint       `gorm:"not null;index" json:"number"`
	Name         string     `gorm:"not null;size:100" json:"name"`
	Abbreviation string     `gorm:"not null;size:20" json:"abbreviation"`
	Version      Versions   `gorm:"foreignKey:VersionID;constraint:OnDelete:CASCADE"`
	Chapters     []Chapters `gorm:"foreignKey:BookID;constraint:OnDelete:CASCADE"`
}

// Chapters corresponds to chapters table, stores independent information for each chapter
type Chapters struct {
	ID     uint     `gorm:"primaryKey" json:"id"`
	BookID uint     `gorm:"not null;index" json:"book_id"`
	Number uint     `gorm:"not null;index" json:"number"`
	Book   Books    `gorm:"foreignKey:BookID;constraint:OnDelete:CASCADE"`
	Verses []Verses `gorm:"foreignKey:ChapterID;constraint:OnDelete:CASCADE"`
}

// Verses corresponds to verses table, stores verse content for each verse
type Verses struct {
	ID        uint     `gorm:"primaryKey" json:"id"`
	ChapterID uint     `gorm:"not null;index" json:"chapter_id"`
	Number    int      `gorm:"not null;index" json:"number"`
	Text      string   `gorm:"type:text;not null" json:"text"`
	Chapter   Chapters `gorm:"foreignKey:ChapterID;constraint:OnDelete:CASCADE"`
}

// BibleVectors corresponds to bible_vectors table, stores embeddings
type BibleVectors struct {
	ID        uint            `gorm:"primaryKey" json:"id"`
	VerseID   uint            `gorm:"not null;index" json:"verse_id"`
	Embedding pgvector.Vector `gorm:"type:vector(1536)" json:"embedding"`
}

// VersionListItem is a version list item
type VersionListItem struct {
	ID        uint   `json:"id"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	UpdatedAt int64  `json:"updated_at"`
}

// BibleContentAPI is the API response structure for getting complete Bible content
type BibleContentAPI struct {
	VersionID   uint               `json:"version_id"`
	VersionCode string             `json:"version_code"`
	VersionName string             `json:"version_name"`
	Books       []BibleContentBook `json:"books"`
}

// BibleContentBook is the book structure in Bible content
type BibleContentBook struct {
	ID           uint                  `json:"id"`
	Number       uint                  `json:"number"`
	Name         string                `json:"name"`
	Abbreviation string                `json:"abbreviation"`
	Chapters     []BibleContentChapter `json:"chapters"`
}

// BibleContentChapter is the chapter structure in Bible content
type BibleContentChapter struct {
	ID     uint                `json:"id"`
	Number uint                `json:"number"`
	Verses []BibleContentVerse `json:"verses"`
}

// BibleContentVerse is the verse structure in Bible content
type BibleContentVerse struct {
	ID     uint   `json:"id"`
	Number int    `json:"number"`
	Text   string `json:"text"`
}

// SearchResult represents a single search result
type SearchResult struct {
	Score         float64 `json:"score"`
	VerseID       string  `json:"verse_id"`
	VersionCode   string  `json:"version_code"`
	BookNumber    uint    `json:"book_number"`
	ChapterNumber uint    `json:"chapter_number"`
	VerseNumber   uint    `json:"verse_number"`
	Text          string  `json:"text"`
}
