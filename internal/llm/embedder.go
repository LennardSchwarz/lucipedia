package llm

import (
	"context"
	"strings"

	"github.com/openai/openai-go/v2"
	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"
)

// Embedder embeds page and query content for Lucipedia search.
type Embedder interface {
	EmbedPage(ctx context.Context, slug string, html string) ([]float32, error)
	EmbedQuery(ctx context.Context, query string) ([]float32, error)
}

// EmbedderOptions configures the OpenRouter-backed embedder.
type EmbedderOptions struct {
	Client *Client
	Model  string
}

type openRouterEmbedder struct {
	client *Client
	logger *logrus.Logger
	model  string
}

// NewEmbedder constructs an Embedder implementation.
func NewEmbedder(opts EmbedderOptions) (Embedder, error) {
	if opts.Client == nil {
		return nil, eris.New("llm client is required")
	}

	model := strings.TrimSpace(opts.Model)
	if model == "" {
		return nil, eris.New("embedder model is required")
	}

	return &openRouterEmbedder{
		client: opts.Client,
		logger: opts.Client.logger,
		model:  model,
	}, nil
}

func (e *openRouterEmbedder) EmbedPage(ctx context.Context, slug string, html string) ([]float32, error) {
	trimmedSlug := strings.TrimSpace(slug)
	if trimmedSlug == "" {
		return nil, eris.New("slug is required")
	}

	trimmedHTML := strings.TrimSpace(html)
	if trimmedHTML == "" {
		return nil, eris.New("page html is required")
	}

	input := trimmedHTML
	return e.embed(ctx, input, logrus.Fields{"slug": trimmedSlug})
}

func (e *openRouterEmbedder) EmbedQuery(ctx context.Context, query string) ([]float32, error) {
	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return nil, eris.New("query is required")
	}

	return e.embed(ctx, trimmedQuery, logrus.Fields{"query": trimmedQuery})
}

func (e *openRouterEmbedder) embed(ctx context.Context, input string, fields logrus.Fields) ([]float32, error) {
	params := openai.EmbeddingNewParams{
		Model: openai.EmbeddingModel(e.model),
		Input: openai.EmbeddingNewParamsInputUnion{OfString: openai.String(input)},
	}

	response, err := e.client.embeddings.New(ctx, params)
	if err != nil {
		e.logError(fields, err, "requesting embedding")
		return nil, eris.Wrap(err, "requesting embedding")
	}

	if response == nil || len(response.Data) == 0 {
		err := eris.New("embedding response contained no vectors")
		e.logError(fields, err, "processing embedding response")
		return nil, err
	}

	vector := response.Data[0].Embedding
	if len(vector) == 0 {
		err := eris.New("embedding vector was empty")
		e.logError(fields, err, "processing embedding response")
		return nil, err
	}

	converted := make([]float32, len(vector))
	for i, value := range vector {
		converted[i] = float32(value)
	}

	return converted, nil
}

func (e *openRouterEmbedder) logError(fields logrus.Fields, err error, message string) {
	if e.logger == nil || err == nil {
		return
	}

	entry := e.logger.WithField("error", err.Error())
	if len(fields) > 0 {
		entry = entry.WithFields(fields)
	}
	entry.Error(message)
}
