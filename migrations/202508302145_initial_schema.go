package migrations

import (
	"hhc/bible-api/internal/models"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// InitialSchema 是一個包含 ID 和遷移函式的結構
var InitialSchema = &gormigrate.Migration{
	// ID 必須是唯一的，通常使用時間戳
	ID: "202508302145_INITIAL_SCHEMA",

	// Migrate 是升級函式
	Migrate: func(tx *gorm.DB) error {
		// 使用 GORM 的 AutoMigrate 建立初始資料表
		// 注意順序：先建立父表，再建立子表
		return tx.AutoMigrate(
			&models.Versions{},
			&models.Books{},
			&models.Chapters{},
			&models.Verses{},
		)
	},

	// Rollback 是降級函式
	Rollback: func(tx *gorm.DB) error {
		// DropTable 接受 interface{}，所以我們傳入模型的指標
		// 注意 Drop 的順序與 Migrate 的順序相反，以處理外鍵約束
		return tx.Migrator().DropTable(
			&models.Verses{},
			&models.Chapters{},
			&models.Books{},
			&models.Versions{},
		)
	},
}
