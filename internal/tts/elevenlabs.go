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
	elevenLabsBaseURL      = "https://api.elevenlabs.io/v1"
	elevenLabsTimeout      = 120 * time.Second
	defaultModel           = "eleven_multilingual_v2"
	defaultVoiceStability  = 0.5
	defaultVoiceSimilarity = 0.75
)

type ElevenLabsClient struct {
	apiKey     string
	httpClient *http.Client
	voiceID    string
	baseURL    string
}

type ElevenLabsOptions struct {
	VoiceID string
}

type timestampResponse struct {
	AudioBase64 string     `json:"audio_base64"`
	Alignment   *alignment `json:"alignment"`
}

type alignment struct {
	Characters          []string  `json:"characters"`
	CharacterStartTimes []float64 `json:"character_start_times_seconds"`
	CharacterEndTimes   []float64 `json:"character_end_times_seconds"`
}

func NewElevenLabsClient(apiKey string, opts ElevenLabsOptions) *ElevenLabsClient {
	voiceID := opts.VoiceID
	if voiceID == "" {
		voiceID = "pNInz6obpgDQGcFmaJgB" // Adam
	}
	return &ElevenLabsClient{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: elevenLabsTimeout},
		voiceID:    voiceID,
	}
}

func (c *ElevenLabsClient) GenerateSpeech(ctx context.Context, text string) ([]byte, error) {
	result, err := c.generateWithTimestamps(ctx, text, c.voiceID)
	if err != nil {
		return nil, err
	}
	return result.Audio, nil
}

func (c *ElevenLabsClient) GenerateSpeechWithTimings(ctx context.Context, text string) (*SpeechResult, error) {
	return c.generateWithTimestamps(ctx, text, c.voiceID)
}

func (c *ElevenLabsClient) GenerateSpeechWithVoice(ctx context.Context, text string, voice VoiceConfig) (*SpeechResult, error) {
	voiceID := voice.ID
	if voiceID == "" {
		voiceID = c.voiceID
	}
	return c.generateWithTimestamps(ctx, text, voiceID)
}

func (c *ElevenLabsClient) generateWithTimestamps(ctx context.Context, text, voiceID string) (*SpeechResult, error) {
	base := c.baseURL
	if base == "" {
		base = elevenLabsBaseURL
	}
	url := fmt.Sprintf("%s/text-to-speech/%s/with-timestamps", base, voiceID)

	payload := map[string]any{
		"text":     text,
		"model_id": defaultModel,
		"voice_settings": map[string]any{
			"stability":        defaultVoiceStability,
			"similarity_boost": defaultVoiceSimilarity,
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
	req.Header.Set("xi-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("elevenlabs: %s - %s", resp.Status, string(body))
	}

	var tsResp timestampResponse
	if err := json.Unmarshal(body, &tsResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	audio, err := base64.StdEncoding.DecodeString(tsResp.AudioBase64)
	if err != nil {
		return nil, fmt.Errorf("decode audio: %w", err)
	}

	timings := c.parseTimings(text, tsResp.Alignment)

	return &SpeechResult{
		Audio:   audio,
		Timings: timings,
	}, nil
}

func (c *ElevenLabsClient) parseTimings(text string, align *alignment) []WordTiming {
	if align == nil || len(align.Characters) == 0 {
		return estimateTimings(text, nil)
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	timings := make([]WordTiming, 0, len(words))
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
			timings = append(timings, WordTiming{
				Word:      word,
				StartTime: align.CharacterStartTimes[startIdx],
				EndTime:   align.CharacterEndTimes[endIdx-1],
			})
		}

		charIdx = endIdx
	}

	if len(timings) == 0 {
		return estimateTimings(text, nil)
	}

	return timings
}
