package dialogue

import (
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLines int
		wantFirst Line
		wantLast  Line
	}{
		{
			name:      "singleLine",
			input:     "Host: Hello world",
			wantLines: 1,
			wantFirst: Line{Speaker: "Host", Text: "Hello world"},
			wantLast:  Line{Speaker: "Host", Text: "Hello world"},
		},
		{
			name:      "multipleLines",
			input:     "Host: First line\nGuest: Second line\nHost: Third line",
			wantLines: 3,
			wantFirst: Line{Speaker: "Host", Text: "First line"},
			wantLast:  Line{Speaker: "Host", Text: "Third line"},
		},
		{
			name:      "withEmptyLines",
			input:     "Host: Hello\n\nGuest: World\n\n",
			wantLines: 2,
			wantFirst: Line{Speaker: "Host", Text: "Hello"},
			wantLast:  Line{Speaker: "Guest", Text: "World"},
		},
		{
			name:      "withWhitespace",
			input:     "  Host  :  Hello world  \n  Guest:Goodbye  ",
			wantLines: 2,
			wantFirst: Line{Speaker: "Host", Text: "Hello world"},
			wantLast:  Line{Speaker: "Guest", Text: "Goodbye"},
		},
		{
			name:      "emptyInput",
			input:     "",
			wantLines: 0,
		},
		{
			name:      "noValidLines",
			input:     "This is not a dialogue\nNeither is this",
			wantLines: 0,
		},
		{
			name:      "speakerWithNumbers",
			input:     "Speaker1: First\nSpeaker2: Second",
			wantLines: 2,
			wantFirst: Line{Speaker: "Speaker1", Text: "First"},
			wantLast:  Line{Speaker: "Speaker2", Text: "Second"},
		},
		{
			name:      "lowercaseSpeaker",
			input:     "host: This should now work\nHost: This also works",
			wantLines: 2,
			wantFirst: Line{Speaker: "host", Text: "This should now work"},
			wantLast:  Line{Speaker: "Host", Text: "This also works"},
		},
		{
			name:      "textWithColons",
			input:     "Host: Time is 10:30 today",
			wantLines: 1,
			wantFirst: Line{Speaker: "Host", Text: "Time is 10:30 today"},
			wantLast:  Line{Speaker: "Host", Text: "Time is 10:30 today"},
		},
		{
			name:      "skipStageDirections",
			input:     "(Scene opens)\nHost: Hello\n[Action]\nGuest: Hi",
			wantLines: 2,
			wantFirst: Line{Speaker: "Host", Text: "Hello"},
			wantLast:  Line{Speaker: "Guest", Text: "Hi"},
		},
		{
			name:      "speakerWithSpaces",
			input:     "The Host: Welcome everyone\nThe Guest: Thanks",
			wantLines: 2,
			wantFirst: Line{Speaker: "The Host", Text: "Welcome everyone"},
			wantLast:  Line{Speaker: "The Guest", Text: "Thanks"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script := Parse(tt.input)

			if len(script.Lines) != tt.wantLines {
				t.Errorf("Parse() got %d lines, want %d", len(script.Lines), tt.wantLines)
				return
			}

			if tt.wantLines > 0 {
				if script.Lines[0] != tt.wantFirst {
					t.Errorf("Parse() first line = %+v, want %+v", script.Lines[0], tt.wantFirst)
				}
				if script.Lines[len(script.Lines)-1] != tt.wantLast {
					t.Errorf("Parse() last line = %+v, want %+v", script.Lines[len(script.Lines)-1], tt.wantLast)
				}
			}
		})
	}
}

func TestScriptSpeakers(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCount int
		wantFirst string
	}{
		{
			name:      "twoSpeakers",
			input:     "Host: Hello\nGuest: Hi\nHost: Bye",
			wantCount: 2,
			wantFirst: "Host",
		},
		{
			name:      "threeSpeakers",
			input:     "A: One\nB: Two\nC: Three",
			wantCount: 3,
			wantFirst: "A",
		},
		{
			name:      "oneSpeaker",
			input:     "Host: Line 1\nHost: Line 2",
			wantCount: 1,
			wantFirst: "Host",
		},
		{
			name:      "empty",
			input:     "",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script := Parse(tt.input)
			speakers := script.Speakers()

			if len(speakers) != tt.wantCount {
				t.Errorf("Speakers() got %d speakers, want %d", len(speakers), tt.wantCount)
			}

			if tt.wantCount > 0 && speakers[0] != tt.wantFirst {
				t.Errorf("Speakers() first = %q, want %q", speakers[0], tt.wantFirst)
			}
		})
	}
}

func TestScriptIsEmpty(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "empty",
			input: "",
			want:  true,
		},
		{
			name:  "noValidLines",
			input: "just text",
			want:  true,
		},
		{
			name:  "hasLines",
			input: "Host: Hello",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script := Parse(tt.input)
			if script.IsEmpty() != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", script.IsEmpty(), tt.want)
			}
		})
	}
}

func TestScriptFullText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "singleLine",
			input: "Host: Hello world",
			want:  "Hello world",
		},
		{
			name:  "multipleLines",
			input: "Host: Hello\nGuest: World",
			want:  "Hello World",
		},
		{
			name:  "empty",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script := Parse(tt.input)
			if script.FullText() != tt.want {
				t.Errorf("FullText() = %q, want %q", script.FullText(), tt.want)
			}
		})
	}
}

func TestParseStickerExtraction(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantStickerID int
		wantText      string
	}{
		{
			name:          "withSticker",
			input:         "Host: [s1] Hello world",
			wantStickerID: 1,
			wantText:      "Hello world",
		},
		{
			name:          "withStickerNumber5",
			input:         "Guest: [s5] Excited greeting!",
			wantStickerID: 5,
			wantText:      "Excited greeting!",
		},
		{
			name:          "withStickerNumber12",
			input:         "Host: [s12] Double digit sticker",
			wantStickerID: 12,
			wantText:      "Double digit sticker",
		},
		{
			name:          "noSticker",
			input:         "Host: Regular text without sticker",
			wantStickerID: 0,
			wantText:      "Regular text without sticker",
		},
		{
			name:          "stickerInMiddle",
			input:         "Host: Text with [s3] in middle",
			wantStickerID: 0,
			wantText:      "Text with [s3] in middle",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			script := Parse(tt.input)
			if len(script.Lines) != 1 {
				t.Fatalf("Parse() got %d lines, want 1", len(script.Lines))
			}

			line := script.Lines[0]
			if line.StickerID != tt.wantStickerID {
				t.Errorf("StickerID = %d, want %d", line.StickerID, tt.wantStickerID)
			}
			if line.Text != tt.wantText {
				t.Errorf("Text = %q, want %q", line.Text, tt.wantText)
			}
		})
	}
}

func TestParseMultipleLinesWithStickers(t *testing.T) {
	input := `Host: [s1] Hello there!
Guest: [s3] Nice to meet you!
Host: No sticker here
Guest: [s7] Final line with sticker`

	script := Parse(input)
	if len(script.Lines) != 4 {
		t.Fatalf("Parse() got %d lines, want 4", len(script.Lines))
	}

	expected := []struct {
		speaker   string
		stickerID int
		text      string
	}{
		{"Host", 1, "Hello there!"},
		{"Guest", 3, "Nice to meet you!"},
		{"Host", 0, "No sticker here"},
		{"Guest", 7, "Final line with sticker"},
	}

	for i, exp := range expected {
		line := script.Lines[i]
		if line.Speaker != exp.speaker {
			t.Errorf("Line %d: Speaker = %q, want %q", i, line.Speaker, exp.speaker)
		}
		if line.StickerID != exp.stickerID {
			t.Errorf("Line %d: StickerID = %d, want %d", i, line.StickerID, exp.stickerID)
		}
		if line.Text != exp.text {
			t.Errorf("Line %d: Text = %q, want %q", i, line.Text, exp.text)
		}
	}
}

func TestParseStripFormattingWithSticker(t *testing.T) {
	input := "Host: [s2] *Bold* and _italic_ text"
	script := Parse(input)

	if len(script.Lines) != 1 {
		t.Fatalf("Parse() got %d lines, want 1", len(script.Lines))
	}

	line := script.Lines[0]
	if line.StickerID != 2 {
		t.Errorf("StickerID = %d, want 2", line.StickerID)
	}
	if line.Text != "Bold and italic text" {
		t.Errorf("Text = %q, want %q", line.Text, "Bold and italic text")
	}
}
