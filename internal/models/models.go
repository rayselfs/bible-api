package models

// Versions corresponds to versions table, stores Bible version information
type Versions struct {
	ID    uint    `gorm:"primaryKey" json:"id"`
	Code  string  `gorm:"uniqueIndex;not null;size:20" json:"code"`
	Name  string  `gorm:"not null;size:100" json:"name"`
	Books []Books `gorm:"foreignKey:VersionID;constraint:OnDelete:CASCADE"`
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

// VersionListItem is a version list item
type VersionListItem struct {
	ID   uint   `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
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

// --- 新增：Azure AI Search 相關模型 ---

// AISearchRequest 是傳送給 AI Search 的混合搜尋請求
type AISearchRequest struct {
	Search        string        `json:"search,omitempty"` // 關鍵字
	VectorQueries []VectorQuery `json:"vectorQueries"`    // 向量
	Filter        string        `json:"filter,omitempty"`
	Top           int           `json:"top"`
	Select        string        `json:"select"`
}

// VectorQuery 是向量搜尋的具體內容
type VectorQuery struct {
	Vector []float64 `json:"vector"`
	Fields string    `json:"fields"`
	K      int       `json:"k"`
}

// AISearchResponse 是 AI Search 傳回的回應
type AISearchResponse struct {
	Value []AISearchResult `json:"value"`
}

// AISearchResult 是 AI Search 傳回的單筆文件
// (欄位名稱必須匹配您在 index 中定義的)
type AISearchResult struct {
	Score         float64 `json:"@search.score"` // AI Search 產生的相關性分數
	VerseID       string  `json:"verse_id"`
	VersionCode   string  `json:"version_code"`
	BookNumber    int     `json:"book_number"`
	ChapterNumber int     `json:"chapter_number"`
	VerseNumber   int     `json:"verse_number"`
	Text          string  `json:"text"`
}

// SearchRequest represents the search request payload
type SearchRequest struct {
	Query     string `json:"query" binding:"required" example:"愛"`
	VersionID *uint  `json:"version_id,omitempty" example:"1"`
	Limit     *int   `json:"limit,omitempty" example:"10"`
}

// SearchResult represents a single search result
type SearchResult struct {
	VerseID       string  `json:"verse_id"`
	VersionCode   string  `json:"version_code"`
	BookNumber    uint    `json:"book_number"`
	ChapterNumber uint    `json:"chapter_number"`
	VerseNumber   uint    `json:"verse_number"`
	Text          string  `json:"text"`
	Score         float64 `json:"score"`
}

// SearchResponse represents the search response
type SearchResponse struct {
	Query   string         `json:"query"`
	Results []SearchResult `json:"results"`
	Total   int            `json:"total"`
}
