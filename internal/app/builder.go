package app

import (
	"craftstory/internal/content/reddit"
	"craftstory/internal/distribution"
	"craftstory/internal/distribution/telegram"
	"craftstory/internal/distribution/youtube"
	"craftstory/internal/llm/groq"
	"craftstory/internal/search"
	"craftstory/internal/search/google"
	"craftstory/internal/search/tenor"
	"craftstory/internal/speech"
	"craftstory/internal/speech/elevenlabs"
	"craftstory/internal/storage"
	"craftstory/internal/video"
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

	llmClient, err := groq.NewClient(cfg.GroqAPIKey, cfg.Groq.Model, p)
	if err != nil {
		return nil, err
	}

	var ttsProvider speech.Provider
	if cfg.ElevenLabs.Enabled {
		apiKeys := cfg.ElevenLabsAPIKeys
		if len(apiKeys) == 0 && cfg.ElevenLabsAPIKey != "" {
			apiKeys = []string{cfg.ElevenLabsAPIKey}
		}
		ttsProvider = elevenlabs.NewClient(elevenlabs.Config{
			APIKeys:    apiKeys,
			VoiceID:    cfg.ElevenLabs.HostVoice.ID,
			Speed:      cfg.ElevenLabs.Speed,
			Stability:  cfg.ElevenLabs.Stability,
			Similarity: cfg.ElevenLabs.Similarity,
		})
	} else {
		wordsPerMinute := speech.DefaultWordsPerMinute * cfg.ElevenLabs.Speed
		if wordsPerMinute <= 0 {
			wordsPerMinute = speech.DefaultWordsPerMinute
		}
		ttsProvider = speech.NewStubProvider(wordsPerMinute)
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

	var imageSearch *google.Client
	if cfg.GoogleSearchAPIKey != "" && cfg.GoogleSearchEngineID != "" {
		imageSearch = google.NewClient(google.Config{
			APIKey:   cfg.GoogleSearchAPIKey,
			EngineID: cfg.GoogleSearchEngineID,
		})
	}

	var gifSearch *tenor.Client
	if cfg.TenorAPIKey != "" && cfg.Visuals.GIFEnabled {
		gifSearch = tenor.NewClient(tenor.Config{APIKey: cfg.TenorAPIKey})
	}

	var fetcher *search.Fetcher
	if imageSearch != nil || gifSearch != nil {
		var gifSearcher search.GIFSearcher
		if gifSearch != nil {
			gifSearcher = gifSearch
		}
		fetcher = search.NewFetcher(imageSearch, gifSearcher, search.FetcherConfig{
			MaxDisplayTime: cfg.Visuals.MaxDisplayTime,
			ImageWidth:     cfg.Visuals.ImageWidth,
			ImageHeight:    cfg.Visuals.ImageHeight,
			MinGap:         cfg.Visuals.MinGap,
		})
	}

	var ytUploader distribution.Uploader
	if cfg.YouTubeClientID != "" && cfg.YouTubeClientSecret != "" {
		auth := youtube.NewAuth(cfg.YouTubeClientID, cfg.YouTubeClientSecret, cfg.YouTubeTokenPath)
		ytUploader = youtube.NewClient(auth)
	}

	var approval *telegram.ApprovalService
	if cfg.TelegramBotToken != "" {
		telegramClient := telegram.NewClient(cfg.TelegramBotToken)
		approval = telegram.NewApprovalService(telegramClient, cfg.Video.OutputDir, cfg.Telegram.DefaultChatID)
	}

	service := NewService(ServiceOptions{
		Config:    cfg,
		LLM:       llmClient,
		TTS:       ttsProvider,
		Uploader:  ytUploader,
		Assembler: assembler,
		Storage:   localStorage,
		Reddit:    redditClient,
		Fetcher:   fetcher,
		Approval:  approval,
	})

	return &BuildResult{
		Service:  service,
		Approval: approval,
	}, nil
}
