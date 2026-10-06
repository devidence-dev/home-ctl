package telegram_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/telegram"
)

func privateText(updateID, userID int64, text string) telegram.Update {
	return telegram.Update{UpdateID: updateID, Message: &telegram.Message{
		MessageID: updateID, Chat: telegram.Chat{ID: userID, Type: "private"},
		From: &telegram.User{ID: userID}, Text: text,
	}}
}

// "Choose date/time" in the postpone menu: typed input goes through the same domain command as the presets.
func TestTypedPostpone_ChoresAndReminders(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	ctx := context.Background()
	loc := env.clock.loc
	newDue := time.Date(2026, 10, 10, 10, 0, 0, 0, loc)

	cdef, _ := env.chores.CreateDefinition(ctx, 101, "Clean bathroom", "🚿", "", nil)
	cocc, _ := env.chores.ScheduleOccurrence(ctx, 101, cdef.ID, 101, env.clock.now.Add(time.Hour))
	rdef, _ := env.reminders.CreateDefinition(ctx, 101, 101, "Buy medicine", "💊", "", nil)
	rocc, _ := env.reminders.ScheduleOccurrence(ctx, 101, rdef.ID, env.clock.now.Add(time.Hour))

	for i, tc := range []struct {
		menu, custom string
		topic        int64
		groupWant    string
		due          func() time.Time
	}{
		{fmt.Sprintf("ch:p:%d", cocc.ID), fmt.Sprintf("ch:p:%d:custom", cocc.ID), 51, "🚿 Clean bathroom — Alice postponed it until Sat, Oct 10, 10:00",
			func() time.Time { o, _ := env.chores.GetOccurrence(ctx, cocc.ID); return o.DueAt }},
		{fmt.Sprintf("re:p:%d", rocc.ID), fmt.Sprintf("re:p:%d:custom", rocc.ID), 63, "💊 Buy medicine — Alice postponed it until Sat, Oct 10, 10:00",
			func() time.Time { o, _ := env.reminders.GetOccurrence(ctx, rocc.ID); return o.DueAt }},
	} {
		base := int64(10 * (i + 1))

		// The postpone menu offers the typed option.
		if err := env.router.HandleUpdate(ctx, callback(base, 101, 101, tc.menu)); err != nil {
			t.Fatalf("menu failed: %v", err)
		}
		found := false
		for _, row := range env.client.LastEdited().Markup.InlineKeyboard {
			for _, b := range row {
				found = found || b.CallbackData == tc.custom
			}
		}
		if !found {
			t.Fatalf("postpone menu lacks the date/time option")
		}

		// Choosing it asks for text; garbage keeps the flow open with guidance.
		if err := env.router.HandleUpdate(ctx, callback(base+1, 101, 101, tc.custom)); err != nil {
			t.Fatalf("custom failed: %v", err)
		}
		if got := env.client.LastSent().Text; !strings.Contains(got, "YYYY-MM-DD HH:MM") {
			t.Fatalf("expected typed-input prompt, got %q", got)
		}
		_ = env.router.HandleUpdate(ctx, privateText(base+2, 101, "banana"))
		if got := env.client.LastSent().Text; !strings.Contains(got, "Invalid date/time format") {
			t.Fatalf("expected guidance for unparsable input, got %q", got)
		}
		_ = env.router.HandleUpdate(ctx, privateText(base+3, 101, "2020-01-01 10:00"))
		if got := env.client.LastSent().Text; !strings.Contains(got, "cannot be in the past") {
			t.Fatalf("expected past-time guidance, got %q", got)
		}

		// A valid time postpones, announces in the topic and confirms privately.
		_ = env.router.HandleUpdate(ctx, privateText(base+4, 101, "2026-10-10 10:00"))
		if !tc.due().Equal(newDue) {
			t.Fatalf("expected due %v, got %v", newDue, tc.due())
		}
		var group, confirm *sentMessage
		env.client.mu.Lock()
		msgs := env.client.sentMessages
		if len(msgs) >= 2 {
			group, confirm = &msgs[len(msgs)-2], &msgs[len(msgs)-1]
		}
		env.client.mu.Unlock()
		if group == nil || group.ThreadID != tc.topic || !strings.Contains(group.Text, tc.groupWant) {
			t.Fatalf("expected group update %q in topic %d, got %+v", tc.groupWant, tc.topic, group)
		}
		if !strings.Contains(confirm.Text, "Postponed until Sat, Oct 10, 10:00") {
			t.Fatalf("expected private confirmation, got %q", confirm.Text)
		}

		// The flow is finished: further text is not treated as a postponement.
		_ = env.router.HandleUpdate(ctx, privateText(base+5, 101, "2026-10-11 10:00"))
		if !tc.due().Equal(newDue) {
			t.Fatalf("flow should be closed after a successful postponement, due=%v", tc.due())
		}
	}
}

// If the occurrence is completed while the user is typing, the typed time must not move it.
func TestTypedPostpone_StaleAndCancelled(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	ctx := context.Background()
	def, _ := env.chores.CreateDefinition(ctx, 101, "Mop", "🧽", "", nil)
	due := env.clock.now.Add(time.Hour)
	occ, _ := env.chores.ScheduleOccurrence(ctx, 101, def.ID, 101, due)

	// Cancel path: no change, flow closed.
	_ = env.router.HandleUpdate(ctx, callback(1, 101, 101, fmt.Sprintf("ch:p:%d:custom", occ.ID)))
	_ = env.router.HandleUpdate(ctx, callback(2, 101, 101, "fl:c"))
	_ = env.router.HandleUpdate(ctx, privateText(3, 101, "/help"))
	if got, _ := env.chores.GetOccurrence(ctx, occ.ID); !got.DueAt.Equal(due) {
		t.Fatalf("cancelled flow changed due_at to %v", got.DueAt)
	}

	// Stale path: completed elsewhere while typing.
	_ = env.router.HandleUpdate(ctx, callback(4, 101, 101, fmt.Sprintf("ch:p:%d:custom", occ.ID)))
	if _, _, err := env.chores.Complete(ctx, 101, occ.ID, env.clock.now); err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	groupBefore := len(env.client.sentMessages)
	_ = env.router.HandleUpdate(ctx, privateText(5, 101, "2026-10-10 10:00"))
	if got := env.client.LastSent().Text; got != env.i18n.T("en", "general.already_resolved") {
		t.Fatalf("expected already-resolved reply, got %q", got)
	}
	if len(env.client.sentMessages) != groupBefore+1 {
		t.Fatalf("stale typed postpone must only reply privately")
	}
	if got, _ := env.chores.GetOccurrence(ctx, occ.ID); !got.DueAt.Equal(due) {
		t.Fatalf("completed occurrence moved to %v", got.DueAt)
	}
}

// Another member cannot postpone through a forged typed flow: authorization lives in the domain.
func TestTypedPostpone_NonOwnerRejected(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	ctx := context.Background()
	def, _ := env.chores.CreateDefinition(ctx, 101, "Mop", "🧽", "", nil)
	due := env.clock.now.Add(time.Hour)
	occ, _ := env.chores.ScheduleOccurrence(ctx, 101, def.ID, 101, due)

	// Bob (102) forges the callback for Alice's occurrence and then types a time.
	_ = env.router.HandleUpdate(ctx, callback(1, 102, 102, fmt.Sprintf("ch:p:%d:custom", occ.ID)))
	_ = env.router.HandleUpdate(ctx, privateText(2, 102, "2026-10-10 10:00"))
	if got := env.client.LastSent().Text; got != env.i18n.T("en", "chores.not_assignee") {
		t.Fatalf("expected not-assignee reply, got %q", got)
	}
	if got, _ := env.chores.GetOccurrence(ctx, occ.ID); !got.DueAt.Equal(due) {
		t.Fatalf("non-owner moved due_at to %v", got.DueAt)
	}
}
