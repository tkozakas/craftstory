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

	"craftstory/internal/storage"
	"craftstory/internal/tts"
)

const (
	defaultFFmpegPath = "ffmpeg"
	defaultFFprobe    = "ffprobe"
	videoEndBuffer    = 1.5
)

type Assembler struct {
	ffmpegPath    string
	ffprobe       string
	outputDir     string
	width         int
	height        int
	subtitleGen   *SubtitleGenerator
	bgProvider    storage.BackgroundProvider
	musicDir      string
	musicVolume   float64
	musicFadeIn   float64
	musicFadeOut  float64
	introPath     string
	outroPath     string
	introDuration float64
	outroDuration float64
}

type AssemblerOptions struct {
	OutputDir     string
	Resolution    string
	SubtitleGen   *SubtitleGenerator
	BgProvider    storage.BackgroundProvider
	MusicDir      string
	MusicVolume   float64
	MusicFadeIn   float64
	MusicFadeOut  float64
	IntroPath     string
	OutroPath     string
	IntroDuration float64
	OutroDuration float64
}

type ImageOverlay struct {
	ImagePath string
	StartTime float64
	EndTime   float64
	Width     int
	Height    int
}

type CharacterOverlay struct {
	Speaker    string
	AvatarPath string
	StartTime  float64
	EndTime    float64
	Position   int
}

type AssembleRequest struct {
	AudioPath         string
	AudioDuration     float64
	Script            string
	OutputPath        string
	WordTimings       []tts.WordTiming
	ImageOverlays     []ImageOverlay
	CharacterOverlays []CharacterOverlay
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
	musicVolume := opts.MusicVolume
	if musicVolume == 0 {
		musicVolume = 0.15
	}
	musicFadeIn := opts.MusicFadeIn
	if musicFadeIn == 0 {
		musicFadeIn = 1.0
	}
	musicFadeOut := opts.MusicFadeOut
	if musicFadeOut == 0 {
		musicFadeOut = 2.0
	}
	return &Assembler{
		ffmpegPath:    defaultFFmpegPath,
		ffprobe:       defaultFFprobe,
		outputDir:     opts.OutputDir,
		width:         width,
		height:        height,
		subtitleGen:   opts.SubtitleGen,
		bgProvider:    opts.BgProvider,
		musicDir:      opts.MusicDir,
		musicVolume:   musicVolume,
		musicFadeIn:   musicFadeIn,
		musicFadeOut:  musicFadeOut,
		introPath:     opts.IntroPath,
		outroPath:     opts.OutroPath,
		introDuration: opts.IntroDuration,
		outroDuration: opts.OutroDuration,
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

	outputPath := req.OutputPath
	if outputPath == "" {
		outputPath = filepath.Join(a.outputDir, fmt.Sprintf("video_%d.mp4", time.Now().Unix()))
	}

	outputDir := filepath.Dir(outputPath)
	assPath := filepath.Join(outputDir, fmt.Sprintf("subs_%d.ass", time.Now().UnixNano()))
	if err := os.WriteFile(assPath, []byte(assContent), 0644); err != nil {
		return nil, fmt.Errorf("failed to write subtitle file: %w", err)
	}
	defer func() { _ = os.Remove(assPath) }()

	musicPath := a.selectMusicTrack()
	filterComplex := a.buildFilterComplex(assPath, req.ImageOverlays, req.CharacterOverlays, musicPath, req.AudioDuration)

	mainVideoPath := outputPath
	needsConcat := a.introPath != "" || a.outroPath != ""
	if needsConcat {
		mainVideoPath = filepath.Join(outputDir, fmt.Sprintf("main_%d.mp4", time.Now().UnixNano()))
		defer func() { _ = os.Remove(mainVideoPath) }()
	}

	args := a.buildFFmpegArgs(backgroundClip, req.AudioPath, musicPath, startTime, req.AudioDuration, filterComplex, req.CharacterOverlays, req.ImageOverlays, mainVideoPath)

	cmd := exec.CommandContext(ctx, a.ffmpegPath, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %w, output: %s", err, string(output))
	}

	totalDuration := req.AudioDuration

	if needsConcat {
		introDur, outroDur, err := a.concatIntroOutro(ctx, mainVideoPath, outputPath)
		if err != nil {
			return nil, fmt.Errorf("failed to concat intro/outro: %w", err)
		}
		totalDuration += introDur + outroDur
	}

	return &AssembleResult{
		OutputPath: outputPath,
		Duration:   totalDuration,
	}, nil
}

func (a *Assembler) buildFilterComplex(assPath string, overlays []ImageOverlay, charOverlays []CharacterOverlay, musicPath string, duration float64) string {
	scaleFilter := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d",
		a.width, a.height, a.width, a.height)

	audioFilter := a.buildAudioFilter(musicPath, duration)

	totalOverlays := len(overlays) + len(charOverlays)
	if totalOverlays == 0 {
		return fmt.Sprintf("[0:v]%s,ass=%s[v];%s",
			scaleFilter, assPath, audioFilter)
	}

	var filters []string
	filters = append(filters, fmt.Sprintf("[0:v]%s,ass=%s[base]", scaleFilter, assPath))

	lastOutput := "base"
	inputOffset := a.getInputOffset(musicPath)

	avatarSize := a.width / 5
	bottomMargin := 200

	for i, charOverlay := range charOverlays {
		inputIdx := inputOffset + i
		scaledName := fmt.Sprintf("char%d", i)
		outputName := fmt.Sprintf("c%d", i)

		filters = append(filters, fmt.Sprintf(
			"[%d:v]scale=%d:-1,format=rgba[%s]",
			inputIdx, avatarSize, scaledName,
		))

		var x string
		if charOverlay.Position == 0 {
			x = "50"
		} else {
			x = "W-w-50"
		}
		y := fmt.Sprintf("H-%d-h", bottomMargin)

		filters = append(filters, fmt.Sprintf(
			"[%s][%s]overlay=%s:%s:enable='between(t,%.2f,%.2f)'[%s]",
			lastOutput, scaledName, x, y, charOverlay.StartTime, charOverlay.EndTime, outputName,
		))

		lastOutput = outputName
	}

	for i, overlay := range overlays {
		inputIdx := inputOffset + len(charOverlays) + i
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
	filters = append(filters, audioFilter)

	return strings.Join(filters, ";")
}

func (a *Assembler) getInputOffset(musicPath string) int {
	if musicPath != "" {
		return 3
	}
	return 2
}

func (a *Assembler) buildFFmpegArgs(bgClip, audioPath, musicPath string, startTime, duration float64, filterComplex string, charOverlays []CharacterOverlay, overlays []ImageOverlay, outputPath string) []string {
	videoDuration := duration + videoEndBuffer

	args := []string{
		"-y",
		"-ss", fmt.Sprintf("%.2f", startTime),
		"-t", fmt.Sprintf("%.2f", videoDuration),
		"-i", bgClip,
		"-i", audioPath,
	}

	if musicPath != "" {
		args = append(args, "-i", musicPath)
	}

	for _, charOverlay := range charOverlays {
		args = append(args, "-i", charOverlay.AvatarPath)
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

	var dur float64
	if _, err := fmt.Sscanf(string(output), "%f", &dur); err != nil {
		return 0, fmt.Errorf("failed to parse duration: %w", err)
	}

	return dur, nil
}

func (a *Assembler) randomStartTime(clipDuration, neededDuration float64) float64 {
	if clipDuration <= neededDuration {
		return 0
	}

	maxStart := clipDuration - neededDuration
	return rand.Float64() * maxStart
}

func (a *Assembler) selectMusicTrack() string {
	if a.musicDir == "" {
		return ""
	}

	entries, err := os.ReadDir(a.musicDir)
	if err != nil {
		return ""
	}

	var musicFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if strings.HasSuffix(name, ".mp3") || strings.HasSuffix(name, ".wav") || strings.HasSuffix(name, ".m4a") {
			musicFiles = append(musicFiles, filepath.Join(a.musicDir, entry.Name()))
		}
	}

	if len(musicFiles) == 0 {
		return ""
	}

	return musicFiles[rand.Intn(len(musicFiles))]
}

func (a *Assembler) buildAudioFilter(musicPath string, duration float64) string {
	if musicPath == "" {
		return "[0:a]volume=0.1[bga];[1:a]volume=1.0[voice];[bga][voice]amix=inputs=2:duration=longest[a]"
	}

	fadeOutStart := duration - a.musicFadeOut
	if fadeOutStart < 0 {
		fadeOutStart = 0
	}

	return fmt.Sprintf(
		"[0:a]volume=0.1[bga];"+
			"[1:a]volume=1.0[voice];"+
			"[2:a]volume=%.2f,afade=t=in:st=0:d=%.2f,afade=t=out:st=%.2f:d=%.2f[music];"+
			"[bga][voice][music]amix=inputs=3:duration=longest:normalize=0[a]",
		a.musicVolume, a.musicFadeIn, fadeOutStart, a.musicFadeOut,
	)
}

func (a *Assembler) concatIntroOutro(ctx context.Context, mainVideoPath, outputPath string) (float64, float64, error) {
	outputDir := filepath.Dir(outputPath)
	var clips []string
	var introDur, outroDur float64

	if a.introPath != "" {
		if _, err := os.Stat(a.introPath); err == nil {
			introClip, dur, err := a.prepareClip(ctx, a.introPath, a.introDuration, outputDir, "intro")
			if err != nil {
				return 0, 0, fmt.Errorf("failed to prepare intro: %w", err)
			}
			clips = append(clips, introClip)
			introDur = dur
			if introClip != a.introPath {
				defer func() { _ = os.Remove(introClip) }()
			}
		}
	}

	clips = append(clips, mainVideoPath)

	if a.outroPath != "" {
		if _, err := os.Stat(a.outroPath); err == nil {
			outroClip, dur, err := a.prepareClip(ctx, a.outroPath, a.outroDuration, outputDir, "outro")
			if err != nil {
				return 0, 0, fmt.Errorf("failed to prepare outro: %w", err)
			}
			clips = append(clips, outroClip)
			outroDur = dur
			if outroClip != a.outroPath {
				defer func() { _ = os.Remove(outroClip) }()
			}
		}
	}

	if len(clips) == 1 {
		return 0, 0, nil
	}

	listPath := filepath.Join(outputDir, fmt.Sprintf("concat_%d.txt", time.Now().UnixNano()))
	var listContent string
	for _, clip := range clips {
		absPath, err := filepath.Abs(clip)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to get absolute path: %w", err)
		}
		listContent += fmt.Sprintf("file '%s'\n", absPath)
	}
	if err := os.WriteFile(listPath, []byte(listContent), 0644); err != nil {
		return 0, 0, fmt.Errorf("failed to write concat list: %w", err)
	}
	defer func() { _ = os.Remove(listPath) }()

	args := []string{
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", listPath,
		"-c", "copy",
		outputPath,
	}

	cmd := exec.CommandContext(ctx, a.ffmpegPath, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return 0, 0, fmt.Errorf("ffmpeg concat failed: %w, output: %s", err, string(output))
	}

	return introDur, outroDur, nil
}

func (a *Assembler) prepareClip(ctx context.Context, clipPath string, maxDuration float64, outputDir, prefix string) (string, float64, error) {
	dur, err := a.getVideoDuration(ctx, clipPath)
	if err != nil {
		return "", 0, err
	}

	targetDuration := dur
	if maxDuration > 0 && dur > maxDuration {
		targetDuration = maxDuration
	}

	outputClip := filepath.Join(outputDir, fmt.Sprintf("%s_%d.mp4", prefix, time.Now().UnixNano()))

	args := []string{
		"-y",
		"-i", clipPath,
		"-t", fmt.Sprintf("%.2f", targetDuration),
		"-vf", fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d", a.width, a.height, a.width, a.height),
		"-c:v", "libx264",
		"-c:a", "aac",
		"-ar", "44100",
		"-preset", "fast",
		outputClip,
	}

	cmd := exec.CommandContext(ctx, a.ffmpegPath, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", 0, fmt.Errorf("ffmpeg re-encode failed: %w, output: %s", err, string(output))
	}

	return outputClip, targetDuration, nil
}
