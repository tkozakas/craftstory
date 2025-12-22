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
gemini:
  project: test-project
  model: test-model
elevenlabs:
  voice_id: test-voice
content:
  script_length: 45
`
	_ = os.WriteFile(filepath.Join(tmp, "config.yaml"), []byte(yaml), 0644)

	cfg, err := Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Gemini.Model != "test-model" {
		t.Errorf("Gemini.Model = %q, want test-model", cfg.Gemini.Model)
	}
	if cfg.ElevenLabs.VoiceID != "test-voice" {
		t.Errorf("ElevenLabs.VoiceID = %q, want test-voice", cfg.ElevenLabs.VoiceID)
	}
	if cfg.Content.ScriptLength != 45 {
		t.Errorf("Content.ScriptLength = %d, want 45", cfg.Content.ScriptLength)
	}
}

func TestLoadFromEnv(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	defer func() { _ = os.Chdir(orig) }()
	_ = os.Chdir(tmp)

	_ = os.WriteFile(filepath.Join(tmp, "config.yaml"), []byte("gemini:\n  model: x"), 0644)

	t.Setenv("ELEVENLABS_API_KEY", "test-eleven")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")

	cfg, err := Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.ElevenLabsAPIKey != "test-eleven" {
		t.Errorf("ElevenLabsAPIKey = %q, want test-eleven", cfg.ElevenLabsAPIKey)
	}
	if cfg.Gemini.Project != "test-project" {
		t.Errorf("Gemini.Project = %q, want test-project", cfg.Gemini.Project)
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
