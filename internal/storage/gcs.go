package storage

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

type GCSStorage struct {
	client        *storage.Client
	bucket        string
	backgroundDir string
	localCacheDir string
}

func NewGCSStorage(ctx context.Context, bucket, backgroundDir, localCacheDir string) (*GCSStorage, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS client: %w", err)
	}

	return &GCSStorage{
		client:        client,
		bucket:        bucket,
		backgroundDir: backgroundDir,
		localCacheDir: localCacheDir,
	}, nil
}

func (s *GCSStorage) Close() error {
	return s.client.Close()
}

func (s *GCSStorage) RandomBackgroundClip(ctx context.Context) (string, error) {
	clips, err := s.listBackgroundClips(ctx)
	if err != nil {
		return "", err
	}

	if len(clips) == 0 {
		return "", fmt.Errorf("no video clips found in gs://%s/%s", s.bucket, s.backgroundDir)
	}

	remotePath := clips[rand.Intn(len(clips))]
	localPath := filepath.Join(s.localCacheDir, filepath.Base(remotePath))

	if _, err := os.Stat(localPath); err == nil {
		return localPath, nil
	}

	if err := s.downloadFile(ctx, remotePath, localPath); err != nil {
		return "", fmt.Errorf("failed to download background clip: %w", err)
	}

	return localPath, nil
}

func (s *GCSStorage) listBackgroundClips(ctx context.Context) ([]string, error) {
	bkt := s.client.Bucket(s.bucket)
	query := &storage.Query{Prefix: s.backgroundDir}

	var clips []string
	it := bkt.Objects(ctx, query)
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", err)
		}

		ext := strings.ToLower(filepath.Ext(attrs.Name))
		if ext == ".mp4" || ext == ".mov" || ext == ".mkv" {
			clips = append(clips, attrs.Name)
		}
	}

	return clips, nil
}

func (s *GCSStorage) downloadFile(ctx context.Context, remotePath, localPath string) error {
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	obj := s.client.Bucket(s.bucket).Object(remotePath)
	r, err := obj.NewReader(ctx)
	if err != nil {
		return fmt.Errorf("failed to create reader: %w", err)
	}
	defer func() { _ = r.Close() }()

	f, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}

	return nil
}

func (s *GCSStorage) EnsureCacheDir() error {
	return os.MkdirAll(s.localCacheDir, 0755)
}
