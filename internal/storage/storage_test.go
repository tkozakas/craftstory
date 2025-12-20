package storage

import (
	"context"
	"testing"
)

func TestLocalStorageRandomBackgroundClip(t *testing.T) {
	tests := []struct {
		name    string
		dir     string
		wantErr bool
	}{
		{
			name:    "nonExistentDir",
			dir:     "/nonexistent/dir",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewLocalStorage(tt.dir, "/tmp")
			_, err := s.RandomBackgroundClip(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("RandomBackgroundClip() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLocalStorageSaveAudio(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewLocalStorage(tmpDir, tmpDir)

	data := []byte("fake audio data")
	path, err := s.SaveAudio(data, "test.mp3")
	if err != nil {
		t.Errorf("SaveAudio() error = %v", err)
	}

	if path == "" {
		t.Error("SaveAudio() returned empty path")
	}
}

func TestLocalStorageListBackgroundClips(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewLocalStorage(tmpDir, tmpDir)

	clips, err := s.ListBackgroundClips()
	if err != nil {
		t.Errorf("ListBackgroundClips() error = %v", err)
	}

	if len(clips) != 0 {
		t.Errorf("ListBackgroundClips() = %v, want empty", clips)
	}
}
