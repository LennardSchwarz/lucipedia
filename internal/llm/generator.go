package llm

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/shared"
	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"

	
)

// Generator defines the behaviour for producing Lucipedia page content.
type Generator interface {
	Generate(ctx context.Context, slug string) (string, []string, error)
}

// GeneratorOptions configures the OpenRouter-backed generator.
type GeneratorOptions struct {
	Client       *Client
	Model        string
	Temperature  float64
	SystemPrompt string
}

type openRouterGenerator struct {
	client       *Client
	logger       *logrus.Logger
	model        string
	temperature  float64
	systemPrompt string
}

const (
	defaultGeneratorSystemPrompt = "You are an expert historian who works on an wikipedia clone called lucipedia. Produce detailed HTML articles with multiple internal backlinks using <a href=\"/wiki/...\"> links. Respond with valid HTML only."
	defaultGeneratorTemperature  = 0.4
)

var wikiLinkPattern = regexp.MustCompile(`href="/wiki/([^"#?]+)"`)

// NewGenerator constructs a Generator implementation backed by OpenRouter.
func NewGenerator(opts GeneratorOptions) (Generator, error) {
	if opts.Client == nil {
		return nil, eris.New("llm client is required")
	}

	model := strings.TrimSpace(opts.Model)
	if model == "" {
		return nil, eris.New("generator model is required")
	}

	temperature := opts.Temperature
	if temperature <= 0 {
		temperature = defaultGeneratorTemperature
	}

	systemPrompt := strings.TrimSpace(opts.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = defaultGeneratorSystemPrompt
	}

	return &openRouterGenerator{
		client:       opts.Client,
		logger:       opts.Client.logger,
		model:        model,
		temperature:  temperature,
		systemPrompt: systemPrompt,
	}, nil
}

func (g *openRouterGenerator) Generate(ctx context.Context, slug string) (string, []string, error) {
	trimmedSlug := strings.TrimSpace(slug)
	if trimmedSlug == "" {
		return "", nil, eris.New("slug is required")
	}

	params := openai.ChatCompletionNewParams{
		Model: shared.ChatModel(g.model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(g.systemPrompt),
			openai.UserMessage(fmt.Sprintf("Write a Lucipedia article for the slug '%s'. Respond with only valid HTML.", trimmedSlug)),
		},
		Temperature: openai.Float(g.temperature),
	}

	completion, err := g.client.chat.New(ctx, params)
	if err != nil {
		g.logError(logrus.Fields{"slug": trimmedSlug}, err, "requesting chat completion")
		return "", nil, eris.Wrap(err, "requesting chat completion")
	}

	if len(completion.Choices) == 0 {
		err := eris.New("llm completion returned no choices")
		g.logError(logrus.Fields{"slug": trimmedSlug}, err, "processing chat completion")
		return "", nil, err
	}

	choice := completion.Choices[0]
	if reason := strings.TrimSpace(choice.FinishReason); strings.EqualFold(reason, "content_filter") {
		err := eris.New("llm blocked the request via content filter")
		g.logError(logrus.Fields{"slug": trimmedSlug}, err, "generator blocked")
		return "", nil, err
	}

	if refusal := strings.TrimSpace(choice.Message.Refusal); refusal != "" {
		err := eris.Errorf("llm refused to generate content: %s", refusal)
		g.logError(logrus.Fields{"slug": trimmedSlug}, err, "generator refused")
		return "", nil, err
	}

	html := strings.TrimSpace(choice.Message.Content)
	if html == "" {
		err := eris.New("llm response content is empty")
		g.logError(logrus.Fields{"slug": trimmedSlug}, err, "empty llm response")
		return "", nil, err
	}

	backlinks := g.extractBacklinks(html)
	return html, backlinks, nil
}

func (g *openRouterGenerator) logError(fields logrus.Fields, err error, message string) {
	if g.logger == nil || err == nil {
		return
	}

	entry := g.logger.WithField("error", err.Error())
	if len(fields) > 0 {
		entry = entry.WithFields(fields)
	}
	entry.Error(message)
}

func (g *openRouterGenerator) extractBacklinks(html string) []string {
	matches := wikiLinkPattern.FindAllStringSubmatch(html, -1)
	if len(matches) == 0 {
		return []string{}
	}

	seen := make(map[string]struct{}, len(matches))
	backlinks := make([]string, 0, len(matches))

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		slug := strings.TrimSpace(match[1])
		if slug == "" {
			continue
		}

		if _, exists := seen[slug]; exists {
			continue
		}
		seen[slug] = struct{}{}
		backlinks = append(backlinks, slug)
	}

	return backlinks
}
