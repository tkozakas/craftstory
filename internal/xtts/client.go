package xtts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultServerURL = "http://localhost:8020"
	defaultTimeout   = 120 * time.Second
	bytesPerSecond   = 48000
	maxStartupWait   = 30
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
	serverURL   string
	httpClient  *http.Client
	voiceSample string
	language    string
}

type Options struct {
	ServerURL   string
	VoiceSample string
	Language    string
}

type CharacterVoice struct {
	Name        string
	VoiceSample string
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
		serverURL: serverURL,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		voiceSample: opts.VoiceSample,
		language:    language,
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
	audio, err := c.generateViaServer(ctx, text)
	if err != nil {
		audio, err = c.generateViaCLI(ctx, text)
		if err != nil {
			return nil, err
		}
	}

	timings := estimateWordTimings(text, audio)

	return &SpeechResult{
		Audio:   audio,
		Timings: timings,
	}, nil
}

func (c *Client) GenerateSpeechWithVoice(ctx context.Context, text string, voiceSample string) (*SpeechResult, error) {
	origSample := c.voiceSample
	c.voiceSample = voiceSample
	defer func() { c.voiceSample = origSample }()

	return c.GenerateSpeechWithTimings(ctx, text)
}

func (c *Client) generateViaServer(ctx context.Context, text string) ([]byte, error) {
	healthURL := c.serverURL + "/health"
	resp, err := c.httpClient.Get(healthURL)
	if err != nil {
		return nil, fmt.Errorf("XTTS server not running: %w", err)
	}
	resp.Body.Close()

	reqBody := map[string]string{
		"text":        text,
		"speaker_wav": c.voiceSample,
		"language":    c.language,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.serverURL+"/tts_to_audio", bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err = c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("TTS request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("TTS server error: %s - %s", resp.Status, string(body))
	}

	return io.ReadAll(resp.Body)
}

func (c *Client) generateViaCLI(ctx context.Context, text string) ([]byte, error) {
	tmpFile, err := os.CreateTemp("", "xtts_*.wav")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	if _, err := os.Stat(c.voiceSample); os.IsNotExist(err) {
		return nil, fmt.Errorf("voice sample not found: %s", c.voiceSample)
	}

	cmd := exec.CommandContext(ctx, "tts",
		"--text", text,
		"--model_name", "tts_models/multilingual/multi-dataset/xtts_v2",
		"--speaker_wav", c.voiceSample,
		"--language_idx", c.language,
		"--out_path", tmpPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("TTS command failed: %w - %s", err, string(output))
	}

	audio, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read audio file: %w", err)
	}

	return audio, nil
}

func estimateWordTimings(text string, audio []byte) []WordTiming {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	audioDuration := float64(len(audio)) / bytesPerSecond
	timePerWord := audioDuration / float64(len(words))

	timings := make([]WordTiming, len(words))
	for i, word := range words {
		timings[i] = WordTiming{
			Word:      word,
			StartTime: float64(i) * timePerWord,
			EndTime:   float64(i+1) * timePerWord,
		}
	}

	return timings
}

func (c *Client) VoiceSample() string {
	return c.voiceSample
}

func (c *Client) SetVoiceSample(path string) {
	c.voiceSample = path
}

func (c *Client) IsServerRunning() bool {
	resp, err := c.httpClient.Get(c.serverURL + "/health")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func StartServer(ctx context.Context, modelPath string) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "python", "-m", "TTS.server", "--model_name", "tts_models/multilingual/multi-dataset/xtts_v2")
	if modelPath != "" {
		cmd = exec.CommandContext(ctx, "python", "-m", "TTS.server", "--model_path", modelPath)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start XTTS server: %w", err)
	}

	for range maxStartupWait {
		time.Sleep(time.Second)
		resp, err := http.Get(defaultServerURL + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return cmd, nil
			}
		}
	}

	cmd.Process.Kill()
	return nil, fmt.Errorf("XTTS server failed to start within %d seconds", maxStartupWait)
}

func LoadCharacterVoices(charactersDir string) ([]CharacterVoice, error) {
	entries, err := os.ReadDir(charactersDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read characters directory: %w", err)
	}

	var voices []CharacterVoice
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		voicePath := filepath.Join(charactersDir, entry.Name(), "voice.wav")
		if _, err := os.Stat(voicePath); err == nil {
			voices = append(voices, CharacterVoice{
				Name:        entry.Name(),
				VoiceSample: voicePath,
			})
		}
	}

	return voices, nil
}
