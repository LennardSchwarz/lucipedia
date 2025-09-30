package http

import (
	"context"
	"errors"
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

func (s *Server) registerAllPagesRoute() {
	huma.Get(s.api, "/all", s.allPagesHandler, htmlOperation(
		"List all Lucipedia pages",
		stdhttp.StatusInternalServerError,
	))
}

func (s *Server) registerRandomRoute() {
	huma.Get(s.api, "/random", s.randomHandler, htmlOperation(
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

	formattedCount := formatCount(count)
	data := templates.HomePageData{
		FormattedPageCount: formattedCount,
	}

	renderCtx := templates.WithPageCount(ctx, formattedCount)
	body, err := renderComponent(renderCtx, templates.HomePage(data))
	if err != nil {
		s.recordError(ctx, err, "rendering home page", nil)
		return s.renderErrorResponse(ctx, stdhttp.StatusInternalServerError, "We couldn't render the homepage.")
	}

	return newHTMLResponse(stdhttp.StatusOK, body), nil
}

func (s *Server) allPagesHandler(ctx context.Context, _ *struct{}) (*htmlResponse, error) {
	pages, err := s.repository.ListPages(ctx)
	if err != nil {
		s.recordError(ctx, err, "listing wiki pages", nil)
		return s.renderErrorResponse(ctx, stdhttp.StatusInternalServerError, "We couldn't load the list of pages.")
	}

	entries := make([]templates.PageListEntry, 0, len(pages))
	for _, page := range pages {
		slug := strings.TrimSpace(page.Slug)
		if slug == "" {
			s.recordError(ctx, eris.New("page slug is empty"), "validating page for listing", logrus.Fields{"page_id": page.ID})
			continue
		}

		entries = append(entries, templates.PageListEntry{
			Title: slug,
			URL:   "/wiki/" + slug,
		})
	}

	data := templates.AllPagesPageData{Pages: entries}
	renderCtx := s.contextWithPageCount(ctx, logrus.Fields{"pages": len(entries)})
	body, err := renderComponent(renderCtx, templates.AllPagesPage(data))
	if err != nil {
		s.recordError(ctx, err, "rendering all pages view", nil)
		return s.renderErrorResponse(ctx, stdhttp.StatusInternalServerError, "We couldn't render the all pages view.")
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

	if page == nil {
		err := eris.Wrap(wiki.ErrNoPages, "most recent page is unavailable")
		s.recordError(ctx, err, "loading most recent page", nil)
		return s.renderErrorResponse(ctx, stdhttp.StatusNotFound, "Lucipedia doesn't have any pages yet. Follow a link to generate the first article.")
	}

	slug := strings.TrimSpace(page.Slug)
	html := strings.TrimSpace(page.HTML)
	if html == "" {
		err := eris.New("most recent page html is empty")
		s.recordError(ctx, err, "validating most recent page html", logrus.Fields{"slug": slug})
		return s.renderErrorResponse(ctx, stdhttp.StatusInternalServerError, errorFallbackMessage)
	}

	title := "Lucipedia"
	if slug != "" {
		title = fmt.Sprintf("%s • Lucipedia", slug)
	}

	data := templates.WikiPageData{
		Title: title,
		HTML:  html,
	}

	renderCtx := s.contextWithPageCount(ctx, logrus.Fields{"slug": slug})
	body, err := renderComponent(renderCtx, templates.WikiPage(data))
	if err != nil {
		s.recordError(ctx, err, "rendering most recent page", logrus.Fields{"slug": slug})
		return s.renderErrorResponse(ctx, stdhttp.StatusInternalServerError, errorFallbackMessage)
	}

	return newHTMLResponse(stdhttp.StatusOK, body), nil
}

func (s *Server) wikiHandler(ctx context.Context, input *wikiInput) (*huma.StreamResponse, error) {
	slug := strings.TrimSpace(input.Slug)
	title := "Lucipedia"
	if slug != "" {
		title = fmt.Sprintf("%s • Lucipedia", slug)
	}

	loadingMessage := "Loading Lucipedia..."
	if slug != "" {
		loadingMessage = fmt.Sprintf("You found a new page! Generating \"%s\"...", slug)
	}

	fields := logrus.Fields{"slug": slug}

	return &huma.StreamResponse{
		Body: func(hctx huma.Context) {
			hctx.SetHeader("Content-Type", htmlContentType)
			hctx.SetStatus(stdhttp.StatusOK)

			writer := hctx.BodyWriter()
			flusher, canFlush := writer.(stdhttp.Flusher)

			renderCtx := s.contextWithPageCount(ctx, fields)

			shell := templates.WikiStreamingShellData{
				Title:          title,
				LoadingMessage: loadingMessage,
			}

			if err := streamComponent(renderCtx, writer, templates.WikiStreamingShell(shell)); err != nil {
				s.recordError(ctx, err, "streaming wiki shell", fields)
				fallback := []byte("<article><p>" + errorFallbackMessage + "</p></article>")
				if _, writeErr := writer.Write(fallback); writeErr != nil {
					s.recordError(ctx, eris.Wrap(writeErr, "writing wiki shell fallback"), "writing wiki shell fallback", fields)
				}
				if canFlush {
					flusher.Flush()
				}
				return
			}

			if canFlush {
				flusher.Flush()
			}

			html, err := s.wiki.GetPage(ctx, slug)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return
				}

				status, message := classifyError(err)
				hctx.SetStatus(status)
				s.recordError(ctx, err, "loading wiki page", fields)

				errData := templates.WikiStreamingErrorData{
					Title:   fmt.Sprintf("%d %s", status, stdhttp.StatusText(status)),
					Message: message,
				}

				if streamErr := streamComponent(renderCtx, writer, templates.WikiStreamingError(errData)); streamErr != nil {
					s.recordError(ctx, streamErr, "streaming wiki error", fields)
				}
				if canFlush {
					flusher.Flush()
				}
				return
			}

			content := templates.WikiStreamingContentData{HTML: html}
			if err := streamComponent(renderCtx, writer, templates.WikiStreamingContent(content)); err != nil {
				s.recordError(ctx, err, "streaming wiki content", fields)
			}
			if canFlush {
				flusher.Flush()
			}
		},
	}, nil
}

func (s *Server) searchHandler(ctx context.Context, input *searchInput) (*huma.StreamResponse, error) {
	query := strings.TrimSpace(input.Query)
	fields := logrus.Fields{"query": query}

	title := "Search • Lucipedia"
	loadingMessage := "Searching Lucipedia..."
	if query != "" {
		loadingMessage = fmt.Sprintf("Searching for \"%s\"...", query)
	}

	return &huma.StreamResponse{
		Body: func(hctx huma.Context) {
			hctx.SetHeader("Content-Type", htmlContentType)
			hctx.SetStatus(stdhttp.StatusOK)

			writer := hctx.BodyWriter()
			flusher, canFlush := writer.(stdhttp.Flusher)

			renderCtx := s.contextWithPageCount(ctx, fields)

			shell := templates.SearchStreamingShellData{
				Title:          title,
				Query:          query,
				LoadingMessage: loadingMessage,
			}

			if err := streamComponent(renderCtx, writer, templates.SearchStreamingShell(shell)); err != nil {
				s.recordError(ctx, err, "streaming search shell", fields)
				fallback := []byte("<article><p>" + errorFallbackMessage + "</p></article>")
				if _, writeErr := writer.Write(fallback); writeErr != nil {
					s.recordError(ctx, eris.Wrap(writeErr, "writing search shell fallback"), "writing search shell fallback", fields)
				}
				if canFlush {
					flusher.Flush()
				}
				return
			}

			if canFlush {
				flusher.Flush()
			}

			pageData := templates.SearchPageData{Query: query}

			if query != "" {
				results, err := s.wiki.Search(ctx, query, searchResultsLimit)
				if err != nil {
					status, message := classifyError(err)
					hctx.SetStatus(status)
					s.recordError(ctx, err, "search request failed", fields)

					if status == stdhttp.StatusBadRequest {
						pageData.ErrorMessage = message
					} else {
						errData := templates.SearchStreamingErrorData{
							Title:   fmt.Sprintf("%d %s", status, stdhttp.StatusText(status)),
							Message: message,
						}
						if streamErr := streamComponent(renderCtx, writer, templates.SearchStreamingError(errData)); streamErr != nil {
							s.recordError(ctx, streamErr, "streaming search error", fields)
						}
						if canFlush {
							flusher.Flush()
						}
						return
					}
				} else {
					pageData.Results = make([]templates.SearchResultView, 0, len(results))
					for _, result := range results {
						pageData.Results = append(pageData.Results, templates.SearchResultView{
							Title: result.Slug,
							URL:   "/wiki/" + result.Slug,
						})
					}
				}
			}

			content := templates.SearchStreamingContentData{Page: pageData}
			if err := streamComponent(renderCtx, writer, templates.SearchStreamingContent(content)); err != nil {
				s.recordError(ctx, err, "streaming search content", fields)
			}
			if canFlush {
				flusher.Flush()
			}
		},
	}, nil
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

func (s *Server) contextWithPageCount(ctx context.Context, fields logrus.Fields) context.Context {
	if s == nil || s.repository == nil {
		return ctx
	}
	count, err := s.repository.CountPages(ctx)
	if err != nil {
		s.recordError(ctx, err, "counting pages for layout", fields)
		return ctx
	}
	return templates.WithPageCount(ctx, formatCount(count))
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
	template := templates.ErrorPage(templates.ErrorPageData{
		StatusLabel: label,
		Message:     message,
	})

	renderCtx := s.contextWithPageCount(ctx, logrus.Fields{"status": status})
	body, err := renderComponent(renderCtx, template)
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
