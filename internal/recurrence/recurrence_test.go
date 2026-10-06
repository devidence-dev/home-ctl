package recurrence_test

import (
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/recurrence"
)

func TestRecurrence_Validation(t *testing.T) {
	tests := []struct {
		name      string
		rule      recurrence.Rule
		expectErr bool
	}{
		{
			name: "valid every 1 day scheduled",
			rule: recurrence.Rule{Interval: 1, Unit: recurrence.UnitDay, Anchor: recurrence.AnchorScheduled},
		},
		{
			name: "valid every 3 days completion",
			rule: recurrence.Rule{Interval: 3, Unit: recurrence.UnitDay, Anchor: recurrence.AnchorCompletion},
		},
		{
			name: "valid every 1 week scheduled",
			rule: recurrence.Rule{Interval: 1, Unit: recurrence.UnitWeek, Anchor: recurrence.AnchorScheduled},
		},
		{
			name: "valid every 2 weeks completion",
			rule: recurrence.Rule{Interval: 2, Unit: recurrence.UnitWeek, Anchor: recurrence.AnchorCompletion},
		},
		{
			name: "valid every 1 month scheduled",
			rule: recurrence.Rule{Interval: 1, Unit: recurrence.UnitMonth, Anchor: recurrence.AnchorScheduled},
		},
		{
			name:      "zero interval rejected",
			rule:      recurrence.Rule{Interval: 0, Unit: recurrence.UnitDay, Anchor: recurrence.AnchorScheduled},
			expectErr: true,
		},
		{
			name:      "negative interval rejected",
			rule:      recurrence.Rule{Interval: -2, Unit: recurrence.UnitDay, Anchor: recurrence.AnchorScheduled},
			expectErr: true,
		},
		{
			name:      "invalid unit rejected",
			rule:      recurrence.Rule{Interval: 1, Unit: "year", Anchor: recurrence.AnchorScheduled},
			expectErr: true,
		},
		{
			name:      "invalid anchor rejected",
			rule:      recurrence.Rule{Interval: 1, Unit: recurrence.UnitDay, Anchor: "unknown"},
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := recurrence.Validate(tc.rule)
			if tc.expectErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.expectErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestRecurrence_ScheduledAnchor(t *testing.T) {
	loc := time.UTC
	rule := recurrence.Rule{
		Interval: 1,
		Unit:     recurrence.UnitWeek,
		Anchor:   recurrence.AnchorScheduled,
	}

	// Series origin: Monday Jan 5, 2026 10:00 UTC
	origin := time.Date(2026, time.January, 5, 10, 0, 0, 0, loc)
	scheduledAt := origin

	// Scenario 1: Early completion on Saturday Jan 3 (before Jan 5)
	// Next slot must be Monday Jan 12, 2026 10:00
	completedEarly := time.Date(2026, time.January, 3, 14, 0, 0, 0, loc)
	next, err := recurrence.NextSlot(rule, origin, scheduledAt, completedEarly, loc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedJan12 := time.Date(2026, time.January, 12, 10, 0, 0, 0, loc)
	if !next.Equal(expectedJan12) {
		t.Fatalf("early completion: expected %v, got %v", expectedJan12, next)
	}

	// Scenario 2: Late completion on Jan 28, 2026 (Wednesday)
	// Missed Jan 12, 19, 26 -> Next occurrence must be Monday Feb 2, 2026 10:00
	completedLate := time.Date(2026, time.January, 28, 11, 0, 0, 0, loc)
	next, err = recurrence.NextSlot(rule, origin, scheduledAt, completedLate, loc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedFeb2 := time.Date(2026, time.February, 2, 10, 0, 0, 0, loc)
	if !next.Equal(expectedFeb2) {
		t.Fatalf("late completion: expected %v, got %v", expectedFeb2, next)
	}
}

func TestRecurrence_CompletionAnchorAndMonthClamping(t *testing.T) {
	loc := time.UTC
	rule3Months := recurrence.Rule{
		Interval: 3,
		Unit:     recurrence.UnitMonth,
		Anchor:   recurrence.AnchorCompletion,
	}

	// Definition origin (determines definition's time of day, e.g. 10:00)
	origin := time.Date(2026, time.March, 18, 10, 0, 0, 0, loc)
	scheduledAt := origin

	// Scenario: Completed Saturday Mar 14 -> Next is Sunday Jun 14 at 10:00
	completedMar14 := time.Date(2026, time.March, 14, 15, 30, 0, 0, loc)
	next, err := recurrence.NextSlot(rule3Months, origin, scheduledAt, completedMar14, loc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedJun14 := time.Date(2026, time.June, 14, 10, 0, 0, 0, loc)
	if !next.Equal(expectedJun14) {
		t.Fatalf("expected Jun 14, got %v", next)
	}

	// Scenario: Late completion Apr 2 -> Next is Jul 2 at 10:00
	completedApr2 := time.Date(2026, time.April, 2, 8, 0, 0, 0, loc)
	next, err = recurrence.NextSlot(rule3Months, origin, scheduledAt, completedApr2, loc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedJul2 := time.Date(2026, time.July, 2, 10, 0, 0, 0, loc)
	if !next.Equal(expectedJul2) {
		t.Fatalf("expected Jul 2, got %v", next)
	}

	// Scenario: Completed Jan 31 + 3 months -> Apr 30 (clamped)
	completedJan31 := time.Date(2026, time.January, 31, 20, 0, 0, 0, loc)
	next, err = recurrence.NextSlot(rule3Months, origin, scheduledAt, completedJan31, loc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedApr30 := time.Date(2026, time.April, 30, 10, 0, 0, 0, loc)
	if !next.Equal(expectedApr30) {
		t.Fatalf("expected Apr 30, got %v", next)
	}

	// Scenario: Monthly origin Jan 31 with scheduled anchor
	// Slots: Feb 28, Mar 31, Apr 30, May 31 (non-leap year 2026)
	ruleMonthlyScheduled := recurrence.Rule{
		Interval: 1,
		Unit:     recurrence.UnitMonth,
		Anchor:   recurrence.AnchorScheduled,
	}
	originJan31 := time.Date(2026, time.January, 31, 9, 0, 0, 0, loc)

	// After Jan 31 -> Feb 28
	slot1, err := recurrence.NextSlot(ruleMonthlyScheduled, originJan31, originJan31, originJan31, loc)
	if err != nil {
		t.Fatalf("slot1 failed: %v", err)
	}
	expectedFeb28 := time.Date(2026, time.February, 28, 9, 0, 0, 0, loc)
	if !slot1.Equal(expectedFeb28) {
		t.Fatalf("expected Feb 28, got %v", slot1)
	}

	// After Feb 28 -> Mar 31 (re-expands to 31)
	slot2, err := recurrence.NextSlot(ruleMonthlyScheduled, originJan31, slot1, slot1, loc)
	if err != nil {
		t.Fatalf("slot2 failed: %v", err)
	}
	expectedMar31 := time.Date(2026, time.March, 31, 9, 0, 0, 0, loc)
	if !slot2.Equal(expectedMar31) {
		t.Fatalf("expected Mar 31, got %v", slot2)
	}

	// After Mar 31 -> Apr 30
	slot3, err := recurrence.NextSlot(ruleMonthlyScheduled, originJan31, slot2, slot2, loc)
	if err != nil {
		t.Fatalf("slot3 failed: %v", err)
	}
	expectedApr30Sch := time.Date(2026, time.April, 30, 9, 0, 0, 0, loc)
	if !slot3.Equal(expectedApr30Sch) {
		t.Fatalf("expected Apr 30, got %v", slot3)
	}

	// After Apr 30 -> May 31
	slot4, err := recurrence.NextSlot(ruleMonthlyScheduled, originJan31, slot3, slot3, loc)
	if err != nil {
		t.Fatalf("slot4 failed: %v", err)
	}
	expectedMay31 := time.Date(2026, time.May, 31, 9, 0, 0, 0, loc)
	if !slot4.Equal(expectedMay31) {
		t.Fatalf("expected May 31, got %v", slot4)
	}
}

func TestRecurrence_DSTBehavior(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("failed to load America/New_York: %v", err)
	}

	rule := recurrence.Rule{
		Interval: 1,
		Unit:     recurrence.UnitWeek,
		Anchor:   recurrence.AnchorScheduled,
	}

	// In 2026, US DST starts on Sunday March 8, 2026.
	// Slot before DST: Monday March 2, 2026 at 18:00 EST (UTC-5 -> 23:00 UTC)
	origin := time.Date(2026, time.March, 2, 18, 0, 0, 0, loc)
	completedMar2 := origin

	next, err := recurrence.NextSlot(rule, origin, origin, completedMar2, loc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Next slot must be Monday March 9, 2026 at 18:00 EDT (local time remains 18:00)
	nextLocal := next.In(loc)
	if nextLocal.Hour() != 18 || nextLocal.Minute() != 0 {
		t.Fatalf("expected local hour 18:00 across DST, got %02d:%02d", nextLocal.Hour(), nextLocal.Minute())
	}
	if nextLocal.Day() != 9 || nextLocal.Month() != time.March || nextLocal.Year() != 2026 {
		t.Fatalf("expected date 2026-03-09, got %v", nextLocal)
	}

	// In EDT (UTC-4), 18:00 corresponds to 22:00 UTC
	expectedUTC := time.Date(2026, time.March, 9, 22, 0, 0, 0, time.UTC)
	if !next.Equal(expectedUTC) {
		t.Fatalf("expected UTC %v, got %v", expectedUTC, next)
	}
}

func TestRecurrence_DailyCompletion(t *testing.T) {
	loc := time.UTC
	rule := recurrence.Rule{
		Interval: 2,
		Unit:     recurrence.UnitDay,
		Anchor:   recurrence.AnchorCompletion,
	}
	origin := time.Date(2026, time.January, 1, 8, 0, 0, 0, loc)
	completed := time.Date(2026, time.January, 5, 23, 0, 0, 0, loc)

	next, err := recurrence.NextSlot(rule, origin, origin, completed, loc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := time.Date(2026, time.January, 7, 8, 0, 0, 0, loc)
	if !next.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, next)
	}
}
