package video

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"craftstory/internal/storage"
)

const (
	defaultFFmpegPath = "ffmpeg"
	defaultFFprobe    = "ffprobe"
)

type Assembler struct {
	ffmpegPath  string
	ffprobe     string
	outputDir   string
	subtitleGen *SubtitleGenerator
	bgProvider  storage.BackgroundProvider
}

type AssembleRequest struct {
	AudioPath     string
	AudioDuration float64
	Script        string
	ScriptID      int64
}

type AssembleResult struct {
	OutputPath string
	Duration   float64
}

func NewAssembler(outputDir string, subtitleGen *SubtitleGenerator, bgProvider storage.BackgroundProvider) *Assembler {
	return &Assembler{
		ffmpegPath:  defaultFFmpegPath,
		ffprobe:     defaultFFprobe,
		outputDir:   outputDir,
		subtitleGen: subtitleGen,
		bgProvider:  bgProvider,
	}
}

func (a *Assembler) Assemble(ctx context.Context, req AssembleRequest) (*AssembleResult, error) {
	backgroundClip, err := a.bgProvider.RandomBackgroundClip(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to select background clip: %w", err)
	}

	clipDuration, err := a.getVideoDuration(ctx, backgroundClip)
	if err != nil {
		return nil, fmt.Errorf("failed to get clip duration: %w", err)
	}

	startTime := a.randomStartTime(clipDuration, req.AudioDuration)

	subtitles := a.subtitleGen.Generate(req.Script, req.AudioDuration)
	assContent := a.subtitleGen.ToASS(subtitles)

	assPath := filepath.Join(a.outputDir, fmt.Sprintf("subs_%d.ass", req.ScriptID))
	if err := os.WriteFile(assPath, []byte(assContent), 0644); err != nil {
		return nil, fmt.Errorf("failed to write subtitle file: %w", err)
	}
	defer func() { _ = os.Remove(assPath) }()

	outputPath := filepath.Join(a.outputDir, fmt.Sprintf("video_%d_%d.mp4", req.ScriptID, time.Now().Unix()))

	args := []string{
		"-y",
		"-ss", fmt.Sprintf("%.2f", startTime),
		"-t", fmt.Sprintf("%.2f", req.AudioDuration),
		"-i", backgroundClip,
		"-i", req.AudioPath,
		"-filter_complex", fmt.Sprintf("[0:v]ass=%s[v];[0:a]volume=0.1[bga];[bga][1:a]amix=inputs=2:duration=shortest[a]", assPath),
		"-map", "[v]",
		"-map", "[a]",
		"-c:v", "libx264",
		"-c:a", "aac",
		"-ar", "44100",
		"-preset", "fast",
		outputPath,
	}

	cmd := exec.CommandContext(ctx, a.ffmpegPath, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %w, output: %s", err, string(output))
	}

	return &AssembleResult{
		OutputPath: outputPath,
		Duration:   req.AudioDuration,
	}, nil
}

func (a *Assembler) getVideoDuration(ctx context.Context, path string) (float64, error) {
	args := []string{
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	}

	cmd := exec.CommandContext(ctx, a.ffprobe, args...)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe failed: %w", err)
	}

	var duration float64
	if _, err := fmt.Sscanf(string(output), "%f", &duration); err != nil {
		return 0, fmt.Errorf("failed to parse duration: %w", err)
	}

	return duration, nil
}

func (a *Assembler) randomStartTime(clipDuration, neededDuration float64) float64 {
	if clipDuration <= neededDuration {
		return 0
	}

	maxStart := clipDuration - neededDuration
	return rand.Float64() * maxStart
}
