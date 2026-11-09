package telegram_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/px1/home-ctl/internal/i18n"
	"github.com/px1/home-ctl/internal/telegram"
)

func TestRegisterCommands_CommandListsAndScopes(t *testing.T) {
	bundle, err := i18n.NewBundle(filepath.Join("..", "..", "locales"), "en", i18n.DefaultRequiredKeys, nil)
	if err != nil {
		t.Fatalf("failed to initialize bundle: %v", err)
	}

	fake := newFakeClient()
	chatID := int64(-1003916572365)
	locales := bundle.Locales()

	telegram.RegisterCommands(context.Background(), fake, bundle, chatID, locales, nil)

	// Expected calls: 2 scopes x 3 languages ("" + en + es) = 6 calls
	if len(fake.setMyCommandsCalls) != 6 {
		t.Fatalf("expected 6 calls to SetMyCommands, got %d", len(fake.setMyCommandsCalls))
	}

	for _, call := range fake.setMyCommandsCalls {
		switch call.Scope.Type {
		case "chat":
			commandsTestCheckGroupCall(t, chatID, call)
		case "all_private_chats":
			commandsTestCheckPrivateCall(t, call)
		default:
			t.Fatalf("unexpected scope type: %s", call.Scope.Type)
		}
	}
}

// commandsTestCheckGroupCall checks the group-scoped menu: add, list, status, help.
func commandsTestCheckGroupCall(t *testing.T, chatID int64, call setMyCommandsCall) {
	t.Helper()
	if call.Scope.ChatID != chatID {
		t.Errorf("expected group chat ID %d, got %d", chatID, call.Scope.ChatID)
	}

	// Group list: add, list, status, help
	var names []string
	for _, cmd := range call.Commands {
		names = append(names, cmd.Command)
		if cmd.Command == "start" {
			t.Errorf("group menu must not advertise 'start'")
		}
	}
	commandsTestExpectNames(t, "group", names, []string{"add", "list", "status", "help"})
}

// commandsTestCheckPrivateCall checks the private-chat menu: start, list, status, help.
func commandsTestCheckPrivateCall(t *testing.T, call setMyCommandsCall) {
	t.Helper()
	if call.Scope.ChatID != 0 {
		t.Errorf("expected private chat ID to be 0, got %d", call.Scope.ChatID)
	}

	// Private list: start, list, status, help
	var names []string
	for _, cmd := range call.Commands {
		names = append(names, cmd.Command)
		if cmd.Command == "add" {
			t.Errorf("private menu must not advertise 'add', found %s", cmd.Command)
		}
	}
	commandsTestExpectNames(t, "private", names, []string{"start", "list", "status", "help"})
}

// commandsTestExpectNames compares the command names of one menu with the expected list in order.
func commandsTestExpectNames(t *testing.T, kind string, names, expected []string) {
	t.Helper()
	if len(names) != len(expected) {
		t.Fatalf("expected %d %s commands, got %d (%v)", len(expected), kind, len(names), names)
	}
	for i, exp := range expected {
		if names[i] != exp {
			t.Errorf("expected %s command at %d to be %s, got %s", kind, i, exp, names[i])
		}
	}
}

// commandsTestDescKey returns the locale key of a command's description. Private /list has its own wording.
func commandsTestDescKey(scope, name string) string {
	if scope == "all_private_chats" && name == "list" {
		return "telegram.commands.private_list.description"
	}
	return "telegram.commands." + name + ".description"
}

// commandsTestCheckDescriptions compares each command description in call with the wording of lang in bundle.
func commandsTestCheckDescriptions(t *testing.T, label string, bundle *i18n.Bundle, lang string, call setMyCommandsCall) {
	t.Helper()
	for _, cmd := range call.Commands {
		want := bundle.T(lang, commandsTestDescKey(call.Scope.Type, cmd.Command))
		if cmd.Description != want {
			t.Errorf("%s description mismatch for %s: got %q, want %q", label, cmd.Command, cmd.Description, want)
		}
	}
}

func TestRegisterCommands_LocalizedDescriptionsAndDefaultLocale(t *testing.T) {
	localesDir := filepath.Join("..", "..", "locales")
	bundleEN, err := i18n.NewBundle(localesDir, "en", i18n.DefaultRequiredKeys, nil)
	if err != nil {
		t.Fatalf("failed to initialize English bundle: %v", err)
	}
	bundleES, err := i18n.NewBundle(localesDir, "es", i18n.DefaultRequiredKeys, nil)
	if err != nil {
		t.Fatalf("failed to initialize Spanish bundle: %v", err)
	}

	t.Run("descriptions match locale files and fallback uses default locale", func(t *testing.T) {
		fake := newFakeClient()
		chatID := int64(-1003916572365)
		telegram.RegisterCommands(context.Background(), fake, bundleEN, chatID, bundleEN.Locales(), nil)

		for _, call := range fake.setMyCommandsCalls {
			switch call.LanguageCode {
			case "en":
				commandsTestCheckDescriptions(t, "en", bundleEN, "en", call)
			case "es":
				commandsTestCheckDescriptions(t, "es", bundleEN, "es", call)
			case "":
				// Fallback must use default locale (en)
				commandsTestCheckDescriptions(t, "fallback", bundleEN, "en", call)
			default:
				t.Errorf("unexpected language code: %q", call.LanguageCode)
			}
		}
	})

	t.Run("fallback uses Spanish when default locale is es", func(t *testing.T) {
		fake := newFakeClient()
		chatID := int64(-1003916572365)
		telegram.RegisterCommands(context.Background(), fake, bundleES, chatID, bundleES.Locales(), nil)

		foundFallback := false
		for _, call := range fake.setMyCommandsCalls {
			if call.LanguageCode == "" {
				foundFallback = true
				commandsTestCheckDescriptions(t, "es fallback", bundleES, "es", call)
			}
		}
		if !foundFallback {
			t.Fatalf("expected fallback call with empty language code")
		}
	})
}

func TestRegisterCommands_FailureToleranceAndLogging(t *testing.T) {
	bundle, err := i18n.NewBundle(filepath.Join("..", "..", "locales"), "en", i18n.DefaultRequiredKeys, nil)
	if err != nil {
		t.Fatalf("failed to initialize bundle: %v", err)
	}

	fake := newFakeClient()
	chatID := int64(-1003916572365)

	fake.onSetMyCommands = func(scope telegram.BotCommandScope, lang string, _ []telegram.BotCommand) error {
		if scope.Type == "chat" && lang == "es" {
			return errors.New("simulated telegram API error for es group menu")
		}
		return nil
	}

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))

	telegram.RegisterCommands(context.Background(), fake, bundle, chatID, bundle.Locales(), logger)

	// Even though the 'es' group call failed, all 6 calls must still be attempted
	if len(fake.setMyCommandsCalls) != 6 {
		t.Fatalf("expected 6 calls despite one failure, got %d", len(fake.setMyCommandsCalls))
	}

	logOutput := logBuf.String()
	if !bytes.Contains(logBuf.Bytes(), []byte(`"scope":"group"`)) {
		t.Errorf("expected log to contain '\"scope\":\"group\"', got: %s", logOutput)
	}
	if !bytes.Contains(logBuf.Bytes(), []byte(`"lang":"es"`)) {
		t.Errorf("expected log to contain '\"lang\":\"es\"', got: %s", logOutput)
	}
}
