package telegram_test

import (
	"context"
	"fmt"
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

	// Create definition and occurrence
	def, err := env.chores.CreateDefinition(ctx, 101, "Sweep", "🧹", "", nil)
	if err != nil {
		t.Fatalf("failed to create def: %v", err)
	}
	occ, err := env.chores.ScheduleOccurrence(ctx, 101, def.ID, 101, env.clock.now.Add(time.Hour))
	if err != nil {
		t.Fatalf("failed to schedule occ: %v", err)
	}

	// Postponement "Tomorrow" from the private list uses the same default.
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
