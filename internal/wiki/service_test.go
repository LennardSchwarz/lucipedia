package wiki

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"

	"lucipedia/app/internal/db"
	"lucipedia/app/internal/llm"
)

func TestServiceGetPageReturnsExisting(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo, generator, searcher, cleanup := setupServiceDependencies(t, "existing.db")
	defer cleanup()

	page := &Page{Slug: "alpha", HTML: "  <p>Alpha</p>  "}
	if err := repo.CreateOrUpdate(ctx, page); err != nil {
		t.Fatalf("CreateOrUpdate returned error: %v", err)
	}

	service, err := NewService(repo, generator, searcher, silentLogger(), nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	html, err := service.GetPage(ctx, " alpha ")
	if err != nil {
		t.Fatalf("GetPage returned error: %v", err)
	}

	if html != "<p>Alpha</p>" {
		t.Fatalf("expected trimmed html '<p>Alpha</p>', got %q", html)
	}

	if generator.calls != 0 {
		t.Fatalf("expected generator not to be invoked, got %d calls", generator.calls)
	}
}

func TestServiceGetPageGeneratesOnMiss(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo, generator, searcher, cleanup := setupServiceDependencies(t, "generate.db")
	defer cleanup()

	generator.html = "<p>Generated content with <a href=\"/wiki/beta\">Beta</a>.</p>"
	generator.backlinks = []string{"beta"}

	service, err := NewService(repo, generator, searcher, silentLogger(), nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	html, err := service.GetPage(ctx, "gamma")
	if err != nil {
		t.Fatalf("GetPage returned error: %v", err)
	}

	if html != generator.html {
		t.Fatalf("expected html %q, got %q", generator.html, html)
	}

	if generator.calls != 1 {
		t.Fatalf("expected generator to be invoked once, got %d", generator.calls)
	}

	stored, err := repo.GetBySlug(ctx, "gamma")
	if err != nil {
		t.Fatalf("GetBySlug returned error: %v", err)
	}
	if stored == nil {
		t.Fatalf("expected page to be persisted after generation")
	}
	if stored.HTML != generator.html {
		t.Fatalf("expected stored html %q, got %q", generator.html, stored.HTML)
	}
}

func TestServiceGetPagePropagatesGeneratorError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo, generator, searcher, cleanup := setupServiceDependencies(t, "generator-error.db")
	defer cleanup()

	generator.err = errStub("boom")

	service, err := NewService(repo, generator, searcher, silentLogger(), nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	if _, err := service.GetPage(ctx, "delta"); err == nil {
		t.Fatalf("expected error from generator to be propagated")
	}

	if generator.calls != 1 {
		t.Fatalf("expected generator to be invoked once, got %d", generator.calls)
	}

	stored, err := repo.GetBySlug(ctx, "delta")
	if err != nil {
		t.Fatalf("GetBySlug returned error: %v", err)
	}
	if stored != nil {
		t.Fatalf("expected page not to be persisted when generation fails")
	}
}

func TestServiceSearchReturnsLLMResults(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo, generator, searcher, cleanup := setupServiceDependencies(t, "search.db")
	defer cleanup()

	service, err := NewService(repo, generator, searcher, silentLogger(), nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	stub := searcher.(*stubSearcher)
	stub.slugs = []string{"history", "alpha", "beta"}

	results, err := service.Search(ctx, "alpha history", 2)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results due to limit, got %d", len(results))
	}

	if results[0].Slug != "history" {
		t.Fatalf("expected history to rank first, got %q", results[0].Slug)
	}

	if results[1].Slug != "alpha" {
		t.Fatalf("expected alpha to rank second, got %q", results[1].Slug)
	}

	if stub.calls != 1 {
		t.Fatalf("expected searcher to be called once, got %d", stub.calls)
	}

	if stub.capturedQuery != "alpha history" {
		t.Fatalf("expected query to be trimmed and passed through, got %q", stub.capturedQuery)
	}

	if stub.capturedLimit != 2 {
		t.Fatalf("expected limit to be honoured, got %d", stub.capturedLimit)
	}
}

func TestServiceSearchPropagatesSearcherError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo, generator, searcher, cleanup := setupServiceDependencies(t, "search-error.db")
	defer cleanup()

	stub := searcher.(*stubSearcher)
	stub.err = errStub("search failed")

	service, err := NewService(repo, generator, searcher, silentLogger(), nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	if _, err := service.Search(ctx, "topic", 3); err == nil {
		t.Fatalf("expected error when searcher fails")
	}

	if stub.calls != 1 {
		t.Fatalf("expected searcher to be invoked once, got %d", stub.calls)
	}
}

// stubGenerator implements llm.Generator for testing.
type stubGenerator struct {
	html      string
	backlinks []string
	err       error
	calls     int
}

var _ llm.Generator = (*stubGenerator)(nil)

func (s *stubGenerator) Generate(ctx context.Context, slug string) (string, []string, error) {
	s.calls++
	if s.err != nil {
		return "", nil, s.err
	}
	return s.html, s.backlinks, nil
}

type stubSearcher struct {
	slugs         []string
	err           error
	calls         int
	capturedQuery string
	capturedLimit int
}

var _ llm.Searcher = (*stubSearcher)(nil)

func (s *stubSearcher) Search(ctx context.Context, query string, limit int) ([]string, error) {
	s.calls++
	s.capturedQuery = query
	s.capturedLimit = limit
	if s.err != nil {
		return nil, s.err
	}
	return s.slugs, nil
}

func setupServiceDependencies(t *testing.T, filename string) (*GormRepository, *stubGenerator, llm.Searcher, func()) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, filename)

	gormDB, err := db.Open(db.Options{Path: path})
	if err != nil {
		t.Fatalf("db.Open returned error: %v", err)
	}

	logger := silentLogger()

	if err := Migrate(context.Background(), gormDB, logger); err != nil {
		t.Fatalf("Migrate returned error: %v", err)
	}

	repo, err := NewRepository(gormDB, logger)
	if err != nil {
		t.Fatalf("NewRepository returned error: %v", err)
	}

	generator := &stubGenerator{}
	searcher := &stubSearcher{}

	cleanup := func() {
		if err := db.Close(gormDB); err != nil {
			t.Fatalf("closing database failed: %v", err)
		}
	}

	return repo, generator, searcher, cleanup
}

func silentLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return logger
}

type errStub string

func (e errStub) Error() string {
	return string(e)
}
