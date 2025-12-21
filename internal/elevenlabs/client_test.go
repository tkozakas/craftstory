package elevenlabs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func makeTimestampResponse(audio []byte) []byte {
	resp := timestampResponse{
		AudioBase64: base64.StdEncoding.EncodeToString(audio),
		Alignment: alignment{
			Characters:          []string{"H", "e", "l", "l", "o", " ", "w", "o", "r", "l", "d"},
			CharacterStartTimes: []float64{0.0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0},
			CharacterEndTimes:   []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0, 1.1},
		},
	}
	data, _ := json.Marshal(resp)
	return data
}

func TestGenerateSpeech(t *testing.T) {
	testAudio := []byte{0x49, 0x44, 0x33}

	tests := []struct {
		name         string
		text         string
		serverStatus int
		serverBody   []byte
		wantErr      bool
	}{
		{
			name:         "successfulGeneration",
			text:         "Hello world",
			serverStatus: http.StatusOK,
			serverBody:   makeTimestampResponse(testAudio),
			wantErr:      false,
		},
		{
			name:         "emptyAudioResponse",
			text:         "Test",
			serverStatus: http.StatusOK,
			serverBody:   makeTimestampResponse([]byte{}),
			wantErr:      true,
		},
		{
			name:         "serverError",
			text:         "Test",
			serverStatus: http.StatusInternalServerError,
			serverBody:   []byte(`{"detail":{"message":"internal error"}}`),
			wantErr:      true,
		},
		{
			name:         "rateLimitError",
			text:         "Test",
			serverStatus: http.StatusTooManyRequests,
			serverBody:   []byte(`{"detail":{"message":"rate limit exceeded"}}`),
			wantErr:      true,
		},
		{
			name:         "invalidApiKey",
			text:         "Test",
			serverStatus: http.StatusUnauthorized,
			serverBody:   []byte(`{"detail":{"message":"invalid api key"}}`),
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if r.Header.Get("xi-api-key") != "test-key" {
					t.Errorf("expected xi-api-key header")
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("expected Content-Type application/json")
				}

				var req request
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Errorf("failed to decode request: %v", err)
				}
				if req.Text != tt.text {
					t.Errorf("request text = %q, want %q", req.Text, tt.text)
				}

				w.WriteHeader(tt.serverStatus)
				_, _ = w.Write(tt.serverBody)
			}))
			defer server.Close()

			client := NewClient("test-key", Options{
				VoiceID:    "voice-123",
				Model:      "eleven_turbo_v2",
				Stability:  0.5,
				Similarity: 0.75,
			})
			client.baseURL = server.URL

			ctx := context.Background()
			got, err := client.GenerateSpeech(ctx, tt.text)

			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateSpeech() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(got) != len(testAudio) {
					t.Errorf("GenerateSpeech() returned %d bytes, want %d", len(got), len(testAudio))
				}
			}
		})
	}
}

func TestGenerateSpeechWithTimings(t *testing.T) {
	testAudio := []byte{0x49, 0x44, 0x33}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(makeTimestampResponse(testAudio))
	}))
	defer server.Close()

	client := NewClient("test-key", Options{
		VoiceID:    "voice-123",
		Model:      "eleven_turbo_v2",
		Stability:  0.5,
		Similarity: 0.75,
	})
	client.baseURL = server.URL

	ctx := context.Background()
	result, err := client.GenerateSpeechWithTimings(ctx, "Hello world")

	if err != nil {
		t.Fatalf("GenerateSpeechWithTimings() error = %v", err)
	}

	if len(result.Audio) != len(testAudio) {
		t.Errorf("Audio length = %d, want %d", len(result.Audio), len(testAudio))
	}

	if len(result.Timings) != 2 {
		t.Errorf("Timings length = %d, want 2 (Hello and world)", len(result.Timings))
	}

	if len(result.Timings) > 0 && result.Timings[0].Word != "Hello" {
		t.Errorf("First word = %q, want %q", result.Timings[0].Word, "Hello")
	}

	if len(result.Timings) > 1 && result.Timings[1].Word != "world" {
		t.Errorf("Second word = %q, want %q", result.Timings[1].Word, "world")
	}
}

func TestExtractWordTimings(t *testing.T) {
	tests := []struct {
		name      string
		alignment alignment
		wantWords []string
	}{
		{
			name: "simpleWords",
			alignment: alignment{
				Characters:          []string{"H", "i", " ", "t", "h", "e", "r", "e"},
				CharacterStartTimes: []float64{0.0, 0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7},
				CharacterEndTimes:   []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8},
			},
			wantWords: []string{"Hi", "there"},
		},
		{
			name: "multipleSpaces",
			alignment: alignment{
				Characters:          []string{"a", " ", " ", "b"},
				CharacterStartTimes: []float64{0.0, 0.1, 0.2, 0.3},
				CharacterEndTimes:   []float64{0.1, 0.2, 0.3, 0.4},
			},
			wantWords: []string{"a", "b"},
		},
		{
			name: "emptyAlignment",
			alignment: alignment{
				Characters:          []string{},
				CharacterStartTimes: []float64{},
				CharacterEndTimes:   []float64{},
			},
			wantWords: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timings := extractWordTimings(tt.alignment)

			if len(timings) != len(tt.wantWords) {
				t.Errorf("extractWordTimings() returned %d words, want %d", len(timings), len(tt.wantWords))
				return
			}

			for i, want := range tt.wantWords {
				if timings[i].Word != want {
					t.Errorf("word[%d] = %q, want %q", i, timings[i].Word, want)
				}
			}
		})
	}
}

func TestVoiceID(t *testing.T) {
	client := NewClient("key", Options{VoiceID: "voice-abc"})
	if got := client.VoiceID(); got != "voice-abc" {
		t.Errorf("VoiceID() = %q, want %q", got, "voice-abc")
	}
}

func TestElevenLabsModel(t *testing.T) {
	client := NewClient("key", Options{Model: "eleven_multilingual_v2"})
	if got := client.Model(); got != "eleven_multilingual_v2" {
		t.Errorf("Model() = %q, want %q", got, "eleven_multilingual_v2")
	}
}

func TestRequestBody(t *testing.T) {
	var capturedReq request

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedReq)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(makeTimestampResponse([]byte{0x00}))
	}))
	defer server.Close()

	client := NewClient("key", Options{
		VoiceID:    "voice-123",
		Model:      "test-model",
		Stability:  0.6,
		Similarity: 0.8,
	})
	client.baseURL = server.URL

	ctx := context.Background()
	_, _ = client.GenerateSpeech(ctx, "Hello")

	if capturedReq.ModelID != "test-model" {
		t.Errorf("request ModelID = %q, want %q", capturedReq.ModelID, "test-model")
	}
	if capturedReq.VoiceSettings.Stability != 0.6 {
		t.Errorf("request Stability = %v, want %v", capturedReq.VoiceSettings.Stability, 0.6)
	}
	if capturedReq.VoiceSettings.SimilarityBoost != 0.8 {
		t.Errorf("request SimilarityBoost = %v, want %v", capturedReq.VoiceSettings.SimilarityBoost, 0.8)
	}
}

func TestGenerateSpeechWithVoice(t *testing.T) {
	testAudio := []byte{0x49, 0x44, 0x33}

	tests := []struct {
		name         string
		text         string
		voice        VoiceConfig
		serverStatus int
		serverBody   []byte
		wantErr      bool
	}{
		{
			name: "successWithCustomVoice",
			text: "Hello",
			voice: VoiceConfig{
				ID:         "custom-voice-id",
				Stability:  0.8,
				Similarity: 0.9,
			},
			serverStatus: http.StatusOK,
			serverBody:   makeTimestampResponse(testAudio),
			wantErr:      false,
		},
		{
			name: "fallbackToClientDefaults",
			text: "Hello",
			voice: VoiceConfig{
				ID:         "another-voice",
				Stability:  0, // Should fallback to client default
				Similarity: 0, // Should fallback to client default
			},
			serverStatus: http.StatusOK,
			serverBody:   makeTimestampResponse(testAudio),
			wantErr:      false,
		},
		{
			name: "serverError",
			text: "Test",
			voice: VoiceConfig{
				ID: "voice-id",
			},
			serverStatus: http.StatusInternalServerError,
			serverBody:   []byte(`{"detail":{"message":"error"}}`),
			wantErr:      true,
		},
		{
			name: "emptyAudio",
			text: "Test",
			voice: VoiceConfig{
				ID: "voice-id",
			},
			serverStatus: http.StatusOK,
			serverBody:   makeTimestampResponse([]byte{}),
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedVoiceID string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				parts := strings.Split(r.URL.Path, "/")
				if len(parts) >= 2 {
					capturedVoiceID = parts[1]
				}

				w.WriteHeader(tt.serverStatus)
				_, _ = w.Write(tt.serverBody)
			}))
			defer server.Close()

			client := NewClient("test-key", Options{
				VoiceID:    "default-voice",
				Model:      "test-model",
				Stability:  0.5,
				Similarity: 0.75,
			})
			client.baseURL = server.URL

			ctx := context.Background()
			result, err := client.GenerateSpeechWithVoice(ctx, tt.text, tt.voice)

			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateSpeechWithVoice() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(result.Audio) != len(testAudio) {
					t.Errorf("Audio length = %d, want %d", len(result.Audio), len(testAudio))
				}
				if capturedVoiceID != tt.voice.ID {
					t.Errorf("Used voice ID = %q, want %q", capturedVoiceID, tt.voice.ID)
				}
			}
		})
	}
}

func TestVoiceConfigStruct(t *testing.T) {
	cfg := VoiceConfig{
		ID:         "voice-123",
		Stability:  0.7,
		Similarity: 0.85,
	}

	if cfg.ID != "voice-123" {
		t.Errorf("ID = %q, want %q", cfg.ID, "voice-123")
	}
	if cfg.Stability != 0.7 {
		t.Errorf("Stability = %v, want %v", cfg.Stability, 0.7)
	}
	if cfg.Similarity != 0.85 {
		t.Errorf("Similarity = %v, want %v", cfg.Similarity, 0.85)
	}
}

func TestWordTimingStruct(t *testing.T) {
	timing := WordTiming{
		Word:      "Hello",
		StartTime: 0.5,
		EndTime:   1.0,
	}

	if timing.Word != "Hello" {
		t.Errorf("Word = %q, want %q", timing.Word, "Hello")
	}
	if timing.StartTime != 0.5 {
		t.Errorf("StartTime = %v, want %v", timing.StartTime, 0.5)
	}
	if timing.EndTime != 1.0 {
		t.Errorf("EndTime = %v, want %v", timing.EndTime, 1.0)
	}
}

func TestSpeechResultStruct(t *testing.T) {
	result := SpeechResult{
		Audio: []byte{0x01, 0x02, 0x03},
		Timings: []WordTiming{
			{Word: "Test", StartTime: 0, EndTime: 0.5},
		},
	}

	if len(result.Audio) != 3 {
		t.Errorf("Audio length = %d, want 3", len(result.Audio))
	}
	if len(result.Timings) != 1 {
		t.Errorf("Timings length = %d, want 1", len(result.Timings))
	}
}
