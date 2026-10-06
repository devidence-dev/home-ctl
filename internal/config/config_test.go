package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/config"
)

func writeTestFile(t *testing.T, dir, filename, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0600)
	if err != nil {
		t.Fatalf("failed to write test file %s: %v", filename, err)
	}
}

func setupValidConfigDir(t *testing.T) (string, string) {
	t.Helper()
	configDir := t.TempDir()
	localesDir := t.TempDir()

	writeTestFile(t, localesDir, "en.yaml", "common:\n  ok: OK\n")
	writeTestFile(t, localesDir, "es.yaml", "common:\n  ok: OK\n")

	writeTestFile(t, configDir, "users.yaml", `
users:
  - id: 1001
    name: "Alice"
    username: "alice"
    locale: "en"
  - id: 1002
    name: "Bob"
    username: "bob"
    locale: "es"
`)

	writeTestFile(t, configDir, "telegram.yaml", `
telegram:
  chat_id: -1001234567890
  topics:
    shopping: 10
    chores: 20
    reminders: 30
`)

	writeTestFile(t, configDir, "app.yaml", `
app:
  default_locale: "en"
  database_path: "data/test.db"
  notifications:
    unresolved_reminder_interval: 1h
  flow:
    ttl: 30m
  calendar:
    default_time_of_day: "09:00"
`)

	return configDir, localesDir
}

func mockEnv(m map[string]string) func(string) string {
	return func(key string) string {
		return m[key]
	}
}

func TestLoad_Scenarios(t *testing.T) {
	tests := []struct {
		name         string
		mutateSetup  func(configDir, localesDir string, env map[string]string)
		expectErrSub string
		checkResult  func(t *testing.T, cfg *config.Config)
	}{
		{
			name: "valid configuration",
			mutateSetup: func(_, _ string, env map[string]string) {
				env["TELEGRAM_BOT_TOKEN"] = "valid_token"
				env["APP_TIMEZONE"] = "America/Guayaquil"
			},
			checkResult: func(t *testing.T, cfg *config.Config) {
				if cfg.BotToken != "valid_token" {
					t.Fatalf("expected bot token 'valid_token', got %q", cfg.BotToken)
				}
				if cfg.Timezone != "America/Guayaquil" {
					t.Fatalf("expected timezone America/Guayaquil, got %q", cfg.Timezone)
				}
				if len(cfg.Users) != 2 {
					t.Fatalf("expected 2 users, got %d", len(cfg.Users))
				}
				if cfg.Telegram.Topics.Shopping != 10 {
					t.Fatalf("expected shopping topic 10, got %d", cfg.Telegram.Topics.Shopping)
				}
				if cfg.App.Notifications.UnresolvedReminderInterval != time.Hour {
					t.Fatalf("expected 1h interval, got %v", cfg.App.Notifications.UnresolvedReminderInterval)
				}
			},
		},
		{
			name: "missing TELEGRAM_BOT_TOKEN",
			mutateSetup: func(_, _ string, env map[string]string) {
				delete(env, "TELEGRAM_BOT_TOKEN")
				env["APP_TIMEZONE"] = "America/Guayaquil"
			},
			expectErrSub: "TELEGRAM_BOT_TOKEN environment variable is required",
		},
		{
			name: "missing APP_TIMEZONE",
			mutateSetup: func(_, _ string, env map[string]string) {
				env["TELEGRAM_BOT_TOKEN"] = "token"
				delete(env, "APP_TIMEZONE")
			},
			expectErrSub: "APP_TIMEZONE environment variable is required",
		},
		{
			name: "invalid APP_TIMEZONE Mars/Olympus",
			mutateSetup: func(_, _ string, env map[string]string) {
				env["TELEGRAM_BOT_TOKEN"] = "token"
				env["APP_TIMEZONE"] = "Mars/Olympus"
			},
			expectErrSub: "Mars/Olympus",
		},
		{
			name: "syntax error in telegram.yaml",
			mutateSetup: func(configDir, _ string, env map[string]string) {
				env["TELEGRAM_BOT_TOKEN"] = "token"
				env["APP_TIMEZONE"] = "UTC"
				writeTestFile(t, configDir, "telegram.yaml", ":bad yaml:")
			},
			expectErrSub: "telegram.yaml",
		},
		{
			name: "token secret in YAML rejected",
			mutateSetup: func(configDir, _ string, env map[string]string) {
				env["TELEGRAM_BOT_TOKEN"] = "token"
				env["APP_TIMEZONE"] = "UTC"
				writeTestFile(t, configDir, "telegram.yaml", `
telegram:
  token: "my-secret-token"
  chat_id: -100
  topics:
    shopping: 1
    chores: 2
    reminders: 3
`)
			},
			expectErrSub: "secrets must come from the environment",
		},
		{
			name: "timezone in YAML rejected",
			mutateSetup: func(configDir, _ string, env map[string]string) {
				env["TELEGRAM_BOT_TOKEN"] = "token"
				env["APP_TIMEZONE"] = "UTC"
				writeTestFile(t, configDir, "app.yaml", `
app:
  timezone: "UTC"
  default_locale: "en"
`)
			},
			expectErrSub: "timezone must come from APP_TIMEZONE environment variable",
		},
		{
			name: "duplicate user ID rejected",
			mutateSetup: func(configDir, _ string, env map[string]string) {
				env["TELEGRAM_BOT_TOKEN"] = "token"
				env["APP_TIMEZONE"] = "UTC"
				writeTestFile(t, configDir, "users.yaml", `
users:
  - id: 1001
    name: "Alice"
  - id: 1001
    name: "Alice Clone"
`)
			},
			expectErrSub: "duplicate user id 1001",
		},
		{
			name: "missing topic reminders rejected",
			mutateSetup: func(configDir, _ string, env map[string]string) {
				env["TELEGRAM_BOT_TOKEN"] = "token"
				env["APP_TIMEZONE"] = "UTC"
				writeTestFile(t, configDir, "telegram.yaml", `
telegram:
  chat_id: -1001234567890
  topics:
    shopping: 10
    chores: 20
`)
			},
			expectErrSub: "telegram.topics.reminders",
		},
		{
			name: "non-positive unresolved reminder interval rejected",
			mutateSetup: func(configDir, _ string, env map[string]string) {
				env["TELEGRAM_BOT_TOKEN"] = "token"
				env["APP_TIMEZONE"] = "UTC"
				writeTestFile(t, configDir, "app.yaml", `
app:
  default_locale: "en"
  notifications:
    unresolved_reminder_interval: 0s
  flow:
    ttl: 10m
`)
			},
			expectErrSub: "unresolved_reminder_interval must be positive",
		},
		{
			name: "unsupported default locale fr",
			mutateSetup: func(configDir, _ string, env map[string]string) {
				env["TELEGRAM_BOT_TOKEN"] = "token"
				env["APP_TIMEZONE"] = "UTC"
				writeTestFile(t, configDir, "app.yaml", `
app:
  default_locale: "fr"
  notifications:
    unresolved_reminder_interval: 10m
  flow:
    ttl: 10m
`)
			},
			expectErrSub: "unsupported default_locale \"fr\"",
		},
		{
			name: "invalid user ID <= 0",
			mutateSetup: func(configDir, _ string, env map[string]string) {
				env["TELEGRAM_BOT_TOKEN"] = "token"
				env["APP_TIMEZONE"] = "UTC"
				writeTestFile(t, configDir, "users.yaml", `
users:
  - id: 0
    name: "Invalid"
`)
			},
			expectErrSub: "invalid user id 0",
		},
		{
			name: "invalid default time of day",
			mutateSetup: func(configDir, _ string, env map[string]string) {
				env["TELEGRAM_BOT_TOKEN"] = "token"
				env["APP_TIMEZONE"] = "UTC"
				writeTestFile(t, configDir, "app.yaml", `
app:
  calendar:
    default_time_of_day: "25:00"
  notifications:
    unresolved_reminder_interval: 10m
  flow:
    ttl: 10m
`)
			},
			expectErrSub: "invalid default_time_of_day",
		},
		{
			name: "non-positive flow ttl",
			mutateSetup: func(configDir, _ string, env map[string]string) {
				env["TELEGRAM_BOT_TOKEN"] = "token"
				env["APP_TIMEZONE"] = "UTC"
				writeTestFile(t, configDir, "app.yaml", `
app:
  notifications:
    unresolved_reminder_interval: 10m
  flow:
    ttl: 0s
`)
			},
			expectErrSub: "flow.ttl must be positive",
		},
		{
			name: "missing telegram section",
			mutateSetup: func(configDir, _ string, env map[string]string) {
				env["TELEGRAM_BOT_TOKEN"] = "token"
				env["APP_TIMEZONE"] = "UTC"
				writeTestFile(t, configDir, "telegram.yaml", `
other_section:
  value: 123
`)
			},
			expectErrSub: "missing 'telegram' section",
		},
		{
			name: "missing telegram.topics section",
			mutateSetup: func(configDir, _ string, env map[string]string) {
				env["TELEGRAM_BOT_TOKEN"] = "token"
				env["APP_TIMEZONE"] = "UTC"
				writeTestFile(t, configDir, "telegram.yaml", `
telegram:
  chat_id: -100
`)
			},
			expectErrSub: "missing 'telegram.topics'",
		},
		{
			name: "invalid chat_id zero",
			mutateSetup: func(configDir, _ string, env map[string]string) {
				env["TELEGRAM_BOT_TOKEN"] = "token"
				env["APP_TIMEZONE"] = "UTC"
				writeTestFile(t, configDir, "telegram.yaml", `
telegram:
  chat_id: 0
  topics:
    shopping: 1
    chores: 2
    reminders: 3
`)
			},
			expectErrSub: "invalid telegram.chat_id",
		},
		{
			name: "invalid shopping topic id <= 0",
			mutateSetup: func(configDir, _ string, env map[string]string) {
				env["TELEGRAM_BOT_TOKEN"] = "token"
				env["APP_TIMEZONE"] = "UTC"
				writeTestFile(t, configDir, "telegram.yaml", `
telegram:
  chat_id: -100
  topics:
    shopping: 0
    chores: 2
    reminders: 3
`)
			},
			expectErrSub: "invalid or non-positive topic id for shopping",
		},
		{
			name: "missing users.yaml file",
			mutateSetup: func(configDir, _ string, env map[string]string) {
				env["TELEGRAM_BOT_TOKEN"] = "token"
				env["APP_TIMEZONE"] = "UTC"
				_ = os.Remove(filepath.Join(configDir, "users.yaml"))
			},
			expectErrSub: "users.yaml",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfgDir, locDir := setupValidConfigDir(t)
			env := make(map[string]string)
			tc.mutateSetup(cfgDir, locDir, env)

			res, err := config.Load(config.Options{
				ConfigDir:  cfgDir,
				LocalesDir: locDir,
				Getenv:     mockEnv(env),
			})

			if tc.expectErrSub != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.expectErrSub)
				}
				if !strings.Contains(err.Error(), tc.expectErrSub) {
					t.Fatalf("expected error containing %q, got %q", tc.expectErrSub, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if tc.checkResult != nil {
					tc.checkResult(t, res)
				}
			}
		})
	}
}
