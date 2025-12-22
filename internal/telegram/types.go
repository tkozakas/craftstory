package telegram

type Update struct {
	UpdateID      int            `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

type Message struct {
	MessageID int    `json:"message_id"`
	From      *User  `json:"from"`
	Chat      *Chat  `json:"chat"`
	Text      string `json:"text"`
}

type User struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	UserName  string `json:"username"`
}

type Chat struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    *User    `json:"from"`
	Message *Message `json:"message"`
	Data    string   `json:"data"`
}

type MessageResponse struct {
	MessageID int   `json:"message_id"`
	Chat      *Chat `json:"chat"`
}

type InlineKeyboard struct {
	InlineKeyboard [][]InlineButton `json:"inline_keyboard"`
}

type InlineButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

type Reviewer struct {
	ChatID   int64  `json:"chat_id"`
	UserName string `json:"username"`
	Name     string `json:"name"`
}

func NewApprovalKeyboard(approveData, rejectData string) *InlineKeyboard {
	return &InlineKeyboard{
		InlineKeyboard: [][]InlineButton{
			{
				{Text: "✅ Upload", CallbackData: approveData},
				{Text: "❌ Reject", CallbackData: rejectData},
			},
		},
	}
}
