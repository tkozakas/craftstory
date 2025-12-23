package visuals

import (
	"testing"

	"craftstory/internal/tts"
	"craftstory/internal/video"
)

func TestEnforceConstraints(t *testing.T) {
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
				{ImagePath: "img2.jpg", StartTime: 2, EndTime: 3.5},
				{ImagePath: "img3.jpg", StartTime: 6, EndTime: 7.5},
				{ImagePath: "img4.jpg", StartTime: 8, EndTime: 9.5},
				{ImagePath: "img5.jpg", StartTime: 14, EndTime: 15.5},
			},
			minGap: 4.0,
			want:   3,
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
			want:   2,
		},
		{
			name: "exactMinGap",
			overlays: []video.ImageOverlay{
				{ImagePath: "img1.jpg", StartTime: 0, EndTime: 1.5},
				{ImagePath: "img2.jpg", StartTime: 5.5, EndTime: 7},
			},
			minGap: 4.0,
			want:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &Fetcher{cfg: Config{MinGap: tt.minGap}}
			got := f.enforceConstraints(tt.overlays)
			if len(got) != tt.want {
				t.Errorf("enforceConstraints() returned %d overlays, want %d", len(got), tt.want)
			}
		})
	}
}

func TestFindKeywordInTimings(t *testing.T) {
	timings := []tts.WordTiming{
		{Word: "The", StartTime: 0, EndTime: 0.2},
		{Word: "quick", StartTime: 0.2, EndTime: 0.5},
		{Word: "brown", StartTime: 0.5, EndTime: 0.8},
		{Word: "fox", StartTime: 0.8, EndTime: 1.0},
		{Word: "jumps", StartTime: 1.0, EndTime: 1.3},
	}

	tests := []struct {
		name    string
		timings []tts.WordTiming
		keyword string
		want    int
	}{
		{
			name:    "simpleMatch",
			timings: timings,
			keyword: "fox",
			want:    3,
		},
		{
			name:    "firstWord",
			timings: timings,
			keyword: "the",
			want:    0,
		},
		{
			name:    "lastWord",
			timings: timings,
			keyword: "jumps",
			want:    4,
		},
		{
			name:    "caseInsensitive",
			timings: timings,
			keyword: "QUICK",
			want:    1,
		},
		{
			name: "withPunctuation",
			timings: []tts.WordTiming{
				{Word: "Look,", StartTime: 0, EndTime: 0.3},
				{Word: "a", StartTime: 0.3, EndTime: 0.4},
				{Word: "cat!", StartTime: 0.4, EndTime: 0.7},
			},
			keyword: "cat",
			want:    2,
		},
		{
			name:    "notFound",
			timings: timings,
			keyword: "elephant",
			want:    -1,
		},
		{
			name:    "emptyKeyword",
			timings: timings,
			keyword: "",
			want:    -1,
		},
		{
			name: "multiWordKeyword",
			timings: []tts.WordTiming{
				{Word: "The", StartTime: 0, EndTime: 0.2},
				{Word: "blue", StartTime: 0.2, EndTime: 0.4},
				{Word: "ringed", StartTime: 0.4, EndTime: 0.6},
				{Word: "octopus", StartTime: 0.6, EndTime: 0.9},
			},
			keyword: "blue ringed",
			want:    1,
		},
		{
			name:    "partialMatch",
			timings: []tts.WordTiming{{Word: "octopuses", StartTime: 0, EndTime: 0.5}},
			keyword: "octopus",
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findKeywordInTimings(tt.timings, tt.keyword)
			if got != tt.want {
				t.Errorf("findKeywordInTimings(%q) = %d, want %d", tt.keyword, got, tt.want)
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
		timings    []tts.WordTiming
		startIndex int
		want       float64
	}{
		{
			name: "singleSpeaker",
			timings: []tts.WordTiming{
				{Word: "Hello", StartTime: 0, EndTime: 0.5, Speaker: "Alice"},
				{Word: "world", StartTime: 0.5, EndTime: 1.0, Speaker: "Alice"},
				{Word: "today", StartTime: 1.0, EndTime: 1.5, Speaker: "Alice"},
			},
			startIndex: 0,
			want:       1.5,
		},
		{
			name: "speakerChangesMidway",
			timings: []tts.WordTiming{
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
			timings: []tts.WordTiming{
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
			timings: []tts.WordTiming{
				{Word: "Hello", StartTime: 0, EndTime: 0.5, Speaker: ""},
				{Word: "world", StartTime: 0.5, EndTime: 1.0, Speaker: ""},
			},
			startIndex: 0,
			want:       1.0,
		},
		{
			name:       "invalidIndex",
			timings:    []tts.WordTiming{},
			startIndex: 5,
			want:       0,
		},
		{
			name: "lastWord",
			timings: []tts.WordTiming{
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
