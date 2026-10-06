// Package clock provides injectable time, timezone handling, and calendar helpers
// for household scheduling.
package clock

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Clock defines time retrieval and localized date calculation methods.
type Clock interface {
	Now() time.Time
	Location() *time.Location
	Today(timeOfDay string) (time.Time, error)
	Tomorrow(timeOfDay string) (time.Time, error)
	ParseDateTime(input, defaultTimeOfDay string) (time.Time, error)
}

// RealClock implements Clock using the system clock and a configured timezone location.
type RealClock struct {
	loc *time.Location
}

// FixedClock implements Clock for testing with a deterministic timestamp and location.
type FixedClock struct {
	now time.Time
	loc *time.Location
}

// New creates a RealClock by resolving the IANA timezone name.
func New(tzName string) (*RealClock, error) {
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone %q: %w", tzName, err)
	}
	return &RealClock{loc: loc}, nil
}

// NewWithLocation creates a RealClock with an existing time.Location.
func NewWithLocation(loc *time.Location) *RealClock {
	if loc == nil {
		loc = time.UTC
	}
	return &RealClock{loc: loc}
}

// NewFixed creates a FixedClock with a fixed timestamp and location.
func NewFixed(now time.Time, loc *time.Location) *FixedClock {
	if loc == nil {
		loc = time.UTC
	}
	return &FixedClock{
		now: now,
		loc: loc,
	}
}

// SetNow updates the fixed clock's current time.
func (f *FixedClock) SetNow(now time.Time) {
	f.now = now
}

// Now returns the current time.
func (r *RealClock) Now() time.Time {
	return time.Now().In(r.loc)
}

// Location returns the configured location.
func (r *RealClock) Location() *time.Location {
	return r.loc
}

// Today returns today's date at the specified timeOfDay ("HH:MM") in the clock's location.
func (r *RealClock) Today(timeOfDay string) (time.Time, error) {
	return computeRelativeDay(r.Now(), r.loc, 0, timeOfDay)
}

// Tomorrow returns tomorrow's date at the specified timeOfDay ("HH:MM") in the clock's location.
func (r *RealClock) Tomorrow(timeOfDay string) (time.Time, error) {
	return computeRelativeDay(r.Now(), r.loc, 1, timeOfDay)
}

// ParseDateTime parses formatted date/time strings into a UTC time.Time based on the clock's location.
// Supported formats:
// - "YYYY-MM-DD HH:MM"
// - "YYYY-MM-DD" (uses defaultTimeOfDay)
// - "HH:MM" (uses today's date)
func (r *RealClock) ParseDateTime(input, defaultTimeOfDay string) (time.Time, error) {
	return parseDateTimeInLocation(input, defaultTimeOfDay, r.Now(), r.loc)
}

// Now returns the fixed time.
func (f *FixedClock) Now() time.Time {
	return f.now.In(f.loc)
}

// Location returns the configured location.
func (f *FixedClock) Location() *time.Location {
	return f.loc
}

// Today returns today's date at the specified timeOfDay ("HH:MM") in the clock's location.
func (f *FixedClock) Today(timeOfDay string) (time.Time, error) {
	return computeRelativeDay(f.Now(), f.loc, 0, timeOfDay)
}

// Tomorrow returns tomorrow's date at the specified timeOfDay ("HH:MM") in the clock's location.
func (f *FixedClock) Tomorrow(timeOfDay string) (time.Time, error) {
	return computeRelativeDay(f.Now(), f.loc, 1, timeOfDay)
}

// ParseDateTime parses formatted date/time strings into a UTC time.Time based on the fixed clock's location.
func (f *FixedClock) ParseDateTime(input, defaultTimeOfDay string) (time.Time, error) {
	return parseDateTimeInLocation(input, defaultTimeOfDay, f.Now(), f.loc)
}

// ParseTimeOfDay parses a "HH:MM" time of day into hour and minute.
func ParseTimeOfDay(tod string) (int, int, error) {
	parts := strings.Split(tod, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid time of day format %q, expected HH:MM", tod)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 || h > 23 {
		return 0, 0, fmt.Errorf("invalid hour in time of day %q", tod)
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("invalid minute in time of day %q", tod)
	}
	return h, m, nil
}

func computeRelativeDay(refTime time.Time, loc *time.Location, dayOffset int, timeOfDay string) (time.Time, error) {
	h, m, err := ParseTimeOfDay(timeOfDay)
	if err != nil {
		return time.Time{}, err
	}
	localTime := refTime.In(loc)
	y, month, d := localTime.Date()
	resolved := time.Date(y, month, d+dayOffset, h, m, 0, 0, loc)
	return resolved.UTC(), nil
}

func parseDateTimeInLocation(input, defaultTimeOfDay string, refTime time.Time, loc *time.Location) (time.Time, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("empty date/time string")
	}

	// 1. "YYYY-MM-DD HH:MM"
	if t, err := time.ParseInLocation("2006-01-02 15:04", trimmed, loc); err == nil {
		return t.UTC(), nil
	}

	// 2. "YYYY-MM-DD"
	if _, err := time.ParseInLocation("2006-01-02", trimmed, loc); err == nil {
		if defaultTimeOfDay == "" {
			defaultTimeOfDay = "09:00"
		}
		h, m, err := ParseTimeOfDay(defaultTimeOfDay)
		if err != nil {
			return time.Time{}, err
		}
		parts := strings.Split(trimmed, "-")
		y, _ := strconv.Atoi(parts[0])
		monthVal, _ := strconv.Atoi(parts[1])
		d, _ := strconv.Atoi(parts[2])
		return time.Date(y, time.Month(monthVal), d, h, m, 0, 0, loc).UTC(), nil
	}

	// 3. "HH:MM"
	if h, m, err := ParseTimeOfDay(trimmed); err == nil {
		localRef := refTime.In(loc)
		y, month, d := localRef.Date()
		return time.Date(y, month, d, h, m, 0, 0, loc).UTC(), nil
	}

	return time.Time{}, fmt.Errorf("cannot parse %q as date/time, expected YYYY-MM-DD HH:MM or HH:MM", input)
}
