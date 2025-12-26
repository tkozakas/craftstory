package tenor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	baseURL        = "https://tenor.googleapis.com/v2"
	defaultTimeout = 15 * time.Second
	defaultLimit   = 10
)

type Client struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

type Config struct {
	APIKey  string
	Timeout time.Duration
}

type GIF struct {
	ID          string
	Title       string
	URL         string
	PreviewURL  string
	Width       int
	Height      int
	ContentType string
}

type searchResponse struct {
	Results []result `json:"results"`
	Next    string   `json:"next"`
}

type result struct {
	ID           string                 `json:"id"`
	Title        string                 `json:"title"`
	MediaFormats map[string]mediaFormat `json:"media_formats"`
	Created      float64                `json:"created"`
}

type mediaFormat struct {
	URL      string  `json:"url"`
	Duration float64 `json:"duration"`
	Dims     []int   `json:"dims"`
	Size     int     `json:"size"`
}

func NewClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	return &Client{
		apiKey:  cfg.APIKey,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) Search(ctx context.Context, query string, limit int) ([]GIF, error) {
	if limit <= 0 {
		limit = defaultLimit
	}

	reqURL := c.buildSearchURL(query, limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	return c.parseSearchResponse(resp.Body)
}

func (c *Client) Download(ctx context.Context, gifURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, gifURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download gif: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed: %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return data, nil
}

func (c *Client) buildSearchURL(query string, limit int) string {
	params := url.Values{}
	params.Set("key", c.apiKey)
	params.Set("q", query)
	params.Set("limit", fmt.Sprintf("%d", limit))
	params.Set("media_filter", "gif,tinygif")
	params.Set("contentfilter", "medium")

	return fmt.Sprintf("%s/search?%s", c.baseURL, params.Encode())
}

func (c *Client) parseSearchResponse(body io.Reader) ([]GIF, error) {
	var resp searchResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	gifs := make([]GIF, 0, len(resp.Results))
	for _, r := range resp.Results {
		gif := c.toGIF(r)
		if gif != nil {
			gifs = append(gifs, *gif)
		}
	}

	return gifs, nil
}

func (c *Client) toGIF(r result) *GIF {
	media := selectMedia(r.MediaFormats)
	if media == nil {
		return nil
	}

	preview := selectPreview(r.MediaFormats)

	return &GIF{
		ID:          r.ID,
		Title:       r.Title,
		URL:         media.URL,
		PreviewURL:  preview,
		Width:       media.Dims[0],
		Height:      media.Dims[1],
		ContentType: "image/gif",
	}
}

func (c *Client) parseError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("tenor api error: %s, body: %s", resp.Status, string(body))
}

func selectMedia(formats map[string]mediaFormat) *mediaFormat {
	priorities := []string{"gif", "tinygif"}
	for _, key := range priorities {
		if m, ok := formats[key]; ok && len(m.Dims) >= 2 {
			return &m
		}
	}
	return nil
}

func selectPreview(formats map[string]mediaFormat) string {
	priorities := []string{"tinygif", "nanogif", "gif"}
	for _, key := range priorities {
		if m, ok := formats[key]; ok {
			return m.URL
		}
	}
	return ""
}
