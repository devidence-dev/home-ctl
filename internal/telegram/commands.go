package telegram

import (
	"context"
	"fmt"
	"log/slog"
)

// Translator translates message keys for a given locale.
type Translator interface {
	T(locale, key string, args ...any) string
}

var (
	groupCommandNames   = []string{"add", "list", "status", "help"}
	privateCommandNames = []string{"start", "list", "status", "help"}
)

// RegisterCommands registers localized command menus with Telegram for the household group and private chats.
// Failures are logged and registration continues best-effort.
func RegisterCommands(ctx context.Context, client Client, translator Translator, chatID int64, locales []string, logger *slog.Logger) {
	if client == nil || translator == nil {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}

	targets := []struct {
		scopeName string
		scope     BotCommandScope
		commands  []string
	}{
		{
			scopeName: "group",
			scope:     BotCommandScope{Type: "chat", ChatID: chatID},
			commands:  groupCommandNames,
		},
		{
			scopeName: "private",
			scope:     BotCommandScope{Type: "all_private_chats"},
			commands:  privateCommandNames,
		},
	}

	langs := []string{""}
	seen := map[string]bool{"": true}
	for _, l := range locales {
		if !seen[l] {
			seen[l] = true
			langs = append(langs, l)
		}
	}

	for _, target := range targets {
		for _, lang := range langs {
			cmds := make([]BotCommand, len(target.commands))
			for i, name := range target.commands {
				key := fmt.Sprintf("telegram.commands.%s.description", name)
				// ponytail: private /list covers chores and reminders only, so it gets its own text
				// instead of the group wording that also mentions items.
				if target.scopeName == "private" && name == "list" {
					key = "telegram.commands.private_list.description"
				}
				cmds[i] = BotCommand{
					Command:     name,
					Description: translator.T(lang, key),
				}
			}

			if err := client.SetMyCommands(ctx, target.scope, lang, cmds); err != nil {
				logger.Error("failed to register bot commands", "scope", target.scopeName, "lang", lang, "err", err)
			}
		}
	}
}
