package app

import (
	"craftstory/internal/llm"
	"craftstory/internal/reddit"
	"craftstory/internal/storage"
	"craftstory/internal/telegram"
	"craftstory/internal/tenor"
	"craftstory/internal/tts"
	"craftstory/internal/uploader"
	"craftstory/internal/video"
	"craftstory/internal/visuals"
	"craftstory/pkg/config"
	"craftstory/pkg/prompts"
)

type BuildResult struct {
	Service  *Service
	Approval *telegram.ApprovalService
}

func BuildService(cfg *config.Config, verbose bool) (*BuildResult, error) {
	p, err := prompts.Load()
	if err != nil {
		return nil, err
	}

	llmClient, err := llm.NewGroqClient(cfg.GroqAPIKey, cfg.Groq.Model, p)
	if err != nil {
		return nil, err
	}

	var ttsProvider tts.Provider
	if cfg.ElevenLabs.Enabled {
		apiKeys := cfg.ElevenLabsAPIKeys
		if len(apiKeys) == 0 && cfg.ElevenLabsAPIKey != "" {
			apiKeys = []string{cfg.ElevenLabsAPIKey}
		}
		ttsProvider = tts.NewElevenLabsClient(tts.ElevenLabsConfig{
			APIKeys:    apiKeys,
			VoiceID:    cfg.ElevenLabs.HostVoice.ID,
			Speed:      cfg.ElevenLabs.Speed,
			Stability:  cfg.ElevenLabs.Stability,
			Similarity: cfg.ElevenLabs.Similarity,
		})
	}

	localStorage := storage.NewLocalStorage(cfg.Video.BackgroundDir, cfg.Video.OutputDir)
	if err := localStorage.EnsureDirectories(); err != nil {
		return nil, err
	}

	subtitleGen := video.NewSubtitleGenerator(video.SubtitleOptions{
		FontName:     cfg.Subtitles.FontName,
		FontSize:     cfg.Subtitles.FontSize,
		PrimaryColor: cfg.Subtitles.PrimaryColor,
		OutlineColor: cfg.Subtitles.OutlineColor,
		OutlineSize:  cfg.Subtitles.OutlineSize,
		ShadowSize:   cfg.Subtitles.ShadowSize,
		Bold:         cfg.Subtitles.Bold,
		Offset:       cfg.Subtitles.Offset,
	})

	var musicDir string
	if cfg.Music.Enabled {
		musicDir = cfg.Music.Dir
	}

	assembler := video.NewAssemblerWithOptions(video.AssemblerOptions{
		OutputDir:    cfg.Video.OutputDir,
		Resolution:   cfg.Video.Resolution,
		SubtitleGen:  subtitleGen,
		BgProvider:   localStorage,
		MusicDir:     musicDir,
		MusicVolume:  cfg.Music.Volume,
		MusicFadeIn:  cfg.Music.FadeIn,
		MusicFadeOut: cfg.Music.FadeOut,
		Verbose:      verbose,
	})

	redditClient := reddit.NewClient()

	var imageSearch *visuals.SearchClient
	if cfg.GoogleSearchAPIKey != "" && cfg.GoogleSearchEngineID != "" {
		imageSearch = visuals.NewSearchClient(cfg.GoogleSearchAPIKey, cfg.GoogleSearchEngineID)
	}

	var gifSearch *tenor.Client
	if cfg.TenorAPIKey != "" {
		gifSearch = tenor.NewClient(tenor.Config{APIKey: cfg.TenorAPIKey})
	}

	var ytUploader uploader.Uploader
	if cfg.YouTubeClientID != "" && cfg.YouTubeClientSecret != "" {
		auth := uploader.NewYouTubeAuth(cfg.YouTubeClientID, cfg.YouTubeClientSecret, cfg.YouTubeTokenPath)
		ytUploader = uploader.NewYouTubeUploader(auth)
	}

	var approval *telegram.ApprovalService
	if cfg.TelegramBotToken != "" {
		telegramClient := telegram.NewClient(cfg.TelegramBotToken)
		approval = telegram.NewApprovalService(telegramClient, cfg.Video.OutputDir, cfg.Telegram.DefaultChatID)
	}

	service := NewService(ServiceOptions{
		Config:      cfg,
		LLM:         llmClient,
		TTS:         ttsProvider,
		Uploader:    ytUploader,
		Assembler:   assembler,
		Storage:     localStorage,
		Reddit:      redditClient,
		ImageSearch: imageSearch,
		GIFSearch:   gifSearch,
		Approval:    approval,
	})

	return &BuildResult{
		Service:  service,
		Approval: approval,
	}, nil
}
