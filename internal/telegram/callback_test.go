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
