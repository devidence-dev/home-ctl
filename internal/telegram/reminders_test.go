package telegram_test

import (
	"context"
	"strings"
	"testing"

	"github.com/px1/home-ctl/internal/telegram"
)

func TestReminders_AddFlow_ConfirmAndCancel(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}

	ctx := context.Background()

	// 1. Alice sends /add in Reminders topic (63)
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
		t.Fatalf("unexpected error: %v", err)
	}
	last := env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "title for the reminder") {
		t.Fatalf("expected title prompt, got: %+v", last)
	}

	// 2. Alice types title "Buy medicine"
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
		t.Fatalf("unexpected error: %v", err)
	}
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "reminder is for") {
		t.Fatalf("expected target prompt, got: %+v", last)
	}

	// 3. Alice selects Bob (102) as target
	uTarget := telegram.Update{
		UpdateID: 3,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_target",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       12,
				MessageThreadID: 63,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "fl:usr:102",
		},
	}
	if err := env.router.HandleUpdate(ctx, uTarget); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "due date/time") {
		t.Fatalf("expected due date prompt, got: %+v", last)
	}

	// 4. Alice chooses "Tomorrow 18:00" preset
	uDue := telegram.Update{
		UpdateID: 4,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_due",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       13,
				MessageThreadID: 63,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "fl:due:tomorrow_18",
		},
	}
	if err := env.router.HandleUpdate(ctx, uDue); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "recurrence") {
		t.Fatalf("expected recurrence prompt, got: %+v", last)
	}

	// 5. Alice selects "None" (one-off)
	uRec := telegram.Update{
		UpdateID: 5,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_rec",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       14,
				MessageThreadID: 63,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "fl:rec:none",
		},
	}
	if err := env.router.HandleUpdate(ctx, uRec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "Confirm") {
		t.Fatalf("expected confirm prompt, got: %+v", last)
	}

	// 6. Alice confirms
	uConfirm := telegram.Update{
		UpdateID: 6,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_confirm",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       15,
				MessageThreadID: 63,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "fl:ok",
		},
	}
	if err := env.router.HandleUpdate(ctx, uConfirm); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "scheduled") {
		t.Fatalf("expected reminder scheduled message, got: %+v", last)
	}

	// Verify Bob has 1 open reminder, Alice has 0!
	bobReminders, err := env.reminders.ListMyOpen(ctx, 102)
	if err != nil || len(bobReminders.Occurrences) != 1 {
		t.Fatalf("expected 1 reminder for Bob, got: %+v", bobReminders)
	}
	aliceReminders, err := env.reminders.ListMyOpen(ctx, 101)
	if err != nil || len(aliceReminders.Occurrences) != 0 {
		t.Fatalf("expected 0 reminders for Alice, got: %+v", aliceReminders)
	}
}

func TestReminders_AddFlow_CancelLeavesNoData(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}

	ctx := context.Background()

	// Start /add in Reminders
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
	_ = env.router.HandleUpdate(ctx, uAdd)

	// Cancel button clicked
	uCancel := telegram.Update{
		UpdateID: 2,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_cancel",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       11,
				MessageThreadID: 63,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "fl:c",
		},
	}
	if err := env.router.HandleUpdate(ctx, uCancel); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last := env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "cancelled") {
		t.Fatalf("expected cancelled message, got: %+v", last)
	}

	// No definitions exist
	defs, err := env.reminders.ListDefinitions(ctx)
	if err != nil || len(defs) != 0 {
		t.Fatalf("expected 0 definitions, got: %+v", defs)
	}
}

func TestReminders_AddFlow_TypedDueAndRecurring(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}

	ctx := context.Background()

	// 1. /add in Reminders
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
	_ = env.router.HandleUpdate(ctx, uAdd)

	// 2. Title
	uTitle := telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID:       11,
			MessageThreadID: 63,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "Pay rent",
		},
	}
	_ = env.router.HandleUpdate(ctx, uTitle)

	// 3. Target Bob
	uTarget := telegram.Update{
		UpdateID: 3,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_target_bob",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       12,
				MessageThreadID: 63,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "fl:usr:102",
		},
	}
	_ = env.router.HandleUpdate(ctx, uTarget)

	// 4. Custom due
	uCustom := telegram.Update{
		UpdateID: 4,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_custom_due",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       13,
				MessageThreadID: 63,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "fl:due:custom",
		},
	}
	_ = env.router.HandleUpdate(ctx, uCustom)

	// 5. Invalid date
	uInvalid := telegram.Update{
		UpdateID: 5,
		Message: &telegram.Message{
			MessageID:       14,
			MessageThreadID: 63,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "invalid-date",
		},
	}
	_ = env.router.HandleUpdate(ctx, uInvalid)

	// 6. Valid date "2026-10-15 12:00"
	uValid := telegram.Update{
		UpdateID: 6,
		Message: &telegram.Message{
			MessageID:       15,
			MessageThreadID: 63,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "2026-10-15 12:00",
		},
	}
	_ = env.router.HandleUpdate(ctx, uValid)

	// 7. Recurrence monthly
	uMonthly := telegram.Update{
		UpdateID: 7,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_rec_monthly",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       16,
				MessageThreadID: 63,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "fl:rec:monthly",
		},
	}
	_ = env.router.HandleUpdate(ctx, uMonthly)

	// 8. Anchor scheduled
	uSched := telegram.Update{
		UpdateID: 8,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_anc_sched",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       17,
				MessageThreadID: 63,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "fl:anc:scheduled",
		},
	}
	_ = env.router.HandleUpdate(ctx, uSched)

	// 9. Confirm
	uConfirm := telegram.Update{
		UpdateID: 9,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_confirm_rent",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID:       18,
				MessageThreadID: 63,
				Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			},
			Data: "fl:ok",
		},
	}
	if err := env.router.HandleUpdate(ctx, uConfirm); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify occurrence exists for Bob
	res, err := env.reminders.ListMyOpen(ctx, 102)
	if err != nil || len(res.Occurrences) != 1 {
		t.Fatalf("expected 1 reminder for Bob, got: %+v", res)
	}
}
