package imagesearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	baseURL        = "https://www.googleapis.com/customsearch/v1"
	defaultTimeout = 15 * time.Second
	minImageWidth  = 400
	minImageHeight = 300
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
	Width    int
	Height   int
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
	Width         int    `json:"width"`
	Height        int    `json:"height"`
}

var blockedDomains = []string{
	"lookaside.instagram.com",
	"instagram.com",
	"fbcdn.net",
	"pinterest.com",
	"pinimg.com",
	"tiktok.com",
	"twitter.com",
	"x.com",
	"shutterstock.com",
	"gettyimages.com",
	"alamy.com",
	"dreamstime.com",
	"istockphoto.com",
	"123rf.com",
	"depositphotos.com",
	"stock.adobe.com",
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
	requestCount := count * 3
	if requestCount > 10 {
		requestCount = 10
	}

	params := url.Values{}
	params.Set("key", c.apiKey)
	params.Set("cx", c.engineID)
	params.Set("q", query)
	params.Set("searchType", "image")
	params.Set("num", fmt.Sprintf("%d", requestCount))
	params.Set("safe", "active")
	params.Set("imgSize", "xlarge")
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

	results := make([]SearchResult, 0, count)
	for _, item := range searchResp.Items {
		if isBlockedDomain(item.Link) {
			continue
		}
		if item.Image.Width >= minImageWidth && item.Image.Height >= minImageHeight {
			results = append(results, SearchResult{
				Title:    item.Title,
				ImageURL: item.Link,
				ThumbURL: item.Image.ThumbnailLink,
				Width:    item.Image.Width,
				Height:   item.Image.Height,
			})
			if len(results) >= count {
				break
			}
		}
	}

	if len(results) == 0 {
		for _, item := range searchResp.Items {
			if isBlockedDomain(item.Link) {
				continue
			}
			results = append(results, SearchResult{
				Title:    item.Title,
				ImageURL: item.Link,
				ThumbURL: item.Image.ThumbnailLink,
				Width:    item.Image.Width,
				Height:   item.Image.Height,
			})
			if len(results) >= count {
				break
			}
		}
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

	contentType := resp.Header.Get("Content-Type")
	if !isImageContentType(contentType) {
		return nil, fmt.Errorf("invalid content type: %s", contentType)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read image data: %w", err)
	}

	return data, nil
}

func isBlockedDomain(imageURL string) bool {
	lowerURL := strings.ToLower(imageURL)
	for _, domain := range blockedDomains {
		if strings.Contains(lowerURL, domain) {
			return true
		}
	}
	return false
}

func isImageContentType(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.HasPrefix(ct, "image/")
}
