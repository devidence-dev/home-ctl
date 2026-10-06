package notification_test

import (
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/notification"
)

func TestAttempt(t *testing.T) {
	now := time.Now().UTC()
	att := notification.Attempt{
		ID:           1,
		EntityType:   "chore",
		OccurrenceID: 10,
		TargetUserID: 101,
		DueAt:        now,
		AttemptTime:  now,
		AttemptSeq:   1,
		Outcome:      "sent",
		CreatedAt:    now,
	}
	if att.ID != 1 || att.EntityType != "chore" {
		t.Fatalf("unexpected attempt values: %+v", att)
	}
	if notification.ErrAlreadyClaimed == nil {
		t.Fatal("expected non-nil ErrAlreadyClaimed")
	}
}
