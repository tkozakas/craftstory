package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromYAML(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	defer func() { _ = os.Chdir(orig) }()
	_ = os.Chdir(tmp)

	yaml := `
groq:
  model: test-model
elevenlabs:
  enabled: true
  host_voice:
    id: "test-voice-id"
    name: "TestVoice"
content:
  word_count: 150
`
	_ = os.WriteFile(filepath.Join(tmp, "config.yaml"), []byte(yaml), 0644)

	cfg, err := Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Groq.Model != "test-model" {
		t.Errorf("Groq.Model = %q, want test-model", cfg.Groq.Model)
	}
	if !cfg.ElevenLabs.Enabled {
		t.Error("ElevenLabs.Enabled = false, want true")
	}
	if cfg.Content.WordCount != 150 {
		t.Errorf("Content.WordCount = %d, want 150", cfg.Content.WordCount)
	}
}

func TestLoadFromEnv(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	defer func() { _ = os.Chdir(orig) }()
	_ = os.Chdir(tmp)

	_ = os.WriteFile(filepath.Join(tmp, "config.yaml"), []byte("groq:\n  model: x"), 0644)

	t.Setenv("GROQ_API_KEY", "test-groq")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")

	cfg, err := Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.GroqAPIKey != "test-groq" {
		t.Errorf("GroqAPIKey = %q, want test-groq", cfg.GroqAPIKey)
	}
	if cfg.GCPProject != "test-project" {
		t.Errorf("GCPProject = %q, want test-project", cfg.GCPProject)
	}
}

func TestLoadMissingConfigFile(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	defer func() { _ = os.Chdir(orig) }()
	_ = os.Chdir(tmp)

	_, err := Load(context.Background())
	if err == nil {
		t.Error("Load() should fail when config.yaml missing")
	}
}

func TestLoadElevenLabsVoices(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	defer func() { _ = os.Chdir(orig) }()
	_ = os.Chdir(tmp)

	configYAML := `
groq:
  model: test
elevenlabs:
  enabled: true
  host_voice:
    id: "adam-id"
    name: "Adam"
    subtitle_color: "#00BFFF"
  guest_voice:
    id: "bella-id"
    name: "Bella"
    subtitle_color: "#FF69B4"
`
	_ = os.WriteFile(filepath.Join(tmp, "config.yaml"), []byte(configYAML), 0644)

	cfg, err := Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.ElevenLabs.HostVoice.ID != "adam-id" {
		t.Errorf("HostVoice.ID = %q, want adam-id", cfg.ElevenLabs.HostVoice.ID)
	}
	if cfg.ElevenLabs.HostVoice.Name != "Adam" {
		t.Errorf("HostVoice.Name = %q, want Adam", cfg.ElevenLabs.HostVoice.Name)
	}
	if cfg.ElevenLabs.GuestVoice.ID != "bella-id" {
		t.Errorf("GuestVoice.ID = %q, want bella-id", cfg.ElevenLabs.GuestVoice.ID)
	}
}
