package llm

import "context"

// Generator produces Lucipedia wiki pages and their backlinks for a given slug.
type Generator interface {
	Generate(ctx context.Context, slug string) (string, []string, error)
}

// Searcher returns suggested slugs based on a freeform search query.
type Searcher interface {
	Search(ctx context.Context, query string, limit int) ([]string, error)
}
