package templates

// HomePageData contains the dynamic values rendered on the landing page.
type HomePageData struct {
	Title              string
	Subtitle           string
	PageCountLabel     string
	Description        string
	IntroParagraphs    []string
	FooterNote         string
	BuilderAttribution string
}

// SearchResultView represents an individual search result entry.
type SearchResultView struct {
	Title string
	URL   string
}

// SearchPageData bundles template data for the search results page.
type SearchPageData struct {
	Title        string
	Query        string
	Results      []SearchResultView
	ErrorMessage string
}

// ErrorPageData holds information for rendering an error view.
type ErrorPageData struct {
	Title       string
	StatusLabel string
	Message     string
}
