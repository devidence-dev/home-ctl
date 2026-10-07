package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client defines the interface for interacting with the Telegram Bot API.
type Client interface {
	SendMessage(ctx context.Context, chatID int64, text string, opts *SendOptions) (int64, error)
	EditMessageText(ctx context.Context, chatID int64, messageID int64, text string, opts *EditOptions) error
	AnswerCallbackQuery(ctx context.Context, callbackQueryID string, text string, showAlert bool) error
	GetUpdates(ctx context.Context, offset int64, timeout int) ([]Update, error)
	SetMyCommands(ctx context.Context, scope BotCommandScope, languageCode string, commands []BotCommand) error
}

// HTTPClient implements Client using raw net/http.
type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewHTTPClient creates a Telegram Client using the provided bot token.
func NewHTTPClient(token string) *HTTPClient {
	return NewHTTPClientWithBaseURL(fmt.Sprintf("https://api.telegram.org/bot%s", token))
}

// NewHTTPClientWithBaseURL creates a Client targeting a custom API base URL (for tests).
func NewHTTPClientWithBaseURL(baseURL string) *HTTPClient {
	return &HTTPClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result,omitempty"`
	Description string          `json:"description,omitempty"`
}

func (c *HTTPClient) call(ctx context.Context, method string, payload interface{}, out interface{}) error {
	var body bytes.Buffer
	if payload != nil {
		if err := json.NewEncoder(&body).Encode(payload); err != nil {
			return fmt.Errorf("failed to encode request: %w", err)
		}
	}

	url := fmt.Sprintf("%s/%s", c.baseURL, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return fmt.Errorf("failed to decode api response: %w", err)
	}

	if !apiResp.OK {
		return fmt.Errorf("telegram api error: %s", apiResp.Description)
	}

	if out != nil && len(apiResp.Result) > 0 {
		if err := json.Unmarshal(apiResp.Result, out); err != nil {
			return fmt.Errorf("failed to parse result: %w", err)
		}
	}

	return nil
}

// SendMessage sends a text message to a chat or topic.
func (c *HTTPClient) SendMessage(ctx context.Context, chatID int64, text string, opts *SendOptions) (int64, error) {
	req := map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	}
	if opts != nil {
		if opts.ThreadID != 0 {
			req["message_thread_id"] = opts.ThreadID
		}
		if opts.ReplyMarkup != nil {
			req["reply_markup"] = opts.ReplyMarkup
		}
		if opts.ParseMode != "" {
			req["parse_mode"] = opts.ParseMode
		}
	}

	var sent Message
	if err := c.call(ctx, "sendMessage", req, &sent); err != nil {
		return 0, err
	}
	return sent.MessageID, nil
}

// EditMessageText edits the text and markup of an existing message.
func (c *HTTPClient) EditMessageText(ctx context.Context, chatID int64, messageID int64, text string, opts *EditOptions) error {
	req := map[string]interface{}{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       text,
	}
	if opts != nil {
		if opts.ReplyMarkup != nil {
			req["reply_markup"] = opts.ReplyMarkup
		}
		if opts.ParseMode != "" {
			req["parse_mode"] = opts.ParseMode
		}
	}

	return c.call(ctx, "editMessageText", req, nil)
}

// AnswerCallbackQuery answers an inline keyboard callback query.
func (c *HTTPClient) AnswerCallbackQuery(ctx context.Context, callbackQueryID string, text string, showAlert bool) error {
	req := map[string]interface{}{
		"callback_query_id": callbackQueryID,
		"text":              text,
		"show_alert":        showAlert,
	}
	return c.call(ctx, "answerCallbackQuery", req, nil)
}

// GetUpdates fetches new updates using long polling.
func (c *HTTPClient) GetUpdates(ctx context.Context, offset int64, timeout int) ([]Update, error) {
	req := map[string]interface{}{
		"offset":  offset,
		"timeout": timeout,
	}
	var updates []Update
	if err := c.call(ctx, "getUpdates", req, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

// SetMyCommands changes the list of the bot's commands for the given scope and user language.
func (c *HTTPClient) SetMyCommands(ctx context.Context, scope BotCommandScope, languageCode string, commands []BotCommand) error {
	req := map[string]interface{}{
		"commands": commands,
		"scope":    scope,
	}
	if languageCode != "" {
		req["language_code"] = languageCode
	}
	return c.call(ctx, "setMyCommands", req, nil)
}
