package wiki

import (
	"context"
	"io"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"

	domainllm "lucipedia/app/internal/domain/llm"
)

func TestServiceGetPageReturnsExisting(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo, generator, searcher := setupServiceDependencies()

	if err := repo.Create(ctx, &Page{Slug: "alpha", HTML: "  <p>Alpha</p>  "}); err != nil {
		t.Fatalf("Create returned error: %v", err)
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
	repo, generator, searcher := setupServiceDependencies()

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

	stored := repo.get("gamma")
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
	repo, generator, searcher := setupServiceDependencies()

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

	stored := repo.get("delta")
	if stored != nil {
		t.Fatalf("expected page not to be persisted when generation fails")
	}
}

func TestServiceSearchReturnsLLMResults(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo, generator, searcher := setupServiceDependencies()

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

func TestServiceRandomSlugReturnsErrorWhenEmpty(t *testing.T) {
	t.Parallel()

	repo, generator, searcher := setupServiceDependencies()

	service, err := NewService(repo, generator, searcher, silentLogger(), nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	_, err = service.RandomSlug(context.Background())
	if err == nil {
		t.Fatalf("expected error when no pages exist")
	}

	if !eris.Is(err, ErrNoPages) {
		t.Fatalf("expected ErrNoPages, got %v", err)
	}
}

func TestServiceRandomSlugReturnsPersistedSlug(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo, generator, searcher := setupServiceDependencies()

	service, err := NewService(repo, generator, searcher, silentLogger(), nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	for _, slug := range []string{"alpha", "beta"} {
		if err := repo.Create(ctx, &Page{Slug: slug, HTML: "<p>" + slug + "</p>"}); err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
	}

	slug, err := service.RandomSlug(ctx)
	if err != nil {
		t.Fatalf("RandomSlug returned error: %v", err)
	}

	if slug != "alpha" && slug != "beta" {
		t.Fatalf("unexpected slug %q", slug)
	}
}

func TestServiceSearchPropagatesSearcherError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo, generator, searcher := setupServiceDependencies()

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

func TestServiceMostRecentPageReturnsErrorWhenEmpty(t *testing.T) {
	t.Parallel()

	repo, generator, searcher := setupServiceDependencies()

	service, err := NewService(repo, generator, searcher, silentLogger(), nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	if _, err := service.MostRecentPage(context.Background()); err == nil {
		t.Fatalf("expected ErrNoPages when repository is empty")
	} else if !eris.Is(err, ErrNoPages) {
		t.Fatalf("expected ErrNoPages, got %v", err)
	}
}

func TestServiceMostRecentPageReturnsLatestEntry(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo, generator, searcher := setupServiceDependencies()

	service, err := NewService(repo, generator, searcher, silentLogger(), nil)
	if err != nil {
		t.Fatalf("NewService returned error: %v", err)
	}

	timestamps := []time.Duration{-2, -1, 0}
	slugs := []string{"alpha", "beta", "gamma"}

	for idx, slug := range slugs {
		page := &Page{Slug: slug, HTML: "  <p>" + slug + "</p>  "}
		if err := repo.injectWithTimestamp(ctx, page, time.Now().Add(time.Duration(timestamps[idx])*time.Hour)); err != nil {
			t.Fatalf("injectWithTimestamp returned error: %v", err)
		}
	}

	page, err := service.MostRecentPage(ctx)
	if err != nil {
		t.Fatalf("MostRecentPage returned error: %v", err)
	}

	if page.Slug != "gamma" {
		t.Fatalf("expected slug gamma, got %q", page.Slug)
	}

	if page.HTML != "<p>gamma</p>" {
		t.Fatalf("expected trimmed HTML '<p>gamma</p>', got %q", page.HTML)
	}
}

type stubRepository struct {
	pages        map[string]*storedPage
	createdOrder []string
	random       *rand.Rand
}

type storedPage struct {
	page      Page
	createdAt time.Time
}

var _ Repository = (*stubRepository)(nil)

func newStubRepository() *stubRepository {
	return &stubRepository{
		pages:  make(map[string]*storedPage),
		random: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (s *stubRepository) GetBySlug(_ context.Context, slug string) (*Page, error) {
	record, ok := s.pages[strings.TrimSpace(slug)]
	if !ok {
		return nil, nil
	}
	copy := record.page
	return &copy, nil
}

func (s *stubRepository) Create(ctx context.Context, page *Page) error {
	slug := strings.TrimSpace(page.Slug)
	if _, exists := s.pages[slug]; exists {
		return eris.Errorf("page with slug %s already exists", slug)
	}
	return s.injectWithTimestamp(ctx, page, time.Now())
}

func (s *stubRepository) injectWithTimestamp(_ context.Context, page *Page, createdAt time.Time) error {
	if page == nil {
		return eris.New("page is nil")
	}
	slug := strings.TrimSpace(page.Slug)
	if slug == "" {
		return eris.New("page slug is required")
	}

	trimmed := Page{Slug: slug, HTML: strings.TrimSpace(page.HTML)}
	if stored, exists := s.pages[slug]; exists {
		stored.page = trimmed
		stored.createdAt = createdAt
		return nil
	}

	s.pages[slug] = &storedPage{page: trimmed, createdAt: createdAt}
	s.createdOrder = append(s.createdOrder, slug)
	return nil
}

func (s *stubRepository) ListPages(_ context.Context) ([]Page, error) {
	pages := make([]Page, 0, len(s.pages))
	for _, slug := range s.createdOrder {
		if record, ok := s.pages[slug]; ok {
			pages = append(pages, record.page)
		}
	}
	return pages, nil
}

func (s *stubRepository) CountPages(_ context.Context) (int64, error) {
	return int64(len(s.pages)), nil
}

func (s *stubRepository) RandomPage(_ context.Context) (*Page, error) {
	if len(s.pages) == 0 {
		return nil, nil
	}

	slugs := make([]string, 0, len(s.pages))
	for slug := range s.pages {
		slugs = append(slugs, slug)
	}
	chosen := slugs[s.random.Intn(len(slugs))]
	copy := s.pages[chosen].page
	return &copy, nil
}

func (s *stubRepository) MostRecentPage(_ context.Context) (*Page, error) {
	if len(s.pages) == 0 {
		return nil, nil
	}

	var latest *storedPage
	for _, record := range s.pages {
		if latest == nil || record.createdAt.After(latest.createdAt) {
			latest = record
		}
	}

	copy := latest.page
	return &copy, nil
}

func (s *stubRepository) get(slug string) *Page {
	record, ok := s.pages[strings.TrimSpace(slug)]
	if !ok {
		return nil
	}
	copy := record.page
	return &copy
}

func setupServiceDependencies() (*stubRepository, *stubGenerator, domainllm.Searcher) {
	repo := newStubRepository()
	generator := &stubGenerator{}
	searcher := &stubSearcher{}
	return repo, generator, searcher
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

type stubGenerator struct {
	html      string
	backlinks []string
	err       error
	calls     int
}

var _ domainllm.Generator = (*stubGenerator)(nil)

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

var _ domainllm.Searcher = (*stubSearcher)(nil)

func (s *stubSearcher) Search(ctx context.Context, query string, limit int) ([]string, error) {
	s.calls++
	s.capturedQuery = query
	s.capturedLimit = limit
	if s.err != nil {
		return nil, s.err
	}
	return s.slugs, nil
}
