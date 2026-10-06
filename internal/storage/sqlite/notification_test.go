package sqlite_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/config"
	"github.com/px1/home-ctl/internal/notification"
	"github.com/px1/home-ctl/internal/storage/sqlite"
)

func TestNotificationStore_Integration(t *testing.T) {
	db := newTestDB(t)
	if err := sqlite.RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}
	ctx := context.Background()

	hStore := sqlite.NewHouseholdStore(db)
	_ = hStore.SyncMembers(ctx, []config.UserConfig{
		{ID: 1001, Name: "Alice"},
	}, "en")

	nStore := sqlite.NewNotificationStore(db)

	now := time.Now().UTC().Truncate(time.Second)
	due := now.Add(time.Hour)

	// 1. Claim chore attempt
	attID, err := nStore.ClaimChoreAttempt(ctx, 1, 1001, due, now)
	if err != nil || attID == 0 {
		t.Fatalf("ClaimChoreAttempt failed: %v", err)
	}

	// Duplicate claim for same chore and due_at rejected
	_, err = nStore.ClaimChoreAttempt(ctx, 1, 1001, due, now)
	if !errors.Is(err, notification.ErrAlreadyClaimed) {
		t.Fatalf("expected ErrAlreadyClaimed on duplicate claim, got %v", err)
	}

	// Update outcome to sent
	msgID := int64(99988)
	err = nStore.UpdateAttemptOutcome(ctx, attID, "sent", "", &msgID)
	if err != nil {
		t.Fatalf("UpdateAttemptOutcome failed: %v", err)
	}

	// 2. Claim reminder attempts with sequential numbering
	rAtt1, seq1, err := nStore.ClaimReminderAttempt(ctx, 5, 1001, due, now)
	if err != nil || seq1 != 1 {
		t.Fatalf("reminder claim 1 failed: %v, seq=%d", err, seq1)
	}
	_ = nStore.UpdateAttemptOutcome(ctx, rAtt1, "sent", "", &msgID)

	rAtt2, seq2, err := nStore.ClaimReminderAttempt(ctx, 5, 1001, due, now.Add(time.Hour))
	if err != nil || seq2 != 2 {
		t.Fatalf("reminder claim 2 failed: %v, seq=%d", err, seq2)
	}
	_ = nStore.UpdateAttemptOutcome(ctx, rAtt2, "failed", "network timeout", nil)

	// 3. List attempts
	attempts, err := nStore.ListAttempts(ctx, "reminder", 5)
	if err != nil || len(attempts) != 2 {
		t.Fatalf("ListAttempts failed: %v, len=%d", err, len(attempts))
	}
	if attempts[0].AttemptSeq != 1 || attempts[0].Outcome != "sent" {
		t.Fatalf("unexpected attempt 0: %+v", attempts[0])
	}
	if attempts[1].AttemptSeq != 2 || attempts[1].Outcome != "failed" || attempts[1].FailureReason != "network timeout" {
		t.Fatalf("unexpected attempt 1: %+v", attempts[1])
	}
}
