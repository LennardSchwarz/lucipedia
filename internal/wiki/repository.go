package wiki

import (
	"context"
	"strings"

	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// Repository defines persistence operations for wiki pages.
type Repository interface {
	GetBySlug(ctx context.Context, slug string) (*Page, error)
	CreateOrUpdate(ctx context.Context, page *Page) error
	ListPages(ctx context.Context) ([]Page, error)
	CountPages(ctx context.Context) (int64, error)
}

// GormRepository persists pages using a Gorm database connection.
type GormRepository struct {
	db     *gorm.DB
	logger *logrus.Logger
}

// NewRepository constructs a Gorm-backed repository implementation.
func NewRepository(db *gorm.DB, logger *logrus.Logger) (*GormRepository, error) {
	if db == nil {
		return nil, eris.New("gorm DB is required")
	}

	return &GormRepository{db: db, logger: logger}, nil
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
		r.logError(logrus.Fields{"slug": trimmed}, err, "fetching page by slug")
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

	page.Slug = strings.TrimSpace(page.Slug)

	if err := r.db.WithContext(ctx).Save(page).Error; err != nil {
		r.logError(logrus.Fields{"slug": page.Slug}, err, "saving page")
		return eris.Wrapf(err, "saving page: %s", page.Slug)
	}

	return nil
}

// ListPages returns every page ordered by slug.
func (r *GormRepository) ListPages(ctx context.Context) ([]Page, error) {
	var pages []Page

	if err := r.db.WithContext(ctx).Order("slug ASC").Find(&pages).Error; err != nil {
		r.logError(nil, err, "listing pages")
		return nil, eris.Wrap(err, "listing pages")
	}

	return pages, nil
}

// CountPages returns the total number of persisted wiki pages.
func (r *GormRepository) CountPages(ctx context.Context) (int64, error) {
	var count int64

	if err := r.db.WithContext(ctx).Model(&Page{}).Count(&count).Error; err != nil {
		r.logError(nil, err, "counting pages")
		return 0, eris.Wrap(err, "counting pages")
	}

	return count, nil
}

func (r *GormRepository) logError(fields logrus.Fields, err error, message string) {
	if r.logger == nil {
		return
	}

	entry := r.logger.WithField("error", err.Error())
	if len(fields) > 0 {
		entry = entry.WithFields(fields)
	}
	entry.Error(message)
}
