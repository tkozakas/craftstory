package groq

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/conneroisu/groq-go"

	"craftstory/internal/llm"
	"craftstory/pkg/prompts"
)

var _ llm.Client = (*Client)(nil)

type Client struct {
	client  *groq.Client
	model   groq.ChatModel
	prompts *prompts.Prompts
}

func NewClient(apiKey, model string, p *prompts.Prompts) (*Client, error) {
	client, err := groq.NewClient(apiKey)
	if err != nil {
		return nil, fmt.Errorf("create groq client: %w", err)
	}

	return &Client{
		client:  client,
		model:   groq.ChatModel(model),
		prompts: p,
	}, nil
}

func (c *Client) GenerateScript(ctx context.Context, topic string, wordCount int) (string, error) {
	prompt, err := c.prompts.RenderScript(prompts.ScriptParams{
		Topic:     topic,
		WordCount: wordCount,
	})
	if err != nil {
		return "", fmt.Errorf("render prompt: %w", err)
	}
	return c.generate(ctx, c.prompts.System.Default, prompt)
}

func (c *Client) GenerateConversation(ctx context.Context, topic string, speakers []string, wordCount int) (string, error) {
	prompt, err := c.prompts.RenderConversation(prompts.ConversationParams{
		Topic:        topic,
		WordCount:    wordCount,
		SpeakerList:  strings.Join(speakers, ", "),
		FirstSpeaker: speakers[0],
		LastSpeaker:  speakers[len(speakers)-1],
	})
	if err != nil {
		return "", fmt.Errorf("render prompt: %w", err)
	}
	return c.generate(ctx, c.prompts.System.Conversation, prompt)
}

func (c *Client) GenerateVisuals(ctx context.Context, script string, count int) ([]llm.VisualCue, error) {
	prompt, err := c.prompts.RenderVisuals(prompts.VisualsParams{Script: script, Count: count})
	if err != nil {
		return nil, fmt.Errorf("render prompt: %w", err)
	}

	content, err := c.generateJSONContent(ctx, c.prompts.System.Visuals, prompt)
	if err != nil {
		return nil, err
	}

	slog.Info("LLM visuals raw response", "content", content)

	visuals, err := parseJSONArray[llm.VisualCue](content, []string{"visuals", "visual_cues", "keywords", "images", "results"})
	if err != nil {
		return nil, err
	}

	return deduplicateVisuals(visuals), nil
}

func deduplicateVisuals(visuals []llm.VisualCue) []llm.VisualCue {
	seen := make(map[string]bool)
	result := make([]llm.VisualCue, 0, len(visuals))

	for _, v := range visuals {
		key := strings.ToLower(v.Keyword)
		if seen[key] {
			slog.Debug("Skipping duplicate keyword", "keyword", v.Keyword)
			continue
		}
		seen[key] = true
		result = append(result, v)
	}

	return result
}

func (c *Client) GenerateTitle(ctx context.Context, script string) (string, error) {
	prompt, err := c.prompts.RenderTitle(prompts.TitleParams{Script: script})
	if err != nil {
		return "", fmt.Errorf("render prompt: %w", err)
	}

	content, err := c.generate(ctx, c.prompts.System.Title, prompt)
	if err != nil {
		return "", err
	}

	return cleanTitle(content), nil
}

func cleanTitle(raw string) string {
	title := strings.TrimSpace(raw)
	title = strings.Trim(title, "\"'")

	if idx := strings.Index(title, "\n"); idx > 0 {
		title = title[:idx]
	}

	title = strings.TrimSpace(title)

	if len(title) > 100 {
		title = title[:100]
	}

	return title
}

func (c *Client) GenerateTags(ctx context.Context, script string, count int) ([]string, error) {
	prompt, err := c.prompts.RenderTags(prompts.TagsParams{Script: script, Count: count})
	if err != nil {
		return nil, fmt.Errorf("render prompt: %w", err)
	}

	content, err := c.generateJSONContent(ctx, c.prompts.System.Tags, prompt)
	if err != nil {
		return nil, err
	}

	tags, err := parseJSONArray[string](content, []string{"tags", "keywords", "results"})
	if err != nil {
		return nil, err
	}

	return cleanTags(tags), nil
}

func parseJSONArray[T any](content string, keys []string) ([]T, error) {
	var direct []T
	if err := json.Unmarshal([]byte(content), &direct); err == nil && len(direct) > 0 {
		return direct, nil
	}

	var wrapped map[string][]T
	if err := json.Unmarshal([]byte(content), &wrapped); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	for _, key := range keys {
		if items, ok := wrapped[key]; ok && len(items) > 0 {
			return items, nil
		}
	}

	for _, items := range wrapped {
		if len(items) > 0 {
			return items, nil
		}
	}

	return nil, fmt.Errorf("no items found in response")
}

func cleanTags(tags []string) []string {
	result := make([]string, 0, len(tags))
	seen := make(map[string]bool)

	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		tag = strings.Trim(tag, "#")
		tag = strings.ToLower(tag)

		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		result = append(result, tag)
	}

	return result
}

func (c *Client) generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	return c.doGenerate(ctx, systemPrompt, userPrompt, false)
}

func (c *Client) generateJSONContent(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	return c.doGenerate(ctx, systemPrompt, userPrompt, true)
}

func (c *Client) doGenerate(ctx context.Context, systemPrompt, userPrompt string, jsonMode bool) (string, error) {
	req := groq.ChatCompletionRequest{
		Model: c.model,
		Messages: []groq.ChatCompletionMessage{
			{Role: groq.RoleSystem, Content: systemPrompt},
			{Role: groq.RoleUser, Content: userPrompt},
		},
	}

	if jsonMode {
		req.ResponseFormat = &groq.ChatResponseFormat{Type: "json_object"}
	}

	resp, err := c.client.ChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("generate: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response")
	}

	content := resp.Choices[0].Message.Content
	if content == "" {
		return "", fmt.Errorf("empty response")
	}

	return content, nil
}
