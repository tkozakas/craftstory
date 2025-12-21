package app

import (
	"context"
	"errors"
	"testing"

	"craftstory/internal/elevenlabs"
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

	svc := NewService(cfg, nil, nil, nil, nil, nil, nil, nil)

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

	if svc.ImageSearch() != nil {
		t.Error("ImageSearch() should return nil when set to nil")
	}
}

func TestNewPipeline(t *testing.T) {
	cfg := &config.Config{}
	svc := NewService(cfg, nil, nil, nil, nil, nil, nil, nil)
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

			svc := NewService(cfg, nil, nil, mockUp, nil, nil, nil, nil)
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

func TestGetAudioDuration(t *testing.T) {
	tests := []struct {
		name    string
		timings []elevenlabs.WordTiming
		want    float64
	}{
		{
			name:    "emptyTimings",
			timings: []elevenlabs.WordTiming{},
			want:    0,
		},
		{
			name:    "nilTimings",
			timings: nil,
			want:    0,
		},
		{
			name: "singleWord",
			timings: []elevenlabs.WordTiming{
				{Word: "Hello", StartTime: 0, EndTime: 0.5},
			},
			want: 0.5,
		},
		{
			name: "multipleWords",
			timings: []elevenlabs.WordTiming{
				{Word: "Hello", StartTime: 0, EndTime: 0.5},
				{Word: "World", StartTime: 0.5, EndTime: 1.0},
				{Word: "Test", StartTime: 1.0, EndTime: 1.5},
			},
			want: 1.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getAudioDuration(tt.timings)
			if got != tt.want {
				t.Errorf("getAudioDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateResultStruct(t *testing.T) {
	result := GenerateResult{
		Title:         "Test Title",
		ScriptContent: "Test script",
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

func TestIsValidImage(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{
			name: "validJPEG",
			data: append([]byte{0xFF, 0xD8, 0xFF}, make([]byte, 100)...),
			want: true,
		},
		{
			name: "validPNG",
			data: append([]byte{0x89, 0x50, 0x4E, 0x47}, make([]byte, 100)...),
			want: true,
		},
		{
			name: "tooSmall",
			data: []byte{0xFF, 0xD8, 0xFF},
			want: false,
		},
		{
			name: "invalidData",
			data: make([]byte, 200),
			want: false,
		},
		{
			name: "emptyData",
			data: []byte{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidImage(tt.data)
			if got != tt.want {
				t.Errorf("isValidImage() = %v, want %v", got, tt.want)
			}
		})
	}
}
