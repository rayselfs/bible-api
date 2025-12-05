package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"hhc/bible-api/internal/models"
	"hhc/bible-api/internal/pkg/openai"

	"github.com/pgvector/pgvector-go"
	"gorm.io/gorm"
)

// JSONBibleData represents the JSON file structure
type JSONBibleData struct {
	Version struct {
		Code string `json:"code"`
		Name string `json:"name"`
	} `json:"version"`
	Books []JSONBook `json:"books"`
}

type JSONBook struct {
	Number       uint          `json:"number"`
	Name         string        `json:"name"`
	Abbreviation string        `json:"abbreviation"`
	Chapters     []JSONChapter `json:"chapters"`
}

type JSONChapter struct {
	Number uint        `json:"number"`
	Verses []JSONVerse `json:"verses"`
}

type JSONVerse struct {
	Number int    `json:"number"`
	Text   string `json:"text"`
}

// Run executes the Bible data import
func Run(db *gorm.DB, openAIService *openai.OpenAIService, filePath string) error {
	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", filePath)
	}

	// Read JSON file
	fmt.Printf("Reading file: %s\n", filePath)
	jsonData, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %v", err)
	}

	// Parse JSON
	var bibleData JSONBibleData
	if err := json.Unmarshal(jsonData, &bibleData); err != nil {
		return fmt.Errorf("failed to parse JSON: %v", err)
	}

	fmt.Printf("Successfully read JSON file\n")
	fmt.Printf("Version: %s (%s)\n", bibleData.Version.Name, bibleData.Version.Code)
	fmt.Printf("Books: %d\n", len(bibleData.Books))

	// Start import
	if err := importBibleData(db, openAIService, &bibleData); err != nil {
		return fmt.Errorf("import failed: %v", err)
	}

	fmt.Println("🎉 Import completed!")
	return nil
}

func importBibleData(db *gorm.DB, openAIService *openai.OpenAIService, data *JSONBibleData) error {
	// Begin transaction
	tx := db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %v", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 1. Create version
	fmt.Printf("Creating version: %s", data.Version.Name)
	version := models.Versions{
		Code: data.Version.Code,
		Name: data.Version.Name,
	}

	// Check if version already exists
	var existingVersion models.Versions
	if err := tx.Where("code = ?", version.Code).First(&existingVersion).Error; err == nil {
		fmt.Printf("Version %s already exists", version.Code)
		version = existingVersion
	} else {
		if err := tx.Create(&version).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to create version: %v", err)
		}
	}

	fmt.Printf("Version created successfully (ID: %d)\n", version.ID)

	// 2. Import books
	totalBooks := len(data.Books)
	totalChapters := 0
	totalVerses := 0
	totalVectors := 0

	ctx := context.Background()

	for i, bookData := range data.Books {
		fmt.Printf("\nImporting book %d/%d: %s\n", i+1, totalBooks, bookData.Name)

		book := models.Books{
			VersionID:    version.ID,
			Number:       bookData.Number,
			Name:         bookData.Name,
			Abbreviation: bookData.Abbreviation,
		}

		if err := tx.Create(&book).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to create book %s: %v", bookData.Name, err)
		}

		bookVerseCount := 0
		bookVectorCount := 0

		// 3. Import chapters
		for _, chapterData := range bookData.Chapters {
			chapter := models.Chapters{
				BookID: book.ID,
				Number: chapterData.Number,
			}

			if err := tx.Create(&chapter).Error; err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to create chapter %s %d: %v", bookData.Name, chapterData.Number, err)
			}

			totalChapters++

			// 4. Import verses with embeddings
			for _, verseData := range chapterData.Verses {
				verse := models.Verses{
					ChapterID: chapter.ID,
					Number:    verseData.Number,
					Text:      verseData.Text,
				}

				if err := tx.Create(&verse).Error; err != nil {
					tx.Rollback()
					return fmt.Errorf("failed to create verse %s %d:%d: %v", bookData.Name, chapterData.Number, verseData.Number, err)
				}

				totalVerses++
				bookVerseCount++

				// 5. Generate and store embedding
				embedding64, err := openAIService.GetEmbedding(ctx, verseData.Text)
				if err != nil {
					fmt.Printf("\n  [ERROR] Failed to get embedding for %s %d:%d: %v", bookData.Name, chapterData.Number, verseData.Number, err)
					// Continue without embedding, don't fail the entire import
					continue
				}

				// Convert []float64 to []float32 for pgvector
				embedding32 := make([]float32, len(embedding64))
				for j, v := range embedding64 {
					embedding32[j] = float32(v)
				}

				bibleVector := models.BibleVectors{
					VerseID:   verse.ID,
					Embedding: pgvector.NewVector(embedding32),
				}

				if err := tx.Create(&bibleVector).Error; err != nil {
					fmt.Printf("\n  [ERROR] Failed to store embedding for %s %d:%d: %v", bookData.Name, chapterData.Number, verseData.Number, err)
					continue
				}

				totalVectors++
				bookVectorCount++

				// Progress indicator every 10 verses
				if bookVerseCount%10 == 0 {
					fmt.Printf("\r  Progress: %d verses, %d vectors", bookVerseCount, bookVectorCount)
				}

				// Rate limiting: avoid hitting API rate limits
				time.Sleep(20 * time.Millisecond)
			}
		}
		fmt.Printf("\r  Completed: %d verses, %d vectors\n", bookVerseCount, bookVectorCount)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	fmt.Printf("Import Statistics:")
	fmt.Printf("Version: %s (%s)", version.Name, version.Code)
	fmt.Printf("Books: %d", totalBooks)
	fmt.Printf("Chapters: %d", totalChapters)
	fmt.Printf("Verses: %d", totalVerses)
	fmt.Printf("Vectors: %d", totalVectors)

	return nil
}

// PrintUsage prints the import command usage
func PrintUsage() {
	fmt.Println("Bible Data Import Tool")
	fmt.Println("Usage: ./app import <JSON_FILE_PATH>")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  ./app import data/bible.json")
	fmt.Println("  ./app import data/bible_simplified.json")
}
