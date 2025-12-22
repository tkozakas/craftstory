package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"time"

	"craftstory/internal/app"
	"craftstory/internal/gemini"
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
	rand.New(rand.NewSource(time.Now().UnixNano()))

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "setup":
		handleSetup()
	case "generate":
		handleGenerate()
	case "reddit":
		handleReddit()
	case "review":
		handleReview()
	case "upload":
		handleUpload()
	case "auth":
		handleAuth()
	default:
		printUsage()
		os.Exit(1)
	}
}

func handleSetup() {
	slog.Info("Setting up Craftstory")

	if _, err := exec.LookPath("gcloud"); err != nil {
		slog.Error("gcloud CLI not found. Run: mise install")
		os.Exit(1)
	}

	if err := runCmd("gcloud", "auth", "print-access-token"); err != nil {
		slog.Info("Authenticating with Google Cloud")
		_ = runCmdInteractive("gcloud", "auth", "login")
		_ = runCmdInteractive("gcloud", "auth", "application-default", "login")
	}

	project := selectProject()
	updateConfigProject(project)
	_ = runCmd("gcloud", "config", "set", "project", project)

	slog.Info("Checking billing")
	for {
		out, _ := exec.Command("gcloud", "billing", "projects", "describe", project, "--format=value(billingEnabled)").Output()
		if strings.TrimSpace(string(out)) == "True" {
			break
		}
		url := "https://console.cloud.google.com/billing/linkedaccount?project=" + project
		slog.Warn("Billing not enabled", "url", url)
		openBrowser(url)
		fmt.Print("Press Enter after enabling billing...")
		_, _ = bufio.NewReader(os.Stdin).ReadBytes('\n')
	}

	slog.Info("Enabling APIs")
	apis := []string{
		"aiplatform.googleapis.com",
		"secretmanager.googleapis.com",
		"youtube.googleapis.com",
		"customsearch.googleapis.com",
	}
	for _, api := range apis {
		_ = runCmdInteractive("gcloud", "services", "enable", api)
	}

	if runCmd("gcloud", "secrets", "describe", "elevenlabs-api-key") != nil {
		url := "https://elevenlabs.io/app/settings/api-keys"
		slog.Info("Get your API key", "url", url)
		openBrowser(url)
		promptAndStoreSecret("elevenlabs-api-key", "ElevenLabs API key")
	}

	if runCmd("gcloud", "secrets", "describe", "google-search-api-key") != nil {
		slog.Info("Setting up Google Custom Search for image overlays")
		url := "https://console.cloud.google.com/apis/credentials"
		slog.Info("Create an API key", "url", url)
		openBrowser(url)
		promptAndStoreSecret("google-search-api-key", "Google API key")

		url = "https://programmablesearchengine.google.com/controlpanel/create"
		slog.Info("Create a Custom Search Engine (enable 'Search the entire web')", "url", url)
		openBrowser(url)
		promptAndStoreSecret("google-search-engine-id", "Search Engine ID (cx)")
	}

	slog.Info("Setup complete! Run: task run -- generate -topic \"your topic\"")
}

func selectProject() string {
	cmd := exec.Command("gcloud", "projects", "list", "--format=value(projectId)")
	output, err := cmd.Output()
	if err != nil {
		slog.Error("Failed to list projects", "error", err)
		os.Exit(1)
	}

	projects := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(projects) == 0 {
		slog.Error("No GCP projects found", "url", "https://console.cloud.google.com")
		os.Exit(1)
	}

	slog.Info("Select a project")
	for i, p := range projects {
		fmt.Printf("  %d. %s\n", i+1, p)
	}
	fmt.Print("Enter number: ")

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	var idx int
	_, _ = fmt.Sscanf(input, "%d", &idx)
	if idx < 1 || idx > len(projects) {
		idx = 1
	}

	return projects[idx-1]
}

func updateConfigProject(project string) {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return
	}

	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "project:") {
			indent := len(line) - len(strings.TrimLeft(line, " \t"))
			lines[i] = strings.Repeat(" ", indent) + "project: \"" + project + "\""
			break
		}
	}

	_ = os.WriteFile("config.yaml", []byte(strings.Join(lines, "\n")), 0644)
	slog.Info("Updated config.yaml", "project", project)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch {
	case commandExists("xdg-open"):
		cmd = exec.Command("xdg-open", url)
	case commandExists("open"):
		cmd = exec.Command("open", url)
	default:
		slog.Info("Open URL", "url", url)
		return
	}
	_ = cmd.Start()
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

func runCmdInteractive(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func promptAndStoreSecret(secretName, prompt string) {
	fmt.Printf("Paste %s here: ", prompt)
	reader := bufio.NewReader(os.Stdin)
	value, _ := reader.ReadString('\n')
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	cmd := exec.Command("gcloud", "secrets", "create", secretName, "--data-file=-")
	cmd.Stdin = strings.NewReader(value)
	_ = cmd.Run()
}

func loadConfig() *config.Config {
	ctx := context.Background()
	cfg, err := config.Load(ctx)
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}
	return cfg
}

func handleGenerate() {
	cfg := loadConfig()
	cmd := flag.NewFlagSet("generate", flag.ExitOnError)
	topic := cmd.String("topic", "", "Topic for the video script (required)")

	if err := cmd.Parse(os.Args[2:]); err != nil {
		slog.Error("Failed to parse flags", "error", err)
		os.Exit(1)
	}

	if *topic == "" {
		slog.Error("-topic is required")
		os.Exit(1)
	}

	svc := initService(cfg)
	pipeline := app.NewPipeline(svc)
	ctx := context.Background()

	result, err := pipeline.Generate(ctx, *topic)
	if err != nil {
		slog.Error("Failed to generate video", "error", err)
		os.Exit(1)
	}

	slog.Info("Video generated",
		"title", result.Title,
		"video", result.VideoPath,
		"duration", fmt.Sprintf("%.1fs", result.Duration))

	if svc.Approval() != nil {
		err := svc.Approval().QueueVideo(telegram.QueuedVideo{
			VideoPath: result.VideoPath,
			Title:     result.Title,
			Script:    result.ScriptContent,
			Topic:     *topic,
		})
		if err != nil {
			slog.Error("Failed to queue video", "error", err)
			os.Exit(1)
		}
		slog.Info("Video queued for review. Use /review in Telegram or run 'craftstory review' to process.")
		return
	}

	if !requestConsoleApproval(result.VideoPath) {
		slog.Info("Upload cancelled")
		return
	}

	uploadVideo(svc, pipeline, ctx, result, *topic)
}

func handleReddit() {
	cfg := loadConfig()
	svc := initService(cfg)
	pipeline := app.NewPipeline(svc)
	ctx := context.Background()

	result, err := pipeline.GenerateFromReddit(ctx)
	if err != nil {
		slog.Error("Failed to generate video from Reddit", "error", err)
		os.Exit(1)
	}

	slog.Info("Video generated",
		"title", result.Title,
		"video", result.VideoPath,
		"duration", fmt.Sprintf("%.1fs", result.Duration))

	if svc.Approval() != nil {
		err := svc.Approval().QueueVideo(telegram.QueuedVideo{
			VideoPath: result.VideoPath,
			Title:     result.Title,
			Script:    result.ScriptContent,
			Topic:     "reddit",
		})
		if err != nil {
			slog.Error("Failed to queue video", "error", err)
			os.Exit(1)
		}
		slog.Info("Video queued for review.")
		return
	}

	if !requestConsoleApproval(result.VideoPath) {
		slog.Info("Upload cancelled")
		return
	}

	uploadVideo(svc, pipeline, ctx, result, "reddit")
}

func handleReview() {
	cfg := loadConfig()
	svc := initService(cfg)
	pipeline := app.NewPipeline(svc)
	ctx := context.Background()

	if svc.Approval() == nil {
		slog.Error("Telegram not configured")
		os.Exit(1)
	}

	slog.Info("Waiting for review decision via Telegram...")
	slog.Info("Send /review in Telegram to get the next video")

	result, video, err := svc.Approval().WaitForResult(ctx)
	if err != nil {
		slog.Error("Failed to get review result", "error", err)
		os.Exit(1)
	}

	if !result.Approved {
		slog.Info("Video rejected", "title", video.Title)
		return
	}

	slog.Info("Video approved, uploading...", "title", video.Title)
	uploadVideoFromQueue(svc, pipeline, ctx, video)
}

func uploadVideo(svc *app.Service, pipeline *app.Pipeline, ctx context.Context, result *app.GenerateResult, topic string) {
	slog.Info("Uploading to YouTube...")
	resp, err := pipeline.Upload(ctx, app.UploadRequest{
		VideoPath:   result.VideoPath,
		Title:       result.Title + " #shorts",
		Description: fmt.Sprintf("A short video about %s\n\n#shorts #facts", topic),
	})
	if err != nil {
		slog.Error("Failed to upload video", "error", err)
		if svc.Approval() != nil {
			svc.Approval().NotifyUploadFailed(result.Title, err)
		}
		os.Exit(1)
	}

	slog.Info("Video uploaded", "url", resp.URL)
	if svc.Approval() != nil {
		svc.Approval().NotifyUploadComplete(result.Title, resp.URL)
	}
}

func uploadVideoFromQueue(svc *app.Service, pipeline *app.Pipeline, ctx context.Context, video *telegram.QueuedVideo) {
	slog.Info("Uploading to YouTube...")
	resp, err := pipeline.Upload(ctx, app.UploadRequest{
		VideoPath:   video.VideoPath,
		Title:       video.Title + " #shorts",
		Description: fmt.Sprintf("A short video about %s\n\n#shorts #facts", video.Topic),
	})
	if err != nil {
		slog.Error("Failed to upload video", "error", err)
		svc.Approval().NotifyUploadFailed(video.Title, err)
		os.Exit(1)
	}

	slog.Info("Video uploaded", "url", resp.URL)
	svc.Approval().NotifyUploadComplete(video.Title, resp.URL)
}

func requestConsoleApproval(videoPath string) bool {
	openFile(videoPath)
	fmt.Print("\nUpload to YouTube? [y/N]: ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes"
}

func openFile(path string) {
	var cmd *exec.Cmd
	switch {
	case commandExists("xdg-open"):
		cmd = exec.Command("xdg-open", path)
	case commandExists("open"):
		cmd = exec.Command("open", path)
	default:
		slog.Info("Open video to review", "path", path)
		return
	}
	_ = cmd.Start()
}

func handleUpload() {
	cfg := loadConfig()
	cmd := flag.NewFlagSet("upload", flag.ExitOnError)
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
	resp, err := pipeline.Upload(ctx, app.UploadRequest{
		VideoPath:   *videoPath,
		Title:       *title,
		Description: *description,
	})
	if err != nil {
		slog.Error("Failed to upload video", "error", err)
		os.Exit(1)
	}

	slog.Info("Video uploaded", "url", resp.URL)
}

func handleAuth() {
	cfg := loadConfig()
	cmd := flag.NewFlagSet("auth", flag.ExitOnError)
	if err := cmd.Parse(os.Args[2:]); err != nil {
		slog.Error("Failed to parse flags", "error", err)
		os.Exit(1)
	}

	auth := uploader.NewYouTubeAuth(cfg.YouTubeClientID, cfg.YouTubeClientSecret, cfg.YouTubeTokenPath)

	if auth.IsAuthenticated() {
		slog.Info("Already authenticated with YouTube")
		return
	}

	slog.Info("Visit URL to authenticate", "url", auth.GetAuthURL())
	fmt.Print("Enter authorization code: ")

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

	slog.Info("Authentication successful")
}

func initService(cfg *config.Config) *app.Service {
	ctx := context.Background()

	p, err := prompts.Load()
	if err != nil {
		slog.Error("Failed to load prompts.yaml", "error", err)
		os.Exit(1)
	}

	llmClient, err := gemini.NewClient(ctx, cfg.Gemini.Project, cfg.Gemini.Location, cfg.Gemini.Model, p)
	if err != nil {
		slog.Error("Failed to create Gemini client", "error", err)
		os.Exit(1)
	}

	var ttsService tts.Provider = tts.NewElevenLabsClient(cfg.ElevenLabsAPIKey, tts.ElevenLabsOptions{
		VoiceID:    cfg.ElevenLabs.VoiceID,
		Model:      cfg.ElevenLabs.Model,
		Stability:  cfg.ElevenLabs.Stability,
		Similarity: cfg.ElevenLabs.Similarity,
	})

	if cfg.FishAudio.Enabled && cfg.FishAudioAPIKey != "" {
		ttsService = tts.NewFishAudioClient(cfg.FishAudioAPIKey, tts.FishAudioOptions{
			VoiceID: cfg.FishAudio.VoiceID,
		})
		slog.Info("Using Fish Audio TTS")
	} else {
		slog.Info("Using ElevenLabs TTS")
	}

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
		Offset:       cfg.Subtitles.Offset,
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

	var approvalService *telegram.ApprovalService
	if cfg.TelegramBotToken != "" {
		client := telegram.NewClient(cfg.TelegramBotToken)
		approvalService = telegram.NewApprovalService(client, cfg.Video.OutputDir, cfg.Telegram.DefaultChatID)
		approvalService.StartBot()
		slog.Info("Telegram bot started")
	}

	return app.NewService(cfg, llmClient, ttsService, ytUploader, assembler, localStorage, redditClient, imgSearchClient, approvalService)
}

func printUsage() {
	fmt.Println(`Usage: craftstory <command>

Commands:
  setup        Setup GCP project and authenticate
  generate     Generate a video from a topic
  reddit       Generate a video from a random Reddit CS thread
  review       Wait for Telegram review and upload approved videos
  upload       Upload a video to YouTube directly
  auth         Authenticate with YouTube

Examples:
  craftstory setup
  craftstory generate -topic "weird history fact"
  craftstory reddit
  craftstory review`)
}
