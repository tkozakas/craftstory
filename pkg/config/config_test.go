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
rvc:
  enabled: true
  edge_voice: en-US-ChristopherNeural
  device: cpu
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
	if !cfg.RVC.Enabled {
		t.Error("RVC.Enabled = false, want true")
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

func TestLoadCharactersWithRVCModel(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	defer func() { _ = os.Chdir(orig) }()
	_ = os.Chdir(tmp)

	charDir := filepath.Join(tmp, "characters", "bocchi")
	_ = os.MkdirAll(charDir, 0755)

	charYAML := `
name: Bocchi
role: host
image: bocchi.png
rvc_model: bocchi.pth
subtitle_color: "#FF69B4"
`
	_ = os.WriteFile(filepath.Join(charDir, "character.yaml"), []byte(charYAML), 0644)
	_ = os.WriteFile(filepath.Join(charDir, "bocchi.png"), []byte("fake"), 0644)
	_ = os.WriteFile(filepath.Join(charDir, "bocchi.pth"), []byte("fake"), 0644)

	configYAML := `
groq:
  model: test
characters:
  dir: ./characters
  host: Bocchi
`
	_ = os.WriteFile(filepath.Join(tmp, "config.yaml"), []byte(configYAML), 0644)

	cfg, err := Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	host := cfg.GetHost()
	if host == nil {
		t.Fatal("GetHost() returned nil")
	}

	if host.Name != "Bocchi" {
		t.Errorf("host.Name = %q, want Bocchi", host.Name)
	}

	expectedImagePath := filepath.Join(tmp, "characters", "bocchi", "bocchi.png")
	if host.ImagePath != expectedImagePath {
		t.Errorf("host.ImagePath = %q, want %q", host.ImagePath, expectedImagePath)
	}

	expectedModelPath := filepath.Join(tmp, "characters", "bocchi", "bocchi.pth")
	if host.RVCModelPath != expectedModelPath {
		t.Errorf("host.RVCModelPath = %q, want %q", host.RVCModelPath, expectedModelPath)
	}
}
