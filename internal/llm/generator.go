package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/shared"
	"github.com/openai/openai-go/v2/shared/constant"
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
	client         *Client
	logger         *logrus.Logger
	model          string
	temperature    float64
	systemPrompt   string
	responseFormat openai.ChatCompletionNewParamsResponseFormatUnion
}

const (
	defaultGeneratorSystemPrompt = "You are Lucipedia, an AI historian. Produce detailed HTML articles with multiple internal backlinks using <a href=\"/wiki/...\"> links."
	defaultGeneratorTemperature  = 0.7
)

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
		client:         opts.Client,
		logger:         opts.Client.logger,
		model:          model,
		temperature:    temperature,
		systemPrompt:   systemPrompt,
		responseFormat: buildArticleResponseFormat(),
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
			openai.UserMessage(fmt.Sprintf("Write a Lucipedia article for the slug '%s'. Return JSON that matches the provided schema.", trimmedSlug)),
		},
		ResponseFormat: g.responseFormat,
		Temperature:    openai.Float(g.temperature),
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

	payload, err := g.parsePayload(choice.Message.Content)
	if err != nil {
		g.logError(logrus.Fields{"slug": trimmedSlug}, err, "parsing llm response")
		return "", nil, err
	}

	return payload.HTML, payload.Backlinks, nil
}

type generatorPayload struct {
	HTML      string   `json:"html"`
	Backlinks []string `json:"backlinks"`
}

func (g *openRouterGenerator) parsePayload(raw string) (*generatorPayload, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, eris.New("llm response content is empty")
	}

	var payload generatorPayload
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return nil, eris.Wrap(err, "decoding llm response json")
	}

	if strings.TrimSpace(payload.HTML) == "" {
		return nil, eris.New("llm response missing html field")
	}

	if payload.Backlinks == nil {
		payload.Backlinks = []string{}
	}

	return &payload, nil
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

func buildArticleResponseFormat() openai.ChatCompletionNewParamsResponseFormatUnion {
	schema := map[string]any{
		"type":     "object",
		"required": []string{"html", "backlinks"},
		"properties": map[string]any{
			"html": map[string]any{
				"type":        "string",
				"description": "Full HTML document for the Lucipedia page.",
			},
			"backlinks": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
				"description": "Slugs referenced within the article as /wiki/{slug} links.",
			},
		},
	}

	return openai.ChatCompletionNewParamsResponseFormatUnion{
		OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
			JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
				Name:        "lucipedia_article",
				Description: openai.String("Structured Lucipedia article payload"),
				Strict:      openai.Bool(true),
				Schema:      schema,
			},
			Type: constant.ValueOf[constant.JSONSchema](),
		},
	}
}
