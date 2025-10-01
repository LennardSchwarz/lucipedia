package wiki

import "context"

// Repository defines persistence operations supported by the wiki domain.
type Repository interface {
	GetBySlug(ctx context.Context, slug string) (*Page, error)
	Create(ctx context.Context, page *Page) error
	ListPages(ctx context.Context) ([]Page, error)
	CountPages(ctx context.Context) (int64, error)
	RandomPage(ctx context.Context) (*Page, error)
	MostRecentPage(ctx context.Context) (*Page, error)
}
