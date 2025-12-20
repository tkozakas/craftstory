package config

import (
	"log/slog"
	"os"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

const (
	defaultConfigPath       = "config.yaml"
	defaultBackgroundDir    = "./assets/backgrounds"
	defaultOutputDir        = "./output"
	defaultCacheDir         = "./.cache"
	defaultResolution       = "1080x1920"
	defaultDuration         = 60
	defaultScriptLength     = 50
	defaultHookDuration     = 3
	defaultPrivacyStatus    = "private"
	defaultTokenPath        = "./youtube_token.json"
	defaultDeepSeekModel    = "deepseek-chat"
	defaultDeepSeekPrompt   = "You are a creative scriptwriter for short-form viral videos. Generate engaging, high-retention scripts with hooks in the first 3 seconds. Use short punchy sentences."
	defaultElevenLabsVoice  = "JBFqnCBsd6RMkjVDRZzb"
	defaultElevenLabsModel  = "eleven_flash_v2_5"
	defaultSubtitleFont     = "Arial"
	defaultSubtitleSize     = 128
	defaultGCSBackgroundDir = "backgrounds"
)

type Config struct {
	DeepSeekAPIKey      string
	ElevenLabsAPIKey    string
	YouTubeClientID     string
	YouTubeClientSecret string
	YouTubeTokenPath    string
	GCSBucket           string

	DeepSeek   DeepSeekConfig   `yaml:"deepseek"`
	ElevenLabs ElevenLabsConfig `yaml:"elevenlabs"`
	Content    ContentConfig    `yaml:"content"`
	Video      VideoConfig      `yaml:"video"`
	Subtitles  SubtitlesConfig  `yaml:"subtitles"`
	YouTube    YouTubeConfig    `yaml:"youtube"`
	GCS        GCSConfig        `yaml:"gcs"`
}

type DeepSeekConfig struct {
	Model        string `yaml:"model"`
	SystemPrompt string `yaml:"system_prompt"`
}

type ElevenLabsConfig struct {
	VoiceID    string  `yaml:"voice_id"`
	Model      string  `yaml:"model"`
	Stability  float64 `yaml:"stability"`
	Similarity float64 `yaml:"similarity"`
}

type ContentConfig struct {
	ScriptLength int `yaml:"script_length"`
	HookDuration int `yaml:"hook_duration"`
}

type VideoConfig struct {
	BackgroundDir string `yaml:"background_dir"`
	OutputDir     string `yaml:"output_dir"`
	CacheDir      string `yaml:"cache_dir"`
	Resolution    string `yaml:"resolution"`
	Duration      int    `yaml:"duration"`
}

type SubtitlesConfig struct {
	FontName string `yaml:"font_name"`
	FontSize int    `yaml:"font_size"`
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

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		slog.Warn("No .env file found, relying on environment variables")
	}

	cfg := &Config{
		DeepSeekAPIKey:      os.Getenv("DEEPSEEK_API_KEY"),
		ElevenLabsAPIKey:    os.Getenv("ELEVENLABS_API_KEY"),
		YouTubeClientID:     os.Getenv("YOUTUBE_CLIENT_ID"),
		YouTubeClientSecret: os.Getenv("YOUTUBE_CLIENT_SECRET"),
		YouTubeTokenPath:    getEnvOrDefault("YOUTUBE_TOKEN_PATH", defaultTokenPath),
		GCSBucket:           os.Getenv("GCS_BUCKET"),
	}

	loadYAMLConfig(cfg)
	applyDefaults(cfg)

	return cfg
}

func loadYAMLConfig(cfg *Config) {
	data, err := os.ReadFile(defaultConfigPath)
	if err != nil {
		slog.Warn("No config.yaml found, using defaults")
		return
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		slog.Error("Failed to parse config.yaml", "error", err)
	}
}

func applyDefaults(cfg *Config) {
	applyDeepSeekDefaults(cfg)
	applyElevenLabsDefaults(cfg)
	applyContentDefaults(cfg)
	applyVideoDefaults(cfg)
	applySubtitlesDefaults(cfg)
	applyYouTubeDefaults(cfg)
	applyGCSDefaults(cfg)
}

func applyDeepSeekDefaults(cfg *Config) {
	if cfg.DeepSeek.Model == "" {
		cfg.DeepSeek.Model = defaultDeepSeekModel
	}
	if cfg.DeepSeek.SystemPrompt == "" {
		cfg.DeepSeek.SystemPrompt = defaultDeepSeekPrompt
	}
}

func applyElevenLabsDefaults(cfg *Config) {
	if cfg.ElevenLabs.VoiceID == "" {
		cfg.ElevenLabs.VoiceID = defaultElevenLabsVoice
	}
	if cfg.ElevenLabs.Model == "" {
		cfg.ElevenLabs.Model = defaultElevenLabsModel
	}
	if cfg.ElevenLabs.Stability == 0 {
		cfg.ElevenLabs.Stability = 0.5
	}
	if cfg.ElevenLabs.Similarity == 0 {
		cfg.ElevenLabs.Similarity = 0.5
	}
}

func applyContentDefaults(cfg *Config) {
	if cfg.Content.ScriptLength == 0 {
		cfg.Content.ScriptLength = defaultScriptLength
	}
	if cfg.Content.HookDuration == 0 {
		cfg.Content.HookDuration = defaultHookDuration
	}
}

func applyVideoDefaults(cfg *Config) {
	if cfg.Video.BackgroundDir == "" {
		cfg.Video.BackgroundDir = defaultBackgroundDir
	}
	if cfg.Video.OutputDir == "" {
		cfg.Video.OutputDir = defaultOutputDir
	}
	if cfg.Video.CacheDir == "" {
		cfg.Video.CacheDir = defaultCacheDir
	}
	if cfg.Video.Resolution == "" {
		cfg.Video.Resolution = defaultResolution
	}
	if cfg.Video.Duration == 0 {
		cfg.Video.Duration = defaultDuration
	}
}

func applySubtitlesDefaults(cfg *Config) {
	if cfg.Subtitles.FontName == "" {
		cfg.Subtitles.FontName = defaultSubtitleFont
	}
	if cfg.Subtitles.FontSize == 0 {
		cfg.Subtitles.FontSize = defaultSubtitleSize
	}
}

func applyYouTubeDefaults(cfg *Config) {
	if len(cfg.YouTube.DefaultTags) == 0 {
		cfg.YouTube.DefaultTags = []string{"shorts", "facts", "history", "space"}
	}
	if cfg.YouTube.PrivacyStatus == "" {
		cfg.YouTube.PrivacyStatus = defaultPrivacyStatus
	}
}

func applyGCSDefaults(cfg *Config) {
	if cfg.GCS.BackgroundDir == "" {
		cfg.GCS.BackgroundDir = defaultGCSBackgroundDir
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
