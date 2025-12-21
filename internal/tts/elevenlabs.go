package tts

import (
	"context"

	"craftstory/internal/elevenlabs"
)

type ElevenLabsAdapter struct {
	client *elevenlabs.Client
}

func NewElevenLabsAdapter(client *elevenlabs.Client) *ElevenLabsAdapter {
	return &ElevenLabsAdapter{client: client}
}

func (a *ElevenLabsAdapter) GenerateSpeech(ctx context.Context, text string) ([]byte, error) {
	return a.client.GenerateSpeech(ctx, text)
}

func (a *ElevenLabsAdapter) GenerateSpeechWithTimings(ctx context.Context, text string) (*SpeechResult, error) {
	result, err := a.client.GenerateSpeechWithTimings(ctx, text)
	if err != nil {
		return nil, err
	}

	timings := make([]WordTiming, len(result.Timings))
	for i, t := range result.Timings {
		timings[i] = WordTiming{
			Word:      t.Word,
			StartTime: t.StartTime,
			EndTime:   t.EndTime,
		}
	}

	return &SpeechResult{
		Audio:   result.Audio,
		Timings: timings,
	}, nil
}

func (a *ElevenLabsAdapter) GenerateSpeechWithVoice(ctx context.Context, text string, voice VoiceConfig) (*SpeechResult, error) {
	elVoice := elevenlabs.VoiceConfig{
		ID:         voice.ID,
		Stability:  voice.Stability,
		Similarity: voice.Similarity,
	}

	result, err := a.client.GenerateSpeechWithVoice(ctx, text, elVoice)
	if err != nil {
		return nil, err
	}

	timings := make([]WordTiming, len(result.Timings))
	for i, t := range result.Timings {
		timings[i] = WordTiming{
			Word:      t.Word,
			StartTime: t.StartTime,
			EndTime:   t.EndTime,
		}
	}

	return &SpeechResult{
		Audio:   result.Audio,
		Timings: timings,
	}, nil
}
