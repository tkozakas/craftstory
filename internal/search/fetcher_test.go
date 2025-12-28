package search

import (
	"testing"

	"craftstory/internal/speech"
	"craftstory/internal/video"
)

func TestEnforceConstraints(t *testing.T) {
	tests := []struct {
		name        string
		overlays    []video.ImageOverlay
		minGap      float64
		wantCount   int
		wantEndTime float64
	}{
		{
			name:        "emptyOverlays",
			overlays:    []video.ImageOverlay{},
			minGap:      1.0,
			wantCount:   0,
			wantEndTime: 0,
		},
		{
			name: "singleOverlay",
			overlays: []video.ImageOverlay{
				{ImagePath: "img1.jpg", StartTime: 0, EndTime: 1.5},
			},
			minGap:      1.0,
			wantCount:   1,
			wantEndTime: 1.5,
		},
		{
			name: "wellSpaced",
			overlays: []video.ImageOverlay{
				{ImagePath: "img1.jpg", StartTime: 0, EndTime: 2},
				{ImagePath: "img2.jpg", StartTime: 4, EndTime: 6},
			},
			minGap:      1.0,
			wantCount:   2,
			wantEndTime: 2,
		},
		{
			name: "truncatesOverlap",
			overlays: []video.ImageOverlay{
				{ImagePath: "img1.jpg", StartTime: 0, EndTime: 5},
				{ImagePath: "img2.jpg", StartTime: 3, EndTime: 8},
			},
			minGap:      1.0,
			wantCount:   2,
			wantEndTime: 2,
		},
		{
			name: "keepsAllImages",
			overlays: []video.ImageOverlay{
				{ImagePath: "img1.jpg", StartTime: 0, EndTime: 4},
				{ImagePath: "img2.jpg", StartTime: 2, EndTime: 6},
				{ImagePath: "img3.jpg", StartTime: 4, EndTime: 8},
			},
			minGap:      1.0,
			wantCount:   3,
			wantEndTime: 1,
		},
		{
			name: "minDuration",
			overlays: []video.ImageOverlay{
				{ImagePath: "img1.jpg", StartTime: 0, EndTime: 10},
				{ImagePath: "img2.jpg", StartTime: 0.3, EndTime: 5},
			},
			minGap:      1.0,
			wantCount:   2,
			wantEndTime: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fetcher{cfg: FetcherConfig{MinGap: tt.minGap}}
			got := f.enforceConstraints(tt.overlays)
			if len(got) != tt.wantCount {
				t.Errorf("enforceConstraints() returned %d overlays, want %d", len(got), tt.wantCount)
			}
			if tt.wantCount > 0 && got[0].EndTime != tt.wantEndTime {
				t.Errorf("enforceConstraints() first overlay end time = %.2f, want %.2f", got[0].EndTime, tt.wantEndTime)
			}
		})
	}
}

func TestFindKeywordInTimings(t *testing.T) {
	timings := []speech.WordTiming{
		{Word: "The", StartTime: 0, EndTime: 0.2},
		{Word: "quick", StartTime: 0.2, EndTime: 0.5},
		{Word: "brown", StartTime: 0.5, EndTime: 0.8},
		{Word: "fox", StartTime: 0.8, EndTime: 1.0},
		{Word: "jumps", StartTime: 1.0, EndTime: 1.3},
	}

	tests := []struct {
		name      string
		timings   []speech.WordTiming
		keyword   string
		startFrom int
		want      int
	}{
		{
			name:      "simpleMatch",
			timings:   timings,
			keyword:   "fox",
			startFrom: 0,
			want:      3,
		},
		{
			name:      "firstWord",
			timings:   timings,
			keyword:   "the",
			startFrom: 0,
			want:      0,
		},
		{
			name:      "lastWord",
			timings:   timings,
			keyword:   "jumps",
			startFrom: 0,
			want:      4,
		},
		{
			name:      "caseInsensitive",
			timings:   timings,
			keyword:   "QUICK",
			startFrom: 0,
			want:      1,
		},
		{
			name: "withPunctuation",
			timings: []speech.WordTiming{
				{Word: "Look,", StartTime: 0, EndTime: 0.3},
				{Word: "a", StartTime: 0.3, EndTime: 0.4},
				{Word: "cat!", StartTime: 0.4, EndTime: 0.7},
			},
			keyword:   "cat",
			startFrom: 0,
			want:      2,
		},
		{
			name:      "notFound",
			timings:   timings,
			keyword:   "elephant",
			startFrom: 0,
			want:      -1,
		},
		{
			name:      "emptyKeyword",
			timings:   timings,
			keyword:   "",
			startFrom: 0,
			want:      -1,
		},
		{
			name: "multiWordKeyword",
			timings: []speech.WordTiming{
				{Word: "The", StartTime: 0, EndTime: 0.2},
				{Word: "blue", StartTime: 0.2, EndTime: 0.4},
				{Word: "ringed", StartTime: 0.4, EndTime: 0.6},
				{Word: "octopus", StartTime: 0.6, EndTime: 0.9},
			},
			keyword:   "blue ringed",
			startFrom: 0,
			want:      1,
		},
		{
			name:      "partialMatch",
			timings:   []speech.WordTiming{{Word: "octopuses", StartTime: 0, EndTime: 0.5}},
			keyword:   "octopus",
			startFrom: 0,
			want:      0,
		},
		{
			name:      "startFromMiddle",
			timings:   timings,
			keyword:   "the",
			startFrom: 1,
			want:      -1,
		},
		{
			name: "findSecondOccurrence",
			timings: []speech.WordTiming{
				{Word: "the", StartTime: 0, EndTime: 0.2},
				{Word: "fox", StartTime: 0.2, EndTime: 0.5},
				{Word: "and", StartTime: 0.5, EndTime: 0.7},
				{Word: "the", StartTime: 0.7, EndTime: 0.9},
				{Word: "dog", StartTime: 0.9, EndTime: 1.2},
			},
			keyword:   "the",
			startFrom: 1,
			want:      3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findKeywordInTimings(tt.timings, tt.keyword, tt.startFrom)
			if got != tt.want {
				t.Errorf("findKeywordInTimings(%q, startFrom=%d) = %d, want %d", tt.keyword, tt.startFrom, got, tt.want)
			}
		})
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

func TestDetectImageFormat(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{
			name: "tooSmall",
			data: []byte{0x89, 0x50, 0x4E, 0x47},
			want: "",
		},
		{
			name: "jpeg",
			data: append([]byte{0xFF, 0xD8, 0xFF}, make([]byte, 20)...),
			want: ".jpg",
		},
		{
			name: "png",
			data: append([]byte{0x89, 0x50, 0x4E, 0x47}, make([]byte, 20)...),
			want: ".png",
		},
		{
			name: "webp",
			data: []byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'E', 'B', 'P', 0, 0, 0, 0},
			want: ".webp",
		},
		{
			name: "unknown",
			data: make([]byte, 20),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectImageFormat(tt.data)
			if got != tt.want {
				t.Errorf("detectImageFormat() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsValidImage(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{
			name: "tooSmall",
			data: []byte{0x89, 0x50, 0x4E, 0x47},
			want: false,
		},
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
			name: "validWebP",
			data: append([]byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'E', 'B', 'P'}, make([]byte, 100)...),
			want: true,
		},
		{
			name: "invalidMagic",
			data: make([]byte, 200),
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

func TestFindSpeakerSegmentEnd(t *testing.T) {
	tests := []struct {
		name       string
		timings    []speech.WordTiming
		startIndex int
		want       float64
	}{
		{
			name: "singleSpeaker",
			timings: []speech.WordTiming{
				{Word: "Hello", StartTime: 0, EndTime: 0.5, Speaker: "Alice"},
				{Word: "world", StartTime: 0.5, EndTime: 1.0, Speaker: "Alice"},
				{Word: "today", StartTime: 1.0, EndTime: 1.5, Speaker: "Alice"},
			},
			startIndex: 0,
			want:       1.5,
		},
		{
			name: "speakerChangesMidway",
			timings: []speech.WordTiming{
				{Word: "Hello", StartTime: 0, EndTime: 0.5, Speaker: "Alice"},
				{Word: "world", StartTime: 0.5, EndTime: 1.0, Speaker: "Alice"},
				{Word: "Hi", StartTime: 1.0, EndTime: 1.5, Speaker: "Bob"},
				{Word: "there", StartTime: 1.5, EndTime: 2.0, Speaker: "Bob"},
			},
			startIndex: 0,
			want:       1.0,
		},
		{
			name: "startFromMiddle",
			timings: []speech.WordTiming{
				{Word: "Hello", StartTime: 0, EndTime: 0.5, Speaker: "Alice"},
				{Word: "world", StartTime: 0.5, EndTime: 1.0, Speaker: "Alice"},
				{Word: "Hi", StartTime: 1.0, EndTime: 1.5, Speaker: "Bob"},
				{Word: "there", StartTime: 1.5, EndTime: 2.0, Speaker: "Bob"},
			},
			startIndex: 2,
			want:       2.0,
		},
		{
			name: "emptySpeaker",
			timings: []speech.WordTiming{
				{Word: "Hello", StartTime: 0, EndTime: 0.5, Speaker: ""},
				{Word: "world", StartTime: 0.5, EndTime: 1.0, Speaker: ""},
			},
			startIndex: 0,
			want:       1.0,
		},
		{
			name:       "invalidIndex",
			timings:    []speech.WordTiming{},
			startIndex: 5,
			want:       0,
		},
		{
			name: "lastWord",
			timings: []speech.WordTiming{
				{Word: "Hello", StartTime: 0, EndTime: 0.5, Speaker: "Alice"},
				{Word: "world", StartTime: 0.5, EndTime: 1.0, Speaker: "Alice"},
			},
			startIndex: 1,
			want:       1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findSpeakerSegmentEnd(tt.timings, tt.startIndex)
			if got != tt.want {
				t.Errorf("findSpeakerSegmentEnd() = %v, want %v", got, tt.want)
			}
		})
	}
}
