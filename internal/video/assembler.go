package video

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"craftstory/internal/elevenlabs"
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
	width       int
	height      int
	subtitleGen *SubtitleGenerator
	bgProvider  storage.BackgroundProvider
}

type AssemblerOptions struct {
	OutputDir   string
	Resolution  string
	SubtitleGen *SubtitleGenerator
	BgProvider  storage.BackgroundProvider
}

type ImageOverlay struct {
	ImagePath string
	StartTime float64
	EndTime   float64
	Width     int
	Height    int
}

type AssembleRequest struct {
	AudioPath     string
	AudioDuration float64
	Script        string
	ScriptID      int64
	WordTimings   []elevenlabs.WordTiming
	ImageOverlays []ImageOverlay
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
		width:       1080,
		height:      1920,
		subtitleGen: subtitleGen,
		bgProvider:  bgProvider,
	}
}

func NewAssemblerWithOptions(opts AssemblerOptions) *Assembler {
	width, height := parseResolution(opts.Resolution)
	return &Assembler{
		ffmpegPath:  defaultFFmpegPath,
		ffprobe:     defaultFFprobe,
		outputDir:   opts.OutputDir,
		width:       width,
		height:      height,
		subtitleGen: opts.SubtitleGen,
		bgProvider:  opts.BgProvider,
	}
}

func parseResolution(res string) (int, int) {
	parts := strings.Split(res, "x")
	if len(parts) != 2 {
		return 1080, 1920
	}
	w, err1 := strconv.Atoi(parts[0])
	h, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 1080, 1920
	}
	return w, h
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

	var subtitles []Subtitle
	if len(req.WordTimings) > 0 {
		subtitles = a.subtitleGen.GenerateFromTimings(req.WordTimings)
	} else {
		subtitles = a.subtitleGen.Generate(req.Script, req.AudioDuration)
	}
	assContent := a.subtitleGen.ToASS(subtitles)

	assPath := filepath.Join(a.outputDir, fmt.Sprintf("subs_%d.ass", req.ScriptID))
	if err := os.WriteFile(assPath, []byte(assContent), 0644); err != nil {
		return nil, fmt.Errorf("failed to write subtitle file: %w", err)
	}
	defer func() { _ = os.Remove(assPath) }()

	outputPath := filepath.Join(a.outputDir, fmt.Sprintf("video_%d_%d.mp4", req.ScriptID, time.Now().Unix()))

	filterComplex := a.buildFilterComplex(assPath, req.ImageOverlays)

	args := a.buildFFmpegArgs(backgroundClip, req.AudioPath, startTime, req.AudioDuration, filterComplex, req.ImageOverlays, outputPath)

	cmd := exec.CommandContext(ctx, a.ffmpegPath, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %w, output: %s", err, string(output))
	}

	return &AssembleResult{
		OutputPath: outputPath,
		Duration:   req.AudioDuration,
	}, nil
}

func (a *Assembler) buildFilterComplex(assPath string, overlays []ImageOverlay) string {
	scaleFilter := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d",
		a.width, a.height, a.width, a.height)

	if len(overlays) == 0 {
		return fmt.Sprintf("[0:v]%s,ass=%s[v];[0:a]volume=0.1[bga];[1:a]volume=1.0[voice];[bga][voice]amix=inputs=2:duration=first[a]",
			scaleFilter, assPath)
	}

	var filters []string

	filters = append(filters, fmt.Sprintf("[0:v]%s,ass=%s[base]", scaleFilter, assPath))

	lastOutput := "base"
	for i, overlay := range overlays {
		inputIdx := i + 2
		scaledName := fmt.Sprintf("img%d", i)
		outputName := fmt.Sprintf("v%d", i)

		filters = append(filters, fmt.Sprintf(
			"[%d:v]scale=%d:%d,format=rgba[%s]",
			inputIdx, overlay.Width, overlay.Height, scaledName,
		))

		x := "(W-w)/2"
		y := "100"

		filters = append(filters, fmt.Sprintf(
			"[%s][%s]overlay=%s:%s:enable='between(t,%.2f,%.2f)'[%s]",
			lastOutput, scaledName, x, y, overlay.StartTime, overlay.EndTime, outputName,
		))

		lastOutput = outputName
	}

	filters = append(filters, fmt.Sprintf("[%s]null[v]", lastOutput))
	filters = append(filters, "[0:a]volume=0.1[bga];[1:a]volume=1.0[voice];[bga][voice]amix=inputs=2:duration=first[a]")

	return strings.Join(filters, ";")
}

func (a *Assembler) buildFFmpegArgs(bgClip, audioPath string, startTime, duration float64, filterComplex string, overlays []ImageOverlay, outputPath string) []string {
	args := []string{
		"-y",
		"-ss", fmt.Sprintf("%.2f", startTime),
		"-t", fmt.Sprintf("%.2f", duration),
		"-i", bgClip,
		"-i", audioPath,
	}

	for _, overlay := range overlays {
		args = append(args, "-i", overlay.ImagePath)
	}

	args = append(args,
		"-filter_complex", filterComplex,
		"-map", "[v]",
		"-map", "[a]",
		"-c:v", "libx264",
		"-c:a", "aac",
		"-ar", "44100",
		"-preset", "fast",
		outputPath,
	)

	return args
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
