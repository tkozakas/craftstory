package app

import (
	"context"
	"errors"
	"testing"

	"craftstory/internal/uploader"
	"craftstory/pkg/config"
)

type mockUploader struct {
	response *uploader.UploadResponse
	err      error
}

func (m *mockUploader) Upload(ctx context.Context, req uploader.UploadRequest) (*uploader.UploadResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func (m *mockUploader) SetPrivacy(ctx context.Context, videoID, privacy string) error {
	return m.err
}

func (m *mockUploader) Platform() string {
	return "mock"
}

func TestServiceGetters(t *testing.T) {
	cfg := &config.Config{
		DeepSeek: config.DeepSeekConfig{Model: "test-model"},
	}

	svc := NewService(cfg, nil, nil, nil, nil, nil, nil)

	if svc.Config() != cfg {
		t.Error("Config() returned wrong config")
	}

	if svc.DeepSeek() != nil {
		t.Error("DeepSeek() should return nil when set to nil")
	}

	if svc.ElevenLabs() != nil {
		t.Error("ElevenLabs() should return nil when set to nil")
	}

	if svc.Uploader() != nil {
		t.Error("Uploader() should return nil when set to nil")
	}

	if svc.Assembler() != nil {
		t.Error("Assembler() should return nil when set to nil")
	}

	if svc.Storage() != nil {
		t.Error("Storage() should return nil when set to nil")
	}

	if svc.Reddit() != nil {
		t.Error("Reddit() should return nil when set to nil")
	}
}

func TestNewPipeline(t *testing.T) {
	cfg := &config.Config{}
	svc := NewService(cfg, nil, nil, nil, nil, nil, nil)
	pipeline := NewPipeline(svc)

	if pipeline == nil {
		t.Fatal("NewPipeline() returned nil")
	}

	if pipeline.svc != svc {
		t.Error("NewPipeline() did not set service correctly")
	}
}

func TestEstimateAudioDuration(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   float64
	}{
		{
			name:   "shortScript",
			script: "Hello world",
			want:   0.8,
		},
		{
			name:   "longerScript",
			script: "This is a longer script that should take more time to read aloud",
			want:   4.8,
		},
		{
			name:   "emptyScript",
			script: "",
			want:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateAudioDuration(tt.script)
			if got != tt.want {
				t.Errorf("estimateAudioDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPipelineUpload(t *testing.T) {
	tests := []struct {
		name        string
		videoPath   string
		title       string
		description string
		uploadResp  *uploader.UploadResponse
		uploadErr   error
		wantErr     bool
	}{
		{
			name:        "successfulUpload",
			videoPath:   "/path/to/video.mp4",
			title:       "Test Title",
			description: "Test Description",
			uploadResp: &uploader.UploadResponse{
				ID:       "abc123",
				URL:      "https://youtube.com/watch?v=abc123",
				Platform: "youtube",
			},
			uploadErr: nil,
			wantErr:   false,
		},
		{
			name:        "uploadError",
			videoPath:   "/path/to/video.mp4",
			title:       "Test Title",
			description: "Test Description",
			uploadResp:  nil,
			uploadErr:   errors.New("upload failed"),
			wantErr:     true,
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

			svc := NewService(cfg, nil, nil, mockUp, nil, nil, nil)
			pipeline := NewPipeline(svc)

			ctx := context.Background()
			resp, err := pipeline.Upload(ctx, tt.videoPath, tt.title, tt.description)

			if (err != nil) != tt.wantErr {
				t.Errorf("Upload() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if resp.ID != tt.uploadResp.ID {
					t.Errorf("Upload() ID = %q, want %q", resp.ID, tt.uploadResp.ID)
				}
			}
		})
	}
}
