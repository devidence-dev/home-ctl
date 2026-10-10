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

func pressChoreCallback(ctx context.Context, env *testEnv, updateID int64, userID int64, data string) error {
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
				MessageThreadID: 51,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
				Text:            text,
			},
			Data: data,
		},
	})
}

// 5.1 & 5.2: One-time chore flow skips interval and anchor, creates exactly 1 occurrence
// with scheduled_at == due_at, editing the same message ID throughout.
func TestChoresFlow_OneTime(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}
	ctx := context.Background()

	// 1. /add in Chores topic
	uAdd := telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID:       10,
			MessageThreadID: 51,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/add",
		},
	}
	if err := env.router.HandleUpdate(ctx, uAdd); err != nil {
		t.Fatalf("/add failed: %v", err)
	}
	initialMsg := env.client.LastSent()
	if initialMsg == nil || !strings.Contains(initialMsg.Text, "name for the new chore") {
		t.Fatalf("expected chore name prompt, got: %+v", initialMsg)
	}
	flowMsgID := initialMsg.MessageID

	// 2. Alice types chore name
	uName := telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID:       11,
			MessageThreadID: 51,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "Clean kitchen",
		},
	}
	if err := env.router.HandleUpdate(ctx, uName); err != nil {
		t.Fatalf("name text failed: %v", err)
	}
	// A typed answer re-anchors: the emoji step is a new message below the text and the name prompt is deleted.
	lastSent := env.client.LastSent()
	if lastSent == nil || lastSent.MessageID == flowMsgID || !strings.Contains(lastSent.Text, "Choose an emoji") {
		t.Fatalf("expected emoji prompt as a new message after the typed name, got: %+v", lastSent)
	}
	if deleted := env.client.DeletedMessages(); len(deleted) != 1 || deleted[0] != flowMsgID {
		t.Fatalf("expected name prompt %d deleted, got %v", flowMsgID, deleted)
	}
	flowMsgID = lastSent.MessageID // the active flow message from here on

	// 2b. Skip the emoji: the assignee step edits the same message
	if err := pressChoreCallback(ctx, env, 4, 101, "fl:em:skip"); err != nil {
		t.Fatalf("emoji skip failed: %v", err)
	}
	if lastEdit := env.client.LastEdited(); lastEdit == nil || lastEdit.MessageID != flowMsgID || !strings.Contains(lastEdit.Text, "Clean kitchen") {
		t.Fatalf("expected assignee prompt on message %d, got: %+v", flowMsgID, lastEdit)
	}

	// 3. Assign to Alice (101) -> moves to recurrence type step
	if err := pressChoreCallback(ctx, env, 3, 101, "fl:usr:101"); err != nil {
		t.Fatalf("assign callback failed: %v", err)
	}
	lastEdit := env.client.LastEdited()
	if lastEdit == nil || lastEdit.MessageID != flowMsgID || !strings.Contains(lastEdit.Text, "recurrence type") {
		t.Fatalf("expected recurrence type prompt on message %d, got: %+v", flowMsgID, lastEdit)
	}

	// 4. Alice picks One time (fl:rt:once) -> skips interval, moves to calendar date step
	if err := pressChoreCallback(ctx, env, 4, 101, "fl:rt:once"); err != nil {
		t.Fatalf("recurrence type callback failed: %v", err)
	}
	lastEdit = env.client.LastEdited()
	if lastEdit == nil || lastEdit.MessageID != flowMsgID || !strings.Contains(lastEdit.Text, "first due date") {
		t.Fatalf("expected calendar date prompt on message %d, got: %+v", flowMsgID, lastEdit)
	}

	// 5. Alice picks date Oct 14 2026 (fl:d:2026-10-14) -> moves to hour step
	if err := pressChoreCallback(ctx, env, 5, 101, "fl:d:2026-10-14"); err != nil {
		t.Fatalf("date callback failed: %v", err)
	}
	lastEdit = env.client.LastEdited()
	if lastEdit == nil || lastEdit.MessageID != flowMsgID || !strings.Contains(lastEdit.Text, "hour") {
		t.Fatalf("expected hour prompt on message %d, got: %+v", flowMsgID, lastEdit)
	}

	// 6. Alice picks hour 10 (fl:h:10) -> moves to minute step
	if err := pressChoreCallback(ctx, env, 6, 101, "fl:h:10"); err != nil {
		t.Fatalf("hour callback failed: %v", err)
	}
	lastEdit = env.client.LastEdited()
	if lastEdit == nil || lastEdit.MessageID != flowMsgID || !strings.Contains(lastEdit.Text, "minute") {
		t.Fatalf("expected minute prompt on message %d, got: %+v", flowMsgID, lastEdit)
	}

	// 7. Alice picks minute 30 (fl:mi:30) -> skips anchor, moves directly to confirm step
	if err := pressChoreCallback(ctx, env, 7, 101, "fl:mi:30"); err != nil {
		t.Fatalf("minute callback failed: %v", err)
	}
	lastEdit = env.client.LastEdited()
	if lastEdit == nil || lastEdit.MessageID != flowMsgID || !strings.Contains(lastEdit.Text, "Confirm") {
		t.Fatalf("expected confirm prompt on message %d, got: %+v", flowMsgID, lastEdit)
	}

	// 8. Confirm (fl:ok) -> closes keyboard, creates occurrence
	if err := pressChoreCallback(ctx, env, 8, 101, "fl:ok"); err != nil {
		t.Fatalf("confirm callback failed: %v", err)
	}
	lastEdit = env.client.LastEdited()
	if lastEdit == nil || lastEdit.MessageID != flowMsgID || len(lastEdit.Markup.InlineKeyboard) != 0 {
		t.Fatalf("expected closed keyboard on message %d after confirm, got: %+v", flowMsgID, lastEdit)
	}

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

	// Verify occurrence in database: scheduled_at == due_at == Oct 14 2026 10:30 local
	res, err := env.chores.ListMyOpen(ctx, 101)
	if err != nil || len(res.Occurrences) != 1 {
		t.Fatalf("expected 1 occurrence for Alice, got: %+v (err=%v)", res, err)
	}
	occ := res.Occurrences[0]
	wantDue := time.Date(2026, 10, 14, 10, 30, 0, 0, env.clock.loc).UTC()
	if !occ.DueAt.Equal(wantDue) {
		t.Fatalf("expected due_at %v, got %v", wantDue, occ.DueAt)
	}
	if !occ.ScheduledAt.Equal(occ.DueAt) {
		t.Fatalf("expected scheduled_at == due_at, got scheduled_at=%v due_at=%v", occ.ScheduledAt, occ.DueAt)
	}
	if occ.Definition.RecurrenceInterval != nil {
		t.Fatalf("expected nil recurrence for one-time chore, got %+v", occ.Definition.RecurrenceInterval)
	}
}

// 5.1: Weekly every 2 weeks from Oct 14 2026 (slots Oct 28 and Nov 11)
func TestChoresFlow_WeeklyEvery2Weeks(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}
	ctx := context.Background()

	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 10, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "/add",
		},
	})
	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID: 11, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "Mow lawn",
		},
	})
	_ = pressChoreCallback(ctx, env, 3, 101, "fl:usr:101")
	_ = pressChoreCallback(ctx, env, 4, 101, "fl:rt:weekly")
	_ = pressChoreCallback(ctx, env, 5, 101, "fl:iv:2")
	_ = pressChoreCallback(ctx, env, 6, 101, "fl:d:2026-10-14")
	_ = pressChoreCallback(ctx, env, 7, 101, "fl:h:10")
	_ = pressChoreCallback(ctx, env, 8, 101, "fl:mi:30")
	_ = pressChoreCallback(ctx, env, 9, 101, "fl:anc:scheduled")
	_ = pressChoreCallback(ctx, env, 10, 101, "fl:ok")

	res, err := env.chores.ListMyOpen(ctx, 101)
	if err != nil || len(res.Occurrences) != 1 {
		t.Fatalf("expected 1 occurrence, got: %+v", res)
	}
	occ := res.Occurrences[0]
	firstSlot := time.Date(2026, 10, 14, 10, 30, 0, 0, env.clock.loc).UTC()
	if !occ.DueAt.Equal(firstSlot) {
		t.Fatalf("expected first slot %v, got %v", firstSlot, occ.DueAt)
	}

	rule := recurrence.Rule{Interval: 2, Unit: recurrence.UnitWeek, Anchor: recurrence.AnchorScheduled}
	slot2, err := recurrence.NextSlot(rule, occ.ScheduledAt, occ.ScheduledAt, occ.ScheduledAt, env.clock.loc)
	if err != nil {
		t.Fatalf("NextSlot error: %v", err)
	}
	wantSlot2 := time.Date(2026, 10, 28, 10, 30, 0, 0, env.clock.loc).UTC()
	if !slot2.Equal(wantSlot2) {
		t.Fatalf("expected slot 2 %v, got %v", wantSlot2, slot2)
	}

	slot3, err := recurrence.NextSlot(rule, occ.ScheduledAt, slot2, slot2, env.clock.loc)
	if err != nil {
		t.Fatalf("NextSlot error: %v", err)
	}
	wantSlot3 := time.Date(2026, 11, 11, 10, 30, 0, 0, env.clock.loc).UTC()
	if !slot3.Equal(wantSlot3) {
		t.Fatalf("expected slot 3 %v, got %v", wantSlot3, slot3)
	}
}

// 5.1: Monthly from Jan 31 2027 (Feb 28, Mar 31)
func TestChoresFlow_MonthlyFromJan31(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}
	ctx := context.Background()

	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 10, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "/add",
		},
	})
	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID: 11, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "Monthly check",
		},
	})
	_ = pressChoreCallback(ctx, env, 3, 101, "fl:usr:101")
	_ = pressChoreCallback(ctx, env, 4, 101, "fl:rt:monthly")
	_ = pressChoreCallback(ctx, env, 5, 101, "fl:iv:1")
	_ = pressChoreCallback(ctx, env, 6, 101, "fl:d:2027-01-31")
	_ = pressChoreCallback(ctx, env, 7, 101, "fl:h:09")
	_ = pressChoreCallback(ctx, env, 8, 101, "fl:mi:00")
	_ = pressChoreCallback(ctx, env, 9, 101, "fl:anc:scheduled")
	_ = pressChoreCallback(ctx, env, 10, 101, "fl:ok")

	res, err := env.chores.ListMyOpen(ctx, 101)
	if err != nil || len(res.Occurrences) != 1 {
		t.Fatalf("expected 1 occurrence, got: %+v", res)
	}
	occ := res.Occurrences[0]
	slot1 := time.Date(2027, 1, 31, 9, 0, 0, 0, env.clock.loc).UTC()
	if !occ.DueAt.Equal(slot1) {
		t.Fatalf("expected slot 1 %v, got %v", slot1, occ.DueAt)
	}

	rule := recurrence.Rule{Interval: 1, Unit: recurrence.UnitMonth, Anchor: recurrence.AnchorScheduled}
	slot2, err := recurrence.NextSlot(rule, occ.ScheduledAt, occ.ScheduledAt, occ.ScheduledAt, env.clock.loc)
	if err != nil {
		t.Fatalf("NextSlot error: %v", err)
	}
	wantSlot2 := time.Date(2027, 2, 28, 9, 0, 0, 0, env.clock.loc).UTC()
	if !slot2.Equal(wantSlot2) {
		t.Fatalf("expected slot 2 %v, got %v", wantSlot2, slot2)
	}

	slot3, err := recurrence.NextSlot(rule, occ.ScheduledAt, slot2, slot2, env.clock.loc)
	if err != nil {
		t.Fatalf("NextSlot error: %v", err)
	}
	wantSlot3 := time.Date(2027, 3, 31, 9, 0, 0, 0, env.clock.loc).UTC()
	if !slot3.Equal(wantSlot3) {
		t.Fatalf("expected slot 3 %v, got %v", wantSlot3, slot3)
	}
}

// 5.1: "every 3 months from completion"
func TestChoresFlow_Every3MonthsFromCompletion(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}
	ctx := context.Background()

	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 10, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "/add",
		},
	})
	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID: 11, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "Change water purifier filter",
		},
	})
	_ = pressChoreCallback(ctx, env, 3, 101, "fl:usr:101")
	_ = pressChoreCallback(ctx, env, 4, 101, "fl:rt:monthly")
	_ = pressChoreCallback(ctx, env, 5, 101, "fl:iv:3")
	_ = pressChoreCallback(ctx, env, 6, 101, "fl:d:2026-10-10")
	_ = pressChoreCallback(ctx, env, 7, 101, "fl:h:10")
	_ = pressChoreCallback(ctx, env, 8, 101, "fl:mi:30")
	_ = pressChoreCallback(ctx, env, 9, 101, "fl:anc:completion")
	_ = pressChoreCallback(ctx, env, 10, 101, "fl:ok")

	res, err := env.chores.ListMyOpen(ctx, 101)
	if err != nil || len(res.Occurrences) != 1 {
		t.Fatalf("expected 1 occurrence, got: %+v", res)
	}
	occ := res.Occurrences[0]
	if occ.Definition.RecurrenceInterval == nil || *occ.Definition.RecurrenceInterval != 3 {
		t.Fatalf("expected interval 3, got %+v", occ.Definition.RecurrenceInterval)
	}
	if occ.Definition.RecurrenceUnit == nil || *occ.Definition.RecurrenceUnit != "month" {
		t.Fatalf("expected unit month, got %+v", occ.Definition.RecurrenceUnit)
	}
	if occ.Definition.RecurrenceAnchor == nil || *occ.Definition.RecurrenceAnchor != "completion" {
		t.Fatalf("expected anchor completion, got %+v", occ.Definition.RecurrenceAnchor)
	}
}

// 5.1: Recurring definition skips recurrence type, interval, and anchor
func TestChoresFlow_ExistingRecurringDefinition(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}
	ctx := context.Background()

	// Pre-existing recurring definition
	rule := &recurrence.Rule{Interval: 2, Unit: recurrence.UnitWeek, Anchor: recurrence.AnchorScheduled}
	def, err := env.chores.CreateDefinition(ctx, 101, "Mow lawn", "🌱", "", rule)
	if err != nil {
		t.Fatalf("CreateDefinition failed: %v", err)
	}

	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 10, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "/add",
		},
	})
	_ = pressChoreCallback(ctx, env, 2, 101, fmt.Sprintf("fl:sel:%d", def.ID))
	_ = pressChoreCallback(ctx, env, 3, 101, "fl:usr:102") // Bob

	// Should skip recurrence steps and show calendar date prompt directly!
	lastEdit := env.client.LastEdited()
	if lastEdit == nil || !strings.Contains(lastEdit.Text, "first due date") {
		t.Fatalf("expected date prompt, got: %+v", lastEdit)
	}

	_ = pressChoreCallback(ctx, env, 4, 101, "fl:d:tomorrow")
	_ = pressChoreCallback(ctx, env, 5, 101, "fl:h:18")
	_ = pressChoreCallback(ctx, env, 6, 101, "fl:mi:00")

	// Should skip anchor step and show confirm prompt directly!
	lastEdit = env.client.LastEdited()
	if lastEdit == nil || !strings.Contains(lastEdit.Text, "Confirm") {
		t.Fatalf("expected confirm prompt, got: %+v", lastEdit)
	}

	_ = pressChoreCallback(ctx, env, 7, 101, "fl:ok")

	resBob, err := env.chores.ListMyOpen(ctx, 102)
	if err != nil || len(resBob.Occurrences) != 1 {
		t.Fatalf("expected 1 occurrence for Bob, got: %+v", resBob)
	}
	occ := resBob.Occurrences[0]
	if !occ.ScheduledAt.Equal(occ.DueAt) {
		t.Fatalf("expected scheduled_at == due_at, got scheduled_at=%v due_at=%v", occ.ScheduledAt, occ.DueAt)
	}
}

// 5.2: Past time on today keeps flow at hour step with date kept
func TestChoresFlow_PastTimeOnToday(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}
	ctx := context.Background()

	// Household time is 14:00 on Oct 5 2026
	loc := env.clock.loc
	env.clock.now = time.Date(2026, 10, 5, 14, 0, 0, 0, loc)

	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 10, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "/add",
		},
	})
	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID: 11, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "Sweep patio",
		},
	})
	_ = pressChoreCallback(ctx, env, 3, 101, "fl:usr:101")
	_ = pressChoreCallback(ctx, env, 4, 101, "fl:rt:once")
	_ = pressChoreCallback(ctx, env, 5, 101, "fl:d:today")
	_ = pressChoreCallback(ctx, env, 6, 101, "fl:h:09")

	// Minute 00 -> 09:00 earlier today is in the past!
	_ = pressChoreCallback(ctx, env, 7, 101, "fl:mi:00")

	// The past time is explained in the flow message, not in an alert
	if lastAns := env.client.LastAnswer(); lastAns == nil || lastAns.Alert {
		t.Fatalf("expected a plain callback answer, got: %+v", lastAns)
	}

	// Message stays at hour step with date kept and the explanation shown
	lastEdit := env.client.LastEdited()
	if lastEdit == nil || !strings.Contains(lastEdit.Text, env.i18n.T("en", "picker.time_passed")) || !strings.Contains(lastEdit.Text, "Choose hour") {
		t.Fatalf("expected time-passed notice on the hour step, got: %+v", lastEdit)
	}

	// No occurrence was created
	res, _ := env.chores.ListMyOpen(ctx, 101)
	if len(res.Occurrences) != 0 {
		t.Fatalf("expected 0 occurrences after past time rejection, got: %d", len(res.Occurrences))
	}

	// Alice picks valid hour 15 and minute 00 -> proceeds
	_ = pressChoreCallback(ctx, env, 8, 101, "fl:h:15")
	_ = pressChoreCallback(ctx, env, 9, 101, "fl:mi:00")
	lastEdit = env.client.LastEdited()
	if lastEdit == nil || !strings.Contains(lastEdit.Text, "Confirm") {
		t.Fatalf("expected confirm prompt, got: %+v", lastEdit)
	}
	_ = pressChoreCallback(ctx, env, 10, 101, "fl:ok")

	res, err = env.chores.ListMyOpen(ctx, 101)
	if err != nil || len(res.Occurrences) != 1 {
		t.Fatalf("expected 1 occurrence after picking future time, got: %+v", res)
	}
	wantDue := time.Date(2026, 10, 5, 15, 0, 0, 0, loc).UTC()
	if !res.Occurrences[0].DueAt.Equal(wantDue) {
		t.Fatalf("expected due %v, got %v", wantDue, res.Occurrences[0].DueAt)
	}
}

// 5.2: Abandoning flow creates nothing
func TestChoresFlow_AbandonCreatesNothing(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}
	ctx := context.Background()

	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 10, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "/add",
		},
	})
	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID: 11, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "Abandoned chore",
		},
	})

	// Advance clock past TTL (35 min)
	env.clock.now = env.clock.now.Add(35 * time.Minute)

	// Press button after expiry -> alert and keyboard removed
	_ = pressChoreCallback(ctx, env, 3, 101, "fl:usr:101")
	lastAns := env.client.LastAnswer()
	if lastAns == nil || !strings.Contains(lastAns.Text, "expired") {
		t.Fatalf("expected expired alert, got: %+v", lastAns)
	}

	// Verify nothing was created in definitions or occurrences
	defs, _ := env.chores.ListDefinitions(ctx)
	if len(defs) != 0 {
		t.Fatalf("expected 0 definitions, got %d", len(defs))
	}
	occs, _ := env.chores.ListMyOpen(ctx, 101)
	if len(occs.Occurrences) != 0 {
		t.Fatalf("expected 0 occurrences, got %d", len(occs.Occurrences))
	}
}

// Cancel closes flow keyboard and sends cancelled notice
func TestChoresFlow_Cancel(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}
	ctx := context.Background()

	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 10, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "/add",
		},
	})
	_ = pressChoreCallback(ctx, env, 2, 101, "fl:c")

	lastSent := env.client.LastSent()
	if lastSent == nil || !strings.Contains(lastSent.Text, "cancelled") {
		t.Fatalf("expected cancelled message, got: %+v", lastSent)
	}
	lastEdit := env.client.LastEdited()
	if lastEdit == nil || len(lastEdit.Markup.InlineKeyboard) != 0 {
		t.Fatalf("expected closed keyboard on cancel, got: %+v", lastEdit)
	}
}

// Calendar navigation changes month on the same message without advancing the step
func TestChoresFlow_CalendarNavigation(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}
	ctx := context.Background()

	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 10, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "/add",
		},
	})
	_ = env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID: 11, MessageThreadID: 51,
			Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: "Nav chore",
		},
	})
	_ = pressChoreCallback(ctx, env, 3, 101, "fl:usr:101")
	_ = pressChoreCallback(ctx, env, 4, 101, "fl:rt:once")

	// Current month is October 2026. Navigate to November 2026.
	_ = pressChoreCallback(ctx, env, 5, 101, "fl:cal:2026-11")
	lastEdit := env.client.LastEdited()
	if lastEdit == nil || !strings.Contains(lastEdit.Markup.InlineKeyboard[0][1].Text, "November 2026") {
		t.Fatalf("expected November 2026 header, got: %+v", lastEdit)
	}

	// Press fl:noop (e.g. past day or header) -> inert, no edit
	editsBefore := len(env.client.EditedMessages())
	_ = pressChoreCallback(ctx, env, 6, 101, "fl:noop")
	if len(env.client.EditedMessages()) != editsBefore {
		t.Fatalf("expected no edit on fl:noop")
	}
}
