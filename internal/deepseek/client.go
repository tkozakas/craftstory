package deepseek

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	baseURL        = "https://api.deepseek.com/v1/chat/completions"
	defaultTimeout = 30 * time.Second
	roleSystem     = "system"
	roleUser       = "user"
)

type Client struct {
	apiKey       string
	httpClient   *http.Client
	model        string
	systemPrompt string
	baseURL      string
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Options struct {
	Model        string
	SystemPrompt string
}

type request struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type response struct {
	ID      string    `json:"id"`
	Choices []choice  `json:"choices"`
	Error   *apiError `json:"error,omitempty"`
}

type choice struct {
	Message Message `json:"message"`
}

type apiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

func NewClient(apiKey string, opts Options) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		model:        opts.Model,
		systemPrompt: opts.SystemPrompt,
		baseURL:      baseURL,
	}
}

func (c *Client) GenerateScript(ctx context.Context, topic string, scriptLength, hookDuration int) (string, error) {
	prompt := fmt.Sprintf(
		"Generate a %d-second script about %s. "+
			"Use high-retention language, short sentences, and a hook in the first %d seconds. "+
			"Do not include stage directions or speaker labels. Just the spoken text.",
		scriptLength, topic, hookDuration,
	)

	messages := []Message{
		{Role: roleSystem, Content: c.systemPrompt},
		{Role: roleUser, Content: prompt},
	}

	reqBody := request{
		Model:    c.model,
		Messages: messages,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.doRequest(ctx, data)
	if err != nil {
		return "", err
	}

	return c.parseResponse(resp)
}

func (c *Client) GenerateTitle(ctx context.Context, script string) (string, error) {
	prompt := fmt.Sprintf(
		"Generate a catchy, clickbait YouTube Shorts title for this script. "+
			"Max 60 characters. No quotes. No hashtags. Just the title.\n\nScript: %s",
		script,
	)

	messages := []Message{
		{Role: roleSystem, Content: "You are a YouTube title expert. Generate viral, engaging titles."},
		{Role: roleUser, Content: prompt},
	}

	reqBody := request{
		Model:    c.model,
		Messages: messages,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.doRequest(ctx, data)
	if err != nil {
		return "", err
	}

	return c.parseResponse(resp)
}

func (c *Client) Chat(ctx context.Context, prompt string, history []Message) (string, error) {
	messages := c.buildMessages(prompt, history)

	reqBody := request{
		Model:    c.model,
		Messages: messages,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.doRequest(ctx, data)
	if err != nil {
		return "", err
	}

	return c.parseResponse(resp)
}

func (c *Client) Model() string {
	return c.model
}

func (c *Client) buildMessages(prompt string, history []Message) []Message {
	messages := make([]Message, 0, len(history)+2)
	messages = append(messages, Message{Role: roleSystem, Content: c.systemPrompt})
	messages = append(messages, history...)
	messages = append(messages, Message{Role: roleUser, Content: prompt})
	return messages
}

func (c *Client) doRequest(ctx context.Context, data []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api error: %s", string(body))
	}

	return body, nil
}

func (c *Client) parseResponse(data []byte) (string, error) {
	var resp response
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if resp.Error != nil {
		return "", fmt.Errorf("deepseek error: %s", resp.Error.Message)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response choices")
	}

	return resp.Choices[0].Message.Content, nil
}
