package youtube

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

	"craftstory/internal/distribution"
)

const (
	uploadURL  = "https://www.googleapis.com/upload/youtube/v3/videos"
	videosURL  = "https://www.googleapis.com/youtube/v3/videos"
	categoryID = "22"
	platform   = "youtube"
)

var _ distribution.Uploader = (*Client)(nil)

type Client struct {
	auth *Auth
}

type Auth struct {
	config    *oauth2.Config
	token     *oauth2.Token
	tokenPath string
}

type uploadResponse struct {
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

var scopes = []string{
	"https://www.googleapis.com/auth/youtube.upload",
	"https://www.googleapis.com/auth/youtube",
}

func NewAuth(clientID, clientSecret, tokenPath string) *Auth {
	return &Auth{
		config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     google.Endpoint,
			Scopes:       scopes,
			RedirectURL:  "http://localhost:8080/callback",
		},
		tokenPath: tokenPath,
	}
}

func NewClient(auth *Auth) *Client {
	return &Client{auth: auth}
}

func (c *Client) Upload(ctx context.Context, req distribution.UploadRequest) (*distribution.UploadResponse, error) {
	httpClient, err := c.auth.Client(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get auth client: %w", err)
	}

	metadata := videoMetadata{
		Snippet: videoSnippet{
			Title:       req.Title,
			Description: req.Description,
			Tags:        req.Tags,
			CategoryID:  categoryID,
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

	url := fmt.Sprintf("%s?uploadType=multipart&part=snippet,status", uploadURL)
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

	var uploadResp uploadResponse
	if err := json.Unmarshal(respBody, &uploadResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &distribution.UploadResponse{
		ID:       uploadResp.ID,
		URL:      fmt.Sprintf("https://youtube.com/watch?v=%s", uploadResp.ID),
		Platform: platform,
	}, nil
}

func (c *Client) SetPrivacy(ctx context.Context, videoID, privacy string) error {
	httpClient, err := c.auth.Client(ctx)
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

	url := fmt.Sprintf("%s?part=status", videosURL)
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

func (c *Client) Platform() string {
	return platform
}

func (c *Client) Auth() *Auth {
	return c.auth
}

func (a *Auth) LoadToken() error {
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

func (a *Auth) SaveToken() error {
	data, err := json.MarshalIndent(a.token, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}

	if err := os.WriteFile(a.tokenPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write token file: %w", err)
	}

	return nil
}

func (a *Auth) GetAuthURL() string {
	return a.config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
}

func (a *Auth) Exchange(ctx context.Context, code string) error {
	token, err := a.config.Exchange(ctx, code)
	if err != nil {
		return fmt.Errorf("failed to exchange code: %w", err)
	}

	a.token = token
	return a.SaveToken()
}

func (a *Auth) Client(ctx context.Context) (*http.Client, error) {
	if a.token == nil {
		if err := a.LoadToken(); err != nil {
			return nil, err
		}
	}

	return a.config.Client(ctx, a.token), nil
}

func (a *Auth) IsAuthenticated() bool {
	if a.token == nil {
		if err := a.LoadToken(); err != nil {
			return false
		}
	}
	return a.token != nil && a.token.Valid()
}
