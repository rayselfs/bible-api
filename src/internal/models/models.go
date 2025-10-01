package models

// Versions 對應 versions 資料表，儲存聖經版本資訊
type Versions struct {
	ID    uint    `gorm:"primaryKey" json:"id"`
	Code  string  `gorm:"uniqueIndex;not null;size:20" json:"code"`
	Name  string  `gorm:"not null;size:100" json:"name"`
	Books []Books `gorm:"foreignKey:VersionID;constraint:OnDelete:CASCADE"`
}

// Books 對應 books 資料表，儲存聖經書卷資訊
type Books struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	VersionID    uint       `gorm:"not null;index" json:"version_id"`
	Number       uint       `gorm:"not null;index" json:"number"`
	Name         string     `gorm:"not null;size:100" json:"name"`
	Abbreviation string     `gorm:"not null;size:20" json:"abbreviation"`
	Version      Versions   `gorm:"foreignKey:VersionID;constraint:OnDelete:CASCADE"`
	Chapters     []Chapters `gorm:"foreignKey:BookID;constraint:OnDelete:CASCADE"`
}

// Chapters 對應 chapters 資料表，儲存每一章的獨立資訊
type Chapters struct {
	ID     uint     `gorm:"primaryKey" json:"id"`
	BookID uint     `gorm:"not null;index" json:"book_id"`
	Number uint     `gorm:"not null;index" json:"number"`
	Book   Books    `gorm:"foreignKey:BookID;constraint:OnDelete:CASCADE"`
	Verses []Verses `gorm:"foreignKey:ChapterID;constraint:OnDelete:CASCADE"`
}

// Verses 對應 verses 資料表，儲存每一節的經文內容
type Verses struct {
	ID        uint     `gorm:"primaryKey" json:"id"`
	ChapterID uint     `gorm:"not null;index" json:"chapter_id"`
	Number    int      `gorm:"not null;index" json:"number"`
	Text      string   `gorm:"type:text;not null" json:"text"`
	Chapter   Chapters `gorm:"foreignKey:ChapterID;constraint:OnDelete:CASCADE"`
}

// VersionListItem 是版本列表項目
type VersionListItem struct {
	ID   uint   `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// BibleContentAPI 是取得全部經文的 API 回應結構
type BibleContentAPI struct {
	VersionID   uint               `json:"version_id"`
	VersionCode string             `json:"version_code"`
	VersionName string             `json:"version_name"`
	Books       []BibleContentBook `json:"books"`
}

// BibleContentBook 是經文內容中的書卷結構
type BibleContentBook struct {
	ID           uint                  `json:"id"`
	Number       uint                  `json:"number"`
	Name         string                `json:"name"`
	Abbreviation string                `json:"abbreviation"`
	Chapters     []BibleContentChapter `json:"chapters"`
}

// BibleContentChapter 是經文內容中的章節結構
type BibleContentChapter struct {
	ID     uint                `json:"id"`
	Number uint                `json:"number"`
	Verses []BibleContentVerse `json:"verses"`
}

// BibleContentVerse 是經文內容中的經節結構
type BibleContentVerse struct {
	ID     uint   `json:"id"`
	Number int    `json:"number"`
	Text   string `json:"text"`
}
