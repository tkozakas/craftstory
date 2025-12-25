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

func TestLoadElevenLabsMultipleKeys(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	defer func() { _ = os.Chdir(orig) }()
	_ = os.Chdir(tmp)

	_ = os.WriteFile(filepath.Join(tmp, "config.yaml"), []byte("groq:\n  model: x"), 0644)

	t.Setenv("ELEVENLABS_API_KEYS", "key1, key2, key3")

	cfg, err := Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if len(cfg.ElevenLabsAPIKeys) != 3 {
		t.Fatalf("ElevenLabsAPIKeys length = %d, want 3", len(cfg.ElevenLabsAPIKeys))
	}
	expected := []string{"key1", "key2", "key3"}
	for i, k := range expected {
		if cfg.ElevenLabsAPIKeys[i] != k {
			t.Errorf("ElevenLabsAPIKeys[%d] = %q, want %q", i, cfg.ElevenLabsAPIKeys[i], k)
		}
	}
}

func TestLoadElevenLabsSingleKeyFallback(t *testing.T) {
	tmp := t.TempDir()
	orig, _ := os.Getwd()
	defer func() { _ = os.Chdir(orig) }()
	_ = os.Chdir(tmp)

	_ = os.WriteFile(filepath.Join(tmp, "config.yaml"), []byte("groq:\n  model: x"), 0644)

	t.Setenv("ELEVENLABS_API_KEY", "single-key")

	cfg, err := Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if len(cfg.ElevenLabsAPIKeys) != 1 {
		t.Fatalf("ElevenLabsAPIKeys length = %d, want 1", len(cfg.ElevenLabsAPIKeys))
	}
	if cfg.ElevenLabsAPIKeys[0] != "single-key" {
		t.Errorf("ElevenLabsAPIKeys[0] = %q, want single-key", cfg.ElevenLabsAPIKeys[0])
	}
}

func TestParseAPIKeys(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "single",
			input: "key1",
			want:  []string{"key1"},
		},
		{
			name:  "multiple",
			input: "key1,key2,key3",
			want:  []string{"key1", "key2", "key3"},
		},
		{
			name:  "withSpaces",
			input: "key1, key2 , key3",
			want:  []string{"key1", "key2", "key3"},
		},
		{
			name:  "emptyEntries",
			input: "key1,,key2",
			want:  []string{"key1", "key2"},
		},
		{
			name:  "empty",
			input: "",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseAPIKeys(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("parseAPIKeys(%q) length = %d, want %d", tt.input, len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseAPIKeys(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}
