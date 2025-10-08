package openai

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/shared"
	"github.com/rotisserie/eris"
	"github.com/sirupsen/logrus"
	"golang.org/x/net/html"

	domainllm "lucipedia/app/internal/domain/llm"
)

// GeneratorOptions configures the OpenRouter-backed generator.
type GeneratorOptions struct {
	Client       *Client
	Model        string
	Temperature  float64
	SystemPrompt string
}

type generator struct {
	client       *Client
	logger       *logrus.Logger
	model        string
	temperature  float64
	systemPrompt string
}

const (
	defaultGeneratorSystemPrompt = `
	You are an expert historian who works on an wikipedia clone called lucipedia.
	Write in standard encyclopedic tone and format, but present claims and statistics that are inflated or exaggerated versions of reality. State "facts" and convoluted statistics with the same matter-of-fact authority as a regular Wikipedia article. Make made-up claims about influence, scale, and impact that seem overstated, but present them neutrally without dramatic language.
	Produce detailed HTML articles with multiple internal backlinks using <a href=\"/wiki/...\"> links. Respond with valid HTML only. Include a title. Include a summary. Do not include a references section. Include a see also section. Do not include a references section. Max 300 words.`
	defaultGeneratorTemperature  = 0.4
)

var wikiLinkPattern = regexp.MustCompile(`href="/wiki/([^"#?]+)"`)

// NewGenerator constructs a Generator implementation backed by OpenRouter.
func NewGenerator(opts GeneratorOptions) (domainllm.Generator, error) {
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

	return &generator{
		client:       opts.Client,
		logger:       opts.Client.logger,
		model:        model,
		temperature:  temperature,
		systemPrompt: systemPrompt,
	}, nil
}

func (g *generator) Generate(ctx context.Context, slug string) (string, []string, error) {
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

	cleanedHTML, err := cleanGeneratedHTML(html)
	if err != nil {
		err := eris.Wrap(err, "cleaning llm html response")
		g.logError(logrus.Fields{"slug": trimmedSlug}, err, "invalid llm response")
		return "", nil, err
	}

	backlinks := g.extractBacklinks(cleanedHTML)
	return cleanedHTML, backlinks, nil
}

func (g *generator) logError(fields logrus.Fields, err error, message string) {
	if g.logger == nil || err == nil {
		return
	}

	entry := g.logger.WithField("error", err.Error())
	if len(fields) > 0 {
		entry = entry.WithFields(fields)
	}
	entry.Error(message)
}

func (g *generator) extractBacklinks(html string) []string {
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

func cleanGeneratedHTML(content string) (string, error) {
	trimmed := strings.TrimSpace(content)
	trimmed = stripCodeFence(trimmed)
	if trimmed == "" {
		return "", eris.New("html content is empty")
	}

	doc, err := html.Parse(strings.NewReader(trimmed))
	if err != nil {
		return "", eris.Wrap(err, "parsing html content")
	}

	root := &html.Node{Type: html.ElementNode, Data: "div"}
	appendSanitizedChildren(root, doc)

	if root.FirstChild == nil {
		return "", eris.New("html content empty after cleaning")
	}

	contentRoot := root
	if div := singleDivChild(root); div != nil {
		div.Parent = nil
		contentRoot = div
	}

	var builder strings.Builder
	if err := html.Render(&builder, contentRoot); err != nil {
		return "", eris.Wrap(err, "rendering cleaned html")
	}

	return builder.String(), nil
}

func stripCodeFence(content string) string {
	if !strings.HasPrefix(content, "```") {
		return content
	}

	body := content[3:]
	newline := strings.IndexByte(body, '\n')
	if newline == -1 {
		return content
	}
	body = body[newline+1:]

	trimmedBody := strings.TrimRight(body, " \t\r\n")
	if !strings.HasSuffix(trimmedBody, "```") {
		return content
	}

	trimmedBody = strings.TrimRight(trimmedBody[:len(trimmedBody)-3], " \t\r\n")
	return strings.TrimSpace(trimmedBody)
}

func appendSanitizedChildren(dst, src *html.Node) {
	if src == nil {
		return
	}

	skipWhitespace := src.Type == html.DocumentNode || (src.Type == html.ElementNode && (strings.EqualFold(src.Data, "html") || strings.EqualFold(src.Data, "body")))

	for child := src.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case html.TextNode:
			if skipWhitespace && isWhitespaceTextNode(child) {
				continue
			}
			dst.AppendChild(&html.Node{Type: html.TextNode, Data: child.Data})
		case html.ElementNode:
			name := strings.ToLower(child.Data)
			switch name {
			case "head":
				continue
			case "html", "body":
				appendSanitizedChildren(dst, child)
				continue
			}

			newName := child.Data
			if name == "header" {
				newName = "div"
			}

			replacement := &html.Node{Type: html.ElementNode, Data: newName, Attr: cloneAttributes(child.Attr)}
			appendSanitizedChildren(replacement, child)
			dst.AppendChild(replacement)
		case html.CommentNode, html.DoctypeNode:
			continue
		default:
			appendSanitizedChildren(dst, child)
		}
	}
}

func cloneAttributes(attrs []html.Attribute) []html.Attribute {
	if len(attrs) == 0 {
		return nil
	}

	cloned := make([]html.Attribute, len(attrs))
	copy(cloned, attrs)
	return cloned
}

func singleDivChild(node *html.Node) *html.Node {
	if node == nil {
		return nil
	}

	first := node.FirstChild
	if first == nil || first.NextSibling != nil {
		return nil
	}

	if first.Type != html.ElementNode || !strings.EqualFold(first.Data, "div") {
		return nil
	}

	return first
}

func isWhitespaceTextNode(node *html.Node) bool {
	if node == nil || node.Type != html.TextNode {
		return false
	}

	return strings.TrimSpace(node.Data) == ""
}
