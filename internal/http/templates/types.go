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
