package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"google.golang.org/genai"

	"craftstory/internal/llm"
	"craftstory/pkg/prompts"
)

const dailyLimit = 1500

type Client struct {
	client    *genai.Client
	model     string
	prompts   *prompts.Prompts
	usageFile string
}

var visualCueSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"keyword":      {Type: genai.TypeString, Description: "Exact word from script to anchor image"},
		"search_query": {Type: genai.TypeString, Description: "2-4 word image search term"},
	},
	Required: []string{"keyword", "search_query"},
}

var scriptWithVisualsSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"script":  {Type: genai.TypeString, Description: "Full script text"},
		"visuals": {Type: genai.TypeArray, Items: visualCueSchema, Description: "Visual cues for images"},
	},
	Required: []string{"script", "visuals"},
}

var visualsArraySchema = &genai.Schema{
	Type:  genai.TypeArray,
	Items: visualCueSchema,
}

func NewClient(ctx context.Context, project, location, model string, p *prompts.Prompts) (*Client, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Project:  project,
		Location: location,
		Backend:  genai.BackendVertexAI,
	})
	if err != nil {
		return nil, fmt.Errorf("create gemini client: %w", err)
	}

	home, _ := os.UserHomeDir()
	usageFile := filepath.Join(home, ".craftstory_usage")

	return &Client{
		client:    client,
		model:     model,
		prompts:   p,
		usageFile: usageFile,
	}, nil
}

func (c *Client) checkUsage() error {
	date, count := c.readUsage()
	today := time.Now().Format("2006-01-02")

	if date != today {
		return nil
	}
	if count >= dailyLimit {
		return fmt.Errorf("daily limit of %d requests reached, resets tomorrow", dailyLimit)
	}
	return nil
}

func (c *Client) incrementUsage() {
	date, count := c.readUsage()
	today := time.Now().Format("2006-01-02")

	if date != today {
		count = 0
	}
	count++

	os.WriteFile(c.usageFile, []byte(fmt.Sprintf("%s:%d", today, count)), 0644)
}

func (c *Client) readUsage() (string, int) {
	data, err := os.ReadFile(c.usageFile)
	if err != nil {
		return "", 0
	}
	parts := strings.Split(string(data), ":")
	if len(parts) != 2 {
		return "", 0
	}
	count, _ := strconv.Atoi(parts[1])
	return parts[0], count
}

func (c *Client) GenerateScript(ctx context.Context, topic string, scriptLength, hookDuration int) (string, error) {
	prompt, err := c.prompts.RenderScript(prompts.ScriptParams{
		Topic:        topic,
		ScriptLength: scriptLength,
		HookDuration: hookDuration,
	})
	if err != nil {
		return "", fmt.Errorf("render prompt: %w", err)
	}
	return c.generate(ctx, c.prompts.System.Default, prompt)
}

func (c *Client) GenerateConversation(ctx context.Context, topic string, speakers []string, scriptLength, hookDuration int) (string, error) {
	prompt, err := c.prompts.RenderConversation(prompts.ConversationParams{
		Topic:        topic,
		ScriptLength: scriptLength,
		HookDuration: hookDuration,
		SpeakerList:  strings.Join(speakers, ", "),
		FirstSpeaker: speakers[0],
		LastSpeaker:  speakers[len(speakers)-1],
	})
	if err != nil {
		return "", fmt.Errorf("render prompt: %w", err)
	}
	return c.generate(ctx, c.prompts.System.Conversation, prompt)
}

func (c *Client) GenerateScriptWithVisuals(ctx context.Context, topic string, scriptLength, hookDuration int) (*llm.ScriptWithVisuals, error) {
	prompt, err := c.prompts.RenderVisuals(prompts.VisualsParams{
		Topic:        topic,
		ScriptLength: scriptLength,
		HookDuration: hookDuration,
	})
	if err != nil {
		return nil, fmt.Errorf("render prompt: %w", err)
	}

	content, err := c.generateJSON(ctx, c.prompts.System.Visuals, prompt, scriptWithVisualsSchema)
	if err != nil {
		return nil, err
	}

	var result llm.ScriptWithVisuals
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &result, nil
}

func (c *Client) GenerateVisualsForScript(ctx context.Context, script string) ([]llm.VisualCue, error) {
	prompt, err := c.prompts.RenderVisualsOnly(prompts.VisualsOnlyParams{Script: script})
	if err != nil {
		return nil, fmt.Errorf("render prompt: %w", err)
	}

	content, err := c.generateJSON(ctx, c.prompts.System.Visuals, prompt, visualsArraySchema)
	if err != nil {
		return nil, err
	}

	var visuals []llm.VisualCue
	if err := json.Unmarshal([]byte(content), &visuals); err != nil {
		return nil, fmt.Errorf("parse visuals: %w", err)
	}
	return visuals, nil
}

func (c *Client) GenerateTitle(ctx context.Context, script string) (string, error) {
	prompt, err := c.prompts.RenderTitle(prompts.TitleParams{Script: script})
	if err != nil {
		return "", fmt.Errorf("render prompt: %w", err)
	}
	return c.generate(ctx, c.prompts.System.Title, prompt)
}

func (c *Client) generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	config := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{{Text: systemPrompt}},
		},
	}
	return c.call(ctx, userPrompt, config)
}

func (c *Client) generateJSON(ctx context.Context, systemPrompt, userPrompt string, schema *genai.Schema) (string, error) {
	config := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{{Text: systemPrompt}},
		},
		ResponseMIMEType: "application/json",
		ResponseSchema:   schema,
	}
	return c.call(ctx, userPrompt, config)
}

func (c *Client) call(ctx context.Context, userPrompt string, config *genai.GenerateContentConfig) (string, error) {
	if err := c.checkUsage(); err != nil {
		return "", err
	}

	resp, err := c.client.Models.GenerateContent(ctx, c.model, genai.Text(userPrompt), config)
	if err != nil {
		return "", fmt.Errorf("generate: %w", err)
	}

	c.incrementUsage()

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no response")
	}

	if resp.Candidates[0].Content.Parts[0].Text == "" {
		return "", fmt.Errorf("empty response")
	}

	return resp.Candidates[0].Content.Parts[0].Text, nil
}
