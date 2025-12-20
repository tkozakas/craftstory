package elevenlabs

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
	baseURL        = "https://api.elevenlabs.io/v1/text-to-speech"
	defaultTimeout = 60 * time.Second
)

type Client struct {
	apiKey     string
	httpClient *http.Client
	voiceID    string
	model      string
	stability  float64
	similarity float64
	baseURL    string
}

type Options struct {
	VoiceID    string
	Model      string
	Stability  float64
	Similarity float64
}

type request struct {
	Text          string        `json:"text"`
	ModelID       string        `json:"model_id"`
	VoiceSettings voiceSettings `json:"voice_settings"`
}

type voiceSettings struct {
	Stability       float64 `json:"stability"`
	SimilarityBoost float64 `json:"similarity_boost"`
}

type errorResponse struct {
	Detail struct {
		Message string `json:"message"`
	} `json:"detail"`
}

func NewClient(apiKey string, opts Options) *Client {
	return &Client{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		voiceID:    opts.VoiceID,
		model:      opts.Model,
		stability:  opts.Stability,
		similarity: opts.Similarity,
		baseURL:    baseURL,
	}
}

func (c *Client) GenerateSpeech(ctx context.Context, text string) ([]byte, error) {
	reqBody := request{
		Text:    text,
		ModelID: c.model,
		VoiceSettings: voiceSettings{
			Stability:       c.stability,
			SimilarityBoost: c.similarity,
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/%s", c.baseURL, c.voiceID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", c.apiKey)

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
		var errResp errorResponse
		if jsonErr := json.Unmarshal(body, &errResp); jsonErr == nil && errResp.Detail.Message != "" {
			return nil, fmt.Errorf("elevenlabs error: %s", errResp.Detail.Message)
		}
		return nil, fmt.Errorf("elevenlabs error: %s", resp.Status)
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("empty response from elevenlabs api")
	}

	return body, nil
}

func (c *Client) VoiceID() string {
	return c.voiceID
}

func (c *Client) Model() string {
	return c.model
}
