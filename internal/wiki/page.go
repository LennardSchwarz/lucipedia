package wiki

import "time"

// Page represents a Wikipedai entry persisted in the database.
type Page struct {
	Slug      string    `gorm:"primaryKey;size:255"`
	HTML      string    `gorm:"type:text;not null"`
	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

// TableName defines the table name for the Page model.
func (Page) TableName() string {
	return "pages"
}
