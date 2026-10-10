package telegram_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/telegram"
)

func TestPrivateList_NavigationAndActions(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}

	ctx := context.Background()

	// 1. Unconnected member Charlie (103) sends /list in Chores topic -> short notice, no list in group
	uCharlieList := telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID:       10,
			MessageThreadID: 51,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 103, FirstName: "Charlie"},
			Text:            "/list",
		},
	}
	if err := env.router.HandleUpdate(ctx, uCharlieList); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last := env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "/start") {
		t.Fatalf("expected unconnected notice, got: %+v", last)
	}
	if last.ChatID != -1001234567890 || last.ThreadID != 51 {
		t.Fatalf("expected notice sent to group chores topic, got: %+v", last)
	}

	// 2. Schedule two chores for Alice
	def1, err := env.chores.CreateDefinition(ctx, 101, "Clean bathroom", "🚿", "", nil)
	if err != nil {
		t.Fatalf("failed to create chore 1: %v", err)
	}
	occ1, err := env.chores.ScheduleOccurrence(ctx, 101, def1.ID, 101, env.clock.now.Add(1*time.Hour))
	if err != nil {
		t.Fatalf("failed to schedule occ 1: %v", err)
	}

	def2, err := env.chores.CreateDefinition(ctx, 101, "Take out trash", "🗑", "", nil)
	if err != nil {
		t.Fatalf("failed to create chore 2: %v", err)
	}
	occ2, err := env.chores.ScheduleOccurrence(ctx, 101, def2.ID, 101, env.clock.now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("failed to schedule occ 2: %v", err)
	}

	// 3. Connected member Alice runs /list in Chores topic -> delivered to Alice's private chat (ChatID: 101)
	uAliceList := telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID:       20,
			MessageThreadID: 51,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/list",
		},
	}
	if err := env.router.HandleUpdate(ctx, uAliceList); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last = env.client.LastSent()
	if last == nil || last.ChatID != 101 {
		t.Fatalf("expected list sent to Alice private chat, got: %+v", last)
	}
	if len(last.Markup.InlineKeyboard) != 3 { // 2 chore buttons + view switch row
		t.Fatalf("expected 2 chore buttons and a switch row, got: %d", len(last.Markup.InlineKeyboard))
	}

	// 4. Detail view navigation: Alice taps "🚿 Clean bathroom" (ch:d:<occ1.ID>)
	uDetail := telegram.Update{
		UpdateID: 3,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_detail",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID: 100,
				Chat:      telegram.Chat{ID: 101, Type: "private"},
			},
			Data: fmt.Sprintf("ch:d:%d", occ1.ID),
		},
	}
	if err := env.router.HandleUpdate(ctx, uDetail); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	edited := env.client.LastEdited()
	if edited == nil || !strings.Contains(edited.Text, "Clean bathroom") {
		t.Fatalf("expected detail view text, got: %+v", edited)
	}
	// The item's own emoji is the only one before its name: no default broom in front of it.
	if !strings.HasPrefix(edited.Text, "🚿 Clean bathroom\n") || strings.Contains(edited.Text, "🧹") {
		t.Fatalf("expected detail title with only the item emoji, got: %q", edited.Text)
	}
	for _, row := range edited.Markup.InlineKeyboard {
		for _, btn := range row {
			if btn.Text == "Done" || btn.Text == "Postpone" || btn.Text == "Back" {
				t.Fatalf("expected detail buttons with icons, got %q", btn.Text)
			}
		}
	}
	if len(edited.Markup.InlineKeyboard) != 2 { // Row 1: Done/Postpone, Row 2: Back
		t.Fatalf("expected detail buttons, got: %+v", edited.Markup)
	}

	// 5. Back navigation: Alice taps Back ("list:ch")
	uBack := telegram.Update{
		UpdateID: 4,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_back",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID: 100,
				Chat:      telegram.Chat{ID: 101, Type: "private"},
			},
			Data: "list:ch",
		},
	}
	if err := env.router.HandleUpdate(ctx, uBack); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	edited = env.client.LastEdited()
	if edited == nil || !strings.Contains(edited.Text, "Your Open Chores") {
		t.Fatalf("expected list view after Back, got: %+v", edited)
	}

	// 6. Postpone navigation: Alice opens detail for occ2, then taps Postpone ("ch:p:<occ2.ID>")
	uPostponeMenu := telegram.Update{
		UpdateID: 5,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_p_menu",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID: 100,
				Chat:      telegram.Chat{ID: 101, Type: "private"},
			},
			Data: fmt.Sprintf("ch:p:%d", occ2.ID),
		},
	}
	if err := env.router.HandleUpdate(ctx, uPostponeMenu); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	edited = env.client.LastEdited()
	if edited == nil || !strings.Contains(edited.Text, "duration") {
		t.Fatalf("expected postpone duration menu, got: %+v", edited)
	}

	// 7. Alice chooses "1h" preset ("ch:p:<occ2.ID>:1h")
	uPostpone1h := telegram.Update{
		UpdateID: 6,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_p_1h",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID: 100,
				Chat:      telegram.Chat{ID: 101, Type: "private"},
			},
			Data: fmt.Sprintf("ch:p:%d:1h", occ2.ID),
		},
	}
	if err := env.router.HandleUpdate(ctx, uPostpone1h); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify group topic update was posted in Chores topic
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "postponed it until") {
		t.Fatalf("expected group postpone update, got: %+v", last)
	}
	// Verify message edited back to list view
	edited = env.client.LastEdited()
	if edited == nil || !strings.Contains(edited.Text, "Your Open Chores") {
		t.Fatalf("expected list view after postpone, got: %+v", edited)
	}

	// 8. Done action: Alice completes occ1 ("ch:c:<occ1.ID>")
	uDone := telegram.Update{
		UpdateID: 7,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_done",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID: 100,
				Chat:      telegram.Chat{ID: 101, Type: "private"},
			},
			Data: fmt.Sprintf("ch:c:%d", occ1.ID),
		},
	}
	if err := env.router.HandleUpdate(ctx, uDone); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "completed by Alice") {
		t.Fatalf("expected group completion update, got: %+v", last)
	}

	// 9. Stale detail view: Alice taps Done again on occ1 -> re-renders current list without error
	uStaleDone := telegram.Update{
		UpdateID: 8,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_stale",
			From: telegram.User{ID: 101, FirstName: "Alice"},
			Message: &telegram.Message{
				MessageID: 100,
				Chat:      telegram.Chat{ID: 101, Type: "private"},
			},
			Data: fmt.Sprintf("ch:c:%d", occ1.ID),
		},
	}
	if err := env.router.HandleUpdate(ctx, uStaleDone); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrivateList_LongList(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}

	ctx := context.Background()

	// Create 25 open chores for Alice
	def, err := env.chores.CreateDefinition(ctx, 101, "Chore item", "🧹", "", nil)
	if err != nil {
		t.Fatalf("failed to create chore def: %v", err)
	}

	for i := 1; i <= 25; i++ {
		_, err := env.chores.ScheduleOccurrence(ctx, 101, def.ID, 101, env.clock.now.Add(time.Duration(i)*time.Hour))
		if err != nil {
			t.Fatalf("failed to schedule chore %d: %v", i, err)
		}
	}

	uList := telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 1,
			Chat:      telegram.Chat{ID: 101, Type: "private"},
			From:      &telegram.User{ID: 101, FirstName: "Alice"},
			Text:      "/list",
		},
	}
	if err := env.router.HandleUpdate(ctx, uList); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last := env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "+5 more") {
		t.Fatalf("expected '+5 more' for 25 items, got: %+v", last)
	}
	if len(last.Markup.InlineKeyboard) != 21 { // 20 capped buttons + view switch row
		t.Fatalf("expected 20 buttons plus switch row for capped list, got: %d", len(last.Markup.InlineKeyboard))
	}
}

func TestPrivateList_RemindersNavigationAndActions(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}

	ctx := context.Background()

	// 1. Schedule a reminder for Bob (102)
	def, err := env.reminders.CreateDefinition(ctx, 101, 102, "Call doctor", "⏰", "", nil)
	if err != nil {
		t.Fatalf("failed to create reminder def: %v", err)
	}
	occ, err := env.reminders.ScheduleOccurrence(ctx, 101, def.ID, env.clock.now.Add(1*time.Hour))
	if err != nil {
		t.Fatalf("failed to schedule reminder occ: %v", err)
	}

	// 2. Bob runs /list in Reminders topic (63) -> sent to Bob's private chat (102)
	uList := telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID:       10,
			MessageThreadID: 63,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 102, FirstName: "Bob"},
			Text:            "/list",
		},
	}
	if err := env.router.HandleUpdate(ctx, uList); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last := env.client.LastSent()
	if last == nil || last.ChatID != 102 {
		t.Fatalf("expected list sent to Bob's private chat, got: %+v", last)
	}

	// 3. Detail view: Bob taps Call doctor ("re:d:<occ.ID>")
	uDetail := telegram.Update{
		UpdateID: 2,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_detail",
			From: telegram.User{ID: 102, FirstName: "Bob"},
			Message: &telegram.Message{
				MessageID: 100,
				Chat:      telegram.Chat{ID: 102, Type: "private"},
			},
			Data: fmt.Sprintf("re:d:%d", occ.ID),
		},
	}
	if err := env.router.HandleUpdate(ctx, uDetail); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	edited := env.client.LastEdited()
	if edited == nil || !strings.Contains(edited.Text, "Call doctor") {
		t.Fatalf("expected detail view text, got: %+v", edited)
	}
	if !strings.HasPrefix(edited.Text, "⏰ Call doctor\n") || strings.Count(edited.Text, "⏰") != 1 {
		t.Fatalf("expected reminder detail title with its emoji once, got: %q", edited.Text)
	}

	// 4. Back navigation: Bob taps Back ("list:re")
	uBack := telegram.Update{
		UpdateID: 3,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_back",
			From: telegram.User{ID: 102, FirstName: "Bob"},
			Message: &telegram.Message{
				MessageID: 100,
				Chat:      telegram.Chat{ID: 102, Type: "private"},
			},
			Data: "list:re",
		},
	}
	if err := env.router.HandleUpdate(ctx, uBack); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	edited = env.client.LastEdited()
	if edited == nil || !strings.Contains(edited.Text, "Your Open Reminders") {
		t.Fatalf("expected reminders list view after Back, got: %+v", edited)
	}

	// 5. Postpone: Bob opens postpone menu ("re:p:<occ.ID>")
	uPostponeMenu := telegram.Update{
		UpdateID: 4,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_p_menu",
			From: telegram.User{ID: 102, FirstName: "Bob"},
			Message: &telegram.Message{
				MessageID: 100,
				Chat:      telegram.Chat{ID: 102, Type: "private"},
			},
			Data: fmt.Sprintf("re:p:%d", occ.ID),
		},
	}
	if err := env.router.HandleUpdate(ctx, uPostponeMenu); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 6. Bob selects "1h" preset ("re:p:<occ.ID>:1h")
	uPostpone1h := telegram.Update{
		UpdateID: 5,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_p_1h",
			From: telegram.User{ID: 102, FirstName: "Bob"},
			Message: &telegram.Message{
				MessageID: 100,
				Chat:      telegram.Chat{ID: 102, Type: "private"},
			},
			Data: fmt.Sprintf("re:p:%d:1h", occ.ID),
		},
	}
	if err := env.router.HandleUpdate(ctx, uPostpone1h); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "postponed it until") {
		t.Fatalf("expected group postpone update, got: %+v", last)
	}

	// 7. Non-target Alice (101) attempts to complete Bob's reminder -> rejected!
	uAliceComplete := telegram.Update{
		UpdateID: 6,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_alice_try",
			From: telegram.User{ID: 101, FirstName: "Alice"}, // Alice is actor
			Message: &telegram.Message{
				MessageID: 100,
				Chat:      telegram.Chat{ID: 102, Type: "private"},
			},
			Data: fmt.Sprintf("re:c:%d", occ.ID),
		},
	}
	if err := env.router.HandleUpdate(ctx, uAliceComplete); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ans := env.client.LastAnswer()
	if ans == nil || !strings.Contains(ans.Text, "Only the target member") {
		t.Fatalf("expected target-only rejection, got: %+v", ans)
	}

	// 8. Target Bob completes ("re:c:<occ.ID>")
	uBobComplete := telegram.Update{
		UpdateID: 7,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_bob_done",
			From: telegram.User{ID: 102, FirstName: "Bob"},
			Message: &telegram.Message{
				MessageID: 100,
				Chat:      telegram.Chat{ID: 102, Type: "private"},
			},
			Data: fmt.Sprintf("re:c:%d", occ.ID),
		},
	}
	if err := env.router.HandleUpdate(ctx, uBobComplete); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "completed by Bob") {
		t.Fatalf("expected group completion update, got: %+v", last)
	}

	// 9. Stale detail view for reminder -> re-renders without error
	uStale := telegram.Update{
		UpdateID: 8,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   "cb_stale_re",
			From: telegram.User{ID: 102, FirstName: "Bob"},
			Message: &telegram.Message{
				MessageID: 100,
				Chat:      telegram.Chat{ID: 102, Type: "private"},
			},
			Data: fmt.Sprintf("re:c:%d", occ.ID),
		},
	}
	if err := env.router.HandleUpdate(ctx, uStale); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
