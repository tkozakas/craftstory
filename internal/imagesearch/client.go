package imagesearch

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
	baseURL        = "https://www.googleapis.com/customsearch/v1"
	defaultTimeout = 15 * time.Second
)

type Client struct {
	apiKey     string
	engineID   string
	httpClient *http.Client
	baseURL    string
}

type SearchResult struct {
	Title    string
	ImageURL string
	ThumbURL string
}

type searchResponse struct {
	Items []searchItem `json:"items"`
}

type searchItem struct {
	Title string    `json:"title"`
	Link  string    `json:"link"`
	Image imageInfo `json:"image"`
}

type imageInfo struct {
	ThumbnailLink string `json:"thumbnailLink"`
}

func NewClient(apiKey, engineID string) *Client {
	return &Client{
		apiKey:   apiKey,
		engineID: engineID,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		baseURL: baseURL,
	}
}

func (c *Client) Search(ctx context.Context, query string, count int) ([]SearchResult, error) {
	if count > 10 {
		count = 10
	}

	params := url.Values{}
	params.Set("key", c.apiKey)
	params.Set("cx", c.engineID)
	params.Set("q", query)
	params.Set("searchType", "image")
	params.Set("num", fmt.Sprintf("%d", count))
	params.Set("safe", "active")
	params.Set("imgSize", "large")
	params.Set("imgType", "photo")

	reqURL := fmt.Sprintf("%s?%s", c.baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search api error: %s, body: %s", resp.Status, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var searchResp searchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	results := make([]SearchResult, 0, len(searchResp.Items))
	for _, item := range searchResp.Items {
		results = append(results, SearchResult{
			Title:    item.Title,
			ImageURL: item.Link,
			ThumbURL: item.Image.ThumbnailLink,
		})
	}

	return results, nil
}

func (c *Client) DownloadImage(ctx context.Context, imageURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download image: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download image: %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read image data: %w", err)
	}

	return data, nil
}
