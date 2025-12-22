package tts

import "context"

// WordTiming represents timing information for a single word in the audio.
type WordTiming struct {
	Word      string
	StartTime float64
	EndTime   float64
}

// SpeechResult contains generated audio and word-level timing information.
type SpeechResult struct {
	Audio   []byte
	Timings []WordTiming
}

// VoiceConfig holds voice-specific settings for speech generation.
type VoiceConfig struct {
	ID         string
	Name       string
	Avatar     string
	Stability  float64
	Similarity float64
}

// Provider defines the interface for text-to-speech services.
type Provider interface {
	GenerateSpeech(ctx context.Context, text string) ([]byte, error)
	GenerateSpeechWithTimings(ctx context.Context, text string) (*SpeechResult, error)
	GenerateSpeechWithVoice(ctx context.Context, text string, voice VoiceConfig) (*SpeechResult, error)
}
