package speech

import (
	"context"
	"strings"
)

const DefaultWordsPerMinute = 150.0

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

func EstimateTimingsFromDuration(text string, duration float64) []WordTiming {
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

func EstimateTimings(text string, audio []byte) []WordTiming {
	duration := EstimateAudioDuration(audio)
	return EstimateTimingsFromDuration(text, duration)
}

func EstimateAudioDuration(audio []byte) float64 {
	bitrate := 128000.0
	return float64(len(audio)*8) / bitrate
}

func AddPauses(text string) string {
	text = strings.ReplaceAll(text, "...", "…")
	text = strings.ReplaceAll(text, ". ", "... ")
	text = strings.ReplaceAll(text, "! ", "!.. ")
	text = strings.ReplaceAll(text, "? ", "?.. ")
	text = strings.ReplaceAll(text, "…", "...")
	return text
}

func Duration(timings []WordTiming) float64 {
	if len(timings) == 0 {
		return 0
	}
	return timings[len(timings)-1].EndTime
}

func BuildVoiceMap(voices []VoiceConfig) map[string]VoiceConfig {
	m := make(map[string]VoiceConfig, len(voices))
	for _, v := range voices {
		m[v.Name] = v
	}
	return m
}

func BuildSpeakerColors(voiceMap map[string]VoiceConfig) map[string]string {
	colors := make(map[string]string, len(voiceMap))
	for name, voice := range voiceMap {
		if voice.SubtitleColor != "" {
			colors[name] = voice.SubtitleColor
		}
	}
	return colors
}
