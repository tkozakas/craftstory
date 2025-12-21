package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"time"

	"craftstory/internal/app"
	"craftstory/internal/deepseek"
	"craftstory/internal/elevenlabs"
	"craftstory/internal/imagesearch"
	"craftstory/internal/reddit"
	"craftstory/internal/storage"
	"craftstory/internal/tts"
	"craftstory/internal/uploader"
	"craftstory/internal/video"
	"craftstory/internal/xtts"
	"craftstory/pkg/config"
	"craftstory/pkg/prompts"
)

func main() {
	rand.New(rand.NewSource(time.Now().UnixNano()))

	generateCmd := flag.NewFlagSet("generate", flag.ExitOnError)
	uploadCmd := flag.NewFlagSet("upload", flag.ExitOnError)
	authCmd := flag.NewFlagSet("auth", flag.ExitOnError)

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cfg := config.Load()

	switch os.Args[1] {
	case "generate":
		handleGenerate(generateCmd, cfg)
	case "upload":
		handleUpload(uploadCmd, cfg)
	case "auth":
		handleAuth(authCmd, cfg)
	default:
		printUsage()
		os.Exit(1)
	}
}

func handleGenerate(cmd *flag.FlagSet, cfg *config.Config) {
	topic := cmd.String("topic", "", "Topic for the video script (required)")
	upload := cmd.Bool("upload", false, "Upload to YouTube after generation")
	provider := cmd.String("provider", "elevenlabs", "TTS provider: elevenlabs or xtts")

	if err := cmd.Parse(os.Args[2:]); err != nil {
		slog.Error("Failed to parse flags", "error", err)
		os.Exit(1)
	}

	if *topic == "" {
		slog.Error("-topic is required")
		os.Exit(1)
	}

	cfg.TTS.Provider = *provider

	svc := initService(cfg)
	pipeline := app.NewPipeline(svc)

	ctx := context.Background()

	if *upload {
		resp, err := pipeline.GenerateAndUpload(ctx, *topic)
		if err != nil {
			slog.Error("Failed to generate and upload video", "error", err)
			os.Exit(1)
		}
		fmt.Printf("\n✅ Video uploaded successfully!\n")
		fmt.Printf("   URL: %s\n\n", resp.URL)
	} else {
		result, err := pipeline.Generate(ctx, *topic)
		if err != nil {
			slog.Error("Failed to generate video", "error", err)
			os.Exit(1)
		}
		fmt.Printf("\n✅ Video generated successfully!\n")
		fmt.Printf("┌─────────────────────────────────────────────────────────────\n")
		fmt.Printf("│ Title:    %s\n", result.Title)
		fmt.Printf("│ Output:   %s\n", result.OutputDir)
		fmt.Printf("│ Video:    %s\n", result.VideoPath)
		fmt.Printf("│ Duration: %.2f seconds\n", result.Duration)
		fmt.Printf("└─────────────────────────────────────────────────────────────\n\n")
	}
}

func handleUpload(cmd *flag.FlagSet, cfg *config.Config) {
	videoPath := cmd.String("video", "", "Path to video file")
	title := cmd.String("title", "", "Video title")
	description := cmd.String("description", "", "Video description")

	if err := cmd.Parse(os.Args[2:]); err != nil {
		slog.Error("Failed to parse flags", "error", err)
		os.Exit(1)
	}

	if *videoPath == "" || *title == "" {
		slog.Error("Video path and title are required")
		os.Exit(1)
	}

	svc := initService(cfg)
	pipeline := app.NewPipeline(svc)

	ctx := context.Background()
	resp, err := pipeline.Upload(ctx, *videoPath, *title, *description)
	if err != nil {
		slog.Error("Failed to upload video", "error", err)
		os.Exit(1)
	}

	fmt.Printf("Video uploaded successfully: %s\n", resp.URL)
}

func handleAuth(cmd *flag.FlagSet, cfg *config.Config) {
	if err := cmd.Parse(os.Args[2:]); err != nil {
		slog.Error("Failed to parse flags", "error", err)
		os.Exit(1)
	}

	auth := uploader.NewYouTubeAuth(cfg.YouTubeClientID, cfg.YouTubeClientSecret, cfg.YouTubeTokenPath)

	if auth.IsAuthenticated() {
		fmt.Println("Already authenticated with YouTube")
		return
	}

	fmt.Println("Visit this URL to authenticate:")
	fmt.Println(auth.GetAuthURL())
	fmt.Print("\nEnter the authorization code: ")

	var code string
	if _, err := fmt.Scanln(&code); err != nil {
		slog.Error("Failed to read authorization code", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()
	if err := auth.Exchange(ctx, code); err != nil {
		slog.Error("Failed to exchange code", "error", err)
		os.Exit(1)
	}

	fmt.Println("Authentication successful!")
}

func initService(cfg *config.Config) *app.Service {
	ctx := context.Background()

	p, err := prompts.Load()
	if err != nil {
		slog.Warn("Failed to load prompts, using defaults", "error", err)
	}

	dsClient := deepseek.NewClient(cfg.DeepSeekAPIKey, deepseek.Options{
		Model:        cfg.DeepSeek.Model,
		SystemPrompt: cfg.DeepSeek.SystemPrompt,
		Prompts:      p,
	})

	ttsProvider := initTTSProvider(cfg)

	ytAuth := uploader.NewYouTubeAuth(cfg.YouTubeClientID, cfg.YouTubeClientSecret, cfg.YouTubeTokenPath)
	ytUploader := uploader.NewYouTubeUploader(ytAuth)

	subtitleGen := video.NewSubtitleGenerator(video.SubtitleOptions{
		FontName:     cfg.Subtitles.FontName,
		FontSize:     cfg.Subtitles.FontSize,
		PrimaryColor: cfg.Subtitles.PrimaryColor,
		OutlineColor: cfg.Subtitles.OutlineColor,
		OutlineSize:  cfg.Subtitles.OutlineSize,
		ShadowSize:   cfg.Subtitles.ShadowSize,
		Bold:         cfg.Subtitles.Bold,
	})

	var bgProvider storage.BackgroundProvider
	if cfg.GCS.Enabled && cfg.GCSBucket != "" {
		gcsStorage, err := storage.NewGCSStorage(ctx, cfg.GCSBucket, cfg.GCS.BackgroundDir, cfg.Video.CacheDir)
		if err != nil {
			slog.Error("Failed to create GCS storage", "error", err)
			os.Exit(1)
		}
		if err := gcsStorage.EnsureCacheDir(); err != nil {
			slog.Error("Failed to create cache directory", "error", err)
			os.Exit(1)
		}
		bgProvider = gcsStorage
		slog.Info("Using GCS storage", "bucket", cfg.GCSBucket)
	} else {
		bgProvider = storage.NewLocalStorage(cfg.Video.BackgroundDir, cfg.Video.OutputDir)
		slog.Info("Using local storage", "dir", cfg.Video.BackgroundDir)
	}

	assembler := video.NewAssemblerWithOptions(video.AssemblerOptions{
		OutputDir:   cfg.Video.OutputDir,
		Resolution:  cfg.Video.Resolution,
		SubtitleGen: subtitleGen,
		BgProvider:  bgProvider,
	})
	localStorage := storage.NewLocalStorage(cfg.Video.BackgroundDir, cfg.Video.OutputDir)
	redditClient := reddit.NewClient()

	var imgSearchClient *imagesearch.Client
	if cfg.Visuals.Enabled && cfg.GoogleSearchAPIKey != "" && cfg.GoogleSearchEngineID != "" {
		imgSearchClient = imagesearch.NewClient(cfg.GoogleSearchAPIKey, cfg.GoogleSearchEngineID)
		slog.Info("Image search enabled")
	}

	return app.NewService(cfg, dsClient, ttsProvider, ytUploader, assembler, localStorage, redditClient, imgSearchClient)
}

func initTTSProvider(cfg *config.Config) tts.Provider {
	switch cfg.TTS.Provider {
	case "xtts":
		client := xtts.NewClient(xtts.Options{
			ServerURL: cfg.XTTS.ServerURL,
			Speaker:   cfg.XTTS.Speaker,
			Language:  cfg.XTTS.Language,
		})

		if !client.IsServerRunning() {
			slog.Info("Waiting for XTTS server to start...", "url", cfg.XTTS.ServerURL)
			if err := client.WaitForServer(5*time.Minute, 5*time.Second); err != nil {
				slog.Error("XTTS server not available", "error", err)
				os.Exit(1)
			}
		}

		slog.Info("Using XTTS provider",
			"url", cfg.XTTS.ServerURL,
			"speaker", cfg.XTTS.Speaker,
			"language", cfg.XTTS.Language)
		return client
	default:
		return newElevenLabsProvider(cfg)
	}
}

func newElevenLabsProvider(cfg *config.Config) tts.Provider {
	elClient := elevenlabs.NewClient(cfg.ElevenLabsAPIKey, elevenlabs.Options{
		VoiceID:    cfg.ElevenLabs.VoiceID,
		Model:      cfg.ElevenLabs.Model,
		Stability:  cfg.ElevenLabs.Stability,
		Similarity: cfg.ElevenLabs.Similarity,
	})
	slog.Info("Using ElevenLabs provider", "voice", cfg.ElevenLabs.VoiceID)
	return tts.NewElevenLabsAdapter(elClient)
}

func printUsage() {
	fmt.Println("Usage: craftstory <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  generate  Generate a video from a topic")
	fmt.Println("  upload    Upload a video to YouTube")
	fmt.Println("  auth      Authenticate with YouTube")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  craftstory generate -topic \"weird history fact\"")
	fmt.Println("  craftstory generate -topic \"space theory\" -upload")
	fmt.Println("  craftstory generate -topic \"fun fact\" -provider xtts")
	fmt.Println("  craftstory upload -video ./output/video.mp4 -title \"My Video\"")
	fmt.Println("  craftstory auth")
}
