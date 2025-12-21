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
	"time"
)

const (
	baseURL        = "https://api.elevenlabs.io/v1/text-to-speech"
	defaultTimeout = 60 * time.Second
)

type WordTiming struct {
	Word      string
	StartTime float64
	EndTime   float64
}

type SpeechResult struct {
	Audio   []byte
	Timings []WordTiming
}

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

type timestampResponse struct {
	AudioBase64 string    `json:"audio_base64"`
	Alignment   alignment `json:"alignment"`
}

type alignment struct {
	Characters          []string  `json:"characters"`
	CharacterStartTimes []float64 `json:"character_start_times_seconds"`
	CharacterEndTimes   []float64 `json:"character_end_times_seconds"`
}

type errorResponse struct {
	Detail struct {
		Message string `json:"message"`
	} `json:"detail"`
}

type VoiceConfig struct {
	ID         string
	Stability  float64
	Similarity float64
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
	result, err := c.GenerateSpeechWithTimings(ctx, text)
	if err != nil {
		return nil, err
	}
	return result.Audio, nil
}

func (c *Client) GenerateSpeechWithTimings(ctx context.Context, text string) (*SpeechResult, error) {
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

	url := fmt.Sprintf("%s/%s/with-timestamps", c.baseURL, c.voiceID)

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

	var tsResp timestampResponse
	if err := json.Unmarshal(body, &tsResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	audio, err := base64.StdEncoding.DecodeString(tsResp.AudioBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode audio: %w", err)
	}

	if len(audio) == 0 {
		return nil, fmt.Errorf("empty audio response from elevenlabs api")
	}

	timings := extractWordTimings(tsResp.Alignment)

	return &SpeechResult{
		Audio:   audio,
		Timings: timings,
	}, nil
}

func extractWordTimings(a alignment) []WordTiming {
	if len(a.Characters) == 0 {
		return nil
	}

	var timings []WordTiming
	var currentWord strings.Builder
	var wordStart float64
	var wordEnd float64
	inWord := false

	for i, char := range a.Characters {
		if char == " " || char == "\n" || char == "\t" {
			if inWord && currentWord.Len() > 0 {
				timings = append(timings, WordTiming{
					Word:      currentWord.String(),
					StartTime: wordStart,
					EndTime:   wordEnd,
				})
				currentWord.Reset()
				inWord = false
			}
			continue
		}

		if !inWord {
			wordStart = a.CharacterStartTimes[i]
			inWord = true
		}
		currentWord.WriteString(char)
		wordEnd = a.CharacterEndTimes[i]
	}

	if currentWord.Len() > 0 {
		timings = append(timings, WordTiming{
			Word:      currentWord.String(),
			StartTime: wordStart,
			EndTime:   wordEnd,
		})
	}

	return timings
}

func (c *Client) GenerateSpeechWithVoice(ctx context.Context, text string, voice VoiceConfig) (*SpeechResult, error) {
	stability := voice.Stability
	similarity := voice.Similarity
	if stability == 0 {
		stability = c.stability
	}
	if similarity == 0 {
		similarity = c.similarity
	}

	reqBody := request{
		Text:    text,
		ModelID: c.model,
		VoiceSettings: voiceSettings{
			Stability:       stability,
			SimilarityBoost: similarity,
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/%s/with-timestamps", c.baseURL, voice.ID)

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

	var tsResp timestampResponse
	if err := json.Unmarshal(body, &tsResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	audio, err := base64.StdEncoding.DecodeString(tsResp.AudioBase64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode audio: %w", err)
	}

	if len(audio) == 0 {
		return nil, fmt.Errorf("empty audio response from elevenlabs api")
	}

	timings := extractWordTimings(tsResp.Alignment)

	return &SpeechResult{
		Audio:   audio,
		Timings: timings,
	}, nil
}

func (c *Client) VoiceID() string {
	return c.voiceID
}

func (c *Client) Model() string {
	return c.model
}
