package groq

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/conneroisu/groq-go"

	"craftstory/internal/llm"
	"craftstory/pkg/prompts"
)

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

func (c *Client) GenerateVisuals(ctx context.Context, script string) ([]llm.VisualCue, error) {
	prompt, err := c.prompts.RenderVisuals(prompts.VisualsParams{Script: script})
	if err != nil {
		return nil, fmt.Errorf("render prompt: %w", err)
	}

	content, err := c.generateJSONContent(ctx, c.prompts.System.Visuals, prompt)
	if err != nil {
		return nil, err
	}

	var visuals []llm.VisualCue
	if err := json.Unmarshal([]byte(content), &visuals); err == nil {
		return visuals, nil
	}

	var wrapped struct {
		Visuals []llm.VisualCue `json:"visuals"`
	}
	if err := json.Unmarshal([]byte(content), &wrapped); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return wrapped.Visuals, nil
}

func (c *Client) GenerateTitle(ctx context.Context, script string) (string, error) {
	prompt, err := c.prompts.RenderTitle(prompts.TitleParams{Script: script})
	if err != nil {
		return "", fmt.Errorf("render prompt: %w", err)
	}
	return c.generate(ctx, c.prompts.System.Title, prompt)
}

func (c *Client) generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	resp, err := c.client.ChatCompletion(ctx, groq.ChatCompletionRequest{
		Model: c.model,
		Messages: []groq.ChatCompletionMessage{
			{Role: groq.RoleSystem, Content: systemPrompt},
			{Role: groq.RoleUser, Content: userPrompt},
		},
	})
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

func (c *Client) generateJSON(ctx context.Context, systemPrompt, userPrompt string, output any) error {
	content, err := c.generateJSONContent(ctx, systemPrompt, userPrompt)
	if err != nil {
		return err
	}

	if err := json.Unmarshal([]byte(content), output); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	return nil
}

func (c *Client) generateJSONContent(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	resp, err := c.client.ChatCompletion(ctx, groq.ChatCompletionRequest{
		Model: c.model,
		Messages: []groq.ChatCompletionMessage{
			{Role: groq.RoleSystem, Content: systemPrompt},
			{Role: groq.RoleUser, Content: userPrompt},
		},
		ResponseFormat: &groq.ChatResponseFormat{
			Type: "json_object",
		},
	})
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
