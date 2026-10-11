// Package recurrence provides pure calculation rules, anchors, and slot generation
// for recurring household tasks and reminders.
package recurrence

import (
	"fmt"
	"time"
)

// Unit represents the cadence unit of a recurrence rule.
type Unit string

const (
	// UnitDay indicates daily recurrence.
	UnitDay Unit = "day"
	// UnitWeek indicates weekly recurrence.
	UnitWeek Unit = "week"
	// UnitMonth indicates monthly recurrence.
	UnitMonth Unit = "month"
)

// Anchor defines how subsequent occurrence dates are anchored.
type Anchor string

const (
	// AnchorScheduled calculates the next slot along the fixed nominal schedule.
	AnchorScheduled Anchor = "scheduled"
	// AnchorCompletion calculates the next slot relative to the completion timestamp.
	AnchorCompletion Anchor = "completion"
)

// Rule defines the recurrence schedule properties.
type Rule struct {
	Interval int    `json:"interval"`
	Unit     Unit   `json:"unit"`
	Anchor   Anchor `json:"anchor"`
}

// Validate verifies that a recurrence rule has a positive interval, a recognized unit,
// and a valid anchor.
func Validate(r Rule) error {
	if r.Interval <= 0 {
		return fmt.Errorf("recurrence interval must be a positive integer, got %d", r.Interval)
	}
	switch r.Unit {
	case UnitDay, UnitWeek, UnitMonth:
	default:
		return fmt.Errorf("invalid recurrence unit %q, expected 'day', 'week', or 'month'", r.Unit)
	}
	switch r.Anchor {
	case AnchorScheduled, AnchorCompletion:
	default:
		return fmt.Errorf("invalid recurrence anchor %q, expected 'scheduled' or 'completion'", r.Anchor)
	}
	return nil
}

// NextSlot computes the next scheduled slot in UTC according to the recurrence rule,
// series origin, occurrence scheduled_at and completed_at, evaluated in the given location.
func NextSlot(rule Rule, seriesOrigin time.Time, scheduledAt, completedAt time.Time, loc *time.Location) (time.Time, error) {
	if err := Validate(rule); err != nil {
		return time.Time{}, err
	}
	if loc == nil {
		loc = time.UTC
	}

	origin := wallTimeOf(seriesOrigin.In(loc))

	switch rule.Anchor {
	case AnchorScheduled:
		// Step k >= 1 along original series origin until slot is > scheduledAt AND > completedAt
		k := 1
		for {
			slotLocal := computeScheduledSlot(origin, rule, k, loc)
			slotUTC := slotLocal.UTC()
			if slotUTC.After(scheduledAt) && slotUTC.After(completedAt) {
				return slotUTC, nil
			}
			k++
			if k > 10000 {
				return time.Time{}, fmt.Errorf("exceeded maximum iterations searching for next recurrence slot")
			}
		}

	case AnchorCompletion:
		// Completion date, keeping the series' nominal time of day
		compY, compM, compD := completedAt.In(loc).Date()
		base := wallTime{year: compY, month: compM, day: compD, hour: origin.hour, minute: origin.minute, sec: origin.sec}

		var nextLocal time.Time
		switch rule.Unit {
		case UnitDay:
			nextLocal = time.Date(base.year, base.month, base.day+rule.Interval, base.hour, base.minute, base.sec, 0, loc)
		case UnitWeek:
			nextLocal = time.Date(base.year, base.month, base.day+rule.Interval*7, base.hour, base.minute, base.sec, 0, loc)
		case UnitMonth:
			nextLocal = addMonthsClamped(base, rule.Interval, loc)
		}
		return nextLocal.UTC(), nil

	default:
		return time.Time{}, fmt.Errorf("unknown anchor %q", rule.Anchor)
	}
}

// wallTime is a calendar date and time of day, not yet placed in a location.
type wallTime struct {
	year   int
	month  time.Month
	day    int
	hour   int
	minute int
	sec    int
}

// wallTimeOf returns the calendar date and time of day of t in t's own location.
func wallTimeOf(t time.Time) wallTime {
	y, m, d := t.Date()
	h, minute, sec := t.Clock()
	return wallTime{year: y, month: m, day: d, hour: h, minute: minute, sec: sec}
}

func computeScheduledSlot(origin wallTime, rule Rule, k int, loc *time.Location) time.Time {
	switch rule.Unit {
	case UnitDay:
		return time.Date(origin.year, origin.month, origin.day+k*rule.Interval, origin.hour, origin.minute, origin.sec, 0, loc)
	case UnitWeek:
		return time.Date(origin.year, origin.month, origin.day+k*rule.Interval*7, origin.hour, origin.minute, origin.sec, 0, loc)
	case UnitMonth:
		return addMonthsClamped(origin, k*rule.Interval, loc)
	default:
		return time.Time{}
	}
}

func addMonthsClamped(base wallTime, monthsToAdd int, loc *time.Location) time.Time {
	totalMonths := int(base.month) - 1 + monthsToAdd
	targetYear := base.year + totalMonths/12
	targetMonth := (totalMonths % 12) + 1
	if targetMonth <= 0 {
		targetMonth += 12
		targetYear--
	}

	m := time.Month(targetMonth)
	maxDays := daysInMonth(targetYear, m)
	day := base.day
	if day > maxDays {
		day = maxDays
	}
	return time.Date(targetYear, m, day, base.hour, base.minute, base.sec, 0, loc)
}

func daysInMonth(year int, m time.Month) int {
	return time.Date(year, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
