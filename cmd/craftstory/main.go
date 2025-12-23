package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"craftstory/internal/app"
	"craftstory/internal/groq"
	"craftstory/internal/imagesearch"
	"craftstory/internal/reddit"
	"craftstory/internal/storage"
	"craftstory/internal/telegram"
	"craftstory/internal/tts"
	"craftstory/internal/uploader"
	"craftstory/internal/video"
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
	case "generate":
		runGenerate()
	case "bot":
		runBotCmd()
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
  generate    Generate a video
  bot         Run Telegram approval bot

Generate options:
  -topic      Topic for video generation
  -reddit     Use random Reddit post as topic
  -upload     Upload to YouTube after generation
  -approve    Queue for Telegram approval instead of uploading

Examples:
  craftstory generate -topic "golang concurrency"
  craftstory generate -reddit -upload
  craftstory generate -topic "weird facts" -approve
  craftstory bot`)
}

func runGenerate() {
	topic := flag.String("topic", "", "Topic for video generation")
	useReddit := flag.Bool("reddit", false, "Generate video from Reddit topic")
	upload := flag.Bool("upload", false, "Upload to YouTube after generation")
	approve := flag.Bool("approve", false, "Queue video for Telegram approval before upload")
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

	var result *app.GenerateResult
	switch {
	case *useReddit:
		slog.Info("Generating video from Reddit...")
		result, err = pipeline.GenerateFromReddit(ctx)
	case *topic != "":
		slog.Info("Generating video...", "topic", *topic)
		result, err = pipeline.Generate(ctx, *topic)
	default:
		slog.Error("Please provide a topic with -topic or use -reddit")
		os.Exit(1)
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

	if *approve {
		if approval == nil {
			slog.Error("Telegram bot token not configured")
			os.Exit(1)
		}

		slog.Info("Queueing video for approval...")
		_, err := approval.RequestApproval(ctx, telegram.ApprovalRequest{
			VideoPath: result.VideoPath,
			Title:     result.Title,
			Script:    result.ScriptContent,
		})
		if err != nil {
			slog.Error("Failed to queue for approval", "error", err)
			os.Exit(1)
		}

		slog.Info("Waiting for approval via Telegram...")
		approval.StartBot()
		defer approval.StopBot()

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		for {
			select {
			case <-sigChan:
				slog.Info("Cancelled")
				return
			default:
			}

			approvalResult, video, err := approval.WaitForResult(ctx)
			if err != nil {
				continue
			}

			if approvalResult.Approved {
				slog.Info("Video approved, uploading...", "title", video.Title)
				resp, err := pipeline.Upload(ctx, app.UploadRequest{
					VideoPath:   video.VideoPath,
					Title:       video.Title,
					Description: video.Script,
				})
				if err != nil {
					slog.Error("Upload failed", "error", err)
					approval.NotifyUploadFailed(video.Title, err)
					os.Exit(1)
				}
				slog.Info("Upload complete", "url", resp.URL)
				approval.NotifyUploadComplete(video.Title, resp.URL)
			} else {
				slog.Info("Video rejected", "title", video.Title)
			}
			return
		}
	}

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

func runBotCmd() {
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

	if approval == nil {
		slog.Error("Telegram bot token not configured")
		os.Exit(1)
	}

	approval.StartBot()
	defer approval.StopBot()

	slog.Info("Bot running. Press Ctrl+C to stop.")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	pipeline := app.NewPipeline(service)

	for {
		select {
		case <-sigChan:
			slog.Info("Shutting down...")
			return
		case <-ctx.Done():
			return
		default:
		}

		result, video, err := approval.WaitForResult(ctx)
		if err != nil {
			continue
		}

		if result.Approved {
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
			slog.Info("Upload complete", "url", resp.URL)
			approval.NotifyUploadComplete(video.Title, resp.URL)
		} else {
			slog.Info("Video rejected", "title", video.Title)
		}
	}
}

func buildService(cfg *config.Config) (*app.Service, *telegram.ApprovalService, error) {
	p, err := prompts.Load()
	if err != nil {
		return nil, nil, err
	}

	llmClient, err := groq.NewClient(cfg.GroqAPIKey, cfg.Groq.Model, p)
	if err != nil {
		return nil, nil, err
	}

	var ttsProvider tts.Provider
	if cfg.RVC.Enabled {
		host := cfg.GetHost()
		modelPath := ""
		if host != nil {
			modelPath = host.RVCModelPath
		}
		ttsProvider = tts.NewRVCClient(tts.RVCOptions{
			DefaultModelPath: modelPath,
			EdgeVoice:        cfg.RVC.EdgeVoice,
			Device:           cfg.RVC.Device,
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

	var imageSearch *imagesearch.Client
	if cfg.GoogleSearchAPIKey != "" && cfg.GoogleSearchEngineID != "" {
		imageSearch = imagesearch.NewClient(cfg.GoogleSearchAPIKey, cfg.GoogleSearchEngineID)
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

	service := app.NewService(
		cfg,
		llmClient,
		ttsProvider,
		ytUploader,
		assembler,
		localStorage,
		redditClient,
		imageSearch,
		approval,
	)

	return service, approval, nil
}
