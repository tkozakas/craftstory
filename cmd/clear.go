package cmd

import (
	"fmt"

	"craftstory/internal/distribution/telegram"
	"craftstory/pkg/config"

	"github.com/spf13/cobra"
)

var clearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear the video queue",
	Long:  `Remove all videos from the approval queue.`,
	RunE:  runClear,
}

func init() {
	rootCmd.AddCommand(clearCmd)
}

func runClear(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cmd.Context())
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	queue := telegram.NewVideoQueue(cfg.Video.OutputDir)
	count := queue.Len()
	queue.Clear()

	fmt.Printf("Cleared %d video(s) from queue\n", count)
	return nil
}
