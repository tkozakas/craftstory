package app

import (
	"context"
	"errors"
	"testing"

	"craftstory/internal/distribution"
	"craftstory/internal/speech"
	"craftstory/pkg/config"
)

type mockUploader struct {
	response *distribution.UploadResponse
	err      error
}

func (m *mockUploader) Upload(_ context.Context, _ distribution.UploadRequest) (*distribution.UploadResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func (m *mockUploader) SetPrivacy(_ context.Context, _, _ string) error {
	return m.err
}

func (m *mockUploader) Platform() string {
	return "mock"
}

func TestServiceCreation(t *testing.T) {
	cfg := &config.Config{}
	svc := NewService(ServiceOptions{Config: cfg})

	if svc == nil {
		t.Error("NewService() returned nil")
	}
}

func TestNewPipeline(t *testing.T) {
	cfg := &config.Config{}
	service := NewService(ServiceOptions{Config: cfg})
	pipeline := NewPipeline(service)

	if pipeline == nil {
		t.Fatal("NewPipeline() returned nil")
	}
}

func TestPipelineUpload(t *testing.T) {
	tests := []struct {
		name       string
		req        UploadRequest
		uploadResp *distribution.UploadResponse
		uploadErr  error
		wantErr    bool
	}{
		{
			name: "successfulUpload",
			req: UploadRequest{
				VideoPath:   "/path/to/video.mp4",
				Title:       "Test Title",
				Description: "Test Description",
			},
			uploadResp: &distribution.UploadResponse{
				ID:       "abc123",
				URL:      "https://youtube.com/watch?v=abc123",
				Platform: "youtube",
			},
			uploadErr: nil,
			wantErr:   false,
		},
		{
			name: "uploadError",
			req: UploadRequest{
				VideoPath:   "/path/to/video.mp4",
				Title:       "Test Title",
				Description: "Test Description",
			},
			uploadResp: nil,
			uploadErr:  errors.New("upload failed"),
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockUp := &mockUploader{
				response: tt.uploadResp,
				err:      tt.uploadErr,
			}

			cfg := &config.Config{
				YouTube: config.YouTubeConfig{
					DefaultTags:   []string{"test"},
					PrivacyStatus: "private",
				},
			}

			svc := NewService(ServiceOptions{Config: cfg, Uploader: mockUp})
			pipeline := NewPipeline(svc)

			resp, err := pipeline.Upload(t.Context(), tt.req)

			if (err != nil) != tt.wantErr {
				t.Errorf("Upload() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && resp.ID != tt.uploadResp.ID {
				t.Errorf("Upload() ID = %q, want %q", resp.ID, tt.uploadResp.ID)
			}
		})
	}
}

func TestGenerateResultStruct(t *testing.T) {
	result := GenerateResult{
		Title:         "Test Title",
		ScriptContent: "Test script",
		OutputDir:     "/output/20250101_120000_test_title",
		AudioPath:     "/path/to/audio.mp3",
		VideoPath:     "/path/to/video.mp4",
		Duration:      30.5,
	}

	if result.Title != "Test Title" {
		t.Errorf("Title = %q, want %q", result.Title, "Test Title")
	}
	if result.ScriptContent != "Test script" {
		t.Errorf("ScriptContent = %q, want %q", result.ScriptContent, "Test script")
	}
	if result.OutputDir != "/output/20250101_120000_test_title" {
		t.Errorf("OutputDir = %q, want %q", result.OutputDir, "/output/20250101_120000_test_title")
	}
	if result.AudioPath != "/path/to/audio.mp3" {
		t.Errorf("AudioPath = %q, want %q", result.AudioPath, "/path/to/audio.mp3")
	}
	if result.VideoPath != "/path/to/video.mp4" {
		t.Errorf("VideoPath = %q, want %q", result.VideoPath, "/path/to/video.mp4")
	}
	if result.Duration != 30.5 {
		t.Errorf("Duration = %v, want %v", result.Duration, 30.5)
	}
}

func TestSanitizeForPath(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simpleTitle",
			input: "Hello World",
			want:  "hello_world",
		},
		{
			name:  "specialChars",
			input: "What?! Is This... Real???",
			want:  "what_is_this_real",
		},
		{
			name:  "numbers",
			input: "Top 10 Facts",
			want:  "top_10_facts",
		},
		{
			name:  "emojisAndSymbols",
			input: "🔥 Amazing! $100 Deal",
			want:  "amazing_100_deal",
		},
		{
			name:  "alreadyClean",
			input: "simple-title_here",
			want:  "simple-title_here",
		},
		{
			name:  "emptyString",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeForPath(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeForPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMaxDurationFromConfig(t *testing.T) {
	tests := []struct {
		name        string
		maxDuration float64
		duration    float64
		wantErr     bool
	}{
		{
			name:        "exceedsLimit",
			maxDuration: 60.0,
			duration:    65.0,
			wantErr:     true,
		},
		{
			name:        "customLowerLimit",
			maxDuration: 30.0,
			duration:    45.0,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Video: config.VideoConfig{
					MaxDuration: tt.maxDuration,
				},
			}

			duration := tt.duration
			maxDuration := cfg.Video.MaxDuration

			exceedsLimit := maxDuration > 0 && duration > maxDuration
			if tt.wantErr && !exceedsLimit {
				t.Errorf("expected duration %.1f to exceed limit %.1f", duration, maxDuration)
			}
		})
	}
}

func TestMaxDurationZeroAllowsAnyDuration(t *testing.T) {
	cfg := &config.Config{
		Video: config.VideoConfig{
			MaxDuration: 0,
		},
	}
	duration := speech.Duration([]speech.WordTiming{
		{Word: "test", StartTime: 0, EndTime: 120.0},
	})
	maxDuration := cfg.Video.MaxDuration

	if maxDuration > 0 && duration > maxDuration {
		t.Error("expected zero maxDuration to allow any duration")
	}
}
