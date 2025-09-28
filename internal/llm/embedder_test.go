package llm

import (
	"context"
	"io"
	"testing"

	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
	"github.com/openai/openai-go/v2/shared/constant"
	"github.com/sirupsen/logrus"
)

type fakeEmbeddingService struct {
	response   *openai.CreateEmbeddingResponse
	err        error
	lastParams openai.EmbeddingNewParams
}

func (f *fakeEmbeddingService) New(ctx context.Context, body openai.EmbeddingNewParams, opts ...option.RequestOption) (*openai.CreateEmbeddingResponse, error) {
	f.lastParams = body
	if f.err != nil {
		return nil, f.err
	}
	return f.response, nil
}

func TestEmbedderConvertsVectorToFloat32(t *testing.T) {
	t.Parallel()

	embeddingResponse := &openai.CreateEmbeddingResponse{
		Data: []openai.Embedding{
			{
				Embedding: []float64{1.5, 2.5, -0.25},
				Index:     0,
				Object:    constant.ValueOf[constant.Embedding](),
			},
		},
		Model:  "embedding-model",
		Object: constant.ValueOf[constant.List](),
		Usage: openai.CreateEmbeddingResponseUsage{
			PromptTokens: 10,
			TotalTokens:  10,
		},
	}

	embeddings := &fakeEmbeddingService{response: embeddingResponse}
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	client := &Client{embeddings: embeddings, logger: logger, baseURL: openRouterBaseURL}

	embedder, err := NewEmbedder(EmbedderOptions{Client: client, Model: "text-embedding"})
	if err != nil {
		t.Fatalf("NewEmbedder returned error: %v", err)
	}

	vector, err := embedder.EmbedPage(context.Background(), "slug", "<p>HTML</p>")
	if err != nil {
		t.Fatalf("EmbedPage returned error: %v", err)
	}

	expected := []float32{1.5, 2.5, -0.25}
	if len(vector) != len(expected) {
		t.Fatalf("expected %d values, got %d", len(expected), len(vector))
	}

	for idx, value := range expected {
		if vector[idx] != value {
			t.Fatalf("expected value %.2f at index %d, got %.2f", value, idx, vector[idx])
		}
	}

	if embeddings.lastParams.Model != "text-embedding" {
		t.Fatalf("expected model text-embedding, got %s", embeddings.lastParams.Model)
	}
}

func TestEmbedderValidatesInputs(t *testing.T) {
	t.Parallel()

	embeddings := &fakeEmbeddingService{}
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	client := &Client{embeddings: embeddings, logger: logger, baseURL: openRouterBaseURL}

	embedder, err := NewEmbedder(EmbedderOptions{Client: client, Model: "text-embedding"})
	if err != nil {
		t.Fatalf("NewEmbedder returned error: %v", err)
	}

	if _, err := embedder.EmbedPage(context.Background(), "", "content"); err == nil {
		t.Fatalf("expected error when slug empty")
	}

	if _, err := embedder.EmbedPage(context.Background(), "slug", " "); err == nil {
		t.Fatalf("expected error when html empty")
	}

	if _, err := embedder.EmbedQuery(context.Background(), " "); err == nil {
		t.Fatalf("expected error when query empty")
	}
}
