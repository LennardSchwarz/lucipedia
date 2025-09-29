package http

import (
	"context"
	"fmt"
	stdhttp "net/http"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/getsentry/sentry-go"
	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"

	"lucipedia/app/internal/db"
	"lucipedia/app/internal/http/templates"
	"lucipedia/app/internal/wiki"
)

const (
	htmlContentType      = "text/html; charset=utf-8"
	searchResultsLimit   = 10
	errorFallbackMessage = "We couldn't process your request right now."
)

type htmlResponse struct {
	Status      int
	ContentType string `header:"Content-Type"`
	Location    string `header:"Location"`
	Body        []byte
}

type wikiInput struct {
	Slug string `path:"slug"`
}

type searchInput struct {
	Query string `query:"q"`
}

type healthResponse struct {
	Status int
	Body   struct {
		Status    string `json:"status"`
		Database  string `json:"database"`
		Generator string `json:"generator"`
	}
}

func (s *Server) registerHomeRoute() {
	huma.Get(s.api, "/", s.homeHandler, htmlOperation("Lucipedia home", stdhttp.StatusInternalServerError))
}

func (s *Server) registerRandomRoute() {
	huma.Get(s.api, "/random/", s.randomHandler, htmlOperation(
		"Redirect to random page",
		stdhttp.StatusFound,
		stdhttp.StatusNotFound,
		stdhttp.StatusInternalServerError,
	))
}

func (s *Server) registerMostRecentRoute() {
	huma.Get(s.api, "/most-recent", s.mostRecentHandler, htmlOperation(
		"Fetch most recent page",
		stdhttp.StatusNotFound,
		stdhttp.StatusInternalServerError,
	))
}

func (s *Server) registerWikiRoute() {
	huma.Get(s.api, "/wiki/{slug}", s.wikiHandler, htmlOperation(
		"Fetch wiki page",
		stdhttp.StatusBadRequest,
		stdhttp.StatusNotFound,
		stdhttp.StatusInternalServerError,
	))
}

func (s *Server) registerSearchRoute() {
	huma.Get(s.api, "/search", s.searchHandler, htmlOperation(
		"Search Lucipedia",
		stdhttp.StatusBadRequest,
		stdhttp.StatusInternalServerError,
	))
}

func (s *Server) registerHealthRoute() {
	huma.Get(s.api, "/healthz", s.healthHandler, func(op *huma.Operation) {
		op.Summary = "Health check"
	})
}

func (s *Server) homeHandler(ctx context.Context, _ *struct{}) (*htmlResponse, error) {
	count, err := s.repository.CountPages(ctx)
	if err != nil {
		s.recordError(ctx, err, "counting pages", nil)
		return s.renderErrorResponse(ctx, stdhttp.StatusInternalServerError, "We couldn't load Lucipedia right now.")
	}

	subtitle := fmt.Sprintf("The infinite encyclopedia. %s pages discovered so far.", formatCount(count))
	data := templates.HomePageData{
		Title:          "Lucipedia",
		Subtitle:       subtitle,
		PageCountLabel: fmt.Sprintf("%s pages discovered so far.", formatCount(count)),
		Description:    "Lucipedia is a continuously generated encyclopedia where every visit uncovers fresh AI-written lore.",
		IntroParagraphs: []string{
			"Whenever you follow a link, Lucipedia first checks whether the page already exists. If not, the language model writes it on demand and saves it for future explorers.",
			"New entries only appear when you follow existing links—there's no shortcut to unknown slugs.",
			"Every article is dreamt up by an AI historian. Treat the knowledge as inspiration, not verified fact.",
		},
		BuilderAttribution: "Built with curiosity for wanderers of emergent knowledge.",
		FooterNote:         "Lucipedia pages are generated on demand. Internal links create new articles the first time they are visited.",
	}

	body, err := renderComponent(ctx, templates.HomePage(data))
	if err != nil {
		s.recordError(ctx, err, "rendering home page", nil)
		return s.renderErrorResponse(ctx, stdhttp.StatusInternalServerError, "We couldn't render the homepage.")
	}

	return newHTMLResponse(stdhttp.StatusOK, body), nil
}

func (s *Server) randomHandler(ctx context.Context, _ *struct{}) (*htmlResponse, error) {
	slug, err := s.wiki.RandomSlug(ctx)
	if err != nil {
		status := stdhttp.StatusInternalServerError
		message := errorFallbackMessage

		if eris.Is(err, wiki.ErrNoPages) {
			status = stdhttp.StatusNotFound
			message = "Lucipedia doesn't have any pages yet. Follow a link to generate the first article."
		}

		s.recordError(ctx, err, "selecting random page", nil)
		return s.renderErrorResponse(ctx, status, message)
	}

	response := newHTMLResponse(stdhttp.StatusFound, nil)
	response.Location = "/wiki/" + slug

	return response, nil
}

func (s *Server) mostRecentHandler(ctx context.Context, _ *struct{}) (*htmlResponse, error) {
	page, err := s.wiki.MostRecentPage(ctx)
	if err != nil {
		status := stdhttp.StatusInternalServerError
		message := errorFallbackMessage

		if eris.Is(err, wiki.ErrNoPages) {
			status = stdhttp.StatusNotFound
			message = "Lucipedia doesn't have any pages yet. Follow a link to generate the first article."
		}

		s.recordError(ctx, err, "loading most recent page", nil)
		return s.renderErrorResponse(ctx, status, message)
	}

	return newHTMLResponse(stdhttp.StatusOK, []byte(page.HTML)), nil
}

func (s *Server) wikiHandler(ctx context.Context, input *wikiInput) (*htmlResponse, error) {
	slug := strings.TrimSpace(input.Slug)
	html, err := s.wiki.GetPage(ctx, slug)
	if err != nil {
		status, message := classifyError(err)
		s.recordError(ctx, err, "loading wiki page", logrus.Fields{"slug": slug})
		return s.renderErrorResponse(ctx, status, message)
	}

	return newHTMLResponse(stdhttp.StatusOK, []byte(html)), nil
}

func (s *Server) searchHandler(ctx context.Context, input *searchInput) (*htmlResponse, error) {
	query := strings.TrimSpace(input.Query)
	data := templates.SearchPageData{
		Title: "Search • Lucipedia",
		Query: query,
	}

	status := stdhttp.StatusOK

	if query != "" {
		results, err := s.wiki.Search(ctx, query, searchResultsLimit)
		if err != nil {
			status, message := classifyError(err)
			s.recordError(ctx, err, "search request failed", logrus.Fields{"query": query})
			if status == stdhttp.StatusBadRequest {
				data.ErrorMessage = message
			} else {
				return s.renderErrorResponse(ctx, status, message)
			}
			status = stdhttp.StatusBadRequest
		} else {
			data.Results = make([]templates.SearchResultView, 0, len(results))
			for _, result := range results {
				data.Results = append(data.Results, templates.SearchResultView{
					Title: result.Slug,
					URL:   "/wiki/" + result.Slug,
				})
			}
		}
	}

	body, err := renderComponent(ctx, templates.SearchPage(data))
	if err != nil {
		s.recordError(ctx, err, "rendering search page", logrus.Fields{"query": query})
		return s.renderErrorResponse(ctx, stdhttp.StatusInternalServerError, "We couldn't render search results right now.")
	}

	return newHTMLResponse(status, body), nil
}

func (s *Server) healthHandler(ctx context.Context, _ *struct{}) (*healthResponse, error) {
	resp := &healthResponse{}
	resp.Body.Status = "ok"
	resp.Body.Database = "ok"
	resp.Body.Generator = "ready"

	sqlDB, err := db.SQLDB(s.db)
	if err != nil {
		s.recordError(ctx, err, "obtaining sql db", nil)
		resp.Body.Status = "degraded"
		resp.Body.Database = "error"
		resp.Status = stdhttp.StatusServiceUnavailable
	} else if pingErr := sqlDB.PingContext(ctx); pingErr != nil {
		s.recordError(ctx, pingErr, "pinging database", nil)
		resp.Body.Status = "degraded"
		resp.Body.Database = "error"
		resp.Status = stdhttp.StatusServiceUnavailable
	}

	if s.generator == nil {
		resp.Body.Status = "degraded"
		resp.Body.Generator = "unconfigured"
		if resp.Status == 0 {
			resp.Status = stdhttp.StatusServiceUnavailable
		}
	}

	if resp.Status == 0 {
		resp.Status = stdhttp.StatusOK
	}

	return resp, nil
}

func newHTMLResponse(status int, body []byte) *htmlResponse {
	return &htmlResponse{
		Status:      status,
		ContentType: htmlContentType,
		Body:        body,
	}
}

func htmlOperation(summary string, statuses ...int) func(op *huma.Operation) {
	return func(op *huma.Operation) {
		if summary != "" {
			op.Summary = summary
		}
		if op.Responses == nil {
			op.Responses = map[string]*huma.Response{}
		}

		statusCodes := append([]int{stdhttp.StatusOK}, statuses...)
		for _, status := range statusCodes {
			code := strconv.Itoa(status)
			op.Responses[code] = &huma.Response{
				Description: stdhttp.StatusText(status),
				Content: map[string]*huma.MediaType{
					htmlContentType: {
						Schema: &huma.Schema{Type: "string"},
					},
				},
			}
		}
	}
}

func classifyError(err error) (int, string) {
	if err == nil {
		return stdhttp.StatusInternalServerError, errorFallbackMessage
	}

	cause := strings.ToLower(eris.Cause(err).Error())
	switch {
	case strings.Contains(cause, "slug is required"):
		return stdhttp.StatusBadRequest, "A wiki slug is required to load a page."
	case strings.Contains(cause, "query is required"):
		return stdhttp.StatusBadRequest, "Enter a search query to explore Lucipedia."
	case strings.Contains(cause, "refus") || strings.Contains(cause, "blocked"):
		return stdhttp.StatusNotFound, "The requested page is not available yet."
	case strings.Contains(cause, "not found"):
		return stdhttp.StatusNotFound, "We couldn't find that page. Try following a different link."
	default:
		return stdhttp.StatusInternalServerError, errorFallbackMessage
	}
}

func (s *Server) renderErrorResponse(ctx context.Context, status int, message string) (*htmlResponse, error) {
	label := fmt.Sprintf("%d %s", status, stdhttp.StatusText(status))
	title := fmt.Sprintf("%s • Lucipedia", label)
	template := templates.ErrorPage(templates.ErrorPageData{
		Title:       title,
		StatusLabel: label,
		Message:     message,
	})

	body, err := renderComponent(ctx, template)
	if err != nil {
		s.recordError(ctx, err, "rendering error page", logrus.Fields{"status": status})
		fallback := []byte(fmt.Sprintf("<html><body><h1>%s</h1><p>%s</p></body></html>", label, message))
		return newHTMLResponse(status, fallback), nil
	}

	return newHTMLResponse(status, body), nil
}

func (s *Server) recordError(ctx context.Context, err error, message string, fields logrus.Fields) {
	if err == nil {
		return
	}

	if s.logger != nil {
		entry := s.logger.WithField("error", err.Error())
		if fields != nil {
			entry = entry.WithFields(fields)
		}
		if requestID := RequestIDFromContext(ctx); requestID != "" {
			entry = entry.WithField("request_id", requestID)
		}
		entry.Error(message)
	}

	if hub := sentry.GetHubFromContext(ctx); hub != nil {
		hub.CaptureException(err)
		return
	}
	if s.sentry != nil {
		s.sentry.CaptureException(err)
	}
}

func formatCount(count int64) string {
	return fmt.Sprintf("%d", count)
}
