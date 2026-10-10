package telegram_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/telegram"
)

// polishChoreText sends a typed message from userID in the chores topic (thread 51).
func polishChoreText(ctx context.Context, env *testEnv, updateID, userID int64, text string) error {
	return env.router.HandleUpdate(ctx, telegram.Update{
		UpdateID: updateID,
		Message: &telegram.Message{
			MessageID: updateID + 1000, MessageThreadID: 51,
			Chat: telegram.Chat{ID: staleTestChatID, Type: "supergroup"},
			From: &telegram.User{ID: userID, FirstName: "Alice"}, Text: text,
		},
	})
}

// startChoreFlow runs /add and types the chore name, leaving the assignee step active.
// It returns the id of the name prompt that the typed answer superseded.
func startChoreFlow(ctx context.Context, t *testing.T, env *testEnv, name string) int64 {
	t.Helper()
	if err := polishChoreText(ctx, env, 1, 101, "/add"); err != nil {
		t.Fatalf("/add failed: %v", err)
	}
	namePrompt := env.client.LastSent().MessageID
	if err := polishChoreText(ctx, env, 2, 101, name); err != nil {
		t.Fatalf("name failed: %v", err)
	}
	return namePrompt
}

func TestRecurrenceText_SingularAndPlural(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	cases := []struct {
		locale, unit string
		interval     int
		want         string
	}{
		{"en", "day", 1, "Every day"},
		{"en", "day", 3, "Every 3 days"},
		{"en", "week", 1, "Every week"},
		{"en", "week", 3, "Every 3 weeks"},
		{"en", "month", 1, "Every month"},
		{"en", "month", 3, "Every 3 months"},
		{"es", "day", 1, "Cada día"},
		{"es", "day", 3, "Cada 3 días"},
		{"es", "week", 1, "Cada semana"},
		{"es", "week", 2, "Cada 2 semanas"},
		{"es", "month", 1, "Cada mes"},
		{"es", "month", 3, "Cada 3 meses"},
	}
	for _, tc := range cases {
		if got := telegram.RecurrenceText(env.i18n, tc.locale, tc.interval, tc.unit); got != tc.want {
			t.Errorf("%s %s x%d: got %q, want %q", tc.locale, tc.unit, tc.interval, got, tc.want)
		}
	}
}

func TestRenderHourStep_PastHoursInertOnlyToday(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	loc := time.UTC
	now := time.Date(2026, time.October, 5, 14, 20, 0, 0, loc)

	_, today := telegram.RenderHourStep(env.i18n, "en", "2026-10-05", now, loc)
	for h := 0; h < 24; h++ {
		btn := today.InlineKeyboard[h/6][h%6]
		inert := btn.CallbackData == "fl:noop"
		if inert != (h < 14) {
			t.Errorf("today hour %02d: inert=%v, want %v (%+v)", h, inert, h < 14, btn)
		}
	}
	if change := today.InlineKeyboard[4][0]; change.CallbackData != "fl:d:back" || change.Text != "Change date" {
		t.Errorf("expected Change date button, got %+v", change)
	}

	_, tomorrow := telegram.RenderHourStep(env.i18n, "en", "2026-10-06", now, loc)
	for _, row := range tomorrow.InlineKeyboard[:4] {
		for _, btn := range row {
			if btn.CallbackData == "fl:noop" {
				t.Errorf("tomorrow must have no inert hour, got %+v", btn)
			}
		}
	}
}

func TestRenderMinuteStep_PastMinutesInertInCurrentHour(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	loc := time.UTC
	now := time.Date(2026, time.October, 5, 14, 20, 0, 0, loc)

	_, kb := telegram.RenderMinuteStep(env.i18n, "en", "2026-10-05", 14, now, loc)
	for m := 0; m < 60; m += 5 {
		btn := kb.InlineKeyboard[(m/5)/4][(m/5)%4]
		inert := btn.CallbackData == "fl:noop"
		if inert != (m <= 20) {
			t.Errorf("today 14:%02d: inert=%v, want %v", m, inert, m <= 20)
		}
	}

	_, later := telegram.RenderMinuteStep(env.i18n, "en", "2026-10-05", 15, now, loc)
	_, tomorrow := telegram.RenderMinuteStep(env.i18n, "en", "2026-10-06", 14, now, loc)
	for _, k := range []*telegram.InlineKeyboardMarkup{later, tomorrow} {
		for _, row := range k.InlineKeyboard[:3] {
			for _, btn := range row {
				if btn.CallbackData == "fl:noop" {
					t.Errorf("no minute may be inert here, got %+v", btn)
				}
			}
		}
	}
}

func TestRenderContextHeader_EnglishAndSpanish(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	loc := time.UTC
	now := time.Date(2026, time.October, 5, 14, 20, 0, 0, loc)
	pc := telegram.PickerContext{Emoji: "🧹", Title: "Clean kitchen", Person: "Alice", Date: "2026-10-14"}

	cases := []struct {
		name, locale string
		pc           telegram.PickerContext
		now          time.Time
		want         string
	}{
		{"english", "en", pc, now, "🧹 Clean kitchen · Alice · October 14"},
		{"spanish", "es", pc, now, "🧹 Clean kitchen · Alice · octubre 14"},
		{"title only", "en", telegram.PickerContext{Emoji: "⏰", Title: "Buy medicine"}, now, "⏰ Buy medicine"},
		{"other year", "en", telegram.PickerContext{Emoji: "🧹", Title: "Filter", Date: "2027-01-31"}, now, "🧹 Filter · January 31 2027"},
		{"no title", "en", telegram.PickerContext{Person: "Alice"}, now, ""},
	}
	for _, tc := range cases {
		if got := telegram.RenderContextHeader(env.i18n, tc.locale, tc.pc, tc.now, loc); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestRenderConfirmationStep_SpanishSummaryLines(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	loc := time.UTC
	now := time.Date(2026, time.October, 5, 12, 0, 0, 0, loc)
	due := time.Date(2026, time.October, 5, 23, 5, 0, 0, loc)

	text, kb := telegram.RenderConfirmationStep(env.i18n, "es", telegram.ConfirmationSummary{
		Emoji:       "🧹",
		Title:       "Limpiar bebedero de los niños",
		PersonLabel: "picker.summary_assignee",
		PersonName:  "Pablito",
		DueAt:       due,
		RecType:     "weekly",
		Interval:    2,
		Anchor:      "completion",
	}, now, loc)

	lines := strings.Split(text, "\n")
	want := []string{
		"🧹 Limpiar bebedero de los niños",
		"👤 Pablito",
		"📅 hoy a las 23:05",
		"🔁 Cada 2 semanas",
		"⏭️ La siguiente se calcula al completar",
	}
	for i, w := range want {
		if i+2 >= len(lines) || lines[i+2] != w {
			t.Fatalf("summary line %d: want %q, got summary %q", i, w, text)
		}
	}
	if kb.InlineKeyboard[0][0].Text != "✅ Confirmar" || kb.InlineKeyboard[0][1].Text != "❌ Cancelar" {
		t.Errorf("unexpected Spanish confirm/cancel labels: %+v", kb.InlineKeyboard[0])
	}
}

func TestTypedMinuteInPast_ExplainsInFlowMessage(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	ctx := context.Background()
	env.clock.now = time.Date(2026, 10, 5, 14, 20, 0, 0, env.clock.loc)

	startChoreFlow(ctx, t, env, "Sweep patio")
	steps := []string{"fl:usr:101", "fl:rt:once", "fl:d:today", "fl:h:14", "fl:mi:o"}
	for i, data := range steps {
		if err := pressChoreCallback(ctx, env, int64(10+i), 101, data); err != nil {
			t.Fatalf("%s failed: %v", data, err)
		}
	}
	beforeTyped := env.client.LastSent().MessageID

	if err := polishChoreText(ctx, env, 20, 101, "10"); err != nil {
		t.Fatalf("typed minute failed: %v", err)
	}

	sent := env.client.LastSent()
	if sent.MessageID == beforeTyped {
		t.Fatalf("expected the hour step as a new message below the typed minute")
	}
	wantNotice := env.i18n.T("en", "picker.time_passed")
	if !strings.HasPrefix(sent.Text, "🧹 Sweep patio · Alice · October 5\n\n") ||
		!strings.Contains(sent.Text, wantNotice) || !strings.Contains(sent.Text, "Choose hour") {
		t.Fatalf("expected header, time-passed notice and hour step, got %q", sent.Text)
	}
	deleted := env.client.DeletedMessages()
	if len(deleted) == 0 || deleted[len(deleted)-1] != beforeTyped {
		t.Fatalf("expected previous flow message %d deleted, got %v", beforeTyped, deleted)
	}

	var payload map[string]any
	state, err := env.flow.Get(ctx, flowKeyFor(101, staleTestChatID, 51), &payload)
	if err != nil || state.Step != "chore_hour" || payload["due_date"] != "2026-10-05" {
		t.Fatalf("expected chore_hour with today kept, got step=%v payload=%v err=%v", state, payload, err)
	}
	if res, _ := env.chores.ListMyOpen(ctx, 101); len(res.Occurrences) != 0 {
		t.Fatalf("a past time must create nothing, got %d occurrences", len(res.Occurrences))
	}
}

func TestChangeDate_KeepsRecurrenceAndInterval(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	ctx := context.Background()
	env.clock.now = time.Date(2026, 10, 5, 14, 20, 0, 0, env.clock.loc)

	startChoreFlow(ctx, t, env, "Water plants")
	for i, data := range []string{"fl:usr:101", "fl:rt:weekly", "fl:iv:2", "fl:d:2026-10-14"} {
		if err := pressChoreCallback(ctx, env, int64(10+i), 101, data); err != nil {
			t.Fatalf("%s failed: %v", data, err)
		}
	}
	if err := pressChoreCallback(ctx, env, 20, 101, "fl:d:back"); err != nil {
		t.Fatalf("Change date failed: %v", err)
	}

	edit := env.client.LastEdited()
	if edit == nil || !strings.Contains(edit.Text, env.i18n.T("en", "picker.prompt_date")) {
		t.Fatalf("expected calendar after Change date, got %+v", edit)
	}
	var payload map[string]any
	state, err := env.flow.Get(ctx, flowKeyFor(101, staleTestChatID, 51), &payload)
	if err != nil || state.Step != "chore_date" {
		t.Fatalf("expected chore_date, got %+v err=%v", state, err)
	}
	if payload["recurrence_type"] != "weekly" || payload["recurrence_interval"] != float64(2) {
		t.Fatalf("expected weekly every 2 kept, got %v", payload)
	}
	if payload["cal_month"] != float64(10) || payload["due_date"] != nil {
		t.Fatalf("expected calendar on October with the date cleared, got %v", payload)
	}
}

func TestTypedAnswer_OldFlowMessageTakesExpiredPath(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	ctx := context.Background()

	namePrompt := startChoreFlow(ctx, t, env, "Clean kitchen")
	if err := pressOnMessage(ctx, env, 30, 101, 51, namePrompt, "fl:usr:101"); err != nil {
		t.Fatalf("callback on superseded message failed: %v", err)
	}
	ans := env.client.LastAnswer()
	if ans == nil || ans.Text != env.i18n.T("en", "general.flow_expired") {
		t.Fatalf("expected expired answer on superseded message, got %+v", ans)
	}
	if state, _ := env.flow.Get(ctx, flowKeyFor(101, staleTestChatID, 51), nil); state == nil || state.Step != "chore_assignee" {
		t.Fatalf("superseded callback must not change the flow, got %+v", state)
	}
}

func TestTypedAnswer_DeleteFailureRemovesButtons(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	ctx := context.Background()
	env.client.onDeleteMessage = func(_, _ int64) error { return errors.New("message can't be deleted") }

	namePrompt := startChoreFlow(ctx, t, env, "Clean kitchen")
	if cleared := env.client.ClearedKeyboards(); len(cleared) != 1 || cleared[0] != namePrompt {
		t.Fatalf("expected buttons removed from superseded message %d, got %v", namePrompt, cleared)
	}

	active := env.client.LastSent().MessageID
	state, _ := env.flow.Get(ctx, flowKeyFor(101, staleTestChatID, 51), nil)
	if state == nil || state.MessageID != active {
		t.Fatalf("expected stored flow message %d, got %+v", active, state)
	}

	// The flow continues in the new message.
	if err := pressChoreCallback(ctx, env, 40, 101, "fl:usr:101"); err != nil {
		t.Fatalf("assignee callback failed: %v", err)
	}
	if edit := env.client.LastEdited(); edit == nil || edit.MessageID != active {
		t.Fatalf("expected the next step to edit message %d, got %+v", active, edit)
	}
}

func TestTypedAnswer_HourStepHeaderInSpanish(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	ctx := context.Background()
	env.clock.now = time.Date(2026, 10, 5, 14, 20, 0, 0, env.clock.loc)

	// Charlie uses Spanish; the header and step text follow his locale.
	charlie := func(id int64, text string) error {
		return env.router.HandleUpdate(ctx, telegram.Update{
			UpdateID: id,
			Message: &telegram.Message{
				MessageID: id + 1000, MessageThreadID: 51,
				Chat: telegram.Chat{ID: staleTestChatID, Type: "supergroup"},
				From: &telegram.User{ID: 103, FirstName: "Charlie"}, Text: text,
			},
		})
	}
	if err := charlie(1, "/add"); err != nil {
		t.Fatalf("/add failed: %v", err)
	}
	if err := charlie(2, "Clean kitchen"); err != nil {
		t.Fatalf("name failed: %v", err)
	}
	for i, data := range []string{"fl:usr:103", "fl:rt:once", "fl:d:2026-10-14"} {
		if err := pressChoreCallback(ctx, env, int64(10+i), 103, data); err != nil {
			t.Fatalf("%s failed: %v", data, err)
		}
	}

	edit := env.client.LastEdited()
	if edit == nil || !strings.HasPrefix(edit.Text, "🧹 Clean kitchen · Charlie · octubre 14\n\n") {
		t.Fatalf("expected Spanish header before the hour step, got %+v", edit)
	}
}
