package app

import (
	"context"
	"errors"
	"testing"

	"craftstory/internal/elevenlabs"
	"craftstory/internal/uploader"
	"craftstory/internal/video"
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

func TestEnforceImageConstraints(t *testing.T) {
	tests := []struct {
		name     string
		overlays []video.ImageOverlay
		minGap   float64
		want     int
	}{
		{
			name:     "emptyOverlays",
			overlays: []video.ImageOverlay{},
			minGap:   4.0,
			want:     0,
		},
		{
			name: "singleOverlay",
			overlays: []video.ImageOverlay{
				{ImagePath: "img1.jpg", StartTime: 0, EndTime: 1.5},
			},
			minGap: 4.0,
			want:   1,
		},
		{
			name: "allWellSpaced",
			overlays: []video.ImageOverlay{
				{ImagePath: "img1.jpg", StartTime: 0, EndTime: 1.5},
				{ImagePath: "img2.jpg", StartTime: 6, EndTime: 7.5},
				{ImagePath: "img3.jpg", StartTime: 12, EndTime: 13.5},
			},
			minGap: 4.0,
			want:   3,
		},
		{
			name: "someTooClose",
			overlays: []video.ImageOverlay{
				{ImagePath: "img1.jpg", StartTime: 0, EndTime: 1.5},
				{ImagePath: "img2.jpg", StartTime: 2, EndTime: 3.5},   // gap 0.5, skipped
				{ImagePath: "img3.jpg", StartTime: 6, EndTime: 7.5},   // gap 4.5 from img1, kept
				{ImagePath: "img4.jpg", StartTime: 8, EndTime: 9.5},   // gap 0.5 from img3, skipped
				{ImagePath: "img5.jpg", StartTime: 14, EndTime: 15.5}, // gap 6.5 from img3, kept
			},
			minGap: 4.0,
			want:   3, // img1, img3, img5
		},
		{
			name: "allTooClose",
			overlays: []video.ImageOverlay{
				{ImagePath: "img1.jpg", StartTime: 0, EndTime: 1.5},
				{ImagePath: "img2.jpg", StartTime: 2, EndTime: 3.5},
				{ImagePath: "img3.jpg", StartTime: 4, EndTime: 5.5},
				{ImagePath: "img4.jpg", StartTime: 6, EndTime: 7.5},
			},
			minGap: 4.0,
			want:   2, // img1, img4 (gap from 1.5 to 6 = 4.5)
		},
		{
			name: "exactMinGap",
			overlays: []video.ImageOverlay{
				{ImagePath: "img1.jpg", StartTime: 0, EndTime: 1.5},
				{ImagePath: "img2.jpg", StartTime: 5.5, EndTime: 7}, // gap exactly 4.0
			},
			minGap: 4.0,
			want:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Visuals: config.VisualsConfig{
					MinGap: tt.minGap,
				},
			}
			svc := NewService(cfg, nil, nil, nil, nil, nil, nil, nil)
			pipeline := NewPipeline(svc)

			got := pipeline.enforceImageConstraints(tt.overlays)
			if len(got) != tt.want {
				t.Errorf("enforceImageConstraints() returned %d overlays, want %d", len(got), tt.want)
			}
		})
	}
}

func TestFindKeywordIndex(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		keyword string
		want    int
	}{
		{
			name:    "simpleMatch",
			script:  "The quick brown fox jumps",
			keyword: "fox",
			want:    3,
		},
		{
			name:    "firstWord",
			script:  "Octopus are amazing creatures",
			keyword: "octopus",
			want:    0,
		},
		{
			name:    "lastWord",
			script:  "Look at that beautiful sunset",
			keyword: "sunset",
			want:    4,
		},
		{
			name:    "caseInsensitive",
			script:  "The TIGER is sleeping",
			keyword: "tiger",
			want:    1,
		},
		{
			name:    "withPunctuation",
			script:  "Look, a cat! Amazing.",
			keyword: "cat",
			want:    2,
		},
		{
			name:    "notFound",
			script:  "The quick brown fox",
			keyword: "elephant",
			want:    -1,
		},
		{
			name:    "emptyKeyword",
			script:  "Some script here",
			keyword: "",
			want:    -1,
		},
		{
			name:    "multiWordKeyword",
			script:  "The blue ringed octopus is dangerous",
			keyword: "blue ringed",
			want:    1,
		},
		{
			name:    "partialMatch",
			script:  "The octopuses swim fast",
			keyword: "octopus",
			want:    1,
		},
		{
			name:    "quotedWord",
			script:  `They call it "magic" for a reason`,
			keyword: "magic",
			want:    3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findKeywordIndex(tt.script, tt.keyword)
			if got != tt.want {
				t.Errorf("findKeywordIndex(%q, %q) = %d, want %d", tt.script, tt.keyword, got, tt.want)
			}
		})
	}
}
