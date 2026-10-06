package clock_test

import (
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/clock"
)

func TestClock_TomorrowNearMidnight(t *testing.T) {
	loc, err := time.LoadLocation("America/Guayaquil")
	if err != nil {
		t.Fatalf("failed to load America/Guayaquil location: %v", err)
	}

	// At 23:30 America/Guayaquil on Oct 6, 2026 (04:30 UTC on Oct 7, 2026)
	localNearMidnight := time.Date(2026, time.October, 6, 23, 30, 0, 0, loc)
	fc := clock.NewFixed(localNearMidnight, loc)

	tomorrow, err := fc.Tomorrow("09:00")
	if err != nil {
		t.Fatalf("unexpected error computing tomorrow: %v", err)
	}

	tomorrowLocal := tomorrow.In(loc)
	y, m, d := tomorrowLocal.Date()
	if y != 2026 || m != time.October || d != 7 {
		t.Fatalf("expected resolved local date to be 2026-10-07, got %d-%02d-%02d", y, m, d)
	}
	if tomorrowLocal.Hour() != 9 || tomorrowLocal.Minute() != 0 {
		t.Fatalf("expected 09:00 local time, got %02d:%02d", tomorrowLocal.Hour(), tomorrowLocal.Minute())
	}

	// Confirm UTC representation corresponds to 14:00 UTC on Oct 7 (UTC-5)
	expectedUTC := time.Date(2026, time.October, 7, 14, 0, 0, 0, time.UTC)
	if !tomorrow.Equal(expectedUTC) {
		t.Fatalf("expected %v UTC, got %v", expectedUTC, tomorrow)
	}
}

func TestClock_InvalidTimezoneFailure(t *testing.T) {
	_, err := clock.New("Mars/Olympus")
	if err == nil {
		t.Fatalf("expected error for invalid timezone Mars/Olympus, got nil")
	}
}

func TestClock_Today(t *testing.T) {
	loc, err := time.LoadLocation("America/Guayaquil")
	if err != nil {
		t.Fatalf("failed to load location: %v", err)
	}
	refTime := time.Date(2026, time.October, 6, 12, 0, 0, 0, loc)
	fc := clock.NewFixed(refTime, loc)

	today, err := fc.Today("18:30")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	todayLocal := today.In(loc)
	if todayLocal.Day() != 6 || todayLocal.Hour() != 18 || todayLocal.Minute() != 30 {
		t.Fatalf("unexpected today result: %v", todayLocal)
	}
}

func TestClock_ParseDateTime(t *testing.T) {
	loc := time.FixedZone("EST", -5*3600)
	refTime := time.Date(2026, time.October, 6, 12, 0, 0, 0, loc)
	fc := clock.NewFixed(refTime, loc)

	tests := []struct {
		name        string
		input       string
		defaultTod  string
		expectedUTC time.Time
		expectErr   bool
	}{
		{
			name:        "full date and time",
			input:       "2026-10-10 10:00",
			expectedUTC: time.Date(2026, time.October, 10, 15, 0, 0, 0, time.UTC),
		},
		{
			name:        "date only uses defaultTod",
			input:       "2026-10-10",
			defaultTod:  "09:00",
			expectedUTC: time.Date(2026, time.October, 10, 14, 0, 0, 0, time.UTC),
		},
		{
			name:        "time only uses today date",
			input:       "15:30",
			expectedUTC: time.Date(2026, time.October, 6, 20, 30, 0, 0, time.UTC),
		},
		{
			name:      "invalid string",
			input:     "not-a-date",
			expectErr: true,
		},
		{
			name:      "empty string",
			input:     "",
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := fc.ParseDateTime(tc.input, tc.defaultTod)
			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !res.Equal(tc.expectedUTC) {
					t.Fatalf("expected %v, got %v", tc.expectedUTC, res)
				}
			}
		})
	}
}

func TestRealClock(t *testing.T) {
	rc, err := clock.New("UTC")
	if err != nil {
		t.Fatalf("failed to create real clock: %v", err)
	}
	if rc.Location() != time.UTC {
		t.Fatalf("expected UTC location")
	}
	now := rc.Now()
	if time.Since(now) > time.Minute {
		t.Fatalf("now is too old: %v", now)
	}

	rcDefault := clock.NewWithLocation(nil)
	if rcDefault.Location() != time.UTC {
		t.Fatalf("expected UTC location for nil")
	}

	today, err := rc.Today("10:00")
	if err != nil {
		t.Fatalf("today failed: %v", err)
	}
	if today.IsZero() {
		t.Fatalf("today was zero")
	}

	tomorrow, err := rc.Tomorrow("10:00")
	if err != nil {
		t.Fatalf("tomorrow failed: %v", err)
	}
	if tomorrow.IsZero() {
		t.Fatalf("tomorrow was zero")
	}

	parsed, err := rc.ParseDateTime("2026-12-01 12:00", "09:00")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if parsed.IsZero() {
		t.Fatalf("parsed was zero")
	}
}
