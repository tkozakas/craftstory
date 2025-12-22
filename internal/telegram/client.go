package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"time"
)

const (
	baseURL        = "https://api.telegram.org/bot"
	defaultTimeout = 35 * time.Second
)

type Client struct {
	token      string
	httpClient *http.Client
	baseURL    string
}

func NewClient(token string) *Client {
	return &Client{
		token:      token,
		httpClient: &http.Client{Timeout: defaultTimeout},
		baseURL:    baseURL + token,
	}
}

func (c *Client) SendMessage(chatID int64, text string) error {
	payload := map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}
	return c.postJSON("/sendMessage", payload)
}

func (c *Client) SendVideo(chatID int64, videoPath string, caption string, keyboard *InlineKeyboard) (*MessageResponse, error) {
	file, err := os.Open(videoPath)
	if err != nil {
		return nil, fmt.Errorf("open video: %w", err)
	}
	defer func() { _ = file.Close() }()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	_ = writer.WriteField("chat_id", fmt.Sprintf("%d", chatID))
	if caption != "" {
		_ = writer.WriteField("caption", caption)
		_ = writer.WriteField("parse_mode", "Markdown")
	}

	if keyboard != nil {
		keyboardJSON, err := json.Marshal(keyboard)
		if err != nil {
			return nil, fmt.Errorf("marshal keyboard: %w", err)
		}
		_ = writer.WriteField("reply_markup", string(keyboardJSON))
	}

	part, err := writer.CreateFormFile("video", file.Name())
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("copy video: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close writer: %w", err)
	}

	resp, err := c.httpClient.Post(c.baseURL+"/sendVideo", writer.FormDataContentType(), &buf)
	if err != nil {
		return nil, fmt.Errorf("send video: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var result struct {
		Ok          bool            `json:"ok"`
		Result      MessageResponse `json:"result"`
		Description string          `json:"description"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if !result.Ok {
		return nil, fmt.Errorf("telegram error: %s", result.Description)
	}

	return &result.Result, nil
}

func (c *Client) EditMessageReplyMarkup(chatID int64, messageID int, keyboard *InlineKeyboard) error {
	payload := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
	}
	if keyboard != nil {
		payload["reply_markup"] = keyboard
	} else {
		payload["reply_markup"] = &InlineKeyboard{InlineKeyboard: [][]InlineButton{}}
	}
	return c.postJSON("/editMessageReplyMarkup", payload)
}

func (c *Client) EditMessageCaption(chatID int64, messageID int, caption string) error {
	payload := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"caption":    caption,
		"parse_mode": "Markdown",
	}
	return c.postJSON("/editMessageCaption", payload)
}

func (c *Client) AnswerCallbackQuery(callbackID string, text string) error {
	payload := map[string]any{
		"callback_query_id": callbackID,
		"text":              text,
	}
	return c.postJSON("/answerCallbackQuery", payload)
}

func (c *Client) GetUpdates(offset int) ([]Update, error) {
	url := fmt.Sprintf("%s/getUpdates?offset=%d&timeout=30", c.baseURL, offset)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Ok     bool     `json:"ok"`
		Result []Update `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return result.Result, nil
}

func (c *Client) GetChatID() (int64, string, error) {
	updates, err := c.GetUpdates(0)
	if err != nil {
		return 0, "", fmt.Errorf("get updates: %w", err)
	}

	for _, update := range updates {
		if update.Message != nil && update.Message.Chat != nil {
			chat := update.Message.Chat
			name := chat.Title
			if name == "" && update.Message.From != nil {
				name = update.Message.From.FirstName
				if update.Message.From.UserName != "" {
					name += " (@" + update.Message.From.UserName + ")"
				}
			}
			return chat.ID, name, nil
		}
	}

	return 0, "", fmt.Errorf("no messages found - send a message to your bot first")
}

func (c *Client) postJSON(endpoint string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Post(c.baseURL+endpoint, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed: %s - %s", resp.Status, string(body))
	}

	return nil
}
