package xtts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"craftstory/internal/tts"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name        string
		opts        Options
		wantURL     string
		wantLang    string
		wantSpeaker string
	}{
		{
			name:        "defaults",
			opts:        Options{},
			wantURL:     defaultServerURL,
			wantLang:    "en",
			wantSpeaker: "",
		},
		{
			name: "custom",
			opts: Options{
				ServerURL: "http://custom:9000",
				Speaker:   "spongebob",
				Language:  "es",
			},
			wantURL:     "http://custom:9000",
			wantLang:    "es",
			wantSpeaker: "spongebob",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient(tt.opts)
			if c.serverURL != tt.wantURL {
				t.Errorf("serverURL = %q, want %q", c.serverURL, tt.wantURL)
			}
			if c.language != tt.wantLang {
				t.Errorf("language = %q, want %q", c.language, tt.wantLang)
			}
			if c.speaker != tt.wantSpeaker {
				t.Errorf("speaker = %q, want %q", c.speaker, tt.wantSpeaker)
			}
		})
	}
}

func TestEstimateWordTimings(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		audioSize int
		wantLen   int
	}{
		{"empty", "", 48000, 0},
		{"single", "Hello", 48000, 1},
		{"multiple", "Hello world test", 48000 * 3, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			audio := make([]byte, tt.audioSize)
			timings := estimateWordTimings(tt.text, audio)
			if len(timings) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(timings), tt.wantLen)
			}
		})
	}
}

func TestSpeakerAccessors(t *testing.T) {
	c := NewClient(Options{Speaker: "initial"})
	if c.Speaker() != "initial" {
		t.Errorf("Speaker() = %q, want initial", c.Speaker())
	}
	c.SetSpeaker("new")
	if c.Speaker() != "new" {
		t.Errorf("Speaker() = %q, want new", c.Speaker())
	}
}

func TestIsServerRunning(t *testing.T) {
	t.Run("up", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		c := NewClient(Options{ServerURL: srv.URL})
		if !c.IsServerRunning() {
			t.Error("expected true")
		}
	})

	t.Run("down", func(t *testing.T) {
		c := NewClient(Options{ServerURL: "http://localhost:59999"})
		if c.IsServerRunning() {
			t.Error("expected false")
		}
	})
}

func TestGenerateSpeech(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tts_to_audio/" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if req["speaker_wav"] == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("missing speaker"))
			return
		}

		_, _ = w.Write(make([]byte, 48000))
	}))
	defer srv.Close()

	c := NewClient(Options{ServerURL: srv.URL, Speaker: "test"})
	ctx := context.Background()

	audio, err := c.GenerateSpeech(ctx, "Hello world")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(audio) == 0 {
		t.Error("audio empty")
	}
}

func TestGenerateSpeechWithTimings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, 48000*2))
	}))
	defer srv.Close()

	c := NewClient(Options{ServerURL: srv.URL, Speaker: "test"})
	ctx := context.Background()

	result, err := c.GenerateSpeechWithTimings(ctx, "Hello world")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(result.Audio) == 0 {
		t.Error("audio empty")
	}
	if len(result.Timings) != 2 {
		t.Errorf("timings = %d, want 2", len(result.Timings))
	}
}

func TestGenerateSpeechWithVoice(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if req["speaker_wav"] != "/app/speakers/patrick.wav" {
			t.Errorf("speaker = %q, want /app/speakers/patrick.wav", req["speaker_wav"])
		}

		_, _ = w.Write(make([]byte, 48000))
	}))
	defer srv.Close()

	c := NewClient(Options{ServerURL: srv.URL, Speaker: "default"})
	ctx := context.Background()

	voice := tts.VoiceConfig{Name: "patrick"}
	result, err := c.GenerateSpeechWithVoice(ctx, "Hello", voice)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(result.Audio) == 0 {
		t.Error("audio empty")
	}
}

func TestGenerateSpeechWithVoiceFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if req["speaker_wav"] != "/app/speakers/default.wav" {
			t.Errorf("speaker = %q, want /app/speakers/default.wav", req["speaker_wav"])
		}

		_, _ = w.Write(make([]byte, 48000))
	}))
	defer srv.Close()

	c := NewClient(Options{ServerURL: srv.URL, Speaker: "default"})
	ctx := context.Background()

	result, err := c.GenerateSpeechWithVoice(ctx, "Hello", tts.VoiceConfig{})
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(result.Audio) == 0 {
		t.Error("audio empty")
	}
}

func TestServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error"))
	}))
	defer srv.Close()

	c := NewClient(Options{ServerURL: srv.URL, Speaker: "test"})
	ctx := context.Background()

	_, err := c.GenerateSpeech(ctx, "Hello")
	if err == nil {
		t.Error("expected error")
	}
}

func TestTimingSequence(t *testing.T) {
	audio := make([]byte, 48000*5)
	timings := estimateWordTimings("one two three four five", audio)

	for i := 1; i < len(timings); i++ {
		if timings[i].StartTime < timings[i-1].EndTime {
			t.Errorf("timing[%d] overlaps with timing[%d]", i, i-1)
		}
	}

	if timings[0].StartTime != 0 {
		t.Errorf("first timing should start at 0")
	}
}
