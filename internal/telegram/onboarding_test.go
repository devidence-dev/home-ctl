package telegram_test

import (
	"context"
	"testing"

	"github.com/px1/home-ctl/internal/telegram"
)

const groupChatID = -1001234567890

func privateMessage(userID int64, text string) telegram.Update {
	return telegram.Update{Message: &telegram.Message{
		MessageID: 1,
		Chat:      telegram.Chat{ID: userID, Type: "private"},
		From:      &telegram.User{ID: userID, FirstName: "Test"},
		Text:      text,
	}}
}

func TestOnboarding_PrivateCommandConnects(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}
	ctx := context.Background()

	// Charlie (103) is not connected; /list in private connects him and still posts his list.
	if err := env.router.HandleUpdate(ctx, privateMessage(103, "/list")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertConnected(t, env, 103, true)
	if last := env.client.LastSent(); last == nil || last.Text == env.i18n.T("es", "general.unconnected_notice") {
		t.Fatalf("expected private list, got: %+v", last)
	}
}

func TestOnboarding_PrivateButtonPressConnects(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}
	ctx := context.Background()

	press := telegram.Update{CallbackQuery: &telegram.CallbackQuery{
		ID:   "cb-1",
		From: telegram.User{ID: 103, FirstName: "Charlie"},
		Data: "list:ch",
		Message: &telegram.Message{
			MessageID: 7,
			Chat:      telegram.Chat{ID: 103, Type: "private"},
		},
	}}
	if err := env.router.HandleUpdate(ctx, press); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertConnected(t, env, 103, true)
}

func TestOnboarding_GroupCommandDoesNotConnect(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}
	ctx := context.Background()

	// Charlie (103) is not connected; group activity never proves the bot can write to him privately.
	group := telegram.Update{Message: &telegram.Message{
		MessageID:       2,
		MessageThreadID: 42,
		Chat:            telegram.Chat{ID: groupChatID, Type: "supergroup"},
		From:            &telegram.User{ID: 103, FirstName: "Charlie"},
		Text:            "/status",
	}}
	if err := env.router.HandleUpdate(ctx, group); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertConnected(t, env, 103, false)
}

func TestOnboarding_StartStillRepliesHelp(t *testing.T) {
	env, err := setupTestEnv()
	if err != nil {
		t.Fatalf("failed to setup test env: %v", err)
	}
	ctx := context.Background()

	if err := env.router.HandleUpdate(ctx, privateMessage(103, "/start")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	last := env.client.LastSent()
	if last == nil || last.Text != env.i18n.T("es", "help.text") {
		t.Fatalf("expected help reply after /start, got: %+v", last)
	}
	assertConnected(t, env, 103, true)
}

func TestUnconnectedNotice_ConnectButton(t *testing.T) {
	ctx := context.Background()
	chorePrompt := telegram.Update{Message: &telegram.Message{
		MessageID:       3,
		MessageThreadID: 51,
		Chat:            telegram.Chat{ID: groupChatID, Type: "supergroup"},
		From:            &telegram.User{ID: 103, FirstName: "Charlie"},
		Text:            "/list",
	}}

	t.Run("with username sends URL button", func(t *testing.T) {
		env, err := setupTestEnv()
		if err != nil {
			t.Fatalf("failed to setup test env: %v", err)
		}
		if err := env.router.HandleUpdate(ctx, chorePrompt); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		last := env.client.LastSent()
		if last == nil || last.Markup == nil || len(last.Markup.InlineKeyboard) != 1 {
			t.Fatalf("expected one button row on notice, got: %+v", last)
		}
		btn := last.Markup.InlineKeyboard[0][0]
		if btn.URL != "https://t.me/home_ctl_bot?start=connect" || btn.CallbackData != "" {
			t.Fatalf("expected connect URL button, got: %+v", btn)
		}
		if btn.Text != env.i18n.T("es", "general.connect_button") {
			t.Fatalf("expected localized button label, got %q", btn.Text)
		}
	})

	t.Run("without username sends no button", func(t *testing.T) {
		env, err := setupTestEnv()
		if err != nil {
			t.Fatalf("failed to setup test env: %v", err)
		}
		router := telegram.NewRouter(telegram.RouterConfig{
			ChatID:           groupChatID,
			ShoppingTopicID:  42,
			ChoresTopicID:    51,
			RemindersTopicID: 63,
			DefaultLocale:    "en",
		}, telegram.RouterDeps{Client: env.client, Household: env.household, Shopping: env.shopping, Chores: env.chores, Reminders: env.reminders, Flow: env.flow, I18n: env.i18n, Clock: env.clock})
		if err := router.HandleUpdate(ctx, chorePrompt); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		last := env.client.LastSent()
		if last == nil || last.Markup != nil {
			t.Fatalf("expected notice without button, got: %+v", last)
		}
	})

	t.Run("start connect connects the member", func(t *testing.T) {
		env, err := setupTestEnv()
		if err != nil {
			t.Fatalf("failed to setup test env: %v", err)
		}
		if err := env.router.HandleUpdate(ctx, privateMessage(103, "/start connect")); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertConnected(t, env, 103, true)
	})
}

func assertConnected(t *testing.T, env *testEnv, userID int64, want bool) {
	t.Helper()
	got, err := env.household.IsConnected(context.Background(), userID)
	if err != nil {
		t.Fatalf("IsConnected(%d) failed: %v", userID, err)
	}
	if got != want {
		t.Fatalf("user %d connected = %v, want %v", userID, got, want)
	}
}
