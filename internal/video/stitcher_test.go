package video

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"craftstory/internal/elevenlabs"
)

func TestNewAudioStitcher(t *testing.T) {
	stitcher := NewAudioStitcher("/tmp/test")

	if stitcher.tempDir != "/tmp/test" {
		t.Errorf("tempDir = %q, want %q", stitcher.tempDir, "/tmp/test")
	}
	if stitcher.ffmpegPath != "ffmpeg" {
		t.Errorf("ffmpegPath = %q, want %q", stitcher.ffmpegPath, "ffmpeg")
	}
}

func TestAdjustTimings(t *testing.T) {
	stitcher := NewAudioStitcher("/tmp")

	tests := []struct {
		name         string
		segments     []AudioSegment
		wantTimings  int
		wantDuration float64
		wantFirst    elevenlabs.WordTiming
		wantLast     elevenlabs.WordTiming
	}{
		{
			name: "singleSegment",
			segments: []AudioSegment{
				{
					Timings: []elevenlabs.WordTiming{
						{Word: "Hello", StartTime: 0, EndTime: 0.5},
						{Word: "World", StartTime: 0.5, EndTime: 1.0},
					},
				},
			},
			wantTimings:  2,
			wantDuration: 1.0,
			wantFirst:    elevenlabs.WordTiming{Word: "Hello", StartTime: 0, EndTime: 0.5},
			wantLast:     elevenlabs.WordTiming{Word: "World", StartTime: 0.5, EndTime: 1.0},
		},
		{
			name: "twoSegments",
			segments: []AudioSegment{
				{
					Timings: []elevenlabs.WordTiming{
						{Word: "First", StartTime: 0, EndTime: 0.5},
						{Word: "Part", StartTime: 0.5, EndTime: 1.0},
					},
				},
				{
					Timings: []elevenlabs.WordTiming{
						{Word: "Second", StartTime: 0, EndTime: 0.5},
						{Word: "Part", StartTime: 0.5, EndTime: 1.0},
					},
				},
			},
			wantTimings:  4,
			wantDuration: 2.0,
			wantFirst:    elevenlabs.WordTiming{Word: "First", StartTime: 0, EndTime: 0.5},
			wantLast:     elevenlabs.WordTiming{Word: "Part", StartTime: 1.5, EndTime: 2.0},
		},
		{
			name: "threeSegments",
			segments: []AudioSegment{
				{Timings: []elevenlabs.WordTiming{{Word: "A", StartTime: 0, EndTime: 1.0}}},
				{Timings: []elevenlabs.WordTiming{{Word: "B", StartTime: 0, EndTime: 1.0}}},
				{Timings: []elevenlabs.WordTiming{{Word: "C", StartTime: 0, EndTime: 1.0}}},
			},
			wantTimings:  3,
			wantDuration: 3.0,
			wantFirst:    elevenlabs.WordTiming{Word: "A", StartTime: 0, EndTime: 1.0},
			wantLast:     elevenlabs.WordTiming{Word: "C", StartTime: 2.0, EndTime: 3.0},
		},
		{
			name:         "emptySegments",
			segments:     []AudioSegment{{Timings: []elevenlabs.WordTiming{}}},
			wantTimings:  0,
			wantDuration: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timings, duration := stitcher.adjustTimings(tt.segments)

			if len(timings) != tt.wantTimings {
				t.Errorf("adjustTimings() got %d timings, want %d", len(timings), tt.wantTimings)
			}

			if duration != tt.wantDuration {
				t.Errorf("adjustTimings() duration = %v, want %v", duration, tt.wantDuration)
			}

			if tt.wantTimings > 0 {
				if timings[0] != tt.wantFirst {
					t.Errorf("first timing = %+v, want %+v", timings[0], tt.wantFirst)
				}
				if timings[len(timings)-1] != tt.wantLast {
					t.Errorf("last timing = %+v, want %+v", timings[len(timings)-1], tt.wantLast)
				}
			}
		})
	}
}

func TestStitchEmptySegments(t *testing.T) {
	stitcher := NewAudioStitcher("/tmp")

	_, err := stitcher.Stitch(t.Context(), []AudioSegment{})
	if err == nil {
		t.Error("expected error for empty segments")
	}
}

func TestStitchSingleSegment(t *testing.T) {
	stitcher := NewAudioStitcher("/tmp")

	segment := AudioSegment{
		Audio: []byte("fake audio data"),
		Timings: []elevenlabs.WordTiming{
			{Word: "Test", StartTime: 0, EndTime: 1.0},
		},
	}

	result, err := stitcher.Stitch(t.Context(), []AudioSegment{segment})
	if err != nil {
		t.Fatalf("Stitch() error = %v", err)
	}

	if string(result.Data) != "fake audio data" {
		t.Error("single segment should return original data")
	}
	if result.Duration != 1.0 {
		t.Errorf("Duration = %v, want 1.0", result.Duration)
	}
	if len(result.Timings) != 1 {
		t.Errorf("got %d timings, want 1", len(result.Timings))
	}
}

func TestStitchSingleSegmentNoTimings(t *testing.T) {
	stitcher := NewAudioStitcher("/tmp")

	segment := AudioSegment{
		Audio:   []byte("fake audio data"),
		Timings: []elevenlabs.WordTiming{},
	}

	result, err := stitcher.Stitch(t.Context(), []AudioSegment{segment})
	if err != nil {
		t.Fatalf("Stitch() error = %v", err)
	}

	if result.Duration != 0 {
		t.Errorf("Duration = %v, want 0", result.Duration)
	}
}

func TestStitchMultipleSegmentsWithFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}

	tmpDir := t.TempDir()
	stitcher := NewAudioStitcher(tmpDir)

	silentMP3 := createSilentMP3(t)

	segments := []AudioSegment{
		{
			Audio:   silentMP3,
			Timings: []elevenlabs.WordTiming{{Word: "Hello", StartTime: 0, EndTime: 0.1}},
		},
		{
			Audio:   silentMP3,
			Timings: []elevenlabs.WordTiming{{Word: "World", StartTime: 0, EndTime: 0.1}},
		},
	}

	result, err := stitcher.Stitch(context.Background(), segments)
	if err != nil {
		t.Fatalf("Stitch() error = %v", err)
	}

	if len(result.Data) == 0 {
		t.Error("expected non-empty audio data")
	}
	if len(result.Timings) != 2 {
		t.Errorf("got %d timings, want 2", len(result.Timings))
	}
	if result.Timings[0].Word != "Hello" {
		t.Errorf("first word = %q, want %q", result.Timings[0].Word, "Hello")
	}
	if result.Timings[1].Word != "World" {
		t.Errorf("second word = %q, want %q", result.Timings[1].Word, "World")
	}
	if result.Timings[1].StartTime != 0.1 {
		t.Errorf("second word start = %v, want 0.1", result.Timings[1].StartTime)
	}
}

func TestStitchWriteSegmentError(t *testing.T) {
	stitcher := NewAudioStitcher("/nonexistent/directory")

	segments := []AudioSegment{
		{Audio: []byte("data1"), Timings: []elevenlabs.WordTiming{{Word: "A", StartTime: 0, EndTime: 1}}},
		{Audio: []byte("data2"), Timings: []elevenlabs.WordTiming{{Word: "B", StartTime: 0, EndTime: 1}}},
	}

	_, err := stitcher.Stitch(context.Background(), segments)
	if err == nil {
		t.Error("expected error for invalid directory")
	}
}

func TestStitchFFmpegError(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}

	tmpDir := t.TempDir()
	stitcher := NewAudioStitcher(tmpDir)

	segments := []AudioSegment{
		{Audio: []byte("not valid mp3"), Timings: []elevenlabs.WordTiming{{Word: "A", StartTime: 0, EndTime: 1}}},
		{Audio: []byte("also invalid"), Timings: []elevenlabs.WordTiming{{Word: "B", StartTime: 0, EndTime: 1}}},
	}

	_, err := stitcher.Stitch(context.Background(), segments)
	if err == nil {
		t.Error("expected error for invalid audio data")
	}
}

func TestStitchUsesAbsolutePaths(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}

	tmpDir := t.TempDir()
	relativeDir := filepath.Base(tmpDir)

	originalWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(originalWd) }()

	if err := os.Chdir(filepath.Dir(tmpDir)); err != nil {
		t.Fatal(err)
	}

	stitcher := NewAudioStitcher(relativeDir)
	silentMP3 := createSilentMP3(t)

	segments := []AudioSegment{
		{Audio: silentMP3, Timings: []elevenlabs.WordTiming{{Word: "A", StartTime: 0, EndTime: 0.1}}},
		{Audio: silentMP3, Timings: []elevenlabs.WordTiming{{Word: "B", StartTime: 0, EndTime: 0.1}}},
	}

	result, err := stitcher.Stitch(context.Background(), segments)
	if err != nil {
		t.Fatalf("Stitch() with relative path error = %v", err)
	}

	if len(result.Data) == 0 {
		t.Error("expected non-empty audio data")
	}
}

func createSilentMP3(t *testing.T) []byte {
	t.Helper()

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "silent.mp3")

	cmd := exec.Command("ffmpeg",
		"-f", "lavfi",
		"-i", "anullsrc=r=44100:cl=mono",
		"-t", "0.1",
		"-q:a", "9",
		outputPath,
	)
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to create silent mp3: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read silent mp3: %v", err)
	}

	return data
}
