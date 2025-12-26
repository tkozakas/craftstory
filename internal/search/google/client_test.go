package google

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClient(t *testing.T) {
	client := NewClient(Config{
		APIKey:   "test-api-key",
		EngineID: "test-engine-id",
	})

	if client.apiKey != "test-api-key" {
		t.Errorf("apiKey = %q, want %q", client.apiKey, "test-api-key")
	}
	if client.engineID != "test-engine-id" {
		t.Errorf("engineID = %q, want %q", client.engineID, "test-engine-id")
	}
	if client.httpClient == nil {
		t.Error("httpClient is nil")
	}
}

func TestSearch(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		count       int
		response    searchResponse
		statusCode  int
		wantErr     bool
		wantResults int
	}{
		{
			name:  "successfulSearch",
			query: "cute cats",
			count: 3,
			response: searchResponse{
				Items: []searchItem{
					{Title: "Cat 1", Link: "http://example.com/cat1.jpg", Image: imageInfo{ThumbnailLink: "http://example.com/cat1_thumb.jpg"}},
					{Title: "Cat 2", Link: "http://example.com/cat2.jpg", Image: imageInfo{ThumbnailLink: "http://example.com/cat2_thumb.jpg"}},
					{Title: "Cat 3", Link: "http://example.com/cat3.jpg", Image: imageInfo{ThumbnailLink: "http://example.com/cat3_thumb.jpg"}},
				},
			},
			statusCode:  http.StatusOK,
			wantErr:     false,
			wantResults: 3,
		},
		{
			name:        "emptyResults",
			query:       "nonexistent thing",
			count:       5,
			response:    searchResponse{Items: []searchItem{}},
			statusCode:  http.StatusOK,
			wantErr:     false,
			wantResults: 0,
		},
		{
			name:       "apiError",
			query:      "test",
			count:      1,
			statusCode: http.StatusUnauthorized,
			wantErr:    true,
		},
		{
			name:  "countCapped",
			query: "test",
			count: 20,
			response: searchResponse{
				Items: []searchItem{
					{Title: "Result", Link: "http://example.com/img.jpg"},
				},
			},
			statusCode:  http.StatusOK,
			wantErr:     false,
			wantResults: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("key") != "test-key" {
					t.Error("missing api key")
				}
				if r.URL.Query().Get("cx") != "test-engine" {
					t.Error("missing engine id")
				}
				if r.URL.Query().Get("searchType") != "image" {
					t.Error("searchType should be image")
				}

				w.WriteHeader(tt.statusCode)
				if tt.statusCode == http.StatusOK {
					_ = json.NewEncoder(w).Encode(tt.response)
				}
			}))
			defer server.Close()

			client := NewClient(Config{APIKey: "test-key", EngineID: "test-engine"})
			client.baseURL = server.URL

			results, err := client.Search(context.Background(), tt.query, tt.count)

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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := searchResponse{
			Items: []searchItem{
				{
					Title: "Test Image",
					Link:  "http://example.com/full.jpg",
					Image: imageInfo{ThumbnailLink: "http://example.com/thumb.jpg"},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{APIKey: "key", EngineID: "engine"})
	client.baseURL = server.URL

	results, err := client.Search(context.Background(), "test", 1)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.Title != "Test Image" {
		t.Errorf("Title = %q, want %q", r.Title, "Test Image")
	}
	if r.ImageURL != "http://example.com/full.jpg" {
		t.Errorf("ImageURL = %q, want %q", r.ImageURL, "http://example.com/full.jpg")
	}
	if r.ThumbURL != "http://example.com/thumb.jpg" {
		t.Errorf("ThumbURL = %q, want %q", r.ThumbURL, "http://example.com/thumb.jpg")
	}
}

func TestDownloadImage(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		contentType string
		body        []byte
		wantErr     bool
	}{
		{
			name:        "success",
			statusCode:  http.StatusOK,
			contentType: "image/png",
			body:        []byte{0x89, 0x50, 0x4E, 0x47},
			wantErr:     false,
		},
		{
			name:        "successJpeg",
			statusCode:  http.StatusOK,
			contentType: "image/jpeg",
			body:        []byte{0xFF, 0xD8, 0xFF},
			wantErr:     false,
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
		{
			name:        "invalidContentType",
			statusCode:  http.StatusOK,
			contentType: "text/html",
			body:        []byte("<html>"),
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tt.contentType != "" {
					w.Header().Set("Content-Type", tt.contentType)
				}
				w.WriteHeader(tt.statusCode)
				if tt.body != nil {
					_, _ = w.Write(tt.body)
				}
			}))
			defer server.Close()

			client := NewClient(Config{APIKey: "key", EngineID: "engine"})

			data, err := client.DownloadImage(context.Background(), server.URL+"/image.png")

			if (err != nil) != tt.wantErr {
				t.Errorf("DownloadImage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(data) != len(tt.body) {
				t.Errorf("DownloadImage() got %d bytes, want %d", len(data), len(tt.body))
			}
		})
	}
}

func TestSearchContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-make(chan struct{})
	}))
	defer server.Close()

	client := NewClient(Config{APIKey: "key", EngineID: "engine"})
	client.baseURL = server.URL

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Search(ctx, "test", 1)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}
