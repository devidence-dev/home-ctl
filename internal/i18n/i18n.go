// Package i18n provides localized message formatting, fallback to English,
// and startup validation of required message keys.
package i18n

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
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
	"datetime.yesterday",
	"datetime.day",
	"datetime.day_year",
	"datetime.day_today",
	"datetime.day_tomorrow",
	"datetime.day_yesterday",
	"telegram.commands.add.description",
	"telegram.commands.list.description",
	"telegram.commands.private_list.description",
	"lists.group_overdue",
	"lists.group_today",
	"lists.group_tomorrow",
	"lists.group_upcoming",
	"lists.entry_detail",
	"lists.entry_recurrence",
	"lists.view_chores",
	"lists.view_reminders",
	"telegram.commands.status.description",
	"telegram.commands.help.description",
	"telegram.commands.start.description",
	"picker.prompt_recurrence",
	"picker.prompt_interval_days",
	"picker.prompt_interval_weeks",
	"picker.prompt_interval_months",
	"picker.prompt_date",
	"picker.prompt_hour",
	"picker.prompt_minute",
	"picker.prompt_anchor",
	"picker.prompt_confirm",
	"picker.rec_once",
	"picker.rec_daily",
	"picker.rec_weekly",
	"picker.rec_monthly",
	"picker.btn_today",
	"picker.btn_tomorrow",
	"picker.btn_type_date",
	"picker.btn_other",
	"picker.anchor_scheduled",
	"picker.anchor_completion",
	"picker.summary_assignee",
	"picker.summary_target",
	"picker.summary_due",
	"picker.summary_recurrence",
	"picker.summary_once",
	"picker.summary_anchor_scheduled",
	"picker.summary_anchor_completion",
	"picker.btn_change_date",
	"picker.time_passed",
	"general.recurrence_every_day_one",
	"general.recurrence_every_day_other",
	"general.recurrence_every_week_one",
	"general.recurrence_every_week_other",
	"general.recurrence_every_month_one",
	"general.recurrence_every_month_other",
	"picker.ownership_notice",
	"picker.invalid_interval",
	"picker.invalid_minute",
	"picker.prompt_type_date",
	"picker.prompt_type_interval",
	"picker.prompt_type_minute",
	"picker.prompt_emoji",
	"picker.prompt_type_emoji",
	"picker.invalid_emoji",
	"picker.btn_skip",
	"picker.emoji_suggestions_chores",
	"picker.emoji_suggestions_reminders",
	"picker.months",
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

// Locales returns the sorted list of loaded locale identifiers.
func (b *Bundle) Locales() []string {
	names := make([]string, 0, len(b.locales))
	for name := range b.locales {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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
// (relative to now) render as "today at 18:00", and yesterday as "yesterday at 18:00"; other days use the locale's weekday and month
// names, with the year added only when it differs from now's. Names and layouts come from the
// locale files, so persisted timestamps and domain logic stay locale-independent.
func (b *Bundle) FormatDateTime(locale string, t, now time.Time, loc *time.Location) string {
	lt := t.In(loc)
	hhmm := fmt.Sprintf("%02d:%02d", lt.Hour(), lt.Minute())

	if offset, ok := relativeDay(t, now, loc); ok {
		switch offset {
		case -1:
			return b.T(locale, "datetime.yesterday", hhmm)
		case 0:
			return b.T(locale, "datetime.today", hhmm)
		default:
			return b.T(locale, "datetime.tomorrow", hhmm)
		}
	}

	weekday, month := b.weekdayAndMonth(locale, lt)
	if lt.Year() != now.In(loc).Year() {
		return b.T(locale, "datetime.full_year", weekday, lt.Day(), month, lt.Hour(), lt.Minute(), lt.Year())
	}
	return b.T(locale, "datetime.full", weekday, lt.Day(), month, lt.Hour(), lt.Minute())
}

// FormatDay renders only the calendar day of t, without a time, for users of locale in loc: "yesterday",
// "today" or "tomorrow" relative to now, otherwise the weekday, day and month, with the year when it
// differs from now's. Used as the day part of a list line such as "ayer · 18:00".
func (b *Bundle) FormatDay(locale string, t, now time.Time, loc *time.Location) string {
	if offset, ok := relativeDay(t, now, loc); ok {
		switch offset {
		case -1:
			return b.T(locale, "datetime.day_yesterday")
		case 0:
			return b.T(locale, "datetime.day_today")
		default:
			return b.T(locale, "datetime.day_tomorrow")
		}
	}

	lt := t.In(loc)
	weekday, month := b.weekdayAndMonth(locale, lt)
	if lt.Year() != now.In(loc).Year() {
		return b.T(locale, "datetime.day_year", weekday, lt.Day(), month, lt.Year())
	}
	return b.T(locale, "datetime.day", weekday, lt.Day(), month)
}

// relativeDay returns -1, 0 or 1 when t's calendar day in loc is yesterday, today or tomorrow relative
// to now, and false otherwise.
func relativeDay(t, now time.Time, loc *time.Location) (int, bool) {
	ty, tm, td := t.In(loc).Date()
	ny, nm, nd := now.In(loc).Date()
	today := time.Date(ny, nm, nd, 0, 0, 0, 0, loc)
	day := time.Date(ty, tm, td, 0, 0, 0, 0, loc)
	for offset := -1; offset <= 1; offset++ {
		if day.Equal(today.AddDate(0, 0, offset)) {
			return offset, true
		}
	}
	return 0, false
}

// weekdayAndMonth returns the locale's abbreviated weekday and month names for lt.
func (b *Bundle) weekdayAndMonth(locale string, lt time.Time) (string, string) {
	weekday := nameAt(b.T(locale, "datetime.weekdays"), int(lt.Weekday()), 7, lt.Weekday().String()[:3])
	month := nameAt(b.T(locale, "datetime.months"), int(lt.Month())-1, 12, lt.Month().String()[:3])
	return weekday, month
}

// nameAt returns the idx-th comma-separated name, or fallback when the list is malformed.
func nameAt(list string, idx, want int, fallback string) string {
	names := strings.Split(list, ",")
	if len(names) != want || idx < 0 || idx >= want {
		return fallback
	}
	return strings.TrimSpace(names[idx])
}
