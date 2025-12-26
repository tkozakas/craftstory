package reddit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	baseURL        = "https://www.reddit.com"
	defaultTimeout = 30 * time.Second
	userAgent      = "craftstory/1.0"
)

type Client struct {
	httpClient *http.Client
	baseURL    string
}

type Post struct {
	Title       string
	Selftext    string
	Author      string
	Score       int
	URL         string
	Permalink   string
	Created     float64
	NumComments int
}

type listingResponse struct {
	Data struct {
		Children []struct {
			Data postData `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

type postData struct {
	Title       string  `json:"title"`
	Selftext    string  `json:"selftext"`
	Author      string  `json:"author"`
	Score       int     `json:"score"`
	URL         string  `json:"url"`
	Permalink   string  `json:"permalink"`
	Created     float64 `json:"created_utc"`
	NumComments int     `json:"num_comments"`
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		baseURL: baseURL,
	}
}

func (c *Client) GetSubredditPosts(ctx context.Context, subreddit, sort string, limit int) ([]Post, error) {
	if sort == "" {
		sort = "hot"
	}
	if limit <= 0 || limit > 100 {
		limit = 25
	}

	url := fmt.Sprintf("%s/r/%s/%s.json?limit=%d", c.baseURL, subreddit, sort, limit)

	body, err := c.doRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	var resp listingResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	posts := make([]Post, 0, len(resp.Data.Children))
	for _, child := range resp.Data.Children {
		posts = append(posts, postFromData(child.Data))
	}

	return posts, nil
}

func (c *Client) doRequest(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reddit api error: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return body, nil
}

func postFromData(data postData) Post {
	return Post(data)
}
