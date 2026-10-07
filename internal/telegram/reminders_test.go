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

// 6.1: One-off reminder for Bob (no interval or anchor asked), with one edited message throughout.
func TestRemindersFlow_OneOff(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}
	ctx := context.Background()

	// 1. /add in Reminders topic
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
	flowMsgID := initialMsg.MessageID

	// 2. Alice types title
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
	lastEdit := env.client.LastEdited()
	if lastEdit == nil || lastEdit.MessageID != flowMsgID || !strings.Contains(lastEdit.Text, "reminder is for") {
		t.Fatalf("expected target prompt on message %d, got: %+v", flowMsgID, lastEdit)
	}

	// 3. Alice selects Bob (102) as target
	if err := pressReminderCallback(ctx, env, 3, 101, "fl:usr:102"); err != nil {
		t.Fatalf("target callback failed: %v", err)
	}
	lastEdit = env.client.LastEdited()
	if lastEdit == nil || lastEdit.MessageID != flowMsgID || !strings.Contains(lastEdit.Text, "recurrence type") {
		t.Fatalf("expected recurrence prompt on message %d, got: %+v", flowMsgID, lastEdit)
	}

	// 4. Alice selects One time (fl:rt:once) -> skips interval, asks date
	if err := pressReminderCallback(ctx, env, 4, 101, "fl:rt:once"); err != nil {
		t.Fatalf("recurrence type callback failed: %v", err)
	}
	lastEdit = env.client.LastEdited()
	if lastEdit == nil || lastEdit.MessageID != flowMsgID || !strings.Contains(lastEdit.Text, "first due date") {
		t.Fatalf("expected date prompt on message %d, got: %+v", flowMsgID, lastEdit)
	}

	// 5. Date: Friday Oct 9 2026
	if err := pressReminderCallback(ctx, env, 5, 101, "fl:d:2026-10-09"); err != nil {
		t.Fatalf("date callback failed: %v", err)
	}
	lastEdit = env.client.LastEdited()
	if lastEdit == nil || lastEdit.MessageID != flowMsgID || !strings.Contains(lastEdit.Text, "hour") {
		t.Fatalf("expected hour prompt on message %d, got: %+v", flowMsgID, lastEdit)
	}

	// 6. Hour 10
	if err := pressReminderCallback(ctx, env, 6, 101, "fl:h:10"); err != nil {
		t.Fatalf("hour callback failed: %v", err)
	}
	lastEdit = env.client.LastEdited()
	if lastEdit == nil || lastEdit.MessageID != flowMsgID || !strings.Contains(lastEdit.Text, "minute") {
		t.Fatalf("expected minute prompt on message %d, got: %+v", flowMsgID, lastEdit)
	}

	// 7. Minute 00 -> skips anchor, asks confirmation
	if err := pressReminderCallback(ctx, env, 7, 101, "fl:mi:00"); err != nil {
		t.Fatalf("minute callback failed: %v", err)
	}
	lastEdit = env.client.LastEdited()
	if lastEdit == nil || lastEdit.MessageID != flowMsgID || !strings.Contains(lastEdit.Text, "Confirm") {
		t.Fatalf("expected confirm prompt on message %d, got: %+v", flowMsgID, lastEdit)
	}

	// 8. Confirm -> closes keyboard, creates occurrence for Bob
	if err := pressReminderCallback(ctx, env, 8, 101, "fl:ok"); err != nil {
		t.Fatalf("confirm callback failed: %v", err)
	}
	lastEdit = env.client.LastEdited()
	if lastEdit == nil || lastEdit.MessageID != flowMsgID || len(lastEdit.Markup.InlineKeyboard) != 0 {
		t.Fatalf("expected closed keyboard after confirm, got: %+v", lastEdit)
	}

	// Assert exactly 1 message was sent through the whole creation flow
	if len(env.client.SentMessages()) != 1 {
		t.Fatalf("expected exactly 1 message sent, got %d", len(env.client.SentMessages()))
	}
	// Assert all edits targeted the same flowMsgID
	for _, em := range env.client.EditedMessages() {
		if em.MessageID != flowMsgID {
			t.Fatalf("expected edit to message %d, got %d", flowMsgID, em.MessageID)
		}
	}

	// Verify Bob has 1 open reminder, Alice has 0
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
	initialMsg := env.client.LastSent()
	flowMsgID := initialMsg.MessageID

	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID: 11, MessageThreadID: 63,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "Take vitamins",
		},
	})
	_ = pressReminderCallback(ctx, env, 3, 101, "fl:usr:102") // Bob
	_ = pressReminderCallback(ctx, env, 4, 101, "fl:rt:daily")
	_ = pressReminderCallback(ctx, env, 5, 101, "fl:iv:2")
	_ = pressReminderCallback(ctx, env, 6, 101, "fl:d:2026-10-10")
	_ = pressReminderCallback(ctx, env, 7, 101, "fl:h:08")
	_ = pressReminderCallback(ctx, env, 8, 101, "fl:mi:00")
	_ = pressReminderCallback(ctx, env, 9, 101, "fl:anc:scheduled")
	_ = pressReminderCallback(ctx, env, 10, 101, "fl:ok")

	// Verify one message throughout
	if len(env.client.SentMessages()) != 1 {
		t.Fatalf("expected exactly 1 message sent, got %d", len(env.client.SentMessages()))
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
