package video

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"craftstory/internal/tts"
)

type AudioSegment struct {
	Audio   []byte
	Timings []tts.WordTiming
}

type StitchedAudio struct {
	Data     []byte
	Timings  []tts.WordTiming
	Duration float64
}

type AudioStitcher struct {
	ffmpegPath string
	tempDir    string
}

func NewAudioStitcher(tempDir string) *AudioStitcher {
	return &AudioStitcher{
		ffmpegPath: "ffmpeg",
		tempDir:    tempDir,
	}
}

func (s *AudioStitcher) Stitch(ctx context.Context, segments []AudioSegment) (*StitchedAudio, error) {
	if len(segments) == 0 {
		return nil, fmt.Errorf("no segments to stitch")
	}

	if len(segments) == 1 {
		duration := float64(0)
		if len(segments[0].Timings) > 0 {
			duration = segments[0].Timings[len(segments[0].Timings)-1].EndTime
		}
		return &StitchedAudio{
			Data:     segments[0].Audio,
			Timings:  segments[0].Timings,
			Duration: duration,
		}, nil
	}

	tempFiles := make([]string, 0, len(segments))
	defer func() {
		for _, f := range tempFiles {
			_ = os.Remove(f)
		}
	}()

	for i, seg := range segments {
		ext := detectAudioFormat(seg.Audio)
		tempPath := filepath.Join(s.tempDir, fmt.Sprintf("seg_%d%s", i, ext))
		if err := os.WriteFile(tempPath, seg.Audio, 0644); err != nil {
			return nil, fmt.Errorf("failed to write segment %d: %w", i, err)
		}
		tempFiles = append(tempFiles, tempPath)
	}

	listPath := filepath.Join(s.tempDir, "concat_list.txt")
	listContent := ""
	for _, f := range tempFiles {
		absPath, err := filepath.Abs(f)
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute path: %w", err)
		}
		listContent += fmt.Sprintf("file '%s'\n", absPath)
	}
	if err := os.WriteFile(listPath, []byte(listContent), 0644); err != nil {
		return nil, fmt.Errorf("failed to write concat list: %w", err)
	}
	defer func() { _ = os.Remove(listPath) }()

	outputPath := filepath.Join(s.tempDir, "stitched.mp3")
	defer func() { _ = os.Remove(outputPath) }()

	args := []string{
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", listPath,
		"-acodec", "libmp3lame",
		"-q:a", "2",
		outputPath,
	}

	cmd := exec.CommandContext(ctx, s.ffmpegPath, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg concat failed: %w, output: %s", err, string(output))
	}

	stitchedData, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read stitched audio: %w", err)
	}

	allTimings, totalDuration := s.adjustTimings(segments)

	return &StitchedAudio{
		Data:     stitchedData,
		Timings:  allTimings,
		Duration: totalDuration,
	}, nil
}

func (s *AudioStitcher) adjustTimings(segments []AudioSegment) ([]tts.WordTiming, float64) {
	var allTimings []tts.WordTiming
	var offset float64

	for _, seg := range segments {
		for _, t := range seg.Timings {
			allTimings = append(allTimings, tts.WordTiming{
				Word:      t.Word,
				StartTime: t.StartTime + offset,
				EndTime:   t.EndTime + offset,
			})
		}
		if len(seg.Timings) > 0 {
			offset = seg.Timings[len(seg.Timings)-1].EndTime + offset
		}
	}

	return allTimings, offset
}

func detectAudioFormat(data []byte) string {
	if len(data) < 4 {
		return ".bin"
	}

	// WAV: starts with "RIFF"
	if data[0] == 'R' && data[1] == 'I' && data[2] == 'F' && data[3] == 'F' {
		return ".wav"
	}

	// MP3: starts with ID3 or 0xFF 0xFB
	if (data[0] == 'I' && data[1] == 'D' && data[2] == '3') ||
		(data[0] == 0xFF && (data[1]&0xE0) == 0xE0) {
		return ".mp3"
	}

	return ".bin"
}
