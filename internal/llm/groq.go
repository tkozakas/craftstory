package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/conneroisu/groq-go"

	"craftstory/pkg/prompts"
)

type GroqClient struct {
	client  *groq.Client
	model   groq.ChatModel
	prompts *prompts.Prompts
}

func NewGroqClient(apiKey, model string, p *prompts.Prompts) (*GroqClient, error) {
	client, err := groq.NewClient(apiKey)
	if err != nil {
		return nil, fmt.Errorf("create groq client: %w", err)
	}

	return &GroqClient{
		client:  client,
		model:   groq.ChatModel(model),
		prompts: p,
	}, nil
}

func (c *GroqClient) GenerateScript(ctx context.Context, topic string, wordCount int) (string, error) {
	prompt, err := c.prompts.RenderScript(prompts.ScriptParams{
		Topic:     topic,
		WordCount: wordCount,
	})
	if err != nil {
		return "", fmt.Errorf("render prompt: %w", err)
	}
	return c.generate(ctx, c.prompts.System.Default, prompt)
}

func (c *GroqClient) GenerateConversation(ctx context.Context, topic string, speakers []string, wordCount int) (string, error) {
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

func (c *GroqClient) GenerateVisuals(ctx context.Context, script string) ([]VisualCue, error) {
	prompt, err := c.prompts.RenderVisuals(prompts.VisualsParams{Script: script})
	if err != nil {
		return nil, fmt.Errorf("render prompt: %w", err)
	}

	content, err := c.generateJSONContent(ctx, c.prompts.System.Visuals, prompt)
	if err != nil {
		return nil, err
	}

	var visuals []VisualCue
	if err := json.Unmarshal([]byte(content), &visuals); err == nil {
		return visuals, nil
	}

	var wrapped struct {
		Visuals []VisualCue `json:"visuals"`
	}
	if err := json.Unmarshal([]byte(content), &wrapped); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return wrapped.Visuals, nil
}

func (c *GroqClient) GenerateTitle(ctx context.Context, script string) (string, error) {
	prompt, err := c.prompts.RenderTitle(prompts.TitleParams{Script: script})
	if err != nil {
		return "", fmt.Errorf("render prompt: %w", err)
	}
	return c.generate(ctx, c.prompts.System.Title, prompt)
}

func (c *GroqClient) generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
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

func (c *GroqClient) generateJSONContent(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
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
