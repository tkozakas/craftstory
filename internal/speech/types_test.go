package speech

import (
	"testing"
)

func TestEstimateTimingsFromDuration(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		duration    float64
		wantCount   int
		wantFirst   string
		wantLast    string
		wantEndTime float64
	}{
		{
			name:        "simpleText",
			text:        "Hello world",
			duration:    2.0,
			wantCount:   2,
			wantFirst:   "Hello",
			wantLast:    "world",
			wantEndTime: 2.0,
		},
		{
			name:        "singleWord",
			text:        "Hello",
			duration:    1.0,
			wantCount:   1,
			wantFirst:   "Hello",
			wantLast:    "Hello",
			wantEndTime: 1.0,
		},
		{
			name:        "emptyText",
			text:        "",
			duration:    5.0,
			wantCount:   0,
			wantFirst:   "",
			wantLast:    "",
			wantEndTime: 0,
		},
		{
			name:        "longSentence",
			text:        "The quick brown fox jumps over the lazy dog",
			duration:    9.0,
			wantCount:   9,
			wantFirst:   "The",
			wantLast:    "dog",
			wantEndTime: 9.0,
		},
	}

	const epsilon = 0.001

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timings := EstimateTimingsFromDuration(tt.text, tt.duration)

			if len(timings) != tt.wantCount {
				t.Fatalf("got %d timings, want %d", len(timings), tt.wantCount)
			}

			if tt.wantCount == 0 {
				return
			}

			if timings[0].Word != tt.wantFirst {
				t.Errorf("first word = %q, want %q", timings[0].Word, tt.wantFirst)
			}

			if timings[len(timings)-1].Word != tt.wantLast {
				t.Errorf("last word = %q, want %q", timings[len(timings)-1].Word, tt.wantLast)
			}

			lastEndTime := timings[len(timings)-1].EndTime
			if diff := lastEndTime - tt.wantEndTime; diff > epsilon || diff < -epsilon {
				t.Errorf("last end time = %v, want %v", lastEndTime, tt.wantEndTime)
			}

			if timings[0].StartTime != 0 {
				t.Errorf("first start time = %v, want 0", timings[0].StartTime)
			}
		})
	}
}

func TestEstimateTimingsNoOverlap(t *testing.T) {
	text := "The quick brown fox jumps over"
	timings := EstimateTimingsFromDuration(text, 6.0)

	for i := 1; i < len(timings); i++ {
		if timings[i].StartTime < timings[i-1].EndTime {
			t.Errorf("timing %d overlaps with %d: %v < %v",
				i, i-1, timings[i].StartTime, timings[i-1].EndTime)
		}
	}
}

func TestEstimateTimingsContiguous(t *testing.T) {
	text := "Hello world test"
	timings := EstimateTimingsFromDuration(text, 3.0)

	const epsilon = 0.001

	for i := 1; i < len(timings); i++ {
		gap := timings[i].StartTime - timings[i-1].EndTime
		if gap > epsilon {
			t.Errorf("gap between timing %d and %d: %v", i-1, i, gap)
		}
	}
}

func TestEstimateTimingsWordLengthInfluence(t *testing.T) {
	text := "I extraordinary"
	timings := EstimateTimingsFromDuration(text, 2.0)

	if len(timings) != 2 {
		t.Fatalf("got %d timings, want 2", len(timings))
	}

	shortDuration := timings[0].EndTime - timings[0].StartTime
	longDuration := timings[1].EndTime - timings[1].StartTime

	if longDuration <= shortDuration {
		t.Errorf("longer word should have longer duration: short=%v, long=%v",
			shortDuration, longDuration)
	}
}

func TestEstimateAudioDuration(t *testing.T) {
	tests := []struct {
		name       string
		audioBytes int
		wantMin    float64
		wantMax    float64
	}{
		{
			name:       "empty",
			audioBytes: 0,
			wantMin:    0,
			wantMax:    0,
		},
		{
			name:       "smallAudio",
			audioBytes: 16000,
			wantMin:    0.5,
			wantMax:    2.0,
		},
		{
			name:       "mediumAudio",
			audioBytes: 160000,
			wantMin:    5.0,
			wantMax:    20.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			audio := make([]byte, tt.audioBytes)
			duration := EstimateAudioDuration(audio)

			if duration < tt.wantMin || duration > tt.wantMax {
				t.Errorf("duration = %v, want between %v and %v",
					duration, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestEstimateTimings(t *testing.T) {
	text := "Hello world"
	audio := make([]byte, 32000)

	timings := EstimateTimings(text, audio)

	if len(timings) != 2 {
		t.Fatalf("got %d timings, want 2", len(timings))
	}

	if timings[0].Word != "Hello" {
		t.Errorf("first word = %q, want Hello", timings[0].Word)
	}
	if timings[1].Word != "world" {
		t.Errorf("second word = %q, want world", timings[1].Word)
	}

	if timings[0].StartTime != 0 {
		t.Errorf("first start = %v, want 0", timings[0].StartTime)
	}
}

func TestAddPauses(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello. World", "Hello... World"},
		{"What! Now", "What!.. Now"},
		{"Really? Yes", "Really?.. Yes"},
		{"Wait...", "Wait..."},
		{"Normal text", "Normal text"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := AddPauses(tt.input)
			if got != tt.want {
				t.Errorf("AddPauses(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestWordTimingFields(t *testing.T) {
	timing := WordTiming{
		Word:      "test",
		StartTime: 1.5,
		EndTime:   2.0,
		Speaker:   "Adam",
	}

	if timing.Word != "test" {
		t.Errorf("Word = %q, want test", timing.Word)
	}
	if timing.StartTime != 1.5 {
		t.Errorf("StartTime = %v, want 1.5", timing.StartTime)
	}
	if timing.EndTime != 2.0 {
		t.Errorf("EndTime = %v, want 2.0", timing.EndTime)
	}
	if timing.Speaker != "Adam" {
		t.Errorf("Speaker = %q, want Adam", timing.Speaker)
	}
}

func TestSpeechResultFields(t *testing.T) {
	result := SpeechResult{
		Audio: []byte("fake audio"),
		Timings: []WordTiming{
			{Word: "Hello", StartTime: 0, EndTime: 0.5},
		},
	}

	if len(result.Audio) != 10 {
		t.Errorf("Audio length = %d, want 10", len(result.Audio))
	}
	if len(result.Timings) != 1 {
		t.Errorf("Timings length = %d, want 1", len(result.Timings))
	}
}

func TestVoiceConfigFields(t *testing.T) {
	voice := VoiceConfig{
		ID:            "voice-id",
		Name:          "Adam",
		SubtitleColor: "#00BFFF",
	}

	if voice.ID != "voice-id" {
		t.Errorf("ID = %q, want voice-id", voice.ID)
	}
	if voice.Name != "Adam" {
		t.Errorf("Name = %q, want Adam", voice.Name)
	}
	if voice.SubtitleColor != "#00BFFF" {
		t.Errorf("SubtitleColor = %q, want #00BFFF", voice.SubtitleColor)
	}
}

func TestDuration(t *testing.T) {
	tests := []struct {
		name    string
		timings []WordTiming
		want    float64
	}{
		{
			name:    "emptyTimings",
			timings: []WordTiming{},
			want:    0,
		},
		{
			name:    "nilTimings",
			timings: nil,
			want:    0,
		},
		{
			name: "singleWord",
			timings: []WordTiming{
				{Word: "Hello", StartTime: 0, EndTime: 0.5},
			},
			want: 0.5,
		},
		{
			name: "multipleWords",
			timings: []WordTiming{
				{Word: "Hello", StartTime: 0, EndTime: 0.5},
				{Word: "World", StartTime: 0.5, EndTime: 1.0},
				{Word: "Test", StartTime: 1.0, EndTime: 1.5},
			},
			want: 1.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Duration(tt.timings)
			if got != tt.want {
				t.Errorf("Duration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildVoiceMap(t *testing.T) {
	voices := []VoiceConfig{
		{ID: "1", Name: "Alice"},
		{ID: "2", Name: "Bob"},
	}

	m := BuildVoiceMap(voices)

	if len(m) != 2 {
		t.Errorf("BuildVoiceMap() returned %d entries, want 2", len(m))
	}
	if m["Alice"].ID != "1" {
		t.Errorf("BuildVoiceMap()[Alice].ID = %q, want %q", m["Alice"].ID, "1")
	}
	if m["Bob"].ID != "2" {
		t.Errorf("BuildVoiceMap()[Bob].ID = %q, want %q", m["Bob"].ID, "2")
	}
}

func TestBuildSpeakerColors(t *testing.T) {
	voiceMap := map[string]VoiceConfig{
		"Alice": {ID: "1", Name: "Alice", SubtitleColor: "#FF0000"},
		"Bob":   {ID: "2", Name: "Bob", SubtitleColor: "#00FF00"},
		"Carol": {ID: "3", Name: "Carol", SubtitleColor: ""},
	}

	colors := BuildSpeakerColors(voiceMap)

	if len(colors) != 2 {
		t.Errorf("BuildSpeakerColors() returned %d entries, want 2", len(colors))
	}
	if colors["Alice"] != "#FF0000" {
		t.Errorf("BuildSpeakerColors()[Alice] = %q, want #FF0000", colors["Alice"])
	}
	if colors["Bob"] != "#00FF00" {
		t.Errorf("BuildSpeakerColors()[Bob] = %q, want #00FF00", colors["Bob"])
	}
	if _, ok := colors["Carol"]; ok {
		t.Error("BuildSpeakerColors() should not include Carol (empty color)")
	}
}

func TestTimingSyncAcrossConversation(t *testing.T) {
	adamTimings := []WordTiming{
		{Word: "Hello", StartTime: 0.0, EndTime: 0.3, Speaker: "Adam"},
		{Word: "there", StartTime: 0.35, EndTime: 0.6, Speaker: "Adam"},
	}

	bellaTimings := []WordTiming{
		{Word: "Hi", StartTime: 0.0, EndTime: 0.2, Speaker: "Bella"},
		{Word: "Adam", StartTime: 0.25, EndTime: 0.5, Speaker: "Bella"},
	}

	offset := adamTimings[len(adamTimings)-1].EndTime

	var combined []WordTiming
	combined = append(combined, adamTimings...)
	for _, timing := range bellaTimings {
		combined = append(combined, WordTiming{
			Word:      timing.Word,
			StartTime: timing.StartTime + offset,
			EndTime:   timing.EndTime + offset,
			Speaker:   timing.Speaker,
		})
	}

	if len(combined) != 4 {
		t.Fatalf("got %d timings, want 4", len(combined))
	}

	if combined[2].StartTime != 0.6 {
		t.Errorf("Bella's first word start = %v, want 0.6", combined[2].StartTime)
	}

	for i := 1; i < len(combined); i++ {
		if combined[i].StartTime < combined[i-1].EndTime {
			t.Errorf("timing %d overlaps with %d", i, i-1)
		}
	}

	if combined[0].Speaker != "Adam" || combined[1].Speaker != "Adam" {
		t.Error("first two words should be from Adam")
	}
	if combined[2].Speaker != "Bella" || combined[3].Speaker != "Bella" {
		t.Error("last two words should be from Bella")
	}
}
