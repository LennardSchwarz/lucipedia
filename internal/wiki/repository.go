package wiki

import (
	"context"
	"strings"

	"github.com/rotisserie/eris"
	"gorm.io/gorm"
)

// Repository defines persistence operations for wiki pages.
type Repository interface {
	GetBySlug(ctx context.Context, slug string) (*Page, error)
	CreateOrUpdate(ctx context.Context, page *Page) error
}

// GormRepository persists pages using a Gorm database connection.
type GormRepository struct {
	db *gorm.DB
}

// NewRepository constructs a Gorm-backed repository implementation.
func NewRepository(db *gorm.DB) (*GormRepository, error) {
	if db == nil {
		return nil, eris.New("gorm DB is required")
	}

	return &GormRepository{db: db}, nil
}

var _ Repository = (*GormRepository)(nil)

// GetBySlug returns the page for the provided slug or nil when not found.
func (r *GormRepository) GetBySlug(ctx context.Context, slug string) (*Page, error) {
	trimmed := strings.TrimSpace(slug)
	if trimmed == "" {
		return nil, eris.New("slug is required")
	}

	var page Page
	err := r.db.WithContext(ctx).First(&page, "slug = ?", trimmed).Error
	if err != nil {
		if eris.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, eris.Wrapf(err, "fetching page by slug: %s", trimmed)
	}

	return &page, nil
}

// CreateOrUpdate stores the wiki page, inserting or updating the row as needed.
func (r *GormRepository) CreateOrUpdate(ctx context.Context, page *Page) error {
	if page == nil {
		return eris.New("page is nil")
	}

	if strings.TrimSpace(page.Slug) == "" {
		return eris.New("page slug is required")
	}

	if err := r.db.WithContext(ctx).Save(page).Error; err != nil {
		return eris.Wrapf(err, "saving page: %s", page.Slug)
	}

	return nil
}
