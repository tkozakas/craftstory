package video

import (
	"strings"
	"testing"

	"craftstory/internal/tts"
)

func TestGenerate(t *testing.T) {
	tests := []struct {
		name          string
		text          string
		audioDuration float64
		wantCount     int
	}{
		{
			name:          "simpleText",
			text:          "Hello world",
			audioDuration: 2.0,
			wantCount:     2,
		},
		{
			name:          "longerText",
			text:          "This is a longer sentence with more words",
			audioDuration: 8.0,
			wantCount:     8,
		},
		{
			name:          "emptyText",
			text:          "",
			audioDuration: 5.0,
			wantCount:     0,
		},
		{
			name:          "singleWord",
			text:          "Hello",
			audioDuration: 1.0,
			wantCount:     1,
		},
		{
			name:          "multipleSpaces",
			text:          "Hello    world",
			audioDuration: 2.0,
			wantCount:     2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen := NewSubtitleGenerator(SubtitleOptions{
				FontName: "Arial",
				FontSize: 48,
			})

			subs := gen.Generate(tt.text, tt.audioDuration)

			if len(subs) != tt.wantCount {
				t.Errorf("Generate() returned %d subtitles, want %d", len(subs), tt.wantCount)
			}

			if tt.wantCount > 0 {
				if subs[0].StartTime != 0 {
					t.Errorf("first subtitle should start at 0, got %v", subs[0].StartTime)
				}

				lastSub := subs[len(subs)-1]
				if lastSub.EndTime != tt.audioDuration {
					t.Errorf("last subtitle should end at %v, got %v", tt.audioDuration, lastSub.EndTime)
				}
			}
		})
	}
}

func TestGenerateTiming(t *testing.T) {
	gen := NewSubtitleGenerator(SubtitleOptions{FontName: "Arial", FontSize: 48})

	subs := gen.Generate("one two three", 3.0)

	if len(subs) != 3 {
		t.Fatalf("expected 3 subtitles, got %d", len(subs))
	}

	expectedTimes := []struct {
		start float64
		end   float64
	}{
		{0.0, 1.0},
		{1.0, 2.0},
		{2.0, 3.0},
	}

	for i, sub := range subs {
		if sub.StartTime != expectedTimes[i].start {
			t.Errorf("subtitle %d: start = %v, want %v", i, sub.StartTime, expectedTimes[i].start)
		}
		if sub.EndTime != expectedTimes[i].end {
			t.Errorf("subtitle %d: end = %v, want %v", i, sub.EndTime, expectedTimes[i].end)
		}
	}
}

func TestToASS(t *testing.T) {
	gen := NewSubtitleGenerator(SubtitleOptions{
		FontName: "Impact",
		FontSize: 72,
	})

	subs := []Subtitle{
		{Word: "Hello", StartTime: 0.0, EndTime: 1.0},
		{Word: "World", StartTime: 1.0, EndTime: 2.0},
	}

	ass := gen.ToASS(subs)

	if !strings.Contains(ass, "[Script Info]") {
		t.Error("ASS output missing [Script Info] section")
	}
	if !strings.Contains(ass, "[V4+ Styles]") {
		t.Error("ASS output missing [V4+ Styles] section")
	}
	if !strings.Contains(ass, "[Events]") {
		t.Error("ASS output missing [Events] section")
	}
	if !strings.Contains(ass, "Impact") {
		t.Error("ASS output missing font name")
	}
	if !strings.Contains(ass, "72") {
		t.Error("ASS output missing font size")
	}
	if !strings.Contains(ass, "Hello") {
		t.Error("ASS output missing subtitle text")
	}
	if !strings.Contains(ass, "World") {
		t.Error("ASS output missing subtitle text")
	}
	if !strings.Contains(ass, "Dialogue:") {
		t.Error("ASS output missing Dialogue entries")
	}
}

func TestToASSEmpty(t *testing.T) {
	gen := NewSubtitleGenerator(SubtitleOptions{FontName: "Arial", FontSize: 48})

	ass := gen.ToASS(nil)

	if !strings.Contains(ass, "[Script Info]") {
		t.Error("empty ASS should still have Script Info")
	}
	if strings.Contains(ass, "Dialogue:") {
		t.Error("empty ASS should not have Dialogue entries")
	}
}

func TestFormatASSTime(t *testing.T) {
	tests := []struct {
		seconds float64
		want    string
	}{
		{0.0, "0:00:00.00"},
		{1.0, "0:00:01.00"},
		{1.5, "0:00:01.50"},
		{60.0, "0:01:00.00"},
		{90.25, "0:01:30.25"},
		{3661.99, "1:01:01.98"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatASSTime(tt.seconds)
			if got != tt.want {
				t.Errorf("formatASSTime(%v) = %q, want %q", tt.seconds, got, tt.want)
			}
		})
	}
}

func TestSubtitleWords(t *testing.T) {
	gen := NewSubtitleGenerator(SubtitleOptions{FontName: "Arial", FontSize: 48})

	text := "The quick brown fox"
	subs := gen.Generate(text, 4.0)

	expectedWords := []string{"The", "quick", "brown", "fox"}

	for i, sub := range subs {
		if sub.Word != expectedWords[i] {
			t.Errorf("subtitle %d: word = %q, want %q", i, sub.Word, expectedWords[i])
		}
	}
}

func TestGenerateFromTimings(t *testing.T) {
	gen := NewSubtitleGenerator(SubtitleOptions{FontName: "Arial", FontSize: 48})

	timings := []tts.WordTiming{
		{Word: "Hello", StartTime: 0.0, EndTime: 0.5},
		{Word: "world", StartTime: 0.6, EndTime: 1.1},
	}

	subs := gen.GenerateFromTimings(timings)

	if len(subs) != 2 {
		t.Fatalf("expected 2 subtitles, got %d", len(subs))
	}

	if subs[0].Word != "Hello" {
		t.Errorf("first word = %q, want %q", subs[0].Word, "Hello")
	}
	if subs[0].StartTime != 0.0 {
		t.Errorf("first start = %v, want %v", subs[0].StartTime, 0.0)
	}
	if subs[0].EndTime != 0.5 {
		t.Errorf("first end = %v, want %v", subs[0].EndTime, 0.5)
	}

	if subs[1].Word != "world" {
		t.Errorf("second word = %q, want %q", subs[1].Word, "world")
	}
	if subs[1].StartTime != 0.6 {
		t.Errorf("second start = %v, want %v", subs[1].StartTime, 0.6)
	}
	if subs[1].EndTime != 1.1 {
		t.Errorf("second end = %v, want %v", subs[1].EndTime, 1.1)
	}
}

func TestToASSCenterAligned(t *testing.T) {
	gen := NewSubtitleGenerator(SubtitleOptions{FontName: "Arial", FontSize: 48})

	subs := []Subtitle{
		{Word: "Test", StartTime: 0.0, EndTime: 1.0},
	}

	ass := gen.ToASS(subs)

	if !strings.Contains(ass, ",5,") {
		t.Error("ASS output should have center alignment (5)")
	}
}

func TestGenerateFromTimingsWithOffset(t *testing.T) {
	tests := []struct {
		name           string
		offset         float64
		timings        []tts.WordTiming
		wantStartTimes []float64
		wantEndTimes   []float64
	}{
		{
			name:   "positiveOffset",
			offset: 0.5,
			timings: []tts.WordTiming{
				{Word: "Hello", StartTime: 0.0, EndTime: 0.5},
				{Word: "world", StartTime: 0.6, EndTime: 1.1},
			},
			wantStartTimes: []float64{0.5, 1.1},
			wantEndTimes:   []float64{1.0, 1.6},
		},
		{
			name:   "negativeOffset",
			offset: -0.2,
			timings: []tts.WordTiming{
				{Word: "Hello", StartTime: 0.5, EndTime: 1.0},
				{Word: "world", StartTime: 1.0, EndTime: 1.5},
			},
			wantStartTimes: []float64{0.3, 0.8},
			wantEndTimes:   []float64{0.8, 1.3},
		},
		{
			name:   "negativeOffsetClampsToZero",
			offset: -1.0,
			timings: []tts.WordTiming{
				{Word: "Hello", StartTime: 0.0, EndTime: 0.5},
				{Word: "world", StartTime: 0.6, EndTime: 1.1},
			},
			wantStartTimes: []float64{0.0, 0.0},
			wantEndTimes:   []float64{0.0, 0.1},
		},
		{
			name:   "zeroOffset",
			offset: 0.0,
			timings: []tts.WordTiming{
				{Word: "Hello", StartTime: 0.0, EndTime: 0.5},
				{Word: "world", StartTime: 0.6, EndTime: 1.1},
			},
			wantStartTimes: []float64{0.0, 0.6},
			wantEndTimes:   []float64{0.5, 1.1},
		},
	}

	const epsilon = 0.0001

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen := NewSubtitleGenerator(SubtitleOptions{
				FontName: "Arial",
				FontSize: 48,
				Offset:   tt.offset,
			})

			subs := gen.GenerateFromTimings(tt.timings)

			if len(subs) != len(tt.timings) {
				t.Fatalf("expected %d subtitles, got %d", len(tt.timings), len(subs))
			}

			for i, sub := range subs {
				if diff := sub.StartTime - tt.wantStartTimes[i]; diff > epsilon || diff < -epsilon {
					t.Errorf("subtitle %d: start = %v, want %v", i, sub.StartTime, tt.wantStartTimes[i])
				}
				if diff := sub.EndTime - tt.wantEndTimes[i]; diff > epsilon || diff < -epsilon {
					t.Errorf("subtitle %d: end = %v, want %v", i, sub.EndTime, tt.wantEndTimes[i])
				}
			}
		})
	}
}
