package config

import (
	"context"
	"fmt"
	"os"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"gopkg.in/yaml.v3"
)

type Config struct {
	ElevenLabsAPIKey     string
	YouTubeClientID      string
	YouTubeClientSecret  string
	YouTubeTokenPath     string
	GCSBucket            string
	GoogleSearchAPIKey   string
	GoogleSearchEngineID string

	Gemini     GeminiConfig     `yaml:"gemini"`
	ElevenLabs ElevenLabsConfig `yaml:"elevenlabs"`
	Content    ContentConfig    `yaml:"content"`
	Video      VideoConfig      `yaml:"video"`
	Music      MusicConfig      `yaml:"music"`
	Subtitles  SubtitlesConfig  `yaml:"subtitles"`
	YouTube    YouTubeConfig    `yaml:"youtube"`
	GCS        GCSConfig        `yaml:"gcs"`
	Visuals    VisualsConfig    `yaml:"visuals"`
}

type GeminiConfig struct {
	Project  string `yaml:"project"`
	Location string `yaml:"location"`
	Model    string `yaml:"model"`
}

type ElevenLabsConfig struct {
	VoiceID    string  `yaml:"voice_id"`
	Model      string  `yaml:"model"`
	Stability  float64 `yaml:"stability"`
	Similarity float64 `yaml:"similarity"`
	Voices     []Voice `yaml:"voices"`
}

type Voice struct {
	ID         string  `yaml:"id"`
	Name       string  `yaml:"name"`
	Stability  float64 `yaml:"stability"`
	Similarity float64 `yaml:"similarity"`
	Avatar     string  `yaml:"avatar"`
}

type ContentConfig struct {
	ScriptLength     int  `yaml:"script_length"`
	HookDuration     int  `yaml:"hook_duration"`
	ConversationMode bool `yaml:"conversation_mode"`
}

type VideoConfig struct {
	BackgroundDir string  `yaml:"background_dir"`
	OutputDir     string  `yaml:"output_dir"`
	CacheDir      string  `yaml:"cache_dir"`
	Resolution    string  `yaml:"resolution"`
	MaxDuration   float64 `yaml:"max_duration"`
}

type MusicConfig struct {
	Enabled bool    `yaml:"enabled"`
	Dir     string  `yaml:"dir"`
	Volume  float64 `yaml:"volume"`
	FadeIn  float64 `yaml:"fade_in"`
	FadeOut float64 `yaml:"fade_out"`
}

type SubtitlesConfig struct {
	FontName     string  `yaml:"font_name"`
	FontSize     int     `yaml:"font_size"`
	PrimaryColor string  `yaml:"primary_color"`
	OutlineColor string  `yaml:"outline_color"`
	OutlineSize  int     `yaml:"outline_size"`
	ShadowSize   int     `yaml:"shadow_size"`
	Bold         bool    `yaml:"bold"`
	Offset       float64 `yaml:"offset"`
}

type YouTubeConfig struct {
	ChannelID     string   `yaml:"channel_id"`
	DefaultTags   []string `yaml:"default_tags"`
	PrivacyStatus string   `yaml:"privacy_status"`
}

type GCSConfig struct {
	Enabled       bool   `yaml:"enabled"`
	BackgroundDir string `yaml:"background_dir"`
}

type VisualsConfig struct {
	Enabled     bool    `yaml:"enabled"`
	Position    string  `yaml:"position"`
	DisplayTime float64 `yaml:"display_time"`
	ImageWidth  int     `yaml:"image_width"`
	ImageHeight int     `yaml:"image_height"`
	MinGap      float64 `yaml:"min_gap"`
}

func Load(ctx context.Context) (*Config, error) {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return nil, fmt.Errorf("read config.yaml: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config.yaml: %w", err)
	}

	if cfg.Gemini.Project == "" {
		cfg.Gemini.Project = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}

	cfg.YouTubeTokenPath = envOr("YOUTUBE_TOKEN_PATH", "./youtube_token.json")
	cfg.GCSBucket = os.Getenv("GCS_BUCKET")
	cfg.GoogleSearchEngineID = os.Getenv("GOOGLE_SEARCH_ENGINE_ID")

	cfg.loadSecrets(ctx)

	return cfg, nil
}

func (cfg *Config) loadSecrets(ctx context.Context) {
	secrets := []struct {
		secretName string
		envName    string
		dest       *string
	}{
		{"elevenlabs-api-key", "ELEVENLABS_API_KEY", &cfg.ElevenLabsAPIKey},
		{"youtube-client-id", "YOUTUBE_CLIENT_ID", &cfg.YouTubeClientID},
		{"youtube-client-secret", "YOUTUBE_CLIENT_SECRET", &cfg.YouTubeClientSecret},
		{"google-search-api-key", "GOOGLE_SEARCH_API_KEY", &cfg.GoogleSearchAPIKey},
	}

	var client *secretmanager.Client
	if cfg.Gemini.Project != "" {
		var err error
		client, err = secretmanager.NewClient(ctx)
		if err == nil {
			defer client.Close()
		}
	}

	for _, s := range secrets {
		if client != nil && cfg.Gemini.Project != "" {
			if val, err := accessSecret(ctx, client, cfg.Gemini.Project, s.secretName); err == nil {
				*s.dest = val
				continue
			}
		}
		*s.dest = os.Getenv(s.envName)
	}
}

func accessSecret(ctx context.Context, client *secretmanager.Client, project, name string) (string, error) {
	req := &secretmanagerpb.AccessSecretVersionRequest{
		Name: fmt.Sprintf("projects/%s/secrets/%s/versions/latest", project, name),
	}
	result, err := client.AccessSecretVersion(ctx, req)
	if err != nil {
		return "", err
	}
	return string(result.Payload.Data), nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
