package telegram_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/flow"
	"github.com/px1/home-ctl/internal/telegram"
)

func TestShowFlowMessage(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setupTestEnv failed: %v", err)
	}
	ctx := context.Background()

	key := flow.Key{UserID: 101, ChatID: -1001234567890, ThreadID: 51}
	if err := env.flow.Set(ctx, key, "step1", nil, 10*time.Minute); err != nil {
		t.Fatalf("flow.Set failed: %v", err)
	}

	state, err := env.flow.Get(ctx, key, nil)
	if err != nil {
		t.Fatalf("flow.Get failed: %v", err)
	}

	// 1. Initial message send (state.MessageID == 0)
	markup := &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: "Option 1", CallbackData: "opt:1"}},
		},
	}
	err = env.router.ShowFlowMessageForTest(ctx, state, "Initial prompt", markup)
	if err != nil {
		t.Fatalf("ShowFlowMessageForTest failed on initial send: %v", err)
	}
	if state.MessageID == 0 {
		t.Fatalf("expected state.MessageID to be set after send, got 0")
	}
	initialMsgID := state.MessageID

	// Verify message ID was persisted
	persisted, err := env.flow.Get(ctx, key, nil)
	if err != nil || persisted.MessageID != initialMsgID {
		t.Fatalf("expected persisted message_id %d, got %d, err=%v", initialMsgID, persisted.MessageID, err)
	}

	// 2. Normal step edits the same id
	sentCountBefore := len(env.client.sentMessages)
	err = env.router.ShowFlowMessageForTest(ctx, state, "Step 2 prompt", markup)
	if err != nil {
		t.Fatalf("ShowFlowMessageForTest failed on normal edit: %v", err)
	}
	if state.MessageID != initialMsgID {
		t.Fatalf("expected messageID %d, got %d", initialMsgID, state.MessageID)
	}
	if len(env.client.sentMessages) != sentCountBefore {
		t.Fatalf("expected no new message sent on edit, got new messages")
	}
	edited := env.client.LastEdited()
	if edited == nil || edited.MessageID != initialMsgID || edited.Text != "Step 2 prompt" {
		t.Fatalf("unexpected edited message: %+v", edited)
	}

	// 3. Edit error "message is not modified" is not treated as failure and does not send replacement
	env.client.onEditMessageText = func(_ context.Context, _ int64, _ int64, _ string, _ *telegram.EditOptions) error {
		return errors.New("Bad Request: message is not modified: specified new message content and reply markup are exactly the same")
	}
	err = env.router.ShowFlowMessageForTest(ctx, state, "Step 2 prompt", markup)
	if err != nil {
		t.Fatalf("expected no error for 'message is not modified', got %v", err)
	}
	if len(env.client.sentMessages) != sentCountBefore {
		t.Fatalf("expected no replacement message for 'message is not modified'")
	}
	if state.MessageID != initialMsgID {
		t.Fatalf("expected messageID %d to remain unchanged, got %d", initialMsgID, state.MessageID)
	}

	// 4. Failing edit sends a replacement and updates the stored id
	env.client.onEditMessageText = func(_ context.Context, _ int64, _ int64, _ string, _ *telegram.EditOptions) error {
		return errors.New("Bad Request: message to edit not found")
	}
	err = env.router.ShowFlowMessageForTest(ctx, state, "Step 3 replacement prompt", markup)
	if err != nil {
		t.Fatalf("expected fallback send to succeed, got: %v", err)
	}
	if state.MessageID == initialMsgID {
		t.Fatalf("expected replacement messageID to differ from initial %d", initialMsgID)
	}
	newMsgID := state.MessageID
	lastSent := env.client.LastSent()
	if lastSent == nil || lastSent.MessageID != newMsgID || lastSent.Text != "Step 3 replacement prompt" {
		t.Fatalf("unexpected replacement message: %+v", lastSent)
	}

	// Verify updated ID is persisted in flow store
	persistedAfterFallback, err := env.flow.Get(ctx, key, nil)
	if err != nil || persistedAfterFallback.MessageID != newMsgID {
		t.Fatalf("expected persisted message_id %d after fallback, got %d, err=%v", newMsgID, persistedAfterFallback.MessageID, err)
	}

	// 5. Force send mode always sends and updates message ID
	env.client.onEditMessageText = nil
	err = env.router.ShowFlowMessageForTest(ctx, state, "Force send prompt", markup, true)
	if err != nil {
		t.Fatalf("ShowFlowMessageForTest failed with forceSend: %v", err)
	}
	if state.MessageID == newMsgID {
		t.Fatalf("expected forceSend to create new message ID, got same %d", newMsgID)
	}
}
