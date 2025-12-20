package elevenlabs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGenerateSpeech(t *testing.T) {
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
			serverBody:   []byte{0x49, 0x44, 0x33},
			wantErr:      false,
		},
		{
			name:         "emptyResponse",
			text:         "Test",
			serverStatus: http.StatusOK,
			serverBody:   []byte{},
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
				if len(got) != len(tt.serverBody) {
					t.Errorf("GenerateSpeech() returned %d bytes, want %d", len(got), len(tt.serverBody))
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
		_, _ = w.Write([]byte{0x00})
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
