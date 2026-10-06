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

	defLocal := seriesOrigin.In(loc)
	defY, defM, defD := defLocal.Date()
	defH, defMin, defSec := defLocal.Clock()

	switch rule.Anchor {
	case AnchorScheduled:
		// Step k >= 1 along original series origin until slot is > scheduledAt AND > completedAt
		k := 1
		for {
			slotLocal := computeScheduledSlot(defY, defM, defD, defH, defMin, defSec, rule, k, loc)
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
		compLocal := completedAt.In(loc)
		compY, compM, compD := compLocal.Date()

		var nextLocal time.Time
		switch rule.Unit {
		case UnitDay:
			nextLocal = time.Date(compY, compM, compD+rule.Interval, defH, defMin, defSec, 0, loc)
		case UnitWeek:
			nextLocal = time.Date(compY, compM, compD+rule.Interval*7, defH, defMin, defSec, 0, loc)
		case UnitMonth:
			nextLocal = addMonthsClamped(compY, compM, compD, rule.Interval, defH, defMin, defSec, loc)
		}
		return nextLocal.UTC(), nil

	default:
		return time.Time{}, fmt.Errorf("unknown anchor %q", rule.Anchor)
	}
}

func computeScheduledSlot(y0 int, m0 time.Month, d0, h0, min0, sec0 int, rule Rule, k int, loc *time.Location) time.Time {
	switch rule.Unit {
	case UnitDay:
		return time.Date(y0, m0, d0+k*rule.Interval, h0, min0, sec0, 0, loc)
	case UnitWeek:
		return time.Date(y0, m0, d0+k*rule.Interval*7, h0, min0, sec0, 0, loc)
	case UnitMonth:
		return addMonthsClamped(y0, m0, d0, k*rule.Interval, h0, min0, sec0, loc)
	default:
		return time.Time{}
	}
}

func addMonthsClamped(y0 int, m0 time.Month, d0 int, monthsToAdd int, h, minute, sec int, loc *time.Location) time.Time {
	totalMonths := int(m0) - 1 + monthsToAdd
	targetYear := y0 + totalMonths/12
	targetMonth := (totalMonths % 12) + 1
	if targetMonth <= 0 {
		targetMonth += 12
		targetYear--
	}

	m := time.Month(targetMonth)
	maxDays := daysInMonth(targetYear, m)
	day := d0
	if day > maxDays {
		day = maxDays
	}
	return time.Date(targetYear, m, day, h, minute, sec, 0, loc)
}

func daysInMonth(year int, m time.Month) int {
	return time.Date(year, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
