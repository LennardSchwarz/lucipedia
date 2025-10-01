package openai

import (
	"context"
	"fmt"
	"strings"

	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/shared"
	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"

	domainllm "lucipedia/app/internal/domain/llm"
)

// SearcherOptions configures the OpenRouter-backed searcher.
type SearcherOptions struct {
	Client       *Client
	Model        string
	Temperature  float64
	SystemPrompt string
}

type searcher struct {
	client       *Client
	logger       *logrus.Logger
	model        string
	temperature  float64
	systemPrompt string
}

const (
	defaultSearcherSystemPrompt = "You write url slugs for a Wikipedia-like encyclopedia called Lucipedia. You are an expert at finding relevant pages for user queries. Given a user query, respond with the requested number of relevant slugs, separated by commas. Slugs must be lowercase, words separated by hyphens, and must not include any additional explanation. Example response: history-of-rome, world-war-ii, albert-einstein"
	defaultSearcherTemperature  = 0.2
)

// NewSearcher constructs a Searcher implementation backed by OpenRouter.
func NewSearcher(opts SearcherOptions) (domainllm.Searcher, error) {
	if opts.Client == nil {
		return nil, eris.New("llm client is required")
	}

	model := strings.TrimSpace(opts.Model)
	if model == "" {
		return nil, eris.New("search model is required")
	}

	temperature := opts.Temperature
	if temperature <= 0 {
		temperature = defaultSearcherTemperature
	}

	systemPrompt := strings.TrimSpace(opts.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = defaultSearcherSystemPrompt
	}

	return &searcher{
		client:       opts.Client,
		logger:       opts.Client.logger,
		model:        model,
		temperature:  temperature,
		systemPrompt: systemPrompt,
	}, nil
}

func (s *searcher) Search(ctx context.Context, query string, numResults int) ([]string, error) {
	trimmedQuery := strings.TrimSpace(query)
	if trimmedQuery == "" {
		return nil, eris.New("query is required")
	}

	if numResults <= 0 {
		return nil, eris.New("number of results must be positive")
	}

	params := openai.ChatCompletionNewParams{
		Model: shared.ChatModel(s.model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(s.systemPrompt),
			openai.UserMessage(fmt.Sprintf("Query: %s\nReturn %d relevant url slugs separated by commas.", trimmedQuery, numResults)),
		},
		Temperature: openai.Float(s.temperature),
	}

	completion, err := s.client.chat.New(ctx, params)
	if err != nil {
		s.logError(logrus.Fields{"query": trimmedQuery}, err, "requesting search completion")
		return nil, eris.Wrap(err, "requesting search completion")
	}

	if len(completion.Choices) == 0 {
		err := eris.New("llm completion returned no choices")
		s.logError(logrus.Fields{"query": trimmedQuery}, err, "search completion empty")
		return nil, err
	}

	choice := completion.Choices[0]
	if reason := strings.TrimSpace(choice.FinishReason); strings.EqualFold(reason, "content_filter") {
		err := eris.New("llm blocked the search via content filter")
		s.logError(logrus.Fields{"query": trimmedQuery}, err, "search blocked")
		return nil, err
	}

	if refusal := strings.TrimSpace(choice.Message.Refusal); refusal != "" {
		err := eris.Errorf("llm refused to perform search: %s", refusal)
		s.logError(logrus.Fields{"query": trimmedQuery}, err, "search refused")
		return nil, err
	}

	content := strings.TrimSpace(choice.Message.Content)
	if content == "" {
		err := eris.New("llm search response is empty")
		s.logError(logrus.Fields{"query": trimmedQuery}, err, "empty search response")
		return nil, err
	}

	rawSlugs := extractCommaSeparated(content)
	if len(rawSlugs) == 0 {
		err := eris.New("llm search returned no slugs")
		s.logError(logrus.Fields{"query": trimmedQuery}, err, "empty search list")
		return nil, err
	}

	cleaned := make([]string, 0, len(rawSlugs))
	for _, slug := range rawSlugs {
		normalized := normalizeSlug(slug)
		if normalized == "" {
			continue
		}
		cleaned = append(cleaned, normalized)
	}

	if len(cleaned) == 0 {
		err := eris.New("llm search returned no valid slugs")
		s.logError(logrus.Fields{"query": trimmedQuery}, err, "no valid search slugs")
		return nil, err
	}

	return cleaned, nil
}

func (s *searcher) logError(fields logrus.Fields, err error, message string) {
	if s.logger == nil || err == nil {
		return
	}

	entry := s.logger.WithField("error", err.Error())
	if len(fields) > 0 {
		entry = entry.WithFields(fields)
	}
	entry.Error(message)
}

// TODO: review this AI generated verbose mumbo jumbo
func extractCommaSeparated(content string) []string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return nil
	}

	if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```")
		trimmed = strings.TrimSpace(trimmed)
		if idx := strings.Index(trimmed, "\n"); idx >= 0 {
			trimmed = trimmed[idx+1:]
		}
		trimmed = strings.TrimSpace(trimmed)
		if strings.HasSuffix(trimmed, "```") {
			trimmed = strings.TrimSpace(trimmed[:len(trimmed)-3])
		}
	}

	replacer := strings.NewReplacer("\n", ",", ";", ",")
	trimmed = replacer.Replace(trimmed)

	parts := strings.Split(trimmed, ",")
	results := make([]string, 0, len(parts))
	for _, part := range parts {
		cleaned := strings.TrimSpace(part)
		if cleaned == "" {
			continue
		}
		results = append(results, cleaned)
	}

	return results
}

func normalizeSlug(slug string) string {
	trimmed := strings.TrimSpace(slug)
	trimmed = strings.Trim(trimmed, "\"'")
	if trimmed == "" {
		return ""
	}

	lowered := strings.ToLower(trimmed)
	lowered = strings.ReplaceAll(lowered, "_", "-")
	lowered = strings.Join(strings.FieldsFunc(lowered, func(r rune) bool {
		return r == ' ' || r == '\n' || r == '\t'
	}), "-")
	lowered = strings.Trim(lowered, "-")
	for strings.Contains(lowered, "--") {
		lowered = strings.ReplaceAll(lowered, "--", "-")
	}

	lowered = strings.Trim(lowered, "-.,;:!?")

	return lowered
}

var _ domainllm.Searcher = (*searcher)(nil)
