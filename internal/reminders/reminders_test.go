package reminders_test

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/household"
	"github.com/px1/home-ctl/internal/recurrence"
	"github.com/px1/home-ctl/internal/reminders"
)

type fakeClock struct {
	now time.Time
	loc *time.Location
}

func (c *fakeClock) Now() time.Time {
	return c.now.In(c.loc)
}

func (c *fakeClock) Location() *time.Location {
	return c.loc
}

type fakeMemberChecker struct {
	members map[int64]*household.Member
}

func (f *fakeMemberChecker) Authorize(_ context.Context, userID int64) (*household.Member, error) {
	m, ok := f.members[userID]
	if !ok || !m.Active {
		return nil, household.ErrNotMember
	}
	return m, nil
}

type fakeRemindersStore struct {
	mu           sync.Mutex
	definitions  map[int64]*reminders.Definition
	occurrences  map[int64]*reminders.Occurrence
	lastNotified map[int64]*time.Time
	nextID       int64
}

func newFakeRemindersStore() *fakeRemindersStore {
	return &fakeRemindersStore{
		definitions:  make(map[int64]*reminders.Definition),
		occurrences:  make(map[int64]*reminders.Occurrence),
		lastNotified: make(map[int64]*time.Time),
		nextID:       1,
	}
}

func (f *fakeRemindersStore) CreateDefinition(_ context.Context, def reminders.Definition) (*reminders.Definition, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	def.ID = f.nextID
	f.nextID++
	f.definitions[def.ID] = &def
	return &def, nil
}

func (f *fakeRemindersStore) GetDefinition(_ context.Context, id int64) (*reminders.Definition, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.definitions[id]
	if !ok {
		return nil, reminders.ErrDefinitionNotFound
	}
	return d, nil
}

func (f *fakeRemindersStore) ListDefinitions(_ context.Context) ([]reminders.Definition, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var list []reminders.Definition
	for _, d := range f.definitions {
		list = append(list, *d)
	}
	return list, nil
}

func (f *fakeRemindersStore) CreateOccurrence(_ context.Context, occ reminders.Occurrence) (*reminders.Occurrence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	occ.ID = f.nextID
	f.nextID++
	occ.Definition = f.definitions[occ.DefinitionID]
	f.occurrences[occ.ID] = &occ
	return &occ, nil
}

func (f *fakeRemindersStore) GetOccurrence(_ context.Context, id int64) (*reminders.Occurrence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	occ, ok := f.occurrences[id]
	if !ok {
		return nil, reminders.ErrOccurrenceNotFound
	}
	return occ, nil
}

func (f *fakeRemindersStore) CompleteOccurrence(_ context.Context, occID, actorID int64, completedAt time.Time, nextSlot *time.Time) (*reminders.Occurrence, *reminders.Occurrence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	occ, ok := f.occurrences[occID]
	if !ok {
		return nil, nil, reminders.ErrOccurrenceNotFound
	}
	if occ.Status == "completed" {
		return occ, nil, nil
	}
	occ.Status = "completed"
	occ.CompletedBy = &actorID
	occ.CompletedAt = &completedAt

	var successor *reminders.Occurrence
	if nextSlot != nil {
		succ := &reminders.Occurrence{
			ID:           f.nextID,
			DefinitionID: occ.DefinitionID,
			Definition:   f.definitions[occ.DefinitionID],
			TargetUserID: occ.TargetUserID,
			ScheduledAt:  *nextSlot,
			DueAt:        *nextSlot,
			Status:       "open",
			CreatedAt:    completedAt,
			UpdatedAt:    completedAt,
		}
		f.nextID++
		f.occurrences[succ.ID] = succ
		successor = succ
	}
	return occ, successor, nil
}

func (f *fakeRemindersStore) PostponeOccurrence(_ context.Context, occID, _ int64, newDueAt, _ time.Time) (*reminders.Occurrence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	occ, ok := f.occurrences[occID]
	if !ok {
		return nil, reminders.ErrOccurrenceNotFound
	}
	occ.DueAt = newDueAt
	delete(f.lastNotified, occID)
	return occ, nil
}

func (f *fakeRemindersStore) ListMyOpenOccurrences(_ context.Context, memberID int64, limit int) (reminders.ListResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var open []reminders.Occurrence
	for _, occ := range f.occurrences {
		if occ.TargetUserID == memberID && occ.Status == "open" {
			open = append(open, *occ)
		}
	}
	sort.Slice(open, func(i, j int) bool {
		return open[i].DueAt.Before(open[j].DueAt)
	})
	total := len(open)
	if len(open) > limit {
		open = open[:limit]
	}
	rem := total - len(open)
	return reminders.ListResult{
		Occurrences:    open,
		TotalOpen:      total,
		RemainingCount: rem,
	}, nil
}

func (f *fakeRemindersStore) ListUpcomingOccurrences(_ context.Context, from, to time.Time) ([]reminders.Occurrence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var upcoming []reminders.Occurrence
	for _, occ := range f.occurrences {
		if occ.Status == "open" && !occ.DueAt.Before(from) && occ.DueAt.Before(to) {
			upcoming = append(upcoming, *occ)
		}
	}
	sort.Slice(upcoming, func(i, j int) bool { return upcoming[i].DueAt.Before(upcoming[j].DueAt) })
	return upcoming, nil
}

func (f *fakeRemindersStore) ListDueOccurrences(_ context.Context, dueBefore time.Time) ([]reminders.Occurrence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var due []reminders.Occurrence
	for _, occ := range f.occurrences {
		if occ.Status == "open" && !occ.DueAt.After(dueBefore) {
			due = append(due, *occ)
		}
	}
	return due, nil
}

func (f *fakeRemindersStore) GetLastNotificationTime(_ context.Context, occID int64) (*time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastNotified[occID], nil
}

func (f *fakeRemindersStore) setLastNotificationTime(occID int64, t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastNotified[occID] = &t
}

func TestReminders_TargetOnly_AndRepeatPolicy(t *testing.T) {
	ctx := context.Background()
	loc := time.UTC
	clock := &fakeClock{
		now: time.Date(2026, time.October, 6, 10, 0, 0, 0, loc),
		loc: loc,
	}
	members := &fakeMemberChecker{
		members: map[int64]*household.Member{
			1001: {ID: 1001, Name: "Alice", Active: true},
			1002: {ID: 1002, Name: "Bob", Active: true},
		},
	}
	store := newFakeRemindersStore()
	svc := reminders.NewService(store, members, clock)

	// Blank title rejected
	_, err := svc.CreateDefinition(ctx, 1001, 1002, "  ", "💊", "", nil)
	if !errors.Is(err, reminders.ErrBlankTitle) {
		t.Fatalf("expected ErrBlankTitle, got %v", err)
	}

	// Target not a member rejected
	_, err = svc.CreateDefinition(ctx, 1001, 9999, "Buy medicine", "💊", "", nil)
	if !errors.Is(err, reminders.ErrNotMember) {
		t.Fatalf("expected ErrNotMember, got %v", err)
	}

	// Create valid reminder for Bob
	def, err := svc.CreateDefinition(ctx, 1001, 1002, "Buy medicine", "💊", "", &recurrence.Rule{
		Interval: 1,
		Unit:     recurrence.UnitDay,
		Anchor:   recurrence.AnchorScheduled,
	})
	if err != nil {
		t.Fatalf("CreateDefinition failed: %v", err)
	}

	// Schedule occurrence due today at 10:00
	occ, err := svc.ScheduleOccurrence(ctx, 1001, def.ID, clock.now)
	if err != nil {
		t.Fatalf("ScheduleOccurrence failed: %v", err)
	}
	if occ.TargetUserID != 1002 {
		t.Fatalf("expected target 1002, got %d", occ.TargetUserID)
	}

	// 1. Target-only check: Alice cannot complete Bob's reminder
	_, _, err = svc.Complete(ctx, 1001, occ.ID, clock.now)
	if !errors.Is(err, reminders.ErrNotTarget) {
		t.Fatalf("expected ErrNotTarget when Alice completes Bob's reminder, got %v", err)
	}

	// Alice cannot postpone Bob's reminder
	_, err = svc.Postpone(ctx, 1001, occ.ID, clock.now.Add(time.Hour))
	if !errors.Is(err, reminders.ErrNotTarget) {
		t.Fatalf("expected ErrNotTarget when Alice postpones, got %v", err)
	}

	// Non-target with a past time still gets ErrNotTarget, not a time error
	_, err = svc.Postpone(ctx, 1001, occ.ID, clock.now.Add(-time.Hour))
	if !errors.Is(err, reminders.ErrNotTarget) {
		t.Fatalf("expected ErrNotTarget for non-target with past time, got %v", err)
	}

	// Target with a past time gets ErrPastDueTime
	_, err = svc.Postpone(ctx, 1002, occ.ID, clock.now.Add(-time.Hour))
	if !errors.Is(err, reminders.ErrPastDueTime) {
		t.Fatalf("expected ErrPastDueTime for target with past time, got %v", err)
	}

	// 2. Repeat-notification policy test (10:00, 11:00, 12:00 repeats)
	interval := time.Hour

	// At 10:00 (first notification due)
	due10, err := svc.IsNotificationDue(ctx, occ, interval, clock.now)
	if err != nil || !due10 {
		t.Fatalf("expected notification due at 10:00")
	}

	// Recorded 10:00 send
	store.setLastNotificationTime(occ.ID, clock.now)

	// At 10:30 (interval not elapsed)
	due1030, err := svc.IsNotificationDue(ctx, occ, interval, clock.now.Add(30*time.Minute))
	if err != nil || due1030 {
		t.Fatalf("expected notification NOT due at 10:30")
	}

	// At 11:00 (repeat due)
	due11, err := svc.IsNotificationDue(ctx, occ, interval, clock.now.Add(time.Hour))
	if err != nil || !due11 {
		t.Fatalf("expected repeat notification due at 11:00")
	}
	store.setLastNotificationTime(occ.ID, clock.now.Add(time.Hour))

	// At 12:00 (next repeat due)
	due12, err := svc.IsNotificationDue(ctx, occ, interval, clock.now.Add(2*time.Hour))
	if err != nil || !due12 {
		t.Fatalf("expected repeat notification due at 12:00")
	}

	// 3. Postpone stops repeats
	// Bob postpones at 11:10 by 3 hours (new due = 14:10)
	postponedDue := clock.now.Add(time.Hour + 10*time.Minute).Add(3 * time.Hour)
	postponedOcc, err := svc.Postpone(ctx, 1002, occ.ID, postponedDue)
	if err != nil {
		t.Fatalf("postpone failed: %v", err)
	}

	// At 12:00 (before new due 14:10) -> NOT due
	dueAfterPostpone, err := svc.IsNotificationDue(ctx, postponedOcc, interval, clock.now.Add(2*time.Hour))
	if err != nil || dueAfterPostpone {
		t.Fatalf("expected NO notification after postponement before new due time")
	}

	// At 14:10 -> due again
	dueAtNewTime, err := svc.IsNotificationDue(ctx, postponedOcc, interval, postponedDue)
	if err != nil || !dueAtNewTime {
		t.Fatalf("expected notification due at new postponed time 14:10")
	}

	// 4. Complete by Bob with successor
	completed, succ, err := svc.Complete(ctx, 1002, occ.ID, postponedDue)
	if err != nil {
		t.Fatalf("Bob complete failed: %v", err)
	}
	if completed.Status != "completed" || succ == nil {
		t.Fatalf("expected completed with successor")
	}

	// Once completed, no more notifications
	dueAfterComplete, err := svc.IsNotificationDue(ctx, completed, interval, postponedDue.Add(time.Hour))
	if err != nil || dueAfterComplete {
		t.Fatalf("expected NO notification once completed")
	}

	// 5. ListMyOpen: Alice sees 0, Bob sees his
	aliceList, err := svc.ListMyOpen(ctx, 1001)
	if err != nil || len(aliceList.Occurrences) != 0 {
		t.Fatalf("Alice should see 0 reminders")
	}
	bobList, err := svc.ListMyOpen(ctx, 1002)
	if err != nil || len(bobList.Occurrences) != 1 {
		t.Fatalf("Bob should see 1 open reminder (successor), got %d", len(bobList.Occurrences))
	}

	// Exercise ListDefinitions and GetOccurrence
	defs, err := svc.ListDefinitions(ctx)
	if err != nil || len(defs) == 0 {
		t.Fatalf("ListDefinitions failed: %v", err)
	}
	gotOcc, err := svc.GetOccurrence(ctx, occ.ID)
	if err != nil || gotOcc.ID != occ.ID {
		t.Fatalf("GetOccurrence failed: %v", err)
	}
}

func TestReminders_ListUpcoming_ReadOnlyLookahead(t *testing.T) {
	ctx := context.Background()
	clock := &fakeClock{now: time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC), loc: time.UTC}
	members := &fakeMemberChecker{members: map[int64]*household.Member{
		1001: {ID: 1001, Name: "Alice", Active: true},
		1002: {ID: 1002, Name: "Bob", Active: true},
	}}
	svc := reminders.NewService(newFakeRemindersStore(), members, clock)

	def, err := svc.CreateDefinition(ctx, 1001, 1002, "Buy medicine", "💊", "", nil)
	if err != nil {
		t.Fatalf("CreateDefinition failed: %v", err)
	}
	later, _ := svc.ScheduleOccurrence(ctx, 1001, def.ID, clock.now.Add(30*time.Hour))
	sooner, _ := svc.ScheduleOccurrence(ctx, 1001, def.ID, clock.now.Add(2*time.Hour))
	_, _ = svc.ScheduleOccurrence(ctx, 1001, def.ID, clock.now.Add(100*time.Hour))

	got, err := svc.ListUpcoming(ctx, clock.now, clock.now.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("ListUpcoming failed: %v", err)
	}
	if len(got) != 2 || got[0].ID != sooner.ID || got[1].ID != later.ID {
		t.Fatalf("expected [sooner, later] by due_at, got %+v", got)
	}
}

func TestReminders_Postpone_RejectsResolvedOccurrence(t *testing.T) {
	ctx := context.Background()
	clock := &fakeClock{now: time.Date(2026, time.October, 6, 12, 0, 0, 0, time.UTC), loc: time.UTC}
	members := &fakeMemberChecker{members: map[int64]*household.Member{
		1001: {ID: 1001, Name: "Alice", Active: true},
		1002: {ID: 1002, Name: "Bob", Active: true},
	}}
	svc := reminders.NewService(newFakeRemindersStore(), members, clock)

	def, _ := svc.CreateDefinition(ctx, 1001, 1002, "Buy medicine", "💊", "", nil)
	due := clock.now.Add(2 * time.Hour)
	occ, _ := svc.ScheduleOccurrence(ctx, 1001, def.ID, due)
	if _, _, err := svc.Complete(ctx, 1002, occ.ID, clock.now); err != nil {
		t.Fatalf("complete failed: %v", err)
	}

	if _, err := svc.Postpone(ctx, 1001, occ.ID, clock.now.Add(-time.Hour)); !errors.Is(err, reminders.ErrNotTarget) {
		t.Fatalf("non-target must get ErrNotTarget even for a resolved occurrence, got %v", err)
	}
	if _, err := svc.Postpone(ctx, 1002, occ.ID, clock.now.Add(-time.Hour)); !errors.Is(err, reminders.ErrOccurrenceResolved) {
		t.Fatalf("resolved check must precede time validation, got %v", err)
	}
	if _, err := svc.Postpone(ctx, 1002, occ.ID, due.Add(3*time.Hour)); !errors.Is(err, reminders.ErrOccurrenceResolved) {
		t.Fatalf("expected ErrOccurrenceResolved, got %v", err)
	}
}
