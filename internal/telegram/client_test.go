package telegram_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/telegram"
)

type mockUpdateStore struct {
	mu        sync.Mutex
	processed map[int64]bool
	failCheck bool
	failMark  bool
}

func newMockUpdateStore() *mockUpdateStore {
	return &mockUpdateStore{processed: make(map[int64]bool)}
}

func (m *mockUpdateStore) IsUpdateProcessed(_ context.Context, updateID int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failCheck {
		return false, errors.New("check failure")
	}
	return m.processed[updateID], nil
}

func (m *mockUpdateStore) MarkUpdateProcessed(_ context.Context, updateID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failMark {
		return errors.New("mark failure")
	}
	m.processed[updateID] = true
	return nil
}

func TestHTTPClient_SendMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sendMessage" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}

		if req["chat_id"].(float64) != 12345 {
			t.Errorf("expected chat_id 12345, got %v", req["chat_id"])
		}
		if req["text"].(string) != "Hello world" {
			t.Errorf("expected text 'Hello world', got %v", req["text"])
		}
		if req["message_thread_id"].(float64) != 42 {
			t.Errorf("expected thread_id 42, got %v", req["message_thread_id"])
		}

		resp := map[string]interface{}{
			"ok": true,
			"result": map[string]interface{}{
				"message_id": 999,
				"chat": map[string]interface{}{
					"id":   12345,
					"type": "supergroup",
				},
				"text": "Hello world",
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := telegram.NewHTTPClientWithBaseURL(server.URL)
	msgID, err := client.SendMessage(context.Background(), 12345, "Hello world", &telegram.SendOptions{
		ThreadID: 42,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msgID != 999 {
		t.Errorf("expected msgID 999, got %d", msgID)
	}
}

func TestHTTPClient_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]interface{}{
			"ok":          false,
			"description": "Bad Request: chat not found",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := telegram.NewHTTPClientWithBaseURL(server.URL)
	_, err := client.SendMessage(context.Background(), 12345, "Hello", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "telegram api error: Bad Request: chat not found" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestHTTPClient_EditMessageText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/editMessageText" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["message_id"].(float64) != 555 {
			t.Errorf("expected message_id 555, got %v", req["message_id"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
	}))
	defer server.Close()

	client := telegram.NewHTTPClientWithBaseURL(server.URL)
	err := client.EditMessageText(context.Background(), 12345, 555, "Updated", &telegram.EditOptions{
		ParseMode: "Markdown",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHTTPClient_DeleteMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/deleteMessage" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["chat_id"].(float64) != 12345 || req["message_id"].(float64) != 555 {
			t.Errorf("unexpected body: %v", req)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "result": true})
	}))
	defer server.Close()

	client := telegram.NewHTTPClientWithBaseURL(server.URL)
	if err := client.DeleteMessage(context.Background(), 12345, 555); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHTTPClient_DeleteMessageAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":          false,
			"description": "Bad Request: message can't be deleted for everyone",
		})
	}))
	defer server.Close()

	client := telegram.NewHTTPClientWithBaseURL(server.URL)
	err := client.DeleteMessage(context.Background(), 12345, 555)
	if err == nil || err.Error() != "telegram api error: Bad Request: message can't be deleted for everyone" {
		t.Fatalf("expected api error, got %v", err)
	}
}

func TestHTTPClient_AnswerCallbackQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/answerCallbackQuery" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["callback_query_id"].(string) != "cb123" {
			t.Errorf("expected cb123, got %v", req["callback_query_id"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
	}))
	defer server.Close()

	client := telegram.NewHTTPClientWithBaseURL(server.URL)
	err := client.AnswerCallbackQuery(context.Background(), "cb123", "Alert text", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHTTPClient_GetUpdates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/getUpdates" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["offset"].(float64) != 100 {
			t.Errorf("expected offset 100, got %v", req["offset"])
		}
		resp := map[string]interface{}{
			"ok": true,
			"result": []map[string]interface{}{
				{
					"update_id": 100,
					"message": map[string]interface{}{
						"message_id": 1,
						"chat": map[string]interface{}{
							"id":   123,
							"type": "private",
						},
						"text": "test",
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := telegram.NewHTTPClientWithBaseURL(server.URL)
	updates, err := client.GetUpdates(context.Background(), 100, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updates) != 1 || updates[0].UpdateID != 100 {
		t.Fatalf("unexpected updates result: %+v", updates)
	}
}

func TestHTTPClient_SetMyCommands(t *testing.T) {
	t.Run("chat scope with language code", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/setMyCommands" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			var req map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("failed to decode request body: %v", err)
			}

			scope, ok := req["scope"].(map[string]interface{})
			if !ok {
				t.Fatalf("missing or invalid scope: %v", req["scope"])
			}
			if scope["type"] != "chat" {
				t.Errorf("expected scope type 'chat', got %v", scope["type"])
			}
			if scope["chat_id"].(float64) != -1003916572365 {
				t.Errorf("expected chat_id -1003916572365, got %v", scope["chat_id"])
			}

			if req["language_code"] != "en" {
				t.Errorf("expected language_code 'en', got %v", req["language_code"])
			}

			cmds, ok := req["commands"].([]interface{})
			if !ok || len(cmds) != 2 {
				t.Fatalf("expected 2 commands, got %v", req["commands"])
			}
			cmd0 := cmds[0].(map[string]interface{})
			if cmd0["command"] != "add" || cmd0["description"] != "Add item" {
				t.Errorf("unexpected command 0: %v", cmd0)
			}

			_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}))
		defer server.Close()

		client := telegram.NewHTTPClientWithBaseURL(server.URL)
		err := client.SetMyCommands(context.Background(),
			telegram.BotCommandScope{Type: "chat", ChatID: -1003916572365},
			"en",
			[]telegram.BotCommand{
				{Command: "add", Description: "Add item"},
				{Command: "list", Description: "List items"},
			},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("private scope with empty language code", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/setMyCommands" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			var req map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("failed to decode request body: %v", err)
			}

			scope, ok := req["scope"].(map[string]interface{})
			if !ok {
				t.Fatalf("missing or invalid scope: %v", req["scope"])
			}
			if scope["type"] != "all_private_chats" {
				t.Errorf("expected scope type 'all_private_chats', got %v", scope["type"])
			}
			if _, hasChatID := scope["chat_id"]; hasChatID {
				t.Errorf("expected chat_id to be omitted, got %v", scope["chat_id"])
			}

			if _, hasLang := req["language_code"]; hasLang {
				t.Errorf("expected language_code to be omitted for empty string, got %v", req["language_code"])
			}

			cmds, ok := req["commands"].([]interface{})
			if !ok || len(cmds) != 1 {
				t.Fatalf("expected 1 command, got %v", req["commands"])
			}

			_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
		}))
		defer server.Close()

		client := telegram.NewHTTPClientWithBaseURL(server.URL)
		err := client.SetMyCommands(context.Background(),
			telegram.BotCommandScope{Type: "all_private_chats"},
			"",
			[]telegram.BotCommand{
				{Command: "start", Description: "Start bot"},
			},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("api error returned", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			resp := map[string]interface{}{
				"ok":          false,
				"description": "Bad Request: chat not found",
			}
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := telegram.NewHTTPClientWithBaseURL(server.URL)
		err := client.SetMyCommands(context.Background(),
			telegram.BotCommandScope{Type: "chat", ChatID: 123},
			"",
			nil,
		)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "telegram api error: Bad Request: chat not found") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}

func TestDispatcher_Deduplication(t *testing.T) {
	store := newMockUpdateStore()
	handled := make([]int64, 0)
	var mu sync.Mutex

	handler := telegram.UpdateHandlerFunc(func(_ context.Context, u telegram.Update) error {
		mu.Lock()
		defer mu.Unlock()
		handled = append(handled, u.UpdateID)
		return nil
	})

	disp := telegram.NewDispatcher(nil, store, handler, nil)

	// Process first time
	u1 := telegram.Update{UpdateID: 42}
	if err := disp.ProcessUpdate(context.Background(), u1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Redelivered update
	if err := disp.ProcessUpdate(context.Background(), u1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	count := len(handled)
	mu.Unlock()

	if count != 1 {
		t.Fatalf("expected update to be handled exactly once, got %d times", count)
	}
}

type fakeClientForPolling struct {
	updates []telegram.Update
}

func (f *fakeClientForPolling) SendMessage(_ context.Context, _ int64, _ string, _ *telegram.SendOptions) (int64, error) {
	return 0, nil
}
func (f *fakeClientForPolling) EditMessageText(_ context.Context, _ int64, _ int64, _ string, _ *telegram.EditOptions) error {
	return nil
}
func (f *fakeClientForPolling) EditMessageReplyMarkup(_ context.Context, _ int64, _ int64, _ *telegram.InlineKeyboardMarkup) error {
	return nil
}
func (f *fakeClientForPolling) DeleteMessage(_ context.Context, _ int64, _ int64) error {
	return nil
}
func (f *fakeClientForPolling) AnswerCallbackQuery(_ context.Context, _ string, _ string, _ bool) error {
	return nil
}
func (f *fakeClientForPolling) GetUpdates(_ context.Context, _ int64, _ int) ([]telegram.Update, error) {
	return f.updates, nil
}
func (f *fakeClientForPolling) SetMyCommands(_ context.Context, _ telegram.BotCommandScope, _ string, _ []telegram.BotCommand) error {
	return nil
}

func TestDispatcher_PollOnce(t *testing.T) {
	store := newMockUpdateStore()
	var handled []int64

	handler := telegram.UpdateHandlerFunc(func(_ context.Context, u telegram.Update) error {
		handled = append(handled, u.UpdateID)
		return nil
	})

	client := &fakeClientForPolling{
		updates: []telegram.Update{
			{UpdateID: 10},
			{UpdateID: 11},
			{UpdateID: 12},
		},
	}

	disp := telegram.NewDispatcher(client, store, handler, nil)

	nextOffset, err := disp.PollOnce(context.Background(), 0, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nextOffset != 13 {
		t.Errorf("expected nextOffset 13, got %d", nextOffset)
	}
	if len(handled) != 3 {
		t.Fatalf("expected 3 handled updates, got %d", len(handled))
	}

	// Poll again with same updates (redelivered)
	handled = nil
	nextOffset, err = disp.PollOnce(context.Background(), 10, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nextOffset != 13 {
		t.Errorf("expected nextOffset 13, got %d", nextOffset)
	}
	if len(handled) != 0 {
		t.Errorf("expected 0 handled on redelivery, got %d", len(handled))
	}
}

func TestDispatcher_Start_Cancel(t *testing.T) {
	client := &fakeClientForPolling{updates: nil}
	disp := telegram.NewDispatcher(client, nil, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := disp.Start(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}
