package xtts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"craftstory/internal/tts"
)

const (
	defaultServerURL = "http://localhost:8020"
	defaultTimeout   = 120 * time.Second
	bytesPerSecond   = 48000
)

type Client struct {
	serverURL  string
	httpClient *http.Client
	speaker    string
	language   string
}

type Options struct {
	ServerURL string
	Speaker   string
	Language  string
}

func NewClient(opts Options) *Client {
	serverURL := opts.ServerURL
	if serverURL == "" {
		serverURL = defaultServerURL
	}

	language := opts.Language
	if language == "" {
		language = "en"
	}

	return &Client{
		serverURL:  serverURL,
		httpClient: &http.Client{Timeout: defaultTimeout},
		speaker:    opts.Speaker,
		language:   language,
	}
}

func (c *Client) GenerateSpeech(ctx context.Context, text string) ([]byte, error) {
	result, err := c.GenerateSpeechWithTimings(ctx, text)
	if err != nil {
		return nil, err
	}
	return result.Audio, nil
}

func (c *Client) GenerateSpeechWithTimings(ctx context.Context, text string) (*tts.SpeechResult, error) {
	audio, err := c.synthesize(ctx, text, c.speaker)
	if err != nil {
		return nil, err
	}

	return &tts.SpeechResult{
		Audio:   audio,
		Timings: estimateWordTimings(text, audio),
	}, nil
}

func (c *Client) GenerateSpeechWithVoice(ctx context.Context, text string, voice tts.VoiceConfig) (*tts.SpeechResult, error) {
	speaker := voice.Name
	if speaker == "" {
		speaker = c.speaker
	}

	audio, err := c.synthesize(ctx, text, speaker)
	if err != nil {
		return nil, err
	}

	return &tts.SpeechResult{
		Audio:   audio,
		Timings: estimateWordTimings(text, audio),
	}, nil
}

func (c *Client) synthesize(ctx context.Context, text, speaker string) ([]byte, error) {
	speakerPath := fmt.Sprintf("/app/speakers/%s.wav", speaker)
	reqBody := map[string]string{
		"text":        text,
		"speaker_wav": speakerPath,
		"language":    c.language,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.serverURL+"/tts_to_audio/", bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server error %s: %s", resp.Status, string(body))
	}

	return io.ReadAll(resp.Body)
}

func (c *Client) IsServerRunning() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.serverURL+"/docs", nil)
	if err != nil {
		return false
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (c *Client) WaitForServer(timeout, interval time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if c.IsServerRunning() {
			return nil
		}
		time.Sleep(interval)
	}

	return fmt.Errorf("xtts server not available after %v", timeout)
}

func (c *Client) Speaker() string     { return c.speaker }
func (c *Client) SetSpeaker(s string) { c.speaker = s }

func estimateWordTimings(text string, audio []byte) []tts.WordTiming {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	duration := float64(len(audio)) / bytesPerSecond
	perWord := duration / float64(len(words))

	timings := make([]tts.WordTiming, len(words))
	for i, word := range words {
		timings[i] = tts.WordTiming{
			Word:      word,
			StartTime: float64(i) * perWord,
			EndTime:   float64(i+1) * perWord,
		}
	}

	return timings
}
