package wiki

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	"lucipedia/app/internal/db"
)

func TestNewRepositoryRequiresDatabase(t *testing.T) {
	t.Parallel()

	if _, err := NewRepository(nil, nil); err == nil {
		t.Fatalf("expected error when database is nil")
	}
}

func TestGetBySlugReturnsNilForMissingPage(t *testing.T) {
	t.Parallel()

	repo := setupRepository(t)

	page, err := repo.GetBySlug(context.Background(), "missing")
	if err != nil {
		t.Fatalf("GetBySlug returned error: %v", err)
	}
	if page != nil {
		t.Fatalf("expected nil page for missing slug, got %#v", page)
	}
}

func TestCreateOrUpdateRoundTrip(t *testing.T) {
	t.Parallel()

	repo := setupRepository(t)
	ctx := context.Background()

	original := &Page{Slug: " example ", HTML: "<p>Hello</p>"}
	if err := repo.CreateOrUpdate(ctx, original); err != nil {
		t.Fatalf("CreateOrUpdate returned error: %v", err)
	}

	if original.Slug != "example" {
		t.Fatalf("expected slug trimmed to 'example', got %q", original.Slug)
	}

	stored, err := repo.GetBySlug(ctx, "example")
	if err != nil {
		t.Fatalf("GetBySlug returned error: %v", err)
	}
	if stored == nil {
		t.Fatalf("expected stored page to be present")
	}
	if stored.HTML != "<p>Hello</p>" {
		t.Fatalf("expected HTML to be preserved, got %q", stored.HTML)
	}
}

func TestListPagesReturnsAlphabeticalOrder(t *testing.T) {
	t.Parallel()

	repo := setupRepository(t)
	ctx := context.Background()

	pages := []Page{
		{Slug: "zulu", HTML: "<p>Z</p>"},
		{Slug: "alpha", HTML: "<p>A</p>"},
		{Slug: "beta", HTML: "<p>B</p>"},
	}

	for _, page := range pages {
		p := page
		if err := repo.CreateOrUpdate(ctx, &p); err != nil {
			t.Fatalf("CreateOrUpdate returned error: %v", err)
		}
	}

	listed, err := repo.ListPages(ctx)
	if err != nil {
		t.Fatalf("ListPages returned error: %v", err)
	}

	expectedOrder := []string{"alpha", "beta", "zulu"}
	if len(listed) != len(expectedOrder) {
		t.Fatalf("expected %d pages, got %d", len(expectedOrder), len(listed))
	}

	for idx, slug := range expectedOrder {
		if listed[idx].Slug != slug {
			t.Fatalf("expected slug %q at index %d, got %q", slug, idx, listed[idx].Slug)
		}
	}
}

func TestCountPagesReturnsTotal(t *testing.T) {
	t.Parallel()

	repo := setupRepository(t)
	ctx := context.Background()

	for _, slug := range []string{"alpha", "beta", "gamma"} {
		page := &Page{Slug: slug, HTML: "<p>Content</p>"}
		if err := repo.CreateOrUpdate(ctx, page); err != nil {
			t.Fatalf("CreateOrUpdate returned error: %v", err)
		}
	}

	count, err := repo.CountPages(ctx)
	if err != nil {
		t.Fatalf("CountPages returned error: %v", err)
	}

	if count != 3 {
		t.Fatalf("expected count 3, got %d", count)
	}
}

func TestRandomPageReturnsNilWhenEmpty(t *testing.T) {
	t.Parallel()

	repo := setupRepository(t)

	page, err := repo.RandomPage(context.Background())
	if err != nil {
		t.Fatalf("RandomPage returned error: %v", err)
	}

	if page != nil {
		t.Fatalf("expected nil page when database is empty, got %#v", page)
	}
}

func TestRandomPageReturnsExistingEntry(t *testing.T) {
	t.Parallel()

	repo := setupRepository(t)
	ctx := context.Background()

	slugs := []string{"alpha", "beta", "gamma"}
	for _, slug := range slugs {
		page := &Page{Slug: slug, HTML: "<p>" + slug + "</p>"}
		if err := repo.CreateOrUpdate(ctx, page); err != nil {
			t.Fatalf("CreateOrUpdate returned error: %v", err)
		}
	}

	page, err := repo.RandomPage(ctx)
	if err != nil {
		t.Fatalf("RandomPage returned error: %v", err)
	}

	if page == nil {
		t.Fatalf("expected RandomPage to return a page")
	}

	valid := map[string]struct{}{
		"alpha": {},
		"beta":  {},
		"gamma": {},
	}

	if _, ok := valid[page.Slug]; !ok {
		t.Fatalf("unexpected slug %q returned", page.Slug)
	}
}

func TestMostRecentPageReturnsNilWhenEmpty(t *testing.T) {
	t.Parallel()

	repo := setupRepository(t)

	page, err := repo.MostRecentPage(context.Background())
	if err != nil {
		t.Fatalf("MostRecentPage returned error: %v", err)
	}

	if page != nil {
		t.Fatalf("expected nil page when repository is empty, got %#v", page)
	}
}

func TestMostRecentPageReturnsLatestEntry(t *testing.T) {
	t.Parallel()

	repo := setupRepository(t)
	ctx := context.Background()

	entries := []struct {
		slug    string
		html    string
		created time.Time
	}{
		{slug: "alpha", html: "<p>A</p>", created: time.Now().Add(-2 * time.Hour)},
		{slug: "beta", html: "<p>B</p>", created: time.Now().Add(-1 * time.Hour)},
		{slug: "gamma", html: "<p>G</p>", created: time.Now()},
	}

	for _, entry := range entries {
		page := &Page{Slug: entry.slug, HTML: entry.html}
		if err := repo.CreateOrUpdate(ctx, page); err != nil {
			t.Fatalf("CreateOrUpdate returned error: %v", err)
		}
		if err := repo.db.WithContext(ctx).Model(&Page{}).Where("slug = ?", entry.slug).Update("created_at", entry.created).Error; err != nil {
			t.Fatalf("updating created_at returned error: %v", err)
		}
	}

	page, err := repo.MostRecentPage(ctx)
	if err != nil {
		t.Fatalf("MostRecentPage returned error: %v", err)
	}

	if page == nil {
		t.Fatalf("expected MostRecentPage to return a page")
	}

	if page.Slug != "gamma" {
		t.Fatalf("expected most recent page slug to be gamma, got %q", page.Slug)
	}

	if page.HTML != "<p>G</p>" {
		t.Fatalf("expected HTML for most recent page to be preserved, got %q", page.HTML)
	}
}

func setupRepository(t *testing.T) *GormRepository {
	t.Helper()

	path := filepath.Join(t.TempDir(), "repo.db")
	gormDB, err := db.Open(db.Options{Path: path})
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}

	t.Cleanup(func() {
		if closeErr := db.Close(gormDB); closeErr != nil {
			t.Fatalf("closing database failed: %v", closeErr)
		}
	})

	logger := logrus.New()
	logger.SetOutput(io.Discard)

	if err := Migrate(context.Background(), gormDB, logger); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}

	repo, err := NewRepository(gormDB, logger)
	if err != nil {
		t.Fatalf("NewRepository returned error: %v", err)
	}

	return repo
}
