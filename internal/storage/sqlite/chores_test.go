package sqlite_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/chores"
	"github.com/px1/home-ctl/internal/config"
	"github.com/px1/home-ctl/internal/storage/sqlite"
)

func TestChoresStore_Integration(t *testing.T) {
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

	cStore := sqlite.NewChoresStore(db)

	now := time.Now().UTC().Truncate(time.Second)
	interval := 1
	unit := "week"
	anchor := "scheduled"

	// 1. Create definition
	def, err := cStore.CreateDefinition(ctx, chores.Definition{
		Name:               "Sweep Floor",
		Emoji:              "🧹",
		RecurrenceInterval: &interval,
		RecurrenceUnit:     &unit,
		RecurrenceAnchor:   &anchor,
		SeriesOrigin:       &now,
		CreatedBy:          1001,
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err != nil {
		t.Fatalf("failed to create chore definition: %v", err)
	}

	// 2. Create occurrence
	dueTime := now.Add(24 * time.Hour)
	occ, err := cStore.CreateOccurrence(ctx, chores.Occurrence{
		DefinitionID: def.ID,
		AssigneeID:   1001,
		ScheduledAt:  dueTime,
		DueAt:        dueTime,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		t.Fatalf("failed to create occurrence: %v", err)
	}

	// 3. Postpone occurrence
	postponeDue := dueTime.Add(3 * time.Hour)
	postponed, err := cStore.PostponeOccurrence(ctx, occ.ID, 1001, postponeDue, now)
	if err != nil {
		t.Fatalf("postpone failed: %v", err)
	}
	if !postponed.DueAt.Equal(postponeDue) {
		t.Fatalf("expected due_at %v, got %v", postponeDue, postponed.DueAt)
	}
	if !postponed.ScheduledAt.Equal(dueTime) {
		t.Fatalf("scheduled_at changed unexpectedly: %v", postponed.ScheduledAt)
	}

	// Verify postponement record in SQLite
	var postCount int
	err = db.QueryRow("SELECT COUNT(*) FROM postponements WHERE occurrence_id = ? AND entity_type = 'chore'", occ.ID).Scan(&postCount)
	if err != nil || postCount != 1 {
		t.Fatalf("expected 1 postponement record in DB, got %d", postCount)
	}

	// 4. Complete occurrence with successor
	nextSlot := dueTime.Add(7 * 24 * time.Hour)
	compTime := now.Add(2 * time.Hour)
	completed, succ, err := cStore.CompleteOccurrence(ctx, occ.ID, 1001, compTime, &nextSlot)
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if completed.Status != "completed" || *completed.CompletedBy != 1001 {
		t.Fatalf("unexpected completion state: %+v", completed)
	}
	if succ == nil {
		t.Fatalf("expected successor occurrence to be created")
	}

	// Retry completion (double Done / crash recovery) -> exactly 1 successor in DB
	completedRetry, succRetry, err := cStore.CompleteOccurrence(ctx, occ.ID, 1001, compTime, &nextSlot)
	if err != nil {
		t.Fatalf("retry complete failed: %v", err)
	}
	if completedRetry.ID != completed.ID {
		t.Fatalf("expected same completed ID on retry")
	}
	if succRetry != nil {
		t.Fatalf("expected nil successor on retry")
	}

	var succCount int
	err = db.QueryRow("SELECT COUNT(*) FROM chore_occurrences WHERE definition_id = ? AND scheduled_at = ?",
		def.ID, nextSlot.Format(time.RFC3339)).Scan(&succCount)
	if err != nil || succCount != 1 {
		t.Fatalf("expected exactly 1 successor occurrence in DB, got %d", succCount)
	}

	// 5. 35 items cap test on SQLite
	for i := 1; i <= 35; i++ {
		slot := now.Add(time.Duration(100+i) * time.Hour)
		_, err := cStore.CreateOccurrence(ctx, chores.Occurrence{
			DefinitionID: def.ID,
			AssigneeID:   1002, // Bob
			ScheduledAt:  slot,
			DueAt:        slot,
			CreatedAt:    now,
			UpdatedAt:    now,
		})
		if err != nil {
			t.Fatalf("failed to insert batch occurrence %d: %v", i, err)
		}
	}

	listRes, err := cStore.ListMyOpenOccurrences(ctx, 1002, 20)
	if err != nil {
		t.Fatalf("ListMyOpenOccurrences failed: %v", err)
	}
	if listRes.TotalOpen != 35 {
		t.Fatalf("expected 35 total open, got %d", listRes.TotalOpen)
	}
	if len(listRes.Occurrences) != 20 {
		t.Fatalf("expected 20 items returned, got %d", len(listRes.Occurrences))
	}
	if listRes.RemainingCount != 15 {
		t.Fatalf("expected 15 remaining, got %d", listRes.RemainingCount)
	}

	// Exercise ListDefinitions and ListDueOccurrences
	defs, err := cStore.ListDefinitions(ctx)
	if err != nil || len(defs) == 0 {
		t.Fatalf("ListDefinitions failed: %v", err)
	}
	dueOccs, err := cStore.ListDueOccurrences(ctx, now.Add(200*time.Hour))
	if err != nil || len(dueOccs) == 0 {
		t.Fatalf("ListDueOccurrences failed: %v", err)
	}
}

func TestChoresStore_ListUpcoming(t *testing.T) {
	db := newTestDB(t)
	if err := sqlite.RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}
	ctx := context.Background()
	_ = sqlite.NewHouseholdStore(db).SyncMembers(ctx, []config.UserConfig{{ID: 1001, Name: "Alice"}}, "en")
	cStore := sqlite.NewChoresStore(db)

	now := time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC)
	def, err := cStore.CreateDefinition(ctx, chores.Definition{Name: "Sweep", Emoji: "🧹", CreatedBy: 1001, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatalf("CreateDefinition failed: %v", err)
	}
	mk := func(due time.Time) *chores.Occurrence {
		occ, err := cStore.CreateOccurrence(ctx, chores.Occurrence{DefinitionID: def.ID, AssigneeID: 1001, ScheduledAt: due, DueAt: due, CreatedAt: now, UpdatedAt: now})
		if err != nil {
			t.Fatalf("CreateOccurrence failed: %v", err)
		}
		return occ
	}
	overdue := mk(now.Add(-time.Hour))
	late := mk(now.Add(30 * time.Hour))
	soon := mk(now.Add(2 * time.Hour))
	done := mk(now.Add(5 * time.Hour))
	beyond := mk(now.Add(72 * time.Hour))
	if _, _, err := cStore.CompleteOccurrence(ctx, done.ID, 1001, now, nil); err != nil {
		t.Fatalf("CompleteOccurrence failed: %v", err)
	}

	got, err := cStore.ListUpcomingOccurrences(ctx, now, now.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("ListUpcomingOccurrences failed: %v", err)
	}
	if len(got) != 2 || got[0].ID != soon.ID || got[1].ID != late.ID {
		t.Fatalf("expected [soon, late] ordered by due_at, got %+v (overdue=%d beyond=%d)", got, overdue.ID, beyond.ID)
	}
	if got[0].Definition == nil || got[0].Definition.Name != "Sweep" {
		t.Fatalf("expected definition to be loaded, got %+v", got[0].Definition)
	}
}

func TestChoresStore_PostponeOnlyOpenOccurrences(t *testing.T) {
	db := newTestDB(t)
	if err := sqlite.RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}
	ctx := context.Background()
	_ = sqlite.NewHouseholdStore(db).SyncMembers(ctx, []config.UserConfig{{ID: 1001, Name: "Alice"}}, "en")
	store := sqlite.NewChoresStore(db)

	now := time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC)
	def, err := store.CreateDefinition(ctx, chores.Definition{Name: "Sweep", Emoji: "🧹", CreatedBy: 1001, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatalf("CreateDefinition failed: %v", err)
	}
	mk := func(due time.Time) int64 {
		occ, err := store.CreateOccurrence(ctx, chores.Occurrence{DefinitionID: def.ID, AssigneeID: 1001, ScheduledAt: due, DueAt: due, CreatedAt: now, UpdatedAt: now})
		if err != nil {
			t.Fatalf("CreateOccurrence failed: %v", err)
		}
		return occ.ID
	}
	postponements := func(id int64) int {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM postponements WHERE entity_type = 'chore' AND occurrence_id = ?`, id).Scan(&n); err != nil {
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
		if !errors.Is(err, chores.ErrOccurrenceResolved) {
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
			case errors.Is(postponeErr, chores.ErrOccurrenceResolved): // completion won
				if !got.DueAt.Equal(due) || postponements(id) != 0 {
					t.Fatalf("iteration %d: completion won but due moved: due=%v records=%d", i, got.DueAt, postponements(id))
				}
			default:
				t.Fatalf("iteration %d: unexpected postpone error: %v", i, postponeErr)
			}
		}
	})
}
