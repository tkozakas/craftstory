package storage

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
)

type LocalStorage struct {
	backgroundDir string
	outputDir     string
}

func NewLocalStorage(backgroundDir, outputDir string) *LocalStorage {
	return &LocalStorage{
		backgroundDir: backgroundDir,
		outputDir:     outputDir,
	}
}

func (s *LocalStorage) RandomBackgroundClip(ctx context.Context) (string, error) {
	clips, err := s.ListBackgroundClips()
	if err != nil {
		return "", err
	}

	if len(clips) == 0 {
		return "", fmt.Errorf("no video clips found in %s", s.backgroundDir)
	}

	return clips[rand.Intn(len(clips))], nil
}

func (s *LocalStorage) SaveAudio(data []byte, filename string) (string, error) {
	path := filepath.Join(s.outputDir, filename)

	if err := os.MkdirAll(s.outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write audio file: %w", err)
	}

	return path, nil
}

func (s *LocalStorage) ListBackgroundClips() ([]string, error) {
	entries, err := os.ReadDir(s.backgroundDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read background directory: %w", err)
	}

	var clips []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext == ".mp4" || ext == ".mov" || ext == ".mkv" {
			clips = append(clips, filepath.Join(s.backgroundDir, entry.Name()))
		}
	}

	return clips, nil
}

func (s *LocalStorage) EnsureDirectories() error {
	if err := os.MkdirAll(s.backgroundDir, 0755); err != nil {
		return fmt.Errorf("failed to create background directory: %w", err)
	}

	if err := os.MkdirAll(s.outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	return nil
}
