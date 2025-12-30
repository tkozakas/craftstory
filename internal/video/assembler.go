package video

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"craftstory/internal/speech"
	"craftstory/internal/storage"
)

const (
	ffmpegBin      = "ffmpeg"
	ffprobeBin     = "ffprobe"
	videoEndBuffer = 1.5
	defaultWidth   = 1080
	defaultHeight  = 1920
	maxOverlays    = 6
)

type Assembler struct {
	ffmpeg      string
	ffprobe     string
	outputDir   string
	width       int
	height      int
	subtitleGen *SubtitleGenerator
	bgProvider  storage.BackgroundProvider
	music       musicConfig
	intro       clipConfig
	outro       clipConfig
	verbose     bool
}

type musicConfig struct {
	dir     string
	volume  float64
	fadeIn  float64
	fadeOut float64
}

type clipConfig struct {
	path     string
	duration float64
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
	Verbose       bool
}

type ImageOverlay struct {
	ImagePath string
	StartTime float64
	EndTime   float64
	Width     int
	Height    int
	IsGif     bool
}

type AssembleRequest struct {
	AudioPath     string
	AudioDuration float64
	Script        string
	OutputPath    string
	WordTimings   []speech.WordTiming
	ImageOverlays []ImageOverlay
	SpeakerColors map[string]string
}

type AssembleResult struct {
	OutputPath string
	Duration   float64
}

type encoder struct {
	name         string
	args         []string
	inputArgs    []string
	filterSuffix string
	test         func() bool
}

var (
	encoderOnce   sync.Once
	encoderCached encoder
)

var encoders = []encoder{
	{
		name:      "nvenc",
		args:      []string{"-c:v", "h264_nvenc", "-preset", "p1", "-tune", "ll", "-rc", "vbr", "-cq", "30", "-pix_fmt", "yuv420p"},
		inputArgs: nil,
		test:      func() bool { return testEnc("h264_nvenc") },
	},
	{
		name:         "vaapi",
		args:         []string{"-c:v", "h264_vaapi", "-qp", "28"},
		inputArgs:    []string{"-vaapi_device", "/dev/dri/renderD128"},
		filterSuffix: ",format=nv12,hwupload",
		test:         testVAAPI,
	},
	{
		name: "v4l2m2m",
		args: []string{"-c:v", "h264_v4l2m2m", "-b:v", "2M", "-pix_fmt", "yuv420p"},
		test: func() bool { return testEnc("h264_v4l2m2m") },
	},
	{
		name: "omx",
		args: []string{"-c:v", "h264_omx", "-b:v", "2M", "-pix_fmt", "yuv420p"},
		test: func() bool { return testEnc("h264_omx") },
	},
}

var softwareEncoder = encoder{
	name: "libx264",
	args: []string{"-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency", "-threads", "0", "-crf", "28", "-pix_fmt", "yuv420p"},
}

func NewAssembler(outputDir string, subtitleGen *SubtitleGenerator, bgProvider storage.BackgroundProvider) *Assembler {
	return &Assembler{
		ffmpeg:      ffmpegBin,
		ffprobe:     ffprobeBin,
		outputDir:   outputDir,
		width:       defaultWidth,
		height:      defaultHeight,
		subtitleGen: subtitleGen,
		bgProvider:  bgProvider,
	}
}

func NewAssemblerWithOptions(opts AssemblerOptions) *Assembler {
	w, h := parseResolution(opts.Resolution)
	return &Assembler{
		ffmpeg:      ffmpegBin,
		ffprobe:     ffprobeBin,
		outputDir:   opts.OutputDir,
		width:       w,
		height:      h,
		subtitleGen: opts.SubtitleGen,
		bgProvider:  opts.BgProvider,
		music: musicConfig{
			dir:     opts.MusicDir,
			volume:  orDefault(opts.MusicVolume, 0.15),
			fadeIn:  orDefault(opts.MusicFadeIn, 1.0),
			fadeOut: orDefault(opts.MusicFadeOut, 2.0),
		},
		intro:   clipConfig{path: opts.IntroPath, duration: opts.IntroDuration},
		outro:   clipConfig{path: opts.OutroPath, duration: opts.OutroDuration},
		verbose: opts.Verbose,
	}
}

func (a *Assembler) log(msg string, args ...any) {
	if !a.verbose {
		return
	}
	slog.Debug(msg, args...)
}

func (a *Assembler) Assemble(ctx context.Context, req AssembleRequest) (*AssembleResult, error) {
	a.log("selecting background clip")
	bgClip, err := a.bgProvider.RandomBackgroundClip(ctx)
	if err != nil {
		return nil, fmt.Errorf("select background: %w", err)
	}
	a.log("selected background", "clip", bgClip)

	clipDur, err := a.videoDuration(ctx, bgClip)
	if err != nil {
		return nil, fmt.Errorf("get clip duration: %w", err)
	}
	a.log("clip duration", "seconds", clipDur)

	startTime := randomStart(clipDur, req.AudioDuration)
	a.log("random start time", "seconds", startTime)

	a.log("generating subtitles")
	subtitles := a.generateSubtitles(req)
	a.log("generated subtitles", "count", len(subtitles))

	assPath, cleanup, err := a.writeSubtitleFile(req.OutputPath, subtitles)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	a.log("wrote subtitle file", "path", assPath)

	outputPath := a.resolveOutputPath(req.OutputPath)
	musicPath := a.selectMusicTrack()
	a.log("selected music", "path", musicPath)

	a.log("building filter complex")
	filterComplex := a.buildFilterComplex(assPath, req.ImageOverlays, musicPath, req.AudioDuration)
	a.log("filter complex", "filter", filterComplex)

	mainPath, cleanupMain := a.prepareMainPath(outputPath)
	defer cleanupMain()

	a.log("building ffmpeg args")
	args := a.buildFFmpegArgs(bgClip, req.AudioPath, musicPath, startTime, req.AudioDuration, filterComplex, req.ImageOverlays, mainPath)
	a.log("ffmpeg command", "args", strings.Join(args, " "))

	a.log("running ffmpeg", "output", mainPath)
	if err := a.runFFmpeg(ctx, args); err != nil {
		return nil, err
	}
	a.log("ffmpeg completed")

	totalDur := req.AudioDuration
	if a.hasIntroOutro() {
		a.log("concatenating intro/outro")
		introDur, outroDur, err := a.concatIntroOutro(ctx, mainPath, outputPath)
		if err != nil {
			return nil, fmt.Errorf("concat intro/outro: %w", err)
		}
		totalDur += introDur + outroDur
		a.log("concat completed", "introDur", introDur, "outroDur", outroDur)
	}

	a.log("assembly completed", "output", outputPath, "duration", totalDur)
	return &AssembleResult{OutputPath: outputPath, Duration: totalDur}, nil
}

func (a *Assembler) generateSubtitles(req AssembleRequest) []Subtitle {
	if len(req.WordTimings) > 0 {
		return a.subtitleGen.GenerateFromTimingsWithColors(req.WordTimings, req.SpeakerColors)
	}
	return a.subtitleGen.Generate(req.Script, req.AudioDuration)
}

func (a *Assembler) writeSubtitleFile(outputPath string, subs []Subtitle) (string, func(), error) {
	dir := filepath.Dir(a.resolveOutputPath(outputPath))
	path := filepath.Join(dir, fmt.Sprintf("subs_%d.ass", time.Now().UnixNano()))
	content := a.subtitleGen.ToASS(subs)

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", func() {}, fmt.Errorf("write subtitle file: %w", err)
	}
	return path, func() { _ = os.Remove(path) }, nil
}

func (a *Assembler) resolveOutputPath(path string) string {
	if path != "" {
		return path
	}
	return filepath.Join(a.outputDir, fmt.Sprintf("video_%d.mp4", time.Now().Unix()))
}

func (a *Assembler) hasIntroOutro() bool {
	return a.intro.path != "" || a.outro.path != ""
}

func (a *Assembler) prepareMainPath(outputPath string) (string, func()) {
	if !a.hasIntroOutro() {
		return outputPath, func() {}
	}
	mainPath := filepath.Join(filepath.Dir(outputPath), fmt.Sprintf("main_%d.mp4", time.Now().UnixNano()))
	return mainPath, func() { _ = os.Remove(mainPath) }
}

func (a *Assembler) buildFilterComplex(assPath string, overlays []ImageOverlay, musicPath string, duration float64) string {
	scale := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d", a.width, a.height, a.width, a.height)
	audio := a.buildAudioFilter(musicPath, duration)

	hwSuffix := ""
	if len(overlays) == 0 {
		hwSuffix = getEncoder().filterSuffix
		return fmt.Sprintf("[0:v]%s,ass=%s%s[v];%s", scale, assPath, hwSuffix, audio)
	}

	if len(overlays) > maxOverlays {
		slog.Info("Limiting overlays", "from", len(overlays), "to", maxOverlays)
		overlays = overlays[:maxOverlays]
	}

	inputOffset := 2
	if musicPath != "" {
		inputOffset = 3
	}

	slog.Info("Building overlay filters", "overlay_count", len(overlays), "input_offset", inputOffset)

	filters := []string{fmt.Sprintf("[0:v]%s,ass=%s[base]", scale, assPath)}
	lastOut := "base"

	for i, ov := range overlays {
		img := fmt.Sprintf("img%d", i)
		out := fmt.Sprintf("v%d", i)

		inputIdx := inputOffset + i
		scaleFilter := fmt.Sprintf("[%d:v]scale=%d:%d,format=rgba[%s]", inputIdx, ov.Width, ov.Height, img)
		overlayFilter := fmt.Sprintf("[%s][%s]overlay=(W-w)/2:100:enable='between(t,%.2f,%.2f)'[%s]", lastOut, img, ov.StartTime, ov.EndTime, out)

		slog.Info("Overlay filter",
			"index", i,
			"input", inputIdx,
			"path", ov.ImagePath,
			"start", ov.StartTime,
			"end", ov.EndTime,
			"is_gif", ov.IsGif,
		)

		filters = append(filters, scaleFilter)
		filters = append(filters, overlayFilter)
		lastOut = out
	}

	filters = append(filters, fmt.Sprintf("[%s]null[v]", lastOut))
	filters = append(filters, audio)
	return strings.Join(filters, ";")
}

func (a *Assembler) buildAudioFilter(musicPath string, duration float64) string {
	if musicPath == "" {
		return "[0:a]volume=0.1[bga];[1:a]volume=1.0[voice];[bga][voice]amix=inputs=2:duration=longest[a]"
	}

	fadeOut := max(duration-a.music.fadeOut, 0)
	return fmt.Sprintf(
		"[0:a]volume=0.1[bga];[1:a]volume=1.0[voice];[2:a]volume=%.2f,afade=t=in:st=0:d=%.2f,afade=t=out:st=%.2f:d=%.2f[music];[bga][voice][music]amix=inputs=3:duration=longest:normalize=0[a]",
		a.music.volume, a.music.fadeIn, fadeOut, a.music.fadeOut,
	)
}

func (a *Assembler) buildFFmpegArgs(bgClip, audioPath, musicPath string, startTime, duration float64, filterComplex string, overlays []ImageOverlay, outputPath string) []string {
	enc := getEncoder()
	if len(overlays) > 0 {
		enc = softwareEncoder
	}
	videoDur := duration + videoEndBuffer

	args := []string{"-y", "-threads", "0"}
	args = append(args, enc.inputArgs...)
	args = append(args, "-ss", fmt.Sprintf("%.2f", startTime), "-t", fmt.Sprintf("%.2f", videoDur), "-i", bgClip, "-i", audioPath)

	if musicPath != "" {
		args = append(args, "-i", musicPath)
	}

	for _, ov := range overlays {
		displayDuration := ov.EndTime - ov.StartTime + 0.5
		if ov.IsGif {
			args = append(args, "-t", fmt.Sprintf("%.2f", displayDuration), "-i", ov.ImagePath)
		} else {
			args = append(args, "-loop", "1", "-t", fmt.Sprintf("%.2f", displayDuration), "-i", ov.ImagePath)
		}
	}

	args = append(args, "-filter_complex", filterComplex, "-map", "[v]", "-map", "[a]")
	args = append(args, enc.args...)
	args = append(args, "-c:a", "aac", "-b:a", "96k", "-ar", "44100", "-movflags", "+faststart", outputPath)
	return args
}

func (a *Assembler) runFFmpeg(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, a.ffmpeg, args...)

	if a.verbose {
		cmd.Stderr = os.Stderr
	}

	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("ffmpeg: %w, output: %s", err, out)
	}
	return nil
}

func (a *Assembler) selectMusicTrack() string {
	if a.music.dir == "" {
		return ""
	}

	entries, err := os.ReadDir(a.music.dir)
	if err != nil {
		return ""
	}

	var tracks []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(e.Name())
		if strings.HasSuffix(name, ".mp3") || strings.HasSuffix(name, ".wav") || strings.HasSuffix(name, ".m4a") {
			tracks = append(tracks, filepath.Join(a.music.dir, e.Name()))
		}
	}

	if len(tracks) == 0 {
		return ""
	}
	return tracks[rand.Intn(len(tracks))]
}

func (a *Assembler) videoDuration(ctx context.Context, path string) (float64, error) {
	cmd := exec.CommandContext(ctx, a.ffprobe, "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path)
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe: %w", err)
	}

	var dur float64
	if _, err := fmt.Sscanf(string(out), "%f", &dur); err != nil {
		return 0, fmt.Errorf("parse duration: %w", err)
	}
	return dur, nil
}

func (a *Assembler) concatIntroOutro(ctx context.Context, mainPath, outputPath string) (float64, float64, error) {
	dir := filepath.Dir(outputPath)
	var clips []string
	var introDur, outroDur float64

	if clip, dur, err := a.prepareClip(ctx, a.intro, dir, "intro"); err == nil && clip != "" {
		clips = append(clips, clip)
		introDur = dur
		defer func() { _ = os.Remove(clip) }()
	}

	clips = append(clips, mainPath)

	if clip, dur, err := a.prepareClip(ctx, a.outro, dir, "outro"); err == nil && clip != "" {
		clips = append(clips, clip)
		outroDur = dur
		defer func() { _ = os.Remove(clip) }()
	}

	if len(clips) == 1 {
		return 0, 0, nil
	}

	listPath := filepath.Join(dir, fmt.Sprintf("concat_%d.txt", time.Now().UnixNano()))
	defer func() { _ = os.Remove(listPath) }()

	var content strings.Builder
	for _, c := range clips {
		abs, err := filepath.Abs(c)
		if err != nil {
			return 0, 0, fmt.Errorf("abs path: %w", err)
		}
		content.WriteString(fmt.Sprintf("file '%s'\n", abs))
	}

	if err := os.WriteFile(listPath, []byte(content.String()), 0644); err != nil {
		return 0, 0, fmt.Errorf("write concat list: %w", err)
	}

	if err := a.runFFmpeg(ctx, []string{"-y", "-f", "concat", "-safe", "0", "-i", listPath, "-c", "copy", outputPath}); err != nil {
		return 0, 0, err
	}
	return introDur, outroDur, nil
}

func (a *Assembler) prepareClip(ctx context.Context, cfg clipConfig, dir, prefix string) (string, float64, error) {
	if cfg.path == "" {
		return "", 0, nil
	}
	if _, err := os.Stat(cfg.path); err != nil {
		return "", 0, nil
	}

	dur, err := a.videoDuration(ctx, cfg.path)
	if err != nil {
		return "", 0, err
	}

	targetDur := dur
	if cfg.duration > 0 && dur > cfg.duration {
		targetDur = cfg.duration
	}

	out := filepath.Join(dir, fmt.Sprintf("%s_%d.mp4", prefix, time.Now().UnixNano()))
	vf := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d", a.width, a.height, a.width, a.height)
	args := []string{"-y", "-i", cfg.path, "-t", fmt.Sprintf("%.2f", targetDur), "-vf", vf, "-c:v", "libx264", "-preset", "ultrafast", "-threads", "0", "-c:a", "aac", "-ar", "44100", out}

	if err := a.runFFmpeg(ctx, args); err != nil {
		return "", 0, err
	}
	return out, targetDur, nil
}

func getEncoder() encoder {
	encoderOnce.Do(func() {
		for _, e := range encoders {
			if e.test() {
				encoderCached = e
				return
			}
		}
		encoderCached = softwareEncoder
	})
	return encoderCached
}

func testEnc(codec string) bool {
	return exec.Command(ffmpegBin, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "nullsrc=s=256x256:d=1", "-c:v", codec, "-frames:v", "1", "-f", "null", "-").Run() == nil
}

func testVAAPI() bool {
	return exec.Command(ffmpegBin, "-hide_banner", "-loglevel", "error", "-vaapi_device", "/dev/dri/renderD128", "-f", "lavfi", "-i", "nullsrc=s=256x256:d=1", "-vf", "format=nv12,hwupload", "-c:v", "h264_vaapi", "-frames:v", "1", "-f", "null", "-").Run() == nil
}

func parseResolution(res string) (int, int) {
	parts := strings.Split(res, "x")
	if len(parts) != 2 {
		return defaultWidth, defaultHeight
	}

	w, err1 := strconv.Atoi(parts[0])
	h, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return defaultWidth, defaultHeight
	}
	return w, h
}

func randomStart(clipDur, needed float64) float64 {
	if clipDur <= needed {
		return 0
	}
	return rand.Float64() * (clipDur - needed)
}

func orDefault(val, def float64) float64 {
	if val == 0 {
		return def
	}
	return val
}

func (a *Assembler) CreatePreview(ctx context.Context, videoPath string, duration float64) (string, error) {
	dir := filepath.Dir(videoPath)
	previewPath := filepath.Join(dir, fmt.Sprintf("preview_%d.mp4", time.Now().UnixNano()))

	args := []string{
		"-y",
		"-i", videoPath,
		"-t", fmt.Sprintf("%.2f", duration),
		"-vf", "scale=540:960",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-crf", "35",
		"-b:v", "500k",
		"-maxrate", "500k",
		"-bufsize", "1M",
		"-c:a", "aac",
		"-b:a", "64k",
		"-ar", "22050",
		"-movflags", "+faststart",
		previewPath,
	}

	if err := a.runFFmpeg(ctx, args); err != nil {
		return "", fmt.Errorf("create preview: %w", err)
	}

	return previewPath, nil
}
