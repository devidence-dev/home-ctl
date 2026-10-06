package chores_test

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/chores"
	"github.com/px1/home-ctl/internal/household"
	"github.com/px1/home-ctl/internal/recurrence"
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

type fakeChoresStore struct {
	mu          sync.Mutex
	definitions map[int64]*chores.Definition
	occurrences map[int64]*chores.Occurrence
	nextID      int64
}

func newFakeChoresStore() *fakeChoresStore {
	return &fakeChoresStore{
		definitions: make(map[int64]*chores.Definition),
		occurrences: make(map[int64]*chores.Occurrence),
		nextID:      1,
	}
}

func (f *fakeChoresStore) CreateDefinition(_ context.Context, def chores.Definition) (*chores.Definition, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	def.ID = f.nextID
	f.nextID++
	f.definitions[def.ID] = &def
	return &def, nil
}

func (f *fakeChoresStore) GetDefinition(_ context.Context, id int64) (*chores.Definition, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.definitions[id]
	if !ok {
		return nil, chores.ErrDefinitionNotFound
	}
	return d, nil
}

func (f *fakeChoresStore) ListDefinitions(_ context.Context) ([]chores.Definition, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var list []chores.Definition
	for _, d := range f.definitions {
		list = append(list, *d)
	}
	return list, nil
}

func (f *fakeChoresStore) CreateOccurrence(_ context.Context, occ chores.Occurrence) (*chores.Occurrence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	occ.ID = f.nextID
	f.nextID++
	occ.Definition = f.definitions[occ.DefinitionID]
	f.occurrences[occ.ID] = &occ
	return &occ, nil
}

func (f *fakeChoresStore) GetOccurrence(_ context.Context, id int64) (*chores.Occurrence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	occ, ok := f.occurrences[id]
	if !ok {
		return nil, chores.ErrOccurrenceNotFound
	}
	return occ, nil
}

func (f *fakeChoresStore) CompleteOccurrence(_ context.Context, occID, actorID int64, completedAt time.Time, nextSlot *time.Time) (*chores.Occurrence, *chores.Occurrence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	occ, ok := f.occurrences[occID]
	if !ok {
		return nil, nil, chores.ErrOccurrenceNotFound
	}
	if occ.Status == "completed" {
		return occ, nil, nil
	}
	occ.Status = "completed"
	occ.CompletedBy = &actorID
	occ.CompletedAt = &completedAt

	var successor *chores.Occurrence
	if nextSlot != nil {
		succ := &chores.Occurrence{
			ID:           f.nextID,
			DefinitionID: occ.DefinitionID,
			Definition:   f.definitions[occ.DefinitionID],
			AssigneeID:   occ.AssigneeID,
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

func (f *fakeChoresStore) PostponeOccurrence(_ context.Context, occID, _ int64, newDueAt, _ time.Time) (*chores.Occurrence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	occ, ok := f.occurrences[occID]
	if !ok {
		return nil, chores.ErrOccurrenceNotFound
	}
	occ.DueAt = newDueAt
	return occ, nil
}

func (f *fakeChoresStore) ListMyOpenOccurrences(_ context.Context, memberID int64, limit int) (chores.ListResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var open []chores.Occurrence
	for _, occ := range f.occurrences {
		if occ.AssigneeID == memberID && occ.Status == "open" {
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
	return chores.ListResult{
		Occurrences:    open,
		TotalOpen:      total,
		RemainingCount: rem,
	}, nil
}

func (f *fakeChoresStore) ListUpcomingOccurrences(_ context.Context, from, to time.Time) ([]chores.Occurrence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var upcoming []chores.Occurrence
	for _, occ := range f.occurrences {
		if occ.Status == "open" && !occ.DueAt.Before(from) && occ.DueAt.Before(to) {
			upcoming = append(upcoming, *occ)
		}
	}
	sort.Slice(upcoming, func(i, j int) bool { return upcoming[i].DueAt.Before(upcoming[j].DueAt) })
	return upcoming, nil
}

func (f *fakeChoresStore) ListDueOccurrences(_ context.Context, dueBefore time.Time) ([]chores.Occurrence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var due []chores.Occurrence
	for _, occ := range f.occurrences {
		if occ.Status == "open" && !occ.DueAt.After(dueBefore) {
			due = append(due, *occ)
		}
	}
	return due, nil
}

func TestChores_DefinitionsAndScheduling(t *testing.T) {
	ctx := context.Background()
	loc := time.UTC
	clock := &fakeClock{
		now: time.Date(2026, time.October, 6, 12, 0, 0, 0, loc),
		loc: loc,
	}
	members := &fakeMemberChecker{
		members: map[int64]*household.Member{
			1001: {ID: 1001, Name: "Alice", Active: true},
			1002: {ID: 1002, Name: "Bob", Active: true},
		},
	}
	store := newFakeChoresStore()
	svc := chores.NewService(store, members, clock)

	// Blank name rejected
	_, err := svc.CreateDefinition(ctx, 1001, "   ", "🧹", "", nil)
	if !errors.Is(err, chores.ErrBlankName) {
		t.Fatalf("expected ErrBlankName, got %v", err)
	}

	// Create valid definition
	def, err := svc.CreateDefinition(ctx, 1001, "Sweep Floor", "🧹", "", &recurrence.Rule{
		Interval: 1,
		Unit:     recurrence.UnitWeek,
		Anchor:   recurrence.AnchorScheduled,
	})
	if err != nil || def.ID == 0 {
		t.Fatalf("failed to create chore definition: %v", err)
	}

	// Due time in the past rejected
	pastTime := clock.now.Add(-time.Hour)
	_, err = svc.ScheduleOccurrence(ctx, 1001, def.ID, 1001, pastTime)
	if !errors.Is(err, chores.ErrPastDueTime) {
		t.Fatalf("expected ErrPastDueTime, got %v", err)
	}

	// Future valid scheduling
	dueTime := clock.now.Add(24 * time.Hour)
	occ, err := svc.ScheduleOccurrence(ctx, 1001, def.ID, 1001, dueTime)
	if err != nil {
		t.Fatalf("ScheduleOccurrence failed: %v", err)
	}
	if !occ.ScheduledAt.Equal(dueTime) || !occ.DueAt.Equal(dueTime) {
		t.Fatalf("expected scheduled_at == due_at, got sched=%v, due=%v", occ.ScheduledAt, occ.DueAt)
	}
	if occ.Status != "open" {
		t.Fatalf("expected status open, got %s", occ.Status)
	}

	// Abandoned flow creates nothing: if user leaves without ScheduleOccurrence, no occurrence exists
	allMyOpen, err := svc.ListMyOpen(ctx, 1002) // Bob has none
	if err != nil || len(allMyOpen.Occurrences) != 0 {
		t.Fatalf("Bob should have 0 open chores")
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

func TestChores_Complete_AssigneeOnly_Idempotent_Early(t *testing.T) {
	ctx := context.Background()
	loc := time.UTC
	clock := &fakeClock{
		now: time.Date(2026, time.October, 6, 12, 0, 0, 0, loc),
		loc: loc,
	}
	members := &fakeMemberChecker{
		members: map[int64]*household.Member{
			1001: {ID: 1001, Name: "Alice", Active: true},
			1002: {ID: 1002, Name: "Bob", Active: true},
		},
	}
	store := newFakeChoresStore()
	svc := chores.NewService(store, members, clock)

	def, _ := svc.CreateDefinition(ctx, 1001, "Take out trash", "🗑", "", &recurrence.Rule{
		Interval: 1,
		Unit:     recurrence.UnitWeek,
		Anchor:   recurrence.AnchorScheduled,
	})

	dueTime := clock.now.Add(48 * time.Hour) // Due Thursday
	occ, _ := svc.ScheduleOccurrence(ctx, 1001, def.ID, 1001, dueTime)

	// Non-assignee Bob attempts to complete -> rejected
	_, _, err := svc.Complete(ctx, 1002, occ.ID, clock.now)
	if !errors.Is(err, chores.ErrNotAssignee) {
		t.Fatalf("expected ErrNotAssignee when Bob completes Alice's chore, got %v", err)
	}

	// Early completion by Alice (before dueTime) -> succeeds and generates successor
	completed, succ, err := svc.Complete(ctx, 1001, occ.ID, clock.now)
	if err != nil {
		t.Fatalf("early completion failed: %v", err)
	}
	if completed.Status != "completed" || *completed.CompletedBy != 1001 {
		t.Fatalf("unexpected completion state: %+v", completed)
	}
	if succ == nil {
		t.Fatalf("expected recurring successor to be generated")
	}
	if !succ.ScheduledAt.After(occ.ScheduledAt) {
		t.Fatalf("expected successor scheduled after current: %v <= %v", succ.ScheduledAt, occ.ScheduledAt)
	}

	// Repeated Done by Alice -> idempotent, returns completed without duplicate successor
	completed2, succ2, err := svc.Complete(ctx, 1001, occ.ID, clock.now)
	if err != nil {
		t.Fatalf("second Done failed: %v", err)
	}
	if succ2 != nil {
		t.Fatalf("expected second Done to not create second successor")
	}
	if completed2.ID != completed.ID {
		t.Fatalf("expected same completed occurrence ID")
	}
}

func TestChores_Postpone(t *testing.T) {
	ctx := context.Background()
	loc := time.UTC
	clock := &fakeClock{
		now: time.Date(2026, time.October, 6, 12, 0, 0, 0, loc),
		loc: loc,
	}
	members := &fakeMemberChecker{
		members: map[int64]*household.Member{
			1001: {ID: 1001, Name: "Alice", Active: true},
			1002: {ID: 1002, Name: "Bob", Active: true},
		},
	}
	store := newFakeChoresStore()
	svc := chores.NewService(store, members, clock)

	def, _ := svc.CreateDefinition(ctx, 1001, "Wash Dishes", "🍽", "", nil)
	dueTime := clock.now.Add(2 * time.Hour)
	occ, _ := svc.ScheduleOccurrence(ctx, 1001, def.ID, 1001, dueTime)

	// Bob cannot postpone
	_, err := svc.Postpone(ctx, 1002, occ.ID, dueTime.Add(3*time.Hour))
	if !errors.Is(err, chores.ErrNotAssignee) {
		t.Fatalf("expected ErrNotAssignee, got %v", err)
	}

	// Bob with a past time still gets ErrNotAssignee, not a time error
	_, err = svc.Postpone(ctx, 1002, occ.ID, clock.now.Add(-time.Hour))
	if !errors.Is(err, chores.ErrNotAssignee) {
		t.Fatalf("expected ErrNotAssignee for non-assignee with past time, got %v", err)
	}

	// Alice with a past time gets ErrPastDueTime
	_, err = svc.Postpone(ctx, 1001, occ.ID, clock.now.Add(-time.Hour))
	if !errors.Is(err, chores.ErrPastDueTime) {
		t.Fatalf("expected ErrPastDueTime for assignee with past time, got %v", err)
	}

	// Postpone by 3 hours
	newDue := dueTime.Add(3 * time.Hour)
	postponed, err := svc.Postpone(ctx, 1001, occ.ID, newDue)
	if err != nil {
		t.Fatalf("postpone failed: %v", err)
	}
	if !postponed.DueAt.Equal(newDue) {
		t.Fatalf("expected due_at %v, got %v", newDue, postponed.DueAt)
	}
	// scheduled_at must remain unchanged!
	if !postponed.ScheduledAt.Equal(dueTime) {
		t.Fatalf("scheduled_at was modified: %v != %v", postponed.ScheduledAt, dueTime)
	}
}

func TestChores_ListMyOpen_Isolation_And35ItemsCap(t *testing.T) {
	ctx := context.Background()
	loc := time.UTC
	clock := &fakeClock{
		now: time.Date(2026, time.October, 6, 12, 0, 0, 0, loc),
		loc: loc,
	}
	members := &fakeMemberChecker{
		members: map[int64]*household.Member{
			1001: {ID: 1001, Name: "Alice", Active: true},
			1002: {ID: 1002, Name: "Bob", Active: true},
		},
	}
	store := newFakeChoresStore()
	svc := chores.NewService(store, members, clock)

	def, _ := svc.CreateDefinition(ctx, 1001, "Chore", "🧹", "", nil)

	// Bob has 1 chore
	_, _ = svc.ScheduleOccurrence(ctx, 1002, def.ID, 1002, clock.now.Add(time.Hour))

	// Alice has 35 chores with varying due dates
	for i := 1; i <= 35; i++ {
		due := clock.now.Add(time.Duration(i) * time.Hour)
		_, _ = svc.ScheduleOccurrence(ctx, 1001, def.ID, 1001, due)
	}

	// Check Alice's list: Bob's chore must not appear, cap at 20, +15 more
	res, err := svc.ListMyOpen(ctx, 1001)
	if err != nil {
		t.Fatalf("ListMyOpen failed: %v", err)
	}
	if res.TotalOpen != 35 {
		t.Fatalf("expected total 35, got %d", res.TotalOpen)
	}
	if len(res.Occurrences) != 20 {
		t.Fatalf("expected 20 occurrences, got %d", len(res.Occurrences))
	}
	if res.RemainingCount != 15 {
		t.Fatalf("expected 15 remaining, got %d", res.RemainingCount)
	}
	for _, o := range res.Occurrences {
		if o.AssigneeID != 1001 {
			t.Fatalf("found chore belonging to another member: %+v", o)
		}
	}
}

func TestChores_ListUpcoming_ReadOnlyLookahead(t *testing.T) {
	ctx := context.Background()
	loc := time.UTC
	clock := &fakeClock{now: time.Date(2026, time.October, 6, 12, 0, 0, 0, loc), loc: loc}
	members := &fakeMemberChecker{members: map[int64]*household.Member{1001: {ID: 1001, Name: "Alice", Active: true}}}
	svc := chores.NewService(newFakeChoresStore(), members, clock)

	def, _ := svc.CreateDefinition(ctx, 1001, "Wash Dishes", "🍽", "", nil)
	later, _ := svc.ScheduleOccurrence(ctx, 1001, def.ID, 1001, clock.now.Add(30*time.Hour))
	sooner, _ := svc.ScheduleOccurrence(ctx, 1001, def.ID, 1001, clock.now.Add(2*time.Hour))
	_, _ = svc.ScheduleOccurrence(ctx, 1001, def.ID, 1001, clock.now.Add(100*time.Hour))

	got, err := svc.ListUpcoming(ctx, clock.now, clock.now.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("ListUpcoming failed: %v", err)
	}
	if len(got) != 2 || got[0].ID != sooner.ID || got[1].ID != later.ID {
		t.Fatalf("expected [sooner, later] by due_at, got %+v", got)
	}
	// Lookahead must not change anything.
	if after, _ := svc.GetOccurrence(ctx, sooner.ID); after.Status != "open" || !after.DueAt.Equal(sooner.DueAt) {
		t.Fatalf("ListUpcoming modified an occurrence: %+v", after)
	}
}

func TestChores_Postpone_RejectsResolvedOccurrence(t *testing.T) {
	ctx := context.Background()
	loc := time.UTC
	clock := &fakeClock{now: time.Date(2026, time.October, 6, 12, 0, 0, 0, loc), loc: loc}
	members := &fakeMemberChecker{members: map[int64]*household.Member{
		1001: {ID: 1001, Name: "Alice", Active: true},
		1002: {ID: 1002, Name: "Bob", Active: true},
	}}
	svc := chores.NewService(newFakeChoresStore(), members, clock)

	def, _ := svc.CreateDefinition(ctx, 1001, "Wash Dishes", "🍽", "", nil)
	due := clock.now.Add(2 * time.Hour)
	occ, _ := svc.ScheduleOccurrence(ctx, 1001, def.ID, 1001, due)
	if _, _, err := svc.Complete(ctx, 1001, occ.ID, clock.now); err != nil {
		t.Fatalf("complete failed: %v", err)
	}

	// Order: authorization first, then open check, then time validation.
	if _, err := svc.Postpone(ctx, 1002, occ.ID, clock.now.Add(-time.Hour)); !errors.Is(err, chores.ErrNotAssignee) {
		t.Fatalf("non-assignee must get ErrNotAssignee even for a resolved occurrence, got %v", err)
	}
	if _, err := svc.Postpone(ctx, 1001, occ.ID, clock.now.Add(-time.Hour)); !errors.Is(err, chores.ErrOccurrenceResolved) {
		t.Fatalf("resolved check must precede time validation, got %v", err)
	}
	if _, err := svc.Postpone(ctx, 1001, occ.ID, due.Add(3*time.Hour)); !errors.Is(err, chores.ErrOccurrenceResolved) {
		t.Fatalf("expected ErrOccurrenceResolved, got %v", err)
	}
	if after, _ := svc.GetOccurrence(ctx, occ.ID); !after.DueAt.Equal(due) {
		t.Fatalf("completed occurrence's due_at changed to %v", after.DueAt)
	}
}
