package telegram_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/recurrence"
	"github.com/px1/home-ctl/internal/telegram"
)

// switchRow returns the chores/reminders switch row of a private list, or nil when absent.
func switchRow(m *telegram.InlineKeyboardMarkup) []telegram.InlineKeyboardButton {
	if m == nil {
		return nil
	}
	for _, row := range m.InlineKeyboard {
		if len(row) == 2 && row[0].CallbackData == "list:ch" && row[1].CallbackData == "list:re" {
			return row
		}
	}
	return nil
}

func privateCmd(updateID, userID int64, text string) telegram.Update {
	return telegram.Update{UpdateID: updateID, Message: &telegram.Message{
		MessageID: 10, Chat: telegram.Chat{ID: userID, Type: "private"},
		From: &telegram.User{ID: userID}, Text: text,
	}}
}

// listOverviewTestMust fails the test on a setup error.
func listOverviewTestMust(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// Pablito-style scenario: Thu Oct 8 2026 21:00. Overdue and recurring entries render in the member's locale.
func TestPrivateList_EntryLayout(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}
	// Schedule while Oct 7 is still ahead (past due times are refused), then move to Thu Oct 8 21:00.
	env.clock.now = time.Date(2026, 10, 7, 12, 0, 0, 0, env.clock.loc)
	at := func(day, hour, minute int) time.Time {
		return time.Date(2026, 10, day, hour, minute, 0, 0, env.clock.loc)
	}

	listOverviewTestSeedCharlieChores(t, env, at)
	listOverviewTestSeedBobReminders(t, env, at)

	env.clock.now = time.Date(2026, 10, 8, 21, 0, 0, 0, env.clock.loc)

	listOverviewTestCharlieChoresView(t, env)
	listOverviewTestBobRemindersView(t, env)
}

// listOverviewTestSeedCharlieChores gives Charlie (es) an overdue one-off chore, an upcoming chore every 2 weeks and a quarterly one.
func listOverviewTestSeedCharlieChores(t *testing.T, env *testEnv, at func(day, hour, minute int) time.Time) {
	t.Helper()
	ctx := context.Background()
	bath, err := env.chores.CreateDefinition(ctx, 103, "Lavar el baño", "🚿", "", nil)
	listOverviewTestMust(t, err)
	_, err = env.chores.ScheduleOccurrence(ctx, 103, bath.ID, 103, at(7, 18, 0))
	listOverviewTestMust(t, err)
	bebedero, err := env.chores.CreateDefinition(ctx, 103, "Limpiar bebedero de los niños", "🧹", "",
		&recurrence.Rule{Interval: 2, Unit: recurrence.UnitWeek, Anchor: recurrence.AnchorScheduled})
	listOverviewTestMust(t, err)
	_, err = env.chores.ScheduleOccurrence(ctx, 103, bebedero.ID, 103, at(8, 23, 5))
	listOverviewTestMust(t, err)
	// Upcoming: a quarterly chore due Sat Oct 17.
	filter, err := env.chores.CreateDefinition(ctx, 103, "Cambiar filtro de agua", "💧", "",
		&recurrence.Rule{Interval: 3, Unit: recurrence.UnitMonth, Anchor: recurrence.AnchorScheduled})
	listOverviewTestMust(t, err)
	_, err = env.chores.ScheduleOccurrence(ctx, 103, filter.ID, 103, at(17, 10, 0))
	listOverviewTestMust(t, err)
}

// listOverviewTestSeedBobReminders gives Bob (en) an overdue reminder, one earlier today that is already past,
// then a one-time reminder without recurrence text.
func listOverviewTestSeedBobReminders(t *testing.T, env *testEnv, at func(day, hour, minute int) time.Time) {
	t.Helper()
	ctx := context.Background()
	callReminder, err := env.reminders.CreateDefinition(ctx, 101, 102, "Call mom", "⏰", "", nil)
	listOverviewTestMust(t, err)
	_, err = env.reminders.ScheduleOccurrence(ctx, 101, callReminder.ID, at(7, 18, 0))
	listOverviewTestMust(t, err)
	pills, err := env.reminders.CreateDefinition(ctx, 101, 102, "Take pills", "⏰", "", nil)
	listOverviewTestMust(t, err)
	_, err = env.reminders.ScheduleOccurrence(ctx, 101, pills.ID, at(8, 9, 0))
	listOverviewTestMust(t, err)
	medicine, err := env.reminders.CreateDefinition(ctx, 101, 102, "Buy medicine", "⏰", "", nil)
	listOverviewTestMust(t, err)
	_, err = env.reminders.ScheduleOccurrence(ctx, 101, medicine.ID, at(9, 10, 0))
	listOverviewTestMust(t, err)
}

// listOverviewTestCharlieChoresView checks Charlie's /list in private: groups, entry layout and button order.
func listOverviewTestCharlieChoresView(t *testing.T, env *testEnv) {
	t.Helper()
	ctx := context.Background()
	listOverviewTestMust(t, env.router.HandleUpdate(ctx, privateCmd(1, 103, "/list")))
	sent := env.client.LastSent()
	if sent == nil {
		t.Fatal("expected chores overview for Charlie")
	}
	// Groups appear in order, each under its header; the empty Tomorrow group is omitted.
	for _, want := range []string{
		"🔴 Vencidas\n🚿 Lavar el baño\n╰┈➤ ayer · 18:00",
		"🟡 Hoy\n🧹 Limpiar bebedero de los niños\n╰┈➤ 23:05 · 🔁 Cada 2 semanas",
		"⚪ Próximas\n💧 Cambiar filtro de agua\n╰┈➤ sáb 17 oct · 10:00 · 🔁 Cada 3 meses",
	} {
		if !strings.Contains(sent.Text, want) {
			t.Errorf("expected body to contain %q, got:\n%s", want, sent.Text)
		}
	}
	if strings.Contains(sent.Text, "🟢 Mañana") {
		t.Errorf("empty Tomorrow group must not render, got:\n%s", sent.Text)
	}
	if strings.Contains(sent.Text, "📅") {
		t.Errorf("entries must not carry the old date line, got:\n%s", sent.Text)
	}
	for i, want := range []string{"🚿 Lavar el baño", "🧹 Limpiar bebedero de los niños", "💧 Cambiar filtro de agua"} {
		if got := sent.Markup.InlineKeyboard[i][0].Text; got != want {
			t.Errorf("button %d: got %q, want %q (unnumbered, body order)", i, got, want)
		}
	}
}

// listOverviewTestBobRemindersView checks Bob's reminders overview in his private chat.
func listOverviewTestBobRemindersView(t *testing.T, env *testEnv) {
	t.Helper()
	ctx := context.Background()
	listOverviewTestMust(t, env.router.HandleUpdate(ctx, telegram.Update{UpdateID: 2, Message: &telegram.Message{
		MessageID: 20, MessageThreadID: 63, Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
		From: &telegram.User{ID: 102}, Text: "/list",
	}}))
	sent := env.client.LastSent()
	if sent == nil || sent.ChatID != 102 {
		t.Fatalf("expected reminders overview in Bob's private chat, got: %+v", sent)
	}
	for _, want := range []string{
		"🔴 Overdue\n⏰ Call mom\n╰┈➤ yesterday · 18:00",
		"⏰ Take pills\n╰┈➤ today · 09:00",
		"🟢 Tomorrow\n⏰ Buy medicine\n╰┈➤ 10:00",
	} {
		if !strings.Contains(sent.Text, want) {
			t.Errorf("expected body to contain %q, got:\n%s", want, sent.Text)
		}
	}
	if strings.Contains(sent.Text, "🔁") {
		t.Errorf("one-time reminders must not show recurrence text, got:\n%s", sent.Text)
	}
	if strings.Contains(sent.Text, "Upcoming") {
		t.Errorf("empty Upcoming group must not render, got:\n%s", sent.Text)
	}
	if got := sent.Markup.InlineKeyboard[1][0].Text; got != "⏰ Take pills" {
		t.Errorf("expected unnumbered button in body order, got %q", got)
	}
}

// The chores and reminders views switch in place, and the switch stays on empty views.
func TestPrivateList_ViewSwitch(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}

	listOverviewTestBobEmptyChoresView(t, env)
	listOverviewTestSwitchToReminders(t, env)
	listOverviewTestAliceEmptyReminders(t, env)
	listOverviewTestSwitchBackToChores(t, env)
}

// listOverviewTestBobEmptyChoresView: Bob has one reminder, no chores: his overview opens on an empty chores view that still switches.
func listOverviewTestBobEmptyChoresView(t *testing.T, env *testEnv) {
	t.Helper()
	ctx := context.Background()
	def, err := env.reminders.CreateDefinition(ctx, 101, 102, "Buy medicine", "⏰", "", nil)
	if err != nil {
		t.Fatalf("failed to create reminder: %v", err)
	}
	if _, err := env.reminders.ScheduleOccurrence(ctx, 101, def.ID, env.clock.now.Add(24*time.Hour)); err != nil {
		t.Fatalf("failed to schedule reminder: %v", err)
	}

	if err := env.router.HandleUpdate(ctx, privateCmd(1, 102, "/list")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sent := env.client.LastSent()
	if sent == nil || sent.Text != env.i18n.T("en", "chores.list_empty") {
		t.Fatalf("expected empty chores view, got: %+v", sent)
	}
	row := switchRow(sent.Markup)
	if row == nil {
		t.Fatalf("expected switch row on empty chores view, got: %+v", sent.Markup)
	}
	if want := env.i18n.T("en", "lists.view_chores"); row[0].Text != want {
		t.Errorf("expected marked chores button %q, got %q", want, row[0].Text)
	}
}

// listOverviewTestSwitchToReminders: pressing reminders on the chores view edits the same message into the reminders view.
func listOverviewTestSwitchToReminders(t *testing.T, env *testEnv) {
	t.Helper()
	ctx := context.Background()
	if err := env.router.HandleUpdate(ctx, callback(2, 102, 102, "list:re")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	edited := env.client.LastEdited()
	if edited == nil || edited.MessageID != 100 || edited.ChatID != 102 {
		t.Fatalf("expected the same message edited, got: %+v", edited)
	}
	if !strings.Contains(edited.Text, "Buy medicine") {
		t.Errorf("expected reminders view, got:\n%s", edited.Text)
	}
	row := switchRow(edited.Markup)
	if row == nil {
		t.Fatalf("expected switch row on reminders view, got: %+v", edited.Markup)
	}
	if want := env.i18n.T("en", "lists.view_reminders"); row[1].Text != want {
		t.Errorf("expected marked reminders button %q, got %q", want, row[1].Text)
	}
}

// listOverviewTestAliceEmptyReminders: Alice has nothing open: the empty reminders view still offers the switch back to chores.
func listOverviewTestAliceEmptyReminders(t *testing.T, env *testEnv) {
	t.Helper()
	ctx := context.Background()
	if err := env.router.HandleUpdate(ctx, callback(3, 101, 101, "list:re")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	edited := env.client.LastEdited()
	if edited == nil || edited.Text != env.i18n.T("en", "reminders.list_empty") {
		t.Fatalf("expected empty reminders view, got: %+v", edited)
	}
	if switchRow(edited.Markup) == nil {
		t.Fatalf("expected switch row on empty reminders view, got: %+v", edited.Markup)
	}
}

// listOverviewTestSwitchBackToChores: switching back to chores from the reminders view edits the same message again.
func listOverviewTestSwitchBackToChores(t *testing.T, env *testEnv) {
	t.Helper()
	ctx := context.Background()
	if err := env.router.HandleUpdate(ctx, callback(4, 102, 102, "list:ch")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	edited := env.client.LastEdited()
	if edited == nil || edited.MessageID != 100 || edited.Text != env.i18n.T("en", "chores.list_empty") {
		t.Fatalf("expected chores view back on the same message, got: %+v", edited)
	}
}
