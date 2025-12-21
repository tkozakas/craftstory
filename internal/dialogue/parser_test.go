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
