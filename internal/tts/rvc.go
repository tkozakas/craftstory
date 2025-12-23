package tts

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type RVCClient struct {
	defaultModelPath string
	edgeVoice        string
	device           string
}

type RVCOptions struct {
	DefaultModelPath string
	EdgeVoice        string
	Device           string
}

func NewRVCClient(opts RVCOptions) *RVCClient {
	edgeVoice := opts.EdgeVoice
	if edgeVoice == "" {
		edgeVoice = "en-US-JennyNeural"
	}
	device := opts.Device
	if device == "" {
		device = "cpu"
	}
	return &RVCClient{
		defaultModelPath: opts.DefaultModelPath,
		edgeVoice:        edgeVoice,
		device:           device,
	}
}

func (c *RVCClient) GenerateSpeech(ctx context.Context, text string) ([]byte, error) {
	return c.generate(ctx, text, c.defaultModelPath)
}

func (c *RVCClient) GenerateSpeechWithTimings(ctx context.Context, text string) (*SpeechResult, error) {
	audio, duration, err := c.generateWithDuration(ctx, text, c.defaultModelPath)
	if err != nil {
		return nil, err
	}
	return &SpeechResult{
		Audio:   audio,
		Timings: estimateTimingsFromDuration(text, duration),
	}, nil
}

func (c *RVCClient) GenerateSpeechWithVoice(ctx context.Context, text string, voice VoiceConfig) (*SpeechResult, error) {
	modelPath := voice.ID
	if modelPath == "" {
		modelPath = c.defaultModelPath
	}
	audio, duration, err := c.generateWithDuration(ctx, text, modelPath)
	if err != nil {
		return nil, err
	}
	return &SpeechResult{
		Audio:   audio,
		Timings: estimateTimingsFromDuration(text, duration),
	}, nil
}

func (c *RVCClient) generate(ctx context.Context, text, modelPath string) ([]byte, error) {
	audio, _, err := c.generateWithDuration(ctx, text, modelPath)
	return audio, err
}

func (c *RVCClient) generateWithDuration(ctx context.Context, text, modelPath string) ([]byte, float64, error) {
	if modelPath == "" {
		return nil, 0, fmt.Errorf("no model path specified")
	}

	absModelPath, err := filepath.Abs(modelPath)
	if err != nil {
		return nil, 0, fmt.Errorf("resolve model path: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "rvc-")
	if err != nil {
		return nil, 0, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	ttsFile := filepath.Join(tempDir, "tts.mp3")
	outFile := filepath.Join(tempDir, "out.wav")

	cmd := exec.CommandContext(ctx, "uv", "run", "edge-tts",
		"--text", addPauses(text),
		"--voice", c.edgeVoice,
		"--write-media", ttsFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, 0, fmt.Errorf("edge-tts: %w: %s", err, out)
	}

	args := []string{"run", "python", "-m", "rvc_python", "cli",
		"-i", ttsFile,
		"-o", outFile,
		"-mp", absModelPath,
		"-de", c.device,
	}

	indexPath := findIndexForModel(absModelPath)
	if indexPath != "" {
		args = append(args, "-ip", indexPath)
	}

	cmd = exec.CommandContext(ctx, "uv", args...)
	cmd.Env = append(os.Environ(), "HSA_OVERRIDE_GFX_VERSION=10.3.0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, 0, fmt.Errorf("rvc: %w: %s", err, out)
	}

	duration, err := getAudioDuration(ctx, outFile)
	if err != nil {
		return nil, 0, fmt.Errorf("get duration: %w", err)
	}

	audio, err := os.ReadFile(outFile)
	if err != nil {
		return nil, 0, err
	}

	return audio, duration, nil
}

func findIndexForModel(modelPath string) string {
	dir := filepath.Dir(modelPath)
	base := filepath.Base(modelPath)
	name := strings.TrimSuffix(base, filepath.Ext(base))

	patterns := []string{
		filepath.Join(dir, name+".index"),
		filepath.Join(dir, "*.index"),
	}
	for _, pattern := range patterns {
		if matches, _ := filepath.Glob(pattern); len(matches) > 0 {
			return matches[0]
		}
	}
	return ""
}
