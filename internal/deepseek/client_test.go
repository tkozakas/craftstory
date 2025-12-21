package deepseek

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGenerateScript(t *testing.T) {
	tests := []struct {
		name           string
		topic          string
		scriptLength   int
		hookDuration   int
		serverResponse response
		serverStatus   int
		wantErr        bool
		wantContent    string
	}{
		{
			name:         "successfulGeneration",
			topic:        "space facts",
			scriptLength: 30,
			hookDuration: 5,
			serverResponse: response{
				ID: "test-123",
				Choices: []choice{
					{Message: Message{Role: "assistant", Content: "Did you know that space is silent?"}},
				},
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
			wantContent:  "Did you know that space is silent?",
		},
		{
			name:         "emptyChoices",
			topic:        "history",
			scriptLength: 30,
			hookDuration: 5,
			serverResponse: response{
				ID:      "test-456",
				Choices: []choice{},
			},
			serverStatus: http.StatusOK,
			wantErr:      true,
		},
		{
			name:         "apiError",
			topic:        "science",
			scriptLength: 30,
			hookDuration: 5,
			serverResponse: response{
				Error: &apiError{Message: "rate limit exceeded", Type: "rate_limit"},
			},
			serverStatus: http.StatusOK,
			wantErr:      true,
		},
		{
			name:         "serverError",
			topic:        "tech",
			scriptLength: 30,
			hookDuration: 5,
			serverStatus: http.StatusInternalServerError,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if r.Header.Get("Authorization") != "Bearer test-key" {
					t.Errorf("expected Authorization header with Bearer token")
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("expected Content-Type application/json")
				}

				w.WriteHeader(tt.serverStatus)
				if tt.serverStatus == http.StatusOK {
					_ = json.NewEncoder(w).Encode(tt.serverResponse)
				}
			}))
			defer server.Close()

			client := NewClient("test-key", Options{
				Model:        "deepseek-chat",
				SystemPrompt: "You are a script writer.",
			})
			client.baseURL = server.URL

			ctx := context.Background()
			got, err := client.GenerateScript(ctx, tt.topic, tt.scriptLength, tt.hookDuration)

			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateScript() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && got != tt.wantContent {
				t.Errorf("GenerateScript() = %q, want %q", got, tt.wantContent)
			}
		})
	}
}

func TestChat(t *testing.T) {
	tests := []struct {
		name           string
		prompt         string
		history        []Message
		serverResponse response
		serverStatus   int
		wantErr        bool
		wantContent    string
	}{
		{
			name:    "chatWithHistory",
			prompt:  "Tell me more",
			history: []Message{{Role: "user", Content: "Hello"}, {Role: "assistant", Content: "Hi!"}},
			serverResponse: response{
				ID: "chat-123",
				Choices: []choice{
					{Message: Message{Role: "assistant", Content: "Here is more info."}},
				},
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
			wantContent:  "Here is more info.",
		},
		{
			name:    "chatWithoutHistory",
			prompt:  "Hello",
			history: nil,
			serverResponse: response{
				ID: "chat-456",
				Choices: []choice{
					{Message: Message{Role: "assistant", Content: "Hi there!"}},
				},
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
			wantContent:  "Hi there!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req request
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Errorf("failed to decode request: %v", err)
				}

				expectedMsgCount := len(tt.history) + 2
				if len(req.Messages) != expectedMsgCount {
					t.Errorf("expected %d messages, got %d", expectedMsgCount, len(req.Messages))
				}

				w.WriteHeader(tt.serverStatus)
				_ = json.NewEncoder(w).Encode(tt.serverResponse)
			}))
			defer server.Close()

			client := NewClient("test-key", Options{
				Model:        "deepseek-chat",
				SystemPrompt: "You are helpful.",
			})
			client.baseURL = server.URL

			ctx := context.Background()
			got, err := client.Chat(ctx, tt.prompt, tt.history)

			if (err != nil) != tt.wantErr {
				t.Errorf("Chat() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && got != tt.wantContent {
				t.Errorf("Chat() = %q, want %q", got, tt.wantContent)
			}
		})
	}
}

func TestModel(t *testing.T) {
	client := NewClient("key", Options{Model: "deepseek-v2"})
	if got := client.Model(); got != "deepseek-v2" {
		t.Errorf("Model() = %q, want %q", got, "deepseek-v2")
	}
}

func TestBuildMessages(t *testing.T) {
	client := NewClient("key", Options{SystemPrompt: "System prompt"})

	tests := []struct {
		name     string
		prompt   string
		history  []Message
		wantLen  int
		wantLast string
	}{
		{
			name:     "noHistory",
			prompt:   "Hello",
			history:  nil,
			wantLen:  2,
			wantLast: "Hello",
		},
		{
			name:    "withHistory",
			prompt:  "Goodbye",
			history: []Message{{Role: "user", Content: "Hi"}, {Role: "assistant", Content: "Hey"}},
			wantLen: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgs := client.buildMessages(tt.prompt, tt.history)
			if len(msgs) != tt.wantLen {
				t.Errorf("buildMessages() len = %d, want %d", len(msgs), tt.wantLen)
			}
			if msgs[0].Role != roleSystem {
				t.Errorf("first message should be system, got %s", msgs[0].Role)
			}
			if msgs[len(msgs)-1].Content != tt.prompt {
				t.Errorf("last message content = %q, want %q", msgs[len(msgs)-1].Content, tt.prompt)
			}
		})
	}
}

func TestGenerateTitle(t *testing.T) {
	tests := []struct {
		name           string
		script         string
		serverResponse response
		serverStatus   int
		wantErr        bool
		wantTitle      string
	}{
		{
			name:   "successfulTitleGeneration",
			script: "Did you know that honey never spoils? Archaeologists have found 3000 year old honey in Egyptian tombs that was still edible!",
			serverResponse: response{
				ID: "title-123",
				Choices: []choice{
					{Message: Message{Role: "assistant", Content: "Honey That Lasts 3000 Years"}},
				},
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
			wantTitle:    "Honey That Lasts 3000 Years",
		},
		{
			name:   "emptyChoices",
			script: "Some script content",
			serverResponse: response{
				ID:      "title-456",
				Choices: []choice{},
			},
			serverStatus: http.StatusOK,
			wantErr:      true,
		},
		{
			name:         "serverError",
			script:       "Some script",
			serverStatus: http.StatusInternalServerError,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}

				w.WriteHeader(tt.serverStatus)
				if tt.serverStatus == http.StatusOK {
					_ = json.NewEncoder(w).Encode(tt.serverResponse)
				}
			}))
			defer server.Close()

			client := NewClient("test-key", Options{
				Model:        "deepseek-chat",
				SystemPrompt: "You are a script writer.",
			})
			client.baseURL = server.URL

			ctx := context.Background()
			got, err := client.GenerateTitle(ctx, tt.script)

			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateTitle() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && got != tt.wantTitle {
				t.Errorf("GenerateTitle() = %q, want %q", got, tt.wantTitle)
			}
		})
	}
}

func TestGenerateConversation(t *testing.T) {
	tests := []struct {
		name           string
		topic          string
		speakers       []string
		scriptLength   int
		hookDuration   int
		serverResponse response
		serverStatus   int
		wantErr        bool
		wantContent    string
	}{
		{
			name:         "successfulConversation",
			topic:        "aliens",
			speakers:     []string{"Host", "Guest"},
			scriptLength: 30,
			hookDuration: 5,
			serverResponse: response{
				ID: "conv-123",
				Choices: []choice{
					{Message: Message{Role: "assistant", Content: "Host: Do you believe in aliens?\nGuest: Absolutely!"}},
				},
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
			wantContent:  "Host: Do you believe in aliens?\nGuest: Absolutely!",
		},
		{
			name:         "threeSpeakers",
			topic:        "debate",
			speakers:     []string{"Alice", "Bob", "Charlie"},
			scriptLength: 60,
			hookDuration: 3,
			serverResponse: response{
				ID: "conv-456",
				Choices: []choice{
					{Message: Message{Role: "assistant", Content: "Alice: Welcome!\nBob: Thanks!\nCharlie: Great to be here!"}},
				},
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
			wantContent:  "Alice: Welcome!\nBob: Thanks!\nCharlie: Great to be here!",
		},
		{
			name:         "serverError",
			topic:        "test",
			speakers:     []string{"A", "B"},
			scriptLength: 30,
			hookDuration: 5,
			serverStatus: http.StatusInternalServerError,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.serverStatus)
				if tt.serverStatus == http.StatusOK {
					_ = json.NewEncoder(w).Encode(tt.serverResponse)
				}
			}))
			defer server.Close()

			client := NewClient("test-key", Options{Model: "deepseek-chat"})
			client.baseURL = server.URL

			ctx := context.Background()
			got, err := client.GenerateConversation(ctx, tt.topic, tt.speakers, tt.scriptLength, tt.hookDuration)

			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateConversation() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && got != tt.wantContent {
				t.Errorf("GenerateConversation() = %q, want %q", got, tt.wantContent)
			}
		})
	}
}

func TestGenerateScriptWithVisuals(t *testing.T) {
	tests := []struct {
		name           string
		topic          string
		scriptLength   int
		hookDuration   int
		serverResponse response
		serverStatus   int
		wantErr        bool
		wantScript     string
		wantVisuals    int
	}{
		{
			name:         "successfulWithVisuals",
			topic:        "space",
			scriptLength: 30,
			hookDuration: 5,
			serverResponse: response{
				ID: "vis-123",
				Choices: []choice{
					{Message: Message{Role: "assistant", Content: `{"script": "Space is vast.", "visuals": [{"search_query": "galaxy", "word_index": 2}]}`}},
				},
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
			wantScript:   "Space is vast.",
			wantVisuals:  1,
		},
		{
			name:         "markdownWrapped",
			topic:        "nature",
			scriptLength: 30,
			hookDuration: 5,
			serverResponse: response{
				ID: "vis-456",
				Choices: []choice{
					{Message: Message{Role: "assistant", Content: "```json\n{\"script\": \"Nature is beautiful.\", \"visuals\": []}\n```"}},
				},
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
			wantScript:   "Nature is beautiful.",
			wantVisuals:  0,
		},
		{
			name:         "invalidJSON",
			topic:        "tech",
			scriptLength: 30,
			hookDuration: 5,
			serverResponse: response{
				ID: "vis-789",
				Choices: []choice{
					{Message: Message{Role: "assistant", Content: "Just a plain script about tech."}},
				},
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
			wantScript:   "Just a plain script about tech.",
			wantVisuals:  0,
		},
		{
			name:         "serverError",
			topic:        "test",
			scriptLength: 30,
			hookDuration: 5,
			serverStatus: http.StatusInternalServerError,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.serverStatus)
				if tt.serverStatus == http.StatusOK {
					_ = json.NewEncoder(w).Encode(tt.serverResponse)
				}
			}))
			defer server.Close()

			client := NewClient("test-key", Options{Model: "deepseek-chat"})
			client.baseURL = server.URL

			ctx := context.Background()
			got, err := client.GenerateScriptWithVisuals(ctx, tt.topic, tt.scriptLength, tt.hookDuration)

			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateScriptWithVisuals() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if got.Script != tt.wantScript {
					t.Errorf("Script = %q, want %q", got.Script, tt.wantScript)
				}
				if len(got.Visuals) != tt.wantVisuals {
					t.Errorf("Visuals count = %d, want %d", len(got.Visuals), tt.wantVisuals)
				}
			}
		})
	}
}

func TestCleanJSONResponse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plainJSON",
			input: `{"script": "test"}`,
			want:  `{"script": "test"}`,
		},
		{
			name:  "markdownJSONBlock",
			input: "```json\n{\"script\": \"test\"}\n```",
			want:  `{"script": "test"}`,
		},
		{
			name:  "markdownPlainBlock",
			input: "```\n{\"script\": \"test\"}\n```",
			want:  `{"script": "test"}`,
		},
		{
			name:  "withWhitespace",
			input: "  \n```json\n{\"script\": \"test\"}\n```  \n",
			want:  `{"script": "test"}`,
		},
		{
			name:  "emptyString",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanJSONResponse(tt.input)
			if got != tt.want {
				t.Errorf("cleanJSONResponse() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVisualCueStruct(t *testing.T) {
	cue := VisualCue{
		Keyword:     "cat",
		SearchQuery: "cute cat",
	}

	if cue.Keyword != "cat" {
		t.Errorf("Keyword = %q, want %q", cue.Keyword, "cat")
	}
	if cue.SearchQuery != "cute cat" {
		t.Errorf("SearchQuery = %q, want %q", cue.SearchQuery, "cute cat")
	}
}
