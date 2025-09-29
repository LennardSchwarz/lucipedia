package llm

import (
	"context"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/shared/constant"
	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"
)

func TestSearcherReturnsCleanSlugs(t *testing.T) {
	t.Parallel()

	chatResponse := &openai.ChatCompletion{
		ID:      "search-1",
		Created: time.Now().Unix(),
		Model:   "test-model",
		Object:  constant.ValueOf[constant.ChatCompletion](),
		Choices: []openai.ChatCompletionChoice{
			{
				FinishReason: "stop",
				Index:        0,
				Message: openai.ChatCompletionMessage{
					Content: "Paris, Seine River, Louvre Museum",
					Role:    constant.ValueOf[constant.Assistant](),
				},
			},
		},
	}

	chat := &fakeChatService{response: chatResponse}
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	client := &Client{chat: chat, logger: logger, baseURL: fakeBaseURL}

	searcher, err := NewSearcher(SearcherOptions{Client: client, Model: "lucipedia-search"})
	if err != nil {
		t.Fatalf("NewSearcher returned error: %v", err)
	}

	slugs, err := searcher.Search(context.Background(), "paris", 5)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}

	expected := []string{"paris", "seine-river", "louvre-museum"}
	if !reflect.DeepEqual(slugs, expected) {
		t.Fatalf("expected slugs %v, got %v", expected, slugs)
	}

	if chat.lastParams.Model != "lucipedia-search" {
		t.Fatalf("expected model lucipedia-search, got %s", chat.lastParams.Model)
	}
}

func TestSearcherErrorsWhenNoSlugs(t *testing.T) {
	t.Parallel()

	chatResponse := &openai.ChatCompletion{
		ID:      "search-bad-json",
		Created: time.Now().Unix(),
		Model:   "test-model",
		Object:  constant.ValueOf[constant.ChatCompletion](),
		Choices: []openai.ChatCompletionChoice{
			{
				FinishReason: "stop",
				Index:        0,
				Message: openai.ChatCompletionMessage{
					Content: "```text\n\n```",
					Role:    constant.ValueOf[constant.Assistant](),
				},
			},
		},
	}

	chat := &fakeChatService{response: chatResponse}
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	client := &Client{chat: chat, logger: logger, baseURL: fakeBaseURL}

	searcher, err := NewSearcher(SearcherOptions{Client: client, Model: "lucipedia-search"})
	if err != nil {
		t.Fatalf("NewSearcher returned error: %v", err)
	}

	if _, err := searcher.Search(context.Background(), "query", 3); err == nil {
		t.Fatalf("expected error when response contains no slugs")
	}
}

func TestSearcherRequiresQuery(t *testing.T) {
	t.Parallel()

	chat := &fakeChatService{}
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	client := &Client{chat: chat, logger: logger, baseURL: fakeBaseURL}

	searcher, err := NewSearcher(SearcherOptions{Client: client, Model: "lucipedia-search"})
	if err != nil {
		t.Fatalf("NewSearcher returned error: %v", err)
	}

	if _, err := searcher.Search(context.Background(), " ", 3); err == nil {
		t.Fatalf("expected error when query is empty")
	}
}

func TestNewSearcherRequiresClient(t *testing.T) {
	t.Parallel()

	if _, err := NewSearcher(SearcherOptions{Model: "model"}); err == nil {
		t.Fatalf("expected error when client is nil")
	}
}

func TestNewSearcherRequiresModel(t *testing.T) {
	t.Parallel()

	chat := &fakeChatService{}
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	client := &Client{chat: chat, logger: logger, baseURL: fakeBaseURL}

	if _, err := NewSearcher(SearcherOptions{Client: client}); err == nil {
		t.Fatalf("expected error when model is empty")
	}
}

func TestSearcherHandlesAPIErrors(t *testing.T) {
	t.Parallel()

	svcErr := eris.New("api search failure")
	chat := &fakeChatService{err: svcErr}
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	client := &Client{chat: chat, logger: logger, baseURL: fakeBaseURL}

	searcher, err := NewSearcher(SearcherOptions{Client: client, Model: "lucipedia-search"})
	if err != nil {
		t.Fatalf("NewSearcher returned error: %v", err)
	}

	if _, err := searcher.Search(context.Background(), "query", 3); err == nil {
		t.Fatalf("expected error when api call fails")
	}
}

func TestSearcherLive(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(os.Stdout)

	if err := godotenv.Load(); err != nil {
		t.Logf("%v", eris.Wrap(err, "loading .env file"))
	}

	if os.Getenv("LLM_LIVE_TEST") != "1" {
		t.Skip("live searcher test disabled; set LLM_LIVE_TEST=1 to enable")
	}

	apiKey := strings.TrimSpace(os.Getenv("LLM_API_KEY"))
	if apiKey == "" {
		t.Skip("LLM_API_KEY is required for the live searcher test")
	}

	baseURL := strings.TrimSpace(os.Getenv("LLM_ENDPOINT"))
	if baseURL == "" {
		t.Skip("LLM_ENDPOINT is required for the live searcher test")
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

	searcher, err := NewSearcher(SearcherOptions{Client: client, Model: model})
	if err != nil {
		t.Fatalf("failed to create live searcher: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	query := "paris"
	start := time.Now()
	slugs, err := searcher.Search(ctx, query, 10)
	duration := time.Since(start)
	if err != nil {
		t.Fatalf("live searcher call failed: %v", err)
	}

	if len(slugs) == 0 {
		t.Fatalf("live searcher returned no slugs")
	}

	preview := slugs
	if len(preview) > 5 {
		preview = preview[:5]
	}

	t.Logf("LLM model %q responded in %s (slugs=%d)", model, duration, len(slugs))
	t.Logf("Slugs: %s", strings.Join(preview, ", "))
}
