package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
// If bookNum and chapterNum are both 0, imports the entire file
// Otherwise, imports only the specified book and chapter
func Run(db *gorm.DB, openAIService *openai.OpenAIService, filePath string, bookNum uint, chapterNum uint) error {
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

	// If bookNum and chapterNum are specified, import only that chapter
	if bookNum > 0 && chapterNum > 0 {
		fmt.Printf("Importing book %d, chapter %d only\n", bookNum, chapterNum)
		if err := importSingleChapter(db, openAIService, &bibleData, bookNum, chapterNum); err != nil {
			return fmt.Errorf("import failed: %v", err)
		}
	} else {
		fmt.Printf("Books: %d\n", len(bibleData.Books))
		// Start full import
		if err := importBibleData(db, openAIService, &bibleData); err != nil {
			return fmt.Errorf("import failed: %v", err)
		}
	}

	fmt.Println("🎉 Import completed!")
	return nil
}

// ImportAllFromDataDir scans the specified directory and imports all JSON files
func ImportAllFromDataDir(db *gorm.DB, openAIService *openai.OpenAIService, dataDir string) error {
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		return fmt.Errorf("data directory not found: %s", dataDir)
	}

	fmt.Printf("Scanning directory: %s\n", dataDir)

	// Find all JSON files
	var jsonFiles []string
	err := filepath.Walk(dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(info.Name()), ".json") {
			jsonFiles = append(jsonFiles, path)
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to scan directory: %v", err)
	}

	if len(jsonFiles) == 0 {
		return fmt.Errorf("no JSON files found in %s", dataDir)
	}

	fmt.Printf("Found %d JSON file(s)\n\n", len(jsonFiles))

	// Import each file
	for i, filePath := range jsonFiles {
		fmt.Printf("%s\n", strings.Repeat("=", 60))
		fmt.Printf("[%d/%d] Importing: %s\n", i+1, len(jsonFiles), filePath)
		fmt.Printf("%s\n", strings.Repeat("=", 60))
		fmt.Println()

		if err := Run(db, openAIService, filePath, 0, 0); err != nil {
			fmt.Printf("\n❌ Failed to import %s: %v\n\n", filePath, err)
			continue
		}

		fmt.Println()
	}

	fmt.Println("🎉 All imports completed!")
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
		// Update existing version (name might have changed)
		existingVersion.Name = version.Name
		if err := tx.Save(&existingVersion).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to update version: %v", err)
		}
		version = existingVersion
		fmt.Printf("Updated version: %s (ID: %d)\n", version.Name, version.ID)
	} else {
		if err := tx.Create(&version).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to create version: %v", err)
		}
		fmt.Printf("Created version: %s (ID: %d)\n", version.Name, version.ID)
	}

	fmt.Printf("Version created successfully (ID: %d)\n", version.ID)

	// 2. Import books
	totalBooks := len(data.Books)
	totalChapters := 0
	totalVerses := 0
	totalUpdatedVerses := 0
	totalVectors := 0

	ctx := context.Background()

	for i, bookData := range data.Books {
		fmt.Printf("\nImporting book %d/%d: %s\n", i+1, totalBooks, bookData.Name)

		// Check if book already exists
		var book models.Books
		if err := tx.Where("version_id = ? AND number = ?", version.ID, bookData.Number).First(&book).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				// Create new book
				book = models.Books{
					VersionID:    version.ID,
					Number:       bookData.Number,
					Name:         bookData.Name,
					Abbreviation: bookData.Abbreviation,
				}
				if err := tx.Create(&book).Error; err != nil {
					tx.Rollback()
					return fmt.Errorf("failed to create book %s: %v", bookData.Name, err)
				}
				fmt.Printf("  Created book: %s\n", book.Name)
			} else {
				tx.Rollback()
				return fmt.Errorf("failed to query book %s: %v", bookData.Name, err)
			}
		} else {
			// Update existing book (name or abbreviation might have changed)
			book.Name = bookData.Name
			book.Abbreviation = bookData.Abbreviation
			if err := tx.Save(&book).Error; err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to update book %s: %v", bookData.Name, err)
			}
			fmt.Printf("  Updated book: %s\n", book.Name)
		}

		bookVerseCount := 0
		bookVectorCount := 0
		bookUpdatedCount := 0

		// 3. Import chapters
		for _, chapterData := range bookData.Chapters {
			// Check if chapter already exists
			var chapter models.Chapters
			isNewChapter := false
			if err := tx.Where("book_id = ? AND number = ?", book.ID, chapterData.Number).First(&chapter).Error; err != nil {
				if err == gorm.ErrRecordNotFound {
					// Create new chapter
					chapter = models.Chapters{
						BookID: book.ID,
						Number: chapterData.Number,
					}
					if err := tx.Create(&chapter).Error; err != nil {
						tx.Rollback()
						return fmt.Errorf("failed to create chapter %s %d: %v", bookData.Name, chapterData.Number, err)
					}
					isNewChapter = true
				} else {
					tx.Rollback()
					return fmt.Errorf("failed to query chapter %s %d: %v", bookData.Name, chapterData.Number, err)
				}
			}
			// Chapter exists, continue to import verses

			if isNewChapter {
				totalChapters++
			}

			// 4. Import verses with embeddings
			for _, verseData := range chapterData.Verses {
				// Check if verse already exists
				var verse models.Verses
				var isNewVerse bool
				if err := tx.Where("chapter_id = ? AND number = ?", chapter.ID, verseData.Number).First(&verse).Error; err != nil {
					if err == gorm.ErrRecordNotFound {
						// Create new verse
						verse = models.Verses{
							ChapterID: chapter.ID,
							Number:    verseData.Number,
							Text:      verseData.Text,
						}
						if err := tx.Create(&verse).Error; err != nil {
							tx.Rollback()
							return fmt.Errorf("failed to create verse %s %d:%d: %v", bookData.Name, chapterData.Number, verseData.Number, err)
						}
						isNewVerse = true
					} else {
						tx.Rollback()
						return fmt.Errorf("failed to query verse %s %d:%d: %v", bookData.Name, chapterData.Number, verseData.Number, err)
					}
				} else {
					// Update existing verse
					verse.Text = verseData.Text
					if err := tx.Save(&verse).Error; err != nil {
						tx.Rollback()
						return fmt.Errorf("failed to update verse %s %d:%d: %v", bookData.Name, chapterData.Number, verseData.Number, err)
					}
					isNewVerse = false
					bookUpdatedCount++
					// Note: Vector will be updated/created in the embedding section below
				}

				if isNewVerse {
					totalVerses++
				} else {
					totalUpdatedVerses++
				}
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

				// Check if vector already exists
				var existingVector models.BibleVectors
				if err := tx.Where("verse_id = ?", verse.ID).First(&existingVector).Error; err != nil {
					if err == gorm.ErrRecordNotFound {
						// Create new vector
						bibleVector := models.BibleVectors{
							VerseID:   verse.ID,
							Embedding: pgvector.NewVector(embedding32),
						}
						if err := tx.Create(&bibleVector).Error; err != nil {
							fmt.Printf("\n  [ERROR] Failed to store embedding for %s %d:%d: %v", bookData.Name, chapterData.Number, verseData.Number, err)
							continue
						}
					} else {
						fmt.Printf("\n  [ERROR] Failed to check existing vector for %s %d:%d: %v", bookData.Name, chapterData.Number, verseData.Number, err)
						continue
					}
				} else {
					// Update existing vector
					existingVector.Embedding = pgvector.NewVector(embedding32)
					if err := tx.Save(&existingVector).Error; err != nil {
						fmt.Printf("\n  [ERROR] Failed to update embedding for %s %d:%d: %v", bookData.Name, chapterData.Number, verseData.Number, err)
						continue
					}
				}

				totalVectors++
				bookVectorCount++

				// Progress indicator every 10 verses
				if bookVerseCount%10 == 0 {
					if bookUpdatedCount > 0 {
						fmt.Printf("\r  Progress: %d verses (%d new, %d updated), %d vectors", bookVerseCount, bookVerseCount-bookUpdatedCount, bookUpdatedCount, bookVectorCount)
					} else {
						fmt.Printf("\r  Progress: %d verses, %d vectors", bookVerseCount, bookVectorCount)
					}
				}

				// Rate limiting: avoid hitting API rate limits
				time.Sleep(20 * time.Millisecond)
			}
		}
		if bookUpdatedCount > 0 {
			fmt.Printf("\r  Completed: %d verses (%d new, %d updated), %d vectors\n", bookVerseCount, bookVerseCount-bookUpdatedCount, bookUpdatedCount, bookVectorCount)
		} else {
			fmt.Printf("\r  Completed: %d verses, %d vectors\n", bookVerseCount, bookVectorCount)
		}
	}

	// Update Version UpdatedAt before commit
	if err := tx.Model(&models.Versions{}).Where("id = ?", version.ID).
		Update("updated_at", gorm.Expr("CURRENT_TIMESTAMP")).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to update version timestamp: %v", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	fmt.Printf("\nImport Statistics:\n")
	fmt.Printf("  Version: %s (%s)\n", version.Name, version.Code)
	fmt.Printf("  Books: %d\n", totalBooks)
	fmt.Printf("  Chapters: %d\n", totalChapters)
	if totalUpdatedVerses > 0 {
		fmt.Printf("  Verses: %d new, %d updated (total: %d)\n", totalVerses, totalUpdatedVerses, totalVerses+totalUpdatedVerses)
	} else {
		fmt.Printf("  Verses: %d\n", totalVerses)
	}
	fmt.Printf("  Vectors: %d\n", totalVectors)

	return nil
}

// importSingleChapter imports a single chapter from the Bible data
func importSingleChapter(db *gorm.DB, openAIService *openai.OpenAIService, data *JSONBibleData, bookNum uint, chapterNum uint) error {
	// Find the book and chapter in the JSON data
	var targetBook *JSONBook
	var targetChapter *JSONChapter

	for i := range data.Books {
		if data.Books[i].Number == bookNum {
			targetBook = &data.Books[i]
			for j := range targetBook.Chapters {
				if targetBook.Chapters[j].Number == chapterNum {
					targetChapter = &targetBook.Chapters[j]
					break
				}
			}
			break
		}
	}

	if targetBook == nil {
		return fmt.Errorf("book %d not found in JSON data", bookNum)
	}
	if targetChapter == nil {
		return fmt.Errorf("chapter %d not found in book %d", chapterNum, bookNum)
	}

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

	// 1. Get or create version
	var version models.Versions
	if err := tx.Where("code = ?", data.Version.Code).First(&version).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			version = models.Versions{
				Code: data.Version.Code,
				Name: data.Version.Name,
			}
			if err := tx.Create(&version).Error; err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to create version: %v", err)
			}
			fmt.Printf("Created version: %s (ID: %d)\n", version.Name, version.ID)
		} else {
			tx.Rollback()
			return fmt.Errorf("failed to query version: %v", err)
		}
	} else {
		// Update existing version (name might have changed)
		version.Name = data.Version.Name
		if err := tx.Save(&version).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to update version: %v", err)
		}
		fmt.Printf("Updated version: %s (ID: %d)\n", version.Name, version.ID)
	}

	// 2. Get or create book
	var book models.Books
	if err := tx.Where("version_id = ? AND number = ?", version.ID, targetBook.Number).First(&book).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			book = models.Books{
				VersionID:    version.ID,
				Number:       targetBook.Number,
				Name:         targetBook.Name,
				Abbreviation: targetBook.Abbreviation,
			}
			if err := tx.Create(&book).Error; err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to create book: %v", err)
			}
			fmt.Printf("Created book: %s (ID: %d)\n", book.Name, book.ID)
		} else {
			tx.Rollback()
			return fmt.Errorf("failed to query book: %v", err)
		}
	} else {
		// Update existing book (name or abbreviation might have changed)
		book.Name = targetBook.Name
		book.Abbreviation = targetBook.Abbreviation
		if err := tx.Save(&book).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to update book: %v", err)
		}
		fmt.Printf("Updated book: %s (ID: %d)\n", book.Name, book.ID)
	}

	// 3. Get or create chapter
	var chapter models.Chapters
	if err := tx.Where("book_id = ? AND number = ?", book.ID, targetChapter.Number).First(&chapter).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			chapter = models.Chapters{
				BookID: book.ID,
				Number: targetChapter.Number,
			}
			if err := tx.Create(&chapter).Error; err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to create chapter: %v", err)
			}
			fmt.Printf("Created chapter: %d (ID: %d)\n", chapter.Number, chapter.ID)
		} else {
			tx.Rollback()
			return fmt.Errorf("failed to query chapter: %v", err)
		}
	} else {
		fmt.Printf("Using existing chapter: %d (ID: %d)\n", chapter.Number, chapter.ID)
	}

	// 4. Import verses with embeddings (using update strategy)
	ctx := context.Background()
	importedVerses := 0
	importedVectors := 0
	updatedVerses := 0

	for _, verseData := range targetChapter.Verses {
		// Check if verse already exists
		var verse models.Verses
		var isNewVerse bool
		if err := tx.Where("chapter_id = ? AND number = ?", chapter.ID, verseData.Number).First(&verse).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				// Create new verse
				verse = models.Verses{
					ChapterID: chapter.ID,
					Number:    verseData.Number,
					Text:      verseData.Text,
				}
				if err := tx.Create(&verse).Error; err != nil {
					tx.Rollback()
					return fmt.Errorf("failed to create verse %s %d:%d: %v", book.Name, chapter.Number, verseData.Number, err)
				}
				isNewVerse = true
			} else {
				tx.Rollback()
				return fmt.Errorf("failed to query verse %s %d:%d: %v", book.Name, chapter.Number, verseData.Number, err)
			}
		} else {
			// Update existing verse
			verse.Text = verseData.Text
			if err := tx.Save(&verse).Error; err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to update verse %s %d:%d: %v", book.Name, chapter.Number, verseData.Number, err)
			}
			isNewVerse = false
			updatedVerses++
		}

		if isNewVerse {
			importedVerses++
		}

		// Generate and store embedding
		embedding64, err := openAIService.GetEmbedding(ctx, verseData.Text)
		if err != nil {
			fmt.Printf("\n  [ERROR] Failed to get embedding for %s %d:%d: %v\n", book.Name, chapter.Number, verseData.Number, err)
			continue
		}

		// Convert []float64 to []float32 for pgvector
		embedding32 := make([]float32, len(embedding64))
		for j, v := range embedding64 {
			embedding32[j] = float32(v)
		}

		// Check if vector already exists
		var existingVector models.BibleVectors
		if err := tx.Where("verse_id = ?", verse.ID).First(&existingVector).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				// Create new vector
				bibleVector := models.BibleVectors{
					VerseID:   verse.ID,
					Embedding: pgvector.NewVector(embedding32),
				}
				if err := tx.Create(&bibleVector).Error; err != nil {
					fmt.Printf("\n  [ERROR] Failed to store embedding for %s %d:%d: %v\n", book.Name, chapter.Number, verseData.Number, err)
					continue
				}
			} else {
				fmt.Printf("\n  [ERROR] Failed to check existing vector for %s %d:%d: %v\n", book.Name, chapter.Number, verseData.Number, err)
				continue
			}
		} else {
			// Update existing vector
			existingVector.Embedding = pgvector.NewVector(embedding32)
			if err := tx.Save(&existingVector).Error; err != nil {
				fmt.Printf("\n  [ERROR] Failed to update embedding for %s %d:%d: %v\n", book.Name, chapter.Number, verseData.Number, err)
				continue
			}
		}

		importedVectors++

		// Progress indicator every 10 verses
		totalProcessed := importedVerses + updatedVerses
		if totalProcessed%10 == 0 {
			if updatedVerses > 0 {
				fmt.Printf("\r  Progress: %d/%d verses (%d new, %d updated), %d vectors", totalProcessed, len(targetChapter.Verses), importedVerses, updatedVerses, importedVectors)
			} else {
				fmt.Printf("\r  Progress: %d/%d verses, %d vectors", totalProcessed, len(targetChapter.Verses), importedVectors)
			}
		}

		// Rate limiting
		time.Sleep(20 * time.Millisecond)
	}

	if updatedVerses > 0 {
		fmt.Printf("\r  Completed: %d verses (%d new, %d updated), %d vectors\n", importedVerses+updatedVerses, importedVerses, updatedVerses, importedVectors)
	} else {
		fmt.Printf("\r  Completed: %d verses, %d vectors\n", importedVerses, importedVectors)
	}

	// Update Version UpdatedAt before commit
	if err := tx.Model(&models.Versions{}).Where("id = ?", version.ID).
		Update("updated_at", gorm.Expr("CURRENT_TIMESTAMP")).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to update version timestamp: %v", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	fmt.Printf("\nImport Statistics:\n")
	fmt.Printf("  Version: %s (%s)\n", version.Name, version.Code)
	fmt.Printf("  Book: %s (%d)\n", book.Name, book.Number)
	fmt.Printf("  Chapter: %d\n", chapter.Number)
	if updatedVerses > 0 {
		fmt.Printf("  Verses: %d new, %d updated (total: %d)\n", importedVerses, updatedVerses, importedVerses+updatedVerses)
	} else {
		fmt.Printf("  Verses: %d\n", importedVerses)
	}
	fmt.Printf("  Vectors: %d\n", importedVectors)

	return nil
}

// PrintUsage prints the import command usage
func PrintUsage() {
	fmt.Println("Bible Data Import Tool")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  ./app import -d <DIRECTORY>                      # Import all JSON files from directory")
	fmt.Println("  ./app import -f <JSON_FILE>                     # Import a single JSON file")
	fmt.Println("  ./app import -f <JSON_FILE> -b <BOOK> -c <CHAPTER>  # Import a single chapter")
	fmt.Println("")
	fmt.Println("Flags:")
	fmt.Println("  -d    Directory path containing JSON files to import")
	fmt.Println("  -f    JSON file path to import")
	fmt.Println("  -b    Book number (required with -c)")
	fmt.Println("  -c    Chapter number (required with -b)")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Println("  ./app import -d ./data")
	fmt.Println("  ./app import -d /path/to/bible/data")
	fmt.Println("  ./app import -f ./data/bible.json")
	fmt.Println("  ./app import -f ./data/bible_niv.json -b 1 -c 1    # Import Genesis chapter 1")
	fmt.Println("  ./app import -f ./data/bible_kjv.json -b 43 -c 3   # Import John chapter 3")
}
