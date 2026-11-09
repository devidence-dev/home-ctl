package telegram_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/telegram"
)

var (
	listTestAlice = telegram.User{ID: 101, FirstName: "Alice"}
	listTestBob   = telegram.User{ID: 102, FirstName: "Bob"}
)

// listTestCallback builds a button press on message 100 of the private chat chatID.
func listTestCallback(updateID int64, cbID string, from telegram.User, chatID int64, data string) telegram.Update {
	return telegram.Update{
		UpdateID: updateID,
		CallbackQuery: &telegram.CallbackQuery{
			ID:   cbID,
			From: from,
			Message: &telegram.Message{
				MessageID: 100,
				Chat:      telegram.Chat{ID: chatID, Type: "private"},
			},
			Data: data,
		},
	}
}

// listTestHandle routes one update and fails the test on error.
func listTestHandle(ctx context.Context, t *testing.T, env *testEnv, u telegram.Update) {
	t.Helper()
	if err := env.router.HandleUpdate(ctx, u); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// listTestUnconnectedNotice: unconnected Charlie gets a /start notice in the chores topic.
func listTestUnconnectedNotice(ctx context.Context, t *testing.T, env *testEnv) {
	t.Helper()
	listTestHandle(ctx, t, env, telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID:       10,
			MessageThreadID: 51,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 103, FirstName: "Charlie"},
			Text:            "/list",
		},
	})
	last := env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "/start") {
		t.Fatalf("expected unconnected notice, got: %+v", last)
	}
	if last.ChatID != -1001234567890 || last.ThreadID != 51 {
		t.Fatalf("expected notice sent to group chores topic, got: %+v", last)
	}
}

// listTestScheduleChores schedules two chores for Alice and returns their occurrence IDs.
func listTestScheduleChores(ctx context.Context, t *testing.T, env *testEnv) (int64, int64) {
	t.Helper()
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
	return occ1.ID, occ2.ID
}

// listTestAliceOpensList: Alice's /list in the chores topic lands in her private chat.
func listTestAliceOpensList(ctx context.Context, t *testing.T, env *testEnv) {
	t.Helper()
	listTestHandle(ctx, t, env, telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID:       20,
			MessageThreadID: 51,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/list",
		},
	})
	last := env.client.LastSent()
	if last == nil || last.ChatID != 101 {
		t.Fatalf("expected list sent to Alice private chat, got: %+v", last)
	}
	if len(last.Markup.InlineKeyboard) != 3 { // 2 chore buttons + view switch row
		t.Fatalf("expected 2 chore buttons and a switch row, got: %d", len(last.Markup.InlineKeyboard))
	}
}

// listTestDetailView: Alice taps a chore and sees its detail view with icon buttons.
func listTestDetailView(ctx context.Context, t *testing.T, env *testEnv, occ1ID int64) {
	t.Helper()
	listTestHandle(ctx, t, env, listTestCallback(3, "cb_detail", listTestAlice, 101, fmt.Sprintf("ch:d:%d", occ1ID)))
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
}

// listTestBackToList: Alice taps Back and the list view is shown again.
func listTestBackToList(ctx context.Context, t *testing.T, env *testEnv) {
	t.Helper()
	listTestHandle(ctx, t, env, listTestCallback(4, "cb_back", listTestAlice, 101, "list:ch"))
	edited := env.client.LastEdited()
	if edited == nil || !strings.Contains(edited.Text, "Your Open Chores") {
		t.Fatalf("expected list view after Back, got: %+v", edited)
	}
}

// listTestPostponePreset: Alice opens the postpone menu for occ2 and picks the 1h preset.
func listTestPostponePreset(ctx context.Context, t *testing.T, env *testEnv, occ2ID int64) {
	t.Helper()
	listTestHandle(ctx, t, env, listTestCallback(5, "cb_p_menu", listTestAlice, 101, fmt.Sprintf("ch:p:%d", occ2ID)))
	edited := env.client.LastEdited()
	if edited == nil || !strings.Contains(edited.Text, "duration") {
		t.Fatalf("expected postpone duration menu, got: %+v", edited)
	}

	listTestHandle(ctx, t, env, listTestCallback(6, "cb_p_1h", listTestAlice, 101, fmt.Sprintf("ch:p:%d:1h", occ2ID)))
	// Verify group topic update was posted in Chores topic
	last := env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "postponed it until") {
		t.Fatalf("expected group postpone update, got: %+v", last)
	}
	// Verify message edited back to list view
	edited = env.client.LastEdited()
	if edited == nil || !strings.Contains(edited.Text, "Your Open Chores") {
		t.Fatalf("expected list view after postpone, got: %+v", edited)
	}
}

// listTestDoneThenStale: Alice completes occ1, then taps its stale Done button again without error.
func listTestDoneThenStale(ctx context.Context, t *testing.T, env *testEnv, occ1ID int64) {
	t.Helper()
	listTestHandle(ctx, t, env, listTestCallback(7, "cb_done", listTestAlice, 101, fmt.Sprintf("ch:c:%d", occ1ID)))
	last := env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "completed by Alice") {
		t.Fatalf("expected group completion update, got: %+v", last)
	}

	listTestHandle(ctx, t, env, listTestCallback(8, "cb_stale", listTestAlice, 101, fmt.Sprintf("ch:c:%d", occ1ID)))
}

func TestPrivateList_NavigationAndActions(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}

	ctx := context.Background()

	listTestUnconnectedNotice(ctx, t, env)
	occ1ID, occ2ID := listTestScheduleChores(ctx, t, env)
	listTestAliceOpensList(ctx, t, env)
	listTestDetailView(ctx, t, env, occ1ID)
	listTestBackToList(ctx, t, env)
	listTestPostponePreset(ctx, t, env, occ2ID)
	listTestDoneThenStale(ctx, t, env, occ1ID)
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

// listTestReminderOpen: a reminder for Bob, opened by Bob's /list in the reminders topic.
// Returns the occurrence ID.
func listTestReminderOpen(ctx context.Context, t *testing.T, env *testEnv) int64 {
	t.Helper()
	def, err := env.reminders.CreateDefinition(ctx, 101, 102, "Call doctor", "⏰", "", nil)
	if err != nil {
		t.Fatalf("failed to create reminder def: %v", err)
	}
	occ, err := env.reminders.ScheduleOccurrence(ctx, 101, def.ID, env.clock.now.Add(1*time.Hour))
	if err != nil {
		t.Fatalf("failed to schedule reminder occ: %v", err)
	}

	// Bob runs /list in Reminders topic (63) -> sent to Bob's private chat (102)
	listTestHandle(ctx, t, env, telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID:       10,
			MessageThreadID: 63,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 102, FirstName: "Bob"},
			Text:            "/list",
		},
	})
	last := env.client.LastSent()
	if last == nil || last.ChatID != 102 {
		t.Fatalf("expected list sent to Bob's private chat, got: %+v", last)
	}
	return occ.ID
}

// listTestReminderDetailAndBack: Bob opens the reminder detail, then taps Back to the reminders list.
func listTestReminderDetailAndBack(ctx context.Context, t *testing.T, env *testEnv, occID int64) {
	t.Helper()
	listTestHandle(ctx, t, env, listTestCallback(2, "cb_detail", listTestBob, 102, fmt.Sprintf("re:d:%d", occID)))
	edited := env.client.LastEdited()
	if edited == nil || !strings.Contains(edited.Text, "Call doctor") {
		t.Fatalf("expected detail view text, got: %+v", edited)
	}
	if !strings.HasPrefix(edited.Text, "⏰ Call doctor\n") || strings.Count(edited.Text, "⏰") != 1 {
		t.Fatalf("expected reminder detail title with its emoji once, got: %q", edited.Text)
	}

	listTestHandle(ctx, t, env, listTestCallback(3, "cb_back", listTestBob, 102, "list:re"))
	edited = env.client.LastEdited()
	if edited == nil || !strings.Contains(edited.Text, "Your Open Reminders") {
		t.Fatalf("expected reminders list view after Back, got: %+v", edited)
	}
}

// listTestReminderPostpone: Bob opens the postpone menu and selects the 1h preset.
func listTestReminderPostpone(ctx context.Context, t *testing.T, env *testEnv, occID int64) {
	t.Helper()
	listTestHandle(ctx, t, env, listTestCallback(4, "cb_p_menu", listTestBob, 102, fmt.Sprintf("re:p:%d", occID)))
	listTestHandle(ctx, t, env, listTestCallback(5, "cb_p_1h", listTestBob, 102, fmt.Sprintf("re:p:%d:1h", occID)))
	last := env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "postponed it until") {
		t.Fatalf("expected group postpone update, got: %+v", last)
	}
}

// listTestReminderCompleteRules: Alice cannot complete Bob's reminder, Bob can, and a stale press is harmless.
func listTestReminderCompleteRules(ctx context.Context, t *testing.T, env *testEnv, occID int64) {
	t.Helper()
	// Non-target Alice (101) attempts to complete Bob's reminder -> rejected!
	listTestHandle(ctx, t, env, listTestCallback(6, "cb_alice_try", listTestAlice, 102, fmt.Sprintf("re:c:%d", occID)))
	ans := env.client.LastAnswer()
	if ans == nil || !strings.Contains(ans.Text, "Only the target member") {
		t.Fatalf("expected target-only rejection, got: %+v", ans)
	}

	// Target Bob completes ("re:c:<occ.ID>")
	listTestHandle(ctx, t, env, listTestCallback(7, "cb_bob_done", listTestBob, 102, fmt.Sprintf("re:c:%d", occID)))
	last := env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "completed by Bob") {
		t.Fatalf("expected group completion update, got: %+v", last)
	}

	// Stale detail view for reminder -> re-renders without error
	listTestHandle(ctx, t, env, listTestCallback(8, "cb_stale_re", listTestBob, 102, fmt.Sprintf("re:c:%d", occID)))
}

func TestPrivateList_RemindersNavigationAndActions(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}

	ctx := context.Background()

	occID := listTestReminderOpen(ctx, t, env)
	listTestReminderDetailAndBack(ctx, t, env, occID)
	listTestReminderPostpone(ctx, t, env, occID)
	listTestReminderCompleteRules(ctx, t, env, occID)
}
