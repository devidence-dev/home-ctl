package sqlite_test

import (
	"context"
	"database/sql"
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
	dueTime := now.Add(2 * time.Hour)

	// 1. Create definition
	def := remindersStoreIntegrationCreateDefinition(ctx, t, rStore, now)

	// 2. Create occurrence
	occ := remindersStoreIntegrationCreateOccurrence(ctx, t, rStore, def.ID, dueTime, now)

	// 3. Postpone occurrence, 4. record a notification attempt, 4b. postpone again
	remindersStoreIntegrationPostponeAndNotify(ctx, t, db, rStore, occ.ID, dueTime, now)

	// 5. Complete occurrence with successor
	remindersStoreIntegrationComplete(ctx, t, rStore, occ.ID, dueTime, now)

	// 6. List definitions, open occurrences, due occurrences
	remindersStoreIntegrationCheckLists(ctx, t, rStore, now)
}

// remindersStoreIntegrationCreateDefinition is step 1 of TestRemindersStore_Integration.
func remindersStoreIntegrationCreateDefinition(ctx context.Context, t *testing.T, rStore *sqlite.RemindersStore, now time.Time) *reminders.Definition {
	t.Helper()
	interval := 1
	unit := "day"
	anchor := "scheduled"
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
	return def
}

// remindersStoreIntegrationCreateOccurrence is step 2 of TestRemindersStore_Integration.
func remindersStoreIntegrationCreateOccurrence(ctx context.Context, t *testing.T, rStore *sqlite.RemindersStore, defID int64, dueTime, now time.Time) *reminders.Occurrence {
	t.Helper()
	occ, err := rStore.CreateOccurrence(ctx, reminders.Occurrence{
		DefinitionID: defID,
		TargetUserID: 1002,
		ScheduledAt:  dueTime,
		DueAt:        dueTime,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		t.Fatalf("failed to create occurrence: %v", err)
	}
	return occ
}

// remindersStoreIntegrationPostponeAndNotify is steps 3, 4 and 4b of TestRemindersStore_Integration.
func remindersStoreIntegrationPostponeAndNotify(ctx context.Context, t *testing.T, db *sql.DB, rStore *sqlite.RemindersStore, occID int64, dueTime, now time.Time) {
	t.Helper()
	// 3. Postpone occurrence
	postponeDue := dueTime.Add(time.Hour)
	postponed, err := rStore.PostponeOccurrence(ctx, occID, 1002, postponeDue, now)
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
		occID, 1002, postponeDue.Format(time.RFC3339), attemptTimeStr, attemptTimeStr)
	if err != nil {
		t.Fatalf("failed to insert notification attempt: %v", err)
	}

	lastNotified, err := rStore.GetLastNotificationTime(ctx, occID)
	if err != nil || lastNotified == nil {
		t.Fatalf("GetLastNotificationTime failed: %v, lastNotified=%v", err, lastNotified)
	}
	if !lastNotified.Equal(attemptTime) {
		t.Fatalf("expected lastNotified %v, got %v", attemptTime, *lastNotified)
	}

	// 4b. Postponing again starts a fresh notification cycle: attempts made for the old
	// due_at must not delay a notification at the new due_at.
	laterDue := postponeDue.Add(15 * time.Minute)
	if _, err = rStore.PostponeOccurrence(ctx, occID, 1002, laterDue, now); err != nil {
		t.Fatalf("second postpone failed: %v", err)
	}
	lastNotified, err = rStore.GetLastNotificationTime(ctx, occID)
	if err != nil || lastNotified != nil {
		t.Fatalf("expected no notification history for the new due_at, got %v (err=%v)", lastNotified, err)
	}
}

// remindersStoreIntegrationComplete is step 5 of TestRemindersStore_Integration.
func remindersStoreIntegrationComplete(ctx context.Context, t *testing.T, rStore *sqlite.RemindersStore, occID int64, dueTime, now time.Time) {
	t.Helper()
	nextSlot := dueTime.Add(24 * time.Hour)
	compTime := now.Add(time.Hour)
	completed, succ, err := rStore.CompleteOccurrence(ctx, occID, 1002, compTime, &nextSlot)
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if completed.Status != "completed" || succ == nil {
		t.Fatalf("expected completed status and non-nil successor")
	}

	// Retry completion -> idempotent, nil successor
	compRetry, succRetry, err := rStore.CompleteOccurrence(ctx, occID, 1002, compTime, &nextSlot)
	if err != nil || compRetry.ID != completed.ID || succRetry != nil {
		t.Fatalf("idempotent completion retry failed")
	}
}

// remindersStoreIntegrationCheckLists is step 6 of TestRemindersStore_Integration.
func remindersStoreIntegrationCheckLists(ctx context.Context, t *testing.T, rStore *sqlite.RemindersStore, now time.Time) {
	t.Helper()
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

// remindersStorePostponeEnv is the shared fixture for TestRemindersStore_PostponeOnlyOpenOccurrences.
type remindersStorePostponeEnv struct {
	db    *sql.DB
	store *sqlite.RemindersStore
	defID int64
	now   time.Time
}

// mk creates an open occurrence due at the given time and returns its ID.
func (e remindersStorePostponeEnv) mk(ctx context.Context, t *testing.T, due time.Time) int64 {
	t.Helper()
	occ, err := e.store.CreateOccurrence(ctx, reminders.Occurrence{DefinitionID: e.defID, TargetUserID: 1001, ScheduledAt: due, DueAt: due, CreatedAt: e.now, UpdatedAt: e.now})
	if err != nil {
		t.Fatalf("CreateOccurrence failed: %v", err)
	}
	return occ.ID
}

// postponements counts the reminder postponement records for an occurrence.
func (e remindersStorePostponeEnv) postponements(t *testing.T, id int64) int {
	t.Helper()
	var n int
	if err := e.db.QueryRow(`SELECT COUNT(*) FROM postponements WHERE entity_type = 'reminder' AND occurrence_id = ?`, id).Scan(&n); err != nil {
		t.Fatalf("count postponements: %v", err)
	}
	return n
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
	env := remindersStorePostponeEnv{db: db, store: store, defID: def.ID, now: now}

	t.Run("completed occurrence is never moved", func(t *testing.T) {
		remindersStoreCompletedNeverMoved(ctx, t, env)
	})
	t.Run("duplicate postpone is a no-op", func(t *testing.T) {
		remindersStoreDuplicatePostponeNoop(ctx, t, env)
	})
	t.Run("concurrent complete and postpone: exactly one valid outcome", func(t *testing.T) {
		remindersStoreConcurrentCompleteAndPostpone(ctx, t, env)
	})
}

// remindersStoreCompletedNeverMoved checks that a completed occurrence cannot be postponed.
func remindersStoreCompletedNeverMoved(ctx context.Context, t *testing.T, e remindersStorePostponeEnv) {
	t.Helper()
	due := e.now.Add(time.Hour)
	id := e.mk(ctx, t, due)
	if _, _, err := e.store.CompleteOccurrence(ctx, id, 1001, e.now, nil); err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	_, err := e.store.PostponeOccurrence(ctx, id, 1001, due.Add(3*time.Hour), e.now)
	if !errors.Is(err, reminders.ErrOccurrenceResolved) {
		t.Fatalf("expected ErrOccurrenceResolved, got %v", err)
	}
	got, _ := e.store.GetOccurrence(ctx, id)
	if !got.DueAt.Equal(due) || e.postponements(t, id) != 0 {
		t.Fatalf("completed occurrence changed: due=%v postponements=%d", got.DueAt, e.postponements(t, id))
	}
}

// remindersStoreDuplicatePostponeNoop checks that postponing twice to the same time records once.
func remindersStoreDuplicatePostponeNoop(ctx context.Context, t *testing.T, e remindersStorePostponeEnv) {
	t.Helper()
	id := e.mk(ctx, t, e.now.Add(2*time.Hour))
	target := e.now.Add(5 * time.Hour)
	for i := 0; i < 2; i++ {
		got, err := e.store.PostponeOccurrence(ctx, id, 1001, target, e.now)
		if err != nil || !got.DueAt.Equal(target) {
			t.Fatalf("postpone %d failed: %v (%+v)", i, err, got)
		}
	}
	if n := e.postponements(t, id); n != 1 {
		t.Fatalf("expected exactly one postponement record, got %d", n)
	}
}

// remindersStoreConcurrentCompleteAndPostpone runs the complete-vs-postpone race 25 times.
func remindersStoreConcurrentCompleteAndPostpone(ctx context.Context, t *testing.T, e remindersStorePostponeEnv) {
	t.Helper()
	for i := 0; i < 25; i++ {
		remindersStoreRaceIteration(ctx, t, e, i)
	}
}

// remindersStoreRaceIteration runs one concurrent complete and postpone and checks the single valid outcome.
func remindersStoreRaceIteration(ctx context.Context, t *testing.T, e remindersStorePostponeEnv, i int) {
	t.Helper()
	due := e.now.Add(time.Duration(10+i) * time.Hour)
	newDue := due.Add(4 * time.Hour)
	id := e.mk(ctx, t, due)

	var wg sync.WaitGroup
	var postponeErr, completeErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, postponeErr = e.store.PostponeOccurrence(ctx, id, 1001, newDue, e.now)
	}()
	go func() {
		defer wg.Done()
		_, _, completeErr = e.store.CompleteOccurrence(ctx, id, 1001, e.now, nil)
	}()
	wg.Wait()

	if completeErr != nil {
		t.Fatalf("iteration %d: complete failed: %v", i, completeErr)
	}
	got, _ := e.store.GetOccurrence(ctx, id)
	if got.Status != "completed" {
		t.Fatalf("iteration %d: expected completed, got %s", i, got.Status)
	}
	switch {
	case postponeErr == nil: // postpone won, then completion
		if !got.DueAt.Equal(newDue) || e.postponements(t, id) != 1 {
			t.Fatalf("iteration %d: postpone won but state inconsistent: due=%v records=%d", i, got.DueAt, e.postponements(t, id))
		}
	case errors.Is(postponeErr, reminders.ErrOccurrenceResolved): // completion won
		if !got.DueAt.Equal(due) || e.postponements(t, id) != 0 {
			t.Fatalf("iteration %d: completion won but due moved: due=%v records=%d", i, got.DueAt, e.postponements(t, id))
		}
	default:
		t.Fatalf("iteration %d: unexpected postpone error: %v", i, postponeErr)
	}
}
