package deepseek

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"craftstory/pkg/prompts"
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
	prompts      *prompts.Prompts
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Options struct {
	Model        string
	SystemPrompt string
	Prompts      *prompts.Prompts
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

type VisualCue struct {
	SearchQuery string  `json:"search_query"`
	WordIndex   int     `json:"word_index"`
	Timestamp   float64 `json:"timestamp,omitempty"` // seconds into the video
	Duration    float64 `json:"duration,omitempty"`  // how long to display (seconds)
}

type ScriptWithVisuals struct {
	Script  string      `json:"script"`
	Visuals []VisualCue `json:"visuals"`
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
		prompts:      opts.Prompts,
	}
}

func (c *Client) GenerateScript(ctx context.Context, topic string, scriptLength, hookDuration int) (string, error) {
	var prompt string
	if c.prompts != nil {
		var err error
		prompt, err = c.prompts.RenderScript(prompts.ScriptParams{
			Topic:        topic,
			ScriptLength: scriptLength,
			HookDuration: hookDuration,
		})
		if err != nil {
			return "", fmt.Errorf("failed to render prompt: %w", err)
		}
	} else {
		prompt = fmt.Sprintf(
			"Generate a %d-second script about %s. "+
				"Use high-retention language, short sentences, and a hook in the first %d seconds. "+
				"Do not include stage directions or speaker labels. Just the spoken text.",
			scriptLength, topic, hookDuration,
		)
	}

	systemPrompt := c.systemPrompt
	if c.prompts != nil && c.prompts.System.Default != "" {
		systemPrompt = c.prompts.System.Default
	}

	return c.chat(ctx, systemPrompt, prompt)
}

func (c *Client) GenerateConversation(ctx context.Context, topic string, speakers []string, scriptLength, hookDuration int) (string, error) {
	speakerList := strings.Join(speakers, ", ")

	var prompt string
	if c.prompts != nil {
		var err error
		prompt, err = c.prompts.RenderConversation(prompts.ConversationParams{
			Topic:        topic,
			ScriptLength: scriptLength,
			HookDuration: hookDuration,
			SpeakerList:  speakerList,
			FirstSpeaker: speakers[0],
			LastSpeaker:  speakers[len(speakers)-1],
		})
		if err != nil {
			return "", fmt.Errorf("failed to render prompt: %w", err)
		}
	} else {
		prompt = fmt.Sprintf(
			`Generate a %d-second conversational script about %s.

Speakers: %s

IMPORTANT FORMAT RULES:
- Each line MUST start with the speaker name followed by a colon
- Example format:
  %s: First line of dialogue here.
  %s: Response dialogue here.

Requirements:
- Hook the viewer in the first %d seconds with something surprising or intriguing
- Use short, punchy sentences (max 15 words per line)
- Natural back-and-forth between speakers
- High energy and entertaining
- NO stage directions, actions, or descriptions in parentheses
- ONLY dialogue lines, nothing else`,
			scriptLength, topic, speakerList, speakers[0], speakers[len(speakers)-1], hookDuration,
		)
	}

	systemPrompt := "You are a scriptwriter for viral short-form video conversations. Write engaging dialogue. Output ONLY dialogue lines in 'Speaker: text' format, one per line."
	if c.prompts != nil && c.prompts.System.Conversation != "" {
		systemPrompt = c.prompts.System.Conversation
	}

	return c.chat(ctx, systemPrompt, prompt)
}

func (c *Client) GenerateScriptWithVisuals(ctx context.Context, topic string, scriptLength, hookDuration int) (*ScriptWithVisuals, error) {
	var prompt string
	if c.prompts != nil {
		var err error
		prompt, err = c.prompts.RenderVisuals(prompts.VisualsParams{
			Topic:        topic,
			ScriptLength: scriptLength,
			HookDuration: hookDuration,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to render prompt: %w", err)
		}
	} else {
		prompt = fmt.Sprintf(
			`Generate a %d-second script about %s with visual cues.

Requirements:
1. Write an engaging script with a hook in the first %d seconds
2. Use short punchy sentences
3. Suggest images to display at key moments throughout the script
4. Each visual should have a simple search query (2-4 words) for Google Image search

Return ONLY valid JSON in this exact format:
{
  "script": "Your full script text here without any speaker labels",
  "visuals": [
    {"search_query": "image search term", "word_index": 5},
    {"search_query": "another search term", "word_index": 15}
  ]
}

The word_index is which word number (0-based) the image should appear at.
Make search queries specific and visual (e.g., "golden gate bridge", "cute puppy", "space nebula").`,
			scriptLength, topic, hookDuration,
		)
	}

	systemPrompt := "You are a creative scriptwriter. Always respond with valid JSON only, no markdown or extra text."
	if c.prompts != nil && c.prompts.System.Visuals != "" {
		systemPrompt = c.prompts.System.Visuals
	}

	content, err := c.chat(ctx, systemPrompt, prompt)
	if err != nil {
		return nil, err
	}

	content = cleanJSONResponse(content)

	var result ScriptWithVisuals
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return &ScriptWithVisuals{Script: content, Visuals: nil}, nil
	}

	return &result, nil
}

func (c *Client) GenerateVisualsForScript(ctx context.Context, script string) ([]VisualCue, error) {
	var prompt string
	if c.prompts != nil && c.prompts.Script.VisualsOnly != "" {
		var err error
		prompt, err = c.prompts.RenderVisualsOnly(prompts.VisualsOnlyParams{
			Script: script,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to render prompt: %w", err)
		}
	} else {
		prompt = fmt.Sprintf(
			`Analyze this script and suggest images to display at key moments throughout.

Script: %s

Return ONLY valid JSON in this exact format:
{
  "script": "copy the original script here unchanged",
  "visuals": [
    {"search_query": "image search term", "word_index": 5},
    {"search_query": "another search term", "word_index": 15}
  ]
}

The word_index is which word number (0-based) the image should appear at.
Make search queries specific and visual (e.g., "golden gate bridge", "cute puppy", "space nebula").`,
			script,
		)
	}

	systemPrompt := "You are a creative scriptwriter. Always respond with valid JSON only, no markdown or extra text."
	if c.prompts != nil && c.prompts.System.Visuals != "" {
		systemPrompt = c.prompts.System.Visuals
	}

	content, err := c.chat(ctx, systemPrompt, prompt)
	if err != nil {
		return nil, err
	}

	content = cleanJSONResponse(content)

	var result ScriptWithVisuals
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("failed to parse visuals response: %w", err)
	}

	return result.Visuals, nil
}

func cleanJSONResponse(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

func (c *Client) GenerateTitle(ctx context.Context, script string) (string, error) {
	var prompt string
	if c.prompts != nil {
		var err error
		prompt, err = c.prompts.RenderTitle(prompts.TitleParams{Script: script})
		if err != nil {
			return "", fmt.Errorf("failed to render prompt: %w", err)
		}
	} else {
		prompt = fmt.Sprintf(
			"Generate a catchy, clickbait YouTube Shorts title for this script. "+
				"Max 60 characters. No quotes. No hashtags. Just the title.\n\nScript: %s",
			script,
		)
	}

	systemPrompt := "You are a YouTube title expert. Generate viral, engaging titles."
	if c.prompts != nil && c.prompts.System.Title != "" {
		systemPrompt = c.prompts.System.Title
	}

	return c.chat(ctx, systemPrompt, prompt)
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

func (c *Client) chat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	messages := []Message{
		{Role: roleSystem, Content: systemPrompt},
		{Role: roleUser, Content: userPrompt},
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
