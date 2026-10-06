package telegram_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/telegram"
)

func routerWithDefaultTime(t *testing.T, env *testEnv, tod string) *telegram.Router {
	t.Helper()
	return telegram.NewRouter(telegram.RouterConfig{
		ChatID: -1001234567890, ShoppingTopicID: 42, ChoresTopicID: 51, RemindersTopicID: 63,
		DefaultLocale: "en", FlowTTL: 30 * time.Minute, DefaultTimeOfDay: tod,
	}, env.client, env.household, env.shopping, env.chores, env.reminders, env.flow, env.i18n, env.clock, nil)
}

// "Tomorrow" in due-date presets and in postponement must use the configured default time of day,
// resolved in the household timezone, for chores and reminders alike.
func TestDefaultTimeOfDayDrivesDateOnlyChoices(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	ctx := context.Background()
	router := routerWithDefaultTime(t, env, "07:30")
	loc := env.clock.loc
	tomorrow := time.Date(2026, 10, 7, 7, 30, 0, 0, loc)

	// Chore flow: pick the existing chore, assignee, then the "Tomorrow" preset.
	_, _ = env.chores.CreateDefinition(ctx, 101, "Sweep", "🧹", "", nil)
	send := func(updateID int64, text string) {
		_ = router.HandleUpdate(ctx, telegram.Update{UpdateID: updateID, Message: &telegram.Message{
			MessageID: updateID, MessageThreadID: 51, Chat: telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From: &telegram.User{ID: 101, FirstName: "Alice"}, Text: text,
		}})
	}
	press := func(updateID int64, data string) {
		u := callback(updateID, 101, -1001234567890, data)
		u.CallbackQuery.Message.Chat.Type = "supergroup"
		u.CallbackQuery.Message.MessageThreadID = 51
		_ = router.HandleUpdate(ctx, u)
	}
	send(1, "/add")
	press(2, "fl:sel:1")
	press(3, "fl:usr:101")
	last := env.client.LastSent()
	if last == nil || last.Markup == nil {
		t.Fatalf("expected the due-date keyboard, got %+v", last)
	}
	labels := ""
	for _, row := range last.Markup.InlineKeyboard {
		for _, b := range row {
			labels += b.Text + "|"
		}
	}
	if !strings.Contains(labels, "Tomorrow 07:30") || !strings.Contains(labels, "Today 18:00") {
		t.Fatalf("expected labels from configured/evening times, got %q", labels)
	}
	press(4, "fl:due:tomorrow")
	press(5, "fl:rec:none")
	press(6, "fl:ok")
	occs, err := env.chores.ListMyOpen(ctx, 101)
	if err != nil || len(occs.Occurrences) != 1 || !occs.Occurrences[0].DueAt.Equal(tomorrow) {
		t.Fatalf("expected chore due %v, got %+v (err=%v)", tomorrow, occs.Occurrences, err)
	}

	// Postponement "Tomorrow" from the private list uses the same default.
	occ := occs.Occurrences[0]
	u := callback(7, 101, 101, fmt.Sprintf("ch:p:%d:tom", occ.ID))
	if err := router.HandleUpdate(ctx, u); err != nil {
		t.Fatalf("postpone failed: %v", err)
	}
	got, _ := env.chores.GetOccurrence(ctx, occ.ID)
	if !got.DueAt.Equal(tomorrow) {
		t.Fatalf("expected postponed due %v, got %v", tomorrow, got.DueAt)
	}
}

func TestInvalidDefaultTimeOfDayFallsBackToNineAM(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	ctx := context.Background()
	router := routerWithDefaultTime(t, env, "25:99")
	def, _ := env.chores.CreateDefinition(ctx, 101, "Mop", "🧽", "", nil)
	occ, _ := env.chores.ScheduleOccurrence(ctx, 101, def.ID, 101, env.clock.now.Add(time.Hour))
	if err := router.HandleUpdate(ctx, callback(1, 101, 101, fmt.Sprintf("ch:p:%d:tom", occ.ID))); err != nil {
		t.Fatalf("postpone failed: %v", err)
	}
	got, _ := env.chores.GetOccurrence(ctx, occ.ID)
	if want := time.Date(2026, 10, 7, 9, 0, 0, 0, env.clock.loc); !got.DueAt.Equal(want) {
		t.Fatalf("expected fallback 09:00 due %v, got %v", want, got.DueAt)
	}
}
