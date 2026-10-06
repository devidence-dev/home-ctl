// Package i18n provides localized message formatting, fallback to English,
// and startup validation of required message keys.
package i18n

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultRequiredKeys contains the core message keys required for the bot to run.
var DefaultRequiredKeys = []string{
	"buttons.done",
	"buttons.postpone",
	"buttons.back",
	"buttons.cancel",
	"buttons.confirm",
	"notifications.chore_due",
	"notifications.reminder_due",
	"group.chore_completed",
	"group.chore_postponed",
	"group.reminder_completed",
	"group.reminder_postponed",
	"group.shopping_purchased",
	"general.not_member",
	"general.flow_expired",
	"general.unmapped_topic",
	"status.connected",
	"status.not_connected",
	"datetime.weekdays",
	"datetime.months",
	"datetime.full",
	"datetime.full_year",
	"datetime.today",
	"datetime.tomorrow",
}

// Bundle holds loaded translations keyed by locale and dot-notated string paths.
type Bundle struct {
	locales       map[string]map[string]string
	defaultLocale string
	logger        *slog.Logger
}

// NewBundle loads all YAML locale files from localesDir, validates required keys against en.yaml,
// and ensures the default locale exists.
func NewBundle(localesDir, defaultLocale string, requiredKeys []string, logger *slog.Logger) (*Bundle, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if defaultLocale == "" {
		defaultLocale = "en"
	}

	entries, err := os.ReadDir(localesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read locales directory %s: %w", localesDir, err)
	}

	locales := make(map[string]map[string]string)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		localeName := strings.TrimSuffix(entry.Name(), ".yaml")
		filePath := filepath.Join(localesDir, entry.Name())
		data, err := os.ReadFile(filepath.Clean(filePath))
		if err != nil {
			return nil, fmt.Errorf("failed to read locale file %s: %w", filePath, err)
		}

		var raw map[string]interface{}
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("syntax error in locale file %s: %w", filePath, err)
		}

		flat := make(map[string]string)
		flattenMap("", raw, flat)
		locales[localeName] = flat
	}

	enMap, ok := locales["en"]
	if !ok {
		return nil, fmt.Errorf("missing base locale 'en.yaml' in %s", localesDir)
	}

	// Startup completeness check: ensure every required key is present in en.yaml
	for _, reqKey := range requiredKeys {
		if _, exists := enMap[reqKey]; !exists {
			return nil, fmt.Errorf("missing required locale key %q in en.yaml", reqKey)
		}
	}

	// Verify defaultLocale is available
	if _, ok := locales[defaultLocale]; !ok {
		return nil, fmt.Errorf("unsupported default_locale %q: missing %s.yaml", defaultLocale, defaultLocale)
	}

	return &Bundle{
		locales:       locales,
		defaultLocale: defaultLocale,
		logger:        logger,
	}, nil
}

// T translates a key into the specified locale with fallback to defaultLocale and English.
func (b *Bundle) T(locale, key string, args ...any) string {
	if locale == "" {
		locale = b.defaultLocale
	}

	template := b.lookup(locale, key)
	if len(args) > 0 {
		return fmt.Sprintf(template, args...)
	}
	return template
}

func (b *Bundle) lookup(locale, key string) string {
	// 1. Try specified locale
	if m, ok := b.locales[locale]; ok {
		if val, exists := m[key]; exists {
			return val
		}
	}

	// Key missing in specified locale: log and fallback
	if locale != "en" {
		b.logger.Warn("missing translation key", "locale", locale, "key", key)
	}

	// 2. Try defaultLocale if different
	if locale != b.defaultLocale {
		if m, ok := b.locales[b.defaultLocale]; ok {
			if val, exists := m[key]; exists {
				return val
			}
		}
	}

	// 3. Fallback to English
	if enMap, ok := b.locales["en"]; ok {
		if val, exists := enMap[key]; exists {
			return val
		}
	}

	// 4. Return raw key if completely untranslated
	return key
}

func flattenMap(prefix string, m map[string]interface{}, out map[string]string) {
	for k, v := range m {
		var path string
		if prefix == "" {
			path = k
		} else {
			path = prefix + "." + k
		}

		switch val := v.(type) {
		case map[string]interface{}:
			flattenMap(path, val, out)
		case string:
			out[path] = val
		default:
			out[path] = fmt.Sprint(val)
		}
	}
}

// FormatDateTime renders t for users of locale in the household timezone loc. Today and tomorrow
// (relative to now) render as "today at 18:00"; other days use the locale's weekday and month
// names, with the year added only when it differs from now's. Names and layouts come from the
// locale files, so persisted timestamps and domain logic stay locale-independent.
func (b *Bundle) FormatDateTime(locale string, t, now time.Time, loc *time.Location) string {
	lt, ln := t.In(loc), now.In(loc)
	hhmm := fmt.Sprintf("%02d:%02d", lt.Hour(), lt.Minute())

	ty, tm, td := lt.Date()
	ny, nm, nd := ln.Date()
	today := time.Date(ny, nm, nd, 0, 0, 0, 0, loc)
	switch day := time.Date(ty, tm, td, 0, 0, 0, 0, loc); {
	case day.Equal(today):
		return b.T(locale, "datetime.today", hhmm)
	case day.Equal(today.AddDate(0, 0, 1)):
		return b.T(locale, "datetime.tomorrow", hhmm)
	}

	weekday := nameAt(b.T(locale, "datetime.weekdays"), int(lt.Weekday()), 7, lt.Weekday().String()[:3])
	month := nameAt(b.T(locale, "datetime.months"), int(lt.Month())-1, 12, lt.Month().String()[:3])
	if ty != ny {
		return b.T(locale, "datetime.full_year", weekday, td, month, lt.Hour(), lt.Minute(), ty)
	}
	return b.T(locale, "datetime.full", weekday, td, month, lt.Hour(), lt.Minute())
}

// nameAt returns the idx-th comma-separated name, or fallback when the list is malformed.
func nameAt(list string, idx, want int, fallback string) string {
	names := strings.Split(list, ",")
	if len(names) != want || idx < 0 || idx >= want {
		return fallback
	}
	return strings.TrimSpace(names[idx])
}
