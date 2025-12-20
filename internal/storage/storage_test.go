package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewLocalStorage(t *testing.T) {
	s := NewLocalStorage("/bg", "/out")

	if s == nil {
		t.Fatal("NewLocalStorage() returned nil")
	}

	if s.backgroundDir != "/bg" {
		t.Errorf("backgroundDir = %q, want %q", s.backgroundDir, "/bg")
	}

	if s.outputDir != "/out" {
		t.Errorf("outputDir = %q, want %q", s.outputDir, "/out")
	}
}

func TestLocalStorageRandomBackgroundClip(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(t *testing.T) string
		wantErr   bool
		errMsg    string
	}{
		{
			name: "nonExistentDir",
			setupFunc: func(t *testing.T) string {
				return "/nonexistent/dir"
			},
			wantErr: true,
			errMsg:  "failed to read",
		},
		{
			name: "emptyDir",
			setupFunc: func(t *testing.T) string {
				return t.TempDir()
			},
			wantErr: true,
			errMsg:  "no video clips found",
		},
		{
			name: "withVideoFiles",
			setupFunc: func(t *testing.T) string {
				dir := t.TempDir()
				files := []string{"video1.mp4", "video2.mov", "video3.mkv"}
				for _, f := range files {
					if err := os.WriteFile(filepath.Join(dir, f), []byte("fake"), 0644); err != nil {
						t.Fatal(err)
					}
				}
				return dir
			},
			wantErr: false,
		},
		{
			name: "onlyNonVideoFiles",
			setupFunc: func(t *testing.T) string {
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("text"), 0644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "image.png"), []byte("img"), 0644); err != nil {
					t.Fatal(err)
				}
				return dir
			},
			wantErr: true,
			errMsg:  "no video clips found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setupFunc(t)
			s := NewLocalStorage(dir, "/tmp")

			clip, err := s.RandomBackgroundClip(context.Background())

			if (err != nil) != tt.wantErr {
				t.Errorf("RandomBackgroundClip() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.errMsg)
			}

			if !tt.wantErr && clip == "" {
				t.Error("RandomBackgroundClip() returned empty path")
			}
		})
	}
}

func TestLocalStorageSaveAudio(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) (string, string)
		data     []byte
		filename string
		wantErr  bool
	}{
		{
			name: "successfulSave",
			setup: func(t *testing.T) (string, string) {
				dir := t.TempDir()
				return dir, dir
			},
			data:     []byte("fake audio data"),
			filename: "test.mp3",
			wantErr:  false,
		},
		{
			name: "createsOutputDir",
			setup: func(t *testing.T) (string, string) {
				dir := t.TempDir()
				return dir, filepath.Join(dir, "new_output")
			},
			data:     []byte("audio"),
			filename: "audio.mp3",
			wantErr:  false,
		},
		{
			name: "nestedDir",
			setup: func(t *testing.T) (string, string) {
				dir := t.TempDir()
				return dir, filepath.Join(dir, "a", "b", "c")
			},
			data:     []byte("nested"),
			filename: "nested.mp3",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bgDir, outDir := tt.setup(t)
			s := NewLocalStorage(bgDir, outDir)

			path, err := s.SaveAudio(tt.data, tt.filename)

			if (err != nil) != tt.wantErr {
				t.Errorf("SaveAudio() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if path == "" {
					t.Error("SaveAudio() returned empty path")
				}

				content, err := os.ReadFile(path)
				if err != nil {
					t.Errorf("failed to read saved file: %v", err)
				}

				if string(content) != string(tt.data) {
					t.Errorf("file content = %q, want %q", content, tt.data)
				}
			}
		})
	}
}

func TestLocalStorageListBackgroundClips(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func(t *testing.T) string
		wantCount int
		wantErr   bool
	}{
		{
			name: "emptyDir",
			setupFunc: func(t *testing.T) string {
				return t.TempDir()
			},
			wantCount: 0,
			wantErr:   false,
		},
		{
			name: "nonExistentDir",
			setupFunc: func(t *testing.T) string {
				return "/nonexistent/dir"
			},
			wantCount: 0,
			wantErr:   true,
		},
		{
			name: "mp4Files",
			setupFunc: func(t *testing.T) string {
				dir := t.TempDir()
				for _, f := range []string{"a.mp4", "b.mp4"} {
					if err := os.WriteFile(filepath.Join(dir, f), []byte("mp4"), 0644); err != nil {
						t.Fatal(err)
					}
				}
				return dir
			},
			wantCount: 2,
			wantErr:   false,
		},
		{
			name: "movFiles",
			setupFunc: func(t *testing.T) string {
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "video.mov"), []byte("mov"), 0644); err != nil {
					t.Fatal(err)
				}
				return dir
			},
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "mkvFiles",
			setupFunc: func(t *testing.T) string {
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "video.mkv"), []byte("mkv"), 0644); err != nil {
					t.Fatal(err)
				}
				return dir
			},
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "mixedFiles",
			setupFunc: func(t *testing.T) string {
				dir := t.TempDir()
				files := []string{"a.mp4", "b.mov", "c.mkv", "d.txt", "e.jpg"}
				for _, f := range files {
					if err := os.WriteFile(filepath.Join(dir, f), []byte("data"), 0644); err != nil {
						t.Fatal(err)
					}
				}
				return dir
			},
			wantCount: 3,
			wantErr:   false,
		},
		{
			name: "skipsSubdirs",
			setupFunc: func(t *testing.T) string {
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, "video.mp4"), []byte("mp4"), 0644); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(filepath.Join(dir, "subdir"), 0755); err != nil {
					t.Fatal(err)
				}
				return dir
			},
			wantCount: 1,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setupFunc(t)
			s := NewLocalStorage(dir, "/tmp")

			clips, err := s.ListBackgroundClips()

			if (err != nil) != tt.wantErr {
				t.Errorf("ListBackgroundClips() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(clips) != tt.wantCount {
				t.Errorf("ListBackgroundClips() returned %d clips, want %d", len(clips), tt.wantCount)
			}
		})
	}
}

func TestLocalStorageEnsureDirectories(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) (string, string)
		wantErr bool
	}{
		{
			name: "createsNewDirs",
			setup: func(t *testing.T) (string, string) {
				base := t.TempDir()
				return filepath.Join(base, "bg"), filepath.Join(base, "out")
			},
			wantErr: false,
		},
		{
			name: "existingDirs",
			setup: func(t *testing.T) (string, string) {
				dir := t.TempDir()
				return dir, dir
			},
			wantErr: false,
		},
		{
			name: "nestedDirs",
			setup: func(t *testing.T) (string, string) {
				base := t.TempDir()
				return filepath.Join(base, "a", "b", "bg"), filepath.Join(base, "x", "y", "out")
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bgDir, outDir := tt.setup(t)
			s := NewLocalStorage(bgDir, outDir)

			err := s.EnsureDirectories()

			if (err != nil) != tt.wantErr {
				t.Errorf("EnsureDirectories() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if _, err := os.Stat(bgDir); os.IsNotExist(err) {
					t.Errorf("background dir %q was not created", bgDir)
				}
				if _, err := os.Stat(outDir); os.IsNotExist(err) {
					t.Errorf("output dir %q was not created", outDir)
				}
			}
		})
	}
}
