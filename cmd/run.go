package cmd

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"craftstory/internal/app"
	"craftstory/internal/distribution/telegram"
	"craftstory/pkg/config"

	"github.com/spf13/cobra"
)

var (
	runInterval time.Duration
	runUpload   bool
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Cron mode: generate from Reddit, queue for approval, repeat",
	Long: `Run in continuous mode, generating videos from Reddit posts at regular intervals.
Videos are queued for Telegram approval unless --upload is specified.`,
	RunE: runCron,
}

func init() {
	runCmd.Flags().DurationVarP(&runInterval, "interval", "i", 15*time.Minute, "Interval between generations")
	runCmd.Flags().BoolVarP(&runUpload, "upload", "u", false, "Upload directly instead of queueing for approval")
	rootCmd.AddCommand(runCmd)
}

func runCron(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	cfg, err := config.Load(ctx)
	if err != nil {
		return err
	}

	service, err := app.BuildService(cfg, verbose)
	if err != nil {
		return err
	}

	pipeline := app.NewPipeline(service)
	approval := service.Approval()

	if !runUpload && approval != nil {
		approval.StartBot()
		defer approval.StopBot()

		go handleApprovals(ctx, pipeline, approval)
		go handleGenerations(ctx, pipeline, approval)
	}

	slog.Info("Starting cron mode", "interval", runInterval, "approval", !runUpload && approval != nil)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	generate := func() {
		if approval != nil && approval.Queue().IsFull() {
			slog.Info("Queue is full, skipping generation")
			return
		}

		slog.Info("Generating video from Reddit...")
		genResult, err := pipeline.GenerateFromReddit(ctx)
		if err != nil {
			slog.Error("Generation failed", "error", err)
			return
		}

		slog.Info("Video generated", "title", genResult.Title, "path", genResult.VideoPath)

		if runUpload {
			resp, err := pipeline.Upload(ctx, app.UploadRequest{
				VideoPath:   genResult.VideoPath,
				Title:       genResult.Title,
				Description: genResult.ScriptContent,
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
				VideoPath:   genResult.VideoPath,
				PreviewPath: genResult.PreviewPath,
				Title:       genResult.Title,
				Script:      genResult.ScriptContent,
			})
			if err != nil {
				slog.Error("Failed to queue for approval", "error", err)
			}
		}
	}

	ticker := time.NewTicker(runInterval)
	defer ticker.Stop()

	generate()

	for {
		select {
		case <-sigChan:
			slog.Info("Shutting down...")
			return nil
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			generate()
		}
	}
}

func handleApprovals(ctx context.Context, pipeline *app.Pipeline, approval *telegram.ApprovalService) {
	for {
		result, video, err := approval.WaitForResult(ctx)
		if err != nil {
			return
		}

		if video == nil {
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
			approval.NotifyUploadFailed(video.Title, err, video)
			continue
		}

		slog.Info("Upload complete", "title", video.Title, "url", resp.URL)
		approval.NotifyUploadComplete(video.Title, resp.URL, video)

		if video.PreviewPath != "" {
			if err := os.Remove(video.PreviewPath); err != nil {
				slog.Warn("Failed to cleanup preview file", "path", video.PreviewPath, "error", err)
			} else {
				slog.Debug("Cleaned up preview file", "path", video.PreviewPath)
			}
		}
	}
}

func handleGenerations(ctx context.Context, pipeline *app.Pipeline, approval *telegram.ApprovalService) {
	for {
		req, err := approval.WaitForGenerationRequest(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			time.Sleep(time.Second)
			continue
		}

		slog.Info("Processing generation request", "topic", req.Topic, "from_reddit", req.FromReddit, "chat_id", req.ChatID)
		approval.NotifyGenerating(req.ChatID, req.Topic)

		var genResult *app.GenerateResult
		if req.FromReddit {
			genResult, err = pipeline.GenerateFromReddit(ctx)
		} else {
			genResult, err = pipeline.Generate(ctx, req.Topic)
		}

		if err != nil {
			slog.Error("Generation failed", "error", err)
			approval.NotifyGenerationFailed(req.ChatID, err.Error())
			approval.FailGeneration(req.ChatID)
			continue
		}

		slog.Info("Video generated", "title", genResult.Title, "path", genResult.VideoPath)
		approval.NotifyGenerationComplete(req.ChatID, genResult.VideoPath, genResult.PreviewPath, genResult.Title, genResult.ScriptContent)
		approval.CompleteGeneration(req.ChatID)
	}
}
