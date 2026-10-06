package scheduler_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/px1/home-ctl/internal/chores"
	"github.com/px1/home-ctl/internal/household"
	"github.com/px1/home-ctl/internal/notification"
	"github.com/px1/home-ctl/internal/reminders"
	"github.com/px1/home-ctl/internal/scheduler"
)

type fakeNotifier struct {
	mu           sync.Mutex
	privateSends []privateCall
	groupPosts   []groupCall
	failNext     error
}

type privateCall struct {
	TargetUserID int64
	EntityType   string
	OccID        int64
	Title        string
	DueAt        time.Time
}

type groupCall struct {
	TopicID int64
	Text    string
}

func (f *fakeNotifier) SendPrivateNotification(_ context.Context, targetUserID int64, entityType string, occID int64, title string, dueAt time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext != nil {
		err := f.failNext
		f.failNext = nil
		return 0, err
	}
	f.privateSends = append(f.privateSends, privateCall{
		TargetUserID: targetUserID,
		EntityType:   entityType,
		OccID:        occID,
		Title:        title,
		DueAt:        dueAt,
	})
	return int64(100 + len(f.privateSends)), nil
}

func (f *fakeNotifier) SendGroupTopicUpdate(_ context.Context, topicID int64, text string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext != nil {
		err := f.failNext
		f.failNext = nil
		return 0, err
	}
	f.groupPosts = append(f.groupPosts, groupCall{
		TopicID: topicID,
		Text:    text,
	})
	return int64(200 + len(f.groupPosts)), nil
}

type fakeOnboarding struct {
	mu        sync.Mutex
	connected map[int64]bool
}

func (f *fakeOnboarding) IsConnected(_ context.Context, userID int64) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.connected[userID], nil
}

func (f *fakeOnboarding) HandleDisconnect(_ context.Context, userID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connected[userID] = false
	return nil
}

type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

func (c *fakeClock) Location() *time.Location {
	return time.UTC
}

type memNotifStore struct {
	mu       sync.Mutex
	attempts []notification.Attempt
	nextID   int64
}

func newMemNotifStore() *memNotifStore {
	return &memNotifStore{nextID: 1}
}

func (m *memNotifStore) ClaimChoreAttempt(_ context.Context, occID, targetUserID int64, dueAt, attemptTime time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.attempts {
		if a.EntityType == "chore" && a.OccurrenceID == occID && a.DueAt.Equal(dueAt) && a.Outcome != "failed" {
			return 0, notification.ErrAlreadyClaimed
		}
	}
	att := notification.Attempt{
		ID:           m.nextID,
		EntityType:   "chore",
		OccurrenceID: occID,
		TargetUserID: targetUserID,
		DueAt:        dueAt,
		AttemptTime:  attemptTime,
		AttemptSeq:   1,
		Outcome:      "claimed",
		CreatedAt:    attemptTime,
	}
	m.nextID++
	m.attempts = append(m.attempts, att)
	return att.ID, nil
}

func (m *memNotifStore) ClaimReminderAttempt(_ context.Context, occID, targetUserID int64, dueAt, attemptTime time.Time) (int64, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	seq := 1
	for _, a := range m.attempts {
		if a.EntityType == "reminder" && a.OccurrenceID == occID && a.DueAt.Equal(dueAt) {
			if a.AttemptSeq >= seq {
				seq = a.AttemptSeq + 1
			}
		}
	}
	att := notification.Attempt{
		ID:           m.nextID,
		EntityType:   "reminder",
		OccurrenceID: occID,
		TargetUserID: targetUserID,
		DueAt:        dueAt,
		AttemptTime:  attemptTime,
		AttemptSeq:   seq,
		Outcome:      "claimed",
		CreatedAt:    attemptTime,
	}
	m.nextID++
	m.attempts = append(m.attempts, att)
	return att.ID, seq, nil
}

func (m *memNotifStore) UpdateAttemptOutcome(_ context.Context, attemptID int64, outcome, failureReason string, msgID *int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.attempts {
		if m.attempts[i].ID == attemptID {
			m.attempts[i].Outcome = outcome
			m.attempts[i].FailureReason = failureReason
			m.attempts[i].TelegramMessageID = msgID
			return nil
		}
	}
	return errors.New("attempt not found")
}

func (m *memNotifStore) ListAttempts(_ context.Context, entityType string, occID int64) ([]notification.Attempt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []notification.Attempt
	for _, a := range m.attempts {
		if a.EntityType == entityType && a.OccurrenceID == occID {
			list = append(list, a)
		}
	}
	return list, nil
}

type stubChoresStore struct {
	due []chores.Occurrence
}

func (s *stubChoresStore) CreateDefinition(_ context.Context, _ chores.Definition) (*chores.Definition, error) {
	return nil, nil
}
func (s *stubChoresStore) GetDefinition(_ context.Context, _ int64) (*chores.Definition, error) {
	return nil, nil
}
func (s *stubChoresStore) ListDefinitions(_ context.Context) ([]chores.Definition, error) {
	return nil, nil
}
func (s *stubChoresStore) CreateOccurrence(_ context.Context, _ chores.Occurrence) (*chores.Occurrence, error) {
	return nil, nil
}
func (s *stubChoresStore) GetOccurrence(_ context.Context, _ int64) (*chores.Occurrence, error) {
	return nil, nil
}
func (s *stubChoresStore) CompleteOccurrence(_ context.Context, _, _ int64, _ time.Time, _ *time.Time) (*chores.Occurrence, *chores.Occurrence, error) {
	return nil, nil, nil
}
func (s *stubChoresStore) PostponeOccurrence(_ context.Context, _, _ int64, _, _ time.Time) (*chores.Occurrence, error) {
	return nil, nil
}
func (s *stubChoresStore) ListMyOpenOccurrences(_ context.Context, _ int64, _ int) (chores.ListResult, error) {
	return chores.ListResult{}, nil
}
func (s *stubChoresStore) ListUpcomingOccurrences(_ context.Context, _, _ time.Time) ([]chores.Occurrence, error) {
	return nil, nil
}
func (s *stubChoresStore) ListDueOccurrences(_ context.Context, _ time.Time) ([]chores.Occurrence, error) {
	return s.due, nil
}

type stubRemindersStore struct {
	due          []reminders.Occurrence
	lastNotified map[int64]*time.Time
}

func (s *stubRemindersStore) CreateDefinition(_ context.Context, _ reminders.Definition) (*reminders.Definition, error) {
	return nil, nil
}
func (s *stubRemindersStore) GetDefinition(_ context.Context, _ int64) (*reminders.Definition, error) {
	return nil, nil
}
func (s *stubRemindersStore) ListDefinitions(_ context.Context) ([]reminders.Definition, error) {
	return nil, nil
}
func (s *stubRemindersStore) CreateOccurrence(_ context.Context, _ reminders.Occurrence) (*reminders.Occurrence, error) {
	return nil, nil
}
func (s *stubRemindersStore) GetOccurrence(_ context.Context, _ int64) (*reminders.Occurrence, error) {
	return nil, nil
}
func (s *stubRemindersStore) CompleteOccurrence(_ context.Context, _, _ int64, _ time.Time, _ *time.Time) (*reminders.Occurrence, *reminders.Occurrence, error) {
	return nil, nil, nil
}
func (s *stubRemindersStore) PostponeOccurrence(_ context.Context, _, _ int64, _, _ time.Time) (*reminders.Occurrence, error) {
	return nil, nil
}
func (s *stubRemindersStore) ListMyOpenOccurrences(_ context.Context, _ int64, _ int) (reminders.ListResult, error) {
	return reminders.ListResult{}, nil
}
func (s *stubRemindersStore) ListUpcomingOccurrences(_ context.Context, _, _ time.Time) ([]reminders.Occurrence, error) {
	return nil, nil
}
func (s *stubRemindersStore) ListDueOccurrences(_ context.Context, _ time.Time) ([]reminders.Occurrence, error) {
	return s.due, nil
}
func (s *stubRemindersStore) GetLastNotificationTime(_ context.Context, occID int64) (*time.Time, error) {
	return s.lastNotified[occID], nil
}

type stubMembers struct{}

func (s *stubMembers) Authorize(_ context.Context, id int64) (*household.Member, error) {
	return &household.Member{ID: id, Active: true}, nil
}

func TestScheduler_Scenarios(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.October, 6, 18, 0, 0, 0, time.UTC)
	clk := &fakeClock{now: now}
	onb := &fakeOnboarding{connected: map[int64]bool{1001: true, 1002: false}}
	notifier := &fakeNotifier{}
	nStore := newMemNotifStore()

	cStore := &stubChoresStore{
		due: []chores.Occurrence{
			{
				ID:           1,
				DefinitionID: 10,
				Definition:   &chores.Definition{Name: "Clean room"},
				AssigneeID:   1001,                       // connected
				DueAt:        now.Add(-10 * time.Minute), // Missed while down!
				Status:       "open",
			},
			{
				ID:           2,
				DefinitionID: 20,
				Definition:   &chores.Definition{Name: "Wash windows"},
				AssigneeID:   1002, // unconnected
				DueAt:        now,
				Status:       "open",
			},
		},
	}

	rStore := &stubRemindersStore{
		lastNotified: make(map[int64]*time.Time),
	}
	rSvc := reminders.NewService(rStore, &stubMembers{}, clk)

	sched := scheduler.New(scheduler.Config{
		ChoresStore:      cStore,
		RemindersStore:   rStore,
		RemindersService: rSvc,
		NotifStore:       nStore,
		Notifier:         notifier,
		Onboarding:       onb,
		Clock:            clk,
		ReminderInterval: time.Hour,
	})

	// 1. Run first scan
	err := sched.ScanAndNotify(ctx)
	if err != nil {
		t.Fatalf("first scan failed: %v", err)
	}

	// Chore 1 (Alice, missed while down): exactly 1 private notification sent
	if len(notifier.privateSends) != 1 {
		t.Fatalf("expected 1 private send, got %d", len(notifier.privateSends))
	}
	if notifier.privateSends[0].OccID != 1 {
		t.Fatalf("expected send for chore 1, got %d", notifier.privateSends[0].OccID)
	}

	// Chore 2 (Bob, unconnected): failed attempt recorded with reason "not connected", chore stays open
	att2, err := nStore.ListAttempts(ctx, "chore", 2)
	if err != nil || len(att2) != 1 {
		t.Fatalf("expected 1 attempt record for unconnected user, got %v", att2)
	}
	if att2[0].Outcome != "failed" || att2[0].FailureReason != "not connected" {
		t.Fatalf("expected failed with 'not connected', got %+v", att2[0])
	}

	// 2. Restart simulation: second scan immediately
	// Chore 1 was already claimed and sent -> no duplicate notification!
	err = sched.ScanAndNotify(ctx)
	if err != nil {
		t.Fatalf("second scan failed: %v", err)
	}
	if len(notifier.privateSends) != 1 {
		t.Fatalf("expected NO duplicate send across restart/rescan, got %d sends", len(notifier.privateSends))
	}

	// 3. Unconnected user connects, retries on next scan
	onb.connected[1002] = true
	err = sched.ScanAndNotify(ctx)
	if err != nil {
		t.Fatalf("third scan failed: %v", err)
	}
	if len(notifier.privateSends) != 2 {
		t.Fatalf("expected retry send for Bob now that he is connected, got %d sends", len(notifier.privateSends))
	}
}

func TestScheduler_PostTopicStatusUpdate_FailurePreservesState(t *testing.T) {
	ctx := context.Background()
	notifier := &fakeNotifier{
		failNext: errors.New("network outage"),
	}

	// Status update fails to send
	scheduler.PostTopicStatusUpdate(ctx, notifier, 42, "Purchase completed", nil)

	// Function does not panic and returns safely
	if len(notifier.groupPosts) != 0 {
		t.Fatalf("expected 0 successful group posts, got %d", len(notifier.groupPosts))
	}
}

type errChoresStore struct {
	stubChoresStore
}

func (e *errChoresStore) ListDueOccurrences(_ context.Context, _ time.Time) ([]chores.Occurrence, error) {
	return nil, errors.New("db error")
}

type errRemindersStore struct {
	stubRemindersStore
}

func (e *errRemindersStore) ListDueOccurrences(_ context.Context, _ time.Time) ([]reminders.Occurrence, error) {
	return nil, errors.New("db error")
}

func TestScheduler_ErrorBranches_AndBlockedDisconnect(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	clk := &fakeClock{now: now}
	onb := &fakeOnboarding{connected: map[int64]bool{1001: true}}
	nStore := newMemNotifStore()

	// 1. Chores store error
	schedErrChores := scheduler.New(scheduler.Config{
		ChoresStore:      &errChoresStore{},
		RemindersStore:   &stubRemindersStore{},
		RemindersService: reminders.NewService(&stubRemindersStore{}, &stubMembers{}, clk),
		NotifStore:       nStore,
		Notifier:         &fakeNotifier{},
		Onboarding:       onb,
		Clock:            clk,
	})
	if err := schedErrChores.ScanAndNotify(ctx); err == nil {
		t.Fatalf("expected error when chores store fails")
	}

	// 2. Reminders store error
	schedErrReminders := scheduler.New(scheduler.Config{
		ChoresStore:      &stubChoresStore{},
		RemindersStore:   &errRemindersStore{},
		RemindersService: reminders.NewService(&stubRemindersStore{}, &stubMembers{}, clk),
		NotifStore:       nStore,
		Notifier:         &fakeNotifier{},
		Onboarding:       onb,
		Clock:            clk,
	})
	if err := schedErrReminders.ScanAndNotify(ctx); err == nil {
		t.Fatalf("expected error when reminders store fails")
	}

	// 3. User blocked bot triggers disconnect
	blockedNotifier := &fakeNotifier{
		failNext: errors.New("Forbidden: bot was blocked by the user"),
	}
	cStore := &stubChoresStore{
		due: []chores.Occurrence{
			{
				ID:           99,
				DefinitionID: 1,
				Definition:   &chores.Definition{Name: "Task"},
				AssigneeID:   1001,
				DueAt:        now,
				Status:       "open",
			},
		},
	}
	schedBlocked := scheduler.New(scheduler.Config{
		ChoresStore:      cStore,
		RemindersStore:   &stubRemindersStore{},
		RemindersService: reminders.NewService(&stubRemindersStore{}, &stubMembers{}, clk),
		NotifStore:       nStore,
		Notifier:         blockedNotifier,
		Onboarding:       onb,
		Clock:            clk,
	})
	_ = schedBlocked.ScanAndNotify(ctx)
	if onb.connected[1001] {
		t.Fatalf("expected user 1001 to be disconnected after blocking bot")
	}
}

// notifBackedRemindersStore derives the last notification time from recorded attempts,
// like the SQLite store does, so repeat behavior is driven by real attempt history.
type notifBackedRemindersStore struct {
	stubRemindersStore
	notif   *memNotifStore
	lastErr error
}

func (s *notifBackedRemindersStore) GetLastNotificationTime(_ context.Context, occID int64) (*time.Time, error) {
	if s.lastErr != nil {
		return nil, s.lastErr
	}
	s.notif.mu.Lock()
	defer s.notif.mu.Unlock()
	var last *time.Time
	for _, a := range s.notif.attempts {
		if a.EntityType == "reminder" && a.OccurrenceID == occID && (last == nil || a.AttemptTime.After(*last)) {
			t := a.AttemptTime
			last = &t
		}
	}
	return last, nil
}

func newReminderScheduler(clk *fakeClock, rStore *notifBackedRemindersStore, nStore notification.Store, cStore chores.Store, notifier *fakeNotifier) *scheduler.Scheduler {
	return scheduler.New(scheduler.Config{
		ChoresStore:      cStore,
		RemindersStore:   rStore,
		RemindersService: reminders.NewService(rStore, &stubMembers{}, clk),
		NotifStore:       nStore,
		Notifier:         notifier,
		Onboarding:       &fakeOnboarding{connected: map[int64]bool{1001: true, 1002: true}},
		Clock:            clk,
		ReminderInterval: time.Hour,
	})
}

func TestScheduler_ReminderRepeatsEveryInterval(t *testing.T) {
	ctx := context.Background()
	due := time.Date(2026, time.October, 6, 10, 0, 0, 0, time.UTC)
	clk := &fakeClock{now: due}
	nStore := newMemNotifStore()
	rStore := &notifBackedRemindersStore{notif: nStore}
	rStore.due = []reminders.Occurrence{{
		ID: 7, DefinitionID: 3, Definition: &reminders.Definition{Title: "Buy medicine"},
		TargetUserID: 1002, DueAt: due, ScheduledAt: due, Status: "open",
	}}
	notifier := &fakeNotifier{}
	sched := newReminderScheduler(clk, rStore, nStore, &stubChoresStore{}, notifier)

	// 10:00 sends, 10:30 does not, 11:00 repeats, 12:00 repeats.
	for _, step := range []struct {
		at        time.Duration
		wantSends int
	}{{0, 1}, {30 * time.Minute, 1}, {time.Hour, 2}, {2 * time.Hour, 3}} {
		clk.now = due.Add(step.at)
		if err := sched.ScanAndNotify(ctx); err != nil {
			t.Fatalf("scan at +%v failed: %v", step.at, err)
		}
		if got := len(notifier.privateSends); got != step.wantSends {
			t.Fatalf("at +%v: expected %d sends, got %d", step.at, step.wantSends, got)
		}
	}
	if notifier.privateSends[0].EntityType != "reminder" || notifier.privateSends[0].Title != "Buy medicine" {
		t.Fatalf("unexpected notification payload: %+v", notifier.privateSends[0])
	}
}

func TestScheduler_ReminderNotYetDueIsSkipped(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.October, 6, 9, 0, 0, 0, time.UTC)
	clk := &fakeClock{now: now}
	nStore := newMemNotifStore()
	rStore := &notifBackedRemindersStore{notif: nStore}
	rStore.due = []reminders.Occurrence{{
		ID: 8, Definition: &reminders.Definition{Title: "Later"}, TargetUserID: 1002,
		DueAt: now.Add(time.Hour), Status: "open",
	}}
	notifier := &fakeNotifier{}
	sched := newReminderScheduler(clk, rStore, nStore, &stubChoresStore{}, notifier)

	if err := sched.ScanAndNotify(ctx); err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if len(notifier.privateSends) != 0 {
		t.Fatalf("expected no send before due_at, got %d", len(notifier.privateSends))
	}
}

// failingClaimStore fails claims for chore 1 and reminder 7 with a non-idempotency error.
type failingClaimStore struct {
	*memNotifStore
}

func (f *failingClaimStore) ClaimChoreAttempt(ctx context.Context, occID, userID int64, dueAt, at time.Time) (int64, error) {
	if occID == 1 {
		return 0, errors.New("db locked")
	}
	return f.memNotifStore.ClaimChoreAttempt(ctx, occID, userID, dueAt, at)
}

func (f *failingClaimStore) ClaimReminderAttempt(ctx context.Context, occID, userID int64, dueAt, at time.Time) (int64, int, error) {
	if occID == 7 {
		return 0, 0, errors.New("db locked")
	}
	return f.memNotifStore.ClaimReminderAttempt(ctx, occID, userID, dueAt, at)
}

func TestScheduler_ClaimFailureDoesNotBlockOtherOccurrences(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.October, 6, 18, 0, 0, 0, time.UTC)
	clk := &fakeClock{now: now}
	mem := newMemNotifStore()
	nStore := &failingClaimStore{mem}
	rStore := &notifBackedRemindersStore{notif: mem}
	rStore.due = []reminders.Occurrence{
		{ID: 7, Definition: &reminders.Definition{Title: "Broken"}, TargetUserID: 1002, DueAt: now, Status: "open"},
		{ID: 9, Definition: &reminders.Definition{Title: "Fine"}, TargetUserID: 1002, DueAt: now, Status: "open"},
	}
	cStore := &stubChoresStore{due: []chores.Occurrence{
		{ID: 1, Definition: &chores.Definition{Name: "Broken"}, AssigneeID: 1001, DueAt: now, Status: "open"},
		{ID: 2, Definition: &chores.Definition{Name: "Fine"}, AssigneeID: 1001, DueAt: now, Status: "open"},
	}}
	notifier := &fakeNotifier{}
	sched := newReminderScheduler(clk, rStore, nStore, cStore, notifier)

	if err := sched.ScanAndNotify(ctx); err != nil {
		t.Fatalf("scan must tolerate per-occurrence claim failures, got %v", err)
	}
	var sentIDs []int64
	for _, s := range notifier.privateSends {
		sentIDs = append(sentIDs, s.OccID)
	}
	if len(sentIDs) != 2 || sentIDs[0] != 2 || sentIDs[1] != 9 {
		t.Fatalf("expected only chore 2 and reminder 9 to be notified, got %v", sentIDs)
	}
}

func TestScheduler_ReminderHistoryErrorSkipsOnlyThatReminder(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.October, 6, 18, 0, 0, 0, time.UTC)
	clk := &fakeClock{now: now}
	nStore := newMemNotifStore()
	rStore := &notifBackedRemindersStore{notif: nStore, lastErr: errors.New("db error")}
	rStore.due = []reminders.Occurrence{
		{ID: 7, Definition: &reminders.Definition{Title: "R"}, TargetUserID: 1002, DueAt: now, Status: "open"},
	}
	notifier := &fakeNotifier{}
	sched := newReminderScheduler(clk, rStore, nStore, &stubChoresStore{}, notifier)

	if err := sched.ScanAndNotify(ctx); err != nil {
		t.Fatalf("scan must not fail on a single reminder history error, got %v", err)
	}
	if len(notifier.privateSends) != 0 {
		t.Fatalf("expected no send when notification history is unreadable, got %d", len(notifier.privateSends))
	}
}

func TestScheduler_FailedDeliveryIsRetriedNextScan(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.October, 6, 18, 0, 0, 0, time.UTC)
	clk := &fakeClock{now: now}
	nStore := newMemNotifStore()
	cStore := &stubChoresStore{due: []chores.Occurrence{
		{ID: 5, Definition: &chores.Definition{Name: "Dishes"}, AssigneeID: 1001, DueAt: now, Status: "open"},
	}}
	notifier := &fakeNotifier{failNext: errors.New("telegram outage")}
	rStore := &notifBackedRemindersStore{notif: nStore}
	sched := newReminderScheduler(clk, rStore, nStore, cStore, notifier)

	if err := sched.ScanAndNotify(ctx); err != nil {
		t.Fatalf("first scan failed: %v", err)
	}
	if len(notifier.privateSends) != 0 {
		t.Fatalf("expected the first delivery to fail, got %d sends", len(notifier.privateSends))
	}
	if err := sched.ScanAndNotify(ctx); err != nil {
		t.Fatalf("second scan failed: %v", err)
	}
	if len(notifier.privateSends) != 1 {
		t.Fatalf("expected a retry after the outage, got %d sends", len(notifier.privateSends))
	}
	attempts, _ := nStore.ListAttempts(ctx, "chore", 5)
	if len(attempts) != 2 || attempts[0].Outcome != "failed" || attempts[1].Outcome != "sent" {
		t.Fatalf("expected failed then sent attempts, got %+v", attempts)
	}
}
