package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"craftstory/internal/app"
	"craftstory/internal/llm"
	"craftstory/internal/reddit"
	"craftstory/internal/storage"
	"craftstory/internal/telegram"
	"craftstory/internal/tts"
	"craftstory/internal/uploader"
	"craftstory/internal/video"
	"craftstory/internal/visuals"
	"craftstory/pkg/config"
	"craftstory/pkg/prompts"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	os.Args = append([]string{os.Args[0]}, os.Args[2:]...)

	switch cmd {
	case "run":
		runCronCmd()
	case "once":
		runOnceCmd()
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Usage: craftstory <command> [options]

Commands:
  run         Cron mode: generate from Reddit, queue for approval, repeat
  once        Generate a single video

Run options:
  -interval   Interval between generations (default: 1h)
  -upload     Upload directly instead of queueing for approval

Once options:
  -topic      Topic for video generation
  -reddit     Use random Reddit post as topic
  -upload     Upload to YouTube after generation

Examples:
  craftstory run                       # cron mode, generates every hour
  craftstory run -interval 30m         # cron with 30min interval
  craftstory once -topic "golang tips" # single video
  craftstory once -reddit -upload      # single from Reddit + upload`)
}

func runCronCmd() {
	interval := flag.Duration("interval", time.Hour, "Interval between generations")
	upload := flag.Bool("upload", false, "Upload directly instead of queueing for approval")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load(ctx)
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	service, approval, err := buildService(cfg)
	if err != nil {
		slog.Error("Failed to build service", "error", err)
		os.Exit(1)
	}

	pipeline := app.NewPipeline(service)

	if !*upload && approval != nil {
		approval.StartBot()
		defer approval.StopBot()

		go handleApprovals(ctx, pipeline, approval)
	}

	slog.Info("Starting cron mode", "interval", *interval, "approval", !*upload && approval != nil)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	generate := func() {
		if approval != nil && approval.Queue().IsFull() {
			slog.Info("Queue is full, skipping generation")
			return
		}

		slog.Info("Generating video from Reddit...")
		result, err := pipeline.GenerateFromReddit(ctx)
		if err != nil {
			slog.Error("Generation failed", "error", err)
			return
		}

		slog.Info("Video generated", "title", result.Title, "path", result.VideoPath)

		if *upload {
			resp, err := pipeline.Upload(ctx, app.UploadRequest{
				VideoPath:   result.VideoPath,
				Title:       result.Title,
				Description: result.ScriptContent,
			})
			if err != nil {
				slog.Error("Upload failed", "error", err)
				return
			}
			slog.Info("Upload complete", "url", resp.URL)
			return
		}

		if approval != nil {
			_, err := approval.RequestApproval(ctx, telegram.ApprovalRequest{
				VideoPath: result.VideoPath,
				Title:     result.Title,
				Script:    result.ScriptContent,
			})
			if err != nil {
				slog.Error("Failed to queue for approval", "error", err)
			}
		}
	}

	generate()

	for {
		select {
		case <-sigChan:
			slog.Info("Shutting down...")
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			generate()
		}
	}
}

func handleApprovals(ctx context.Context, pipeline *app.Pipeline, approval *telegram.ApprovalService) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		result, video, err := approval.WaitForResult(ctx)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}

		if !result.Approved {
			slog.Info("Video rejected", "title", video.Title)
			continue
		}

		slog.Info("Video approved, uploading...", "title", video.Title)
		resp, err := pipeline.Upload(ctx, app.UploadRequest{
			VideoPath:   video.VideoPath,
			Title:       video.Title,
			Description: video.Script,
		})
		if err != nil {
			slog.Error("Upload failed", "error", err)
			approval.NotifyUploadFailed(video.Title, err)
			continue
		}

		slog.Info("Upload complete", "title", video.Title, "url", resp.URL)
		approval.NotifyUploadComplete(video.Title, resp.URL)
	}
}

func runOnceCmd() {
	topic := flag.String("topic", "", "Topic for video generation")
	useReddit := flag.Bool("reddit", false, "Generate video from Reddit topic")
	upload := flag.Bool("upload", false, "Upload to YouTube after generation")
	flag.Parse()

	if *topic == "" && !*useReddit {
		slog.Error("Please provide -topic or -reddit")
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load(ctx)
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	service, _, err := buildService(cfg)
	if err != nil {
		slog.Error("Failed to build service", "error", err)
		os.Exit(1)
	}

	pipeline := app.NewPipeline(service)

	var result *app.GenerateResult
	if *useReddit {
		slog.Info("Generating video from Reddit...")
		result, err = pipeline.GenerateFromReddit(ctx)
	} else {
		slog.Info("Generating video...", "topic", *topic)
		result, err = pipeline.Generate(ctx, *topic)
	}

	if err != nil {
		slog.Error("Generation failed", "error", err)
		os.Exit(1)
	}

	slog.Info("Video generated",
		"title", result.Title,
		"path", result.VideoPath,
		"duration", result.Duration,
	)

	if *upload {
		slog.Info("Uploading to YouTube...")
		resp, err := pipeline.Upload(ctx, app.UploadRequest{
			VideoPath:   result.VideoPath,
			Title:       result.Title,
			Description: result.ScriptContent,
		})
		if err != nil {
			slog.Error("Upload failed", "error", err)
			os.Exit(1)
		}
		slog.Info("Upload complete", "url", resp.URL)
	}
}

func buildService(cfg *config.Config) (*app.Service, *telegram.ApprovalService, error) {
	p, err := prompts.Load()
	if err != nil {
		return nil, nil, err
	}

	llmClient, err := llm.NewGroqClient(cfg.GroqAPIKey, cfg.Groq.Model, p)
	if err != nil {
		return nil, nil, err
	}

	var ttsProvider tts.Provider
	if cfg.ElevenLabs.Enabled {
		ttsProvider = tts.NewElevenLabsClient(cfg.ElevenLabsAPIKey, tts.ElevenLabsOptions{
			VoiceID: cfg.ElevenLabs.HostVoice.ID,
			Speed:   cfg.ElevenLabs.Speed,
		})
	}

	localStorage := storage.NewLocalStorage(cfg.Video.BackgroundDir, cfg.Video.OutputDir)
	if err := localStorage.EnsureDirectories(); err != nil {
		return nil, nil, err
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
	})

	redditClient := reddit.NewClient()

	var imageSearch *visuals.SearchClient
	if cfg.GoogleSearchAPIKey != "" && cfg.GoogleSearchEngineID != "" {
		imageSearch = visuals.NewSearchClient(cfg.GoogleSearchAPIKey, cfg.GoogleSearchEngineID)
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

	service := app.NewService(app.ServiceOptions{
		Config:      cfg,
		LLM:         llmClient,
		TTS:         ttsProvider,
		Uploader:    ytUploader,
		Assembler:   assembler,
		Storage:     localStorage,
		Reddit:      redditClient,
		ImageSearch: imageSearch,
		Approval:    approval,
	})

	return service, approval, nil
}
