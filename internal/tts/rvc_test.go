package tts

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestFindIndexForModel(t *testing.T) {
	tmp := t.TempDir()

	_ = os.WriteFile(filepath.Join(tmp, "bocchi.pth"), []byte("fake"), 0644)
	_ = os.WriteFile(filepath.Join(tmp, "bocchi.index"), []byte("fake"), 0644)

	modelPath := filepath.Join(tmp, "bocchi.pth")
	got := findIndexForModel(modelPath)
	want := filepath.Join(tmp, "bocchi.index")
	if got != want {
		t.Errorf("findIndexForModel() = %q, want %q", got, want)
	}

	noIndexPath := filepath.Join(tmp, "other.pth")
	_ = os.WriteFile(noIndexPath, []byte("fake"), 0644)
	got = findIndexForModel(noIndexPath)
	if got != want {
		t.Errorf("findIndexForModel() = %q, want %q (glob fallback)", got, want)
	}
}

func TestNewRVCClientDefaults(t *testing.T) {
	client := NewRVCClient(RVCOptions{})

	if client.edgeVoice != "en-US-JennyNeural" {
		t.Errorf("edgeVoice = %q, want en-US-JennyNeural", client.edgeVoice)
	}
	if client.device != "cpu" {
		t.Errorf("device = %q, want cpu", client.device)
	}
}

func TestNewRVCClientCustomOptions(t *testing.T) {
	client := NewRVCClient(RVCOptions{
		DefaultModelPath: "/models/bocchi.pth",
		EdgeVoice:        "en-GB-SoniaNeural",
		Device:           "cuda:0",
	})

	if client.defaultModelPath != "/models/bocchi.pth" {
		t.Errorf("defaultModelPath = %q, want /models/bocchi.pth", client.defaultModelPath)
	}
	if client.edgeVoice != "en-GB-SoniaNeural" {
		t.Errorf("edgeVoice = %q, want en-GB-SoniaNeural", client.edgeVoice)
	}
	if client.device != "cuda:0" {
		t.Errorf("device = %q, want cuda:0", client.device)
	}
}

func TestRVCClientGenerateSpeechNoModel(t *testing.T) {
	client := NewRVCClient(RVCOptions{})

	ctx := context.Background()
	_, err := client.GenerateSpeech(ctx, "hello")
	if err == nil {
		t.Error("GenerateSpeech should fail with no model path")
	}
}

func hasEdgeTTS() bool {
	_, err := exec.LookPath("edge-tts")
	return err == nil
}

func hasRVCPython() bool {
	if os.Getenv("RVC_SKIP_CHECK") != "" {
		return true
	}
	cmd := exec.Command("python", "-c", "import rvc_python")
	return cmd.Run() == nil
}

func TestRVCClientIntegration(t *testing.T) {
	if !hasEdgeTTS() {
		t.Skip("edge-tts not installed")
	}
	if !hasRVCPython() {
		t.Skip("rvc-python not installed")
	}

	modelPath := os.Getenv("RVC_MODEL_PATH")
	if modelPath == "" {
		t.Skip("RVC_MODEL_PATH not set")
	}

	client := NewRVCClient(RVCOptions{
		DefaultModelPath: modelPath,
		EdgeVoice:        "en-US-ChristopherNeural",
		Device:           "cpu",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	audio, err := client.GenerateSpeech(ctx, "Hello, this is a test.")
	if err != nil {
		t.Fatalf("GenerateSpeech failed: %v", err)
	}

	if len(audio) < 1000 {
		t.Errorf("audio too small: %d bytes", len(audio))
	}
}

func TestRVCClientGenerateSpeechWithTimingsIntegration(t *testing.T) {
	if !hasEdgeTTS() {
		t.Skip("edge-tts not installed")
	}
	if !hasRVCPython() {
		t.Skip("rvc-python not installed")
	}

	modelPath := os.Getenv("RVC_MODEL_PATH")
	if modelPath == "" {
		t.Skip("RVC_MODEL_PATH not set")
	}

	client := NewRVCClient(RVCOptions{
		DefaultModelPath: modelPath,
		Device:           "cpu",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	result, err := client.GenerateSpeechWithTimings(ctx, "Hello world test")
	if err != nil {
		t.Fatalf("GenerateSpeechWithTimings failed: %v", err)
	}

	if len(result.Audio) < 1000 {
		t.Errorf("audio too small: %d bytes", len(result.Audio))
	}

	if len(result.Timings) != 3 {
		t.Errorf("expected 3 word timings, got %d", len(result.Timings))
	}
}

func TestRVCClientGenerateSpeechWithVoiceIntegration(t *testing.T) {
	if !hasEdgeTTS() {
		t.Skip("edge-tts not installed")
	}
	if !hasRVCPython() {
		t.Skip("rvc-python not installed")
	}

	modelPath := os.Getenv("RVC_MODEL_PATH")
	if modelPath == "" {
		t.Skip("RVC_MODEL_PATH not set")
	}

	client := NewRVCClient(RVCOptions{
		Device: "cpu",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	voice := VoiceConfig{ID: modelPath, Name: "Test Voice"}
	result, err := client.GenerateSpeechWithVoice(ctx, "Testing voice config", voice)
	if err != nil {
		t.Fatalf("GenerateSpeechWithVoice failed: %v", err)
	}

	if len(result.Audio) < 1000 {
		t.Errorf("audio too small: %d bytes", len(result.Audio))
	}
}
