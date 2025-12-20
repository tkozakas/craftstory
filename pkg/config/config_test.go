package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyDefaults(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)

	tests := []struct {
		name string
		got  any
		want any
	}{
		{"deepseekModel", cfg.DeepSeek.Model, defaultDeepSeekModel},
		{"deepseekPrompt", cfg.DeepSeek.SystemPrompt, defaultDeepSeekPrompt},
		{"elevenLabsVoice", cfg.ElevenLabs.VoiceID, defaultElevenLabsVoice},
		{"elevenLabsModel", cfg.ElevenLabs.Model, defaultElevenLabsModel},
		{"elevenLabsStability", cfg.ElevenLabs.Stability, 0.5},
		{"elevenLabsSimilarity", cfg.ElevenLabs.Similarity, 0.5},
		{"scriptLength", cfg.Content.ScriptLength, defaultScriptLength},
		{"hookDuration", cfg.Content.HookDuration, defaultHookDuration},
		{"backgroundDir", cfg.Video.BackgroundDir, defaultBackgroundDir},
		{"outputDir", cfg.Video.OutputDir, defaultOutputDir},
		{"cacheDir", cfg.Video.CacheDir, defaultCacheDir},
		{"resolution", cfg.Video.Resolution, defaultResolution},
		{"duration", cfg.Video.Duration, defaultDuration},
		{"fontName", cfg.Subtitles.FontName, defaultSubtitleFont},
		{"fontSize", cfg.Subtitles.FontSize, defaultSubtitleSize},
		{"privacyStatus", cfg.YouTube.PrivacyStatus, defaultPrivacyStatus},
		{"gcsBackgroundDir", cfg.GCS.BackgroundDir, defaultGCSBackgroundDir},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestApplyDefaultsPreservesValues(t *testing.T) {
	cfg := &Config{
		DeepSeek: DeepSeekConfig{
			Model:        "custom-model",
			SystemPrompt: "custom prompt",
		},
		ElevenLabs: ElevenLabsConfig{
			VoiceID:    "custom-voice",
			Model:      "custom-tts",
			Stability:  0.8,
			Similarity: 0.9,
		},
		Content: ContentConfig{
			ScriptLength: 100,
			HookDuration: 5,
		},
		Video: VideoConfig{
			BackgroundDir: "/custom/bg",
			OutputDir:     "/custom/out",
			CacheDir:      "/custom/cache",
			Resolution:    "720x1280",
			Duration:      30,
		},
		Subtitles: SubtitlesConfig{
			FontName: "CustomFont",
			FontSize: 72,
		},
		YouTube: YouTubeConfig{
			DefaultTags:   []string{"custom"},
			PrivacyStatus: "public",
		},
		GCS: GCSConfig{
			BackgroundDir: "custom-bg",
		},
	}

	applyDefaults(cfg)

	if cfg.DeepSeek.Model != "custom-model" {
		t.Errorf("DeepSeek.Model was overwritten")
	}
	if cfg.ElevenLabs.VoiceID != "custom-voice" {
		t.Errorf("ElevenLabs.VoiceID was overwritten")
	}
	if cfg.Content.ScriptLength != 100 {
		t.Errorf("Content.ScriptLength was overwritten")
	}
	if cfg.Video.BackgroundDir != "/custom/bg" {
		t.Errorf("Video.BackgroundDir was overwritten")
	}
	if cfg.Subtitles.FontName != "CustomFont" {
		t.Errorf("Subtitles.FontName was overwritten")
	}
	if cfg.YouTube.PrivacyStatus != "public" {
		t.Errorf("YouTube.PrivacyStatus was overwritten")
	}
	if cfg.GCS.BackgroundDir != "custom-bg" {
		t.Errorf("GCS.BackgroundDir was overwritten")
	}
}

func TestGetEnvOrDefault(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		envValue     string
		want         string
	}{
		{
			name:         "usesEnvValue",
			key:          "TEST_CONFIG_VAR",
			defaultValue: "default",
			envValue:     "from-env",
			want:         "from-env",
		},
		{
			name:         "usesDefault",
			key:          "TEST_MISSING_VAR",
			defaultValue: "default",
			envValue:     "",
			want:         "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				_ = os.Setenv(tt.key, tt.envValue)
				defer func() { _ = os.Unsetenv(tt.key) }()
			}

			got := getEnvOrDefault(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getEnvOrDefault() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoadYAMLConfig(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	yamlContent := `
deepseek:
  model: yaml-model
elevenlabs:
  voice_id: yaml-voice
content:
  script_length: 45
video:
  background_dir: /yaml/bg
subtitles:
  font_name: YamlFont
youtube:
  privacy_status: unlisted
gcs:
  enabled: true
  background_dir: yaml-backgrounds
`
	err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(yamlContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg := &Config{}
	loadYAMLConfig(cfg)

	if cfg.DeepSeek.Model != "yaml-model" {
		t.Errorf("DeepSeek.Model = %q, want %q", cfg.DeepSeek.Model, "yaml-model")
	}
	if cfg.ElevenLabs.VoiceID != "yaml-voice" {
		t.Errorf("ElevenLabs.VoiceID = %q, want %q", cfg.ElevenLabs.VoiceID, "yaml-voice")
	}
	if cfg.Content.ScriptLength != 45 {
		t.Errorf("Content.ScriptLength = %d, want %d", cfg.Content.ScriptLength, 45)
	}
	if cfg.Video.BackgroundDir != "/yaml/bg" {
		t.Errorf("Video.BackgroundDir = %q, want %q", cfg.Video.BackgroundDir, "/yaml/bg")
	}
	if cfg.Subtitles.FontName != "YamlFont" {
		t.Errorf("Subtitles.FontName = %q, want %q", cfg.Subtitles.FontName, "YamlFont")
	}
	if cfg.YouTube.PrivacyStatus != "unlisted" {
		t.Errorf("YouTube.PrivacyStatus = %q, want %q", cfg.YouTube.PrivacyStatus, "unlisted")
	}
	if !cfg.GCS.Enabled {
		t.Errorf("GCS.Enabled = %v, want %v", cfg.GCS.Enabled, true)
	}
}

func TestLoadYAMLConfigMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	cfg := &Config{}
	loadYAMLConfig(cfg)

	if cfg.DeepSeek.Model != "" {
		t.Errorf("expected empty model for missing config, got %q", cfg.DeepSeek.Model)
	}
}

func TestLoadEnvVariables(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(origDir) }()
	_ = os.Chdir(tmpDir)

	_ = os.Setenv("DEEPSEEK_API_KEY", "test-deepseek-key")
	_ = os.Setenv("ELEVENLABS_API_KEY", "test-elevenlabs-key")
	_ = os.Setenv("YOUTUBE_CLIENT_ID", "test-client-id")
	_ = os.Setenv("YOUTUBE_CLIENT_SECRET", "test-client-secret")
	_ = os.Setenv("GCS_BUCKET", "test-bucket")
	defer func() {
		_ = os.Unsetenv("DEEPSEEK_API_KEY")
		_ = os.Unsetenv("ELEVENLABS_API_KEY")
		_ = os.Unsetenv("YOUTUBE_CLIENT_ID")
		_ = os.Unsetenv("YOUTUBE_CLIENT_SECRET")
		_ = os.Unsetenv("GCS_BUCKET")
	}()

	cfg := Load()

	if cfg.DeepSeekAPIKey != "test-deepseek-key" {
		t.Errorf("DeepSeekAPIKey = %q, want %q", cfg.DeepSeekAPIKey, "test-deepseek-key")
	}
	if cfg.ElevenLabsAPIKey != "test-elevenlabs-key" {
		t.Errorf("ElevenLabsAPIKey = %q, want %q", cfg.ElevenLabsAPIKey, "test-elevenlabs-key")
	}
	if cfg.YouTubeClientID != "test-client-id" {
		t.Errorf("YouTubeClientID = %q, want %q", cfg.YouTubeClientID, "test-client-id")
	}
	if cfg.YouTubeClientSecret != "test-client-secret" {
		t.Errorf("YouTubeClientSecret = %q, want %q", cfg.YouTubeClientSecret, "test-client-secret")
	}
	if cfg.GCSBucket != "test-bucket" {
		t.Errorf("GCSBucket = %q, want %q", cfg.GCSBucket, "test-bucket")
	}
}
