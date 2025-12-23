package visuals

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
	googleSearchURL    = "https://www.googleapis.com/customsearch/v1"
	searchTimeout      = 15 * time.Second
	minSearchImgWidth  = 400
	minSearchImgHeight = 300
)

type SearchClient struct {
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

func NewSearchClient(apiKey, engineID string) *SearchClient {
	return &SearchClient{
		apiKey:   apiKey,
		engineID: engineID,
		httpClient: &http.Client{
			Timeout: searchTimeout,
		},
		baseURL: googleSearchURL,
	}
}

func (c *SearchClient) Search(ctx context.Context, query string, count int) ([]SearchResult, error) {
	return c.search(ctx, query, count, "photo")
}

func (c *SearchClient) SearchGif(ctx context.Context, query string, count int) ([]SearchResult, error) {
	return c.search(ctx, query, count, "animated")
}

func (c *SearchClient) search(ctx context.Context, query string, count int, imgType string) ([]SearchResult, error) {
	reqURL := c.buildSearchURL(query, count, imgType)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search api error: %s, body: %s", resp.Status, string(body))
	}

	return c.parseSearchResponse(resp.Body, count)
}

func (c *SearchClient) buildSearchURL(query string, count int, imgType string) string {
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
	params.Set("imgType", imgType)

	return fmt.Sprintf("%s?%s", c.baseURL, params.Encode())
}

func (c *SearchClient) parseSearchResponse(body io.Reader, count int) ([]SearchResult, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var searchResp searchResponse
	if err := json.Unmarshal(data, &searchResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	results := filterResults(searchResp.Items, count)
	if len(results) == 0 {
		results = filterResultsNoSize(searchResp.Items, count)
	}

	return results, nil
}

func filterResults(items []searchItem, count int) []SearchResult {
	results := make([]SearchResult, 0, count)
	for _, item := range items {
		if isBlockedDomain(item.Link) {
			continue
		}
		if item.Image.Width < minSearchImgWidth || item.Image.Height < minSearchImgHeight {
			continue
		}
		results = append(results, toSearchResult(item))
		if len(results) >= count {
			break
		}
	}
	return results
}

func filterResultsNoSize(items []searchItem, count int) []SearchResult {
	results := make([]SearchResult, 0, count)
	for _, item := range items {
		if isBlockedDomain(item.Link) {
			continue
		}
		results = append(results, toSearchResult(item))
		if len(results) >= count {
			break
		}
	}
	return results
}

func toSearchResult(item searchItem) SearchResult {
	return SearchResult{
		Title:    item.Title,
		ImageURL: item.Link,
		ThumbURL: item.Image.ThumbnailLink,
		Width:    item.Image.Width,
		Height:   item.Image.Height,
	}
}

func (c *SearchClient) DownloadImage(ctx context.Context, imageURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download image: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download image: %s", resp.Status)
	}

	contentType := resp.Header.Get("Content-Type")
	if !isImageContentType(contentType) {
		return nil, fmt.Errorf("invalid content type: %s", contentType)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read image data: %w", err)
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
	return strings.HasPrefix(strings.ToLower(contentType), "image/")
}
