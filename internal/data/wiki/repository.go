package wiki

import (
	"context"
	"errors"
	"strings"

	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	domainwiki "lucipedia/app/internal/domain/wiki"
)

// Repository persists wiki pages using a Gorm database connection.
type Repository struct {
	db     *gorm.DB
	logger *logrus.Logger
}

// NewRepository constructs a Gorm-backed repository implementation.
func NewRepository(db *gorm.DB, logger *logrus.Logger) (*Repository, error) {
	if db == nil {
		return nil, eris.New("gorm DB is required")
	}

	return &Repository{db: db, logger: logger}, nil
}

var _ domainwiki.Repository = (*Repository)(nil)

// GetBySlug returns the page for the provided slug or nil when not found.
func (r *Repository) GetBySlug(ctx context.Context, slug string) (*domainwiki.Page, error) {
	trimmed := strings.TrimSpace(slug)
	if trimmed == "" {
		return nil, eris.New("slug is required")
	}

	var record PageRecord
	err := r.db.WithContext(ctx).First(&record, "slug = ?", trimmed).Error
	if err != nil {
		if eris.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		r.logError(logrus.Fields{"slug": trimmed}, err, "fetching page by slug")
		return nil, eris.Wrapf(err, "fetching page by slug: %s", trimmed)
	}

	return toDomainPage(&record), nil
}

// Create stores a new wiki page. It returns an error when the slug already exists.
func (r *Repository) Create(ctx context.Context, page *domainwiki.Page) error {
	if page == nil {
		return eris.New("page is nil")
	}

	trimmedSlug := strings.TrimSpace(page.Slug)
	if trimmedSlug == "" {
		return eris.New("page slug is required")
	}

	record := &PageRecord{
		Slug: trimmedSlug,
		HTML: strings.TrimSpace(page.HTML),
	}

	if err := r.db.WithContext(ctx).Create(record).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(strings.ToLower(err.Error()), "unique") {
			dupErr := eris.Errorf("page with slug %s already exists", trimmedSlug)
			r.logError(logrus.Fields{"slug": trimmedSlug}, dupErr, "creating page with duplicate slug")
			return dupErr
		}
		r.logError(logrus.Fields{"slug": trimmedSlug}, err, "creating page")
		return eris.Wrapf(err, "creating page: %s", trimmedSlug)
	}

	page.Slug = trimmedSlug
	return nil
}

// ListPages returns every page ordered by slug.
func (r *Repository) ListPages(ctx context.Context) ([]domainwiki.Page, error) {
	var records []PageRecord

	if err := r.db.WithContext(ctx).Order("slug ASC").Find(&records).Error; err != nil {
		r.logError(nil, err, "listing pages")
		return nil, eris.Wrap(err, "listing pages")
	}

	pages := make([]domainwiki.Page, 0, len(records))
	for _, record := range records {
		domain := toDomainPage(&record)
		pages = append(pages, *domain)
	}

	return pages, nil
}

// CountPages returns the total number of persisted wiki pages.
func (r *Repository) CountPages(ctx context.Context) (int64, error) {
	var count int64

	if err := r.db.WithContext(ctx).Model(&PageRecord{}).Count(&count).Error; err != nil {
		r.logError(nil, err, "counting pages")
		return 0, eris.Wrap(err, "counting pages")
	}

	return count, nil
}

// RandomPage returns a single random page or nil when the table is empty.
func (r *Repository) RandomPage(ctx context.Context) (*domainwiki.Page, error) {
	var record PageRecord

	if err := r.db.WithContext(ctx).Order("RANDOM()").First(&record).Error; err != nil {
		if eris.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		r.logError(nil, err, "selecting random page")
		return nil, eris.Wrap(err, "selecting random page")
	}

	return toDomainPage(&record), nil
}

// MostRecentPage returns the latest created page ordered by CreatedAt descending.
func (r *Repository) MostRecentPage(ctx context.Context) (*domainwiki.Page, error) {
	var record PageRecord

	if err := r.db.WithContext(ctx).Order("created_at DESC").First(&record).Error; err != nil {
		if eris.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		r.logError(nil, err, "selecting most recent page")
		return nil, eris.Wrap(err, "selecting most recent page")
	}

	return toDomainPage(&record), nil
}

func (r *Repository) logError(fields logrus.Fields, err error, message string) {
	if r.logger == nil || err == nil {
		return
	}

	entry := r.logger.WithField("error", err.Error())
	if len(fields) > 0 {
		entry = entry.WithFields(fields)
	}
	entry.Error(message)
}

func toDomainPage(record *PageRecord) *domainwiki.Page {
	if record == nil {
		return nil
	}

	return &domainwiki.Page{
		Slug: strings.TrimSpace(record.Slug),
		HTML: strings.TrimSpace(record.HTML),
	}
}
