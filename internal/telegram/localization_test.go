package telegram_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/telegram"
)

func callback(updateID int64, userID, chatID int64, data string) telegram.Update {
	return telegram.Update{
		UpdateID: updateID,
		CallbackQuery: &telegram.CallbackQuery{
			ID:      fmt.Sprintf("cb_%d", updateID),
			From:    telegram.User{ID: userID},
			Message: &telegram.Message{MessageID: 100, Chat: telegram.Chat{ID: chatID, Type: "private"}},
			Data:    data,
		},
	}
}

// A Spanish-locale member must get callback feedback from es.yaml, not hardcoded English.
func TestCallbackFeedbackUsesMemberLocale(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	ctx := context.Background()

	// Charlie (es) connects privately, then completes and postpones his own chores.
	start := telegram.Update{UpdateID: 1, Message: &telegram.Message{
		MessageID: 10, Chat: telegram.Chat{ID: 103, Type: "private"},
		From: &telegram.User{ID: 103, FirstName: "Charlie"}, Text: "/start",
	}}
	if err := env.router.HandleUpdate(ctx, start); err != nil {
		t.Fatalf("/start failed: %v", err)
	}
	def, _ := env.chores.CreateDefinition(ctx, 103, "Barrer", "🧹", "", nil)
	done, _ := env.chores.ScheduleOccurrence(ctx, 103, def.ID, 103, env.clock.now.Add(time.Hour))
	later, _ := env.chores.ScheduleOccurrence(ctx, 103, def.ID, 103, env.clock.now.Add(2*time.Hour))

	if err := env.router.HandleUpdate(ctx, callback(2, 103, 103, fmt.Sprintf("ch:c:%d", done.ID))); err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if got, want := env.client.LastAnswer().Text, env.i18n.T("es", "general.done"); got != want || got == "Done!" {
		t.Fatalf("expected Spanish done feedback %q, got %q", want, got)
	}

	if err := env.router.HandleUpdate(ctx, callback(3, 103, 103, fmt.Sprintf("ch:p:%d:1h", later.ID))); err != nil {
		t.Fatalf("postpone failed: %v", err)
	}
	if got, want := env.client.LastAnswer().Text, env.i18n.T("es", "chores.postponed_notice"); got != want {
		t.Fatalf("expected Spanish postpone feedback %q, got %q", want, got)
	}
}

// Domain errors must reach users as localized text, never as raw err.Error() values.
func TestCallbackErrorsAreLocalizedAndSafe(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	ctx := context.Background()

	for i, data := range []string{"ch:c:9999", "ch:p:9999:1h", "re:c:9999", "re:p:9999:1h"} {
		if err := env.router.HandleUpdate(ctx, callback(int64(10+i), 101, 101, data)); err != nil {
			t.Fatalf("%s: unexpected error: %v", data, err)
		}
		ans := env.client.LastAnswer()
		if want := env.i18n.T("en", "general.not_found"); ans.Text != want {
			t.Fatalf("%s: expected localized %q, got %q", data, want, ans.Text)
		}
	}
}

// A postpone button pressed after another path already completed the occurrence must be rejected
// with localized feedback, leave the occurrence untouched and post nothing to the group.
func TestStalePostponeCallbackAfterCompletion(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	ctx := context.Background()

	cdef, _ := env.chores.CreateDefinition(ctx, 101, "Clean bathroom", "🚿", "", nil)
	cdue := env.clock.now.Add(time.Hour)
	cocc, _ := env.chores.ScheduleOccurrence(ctx, 101, cdef.ID, 101, cdue)
	rdef, _ := env.reminders.CreateDefinition(ctx, 101, 101, "Buy medicine", "💊", "", nil)
	rdue := env.clock.now.Add(time.Hour)
	rocc, _ := env.reminders.ScheduleOccurrence(ctx, 101, rdef.ID, rdue)

	// Both resolved elsewhere (for example from the due notification).
	if _, _, err := env.chores.Complete(ctx, 101, cocc.ID, env.clock.now); err != nil {
		t.Fatalf("complete chore failed: %v", err)
	}
	if _, _, err := env.reminders.Complete(ctx, 101, rocc.ID, env.clock.now); err != nil {
		t.Fatalf("complete reminder failed: %v", err)
	}
	groupPostsBefore := len(env.client.sentMessages)

	for i, data := range []string{fmt.Sprintf("ch:p:%d:1h", cocc.ID), fmt.Sprintf("re:p:%d:1h", rocc.ID)} {
		if err := env.router.HandleUpdate(ctx, callback(int64(200+i), 101, 101, data)); err != nil {
			t.Fatalf("%s: unexpected error: %v", data, err)
		}
		ans := env.client.LastAnswer()
		if want := env.i18n.T("en", "general.already_resolved"); ans.Text != want || !ans.Alert {
			t.Fatalf("%s: expected alert %q, got %+v", data, want, ans)
		}
		if edited := env.client.LastEdited(); edited == nil || !strings.Contains(edited.Text, "no open") {
			t.Fatalf("%s: expected the list to be re-rendered, got %+v", data, edited)
		}
	}
	if len(env.client.sentMessages) != groupPostsBefore {
		t.Fatalf("stale postpone must not post group updates, got %d new messages", len(env.client.sentMessages)-groupPostsBefore)
	}
	if got, _ := env.chores.GetOccurrence(ctx, cocc.ID); !got.DueAt.Equal(cdue) {
		t.Fatalf("completed chore due_at changed to %v", got.DueAt)
	}
}

// Dates shown to users come from the locale files and the household timezone, never Go's English names.
func TestDateTimeRenderingIsLocalized(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	ctx := context.Background()

	start := telegram.Update{UpdateID: 1, Message: &telegram.Message{
		MessageID: 10, Chat: telegram.Chat{ID: 103, Type: "private"},
		From: &telegram.User{ID: 103, FirstName: "Charlie"}, Text: "/start",
	}}
	if err := env.router.HandleUpdate(ctx, start); err != nil {
		t.Fatalf("/start failed: %v", err)
	}
	def, _ := env.chores.CreateDefinition(ctx, 103, "Barrer", "🧹", "", nil)
	due := time.Date(2026, 10, 10, 10, 0, 0, 0, env.clock.loc) // a Saturday
	occ, _ := env.chores.ScheduleOccurrence(ctx, 103, def.ID, 103, due)

	if err := env.router.HandleUpdate(ctx, callback(2, 103, 103, fmt.Sprintf("ch:d:%d", occ.ID))); err != nil {
		t.Fatalf("detail failed: %v", err)
	}
	if text := env.client.LastEdited().Text; !strings.Contains(text, "sáb 10 oct, 10:00") || strings.Contains(text, "Sat") {
		t.Fatalf("expected Spanish date in detail view, got %q", text)
	}

	// Group postponement update is rendered in the default locale, with a relative day.
	if err := env.router.HandleUpdate(ctx, callback(3, 103, 103, fmt.Sprintf("ch:p:%d:tom", occ.ID))); err != nil {
		t.Fatalf("postpone failed: %v", err)
	}
	if text := env.client.LastSent().Text; !strings.Contains(text, "postponed it until tomorrow at 09:00") {
		t.Fatalf("expected localized group update, got %q", text)
	}
}
