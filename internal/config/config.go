// Package config handles multi-file YAML configuration loading, validation,
// and environment variable extraction for the homectl service.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// UserConfig defines a household member loaded from configuration.
type UserConfig struct {
	ID       int64  `yaml:"id"`
	Name     string `yaml:"name"`
	Username string `yaml:"username"`
	Locale   string `yaml:"locale"`
}

// TelegramTopics defines the mapping of forum topics to their thread IDs.
type TelegramTopics struct {
	Shopping  int64 `yaml:"shopping"`
	Chores    int64 `yaml:"chores"`
	Reminders int64 `yaml:"reminders"`
}

// TelegramConfig holds group and topic configuration for Telegram.
type TelegramConfig struct {
	ChatID int64          `yaml:"chat_id"`
	Topics TelegramTopics `yaml:"topics"`
}

// NotificationsConfig holds notification preferences and intervals.
type NotificationsConfig struct {
	UnresolvedReminderInterval time.Duration `yaml:"unresolved_reminder_interval"`
}

// FlowConfig holds flow state configuration.
type FlowConfig struct {
	TTL time.Duration `yaml:"ttl"`
}

// CalendarConfig holds calendar and scheduling options.
type CalendarConfig struct {
	DefaultTimeOfDay string `yaml:"default_time_of_day"`
}

// AppConfig holds core application runtime settings.
type AppConfig struct {
	DefaultLocale string              `yaml:"default_locale"`
	DatabasePath  string              `yaml:"database_path"`
	Notifications NotificationsConfig `yaml:"notifications"`
	Flow          FlowConfig          `yaml:"flow"`
	Calendar      CalendarConfig      `yaml:"calendar"`
}

// Config represents the complete validated runtime configuration.
type Config struct {
	BotToken string
	Timezone string
	Location *time.Location
	Users    []UserConfig
	Telegram TelegramConfig
	App      AppConfig
}

// Options parameters for loading configuration.
type Options struct {
	ConfigDir  string
	LocalesDir string
	Getenv     func(string) string
}

var timeOfDayRegex = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// Load reads and validates configuration from the provided options.
func Load(opts Options) (*Config, error) {
	getenv := opts.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	configDir := opts.ConfigDir
	if configDir == "" {
		configDir = "config"
	}
	localesDir := opts.LocalesDir
	if localesDir == "" {
		localesDir = "locales"
	}

	// 1. Secrets and environment validation
	botToken := strings.TrimSpace(getenv("TELEGRAM_BOT_TOKEN"))
	if botToken == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN environment variable is required")
	}

	tzStr := strings.TrimSpace(getenv("APP_TIMEZONE"))
	if tzStr == "" {
		return nil, fmt.Errorf("APP_TIMEZONE environment variable is required")
	}
	loc, err := time.LoadLocation(tzStr)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone %q in APP_TIMEZONE: %w", tzStr, err)
	}

	// 2. Load and parse YAML files with secret and timezone checks
	usersFile := filepath.Join(configDir, "users.yaml")
	telegramFile := filepath.Join(configDir, "telegram.yaml")
	appFile := filepath.Join(configDir, "app.yaml")

	usersBytes, err := readAndCheckYAML(usersFile)
	if err != nil {
		return nil, err
	}
	telegramBytes, err := readAndCheckYAML(telegramFile)
	if err != nil {
		return nil, err
	}
	appBytes, err := readAndCheckYAML(appFile)
	if err != nil {
		return nil, err
	}

	// 3. Unmarshal users.yaml
	var usersWrapper struct {
		Users []UserConfig `yaml:"users"`
	}
	if err := yaml.Unmarshal(usersBytes, &usersWrapper); err != nil {
		return nil, fmt.Errorf("error parsing %s: %w", usersFile, err)
	}

	// Check for duplicate user IDs and validate user items
	seenUsers := make(map[int64]bool)
	for _, u := range usersWrapper.Users {
		if u.ID <= 0 {
			return nil, fmt.Errorf("invalid user id %d in %s", u.ID, usersFile)
		}
		if seenUsers[u.ID] {
			return nil, fmt.Errorf("duplicate user id %d in %s", u.ID, usersFile)
		}
		seenUsers[u.ID] = true
	}

	// 4. Unmarshal telegram.yaml
	var telegramRaw map[string]interface{}
	if err := yaml.Unmarshal(telegramBytes, &telegramRaw); err != nil {
		return nil, fmt.Errorf("error parsing %s: %w", telegramFile, err)
	}
	tgSection, ok := telegramRaw["telegram"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing 'telegram' section in %s", telegramFile)
	}
	topicsSection, ok := tgSection["topics"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing 'telegram.topics' in %s", telegramFile)
	}
	// Explicit check for required topics
	requiredTopics := []string{"shopping", "chores", "reminders"}
	for _, topic := range requiredTopics {
		if _, exists := topicsSection[topic]; !exists {
			return nil, fmt.Errorf("missing required topic mapping 'telegram.topics.%s' in %s", topic, telegramFile)
		}
	}

	var telegramWrapper struct {
		Telegram TelegramConfig `yaml:"telegram"`
	}
	if err := yaml.Unmarshal(telegramBytes, &telegramWrapper); err != nil {
		return nil, fmt.Errorf("error parsing %s: %w", telegramFile, err)
	}
	if telegramWrapper.Telegram.ChatID == 0 {
		return nil, fmt.Errorf("invalid telegram.chat_id in %s", telegramFile)
	}
	if telegramWrapper.Telegram.Topics.Shopping <= 0 {
		return nil, fmt.Errorf("invalid or non-positive topic id for shopping in %s", telegramFile)
	}
	if telegramWrapper.Telegram.Topics.Chores <= 0 {
		return nil, fmt.Errorf("invalid or non-positive topic id for chores in %s", telegramFile)
	}
	if telegramWrapper.Telegram.Topics.Reminders <= 0 {
		return nil, fmt.Errorf("invalid or non-positive topic id for reminders in %s", telegramFile)
	}

	// 5. Unmarshal app.yaml
	var appWrapper struct {
		App AppConfig `yaml:"app"`
	}
	if err := yaml.Unmarshal(appBytes, &appWrapper); err != nil {
		return nil, fmt.Errorf("error parsing %s: %w", appFile, err)
	}
	app := appWrapper.App
	if app.DefaultLocale == "" {
		app.DefaultLocale = "en"
	}
	if app.DatabasePath == "" {
		app.DatabasePath = "data/homectl.db"
	}
	if app.Calendar.DefaultTimeOfDay == "" {
		app.Calendar.DefaultTimeOfDay = "09:00"
	}

	// Validate default locale against locales dir
	localeFile := filepath.Join(localesDir, app.DefaultLocale+".yaml")
	if _, err := os.Stat(localeFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("unsupported default_locale %q: missing %s", app.DefaultLocale, localeFile)
	}

	// Validate intervals
	if app.Notifications.UnresolvedReminderInterval <= 0 {
		return nil, fmt.Errorf("unresolved_reminder_interval must be positive, got %v", app.Notifications.UnresolvedReminderInterval)
	}
	if app.Flow.TTL <= 0 {
		return nil, fmt.Errorf("flow.ttl must be positive, got %v", app.Flow.TTL)
	}
	if !timeOfDayRegex.MatchString(app.Calendar.DefaultTimeOfDay) {
		return nil, fmt.Errorf("invalid default_time_of_day %q, expected HH:MM", app.Calendar.DefaultTimeOfDay)
	}

	return &Config{
		BotToken: botToken,
		Timezone: tzStr,
		Location: loc,
		Users:    usersWrapper.Users,
		Telegram: telegramWrapper.Telegram,
		App:      app,
	}, nil
}

func readAndCheckYAML(filePath string) ([]byte, error) {
	cleanPath := filepath.Clean(filePath)
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("error reading %s: %w", cleanPath, err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("syntax error in %s: %w", cleanPath, err)
	}

	if err := checkDisallowedKeys(&root, cleanPath); err != nil {
		return nil, err
	}

	return data, nil
}

func checkDisallowedKeys(node *yaml.Node, filePath string) error {
	if node == nil {
		return nil
	}
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			if err := checkDisallowedKeys(child, filePath); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			key := strings.ToLower(keyNode.Value)
			if strings.Contains(key, "token") || key == "secret" || key == "password" {
				return fmt.Errorf("forbidden secret key %q in %s: secrets must come from the environment", keyNode.Value, filePath)
			}
			if key == "timezone" || key == "app_timezone" {
				return fmt.Errorf("forbidden timezone key %q in %s: timezone must come from APP_TIMEZONE environment variable", keyNode.Value, filePath)
			}
			if err := checkDisallowedKeys(node.Content[i+1], filePath); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			if err := checkDisallowedKeys(child, filePath); err != nil {
				return err
			}
		}
	}
	return nil
}
