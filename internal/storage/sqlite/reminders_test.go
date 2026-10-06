package sqlite_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/config"
	"github.com/px1/home-ctl/internal/reminders"
	"github.com/px1/home-ctl/internal/storage/sqlite"
)

func TestRemindersStore_Integration(t *testing.T) {
	db := newTestDB(t)
	if err := sqlite.RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}
	ctx := context.Background()

	hStore := sqlite.NewHouseholdStore(db)
	_ = hStore.SyncMembers(ctx, []config.UserConfig{
		{ID: 1001, Name: "Alice"},
		{ID: 1002, Name: "Bob"},
	}, "en")

	rStore := sqlite.NewRemindersStore(db)

	now := time.Now().UTC().Truncate(time.Second)
	interval := 1
	unit := "day"
	anchor := "scheduled"

	// 1. Create definition
	def, err := rStore.CreateDefinition(ctx, reminders.Definition{
		Title:              "Take Medicine",
		Emoji:              "💊",
		TargetUserID:       1002, // Bob
		RecurrenceInterval: &interval,
		RecurrenceUnit:     &unit,
		RecurrenceAnchor:   &anchor,
		SeriesOrigin:       &now,
		CreatedBy:          1001,
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err != nil {
		t.Fatalf("failed to create reminder definition: %v", err)
	}

	// 2. Create occurrence
	dueTime := now.Add(2 * time.Hour)
	occ, err := rStore.CreateOccurrence(ctx, reminders.Occurrence{
		DefinitionID: def.ID,
		TargetUserID: 1002,
		ScheduledAt:  dueTime,
		DueAt:        dueTime,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		t.Fatalf("failed to create occurrence: %v", err)
	}

	// 3. Postpone occurrence
	postponeDue := dueTime.Add(time.Hour)
	postponed, err := rStore.PostponeOccurrence(ctx, occ.ID, 1002, postponeDue, now)
	if err != nil {
		t.Fatalf("postpone failed: %v", err)
	}
	if !postponed.DueAt.Equal(postponeDue) {
		t.Fatalf("expected due_at %v, got %v", postponeDue, postponed.DueAt)
	}

	// 4. Record a notification attempt in SQLite and verify GetLastNotificationTime
	attemptTime := now.Add(30 * time.Minute)
	attemptTimeStr := attemptTime.Format(time.RFC3339)
	_, err = db.Exec(`
INSERT INTO notification_attempts (
	entity_type, occurrence_id, target_user_id, due_at, attempt_time, outcome, created_at
) VALUES ('reminder', ?, ?, ?, ?, 'sent', ?)`,
		occ.ID, 1002, postponeDue.Format(time.RFC3339), attemptTimeStr, attemptTimeStr)
	if err != nil {
		t.Fatalf("failed to insert notification attempt: %v", err)
	}

	lastNotified, err := rStore.GetLastNotificationTime(ctx, occ.ID)
	if err != nil || lastNotified == nil {
		t.Fatalf("GetLastNotificationTime failed: %v, lastNotified=%v", err, lastNotified)
	}
	if !lastNotified.Equal(attemptTime) {
		t.Fatalf("expected lastNotified %v, got %v", attemptTime, *lastNotified)
	}

	// 4b. Postponing again starts a fresh notification cycle: attempts made for the old
	// due_at must not delay a notification at the new due_at.
	laterDue := postponeDue.Add(15 * time.Minute)
	if _, err = rStore.PostponeOccurrence(ctx, occ.ID, 1002, laterDue, now); err != nil {
		t.Fatalf("second postpone failed: %v", err)
	}
	lastNotified, err = rStore.GetLastNotificationTime(ctx, occ.ID)
	if err != nil || lastNotified != nil {
		t.Fatalf("expected no notification history for the new due_at, got %v (err=%v)", lastNotified, err)
	}

	// 5. Complete occurrence with successor
	nextSlot := dueTime.Add(24 * time.Hour)
	compTime := now.Add(time.Hour)
	completed, succ, err := rStore.CompleteOccurrence(ctx, occ.ID, 1002, compTime, &nextSlot)
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if completed.Status != "completed" || succ == nil {
		t.Fatalf("expected completed status and non-nil successor")
	}

	// Retry completion -> idempotent, nil successor
	compRetry, succRetry, err := rStore.CompleteOccurrence(ctx, occ.ID, 1002, compTime, &nextSlot)
	if err != nil || compRetry.ID != completed.ID || succRetry != nil {
		t.Fatalf("idempotent completion retry failed")
	}

	// 6. List definitions, open occurrences, due occurrences
	defs, err := rStore.ListDefinitions(ctx)
	if err != nil || len(defs) == 0 {
		t.Fatalf("ListDefinitions failed: %v", err)
	}
	myOpen, err := rStore.ListMyOpenOccurrences(ctx, 1002, 20)
	if err != nil || myOpen.TotalOpen != 1 {
		t.Fatalf("ListMyOpenOccurrences failed: %v, total=%d", err, myOpen.TotalOpen)
	}
	dueList, err := rStore.ListDueOccurrences(ctx, now.Add(48*time.Hour))
	if err != nil || len(dueList) == 0 {
		t.Fatalf("ListDueOccurrences failed: %v", err)
	}
}

func TestRemindersStore_ListUpcoming(t *testing.T) {
	db := newTestDB(t)
	if err := sqlite.RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}
	ctx := context.Background()
	_ = sqlite.NewHouseholdStore(db).SyncMembers(ctx, []config.UserConfig{{ID: 1001, Name: "Alice"}}, "en")
	rStore := sqlite.NewRemindersStore(db)

	now := time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC)
	def, err := rStore.CreateDefinition(ctx, reminders.Definition{Title: "Medicine", Emoji: "💊", TargetUserID: 1001, CreatedBy: 1001, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatalf("CreateDefinition failed: %v", err)
	}
	mk := func(due time.Time) *reminders.Occurrence {
		occ, err := rStore.CreateOccurrence(ctx, reminders.Occurrence{DefinitionID: def.ID, TargetUserID: 1001, ScheduledAt: due, DueAt: due, CreatedAt: now, UpdatedAt: now})
		if err != nil {
			t.Fatalf("CreateOccurrence failed: %v", err)
		}
		return occ
	}
	mk(now.Add(-time.Hour))
	late := mk(now.Add(30 * time.Hour))
	soon := mk(now.Add(2 * time.Hour))
	mk(now.Add(72 * time.Hour))

	got, err := rStore.ListUpcomingOccurrences(ctx, now, now.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("ListUpcomingOccurrences failed: %v", err)
	}
	if len(got) != 2 || got[0].ID != soon.ID || got[1].ID != late.ID {
		t.Fatalf("expected [soon, late] ordered by due_at, got %+v", got)
	}
}

func TestRemindersStore_PostponeOnlyOpenOccurrences(t *testing.T) {
	db := newTestDB(t)
	if err := sqlite.RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}
	ctx := context.Background()
	_ = sqlite.NewHouseholdStore(db).SyncMembers(ctx, []config.UserConfig{{ID: 1001, Name: "Alice"}}, "en")
	store := sqlite.NewRemindersStore(db)

	now := time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC)
	def, err := store.CreateDefinition(ctx, reminders.Definition{Title: "Medicine", Emoji: "💊", TargetUserID: 1001, CreatedBy: 1001, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatalf("CreateDefinition failed: %v", err)
	}
	mk := func(due time.Time) int64 {
		occ, err := store.CreateOccurrence(ctx, reminders.Occurrence{DefinitionID: def.ID, TargetUserID: 1001, ScheduledAt: due, DueAt: due, CreatedAt: now, UpdatedAt: now})
		if err != nil {
			t.Fatalf("CreateOccurrence failed: %v", err)
		}
		return occ.ID
	}
	postponements := func(id int64) int {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM postponements WHERE entity_type = 'reminder' AND occurrence_id = ?`, id).Scan(&n); err != nil {
			t.Fatalf("count postponements: %v", err)
		}
		return n
	}

	t.Run("completed occurrence is never moved", func(t *testing.T) {
		due := now.Add(time.Hour)
		id := mk(due)
		if _, _, err := store.CompleteOccurrence(ctx, id, 1001, now, nil); err != nil {
			t.Fatalf("complete failed: %v", err)
		}
		_, err := store.PostponeOccurrence(ctx, id, 1001, due.Add(3*time.Hour), now)
		if !errors.Is(err, reminders.ErrOccurrenceResolved) {
			t.Fatalf("expected ErrOccurrenceResolved, got %v", err)
		}
		got, _ := store.GetOccurrence(ctx, id)
		if !got.DueAt.Equal(due) || postponements(id) != 0 {
			t.Fatalf("completed occurrence changed: due=%v postponements=%d", got.DueAt, postponements(id))
		}
	})

	t.Run("duplicate postpone is a no-op", func(t *testing.T) {
		id := mk(now.Add(2 * time.Hour))
		target := now.Add(5 * time.Hour)
		for i := 0; i < 2; i++ {
			got, err := store.PostponeOccurrence(ctx, id, 1001, target, now)
			if err != nil || !got.DueAt.Equal(target) {
				t.Fatalf("postpone %d failed: %v (%+v)", i, err, got)
			}
		}
		if n := postponements(id); n != 1 {
			t.Fatalf("expected exactly one postponement record, got %d", n)
		}
	})

	t.Run("concurrent complete and postpone: exactly one valid outcome", func(t *testing.T) {
		for i := 0; i < 25; i++ {
			due := now.Add(time.Duration(10+i) * time.Hour)
			newDue := due.Add(4 * time.Hour)
			id := mk(due)

			var wg sync.WaitGroup
			var postponeErr, completeErr error
			wg.Add(2)
			go func() {
				defer wg.Done()
				_, postponeErr = store.PostponeOccurrence(ctx, id, 1001, newDue, now)
			}()
			go func() {
				defer wg.Done()
				_, _, completeErr = store.CompleteOccurrence(ctx, id, 1001, now, nil)
			}()
			wg.Wait()

			if completeErr != nil {
				t.Fatalf("iteration %d: complete failed: %v", i, completeErr)
			}
			got, _ := store.GetOccurrence(ctx, id)
			if got.Status != "completed" {
				t.Fatalf("iteration %d: expected completed, got %s", i, got.Status)
			}
			switch {
			case postponeErr == nil: // postpone won, then completion
				if !got.DueAt.Equal(newDue) || postponements(id) != 1 {
					t.Fatalf("iteration %d: postpone won but state inconsistent: due=%v records=%d", i, got.DueAt, postponements(id))
				}
			case errors.Is(postponeErr, reminders.ErrOccurrenceResolved): // completion won
				if !got.DueAt.Equal(due) || postponements(id) != 0 {
					t.Fatalf("iteration %d: completion won but due moved: due=%v records=%d", i, got.DueAt, postponements(id))
				}
			default:
				t.Fatalf("iteration %d: unexpected postpone error: %v", i, postponeErr)
			}
		}
	})
}
