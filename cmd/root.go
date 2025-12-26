package cmd

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

var verbose bool

var rootCmd = &cobra.Command{
	Use:   "craftstory",
	Short: "Generate and publish video content",
	Long: `Craftstory generates video content from topics or Reddit posts,
with text-to-speech narration, background visuals, and optional YouTube upload.`,
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable debug logging")
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		setupLogger()
	}
}

func Execute() error {
	return rootCmd.Execute()
}

func setupLogger() {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
}
