package tts

import "context"

type WordTiming struct {
	Word      string
	StartTime float64
	EndTime   float64
}

type SpeechResult struct {
	Audio   []byte
	Timings []WordTiming
}

type VoiceConfig struct {
	ID         string
	Name       string
	Avatar     string
	Stability  float64
	Similarity float64
}

type Provider interface {
	GenerateSpeech(ctx context.Context, text string) ([]byte, error)
	GenerateSpeechWithTimings(ctx context.Context, text string) (*SpeechResult, error)
	GenerateSpeechWithVoice(ctx context.Context, text string, voice VoiceConfig) (*SpeechResult, error)
}
