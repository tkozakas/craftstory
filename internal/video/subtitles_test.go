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

func TestGenerateFromTimingsWithSpeakerColors(t *testing.T) {
	gen := NewSubtitleGenerator(SubtitleOptions{FontName: "Arial", FontSize: 48})

	timings := []tts.WordTiming{
		{Word: "Hello", StartTime: 0.0, EndTime: 0.5, Speaker: "Adam"},
		{Word: "there", StartTime: 0.6, EndTime: 1.0, Speaker: "Adam"},
		{Word: "Hi", StartTime: 1.1, EndTime: 1.5, Speaker: "Bella"},
		{Word: "back", StartTime: 1.6, EndTime: 2.0, Speaker: "Bella"},
	}

	speakerColors := map[string]string{
		"Adam":  "#00BFFF",
		"Bella": "#FF69B4",
	}

	subs := gen.GenerateFromTimingsWithColors(timings, speakerColors)

	if len(subs) != 4 {
		t.Fatalf("expected 4 subtitles, got %d", len(subs))
	}

	if subs[0].Color != "#00BFFF" {
		t.Errorf("subtitle 0: color = %q, want #00BFFF", subs[0].Color)
	}
	if subs[1].Color != "#00BFFF" {
		t.Errorf("subtitle 1: color = %q, want #00BFFF", subs[1].Color)
	}
	if subs[2].Color != "#FF69B4" {
		t.Errorf("subtitle 2: color = %q, want #FF69B4", subs[2].Color)
	}
	if subs[3].Color != "#FF69B4" {
		t.Errorf("subtitle 3: color = %q, want #FF69B4", subs[3].Color)
	}
}

func TestToASSWithSpeakerColors(t *testing.T) {
	gen := NewSubtitleGenerator(SubtitleOptions{FontName: "Arial", FontSize: 48})

	subs := []Subtitle{
		{Word: "Hello", StartTime: 0.0, EndTime: 0.5, Color: "#00BFFF"},
		{Word: "Hi", StartTime: 0.6, EndTime: 1.0, Color: "#FF69B4"},
	}

	ass := gen.ToASS(subs)

	if !strings.Contains(ass, "{\\c&H00FFBF00}Hello") {
		t.Errorf("ASS should contain color override for Hello, got: %s", ass)
	}
	if !strings.Contains(ass, "{\\c&H00B469FF}Hi") {
		t.Errorf("ASS should contain color override for Hi, got: %s", ass)
	}
}

func TestToASSColor(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"#FFFFFF", "&H00FFFFFF"},
		{"#000000", "&H00000000"},
		{"#00BFFF", "&H00FFBF00"},
		{"#FF69B4", "&H00B469FF"},
		{"&H00FFFFFF", "&H00FFFFFF"},
		{"invalid", "&H00FFFFFF"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := toASSColor(tt.input)
			if got != tt.want {
				t.Errorf("toASSColor(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSubtitleTimingSyncWithTTS(t *testing.T) {
	gen := NewSubtitleGenerator(SubtitleOptions{FontName: "Arial", FontSize: 48})

	timings := []tts.WordTiming{
		{Word: "The", StartTime: 0.0, EndTime: 0.2},
		{Word: "quick", StartTime: 0.25, EndTime: 0.5},
		{Word: "brown", StartTime: 0.55, EndTime: 0.8},
		{Word: "fox", StartTime: 0.85, EndTime: 1.1},
	}

	subs := gen.GenerateFromTimings(timings)

	for i, timing := range timings {
		if subs[i].Word != timing.Word {
			t.Errorf("word mismatch at %d: got %q, want %q", i, subs[i].Word, timing.Word)
		}
		if subs[i].StartTime != timing.StartTime {
			t.Errorf("start time mismatch at %d: got %v, want %v", i, subs[i].StartTime, timing.StartTime)
		}
		if subs[i].EndTime != timing.EndTime {
			t.Errorf("end time mismatch at %d: got %v, want %v", i, subs[i].EndTime, timing.EndTime)
		}
	}
}

func TestSubtitleNoOverlap(t *testing.T) {
	gen := NewSubtitleGenerator(SubtitleOptions{FontName: "Arial", FontSize: 48})

	timings := []tts.WordTiming{
		{Word: "Hello", StartTime: 0.0, EndTime: 0.5},
		{Word: "world", StartTime: 0.6, EndTime: 1.1},
		{Word: "test", StartTime: 1.2, EndTime: 1.8},
	}

	subs := gen.GenerateFromTimings(timings)

	for i := 1; i < len(subs); i++ {
		if subs[i].StartTime < subs[i-1].EndTime {
			t.Errorf("subtitle %d overlaps with %d: %v < %v",
				i, i-1, subs[i].StartTime, subs[i-1].EndTime)
		}
	}
}

func TestImageOverlayTimingWithTTS(t *testing.T) {
	timings := []tts.WordTiming{
		{Word: "The", StartTime: 0.0, EndTime: 0.2, Speaker: "Adam"},
		{Word: "quick", StartTime: 0.25, EndTime: 0.5, Speaker: "Adam"},
		{Word: "fox", StartTime: 0.55, EndTime: 0.8, Speaker: "Adam"},
		{Word: "jumps", StartTime: 0.85, EndTime: 1.1, Speaker: "Bella"},
		{Word: "high", StartTime: 1.15, EndTime: 1.4, Speaker: "Bella"},
	}

	overlay := ImageOverlay{
		ImagePath: "/tmp/fox.jpg",
		StartTime: timings[2].StartTime,
		EndTime:   timings[2].EndTime,
		Width:     800,
		Height:    600,
	}

	if overlay.StartTime != 0.55 {
		t.Errorf("overlay start = %v, want 0.55", overlay.StartTime)
	}
	if overlay.EndTime != 0.8 {
		t.Errorf("overlay end = %v, want 0.8", overlay.EndTime)
	}

	if overlay.StartTime < 0 {
		t.Error("overlay start time should not be negative")
	}
	if overlay.EndTime <= overlay.StartTime {
		t.Error("overlay end time should be after start time")
	}
}

func TestImageOverlayExtendedToSpeakerSegment(t *testing.T) {
	timings := []tts.WordTiming{
		{Word: "The", StartTime: 0.0, EndTime: 0.2, Speaker: "Adam"},
		{Word: "quick", StartTime: 0.25, EndTime: 0.5, Speaker: "Adam"},
		{Word: "fox", StartTime: 0.55, EndTime: 0.8, Speaker: "Adam"},
		{Word: "jumps", StartTime: 0.85, EndTime: 1.1, Speaker: "Bella"},
	}

	keywordIndex := 2
	segmentEnd := findSpeakerSegmentEnd(timings, keywordIndex)

	overlay := ImageOverlay{
		ImagePath: "/tmp/fox.jpg",
		StartTime: timings[keywordIndex].StartTime,
		EndTime:   segmentEnd,
		Width:     800,
		Height:    600,
	}

	if overlay.StartTime != 0.55 {
		t.Errorf("overlay start = %v, want 0.55", overlay.StartTime)
	}
	if overlay.EndTime != 0.8 {
		t.Errorf("overlay end = %v, want 0.8 (end of Adam's segment)", overlay.EndTime)
	}
}

func findSpeakerSegmentEnd(timings []tts.WordTiming, startIndex int) float64 {
	if startIndex < 0 || startIndex >= len(timings) {
		return 0
	}

	speaker := timings[startIndex].Speaker
	lastEndTime := timings[startIndex].EndTime

	for i := startIndex + 1; i < len(timings); i++ {
		if timings[i].Speaker != speaker && speaker != "" {
			break
		}
		lastEndTime = timings[i].EndTime
	}

	return lastEndTime
}

func TestMultipleImageOverlaysNoOverlap(t *testing.T) {
	overlays := []ImageOverlay{
		{ImagePath: "/tmp/1.jpg", StartTime: 0.0, EndTime: 1.0},
		{ImagePath: "/tmp/2.jpg", StartTime: 1.5, EndTime: 2.5},
		{ImagePath: "/tmp/3.jpg", StartTime: 3.0, EndTime: 4.0},
	}

	minGap := 0.5
	for i := 1; i < len(overlays); i++ {
		gap := overlays[i].StartTime - overlays[i-1].EndTime
		if gap < minGap {
			t.Errorf("gap between overlay %d and %d is %v, want >= %v",
				i-1, i, gap, minGap)
		}
	}
}

func TestConversationSubtitleSync(t *testing.T) {
	gen := NewSubtitleGenerator(SubtitleOptions{FontName: "Arial", FontSize: 48})

	timings := []tts.WordTiming{
		{Word: "Hello", StartTime: 0.0, EndTime: 0.3, Speaker: "Adam"},
		{Word: "there", StartTime: 0.35, EndTime: 0.6, Speaker: "Adam"},
		{Word: "Hi", StartTime: 0.7, EndTime: 0.9, Speaker: "Bella"},
		{Word: "Adam", StartTime: 0.95, EndTime: 1.2, Speaker: "Bella"},
		{Word: "How", StartTime: 1.3, EndTime: 1.5, Speaker: "Adam"},
		{Word: "are", StartTime: 1.55, EndTime: 1.7, Speaker: "Adam"},
		{Word: "you", StartTime: 1.75, EndTime: 2.0, Speaker: "Adam"},
	}

	speakerColors := map[string]string{
		"Adam":  "#00BFFF",
		"Bella": "#FF69B4",
	}

	subs := gen.GenerateFromTimingsWithColors(timings, speakerColors)

	adamColor := "#00BFFF"
	bellaColor := "#FF69B4"

	expectedColors := []string{adamColor, adamColor, bellaColor, bellaColor, adamColor, adamColor, adamColor}

	for i, sub := range subs {
		if sub.Color != expectedColors[i] {
			t.Errorf("subtitle %d (%s): color = %q, want %q",
				i, sub.Word, sub.Color, expectedColors[i])
		}
	}

	for i := 1; i < len(subs); i++ {
		if subs[i].StartTime < subs[i-1].EndTime {
			t.Errorf("subtitle %d starts before %d ends", i, i-1)
		}
	}
}

func TestBuildSpeakerColors(t *testing.T) {
	voiceMap := map[string]struct {
		id            string
		subtitleColor string
	}{
		"Adam":  {id: "adam-id", subtitleColor: "#00BFFF"},
		"Bella": {id: "bella-id", subtitleColor: "#FF69B4"},
	}

	speakerColors := make(map[string]string)
	for name, voice := range voiceMap {
		speakerColors[name] = voice.subtitleColor
	}

	if speakerColors["Adam"] != "#00BFFF" {
		t.Errorf("Adam color = %q, want #00BFFF", speakerColors["Adam"])
	}
	if speakerColors["Bella"] != "#FF69B4" {
		t.Errorf("Bella color = %q, want #FF69B4", speakerColors["Bella"])
	}
}
