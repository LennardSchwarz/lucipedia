package wiki

// Page represents a Lucipedia entry within the domain layer.
type Page struct {
	Slug string
	HTML string
}

// SearchResult represents a wiki entry returned by search operations.
type SearchResult struct {
	Slug string
}
