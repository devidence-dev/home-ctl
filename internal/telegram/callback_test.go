package telegram_test

import (
	"errors"
	"testing"

	"github.com/px1/home-ctl/internal/telegram"
)

func TestParseCallbackData(t *testing.T) {
	tests := []struct {
		name       string
		data       string
		actorID    int64
		wantKind   telegram.CallbackKind
		wantTarget int64
		wantPay    string
		wantErr    bool
	}{
		{"empty", "", 101, "", 0, "", true},
		{"too long", string(make([]byte, 65)), 101, "", 0, "", true},
		{"list chores", "list:ch", 101, telegram.CallbackListChores, 0, "", false},
		{"list reminders", "list:re", 101, telegram.CallbackListReminders, 0, "", false},
		{"flow cancel", "fl:c", 101, telegram.CallbackFlowCancel, 0, "", false},
		{"flow confirm", "fl:ok", 101, telegram.CallbackFlowConfirm, 0, "", false},
		{"flow new item", "fl:new", 101, telegram.CallbackFlowNewItem, 0, "", false},
		{"shopping purchase", "sh:p:42", 101, telegram.CallbackShoppingPurchase, 42, "", false},
		{"shopping purchase invalid", "sh:p:abc", 101, "", 0, "", true},
		{"chore complete", "ch:c:10", 101, telegram.CallbackChoreComplete, 10, "", false},
		{"chore detail", "ch:d:10", 101, telegram.CallbackChoreDetail, 10, "", false},
		{"chore postpone menu", "ch:p:10", 101, telegram.CallbackChorePostpone, 10, "", false},
		{"chore postpone preset", "ch:p:10:3h", 101, telegram.CallbackChorePostpone, 10, "3h", false},
		{"chore postpone invalid", "ch:p:abc", 101, "", 0, "", true},
		{"reminder complete", "re:c:20", 101, telegram.CallbackReminderComplete, 20, "", false},
		{"reminder detail", "re:d:20", 101, telegram.CallbackReminderDetail, 20, "", false},
		{"reminder postpone menu", "re:p:20", 101, telegram.CallbackReminderPostpone, 20, "", false},
		{"reminder postpone preset", "re:p:20:1h", 101, telegram.CallbackReminderPostpone, 20, "1h", false},
		{"reminder postpone invalid", "re:p:abc", 101, "", 0, "", true},
		{"flow select", "fl:sel:5", 101, telegram.CallbackFlowSelect, 5, "", false},
		{"flow user", "fl:usr:102", 101, telegram.CallbackFlowUser, 102, "", false},
		{"flow due", "fl:due:tomorrow_18", 101, telegram.CallbackFlowDue, 0, "tomorrow_18", false},
		{"flow recurrence", "fl:rec:weekly", 101, telegram.CallbackFlowRecurrence, 0, "weekly", false},
		{"flow anchor", "fl:anc:completion", 101, telegram.CallbackFlowAnchor, 0, "completion", false},
		{"flow noop", "fl:noop", 101, telegram.CallbackFlowNoop, 0, "", false},
		{"flow noop extra", "fl:noop:extra", 101, "", 0, "", true},
		{"flow recurrence type daily", "fl:rt:daily", 101, telegram.CallbackFlowRecurrenceType, 0, "daily", false},
		{"flow recurrence type weekly", "fl:rt:weekly", 101, telegram.CallbackFlowRecurrenceType, 0, "weekly", false},
		{"flow recurrence type monthly", "fl:rt:monthly", 101, telegram.CallbackFlowRecurrenceType, 0, "monthly", false},
		{"flow recurrence type once", "fl:rt:once", 101, telegram.CallbackFlowRecurrenceType, 0, "once", false},
		{"flow recurrence type invalid", "fl:rt:yearly", 101, "", 0, "", true},
		{"flow recurrence type missing", "fl:rt", 101, "", 0, "", true},
		{"flow interval 1", "fl:iv:1", 101, telegram.CallbackFlowInterval, 1, "1", false},
		{"flow interval 12", "fl:iv:12", 101, telegram.CallbackFlowInterval, 12, "12", false},
		{"flow interval 99", "fl:iv:99", 101, telegram.CallbackFlowInterval, 99, "99", false},
		{"flow interval other", "fl:iv:o", 101, telegram.CallbackFlowInterval, 0, "o", false},
		{"flow interval zero", "fl:iv:0", 101, "", 0, "", true},
		{"flow interval too high", "fl:iv:100", 101, "", 0, "", true},
		{"flow interval non-numeric", "fl:iv:abc", 101, "", 0, "", true},
		{"flow calendar valid", "fl:cal:2026-10", 101, telegram.CallbackFlowCalendar, 0, "2026-10", false},
		{"flow calendar invalid month", "fl:cal:2026-13", 101, "", 0, "", true},
		{"flow calendar invalid format", "fl:cal:2026-1", 101, "", 0, "", true},
		{"flow calendar invalid chars", "fl:cal:abcd-ef", 101, "", 0, "", true},
		{"flow date day", "fl:d:2026-10-14", 101, telegram.CallbackFlowDate, 0, "2026-10-14", false},
		{"flow date today", "fl:d:today", 101, telegram.CallbackFlowDate, 0, "today", false},
		{"flow date tomorrow", "fl:d:tomorrow", 101, telegram.CallbackFlowDate, 0, "tomorrow", false},
		{"flow date type", "fl:d:type", 101, telegram.CallbackFlowDate, 0, "type", false},
		{"flow date invalid format", "fl:d:2026-10", 101, "", 0, "", true},
		{"flow date invalid day", "fl:d:2026-10-32", 101, "", 0, "", true},
		{"flow hour 00", "fl:h:00", 101, telegram.CallbackFlowHour, 0, "00", false},
		{"flow hour 10", "fl:h:10", 101, telegram.CallbackFlowHour, 10, "10", false},
		{"flow hour 23", "fl:h:23", 101, telegram.CallbackFlowHour, 23, "23", false},
		{"flow hour 24 invalid", "fl:h:24", 101, "", 0, "", true},
		{"flow hour single digit invalid", "fl:h:9", 101, "", 0, "", true},
		{"flow hour non-numeric", "fl:h:ab", 101, "", 0, "", true},
		{"flow minute 00", "fl:mi:00", 101, telegram.CallbackFlowMinute, 0, "00", false},
		{"flow minute 30", "fl:mi:30", 101, telegram.CallbackFlowMinute, 30, "30", false},
		{"flow minute 55", "fl:mi:55", 101, telegram.CallbackFlowMinute, 55, "55", false},
		{"flow minute other", "fl:mi:o", 101, telegram.CallbackFlowMinute, 0, "o", false},
		{"flow minute 60 invalid", "fl:mi:60", 101, "", 0, "", true},
		{"flow minute single digit invalid", "fl:mi:5", 101, "", 0, "", true},
		{"flow minute non-numeric", "fl:mi:ab", 101, "", 0, "", true},
		{"unknown prefix", "unknown:data", 101, "", 0, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := telegram.ParseCallbackData(tt.data, tt.actorID)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !errors.Is(err, telegram.ErrMalformedCallback) {
					t.Fatalf("expected ErrMalformedCallback, got: %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Kind != tt.wantKind {
				t.Errorf("expected kind %s, got %s", tt.wantKind, res.Kind)
			}
			if res.TargetID != tt.wantTarget {
				t.Errorf("expected target %d, got %d", tt.wantTarget, res.TargetID)
			}
			if res.Payload != tt.wantPay {
				t.Errorf("expected payload %q, got %q", tt.wantPay, res.Payload)
			}
			if res.ActorID != tt.actorID {
				t.Errorf("expected actor %d, got %d", tt.actorID, res.ActorID)
			}
		})
	}
}

func TestGeneratedCallbacks_Max64Bytes(t *testing.T) {
	callbacks := []string{
		"fl:noop",
		telegram.FlowRecurrenceTypeCallback("daily"),
		telegram.FlowRecurrenceTypeCallback("weekly"),
		telegram.FlowRecurrenceTypeCallback("monthly"),
		telegram.FlowRecurrenceTypeCallback("once"),
		telegram.FlowIntervalCallback("1"),
		telegram.FlowIntervalCallback("12"),
		telegram.FlowIntervalCallback("99"),
		telegram.FlowIntervalCallback("o"),
		telegram.FlowCalendarCallback("2026-10"),
		telegram.FlowDateCallback("2026-10-14"),
		telegram.FlowDateCallback("today"),
		telegram.FlowDateCallback("tomorrow"),
		telegram.FlowDateCallback("type"),
		telegram.FlowHourCallback(0),
		telegram.FlowHourCallback(10),
		telegram.FlowHourCallback(23),
		telegram.FlowMinuteCallback(0),
		telegram.FlowMinuteCallback(30),
		telegram.FlowMinuteCallback(55),
	}

	for _, cb := range callbacks {
		if len(cb) > 64 {
			t.Errorf("callback data %q exceeds 64 bytes (len=%d)", cb, len(cb))
		}
		// Also verify it parses cleanly
		parsed, err := telegram.ParseCallbackData(cb, 123)
		if err != nil {
			t.Errorf("generated callback %q failed to parse: %v", cb, err)
		}
		if parsed == nil {
			t.Errorf("parsed result for %q is nil", cb)
		}
	}
}
