package wiki

import (
	"time"

	"gorm.io/gorm"
)

// Page represents a Wikipedai entry persisted in the database.
type Page struct {
	gorm.Model                     // https://gorm.io/docs/models.html#gorm-Model
	Slug      string    `gorm:"index;size:255"`
	HTML      string    `gorm:"type:text;not null"`
	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

// TableName defines the table name for the Page model.
func (Page) TableName() string {
	return "pages"
}
