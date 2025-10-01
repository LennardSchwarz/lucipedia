package wiki

import "gorm.io/gorm"

// PageRecord represents a Lucipedia entry persisted in the database.
type PageRecord struct {
	gorm.Model
	Slug string `gorm:"size:255;uniqueIndex:idx_pages_slug;not null"`
	HTML string `gorm:"type:text;not null"`
}

// TableName defines the table name for the Page model.
func (PageRecord) TableName() string {
	return "pages"
}
