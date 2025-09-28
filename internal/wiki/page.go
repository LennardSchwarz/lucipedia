package wiki

import "gorm.io/gorm"

// Page represents a Lucipedia entry persisted in the database.
type Page struct {
	gorm.Model
	Slug      string `gorm:"size:255;uniqueIndex:idx_pages_slug;not null"`
	HTML      string `gorm:"type:text;not null"`
	Embedding []byte `gorm:"type:blob"`
}

// TableName defines the table name for the Page model.
func (Page) TableName() string {
	return "pages"
}
