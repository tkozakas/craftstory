package speech

import (
	"context"
	"encoding/binary"
	"strings"
)

const (
	wavSampleRate      = 44100
	wavNumChannels     = 1
	wavBitsPerSample   = 16
	wavHeaderSize      = 44
	wavSubchunkSize    = 16
	wavAudioFormat     = 1
	wavChunkSizeOffset = 36
)

type StubProvider struct {
	wordsPerMinute float64
}

func NewStubProvider(wordsPerMinute float64) Provider {
	if wordsPerMinute <= 0 {
		wordsPerMinute = DefaultWordsPerMinute
	}
	return &StubProvider{wordsPerMinute: wordsPerMinute}
}

func (s *StubProvider) GenerateSpeech(ctx context.Context, text string) ([]byte, error) {
	duration := s.estimateDuration(text)
	return generateSilentWAV(duration), nil
}

func (s *StubProvider) GenerateSpeechWithTimings(ctx context.Context, text string) (*SpeechResult, error) {
	duration := s.estimateDuration(text)
	return &SpeechResult{
		Audio:   generateSilentWAV(duration),
		Timings: EstimateTimingsFromDuration(text, duration),
	}, nil
}

func (s *StubProvider) GenerateSpeechWithVoice(ctx context.Context, text string, voice VoiceConfig) (*SpeechResult, error) {
	return s.GenerateSpeechWithTimings(ctx, text)
}

func (s *StubProvider) estimateDuration(text string) float64 {
	wordCount := len(strings.Fields(text))
	return float64(wordCount) / s.wordsPerMinute * 60.0
}

func generateSilentWAV(durationSec float64) []byte {
	bytesPerSample := wavBitsPerSample / 8
	numSamples := int(durationSec * float64(wavSampleRate))
	dataSize := numSamples * wavNumChannels * bytesPerSample
	byteRate := wavSampleRate * wavNumChannels * bytesPerSample
	blockAlign := wavNumChannels * bytesPerSample

	buf := make([]byte, wavHeaderSize+dataSize)

	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], uint32(wavChunkSizeOffset+dataSize))
	copy(buf[8:12], "WAVE")

	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:20], wavSubchunkSize)
	binary.LittleEndian.PutUint16(buf[20:22], wavAudioFormat)
	binary.LittleEndian.PutUint16(buf[22:24], wavNumChannels)
	binary.LittleEndian.PutUint32(buf[24:28], wavSampleRate)
	binary.LittleEndian.PutUint32(buf[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(buf[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(buf[34:36], wavBitsPerSample)

	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:44], uint32(dataSize))

	return buf
}
