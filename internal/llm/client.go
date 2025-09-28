package llm

import (
	"context"
	"net/http"
	"strings"

	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"
)

const openRouterBaseURL = "https://openrouter.ai/api/v1"

// ClientOptions controls how the OpenRouter client is initialised.
type ClientOptions struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
	Logger     *logrus.Logger
}

// Client wraps the OpenAI SDK services used by Lucipedia.
type Client struct {
	chat       chatCompletionClient
	embeddings embeddingClient
	logger     *logrus.Logger
	baseURL    string
}

type chatCompletionClient interface {
	New(ctx context.Context, body openai.ChatCompletionNewParams, opts ...option.RequestOption) (*openai.ChatCompletion, error)
}

type embeddingClient interface {
	New(ctx context.Context, body openai.EmbeddingNewParams, opts ...option.RequestOption) (*openai.CreateEmbeddingResponse, error)
}

// NewClient constructs a Client configured for OpenRouter.
func NewClient(opts ClientOptions) (*Client, error) {
	if strings.TrimSpace(opts.APIKey) == "" {
		return nil, eris.New("llm api key is required")
	}

	baseURL := strings.TrimSpace(opts.BaseURL)
	if baseURL == "" {
		baseURL = openRouterBaseURL
	}

	requestOptions := []option.RequestOption{
		option.WithAPIKey(opts.APIKey),
		option.WithBaseURL(baseURL),
	}

	if opts.HTTPClient != nil {
		requestOptions = append(requestOptions, option.WithHTTPClient(opts.HTTPClient))
	}

	apiClient := openai.NewClient(requestOptions...)

	return &Client{
		chat:       &apiClient.Chat.Completions,
		embeddings: &apiClient.Embeddings,
		logger:     opts.Logger,
		baseURL:    baseURL,
	}, nil
}

// Logger exposes the logger associated with the client.
func (c *Client) Logger() *logrus.Logger {
	return c.logger
}

// BaseURL returns the configured base URL for outbound requests.
func (c *Client) BaseURL() string {
	return c.baseURL
}
