package llm

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/joho/godotenv"

	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
	"github.com/openai/openai-go/v2/shared/constant"
	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"
)

type fakeChatService struct {
	response   *openai.ChatCompletion
	err        error
	lastParams openai.ChatCompletionNewParams
}

var fakeBaseURL = "https://fake-llm-provider.ai/api/v1"

func (f *fakeChatService) New(ctx context.Context, body openai.ChatCompletionNewParams, opts ...option.RequestOption) (*openai.ChatCompletion, error) {
	f.lastParams = body
	if f.err != nil {
		return nil, f.err
	}
	return f.response, nil
}

func TestGeneratorProducesHTMLAndBacklinks(t *testing.T) {
	t.Parallel()

	chatResponse := &openai.ChatCompletion{
		ID:      "gen-1",
		Created: time.Now().Unix(),
		Model:   "test-model",
		Object:  constant.ValueOf[constant.ChatCompletion](),
		Choices: []openai.ChatCompletionChoice{
			{
				FinishReason: "stop",
				Index:        0,
				Logprobs: openai.ChatCompletionChoiceLogprobs{
					Content: []openai.ChatCompletionTokenLogprob{},
					Refusal: []openai.ChatCompletionTokenLogprob{},
				},
				Message: openai.ChatCompletionMessage{
					Content: "\n<p>Example about <a href=\"/wiki/alpha\">Alpha</a> and <a href=\"/wiki/beta\">Beta</a>.</p>\n<p>Another link to <a href=\"/wiki/alpha\">Alpha</a>.</p>\n",
					Refusal: "",
					Role:    constant.ValueOf[constant.Assistant](),
				},
			},
		},
	}

	chat := &fakeChatService{response: chatResponse}
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	client := &Client{chat: chat, logger: logger, baseURL: fakeBaseURL}

	generator, err := NewGenerator(GeneratorOptions{Client: client, Model: "llm-stub-model"})
	if err != nil {
		t.Fatalf("NewGenerator returned error: %v", err)
	}

	html, backlinks, err := generator.Generate(context.Background(), " example-slug")
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	expectedHTML := "<p>Example about <a href=\"/wiki/alpha\">Alpha</a> and <a href=\"/wiki/beta\">Beta</a>.</p>\n<p>Another link to <a href=\"/wiki/alpha\">Alpha</a>.</p>"
	if html != expectedHTML {
		t.Fatalf("expected html %q, got %q", expectedHTML, html)
	}

	expectedBacklinks := []string{"alpha", "beta"}
	if len(backlinks) != len(expectedBacklinks) {
		t.Fatalf("expected %d backlinks, got %d", len(expectedBacklinks), len(backlinks))
	}

	for idx, slug := range expectedBacklinks {
		if backlinks[idx] != slug {
			t.Fatalf("expected backlink %q at index %d, got %q", slug, idx, backlinks[idx])
		}
	}

	if chat.lastParams.Model != "llm-stub-model" {
		t.Fatalf("expected model llm-stub-model, got %s", chat.lastParams.Model)
	}

	if len(chat.lastParams.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(chat.lastParams.Messages))
	}

	if chat.lastParams.ResponseFormat.OfJSONSchema != nil {
		t.Fatalf("expected response format to be unset")
	}
}

func TestGeneratorPropagatesAPIError(t *testing.T) {
	t.Parallel()

	svcErr := eris.New("api failure")
	chat := &fakeChatService{err: svcErr}
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	client := &Client{chat: chat, logger: logger, baseURL: fakeBaseURL}

	generator, err := NewGenerator(GeneratorOptions{Client: client, Model: "lucipedia-model"})
	if err != nil {
		t.Fatalf("NewGenerator returned error: %v", err)
	}

	if _, _, err := generator.Generate(context.Background(), "slug"); err == nil {
		t.Fatalf("expected error when chat service returns failure")
	}
}

func TestGeneratorLive(t *testing.T) {
	// THIS TEST NEEDS AN .env FILE ON SAME LEVEL AS THIS TEST FILE. SEE .env.example
	logger := logrus.New()
	logger.SetOutput(os.Stdout)

	err := godotenv.Load()
	if err != nil {
		eris.Wrap(err, "Failed to load .env file")
	}

	if os.Getenv("LLM_LIVE_TEST") != "1" {
		t.Skip("live generator test disabled; set LLM_LIVE_TEST=1 to enable")
	}

	apiKey := strings.TrimSpace(os.Getenv("LLM_API_KEY"))
	if apiKey == "" {
		t.Skip("LLM_API_KEY is required for the live generator test")
	}

	baseURL := strings.TrimSpace(os.Getenv("LLM_ENDPOINT"))

	if baseURL == "" {
		eris.Wrap(err, "LLM_ENDPOINT is required for the live generator test")
	}

	client, err := NewClient(ClientOptions{APIKey: apiKey, BaseURL: baseURL, Logger: logger})
	if err != nil {
		t.Fatalf("failed to build live client: %v", err)
	}

	model := ""
	modelCandidates := strings.TrimSpace(os.Getenv("LLM_MODELS"))
	if modelCandidates != "" {
		trimmed := strings.Trim(modelCandidates, "[]")
		for _, candidate := range strings.Split(trimmed, ",") {
			candidate = strings.TrimSpace(candidate)
			candidate = strings.Trim(candidate, "\"'")
			if candidate != "" {
				model = candidate
				break
			}
		}
	}

	generator, err := NewGenerator(GeneratorOptions{Client: client, Model: model})
	if err != nil {
		t.Fatalf("failed to create live generator: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	slug := "paris"
	start := time.Now()
	html, backlinks, err := generator.Generate(ctx, slug)
	duration := time.Since(start)
	if err != nil {
		t.Fatalf("live generator call failed: %v", err)
	}

	html = strings.TrimSpace(html)
	if html == "" {
		t.Fatalf("live generator returned empty html")
	}

	preview := html
	const previewLimit = 800
	if len(preview) > previewLimit {
		preview = preview[:previewLimit]
	}

	t.Logf("LLM model %q responded in %s (html length=%d, backlinks=%d)", model, duration, len(html), len(backlinks))
	t.Logf("HTML preview:\n%s", preview)

	for idx, link := range backlinks {
		backlinks[idx] = strings.TrimSpace(link)
	}
	if len(backlinks) > 0 {
		t.Logf("Backlinks: %s", strings.Join(backlinks, ", "))
	}
}


