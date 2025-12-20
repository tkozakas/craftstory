package reddit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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

type Comment struct {
	Body   string
	Author string
	Score  int
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

type threadResponse []json.RawMessage

type commentListing struct {
	Data struct {
		Children []struct {
			Data commentData `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

type commentData struct {
	Body   string `json:"body"`
	Author string `json:"author"`
	Score  int    `json:"score"`
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
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	posts := make([]Post, 0, len(resp.Data.Children))
	for _, child := range resp.Data.Children {
		posts = append(posts, postFromData(child.Data))
	}

	return posts, nil
}

func (c *Client) GetPostComments(ctx context.Context, permalink string, limit int) ([]Comment, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}

	permalink = strings.TrimSuffix(permalink, "/")
	url := fmt.Sprintf("%s%s.json?limit=%d", c.baseURL, permalink, limit)

	body, err := c.doRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	var raw threadResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(raw) < 2 {
		return nil, nil
	}

	var commentList commentListing
	if err := json.Unmarshal(raw[1], &commentList); err != nil {
		return nil, fmt.Errorf("failed to parse comments: %w", err)
	}

	comments := make([]Comment, 0, len(commentList.Data.Children))
	for _, child := range commentList.Data.Children {
		if child.Data.Body == "" || child.Data.Body == "[deleted]" {
			continue
		}
		comments = append(comments, Comment{
			Body:   child.Data.Body,
			Author: child.Data.Author,
			Score:  child.Data.Score,
		})
	}

	return comments, nil
}

func (c *Client) GetTopStories(ctx context.Context, subreddit string, limit int) ([]Post, error) {
	return c.GetSubredditPosts(ctx, subreddit, "top", limit)
}

func (c *Client) GetPostWithComments(ctx context.Context, permalink string, commentLimit int) (*Post, []Comment, error) {
	if commentLimit <= 0 || commentLimit > 100 {
		commentLimit = 25
	}

	permalink = strings.TrimSuffix(permalink, "/")
	url := fmt.Sprintf("%s%s.json?limit=%d", c.baseURL, permalink, commentLimit)

	body, err := c.doRequest(ctx, url)
	if err != nil {
		return nil, nil, err
	}

	var raw threadResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(raw) == 0 {
		return nil, nil, fmt.Errorf("post not found")
	}

	var postListing listingResponse
	if err := json.Unmarshal(raw[0], &postListing); err != nil {
		return nil, nil, fmt.Errorf("failed to parse post: %w", err)
	}

	if len(postListing.Data.Children) == 0 {
		return nil, nil, fmt.Errorf("post not found")
	}

	post := postFromData(postListing.Data.Children[0].Data)

	var comments []Comment
	if len(raw) > 1 {
		var commentList commentListing
		if err := json.Unmarshal(raw[1], &commentList); err == nil {
			for _, child := range commentList.Data.Children {
				if child.Data.Body == "" || child.Data.Body == "[deleted]" {
					continue
				}
				comments = append(comments, Comment{
					Body:   child.Data.Body,
					Author: child.Data.Author,
					Score:  child.Data.Score,
				})
			}
		}
	}

	return &post, comments, nil
}

func (c *Client) SearchSubreddit(ctx context.Context, subreddit, query string, limit int) ([]Post, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}

	url := fmt.Sprintf("%s/r/%s/search.json?q=%s&restrict_sr=1&limit=%d", c.baseURL, subreddit, query, limit)

	body, err := c.doRequest(ctx, url)
	if err != nil {
		return nil, err
	}

	var resp listingResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
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
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reddit api error: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return body, nil
}

func postFromData(data postData) Post {
	return Post(data)
}
