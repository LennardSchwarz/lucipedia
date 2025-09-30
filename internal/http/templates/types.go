package templates

import "context"

type pageCountKey struct{}

// WithPageCount stores the formatted page count in the provided context for later retrieval during template rendering.
func WithPageCount(ctx context.Context, count string) context.Context {
	return context.WithValue(ctx, pageCountKey{}, count)
}

// PageCountFromContext returns the formatted page count if it was previously set via WithPageCount.
func PageCountFromContext(ctx context.Context) string {
	value, _ := ctx.Value(pageCountKey{}).(string)
	return value
}

// HomePageData contains dynamic values rendered on the landing page.
type HomePageData struct {
	FormattedPageCount string
}

// PageListEntry represents a link to a wiki page used in list views.
type PageListEntry struct {
	Title string
	URL   string
}

// AllPagesPageData contains data required to render the list of all wiki pages.
type AllPagesPageData struct {
	Pages []PageListEntry
}

// SearchResultView represents an individual search result entry.
type SearchResultView struct {
	Title string
	URL   string
}

// SearchPageData bundles template data for the search results page.
type SearchPageData struct {
	Query        string
	Results      []SearchResultView
	ErrorMessage string
}

// SearchStreamingShellData holds information required to render the streaming shell for search results.
type SearchStreamingShellData struct {
	Title          string
	Query          string
	LoadingMessage string
}

// SearchStreamingContentData wraps the dynamic search data for streaming.
type SearchStreamingContentData struct {
	Page SearchPageData
}

// SearchStreamingErrorData represents an inline error inside the streamed search results.
type SearchStreamingErrorData struct {
	Title   string
	Message string
}

// ErrorPageData holds information for rendering an error view.
type ErrorPageData struct {
	StatusLabel string
	Message     string
}

// WikiPageData contains the dynamic values for a generated wiki entry.
type WikiPageData struct {
	Title string
	HTML  string
}

// WikiStreamingShellData holds information for the initial streamed layout.
type WikiStreamingShellData struct {
	Title          string
	LoadingMessage string
}

// WikiStreamingContentData wraps the generated wiki HTML for streaming.
type WikiStreamingContentData struct {
	HTML string
}

// WikiStreamingErrorData represents an inline error message for streaming.
type WikiStreamingErrorData struct {
	Title   string
	Message string
}
