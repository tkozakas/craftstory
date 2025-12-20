package uploader

import "context"

type UploadRequest struct {
	FilePath    string
	Title       string
	Description string
	Tags        []string
	Privacy     string
}

type UploadResponse struct {
	ID       string
	URL      string
	Platform string
}

type Uploader interface {
	Upload(ctx context.Context, req UploadRequest) (*UploadResponse, error)
	SetPrivacy(ctx context.Context, videoID, privacy string) error
	Platform() string
}
