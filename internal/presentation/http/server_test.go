package http

import (
	"context"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"

	"lucipedia/app/internal/domain/wiki"
)

func TestHomeRouteRendersPage(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &stubWikiService{pageCount: 42, generatorReady: true})
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

	if !contains(body, "Undiscovered articles are generated on demand.")  {
		t.Fatalf("expected footer copy with page count in body, got %q", body)
	}
}

func TestWikiRouteServesHTML(t *testing.T) {
	t.Parallel()

	service := &stubWikiService{pageHTML: "<p>Alpha</p>", pageCount: 1, generatorReady: true}
	srv := newTestServer(t, service)

	req := httptest.NewRequest("GET", "/wiki/alpha", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !contains(body, "<p>Alpha</p>") {
		t.Fatalf("expected wiki HTML in body, got %q", body)
	}

	if !contains(body, "<header") {
		t.Fatalf("expected header markup in body, got %q", body)
	}

    if !contains(body, "Undiscovered articles are generated on demand.") || !contains(body, "1 articles discovered so far.") {
        t.Fatalf("expected footer note in body, got %q", body)
    }
}

func TestWikiRouteReturns404OnUnavailablePage(t *testing.T) {
	t.Parallel()

	err := eris.New("request blocked by content filter")
	service := &stubWikiService{pageErr: err, pageCount: 1, generatorReady: true}
	srv := newTestServer(t, service)

	req := httptest.NewRequest("GET", "/wiki/blocked", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if ct := rec.Header().Get("Content-Type"); ct != htmlContentType {
		t.Fatalf("expected content type %q, got %q", htmlContentType, ct)
	}

	body := rec.Body.String()
	if !contains(body, "The requested page is not available yet.") {
		t.Fatalf("expected inline error message, got %q", body)
	}

	if !contains(body, "wiki-content-template") {
		t.Fatalf("expected streamed template markup, got %q", body)
	}
}

func TestRandomRouteRedirectsToWikiSlug(t *testing.T) {
	t.Parallel()

	service := &stubWikiService{randomSlug: "alpha", pageCount: 1, generatorReady: true}
	srv := newTestServer(t, service)

	req := httptest.NewRequest("GET", "/random", nil)
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

	service := &stubWikiService{mostRecent: &wiki.Page{Slug: "alpha", HTML: "<p>Alpha</p>"}, pageCount: 1, generatorReady: true}
	srv := newTestServer(t, service)

	req := httptest.NewRequest("GET", "/most-recent", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !contains(body, "<p>Alpha</p>") {
		t.Fatalf("expected most recent wiki HTML in body, got %q", body)
	}

	if !contains(body, "<header") {
		t.Fatalf("expected header markup in body, got %q", body)
	}

    if !contains(body, "Undiscovered articles are generated on demand.") || !contains(body, "1 articles discovered so far.") {
        t.Fatalf("expected footer note in body, got %q", body)
    }

}

func TestMostRecentRouteHandlesMissingPages(t *testing.T) {
	t.Parallel()

	service := &stubWikiService{mostRecentErr: wiki.ErrNoPages, pageCount: 0, generatorReady: true}
	srv := newTestServer(t, service)

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

	service := &stubWikiService{randomErr: wiki.ErrNoPages, pageCount: 0, generatorReady: true}
	srv := newTestServer(t, service)

	req := httptest.NewRequest("GET", "/random", nil)
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
		searchResults:  []wiki.SearchResult{{Slug: "alpha"}},
		pageCount:      1,
		generatorReady: true,
	}
	srv := newTestServer(t, service)

	req := httptest.NewRequest("GET", "/search?q=alpha", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !contains(body, "search-content-template") {
		t.Fatalf("expected streamed search template in body, got %q", body)
	}

	if !contains(body, "/wiki/alpha") {
		t.Fatalf("expected search results to include wiki link, got %q", body)
	}
}

func TestSearchRouteReturns500OnFailure(t *testing.T) {
	t.Parallel()

	service := &stubWikiService{searchErr: eris.New("llm search failure"), pageCount: 1, generatorReady: true}
	srv := newTestServer(t, service)

	req := httptest.NewRequest("GET", "/search?q=broken", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if ct := rec.Header().Get("Content-Type"); ct != htmlContentType {
		t.Fatalf("expected content type %q, got %q", htmlContentType, ct)
	}

	body := rec.Body.String()
	if !contains(body, "search-content-template") {
		t.Fatalf("expected streamed error template, got %q", body)
	}

	if !contains(body, "We couldn&#39;t process your request right now.") {
		t.Fatalf("expected fallback error message, got %q", body)
	}
}

func TestRateLimiterMiddlewareCapsRequests(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &stubWikiService{pageCount: 1, generatorReady: true})

	current := time.Unix(0, 0)
	srv.rateLimiter.now = func() time.Time {
		return current
	}

	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		srv.ServeHTTP(rec, req)
		if rec.Code != stdhttp.StatusOK {
			t.Fatalf("expected request %d to be allowed, got status %d", i+1, rec.Code)
		}
	}

	fourthRec := httptest.NewRecorder()
	fourthReq := httptest.NewRequest("GET", "/", nil)
	srv.ServeHTTP(fourthRec, fourthReq)

	if fourthRec.Code != stdhttp.StatusTooManyRequests {
		t.Fatalf("expected status %d, got %d", stdhttp.StatusTooManyRequests, fourthRec.Code)
	}

	if header := fourthRec.Header().Get("Retry-After"); header != "1" {
		t.Fatalf("expected Retry-After header to be 1, got %q", header)
	}

	if body := fourthRec.Body.String(); !contains(body, "Too Many Requests") || !contains(body, "Please wait a moment") {
		t.Fatalf("expected rate limit message in body, got %q", body)
	}

	current = current.Add(time.Second)

	postRec := httptest.NewRecorder()
	postReq := httptest.NewRequest("GET", "/", nil)
	srv.ServeHTTP(postRec, postReq)

	if postRec.Code != stdhttp.StatusOK {
		t.Fatalf("expected status %d after refill, got %d", stdhttp.StatusOK, postRec.Code)
	}
}

func TestHealthRouteReportsOK(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t, &stubWikiService{pageCount: 1, generatorReady: true})

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

}

// helper utilities

func newTestServer(t *testing.T, svc wiki.Service) *Server {
	t.Helper()

	logger := logrus.New()
	logger.SetOutput(io.Discard)

	srv, err := NewServer(Options{
		WikiService: svc,
		Logger:      logger,
		RateLimiter: RateLimiterSettings{
			Burst:             3,
			RequestsPerSecond: 3,
			ClientTTL:         time.Minute,
		},
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
	pageHTML       string
	pageErr        error
	searchResults  []wiki.SearchResult
	searchErr      error
	randomSlug     string
	randomErr      error
	mostRecent     *wiki.Page
	mostRecentErr  error
	listPages      []wiki.Page
	pageCount      int64
	countErr       error
	generatorReady bool
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

func (s *stubWikiService) ListPages(_ context.Context) ([]wiki.Page, error) {
	return s.listPages, nil
}

func (s *stubWikiService) CountPages(_ context.Context) (int64, error) {
	if s.countErr != nil {
		return 0, s.countErr
	}
	return s.pageCount, nil
}

func (s *stubWikiService) GeneratorReady() bool {
	return s.generatorReady
}

var _ wiki.Service = (*stubWikiService)(nil)
