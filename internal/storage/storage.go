package storage

import "context"

type BackgroundProvider interface {
	RandomBackgroundClip(ctx context.Context) (string, error)
}
