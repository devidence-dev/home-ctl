package telegram

import (
	"fmt"
	"time"

	"github.com/px1/home-ctl/internal/clock"
)

// eveningTimeOfDay is the explicit time offered by the "Today"/"Tomorrow" evening presets.
const eveningTimeOfDay = "18:00"

// fallbackDefaultTimeOfDay is used only when the router is built without a valid configured value.
const fallbackDefaultTimeOfDay = "09:00"

// timeOfDay is a validated hour and minute.
type timeOfDay struct{ hour, minute int }

func (t timeOfDay) String() string { return fmt.Sprintf("%02d:%02d", t.hour, t.minute) }

func parseTimeOfDay(s string) (timeOfDay, error) {
	h, m, err := clock.ParseTimeOfDay(s)
	return timeOfDay{hour: h, minute: m}, err
}

// evening is the parsed eveningTimeOfDay constant.
var evening = func() timeOfDay {
	t, err := parseTimeOfDay(eveningTimeOfDay)
	if err != nil {
		panic(err) // programmer error: eveningTimeOfDay is a constant
	}
	return t
}()

// dayAt returns the household-timezone calendar day dayOffset days from today at tod.
// It is the single place where "today", "tomorrow" and the default time of day are resolved.
func (r *Router) dayAt(dayOffset int, tod timeOfDay) time.Time {
	loc := r.clock.Location()
	y, m, d := r.clock.Now().In(loc).Date()
	return time.Date(y, m, d+dayOffset, tod.hour, tod.minute, 0, 0, loc)
}

// resolveDuePreset maps a due-date preset button to a concrete time. Date-only choices
// ("tomorrow") use the configured default time of day; the evening presets carry an explicit time.
func (r *Router) resolveDuePreset(preset string) time.Time {
	switch preset {
	case "today_18":
		due := r.dayAt(0, evening)
		if due.Before(r.clock.Now()) {
			due = r.dayAt(1, evening)
		}
		return due
	case "tomorrow_18":
		return r.dayAt(1, evening)
	case "tomorrow":
		return r.dayAt(1, r.defaultTime)
	default:
		return r.clock.Now().Add(24 * time.Hour)
	}
}

// dueDateRows builds the shared due-date preset keyboard for chore and reminder flows.
func (r *Router) dueDateRows(locale string) [][]InlineKeyboardButton {
	return [][]InlineKeyboardButton{
		{
			{Text: r.i18n.T(locale, "buttons.due_today_at", evening), CallbackData: "fl:due:today_18"},
			{Text: r.i18n.T(locale, "buttons.due_tomorrow_at", evening), CallbackData: "fl:due:tomorrow_18"},
		},
		{
			{Text: r.i18n.T(locale, "buttons.due_tomorrow_at", r.defaultTime), CallbackData: "fl:due:tomorrow"},
			{Text: r.i18n.T(locale, "buttons.postpone_custom"), CallbackData: "fl:due:custom"},
		},
		{
			{Text: r.i18n.T(locale, "buttons.cancel"), CallbackData: "fl:c"},
		},
	}
}

// formatDateTime renders t for locale in the household timezone.
func (r *Router) formatDateTime(locale string, t time.Time) string {
	return r.i18n.FormatDateTime(locale, t, r.clock.Now(), r.clock.Location())
}
