package tts

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
	elevenLabsBaseURL = "https://api.elevenlabs.io/v1/text-to-speech"
	defaultTimeout    = 60 * time.Second
)

type elevenlabsRequest struct {
	Text          string                `json:"text"`
	ModelID       string                `json:"model_id"`
	VoiceSettings elevenlabsVoiceConfig `json:"voice_settings"`
}

type elevenlabsVoiceConfig struct {
	Stability       float64 `json:"stability"`
	SimilarityBoost float64 `json:"similarity_boost"`
	Speed           float64 `json:"speed,omitempty"`
}

type elevenlabsTimestampResponse struct {
	AudioBase64 string              `json:"audio_base64"`
	Alignment   elevenlabsAlignment `json:"alignment"`
}

type elevenlabsAlignment struct {
	Characters          []string  `json:"characters"`
	CharacterStartTimes []float64 `json:"character_start_times_seconds"`
	CharacterEndTimes   []float64 `json:"character_end_times_seconds"`
}

type elevenlabsErrorResponse struct {
	Detail struct {
		Message string `json:"message"`
	} `json:"detail"`
}

// ElevenLabsClient implements Provider using the ElevenLabs API.
type ElevenLabsClient struct {
	apiKey     string
	httpClient *http.Client
	voiceID    string
	model      string
	stability  float64
	similarity float64
	speed      float64
	baseURL    string
}

// ElevenLabsOptions configures the ElevenLabs client.
type ElevenLabsOptions struct {
	VoiceID    string
	Model      string
	Stability  float64
	Similarity float64
	Speed      float64
}

// NewElevenLabsClient creates a new ElevenLabs TTS client.
func NewElevenLabsClient(apiKey string, opts ElevenLabsOptions) *ElevenLabsClient {
	speed := opts.Speed
	if speed == 0 {
		speed = 1.0
	}
	return &ElevenLabsClient{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		voiceID:    opts.VoiceID,
		model:      opts.Model,
		stability:  opts.Stability,
		similarity: opts.Similarity,
		speed:      speed,
		baseURL:    elevenLabsBaseURL,
	}
}

// GenerateSpeech generates audio from text using the default voice.
func (c *ElevenLabsClient) GenerateSpeech(ctx context.Context, text string) ([]byte, error) {
	result, err := c.GenerateSpeechWithTimings(ctx, text)
	if err != nil {
		return nil, err
	}
	return result.Audio, nil
}

// GenerateSpeechWithTimings generates audio with word-level timing data.
func (c *ElevenLabsClient) GenerateSpeechWithTimings(ctx context.Context, text string) (*SpeechResult, error) {
	return c.generateWithVoiceID(ctx, text, c.voiceID, c.stability, c.similarity, c.speed)
}

// GenerateSpeechWithVoice generates audio using a specific voice configuration.
func (c *ElevenLabsClient) GenerateSpeechWithVoice(ctx context.Context, text string, voice VoiceConfig) (*SpeechResult, error) {
	stability := voice.Stability
	similarity := voice.Similarity
	if stability == 0 {
		stability = c.stability
	}
	if similarity == 0 {
		similarity = c.similarity
	}
	return c.generateWithVoiceID(ctx, text, voice.ID, stability, similarity, c.speed)
}

func (c *ElevenLabsClient) generateWithVoiceID(ctx context.Context, text, voiceID string, stability, similarity, speed float64) (*SpeechResult, error) {
	reqBody := elevenlabsRequest{
		Text:    text,
		ModelID: c.model,
		VoiceSettings: elevenlabsVoiceConfig{
			Stability:       stability,
			SimilarityBoost: similarity,
			Speed:           speed,
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/%s/with-timestamps", c.baseURL, voiceID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("xi-api-key", c.apiKey)

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
		var errResp elevenlabsErrorResponse
		if jsonErr := json.Unmarshal(body, &errResp); jsonErr == nil && errResp.Detail.Message != "" {
			return nil, fmt.Errorf("elevenlabs: %s", errResp.Detail.Message)
		}
		return nil, fmt.Errorf("elevenlabs: %s", resp.Status)
	}

	var tsResp elevenlabsTimestampResponse
	if err := json.Unmarshal(body, &tsResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	audio, err := base64.StdEncoding.DecodeString(tsResp.AudioBase64)
	if err != nil {
		return nil, fmt.Errorf("decode audio: %w", err)
	}

	if len(audio) == 0 {
		return nil, fmt.Errorf("elevenlabs: empty audio response")
	}

	return &SpeechResult{
		Audio:   audio,
		Timings: extractWordTimings(tsResp.Alignment),
	}, nil
}

func extractWordTimings(a elevenlabsAlignment) []WordTiming {
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

// VoiceID returns the default voice ID.
func (c *ElevenLabsClient) VoiceID() string {
	return c.voiceID
}

// Model returns the model ID.
func (c *ElevenLabsClient) Model() string {
	return c.model
}

// SetBaseURL sets the base URL for testing.
func (c *ElevenLabsClient) SetBaseURL(url string) {
	c.baseURL = url
}
