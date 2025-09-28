package llm

import (
	"context"
	"io"
	"testing"
	"time"

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
					Content: `{"html":"<p>Example</p>","backlinks":["alpha","beta"]}`,
					Refusal: "",
					Role:    constant.ValueOf[constant.Assistant](),
				},
			},
		},
	}

	chat := &fakeChatService{response: chatResponse}
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	client := &Client{chat: chat, logger: logger, baseURL: openRouterBaseURL}

	generator, err := NewGenerator(GeneratorOptions{Client: client, Model: "lucipedia-model"})
	if err != nil {
		t.Fatalf("NewGenerator returned error: %v", err)
	}

	html, backlinks, err := generator.Generate(context.Background(), " example ")
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if html != "<p>Example</p>" {
		t.Fatalf("expected html <p>Example</p>, got %q", html)
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

	if chat.lastParams.Model != "lucipedia-model" {
		t.Fatalf("expected model lucipedia-model, got %s", chat.lastParams.Model)
	}

	if len(chat.lastParams.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(chat.lastParams.Messages))
	}
}

func TestGeneratorPropagatesAPIError(t *testing.T) {
	t.Parallel()

	svcErr := eris.New("api failure")
	chat := &fakeChatService{err: svcErr}
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	client := &Client{chat: chat, logger: logger, baseURL: openRouterBaseURL}

	generator, err := NewGenerator(GeneratorOptions{Client: client, Model: "lucipedia-model"})
	if err != nil {
		t.Fatalf("NewGenerator returned error: %v", err)
	}

	if _, _, err := generator.Generate(context.Background(), "slug"); err == nil {
		t.Fatalf("expected error when chat service returns failure")
	}
}
