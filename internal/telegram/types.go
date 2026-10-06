// Package telegram provides the interaction adapter, Telegram Bot API client,
// topic routers, and inline keyboard workflows.
package telegram

// User represents a Telegram user.
type User struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	Username  string `json:"username,omitempty"`
}

// Chat represents a Telegram chat.
type Chat struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"` // "private", "group", "supergroup"
	Title string `json:"title,omitempty"`
}

// InlineKeyboardButton represents a button in an inline keyboard.
type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
}

// InlineKeyboardMarkup represents an inline keyboard markup.
type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

// Message represents a Telegram message.
type Message struct {
	MessageID       int64                 `json:"message_id"`
	From            *User                 `json:"from,omitempty"`
	Chat            Chat                  `json:"chat"`
	MessageThreadID int64                 `json:"message_thread_id,omitempty"`
	Text            string                `json:"text,omitempty"`
	ReplyMarkup     *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

// CallbackQuery represents an incoming callback query from an inline keyboard button.
type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message,omitempty"`
	Data    string   `json:"data"`
}

// Update represents an incoming update from Telegram long polling.
type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

// SendOptions specifies optional parameters when sending messages.
type SendOptions struct {
	ThreadID    int64
	ReplyMarkup *InlineKeyboardMarkup
	ParseMode   string
}

// EditOptions specifies optional parameters when editing messages.
type EditOptions struct {
	ReplyMarkup *InlineKeyboardMarkup
	ParseMode   string
}
