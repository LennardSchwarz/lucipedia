package wiki

import (
	"context"
	"io"
	"path/filepath"
	"testing"

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
