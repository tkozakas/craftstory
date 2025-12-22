package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	fishAudioBaseURL = "https://api.fish.audio/v1/tts"
	fishAudioTimeout = 120 * time.Second
)

type fishAudioRequest struct {
	Text        string            `json:"text"`
	Format      string            `json:"format,omitempty"`
	MP3Bitrate  int               `json:"mp3_bitrate,omitempty"`
	ReferenceID string            `json:"reference_id,omitempty"`
	Normalize   bool              `json:"normalize,omitempty"`
	Latency     string            `json:"latency,omitempty"`
	Prosody     *fishAudioProsody `json:"prosody,omitempty"`
}

type fishAudioProsody struct {
	Speed  float64 `json:"speed"`
	Volume float64 `json:"volume,omitempty"`
}

type FishAudioClient struct {
	apiKey     string
	httpClient *http.Client
	voiceID    string
	baseURL    string
}

type FishAudioOptions struct {
	VoiceID string
}

func NewFishAudioClient(apiKey string, opts FishAudioOptions) *FishAudioClient {
	return &FishAudioClient{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: fishAudioTimeout},
		voiceID:    opts.VoiceID,
		baseURL:    fishAudioBaseURL,
	}
}

func (c *FishAudioClient) GenerateSpeech(ctx context.Context, text string) ([]byte, error) {
	return c.generateWithVoiceID(ctx, text, c.voiceID)
}

func (c *FishAudioClient) GenerateSpeechWithTimings(ctx context.Context, text string) (*SpeechResult, error) {
	audio, err := c.generateWithVoiceID(ctx, text, c.voiceID)
	if err != nil {
		return nil, err
	}
	return &SpeechResult{
		Audio:   audio,
		Timings: estimateTimings(text, audio),
	}, nil
}

func (c *FishAudioClient) GenerateSpeechWithVoice(ctx context.Context, text string, voice VoiceConfig) (*SpeechResult, error) {
	voiceID := voice.ID
	if voiceID == "" {
		voiceID = c.voiceID
	}
	audio, err := c.generateWithVoiceID(ctx, text, voiceID)
	if err != nil {
		return nil, err
	}
	return &SpeechResult{
		Audio:   audio,
		Timings: estimateTimings(text, audio),
	}, nil
}

func (c *FishAudioClient) generateWithVoiceID(ctx context.Context, text, voiceID string) ([]byte, error) {
	req := fishAudioRequest{
		Text:        addPauses(text),
		Format:      "mp3",
		MP3Bitrate:  128,
		ReferenceID: voiceID,
		Normalize:   true,
		Latency:     "normal",
		Prosody:     &fishAudioProsody{Speed: 0.9},
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fish audio: %s - %s", resp.Status, string(body))
	}

	return body, nil
}

func estimateTimings(text string, audio []byte) []WordTiming {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	duration := estimateAudioDuration(audio)
	avgWordDuration := duration / float64(len(words))

	timings := make([]WordTiming, len(words))
	currentTime := 0.0

	for i, word := range words {
		wordDuration := avgWordDuration * (0.8 + 0.4*float64(len(word))/5.0)
		timings[i] = WordTiming{
			Word:      word,
			StartTime: currentTime,
			EndTime:   currentTime + wordDuration,
		}
		currentTime += wordDuration
	}

	if len(timings) > 0 && currentTime > 0 {
		scale := duration / currentTime
		for i := range timings {
			timings[i].StartTime *= scale
			timings[i].EndTime *= scale
		}
	}

	return timings
}

func estimateAudioDuration(audio []byte) float64 {
	bitrate := 128000.0
	return float64(len(audio)*8) / bitrate
}

func addPauses(text string) string {
	text = strings.ReplaceAll(text, "...", "…")
	text = strings.ReplaceAll(text, ". ", "... ")
	text = strings.ReplaceAll(text, "! ", "!.. ")
	text = strings.ReplaceAll(text, "? ", "?.. ")
	text = strings.ReplaceAll(text, "…", "...")
	return text
}

func (c *FishAudioClient) VoiceID() string {
	return c.voiceID
}

func (c *FishAudioClient) SetBaseURL(url string) {
	c.baseURL = url
}
