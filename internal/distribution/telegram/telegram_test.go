package telegram

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(server *httptest.Server) *Client {
	return &Client{
		token:      "test-token",
		httpClient: server.Client(),
		baseURL:    server.URL,
	}
}

func TestSendMessage(t *testing.T) {
	tests := []struct {
		name       string
		chatID     int64
		text       string
		statusCode int
		wantErr    bool
	}{
		{
			name:       "successfulSend",
			chatID:     12345,
			text:       "Hello, world!",
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "serverError",
			chatID:     12345,
			text:       "Hello",
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
		},
		{
			name:       "unauthorized",
			chatID:     12345,
			text:       "Hello",
			statusCode: http.StatusUnauthorized,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/sendMessage" {
					t.Errorf("expected path /sendMessage, got %s", r.URL.Path)
				}
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
				}

				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Errorf("failed to decode request body: %v", err)
				}
				if payload["chat_id"].(float64) != float64(tt.chatID) {
					t.Errorf("expected chat_id %d, got %v", tt.chatID, payload["chat_id"])
				}
				if payload["text"].(string) != tt.text {
					t.Errorf("expected text %q, got %v", tt.text, payload["text"])
				}

				w.WriteHeader(tt.statusCode)
				if tt.statusCode == http.StatusOK {
					_, _ = w.Write([]byte(`{"ok":true}`))
				} else {
					_, _ = w.Write([]byte(`{"ok":false,"description":"error"}`))
				}
			}))
			defer server.Close()

			client := newTestClient(server)
			err := client.SendMessage(tt.chatID, tt.text)

			if (err != nil) != tt.wantErr {
				t.Errorf("SendMessage() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetUpdates(t *testing.T) {
	tests := []struct {
		name        string
		offset      int
		response    string
		wantUpdates int
		wantErr     bool
	}{
		{
			name:   "successfulWithUpdates",
			offset: 0,
			response: `{
				"ok": true,
				"result": [
					{"update_id": 1, "message": {"message_id": 1, "text": "Hello", "chat": {"id": 123}}},
					{"update_id": 2, "message": {"message_id": 2, "text": "World", "chat": {"id": 123}}}
				]
			}`,
			wantUpdates: 2,
			wantErr:     false,
		},
		{
			name:        "successfulEmpty",
			offset:      100,
			response:    `{"ok": true, "result": []}`,
			wantUpdates: 0,
			wantErr:     false,
		},
		{
			name:        "invalidJSON",
			offset:      0,
			response:    `{invalid json}`,
			wantUpdates: 0,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasPrefix(r.URL.Path, "/getUpdates") {
					t.Errorf("expected path /getUpdates, got %s", r.URL.Path)
				}
				if r.Method != http.MethodGet {
					t.Errorf("expected GET, got %s", r.Method)
				}

				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tt.response))
			}))
			defer server.Close()

			client := newTestClient(server)
			updates, err := client.GetUpdates(tt.offset)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetUpdates() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(updates) != tt.wantUpdates {
				t.Errorf("GetUpdates() returned %d updates, want %d", len(updates), tt.wantUpdates)
			}
		})
	}
}

func TestAnswerCallbackQuery(t *testing.T) {
	tests := []struct {
		name       string
		callbackID string
		text       string
		statusCode int
		wantErr    bool
	}{
		{
			name:       "successfulAnswer",
			callbackID: "callback123",
			text:       "Success!",
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "serverError",
			callbackID: "callback123",
			text:       "Error",
			statusCode: http.StatusBadRequest,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/answerCallbackQuery" {
					t.Errorf("expected path /answerCallbackQuery, got %s", r.URL.Path)
				}

				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Errorf("failed to decode request body: %v", err)
				}
				if payload["callback_query_id"].(string) != tt.callbackID {
					t.Errorf("expected callback_query_id %q, got %v", tt.callbackID, payload["callback_query_id"])
				}

				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(`{"ok":true}`))
			}))
			defer server.Close()

			client := newTestClient(server)
			err := client.AnswerCallbackQuery(tt.callbackID, tt.text)

			if (err != nil) != tt.wantErr {
				t.Errorf("AnswerCallbackQuery() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEditMessageReplyMarkup(t *testing.T) {
	tests := []struct {
		name       string
		chatID     int64
		messageID  int
		keyboard   *InlineKeyboard
		statusCode int
		wantErr    bool
	}{
		{
			name:       "removeKeyboard",
			chatID:     12345,
			messageID:  100,
			keyboard:   nil,
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:      "setKeyboard",
			chatID:    12345,
			messageID: 100,
			keyboard: &InlineKeyboard{
				InlineKeyboard: [][]InlineButton{
					{{Text: "Button", CallbackData: "data"}},
				},
			},
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "serverError",
			chatID:     12345,
			messageID:  100,
			keyboard:   nil,
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/editMessageReplyMarkup" {
					t.Errorf("expected path /editMessageReplyMarkup, got %s", r.URL.Path)
				}

				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(`{"ok":true}`))
			}))
			defer server.Close()

			client := newTestClient(server)
			err := client.EditMessageReplyMarkup(tt.chatID, tt.messageID, tt.keyboard)

			if (err != nil) != tt.wantErr {
				t.Errorf("EditMessageReplyMarkup() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEditMessageCaption(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/editMessageCaption" {
			t.Errorf("expected path /editMessageCaption, got %s", r.URL.Path)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}

		if payload["chat_id"].(float64) != 12345 {
			t.Errorf("expected chat_id 12345, got %v", payload["chat_id"])
		}
		if payload["message_id"].(float64) != 100 {
			t.Errorf("expected message_id 100, got %v", payload["message_id"])
		}
		if payload["caption"].(string) != "New caption" {
			t.Errorf("expected caption 'New caption', got %v", payload["caption"])
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := newTestClient(server)
	err := client.EditMessageCaption(12345, 100, "New caption")

	if err != nil {
		t.Errorf("EditMessageCaption() error = %v", err)
	}
}

func TestGetChatID(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantID   int64
		wantName string
		wantErr  bool
	}{
		{
			name: "chatWithTitle",
			response: `{
				"ok": true,
				"result": [
					{"update_id": 1, "message": {"message_id": 1, "chat": {"id": 12345, "title": "Test Group"}}}
				]
			}`,
			wantID:   12345,
			wantName: "Test Group",
			wantErr:  false,
		},
		{
			name: "privateChat",
			response: `{
				"ok": true,
				"result": [
					{"update_id": 1, "message": {"message_id": 1, "from": {"first_name": "John", "username": "john123"}, "chat": {"id": 67890}}}
				]
			}`,
			wantID:   67890,
			wantName: "John (@john123)",
			wantErr:  false,
		},
		{
			name:     "noMessages",
			response: `{"ok": true, "result": []}`,
			wantID:   0,
			wantName: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tt.response))
			}))
			defer server.Close()

			client := newTestClient(server)
			id, name, err := client.GetChatID()

			if (err != nil) != tt.wantErr {
				t.Errorf("GetChatID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if id != tt.wantID {
					t.Errorf("GetChatID() id = %d, want %d", id, tt.wantID)
				}
				if name != tt.wantName {
					t.Errorf("GetChatID() name = %q, want %q", name, tt.wantName)
				}
			}
		})
	}
}

func TestNewApprovalKeyboard(t *testing.T) {
	keyboard := NewApprovalKeyboard("approve", "reject")

	if keyboard == nil {
		t.Fatal("NewApprovalKeyboard() returned nil")
	}

	if len(keyboard.InlineKeyboard) != 1 {
		t.Errorf("expected 1 row, got %d", len(keyboard.InlineKeyboard))
	}

	row := keyboard.InlineKeyboard[0]
	if len(row) != 2 {
		t.Errorf("expected 2 buttons, got %d", len(row))
	}

	if row[0].CallbackData != "approve" {
		t.Errorf("expected approve callback, got %q", row[0].CallbackData)
	}
	if row[1].CallbackData != "reject" {
		t.Errorf("expected reject callback, got %q", row[1].CallbackData)
	}
}
