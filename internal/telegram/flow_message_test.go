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

	markup := &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: "Option 1", CallbackData: "opt:1"}},
		},
	}
	// 1. Initial message send
	initialMsgID := flowMessageTestInitialSend(t, env, key, state, markup)
	sentCountBefore := len(env.client.sentMessages)
	flowMessageTestNormalEdit(t, env, state, markup, initialMsgID, sentCountBefore)
	flowMessageTestNotModified(t, env, state, markup, initialMsgID, sentCountBefore)
	newMsgID := flowMessageTestFailedEditFallback(t, env, key, state, markup, initialMsgID)
	flowMessageTestForceSend(t, env, state, markup, newMsgID)
}

// flowMessageTestInitialSend covers step 1: the first show sends a message and persists its ID.
func flowMessageTestInitialSend(t *testing.T, env *testEnv, key flow.Key, state *flow.State, markup *telegram.InlineKeyboardMarkup) int64 {
	t.Helper()
	ctx := context.Background()
	err := env.router.ShowFlowMessageForTest(ctx, state, "Initial prompt", markup)
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
	return initialMsgID
}

// flowMessageTestNormalEdit covers step 2: a normal step edits the same message ID.
func flowMessageTestNormalEdit(t *testing.T, env *testEnv, state *flow.State, markup *telegram.InlineKeyboardMarkup, initialMsgID int64, sentCountBefore int) {
	t.Helper()
	ctx := context.Background()
	err := env.router.ShowFlowMessageForTest(ctx, state, "Step 2 prompt", markup)
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
}

// flowMessageTestNotModified covers step 3: "message is not modified" is not a failure and sends nothing.
func flowMessageTestNotModified(t *testing.T, env *testEnv, state *flow.State, markup *telegram.InlineKeyboardMarkup, initialMsgID int64, sentCountBefore int) {
	t.Helper()
	ctx := context.Background()
	env.client.onEditMessageText = func(_ context.Context, _ int64, _ int64, _ string, _ *telegram.EditOptions) error {
		return errors.New("Bad Request: message is not modified: specified new message content and reply markup are exactly the same")
	}
	err := env.router.ShowFlowMessageForTest(ctx, state, "Step 2 prompt", markup)
	if err != nil {
		t.Fatalf("expected no error for 'message is not modified', got %v", err)
	}
	if len(env.client.sentMessages) != sentCountBefore {
		t.Fatalf("expected no replacement message for 'message is not modified'")
	}
	if state.MessageID != initialMsgID {
		t.Fatalf("expected messageID %d to remain unchanged, got %d", initialMsgID, state.MessageID)
	}
}

// flowMessageTestFailedEditFallback covers step 4: a failing edit sends a replacement and updates the stored ID.
func flowMessageTestFailedEditFallback(t *testing.T, env *testEnv, key flow.Key, state *flow.State, markup *telegram.InlineKeyboardMarkup, initialMsgID int64) int64 {
	t.Helper()
	ctx := context.Background()
	env.client.onEditMessageText = func(_ context.Context, _ int64, _ int64, _ string, _ *telegram.EditOptions) error {
		return errors.New("Bad Request: message to edit not found")
	}
	err := env.router.ShowFlowMessageForTest(ctx, state, "Step 3 replacement prompt", markup)
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
	return newMsgID
}

// flowMessageTestForceSend covers step 5: force send mode always sends and updates the message ID.
func flowMessageTestForceSend(t *testing.T, env *testEnv, state *flow.State, markup *telegram.InlineKeyboardMarkup, newMsgID int64) {
	t.Helper()
	ctx := context.Background()
	env.client.onEditMessageText = nil
	err := env.router.ShowFlowMessageForTest(ctx, state, "Force send prompt", markup, true)
	if err != nil {
		t.Fatalf("ShowFlowMessageForTest failed with forceSend: %v", err)
	}
	if state.MessageID == newMsgID {
		t.Fatalf("expected forceSend to create new message ID, got same %d", newMsgID)
	}
}
