package tenor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name        string
		cfg         Config
		wantTimeout time.Duration
	}{
		{
			name:        "defaultTimeout",
			cfg:         Config{APIKey: "test-key"},
			wantTimeout: defaultTimeout,
		},
		{
			name:        "customTimeout",
			cfg:         Config{APIKey: "test-key", Timeout: 30 * time.Second},
			wantTimeout: 30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.cfg)

			if client.apiKey != tt.cfg.APIKey {
				t.Errorf("apiKey = %q, want %q", client.apiKey, tt.cfg.APIKey)
			}
			if client.httpClient == nil {
				t.Error("httpClient is nil")
			}
			if client.httpClient.Timeout != tt.wantTimeout {
				t.Errorf("timeout = %v, want %v", client.httpClient.Timeout, tt.wantTimeout)
			}
			if client.baseURL != baseURL {
				t.Errorf("baseURL = %q, want %q", client.baseURL, baseURL)
			}
		})
	}
}

func TestSearch(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		limit       int
		response    searchResponse
		statusCode  int
		wantErr     bool
		wantResults int
	}{
		{
			name:  "successfulSearch",
			query: "funny cat",
			limit: 3,
			response: searchResponse{
				Results: []result{
					{
						ID:    "123",
						Title: "Funny Cat",
						MediaFormats: map[string]mediaFormat{
							"gif":     {URL: "http://example.com/cat.gif", Dims: []int{480, 360}},
							"tinygif": {URL: "http://example.com/cat_tiny.gif", Dims: []int{220, 165}},
						},
					},
					{
						ID:    "456",
						Title: "Another Cat",
						MediaFormats: map[string]mediaFormat{
							"gif": {URL: "http://example.com/cat2.gif", Dims: []int{400, 300}},
						},
					},
				},
			},
			statusCode:  http.StatusOK,
			wantErr:     false,
			wantResults: 2,
		},
		{
			name:        "emptyResults",
			query:       "nonexistent",
			limit:       5,
			response:    searchResponse{Results: []result{}},
			statusCode:  http.StatusOK,
			wantErr:     false,
			wantResults: 0,
		},
		{
			name:       "apiError",
			query:      "test",
			limit:      1,
			statusCode: http.StatusUnauthorized,
			wantErr:    true,
		},
		{
			name:  "defaultLimit",
			query: "test",
			limit: 0,
			response: searchResponse{
				Results: []result{
					{
						ID:           "789",
						MediaFormats: map[string]mediaFormat{"gif": {URL: "http://example.com/g.gif", Dims: []int{100, 100}}},
					},
				},
			},
			statusCode:  http.StatusOK,
			wantErr:     false,
			wantResults: 1,
		},
		{
			name:  "skipResultsWithoutMedia",
			query: "test",
			limit: 5,
			response: searchResponse{
				Results: []result{
					{ID: "1", MediaFormats: map[string]mediaFormat{"gif": {URL: "http://example.com/1.gif", Dims: []int{100, 100}}}},
					{ID: "2", MediaFormats: map[string]mediaFormat{}},
					{ID: "3", MediaFormats: map[string]mediaFormat{"gif": {URL: "http://example.com/3.gif", Dims: []int{100, 100}}}},
				},
			},
			statusCode:  http.StatusOK,
			wantErr:     false,
			wantResults: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("key") != "test-key" {
					t.Error("missing api key")
				}
				if r.URL.Query().Get("q") != tt.query {
					t.Errorf("query = %q, want %q", r.URL.Query().Get("q"), tt.query)
				}
				if r.URL.Query().Get("media_filter") != "gif,tinygif" {
					t.Error("missing media_filter")
				}

				w.WriteHeader(tt.statusCode)
				if tt.statusCode == http.StatusOK {
					_ = json.NewEncoder(w).Encode(tt.response)
				}
			}))
			defer server.Close()

			client := NewClient(Config{APIKey: "test-key"})
			client.baseURL = server.URL

			results, err := client.Search(context.Background(), tt.query, tt.limit)

			if (err != nil) != tt.wantErr {
				t.Errorf("Search() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(results) != tt.wantResults {
				t.Errorf("Search() got %d results, want %d", len(results), tt.wantResults)
			}
		})
	}
}

func TestSearchResultFields(t *testing.T) {
	response := searchResponse{
		Results: []result{
			{
				ID:    "test-id",
				Title: "Test GIF",
				MediaFormats: map[string]mediaFormat{
					"gif":     {URL: "http://example.com/full.gif", Dims: []int{480, 360}},
					"tinygif": {URL: "http://example.com/tiny.gif", Dims: []int{220, 165}},
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(Config{APIKey: "test-key"})
	client.baseURL = server.URL

	results, err := client.Search(context.Background(), "test", 1)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	gif := results[0]
	if gif.ID != "test-id" {
		t.Errorf("ID = %q, want %q", gif.ID, "test-id")
	}
	if gif.Title != "Test GIF" {
		t.Errorf("Title = %q, want %q", gif.Title, "Test GIF")
	}
	if gif.URL != "http://example.com/full.gif" {
		t.Errorf("URL = %q, want %q", gif.URL, "http://example.com/full.gif")
	}
	if gif.PreviewURL != "http://example.com/tiny.gif" {
		t.Errorf("PreviewURL = %q, want %q", gif.PreviewURL, "http://example.com/tiny.gif")
	}
	if gif.Width != 480 {
		t.Errorf("Width = %d, want %d", gif.Width, 480)
	}
	if gif.Height != 360 {
		t.Errorf("Height = %d, want %d", gif.Height, 360)
	}
	if gif.ContentType != "image/gif" {
		t.Errorf("ContentType = %q, want %q", gif.ContentType, "image/gif")
	}
}

func TestDownload(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       []byte
		wantErr    bool
	}{
		{
			name:       "success",
			statusCode: http.StatusOK,
			body:       []byte("GIF89a"),
			wantErr:    false,
		},
		{
			name:       "notFound",
			statusCode: http.StatusNotFound,
			wantErr:    true,
		},
		{
			name:       "serverError",
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
				if tt.body != nil {
					_, _ = w.Write(tt.body)
				}
			}))
			defer server.Close()

			client := NewClient(Config{APIKey: "test-key"})

			data, err := client.Download(context.Background(), server.URL+"/test.gif")

			if (err != nil) != tt.wantErr {
				t.Errorf("Download() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(data) != len(tt.body) {
				t.Errorf("Download() got %d bytes, want %d", len(data), len(tt.body))
			}
		})
	}
}

func TestSelectMedia(t *testing.T) {
	tests := []struct {
		name    string
		formats map[string]mediaFormat
		wantURL string
		wantNil bool
	}{
		{
			name: "prefersGif",
			formats: map[string]mediaFormat{
				"gif":     {URL: "http://example.com/gif.gif", Dims: []int{480, 360}},
				"tinygif": {URL: "http://example.com/tiny.gif", Dims: []int{220, 165}},
			},
			wantURL: "http://example.com/gif.gif",
		},
		{
			name: "fallsBackToTinygif",
			formats: map[string]mediaFormat{
				"tinygif": {URL: "http://example.com/tiny.gif", Dims: []int{220, 165}},
				"nanogif": {URL: "http://example.com/nano.gif", Dims: []int{90, 68}},
			},
			wantURL: "http://example.com/tiny.gif",
		},
		{
			name:    "emptyFormats",
			formats: map[string]mediaFormat{},
			wantNil: true,
		},
		{
			name: "missingDims",
			formats: map[string]mediaFormat{
				"gif": {URL: "http://example.com/gif.gif", Dims: []int{}},
			},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			media := selectMedia(tt.formats)

			if tt.wantNil {
				if media != nil {
					t.Errorf("selectMedia() = %v, want nil", media)
				}
				return
			}

			if media == nil {
				t.Fatal("selectMedia() = nil, want non-nil")
			}
			if media.URL != tt.wantURL {
				t.Errorf("selectMedia().URL = %q, want %q", media.URL, tt.wantURL)
			}
		})
	}
}

func TestSelectPreview(t *testing.T) {
	tests := []struct {
		name    string
		formats map[string]mediaFormat
		wantURL string
	}{
		{
			name: "prefersTinygif",
			formats: map[string]mediaFormat{
				"gif":     {URL: "http://example.com/gif.gif"},
				"tinygif": {URL: "http://example.com/tiny.gif"},
				"nanogif": {URL: "http://example.com/nano.gif"},
			},
			wantURL: "http://example.com/tiny.gif",
		},
		{
			name: "fallsBackToNanogif",
			formats: map[string]mediaFormat{
				"gif":     {URL: "http://example.com/gif.gif"},
				"nanogif": {URL: "http://example.com/nano.gif"},
			},
			wantURL: "http://example.com/nano.gif",
		},
		{
			name: "fallsBackToGif",
			formats: map[string]mediaFormat{
				"gif": {URL: "http://example.com/gif.gif"},
			},
			wantURL: "http://example.com/gif.gif",
		},
		{
			name:    "emptyFormats",
			formats: map[string]mediaFormat{},
			wantURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := selectPreview(tt.formats)

			if url != tt.wantURL {
				t.Errorf("selectPreview() = %q, want %q", url, tt.wantURL)
			}
		})
	}
}

func TestSearchContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-make(chan struct{})
	}))
	defer server.Close()

	client := NewClient(Config{APIKey: "test-key", Timeout: 100 * time.Millisecond})
	client.baseURL = server.URL

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Search(ctx, "test", 1)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestBuildSearchURL(t *testing.T) {
	client := NewClient(Config{APIKey: "my-api-key"})

	url := client.buildSearchURL("funny cat", 5)

	if url == "" {
		t.Fatal("buildSearchURL() returned empty string")
	}

	wantContains := []string{
		"key=my-api-key",
		"q=funny+cat",
		"limit=5",
		"media_filter=gif",
		"contentfilter=medium",
	}

	for _, want := range wantContains {
		if !strings.Contains(url, want) {
			t.Errorf("buildSearchURL() = %q, want to contain %q", url, want)
		}
	}
}
