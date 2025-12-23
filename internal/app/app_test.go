package app

import (
	"context"
	"errors"
	"testing"

	"craftstory/internal/tts"
	"craftstory/internal/uploader"
	"craftstory/pkg/config"
)

type mockUploader struct {
	response *uploader.UploadResponse
	err      error
}

func (m *mockUploader) Upload(_ context.Context, _ uploader.UploadRequest) (*uploader.UploadResponse, error) {
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

func TestServiceGetters(t *testing.T) {
	cfg := &config.Config{}
	svc := NewService(ServiceOptions{Config: cfg})

	if svc.Config() != cfg {
		t.Error("Config() returned wrong config")
	}
	if svc.LLM() != nil {
		t.Error("LLM() should return nil when set to nil")
	}
	if svc.TTS() != nil {
		t.Error("TTS() should return nil when set to nil")
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
	service := NewService(ServiceOptions{Config: cfg})
	pipeline := NewPipeline(service)

	if pipeline == nil {
		t.Fatal("NewPipeline() returned nil")
	}
}

func TestAudioDuration(t *testing.T) {
	tests := []struct {
		name    string
		timings []tts.WordTiming
		want    float64
	}{
		{
			name:    "emptyTimings",
			timings: []tts.WordTiming{},
			want:    0,
		},
		{
			name:    "nilTimings",
			timings: nil,
			want:    0,
		},
		{
			name: "singleWord",
			timings: []tts.WordTiming{
				{Word: "Hello", StartTime: 0, EndTime: 0.5},
			},
			want: 0.5,
		},
		{
			name: "multipleWords",
			timings: []tts.WordTiming{
				{Word: "Hello", StartTime: 0, EndTime: 0.5},
				{Word: "World", StartTime: 0.5, EndTime: 1.0},
				{Word: "Test", StartTime: 1.0, EndTime: 1.5},
			},
			want: 1.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := audioDuration(tt.timings)
			if got != tt.want {
				t.Errorf("audioDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPipelineUpload(t *testing.T) {
	tests := []struct {
		name       string
		req        UploadRequest
		uploadResp *uploader.UploadResponse
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
			uploadResp: &uploader.UploadResponse{
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

func TestBuildVoiceMap(t *testing.T) {
	voices := []tts.VoiceConfig{
		{ID: "1", Name: "Alice"},
		{ID: "2", Name: "Bob"},
	}

	m := buildVoiceMap(voices)

	if len(m) != 2 {
		t.Errorf("buildVoiceMap() returned %d entries, want 2", len(m))
	}
	if m["Alice"].ID != "1" {
		t.Errorf("buildVoiceMap()[Alice].ID = %q, want %q", m["Alice"].ID, "1")
	}
	if m["Bob"].ID != "2" {
		t.Errorf("buildVoiceMap()[Bob].ID = %q, want %q", m["Bob"].ID, "2")
	}
}

func TestCleanWord(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello", "hello"},
		{"Hello!", "hello"},
		{"\"quoted\"", "quoted"},
		{"(parens)", "parens"},
		{"word.", "word"},
		{"UPPER", "upper"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := cleanWord(tt.input)
			if got != tt.want {
				t.Errorf("cleanWord(%q) = %q, want %q", tt.input, got, tt.want)
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
	duration := audioDuration([]tts.WordTiming{
		{Word: "test", StartTime: 0, EndTime: 120.0},
	})
	maxDuration := cfg.Video.MaxDuration

	if maxDuration > 0 && duration > maxDuration {
		t.Error("expected zero maxDuration to allow any duration")
	}
}
