package elevenlabs

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"craftstory/internal/speech"
)

const (
	baseURL = "https://api.elevenlabs.io/v1"
	timeout = 120 * time.Second
	model   = "eleven_multilingual_v2"
)

type Client struct {
	apiKeys    []string
	keyIndex   uint64
	httpClient *http.Client
	voiceID    string
	baseURL    string
	speed      float64
	stability  float64
	similarity float64
}

type Config struct {
	APIKeys    []string
	VoiceID    string
	Speed      float64
	Stability  float64
	Similarity float64
}

type option func(*Client)

type timestampResponse struct {
	AudioBase64 string     `json:"audio_base64"`
	Alignment   *alignment `json:"alignment"`
}

type alignment struct {
	Characters          []string  `json:"characters"`
	CharacterStartTimes []float64 `json:"character_start_times_seconds"`
	CharacterEndTimes   []float64 `json:"character_end_times_seconds"`
}

func withBaseURL(url string) option {
	return func(c *Client) {
		c.baseURL = url
	}
}

func withHTTPClient(client *http.Client) option {
	return func(c *Client) {
		c.httpClient = client
	}
}

func NewClient(cfg Config) speech.Provider {
	keys := cfg.APIKeys
	if len(keys) == 0 {
		keys = []string{""}
	}

	return &Client{
		apiKeys:    keys,
		httpClient: &http.Client{Timeout: timeout},
		voiceID:    cfg.VoiceID,
		speed:      cfg.Speed,
		stability:  cfg.Stability,
		similarity: cfg.Similarity,
	}
}

func newClient(cfg Config, opts ...option) *Client {
	keys := cfg.APIKeys
	if len(keys) == 0 {
		keys = []string{""}
	}

	c := &Client{
		apiKeys:    keys,
		httpClient: &http.Client{Timeout: timeout},
		voiceID:    cfg.VoiceID,
		speed:      cfg.Speed,
		stability:  cfg.Stability,
		similarity: cfg.Similarity,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *Client) GenerateSpeech(ctx context.Context, text string) ([]byte, error) {
	result, err := c.generateWithTimestamps(ctx, text, c.voiceID)
	if err != nil {
		return nil, err
	}
	return result.Audio, nil
}

func (c *Client) GenerateSpeechWithTimings(ctx context.Context, text string) (*speech.SpeechResult, error) {
	return c.generateWithTimestamps(ctx, text, c.voiceID)
}

func (c *Client) GenerateSpeechWithVoice(ctx context.Context, text string, voice speech.VoiceConfig) (*speech.SpeechResult, error) {
	voiceID := voice.ID
	if voiceID == "" {
		voiceID = c.voiceID
	}
	return c.generateWithTimestamps(ctx, text, voiceID)
}

func (c *Client) nextAPIKey() string {
	if len(c.apiKeys) == 1 {
		return c.apiKeys[0]
	}
	idx := atomic.AddUint64(&c.keyIndex, 1)
	return c.apiKeys[idx%uint64(len(c.apiKeys))]
}

func (c *Client) getKeyAtOffset(offset int) string {
	idx := atomic.LoadUint64(&c.keyIndex)
	return c.apiKeys[(idx+uint64(offset))%uint64(len(c.apiKeys))]
}

func (c *Client) generateWithTimestamps(ctx context.Context, text, voiceID string) (*speech.SpeechResult, error) {
	url := c.buildURL(voiceID)

	startKey := c.nextAPIKey()
	result, err := c.doRequestWithKey(ctx, url, text, startKey)
	if err == nil {
		return result, nil
	}
	if !isQuotaError(err) {
		return nil, err
	}

	for i := 1; i < len(c.apiKeys); i++ {
		key := c.getKeyAtOffset(i)
		if key == startKey {
			continue
		}
		result, err = c.doRequestWithKey(ctx, url, text, key)
		if err == nil {
			return result, nil
		}
		if !isQuotaError(err) {
			return nil, err
		}
	}

	return nil, fmt.Errorf("all API keys exhausted: %w", err)
}

func (c *Client) doRequestWithKey(ctx context.Context, url, text, apiKey string) (*speech.SpeechResult, error) {
	req, err := c.buildRequestWithKey(ctx, url, text, apiKey)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("elevenlabs: %s - %s", resp.Status, string(body))
	}

	return c.parseResponse(text, body)
}

func isQuotaError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "quota_exceeded") ||
		strings.Contains(msg, "rate_limit") ||
		strings.Contains(msg, "429")
}

func (c *Client) buildURL(voiceID string) string {
	base := c.baseURL
	if base == "" {
		base = baseURL
	}
	return fmt.Sprintf("%s/text-to-speech/%s/with-timestamps", base, voiceID)
}

func (c *Client) buildRequestWithKey(ctx context.Context, url, text, apiKey string) (*http.Request, error) {
	payload := map[string]any{
		"text":     text,
		"model_id": model,
		"voice_settings": map[string]any{
			"stability":        c.stability,
			"similarity_boost": c.similarity,
			"speed":            c.speed,
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", apiKey)

	return req, nil
}

func (c *Client) parseResponse(text string, body []byte) (*speech.SpeechResult, error) {
	var tsResp timestampResponse
	if err := json.Unmarshal(body, &tsResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	audio, err := base64.StdEncoding.DecodeString(tsResp.AudioBase64)
	if err != nil {
		return nil, fmt.Errorf("decode audio: %w", err)
	}

	return &speech.SpeechResult{
		Audio:   audio,
		Timings: parseTimings(text, tsResp.Alignment),
	}, nil
}

func parseTimings(text string, align *alignment) []speech.WordTiming {
	if align == nil || len(align.Characters) == 0 {
		return speech.EstimateTimings(text, nil)
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	timings := make([]speech.WordTiming, 0, len(words))
	charIdx := 0

	for _, word := range words {
		for charIdx < len(align.Characters) && align.Characters[charIdx] == " " {
			charIdx++
		}

		if charIdx >= len(align.Characters) {
			break
		}

		startIdx := charIdx
		wordLen := len(word)
		endIdx := startIdx
		matchedChars := 0
		for endIdx < len(align.Characters) && matchedChars < wordLen {
			if align.Characters[endIdx] != " " {
				matchedChars++
			}
			endIdx++
		}

		if startIdx < len(align.CharacterStartTimes) && endIdx > 0 && endIdx-1 < len(align.CharacterEndTimes) {
			timings = append(timings, speech.WordTiming{
				Word:      word,
				StartTime: align.CharacterStartTimes[startIdx],
				EndTime:   align.CharacterEndTimes[endIdx-1],
			})
		}

		charIdx = endIdx
	}

	if len(timings) == 0 {
		return speech.EstimateTimings(text, nil)
	}

	return timings
}
