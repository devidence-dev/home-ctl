package telegram

import (
	"fmt"
	"time"

	"github.com/px1/home-ctl/internal/clock"
)

// fallbackDefaultTimeOfDay is used only when the router is built without a valid configured value.
const fallbackDefaultTimeOfDay = "09:00"

// timeOfDay is a validated hour and minute.
type timeOfDay struct{ hour, minute int }

func (t timeOfDay) String() string { return fmt.Sprintf("%02d:%02d", t.hour, t.minute) }

func parseTimeOfDay(s string) (timeOfDay, error) {
	h, m, err := clock.ParseTimeOfDay(s)
	return timeOfDay{hour: h, minute: m}, err
}

// dayAt returns the household-timezone calendar day dayOffset days from today at tod.
// It is the single place where "today", "tomorrow" and the default time of day are resolved.
func (r *Router) dayAt(dayOffset int, tod timeOfDay) time.Time {
	loc := r.clock.Location()
	y, m, d := r.clock.Now().In(loc).Date()
	return time.Date(y, m, d+dayOffset, tod.hour, tod.minute, 0, 0, loc)
}

// formatDateTime renders t for locale in the household timezone.
func (r *Router) formatDateTime(locale string, t time.Time) string {
	return r.i18n.FormatDateTime(locale, t, r.clock.Now(), r.clock.Location())
}
