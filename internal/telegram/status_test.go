package telegram_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/telegram"
)

func TestStatus_StartHelpAndStatus(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}

	ctx := context.Background()

	// 1. Charlie (103) is initially NOT connected.
	// Charlie sends /start in private chat
	uCharlieStart := telegram.Update{
		UpdateID: 1,
		Message: &telegram.Message{
			MessageID: 10,
			Chat:      telegram.Chat{ID: 103, Type: "private"},
			From:      &telegram.User{ID: 103, FirstName: "Charlie"},
			Text:      "/start",
		},
	}
	if err := env.router.HandleUpdate(ctx, uCharlieStart); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last := env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "home-ctl") {
		t.Fatalf("expected welcome text, got: %+v", last)
	}

	// Verify Charlie is now connected
	connected, err := env.household.IsConnected(ctx, 103)
	if err != nil || !connected {
		t.Fatalf("Charlie should now be connected: %v", connected)
	}

	// 2. /status in group chat shows all connected
	uStatus := telegram.Update{
		UpdateID: 2,
		Message: &telegram.Message{
			MessageID:       11,
			MessageThreadID: 42,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: 101, FirstName: "Alice"},
			Text:            "/status",
		},
	}
	if err := env.router.HandleUpdate(ctx, uStatus); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "🏠 Household members") {
		t.Fatalf("expected status header, got: %+v", last)
	}
	if !strings.Contains(last.Text, "Alice — connected") || !strings.Contains(last.Text, "Bob — connected") {
		t.Fatalf("expected Alice and Bob connected, got: %s", last.Text)
	}

	// 3. /help text
	uHelp := telegram.Update{
		UpdateID: 3,
		Message: &telegram.Message{
			MessageID: 12,
			Chat:      telegram.Chat{ID: 101, Type: "private"},
			From:      &telegram.User{ID: 101, FirstName: "Alice"},
			Text:      "/help",
		},
	}
	if err := env.router.HandleUpdate(ctx, uHelp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "home-ctl") {
		t.Fatalf("expected help text, got: %+v", last)
	}
}

func TestNotifier_SendPrivateAndGroup(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}

	notifier := telegram.NewNotifier(env.client, env.household, env.i18n, -1001234567890, "en")
	ctx := context.Background()

	// 1. SendPrivateNotification for chore
	msgID, err := notifier.SendPrivateNotification(ctx, 101, "chore", 99, "Clean kitchen", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msgID == 0 {
		t.Fatalf("expected valid message ID, got 0")
	}
	last := env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "Clean kitchen") {
		t.Fatalf("expected chore title in notification, got: %+v", last)
	}
	if len(last.Markup.InlineKeyboard[0]) != 2 {
		t.Fatalf("expected Done and Postpone buttons on due notification")
	}
	if last.Markup.InlineKeyboard[0][0].CallbackData != "ch:c:99" {
		t.Errorf("expected ch:c:99 callback, got: %s", last.Markup.InlineKeyboard[0][0].CallbackData)
	}
	if last.Markup.InlineKeyboard[0][1].CallbackData != "ch:p:99" {
		t.Errorf("expected ch:p:99 callback, got: %s", last.Markup.InlineKeyboard[0][1].CallbackData)
	}

	// 2. SendPrivateNotification for reminder
	_, err = notifier.SendPrivateNotification(ctx, 102, "reminder", 88, "Take medicine", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last = env.client.LastSent()
	if last == nil || !strings.Contains(last.Text, "Take medicine") {
		t.Fatalf("expected reminder title, got: %+v", last)
	}
	if last.Markup.InlineKeyboard[0][0].CallbackData != "re:c:88" {
		t.Errorf("expected re:c:88 callback, got: %s", last.Markup.InlineKeyboard[0][0].CallbackData)
	}

	// 3. SendGroupTopicUpdate
	_, err = notifier.SendGroupTopicUpdate(ctx, 51, "Group message")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last = env.client.LastSent()
	if last == nil || last.Text != "Group message" || last.ThreadID != 51 {
		t.Fatalf("expected group update sent to thread 51, got: %+v", last)
	}
}

func TestStatus_MixedStateInEnglishAndSpanish(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}
	ctx := context.Background()

	// Alice (en) asks in the group; Charlie (es) is still pending.
	statusFrom := func(userID int64) {
		t.Helper()
		err := env.router.HandleUpdate(ctx, telegram.Update{Message: &telegram.Message{
			MessageID:       20,
			MessageThreadID: 42,
			Chat:            telegram.Chat{ID: -1001234567890, Type: "supergroup"},
			From:            &telegram.User{ID: userID, FirstName: "Someone"},
			Text:            "/status",
		}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	statusFrom(101)
	en := env.client.LastSent()
	for _, want := range []string{
		"🏠 Household members",
		"✅ Alice — connected",
		"✅ Bob — connected",
		"⏳ Charlie — pending: open the bot and tap Start",
	} {
		if !strings.Contains(en.Text, want) {
			t.Errorf("English status missing %q, got:\n%s", want, en.Text)
		}
	}

	statusFrom(103)
	es := env.client.LastSent()
	for _, want := range []string{
		"🏠 Miembros del hogar",
		"✅ Alice — conectado",
		"⏳ Charlie — pendiente: abre el bot y toca Iniciar",
	} {
		if !strings.Contains(es.Text, want) {
			t.Errorf("Spanish status missing %q, got:\n%s", want, es.Text)
		}
	}
}
