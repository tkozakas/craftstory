package elevenlabs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"craftstory/internal/speech"
)

func TestNewClient(t *testing.T) {
	client := newTestClient(Config{
		APIKeys: []string{"test-key"},
		VoiceID: "test-voice",
		Speed:   1.0,
	})

	if len(client.apiKeys) != 1 || client.apiKeys[0] != "test-key" {
		t.Errorf("apiKeys = %v, want [test-key]", client.apiKeys)
	}
	if client.voiceID != "test-voice" {
		t.Errorf("voiceID = %q, want test-voice", client.voiceID)
	}
}

func TestNewClientMultipleKeys(t *testing.T) {
	client := newTestClient(Config{
		APIKeys: []string{"key1", "key2", "key3"},
		VoiceID: "test-voice",
	})

	if len(client.apiKeys) != 3 {
		t.Errorf("apiKeys length = %d, want 3", len(client.apiKeys))
	}
}

func mockTimestampResponse(audio []byte) []byte {
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

func TestGenerateSpeechWithTimings(t *testing.T) {
	fakeAudio := []byte("fake audio data")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("xi-api-key") != "test-key" {
			t.Error("missing or incorrect API key header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing Content-Type header")
		}
		if r.URL.Path != "/text-to-speech/test-voice/with-timestamps" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(mockTimestampResponse(fakeAudio))
	}))
	defer server.Close()

	client := newTestClient(Config{
		APIKeys:    []string{"test-key"},
		VoiceID:    "test-voice",
		Speed:      1.0,
		Stability:  0.5,
		Similarity: 0.75,
	}, withBaseURL(server.URL), withHTTPClient(server.Client()))

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
		if result.Timings[0].StartTime != 0.0 {
			t.Errorf("first word start = %f, want 0.0", result.Timings[0].StartTime)
		}
	}
}

func TestGenerateSpeechWithVoice(t *testing.T) {
	fakeAudio := []byte("fake audio data")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/text-to-speech/custom-voice/with-timestamps" {
			t.Errorf("unexpected path: %s, want custom-voice in path", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(mockTimestampResponse(fakeAudio))
	}))
	defer server.Close()

	client := newTestClient(Config{
		APIKeys: []string{"test-key"},
		VoiceID: "default-voice",
	}, withBaseURL(server.URL), withHTTPClient(server.Client()))

	voice := speech.VoiceConfig{
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

func TestGenerateSpeechWithVoiceDefaultFallback(t *testing.T) {
	fakeAudio := []byte("fake audio data")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/text-to-speech/default-voice/with-timestamps" {
			t.Errorf("unexpected path: %s, want default-voice in path", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(mockTimestampResponse(fakeAudio))
	}))
	defer server.Close()

	client := newTestClient(Config{
		APIKeys: []string{"test-key"},
		VoiceID: "default-voice",
	}, withBaseURL(server.URL), withHTTPClient(server.Client()))

	voice := speech.VoiceConfig{ID: "", Name: "NoVoice"}

	result, err := client.GenerateSpeechWithVoice(context.Background(), "Hello", voice)
	if err != nil {
		t.Fatalf("GenerateSpeechWithVoice() error = %v", err)
	}

	if len(result.Audio) == 0 {
		t.Error("expected non-empty audio with fallback voice")
	}
}

func TestAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error": "invalid api key"}`))
	}))
	defer server.Close()

	client := newTestClient(Config{
		APIKeys: []string{"bad-key"},
		VoiceID: "test-voice",
	}, withBaseURL(server.URL), withHTTPClient(server.Client()))

	_, err := client.GenerateSpeech(context.Background(), "Hello")
	if err == nil {
		t.Error("expected error for unauthorized request")
	}
}

func TestParseTimingsNoAlignment(t *testing.T) {
	timings := parseTimings("Hello world", nil)
	if len(timings) != 2 {
		t.Errorf("got %d timings, want 2", len(timings))
	}
}

func TestGenerateSpeech(t *testing.T) {
	fakeAudio := []byte("fake audio data")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(mockTimestampResponse(fakeAudio))
	}))
	defer server.Close()

	client := newTestClient(Config{
		APIKeys: []string{"test-key"},
		VoiceID: "test-voice",
	}, withBaseURL(server.URL), withHTTPClient(server.Client()))

	audio, err := client.GenerateSpeech(context.Background(), "Hello")
	if err != nil {
		t.Fatalf("GenerateSpeech() error = %v", err)
	}

	if string(audio) != "fake audio data" {
		t.Errorf("audio = %q, want 'fake audio data'", string(audio))
	}
}

func TestKeyRotation(t *testing.T) {
	keys := []string{"key1", "key2", "key3"}
	client := newTestClient(Config{APIKeys: keys})

	seen := make(map[string]int)
	for range 6 {
		key := client.nextAPIKey()
		seen[key]++
	}

	for _, k := range keys {
		if seen[k] != 2 {
			t.Errorf("key %q used %d times, want 2", k, seen[k])
		}
	}
}

func TestKeyRotationSingleKey(t *testing.T) {
	client := newTestClient(Config{APIKeys: []string{"single-key"}})

	for range 5 {
		key := client.nextAPIKey()
		if key != "single-key" {
			t.Errorf("nextAPIKey() = %q, want single-key", key)
		}
	}
}

func newTestClient(cfg Config, opts ...option) *Client {
	return newClient(cfg, opts...)
}
