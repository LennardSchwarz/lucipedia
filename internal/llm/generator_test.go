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
					Content: "<div>\n<p>Example about <a href=\"/wiki/alpha\">Alpha</a> and <a href=\"/wiki/beta\">Beta</a>.</p>\n<p>Another link to <a href=\"/wiki/alpha\">Alpha</a>.</p>\n</div>",
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

	expectedHTML := "<div>\n<p>Example about <a href=\"/wiki/alpha\">Alpha</a> and <a href=\"/wiki/beta\">Beta</a>.</p>\n<p>Another link to <a href=\"/wiki/alpha\">Alpha</a>.</p>\n</div>"
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

func TestCleanGeneratedHTMLConvertsDocumentToDiv(t *testing.T) {
	t.Parallel()

	input := `<html><body><header class="hero"><h1>Title</h1></header><main><p>Body</p></main></body></html>`
	cleaned, err := cleanGeneratedHTML(input)
	if err != nil {
		t.Fatalf("cleanGeneratedHTML returned error: %v", err)
	}

	const expected = `<div><div class="hero"><h1>Title</h1></div><main><p>Body</p></main></div>`
	if cleaned != expected {
		t.Fatalf("expected cleaned html %q, got %q", expected, cleaned)
	}
}

func TestCleanGeneratedHTMLPreservesInlineWhitespace(t *testing.T) {
	t.Parallel()

	input := `<body><p><span>Alpha</span> <span>Beta</span></p></body>`
	cleaned, err := cleanGeneratedHTML(input)
	if err != nil {
		t.Fatalf("cleanGeneratedHTML returned error: %v", err)
	}

	const expected = `<div><p><span>Alpha</span> <span>Beta</span></p></div>`
	if cleaned != expected {
		t.Fatalf("expected inline whitespace preserved, got %q", cleaned)
	}
}

func TestCleanGeneratedHTMLStripsCodeFence(t *testing.T) {
	t.Parallel()

	input := "```html\n<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n    <meta charset=\"UTF-8\">\n    <meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n    <title>Paris - Lucipedia</title>\n</head>\n<body>\n    <h1>Paris</h1>\n    <p><strong>Paris</strong> is the capital and most populous city of <a href=\"/wiki/France\">France</a>, with an estimated population of 2.1 million people in the city proper. Since the 17th century, Paris has been one of the world's major centers of finance, diplomacy, commerce, fashion, gastronomy, science, and the arts. The <a href=\"/wiki/Paris\">City of Light</a> is a global center for art, fashion, gastronomy, and culture, and is one of the world's major global cities.</p>\n\n    <h2>History</h2>\n    <p>Paris has a rich history dating back to ancient times. Originally a Celtic settlement, it was later conquered by the Romans and named <em>Lutetia</em>. Over the centuries, Paris grew into a major political and cultural center, playing a pivotal role in European history. The city was the site of many significant events, including the <a href=\"/wiki/French_Revolution\">French Revolution</a> and both World Wars.</p>\n\n    <h2>Landmarks</h2>\n    <p>Paris is home to some of the world's most iconic landmarks, including the <a href=\"/wiki/Eiffel_Tower\">Eiffel Tower</a>, the <a href=\"/wiki/Louvre\">Louvre Museum</a>, and <a href=\"/wiki/Notre-Dame_Cathedral\">Notre-Dame Cathedral</a>. The city's architecture is a blend of historic and modern styles, with notable districts like the <a href=\"/wiki/Le_Marais\">Le Marais</a> and the <a href=\"/wiki/Champs-Élysées\">Champs-Élysées</a>.</p>\n\n    <h2>Culture</h2>\n    <p>Paris is renowned for its cultural institutions, including the <a href=\"/wiki/Opéra_Garnier\">Opéra Garnier</a> and the <a href=\"/wiki/Palace_of_Versailles\">Palace of Versailles</a>. The city is also famous for its cuisine, fashion houses, and vibrant art scene. It has been a haven for artists, writers, and thinkers throughout history.</p>\n\n    <h2>See Also</h2>\n    <ul>\n        <li><a href=\"/wiki/French_Revolution\">French Revolution</a></li>\n        <li><a href=\"/wiki/Eiffel_Tower\">Eiffel Tower</a></li>\n        <li><a href=\"/wiki/Louvre\">Louvre Museum</a></li>\n        <li><a href=\"/wiki/Notre-Dame_Cathedral\">Notre-Dame Cathedral</a></li>\n        <li><a href=\"/wiki/Le_Marais\">Le Marais</a></li>\n        <li><a href=\"/wiki/Champs-Élysées\">Champs-Élysées</a></li>\n        <li><a href=\"/wiki/Opéra_Garnier\">Opéra Garnier</a></li>\n        <li><a href=\"/wiki/Palace_of_Versailles\">Palace of Versailles</a></li>\n    </ul>\n</body>\n</html>\n```"

	cleaned, err := cleanGeneratedHTML(input)
	if err != nil {
		t.Fatalf("cleanGeneratedHTML returned error: %v", err)
	}

	if strings.Contains(cleaned, "```") {
		t.Fatalf("expected code fences to be removed, got %q", cleaned)
	}

	if !strings.HasPrefix(cleaned, "<div>") {
		t.Fatalf("expected cleaned html to start with <div>, got %q", cleaned)
	}

	if !strings.Contains(cleaned, "<h1>Paris</h1>") {
		t.Fatalf("expected cleaned html to contain Paris heading, got %q", cleaned)
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

	if err := godotenv.Load(); err != nil {
		t.Logf("%v", eris.Wrap(err, "loadig .env file"))
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
		t.Skip("LLM_ENDPOINT is required for the live generator test")
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
