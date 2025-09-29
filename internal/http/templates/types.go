package templates

// DefaultFooterNote is shown in the shared layout when a page does not supply custom text.
const DefaultFooterNote = "Lucipedia pages are generated on demand. Internal links create new articles the first time they are visited."

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
