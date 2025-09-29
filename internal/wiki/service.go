package wiki

import (
	"context"
	"strings"

	"github.com/getsentry/sentry-go"
	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"

	"lucipedia/app/internal/llm"
)

// Service defines higher-level wiki operations built on top of the repository and generator.
type Service interface {
	GetPage(ctx context.Context, slug string) (string, error)
	Search(ctx context.Context, query string, limit int) ([]SearchResult, error)
	RandomSlug(ctx context.Context) (string, error)
	MostRecentPage(ctx context.Context) (*Page, error)
}

type service struct {
	repo      Repository
	generator llm.Generator
	searcher  llm.Searcher
	logger    *logrus.Logger
	sentryHub *sentry.Hub
}

var _ Service = (*service)(nil)

// ErrNoPages indicates there are no persisted wiki pages to select from.
var ErrNoPages = eris.New("no wiki pages available")

const (
	defaultSearchLimit           = 10
	disallowedBacklinkCharacters = " \"#?<>\\"
)

// SearchResult represents a wiki entry returned by the search service.
type SearchResult struct {
	Slug string
}

// NewService wires the wiki service with its dependencies.
func NewService(repo Repository, generator llm.Generator, searcher llm.Searcher, logger *logrus.Logger, hub *sentry.Hub) (Service, error) {
	if repo == nil {
		return nil, eris.New("wiki repository is required")
	}
	if generator == nil {
		return nil, eris.New("llm generator is required")
	}
	if searcher == nil {
		return nil, eris.New("llm searcher is required")
	}

	return &service{
		repo:      repo,
		generator: generator,
		searcher:  searcher,
		logger:    logger,
		sentryHub: hub,
	}, nil
}

func (s *service) GetPage(ctx context.Context, slug string) (string, error) {
	trimmedSlug := strings.TrimSpace(slug)
	if trimmedSlug == "" {
		return "", eris.New("slug is required")
	}

	page, err := s.repo.GetBySlug(ctx, trimmedSlug)
	if err != nil {
		s.recordError(logrus.Fields{"slug": trimmedSlug}, err, "retrieving page from repository")
		return "", eris.Wrapf(err, "retrieving page: %s", trimmedSlug)
	}

	if page != nil && strings.TrimSpace(page.HTML) != "" {
		return strings.TrimSpace(page.HTML), nil
	}

	html, backlinks, err := s.generator.Generate(ctx, trimmedSlug)
	if err != nil {
		s.recordError(logrus.Fields{"slug": trimmedSlug}, err, "llm wiki wiki page generation")
		return "", eris.Wrapf(err, "generating page: %s", trimmedSlug)
	}

	html = strings.TrimSpace(html)
	if html == "" {
		err := eris.New("generated html is empty")
		s.recordError(logrus.Fields{"slug": trimmedSlug}, err, "validating llm generated html")
		return "", eris.Wrapf(err, "validating generated html for slug %s", trimmedSlug)
	}

	if err := validateBacklinks(html, backlinks); err != nil {
		s.recordError(logrus.Fields{"slug": trimmedSlug}, err, "validating backlinks during wiki page generation")
		return "", eris.Wrapf(err, "validating backlinks for slug %s", trimmedSlug)
	}

	newPage := &Page{Slug: trimmedSlug, HTML: html}
	if err := s.repo.CreateOrUpdate(ctx, newPage); err != nil {
		s.recordError(logrus.Fields{"slug": trimmedSlug}, err, "persisting generated page to repository")
		return "", eris.Wrapf(err, "persisting generated page: %s", trimmedSlug)
	}

	return html, nil
}

func (s *service) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return nil, eris.New("query is required")
	}

	if limit <= 0 {
		limit = defaultSearchLimit
	}

	slugs, err := s.searcher.Search(ctx, trimmedQuery, limit)
	if err != nil {
		s.recordError(logrus.Fields{"query": trimmedQuery}, err, "performing search")
		return nil, eris.Wrap(err, "llm search failure")
	}

	results := make([]SearchResult, 0, len(slugs))
	for _, slug := range slugs {
		trimmedSlug := strings.TrimSpace(slug)
		if trimmedSlug == "" {
			continue
		}
		results = append(results, SearchResult{Slug: trimmedSlug})
	}

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

func (s *service) RandomSlug(ctx context.Context) (string, error) {
	page, err := s.repo.RandomPage(ctx)
	if err != nil {
		s.recordError(nil, err, "selecting random wiki page")
		return "", eris.Wrap(err, "selecting random wiki page")
	}

	if page == nil {
		wrapped := eris.Wrap(ErrNoPages, "selecting random wiki page")
		s.recordError(nil, wrapped, "selecting random wiki page")
		return "", wrapped
	}

	slug := strings.TrimSpace(page.Slug)
	if slug == "" {
		err := eris.New("random page slug is empty")
		s.recordError(logrus.Fields{"page_id": page.ID}, err, "validating random wiki page")
		return "", eris.Wrap(err, "validating random wiki page")
	}

	return slug, nil
}

func (s *service) MostRecentPage(ctx context.Context) (*Page, error) {
	page, err := s.repo.MostRecentPage(ctx)
	if err != nil {
		s.recordError(nil, err, "retrieving most recent wiki page")
		return nil, eris.Wrap(err, "retrieving most recent wiki page")
	}

	if page == nil {
		wrapped := eris.Wrap(ErrNoPages, "retrieving most recent wiki page")
		s.recordError(nil, wrapped, "retrieving most recent wiki page")
		return nil, wrapped
	}

	trimmedSlug := strings.TrimSpace(page.Slug)
	if trimmedSlug == "" {
		err := eris.New("most recent page slug is empty")
		s.recordError(logrus.Fields{"page_id": page.ID}, err, "validating most recent wiki page")
		return nil, eris.Wrap(err, "validating most recent wiki page")
	}

	html := strings.TrimSpace(page.HTML)
	if html == "" {
		err := eris.New("most recent page html is empty")
		s.recordError(logrus.Fields{"page_id": page.ID}, err, "validating most recent wiki page")
		return nil, eris.Wrap(err, "validating most recent wiki page")
	}

	copyPage := *page
	copyPage.Slug = trimmedSlug
	copyPage.HTML = html

	return &copyPage, nil
}

func (s *service) recordError(fields logrus.Fields, err error, message string) {
	if err == nil {
		return
	}

	if s.logger != nil {
		entry := s.logger.WithField("error", err.Error())
		if len(fields) > 0 {
			entry = entry.WithFields(fields)
		}
		entry.Error(message)
	}

	if s.sentryHub != nil {
		s.sentryHub.CaptureException(err)
	}
}

func validateBacklinks(html string, backlinks []string) error {
	if len(backlinks) == 0 {
		return nil
	}

	lowerHTML := strings.ToLower(html)

	for _, link := range backlinks {
		trimmed := strings.TrimSpace(link)
		if trimmed == "" {
			return eris.New("backlink slug is empty")
		}
		if strings.Contains(trimmed, "/") {
			return eris.Errorf("backlink slug %s contains invalid path separator", trimmed)
		}
		if strings.ContainsAny(trimmed, disallowedBacklinkCharacters) {
			return eris.Errorf("backlink slug %s contains invalid characters", trimmed)
		}

		expected := "/wiki/" + strings.ToLower(trimmed)
		if !strings.Contains(lowerHTML, expected) {
			return eris.Errorf("backlink slug %s is missing from html", trimmed)
		}
	}

	return nil
}
