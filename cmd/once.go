package cmd

import (
	"errors"
	"log/slog"

	"craftstory/internal/app"
	"craftstory/pkg/config"

	"github.com/spf13/cobra"
)

var (
	onceTopic     string
	onceUseReddit bool
	onceUpload    bool
)

var onceCmd = &cobra.Command{
	Use:   "once",
	Short: "Generate a single video",
	Long:  `Generate a single video from a topic or random Reddit post.`,
	RunE:  runOnce,
}

func init() {
	onceCmd.Flags().StringVarP(&onceTopic, "topic", "t", "", "Topic for video generation")
	onceCmd.Flags().BoolVarP(&onceUseReddit, "reddit", "r", false, "Generate video from Reddit topic")
	onceCmd.Flags().BoolVarP(&onceUpload, "upload", "u", false, "Upload to YouTube after generation")
	rootCmd.AddCommand(onceCmd)
}

func runOnce(cmd *cobra.Command, args []string) error {
	if onceTopic == "" && !onceUseReddit {
		return errors.New("please provide --topic or --reddit")
	}

	ctx := cmd.Context()

	cfg, err := config.Load(ctx)
	if err != nil {
		return err
	}

	result, err := app.BuildService(cfg, verbose)
	if err != nil {
		return err
	}

	pipeline := app.NewPipeline(result.Service)

	var genResult *app.GenerateResult
	if onceUseReddit {
		slog.Info("Generating video from Reddit...")
		genResult, err = pipeline.GenerateFromReddit(ctx)
	} else {
		slog.Info("Generating video...", "topic", onceTopic)
		genResult, err = pipeline.Generate(ctx, onceTopic)
	}

	if err != nil {
		return err
	}

	slog.Info("Video generated",
		"title", genResult.Title,
		"path", genResult.VideoPath,
		"duration", genResult.Duration,
	)

	if onceUpload {
		slog.Info("Uploading to YouTube...")
		resp, err := pipeline.Upload(ctx, app.UploadRequest{
			VideoPath:   genResult.VideoPath,
			Title:       genResult.Title,
			Description: genResult.ScriptContent,
		})
		if err != nil {
			return err
		}
		slog.Info("Upload complete", "url", resp.URL)
	}

	return nil
}
