package video

import (
	"strings"
	"testing"
)

func TestNewAssembler(t *testing.T) {
	subGen := NewSubtitleGenerator(SubtitleOptions{FontName: "Arial", FontSize: 48})
	assembler := NewAssembler("/output", subGen, nil)

	if assembler.outputDir != "/output" {
		t.Errorf("outputDir = %q, want %q", assembler.outputDir, "/output")
	}
	if assembler.ffmpegPath != "ffmpeg" {
		t.Errorf("ffmpegPath = %q, want %q", assembler.ffmpegPath, "ffmpeg")
	}
	if assembler.ffprobe != "ffprobe" {
		t.Errorf("ffprobe = %q, want %q", assembler.ffprobe, "ffprobe")
	}
	if assembler.subtitleGen != subGen {
		t.Error("subtitleGen not set correctly")
	}
}

func TestBuildFilterComplex(t *testing.T) {
	subGen := NewSubtitleGenerator(SubtitleOptions{FontName: "Arial", FontSize: 48})
	assembler := NewAssembler("/output", subGen, nil)

	tests := []struct {
		name            string
		assPath         string
		overlays        []ImageOverlay
		musicPath       string
		duration        float64
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:      "noOverlaysNoMusic",
			assPath:   "/tmp/subs.ass",
			overlays:  nil,
			musicPath: "",
			duration:  30.0,
			wantContains: []string{
				"scale=1080:1920",
				"crop=1080:1920",
				"ass=/tmp/subs.ass[v]",
				"volume=0.1",
				"amix=inputs=2",
				"duration=longest",
			},
			wantNotContains: []string{
				"overlay",
			},
		},
		{
			name:      "singleOverlayNoMusic",
			assPath:   "/tmp/subs.ass",
			musicPath: "",
			duration:  30.0,
			overlays: []ImageOverlay{
				{ImagePath: "/tmp/img1.png", StartTime: 1.0, EndTime: 3.0, Width: 400, Height: 300},
			},
			wantContains: []string{
				"scale=1080:1920",
				"crop=1080:1920",
				"ass=/tmp/subs.ass[base]",
				"[2:v]scale=400:300",
				"overlay",
				"enable='between(t,1.00,3.00)'",
				"[v]",
			},
		},
		{
			name:      "multipleOverlaysNoMusic",
			assPath:   "/tmp/subs.ass",
			musicPath: "",
			duration:  30.0,
			overlays: []ImageOverlay{
				{ImagePath: "/tmp/img1.png", StartTime: 1.0, EndTime: 2.0, Width: 400, Height: 300},
				{ImagePath: "/tmp/img2.png", StartTime: 3.0, EndTime: 4.0, Width: 500, Height: 400},
			},
			wantContains: []string{
				"[2:v]scale=400:300",
				"[3:v]scale=500:400",
				"enable='between(t,1.00,2.00)'",
				"enable='between(t,3.00,4.00)'",
			},
		},
		{
			name:      "withMusic",
			assPath:   "/tmp/subs.ass",
			overlays:  nil,
			musicPath: "/music/track.mp3",
			duration:  30.0,
			wantContains: []string{
				"amix=inputs=3",
				"afade=t=in",
				"afade=t=out",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := assembler.buildFilterComplex(tt.assPath, tt.overlays, tt.musicPath, tt.duration)

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("buildFilterComplex() missing %q\ngot: %s", want, result)
				}
			}

			for _, notWant := range tt.wantNotContains {
				if strings.Contains(result, notWant) {
					t.Errorf("buildFilterComplex() should not contain %q\ngot: %s", notWant, result)
				}
			}
		})
	}
}

func TestBuildFFmpegArgs(t *testing.T) {
	subGen := NewSubtitleGenerator(SubtitleOptions{FontName: "Arial", FontSize: 48})
	assembler := NewAssembler("/output", subGen, nil)

	tests := []struct {
		name         string
		bgClip       string
		audioPath    string
		musicPath    string
		startTime    float64
		duration     float64
		overlays     []ImageOverlay
		wantContains []string
	}{
		{
			name:      "basicArgs",
			bgClip:    "/bg/video.mp4",
			audioPath: "/audio/voice.mp3",
			musicPath: "",
			startTime: 5.0,
			duration:  30.0,
			overlays:  nil,
			wantContains: []string{
				"-y",
				"-ss", "5.00",
				"-t", "31.50", // 30.0 + 1.5 buffer
				"-i", "/bg/video.mp4",
				"-i", "/audio/voice.mp3",
				"-map", "[v]",
				"-map", "[a]",
				"-c:v", "libx264",
				"-c:a", "aac",
			},
		},
		{
			name:      "withOverlays",
			bgClip:    "/bg/video.mp4",
			audioPath: "/audio/voice.mp3",
			musicPath: "",
			startTime: 0,
			duration:  10.0,
			overlays: []ImageOverlay{
				{ImagePath: "/img/overlay1.png"},
				{ImagePath: "/img/overlay2.png"},
			},
			wantContains: []string{
				"-i", "/img/overlay1.png",
				"-i", "/img/overlay2.png",
			},
		},
		{
			name:      "withMusic",
			bgClip:    "/bg/video.mp4",
			audioPath: "/audio/voice.mp3",
			musicPath: "/music/track.mp3",
			startTime: 0,
			duration:  30.0,
			overlays:  nil,
			wantContains: []string{
				"-i", "/music/track.mp3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filterComplex := assembler.buildFilterComplex("/tmp/subs.ass", tt.overlays, tt.musicPath, tt.duration)
			args := assembler.buildFFmpegArgs(
				tt.bgClip, tt.audioPath, tt.musicPath, tt.startTime, tt.duration,
				filterComplex, tt.overlays, "/output/out.mp4",
			)

			argsStr := strings.Join(args, " ")
			for _, want := range tt.wantContains {
				if !strings.Contains(argsStr, want) {
					t.Errorf("buildFFmpegArgs() missing %q\ngot: %v", want, args)
				}
			}
		})
	}
}

func TestRandomStartTime(t *testing.T) {
	subGen := NewSubtitleGenerator(SubtitleOptions{FontName: "Arial", FontSize: 48})
	assembler := NewAssembler("/output", subGen, nil)

	tests := []struct {
		name           string
		clipDuration   float64
		neededDuration float64
		wantZero       bool
	}{
		{
			name:           "clipShorterThanNeeded",
			clipDuration:   10.0,
			neededDuration: 20.0,
			wantZero:       true,
		},
		{
			name:           "clipEqualToNeeded",
			clipDuration:   10.0,
			neededDuration: 10.0,
			wantZero:       true,
		},
		{
			name:           "clipLongerThanNeeded",
			clipDuration:   60.0,
			neededDuration: 30.0,
			wantZero:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := 0; i < 10; i++ {
				result := assembler.randomStartTime(tt.clipDuration, tt.neededDuration)

				if tt.wantZero && result != 0 {
					t.Errorf("randomStartTime() = %v, want 0", result)
				}

				if !tt.wantZero {
					maxStart := tt.clipDuration - tt.neededDuration
					if result < 0 || result > maxStart {
						t.Errorf("randomStartTime() = %v, want 0 <= x <= %v", result, maxStart)
					}
				}
			}
		})
	}
}

func TestImageOverlayStruct(t *testing.T) {
	overlay := ImageOverlay{
		ImagePath: "/path/to/image.png",
		StartTime: 1.5,
		EndTime:   4.5,
		Width:     400,
		Height:    300,
	}

	if overlay.ImagePath != "/path/to/image.png" {
		t.Errorf("ImagePath = %q, want %q", overlay.ImagePath, "/path/to/image.png")
	}
	if overlay.StartTime != 1.5 {
		t.Errorf("StartTime = %v, want %v", overlay.StartTime, 1.5)
	}
	if overlay.EndTime != 4.5 {
		t.Errorf("EndTime = %v, want %v", overlay.EndTime, 4.5)
	}
	if overlay.Width != 400 {
		t.Errorf("Width = %v, want %v", overlay.Width, 400)
	}
	if overlay.Height != 300 {
		t.Errorf("Height = %v, want %v", overlay.Height, 300)
	}
}

func TestAssembleRequestStruct(t *testing.T) {
	req := AssembleRequest{
		AudioPath:     "/audio/test.mp3",
		AudioDuration: 30.5,
		Script:        "Hello world",
		OutputPath:    "/output/video.mp4",
		ImageOverlays: []ImageOverlay{
			{ImagePath: "/img/test.png"},
		},
	}

	if req.AudioPath != "/audio/test.mp3" {
		t.Errorf("AudioPath = %q, want %q", req.AudioPath, "/audio/test.mp3")
	}
	if req.AudioDuration != 30.5 {
		t.Errorf("AudioDuration = %v, want %v", req.AudioDuration, 30.5)
	}
	if req.Script != "Hello world" {
		t.Errorf("Script = %q, want %q", req.Script, "Hello world")
	}
	if req.OutputPath != "/output/video.mp4" {
		t.Errorf("OutputPath = %q, want %q", req.OutputPath, "/output/video.mp4")
	}
	if len(req.ImageOverlays) != 1 {
		t.Errorf("ImageOverlays len = %d, want 1", len(req.ImageOverlays))
	}
}

func TestParseResolution(t *testing.T) {
	tests := []struct {
		name       string
		resolution string
		wantWidth  int
		wantHeight int
	}{
		{
			name:       "validVertical",
			resolution: "1080x1920",
			wantWidth:  1080,
			wantHeight: 1920,
		},
		{
			name:       "validHorizontal",
			resolution: "1920x1080",
			wantWidth:  1920,
			wantHeight: 1080,
		},
		{
			name:       "invalidFormat",
			resolution: "1080-1920",
			wantWidth:  1080,
			wantHeight: 1920,
		},
		{
			name:       "emptyString",
			resolution: "",
			wantWidth:  1080,
			wantHeight: 1920,
		},
		{
			name:       "invalidNumbers",
			resolution: "abcxdef",
			wantWidth:  1080,
			wantHeight: 1920,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotWidth, gotHeight := parseResolution(tt.resolution)
			if gotWidth != tt.wantWidth {
				t.Errorf("parseResolution() width = %v, want %v", gotWidth, tt.wantWidth)
			}
			if gotHeight != tt.wantHeight {
				t.Errorf("parseResolution() height = %v, want %v", gotHeight, tt.wantHeight)
			}
		})
	}
}

func TestNewAssemblerWithOptions(t *testing.T) {
	subGen := NewSubtitleGenerator(SubtitleOptions{FontName: "Arial", FontSize: 48})
	assembler := NewAssemblerWithOptions(AssemblerOptions{
		OutputDir:   "/output",
		Resolution:  "720x1280",
		SubtitleGen: subGen,
		BgProvider:  nil,
	})

	if assembler.outputDir != "/output" {
		t.Errorf("outputDir = %q, want %q", assembler.outputDir, "/output")
	}
	if assembler.width != 720 {
		t.Errorf("width = %d, want %d", assembler.width, 720)
	}
	if assembler.height != 1280 {
		t.Errorf("height = %d, want %d", assembler.height, 1280)
	}
}

func TestNewAssemblerWithMusicOptions(t *testing.T) {
	subGen := NewSubtitleGenerator(SubtitleOptions{FontName: "Arial", FontSize: 48})
	assembler := NewAssemblerWithOptions(AssemblerOptions{
		OutputDir:    "/output",
		Resolution:   "1080x1920",
		SubtitleGen:  subGen,
		BgProvider:   nil,
		MusicDir:     "/music",
		MusicVolume:  0.2,
		MusicFadeIn:  1.5,
		MusicFadeOut: 2.5,
	})

	if assembler.musicDir != "/music" {
		t.Errorf("musicDir = %q, want %q", assembler.musicDir, "/music")
	}
	if assembler.musicVolume != 0.2 {
		t.Errorf("musicVolume = %v, want %v", assembler.musicVolume, 0.2)
	}
	if assembler.musicFadeIn != 1.5 {
		t.Errorf("musicFadeIn = %v, want %v", assembler.musicFadeIn, 1.5)
	}
	if assembler.musicFadeOut != 2.5 {
		t.Errorf("musicFadeOut = %v, want %v", assembler.musicFadeOut, 2.5)
	}
}

func TestNewAssemblerWithIntroOutroOptions(t *testing.T) {
	subGen := NewSubtitleGenerator(SubtitleOptions{FontName: "Arial", FontSize: 48})
	assembler := NewAssemblerWithOptions(AssemblerOptions{
		OutputDir:     "/output",
		Resolution:    "1080x1920",
		SubtitleGen:   subGen,
		BgProvider:    nil,
		IntroPath:     "/assets/intro.mp4",
		OutroPath:     "/assets/outro.mp4",
		IntroDuration: 3.0,
		OutroDuration: 5.0,
	})

	if assembler.introPath != "/assets/intro.mp4" {
		t.Errorf("introPath = %q, want %q", assembler.introPath, "/assets/intro.mp4")
	}
	if assembler.outroPath != "/assets/outro.mp4" {
		t.Errorf("outroPath = %q, want %q", assembler.outroPath, "/assets/outro.mp4")
	}
	if assembler.introDuration != 3.0 {
		t.Errorf("introDuration = %v, want %v", assembler.introDuration, 3.0)
	}
	if assembler.outroDuration != 5.0 {
		t.Errorf("outroDuration = %v, want %v", assembler.outroDuration, 5.0)
	}
}

func TestBuildAudioFilter(t *testing.T) {
	subGen := NewSubtitleGenerator(SubtitleOptions{FontName: "Arial", FontSize: 48})
	assembler := NewAssemblerWithOptions(AssemblerOptions{
		OutputDir:    "/output",
		Resolution:   "1080x1920",
		SubtitleGen:  subGen,
		MusicVolume:  0.15,
		MusicFadeIn:  1.0,
		MusicFadeOut: 2.0,
	})

	tests := []struct {
		name         string
		musicPath    string
		duration     float64
		wantContains []string
	}{
		{
			name:      "noMusic",
			musicPath: "",
			duration:  30.0,
			wantContains: []string{
				"amix=inputs=2",
				"volume=0.1[bga]",
				"volume=1.0[voice]",
			},
		},
		{
			name:      "withMusic",
			musicPath: "/music/track.mp3",
			duration:  30.0,
			wantContains: []string{
				"amix=inputs=3",
				"volume=0.15",
				"afade=t=in:st=0:d=1.00",
				"afade=t=out:st=28.00:d=2.00",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := assembler.buildAudioFilter(tt.musicPath, tt.duration)
			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("buildAudioFilter() missing %q\ngot: %s", want, result)
				}
			}
		})
	}
}

func TestImageInputIndex(t *testing.T) {
	subGen := NewSubtitleGenerator(SubtitleOptions{FontName: "Arial", FontSize: 48})
	assembler := NewAssembler("/output", subGen, nil)

	tests := []struct {
		name      string
		imageIdx  int
		musicPath string
		want      int
	}{
		{
			name:      "firstImageNoMusic",
			imageIdx:  0,
			musicPath: "",
			want:      2,
		},
		{
			name:      "secondImageNoMusic",
			imageIdx:  1,
			musicPath: "",
			want:      3,
		},
		{
			name:      "firstImageWithMusic",
			imageIdx:  0,
			musicPath: "/music/track.mp3",
			want:      3,
		},
		{
			name:      "secondImageWithMusic",
			imageIdx:  1,
			musicPath: "/music/track.mp3",
			want:      4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := assembler.imageInputIndex(tt.imageIdx, tt.musicPath)
			if got != tt.want {
				t.Errorf("imageInputIndex() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSelectMusicTrack(t *testing.T) {
	subGen := NewSubtitleGenerator(SubtitleOptions{FontName: "Arial", FontSize: 48})

	t.Run("noMusicDir", func(t *testing.T) {
		assembler := NewAssemblerWithOptions(AssemblerOptions{
			OutputDir:   "/output",
			Resolution:  "1080x1920",
			SubtitleGen: subGen,
			MusicDir:    "",
		})
		result := assembler.selectMusicTrack()
		if result != "" {
			t.Errorf("selectMusicTrack() = %q, want empty string", result)
		}
	})

	t.Run("nonExistentDir", func(t *testing.T) {
		assembler := NewAssemblerWithOptions(AssemblerOptions{
			OutputDir:   "/output",
			Resolution:  "1080x1920",
			SubtitleGen: subGen,
			MusicDir:    "/nonexistent/path",
		})
		result := assembler.selectMusicTrack()
		if result != "" {
			t.Errorf("selectMusicTrack() = %q, want empty string", result)
		}
	})
}
