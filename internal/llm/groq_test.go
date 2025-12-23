package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/conneroisu/groq-go"

	"craftstory/pkg/prompts"
)

type groqResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func testPrompts() *prompts.Prompts {
	return &prompts.Prompts{
		System: prompts.SystemPrompts{
			Default:      "You are a helpful assistant.",
			Conversation: "You are a conversation writer.",
			Visuals:      "You generate visual cues as JSON.",
			Title:        "You generate titles.",
		},
		Script: prompts.ScriptPrompts{
			Single:       "Write about {{.Topic}} in {{.WordCount}} words.",
			Conversation: "Write a conversation about {{.Topic}} with {{.SpeakerList}}.",
			Visuals:      "Generate visuals for: {{.Script}}",
		},
		Title: prompts.TitlePrompts{
			Generate: "Generate a title for: {{.Script}}",
		},
	}
}

// makeGroqResponse creates a valid Groq API response with the given content
func makeGroqResponse(content string) groqResponse {
	return groqResponse{
		ID:      "test-id",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "llama3-8b-8192",
		Choices: []struct {
			Index   int `json:"index"`
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		}{
			{
				Index: 0,
				Message: struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				}{
					Role:    "assistant",
					Content: content,
				},
				FinishReason: "stop",
			},
		},
		Usage: struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		}{
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
		},
	}
}

// makeEmptyChoicesResponse creates a response with no choices
func makeEmptyChoicesResponse() groqResponse {
	resp := makeGroqResponse("")
	resp.Choices = nil
	return resp
}

// newTestClient creates a GroqClient pointing to the test server
func newTestClient(t *testing.T, serverURL string) *GroqClient {
	t.Helper()
	client, err := groq.NewClient("test-api-key", groq.WithBaseURL(serverURL+"/"))
	if err != nil {
		t.Fatalf("failed to create groq client: %v", err)
	}
	return &GroqClient{
		client:  client,
		model:   groq.ChatModel("llama3-8b-8192"),
		prompts: testPrompts(),
	}
}

func TestGenerateScript(t *testing.T) {
	tests := []struct {
		name           string
		topic          string
		wordCount      int
		responseBody   string
		statusCode     int
		wantErr        bool
		wantErrContain string
		wantContent    string
	}{
		{
			name:         "successfulGeneration",
			topic:        "space exploration",
			wordCount:    100,
			responseBody: mustJSON(makeGroqResponse("Space is vast and mysterious. Humans have always looked up at the stars.")),
			statusCode:   http.StatusOK,
			wantErr:      false,
			wantContent:  "Space is vast and mysterious. Humans have always looked up at the stars.",
		},
		{
			name:           "emptyResponse",
			topic:          "test topic",
			wordCount:      50,
			responseBody:   mustJSON(makeGroqResponse("")),
			statusCode:     http.StatusOK,
			wantErr:        true,
			wantErrContain: "empty response",
		},
		{
			name:           "noChoices",
			topic:          "test topic",
			wordCount:      50,
			responseBody:   mustJSON(makeEmptyChoicesResponse()),
			statusCode:     http.StatusOK,
			wantErr:        true,
			wantErrContain: "no response",
		},
		{
			name:      "httpErrorUnauthorized",
			topic:     "test topic",
			wordCount: 50,
			// Use 401 Unauthorized - groq-go doesn't retry on this status
			responseBody:   `{"error": {"message": "invalid api key", "type": "authentication_error"}}`,
			statusCode:     http.StatusUnauthorized,
			wantErr:        true,
			wantErrContain: "generate",
		},
		{
			name:      "httpErrorBadRequest",
			topic:     "test topic",
			wordCount: 50,
			// Use 400 Bad Request - groq-go doesn't retry on this status
			responseBody:   `{"error": {"message": "bad request", "type": "invalid_request_error"}}`,
			statusCode:     http.StatusBadRequest,
			wantErr:        true,
			wantErrContain: "generate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			client := newTestClient(t, server.URL)
			got, err := client.GenerateScript(context.Background(), tt.topic, tt.wordCount)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GenerateScript() expected error containing %q, got nil", tt.wantErrContain)
					return
				}
				if !strings.Contains(err.Error(), tt.wantErrContain) {
					t.Errorf("GenerateScript() error = %v, want error containing %q", err, tt.wantErrContain)
				}
				return
			}

			if err != nil {
				t.Errorf("GenerateScript() unexpected error: %v", err)
				return
			}

			if got != tt.wantContent {
				t.Errorf("GenerateScript() = %q, want %q", got, tt.wantContent)
			}
		})
	}
}

func TestGenerateConversation(t *testing.T) {
	tests := []struct {
		name           string
		topic          string
		speakers       []string
		wordCount      int
		responseBody   string
		statusCode     int
		wantErr        bool
		wantErrContain string
		wantContent    string
	}{
		{
			name:         "successfulConversation",
			topic:        "climate change",
			speakers:     []string{"Alice", "Bob"},
			wordCount:    200,
			responseBody: mustJSON(makeGroqResponse("Alice: Climate change is a pressing issue.\nBob: I agree, we need to act now.")),
			statusCode:   http.StatusOK,
			wantErr:      false,
			wantContent:  "Alice: Climate change is a pressing issue.\nBob: I agree, we need to act now.",
		},
		{
			name:         "threeSpeakers",
			topic:        "technology",
			speakers:     []string{"Host", "Expert", "Guest"},
			wordCount:    300,
			responseBody: mustJSON(makeGroqResponse("Host: Welcome to our show.\nExpert: Thanks for having me.\nGuest: Excited to be here.")),
			statusCode:   http.StatusOK,
			wantErr:      false,
			wantContent:  "Host: Welcome to our show.\nExpert: Thanks for having me.\nGuest: Excited to be here.",
		},
		{
			name:           "emptyResponse",
			topic:          "test",
			speakers:       []string{"A", "B"},
			wordCount:      100,
			responseBody:   mustJSON(makeGroqResponse("")),
			statusCode:     http.StatusOK,
			wantErr:        true,
			wantErrContain: "empty response",
		},
		{
			name:           "httpErrorForbidden",
			topic:          "test",
			speakers:       []string{"A", "B"},
			wordCount:      100,
			responseBody:   `{"error": {"message": "forbidden", "type": "permission_error"}}`,
			statusCode:     http.StatusForbidden,
			wantErr:        true,
			wantErrContain: "generate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			client := newTestClient(t, server.URL)
			got, err := client.GenerateConversation(context.Background(), tt.topic, tt.speakers, tt.wordCount)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GenerateConversation() expected error containing %q, got nil", tt.wantErrContain)
					return
				}
				if !strings.Contains(err.Error(), tt.wantErrContain) {
					t.Errorf("GenerateConversation() error = %v, want error containing %q", err, tt.wantErrContain)
				}
				return
			}

			if err != nil {
				t.Errorf("GenerateConversation() unexpected error: %v", err)
				return
			}

			if got != tt.wantContent {
				t.Errorf("GenerateConversation() = %q, want %q", got, tt.wantContent)
			}
		})
	}
}

func TestGenerateVisuals(t *testing.T) {
	tests := []struct {
		name           string
		script         string
		responseBody   string
		statusCode     int
		wantErr        bool
		wantErrContain string
		wantVisuals    []VisualCue
	}{
		{
			name:         "successfulVisualsArray",
			script:       "The ocean is vast and beautiful.",
			responseBody: mustJSON(makeGroqResponse(`[{"keyword": "ocean", "search_query": "vast ocean waves"}]`)),
			statusCode:   http.StatusOK,
			wantErr:      false,
			wantVisuals: []VisualCue{
				{Keyword: "ocean", SearchQuery: "vast ocean waves"},
			},
		},
		{
			name:         "successfulVisualsWrapped",
			script:       "Mountains reach toward the sky.",
			responseBody: mustJSON(makeGroqResponse(`{"visuals": [{"keyword": "mountains", "search_query": "tall mountain peaks"}]}`)),
			statusCode:   http.StatusOK,
			wantErr:      false,
			wantVisuals: []VisualCue{
				{Keyword: "mountains", SearchQuery: "tall mountain peaks"},
			},
		},
		{
			name:   "multipleVisuals",
			script: "The forest is home to many animals.",
			responseBody: mustJSON(makeGroqResponse(`[
				{"keyword": "forest", "search_query": "dense green forest"},
				{"keyword": "animals", "search_query": "forest wildlife animals"}
			]`)),
			statusCode: http.StatusOK,
			wantErr:    false,
			wantVisuals: []VisualCue{
				{Keyword: "forest", SearchQuery: "dense green forest"},
				{Keyword: "animals", SearchQuery: "forest wildlife animals"},
			},
		},
		{
			name:           "invalidJSON",
			script:         "test script",
			responseBody:   mustJSON(makeGroqResponse(`not valid json`)),
			statusCode:     http.StatusOK,
			wantErr:        true,
			wantErrContain: "parse response",
		},
		{
			name:           "emptyResponse",
			script:         "test script",
			responseBody:   mustJSON(makeGroqResponse("")),
			statusCode:     http.StatusOK,
			wantErr:        true,
			wantErrContain: "empty response",
		},
		{
			name:           "noChoices",
			script:         "test script",
			responseBody:   mustJSON(makeEmptyChoicesResponse()),
			statusCode:     http.StatusOK,
			wantErr:        true,
			wantErrContain: "no response",
		},
		{
			name:           "httpErrorUnauthorized",
			script:         "test script",
			responseBody:   `{"error": {"message": "unauthorized", "type": "authentication_error"}}`,
			statusCode:     http.StatusUnauthorized,
			wantErr:        true,
			wantErrContain: "generate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			client := newTestClient(t, server.URL)
			got, err := client.GenerateVisuals(context.Background(), tt.script, 5)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GenerateVisuals() expected error containing %q, got nil", tt.wantErrContain)
					return
				}
				if !strings.Contains(err.Error(), tt.wantErrContain) {
					t.Errorf("GenerateVisuals() error = %v, want error containing %q", err, tt.wantErrContain)
				}
				return
			}

			if err != nil {
				t.Errorf("GenerateVisuals() unexpected error: %v", err)
				return
			}

			if len(got) != len(tt.wantVisuals) {
				t.Errorf("GenerateVisuals() returned %d visuals, want %d", len(got), len(tt.wantVisuals))
				return
			}

			for i, v := range got {
				if v.Keyword != tt.wantVisuals[i].Keyword {
					t.Errorf("GenerateVisuals()[%d].Keyword = %q, want %q", i, v.Keyword, tt.wantVisuals[i].Keyword)
				}
				if v.SearchQuery != tt.wantVisuals[i].SearchQuery {
					t.Errorf("GenerateVisuals()[%d].SearchQuery = %q, want %q", i, v.SearchQuery, tt.wantVisuals[i].SearchQuery)
				}
			}
		})
	}
}

func TestGenerateTitle(t *testing.T) {
	tests := []struct {
		name           string
		script         string
		responseBody   string
		statusCode     int
		wantErr        bool
		wantErrContain string
		wantTitle      string
	}{
		{
			name:         "successfulTitle",
			script:       "This is a story about adventure and discovery.",
			responseBody: mustJSON(makeGroqResponse("The Great Adventure")),
			statusCode:   http.StatusOK,
			wantErr:      false,
			wantTitle:    "The Great Adventure",
		},
		{
			name:         "titleWithSpecialCharacters",
			script:       "A tale of love and loss.",
			responseBody: mustJSON(makeGroqResponse("Love & Loss: A Journey")),
			statusCode:   http.StatusOK,
			wantErr:      false,
			wantTitle:    "Love & Loss: A Journey",
		},
		{
			name:           "emptyResponse",
			script:         "test script",
			responseBody:   mustJSON(makeGroqResponse("")),
			statusCode:     http.StatusOK,
			wantErr:        true,
			wantErrContain: "empty response",
		},
		{
			name:           "noChoices",
			script:         "test script",
			responseBody:   mustJSON(makeEmptyChoicesResponse()),
			statusCode:     http.StatusOK,
			wantErr:        true,
			wantErrContain: "no response",
		},
		{
			name:           "httpErrorNotFound",
			script:         "test script",
			responseBody:   `{"error": {"message": "not found", "type": "not_found_error"}}`,
			statusCode:     http.StatusNotFound,
			wantErr:        true,
			wantErrContain: "generate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			client := newTestClient(t, server.URL)
			got, err := client.GenerateTitle(context.Background(), tt.script)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GenerateTitle() expected error containing %q, got nil", tt.wantErrContain)
					return
				}
				if !strings.Contains(err.Error(), tt.wantErrContain) {
					t.Errorf("GenerateTitle() error = %v, want error containing %q", err, tt.wantErrContain)
				}
				return
			}

			if err != nil {
				t.Errorf("GenerateTitle() unexpected error: %v", err)
				return
			}

			if got != tt.wantTitle {
				t.Errorf("GenerateTitle() = %q, want %q", got, tt.wantTitle)
			}
		})
	}
}

func TestRequestValidation(t *testing.T) {
	t.Run("verifiesRequestBody", func(t *testing.T) {
		var receivedBody map[string]any

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST request, got %s", r.Method)
			}

			if ct := r.Header.Get("Content-Type"); ct != "application/json" {
				t.Errorf("expected Content-Type application/json, got %s", ct)
			}

			if auth := r.Header.Get("Authorization"); auth != "Bearer test-api-key" {
				t.Errorf("expected Authorization Bearer test-api-key, got %s", auth)
			}

			decoder := json.NewDecoder(r.Body)
			if err := decoder.Decode(&receivedBody); err != nil {
				t.Errorf("failed to decode request body: %v", err)
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(mustJSON(makeGroqResponse("test response"))))
		}))
		defer server.Close()

		client := newTestClient(t, server.URL)
		_, err := client.GenerateScript(context.Background(), "test topic", 100)
		if err != nil {
			t.Fatalf("GenerateScript() error: %v", err)
		}

		if receivedBody["model"] != "llama3-8b-8192" {
			t.Errorf("expected model llama3-8b-8192, got %v", receivedBody["model"])
		}

		messages, ok := receivedBody["messages"].([]any)
		if !ok || len(messages) != 2 {
			t.Errorf("expected 2 messages, got %v", receivedBody["messages"])
		}
	})
}

func TestContextCancellation(t *testing.T) {
	t.Run("respectsContextCancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Simulate slow response - but we'll cancel before it completes
			<-r.Context().Done()
		}))
		defer server.Close()

		client := newTestClient(t, server.URL)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := client.GenerateScript(ctx, "test", 100)
		if err == nil {
			t.Error("expected error due to cancelled context, got nil")
		}
	})
}

func TestRateLimitError(t *testing.T) {
	t.Run("handlesRateLimitError", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// 429 Too Many Requests - groq-go doesn't retry on this status
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error": {"message": "rate limit exceeded", "type": "rate_limit_error"}}`))
		}))
		defer server.Close()

		client := newTestClient(t, server.URL)
		_, err := client.GenerateScript(context.Background(), "test", 100)

		if err == nil {
			t.Error("expected error for rate limit, got nil")
			return
		}

		if !strings.Contains(err.Error(), "generate") {
			t.Errorf("expected error containing 'generate', got: %v", err)
		}
	})
}

// mustJSON marshals v to JSON and panics on error (for test setup only)
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
