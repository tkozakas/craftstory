package tts

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type WordTiming struct {
	Word      string
	StartTime float64
	EndTime   float64
	Speaker   string
}

type SpeechResult struct {
	Audio   []byte
	Timings []WordTiming
}

type VoiceConfig struct {
	ID            string
	Name          string
	SubtitleColor string
}

type Provider interface {
	GenerateSpeech(ctx context.Context, text string) ([]byte, error)
	GenerateSpeechWithTimings(ctx context.Context, text string) (*SpeechResult, error)
	GenerateSpeechWithVoice(ctx context.Context, text string, voice VoiceConfig) (*SpeechResult, error)
}

func getAudioDuration(ctx context.Context, path string) (float64, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path)
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe: %w", err)
	}
	return strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
}

func estimateTimingsFromDuration(text string, duration float64) []WordTiming {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	avgWordDuration := duration / float64(len(words))
	timings := make([]WordTiming, len(words))
	currentTime := 0.0

	for i, word := range words {
		wordDuration := avgWordDuration * (0.8 + 0.4*float64(len(word))/5.0)
		timings[i] = WordTiming{
			Word:      word,
			StartTime: currentTime,
			EndTime:   currentTime + wordDuration,
		}
		currentTime += wordDuration
	}

	if len(timings) > 0 && currentTime > 0 {
		scale := duration / currentTime
		for i := range timings {
			timings[i].StartTime *= scale
			timings[i].EndTime *= scale
		}
	}

	return timings
}

func estimateTimings(text string, audio []byte) []WordTiming {
	duration := estimateAudioDuration(audio)
	return estimateTimingsFromDuration(text, duration)
}

func estimateAudioDuration(audio []byte) float64 {
	bitrate := 128000.0
	return float64(len(audio)*8) / bitrate
}

func addPauses(text string) string {
	text = strings.ReplaceAll(text, "...", "…")
	text = strings.ReplaceAll(text, ". ", "... ")
	text = strings.ReplaceAll(text, "! ", "!.. ")
	text = strings.ReplaceAll(text, "? ", "?.. ")
	text = strings.ReplaceAll(text, "…", "...")
	return text
}
