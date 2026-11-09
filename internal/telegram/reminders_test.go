package telegram_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/recurrence"
	"github.com/px1/home-ctl/internal/telegram"
)

func pressReminderCallback(ctx context.Context, env *testEnv, updateID int64, userID int64, data string) error {
	msgID := int64(101)
	text := ""
	if last := env.client.LastSent(); last != nil {
		msgID = last.MessageID
		text = last.Text
	}
	if lastEdit := env.client.LastEdited(); lastEdit != nil && lastEdit.MessageID == msgID {
		text = lastEdit.Text
	}
	return env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: updateID,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   fmt.Sprintf("cb_%d", updateID),
			From: telegram.User{ID: userID, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       msgID,
				MessageThreadID: 63,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
				Text:            text,
			},
			Data: data,
		},
	})
}

// remindersFlowStart runs /add in the Reminders topic and checks the title prompt. Returns its message ID.
func remindersFlowStart(ctx context.Context, t *testing.T, env *testEnv) int64 {
	t.Helper()
	uAdd := telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID:       10,
			MessageThreadID: 63,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	}
	if err := env.router.HandleUpdate(ctx, uAdd); err != nil {
		t.Fatalf("/add failed: %v", err)
	}
	initialMsg := env.client.LastSent()
	if initialMsg == nil || !strings.Contains(initialMsg.Text, "title for the reminder") {
		t.Fatalf("expected title prompt, got: %+v", initialMsg)
	}
	return initialMsg.MessageID
}

// remindersFlowTitle types the title, which re-anchors the flow to a new emoji prompt. Returns the new flow message ID.
func remindersFlowTitle(ctx context.Context, t *testing.T, env *testEnv, initialMsgID int64) int64 {
	t.Helper()
	uTitle := telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID:       11,
			MessageThreadID: 63,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "Buy medicine",
		},
	}
	if err := env.router.HandleUpdate(ctx, uTitle); err != nil {
		t.Fatalf("title text failed: %v", err)
	}
	// A typed title re-anchors: the emoji step is a new message below the text and the title prompt is deleted.
	lastSent := env.client.LastSent()
	if lastSent == nil || lastSent.MessageID == initialMsgID || !strings.Contains(lastSent.Text, "Choose an emoji") {
		t.Fatalf("expected emoji prompt as a new message after the typed title, got: %+v", lastSent)
	}
	if deleted := env.client.DeletedMessages(); len(deleted) != 1 || deleted[0] != initialMsgID {
		t.Fatalf("expected title prompt %d deleted, got %v", initialMsgID, deleted)
	}
	return lastSent.MessageID // the active flow message from here on
}

// remindersFlowPressExpect presses one flow button and checks the flow message now shows the wanted prompt.
func remindersFlowPressExpect(ctx context.Context, t *testing.T, env *testEnv, flowMsgID, updateID int64, data, want, prompt string) {
	t.Helper()
	if err := pressReminderCallback(ctx, env, updateID, 101, data); err != nil {
		t.Fatalf("%s failed: %v", data, err)
	}
	lastEdit := env.client.LastEdited()
	if lastEdit == nil || lastEdit.MessageID != flowMsgID || !strings.Contains(lastEdit.Text, want) {
		t.Fatalf("expected %s prompt on message %d, got: %+v", prompt, flowMsgID, lastEdit)
	}
}

// remindersFlowConfirm presses confirm and checks the flow message lost its keyboard.
func remindersFlowConfirm(ctx context.Context, t *testing.T, env *testEnv, flowMsgID int64) {
	t.Helper()
	if err := pressReminderCallback(ctx, env, 8, 101, "fl:ok"); err != nil {
		t.Fatalf("confirm callback failed: %v", err)
	}
	lastEdit := env.client.LastEdited()
	if lastEdit == nil || lastEdit.MessageID != flowMsgID || len(lastEdit.Markup.InlineKeyboard) != 0 {
		t.Fatalf("expected closed keyboard after confirm, got: %+v", lastEdit)
	}
}

// remindersFlowVerifyMessages checks that the flow used exactly two messages and edited only the flow message.
func remindersFlowVerifyMessages(t *testing.T, env *testEnv, flowMsgID int64) {
	t.Helper()
	// Two messages: the initial prompt, then the step re-anchored below the typed answer; button steps edit that one
	if len(env.client.SentMessages()) != 2 {
		t.Fatalf("expected 2 messages sent, got %d", len(env.client.SentMessages()))
	}
	// Assert all edits targeted the same flowMsgID
	for _, em := range env.client.EditedMessages() {
		if em.MessageID != flowMsgID {
			t.Fatalf("expected edit to message %d, got %d", flowMsgID, em.MessageID)
		}
	}
}

// remindersFlowVerifyStored checks the one-off reminder stored for Bob and nothing for Alice.
func remindersFlowVerifyStored(ctx context.Context, t *testing.T, env *testEnv) {
	t.Helper()
	bobReminders, err := env.reminders.ListMyOpen(ctx, 102)
	if err != nil || len(bobReminders.Occurrences) != 1 {
		t.Fatalf("expected 1 reminder for Bob, got: %+v", bobReminders)
	}
	aliceReminders, _ := env.reminders.ListMyOpen(ctx, 101)
	if len(aliceReminders.Occurrences) != 0 {
		t.Fatalf("expected 0 reminders for Alice, got: %+v", aliceReminders)
	}
	occ := bobReminders.Occurrences[0]
	wantDue := time.Date(2026, 10, 9, 10, 0, 0, 0, env.clock.loc).UTC()
	if !occ.DueAt.Equal(wantDue) {
		t.Fatalf("expected due %v, got %v", wantDue, occ.DueAt)
	}
	if !occ.ScheduledAt.Equal(occ.DueAt) {
		t.Fatalf("expected scheduled_at == due_at, got scheduled_at=%v due_at=%v", occ.ScheduledAt, occ.DueAt)
	}
	if occ.Definition.RecurrenceInterval != nil {
		t.Fatalf("expected nil recurrence for one-off reminder, got: %+v", occ.Definition.RecurrenceInterval)
	}
}

// 6.1: One-off reminder for Bob (no interval or anchor asked), with one edited message throughout.
func TestRemindersFlow_OneOff(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}
	ctx := context.Background()

	flowMsgID := remindersFlowStart(ctx, t, env)
	flowMsgID = remindersFlowTitle(ctx, t, env, flowMsgID)

	// 2b. Skip the emoji: the target step edits the same message
	remindersFlowPressExpect(ctx, t, env, flowMsgID, 30, "fl:em:skip", "reminder is for", "target")
	// 3. Alice selects Bob (102) as target
	remindersFlowPressExpect(ctx, t, env, flowMsgID, 3, "fl:usr:102", "recurrence type", "recurrence type")
	// 4. Alice selects One time (fl:rt:once) -> skips interval, asks date
	remindersFlowPressExpect(ctx, t, env, flowMsgID, 4, "fl:rt:once", "first due date", "date")
	// 5. Date: Friday Oct 9 2026
	remindersFlowPressExpect(ctx, t, env, flowMsgID, 5, "fl:d:2026-10-09", "hour", "hour")
	// 6. Hour 10
	remindersFlowPressExpect(ctx, t, env, flowMsgID, 6, "fl:h:10", "minute", "minute")
	// 7. Minute 00 -> skips anchor, asks confirmation
	remindersFlowPressExpect(ctx, t, env, flowMsgID, 7, "fl:mi:00", "Confirm", "confirm")
	// 8. Confirm -> closes keyboard, creates occurrence for Bob
	remindersFlowConfirm(ctx, t, env, flowMsgID)

	remindersFlowVerifyMessages(t, env, flowMsgID)
	remindersFlowVerifyStored(ctx, t, env)
}

// 6.1: Daily every-2-days reminder whose next slot is two days after the first,
// with one edited message throughout.
func TestRemindersFlow_DailyEvery2Days(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}
	ctx := context.Background()

	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 10, MessageThreadID: 63,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "/add",
		},
	})
	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID: 11, MessageThreadID: 63,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "Take vitamins",
		},
	})
	// The typed title re-anchored the flow: the target step is the active message from here on.
	flowMsgID := env.client.LastSent().MessageID
	_ = pressReminderCallback(ctx, env, 3, 101, "fl:usr:102") // Bob
	_ = pressReminderCallback(ctx, env, 4, 101, "fl:rt:daily")
	_ = pressReminderCallback(ctx, env, 5, 101, "fl:iv:2")
	_ = pressReminderCallback(ctx, env, 6, 101, "fl:d:2026-10-10")
	_ = pressReminderCallback(ctx, env, 7, 101, "fl:h:08")
	_ = pressReminderCallback(ctx, env, 8, 101, "fl:mi:00")
	_ = pressReminderCallback(ctx, env, 9, 101, "fl:anc:scheduled")
	_ = pressReminderCallback(ctx, env, 10, 101, "fl:ok")

	// Verify one message throughout the button steps: the title prompt, then the target prompt sent below the typed title
	if len(env.client.SentMessages()) != 2 {
		t.Fatalf("expected 2 messages sent (title prompt, then target prompt), got %d", len(env.client.SentMessages()))
	}
	for _, em := range env.client.EditedMessages() {
		if em.MessageID != flowMsgID {
			t.Fatalf("expected edit to message %d, got %d", flowMsgID, em.MessageID)
		}
	}

	bobReminders, err := env.reminders.ListMyOpen(ctx, 102)
	if err != nil || len(bobReminders.Occurrences) != 1 {
		t.Fatalf("expected 1 reminder for Bob, got: %+v", bobReminders)
	}
	occ := bobReminders.Occurrences[0]
	firstSlot := time.Date(2026, 10, 10, 8, 0, 0, 0, env.clock.loc).UTC()
	if !occ.DueAt.Equal(firstSlot) {
		t.Fatalf("expected first slot %v, got %v", firstSlot, occ.DueAt)
	}

	rule := recurrence.Rule{Interval: 2, Unit: recurrence.UnitDay, Anchor: recurrence.AnchorScheduled}
	slot2, err := recurrence.NextSlot(rule, occ.ScheduledAt, occ.ScheduledAt, occ.ScheduledAt, env.clock.loc)
	if err != nil {
		t.Fatalf("NextSlot error: %v", err)
	}
	wantSlot2 := time.Date(2026, 10, 12, 8, 0, 0, 0, env.clock.loc).UTC()
	if !slot2.Equal(wantSlot2) {
		t.Fatalf("expected next slot two days after %v, got %v", wantSlot2, slot2)
	}
}

// Cancel in Reminders
func TestRemindersFlow_Cancel(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}
	ctx := context.Background()

	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 10, MessageThreadID: 63,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "/add",
		},
	})
	_ = pressReminderCallback(ctx, env, 2, 101, "fl:c")

	lastSent := env.client.LastSent()
	if lastSent == nil || !strings.Contains(lastSent.Text, "cancelled") {
		t.Fatalf("expected cancelled message, got: %+v", lastSent)
	}
	defs, _ := env.reminders.ListDefinitions(ctx)
	if len(defs) != 0 {
		t.Fatalf("expected 0 definitions, got %d", len(defs))
	}
}

// Abandon in Reminders
func TestRemindersFlow_AbandonCreatesNothing(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}
	ctx := context.Background()

	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 10, MessageThreadID: 63,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "/add",
		},
	})
	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID: 11, MessageThreadID: 63,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "Abandoned reminder",
		},
	})

	env.clock.now = env.clock.now.Add(35 * time.Minute)
	_ = pressReminderCallback(ctx, env, 3, 101, "fl:usr:102")

	lastAns := env.client.LastAnswer()
	if lastAns == nil || !strings.Contains(lastAns.Text, "expired") {
		t.Fatalf("expected expired alert, got: %+v", lastAns)
	}
	defs, _ := env.reminders.ListDefinitions(ctx)
	if len(defs) != 0 {
		t.Fatalf("expected 0 definitions, got %d", len(defs))
	}
}
