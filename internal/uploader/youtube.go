package uploader

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	youtubeUploadURL  = "https://www.googleapis.com/upload/youtube/v3/videos"
	youtubeVideosURL  = "https://www.googleapis.com/youtube/v3/videos"
	youtubeCategoryID = "22"
	youtubePlatform   = "youtube"
)

type YouTubeUploader struct {
	auth *YouTubeAuth
}

type YouTubeAuth struct {
	config    *oauth2.Config
	token     *oauth2.Token
	tokenPath string
}

type youtubeUploadResponse struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

type videoSnippet struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	CategoryID  string   `json:"categoryId"`
}

type videoStatus struct {
	PrivacyStatus string `json:"privacyStatus"`
}

type videoMetadata struct {
	Snippet videoSnippet `json:"snippet"`
	Status  videoStatus  `json:"status"`
}

var youtubeScopes = []string{
	"https://www.googleapis.com/auth/youtube.upload",
	"https://www.googleapis.com/auth/youtube",
}

func NewYouTubeAuth(clientID, clientSecret, tokenPath string) *YouTubeAuth {
	return &YouTubeAuth{
		config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     google.Endpoint,
			Scopes:       youtubeScopes,
			RedirectURL:  "http://localhost:8080/callback",
		},
		tokenPath: tokenPath,
	}
}

func NewYouTubeUploader(auth *YouTubeAuth) *YouTubeUploader {
	return &YouTubeUploader{auth: auth}
}

func (u *YouTubeUploader) Upload(ctx context.Context, req UploadRequest) (*UploadResponse, error) {
	httpClient, err := u.auth.Client(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get auth client: %w", err)
	}

	metadata := videoMetadata{
		Snippet: videoSnippet{
			Title:       req.Title,
			Description: req.Description,
			Tags:        req.Tags,
			CategoryID:  youtubeCategoryID,
		},
		Status: videoStatus{
			PrivacyStatus: req.Privacy,
		},
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	videoFile, err := os.Open(req.FilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open video file: %w", err)
	}
	defer func() { _ = videoFile.Close() }()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	metadataPart, err := writer.CreateFormField("snippet")
	if err != nil {
		return nil, fmt.Errorf("failed to create metadata part: %w", err)
	}
	if _, err := metadataPart.Write(metadataJSON); err != nil {
		return nil, fmt.Errorf("failed to write metadata: %w", err)
	}

	videoPart, err := writer.CreateFormFile("file", filepath.Base(req.FilePath))
	if err != nil {
		return nil, fmt.Errorf("failed to create video part: %w", err)
	}
	if _, err := io.Copy(videoPart, videoFile); err != nil {
		return nil, fmt.Errorf("failed to copy video: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close writer: %w", err)
	}

	url := fmt.Sprintf("%s?uploadType=multipart&part=snippet,status", youtubeUploadURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to upload video: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upload failed: %s", string(respBody))
	}

	var uploadResp youtubeUploadResponse
	if err := json.Unmarshal(respBody, &uploadResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &UploadResponse{
		ID:       uploadResp.ID,
		URL:      fmt.Sprintf("https://youtube.com/watch?v=%s", uploadResp.ID),
		Platform: youtubePlatform,
	}, nil
}

func (u *YouTubeUploader) SetPrivacy(ctx context.Context, videoID, privacy string) error {
	httpClient, err := u.auth.Client(ctx)
	if err != nil {
		return fmt.Errorf("failed to get auth client: %w", err)
	}

	body := map[string]any{
		"id": videoID,
		"status": map[string]string{
			"privacyStatus": privacy,
		},
	}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal body: %w", err)
	}

	url := fmt.Sprintf("%s?part=status", youtubeVideosURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewBuffer(bodyJSON))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update video: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update failed: %s", string(respBody))
	}

	return nil
}

func (u *YouTubeUploader) Platform() string {
	return youtubePlatform
}

func (u *YouTubeUploader) Auth() *YouTubeAuth {
	return u.auth
}

func (a *YouTubeAuth) LoadToken() error {
	data, err := os.ReadFile(a.tokenPath)
	if err != nil {
		return fmt.Errorf("failed to read token file: %w", err)
	}

	var token oauth2.Token
	if err := json.Unmarshal(data, &token); err != nil {
		return fmt.Errorf("failed to parse token: %w", err)
	}

	a.token = &token
	return nil
}

func (a *YouTubeAuth) SaveToken() error {
	data, err := json.MarshalIndent(a.token, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}

	if err := os.WriteFile(a.tokenPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write token file: %w", err)
	}

	return nil
}

func (a *YouTubeAuth) GetAuthURL() string {
	return a.config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
}

func (a *YouTubeAuth) Exchange(ctx context.Context, code string) error {
	token, err := a.config.Exchange(ctx, code)
	if err != nil {
		return fmt.Errorf("failed to exchange code: %w", err)
	}

	a.token = token
	return a.SaveToken()
}

func (a *YouTubeAuth) Client(ctx context.Context) (*http.Client, error) {
	if a.token == nil {
		if err := a.LoadToken(); err != nil {
			return nil, err
		}
	}

	return a.config.Client(ctx, a.token), nil
}

func (a *YouTubeAuth) IsAuthenticated() bool {
	if a.token == nil {
		if err := a.LoadToken(); err != nil {
			return false
		}
	}
	return a.token != nil && a.token.Valid()
}
