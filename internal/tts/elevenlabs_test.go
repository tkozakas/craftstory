package tts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewElevenLabsClient(t *testing.T) {
	client := NewElevenLabsClient("test-key", ElevenLabsOptions{})

	if client.apiKey != "test-key" {
		t.Errorf("apiKey = %q, want test-key", client.apiKey)
	}
	if client.voiceID != "pNInz6obpgDQGcFmaJgB" {
		t.Errorf("voiceID = %q, want pNInz6obpgDQGcFmaJgB (Adam default)", client.voiceID)
	}
}

func TestNewElevenLabsClientCustomVoice(t *testing.T) {
	client := NewElevenLabsClient("test-key", ElevenLabsOptions{
		VoiceID: "custom-voice-id",
	})

	if client.voiceID != "custom-voice-id" {
		t.Errorf("voiceID = %q, want custom-voice-id", client.voiceID)
	}
}

func mockTimestampResponse(audio []byte, text string) []byte {
	resp := timestampResponse{
		AudioBase64: base64.StdEncoding.EncodeToString(audio),
		Alignment: &alignment{
			Characters:          []string{"H", "e", "l", "l", "o", " ", "w", "o", "r", "l", "d"},
			CharacterStartTimes: []float64{0.0, 0.05, 0.1, 0.15, 0.2, 0.25, 0.3, 0.35, 0.4, 0.45, 0.5},
			CharacterEndTimes:   []float64{0.05, 0.1, 0.15, 0.2, 0.25, 0.3, 0.35, 0.4, 0.45, 0.5, 0.55},
		},
	}
	data, _ := json.Marshal(resp)
	return data
}

func TestElevenLabsGenerateSpeechWithTimings(t *testing.T) {
	fakeAudio := []byte("fake audio data")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("xi-api-key") != "test-key" {
			t.Error("missing or incorrect API key header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing Content-Type header")
		}
		// Check URL contains /with-timestamps
		if r.URL.Path != "/text-to-speech/test-voice/with-timestamps" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(mockTimestampResponse(fakeAudio, "Hello world"))
	}))
	defer server.Close()

	client := &ElevenLabsClient{
		apiKey:     "test-key",
		httpClient: server.Client(),
		voiceID:    "test-voice",
		baseURL:    server.URL,
	}

	result, err := client.GenerateSpeechWithTimings(context.Background(), "Hello world")
	if err != nil {
		t.Fatalf("GenerateSpeechWithTimings() error = %v", err)
	}

	if string(result.Audio) != "fake audio data" {
		t.Errorf("audio = %q, want 'fake audio data'", string(result.Audio))
	}

	if len(result.Timings) != 2 {
		t.Errorf("got %d timings, want 2 (Hello, world)", len(result.Timings))
	}

	if len(result.Timings) >= 2 {
		if result.Timings[0].Word != "Hello" {
			t.Errorf("first word = %q, want Hello", result.Timings[0].Word)
		}
		if result.Timings[1].Word != "world" {
			t.Errorf("second word = %q, want world", result.Timings[1].Word)
		}
		// Check timings are parsed from API response
		if result.Timings[0].StartTime != 0.0 {
			t.Errorf("first word start = %f, want 0.0", result.Timings[0].StartTime)
		}
	}
}

func TestElevenLabsGenerateSpeechWithVoice(t *testing.T) {
	fakeAudio := []byte("fake audio data")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify custom voice ID is in path
		if r.URL.Path != "/text-to-speech/custom-voice/with-timestamps" {
			t.Errorf("unexpected path: %s, want custom-voice in path", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(mockTimestampResponse(fakeAudio, "Hello"))
	}))
	defer server.Close()

	client := &ElevenLabsClient{
		apiKey:     "test-key",
		httpClient: server.Client(),
		voiceID:    "default-voice",
		baseURL:    server.URL,
	}

	voice := VoiceConfig{
		ID:            "custom-voice",
		Name:          "Bella",
		SubtitleColor: "#FF69B4",
	}

	result, err := client.GenerateSpeechWithVoice(context.Background(), "Hello", voice)
	if err != nil {
		t.Fatalf("GenerateSpeechWithVoice() error = %v", err)
	}

	if len(result.Audio) == 0 {
		t.Error("expected non-empty audio")
	}
}

func TestElevenLabsGenerateSpeechWithVoiceDefaultFallback(t *testing.T) {
	fakeAudio := []byte("fake audio data")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should use default voice when VoiceConfig.ID is empty
		if r.URL.Path != "/text-to-speech/default-voice/with-timestamps" {
			t.Errorf("unexpected path: %s, want default-voice in path", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(mockTimestampResponse(fakeAudio, "Hello"))
	}))
	defer server.Close()

	client := &ElevenLabsClient{
		apiKey:     "test-key",
		httpClient: server.Client(),
		voiceID:    "default-voice",
		baseURL:    server.URL,
	}

	voice := VoiceConfig{
		ID:   "", // Empty - should fallback to default
		Name: "NoVoice",
	}

	result, err := client.GenerateSpeechWithVoice(context.Background(), "Hello", voice)
	if err != nil {
		t.Fatalf("GenerateSpeechWithVoice() error = %v", err)
	}

	if len(result.Audio) == 0 {
		t.Error("expected non-empty audio with fallback voice")
	}
}

func TestElevenLabsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error": "invalid api key"}`))
	}))
	defer server.Close()

	client := &ElevenLabsClient{
		apiKey:     "bad-key",
		httpClient: server.Client(),
		voiceID:    "test-voice",
		baseURL:    server.URL,
	}

	_, err := client.GenerateSpeech(context.Background(), "Hello")
	if err == nil {
		t.Error("expected error for unauthorized request")
	}
}

func TestParseTimingsNoAlignment(t *testing.T) {
	client := &ElevenLabsClient{}

	// Should fallback to estimation when no alignment
	timings := client.parseTimings("Hello world", nil)
	if len(timings) != 2 {
		t.Errorf("got %d timings, want 2", len(timings))
	}
}

func TestGenerateSpeech(t *testing.T) {
	fakeAudio := []byte("fake audio data")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(mockTimestampResponse(fakeAudio, "Hello"))
	}))
	defer server.Close()

	client := &ElevenLabsClient{
		apiKey:     "test-key",
		httpClient: server.Client(),
		voiceID:    "test-voice",
		baseURL:    server.URL,
	}

	audio, err := client.GenerateSpeech(context.Background(), "Hello")
	if err != nil {
		t.Fatalf("GenerateSpeech() error = %v", err)
	}

	if string(audio) != "fake audio data" {
		t.Errorf("audio = %q, want 'fake audio data'", string(audio))
	}
}
