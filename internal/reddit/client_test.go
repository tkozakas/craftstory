package reddit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetSubredditPosts(t *testing.T) {
	tests := []struct {
		name         string
		subreddit    string
		sort         string
		limit        int
		serverResp   listingResponse
		serverStatus int
		wantErr      bool
		wantCount    int
	}{
		{
			name:      "successfulFetch",
			subreddit: "golang",
			sort:      "hot",
			limit:     10,
			serverResp: listingResponse{
				Data: struct {
					Children []struct {
						Data postData `json:"data"`
					} `json:"children"`
				}{
					Children: []struct {
						Data postData `json:"data"`
					}{
						{Data: postData{Title: "Post 1", Score: 100}},
						{Data: postData{Title: "Post 2", Score: 50}},
					},
				},
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
			wantCount:    2,
		},
		{
			name:         "emptySubreddit",
			subreddit:    "emptysub",
			sort:         "hot",
			limit:        10,
			serverResp:   listingResponse{},
			serverStatus: http.StatusOK,
			wantErr:      false,
			wantCount:    0,
		},
		{
			name:         "serverError",
			subreddit:    "test",
			sort:         "new",
			limit:        5,
			serverStatus: http.StatusInternalServerError,
			wantErr:      true,
		},
		{
			name:      "defaultSort",
			subreddit: "test",
			sort:      "",
			limit:     5,
			serverResp: listingResponse{
				Data: struct {
					Children []struct {
						Data postData `json:"data"`
					} `json:"children"`
				}{
					Children: []struct {
						Data postData `json:"data"`
					}{
						{Data: postData{Title: "Post 1"}},
					},
				},
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
			wantCount:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("User-Agent") != userAgent {
					t.Errorf("expected User-Agent %q", userAgent)
				}

				w.WriteHeader(tt.serverStatus)
				if tt.serverStatus == http.StatusOK {
					_ = json.NewEncoder(w).Encode(tt.serverResp)
				}
			}))
			defer server.Close()

			client := NewClient()
			client.baseURL = server.URL

			ctx := context.Background()
			posts, err := client.GetSubredditPosts(ctx, tt.subreddit, tt.sort, tt.limit)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetSubredditPosts() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(posts) != tt.wantCount {
				t.Errorf("GetSubredditPosts() returned %d posts, want %d", len(posts), tt.wantCount)
			}
		})
	}
}

func TestGetTopStories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := listingResponse{
			Data: struct {
				Children []struct {
					Data postData `json:"data"`
				} `json:"children"`
			}{
				Children: []struct {
					Data postData `json:"data"`
				}{
					{Data: postData{Title: "Top Story", Score: 1000}},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL

	ctx := context.Background()
	posts, err := client.GetTopStories(ctx, "news", 5)
	if err != nil {
		t.Fatalf("GetTopStories() error = %v", err)
	}

	if len(posts) != 1 {
		t.Errorf("GetTopStories() returned %d posts, want 1", len(posts))
	}

	if posts[0].Title != "Top Story" {
		t.Errorf("post title = %q, want %q", posts[0].Title, "Top Story")
	}
}

func TestGetPostComments(t *testing.T) {
	tests := []struct {
		name         string
		permalink    string
		limit        int
		serverResp   []json.RawMessage
		serverStatus int
		wantErr      bool
		wantCount    int
	}{
		{
			name:      "successfulFetch",
			permalink: "/r/test/comments/abc123/test_post/",
			limit:     10,
			serverResp: func() []json.RawMessage {
				post, _ := json.Marshal(listingResponse{})
				comments, _ := json.Marshal(commentListing{
					Data: struct {
						Children []struct {
							Data commentData `json:"data"`
						} `json:"children"`
					}{
						Children: []struct {
							Data commentData `json:"data"`
						}{
							{Data: commentData{Body: "Great post!", Author: "user1", Score: 10}},
							{Data: commentData{Body: "Thanks!", Author: "user2", Score: 5}},
						},
					},
				})
				return []json.RawMessage{post, comments}
			}(),
			serverStatus: http.StatusOK,
			wantErr:      false,
			wantCount:    2,
		},
		{
			name:      "deletedComments",
			permalink: "/r/test/comments/xyz789/deleted/",
			limit:     10,
			serverResp: func() []json.RawMessage {
				post, _ := json.Marshal(listingResponse{})
				comments, _ := json.Marshal(commentListing{
					Data: struct {
						Children []struct {
							Data commentData `json:"data"`
						} `json:"children"`
					}{
						Children: []struct {
							Data commentData `json:"data"`
						}{
							{Data: commentData{Body: "[deleted]", Author: "[deleted]"}},
							{Data: commentData{Body: "", Author: "empty"}},
							{Data: commentData{Body: "Valid comment", Author: "user1"}},
						},
					},
				})
				return []json.RawMessage{post, comments}
			}(),
			serverStatus: http.StatusOK,
			wantErr:      false,
			wantCount:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.serverStatus)
				if tt.serverStatus == http.StatusOK {
					_ = json.NewEncoder(w).Encode(tt.serverResp)
				}
			}))
			defer server.Close()

			client := NewClient()
			client.baseURL = server.URL

			ctx := context.Background()
			comments, err := client.GetPostComments(ctx, tt.permalink, tt.limit)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetPostComments() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(comments) != tt.wantCount {
				t.Errorf("GetPostComments() returned %d comments, want %d", len(comments), tt.wantCount)
			}
		})
	}
}

func TestSearchSubreddit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") == "" {
			t.Error("expected query parameter 'q'")
		}
		if r.URL.Query().Get("restrict_sr") != "1" {
			t.Error("expected restrict_sr=1")
		}

		resp := listingResponse{
			Data: struct {
				Children []struct {
					Data postData `json:"data"`
				} `json:"children"`
			}{
				Children: []struct {
					Data postData `json:"data"`
				}{
					{Data: postData{Title: "Search Result", Score: 50}},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL

	ctx := context.Background()
	posts, err := client.SearchSubreddit(ctx, "golang", "concurrency", 10)
	if err != nil {
		t.Fatalf("SearchSubreddit() error = %v", err)
	}

	if len(posts) != 1 {
		t.Errorf("SearchSubreddit() returned %d posts, want 1", len(posts))
	}
}

func TestLimitValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(listingResponse{})
	}))
	defer server.Close()

	client := NewClient()
	client.baseURL = server.URL

	ctx := context.Background()

	_, err := client.GetSubredditPosts(ctx, "test", "hot", -1)
	if err != nil {
		t.Errorf("should handle negative limit: %v", err)
	}

	_, err = client.GetSubredditPosts(ctx, "test", "hot", 0)
	if err != nil {
		t.Errorf("should handle zero limit: %v", err)
	}

	_, err = client.GetSubredditPosts(ctx, "test", "hot", 200)
	if err != nil {
		t.Errorf("should handle limit > 100: %v", err)
	}
}

func TestPostFromData(t *testing.T) {
	data := postData{
		Title:       "Test Title",
		Selftext:    "Test body",
		Author:      "testuser",
		Score:       42,
		URL:         "https://reddit.com/r/test/abc",
		Permalink:   "/r/test/comments/abc/test/",
		Created:     1234567890.0,
		NumComments: 10,
	}

	post := postFromData(data)

	if post.Title != data.Title {
		t.Errorf("Title = %q, want %q", post.Title, data.Title)
	}
	if post.Author != data.Author {
		t.Errorf("Author = %q, want %q", post.Author, data.Author)
	}
	if post.Score != data.Score {
		t.Errorf("Score = %d, want %d", post.Score, data.Score)
	}
}
