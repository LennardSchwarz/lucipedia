package http

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"lucipedia/app/internal/llm"
	"lucipedia/app/internal/wiki"
)

func TestHomeRouteRendersPage(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &stubWikiService{}, &stubRepository{count: 42})
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if ct := rec.Header().Get("Content-Type"); ct != htmlContentType {
		t.Fatalf("expected content type %q, got %q", htmlContentType, ct)
	}

	body := rec.Body.String()
	if !contains(body, "Lucipedia") {
		t.Fatalf("expected body to contain site title, got %q", body)
	}

	if !contains(body, "42 pages discovered so far") {
		t.Fatalf("expected page count in body, got %q", body)
	}
}

func TestWikiRouteServesHTML(t *testing.T) {
	t.Parallel()

	service := &stubWikiService{pageHTML: "<p>Alpha</p>"}
	srv := newTestServer(t, service, &stubRepository{count: 1})

	req := httptest.NewRequest("GET", "/wiki/alpha", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if rec.Body.String() != "<p>Alpha</p>" {
		t.Fatalf("expected wiki HTML in body, got %q", rec.Body.String())
	}
}

func TestWikiRouteReturns404OnUnavailablePage(t *testing.T) {
	t.Parallel()

	err := eris.New("request blocked by content filter")
	service := &stubWikiService{pageErr: err}
	srv := newTestServer(t, service, &stubRepository{count: 1})

	req := httptest.NewRequest("GET", "/wiki/blocked", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}

	if ct := rec.Header().Get("Content-Type"); ct != htmlContentType {
		t.Fatalf("expected content type %q, got %q", htmlContentType, ct)
	}
}

func TestRandomRouteRedirectsToWikiSlug(t *testing.T) {
	t.Parallel()

	service := &stubWikiService{randomSlug: "alpha"}
	srv := newTestServer(t, service, &stubRepository{count: 1})

	req := httptest.NewRequest("GET", "/random/", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != 302 {
		t.Fatalf("expected status 302, got %d", rec.Code)
	}

	if location := rec.Header().Get("Location"); location != "/wiki/alpha" {
		t.Fatalf("expected redirect to /wiki/alpha, got %q", location)
	}
}

func TestMostRecentRouteServesHTML(t *testing.T) {
	t.Parallel()

	service := &stubWikiService{mostRecent: &wiki.Page{Slug: "alpha", HTML: "<p>Alpha</p>"}}
	srv := newTestServer(t, service, &stubRepository{count: 1})

	req := httptest.NewRequest("GET", "/most-recent", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if rec.Body.String() != "<p>Alpha</p>" {
		t.Fatalf("expected most recent HTML in body, got %q", rec.Body.String())
	}
}

func TestMostRecentRouteHandlesMissingPages(t *testing.T) {
	t.Parallel()

	service := &stubWikiService{mostRecentErr: wiki.ErrNoPages}
	srv := newTestServer(t, service, &stubRepository{count: 0})

	req := httptest.NewRequest("GET", "/most-recent", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}

	if !contains(rec.Body.String(), "Follow a link to generate the first article") {
		t.Fatalf("expected helpful message in body, got %q", rec.Body.String())
	}
}

func TestRandomRouteHandlesMissingPages(t *testing.T) {
	t.Parallel()

	service := &stubWikiService{randomErr: wiki.ErrNoPages}
	srv := newTestServer(t, service, &stubRepository{count: 0})

	req := httptest.NewRequest("GET", "/random/", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != 404 {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}

	if ct := rec.Header().Get("Content-Type"); ct != htmlContentType {
		t.Fatalf("expected content type %q, got %q", htmlContentType, ct)
	}

	if !contains(rec.Body.String(), "Follow a link to generate the first article") {
		t.Fatalf("expected helpful message in body, got %q", rec.Body.String())
	}
}

func TestSearchRouteRendersResults(t *testing.T) {
	t.Parallel()

	service := &stubWikiService{
		searchResults: []wiki.SearchResult{{Slug: "alpha"}},
	}
	srv := newTestServer(t, service, &stubRepository{count: 1})

	req := httptest.NewRequest("GET", "/search?q=alpha", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if !contains(rec.Body.String(), "/wiki/alpha") {
		t.Fatalf("expected search results to include wiki link, got %q", rec.Body.String())
	}
}

func TestSearchRouteReturns500OnFailure(t *testing.T) {
	t.Parallel()

	service := &stubWikiService{searchErr: eris.New("llm search failure")}
	srv := newTestServer(t, service, &stubRepository{count: 1})

	req := httptest.NewRequest("GET", "/search?q=broken", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != 500 {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}

	if ct := rec.Header().Get("Content-Type"); ct != htmlContentType {
		t.Fatalf("expected content type %q, got %q", htmlContentType, ct)
	}
}

func TestHealthRouteReportsOK(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &stubWikiService{}, &stubRepository{count: 1})

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

// helper utilities

func newTestServer(t *testing.T, svc wiki.Service, repo wiki.Repository) *Server {
	t.Helper()

	gormDB, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open returned error: %v", err)
	}

	logger := logrus.New()
	logger.SetOutput(io.Discard)

	srv, err := NewServer(Options{
		WikiService: svc,
		Repository:  repo,
		Generator:   &stubGenerator{},
		Database:    gormDB,
		Logger:      logger,
	})
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	return srv
}

func contains(body, substring string) bool {
	return strings.Contains(body, substring)
}

// stubs

type stubWikiService struct {
	pageHTML      string
	pageErr       error
	searchResults []wiki.SearchResult
	searchErr     error
	randomSlug    string
	randomErr     error
	mostRecent    *wiki.Page
	mostRecentErr error
}

func (s *stubWikiService) GetPage(_ context.Context, _ string) (string, error) {
	if s.pageErr != nil {
		return "", s.pageErr
	}
	return s.pageHTML, nil
}

func (s *stubWikiService) RandomSlug(_ context.Context) (string, error) {
	if s.randomErr != nil {
		return "", s.randomErr
	}
	return s.randomSlug, nil
}

func (s *stubWikiService) MostRecentPage(_ context.Context) (*wiki.Page, error) {
	if s.mostRecentErr != nil {
		return nil, s.mostRecentErr
	}
	return s.mostRecent, nil
}

func (s *stubWikiService) Search(_ context.Context, _ string, _ int) ([]wiki.SearchResult, error) {
	if s.searchErr != nil {
		return nil, s.searchErr
	}
	return s.searchResults, nil
}

type stubRepository struct {
	count int64
}

func (s *stubRepository) GetBySlug(_ context.Context, _ string) (*wiki.Page, error) {
	return nil, nil
}

func (s *stubRepository) CreateOrUpdate(_ context.Context, _ *wiki.Page) error {
	return nil
}

func (s *stubRepository) ListPages(_ context.Context) ([]wiki.Page, error) {
	return nil, nil
}

func (s *stubRepository) RandomPage(_ context.Context) (*wiki.Page, error) {
	return nil, nil
}

func (s *stubRepository) MostRecentPage(_ context.Context) (*wiki.Page, error) {
	return nil, nil
}

func (s *stubRepository) CountPages(_ context.Context) (int64, error) {
	return s.count, nil
}

type stubGenerator struct{}

func (s *stubGenerator) Generate(_ context.Context, _ string) (string, []string, error) {
	return "", nil, nil
}

var _ wiki.Service = (*stubWikiService)(nil)
var _ wiki.Repository = (*stubRepository)(nil)
var _ llm.Generator = (*stubGenerator)(nil)
